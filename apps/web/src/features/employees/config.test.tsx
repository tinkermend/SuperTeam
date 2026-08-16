import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeConfigView } from "./config";
import type { EmployeeConfigTab } from "./config-utils";
import { submitPermissionErrorMessage } from "./config-utils";
import { useState } from "react";

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

vi.mock("./components/employee-capabilities-panel", () => ({
  EmployeeCapabilitiesPanel: () => <div data-testid="capabilities-panel" />,
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string; params?: unknown }) => (
    <a href={to}>{children}</a>
  ),
  useNavigate: () => vi.fn(),
  useBlocker: () => ({ status: "idle", proceed: () => {}, reset: () => {} }),
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });
}

const employee = {
  id: "11111111-1111-4111-8111-111111111111",
  tenant_id: "22222222-2222-4222-8222-222222222222",
  team_id: "33333333-3333-4333-8333-333333333333",
  team_name: "平台工程",
  owner_user_id: "44444444-4444-4444-8444-444444444444",
  employee_type: "requirements_analyst",
  provider_type: "codex",
  name: "需求分析员工",
  role: "requirements_analyst",
  role_keys: ["developer"],
  description: "负责需求拆解和交付风险识别",
  status: "active" as const,
  permission_policy: { grants: ["database.read:dev_db"] },
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

function createEmployeeConfigFetcher(overrides?: {
  roles?: (init?: RequestInit) => Promise<Response>;
  permissionChange?: Response;
}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const key = routeKey(input, init);

    if (key === `GET /api/v1/digital-employees/${employee.id}`) {
      return jsonResponse(employee);
    }
    if (key === `GET /api/v1/digital-employees/${employee.id}/permission-change`) {
      return overrides?.permissionChange ?? new Response(null, { status: 204 });
    }
    if (key === "GET /api/v1/role-vocabulary") {
      return jsonResponse([
        {
          id: "rv-1",
          tenant_id: employee.tenant_id,
          role_key: "developer",
          title: "开发",
          description: "",
          status: "active",
          created_at: "2026-08-05T00:00:00Z",
          updated_at: "2026-08-05T00:00:00Z",
        },
        {
          id: "rv-2",
          tenant_id: employee.tenant_id,
          role_key: "reviewer",
          title: "审查",
          description: "",
          status: "active",
          created_at: "2026-08-05T00:00:00Z",
          updated_at: "2026-08-05T00:00:00Z",
        },
      ]);
    }
    if (key === `PUT /api/v1/digital-employees/${employee.id}/profile`) {
      const body = JSON.parse(String(init?.body ?? "{}")) as { description?: string; role?: string };
      return jsonResponse({ ...employee, description: body.description ?? "", role: body.role ?? employee.role });
    }
    if (key === `PUT /api/v1/digital-employees/${employee.id}/roles`) {
      if (overrides?.roles) return overrides.roles(init);
      const body = JSON.parse(String(init?.body ?? "{}")) as { role_keys?: string[] };
      return jsonResponse({ role_keys: body.role_keys ?? [] });
    }
    if (key === `POST /api/v1/digital-employees/${employee.id}/config-revisions`) {
      return jsonResponse({ id: "revision-1", status: "draft" }, 201);
    }
    if (key === `POST /api/v1/digital-employees/${employee.id}/permission-changes`) {
      return jsonResponse({ error: "employee has active work" }, 409);
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

function Harness({
  fetcher,
  initialTab = "identity",
}: {
  fetcher: ReturnType<typeof createEmployeeConfigFetcher>;
  initialTab?: EmployeeConfigTab;
}) {
  const [tab, setTab] = useState<EmployeeConfigTab>(initialTab);
  return (
    <QueryClientProvider client={createQueryClient()}>
      <EmployeeConfigView
        apiBaseUrl="http://localhost:8080"
        employeeId={employee.id}
        fetcher={fetcher}
        tab={tab}
        onTabChange={setTab}
      />
    </QueryClientProvider>
  );
}

function renderConfig(
  fetcher: ReturnType<typeof createEmployeeConfigFetcher>,
  initialTab?: EmployeeConfigTab,
) {
  return render(<Harness fetcher={fetcher} initialTab={initialTab} />);
}

describe("EmployeeConfigView", () => {
  it("renders locator header and identity tab", async () => {
    const screen = await renderConfig(createEmployeeConfigFetcher());

    await expect.element(screen.getByText("Provider（不可改）")).toBeVisible();
    await expect.element(screen.getByText("Codex")).toBeVisible();
    await expect.element(screen.getByText("职责描述").first()).toBeVisible();
    await expect.element(screen.getByText("平台工程")).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "身份" })).toBeVisible();
    await expect.element(screen.getByLabelText("员工说明")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "保存" })).toBeDisabled();
  });

  it("falls back illegal tab via parse on identity content", async () => {
    const screen = await renderConfig(createEmployeeConfigFetcher(), "identity");
    await expect.element(screen.getByLabelText("员工说明")).toBeVisible();
  });

  it("saves identity profile via profile endpoint", async () => {
    const fetcher = createEmployeeConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.fill(screen.getByLabelText("员工说明"), "更新后的员工说明");
    await userEvent.click(screen.getByRole("button", { name: "保存" }));

    const body = requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/profile`, "PUT");
    expect(body.description).toEqual("更新后的员工说明");
  });

  it("does not prompt when switching tabs without edits", async () => {
    const screen = await renderConfig(createEmployeeConfigFetcher());
    await userEvent.click(screen.getByRole("tab", { name: "能力" }));
    await expect.element(screen.getByRole("tab", { name: "能力" })).toHaveAttribute("data-state", "active");
    expect(screen.getByText("放弃未保存的更改？").query()).toBeNull();
    await userEvent.click(screen.getByRole("tab", { name: "执行配置" }));
    await expect.element(screen.getByLabelText("人格记忆.md")).toBeVisible();
    expect(screen.getByText("放弃未保存的更改？").query()).toBeNull();
  });

  it("keeps execution save disabled until a field is edited", async () => {
    const fetcher = createEmployeeConfigFetcher();
    const screen = await renderConfig(fetcher);
    await userEvent.click(screen.getByRole("tab", { name: "执行配置" }));
    await expect.element(screen.getByRole("button", { name: "保存" })).toBeDisabled();
    expect(
      hasRequest(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST"),
    ).toBe(false);
  });

  it("saves the full immediate snapshot as an active revision, stripping legacy binding keys", async () => {
    const fetcher = createEmployeeConfigFetcher();
    const screen = await renderConfig(fetcher);
    await userEvent.click(screen.getByRole("tab", { name: "执行配置" }));

    await userEvent.fill(screen.getByLabelText("人格记忆.md"), "# 人格画像\n需求拆解优先");
    await userEvent.click(screen.getByRole("button", { name: "保存" }));

    const body = requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST");
    expect(body).toEqual({
      persona_memory_markdown: "# 人格画像\n需求拆解优先",
      capability_bindings: {
        external_capabilities: [],
        environment_variable_refs: ["PG_DSN"],
      },
      budget_policy: {},
    });
  });

  it.each(["0", "12.5"])("blocks invalid daily token budget %s when saving", async (invalidValue) => {
    const fetcher = createEmployeeConfigFetcher();
    const screen = await renderConfig(fetcher);
    await userEvent.click(screen.getByRole("tab", { name: "执行配置" }));

    await userEvent.type(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" }), invalidValue);
    await userEvent.click(screen.getByRole("button", { name: "保存" }));

    await expect.element(screen.getByText("每日 Token 预算上限必须是正整数")).toBeVisible();
    expect(
      hasRequest(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST"),
    ).toBe(false);
  });

  it("confirms casting impact then resends roles", async () => {
    let calls = 0;
    const fetcher = createEmployeeConfigFetcher({
      roles: async (init) => {
        calls += 1;
        const body = JSON.parse(String(init?.body ?? "{}")) as { confirm_impact?: boolean };
        if (!body.confirm_impact) {
          return jsonResponse(
            {
              code: "casting_impact_requires_confirm",
              affected_count: 1,
              affected_castings: [
                {
                  project_id: "p1",
                  project_name: "交付项目",
                  scenario_template_key: "software_delivery",
                  template_name: "软件交付",
                  role_key: "developer",
                },
              ],
            },
            400,
          );
        }
        return jsonResponse({ role_keys: [] });
      },
    });
    const screen = await renderConfig(fetcher);
    await userEvent.click(screen.getByLabelText("剧本角色 开发"));
    await userEvent.click(screen.getByRole("button", { name: "保存" }));
    await expect.element(screen.getByText("确认解除受影响编制")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "确认并保存" }));
    expect(calls).toBeGreaterThanOrEqual(2);
  });

  it("maps permission submit errors", () => {
    expect(submitPermissionErrorMessage(new Error("employee has active work"))).toContain("进行中的工作");
  });

  it("treats 204 pending as empty", async () => {
    const fetcher = createEmployeeConfigFetcher();
    const screen = await renderConfig(fetcher);
    await userEvent.click(screen.getByRole("tab", { name: "权限" }));
    await expect.element(screen.getByText("资源授权 · scope:resource 形式")).toBeVisible();
    expect(screen.getByText("有 1 条待审批的权限变更").query()).toBeNull();
  });

  it("disables submit when a pending permission change exists", async () => {
    const fetcher = createEmployeeConfigFetcher({
      permissionChange: jsonResponse({
        request_id: "req-1",
        status: "pending",
        risk_level: "high",
        created_at: "2026-08-14T00:00:00Z",
        requester_name: "张三",
        approver_name: "李四",
        current_permission_policy: { grants: ["database.read:dev_db"] },
        target_permission_policy: { grants: ["database.read:dev_db", "repo.read:main"] },
      }),
    });
    const screen = await renderConfig(fetcher, "permission");
    await expect.element(screen.getByText(/有 1 条待审批的权限变更/)).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "提交审批" })).toBeDisabled();
  });
});
