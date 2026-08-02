// Command encoding is an example plugin bundling three small, unrelated
// utility actions that don't individually warrant their own supervised
// process — "base64.encode@1", "base64.decode@1", "hash.sha256@1", and
// "uuid.generate@1" — the same "several actions, one process" pattern the
// "text" example plugin already establishes for string operations.
//
// It depends only on the SDK (sdk/go-plugin) and the standard library, plus
// github.com/google/uuid (already a direct dependency of the module), never
// on any internal/ package of the agent.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/google/uuid"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

type base64EncodeAction struct{}

func (base64EncodeAction) ID() string { return "base64.encode@1" }

func (base64EncodeAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	return patchcord.ActionOutput{"value": base64.StdEncoding.EncodeToString([]byte(value))}, nil
}

type base64DecodeAction struct{}

func (base64DecodeAction) ID() string { return "base64.decode@1" }

func (base64DecodeAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	return patchcord.ActionOutput{"value": string(decoded)}, nil
}

type sha256Action struct{}

func (sha256Action) ID() string { return "hash.sha256@1" }

func (sha256Action) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	sum := sha256.Sum256([]byte(value))
	return patchcord.ActionOutput{"value": hex.EncodeToString(sum[:])}, nil
}

type uuidGenerateAction struct{}

func (uuidGenerateAction) ID() string { return "uuid.generate@1" }

func (uuidGenerateAction) Run(_ context.Context, _ patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	return patchcord.ActionOutput{"value": uuid.NewString()}, nil
}

func main() {
	plugin := patchcord.Plugin{
		Manifest: patchcord.Manifest{
			ID:      "io.patchcord.example-encoding",
			Version: "1.0.0",
		},
		Actions: []patchcord.Action{
			base64EncodeAction{},
			base64DecodeAction{},
			sha256Action{},
			uuidGenerateAction{},
		},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
