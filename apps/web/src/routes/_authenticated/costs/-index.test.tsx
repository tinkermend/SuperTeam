import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { Route } from "./index";

const projectId = "11111111-1111-4111-8111-111111111111";
const routerState = vi.hoisted(() => ({
  search: {} as Record<string, string>,
}));
type TestFetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router",
  );

  return {
    ...actual,
    useSearch: () => routerState.search,
  };
});

vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local",
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: {
      "content-type": "application/json",
    },
    status: 200,
  });
}

function createCostsFetcher() {
  const fetcher = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    const url = new URL(String(input));

    if (url.pathname === `/api/v1/projects/${projectId}/budget-ledger`) {
      return jsonResponse([
        {
          actual_cost: "0.32",
          actual_tokens: 1800,
          cost_type: "inference",
          estimated_cost: "0.40",
          estimated_tokens: 2200,
          id: "budget-ledger-1",
          project_id: projectId,
          reason: "route decision execution",
          source: "runtime-agent",
          tenant_id: "tenant-1",
        },
      ]);
    }

    if (url.pathname === `/api/v1/projects/${projectId}/budget-summary`) {
      return jsonResponse({
        actual_cost: "0.32",
        actual_tokens: 1800,
        estimated_cost: "0.40",
        estimated_tokens: 2200,
        ledger_count: 1,
      });
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: {
        "content-type": "application/json",
      },
      status: 404,
    });
  });

  return fetcher;
}

async function renderCostsRoute(
  fetcher: TestFetcher = vi.fn(async () => new Response(null)),
) {
  vi.stubGlobal("fetch", fetcher);
  const CostsComponent = Route.options.component!;

  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <CostsComponent />
    </QueryClientProvider>,
  );
}

describe("CostsRoute", () => {
  it("renders the no-project state with v3 surfaces only", async () => {
    routerState.search = {};
    const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(null),
    );
    const screen = await renderCostsRoute(fetcher);

    await expect.element(screen.getByRole("heading", { name: "成本中心" })).toBeVisible();
    await expect.element(screen.getByText("请选择项目后查看成本")).toBeVisible();

    expect(document.body.querySelector('[data-slot="v3-page-header"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-work-surface"]')).not.toBeNull();
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("passes project_id through to the budget panel with v3 surfaces", async () => {
    routerState.search = { project_id: projectId };
    const fetcher = createCostsFetcher();
    const screen = await renderCostsRoute(fetcher);

    await expect.element(screen.getByText("预算流水")).toBeVisible();
    await expect.element(screen.getByText("route decision execution")).toBeVisible();

    await vi.waitFor(() => {
      const requestedPaths = fetcher.mock.calls.map((call) => new URL(String(call[0])).pathname);
      expect(requestedPaths).toContain(`/api/v1/projects/${projectId}/budget-ledger`);
      expect(requestedPaths).toContain(`/api/v1/projects/${projectId}/budget-summary`);
    });
    expect(document.body.querySelector('[data-slot="v3-table"]')).not.toBeNull();
  });
});
