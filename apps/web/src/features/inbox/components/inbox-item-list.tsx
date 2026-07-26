import { AlertTriangle, ArrowUpRight, Clock, FileText, Lightbulb } from "lucide-react";
import { Link } from "@tanstack/react-router";
import {
  IconTile,
  StatusPill,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import type { InboxItem } from "@/lib/api/inbox";
import { formatDateTime, formatRelativeTime } from "@/lib/format-time";
import { decisionTypeLabel, humanTaskKindLabel } from "@/lib/status-labels";
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
  medium: "中风险"
};

export const riskTone: Record<string, Tone> = {
  blocked: "danger",
  high: "danger",
  low: "mute",
  medium: "warn"
};

const itemTypeLabel: Record<string, string> = {
  approval: "审批",
  project_decision: "项目决策",
  team_pending_delete: "团队待删",
  // 运行恢复链已随「运行必须归属项目」spec 退役；键保留给历史 resolved 事项渲染。
  digital_employee_run_recovery: "运行恢复"
};

const sourceTypeLabel: Record<string, string> = {
  approval_request: "审批请求",
  project_decision_request: "项目决策请求",
  team_pending_delete: "团队待删",
  digital_employee_run: "数字员工运行"
};

type InboxSection = { key: string; label: string; items: InboxItem[] };

// §6.1 收件箱按人类待办类型分组:计划确认 / 执行放行 / 下游放行 / 验收签署 / 结项确认 /
// 异常处理。类型取自服务端 HumanTask kind(§4.2);其余(planning_gap / task_failure_recovery /
// 其它 project_task_* 以及审批 / 运行恢复 / 团队待删)归入"异常处理"。组内保持上游顺序
// (上游按风险/关注度排序),不再二次排序。
const INBOX_CATEGORY_ORDER: { key: string; label: string; kinds: string[] }[] = [
  { key: "plan_review", label: "计划确认", kinds: ["plan_review"] },
  { key: "dispatch_release", label: "执行放行", kinds: ["dispatch_release"] },
  { key: "downstream_release", label: "下游放行", kinds: ["downstream_release"] },
  { key: "acceptance_sign", label: "验收签署", kinds: ["acceptance_sign"] },
  { key: "closure_confirm", label: "结项确认", kinds: ["closure_confirm"] },
];
const INBOX_EXCEPTION_SECTION = { key: "exception", label: "异常处理" };

function inboxCategoryKey(item: InboxItem): string {
  const kind = typeof item.kind === "string" ? item.kind : "";
  for (const category of INBOX_CATEGORY_ORDER) {
    if (category.kinds.includes(kind)) {
      return category.key;
    }
  }
  return INBOX_EXCEPTION_SECTION.key;
}

/** Bucket inbox items into the §6.1 human-task categories, preserving upstream order within each. */
export function groupInboxItems(items: InboxItem[]): InboxSection[] {
  const buckets = new Map<string, InboxItem[]>();
  for (const item of items) {
    const key = inboxCategoryKey(item);
    const bucket = buckets.get(key) ?? [];
    bucket.push(item);
    buckets.set(key, bucket);
  }
  const sections: InboxSection[] = [];
  for (const category of INBOX_CATEGORY_ORDER) {
    const bucketItems = buckets.get(category.key);
    if (bucketItems && bucketItems.length > 0) {
      sections.push({ key: category.key, label: category.label, items: bucketItems });
    }
  }
  const exception = buckets.get(INBOX_EXCEPTION_SECTION.key);
  if (exception && exception.length > 0) {
    sections.push({ key: INBOX_EXCEPTION_SECTION.key, label: INBOX_EXCEPTION_SECTION.label, items: exception });
  }
  return sections;
}

/**
 * 紧凑列表：每行带风险 accent bar + 图标 + 标题 + 风险pill + 摘要 + 来源·节点 + 时间。
 * 装入 WorkSurface 软壳，保持 v3 脆数据面容器语义。
 */
export function InboxItemList({ items, onSelect, selectedItemId }: InboxItemListProps) {
  const highRiskCount = items.filter(
    (item) => item.risk_level === "blocked" || item.risk_level === "high",
  ).length;

  const sections = groupInboxItems(items);

  const renderRow = (item: InboxItem) => {
          const isSelected = item.id === selectedItemId;
          const isHighRisk = item.risk_level === "blocked" || item.risk_level === "high";
          const isMediumRisk = item.risk_level === "medium";
          const accentShadow = isSelected
            ? "shadow-[inset_3px_0_0_var(--brand)]"
            : isHighRisk
              ? "shadow-[inset_3px_0_0_var(--danger)]"
              : isMediumRisk
                ? "shadow-[inset_3px_0_0_var(--warn)]"
                : "shadow-[inset_3px_0_0_var(--line-strong)]";
          const iconTone: Tone = isHighRisk
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
                "flex cursor-pointer items-start gap-3 border-b border-line px-4 py-3 transition-colors",
                "hover:bg-card-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand/60",
                accentShadow,
                isSelected && "bg-brand-soft",
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
                      "text-left text-sm font-bold text-ink",
                      isSelected && "text-brand-deep",
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
                  <p className="mt-1 line-clamp-2 max-w-full break-words text-xs leading-5 text-ink-2">
                    {item.summary}
                  </p>
                ) : null}
                {item.why ? (
                  <p className="mt-1 line-clamp-2 max-w-full break-words text-xs leading-5 text-ink-2">
                    {item.why}
                  </p>
                ) : null}
                <InboxProgressBar progress={readInboxProgress(item)} />
                <div className="mt-1.5 flex min-w-0 flex-wrap items-center gap-2 text-xs text-ink-3">
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
                    className="inline-flex w-fit items-center gap-1 font-semibold text-brand-deep hover:text-brand"
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
  };

  return (
    <WorkSurface className="flex min-h-0 flex-col xl:h-full">
      <div className="flex shrink-0 items-center justify-between gap-2 border-b border-line bg-card-soft px-5 py-3.5">
        <span className="text-sm font-bold text-ink">待处理事项</span>
        <div className="flex items-center gap-2">
          {highRiskCount > 0 ? (
            <StatusPill tone="danger" showDot={false} className="px-2 py-0.5 text-[11px]">
              {highRiskCount} 高风险
            </StatusPill>
          ) : null}
          <span className="font-mono text-xs text-ink-3">{items.length} 项 · 按类型分组</span>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {sections.map((section) => (
          <section key={section.key}>
            {/* §6.1 领域分组表头(计划确认/执行放行/下游放行/验收签署/结项确认/异常处理);组内保持上游关注度排序。 */}
            <div className="flex items-center justify-between gap-2 border-b border-line bg-card-soft/80 px-4 py-1.5 text-[11px] font-bold text-ink-2">
              <span>{section.label}</span>
              <span className="font-mono text-ink-3">{section.items.length}</span>
            </div>
            {section.items.map(renderRow)}
          </section>
        ))}
      </div>
    </WorkSurface>
  );
}

export type InboxProgress = {
  step: number;
  total: number;
  label: string;
};

/** Read HumanTask.progress from top-level field or context (§4.1 / §6.1). */
export function readInboxProgress(item: InboxItem): InboxProgress | null {
  const fromTop = item.progress;
  if (
    fromTop &&
    typeof fromTop.step === "number" &&
    typeof fromTop.total === "number" &&
    typeof fromTop.label === "string" &&
    fromTop.total > 0
  ) {
    return fromTop;
  }
  const raw = item.context?.progress;
  if (!raw || typeof raw !== "object") return null;
  const record = raw as Record<string, unknown>;
  const step = typeof record.step === "number" ? record.step : Number(record.step);
  const total = typeof record.total === "number" ? record.total : Number(record.total);
  const label = typeof record.label === "string" ? record.label : "";
  if (!Number.isFinite(step) || !Number.isFinite(total) || total <= 0 || !label) {
    return null;
  }
  return { step, total, label };
}

/** §6.1 闭环进度条：细轨 + 当前步填充 + 中文 label。 */
export function InboxProgressBar({ progress }: { progress: InboxProgress | null }) {
  if (!progress) return null;
  const ratio = Math.max(0, Math.min(1, progress.step / progress.total));
  return (
    <div className="mt-1.5 grid gap-1" data-testid="inbox-progress-bar">
      <div className="h-1 overflow-hidden rounded-full bg-line">
        <div
          className="h-full rounded-full bg-brand transition-[width]"
          style={{ width: `${Math.round(ratio * 100)}%` }}
        />
      </div>
      <p className="line-clamp-1 text-[11px] leading-4 text-ink-3">{progress.label}</p>
    </div>
  );
}

export type InboxDemandRef = {
  id?: string;
  title: string;
  taskTitles: string[];
};

/** Prefer demand/task identity from context; project is only a fallback container. */
export function readDemandRefs(item: InboxItem): InboxDemandRef[] {
  const context = item.context ?? {};
  const rawDemands = context.demands;
  if (Array.isArray(rawDemands) && rawDemands.length > 0) {
    const refs: InboxDemandRef[] = [];
    for (const entry of rawDemands) {
      if (!entry || typeof entry !== "object") continue;
      const record = entry as Record<string, unknown>;
      const id = typeof record.id === "string" && record.id.trim() ? record.id.trim() : undefined;
      const title =
        typeof record.title === "string" && record.title.trim()
          ? record.title.trim()
          : (id ?? "");
      if (!title) continue;
      const taskTitles: string[] = [];
      if (Array.isArray(record.task_titles)) {
        for (const taskTitle of record.task_titles) {
          if (typeof taskTitle === "string" && taskTitle.trim()) {
            taskTitles.push(taskTitle.trim());
          }
        }
      }
      refs.push({ id, title, taskTitles });
    }
    if (refs.length > 0) return refs;
  }

  const demandId =
    readContextText(context, ["primary_demand_id", "demand_id"]) ?? undefined;
  const demandTitle = readContextText(context, ["demand_title"]);
  if (demandTitle || demandId) {
    return [
      {
        id: demandId,
        title: demandTitle ?? demandId!,
        taskTitles: []
},
    ];
  }
  return [];
}

export function primaryDemandLabel(item: InboxItem): string | undefined {
  const refs = readDemandRefs(item);
  if (refs.length === 0) return undefined;
  if (refs.length === 1) return refs[0].title;
  return `${refs[0].title} 等 ${refs.length} 项`;
}

export function primaryTaskLabel(item: InboxItem): string | undefined {
  const refs = readDemandRefs(item);
  for (const ref of refs) {
    if (ref.taskTitles[0]) return ref.taskTitles[0];
  }
  return readContextText(item.context, ["task_title"]) ?? item.source_task_name ?? undefined;
}

export function formatContext(item: InboxItem) {
  const demandLabel = primaryDemandLabel(item);
  const taskLabel = primaryTaskLabel(item);
  if (demandLabel) {
    return taskLabel && taskLabel !== demandLabel
      ? `${demandLabel}（任务：${taskLabel}）`
      : demandLabel;
  }

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
  const node = readContextText(item.context, ["current_node", "node_title", "workflow_node", "stage"]);
  if (node) {
    return node;
  }
  // §6.1/§12 + F3：meta 行禁止裸英文技术枚举。规范化 kind(§4.2 中文名)优先,再退
  // decision_type 词表映射,最后 itemType——一律经 status-labels.ts,不留 project_acceptance 之类原文。
  if (item.kind) {
    return humanTaskKindLabel(item.kind);
  }
  const decisionType = readContextText(item.context, ["decision_type"]);
  if (decisionType) {
    return decisionTypeLabel(decisionType);
  }
  return formatItemType(item);
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
  // F3(§5.4.3): primary_surface 是服务端算好的唯一权威落点。前端一律以它为准,不再各自
  // 推导深链(旧的 resolveWorkflowInstanceHref / resolveWorkflowTemplateHref / reviewHref
  // 已下线),从根上消除"同一待办在不同入口跳不同页"。
  // 优先读 HumanTask.primary_surface 具名字段；deep_link 仅作旧载荷兼容。
  const primarySurface =
    (typeof item.primary_surface === "string" ? item.primary_surface : undefined) ||
    (typeof item.deep_link.primary_surface === "string" ? item.deep_link.primary_surface : undefined);
  if (primarySurface && isSafeAppPath(primarySurface)) {
    return primarySurface;
  }
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
