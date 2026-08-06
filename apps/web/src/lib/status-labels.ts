import type {
  DispatchGateStatus,
  ProjectAcceptanceStatus,
  ProjectEvidenceVerificationStatus,
} from "@/lib/api/projects";

const STATUS_LABELS: Record<string, string> = {
  acceptance: "验收中",
  accepted: "已接受",
  active: "启用中",
  actor_deactivated: "创建者账号已停用",
  actor_removed_from_project: "创建者已离开项目",
  approved: "已批准",
  archived: "已归档",
  automation_alert: "自动化告警",
  assigned: "已分派",
  blocked: "已阻塞",
  cancelled: "已取消",
  casting_invalidated: "编制失效",
  cancelling: "取消中",
  chat: "对话",
  completed: "已完成",
  configuring: "配置中",
  consecutive_fire_failures: "连续发起失败 3 次",
  decomposed: "已分解",
  decomposing: "分解中",
  deleted: "已删除",
  disabled: "已禁用",
  dismissed: "已清理",
  dispatchable: "可分派",
  dispatching: "分派中",
  done: "已完成",
  draft: "草稿",
  dependency_closure: "依赖补全",
  employee: "员工",
  project_binding: "项目绑定",
  error: "异常",
  exempted: "已豁免",
  failed: "失败",
  held: "已拦截",
  in_progress: "进行中",
  linked: "已关联",
  loop: "循环",
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
  plan: "计划",
  planned: "已计划",
  planning: "规划中",
  planning_pending: "排队规划中",
  planning_failed: "规划失败",
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
  skipped_disabled: "已跳过（规则停用）",
  skipped_overlap: "已跳过（上次未结束）",
  stale: "已过期",
  started: "已开始",
  submitted: "已提交",
  succeeded: "已成功",
  success: "已完成",
  team: "团队",
  superseded: "已被替代",
  timed_out: "已超时",
  unknown: "未知",
  unverified: "自述·不构成证据",
  user_disabled: "已手动停用",
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

/**
 * 决策请求 status_snapshot 是「pending → 人类决策动词」双语义字段：决议后写入侧
 * 回填 resolution 动词（既定设计，不改写入侧）。这些动词只在决策域出现，
 * 用域内覆盖补词，不进全局 STATUS_LABELS（避免污染 retry 等通用键）。
 * 措辞与收件箱动作文案对齐（inbox/components/action-format.ts、project-ops-home BlockerActions）。
 */
export function decisionStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    cancel_downstream: "取消下游",
    close_demand: "关闭需求",
    reassign: "改派",
    retry: "重试",
    retry_planning: "重新规划",
  });
}

export function dispatchGateStatusLabel(status: DispatchGateStatus): string {
  return statusLabel(status);
}

export function evidenceStatusLabel(status: ProjectEvidenceVerificationStatus): string {
  // 核验元数据，不是项目/任务业务进度态；文案加「核验」前缀避免与待办状态混淆。
  return labelWithOverrides(status, {
    linked: "核验·已关联",
    rejected: "核验·未通过",
    submitted: "核验·待确认",
    verified: "核验·已通过",
  });
}

/** 工件保留策略元数据，禁止复用全局「待处理」以免与业务待办混淆。 */
export function retentionStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    deleted: "已删除",
    expired: "已过期",
    pending: "保留未决",
    retained: "长期保留",
    retention_pending: "保留未决",
  });
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
  return labelWithOverrides(status, {
    // 项目生命周期 running = 已可调度/推进；与任务/run 的「运行中」语义区分。
    running: "已就绪",
  });
}

/** 项目工作区首启就绪态；pending 不复用全局「待处理」，以免与审批待办混淆。 */
export function workspaceReadyStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    error: "工作区异常",
    pending: "工作区准备中",
    ready: "工作区就绪",
  });
}

export function demandStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    acceptance_pending: "待验收",
    executing: "执行中",
    recorded: "已记录",
    submitted: "待计划",
  });
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

const DECISION_TYPE_LABELS: Record<string, string> = {
  budget_approval: "预算审批",
  casting_expansion: "扩编请求",
  demand_acceptance: "需求验收",
  plan_review: "计划确认",
  planning_gap: "规划缺口",
  project_acceptance: "项目验收",
  project_task_acceptance: "任务验收",
  project_task_approval: "任务审批",
  project_task_clarification: "任务澄清",
  project_task_recovery: "任务恢复",
  route_review: "路由复核",
};

/** 收件箱/决策卡上的 decision_type 面向用户显示。 */
export function decisionTypeLabel(type: string | undefined): string {
  if (!type) {
    return "未知";
  }
  const normalized = type.trim().toLowerCase();
  return DECISION_TYPE_LABELS[normalized] ?? type;
}

// 规范化 HumanTask kind(§4.2)的中文名。kind 是服务端从 decision_type 映射出的读模型
// 元数据(见 InboxItem.kind),控制台按此分组/命名人类待办。
// 必须与 contracts/control-plane/human-task-kind-labels.json 逐键逐值一致
// （见 human-task-kind-labels.guard.test.ts；2026-07-25 §5.4）。
export const HUMAN_TASK_KIND_LABELS: Record<string, string> = {
  plan_review: "计划确认",
  dispatch_release: "执行放行",
  downstream_release: "下游放行",
  acceptance_sign: "验收签署",
  closure_confirm: "结项确认",
  planning_failed: "规划失败",
  planning_gap: "规划缺口",
  casting_expansion: "扩编请求",
  task_failure_recovery: "任务失败恢复",
};

/** HumanTask.kind 面向用户显示(§4.2);未登记 kind 回退 decisionTypeLabel。 */
export function humanTaskKindLabel(kind: string | undefined): string {
  if (!kind) {
    return "未知";
  }
  const normalized = kind.trim().toLowerCase();
  return HUMAN_TASK_KIND_LABELS[normalized] ?? decisionTypeLabel(kind);
}

// 人类待办层级(§4.1)中文名。
const HUMAN_TASK_LAYER_LABELS: Record<string, string> = {
  task: "任务级",
  demand: "需求级",
  project: "项目级",
};

/** HumanTask.layer 面向用户显示(§4.1)。 */
export function humanTaskLayerLabel(layer: string | undefined): string {
  if (!layer) {
    return "";
  }
  const normalized = layer.trim().toLowerCase();
  return HUMAN_TASK_LAYER_LABELS[normalized] ?? layer;
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

const PERMISSION_RESOURCE_TYPE_LABELS: Record<string, string> = {
  digital_employee_config_revision: "数字员工配置修订",
  team_privileged_role_request: "团队特权角色申请",
};

export function permissionResourceTypeLabel(type: string | undefined): string {
  if (!type) {
    return "未知";
  }
  const normalized = type.trim().toLowerCase();
  return PERMISSION_RESOURCE_TYPE_LABELS[normalized] ?? type;
}

const PERMISSION_CATEGORY_LABELS: Record<string, string> = {
  permission: "权限审批",
  project_task: "任务验收",
};

export function permissionCategoryLabel(category: string | undefined): string {
  if (!category) {
    return "未知";
  }
  const normalized = category.trim().toLowerCase();
  return PERMISSION_CATEGORY_LABELS[normalized] ?? category;
}

const PERMISSION_DECISION_LABELS: Record<string, string> = {
  approved: "同意",
  rejected: "驳回",
  needs_more_evidence: "要求补证",
};

export function permissionDecisionLabel(decision: string | undefined): string {
  if (!decision) {
    return "未知";
  }
  const normalized = decision.trim().toLowerCase();
  return PERMISSION_DECISION_LABELS[normalized] ?? decision;
}

const PRIVILEGED_ROLE_LABELS: Record<string, string> = {
  admin: "管理员",
  approver: "审批人",
  owner: "负责人",
};

export function privilegedRoleLabel(role: string | undefined): string {
  if (!role) {
    return "未知";
  }
  const normalized = role.trim().toLowerCase();
  return PRIVILEGED_ROLE_LABELS[normalized] ?? role;
}

const TENANT_ROLE_LABELS: Record<string, string> = {
  admin: "管理员",
  member: "成员",
  owner: "所有者",
  viewer: "观察者",
};

export function tenantRoleLabel(role: string | undefined): string {
  if (!role) {
    return "未知";
  }
  const normalized = role.trim().toLowerCase();
  return TENANT_ROLE_LABELS[normalized] ?? role;
}

// 任务图 current_blocker.type（CP 写入 project_task / decision_request）；
// run 预留给执行运行类阻塞。面向用户不得英文直出。
const BLOCKER_RESOURCE_TYPE_LABELS: Record<string, string> = {
  decision_request: "人工决策",
  project_task: "项目任务",
  run: "执行运行",
};

export function blockerResourceTypeLabel(type: string | undefined): string {
  if (!type) {
    return "";
  }
  const normalized = type.trim().toLowerCase();
  return BLOCKER_RESOURCE_TYPE_LABELS[normalized] ?? type;
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

// 团队审计动作词表：团队审计流是面向人的变更流水，动作名不得裸露英文枚举。
// 新增团队维度审计动作时同步补键（宪法「新增枚举的完成定义」）。
const TEAM_AUDIT_ACTION_LABELS: Record<string, string> = {
  "team.create": "创建团队",
  "team.delete": "删除团队",
  "team.delete.confirmed": "确认彻底删除",
  "team.restore": "恢复团队",
  "team.update": "修改团队身份",
  "team.constitution.update": "修改团队宪法",
  "team.member.add": "添加人类成员",
  "team.member.remove": "移除人类成员",
  "team.member.change_role": "变更成员角色",
  "team.member.grant_privileged_role": "授予特权角色",
  "team.skill.bind": "安装公共技能",
  "team.skill.unbind": "移除公共技能",
  "team.mcp.bind": "绑定公共 MCP",
  "team.mcp.unbind": "解绑公共 MCP",
  "team.digital_employee.bind": "收编数字员工",
  "team.digital_employee.unbind": "移出数字员工",
  "team.digital_employee.transfer_in": "数字员工转入",
  "team.digital_employee.transfer_out": "数字员工转出",
};

export function teamAuditActionLabel(action: string | undefined): string {
  if (!action) {
    return "未知";
  }
  return TEAM_AUDIT_ACTION_LABELS[action.trim()] ?? action;
}

// 团队宪法规则分类词表。D9：宪法只是注入 provider 提示词的软提醒，不触发任何门禁
// 或审批。标签刻意不用"禁止/必须/需审批"这类暗示强制力的词——尤其"需审批"，
// 平台里真有审批流程（权限中心）时那个词才该出现；这里没有，用了就是过度承诺。
const CONSTITUTION_CATEGORY_LABELS: Record<string, string> = {
  forbid: "尽量避免",
  must: "尽量遵循",
  require_approval: "重点提醒",
};

export function constitutionCategoryLabel(category: string | undefined): string {
  if (!category) {
    return "未知";
  }
  return CONSTITUTION_CATEGORY_LABELS[category] ?? category;
}

// 一单卷宗（spec 2026-07-29 R2）。时间线 kind 的中文由服务端在 title 上给全，
// 这里只覆盖前端自己要显示 kind 的地方（分组/筛选/无障碍标签）与右轨槽名、
// 密度名。服务端已有同名词表（internal/project/event_narrative.go、
// DossierRailKindLabel）——两边必须一起改，别只改一头。
const DOSSIER_TIMELINE_KIND_LABELS: Record<string, string> = {
  coordination_blocked: "协调受阻",
  coordination_started: "协调开始",
  decision_opened: "待人工决策",
  decision_resolved: "决策已处理",
  demand_submitted: "需求已提交",
  dispatch_blocked: "派发受阻",
  other: "协调更新",
  plan_accepted: "计划已确认",
  plan_change_requested: "要求变更",
  plan_ready: "计划已生成",
  plan_rejected: "计划被驳回",
  result_accepted: "结果已采纳",
  result_recorded: "结果已记录",
  result_rejected: "结果被驳回",
  staffing_gap: "选角缺口",
  task_cancelled: "任务取消",
  task_completed: "任务完成",
  task_created: "任务创建",
  task_dispatched: "任务开始",
  task_failed: "任务失败",
  task_waiting_human: "等待人工",
};

export function dossierTimelineKindLabel(kind: string | undefined): string {
  if (!kind) {
    return "协调更新";
  }
  return DOSSIER_TIMELINE_KIND_LABELS[kind] ?? "协调更新";
}

// 右轨槽名。未知 kind 回落「交付物」——技术键不得当标题吐给用户。
const DOSSIER_RAIL_KIND_LABELS: Record<string, string> = {
  artifact_ref: "工件",
  branch_ref: "分支",
  conclusion: "结论",
  decision_record: "决策记录",
  evidence_ref: "证据",
  git_commit: "提交",
  report_ref: "报告",
};

export function dossierRailKindLabel(kind: string | undefined): string {
  if (!kind) {
    return "交付物";
  }
  return DOSSIER_RAIL_KIND_LABELS[kind] ?? "交付物";
}

const DOSSIER_RAIL_ITEM_STATE_LABELS: Record<string, string> = {
  delivered: "已交付",
  info: "参考",
  missing: "未交付",
  unknown: "暂无声明",
};

export function dossierRailItemStateLabel(state: string | undefined): string {
  if (!state) {
    return "暂无声明";
  }
  return DOSSIER_RAIL_ITEM_STATE_LABELS[state] ?? "暂无声明";
}

const DOSSIER_DENSITY_LABELS: Record<string, string> = {
  drive: "驱动",
  inspect: "巡检",
};

export function dossierDensityLabel(density: string | undefined): string {
  if (!density) {
    return "驱动";
  }
  return DOSSIER_DENSITY_LABELS[density] ?? "驱动";
}
