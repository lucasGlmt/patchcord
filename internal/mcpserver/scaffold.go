package mcpserver

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/bundles"
)

// scaffoldTemplateStatic/scaffoldTemplateVite mirror the two template
// names internal/cli/app.go and internal/cli/bundle.go already accept on
// their own --template flag — same vocabulary, not shared code, since
// internal/mcpserver has no reason to depend on internal/cli (the
// dependency only ever runs the other way: internal/cli/mcp.go calls into
// this package, never the reverse).
const (
	scaffoldTemplateStatic = "static"
	scaffoldTemplateVite   = "vite"
)

// registerScaffoldTools adds the two write tools to server. Unlike every
// other tool in this package, these have a side effect (writing files) —
// deliberately scoped to just these two rather than wrapping every write
// path this agent exposes (ADR-0064): scaffolding a multi-file project is
// the one write operation a single structured MCP call meaningfully
// simplifies over composing a shell command, unlike e.g. `plugin install`
// or `workflow install`, which an agent's own Bash tool already runs
// exactly as well.
func registerScaffoldTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "scaffold_app",
		Description: "Writes a new application project into an empty (or not yet existing) directory: a patchcord-app.yaml manifest plus either a plain static template or a Vite + TypeScript template.",
	}, scaffoldApp)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "scaffold_bundle",
		Description: "Writes a new bundle project into an empty (or not yet existing) directory: a bundle.yaml manifest, an embedded app (same templates as scaffold_app), and a workflows/ directory.",
	}, scaffoldBundle)
}

type scaffoldIn struct {
	Dir      string `json:"dir" jsonschema:"The directory to write into. Must not exist yet, or must be empty."`
	ID       string `json:"id" jsonschema:"The stable id to declare in the manifest, e.g. \"io.example.my-app\"."`
	Version  string `json:"version" jsonschema:"The version to declare in the manifest, e.g. \"0.1.0\"."`
	Template string `json:"template,omitempty" jsonschema:"\"static\" (plain HTML) or \"vite\" (Vite + TypeScript, build before install/dev/pack). Defaults to \"static\"."`
}

type scaffoldOut struct {
	Dir      string   `json:"dir"`
	Template string   `json:"template"`
	Files    []string `json:"files"`
}

// resolveScaffoldTemplate validates template against the two names
// internal/cli's own --template flag accepts, defaulting an empty value
// to "static" the same way that flag's own default does.
func resolveScaffoldTemplate(template string) (string, error) {
	switch template {
	case "":
		return scaffoldTemplateStatic, nil
	case scaffoldTemplateStatic, scaffoldTemplateVite:
		return template, nil
	default:
		return "", fmt.Errorf("unknown template %q, want %q or %q", template, scaffoldTemplateStatic, scaffoldTemplateVite)
	}
}

func scaffoldApp(ctx context.Context, _ *mcp.CallToolRequest, in scaffoldIn) (*mcp.CallToolResult, scaffoldOut, error) {
	template, err := resolveScaffoldTemplate(in.Template)
	if err != nil {
		return nil, scaffoldOut{}, fmt.Errorf("scaffold_app: %w", err)
	}

	if template == scaffoldTemplateVite {
		err = apps.ScaffoldVite(in.Dir, in.ID, in.Version)
	} else {
		err = apps.Scaffold(in.Dir, in.ID, in.Version)
	}
	if err != nil {
		return nil, scaffoldOut{}, fmt.Errorf("scaffold_app: %w", err)
	}

	files, err := listScaffoldedFiles(in.Dir)
	if err != nil {
		return nil, scaffoldOut{}, fmt.Errorf("scaffold_app: %w", err)
	}
	return nil, scaffoldOut{Dir: in.Dir, Template: template, Files: files}, nil
}

func scaffoldBundle(ctx context.Context, _ *mcp.CallToolRequest, in scaffoldIn) (*mcp.CallToolResult, scaffoldOut, error) {
	template, err := resolveScaffoldTemplate(in.Template)
	if err != nil {
		return nil, scaffoldOut{}, fmt.Errorf("scaffold_bundle: %w", err)
	}

	if template == scaffoldTemplateVite {
		err = bundles.ScaffoldVite(in.Dir, in.ID, in.Version)
	} else {
		err = bundles.Scaffold(in.Dir, in.ID, in.Version)
	}
	if err != nil {
		return nil, scaffoldOut{}, fmt.Errorf("scaffold_bundle: %w", err)
	}

	files, err := listScaffoldedFiles(in.Dir)
	if err != nil {
		return nil, scaffoldOut{}, fmt.Errorf("scaffold_bundle: %w", err)
	}
	return nil, scaffoldOut{Dir: in.Dir, Template: template, Files: files}, nil
}

// listScaffoldedFiles walks dir and returns every regular file's path
// relative to dir, sorted — so a scaffold tool's result tells the caller
// exactly what landed on disk, not just "it succeeded".
func listScaffoldedFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list scaffolded files: %w", err)
	}
	sort.Strings(files)
	return files, nil
}
