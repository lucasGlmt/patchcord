package cli

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasglmt/patchcord/internal/bundles"
)

func TestNewRootCommand_HasDevSubcommand(t *testing.T) {
	root := NewRootCommand()

	dev, _, err := root.Find([]string{"dev"})
	if err != nil {
		t.Fatalf("Find(dev) error = %v", err)
	}
	if dev.Name() != "dev" {
		t.Fatalf("found command %q, want %q", dev.Name(), "dev")
	}

	for _, name := range []string{"listen", "data-dir", "config", "secrets-master-key-file", "app-dev-cmd", "no-app-dev"} {
		if dev.Flags().Lookup(name) == nil {
			t.Fatalf("dev command has no --%s flag", name)
		}
	}

	if got := dev.Flags().Lookup("listen").DefValue; got != defaultListenAddr {
		t.Fatalf("--listen default = %q, want %q", got, defaultListenAddr)
	}
	if got := dev.Flags().Lookup("app-dev-cmd").DefValue; got != "npm run dev" {
		t.Fatalf("--app-dev-cmd default = %q, want %q", got, "npm run dev")
	}
}

func TestAddressAlreadyInUse(t *testing.T) {
	t.Run("true when the address is already bound", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.Listen() error = %v", err)
		}
		defer l.Close()

		_, err = net.Listen("tcp", l.Addr().String())
		if err == nil {
			t.Fatal("expected a bind conflict, got nil error")
		}
		if !addressAlreadyInUse(err) {
			t.Fatalf("addressAlreadyInUse(%v) = false, want true", err)
		}
	})

	t.Run("false for an unrelated error", func(t *testing.T) {
		if addressAlreadyInUse(errors.New("some other failure")) {
			t.Fatal("addressAlreadyInUse() = true, want false")
		}
	})
}

func TestFindAppDevDir(t *testing.T) {
	t.Run("finds the Vite template's package.json even though bundle.yaml points at app/dist", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "bundle")
		if err := bundles.ScaffoldVite(dir, "io.patchcord.dev-fixture", "0.1.0"); err != nil {
			t.Fatalf("ScaffoldVite() error = %v", err)
		}

		appDir, ok, err := findAppDevDir(dir)
		if err != nil {
			t.Fatalf("findAppDevDir() error = %v", err)
		}
		if !ok {
			t.Fatal("findAppDevDir() ok = false, want true")
		}
		if want := filepath.Join(dir, "app"); appDir != want {
			t.Fatalf("findAppDevDir() dir = %q, want %q", appDir, want)
		}
	})

	t.Run("finds nothing to run for the static template", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "bundle")
		if err := bundles.Scaffold(dir, "io.patchcord.dev-fixture", "0.1.0"); err != nil {
			t.Fatalf("Scaffold() error = %v", err)
		}

		_, ok, err := findAppDevDir(dir)
		if err != nil {
			t.Fatalf("findAppDevDir() error = %v", err)
		}
		if ok {
			t.Fatal("findAppDevDir() ok = true, want false (static template has no package.json)")
		}
	})

	t.Run("finds nothing for a bundle with no embedded app", func(t *testing.T) {
		dir := t.TempDir()
		manifest := "id: io.patchcord.dev-fixture\nversion: \"0.1.0\"\nworkflows: []\n"
		if err := os.WriteFile(filepath.Join(dir, bundles.ManifestFileName), []byte(manifest), 0o644); err != nil {
			t.Fatalf("write bundle.yaml: %v", err)
		}

		_, ok, err := findAppDevDir(dir)
		if err != nil {
			t.Fatalf("findAppDevDir() error = %v", err)
		}
		if ok {
			t.Fatal("findAppDevDir() ok = true, want false (no app field at all)")
		}
	})
}

func TestLinePrefixWriter(t *testing.T) {
	var out bytes.Buffer
	w := &linePrefixWriter{prefix: "[app] ", out: &out}

	if _, err := w.Write([]byte("hello ")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("out = %q before a newline was written, want empty", out.String())
	}

	if _, err := w.Write([]byte("world\nsecond line\nthird ")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := w.Write([]byte("line\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	want := "[app] hello world\n[app] second line\n[app] third line\n"
	if out.String() != want {
		t.Fatalf("out = %q, want %q", out.String(), want)
	}
}

func TestRunAppDev(t *testing.T) {
	t.Run("stops gracefully on context cancellation instead of waiting out the command", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error, 1)
		start := time.Now()
		go func() {
			done <- runAppDev(ctx, t.TempDir(), []string{"sleep", "30"}, &bytes.Buffer{})
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("runAppDev() error = %v, want nil on a graceful shutdown", err)
			}
			if elapsed := time.Since(start); elapsed > appDevWaitDelay+2*time.Second {
				t.Fatalf("runAppDev() took %s to return after cancellation, want well under %s", elapsed, appDevWaitDelay)
			}
		case <-time.After(appDevWaitDelay + 2*time.Second):
			t.Fatal("runAppDev() did not return after context cancellation")
		}
	})

	t.Run("returns an error for a command that fails on its own", func(t *testing.T) {
		err := runAppDev(context.Background(), t.TempDir(), []string{"false"}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("prefixes the command's combined output", func(t *testing.T) {
		var out bytes.Buffer
		if err := runAppDev(context.Background(), t.TempDir(), []string{"echo", "hello"}, &out); err != nil {
			t.Fatalf("runAppDev() error = %v", err)
		}
		if !strings.Contains(out.String(), "[app] hello") {
			t.Fatalf("out = %q, want it to contain %q", out.String(), "[app] hello")
		}
	})
}
