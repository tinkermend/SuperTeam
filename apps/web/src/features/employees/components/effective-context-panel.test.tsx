import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EffectiveContextPanel } from "./effective-context-panel";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
}));

const employee = {
  id: "employee-1",
  tenant_id: "tenant-1",
  owner_user_id: "user-1",
  employee_type: "backend_engineer",
  provider_type: "codex",
  name: "后端实现员",
  role: "backend_engineer",
  status: "active" as const,
  permission_policy: {},
  context_policy: {},
  approval_policy: {},
  persona_memory_markdown: "# 人格画像\n证据优先",
  risk_level: "medium",
};

describe("EffectiveContextPanel", () => {
  it("renders skill/mcp counts, project constitution, env vars and persona memory status", async () => {
    const onManageCapabilities = vi.fn();
    const screen = await render(
      <EffectiveContextPanel
        employee={employee}
        employeeId="employee-1"
        envVars={{
          isLoading: false,
          isError: false,
          configuredCount: 1,
          totalCount: 2,
          missingNames: ["REDIS_URL"],
        }}
        executionInstance={undefined}
        mcp={{ isLoading: false, isError: false, personalCount: 0, inheritedCount: 1, totalCount: 1 }}
        onManageCapabilities={onManageCapabilities}
        skills={{ isLoading: false, isError: false, personalCount: 1, inheritedCount: 2, totalCount: 3 }}
      />,
    );

    await expect.element(screen.getByText("个人技能 1")).toBeVisible();
    await expect.element(screen.getByText("团队继承技能 2")).toBeVisible();
    await expect.element(screen.getByText("生效总数 3")).toBeVisible();
    await expect.element(screen.getByText("人格记忆：已配置")).toBeVisible();
    await expect.element(screen.getByText("已配置 1")).toBeVisible();
    await expect.element(screen.getByText("REDIS_URL")).toBeVisible();

    // 与头部重复的「状态」行已移除
    expect(screen.getByText("状态", { exact: true }).query()).toBeNull();

    // 技能入口打开管理抽屉而不是跳全局技能页
    await screen.getByRole("button", { name: "管理" }).click();
    expect(onManageCapabilities).toHaveBeenCalledTimes(1);
  });
});
