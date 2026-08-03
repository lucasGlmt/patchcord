import type { Connector, PatchcordClient, Run, WorkflowDetail, WorkflowSummary } from "@patchcord/sdk";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import PlayArrowIcon from "@mui/icons-material/PlayArrow";
import Accordion from "@mui/material/Accordion";
import AccordionDetails from "@mui/material/AccordionDetails";
import AccordionSummary from "@mui/material/AccordionSummary";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import IconButton from "@mui/material/IconButton";
import Link from "@mui/material/Link";
import MenuItem from "@mui/material/MenuItem";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link as RouterLink, useNavigate, useParams, useSearchParams } from "react-router-dom";

import JsonBlock from "../components/JsonBlock";
import StatusChip from "../components/StatusChip";
import StepTimeline, { type DisplayStep } from "../components/StepTimeline";
import WorkflowInputField from "../components/WorkflowInputField";
import { actionIcon } from "../lib/actionIcons";
import { buildTypedInputs, initialTypedInputs, parseJSONRecord, type TypedInputValue } from "../lib/workflowInputs";
import { CrossFade, PageFade, StaggerItem, StaggerList } from "../motion";

type Phase = "form" | "running" | "done" | "error";

export default function WorkflowDetailPage({ client }: { client: PatchcordClient }) {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [versions, setVersions] = useState<WorkflowSummary[] | undefined>();
  const [detail, setDetail] = useState<WorkflowDetail | undefined>();
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [loadError, setLoadError] = useState<string | undefined>();

  const versionParam = searchParams.get("version");
  const selectedVersion = versionParam ? Number(versionParam) : versions?.[0]?.version;

  useEffect(() => {
    let cancelled = false;
    client.workflows
      .list()
      .then((all) => {
        if (cancelled) return;
        setVersions(all.filter((w) => w.id === id).sort((a, b) => b.version - a.version));
      })
      .catch((err: Error) => !cancelled && setLoadError(err.message));
    return () => {
      cancelled = true;
    };
  }, [client, id]);

  useEffect(() => {
    let cancelled = false;
    setDetail(undefined);
    setLoadError(undefined);
    client.workflows
      .get(id, selectedVersion ? { version: selectedVersion } : undefined)
      .then((d) => !cancelled && setDetail(d))
      .catch((err: Error) => !cancelled && setLoadError(err.message));
    return () => {
      cancelled = true;
    };
  }, [client, id, selectedVersion]);

  useEffect(() => {
    client.connectors
      .list()
      .then(setConnectors)
      .catch(() => setConnectors([]));
  }, [client]);

  return (
    <PageFade>
      <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 2 }}>
        <IconButton size="small" onClick={() => navigate("/workflows")}>
          <ArrowBackIcon fontSize="small" />
        </IconButton>
        <Typography variant="h6" fontFamily="ui-monospace, monospace" fontWeight={700}>
          {id}
        </Typography>
        {versions && versions.length > 1 ? (
          <TextField
            select
            size="small"
            value={selectedVersion ?? ""}
            onChange={(e) => setSearchParams({ version: e.target.value })}
            sx={{ width: 110, ml: 1 }}
          >
            {versions.map((v) => (
              <MenuItem key={v.version} value={v.version}>
                v{v.version}
              </MenuItem>
            ))}
          </TextField>
        ) : (
          versions && versions[0] && <Chip label={`v${versions[0].version}`} size="small" sx={{ ml: 1 }} />
        )}
      </Stack>

      {loadError && <Alert severity="error">{loadError}</Alert>}

      {!detail && !loadError && (
        <Stack alignItems="center" sx={{ py: 6 }}>
          <CircularProgress size={28} />
        </Stack>
      )}

      {detail && (
        <Box sx={{ display: "flex", gap: 3, flexWrap: "wrap", alignItems: "flex-start" }}>
          <Box sx={{ flex: "1 1 520px", minWidth: 0 }}>
            <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
              <Chip label={`trigger: ${detail.triggerType}`} size="small" variant="outlined" />
              <Chip label={`schema v${detail.schemaVersion}`} size="small" variant="outlined" />
              <Chip label={`${detail.steps.length} step${detail.steps.length === 1 ? "" : "s"}`} size="small" variant="outlined" />
            </Stack>

            <StaggerList>
              <Stack spacing={1}>
                {detail.steps.map((step, i) => (
                  <StaggerItem key={step.id}>
                    <Paper variant="outlined" sx={{ p: 1.5 }}>
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Typography variant="caption" color="text.secondary">
                          {i + 1}.
                        </Typography>
                        {actionIcon(step.uses)}
                        <Typography variant="body2" fontWeight={600} fontFamily="ui-monospace, monospace">
                          {step.id}
                        </Typography>
                        <Chip label={step.uses} size="small" variant="outlined" />
                        {step.bindingName && (
                          <Chip label={`binding: ${step.bindingName}`} size="small" color="secondary" variant="outlined" />
                        )}
                      </Stack>
                      {step.with && Object.keys(step.with).length > 0 && <JsonBlock label="with" value={step.with} />}
                    </Paper>
                  </StaggerItem>
                ))}
              </Stack>
            </StaggerList>

            <Accordion disableGutters variant="outlined" sx={{ mt: 2 }}>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography variant="body2">View YAML source</Typography>
              </AccordionSummary>
              <AccordionDetails>
                <JsonBlock value={detail.source} />
              </AccordionDetails>
            </Accordion>
          </Box>

          <Box sx={{ flex: "1 1 360px", minWidth: 320, position: "sticky", top: 16 }}>
            <Paper variant="outlined" sx={{ p: 2 }}>
              <RunPanel client={client} workflow={detail} connectors={connectors} />
            </Paper>
          </Box>
        </Box>
      )}
    </PageFade>
  );
}

function RunPanel({
  client,
  workflow,
  connectors,
}: {
  client: PatchcordClient;
  workflow: WorkflowDetail;
  connectors: Connector[];
}) {
  const declaresInputs = workflow.inputs.length > 0;
  // Steps whose connector expression didn't reduce to a simple
  // "${{ bindings.<name> }}" shape (e.g. an expression over workflow.inputs
  // or steps.*.outputs) have no entry in workflow.bindings — there's no
  // static "which connector" a <select> could offer them ahead of a run, so
  // they fall back to a single advanced JSON field instead.
  const hasAdvancedBindings = workflow.steps.some((s) => s.connector && !s.bindingName);

  const [phase, setPhase] = useState<Phase>("form");
  const [typedInputs, setTypedInputs] = useState<Record<string, TypedInputValue>>(() => initialTypedInputs(workflow.inputs));
  const [inputsText, setInputsText] = useState("{}");
  const [bindingSelections, setBindingSelections] = useState<Record<string, string>>({});
  const [advancedBindingsText, setAdvancedBindingsText] = useState("{}");
  const [formError, setFormError] = useState<string | undefined>();
  const [steps, setSteps] = useState<DisplayStep[]>([]);
  const [runId, setRunId] = useState<string | undefined>();
  const [runStatus, setRunStatus] = useState<string>("queued");
  const [runError, setRunError] = useState<string | undefined>();
  const runRef = useRef<Run | undefined>(undefined);

  const freshTypedInputs = useMemo(() => initialTypedInputs(workflow.inputs), [workflow]);

  function reset() {
    setPhase("form");
    setTypedInputs(freshTypedInputs);
    setInputsText("{}");
    setBindingSelections({});
    setAdvancedBindingsText("{}");
    setFormError(undefined);
    setSteps([]);
    setRunId(undefined);
    setRunStatus("queued");
    setRunError(undefined);
    runRef.current = undefined;
  }

  useEffect(reset, [workflow]);

  async function start() {
    setFormError(undefined);

    let inputs: Record<string, unknown>;
    let bindings: Record<string, string>;
    try {
      inputs = declaresInputs ? buildTypedInputs(workflow.inputs, typedInputs) : parseJSONRecord(inputsText, "Inputs");
      const advanced = parseJSONRecord(advancedBindingsText, "Additional bindings") as Record<string, string>;
      bindings = { ...bindingSelections, ...advanced };
    } catch (err) {
      setFormError((err as Error).message);
      return;
    }

    setPhase("running");
    setSteps(workflow.steps.map((s) => ({ id: s.id, uses: s.uses, status: "pending" })));
    setRunStatus("queued");

    try {
      const run = await client.workflows.run(workflow.id, { inputs, bindings });
      runRef.current = run;
      setRunId(run.id);

      for await (const event of run.events()) {
        if (event.stepId) {
          setSteps((prev) => prev.map((s) => (s.id === event.stepId ? { ...s, status: event.status, error: event.error } : s)));
        } else {
          setRunStatus(event.status);
        }
      }

      const final = await run.fetch();
      setRunStatus(final.status);
      setRunError(final.error);
      if (final.steps) {
        setSteps(
          final.steps.map((s) => ({
            id: s.id,
            uses: workflow.steps.find((ws) => ws.id === s.id)?.uses ?? "",
            status: s.status,
            input: s.input,
            output: s.output,
            error: s.error,
          })),
        );
      }
      setPhase("done");
    } catch (err) {
      setPhase("error");
      setRunError((err as Error).message);
    }
  }

  async function cancel() {
    try {
      await runRef.current?.cancel();
    } catch {
      // The run may already have finished by the time Cancel is clicked —
      // the next event (or the final fetch below) reflects its real
      // terminal status either way.
    }
  }

  const running = phase === "running";

  return (
    <Stack spacing={2}>
      <Typography variant="subtitle1" fontWeight={700}>
        Run this workflow
      </Typography>

      <CrossFade id={phase}>
        {phase === "form" ? (
          <Stack spacing={2}>
            {formError && <Alert severity="error">{formError}</Alert>}

            {declaresInputs
              ? workflow.inputs.map((input) => (
                  <WorkflowInputField
                    key={input.name}
                    input={input}
                    value={typedInputs[input.name]}
                    onChange={(value) => setTypedInputs((prev) => ({ ...prev, [input.name]: value }))}
                  />
                ))
              : (
                <TextField
                  label="Inputs (JSON)"
                  multiline
                  minRows={3}
                  value={inputsText}
                  onChange={(e) => setInputsText(e.target.value)}
                  slotProps={{ input: { sx: { fontFamily: "ui-monospace, monospace", fontSize: 13 } } }}
                />
              )}

            {workflow.bindings.length > 0 && (
              <Stack spacing={1.5}>
                <Typography variant="caption" color="text.secondary">
                  Connector bindings
                </Typography>
                {workflow.bindings.map((binding) => {
                  const matching = binding.connectorType
                    ? connectors.filter((c) => c.type === binding.connectorType)
                    : connectors;
                  return (
                    <TextField
                      key={binding.name}
                      select
                      label={binding.name}
                      helperText={
                        binding.connectorType && matching.length === 0
                          ? `No connector of type ${binding.connectorType} yet`
                          : binding.connectorType
                      }
                      value={bindingSelections[binding.name] ?? ""}
                      onChange={(e) => setBindingSelections((prev) => ({ ...prev, [binding.name]: e.target.value }))}
                    >
                      {matching.map((c) => (
                        <MenuItem key={c.id} value={c.id}>
                          {c.id}
                        </MenuItem>
                      ))}
                    </TextField>
                  );
                })}
                {workflow.bindings.some((b) => b.connectorType && connectors.filter((c) => c.type === b.connectorType).length === 0) && (
                  <Link component={RouterLink} to="/connectors" variant="caption">
                    Create a connector →
                  </Link>
                )}
              </Stack>
            )}

            {hasAdvancedBindings && (
              <TextField
                label="Additional bindings (JSON)"
                multiline
                minRows={2}
                value={advancedBindingsText}
                onChange={(e) => setAdvancedBindingsText(e.target.value)}
                helperText="For bindings this workflow computes dynamically — not offered as a select above."
                slotProps={{ input: { sx: { fontFamily: "ui-monospace, monospace", fontSize: 13 } } }}
              />
            )}

            <Button variant="contained" startIcon={<PlayArrowIcon />} onClick={() => void start()}>
              Run
            </Button>
          </Stack>
        ) : (
          <Stack spacing={2}>
            <Stack direction="row" spacing={1} alignItems="center">
              <StatusChip status={runStatus} />
              {runId && (
                <Typography variant="caption" color="text.secondary" fontFamily="ui-monospace, monospace">
                  {runId}
                </Typography>
              )}
            </Stack>
            {runError && <Alert severity="error">{runError}</Alert>}
            <StepTimeline steps={steps} />
            <Stack direction="row" spacing={1}>
              {running && (
                <Button onClick={() => void cancel()} color="warning" size="small">
                  Cancel run
                </Button>
              )}
              {!running && (
                <Button onClick={reset} size="small">
                  Run again
                </Button>
              )}
            </Stack>
          </Stack>
        )}
      </CrossFade>
    </Stack>
  );
}
