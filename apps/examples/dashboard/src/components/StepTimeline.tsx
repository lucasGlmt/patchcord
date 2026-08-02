import Step from "@mui/material/Step";
import StepContent from "@mui/material/StepContent";
import StepLabel from "@mui/material/StepLabel";
import Stepper from "@mui/material/Stepper";
import Typography from "@mui/material/Typography";

import JsonBlock from "./JsonBlock";
import StatusChip from "./StatusChip";

// A step as this timeline displays it: `uses` comes from the workflow's
// static definition (known before the run even starts), the rest tracks
// what's known about this particular run of it so far. `input`/`output`
// only ever arrive once the run's final GET /v1/runs/{id} is fetched —
// GET /v1/runs/{id}/events only ever carries a status string (see
// docs/book/src/workflows/events.md) — so both stay undefined while a run
// is still in flight.
export interface DisplayStep {
  id: string;
  uses: string;
  status: string;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  error?: string;
}

export default function StepTimeline({ steps }: { steps: DisplayStep[] }) {
  const activeIndex = steps.findIndex((s) => s.status === "running");
  const allDone = steps.every((s) => s.status !== "pending" && s.status !== "running");

  return (
    <Stepper activeStep={activeIndex === -1 ? (allDone ? steps.length : 0) : activeIndex} orientation="vertical" nonLinear>
      {steps.map((step) => (
        <Step key={step.id} completed={step.status === "succeeded"}>
          <StepLabel error={step.status === "failed"} optional={<StatusChip status={step.status} />}>
            <Typography variant="body2" component="span" fontWeight={600} fontFamily="ui-monospace, monospace">
              {step.id}
            </Typography>
            <Typography variant="caption" color="text.secondary" component="span" sx={{ ml: 1 }}>
              {step.uses}
            </Typography>
          </StepLabel>
          <StepContent TransitionProps={{ in: true }}>
            {step.input && Object.keys(step.input).length > 0 && <JsonBlock label="Input" value={step.input} />}
            {step.output && Object.keys(step.output).length > 0 && <JsonBlock label="Output" value={step.output} />}
            {step.error && <JsonBlock label="Error" value={step.error} />}
          </StepContent>
        </Step>
      ))}
    </Stepper>
  );
}
