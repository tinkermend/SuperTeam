import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { Users } from "@/features/users";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>,
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children, fluid }: { children: ReactNode; fluid?: boolean }) => (
    <main data-fluid={fluid ? "true" : "false"}>{children}</main>
  ),
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
    to: string;
  };
  const Link = forwardRef<HTMLAnchorElement, MockLinkProps>(
    ({ children, to, ...props }, ref) => (
      <a {...props} data-router-link="true" href={to} ref={ref}>
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

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status: 200,
  });
}

function createUsersFetcher() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/auth/users" && method === "GET") {
      const status = url.searchParams.get("status");
      const users = [
        {
          avatar_asset_id: "engineer-f-03",
          display_name: "平台管理员",
          email: "operator@example.com",
          id: "user-1",
          username: "operator",
          status: "active",
          avatar: {
            provider: "dicebear",
            style: "adventurer",
            seed: "operator-avatar",
          },
        },
        {
          avatar_asset_id: "engineer-m-02",
          display_name: "审计员",
          email: "auditor@example.com",
          id: "user-2",
          username: "auditor",
          status: "disabled",
          avatar: {
            provider: "dicebear",
            style: "adventurer",
            seed: "auditor-avatar",
          },
        },
      ];

      return jsonResponse({
        items: status ? users.filter((user) => user.status === status) : users,
      });
    }

    if (url.pathname === "/api/auth/users" && method === "POST") {
      return jsonResponse({
        user: {
          avatar: {
            provider: "dicebear",
            style: "adventurer",
            seed: "new-operator",
          },
          avatar_asset_id: "engineer-f-03",
          display_name: "新管理员",
          email: null,
          id: "user-3",
          status: "active",
          username: "new-operator",
        },
      });
    }

    if (url.pathname === "/api/auth/users/user-1/project-team-scopes" && method === "GET") {
      return jsonResponse({
        items: [
          {
            created_at: "2026-06-04T02:28:13Z",
            granted_by_user_id: "admin-1",
            id: "scope-1",
            revoked_at: null,
            status: "active",
            team: {
              current_revision: 3,
              digital_employee_count: 4,
              governance_status: "active",
              human_owners: [],
              id: "team-ops",
              name: "平台运营",
              pending_draft_count: 0,
              risk_summary: "低风险",
              slug: "ops",
              status: "active",
            },
            team_id: "team-ops",
            tenant_id: "tenant-1",
            updated_at: "2026-06-04T02:28:13Z",
            user_id: "user-1",
          },
          {
            created_at: "2026-06-04T02:28:13Z",
            granted_by_user_id: "admin-1",
            id: "scope-2",
            revoked_at: null,
            status: "active",
            team: {
              current_revision: 1,
              digital_employee_count: 2,
              governance_status: "draft_pending",
              human_owners: [],
              id: "team-risk",
              name: "风控审查",
              pending_draft_count: 1,
              risk_summary: "需审批",
              slug: "risk",
              status: "active",
            },
            team_id: "team-risk",
            tenant_id: "tenant-1",
            updated_at: "2026-06-04T02:28:13Z",
            user_id: "user-1",
          },
        ],
      });
    }

    if (url.pathname === "/api/v1/digital-employee-avatar-assets" && method === "GET") {
      return jsonResponse([
        {
          age_range: "27",
          gender: "female",
          id: "engineer-f-03",
          image_url: "/images/digital-employee-avatars/engineer-f-03.webp",
          label: "工程师头像 F03",
          license: "internal_product_asset",
          source: "test",
          status: "active",
          style: "photorealistic_2d",
          thumbnail_url: "/images/digital-employee-avatars/engineer-f-03-256.webp",
        },
        {
          age_range: "31",
          gender: "male",
          id: "engineer-m-02",
          image_url: "/images/digital-employee-avatars/engineer-m-02.webp",
          label: "工程师头像 M02",
          license: "internal_product_asset",
          source: "test",
          status: "active",
          style: "photorealistic_2d",
          thumbnail_url: "/images/digital-employee-avatars/engineer-m-02-256.webp",
        },
      ]);
    }

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse([
        {
          capability_count: 2,
          current_revision: 3,
          digital_employee_count: 4,
          governance_status: "active",
          human_owner_user_ids: ["user-1"],
          id: "team-ops",
          member_count: 8,
          name: "平台运营",
          pending_draft_count: 0,
          risk_summary: "低风险",
          slug: "ops",
          status: "active",
          tenant_id: "tenant-1",
        },
        {
          capability_count: 1,
          current_revision: 1,
          digital_employee_count: 2,
          governance_status: "draft_pending",
          human_owner_user_ids: ["user-1"],
          id: "team-risk",
          member_count: 5,
          name: "风控审查",
          pending_draft_count: 1,
          risk_summary: "需审批",
          slug: "risk",
          status: "active",
          tenant_id: "tenant-1",
        },
      ]);
    }

    if (url.pathname === "/api/authz/members" && method === "GET") {
      return jsonResponse({
        items: [
          {
            user_id: "user-1",
            username: "operator",
            display_name: "平台管理员",
            email: "operator@example.com",
            account_status: "active",
            console_access: true,
            recent_denied_reason: "team.member.change_role requires privileged role approval",
            memberships: [
              {
                tenant_id: "tenant-1",
                team_id: null,
                principal_type: "user",
                principal_id: "user-1",
                role: "owner",
                status: "active",
              },
              {
                tenant_id: "tenant-1",
                team_id: "team-ops",
                principal_type: "user",
                principal_id: "user-1",
                role: "admin",
                status: "active",
              },
            ],
          },
        ],
      });
    }

    if (url.pathname === "/api/auth/login-logs" && method === "GET") {
      return jsonResponse({
        items: [
          {
            id: "login-1",
            event_type: "login_succeeded",
            user_id: "user-1",
            username: "operator",
            session_id: "session-1",
            client_ip: "127.0.0.1",
            user_agent: "Chrome 125 / macOS",
            result: "succeeded",
            created_at: "2026-06-04T02:28:13Z",
          },
        ],
      });
    }

    if (url.pathname === "/api/authz/decisions" && method === "GET") {
      return jsonResponse({
        items: [
          {
            id: "decision-1",
            tenant_id: "tenant-1",
            user_id: "user-1",
            username: "operator",
            module: "team",
            action: "team.member.change_role",
            result: "failed",
            resource_type: "team",
            resource_id: "team-ops",
            reason: "requires privileged role approval",
            actor_type: "user",
            actor_id: "user-1",
            created_at: "2026-06-04T01:44:00Z",
          },
        ],
      });
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: { "content-type": "application/json" },
      status: 404,
    });
  });
}

describe("Users", () => {
  it("renders a master detail user management workspace", async () => {
    const fetcher = createUsersFetcher();
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    await expect.element(screen.getByRole("main")).toHaveAttribute("data-fluid", "true");
    await expect.element(screen.getByTestId("users-management-layout")).toHaveAttribute("data-columns", "wide-list-balanced-detail");
    await expect.element(screen.getByTestId("users-overview-hero")).toHaveAttribute("data-layout", "equal-three-cards");
    await expect.element(screen.getByTestId("users-overview-basic-card")).toBeInTheDocument();
    await expect.element(screen.getByTestId("users-overview-permission-card")).toBeInTheDocument();
    await expect.element(screen.getByTestId("users-overview-timeline-card")).toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "平台管理员" })).toBeInTheDocument();
    await expect.element(screen.getByText("用户 360")).toBeInTheDocument();
    await expect.element(screen.getByText("operator@example.com").first()).toBeInTheDocument();
    await expect.element(screen.getByText("team.member.change_role", { exact: true }).first()).toBeInTheDocument();
    await expect.element(screen.getByText("Chrome 125 / macOS").first()).toBeInTheDocument();
    await expect.element(screen.getByRole("link", { name: "去团队管理分配" })).toHaveAttribute("data-router-link", "true");
    await expect.element(screen.getByRole("link", { name: "查看权限中心" })).toHaveAttribute("data-router-link", "true");

    const avatar = screen.getByAltText("平台管理员 的头像").first();
    await expect.element(avatar).toBeInTheDocument();
    await expect.element(avatar).toHaveAttribute("src", "/images/digital-employee-avatars/engineer-f-03-256.webp");
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/auth/users?limit=50&offset=0"), expect.any(Object));
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/authz/members?limit=100&offset=0"), expect.any(Object));
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining("/api/authz/decisions?result=failed&actor_type=user&actor_id=user-1&limit=8&offset=0"),
      expect.any(Object),
    );
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining("/api/auth/users/user-1/project-team-scopes"),
      expect.any(Object),
    );
  });

  it("filters users by disabled account status", async () => {
    const fetcher = createUsersFetcher();
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await userEvent.click(screen.getByRole("button", { exact: true, name: "禁用" }));

    await expect.element(screen.getByRole("heading", { name: "审计员" })).toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/auth/users?status=disabled&limit=50&offset=0"), expect.any(Object));
  });

  it("opens the human user creation drawer without legacy employee fields", async () => {
    const fetcher = createUsersFetcher();
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "新建用户" }));

    await expect.element(screen.getByLabelText("用户名")).toBeInTheDocument();
    await expect.element(screen.getByLabelText("名称")).toBeInTheDocument();
    await expect.element(screen.getByLabelText("密码")).toBeInTheDocument();
    await expect.element(screen.getByText("头像", { exact: true })).toBeInTheDocument();
    await expect.element(screen.getByText("选择可选团队", { exact: true })).toBeInTheDocument();
    await expect.element(screen.getByText("平台运营")).toBeInTheDocument();
    await expect.element(screen.getByText("风控审查")).toBeInTheDocument();

    await expect.element(screen.getByText("头像种子")).not.toBeInTheDocument();
    await expect.element(screen.getByText("发送邀请链接")).not.toBeInTheDocument();
    await expect.element(screen.getByText("MFA")).not.toBeInTheDocument();
    await expect.element(screen.getByText("团队归属")).not.toBeInTheDocument();
    await expect.element(screen.getByText("可调用团队员工池")).not.toBeInTheDocument();

    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/digital-employee-avatar-assets"),
      expect.any(Object),
    );
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/v1/teams"), expect.any(Object));
  });

  it("creates a human user with avatar asset and multiple selectable teams", async () => {
    const fetcher = createUsersFetcher();
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "新建用户" }));
    await userEvent.fill(screen.getByLabelText("用户名"), "new-operator");
    await userEvent.fill(screen.getByLabelText("名称"), "新管理员");
    await userEvent.fill(screen.getByLabelText("密码"), "secret-pass");
    await userEvent.click(screen.getByRole("button", { name: "选择头像 工程师头像 F03" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "平台运营" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "风控审查" }));
    await userEvent.click(screen.getByRole("button", { name: "创建用户" }));

    const postCall = fetcher.mock.calls.find(([input, init]) => {
      const url = new URL(String(input));
      return url.pathname === "/api/auth/users" && init?.method === "POST";
    });
    expect(postCall).toBeTruthy();
    expect(JSON.parse(String(postCall?.[1]?.body))).toEqual({
      avatar_asset_id: "engineer-f-03",
      display_name: "新管理员",
      password: "secret-pass",
      selectable_team_ids: ["team-ops", "team-risk"],
      username: "new-operator",
    });
  });

  it("renders selected user selectable team scopes", async () => {
    const fetcher = createUsersFetcher();
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "平台管理员" })).toBeInTheDocument();
    await expect.element(screen.getByRole("tab", { name: "可选团队" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("tab", { name: "可选团队" }));

    await expect.element(screen.getByText("当前用户创建或协作项目时可选择的团队范围。")).toBeInTheDocument();
    await expect.element(screen.getByText("平台运营")).toBeInTheDocument();
    await expect.element(screen.getByText("风控审查")).toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining("/api/auth/users/user-1/project-team-scopes"),
      expect.any(Object),
    );
  });
});
