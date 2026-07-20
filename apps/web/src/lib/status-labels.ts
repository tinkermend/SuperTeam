import type {
  DispatchGateStatus,
  ProjectAcceptanceStatus,
  ProjectEvidenceVerificationStatus,
} from "@/lib/api/projects";

const STATUS_LABELS: Record<string, string> = {
  acceptance: "验收中",
  accepted: "已接受",
  active: "启用中",
  approved: "已批准",
  archived: "已归档",
  assigned: "已分派",
  blocked: "已阻塞",
  cancelled: "已取消",
  cancelling: "取消中",
  completed: "已完成",
  configuring: "配置中",
  decomposed: "已分解",
  decomposing: "分解中",
  disabled: "已禁用",
  dispatchable: "可分派",
  dispatching: "分派中",
  done: "已完成",
  draft: "草稿",
  error: "异常",
  exempted: "已豁免",
  failed: "失败",
  held: "已拦截",
  in_progress: "进行中",
  linked: "已关联",
  missing: "缺失",
  needs_more_evidence: "需要补充证据",
  not_applicable: "不适用",
  offline: "离线",
  ok: "正常",
  online: "在线",
  open: "待处理",
  partially_accepted: "部分接受",
  paused: "已暂停",
  passed: "已通过",
  pending: "待处理",
  pending_review: "待复核",
  planned: "已计划",
  planning: "规划中",
  planning_pending: "待计划",
  queued: "排队中",
  ready: "就绪",
  rejected: "已拒绝",
  replan_required: "需要重新计划",
  request_changes: "要求修改",
  requested: "已请求",
  resolved: "已解决",
  restaffed: "已补员",
  retry_later: "稍后重试",
  retained: "已保留",
  retention_pending: "保留待处理",
  running: "运行中",
  stale: "已过期",
  started: "已开始",
  submitted: "已提交",
  succeeded: "已成功",
  success: "已完成",
  superseded: "已被替代",
  timed_out: "已超时",
  unknown: "未知",
  unverified: "自述·不构成证据",
  verified: "已验证",
  waiting: "等待中",
  waiting_human: "等待人工",
};

export function statusLabel(status: string | undefined): string {
  if (!status) {
    return "未知";
  }
  const normalized = status.trim().toLowerCase();
  return STATUS_LABELS[normalized] ?? status;
}

export function taskStatusLabel(status: string | undefined): string {
  return statusLabel(status);
}

export function runStatusLabel(status: string | undefined): string {
  return statusLabel(status);
}

export function decisionStatusLabel(status: string | undefined): string {
  return statusLabel(status);
}

export function dispatchGateStatusLabel(status: DispatchGateStatus): string {
  return statusLabel(status);
}

export function evidenceStatusLabel(status: ProjectEvidenceVerificationStatus): string {
  return statusLabel(status);
}

export function acceptanceStatusLabel(status: ProjectAcceptanceStatus): string {
  return statusLabel(status);
}

function labelWithOverrides(
  status: string | undefined,
  overrides: Record<string, string>,
): string {
  if (!status) {
    return "未知";
  }
  const normalized = status.trim().toLowerCase();
  return overrides[normalized] ?? statusLabel(normalized);
}

export function teamStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    active: "活跃",
  });
}

export function governanceStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    active: "已生效",
    draft_pending: "草案待批准",
    needs_update: "需更新",
    not_configured: "未配置",
  });
}

export function employeeStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    active: "运行中",
    error: "异常",
  });
}

export function projectStatusLabel(status: string | undefined): string {
  return statusLabel(status);
}

const RISK_LEVEL_LABELS: Record<string, string> = {
  blocked: "阻断",
  high: "高风险",
  low: "低风险",
  medium: "中风险",
};

export function riskLevelLabel(level: string | undefined): string {
  if (!level) {
    return "未知";
  }
  const normalized = level.trim().toLowerCase();
  return RISK_LEVEL_LABELS[normalized] ?? level;
}

const PROJECT_ROLE_LABELS: Record<string, string> = {
  executor: "执行者",
  leader: "负责人",
  member: "成员",
  observer: "观察者",
  owner: "负责人",
  reviewer: "评审者",
};

export function projectRoleLabel(role: string | undefined): string {
  if (!role) {
    return "未知";
  }
  const normalized = role.trim().toLowerCase();
  return PROJECT_ROLE_LABELS[normalized] ?? role;
}

const PRINCIPAL_TYPE_LABELS: Record<string, string> = {
  digital_employee: "数字员工",
  human_user: "人类成员",
  team: "团队",
};

export function principalTypeLabel(type: string | undefined): string {
  if (!type) {
    return "未知";
  }
  const normalized = type.trim().toLowerCase();
  return PRINCIPAL_TYPE_LABELS[normalized] ?? type;
}

const DELETE_BLOCKER_TYPE_LABELS: Record<string, string> = {
  project_task: "项目任务",
  run: "执行运行",
};

export function deleteBlockerTypeLabel(type: string | undefined): string {
  if (!type) {
    return "未知";
  }
  return DELETE_BLOCKER_TYPE_LABELS[type] ?? type;
}
