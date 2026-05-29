export type TaskSummaryInput = {
  id: unknown;
  provider_type?: unknown;
  providerType?: unknown;
};

export type TaskSummary = {
  id: string;
  providerLabel: string;
};

function toLabel(value: unknown, fallback: string): string {
  return typeof value === "string" && value.length > 0 ? value : fallback;
}

export function createTaskSummary(task: TaskSummaryInput): TaskSummary {
  return {
    id: String(task.id),
    providerLabel: toLabel(task.provider_type ?? task.providerType, "unknown"),
  };
}
