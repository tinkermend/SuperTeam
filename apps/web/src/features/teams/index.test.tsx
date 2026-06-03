import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { TeamDetailView, TeamsView } from "@/features/teams";

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

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function createTeamsFetcher(options: { disabledOverview?: boolean } = {}) {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse([
        {
          id: "team-1",
          tenant_id: "tenant-1",
          slug: "ops",
          name: "运维团队",
          status: "active",
          human_owner_user_id: "human-owner-1",
          member_count: 18,
          digital_employee_count: 6,
          capability_count: 12,
          governance_status: "active",
          current_revision: 7,
          pending_draft_count: 3,
          risk_summary: "生产写操作需审批",
        },
      ]);
    }

    if (url.pathname === "/api/v1/teams/team-1/overview" && method === "GET") {
      return jsonResponse({
        team: {
          id: "team-1",
          tenant_id: "tenant-1",
          slug: "ops",
          name: "运维团队",
          status: options.disabledOverview ? "disabled" : "active",
          human_owner_user_id: "human-owner-1",
        },
        member_count: 18,
        digital_employee_count: 6,
        capability_count: 12,
        pending_draft_count: 3,
        pending_item_count: 3,
        allowed_actions: options.disabledOverview
          ? ["team.restore"]
          : ["team.update", "team.disable", "team.archive", "team.member.add", "team.governance.edit"],
      });
    }

    if (url.pathname === "/api/v1/teams/team-1/disable" && method === "POST") {
      return jsonResponse({ id: "team-1", tenant_id: "tenant-1", slug: "ops", name: "运维团队", status: "disabled" });
    }

    if (url.pathname === "/api/v1/teams/team-1/archive" && method === "POST") {
      return jsonResponse({ id: "team-1", tenant_id: "tenant-1", slug: "ops", name: "运维团队", status: "archived" });
    }

    if (url.pathname === "/api/v1/teams/team-1/restore" && method === "POST") {
      return jsonResponse({ id: "team-1", tenant_id: "tenant-1", slug: "ops", name: "运维团队", status: "active" });
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

    if (url.pathname === "/api/v1/digital-employees/employee-unbound/execution-instance" && method === "GET") {
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

    if (url.pathname.startsWith("/api/v1/digital-employees/") && url.pathname.endsWith("/execution-instance")) {
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
  }) as unknown as typeof fetch;

  return fetcher;
}

async function renderWithQueryClient(children: ReactNode) {
  return await render(<QueryClientProvider client={createQueryClient()}>{children}</QueryClientProvider>);
}

describe("TeamsView", () => {
  it("renders a dense team summary table", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(<TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />);

    await expect.element(screen.getByRole("heading", { name: "团队管理" })).toBeVisible();
    for (const column of ["负责人", "成员", "数字员工", "能力", "治理状态", "当前版本", "待批准"]) {
      await expect.element(screen.getByText(column)).toBeVisible();
    }
    await expect.element(screen.getByText("运维团队")).toBeVisible();
    await expect.element(screen.getByText("human-owner-1")).toBeVisible();
    await expect.element(screen.getByText("18")).toBeVisible();
    await expect.element(screen.getByText("v7")).toBeVisible();
    await expect.element(screen.getByText("3")).toBeVisible();
  });
});

describe("TeamDetailView", () => {
  it("renders detail tabs for the team shell", async () => {
    const screen = await renderWithQueryClient(
      <TeamDetailView apiBaseUrl="http://control-plane.local" fetcher={createTeamsFetcher()} teamId="team-1" />,
    );

    await expect.element(screen.getByRole("heading", { name: "运维团队" })).toBeVisible();
    for (const tab of ["概览", "成员", "数字员工", "能力与知识", "治理策略", "审计记录"]) {
      await expect.element(screen.getByRole("tab", { name: tab })).toBeVisible();
    }
    await screen.getByRole("tab", { name: "成员" }).click();
    await expect.element(screen.getByText("Plan 2 会接入成员与角色管理。")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "添加成员" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "创建治理草案" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "禁用团队" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "归档团队" })).toBeVisible();
  });

  it("calls lifecycle APIs from detail actions", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView apiBaseUrl="http://control-plane.local" fetcher={fetcher} teamId="team-1" />,
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

  it("does not show member or governance creation actions for a disabled team", async () => {
    const fetcher = createTeamsFetcher({ disabledOverview: true });
    const screen = await renderWithQueryClient(
      <TeamDetailView apiBaseUrl="http://control-plane.local" fetcher={fetcher} teamId="team-1" />,
    );

    await expect.element(screen.getByRole("heading", { name: "运维团队" })).toBeVisible();
    await expect.element(screen.getByText("已禁用")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "添加成员" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "创建治理草案" })).not.toBeInTheDocument();
    await screen.getByRole("button", { name: "恢复团队" }).click();
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/restore",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );
  });

  it("renders the team digital employees tab with metrics, table, and team-scoped quick create", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView apiBaseUrl="http://control-plane.local" fetcher={fetcher} teamId="team-1" />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "数字员工" }));

    await expect.element(screen.getByText("Plan 4 会接入团队数字员工列表和快速创建。")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("group", { name: "数字员工 11 总数" })).toBeVisible();
    await expect.element(screen.getByRole("group", { name: "active 10 正常运行" })).toBeVisible();
    await expect.element(screen.getByRole("group", { name: "draft 1 未发布" })).toBeVisible();
    await expect.element(screen.getByRole("group", { name: "继承配置过期 1 需更新" })).toBeVisible();
    await expect.element(screen.getByRole("group", { name: "未绑定 Runtime 1 当前页需绑定" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "从此团队创建数字员工" })).toBeVisible();

    for (const column of ["数字员工", "角色", "状态", "风险", "生效配置", "执行实例"]) {
      await expect.element(screen.getByRole("cell", { name: column })).toBeVisible();
    }
    await expect.element(screen.getByText("数据库运维员工")).toBeVisible();
    await expect.element(screen.getByText("v3（继承团队）")).toBeVisible();
    await expect.element(screen.getByText("ops-node-01")).toBeVisible();

    await expect.element(screen.getByLabelText("名称")).toBeVisible();
    await expect.element(screen.getByLabelText("角色")).toBeVisible();
    await expect.element(screen.getByLabelText("描述")).toBeVisible();
    await userEvent.fill(screen.getByLabelText("名称"), "日志分析员工");
    await userEvent.fill(screen.getByLabelText("角色"), "log_analyst");
    await userEvent.fill(screen.getByLabelText("描述"), "分析异常日志");
    await userEvent.click(screen.getByRole("button", { name: "从此团队创建数字员工" }));

    await expect.element(screen.getByText("已创建草稿：日志分析员工")).toBeVisible();
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees?team_id=team-1",
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      }),
    );
    expect(fetcher).not.toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee-hidden-unbound/execution-instance",
      expect.anything(),
    );
  });

  it("renders the team audit tab with summary, authorization action, and before after detail", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView apiBaseUrl="http://control-plane.local" fetcher={fetcher} teamId="team-1" />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "审计记录" }));

    await expect.element(screen.getByText("Plan 4 会接入团队审计记录。")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("group", { name: "今日操作 5 24 小时内" })).toBeVisible();
    await expect.element(screen.getByRole("group", { name: "成员变更 1 成员与角色" })).toBeVisible();
    await expect.element(screen.getByRole("group", { name: "治理版本 1 草案与审批" })).toBeVisible();
    await expect.element(screen.getByRole("group", { name: "能力绑定 1 外部能力" })).toBeVisible();
    await expect.element(screen.getByRole("group", { name: "被拒绝 1 需复核" })).toBeVisible();
    await expect.element(screen.getByRole("cell", { name: "授权动作" })).toBeVisible();
    await expect.element(screen.getByText("team.create")).toBeVisible();
    await expect.element(screen.getByText("创建团队“运维团队”")).toBeVisible();

    await userEvent.click(screen.getByRole("row", { name: /添加成员 孙悦/ }));

    await expect.element(screen.getByText("事件详情")).toBeVisible();
    await expect.element(screen.getByText("授权动作：team.member.add")).toBeVisible();
    await expect.element(screen.getByText("变更内容（前后对比）")).toBeVisible();
    await expect.element(screen.getByText('"role": "-"')).toBeVisible();
    await expect.element(screen.getByText('"role": "operator"')).toBeVisible();
  });
});
