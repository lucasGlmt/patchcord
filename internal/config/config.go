// Package config loads the settings `patchcord serve` runs with from three
// layered sources — a YAML file, environment variables, and CLI flags — in
// that increasing order of precedence: a flag explicitly passed always
// wins, then an environment variable, then the config file, then a
// built-in default (ADR-0038). It only holds the settings themselves; the
// CLI (internal/cli/serve.go) owns the built-in defaults and the flag
// layer, since only it knows which flags were actually passed
// (cobra's Flags().Changed).
package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	envListen               = "PATCHCORD_LISTEN"
	envDataDir              = "PATCHCORD_DATA_DIR"
	envSecretsMasterKeyFile = "PATCHCORD_SECRETS_MASTER_KEY_FILE"
)

// Config holds the subset of runtime.Config that can come from a file or
// environment variables, not just CLI flags. An empty field means that
// source expressed no opinion — see Merge.
type Config struct {
	Listen  string `yaml:"listen"`
	DataDir string `yaml:"data_dir"`
	// SecretsMasterKeyFile points to the file holding the base64 AES-256
	// master key for the "file" secret store (secrets.FileStore). Left
	// empty, the "file" secret reference type is simply not available on
	// this agent — see ADR-0040.
	SecretsMasterKeyFile string `yaml:"secrets_master_key_file"`
}

// Load reads and parses a YAML config file. Unknown top-level keys are
// rejected (a typo'd "liste:" is caught here, not silently ignored) — the
// same discipline internal/workflow.Validate already applies to workflow
// YAML.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}

	return cfg, nil
}

// FromEnv reads PATCHCORD_LISTEN, PATCHCORD_DATA_DIR and
// PATCHCORD_SECRETS_MASTER_KEY_FILE. An unset variable leaves the
// corresponding field empty, so Merge falls through to a lower-precedence
// source for it.
func FromEnv() Config {
	return Config{
		Listen:               os.Getenv(envListen),
		DataDir:              os.Getenv(envDataDir),
		SecretsMasterKeyFile: os.Getenv(envSecretsMasterKeyFile),
	}
}

// Merge layers override on top of base, field by field: a non-empty field
// in override replaces base's; an empty one leaves base untouched. Callers
// apply this once per source, from lowest to highest precedence — file,
// then env, then flags (internal/cli/serve.go).
func Merge(base, override Config) Config {
	if override.Listen != "" {
		base.Listen = override.Listen
	}
	if override.DataDir != "" {
		base.DataDir = override.DataDir
	}
	if override.SecretsMasterKeyFile != "" {
		base.SecretsMasterKeyFile = override.SecretsMasterKeyFile
	}
	return base
}
