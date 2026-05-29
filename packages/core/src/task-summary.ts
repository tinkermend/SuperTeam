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
  providerLabel: string;
};

function toLabel(value: unknown, fallback: string): string {
  return typeof value === "string" && value.length > 0 ? value : fallback;
}

export function summarizeTask(raw: TaskSummaryInput): TaskSummary {
  return {
    id: String(raw.id),
    title: raw.title,
    status: raw.status,
    providerLabel: toLabel(raw.provider_type ?? raw.providerType, "unknown"),
  };
}
