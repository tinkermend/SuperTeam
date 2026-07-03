import type { ApiClientOptions } from "./client";
import { getJson, postJson, postJsonWithoutBody, putJson } from "./client";

export type TaskStatus = "pending" | "claimed" | "running" | "completed" | "failed" | "cancelled";

export type TaskResponse = {
  id: string;
  tenant_id?: string;
  team_id?: string;
  title: string;
  provider_type: string;
  status: TaskStatus;
  priority: number;
  description?: string;
  creator_id?: string;
  target_node_id?: string;
  assigned_node_id?: string;
  workspace_path?: string;
  params?: Record<string, unknown>;
  cancelled_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateTaskInput = {
  title: string;
  provider_type: string;
  params: Record<string, unknown>;
  description?: string;
  priority?: number;
  target_node_id?: string;
  workspace_path?: string;
};

export type UpdateTaskStatusInput = {
  status: TaskStatus;
};

export type ListTasksOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
};

function listTasksPath(options: ListTasksOptions): string {
  const params = new URLSearchParams();

  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }

  if (options.offset !== undefined) {
    params.set("offset", String(options.offset));
  }

  const query = params.toString();
  return `/api/v1/tasks${query ? `?${query}` : ""}`;
}

export function listTasks(options: ListTasksOptions): Promise<TaskResponse[]> {
  return getJson<TaskResponse[]>(options, listTasksPath(options), "tasks");
}

export function createTask(options: ApiClientOptions, input: CreateTaskInput): Promise<TaskResponse> {
  return postJson<TaskResponse>(options, "/api/v1/tasks", input, "tasks");
}

export function getTask(options: ApiClientOptions, taskId: string): Promise<TaskResponse> {
  return getJson<TaskResponse>(options, `/api/v1/tasks/${taskId}`, "tasks");
}

export function updateTaskStatus(
  options: ApiClientOptions,
  taskId: string,
  input: UpdateTaskStatusInput,
): Promise<TaskResponse> {
  return putJson<TaskResponse>(options, `/api/v1/tasks/${taskId}/status`, input, "tasks");
}

export function cancelTask(options: ApiClientOptions, taskId: string): Promise<TaskResponse> {
  return postJsonWithoutBody<TaskResponse>(options, `/api/v1/tasks/${taskId}/cancel`, "tasks");
}
