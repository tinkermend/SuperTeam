import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type RuntimeNodeStatus = "online" | "offline";
export type RuntimeEnrollmentStatus = "pending" | "approved" | "rejected" | "revoked";
export type RuntimeEventSeverity = "info" | "success" | "warning" | "error";

export type RuntimeNodeResponse = {
  runtime_node_id?: string;
  node_id: string;
  name: string;
  supported_providers: string[];
  max_slots: number;
  current_load: number;
  status: RuntimeNodeStatus;
  command_channel_connected?: boolean;
  metadata?: Record<string, unknown>;
  last_heartbeat_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type RuntimeEnrollment = {
  id: string;
  tenant_id?: string;
  node_id: string;
  runtime_node_id?: string;
  bootstrap_key_id?: string;
  status: RuntimeEnrollmentStatus;
  request_payload?: Record<string, unknown>;
  approved_by?: string;
  approved_at?: string;
  rejected_by?: string;
  rejected_at?: string;
  reject_reason?: string;
  revoked_by?: string;
  revoked_at?: string;
  revoke_reason?: string;
  last_hello_at?: string;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export type RuntimeEvent = {
  id: string;
  tenant_id?: string;
  runtime_node_id?: string;
  node_id?: string;
  event_type: string;
  severity: RuntimeEventSeverity;
  source: string;
  title: string;
  description?: string;
  provider_type?: string;
  correlation_type?: string;
  correlation_id?: string;
  payload?: Record<string, unknown>;
  created_at: string;
};

export type RuntimeProviderCapabilitySummary = {
  provider_type: string;
  node_count: number;
  available_count: number;
  healthy_count: number;
  last_seen_at?: string;
};

export type RuntimeOverview = {
  summary: {
    online_nodes: number;
    total_nodes: number;
    pending_enrollments: number;
    active_provider_sessions: number;
    blocked_events: number;
  };
  pending_enrollments: RuntimeEnrollment[];
  nodes: RuntimeNodeResponse[];
  provider_capabilities: RuntimeProviderCapabilitySummary[];
  recent_events: RuntimeEvent[];
};

export type RuntimeEventList = {
  items: RuntimeEvent[];
  limit: number;
  offset: number;
};

export type ListRuntimeNodesOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
};

export type ListRuntimeEventsOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
  event_type?: string;
  severity?: RuntimeEventSeverity;
  node_id?: string;
  provider_type?: string;
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

function buildListRuntimeEventsUrl(options: ListRuntimeEventsOptions): string {
  const params = new URLSearchParams();

  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }

  if (options.offset !== undefined) {
    params.set("offset", String(options.offset));
  }

  if (options.event_type) {
    params.set("event_type", options.event_type);
  }

  if (options.severity) {
    params.set("severity", options.severity);
  }

  if (options.node_id) {
    params.set("node_id", options.node_id);
  }

  if (options.provider_type) {
    params.set("provider_type", options.provider_type);
  }

  const query = params.toString();
  return buildApiUrl(options.baseUrl, `/api/v1/runtime/events${query ? `?${query}` : ""}`);
}

export async function getRuntimeOverview(options: ApiClientOptions): Promise<RuntimeOverview> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/runtime/overview"), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<RuntimeOverview>(response, "runtime overview");
}

export async function listRuntimeEvents(options: ListRuntimeEventsOptions): Promise<RuntimeEventList> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildListRuntimeEventsUrl(options), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<RuntimeEventList>(response, "runtime events");
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

export async function rejectRuntimeEnrollment(
  options: ApiClientOptions,
  enrollmentId: string,
  reason: string,
): Promise<RuntimeEnrollment> {
  const fetcher = options.fetcher ?? fetch;
  const encodedEnrollmentId = encodeURIComponent(enrollmentId);
  const response = await fetcher(
    buildApiUrl(options.baseUrl, `/api/v1/runtime/enrollments/${encodedEnrollmentId}/reject`),
    {
      body: JSON.stringify({ reason }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    },
  );

  return parseJson<RuntimeEnrollment>(response, "reject runtime enrollment");
}
