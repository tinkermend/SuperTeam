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
