import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { SkillsView } from "@/features/skills";
import type { Skill } from "@/lib/api/skills";

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

vi.mock("@/components/ui/select", async () => {
  const React = await import("react");
  type SelectContextValue = {
    onValueChange?: (value: string) => void;
    value?: string;
  };
  const SelectContext = React.createContext<SelectContextValue>({});

  return {
    Select: ({
      children,
      disabled,
      onValueChange,
      value,
    }: {
      children: ReactNode;
      disabled?: boolean;
      onValueChange?: (value: string) => void;
      value?: string;
    }) => (
      <SelectContext value={{ onValueChange, value }}>
        <div aria-disabled={disabled || undefined} data-select-value={value}>
          {children}
        </div>
      </SelectContext>
    ),
    SelectContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    SelectGroup: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    SelectItem: ({ children, value }: { children: ReactNode; value: string }) => {
      const { onValueChange, value: selectedValue } = React.useContext(SelectContext);
      return (
        <button
          aria-pressed={selectedValue === value}
          onClick={() => onValueChange?.(value)}
          type="button"
        >
          {children}
        </button>
      );
    },
    SelectLabel: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    SelectScrollDownButton: ({ children }: { children?: ReactNode }) => <button type="button">{children}</button>,
    SelectScrollUpButton: ({ children }: { children?: ReactNode }) => <button type="button">{children}</button>,
    SelectSeparator: () => <hr />,
    SelectTrigger: ({
      "aria-label": ariaLabel,
      children,
    }: {
      "aria-label"?: string;
      children: ReactNode;
    }) => (
      <button aria-label={ariaLabel} type="button">
        {children}
      </button>
    ),
    SelectValue: ({ placeholder }: { placeholder?: string }) => <span>{placeholder}</span>,
  };
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...props }: { children: ReactNode; to: string }) => (
    <a {...props} data-router-link="true" href={to}>{children}</a>
  ),
}));

const skillsFixture = [
  {
    id: "skill-requirement",
    tenant_id: "tenant-1",
    slug: "requirement-clarifier",
    name: "需求澄清助手",
    description: "基于结构化提问流程，输出需求澄清记录",
    version: "1.3.0",
    source: "upload",
    risk_level: "low",
    icon_key: "stethoscope",
    color_token: "cyan",
    tags: ["需求分析", "沟通协作", "交付"],
    archive_object_ref: "s3://bucket/skills/requirement-clarifier.zip",
    archive_filename: "requirement-clarifier.zip",
    archive_size_bytes: 4096,
    archive_checksum_sha256: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
    archive_file_count: 3,
    created_by: "user-1",
    created_by_name: "开发管理员",
    team_bindings: [
      { team_id: "team-1", team_name: "平台工程" },
      { team_id: "team-2", team_name: "产品团队" },
    ],
    agent_bindings: [
      { agent_id: "agent-1", agent_name: "需求澄清 Agent", team_name: "产品团队", status: "enabled" },
      { agent_id: "agent-2", agent_name: "项目协调 Agent", team_name: "平台工程", status: "enabled" },
    ],
    runtime_dependencies: { tools: [], env: [] },
  },
  {
    id: "skill-api-doc",
    tenant_id: "tenant-1",
    slug: "api-doc-generator",
    name: "接口文档生成",
    description: "根据 OpenAPI 规范生成接口文档与示例",
    version: "1.2.1",
    source: "upload",
    risk_level: "medium",
    icon_key: "flask",
    color_token: "emerald",
    tags: ["文档生成", "API", "规范"],
    archive_object_ref: "s3://bucket/skills/api-doc.zip",
    archive_filename: "api-doc.zip",
    archive_size_bytes: 2048,
    archive_checksum_sha256: "def456abc123def456abc123def456abc123def456abc123def456abc123def4567",
    archive_file_count: 1,
    created_by: "user-1",
    created_by_name: "开发管理员",
    team_bindings: [],
    agent_bindings: [],
    runtime_dependencies: { tools: [], env: [] },
  },
  {
    id: "skill-incident-review",
    tenant_id: "tenant-1",
    slug: "incident-review",
    name: "生产事故复盘",
    description: "结构化复盘生产事故，输出根因与改进建议",
    version: "1.0.2",
    source: "upload",
    risk_level: "high",
    icon_key: "shield-check",
    color_token: "violet",
    tags: ["运维", "复盘", "高风险"],
    archive_object_ref: "s3://bucket/skills/incident-review.zip",
    archive_filename: "incident-review.zip",
    archive_size_bytes: 8192,
    archive_checksum_sha256: "fed456abc123def456abc123def456abc123def456abc123def456abc123def456",
    archive_file_count: 2,
    created_by: "user-1",
    created_by_name: "开发管理员",
    team_bindings: [
      { team_id: "team-1", team_name: "平台工程" },
      { team_id: "team-2", team_name: "安全治理" },
    ],
    agent_bindings: [
      { agent_id: "agent-3", agent_name: "运维复盘 Agent", team_name: "平台工程", status: "enabled" },
    ],
    runtime_dependencies: { tools: [], env: [] },
  },
] satisfies Skill[];

const teamsFixture = [
  { id: "team-1", tenant_id: "tenant-1", slug: "platform", name: "平台工程", status: "active", member_count: 12, digital_employee_count: 3, capability_count: 8, governance_status: "active", pending_draft_count: 0, risk_summary: "常规" },
  { id: "team-2", tenant_id: "tenant-1", slug: "security", name: "安全治理", status: "active", member_count: 5, digital_employee_count: 2, capability_count: 4, governance_status: "active", pending_draft_count: 0, risk_summary: "高风险需审批" },
];

const employeesFixture = [
  {
    id: "agent-1",
    tenant_id: "tenant-1",
    team_id: "team-2",
    owner_user_id: "user-1",
    employee_type: "analyst",
    name: "需求澄清 Agent",
    role: "需求澄清",
    status: "active",
    permission_policy: {},
    context_policy: {},
    approval_policy: {},
    risk_level: "low",
  },
  {
    id: "agent-2",
    tenant_id: "tenant-1",
    team_id: "team-1",
    owner_user_id: "user-1",
    employee_type: "coordinator",
    name: "项目协调 Agent",
    role: "项目协调",
    status: "ready",
    permission_policy: {},
    context_policy: {},
    approval_policy: {},
    risk_level: "medium",
  },
];

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

function createSkillsFetcher() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    if (url.pathname === "/api/v1/skills" && method === "GET") {
      const q = url.searchParams.get("q")?.trim() ?? "";
      const rows = q
        ? skillsFixture.filter((skill) => `${skill.name} ${skill.description} ${skill.tags.join(" ")}`.includes(q))
        : skillsFixture;
      return jsonResponse(rows);
    }
    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse(teamsFixture);
    }
    if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(employeesFixture);
    }
    if (url.pathname === "/api/v1/skills/skill-api-doc/install" && method === "POST") {
      return jsonResponse({
        skill_id: "skill-api-doc",
        target_scope: "employee",
        digital_employee_id: "agent-1",
        installed_count: 1,
        installations: [
          {
            digital_employee_id: "agent-1",
            employee_name: "需求澄清 Agent",
            provider_type: "codex",
            runtime_node_id: "runtime-1",
            node_id: "node-1",
            installed_path: "/var/superteam/skills/skill-api-doc",
            archive_checksum_sha256: "def456abc123",
            installed_at: "2026-06-24T08:00:00Z",
          },
        ],
      }, 201);
    }
    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 500);
  });
}

function createPendingSkillsFetcher() {
  let resolveSkills: (response: Response) => void = () => {};
  const pendingSkills = new Promise<Response>((resolve) => {
    resolveSkills = resolve;
  });
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    if (url.pathname === "/api/v1/skills") {
      return pendingSkills;
    }
    return jsonResponse([]);
  });
  return {
    fetcher,
    resolveSkills: () => resolveSkills(jsonResponse([])),
  };
}

function createFailingSkillsFetcher() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    if (url.pathname === "/api/v1/skills") {
      return jsonResponse({ error: "skills API offline" }, 503);
    }
    return jsonResponse([]);
  });
}

function createInstallConflictFetcher() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    if (url.pathname === "/api/v1/skills" && method === "GET") {
      return jsonResponse(skillsFixture);
    }
    if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(employeesFixture);
    }
    if (url.pathname === "/api/v1/skills/skill-api-doc/install" && method === "POST") {
      return jsonResponse({
        error: "skill_install_failed",
        phase: "preflight",
        message: "技能安装预检失败",
        blocked_targets: [
          {
            digital_employee_id: "agent-1",
            employee_name: "需求澄清 Agent",
            provider_type: "codex",
            runtime_node_id: "runtime-1",
            node_id: "node-a",
            reason_code: "runtime_disconnected",
            message: "运行节点未连接",
          },
        ],
      }, 409);
    }
    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 500);
  });
}

function countFetcherCalls(fetcher: ReturnType<typeof vi.fn>, pathname: string) {
  return fetcher.mock.calls.filter(([input]) => new URL(String(input)).pathname === pathname).length;
}

async function renderSkillsView(fetcher = createSkillsFetcher()) {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <SkillsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("SkillsView", () => {
  it("renders the skill market homepage with metric strip and action buttons", async () => {
    const screen = await renderSkillsView();

    await expect.element(screen.getByRole("heading", { name: "技能市场" })).toBeVisible();
    await expect.element(screen.getByText("发现、校验并安装技能到团队或数字员工")).toBeVisible();
    await expect.element(screen.getByRole("region", { name: "技能市场指标" }).getByText("可安装")).toBeVisible();
    await expect.element(screen.getByRole("region", { name: "技能市场指标" }).getByText("需审批")).toBeVisible();
    await expect.element(screen.getByRole("region", { name: "技能市场指标" }).getByText("团队绑定")).toBeVisible();
    await expect.element(screen.getByRole("region", { name: "技能市场指标" }).getByText("数字员工绑定")).toBeVisible();
    await expect.element(screen.getByRole("columnheader", { name: "技能" })).toBeVisible();
    await expect.element(screen.getByRole("columnheader", { name: "风险" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "查看详情 需求澄清助手" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "安装 接口文档生成" })).toBeVisible();
  });

  it("uses shared v3 controls for the primary action and view switcher", async () => {
    const screen = await renderSkillsView();

    expect(document.body.querySelector('[data-slot="v3-page-header"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-page-header"] [data-slot="v3-icon-tile"]')).not.toBeNull();

    const uploadLink = screen.getByRole("link", { name: "上传技能" });
    await expect.element(uploadLink).toHaveAttribute("data-slot", "v3-button");
    await expect.element(uploadLink).toHaveAttribute("data-variant", "primary");

    const viewSwitcher = document.body.querySelector('[data-slot="v3-segmented"][aria-label="技能视图"]');
    expect(viewSwitcher).not.toBeNull();
  });

  it("renders a v3 loading state inside the page shell", async () => {
    const pending = createPendingSkillsFetcher();
    const loadingScreen = await renderSkillsView(pending.fetcher);
    await expect.element(loadingScreen.getByText("加载技能数据…")).toBeVisible();
    expect(document.body.querySelector('[data-slot="v3-loading-state"]')).not.toBeNull();

    pending.resolveSkills();
    loadingScreen.unmount();
  });

  it("renders a v3 error state inside the page shell", async () => {
    const errorScreen = await renderSkillsView(createFailingSkillsFetcher());
    await expect.element(errorScreen.getByText("技能数据加载失败")).toBeVisible();
    await expect.element(errorScreen.getByText(/skills API offline/)).toBeVisible();
    expect(document.body.querySelector('[data-slot="v3-error-state"]')).not.toBeNull();
  });

  it("uses router navigation for the upload page without designing that flow here", async () => {
    const screen = await renderSkillsView();

    const uploadLink = screen.getByRole("link", { name: "上传技能" });
    await expect.element(uploadLink).toHaveAttribute("href", "/skills/upload");
    await expect.element(uploadLink).toHaveAttribute("data-router-link", "true");
    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
  });

  it("filters the market by search text while keeping the table chrome visible", async () => {
    const fetcher = createSkillsFetcher();
    const screen = await renderSkillsView(fetcher);

    await userEvent.fill(screen.getByRole("searchbox", { name: "搜索技能" }), "文档");

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills?q=%E6%96%87%E6%A1%A3",
      expect.objectContaining({ method: "GET" }),
    );
    await expect.element(screen.getByRole("columnheader", { name: "技能" })).toBeVisible();
    await expect.element(screen.getByText("接口文档生成")).toBeVisible();
  });

  it("opens the install dialog from the table install button", async () => {
    const screen = await renderSkillsView();

    await userEvent.click(screen.getByRole("button", { name: "安装 接口文档生成" }));

    const dialog = screen.getByRole("dialog", { name: "安装技能" });
    await expect.element(dialog).toBeVisible();
    await expect.element(dialog.getByText("接口文档生成 · 1.2.1")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "确认安装" })).toBeDisabled();
  });

  it("loads only employee targets on default open and fetches teams after team scope is selected", async () => {
    const fetcher = createSkillsFetcher();
    const screen = await renderSkillsView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "安装 接口文档生成" }));
    await expect.element(screen.getByRole("dialog", { name: "安装技能" })).toBeVisible();

    expect(countFetcherCalls(fetcher, "/api/v1/digital-employees")).toBe(1);
    expect(countFetcherCalls(fetcher, "/api/v1/teams")).toBe(0);

    await userEvent.click(screen.getByRole("radio", { name: /团队/ }));

    await expect.element(screen.getByRole("button", { name: "平台工程" })).toBeVisible();
    expect(countFetcherCalls(fetcher, "/api/v1/teams")).toBe(1);

    await userEvent.click(screen.getByRole("button", { name: "取消" }));
    await userEvent.click(screen.getByRole("button", { name: "安装 接口文档生成" }));
    await expect.element(screen.getByRole("dialog", { name: "安装技能" })).toBeVisible();

    expect(countFetcherCalls(fetcher, "/api/v1/teams")).toBe(1);

    await userEvent.click(screen.getByRole("radio", { name: /团队/ }));
    await expect.element(screen.getByRole("button", { name: "平台工程" })).toBeVisible();
    expect(countFetcherCalls(fetcher, "/api/v1/teams")).toBe(2);
  });

  it("submits the selected employee target and displays install success", async () => {
    const fetcher = createSkillsFetcher();
    const screen = await renderSkillsView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "安装 接口文档生成" }));
    await userEvent.click(screen.getByRole("button", { name: "需求澄清 Agent" }));
    await userEvent.click(screen.getByRole("button", { name: "确认安装" }));

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills/skill-api-doc/install",
      expect.objectContaining({
        body: JSON.stringify({
          target_scope: "employee",
          digital_employee_id: "agent-1",
          timeout_sec: 15,
        }),
        method: "POST",
      }),
    );
    await expect.element(screen.getByText("已安装到 1 个目标")).toBeVisible();
  });

  it("renders blocked target details when install returns a structured conflict", async () => {
    const fetcher = createInstallConflictFetcher();
    const screen = await renderSkillsView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "安装 接口文档生成" }));
    await userEvent.click(screen.getByRole("button", { name: "需求澄清 Agent" }));
    await userEvent.click(screen.getByRole("button", { name: "确认安装" }));

    const dialog = screen.getByRole("dialog", { name: "安装技能" });
    await expect.element(dialog.getByText("技能安装预检失败")).toBeVisible();
    const blockedTarget = screen.getByTestId("skill-install-blocked-target");
    await expect.element(blockedTarget.getByText("需求澄清 Agent")).toBeVisible();
    await expect.element(blockedTarget.getByText("runtime_disconnected")).toBeVisible();
    await expect.element(dialog.getByText("运行节点未连接")).toBeVisible();
    await expect.element(dialog.getByText("codex · node-a")).toBeVisible();
  });
});
