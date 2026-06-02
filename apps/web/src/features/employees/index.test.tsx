import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
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
          },
        ]),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        },
      );
    }
    if (
      url.pathname ===
        "/api/v1/digital-employees/11111111-1111-4111-8111-111111111111/execution-instance" &&
      (init?.method ?? "GET") === "GET"
    ) {
      return new Response(
        JSON.stringify({
          id: "22222222-2222-4222-8222-222222222222",
          digital_employee_id: "11111111-1111-4111-8111-111111111111",
          runtime_node_id: "33333333-3333-4333-8333-333333333333",
          provider_type: "codex",
          status: "ready",
        }),
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

function createEmployeePreviewFetcher({
  previewStatus = 200,
  teamConfigStatus = 200,
  teams = [{ id: "team-1", name: "运维团队", slug: "ops", status: "active" }],
  teamsStatus = 200,
}: {
  previewStatus?: number;
  teamConfigStatus?: number;
  teams?: Array<{ id: string; name: string; slug: string; status: string }>;
  teamsStatus?: number;
} = {}) {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return new Response(JSON.stringify([]), {
        headers: { "content-type": "application/json" },
        status: 200,
      });
    }

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return new Response(JSON.stringify(teamsStatus === 200 ? teams : { error: "teams failed" }), {
        headers: { "content-type": "application/json" },
        status: teamsStatus,
      });
    }

    if (url.pathname === "/api/v1/teams/team-1/config-revisions/current" && method === "GET") {
      if (teamConfigStatus === 404) {
        return new Response(JSON.stringify({ error: "not found" }), {
          headers: { "content-type": "application/json" },
          status: 404,
        });
      }

      return new Response(
        JSON.stringify({
          id: "team-config-rev-1",
          team_id: "team-1",
          revision_number: 1,
          constitution: { hard_rules: ["必须记录审计"] },
          capability_policy: { allowed_skills: ["database"] },
          context_policy: { allowed_sources: ["runbook"] },
          approval_policy: { required: true },
          artifact_contract: {},
          internal_collaboration_policy: {},
          runtime_scope_policy: {},
          status: "active",
        }),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        },
      );
    }

    if (url.pathname === "/api/v1/digital-employees" && method === "POST") {
      expect(JSON.parse(String(init?.body))).toEqual({
        name: "数据库运维员工",
        role: "database_operator",
        team_id: "team-1",
      });

      return new Response(
        JSON.stringify({
          id: "employee-1",
          team_id: "team-1",
          name: "数据库运维员工",
          role: "database_operator",
          status: "draft",
        }),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        },
      );
    }

    if (url.pathname === "/api/v1/digital-employees/employee-1/config-revisions" && method === "POST") {
      expect(JSON.parse(String(init?.body))).toEqual(
        expect.objectContaining({
          role_profile: { role: "database_operator" },
          capability_selection: { enabled_skills: [] },
          status: "draft",
        }),
      );

      return new Response(
        JSON.stringify({
          id: "employee-config-rev-1",
          digital_employee_id: "employee-1",
          revision_number: 1,
          role_profile: { role: "database_operator" },
          constitution_addendum: {},
          capability_selection: { enabled_skills: [] },
          context_policy_override: {},
          approval_policy_override: {},
          output_contract_addendum: {},
          status: "draft",
        }),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        },
      );
    }

    if (url.pathname === "/api/v1/digital-employees/employee-1/effective-configs/preview" && method === "POST") {
      expect(JSON.parse(String(init?.body))).toEqual({
        team_config: { id: "team-config-rev-1" },
        employee_config: { id: "employee-config-rev-1" },
      });

      if (previewStatus !== 200) {
        return new Response(JSON.stringify({ error: "preview failed" }), {
          headers: { "content-type": "application/json" },
          status: previewStatus,
        });
      }

      return new Response(
        JSON.stringify({
          team_config_revision_id: "team-config-rev-1",
          employee_config_revision_id: "employee-config-rev-1",
          effective_config: {},
          validation: {
            blocking_errors: [],
            warnings: [],
          },
        }),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        },
      );
    }

    return new Response(JSON.stringify({ error: `unhandled ${method} ${url.pathname}` }), {
      headers: { "content-type": "application/json" },
      status: 404,
    });
  }) as unknown as typeof fetch;

  return fetcher;
}

function countFetchCalls(fetcher: typeof fetch, path: string, method: string) {
  return (
    fetcher as unknown as {
      mock: { calls: Array<[RequestInfo | URL, RequestInit | undefined]> };
    }
  ).mock.calls.filter(([input, init]) => {
    const url = new URL(String(input));

    return url.pathname === path && (init?.method ?? "GET") === method;
  }).length;
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
    const fetcher = createEmployeesFetcher();
    const screen = await renderEmployeesView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "数字员工" })).toBeVisible();
    await expect.element(screen.getByText("需求分析员工")).toBeVisible();
    await expect.element(screen.getByText("负责需求拆解和交付风险识别")).toBeVisible();
    await expect.element(screen.getByText("codex · ready")).toBeVisible();
    await expect.element(screen.getByText("33333333-3333-4333-8333-333333333333")).toBeVisible();
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/11111111-1111-4111-8111-111111111111/execution-instance",
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      }),
    );
  });

  it("creates a draft digital employee and previews effective config", async () => {
    const fetcher = createEmployeePreviewFetcher();
    const screen = await renderEmployeesView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "创建数字员工" }));
    await userEvent.fill(screen.getByLabelText("名称"), "  数据库运维员工  ");
    await userEvent.fill(screen.getByLabelText("角色"), "  database_operator  ");
    await expect.element(screen.getByLabelText("归属团队")).toHaveValue("team-1");

    await userEvent.click(screen.getByRole("button", { name: "预览生效配置" }));

    await expect.element(screen.getByText("可提交负责人确认")).toBeVisible();
  });

  it("shows feedback when current team config is missing", async () => {
    const fetcher = createEmployeePreviewFetcher({ teamConfigStatus: 404 });
    const screen = await renderEmployeesView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "创建数字员工" }));
    await userEvent.fill(screen.getByLabelText("名称"), "数据库运维员工");
    await userEvent.fill(screen.getByLabelText("角色"), "database_operator");
    await userEvent.click(screen.getByRole("button", { name: "预览生效配置" }));

    await expect.element(screen.getByText("生效配置预览失败")).toBeVisible();
  });

  it("keeps the created draft visible after preview fails without creating duplicates", async () => {
    const fetcher = createEmployeePreviewFetcher({ previewStatus: 500 });
    const screen = await renderEmployeesView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "创建数字员工" }));
    await userEvent.fill(screen.getByLabelText("名称"), "数据库运维员工");
    await userEvent.fill(screen.getByLabelText("角色"), "database_operator");
    await userEvent.click(screen.getByRole("button", { name: "预览生效配置" }));

    await expect.element(screen.getByText("草稿已创建但预览失败")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "预览生效配置" })).toBeDisabled();

    expect(countFetchCalls(fetcher, "/api/v1/digital-employees", "POST")).toBe(1);
    expect(countFetchCalls(fetcher, "/api/v1/digital-employees", "GET")).toBeGreaterThanOrEqual(2);
  });

  it("shows feedback when teams cannot be loaded", async () => {
    const fetcher = createEmployeePreviewFetcher({ teamsStatus: 500 });
    const screen = await renderEmployeesView(fetcher);

    await expect.element(screen.getByText("团队列表加载失败")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "创建数字员工" })).toBeDisabled();
  });

  it("shows feedback when no team is available", async () => {
    const fetcher = createEmployeePreviewFetcher({ teams: [] });
    const screen = await renderEmployeesView(fetcher);

    await expect.element(screen.getByText("暂无团队，需先创建团队并配置治理版本")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "创建数字员工" })).toBeDisabled();
  });
});
