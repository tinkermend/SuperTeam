import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { DemandTaskTable } from "./demand-task-table";
import type { ProjectDemand, ProjectDemandDossier, ProjectTaskGraph } from "@/lib/api/projects";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    ...props
  }: {
    children: React.ReactNode;
    to: string;
  } & Record<string, unknown>) => (
    <a href={typeof to === "string" ? to : "/inbox"} {...props}>
      {children}
    </a>
  ),
}));

const demand: ProjectDemand = {
  attachments: [],
  content: "验收",
  id: "demand-1",
  project_id: "project-1",
  reviewer: null,
  source_refs: {},
  source_type: "manual",
  status: "executing",
  submitted_by_user_id: "human-1",
  tenant_id: "tenant-1",
  title: "补充验收材料",
};

const graph: ProjectTaskGraph = {
  blocking_facts: [],
  decision_requests: [
    {
      approval_request_id: "approval-1",
      decision_type: "dispatch_release",
      id: "decision-1",
      project_id: "project-1",
      project_task_id: "task-1",
      status_snapshot: "pending",
      target_user_id: "human-1",
      tenant_id: "tenant-1",
      title_snapshot: "放行执行",
    },
  ],
  dispatch_gates: [],
  edges: [],
  employees: [
    {
      digital_employee_id: "employee-1",
      display_name: "验收执行员工",
      project_role: "executor",
      status: "active",
    },
  ],
  execution_summaries: [],
  nodes: [
    {
      assigned_digital_employee_id: "employee-1",
      expected_outputs: [],
      handoff_contract: {},
      id: "task-1",
      input_requirements: {},
      planner_metadata: {},
      project_id: "project-1",
      requires_human_approval: false,
      status: "waiting_human",
      tenant_id: "tenant-1",
      title: "整理接入证据",
    },
    {
      dismissed_at: "2026-08-01T00:00:00Z",
      expected_outputs: [],
      handoff_contract: {},
      id: "task-dismissed",
      input_requirements: {},
      planner_metadata: {},
      project_id: "project-1",
      requires_human_approval: false,
      status: "failed",
      tenant_id: "tenant-1",
      title: "已清理任务",
    },
  ],
  recent_events: [],
  runs: [],
};

const dossier = {
  demand,
  handoff_summary: {
    assessments: [
      {
        project_task_id: "task-1",
        status: "partial",
        deliverables: [{ name: "简报", verdict: "delivered" }],
      },
    ],
    fulfilled: 0,
    partial: 1,
    unfulfilled: 0,
    unknown: 0,
  },
  rail: {
    slots: [
      {
        kind: "artifact",
        title: "工件",
        items: [
          {
            id: "art-1",
            project_task_id: "task-1",
            state: "delivered",
            title: "验收简报",
          },
        ],
      },
    ],
  },
} as ProjectDemandDossier;

describe("DemandTaskTable", () => {
  it("renders demand as the parent row and graph nodes as children with handling and assets", async () => {
    const onOpenTask = vi.fn();
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <DemandTaskTable
          demand={demand}
          dossier={dossier}
          graph={graph}
          onOpenTask={onOpenTask}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("需求流程 · 1 条子任务")).toBeVisible();
    await expect.element(screen.getByText("已清理任务")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "整理接入证据" })).toBeVisible();
    await expect.element(screen.getByText("执行放行 · 待决")).toBeVisible();
    await expect.element(screen.getByText("工件 1")).toBeVisible();
  });
});
