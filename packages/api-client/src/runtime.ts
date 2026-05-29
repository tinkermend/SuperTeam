import type { ApiClientOptions } from "./health";

export type RuntimeNodeResponse = {
  node_id: string;
  name: string;
  supported_providers?: string[];
  max_slots: number;
  current_load: number;
  status: string;
  metadata?: Record<string, unknown>;
  last_heartbeat_at?: string;
  created_at?: string;
  updated_at?: string;
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

export async function listRuntimeNodes(
  options: ApiClientOptions,
): Promise<RuntimeNodeResponse[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/runtime/nodes"), {
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<RuntimeNodeResponse[]>(response, "runtime nodes");
}
