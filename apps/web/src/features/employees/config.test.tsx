import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeConfigView } from "./config";
import type { McpServer, UserCredential } from "@/lib/api/capabilities";
import type { WorkspaceFile } from "@/lib/api/employees";
import type { EffectiveEmployeeSkill, Skill } from "@/lib/api/skills";

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

vi.mock("@monaco-editor/react", () => ({
  default: ({ value, onChange }: { value?: string; onChange?: (value?: string) => void }) => (
    <textarea
      aria-label="Workspace file editor"
      value={value ?? ""}
      onChange={(event) => onChange?.(event.currentTarget.value)}
    />
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

function latestRequestBody(fetcher: ReturnType<typeof createEmployeeConfigFetcher>, path: string, method: string) {
  const call = [...fetcher.mock.calls].reverse().find(([input, init]) => {
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

function skillFixture(id: string, name: string): Skill {
  return {
    id,
    tenant_id: "tenant-1",
    slug: name,
    name,
    description: `${name} 技能`,
    version: "v1.0.0",
    source: "internal_market",
    risk_level: "low",
    status: "installed",
    icon_key: "boxes",
    color_token: "cyan",
    tags: ["自动化"],
    files: [{ path: "SKILL.md", file_type: "file", content: `# ${name}`, size_bytes: name.length + 2 }],
    team_bindings: [],
    agent_bindings: [],
  };
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
    await expect.element(screen.getByText("配置员工技能、策略和输出契约")).toBeVisible();
    await userEvent.click(screen.getByRole("tab", { name: "高级配置" }));
    await expect.element(screen.getByLabelText("Role Profile (JSON)")).toBeVisible();
    await expect.element(screen.getByLabelText("Constitution Addendum (JSON)")).toBeVisible();
  });

  it("submits changed advanced JSON config revision on save", async () => {
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

    await userEvent.click(screen.getByRole("tab", { name: "高级配置" }));
    await expect.element(screen.getByRole("button", { name: /保存配置/ })).toBeVisible();
    await userEvent.fill(screen.getByLabelText("Role Profile (JSON)"), '{"title":"analyst"}');
    await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));
    await expect.element(screen.getByText("配置已保存")).toBeVisible();

    const body = requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/config-revisions`, "POST");
    expect(body).toEqual({
      role_profile: { title: "analyst" },
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

    await userEvent.click(screen.getByRole("tab", { name: "高级配置" }));
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

    await userEvent.click(screen.getByRole("tab", { name: "高级配置" }));
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

    await userEvent.click(screen.getByRole("tab", { name: "高级配置" }));
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

    await userEvent.click(screen.getByRole("tab", { name: "高级配置" }));
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

    await userEvent.click(screen.getByRole("tab", { name: "高级配置" }));
    await userEvent.fill(screen.getByLabelText("Role Profile (JSON)"), '{"title":');
    await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));

    await expect.element(screen.getByText("Role Profile 必须是有效 JSON")).toBeVisible();
    const postCall = fetcher.mock.calls.find(
      ([input, init]) => requestUrl(input).includes("/config-revisions") && init?.method === "POST",
    );
    expect(postCall).toBeUndefined();
  });

  it("edits workspace files and manages personal capabilities with inherited preview", async () => {
    const workspaceFile = {
      id: "workspace-file-1",
      team_id: employee.team_id,
      path: "AGENTS.md",
      file_role: "entrypoint",
      mime_type: "text/markdown",
      sync_policy: "auto",
      status: "active",
      current_revision_id: "revision-1",
      revision_number: 1,
      content: "# 原则",
      content_hash: "sha-old",
      size_bytes: 8,
      storage_backend: "db",
      updated_at: "2026-06-10T00:00:00Z",
    } satisfies WorkspaceFile;
    const diagnose = skillFixture("skill-diagnose", "diagnose");
    const personal = skillFixture("skill-personal", "context-pack");
    const sqlReview = skillFixture("skill-extra", "sql-review");
    const effectiveSkills = [
      { skill: diagnose, read_only: true, inherited: true, source_scope: "team" },
      { skill: personal, read_only: false, inherited: false, source_scope: "employee" },
    ] satisfies EffectiveEmployeeSkill[];
    const credentials = [
      {
        id: "credential-1",
        tenant_id: "tenant-1",
        user_id: "user-1",
        name: "ops-token",
        credential_type: "mcp_token",
        last_four: "7890",
        status: "active",
      },
    ] satisfies UserCredential[];
    const inheritedMcp = [
      {
        id: "mcp-team",
        tenant_id: "tenant-1",
        team_id: employee.team_id,
        name: "team-observe",
        url: "https://team.example.com/mcp",
        status: "active",
        source_scope: "team",
        inherited: true,
      },
    ] satisfies McpServer[];
    const fetcher = createEmployeeConfigFetcher({
      extraRoutes: {
        [`GET /api/v1/digital-employees/${employee.id}/workspace-files`]: [workspaceFile],
        [`PUT /api/v1/digital-employees/${employee.id}/workspace-files`]: {
          ...workspaceFile,
          content: "# 新原则",
          content_hash: "sha-new",
          current_revision_id: "revision-2",
          revision_number: 2,
        },
        [`GET /api/v1/digital-employees/${employee.id}/skills`]: effectiveSkills,
        "GET /api/v1/skills": [sqlReview],
        "GET /api/v1/user-credentials?credential_type=mcp_token": credentials,
        [`GET /api/v1/digital-employees/${employee.id}/mcp-bindings`]: [],
        [`GET /api/v1/digital-employees/${employee.id}/effective-mcp-servers`]: inheritedMcp,
        [`POST /api/v1/digital-employees/${employee.id}/skills`]: sqlReview,
        [`POST /api/v1/digital-employees/${employee.id}/mcp-bindings`]: {
          id: "mcp-personal",
          tenant_id: "tenant-1",
          digital_employee_id: employee.id,
          name: "个人检索 MCP",
          url: "https://personal.example.com/mcp",
          credential_id: "credential-1",
          credential_name: "ops-token",
          credential_type: "mcp_token",
          credential_last_four: "7890",
          status: "active",
          source_scope: "employee",
          inherited: false,
        } satisfies McpServer,
      },
    });

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await userEvent.click(screen.getByRole("tab", { name: "宪法/人格" }));
    await expect.element(screen.getByRole("button", { name: "AGENTS.md" })).toBeVisible();
    await userEvent.fill(screen.getByLabelText("Workspace file editor"), "# 新原则");
    await userEvent.click(screen.getByRole("button", { name: "保存文件" }));
    await expect
      .poll(() => requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/workspace-files`, "PUT"))
      .toEqual({ path: "AGENTS.md", content: "# 新原则" });

    await userEvent.click(screen.getByRole("tab", { name: "能力设置" }));
    await expect.element(screen.getByText("diagnose")).toBeVisible();
    await expect.element(screen.getByText("团队继承").first()).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "移除 diagnose" })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: "安装 sql-review" }));
    await expect
      .poll(() => requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/skills`, "POST"))
      .toEqual({ skill_id: "skill-extra" });

    await userEvent.fill(screen.getByRole("textbox", { name: "个人 MCP 名称" }), " 个人检索 MCP ");
    await userEvent.fill(
      screen.getByRole("textbox", { name: "个人 MCP URL" }),
      " https://personal.example.com/mcp ",
    );
    await userEvent.click(screen.getByRole("combobox", { name: "个人 MCP 凭据" }));
    await userEvent.click(screen.getByRole("option", { name: "ops-token ****7890" }));
    await userEvent.click(screen.getByRole("button", { name: "添加个人 MCP" }));
    await expect
      .poll(() => requestBody(fetcher, `/api/v1/digital-employees/${employee.id}/mcp-bindings`, "POST"))
      .toEqual({
        name: "个人检索 MCP",
        url: "https://personal.example.com/mcp",
        credential_id: "credential-1",
      });

    await userEvent.fill(screen.getByRole("textbox", { name: "个人 MCP 名称" }), "无凭据 MCP");
    await userEvent.fill(
      screen.getByRole("textbox", { name: "个人 MCP URL" }),
      "https://public.example.com/mcp",
    );
    await userEvent.click(screen.getByRole("button", { name: "添加个人 MCP" }));
    await expect
      .poll(() => latestRequestBody(fetcher, `/api/v1/digital-employees/${employee.id}/mcp-bindings`, "POST"))
      .toEqual({
        name: "无凭据 MCP",
        url: "https://public.example.com/mcp",
      });
  });

  it("preserves unsaved workspace file drafts when switching files and creating a local file", async () => {
    const workspaceFiles = [
      {
        id: "workspace-agents",
        team_id: employee.team_id,
        path: "AGENTS.md",
        file_role: "entrypoint",
        mime_type: "text/markdown",
        sync_policy: "auto",
        status: "active",
        current_revision_id: "revision-agents",
        revision_number: 1,
        content: "# 原则",
        content_hash: "sha-agents",
        size_bytes: 8,
        storage_backend: "db",
      },
      {
        id: "workspace-notes",
        team_id: employee.team_id,
        path: "NOTES.md",
        file_role: "supporting_doc",
        mime_type: "text/markdown",
        sync_policy: "auto",
        status: "active",
        current_revision_id: "revision-notes",
        revision_number: 1,
        content: "# 备注",
        content_hash: "sha-notes",
        size_bytes: 8,
        storage_backend: "db",
      },
    ] satisfies WorkspaceFile[];
    const fetcher = createEmployeeConfigFetcher({
      extraRoutes: {
        [`GET /api/v1/digital-employees/${employee.id}/workspace-files`]: workspaceFiles,
      },
    });

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await userEvent.click(screen.getByRole("tab", { name: "宪法/人格" }));
    await expect.element(screen.getByRole("button", { name: "AGENTS.md" })).toBeVisible();
    await userEvent.fill(screen.getByLabelText("Workspace file editor"), "# AGENTS 草稿");

    await userEvent.click(screen.getByRole("button", { name: "NOTES.md" }));
    await expect.element(screen.getByLabelText("Workspace file editor")).toHaveValue("# 备注");
    await userEvent.fill(screen.getByLabelText("Workspace file editor"), "# NOTES 草稿");

    await userEvent.click(screen.getByRole("button", { name: "AGENTS.md" }));
    await expect.element(screen.getByLabelText("Workspace file editor")).toHaveValue("# AGENTS 草稿");

    await userEvent.fill(screen.getByRole("textbox", { name: "新文件路径" }), "LOCAL.md");
    await userEvent.click(screen.getByRole("button", { name: "新建文件" }));
    await expect.element(screen.getByLabelText("Workspace file editor")).toHaveValue("");
    await expect.element(screen.getByText("未保存")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "AGENTS.md" }));
    await expect.element(screen.getByLabelText("Workspace file editor")).toHaveValue("# AGENTS 草稿");
  });

  it("refreshes clean workspace file draft from server data without enabling save", async () => {
    const queryClient = createQueryClient();
    const workspaceFiles = [
      {
        id: "workspace-agents",
        team_id: employee.team_id,
        path: "AGENTS.md",
        file_role: "entrypoint",
        mime_type: "text/markdown",
        sync_policy: "auto",
        status: "active",
        current_revision_id: "revision-agents",
        revision_number: 1,
        content: "# 旧原则",
        content_hash: "sha-old",
        size_bytes: 12,
        storage_backend: "db",
      },
    ] satisfies WorkspaceFile[];
    const fetcher = createEmployeeConfigFetcher({
      extraRoutes: {
        [`GET /api/v1/digital-employees/${employee.id}/workspace-files`]: workspaceFiles,
      },
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

    await userEvent.click(screen.getByRole("tab", { name: "宪法/人格" }));
    await expect.element(screen.getByRole("button", { name: "AGENTS.md" })).toBeVisible();
    await expect.element(screen.getByLabelText("Workspace file editor")).toHaveValue("# 旧原则");
    await expect.element(screen.getByRole("button", { name: "保存文件" })).toBeDisabled();

    workspaceFiles[0] = {
      ...workspaceFiles[0],
      content: "# 新原则",
      content_hash: "sha-new",
      current_revision_id: "revision-agents-2",
      revision_number: 2,
      size_bytes: 12,
    };
    await queryClient.invalidateQueries({ queryKey: ["employee-workspace-files", employee.id] });

    await expect.element(screen.getByLabelText("Workspace file editor")).toHaveValue("# 新原则");
    await expect.element(screen.getByRole("button", { name: "保存文件" })).toBeDisabled();
  });

  it("removing a personal duplicate skill keeps inherited team capability visible", async () => {
    const diagnose = skillFixture("skill-diagnose", "diagnose");
    const effectiveSkills = [
      { skill: diagnose, read_only: true, inherited: true, source_scope: "team" },
      { skill: diagnose, read_only: false, inherited: false, source_scope: "employee" },
    ] satisfies EffectiveEmployeeSkill[];
    const fetcher = createEmployeeConfigFetcher({
      extraRoutes: {
        [`GET /api/v1/digital-employees/${employee.id}/skills`]: effectiveSkills,
        "GET /api/v1/skills": [],
        "GET /api/v1/user-credentials?credential_type=mcp_token": [],
        [`GET /api/v1/digital-employees/${employee.id}/mcp-bindings`]: [],
        [`GET /api/v1/digital-employees/${employee.id}/effective-mcp-servers`]: [],
        [`DELETE /api/v1/digital-employees/${employee.id}/skills/skill-diagnose`]: {},
      },
    });

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <EmployeeConfigView
          apiBaseUrl="http://localhost:8080"
          employeeId={employee.id}
          fetcher={fetcher}
        />
      </QueryClientProvider>,
    );

    await userEvent.click(screen.getByRole("tab", { name: "能力设置" }));
    await expect.element(screen.getByRole("button", { name: "移除 diagnose" }).first()).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "移除 diagnose" }).last());
    await expect
      .poll(() => hasRequest(fetcher, `/api/v1/digital-employees/${employee.id}/skills/skill-diagnose`, "DELETE"))
      .toBe(true);
    await expect.element(screen.getByRole("button", { name: "移除 diagnose" }).first()).toBeDisabled();
    await expect.element(screen.getByText("团队继承")).toBeVisible();
  });
});
