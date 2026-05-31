import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type RuntimeNodeStatus = "online" | "offline";

export type RuntimeNodeResponse = {
  node_id: string;
  name: string;
  supported_providers: string[];
  max_slots: number;
  current_load: number;
  status: RuntimeNodeStatus;
  metadata?: Record<string, unknown>;
  last_heartbeat_at?: string;
  created_at?: string;
  updated_at?: string;
};

export async function listRuntimeNodes(options: ApiClientOptions): Promise<RuntimeNodeResponse[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/runtime/nodes"), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<RuntimeNodeResponse[]>(response, "runtime nodes");
}

export async function getRuntimeNode(options: ApiClientOptions, nodeId: string): Promise<RuntimeNodeResponse> {
  const fetcher = options.fetcher ?? fetch;
  const encodedNodeId = encodeURIComponent(nodeId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/runtime/nodes/${encodedNodeId}`), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<RuntimeNodeResponse>(response, "runtime nodes");
}
