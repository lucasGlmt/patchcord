package runs

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"

	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/workflow"
	"github.com/lucasglmt/patchcord/migrations"
)

// openTestDB returns a freshly migrated, empty database.
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

// knownActions is the set of action ids the tests' fakeExecutor pretends
// are contributed by installed plugins.
var knownActions = map[string]struct{}{
	"text.uppercase@1": {},
}

// fakeExecutor is an in-memory ActionExecutor: it never launches a real
// process, keeping the runner's orchestration logic testable on its own,
// independent of internal/plugins (see internal/plugins.Supervisor for the
// real implementation, and the end-to-end test in internal/runtime).
// connectorsReceived records the connector each call in calls received, in
// the same order, so tests can assert a step's bound connector actually
// reached the executor (or that an unbound step received nil).
type fakeExecutor struct {
	responses map[string]map[string]any
	errs      map[string]error
	calls     []string

	connectorsReceived []*connectors.ResolvedConnector
}

func (f *fakeExecutor) ExecuteAction(_ context.Context, actionID string, _ map[string]any, connector *connectors.ResolvedConnector) (map[string]any, error) {
	f.calls = append(f.calls, actionID)
	f.connectorsReceived = append(f.connectorsReceived, connector)
	if err, ok := f.errs[actionID]; ok {
		return nil, err
	}
	return f.responses[actionID], nil
}

func installTestWorkflow(t *testing.T, db *sql.DB, source string) *workflow.Definition {
	t.Helper()
	def, err := InstallWorkflow(context.Background(), db, []byte(source), knownActions)
	if err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}
	return def
}
