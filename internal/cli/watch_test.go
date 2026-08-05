package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// withShortWatchDebounce shrinks watchDebounce for the duration of one test
// so it doesn't have to sleep for the production 300ms on every assertion.
func withShortWatchDebounce(t *testing.T, d time.Duration) {
	t.Helper()
	orig := watchDebounce
	watchDebounce = d
	t.Cleanup(func() { watchDebounce = orig })
}

// startWatch runs watchDir in a background goroutine and returns a channel
// that receives its final error once ctx is cancelled (or a setup failure
// happens beforehand).
func startWatch(ctx context.Context, dir string, onChange func() error, reportErr func(error)) <-chan error {
	done := make(chan error, 1)
	go func() { done <- watchDir(ctx, dir, onChange, reportErr) }()
	return done
}

func TestWatchDir_CallsOnChangeAfterFileWrite(t *testing.T) {
	withShortWatchDebounce(t, 20*time.Millisecond)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(filePath, []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	changed := make(chan struct{}, 1)
	onChange := func() error {
		select {
		case changed <- struct{}{}:
		default:
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := startWatch(ctx, dir, onChange, func(error) {})

	// Let the watcher register dir before triggering the change it should
	// react to.
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filePath, []byte("v2"), 0o644); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}

	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("onChange was not called after a file write")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("watchDir() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watchDir did not return after context cancellation")
	}
}

// TestWatchDir_ContinuesAfterOnChangeError guards `bundle dev --watch`'s
// core promise: a failing reinstall (e.g. a workflow version that was not
// bumped, ADR-0008) must be reported, not stop the watch — the developer
// fixes it and saves again.
func TestWatchDir_ContinuesAfterOnChangeError(t *testing.T) {
	withShortWatchDebounce(t, 20*time.Millisecond)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(filePath, []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var calls int32
	changed := make(chan struct{}, 2)
	onChange := func() error {
		n := atomic.AddInt32(&calls, 1)
		changed <- struct{}{}
		if n == 1 {
			return errors.New("boom")
		}
		return nil
	}

	var reportedErrs int32
	reportErr := func(error) { atomic.AddInt32(&reportedErrs, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startWatch(ctx, dir, onChange, reportErr)

	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filePath, []byte("v2"), 0o644); err != nil {
		t.Fatalf("rewrite file (1): %v", err)
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("onChange was not called after the first write")
	}

	time.Sleep(50 * time.Millisecond) // let watchDir report the error and resume watching
	if err := os.WriteFile(filePath, []byte("v3"), 0o644); err != nil {
		t.Fatalf("rewrite file (2): %v", err)
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("onChange was not called again after the failing call — the watch must survive an install error")
	}

	if n := atomic.LoadInt32(&reportedErrs); n != 1 {
		t.Fatalf("reportErr called %d times, want 1", n)
	}
}

// TestWatchDir_DebouncesBurstIntoOneCall guards the debounce: a burst of
// writes close together (what a build tool like `vite build` produces for
// one logical change) must collapse into a single onChange call, not one
// per file.
func TestWatchDir_DebouncesBurstIntoOneCall(t *testing.T) {
	withShortWatchDebounce(t, 150*time.Millisecond)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(filePath, []byte("v0"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startWatch(ctx, dir, func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	}, func(error) {})

	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filePath, []byte(fmt.Sprintf("v%d", i)), 0o644); err != nil {
			t.Fatalf("rewrite file: %v", err)
		}
		time.Sleep(20 * time.Millisecond) // well under watchDebounce: keeps resetting it
	}

	time.Sleep(400 * time.Millisecond) // let the debounce settle and onChange fire once

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("onChange called %d times, want exactly 1 (debounce should coalesce the burst)", n)
	}
}

// TestWatchDir_IgnoresHiddenDirectories guards the deliberate exclusion of
// hidden directories (".git" and friends) from the watch: they churn
// independently of the source being developed and would otherwise trigger
// spurious reinstalls.
func TestWatchDir_IgnoresHiddenDirectories(t *testing.T) {
	withShortWatchDebounce(t, 20*time.Millisecond)

	dir := t.TempDir()
	hiddenDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startWatch(ctx, dir, func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	}, func(error) {})

	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(hiddenDir, "HEAD"), []byte("ref"), 0o644); err != nil {
		t.Fatalf("write into hidden dir: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("onChange called %d times for a change inside a hidden directory, want 0", n)
	}
}

// TestWatchDir_IgnoresNodeModules guards the deliberate exclusion of
// node_modules from the watch: a Vite-based embedded app's node_modules
// sits right under the directory `bundle dev --watch` watches, and
// registering an fsnotify watcher on every one of its subdirectories would
// be slow at start-up at best and exhaust the platform's watch limit at
// worst.
func TestWatchDir_IgnoresNodeModules(t *testing.T) {
	withShortWatchDebounce(t, 20*time.Millisecond)

	dir := t.TempDir()
	nodeModules := filepath.Join(dir, "node_modules", "some-package")
	if err := os.MkdirAll(nodeModules, 0o755); err != nil {
		t.Fatalf("mkdir node_modules/some-package: %v", err)
	}

	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startWatch(ctx, dir, func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	}, func(error) {})

	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(nodeModules, "index.js"), []byte("module.exports = {};"), 0o644); err != nil {
		t.Fatalf("write into node_modules: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("onChange called %d times for a change inside node_modules, want 0", n)
	}
}

// TestWatchDir_PicksUpNewlyCreatedSubdirectory guards the dynamic side of
// the recursive watch: a subdirectory created after watchDir has started
// (e.g. a build tool's first `dist/` output) must still be watched, not
// just the tree that existed at startup.
func TestWatchDir_PicksUpNewlyCreatedSubdirectory(t *testing.T) {
	withShortWatchDebounce(t, 20*time.Millisecond)

	dir := t.TempDir()

	changed := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startWatch(ctx, dir, func() error {
		select {
		case changed <- struct{}{}:
		default:
		}
		return nil
	}, func(error) {})

	time.Sleep(100 * time.Millisecond)

	subDir := filepath.Join(dir, "dist")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	// The mkdir itself is a change under dir, so drain it before checking
	// that a write inside the new subdirectory is also picked up.
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("onChange was not called for the new subdirectory's creation")
	}

	time.Sleep(100 * time.Millisecond) // let watchDir finish registering subDir
	if err := os.WriteFile(filepath.Join(subDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write into new subdirectory: %v", err)
	}

	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("onChange was not called for a write inside the newly created subdirectory")
	}
}
