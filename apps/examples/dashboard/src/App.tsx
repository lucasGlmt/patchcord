import { PatchcordClient } from "@patchcord/sdk";
import DarkModeIcon from "@mui/icons-material/DarkMode";
import LightModeIcon from "@mui/icons-material/LightMode";
import SettingsIcon from "@mui/icons-material/Settings";
import AppBar from "@mui/material/AppBar";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import IconButton from "@mui/material/IconButton";
import Popover from "@mui/material/Popover";
import Stack from "@mui/material/Stack";
import Tab from "@mui/material/Tab";
import Tabs from "@mui/material/Tabs";
import TextField from "@mui/material/TextField";
import Toolbar from "@mui/material/Toolbar";
import Typography from "@mui/material/Typography";
import { useEffect, useMemo, useState } from "react";

import AppsPanel from "./components/AppsPanel";
import HealthChip, { type HealthState } from "./components/HealthChip";
import WorkflowsPanel from "./components/WorkflowsPanel";
import { defaultBaseUrl } from "./apiClient";

export default function App({ onToggleMode, mode }: { onToggleMode: () => void; mode: "light" | "dark" }) {
  const [baseUrl, setBaseUrl] = useState(defaultBaseUrl);
  const [tab, setTab] = useState(0);
  const [health, setHealth] = useState<HealthState>("checking");
  const [settingsAnchor, setSettingsAnchor] = useState<HTMLElement | null>(null);

  const client = useMemo(() => new PatchcordClient({ baseUrl }), [baseUrl]);

  useEffect(() => {
    let cancelled = false;
    setHealth("checking");
    client.system
      .health()
      .then((status) => {
        if (!cancelled) setHealth(status.status === "ok" ? "ok" : "degraded");
      })
      .catch(() => {
        if (!cancelled) setHealth("unreachable");
      });
    return () => {
      cancelled = true;
    };
  }, [client]);

  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "background.default" }}>
      <AppBar position="static" color="transparent" sx={{ borderBottom: "1px solid", borderColor: "divider" }}>
        <Toolbar>
          <Typography variant="h6" component="div" sx={{ flexGrow: 1, fontWeight: 700 }}>
            Patchcord Dashboard
          </Typography>
          <Stack direction="row" spacing={1.5} alignItems="center">
            <HealthChip state={health} />
            <IconButton size="small" onClick={(e) => setSettingsAnchor(e.currentTarget)} title="Agent base URL">
              <SettingsIcon fontSize="small" />
            </IconButton>
            <IconButton size="small" onClick={onToggleMode} title="Toggle theme">
              {mode === "dark" ? <LightModeIcon fontSize="small" /> : <DarkModeIcon fontSize="small" />}
            </IconButton>
          </Stack>
        </Toolbar>
      </AppBar>

      <Popover
        open={Boolean(settingsAnchor)}
        anchorEl={settingsAnchor}
        onClose={() => setSettingsAnchor(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
      >
        <Box sx={{ p: 2, width: 320 }}>
          <TextField
            label="Agent base URL"
            fullWidth
            size="small"
            value={baseUrl}
            onChange={(e) => setBaseUrl(e.target.value)}
            helperText="The Patchcord agent's HTTP API"
          />
        </Box>
      </Popover>

      <Container maxWidth="md" sx={{ py: 3 }}>
        <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 3 }}>
          <Tab label="Workflows" />
          <Tab label="Apps" />
        </Tabs>

        {tab === 0 && <WorkflowsPanel client={client} />}
        {tab === 1 && <AppsPanel client={client} agentBaseUrl={baseUrl} />}
      </Container>
    </Box>
  );
}
