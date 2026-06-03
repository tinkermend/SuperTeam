import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type TeamStatus = "active" | "disabled" | "archived";
export type TeamConfigRevisionStatus = "draft" | "active";
export type GovernanceSummaryStatus = "not_configured" | "draft_pending" | "active" | "needs_update";
export type AllowedTeamAction =
  | "team.update"
  | "team.disable"
  | "team.archive"
  | "team.restore"
  | "team.member.add"
  | "team.member.request_privileged_role"
  | "team.governance.edit"
  | "team.governance.approve"
  | "team.capability.bind"
  | "team.capability.unbind"
  | "team.audit.read";

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

export type TeamListItem = Team & {
  member_count: number;
  digital_employee_count: number;
  capability_count: number;
  governance_status: GovernanceSummaryStatus;
  current_revision?: number;
  pending_draft_count: number;
  risk_summary: string;
};

export type TeamOverview = {
  team: Team;
  member_count: number;
  digital_employee_count: number;
  capability_count: number;
  current_revision?: TeamConfigRevision;
  pending_draft_count: number;
  pending_item_count: number;
  allowed_actions: AllowedTeamAction[];
};

export type TeamAuditEvent = {
  id: string;
  tenant_id: string;
  event_type: string;
  actor_type: string;
  actor_id: string;
  resource_type: string;
  resource_id: string;
  action: string;
  details: Record<string, unknown>;
  ip_address?: string;
  created_at?: string;
};

export type CreateTeamInput = {
  slug: string;
  name: string;
  human_owner_user_id: string;
  status?: TeamStatus;
  metadata?: Record<string, unknown>;
};

export type ListTeamSummariesFilters = {
  status?: TeamStatus;
  q?: string;
};

export type ListTeamAuditEventsFilters = {
  limit?: number;
  offset?: number;
};

export type UpdateTeamInput = {
  slug: string;
  name: string;
  human_owner_user_id?: string;
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

async function patchJson<T>(options: ApiClientOptions, path: string, input: unknown, resource: string): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "PATCH",
  });

  return parseJson<T>(response, resource);
}

function teamPath(teamId: string, suffix = ""): string {
  return `/api/v1/teams/${encodeURIComponent(teamId)}${suffix}`;
}

function teamListPath(filters: ListTeamSummariesFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.status) {
    params.set("status", filters.status);
  }
  const q = filters.q?.trim();
  if (q) {
    params.set("q", q);
  }
  const query = params.toString();
  return query ? `/api/v1/teams?${query}` : "/api/v1/teams";
}

function teamAuditPath(teamId: string, filters: ListTeamAuditEventsFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.limit !== undefined) {
    params.set("limit", String(filters.limit));
  }
  if (filters.offset !== undefined) {
    params.set("offset", String(filters.offset));
  }
  const query = params.toString();
  return `${teamPath(teamId, "/audit")}${query ? `?${query}` : ""}`;
}

export function listTeamSummaries(
  options: ApiClientOptions,
  filters: ListTeamSummariesFilters = {},
): Promise<TeamListItem[]> {
  return getJson<TeamListItem[]>(options, teamListPath(filters), "team summaries");
}

export function listTeams(options: ApiClientOptions): Promise<TeamListItem[]> {
  return listTeamSummaries(options);
}

export function createTeam(options: ApiClientOptions, input: CreateTeamInput): Promise<Team> {
  return postJson<Team>(options, "/api/v1/teams", input, "create team");
}

export function getTeamOverview(options: ApiClientOptions, teamId: string): Promise<TeamOverview> {
  return getJson<TeamOverview>(options, teamPath(teamId, "/overview"), "team overview");
}

export function updateTeam(options: ApiClientOptions, teamId: string, input: UpdateTeamInput): Promise<Team> {
  return patchJson<Team>(options, teamPath(teamId), input, "update team");
}

export function disableTeam(options: ApiClientOptions, teamId: string): Promise<Team> {
  return postJson<Team>(options, teamPath(teamId, "/disable"), {}, "disable team");
}

export function archiveTeam(options: ApiClientOptions, teamId: string): Promise<Team> {
  return postJson<Team>(options, teamPath(teamId, "/archive"), {}, "archive team");
}

export function restoreTeam(options: ApiClientOptions, teamId: string): Promise<Team> {
  return postJson<Team>(options, teamPath(teamId, "/restore"), {}, "restore team");
}

export function createTeamConfigRevision(
  options: ApiClientOptions,
  teamId: string,
  input: CreateTeamConfigRevisionInput,
): Promise<TeamConfigRevision> {
  return postJson<TeamConfigRevision>(
    options,
    teamPath(teamId, "/config-revisions"),
    input,
    "create team config revision",
  );
}

export function getCurrentTeamConfigRevision(
  options: ApiClientOptions,
  teamId: string,
): Promise<TeamConfigRevision> {
  return getJson<TeamConfigRevision>(
    options,
    teamPath(teamId, "/config-revisions/current"),
    "current team config revision",
  );
}

export function listTeamAuditEvents(
  options: ApiClientOptions,
  teamId: string,
  filters: ListTeamAuditEventsFilters = {},
): Promise<TeamAuditEvent[]> {
  return getJson<TeamAuditEvent[]>(options, teamAuditPath(teamId, filters), "team audit events");
}
