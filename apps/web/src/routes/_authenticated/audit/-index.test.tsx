import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { Route } from "./index";

const projectId = "11111111-1111-4111-8111-111111111111";

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
    useSearch: () => ({ project_id: projectId })
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

function createAuditFetcher() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));

    if (url.pathname === "/api/v1/audit/events") {
      return jsonResponse([
        {
          action: "project.updated",
          actor_id: "admin-user",
          actor_type: "human",
          created_at: "2026-06-20T08:30:00Z",
          details: { field: "status" },
          event_type: "project.lifecycle",
          id: "audit-1",
          ip_address: "127.0.0.1",
          resource_id: projectId,
          resource_type: "project",
          tenant_id: "tenant-1"
},
      ]);
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: {
        "content-type": "application/json"
},
      status: 404
});
  }) as unknown as typeof fetch;
}

async function renderAuditRoute(fetcher = createAuditFetcher()) {
  vi.stubGlobal("fetch", fetcher);
  const AuditComponent = Route.options.component!;

  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <AuditComponent />
    </QueryClientProvider>,
  );
}

describe("AuditRoute", () => {
  it("renders project audit events with v3 surfaces only", async () => {
    const screen = await renderAuditRoute();

    await expect.element(screen.getByRole("heading", { name: "审计中心" })).toBeVisible();
    await expect.element(screen.getByText("project.updated")).toBeVisible();

    expect(document.body.querySelector('[data-slot="work-surface"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="data-table"]')).not.toBeNull();
  });
});
