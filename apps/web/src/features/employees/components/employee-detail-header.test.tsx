import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeDetailHeader } from "./employee-detail-header";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
}));

const employee = {
  id: "11111111-1111-4111-8111-111111111111",
  tenant_id: "tenant-1",
  owner_user_id: "user-1",
  employee_type: "backend_engineer",
  name: "后端实现员",
  role: "backend_engineer",
  description: "负责后端实现、接口补全、数据库迁移与测试修复",
  status: "active" as const,
  permission_policy: {},
  context_policy: {},
  approval_policy: {},
  risk_level: "medium",
};

describe("EmployeeDetailHeader", () => {
  it("renders name, status and triggers start task", async () => {
    const onStartTask = vi.fn();
    const onManageCapabilities = vi.fn();
    const screen = await render(
      <EmployeeDetailHeader employee={employee} onManageCapabilities={onManageCapabilities} onStartTask={onStartTask} />,
    );

    await expect.element(screen.getByRole("heading", { level: 2, name: "后端实现员" })).toBeVisible();
    await expect.element(screen.getByText("active")).toBeVisible();
    await screen.getByRole("button", { name: "开始任务" }).click();
    expect(onStartTask).toHaveBeenCalledTimes(1);
  });
});
