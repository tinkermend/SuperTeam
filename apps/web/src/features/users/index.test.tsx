import type { ReactNode } from "react";
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

    const avatar = screen.getByAltText("平台管理员 的头像");
    await expect.element(avatar).toBeInTheDocument();
    await expect.element(avatar).toHaveAttribute("src", expect.stringContaining("data:image/svg+xml"));
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/auth/users?limit=50&offset=0"), expect.any(Object));
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/authz/members?limit=100&offset=0"), expect.any(Object));
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining("/api/authz/decisions?result=failed&actor_type=user&actor_id=user-1&limit=8&offset=0"),
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

    await expect.element(screen.getByRole("heading", { name: "auditor" })).toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/auth/users?status=disabled&limit=50&offset=0"), expect.any(Object));
  });
});
