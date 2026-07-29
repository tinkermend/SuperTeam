import type { Tone } from "@/components/superteam";
import type {
  Project,
  ProjectDecisionRequest,
  ProjectEvidenceRef,
  ProjectMember,
  ProjectPrincipalType,
  ProjectTask,
} from "@/lib/api/projects";
import { isAwaitingHumanApproval } from "@/lib/task-status";

export type ProjectRiskLevel = "none" | "info" | "warn" | "danger";

export type ProjectRiskReasonType =
  | "human_decision"
  | "waiting_human"
  | "execution_failed"
  | "evidence_required"
  | "sla_waiting"
  | "runtime_or_coordination";

export type ProjectRiskFilter = "all" | "blocked" | ProjectRiskReasonType;

export type ProjectRiskReason = {
  id: string;
  type: ProjectRiskReasonType;
  level: Exclude<ProjectRiskLevel, "none">;
  source: "project" | "tasks" | "decisions" | "evidence" | "events";
  title: string;
  detail?: string;
  sourceId?: string;
  waitingSince?: string;
};

export type ProjectRiskSummaryState = "ready" | "pending" | "error";

export type ProjectRiskSummary = {
  projectId: string;
  project: Project;
  level: ProjectRiskLevel;
  state: ProjectRiskSummaryState;
  owner?: ProjectTaskHandler;
  currentHandler?: ProjectTaskHandler;
  currentTask?: ProjectTask;
  reasons: ProjectRiskReason[];
  primaryReason?: ProjectRiskReason;
  requiresHuman: boolean;
  waitingSince?: string;
  updatedAt?: string;
};

export type ProjectRiskSummaryMap = Record<string, ProjectRiskSummary>;

export type ProjectTaskHandler = {
  id: string;
  label: string;
  principalType: ProjectPrincipalType;
};

export type ProjectRiskInput = {
  project: Project;
  tasks?: ProjectTask[];
  decisions?: ProjectDecisionRequest[];
  evidence?: ProjectEvidenceRef[];
  events?: unknown[];
  members?: ProjectMember[];
  /** principal_id → 展示名，用于成员快照缺失时的名称回退（如全局数字员工目录）。 */
  principalNamesById?: ReadonlyMap<string, string>;
};

export type ProjectRiskOptions = {
  now?: Date | string | number;
  state?: ProjectRiskSummaryState;
  slaWaitMs?: number;
};

export type ProjectRiskCounts = Record<ProjectRiskFilter, number>;

export const PROJECT_RISK_FILTERS: Array<{
  value: ProjectRiskFilter;
  label: string;
}> = [
  { value: "all", label: "全部项目" },
  { value: "blocked", label: "存在风险" },
  { value: "human_decision", label: "待决决策" },
  { value: "waiting_human", label: "等人任务" },
  { value: "execution_failed", label: "执行失败" },
  { value: "runtime_or_coordination", label: "协调异常" },
  { value: "evidence_required", label: "证据待核" },
  { value: "sla_waiting", label: "等待超时" },
];

const activeDecisionStatuses = new Set(["pending", "waiting", "requested", "open"]);
// cancelled 是终态清理结果，不是「执行失败待处理」；勿并入失败袋虚增待办。
const failedTaskStatuses = new Set(["failed", "error", "blocked"]);
const waitingHumanTaskStatuses = new Set([
  "waiting_human",
  "pending_human",
  "pending_review",
  "approval_required",
]);
const healthyCoordinationStatuses = new Set([
  "",
  "active",
  "idle",
  "ready",
  "registered",
  "running",
  "started",
]);
const evidenceRequiredStatuses = new Set(["rejected", "submitted"]);

const slaWaitMs = 2 * 60 * 60 * 1000;

const levelRank: Record<ProjectRiskLevel, number> = {
  none: 0,
  info: 1,
  warn: 2,
  danger: 3,
};

const reasonPriority: Record<ProjectRiskReasonType, number> = {
  human_decision: 6,
  waiting_human: 5,
  execution_failed: 4,
  runtime_or_coordination: 3,
  evidence_required: 2,
  sla_waiting: 1,
};

const reasonLabels: Record<ProjectRiskReasonType, string> = {
  human_decision: "待决决策",
  waiting_human: "任务等待人工",
  execution_failed: "执行失败",
  runtime_or_coordination: "协调异常",
  evidence_required: "证据待核",
  sla_waiting: "等待超时",
};

/** 需要负责人立刻行动的 reason（不含证据核验元数据、不含 SLA 附注）。 */
const ACTIONABLE_REASON_TYPES = new Set<ProjectRiskReasonType>([
  "human_decision",
  "waiting_human",
  "execution_failed",
  "runtime_or_coordination",
]);

const taskStatusPriority: Record<string, number> = {
  running: 100,
  executing: 95,
  in_progress: 95,
  dispatching: 90,
  claimed: 85,
  waiting_human: 80,
  pending_human: 80,
  pending_review: 80,
  approval_required: 80,
  queued: 70,
  pending: 65,
  planned: 60,
  failed: 50,
  error: 50,
  blocked: 50,
  completed: 20,
  cancelled: 10,
};

export function deriveProjectRiskSummary(
  input: ProjectRiskInput,
  options: ProjectRiskOptions = {},
): ProjectRiskSummary {
  const reasons: ProjectRiskReason[] = [];
  const { project } = input;

  // 已归档项目不再用证据核验/历史等人态刷注意力队列；归档本身即闭环。
  if (normalize(project.status) === "archived" || project.archived_at) {
    const members = input.members ?? [];
    const principalNames = input.principalNamesById;
    return {
      projectId: project.id,
      project,
      level: "none",
      state: options.state ?? "ready",
      owner: selectProjectOwner(project, members, principalNames),
      currentHandler: undefined,
      currentTask: undefined,
      reasons: [],
      primaryReason: undefined,
      requiresHuman: false,
      waitingSince: undefined,
      updatedAt: project.updated_at,
    };
  }

  for (const decision of input.decisions ?? []) {
    if (!activeDecisionStatuses.has(normalize(decision.status_snapshot))) {
      continue;
    }
    reasons.push({
      id: `decision:${decision.id}`,
      type: "human_decision",
      level: "danger",
      source: "decisions",
      title: decision.title_snapshot,
      detail: decision.decision_type,
      sourceId: decision.id,
      waitingSince: decision.created_at,
    });
  }

  for (const projectTask of input.tasks ?? []) {
    if (projectTask.dismissed_at) {
      continue;
    }
    const status = normalize(projectTask.status);
    if (failedTaskStatuses.has(status)) {
      reasons.push({
        id: `task:${projectTask.id}:execution_failed`,
        type: "execution_failed",
        level: "danger",
        source: "tasks",
        title: projectTask.title,
        detail: projectTask.status,
        sourceId: projectTask.id,
      });
      continue;
    }
    if (isAwaitingHumanApproval(projectTask) || waitingHumanTaskStatuses.has(status)) {
      // 任务停泊「等人」≠ 已有 open decision 卡；单独分桶，避免与决策待办混成「N 项待处理」。
      reasons.push({
        id: `task:${projectTask.id}:waiting_human`,
        type: "waiting_human",
        level: "danger",
        source: "tasks",
        title: projectTask.title,
        detail: projectTask.status,
        sourceId: projectTask.id,
        waitingSince: projectTask.updated_at ?? projectTask.created_at,
      });
    }
  }

  const coordinationStatus = normalize(project.coordination_status);
  if (!healthyCoordinationStatuses.has(coordinationStatus)) {
    reasons.push({
      id: `project:${project.id}:runtime_or_coordination`,
      type: "runtime_or_coordination",
      level: "danger",
      source: "project",
      title: project.name,
      detail: project.coordination_status,
      sourceId: project.id,
    });
  }

  for (const evidence of input.evidence ?? []) {
    if (!evidenceRequiredStatuses.has(normalize(evidence.verification_status))) {
      continue;
    }
    reasons.push({
      id: `evidence:${evidence.id}`,
      type: "evidence_required",
      level: "warn",
      source: "evidence",
      title: evidence.title,
      detail: evidence.verification_status,
      sourceId: evidence.id,
      waitingSince: evidence.updated_at ?? evidence.created_at,
    });
  }

  const waitingSince = staleWaitingSince(input, options);
  if (waitingSince) {
    reasons.push({
      id: `project:${project.id}:sla_waiting`,
      type: "sla_waiting",
      level: "warn",
      source: "project",
      title: project.name,
      detail: "waiting_over_sla",
      sourceId: project.id,
      waitingSince,
    });
  }

  const primaryReason = pickPrimaryReason(reasons);
  const level = primaryReason?.level ?? "none";
  const currentTask = selectCurrentTask(input.tasks ?? []);
  const members = input.members ?? [];
  const principalNames = input.principalNamesById;
  const currentHandler = selectCurrentHandler(
    project,
    currentTask,
    members,
    principalNames,
  );
  const owner = selectProjectOwner(project, members, principalNames);

  return {
    projectId: project.id,
    project,
    level,
    state: options.state ?? "ready",
    owner,
    currentHandler,
    currentTask,
    reasons,
    primaryReason,
    requiresHuman: reasons.some(
      (reason) => reason.type === "human_decision" || reason.type === "waiting_human",
    ),
    waitingSince: earliestWaitingSince(reasons),
    updatedAt: project.updated_at,
  };
}

export function emptyProjectRiskSummary(
  project: Project,
  options: ProjectRiskOptions = {},
): ProjectRiskSummary {
  return {
    projectId: project.id,
    project,
    level: "none",
    state: options.state ?? "ready",
    reasons: [],
    requiresHuman: false,
    updatedAt: project.updated_at,
  };
}

export function buildRiskCounts(
  summaries: ProjectRiskSummary[],
): ProjectRiskCounts {
  const counts: ProjectRiskCounts = {
    all: summaries.length,
    blocked: 0,
    human_decision: 0,
    waiting_human: 0,
    execution_failed: 0,
    runtime_or_coordination: 0,
    evidence_required: 0,
    sla_waiting: 0,
  };

  for (const summary of summaries) {
    if (summary.level === "danger" || summary.level === "warn") {
      counts.blocked += 1;
    }
    const reasonTypes = new Set(summary.reasons.map((reason) => reason.type));
    for (const reasonType of reasonTypes) {
      counts[reasonType] += 1;
    }
  }

  return counts;
}

export type ProjectPortfolioCounts = {
  total: number;
  running: number;
  acceptance: number;
  paused: number;
  configuring: number;
  archived: number;
  coordinationAnomaly: number;
};

/**
 * 组合层可全量计算的项目组合真值：只读项目对象上的廉价字段（status /
 * coordination_status），不做每项目 fan-out，因此可覆盖当前已加载的完整列表，
 * 而不像逐项目风险信号那样只能覆盖当前页。
 */
export function buildProjectPortfolioCounts(
  projects: Project[],
): ProjectPortfolioCounts {
  const counts: ProjectPortfolioCounts = {
    total: projects.length,
    running: 0,
    acceptance: 0,
    paused: 0,
    configuring: 0,
    archived: 0,
    coordinationAnomaly: 0,
  };

  for (const project of projects) {
    switch (project.status) {
      case "running":
        counts.running += 1;
        break;
      case "acceptance":
        counts.acceptance += 1;
        break;
      case "paused":
        counts.paused += 1;
        break;
      case "configuring":
      case "draft":
        counts.configuring += 1;
        break;
      case "archived":
        counts.archived += 1;
        break;
      default:
        break;
    }
    if (
      project.status !== "archived" &&
      !healthyCoordinationStatuses.has(normalize(project.coordination_status))
    ) {
      counts.coordinationAnomaly += 1;
    }
  }

  return counts;
}

export function matchesProjectRiskFilter(
  summary: ProjectRiskSummary,
  filter: ProjectRiskFilter,
): boolean {
  if (filter === "all") {
    return true;
  }
  if (filter === "blocked") {
    return summary.level === "danger" || summary.level === "warn";
  }
  return summary.reasons.some((reason) => reason.type === filter);
}

export function sortProjectsByRisk(
  projects: Project[],
  summaries: ProjectRiskSummaryMap,
): Project[] {
  return [...projects].sort((left, right) => {
    const leftSummary = summaries[left.id] ?? emptyProjectRiskSummary(left);
    const rightSummary = summaries[right.id] ?? emptyProjectRiskSummary(right);

    return (
      compareDesc(levelRank[leftSummary.level], levelRank[rightSummary.level]) ||
      compareDesc(
        Number(leftSummary.requiresHuman),
        Number(rightSummary.requiresHuman),
      ) ||
      compareDesc(
        summaryReasonPriority(leftSummary),
        summaryReasonPriority(rightSummary),
      ) ||
      compareWaitingSince(leftSummary.waitingSince, rightSummary.waitingSince) ||
      compareUpdatedAt(left.updated_at, right.updated_at)
    );
  });
}

export function projectRiskLevelTone(level: ProjectRiskLevel): Tone {
  if (level === "danger") {
    return "danger";
  }
  if (level === "warn") {
    return "warn";
  }
  if (level === "info") {
    return "info";
  }
  return "mute";
}

export function projectRiskLevelLabel(summary: ProjectRiskSummary): string {
  if (summary.state === "pending") {
    return "识别中";
  }
  if (summary.state === "error") {
    return "风险待确认";
  }
  if (summary.primaryReason) {
    return reasonLabels[summary.primaryReason.type];
  }
  return "暂无阻塞";
}

export type ProjectAttentionBreakdown = {
  decisions: number;
  waitingHuman: number;
  executionFailed: number;
  coordination: number;
  evidence: number;
  sla: number;
  /** 决策/等人/失败/协调 — 应对齐项目内可下钻的行动项 */
  actionableTotal: number;
  /** 证据核验 + SLA 等信号，默认不叫「待处理」 */
  signalTotal: number;
  allTotal: number;
};

export function buildAttentionBreakdown(
  summary: Pick<ProjectRiskSummary, "reasons">,
): ProjectAttentionBreakdown {
  const breakdown: ProjectAttentionBreakdown = {
    decisions: 0,
    waitingHuman: 0,
    executionFailed: 0,
    coordination: 0,
    evidence: 0,
    sla: 0,
    actionableTotal: 0,
    signalTotal: 0,
    allTotal: summary.reasons.length,
  };

  for (const reason of summary.reasons) {
    switch (reason.type) {
      case "human_decision":
        breakdown.decisions += 1;
        break;
      case "waiting_human":
        breakdown.waitingHuman += 1;
        break;
      case "execution_failed":
        breakdown.executionFailed += 1;
        break;
      case "runtime_or_coordination":
        breakdown.coordination += 1;
        break;
      case "evidence_required":
        breakdown.evidence += 1;
        break;
      case "sla_waiting":
        breakdown.sla += 1;
        break;
      default:
        break;
    }
    if (ACTIONABLE_REASON_TYPES.has(reason.type)) {
      breakdown.actionableTotal += 1;
    } else {
      breakdown.signalTotal += 1;
    }
  }

  return breakdown;
}

/**
 * 队列/右栏主文案：禁止再用 reasons.length 显示「N 项待处理」。
 * 可行动项按桶拼接；仅有证据/SLA 时用中性措辞。
 */
export function formatAttentionHeadline(
  summary: Pick<ProjectRiskSummary, "reasons" | "state">,
): { primary: string; detail?: string; hasActionable: boolean } {
  if (summary.state === "pending") {
    return { primary: "识别中", hasActionable: false };
  }
  if (summary.state === "error") {
    return { primary: "风险待确认", hasActionable: false };
  }

  const b = buildAttentionBreakdown(summary);
  const parts: string[] = [];
  if (b.decisions > 0) parts.push(`${b.decisions} 待决`);
  if (b.waitingHuman > 0) parts.push(`${b.waitingHuman} 等人`);
  if (b.executionFailed > 0) parts.push(`${b.executionFailed} 失败`);
  if (b.coordination > 0) parts.push(`${b.coordination} 协调异常`);

  if (parts.length > 0) {
    const detailParts: string[] = [];
    if (b.evidence > 0) detailParts.push(`${b.evidence} 证据待核`);
    if (b.sla > 0) detailParts.push("等待超时");
    return {
      primary: parts.join(" · "),
      detail: detailParts.length > 0 ? detailParts.join(" · ") : undefined,
      hasActionable: true,
    };
  }

  if (b.evidence > 0 || b.sla > 0) {
    const soft: string[] = [];
    if (b.evidence > 0) soft.push(`${b.evidence} 条证据待核`);
    if (b.sla > 0) soft.push("等待超时");
    return { primary: soft.join(" · "), hasActionable: false };
  }

  return { primary: "无待办", hasActionable: false };
}

export function projectRiskReasonLabel(type: ProjectRiskReasonType): string {
  return reasonLabels[type];
}

export function isActionableRiskReason(type: ProjectRiskReasonType): boolean {
  return ACTIONABLE_REASON_TYPES.has(type);
}

function normalize(value?: string): string {
  return (value ?? "").trim().toLowerCase();
}

function staleWaitingSince(
  input: ProjectRiskInput,
  options: ProjectRiskOptions,
): string | undefined {
  const now = parseTime(options.now ?? new Date());
  if (now === undefined) {
    return undefined;
  }
  const threshold = options.slaWaitMs ?? slaWaitMs;
  const candidates: string[] = [];

  // 「等待超时」必须挂在真实等待主体上（未决决策 / 等人任务），
  // 不能仅因 project.status=running 且 updated_at 变旧就报「待处理」——
  // 空任务/已无动作的 running 项目会假阳性（如配置页体验走查E2E）。
  for (const decision of input.decisions ?? []) {
    if (!activeDecisionStatuses.has(normalize(decision.status_snapshot))) {
      continue;
    }
    const since = decision.created_at;
    const sinceMs = since ? Date.parse(since) : Number.NaN;
    if (!Number.isNaN(sinceMs) && now - sinceMs > threshold) {
      candidates.push(since!);
    }
  }

  for (const projectTask of input.tasks ?? []) {
    if (projectTask.dismissed_at) {
      continue;
    }
    const status = normalize(projectTask.status);
    // 与员工运营态一致：requires_human_approval 非等待态不算「在等人」。
    if (!waitingHumanTaskStatuses.has(status)) {
      continue;
    }
    const since = projectTask.updated_at ?? projectTask.created_at;
    const sinceMs = since ? Date.parse(since) : Number.NaN;
    if (!Number.isNaN(sinceMs) && now - sinceMs > threshold) {
      candidates.push(since!);
    }
  }

  return candidates.sort((left, right) => Date.parse(left) - Date.parse(right))[0];
}

function parseTime(value: Date | string | number): number | undefined {
  const time =
    value instanceof Date
      ? value.getTime()
      : typeof value === "number"
        ? value
        : Date.parse(value);
  return Number.isNaN(time) ? undefined : time;
}

function pickPrimaryReason(
  reasons: ProjectRiskReason[],
): ProjectRiskReason | undefined {
  return [...reasons].sort((left, right) => {
    return (
      compareDesc(levelRank[left.level], levelRank[right.level]) ||
      compareDesc(reasonPriority[left.type], reasonPriority[right.type]) ||
      compareWaitingSince(left.waitingSince, right.waitingSince)
    );
  })[0];
}

function selectCurrentTask(tasks: ProjectTask[]): ProjectTask | undefined {
  return [...tasks].sort((left, right) => {
    const leftPriority = taskStatusPriority[normalize(left.status)] ?? 40;
    const rightPriority = taskStatusPriority[normalize(right.status)] ?? 40;
    return (
      compareDesc(leftPriority, rightPriority) ||
      compareAsc(left.stage_index, right.stage_index)
    );
  })[0];
}

function selectCurrentHandler(
  project: Project,
  task: ProjectTask | undefined,
  members: ProjectMember[],
  principalNames?: ReadonlyMap<string, string>,
): ProjectTaskHandler | undefined {
  const assignedDigitalEmployeeId = task?.assigned_digital_employee_id?.trim();
  if (assignedDigitalEmployeeId) {
    return buildTaskHandler(
      assignedDigitalEmployeeId,
      "digital_employee",
      members,
      principalNames,
    );
  }

  const humanOwnerId = project.human_owner_user_id?.trim();
  if (humanOwnerId && isHumanHandledTask(task)) {
    return buildTaskHandler(humanOwnerId, "human_user", members, principalNames);
  }

  return undefined;
}

function selectProjectOwner(
  project: Project,
  members: ProjectMember[],
  principalNames?: ReadonlyMap<string, string>,
): ProjectTaskHandler | undefined {
  const humanOwnerId = project.human_owner_user_id?.trim();
  if (!humanOwnerId) {
    return undefined;
  }
  return buildTaskHandler(humanOwnerId, "human_user", members, principalNames);
}

/** 项目负责人展示名：摘要 label → 目录补名 → 裸 id（仅名称不可得时）→ 未设置。 */
export function resolveProjectOwnerLabel(
  project: Project,
  owner?: ProjectTaskHandler,
  principalNamesById?: ReadonlyMap<string, string>,
): string {
  const ownerId = (owner?.id || project.human_owner_user_id || "").trim();
  const ownerLabel = owner?.label?.trim();
  if (ownerLabel && ownerLabel !== ownerId) {
    return ownerLabel;
  }
  if (ownerId) {
    const named = principalNamesById?.get(ownerId)?.trim();
    if (named) {
      return named;
    }
  }
  if (ownerLabel) {
    return ownerLabel;
  }
  return ownerId || "未设置";
}

function buildTaskHandler(
  id: string,
  principalType: ProjectPrincipalType,
  members: ProjectMember[],
  principalNames?: ReadonlyMap<string, string>,
): ProjectTaskHandler {
  const member = members.find(
    (item) =>
      item.principal_type === principalType &&
      item.principal_id === id &&
      item.status !== "removed",
  );
  const displayName =
    member?.display_name_snapshot?.trim() || principalNames?.get(id)?.trim();
  return {
    id,
    label: displayName || id,
    principalType,
  };
}

function isHumanHandledTask(task: ProjectTask | undefined): boolean {
  if (!task) {
    return false;
  }
  const status = normalize(task.status);
  return (
    isAwaitingHumanApproval(task) ||
    status === "waiting_human" ||
    status === "pending_human" ||
    status === "pending_review" ||
    status === "approval_required"
  );
}

function earliestWaitingSince(
  reasons: ProjectRiskReason[],
): string | undefined {
  return reasons
    .map((reason) => reason.waitingSince)
    .filter((value): value is string => Boolean(value))
    .sort((left, right) => Date.parse(left) - Date.parse(right))[0];
}

function summaryReasonPriority(summary: ProjectRiskSummary): number {
  return summary.reasons.reduce(
    (priority, reason) => Math.max(priority, reasonPriority[reason.type]),
    0,
  );
}

function compareDesc(left: number, right: number): number {
  return right - left;
}

function compareAsc(left?: number, right?: number): number {
  if (left === undefined && right === undefined) {
    return 0;
  }
  if (left === undefined) {
    return 1;
  }
  if (right === undefined) {
    return -1;
  }
  return left - right;
}

function compareWaitingSince(left?: string, right?: string): number {
  const leftTime = left ? Date.parse(left) : undefined;
  const rightTime = right ? Date.parse(right) : undefined;
  const leftValid = leftTime !== undefined && !Number.isNaN(leftTime);
  const rightValid = rightTime !== undefined && !Number.isNaN(rightTime);
  if (!leftValid && !rightValid) {
    return 0;
  }
  if (!leftValid) {
    return 1;
  }
  if (!rightValid) {
    return -1;
  }
  return leftTime - rightTime;
}

function compareUpdatedAt(left?: string, right?: string): number {
  const leftTime = left ? Date.parse(left) : undefined;
  const rightTime = right ? Date.parse(right) : undefined;
  const leftValid = leftTime !== undefined && !Number.isNaN(leftTime);
  const rightValid = rightTime !== undefined && !Number.isNaN(rightTime);
  if (!leftValid && !rightValid) {
    return 0;
  }
  if (!leftValid) {
    return 1;
  }
  if (!rightValid) {
    return -1;
  }
  return rightTime - leftTime;
}
