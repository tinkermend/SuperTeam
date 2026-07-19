import { describe, expect, it } from "vitest";
import {
  deleteBlockerTypeLabel,
  employeeStatusLabel,
  governanceStatusLabel,
  projectStatusLabel,
  riskLevelLabel,
  statusLabel,
  teamStatusLabel,
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
    expect(projectStatusLabel("running")).toBe("运行中");
    expect(projectStatusLabel("paused")).toBe("已暂停");
    expect(projectStatusLabel("acceptance")).toBe("验收中");
    expect(projectStatusLabel("archived")).toBe("已归档");
  });
});
