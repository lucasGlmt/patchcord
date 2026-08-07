package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/scheduler"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

// runWorkflowRequest is the optional JSON body of POST
// /v1/workflows/{id}/run — the HTTP equivalent of `workflow run`'s
// --input/--binding flags.
type runWorkflowRequest struct {
	Inputs   map[string]any    `json:"inputs"`
	Bindings map[string]string `json:"bindings"`
}

// workflowSummary is the JSON shape of one installed workflow version, as
// returned by GET /v1/workflows.
type workflowSummary struct {
	ID          string    `json:"id"`
	Version     int       `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
}

// workflowStepDetail is the JSON shape of one step within a workflowDetail.
type workflowStepDetail struct {
	ID        string         `json:"id"`
	Uses      string         `json:"uses"`
	With      map[string]any `json:"with,omitempty"`
	Connector string         `json:"connector,omitempty"`
	// BindingName is the logical name this step's Connector expression
	// refers to (the "db" in "${{ bindings.db }}"), populated only when
	// Connector is exactly that shape — see bindingName.
	BindingName string `json:"binding_name,omitempty"`
	// ConnectorType is the connector type inferred for BindingName, so a
	// client can offer a <select> of matching connectors instead of a
	// free-text id — see bindingConnectorType. Left empty when it can't be
	// inferred (no owning plugin found, or that plugin declares more than
	// one connector type).
	ConnectorType string `json:"connector_type,omitempty"`
}

// workflowBindingDetail is one connector binding a workflow declares across
// its steps, deduplicated by name — the same design as workflowInputDetail:
// a small typed array a client iterates to render one form control per
// binding (here, a <select> of connectors) instead of a free-form JSON blob.
type workflowBindingDetail struct {
	Name string `json:"name"`
	// ConnectorType is empty when it could not be inferred for every step
	// that references this binding name, or when different steps referring
	// to the same name disagree on it.
	ConnectorType string `json:"connector_type,omitempty"`
}

// bindingNamePattern matches exactly "${{ bindings.<name> }}" (arbitrary
// inner whitespace), the only connector expression shape this endpoint
// tries to interpret — see bindingName's doc comment.
var bindingNamePattern = regexp.MustCompile(`^\$\{\{\s*bindings\.([a-zA-Z0-9_]+)\s*\}\}$`)

// bindingName extracts the logical binding name from a step's Connector
// expression when it is exactly "${{ bindings.<name> }}" — the shape every
// current example uses (workflow.ResolveConnector also accepts a connector
// expression over workflow.inputs or steps.*.outputs, but those compute a
// connector id dynamically at run time, so there's no static "which
// connector" a client could offer ahead of a run; ok is false for those,
// and the dashboard falls back to its free-JSON bindings field for them).
func bindingName(connectorExpr string) (name string, ok bool) {
	match := bindingNamePattern.FindStringSubmatch(connectorExpr)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// bindingConnectorType infers the connector type a step's action requires
// by finding the installed plugin that contributes uses (its Actions
// declared at handshake time) and returning its declared connector type. It
// returns ok=false when no installed plugin contributes uses, or when that
// plugin declares zero or more than one connector type — this project's
// connector-consuming plugins (openai, http, postgresql, mysql) each
// declare exactly one today, so the ambiguous case is left unresolved
// rather than guessed at.
func bindingConnectorType(uses string, catalog []plugins.CatalogEntry) (connectorType string, ok bool) {
	for _, entry := range catalog {
		for _, action := range entry.Actions {
			if action.ID != uses {
				continue
			}
			if len(entry.Connectors) != 1 {
				return "", false
			}
			return entry.Connectors[0].Type, true
		}
	}
	return "", false
}

// workflowInputDetail is the JSON shape of one declared input within a
// workflowDetail — enough for a client (the dashboard's Run dialog) to
// render a typed form field instead of a raw JSON blob.
type workflowInputDetail struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Description string   `json:"description,omitempty"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// workflowDetail is the JSON shape of one workflow version's full
// definition, as returned by GET /v1/workflows/{id}. Unlike workflowSummary
// (id, version, installed_at only — enough to list every installed
// version), this carries the parsed steps a client needs to render the
// workflow's structure, plus its raw YAML source for a "view source"
// affordance — the same source `patchcord workflow export` prints.
type workflowDetail struct {
	ID            string `json:"id"`
	Version       int    `json:"version"`
	SchemaVersion int    `json:"schema_version"`
	TriggerType   string `json:"trigger_type"`
	// TriggerCron and TriggerOnMissed are only set when TriggerType is
	// "schedule" — see internal/workflow.Trigger (ADR-0035).
	TriggerCron     string `json:"trigger_cron,omitempty"`
	TriggerOnMissed string `json:"trigger_on_missed,omitempty"`
	// NextRunAt is this workflow's next scheduled firing, populated from
	// the schedules table (internal/scheduler) rather than parsed from the
	// definition — nil for a "manual" trigger, or if the schedules row
	// hasn't caught up yet with a just-installed version.
	NextRunAt *time.Time            `json:"next_run_at,omitempty"`
	Inputs    []workflowInputDetail `json:"inputs,omitempty"`
	Steps     []workflowStepDetail  `json:"steps"`
	// Bindings is every distinct connector binding name referenced by Steps
	// (via a "${{ bindings.<name> }}" expression), each paired with its
	// inferred connector type when one could be — see bindingConnectorType.
	Bindings []workflowBindingDetail `json:"bindings,omitempty"`
	Source   string                  `json:"source"`
}

// effectiveOnMissed returns t.OnMissed, resolving the default a Trigger
// leaves implicit (an empty OnMissed means scheduler.OnMissedSkip — see
// internal/workflow.Trigger and scheduler.Sync) so a client always sees the
// policy that actually governs the schedule, not the raw possibly-empty
// YAML field.
func effectiveOnMissed(t workflow.Trigger) string {
	if t.Type != "schedule" {
		return ""
	}
	if t.OnMissed == "" {
		return scheduler.OnMissedSkip
	}
	return t.OnMissed
}

// toWorkflowDetail builds a workflowDetail from a parsed workflow
// definition. catalog is every installed plugin, used to infer each
// binding's connector type (bindingConnectorType) — pass nil when that
// inference isn't needed (every step's Connector/ConnectorType/BindingName
// then comes back empty, same as before this field existed). nextRunAt is
// looked up separately (scheduler.NextRunAt) rather than derived from def,
// since it depends on the schedules table's live state, not the
// definition alone — pass nil when it isn't needed (e.g. handleListWorkflows
// doesn't call toWorkflowDetail at all today).
func toWorkflowDetail(def *workflow.Definition, source string, catalog []plugins.CatalogEntry, nextRunAt *time.Time) workflowDetail {
	inputs := make([]workflowInputDetail, 0, len(def.Inputs))
	for _, input := range def.Inputs {
		inputs = append(inputs, workflowInputDetail{
			Name:        input.Name,
			Type:        input.Type,
			Required:    input.Required,
			Description: input.Description,
			Default:     input.Default,
			Enum:        input.Enum,
		})
	}

	steps := make([]workflowStepDetail, 0, len(def.Steps))
	// bindingTypes tracks, per distinct binding name, the connector type
	// found so far and whether every step referencing it has agreed —
	// preserving first-seen order so the response is deterministic.
	var bindingOrder []string
	bindingTypes := make(map[string]string)
	bindingConflict := make(map[string]bool)

	for _, step := range def.Steps {
		detail := workflowStepDetail{
			ID:        step.ID,
			Uses:      step.Uses,
			With:      step.With,
			Connector: step.Connector,
		}

		if name, ok := bindingName(step.Connector); ok {
			detail.BindingName = name
			if connectorType, ok := bindingConnectorType(step.Uses, catalog); ok {
				detail.ConnectorType = connectorType

				if existing, seen := bindingTypes[name]; !seen {
					bindingOrder = append(bindingOrder, name)
					bindingTypes[name] = connectorType
				} else if existing != connectorType {
					bindingConflict[name] = true
				}
			} else if _, seen := bindingTypes[name]; !seen {
				bindingOrder = append(bindingOrder, name)
				bindingTypes[name] = ""
			}
		}

		steps = append(steps, detail)
	}

	bindings := make([]workflowBindingDetail, 0, len(bindingOrder))
	for _, name := range bindingOrder {
		connectorType := bindingTypes[name]
		if bindingConflict[name] {
			connectorType = ""
		}
		bindings = append(bindings, workflowBindingDetail{Name: name, ConnectorType: connectorType})
	}

	return workflowDetail{
		ID:              def.ID,
		Version:         def.Version,
		SchemaVersion:   def.SchemaVersion,
		TriggerType:     def.Trigger.Type,
		TriggerCron:     def.Trigger.Cron,
		TriggerOnMissed: effectiveOnMissed(def.Trigger),
		NextRunAt:       nextRunAt,
		Inputs:          inputs,
		Steps:           steps,
		Bindings:        bindings,
		Source:          source,
	}
}

// @Summary      List installed workflows
// @Description  Returns every installed workflow version, most recently installed first. The same workflow id can appear more than once if multiple versions are installed (workflows are immutable once published — installing a new version never replaces an old one).
// @Tags         workflows
// @Produce      json
// @Success      200  {array}  workflowSummary
// @Security     BearerAuth
// @Router       /workflows [get]
func handleListWorkflows(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versions, err := runs.ListWorkflows(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "list workflows: "+err.Error(), http.StatusInternalServerError)
			return
		}

		summaries := make([]workflowSummary, 0, len(versions))
		for _, v := range versions {
			summaries = append(summaries, workflowSummary{ID: v.WorkflowID, Version: v.Version, InstalledAt: v.InstalledAt})
		}

		writeJSON(w, http.StatusOK, summaries)
	}
}

// handleGetWorkflow returns one workflow version's full definition — its
// steps and raw YAML source — unlike handleListWorkflows, which only ever
// returns enough to list every installed version (id, version,
// installed_at). version defaults to the latest installed one, matching
// runs.WorkflowSource and `patchcord workflow export --version`.
// @Summary      Get a workflow version's definition
// @Description  Returns one workflow version's full definition: its steps and raw YAML source. Defaults to the latest installed version.
// @Tags         workflows
// @Produce      json
// @Param        id       path   string  true   "Workflow id"
// @Param        version  query  int     false  "Workflow version (defaults to the latest installed one)"
// @Success      200  {object}  workflowDetail
// @Failure      400  {string}  string  "version is not a valid integer"
// @Failure      404  {string}  string  "workflow not found"
// @Security     BearerAuth
// @Router       /workflows/{id} [get]
func handleGetWorkflow(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		version := 0
		if raw := r.URL.Query().Get("version"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				http.Error(w, "version must be an integer", http.StatusBadRequest)
				return
			}
			version = parsed
		}

		source, err := runs.WorkflowSource(r.Context(), deps.DB, id, version)
		if errors.Is(err, runs.ErrWorkflowNotFound) {
			http.Error(w, fmt.Sprintf("workflow %q was not found", id), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "get workflow: "+err.Error(), http.StatusInternalServerError)
			return
		}

		def, err := workflow.Parse([]byte(source))
		if err != nil {
			http.Error(w, "get workflow: "+err.Error(), http.StatusInternalServerError)
			return
		}

		catalog, err := plugins.List(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "get workflow: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// The schedules table is keyed by workflow_id, not (workflow_id,
		// version) — it always reflects the currently installed trigger
		// going forward, so this is populated even when an older version is
		// being viewed (?version=N).
		var nextRunAt *time.Time
		if def.Trigger.Type == "schedule" {
			if t, ok, err := scheduler.NextRunAt(r.Context(), deps.DB, id); err != nil {
				http.Error(w, "get workflow: "+err.Error(), http.StatusInternalServerError)
				return
			} else if ok {
				nextRunAt = &t
			}
		}

		writeJSON(w, http.StatusOK, toWorkflowDetail(def, source, catalog, nextRunAt))
	}
}

// handleRunWorkflow starts a new run of the latest installed version of the
// named workflow and returns immediately with the run's id and initial
// status, without waiting for any step to execute (vision document, section
// 10.2's `client.workflows.run(...)`) — a client watches
// GET /v1/runs/{id}/events or polls GET /v1/runs/{id} for progress and the
// final result.
//
// It calls runs.Start synchronously (fast: only the run's creation is
// persisted), then runs.Continue in a background goroutine bound to
// deps.RunCtx, not this request's context — the run must keep going after
// this handler has already responded and the request is gone. A failure
// inside that background Continue call has no HTTP request left to report
// to, so it is only logged.
// @Summary      Trigger a workflow run
// @Description  Starts a new run of the latest installed version of the named workflow and returns immediately with its id and initial status — it does not wait for any step to execute. Watch GET /runs/{id}/events or poll GET /runs/{id} for progress and the final result.
// @Tags         workflows
// @Accept       json
// @Produce      json
// @Param        id    path  string               true  "Workflow id"
// @Param        body  body  runWorkflowRequest  false  "Inputs and connector bindings (both optional, default to none)"
// @Success      202  {object}  runSummary
// @Failure      400  {string}  string  "malformed request body, or inputs don't satisfy the workflow's declared schema"
// @Failure      404  {string}  string  "workflow not found"
// @Failure      500  {string}  string  "no action executor configured, or a persistence failure"
// @Security     BearerAuth
// @Router       /workflows/{id}/run [post]
func handleRunWorkflow(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workflowID := r.PathValue("id")

		var body runWorkflowRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "decode request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		startRunAndRespond(w, r, deps, workflowID, body.Inputs, body.Bindings)
	}
}

// startRunAndRespond starts a new run of workflowID's latest installed
// version with inputs/bindings, continues it in a background goroutine
// bound to deps.RunCtx (so it keeps going after this handler has already
// responded and the request is gone — see handleRunWorkflow's doc
// comment), and responds 202 with the run's initial status. Shared by
// handleRunWorkflow and handleWebhookTrigger (webhooks.go, ADR-0037), which
// differ only in how they authenticate the caller and where inputs/bindings
// come from.
//
// If r's context carries an app session (withRunAuth stashed one via
// withSession — ADR-0071), the run is restricted to that session's
// Permissions.ConnectorsUse: any step that resolves to a connector outside
// that list fails with runs.ErrConnectorNotPermitted. handleWebhookTrigger
// never goes through withRunAuth, so it always runs unrestricted here —
// with no consequence, since a webhook-triggered workflow can never bind a
// connector in the first place (internal/workflow/compile.go's
// validateNoConnectorBoundStep).
func startRunAndRespond(w http.ResponseWriter, r *http.Request, deps Deps, workflowID string, inputs map[string]any, bindings map[string]string) {
	if deps.Executor == nil {
		http.Error(w, "run workflow: no action executor configured", http.StatusInternalServerError)
		return
	}

	def, run, preparedInputs, err := runs.Start(r.Context(), deps.DB, workflowID, inputs, deps.metrics())
	if errors.Is(err, runs.ErrWorkflowNotFound) {
		http.Error(w, fmt.Sprintf("workflow %q was not found", workflowID), http.StatusNotFound)
		return
	}
	if errors.Is(err, workflow.ErrInvalidInputs) {
		http.Error(w, "run workflow: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "run workflow: "+err.Error(), http.StatusInternalServerError)
		return
	}

	opts := runs.ExecuteOptions{Secrets: deps.secrets(), Metrics: deps.metrics()}
	if session, ok := sessionFromContext(r.Context()); ok {
		opts.AllowedConnectors = &session.Permissions.ConnectorsUse
	}

	go func() {
		if err := runs.Continue(deps.runCtx(), deps.DB, deps.Executor, def, run, preparedInputs, bindings, opts); err != nil {
			deps.logger().Error("continue run",
				slog.String("run_id", run.ID),
				slog.String("workflow_id", workflowID),
				slog.String("error", err.Error()))
		}
	}()

	writeJSON(w, http.StatusAccepted, toRunSummary(run, nil))
}
