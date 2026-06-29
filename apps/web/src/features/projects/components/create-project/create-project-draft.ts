import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import type { DigitalEmployee } from "@/lib/api/employees";
import type { CreateProjectInput, ProjectMemberInput } from "@/lib/api/projects";

export type ProjectCreateStep = "basics" | "owners" | "digitalEmployees" | "policies";

export const projectCreateSteps: Array<{ id: ProjectCreateStep; label: string }> = [
  { id: "basics", label: "基础信息" },
  { id: "owners", label: "人类负责人" },
  { id: "digitalEmployees", label: "数字员工池" },
  { id: "policies", label: "策略预设" },
];

export type ProjectPolicyPreset = "standard" | "lightweight" | "highRisk";

export type ProjectCreateDraft = {
  description: string;
  goal: string;
  name: string;
  ownerUsers: UserSummary[];
  policyPreset: ProjectPolicyPreset;
  policyToggles: {
    auditLogEnabled: boolean;
    budgetOverrunNeedsOwnerApproval: boolean;
    highRiskActionNeedsConfirmation: boolean;
    newDemandNeedsHumanConfirmation: boolean;
    requireEvidenceBeforeAcceptance: boolean;
  };
  selectedDigitalEmployees: DigitalEmployee[];
  sourceTeamIds: string[];
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
  ownerUsers: [],
  selectedDigitalEmployees: [],
  sourceTeamIds: [],
};

export function activeSelectableTeams(scopes: UserProjectTeamScope[] | undefined) {
  return (scopes ?? []).filter(
    (scope) => scope.status === "active" && !scope.revoked_at && scope.team.status === "active",
  );
}

export function selectedSourceTeams(
  scopes: UserProjectTeamScope[] | undefined,
  sourceTeamIds: string[],
) {
  const selectedIds = new Set(sourceTeamIds);
  return activeSelectableTeams(scopes).filter((scope) => selectedIds.has(scope.team_id));
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
  const sourceTeamsAuthorized =
    draft.sourceTeamIds.length > 0 &&
    draft.sourceTeamIds.every((sourceTeamId) => authorizedTeamIds.has(sourceTeamId));

  return {
    basics: Boolean(draft.name.trim()) && Boolean(draft.goal.trim()),
    currentUser: Boolean(currentUserId),
    digitalEmployees: true, // global constraint: optional
    policies: draft.policyToggles.auditLogEnabled,
    sourceTeams: sourceTeamsAuthorized,
    teamAuthorized: sourceTeamsAuthorized,
  };
}

export function buildProjectCreateInput(
  draft: ProjectCreateDraft,
  currentUser: UserSummary,
): CreateProjectInput {
  const members: ProjectMemberInput[] = draft.ownerUsers.map((owner) => ({
    display_name_snapshot: owner.display_name ?? owner.username,
    principal_id: owner.id,
    principal_type: "human_user",
    project_role: "owner",
  }));

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
    members,
    name: draft.name.trim(),
    team_id: draft.sourceTeamIds[0],
  };
}
