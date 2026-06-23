import { AlertTriangle, ArrowUpRight, Clock, FileText } from "lucide-react";
import { Link } from "@tanstack/react-router";
import {
  IconTile,
  StatusPill,
  V3Button,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3ButtonVariant,
  type V3Tone,
} from "@/components/superteam";
import type { InboxAction, InboxItem, InboxViewMode } from "@/lib/api/inbox";
import { cn } from "@/lib/utils";

type InboxItemListProps = {
  items: InboxItem[];
  onAction: (item: InboxItem, action: InboxAction) => void;
  view: InboxViewMode;
};

const riskLabel: Record<string, string> = {
  blocked: "阻断",
  high: "高风险",
  low: "低风险",
  medium: "中风险",
};

const riskTone: Record<string, V3Tone> = {
  blocked: "danger",
  high: "danger",
  low: "mute",
  medium: "warn",
};

const actionToneVariant: Record<string, V3ButtonVariant> = {
  danger: "danger",
  destructive: "danger",
  primary: "primary",
  success: "outline",
  warning: "outline",
};

const actionToneClass: Record<string, string> = {
  primary: "",
  success: "border-v3-ok text-v3-ok hover:bg-v3-ok-soft",
  warning: "border-v3-warn text-v3-warn hover:bg-v3-warn-soft",
};

export function InboxItemList({ items, onAction, view }: InboxItemListProps) {
  return (
    <WorkSurface>
      <div className="flex flex-col gap-1 border-b border-v3-line px-5 py-4">
        <h2 className="text-lg font-extrabold text-v3-ink">待处理事项</h2>
        <p className="text-[13px] text-v3-ink-2">逐项查看来源、风险、时间和可执行动作。</p>
      </div>
      <V3Table>
        <thead>
          <tr>
            <V3Th className="min-w-[22rem]">事项</V3Th>
            <V3Th className="min-w-[16rem]">上下文</V3Th>
            <V3Th>更新时间</V3Th>
            <V3Th className="text-right">操作</V3Th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => {
            const actions = Array.isArray(item.actions) ? item.actions : [];
            const contextLabel = formatContext(item);
            const rowTone = item.risk_level === "blocked" || item.risk_level === "high" ? "danger" : item.risk_level === "medium" ? "warn" : undefined;

            return (
              <V3Tr key={item.id} tone={rowTone}>
                <V3Td>
                  <div className="flex min-w-0 gap-3">
                    <IconTile tone={item.risk_level === "high" || item.risk_level === "blocked" ? "danger" : "info"} size="sm">
                      {item.risk_level === "high" || item.risk_level === "blocked" ? <AlertTriangle /> : <FileText />}
                    </IconTile>
                    <div className="min-w-0">
                      <div className="flex min-w-0 flex-wrap items-center gap-2">
                        <h2 className="min-w-0 text-[15px] font-bold text-v3-ink">
                          {item.title}
                        </h2>
                        {item.risk_level ? (
                          <StatusPill tone={riskTone[item.risk_level] ?? "mute"}>
                            {riskLabel[item.risk_level] ?? item.risk_level}
                          </StatusPill>
                        ) : null}
                      </div>
                      {item.summary ? (
                        <p className="mt-1 max-w-[38rem] text-[13px] leading-5 text-v3-ink-2">
                          {item.summary}
                        </p>
                      ) : null}
                    </div>
                  </div>
                </V3Td>
                <V3Td>
                  <div className="flex min-w-0 flex-col gap-1 text-[13px] text-v3-ink-2">
                    {contextLabel ? <span className="truncate font-semibold text-v3-ink">{contextLabel}</span> : null}
                    {item.source_task_id ? (
                      <span className="truncate font-mono text-xs text-v3-ink-3">任务 {item.source_task_id}</span>
                    ) : null}
                    <Link
                      className="inline-flex w-fit items-center gap-1 font-semibold text-v3-brand-deep hover:text-v3-brand"
                      to={resolveInboxHref(item)}
                    >
                      查看上下文
                      <ArrowUpRight aria-hidden className="size-3.5" />
                    </Link>
                  </div>
                </V3Td>
                <V3Td className="whitespace-nowrap text-v3-ink-2 tabular-nums">
                  <span className="inline-flex items-center gap-1">
                    <Clock aria-hidden className="size-3.5" />
                    {formatDateTime(item.last_activity_at)}
                  </span>
                </V3Td>
                <V3Td>
                  {view === "mine" && actions.length > 0 ? (
                    <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                      {actions.map((action) => (
                        <V3Button
                          className={cn(actionToneClass[action.tone])}
                          key={action.key}
                          onClick={() => onAction(item, action)}
                          size="sm"
                          type="button"
                          variant={actionToneVariant[action.tone] ?? "outline"}
                        >
                          {action.label}
                        </V3Button>
                      ))}
                    </div>
                  ) : (
                    <span className="block text-right text-xs font-semibold text-v3-ink-3">只读</span>
                  )}
                </V3Td>
              </V3Tr>
            );
          })}
        </tbody>
      </V3Table>
    </WorkSurface>
  );
}

function formatContext(item: InboxItem) {
  const projectName = readContextText(item.context, ["project_name", "project", "project_title"]);
  const sourceName = readContextText(item.context, ["source_title", "approval_title", "task_title"]);

  if (projectName && sourceName) {
    return `${projectName} / ${sourceName}`;
  }
  return projectName ?? sourceName ?? (item.source_project_id ? `项目 ${item.source_project_id}` : undefined);
}

function readContextText(context: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = context[key];
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return undefined;
}

function resolveInboxHref(item: InboxItem) {
  const route = typeof item.deep_link.route === "string" ? item.deep_link.route : undefined;
  const anchor = typeof item.deep_link.anchor === "string" ? item.deep_link.anchor : undefined;
  const path = resolveSafeInboxPath(route, item.source_project_id);

  return anchor ? `${path}#${encodeURIComponent(anchor)}` : path;
}

function resolveSafeInboxPath(route: string | undefined, sourceProjectId: string | undefined) {
  if (route && isSafeAppPath(route)) {
    return route;
  }

  if (sourceProjectId) {
    return `/projects/${encodeURIComponent(sourceProjectId)}`;
  }

  return "/inbox";
}

function isSafeAppPath(route: string) {
  if (
    !route.startsWith("/") ||
    route.startsWith("//") ||
    route.includes("\\") ||
    /[\u0000-\u001f\u007f]/.test(route) ||
    /^[a-zA-Z][a-zA-Z\d+.-]*:/.test(route)
  ) {
    return false;
  }

  try {
    const parsed = new URL(route, "http://superteam.local");
    return parsed.origin === "http://superteam.local" && parsed.pathname.startsWith("/");
  } catch {
    return false;
  }
}

function formatDateTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
  }).format(date);
}
