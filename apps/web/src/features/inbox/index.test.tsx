import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { InboxView } from "@/features/inbox";
import { resolveInboxHref } from "@/features/inbox/components/inbox-item-list";
import type { InboxItem, InboxListResponse } from "@/lib/api/inbox";

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

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a data-router-link="true" href={to}>{children}</a>
  ),
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function makeInboxItem(overrides: Partial<InboxItem> = {}): InboxItem {
  return {
    actions: [
      {
        key: "approved",
        label: "Approve",
        requires_comment: false,
        tone: "positive",
      },
      {
        key: "rejected",
        label: "Reject",
        requires_comment: true,
        tone: "destructive",
      },
      {
        key: "needs_more_evidence",
        label: "Request evidence",
        requires_comment: true,
        tone: "warning",
      },
    ],
    context: {
      project_name: "客户接入项目",
      source_title: "准入审批",
    },
    created_at: "2026-06-12T01:30:00Z",
    deep_link: {
      anchor: "approval-1",
      route: "/projects/project-1/approvals",
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
    ...overrides,
  };
}

function makeListResponse(items: InboxItem[]): InboxListResponse {
  return {
    items,
    pagination: {
      has_more: false,
      limit: 50,
      offset: 0,
    },
    summary: {
      blocked_count: 1,
      high_risk_count: 1,
      open_count: items.length,
    },
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
      url: url.toString(),
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
                title: "团队发布窗口确认",
              }),
          ]),
        );
      }

      return jsonResponse(
        makeListResponse(options.mineItems ?? [options.mineItem ?? makeInboxItem()]),
      );
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
          status: "approved",
        },
      });
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: { "content-type": "application/json" },
      status: 404,
    });
  }) as unknown as typeof fetch & { requests: typeof requests };

  Object.assign(fetcher, { requests });
  return fetcher;
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
          source_project_id: "project-1",
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
            route: "/projects/project-1",
          },
          item_type: "project_decision",
          source_id: "decision-1",
          source_project_id: "project-1",
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

  it("shows a default guidance panel until a human decision item is selected", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "选择一条事项查看详情" })).toBeVisible();
    await expect.element(screen.getByText("今日待处理摘要")).toBeVisible();
    await expect.element(screen.getByText("选择事项后可执行")).toBeVisible();
    expect(screen.getByRole("button", { name: "同意" }).query()).toBeNull();
    expect(screen.getByRole("button", { name: "驳回" }).query()).toBeNull();
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
    await expect.element(screen.getByRole("link", { name: "进入流程实例" })).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "查看流程编排" })).toBeVisible();
  });

  // 已处理事项(如经飞书批准)在"已处理"过滤下选中:详情必须呈终态,不得再渲染
  // "待我处理"徽标与可执行按钮(回归:详情面板曾写死待办态)。
  it("renders resolved items as settled: no pending badge, no action buttons", async () => {
    const resolvedItem = makeInboxItem({
      status: "resolved",
      resolved_at: "2026-07-19T05:05:00Z",
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

  it("opens item details when clicking the pending item row body", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByRole("heading", { name: "选择一条事项查看详情" })).toBeVisible();
    await userEvent.click(screen.getByText("需要确认客户侧 Runtime 节点接入证据。"));

    await expect.element(screen.getByRole("heading", { name: "确认客户 Runtime 接入" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "同意" })).toBeVisible();
  });

  it("renders inbox items in a compact list within a work surface", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();

    expect(document.body.querySelector('[data-slot="v3-work-surface"]')).not.toBeNull();
    const itemRows = document.body.querySelectorAll('[role="button"][aria-label^="打开事项"]');
    expect(itemRows.length).toBeGreaterThanOrEqual(1);
  });

  it("renders the inbox with v3 Soft-Flat containers", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();

    expect(document.body.querySelector('[data-slot="v3-page-header"] [data-slot="v3-icon-tile"]')).not.toBeNull();
    expect(document.body.querySelectorAll('[data-slot="v3-soft-card"]').length).toBeGreaterThanOrEqual(2);
    expect(document.body.querySelector('[data-slot="v3-work-surface"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-tabs"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-status-pill"]')).not.toBeNull();
  });

  it("keeps summary metrics in cards instead of repeating them in the page header", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();

    const pageHeader = document.body.querySelector('[data-slot="v3-page-header"]');
    expect(pageHeader).not.toBeNull();
    expect(pageHeader?.textContent).not.toContain("开放 1");
    expect(pageHeader?.textContent).not.toContain("高风险 1");
    expect(pageHeader?.textContent).not.toContain("阻断 1");
    const metricLabels = Array.from(document.body.querySelectorAll('[data-slot="v3-soft-card"] p'))
      .map((node) => node.textContent?.trim())
      .filter(Boolean);
    expect(metricLabels).toContain("开放事项");
    expect(metricLabels).toContain("高风险");
    expect(metricLabels).toContain("阻断");
  });

  it("renders read-only inbox items when API returns null actions", async () => {
    const itemWithNullActions = makeInboxItem({
      actions: null as unknown as InboxItem["actions"],
      title: "验收项目交付",
    });
    const screen = await renderInboxView(createInboxFetcher({ mineItem: itemWithNullActions }));

    await expect.element(screen.getByText("验收项目交付")).toBeVisible();
    expect(screen.getByRole("button", { name: "同意" }).query()).toBeNull();
    expect(screen.getByRole("button", { name: "驳回" }).query()).toBeNull();
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

  it("validates project and target user UUID filters before applying them", async () => {
    const fetcher = createInboxFetcher();
    const screen = await renderInboxView(fetcher);
    const projectId = "11111111-1111-4111-8111-111111111111";
    const targetUserId = "22222222-2222-4222-8222-222222222222";

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "更多筛选" }));
    await userEvent.fill(screen.getByLabelText("项目 ID"), "project-42");
    await userEvent.fill(screen.getByLabelText("目标用户 ID"), "human-owner-42");

    await vi.waitFor(() => {
      const requestUrl = latestInboxRequestUrl(fetcher);
      expect(requestUrl?.searchParams.has("project_id")).toBe(false);
      expect(requestUrl?.searchParams.has("target_user_id")).toBe(false);
      expect(requestUrl?.searchParams.get("offset")).toBe("0");
    });
    await expect.element(screen.getByText("请输入有效 UUID")).toBeVisible();

    await userEvent.fill(screen.getByLabelText("项目 ID"), projectId);
    await userEvent.fill(screen.getByLabelText("目标用户 ID"), targetUserId);

    await vi.waitFor(() => {
      const requestUrl = latestInboxRequestUrl(fetcher);
      expect(requestUrl?.searchParams.get("project_id")).toBe(projectId);
      expect(requestUrl?.searchParams.get("target_user_id")).toBe(targetUserId);
      expect(requestUrl?.searchParams.get("offset")).toBe("0");
    });

    await userEvent.click(screen.getByRole("button", { name: "重置" }));

    await vi.waitFor(() => {
      const requestUrl = latestInboxRequestUrl(fetcher);
      expect(requestUrl?.searchParams.has("project_id")).toBe(false);
      expect(requestUrl?.searchParams.has("target_user_id")).toBe(false);
      expect(requestUrl?.searchParams.get("status")).toBe("open");
      expect(requestUrl?.searchParams.get("offset")).toBe("0");
    });
    await expect.element(screen.getByLabelText("项目 ID")).toHaveValue("");
    await expect.element(screen.getByLabelText("目标用户 ID")).toHaveValue("");
    await expect.element(screen.getByRole("button", { name: "状态" })).toHaveTextContent(
      "开放",
    );
  });

  it("refetches the inbox list when the SSE stream pushes inbox-changed", async () => {
    const fetcher = createInboxFetcher();
    const listeners: Record<string, () => void> = {};
    const fakeSource = {
      addEventListener: (type: string, listener: () => void) => {
        listeners[type] = listener;
      },
      removeEventListener: () => {},
      close: () => {},
    } as unknown as EventSource;
    const streamUrls: string[] = [];
    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <InboxView
          apiBaseUrl="http://control-plane.local"
          fetcher={fetcher}
          eventSourceFactory={(url) => {
            streamUrls.push(url);
            return fakeSource;
          }}
        />
      </QueryClientProvider>,
    );

    // 先等首查询渲染完成,再触发推送,确保 invalidate 不会与在途首查询去重。
    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    expect(streamUrls).toEqual(["http://control-plane.local/api/v1/inbox/stream"]);
    const before = inboxRequestUrls(fetcher).length;

    listeners["inbox-changed"]?.();
    await expect.poll(() => inboxRequestUrls(fetcher).length).toBeGreaterThan(before);
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
        route: "//evil.example/path",
      },
      source_project_id: "safe-project",
    });
    const screen = await renderInboxView(createInboxFetcher({ mineItem: unsafeItem }));

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    const contextLink = screen.getByRole("link", { name: "查看上下文" });
    await expect.element(contextLink).toHaveAttribute("href", "/projects/safe-project#approval-1");
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
    await expect.element(dialog.getByText("风险等级")).toBeVisible();
    await expect.element(dialog.getByText("高风险")).toBeVisible();
    await expect.element(dialog.getByText("来源项目")).toBeVisible();
    await expect.element(dialog.getByText("project-1")).toBeVisible();
    await expect.element(dialog.getByText("来源任务")).toBeVisible();
    await expect.element(dialog.getByText("task-1")).toBeVisible();
    await expect.element(dialog.getByText("上下文摘要")).toBeVisible();
    await expect.element(dialog.getByText("客户接入项目")).toBeVisible();
    await expect.element(dialog.getByText("上游审批服务暂时不可用")).toBeVisible();
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
      title: "确认二号任务",
    });
    const fetcher = createInboxFetcher({
      actionDelays: { "inbox-item-1": deferred.promise },
      mineItems: [itemA, itemB],
    });
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "打开事项：确认客户 Runtime 接入" }));
    await userEvent.click(screen.getByRole("button", { name: "同意" }));
    await userEvent.click(screen.getByRole("button", { name: "提交" }));

    // 第一条仍在提交:弹窗可直接关闭,提交在后台继续。
    await userEvent.click(screen.getByRole("button", { name: "关闭" }));
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
    await Promise.all([
      userEvent.click(screen.getByRole("button", { name: "提交" })),
      userEvent.click(screen.getByRole("button", { name: "提交" })),
    ]);

    expect(fetcher.requests.filter((request) => request.method === "POST")).toHaveLength(1);
    expect(JSON.parse(fetcher.requests.find((request) => request.method === "POST")?.body ?? "{}")).toMatchObject({
      action: "approved",
    });
    deferred.resolve();
  });
});
