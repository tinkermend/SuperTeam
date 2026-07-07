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

  return { Link, useNavigate: () => () => undefined };
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

type ExtraRoutes = Record<string, unknown>;

function routeKey(input: RequestInfo | URL, init?: RequestInit) {
  const url = new URL(String(input));
  const method = init?.method ?? "GET";

  return `${method} ${url.pathname}${url.search}`;
}

function routeResponse(
  extraRoutes: ExtraRoutes | undefined,
  input: RequestInfo | URL,
  init?: RequestInit,
) {
  if (!extraRoutes) {
    return undefined;
  }
  const key = routeKey(input, init);
  if (!Object.prototype.hasOwnProperty.call(extraRoutes, key)) {
    return undefined;
  }
  const method = init?.method ?? "GET";
  const status = method === "POST" ? 201 : method === "DELETE" ? 200 : 200;

  return jsonResponse(extraRoutes[key], status);
}



function makeTeamSummary(index: number) {
  const isPrimary = index === 1;

  return {
    id: `team-${index}`,
    tenant_id: "tenant-1",
    slug: isPrimary ? "ops" : `team-${index}`,
    name: isPrimary ? "运维团队" : `团队 ${index}`,
    status: "active",
    human_owner_user_ids: [isPrimary ? "human-owner-1" : `human-owner-${index}`],
    human_owners: [{
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
    }],
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
    extraRoutes?: ExtraRoutes;
    secondPageMode?: "empty" | "error" | "normal";
  } = {},
) {
  const fetcher = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const extraRouteResponse = routeResponse(options.extraRoutes, input, init);
      if (extraRouteResponse) {
        return extraRouteResponse;
      }
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
        human_owner_user_ids: ["human-owner-1"],
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
            human_owner_user_ids: ["human-owner-1"],
            human_owners: [{user_id: "human-owner-1",
              username: "owner",
              display_name: "负责人甲",
              email: "owner@example.com",
              status: "active",}],
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
        const teamId = url.searchParams.get("team_id");
        if (teamId !== "team-1") {
          return jsonResponse([]);
        }

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

function requestBody(fetcher: typeof fetch, pathname: string, method: string) {
  const call = fetchCalls(fetcher).find(([url, init]) => {
    const requestUrl = new URL(String(url));

    return requestUrl.pathname === pathname && init?.method === method;
  });

  return JSON.parse(String(call?.[1]?.body));
}

function hasRequest(fetcher: typeof fetch, pathname: string, method: string) {
  return fetchCalls(fetcher).some(([url, init]) => {
    const requestUrl = new URL(String(url));

    return requestUrl.pathname === pathname && init?.method === method;
  });
}

describe("TeamsView", () => {
  it("renders team card grid with summary stats", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await expect
      .element(screen.getByRole("heading", { name: "团队管理" }))
      .toBeVisible();

    // Summary stats
    await expect.element(screen.getByText("20 个团队")).toBeVisible();
    await expect.element(screen.getByText("25 位 agent")).toBeVisible();

    // Card details
    await expect.element(screen.getByText("运维团队")).toBeVisible();
    
    const links = screen.getByRole("link", { name: /查看完整部门/ }).all();
    await expect.element(links[0]).toHaveAttribute("data-router-link", "true");
    await expect.element(links[0]).toHaveAttribute("href", "/teams/team-1");
    
    await expect
      .element(screen.getByLabelText("运维团队图标"))
      .toBeVisible();
    await expect
      .element(screen.getByText("负责人甲", { exact: true }))
      .toBeVisible();
    await expect
      .element(screen.getByText("owner@example.com", { exact: true }))
      .toBeVisible();
      
    const employeeCounts = screen.getByText("6 位数字员工").all();
    await expect.element(employeeCounts[0]).toBeVisible();
    
    const levelLabels = screen.getByText("L1").all();
    await expect.element(levelLabels[0]).toBeVisible();

    expect(document.querySelectorAll('[data-slot="v3-glass-card"]').length).toBeGreaterThan(0);
    expect(document.querySelectorAll('[data-slot="v3-status-pill"]').length).toBeGreaterThan(0);
  });

  it("keeps long team names inside the card header", async () => {
    const longTeamName =
      "Codex Execution Ledger Smoke 20260628-171756-very-long-team-name-for-layout-regression";
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";
      if (url.pathname === "/api/v1/teams" && method === "GET") {
        return jsonResponse([
          {
            ...makeTeamSummary(1),
            digital_employee_count: 1,
            id: "team-long",
            name: longTeamName,
            slug: "codex-execution-ledger-smoke",
          },
        ]);
      }
      if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
        return jsonResponse([]);
      }

      return jsonResponse({ error: `unhandled ${url.pathname}` }, 404);
    }) as unknown as typeof fetch;

    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    const headingLocator = screen.getByRole("heading", { name: longTeamName });
    await expect.element(headingLocator).toBeVisible();
    const heading = headingLocator.element();
    const textColumn = heading.parentElement;
    const identityGroup = textColumn?.parentElement;
    const cardHeader = identityGroup?.parentElement;

    await expect.element(screen.getByText("L1")).toBeVisible();
    expect(heading).toHaveAttribute("title", longTeamName);
    expect(heading).toHaveClass("line-clamp-2");
    expect(textColumn).toHaveClass("min-w-0");
    expect(textColumn).toHaveClass("flex-1");
    expect(identityGroup).toHaveClass("min-w-0");
    expect(identityGroup).toHaveClass("flex-1");
    expect(cardHeader).toHaveClass("min-w-0");
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
      screen.getByPlaceholder("搜索团队名称、slug、负责人..."),
      "安全",
    );
    await userEvent.click(screen.getByRole("combobox", { name: "团队状态" }));
    await userEvent.click(screen.getByRole("option", { name: "活跃" }));

    await userEvent.click(screen.getByRole("combobox", { name: "治理状态" }));
    await userEvent.click(screen.getByRole("option", { name: "草案待批准" }));

    await expect
      .poll(() => fetchCalls(fetcher).map(([url]) => String(url)))
      .toContain(
        "http://control-plane.local/api/v1/teams?status=active&governance_status=draft_pending&q=%E5%AE%89%E5%85%A8",
      );
  });

  it("links the create action to the new team route", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    const createLink = screen.getByRole("link", { name: "新建团队" });
    await expect.element(createLink).toHaveAttribute("href", "/teams/new");
    await expect
      .element(createLink)
      .toHaveAttribute("data-router-link", "true");
  });
});

describe("TeamDetailView", () => {
  it("shows only overview, capabilities, and governance tabs on team detail", async () => {
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
    for (const tab of ["概览", "能力与知识", "治理策略"]) {
      await expect.element(screen.getByRole("tab", { name: tab })).toBeVisible();
    }
    await expect
      .element(screen.getByRole("tab", { name: "借调" }))
      .not.toBeInTheDocument();
    await expect
      .element(screen.getByRole("tab", { name: "审计记录" }))
      .not.toBeInTheDocument();
  });

  it("separates digital employees from human management members in overview", async () => {
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={createTeamsFetcher()}
        teamId="team-1"
      />,
    );

    await expect.element(screen.getByRole("heading", { name: "数字员工" })).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "人类管理成员" })).toBeVisible();
    await expect.element(screen.getByText("数据库运维员工")).toBeVisible();
    await expect.element(screen.getByText("负责人甲", { exact: true })).toBeVisible();
    await expect
      .element(screen.getByText("团队成员与代理"))
      .not.toBeInTheDocument();
  });

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
    for (const tab of ["概览", "能力与知识", "治理策略"]) {
      await expect
        .element(screen.getByRole("tab", { name: tab }))
        .toBeVisible();
    }
    await expect.element(screen.getByRole("heading", { name: "数字员工" })).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "人类管理成员" })).toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "创建治理草案" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "禁用团队" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "归档团队" }))
      .toBeVisible();

    expect(document.querySelectorAll('[data-slot="v3-tabs"]').length).toBeGreaterThan(0);
    expect(document.querySelectorAll('[data-slot="v3-work-surface"]').length).toBeGreaterThan(0);
    expect(document.querySelectorAll('[data-slot="v3-table"]').length).toBeGreaterThan(0);
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

  it("manages team public skills and MCP servers", async () => {
    const installedSkill = {
      id: "skill-observe",
      tenant_id: "tenant-1",
      slug: "observe",
      name: "observe",
      description: "观测巡检",
      version: "1.0.0",
      source: "upload",
      risk_level: "low",
      icon_key: "network",
      color_token: "info",
      tags: ["ops"],
      archive_object_ref: "s3://bucket/skills/observe.zip",
      archive_filename: "observe.zip",
      archive_size_bytes: 1024,
      archive_checksum_sha256: "abc123",
      archive_file_count: 2,
      created_by: "user-1",
      created_by_name: "开发管理员",
      team_bindings: [{ team_id: "team-1", team_name: "运维团队" }],
      agent_bindings: [],
    };
    const installableSkill = {
      id: "skill-diagnose",
      tenant_id: "tenant-1",
      slug: "diagnose",
      name: "diagnose",
      description: "故障诊断",
      version: "1.0.0",
      source: "upload",
      risk_level: "medium",
      icon_key: "boxes",
      color_token: "warning",
      tags: ["incident"],
      archive_object_ref: "s3://bucket/skills/diagnose.zip",
      archive_filename: "diagnose.zip",
      archive_size_bytes: 2048,
      archive_checksum_sha256: "def456",
      archive_file_count: 1,
      created_by: "user-1",
      created_by_name: "开发管理员",
      team_bindings: [],
      agent_bindings: [],
    };
    const fetcher = createTeamsFetcher({
      extraRoutes: {
        "GET /api/v1/skills": [installedSkill, installableSkill],
        "GET /api/v1/teams/team-1/skills": [installedSkill],
        "GET /api/v1/mcp-servers": [
          {
            id: "mcp-github",
            tenant_id: "tenant-1",
            name: "GitHub MCP",
            server_key: "github",
            description: "",
            transport: "streamable_http",
            url: "https://api.githubcopilot.com/mcp/",
            auth_strategy: "bearer_env",
            required_env_vars: ["GITHUB_TOKEN"],
            optional_env_vars: [],
            tool_allowlist: [],
            risk_level: "medium",
            status: "active",
          },
        ],
        "GET /api/v1/teams/team-1/mcp-bindings": [
          {
            id: "binding-existing",
            tenant_id: "tenant-1",
            team_id: "team-1",
            mcp_server_id: "mcp-github",
            server_key: "github",
            server_name: "GitHub MCP",
            url: "https://api.githubcopilot.com/mcp/",
            transport: "streamable_http",
            auth_strategy: "bearer_env",
            credential_env_var: "GITHUB_TOKEN",
            required_env_vars: ["GITHUB_TOKEN"],
            source_scope: "team",
            status: "active",
          },
        ],
        "POST /api/v1/teams/team-1/skills": {
          ...installableSkill,
          team_bindings: [{ team_id: "team-1", team_name: "运维团队" }],
        },
        "POST /api/v1/teams/team-1/mcp-bindings": {
          id: "binding-created",
          tenant_id: "tenant-1",
          team_id: "team-1",
          mcp_server_id: "mcp-github",
          server_key: "github",
          server_name: "GitHub MCP",
          credential_env_var: "GITHUB_TOKEN",
          required_env_vars: ["GITHUB_TOKEN"],
          source_scope: "team",
          status: "active",
        },
        "DELETE /api/v1/teams/team-1/skills/skill-observe": {},
        "DELETE /api/v1/teams/team-1/mcp-bindings/binding-existing": {},
      },
    });
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "能力与知识" }));

    await expect
      .element(screen.getByRole("heading", { name: "公共技能" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("heading", { name: "公共 MCP" }))
      .toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "安装 diagnose" }));
    await expect
      .poll(() =>
        hasRequest(fetcher, "/api/v1/teams/team-1/skills", "POST"),
      )
      .toBe(true);
    expect(requestBody(fetcher, "/api/v1/teams/team-1/skills", "POST")).toEqual({
      skill_id: "skill-diagnose",
    });

    await userEvent.click(screen.getByRole("button", { name: "移除 observe" }));
    await expect
      .poll(() =>
        hasRequest(fetcher, "/api/v1/teams/team-1/skills/skill-observe", "DELETE"),
      )
      .toBe(true);

    // Bind a registered MCP server by mcp_server_id + credential_env_var.
    await userEvent.click(screen.getByRole("combobox", { name: "注册表 MCP" }));
    await userEvent.click(screen.getByRole("option", { name: "GitHub MCP（github）" }));
    await userEvent.fill(
      screen.getByRole("textbox", { name: "凭据环境变量（可选）" }),
      "GITHUB_TOKEN",
    );
    await userEvent.click(screen.getByRole("button", { name: "绑定公共 MCP" }));

    await expect
      .poll(() =>
        hasRequest(fetcher, "/api/v1/teams/team-1/mcp-bindings", "POST"),
      )
      .toBe(true);
    expect(requestBody(fetcher, "/api/v1/teams/team-1/mcp-bindings", "POST")).toEqual({
      mcp_server_id: "mcp-github",
      credential_env_var: "GITHUB_TOKEN",
    });

    await userEvent.click(screen.getByRole("button", { name: "移除 MCP GitHub MCP" }));
    await expect
      .poll(() =>
        hasRequest(fetcher, "/api/v1/teams/team-1/mcp-bindings/binding-existing", "DELETE"),
      )
      .toBe(true);
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

  it("renders overview roster and safe direct roles", async () => {
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={createTeamsFetcher()}
        teamId="team-1"
      />,
    );

    await expect.element(screen.getByText("负责人甲", { exact: true })).toBeVisible();
    for (const label of [
      "人类",
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
      .element(screen.getByRole("combobox", { name: "直接生效角色" }))
      .toBeVisible();
  });

  it("uses user search for direct member add", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await expect
      .element(screen.getByRole("searchbox", { name: "搜索直接添加用户" }))
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
  });

  it("renders digital employees in the overview tab list", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await expect
      .element(screen.getByRole("link", { name: "新建数字员工" }))
      .toBeVisible();

    await expect.element(screen.getByText("数据库运维员工")).toBeVisible();

    await expect
      .element(screen.getByRole("link", { name: "新建数字员工" }))
      .toHaveAttribute("href", "/employees/new");
    await expect
      .element(screen.getByRole("link", { name: "新建数字员工" }))
      .toHaveAttribute("data-router-link", "true");
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees?team_id=team-1",
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      }),
    );
  });
});
