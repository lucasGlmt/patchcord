import type { PatchcordClient, Run, RunSummary } from "@glmtsolutions/patchcord-sdk";
import RefreshIcon from "@mui/icons-material/Refresh";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import CircularProgress from "@mui/material/CircularProgress";
import Collapse from "@mui/material/Collapse";
import IconButton from "@mui/material/IconButton";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import { useEffect, useState } from "react";

import StatusChip from "../components/StatusChip";
import StepTimeline, { type DisplayStep } from "../components/StepTimeline";
import { PageFade } from "../motion";

function durationLabel(run: RunSummary): string {
  if (!run.startedAt) return "—";
  const end = run.finishedAt ? new Date(run.finishedAt) : new Date();
  const ms = end.getTime() - new Date(run.startedAt).getTime();
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export default function RunsPage({ client }: { client: PatchcordClient }) {
  const [runs, setRuns] = useState<Run[] | undefined>();
  const [error, setError] = useState<string | undefined>();
  const [expandedId, setExpandedId] = useState<string | undefined>();
  const [expandedSteps, setExpandedSteps] = useState<DisplayStep[]>([]);

  function load() {
    setError(undefined);
    client.runs
      .list()
      .then(setRuns)
      .catch((err: Error) => setError(err.message));
  }

  useEffect(load, [client]);

  async function toggleExpand(run: Run) {
    if (expandedId === run.id) {
      setExpandedId(undefined);
      return;
    }
    setExpandedId(run.id);
    setExpandedSteps([]);
    const full = await run.fetch();
    setExpandedSteps(
      (full.steps ?? []).map((s) => ({
        id: s.id,
        uses: "",
        status: s.status,
        input: s.input,
        output: s.output,
        error: s.error,
      })),
    );
  }

  return (
    <PageFade>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.5 }}>
        <Typography variant="h6">Runs</Typography>
        <Tooltip title="Refresh">
          <IconButton size="small" onClick={load}>
            <RefreshIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      </Stack>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {!runs && !error && (
        <Stack alignItems="center" sx={{ py: 6 }}>
          <CircularProgress size={28} />
        </Stack>
      )}

      {runs && runs.length === 0 && !error && (
        <Typography color="text.secondary">No run recorded yet — run a workflow to see it appear here.</Typography>
      )}

      {runs && runs.length > 0 && (
        <Paper variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Workflow</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Created</TableCell>
                <TableCell>Duration</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {runs.map((run) => (
                <RunRow key={run.id} run={run} expanded={expandedId === run.id} steps={expandedSteps} onToggle={() => void toggleExpand(run)} />
              ))}
            </TableBody>
          </Table>
        </Paper>
      )}
    </PageFade>
  );
}

function RunRow({
  run,
  expanded,
  steps,
  onToggle,
}: {
  run: Run;
  expanded: boolean;
  steps: DisplayStep[];
  onToggle: () => void;
}) {
  return (
    <>
      <TableRow hover sx={{ cursor: "pointer" }} onClick={onToggle}>
        <TableCell>
          <Stack direction="row" spacing={1} alignItems="center">
            <Typography fontFamily="ui-monospace, monospace" fontWeight={600}>
              {run.workflowId}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              v{run.workflowVersion}
            </Typography>
          </Stack>
        </TableCell>
        <TableCell>
          <StatusChip status={run.status} />
        </TableCell>
        <TableCell>
          <Typography variant="body2" color="text.secondary">
            {new Date(run.createdAt).toLocaleString()}
          </Typography>
        </TableCell>
        <TableCell>
          <Typography variant="body2" color="text.secondary">
            {durationLabel(run)}
          </Typography>
        </TableCell>
      </TableRow>
      <TableRow>
        <TableCell colSpan={4} sx={{ py: 0, border: expanded ? undefined : "none" }}>
          <Collapse in={expanded} unmountOnExit>
            <Box sx={{ py: 2 }}>
              {steps.length === 0 ? <CircularProgress size={20} /> : <StepTimeline steps={steps} />}
            </Box>
          </Collapse>
        </TableCell>
      </TableRow>
    </>
  );
}
