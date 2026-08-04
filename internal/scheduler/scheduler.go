// Package scheduler fires workflows whose trigger is "schedule"
// (internal/workflow.Trigger, ADR-0035) on their declared cron cadence,
// unattended — no HTTP request or CLI invocation supplies inputs or
// bindings the way a manual run does. It has two halves: Sync keeps the
// schedules table in step with whichever workflow version is currently
// installed, and Runner is the background loop that watches that table and
// calls runs.Execute once a row comes due.
package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/secrets"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

const (
	// OnMissedSkip drops any backlog of occurrences the scheduler could not
	// fire while the agent was offline and resumes at the next future one.
	// It is the default a Trigger with an empty OnMissed gets.
	OnMissedSkip = "skip"
	// OnMissedFireOnce runs once for the most recently missed occurrence,
	// then resumes normal cadence.
	OnMissedFireOnce = "fire_once"
)

// pollInterval bounds how long Runner sleeps between checking the
// schedules table for due rows. It cannot simply sleep until the earliest
// known next_run_at: `patchcord workflow install` runs as a separate
// process against the same SQLite file and has no way to wake an already
// running `patchcord serve` process the moment it writes a new or updated
// row.
const pollInterval = 30 * time.Second

// maxMissedOccurrences bounds how many times fire walks a schedule forward
// one cron occurrence at a time while counting a backlog. Beyond it, the
// exact count no longer matters — on_missed's decision is already the
// "more than one missed" branch either way — so fire jumps straight to
// cron.Schedule.Next(now) instead of continuing to iterate one occurrence
// at a time (relevant for a minute-level cron left offline for a long
// stretch).
const maxMissedOccurrences = 1000

// Sync brings the schedules table's row for def.ID in step with def's
// trigger: a "schedule" trigger upserts a row with a freshly computed
// next_run_at — installing a new version always reschedules from now,
// regardless of what an earlier version's cron was — and any other trigger
// deletes the row. Call it once per successful runs.InstallWorkflow (see
// internal/cli/workflow.go); def must already have passed workflow.Validate,
// so its cron expression (if any) is assumed well-formed.
func Sync(ctx context.Context, db *sql.DB, def *workflow.Definition) error {
	if def.Trigger.Type != "schedule" {
		if _, err := db.ExecContext(ctx, `DELETE FROM schedules WHERE workflow_id = ?`, def.ID); err != nil {
			return fmt.Errorf("remove schedule for workflow %q: %w", def.ID, err)
		}
		return nil
	}

	schedule, err := cron.ParseStandard(def.Trigger.Cron)
	if err != nil {
		return fmt.Errorf("parse cron expression %q: %w", def.Trigger.Cron, err)
	}

	onMissed := def.Trigger.OnMissed
	if onMissed == "" {
		onMissed = OnMissedSkip
	}

	nextRunAt := schedule.Next(time.Now())

	if _, err := db.ExecContext(ctx, `
		INSERT INTO schedules (workflow_id, cron, on_missed, next_run_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (workflow_id) DO UPDATE SET
			cron        = excluded.cron,
			on_missed   = excluded.on_missed,
			next_run_at = excluded.next_run_at,
			updated_at  = excluded.updated_at
	`, def.ID, def.Trigger.Cron, onMissed, nextRunAt); err != nil {
		return fmt.Errorf("schedule workflow %q: %w", def.ID, err)
	}

	return nil
}

// NextRunAt returns the next time workflowID's schedule will fire, and
// false if it has no "schedule" trigger installed.
func NextRunAt(ctx context.Context, db *sql.DB, workflowID string) (time.Time, bool, error) {
	var nextRunAt time.Time
	err := db.QueryRowContext(ctx, `SELECT next_run_at FROM schedules WHERE workflow_id = ?`, workflowID).Scan(&nextRunAt)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("load next run of workflow %q: %w", workflowID, err)
	}
	return nextRunAt, true, nil
}

// Runner polls the schedules table and fires each row's workflow through
// runs.Execute once it comes due, the same run path internal/api's
// handleRunWorkflow and the CLI's `workflow run` use — a scheduled run is
// not a different kind of run, just a differently triggered one.
type Runner struct {
	db       *sql.DB
	executor runs.ActionExecutor
	logger   *slog.Logger
	secrets  secrets.Store
}

// NewRunner builds a Runner. logger defaults to slog.Default() when nil,
// secretStore to secrets.EnvStore{} when nil — same default a directly
// built runs.ExecuteOptions{} falls back to, so a scheduled run resolves
// connector secrets exactly like a manual one unless the caller wires in
// the agent's configured secrets.MultiStore (internal/runtime.NewAgent).
func NewRunner(db *sql.DB, executor runs.ActionExecutor, logger *slog.Logger, secretStore secrets.Store) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	if secretStore == nil {
		secretStore = secrets.EnvStore{}
	}
	return &Runner{db: db, executor: executor, logger: logger, secrets: secretStore}
}

// Run polls for due schedules immediately, then every pollInterval, until
// ctx is cancelled. It blocks until then, so callers run it in its own
// goroutine. ctx is also the context every fired run executes under —
// cancelling it (agent shutdown) cancels any scheduled run still in flight,
// exactly like a run started from an HTTP request (see
// internal/runtime.Agent's runCtx).
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	r.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *Runner) tick(ctx context.Context) {
	now := time.Now()

	due, err := dueSchedules(ctx, r.db, now)
	if err != nil {
		r.logger.Error("list due schedules", slog.String("error", err.Error()))
		return
	}

	for _, s := range due {
		r.fire(ctx, s, now)
	}
}

// scheduleRow is one row of the schedules table.
type scheduleRow struct {
	WorkflowID string
	Cron       string
	OnMissed   string
	NextRunAt  time.Time
}

func dueSchedules(ctx context.Context, db *sql.DB, now time.Time) ([]scheduleRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT workflow_id, cron, on_missed, next_run_at FROM schedules
		WHERE next_run_at <= ?
	`, now)
	if err != nil {
		return nil, fmt.Errorf("query due schedules: %w", err)
	}
	defer rows.Close()

	var due []scheduleRow
	for rows.Next() {
		var s scheduleRow
		if err := rows.Scan(&s.WorkflowID, &s.Cron, &s.OnMissed, &s.NextRunAt); err != nil {
			return nil, fmt.Errorf("scan schedule row: %w", err)
		}
		due = append(due, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query due schedules: %w", err)
	}

	return due, nil
}

// fire advances s past every occurrence due by now, deciding whether to
// actually start a run based on how many occurrences it caught up on:
// exactly one (the common, on-time case) always fires; more than one (the
// agent was offline across at least one full cron period) only fires when
// s.OnMissed is OnMissedFireOnce, and only once regardless of how many were
// missed. now is passed in rather than read from time.Now() so the
// occurrence-counting logic is deterministically testable.
func (r *Runner) fire(ctx context.Context, s scheduleRow, now time.Time) {
	schedule, err := cron.ParseStandard(s.Cron)
	if err != nil {
		// Cron expressions are validated at install time (workflow.Validate),
		// so a stored row failing to parse means it predates a fix to that
		// validation — drop it rather than retry it forever every tick.
		r.logger.Error("stored schedule has an invalid cron expression, removing it",
			slog.String("workflow_id", s.WorkflowID), slog.String("error", err.Error()))
		if _, err := r.db.ExecContext(ctx, `DELETE FROM schedules WHERE workflow_id = ?`, s.WorkflowID); err != nil {
			r.logger.Error("remove invalid schedule", slog.String("workflow_id", s.WorkflowID), slog.String("error", err.Error()))
		}
		return
	}

	missed := 0
	next := s.NextRunAt
	for !next.After(now) && missed < maxMissedOccurrences {
		missed++
		next = schedule.Next(next)
	}
	if !next.After(now) {
		next = schedule.Next(now)
	}

	shouldFire := missed == 1 || (missed > 1 && s.OnMissed == OnMissedFireOnce)

	if _, err := r.db.ExecContext(ctx, `
		UPDATE schedules SET next_run_at = ?, updated_at = CURRENT_TIMESTAMP WHERE workflow_id = ?
	`, next, s.WorkflowID); err != nil {
		r.logger.Error("advance schedule", slog.String("workflow_id", s.WorkflowID), slog.String("error", err.Error()))
		return
	}

	if !shouldFire {
		r.logger.Info("schedule caught up without firing",
			slog.String("workflow_id", s.WorkflowID), slog.Int("missed_occurrences", missed), slog.String("on_missed", s.OnMissed))
		return
	}

	go func() {
		if _, err := runs.Execute(ctx, r.db, r.executor, s.WorkflowID, map[string]any{}, map[string]string{}, runs.ExecuteOptions{Secrets: r.secrets}); err != nil {
			r.logger.Error("scheduled run failed", slog.String("workflow_id", s.WorkflowID), slog.String("error", err.Error()))
		}
	}()
}
