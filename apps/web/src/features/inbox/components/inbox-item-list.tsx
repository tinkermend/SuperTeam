import { AlertTriangle, ArrowUpRight, Clock, FileText, Lightbulb } from "lucide-react";
import { Link } from "@tanstack/react-router";
import {
  IconTile,
  StatusPill,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import type { InboxItem } from "@/lib/api/inbox";
import { formatDateTime, formatRelativeTime } from "@/lib/format-time";
import { cn } from "@/lib/utils";

export { formatDateTime, formatRelativeTime };

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

/**
 * 紧凑列表：每行带风险 accent bar + 图标 + 标题 + 风险pill + 摘要 + 来源·节点 + 时间。
 * 装入 WorkSurface 软壳，保持 v3 脆数据面容器语义。
 */
export function InboxItemList({ items, onSelect, selectedItemId }: InboxItemListProps) {
  const highRiskCount = items.filter(
    (item) => item.risk_level === "blocked" || item.risk_level === "high",
  ).length;

  return (
    <WorkSurface className="flex min-h-0 flex-col xl:h-full">
      <div className="flex shrink-0 items-center justify-between gap-2 border-b border-v3-line bg-v3-card-soft px-5 py-3.5">
        <span className="text-sm font-bold text-v3-ink">待处理事项</span>
        <div className="flex items-center gap-2">
          {highRiskCount > 0 ? (
            <StatusPill tone="danger" showDot={false} className="px-2 py-0.5 text-[11px]">
              {highRiskCount} 高风险
            </StatusPill>
          ) : null}
          <span className="font-mono text-xs text-v3-ink-3">{items.length} 项 · 按风险排序</span>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {items.map((item) => {
          const isSelected = item.id === selectedItemId;
          const isHighRisk = item.risk_level === "blocked" || item.risk_level === "high";
          const isMediumRisk = item.risk_level === "medium";
          const accentShadow = isSelected
            ? "shadow-[inset_3px_0_0_var(--v3-brand)]"
            : isHighRisk
              ? "shadow-[inset_3px_0_0_var(--v3-danger)]"
              : isMediumRisk
                ? "shadow-[inset_3px_0_0_var(--v3-warn)]"
                : "shadow-[inset_3px_0_0_var(--v3-line-strong)]";
          const iconTone: V3Tone = isHighRisk
            ? "danger"
            : item.item_type === "project_decision"
              ? "artifact"
              : "info";
          const icon = isHighRisk ? (
            <AlertTriangle />
          ) : item.item_type === "project_decision" ? (
            <Lightbulb />
          ) : (
            <FileText />
          );

          return (
            <div
              key={item.id}
              role="button"
              tabIndex={0}
              aria-label={`打开事项：${item.title}`}
              aria-selected={isSelected}
              className={cn(
                "flex cursor-pointer items-start gap-3 border-b border-v3-line px-4 py-3 transition-colors",
                "hover:bg-v3-card-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-v3-brand/60",
                accentShadow,
                isSelected && "bg-v3-brand-soft",
              )}
              onClick={() => onSelect(item)}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  onSelect(item);
                }
              }}
            >
              <IconTile tone={iconTone} size="sm" className="mt-0.5">
                {icon}
              </IconTile>
              <div className="min-w-0 flex-1">
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <span
                    className={cn(
                      "text-left text-sm font-bold text-v3-ink",
                      isSelected && "text-v3-brand-deep",
                    )}
                  >
                    {item.title}
                  </span>
                  {item.risk_level ? (
                    <StatusPill
                      tone={riskTone[item.risk_level] ?? "mute"}
                      showDot={false}
                      className="px-2 py-0.5 text-[11px]"
                    >
                      {riskLabel[item.risk_level] ?? item.risk_level}
                    </StatusPill>
                  ) : null}
                </div>
                {item.summary ? (
                  <p className="mt-1 line-clamp-2 max-w-full break-words text-xs leading-5 text-v3-ink-2">
                    {item.summary}
                  </p>
                ) : null}
                <div className="mt-1.5 flex min-w-0 flex-wrap items-center gap-2 text-xs text-v3-ink-3">
                  <StatusPill
                    tone={item.item_type === "approval" ? "info" : "artifact"}
                    showDot={false}
                    className="px-2 py-0.5 text-[11px]"
                  >
                    {formatItemType(item)}
                  </StatusPill>
                  <span className="min-w-0 truncate font-mono text-[11px]">
                    {formatContext(item) ?? formatSourceType(item)} · {formatCurrentNode(item)}
                  </span>
                  <span className="inline-flex items-center gap-1 whitespace-nowrap">
                    <Clock aria-hidden className="size-3" />
                    {formatRelativeTime(item.last_activity_at)}
                  </span>
                  <Link
                    className="inline-flex w-fit items-center gap-1 font-semibold text-v3-brand-deep hover:text-v3-brand"
                    onClick={(event) => event.stopPropagation()}
                    to={resolveInboxHref(item)}
                  >
                    查看上下文
                    <ArrowUpRight aria-hidden className="size-3" />
                  </Link>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </WorkSurface>
  );
}

export function formatContext(item: InboxItem) {
  // 服务端读时补名优先于 context 快照;两者都缺时才回退裸 id。
  const projectName =
    item.source_project_name ??
    readContextText(item.context, ["project_name", "project", "project_title"]);
  const sourceName =
    readContextText(item.context, ["source_title", "approval_title", "task_title"]) ??
    item.source_task_name;

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
  const projectDecisionPath = resolveProjectDecisionPath(item, route);
  if (projectDecisionPath) {
    return projectDecisionPath;
  }

  if (route && isSafeAppPath(route)) {
    return anchor ? `${route}#${encodeURIComponent(anchor)}` : route;
  }

  const path = resolveSafeInboxPath(undefined, item.source_project_id);
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

function resolveProjectDecisionPath(item: InboxItem, route?: string) {
  if (item.item_type !== "project_decision" || !item.source_project_id || !item.source_id) {
    return undefined;
  }

  if (route && isSafeAppPath(route) && !isProjectDeepLink(route, item.source_project_id)) {
    return undefined;
  }

  const params = new URLSearchParams();
  params.set("tab", "approval");
  params.set("focus", item.source_id);
  return `/projects/${encodeURIComponent(item.source_project_id)}?${params.toString()}`;
}

function isProjectDeepLink(route: string, projectId: string) {
  try {
    const parsed = new URL(route, "http://superteam.local");
    const expectedPath = `/projects/${encodeURIComponent(projectId)}`;
    return parsed.origin === "http://superteam.local" && parsed.pathname === expectedPath;
  } catch {
    return false;
  }
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

/** 已等待时长（毫秒）→ "X 时 Y 分" 格式，负值钳为 0。 */
export function formatElapsedDuration(ms: number): string {
  const clamped = Math.max(0, ms);
  const totalMinutes = Math.floor(clamped / 60000);
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (hours > 0) return `${hours} 时 ${minutes} 分`;
  return `${minutes} 分`;
}

/** 已等待时长（毫秒）→ 指标卡短格式 "X.Yh" / "Xm"，负值钳为 0。 */
export function formatWaitShort(ms: number): string {
  const clamped = Math.max(0, ms);
  const totalHours = clamped / 3600000;
  if (totalHours >= 1) return `${totalHours.toFixed(1)}h`;
  const totalMinutes = Math.floor(clamped / 60000);
  return `${totalMinutes}m`;
}
