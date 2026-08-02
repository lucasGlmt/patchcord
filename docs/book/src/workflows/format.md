# Workflow Format

A workflow is YAML, parsed by `workflow.Parse` (`internal/workflow/definition.go`) into a `Definition`:

```yaml
schema_version: 1

id: greet_twice
version: 4

trigger:
  type: manual

inputs:
  - name: name
    type: string
    required: true
    description: Name to greet.

steps:
  - id: shout
    uses: text.uppercase@1
    with:
      value: "${{ workflow.inputs.name }}"

  - id: shout_again
    uses: text.uppercase@1
    with:
      value: "${{ steps.shout.outputs.value }}"

  - id: say_hello
    uses: text.join@1
    with:
      values:
        - "Salut, "
        - "${{ steps.shout.outputs.value }}"
```

(`workflows/examples/greet_twice.yaml` in full — install and run it with `patchcord workflow install workflows/examples/greet_twice.yaml && patchcord workflow run greet_twice --input name=world`.)

| Field | Meaning |
|---|---|
| `schema_version` | Must equal `workflow.SupportedSchemaVersion` (currently `1`) — the only version this engine understands. |
| `id` | The workflow's stable id, referenced by `patchcord workflow run <id>` and `POST /v1/workflows/{id}/run`. |
| `version` | A positive integer. Installing a workflow never overwrites an existing `(id, version)` — see [Concepts](concepts.md). |
| `trigger.type` | Must be `"manual"` — the only trigger the engine supports today. Scheduled and webhook triggers belong to the scheduler, a later phase (CLAUDE.md §9). |
| `inputs` | Optional — the workflow's declared input schema. See [Declared inputs](#declared-inputs). A workflow with no `inputs` accepts any `${{ workflow.inputs.<key> }}` key, unvalidated, exactly as before this field existed. |
| `steps` | A non-empty list of steps, executed strictly in order. |

Each step:

| Field | Meaning |
|---|---|
| `id` | Unique within the workflow. Other steps reference its outputs as `steps.<id>.outputs.<key>`. |
| `uses` | The action id this step invokes, e.g. `text.uppercase@1`. Must be contributed by a currently installed plugin. |
| `with` | The action's input values, a `map[string]any`. |
| `connector` | Optional — binds a connector to this step's action call. See [Connector binding](#connector-binding). |

## Validation ("compiling" a workflow)

`workflow.Validate` (`internal/workflow/compile.go`) checks a parsed `Definition` before it can be installed or run (vision document, section 12.5):

- `schema_version` is supported, `id` is non-empty, `version` is positive, `trigger.type` is `"manual"`;
- at least one step; every step id is non-empty and unique;
- every `uses` is in `knownActions` — the set of action ids currently installed plugins contribute (`plugins.KnownActions`), passed in by the caller so this package stays free of any persistence or process dependency;
- every `${{ ... }}` expression in `with` or `connector` has a supported shape (below), and every `steps.<id>.outputs...` reference points at a step defined **earlier** in the same list — a forward reference or a typo'd step id is rejected at install time, not at run time.

`patchcord workflow validate <path.yaml>` runs exactly this check without installing anything; `patchcord workflow install <path.yaml>` runs it as a prerequisite to recording the version.

## Declared inputs

`inputs` declares what a workflow expects for its `${{ workflow.inputs.<key> }}` expressions, so a client can validate and collect them before a run starts — a dashboard renders a typed form field per input instead of a free-form JSON textarea, and `GET /v1/workflows/{id}` exposes the schema for exactly that (`workflowDetail.inputs`, `internal/api/workflows.go`).

Each declared input:

| Field | Meaning |
|---|---|
| `name` | The key referenced as `${{ workflow.inputs.<name> }}`. Required, unique within the workflow. |
| `type` | `string` (the default when omitted), `number`, `boolean`, or `enum`. |
| `required` | The run fails fast (before any step runs) if this input isn't provided and has no `default`. Mutually exclusive with `default` — a default would silently satisfy "required", making the flag meaningless. |
| `description` | A human-readable hint (e.g. a generated form field's label). |
| `default` | Used when a run doesn't supply this input. Its YAML type must match `type`. |
| `enum` | The list of values a `type: enum` input may take. Required for `enum`, rejected for every other type. |

`workflow.PrepareInputs` (`internal/workflow/inputs.go`) applies this schema whenever a run starts (`runs.Start`, called by both `patchcord workflow run` and `POST /v1/workflows/{id}/run`):

- a provided key not declared in `inputs` is rejected — a typo'd `--input` flag fails immediately with a clear message, rather than being silently ignored and only surfacing later as "workflow input X was not provided";
- a missing input falls back to `default` if set, else fails if `required`, else is simply omitted;
- values are coerced to their declared type — needed because the CLI's `--input key=value` only ever supplies strings, while an HTTP JSON body already carries typed values, so both paths go through the same coercion.

A workflow that declares no `inputs` at all keeps working exactly as before this field existed: any `${{ workflow.inputs.<key> }}` key may be passed, unvalidated.

## Expressions

A step's `with` values and its `connector` field may reference the workflow's inputs, an earlier step's outputs, or a connector binding, using `${{ ... }}` (`internal/workflow/expr.go`). Three shapes are supported:

| Expression | Resolves to |
|---|---|
| `${{ workflow.inputs.<key> }}` | The value passed for `<key>` when the run was started (`--input key=value` on the CLI, or `inputs` in the HTTP trigger body). |
| `${{ steps.<id>.outputs.<key> }}` | The `<key>` output of step `<id>`, which must already have run. |
| `${{ bindings.<name> }}` | The connector id bound to logical name `<name>` for this run (`--binding name=connector-id`, or `bindings` in the HTTP trigger body). |

A value is only treated as an expression if it is a **string entirely made of one** `${{ ... }}` block — `"${{ steps.shout.outputs.value }}"` resolves, but `"hello ${{ steps.shout.outputs.value }}"` is passed through unchanged, literally, expression syntax and all. There is no partial interpolation.

This applies at any nesting depth inside a `with` value: an expression nested inside a list or object — e.g. `text.join@1`'s `values` list — is resolved the same way a top-level string is ([ADR-0029](../../../adr/0029-expressions-resolution-recursive-listes-objets.md)):

```yaml
with:
  values:
    - "Salut, "
    - "${{ steps.shout.outputs.value }}"
```

## Connector binding

A step's `connector:` field, when present, must itself be a `${{ ... }}` expression — never a literal connector id (`workflow.Step.Connector`'s doc comment; enforced by `Validate`). This is what keeps a published, immutable workflow version portable: it never bakes in one deployment's specific connector identity, only an indirection resolved at run time — typically `${{ bindings.<name> }}`, though `${{ workflow.inputs.<key> }}` and `${{ steps.<id>.outputs.<key> }}` are equally legitimate ([ADR-0021](../../../adr/0021-binding-connecteur-workflow-protocole.md), consistent with [ADR-0008](../../../adr/0008-workflows-publies-immuables.md)'s immutability guarantee).

```yaml
steps:
  - id: show_connector
    uses: text.echo_connector@1
    connector: "${{ bindings.demo }}"
    with: {}
```

(`workflows/examples/connector_binding_demo.yaml`.) Run with a connector already created under some id, mapped to the workflow's logical `demo` binding name:

```bash
patchcord connector create demo_conn --type "demo.connection@1" --config greeting=hello --secret token=env:DEMO_TOKEN
DEMO_TOKEN=s3cr3t patchcord workflow run connector_binding_demo --binding demo=demo_conn
```
