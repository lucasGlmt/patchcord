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
| `trigger.type` | `"manual"` or `"schedule"`. Webhook and event triggers remain a later phase (CLAUDE.md §9). See [Schedule trigger](#schedule-trigger). |
| `inputs` | Optional — the workflow's declared input schema. See [Declared inputs](#declared-inputs). A workflow with no `inputs` accepts any `${{ workflow.inputs.<key> }}` key, unvalidated, exactly as before this field existed. |
| `steps` | A non-empty list of steps, executed strictly in order. |

Each step:

| Field | Meaning |
|---|---|
| `id` | Unique within the workflow. Other steps reference its outputs as `steps.<id>.outputs.<key>`. |
| `uses` | The action id this step invokes, e.g. `text.uppercase@1`. Must be contributed by a currently installed plugin. |
| `with` | The action's input values, a `map[string]any`. |
| `connector` | Optional — binds a connector to this step's action call. See [Connector binding](#connector-binding). |
| `if` | Optional — gates whether this step runs at all. See [Conditional steps](#conditional-steps). |
| `stop_if_false` | Optional, boolean — when `if` resolves to `false`, ends the run there instead of only skipping this step. See [Conditional steps](#conditional-steps). |
| `else_of` | Optional — names an earlier step; this step is skipped whenever that step actually ran. See [Branching without nesting](#branching-without-nesting-else_of). |
| `foreach` | Optional — runs this step's action once per item of a list. See [Foreach steps](#foreach-steps). |

## Validation ("compiling" a workflow)

`workflow.Validate` (`internal/workflow/compile.go`) checks a parsed `Definition` before it can be installed or run (vision document, section 12.5):

- `schema_version` is supported, `id` is non-empty, `version` is positive, `trigger.type` is `"manual"` or `"schedule"` — see [Schedule trigger](#schedule-trigger) for the extra rules a `"schedule"` trigger must satisfy;
- at least one step; every step id is non-empty and unique;
- every `uses` is in `knownActions` — the set of action ids currently installed plugins contribute (`plugins.KnownActions`), passed in by the caller so this package stays free of any persistence or process dependency;
- every `${{ ... }}` expression in `with`, `connector`, `if` or `foreach` has a supported shape (below), and every `steps.<id>.outputs...` reference points at a step defined **earlier** in the same list — a forward reference or a typo'd step id is rejected at install time, not at run time;
- a step's `if`, when set, is either a literal boolean or a `${{ ... }}` expression — never any other literal (a string, a number...);
- a step's `foreach`, when set, is either a literal list or a `${{ ... }}` expression;
- `${{ each }}` only appears inside the `with` of the step declaring `foreach` — anywhere else (`if`, `connector`, `foreach` itself, another step's `with`) it is rejected at install time, since no iteration is in progress there;
- a comparison expression's (`<path> <op> <literal>`, see [Comparisons](#comparisons)) left-hand path is a supported shape and its literal right-hand side is well-formed;
- `stop_if_false` requires `if` to be set — there would otherwise be nothing for it to react to;
- `else_of`, when set, names a step defined **earlier** in the same workflow, the same forward-reference rule as everything else.

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

## Schedule trigger

`trigger: { type: schedule, cron: "...", on_missed: ... }` fires a workflow unattended, on a cron cadence, with nobody supplying inputs or connector bindings the way a manual run's caller does (`workflow.Trigger`, `internal/scheduler`, [ADR-0035](../../../adr/0035-trigger-schedule-scheduler-persistant.md)):

```yaml
trigger:
  type: schedule
  cron: "*/5 * * * *"
  on_missed: skip
```

(`workflows/examples/scheduled_demo.yaml`. Install it and it starts firing on its own — no `workflow run` needed: `patchcord workflow install workflows/examples/scheduled_demo.yaml`.)

| Field | Meaning |
|---|---|
| `cron` | A standard 5-field cron expression (`minute hour day-of-month month day-of-week`). Required, validated by `Validate` at install time — a typo is rejected before the workflow is ever published, not discovered the first time it fails to fire. |
| `on_missed` | `skip` (the default when omitted) or `fire_once`. Governs what happens to occurrences the scheduler couldn't fire because the agent was offline across more than one of them — see below. |

Because nobody is present to supply inputs or a connector id when a scheduled run starts, `Validate` additionally rejects, for a `"schedule"` trigger only:

- any declared input with `required: true` and no `default` (see [Declared inputs](#declared-inputs)) — there would be nothing to satisfy it;
- any step with a `connector:` binding — there is no `bindings` map for it to resolve against.

A workflow needing either should stay `"manual"`, or move its connector/input decisions into the action itself (a fixed `with` value, or a connector referenced by a fixed id is still rejected — bindings must stay an expression per [Connector binding](#connector-binding) — so today that really does mean staying `"manual"`).

`internal/scheduler.Runner` polls for due workflows and fires each one through the same `runs.Execute` path a manual run or `POST /v1/workflows/{id}/run` uses — a scheduled run is not a different kind of run, just a differently triggered one, and shows up identically in `patchcord workflow runs <id>` / `GET /v1/runs`.

Installing a new version of a `"schedule"`-triggered workflow always reschedules from that moment on — `internal/scheduler.Sync` recomputes `next_run_at` from "now" using the newly installed version's `cron`, regardless of what an earlier version's cron was. Switching a version's trigger back to `"manual"` removes the schedule entirely.

### Missed occurrences

If the agent was offline past a scheduled firing, `on_missed` decides what happens once it comes back:

- **`skip`** (default) — drop the backlog and resume at the next future occurrence. No burst of catch-up runs on restart.
- **`fire_once`** — run once for the most recently missed occurrence, then resume normal cadence, regardless of how many occurrences were actually missed.

Missing exactly one occurrence (the ordinary case — the agent was running continuously and the schedule simply came due) always fires, independent of `on_missed`; the policy only kicks in once more than one occurrence has gone by, which only happens after a stretch of downtime.

`GET /v1/workflows/{id}` exposes the resolved state for a client: `trigger_type`, `trigger_cron`, `trigger_on_missed` (always the effective policy — `"skip"` even when the YAML left it implicit), and `next_run_at` (from the `schedules` table, so it reflects the live schedule even when viewing an older installed version).

## Expressions

A step's `with` values, its `connector` field, its `if` field, and its `foreach` field may reference the workflow's inputs, an earlier step's outputs, or a connector binding, using `${{ ... }}` (`internal/workflow/expr.go`). Four shapes are supported:

| Expression | Resolves to |
|---|---|
| `${{ workflow.inputs.<key> }}` | The value passed for `<key>` when the run was started (`--input key=value` on the CLI, or `inputs` in the HTTP trigger body). |
| `${{ steps.<id>.outputs.<key> }}` | The `<key>` output of step `<id>`, which must already have run. |
| `${{ bindings.<name> }}` | The connector id bound to logical name `<name>` for this run (`--binding name=connector-id`, or `bindings` in the HTTP trigger body). |
| `${{ each }}` | The current item of a `foreach` iteration. Only valid inside the `with` of the step declaring `foreach` — see [Foreach steps](#foreach-steps). |

A value is only treated as an expression if it is a **string entirely made of one** `${{ ... }}` block — `"${{ steps.shout.outputs.value }}"` resolves, but `"hello ${{ steps.shout.outputs.value }}"` is passed through unchanged, literally, expression syntax and all. There is no partial interpolation.

This applies at any nesting depth inside a `with` value: an expression nested inside a list or object — e.g. `text.join@1`'s `values` list — is resolved the same way a top-level string is ([ADR-0029](../../../adr/0029-expressions-resolution-recursive-listes-objets.md)):

```yaml
with:
  values:
    - "Salut, "
    - "${{ steps.shout.outputs.value }}"
```

### Comparisons

The content inside `${{ ... }}` may also be a comparison — `<path> <op> <literal>` — instead of a bare path, resolving to a boolean ([ADR-0033](../../../adr/0033-comparaisons-arret-anticipe-else-of.md)):

```yaml
if: "${{ steps.get_status.outputs.value >= 8 }}"
```

Six operators are supported: `==`, `!=`, `>`, `>=`, `<`, `<=`. The left-hand side is any of the four expression shapes above; the right-hand side is a **literal**, never another expression — a quoted string (`'active'` or `"active"`), a bare number, or `true`/`false`. `==`/`!=` compare any pair of these three types (always `false` across mismatched types, e.g. a number compared to a string); the ordering operators (`>`, `>=`, `<`, `<=`) require both sides to be numbers and fail with a clear error otherwise — comparing strings lexicographically would be a silent, locale-dependent surprise, not the arithmetic comparison the syntax suggests.

This is deliberately a closed, fixed grammar — one path, one operator, one literal — not a growing expression language: no `&&`/`||`/`!`, and no comparing one expression against another. Combine multiple comparisons across separate steps ([else_of](#branching-without-nesting-else_of) below) rather than in one `if`.

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

`GET /v1/workflows/{id}` exposes each such binding for a client that wants to offer a picker instead of a free-text connector id: `workflowDetail.bindings` lists every distinct `${{ bindings.<name> }}` name used across the workflow's steps, each paired with a `connectorType` **inferred** server-side (the installed plugin that contributes the step's `uses` action, and its declared connector type) when unambiguous ([ADR-0034](../../../adr/0034-connecteurs-catalogue-greffons-http-bindings-dashboard.md)). A `connector:` expression over `workflow.inputs` or `steps.*.outputs` has no static connector id to offer ahead of a run, so it never appears in `bindings` — only the `${{ bindings.<name> }}` shape does. The dashboard's workflow detail page (`apps/examples/dashboard`) uses exactly this to render one `<select>` per binding, filled from `GET /v1/connectors` filtered by `connectorType`.

## Conditional steps

A step's `if:` field gates whether it runs at all (`workflow.Step.If`; enforced and resolved by `Validate` and `workflow.ResolveIf`, [ADR-0031](../../../adr/0031-etapes-conditionnelles-if.md)). It is either a literal boolean, or a `${{ ... }}` expression that must itself resolve to one — the same three shapes as any other expression, referencing a workflow input, an earlier step's output, or a connector binding. A step with no `if` always runs, exactly as before this field existed.

```yaml
steps:
  - id: shout
    if: "${{ workflow.inputs.shout }}"
    uses: text.uppercase@1
    with:
      value: "hello, patchcord"
```

(`workflows/examples/conditional_step_demo.yaml`.) When `if` resolves to `false`, the step is recorded as `skipped` — its action is never invoked, and its outputs are unavailable to later steps (referencing `${{ steps.<id>.outputs.<key> }}` for a skipped step fails the run, the same way referencing a step that never ran does). Unlike a step skipped because an earlier step failed, a step skipped by its own `if` does **not** stop the run: execution moves on to the next step normally.

```bash
patchcord workflow run conditional_step_demo --input shout=true   # runs "shout"
patchcord workflow run conditional_step_demo --input shout=false  # skips "shout"
```

There is no dedicated action for conditions (no `cond.if@1`) — `if` is a step-level control attribute the engine evaluates before calling the action, not a capability a plugin contributes. `foreach` follows the same pattern; see below.

### Stopping the run early

`stop_if_false: true`, alongside `if`, changes what a `false` does: instead of only skipping this one step, it skips this step **and every step after it**, ending the run — a guard clause's early return, not an error. The run's final status is still `succeeded`: nothing failed, the workflow simply chose to stop ([ADR-0033](../../../adr/0033-comparaisons-arret-anticipe-else-of.md)).

```yaml
steps:
  - id: guard
    if: "${{ workflow.inputs.score <= 100 }}"
    stop_if_false: true
    uses: text.uppercase@1
    with:
      value: "score in valid range"

  - id: after_guard   # never runs when guard's if was false
    uses: text.uppercase@1
    with:
      value: "after guard"
```

`stop_if_false` requires `if` to be set (`Validate` rejects it otherwise) — there is nothing for it to react to on a step that always runs.

## Branching without nesting (`else_of`)

`else_of: <step_id>` names an earlier step: this step is skipped whenever that step actually ran (`workflow.Step.ElseOf`, resolved by `internal/runs`'s runner, [ADR-0033](../../../adr/0033-comparaisons-arret-anticipe-else-of.md)). It combines with this step's own `if`, if present, as an additional condition (both must hold for the step to run) — a step with `else_of` but no `if` of its own is that chain's unconditional "else".

```yaml
steps:
  - id: case_high
    if: "${{ workflow.inputs.score >= 8 }}"
    uses: text.uppercase@1
    with:
      value: "high score"

  - id: case_mid
    else_of: case_high
    if: "${{ workflow.inputs.score >= 5 }}"
    uses: text.uppercase@1
    with:
      value: "mid score"

  - id: case_low
    else_of: case_mid       # chains onto the previous link, not case_high
    uses: text.uppercase@1
    with:
      value: "low score"
```

(`workflows/examples/branching_demo.yaml`.) Chaining `else_of` onto the **immediately preceding step in the chain** — not always the first one — builds an if/elseif/else without any nesting: exactly one of `case_high`/`case_mid`/`case_low` runs for a given `score`. This works because "did an earlier link in the chain run" propagates forward through the chain, not just "did my direct predecessor run" — a step skipped via its own `else_of` is itself treated as having its branch "taken" by whichever step caused that skip, so a later `else_of` pointing at it still sees the chain as resolved.

A step whose `else_of` step never ran (skipped, itself `else_of`'d out, or the run never reached it) is unaffected by `else_of` — its own `if`, if any, is evaluated normally.

There is still no dedicated action for any of this (no `cond.if@1`, no `switch@1`) — `if`, `stop_if_false`, `else_of` and `foreach` are all step-level control attributes the engine evaluates before calling an action, never a capability a plugin contributes.

## Foreach steps

A step's `foreach:` field runs its action once per item of a list, instead of once (`workflow.Step.Foreach`; enforced and resolved by `Validate` and `workflow.ResolveForeach`, [ADR-0032](../../../adr/0032-etapes-foreach-map-agregation-sortie.md)). It is either a literal list, or a `${{ ... }}` expression that must itself resolve to one. Inside that step's own `with` — nowhere else — `${{ each }}` resolves to the current item.

```yaml
steps:
  - id: shout
    uses: text.uppercase@1
    foreach: "${{ steps.extract_username.outputs.values }}"
    with:
      value: "${{ each }}"
```

(`workflows/examples/foreach_demo.yaml`; also `workflows/examples/http_api.yaml`, where a `json.jsonpath@1` step's list of extracted usernames feeds a `foreach` uppercase step — the motivating case: most actions take a single string, not a list, so the list has to be iterated rather than passed straight through.) The step's outputs are **lists**, not scalars: `steps.shout.outputs.value` above is `["ALICE", "BOB", ...]`, one entry per item, in order — the action's own output shape (a `map[string]any`) is unchanged, only every value becomes an array (a "map" over the list, in the `Array.prototype.map` sense, generic over whatever the action returns — not limited to string-returning actions).

A `connector:` on a foreach step is resolved once, before the first item, and reused for every iteration — it does not vary per item the way `with` does. Iterations run **sequentially**, sharing the step's single `StepTimeout` budget across every item combined (a long list needs a correspondingly larger `--step-timeout`, not a per-item one). The first item to fail stops the run exactly like a normal step failure — there is no retry or continue-on-error policy yet, so review that item's error, fix the workflow or its data, and rerun. An empty list is not an error: the step succeeds having called its action zero times, with an empty outputs map (there are no output keys to know about without at least one call).

`if` and `foreach` combine on the same step: `if` is evaluated first and, when false, skips the step (and thus the whole iteration) exactly as it does for a non-foreach step.
