import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi, type Mock } from "vitest";
import { render } from "vitest-browser-react";
import { AccountSettings } from "@/features/account";

const refreshCurrentUser = vi.fn();
const currentUser = {
  id: "user-1",
  username: "operator",
  display_name: "值班负责人",
  email: "operator@example.com",
  status: "active" as const,
  avatar: {
    provider: "dicebear" as const,
    style: "adventurer" as const,
    seed: "operator-avatar",
  },
};

vi.mock("@/features/auth/use-auth", () => ({
  useAuth: () => ({
    refreshCurrentUser,
    user: currentUser,
  }),
}));

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

function createAccountFetcher() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/auth/account/login-logs" && method === "GET") {
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
            created_at: "2026-06-12T08:00:00Z",
          },
        ],
      });
    }

    if (url.pathname === "/api/auth/account/profile" && method === "PATCH") {
      return jsonResponse({
        user: {
          id: "user-1",
          username: "operator",
          display_name: "新值班负责人",
          email: "new-operator@example.com",
          status: "active",
          avatar: {
            provider: "dicebear",
            style: "adventurer",
            seed: "operator-v2",
          },
        },
      });
    }

    if (url.pathname === "/api/auth/account/password" && method === "POST") {
      return jsonResponse({
        user: {
          id: "user-1",
          username: "operator",
          display_name: "值班负责人",
          email: "operator@example.com",
          status: "active",
          avatar: {
            provider: "dicebear",
            style: "adventurer",
            seed: "operator-avatar",
          },
        },
      });
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: { "content-type": "application/json" },
      status: 404,
    });
  });
}

function renderAccountSettings(fetcher: Mock) {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <AccountSettings fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("AccountSettings", () => {
  it("renders current user profile and recent login logs", async () => {
    const fetcher = createAccountFetcher();
    const screen = await renderAccountSettings(fetcher);

    await expect.element(screen.getByRole("heading", { name: "账户设置" })).toBeInTheDocument();
    await expect.element(screen.getByRole("main")).toHaveAttribute("data-fluid", "true");
    await expect.element(screen.getByText("值班负责人")).toBeInTheDocument();
    await expect.element(screen.getByText("operator@example.com")).toBeInTheDocument();
    await expect.element(screen.getByAltText("值班负责人 的头像")).toBeInTheDocument();
    await expect.element(screen.getByText("Chrome 125 / macOS")).toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining("/api/auth/account/login-logs?limit=10&offset=0"),
      expect.any(Object),
    );
  });

  it("saves profile changes and refreshes the current user", async () => {
    const fetcher = createAccountFetcher();
    const screen = await renderAccountSettings(fetcher);

    await userEvent.clear(screen.getByLabelText("展示名称"));
    await userEvent.fill(screen.getByLabelText("展示名称"), "新值班负责人");
    await userEvent.clear(screen.getByLabelText("邮箱"));
    await userEvent.fill(screen.getByLabelText("邮箱"), "new-operator@example.com");
    await userEvent.clear(screen.getByLabelText("头像 Seed"));
    await userEvent.fill(screen.getByLabelText("头像 Seed"), "operator-v2");
    await userEvent.click(screen.getByRole("button", { name: "保存资料" }));

    expect(
      fetcher.mock.calls.some(([input, init]) => {
        return (
          String(input).endsWith("/api/auth/account/profile") &&
          init?.method === "PATCH" &&
          init.body ===
            JSON.stringify({
              avatar: {
                provider: "dicebear",
                seed: "operator-v2",
                style: "adventurer",
              },
              display_name: "新值班负责人",
              email: "new-operator@example.com",
            })
        );
      }),
    ).toBe(true);
    expect(refreshCurrentUser).toHaveBeenCalledWith({ showLoading: false });
    await expect.element(screen.getByText("资料已保存")).toBeInTheDocument();
  });

  it("changes the current user password with the current password", async () => {
    const fetcher = createAccountFetcher();
    const screen = await renderAccountSettings(fetcher);

    await userEvent.fill(screen.getByLabelText("当前密码"), "old-secret");
    await userEvent.fill(screen.getByLabelText("新密码"), "new-secret");
    await userEvent.click(screen.getByRole("button", { name: "修改密码" }));

    expect(
      fetcher.mock.calls.some(([input, init]) => {
        return (
          String(input).endsWith("/api/auth/account/password") &&
          init?.method === "POST" &&
          init.body ===
            JSON.stringify({
              current_password: "old-secret",
              password: "new-secret",
            })
        );
      }),
    ).toBe(true);
    await expect.element(screen.getByText("密码已更新")).toBeInTheDocument();
  });
});
