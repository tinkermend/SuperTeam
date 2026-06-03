import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type TeamStatus = "active" | "disabled" | "archived";
export type TeamConfigRevisionStatus = "draft" | "active" | "rejected" | "archived";
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

export type GovernanceDraftInput = {
  human_owner_user_id?: string;
  constitution?: Record<string, unknown>;
  capability_policy?: Record<string, unknown>;
  context_policy?: Record<string, unknown>;
  approval_policy?: Record<string, unknown>;
  artifact_contract?: Record<string, unknown>;
  internal_collaboration_policy?: Record<string, unknown>;
  runtime_scope_policy?: Record<string, unknown>;
};

export type GovernanceValidationIssue = {
  field: string;
  message: string;
  severity: string;
};

export type GovernanceDiffSummary = {
  added_hard_rules: number;
  changed_capabilities: number;
  changed_approval_rules: number;
  warnings: GovernanceValidationIssue[];
  blocking_errors: GovernanceValidationIssue[];
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

export function getCurrentTeamGovernance(options: ApiClientOptions, teamId: string): Promise<TeamConfigRevision> {
  return getJson<TeamConfigRevision>(options, teamPath(teamId, "/governance/current"), "current team governance");
}

export function listTeamGovernanceDrafts(options: ApiClientOptions, teamId: string): Promise<TeamConfigRevision[]> {
  return getJson<TeamConfigRevision[]>(options, teamPath(teamId, "/governance/drafts"), "team governance drafts");
}

export function createTeamGovernanceDraft(
  options: ApiClientOptions,
  teamId: string,
  input: GovernanceDraftInput,
): Promise<TeamConfigRevision> {
  return postJson<TeamConfigRevision>(options, teamPath(teamId, "/governance/drafts"), input, "create governance draft");
}

export function updateTeamGovernanceDraft(
  options: ApiClientOptions,
  teamId: string,
  draftId: string,
  input: GovernanceDraftInput,
): Promise<TeamConfigRevision> {
  return patchJson<TeamConfigRevision>(
    options,
    teamPath(teamId, `/governance/drafts/${encodeURIComponent(draftId)}`),
    input,
    "update governance draft",
  );
}

export function approveTeamGovernanceDraft(
  options: ApiClientOptions,
  teamId: string,
  draftId: string,
): Promise<TeamConfigRevision> {
  return postJson<TeamConfigRevision>(
    options,
    teamPath(teamId, `/governance/drafts/${encodeURIComponent(draftId)}/approve`),
    {},
    "approve governance draft",
  );
}

export function rejectTeamGovernanceDraft(
  options: ApiClientOptions,
  teamId: string,
  draftId: string,
): Promise<TeamConfigRevision> {
  return postJson<TeamConfigRevision>(
    options,
    teamPath(teamId, `/governance/drafts/${encodeURIComponent(draftId)}/reject`),
    {},
    "reject governance draft",
  );
}

export function previewTeamGovernanceDiff(
  options: ApiClientOptions,
  teamId: string,
  draftId: string,
): Promise<GovernanceDiffSummary> {
  return getJson<GovernanceDiffSummary>(
    options,
    teamPath(teamId, `/governance/drafts/${encodeURIComponent(draftId)}/diff`),
    "preview governance diff",
  );
}
