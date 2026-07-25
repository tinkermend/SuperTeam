import type { ApiClientOptions } from "./client";
import { ApiRequestError, buildApiUrl, parseJson } from "./client";

export { ApiRequestError };

export type UserSummary = {
  avatar: UserAvatar;
  avatar_asset_id?: string | null;
  display_name?: string | null;
  email?: string | null;
  id: string;
  status: "active" | "disabled";
  username: string;
};

export type UserAvatar = {
  options?: Record<string, unknown>;
  provider: "dicebear";
  seed: string;
  style: "adventurer";
};

export type LoginRequest = {
  captcha_code?: string;
  captcha_id?: string;
  password: string;
  username: string;
};

export type LoginResponse = {
  user: UserSummary;
};

export type CaptchaChallengeResponse = {
  enabled: false;
  captcha_id?: never;
  expires_at?: never;
  image_data_url?: never;
} | {
  enabled: true;
  captcha_id: string;
  expires_at: string;
  image_data_url: string;
};

export type CurrentUserResponse = {
  user: UserSummary;
};

export type UserListResponse = {
  items: UserSummary[];
};

export type UserResponse = {
  user: UserSummary;
};

export type ListUsersOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
  q?: string;
  status?: UserSummary["status"];
};

export type TenantRole = "owner" | "admin" | "member" | "viewer";

export type CreateUserRequest = {
  avatar: UserAvatar;
  display_name: string;
  password: string;
  selectable_team_ids: string[];
  tenant_role: TenantRole;
  username: string;
};

export type TenantMembership = {
  created_at: string;
  id: string;
  role: TenantRole;
  status: "active" | "disabled";
  tenant_id: string;
  updated_at: string;
  user_id: string;
};

export type TenantMembershipResponse = {
  membership: TenantMembership;
};

export type UserProjectTeamSummary = {
  current_revision?: number | null;
  digital_employee_count: number;
  governance_status: string;
  human_owners: UserProjectTeamOwner[];
  id: string;
  name: string;
  pending_draft_count: number;
  risk_summary: string;
  slug: string;
  status: string;
};

export type UserProjectTeamOwner = {
  avatar: UserAvatar;
  avatar_asset_id?: string | null;
  display_name?: string | null;
  email?: string | null;
  id: string;
  status: string;
  username: string;
};

export type UserProjectTeamScope = {
  created_at: string;
  granted_by_user_id?: string | null;
  id: string;
  revoked_at?: string | null;
  status: string;
  team: UserProjectTeamSummary;
  team_id: string;
  tenant_id: string;
  updated_at: string;
  user_id: string;
};

export type UserProjectTeamScopeListResponse = {
  items: UserProjectTeamScope[];
};

export type ReplaceUserProjectTeamScopesRequest = {
  team_ids: string[];
};

export type LoginLogEventType = "login_succeeded" | "login_failed" | "logout_succeeded";

export type LoginLogResult = "succeeded" | "failed";

export type LoginLogRecord = {
  client_ip?: string;
  created_at: string;
  event_type: LoginLogEventType;
  failure_reason?: string;
  id: string;
  result: LoginLogResult;
  session_id?: string;
  user_agent?: string;
  user_id?: string;
  username: string;
};

export type LoginLogListResponse = {
  items: LoginLogRecord[];
};

export type ListLoginLogsOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
  event_type?: LoginLogEventType;
  result?: LoginLogResult;
};

export type OperationLogResult = "succeeded" | "failed";

export type OperationLogRecord = {
  action: string;
  client_ip?: string;
  created_at: string;
  id: string;
  module: string;
  request_id?: string;
  resource_id?: string;
  resource_type?: string;
  result: OperationLogResult;
  user_agent?: string;
  user_id?: string;
  username?: string;
};

export type OperationLogListResponse = {
  items: OperationLogRecord[];
};

export type ListOperationLogsOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
  module?: string;
  action?: string;
  result?: OperationLogResult;
};

export type UpdateCurrentUserProfileRequest = {
  avatar?: UserAvatar;
  display_name?: string;
  email?: string;
};

export type ChangeCurrentUserPasswordRequest = {
  current_password: string;
  password: string;
};

export async function login(options: ApiClientOptions, input: LoginRequest): Promise<LoginResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/auth/login"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "POST",
  });

  return parseJson<LoginResponse>(response, "auth login");
}

export async function getLoginCaptcha(options: ApiClientOptions): Promise<CaptchaChallengeResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/auth/captcha"), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<CaptchaChallengeResponse>(response, "auth captcha");
}

export async function getCurrentUser(options: ApiClientOptions): Promise<CurrentUserResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/auth/me"), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<CurrentUserResponse>(response, "auth current user");
}

// 自愈写回当前用户预渲染头像 data-URI（P1-D 2b）。仅写当前登录用户自己。
export async function setCurrentUserAvatarSvg(
  options: ApiClientOptions,
  svg: string,
): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/auth/me/avatar-svg"), {
    body: JSON.stringify({ svg }),
    credentials: "include",
    headers: { "content-type": "application/json" },
    method: "PUT",
  });
  if (!response.ok) {
    throw new Error(`set avatar svg failed: ${response.status}`);
  }
}

export async function logout(options: ApiClientOptions): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/auth/logout"), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "POST",
  });

  await parseJson<{ message: string }>(response, "auth logout");
}

export async function listLoginLogs(options: ListLoginLogsOptions): Promise<LoginLogListResponse> {
  const fetcher = options.fetcher ?? fetch;
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
  if (options.result) {
    params.set("result", options.result);
  }
  const query = params.toString();
  const path = query ? `/api/auth/login-logs?${query}` : "/api/auth/login-logs";
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<LoginLogListResponse>(response, "auth login logs");
}

export async function listCurrentUserLoginLogs(options: ListLoginLogsOptions): Promise<LoginLogListResponse> {
  const fetcher = options.fetcher ?? fetch;
  const params = new URLSearchParams();
  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }
  if (options.offset !== undefined) {
    params.set("offset", String(options.offset));
  }
  const query = params.toString();
  const path = query ? `/api/auth/account/login-logs?${query}` : "/api/auth/account/login-logs";
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<LoginLogListResponse>(response, "auth current user login logs");
}

export async function listOperationLogs(options: ListOperationLogsOptions): Promise<OperationLogListResponse> {
  const fetcher = options.fetcher ?? fetch;
  const params = new URLSearchParams();
  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }
  if (options.offset !== undefined) {
    params.set("offset", String(options.offset));
  }
  if (options.module) {
    params.set("module", options.module);
  }
  if (options.action) {
    params.set("action", options.action);
  }
  if (options.result) {
    params.set("result", options.result);
  }
  const query = params.toString();
  const path = query ? `/api/auth/operation-logs?${query}` : "/api/auth/operation-logs";
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<OperationLogListResponse>(response, "auth operation logs");
}

export async function listUsers(options: ListUsersOptions): Promise<UserListResponse> {
  const fetcher = options.fetcher ?? fetch;
  const params = new URLSearchParams();
  const q = options.q?.trim();
  if (q) {
    params.set("q", q);
  }
  if (options.status !== undefined) {
    params.set("status", options.status);
  }
  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }
  if (options.offset !== undefined) {
    params.set("offset", String(options.offset));
  }
  const query = params.toString();
  const path = query ? `/api/auth/users?${query}` : "/api/auth/users";
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<UserListResponse>(response, "auth users");
}

export async function createUser(options: ApiClientOptions, input: CreateUserRequest): Promise<UserResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/auth/users"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "POST",
  });

  return parseJson<UserResponse>(response, "auth create user");
}

export async function getUserTenantMembership(
  options: ApiClientOptions,
  userID: string,
): Promise<TenantMembershipResponse> {
  const fetcher = options.fetcher ?? fetch;
  const encodedUserID = encodeURIComponent(userID);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/auth/users/${encodedUserID}/tenant-membership`), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<TenantMembershipResponse>(response, "auth user tenant membership");
}

export async function upsertUserTenantMembership(
  options: ApiClientOptions,
  userID: string,
  role: TenantRole,
): Promise<TenantMembershipResponse> {
  const fetcher = options.fetcher ?? fetch;
  const encodedUserID = encodeURIComponent(userID);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/auth/users/${encodedUserID}/tenant-membership`), {
    body: JSON.stringify({ role }),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "PUT",
  });

  return parseJson<TenantMembershipResponse>(response, "auth upsert user tenant membership");
}

export async function deleteUserTenantMembership(options: ApiClientOptions, userID: string): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const encodedUserID = encodeURIComponent(userID);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/auth/users/${encodedUserID}/tenant-membership`), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "DELETE",
  });

  if (!response.ok) {
    await parseJson<unknown>(response, "auth delete user tenant membership");
  }
}

export async function listUserProjectTeamScopes(
  options: ApiClientOptions,
  userID: string,
): Promise<UserProjectTeamScopeListResponse> {
  const fetcher = options.fetcher ?? fetch;
  const encodedUserID = encodeURIComponent(userID);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/auth/users/${encodedUserID}/project-team-scopes`), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<UserProjectTeamScopeListResponse>(response, "auth user project team scopes");
}

export async function replaceUserProjectTeamScopes(
  options: ApiClientOptions,
  userID: string,
  input: ReplaceUserProjectTeamScopesRequest,
): Promise<UserProjectTeamScopeListResponse> {
  const fetcher = options.fetcher ?? fetch;
  const encodedUserID = encodeURIComponent(userID);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/auth/users/${encodedUserID}/project-team-scopes`), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "PUT",
  });

  return parseJson<UserProjectTeamScopeListResponse>(response, "auth replace user project team scopes");
}

export async function updateCurrentUserProfile(
  options: ApiClientOptions,
  input: UpdateCurrentUserProfileRequest,
): Promise<UserResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/auth/account/profile"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "PATCH",
  });

  return parseJson<UserResponse>(response, "auth current user profile");
}

export async function updateCurrentUserPassword(
  options: ApiClientOptions,
  input: ChangeCurrentUserPasswordRequest,
): Promise<UserResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/auth/account/password"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "POST",
  });

  return parseJson<UserResponse>(response, "auth current user password");
}

export async function updateUserStatus(
  options: ApiClientOptions,
  userID: string,
  status: UserSummary["status"],
): Promise<UserResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/auth/users/${userID}/status`), {
    body: JSON.stringify({ status }),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "PATCH",
  });

  return parseJson<UserResponse>(response, "auth update user status");
}

export async function resetUserPassword(options: ApiClientOptions, userID: string, password: string): Promise<UserResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/auth/users/${userID}/reset-password`), {
    body: JSON.stringify({ password }),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "POST",
  });

  return parseJson<UserResponse>(response, "auth reset user password");
}
