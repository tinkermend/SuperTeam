import type { ApiClientOptions } from "./health";

export type TaskResponse = {
  id: string | number;
  title: string;
  provider_type: string;
  status: string;
  priority?: number;
  description?: string;
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

function buildApiUrl(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/+$/, "")}${path}`;
}

async function parseJson<T>(response: Response, resource: string): Promise<T> {
  if (!response.ok) {
    throw new Error(`${resource} request failed with status ${response.status}`);
  }

  return (await response.json()) as T;
}

export async function listTasks(options: ApiClientOptions): Promise<TaskResponse[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/tasks"), {
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<TaskResponse[]>(response, "tasks");
}

export async function createTask(
  options: ApiClientOptions,
  input: CreateTaskInput,
): Promise<TaskResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/tasks"), {
    body: JSON.stringify(input),
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "POST",
  });

  return parseJson<TaskResponse>(response, "tasks");
}
