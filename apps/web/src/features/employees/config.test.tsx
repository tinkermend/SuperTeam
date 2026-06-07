import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeConfigView } from "./config";

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
  Link: ({ children, to }: { children: ReactNode; to: string; params?: unknown }) => (
    <a href={to}>{children}</a>
  ),
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

const employee = {
  id: "11111111-1111-4111-8111-111111111111",
  tenant_id: "22222222-2222-4222-8222-222222222222",
  team_id: "33333333-3333-4333-8333-333333333333",
  owner_user_id: "44444444-4444-4444-8444-444444444444",
  employee_type: "requirements_analyst",
  name: "需求分析员工",
  role: "requirements_analyst",
  description: "负责需求拆解和交付风险识别",
  status: "active" as const,
  permission_policy: {},
  context_policy: {},
  approval_policy: {},
  risk_level: "medium",
  created_at: "2026-06-07T00:00:00Z",
  updated_at: "2026-06-07T00:00:00Z",
};

function requestUrl(input: RequestInfo | URL) {
  return input instanceof Request ? input.url : input.toString();
}

function requestMethod(input: RequestInfo | URL, init?: RequestInit) {
  return init?.method ?? (input instanceof Request ? input.method : "GET");
}

describe("EmployeeConfigView", () => {
  it("renders employee name and config form", async () => {
    const queryClient = createQueryClient();
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestUrl(input);
      if (url.includes("/digital-employees/")) {
        return new Response(JSON.stringify(employee), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      return new Response(JSON.stringify({}), { status: 404 });
    });

    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText(employee.name)).toBeVisible();
    await expect.element(screen.getByText("配置员工技能、策略和输出契约")).toBeVisible();
    await expect.element(screen.getByLabelText("Role Profile (JSON)")).toBeVisible();
    await expect.element(screen.getByLabelText("Constitution Addendum (JSON)")).toBeVisible();
  });

  it("submits config revision on save", async () => {
    const queryClient = createQueryClient();
    const fetcher = vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
      const url = requestUrl(input);
      const method = requestMethod(input, options);
      if (url.includes("/digital-employees/") && method === "POST") {
        return new Response(JSON.stringify({ id: "revision-1", status: "draft" }), {
          status: 201,
          headers: { "content-type": "application/json" },
        });
      }
      if (url.includes("/digital-employees/")) {
        return new Response(JSON.stringify(employee), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      return new Response(JSON.stringify({}), { status: 404 });
    });

    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("button", { name: /保存配置/ })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));
    await expect.element(screen.getByText("配置已保存")).toBeVisible();
  });

  it("submits budget policy as part of a config revision", async () => {
    const queryClient = createQueryClient();
    const fetcher = vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
      const url = requestUrl(input);
      const method = requestMethod(input, options);
      if (url.includes("/digital-employees/") && method === "POST") {
        return new Response(JSON.stringify({ id: "revision-1", status: "draft" }), {
          status: 201,
          headers: { "content-type": "application/json" },
        });
      }
      if (url.includes("/digital-employees/")) {
        return new Response(JSON.stringify(employee), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      return new Response(JSON.stringify({}), { status: 404 });
    });

    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("button", { name: /保存配置/ })).toBeVisible();
    await userEvent.type(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" }), "15000");
    await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));

    const postCall = fetcher.mock.calls.find(
      ([input, init]) => requestUrl(input).includes("/config-revisions") && init?.method === "POST",
    );
    expect(postCall).toBeTruthy();
    const body = JSON.parse(String(postCall?.[1]?.body));
    expect(body.budget_policy).toEqual({ daily_token_limit: 15000 });
  });

  it("submits empty budget policy when the daily token budget is empty", async () => {
    const queryClient = createQueryClient();
    const fetcher = vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
      const url = requestUrl(input);
      const method = requestMethod(input, options);
      if (url.includes("/digital-employees/") && method === "POST") {
        return new Response(JSON.stringify({ id: "revision-1", status: "draft" }), {
          status: 201,
          headers: { "content-type": "application/json" },
        });
      }
      if (url.includes("/digital-employees/")) {
        return new Response(JSON.stringify(employee), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      return new Response(JSON.stringify({}), { status: 404 });
    });

    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("button", { name: /保存配置/ })).toBeVisible();
    await expect.element(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" })).toHaveValue(null);
    await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));

    const postCall = fetcher.mock.calls.find(
      ([input, init]) => requestUrl(input).includes("/config-revisions") && init?.method === "POST",
    );
    expect(postCall).toBeTruthy();
    const body = JSON.parse(String(postCall?.[1]?.body));
    expect(body.budget_policy).toEqual({});
  });

  it.each(["0", "12.5"])("blocks invalid daily token budget %s when saving config", async (invalidValue) => {
    const queryClient = createQueryClient();
    const fetcher = vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
      const url = requestUrl(input);
      const method = requestMethod(input, options);
      if (url.includes("/digital-employees/") && method === "POST") {
        return new Response(JSON.stringify({ id: "revision-1", status: "draft" }), {
          status: 201,
          headers: { "content-type": "application/json" },
        });
      }
      if (url.includes("/digital-employees/")) {
        return new Response(JSON.stringify(employee), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      return new Response(JSON.stringify({}), { status: 404 });
    });

    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("button", { name: /保存配置/ })).toBeVisible();
    await userEvent.type(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" }), invalidValue);
    await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));

    await expect.element(screen.getByText("每日 Token 预算上限必须是正整数")).toBeVisible();
    const postCall = fetcher.mock.calls.find(
      ([input, init]) => requestUrl(input).includes("/config-revisions") && init?.method === "POST",
    );
    expect(postCall).toBeUndefined();
  });
});
