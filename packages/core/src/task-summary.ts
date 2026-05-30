export type SummaryTone = "danger" | "info" | "neutral" | "success" | "warning";

export type TaskSummaryInput = {
  id: string | number;
  title: string;
  status: string;
  provider_type?: string;
  providerType?: string;
};

export type TaskSummary = {
  id: string;
  title: string;
  status: string;
  statusTone: SummaryTone;
  providerLabel: string;
};

function toLabel(value: unknown, fallback: string): string {
  return typeof value === "string" && value.length > 0 ? value : fallback;
}

function toTaskStatusTone(status: string): SummaryTone {
  switch (status) {
    case "pending":
      return "warning";
    case "claimed":
    case "running":
      return "info";
    case "completed":
      return "success";
    case "failed":
      return "danger";
    case "cancelled":
    default:
      return "neutral";
  }
}

export function summarizeTask(raw: TaskSummaryInput): TaskSummary {
  return {
    id: String(raw.id),
    title: raw.title,
    status: raw.status,
    statusTone: toTaskStatusTone(raw.status),
    providerLabel: toLabel(raw.provider_type ?? raw.providerType, "unknown"),
  };
}
