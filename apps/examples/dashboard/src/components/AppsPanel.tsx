import type { PatchcordClient, AppSummary } from "@patchcord/sdk";
import RefreshIcon from "@mui/icons-material/Refresh";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import IconButton from "@mui/material/IconButton";
import Link from "@mui/material/Link";
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

export default function AppsPanel({ client, agentBaseUrl }: { client: PatchcordClient; agentBaseUrl: string }) {
  const [apps, setApps] = useState<AppSummary[] | undefined>();
  const [error, setError] = useState<string | undefined>();

  function load() {
    setError(undefined);
    client.apps
      .list()
      .then(setApps)
      .catch((err: Error) => setError(err.message));
  }

  useEffect(load, [client]);

  return (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.5 }}>
        <Typography variant="h6">Installed applications</Typography>
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

      {!apps && !error && (
        <Stack alignItems="center" sx={{ py: 6 }}>
          <CircularProgress size={28} />
        </Stack>
      )}

      {apps && apps.length === 0 && !error && (
        <Typography color="text.secondary">
          No application installed yet — try <code>patchcord app install apps/examples/greeter</code>.
        </Typography>
      )}

      {apps && apps.length > 0 && (
        <Paper variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>App</TableCell>
                <TableCell>Version</TableCell>
                <TableCell>Permitted workflows</TableCell>
                <TableCell>Served at</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {apps.map((app) => (
                <TableRow key={app.id} hover>
                  <TableCell>
                    <Typography fontFamily="ui-monospace, monospace" fontWeight={600}>
                      {app.id}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip label={app.version} size="small" />
                  </TableCell>
                  <TableCell>
                    {app.workflowsRun.length === 0 ? (
                      <Typography variant="caption" color="text.secondary">
                        none declared
                      </Typography>
                    ) : (
                      <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                        {app.workflowsRun.map((w) => (
                          <Chip key={w} label={w} size="small" variant="outlined" />
                        ))}
                      </Stack>
                    )}
                  </TableCell>
                  <TableCell>
                    <Link href={`${agentBaseUrl}/apps/${app.id}/`} target="_blank" rel="noreferrer" underline="hover">
                      /apps/{app.id}/
                    </Link>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Paper>
      )}
    </Box>
  );
}
