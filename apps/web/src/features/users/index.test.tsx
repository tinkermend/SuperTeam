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
const USER_OPERATOR_ID = "44444444-4444-4444-8444-444444444444";
const USER_AUDITOR_ID = "55555555-5555-4555-8555-555555555555";
const USER_CREATED_ID = "66666666-6666-4666-8666-666666666666";
const USER_ADMIN_ID = "77777777-7777-4777-8777-777777777777";
const TENANT_ID = "88888888-8888-4888-8888-888888888888";
const SCOPE_OPS_ID = "99999999-9999-4999-8999-999999999999";
const SCOPE_RISK_ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const LOGIN_EVENT_ID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
const SESSION_ID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
const AUTHZ_DECISION_ID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd";
const NEW_USER_AVATAR = {
  provider: "dicebear" as const,
  seed: "user:zhoumin",
  style: "adventurer" as const,
};

function createDeferredResponse() {
  let resolve!: (value: Response) => void;
  const promise = new Promise<Response>((nextResolve) => {
    resolve = nextResolve;
  });

  return { promise, resolve };
}

type CreateUsersFetcherOptions = {
  projectTeamScopesDeferred?: ReturnType<typeof createDeferredResponse>;
  projectTeamScopesStatus?: "empty" | "error" | "loading" | "ok";
  teamsDeferred?: ReturnType<typeof createDeferredResponse>;
  teamsStatus?: "empty" | "error" | "loading" | "ok";
};

function createUsersFetcher({
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
        avatar_asset_id: null;
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
          id: USER_OPERATOR_ID,
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
          id: USER_AUDITOR_ID,
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
      const body = init?.body ? JSON.parse(String(init.body)) : {};
      createdUser = {
        avatar: body.avatar ?? NEW_USER_AVATAR,
        avatar_asset_id: null,
        display_name: "新管理员",
        email: null,
        id: USER_CREATED_ID,
        status: "active",
        username: "new-operator",
      };

      return jsonResponse({
        user: createdUser,
      });
    }

    if (url.pathname === `/api/auth/users/${USER_OPERATOR_ID}/project-team-scopes` && method === "GET") {
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
            granted_by_user_id: USER_ADMIN_ID,
            id: SCOPE_OPS_ID,
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
            tenant_id: TENANT_ID,
            updated_at: "2026-06-04T02:28:13Z",
            user_id: USER_OPERATOR_ID,
          },
          {
            created_at: "2026-06-04T02:28:13Z",
            granted_by_user_id: USER_ADMIN_ID,
            id: SCOPE_RISK_ID,
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
            tenant_id: TENANT_ID,
            updated_at: "2026-06-04T02:28:13Z",
            user_id: USER_OPERATOR_ID,
          },
        ],
      });
    }

    if (url.pathname === `/api/auth/users/${USER_CREATED_ID}/project-team-scopes` && method === "GET") {
      return jsonResponse({ items: [] });
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
          human_owner_user_ids: [USER_OPERATOR_ID],
          id: TEAM_OPS_ID,
          member_count: 8,
          name: "平台运营",
          pending_draft_count: 0,
          risk_summary: "低风险",
          slug: "ops",
          status: "active",
          tenant_id: TENANT_ID,
        },
        {
          capability_count: 1,
          current_revision: 1,
          digital_employee_count: 2,
          governance_status: "draft_pending",
          human_owner_user_ids: [USER_OPERATOR_ID],
          id: TEAM_RISK_ID,
          member_count: 5,
          name: "风控审查",
          pending_draft_count: 1,
          risk_summary: "需审批",
          slug: "risk",
          status: "active",
          tenant_id: TENANT_ID,
        },
        {
          capability_count: 0,
          current_revision: 1,
          digital_employee_count: 0,
          governance_status: "active",
          human_owner_user_ids: [USER_OPERATOR_ID],
          id: TEAM_DISABLED_ID,
          member_count: 1,
          name: "停用团队",
          pending_draft_count: 0,
          risk_summary: "不可选",
          slug: "disabled",
          status: "disabled",
          tenant_id: TENANT_ID,
        },
      ]);
    }

    if (url.pathname === "/api/authz/members" && method === "GET") {
      return jsonResponse({
        items: [
          {
            user_id: USER_OPERATOR_ID,
            username: "operator",
            display_name: "平台管理员",
            email: "operator@example.com",
            account_status: "active",
            console_access: true,
            recent_denied_reason: "team.member.change_role requires privileged role approval",
            memberships: [
              {
                tenant_id: TENANT_ID,
                team_id: null,
                principal_type: "user",
                principal_id: USER_OPERATOR_ID,
                role: "owner",
                status: "active",
              },
              {
                tenant_id: TENANT_ID,
                team_id: TEAM_OPS_ID,
                principal_type: "user",
                principal_id: USER_OPERATOR_ID,
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
            id: LOGIN_EVENT_ID,
            event_type: "login_succeeded",
            user_id: USER_OPERATOR_ID,
            username: "operator",
            session_id: SESSION_ID,
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
            id: AUTHZ_DECISION_ID,
            tenant_id: TENANT_ID,
            user_id: USER_OPERATOR_ID,
            username: "operator",
            module: "team",
            action: "team.member.change_role",
            result: "failed",
            resource_type: "team",
            resource_id: TEAM_OPS_ID,
            reason: "requires privileged role approval",
            actor_type: "user",
            actor_id: USER_OPERATOR_ID,
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
    await expect.element(screen.getByRole("tab", { name: "可选团队" })).toBeInTheDocument();
    await expect.element(screen.getByRole("tab", { name: "团队与角色" })).not.toBeInTheDocument();
    await expect.element(screen.getByText("成员身份记录")).toBeInTheDocument();
    await expect.element(screen.getByText("团队与角色")).not.toBeInTheDocument();
    await expect.element(screen.getByText("所属团队 & 角色")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "平台管理员" })).toBeInTheDocument();
    await expect.element(screen.getByText("用户 360")).toBeInTheDocument();
    await expect.element(screen.getByText("operator@example.com").first()).toBeInTheDocument();
    await expect.element(screen.getByText("team.member.change_role", { exact: true }).first()).toBeInTheDocument();
    await expect.element(screen.getByText("Chrome 125 / macOS").first()).toBeInTheDocument();
    expect(document.querySelectorAll('[data-slot="v3-soft-card"]').length).toBeGreaterThan(0);
    expect(document.querySelectorAll('[data-slot="v3-icon-tile"]').length).toBeGreaterThan(0);
    expect(document.querySelectorAll('[data-slot="v3-status-pill"]').length).toBeGreaterThan(0);
    expect(document.querySelectorAll('[data-slot="v3-work-surface"]').length).toBeGreaterThan(0);
    expect(document.querySelectorAll('[data-slot="v3-table"]').length).toBeGreaterThan(0);
    await expect.element(screen.getByRole("link", { name: "去团队管理分配" })).toHaveAttribute("data-router-link", "true");
    await expect.element(screen.getByRole("link", { name: "查看权限中心" })).toHaveAttribute("data-router-link", "true");

    const avatar = screen.getByAltText("平台管理员 的头像").first();
    await expect.element(avatar).toBeInTheDocument();
    await expect.element(avatar).toHaveAttribute("src", "/images/digital-employee-avatars/engineer-f-03-256.webp");
    await expect.element(avatar).not.toHaveAttribute("src", expect.stringContaining("data:image/svg+xml"));
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/auth/users?limit=50&offset=0"), expect.any(Object));
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/authz/members?limit=100&offset=0"), expect.any(Object));
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining(`/api/authz/decisions?result=failed&actor_type=user&actor_id=${USER_OPERATOR_ID}&limit=8&offset=0`),
      expect.any(Object),
    );
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining(`/api/auth/users/${USER_OPERATOR_ID}/project-team-scopes`),
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
    await expect.element(screen.getByRole("button", { name: "选择头像 人类头像 03" })).toBeInTheDocument();
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

    expect(
      fetcher.mock.calls.some(([input]) => new URL(String(input)).pathname === "/api/v1/digital-employee-avatar-assets"),
    ).toBe(false);
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/v1/teams?status=active"), expect.any(Object));
  });

  it("creates a human user with human avatar config and multiple selectable teams", async () => {
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
    await userEvent.click(screen.getByRole("button", { name: "选择头像 人类头像 03" }));
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
      avatar: NEW_USER_AVATAR,
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
    await userEvent.click(screen.getByRole("button", { name: "选择头像 人类头像 03" }));
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

  it("keeps user creation disabled until a human avatar is selected", async () => {
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
    await userEvent.click(screen.getByRole("checkbox", { name: "平台运营" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: "选择头像 人类头像 03" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).not.toBeDisabled();
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
    await userEvent.click(screen.getByRole("button", { name: "选择头像 人类头像 03" }));
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
    await userEvent.click(screen.getByRole("button", { name: "选择头像 人类头像 03" }));
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
    await userEvent.click(screen.getByRole("button", { name: "选择头像 人类头像 03" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
  });

  it("blocks retained team choices while create drawer team query refetches", async () => {
    const teamsDeferred = createDeferredResponse();
    let mode: "loading" | "ready" = "ready";
    const submit = vi.fn();
    const fetcher = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

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
            human_owner_user_ids: [USER_OPERATOR_ID],
            id: TEAM_OPS_ID,
            member_count: 8,
            name: "平台运营",
            pending_draft_count: 0,
            risk_summary: "低风险",
            slug: "ops",
            status: "active",
            tenant_id: TENANT_ID,
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
    await userEvent.click(screen.getByRole("button", { name: "选择头像 人类头像 03" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "平台运营" }));
    await expect.element(screen.getByRole("button", { name: "创建用户" })).not.toBeDisabled();

    mode = "loading";
    void queryClient.invalidateQueries({ queryKey: ["users", "create"] });

    await expect.element(screen.getByText("加载可选团队中")).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "选择头像 人类头像 03" })).toBeInTheDocument();
    await expect.element(screen.getByRole("checkbox", { name: "平台运营" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "创建用户" })).toBeDisabled();
    expect(submit).not.toHaveBeenCalled();
    expect(
      fetcher.mock.calls.some(([input, init]) => {
        const url = new URL(String(input));
        return url.pathname === "/api/auth/users" && init?.method === "POST";
      }),
    ).toBe(false);

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
      expect.stringContaining(`/api/auth/users/${USER_OPERATOR_ID}/project-team-scopes`),
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
