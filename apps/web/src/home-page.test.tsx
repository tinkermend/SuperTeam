import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";

import { HomePage } from "./home-page";

const logout = vi.fn();
let pathname = "/";
let authState = {
  logout,
  user: {
    id: 1,
    status: "active" as const,
    username: "admin",
  },
};

vi.mock("@superteam/core/auth", () => ({
  useAuth: () => authState,
}));

vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
}));

describe("HomePage", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    logout.mockReset();
    pathname = "/";
    authState = {
      logout,
      user: {
        id: 1,
        status: "active",
        username: "admin",
      },
    };
  });

  it("renders the external system shell with navigation, search, and a placeholder workspace", () => {
    render(
      <TooltipProvider>
        <HomePage summary="control-plane is ok" />
      </TooltipProvider>,
    );

    expect(screen.getByRole("heading", { name: "首页" })).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "主导航" })).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: "全局搜索" })).toHaveAttribute(
      "placeholder",
      "搜索任务、工件、员工、流程...",
    );
    expect(screen.getByRole("heading", { name: "页面正在开发中" })).toBeInTheDocument();
    expect(screen.getByText("control-plane is ok")).toBeInTheDocument();
  });

  it("renders the authenticated user and supports logout", () => {
    render(
      <TooltipProvider>
        <HomePage summary="control-plane is ok" />
      </TooltipProvider>,
    );

    expect(screen.getAllByText("admin").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "admin 平台成员" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "退出登录" }));

    expect(logout).toHaveBeenCalledTimes(1);
  });
});
