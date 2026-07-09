import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import type { EmployeeTemplate } from "@/lib/api/employee-templates";
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

function buildTemplate(overrides: Partial<EmployeeTemplate> = {}): EmployeeTemplate {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    tenant_id: "tenant-1",
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
    metadata: {},
    status: "active",
    is_system: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(body === null ? null : JSON.stringify(body), {
    headers: body === null ? {} : { "content-type": "application/json" },
    status,
  });
}

type FetcherOptions = {
  initial?: EmployeeTemplate[];
  failStatus?: boolean;
  failDelete?: boolean;
};

const TEMPLATES_PATH = "/api/v1/digital-employee-templates";

function createTemplatesFetcher({ initial = [], failStatus = false, failDelete = false }: FetcherOptions = {}) {
  let templates = [...initial];
  let nextId = 900;

  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const path = url.pathname;
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : undefined;

    if (path === TEMPLATES_PATH && method === "GET") {
      return jsonResponse(templates);
    }

    if (path === TEMPLATES_PATH && method === "POST") {
      const created = buildTemplate({
        id: `generated-${nextId++}`,
        type: String(body?.type ?? ""),
        label: String(body?.label ?? ""),
        description: String(body?.description ?? ""),
        default_role: String(body?.default_role ?? ""),
        recommended_skills: (body?.recommended_skills as string[]) ?? [],
        recommended_mcp_servers: (body?.recommended_mcp_servers as string[]) ?? [],
        recommended_provider_types: (body?.recommended_provider_types as string[]) ?? [],
        default_capability_selection: (body?.default_capability_selection as Record<string, unknown>) ?? {},
        default_context_policy_override: (body?.default_context_policy_override as Record<string, unknown>) ?? {},
        default_approval_policy: (body?.default_approval_policy as Record<string, unknown>) ?? {},
        status: "active",
        is_system: false,
      });
      templates = [...templates, created];
      return jsonResponse(created, 201);
    }

    const statusMatch = path.match(/^\/api\/v1\/digital-employee-templates\/([^/]+)\/status$/);
    if (statusMatch && method === "PATCH") {
      if (failStatus) {
        return jsonResponse({ error: "状态更新失败" }, 500);
      }
      const id = statusMatch[1];
      const existing = templates.find((template) => template.id === id);
      if (!existing) return jsonResponse({ error: "not found" }, 404);
      const updated = { ...existing, status: body?.status as EmployeeTemplate["status"] };
      templates = templates.map((template) => (template.id === id ? updated : template));
      return jsonResponse(updated);
    }

    const itemMatch = path.match(/^\/api\/v1\/digital-employee-templates\/([^/]+)$/);
    if (itemMatch && method === "PATCH") {
      const id = itemMatch[1];
      const existing = templates.find((template) => template.id === id);
      if (!existing) return jsonResponse({ error: "not found" }, 404);
      const updated = { ...existing, ...body, updated_at: "2026-01-03T00:00:00Z" } as EmployeeTemplate;
      templates = templates.map((template) => (template.id === id ? updated : template));
      return jsonResponse(updated);
    }

    if (itemMatch && method === "DELETE") {
      if (failDelete) {
        return jsonResponse({ error: "删除失败" }, 500);
      }
      const id = itemMatch[1];
      templates = templates.filter((template) => template.id !== id);
      return jsonResponse(null, 204);
    }

    return jsonResponse({ error: `unhandled ${method} ${path}` }, 404);
  }) as unknown as typeof fetch;
}

function fetchCalls(fetcher: typeof fetch) {
  return (
    fetcher as unknown as {
      mock: { calls: Array<[RequestInfo | URL, RequestInit | undefined]> };
    }
  ).mock.calls;
}

function findCall(fetcher: typeof fetch, method: string, pathname: string) {
  return fetchCalls(fetcher).find(([input, init]) => {
    const url = new URL(String(input));
    return url.pathname === pathname && (init?.method ?? "GET") === method;
  });
}

function callBody(call: ReturnType<typeof findCall>) {
  if (!call) throw new Error("expected a matching fetch call");
  const [, init] = call;
  return init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : undefined;
}

async function renderTemplateListView(fetcher: typeof fetch) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <TemplateListView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

async function renderTemplateDetailView(templateType: string, fetcher: typeof fetch) {
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
  it("renders template rows with status pill, 内置 badge, and row actions", async () => {
    const systemTemplate = buildTemplate();
    const customTemplate = buildTemplate({
      id: "22222222-2222-4222-8222-222222222222",
      type: "frontend_engineer",
      label: "前端开发",
      is_system: false,
      status: "disabled",
    });
    const fetcher = createTemplatesFetcher({ initial: [systemTemplate, customTemplate] });
    const screen = await renderTemplateListView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "数字员工模板" })).toBeVisible();
    await expect.element(screen.getByText("数据库管理员")).toBeVisible();
    await expect.element(screen.getByText("前端开发")).toBeVisible();

    // Only the system template carries the built-in badge.
    expect(screen.getByText("内置").all()).toHaveLength(1);

    await expect.element(screen.getByText("已启用")).toBeVisible();
    await expect.element(screen.getByText("已禁用")).toBeVisible();

    await expect
      .element(screen.getByRole("link", { name: "查看数据库管理员模板详情" }))
      .toHaveAttribute("href", "/employees/templates/database_admin");

    expect(screen.getByRole("button", { name: "配置" }).all()).toHaveLength(2);
    expect(screen.getByRole("button", { name: "删除" }).all()).toHaveLength(2);
    await expect.element(screen.getByRole("button", { name: "禁用" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "启用" })).toBeVisible();
  });

  it("renders an inviting empty state with a 新建模板 call to action", async () => {
    const fetcher = createTemplatesFetcher({ initial: [] });
    const screen = await renderTemplateListView(fetcher);

    await expect.element(screen.getByText("还没有数字员工模板")).toBeVisible();
    await expect
      .element(screen.getByText("点击“新建模板”创建第一个模板，定义默认角色、能力建议和治理策略默认值。"))
      .toBeVisible();
    expect(document.body.textContent).not.toContain("出错");
    expect(document.body.textContent).not.toContain("失败");

    // One 新建模板 button lives in the page header, one in the empty-state CTA.
    expect(screen.getByRole("button", { name: "新建模板" }).all()).toHaveLength(2);
  });

  it("creates a template: filling type + label and saving POSTs the expected body, then closes and refetches", async () => {
    const existing = buildTemplate();
    const fetcher = createTemplatesFetcher({ initial: [existing] });
    const screen = await renderTemplateListView(fetcher);

    await expect.element(screen.getByText("数据库管理员")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "新建模板" }));

    const dialog = screen.getByRole("dialog", { name: "新建模板" });
    await expect.element(dialog).toBeVisible();

    // Label/Input pairs in TemplateFormDialog are not wired with htmlFor/id,
    // so fields are addressed by placeholder (type) or position (label).
    await userEvent.fill(dialog.getByPlaceholder("例如 custom_reviewer"), "custom_reviewer");
    await userEvent.fill(dialog.getByRole("textbox").nth(1), "自定义评审官");

    await expect.element(dialog.getByRole("button", { name: "保存" })).not.toBeDisabled();
    await userEvent.click(dialog.getByRole("button", { name: "保存" }));

    await expect.element(screen.getByRole("dialog", { name: "新建模板" })).not.toBeInTheDocument();
    await expect.element(screen.getByText("自定义评审官")).toBeVisible();

    const createCall = findCall(fetcher, "POST", TEMPLATES_PATH);
    expect(callBody(createCall)).toEqual({
      label: "自定义评审官",
      description: "",
      default_role: "",
      recommended_skills: [],
      recommended_mcp_servers: [],
      recommended_provider_types: [],
      default_capability_selection: {},
      default_context_policy_override: {},
      default_approval_policy: {},
      type: "custom_reviewer",
    });
  });

  it("configures an existing template: edit dialog is pre-filled, type is not editable, and PATCH omits type", async () => {
    const existing = buildTemplate();
    const fetcher = createTemplatesFetcher({ initial: [existing] });
    const screen = await renderTemplateListView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "配置" }));

    const dialog = screen.getByRole("dialog", { name: `配置模板：${existing.label}` });
    await expect.element(dialog).toBeVisible();
    await expect.element(dialog.getByText("模板标识（type）")).not.toBeInTheDocument();

    const nameField = dialog.getByRole("textbox").nth(0);
    await expect.element(nameField).toHaveValue(existing.label);

    await userEvent.fill(nameField, "数据库管理员（更新）");
    await userEvent.click(dialog.getByRole("button", { name: "保存" }));

    await expect.element(screen.getByRole("dialog", { name: `配置模板：${existing.label}` })).not.toBeInTheDocument();
    await expect.element(screen.getByText("数据库管理员（更新）")).toBeVisible();

    const patchCall = findCall(fetcher, "PATCH", `${TEMPLATES_PATH}/${existing.id}`);
    const body = callBody(patchCall);
    expect(body).not.toBeUndefined();
    expect(body).not.toHaveProperty("type");
    expect(body).toMatchObject({
      label: "数据库管理员（更新）",
      description: existing.description,
      default_role: existing.default_role,
      recommended_skills: existing.recommended_skills,
      recommended_mcp_servers: existing.recommended_mcp_servers,
      recommended_provider_types: existing.recommended_provider_types,
    });
  });

  it("toggles status: clicking the row action PATCHes the opposite status and the pill flips", async () => {
    const existing = buildTemplate({ status: "active" });
    const fetcher = createTemplatesFetcher({ initial: [existing] });
    const screen = await renderTemplateListView(fetcher);

    await expect.element(screen.getByText("已启用")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "禁用" }));

    await expect.element(screen.getByText("已禁用")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "启用" })).toBeVisible();

    const statusCall = findCall(fetcher, "PATCH", `${TEMPLATES_PATH}/${existing.id}/status`);
    expect(callBody(statusCall)).toEqual({ status: "disabled" });
  });

  it("deletes a template: confirming in the ConfirmDialog DELETEs it and removes the row", async () => {
    const existing = buildTemplate();
    const fetcher = createTemplatesFetcher({ initial: [existing] });
    const screen = await renderTemplateListView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "删除" }));

    const confirmDialog = screen.getByRole("alertdialog", { name: "删除模板" });
    await expect.element(confirmDialog).toBeVisible();
    await expect.element(confirmDialog.getByText(/数据库管理员/)).toBeVisible();

    await userEvent.click(confirmDialog.getByRole("button", { name: "继续" }));

    await expect.element(screen.getByRole("alertdialog", { name: "删除模板" })).not.toBeInTheDocument();
    await expect.element(screen.getByText("还没有数字员工模板")).toBeVisible();

    expect(findCall(fetcher, "DELETE", `${TEMPLATES_PATH}/${existing.id}`)).not.toBeUndefined();
  });

  it("shows a dismissible error banner when a status toggle fails, and leaves the row unchanged", async () => {
    const existing = buildTemplate({ status: "active" });
    const fetcher = createTemplatesFetcher({ initial: [existing], failStatus: true });
    const screen = await renderTemplateListView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "禁用" }));

    await expect.element(screen.getByText(/状态更新失败/)).toBeVisible();
    // The row is untouched because the mutation failed server-side.
    await expect.element(screen.getByText("已启用")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "禁用" })).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "关闭" }));
    await expect.element(screen.getByText(/状态更新失败/)).not.toBeInTheDocument();
  });
});

describe("TemplateDetailView", () => {
  it("renders the selected template's fields and create entry", async () => {
    const existing = buildTemplate();
    const fetcher = createTemplatesFetcher({ initial: [existing] });
    const screen = await renderTemplateDetailView("database_admin", fetcher);

    await expect.element(screen.getByRole("heading", { name: "数据库管理员" })).toBeVisible();
    expect(document.body.textContent).toContain("database_admin");
    expect(document.body.textContent).toContain("database-troubleshooting");
    expect(screen.getByText("已启用").all()).toHaveLength(2);
    await expect
      .element(screen.getByRole("link", { name: "用此模板创建数字员工" }))
      .toHaveAttribute("href", "/employees/new?template=database_admin");
  });

  it("shows an unknown-template state with a return link", async () => {
    const existing = buildTemplate();
    const fetcher = createTemplatesFetcher({ initial: [existing] });
    const screen = await renderTemplateDetailView("unknown_template", fetcher);

    await expect.element(screen.getByText("模板不存在")).toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "返回模板管理" }))
      .toHaveAttribute("href", "/employees/templates");
  });
});
