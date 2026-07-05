import { AlertTriangle, ArrowUpRight, Clock, FileText } from "lucide-react";
import { Link } from "@tanstack/react-router";
import {
  IconTile,
  StatusPill,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import type { InboxItem } from "@/lib/api/inbox";
import { cn } from "@/lib/utils";

type InboxItemListProps = {
  items: InboxItem[];
  onSelect: (item: InboxItem) => void;
  selectedItemId: string | null;
};

export const riskLabel: Record<string, string> = {
  blocked: "阻断",
  high: "高风险",
  low: "低风险",
  medium: "中风险",
};

export const riskTone: Record<string, V3Tone> = {
  blocked: "danger",
  high: "danger",
  low: "mute",
  medium: "warn",
};

const itemTypeLabel: Record<string, string> = {
  approval: "审批",
  project_decision: "项目决策",
};

const sourceTypeLabel: Record<string, string> = {
  approval_request: "审批请求",
  project_decision_request: "项目决策请求",
};

export function InboxItemList({ items, onSelect, selectedItemId }: InboxItemListProps) {
  return (
    <WorkSurface>
      <div className="flex flex-col gap-1 border-b border-v3-line px-5 py-4">
        <h2 className="text-lg font-extrabold text-v3-ink">待处理事项</h2>
        <p className="text-[13px] text-v3-ink-2">逐项查看需要你同意、审核、确认或验收的事项。</p>
      </div>
      <V3Table className="table-fixed">
        <colgroup>
          <col className="w-[35%]" />
          <col className="w-[10%]" />
          <col className="w-[27%]" />
          <col className="w-[16%]" />
          <col className="w-[12%]" />
        </colgroup>
        <thead>
          <tr>
            <V3Th className="whitespace-normal">事项</V3Th>
            <V3Th className="whitespace-normal">类型</V3Th>
            <V3Th className="whitespace-normal">来源 / 关联对象</V3Th>
            <V3Th className="whitespace-normal">当前节点</V3Th>
            <V3Th className="whitespace-normal">更新时间</V3Th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => {
            const contextLabel = formatContext(item);
            const currentNode = formatCurrentNode(item);
            const riskAccent =
              item.risk_level === "blocked" || item.risk_level === "high"
                ? "[&>td:first-child]:shadow-[inset_3px_0_0_var(--v3-danger)]"
                : item.risk_level === "medium"
                  ? "[&>td:first-child]:shadow-[inset_3px_0_0_var(--v3-warn)]"
                  : undefined;
            const isSelected = item.id === selectedItemId;

            return (
              <V3Tr
                aria-label={`打开事项：${item.title}`}
                aria-selected={isSelected}
                className={cn(
                  "group cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-v3-brand/60",
                  riskAccent,
                  isSelected && "[&>td]:bg-v3-brand-soft/60 [&>td:first-child]:shadow-[inset_3px_0_0_var(--v3-brand)]",
                )}
                key={item.id}
                onClick={() => onSelect(item)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    onSelect(item);
                  }
                }}
                role="button"
                tabIndex={0}
              >
                <V3Td className="min-w-0">
                  <div className="flex min-w-0 gap-3">
                    <IconTile className="hidden shrink-0 2xl:grid" tone={item.risk_level === "high" || item.risk_level === "blocked" ? "danger" : "info"} size="sm">
                      {item.risk_level === "high" || item.risk_level === "blocked" ? <AlertTriangle /> : <FileText />}
                    </IconTile>
                    <div className="min-w-0">
                      <div className="flex min-w-0 flex-wrap items-center gap-2">
                        <span className="min-w-0 text-left text-[15px] font-bold text-v3-ink group-hover:text-v3-brand">
                          {item.title}
                        </span>
                        {item.risk_level ? (
                          <StatusPill tone={riskTone[item.risk_level] ?? "mute"}>
                            {riskLabel[item.risk_level] ?? item.risk_level}
                          </StatusPill>
                        ) : null}
                      </div>
                      {item.summary ? (
                        <p className="mt-1 line-clamp-2 max-w-full break-words text-[13px] leading-5 text-v3-ink-2">
                          {item.summary}
                        </p>
                      ) : null}
                    </div>
                  </div>
                </V3Td>
                <V3Td className="min-w-0">
                  <StatusPill className="max-w-full px-2 text-[11px]" showDot={false} tone={item.item_type === "approval" ? "info" : "artifact"}>
                    {formatItemType(item)}
                  </StatusPill>
                </V3Td>
                <V3Td className="min-w-0">
                  <div className="flex min-w-0 flex-col gap-1 text-[13px] text-v3-ink-2">
                    <span className="line-clamp-2 break-words font-semibold text-v3-ink">
                      {contextLabel ?? formatSourceType(item)}
                    </span>
                    {item.source_task_id ? (
                      <span className="line-clamp-2 break-all font-mono text-xs text-v3-ink-3">任务 {item.source_task_id}</span>
                    ) : null}
                    <Link
                      className="inline-flex w-fit items-center gap-1 font-semibold text-v3-brand-deep hover:text-v3-brand"
                      onClick={(event) => event.stopPropagation()}
                      to={resolveInboxHref(item)}
                    >
                      查看上下文
                      <ArrowUpRight aria-hidden className="size-3.5" />
                    </Link>
                  </div>
                </V3Td>
                <V3Td className="min-w-0">
                  <div className="flex min-w-0 flex-col gap-1 text-[13px] text-v3-ink-2">
                    <span className="line-clamp-2 break-words font-semibold text-v3-ink">{currentNode}</span>
                    <span className="line-clamp-2 break-words text-xs text-v3-ink-3">{formatSourceType(item)}</span>
                  </div>
                </V3Td>
                <V3Td className="min-w-0 text-v3-ink-2 tabular-nums">
                  <span className="inline-flex min-w-0 items-center gap-1 whitespace-normal break-words">
                    <Clock aria-hidden className="hidden size-3.5 shrink-0 2xl:block" />
                    {formatDateTime(item.last_activity_at)}
                  </span>
                </V3Td>
              </V3Tr>
            );
          })}
        </tbody>
      </V3Table>
    </WorkSurface>
  );
}

export function formatContext(item: InboxItem) {
  const projectName = readContextText(item.context, ["project_name", "project", "project_title"]);
  const sourceName = readContextText(item.context, ["source_title", "approval_title", "task_title"]);

  if (projectName && sourceName) {
    return `${projectName} / ${sourceName}`;
  }
  return projectName ?? sourceName ?? (item.source_project_id ? `项目 ${item.source_project_id}` : undefined);
}

export function formatItemType(item: InboxItem) {
  return itemTypeLabel[item.item_type] ?? item.item_type;
}

export function formatSourceType(item: InboxItem) {
  return sourceTypeLabel[item.source_type] ?? item.source_type;
}

export function formatCurrentNode(item: InboxItem) {
  return (
    readContextText(item.context, [
      "current_node",
      "node_title",
      "workflow_node",
      "stage",
      "decision_type",
    ]) ?? formatItemType(item)
  );
}

export function readContextText(context: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = context[key];
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return undefined;
}

export function resolveInboxHref(item: InboxItem) {
  const route = typeof item.deep_link.route === "string" ? item.deep_link.route : undefined;
  const anchor = typeof item.deep_link.anchor === "string" ? item.deep_link.anchor : undefined;
  const path = resolveSafeInboxPath(route, item.source_project_id);

  return anchor ? `${path}#${encodeURIComponent(anchor)}` : path;
}

export function resolveSafeInboxPath(route: string | undefined, sourceProjectId: string | undefined) {
  if (route && isSafeAppPath(route)) {
    return route;
  }

  if (sourceProjectId) {
    return `/projects/${encodeURIComponent(sourceProjectId)}`;
  }

  return "/inbox";
}

export function isSafeAppPath(route: string) {
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

export function formatDateTime(value: string) {
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
