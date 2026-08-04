package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

func TestLoad(t *testing.T) {
	t.Run("parses listen and data_dir", func(t *testing.T) {
		path := writeConfigFile(t, "listen: 0.0.0.0:7331\ndata_dir: /data\n")

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Listen != "0.0.0.0:7331" || cfg.DataDir != "/data" {
			t.Fatalf("Load() = %+v, want Listen=0.0.0.0:7331 DataDir=/data", cfg)
		}
	})

	t.Run("a missing file is an error", func(t *testing.T) {
		if _, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
			t.Fatal("expected an error for a missing config file, got nil")
		}
	})

	t.Run("an unknown top-level key is rejected", func(t *testing.T) {
		path := writeConfigFile(t, "liste: 0.0.0.0:7331\n")

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected an error for an unknown key, got nil")
		}
		if !strings.Contains(err.Error(), "parse config file") {
			t.Fatalf("error = %q, want it to mention config file parsing", err.Error())
		}
	})

	t.Run("a partial file leaves the other field empty", func(t *testing.T) {
		path := writeConfigFile(t, "data_dir: /data\n")

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Listen != "" {
			t.Fatalf("Listen = %q, want empty", cfg.Listen)
		}
		if cfg.DataDir != "/data" {
			t.Fatalf("DataDir = %q, want /data", cfg.DataDir)
		}
	})
}

func TestFromEnv(t *testing.T) {
	t.Run("reads set variables", func(t *testing.T) {
		t.Setenv(envListen, "0.0.0.0:7331")
		t.Setenv(envDataDir, "/data")

		cfg := FromEnv()
		if cfg.Listen != "0.0.0.0:7331" || cfg.DataDir != "/data" {
			t.Fatalf("FromEnv() = %+v, want Listen=0.0.0.0:7331 DataDir=/data", cfg)
		}
	})

	t.Run("leaves fields empty when unset", func(t *testing.T) {
		cfg := FromEnv()
		if cfg.Listen != "" || cfg.DataDir != "" {
			t.Fatalf("FromEnv() = %+v, want both fields empty", cfg)
		}
	})
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name     string
		base     Config
		override Config
		want     Config
	}{
		{
			name:     "override's non-empty fields replace base's",
			base:     Config{Listen: "127.0.0.1:7331", DataDir: "./data"},
			override: Config{Listen: "0.0.0.0:7331", DataDir: "/data"},
			want:     Config{Listen: "0.0.0.0:7331", DataDir: "/data"},
		},
		{
			name:     "an empty override field leaves base's value",
			base:     Config{Listen: "127.0.0.1:7331", DataDir: "./data"},
			override: Config{Listen: "0.0.0.0:7331"},
			want:     Config{Listen: "0.0.0.0:7331", DataDir: "./data"},
		},
		{
			name:     "a fully empty override changes nothing",
			base:     Config{Listen: "127.0.0.1:7331", DataDir: "./data"},
			override: Config{},
			want:     Config{Listen: "127.0.0.1:7331", DataDir: "./data"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Merge(tt.base, tt.override)
			if got != tt.want {
				t.Fatalf("Merge() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
