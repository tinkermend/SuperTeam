"use client";

import { usePathname } from "next/navigation";
import { Bell, GitFork, LogOut } from "lucide-react";
import type { ReactNode } from "react";

import { useAuth } from "@superteam/core/auth";
import { ConsoleShell } from "@superteam/views";

import { Button } from "@/components/ui/button";
import { createConsoleBreadcrumbItems, createConsoleNavItems } from "@/console-nav";

type ConsoleAppShellProps = {
  children: ReactNode;
  pageActions?: ReactNode;
  pageDescription?: ReactNode;
  pageTitle: string;
};

export function ConsoleAppShell({ children, pageActions, pageDescription, pageTitle }: ConsoleAppShellProps) {
  const pathname = usePathname();
  const { logout, user } = useAuth();
  const userName = user?.username ?? "未知用户";

  return (
    <ConsoleShell
      activeWorkspace="默认工作区"
      breadcrumbs={createConsoleBreadcrumbItems(pathname, pageTitle)}
      navItems={createConsoleNavItems(pathname)}
      pageActions={pageActions}
      pageDescription={pageDescription}
      pageTitle={pageTitle}
      productName="SuperTeam"
      tenantName="示例科技有限公司"
      user={{ name: userName, role: "平台成员" }}
      userActions={
        <button
          className="flex h-8 items-center gap-2 rounded-md px-2 text-left text-sm text-destructive hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onClick={() => {
            void logout();
          }}
          role="menuitem"
          type="button"
        >
          <LogOut data-icon="inline-start" aria-hidden="true" />
          退出登录
        </button>
      }
    >
      {children}
    </ConsoleShell>
  );
}

export function DefaultConsolePageActions() {
  return (
    <div className="hidden items-center gap-2 md:flex">
      <Button aria-label="通知" size="icon" variant="ghost">
        <Bell aria-hidden="true" />
      </Button>
      <Button aria-label="能力面板" size="icon" variant="ghost">
        <GitFork aria-hidden="true" />
      </Button>
    </div>
  );
}
