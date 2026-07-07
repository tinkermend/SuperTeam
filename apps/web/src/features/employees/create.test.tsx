import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { CreateEmployeeView } from "./create";

const navigate = vi.fn();
const search = vi.fn(() => ({}));

afterEach(() => {
  vi.restoreAllMocks();
});

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

vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...actual,
    Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
    useNavigate: () => navigate,
    useSearch: () => search(),
  };
});

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

const team = {
  id: "99999999-9999-4999-8999-999999999999",
  name: "数据平台团队",
  slug: "data-platform",
  status: "active",
};

const secondTeam = {
  id: "88888888-8888-4888-8888-888888888888",
  name: "安全运营团队",
  slug: "security-operations",
  status: "active",
};

const avatarAsset = {
  id: "engineer-m-01",
  label: "工程师头像 M01",
  gender: "male",
  age_range: "26-32",
  style: "photorealistic_2d",
  image_url: "/images/digital-employee-avatars/engineer-m-01.webp",
  thumbnail_url: "/images/digital-employee-avatars/engineer-m-01-256.webp",
  source: "ai_generated_internal_pack",
  license: "internal_product_asset",
  status: "active",
};

type RuntimeAvailabilityMode = "all" | "first-unavailable" | "none";
type ExpectedCreateBody = Record<string, unknown>;

function createOptionsFixture({
  runtimeAvailability = "all",
  runtimeCount = 1,
  sameRuntimeNodeProviders = false,
  includePolicyExcludedProvider = false,
  includeFrontendTemplate = false,
  includeCapabilityBoundaryBlock = false,
  capabilityBoundaryKey = "capability_policy",
  includeOtherBlockedCheck = false,
  allowedEmployeeTypes = ["database_admin"],
}: {
  runtimeAvailability?: RuntimeAvailabilityMode;
  runtimeCount?: 1 | 2;
  sameRuntimeNodeProviders?: boolean;
  includePolicyExcludedProvider?: boolean;
  includeFrontendTemplate?: boolean;
  includeCapabilityBoundaryBlock?: boolean;
  capabilityBoundaryKey?: "capability_policy" | "capability_boundary";
  includeOtherBlockedCheck?: boolean;
  allowedEmployeeTypes?: string[];
} = {}) {
  const firstRuntimeAvailable = runtimeAvailability === "all";
  const secondRuntimeAvailable = runtimeAvailability !== "none";
  const firstRuntimeOption = {
    runtime_node_id: "33333333-3333-4333-8333-333333333333",
    node_id: "runtime-a",
    runtime_name: "客户侧执行机 A",
    provider_type: "codex",
    runtime_status: firstRuntimeAvailable ? "online" : "offline",
    provider_status: firstRuntimeAvailable ? "healthy" : "unhealthy",
    health_status: firstRuntimeAvailable ? "healthy" : "unhealthy",
    current_load: 0,
    max_slots: 2,
    agent_home_dir: "/Users/wangpei/.codex",
    agent_home_dir_available: firstRuntimeAvailable,
    available: firstRuntimeAvailable,
    disabled_reason: firstRuntimeAvailable ? undefined : "runtime_session_inactive",
  };
  const sameNodeProviderOption = {
    runtime_node_id: "33333333-3333-4333-8333-333333333333",
    node_id: "runtime-a",
    runtime_name: "客户侧执行机 A",
    provider_type: "claude_code",
    runtime_status: firstRuntimeAvailable ? "online" : "offline",
    provider_status: firstRuntimeAvailable ? "healthy" : "unhealthy",
    health_status: firstRuntimeAvailable ? "healthy" : "unhealthy",
    current_load: 0,
    max_slots: 2,
    agent_home_dir: "/Users/wangpei/.claude",
    agent_home_dir_available: firstRuntimeAvailable,
    available: firstRuntimeAvailable,
    disabled_reason: firstRuntimeAvailable ? undefined : "runtime_session_inactive",
  };
  const secondRuntimeOption = {
    runtime_node_id: "44444444-4444-4444-8444-444444444444",
    node_id: "runtime-b",
    runtime_name: "客户侧执行机 B",
    provider_type: "codex",
    runtime_status: secondRuntimeAvailable ? "online" : "offline",
    provider_status: secondRuntimeAvailable ? "healthy" : "unhealthy",
    health_status: secondRuntimeAvailable ? "healthy" : "unhealthy",
    current_load: 1,
    max_slots: 2,
    agent_home_dir: "/Users/wangpei/.codex",
    agent_home_dir_available: secondRuntimeAvailable,
    available: secondRuntimeAvailable,
    disabled_reason: secondRuntimeAvailable ? undefined : "runtime_session_inactive",
  };
  const runtimeProviderOptions = [
    firstRuntimeOption,
    ...(sameRuntimeNodeProviders || includePolicyExcludedProvider ? [sameNodeProviderOption] : []),
    ...(!sameRuntimeNodeProviders && runtimeCount === 2 ? [secondRuntimeOption] : []),
  ];

  return {
    team_config: {
      id: "55555555-5555-4555-8555-555555555555",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: team.id,
      revision_number: 3,
      status: "approved",
      allowed_employee_types: allowedEmployeeTypes,
      allowed_provider_types: sameRuntimeNodeProviders ? ["codex", "claude_code"] : ["codex"],
      allowed_skills: ["incident-diagnosis", "sql-review"],
      allowed_mcp_servers: ["postgres"],
      allowed_external_capabilities: ["jira.search"],
      capability_policy: { mode: "allow_list" },
      context_policy: { max_refs: 8 },
      approval_policy: { required: true },
      artifact_contract: { required: ["summary"] },
      internal_collaboration_policy: { handoff: "structured" },
      runtime_scope_policy: { allowed_nodes: ["runtime-a"] },
    },
    employee_types: [
      {
        type: "database_admin",
        label: "数据库管理员",
        description: "负责数据库变更、备份、性能诊断和恢复验证",
        default_role: "database_admin",
        recommended_skills: ["incident-diagnosis"],
        recommended_mcp_servers: ["postgres"],
        recommended_provider_types: ["codex"],
        default_capability_selection: {
          enabled_skills: ["sql-review"],
          enabled_mcp_servers: ["postgres"],
          enabled_external_capabilities: ["jira.search"],
        },
        default_context_policy_override: { max_refs: 8 },
        default_approval_policy: { min_risk_for_human: "high" },
        metadata: { title: "数据库管理员" },
      },
      ...(includeFrontendTemplate
        ? [
            {
              type: "frontend_engineer",
              label: "前端开发",
              description: "负责 Web 控制台界面开发和页面问题诊断",
              default_role: "frontend_engineer",
              recommended_skills: ["frontend-implementation"],
              recommended_mcp_servers: ["browser"],
              recommended_provider_types: ["codex"],
              default_capability_selection: {
                enabled_skills: ["frontend-implementation"],
                enabled_mcp_servers: ["browser"],
                enabled_provider_types: ["codex"],
              },
              default_context_policy_override: { max_refs: 6 },
              default_approval_policy: { min_risk_for_human: "medium" },
              metadata: { title: "前端开发" },
            },
          ]
        : []),
    ],
    capability_options: {
      provider_types: sameRuntimeNodeProviders ? ["codex", "claude_code"] : ["codex"],
      skills: ["incident-diagnosis", "sql-review"],
      mcp_servers: ["postgres"],
      external_capabilities: ["jira.search"],
    },
    runtime_provider_options: runtimeProviderOptions,
    creation_checks: [
      {
        key: "team_governance",
        label: "团队治理版本",
        status: "passed",
        message: "#3 approved",
      },
      {
        key: "employee_templates",
        label: "专业模板",
        status: "passed",
        message: "1 个可用模板",
      },
      {
        key: "runtime_provider",
        label: "Provider 类型预览",
        status: runtimeProviderOptions.some((option) => option.available) ? "passed" : "warning",
        message: `${runtimeProviderOptions.filter((option) => option.available).length}/${runtimeProviderOptions.length} 个 Provider 候选当前在线；创建时不绑定 Runtime 节点`,
      },
      ...(includeCapabilityBoundaryBlock
        ? [
            {
              key: capabilityBoundaryKey,
              label: "能力边界",
              status: "blocked",
              message: "技能 0 · MCP 0 · 外部能力 0",
            },
          ]
        : []),
      ...(includeOtherBlockedCheck
        ? [
            {
              key: "other_blocker",
              label: "其他阻断",
              status: "blocked",
              message: "需要先完成外部前置条件",
            },
          ]
        : []),
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

function createWizardFetcher({
  expectedCreateBody,
  expectedEnvironmentVariables,
  expectedProviderType = "codex",
  expectedRuntimeNodeId = "33333333-3333-4333-8333-333333333333",
  expectedRuntimeNodeIdSubmitted = false,
  expectedTeamId,
  runtimeAvailability = "all",
  runtimeCount = 1,
  sameRuntimeNodeProviders = false,
  includePolicyExcludedProvider = false,
  includeFrontendTemplate = false,
  includeCapabilityBoundaryBlock = false,
  capabilityBoundaryKey = "capability_policy",
  includeOtherBlockedCheck = false,
  teams = [team],
  createOptionsErrorForTeamId,
  allowedEmployeeTypes = ["database_admin"],
}: {
  expectedCreateBody?: ExpectedCreateBody;
  expectedEnvironmentVariables?: Array<{ name: string; value: string; sensitive: boolean }>;
  expectedProviderType?: string;
  expectedRuntimeNodeId?: string;
  expectedRuntimeNodeIdSubmitted?: boolean;
  expectedTeamId?: string | undefined;
  runtimeAvailability?: RuntimeAvailabilityMode;
  runtimeCount?: 1 | 2;
  sameRuntimeNodeProviders?: boolean;
  includePolicyExcludedProvider?: boolean;
  includeFrontendTemplate?: boolean;
  includeCapabilityBoundaryBlock?: boolean;
  capabilityBoundaryKey?: "capability_policy" | "capability_boundary";
  includeOtherBlockedCheck?: boolean;
  teams?: Array<typeof team>;
  createOptionsErrorForTeamId?: string;
  allowedEmployeeTypes?: string[];
} = {}) {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse(teams);
    }

    if (url.pathname === "/api/v1/digital-employees/create-options" && method === "GET") {
      if (url.searchParams.has("team_id")) {
        expect(teams.map((item) => item.id)).toContain(url.searchParams.get("team_id"));
      }
      if (createOptionsErrorForTeamId && url.searchParams.get("team_id") === createOptionsErrorForTeamId) {
        return jsonResponse(
          {
            code: "team_governance_config_required",
            message: "employee effective config required: active team governance config is required",
          },
          422,
        );
      }
      return jsonResponse(
        createOptionsFixture({
          allowedEmployeeTypes,
          capabilityBoundaryKey,
          includeFrontendTemplate,
          includeCapabilityBoundaryBlock,
          includeOtherBlockedCheck,
          includePolicyExcludedProvider,
          runtimeAvailability,
          runtimeCount,
          sameRuntimeNodeProviders,
        }),
      );
    }

    if (url.pathname === "/api/v1/digital-employee-avatar-assets" && method === "GET") {
      return jsonResponse([avatarAsset]);
    }

    if (url.pathname === "/api/v1/digital-employees" && method === "POST") {
      const body = JSON.parse(String(init?.body));
      const { budget_policy: _budgetPolicy, ...bodyWithoutBudgetPolicy } = body;
      const defaultExpectedBody = {
        ...(expectedTeamId ? { team_id: expectedTeamId } : {}),
        employee_type: "database_admin",
        name: "数据库管理员工",
        role: "database_admin",
        risk_level: "high",
        avatar_asset_id: avatarAsset.id,
        role_profile: {
          employee_type: "database_admin",
          role: "database_admin",
          title: "数据库管理员",
        },
        capability_selection: {
          enabled_skills: ["sql-review"],
          enabled_mcp_servers: ["postgres"],
          enabled_external_capabilities: ["jira.search"],
        },
        context_policy_override: { max_refs: 8 },
        approval_policy_override: { min_risk_for_human: "high" },
        output_contract_addendum: {},
        ...(expectedRuntimeNodeIdSubmitted ? { runtime_node_id: expectedRuntimeNodeId } : {}),
        provider_type: expectedProviderType,
        session_policy: { mode: "reuse_latest" },
        workspace_policy: {},
        environment_variables: expectedEnvironmentVariables ?? [],
      };
      expect(bodyWithoutBudgetPolicy).toEqual(expectedCreateBody ?? defaultExpectedBody);

      return jsonResponse(
        {
          id: "11111111-1111-4111-8111-111111111111",
          tenant_id: "22222222-2222-4222-8222-222222222222",
          ...(expectedTeamId ? { team_id: expectedTeamId } : {}),
          owner_user_id: "66666666-6666-4666-8666-666666666666",
          employee_type: "database_admin",
          name: "数据库管理员工",
          role: "database_admin",
          status: "ready",
          permission_policy: {},
          context_policy: {},
          approval_policy: {},
          risk_level: "high",
        },
        201,
      );
    }

    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;

  return fetcher;
}

type FetchMockCall = [RequestInfo | URL, RequestInit | undefined];

function findCreateEmployeePost(fetcher: typeof fetch) {
  const calls = (fetcher as unknown as { mock: { calls: FetchMockCall[] } }).mock.calls;
  return calls.find(
    ([input, init]) => String(input).endsWith("/api/v1/digital-employees") && init?.method === "POST",
  );
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

async function renderCreateEmployeeView(fetcher = createWizardFetcher(), routerSearch: Record<string, unknown> = {}) {
  navigate.mockReset();
  search.mockReset();
  search.mockReturnValue(routerSearch);
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <CreateEmployeeView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

async function enterConfiguration(screen: Awaited<ReturnType<typeof renderCreateEmployeeView>>) {
  await expect.element(screen.getByRole("heading", { name: "选择内置模板" })).toBeVisible();
  await expect.element(screen.getByRole("button", { name: "进入完成配置" })).toBeEnabled();
  await userEvent.click(screen.getByRole("button", { name: "进入完成配置" }));
  await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();
}

async function enterBlankCustomConfiguration(screen: Awaited<ReturnType<typeof renderCreateEmployeeView>>) {
  await expect.element(screen.getByRole("button", { name: /^空白自定义/ })).toBeEnabled();
  await userEvent.click(screen.getByRole("button", { name: /^空白自定义/ }));
  await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();
  expect(document.body.textContent).toContain("自定义身份");
  expect(document.body.textContent).not.toContain("选择员工类型");
  expect(document.body.textContent).not.toContain("配置预检");
}

async function enterConfirmCreation(screen: Awaited<ReturnType<typeof renderCreateEmployeeView>>) {
  await userEvent.click(screen.getByRole("button", { name: "进入确认创建" }));
  await expect.element(screen.getByRole("heading", { name: "确认创建" })).toBeVisible();
}

function findTemplateSelectionTableText() {
  return document.body.querySelector('[data-testid="template-selection-table"]')?.textContent ?? "";
}

function findFirstTemplateCellText() {
  return document.body.querySelector('[data-testid="template-selection-table"] tbody td')?.textContent ?? "";
}

describe("CreateEmployeeView", () => {
  it("shows a streamlined creation entry without a standalone preflight step", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("heading", { name: "创建数字员工" })).toBeVisible();
    await expect.element(screen.getByText("创建方式", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("完成配置", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("确认创建", { exact: true })).toBeVisible();
    expect(document.body.textContent).not.toContain("配置预检");
    await expect.element(screen.getByRole("heading", { name: "创建路径" })).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "选择内置模板" })).toBeVisible();
    await expect.element(screen.getByText("从专业模板创建")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: /进入完成配置/ })).toBeVisible();
    expect(document.body.textContent).not.toContain("Runtime 可用");
    expect(document.body.textContent).not.toContain("即将创建");
  });

  it("keeps the configure step actions fixed to the viewport bottom", async () => {
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);

    const actions = document.body.querySelector('[data-testid="employee-configure-actions"]');
    expect(actions).toBeTruthy();
    expect(actions).toHaveClass("sticky");
    expect(actions).toHaveClass("bottom-0");
    await expect.element(screen.getByRole("button", { name: "下一步" })).toBeVisible();
  });

  it("loads built-in templates without a team-scoped create-options request before team selection", async () => {
    const fetcher = createWizardFetcher({ teams: [team, secondTeam] });
    const screen = await renderCreateEmployeeView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "选择内置模板" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "已选择数据库管理员模板" })).toBeVisible();

    const createOptionsCalls = (fetcher as unknown as { mock: { calls: FetchMockCall[] } }).mock.calls.filter(
      ([input, init]) =>
        String(input).includes("/api/v1/digital-employees/create-options") && (init?.method ?? "GET") === "GET",
    );
    expect(createOptionsCalls.length).toBeGreaterThan(0);
    expect(createOptionsCalls.every(([input]) => !new URL(String(input)).searchParams.has("team_id"))).toBe(true);
  });

  it("opens blank custom as a custom identity without asking for employee type", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("button", { name: /^从专业模板创建/ })).toBeEnabled();
    await expect.element(screen.getByRole("button", { name: /^空白自定义/ })).toBeEnabled();
    await expect.element(screen.getByRole("button", { name: /^从团队角色复制/ })).toBeDisabled();
    await expect.element(screen.getByRole("button", { name: /^从历史员工克隆/ })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: /^空白自定义/ }));

    await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();
    expect(document.body.textContent).toContain("自定义身份");
    expect(document.body.textContent).not.toContain("选择员工类型");
    expect(document.body.textContent).not.toContain("底层类型");
    expect(document.body.textContent).not.toContain("选择内置模板");
    expect(document.body.textContent).not.toContain("配置预检");
  });

  it("keeps the template picker provider-neutral (no runtime state)", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("button", { name: /数据库管理员/ })).toBeVisible();
    await expect.element(screen.getByText("风险等级")).toBeVisible();

    const tableText = findTemplateSelectionTableText();
    expect(tableText).toContain("技能");
    expect(tableText).toContain("MCP");
    expect(tableText).not.toContain("推荐 Provider");
    expect(tableText).not.toContain("Provider 可用");
    expect(tableText).not.toContain("Runtime");
  });

  it("renders built-in templates as a scalable table using precise template fields", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("heading", { name: "选择内置模板" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "已选择数据库管理员模板" })).toBeVisible();

    const tableText = findTemplateSelectionTableText();
    expect(tableText).toContain("模板");
    expect(tableText).toContain("默认角色");
    expect(tableText).toContain("模板能力");
    expect(tableText).toContain("默认注入");
    expect(tableText).toContain("风险等级");
    expect(tableText).toContain("数据库管理员");
    expect(tableText).toContain("技能 1");
    expect(tableText).toContain("MCP 1");
    expect(tableText).toContain("Provider 1");
    expect(tableText).toContain("外部能力 1");
    expect(tableText).toContain("高");
    expect(tableText).not.toContain("推荐能力");
    expect(tableText).not.toContain("风险触发");
    expect(tableText).not.toContain("运行可用性");
    expect(tableText).not.toContain("Provider 可用");
  });

  it("does not duplicate the template type in the template column", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("button", { name: "选择数据库管理员模板" })).toBeVisible();

    expect(findFirstTemplateCellText()).toContain("数据库管理员");
    expect(findFirstTemplateCellText()).toContain("负责数据库变更");
    expect(findFirstTemplateCellText()).not.toContain("database_admin");
  });

  it("preselects the template from the template search parameter", async () => {
    const screen = await renderCreateEmployeeView(
      createWizardFetcher({ includeFrontendTemplate: true }),
      { template: "frontend_engineer" },
    );

    await expect.element(screen.getByRole("button", { name: "选择前端开发模板" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "已选择前端开发模板" })).toBeVisible();
    const tableText = findTemplateSelectionTableText();
    expect(tableText).toContain("前端开发");

    await enterConfiguration(screen);
    expect(screen.getByLabelText("员工类型").query()).toBeNull();
    expect(document.body.textContent).toContain("前端开发");
    await expect.element(screen.getByLabelText("职责定位")).toHaveValue("frontend_engineer");
  });

  it("shows the profile blueprint without a redundant template summary after entering configuration", async () => {
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);

    await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();
    expect(screen.getByText("已选模板", { exact: true }).query()).toBeNull();
    expect(screen.getByRole("button", { name: /更换创建路径/ }).query()).toBeNull();
    expect(document.body.textContent).not.toContain("推荐起步画像");
    expect(document.body.textContent).not.toContain("从空白开始自定义");
    expect(document.body.textContent).not.toContain("模板只提供默认值和推荐能力");
  });

  it("keeps the selected template visible without direct employee type editing", async () => {
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);

    expect(screen.getByLabelText("员工类型").query()).toBeNull();
    expect(document.body.textContent).toContain("数据库管理员");
    expect(document.body.textContent).toContain("负责数据库变更、备份、性能诊断和恢复验证");
  });

  it("keeps edited configuration when change-template confirmation is cancelled", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "返回" }));

    expect(confirm).toHaveBeenCalledWith("更换创建路径会重置当前配置草稿，是否继续？");
    await expect.element(screen.getByRole("heading", { name: "身份" })).toBeVisible();
    await expect.element(screen.getByLabelText("名称")).toHaveValue("数据库管理员工");
  });

  it("resets the configuration draft when change-template confirmation is accepted", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "返回" }));

    expect(confirm).toHaveBeenCalledWith("更换创建路径会重置当前配置草稿，是否继续？");
    await expect.element(screen.getByRole("heading", { name: "选择内置模板" })).toBeVisible();

    await enterConfiguration(screen);
    await expect.element(screen.getByLabelText("名称")).toHaveValue("");
  });

  it("keeps edited configuration when team-change confirmation is cancelled", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    const screen = await renderCreateEmployeeView(createWizardFetcher({ teams: [team, secondTeam] }));

    await enterConfiguration(screen);
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), team.id);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), secondTeam.id);

    expect(confirm).toHaveBeenCalledWith("更换团队会重置当前配置草稿，是否继续？");
    await expect.element(screen.getByLabelText("归属团队")).toHaveValue(team.id);
    await expect.element(screen.getByLabelText("名称")).toHaveValue("数据库管理员工");
  });

  it("resets the configuration draft when team-change confirmation is accepted", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const screen = await renderCreateEmployeeView(createWizardFetcher({ teams: [team, secondTeam] }));

    await enterConfiguration(screen);
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), team.id);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), secondTeam.id);

    expect(confirm).toHaveBeenCalledWith("更换团队会重置当前配置草稿，是否继续？");
    await expect.element(screen.getByLabelText("归属团队")).toHaveValue(secondTeam.id);
    await expect.element(screen.getByLabelText("名称")).toHaveValue("");
  });

  it("returns blank-custom team changes to employee type selection after confirmation", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const screen = await renderCreateEmployeeView(createWizardFetcher({ teams: [team, secondTeam] }));

    await enterBlankCustomConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), secondTeam.id);

    expect(confirm).toHaveBeenCalledWith("更换团队会重置当前配置草稿，是否继续？");
    await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();
    expect(document.body.textContent).toContain("自定义身份");
    expect(document.body.textContent).not.toContain("选择员工类型");
    expect(document.body.textContent).not.toContain("配置预检");
  });

  it("shows a business blocker when selected team lacks governance config", async () => {
    const screen = await renderCreateEmployeeView(
      createWizardFetcher({ teams: [team, secondTeam], createOptionsErrorForTeamId: secondTeam.id }),
    );

    await enterConfiguration(screen);
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), secondTeam.id);

    await expect.element(screen.getByText("该团队尚未启用治理配置，不能在此团队下创建数字员工。")).toBeVisible();
    expect(document.body.textContent).not.toContain("employee effective config required");
    await expect.element(screen.getByRole("button", { name: "先不归属团队创建" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "下一步" })).toBeDisabled();
  });

  it("blocks blank custom when selected team does not allow custom_agent", async () => {
    const screen = await renderCreateEmployeeView(
      createWizardFetcher({ teams: [team], allowedEmployeeTypes: ["database_admin"] }),
    );

    await enterBlankCustomConfiguration(screen);
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), team.id);

    await expect.element(screen.getByText("该团队不允许创建自定义数字员工。")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "下一步" })).toBeDisabled();
  });

  it("creates a ready digital employee through the streamlined wizard", async () => {
    const fetcher = createWizardFetcher({ expectedTeamId: team.id });
    const screen = await renderCreateEmployeeView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "创建数字员工" })).toBeVisible();
    await enterConfiguration(screen);
    await expect.element(screen.getByLabelText("归属团队")).toHaveValue("");
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), team.id);
    expect(screen.getByLabelText("员工类型").query()).toBeNull();
    await expect.element(screen.getByLabelText("职责定位")).toHaveValue("database_admin");
    await expect.element(screen.getByLabelText("风险等级")).toHaveValue("high");
    await expect.element(screen.getByAltText("工程师头像 M01")).toBeVisible();

    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByLabelText("Codex")).toBeChecked();

    await enterConfirmCreation(screen);
    await expect.element(screen.getByText("数据库管理员工")).toBeVisible();
    await expect.element(screen.getByText("Codex")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    expect(navigate).toHaveBeenCalledWith({
      params: { employeeId: "11111111-1111-4111-8111-111111111111" },
      to: "/employees/$employeeId",
    });
  });

  it("starts blank-custom configuration without template-injected capabilities", async () => {
    const screen = await renderCreateEmployeeView();

    await enterBlankCustomConfiguration(screen);

    expect(screen.getByLabelText("员工类型").query()).toBeNull();
    await expect.element(screen.getByLabelText("职责定位")).toHaveValue("");

    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.fill(screen.getByLabelText("职责定位"), "数据库变更与恢复验证");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByText("团队继承能力", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("员工扩展能力", { exact: true })).toBeVisible();
    await expect.element(screen.getByRole("checkbox", { name: "incident-diagnosis" })).not.toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "sql-review" })).not.toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "postgres" })).not.toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "jira.search" })).not.toBeChecked();
  });

  it("shows 团队继承能力 and 员工扩展能力 as separate capability sections", async () => {
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByText("团队继承能力", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("员工扩展能力", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("团队绑定能力只读展示，不会作为员工扩展能力重复提交。")).toBeVisible();
    await expect.element(screen.getByText("这里只提交员工个人扩展项。")).toBeVisible();
  });

  it("configures blank custom without employee type or description fields", async () => {
    const screen = await renderCreateEmployeeView();

    await enterBlankCustomConfiguration(screen);

    await expect.element(screen.getByRole("heading", { name: "身份" })).toBeVisible();
    expect(screen.getByLabelText("员工类型").query()).toBeNull();
    expect(screen.getByLabelText("描述").query()).toBeNull();
    expect(document.body.textContent).toContain("自定义身份");
    await expect.element(screen.getByLabelText("职责定位")).toHaveValue("");
  });

  it("does not show capability boundary as a blank-custom configuration summary blocker", async () => {
    const screen = await renderCreateEmployeeView(createWizardFetcher({ includeCapabilityBoundaryBlock: true }));

    await userEvent.click(screen.getByRole("button", { name: /^空白自定义/ }));
    await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();

    expect(screen.getByText("能力边界", { exact: true }).query()).toBeNull();
    expect(screen.getByText("技能 0 · MCP 0 · 外部能力 0", { exact: true }).query()).toBeNull();
    await userEvent.fill(screen.getByLabelText("名称"), "空白自定义员工");
    await userEvent.fill(screen.getByLabelText("职责定位"), "自定义诊断职责");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByRole("heading", { name: "能力" })).toBeVisible();
  });

  it("also hides the legacy capability boundary check key from blank-custom configuration summary", async () => {
    const screen = await renderCreateEmployeeView(
      createWizardFetcher({
        capabilityBoundaryKey: "capability_boundary",
        includeCapabilityBoundaryBlock: true,
      }),
    );

    await userEvent.click(screen.getByRole("button", { name: /^空白自定义/ }));
    await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();

    expect(screen.getByText("能力边界", { exact: true }).query()).toBeNull();
  });

  it("shows non-capability blockers in the blank-custom configuration summary", async () => {
    const screen = await renderCreateEmployeeView(
      createWizardFetcher({
        includeCapabilityBoundaryBlock: true,
        includeOtherBlockedCheck: true,
      }),
    );

    await userEvent.click(screen.getByRole("button", { name: /^空白自定义/ }));
    await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();

    await expect.element(screen.getByText("其他阻断", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("需要先完成外部前置条件", { exact: true })).toBeVisible();
    expect(screen.getByText("能力边界", { exact: true }).query()).toBeNull();
    expect(screen.getByText("能力边界: 技能 0 · MCP 0 · 外部能力 0").query()).toBeNull();
  });

  it("does not show capability boundary as a template configuration summary blocker", async () => {
    const screen = await renderCreateEmployeeView(createWizardFetcher({ includeCapabilityBoundaryBlock: true }));

    await userEvent.click(screen.getByRole("button", { name: "进入完成配置" }));
    await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();

    expect(screen.getByText("能力边界", { exact: true }).query()).toBeNull();
    expect(screen.getByText("能力边界: 技能 0 · MCP 0 · 外部能力 0").query()).toBeNull();
    await userEvent.fill(screen.getByLabelText("名称"), "模板员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByRole("heading", { name: "能力" })).toBeVisible();
  });

  it("shows blank-custom source on the confirm step", async () => {
    const screen = await renderCreateEmployeeView();

    await enterBlankCustomConfiguration(screen);

    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.fill(screen.getByLabelText("职责定位"), "数据库变更与恢复验证");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByLabelText("Codex"));
    await enterConfirmCreation(screen);

    expect(document.body.textContent).toContain("自定义身份");
    expect(document.body.textContent).not.toContain("底层类型");
  });

  it("submits blank-custom creation without template-injected capabilities or policy overrides", async () => {
    const fetcher = createWizardFetcher({
      expectedCreateBody: {
        employee_type: "custom_agent",
        name: "数据库管理员工",
        role: "数据库变更与恢复验证",
        risk_level: "medium",
        avatar_asset_id: avatarAsset.id,
        role_profile: {
          employee_type: "custom_agent",
          role: "数据库变更与恢复验证",
          title: "custom_agent",
        },
        capability_selection: {
          enabled_skills: [],
          enabled_mcp_servers: [],
          enabled_external_capabilities: [],
        },
        context_policy_override: {},
        approval_policy_override: {},
        output_contract_addendum: {},
        provider_type: "codex",
        session_policy: { mode: "reuse_latest" },
        workspace_policy: {},
        environment_variables: [],
        metadata: { creation_mode: "blank_custom" },
      },
    });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterBlankCustomConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.fill(screen.getByLabelText("职责定位"), "数据库变更与恢复验证");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByLabelText("Codex"));
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.capability_selection).toEqual({
      enabled_skills: [],
      enabled_mcp_servers: [],
      enabled_external_capabilities: [],
    });
    expect(body.context_policy_override).toEqual({});
    expect(body.approval_policy_override).toEqual({});
    expect(body.metadata).toEqual({ creation_mode: "blank_custom" });
  });

  it("keeps template creation seeded with template capability and policy defaults", async () => {
    const fetcher = createWizardFetcher();
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("checkbox", { name: "sql-review" })).toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "postgres" })).toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "jira.search" })).toBeChecked();

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.capability_selection).toEqual({
      enabled_skills: ["sql-review"],
      enabled_mcp_servers: ["postgres"],
      enabled_external_capabilities: ["jira.search"],
    });
    expect(body.context_policy_override).toEqual({ max_refs: 8 });
    expect(body.approval_policy_override).toEqual({ min_risk_for_human: "high" });
    expect(body.metadata).toBeUndefined();
  });

  it("supports creating a team-less digital employee when the user selects no team", async () => {
    const fetcher = createWizardFetcher({
      expectedTeamId: undefined,
      teams: [team, secondTeam],
    });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await expect.element(screen.getByLabelText("归属团队")).toHaveValue("");
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createOptionsCalls = (fetcher as unknown as { mock: { calls: FetchMockCall[] } }).mock.calls.filter(
      ([input, init]) =>
        String(input).includes("/api/v1/digital-employees/create-options") && (init?.method ?? "GET") === "GET",
    );
    expect(createOptionsCalls.some(([input]) => !new URL(String(input)).searchParams.has("team_id"))).toBe(true);

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.team_id).toBeUndefined();
  });

  it("submits an optional daily token budget when creating a digital employee", async () => {
    const fetcher = createWizardFetcher();
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.type(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" }), "12000");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByLabelText("Codex")).toBeChecked();
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.budget_policy).toEqual({ daily_token_limit: 12000 });
  });

  it("submits environment variables from the runtime step when creating a digital employee", async () => {
    const fetcher = createWizardFetcher({
      expectedEnvironmentVariables: [{ name: "GH_TOKEN", value: "ghp_secret", sensitive: true }],
    });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await userEvent.click(screen.getByRole("button", { name: "添加环境变量" }));
    await userEvent.fill(screen.getByLabelText("环境变量名称 1"), "GH_TOKEN");
    await userEvent.fill(screen.getByLabelText("环境变量值 1"), "ghp_secret");
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.environment_variables).toEqual([
      { name: "GH_TOKEN", value: "ghp_secret", sensitive: true },
    ]);
  });

  it("omits daily token budget when the field is empty", async () => {
    const fetcher = createWizardFetcher();
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" })).toHaveValue(null);
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.budget_policy).toEqual({});
  });

  it.each(["0", "12.5"])("blocks invalid daily token budget %s on the governance step", async (invalidValue) => {
    const fetcher = createWizardFetcher();
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.type(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" }), invalidValue);
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByText("每日 Token 预算上限必须是正整数")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "治理" })).toBeVisible();
    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeUndefined();
  });

  it("blocks the next step until identity fields are valid", async () => {
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);
    await expect.element(screen.getByRole("heading", { name: "身份" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByText("名称不能为空")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "身份" })).toBeVisible();
    await expect.element(screen.getByLabelText("名称")).toBeVisible();
  });

  it("keeps avatar choices compact in the identity step", async () => {
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);
    await expect.element(screen.getByRole("button", { name: "工程师头像 M01" })).toHaveClass("size-20");
  });

  it("auto-selects a provider even when multiple runtimes are available", async () => {
    const screen = await renderCreateEmployeeView(createWizardFetcher({ runtimeCount: 2 }));

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByLabelText("Codex")).toBeChecked();
    await expect.element(screen.getByText("2 个 Runtime 节点候选会在项目运行准备中评估")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "进入确认创建" })).toBeEnabled();
  });

  it("describes provider type as required without promising runtime binding", async () => {
    const screen = await renderCreateEmployeeView(createWizardFetcher({ runtimeCount: 2 }));

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("heading", { name: "Provider 类型" })).toBeVisible();
    expect(document.body.textContent).toContain("数字员工必须选择一个 Provider 类型");
    expect(document.body.textContent).toContain("Runtime 节点会在项目运行准备中决定，不在创建时绑定到员工。");
    expect(document.body.textContent).not.toContain("运行绑定");
    expect(document.body.textContent).not.toContain("绑定到 Runtime");
    expect(document.body.textContent).not.toContain("Runtime 当前可调度");
    expect(document.body.textContent).not.toContain("Runtime placement");

    await enterConfirmCreation(screen);
    await expect.element(screen.getByText("Provider 类型", { exact: true })).toBeVisible();
    expect(document.body.textContent).toContain("Runtime 节点会在项目运行准备中决定，不在创建时绑定到员工。");
    expect(document.body.textContent).not.toContain("运行绑定");
    expect(document.body.textContent).not.toContain("绑定到 Runtime");
    expect(document.body.textContent).not.toContain("Runtime 当前可调度");
    expect(document.body.textContent).not.toContain("Runtime placement");
  });

  it("shows provider dispatch preview when only some runtimes are available", async () => {
    const screen = await renderCreateEmployeeView(
      createWizardFetcher({
        runtimeAvailability: "first-unavailable",
        runtimeCount: 2,
      }),
    );

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("radio", { name: "Codex" })).toBeChecked();
    await expect.element(screen.getByText("1/2 个 Runtime 节点当前在线，仅用于项目运行准备参考")).toBeVisible();
  });

  it("shows runtime provider summary without blocking configuration when there are no bindable options", async () => {
    const screen = await renderCreateEmployeeView(createWizardFetcher({ runtimeAvailability: "none" }));

    await expect.element(screen.getByRole("button", { name: "进入完成配置" })).toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "进入完成配置" }));
    await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();

    await expect.element(screen.getByText("Provider 类型预览", { exact: true })).toBeVisible();
    await expect
      .element(screen.getByText("0/1 个 Provider 候选当前在线；创建时不绑定 Runtime 节点"))
      .toBeVisible();
  });

  it("does not expose runtime preview providers outside team policy as selectable providers", async () => {
    const screen = await renderCreateEmployeeView(
      createWizardFetcher({
        includePolicyExcludedProvider: true,
      }),
    );

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByLabelText("Codex")).toBeChecked();
    expect(document.body.textContent).not.toContain("claude_code");
  });

  it("submits provider-only creation when no runtime is currently available", async () => {
    const fetcher = createWizardFetcher({
      expectedRuntimeNodeIdSubmitted: false,
      runtimeAvailability: "none",
    });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByLabelText("Codex"));
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.provider_type).toBe("codex");
    expect(body).not.toHaveProperty("runtime_node_id");
  });

  it("shows Chinese risk labels and canonical Provider labels while submitting enum values", async () => {
    const fetcher = createWizardFetcher({
      sameRuntimeNodeProviders: true,
      expectedProviderType: "claude-code",
    });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await expect.element(screen.getByLabelText("风险等级")).toHaveDisplayValue("高");
    expect(document.body.textContent).not.toContain("风险 high");
    await userEvent.selectOptions(screen.getByLabelText("风险等级"), "critical");
    await expect.element(screen.getByLabelText("风险等级")).toHaveDisplayValue("严重");

    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByLabelText("Codex")).toBeVisible();
    await expect.element(screen.getByLabelText("Claude Code")).toBeVisible();
    expect(screen.getByLabelText("claude_code").query()).toBeNull();
    await userEvent.click(screen.getByLabelText("Claude Code"));

    await enterConfirmCreation(screen);
    await expect.element(screen.getByText("严重", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("Claude Code", { exact: true })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.risk_level).toBe("critical");
    expect(body.provider_type).toBe("claude-code");
  });

  it("submits the selected provider when one runtime exposes multiple providers", async () => {
    const fetcher = createWizardFetcher({
      expectedProviderType: "claude-code",
      sameRuntimeNodeProviders: true,
    });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("button", { name: "进入确认创建" })).toBeDisabled();
    await expect.element(screen.getByLabelText("Codex")).not.toBeChecked();
    await expect.element(screen.getByLabelText("Claude Code")).not.toBeChecked();
    expect(document.body.textContent).not.toContain("claude_code");

    await userEvent.click(screen.getByLabelText("Claude Code"));
    await expect.element(screen.getByRole("button", { name: "进入确认创建" })).toBeEnabled();
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.provider_type).toBe("claude-code");

    expect(navigate).toHaveBeenCalledWith({
      params: { employeeId: "11111111-1111-4111-8111-111111111111" },
      to: "/employees/$employeeId",
    });
  });
});
