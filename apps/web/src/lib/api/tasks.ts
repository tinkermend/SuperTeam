import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type TaskStatus = "pending" | "claimed" | "running" | "completed" | "failed" | "cancelled";

export type TaskResponse = {
  id: number;
  title: string;
  provider_type: string;
  status: TaskStatus;
  priority: number;
  description?: string;
  creator_id?: number;
  target_node_id?: string;
  assigned_node_id?: string;
  workspace_path?: string;
  params?: Record<string, unknown>;
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

export async function listTasks(options: ApiClientOptions): Promise<TaskResponse[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/tasks"), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<TaskResponse[]>(response, "tasks");
}

export async function createTask(options: ApiClientOptions, input: CreateTaskInput): Promise<TaskResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/tasks"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "POST",
  });

  return parseJson<TaskResponse>(response, "tasks");
}

export async function getTask(options: ApiClientOptions, taskId: number): Promise<TaskResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/tasks/${taskId}`), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<TaskResponse>(response, "tasks");
}

export async function updateTaskStatus(
  options: ApiClientOptions,
  taskId: number,
  input: UpdateTaskStatusInput,
): Promise<TaskResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/tasks/${taskId}/status`), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "PUT",
  });

  return parseJson<TaskResponse>(response, "tasks");
}

export async function cancelTask(options: ApiClientOptions, taskId: number): Promise<TaskResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/tasks/${taskId}/cancel`), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "POST",
  });

  return parseJson<TaskResponse>(response, "tasks");
}
