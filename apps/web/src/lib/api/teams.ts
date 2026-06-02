import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type TeamStatus = "active" | "disabled";
export type TeamConfigRevisionStatus = "draft" | "active";

export type Team = {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  status: TeamStatus;
  human_owner_user_id?: string;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export type TeamConfigRevision = {
  id: string;
  tenant_id: string;
  team_id: string;
  revision_number: number;
  constitution: Record<string, unknown>;
  capability_policy: Record<string, unknown>;
  context_policy: Record<string, unknown>;
  approval_policy: Record<string, unknown>;
  artifact_contract: Record<string, unknown>;
  internal_collaboration_policy: Record<string, unknown>;
  runtime_scope_policy: Record<string, unknown>;
  human_owner_user_id?: string;
  status: TeamConfigRevisionStatus;
  approved_by?: string;
  approved_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateTeamInput = {
  slug: string;
  name: string;
  human_owner_user_id: string;
  status?: TeamStatus;
  metadata?: Record<string, unknown>;
};

export type CreateTeamConfigRevisionInput = {
  human_owner_user_id: string;
  constitution?: Record<string, unknown>;
  capability_policy?: Record<string, unknown>;
  context_policy?: Record<string, unknown>;
  approval_policy?: Record<string, unknown>;
  artifact_contract?: Record<string, unknown>;
  internal_collaboration_policy?: Record<string, unknown>;
  runtime_scope_policy?: Record<string, unknown>;
  status?: TeamConfigRevisionStatus;
};

async function getJson<T>(options: ApiClientOptions, path: string, resource: string): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<T>(response, resource);
}

async function postJson<T>(options: ApiClientOptions, path: string, input: unknown, resource: string): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  return parseJson<T>(response, resource);
}

export function listTeams(options: ApiClientOptions): Promise<Team[]> {
  return getJson<Team[]>(options, "/api/v1/teams", "teams");
}

export function createTeam(options: ApiClientOptions, input: CreateTeamInput): Promise<Team> {
  return postJson<Team>(options, "/api/v1/teams", input, "create team");
}

export function createTeamConfigRevision(
  options: ApiClientOptions,
  teamId: string,
  input: CreateTeamConfigRevisionInput,
): Promise<TeamConfigRevision> {
  const encodedTeamId = encodeURIComponent(teamId);
  return postJson<TeamConfigRevision>(
    options,
    `/api/v1/teams/${encodedTeamId}/config-revisions`,
    input,
    "create team config revision",
  );
}

export function getCurrentTeamConfigRevision(
  options: ApiClientOptions,
  teamId: string,
): Promise<TeamConfigRevision> {
  const encodedTeamId = encodeURIComponent(teamId);
  return getJson<TeamConfigRevision>(
    options,
    `/api/v1/teams/${encodedTeamId}/config-revisions/current`,
    "current team config revision",
  );
}
