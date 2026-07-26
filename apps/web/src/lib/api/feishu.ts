import { buildApiUrl, getJson, postJson, type ApiClientOptions } from "./client";

export type FeishuIdentity = {
  auth_user_id: string;
  open_id: string;
  bound_via: "contact_sync" | "oauth";
};

export type FeishuContactSyncReport = {
  app_id: string;
  matched: number;
  bound: number;
  already_bound: number;
  unmatched: number;
  conflicts?: number;
};

export async function listFeishuIdentities(options: ApiClientOptions): Promise<FeishuIdentity[]> {
  const payload = await getJson<{ identities: FeishuIdentity[] }>(
    options,
    "/api/v1/admin/feishu/identities",
    "feishu identities",
  );
  return payload.identities ?? [];
}

export async function syncFeishuContacts(options: ApiClientOptions): Promise<FeishuContactSyncReport[]> {
  const payload = await postJson<{ reports: FeishuContactSyncReport[] }>(
    options,
    "/api/v1/admin/feishu/contact-sync",
    {},
    "feishu contact sync",
  );
  return payload.reports ?? [];
}

// OAuth 绑定是整页跳转(外部授权页),返回目标地址由调用方赋给 window.location。
export function feishuOAuthStartUrl(options: ApiClientOptions, returnTo: string): string {
  const query = new URLSearchParams({ return_to: returnTo });
  return buildApiUrl(options.baseUrl, `/api/v1/auth/feishu/oauth-start?${query.toString()}`);
}
