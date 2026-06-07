import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { CreateEmployeeView } from "./create";

const navigate = vi.fn();

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

function createOptionsFixture({
  runtimeCount = 1,
  sameRuntimeNodeProviders = false,
}: {
  runtimeCount?: 1 | 2;
  sameRuntimeNodeProviders?: boolean;
} = {}) {
  const runtimeProviderOptions = [
    {
      runtime_node_id: "33333333-3333-4333-8333-333333333333",
      node_id: "runtime-a",
      runtime_name: "客户侧执行机 A",
      provider_type: "codex",
      runtime_status: "online",
      provider_status: "healthy",
      health_status: "healthy",
      current_load: 0,
      max_slots: 2,
      agent_home_dir: "/Users/wangpei/.codex",
      agent_home_dir_available: true,
      available: true,
    },
    ...(sameRuntimeNodeProviders
      ? [
          {
            runtime_node_id: "33333333-3333-4333-8333-333333333333",
            node_id: "runtime-a",
            runtime_name: "客户侧执行机 A",
            provider_type: "claude_code",
            runtime_status: "online",
            provider_status: "healthy",
            health_status: "healthy",
            current_load: 0,
            max_slots: 2,
            agent_home_dir: "/Users/wangpei/.claude",
            agent_home_dir_available: true,
            available: true,
          },
        ]
      : []),
    ...(!sameRuntimeNodeProviders && runtimeCount === 2
      ? [
          {
            runtime_node_id: "44444444-4444-4444-8444-444444444444",
            node_id: "runtime-b",
            runtime_name: "客户侧执行机 B",
            provider_type: "codex",
            runtime_status: "online",
            provider_status: "healthy",
            health_status: "healthy",
            current_load: 1,
            max_slots: 2,
            agent_home_dir: "/Users/wangpei/.codex",
            agent_home_dir_available: true,
            available: true,
          },
        ]
      : []),
  ];

  return {
    team_config: {
      id: "55555555-5555-4555-8555-555555555555",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: team.id,
      revision_number: 3,
      status: "approved",
      allowed_employee_types: ["database_admin"],
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
    ],
    capability_options: {
      provider_types: sameRuntimeNodeProviders ? ["codex", "claude_code"] : ["codex"],
      skills: ["incident-diagnosis", "sql-review"],
      mcp_servers: ["postgres"],
      external_capabilities: ["jira.search"],
    },
    runtime_provider_options: runtimeProviderOptions,
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
  expectedProviderType = "codex",
  expectedRuntimeNodeId = "33333333-3333-4333-8333-333333333333",
  runtimeCount = 1,
  sameRuntimeNodeProviders = false,
}: {
  expectedProviderType?: string;
  expectedRuntimeNodeId?: string;
  runtimeCount?: 1 | 2;
  sameRuntimeNodeProviders?: boolean;
} = {}) {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse([team]);
    }

    if (url.pathname === "/api/v1/digital-employees/create-options" && method === "GET") {
      expect(url.searchParams.get("team_id")).toBe(team.id);
      return jsonResponse(createOptionsFixture({ runtimeCount, sameRuntimeNodeProviders }));
    }

    if (url.pathname === "/api/v1/digital-employee-avatar-assets" && method === "GET") {
      return jsonResponse([avatarAsset]);
    }

    if (url.pathname === "/api/v1/digital-employees" && method === "POST") {
      expect(JSON.parse(String(init?.body))).toEqual({
        team_id: team.id,
        employee_type: "database_admin",
        name: "数据库管理员工",
        role: "database_admin",
        description: "负责生产数据库变更和恢复验证",
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
        runtime_node_id: expectedRuntimeNodeId,
        provider_type: expectedProviderType,
        session_policy: { mode: "reuse_latest" },
        workspace_policy: {},
      });

      return jsonResponse(
        {
          id: "11111111-1111-4111-8111-111111111111",
          tenant_id: "22222222-2222-4222-8222-222222222222",
          team_id: team.id,
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

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

async function renderCreateEmployeeView(fetcher = createWizardFetcher()) {
  navigate.mockReset();
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <CreateEmployeeView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("CreateEmployeeView", () => {
  it("creates a ready digital employee through the four-step wizard", async () => {
    const fetcher = createWizardFetcher();
    const screen = await renderCreateEmployeeView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "创建数字员工" })).toBeVisible();
    await expect.element(screen.getByLabelText("归属团队")).toHaveValue(team.id);
    await expect.element(screen.getByLabelText("员工类型")).toHaveValue("database_admin");
    await expect.element(screen.getByLabelText("角色")).toHaveValue("database_admin");
    await expect.element(screen.getByLabelText("风险等级")).toHaveValue("high");
    await expect.element(screen.getByAltText("工程师头像 M01")).toBeVisible();

    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.fill(screen.getByLabelText("描述"), "负责生产数据库变更和恢复验证");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByLabelText("客户侧执行机 A / codex")).toBeChecked();

    await userEvent.click(screen.getByRole("button", { name: "创建数字员工" }));

    expect(navigate).toHaveBeenCalledWith({
      params: { employeeId: "11111111-1111-4111-8111-111111111111" },
      to: "/employees/$employeeId",
    });
  });

  it("blocks the next step until identity fields are valid", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("heading", { name: "身份" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByText("名称不能为空")).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "身份" })).toBeVisible();
    await expect.element(screen.getByLabelText("名称")).toBeVisible();
  });

  it("requires explicit runtime selection when multiple runtimes are available", async () => {
    const screen = await renderCreateEmployeeView(createWizardFetcher({ runtimeCount: 2 }));

    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("button", { name: "创建数字员工" })).toBeDisabled();
    await expect.element(screen.getByLabelText("客户侧执行机 A / codex")).not.toBeChecked();
    await expect.element(screen.getByLabelText("客户侧执行机 B / codex")).not.toBeChecked();

    await userEvent.click(screen.getByLabelText("客户侧执行机 A / codex"));
    await expect.element(screen.getByRole("button", { name: "创建数字员工" })).toBeEnabled();
  });

  it("submits the selected provider when one runtime exposes multiple providers", async () => {
    const fetcher = createWizardFetcher({
      expectedProviderType: "claude_code",
      sameRuntimeNodeProviders: true,
    });
    const screen = await renderCreateEmployeeView(fetcher);

    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.fill(screen.getByLabelText("描述"), "负责生产数据库变更和恢复验证");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("button", { name: "创建数字员工" })).toBeDisabled();
    await expect.element(screen.getByLabelText("客户侧执行机 A / codex")).not.toBeChecked();
    await expect.element(screen.getByLabelText("客户侧执行机 A / claude_code")).not.toBeChecked();

    await userEvent.click(screen.getByLabelText("客户侧执行机 A / claude_code"));
    await expect.element(screen.getByRole("button", { name: "创建数字员工" })).toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "创建数字员工" }));

    expect(navigate).toHaveBeenCalledWith({
      params: { employeeId: "11111111-1111-4111-8111-111111111111" },
      to: "/employees/$employeeId",
    });
  });
});
