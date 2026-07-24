import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import type { DigitalEmployeeSchedulingReadiness } from "@/lib/api/employees";
import { EmployeeCapabilityRail } from "./employee-capability-rail";

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
  budget_policy: { daily_token_limit: 12000 },
  risk_level: "medium",
};

const readiness: DigitalEmployeeSchedulingReadiness = {
  employee_id: "employee-1",
  status: "active",
  ready_for_project_scheduling: true,
  project_execution_source: "project_runtime_readiness",
  checks: [
    {
      code: "effective_config",
      status: "passed" as const,
      label: "生效配置",
      message: "已批准",
    },
  ],
  capabilities: {
    skills: { personal_count: 1, inherited_count: 1, missing_required: [] },
    mcp_servers: { personal_count: 1, inherited_count: 0 },
    environment_variables: { configured_count: 1, missing_names: [] },
  },
};

describe("EmployeeCapabilityRail", () => {
  it("renders capability summary, persona, budget and collapsed readiness", async () => {
    const screen = await render(
      <EmployeeCapabilityRail
        employee={employee}
        employeeId="employee-1"
        envVars={{
          isLoading: false,
          isError: false,
          configuredCount: 1,
          totalCount: 1,
          missingNames: [],
        }}
        mcp={{ isLoading: false, isError: false, personalCount: 1, inheritedCount: 0, totalCount: 1 }}
        onRetryReadiness={vi.fn()}
        readiness={readiness}
        readinessError={false}
        readinessLoading={false}
        skills={{ isLoading: false, isError: false, personalCount: 1, inheritedCount: 2, totalCount: 3 }}
      />,
    );

    await expect.element(screen.getByRole("heading", { name: "可调度能力" })).toBeVisible();
    await expect.element(screen.getByText("个人 1 · 继承 2 · 生效 3")).toBeVisible();
    await expect.element(screen.getByText("有内容")).toBeVisible();
    await expect.element(screen.getByText("12,000")).toBeVisible();
    await expect.element(screen.getByText("可进入项目调度池")).toBeVisible();
    await expect.element(screen.getByText("未绑定任何项目")).toBeVisible();
    expect(screen.getByRole("link", { name: "进入项目" }).query()).toBeNull();
  });

  it("lists bound projects with deep links", async () => {
    const screen = await render(
      <EmployeeCapabilityRail
        employee={{
          ...employee,
          project_summary: {
            project_count: 1,
            projects: [
              {
                project_id: "33333333-3333-4333-8333-333333333333",
                name: "试点项目 A",
                status: "active",
                is_member: true,
                active_task_count: 2,
                working_task_count: 1,
                total_task_count: 5,
              },
            ],
          },
        }}
        employeeId="employee-1"
        envVars={{
          isLoading: false,
          isError: false,
          configuredCount: 1,
          totalCount: 1,
          missingNames: [],
        }}
        mcp={{ isLoading: false, isError: false, personalCount: 1, inheritedCount: 0, totalCount: 1 }}
        onRetryReadiness={vi.fn()}
        readiness={readiness}
        readinessError={false}
        readinessLoading={false}
        skills={{ isLoading: false, isError: false, personalCount: 1, inheritedCount: 2, totalCount: 3 }}
      />,
    );

    await expect.element(screen.getByRole("link", { name: /试点项目 A/ })).toHaveAttribute(
      "href",
      "/projects/$projectId",
    );
    await expect.element(screen.getByRole("link", { name: "查看绑定项目" })).toHaveAttribute(
      "href",
      "#employee-bound-projects",
    );
  });
});
