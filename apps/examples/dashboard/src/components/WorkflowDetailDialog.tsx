import type { PatchcordClient, WorkflowDetail, WorkflowSummary } from "@patchcord/sdk";
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
import Dialog from "@mui/material/Dialog";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import MenuItem from "@mui/material/MenuItem";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import { useEffect, useState } from "react";

import JsonBlock from "./JsonBlock";
import RunDialog from "./RunDialog";

export default function WorkflowDetailDialog({
  client,
  workflowId,
  versions,
  open,
  onClose,
}: {
  client: PatchcordClient;
  workflowId: string;
  /** Every installed version of this workflow id, most recent first. */
  versions: WorkflowSummary[];
  open: boolean;
  onClose: () => void;
}) {
  const [selectedVersion, setSelectedVersion] = useState<number | undefined>(versions[0]?.version);
  const [detail, setDetail] = useState<WorkflowDetail | undefined>();
  const [error, setError] = useState<string | undefined>();
  const [runOpen, setRunOpen] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    setSelectedVersion(versions[0]?.version);
  }, [open, versions]);

  useEffect(() => {
    if (!open || selectedVersion === undefined) {
      return;
    }
    let cancelled = false;
    setDetail(undefined);
    setError(undefined);
    client.workflows
      .get(workflowId, { version: selectedVersion })
      .then((d) => {
        if (!cancelled) setDetail(d);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });
    return () => {
      cancelled = true;
    };
  }, [client, workflowId, selectedVersion, open]);

  return (
    <>
      <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
        <DialogTitle>
          <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2}>
            <code>{workflowId}</code>
            {versions.length > 1 ? (
              <TextField
                select
                size="small"
                label="Version"
                value={selectedVersion ?? ""}
                onChange={(e) => setSelectedVersion(Number(e.target.value))}
                sx={{ width: 140 }}
              >
                {versions.map((v) => (
                  <MenuItem key={v.version} value={v.version}>
                    v{v.version}
                  </MenuItem>
                ))}
              </TextField>
            ) : (
              <Chip label={`v${versions[0]?.version}`} size="small" />
            )}
          </Stack>
        </DialogTitle>
        <DialogContent dividers>
          {error && <Alert severity="error">{error}</Alert>}
          {!detail && !error && (
            <Stack alignItems="center" sx={{ py: 4 }}>
              <CircularProgress size={28} />
            </Stack>
          )}
          {detail && (
            <Stack spacing={2}>
              <Stack direction="row" spacing={1}>
                <Chip label={`trigger: ${detail.triggerType}`} size="small" variant="outlined" />
                <Chip label={`schema v${detail.schemaVersion}`} size="small" variant="outlined" />
                <Chip label={`${detail.steps.length} step${detail.steps.length === 1 ? "" : "s"}`} size="small" variant="outlined" />
              </Stack>

              <Stack spacing={1}>
                {detail.steps.map((step, i) => (
                  <Box
                    key={step.id}
                    sx={{
                      p: 1.25,
                      borderRadius: 1,
                      border: "1px solid",
                      borderColor: "divider",
                    }}
                  >
                    <Stack direction="row" spacing={1} alignItems="baseline">
                      <Typography variant="caption" color="text.secondary">
                        {i + 1}.
                      </Typography>
                      <Typography variant="body2" fontWeight={600} fontFamily="ui-monospace, monospace">
                        {step.id}
                      </Typography>
                      <Chip label={step.uses} size="small" variant="outlined" />
                      {step.connector && <Chip label={`connector: ${step.connector}`} size="small" color="secondary" variant="outlined" />}
                    </Stack>
                    {step.with && Object.keys(step.with).length > 0 && <JsonBlock label="with" value={step.with} />}
                  </Box>
                ))}
              </Stack>

              <Accordion disableGutters variant="outlined">
                <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                  <Typography variant="body2">View YAML source</Typography>
                </AccordionSummary>
                <AccordionDetails>
                  <JsonBlock value={detail.source} />
                </AccordionDetails>
              </Accordion>

              <Button variant="contained" startIcon={<PlayArrowIcon />} onClick={() => setRunOpen(true)} sx={{ alignSelf: "flex-start" }}>
                Run this workflow
              </Button>
            </Stack>
          )}
        </DialogContent>
      </Dialog>

      {detail && <RunDialog client={client} workflow={detail} open={runOpen} onClose={() => setRunOpen(false)} />}
    </>
  );
}
