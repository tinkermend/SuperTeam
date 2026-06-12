import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { InboxView } from "@/features/inbox";
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
        key: "approve",
        label: "通过",
        requires_comment: false,
        tone: "success",
      },
      {
        key: "reject",
        label: "退回",
        requires_comment: true,
        tone: "danger",
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

function createInboxFetcher(options: { slowTeamView?: boolean } = {}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/inbox/items" && method === "GET") {
      const view = url.searchParams.get("view") ?? "mine";

      if (view === "team") {
        if (options.slowTeamView) {
          await new Promise((resolve) => setTimeout(resolve, 120));
        }

        return jsonResponse(
          makeListResponse([
            makeInboxItem({
              id: "team-inbox-item-1",
              summary: "团队负责人需要确认发布窗口。",
              target_user_id: "human-owner-1",
              title: "团队发布窗口确认",
            }),
          ]),
        );
      }

      return jsonResponse(makeListResponse([makeInboxItem()]));
    }

    if (url.pathname === "/api/v1/inbox/items/inbox-item-1/actions" && method === "POST") {
      return jsonResponse({
        item: makeInboxItem({ status: "resolved" }),
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
  }) as unknown as typeof fetch;
}

async function renderInboxView(fetcher = createInboxFetcher()) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <InboxView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("InboxView", () => {
  it("renders mine inbox by default", async () => {
    const screen = await renderInboxView();

    await expect.element(screen.getByRole("heading", { name: "收件箱" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "我的待办", selected: true })).toBeVisible();
    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
  });

  it("keeps existing data while switching to team inbox", async () => {
    const fetcher = createInboxFetcher({ slowTeamView: true });
    const screen = await renderInboxView(fetcher);

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await userEvent.click(screen.getByRole("tab", { name: "团队待办" }));

    await expect.element(screen.getByText("确认客户 Runtime 接入")).toBeVisible();
    await expect.element(screen.getByText("正在刷新")).toBeVisible();
    await expect.element(screen.getByText("团队发布窗口确认")).toBeVisible();
  });

  it("hides action buttons in team view and still shows context link", async () => {
    const screen = await renderInboxView();

    await userEvent.click(screen.getByRole("tab", { name: "团队待办" }));

    await expect.element(screen.getByText("团队发布窗口确认")).toBeVisible();
    expect(screen.getByRole("button", { name: "通过" }).query()).toBeNull();
    expect(screen.getByRole("button", { name: "退回" }).query()).toBeNull();
    await expect.element(screen.getByRole("link", { name: "查看上下文" })).toHaveAttribute(
      "href",
      "/projects/project-1/approvals#approval-1",
    );
  });
});
