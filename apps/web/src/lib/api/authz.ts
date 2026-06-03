import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type AuthzDecisionResult = "succeeded" | "failed";
export type RuntimeScopeStatus = "active" | "disabled";
export type RuntimeScopeType = "tenant" | "team";

type AuthzRef = {
  id: string;
  type: string;
};

type AuthzMembershipRecord = {
  tenant_id: string;
  team_id?: string | null;
  principal_type: string;
  principal_id: string;
  role: string;
  status: string;
};

export type AuthzDecisionRecord = {
  id: string;
  tenant_id: string;
  user_id?: string | null;
  username?: string | null;
  module: string;
  action: string;
  result: AuthzDecisionResult;
  resource_type?: string | null;
  resource_id?: string | null;
  request_id?: string | null;
  engine?: string | null;
  reason?: string | null;
  matched_rule?: string | null;
  actor_type?: string | null;
  actor_id?: string | null;
  details?: Record<string, unknown>;
  created_at: string;
};

export type AuthzOverviewResponse = {
  engine: {
    engine: string;
    status: string;
    engine_version?: string | null;
  };
  totals: {
    total: number;
    allowed: number;
    denied: number;
    denied_rate: number;
  };
  top_denied_actions: Array<{
    action: string;
    count: number;
  }>;
  recent_events: AuthzDecisionRecord[];
};

export type AuthzDecisionListResponse = {
  items: AuthzDecisionRecord[];
};

export type RuntimeScope = {
  id: string;
  tenant_id: string;
  runtime_node_id: string;
  team_id?: string | null;
  scope_type: RuntimeScopeType;
  scope_value: string;
  status: RuntimeScopeStatus;
  disabled_at?: string | null;
  created_at: string;
  updated_at: string;
};

export type RuntimeScopeNode = {
  runtime_node_id: string;
  node_id: string;
  name: string;
  supported_providers: string[];
  max_slots: number;
  current_load: number;
  status: string;
  last_heartbeat_at?: string | null;
  recent_denied_reason?: string | null;
  scopes: RuntimeScope[];
};

export type RuntimeScopeListResponse = {
  nodes: RuntimeScopeNode[];
};

export type RuntimeScopeResponse = {
  scope: RuntimeScope;
};

export type CreateRuntimeScopeRequest = {
  runtime_node_id: string;
  tenant_id: string;
  team_id?: string | null;
  scope_type: RuntimeScopeType;
  scope_value: string;
};

export type AuthzMemberRecord = {
  user_id: string;
  username: string;
  email?: string | null;
  display_name?: string | null;
  account_status: string;
  memberships: AuthzMembershipRecord[];
  console_access: boolean;
  recent_denied_reason?: string | null;
};

export type AuthzMemberListResponse = {
  items: AuthzMemberRecord[];
};

export type CheckPermissionRequest = {
  actor: AuthzRef;
  action:
    | "console.access"
    | "tenant.access"
    | "team.access"
    | "task.claim"
    | "authz_center.read"
    | "runtime_scope.manage"
    | "team.create"
    | "team.read"
    | "team.update"
    | "team.disable"
    | "team.archive"
    | "team.restore"
    | "team.member.add"
    | "team.member.remove"
    | "team.member.change_role"
    | "team.member.request_privileged_role"
    | "team.member.approve_privileged_role"
    | "team.governance.read"
    | "team.governance.edit"
    | "team.governance.approve"
    | "team.capability.bind"
    | "team.capability.unbind"
    | "team.audit.read";
  resource: AuthzRef;
  tenant_id: string;
  team_id?: string | null;
};

export type CheckPermissionResponse = {
  allowed: boolean;
  reason: string;
  matched_rule: string;
  engine: string;
  snapshot?: Record<string, unknown>;
};

export type ListAuthzDecisionsOptions = ApiClientOptions & {
  result?: AuthzDecisionResult;
  action?: string;
  actor_type?: string;
  actor_id?: string;
  resource_type?: string;
  resource_id?: string;
  request_id?: string;
  limit?: number;
  offset?: number;
};

export type ListAuthzMembersOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
};

function buildQueryPath(path: string, params: Record<string, number | string | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) {
      search.set(key, String(value));
    }
  }

  const query = search.toString();
  return query ? `${path}?${query}` : path;
}

async function getJson<T>(options: ApiClientOptions, path: string, resource: string): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<T>(response, resource);
}

export function getAuthzOverview(options: ApiClientOptions): Promise<AuthzOverviewResponse> {
  return getJson<AuthzOverviewResponse>(options, "/api/authz/overview", "authz overview");
}

export function listAuthzDecisions(options: ListAuthzDecisionsOptions): Promise<AuthzDecisionListResponse> {
  return getJson<AuthzDecisionListResponse>(
    options,
    buildQueryPath("/api/authz/decisions", {
      result: options.result,
      action: options.action,
      actor_type: options.actor_type,
      actor_id: options.actor_id,
      resource_type: options.resource_type,
      resource_id: options.resource_id,
      request_id: options.request_id,
      limit: options.limit,
      offset: options.offset,
    }),
    "authz decisions",
  );
}

export function listRuntimeScopes(options: ApiClientOptions): Promise<RuntimeScopeListResponse> {
  return getJson<RuntimeScopeListResponse>(options, "/api/authz/runtime-scopes", "runtime scopes");
}

export function listAuthzMembers(options: ListAuthzMembersOptions): Promise<AuthzMemberListResponse> {
  return getJson<AuthzMemberListResponse>(
    options,
    buildQueryPath("/api/authz/members", {
      limit: options.limit,
      offset: options.offset,
    }),
    "authz members",
  );
}

export async function createRuntimeScope(
  options: ApiClientOptions,
  input: CreateRuntimeScopeRequest,
): Promise<RuntimeScopeResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/authz/runtime-scopes"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "POST",
  });

  return parseJson<RuntimeScopeResponse>(response, "create runtime scope");
}

export async function updateRuntimeScope(
  options: ApiClientOptions,
  scopeID: string,
  status: RuntimeScopeStatus,
): Promise<RuntimeScopeResponse> {
  const fetcher = options.fetcher ?? fetch;
  const encodedScopeID = encodeURIComponent(scopeID);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/authz/runtime-scopes/${encodedScopeID}`), {
    body: JSON.stringify({ status }),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "PATCH",
  });

  return parseJson<RuntimeScopeResponse>(response, "update runtime scope");
}

export async function checkPermission(
  options: ApiClientOptions,
  input: CheckPermissionRequest,
): Promise<CheckPermissionResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/authz/check"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "POST",
  });

  return parseJson<CheckPermissionResponse>(response, "authz check");
}
