package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// appDirName is the subdirectory patchcord's data lives under, inside
// whichever per-user system directory DefaultDataDir resolves for the
// current OS.
const appDirName = "patchcord"

// fallbackDataDir is what DefaultDataDir returns when it cannot resolve a
// per-user system directory at all (e.g. no home directory available, such
// as in a minimal container). --data-dir and PATCHCORD_DATA_DIR remain a
// full override either way; a command still has to run somewhere.
const fallbackDataDir = "./data"

// DefaultDataDir resolves the built-in default for --data-dir /
// PATCHCORD_DATA_DIR (ADR-0038, ADR-0049): the lowest-precedence source,
// used only when neither a flag nor the environment variable is set.
// Unlike the "./data" it replaces, this is a per-user, per-machine
// location that does not depend on the directory a command happens to be
// run from — every patchcord command run by the same user resolves the
// same agent database by default (ADR-0052).
//
// It follows each OS's own convention for per-user application data:
//   - macOS: ~/Library/Application Support/patchcord
//   - Linux/BSD: $XDG_DATA_HOME/patchcord, or ~/.local/share/patchcord if unset
//   - Windows: %LOCALAPPDATA%\patchcord
func DefaultDataDir() string {
	return defaultDataDirFor(runtime.GOOS, os.Getenv, os.UserHomeDir)
}

// defaultDataDirFor is DefaultDataDir's testable core: goos, getenv and
// homeDir are injected so every OS branch can be exercised from a single
// test binary instead of only the one it happens to run on.
func defaultDataDirFor(goos string, getenv func(string) string, homeDir func() (string, error)) string {
	switch goos {
	case "darwin":
		home, err := homeDir()
		if err != nil || home == "" {
			return fallbackDataDir
		}
		return filepath.Join(home, "Library", "Application Support", appDirName)

	case "windows":
		if dir := getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, appDirName)
		}
		home, err := homeDir()
		if err != nil || home == "" {
			return fallbackDataDir
		}
		return filepath.Join(home, "AppData", "Local", appDirName)

	default: // Linux and other Unix-likes: XDG Base Directory spec.
		if dir := getenv("XDG_DATA_HOME"); dir != "" {
			return filepath.Join(dir, appDirName)
		}
		home, err := homeDir()
		if err != nil || home == "" {
			return fallbackDataDir
		}
		return filepath.Join(home, ".local", "share", appDirName)
	}
}
