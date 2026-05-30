import type { ApiClientOptions } from "./health";

export type UserSummary = {
  id: string;
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

function buildApiUrl(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/+$/, "")}${path}`;
}

async function parseJson<T>(response: Response, resource: string): Promise<T> {
  if (!response.ok) {
    throw new Error(`${resource} request failed with status ${response.status}`);
  }

  return (await response.json()) as T;
}

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
