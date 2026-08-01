package workflow

import (
	"fmt"
	"slices"
)

// RunStatus is one state in a Run's explicit state machine (vision
// document, section 12.2).
type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

// runTransitions lists, for each Run status, the statuses it may move to.
// Succeeded, failed and cancelled are terminal: nothing may follow them.
var runTransitions = map[RunStatus][]RunStatus{
	RunQueued:  {RunRunning, RunCancelled},
	RunRunning: {RunSucceeded, RunFailed, RunCancelled},
}

// ValidateRunTransition returns an error unless a Run may move from from to
// to.
func ValidateRunTransition(from, to RunStatus) error {
	if slices.Contains(runTransitions[from], to) {
		return nil
	}
	return fmt.Errorf("invalid run transition: %s -> %s", from, to)
}

// StepStatus is one state in a Step's explicit state machine (vision
// document, section 12.2).
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepSucceeded StepStatus = "succeeded"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
	StepCancelled StepStatus = "cancelled"
)

// stepTransitions lists, for each Step status, the statuses it may move
// to. Succeeded, failed, skipped and cancelled are terminal.
var stepTransitions = map[StepStatus][]StepStatus{
	StepPending: {StepRunning, StepSkipped, StepCancelled},
	StepRunning: {StepSucceeded, StepFailed, StepCancelled},
}

// ValidateStepTransition returns an error unless a Step may move from from
// to to.
func ValidateStepTransition(from, to StepStatus) error {
	if slices.Contains(stepTransitions[from], to) {
		return nil
	}
	return fmt.Errorf("invalid step transition: %s -> %s", from, to)
}
