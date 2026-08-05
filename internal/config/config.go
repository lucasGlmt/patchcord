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
	"strconv"

	"gopkg.in/yaml.v3"
)

const (
	envListen                      = "PATCHCORD_LISTEN"
	envDataDir                     = "PATCHCORD_DATA_DIR"
	envSecretsMasterKeyFile        = "PATCHCORD_SECRETS_MASTER_KEY_FILE"
	envAppsDirectoryListingEnabled = "PATCHCORD_APPS_DIRECTORY_LISTING_ENABLED"
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
	// Apps holds settings for the /apps/{id}/ hosting surface (ADR-0026).
	Apps AppsConfig `yaml:"apps"`
}

// AppsConfig holds settings for installed application hosting.
type AppsConfig struct {
	DirectoryListing DirectoryListingConfig `yaml:"directory_listing"`
}

// DirectoryListingConfig controls GET /apps/, an Apache-style index page
// listing every installed application with a link to its /apps/{id}/ — see
// ADR-0061.
type DirectoryListingConfig struct {
	// Enabled turns the listing on. Left false (the default), GET /apps/
	// returns a plain 404, unchanged from before this setting existed —
	// the operator opts in explicitly (ADR-0007: no behavior change without
	// one).
	Enabled bool `yaml:"enabled"`
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

// FromEnv reads PATCHCORD_LISTEN, PATCHCORD_DATA_DIR,
// PATCHCORD_SECRETS_MASTER_KEY_FILE and
// PATCHCORD_APPS_DIRECTORY_LISTING_ENABLED. An unset variable leaves the
// corresponding field empty (or false), so Merge falls through to a
// lower-precedence source for it. An unparseable
// PATCHCORD_APPS_DIRECTORY_LISTING_ENABLED value (anything strconv.ParseBool
// rejects) is treated the same as unset, rather than failing startup — this
// setting only ever turns a 404 into an index page, never something worth
// refusing to boot over.
func FromEnv() Config {
	directoryListingEnabled, _ := strconv.ParseBool(os.Getenv(envAppsDirectoryListingEnabled))
	return Config{
		Listen:               os.Getenv(envListen),
		DataDir:              os.Getenv(envDataDir),
		SecretsMasterKeyFile: os.Getenv(envSecretsMasterKeyFile),
		Apps:                 AppsConfig{DirectoryListing: DirectoryListingConfig{Enabled: directoryListingEnabled}},
	}
}

// Merge layers override on top of base, field by field: a non-empty field
// in override replaces base's; an empty one leaves base untouched. Callers
// apply this once per source, from lowest to highest precedence — file,
// then env, then flags (internal/cli/serve.go).
//
// Apps.DirectoryListing.Enabled is boolean, so it can't distinguish "this
// source said false" from "this source expressed no opinion" the way an
// empty string can — Merge treats override's true as an opinion and leaves
// base alone otherwise. That is a real limitation (a higher-precedence
// source can enable but never force-disable what a lower one enabled), but
// nothing in this setting's three sources needs to force-disable it today:
// there is no CLI flag for it, so the highest-precedence source is env, and
// an operator who wants it off can simply not set the env var or the file
// key and rely on the false default.
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
	if override.Apps.DirectoryListing.Enabled {
		base.Apps.DirectoryListing.Enabled = true
	}
	return base
}
