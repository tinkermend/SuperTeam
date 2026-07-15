import { ApiRequestError, type ApiClientOptions } from "./client";
import { buildApiUrl, parseJson, getJson, putJson } from "./client";

export type SkillTeamBinding = {
  team_id: string;
  team_name: string;
};

export type SkillAgentBinding = {
  agent_id: string;
  agent_name: string;
  team_id?: string;
  team_name?: string;
  status: string;
};

export type SkillRuntimeDependencies = {
  tools: string[];
  env: string[];
};

export type SkillRuntimeDependencyStatus = {
  load_status?: string;
  missing_tools?: string[];
  missing_env?: string[];
};

export type Skill = {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  description: string;
  version: string;
  source: string;
  risk_level: string;
  icon_key: string;
  color_token: string;
  tags: string[];
  archive_object_ref: string;
  archive_filename: string;
  archive_size_bytes: number;
  archive_checksum_sha256: string;
  archive_file_count: number;
  created_by: string;
  created_by_name: string;
  team_bindings: SkillTeamBinding[];
  agent_bindings: SkillAgentBinding[];
  runtime_dependencies?: SkillRuntimeDependencies;
  created_at?: string;
  updated_at?: string;
};

export type EffectiveEmployeeSkill = {
  skill: Skill;
  source_scope: "team" | "employee";
  inherited: boolean;
  read_only: boolean;
  runtime_dependency_status?: SkillRuntimeDependencyStatus;
};

export type ListSkillsFilters = {
  q?: string;
};

export type UploadSkillInput = {
  description?: string;
  file: File;
  name?: string;
  risk_level?: string;
  runtime_dependencies?: SkillRuntimeDependencies;
  tags?: string[];
};

export type SkillInstallTargetScope = "team" | "employee";

export type InstallSkillInput = {
  target_scope: SkillInstallTargetScope;
  team_id?: string;
  digital_employee_id?: string;
  timeout_sec?: number;
};

export type SkillInstallBlockedTarget = {
  digital_employee_id?: string;
  employee_name?: string;
  provider_type?: string;
  runtime_node_id?: string;
  node_id?: string;
  reason_code: string;
  message: string;
};

export type SkillInstallation = {
  id?: string;
  tenant_id?: string;
  skill_id?: string;
  target_scope?: SkillInstallTargetScope;
  team_id?: string;
  digital_employee_id?: string;
  employee_name?: string;
  runtime_node_id?: string;
  node_id?: string;
  provider_type: "opencode" | "codex" | "claude-code";
  installed_path: string;
  archive_checksum_sha256?: string;
  installed_by?: string;
  installed_at?: string;
  metadata?: Record<string, unknown>;
};

export type InstallSkillResult = {
  skill_id: string;
  target_scope: SkillInstallTargetScope;
  team_id?: string;
  digital_employee_id?: string;
  installed_count: number;
  installations: SkillInstallation[];
  blocked_targets?: SkillInstallBlockedTarget[];
};

type InstallSkillErrorResponse = {
  error?: unknown;
  phase?: unknown;
  message?: unknown;
  blocked_targets?: unknown;
};

export class InstallSkillError extends ApiRequestError {
  readonly code: string;
  readonly phase?: string;
  readonly blockedTargets: SkillInstallBlockedTarget[];

  constructor(status: number, response: InstallSkillErrorResponse) {
    const message = typeof response.message === "string" && response.message ? response.message : undefined;
    super("install skill", status, message);
    this.name = "InstallSkillError";
    if (message) {
      this.message = message;
    }
    this.code = typeof response.error === "string" && response.error ? response.error : "skill_install_failed";
    this.phase = typeof response.phase === "string" && response.phase ? response.phase : undefined;
    this.blockedTargets = parseBlockedTargets(response.blocked_targets);
  }
}

export async function deleteSkill(
  options: ApiClientOptions,
  skillId: string,
): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const encodedSkillId = encodeURIComponent(skillId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/skills/${encodedSkillId}`), {
    credentials: "include",
    method: "DELETE",
  });

  if (!response.ok) {
    await parseJson<unknown>(response, "delete skill");
  }
}

export async function listSkills(
  options: ApiClientOptions,
  filters: ListSkillsFilters = {},
): Promise<Skill[]> {
  const searchParams = new URLSearchParams();
  if (filters.q?.trim()) {
    searchParams.set("q", filters.q.trim());
  }
  const query = searchParams.toString();
  const path = `/api/v1/skills${query ? `?${query}` : ""}`;
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<Skill[]>(response, "skills");
}

export async function getSkill(
  options: ApiClientOptions,
  skillId: string,
): Promise<Skill> {
  const fetcher = options.fetcher ?? fetch;
  const encodedSkillId = encodeURIComponent(skillId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/skills/${encodedSkillId}`), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<Skill>(response, "skill detail");
}

export async function uploadSkill(
  options: ApiClientOptions,
  input: UploadSkillInput,
): Promise<Skill> {
  const formData = new FormData();
  formData.set("file", input.file);
  const name = input.name?.trim();
  const description = input.description?.trim();
  const runtimeTools = cleanUploadList(input.runtime_dependencies?.tools);
  const runtimeEnv = cleanUploadList(input.runtime_dependencies?.env);
  const tags = cleanUploadList(input.tags);
  if (name) {
    formData.set("name", name);
  }
  if (description) {
    formData.set("description", description);
  }
  if (input.risk_level) {
    formData.set("risk_level", input.risk_level);
  }
  if (tags.length) {
    formData.set("tags", tags.join(","));
  }
  if (runtimeTools.length) {
    formData.set("runtime_tools", runtimeTools.join(","));
  }
  if (runtimeEnv.length) {
    formData.set("runtime_env", runtimeEnv.join(","));
  }
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/skills/uploads"), {
    body: formData,
    credentials: "include",
    method: "POST",
  });

  return parseJson<Skill>(response, "upload skill");
}

export async function installSkill(
  options: ApiClientOptions,
  skillId: string,
  input: InstallSkillInput,
): Promise<InstallSkillResult> {
  const fetcher = options.fetcher ?? fetch;
  const encodedSkillId = encodeURIComponent(skillId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/skills/${encodedSkillId}/install`), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  if (!response.ok) {
    await parseInstallSkillError(response);
  }

  return parseJson<InstallSkillResult>(response, "install skill");
}

export async function listSkillInstallations(
  options: ApiClientOptions,
  skillId: string,
): Promise<SkillInstallation[]> {
  const fetcher = options.fetcher ?? fetch;
  const encodedSkillId = encodeURIComponent(skillId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/skills/${encodedSkillId}/installations`), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<SkillInstallation[]>(response, "skill installations");
}

async function parseInstallSkillError(response: Response): Promise<never> {
  const contentType = response.headers.get("content-type") ?? "";
  if (response.status === 409 && contentType.includes("application/json")) {
    const parsed = (await response.clone().json()) as InstallSkillErrorResponse;
    if (parsed.error === "skill_install_failed") {
      throw new InstallSkillError(response.status, parsed);
    }
  }

  await parseJson<unknown>(response, "install skill");
  throw new ApiRequestError("install skill", response.status);
}

function parseBlockedTargets(value: unknown): SkillInstallBlockedTarget[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item): SkillInstallBlockedTarget | undefined => {
      if (!item || typeof item !== "object") {
        return undefined;
      }
      const record = item as Record<string, unknown>;
      const reasonCode = stringValue(record.reason_code);
      const message = stringValue(record.message);
      if (!reasonCode || !message) {
        return undefined;
      }
      return {
        digital_employee_id: stringValue(record.digital_employee_id),
        employee_name: stringValue(record.employee_name),
        provider_type: stringValue(record.provider_type),
        runtime_node_id: stringValue(record.runtime_node_id),
        node_id: stringValue(record.node_id),
        reason_code: reasonCode,
        message,
      };
    })
    .filter((item): item is SkillInstallBlockedTarget => Boolean(item));
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

function cleanUploadList(items?: string[]): string[] {
  return items?.map((item) => item.trim()).filter(Boolean) ?? [];
}

export async function listTeamSkills(
  options: ApiClientOptions,
  teamId: string,
): Promise<Skill[]> {
  const fetcher = options.fetcher ?? fetch;
  const encodedTeamId = encodeURIComponent(teamId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/teams/${encodedTeamId}/skills`), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<Skill[]>(response, "team skills");
}

export async function bindTeamSkill(
  options: ApiClientOptions,
  teamId: string,
  skillId: string,
): Promise<Skill> {
  const fetcher = options.fetcher ?? fetch;
  const encodedTeamId = encodeURIComponent(teamId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/teams/${encodedTeamId}/skills`), {
    body: JSON.stringify({ skill_id: skillId }),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  return parseJson<Skill>(response, "bind team skill");
}

export async function unbindTeamSkill(
  options: ApiClientOptions,
  teamId: string,
  skillId: string,
): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const encodedTeamId = encodeURIComponent(teamId);
  const encodedSkillId = encodeURIComponent(skillId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/teams/${encodedTeamId}/skills/${encodedSkillId}`), {
    credentials: "include",
    method: "DELETE",
  });

  if (!response.ok) {
    await parseJson<unknown>(response, "unbind team skill");
  }
}

export async function listEmployeeSkills(
  options: ApiClientOptions,
  employeeId: string,
): Promise<EffectiveEmployeeSkill[]> {
  const fetcher = options.fetcher ?? fetch;
  const encodedEmployeeId = encodeURIComponent(employeeId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodedEmployeeId}/skills`), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<EffectiveEmployeeSkill[]>(response, "employee skills");
}

export async function bindEmployeeSkill(
  options: ApiClientOptions,
  employeeId: string,
  skillId: string,
): Promise<Skill> {
  const fetcher = options.fetcher ?? fetch;
  const encodedEmployeeId = encodeURIComponent(employeeId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodedEmployeeId}/skills`), {
    body: JSON.stringify({ skill_id: skillId }),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  return parseJson<Skill>(response, "bind employee skill");
}

export async function unbindEmployeeSkill(
  options: ApiClientOptions,
  employeeId: string,
  skillId: string,
): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const encodedEmployeeId = encodeURIComponent(employeeId);
  const encodedSkillId = encodeURIComponent(skillId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodedEmployeeId}/skills/${encodedSkillId}`), {
    credentials: "include",
    method: "DELETE",
  });

  if (!response.ok) {
    await parseJson<unknown>(response, "unbind employee skill");
  }
}

// ----------------------------------------------------------------------------
// Skill MCP dependency management
// ----------------------------------------------------------------------------

export interface SkillMcpDependency {
  id: string;
  skill_id: string;
  mcp_server_id: string;
  note: string;
  server_key: string;
  server_name: string;
  auth_strategy: string;
  risk_level: string;
  server_status: string;
  created_at: string;
}

export function listSkillMcpDependencies(
  options: ApiClientOptions,
  skillId: string,
): Promise<SkillMcpDependency[]> {
  return getJson<SkillMcpDependency[]>(
    options,
    `/api/v1/skills/${encodeURIComponent(skillId)}/mcp-dependencies`,
    "skill mcp dependencies",
  );
}

export function replaceSkillMcpDependencies(
  options: ApiClientOptions,
  skillId: string,
  input: { items: Array<{ mcp_server_id: string; note?: string }> },
): Promise<SkillMcpDependency[]> {
  return putJson<SkillMcpDependency[]>(
    options,
    `/api/v1/skills/${encodeURIComponent(skillId)}/mcp-dependencies`,
    input,
    "replace skill mcp dependencies",
  );
}
