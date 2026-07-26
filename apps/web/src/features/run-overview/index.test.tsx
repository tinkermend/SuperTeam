import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";

let routerSearch: Record<string, string | undefined> = {};

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    params,
    search,
    to
}: {
    children: ReactNode;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
    to: string;
  }) => {
    let href = to;
    if (params) {
      for (const [key, value] of Object.entries(params)) {
        href = href.replace(`$${key}`, encodeURIComponent(value));
      }
    }
    const query = search
      ? `?${new URLSearchParams(Object.entries(search).filter((entry): entry is [string, string] => Boolean(entry[1]))).toString()}`
      : "";
    return <a href={`${href}${query}`}>{children}</a>;
  },
  useSearch: () => routerSearch,
  useNavigate: () => () => Promise.resolve()
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main data-testid="run-overview-main">{children}</main>
}));

vi.mock("@/components/layout/shell-page-header", () => ({
  ShellPageHeader: ({
    actions,
    subtitle,
    title
}: {
    actions?: ReactNode;
    subtitle?: ReactNode;
    title: ReactNode;
  }) => (
    <header data-testid="run-overview-shell-header">
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
      {actions ? <div data-testid="run-overview-shell-header-actions">{actions}</div> : null}
    </header>
  )
}));

import { RunOverviewView } from "@/features/run-overview";
import {
  digitalEmployeeActivityFixture,
  digitalEmployeeOverviewFixture,
  digitalEmployeeOverviewWithUnassignedFixture,
  projectRunSummaryFixture,
  projectTaskGraphFixture,
  teamListFixture
} from "./runtime-overview-fixtures";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status: 200
});
}

const emptyTaskGraph = {
  nodes: [],
  edges: [],
  employees: [],
  runs: [],
  execution_summaries: [],
  recent_events: [],
  decision_requests: [],
  blocking_facts: []
};

function createFetcher({
  withActivity = true,
  withUnassigned = false,
  withSecondDemand = false,
  withGrowingDemands = false,
  withRunSummary = true
}: {
  withActivity?: boolean;
  withUnassigned?: boolean;
  // 第二个（更早的）demand：demand-1 为最新有链需求，demand-0 为无链需求。
  withSecondDemand?: boolean;
  // 首次返回单 demand，后续请求头部插入新 demand（模拟并行需求抢"最新"位）。
  withGrowingDemands?: boolean;
  // 关闭时 run-summary 返回 404，运行带走员工反向聚合降级路径。
  withRunSummary?: boolean;
} = {}) {
  const requests: Array<{ pathname: string; search: string }> = [];
  let demandCalls = 0;
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    requests.push({ pathname: url.pathname, search: url.search });
    if (url.pathname === "/api/v1/digital-employees/overview") {
      return jsonResponse(withUnassigned ? digitalEmployeeOverviewWithUnassignedFixture : digitalEmployeeOverviewFixture);
    }
    if (url.pathname === "/api/v1/digital-employees/activity" && withActivity) {
      return jsonResponse(digitalEmployeeActivityFixture);
    }
    if (url.pathname === "/api/v1/teams") {
      return jsonResponse(teamListFixture);
    }
    if (url.pathname === "/api/v1/projects/run-summary" && withRunSummary) {
      return jsonResponse(projectRunSummaryFixture);
    }
    if (/^\/api\/v1\/projects\/[^/]+\/demands$/.test(url.pathname)) {
      demandCalls += 1;
      if (withGrowingDemands && demandCalls > 1) {
        return jsonResponse([
          { id: "demand-new", title: "半路插入的新需求" },
          { id: "demand-1", title: "链路需求" },
        ]);
      }
      if (withSecondDemand) {
        return jsonResponse([
          { id: "demand-1", title: "链路需求" },
          { id: "demand-0", title: "历史空需求" },
        ]);
      }
      return jsonResponse([{ id: "demand-1", title: "链路需求" }]);
    }
    if (/^\/api\/v1\/projects\/[^/]+\/task-graph$/.test(url.pathname)) {
      const demandId = url.searchParams.get("demand_id");
      return jsonResponse(demandId === "demand-1" ? projectTaskGraphFixture : emptyTaskGraph);
    }
    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), { status: 404 });
  }) as unknown as typeof fetch;
  return { fetcher, requests };
}

function queryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false }
}
});
}

async function renderPage(fetcher: typeof fetch, search: Record<string, string | undefined> = {}) {
  routerSearch = search;
  return await render(
    <QueryClientProvider client={queryClient()}>
      <RunOverviewView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("RunOverviewView", () => {
  afterEach(() => {
    routerSearch = {};
  });

  it("renders the runtime overview map from existing Control Plane read APIs", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("heading", { name: "运行总览" })).toBeVisible();
    await expect.element(screen.getByText("开发团队").first()).toBeVisible();
    await expect.element(screen.getByText("运维团队").first()).toBeVisible();
    await expect.element(screen.getByText("高秀英").first()).toBeVisible();
    await expect.element(screen.getByText("工位 3/3").first()).toBeVisible();
    expect(requests.some((request) => request.pathname === "/api/v1/digital-employees/overview")).toBe(true);
    expect(requests.some((request) => request.pathname === "/api/v1/teams")).toBe(true);
  });

  it("keeps shell header outside the page content and page actions inside the map toolbar", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("heading", { name: "运行总览" })).toBeVisible();
    const main = screen.container.querySelector<HTMLElement>("[data-testid='run-overview-main']");
    const shellHeader = screen.container.querySelector<HTMLElement>("[data-testid='run-overview-shell-header']");
    const shellHeaderActions = screen.container.querySelector<HTMLElement>("[data-testid='run-overview-shell-header-actions']");
    expect(main).not.toBeNull();
    expect(shellHeader).not.toBeNull();
    expect(shellHeaderActions).toBeNull();
    expect(main?.contains(shellHeader)).toBe(false);
    await expect.element(screen.getByRole("button", { name: "1层" })).toBeVisible();
    expect(main?.querySelector("[data-runtime-overview-toolbar]")).not.toBeNull();
    expect(main?.querySelector("button[aria-label='刷新运行总览']")).not.toBeNull();
    await expect.element(screen.getByRole("button", { name: "拖动画布" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "缩小" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "放大" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "适应视图" })).not.toBeInTheDocument();
  });

  it("switches floors without refetching layout data", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("button", { name: "1层" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "2层" }));
    await expect.poll(() => screen.container.querySelector("[data-runtime-map-scene]")?.getAttribute("data-runtime-map-scene")).toBe("floor-2");
    // 楼层信息只由按钮选中态表达，不再有"当前楼层"文案行。
    expect(screen.container.textContent).not.toContain("当前楼层");
    expect(requests.filter((request) => request.pathname === "/api/v1/digital-employees/overview").length).toBe(1);
  });

  it("renders office-zone capacity workspaces and selectable employee avatars", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    expect(screen.container.querySelectorAll("[data-runtime-seat='team-dev']").length).toBe(3);
    expect(screen.container.querySelectorAll("[data-runtime-seat='team-ops']").length).toBe(4);
    expect(screen.container.textContent).not.toMatch(/\+\d+\s*空闲/);
    expect(screen.container.querySelectorAll("[data-runtime-team-callout='team-dev']").length).toBe(1);
    expect(screen.container.querySelectorAll("button[data-employee-id] > span.absolute.right-1.bottom-1.size-3.rounded-full.border-2.border-white").length).toBe(0);
    const calloutLink = screen.container.querySelector("[data-runtime-team-callout-link='team-dev']");
    expect(calloutLink).not.toBeNull();
    expect(calloutLink?.getAttribute("stroke")).toBe("#6482A3");

    await userEvent.click(screen.getByRole("button", { name: /高秀英/ }));
    await expect.element(screen.getByText("当前选择：高秀英")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();
    expect(screen.container.querySelectorAll("[data-runtime-team-selection-frame]").length).toBe(0);
  });

  it("keeps the office map unframed and feathered into the page background", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    const mapStage = screen.container.querySelector<HTMLElement>("[aria-label='运行总览地图画布']");
    const mapScene = screen.container.querySelector<HTMLElement>("[data-runtime-map-scene]");
    const mapFeather = screen.container.querySelector<HTMLElement>("[data-runtime-map-feather]");
    const mapBackground = screen.container.querySelector<HTMLElement>("[data-runtime-map-background]");
    expect(mapStage).not.toBeNull();
    expect(mapScene).not.toBeNull();
    expect(mapFeather).not.toBeNull();
    expect(mapBackground).not.toBeNull();
    expect(mapStage?.className).not.toContain("border-[var(--shell-glass-border)]");
    expect(mapStage?.className).not.toContain("bg-[var(--shell-glass)]");
    expect(mapStage?.className).not.toContain("p-1");
    expect(mapStage?.className).not.toContain("shadow-card");
    expect(mapScene?.style.clipPath).not.toContain("polygon(");
    expect(
      `${mapScene?.style.maskImage ?? ""} ${mapScene?.style.getPropertyValue("-webkit-mask-image")}`,
    ).toContain("linear-gradient");
    expect(
      `${mapFeather?.style.maskImage ?? ""} ${mapFeather?.style.getPropertyValue("-webkit-mask-image")}`,
    ).toContain("linear-gradient");
    expect(mapBackground?.className).toContain("mix-blend-multiply");

    await userEvent.click(screen.getByRole("button", { name: "2层" }));
    expect(screen.container.querySelector<HTMLElement>("[data-runtime-map-scene]")?.style.clipPath).not.toContain("polygon(");
    expect(screen.container.querySelector<HTMLElement>("[data-runtime-map-feather]")?.style.maskImage).toContain("linear-gradient");
    await userEvent.click(screen.getByRole("button", { name: "3层" }));
    expect(screen.container.querySelector<HTMLElement>("[data-runtime-map-scene]")?.style.clipPath).not.toContain("polygon(");
    expect(screen.container.querySelector<HTMLElement>("[data-runtime-map-feather]")?.style.maskImage).toContain("linear-gradient");
  });

  it("renders connector guide lines without center node circles", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    expect(screen.container.querySelectorAll("[aria-label='运行总览地图画布'] svg circle").length).toBe(0);
  });

  it("shows selected employee details without a table view fallback", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await userEvent.click(screen.getByRole("button", { name: /高秀英/ }));
    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();
    await expect.element(screen.getByText("排查线上告警并生成修复计划", { exact: true })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "表格视图" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("table", { name: "运行总览表格" })).not.toBeInTheDocument();
  });

  it("only shows daily cumulative usage in the selected employee consumption panel", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-dev-1" });

    await expect.element(screen.getByText("消耗情况")).toBeVisible();
    await expect.element(screen.getByText("今日累计")).toBeVisible();
    await expect.element(screen.getByText("本任务消耗")).not.toBeInTheDocument();
  });

  it("deep-links to employees and runtime nodes from the selected employee panel", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-dev-1" });

    await expect.element(screen.getByText("当前选择：陆一鸣")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "陆一鸣" })).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "查看员工详情" })).toHaveAttribute(
      "href",
      "/employees/emp-dev-1",
    );
    await expect.element(screen.getByRole("link", { name: "查看 Runtime 节点" })).toHaveAttribute(
      "href",
      "/runtime?node=local-dev-node",
    );
  });

  it("differentiates live avatar status with halo animation only for active states", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    const workingButton = screen.container.querySelector<HTMLElement>("button[data-employee-id='emp-ops-1']");
    expect(workingButton?.getAttribute("data-employee-status")).toBe("working");
    expect(workingButton?.querySelector("[data-status-halo]")).not.toBeNull();
    expect(workingButton?.querySelector("[data-status-halo]")?.className).toContain("animate-ping");
    const idleButton = screen.container.querySelector<HTMLElement>("button[data-employee-id='emp-dev-2']");
    expect(idleButton?.getAttribute("data-employee-status")).toBe("idle");
    expect(idleButton?.querySelector("[data-status-halo]")).toBeNull();
    const waitingButton = screen.container.querySelector<HTMLElement>("button[data-employee-id='emp-ops-2']");
    expect(waitingButton?.querySelector("[data-status-halo]")?.className).toContain("animate-pulse");
  });

  it("shows the current task's project deep link and the linked project count", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-ops-1" });

    await expect.element(screen.getByText("所属项目")).toBeVisible();
    // 侧栏项目透镜选择器里也会出现同名项目，深链断言限定在运行快照卡内。
    await expect.element(screen.getByText("运维团队交付项目").first()).toBeVisible();
    const projectLink = screen.container.querySelector("[data-employee-current-project] a");
    expect(projectLink?.getAttribute("href")).toBe("/projects/emp-ops-1-project");
    await expect.element(screen.getByText("关联 1 个项目")).toBeVisible();
  });

  it("explains the current status with duration and reasons in the snapshot card", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-ops-2" });

    await expect.element(screen.getByRole("heading", { name: "罗明" })).toBeVisible();
    const statusBlock = screen.container.querySelector<HTMLElement>("[data-employee-status-block]");
    expect(statusBlock).not.toBeNull();
    expect(statusBlock?.textContent).toContain("待确认");
    expect(statusBlock?.textContent).toContain("已等待");
    await expect.element(screen.getByText("等待人工确认后继续执行")).toBeVisible();
  });

  it("hides project affordances for employees without linked projects", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-dev-2" });

    await expect.element(screen.getByRole("heading", { name: "沈嘉" })).toBeVisible();
    expect(screen.container.querySelector("[data-employee-current-project]")).toBeNull();
    expect(screen.container.querySelector("[data-employee-project-count]")).toBeNull();
    await expect.element(screen.getByText("暂无进行中的任务")).toBeVisible();
  });

  it("surfaces company-wide project count, today token usage and latest activity", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByText("关联项目", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("今日消耗 tokens")).toBeVisible();
    await expect.element(screen.getByText("最新动态")).toBeVisible();
    // 动态流来自 activity 端点：服务端映射标签 + 任务标题。
    await expect.element(screen.getByText("运行完成 · 排查线上告警并生成修复计划")).toBeVisible();
    const activityItems = screen.container.querySelectorAll("[data-runtime-recent-activity] li");
    expect(activityItems.length).toBe(2);
  });

  it("refreshes activity and overview queries when the SSE stream pushes an event", async () => {
    const { fetcher, requests } = createFetcher();
    const listeners: Record<string, () => void> = {};
    const fakeSource = {
      addEventListener: (type: string, listener: () => void) => {
        listeners[type] = listener;
      },
      removeEventListener: () => {},
      close: () => {}
} as unknown as EventSource;
    const streamUrls: string[] = [];
    routerSearch = {};
    const screen = await render(
      <QueryClientProvider client={queryClient()}>
        <RunOverviewView
          apiBaseUrl="http://control-plane.local"
          fetcher={fetcher}
          eventSourceFactory={(url) => {
            streamUrls.push(url);
            return fakeSource;
          }}
        />
      </QueryClientProvider>,
    );

    // 等两个查询都完成渲染（概况卡 + 动态流内容），再触发推送，确保 invalidate 不会与在途首查询去重。
    await expect.element(screen.getByText("运行概况")).toBeVisible();
    await expect.element(screen.getByText("运行完成 · 排查线上告警并生成修复计划")).toBeVisible();
    expect(streamUrls).toEqual(["http://control-plane.local/api/v1/digital-employees/activity/stream"]);
    const overviewRequests = () =>
      requests.filter((request) => request.pathname === "/api/v1/digital-employees/overview").length;
    const activityRequests = () =>
      requests.filter((request) => request.pathname === "/api/v1/digital-employees/activity").length;
    const overviewBefore = overviewRequests();
    const activityBefore = activityRequests();

    listeners["activity"]?.();
    await expect.poll(() => overviewRequests()).toBeGreaterThan(overviewBefore);
    await expect.poll(() => activityRequests()).toBeGreaterThan(activityBefore);
  });

  it("falls back to overview-derived activity when the activity endpoint is unavailable", async () => {
    const { fetcher } = createFetcher({ withActivity: false });
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByText("最新动态")).toBeVisible();
    const activityItems = screen.container.querySelectorAll("[data-runtime-recent-activity] li");
    expect(activityItems.length).toBeGreaterThan(0);
    expect(activityItems.length).toBeLessThanOrEqual(5);
  });

  it("always places the employee card below the map instead of the rail", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-ops-1" });

    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();
    const card = screen.container.querySelector<HTMLElement>("[data-employee-detail-card]");
    expect(card).not.toBeNull();
    expect(screen.container.querySelectorAll("[data-employee-detail-card]").length).toBe(1);
    // 卡片必须位于地图所在 master 列内（填补地图下方空间），而不是右栏。
    const mapStage = screen.container.querySelector("[aria-label='运行总览地图画布']");
    expect(card?.parentElement?.parentElement?.contains(mapStage)).toBe(true);
    // 当前任务所属项目深链在卡内仍完整可用。
    expect(card?.querySelector("[data-employee-current-project] a")?.getAttribute("href")).toBe("/projects/emp-ops-1-project");
  });

  it("declares container-width driven column steps on the employee card grid", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-ops-1" });

    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();
    const card = screen.container.querySelector<HTMLElement>("[data-employee-detail-card]");
    // 卡片自身是容器查询锚点，内部网格按卡宽升列：窄单列 → 中两列 → 宽三列。
    expect(card?.className).toContain("@container");
    const grid = card?.firstElementChild as HTMLElement;
    expect(grid.className).toContain("grid-cols-1");
    expect(grid.className).toContain("@xl:grid-cols-2");
    expect(grid.className).toContain("@5xl:grid-cols-3");
  });

  it("auto-focuses the most urgent active employee and pauses the carousel on avatar click", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    // 默认无交互：焦点轮播启动，队列 = 非空闲员工（待确认 > 工作中），焦点为最紧迫的罗明。
    await expect.element(screen.getByText(/焦点轮播 1 \/ 3/)).toBeVisible();
    // 轮播指示器位于顶部工具栏右侧。
    expect(screen.container.querySelector("[data-runtime-overview-toolbar] [data-runtime-carousel-indicator]")).not.toBeNull();
    await expect.element(screen.getByRole("heading", { name: "罗明" })).toBeVisible();

    // 点击头像 = 交互暂停，选中对象切换为用户点击的员工。
    await userEvent.click(screen.getByRole("button", { name: /高秀英/ }));
    await expect.element(screen.getByText("轮播已暂停 · 稍后自动恢复")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();

    // 手动恢复按钮回到轮播焦点。
    await userEvent.click(screen.getByRole("button", { name: "恢复轮播" }));
    await expect.element(screen.getByText(/焦点轮播/)).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "罗明" })).toBeVisible();
  });

  it("seats unassigned employees on a dedicated lobby floor with consistent headcount", async () => {
    const { fetcher } = createFetcher({ withUnassigned: true });
    const screen = await renderPage(fetcher);

    // 大厅 tab 仅在有候岗员工时出现，并带人数徽标。
    await expect.element(screen.getByRole("button", { name: /大厅/ })).toBeVisible();
    expect(screen.container.querySelector("[data-runtime-lobby-count]")?.textContent).toContain("1");
    await userEvent.click(screen.getByRole("button", { name: /大厅/ }));
    await expect.poll(() => screen.container.querySelector("[data-runtime-map-scene]")?.getAttribute("data-runtime-map-scene")).toBe("lobby");

    await expect.element(screen.getByRole("heading", { name: "候岗区" })).toBeVisible();
    await expect.element(screen.getByText("待编组 1 名")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: /赵新/ })).toBeVisible();
    expect(screen.container.querySelectorAll("[data-runtime-seat='unassigned']").length).toBe(1);

    // 选中候岗员工：详情卡显示"未归属团队"。
    await userEvent.click(screen.getByRole("button", { name: /赵新/ }));
    await expect.element(screen.getByRole("heading", { name: "赵新" })).toBeVisible();
    await expect.element(screen.getByText("未归属团队")).toBeVisible();
  });

  it("hides the lobby floor when every employee belongs to a team", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: /大厅/ })).not.toBeInTheDocument();
    expect(screen.container.querySelector("[data-runtime-lobby-callout]")).toBeNull();
  });

  it("activates the project lens with highlighted participants and handoff edges", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByText("项目运行", { exact: true })).toBeVisible();
    const option = screen.container.querySelector<HTMLElement>("[data-runtime-lens-project='emp-ops-1-project']");
    expect(option).not.toBeNull();
    await userEvent.click(option as HTMLElement);

    // 任务链路按最新 demand 拉取。
    await expect.poll(() => requests.some((request) => request.pathname.endsWith("/task-graph"))).toBe(true);
    expect(requests.find((request) => request.pathname.endsWith("/task-graph"))?.search).toContain("demand_id=demand-1");

    // 交接连线：完成段→活跃段 primary、待开始段 muted。
    await expect.poll(() => screen.container.querySelectorAll("[data-runtime-lens-edge]").length).toBe(2);
    expect(
      screen.container.querySelector("[data-runtime-lens-edge='emp-dev-1->emp-ops-1']")?.getAttribute("data-runtime-lens-edge-tone"),
    ).toBe("primary");
    expect(
      screen.container.querySelector("[data-runtime-lens-edge='emp-ops-1->emp-dev-2']")?.getAttribute("data-runtime-lens-edge-tone"),
    ).toBe("muted");

    // 参与者高亮/停留脉冲，非参与者降暗。
    expect(screen.container.querySelector("button[data-employee-id='emp-ops-1']")?.getAttribute("data-lens-state")).toBe("stop");
    expect(screen.container.querySelector("button[data-employee-id='emp-dev-1']")?.getAttribute("data-lens-state")).toBe(
      "participant",
    );
    expect(screen.container.querySelector("button[data-employee-id='emp-dev-3']")?.getAttribute("data-lens-state")).toBe("dimmed");

    // 侧栏摘要与轮播互斥。
    await expect.element(screen.getByText("参与 3 人")).toBeVisible();
    await expect.element(screen.getByText("交接 2 段")).toBeVisible();
    await expect.element(screen.getByText("待派发 1")).toBeVisible();
    await expect.element(screen.getByText("项目透镜聚焦中 · 轮播已暂停")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "查看项目详情" })).toBeVisible();
  });

  it("renders resident run-band counts from the run-summary endpoint", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByText("项目运行", { exact: true })).toBeVisible();
    await expect.poll(() => requests.some((request) => request.pathname === "/api/v1/projects/run-summary")).toBe(true);

    // 不选中任何项目,计数徽标常驻可见(权威源 run-summary)。
    const opsRow = screen.container.querySelector<HTMLElement>("[data-runtime-lens-project='emp-ops-1-project']");
    expect(opsRow?.textContent).toContain("运行 1");
    expect(opsRow?.textContent).toContain("失败 1");
    expect(opsRow?.textContent).toContain("待人工 1");
    expect(opsRow?.textContent).toContain("2 人");

    // 全待派发、无参与员工的项目在反向聚合里不可见,权威源下必须出现(盲区回归防线)。
    const unstaffedRow = screen.container.querySelector<HTMLElement>("[data-runtime-lens-project='project-unstaffed']");
    expect(unstaffedRow).not.toBeNull();
    expect(unstaffedRow?.textContent).toContain("待派发 2");
  });

  it("falls back to employee-side aggregation when run-summary is unavailable", async () => {
    const { fetcher } = createFetcher({ withRunSummary: false });
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByText("项目运行", { exact: true })).toBeVisible();
    // 降级源仍能列出有参与员工的项目并进入透镜,但看不到无参与员工的项目。
    const opsRow = screen.container.querySelector<HTMLElement>("[data-runtime-lens-project='emp-ops-1-project']");
    expect(opsRow).not.toBeNull();
    expect(screen.container.querySelector("[data-runtime-lens-project='project-unstaffed']")).toBeNull();
  });

  it("labels the lens with its demand and switches chains via the demand selector", async () => {
    const { fetcher, requests } = createFetcher({ withSecondDemand: true });
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByText("项目运行", { exact: true })).toBeVisible();
    const option = screen.container.querySelector<HTMLElement>("[data-runtime-lens-project='emp-ops-1-project']");
    await userEvent.click(option as HTMLElement);

    // 默认选最新 demand（列表第一个）并显式标注；有第二个 demand 时出现切换器。
    await expect.poll(() => screen.container.querySelectorAll("[data-runtime-lens-edge]").length).toBe(2);
    const demandRow = screen.container.querySelector("[data-runtime-lens-demand]");
    expect(demandRow?.textContent).toContain("链路需求");

    // 切到历史空需求：任务图按所选 demand 重新拉取，链路清空。
    await userEvent.click(screen.getByRole("combobox", { name: "切换需求" }));
    await userEvent.click(screen.getByRole("option", { name: "历史空需求" }));
    await expect.poll(() =>
      requests.some((request) => request.pathname.endsWith("/task-graph") && request.search.includes("demand-0")),
    ).toBe(true);
    await expect.poll(() => screen.container.querySelectorAll("[data-runtime-lens-edge]").length).toBe(0);
    await expect.element(screen.getByText("参与 0 人")).toBeVisible();

    // 切回链路需求：链路恢复。
    await userEvent.click(screen.getByRole("combobox", { name: "切换需求" }));
    await userEvent.click(screen.getByRole("option", { name: "链路需求" }));
    await expect.poll(() => screen.container.querySelectorAll("[data-runtime-lens-edge]").length).toBe(2);
  });

  it("pins the selected demand when a newer demand appears", async () => {
    const { fetcher } = createFetcher({ withGrowingDemands: true });
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByText("项目运行", { exact: true })).toBeVisible();
    const option = screen.container.querySelector<HTMLElement>("[data-runtime-lens-project='emp-ops-1-project']");
    await userEvent.click(option as HTMLElement);
    await expect.poll(() => screen.container.querySelectorAll("[data-runtime-lens-edge]").length).toBe(2);

    // 刷新拉到头部插入的新 demand：当前选中钉住不被抢位，链路不变。
    await userEvent.click(screen.getByRole("button", { name: "刷新" }));
    await expect.poll(() =>
      screen.container.querySelector("[data-runtime-lens-demand]")?.textContent ?? "",
    ).toContain("链路需求");
    expect(screen.container.querySelectorAll("[data-runtime-lens-edge]").length).toBe(2);
    // 新需求进入了切换器选项，但未自动选中。
    await userEvent.click(screen.getByRole("combobox", { name: "切换需求" }));
    await expect.element(screen.getByRole("option", { name: "半路插入的新需求" })).toBeVisible();
  });

  it("clears highlights and resumes the carousel when exiting the lens", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByText("项目运行", { exact: true })).toBeVisible();
    const option = screen.container.querySelector<HTMLElement>("[data-runtime-lens-project='emp-ops-1-project']");
    expect(option).not.toBeNull();
    await userEvent.click(option as HTMLElement);
    await expect.poll(() => screen.container.querySelectorAll("[data-runtime-lens-edge]").length).toBe(2);

    await userEvent.click(screen.getByText("退出透镜"));
    await expect.poll(() => screen.container.querySelectorAll("[data-runtime-lens-edge]").length).toBe(0);
    expect(screen.container.querySelector("button[data-employee-id='emp-dev-3']")?.getAttribute("data-lens-state")).toBeNull();
    await expect.element(screen.getByText(/焦点轮播/)).toBeVisible();
  });

  it("enters the lens from a project deep link", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { project: "emp-ops-1-project" });

    await expect.poll(() => screen.container.querySelectorAll("[data-runtime-lens-edge]").length).toBe(2);
    await expect.element(screen.getByText("退出透镜")).toBeVisible();
    await expect.element(screen.getByText("项目透镜聚焦中 · 轮播已暂停")).toBeVisible();
  });

  it("starts paused when opened with an employee deep link", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-dev-2" });

    await expect.element(screen.getByText("轮播已暂停 · 稍后自动恢复")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "沈嘉" })).toBeVisible();
  });

  it("lets employee query changes override the previous local selection", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-dev-1" });

    await expect.element(screen.getByText("当前选择：陆一鸣")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "陆一鸣" })).toBeVisible();

    routerSearch = { employee: "emp-ops-1" };
    await screen.rerender(
      <QueryClientProvider client={queryClient()}>
        <RunOverviewView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("当前选择：高秀英")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();
  });
});
