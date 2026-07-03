import type { ApiClientOptions } from "./client";
import { deleteJson, getJson, patchJson, postJson, putJson } from "./client";

export type TeamStatus = "active" | "disabled" | "archived";
export type TeamConfigRevisionStatus =
  | "draft"
  | "active"
  | "rejected"
  | "archived";
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
export type TeamMemberRoleRequestStatus = "pending" | "approved" | "rejected";
export type AllowedTeamAction =
  | "team.update"
  | "team.disable"
  | "team.archive"
  | "team.restore"
  | "team.delete"
  | "team.member.add"
  | "team.member.request_privileged_role"
  | "team.governance.edit"
  | "team.governance.approve"
  | "team.capability.bind"
  | "team.capability.unbind"
  | "team.audit.read"
  | "team.lending.policy.read"
  | "team.lending.policy.edit"
  | "team.lending.request.read"
  | "team.lending.request.decide";

export type Team = {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  status: TeamStatus;
  human_owner_user_ids?: string[];
  human_owners?: TeamHumanOwner[];
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
  human_owner_user_ids?: string[];
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
  human_owners?: TeamHumanOwner[];
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

export type TeamMemberRoleRequest = {
  id: string;
  tenant_id: string;
  team_id: string;
  target_user_id: string;
  requested_role: TeamMemberRole;
  requested_by: string;
  status: TeamMemberRoleRequestStatus;
  reason: string;
  decided_by?: string;
  decided_at?: string;
  decision_reason: string;
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
  human_owner_user_ids?: string[];
  metadata?: Record<string, unknown>;
};

export type CreateTeamConfigRevisionInput = {
  human_owner_user_ids: string[];
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
  human_owner_user_ids?: string[];
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

export type AddTeamMemberInput = {
  user_id: string;
  role: Extract<TeamMemberRole, "member" | "viewer">;
};

export type CreateTeamMemberRoleRequestInput = {
  target_user_id: string;
  requested_role: Extract<TeamMemberRole, "owner" | "admin" | "approver">;
  reason: string;
};

export type DecideTeamMemberRoleRequestInput = {
  decision_reason?: string;
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

export function disableTeam(
  options: ApiClientOptions,
  teamId: string,
): Promise<Team> {
  return postJson<Team>(
    options,
    teamPath(teamId, "/disable"),
    {},
    "disable team",
  );
}

export function archiveTeam(
  options: ApiClientOptions,
  teamId: string,
): Promise<Team> {
  return postJson<Team>(
    options,
    teamPath(teamId, "/archive"),
    {},
    "archive team",
  );
}

export function restoreTeam(
  options: ApiClientOptions,
  teamId: string,
): Promise<Team> {
  return postJson<Team>(
    options,
    teamPath(teamId, "/restore"),
    {},
    "restore team",
  );
}

export function deleteTeam(
  options: ApiClientOptions,
  teamId: string,
): Promise<void> {
  return deleteJson(options, teamPath(teamId), "delete team");
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

export function getCurrentTeamGovernance(
  options: ApiClientOptions,
  teamId: string,
): Promise<TeamConfigRevision> {
  return getJson<TeamConfigRevision>(
    options,
    teamPath(teamId, "/governance/current"),
    "current team governance",
  );
}

export function listTeamGovernanceDrafts(
  options: ApiClientOptions,
  teamId: string,
): Promise<TeamConfigRevision[]> {
  return getJson<TeamConfigRevision[]>(
    options,
    teamPath(teamId, "/governance/drafts"),
    "team governance drafts",
  );
}

export function createTeamGovernanceDraft(
  options: ApiClientOptions,
  teamId: string,
  input: GovernanceDraftInput,
): Promise<TeamConfigRevision> {
  return postJson<TeamConfigRevision>(
    options,
    teamPath(teamId, "/governance/drafts"),
    input,
    "create governance draft",
  );
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
    teamPath(
      teamId,
      `/governance/drafts/${encodeURIComponent(draftId)}/approve`,
    ),
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
    teamPath(
      teamId,
      `/governance/drafts/${encodeURIComponent(draftId)}/reject`,
    ),
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

export function listTeamMemberRoleRequests(
  options: ApiClientOptions,
  teamId: string,
  status?: TeamMemberRoleRequestStatus,
): Promise<TeamMemberRoleRequest[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  return getJson<TeamMemberRoleRequest[]>(
    options,
    teamPath(teamId, `/member-role-requests${query}`),
    "team member role requests",
  );
}

export function createTeamMemberRoleRequest(
  options: ApiClientOptions,
  teamId: string,
  input: CreateTeamMemberRoleRequestInput,
): Promise<TeamMemberRoleRequest> {
  return postJson<TeamMemberRoleRequest>(
    options,
    teamPath(teamId, "/member-role-requests"),
    input,
    "create team member role request",
  );
}

export function approveTeamMemberRoleRequest(
  options: ApiClientOptions,
  teamId: string,
  requestId: string,
  input: DecideTeamMemberRoleRequestInput = {},
): Promise<TeamMemberRoleRequest> {
  return postJson<TeamMemberRoleRequest>(
    options,
    teamPath(
      teamId,
      `/member-role-requests/${encodeURIComponent(requestId)}/approve`,
    ),
    input,
    "approve team member role request",
  );
}

export function rejectTeamMemberRoleRequest(
  options: ApiClientOptions,
  teamId: string,
  requestId: string,
  input: DecideTeamMemberRoleRequestInput = {},
): Promise<TeamMemberRoleRequest> {
  return postJson<TeamMemberRoleRequest>(
    options,
    teamPath(
      teamId,
      `/member-role-requests/${encodeURIComponent(requestId)}/reject`,
    ),
    input,
    "reject team member role request",
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

// ---- 团队借调（lending）----

export type TeamLendingApprovalMode = "auto" | "manual";
export type TeamLendingRequestStatus =
  | "pending"
  | "auto_approved"
  | "approved"
  | "rejected"
  | "revoked";

export type TeamLendingPolicy = {
  id: string;
  tenant_id: string;
  team_id: string;
  allow_lending: boolean;
  approval_mode: TeamLendingApprovalMode;
  budget_ceiling?: string;
  capability_ceiling: Record<string, unknown>;
  project_match: Record<string, unknown>;
  status: string;
  created_by_user_id?: string;
  updated_by_user_id?: string;
  created_at?: string;
  updated_at?: string;
};

export type UpsertTeamLendingPolicyInput = {
  allow_lending: boolean;
  approval_mode: TeamLendingApprovalMode;
  budget_ceiling?: string;
  capability_ceiling?: Record<string, unknown>;
  project_match?: Record<string, unknown>;
};

export type TeamLendingRequest = {
  id: string;
  tenant_id: string;
  team_id: string;
  project_id: string;
  status: TeamLendingRequestStatus;
  requested_by_user_id: string;
  request_reason: string;
  requested_budget?: string;
  requested_capability: Record<string, unknown>;
  granted_budget?: string;
  granted_capability: Record<string, unknown>;
  is_exception: boolean;
  decided_by_user_id?: string;
  decided_at?: string;
  decision_reason?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateProjectLendingRequestInput = {
  team_id: string;
  request_reason?: string;
  requested_budget?: string;
  requested_capability?: Record<string, unknown>;
};

export type DecideTeamLendingRequestInput = {
  decision_reason?: string;
  granted_budget?: string;
  granted_capability?: Record<string, unknown>;
};

export async function getTeamLendingPolicy(
  options: ApiClientOptions,
  teamId: string,
): Promise<TeamLendingPolicy> {
  return getJson<TeamLendingPolicy>(
    options,
    teamPath(teamId, "/lending-policy"),
    "team lending policy",
  );
}

export function upsertTeamLendingPolicy(
  options: ApiClientOptions,
  teamId: string,
  input: UpsertTeamLendingPolicyInput,
): Promise<TeamLendingPolicy> {
  return putJson<TeamLendingPolicy>(
    options,
    teamPath(teamId, "/lending-policy"),
    input,
    "upsert team lending policy",
  );
}

export async function listTeamLendingRequests(
  options: ApiClientOptions,
  teamId: string,
  status?: TeamLendingRequestStatus,
): Promise<TeamLendingRequest[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  return getJson<TeamLendingRequest[]>(
    options,
    teamPath(teamId, `/lending-requests${query}`),
    "team lending requests",
  );
}

export function approveTeamLendingRequest(
  options: ApiClientOptions,
  teamId: string,
  requestId: string,
  input: DecideTeamLendingRequestInput = {},
): Promise<TeamLendingRequest> {
  return postJson<TeamLendingRequest>(
    options,
    teamPath(teamId, `/lending-requests/${encodeURIComponent(requestId)}/approve`),
    input,
    "approve team lending request",
  );
}

export function rejectTeamLendingRequest(
  options: ApiClientOptions,
  teamId: string,
  requestId: string,
  input: DecideTeamLendingRequestInput = {},
): Promise<TeamLendingRequest> {
  return postJson<TeamLendingRequest>(
    options,
    teamPath(teamId, `/lending-requests/${encodeURIComponent(requestId)}/reject`),
    input,
    "reject team lending request",
  );
}

export function revokeTeamLendingRequest(
  options: ApiClientOptions,
  teamId: string,
  requestId: string,
  input: DecideTeamLendingRequestInput = {},
): Promise<TeamLendingRequest> {
  return postJson<TeamLendingRequest>(
    options,
    teamPath(teamId, `/lending-requests/${encodeURIComponent(requestId)}/revoke`),
    input,
    "revoke team lending request",
  );
}

export function createProjectLendingRequest(
  options: ApiClientOptions,
  projectId: string,
  input: CreateProjectLendingRequestInput,
): Promise<TeamLendingRequest> {
  return postJson<TeamLendingRequest>(
    options,
    `/api/v1/projects/${encodeURIComponent(projectId)}/lending-requests`,
    input,
    "create project lending request",
  );
}

export async function listProjectLendingRequests(
  options: ApiClientOptions,
  projectId: string,
  status?: TeamLendingRequestStatus,
): Promise<TeamLendingRequest[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  return getJson<TeamLendingRequest[]>(
    options,
    `/api/v1/projects/${encodeURIComponent(projectId)}/lending-requests${query}`,
    "project lending requests",
  );
}
