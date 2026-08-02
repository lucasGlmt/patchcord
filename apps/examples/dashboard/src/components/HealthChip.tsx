import Chip from "@mui/material/Chip";

export type HealthState = "checking" | "ok" | "degraded" | "unreachable";

const config: Record<HealthState, { label: string; color: "default" | "success" | "warning" | "error" }> = {
  checking: { label: "Checking…", color: "default" },
  ok: { label: "Agent healthy", color: "success" },
  degraded: { label: "Agent degraded", color: "warning" },
  unreachable: { label: "Agent unreachable", color: "error" },
};

export default function HealthChip({ state }: { state: HealthState }) {
  const c = config[state];
  return <Chip label={c.label} color={c.color} size="small" variant="outlined" />;
}
