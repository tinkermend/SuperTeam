import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ConsoleShell } from "./console-shell";

const navItems = [
  { label: "任务中心", href: "/tasks", active: true },
  { label: "审批中心", href: "/approvals" },
  { label: "审计日志", href: "/audit" },
];

describe("ConsoleShell", () => {
  it("renders the reusable console shell without owning page content", () => {
    render(
      <ConsoleShell
        activeWorkspace="默认工作区"
        navItems={navItems}
        pageTitle="任务中心"
        productName="SuperTeam"
        tenantName="示例科技"
        user={{ name: "张伟", role: "平台管理员" }}
      >
        <section aria-label="任务内容">主链路内容</section>
      </ConsoleShell>,
    );

    expect(screen.getByText("SuperTeam")).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "主导航" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "任务中心" })).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: "全局搜索" })).toBeInTheDocument();
    expect(screen.getByLabelText("任务内容")).toHaveTextContent("主链路内容");

    const activeLink = within(screen.getByRole("navigation", { name: "主导航" })).getByRole("link", {
      name: "任务中心",
    });
    expect(activeLink).toHaveAttribute("aria-current", "page");
  });

  it("renders account actions when provided", () => {
    render(
      <ConsoleShell
        activeWorkspace="默认工作区"
        navItems={navItems}
        pageTitle="任务中心"
        productName="SuperTeam"
        tenantName="示例科技"
        user={{ name: "admin", role: "平台管理员" }}
        userActions={<button type="button">退出登录</button>}
      >
        <section aria-label="任务内容">主链路内容</section>
      </ConsoleShell>,
    );

    fireEvent.click(screen.getByRole("button", { name: "admin 平台管理员" }));

    expect(screen.getByRole("button", { name: "退出登录" })).toBeInTheDocument();
  });

  it("renders breadcrumbs when provided", () => {
    render(
      <ConsoleShell
        activeWorkspace="默认工作区"
        breadcrumbs={[
          { label: "首页", href: "/" },
          { label: "用户管理" },
        ]}
        navItems={navItems}
        pageTitle="用户管理"
        productName="SuperTeam"
        tenantName="示例科技"
        user={{ name: "admin", role: "平台管理员" }}
      >
        <section aria-label="用户内容">用户列表</section>
      </ConsoleShell>,
    );

    const breadcrumbs = screen.getByRole("navigation", { name: "面包屑" });
    expect(within(breadcrumbs).getByRole("link", { name: "首页" })).toHaveAttribute("href", "/");
    expect(within(breadcrumbs).getByText("用户管理")).toHaveAttribute("aria-current", "page");
  });
});
