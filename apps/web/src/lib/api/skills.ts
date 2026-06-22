import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

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
  name: string;
  risk_level?: string;
  runtime_dependencies?: SkillRuntimeDependencies;
  tags?: string[];
};

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

export async function uploadSkill(
  options: ApiClientOptions,
  input: UploadSkillInput,
): Promise<Skill> {
  const formData = new FormData();
  formData.set("file", input.file);
  formData.set("name", input.name);
  if (input.description) {
    formData.set("description", input.description);
  }
  if (input.risk_level) {
    formData.set("risk_level", input.risk_level);
  }
  if (input.tags?.length) {
    formData.set("tags", input.tags.join(","));
  }
  if (input.runtime_dependencies?.tools?.length) {
    formData.set("runtime_tools", input.runtime_dependencies.tools.join(","));
  }
  if (input.runtime_dependencies?.env?.length) {
    formData.set("runtime_env", input.runtime_dependencies.env.join(","));
  }
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/skills/uploads"), {
    body: formData,
    credentials: "include",
    method: "POST",
  });

  return parseJson<Skill>(response, "upload skill");
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
