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

export type ProjectSourceKind = "git" | "directory";

export type ProjectCreateDraft = {
  description: string;
  /** 非 Git：用户填写的 Runtime 目录名；Git：可空，由服务端从 URL 推导。 */
  directoryName: string;
  goal: string;
  /** 项目展示名称（允许中文）。 */
  name: string;
  repoDefaultBranch: string;
  repoUrl: string;
  sourceKind: ProjectSourceKind;
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
  directoryName: "",
  goal: "",
  name: "",
  repoDefaultBranch: "main",
  repoUrl: "",
  sourceKind: "directory",
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

/** 与服务端 ValidateProjectDirectoryName 对齐。 */
export const PROJECT_DIRECTORY_NAME_MAX = 64;
export const PROJECT_DISPLAY_NAME_MAX = 120;
const PROJECT_DIRECTORY_NAME_PATTERN =
  /^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$|^[a-zA-Z0-9]$/;

export function validateDisplayProjectName(name: string): string | null {
  const trimmed = name.trim();
  if (!trimmed) {
    return "项目名称不能为空";
  }
  if (trimmed !== name) {
    return "项目名称首尾不能有空白";
  }
  if ([...trimmed].length > PROJECT_DISPLAY_NAME_MAX) {
    return `项目名称最多 ${PROJECT_DISPLAY_NAME_MAX} 个字符`;
  }
  return null;
}

/**
 * 校验项目目录名。返回面向用户的中文错误；合法时返回 null。
 */
export function validateProjectDirectoryName(name: string): string | null {
  const trimmed = name.trim();
  if (!trimmed) {
    return "项目目录名不能为空";
  }
  if (trimmed !== name) {
    return "项目目录名首尾不能有空白";
  }
  if ([...trimmed].some((ch) => ch.charCodeAt(0) > 0x7f)) {
    return "项目目录名禁止中文及其它非 ASCII 字符";
  }
  if (trimmed.length < 1 || trimmed.length > PROJECT_DIRECTORY_NAME_MAX) {
    return `项目目录名长度须为 1–${PROJECT_DIRECTORY_NAME_MAX} 个字符`;
  }
  if (trimmed === "." || trimmed === "..") {
    return "项目目录名不能是 . 或 ..";
  }
  if (/[/\\]/.test(trimmed) || trimmed.includes("\0")) {
    return "项目目录名不能包含路径分隔符";
  }
  if (!PROJECT_DIRECTORY_NAME_PATTERN.test(trimmed)) {
    return "项目目录名仅允许字母、数字、点、下划线、连字符，且不能以点/连字符开头或结尾（单字符除外）";
  }
  return null;
}

/** 从 Git URL 推导目录名预览（与服务端 DirectoryNameFromGitURL 对齐，仅用于 UI 提示）。 */
export function directoryNameHintFromGitURL(raw: string): string | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  let candidate = trimmed;
  try {
    if (trimmed.includes("://")) {
      const parsed = new URL(trimmed);
      candidate = parsed.pathname.split("/").filter(Boolean).pop() ?? "";
    } else {
      const scp = trimmed.match(/^[^/@]+@[^:]+:(.+)$/);
      candidate = scp ? (scp[1].split("/").pop() ?? "") : (trimmed.split("/").pop() ?? "");
    }
  } catch {
    candidate = trimmed.split("/").pop() ?? "";
  }
  candidate = candidate.replace(/\.git$/i, "").trim();
  if (!candidate || validateProjectDirectoryName(candidate)) {
    return null;
  }
  return candidate;
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
  const nameError = validateDisplayProjectName(draft.name);
  const goalOk = Boolean(draft.goal.trim());
  let directoryError: string | null = null;
  let repoError: string | null = null;
  if (draft.sourceKind === "git") {
    if (!draft.repoUrl.trim()) {
      repoError = "请填写 Git 仓库 URL";
    }
    const hint = directoryNameHintFromGitURL(draft.repoUrl);
    if (draft.directoryName.trim()) {
      directoryError = validateProjectDirectoryName(draft.directoryName);
    } else if (!hint && draft.repoUrl.trim()) {
      directoryError = "无法从 URL 推导合法目录名，请手填项目目录名";
    }
  } else {
    directoryError = validateProjectDirectoryName(draft.directoryName);
  }
  const basics =
    nameError === null &&
    goalOk &&
    directoryError === null &&
    repoError === null;

  return {
    basics,
    currentUser: Boolean(currentUserId),
    digitalEmployees: true,
    directoryError,
    nameError,
    policies: draft.policyToggles.auditLogEnabled,
    repoError,
    runtimeNodes: draft.runtimeNodeIds.length > 0,
    sourceTeams: sourceTeamsAuthorized,
    teamAuthorized: sourceTeamsAuthorized,
  };
}

export function buildProjectCreateInput(
  draft: ProjectCreateDraft,
  currentUser: UserSummary,
): CreateProjectInput {
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

  const isGit = draft.sourceKind === "git";
  const repoUrl = draft.repoUrl.trim();
  const repoDefaultBranch = draft.repoDefaultBranch.trim() || "main";
  const directoryName = draft.directoryName.trim();

  return {
    coordination_policy: {
      audit_log_enabled: draft.policyToggles.auditLogEnabled,
      require_human_review_for_new_demands: draft.policyToggles.newDemandNeedsHumanConfirmation,
      preset: draft.policyPreset,
    },
    description: draft.description.trim() || undefined,
    ...(directoryName ? { directory_name: directoryName } : {}),
    goal: draft.goal.trim(),
    human_owner_user_id: ownerIDs[0],
    human_owner_user_ids: ownerIDs,
    members,
    name: draft.name.trim(),
    ...(isGit && repoUrl
      ? {
          repo_binding: {
            status: "bound" as const,
            url: repoUrl,
            default_branch: repoDefaultBranch,
          },
        }
      : {}),
    runtime_node_ids: draft.runtimeNodeIds,
    scenario_template_key: draft.scenarioTemplateKey.trim() || undefined,
    team_id: draft.sourceTeamIds[0],
  };
}
