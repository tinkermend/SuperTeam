import type { ApiClientOptions } from "./health";

export type TaskResponse = Record<string, unknown>;

export type CreateTaskInput = {
  title: string;
  description?: string;
  provider_type?: string;
  risk_level?: string;
  context?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
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
  input: CreateTaskInput,
  options: ApiClientOptions,
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
