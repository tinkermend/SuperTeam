import type { ApiClientOptions } from "./client";
import { getJson, postJson, postJsonWithoutBody, putJson } from "./client";

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

function listRuntimeNodesPath(options: ListRuntimeNodesOptions): string {
  const params = new URLSearchParams();

  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }

  if (options.offset !== undefined) {
    params.set("offset", String(options.offset));
  }

  const query = params.toString();
  return `/api/v1/runtime/nodes${query ? `?${query}` : ""}`;
}

function listRuntimeEventsPath(options: ListRuntimeEventsOptions): string {
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
  return `/api/v1/runtime/events${query ? `?${query}` : ""}`;
}

export function getRuntimeOverview(options: ApiClientOptions): Promise<RuntimeOverview> {
  return getJson<RuntimeOverview>(options, "/api/v1/runtime/overview", "runtime overview");
}

export function listRuntimeEvents(options: ListRuntimeEventsOptions): Promise<RuntimeEventList> {
  return getJson<RuntimeEventList>(options, listRuntimeEventsPath(options), "runtime events");
}

export function listRuntimeNodes(options: ListRuntimeNodesOptions): Promise<RuntimeNodeResponse[]> {
  return getJson<RuntimeNodeResponse[]>(options, listRuntimeNodesPath(options), "runtime nodes");
}

export function getRuntimeNode(options: ApiClientOptions, nodeId: string): Promise<RuntimeNodeResponse> {
  const encodedNodeId = encodeURIComponent(nodeId);
  return getJson<RuntimeNodeResponse>(options, `/api/v1/runtime/nodes/${encodedNodeId}`, "runtime nodes");
}

export function listRuntimeEnrollments(options: ApiClientOptions): Promise<RuntimeEnrollment[]> {
  return getJson<RuntimeEnrollment[]>(options, "/api/v1/runtime/enrollments", "runtime enrollments");
}

export function approveRuntimeEnrollment(
  options: ApiClientOptions,
  enrollmentId: string,
): Promise<RuntimeEnrollment> {
  const encodedEnrollmentId = encodeURIComponent(enrollmentId);
  return postJsonWithoutBody<RuntimeEnrollment>(
    options,
    `/api/v1/runtime/enrollments/${encodedEnrollmentId}/approve`,
    "approve runtime enrollment",
  );
}

export function rejectRuntimeEnrollment(
  options: ApiClientOptions,
  enrollmentId: string,
  reason: string,
): Promise<RuntimeEnrollment> {
  const encodedEnrollmentId = encodeURIComponent(enrollmentId);
  return postJson<RuntimeEnrollment>(
    options,
    `/api/v1/runtime/enrollments/${encodedEnrollmentId}/reject`,
    { reason },
    "reject runtime enrollment",
  );
}

export type ProviderNativeConfigListItem = {
  provider_type: string;
  config_key: string;
  resolved_path?: string;
  format?: string;
  file_content_hash?: string;
  exists_on_node: boolean;
  manageable: boolean;
  unmanageable_reason?: string;
  source?: string;
  snapshot_at?: string;
  last_pulled_at?: string;
  last_pushed_at?: string;
  node_online: boolean;
};

export type ProviderNativeConfigDetail = {
  provider_type: string;
  config_key: string;
  resolved_path?: string;
  format?: string;
  managed_values: Record<string, unknown>;
  file_content_hash?: string;
  exists_on_node: boolean;
  manageable: boolean;
  unmanageable_reason?: string;
  source?: string;
  snapshot_at?: string;
  stale_hint: boolean;
  node_online: boolean;
  last_pulled_at?: string;
  last_pushed_at?: string;
};

export function listProviderNativeConfigs(
  options: ApiClientOptions,
  nodeId: string,
): Promise<ProviderNativeConfigListItem[]> {
  const encodedNodeId = encodeURIComponent(nodeId);
  return getJson<ProviderNativeConfigListItem[]>(
    options,
    `/api/v1/runtime/nodes/${encodedNodeId}/provider-native-configs`,
    "provider native configs",
  );
}

export function pullProviderNativeConfig(
  options: ApiClientOptions,
  nodeId: string,
  providerType: string,
  configKey: string,
): Promise<ProviderNativeConfigDetail> {
  const encodedNodeId = encodeURIComponent(nodeId);
  return postJson<ProviderNativeConfigDetail>(
    options,
    `/api/v1/runtime/nodes/${encodedNodeId}/provider-native-configs/pull`,
    { provider_type: providerType, config_key: configKey },
    "pull provider native config",
  );
}

export function getProviderNativeConfig(
  options: ApiClientOptions,
  nodeId: string,
  providerType: string,
  configKey: string,
): Promise<ProviderNativeConfigDetail> {
  const encodedNodeId = encodeURIComponent(nodeId);
  const encodedProvider = encodeURIComponent(providerType);
  const encodedKey = encodeURIComponent(configKey);
  return getJson<ProviderNativeConfigDetail>(
    options,
    `/api/v1/runtime/nodes/${encodedNodeId}/provider-native-configs/${encodedProvider}/${encodedKey}`,
    "provider native config snapshot",
  );
}

export function putProviderNativeConfig(
  options: ApiClientOptions,
  nodeId: string,
  providerType: string,
  configKey: string,
  values: Record<string, unknown>,
  expectedFileContentHash: string,
): Promise<ProviderNativeConfigDetail> {
  const encodedNodeId = encodeURIComponent(nodeId);
  const encodedProvider = encodeURIComponent(providerType);
  const encodedKey = encodeURIComponent(configKey);
  return putJson<ProviderNativeConfigDetail>(
    options,
    `/api/v1/runtime/nodes/${encodedNodeId}/provider-native-configs/${encodedProvider}/${encodedKey}`,
    {
      values,
      expected_file_content_hash: expectedFileContentHash,
    },
    "push provider native config",
  );
}
