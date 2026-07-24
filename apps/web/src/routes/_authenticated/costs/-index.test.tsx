import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import type { CostSummary } from "@/lib/api/costs";
import { Route } from "./index";

type TestFetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

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

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router",
  );

  return {
    ...actual,
    Link: ({
      children,
      params,
      to,
      ...props
    }: {
      children: ReactNode;
      params?: Record<string, string>;
      to: string;
    }) => {
      const href =
        to === "/employees/$employeeId" ? `/employees/${params?.employeeId ?? "$employeeId"}` : to;
      return (
        <a {...props} href={href}>
          {children}
        </a>
      );
    }
};
});

vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local"
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false
}
}
});
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: {
      "content-type": "application/json"
},
    status: 200
});
}

const summaryFixture: CostSummary = {
  period_start: "2026-06-03",
  period_end: "2026-07-03",
  total_tokens: 1800,
  total_runs: 5,
  by_employee: [
    {
      employee_id: "11111111-1111-4111-8111-111111111111",
      employee_name: "需求分析员工",
      provider_type: "claude-code",
      run_count: 5,
      total_tokens: 1800
},
  ],
  by_provider: { "claude-code": 1800 },
  daily_trend: []
};

function createCostsFetcher(summary: CostSummary = summaryFixture) {
  return vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    const url = new URL(String(input));

    if (url.pathname === "/api/v1/costs/summary") {
      return jsonResponse(summary);
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: {
        "content-type": "application/json"
},
      status: 404
});
  });
}

async function renderCostsRoute(fetcher: TestFetcher) {
  vi.stubGlobal("fetch", fetcher);
  const CostsComponent = Route.options.component!;

  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <CostsComponent />
    </QueryClientProvider>,
  );
}

describe("CostsRoute", () => {
  it("renders the token usage summary with v3 surfaces", async () => {
    const fetcher = createCostsFetcher();
    const screen = await renderCostsRoute(fetcher);

    await expect.element(screen.getByRole("heading", { name: "成本管理" })).toBeVisible();
    await expect.element(screen.getByText("数字员工 Token 用量统计")).toBeVisible();
    await expect.element(screen.getByText("需求分析员工")).toBeVisible();

    await vi.waitFor(() => {
      const requestedPaths = fetcher.mock.calls.map((call) => new URL(String(call[0])).pathname);
      expect(requestedPaths).toContain("/api/v1/costs/summary");
    });
    expect(document.body.querySelector('[data-slot="page-header"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="data-table"]')).not.toBeNull();
  });

  it("renders the empty state when there is no execution data", async () => {
    const fetcher = createCostsFetcher({
      ...summaryFixture,
      total_tokens: 0,
      total_runs: 0,
      by_employee: [],
      by_provider: {}
});
    const screen = await renderCostsRoute(fetcher);

    await expect.element(screen.getByRole("heading", { name: "成本管理" })).toBeVisible();
    await expect.element(screen.getByText("暂无执行数据")).toBeVisible();
    expect(document.body.querySelector('[data-slot="empty-state"]')).not.toBeNull();
  });
});
