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
  ProjectPlanRevision,
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
    attachments: [],
    content: "补充上线验收说明",
    id: "demand-1",
    project_id: "project-1",
    reviewer: null,
    source_refs: {},
    source_type: "manual",
    status: "submitted",
    submitted_by_user_id: "human-owner-1",
    tenant_id: "tenant-1",
    title: "补充上线验收说明",
  },
];

const decisionRequests: ProjectDecisionRequest[] = [
  {
    approval_request_id: "approval-1",
    decision_type: "risk_review",
    id: "decision-1",
    project_id: "project-1",
    status_snapshot: "pending",
    summary_snapshot: "需要确认上线风险",
    target_user_id: "human-owner-1",
    tenant_id: "tenant-1",
    title_snapshot: "确认上线风险",
  },
];

const longAcceptanceStatement =
  "客户接入证据齐全并可审计，包含合同签署记录、系统对接日志、权限开通确认、双方签字盖章的验收材料副本，以及上线前安全检查与回滚预案确认记录，需经负责人逐项核对后方可进入验收结论阶段。";

const planRevisions: ProjectPlanRevision[] = [
  {
    created_task_ids: [],
    demand_id: "demand-1",
    id: "plan-revision-1",
    payload: {
      plan_acceptance_criteria: [
        {
          id: "evidence_complete",
          satisfied_by: ["collect-evidence"],
          statement: longAcceptanceStatement,
        },
        {
          id: "review_complete",
          satisfied_by: ["review-evidence"],
          statement: "验收结论已由负责人复核。",
        },
      ],
      summary: "生成客户接入验收计划。",
      tasks: [
        {
          employee_selection_reason: "负责收集客户接入材料。",
          planned_task_key: "collect-evidence",
          selected_employee_id: "employee-collector",
          title: "收集接入证据",
        },
        {
          depends_on: ["collect-evidence"],
          employee_selection_reason: "负责复核证据并形成结论。",
          planned_task_key: "review-evidence",
          selected_employee_id: "employee-reviewer",
          title: "复核接入证据",
        },
      ],
    },
    plan_fingerprint: "fingerprint",
    project_id: "project-1",
    review_required: true,
    revision_number: 1,
    status: "pending_review",
    tenant_id: "tenant-1",
    validation_errors: [],
    validation_warnings: [],
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
      onDeleteProject={vi.fn()}
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

  it("shows dispatch order and acceptance criteria from the latest plan revision", async () => {
    const screen = await renderDetail({ planRevisions });
    const dispatchOrder = screen.getByTestId("plan-dispatch-order");
    const acceptanceCriteria = screen.getByTestId("plan-acceptance-criteria");

    await expect.element(screen.getByText("调度顺序")).toBeVisible();
    await expect.element(screen.getByText("验收判据")).toBeVisible();
    await expect.element(dispatchOrder.getByText("收集接入证据")).toBeVisible();
    await expect.element(dispatchOrder.getByText("复核接入证据")).toBeVisible();
    await expect
      .element(acceptanceCriteria.getByText(longAcceptanceStatement))
      .toBeVisible();
    await expect
      .element(acceptanceCriteria.getByText("验收结论已由负责人复核。"))
      .toBeVisible();
    const longStatementElement = screen.container.querySelector(
      "[data-testid='plan-acceptance-criterion-statement-evidence_complete']",
    );
    expect(longStatementElement?.className).toContain("line-clamp-3");
    await userEvent.click(
      screen.getByRole("button", { name: "展开验收判据 evidence_complete" }),
    );
    expect(longStatementElement?.className).not.toContain("line-clamp");
    await expect
      .element(screen.getByRole("button", { name: "收起验收判据 evidence_complete" }))
      .toBeVisible();
    await expect.element(dispatchOrder.getByText("employee-collector")).toBeVisible();
    await expect
      .element(dispatchOrder.getByText("负责收集客户接入材料。"))
      .toBeVisible();
    const dispatchOrderText =
      screen.container.querySelector("[data-testid='plan-dispatch-order']")?.textContent ?? "";
    expect(dispatchOrderText.indexOf("收集接入证据")).toBeLessThan(
      dispatchOrderText.indexOf("复核接入证据"),
    );
    await expect
      .element(acceptanceCriteria.getByText("收集接入证据"))
      .toBeVisible();
    await expect
      .element(acceptanceCriteria.getByText("复核接入证据"))
      .toBeVisible();
  });

  it("orders tasks using blocked_by_keys when depends_on is absent", async () => {
    const blockedByKeysRevision: ProjectPlanRevision = {
      ...planRevisions[0],
      payload: {
        ...planRevisions[0].payload,
        tasks: [
          {
            employee_selection_reason: "负责收集客户接入材料。",
            planned_task_key: "collect-evidence",
            selected_employee_id: "employee-collector",
            title: "收集接入证据",
          },
          {
            blocked_by_keys: ["collect-evidence"],
            employee_selection_reason: "负责复核证据并形成结论。",
            planned_task_key: "review-evidence",
            selected_employee_id: "employee-reviewer",
            title: "复核接入证据",
          },
        ],
      },
    };
    const screen = await renderDetail({ planRevisions: [blockedByKeysRevision] });
    const dispatchOrderText =
      screen.container.querySelector("[data-testid='plan-dispatch-order']")?.textContent ?? "";
    expect(dispatchOrderText.indexOf("收集接入证据")).toBeLessThan(
      dispatchOrderText.indexOf("复核接入证据"),
    );
  });

  it("shows archive and delete actions in the detail header when allowed", async () => {
    const onArchiveProject = vi.fn();
    const onDeleteProject = vi.fn();
    const screen = await renderDetail({
      onArchiveProject,
      onDeleteProject,
      project: {
        ...project,
        allowed_actions: ["project.archive", "project.delete"],
      },
    });

    const archiveButton = screen.getByRole("button", { name: "归档项目" });
    const deleteButton = screen.getByRole("button", { name: "删除项目" });
    await expect.element(archiveButton).toHaveAttribute("data-variant", "outline");
    await expect.element(deleteButton).toHaveAttribute("data-variant", "danger");

    await archiveButton.click();
    await deleteButton.click();
    expect(onArchiveProject).toHaveBeenCalledTimes(1);
    expect(onDeleteProject).toHaveBeenCalledTimes(1);
  });

  it("hides archive and delete when allowed_actions omit them", async () => {
    const screen = await renderDetail({
      onDeleteProject: vi.fn(),
      project: {
        ...project,
        allowed_actions: ["project.demand.submit"],
      },
    });

    await expect.element(screen.getByRole("button", { name: "归档项目" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "删除项目" })).not.toBeInTheDocument();
  });
});
