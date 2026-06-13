import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type CredentialType = "mcp_token";

export type UserCredential = {
  id: string;
  tenant_id: string;
  user_id: string;
  name: string;
  credential_type: CredentialType;
  last_four: string;
  status: string;
  disabled_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateUserCredentialInput = {
  name: string;
  credential_type: CredentialType;
  credential_value: string;
};

export type McpServer = {
  id: string;
  tenant_id: string;
  team_id?: string;
  digital_employee_id?: string;
  name: string;
  url: string;
  credential_id?: string;
  credential_name?: string;
  credential_type?: CredentialType;
  credential_last_four?: string;
  status: string;
  source_scope: "team" | "employee";
  inherited: boolean;
  created_by?: string;
  disabled_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateMcpServerInput = {
  name: string;
  url: string;
  credential_id?: string;
};

async function getJson<T>(
  options: ApiClientOptions,
  path: string,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<T>(response, resource);
}

async function postJson<T>(
  options: ApiClientOptions,
  path: string,
  input: unknown,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  return parseJson<T>(response, resource);
}

async function deleteJson(
  options: ApiClientOptions,
  path: string,
  resource: string,
): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    method: "DELETE",
  });

  if (!response.ok) {
    await parseJson<unknown>(response, resource);
  }
}

function encodePathSegment(value: string) {
  return encodeURIComponent(value);
}

export function listUserCredentials(
  options: ApiClientOptions,
  credentialType?: CredentialType,
): Promise<UserCredential[]> {
  const searchParams = new URLSearchParams();
  if (credentialType) {
    searchParams.set("credential_type", credentialType);
  }
  const query = searchParams.toString();

  return getJson<UserCredential[]>(
    options,
    `/api/v1/user-credentials${query ? `?${query}` : ""}`,
    "user credentials",
  );
}

export function createUserCredential(
  options: ApiClientOptions,
  input: CreateUserCredentialInput,
): Promise<UserCredential> {
  return postJson<UserCredential>(
    options,
    "/api/v1/user-credentials",
    input,
    "create user credential",
  );
}

export function listTeamMcpServers(
  options: ApiClientOptions,
  teamId: string,
): Promise<McpServer[]> {
  const encodedTeamId = encodePathSegment(teamId);

  return getJson<McpServer[]>(
    options,
    `/api/v1/teams/${encodedTeamId}/mcp-servers`,
    "team mcp servers",
  );
}

export function createTeamMcpServer(
  options: ApiClientOptions,
  teamId: string,
  input: CreateMcpServerInput,
): Promise<McpServer> {
  const encodedTeamId = encodePathSegment(teamId);

  return postJson<McpServer>(
    options,
    `/api/v1/teams/${encodedTeamId}/mcp-servers`,
    input,
    "create team mcp server",
  );
}

export function deleteTeamMcpServer(
  options: ApiClientOptions,
  teamId: string,
  serverId: string,
): Promise<void> {
  const encodedTeamId = encodePathSegment(teamId);
  const encodedServerId = encodePathSegment(serverId);

  return deleteJson(
    options,
    `/api/v1/teams/${encodedTeamId}/mcp-servers/${encodedServerId}`,
    "delete team mcp server",
  );
}

export function listEmployeeMcpBindings(
  options: ApiClientOptions,
  employeeId: string,
): Promise<McpServer[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<McpServer[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/mcp-bindings`,
    "employee mcp bindings",
  );
}

export function createEmployeeMcpBinding(
  options: ApiClientOptions,
  employeeId: string,
  input: CreateMcpServerInput,
): Promise<McpServer> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return postJson<McpServer>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/mcp-bindings`,
    input,
    "create employee mcp binding",
  );
}

export function deleteEmployeeMcpBinding(
  options: ApiClientOptions,
  employeeId: string,
  bindingId: string,
): Promise<void> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const encodedBindingId = encodePathSegment(bindingId);

  return deleteJson(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/mcp-bindings/${encodedBindingId}`,
    "delete employee mcp binding",
  );
}

export function listEffectiveMcpServers(
  options: ApiClientOptions,
  employeeId: string,
): Promise<McpServer[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<McpServer[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/effective-mcp-servers`,
    "effective mcp servers",
  );
}
