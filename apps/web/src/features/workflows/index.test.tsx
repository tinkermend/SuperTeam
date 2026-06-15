import {
  forwardRef,
  type AnchorHTMLAttributes,
  type MouseEvent,
  type ReactNode,
} from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
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
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>,
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>,
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

  type MockReactFlowProps = {
    children?: ReactNode;
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
    ReactFlow: ({ children, nodes = [], onNodeClick, onPaneClick }: MockReactFlowProps) => (
      <div data-testid="workflow-canvas">
        <button onClick={onPaneClick} type="button">
          canvas pane
        </button>
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

describe("WorkflowView", () => {
  it("renders visible workflow instances and selected planning detail", async () => {
    const screen = await renderWorkflowView();

    await expect.element(screen.getByText("支付成功率下降").first()).toBeVisible();
    await expect.element(screen.getByText("支付项目").first()).toBeVisible();
    await expect.element(screen.getByText("任务正在规划")).toBeVisible();
  });

  it("renders selected demand task and other visible instances when graph has nodes", async () => {
    const screen = await renderWorkflowView({
      fetcher: createWorkflowFetcher({
        graph: makeGraph([makeGraphNode("task-1", "服务健康巡检", "assigned")]),
      }),
    });

    await expect.element(screen.getByText("服务健康巡检").first()).toBeVisible();
    await expect.element(screen.getByTestId("workflow-canvas")).toBeVisible();
    await expect.element(screen.getByText("节点详情")).toBeVisible();
    await expect.element(screen.getByText("assigned")).toBeVisible();
    await expect.element(screen.getByText("PR 审查")).toBeVisible();
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

  it("updates the inspector on task selection and resets pane clicks to the initial task", async () => {
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

    await expect.element(screen.getByRole("heading", { name: "失败任务" })).toBeVisible();
    await expect.element(screen.getByText("failed").first()).toBeVisible();
    await expect.element(screen.getByText("失败报告")).toBeVisible();
    await expect.element(screen.getByText("failed · codex · runtime-a")).toBeVisible();

    await screen.getByRole("button", { name: "巡检任务" }).click();

    await expect.element(screen.getByRole("heading", { name: "巡检任务" })).toBeVisible();
    await expect.element(screen.getByText("assigned").first()).toBeVisible();
    await expect.element(screen.getByText("巡检报告")).toBeVisible();
    await expect.element(screen.getByText("queued · codex · runtime-b")).toBeVisible();

    await screen.getByRole("button", { name: "canvas pane" }).click();

    await expect.element(screen.getByRole("heading", { name: "失败任务" })).toBeVisible();
    await expect.element(screen.getByText("失败报告")).toBeVisible();
  });

  it("keeps the parent task selected when a decision attachment node is clicked", async () => {
    const graph = makeGraph(
      [
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

    await expect.element(screen.getByRole("heading", { name: "待审批任务" })).toBeVisible();

    await screen.getByRole("button", { name: "确认上线风险" }).click();

    await expect.element(screen.getByRole("heading", { name: "待审批任务" })).toBeVisible();
    await expect.element(screen.getByText("选择节点查看详情")).not.toBeInTheDocument();
    await expect.element(screen.getByText("审批结果")).toBeVisible();
  });

  it("navigates to the first visible instance when no demand id is provided", async () => {
    mocks.navigate.mockClear();
    await renderWorkflowView({ demandId: undefined });

    await vi.waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        params: { demandId: "demand-running" },
        replace: true,
        to: "/workflows/$demandId",
      });
    });
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

    const requestedUrls = (
      fetcher as unknown as { mock: { calls: [RequestInfo | URL, RequestInit?][] } }
    ).mock.calls.map(([input]) => String(input));

    expect(requestedUrls).not.toContain(
      "http://control-plane.local/api/v1/project-demands/demand-running/launch-detail",
    );
  });
});
