// Command time is an example plugin contributing basic date/time actions —
// "time.now@1", "time.format@1", "time.parse@1", "time.add@1", and
// "time.sleep@1" — the kind of utility operations most real workflows need
// (timestamps, expiries, scheduling windows, pacing). Every action that
// produces a moment in time represents it as RFC3339 in UTC, so the actions
// compose directly: "time.now@1"'s output feeds "time.add@1", whose output
// feeds "time.format@1", without any conversion step in between.
//
// It depends only on the SDK (sdk/go-plugin), never on any internal/
// package of the agent.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

type nowAction struct{}

func (nowAction) ID() string { return "time.now@1" }

func (nowAction) Run(_ context.Context, _ patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	now := time.Now().UTC()
	return patchcord.ActionOutput{
		"value": now.Format(time.RFC3339),
		"unix":  float64(now.Unix()),
	}, nil
}

type formatAction struct{}

func (formatAction) ID() string { return "time.format@1" }

func (formatAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	layout, ok := input["layout"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "layout")
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("input %q must be RFC3339: %w", "value", err)
	}
	return patchcord.ActionOutput{"value": parsed.Format(layout)}, nil
}

type parseAction struct{}

func (parseAction) ID() string { return "time.parse@1" }

func (parseAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	layout, ok := input["layout"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "layout")
	}

	parsed, err := time.Parse(layout, value)
	if err != nil {
		return nil, fmt.Errorf("parse time: %w", err)
	}
	utc := parsed.UTC()
	return patchcord.ActionOutput{
		"value": utc.Format(time.RFC3339),
		"unix":  float64(utc.Unix()),
	}, nil
}

type addAction struct{}

func (addAction) ID() string { return "time.add@1" }

func (addAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	durationStr, ok := input["duration"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "duration")
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("input %q must be RFC3339: %w", "value", err)
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("input %q: %w", "duration", err)
	}

	return patchcord.ActionOutput{"value": parsed.Add(duration).UTC().Format(time.RFC3339)}, nil
}

type sleepAction struct{}

func (sleepAction) ID() string { return "time.sleep@1" }

// Run pauses the step for the given duration, or returns early with ctx's
// error if the run is cancelled or its timeout elapses first (ADR-0018) —
// a sleep must never outlast the run that started it.
func (sleepAction) Run(ctx context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	durationStr, ok := input["duration"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "duration")
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("input %q: %w", "duration", err)
	}
	if duration < 0 {
		return nil, fmt.Errorf("input %q must not be negative: %q", "duration", durationStr)
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return patchcord.ActionOutput{"slept_for": durationStr}, nil
	}
}

func main() {
	plugin := patchcord.Plugin{
		Manifest: patchcord.Manifest{
			ID:      "io.patchcord.example-time",
			Version: "1.0.0",
		},
		Actions: []patchcord.Action{
			nowAction{},
			formatAction{},
			parseAction{},
			addAction{},
			sleepAction{},
		},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
