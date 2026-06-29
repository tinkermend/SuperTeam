import type { V3Tone } from "@/components/superteam";
import type {
  Project,
  ProjectDecisionRequest,
  ProjectEvidenceRef,
  ProjectTask,
} from "@/lib/api/projects";

export type ProjectRiskLevel = "none" | "info" | "warn" | "danger";

export type ProjectRiskReasonType =
  | "human_decision"
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
  reasons: ProjectRiskReason[];
  primaryReason?: ProjectRiskReason;
  requiresHuman: boolean;
  waitingSince?: string;
  updatedAt?: string;
};

export type ProjectRiskSummaryMap = Record<string, ProjectRiskSummary>;

export type ProjectRiskInput = {
  project: Project;
  tasks?: ProjectTask[];
  decisions?: ProjectDecisionRequest[];
  evidence?: ProjectEvidenceRef[];
  events?: unknown[];
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
  { value: "human_decision", label: "人工决策" },
  { value: "execution_failed", label: "执行失败" },
  { value: "runtime_or_coordination", label: "协调异常" },
  { value: "evidence_required", label: "证据待补" },
  { value: "sla_waiting", label: "等待超时" },
];

const activeDecisionStatuses = new Set(["pending", "waiting", "requested", "open"]);
const failedTaskStatuses = new Set(["failed", "error", "blocked", "cancelled"]);
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
  human_decision: 5,
  execution_failed: 4,
  runtime_or_coordination: 3,
  evidence_required: 2,
  sla_waiting: 1,
};

const reasonLabels: Record<ProjectRiskReasonType, string> = {
  human_decision: "等待人工决策",
  execution_failed: "执行失败",
  runtime_or_coordination: "协调异常",
  evidence_required: "证据待补",
  sla_waiting: "等待超时",
};

export function deriveProjectRiskSummary(
  input: ProjectRiskInput,
  options: ProjectRiskOptions = {},
): ProjectRiskSummary {
  const reasons: ProjectRiskReason[] = [];
  const { project } = input;

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
    if (
      projectTask.requires_human_approval ||
      waitingHumanTaskStatuses.has(status)
    ) {
      reasons.push({
        id: `task:${projectTask.id}:human_decision`,
        type: "human_decision",
        level: "danger",
        source: "tasks",
        title: projectTask.title,
        detail: projectTask.status,
        sourceId: projectTask.id,
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

  const waitingSince = staleWaitingSince(project, options);
  if (waitingSince) {
    reasons.push({
      id: `project:${project.id}:sla_waiting`,
      type: "sla_waiting",
      level: "warn",
      source: "project",
      title: project.name,
      detail: "running",
      sourceId: project.id,
      waitingSince,
    });
  }

  const primaryReason = pickPrimaryReason(reasons);
  const level = primaryReason?.level ?? "none";

  return {
    projectId: project.id,
    project,
    level,
    state: options.state ?? "ready",
    reasons,
    primaryReason,
    requiresHuman: reasons.some((reason) => reason.type === "human_decision"),
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

export function projectRiskLevelTone(level: ProjectRiskLevel): V3Tone {
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

function normalize(value?: string): string {
  return (value ?? "").trim().toLowerCase();
}

function staleWaitingSince(
  project: Project,
  options: ProjectRiskOptions,
): string | undefined {
  if (project.status !== "running" || !project.updated_at) {
    return undefined;
  }
  const updatedAt = Date.parse(project.updated_at);
  if (Number.isNaN(updatedAt)) {
    return undefined;
  }
  const now = parseTime(options.now ?? new Date());
  if (now === undefined) {
    return undefined;
  }
  const threshold = options.slaWaitMs ?? slaWaitMs;
  return now - updatedAt > threshold ? project.updated_at : undefined;
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
