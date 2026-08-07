import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { InboxView } from "@/features/inbox";
import { groupInboxItems, readInboxProgress, resolveInboxHref } from "@/features/inbox/components/inbox-item-list";
import type { InboxItem, InboxListResponse } from "@/lib/api/inbox";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a data-router-link="true" href={to}>{children}</a>
  )
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false }
}
});
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status
});
}

function makeInboxItem(overrides: Partial<InboxItem> = {}): InboxItem {
  return {
    actions: [
      {
        key: "approved",
        label: "Approve",
        requires_comment: false,
        tone: "positive"
},
      {
        key: "rejected",
        label: "Reject",
        requires_comment: true,
        tone: "destructive"
},
      {
        key: "needs_more_evidence",
        label: "Request evidence",
        requires_comment: true,
        tone: "warning"
},
    ],
    context: {
      project_name: "客户接入项目",
      source_title: "准入审批"
},
    created_at: "2026-06-12T01:30:00Z",
    deep_link: {
      anchor: "approval-1",
      route: "/projects/project-1/approvals"
},
    id: "inbox-item-1",
    item_type: "approval",
    last_activity_at: "2026-06-12T02:30:00Z",
    priority: "high",
    risk_level: "high",
    source_approval_request_id: "approval-1",
    source_id: "approval-1",
    source_project_id: "project-1",
    source_task_id: "task-1",
    source_type: "approval_request",
    status: "open",
    summary: "需要确认客户侧 Runtime 节点接入证据。",
    target_user_id: "human-owner-1",
    tenant_id: "tenant-1",
    title: "确认客户 Runtime 接入",
    updated_at: "2026-06-12T02:30:00Z",
    ...overrides
};
}

function makeListResponse(items: InboxItem[]): InboxListResponse {
  return {
    items,
    pagination: {
      has_more: false,
      limit: 50,
      offset: 0
},
    summary: {
      blocked_count: 1,
      high_risk_count: 1,
      open_count: items.length
}
};
}

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });

  return { promise, reject, resolve };
}

function createInboxFetcher(
  options: {
    actionDelay?: Promise<void>;
    actionDelays?: Record<string, Promise<void>>;
    actionStatus?: number;
    actionStatuses?: Record<string, number>;
    mineItem?: InboxItem;
    mineItems?: InboxItem[];
    projects?: Array<{ id: string; name: string; status: "running" | "draft" | "archived" }>;
    slowTeamView?: boolean;
    teamItem?: InboxItem;
  } = {},
) {
  const requests: Array<{ body?: string; method: string; pathname: string; url: string }> = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    requests.push({
      body: typeof init?.body === "string" ? init.body : undefined,
      method,
      pathname: url.pathname,
      url: url.toString()
    });

    if (url.pathname === "/api/v1/inbox/items" && method === "GET") {
      const view = url.searchParams.get("view") ?? "mine";

      if (view === "team") {
        if (options.slowTeamView) {
          await new Promise((resolve) => setTimeout(resolve, 120));
        }

        return jsonResponse(
          makeListResponse([
            options.teamItem ??
              makeInboxItem({
                id: "team-inbox-item-1",
                summary: "团队负责人需要确认发布窗口。",
                target_user_id: "human-owner-1",
                title: "团队发布窗口确认"
              }),
          ]),
        );
      }

      return jsonResponse(
        makeListResponse(options.mineItems ?? [options.mineItem ?? makeInboxItem()]),
      );
    }

    if (url.pathname === "/api/v1/projects" && method === "GET") {
      let projects = options.projects ?? [];
      const q = url.searchParams.get("q")?.trim().toLowerCase();
      if (q) {
        projects = projects.filter(
          (project) =>
            project.name.toLowerCase().includes(q) ||
            (project.status ?? "").toLowerCase().includes(q),
        );
      }
      return jsonResponse(projects);
    }

    const projectMatch = url.pathname.match(/^\/api\/v1\/projects\/([^/]+)$/);
    if (projectMatch && method === "GET") {
      const projectId = decodeURIComponent(projectMatch[1]);
      const project = (options.projects ?? []).find((entry) => entry.id === projectId);
      if (project) {
        return jsonResponse(project);
      }
      return jsonResponse({ error: "project not found" }, 404);
    }

    if (url.pathname === "/api/auth/users" && method === "GET") {
      return jsonResponse({ items: [], total: 0 });
    }

    const actionMatch = url.pathname.match(/^\/api\/v1\/inbox\/items\/([^/]+)\/actions$/);
    if (actionMatch && method === "POST") {
      const itemId = decodeURIComponent(actionMatch[1]);
      const delay = options.actionDelays?.[itemId] ?? options.actionDelay;
      if (delay) {
        await delay;
      }
      const status = options.actionStatuses?.[itemId] ?? options.actionStatus;
      if (status && status >= 400) {
        return jsonResponse({ error: "上游审批服务暂时不可用" }, status);
      }

      return jsonResponse({
        item: makeInboxItem({ id: itemId, status: "resolved" }),
        source_result: {
          source_id: "approval-1",
          source_type: "approval_request",
          status: "approved"
        }
      });
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: { "content-type": "application/json" },
      status: 404
    });
  }) as unknown as typeof fetch & { requests: typeof requests };

  Object.assign(fetcher, { requests });
  return fetcher;
}

function actionRequestBodies(fetcher: typeof fetch) {
  return (
    fetcher as unknown as {
      requests: Array<{ body?: string; method: string; pathname: string }>;
    }
  ).requests.filter(
    (request) => request.method === "POST" && request.pathname.includes("/actions"),
  );
}

function inboxRequestUrls(fetcher: typeof fetch) {
  return (
    fetcher as unknown as {
      requests: Array<{ method: string; pathname: string; url: string }>;
    }
  ).requests
    .filter((request) => request.method === "GET" && request.pathname === "/api/v1/inbox/items")
    .map((request) => new URL(request.url));
}

function latestInboxRequestUrl(fetcher: typeof fetch) {
  const urls = inboxRequestUrls(fetcher);
  return urls[urls.length - 1];
}

async function renderInboxView(fetcher = createInboxFetcher()) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <InboxView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("InboxView", () => {
  it("builds project approval focus links when no safe deep link route is provided", () => {
    expect(
      resolveInboxHref(
        makeInboxItem({
          deep_link: {},
          item_type: "project_decision",
          source_id: "decision-1",
          source_project_id: "project-1"
}),
      ),
    ).toBe("/projects/project-1?tab=approval&focus=decision-1");
  });

  it("normalizes project decision deep links to the project approval tab and focus query", () => {
    expect(
      resolveInboxHref(
        makeInboxItem({
          deep_link: {
            anchor: "decision-1",
            route: "/projects/project-1"
},
          item_type: "project_decision",
          source_id: "decision-1",
          source_project_id: "project-1"
}),
      ),
    ).toBe("/projects/project-1?tab=approval&focus=decision-1");
  });

  it("renders mine inbox by default", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByRole("heading", { name: "收件箱" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "我的待办", selected: true })).toBeVisible();
    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
  });

  it("keeps the list full-width until a human decision item is selected", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    // 布局宪法：未选中不渲染详情/空态占位栏
    expect(screen.getByRole("heading", { name: "选择一条事项查看详情" }).query()).toBeNull();
    expect(screen.getByText("今日待处理摘要").query()).toBeNull();
    expect(screen.getByText("选择事项后可执行").query()).toBeNull();
    expect(screen.getByText("过程记录").query()).toBeNull();
    // 详情栏动作未展开；高风险 open 卡可有行内 CTA
    expect(screen.getByRole("button", { name: "驳回" }).query()).toBeNull();
    await expect.element(screen.getByRole("button", { name: "行内决策：同意" })).toBeVisible();
  });

  it("opens item details with process records, evidence, actions, and flow links", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "打开事项：确认客户 Runtime 接入" }));

    await expect.element(screen.getByRole("heading", { name: "确认客户 Runtime 接入" })).toBeVisible();
    await expect.element(screen.getByText("过程记录")).toBeVisible();
    await expect.element(screen.getByText("关联引用")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "同意" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "驳回" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "要求补证" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Approve" }).query()).toBeNull();
    expect(screen.getByRole("button", { name: "Reject" }).query()).toBeNull();
    expect(screen.getByRole("button", { name: "Request evidence" }).query()).toBeNull();
    await expect.element(screen.getByRole("link", { name: "查看完整详情" })).toHaveAttribute(
      "href",
      "/projects/project-1/approvals#approval-1",
    );
    // F3(§5.4.3): 原"进入流程实例/查看流程编排"两个各自推导入口已下线,只保留服务端
    // primary_surface 的唯一权威落点。
    expect(screen.getByRole("link", { name: "进入流程实例" }).query()).toBeNull();
    expect(screen.getByRole("link", { name: "查看流程编排" }).query()).toBeNull();
  });

  // 已处理事项(如经飞书批准)在"已处理"过滤下选中:详情必须呈终态,不得再渲染
  // "待我处理"徽标与可执行按钮(回归:详情面板曾写死待办态)。
  it("renders resolved items as settled: no pending badge, no action buttons", async () => {
    const resolvedItem = makeInboxItem({
      status: "resolved",
      resolved_at: "2026-07-19T05:05:00Z"
});
    const screen = await renderInboxView(createInboxFetcher({ mineItem: resolvedItem }));

    await userEvent.click(screen.getByText("需要确认客户侧 Runtime 节点接入证据。"));
    await expect.element(screen.getByRole("heading", { name: "确认客户 Runtime 接入" })).toBeVisible();

    await expect.element(screen.getByText("已处理", { exact: true }).first()).toBeVisible();
    expect(screen.getByText("待我处理").query()).toBeNull();
    expect(screen.getByRole("button", { name: "同意" }).query()).toBeNull();
    expect(screen.getByRole("button", { name: "驳回" }).query()).toBeNull();
    await expect.element(screen.getByText("处理耗时").first()).toBeVisible();
    await expect.element(screen.getByText(/该事项已处理/)).toBeVisible();
    expect(screen.getByText(/进行中/).query()).toBeNull();
  });

  // 终态 snapshot：过程记录与动作面板展示 who / channel / verb，且无「待你」进度。
  it("renders resolution snapshot (who / channel / verb) on terminal items", async () => {
    const resolvedItem = makeInboxItem({
      status: "resolved",
      resolved_at: "2026-08-03T05:05:00Z",
      actions: [],
      progress: {
        step: 1,
        total: 4,
        label: "计划确认 已过（已批准） → 执行 待开始 → 验收 未开始 → 结项 未开始"
      },
      context: {
        project_name: "客户接入项目",
        source_title: "准入审批",
        progress: {
          step: 1,
          total: 4,
          label: "计划确认 已过（已批准） → 执行 待开始 → 验收 未开始 → 结项 未开始"
        },
        resolution: {
          decision: "approved",
          decision_label: "批准",
          resolved_by_name: "开发管理员",
          channel: "feishu",
          channel_label: "飞书",
          comment: "联调通过"
        }
      }
    });
    const screen = await renderInboxView(createInboxFetcher({ mineItem: resolvedItem }));

    await userEvent.click(screen.getByText("需要确认客户侧 Runtime 节点接入证据。"));
    await expect.element(screen.getByRole("heading", { name: "确认客户 Runtime 接入" })).toBeVisible();

    await expect.element(screen.getByText("已批准").first()).toBeVisible();
    await expect.element(screen.getByText("开发管理员").first()).toBeVisible();
    await expect.element(screen.getByText("飞书").first()).toBeVisible();
    await expect
      .element(screen.getByText("开发管理员 经 飞书 已批准。 备注：联调通过", { exact: true }))
      .toBeVisible();
    await expect
      .element(screen.getByText("已由 开发管理员 经 飞书 已批准，无需再操作。", { exact: true }))
      .toBeVisible();
    expect(screen.getByText(/待你/).query()).toBeNull();
    expect(screen.getByRole("button", { name: "同意" }).query()).toBeNull();
  });

  it("opens item details when clicking the pending item row body", async () => {
    const screen = await renderInboxView();

    // 未选中：列表独占，无常驻空态详情栏
    expect(screen.getByRole("heading", { name: "选择一条事项查看详情" }).query()).toBeNull();
    await userEvent.click(screen.getByText("需要确认客户侧 Runtime 节点接入证据。"));

    await expect.element(screen.getByRole("heading", { name: "确认客户 Runtime 接入" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "同意" })).toBeVisible();
  });

  it("renders inbox items in a compact list within a work surface", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();

    expect(document.body.querySelector('[data-slot="work-surface"]')).not.toBeNull();
    const itemRows = document.body.querySelectorAll('[role="button"][aria-label^="打开事项"]');
    expect(itemRows.length).toBeGreaterThanOrEqual(1);
  });

  it("renders the inbox with v3 Soft-Flat containers", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();

    expect(document.body.querySelector('[data-slot="page-header"] [data-slot="icon-tile"]')).not.toBeNull();
    // KPI 走 MetricCard（data-slot=metric-card）；工具条/列表用 SoftCard / WorkSurface
    expect(document.body.querySelectorAll('[data-slot="metric-card"]').length).toBeGreaterThanOrEqual(4);
    expect(document.body.querySelectorAll('[data-slot="soft-card"]').length).toBeGreaterThanOrEqual(1);
    expect(document.body.querySelector('[data-slot="work-surface"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="page-tabs"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="status-pill"]')).not.toBeNull();
  });

  it("keeps summary metrics in cards instead of repeating them in the page header", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();

    const pageHeader = document.body.querySelector('[data-slot="page-header"]');
    expect(pageHeader).not.toBeNull();
    expect(pageHeader?.textContent).not.toContain("开放 1");
    expect(pageHeader?.textContent).not.toContain("高风险 1");
    expect(pageHeader?.textContent).not.toContain("阻断 1");
    const metricLabels = Array.from(document.body.querySelectorAll('[data-slot="metric-card"] p'))
      .map((node) => node.textContent?.trim())
      .filter(Boolean);
    expect(metricLabels).toContain("开放事项");
    expect(metricLabels).toContain("高风险");
    expect(metricLabels).toContain("阻断");
  });

  it("renders read-only inbox items when API returns null actions", async () => {
    const itemWithNullActions = makeInboxItem({
      actions: null as unknown as InboxItem["actions"],
      title: "验收 · 帮我分析 Claude Code"
});
    const screen = await renderInboxView(createInboxFetcher({ mineItem: itemWithNullActions }));

    await expect.element(screen.getByText("验收 · 帮我分析 Claude Code")).toBeVisible();
    expect(screen.getByRole("button", { name: "同意" }).query()).toBeNull();
    expect(screen.getByRole("button", { name: "驳回" }).query()).toBeNull();
  });

  it("tags terminal demand status and leads with the server primary demand on closure cards", async () => {
    const completedDemandId = "6cfc23eb-aaaa-2222-3333-444444444444";
    const cancelledDemandId = "6cfc23eb-bbbb-2222-3333-444444444444";
    const closureItem = makeInboxItem({
      context: {
        decision_type: "project_acceptance",
        // 服务端按 updated_at 倒序,刚被取消的需求排在前面。
        demands: [
          { id: cancelledDemandId, status: "cancelled", title: "遗留 E2E 夹具需求" },
          {
            id: completedDemandId,
            status: "completed",
            task_titles: ["分析 CPU 使用率及高占用进程"],
            title: "分析 CPU 使用率"
},
        ],
        kind: "closure_confirm",
        primary_demand_id: completedDemandId,
        project_name: "测试项目"
},
      item_type: "project_decision",
      source_approval_request_id: undefined,
      source_project_name: "测试项目",
      source_task_id: undefined,
      source_type: "project_decision_request",
      title: "结项确认 · 测试项目"
});
    const screen = await renderInboxView(createInboxFetcher({ mineItem: closureItem }));

    await expect.element(screen.getByText("结项确认 · 测试项目")).toBeVisible();
    // headline 取 primary(已完成)需求,而不是列表首位的已取消需求。
    await expect.element(screen.getByText(/分析 CPU 使用率 等 2 项/).first()).toBeVisible();
    expect(screen.getByText(/遗留 E2E 夹具需求 等 2 项/).query()).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "打开事项：结项确认 · 测试项目" }));
    await expect.element(screen.getByText("关联需求 · 分析 CPU 使用率（已完成）")).toBeVisible();
    await expect.element(screen.getByText("关联需求 · 遗留 E2E 夹具需求（已取消）")).toBeVisible();
  });

  it("lists demand and task refs before project for project_acceptance cards", async () => {
    const demandId = "6cfc23eb-1111-2222-3333-444444444444";
    const acceptanceItem = makeInboxItem({
      context: {
        decision_type: "project_acceptance",
        demands: [
          {
            id: demandId,
            status: "completed",
            task_titles: ["分析服务器中的Claude Code配置合理性"],
            title: "帮我分析 Claude Code"
},
        ],
        primary_demand_id: demandId,
        project_name: "测试项目"
},
      deep_link: {
        anchor: "decision-1",
        route: `/workflows/${demandId}`
},
      item_type: "project_decision",
      source_approval_request_id: undefined,
      source_project_name: "测试项目",
      source_task_id: undefined,
      source_type: "project_decision_request",
      title: "验收 · 帮我分析 Claude Code"
});
    const screen = await renderInboxView(createInboxFetcher({ mineItem: acceptanceItem }));

    await expect.element(screen.getByText("验收 · 帮我分析 Claude Code")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "打开事项：验收 · 帮我分析 Claude Code" }));

    await expect.element(screen.getByText("关联需求 · 帮我分析 Claude Code")).toBeVisible();
    await expect
      .element(screen.getByText("关联任务 · 分析服务器中的Claude Code配置合理性"))
      .toBeVisible();
    await expect.element(screen.getByText("关联项目 · 测试项目")).toBeVisible();
    // 一单卷宗 canonical 落点：事项自带 source_project_id 时直接指项目详情需求处所，
    // 不再多绕 /workflows/{id} 那一跳。
    await expect
      .element(screen.getByRole("link", { name: /关联需求 · 帮我分析 Claude Code/ }))
      .toHaveAttribute("href", `/projects/project-1?demand=${demandId}&tab=demands`);
    // 列表 meta 与详情「关联对象」同文案，取详情区内可见的那份即可。
    await expect
      .element(
        screen
          .getByText("帮我分析 Claude Code（任务：分析服务器中的Claude Code配置合理性）", {
            exact: true,
          })
          .first(),
      )
      .toBeVisible();
    await expect.element(screen.getByText("项目 · 测试项目", { exact: true }).first()).toBeVisible();
  });

  // 兜底不能丢：旧数据/飞书历史卡片没有项目身份时仍走 /workflows/{id} 重定向壳。
  it("falls back to the /workflows redirect when the item carries no project id", async () => {
    const demandId = "6cfc23eb-5555-6666-7777-888888888888";
    const item = makeInboxItem({
      context: {
        decision_type: "project_acceptance",
        demands: [{ id: demandId, status: "completed", task_titles: [], title: "无项目身份需求" }],
        primary_demand_id: demandId
},
      item_type: "project_decision",
      source_approval_request_id: undefined,
      source_project_id: undefined,
      source_task_id: undefined,
      source_type: "project_decision_request",
      title: "验收 · 无项目身份需求"
});
    const screen = await renderInboxView(createInboxFetcher({ mineItem: item }));

    await userEvent.click(screen.getByRole("button", { name: "打开事项：验收 · 无项目身份需求" }));
    await expect
      .element(screen.getByRole("link", { name: /关联需求 · 无项目身份需求/ }))
      .toHaveAttribute("href", `/workflows/${demandId}`);
  });

  it("requests open inbox items by default", async () => {
    // 默认 status=open:服务端无 status 过滤会返回 resolved/cancelled 项,
    // "待处理事项"会把已处理旧项继续标成待处理(外部渠道 resolve 后也不消失)。
    const fetcher = createInboxFetcher();
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();

    const requestUrl = latestInboxRequestUrl(fetcher);
    expect(requestUrl?.searchParams.get("status")).toBe("open");
    expect(requestUrl?.searchParams.get("limit")).toBe("50");
    expect(requestUrl?.searchParams.get("offset")).toBe("0");
  });

  it("offers backend-supported status filters including all", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "状态" }));

    await expect.element(screen.getByRole("button", { name: "开放" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "已处理" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "已取消" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "所有" })).toBeVisible();
  });

  it("requests selected status, type, and risk filters", async () => {
    const fetcher = createInboxFetcher();
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "状态" }));
    await userEvent.click(screen.getByRole("button", { name: "已处理" }));
    await userEvent.click(screen.getByRole("button", { name: "事项类型" }));
    await userEvent.click(screen.getByRole("button", { name: "项目决策" }));
    await userEvent.click(screen.getByRole("button", { name: "风险等级" }));
    await userEvent.click(screen.getByRole("button", { name: "高风险" }));

    await vi.waitFor(() => {
      expect(
        inboxRequestUrls(fetcher).some((url) => {
          return (
            url.searchParams.get("status") === "resolved" &&
            url.searchParams.get("item_type") === "project_decision" &&
            url.searchParams.get("risk_level") === "high" &&
            url.searchParams.get("offset") === "0"
          );
        }),
      ).toBe(true);
    });
  });

  it("filters by project name picker without UUID inputs", async () => {
    const projectId = "11111111-1111-4111-8111-111111111111";
    const fetcher = createInboxFetcher({
      projects: [{ id: projectId, name: "客户接入项目", status: "running" }],
    });
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "更多筛选" }));
    // 无手输 UUID 字段
    expect(screen.getByLabelText("项目 ID").query()).toBeNull();
    expect(screen.getByLabelText("目标用户 ID").query()).toBeNull();
    // 我的待办下无目标用户筛选
    expect(screen.getByRole("button", { name: "筛选目标用户" }).query()).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "筛选项目" }));
    await userEvent.click(screen.getByRole("option", { name: "客户接入项目" }));

    await vi.waitFor(() => {
      const requestUrl = latestInboxRequestUrl(fetcher);
      expect(requestUrl?.searchParams.get("project_id")).toBe(projectId);
      expect(requestUrl?.searchParams.get("offset")).toBe("0");
    });

    await userEvent.click(screen.getByRole("button", { name: "重置" }));

    await vi.waitFor(() => {
      const requestUrl = latestInboxRequestUrl(fetcher);
      expect(requestUrl?.searchParams.has("project_id")).toBe(false);
      expect(requestUrl?.searchParams.get("status")).toBe("open");
      expect(requestUrl?.searchParams.get("offset")).toBe("0");
    });
    await expect.element(screen.getByRole("button", { name: "状态" })).toHaveTextContent(
      "开放",
    );
  });

  it("shows target user filter only in team view", async () => {
    const screen = await renderInboxView();

    await userEvent.click(screen.getByRole("button", { name: "更多筛选" }));
    expect(screen.getByRole("button", { name: "筛选目标用户" }).query()).toBeNull();

    await userEvent.click(screen.getByRole("tab", { name: "团队待办" }));
    // 更多筛选已展开时保持展开；切视图不自动收起
    await expect.element(screen.getByRole("button", { name: "筛选目标用户" })).toBeVisible();
  });

  it("opens the decision dialog from the high-risk inline CTA without submitting", async () => {
    const fetcher = createInboxFetcher();
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "行内决策：同意" }));

    await expect.element(screen.getByRole("dialog")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "同意" })).toBeVisible();
    // 仅打开弹窗，未提交
    expect(
      actionRequestBodies(fetcher).length,
    ).toBe(0);
  });

  it("refetches the inbox list when the SSE stream pushes inbox-changed", async () => {
    // SSE 已提升到已登录布局；本用例验证列表查询键仍可被脏通知 invalidate 触发重拉。
    const fetcher = createInboxFetcher();
    const queryClient = createQueryClient();
    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <InboxView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    const before = inboxRequestUrls(fetcher).length;

    await queryClient.invalidateQueries({ queryKey: ["inbox-items"] });
    await expect.poll(() => inboxRequestUrls(fetcher).length).toBeGreaterThan(before);
  });

  it("refetches inbox items when the explicit refresh button is clicked", async () => {
    const fetcher = createInboxFetcher();
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    const before = inboxRequestUrls(fetcher).length;

    await userEvent.click(screen.getByRole("button", { name: "刷新收件箱" }));
    await expect.poll(() => inboxRequestUrls(fetcher).length).toBeGreaterThan(before);
    await expect.element(screen.getByRole("status")).toBeVisible();
  });

  it("keeps existing data while switching to team inbox", async () => {
    const fetcher = createInboxFetcher({ slowTeamView: true });
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("tab", { name: "团队待办" }));

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await expect.element(screen.getByText("团队发布窗口确认")).toBeVisible();
  });

  it("hides action buttons in team view and still shows context link", async () => {
    const screen = await renderInboxView();

    await userEvent.click(screen.getByRole("tab", { name: "团队待办" }));

    await expect.element(screen.getByText("团队发布窗口确认")).toBeVisible();
    expect(screen.getByRole("button", { name: "同意" }).query()).toBeNull();
    expect(screen.getByRole("button", { name: "驳回" }).query()).toBeNull();
    const contextLink = screen.getByRole("link", { name: "查看上下文" });
    await expect.element(contextLink).toHaveAttribute("href", "/projects/project-1/approvals#approval-1");
    await expect.element(contextLink).toHaveAttribute("data-router-link", "true");
  });

  it("falls back to the project link when deep_link route is unsafe", async () => {
    const unsafeItem = makeInboxItem({
      deep_link: {
        anchor: "approval-1",
        route: "//evil.example/path"
},
      source_project_id: "safe-project"
});
    const screen = await renderInboxView(createInboxFetcher({ mineItem: unsafeItem }));

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    const contextLink = screen.getByRole("link", { name: "查看上下文" });
    await expect.element(contextLink).toHaveAttribute("href", "/projects/safe-project#approval-1");
    await expect.element(contextLink).toHaveAttribute("data-router-link", "true");
  });

  // F3(§5.4.3) + 2026-07-25 §4: primary_surface 具名字段优先，deep_link 兼容。
  it("prefers the server-computed primary_surface as the single deep link", async () => {
    const item = makeInboxItem({
      primary_surface: "/workflows/demand-top",
      deep_link: {
        anchor: "approval-1",
        route: "/projects/project-1",
        primary_surface: "/workflows/demand-xyz"
},
      source_project_id: "project-1"
});
    const screen = await renderInboxView(createInboxFetcher({ mineItem: item }));

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    const contextLink = screen.getByRole("link", { name: "查看上下文" });
    await expect.element(contextLink).toHaveAttribute("href", "/workflows/demand-top");
    await expect.element(contextLink).toHaveAttribute("data-router-link", "true");
  });

  it("shows failed action submissions inside the dialog without closing it", async () => {
    const screen = await renderInboxView(createInboxFetcher({ actionStatus: 500 }));

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "打开事项：确认客户 Runtime 接入" }));
    await userEvent.click(screen.getByRole("button", { name: "同意" }));
    await userEvent.click(screen.getByRole("button", { name: "提交" }));

    await expect.element(screen.getByRole("dialog")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "同意" })).toBeVisible();
    const dialog = screen.getByRole("dialog");
    await expect.element(dialog.getByText("风险提示")).toBeVisible();
    await expect.element(dialog.getByText("高风险")).toBeVisible();
    await expect.element(dialog.getByText("所属项目")).toBeVisible();
    await expect.element(dialog.getByText("相关任务")).toBeVisible();
    await expect.element(dialog.getByText("未命名任务 (task…)")).toBeVisible();
    await expect.element(dialog.getByText("技术详情")).toBeVisible();
    await expect.element(dialog.getByText("上游审批服务暂时不可用")).toBeVisible();
  });

  it("frames project acceptance approve dialog around conclusion and consequences", async () => {
    const demandId = "6cfc23eb-3e1a-471a-adb3-c303d14e4cbe";
    const acceptanceItem = makeInboxItem({
      context: {
        decision_type: "project_acceptance",
        demands: [
          {
            id: demandId,
            status: "completed",
            task_titles: ["分析服务器中的Claude Code配置合理性"],
            title: "帮我分析一下当前服务器中的 claude code 配置是否合理"
},
        ],
        primary_demand_id: demandId,
        project_id: "9a9418d6-f1cd-4ca3-94f6-83c16f0d7fb4",
        project_name: "测试项目"
},
      // F3(§5.4.3): 服务端为 demand 关联决策下发的唯一权威落点。
      deep_link: {
        anchor: "approval-1",
        primary_surface: `/workflows/${demandId}`,
        route: `/workflows/${demandId}`
},
      item_type: "project_decision",
      source_project_name: "测试项目",
      source_task_id: undefined,
      source_type: "project_decision_request",
      summary:
        "项目「测试项目」· 需求「帮我分析一下当前服务器中的 claude code 配置是否合理」（含任务：分析服务器中的Claude Code配置合理性）已完成，请确认项目验收",
      title: "验收 · 帮我分析一下当前服务器中的 claude code 配置是否合理"
});
    const screen = await renderInboxView(createInboxFetcher({ mineItem: acceptanceItem }));

    await expect
      .element(screen.getByText("验收 · 帮我分析一下当前服务器中的 claude code 配置是否合理"))
      .toBeVisible();
    await userEvent.click(
      screen.getByRole("button", {
        name: "打开事项：验收 · 帮我分析一下当前服务器中的 claude code 配置是否合理"
}),
    );
    await userEvent.click(screen.getByRole("button", { name: "同意" }));

    const dialog = screen.getByRole("dialog");
    await expect.element(dialog.getByText("你在确认：项目整体可关闭")).toBeVisible();
    await expect.element(dialog.getByText(/本次「同意」后：/)).toBeVisible();
    await expect.element(dialog.getByText(/项目将归档关闭/)).toBeVisible();
    await expect.element(dialog.getByText("触发需求")).toBeVisible();
    await expect.element(dialog.getByText("相关任务")).toBeVisible();
    await expect
      .element(dialog.getByText("分析服务器中的Claude Code配置合理性"))
      .toBeVisible();
    await expect.element(dialog.getByText("所属项目")).toBeVisible();
    await expect.element(dialog.getByText("测试项目")).toBeVisible();
    await expect.element(dialog.getByText(/项目级终态决策/)).toBeVisible();
    await expect.element(dialog.getByRole("link", { name: "查看需求流程与产出" })).toHaveAttribute(
      "href",
      `/workflows/${demandId}`,
    );
    // 长摘要与裸 decision_type 不应再堆在主区
    expect(dialog.getByText(/请确认项目验收/).query()).toBeNull();
    expect(dialog.getByText("project_acceptance").query()).toBeNull();
    await expect.element(dialog.getByText("技术详情")).toBeVisible();
  });

  // 修复回归:此前页面级单飞锁会把第二条事项的提交静默吞掉(弹窗永远"提交中",
  // 再被第一条的成功回调顺带关闭)。现在提交按事项并行,第二条必须真实发出。
  it("submits actions for different items concurrently without dropping the second", async () => {
    const deferred = createDeferred<void>();
    const itemA = makeInboxItem();
    const itemB = makeInboxItem({
      id: "inbox-item-2",
      source_id: "approval-2",
      summary: "第二条待批事项。",
      title: "确认二号任务"
});
    const fetcher = createInboxFetcher({
      actionDelays: { "inbox-item-1": deferred.promise },
      mineItems: [itemA, itemB]
});
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "打开事项：确认客户 Runtime 接入" }));
    await userEvent.click(screen.getByRole("button", { name: "同意" }));
    await userEvent.click(screen.getByRole("button", { name: "提交" }));

    // 第一条仍在提交:弹窗可直接关闭,提交在后台继续。
    await userEvent.click(screen.getByRole("button", { name: "关闭" }));
    // 窄容器下详情为 Sheet，先关抽屉再选下一项（避免遮罩挡住列表）。
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "打开事项：确认二号任务" }));
    await userEvent.click(screen.getByRole("button", { name: "同意" }));
    await userEvent.click(screen.getByRole("button", { name: "提交" }));

    await vi.waitFor(() => {
      const posts = fetcher.requests
        .filter((request) => request.method === "POST")
        .map((request) => request.pathname);
      expect(posts).toContain("/api/v1/inbox/items/inbox-item-2/actions");
      expect(posts).toContain("/api/v1/inbox/items/inbox-item-1/actions");
    });
    deferred.resolve();
  });

  it("surfaces background submission failures after the dialog was closed", async () => {
    const deferred = createDeferred<void>();
    const fetcher = createInboxFetcher({ actionDelay: deferred.promise, actionStatus: 500 });
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "打开事项：确认客户 Runtime 接入" }));
    await userEvent.click(screen.getByRole("button", { name: "同意" }));
    await userEvent.click(screen.getByRole("button", { name: "提交" }));
    await userEvent.click(screen.getByRole("button", { name: "关闭" }));
    deferred.resolve();

    // 弹窗已关:失败不静默,升级到页面横幅。
    await expect.element(screen.getByText("操作未完成")).toBeVisible();
    await expect.element(screen.getByText("上游审批服务暂时不可用")).toBeVisible();
  });

  it("guards rapid duplicate action submissions", async () => {
    const deferred = createDeferred<void>();
    const fetcher = createInboxFetcher({ actionDelay: deferred.promise });
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "打开事项：确认客户 Runtime 接入" }));
    await userEvent.click(screen.getByRole("button", { name: "同意" }));

    // 第二次点击不能用 userEvent：首次点击后按钮会禁用并改名「提交中」，userEvent
    // 的自动等待会一直等一个不复存在的 enabled「提交」按钮直到 15s 超时（此前全量跑
    // 偶发失败的根因——两次点击与 React 重渲染抢时序）。改为拿到 DOM 节点后同步派发
    // 两次原生 click：若守卫已禁用按钮则第二次天然 no-op，若尚未重渲染则由提交处理
    // 器去重——两条路径都必须只产生一次 POST，这正是本用例要钉住的行为。
    const submitButton = screen
      .getByRole("button", { name: "提交" })
      .element() as HTMLButtonElement;
    submitButton.click();
    submitButton.click();
    await expect.element(screen.getByRole("button", { name: "提交中" })).toBeVisible();

    expect(fetcher.requests.filter((request) => request.method === "POST")).toHaveLength(1);
    expect(JSON.parse(fetcher.requests.find((request) => request.method === "POST")?.body ?? "{}")).toMatchObject({
      action: "approved"
});
    deferred.resolve();
  });
});

describe("groupInboxItems (§6.1 领域分组)", () => {
  it("orders sections by human-task category and buckets others into 异常处理", () => {
    const mk = (id: string, kind?: string) => makeInboxItem({ id, source_id: id, kind });
    const items = [
      mk("a", "closure_confirm"),
      mk("b", "plan_review"),
      mk("c", "acceptance_sign"),
      mk("d", "task_failure_recovery"),
      mk("e"),
      mk("f", "dispatch_release"),
    ];

    const sections = groupInboxItems(items);

    expect(sections.map((section) => section.key)).toEqual([
      "plan_review",
      "dispatch_release",
      "acceptance_sign",
      "closure_confirm",
      "exception",
    ]);
    const exception = sections.find((section) => section.key === "exception");
    expect(exception?.label).toBe("异常处理");
    // task_failure_recovery and the kind-less item both fall into 异常处理, in入参顺序.
    expect(exception?.items.map((item) => item.id)).toEqual(["d", "e"]);
    expect(sections.find((section) => section.key === "plan_review")?.label).toBe("计划确认");
  });
});

describe("readInboxProgress (§6.1 闭环进度)", () => {
  it("prefers top-level progress and falls back to context.progress", () => {
    expect(
      readInboxProgress(
        makeInboxItem({
          progress: { step: 3, total: 4, label: "验收签署 待你" }
}),
      ),
    ).toEqual({ step: 3, total: 4, label: "验收签署 待你" });

    expect(
      readInboxProgress(
        makeInboxItem({
          context: { progress: { step: 4, total: 4, label: "结项确认 待你" } }
}),
      ),
    ).toEqual({ step: 4, total: 4, label: "结项确认 待你" });

    expect(readInboxProgress(makeInboxItem({}))).toBeNull();
  });
});
