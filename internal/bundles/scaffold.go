package bundles

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

const scaffoldManifestTemplate = `id: %s
version: "%s"
app: app
workflows:
  - workflows/main.yaml
requires_plugins: []
`

// scaffoldViteManifestTemplate points app at app/dist rather than app: a
// Vite project's patchcord-app.yaml only exists there after `npm run
// build` copies it out of app/public — see apps.ScaffoldVite.
const scaffoldViteManifestTemplate = `id: %s
version: "%s"
app: app/dist
workflows:
  - workflows/main.yaml
requires_plugins: []
`

// Scaffold writes a minimal bundle.yaml to dir, plus an embedded app
// (delegating to apps.Scaffold) and workflow (delegating to
// workflow.Scaffold) so the result is `bundle pack`-able as-is —
// requires_plugins starts empty since Scaffold has no way to know what
// plugin the bundle should depend on. It returns an error if dir already
// exists and is not empty.
func Scaffold(dir, id, version string) error {
	if err := ensureEmptyDir(dir); err != nil {
		return err
	}

	manifest := fmt.Sprintf(scaffoldManifestTemplate, id, version)
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(manifest), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ManifestFileName, err)
	}

	if err := apps.Scaffold(filepath.Join(dir, "app"), id, version); err != nil {
		return fmt.Errorf("scaffold embedded app: %w", err)
	}

	workflowsDir := filepath.Join(dir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		return fmt.Errorf("create %q: %w", workflowsDir, err)
	}
	if err := workflow.Scaffold(filepath.Join(workflowsDir, "main.yaml"), id+"_workflow", 1); err != nil {
		return fmt.Errorf("scaffold embedded workflow: %w", err)
	}

	return nil
}

// ScaffoldVite writes a bundle.yaml to dir, plus an embedded Vite +
// TypeScript project (delegating to apps.ScaffoldVite) and workflow
// (delegating to workflow.Scaffold). Unlike Scaffold, the result is not
// `bundle pack`-able as-is: the embedded app's patchcord-app.yaml only
// exists at app/dist after building it —
//
//	cd dir/app && npm install && npm run build
//
// bundle.yaml's app field already points at app/dist, so `bundle
// pack`/`bundle dev [--watch]` need nothing further once that build has
// run at least once. It returns an error if dir already exists and is not
// empty.
func ScaffoldVite(dir, id, version string) error {
	if err := ensureEmptyDir(dir); err != nil {
		return err
	}

	manifest := fmt.Sprintf(scaffoldViteManifestTemplate, id, version)
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(manifest), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ManifestFileName, err)
	}

	if err := apps.ScaffoldVite(filepath.Join(dir, "app"), id, version); err != nil {
		return fmt.Errorf("scaffold embedded app: %w", err)
	}

	workflowsDir := filepath.Join(dir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		return fmt.Errorf("create %q: %w", workflowsDir, err)
	}
	if err := workflow.Scaffold(filepath.Join(workflowsDir, "main.yaml"), id+"_workflow", 1); err != nil {
		return fmt.Errorf("scaffold embedded workflow: %w", err)
	}

	return nil
}

// ensureEmptyDir creates dir if it doesn't exist yet, or errors if it
// exists and already has entries — same rule as apps.Scaffold and
// plugins.Scaffold, kept package-local rather than shared since each is a
// few lines wired into a different set of writes.
func ensureEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err == nil {
		if len(entries) > 0 {
			return fmt.Errorf("%q already exists and is not empty", dir)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("check %q: %w", dir, err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %q: %w", dir, err)
	}
	return nil
}
