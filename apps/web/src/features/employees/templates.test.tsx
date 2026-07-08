import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import { TemplateDetailView, TemplateListView } from "./templates";

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
  Link: ({
    children,
    params,
    search,
    to,
    ...props
  }: {
    children: ReactNode;
    params?: Record<string, string>;
    search?: Record<string, string>;
    to: string;
  } & React.AnchorHTMLAttributes<HTMLAnchorElement>) => {
    let href = to;
    if (params) {
      for (const [key, value] of Object.entries(params)) {
        href = href.replace(`$${key}`, encodeURIComponent(value));
      }
    }
    if (search) {
      const query = new URLSearchParams(search).toString();
      href = query ? `${href}?${query}` : href;
    }

    return <a href={href} {...props}>{children}</a>;
  },
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

const activeTeam = {
  id: "99999999-9999-4999-8999-999999999999",
  tenant_id: "tenant-1",
  slug: "data-platform",
  name: "数据平台团队",
  status: "active",
  member_count: 6,
  digital_employee_count: 2,
  capability_count: 4,
  governance_status: "active",
  pending_draft_count: 0,
  risk_summary: "常规",
};

function createOptionsFixture() {
  return {
    team_config: {
      id: "55555555-5555-4555-8555-555555555555",
      tenant_id: "tenant-1",
      team_id: activeTeam.id,
      constitution: { hard_rules: ["生产变更必须人工审批"] },
      skills: ["ui-implementation"],
      mcp_servers: ["postgres"],
    },
    employee_types: [
      {
        type: "database_admin",
        label: "数据库管理员",
        description: "负责数据库变更、备份、性能诊断和恢复验证",
        default_role: "database_admin",
        recommended_skills: ["database-troubleshooting"],
        recommended_mcp_servers: ["postgres"],
        recommended_provider_types: ["codex"],
        default_capability_selection: {
          enabled_skills: ["database-troubleshooting"],
          enabled_mcp_servers: ["postgres"],
          enabled_provider_types: ["codex"],
        },
        default_context_policy_override: { max_refs: 8 },
        default_approval_policy: { min_risk_for_human: "high" },
        metadata: { title: "数据库管理员" },
      },
      {
        type: "frontend_engineer",
        label: "前端开发",
        description: "负责控制台页面、交互状态和前端质量验证",
        default_role: "frontend_engineer",
        recommended_skills: ["ui-implementation"],
        recommended_mcp_servers: [],
        recommended_provider_types: ["codex"],
        default_capability_selection: {
          enabled_skills: ["ui-implementation"],
          enabled_mcp_servers: [],
          enabled_provider_types: ["codex"],
        },
        default_context_policy_override: { max_refs: 6 },
        default_approval_policy: { min_risk_for_human: "medium" },
        metadata: { title: "前端开发" },
      },
    ],
    capability_options: {
      provider_types: ["codex"],
      skills: ["database-troubleshooting", "ui-implementation"],
      mcp_servers: ["postgres"],
    },
    runtime_provider_options: [],
    creation_checks: [
      {
        key: "team_baseline",
        label: "团队继承基线",
        status: "passed",
        message: "skills 1 · MCP 1",
      },
    ],
    policy_defaults: {
      permission_policy: { mode: "least_privilege" },
      context_policy_override: { max_refs: 6 },
      approval_policy: { required: true },
      capability_selection: { source: "team_default" },
      runtime_selector: { strategy: "manual" },
      workspace_policy: { mode: "ephemeral" },
      session_policy: { mode: "reuse_latest" },
      metadata: { source: "team_config" },
    },
  };
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function createTemplatesFetcher() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse([activeTeam]);
    }

    if (url.pathname === "/api/v1/digital-employees/create-options" && method === "GET") {
      expect(url.searchParams.get("team_id")).toBe(activeTeam.id);
      return jsonResponse(createOptionsFixture());
    }

    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;
}

function createFailingTeamsFetcher() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse({ error: "teams failed" }, 500);
    }

    if (url.pathname === "/api/v1/digital-employees/create-options" && method === "GET") {
      return jsonResponse(createOptionsFixture());
    }

    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;
}

function fetchCalls(fetcher: typeof fetch) {
  return (
    fetcher as unknown as {
      mock: { calls: Array<[RequestInfo | URL, RequestInit | undefined]> };
    }
  ).mock.calls;
}

function createOptionsCallCount(fetcher: typeof fetch) {
  return fetchCalls(fetcher).filter(([input, init]) => {
    const url = new URL(String(input));

    return url.pathname === "/api/v1/digital-employees/create-options" && (init?.method ?? "GET") === "GET";
  }).length;
}

async function renderTemplateListView(fetcher = createTemplatesFetcher()) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <TemplateListView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

async function renderTemplateDetailView(templateType: string, fetcher = createTemplatesFetcher()) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <TemplateDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        templateType={templateType}
      />
    </QueryClientProvider>,
  );
}

describe("TemplateListView", () => {
  it("renders readonly built-in template rows with baseline status and detail links", async () => {
    const screen = await renderTemplateListView();

    await expect.element(screen.getByRole("heading", { name: "数字员工模板" })).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "内置模板目录" })).toBeVisible();
    await expect.element(screen.getByText("数据库管理员")).toBeVisible();
    await expect.element(screen.getByText("前端开发")).toBeVisible();
    await expect.element(screen.getByText("模板能力")).toBeVisible();
    await expect.element(screen.getByText("默认注入")).toBeVisible();
    await expect.element(screen.getByRole("cell", { name: "继承团队基线" }).first()).toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "查看数据库管理员模板详情" }))
      .toHaveAttribute("href", "/employees/templates/database_admin");
  });

  it("does not refetch create-options when retrying after teams still fails", async () => {
    const fetcher = createFailingTeamsFetcher();
    const screen = await renderTemplateListView(fetcher);

    await expect.element(screen.getByText("加载模板失败")).toBeVisible();
    expect(createOptionsCallCount(fetcher)).toBe(0);

    await userEvent.click(screen.getByRole("button", { name: "重试" }));
    await expect.element(screen.getByText("加载模板失败")).toBeVisible();

    expect(createOptionsCallCount(fetcher)).toBe(0);
  });
});

describe("TemplateDetailView", () => {
  it("renders the selected template fields, inherited baseline notes, and create entry", async () => {
    const screen = await renderTemplateDetailView("database_admin");

    await expect.element(screen.getByRole("heading", { name: "数据库管理员" })).toBeVisible();
    await expect.element(screen.getByText("默认角色")).toBeVisible();
    expect(document.body.textContent).toContain("database_admin");
    expect(document.body.textContent).toContain("database-troubleshooting");
    await expect.element(screen.getByText("默认注入")).toBeVisible();
    await expect.element(screen.getByText("Provider 1")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "继承基线" })).toBeVisible();
    expect(document.body.textContent).toContain("模板已定义的技能、MCP 与 Provider 能力");
    expect(document.body.textContent).not.toContain("模板推荐能力");
    await expect.element(screen.getByText("团队继承技能 1")).toBeVisible();
    await expect.element(screen.getByText("团队继承 MCP 1")).toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "用此模板创建数字员工" }))
      .toHaveAttribute("href", "/employees/new?template=database_admin");
  });

  it("shows an unknown-template state with a return link", async () => {
    const screen = await renderTemplateDetailView("unknown_template");

    await expect.element(screen.getByText("模板不存在")).toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "返回模板管理" }))
      .toHaveAttribute("href", "/employees/templates");
  });
});
