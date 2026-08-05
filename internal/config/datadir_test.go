package config

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDefaultDataDirFor(t *testing.T) {
	noEnv := func(string) string { return "" }
	home := func() (string, error) { return "/home/alice", nil }
	noHome := func() (string, error) { return "", errors.New("no home directory") }

	t.Run("darwin uses Library/Application Support", func(t *testing.T) {
		got := defaultDataDirFor("darwin", noEnv, home)
		want := filepath.Join("/home/alice", "Library", "Application Support", "patchcord")
		if got != want {
			t.Fatalf("defaultDataDirFor(darwin) = %q, want %q", got, want)
		}
	})

	t.Run("darwin falls back to ./data without a home directory", func(t *testing.T) {
		if got := defaultDataDirFor("darwin", noEnv, noHome); got != fallbackDataDir {
			t.Fatalf("defaultDataDirFor(darwin) = %q, want %q", got, fallbackDataDir)
		}
	})

	t.Run("linux prefers XDG_DATA_HOME", func(t *testing.T) {
		env := func(key string) string {
			if key == "XDG_DATA_HOME" {
				return "/home/alice/.data"
			}
			return ""
		}
		got := defaultDataDirFor("linux", env, home)
		want := filepath.Join("/home/alice/.data", "patchcord")
		if got != want {
			t.Fatalf("defaultDataDirFor(linux) = %q, want %q", got, want)
		}
	})

	t.Run("linux falls back to ~/.local/share without XDG_DATA_HOME", func(t *testing.T) {
		got := defaultDataDirFor("linux", noEnv, home)
		want := filepath.Join("/home/alice", ".local", "share", "patchcord")
		if got != want {
			t.Fatalf("defaultDataDirFor(linux) = %q, want %q", got, want)
		}
	})

	t.Run("linux falls back to ./data without a home directory either", func(t *testing.T) {
		if got := defaultDataDirFor("linux", noEnv, noHome); got != fallbackDataDir {
			t.Fatalf("defaultDataDirFor(linux) = %q, want %q", got, fallbackDataDir)
		}
	})

	t.Run("windows prefers LOCALAPPDATA", func(t *testing.T) {
		env := func(key string) string {
			if key == "LOCALAPPDATA" {
				return `C:\Users\alice\AppData\Local`
			}
			return ""
		}
		got := defaultDataDirFor("windows", env, home)
		want := filepath.Join(`C:\Users\alice\AppData\Local`, "patchcord")
		if got != want {
			t.Fatalf("defaultDataDirFor(windows) = %q, want %q", got, want)
		}
	})

	t.Run("windows falls back to home AppData\\Local without LOCALAPPDATA", func(t *testing.T) {
		got := defaultDataDirFor("windows", noEnv, home)
		want := filepath.Join("/home/alice", "AppData", "Local", "patchcord")
		if got != want {
			t.Fatalf("defaultDataDirFor(windows) = %q, want %q", got, want)
		}
	})

	t.Run("windows falls back to ./data without a home directory either", func(t *testing.T) {
		if got := defaultDataDirFor("windows", noEnv, noHome); got != fallbackDataDir {
			t.Fatalf("defaultDataDirFor(windows) = %q, want %q", got, fallbackDataDir)
		}
	})
}

func TestDefaultDataDir(t *testing.T) {
	// DefaultDataDir just wires the real OS environment into
	// defaultDataDirFor; a smoke test that it never returns an empty
	// string is enough; the branch logic itself is covered above.
	if got := DefaultDataDir(); got == "" {
		t.Fatal("DefaultDataDir() = \"\", want a non-empty path")
	}
}
