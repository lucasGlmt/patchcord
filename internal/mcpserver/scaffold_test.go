package mcpserver

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

func TestScaffoldApp(t *testing.T) {
	t.Run("static template is the default", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "my-app")
		_, out, err := scaffoldApp(context.Background(), nil, scaffoldIn{Dir: dir, ID: "io.example.my-app", Version: "0.1.0"})
		if err != nil {
			t.Fatalf("scaffoldApp() error = %v", err)
		}
		if out.Template != "static" {
			t.Fatalf("Template = %q, want %q", out.Template, "static")
		}
		if !slices.Contains(out.Files, "patchcord-app.yaml") {
			t.Fatalf("Files = %v, want it to contain patchcord-app.yaml", out.Files)
		}
	})

	t.Run("vite template", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "my-app")
		_, out, err := scaffoldApp(context.Background(), nil, scaffoldIn{Dir: dir, ID: "io.example.my-app", Version: "0.1.0", Template: "vite"})
		if err != nil {
			t.Fatalf("scaffoldApp() error = %v", err)
		}
		if out.Template != "vite" {
			t.Fatalf("Template = %q, want %q", out.Template, "vite")
		}
		if !slices.Contains(out.Files, "package.json") {
			t.Fatalf("Files = %v, want it to contain package.json", out.Files)
		}
	})

	t.Run("unknown template is rejected", func(t *testing.T) {
		dir := t.TempDir()
		_, _, err := scaffoldApp(context.Background(), nil, scaffoldIn{Dir: dir, ID: "io.example.my-app", Version: "0.1.0", Template: "react-native"})
		if err == nil {
			t.Fatal("expected an error for an unknown template, got nil")
		}
	})

	t.Run("refuses to write into a non-empty directory", func(t *testing.T) {
		dir := t.TempDir()
		if _, _, err := scaffoldApp(context.Background(), nil, scaffoldIn{Dir: dir, ID: "io.example.a", Version: "0.1.0"}); err != nil {
			t.Fatalf("first scaffoldApp() error = %v", err)
		}
		if _, _, err := scaffoldApp(context.Background(), nil, scaffoldIn{Dir: dir, ID: "io.example.b", Version: "0.1.0"}); err == nil {
			t.Fatal("expected an error scaffolding into an already-populated directory, got nil")
		}
	})
}

func TestScaffoldBundle(t *testing.T) {
	t.Run("static template is the default", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "my-bundle")
		_, out, err := scaffoldBundle(context.Background(), nil, scaffoldIn{Dir: dir, ID: "io.example.my-bundle", Version: "0.1.0"})
		if err != nil {
			t.Fatalf("scaffoldBundle() error = %v", err)
		}
		if out.Template != "static" {
			t.Fatalf("Template = %q, want %q", out.Template, "static")
		}
		if !slices.Contains(out.Files, "bundle.yaml") {
			t.Fatalf("Files = %v, want it to contain bundle.yaml", out.Files)
		}
		if !slices.Contains(out.Files, "app/patchcord-app.yaml") {
			t.Fatalf("Files = %v, want it to contain the embedded app's manifest", out.Files)
		}
	})

	t.Run("vite template", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "my-bundle")
		_, out, err := scaffoldBundle(context.Background(), nil, scaffoldIn{Dir: dir, ID: "io.example.my-bundle", Version: "0.1.0", Template: "vite"})
		if err != nil {
			t.Fatalf("scaffoldBundle() error = %v", err)
		}
		if !slices.Contains(out.Files, "app/package.json") {
			t.Fatalf("Files = %v, want it to contain the embedded app's package.json", out.Files)
		}
	})
}
