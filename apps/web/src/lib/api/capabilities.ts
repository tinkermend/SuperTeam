import type { ApiClientOptions } from "./client";
import { deleteJson, getJson, postJson } from "./client";

function encodePathSegment(value: string) {
  return encodeURIComponent(value);
}

export type McpAuthStrategy = "none" | "bearer_env" | "headers_env";
export type McpTransport = "http" | "streamable_http";

export type McpServerDefinition = {
  id: string;
  tenant_id: string;
  name: string;
  server_key: string;
  description: string;
  transport: McpTransport;
  url: string;
  auth_strategy: McpAuthStrategy;
  required_env_vars: string[];
  optional_env_vars: string[];
  provider_visibility?: Record<string, boolean>;
  tool_allowlist: string[];
  risk_level: string;
  status: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateMcpServerDefinitionInput = {
  name: string;
  server_key: string;
  description?: string;
  transport: McpTransport;
  url: string;
  auth_strategy?: McpAuthStrategy;
  required_env_vars?: string[];
  optional_env_vars?: string[];
  provider_visibility?: Record<string, boolean>;
  tool_allowlist?: string[];
  risk_level?: string;
};

export type McpBinding = {
  id: string;
  tenant_id: string;
  team_id?: string;
  digital_employee_id?: string;
  mcp_server_id: string;
  server_key?: string;
  server_name?: string;
  url?: string;
  transport?: McpTransport;
  auth_strategy?: McpAuthStrategy;
  credential_env_var?: string;
  required_env_vars?: string[];
  missing_env_vars?: string[];
  source_scope?: string;
  status: string;
  risk_level?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateMcpBindingInput = {
  mcp_server_id: string;
  credential_env_var?: string;
};

export type EffectiveMcpServer = {
  server_id: string;
  server_key: string;
  name: string;
  transport: McpTransport;
  url: string;
  auth_strategy: McpAuthStrategy;
  credential_env_var?: string;
  required_env_vars?: string[];
  missing_env_vars?: string[];
  tool_allowlist?: string[];
  risk_level?: string;
  source_scope: string;
  status: string;
};

export function listMcpServerDefinitions(
  options: ApiClientOptions,
): Promise<McpServerDefinition[]> {
  return getJson<McpServerDefinition[]>(
    options,
    "/api/v1/mcp-servers",
    "mcp server definitions",
  );
}

export function createMcpServerDefinition(
  options: ApiClientOptions,
  input: CreateMcpServerDefinitionInput,
): Promise<McpServerDefinition> {
  return postJson<McpServerDefinition>(
    options,
    "/api/v1/mcp-servers",
    input,
    "create mcp server definition",
  );
}

export function deleteMcpServerDefinition(
  options: ApiClientOptions,
  serverId: string,
): Promise<void> {
  const encodedServerId = encodePathSegment(serverId);

  return deleteJson(
    options,
    `/api/v1/mcp-servers/${encodedServerId}`,
    "delete mcp server definition",
  );
}

export function bindTeamMcpServer(
  options: ApiClientOptions,
  teamId: string,
  input: CreateMcpBindingInput,
): Promise<McpBinding> {
  const encodedTeamId = encodePathSegment(teamId);

  return postJson<McpBinding>(
    options,
    `/api/v1/teams/${encodedTeamId}/mcp-bindings`,
    input,
    "bind team mcp server",
  );
}

export function listTeamMcpBindings(
  options: ApiClientOptions,
  teamId: string,
): Promise<McpBinding[]> {
  const encodedTeamId = encodePathSegment(teamId);

  return getJson<McpBinding[]>(
    options,
    `/api/v1/teams/${encodedTeamId}/mcp-bindings`,
    "team mcp bindings",
  );
}

export function deleteTeamMcpBinding(
  options: ApiClientOptions,
  teamId: string,
  bindingId: string,
): Promise<void> {
  const encodedTeamId = encodePathSegment(teamId);
  const encodedBindingId = encodePathSegment(bindingId);

  return deleteJson(
    options,
    `/api/v1/teams/${encodedTeamId}/mcp-bindings/${encodedBindingId}`,
    "delete team mcp binding",
  );
}

export function bindEmployeeMcpServer(
  options: ApiClientOptions,
  employeeId: string,
  input: CreateMcpBindingInput,
): Promise<McpBinding> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return postJson<McpBinding>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/mcp-bindings-v2`,
    input,
    "bind employee mcp server",
  );
}

export function listEmployeeMcpBindingsV2(
  options: ApiClientOptions,
  employeeId: string,
): Promise<McpBinding[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<McpBinding[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/mcp-bindings-v2`,
    "employee mcp bindings",
  );
}

export function deleteEmployeeMcpBindingV2(
  options: ApiClientOptions,
  employeeId: string,
  bindingId: string,
): Promise<void> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const encodedBindingId = encodePathSegment(bindingId);

  return deleteJson(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/mcp-bindings-v2/${encodedBindingId}`,
    "delete employee mcp binding",
  );
}

export function listEffectiveMcpConfig(
  options: ApiClientOptions,
  employeeId: string,
): Promise<EffectiveMcpServer[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<EffectiveMcpServer[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/effective-mcp-config`,
    "effective mcp config",
  );
}

// ----------------------------------------------------------------------------
// Skill MCP dependency queries
// ----------------------------------------------------------------------------

export interface DependentSkill {
  skill_id: string;
  slug: string;
  name: string;
}

export function listMcpServerDependentSkills(
  options: ApiClientOptions,
  serverId: string,
): Promise<DependentSkill[]> {
  const encodedServerId = encodePathSegment(serverId);
  return getJson<DependentSkill[]>(
    options,
    `/api/v1/mcp-servers/${encodedServerId}/dependent-skills`,
    "mcp server dependent skills",
  );
}

export interface EmployeeSkillMcpDependencyItem {
  mcp_server_id: string;
  server_key: string;
  server_name: string;
  status: "satisfied" | "missing_binding" | "blocked_missing_env";
  missing_env_vars: string[];
}

export interface EmployeeSkillMcpDependencyStatus {
  skill_id: string;
  skill_slug: string;
  dependencies: EmployeeSkillMcpDependencyItem[];
}

export function listEmployeeSkillMcpDependencyStatus(
  options: ApiClientOptions,
  employeeId: string,
): Promise<EmployeeSkillMcpDependencyStatus[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  return getJson<EmployeeSkillMcpDependencyStatus[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/skill-mcp-dependency-status`,
    "employee skill mcp dependency status",
  );
}
