import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import { EmployeeRunHistoryTable } from "./employee-run-history-table";
import type { DigitalEmployeeRunListResult } from "@/lib/api/employees";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    search,
    to,
    ...props
  }: {
    children: ReactNode;
    search?: Record<string, string | undefined>;
    to: string;
  } & React.AnchorHTMLAttributes<HTMLAnchorElement>) => {
    const query = search
      ? `?${new URLSearchParams(Object.entries(search).filter((entry): entry is [string, string] => Boolean(entry[1]))).toString()}`
      : "";
    return (
      <a href={`${to}${query}`} {...props}>
        {children}
      </a>
    );
  },
}));

const result: DigitalEmployeeRunListResult = {
  items: [
    {
      id: "run-1",
      tenant_id: "tenant-1",
      task_id: "task-1",
      digital_employee_id: "employee-1",
      execution_instance_id: "instance-1",
      runtime_node_id: "node-uuid-1",
      node_id: "node-a",
      command_id: "cmd-1",
      provider_type: "claude_code",
      status: "completed",
      result: {},
      diagnostic: {},
      work_products: [],
      session_state: {},
      timed_out: false,
      run_kind: "task",
      task_title: "数据库迁移脚本校验",
      project_name: "数据库平台",
      work_product_count: 2,
      duration_sec: 1095,
      created_at: "2026-05-20T10:32:00Z",
    },
  ],
  total_count: 1,
  filters: {
    statuses: [{ value: "completed", label: "已完成" }],
    projects: [{ value: "project-1", label: "数据库平台" }],
  },
};

const chatResult: DigitalEmployeeRunListResult = {
  items: [
    {
      ...result.items[0],
      id: "run-2",
      run_kind: "chat",
      task_title: "与员工的即时对话",
    },
  ],
  total_count: 1,
  filters: result.filters,
};

describe("EmployeeRunHistoryTable", () => {
  it("renders run rows and triggers row click", async () => {
    const onRowClick = vi.fn();
    const screen = await render(
      <EmployeeRunHistoryTable
        employeeId="employee-1"
        onPageChange={vi.fn()}
        onRetry={vi.fn()}
        onRowClick={onRowClick}
        onRunKindFilterChange={vi.fn()}
        onStatusFilterChange={vi.fn()}
        page={1}
        pageSize={10}
        result={result}
        runKindFilter={undefined}
        statusFilter={undefined}
      />,
    );

    await expect.element(screen.getByText("数据库迁移脚本校验")).toBeVisible();
    await expect.element(screen.getByText("数据库平台")).toBeVisible();
    await expect.element(screen.getByText("已完成")).toBeVisible();
    await expect
      .element(screen.getByRole("row", { name: /数据库迁移脚本校验/ }).getByText("任务", { exact: true }))
      .toBeVisible();
    await expect.element(screen.getByRole("link", { name: "在运行总览查看" })).toHaveAttribute(
      "href",
      "/run-overview?employee=employee-1",
    );
    await userEvent.click(screen.getByText("数据库迁移脚本校验"));
    expect(onRowClick).toHaveBeenCalledWith(result.items[0]);
  });

  it("shows empty state when there are no runs", async () => {
    const screen = await render(
      <EmployeeRunHistoryTable
        employeeId="employee-1"
        onPageChange={vi.fn()}
        onRetry={vi.fn()}
        onRowClick={vi.fn()}
        onRunKindFilterChange={vi.fn()}
        onStatusFilterChange={vi.fn()}
        page={1}
        pageSize={10}
        result={{ items: [], total_count: 0, filters: { statuses: [], projects: [] } }}
        runKindFilter={undefined}
        statusFilter={undefined}
      />,
    );

    await expect.element(screen.getByText("暂无数据")).toBeVisible();
  });

  it("renders a 对话 badge for chat-kind runs", async () => {
    const screen = await render(
      <EmployeeRunHistoryTable
        employeeId="employee-1"
        onPageChange={vi.fn()}
        onRetry={vi.fn()}
        onRowClick={vi.fn()}
        onRunKindFilterChange={vi.fn()}
        onStatusFilterChange={vi.fn()}
        page={1}
        pageSize={10}
        result={chatResult}
        runKindFilter={undefined}
        statusFilter={undefined}
      />,
    );

    await expect.element(screen.getByText("与员工的即时对话")).toBeVisible();
    await expect
      .element(screen.getByRole("row", { name: /与员工的即时对话/ }).getByText("对话", { exact: true }))
      .toBeVisible();
  });

  it("re-queries with run_kind=chat when the 对话 chip is clicked", async () => {
    const onRunKindFilterChange = vi.fn();
    const screen = await render(
      <EmployeeRunHistoryTable
        employeeId="employee-1"
        onPageChange={vi.fn()}
        onRetry={vi.fn()}
        onRowClick={vi.fn()}
        onRunKindFilterChange={onRunKindFilterChange}
        onStatusFilterChange={vi.fn()}
        page={1}
        pageSize={10}
        result={result}
        runKindFilter={undefined}
        statusFilter={undefined}
      />,
    );

    await userEvent.click(screen.getByText("对话"));
    expect(onRunKindFilterChange).toHaveBeenCalledWith("chat");
  });
});
