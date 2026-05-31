import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";

import { UsersPage } from "./users-page";

const logout = vi.fn();
let pathname = "/users";

vi.mock("@superteam/core/auth", () => ({
  useAuth: () => ({
    logout,
    user: {
      id: 1,
      status: "active",
      username: "admin",
    },
  }),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
}));

describe("UsersPage", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    logout.mockReset();
    pathname = "/users";
  });

  it("loads users and supports user management actions", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            items: [
              {
                id: 1,
                username: "admin",
                status: "active",
              },
              {
                id: 2,
                username: "operator",
                status: "disabled",
              },
            ],
          }),
          { headers: { "content-type": "application/json" }, status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ user: { id: 3, username: "analyst", status: "active" } }), {
          headers: { "content-type": "application/json" },
          status: 201,
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            items: [
              { id: 1, username: "admin", status: "active" },
              { id: 2, username: "operator", status: "disabled" },
              { id: 3, username: "analyst", status: "active" },
            ],
          }),
          { headers: { "content-type": "application/json" }, status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ user: { id: 2, username: "operator", status: "active" } }), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ user: { id: 1, username: "admin", status: "active" } }), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
      );

    render(
      <TooltipProvider>
        <UsersPage apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
      </TooltipProvider>,
    );

    expect(await screen.findByRole("heading", { name: "用户管理" })).toBeInTheDocument();
    expect(await screen.findByText("operator")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("新用户账号"), { target: { value: "analyst" } });
    fireEvent.change(screen.getByLabelText("初始密码"), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "创建用户" }));

    await waitFor(() => {
      expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/users", expect.objectContaining({ method: "POST" }));
    });

    fireEvent.click(await screen.findByRole("button", { name: "启用 operator" }));
    fireEvent.change(screen.getByLabelText("admin 的新密码"), { target: { value: "new-secret" } });
    fireEvent.click(screen.getByRole("button", { name: "重置 admin 密码" }));

    await waitFor(() => {
      expect(fetcher).toHaveBeenCalledWith(
        "http://control-plane.local/api/auth/users/2/status",
        expect.objectContaining({ method: "PATCH" }),
      );
      expect(fetcher).toHaveBeenCalledWith(
        "http://control-plane.local/api/auth/users/1/reset-password",
        expect.objectContaining({ method: "POST" }),
      );
    });
  });

  it("renders a shared loading and empty state for the user list", async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(
      new Response(JSON.stringify({ items: [] }), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    render(
      <TooltipProvider>
        <UsersPage apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
      </TooltipProvider>,
    );

    expect(screen.getByRole("status")).toHaveTextContent("正在加载用户列表");
    expect(await screen.findByRole("heading", { name: "暂无用户" })).toBeInTheDocument();
    expect(screen.getByText("当前租户还没有可管理的平台账号。")).toBeInTheDocument();
  });

  it("renders a shared error state when the initial user list load fails", async () => {
    const fetcher = vi.fn().mockRejectedValueOnce(new Error("network failed"));

    render(
      <TooltipProvider>
        <UsersPage apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
      </TooltipProvider>,
    );

    expect(await screen.findByRole("alert")).toHaveTextContent("用户列表加载失败");
    expect(screen.getByRole("button", { name: "重新加载" })).toBeInTheDocument();
  });
});
