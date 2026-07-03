import {
  forwardRef,
  type AnchorHTMLAttributes,
  type MouseEvent,
  type ReactNode,
} from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { WorkflowView } from "@/features/workflows";
import type {
  ProjectDecisionRequest,
  ProjectExecutionSummary,
  Project,
  ProjectDemandLaunchDetail,
  ProjectTaskGraph,
  ProjectTaskGraphNode,
  ProjectTaskGraphRun,
  WorkflowInstanceSummary,
} from "@/lib/api/projects";

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
}));

vi.mock("@/components/layout/header", () => ({
  Header: ({
    children,
    showSidebarTrigger = true,
  }: {
    children?: ReactNode;
    showSidebarTrigger?: boolean;
  }) => (
    <header data-slot="v3-shell-header">
      {showSidebarTrigger ? (
        <button data-slot="sidebar-trigger" type="button">
          切换侧栏
        </button>
      ) : (
        children
      )}
      <button
        className={showSidebarTrigger ? "" : "justify-self-center max-w-sm"}
        type="button"
      >
        搜索任务、数字员工、能力、文档或快捷命令... ⌘ K
      </button>
    </header>
  ),
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children, className }: { children: ReactNode; className?: string }) => (
    <main className={className}>{children}</main>
  ),
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>,
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>,
}));

vi.mock("@xyflow/react", () => {
  type MockNode = {
    data?: {
      title?: string;
    };
    id: string;
    parentId?: string;
  };

  type MockEdge = {
    id: string;
    label?: ReactNode;
  };

  type MockReactFlowProps = {
    children?: ReactNode;
    edges?: MockEdge[];
    nodes?: MockNode[];
    onNodeClick?: (event: MouseEvent<HTMLButtonElement>, node: MockNode) => void;
    onPaneClick?: () => void;
  };

  return {
    Background: () => null,
    Controls: () => null,
    Handle: () => null,
    MiniMap: () => null,
    Position: { Bottom: "bottom", Top: "top" },
    ReactFlow: ({
      children,
      edges = [],
      nodes = [],
      onNodeClick,
      onPaneClick,
    }: MockReactFlowProps) => (
      <div data-testid="workflow-canvas">
        <button onClick={onPaneClick} type="button">
          canvas pane
        </button>
        {edges.map((edge) => (
          <span data-testid={`workflow-edge-label-${edge.id}`} key={edge.id}>
            {edge.label}
          </span>
        ))}
        {nodes.map((node) => (
          <button key={node.id} onClick={(event) => onNodeClick?.(event, node)} type="button">
            {node.data?.title ?? node.id}
          </button>
        ))}
        {children}
      </div>
    ),
  };
});

vi.mock("@tanstack/react-router", () => {
  type MockLinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
    children: ReactNode;
    params?: Record<string, string>;
    to: string;
  };
  const Link = forwardRef<HTMLAnchorElement, MockLinkProps>(
    ({ children, params, to, ...props }, ref) => {
      const href = Object.entries(params ?? {}).reduce(
        (path, [key, value]) => path.replace(`$${key}`, encodeURIComponent(value)),
        to,
      );

      return (
        <a {...props} href={href} ref={ref}>
          {children}
        </a>
      );
    },
  );
  Link.displayName = "MockRouterLink";

  return {
    Link,
    useNavigate: () => mocks.navigate,
  };
});

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function makeWorkflowInstance(
  demandId: string,
  overrides: Partial<WorkflowInstanceSummary> = {},
): WorkflowInstanceSummary {
  return {
    created_at: "2026-06-15T08:00:00Z",
    demand_id: demandId,
    progress: {
      blocked_nodes: 0,
      completed_nodes: 0,
      running_nodes: 0,
      total_nodes: 0,
      waiting_human_nodes: 0,
    },
    project_id: "project-1",
    project_name: "支付项目",
    selected_coordination_job_id: "job-1",
    status: "planning",
    status_reason: "等待协调线程生成任务",
    submitted_by_display_name: "负责人",
    submitted_by_user_id: "owner-1",
    title: "支付成功率下降",
    updated_at: "2026-06-15T08:05:00Z",
    ...overrides,
  };
}

function makeProject(): Project {
  return {
    approval_policy: {},
    coordination_policy: {},
    coordination_status: "registered",
    coordination_workflow_id: "project-coordinator:project-1",
    evidence_policy: {},
    goal: "恢复支付成功率",
    human_owner_user_id: "owner-1",
    id: "project-1",
    name: "支付项目",
    status: "running",
    tenant_id: "tenant-1",
  };
}

function makeLaunchDetail(
  demandId: string,
  overrides: Partial<ProjectDemandLaunchDetail> = {},
): ProjectDemandLaunchDetail {
  return {
    coordination_jobs: [
      {
        id: "job-1",
        input_snapshot_ref: {},
        job_type: "demand_planning",
        output_event_ids: [],
        project_id: "project-1",
        status: "running",
        tenant_id: "tenant-1",
        workflow_id: "project-coordinator:project-1",
      },
    ],
    decision_requests: [],
    demand: {
      attachments: [],
      content: "生产支付链路成功率持续下降，需要定位并恢复。",
      id: demandId,
      project_id: "project-1",
      reviewer: null,
      source_refs: {},
      source_type: "manual",
      status: "planning_pending",
      submitted_by_user_id: "owner-1",
      tenant_id: "tenant-1",
      title: "支付成功率下降",
    },
    project: makeProject(),
    project_tasks: [],
    recent_events: [],
    reviewer: null,
    route_decisions: [],
    ...overrides,
  };
}

function makeGraph(
  nodes: ProjectTaskGraphNode[] = [],
  overrides: Partial<ProjectTaskGraph> = {},
): ProjectTaskGraph {
  return {
    decision_requests: overrides.decision_requests ?? [],
    edges: [],
    employees: [],
    execution_summaries: overrides.execution_summaries ?? [],
    nodes,
    recent_events: [],
    runs: overrides.runs ?? [],
    ...overrides,
  };
}

function makeGraphNode(
  id: string,
  title: string,
  status = "running",
  overrides: Partial<ProjectTaskGraphNode> = {},
): ProjectTaskGraphNode {
  return {
    expected_outputs: [],
    handoff_contract: {},
    id,
    input_requirements: {},
    planner_metadata: {},
    project_id: "project-1",
    requires_human_approval: false,
    status,
    tenant_id: "tenant-1",
    title,
    ...overrides,
  };
}

function createWorkflowFetcher({
  graph = makeGraph(),
  graphsByDemandId,
  instances = [
    makeWorkflowInstance("demand-running"),
    makeWorkflowInstance("demand-pr", {
      demand_id: "demand-pr",
      progress: {
        blocked_nodes: 0,
        completed_nodes: 2,
        running_nodes: 1,
        total_nodes: 4,
        waiting_human_nodes: 0,
      },
      project_name: "代码审查项目",
      status: "running",
      status_reason: "",
      title: "PR 审查",
    }),
  ],
}: {
  graph?: ProjectTaskGraph;
  graphsByDemandId?: Record<string, ProjectTaskGraph>;
  instances?: WorkflowInstanceSummary[];
} = {}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/workflow-instances" && method === "GET") {
      return jsonResponse(instances);
    }

    if (
      url.pathname === "/api/v1/project-demands/demand-running/launch-detail" &&
      method === "GET"
    ) {
      return jsonResponse(makeLaunchDetail("demand-running"));
    }

    if (
      url.pathname === "/api/v1/project-demands/demand-pr/launch-detail" &&
      method === "GET"
    ) {
      return jsonResponse(makeLaunchDetail("demand-pr", {
        demand: {
          ...makeLaunchDetail("demand-pr").demand,
          title: "PR 审查",
        },
      }));
    }

    if (url.pathname === "/api/v1/projects/project-1/task-graph" && method === "GET") {
      const demandId = url.searchParams.get("demand_id");
      return jsonResponse(graphsByDemandId?.[demandId ?? ""] ?? graph);
    }

    return jsonResponse({ error: `unhandled ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;
}

async function renderWorkflowView(options: {
  demandId?: string;
  fetcher?: typeof fetch;
} = {}) {
  const { demandId, fetcher = createWorkflowFetcher() } = options;
  const resolvedDemandId = Object.prototype.hasOwnProperty.call(
    options,
    "demandId",
  )
    ? demandId
    : "demand-running";

  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <WorkflowView
        apiBaseUrl="http://control-plane.local"
        demandId={resolvedDemandId}
        fetcher={fetcher}
      />
    </QueryClientProvider>,
  );
}

function requestedUrls(fetcher: typeof fetch): string[] {
  return (
    fetcher as unknown as { mock: { calls: [RequestInfo | URL, RequestInit?][] } }
  ).mock.calls.map(([input]) => String(input));
}

describe("WorkflowView", () => {
  it("renders workflow entrance with visible workflow instances", async () => {
    const fetcher = createWorkflowFetcher();
    const screen = await renderWorkflowView({ demandId: undefined, fetcher });

    await expect.element(screen.getByRole("heading", { name: "流程编排" })).toBeVisible();
    await expect.element(screen.getByRole("link", { name: /支付成功率下降/ })).toBeVisible();
    await expect.element(screen.getByText("支付项目").first()).toBeVisible();
    await expect.element(screen.getByText("等待协调线程生成任务")).toBeVisible();
    await expect.element(screen.getByText("任务正在规划")).not.toBeInTheDocument();
    await expect.element(screen.getByTestId("workflow-canvas")).not.toBeInTheDocument();

    const urls = requestedUrls(fetcher);
    expect(urls).toEqual([
      "http://control-plane.local/api/v1/workflow-instances?limit=50&offset=0",
    ]);
    expect(urls.some((url) => url.includes("/project-demands/"))).toBe(false);
    expect(urls.some((url) => url.includes("/task-graph"))).toBe(false);
  });

  it("renders workflow entrance with v3 soft-flat containers", async () => {
    const screen = await renderWorkflowView({ demandId: undefined });

    await expect.element(screen.getByRole("heading", { name: "流程编排" })).toBeVisible();
    await expect.element(screen.getByRole("table", { name: "流程实例列表" })).toBeVisible();

    expect(document.body.querySelector('[data-slot="v3-soft-card"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-signature-card"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-work-surface"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-table"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-status-pill"]')).not.toBeNull();
    expect(document.body.innerHTML).not.toContain(["superteam", ""].join("-"));
  });

  it("keeps the workflow page header in the global top bar", async () => {
    const screen = await renderWorkflowView({ demandId: undefined });

    const heading = screen.getByRole("heading", { name: "流程编排" }).element() as HTMLElement;
    const shellHeader = document.body.querySelector('[data-slot="v3-shell-header"]');
    const mainHeader = document.body.querySelector('main [data-slot="v3-page-header"]');
    const sidebarTrigger = document.body.querySelector('[data-slot="sidebar-trigger"]');
    const subtitle = screen.getByText("查看需求触发的规划、执行、阻塞和结果状态").element() as HTMLElement;
    const search = screen.getByRole("button", {
      name: /搜索任务、数字员工、能力、文档或快捷命令/,
    }).element() as HTMLElement;

    expect(shellHeader).toBeInstanceOf(HTMLElement);
    expect(shellHeader?.contains(heading)).toBe(true);
    expect(mainHeader).toBeNull();
    expect(sidebarTrigger).toBeNull();
    expect(subtitle.className).toContain("text-xs");
    expect(search.className).toContain("justify-self-center");
    expect(search.className).toContain("max-w-sm");
    expect(document.body.querySelector("main")?.className).not.toContain("bg-v3-bg");
  });

  it("shows optional SLA priority and risk on workflow cards only when present", async () => {
    const screen = await renderWorkflowView({
      demandId: undefined,
      fetcher: createWorkflowFetcher({
        instances: [
          makeWorkflowInstance("demand-running", {
            priority: {
              label: "P1",
              source: "project_profile",
              value: "p1",
            },
            risk: {
              label: "高风险",
              level: "high",
              source: "risk_policy",
            },
            sla: {
              breached: false,
              label: "剩余 18 分钟",
              remaining_seconds: 1080,
              source: "sla_policy",
            },
          }),
          makeWorkflowInstance("demand-pr", {
            demand_id: "demand-pr",
            project_name: "代码审查项目",
            status: "running",
            status_reason: "",
            title: "PR 审查",
          }),
        ],
      }),
    });

    await expect.element(screen.getByText("P1")).toBeVisible();
    await expect.element(screen.getByText("高风险")).toBeVisible();
    await expect.element(screen.getByText("剩余 18 分钟")).toBeVisible();

    const paymentLink = screen.getByRole("link", { name: /支付成功率下降/ }).element();
    expect(paymentLink.textContent).toContain("P1");
    expect(paymentLink.textContent).toContain("高风险");
    expect(paymentLink.textContent).toContain("剩余 18 分钟");

    const prLink = screen.getByRole("link", { name: /PR 审查/ }).element();
    expect(prLink.textContent).not.toContain("P1");
  });

  it("renders the selected demand task graph full width without a sidebar list", async () => {
    const screen = await renderWorkflowView({
      fetcher: createWorkflowFetcher({
        graph: makeGraph([makeGraphNode("task-1", "服务健康巡检", "assigned")]),
      }),
    });

    await expect.element(screen.getByText("服务健康巡检").first()).toBeVisible();
    await expect.element(screen.getByTestId("workflow-canvas")).toBeVisible();
    expect(document.body.querySelector('[data-slot="v3-soft-card"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-icon-tile"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-status-pill"]')).not.toBeNull();

    // 节点详情仅在点击节点后以弹窗形式出现，进入页面时不预选、不渲染固定卡片
    await expect.element(screen.getByRole("dialog", { name: "节点详情" })).not.toBeInTheDocument();

    // 流程实例侧栏已废弃，"PR 审查" 不再作为详情页条目出现
    await expect.element(screen.getByText("PR 审查")).not.toBeInTheDocument();
  });

  it("renders the demand summary in the graph surface above the canvas", async () => {
    const screen = await renderWorkflowView({
      fetcher: createWorkflowFetcher({
        graph: makeGraph([makeGraphNode("task-1", "服务健康巡检", "assigned")]),
      }),
    });

    await expect.element(screen.getByTestId("workflow-canvas")).toBeVisible();

    const canvasElement = screen.getByTestId("workflow-canvas").element();
    const graphShell = canvasElement.parentElement;
    const graphAndInspectorGrid = graphShell?.parentElement;
    const detailMainCard = graphAndInspectorGrid?.parentElement;

    expect(detailMainCard?.textContent).toContain("需求摘要");
    expect(detailMainCard?.textContent).toContain("支付成功率下降");
    expect(detailMainCard?.textContent).toContain("生产支付链路成功率持续下降，需要定位并恢复。");
  });

  it("renders workflow dependency edge statuses in Chinese", async () => {
    const screen = await renderWorkflowView({
      fetcher: createWorkflowFetcher({
        graph: makeGraph(
          [
            makeGraphNode("task-1", "任务一", "planned"),
            makeGraphNode("task-2", "任务二", "waiting_human"),
            makeGraphNode("task-3", "任务三", "completed"),
          ],
          {
            edges: [
              {
                blocker_task_id: "task-1",
                dependent_task_id: "task-2",
                edge_status: "waiting_human",
              },
              {
                blocker_task_id: "task-2",
                dependent_task_id: "task-3",
                edge_status: "completed",
              },
            ],
          },
        ),
      }),
    });

    await expect.element(screen.getByTestId("workflow-canvas")).toBeVisible();

    const canvasText = screen.getByTestId("workflow-canvas").element().textContent ?? "";
    expect(canvasText).toContain("等待人工");
    expect(canvasText).toContain("已完成");
    expect(canvasText).not.toContain("waiting_human");
    expect(canvasText).not.toContain("completed");
  });

  it("renders the canvas full width without a fixed inspector column", async () => {
    const screen = await renderWorkflowView({
      fetcher: createWorkflowFetcher({
        graph: makeGraph([makeGraphNode("task-1", "服务健康巡检", "assigned")]),
      }),
    });

    await expect.element(screen.getByTestId("workflow-canvas")).toBeVisible();

    // 整个流程详情根节点不再使用「图 + 检查器」双列 grid，
    // 也不再使用任何固定检查器列宽
    const root = screen.getByTestId("workflow-canvas").element().closest(".min-w-0");
    const detailHtml = document.body.innerHTML;

    expect(detailHtml).not.toContain("@5xl/workflow-graph:grid-cols-[minmax(0,1fr)_360px]");
    expect(detailHtml).not.toContain("xl:grid-cols-[360px_minmax(0,1fr)]");
    // 容器宽度仍受 min-w-0 约束，保证画布可缩放
    expect(root).not.toBeNull();
  });

  it("does not render a previous demand graph under the current demand detail", async () => {
    const screen = await renderWorkflowView({
      demandId: "demand-pr",
      fetcher: createWorkflowFetcher({
        graphsByDemandId: {
          "demand-pr": makeGraph([
            makeGraphNode("task-stale", "上一需求任务", "assigned", {
              demand_id: "demand-running",
            }),
          ]),
        },
      }),
    });

    await expect.element(screen.getByText("PR 审查").first()).toBeVisible();
    await expect.element(screen.getByText("任务正在规划")).toBeVisible();
    await expect.element(screen.getByText("上一需求任务")).not.toBeInTheDocument();
  });

  it("opens a centered dialog with node details when a task node is clicked", async () => {
    const graph = makeGraph(
      [
        makeGraphNode("task-failed", "失败任务", "failed", {
          expected_outputs: ["失败报告"],
        }),
        makeGraphNode("task-assigned", "巡检任务", "assigned", {
          expected_outputs: ["巡检报告"],
        }),
      ],
      {
        execution_summaries: [
          {
            artifact_refs: [],
            confidence_factors: {},
            conclusion: "失败任务结论",
            digital_employee_id: "employee-1",
            evidence_refs: [],
            id: "summary-failed",
            missing_information: [],
            project_id: "project-1",
            project_task_id: "task-failed",
            requires_human_review: false,
            tenant_id: "tenant-1",
          } satisfies ProjectExecutionSummary,
          {
            artifact_refs: [],
            confidence_factors: {},
            conclusion: "巡检任务结论",
            digital_employee_id: "employee-2",
            evidence_refs: [],
            id: "summary-assigned",
            missing_information: [],
            project_id: "project-1",
            project_task_id: "task-assigned",
            requires_human_review: false,
            tenant_id: "tenant-1",
          } satisfies ProjectExecutionSummary,
        ],
        runs: [
          {
            project_task_id: "task-failed",
            provider_type: "codex",
            runtime_node_summary: "runtime-a",
            runtime_task_id: "runtime-task-failed",
            status: "failed",
          } satisfies ProjectTaskGraphRun,
          {
            project_task_id: "task-assigned",
            provider_type: "codex",
            runtime_node_summary: "runtime-b",
            runtime_task_id: "runtime-task-assigned",
            status: "queued",
          } satisfies ProjectTaskGraphRun,
        ],
      },
    );
    const screen = await renderWorkflowView({
      fetcher: createWorkflowFetcher({ graph }),
    });

    // 进入页面时不预选节点，节点详情弹窗不出现，也不会泄露其他节点内容
    await expect.element(screen.getByRole("dialog", { name: "节点详情" })).not.toBeInTheDocument();
    await expect.element(screen.getByText("失败报告")).not.toBeInTheDocument();
    await expect.element(screen.getByText("巡检报告")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "巡检任务" }));

    // 点击节点后弹出居中弹窗，展示该节点详情
    await expect.element(screen.getByRole("dialog", { name: "节点详情" })).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "巡检任务" })).toBeVisible();
    await expect.element(screen.getByText("巡检报告")).toBeVisible();
    await expect.element(screen.getByText("排队中 · codex · runtime-b")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "查看巡检任务 Runtime" })).toBeVisible();

    // 通过弹窗关闭按钮收起弹窗
    await userEvent.click(screen.getByRole("button", { name: "Close" }));

    await expect.element(screen.getByRole("dialog", { name: "节点详情" })).not.toBeInTheDocument();
  });

  it("updates the inspector to the parent task when a decision attachment node is clicked", async () => {
    const graph = makeGraph(
      [
        makeGraphNode("task-failed", "失败任务", "failed", {
          expected_outputs: ["失败报告"],
        }),
        makeGraphNode("task-review", "待审批任务", "waiting_human", {
          expected_outputs: ["审批结果"],
        }),
      ],
      {
        decision_requests: [
          {
            approval_request_id: "approval-1",
            decision_type: "human_approval",
            id: "decision-1",
            project_id: "project-1",
            project_task_id: "task-review",
            status_snapshot: "pending",
            target_user_id: "owner-1",
            tenant_id: "tenant-1",
            title_snapshot: "确认上线风险",
          } satisfies ProjectDecisionRequest,
        ],
      },
    );
    const screen = await renderWorkflowView({
      fetcher: createWorkflowFetcher({ graph }),
    });

    await expect.element(screen.getByRole("dialog", { name: "节点详情" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "确认上线风险" }));

    // 点击决策附件节点后弹出居中弹窗，并解析到其父任务「待审批任务」
    await expect.element(screen.getByRole("dialog", { name: "节点详情" })).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "待审批任务" })).toBeVisible();
    await expect.element(screen.getByText("审批结果")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "查看待审批任务审批" })).toBeVisible();
  });

  it("does not navigate to the first visible instance when no demand id is provided", async () => {
    mocks.navigate.mockClear();
    const fetcher = createWorkflowFetcher();
    const screen = await renderWorkflowView({ demandId: undefined, fetcher });

    await expect.element(screen.getByRole("link", { name: /支付成功率下降/ })).toBeVisible();
    expect(requestedUrls(fetcher)).toEqual([
      "http://control-plane.local/api/v1/workflow-instances?limit=50&offset=0",
    ]);

    expect(mocks.navigate).not.toHaveBeenCalled();
  });

  it("replace-navigates stale demand ids without fetching the first visible demand detail", async () => {
    mocks.navigate.mockClear();
    const fetcher = createWorkflowFetcher();

    await renderWorkflowView({ demandId: "missing-demand", fetcher });

    await vi.waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        params: { demandId: "demand-running" },
        replace: true,
        to: "/workflows/$demandId",
      });
    });

    expect(requestedUrls(fetcher)).not.toContain(
      "http://control-plane.local/api/v1/project-demands/demand-running/launch-detail",
    );
  });
});
