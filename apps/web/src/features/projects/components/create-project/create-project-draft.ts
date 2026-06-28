import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import type { DigitalEmployee } from "@/lib/api/employees";
import type { CreateProjectInput, ProjectMemberInput } from "@/lib/api/projects";

export type ProjectCreateStep = "basics" | "roles" | "digitalEmployees" | "policies" | "review";

export const projectCreateSteps: Array<{ id: ProjectCreateStep; label: string }> = [
  { id: "basics", label: "基础信息" },
  { id: "roles", label: "团队与人类角色" },
  { id: "digitalEmployees", label: "数字员工池" },
  { id: "policies", label: "策略预设" },
  { id: "review", label: "确认创建" },
];

export type ProjectPolicyPreset = "standard" | "lightweight" | "highRisk";

export type ProjectCreateDraft = {
  acceptanceUser?: UserSummary;
  description: string;
  goal: string;
  leaderUser?: UserSummary;
  name: string;
  policyPreset: ProjectPolicyPreset;
  policyToggles: {
    auditLogEnabled: boolean;
    budgetOverrunNeedsOwnerApproval: boolean;
    highRiskActionNeedsConfirmation: boolean;
    newDemandNeedsHumanConfirmation: boolean;
    requireEvidenceBeforeAcceptance: boolean;
  };
  reviewerUsers: UserSummary[];
  selectedDigitalEmployees: DigitalEmployee[];
  teamId: string;
};

export const emptyProjectCreateDraft: ProjectCreateDraft = {
  description: "",
  goal: "",
  name: "",
  policyPreset: "standard",
  policyToggles: {
    auditLogEnabled: true,
    budgetOverrunNeedsOwnerApproval: false,
    highRiskActionNeedsConfirmation: true,
    newDemandNeedsHumanConfirmation: true,
    requireEvidenceBeforeAcceptance: true,
  },
  reviewerUsers: [],
  selectedDigitalEmployees: [],
  teamId: "",
};

export function activeSelectableTeams(scopes: UserProjectTeamScope[] | undefined) {
  return (scopes ?? []).filter(
    (scope) => scope.status === "active" && !scope.revoked_at && scope.team.status === "active",
  );
}

export function selectedTeam(scopes: UserProjectTeamScope[] | undefined, teamId: string) {
  return activeSelectableTeams(scopes).find((scope) => scope.team_id === teamId);
}

const POLICY_PRESETS: Record<ProjectPolicyPreset, ProjectCreateDraft["policyToggles"]> = {
  standard: {
    auditLogEnabled: true,
    budgetOverrunNeedsOwnerApproval: false,
    highRiskActionNeedsConfirmation: true,
    newDemandNeedsHumanConfirmation: true,
    requireEvidenceBeforeAcceptance: true,
  },
  lightweight: {
    auditLogEnabled: true,
    budgetOverrunNeedsOwnerApproval: false,
    highRiskActionNeedsConfirmation: true,
    newDemandNeedsHumanConfirmation: false,
    requireEvidenceBeforeAcceptance: false,
  },
  highRisk: {
    auditLogEnabled: true,
    budgetOverrunNeedsOwnerApproval: true,
    highRiskActionNeedsConfirmation: true,
    newDemandNeedsHumanConfirmation: true,
    requireEvidenceBeforeAcceptance: true,
  },
};

export function applyPolicyPreset(
  draft: ProjectCreateDraft,
  preset: ProjectPolicyPreset,
): ProjectCreateDraft {
  const policyToggles = POLICY_PRESETS[preset];

  return { ...draft, policyPreset: preset, policyToggles };
}

export function projectCreateValidation(
  draft: ProjectCreateDraft,
  currentUserId: string | undefined,
  selectableTeams: UserProjectTeamScope[],
) {
  const authorizedTeamIds = new Set(activeSelectableTeams(selectableTeams).map((scope) => scope.team_id));
  return {
    basics: Boolean(draft.name.trim()) && Boolean(draft.goal.trim()) && Boolean(draft.teamId),
    currentUser: Boolean(currentUserId),
    digitalEmployees: true, // global constraint: optional
    policies: draft.policyToggles.auditLogEnabled,
    teamAuthorized: Boolean(draft.teamId) && authorizedTeamIds.has(draft.teamId),
  };
}

export function buildProjectCreateInput(
  draft: ProjectCreateDraft,
  currentUser: UserSummary,
): CreateProjectInput {
  const members: ProjectMemberInput[] = [];

  if (draft.leaderUser) {
    members.push({
      display_name_snapshot: draft.leaderUser.display_name ?? draft.leaderUser.username,
      principal_id: draft.leaderUser.id,
      principal_type: "human_user",
      project_role: "leader",
    });
  }

  if (draft.acceptanceUser) {
    members.push({
      display_name_snapshot: draft.acceptanceUser.display_name ?? draft.acceptanceUser.username,
      principal_id: draft.acceptanceUser.id,
      principal_type: "human_user",
      project_role: "acceptance",
    });
  }

  for (const reviewer of draft.reviewerUsers) {
    members.push({
      display_name_snapshot: reviewer.display_name ?? reviewer.username,
      principal_id: reviewer.id,
      principal_type: "human_user",
      project_role: "reviewer",
    });
  }

  for (const employee of draft.selectedDigitalEmployees) {
    members.push({
      display_name_snapshot: employee.name,
      principal_id: employee.id,
      principal_type: "digital_employee",
      project_role: "executor",
      settings: {
        role: employee.role,
        risk_level: employee.risk_level,
      },
    });
  }

  return {
    approval_policy: {
      budget_overrun_requires_owner_approval: draft.policyToggles.budgetOverrunNeedsOwnerApproval,
      high_risk_action_requires_confirmation: draft.policyToggles.highRiskActionNeedsConfirmation,
      new_demand_requires_human_confirmation: draft.policyToggles.newDemandNeedsHumanConfirmation,
      preset: draft.policyPreset,
    },
    coordination_policy: {
      audit_log_enabled: draft.policyToggles.auditLogEnabled,
      preset: draft.policyPreset,
    },
    description: draft.description.trim() || undefined,
    evidence_policy: {
      acceptance_requires_evidence: draft.policyToggles.requireEvidenceBeforeAcceptance,
      preset: draft.policyPreset,
    },
    goal: draft.goal.trim(),
    human_owner_user_id: currentUser.id,
    leader_user_id: draft.leaderUser?.id,
    acceptance_user_id: draft.acceptanceUser?.id,
    members,
    name: draft.name.trim(),
    team_id: draft.teamId,
  };
}
