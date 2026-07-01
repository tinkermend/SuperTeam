import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { Route } from "./index";

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

function createFetcher() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));

    if (url.pathname === "/api/auth/operation-logs") {
      return jsonResponse({
        items: [
          {
            action: "user.create",
            client_ip: "127.0.0.1",
            created_at: "2026-06-20T08:30:00Z",
            id: "op-1",
            module: "users",
            resource_id: "user-9",
            resource_type: "user",
            result: "succeeded",
            username: "admin",
          },
        ],
      });
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: {
        "content-type": "application/json",
      },
      status: 404,
    });
  }) as unknown as typeof fetch;
}

async function renderRoute(fetcher = createFetcher()) {
  vi.stubGlobal("fetch", fetcher);
  const Component = Route.options.component!;

  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <Component />
    </QueryClientProvider>,
  );
}

describe("OperationLogsRoute", () => {
  it("renders operation logs from the control plane with v3 surfaces", async () => {
    const screen = await renderRoute();

    await expect.element(screen.getByRole("heading", { name: "操作日志" })).toBeVisible();
    await expect.element(screen.getByText("user.create")).toBeVisible();
    await expect.element(screen.getByText("users")).toBeVisible();

    expect(document.body.querySelector('[data-slot="v3-work-surface"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-table"]')).not.toBeNull();
  });
});
