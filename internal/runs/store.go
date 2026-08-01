// Package runs is the run manager: it persists workflow versions, runs and
// their steps, and orchestrates execution on top of internal/workflow's
// engine and an ActionExecutor (typically internal/plugins.Supervisor).
package runs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lucasglmt/patchcord/internal/workflow"
)

// ErrWorkflowNotFound is returned when no workflow (or no such version of
// it) has been installed.
var ErrWorkflowNotFound = errors.New("workflow not found")

// ErrRunNotFound is returned when no run with the given id has been
// recorded.
var ErrRunNotFound = errors.New("run not found")

// InstallWorkflow validates def against knownActions, then records it as a
// new, immutable version (ADR-0008): publishing never overwrites an
// existing (workflow_id, version) row.
func InstallWorkflow(ctx context.Context, db *sql.DB, source []byte, knownActions map[string]struct{}) (*workflow.Definition, error) {
	def, err := workflow.Parse(source)
	if err != nil {
		return nil, err
	}
	if err := workflow.Validate(def, knownActions); err != nil {
		return nil, fmt.Errorf("validate workflow: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO workflow_versions (workflow_id, version, definition)
		VALUES (?, ?, ?)
	`, def.ID, def.Version, string(source))
	if err != nil {
		return nil, fmt.Errorf("record workflow %s version %d: %w", def.ID, def.Version, err)
	}

	return def, nil
}

// LatestWorkflow returns the highest installed version of workflowID.
func LatestWorkflow(ctx context.Context, db *sql.DB, workflowID string) (*workflow.Definition, error) {
	var source string
	err := db.QueryRowContext(ctx, `
		SELECT definition FROM workflow_versions
		WHERE workflow_id = ?
		ORDER BY version DESC
		LIMIT 1
	`, workflowID).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkflowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load latest version of workflow %q: %w", workflowID, err)
	}

	return workflow.Parse([]byte(source))
}

// Run is one execution of a workflow version, as persisted.
type Run struct {
	ID              string
	WorkflowID      string
	WorkflowVersion int
	Status          workflow.RunStatus
	Inputs          map[string]any
	Outputs         map[string]any
	Error           string
	CreatedAt       time.Time
}

// Step is one step of a Run, as persisted.
type Step struct {
	RunID  string
	StepID string
	Status workflow.StepStatus
	Input  map[string]any
	Output map[string]any
	Error  string
}

// createRun inserts a new run in the queued state and one pending row per
// step of def, all in a single transaction.
func createRun(ctx context.Context, db *sql.DB, def *workflow.Definition, inputs map[string]any) (*Run, error) {
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("encode run inputs: %w", err)
	}

	run := &Run{
		ID:              uuid.NewString(),
		WorkflowID:      def.ID,
		WorkflowVersion: def.Version,
		Status:          workflow.RunQueued,
		Inputs:          inputs,
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runs (id, workflow_id, workflow_version, status, inputs)
		VALUES (?, ?, ?, ?, ?)
	`, run.ID, run.WorkflowID, run.WorkflowVersion, string(run.Status), string(inputsJSON)); err != nil {
		return nil, fmt.Errorf("record run: %w", err)
	}

	for _, step := range def.Steps {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO run_steps (run_id, step_id, status)
			VALUES (?, ?, ?)
		`, run.ID, step.ID, string(workflow.StepPending)); err != nil {
			return nil, fmt.Errorf("record step %q: %w", step.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return run, nil
}

// updateRunStatus validates the transition, then persists it. On the
// transition into a terminal state, outputs and runErr (if any) are
// recorded and finished_at is set; on the transition out of queued,
// started_at is set.
func updateRunStatus(ctx context.Context, db *sql.DB, run *Run, to workflow.RunStatus, outputs map[string]any, runErr error) error {
	if err := workflow.ValidateRunTransition(run.Status, to); err != nil {
		return err
	}

	outputsJSON, err := json.Marshal(outputs)
	if err != nil {
		return fmt.Errorf("encode run outputs: %w", err)
	}

	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}

	var startedAtClause, finishedAtClause string
	if run.Status == workflow.RunQueued {
		startedAtClause = ", started_at = CURRENT_TIMESTAMP"
	}
	if to == workflow.RunSucceeded || to == workflow.RunFailed || to == workflow.RunCancelled {
		finishedAtClause = ", finished_at = CURRENT_TIMESTAMP"
	}

	_, err = db.ExecContext(ctx, `
		UPDATE runs SET status = ?, outputs = ?, error = NULLIF(?, '')`+startedAtClause+finishedAtClause+`
		WHERE id = ?
	`, string(to), string(outputsJSON), errText, run.ID)
	if err != nil {
		return fmt.Errorf("update run %s status: %w", run.ID, err)
	}

	run.Status = to
	run.Outputs = outputs
	if runErr != nil {
		run.Error = errText
	}
	return nil
}

// updateStepStatus validates the transition, then persists it.
func updateStepStatus(ctx context.Context, db *sql.DB, runID, stepID string, from, to workflow.StepStatus, input, output map[string]any, stepErr error) error {
	if err := workflow.ValidateStepTransition(from, to); err != nil {
		return err
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode step input: %w", err)
	}
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("encode step output: %w", err)
	}

	errText := ""
	if stepErr != nil {
		errText = stepErr.Error()
	}

	var startedAtClause, finishedAtClause string
	if from == workflow.StepPending {
		startedAtClause = ", started_at = CURRENT_TIMESTAMP"
	}
	if to == workflow.StepSucceeded || to == workflow.StepFailed || to == workflow.StepSkipped || to == workflow.StepCancelled {
		finishedAtClause = ", finished_at = CURRENT_TIMESTAMP"
	}

	_, err = db.ExecContext(ctx, `
		UPDATE run_steps SET status = ?, input = ?, output = ?, error = NULLIF(?, '')`+startedAtClause+finishedAtClause+`
		WHERE run_id = ? AND step_id = ?
	`, string(to), string(inputJSON), string(outputJSON), errText, runID, stepID)
	if err != nil {
		return fmt.Errorf("update step %s/%s status: %w", runID, stepID, err)
	}

	return nil
}

// GetRun returns a run and its steps by id. It returns ErrRunNotFound if
// no such run has been recorded.
func GetRun(ctx context.Context, db *sql.DB, id string) (*Run, []Step, error) {
	var (
		run         Run
		status      string
		inputsJSON  string
		outputsJSON string
		errText     sql.NullString
		createdAt   time.Time
	)

	err := db.QueryRowContext(ctx, `
		SELECT id, workflow_id, workflow_version, status, inputs, outputs, error, created_at
		FROM runs WHERE id = ?
	`, id).Scan(&run.ID, &run.WorkflowID, &run.WorkflowVersion, &status, &inputsJSON, &outputsJSON, &errText, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrRunNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get run %s: %w", id, err)
	}

	run.Status = workflow.RunStatus(status)
	run.Error = errText.String
	run.CreatedAt = createdAt
	if err := json.Unmarshal([]byte(inputsJSON), &run.Inputs); err != nil {
		return nil, nil, fmt.Errorf("decode run inputs: %w", err)
	}
	if err := json.Unmarshal([]byte(outputsJSON), &run.Outputs); err != nil {
		return nil, nil, fmt.Errorf("decode run outputs: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT run_id, step_id, status, input, output, error
		FROM run_steps WHERE run_id = ?
		ORDER BY rowid
	`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("list steps for run %s: %w", id, err)
	}
	defer rows.Close()

	var steps []Step
	for rows.Next() {
		var (
			step        Step
			stepStatus  string
			inputJSON   string
			outputJSON  string
			stepErrText sql.NullString
		)
		if err := rows.Scan(&step.RunID, &step.StepID, &stepStatus, &inputJSON, &outputJSON, &stepErrText); err != nil {
			return nil, nil, fmt.Errorf("scan step: %w", err)
		}
		step.Status = workflow.StepStatus(stepStatus)
		step.Error = stepErrText.String
		if err := json.Unmarshal([]byte(inputJSON), &step.Input); err != nil {
			return nil, nil, fmt.Errorf("decode step input: %w", err)
		}
		if err := json.Unmarshal([]byte(outputJSON), &step.Output); err != nil {
			return nil, nil, fmt.Errorf("decode step output: %w", err)
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("list steps for run %s: %w", id, err)
	}

	return &run, steps, nil
}
