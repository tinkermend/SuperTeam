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
  provider_type: "codex",
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
    const screen = await render(
      <EmployeeDetailHeader
        employee={employee}
        onDelete={vi.fn()}
        onStartTask={onStartTask}
      />,
    );

    await expect.element(screen.getByRole("heading", { level: 2, name: "后端实现员" })).toBeVisible();
    await expect.element(screen.getByText("身份：运行中")).toBeVisible();
    const startButton = screen.getByRole("button", { name: "开始任务" });
    await expect.element(startButton).toHaveAttribute("data-variant", "outline");

    await startButton.click();
    expect(onStartTask).toHaveBeenCalledTimes(1);
  });

  it("shows the delete action only when the employee exposes employee.delete", async () => {
    const onDelete = vi.fn();
    const screen = await render(
      <EmployeeDetailHeader
        employee={{ ...employee, allowed_actions: ["employee.delete"] }}
        onDelete={onDelete}
        onStartTask={vi.fn()}
      />,
    );

    const deleteButton = screen.getByRole("button", { name: "删除员工" });
    await expect.element(deleteButton).toHaveAttribute("data-variant", "danger");

    await deleteButton.click();
    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it("hides the delete action when employee.delete is not allowed", async () => {
    const screen = await render(
      <EmployeeDetailHeader
        employee={{ ...employee, allowed_actions: ["employee.run.create"] }}
        onDelete={vi.fn()}
        onStartTask={vi.fn()}
      />,
    );

    await expect.element(screen.getByRole("button", { name: "删除员工" })).not.toBeInTheDocument();
  });

  it("renders a stable avatar image even when metadata.avatar is missing", async () => {
    const screen = await render(
      <EmployeeDetailHeader employee={employee} onStartTask={vi.fn()} />,
    );

    const avatar = screen.getByAltText("后端实现员 的头像");
    await expect.element(avatar).toBeVisible();
    expect(avatar.element().getAttribute("src")).toMatch(
      /\/images\/digital-employee-avatars\/.+-256\.webp$/,
    );
  });

  it("prefers the built-in asset resolved from metadata.avatar.id", async () => {
    const screen = await render(
      <EmployeeDetailHeader
        employee={{
          ...employee,
          metadata: {
            avatar: {
              id: "engineer-f-01",
            },
          },
        }}
        onStartTask={vi.fn()}
      />,
    );

    const avatar = screen.getByAltText("后端实现员 的头像");
    await expect
      .element(avatar)
      .toHaveAttribute("src", "/images/digital-employee-avatars/engineer-f-01-256.webp");
  });

  it("resolves the built-in asset from top-level metadata.avatar_asset_id", async () => {
    const screen = await render(
      <EmployeeDetailHeader
        employee={{
          ...employee,
          metadata: {
            avatar_asset_id: "engineer-m-02",
          },
        }}
        onStartTask={vi.fn()}
      />,
    );

    const avatar = screen.getByAltText("后端实现员 的头像");
    await expect
      .element(avatar)
      .toHaveAttribute("src", "/images/digital-employee-avatars/engineer-m-02-256.webp");
  });
});
