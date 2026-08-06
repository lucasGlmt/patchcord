# patchcord mcp

Starts an MCP (Model Context Protocol) server that exposes this agent's installed-plugin catalog and workflow validation to a coding agent (Claude Code, Codex) building a bundle/app — so it can ground itself in real action/connector schemas and validate a workflow draft against the live catalog instead of guessing (see [ADR-0062](../../../../adr/0062-descripteurs-schema-actions-et-connecteurs.md), [ADR-0063](../../../../adr/0063-validation-input-schema-step-workflow.md), [ADR-0064](../../../../adr/0064-serveur-mcp-local-catalogue-validation-scaffold.md)).

## `serve`

```bash
patchcord mcp serve
patchcord mcp serve --data-dir /path/to/data
```

Starts the server over **stdio** — the transport a coding agent registers a local MCP server through, as a subprocess it launches and talks to over stdin/stdout. There is no HTTP transport: this command never binds a port, consistent with the agent never requiring a cloud service to run ([ADR-0007](../../../../adr/0007-cloud-jamais-requis.md)). It blocks until the client disconnects or the process is signaled (`Ctrl-C`/`SIGTERM`), same as `patchcord serve`.

Reads the same `--data-dir` catalog `patchcord serve`/`plugin install` populate — it never launches or supervises plugin processes itself, only reads what's already installed.

### Registering with Claude Code

```bash
claude mcp add patchcord -- patchcord mcp serve --data-dir /path/to/data
```

or, in a project's `.mcp.json`:

```json
{
  "mcpServers": {
    "patchcord": {
      "command": "patchcord",
      "args": ["mcp", "serve", "--data-dir", "/path/to/data"]
    }
  }
}
```

### Registering with Codex

```bash
codex mcp add patchcord -- patchcord mcp serve --data-dir /path/to/data
```

or, in `~/.codex/config.toml` (or a trusted project's `.codex/config.toml`):

```toml
[mcp_servers.patchcord]
command = "patchcord"
args = ["mcp", "serve", "--data-dir", "/path/to/data"]
```

### Tools

| Tool | Purpose |
|---|---|
| `list_plugins` | Lists every installed plugin (id, version, permissions, action/connector counts). |
| `list_actions` | Lists every action across installed plugins, optionally filtered to one plugin id. Descriptions only. |
| `describe_action` | One action's full description, input/output JSON Schema, and default timeout. |
| `list_connectors` | Lists every connector type across installed plugins. Descriptions only. |
| `describe_connector` | One connector type's full description and non-secret configuration JSON Schema. |
| `validate_workflow` | Parses and validates a workflow YAML draft against the live catalog — reports validity as data (`valid`/`error` fields), never as a tool failure, since a rejected draft is this tool's normal, useful result. |
| `list_workflows` | Lists every installed workflow version. |
| `get_workflow_source` | Fetches one installed workflow version's raw YAML. |
| `scaffold_app` | Writes a new application project (`static` or `vite` template — same two names as [`app new --template`](app.md)). |
| `scaffold_bundle` | Writes a new bundle project (app + workflows + manifest, same templates). |

`scaffold_app`/`scaffold_bundle` are the only two tools with a side effect (they write files) — every other tool is read-only. Both refuse to write into a directory that already has files in it, the same guard `app new`/`bundle new` already enforce.

The server's `Instructions` (surfaced to the client at initialization, not a tool) point to the published documentation: <https://lucasglmt.github.io/patchcord/>.

### Manual testing

[`@modelcontextprotocol/inspector`](https://github.com/modelcontextprotocol/inspector) is the standard way to exercise a stdio MCP server by hand — it launches the command as a subprocess and gives you a browser UI to list tools, fill in arguments, and inspect raw JSON-RPC traffic:

```bash
npx @modelcontextprotocol/inspector patchcord mcp serve -- --data-dir /tmp/patchcord-mcp-smoke
```
