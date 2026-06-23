import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import type {
  ProjectTask,
  ProjectTaskGraphEdge,
  ProjectTaskGraphEmployee,
  ProjectTaskGraphStageSummary,
} from "@/lib/api/projects";
import { PlanTaskGraph } from "./plan-task-graph";

function task(overrides: Partial<ProjectTask> & { id: string; title: string }): ProjectTask {
  return {
    tenant_id: "tenant-1",
    project_id: "project-1",
    status: "pending",
    requires_human_approval: false,
    ...overrides,
  } as ProjectTask;
}

describe("PlanTaskGraph", () => {
  it("renders empty state when there are no nodes", async () => {
    const screen = await render(<PlanTaskGraph nodes={[]} />);
    await expect.element(screen.getByText("暂无协调任务计划")).toBeInTheDocument();
    await expect
      .element(screen.container.querySelector<HTMLElement>('[data-slot="v3-soft-card"]'))
      .toBeInTheDocument();
  });

  it("groups tasks by stage and shows stage titles from summaries", async () => {
    const nodes: ProjectTask[] = [
      task({ id: "t1", title: "需求分析", stage_index: 0 }),
      task({ id: "t2", title: "方案实现", stage_index: 1 }),
    ];
    const stageSummaries: ProjectTaskGraphStageSummary[] = [
      {
        stage_index: 0,
        title: "调研阶段",
        total_nodes: 1,
        completed_nodes: 0,
        running_nodes: 0,
        waiting_human_nodes: 0,
        blocked_nodes: 0,
      },
    ];
    const screen = await render(
      <PlanTaskGraph nodes={nodes} stageSummaries={stageSummaries} />,
    );
    // Named stage from summary, fallback name for the unnamed stage.
    await expect.element(screen.getByText("调研阶段")).toBeInTheDocument();
    await expect.element(screen.getByText("阶段 2")).toBeInTheDocument();
    await expect.element(screen.getByText("需求分析")).toBeInTheDocument();
    await expect.element(screen.getByText("方案实现")).toBeInTheDocument();
    expect(screen.container.querySelectorAll<HTMLElement>('[data-slot="v3-soft-card"]').length).toBe(2);
    expect(screen.container.querySelectorAll<HTMLElement>('[data-slot="v3-status-pill"]').length).toBeGreaterThan(0);
  });

  it("annotates dependencies with blocker task titles", async () => {
    const nodes: ProjectTask[] = [
      task({ id: "t1", title: "数据准备", stage_index: 0 }),
      task({ id: "t2", title: "模型训练", stage_index: 1 }),
    ];
    const edges: ProjectTaskGraphEdge[] = [
      { dependent_task_id: "t2", blocker_task_id: "t1", edge_status: "open" },
    ];
    const screen = await render(<PlanTaskGraph nodes={nodes} edges={edges} />);
    await expect.element(screen.getByText("依赖：数据准备")).toBeInTheDocument();
  });

  it("maps assigned employee id to display name and flags approval", async () => {
    const nodes: ProjectTask[] = [
      task({
        id: "t1",
        title: "上线发布",
        stage_index: 0,
        assigned_digital_employee_id: "emp-1",
        requires_human_approval: true,
      }),
    ];
    const employees: ProjectTaskGraphEmployee[] = [
      {
        digital_employee_id: "emp-1",
        display_name: "发布助手",
        project_role: "executor",
        status: "active",
      },
    ];
    const screen = await render(<PlanTaskGraph nodes={nodes} employees={employees} />);
    await expect.element(screen.getByText("发布助手")).toBeInTheDocument();
    await expect.element(screen.getByText("需审批")).toBeInTheDocument();
  });
});
