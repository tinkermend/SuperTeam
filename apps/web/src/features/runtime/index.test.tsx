import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { RuntimeNodesView } from "@/features/runtime";

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
  const requests: Array<{ method: string; pathname: string }> = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    requests.push({ method, pathname: url.pathname });

    if (url.pathname === "/api/v1/runtime/enrollments" && method === "GET") {
      return jsonResponse([
        {
          id: "11111111-1111-4111-8111-111111111111",
          node_id: "customer-vm-01",
          status: "pending",
        },
      ]);
    }
    if (
      url.pathname === "/api/v1/runtime/enrollments/11111111-1111-4111-8111-111111111111/approve" &&
      method === "POST"
    ) {
      return jsonResponse({
        id: "11111111-1111-4111-8111-111111111111",
        node_id: "customer-vm-01",
        status: "approved",
      });
    }
    if (url.pathname === "/api/v1/runtime/nodes" && method === "GET") {
      return jsonResponse([
        {
          node_id: "runtime-node-01",
          name: "developer-machine",
          supported_providers: ["claude-code", "opencode"],
          max_slots: 4,
          current_load: 1,
          status: "online",
        },
      ]);
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
  it("renders runtime enrollments and nodes", async () => {
    const { fetcher } = createRuntimeFetcher();
    const screen = await renderRuntimeNodesView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "Runtime 节点" })).toBeVisible();
    await expect.element(screen.getByText("customer-vm-01")).toBeVisible();
    await expect.element(screen.getByText("developer-machine")).toBeVisible();
    await expect.element(screen.getByText("claude-code, opencode")).toBeVisible();
  });

  it("approves pending runtime enrollment", async () => {
    const { fetcher, requests } = createRuntimeFetcher();
    const screen = await renderRuntimeNodesView(fetcher);

    await expect.element(screen.getByText("customer-vm-01")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "接入" }));

    await vi.waitFor(() => {
      expect(requests).toContainEqual({
        method: "POST",
        pathname: "/api/v1/runtime/enrollments/11111111-1111-4111-8111-111111111111/approve",
      });
    });
  });
});
