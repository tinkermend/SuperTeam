import { buildApiUrl, getJson, parseJson, postJson, type ApiClientOptions } from "./client";

export type FeishuAppConfigStatus = "active" | "unverified" | "disabled";

export type FeishuAppConfig = {
  id: string;
  app_id: string;
  status: FeishuAppConfigStatus;
};

export type FeishuConnectivityProbe = {
  key: string;
  label: string;
  ok: boolean;
  hint: string;
  code?: number;
  raw_msg?: string;
};

export type FeishuConnectivityReport = {
  token_ok: boolean;
  ok: boolean;
  probes: FeishuConnectivityProbe[];
  summary: string;
};

export type UpsertFeishuAppConfigResponse = {
  config: FeishuAppConfig;
  verify: FeishuConnectivityReport;
};

export type ServiceToken = {
  id: string;
  service_name: string;
  status: "active" | "revoked";
  created_at: string;
  last_used_at?: string | null;
  revoked_at?: string | null;
};

export type IssuedServiceToken = {
  id: string;
  service_name: string;
  token: string;
};

export type FeishuChannelHealthStatus = "healthy" | "stale" | "missing";

export type FeishuChannelAppStatus = {
  app_id: string;
  config_id?: string;
  ws_status: "connected" | "reconnecting" | "stopped" | "unknown" | string;
  last_ws_event_at?: string | null;
};

export type FeishuChannelHealth = {
  service_name: string;
  version?: string;
  status: FeishuChannelHealthStatus;
  last_heartbeat_at?: string | null;
  last_outbox_poll_at?: string | null;
  age_seconds?: number | null;
  apps: FeishuChannelAppStatus[];
  timeout_seconds: number;
};

export type FeishuOperationalOutboxItem = {
  id: string;
  kind: string;
  status: string;
  resource_type: string;
  resource_id: string;
  project_id?: string;
  recipient_user_id: string;
  recipient_open_id: string;
  attempts: number;
  last_error?: string | null;
  created_at: string;
  updated_at: string;
};

export async function listFeishuAppConfigs(options: ApiClientOptions): Promise<FeishuAppConfig[]> {
  const payload = await getJson<{ configs: FeishuAppConfig[] }>(
    options,
    "/api/v1/admin/feishu/app-configs",
    "feishu app configs",
  );
  return payload.configs ?? [];
}

export async function upsertFeishuAppConfig(
  options: ApiClientOptions,
  input: { app_id: string; app_secret: string },
): Promise<UpsertFeishuAppConfigResponse> {
  return postJson<UpsertFeishuAppConfigResponse>(
    options,
    "/api/v1/admin/feishu/app-configs",
    input,
    "upsert feishu app config",
  );
}

export async function verifyFeishuAppConfig(
  options: ApiClientOptions,
  input: { app_id: string; app_secret: string },
): Promise<FeishuConnectivityReport> {
  return postJson<FeishuConnectivityReport>(
    options,
    "/api/v1/admin/feishu/app-configs/verify",
    input,
    "verify feishu app config",
  );
}

export async function setFeishuAppConfigStatus(
  options: ApiClientOptions,
  configId: string,
  status: FeishuAppConfigStatus,
): Promise<FeishuAppConfig> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(
    buildApiUrl(options.baseUrl, `/api/v1/admin/feishu/app-configs/${configId}/status`),
    {
      body: JSON.stringify({ status }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "PATCH",
    },
  );
  return parseJson<FeishuAppConfig>(response, "set feishu app config status");
}

export async function listServiceTokens(options: ApiClientOptions): Promise<ServiceToken[]> {
  const payload = await getJson<{ tokens: ServiceToken[] }>(
    options,
    "/api/v1/admin/service-tokens",
    "service tokens",
  );
  return payload.tokens ?? [];
}

export async function issueServiceToken(
  options: ApiClientOptions,
  serviceName: string,
): Promise<IssuedServiceToken> {
  return postJson<IssuedServiceToken>(
    options,
    "/api/v1/admin/service-tokens",
    { service_name: serviceName },
    "issue service token",
  );
}

export async function revokeServiceToken(options: ApiClientOptions, tokenId: string): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(
    buildApiUrl(options.baseUrl, `/api/v1/admin/service-tokens/${tokenId}`),
    {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "DELETE",
    },
  );
  if (!response.ok) {
    await parseJson(response, "revoke service token");
  }
}

export async function getFeishuChannelHealth(options: ApiClientOptions): Promise<FeishuChannelHealth> {
  return getJson<FeishuChannelHealth>(
    options,
    "/api/v1/admin/feishu/channel-health",
    "feishu channel health",
  );
}

export async function listFeishuOperationalOutbox(
  options: ApiClientOptions,
  params?: { status?: string; limit?: number; offset?: number },
): Promise<{ items: FeishuOperationalOutboxItem[]; total: number }> {
  const query = new URLSearchParams();
  if (params?.status) query.set("status", params.status);
  if (params?.limit != null) query.set("limit", String(params.limit));
  if (params?.offset != null) query.set("offset", String(params.offset));
  const suffix = query.toString() ? `?${query.toString()}` : "";
  return getJson<{ items: FeishuOperationalOutboxItem[]; total: number }>(
    options,
    `/api/v1/admin/feishu/outbox${suffix}`,
    "feishu operational outbox",
  );
}

export async function requeueFeishuOutbox(
  options: ApiClientOptions,
  outboxId: string,
): Promise<FeishuOperationalOutboxItem> {
  return postJson<FeishuOperationalOutboxItem>(
    options,
    `/api/v1/admin/feishu/outbox/${outboxId}/requeue`,
    {},
    "requeue feishu outbox",
  );
}
