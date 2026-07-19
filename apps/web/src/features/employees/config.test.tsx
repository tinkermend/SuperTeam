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
      mutations: {
        retry: false,
      },
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
  provider_type: "codex",
  name: "需求分析员工",
  role: "requirements_analyst",
  description: "负责需求拆解和交付风险识别",
  status: "active" as const,
  permission_policy: {},
  persona_memory_markdown: "# 人格画像\n证据优先",
  capability_bindings: {
    skills: ["incident-diagnosis"],
    mcp_servers: ["postgres-readonly"],
    external_capabilities: [],
    environment_variable_refs: ["PG_DSN"],
  },
  budget_policy: {},
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

type ExtraRoutes = Record<string, unknown>;

function routeKey(input: RequestInfo | URL, init?: RequestInit) {
  const url = new URL(requestUrl(input));
  return `${requestMethod(input, init)} ${url.pathname}${url.search}`;
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function createEmployeeConfigFetcher({ extraRoutes = {} }: { extraRoutes?: ExtraRoutes } = {}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const key = routeKey(input, init);
    if (key in extraRoutes) {
      const value = extraRoutes[key];
      return value instanceof Response ? value : jsonResponse(value);
    }

    if (key === `GET /api/v1/digital-employees/${employee.id}`) {
      return jsonResponse(employee);
    }

    if (key === `POST /api/v1/digital-employees/${employee.id}/config-revisions`) {
      return jsonResponse({ id: "revision-1", status: "draft" }, 201);
    }

    return jsonResponse({ error: `unhandled ${key}` }, 404);
  });
}

function requestBody(fetcher: ReturnType<typeof createEmployeeConfigFetcher>, path: string, method: string) {
  const call = fetcher.mock.calls.find(([input, init]) => {
    const url = new URL(requestUrl(input));
    return url.pathname === path && requestMethod(input, init) === method;
  });
  expect(call).toBeTruthy();

  return JSON.parse(String(call?.[1]?.body));
}

function hasRequest(fetcher: ReturnType<typeof createEmployeeConfigFetcher>, path: string, method: string) {
  return fetcher.mock.calls.some(([input, init]) => {
    const url = new URL(requestUrl(input));
    return url.pathname === path && requestMethod(input, init) === method;
  });
}

describe("EmployeeConfigView", () => {
  it("renders employee name and config form", async () => {
    const queryClient = createQueryClient();
    const fetcher = createEmployeeConfigFetcher();

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
    await expect.element(screen.getByText("配置员工人格记忆、能力绑定和预算策略")).toBeVisible();
    expect(screen.getByRole("tab", { name: "高级配置" }).query()).toBeNull();
    expect(screen.getByText("角色配置").query()).toBeNull();
    expect(screen.getByText("能力与策略").query()).toBeNull();
    await expect.element(screen.getByLabelText("人格记忆.md")).toBeVisible();
    await expect.element(screen.getByLabelText("能力绑定")).toBeVisible();
  });

  it("submits persona memory config revision on save", async () => {
    const queryClient = createQueryClient();
    const fetcher = createEmployeeConfigFetcher();

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
    await userEvent.fill(screen.getByLabelText("人格记忆.md"), "# 人格画像\n需求拆解优先");
    await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));
    await expect.element(screen.getByText("配置已保存")).toBeVisible();

    const body = requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST");
    expect(body).toEqual({
      persona_memory_markdown: "# 人格画像\n需求拆解优先",
      status: "draft",
    });
  });

  it("submits only budget policy for a budget-only config revision", async () => {
    const queryClient = createQueryClient();
    const fetcher = createEmployeeConfigFetcher();

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

    const body = requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST");
    expect(body).toEqual({
      budget_policy: { daily_token_limit: 15000 },
      status: "draft",
    });
  });

  it("keeps save disabled when the untouched daily token budget is empty", async () => {
    const queryClient = createQueryClient();
    const fetcher = createEmployeeConfigFetcher();

    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("button", { name: /保存配置/ })).toBeDisabled();
    await expect.element(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" })).toHaveValue(null);
    expect(
      hasRequest(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST"),
    ).toBe(false);
  });

  it("submits empty budget policy when the edited daily token budget is cleared", async () => {
    const queryClient = createQueryClient();
    const fetcher = createEmployeeConfigFetcher();

    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    const budgetInput = screen.getByRole("spinbutton", { name: "每日 Token 预算上限" });
    await userEvent.type(budgetInput, "15000");
    await userEvent.clear(budgetInput);
    await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));

    const body = requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST");
    expect(body).toEqual({
      budget_policy: {},
      status: "draft",
    });
  });

  it.each(["0", "12.5"])("blocks invalid daily token budget %s when saving config", async (invalidValue) => {
    const queryClient = createQueryClient();
    const fetcher = createEmployeeConfigFetcher();

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

  it("blocks invalid advanced JSON when saving config", async () => {
    const queryClient = createQueryClient();
    const fetcher = createEmployeeConfigFetcher();

    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await userEvent.fill(screen.getByLabelText("能力绑定"), '{"skills":');
    await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));

    await expect.element(screen.getByText("能力绑定必须是有效 JSON object")).toBeVisible();
    const postCall = fetcher.mock.calls.find(
      ([input, init]) => requestUrl(input).includes("/config-revisions") && init?.method === "POST",
    );
    expect(postCall).toBeUndefined();
  });

});
