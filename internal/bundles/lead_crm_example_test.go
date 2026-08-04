package bundles

import (
	"context"
	"testing"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
)

// TestInstallPackage_LeadCRMExampleBundle installs the repo's reference
// "first complete business application" (bundles/examples/lead-crm) for
// real, proving its manifest parses, both plugin dependencies resolve at
// their exact declared version, its embedded app installs, and both
// embedded workflows validate against the http/postgresql plugins' real
// actions. It does not need a live PostgreSQL server: installation only
// validates shape and known actions, it never executes a workflow — see
// bundle.yaml's header comment for how to exercise this bundle for real.
func TestInstallPackage_LeadCRMExampleBundle(t *testing.T) {
	db := openTestDB(t)
	dataDir := t.TempDir()

	if _, err := plugins.Install(context.Background(), db, httpPluginPath); err != nil {
		t.Fatalf("install http plugin: %v", err)
	}
	if _, err := plugins.Install(context.Background(), db, postgresqlPluginPath); err != nil {
		t.Fatalf("install postgresql plugin: %v", err)
	}
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	packagePath := packBundle(t, "../../bundles/examples/lead-crm", nil)

	b, _, err := InstallPackage(context.Background(), db, dataDir, packagePath, knownActions, false)
	if err != nil {
		t.Fatalf("InstallPackage() error = %v", err)
	}
	if b.ID != "io.patchcord.example-lead-crm" || b.Version != "0.1.0" {
		t.Fatalf("bundle = %+v, want id=io.patchcord.example-lead-crm version=0.1.0", b)
	}

	if _, err := apps.Get(context.Background(), db, "lead-crm"); err != nil {
		t.Fatalf("embedded app was not installed: %v", err)
	}

	for _, id := range []string{"lead_enrichment", "list_leads"} {
		def, err := runs.LatestWorkflow(context.Background(), db, id)
		if err != nil {
			t.Fatalf("workflow %q was not installed: %v", id, err)
		}
		if def.Version != 1 {
			t.Fatalf("workflow %q version = %d, want 1", id, def.Version)
		}
	}
}
