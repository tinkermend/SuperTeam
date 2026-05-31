import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";

import { ConsoleAppShell } from "./console-app-shell";
import { createConsoleBreadcrumbItems, createConsoleNavItems } from "./console-nav";

const logout = vi.fn();
let pathname = "/users/42";

vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
}));

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

describe("ConsoleAppShell", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    logout.mockReset();
    pathname = "/users/42";
  });

  it("drives active navigation and breadcrumbs from the current route", () => {
    render(
      <TooltipProvider>
        <ConsoleAppShell pageTitle="用户详情">
          <main>用户详情内容</main>
        </ConsoleAppShell>
      </TooltipProvider>,
    );

    const mainNav = screen.getByRole("navigation", { name: "主导航" });
    expect(within(mainNav).getByRole("link", { name: "用户管理" })).toHaveAttribute("aria-current", "page");

    const breadcrumbs = screen.getByRole("navigation", { name: "面包屑" });
    expect(within(breadcrumbs).getByRole("link", { name: "首页" })).toHaveAttribute("href", "/");
    expect(within(breadcrumbs).getByText("用户管理")).toHaveAttribute("aria-current", "page");
  });

  it("keeps logout behavior in the shared shell wrapper", () => {
    render(
      <TooltipProvider>
        <ConsoleAppShell pageTitle="首页">
          <main>首页内容</main>
        </ConsoleAppShell>
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "admin 平台成员" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "退出登录" }));

    expect(logout).toHaveBeenCalledTimes(1);
  });
});

describe("console navigation helpers", () => {
  it("marks nested routes as active without marking the home item active", () => {
    const navItems = createConsoleNavItems("/users/42");

    expect(navItems.find((item) => item.href === "/")?.active).toBe(false);
    expect(navItems.find((item) => item.href === "/users")?.active).toBe(true);
  });

  it("exposes external capability as a top-level menu item", () => {
    const navItems = createConsoleNavItems("/capabilities");

    expect(navItems.find((item) => item.href === "/capabilities")).toMatchObject({
      active: true,
      label: "外部能力",
    });
  });

  it("builds breadcrumbs for known and unknown routes", () => {
    expect(createConsoleBreadcrumbItems("/capabilities", "外部能力")).toEqual([
      { label: "首页", href: "/" },
      { label: "外部能力" },
    ]);
    expect(createConsoleBreadcrumbItems("/users/42", "用户详情")).toEqual([
      { label: "首页", href: "/" },
      { label: "用户管理" },
    ]);
    expect(createConsoleBreadcrumbItems("/unknown", "未知页面")).toEqual([
      { label: "首页", href: "/" },
      { label: "未知页面" },
    ]);
  });
});
