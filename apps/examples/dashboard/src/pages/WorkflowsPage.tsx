import type { PatchcordClient, WorkflowSummary } from "@glmtsolutions/patchcord-sdk";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import RefreshIcon from "@mui/icons-material/Refresh";
import Alert from "@mui/material/Alert";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
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
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { PageFade } from "../motion";

interface WorkflowGroup {
  id: string;
  latest: WorkflowSummary;
  versions: WorkflowSummary[];
}

export default function WorkflowsPage({ client }: { client: PatchcordClient }) {
  const [summaries, setSummaries] = useState<WorkflowSummary[] | undefined>();
  const [error, setError] = useState<string | undefined>();
  const navigate = useNavigate();

  function load() {
    setError(undefined);
    client.workflows
      .list()
      .then(setSummaries)
      .catch((err: Error) => setError(err.message));
  }

  useEffect(load, [client]);

  const groups = useMemo<WorkflowGroup[]>(() => {
    if (!summaries) return [];
    const byId = new Map<string, WorkflowSummary[]>();
    for (const s of summaries) {
      const list = byId.get(s.id) ?? [];
      list.push(s);
      byId.set(s.id, list);
    }
    return [...byId.entries()]
      .map(([id, versions]) => {
        const sorted = [...versions].sort((a, b) => b.version - a.version);
        return { id, latest: sorted[0], versions: sorted };
      })
      .sort((a, b) => a.id.localeCompare(b.id));
  }, [summaries]);

  return (
    <PageFade>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.5 }}>
        <Typography variant="h6">Installed workflows</Typography>
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

      {!summaries && !error && (
        <Stack alignItems="center" sx={{ py: 6 }}>
          <CircularProgress size={28} />
        </Stack>
      )}

      {summaries && groups.length === 0 && !error && (
        <Typography color="text.secondary">
          No workflow installed yet — try <code>patchcord workflow install workflows/examples/hello_patchcord.yaml</code>.
        </Typography>
      )}

      {groups.length > 0 && (
        <Paper variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Workflow</TableCell>
                <TableCell>Latest version</TableCell>
                <TableCell>Installed at</TableCell>
                <TableCell align="right" />
              </TableRow>
            </TableHead>
            <TableBody>
              {groups.map((group) => (
                <TableRow key={group.id} hover sx={{ cursor: "pointer" }} onClick={() => navigate(`/workflows/${group.id}`)}>
                  <TableCell>
                    <Typography fontFamily="ui-monospace, monospace" fontWeight={600}>
                      {group.id}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Chip label={`v${group.latest.version}`} size="small" />
                      {group.versions.length > 1 && (
                        <Typography variant="caption" color="text.secondary">
                          {group.versions.length} versions
                        </Typography>
                      )}
                    </Stack>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" color="text.secondary">
                      {new Date(group.latest.installedAt).toLocaleString()}
                    </Typography>
                  </TableCell>
                  <TableCell align="right">
                    <ChevronRightIcon fontSize="small" color="action" />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Paper>
      )}
    </PageFade>
  );
}
