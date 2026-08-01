package workflow

import "testing"

func TestValidateRunTransition(t *testing.T) {
	tests := []struct {
		from    RunStatus
		to      RunStatus
		wantErr bool
	}{
		// Valid transitions.
		{RunQueued, RunRunning, false},
		{RunQueued, RunCancelled, false},
		{RunRunning, RunSucceeded, false},
		{RunRunning, RunFailed, false},
		{RunRunning, RunCancelled, false},

		// Invalid: skipping states.
		{RunQueued, RunSucceeded, true},
		{RunQueued, RunFailed, true},

		// Invalid: terminal states never transition further.
		{RunSucceeded, RunRunning, true},
		{RunFailed, RunRunning, true},
		{RunCancelled, RunRunning, true},
		{RunSucceeded, RunFailed, true},

		// Invalid: no transition backwards or to the same state.
		{RunRunning, RunQueued, true},
		{RunRunning, RunRunning, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			err := ValidateRunTransition(tt.from, tt.to)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateRunTransition(%s, %s) = nil, want an error", tt.from, tt.to)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateRunTransition(%s, %s) = %v, want nil", tt.from, tt.to, err)
			}
		})
	}
}

func TestValidateStepTransition(t *testing.T) {
	tests := []struct {
		from    StepStatus
		to      StepStatus
		wantErr bool
	}{
		// Valid transitions.
		{StepPending, StepRunning, false},
		{StepPending, StepSkipped, false},
		{StepPending, StepCancelled, false},
		{StepRunning, StepSucceeded, false},
		{StepRunning, StepFailed, false},
		{StepRunning, StepCancelled, false},

		// Invalid: skipping states.
		{StepPending, StepSucceeded, true},
		{StepPending, StepFailed, true},

		// Invalid: terminal states never transition further.
		{StepSucceeded, StepRunning, true},
		{StepFailed, StepRunning, true},
		{StepSkipped, StepRunning, true},
		{StepCancelled, StepRunning, true},

		// Invalid: no transition backwards or to the same state.
		{StepRunning, StepPending, true},
		{StepRunning, StepRunning, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			err := ValidateStepTransition(tt.from, tt.to)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateStepTransition(%s, %s) = nil, want an error", tt.from, tt.to)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateStepTransition(%s, %s) = %v, want nil", tt.from, tt.to, err)
			}
		})
	}
}
