import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { RuntimeNodesView } from "@/features/runtime";
import type { RuntimeOverview } from "@/lib/api/runtime";

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

const runtimeOverviewFixture = {
  summary: {
    online_nodes: 6,
    total_nodes: 8,
    pending_enrollments: 2,
    active_provider_sessions: 14,
    blocked_events: 1,
  },
  pending_enrollments: [
    {
      id: "11111111-1111-4111-8111-111111111111",
      node_id: "customer-vm-east-01",
      status: "pending",
      created_at: "2026-06-05T03:21:33Z",
      last_hello_at: "2026-06-05T03:22:00Z",
      request_payload: {
        name: "customer-vm-east-01",
        supported_providers: ["codex"],
        max_slots: 4,
      },
    },
  ],
  nodes: [
    {
      node_id: "prod-runtime-shanghai-01",
      name: "prod-runtime-shanghai-01",
      supported_providers: ["claude-code"],
      max_slots: 10,
      current_load: 6,
      status: "online",
      last_heartbeat_at: "2026-06-05T03:22:30Z",
    },
  ],
  provider_capabilities: [
    {
      provider_type: "claude-code",
      node_count: 1,
      available_count: 1,
      healthy_count: 1,
      last_seen_at: "2026-06-05T03:22:20Z",
    },
  ],
  recent_events: [
    {
      id: "22222222-2222-4222-8222-222222222222",
      event_type: "command_completed",
      severity: "success",
      source: "runtime_command",
      title: "Runtime command completed",
      node_id: "prod-runtime-shanghai-01",
      provider_type: "claude-code",
      created_at: "2026-06-05T03:22:20Z",
    },
  ],
} satisfies RuntimeOverview;

type RuntimeRequest = {
  body?: unknown;
  method: string;
  pathname: string;
  search: string;
};

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        retry: false,
      },
    },
  });
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status: 200,
  });
}

function createRuntimeFetcher() {
  const requests: RuntimeRequest[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const request: RuntimeRequest = { method, pathname: url.pathname, search: url.search };

    if (typeof init?.body === "string") {
      request.body = JSON.parse(init.body);
    }

    requests.push(request);

    if (url.pathname === "/api/v1/runtime/overview" && method === "GET") {
      return jsonResponse(runtimeOverviewFixture);
    }

    if (url.pathname === "/api/v1/runtime/events" && method === "GET") {
      return jsonResponse({
        items: url.searchParams.get("severity") ? [] : runtimeOverviewFixture.recent_events,
        limit: Number(url.searchParams.get("limit") ?? 50),
        offset: Number(url.searchParams.get("offset") ?? 0),
      });
    }

    if (
      url.pathname === "/api/v1/runtime/enrollments/11111111-1111-4111-8111-111111111111/approve" &&
      method === "POST"
    ) {
      return jsonResponse({
        ...runtimeOverviewFixture.pending_enrollments[0],
        status: "approved",
      });
    }

    if (
      url.pathname === "/api/v1/runtime/enrollments/11111111-1111-4111-8111-111111111111/reject" &&
      method === "POST"
    ) {
      return jsonResponse({
        ...runtimeOverviewFixture.pending_enrollments[0],
        status: "rejected",
      });
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: { "content-type": "application/json" },
      status: 404,
    });
  }) as unknown as typeof fetch;

  return { fetcher, requests };
}

async function renderRuntimeNodesView(fetcher = createRuntimeFetcher().fetcher) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <RuntimeNodesView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("RuntimeNodesView", () => {
  it("renders the runtime overview console", async () => {
    const { fetcher } = createRuntimeFetcher();
    const screen = await renderRuntimeNodesView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "Runtime 节点" })).toBeVisible();
    await expect.element(screen.getByText("6 / 8")).toBeVisible();
    await expect.element(screen.getByText("customer-vm-east-01")).toBeVisible();
    await expect.element(screen.getByText("节点 ID：prod-runtime-shanghai-01")).toBeVisible();
    await expect.element(screen.getByText("Provider：claude-code").first()).toBeVisible();
    await expect.element(screen.getByText("Runtime command completed")).toBeVisible();
  });

  it("does not render out-of-scope runtime management surfaces", async () => {
    const { fetcher } = createRuntimeFetcher();
    const screen = await renderRuntimeNodesView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "Runtime 节点" })).toBeVisible();
    expect(screen.getByText(/详情|诊断包|下载诊断|接入密钥|创建接入密钥|撤销/).query()).toBeNull();
  });

  it("rejects a pending runtime enrollment with a reason", async () => {
    const { fetcher, requests } = createRuntimeFetcher();
    const screen = await renderRuntimeNodesView(fetcher);

    await expect.element(screen.getByText("customer-vm-east-01")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "拒绝" }));
    await userEvent.fill(screen.getByLabelText("拒绝原因"), "节点归属未完成线下确认");
    await userEvent.click(screen.getByRole("button", { name: "确认拒绝" }));

    await vi.waitFor(() => {
      expect(requests).toContainEqual({
        body: { reason: "节点归属未完成线下确认" },
        method: "POST",
        pathname: "/api/v1/runtime/enrollments/11111111-1111-4111-8111-111111111111/reject",
        search: "",
      });
    });

    await vi.waitFor(() => {
      expect(screen.getByRole("dialog").query()).toBeNull();
    });

    await userEvent.click(screen.getByRole("button", { name: "拒绝" }));
    await expect.element(screen.getByLabelText("拒绝原因")).toHaveValue("");
  });

  it("filters event audit by severity", async () => {
    const { fetcher, requests } = createRuntimeFetcher();
    const screen = await renderRuntimeNodesView(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "事件审计" }));
    await userEvent.click(screen.getByLabelText("严重级别"));
    await userEvent.click(screen.getByRole("option", { name: "错误" }));

    await vi.waitFor(() => {
      expect(
        requests.some((request) => {
          const params = new URLSearchParams(request.search);
          return (
            request.method === "GET" &&
            request.pathname === "/api/v1/runtime/events" &&
            params.get("severity") === "error"
          );
        }),
      ).toBe(true);
    });

    const severityErrorRequestIndex = requests.findIndex((request) => {
      const params = new URLSearchParams(request.search);
      return (
        request.method === "GET" &&
        request.pathname === "/api/v1/runtime/events" &&
        params.get("severity") === "error"
      );
    });

    await userEvent.click(screen.getByLabelText("严重级别"));
    await userEvent.click(screen.getByRole("option", { name: "全部" }));

    await vi.waitFor(() => {
      expect(
        requests.slice(severityErrorRequestIndex + 1).some((request) => {
          const params = new URLSearchParams(request.search);
          return (
            request.method === "GET" &&
            request.pathname === "/api/v1/runtime/events" &&
            !params.has("severity")
          );
        }),
      ).toBe(true);
    });
  });

  it("approves a pending runtime enrollment", async () => {
    const { fetcher, requests } = createRuntimeFetcher();
    const screen = await renderRuntimeNodesView(fetcher);

    await expect.element(screen.getByText("customer-vm-east-01")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "批准接入" }));
    await userEvent.click(screen.getByRole("button", { name: "确认接入" }));

    await vi.waitFor(() => {
      expect(requests).toContainEqual({
        method: "POST",
        pathname: "/api/v1/runtime/enrollments/11111111-1111-4111-8111-111111111111/approve",
        search: "",
      });
    });

    await vi.waitFor(() => {
      expect(screen.getByRole("alertdialog").query()).toBeNull();
    });
  });
});
