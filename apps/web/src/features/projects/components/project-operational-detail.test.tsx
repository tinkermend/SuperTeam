import type { AnchorHTMLAttributes, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
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
  ProjectTaskGraph
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
    }
};
});

const project: Project = {
  coordination_policy: {},
  coordination_status: "registered",
  coordination_workflow_id: "project-coordinator:project-1",
  directory_name: "customer-acceptance",
  goal: "完成客户接入验收闭环",
  human_owner_user_id: "human-owner-1",
  id: "project-1",
  name: "客户接入验收",
  status: "running",
  tenant_id: "tenant-1",
  workspace_ready_status: "ready"
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
    ...overrides
};
}

// 概览不再返回任务列表（active_tasks 已退役）；任务明细由 tasks prop 单独注入。
const overviewTasks: ProjectTask[] = [
  {
    assigned_digital_employee_id: "employee-1",
    id: "task-1",
    project_id: "project-1",
    requires_human_approval: false,
    status: "running",
    summary: "整理客户接入证据",
    tenant_id: "tenant-1",
    title: "整理接入证据"
},
];

const overview: ProjectOverview = {
  coordination_workflow: {
    status: "registered",
    workflow_id: "project-coordinator:project-1"
},
  digital_employee_pool: [
    member({
      display_name_snapshot: "验收执行员工",
      id: "member-employee-1",
      principal_id: "employee-1",
      principal_type: "digital_employee"
}),
  ],
  human_roles: [
    member({
      display_name_snapshot: "负责人甲",
      id: "member-owner-1",
      principal_id: "human-owner-1",
      principal_type: "human_user",
      project_role: "owner"
}),
  ],
  project,
  recent_events: [],
  status_summary: { current_phase: "running", is_archived: false },
  task_summary: {
    active_tasks: 1,
    cancelled_tasks: 0,
    completed_tasks: 0,
    failed_tasks: 0,
    pending_human_tasks: 0,
    running_tasks: 1,
    total_tasks: 1
}
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
    title: "补充上线验收说明"
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
    title_snapshot: "确认上线风险"
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
          statement: longAcceptanceStatement
},
        {
          id: "review_complete",
          satisfied_by: ["review-evidence"],
          statement: "验收结论已由负责人复核。"
},
      ],
      summary: "生成客户接入验收计划。",
      tasks: [
        {
          employee_selection_reason: "负责收集客户接入材料。",
          planned_task_key: "collect-evidence",
          selected_employee_id: "employee-collector",
          title: "收集接入证据"
},
        {
          depends_on: ["collect-evidence"],
          employee_selection_reason: "负责复核证据并形成结论。",
          planned_task_key: "review-evidence",
          selected_employee_id: "employee-reviewer",
          title: "复核接入证据"
},
      ]
},
    plan_fingerprint: "fingerprint",
    project_id: "project-1",
    review_required: true,
    revision_number: 1,
    status: "pending_review",
    tenant_id: "tenant-1",
    validation_errors: [],
    validation_warnings: []
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
      tasks={overviewTasks}
      transferRequests={[]}
      {...props}
    />
  );
}

function renderDetail(
  props: Partial<React.ComponentProps<typeof ProjectOperationalDetail>> = {},
) {
  // 任务详情弹层内含懒查执行图的 useQuery，渲染需要 QueryClientProvider。
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } }
});
  return render(
    <QueryClientProvider client={queryClient}>
      {detailElement(props)}
    </QueryClientProvider>,
  );
}

describe("ProjectOperationalDetail", () => {
  it("renders section nav with workbench default and navigable project context", async () => {
    const screen = await renderDetail();

    await expect.element(screen.getByRole("tab", { name: "工作台" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "需求流程" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "任务" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "决策历史" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "资产" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "概览" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("tab", { name: "配置" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "客户接入验收" })).toBeVisible();
    await expect.element(screen.getByText("工作区就绪")).not.toBeInTheDocument();
    await expect.element(screen.getByText("目录名")).toBeVisible();
    await expect.element(screen.getByText("阶段 已就绪")).not.toBeInTheDocument();
    await expect.element(screen.getByText("完成客户接入验收闭环")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("link", { name: /提交需求/ })).toHaveAttribute(
      "href",
      "/task-launches?mode=plan&project=project-1",
    );
    await expect.element(screen.getByTestId("project-ops-home")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: /验收执行员工/ })).toHaveAttribute(
      "href",
      "/employees/employee-1",
    );
    await expect.element(screen.getByTestId("project-owner-avatars")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "负责人 负责人甲" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "负责人 负责人甲" }));
    await expect.element(screen.getByText("负责人甲")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "在用户管理中查看" })).toHaveAttribute(
      "href",
      "/users?user=human-owner-1",
    );
  });

  it("renders the stage pipeline with deep links and per-stage status", async () => {
    const screen = await renderDetail({ planRevisions });

    // 头部事实条的「执行中」与执行格同判据（运行中/等待人工），不是任务总数。
    await expect.element(screen.getByRole("button", { name: "执行中 1" })).toBeVisible();

    const pipeline = screen.getByTestId("project-stage-pipeline");
    await expect.element(pipeline).toBeVisible();

    // 需求格：状态 + 计数 + 深链 ?tab=demands
    const demandCell = screen.getByTestId("pipeline-stage-demands");
    await expect.element(demandCell).toHaveAttribute("href", ".?tab=demands");
    await expect.element(demandCell.getByText("待计划")).toBeVisible();
    await expect.element(demandCell.getByText("共 1 条")).toBeVisible();

    // 执行格：任务计数 + 深链任务区
    const executionCell = screen.getByTestId("pipeline-stage-execution");
    await expect.element(executionCell).toHaveAttribute("href", ".?tab=tasks");
    await expect
      .element(executionCell.getByText("执行中 1 · 共 1 项"))
      .toBeVisible();

    // 结果格：验收/工件深链（?tab=acceptance 映射资产区验收页签）
    const resultsCell = screen.getByTestId("pipeline-stage-results");
    await expect.element(resultsCell).toHaveAttribute("href", ".?tab=acceptance");
    await expect.element(resultsCell.getByText("未验收")).toBeVisible();
    await expect.element(resultsCell.getByText("工件 0 项")).toBeVisible();

    // 计划格：就地展开/收起计划确认卡
    const planCell = screen.getByTestId("pipeline-stage-plan");
    await expect.element(planCell.getByText("计划 v1")).toBeVisible();
    await expect.element(planCell).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(planCell);
    await expect.element(planCell).toHaveAttribute("aria-expanded", "true");
    await expect.element(screen.getByText("调度顺序")).toBeVisible();
    await expect.element(screen.getByText("验收判据")).toBeVisible();
    await userEvent.click(planCell);
    await expect.element(screen.getByText("调度顺序")).not.toBeInTheDocument();
  });

  it("surfaces failed demand and failed task counts as pipeline pending actions", async () => {
    const failedTask: ProjectTask = {
      id: "task-failed",
      project_id: "project-1",
      requires_human_approval: false,
      status: "failed",
      tenant_id: "tenant-1",
      title: "失败任务"
};
    const screen = await renderDetail({
      demands: [{ ...demands[0], status: "failed" }],
      // 计数以服务端聚合为权威，夹具的任务列表与 task_summary 必须自洽。
      overview: {
        ...overview,
        task_summary: {
          active_tasks: 0,
          cancelled_tasks: 0,
          completed_tasks: 0,
          failed_tasks: 1,
          pending_human_tasks: 0,
          running_tasks: 0,
          total_tasks: 1
}
},
      tasks: [failedTask]
});

    const demandCell = screen.getByTestId("pipeline-stage-demands");
    await expect.element(demandCell.getByText("1 条失败待处理 →")).toBeVisible();
    const executionCell = screen.getByTestId("pipeline-stage-execution");
    await expect.element(executionCell.getByText("1 项失败待处理 →")).toBeVisible();
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

  it("opens the demands section from ?tab=demands with the demand switcher", async () => {
    // 需求流程区直连血缘/待决 API：给最小 stub 响应即可。
    const fetcher: typeof fetch = async (input: RequestInfo | URL) => {
      const url = String(input);
      const payload = url.includes("/launch-detail")
        ? {
            coordination_jobs: [],
            decision_requests: [],
            demand: { id: "demand-1" },
            execution_summaries: [],
            project: { id: "project-1" },
            project_tasks: [],
            recent_events: [],
            reviewer: null,
            route_decisions: []
}
        : { criteria: [], demand_status: "submitted" };
      return new Response(JSON.stringify(payload), {
        headers: { "Content-Type": "application/json" },
        status: 200
});
    };
    const screen = await renderDetail({
      apiBaseUrl: "http://cp.test",
      apiOptions: { baseUrl: "http://cp.test", fetcher },
      initialTab: "demands"
});

    await expect
      .element(screen.getByRole("tab", { name: "需求流程", selected: true }))
      .toBeVisible();
    await expect.element(screen.getByTestId("project-demands-section")).toBeVisible();
    const header = screen.getByTestId("demand-status-header");
    await expect.element(header.getByText("补充上线验收说明")).toBeVisible();
    await expect.element(header.getByText("待计划")).toBeVisible();
    await expect
      .element(screen.getByTestId("demand-list-item-demand-1"))
      .toHaveAttribute("href", "/projects/project-1?demand=demand-1&tab=demands");
  });

  it("reports zero running tasks in the hero facts once every task is completed", async () => {
    // 计数只认服务端聚合：即便任务列表里还摆着任务，全部完成时头部也必须显示 0。
    const completedTasks: ProjectTask[] = [
      { ...overviewTasks[0], status: "completed" },
    ];
    const screen = await renderDetail({
      overview: {
        ...overview,
        task_summary: {
          active_tasks: 0,
          cancelled_tasks: 0,
          completed_tasks: 1,
          failed_tasks: 0,
          pending_human_tasks: 0,
          running_tasks: 0,
          total_tasks: 1,
        },
      },
      tasks: completedTasks,
    });

    await expect.element(screen.getByRole("button", { name: "执行中 0" })).toBeVisible();
  });

  it("counts from the server task summary, not the paginated task page", async () => {
    // 任务列表是 limit 20 的页，计数必须来自服务端全表聚合，否则任务超过 20 条即漏计。
    const screen = await renderDetail({
      overview: {
        ...overview,
        task_summary: {
          active_tasks: 9,
          cancelled_tasks: 3,
          completed_tasks: 30,
          failed_tasks: 2,
          pending_human_tasks: 4,
          running_tasks: 5,
          total_tasks: 44,
        },
      },
    });

    // 执行中 = running + waiting_human = 9，来自聚合而非已加载的 1 条任务。
    await expect.element(screen.getByRole("button", { name: "执行中 9" })).toBeVisible();
    const executionCell = screen.getByTestId("pipeline-stage-execution");
    await expect.element(executionCell.getByText("执行中 9 · 共 44 项")).toBeVisible();
  });

  it("shows week pulse calendar with centered empty copy when project has no activity", async () => {
    const emptyOverview: ProjectOverview = {
      ...overview,
      digital_employee_pool: [],
      recent_events: []
};
    const screen = await renderDetail({
      decisionRequests: [],
      demands: [],
      events: [],
      overview: emptyOverview,
      planRevisions: [],
      tasks: []
});

    await expect.element(screen.getByTestId("project-ops-pulse")).toBeVisible();
    await expect.element(screen.getByTestId("project-ops-pulse-empty")).toBeVisible();
    await expect.element(screen.getByText("本周暂无任务活动")).toBeVisible();
    await expect.element(screen.getByTestId("project-ops-rail")).toBeVisible();
    await expect.element(screen.getByText("当前无阻塞")).toBeVisible();
    await expect.element(screen.getByTestId("project-ops-running")).not.toBeInTheDocument();
    await expect.element(screen.getByTestId("project-ops-startup")).not.toBeInTheDocument();
  });

  it("opens project task detail dialog from pulse chip and resolves decision inline", async () => {
    const onResolveDecision = vi.fn();
    const nowIso = new Date().toISOString();
    const weekTask: ProjectTask = {
      assigned_digital_employee_id: "employee-1",
      created_at: nowIso,
      demand_id: "demand-1",
      id: "task-1",
      project_id: "project-1",
      requires_human_approval: false,
      status: "running",
      summary: "整理客户接入证据",
      tenant_id: "tenant-1",
      title: "整理接入证据",
      updated_at: nowIso
};
    const taskDecision: ProjectDecisionRequest = {
      ...decisionRequests[0],
      project_task_id: "task-1"
};
    const screen = await renderDetail({
      decisionRequests: [taskDecision],
      onResolveDecision,
      tasks: [weekTask]
});

    // 脉搏芯片(带 title 的按钮)点击应打开任务详情弹层
    await userEvent.click(screen.getByTitle("整理接入证据"));
    const dialog = screen.getByTestId("project-task-detail-dialog");
    await expect.element(dialog).toBeVisible();

    // 任务自身事实 + 编排降级 + 运行空态
    await expect.element(dialog.getByText("整理客户接入证据")).toBeVisible();
    await expect.element(dialog.getByText("验收执行员工")).toBeVisible();
    await expect.element(dialog.getByText(/当前执行图未包含该任务/)).toBeVisible();
    await expect.element(dialog.getByText("暂无运行记录")).toBeVisible();

    // 待决事项就地处理,走与工作台同一条 onResolveDecision 出口
    await expect
      .element(dialog.getByText("确认上线风险", { exact: true }))
      .toBeVisible();
    await userEvent.click(dialog.getByRole("button", { name: "批准" }));
    expect(onResolveDecision).toHaveBeenCalledWith("decision-1", "approved");

    // 编排深链（流程编排页已退役，指向本项目需求流程区）
    await expect
      .element(dialog.getByRole("link", { name: "查看该任务所在需求流程" }))
      .toHaveAttribute("href", "/projects/project-1?demand=demand-1&tab=demands");
  });

  it("lazily fetches the task's demand graph when it is missing from the preloaded graph", async () => {
    const nowIso = new Date().toISOString();
    const historicalTask: ProjectTask = {
      assigned_digital_employee_id: "employee-1",
      created_at: nowIso,
      demand_id: "demand-old",
      id: "task-old",
      project_id: "project-1",
      requires_human_approval: false,
      status: "completed",
      summary: "历史需求下的任务",
      tenant_id: "tenant-1",
      title: "历史任务",
      updated_at: nowIso
};
    const lazyGraph: ProjectTaskGraph = {
      blocking_facts: [],
      decision_requests: [],
      edges: [],
      employees: [
        {
          digital_employee_id: "employee-1",
          display_name: "验收执行员工",
          project_role: "executor",
          status: "active"
},
      ],
      execution_summaries: [
        {
          artifact_refs: [],
          conclusion: "历史任务已完成并产出简报",
          confidence_factors: {},
          created_at: nowIso,
          digital_employee_id: "employee-1",
          evidence_refs: [],
          id: "summary-1",
          missing_information: [],
          project_id: "project-1",
          project_task_id: "task-old",
          requires_human_review: false,
          tenant_id: "tenant-1"
},
      ],
      nodes: [
        {
          ...historicalTask,
          expected_outputs: ["中文简报"],
          handoff_contract: {},
          input_requirements: {},
          planner_metadata: {}
},
      ],
      recent_events: [],
      runs: [
        {
          finished_at: nowIso,
          project_task_id: "task-old",
          provider_type: "claude_code",
          runtime_node_summary: "macbook-dev",
          started_at: nowIso,
          status: "completed"
},
      ]
};
    const fetchTaskGraph = vi.fn().mockResolvedValue(lazyGraph);
    const screen = await renderDetail({
      fetchTaskGraph,
      tasks: [historicalTask]
});

    await userEvent.click(screen.getByTitle("历史任务"));
    const dialog = screen.getByTestId("project-task-detail-dialog");
    await expect.element(dialog).toBeVisible();

    // 懒查询按任务的 demand_id 补图,编排切片与执行结论渲染出来
    await expect.element(dialog.getByText("中文简报")).toBeVisible();
    await expect.element(dialog.getByText("历史任务已完成并产出简报")).toBeVisible();
    await expect
      .element(dialog.getByText(/当前执行图未包含该任务/))
      .not.toBeInTheDocument();
    expect(fetchTaskGraph).toHaveBeenCalledWith("demand-old");

    // 「查看执行轨迹」深链带 &task=：轨迹面板按该任务过滤定位。
    await expect
      .element(dialog.getByRole("link", { name: "查看历史任务执行轨迹" }))
      .toHaveAttribute("href", "/projects/project-1?tab=trace&task=task-old");
  });

  it("opens the same task detail dialog from the tasks tab title", async () => {
    const screen = await renderDetail();

    await userEvent.click(screen.getByRole("tab", { name: "任务" }));
    await userEvent.click(screen.getByRole("button", { name: "整理接入证据" }));
    await expect
      .element(screen.getByTestId("project-task-detail-dialog"))
      .toBeVisible();
  });

  it("resolves owner and service-pool names when membership snapshots are empty", async () => {
    const unnamedOverview: ProjectOverview = {
      ...overview,
      digital_employee_pool: [
        member({
          display_name_snapshot: undefined,
          id: "member-employee-unnamed",
          principal_id: "employee-unnamed-1",
          principal_type: "digital_employee"
}),
      ],
      human_roles: [
        member({
          display_name_snapshot: undefined,
          id: "member-owner-unnamed",
          principal_id: "human-owner-unnamed-1",
          principal_type: "human_user",
          project_role: "owner"
}),
      ]
};
    const screen = await renderDetail({
      overview: unnamedOverview,
      principalNamesById: new Map([
        ["employee-unnamed-1", "运维检索员工"],
        ["human-owner-unnamed-1", "李娜"],
      ]),
      project: {
        ...project,
        human_owner_user_id: "human-owner-unnamed-1"
}
});

    await expect.element(screen.getByRole("button", { name: "负责人 李娜" })).toBeVisible();
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
            recommended_action: "为项目补充可调度员工或换用模板"
},
        ],
        decision_requests: [],
        edges: [],
        employees: [],
        execution_summaries: [],
        nodes: [],
        recent_events: [],
        runs: []
}
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
      ".?demand=demand-1&tab=demands",
    );
  });

  it("replaces the advanced-facts execution graph with a demands deep link", async () => {
    // 即使预载图带节点也不再渲染画布：执行图唯一渲染点在需求流程区。
    const graphWithNodes: ProjectTaskGraph = {
      blocking_facts: [],
      decision_requests: [],
      edges: [],
      employees: [],
      execution_summaries: [],
      nodes: [
        {
          ...overviewTasks[0],
          expected_outputs: [],
          handoff_contract: {},
          input_requirements: {},
          planner_metadata: {}
},
      ],
      recent_events: [],
      runs: []
};
    const screen = await renderDetail({ taskGraph: graphWithNodes });

    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));
    expect(
      screen.container.querySelector("[data-testid='project-plan-graph-section']"),
    ).toBeNull();
    await expect.element(screen.getByText("当前执行图")).not.toBeInTheDocument();
    const deeplink = screen.getByTestId("execution-graph-deeplink");
    await expect
      .element(deeplink.getByText("执行图已迁入需求流程区"))
      .toBeVisible();
    await expect
      .element(deeplink.getByRole("link", { name: "前往需求流程 →" }))
      .toHaveAttribute("href", ".?tab=demands");
  });

  it("shows the owning demand column with deep link and fallbacks in the tasks tab", async () => {
    const demandTask: ProjectTask = {
      ...overviewTasks[0],
      demand_id: "demand-1"
};
    const unknownDemandTask: ProjectTask = {
      id: "task-unknown-demand",
      demand_id: "demand-gone-1234567890",
      project_id: "project-1",
      requires_human_approval: false,
      status: "running",
      tenant_id: "tenant-1",
      title: "历史需求任务"
};
    const noDemandTask: ProjectTask = {
      id: "task-no-demand",
      project_id: "project-1",
      requires_human_approval: false,
      status: "running",
      tenant_id: "tenant-1",
      title: "无需求任务"
};
    const screen = await renderDetail({
      tasks: [demandTask, unknownDemandTask, noDemandTask]
});

    await userEvent.click(screen.getByRole("tab", { name: "任务" }));
    await expect.element(screen.getByText("所属需求")).toBeVisible();

    // 有需求且能解析标题：显示需求名称并深链需求流程区
    const demandCell = screen.getByTestId("task-demand-cell-task-1");
    await expect
      .element(demandCell.getByRole("link", { name: "补充上线验收说明" }))
      .toHaveAttribute("href", ".?demand=demand-1&tab=demands");

    // demand_id 解析不到标题：回退 8 位短 id（mono），不裸整段 UUID
    const unknownCell = screen.getByTestId("task-demand-cell-task-unknown-demand");
    const unknownLink = unknownCell.getByRole("link", { name: "demand-g" });
    await expect
      .element(unknownLink)
      .toHaveAttribute("href", ".?demand=demand-gone-1234567890&tab=demands");
    expect(unknownCell.query()?.textContent).not.toContain("demand-gone-1234567890");
    expect(unknownLink.query()?.className).toContain("font-mono");

    // 无 demand_id：显示占位 —
    expect(
      screen.getByTestId("task-demand-cell-task-no-demand").query()?.textContent,
    ).toBe("—");
  });

  it("does not show a diagnosis line for a non-failed demand", async () => {
    const screen = await renderDetail();
    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

    await expect
      .element(screen.getByRole("link", { name: "查看缺口处理 →" }))
      .not.toBeInTheDocument();
  });

  it("opens decision-history tab from query focus and highlights the decision without write actions", async () => {
    const onResolveDecision = vi.fn();
    const screen = await renderDetail({
      focusDecisionId: "decision-1",
      initialTab: "approval",
      onResolveDecision
});

    await expect
      .element(screen.getByRole("tab", { name: "决策历史", selected: true }))
      .toBeVisible();
    const focusedDecision = screen.container.querySelector("[data-focused-decision='true']");
    expect(focusedDecision?.textContent).toContain("确认上线风险");
    await expect
      .element(screen.getByRole("button", { name: "批准 确认上线风险" }))
      .not.toBeInTheDocument();
    expect(onResolveDecision).not.toHaveBeenCalled();
  });

  it("shows dispatch order and acceptance criteria from the latest plan revision", async () => {
    const screen = await renderDetail({ planRevisions });
    await userEvent.click(screen.getByTestId("pipeline-stage-plan"));
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
    const screen = await renderDetail({ planRevisions });
    await userEvent.click(screen.getByTestId("pipeline-stage-plan"));

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
            verification_method: "automated_test"
},
          {
            id: "human_review",
            satisfied_by: [],
            statement: "负责人确认交付满足业务预期。",
            verification_method: "human_judgment"
},
          {
            id: "non_blocking_check",
            satisfied_by: ["review-evidence"],
            severity: "non_blocking",
            statement: "补充材料齐全（非阻断）。"
},
          {
            ambiguity_flag: true,
            evidence_hint: "上传验收报告截图作为证据。",
            id: "ambiguous_check",
            satisfied_by: ["review-evidence"],
            statement: "系统表现良好。"
},
        ]
}
};
    const screen = await renderDetail({ planRevisions: [semanticCriteriaRevision] });
    await userEvent.click(screen.getByTestId("pipeline-stage-plan"));

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
              "发布任务已强制人类审批：由 human_gate@software_delivery v2 触发"
},
        ],
        exit_deliverable: "review_verdict",
        template_key: "software_delivery",
        template_version: 2
}
};
    const screen = await renderDetail({ planRevisions: [templatedRevision] });
    await userEvent.click(screen.getByTestId("pipeline-stage-plan"));

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
        template_key: "tech_risk_analysis"
}
};
    const screen = await renderDetail({ planRevisions: [unboundRevision] });
    await userEvent.click(screen.getByTestId("pipeline-stage-plan"));

    await expect.element(screen.getByText("调度顺序")).toBeVisible();
    await expect.element(screen.getByText("tech_risk_analysis")).not.toBeInTheDocument();
    await expect.element(screen.getByText("risk_report")).not.toBeInTheDocument();
    await expect.element(screen.getByText("交付出口")).not.toBeInTheDocument();
  });

  it("keeps plan review read-only in the pipeline plan cell (inbox is the write entry)", async () => {
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
        template_version: 2
}
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
      title_snapshot: "确认计划版本 v1"
};
    const screen = await renderDetail({
      decisionRequests: [planReviewDecision],
      onResolveDecision,
      planRevisions: [templatedRevision]
});

    const planCell = screen.getByTestId("pipeline-stage-plan");
    await expect
      .element(planCell.getByText("待收件箱确认 · 点击查看详情"))
      .toBeVisible();
    await userEvent.click(planCell);
    await expect
      .element(screen.getByTestId("plan-review-inbox-only"))
      .toBeVisible();
    await expect
      .element(screen.getByRole("combobox", { name: "改选交付出口" }))
      .not.toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "要求修改计划版本 v1" }))
      .not.toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "批准计划版本 v1" }))
      .not.toBeInTheDocument();
    expect(onResolveDecision).not.toHaveBeenCalled();
  });

  it("shows pending plan review as read-only when a newer revision arrives", async () => {
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
        template_version: 2
},
      revision_number: 1
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
        exit_deliverable: "review_verdict"
},
      revision_number: 2
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
      title_snapshot: "确认计划版本"
};
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } }
});
    const screen = await render(
      <QueryClientProvider client={queryClient}>
        {detailElement({
          decisionRequests: [planReviewDecision],
          onResolveDecision,
          planRevisions: [revisionA]
})}
      </QueryClientProvider>,
    );

    await userEvent.click(screen.getByTestId("pipeline-stage-plan"));
    await expect
      .element(screen.getByTestId("plan-review-inbox-only"))
      .toBeVisible();

    await screen.rerender(
      <QueryClientProvider client={queryClient}>
        {detailElement({
          decisionRequests: [planReviewDecision],
          onResolveDecision,
          planRevisions: [revisionB]
})}
      </QueryClientProvider>,
    );

    await expect
      .element(screen.getByTestId("plan-review-inbox-only"))
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "要求修改计划版本 v2" }))
      .not.toBeInTheDocument();
    expect(onResolveDecision).not.toHaveBeenCalled();
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
            title: "收集接入证据"
},
          {
            blocked_by_keys: ["collect-evidence"],
            employee_selection_reason: "负责复核证据并形成结论。",
            planned_task_key: "review-evidence",
            selected_employee_id: "employee-reviewer",
            title: "复核接入证据"
},
        ]
}
};
    const screen = await renderDetail({ planRevisions: [blockedByKeysRevision] });
    await userEvent.click(screen.getByTestId("pipeline-stage-plan"));
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
        allowed_actions: ["project.archive", "project.delete"]
}
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
        allowed_actions: ["project.demand.submit"]
}
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
            tenant_id: "tenant-1"
},
        ]
}
});

    await expect.element(screen.getByText("接入证据整理完成")).toBeVisible();
    const time = screen.container.querySelector('time[datetime="2026-07-20T01:00:00Z"]');
    expect(time).not.toBeNull();
    expect(time?.textContent ?? "").toMatch(/\d+\s*天前/);
  });
});
