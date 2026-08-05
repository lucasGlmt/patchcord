package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchDebounce is how long watchDir waits after the last filesystem event
// before calling onChange — long enough to coalesce the burst of writes a
// build tool (e.g. `vite build`) produces for one logical change into a
// single reinstall, instead of one per file. A var, not a const, so tests
// can shrink it instead of sleeping for 300ms per assertion.
var watchDebounce = 300 * time.Millisecond

// watchDir watches every directory under root — added recursively at
// start, and again for any directory created afterwards — and calls
// onChange once, debounced, per burst of filesystem activity, until ctx is
// cancelled. Directories matched by skipWatchDir (hidden directories,
// node_modules) are never watched.
//
// onChange's own errors are reported through reportErr rather than
// stopping the watch: `bundle dev --watch` must survive a bad save (e.g. a
// workflow version that was not bumped, ADR-0008) and keep watching for
// the next one. watchDir itself only returns an error for a setup failure
// (the watcher could not be created, or root could not be walked); a nil
// return means ctx was cancelled.
func watchDir(ctx context.Context, root string, onChange func() error, reportErr func(error)) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	if err := addRecursive(watcher, root); err != nil {
		return fmt.Errorf("watch %q: %w", root, err)
	}

	var debounce *time.Timer
	pending := make(chan struct{}, 1)

	for {
		select {
		case <-ctx.Done():
			if debounce != nil {
				debounce.Stop()
			}
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Create) && !skipWatchDir(filepath.Base(event.Name)) {
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					_ = addRecursive(watcher, event.Name)
				}
			}
			if debounce == nil {
				debounce = time.AfterFunc(watchDebounce, func() {
					select {
					case pending <- struct{}{}:
					default:
					}
				})
			} else {
				debounce.Reset(watchDebounce)
			}

		case <-pending:
			debounce = nil
			if err := onChange(); err != nil {
				reportErr(err)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			reportErr(fmt.Errorf("watch: %w", err))
		}
	}
}

// addRecursive registers root and every directory under it with watcher,
// skipping directories matched by skipWatchDir (see watchDir's doc
// comment).
func addRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && skipWatchDir(d.Name()) {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}

// skipWatchDir reports whether a directory name must never be watched.
// Hidden directories (e.g. ".git") churn independently of the source being
// developed and would otherwise trigger spurious reinstalls. node_modules
// is excluded for a sharper reason: registering an fsnotify watcher on
// every one of its subdirectories is slow at start-up at best and exhausts
// the platform's watch limit at worst (inotify on Linux) — a near-certainty
// for a Vite-based embedded app, since node_modules sits right under the
// directory `bundle dev --watch` watches.
func skipWatchDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}
