import { describe, expect, it } from "vitest";
import {
  blockerResourceTypeLabel,
  decisionStatusLabel,
  decisionTypeLabel,
  deleteBlockerTypeLabel,
  deliveryOutboxKindLabel,
  deliveryOutboxStatusLabel,
  demandStatusLabel,
  employeeStatusLabel,
  governanceStatusLabel,
  missingObjectLabel,
  projectStatusLabel,
  humanWaitLabel,
  relatedRefMetaLabel,
  riskLevelLabel,
  statusLabel,
  teamStatusLabel,
  workspaceReadyStatusLabel,
  workspaceGitCleanLabel,
  workspaceGitFileCategoryLabel,
  workspaceGitRepoStateLabel,
  workspaceGitSampleErrorLabel,
  logActionLabel,
  logModuleLabel,
  logResourceTypeLabel,
  loginFailureReasonLabel,
  loginLogEventLabel,
  runtimeEventSeverityLabel,
  runtimeEventSourceLabel,
  runtimeEventTypeLabel,
} from "./status-labels";

describe("statusLabel", () => {
  it("maps shared codes and defaults active to 启用中", () => {
    expect(statusLabel("active")).toBe("启用中");
    expect(statusLabel("archived")).toBe("已归档");
    expect(statusLabel("paused")).toBe("已暂停");
    expect(statusLabel("configuring")).toBe("配置中");
    expect(statusLabel("acceptance")).toBe("验收中");
    expect(statusLabel("error")).toBe("异常");
    expect(statusLabel("  READY ")).toBe("就绪");
    expect(statusLabel(undefined)).toBe("未知");
    expect(statusLabel("totally_unknown")).toBe("totally_unknown");
  });

  it("covers approval and decision resolution codes", () => {
    expect(statusLabel("approved")).toBe("已批准");
    expect(statusLabel("request_changes")).toBe("要求修改");
    expect(statusLabel("restaffed")).toBe("已补员");
    expect(statusLabel("exempted")).toBe("已豁免");
    expect(statusLabel("needs_more_evidence")).toBe("需要补充证据");
  });

  it("maps decision types for inbox framing", () => {
    expect(decisionTypeLabel("project_acceptance")).toBe("项目验收");
    expect(decisionTypeLabel("demand_acceptance")).toBe("需求验收");
    expect(decisionTypeLabel("plan_review")).toBe("计划确认");
  });

  it("covers node and engine health codes", () => {
    expect(statusLabel("online")).toBe("在线");
    expect(statusLabel("offline")).toBe("离线");
    expect(statusLabel("ok")).toBe("正常");
  });
});

describe("riskLevelLabel", () => {
  it("maps risk levels and falls back to raw value", () => {
    expect(riskLevelLabel("high")).toBe("高风险");
    expect(riskLevelLabel("medium")).toBe("中风险");
    expect(riskLevelLabel("low")).toBe("低风险");
    expect(riskLevelLabel("blocked")).toBe("阻断");
    expect(riskLevelLabel(undefined)).toBe("未知");
    expect(riskLevelLabel("custom")).toBe("custom");
  });
});

describe("decisionStatusLabel", () => {
  it("maps resolution verbs written back into status_snapshot (decision domain only)", () => {
    // status_snapshot 决议后回填人类决策动词（写入侧既定设计）；域内补词，不进全局表。
    expect(decisionStatusLabel("cancel_downstream")).toBe("取消下游");
    expect(decisionStatusLabel("retry")).toBe("重试");
    expect(decisionStatusLabel("reassign")).toBe("改派");
    expect(decisionStatusLabel("retry_planning")).toBe("重新规划");
    expect(decisionStatusLabel("close_demand")).toBe("关闭需求");
    // 通用键仍走全局词表
    expect(decisionStatusLabel("pending")).toBe("待处理");
    expect(decisionStatusLabel("approved")).toBe("已批准");
    // 全局 statusLabel 不受决策域覆盖污染
    expect(statusLabel("cancel_downstream")).toBe("cancel_downstream");
  });
});

describe("blockerResourceTypeLabel", () => {
  it("maps task graph current_blocker types to Chinese", () => {
    expect(blockerResourceTypeLabel("project_task")).toBe("项目任务");
    expect(blockerResourceTypeLabel("decision_request")).toBe("人工决策");
    expect(blockerResourceTypeLabel("run")).toBe("执行运行");
    expect(blockerResourceTypeLabel(undefined)).toBe("");
    expect(blockerResourceTypeLabel("custom_type")).toBe("custom_type");
  });
});

describe("deleteBlockerTypeLabel", () => {
  it("maps blocker types", () => {
    expect(deleteBlockerTypeLabel("run")).toBe("执行运行");
    expect(deleteBlockerTypeLabel("project_task")).toBe("项目任务");
    expect(deleteBlockerTypeLabel(undefined)).toBe("未知");
  });
});

describe("domain overrides", () => {
  it("teamStatusLabel overrides active", () => {
    expect(teamStatusLabel("active")).toBe("活跃");
    expect(teamStatusLabel("disabled")).toBe("已禁用");
    expect(teamStatusLabel("archived")).toBe("已归档");
  });

  it("governanceStatusLabel overrides active and governance-only codes", () => {
    expect(governanceStatusLabel("active")).toBe("已生效");
    expect(governanceStatusLabel("not_configured")).toBe("未配置");
    expect(governanceStatusLabel("draft_pending")).toBe("草案待批准");
    expect(governanceStatusLabel("needs_update")).toBe("需更新");
  });

  it("employeeStatusLabel overrides active and error", () => {
    expect(employeeStatusLabel("active")).toBe("运行中");
    expect(employeeStatusLabel("error")).toBe("异常");
    expect(employeeStatusLabel("draft")).toBe("草稿");
    expect(employeeStatusLabel("ready")).toBe("就绪");
    expect(employeeStatusLabel("disabled")).toBe("已禁用");
  });

  it("projectStatusLabel covers lifecycle codes", () => {
    expect(projectStatusLabel("draft")).toBe("草稿");
    expect(projectStatusLabel("configuring")).toBe("配置中");
    expect(projectStatusLabel("running")).toBe("已就绪");
    expect(projectStatusLabel("paused")).toBe("已暂停");
    expect(projectStatusLabel("acceptance")).toBe("验收中");
    expect(projectStatusLabel("archived")).toBe("已归档");
  });

  it("workspaceReadyStatusLabel uses domain-specific pending copy", () => {
    expect(workspaceReadyStatusLabel("pending")).toBe("工作区准备中");
    expect(workspaceReadyStatusLabel("ready")).toBe("工作区就绪");
    expect(workspaceReadyStatusLabel("error")).toBe("工作区异常");
    expect(workspaceReadyStatusLabel(undefined)).toBe("未知");
  });

  it("workspace git labels cover clean/dirty and repo middle states", () => {
    expect(workspaceGitCleanLabel("clean")).toBe("工作区干净");
    expect(workspaceGitCleanLabel("dirty")).toBe("工作区脏");
    expect(workspaceGitRepoStateLabel("rebase")).toBe("变基进行中");
    expect(workspaceGitRepoStateLabel("merge")).toBe("合并冲突中");
    expect(workspaceGitFileCategoryLabel("untracked")).toBe("未跟踪");
    expect(workspaceGitSampleErrorLabel("节点离线，显示的是 3 分钟前的现场")).toBe(
      "节点离线，显示的是 3 分钟前的现场",
    );
  });

  it("demandStatusLabel covers demand lifecycle codes", () => {
    expect(demandStatusLabel("submitted")).toBe("待计划");
    // §5.5: planning_pending 拆词后为「排队规划中」,planning_failed 为「规划失败」。
    expect(demandStatusLabel("planning_pending")).toBe("排队规划中");
    expect(demandStatusLabel("planning_failed")).toBe("规划失败");
    expect(demandStatusLabel("planned")).toBe("已计划");
    expect(demandStatusLabel("executing")).toBe("执行中");
    expect(demandStatusLabel("acceptance_pending")).toBe("待验收");
    expect(demandStatusLabel("completed")).toBe("已完成");
    expect(demandStatusLabel("failed")).toBe("失败");
    expect(demandStatusLabel("cancelled")).toBe("已取消");
    expect(demandStatusLabel("recorded")).toBe("已记录");
    expect(demandStatusLabel(undefined)).toBe("未知");
  });
});

describe("missingObjectLabel / relatedRefMetaLabel", () => {
  it("formats D3 missing-name labels with short id", () => {
    expect(missingObjectLabel("project", "25a6b54b-1111-2222-3333-444444444444")).toBe(
      "未命名项目 (25a6b54b…)",
    );
    expect(missingObjectLabel("team", "00000000-0000-0000-0000-000000000101")).toBe(
      "未命名团队 (00000000…)",
    );
  });

  it("maps related ref meta to Chinese actions", () => {
    expect(relatedRefMetaLabel("demand_open")).toBe("打开需求");
    expect(relatedRefMetaLabel("task")).toBe("任务");
    expect(relatedRefMetaLabel("project_open")).toBe("打开项目");
    expect(relatedRefMetaLabel("approval_open")).toBe("打开审批");
    expect(relatedRefMetaLabel("audit")).toBe("审计");
  });
});

describe("humanWaitLabel", () => {
  it("keeps action and object surfaces distinct without triple synonyms", () => {
    expect(humanWaitLabel("inbox_kpi")).toBe("待我处理");
    expect(humanWaitLabel("inbox_badge")).toBe("待我处理");
    expect(humanWaitLabel("project_rail")).toBe("待我决策");
    expect(humanWaitLabel("automations_gate")).toBe("待我处理");
    expect(humanWaitLabel("employee_card")).toBe("待人工确认");
    expect(humanWaitLabel("run_overview_kpi")).toBe("待人工");
    expect(humanWaitLabel("run_overview_badge")).toBe("待人工");
    expect(humanWaitLabel("project_object")).toBe("待决决策");
    expect(humanWaitLabel("inbox_progress_second_person")).toBe("待你");
  });
});

describe("log labels", () => {
  it("maps console log modules, actions, login events, runtime events, and delivery statuses", () => {
    expect(logModuleLabel("auth")).toBe("用户");
    expect(logModuleLabel("authz")).toBe("授权判定");
    expect(logActionLabel("user.create")).toBe("创建用户");
    expect(logActionLabel("team.skill.bind")).toBe("安装公共技能");
    expect(loginLogEventLabel("login_failed")).toBe("登录失败");
    expect(runtimeEventTypeLabel("node_offline")).toBe("节点离线");
    expect(runtimeEventSourceLabel("runtime_node")).toBe("Runtime 节点");
    expect(runtimeEventSeverityLabel("error")).toBe("错误");
    expect(deliveryOutboxStatusLabel("sent")).toBe("已送达");
    expect(deliveryOutboxStatusLabel("skipped_unbound")).toBe("未绑定跳过");
    expect(logResourceTypeLabel("user")).toBe("用户");
    expect(logResourceTypeLabel("project_demand")).toBe("项目需求");
    expect(loginFailureReasonLabel("invalid_credentials")).toBe("账号或密码不正确");
    expect(deliveryOutboxKindLabel("result_notice")).toBe("结果通知");
  });
});
