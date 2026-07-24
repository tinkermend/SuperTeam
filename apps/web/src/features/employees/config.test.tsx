import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeConfigView } from "./config";

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

// 能力面板有自己的数据获取与测试;配置页单测在此边界打桩,只验 config.tsx 自身逻辑。
vi.mock("./components/employee-capabilities-panel", () => ({
  EmployeeCapabilitiesPanel: () => <div data-testid="capabilities-panel" />
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string; params?: unknown }) => (
    <a href={to}>{children}</a>
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
  permission_policy: { grants: ["database.read:dev_db"] },
  persona_memory_markdown: "# 人格画像\n证据优先",
  // skills/mcp_servers 是已废弃键:hydration 必须剥离,保存时绝不回传(否则服务端 400)。
  capability_bindings: {
    skills: ["incident-diagnosis"],
    mcp_servers: ["postgres-readonly"],
    external_capabilities: [],
    environment_variable_refs: ["PG_DSN"]
},
  budget_policy: {},
  risk_level: "medium",
  created_at: "2026-06-07T00:00:00Z",
  updated_at: "2026-06-07T00:00:00Z"
};

function requestUrl(input: RequestInfo | URL) {
  return input instanceof Request ? input.url : input.toString();
}

function requestMethod(input: RequestInfo | URL, init?: RequestInit) {
  return init?.method ?? (input instanceof Request ? input.method : "GET");
}

function routeKey(input: RequestInfo | URL, init?: RequestInit) {
  const url = new URL(requestUrl(input));
  return `${requestMethod(input, init)} ${url.pathname}${url.search}`;
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" }
});
}

function createEmployeeConfigFetcher() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const key = routeKey(input, init);

    if (key === `GET /api/v1/digital-employees/${employee.id}`) {
      return jsonResponse(employee);
    }
    if (key === `PUT /api/v1/digital-employees/${employee.id}/profile`) {
      const body = JSON.parse(String(init?.body ?? "{}")) as { description?: string };
      return jsonResponse({ ...employee, description: body.description ?? "" });
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

function renderConfig(fetcher: ReturnType<typeof createEmployeeConfigFetcher>) {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <EmployeeConfigView apiBaseUrl="http://localhost:8080" employeeId={employee.id} fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("EmployeeConfigView", () => {
  it("renders locator header, immediate tier form and read-only permission tier", async () => {
    const screen = await renderConfig(createEmployeeConfigFetcher());

    // 定位头:名称 + Provider 只读 + 角色
    await expect.element(screen.getByText(employee.name).first()).toBeVisible();
    await expect.element(screen.getByText("Provider（不可改）")).toBeVisible();
    await expect.element(screen.getByText("Codex")).toBeVisible();

    // 身份资料：员工说明可编辑
    await expect.element(screen.getByRole("heading", { name: "身份资料" })).toBeVisible();
    await expect.element(screen.getByLabelText("员工说明")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "保存员工说明" })).toBeDisabled();

    // 分层标题
    await expect.element(screen.getByRole("heading", { name: "即时生效配置" })).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "权限审批配置" })).toBeVisible();

    // 即时层字段
    await expect.element(screen.getByLabelText("人格记忆.md")).toBeVisible();
    await expect.element(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "保存即时配置" })).toBeVisible();

    // 权限审批层为只读呈现(role/grants),无编辑控件
    await expect.element(screen.getByText("角色与权限")).toBeVisible();
    await expect.element(screen.getByText("database.read:dev_db")).toBeVisible();
  });

  it("saves employee description via profile endpoint", async () => {
    const fetcher = createEmployeeConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.fill(screen.getByLabelText("员工说明"), "更新后的员工说明");
    await userEvent.click(screen.getByRole("button", { name: "保存员工说明" }));

    await expect.element(screen.getByText("已保存")).toBeVisible();
    const body = requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/profile`, "PUT");
    expect(body).toEqual({ description: "更新后的员工说明" });
  });

  it("keeps save disabled until an immediate field is edited", async () => {
    const fetcher = createEmployeeConfigFetcher();
    const screen = await renderConfig(fetcher);

    await expect.element(screen.getByRole("button", { name: "保存即时配置" })).toBeDisabled();
    expect(
      hasRequest(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST"),
    ).toBe(false);
  });

  it("saves the full immediate snapshot as an active revision, stripping legacy binding keys", async () => {
    const fetcher = createEmployeeConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.fill(screen.getByLabelText("人格记忆.md"), "# 人格画像\n需求拆解优先");
    await userEvent.click(screen.getByRole("button", { name: "保存即时配置" }));

    await expect.element(screen.getByText("已保存并生效")).toBeVisible();

    const body = requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST");
    expect(body).toEqual({
      persona_memory_markdown: "# 人格画像\n需求拆解优先",
      capability_bindings: {
        external_capabilities: [],
        environment_variable_refs: ["PG_DSN"]
},
      budget_policy: {}
});
  });

  it.each(["0", "12.5"])("blocks invalid daily token budget %s when saving", async (invalidValue) => {
    const fetcher = createEmployeeConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.type(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" }), invalidValue);
    await userEvent.click(screen.getByRole("button", { name: "保存即时配置" }));

    await expect.element(screen.getByText("每日 Token 预算上限必须是正整数")).toBeVisible();
    expect(
      hasRequest(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST"),
    ).toBe(false);
  });
});
