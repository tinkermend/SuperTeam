import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import type { DigitalEmployee } from "@/lib/api/employees";
import type { CreateProjectInput, ProjectMemberInput } from "@/lib/api/projects";

export type ProjectCreateStep = "basics" | "owners" | "digitalEmployees" | "runtimeNodes" | "policies";

export const projectCreateSteps: Array<{ id: ProjectCreateStep; label: string }> = [
  { id: "basics", label: "基础信息" },
  { id: "owners", label: "人类负责人" },
  { id: "digitalEmployees", label: "数字员工池" },
  { id: "runtimeNodes", label: "可运行节点" },
  { id: "policies", label: "协调策略" },
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
    newDemandNeedsHumanConfirmation: boolean;
  };
  runtimeNodeIds: string[];
  scenarioTemplateKey: string;
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
    newDemandNeedsHumanConfirmation: true,
  },
  ownerUsers: [],
  runtimeNodeIds: [],
  scenarioTemplateKey: "",
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
    newDemandNeedsHumanConfirmation: true,
  },
  lightweight: {
    auditLogEnabled: true,
    newDemandNeedsHumanConfirmation: false,
  },
  highRisk: {
    auditLogEnabled: true,
    newDemandNeedsHumanConfirmation: true,
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
    runtimeNodes: draft.runtimeNodeIds.length > 0,
    sourceTeams: sourceTeamsAuthorized,
    teamAuthorized: sourceTeamsAuthorized,
  };
}

export function buildProjectCreateInput(
  draft: ProjectCreateDraft,
  currentUser: UserSummary,
): CreateProjectInput {
  // 多负责人:选中的人类负责人即为项目 owners;未选时回退为创建者本人(保证 ≥1)。
  const ownerUsers = draft.ownerUsers.length > 0 ? draft.ownerUsers : [currentUser];
  const ownerIDs = ownerUsers.map((owner) => owner.id);
  const members: ProjectMemberInput[] = ownerUsers.map((owner) => ({
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
    coordination_policy: {
      audit_log_enabled: draft.policyToggles.auditLogEnabled,
      require_human_review_for_new_demands: draft.policyToggles.newDemandNeedsHumanConfirmation,
      preset: draft.policyPreset,
    },
    description: draft.description.trim() || undefined,
    goal: draft.goal.trim(),
    human_owner_user_id: ownerIDs[0],
    human_owner_user_ids: ownerIDs,
    members,
    name: draft.name.trim(),
    runtime_node_ids: draft.runtimeNodeIds,
    scenario_template_key: draft.scenarioTemplateKey.trim() || undefined,
    team_id: draft.sourceTeamIds[0],
  };
}
