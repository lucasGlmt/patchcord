import type { Connector, ConnectorTestResult, PatchcordClient, PluginSummary } from "@patchcord/sdk";
import AddIcon from "@mui/icons-material/Add";
import BoltIcon from "@mui/icons-material/Bolt";
import DeleteIcon from "@mui/icons-material/Delete";
import EditIcon from "@mui/icons-material/Edit";
import RefreshIcon from "@mui/icons-material/Refresh";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Divider from "@mui/material/Divider";
import Drawer from "@mui/material/Drawer";
import IconButton from "@mui/material/IconButton";
import MenuItem from "@mui/material/MenuItem";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import { useEffect, useState } from "react";

import { PageFade } from "../motion";

interface ConfigRow {
  key: string;
  value: string;
}

interface SecretRow {
  name: string;
  type: string;
  key: string;
}

function connectorToRows(connector: Connector): { config: ConfigRow[]; secrets: SecretRow[] } {
  return {
    config: Object.entries(connector.config).map(([key, value]) => ({ key, value: String(value) })),
    secrets: Object.entries(connector.secretRefs).map(([name, ref]) => ({ name, type: ref.type, key: ref.key })),
  };
}

export default function ConnectorsPage({ client }: { client: PatchcordClient }) {
  const [connectors, setConnectors] = useState<Connector[] | undefined>();
  const [plugins, setPlugins] = useState<PluginSummary[]>([]);
  const [error, setError] = useState<string | undefined>();
  const [testResults, setTestResults] = useState<Record<string, ConnectorTestResult>>({});
  const [testingId, setTestingId] = useState<string | undefined>();
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | undefined>();

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | undefined>();
  const [formId, setFormId] = useState("");
  const [formType, setFormType] = useState("");
  const [configRows, setConfigRows] = useState<ConfigRow[]>([]);
  const [secretRows, setSecretRows] = useState<SecretRow[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | undefined>();

  function load() {
    setError(undefined);
    client.connectors
      .list()
      .then(setConnectors)
      .catch((err: Error) => setError(err.message));
  }

  useEffect(load, [client]);
  useEffect(() => {
    client.plugins
      .list()
      .then(setPlugins)
      .catch(() => setPlugins([]));
  }, [client]);

  const typeOptions = Array.from(
    new Set([...plugins.flatMap((p) => p.connectors), ...(formType ? [formType] : [])]),
  );

  function openCreate() {
    setEditingId(undefined);
    setFormId("");
    setFormType("");
    setConfigRows([]);
    setSecretRows([]);
    setSubmitError(undefined);
    setDrawerOpen(true);
  }

  function openEdit(connector: Connector) {
    const rows = connectorToRows(connector);
    setEditingId(connector.id);
    setFormId(connector.id);
    setFormType(connector.type);
    setConfigRows(rows.config);
    setSecretRows(rows.secrets);
    setSubmitError(undefined);
    setDrawerOpen(true);
  }

  function buildConfig(): Record<string, unknown> {
    const config: Record<string, unknown> = {};
    for (const row of configRows) {
      if (row.key.trim() === "") continue;
      config[row.key.trim()] = row.value;
    }
    return config;
  }

  function buildSecretRefs(): Record<string, { type: string; key: string }> {
    const refs: Record<string, { type: string; key: string }> = {};
    for (const row of secretRows) {
      if (row.name.trim() === "") continue;
      refs[row.name.trim()] = { type: row.type.trim() || "env", key: row.key.trim() };
    }
    return refs;
  }

  async function submit() {
    setSubmitError(undefined);
    if (formId.trim() === "" || formType.trim() === "") {
      setSubmitError("Both id and type are required.");
      return;
    }

    setSubmitting(true);
    try {
      if (editingId) {
        // Delete-then-recreate (ADR-0020: no update endpoint) — if the
        // recreate below fails, the connector is genuinely gone; surface
        // that plainly and drop into "create" mode with the same values
        // still filled in, rather than losing the form.
        await client.connectors.delete(editingId);
        try {
          await client.connectors.create({ id: formId, type: formType, config: buildConfig(), secretRefs: buildSecretRefs() });
          setDrawerOpen(false);
          load();
        } catch (createErr) {
          setEditingId(undefined);
          setSubmitError(
            `"${editingId}" was deleted but recreating it failed: ${(createErr as Error).message}. ` +
              "It no longer exists — fix the fields below and submit again to recreate it.",
          );
          load();
        }
      } else {
        await client.connectors.create({ id: formId, type: formType, config: buildConfig(), secretRefs: buildSecretRefs() });
        setDrawerOpen(false);
        load();
      }
    } catch (err) {
      setSubmitError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  async function runDelete(id: string) {
    setConfirmDeleteId(undefined);
    try {
      await client.connectors.delete(id);
      load();
    } catch (err) {
      setError((err as Error).message);
    }
  }

  async function runTest(id: string) {
    setTestingId(id);
    try {
      const result = await client.connectors.test(id);
      setTestResults((prev) => ({ ...prev, [id]: result }));
    } catch (err) {
      setTestResults((prev) => ({ ...prev, [id]: { ok: false, message: (err as Error).message } }));
    } finally {
      setTestingId(undefined);
    }
  }

  return (
    <PageFade>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.5 }}>
        <Typography variant="h6">Connectors</Typography>
        <Stack direction="row" spacing={1}>
          <Tooltip title="Refresh">
            <IconButton size="small" onClick={load}>
              <RefreshIcon fontSize="small" />
            </IconButton>
          </Tooltip>
          <Button variant="contained" size="small" startIcon={<AddIcon />} onClick={openCreate}>
            New connector
          </Button>
        </Stack>
      </Stack>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {!connectors && !error && (
        <Stack alignItems="center" sx={{ py: 6 }}>
          <CircularProgress size={28} />
        </Stack>
      )}

      {connectors && connectors.length === 0 && !error && (
        <Typography color="text.secondary">No connector created yet — click "New connector" to add one.</Typography>
      )}

      {connectors && connectors.length > 0 && (
        <Paper variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Connector</TableCell>
                <TableCell>Type</TableCell>
                <TableCell>Secrets</TableCell>
                <TableCell>Created</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {connectors.map((connector) => (
                <TableRow key={connector.id} hover>
                  <TableCell>
                    <Typography fontFamily="ui-monospace, monospace" fontWeight={600}>
                      {connector.id}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Chip label={connector.type} size="small" variant="outlined" />
                  </TableCell>
                  <TableCell>
                    <Typography variant="caption" color="text.secondary">
                      {Object.keys(connector.secretRefs).length}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" color="text.secondary">
                      {new Date(connector.createdAt).toLocaleString()}
                    </Typography>
                  </TableCell>
                  <TableCell align="right">
                    <Stack direction="row" spacing={0.5} alignItems="center" justifyContent="flex-end">
                      {testResults[connector.id] && (
                        <Tooltip title={testResults[connector.id].message || ""}>
                          <Chip
                            size="small"
                            label={testResults[connector.id].message || (testResults[connector.id].ok ? "OK" : "Failed")}
                            color={testResults[connector.id].ok ? "success" : "error"}
                            variant="outlined"
                            sx={{ maxWidth: 220, "& .MuiChip-label": { overflow: "hidden", textOverflow: "ellipsis" } }}
                          />
                        </Tooltip>
                      )}
                      {confirmDeleteId === connector.id ? (
                        <>
                          <Button size="small" color="error" onClick={() => void runDelete(connector.id)}>
                            Confirm
                          </Button>
                          <Button size="small" onClick={() => setConfirmDeleteId(undefined)}>
                            Cancel
                          </Button>
                        </>
                      ) : (
                        <>
                          <Tooltip title="Test connection">
                            <IconButton size="small" onClick={() => void runTest(connector.id)} disabled={testingId === connector.id}>
                              {testingId === connector.id ? <CircularProgress size={16} /> : <BoltIcon fontSize="small" />}
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Modifier">
                            <IconButton size="small" onClick={() => openEdit(connector)}>
                              <EditIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Supprimer">
                            <IconButton size="small" onClick={() => setConfirmDeleteId(connector.id)}>
                              <DeleteIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                        </>
                      )}
                    </Stack>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Paper>
      )}

      <Drawer anchor="right" open={drawerOpen} onClose={() => !submitting && setDrawerOpen(false)}>
        <Box sx={{ width: 420, p: 3 }}>
          <Typography variant="h6" sx={{ mb: 2 }}>
            {editingId ? `Modifier ${editingId}` : "New connector"}
          </Typography>

          {editingId && (
            <Alert severity="warning" sx={{ mb: 2 }}>
              Modifying recreates this connector under the hood (delete, then create) — there is a brief moment where it
              doesn't exist. If it's referenced by a workflow binding elsewhere, that binding will fail until this
              finishes.
            </Alert>
          )}

          {submitError && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {submitError}
            </Alert>
          )}

          <Stack spacing={2}>
            <TextField
              label="id"
              value={formId}
              onChange={(e) => setFormId(e.target.value)}
              disabled={Boolean(editingId)}
              size="small"
              required
            />

            {typeOptions.length > 0 ? (
              <TextField
                select
                label="type"
                value={formType}
                onChange={(e) => setFormType(e.target.value)}
                size="small"
                required
              >
                {typeOptions.map((t) => (
                  <MenuItem key={t} value={t}>
                    {t}
                  </MenuItem>
                ))}
              </TextField>
            ) : (
              <TextField
                label="type"
                value={formType}
                onChange={(e) => setFormType(e.target.value)}
                size="small"
                required
                helperText="No installed plugin declares a connector type yet — enter one manually."
              />
            )}

            <Divider />

            <ConfigRowsEditor rows={configRows} onChange={setConfigRows} />
            <SecretRowsEditor rows={secretRows} onChange={setSecretRows} />

            <Stack direction="row" spacing={1} justifyContent="flex-end" sx={{ mt: 1 }}>
              <Button onClick={() => setDrawerOpen(false)} disabled={submitting}>
                Cancel
              </Button>
              <Button variant="contained" onClick={() => void submit()} disabled={submitting}>
                {submitting ? "Saving…" : editingId ? "Save changes" : "Create"}
              </Button>
            </Stack>
          </Stack>
        </Box>
      </Drawer>
    </PageFade>
  );
}

function ConfigRowsEditor({ rows, onChange }: { rows: ConfigRow[]; onChange: (rows: ConfigRow[]) => void }) {
  return (
    <Stack spacing={1}>
      <Typography variant="caption" color="text.secondary">
        Config
      </Typography>
      {rows.map((row, i) => (
        <Stack direction="row" spacing={1} key={i}>
          <TextField
            size="small"
            placeholder="key"
            value={row.key}
            onChange={(e) => onChange(rows.map((r, j) => (j === i ? { ...r, key: e.target.value } : r)))}
          />
          <TextField
            size="small"
            placeholder="value"
            value={row.value}
            onChange={(e) => onChange(rows.map((r, j) => (j === i ? { ...r, value: e.target.value } : r)))}
          />
          <IconButton size="small" onClick={() => onChange(rows.filter((_, j) => j !== i))}>
            <DeleteIcon fontSize="small" />
          </IconButton>
        </Stack>
      ))}
      <Button size="small" startIcon={<AddIcon />} onClick={() => onChange([...rows, { key: "", value: "" }])} sx={{ alignSelf: "flex-start" }}>
        Add config value
      </Button>
    </Stack>
  );
}

function SecretRowsEditor({ rows, onChange }: { rows: SecretRow[]; onChange: (rows: SecretRow[]) => void }) {
  return (
    <Stack spacing={1}>
      <Typography variant="caption" color="text.secondary">
        Secret references (never a resolved value — only a pointer, e.g. an environment variable name)
      </Typography>
      {rows.map((row, i) => (
        <Stack direction="row" spacing={1} key={i}>
          <TextField
            size="small"
            placeholder="name"
            value={row.name}
            onChange={(e) => onChange(rows.map((r, j) => (j === i ? { ...r, name: e.target.value } : r)))}
          />
          <TextField
            select
            size="small"
            value={row.type}
            onChange={(e) => onChange(rows.map((r, j) => (j === i ? { ...r, type: e.target.value } : r)))}
            sx={{ width: 90 }}
          >
            <MenuItem value="env">env</MenuItem>
          </TextField>
          <TextField
            size="small"
            placeholder="ENV_VAR_NAME"
            value={row.key}
            onChange={(e) => onChange(rows.map((r, j) => (j === i ? { ...r, key: e.target.value } : r)))}
          />
          <IconButton size="small" onClick={() => onChange(rows.filter((_, j) => j !== i))}>
            <DeleteIcon fontSize="small" />
          </IconButton>
        </Stack>
      ))}
      <Button
        size="small"
        startIcon={<AddIcon />}
        onClick={() => onChange([...rows, { name: "", type: "env", key: "" }])}
        sx={{ alignSelf: "flex-start" }}
      >
        Add secret reference
      </Button>
    </Stack>
  );
}
