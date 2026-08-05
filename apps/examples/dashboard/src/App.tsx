import { PatchcordClient } from "@glmtsolutions/patchcord-sdk";
import DarkModeIcon from "@mui/icons-material/DarkMode";
import LightModeIcon from "@mui/icons-material/LightMode";
import SettingsIcon from "@mui/icons-material/Settings";
import AppBar from "@mui/material/AppBar";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import IconButton from "@mui/material/IconButton";
import Popover from "@mui/material/Popover";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Toolbar from "@mui/material/Toolbar";
import { useEffect, useMemo, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";

import Sidebar, { sidebarWidth } from "./components/Sidebar";
import HealthChip, { type HealthState } from "./components/HealthChip";
import AppsPage from "./pages/AppsPage";
import ConnectorsPage from "./pages/ConnectorsPage";
import RunsPage from "./pages/RunsPage";
import WorkflowDetailPage from "./pages/WorkflowDetailPage";
import WorkflowsPage from "./pages/WorkflowsPage";
import { defaultBaseUrl } from "./apiClient";

export default function App({ onToggleMode, mode }: { onToggleMode: () => void; mode: "light" | "dark" }) {
  const [baseUrl, setBaseUrl] = useState(defaultBaseUrl);
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
    <Box sx={{ display: "flex", minHeight: "100vh", bgcolor: "background.default" }}>
      <Sidebar />

      <Box sx={{ flexGrow: 1, minWidth: 0, width: `calc(100% - ${sidebarWidth}px)` }}>
        <AppBar position="static" color="transparent" sx={{ borderBottom: "1px solid", borderColor: "divider" }}>
          <Toolbar>
            <Box sx={{ flexGrow: 1 }} />
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

        <Container maxWidth="lg" sx={{ py: 3 }}>
          <Routes>
            <Route path="/" element={<Navigate to="/workflows" replace />} />
            <Route path="/workflows" element={<WorkflowsPage client={client} />} />
            <Route path="/workflows/:id" element={<WorkflowDetailPage client={client} />} />
            <Route path="/runs" element={<RunsPage client={client} />} />
            <Route path="/connectors" element={<ConnectorsPage client={client} />} />
            <Route path="/apps" element={<AppsPage client={client} agentBaseUrl={baseUrl} />} />
            <Route path="*" element={<Navigate to="/workflows" replace />} />
          </Routes>
        </Container>
      </Box>
    </Box>
  );
}
