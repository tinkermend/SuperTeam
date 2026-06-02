import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type RuntimeNodeStatus = "online" | "offline";
export type RuntimeEnrollmentStatus = "pending" | "approved" | "rejected" | "revoked";

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

export type RuntimeEnrollment = {
  id: string;
  node_id: string;
  status: RuntimeEnrollmentStatus;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export type ListRuntimeNodesOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
};

function buildListRuntimeNodesUrl(options: ListRuntimeNodesOptions): string {
  const params = new URLSearchParams();

  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }

  if (options.offset !== undefined) {
    params.set("offset", String(options.offset));
  }

  const query = params.toString();
  return buildApiUrl(options.baseUrl, `/api/v1/runtime/nodes${query ? `?${query}` : ""}`);
}

export async function listRuntimeNodes(options: ListRuntimeNodesOptions): Promise<RuntimeNodeResponse[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildListRuntimeNodesUrl(options), {
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

export async function listRuntimeEnrollments(options: ApiClientOptions): Promise<RuntimeEnrollment[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/runtime/enrollments"), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<RuntimeEnrollment[]>(response, "runtime enrollments");
}

export async function approveRuntimeEnrollment(
  options: ApiClientOptions,
  enrollmentId: string,
): Promise<RuntimeEnrollment> {
  const fetcher = options.fetcher ?? fetch;
  const encodedEnrollmentId = encodeURIComponent(enrollmentId);
  const response = await fetcher(
    buildApiUrl(options.baseUrl, `/api/v1/runtime/enrollments/${encodedEnrollmentId}/approve`),
    {
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "POST",
    },
  );

  return parseJson<RuntimeEnrollment>(response, "approve runtime enrollment");
}
