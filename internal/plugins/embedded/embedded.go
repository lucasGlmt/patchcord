// Package embedded holds Patchcord's bundled reference plugins — text,
// json, encoding, http and time (plugins/examples/*, chosen because none
// of them has a concrete external service behind it) — as prebuilt
// executables embedded straight into the patchcord binary for the
// platform it was built for. See ADR-0059.
//
// It exists so a freshly installed agent has a useful set of actions
// available without anyone having to manually locate and `plugin install`
// anything first. Nothing here changes what a plugin *is*: what's embedded
// is byte-for-byte the same standalone executable `plugin install` would
// otherwise be pointed at, still launched and supervised as its own OS
// process speaking the plugin RPC protocol (non-negotiable #3 and #7 in
// CLAUDE.md, and ADR-0002) — embedding only removes the manual step of
// finding the executable in the first place.
//
// internal/plugins.SeedEmbedded is this package's only caller.
package embedded

import (
	"fmt"
	"io/fs"
	"strings"
)

// File is one bundled plugin executable, ready to be written to disk and
// installed.
type File struct {
	// Name is the executable's file name (e.g. "text", or "text.exe" on
	// Windows). It carries no meaning beyond that — a plugin's identity
	// comes entirely from its own handshake manifest, exactly as with any
	// executable passed to `plugin install`.
	Name string
	Data []byte
}

// Files returns every reference plugin executable embedded for the
// current platform.
//
// It returns an empty (not nil) slice, never an error over missing
// content, on a build where the embedding step never ran — e.g. a bare
// `go build` on a fresh checkout, before `make build-embedded-plugins` (or
// the release pipeline's per-target equivalent) has populated bin/. An
// error here means the embedded filesystem itself is malformed, which
// signals a build problem, not "nothing to seed".
func Files() ([]File, error) {
	entries, err := fs.ReadDir(platformFS, platformDir)
	if err != nil {
		return nil, fmt.Errorf("read embedded plugins directory: %w", err)
	}

	files := make([]File, 0, len(entries))
	for _, entry := range entries {
		// .gitkeep (and any other dotfile) is a placeholder that keeps the
		// platform directory present in git and go:embed satisfied even
		// before any real plugin has been built into it — never a plugin
		// to install.
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		data, err := fs.ReadFile(platformFS, platformDir+"/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read embedded plugin %q: %w", entry.Name(), err)
		}
		files = append(files, File{Name: entry.Name(), Data: data})
	}

	return files, nil
}
