package scheduler

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/workflow"
	"github.com/lucasglmt/patchcord/migrations"
)

// openTestDB returns a freshly migrated, empty database — same pattern as
// internal/runs's helpers_test.go.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatalf("persistence.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := persistence.Migrate(context.Background(), db, migrations.FS, logger); err != nil {
		t.Fatalf("persistence.Migrate() error = %v", err)
	}

	return db
}

var knownActions = map[string]struct{}{"text.uppercase@1": {}}

const scheduledWorkflowSource = `
schema_version: 1
id: nightly_report
version: 1
trigger:
  type: schedule
  cron: "*/5 * * * *"
steps:
  - id: step1
    uses: text.uppercase@1
    with:
      value: "hello"
`

func installScheduledWorkflow(t *testing.T, db *sql.DB) *workflow.Definition {
	t.Helper()
	def, err := runs.InstallWorkflow(context.Background(), db, []byte(scheduledWorkflowSource), knownActions)
	if err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}
	return def
}

// recordingExecutor is an in-memory runs.ActionExecutor that reports each
// call on a channel, so a test can synchronize on a scheduled run actually
// firing without sleeping.
type recordingExecutor struct {
	calls chan string
}

func newRecordingExecutor() *recordingExecutor {
	return &recordingExecutor{calls: make(chan string, 8)}
}

func (e *recordingExecutor) ExecuteAction(_ context.Context, actionID string, _ map[string]any, _ *connectors.ResolvedConnector) (map[string]any, error) {
	e.calls <- actionID
	return map[string]any{"value": "HELLO"}, nil
}

func (e *recordingExecutor) expectCall(t *testing.T) {
	t.Helper()
	select {
	case <-e.calls:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the scheduled workflow to fire")
	}
}

func (e *recordingExecutor) expectNoCall(t *testing.T) {
	t.Helper()
	select {
	case actionID := <-e.calls:
		t.Fatalf("workflow fired unexpectedly (action %q)", actionID)
	case <-time.After(200 * time.Millisecond):
	}
}

func scheduleRowFor(t *testing.T, db *sql.DB, workflowID string) (cron_ string, onMissed string, nextRunAt time.Time, found bool) {
	t.Helper()
	err := db.QueryRow(`SELECT cron, on_missed, next_run_at FROM schedules WHERE workflow_id = ?`, workflowID).
		Scan(&cron_, &onMissed, &nextRunAt)
	if err == sql.ErrNoRows {
		return "", "", time.Time{}, false
	}
	if err != nil {
		t.Fatalf("query schedules row: %v", err)
	}
	return cron_, onMissed, nextRunAt, true
}

func TestSync(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	def := installScheduledWorkflow(t, db)

	if err := Sync(ctx, db, def); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	gotCron, gotOnMissed, nextRunAt, found := scheduleRowFor(t, db, def.ID)
	if !found {
		t.Fatal("expected a schedules row after Sync, found none")
	}
	if gotCron != def.Trigger.Cron {
		t.Errorf("cron = %q, want %q", gotCron, def.Trigger.Cron)
	}
	if gotOnMissed != OnMissedSkip {
		t.Errorf("on_missed = %q, want default %q", gotOnMissed, OnMissedSkip)
	}
	if !nextRunAt.After(time.Now()) {
		t.Errorf("next_run_at = %v, want a time in the future", nextRunAt)
	}

	// Reinstalling with an explicit on_missed persists it.
	def.Trigger.OnMissed = OnMissedFireOnce
	if err := Sync(ctx, db, def); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if _, gotOnMissed, _, _ := scheduleRowFor(t, db, def.ID); gotOnMissed != OnMissedFireOnce {
		t.Errorf("on_missed = %q, want %q", gotOnMissed, OnMissedFireOnce)
	}

	// Switching to a manual trigger removes the row.
	def.Trigger = workflow.Trigger{Type: "manual"}
	if err := Sync(ctx, db, def); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if _, _, _, found := scheduleRowFor(t, db, def.ID); found {
		t.Error("expected the schedules row to be removed after switching to a manual trigger")
	}
}

func TestNextRunAt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, ok, err := NextRunAt(ctx, db, "nightly_report"); err != nil || ok {
		t.Fatalf("NextRunAt() = (_, %v, %v), want ok=false for an unscheduled workflow", ok, err)
	}

	def := installScheduledWorkflow(t, db)
	if err := Sync(ctx, db, def); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	next, ok, err := NextRunAt(ctx, db, def.ID)
	if err != nil || !ok {
		t.Fatalf("NextRunAt() = (_, %v, %v), want ok=true", ok, err)
	}
	if !next.After(time.Now()) {
		t.Errorf("NextRunAt() = %v, want a time in the future", next)
	}
}

// occurrences returns count consecutive occurrences of expr starting after
// anchor, so missed-occurrence tests can build scenarios ("now" past exactly
// one occurrence, or past several) without depending on real wall-clock
// cron boundaries.
func occurrences(t *testing.T, expr string, anchor time.Time, count int) []time.Time {
	t.Helper()
	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		t.Fatalf("cron.ParseStandard(%q) error = %v", expr, err)
	}
	out := make([]time.Time, count)
	t_ := anchor
	for i := range out {
		t_ = schedule.Next(t_)
		out[i] = t_
	}
	return out
}

func TestRunner_fire(t *testing.T) {
	const expr = "*/5 * * * *"
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	occ := occurrences(t, expr, anchor, 4) // occ[0] < occ[1] < occ[2] < occ[3]

	tests := []struct {
		name         string
		onMissed     string
		nextRunAt    time.Time
		now          time.Time
		wantFire     bool
		wantNextFrom time.Time // fire's new next_run_at must equal schedule.Next(wantNextFrom)
	}{
		{
			name:         "fires for the normal, on-time single occurrence",
			onMissed:     OnMissedSkip,
			nextRunAt:    occ[0],
			now:          occ[0].Add(time.Second),
			wantFire:     true,
			wantNextFrom: occ[0],
		},
		{
			name:         "skips a backlog of missed occurrences when on_missed is skip",
			onMissed:     OnMissedSkip,
			nextRunAt:    occ[0],
			now:          occ[2].Add(time.Second),
			wantFire:     false,
			wantNextFrom: occ[2],
		},
		{
			name:         "fires once for a backlog when on_missed is fire_once",
			onMissed:     OnMissedFireOnce,
			nextRunAt:    occ[0],
			now:          occ[2].Add(time.Second),
			wantFire:     true,
			wantNextFrom: occ[2],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			def := installScheduledWorkflow(t, db)

			if _, err := db.ExecContext(ctx, `
				INSERT INTO schedules (workflow_id, cron, on_missed, next_run_at)
				VALUES (?, ?, ?, ?)
			`, def.ID, expr, tt.onMissed, tt.nextRunAt); err != nil {
				t.Fatalf("seed schedules row: %v", err)
			}

			executor := newRecordingExecutor()
			r := NewRunner(db, executor, slog.New(slog.NewTextHandler(io.Discard, nil)))

			row := scheduleRow{WorkflowID: def.ID, Cron: expr, OnMissed: tt.onMissed, NextRunAt: tt.nextRunAt}
			r.fire(ctx, row, tt.now)

			if tt.wantFire {
				executor.expectCall(t)
			} else {
				executor.expectNoCall(t)
			}

			wantNext, err := cron.ParseStandard(expr)
			if err != nil {
				t.Fatalf("cron.ParseStandard() error = %v", err)
			}
			_, _, gotNextRunAt, found := scheduleRowFor(t, db, def.ID)
			if !found {
				t.Fatal("expected the schedules row to still exist after fire")
			}
			if want := wantNext.Next(tt.wantNextFrom); !gotNextRunAt.Equal(want) {
				t.Errorf("next_run_at = %v, want %v", gotNextRunAt, want)
			}
		})
	}
}

func TestRunner_tick_firesDueSchedule(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	def := installScheduledWorkflow(t, db)

	if err := Sync(ctx, db, def); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	// Sync always computes a future next_run_at; backdate it directly so
	// the very next tick finds it due.
	if _, err := db.ExecContext(ctx, `UPDATE schedules SET next_run_at = ? WHERE workflow_id = ?`,
		time.Now().Add(-time.Second), def.ID); err != nil {
		t.Fatalf("backdate schedules row: %v", err)
	}

	executor := newRecordingExecutor()
	r := NewRunner(db, executor, slog.New(slog.NewTextHandler(io.Discard, nil)))

	r.tick(ctx)

	executor.expectCall(t)
}

func TestRunner_tick_ignoresNotYetDueSchedule(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	def := installScheduledWorkflow(t, db)

	if err := Sync(ctx, db, def); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	executor := newRecordingExecutor()
	r := NewRunner(db, executor, slog.New(slog.NewTextHandler(io.Discard, nil)))

	r.tick(ctx)

	executor.expectNoCall(t)
}
