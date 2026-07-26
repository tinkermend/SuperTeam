import type { AnchorHTMLAttributes, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectDemandsSection } from "./project-demands-section";
import type { ProjectDemand, ProjectTaskGraph } from "@/lib/api/projects";

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

function demand(overrides: Partial<ProjectDemand>): ProjectDemand {
  return {
    attachments: [],
    content: "需求内容",
    id: "demand-1",
    project_id: "project-1",
    reviewer: null,
    source_refs: {},
    source_type: "manual",
    status: "executing",
    submitted_by_user_id: "human-owner-1",
    tenant_id: "tenant-1",
    title: "需求一",
    ...overrides
};
}

/** 固定 3 小时 5 分钟前，保证 formatRelativeTime 稳定输出 "3 小时前"。 */
const latestDemandCreatedAt = new Date(
  Date.now() - (3 * 60 + 5) * 60 * 1000,
).toISOString();

const demands: ProjectDemand[] = [
  demand({
    content: "为验收整理接入材料",
    created_at: latestDemandCreatedAt,
    id: "demand-latest",
    status: "acceptance_pending",
    title: "整理验收材料"
}),
  // demand-old 不带 created_at：覆盖缺值不渲染时间的分支。
  demand({
    content: "历史需求内容",
    id: "demand-old",
    status: "completed",
    title: "历史巡检需求"
}),
];

function graphFor(demandId: string, taskTitle: string): ProjectTaskGraph {
  return {
    blocking_facts: [],
    decision_requests: [],
    edges: [],
    employees: [],
    execution_summaries: [],
    nodes: [
      {
        demand_id: demandId,
        expected_outputs: [],
        handoff_contract: {},
        id: `task-${demandId}`,
        input_requirements: {},
        planner_metadata: {},
        project_id: "project-1",
        requires_human_approval: false,
        stage_index: 0,
        status: "running",
        tenant_id: "tenant-1",
        title: taskTitle
},
    ],
    recent_events: [],
    runs: []
};
}

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    headers: { "Content-Type": "application/json" },
    status
});
}

/** 直连 API 的最小 stub：血缘为空、launch detail 携带一条 pending 决策。 */
function stubFetcher(pendingDecisionTitle = "确认执行风险"): typeof fetch {
  return async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes("/launch-detail")) {
      const demandId = /project-demands\/([^/]+)\/launch-detail/.exec(url)?.[1] ?? "";
      return jsonResponse({
        coordination_jobs: [],
        decision_requests: [
          {
            approval_request_id: "approval-1",
            created_at: "2026-07-25T08:00:00Z",
            decision_type: "risk_review",
            id: `decision-${demandId}`,
            project_id: "project-1",
            status_snapshot: "pending",
            summary_snapshot: "需要负责人确认",
            target_user_id: "human-owner-1",
            tenant_id: "tenant-1",
            title_snapshot: pendingDecisionTitle
},
        ],
        demand: { id: demandId },
        execution_summaries: [],
        project: { id: "project-1" },
        project_tasks: [],
        recent_events: [],
        reviewer: null,
        route_decisions: []
});
    }
    if (url.includes("/acceptance-criteria")) {
      return jsonResponse({ criteria: [], demand_status: "executing" });
    }
    return jsonResponse({ detail: "not found" }, 404);
  };
}

function renderSection(
  props: Partial<React.ComponentProps<typeof ProjectDemandsSection>> = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } }
});
  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectDemandsSection
        apiBaseUrl="http://cp.test"
        apiOptions={{ baseUrl: "http://cp.test", fetcher: stubFetcher() }}
        demands={demands}
        fetchTaskGraph={vi
          .fn()
          .mockImplementation((demandId: string) =>
            Promise.resolve(
              graphFor(
                demandId,
                demandId === "demand-latest" ? "整理材料任务" : "巡检任务",
              ),
            ),
          )}
        onOpenTask={vi.fn()}
        projectId="project-1"
        {...props}
      />
    </QueryClientProvider>,
  );
}

describe("ProjectDemandsSection", () => {
  it("renders the demand switcher with status pills and defaults to the latest demand", async () => {
    const fetchTaskGraph = vi
      .fn()
      .mockImplementation((demandId: string) =>
        Promise.resolve(graphFor(demandId, "整理材料任务")),
      );
    const screen = await renderSection({ fetchTaskGraph });

    // 左侧切换器：两条需求 + 中文状态 pill + ?tab=demands&demand= 深链。
    await expect
      .element(screen.getByTestId("demand-list-item-demand-latest"))
      .toHaveAttribute("href", "/projects/project-1?demand=demand-latest&tab=demands");
    await expect
      .element(screen.getByTestId("demand-list-item-demand-old"))
      .toHaveAttribute("href", "/projects/project-1?demand=demand-old&tab=demands");
    await expect
      .element(screen.getByTestId("demand-list-item-demand-latest"))
      .toHaveAttribute("aria-current", "true");
    await expect.element(screen.getByText("已完成")).toBeVisible();

    // 状态 pill 旁的相对创建时间：有 created_at 渲染，缺值不渲染。
    const latestItem = screen.getByTestId("demand-list-item-demand-latest");
    await expect.element(latestItem.getByText("3 小时前")).toBeVisible();
    const oldItemElement = screen
      .getByTestId("demand-list-item-demand-old")
      .element();
    expect(oldItemElement.querySelector("time")).toBeNull();

    // 右侧状态头默认最新需求，权威图按该需求拉取。
    const header = screen.getByTestId("demand-status-header");
    await expect.element(header.getByText("整理验收材料")).toBeVisible();
    await expect.element(header.getByText("待验收")).toBeVisible();
    await expect.element(header.getByText("为验收整理接入材料")).toBeVisible();
    expect(fetchTaskGraph).toHaveBeenCalledWith("demand-latest");
    await expect.element(screen.getByTestId("flow-graph-canvas")).toBeInTheDocument();

    // 该需求的待决面板：pending 卡片 + 收件箱深链，不新造交互。
    await expect.element(screen.getByText("确认执行风险")).toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "前往收件箱处理待决事项" }))
      .toHaveAttribute("href", "/inbox");

    // 验收血缘面板（迁移后的 DemandCriteriaPanel）就地渲染。
    await expect.element(screen.getByText("本需求未声明验收判据")).toBeVisible();
  });

  it("selects the demand from the ?demand= deep link and fetches its graph", async () => {
    const fetchTaskGraph = vi
      .fn()
      .mockImplementation((demandId: string) =>
        Promise.resolve(graphFor(demandId, "巡检任务")),
      );
    const screen = await renderSection({
      fetchTaskGraph,
      selectedDemandId: "demand-old"
});

    const header = screen.getByTestId("demand-status-header");
    await expect.element(header.getByText("历史巡检需求")).toBeVisible();
    await expect.element(header.getByText("历史需求内容")).toBeVisible();
    await expect
      .element(screen.getByTestId("demand-list-item-demand-old"))
      .toHaveAttribute("aria-current", "true");
    expect(fetchTaskGraph).toHaveBeenCalledWith("demand-old");
    expect(fetchTaskGraph).not.toHaveBeenCalledWith("demand-latest");
  });

  it("shows an empty state when the project has no demands", async () => {
    const screen = await renderSection({ demands: [] });

    await expect.element(screen.getByText("暂无需求")).toBeVisible();
  });

  it("renders the authoritative graph in live mode with a 30s fallback polling loop", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const fetchTaskGraph = vi
        .fn()
        .mockImplementation((demandId: string) =>
          Promise.resolve(graphFor(demandId, "整理材料任务")),
        );
      const screen = await renderSection({ fetchTaskGraph });

      // 活图模式只在需求流程区开启（spec 2026-07-27 §2）。
      await expect
        .element(screen.getByTestId("flow-graph-canvas"))
        .toHaveAttribute("data-live", "true");

      // 数据活性（spec §5 P2-E）：秒级刷新走 SSE invalidate，轮询降为 30s 保底。
      const callsBeforePoll = fetchTaskGraph.mock.calls.length;
      await vi.advanceTimersByTimeAsync(10_000);
      expect(fetchTaskGraph.mock.calls.length).toBe(callsBeforePoll);
      await vi.advanceTimersByTimeAsync(21_000);
      expect(fetchTaskGraph.mock.calls.length).toBeGreaterThan(callsBeforePoll);
    } finally {
      vi.useRealTimers();
    }
  });

  it("refetches the graph when the activity SSE pushes an event of this project", async () => {
    const listeners: Record<string, Array<(event: { data: string }) => void>> = {};
    const streamUrls: string[] = [];
    const fakeSource = {
      addEventListener: (type: string, listener: (event: { data: string }) => void) => {
        (listeners[type] ??= []).push(listener);
      },
      close: () => undefined,
      removeEventListener: () => undefined
} as unknown as EventSource;
    const fetchTaskGraph = vi
      .fn()
      .mockImplementation((demandId: string) =>
        Promise.resolve(graphFor(demandId, "整理材料任务")),
      );
    const screen = await renderSection({
      eventSourceFactory: (url) => {
        streamUrls.push(url);
        return fakeSource;
      },
      fetchTaskGraph
});
    await expect.element(screen.getByTestId("flow-graph-canvas")).toBeInTheDocument();
    expect(streamUrls).toEqual([
      "http://cp.test/api/v1/digital-employees/activity/stream",
    ]);

    // 非本项目事件被过滤：不触发重取。
    const callsBefore = fetchTaskGraph.mock.calls.length;
    for (const listener of listeners["activity"] ?? []) {
      listener({ data: JSON.stringify({ event_id: "evt-x", project_id: "project-2" }) });
    }
    await new Promise((resolve) => setTimeout(resolve, 150));
    expect(fetchTaskGraph.mock.calls.length).toBe(callsBefore);

    // 本项目事件：invalidate ["project-task-graph", projectId] → 活跃 graph 查询重取。
    for (const listener of listeners["activity"] ?? []) {
      listener({ data: JSON.stringify({ event_id: "evt-1", project_id: "project-1" }) });
    }
    await expect
      .poll(() => fetchTaskGraph.mock.calls.length)
      .toBeGreaterThan(callsBefore);
  });
});
