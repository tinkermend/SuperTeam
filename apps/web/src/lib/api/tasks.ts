import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

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

function buildListTasksUrl(options: ListTasksOptions): string {
  const params = new URLSearchParams();

  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }

  if (options.offset !== undefined) {
    params.set("offset", String(options.offset));
  }

  const query = params.toString();
  return buildApiUrl(options.baseUrl, `/api/v1/tasks${query ? `?${query}` : ""}`);
}

export async function listTasks(options: ListTasksOptions): Promise<TaskResponse[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildListTasksUrl(options), {
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

export async function getTask(options: ApiClientOptions, taskId: string): Promise<TaskResponse> {
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
  taskId: string,
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

export async function cancelTask(options: ApiClientOptions, taskId: string): Promise<TaskResponse> {
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
