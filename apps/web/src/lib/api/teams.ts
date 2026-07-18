import type { ApiClientOptions } from "./client";
import { buildApiUrl, deleteJson, getJson, parseJson, patchJson, postJson } from "./client";

// 团队生命周期收敛：存活团队唯一状态 active；删除进入 pending_delete 待确认态
// （全站不可见），管理员恢复或确认后才物理删除。
export type TeamStatus = "active" | "pending_delete";
export type GovernanceSummaryStatus =
  | "not_configured"
  | "draft_pending"
  | "active"
  | "needs_update";
export type TeamMemberRole =
  | "owner"
  | "admin"
  | "approver"
  | "member"
  | "viewer";
export type AllowedTeamAction =
  | "team.update"
  | "team.delete"
  | "team.member.add"
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
  description?: string;
  status: TeamStatus;
  human_owner_user_ids?: string[];
  human_owners?: TeamHumanOwner[];
  constitution: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export type TeamUserAvatar = {
  options?: Record<string, unknown>;
  provider: "dicebear";
  seed: string;
  style: "adventurer";
};

export type TeamHumanOwner = {
  avatar?: TeamUserAvatar;
  display_name?: string;
  email?: string;
  status: string;
  user_id: string;
  username: string;
};

export type TeamListItem = Team & {
  member_count: number;
  digital_employee_count: number;
  capability_count: number;
  governance_status: GovernanceSummaryStatus;
  current_revision?: number;
  pending_draft_count: number;
  risk_summary: string;
  human_owners?: TeamHumanOwner[];
};

export type TeamOverview = {
  team: Team;
  member_count: number;
  digital_employee_count: number;
  capability_count: number;
  pending_draft_count: number;
  pending_item_count: number;
  allowed_actions: AllowedTeamAction[];
};

export type TeamMember = {
  avatar?: TeamUserAvatar;
  membership_id: string;
  tenant_id: string;
  team_id: string;
  user_id: string;
  username: string;
  display_name: string;
  email: string;
  account_status: string;
  role: TeamMemberRole;
  membership_status: string;
  created_at?: string;
  updated_at?: string;
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
  description?: string;
  human_owner_user_ids: string[];
  initial_members?: InitialTeamMemberInput[];
  initial_digital_employee_ids?: string[];
  status?: TeamStatus;
  metadata?: Record<string, unknown>;
};

export type InitialTeamMemberInput = {
  user_id: string;
  role: Extract<TeamMemberRole, "member" | "viewer">;
};

export type ListTeamSummariesFilters = {
  status?: TeamStatus;
  governance_status?: GovernanceSummaryStatus;
  q?: string;
  limit?: number;
  offset?: number;
};

export type ListTeamAuditEventsFilters = {
  limit?: number;
  offset?: number;
};

export type UpdateTeamInput = {
  slug: string;
  name: string;
  description?: string;
  human_owner_user_ids?: string[];
  metadata?: Record<string, unknown>;
};

export type UpdateTeamConstitutionInput = Record<string, unknown>;

export type AddTeamMemberInput = {
  user_id: string;
  role: Extract<TeamMemberRole, "member" | "viewer">;
};

function teamPath(teamId: string, suffix = ""): string {
  return `/api/v1/teams/${encodeURIComponent(teamId)}${suffix}`;
}

function teamListPath(filters: ListTeamSummariesFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.status) {
    params.set("status", filters.status);
  }
  if (filters.governance_status) {
    params.set("governance_status", filters.governance_status);
  }
  const q = filters.q?.trim();
  if (q) {
    params.set("q", q);
  }
  if (filters.limit !== undefined) {
    params.set("limit", String(filters.limit));
  }
  if (filters.offset !== undefined) {
    params.set("offset", String(filters.offset));
  }
  const query = params.toString();
  return query ? `/api/v1/teams?${query}` : "/api/v1/teams";
}

function teamAuditPath(
  teamId: string,
  filters: ListTeamAuditEventsFilters = {},
): string {
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

export async function listTeamSummaries(
  options: ApiClientOptions,
  filters: ListTeamSummariesFilters = {},
): Promise<TeamListItem[]> {
  return getJson<TeamListItem[]>(
    options,
    teamListPath(filters),
    "team summaries",
  );
}

export function listTeams(options: ApiClientOptions): Promise<TeamListItem[]> {
  return listTeamSummaries(options);
}

export function createTeam(
  options: ApiClientOptions,
  input: CreateTeamInput,
): Promise<TeamOverview> {
  return postJson<TeamOverview>(options, "/api/v1/teams", input, "create team");
}

export function getTeamOverview(
  options: ApiClientOptions,
  teamId: string,
): Promise<TeamOverview> {
  return getJson<TeamOverview>(
    options,
    teamPath(teamId, "/overview"),
    "team overview",
  );
}

export function updateTeam(
  options: ApiClientOptions,
  teamId: string,
  input: UpdateTeamInput,
): Promise<Team> {
  return patchJson<Team>(options, teamPath(teamId), input, "update team");
}

export function deleteTeam(
  options: ApiClientOptions,
  teamId: string,
): Promise<void> {
  return deleteJson(options, teamPath(teamId), "delete team");
}

export type PendingDeleteTeam = Team & {
  deleted_at: string;
  delete_requested_by?: string;
};

export function listPendingDeleteTeams(
  options: ApiClientOptions,
): Promise<PendingDeleteTeam[]> {
  return getJson<PendingDeleteTeam[]>(options, "/api/v1/teams/pending-deletes", "pending delete teams");
}

export function restorePendingDeleteTeam(
  options: ApiClientOptions,
  teamId: string,
): Promise<Team> {
  return postJson<Team>(options, teamPath(teamId, "/restore"), {}, "restore pending delete team");
}

export async function confirmTeamDelete(
  options: ApiClientOptions,
  teamId: string,
): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, teamPath(teamId, "/confirm-delete")), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "POST",
  });
  if (!response.ok) {
    await parseJson<unknown>(response, "confirm team delete");
  }
}

export function updateTeamConstitution(
  options: ApiClientOptions,
  teamId: string,
  input: UpdateTeamConstitutionInput,
): Promise<Team> {
  const encodedTeamId = encodeURIComponent(teamId);
  return patchJson<Team>(
    options,
    `/api/v1/teams/${encodedTeamId}/constitution`,
    input,
    "update team constitution",
  );
}

export function listTeamMembers(
  options: ApiClientOptions,
  teamId: string,
): Promise<TeamMember[]> {
  return getJson<TeamMember[]>(
    options,
    teamPath(teamId, "/members"),
    "team members",
  );
}

export function addTeamMember(
  options: ApiClientOptions,
  teamId: string,
  input: AddTeamMemberInput,
): Promise<TeamMember> {
  return postJson<TeamMember>(
    options,
    teamPath(teamId, "/members"),
    input,
    "add team member",
  );
}

export type BindTeamDigitalEmployeeResult = {
  digital_employee_id: string;
  team_id: string;
};

/** 收编候岗（无归属）数字员工进本团队；已有归属的员工会被 400 拒绝。 */
export function bindTeamDigitalEmployee(
  options: ApiClientOptions,
  teamId: string,
  digitalEmployeeId: string,
): Promise<BindTeamDigitalEmployeeResult> {
  return postJson<BindTeamDigitalEmployeeResult>(
    options,
    teamPath(teamId, "/digital-employees"),
    { digital_employee_id: digitalEmployeeId },
    "bind team digital employee",
  );
}

export function removeTeamMember(
  options: ApiClientOptions,
  teamId: string,
  memberId: string,
): Promise<void> {
  return deleteJson(
    options,
    teamPath(teamId, `/members/${encodeURIComponent(memberId)}`),
    "remove team member",
  );
}

export function listTeamAuditEvents(
  options: ApiClientOptions,
  teamId: string,
  filters: ListTeamAuditEventsFilters = {},
): Promise<TeamAuditEvent[]> {
  return getJson<TeamAuditEvent[]>(
    options,
    teamAuditPath(teamId, filters),
    "team audit events",
  );
}
