import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeesView } from "@/features/employees";

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
      queries: {
        retry: false,
      },
    },
  });
}

function createEmployeesFetcher() {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    if (url.pathname === "/api/v1/digital-employees" && (init?.method ?? "GET") === "GET") {
      return new Response(
        JSON.stringify([
          {
            id: "11111111-1111-4111-8111-111111111111",
            name: "需求分析员工",
            role: "requirements_analyst",
            description: "负责需求拆解和交付风险识别",
            status: "active",
            risk_level: "medium",
            execution_instance: {
              id: "22222222-2222-4222-8222-222222222222",
              runtime_node_id: "33333333-3333-4333-8333-333333333333",
              provider_type: "codex",
              status: "ready",
            },
          },
        ]),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        },
      );
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: { "content-type": "application/json" },
      status: 404,
    });
  }) as unknown as typeof fetch;

  return fetcher;
}

async function renderEmployeesView(fetcher = createEmployeesFetcher()) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <EmployeesView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("EmployeesView", () => {
  it("renders digital employees and execution instance state", async () => {
    const screen = await renderEmployeesView();

    await expect.element(screen.getByRole("heading", { name: "数字员工" })).toBeVisible();
    await expect.element(screen.getByText("需求分析员工")).toBeVisible();
    await expect.element(screen.getByText("负责需求拆解和交付风险识别")).toBeVisible();
    await expect.element(screen.getByText("codex · ready")).toBeVisible();
    await expect.element(screen.getByText("33333333-3333-4333-8333-333333333333")).toBeVisible();
  });
});
