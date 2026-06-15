import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { WorkflowView } from "@/features/workflows";
import type {
  Project,
  ProjectDemandLaunchDetail,
  ProjectTaskGraph,
  ProjectTaskGraphNode,
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

function makeGraph(nodes: ProjectTaskGraphNode[] = []): ProjectTaskGraph {
  return {
    decision_requests: [],
    edges: [],
    employees: [],
    execution_summaries: [],
    nodes,
    recent_events: [],
    runs: [],
  };
}

function makeGraphNode(
  id: string,
  title: string,
  status = "running",
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
  };
}

function createWorkflowFetcher({
  graph = makeGraph(),
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

    if (url.pathname === "/api/v1/projects/project-1/task-graph" && method === "GET") {
      expect(url.searchParams.get("demand_id")).toBe("demand-running");
      return jsonResponse(graph);
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
        graph: makeGraph([makeGraphNode("task-1", "服务健康巡检")]),
      }),
    });

    await expect.element(screen.getByText("服务健康巡检")).toBeVisible();
    await expect.element(screen.getByText("PR 审查")).toBeVisible();
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
