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
  coordination_policy: {},
  coordination_status: "registered",
  coordination_workflow_id: "project-coordinator:project-1",
  goal: "完成客户接入验收闭环",
  human_owner_user_id: "human-owner-1",
  id: "project-1",
  name: "客户接入验收",
  status: "running",
  tenant_id: "tenant-1",
  workspace_ready_status: "ready",
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

function detailElement(
  props: Partial<React.ComponentProps<typeof ProjectOperationalDetail>> = {},
) {
  return (
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
    />
  );
}

function renderDetail(
  props: Partial<React.ComponentProps<typeof ProjectOperationalDetail>> = {},
) {
  return render(detailElement(props));
}

describe("ProjectOperationalDetail", () => {
  it("renders section nav with workbench default and navigable project context", async () => {
    const screen = await renderDetail();

    await expect.element(screen.getByRole("tab", { name: "工作台" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "任务" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "审批" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "资产" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "概览" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("tab", { name: "配置" })).not.toBeInTheDocument();
    await expect.element(screen.getByText("工作区就绪")).toBeVisible();
    await expect.element(screen.getByText("目录名 客户接入验收")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: /提交需求/ })).toHaveAttribute(
      "href",
      "/task-launches?mode=plan&project=project-1",
    );
    await expect.element(screen.getByTestId("project-ops-home")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: /验收执行员工/ })).toHaveAttribute(
      "href",
      "/employees/employee-1",
    );
    await expect.element(screen.getByRole("link", { name: /负责人甲/ })).toHaveAttribute(
      "href",
      "/users",
    );
  });

  it("maps legacy tab deep links onto sections and assets sub-tabs", async () => {
    const artifactsScreen = await renderDetail({ initialTab: "artifacts" });
    await expect
      .element(artifactsScreen.getByRole("tab", { name: "资产", selected: true }))
      .toBeVisible();
    await expect
      .element(artifactsScreen.getByRole("tab", { name: "工件", selected: true }))
      .toBeVisible();

    const budgetScreen = await renderDetail({ initialTab: "budget" });
    await expect
      .element(budgetScreen.getByRole("tab", { name: "预算", selected: true }))
      .toBeVisible();

    const overviewScreen = await renderDetail({ initialTab: "overview" });
    await expect
      .element(overviewScreen.getByRole("tab", { name: "工作台", selected: true }))
      .toBeVisible();
    await expect.element(overviewScreen.getByTestId("project-ops-home")).toBeVisible();
  });

  it("compresses empty project into a startup card instead of giant empty vaults", async () => {
    const emptyOverview: ProjectOverview = {
      ...overview,
      active_tasks: [],
      digital_employee_pool: [],
      recent_events: [],
    };
    const screen = await renderDetail({
      decisionRequests: [],
      demands: [],
      events: [],
      overview: emptyOverview,
      planRevisions: [],
      tasks: [],
    });

    await expect.element(screen.getByTestId("project-ops-startup")).toBeVisible();
    await expect
      .element(
        screen.getByText(
          "项目已就绪，还没有任务活动。用头卡「提交需求」发起后，这里会出现运行脉搏与执行态。",
        ),
      )
      .toBeVisible();
    await expect.element(screen.getByText("本周 0 次活动")).toBeVisible();
    await expect.element(screen.getByTestId("project-ops-rail")).toBeVisible();
    await expect.element(screen.getByText("当前无阻塞")).toBeVisible();
    await expect.element(screen.getByText("暂无执行/审批事件")).toBeVisible();
    await expect.element(screen.getByTestId("project-ops-pulse")).not.toBeInTheDocument();
    await expect.element(screen.getByTestId("project-ops-running")).not.toBeInTheDocument();
    // 主 CTA 只在头卡，启动区不重复「提交需求」按钮
    const submitLinks = screen.getByRole("link", { name: /提交需求/ });
    await expect.element(submitLinks).toBeVisible();
  });

  it("resolves owner and service-pool names when membership snapshots are empty", async () => {
    const unnamedOverview: ProjectOverview = {
      ...overview,
      digital_employee_pool: [
        member({
          display_name_snapshot: undefined,
          id: "member-employee-unnamed",
          principal_id: "employee-unnamed-1",
          principal_type: "digital_employee",
        }),
      ],
      human_roles: [
        member({
          display_name_snapshot: undefined,
          id: "member-owner-unnamed",
          principal_id: "human-owner-unnamed-1",
          principal_type: "human_user",
          project_role: "owner",
        }),
      ],
    };
    const screen = await renderDetail({
      overview: unnamedOverview,
      principalNamesById: new Map([
        ["employee-unnamed-1", "运维检索员工"],
        ["human-owner-unnamed-1", "李娜"],
      ]),
      project: {
        ...project,
        human_owner_user_id: "human-owner-unnamed-1",
      },
    });

    await expect.element(screen.getByRole("link", { name: /李娜/ })).toHaveAttribute(
      "href",
      "/users",
    );
    await expect.element(screen.getByRole("link", { name: /运维检索员工/ })).toHaveAttribute(
      "href",
      "/employees/employee-unnamed-1",
    );
    expect(screen.container.textContent).not.toContain("human-owner-unnamed-1");
    expect(screen.container.textContent).not.toContain("employee-unnamed-1");
  });

  it("shows a diagnosis line and gap deep link for a failed demand", async () => {
    const failedDemands: ProjectDemand[] = [
      { ...demands[0], status: "failed" },
    ];
    const screen = await renderDetail({
      demands: failedDemands,
      taskGraph: {
        blocking_facts: [
          {
            created_at: "2026-07-10T08:00:00Z",
            message: "项目员工池无法满足审查独立性约束（需≥2名可调度员工）",
            reason_code: "no_suitable_employee",
            recommended_action: "为项目补充可调度员工或换用模板",
          },
        ],
        decision_requests: [],
        edges: [],
        employees: [],
        execution_summaries: [],
        nodes: [],
        recent_events: [],
        runs: [],
      },
    });

    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));
    await expect
      .element(screen.getByText("项目员工池无法满足审查独立性约束（需≥2名可调度员工）"))
      .toBeVisible();
    await expect
      .element(screen.getByText("下一步：为项目补充可调度员工或换用模板"))
      .toBeVisible();
    await expect.element(screen.getByRole("link", { name: "查看缺口处理 →" })).toHaveAttribute(
      "href",
      "/workflows/demand-1",
    );
  });

  it("does not show a diagnosis line for a non-failed demand", async () => {
    const screen = await renderDetail();
    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

    await expect
      .element(screen.getByRole("link", { name: "查看缺口处理 →" }))
      .not.toBeInTheDocument();
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
    const screen = await renderDetail({ initialTab: "approval", planRevisions });
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

  it("defaults to automated-verification badge and no ambiguity warning for legacy criteria payload", async () => {
    const screen = await renderDetail({ initialTab: "approval", planRevisions });

    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-method-evidence_complete']",
      )?.textContent,
    ).toContain("自动验证");
    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-severity-evidence_complete']",
      ),
    ).toBeNull();
    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-ambiguity-evidence_complete']",
      ),
    ).toBeNull();
    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-evidence-hint-evidence_complete']",
      ),
    ).toBeNull();
  });

  it("shows verification method, severity, ambiguity and evidence hint badges from criterion payload", async () => {
    const semanticCriteriaRevision: ProjectPlanRevision = {
      ...planRevisions[0],
      payload: {
        ...planRevisions[0].payload,
        plan_acceptance_criteria: [
          {
            id: "automated_check",
            satisfied_by: ["collect-evidence"],
            statement: "自动化测试用例全部通过。",
            verification_method: "automated_test",
          },
          {
            id: "human_review",
            satisfied_by: [],
            statement: "负责人确认交付满足业务预期。",
            verification_method: "human_judgment",
          },
          {
            id: "non_blocking_check",
            satisfied_by: ["review-evidence"],
            severity: "non_blocking",
            statement: "补充材料齐全（非阻断）。",
          },
          {
            ambiguity_flag: true,
            evidence_hint: "上传验收报告截图作为证据。",
            id: "ambiguous_check",
            satisfied_by: ["review-evidence"],
            statement: "系统表现良好。",
          },
        ],
      },
    };
    const screen = await renderDetail({ initialTab: "approval", planRevisions: [semanticCriteriaRevision] });

    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-method-automated_check']",
      )?.textContent,
    ).toContain("自动验证");
    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-method-human_review']",
      )?.textContent,
    ).toContain("人类判定");

    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-severity-non_blocking_check']",
      )?.textContent,
    ).toContain("非阻断");
    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-severity-automated_check']",
      ),
    ).toBeNull();

    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-ambiguity-ambiguous_check']",
      )?.textContent,
    ).toContain("断言可能不可判定，请改写后再批准");
    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-ambiguity-automated_check']",
      ),
    ).toBeNull();

    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-evidence-hint-ambiguous_check']",
      )?.textContent,
    ).toContain("上传验收报告截图作为证据。");
    expect(
      screen.container.querySelector(
        "[data-testid='plan-acceptance-criterion-evidence-hint-automated_check']",
      ),
    ).toBeNull();

    await expect.element(screen.getByText("需求级人类判定")).toBeVisible();
  });

  it("shows template version, exit deliverable label and constraint notes from plan payload", async () => {
    const templatedRevision: ProjectPlanRevision = {
      ...planRevisions[0],
      payload: {
        ...planRevisions[0].payload,
        available_exits: [
          { deliverable: "review_verdict", label: "审查通过并合入" },
          { deliverable: "release_notes", label: "发布说明就绪" },
        ],
        constraint_notes: [
          {
            kind: "human_gate",
            message:
              "发布任务已强制人类审批：由 human_gate@software_delivery v2 触发",
          },
        ],
        exit_deliverable: "review_verdict",
        template_key: "software_delivery",
        template_version: 2,
      },
    };
    const screen = await renderDetail({ initialTab: "approval", planRevisions: [templatedRevision] });

    await expect.element(screen.getByText("software_delivery@v2")).toBeVisible();
    await expect.element(screen.getByText("审查通过并合入")).toBeVisible();
    await expect
      .element(
        screen.getByText(
          "发布任务已强制人类审批：由 human_gate@software_delivery v2 触发",
        ),
      )
      .toBeVisible();
  });

  it("hides template and exit rows for an unbound demand carrying only a hallucinated template_key", async () => {
    const unboundRevision: ProjectPlanRevision = {
      ...planRevisions[0],
      payload: {
        ...planRevisions[0].payload,
        // Planner echoed a template lineage for a template-less demand; without a
        // template_version binding marker, neither the 场景模板 nor 交付出口 row
        // may render.
        exit_deliverable: "risk_report",
        template_key: "tech_risk_analysis",
      },
    };
    const screen = await renderDetail({ initialTab: "approval", planRevisions: [unboundRevision] });

    await expect.element(screen.getByText("调度顺序")).toBeVisible();
    await expect.element(screen.getByText("tech_risk_analysis")).not.toBeInTheDocument();
    await expect.element(screen.getByText("risk_report")).not.toBeInTheDocument();
    await expect.element(screen.getByText("交付出口")).not.toBeInTheDocument();
  });

  it("submits the selected target exit deliverable alongside request_changes", async () => {
    const onResolveDecision = vi.fn();
    const templatedRevision: ProjectPlanRevision = {
      ...planRevisions[0],
      coordination_job_id: "coordination-job-1",
      payload: {
        ...planRevisions[0].payload,
        available_exits: [
          { deliverable: "review_verdict", label: "审查通过并合入" },
          { deliverable: "release_notes", label: "发布说明就绪" },
        ],
        exit_deliverable: "review_verdict",
        template_key: "software_delivery",
        template_version: 2,
      },
    };
    const planReviewDecision: ProjectDecisionRequest = {
      approval_request_id: "approval-2",
      coordination_job_id: "coordination-job-1",
      decision_type: "plan_review",
      id: "decision-plan-review-1",
      project_id: "project-1",
      status_snapshot: "pending",
      summary_snapshot: "确认计划版本 v1",
      target_user_id: "human-owner-1",
      tenant_id: "tenant-1",
      title_snapshot: "确认计划版本 v1",
    };
    const screen = await renderDetail({
      decisionRequests: [planReviewDecision],
      initialTab: "approval",
      onResolveDecision,
      planRevisions: [templatedRevision],
    });

    await userEvent.click(screen.getByRole("combobox", { name: "改选交付出口" }));
    await userEvent.click(screen.getByRole("option", { name: "发布说明就绪" }));
    await userEvent.click(
      screen.getByRole("button", { name: "要求修改计划版本 v1" }),
    );

    expect(onResolveDecision).toHaveBeenCalledWith(
      "decision-plan-review-1",
      "request_changes",
      "release_notes",
    );
  });

  it("resets the selected target exit deliverable when a newer plan revision replaces it", async () => {
    const onResolveDecision = vi.fn();
    const revisionA: ProjectPlanRevision = {
      ...planRevisions[0],
      coordination_job_id: "coordination-job-1",
      id: "plan-revision-a",
      payload: {
        ...planRevisions[0].payload,
        available_exits: [
          { deliverable: "review_verdict", label: "审查通过并合入" },
          { deliverable: "release_notes", label: "发布说明就绪" },
        ],
        exit_deliverable: "review_verdict",
        template_key: "software_delivery",
        template_version: 2,
      },
      revision_number: 1,
    };
    const revisionB: ProjectPlanRevision = {
      ...revisionA,
      id: "plan-revision-b",
      payload: {
        ...revisionA.payload,
        available_exits: [
          { deliverable: "review_verdict", label: "审查通过并合入" },
          { deliverable: "hotfix_notes", label: "补丁说明就绪" },
        ],
        exit_deliverable: "review_verdict",
      },
      revision_number: 2,
    };
    const planReviewDecision: ProjectDecisionRequest = {
      approval_request_id: "approval-2",
      coordination_job_id: "coordination-job-1",
      decision_type: "plan_review",
      id: "decision-plan-review-1",
      project_id: "project-1",
      status_snapshot: "pending",
      summary_snapshot: "确认计划版本",
      target_user_id: "human-owner-1",
      tenant_id: "tenant-1",
      title_snapshot: "确认计划版本",
    };
    const screen = await render(
      detailElement({
        decisionRequests: [planReviewDecision],
        initialTab: "approval",
        onResolveDecision,
        planRevisions: [revisionA],
      }),
    );

    await userEvent.click(screen.getByRole("combobox", { name: "改选交付出口" }));
    await userEvent.click(screen.getByRole("option", { name: "发布说明就绪" }));
    await expect
      .element(screen.getByRole("combobox", { name: "改选交付出口" }))
      .toHaveTextContent("发布说明就绪");

    // 计划修订到 v2:v1 挑选的出口在新修订里已不是同一个候选,必须重置,
    // 否则会带着 stale 选择提交给新修订。
    await screen.rerender(
      detailElement({
        decisionRequests: [planReviewDecision],
        initialTab: "approval",
        onResolveDecision,
        planRevisions: [revisionB],
      }),
    );

    await expect
      .element(screen.getByRole("combobox", { name: "改选交付出口" }))
      .toHaveTextContent("审查通过并合入");
    await userEvent.click(
      screen.getByRole("button", { name: "要求修改计划版本 v2" }),
    );
    expect(onResolveDecision).toHaveBeenCalledWith(
      "decision-plan-review-1",
      "request_changes",
      "review_verdict",
    );
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
    const screen = await renderDetail({ initialTab: "approval", planRevisions: [blockedByKeysRevision] });
    const dispatchOrderText =
      screen.container.querySelector("[data-testid='plan-dispatch-order']")?.textContent ?? "";
    expect(dispatchOrderText.indexOf("收集接入证据")).toBeLessThan(
      dispatchOrderText.indexOf("复核接入证据"),
    );
  });

  it("shows archive and delete actions in the detail header menu when allowed", async () => {
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

    await userEvent.click(screen.getByRole("button", { name: "更多项目操作" }));
    await expect
      .element(screen.getByRole("menuitem", { name: "删除项目" }))
      .toHaveAttribute("data-variant", "destructive");
    await userEvent.click(screen.getByRole("menuitem", { name: "归档项目" }));
    expect(onArchiveProject).toHaveBeenCalledTimes(1);

    await userEvent.click(screen.getByRole("button", { name: "更多项目操作" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "删除项目" }));
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

    await userEvent.click(screen.getByRole("button", { name: "更多项目操作" }));
    await expect
      .element(screen.getByRole("menuitem", { name: "配置项目" }))
      .toBeInTheDocument();
    await expect.element(screen.getByRole("menuitem", { name: "归档项目" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("menuitem", { name: "删除项目" })).not.toBeInTheDocument();
  });

  it("shows relative timestamps on the event stream", async () => {
    const screen = await renderDetail({
      overview: {
        ...overview,
        recent_events: [
          {
            actor_id: "system",
            actor_type: "system",
            created_at: "2026-07-20T01:00:00Z",
            event_type: "project_task.completed",
            id: "event-completed",
            payload: {},
            project_id: "project-1",
            sequence_number: 1,
            summary: "接入证据整理完成",
            tenant_id: "tenant-1",
          },
        ],
      },
    });

    await expect.element(screen.getByText("接入证据整理完成")).toBeVisible();
    const time = screen.container.querySelector('time[datetime="2026-07-20T01:00:00Z"]');
    expect(time).not.toBeNull();
    expect(time?.textContent ?? "").toMatch(/\d+\s*天前/);
  });
});
