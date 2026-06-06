import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type SkillStatus = "installed" | "available";

export type SkillFileType = "file";

export type SkillFile = {
  id?: string;
  path: string;
  file_type: SkillFileType;
  content?: string;
  size_bytes: number;
  checksum_sha256?: string;
  updated_at?: string;
};

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

export type Skill = {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  description: string;
  version: string;
  source: string;
  risk_level: string;
  status: SkillStatus;
  icon_key: string;
  color_token: string;
  tags: string[];
  files: SkillFile[];
  team_bindings: SkillTeamBinding[];
  agent_bindings: SkillAgentBinding[];
  created_at?: string;
  updated_at?: string;
};

export type ListSkillsFilters = {
  q?: string;
  status?: SkillStatus;
};

export type UploadSkillInput = {
  description?: string;
  file: File;
  name: string;
  risk_level?: string;
  tags?: string[];
  team_ids?: string[];
};

export async function listSkills(
  options: ApiClientOptions,
  filters: ListSkillsFilters = {},
): Promise<Skill[]> {
  const searchParams = new URLSearchParams();
  if (filters.status) {
    searchParams.set("status", filters.status);
  }
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
  if (input.team_ids?.length) {
    formData.set("team_ids", input.team_ids.join(","));
  }
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/skills/uploads"), {
    body: formData,
    credentials: "include",
    method: "POST",
  });

  return parseJson<Skill>(response, "upload skill");
}

export async function updateSkillFile(
  options: ApiClientOptions,
  skillId: string,
  filePath: string,
  content: string,
): Promise<SkillFile> {
  const fetcher = options.fetcher ?? fetch;
  const encodedPath = filePath.split("/").map(encodeURIComponent).join("/");
  const encodedSkillId = encodeURIComponent(skillId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/skills/${encodedSkillId}/files/${encodedPath}`), {
    body: JSON.stringify({ content }),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "PUT",
  });

  return parseJson<SkillFile>(response, "update skill file");
}
