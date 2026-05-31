import {
  Bot,
  ClipboardCheck,
  FileClock,
  Home,
  LayoutGrid,
  Network,
  ShieldCheck,
  Users,
  Workflow,
} from "lucide-react";
import type { ConsoleNavItem } from "@superteam/views";

import type { ConsoleBreadcrumbItem } from "@superteam/views";

const consoleNavItems: Array<Omit<ConsoleNavItem, "active">> = [
  { label: "首页", href: "/", icon: Home },
  { label: "工作台", href: "/workspace", icon: LayoutGrid },
  { label: "任务中心", href: "/tasks", icon: ClipboardCheck },
  { label: "数字员工", href: "/employees", icon: Bot },
  { label: "流程编排", href: "/workflows", icon: Workflow },
  { label: "外部能力", href: "/capabilities", icon: Network },
  { label: "审批中心", href: "/approvals", icon: ShieldCheck },
  { label: "用户管理", href: "/users", icon: Users },
  { label: "审计日志", href: "/audit", icon: FileClock },
];

export function isActiveConsoleRoute(pathname: string, href: string) {
  if (href === "/") {
    return pathname === "/";
  }

  return pathname === href || pathname.startsWith(`${href}/`);
}

export function createConsoleNavItems(pathname: string): ConsoleNavItem[] {
  return consoleNavItems.map((item) => ({
    ...item,
    active: isActiveConsoleRoute(pathname, item.href),
  }));
}

export function createConsoleBreadcrumbItems(pathname: string, pageTitle: string): ConsoleBreadcrumbItem[] {
  const matchedItem = consoleNavItems
    .filter((item) => item.href !== "/" && isActiveConsoleRoute(pathname, item.href))
    .sort((left, right) => right.href.length - left.href.length)[0];

  if (pathname === "/") {
    return [{ label: "首页" }];
  }

  return [
    { label: "首页", href: "/" },
    { label: matchedItem?.label ?? pageTitle },
  ];
}
