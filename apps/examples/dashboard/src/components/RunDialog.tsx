import type { PatchcordClient, Run, WorkflowDetail, WorkflowInputDetail } from "@patchcord/sdk";
import Alert from "@mui/material/Alert";
import Button from "@mui/material/Button";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import FormControlLabel from "@mui/material/FormControlLabel";
import MenuItem from "@mui/material/MenuItem";
import Stack from "@mui/material/Stack";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import { useMemo, useRef, useState } from "react";

import StatusChip from "./StatusChip";
import StepTimeline, { type DisplayStep } from "./StepTimeline";

type Phase = "form" | "running" | "done" | "error";

function parseJSONRecord(raw: string, fieldName: string): Record<string, unknown> {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return {};
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch (err) {
    throw new Error(`${fieldName} is not valid JSON: ${(err as Error).message}`);
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error(`${fieldName} must be a JSON object`);
  }
  return parsed as Record<string, unknown>;
}

// A boolean input always has a value (its switch is on or off), so it's
// stored as-is; every other declared type is edited as text (a select for
// "enum") and only converted to its declared type in buildTypedInputs, so a
// required-but-empty string/number/enum field can be told apart from "0" or
// "false".
type TypedInputValue = string | boolean;

function initialTypedInputs(inputs: WorkflowInputDetail[]): Record<string, TypedInputValue> {
  const values: Record<string, TypedInputValue> = {};
  for (const input of inputs) {
    if (input.type === "boolean") {
      values[input.name] = typeof input.default === "boolean" ? input.default : false;
    } else {
      values[input.name] = input.default !== undefined ? String(input.default) : "";
    }
  }
  return values;
}

// Converts the form's string/boolean state back into the typed record
// client.workflows.run expects, validating required fields along the way.
// Coercion mirrors internal/workflow.PrepareInputs (number/boolean parsing)
// since this is the same declared schema, just enforced client-side first
// for a fast, in-form error instead of a round trip to get a 400.
function buildTypedInputs(inputs: WorkflowInputDetail[], values: Record<string, TypedInputValue>): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const input of inputs) {
    const raw = values[input.name];

    if (input.type === "boolean") {
      result[input.name] = raw;
      continue;
    }

    const text = typeof raw === "string" ? raw.trim() : "";
    if (text === "") {
      if (input.required) {
        throw new Error(`"${input.name}" is required`);
      }
      continue;
    }

    if (input.type === "number") {
      const n = Number(text);
      if (Number.isNaN(n)) {
        throw new Error(`"${input.name}" must be a number`);
      }
      result[input.name] = n;
    } else {
      result[input.name] = text;
    }
  }
  return result;
}

export default function RunDialog({
  client,
  workflow,
  open,
  onClose,
}: {
  client: PatchcordClient;
  workflow: WorkflowDetail;
  open: boolean;
  onClose: () => void;
}) {
  const declaresInputs = workflow.inputs.length > 0;

  const [phase, setPhase] = useState<Phase>("form");
  const [typedInputs, setTypedInputs] = useState<Record<string, TypedInputValue>>(() => initialTypedInputs(workflow.inputs));
  const [inputsText, setInputsText] = useState("{}");
  const [bindingsText, setBindingsText] = useState("{}");
  const [formError, setFormError] = useState<string | undefined>();
  const [steps, setSteps] = useState<DisplayStep[]>([]);
  const [runId, setRunId] = useState<string | undefined>();
  const [runStatus, setRunStatus] = useState<string>("queued");
  const [runError, setRunError] = useState<string | undefined>();
  const runRef = useRef<Run | undefined>(undefined);

  // Recomputed whenever the dialog is reopened for a possibly different
  // workflow (or version) than last time — see reset().
  const freshTypedInputs = useMemo(() => initialTypedInputs(workflow.inputs), [workflow]);

  function reset() {
    setPhase("form");
    setTypedInputs(freshTypedInputs);
    setInputsText("{}");
    setFormError(undefined);
    setSteps([]);
    setRunId(undefined);
    setRunStatus("queued");
    setRunError(undefined);
    runRef.current = undefined;
  }

  function handleClose() {
    onClose();
    // Deferred so the dialog's own close transition doesn't visibly flash
    // back to the form while it's animating out.
    setTimeout(reset, 200);
  }

  async function start() {
    setFormError(undefined);

    let inputs: Record<string, unknown>;
    let bindings: Record<string, unknown>;
    try {
      inputs = declaresInputs ? buildTypedInputs(workflow.inputs, typedInputs) : parseJSONRecord(inputsText, "Inputs");
      bindings = parseJSONRecord(bindingsText, "Bindings");
    } catch (err) {
      setFormError((err as Error).message);
      return;
    }

    setPhase("running");
    setSteps(workflow.steps.map((s) => ({ id: s.id, uses: s.uses, status: "pending" })));
    setRunStatus("queued");

    try {
      const run = await client.workflows.run(workflow.id, { inputs, bindings: bindings as Record<string, string> });
      runRef.current = run;
      setRunId(run.id);

      for await (const event of run.events()) {
        if (event.stepId) {
          setSteps((prev) =>
            prev.map((s) => (s.id === event.stepId ? { ...s, status: event.status, error: event.error } : s)),
          );
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
      // terminal status either way, so a failed cancel attempt here is not
      // worth surfacing as its own error.
    }
  }

  const running = phase === "running";

  return (
    <Dialog open={open} onClose={running ? undefined : handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        Run <code>{workflow.id}</code>
        <Typography variant="caption" color="text.secondary" display="block">
          version {workflow.version}
        </Typography>
      </DialogTitle>
      <DialogContent dividers>
        {phase === "form" && (
          <Stack spacing={2} sx={{ mt: 1 }}>
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

            <TextField
              label="Bindings (JSON, connector-id per logical name)"
              multiline
              minRows={2}
              value={bindingsText}
              onChange={(e) => setBindingsText(e.target.value)}
              slotProps={{ input: { sx: { fontFamily: "ui-monospace, monospace", fontSize: 13 } } }}
            />
          </Stack>
        )}

        {phase !== "form" && (
          <Stack spacing={2} sx={{ mt: 1 }}>
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
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        {phase === "form" && (
          <>
            <Button onClick={handleClose}>Cancel</Button>
            <Button variant="contained" onClick={() => void start()}>
              Run
            </Button>
          </>
        )}
        {running && (
          <>
            <Button onClick={() => void cancel()} color="warning">
              Cancel run
            </Button>
            <Button disabled>Running…</Button>
          </>
        )}
        {(phase === "done" || phase === "error") && (
          <>
            <Button onClick={reset}>Run again</Button>
            <Button variant="contained" onClick={handleClose}>
              Close
            </Button>
          </>
        )}
      </DialogActions>
    </Dialog>
  );
}

// One form control per declared input's type: a switch for "boolean", a
// select for "enum", a (possibly numeric) text field otherwise.
function WorkflowInputField({
  input,
  value,
  onChange,
}: {
  input: WorkflowInputDetail;
  value: TypedInputValue;
  onChange: (value: TypedInputValue) => void;
}) {
  const helperText = input.description;

  if (input.type === "boolean") {
    return (
      <FormControlLabel
        control={<Switch checked={Boolean(value)} onChange={(e) => onChange(e.target.checked)} />}
        label={
          <Stack>
            <Typography variant="body2">{input.name}</Typography>
            {helperText && (
              <Typography variant="caption" color="text.secondary">
                {helperText}
              </Typography>
            )}
          </Stack>
        }
      />
    );
  }

  if (input.type === "enum") {
    return (
      <TextField
        select
        label={input.name}
        required={input.required}
        helperText={helperText}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {(input.enum ?? []).map((option) => (
          <MenuItem key={option} value={option}>
            {option}
          </MenuItem>
        ))}
      </TextField>
    );
  }

  return (
    <TextField
      label={input.name}
      type={input.type === "number" ? "number" : "text"}
      required={input.required}
      helperText={helperText}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}
