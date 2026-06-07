import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { TeamDetailView, TeamsView } from "@/features/teams";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => (
    <header>{children}</header>
  ),
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

vi.mock("@tanstack/react-router", () => {
  type MockLinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
    children: ReactNode;
    params?: Record<string, string>;
    to: string;
  };
  const Link = forwardRef<HTMLAnchorElement, MockLinkProps>(
    ({ children, params, to, ...props }, ref) => (
      <a
        {...props}
        data-router-link="true"
        href={
          params?.teamId
            ? to.replace("$teamId", encodeURIComponent(params.teamId))
            : to
        }
        ref={ref}
      >
        {children}
      </a>
    ),
  );
  Link.displayName = "MockRouterLink";

  return { Link };
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

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function fetchCalls(fetcher: typeof fetch) {
  return (
    fetcher as unknown as {
      mock: { calls: [RequestInfo | URL, RequestInit | undefined][] };
    }
  ).mock.calls;
}

function teamSummaryGetCalls(fetcher: typeof fetch, offset?: number) {
  return fetchCalls(fetcher).filter(([input, init]) => {
    const url = new URL(String(input));

    if (url.pathname !== "/api/v1/teams" || init?.method !== "GET") {
      return false;
    }

    return offset === undefined
      ? true
      : url.searchParams.get("offset") === String(offset);
  });
}

function createTeamPostIndex(fetcher: typeof fetch) {
  return fetchCalls(fetcher).findIndex(([url, init]) => {
    return String(url).endsWith("/api/v1/teams") && init?.method === "POST";
  });
}

function makeTeamSummary(index: number) {
  const isPrimary = index === 1;

  return {
    id: `team-${index}`,
    tenant_id: "tenant-1",
    slug: isPrimary ? "ops" : `team-${index}`,
    name: isPrimary ? "运维团队" : `团队 ${index}`,
    status: "active",
    human_owner_user_id: isPrimary ? "human-owner-1" : `human-owner-${index}`,
    human_owner: {
      user_id: isPrimary ? "human-owner-1" : `human-owner-${index}`,
      username: isPrimary ? "owner" : `owner-${index}`,
      display_name: isPrimary ? "负责人甲" : `负责人 ${index}`,
      email: isPrimary ? "owner@example.com" : `owner-${index}@example.com`,
      status: "active",
      avatar: {
        provider: "dicebear",
        seed: isPrimary ? "owner" : `owner-${index}`,
        style: "adventurer",
      },
    },
    member_count: isPrimary ? 18 : index,
    digital_employee_count: isPrimary ? 6 : 1,
    capability_count: isPrimary ? 12 : 2,
    governance_status: "active",
    current_revision: isPrimary ? 7 : 1,
    pending_draft_count: isPrimary ? 3 : 0,
    risk_summary: isPrimary ? "生产写操作需审批" : "常规团队策略",
    metadata: {
      display: {
        color_tone: isPrimary ? "cyan" : "neutral",
        icon_key: isPrimary ? "ops" : "default",
      },
    },
  };
}

function createTeamsFetcher(
  options: {
    createStatus?: number;
    disabledOverview?: boolean;
    secondPageMode?: "empty" | "error" | "normal";
  } = {},
) {
  const fetcher = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";
      const governanceRevision = {
        id: "governance-current",
        tenant_id: "tenant-1",
        team_id: "team-1",
        revision_number: 7,
        constitution: {
          hard_rules: ["所有生产写操作必须审批"],
          principles: ["安全优先，稳定可靠"],
        },
        capability_policy: {
          external_capability_bindings: ["告警系统"],
          knowledge_base_bindings: ["运维知识库"],
          mcp_bindings: ["ops-mcp-server"],
          skill_bindings: ["incident-diagnosis"],
        },
        context_policy: {},
        approval_policy: { high_risk: "required" },
        artifact_contract: {},
        internal_collaboration_policy: {},
        runtime_scope_policy: { provider_types: ["codex"] },
        human_owner_user_id: "human-owner-1",
        status: "active",
      };
      const governanceDraft = {
        ...governanceRevision,
        id: "governance-draft-1",
        revision_number: 8,
        status: "draft",
      };

      if (url.pathname === "/api/v1/teams" && method === "GET") {
        const offset = Number(url.searchParams.get("offset") ?? 0);
        if (offset >= 20 && options.secondPageMode === "error") {
          return jsonResponse({ error: "team list unavailable" }, 500);
        }
        const page =
          offset >= 20
            ? options.secondPageMode === "empty"
              ? []
              : Array.from({ length: 5 }, (_, index) =>
                  makeTeamSummary(index + 21),
                )
            : Array.from({ length: 20 }, (_, index) =>
                makeTeamSummary(index + 1),
              );

        return jsonResponse(page);
      }

      if (url.pathname === "/api/auth/users" && method === "GET") {
        const q = url.searchParams.get("q")?.trim().toLowerCase();
        const users = [
          {
            avatar: {
              provider: "dicebear",
              seed: "owner",
              style: "adventurer",
            },
            id: "owner-user",
            status: "active",
            username: "owner",
          },
          {
            avatar: {
              provider: "dicebear",
              seed: "member",
              style: "adventurer",
            },
            id: "member-user",
            status: "active",
            username: "member",
          },
          {
            avatar: {
              provider: "dicebear",
              seed: "viewer",
              style: "adventurer",
            },
            id: "viewer-user",
            status: "active",
            username: "viewer",
          },
        ];

        return jsonResponse({
          items: q
            ? users.filter((user) => user.username.includes(q))
            : users,
        });
      }

      if (url.pathname === "/api/v1/teams" && method === "POST") {
        if (options.createStatus && options.createStatus >= 400) {
          return jsonResponse({ error: "create team unavailable" }, options.createStatus);
        }

        return jsonResponse(
          {
            team: {
              id: "team-security",
              tenant_id: "tenant-1",
              name: "安全团队",
              slug: "security",
              status: "active",
            },
            member_count: 3,
            digital_employee_count: 0,
            capability_count: 0,
            pending_draft_count: 0,
            pending_item_count: 0,
            allowed_actions: [],
          },
          201,
        );
      }

      if (
        url.pathname === "/api/v1/teams/team-1/overview" &&
        method === "GET"
      ) {
        return jsonResponse({
          team: {
            id: "team-1",
            tenant_id: "tenant-1",
            slug: "ops",
            name: "运维团队",
            status: options.disabledOverview ? "disabled" : "active",
            human_owner_user_id: "human-owner-1",
            human_owner: {
              user_id: "human-owner-1",
              username: "owner",
              display_name: "负责人甲",
              email: "owner@example.com",
              status: "active",
            },
          },
          member_count: 18,
          digital_employee_count: 6,
          capability_count: 12,
          pending_draft_count: 3,
          pending_item_count: 3,
          allowed_actions: options.disabledOverview
            ? ["team.restore"]
            : [
                "team.update",
                "team.disable",
                "team.archive",
                "team.member.add",
                "team.member.request_privileged_role",
                "team.governance.edit",
                "team.governance.approve",
              ],
          current_revision: governanceRevision,
        });
      }

      if (
        url.pathname === "/api/v1/teams/team-1/governance/current" &&
        method === "GET"
      ) {
        return jsonResponse(governanceRevision);
      }

      if (
        url.pathname === "/api/v1/teams/team-1/governance/drafts" &&
        method === "GET"
      ) {
        return jsonResponse([governanceDraft]);
      }

      if (
        url.pathname === "/api/v1/teams/team-1/governance/drafts" &&
        method === "POST"
      ) {
        return jsonResponse(governanceDraft, 201);
      }

      if (
        url.pathname ===
          "/api/v1/teams/team-1/governance/drafts/governance-draft-1" &&
        method === "PATCH"
      ) {
        return jsonResponse(governanceDraft);
      }

      if (
        url.pathname ===
          "/api/v1/teams/team-1/governance/drafts/governance-draft-1/approve" &&
        method === "POST"
      ) {
        return jsonResponse({ ...governanceDraft, status: "active" });
      }

      if (
        url.pathname ===
          "/api/v1/teams/team-1/governance/drafts/governance-draft-1/reject" &&
        method === "POST"
      ) {
        return jsonResponse({ ...governanceDraft, status: "rejected" });
      }

      if (
        url.pathname ===
          "/api/v1/teams/team-1/governance/drafts/governance-draft-1/diff" &&
        method === "GET"
      ) {
        return jsonResponse({
          added_hard_rules: 1,
          changed_approval_rules: 1,
          changed_capabilities: 1,
          blocking_errors: [],
          warnings: [
            {
              field: "constitution.hard_rules",
              message: "新增硬性规则需要复核",
              severity: "warning",
            },
          ],
        });
      }

      if (url.pathname === "/api/v1/teams/team-1/members" && method === "GET") {
        return jsonResponse([
          {
            membership_id: "membership-owner",
            tenant_id: "tenant-1",
            team_id: "team-1",
            user_id: "owner-user",
            username: "owner",
            display_name: "负责人甲",
            email: "owner@example.com",
            account_status: "active",
            role: "owner",
            membership_status: "active",
          },
          {
            membership_id: "membership-admin",
            tenant_id: "tenant-1",
            team_id: "team-1",
            user_id: "admin-user",
            username: "admin",
            display_name: "管理员乙",
            email: "admin@example.com",
            account_status: "active",
            role: "admin",
            membership_status: "active",
          },
          {
            membership_id: "membership-approver",
            tenant_id: "tenant-1",
            team_id: "team-1",
            user_id: "approver-user",
            username: "approver",
            display_name: "审批人丙",
            email: "approver@example.com",
            account_status: "active",
            role: "approver",
            membership_status: "active",
          },
          {
            membership_id: "membership-member",
            tenant_id: "tenant-1",
            team_id: "team-1",
            user_id: "roster-member-user",
            username: "member",
            display_name: "普通成员丁",
            email: "member@example.com",
            avatar: {
              provider: "dicebear",
              seed: "roster-member",
              style: "adventurer",
            },
            account_status: "active",
            role: "member",
            membership_status: "active",
          },
          {
            membership_id: "membership-viewer",
            tenant_id: "tenant-1",
            team_id: "team-1",
            user_id: "roster-viewer-user",
            username: "viewer",
            display_name: "观察者戊",
            email: "viewer@example.com",
            avatar: {
              provider: "dicebear",
              seed: "roster-viewer",
              style: "adventurer",
            },
            account_status: "active",
            role: "viewer",
            membership_status: "active",
          },
        ]);
      }

      if (
        url.pathname === "/api/v1/teams/team-1/member-role-requests" &&
        method === "GET"
      ) {
        return jsonResponse([
          {
            id: "request-admin",
            tenant_id: "tenant-1",
            team_id: "team-1",
            target_user_id: "candidate-admin",
            requested_role: "admin",
            requested_by: "owner-user",
            status: "pending",
            reason: "需要维护成员配置",
            decision_reason: "",
          },
        ]);
      }

      if (url.pathname === "/api/v1/teams/team-1/members" && method === "POST") {
        return jsonResponse(
          {
            membership_id: "membership-added",
            tenant_id: "tenant-1",
            team_id: "team-1",
            user_id: "member-user",
            username: "member",
            display_name: "新增成员",
            email: "member-new@example.com",
            account_status: "active",
            role: "member",
            membership_status: "active",
          },
          201,
        );
      }

      if (
        url.pathname === "/api/v1/teams/team-1/member-role-requests" &&
        method === "POST"
      ) {
        return jsonResponse(
          {
            id: "request-viewer-admin",
            tenant_id: "tenant-1",
            team_id: "team-1",
            target_user_id: "viewer-user",
            requested_role: "admin",
            requested_by: "owner-user",
            status: "pending",
            reason: "需要维护团队治理",
            decision_reason: "",
          },
          201,
        );
      }

      if (
        url.pathname === "/api/v1/teams/team-1/disable" &&
        method === "POST"
      ) {
        return jsonResponse({
          id: "team-1",
          tenant_id: "tenant-1",
          slug: "ops",
          name: "运维团队",
          status: "disabled",
        });
      }

      if (
        url.pathname === "/api/v1/teams/team-1/archive" &&
        method === "POST"
      ) {
        return jsonResponse({
          id: "team-1",
          tenant_id: "tenant-1",
          slug: "ops",
          name: "运维团队",
          status: "archived",
        });
      }

      if (
        url.pathname === "/api/v1/teams/team-1/restore" &&
        method === "POST"
      ) {
        return jsonResponse({
          id: "team-1",
          tenant_id: "tenant-1",
          slug: "ops",
          name: "运维团队",
          status: "active",
        });
      }

      if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
        expect(url.searchParams.get("team_id")).toBe("team-1");

        return jsonResponse([
          {
            id: "employee-active",
            team_id: "team-1",
            name: "数据库运维员工",
            role: "database_operator",
            description: "负责数据库变更巡检",
            status: "active",
            risk_level: "medium",
            metadata: {
              effective_config_label: "v5（继承团队）",
              effective_config_status: "approved",
            },
          },
          {
            id: "employee-draft",
            team_id: "team-1",
            name: "发布检查员工",
            role: "release_checker",
            description: "上线前校验发布清单",
            status: "draft",
            risk_level: "low",
            metadata: {
              effective_config_label: "v1（本地草稿）",
              effective_config_status: "draft",
            },
          },
          {
            id: "employee-stale",
            team_id: "team-1",
            name: "缓存运维员工",
            role: "cache_operator",
            description: "处理缓存刷新与回滚",
            status: "active",
            risk_level: "medium",
            metadata: {
              effective_config_label: "v2（继承团队）",
              effective_config_status: "stale",
            },
          },
          {
            id: "employee-unbound",
            team_id: "team-1",
            name: "回归测试员工",
            role: "regression_tester",
            description: "执行回归验证",
            status: "active",
            risk_level: "low",
            metadata: {
              effective_config_label: "v3（继承团队）",
              effective_config_status: "approved",
            },
          },
          ...Array.from({ length: 6 }, (_, index) => ({
            id: `employee-extra-${index + 1}`,
            team_id: "team-1",
            name: `巡检员工 ${index + 1}`,
            role: "inspection_operator",
            description: "执行例行巡检",
            status: "active",
            risk_level: "low",
            metadata: {
              effective_config_label: "v4（继承团队）",
              effective_config_status: "approved",
            },
          })),
          {
            id: "employee-hidden-unbound",
            team_id: "team-1",
            name: "第二页未绑定员工",
            role: "hidden_unbound",
            description: "第二页执行实例不应在首页加载",
            status: "active",
            risk_level: "low",
            metadata: {
              effective_config_label: "v6（继承团队）",
              effective_config_status: "approved",
            },
          },
        ]);
      }

      if (url.pathname === "/api/v1/digital-employees" && method === "POST") {
        expect(JSON.parse(String(init?.body))).toEqual({
          name: "日志分析员工",
          role: "log_analyst",
          description: "分析异常日志",
          team_id: "team-1",
        });

        return jsonResponse({
          id: "employee-created",
          team_id: "team-1",
          name: "日志分析员工",
          role: "log_analyst",
          description: "分析异常日志",
          status: "draft",
        });
      }

      if (
        url.pathname ===
          "/api/v1/digital-employees/employee-unbound/execution-instance" &&
        method === "GET"
      ) {
        return jsonResponse({ error: "not found" }, 404);
      }

      if (url.pathname === "/api/v1/teams/team-1/audit" && method === "GET") {
        expect(url.searchParams.get("limit")).toBe("20");
        expect(url.searchParams.get("offset")).toBe("0");

        return jsonResponse([
          {
            id: "audit-create",
            tenant_id: "tenant-1",
            event_type: "team_management",
            actor_type: "user",
            actor_id: "王一",
            resource_type: "team",
            resource_id: "team-1",
            action: "team.create",
            details: {
              summary: "创建团队“运维团队”",
              result: "success",
              resource_label: "运维团队",
              authorization_action: "team.create",
              before: { name: "-", slug: "-" },
              after: { name: "运维团队", slug: "ops" },
            },
            ip_address: "10.20.2.15",
            created_at: "2026-06-03T09:30:00Z",
          },
          {
            id: "audit-member",
            tenant_id: "tenant-1",
            event_type: "team_management",
            actor_type: "user",
            actor_id: "李娜",
            resource_type: "member",
            resource_id: "member-1",
            action: "team.member.add",
            details: {
              summary: "添加成员 孙悦",
              result: "success",
              resource_label: "孙悦",
              authorization_action: "team.member.add",
              before: { role: "-" },
              after: { role: "operator" },
            },
            ip_address: "10.20.2.16",
            created_at: "2026-06-03T09:20:00Z",
          },
          {
            id: "audit-governance",
            tenant_id: "tenant-1",
            event_type: "team_management",
            actor_type: "user",
            actor_id: "赵强",
            resource_type: "team",
            resource_id: "team-1",
            action: "team.governance.approve",
            details: {
              summary: "批准治理版本 v7",
              result: "success",
              resource_label: "gov_draft_v7",
              authorization_action: "team.governance.approve",
              before: { status: "draft" },
              after: { status: "active" },
            },
            ip_address: "10.20.2.17",
            created_at: "2026-06-03T09:10:00Z",
          },
          {
            id: "audit-capability",
            tenant_id: "tenant-1",
            event_type: "team_management",
            actor_type: "user",
            actor_id: "孙悦",
            resource_type: "capability",
            resource_id: "mcp-1",
            action: "team.capability.bind",
            details: {
              summary: "绑定 MCP 服务",
              result: "success",
              resource_label: "监控告警 MCP",
              authorization_action: "team.capability.bind",
              before: { enabled: false },
              after: { enabled: true },
            },
            ip_address: "10.20.2.18",
            created_at: "2026-06-03T08:55:00Z",
          },
          {
            id: "audit-rejected",
            tenant_id: "tenant-1",
            event_type: "team_management",
            actor_type: "user",
            actor_id: "陈磊",
            resource_type: "team",
            resource_id: "team-1",
            action: "team.archive.confirm",
            details: {
              summary: "归档确认被拒绝",
              result: "rejected",
              resource_label: "team.archive_20260603",
              authorization_action: "team.archive.confirm",
              before: { status: "active" },
              after: { status: "active" },
            },
            ip_address: "10.20.2.19",
            created_at: "2026-06-03T08:40:00Z",
          },
        ]);
      }

      if (
        url.pathname.startsWith("/api/v1/digital-employees/") &&
        url.pathname.endsWith("/execution-instance")
      ) {
        const pathParts = url.pathname.split("/");
        const employeeId = pathParts[pathParts.length - 2];

        return jsonResponse({
          id: `instance-${employeeId}`,
          digital_employee_id: employeeId,
          runtime_node_id:
            employeeId === "employee-active"
              ? "ops-node-01"
              : employeeId === "employee-draft"
                ? "ops-node-review"
                : "ops-node-02",
          provider_type: "codex",
          status: "ready",
        });
      }

      return jsonResponse({ error: `unhandled ${url.pathname}` }, 404);
    },
  ) as unknown as typeof fetch;

  return fetcher;
}

async function renderWithQueryClient(children: ReactNode) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      {children}
    </QueryClientProvider>,
  );
}

async function openCreateTeamMembersStep(
  screen: Awaited<ReturnType<typeof renderWithQueryClient>>,
) {
  await userEvent.click(screen.getByRole("button", { name: "新建团队" }));
  await userEvent.fill(
    screen.getByRole("textbox", { name: "团队名称", exact: true }),
    "安全团队",
  );
  await userEvent.fill(
    screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
    "security",
  );
  await userEvent.type(
    screen.getByRole("searchbox", { name: "负责人" }),
    "owner",
  );
  await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await expect
    .element(screen.getByRole("heading", { name: "基础信息" }))
    .toBeVisible();
}

async function selectRoleFilter(
  screen: Awaited<ReturnType<typeof renderWithQueryClient>>,
  optionName: string,
) {
  await userEvent.click(screen.getByRole("combobox", { name: "角色筛选" }));
  await userEvent.click(screen.getByRole("option", { name: optionName }));
}

async function changeCandidateRole(
  screen: Awaited<ReturnType<typeof renderWithQueryClient>>,
  label: string,
  optionName: string,
) {
  await userEvent.click(screen.getByRole("combobox", { name: label }));
  await userEvent.click(screen.getByRole("option", { name: optionName }));
}

function createTeamPostBody(fetcher: typeof fetch) {
  const postCall = fetchCalls(fetcher).find(
    ([url, init]) =>
      String(url).endsWith("/api/v1/teams") && init?.method === "POST",
  );

  return JSON.parse(String(postCall?.[1]?.body));
}

describe("TeamsView", () => {
  it("renders a dense team summary table", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await expect
      .element(screen.getByRole("heading", { name: "团队管理" }))
      .toBeVisible();
    for (const column of [
      "负责人",
      "成员",
      "数字员工",
      "能力",
      "治理状态",
      "当前版本",
      "待批准",
    ]) {
      await expect
        .element(screen.getByRole("cell", { name: column, exact: true }))
        .toBeVisible();
    }
    await expect.element(screen.getByText("运维团队")).toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "运维团队" }))
      .toHaveAttribute("data-router-link", "true");
    await expect
      .element(screen.getByRole("link", { name: "运维团队" }))
      .toHaveAttribute("href", "/teams/team-1");
    await expect
      .element(screen.getByLabelText("运维团队图标"))
      .toBeVisible();
    await expect
      .element(screen.getByText("负责人甲", { exact: true }))
      .toBeVisible();
    await expect
      .element(screen.getByText("owner@example.com", { exact: true }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("cell", { name: "18" }).first())
      .toBeVisible();
    await expect
      .element(screen.getByRole("cell", { name: "v7" }).first())
      .toBeVisible();
    await expect
      .element(screen.getByRole("cell", { name: "3" }).first())
      .toBeVisible();
  });

  it("paginates team summaries and opens row actions", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await expect.element(screen.getByText("第 1 页")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "上一页" }))
      .toBeDisabled();
    await expect
      .element(screen.getByRole("button", { name: "下一页" }))
      .toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "下一页" }));
    expect(
      fetchCalls(fetcher).some(([input]) =>
        String(input).includes("limit=20&offset=20"),
      ),
    ).toBe(true);
    await expect.element(screen.getByText("第 2 页")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "下一页" }))
      .toBeDisabled();
    await userEvent.click(
      screen.getByRole("button", { name: "团队 21 (team-21) 行操作" }),
    );
    await expect
      .element(screen.getByRole("menuitem", { name: "查看详情" }))
      .toBeInTheDocument();
    await expect
      .element(screen.getByRole("menuitem", { name: "查看详情" }))
      .toHaveAttribute("data-router-link", "true");
  });

  it("opens row actions for teams with duplicate names by unique slug label", async () => {
    const teams = [
      {
        ...makeTeamSummary(1),
        id: "team-platform-a",
        name: "平台团队",
        slug: "platform-a",
      },
      {
        ...makeTeamSummary(2),
        id: "team-platform-b",
        name: "平台团队",
        slug: "platform-b",
      },
    ];
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input));
      if (url.pathname === "/api/v1/teams") {
        return jsonResponse(teams);
      }
      return jsonResponse({});
    }) as unknown as typeof fetch;
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "平台团队 (platform-a) 行操作" }),
    );
    await expect
      .element(screen.getByRole("menuitem", { name: "查看详情" }))
      .toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await userEvent.click(
      screen.getByRole("button", { name: "平台团队 (platform-b) 行操作" }),
    );
    await expect
      .element(screen.getByRole("menuitem", { name: "查看详情" }))
      .toBeInTheDocument();
  });

  it("keeps pagination recoverable on an empty page", async () => {
    const fetcher = createTeamsFetcher({ secondPageMode: "empty" });
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "下一页" }));

    await expect.element(screen.getByText("第 2 页")).toBeInTheDocument();
    await expect.element(screen.getByText("暂无团队")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "上一页" }))
      .toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "上一页" }));
    await expect.element(screen.getByText("第 1 页")).toBeInTheDocument();
  });

  it("keeps pagination recoverable on an error page", async () => {
    const fetcher = createTeamsFetcher({ secondPageMode: "error" });
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "下一页" }));

    await expect.element(screen.getByText("第 2 页")).toBeInTheDocument();
    await expect
      .element(screen.getByText("团队列表加载失败"))
      .toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "上一页" }))
      .toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "上一页" }));
    await expect.element(screen.getByText("第 1 页")).toBeInTheDocument();
  });

  it("filters team summaries through the real list endpoint", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input));
      if (url.pathname === "/api/v1/teams") {
        return jsonResponse([]);
      }
      return jsonResponse({});
    }) as unknown as typeof fetch;

    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await expect.element(screen.getByText("团队管理")).toBeInTheDocument();
    await userEvent.type(
      screen.getByPlaceholder("搜索团队名称、slug、负责人"),
      "安全",
    );
    await userEvent.selectOptions(screen.getByLabelText("团队状态"), "active");
    await userEvent.selectOptions(
      screen.getByLabelText("治理状态"),
      "draft_pending",
    );

    await expect
      .poll(() => fetchCalls(fetcher).map(([url]) => String(url)))
      .toContain(
        "http://control-plane.local/api/v1/teams?status=active&governance_status=draft_pending&q=%E5%AE%89%E5%85%A8&limit=20&offset=0",
      );
  });

  it("resets pagination when filters or page size change", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "下一页" }));
    await expect.element(screen.getByText("第 2 页")).toBeInTheDocument();

    await userEvent.selectOptions(screen.getByLabelText("团队状态"), "active");

    await expect.element(screen.getByText("第 1 页")).toBeInTheDocument();
    expect(
      fetchCalls(fetcher).some(([input]) =>
        String(input).includes("status=active&limit=20&offset=0"),
      ),
    ).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: "下一页" }));
    await expect.element(screen.getByText("第 2 页")).toBeInTheDocument();
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "每页数量" }),
      "50",
    );

    await expect.element(screen.getByText("第 1 页")).toBeInTheDocument();
    expect(
      fetchCalls(fetcher).some(([input]) =>
        String(input).includes("limit=50&offset=0"),
      ),
    ).toBe(true);
  });

  it("opens create team drawer and requires name slug and owner before next step", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "新建团队" }));
    await expect
      .element(screen.getByRole("heading", { name: "新建团队" }))
      .toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect
      .element(screen.getByText("团队名称不能为空"))
      .toBeInTheDocument();
    await expect
      .element(screen.getByText("团队标识不能为空"))
      .toBeInTheDocument();
    await expect.element(screen.getByText("请选择负责人")).toBeInTheDocument();
  });

  it("creates team with display metadata and editable initial members", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "新建团队" }));
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "选择安全团队图标" }),
    );
    await userEvent.type(
      screen.getByRole("searchbox", { name: "负责人" }),
      "owner",
    );
    await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect
      .element(screen.getByRole("heading", { name: "基础信息" }))
      .toBeVisible();
    await userEvent.click(screen.getByLabelText("选择 member 为初始成员"));
    await userEvent.click(screen.getByLabelText("选择 viewer 为初始成员"));
    await changeCandidateRole(screen, "viewer 初始角色", "只读观察者");
    await userEvent.click(screen.getByRole("button", { name: "移除 viewer" }));
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect.poll(() => createTeamPostIndex(fetcher)).not.toBe(-1);
    expect(createTeamPostBody(fetcher)).toEqual({
      human_owner_user_id: "owner-user",
      initial_members: [{ role: "member", user_id: "member-user" }],
      metadata: { display: { color_tone: "teal", icon_key: "security" } },
      name: "安全团队",
      slug: "security",
    });
  });

  it("removes the selected owner from initial members when owner changes", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await openCreateTeamMembersStep(screen);
    await userEvent.click(screen.getByLabelText("选择 member 为初始成员"));
    await userEvent.click(screen.getByRole("button", { name: "上一步" }));
    await userEvent.type(
      screen.getByRole("searchbox", { name: "负责人" }),
      "member",
    );
    await userEvent.click(screen.getByRole("button", { name: "选择 member" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect.poll(() => createTeamPostIndex(fetcher)).not.toBe(-1);
    const postBody = createTeamPostBody(fetcher);
    expect(postBody).toEqual({
      human_owner_user_id: "member-user",
      initial_members: [],
      metadata: { display: { color_tone: "teal", icon_key: "security" } },
      name: "安全团队",
      slug: "security",
    });
    expect(postBody.initial_members).not.toContainEqual(
      expect.objectContaining({ user_id: postBody.human_owner_user_id }),
    );
  });

  it("continues inferred display until the icon is manually selected", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "新建团队" }));
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await expect
      .element(screen.getByRole("button", { name: "选择安全团队图标" }))
      .toHaveAttribute("aria-pressed", "true");
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "ops",
    );
    await expect
      .element(screen.getByRole("button", { name: "选择运维团队图标" }))
      .toHaveAttribute("aria-pressed", "true");

    await userEvent.click(
      screen.getByRole("button", { name: "选择安全团队图标" }),
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "dev",
    );
    await expect
      .element(screen.getByRole("button", { name: "选择安全团队图标" }))
      .toHaveAttribute("aria-pressed", "true");
  });

  it("filters create member candidates by effective candidate role", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await openCreateTeamMembersStep(screen);

    await selectRoleFilter(screen, "普通成员");
    await expect
      .element(screen.getByLabelText("选择 member 为初始成员"))
      .toBeVisible();
    await expect
      .element(screen.getByLabelText("选择 viewer 为初始成员"))
      .toBeVisible();

    await selectRoleFilter(screen, "全部");
    await changeCandidateRole(screen, "viewer 初始角色", "只读观察者");
    await selectRoleFilter(screen, "只读观察者");

    await expect
      .element(screen.getByLabelText("选择 viewer 为初始成员"))
      .toBeVisible();
    await expect
      .element(screen.getByLabelText("选择 member 为初始成员"))
      .not.toBeInTheDocument();
  });

  it("does not submit unselected candidates after changing candidate role", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await openCreateTeamMembersStep(screen);
    await expect
      .element(screen.getByLabelText("选择 viewer 为初始成员"))
      .toBeVisible();
    await changeCandidateRole(screen, "viewer 初始角色", "只读观察者");

    await expect
      .element(screen.getByText("已选择的初始成员（0）"))
      .toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect.poll(() => createTeamPostIndex(fetcher)).not.toBe(-1);
    expect(createTeamPostBody(fetcher)).toEqual({
      human_owner_user_id: "owner-user",
      initial_members: [],
      metadata: { display: { color_tone: "teal", icon_key: "security" } },
      name: "安全团队",
      slug: "security",
    });
  });

  it("resets the create drawer draft after a successful create", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await openCreateTeamMembersStep(screen);
    await userEvent.click(screen.getByLabelText("选择 member 为初始成员"));
    await expect
      .element(screen.getByText("已选择的初始成员（1）"))
      .toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect.poll(() => createTeamPostIndex(fetcher)).not.toBe(-1);
    await expect
      .element(screen.getByRole("heading", { name: "新建团队" }))
      .not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "新建团队" }));

    await expect
      .element(screen.getByRole("textbox", { name: "团队名称", exact: true }))
      .toHaveValue("");
    await expect
      .element(screen.getByRole("textbox", { name: "团队标识 slug", exact: true }))
      .toHaveValue("");
    await expect
      .element(screen.getByRole("heading", { name: "基础信息" }))
      .not.toBeInTheDocument();
    await expect
      .element(screen.getByText("已选择的初始成员（1）"))
      .not.toBeInTheDocument();
  });

  it("clears a failed create error after closing and reopening the drawer", async () => {
    const fetcher = createTeamsFetcher({ createStatus: 500 });
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await openCreateTeamMembersStep(screen);
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect.poll(() => createTeamPostIndex(fetcher)).not.toBe(-1);
    await expect
      .element(screen.getByText("create team request failed with status 500"))
      .toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "取消" }));
    await expect
      .element(screen.getByRole("heading", { name: "新建团队" }))
      .not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "新建团队" }));

    await expect
      .element(screen.getByText("create team request failed with status 500"))
      .not.toBeInTheDocument();
  });

  it("refetches the first team page after creating while already on the first page", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await expect.element(screen.getByText("第 1 页")).toBeInTheDocument();
    expect(teamSummaryGetCalls(fetcher, 0)).toHaveLength(1);

    await userEvent.click(screen.getByRole("button", { name: "新建团队" }));
    await userEvent.type(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.type(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await userEvent.type(
      screen.getByRole("searchbox", { name: "负责人" }),
      "owner",
    );
    await expect
      .poll(() => fetchCalls(fetcher).map(([url]) => String(url)))
      .toContain(
        "http://control-plane.local/api/auth/users?q=owner&status=active&limit=20&offset=0",
      );
    await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect.poll(() => createTeamPostIndex(fetcher)).not.toBe(-1);
    const postIndex = createTeamPostIndex(fetcher);
    await expect
      .poll(
        () =>
          fetchCalls(fetcher)
            .slice(postIndex + 1)
            .filter(([url, init]) => {
              const requestUrl = new URL(String(url));

              return (
                requestUrl.pathname === "/api/v1/teams" &&
                requestUrl.searchParams.get("limit") === "20" &&
                requestUrl.searchParams.get("offset") === "0" &&
                init?.method === "GET"
              );
            }).length,
      )
      .toBe(1);
  });

  it("creates a team with selected owner and initial members", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "下一页" }));
    await expect.element(screen.getByText("第 2 页")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "新建团队" }));
    await userEvent.type(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.type(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await userEvent.type(
      screen.getByRole("searchbox", { name: "负责人" }),
      "owner",
    );
    await expect
      .poll(() => fetchCalls(fetcher).map(([url]) => String(url)))
      .toContain(
        "http://control-plane.local/api/auth/users?q=owner&status=active&limit=20&offset=0",
      );
    await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByLabelText("选择 member 为初始成员"));
    await userEvent.click(screen.getByLabelText("选择 viewer 为初始成员"));
    await changeCandidateRole(screen, "viewer 初始角色", "只读观察者");
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect
      .poll(() =>
        fetchCalls(fetcher).some(
          ([url, init]) =>
            String(url).endsWith("/api/v1/teams") && init?.method === "POST",
        ),
      )
      .toBe(true);
    expect(createTeamPostBody(fetcher)).toEqual({
      name: "安全团队",
      slug: "security",
      human_owner_user_id: "owner-user",
      initial_members: [
        { user_id: "member-user", role: "member" },
        { user_id: "viewer-user", role: "viewer" },
      ],
      metadata: { display: { color_tone: "teal", icon_key: "security" } },
    });
    await expect.element(screen.getByText("第 1 页")).toBeInTheDocument();
    const postIndex = fetchCalls(fetcher).findIndex(
      ([url, init]) =>
        String(url).endsWith("/api/v1/teams") && init?.method === "POST",
    );
    expect(
      fetchCalls(fetcher)
        .slice(postIndex + 1)
        .filter(
          ([url, init]) =>
            String(url).includes("limit=20&offset=0") &&
            init?.method === "GET",
        ),
    ).toHaveLength(1);
    expect(
      fetchCalls(fetcher)
        .slice(postIndex + 1)
        .filter(
          ([url, init]) =>
            String(url).includes("limit=20&offset=20") &&
            init?.method === "GET",
        ),
    ).toHaveLength(0);
  });
});

describe("TeamDetailView", () => {
  it("renders detail tabs for the team shell", async () => {
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={createTeamsFetcher()}
        teamId="team-1"
      />,
    );

    await expect
      .element(screen.getByRole("heading", { name: "运维团队" }))
      .toBeVisible();
    for (const tab of [
      "概览",
      "成员",
      "数字员工",
      "能力与知识",
      "治理策略",
      "审计记录",
    ]) {
      await expect
        .element(screen.getByRole("tab", { name: tab }))
        .toBeVisible();
    }
    await screen.getByRole("tab", { name: "成员" }).click();
    await expect.element(screen.getByText("成员名册")).toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "添加成员" }).first())
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "创建治理草案" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "禁用团队" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "归档团队" }))
      .toBeVisible();
  });

  it("calls lifecycle APIs from detail actions", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await screen.getByRole("button", { name: "禁用团队" }).click();

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/disable",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );

    await screen.getByRole("button", { name: "归档团队" }).click();

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/archive",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );
  });

  it("renders capabilities and saves binding changes as a governance draft", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "能力与知识" }));

    for (const section of ["Skills", "MCP", "知识库", "外部能力"]) {
      await expect
        .element(screen.getByText(section, { exact: true }))
        .toBeVisible();
    }
    await expect.element(screen.getByText("绑定不会立即生效")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "保存绑定草稿" }));

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/governance/drafts/governance-draft-1",
      expect.objectContaining({
        credentials: "include",
        method: "PATCH",
      }),
    );
  });

  it("renders governance editor with JSON preview and approval action", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "治理策略" }));

    await expect.element(screen.getByLabelText("团队宪法")).toBeVisible();
    await expect.element(screen.getByLabelText("审批策略")).toBeVisible();
    await expect.element(screen.getByText("JSON 快照预览")).toBeVisible();
    await expect
      .element(screen.getByText("新增硬性规则需要复核"))
      .toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "保存草稿" }));
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/governance/drafts/governance-draft-1",
      expect.objectContaining({
        credentials: "include",
        method: "PATCH",
      }),
    );

    await userEvent.click(
      screen.getByRole("button", { name: "提交负责人批准" }),
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/governance/drafts/governance-draft-1/approve",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );

    await userEvent.click(screen.getByRole("button", { name: "驳回草稿" }));
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/governance/drafts/governance-draft-1/reject",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );
  });

  it("does not show member or governance creation actions for a disabled team", async () => {
    const fetcher = createTeamsFetcher({ disabledOverview: true });
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await expect
      .element(screen.getByRole("heading", { name: "运维团队" }))
      .toBeVisible();
    await expect.element(screen.getByText("已禁用")).toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "添加成员" }))
      .not.toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "创建治理草案" }))
      .not.toBeInTheDocument();
    await screen.getByRole("button", { name: "恢复团队" }).click();
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/restore",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );
  });

  it("renders members tab roster, safe direct roles, pending requests, and owner protection", async () => {
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={createTeamsFetcher()}
        teamId="team-1"
      />,
    );

    await screen.getByRole("tab", { name: "成员" }).click();

    await expect.element(screen.getByText("负责人甲", { exact: true })).toBeVisible();
    for (const label of [
      "人类成员",
      "负责人",
      "管理员",
      "审批人",
      "直接生效角色",
    ]) {
      expect(document.body.textContent).toContain(label);
    }
    await expect.element(screen.getByText("管理员乙")).toBeVisible();
    await expect.element(screen.getByText("admin@example.com", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("审批人丙")).toBeVisible();
    await expect.element(screen.getByText("普通成员丁")).toBeVisible();
    await expect.element(screen.getByText("观察者戊")).toBeVisible();
    await expect
      .element(
        screen.getByText(
          "删除或禁用最后一位负责人会被控制平面拒绝；请先完成负责人交接，再移除原负责人。",
        ),
      )
      .toBeVisible();

    await expect.element(screen.getByText("candidate-admin")).toBeVisible();
    await expect
      .element(screen.getByRole("combobox", { name: "直接生效角色" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("combobox", { name: "申请角色" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "拒绝" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "审批" }))
      .toBeVisible();
  });

  it("uses user search for direct member add and privileged role requests", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "成员" }));

    await expect
      .element(screen.getByRole("searchbox", { name: "搜索直接添加用户" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("searchbox", { name: "搜索高权限申请目标用户" }))
      .toBeVisible();
    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜索直接添加用户" }),
      "member",
    );
    await userEvent.click(
      screen.getByRole("button", { name: /member/ }).first(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "添加成员" }).last(),
    );

    await expect
      .poll(() =>
        fetchCalls(fetcher).find(
          ([url, init]) =>
            String(url).endsWith("/api/v1/teams/team-1/members") &&
            init?.method === "POST",
        ),
      )
      .toBeTruthy();
    const addMemberCall = fetchCalls(fetcher).find(
      ([url, init]) =>
        String(url).endsWith("/api/v1/teams/team-1/members") &&
        init?.method === "POST",
    );
    expect(JSON.parse(String(addMemberCall?.[1]?.body))).toMatchObject({
      role: "member",
      user_id: "member-user",
    });
    await expect
      .element(screen.getByRole("button", { name: "添加成员" }).last())
      .toBeDisabled();

    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜索高权限申请目标用户" }),
      "viewer",
    );
    await userEvent.click(screen.getByRole("button", { name: /viewer/ }));
    await userEvent.type(
      screen.getByLabelText("申请原因"),
      "需要维护团队治理",
    );
    await userEvent.click(screen.getByRole("button", { name: "提交申请" }));

    await expect
      .poll(() =>
        fetchCalls(fetcher).find(
          ([url, init]) =>
            String(url).endsWith(
              "/api/v1/teams/team-1/member-role-requests",
            ) && init?.method === "POST",
        ),
      )
      .toBeTruthy();
    const requestCall = fetchCalls(fetcher).find(
      ([url, init]) =>
        String(url).endsWith("/api/v1/teams/team-1/member-role-requests") &&
        init?.method === "POST",
    );
    expect(JSON.parse(String(requestCall?.[1]?.body))).toMatchObject({
      reason: "需要维护团队治理",
      requested_role: "admin",
      target_user_id: "viewer-user",
    });
    await expect.element(screen.getByLabelText("申请原因")).toHaveValue("");
    await expect
      .element(screen.getByRole("button", { name: "提交申请" }))
      .toBeDisabled();
    await expect
      .element(screen.getByRole("button", { name: "添加成员" }).last())
      .toBeDisabled();
    expect(
      fetchCalls(fetcher).filter(
        ([url, init]) =>
          String(url).endsWith("/api/v1/teams/team-1/members") &&
          init?.method === "POST",
      ),
    ).toHaveLength(1);
  });

  it("renders the team digital employees tab with metrics, table, and team-scoped quick create", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "数字员工" }));

    await expect
      .element(screen.getByText("Plan 4 会接入团队数字员工列表和快速创建。"))
      .not.toBeInTheDocument();
    await expect
      .element(screen.getByRole("group", { name: "数字员工 11 总数" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("group", { name: "active 10 正常运行" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("group", { name: "draft 1 未发布" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("group", { name: "继承配置过期 1 需更新" }))
      .toBeVisible();
    await expect
      .element(
        screen.getByRole("group", { name: "未绑定 Runtime 1 当前页需绑定" }),
      )
      .toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "从此团队创建数字员工" }))
      .toBeVisible();

    for (const column of [
      "数字员工",
      "角色",
      "状态",
      "风险",
      "生效配置",
      "执行实例",
    ]) {
      await expect
        .element(screen.getByRole("cell", { name: column }))
        .toBeVisible();
    }
    await expect.element(screen.getByText("数据库运维员工")).toBeVisible();
    await expect.element(screen.getByText("v3（继承团队）")).toBeVisible();
    await expect.element(screen.getByText("ops-node-01")).toBeVisible();

    await expect
      .element(
        screen.getByText(
          "新数字员工需要选择专业类型、能力、治理和 Runtime 绑定，请进入创建向导完成。",
        ),
      )
      .toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "从此团队创建数字员工" }))
      .toHaveAttribute("href", "/employees/new");
    await expect
      .element(screen.getByRole("link", { name: "从此团队创建数字员工" }))
      .toHaveAttribute("data-router-link", "true");
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees?team_id=team-1",
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      }),
    );
    expect(fetcher).not.toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees",
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetcher).not.toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee-hidden-unbound/execution-instance",
      expect.anything(),
    );
  });

  it("renders the team audit tab with summary, authorization action, and before after detail", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "审计记录" }));

    await expect
      .element(screen.getByText("Plan 4 会接入团队审计记录。"))
      .not.toBeInTheDocument();
    await expect
      .element(screen.getByRole("group", { name: "今日操作 0 24 小时内" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("group", { name: "成员变更 1 成员与角色" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("group", { name: "治理版本 1 草案与审批" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("group", { name: "能力绑定 1 外部能力" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("group", { name: "被拒绝 1 需复核" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("cell", { name: "授权动作" }))
      .toBeVisible();
    await expect.element(screen.getByText("team.create")).toBeVisible();
    await expect.element(screen.getByText("创建团队“运维团队”")).toBeVisible();

    await userEvent.click(screen.getByRole("row", { name: /添加成员 孙悦/ }));

    await expect.element(screen.getByText("事件详情")).toBeVisible();
    await expect
      .element(screen.getByText("授权动作：team.member.add"))
      .toBeVisible();
    await expect
      .element(screen.getByText("变更内容（前后对比）"))
      .toBeVisible();
    await expect.element(screen.getByText('"role": "-"')).toBeVisible();
    await expect.element(screen.getByText('"role": "operator"')).toBeVisible();
  });
});
