import type { ApiClientOptions } from "./client";
import { ApiRequestError, buildApiUrl, parseJson } from "./client";

export { ApiRequestError };

export type UserSummary = {
  id: number;
  status: "active" | "disabled";
  username: string;
};

export type LoginRequest = {
  password: string;
  username: string;
};

export type LoginResponse = {
  user: UserSummary;
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
  status?: UserSummary["status"];
};

export type CreateUserRequest = {
  password: string;
  username: string;
};

export type LoginLogEventType = "login_succeeded" | "login_failed" | "logout_succeeded";

export type LoginLogResult = "succeeded" | "failed";

export type LoginLogRecord = {
  client_ip: string | null;
  created_at: string;
  event_type: LoginLogEventType;
  failure_reason: string | null;
  id: number;
  result: LoginLogResult;
  session_id: string | null;
  user_agent: string | null;
  user_id: number | null;
  username: string;
};

export type LoginLogListResponse = {
  items: LoginLogRecord[];
};

export type ListLoginLogsOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
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

export async function listUsers(options: ListUsersOptions): Promise<UserListResponse> {
  const fetcher = options.fetcher ?? fetch;
  const params = new URLSearchParams();
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

export async function updateUserStatus(
  options: ApiClientOptions,
  userID: number,
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

export async function resetUserPassword(options: ApiClientOptions, userID: number, password: string): Promise<UserResponse> {
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
