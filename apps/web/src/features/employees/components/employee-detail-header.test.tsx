import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { EmployeeDetailHeader } from "./employee-detail-header";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    ...rest
  }: {
    children: ReactNode;
    to: string;
    [key: string]: unknown;
  }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
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

const stats = {
  total_count: 76,
  succeeded_count: 68,
  failed_count: 5,
  cancelled_count: 3,
  success_rate: 68 / 76,
  avg_duration_sec: 29 * 60,
  p90_duration_sec: 48 * 60,
  last_7d_count: 12,
  prev_7d_count: 10,
};

describe("EmployeeDetailHeader", () => {
  it("renders name, status, KPI strip and config action without start-task", async () => {
    const screen = await render(
      <EmployeeDetailHeader employee={employee} onDelete={vi.fn()} stats={stats} />,
    );

    await expect.element(screen.getByRole("heading", { level: 2, name: "后端实现员" })).toBeVisible();
    await expect.element(screen.getByText("身份：运行中")).toBeVisible();
    await expect.element(screen.getByLabelText("工作节奏摘要")).toBeVisible();
    await expect.element(screen.getByText("12")).toBeVisible();
    await expect.element(screen.getByText("89.5%")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "编辑配置" })).toHaveAttribute(
      "data-variant",
      "primary",
    );
    await expect.element(screen.getByText("无团队归属")).toBeVisible();
    expect(screen.getByRole("button", { name: "开始任务" }).query()).toBeNull();
  });

  it("links to the team detail when team affiliation is present", async () => {
    const screen = await render(
      <EmployeeDetailHeader
        employee={{
          ...employee,
          team_id: "22222222-2222-4222-8222-222222222222",
          team_name: "平台组",
        }}
      />,
    );

    await expect.element(screen.getByRole("link", { name: "平台组" })).toHaveAttribute(
      "href",
      "/teams/$teamId",
    );
  });

  it("shows placeholder dashes when stats are unavailable", async () => {
    const screen = await render(<EmployeeDetailHeader employee={employee} />);

    await expect.element(screen.getByText("成功率")).toBeVisible();
    expect(screen.getByText("—").elements().length).toBeGreaterThan(0);
  });

  it("shows the delete action only when the employee exposes employee.delete", async () => {
    const onDelete = vi.fn();
    const screen = await render(
      <EmployeeDetailHeader
        employee={{ ...employee, allowed_actions: ["employee.delete"] }}
        onDelete={onDelete}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "更多员工操作" }));
    const deleteButton = screen.getByRole("menuitem", { name: "删除员工" });
    await expect.element(deleteButton).toBeVisible();

    await deleteButton.click();
    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it("hides the delete action when employee.delete is not allowed", async () => {
    const screen = await render(
      <EmployeeDetailHeader
        employee={{ ...employee, allowed_actions: ["employee.run.create"] }}
        onDelete={vi.fn()}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "更多员工操作" }));
    await expect.element(screen.getByRole("menuitem", { name: "删除员工" })).not.toBeInTheDocument();
  });

  it("renders a stable avatar image even when metadata.avatar is missing", async () => {
    const screen = await render(<EmployeeDetailHeader employee={employee} />);

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
      />,
    );

    const avatar = screen.getByAltText("后端实现员 的头像");
    await expect
      .element(avatar)
      .toHaveAttribute("src", "/images/digital-employee-avatars/engineer-m-02-256.webp");
  });

  it("opens a large avatar preview dialog from the image_url asset", async () => {
    const screen = await render(
      <EmployeeDetailHeader
        employee={{
          ...employee,
          metadata: {
            avatar: {
              id: "engineer-m-14",
            },
          },
        }}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "查看 后端实现员 的大图头像" }));
    const preview = screen.getByRole("img", { name: "后端实现员 的大图头像" });
    await expect.element(preview).toBeVisible();
    await expect
      .element(preview)
      .toHaveAttribute("src", "/images/digital-employee-avatars/engineer-m-14.webp");
  });
});
