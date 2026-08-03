import type { AnchorHTMLAttributes, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectDemandsSection } from "./project-demands-section";
import type {
  DispatchGateStatus,
  ProjectDemand,
  ProjectEvent,
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

/** 带一条闸门裁决的执行图：闸门记录是"当前是否被闸住"的权威源。 */
function graphWithGate(demandId: string, status: DispatchGateStatus): ProjectTaskGraph {
  const graph = graphFor(demandId, "整理材料任务");
  graph.dispatch_gates = [
    {
      checked_at: "2026-07-25T08:00:00Z",
      project_task_id: graph.nodes[0].id,
      status
},
  ];
  return graph;
}

/** 派发闸门流水事件：仅用于回归"按事件推断会漏报"的场景。 */
function gateEvent(
  eventType: ProjectEvent["event_type"],
  sequenceNumber: number,
  taskId = "task-demand-latest",
): ProjectEvent {
  return {
    actor_id: "coordinator-1",
    actor_type: "workflow",
    created_at: "2026-07-25T08:00:00Z",
    event_type: eventType,
    id: `event-${sequenceNumber}`,
    payload: { project_task_id: taskId },
    project_id: "project-1",
    sequence_number: sequenceNumber,
    tenant_id: "tenant-1",
  };
}

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    headers: { "Content-Type": "application/json" },
    status
});
}

/** 卷宗 stub：服务端已归一的时间线 + 右轨 + 一条待你处理。 */
function dossierFor(demandId: string, overrides: Record<string, unknown> = {}) {
  const source = demands.find((item) => item.id === demandId) ?? demands[0];
  return {
    acceptance: { criteria_total: 0, demand_status: source.status, pending_human_judgment: 0 },
    demand: source,
    effective_playbook: { name: "", produce_kinds: [], source: "none", template_key: null },
    handoff_summary: {
      assessments: [],
      fulfilled: 0,
      partial: 0,
      unfulfilled: 0,
      unknown: 0
},
    pending_actions: [
      {
        created_at: "2026-07-25T08:00:00Z",
        href: { demand_id: demandId, project_id: "project-1", type: "inbox" },
        id: `decision-${demandId}`,
        kind: "risk_review",
        status: "pending",
        title: "确认执行风险"
},
    ],
    project: { id: "project-1", name: "项目一", status: "running" },
    rail: { slots: [] },
    sibling_pending: [
      { demand_id: "demand-latest", open_decisions: 1 },
      { demand_id: "demand-old", open_decisions: 0 },
    ],
    signals: { active_task_count: 1, demand_terminal: false, has_open_decisions: true },
    timeline: {
      items: [
        {
          id: "event-1",
          kind: "task_dispatched",
          occurred_at: "2026-07-25T08:00:00Z",
          severity: "info",
          title: "任务开始 · 整理材料任务"
},
      ],
      truncated: false
},
    ...overrides
};
}

/** 直连 API 的最小 stub：血缘为空、卷宗携带一条待你处理。 */
function stubFetcher(dossierOverrides: Record<string, unknown> = {}): typeof fetch {
  return async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes("/dossier")) {
      const demandId = /project-demands\/([^/?]+)\/dossier/.exec(url)?.[1] ?? "";
      return jsonResponse(dossierFor(demandId, dossierOverrides));
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

    // 左轨待你处理角标：来自卷宗 sibling_pending，需求级决策也算得进。
    await expect
      .element(screen.getByTestId("demand-list-pending-demand-latest"))
      .toHaveTextContent("1");
    expect(
      screen.container.querySelector("[data-testid=demand-list-pending-demand-old]"),
    ).toBeNull();

    // 中栏单头默认最新需求。
    const header = screen.getByTestId("demand-dossier-header");
    await expect.element(header.getByText("整理验收材料")).toBeVisible();
    await expect.element(header.getByText("待验收")).toBeVisible();
    await expect.element(header.getByText("为验收整理接入材料")).toBeVisible();
    expect(fetchTaskGraph).toHaveBeenCalledWith("demand-latest");

    // 默认视图 = 时间线（叙事优先），图不渲染。
    await expect.element(screen.getByTestId("demand-dossier-timeline")).toBeVisible();
    expect(screen.container.querySelector("[data-testid=flow-graph-canvas]")).toBeNull();
    await expect.element(screen.getByText("任务开始 · 整理材料任务")).toBeVisible();

    // 诚实边界：时间线自证是叙事而非完整审计流水，并给执行轨迹出口。
    await expect
      .element(screen.getByText("协调叙事视图，按关键节点归纳；完整执行事件见", { exact: false }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "执行轨迹" }))
      .toHaveAttribute("href", "/projects/project-1?tab=trace");

    // 待你处理：摘要 + 收件箱深链，不在卷宗内新造审批交互。
    await expect.element(screen.getByTestId("demand-dossier-pending")).toBeVisible();
    await expect.element(screen.getByText("确认执行风险")).toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "去收件箱处理待你处理事项" }))
      .toHaveAttribute("href", "/inbox");

    // 右轨：无交付事实时给诚实空态，而不是空白或假槽位。
    await expect.element(screen.getByTestId("demand-dossier-rail")).toBeVisible();
    await expect.element(screen.getByText("本单尚未形成可展示的交付事实")).toBeVisible();

    // 非目标护栏：卷宗里不得出现任何形态的「继续此任务」。
    expect(screen.container.textContent).not.toContain("继续此任务");
  });

  it("renders the flow graph only in the graph view", async () => {
    const screen = await renderSection({ view: "graph" });

    await expect.element(screen.getByTestId("flow-graph-canvas")).toBeInTheDocument();
    expect(screen.container.querySelector("[data-testid=demand-dossier-timeline]")).toBeNull();
  });

  it("collapses the timeline in inspect density and expands on demand", async () => {
    const manyItems = Array.from({ length: 6 }, (_, index) => ({
      id: `event-${index}`,
      kind: "task_completed",
      occurred_at: "2026-07-25T08:00:00Z",
      severity: "success",
      title: `任务完成 · 任务${index}`
}));
    const screen = await renderSection({
      apiOptions: {
        baseUrl: "http://cp.test",
        fetcher: stubFetcher({
          pending_actions: [],
          signals: { active_task_count: 0, demand_terminal: true, has_open_decisions: false },
          timeline: { items: manyItems, truncated: false }
})
},
    });

    // 终态且无待办 → 巡检态：只露最近 3 条。
    await expect.element(screen.getByText("任务完成 · 任务0")).toBeVisible();
    expect(screen.container.textContent).not.toContain("任务完成 · 任务5");

    await screen.getByTestId("demand-dossier-timeline-expand").click();
    await expect.element(screen.getByText("任务完成 · 任务5")).toBeVisible();
  });

  it("keeps unknown handoff verdicts out of the failure reading", async () => {
    const screen = await renderSection({
      apiOptions: {
        baseUrl: "http://cp.test",
        fetcher: stubFetcher({
          handoff_summary: {
            assessments: [
              {
                deliverables: [],
                project_task_id: "task-1",
                project_task_name: "整理材料任务",
                status: "unknown"
},
            ],
            fulfilled: 0,
            partial: 0,
            unfulfilled: 0,
            unknown: 1
}
})
},
    });

    await expect.element(screen.getByTestId("demand-dossier-handoff-summary")).toBeVisible();
    await expect.element(screen.getByText("暂无声明 1")).toBeVisible();
    await expect
      .element(screen.getByText("「暂无声明」表示该任务没有结构化交付声明，无法判定，并不代表未完成。"))
      .toBeVisible();
  });

  it("orders rail slots by the effective playbook produce kinds", async () => {
    const screen = await renderSection({
      apiOptions: {
        baseUrl: "http://cp.test",
        fetcher: stubFetcher({
          effective_playbook: {
            name: "软件交付",
            produce_kinds: ["branch_ref", "conclusion"],
            source: "project",
            template_key: "software_delivery"
},
          rail: {
            slots: [
              { items: [], kind: "branch_ref", title: "分支" },
              {
                items: [
                  {
                    id: "summary:1",
                    project_task_name: "整理材料任务",
                    state: "info",
                    summary: "已整理完毕",
                    title: "整理材料任务"
},
                ],
                kind: "conclusion",
                title: "结论"
},
            ]
}
})
},
    });

    await expect.element(screen.getByTestId("demand-dossier-slot-branch_ref")).toBeVisible();
    await expect.element(screen.getByTestId("demand-dossier-slot-conclusion")).toBeVisible();
    await expect.element(screen.getByText("已整理完毕")).toBeVisible();
    // 剧本名进单头，用户能看见"这一单按哪套剧本走"。
    await expect.element(screen.getByText("剧本 · 软件交付")).toBeVisible();
  });

  it("shows the pinned exit in the header so scope is visible, not just the playbook", async () => {
    const screen = await renderSection({
      apiOptions: {
        baseUrl: "http://cp.test",
        fetcher: stubFetcher({
          effective_playbook: {
            exit_deliverable: "review_verdict",
            exit_label: "审查通过并合入",
            exit_pending: false,
            name: "软件交付",
            produce_kinds: ["conclusion"],
            source: "project",
            template_key: "software_delivery"
},
})
},
    });

    await expect.element(screen.getByTestId("demand-dossier-exit")).toBeVisible();
    await expect.element(screen.getByText("收口 · 审查通过并合入")).toBeVisible();
  });

  it("marks an unconfirmed plan's exit as proposed instead of stating it as fact", async () => {
    const screen = await renderSection({
      apiOptions: {
        baseUrl: "http://cp.test",
        fetcher: stubFetcher({
          effective_playbook: {
            exit_deliverable: "release_record",
            exit_label: "发布上线",
            exit_pending: true,
            name: "软件交付",
            produce_kinds: ["conclusion"],
            source: "project",
            template_key: "software_delivery"
},
})
},
    });

    await expect.element(screen.getByText("拟收口 · 发布上线")).toBeVisible();
  });

  it("shows the dispatch blocker banner while the latest gate verdict still holds the task", async () => {
    const screen = await renderSection({
      fetchTaskGraph: vi
        .fn()
        .mockImplementation((demandId: string) =>
          Promise.resolve(graphWithGate(demandId, "waiting_human")),
        ),
    });

    await expect.element(screen.getByTestId("demand-dispatch-blocker")).toBeVisible();
    await expect.element(screen.getByText("等待负责人确认")).toBeVisible();
  });

  it("drops the blocker banner once the latest gate verdict passed", async () => {
    const screen = await renderSection({
      fetchTaskGraph: vi
        .fn()
        .mockImplementation((demandId: string) =>
          Promise.resolve(graphWithGate(demandId, "passed")),
        ),
    });

    await expect.element(screen.getByTestId("demand-dossier-timeline")).toBeVisible();
    expect(screen.container.querySelector("[data-testid=demand-dispatch-blocker]")).toBeNull();
  });

  it("drops the blocker banner when the gated task already reached a terminal status", async () => {
    const screen = await renderSection({
      fetchTaskGraph: vi.fn().mockImplementation((demandId: string) => {
        const graph = graphWithGate(demandId, "waiting_human");
        graph.nodes[0].status = "completed";
        return Promise.resolve(graph);
      }),
    });

    await expect.element(screen.getByTestId("demand-dossier-timeline")).toBeVisible();
    expect(screen.container.querySelector("[data-testid=demand-dispatch-blocker]")).toBeNull();
  });

  // 回归：闸门事件按 (任务, 事件类型) 至多发一次，任务重试后二次卡人工不会再产生
  // waiting_human 事件——事件流里最后一条永远是 dispatched。按事件推断会漏报，
  // 按闸门记录判断才能重新亮起横幅。
  it("still raises the banner when the task is gated again after a dispatched event", async () => {
    const screen = await renderSection({
      fetchTaskGraph: vi.fn().mockImplementation((demandId: string) => {
        const graph = graphWithGate(demandId, "waiting_human");
        graph.recent_events = [
          gateEvent("project_task.dispatch_gate.waiting_human", 36),
          gateEvent("project_task.dispatch_gate.checked", 40),
          gateEvent("project_task.dispatched", 41),
        ];
        return Promise.resolve(graph);
      }),
    });

    await expect.element(screen.getByTestId("demand-dispatch-blocker")).toBeVisible();
    await expect.element(screen.getByText("等待负责人确认")).toBeVisible();
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

    const header = screen.getByTestId("demand-dossier-header");
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
      const screen = await renderSection({ fetchTaskGraph, view: "graph" });

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
    await expect.element(screen.getByTestId("demand-dossier-timeline")).toBeVisible();
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

  // 真实 E2E 揪出的漏网：活动 SSE 只 invalidate 了图与 launch-detail，卷宗自己
  // 停在旧值，只能等 30s 兜底轮询才追上。卷宗是这一处所的主读模型，必须在列。
  it("refetches the dossier when the activity SSE pushes an event of this project", async () => {
    const listeners: Record<string, Array<(event: { data: string }) => void>> = {};
    const fakeSource = {
      addEventListener: (type: string, listener: (event: { data: string }) => void) => {
        (listeners[type] ??= []).push(listener);
      },
      close: () => undefined,
      removeEventListener: () => undefined
} as unknown as EventSource;
    const dossierUrls: string[] = [];
    const fetcher: typeof fetch = async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/dossier")) {
        dossierUrls.push(url);
        const demandId = /project-demands\/([^/?]+)\/dossier/.exec(url)?.[1] ?? "";
        return jsonResponse(dossierFor(demandId));
      }
      if (url.includes("/acceptance-criteria")) {
        return jsonResponse({ criteria: [], demand_status: "executing" });
      }
      return jsonResponse({ detail: "not found" }, 404);
    };
    const screen = await renderSection({
      apiOptions: { baseUrl: "http://cp.test", fetcher },
      eventSourceFactory: () => fakeSource
});

    await expect.element(screen.getByTestId("demand-dossier-timeline")).toBeVisible();
    const callsBefore = dossierUrls.length;

    // 非本项目事件被过滤：不触发重取。
    for (const listener of listeners["activity"] ?? []) {
      listener({ data: JSON.stringify({ event_id: "evt-x", project_id: "project-2" }) });
    }
    await new Promise((resolve) => setTimeout(resolve, 150));
    expect(dossierUrls.length).toBe(callsBefore);

    for (const listener of listeners["activity"] ?? []) {
      listener({ data: JSON.stringify({ event_id: "evt-1", project_id: "project-1" }) });
    }
    await expect.poll(() => dossierUrls.length).toBeGreaterThan(callsBefore);
  });
});
