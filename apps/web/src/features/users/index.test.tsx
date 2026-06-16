import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { Users } from "@/features/users";
import { CreateUserDrawer } from "@/features/users/components/create-user-drawer";

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

const TEAM_OPS_ID = "11111111-1111-4111-8111-111111111111";
const TEAM_RISK_ID = "22222222-2222-4222-8222-222222222222";
const TEAM_DISABLED_ID = "33333333-3333-4333-8333-333333333333";

function createDeferredResponse() {
  let resolve!: (value: Response) => void;
  const promise = new Promise<Response>((nextResolve) => {
    resolve = nextResolve;
  });

  return { promise, resolve };
}

type CreateUsersFetcherOptions = {
  avatarAssetsStatus?: "empty" | "error" | "ok";
  projectTeamScopesDeferred?: ReturnType<typeof createDeferredResponse>;
  projectTeamScopesStatus?: "empty" | "error" | "loading" | "ok";
  teamsDeferred?: ReturnType<typeof createDeferredResponse>;
  teamsStatus?: "empty" | "error" | "loading" | "ok";
};

function createUsersFetcher({
  avatarAssetsStatus = "ok",
  projectTeamScopesDeferred,
  projectTeamScopesStatus = "ok",
  teamsDeferred,
  teamsStatus = "ok",
}: CreateUsersFetcherOptions = {}) {
  let createdUser:
    | {
        avatar: {
          provider: "dicebear";
          seed: string;
          style: "adventurer";
        };
        avatar_asset_id: string;
        display_name: string;
        email: null;
        id: string;
        status: "active";
        username: string;
      }
    | undefined;

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
      const visibleUsers = createdUser ? [createdUser, ...users] : users;

      return jsonResponse({
        items: status ? visibleUsers.filter((user) => user.status === status) : visibleUsers,
      });
    }

    if (url.pathname === "/api/auth/users" && method === "POST") {
      createdUser = {
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
      };

      return jsonResponse({
        user: createdUser,
      });
    }

    if (url.pathname === "/api/auth/users/user-1/project-team-scopes" && method === "GET") {
      if (projectTeamScopesStatus === "loading" && projectTeamScopesDeferred) {
        return projectTeamScopesDeferred.promise;
      }

      if (projectTeamScopesStatus === "error") {
        return new Response(JSON.stringify({ error: "scope load failed" }), {
          headers: { "content-type": "application/json" },
          status: 500,
        });
      }

      if (projectTeamScopesStatus === "empty") {
        return jsonResponse({ items: [] });
      }

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
              id: TEAM_OPS_ID,
              name: "平台运营",
              pending_draft_count: 0,
              risk_summary: "低风险",
              slug: "ops",
              status: "active",
            },
            team_id: TEAM_OPS_ID,
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
              id: TEAM_RISK_ID,
              name: "风控审查",
              pending_draft_count: 1,
              risk_summary: "需审批",
              slug: "risk",
              status: "active",
            },
            team_id: TEAM_RISK_ID,
            tenant_id: "tenant-1",
            updated_at: "2026-06-04T02:28:13Z",
            user_id: "user-1",
          },
        ],
      });
    }

    if (url.pathname === "/api/auth/users/user-3/project-team-scopes" && method === "GET") {
      return jsonResponse({ items: [] });
    }

    if (url.pathname === "/api/v1/digital-employee-avatar-assets" && method === "GET") {
      if (avatarAssetsStatus === "error") {
        return new Response(JSON.stringify({ error: "avatar assets failed" }), {
          headers: { "content-type": "application/json" },
          status: 500,
        });
      }

      if (avatarAssetsStatus === "empty") {
        return jsonResponse([]);
      }

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
      if (teamsStatus === "loading" && teamsDeferred) {
        return teamsDeferred.promise;
      }

      if (teamsStatus === "error") {
        return new Response(JSON.stringify({ error: "teams failed" }), {
          headers: { "content-type": "application/json" },
          status: 500,
        });
      }

      if (teamsStatus === "empty") {
        return jsonResponse([]);
      }

      return jsonResponse([
        {
          capability_count: 2,
          current_revision: 3,
          digital_employee_count: 4,
          governance_status: "active",
          human_owner_user_ids: ["user-1"],
          id: TEAM_OPS_ID,
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
          id: TEAM_RISK_ID,
          member_count: 5,
          name: "风控审查",
          pending_draft_count: 1,
          risk_summary: "需审批",
          slug: "risk",
          status: "active",
          tenant_id: "tenant-1",
        },
        {
          capability_count: 0,
          current_revision: 1,
          digital_employee_count: 0,
          governance_status: "active",
          human_owner_user_ids: ["user-1"],
          id: TEAM_DISABLED_ID,
          member_count: 1,
          name: "停用团队",
          pending_draft_count: 0,
          risk_summary: "不可选",
          slug: "disabled",
          status: "disabled",
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
                team_id: TEAM_OPS_ID,
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
            resource_id: TEAM_OPS_ID,
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
    await expect.element(screen.getByText("停用团队")).not.toBeInTheDocument();

    await expect.element(screen.getByText("头像种子")).not.toBeInTheDocument();
    await expect.element(screen.getByText("发送邀请链接")).not.toBeInTheDocument();
    await expect.element(screen.getByText("MFA")).not.toBeInTheDocument();
    await expect.element(screen.getByText("团队归属")).not.toBeInTheDocument();
    await expect.element(screen.getByText("可调用团队员工池")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();

    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/digital-employee-avatar-assets"),
      expect.any(Object),
    );
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/v1/teams?status=active"), expect.any(Object));
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
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
    await userEvent.fill(screen.getByLabelText("用户名"), "new-operator");
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
    await userEvent.fill(screen.getByLabelText("名称"), "新管理员");
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
    await userEvent.fill(screen.getByLabelText("密码"), "secret-pass");
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "选择头像 工程师头像 F03" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
    await userEvent.click(screen.getByRole("checkbox", { name: "平台运营" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).not.toBeDisabled();
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
      selectable_team_ids: [TEAM_OPS_ID, TEAM_RISK_ID],
      username: "new-operator",
    });
    await expect.element(screen.getByLabelText("用户名")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "新管理员" })).toBeInTheDocument();
    expect(
      fetcher.mock.calls.filter(([input, init]) => {
        const url = new URL(String(input));
        return url.pathname === "/api/auth/users" && (init?.method ?? "GET") === "GET";
      }).length,
    ).toBeGreaterThan(1);

    await userEvent.click(screen.getByRole("button", { name: "新建用户" }));
    await expect.element(screen.getByLabelText("用户名")).toHaveValue("");
    await expect.element(screen.getByLabelText("名称")).toHaveValue("");
    await expect.element(screen.getByLabelText("密码")).toHaveValue("");
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
  });

  it("resets filters and selects a newly created active user from a disabled-only view", async () => {
    const fetcher = createUsersFetcher();
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { exact: true, name: "禁用" }));
    await expect.element(screen.getByRole("heading", { name: "审计员" })).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { exact: true, name: "禁用" })).toHaveAttribute("aria-pressed", "true");

    await userEvent.click(screen.getByRole("button", { name: "新建用户" }));
    await userEvent.fill(screen.getByLabelText("用户名"), "new-operator");
    await userEvent.fill(screen.getByLabelText("名称"), "新管理员");
    await userEvent.fill(screen.getByLabelText("密码"), "secret-pass");
    await userEvent.click(screen.getByRole("button", { name: "选择头像 工程师头像 F03" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "平台运营" }));
    await userEvent.click(screen.getByRole("button", { name: "创建用户" }));

    await expect.element(screen.getByRole("heading", { name: "新管理员" })).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { exact: true, name: "全部" })).toHaveAttribute("aria-pressed", "true");

    const postCallIndex = fetcher.mock.calls.findIndex(([input, init]) => {
      const url = new URL(String(input));
      return url.pathname === "/api/auth/users" && init?.method === "POST";
    });
    expect(postCallIndex).toBeGreaterThan(-1);
    expect(
      fetcher.mock.calls.slice(postCallIndex + 1).some(([input, init]) => {
        const url = new URL(String(input));
        return (
          url.pathname === "/api/auth/users" &&
          (init?.method ?? "GET") === "GET" &&
          !url.searchParams.has("status")
        );
      }),
    ).toBe(true);
  });

  it("keeps user creation disabled when avatar assets fail", async () => {
    const errorFetcher = createUsersFetcher({ avatarAssetsStatus: "error" });
    vi.stubGlobal("fetch", errorFetcher);

    const errorScreen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(errorScreen.getByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    await userEvent.click(errorScreen.getByRole("button", { name: "新建用户" }));
    await expect.element(errorScreen.getByText("头像加载失败")).toBeInTheDocument();
    await expect.element(errorScreen.getByText("工程师头像 F03")).not.toBeInTheDocument();
    await userEvent.fill(errorScreen.getByLabelText("用户名"), "new-operator");
    await userEvent.fill(errorScreen.getByLabelText("名称"), "新管理员");
    await userEvent.fill(errorScreen.getByLabelText("密码"), "secret-pass");
    await userEvent.click(errorScreen.getByRole("checkbox", { name: "平台运营" }));
    await expect.element(errorScreen.getByRole("button", { name: "创建用户" })).toBeDisabled();
  });

  it("keeps user creation disabled when avatar assets are empty", async () => {
    const emptyFetcher = createUsersFetcher({ avatarAssetsStatus: "empty" });
    vi.stubGlobal("fetch", emptyFetcher);

    const emptyScreen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(emptyScreen.getByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    await userEvent.click(emptyScreen.getByRole("button", { name: "新建用户" }));
    await expect.element(emptyScreen.getByText("暂无可选头像")).toBeInTheDocument();
    await expect.element(emptyScreen.getByText("工程师头像 F03")).not.toBeInTheDocument();
    await expect.element(emptyScreen.getByRole("button", { name: "创建用户" })).toBeDisabled();
  });

  it("keeps user creation disabled while selectable teams are loading", async () => {
    const deferred = createDeferredResponse();
    const fetcher = createUsersFetcher({
      teamsDeferred: deferred,
      teamsStatus: "loading",
    });
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "新建用户" }));
    await expect.element(screen.getByText("加载可选团队中")).toBeInTheDocument();
    await userEvent.fill(screen.getByLabelText("用户名"), "new-operator");
    await userEvent.fill(screen.getByLabelText("名称"), "新管理员");
    await userEvent.fill(screen.getByLabelText("密码"), "secret-pass");
    await userEvent.click(screen.getByRole("button", { name: "选择头像 工程师头像 F03" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
    deferred.resolve(jsonResponse([]));
  });

  it("keeps user creation disabled when selectable teams fail", async () => {
    const fetcher = createUsersFetcher({ teamsStatus: "error" });
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "新建用户" }));
    await expect.element(screen.getByText("可选团队加载失败")).toBeInTheDocument();
    await userEvent.fill(screen.getByLabelText("用户名"), "new-operator");
    await userEvent.fill(screen.getByLabelText("名称"), "新管理员");
    await userEvent.fill(screen.getByLabelText("密码"), "secret-pass");
    await userEvent.click(screen.getByRole("button", { name: "选择头像 工程师头像 F03" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
  });

  it("keeps user creation disabled when selectable teams are empty", async () => {
    const fetcher = createUsersFetcher({ teamsStatus: "empty" });
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "新建用户" }));
    await expect.element(screen.getByText("暂无可选团队。")).toBeInTheDocument();
    await userEvent.fill(screen.getByLabelText("用户名"), "new-operator");
    await userEvent.fill(screen.getByLabelText("名称"), "新管理员");
    await userEvent.fill(screen.getByLabelText("密码"), "secret-pass");
    await userEvent.click(screen.getByRole("button", { name: "选择头像 工程师头像 F03" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
  });

  it("blocks retained avatar and team choices while create drawer queries refetch", async () => {
    const avatarDeferred = createDeferredResponse();
    const teamsDeferred = createDeferredResponse();
    let mode: "loading" | "ready" = "ready";
    const submit = vi.fn();
    const fetcher = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname === "/api/v1/digital-employee-avatar-assets") {
        if (mode === "loading") {
          return avatarDeferred.promise;
        }

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
        ]);
      }

      if (url.pathname === "/api/v1/teams") {
        if (mode === "loading") {
          return teamsDeferred.promise;
        }

        return jsonResponse([
          {
            capability_count: 2,
            current_revision: 3,
            digital_employee_count: 4,
            governance_status: "active",
            human_owner_user_ids: ["user-1"],
            id: TEAM_OPS_ID,
            member_count: 8,
            name: "平台运营",
            pending_draft_count: 0,
            risk_summary: "低风险",
            slug: "ops",
            status: "active",
            tenant_id: "tenant-1",
          },
        ]);
      }

      return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
        headers: { "content-type": "application/json" },
        status: 404,
      });
    });
    const queryClient = createQueryClient();
    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <CreateUserDrawer
          apiBaseUrl="http://127.0.0.1:8081"
          fetcher={fetcher}
          onOpenChange={() => undefined}
          onSubmit={submit}
          open
        />
      </QueryClientProvider>,
    );

    await userEvent.fill(screen.getByLabelText("用户名"), "new-operator");
    await userEvent.fill(screen.getByLabelText("名称"), "新管理员");
    await userEvent.fill(screen.getByLabelText("密码"), "secret-pass");
    await userEvent.click(screen.getByRole("button", { name: "选择头像 工程师头像 F03" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "平台运营" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).not.toBeDisabled();

    mode = "loading";
    void queryClient.invalidateQueries({ queryKey: ["users", "create"] });

    await expect.element(screen.getByText("加载头像中")).toBeInTheDocument();
    await expect.element(screen.getByText("加载可选团队中")).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "选择头像 工程师头像 F03" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("checkbox", { name: "平台运营" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
    expect(submit).not.toHaveBeenCalled();
    expect(
      fetcher.mock.calls.some(([input, init]) => {
        const url = new URL(String(input));
        return url.pathname === "/api/auth/users" && init?.method === "POST";
      }),
    ).toBe(false);

    avatarDeferred.resolve(jsonResponse([]));
    teamsDeferred.resolve(jsonResponse([]));
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

  it("renders selected user selectable team scope loading state", async () => {
    const deferred = createDeferredResponse();
    const fetcher = createUsersFetcher({
      projectTeamScopesDeferred: deferred,
      projectTeamScopesStatus: "loading",
    });
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "平台管理员" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("tab", { name: "可选团队" }));
    await expect.element(screen.getByText("加载可选团队中")).toBeInTheDocument();
    deferred.resolve(jsonResponse({ items: [] }));
  });

  it("renders selected user selectable team scope error state", async () => {
    const fetcher = createUsersFetcher({ projectTeamScopesStatus: "error" });
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "平台管理员" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("tab", { name: "可选团队" }));
    await expect.element(screen.getByText("可选团队加载失败")).toBeInTheDocument();
    await expect.element(screen.getByText("请检查用户团队范围接口。")).toBeInTheDocument();
  });

  it("renders selected user selectable team scope empty state", async () => {
    const fetcher = createUsersFetcher({ projectTeamScopesStatus: "empty" });
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "平台管理员" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("tab", { name: "可选团队" }));
    await expect.element(screen.getByText("暂无可选团队。")).toBeInTheDocument();
  });
});
