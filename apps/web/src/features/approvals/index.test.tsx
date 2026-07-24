import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { ApprovalsCenterView } from "@/features/approvals";
import type { InboxItem, InboxListResponse } from "@/lib/api/inbox";

let routerSearch: Record<string, string | undefined> = {};

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a data-router-link="true" href={to}>
      {children}
    </a>
  ),
  useSearch: () => routerSearch
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>
}));

vi.mock("@/components/layout/shell-page-header", () => ({
  ShellPageHeader: ({
    subtitle,
    title
}: {
    subtitle?: ReactNode;
    title: ReactNode;
  }) => (
    <header>
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
    </header>
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

function makeApprovalItem(overrides: Partial<InboxItem> = {}): InboxItem {
  return {
    actions: [
      {
        key: "approved",
        label: "Approve",
        requires_comment: false,
        tone: "positive"
},
    ],
    context: {
      project_name: "客户接入项目",
      source_title: "上线审批"
},
    created_at: "2026-07-06T09:00:00Z",
    deep_link: {},
    id: "approval-item-1",
    item_type: "project_decision",
    last_activity_at: "2026-07-06T09:20:00Z",
    priority: "high",
    risk_level: "high",
    source_id: "decision-1",
    source_project_id: "project-1",
    source_task_id: "task-1",
    source_type: "project_decision_request",
    status: "open",
    summary: "需要确认客户接入上线风险。",
    target_user_id: "human-owner-1",
    tenant_id: "tenant-1",
    title: "确认上线风险",
    updated_at: "2026-07-06T09:20:00Z",
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
      blocked_count: 0,
      high_risk_count: items.filter((item) => item.risk_level === "high").length,
      open_count: items.length
}
};
}

function createApprovalsFetcher(options: { actionStatus?: number } = {}) {
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
      return jsonResponse(makeListResponse([makeApprovalItem()]));
    }

    if (url.pathname === "/api/v1/inbox/items/approval-item-1/actions" && method === "POST") {
      if (options.actionStatus && options.actionStatus >= 400) {
        return jsonResponse({ error: "审批源暂时不可用" }, options.actionStatus);
      }
      return jsonResponse({
        item: makeApprovalItem({ status: "resolved" }),
        source_result: {
          source_id: "decision-1",
          source_type: "project_decision_request",
          status: "approved"
}
});
    }

    return jsonResponse({ error: `unhandled ${url.pathname}` }, 404);
  }) as unknown as typeof fetch & { requests: typeof requests };

  Object.assign(fetcher, { requests });
  return fetcher;
}

async function renderApprovals(fetcher: typeof fetch, search: Record<string, string | undefined> = {}) {
  routerSearch = search;
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <ApprovalsCenterView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("ApprovalsCenterView", () => {
  it("renders real inbox approval data and applies URL filters", async () => {
    const fetcher = createApprovalsFetcher();
    const screen = await renderApprovals(fetcher, {
      project: "project-1",
      risk: "high",
      status: "open"
});

    await expect.element(screen.getByRole("heading", { name: "审批中心" })).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "确认上线风险" })).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "查看项目审批" })).toHaveAttribute(
      "href",
      "/projects/project-1?tab=approval&focus=decision-1",
    );
    expect(screen.getByText("承载高风险动作").query()).toBeNull();

    const listRequest = fetcher.requests.find(
      (request) => request.method === "GET" && request.pathname === "/api/v1/inbox/items",
    );
    expect(listRequest).toBeDefined();
    const url = new URL(listRequest?.url ?? "http://missing");
    expect(url.searchParams.get("status")).toBe("open");
    expect(url.searchParams.get("risk_level")).toBe("high");
    expect(url.searchParams.get("project_id")).toBe("project-1");
  });

  it("reuses the inbox action dialog for approval actions", async () => {
    const fetcher = createApprovalsFetcher({ actionStatus: 500 });
    const screen = await renderApprovals(fetcher);

    await expect.element(screen.getByRole("heading", { name: "确认上线风险" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "同意 确认上线风险" }));
    await userEvent.click(screen.getByRole("button", { name: "提交" }));

    await expect.element(screen.getByRole("dialog")).toBeVisible();
    await expect.element(screen.getByText("审批源暂时不可用")).toBeVisible();
  });
});
