import { useEffect } from "react";
import { useRouterState } from "@tanstack/react-router";

const DEFAULT_TITLE = "炬枢平台";

/** 路径前缀 → 页名（最长前缀优先）。详情页可用 usePageTitle(对象名) 覆盖第二段。 */
const ROUTE_TITLES: Array<{ prefix: string; title: string }> = [
  { prefix: "/inbox", title: "收件箱" },
  { prefix: "/run-overview", title: "运行总览" },
  { prefix: "/projects", title: "项目管理" },
  { prefix: "/employees", title: "数字员工" },
  { prefix: "/skills", title: "技能市场" },
  { prefix: "/teams", title: "团队管理" },
  { prefix: "/automations", title: "自动化任务" },
  { prefix: "/capabilities", title: "外部能力" },
  { prefix: "/mcp", title: "MCP 管理" },
  { prefix: "/scenario-templates", title: "场景模板" },
  { prefix: "/role-vocabulary", title: "角色词表" },
  { prefix: "/collaboration", title: "协作集成" },
  { prefix: "/runtime", title: "Runtime 节点" },
  { prefix: "/permissions", title: "权限中心" },
  { prefix: "/costs", title: "成本管理" },
  { prefix: "/users", title: "用户管理" },
  { prefix: "/audit", title: "审计中心" },
  { prefix: "/logs", title: "日志管理" },
  { prefix: "/system-config", title: "系统配置" },
  { prefix: "/settings", title: "系统配置" },
  { prefix: "/task-launches", title: "任务中枢" },
  { prefix: "/workflows", title: "流程实例" },
  { prefix: "/tasks", title: "任务中枢" },
  { prefix: "/", title: "任务中枢" },
];

function titleForPath(pathname: string): string {
  const normalized = pathname.replace(/\/$/, "") || "/";
  const hit = ROUTE_TITLES
    .filter((entry) => normalized === entry.prefix || normalized.startsWith(`${entry.prefix}/`) || (entry.prefix === "/" && normalized === "/"))
    .sort((a, b) => b.prefix.length - a.prefix.length)[0];
  return hit?.title ?? "工作台";
}

/**
 * 设置 document.title 为 `炬枢 · {页名}` 或 `炬枢 · {detail}`。
 * - 不传参：随路由 pathname 自动映射
 * - 传 string：详情覆盖（如对象名），路由变化时仍先套页名再被本 effect 覆盖
 */
export function usePageTitle(detail?: string | null) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  useEffect(() => {
    const page = titleForPath(pathname);
    const segment = detail?.trim() || page;
    document.title = segment ? `炬枢 · ${segment}` : DEFAULT_TITLE;
    return () => {
      document.title = DEFAULT_TITLE;
    };
  }, [pathname, detail]);
}

export function formatPageTitle(pageOrDetail: string): string {
  const segment = pageOrDetail.trim();
  return segment ? `炬枢 · ${segment}` : DEFAULT_TITLE;
}
