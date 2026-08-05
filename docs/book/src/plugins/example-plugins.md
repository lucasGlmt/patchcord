# Example Plugins

`plugins/examples/` contains the reference plugins used to develop and test the protocol. Every action below is real: the ID, inputs, and outputs come directly from the plugin's source, and each usage snippet is lifted from a matching workflow in `workflows/examples/` (see [Writing a Plugin in Go](writing-a-plugin-go.md) for how these plugins are built, and [Workflow Format](../workflows/format.md) for the full step syntax).

`text`, `encoding`, `json`, `time` and `http` are bundled in the `patchcord` binary and auto-installed into any `--data-dir` the first time it's touched, so their `plugin install` step below is shown for completeness but isn't actually needed — see [ADR-0059](../../../adr/0059-greffons-de-reference-embarques-et-auto-installes.md). `openai`, `postgresql` and `mysql` are not bundled: `plugin install` is required for those.

Every number crossing the plugin protocol arrives and leaves as a `float64` (the only numeric kind `google.protobuf.Struct` has) — this is why, e.g., `time.now@1`'s `unix` output or `postgresql.execute@1`'s `rows_affected` are numbers with no integer/float distinction on the wire.

## text — `io.patchcord.example-text`

No connector required for its four text actions; `text.echo_connector@1` exists only to prove that a bound connector reaches a plugin process intact ([ADR-0021](../../../adr/0021-binding-connecteur-workflow-protocole.md)) and also declares a demo connector type, `demo.connection@1`, that exists only for this purpose.

| Action | Input | Output |
|---|---|---|
| `text.uppercase@1` | `value` (string) | `value` (string) |
| `text.lowercase@1` | `value` (string) | `value` (string) |
| `text.join@1` | `values` (list of strings), `separator` (string, optional) | `value` (string) |
| `text.split@1` | `value` (string), `separator` (string) | `values` (list of strings) |
| `text.echo_connector@1` | none | `bound` (bool); if `true`, also `type` (string) and `config` (object) — never `connector.Secrets` (see [Manifest & Actions](manifest-and-actions.md#a-connector-consuming-action)) |

This is the reference vertical slice from the vision document (section 20) — `hello_patchcord.yaml` runs a single step:

```yaml
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "Welcome Patchcord"
```

`greet_twice.yaml` chains three steps to show step-to-step output binding with `${{ steps.<id>.outputs.<field> }}`:

```yaml
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

`connector_binding_demo.yaml` shows binding a connector to a step via `${{ bindings.<name> }}` — never a literal connector ID, so a published workflow version never bakes in one deployment's connector identity ([ADR-0008](../../../adr/0008-workflows-publies-immuables.md)):

```yaml
steps:
  - id: show_connector
    uses: text.echo_connector@1
    connector: "${{ bindings.demo }}"
    with: {}
```

Run it with:

```bash
patchcord connector create demo_conn --type "demo.connection@1" \
    --config greeting=hello --secret token=env:DEMO_TOKEN
DEMO_TOKEN=s3cr3t patchcord workflow run connector_binding_demo \
    --binding demo=demo_conn
```

## encoding — `io.patchcord.example-encoding`

No connector. Bundles small, unrelated utility actions that don't individually warrant their own supervised process.

| Action | Input | Output |
|---|---|---|
| `base64.encode@1` | `value` (string) | `value` (string) |
| `base64.decode@1` | `value` (string, base64) | `value` (string) |
| `hash.sha256@1` | `value` (string) | `value` (hex-encoded string) |
| `uuid.generate@1` | none | `value` (string, UUIDv4) |

`encoding_utils_demo.yaml` generates a UUID, base64-encodes it, then hashes the encoded form:

```yaml
steps:
  - id: id
    uses: uuid.generate@1
    with: {}

  - id: encoded
    uses: base64.encode@1
    with:
      value: "${{ steps.id.outputs.value }}"

  - id: hashed
    uses: hash.sha256@1
    with:
      value: "${{ steps.encoded.outputs.value }}"
```

```bash
patchcord plugin install ./bin/plugins/encoding
patchcord workflow install workflows/examples/encoding_utils_demo.yaml
patchcord workflow run encoding_utils_demo
```

## json — `io.patchcord.example-json`

No connector. `json.jsonpath@1` implements a deliberately minimal subset of JSONPath: the root selector (`$`), dot access (`.key`), bracket access with a quoted key (`['key']`/`["key"]`) or a numeric index (`[0]`), and the wildcard (`[*]`). Recursive descent (`..`) and filter expressions (`[?(...)]`) are not supported.

| Action | Input | Output |
|---|---|---|
| `json.parse@1` | `value` (string) | `value` (any decoded JSON value) |
| `json.stringify@1` | `value` (any, required), `pretty` (bool, optional) | `value` (string) |
| `json.jsonpath@1` | `value` (any), `path` (string) | `found` (bool), `value` (first match, or `null`), `values` (list of all matches) |
| `json.merge@1` | `base` (object), `patch` (object) | `value` (object — shallow merge, `patch` wins on key conflicts) |

`json_pipeline_demo.yaml` parses a JSON string, extracts a field with `json.jsonpath@1`, merges in an extra field, then serializes the result back:

```yaml
steps:
  - id: parse
    uses: json.parse@1
    with:
      value: '{"name": "Patchcord", "tags": ["core", "plugin"]}'

  - id: extract_name
    uses: json.jsonpath@1
    with:
      value: "${{ steps.parse.outputs.value }}"
      path: "$.name"

  - id: merge_greeting
    uses: json.merge@1
    with:
      base: "${{ steps.parse.outputs.value }}"
      patch:
        greeting: "hello"

  - id: stringify
    uses: json.stringify@1
    with:
      value: "${{ steps.merge_greeting.outputs.value }}"
      pretty: true
```

## time — `io.patchcord.example-time`

No connector. Every action that produces a moment in time represents it as RFC3339 in UTC, so the actions compose directly without a conversion step in between.

| Action | Input | Output |
|---|---|---|
| `time.now@1` | none | `value` (RFC3339 UTC string), `unix` (number) |
| `time.format@1` | `value` (RFC3339 string), `layout` (Go reference-time layout, e.g. `2006-01-02`) | `value` (string, formatted per `layout`) |
| `time.parse@1` | `value` (string per `layout`), `layout` (string) | `value` (RFC3339 UTC string), `unix` (number) |
| `time.add@1` | `value` (RFC3339 string), `duration` (Go duration string, e.g. `24h`) | `value` (RFC3339 UTC string) |

`time_utils_demo.yaml` gets the current time, adds 24 hours, then formats the result:

```yaml
steps:
  - id: now
    uses: time.now@1
    with: {}

  - id: tomorrow
    uses: time.add@1
    with:
      value: "${{ steps.now.outputs.value }}"
      duration: "24h"

  - id: formatted
    uses: time.format@1
    with:
      value: "${{ steps.tomorrow.outputs.value }}"
      layout: "2006-01-02"
```

## http — `io.patchcord.example-http`

Connector: `http.connection@1` — `Config.base_url` (string, required), `Secrets.authorization` (optional, sent as the `Authorization` header). Declares `network.outbound`. `http.request@1` requires a bound connector and only returns a Go error for a request that never completed (bad configuration, network failure, cancelled/timed-out context) — a non-2xx response is a legitimate result reported via `status`, so a workflow can branch on it.

| Action | Input | Output |
|---|---|---|
| `http.request@1` | `method` (string, optional, defaults to `GET`), `path` (string, optional, appended to `base_url`), `body` (string, optional), `headers` (object of string values, optional) | `status` (number), `headers` (object), `body` (string) |

`http_httpbin_demo.yaml` binds a connector via `${{ bindings.api }}` and calls the public httpbin test service:

```yaml
steps:
  - id: call
    uses: http.request@1
    connector: "${{ bindings.api }}"
    with:
      path: "/get"
```

```bash
patchcord plugin install ./bin/plugins/http
patchcord connector create httpbin --type "http.connection@1" \
    --config base_url=https://httpbin.org
patchcord workflow install workflows/examples/http_httpbin_demo.yaml
patchcord workflow run http_httpbin_demo --binding api=httpbin
```

## openai — `io.patchcord.example-openai`

Connector: `openai.connection@1` — `Config.base_url` (optional, defaults to `https://api.openai.com/v1`, so this plugin also reaches Azure OpenAI or another OpenAI-compatible proxy), `Secrets.api_key` (required). Declares `network.outbound`. Unlike `http.request@1`, `ai.generate_text@1` knows the exact shape of what it's calling: a non-2xx response *is* a Go error here, since there is no meaningful `text` to hand back on failure.

| Action | Input | Output |
|---|---|---|
| `ai.generate_text@1` | `model` (string), `prompt` (string), `system` (string, optional), `temperature` (number, optional), `max_tokens` (number, optional) | `text` (string), `finish_reason` (string), `usage` (object: `prompt_tokens`, `completion_tokens`, `total_tokens`) |

`ai_generate_text_demo.yaml`:

```yaml
steps:
  - id: ask
    uses: ai.generate_text@1
    connector: "${{ bindings.provider }}"
    with:
      model: gpt-4o-mini
      prompt: "In one short sentence, what is Patchcord?"
```

Every run against a real key is billed by OpenAI — never put the key directly in `--secret`; export it as an environment variable and reference it by name:

```bash
patchcord plugin install ./bin/plugins/openai
export OPENAI_API_KEY=sk-...
patchcord connector create openai --type "openai.connection@1" \
    --secret api_key=env:OPENAI_API_KEY
patchcord workflow install workflows/examples/ai_generate_text_demo.yaml
patchcord workflow run ai_generate_text_demo --binding provider=openai
```

## postgresql — `io.patchcord.example-postgresql`

Connector: `postgresql.connection@1` — `Config.host`, `Config.database`, `Config.user` (all required), `Config.port` (optional, defaults to `5432`), `Config.sslmode` (optional, defaults to `disable`), `Secrets.password` (optional). Declares `network.outbound` and implements `ConnectorTester` (opens a connection and pings it — see [Testing a Connector](connectors/testing.md)). A connection is opened per action call and closed before it returns, to keep the action stateless.

| Action | Input | Output |
|---|---|---|
| `postgresql.query@1` | `sql` (string), `args` (list, optional) | `rows` (list of objects), `row_count` (number) |
| `postgresql.execute@1` | `sql` (string), `args` (list, optional) | `rows_affected` (number) |

`postgresql_query_demo.yaml` runs a trivial query needing no schema, so it works against any reachable server/database/user combination:

```yaml
steps:
  - id: ping
    uses: postgresql.query@1
    connector: "${{ bindings.db }}"
    with:
      sql: "SELECT 1 AS ok"
```

```bash
patchcord plugin install ./bin/plugins/postgresql
patchcord connector create pg --type "postgresql.connection@1" \
    --config host=localhost --config port=5432 \
    --config database=app --config user=admin \
    --secret password=env:PG_PASSWORD
patchcord connector test pg   # optional: verify the connection first
patchcord workflow install workflows/examples/postgresql_query_demo.yaml
patchcord workflow run postgresql_query_demo --binding db=pg
```

## mysql — `io.patchcord.example-mysql`

Connector: `mysql.connection@1` — `Config.host`, `Config.database`, `Config.user` (all required), `Config.port` (optional, defaults to `3306`), `Config.params` (object of string values, optional, passed through to the driver DSN), `Secrets.password` (optional). Same shape as `postgresql` in every other respect: declares `network.outbound`, implements `ConnectorTester`, opens/closes a connection per call.

| Action | Input | Output |
|---|---|---|
| `mysql.query@1` | `sql` (string), `args` (list, optional) | `rows` (list of objects), `row_count` (number) |
| `mysql.execute@1` | `sql` (string), `args` (list, optional) | `rows_affected` (number) |

`mysql_query_demo.yaml`:

```yaml
steps:
  - id: ping
    uses: mysql.query@1
    connector: "${{ bindings.db }}"
    with:
      sql: "SELECT 1 AS ok"
```

```bash
patchcord plugin install ./bin/plugins/mysql
patchcord connector create mysql-db --type "mysql.connection@1" \
    --config host=localhost --config port=3306 \
    --config database=app --config user=admin \
    --secret password=env:MYSQL_PASSWORD
patchcord connector test mysql-db   # optional: verify the connection first
patchcord workflow install workflows/examples/mysql_query_demo.yaml
patchcord workflow run mysql_query_demo --binding db=mysql-db
```
