import type { AnchorHTMLAttributes, ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { ProjectOperationalDetail } from "./project-operational-detail";
import type {
  Project,
  ProjectDecisionRequest,
  ProjectDemand,
  ProjectMember,
  ProjectOverview,
  ProjectTask,
} from "@/lib/api/projects";

vi.mock("@tanstack/react-router", () => {
  type MockLinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
    children: ReactNode;
    params?: Record<string, string>;
    search?: Record<string, string>;
    to: string;
  };

  return {
    Link: ({ children, params, search, to, ...props }: MockLinkProps) => {
      let href = to;
      if (params) {
        for (const [key, value] of Object.entries(params)) {
          href = href.replace(`$${key}`, encodeURIComponent(value));
        }
      }
      const query = search ? `?${new URLSearchParams(search).toString()}` : "";
      return (
        <a {...props} data-router-link="true" href={`${href}${query}`}>
          {children}
        </a>
      );
    },
  };
});

const project: Project = {
  approval_policy: {},
  coordination_policy: {},
  coordination_status: "registered",
  coordination_workflow_id: "project-coordinator:project-1",
  evidence_policy: {},
  goal: "完成客户接入验收闭环",
  human_owner_user_id: "human-owner-1",
  id: "project-1",
  name: "客户接入验收",
  status: "running",
  tenant_id: "tenant-1",
};

function member(overrides: Partial<ProjectMember>): ProjectMember {
  return {
    display_name_snapshot: "成员",
    id: "member-1",
    principal_id: "principal-1",
    principal_type: "digital_employee",
    project_id: "project-1",
    project_role: "executor",
    settings: {},
    status: "active",
    tenant_id: "tenant-1",
    ...overrides,
  };
}

const overview: ProjectOverview = {
  active_tasks: [
    {
      assigned_digital_employee_id: "employee-1",
      id: "task-1",
      project_id: "project-1",
      requires_human_approval: false,
      status: "running",
      summary: "整理客户接入证据",
      tenant_id: "tenant-1",
      title: "整理接入证据",
    },
  ],
  coordination_workflow: {
    status: "registered",
    workflow_id: "project-coordinator:project-1",
  },
  digital_employee_pool: [
    member({
      display_name_snapshot: "验收执行员工",
      id: "member-employee-1",
      principal_id: "employee-1",
      principal_type: "digital_employee",
    }),
  ],
  human_roles: [
    member({
      display_name_snapshot: "负责人甲",
      id: "member-owner-1",
      principal_id: "human-owner-1",
      principal_type: "human_user",
      project_role: "owner",
    }),
  ],
  project,
  recent_events: [],
  status_summary: { current_phase: "running", is_archived: false },
  task_summary: {
    active_tasks: 1,
    completed_tasks: 0,
    failed_tasks: 0,
    pending_human_tasks: 0,
  },
};

const demands: ProjectDemand[] = [
  {
    content: "补充上线验收说明",
    created_at: "2026-07-06T09:00:00Z",
    id: "demand-1",
    project_id: "project-1",
    status: "submitted",
    submitted_by_user_id: "human-owner-1",
    tenant_id: "tenant-1",
    title: "补充上线验收说明",
  },
];

const decisionRequests: ProjectDecisionRequest[] = [
  {
    created_at: "2026-07-06T09:30:00Z",
    decision_type: "risk_review",
    id: "decision-1",
    project_id: "project-1",
    requested_by: "system",
    status_snapshot: "pending",
    summary_snapshot: "需要确认上线风险",
    tenant_id: "tenant-1",
    title_snapshot: "确认上线风险",
    updated_at: "2026-07-06T09:30:00Z",
  },
];

function renderDetail(
  props: Partial<React.ComponentProps<typeof ProjectOperationalDetail>> = {},
) {
  return render(
    <ProjectOperationalDetail
      acceptance={undefined}
      archivePreview={undefined}
      archiveSnapshots={[]}
      artifacts={[]}
      budgetLedger={[]}
      budgetSummary={undefined}
      coordinationJobs={[]}
      decisionRequests={decisionRequests}
      demands={demands}
      evidence={[]}
      events={[]}
      executionSummaries={[]}
      onArchiveProject={vi.fn()}
      onCreateAcceptance={vi.fn()}
      onCreateArchiveSnapshot={vi.fn()}
      onCreateEvidence={vi.fn()}
      onPatchEvidence={vi.fn()}
      onResolveDecision={vi.fn()}
      onSubmitDemand={vi.fn()}
      overview={overview}
      planRevisions={[]}
      project={project}
      reports={[]}
      routeDecisions={[]}
      runtimePlacementPanel={<div>Runtime placement</div>}
      tasks={overview.active_tasks as ProjectTask[]}
      transferRequests={[]}
      {...props}
    />,
  );
}

describe("ProjectOperationalDetail", () => {
  it("renders top-level work hub tabs and navigable project context", async () => {
    const screen = await renderDetail();

    await expect.element(screen.getByRole("tab", { name: "概览" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "任务" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "审批" })).toBeVisible();
    await expect.element(screen.getByRole("link", { name: /在运行总览查看/ })).toHaveAttribute(
      "href",
      "/run-overview?project=project-1",
    );
    await expect.element(screen.getByRole("link", { name: /补充上线验收说明/ })).toHaveAttribute(
      "href",
      "/workflows/demand-1",
    );
    await expect.element(screen.getByRole("link", { name: /整理接入证据/ })).toHaveAttribute(
      "href",
      "/employees/employee-1",
    );
    await expect.element(screen.getByRole("link", { name: /验收执行员工/ })).toHaveAttribute(
      "href",
      "/employees/employee-1",
    );
    await expect.element(screen.getByRole("link", { name: /负责人甲/ })).toHaveAttribute(
      "href",
      "/users",
    );
  });

  it("opens approval tab from query focus and highlights the decision", async () => {
    const onResolveDecision = vi.fn();
    const screen = await renderDetail({
      focusDecisionId: "decision-1",
      initialTab: "approval",
      onResolveDecision,
    });

    await expect.element(screen.getByRole("tab", { name: "审批", selected: true })).toBeVisible();
    const focusedDecision = screen.container.querySelector("[data-focused-decision='true']");
    expect(focusedDecision?.textContent).toContain("确认上线风险");

    await userEvent.click(screen.getByRole("button", { name: "批准 确认上线风险" }));
    expect(onResolveDecision).toHaveBeenCalledWith("decision-1", "approved");
  });
});
