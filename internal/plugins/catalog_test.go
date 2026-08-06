package plugins

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/migrations"
)

// actionDescriptors and connectorDescriptors build minimal descriptor
// slices from bare ids, for tests that only care about identity, not about
// the description/schema fields ADR-0062 added.
func actionDescriptors(ids ...string) []ActionDescriptor {
	descs := make([]ActionDescriptor, len(ids))
	for i, id := range ids {
		descs[i] = ActionDescriptor{ID: id}
	}
	return descs
}

func connectorDescriptors(types ...string) []ConnectorDescriptor {
	descs := make([]ConnectorDescriptor, len(types))
	for i, typ := range types {
		descs[i] = ConnectorDescriptor{Type: typ}
	}
	return descs
}

// openCatalogTestDB returns a freshly migrated, empty database, ready for
// the plugins table catalog.go operates on.
func openCatalogTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatalf("persistence.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := persistence.Migrate(context.Background(), db, migrations.FS, logger); err != nil {
		t.Fatalf("persistence.Migrate() error = %v", err)
	}

	return db
}

func TestInstall_LaunchesHandshakesAndRecordsTheManifest(t *testing.T) {
	db := openCatalogTestDB(t)

	entry, err := Install(context.Background(), db, examplePluginPath)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if entry.PluginID != "io.patchcord.example-text" {
		t.Fatalf("PluginID = %q, want %q", entry.PluginID, "io.patchcord.example-text")
	}
	if entry.Version != "1.0.0" {
		t.Fatalf("Version = %q, want %q", entry.Version, "1.0.0")
	}
	if entry.ExecutablePath != examplePluginPath {
		t.Fatalf("ExecutablePath = %q, want %q", entry.ExecutablePath, examplePluginPath)
	}
	wantActions := []string{"text.uppercase@1", "text.lowercase@1", "text.join@1", "text.split@1", "text.echo_connector@1", "text.replace@1"}
	if !slices.Equal(ActionIDs(entry.Actions), wantActions) {
		t.Fatalf("Actions = %v, want %v", entry.Actions, wantActions)
	}
	for _, action := range entry.Actions {
		if action.Description == "" {
			t.Fatalf("action %q has an empty Description", action.ID)
		}
	}

	got, err := Get(context.Background(), db, entry.PluginID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.PluginID != entry.PluginID {
		t.Fatalf("Get().PluginID = %q, want %q", got.PluginID, entry.PluginID)
	}
}

// TestInstall_RecordsAnAbsolutePathEvenWhenGivenARelativeOne guards against
// a bug where a plugin installed with a relative path (e.g. `patchcord
// plugin install ./text`) would launch fine at install time — since exec
// resolves it against the current working directory — but silently fail to
// launch later, whenever the Supervisor started it (e.g. via `patchcord
// serve`) from a different working directory. `plugin list` would still
// report the plugin as installed, while every action it contributes
// reported "not currently available".
func TestInstall_RecordsAnAbsolutePathEvenWhenGivenARelativeOne(t *testing.T) {
	db := openCatalogTestDB(t)

	dir, file := filepath.Split(examplePluginPath)
	relativePath := "./" + file

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%q) error = %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("os.Chdir(%q) error = %v", oldWD, err)
		}
	})

	entry, err := Install(context.Background(), db, relativePath)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if !filepath.IsAbs(entry.ExecutablePath) {
		t.Fatalf("ExecutablePath = %q, want an absolute path", entry.ExecutablePath)
	}
	// Compare by file identity rather than string equality: on macOS, the
	// temp dir itself sits behind a symlink (/tmp -> /private/tmp), and
	// os.Getwd() after os.Chdir resolves it, while examplePluginPath does
	// not — an artifact of the test fixture, not of Install's behavior.
	sameFile, err := isSameFile(entry.ExecutablePath, examplePluginPath)
	if err != nil {
		t.Fatalf("compare ExecutablePath: %v", err)
	}
	if !sameFile {
		t.Fatalf("ExecutablePath = %q, want it to resolve to the same file as %q", entry.ExecutablePath, examplePluginPath)
	}

	got, err := Get(context.Background(), db, entry.PluginID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !filepath.IsAbs(got.ExecutablePath) {
		t.Fatalf("Get().ExecutablePath = %q, want an absolute path", got.ExecutablePath)
	}
}

// isSameFile reports whether a and b name the same file on disk, resolving
// symlinks on both sides first.
func isSameFile(a, b string) (bool, error) {
	resolvedA, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false, fmt.Errorf("resolve %q: %w", a, err)
	}
	resolvedB, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false, fmt.Errorf("resolve %q: %w", b, err)
	}
	return resolvedA == resolvedB, nil
}

func TestInstall_FailsForABrokenBinary(t *testing.T) {
	t.Setenv("FAKE_PLUGIN_MODE", "garbage")
	db := openCatalogTestDB(t)

	if _, err := Install(context.Background(), db, fakePluginPath); err == nil {
		t.Fatal("expected an error for a plugin that never completes its bootstrap, got nil")
	}
}

func TestInstall_FailsForAMissingBinary(t *testing.T) {
	db := openCatalogTestDB(t)

	if _, err := Install(context.Background(), db, "/does/not/exist"); err == nil {
		t.Fatal("expected an error for a missing binary, got nil")
	}
}

func TestInstall_ReinstallingReplacesTheEntry(t *testing.T) {
	db := openCatalogTestDB(t)

	if _, err := Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if _, err := Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}

	entries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1 (reinstall must not duplicate the catalog entry)", len(entries))
	}
}

func TestList_OrdersByPluginID(t *testing.T) {
	db := openCatalogTestDB(t)

	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.zzz", Version: "1.0.0", ExecutablePath: "/bin/zzz", ProtocolVersion: 1,
	}); err != nil {
		t.Fatalf("seed zzz: %v", err)
	}
	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.aaa", Version: "1.0.0", ExecutablePath: "/bin/aaa", ProtocolVersion: 1,
	}); err != nil {
		t.Fatalf("seed aaa: %v", err)
	}

	entries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 2 || entries[0].PluginID != "io.patchcord.aaa" || entries[1].PluginID != "io.patchcord.zzz" {
		t.Fatalf("entries = %v, want [aaa, zzz] in order", entries)
	}
}

// TestList_SkipsAPluginWithAnUndecodableCatalogEntry reproduces the bug
// that surfaced in production: a plugin seeded before ADR-0062 (protocol
// v1, actions stored as a bare JSON string array) sits in the same catalog
// as plugins installed after it (protocol v2, actions stored as full
// descriptor objects). List must keep returning the still-readable
// entries instead of one legacy row failing the whole call — every caller
// downstream of List (plugin list, KnownActions, KnownConnectorTypes,
// FindAction, FindConnector) would otherwise break for every installed
// plugin because of one unrelated stale entry.
func TestList_SkipsAPluginWithAnUndecodableCatalogEntry(t *testing.T) {
	db := openCatalogTestDB(t)

	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.readable", Version: "1.0.0", ExecutablePath: "/bin/readable", ProtocolVersion: 2,
		Actions: actionDescriptors("readable.action@1"),
	}); err != nil {
		t.Fatalf("seed readable: %v", err)
	}

	// Bypasses upsertCatalogEntry deliberately: it always marshals from an
	// ActionDescriptor slice, which can never produce the bare-string
	// shape a real protocol-v1 catalog entry has on disk.
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO plugins (plugin_id, version, executable_path, protocol_version, connectors, actions, permissions)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "io.patchcord.legacy", "1.0.0", "/bin/legacy", 1, "[]", `["legacy.action@1"]`, "[]"); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	entries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v, want nil (the legacy entry should be skipped, not fail the call)", err)
	}
	if len(entries) != 1 || entries[0].PluginID != "io.patchcord.readable" {
		t.Fatalf("entries = %v, want only io.patchcord.readable", entries)
	}
}

// TestGet_ReturnsAnActionableErrorForAnUndecodableCatalogEntry checks that
// asking for one specific stale plugin by id — as opposed to List, which
// skips it — still fails, but with an error that names the plugin and
// suggests reinstalling it, rather than a bare encoding/json type error.
func TestGet_ReturnsAnActionableErrorForAnUndecodableCatalogEntry(t *testing.T) {
	db := openCatalogTestDB(t)

	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO plugins (plugin_id, version, executable_path, protocol_version, connectors, actions, permissions)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "io.patchcord.legacy", "1.0.0", "/bin/legacy", 1, "[]", `["legacy.action@1"]`, "[]"); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	_, err := Get(context.Background(), db, "io.patchcord.legacy")
	if err == nil {
		t.Fatal("Get() error = nil, want a decode error naming the plugin")
	}
	if !strings.Contains(err.Error(), "io.patchcord.legacy") {
		t.Fatalf("Get() error = %v, want it to name the plugin id", err)
	}
	if !strings.Contains(err.Error(), "plugin uninstall") {
		t.Fatalf("Get() error = %v, want it to suggest reinstalling", err)
	}
}

func TestGet_ReturnsErrNotInstalledForAnUnknownID(t *testing.T) {
	db := openCatalogTestDB(t)

	_, err := Get(context.Background(), db, "io.patchcord.unknown")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Get() error = %v, want ErrNotInstalled", err)
	}
}

func TestFindAction(t *testing.T) {
	db := openCatalogTestDB(t)

	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.a", Version: "1.0.0", ExecutablePath: "/bin/a", ProtocolVersion: 1,
		Actions: []ActionDescriptor{
			{
				ID:          "a.one@1",
				Description: "Does one thing.",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}); err != nil {
		t.Fatalf("seed a: %v", err)
	}

	action, pluginID, err := FindAction(context.Background(), db, "a.one@1")
	if err != nil {
		t.Fatalf("FindAction() error = %v", err)
	}
	if pluginID != "io.patchcord.a" {
		t.Fatalf("FindAction() pluginID = %q, want %q", pluginID, "io.patchcord.a")
	}
	if action.Description != "Does one thing." {
		t.Fatalf("FindAction() Description = %q, want %q", action.Description, "Does one thing.")
	}

	_, _, err = FindAction(context.Background(), db, "a.unknown@1")
	if !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("FindAction() error = %v, want ErrActionNotFound", err)
	}
}

func TestFindConnector(t *testing.T) {
	db := openCatalogTestDB(t)

	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.a", Version: "1.0.0", ExecutablePath: "/bin/a", ProtocolVersion: 1,
		Connectors: []ConnectorDescriptor{
			{
				Type:         "a.connection@1",
				Description:  "Reaches system A.",
				ConfigSchema: map[string]any{"type": "object"},
			},
		},
	}); err != nil {
		t.Fatalf("seed a: %v", err)
	}

	connector, pluginID, err := FindConnector(context.Background(), db, "a.connection@1")
	if err != nil {
		t.Fatalf("FindConnector() error = %v", err)
	}
	if pluginID != "io.patchcord.a" {
		t.Fatalf("FindConnector() pluginID = %q, want %q", pluginID, "io.patchcord.a")
	}
	if connector.Description != "Reaches system A." {
		t.Fatalf("FindConnector() Description = %q, want %q", connector.Description, "Reaches system A.")
	}

	_, _, err = FindConnector(context.Background(), db, "a.unknown@1")
	if !errors.Is(err, ErrConnectorNotFound) {
		t.Fatalf("FindConnector() error = %v, want ErrConnectorNotFound", err)
	}
}

func TestUninstall(t *testing.T) {
	db := openCatalogTestDB(t)

	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.test", Version: "1.0.0", ExecutablePath: "/bin/test", ProtocolVersion: 1,
	}); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	if err := Uninstall(context.Background(), db, "io.patchcord.test"); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if _, err := Get(context.Background(), db, "io.patchcord.test"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Get() after Uninstall() error = %v, want ErrNotInstalled", err)
	}
}

func TestUninstall_ReturnsErrNotInstalledForAnUnknownID(t *testing.T) {
	db := openCatalogTestDB(t)

	err := Uninstall(context.Background(), db, "io.patchcord.unknown")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Uninstall() error = %v, want ErrNotInstalled", err)
	}
}

func TestKnownActions(t *testing.T) {
	db := openCatalogTestDB(t)

	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.a", Version: "1.0.0", ExecutablePath: "/bin/a", ProtocolVersion: 1,
		Actions: actionDescriptors("a.one@1", "a.two@1"),
	}); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.b", Version: "1.0.0", ExecutablePath: "/bin/b", ProtocolVersion: 1,
		Actions: actionDescriptors("b.one@1"),
	}); err != nil {
		t.Fatalf("seed b: %v", err)
	}

	actions, err := KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	for _, want := range []string{"a.one@1", "a.two@1", "b.one@1"} {
		if _, ok := actions[want]; !ok {
			t.Fatalf("KnownActions() = %v, want it to contain %q", actions, want)
		}
	}
	if len(actions) != 3 {
		t.Fatalf("len(KnownActions()) = %d, want 3", len(actions))
	}
}

func TestKnownConnectorTypes(t *testing.T) {
	db := openCatalogTestDB(t)

	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.a", Version: "1.0.0", ExecutablePath: "/bin/a", ProtocolVersion: 1,
		Connectors: connectorDescriptors("a.connection@1"),
	}); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.b", Version: "1.0.0", ExecutablePath: "/bin/b", ProtocolVersion: 1,
		Connectors: connectorDescriptors("b.connection@1"),
	}); err != nil {
		t.Fatalf("seed b: %v", err)
	}
	// A plugin that contributes actions but no connector, like text, must
	// not blow up KnownConnectorTypes.
	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.c", Version: "1.0.0", ExecutablePath: "/bin/c", ProtocolVersion: 1,
		Actions: actionDescriptors("c.one@1"),
	}); err != nil {
		t.Fatalf("seed c: %v", err)
	}

	types, err := KnownConnectorTypes(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownConnectorTypes() error = %v", err)
	}

	for _, want := range []string{"a.connection@1", "b.connection@1"} {
		if _, ok := types[want]; !ok {
			t.Fatalf("KnownConnectorTypes() = %v, want it to contain %q", types, want)
		}
	}
	if len(types) != 2 {
		t.Fatalf("len(KnownConnectorTypes()) = %d, want 2", len(types))
	}
}
