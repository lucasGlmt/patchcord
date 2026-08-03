import BlockIcon from "@mui/icons-material/Block";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import ErrorIcon from "@mui/icons-material/Error";
import HourglassEmptyIcon from "@mui/icons-material/HourglassEmpty";
import RefreshIcon from "@mui/icons-material/Refresh";
import Chip, { type ChipProps } from "@mui/material/Chip";

import { RunningPulse } from "../motion";

// Covers both a run's and a step's status (internal/workflow.RunStatus /
// StepStatus) — "pending" and "queued" are the two entities' respective
// "hasn't started" state, kept as separate literals here rather than
// unified so a caller can pass either type straight through.
export type Status = "queued" | "pending" | "running" | "succeeded" | "failed" | "skipped" | "cancelled";

const statusConfig: Record<Status, { label: string; color: ChipProps["color"]; icon: React.ReactElement }> = {
  queued: { label: "Queued", color: "default", icon: <HourglassEmptyIcon /> },
  pending: { label: "Pending", color: "default", icon: <HourglassEmptyIcon /> },
  running: { label: "Running", color: "info", icon: <RefreshIcon className="spin" /> },
  succeeded: { label: "Succeeded", color: "success", icon: <CheckCircleIcon /> },
  failed: { label: "Failed", color: "error", icon: <ErrorIcon /> },
  skipped: { label: "Skipped", color: "default", icon: <BlockIcon /> },
  cancelled: { label: "Cancelled", color: "warning", icon: <BlockIcon /> },
};

export default function StatusChip({ status, size = "small" }: { status: string; size?: ChipProps["size"] }) {
  const config = statusConfig[status as Status] ?? { label: status, color: "default" as const, icon: undefined };
  const chip = <Chip label={config.label} color={config.color} icon={config.icon} size={size} variant="outlined" />;
  return status === "running" ? <RunningPulse>{chip}</RunningPulse> : chip;
}
