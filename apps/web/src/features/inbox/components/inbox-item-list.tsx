import { useEffect, useMemo, useRef, type KeyboardEvent } from "react";
import { ArrowUpRight, Clock } from "lucide-react";
import { Link } from "@tanstack/react-router";
import {
  Button,
  StatusPill,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import type { InboxAction, InboxItem, InboxViewMode } from "@/lib/api/inbox";
import { formatDateTime, formatRelativeTime } from "@/lib/format-time";
import { decisionTypeLabel, humanTaskKindLabel, missingObjectLabel } from "@/lib/status-labels";
import { cn } from "@/lib/utils";
import { formatInboxActionLabel } from "./action-format";

export { formatDateTime, formatRelativeTime };

type InboxItemListProps = {
  items: InboxItem[];
  onSelect: (item: InboxItem) => void;
  /** Esc（无弹窗时）清除选中。 */
  onClearSelection?: () => void;
  selectedItemId: string | null;
  /** 行内主 CTA：打开决策弹窗（不提交），仅 mine + open + 高风险。 */
  onAction?: (item: InboxItem, action: InboxAction) => void;
  /** 列表排序档，影响分组/优先区（§4.4）。 */
  sort?: "risk" | "oldest" | string;
  /**
   * 决策弹窗关闭时由父级递增：把焦点还给选中行。
   * Radix 的 return-focus-to-trigger 在此不适用——弹窗由行的 Enter 或行内 CTA
   * 程序化打开，没有可归还的 trigger 元素，关闭后焦点会落到 body。
   */
  refocusToken?: number;
  view?: InboxViewMode;
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
  project_workspace_pending_delete: "工作区待删",
  project_workspace_provision_pending: "工作区待供给",
  channel_alert: "通道告警",
  automation_alert: "自动化告警",
  casting_invalidated: "编制失效",
  // 运行恢复链已随「运行必须归属项目」spec 退役；键保留给历史 resolved 事项渲染。
  digital_employee_run_recovery: "运行恢复"
};

const sourceTypeLabel: Record<string, string> = {
  approval_request: "审批请求",
  project_decision_request: "项目决策请求",
  team_pending_delete: "团队待删",
  project_workspace_pending_delete: "工作区待删",
  project_workspace_provision_pending: "工作区待供给",
  feishu_channel: "飞书通道",
  automation_rule: "自动化规则",
  project_casting: "项目编制",
  digital_employee_run: "数字员工运行"
};

type InboxSection = { key: string; label: string; items: InboxItem[] };

// §6.1 收件箱按人类待办类型分组:计划确认 / 执行放行 / 下游放行 / 验收签署 / 结项确认 /
// 异常处理。类型取自服务端 HumanTask kind(§4.2);其余(planning_gap / task_failure_recovery /
// 其它 project_task_* 以及审批 / 运行恢复 / 团队待删)归入"异常处理"。
// 组内保持上游顺序、不再二次排序。排序契约由服务端 ListInboxItems 承担：
// risk_level 优先（blocked→high→medium→low；NULL/未登记值最后），同级 last_activity_at DESC。
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

export type InboxGroupOptions = {
  /**
   * 排序档。risk（默认）= 优先区 + 领域分组；
   * oldest = 平列表（无优先区、无领域分组），顺序由服务端承担。
   */
  sort?: "risk" | "oldest" | string;
};

/** Bucket inbox items into the §6.1 human-task categories, preserving upstream order within each. */
export function groupInboxItems(
  items: InboxItem[],
  options: InboxGroupOptions = {},
): InboxSection[] {
  const sort = options.sort === "oldest" ? "oldest" : "risk";

  // 时间档：摊平列表，不渲染优先区与领域分组（§4.4.2）。
  // label 留空：排序档已由列表头「N 项 · 按等待时长」表达，再出一个同名同计数的
  // 分区表头就是相隔 40px 的重复——正是本 spec（D3/D4）要消除的那类冗余。
  // 渲染侧据空 label 跳过表头，见 InboxItemList。
  if (sort === "oldest") {
    if (items.length === 0) return [];
    return [{ key: "flat", label: "", items }];
  }

  // 优先处理区：open + blocked/high，从领域分组中抽出不重复（§4.4.1 / U1 / U12）。
  const priorityItems: InboxItem[] = [];
  const remainder: InboxItem[] = [];
  for (const item of items) {
    const risk = item.risk_level;
    if (
      item.status === "open" &&
      (risk === "blocked" || risk === "high")
    ) {
      priorityItems.push(item);
    } else {
      remainder.push(item);
    }
  }

  const buckets = new Map<string, InboxItem[]>();
  for (const item of remainder) {
    const key = inboxCategoryKey(item);
    const bucket = buckets.get(key) ?? [];
    bucket.push(item);
    buckets.set(key, bucket);
  }
  const sections: InboxSection[] = [];
  if (priorityItems.length > 0) {
    sections.push({ key: "priority", label: "优先处理", items: priorityItems });
  }
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

/** 渲染序列（跨分组连续），供键盘导航与处理后自动前进共用。 */
export function flatInboxRenderOrder(
  items: InboxItem[],
  options: InboxGroupOptions = {},
): InboxItem[] {
  return groupInboxItems(items, options).flatMap((section) => section.items);
}

/**
 * 紧凑两行列表（inbox-triage-workbench §4.2）：
 * 第 1 行 = 风险 accent + 身份标题 + kind pill + 风险 pill + 相对时间 +（高风险）CTA
 * 第 2 行 = summary 单行 clamp
 * why / 进度条 / item_type pill 已迁出（why+进度→详情栏；item_type 恒同已删）。
 */
export function InboxItemList({
  items,
  onSelect,
  onClearSelection,
  selectedItemId,
  onAction,
  sort = "risk",
  refocusToken = 0,
  view = "mine",
}: InboxItemListProps) {
  const highRiskCount = items.filter(
    (item) => item.risk_level === "blocked" || item.risk_level === "high",
  ).length;

  const groupOptions = useMemo(() => ({ sort }), [sort]);
  const sections = useMemo(() => groupInboxItems(items, groupOptions), [items, groupOptions]);
  const flatItems = useMemo(
    () => flatInboxRenderOrder(items, groupOptions),
    [items, groupOptions],
  );
  const rowRefs = useRef(new Map<string, HTMLDivElement>());

  // roving tabindex 焦点目标：选中行优先，否则首行。
  const focusItemId = selectedItemId && flatItems.some((item) => item.id === selectedItemId)
    ? selectedItemId
    : (flatItems[0]?.id ?? null);

  useEffect(() => {
    if (!selectedItemId) return;
    const el = rowRefs.current.get(selectedItemId);
    if (!el || document.activeElement === el) return;
    const active = document.activeElement;
    // 弹窗打开时不抢焦点；否则在选中变化时把焦点还给行（含提交成功自动前进后
    // focus 落在 body/已卸载节点的情况，保证键盘队列可继续）。
    if (active instanceof HTMLElement && active.closest('[role="dialog"]')) {
      return;
    }
    el.focus({ preventScroll: false });
  }, [selectedItemId]);

  // 弹窗关闭后把焦点还给选中行。上面那个 effect 只在 selectedItemId 变化时跑，
  // 而「Enter 开弹窗 → Esc 取消」不改选中，于是焦点停在 body：再按 ↓/↑ 全无反应，
  // 键盘队列就此断掉（实测 Esc 后需 Tab 穿过整个侧栏才能回到列表）。
  // 逐帧重试：onOpenChange(false) 触发时 Radix 可能仍在收敛/归还焦点，
  // 单帧不足以稳定命中；命中或帧数耗尽即停。
  useEffect(() => {
    if (!refocusToken || !selectedItemId) return;
    let frame = 0;
    let raf = 0;
    const tick = () => {
      const el = rowRefs.current.get(selectedItemId);
      const active = document.activeElement;
      if (el && active !== el && !(active instanceof HTMLElement && active.closest('[role="dialog"]'))) {
        el.focus({ preventScroll: false });
        return;
      }
      if (el && active === el) return;
      if (frame++ < 6) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [refocusToken, selectedItemId]);

  function moveSelection(delta: number) {
    if (flatItems.length === 0) return;
    const currentIndex = Math.max(
      0,
      flatItems.findIndex((item) => item.id === (selectedItemId ?? focusItemId)),
    );
    const nextIndex = Math.min(
      flatItems.length - 1,
      Math.max(0, currentIndex + delta),
    );
    const next = flatItems[nextIndex];
    if (!next) return;
    onSelect(next);
    requestAnimationFrame(() => {
      rowRefs.current.get(next.id)?.focus({ preventScroll: false });
    });
  }

  function jumpSelection(to: "start" | "end") {
    if (flatItems.length === 0) return;
    const next = to === "start" ? flatItems[0] : flatItems[flatItems.length - 1];
    onSelect(next);
    requestAnimationFrame(() => {
      rowRefs.current.get(next.id)?.focus({ preventScroll: false });
    });
  }

  function handleRowKeyDown(event: KeyboardEvent, item: InboxItem) {
    const key = event.key;
    if (key === "ArrowDown" || key === "j" || key === "J") {
      event.preventDefault();
      moveSelection(1);
      return;
    }
    if (key === "ArrowUp" || key === "k" || key === "K") {
      event.preventDefault();
      moveSelection(-1);
      return;
    }
    if (key === "Home") {
      event.preventDefault();
      jumpSelection("start");
      return;
    }
    if (key === "End") {
      event.preventDefault();
      jumpSelection("end");
      return;
    }
    if (key === "Escape") {
      event.preventDefault();
      onClearSelection?.();
      return;
    }
    if (key === "Enter") {
      event.preventDefault();
      onSelect(item);
      const isHighRisk = item.risk_level === "blocked" || item.risk_level === "high";
      const primaryAction =
        view === "mine" && item.status === "open" && isHighRisk && onAction
          ? firstPositiveAction(item)
          : undefined;
      if (primaryAction) {
        onAction?.(item, primaryAction);
      }
      return;
    }
    if (key === " ") {
      event.preventDefault();
      onSelect(item);
    }
  }

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
          const identityTitle = inboxItemIdentityTitle(item);
          const summaryText = item.summary?.trim() ?? "";
          // kind 行内 pill：与 2026-08-07 §3.3「meta 不再回退 kind」不冲突——
          // §3.3 管的是 meta 行与分组表头恒等重复；此处 kind 是行内 pill，
          // 位置与角色不同（表头=这一段属哪类；pill=这一条属哪类）。
          // 阶段 4 优先区（跨 kind）与时间档（不分组）里后者不可省。
          const kindLabel =
            typeof item.kind === "string" && item.kind
              ? humanTaskKindLabel(item.kind)
              : undefined;
          const primaryAction =
            view === "mine" &&
            item.status === "open" &&
            isHighRisk &&
            onAction
              ? firstPositiveAction(item)
              : undefined;
          const isRovingFocus = item.id === focusItemId;

          return (
            <div
              key={item.id}
              ref={(node) => {
                if (node) rowRefs.current.set(item.id, node);
                else rowRefs.current.delete(item.id);
              }}
              role="button"
              tabIndex={isRovingFocus ? 0 : -1}
              aria-label={`打开事项：${identityTitle}`}
              aria-selected={isSelected}
              data-inbox-item-id={item.id}
              className={cn(
                // overflow-hidden：防止绝对子元素（查看上下文）在窄列下画到邻行/详情区
                "group relative flex cursor-pointer items-start gap-2 overflow-hidden border-b border-line px-4 py-2 transition-colors",
                "hover:bg-card-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand/60",
                accentShadow,
                isSelected && "bg-brand-soft",
              )}
              onClick={() => onSelect(item)}
              onKeyDown={(event) => handleRowKeyDown(event, item)}
            >
              <div className="min-w-0 flex-1">
                {/* §4.2.2 第 1 行：身份 + kind + 风险 + 时间 +（高风险）CTA；全部 in-flow，互不叠压 */}
                <div className="flex min-w-0 items-center gap-1.5">
                  <span
                    className={cn(
                      "min-w-0 flex-1 truncate text-left text-sm font-bold text-ink",
                      isSelected && "text-brand-deep",
                    )}
                    title={identityTitle}
                  >
                    {identityTitle}
                  </span>
                  {kindLabel ? (
                    <StatusPill
                      tone="mute"
                      showDot={false}
                      className="shrink-0 px-2 py-0.5 text-[11px]"
                    >
                      {kindLabel}
                    </StatusPill>
                  ) : null}
                  {item.risk_level ? (
                    <StatusPill
                      tone={riskTone[item.risk_level] ?? "mute"}
                      showDot={false}
                      className="shrink-0 px-2 py-0.5 text-[11px]"
                    >
                      {riskLabel[item.risk_level] ?? item.risk_level}
                    </StatusPill>
                  ) : null}
                  <span className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap text-[11px] tabular-nums text-ink-3">
                    <Clock aria-hidden className="size-3" />
                    {formatRelativeTime(item.last_activity_at)}
                  </span>
                  {primaryAction ? (
                    <Button
                      aria-label={`行内决策：${formatInboxActionLabel(primaryAction)}`}
                      className="h-6 max-w-[7.5rem] shrink-0 truncate px-2 text-[11px]"
                      size="sm"
                      tabIndex={-1}
                      type="button"
                      variant="primary"
                      onClick={(event) => {
                        event.stopPropagation();
                        onSelect(item);
                        onAction?.(item, primaryAction);
                      }}
                    >
                      {formatInboxActionLabel(primaryAction)}
                    </Button>
                  ) : null}
                </div>
                {/*
                  第 2 行：summary + U3「查看上下文」。
                  链接绝对定位在本行容器右下（非整行 top-end），避免与第 1 行 CTA/时间叠字；
                  summary 预留 pe 使 hover 显形时不盖住文案末尾。
                */}
                <div className="relative mt-0.5 min-h-5">
                  {summaryText ? (
                    <p className="line-clamp-1 max-w-full break-words pe-[5.5rem] text-xs leading-5 text-ink-2">
                      {summaryText}
                    </p>
                  ) : null}
                  <Link
                    tabIndex={-1}
                    className={cn(
                      "absolute end-0 top-0 inline-flex items-center gap-1 text-[11px] font-semibold text-brand-deep",
                      "opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100",
                    )}
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

  // h-full 不带容器变体：master 在宽/窄容器下都是 in-flow 唯一主列
  // （详情窄容器走 Sheet），限高一律由 MasterDetailLayout fill 透传。
  return (
    <WorkSurface className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-center justify-between gap-2 border-b border-line bg-card-soft px-5 py-3.5">
        <span className="text-sm font-bold text-ink">待处理事项</span>
        <div className="flex items-center gap-2">
          {highRiskCount > 0 ? (
            <StatusPill tone="danger" showDot={false} className="px-2 py-0.5 text-[11px]">
              {highRiskCount} 高风险
            </StatusPill>
          ) : null}
          <span className="font-mono text-xs text-ink-3">
            {items.length} 项 · {sort === "oldest" ? "按等待时长" : "风险优先 · 按类型分组"}
          </span>
        </div>
      </div>
      <div
        className="min-h-0 flex-1 overflow-y-auto"
        data-inbox-list
        aria-label="待处理事项列表"
      >
        {sections.map((section) => (
          <section key={section.key}>
            {/* §6.1 领域分组表头(计划确认/执行放行/下游放行/验收签署/结项确认/异常处理);组内保持上游关注度排序。
                空 label = 摊平的时间档单分区，不出表头（避免与列表头同名同计数重复）。 */}
            {section.label ? (
              <div className="flex items-center justify-between gap-2 border-b border-line bg-card-soft/80 px-4 py-1.5 text-[11px] font-bold text-ink-2">
                <span>{section.label}</span>
                <span className="font-mono text-ink-3">{section.items.length}</span>
              </div>
            ) : null}
            {section.items.map(renderRow)}
          </section>
        ))}
      </div>
    </WorkSurface>
  );
}

/** 行内主 CTA：actions[] 中首个 positive/primary 动作。 */
function firstPositiveAction(item: InboxItem): InboxAction | undefined {
  const actions = Array.isArray(item.actions) ? item.actions : [];
  return actions.find((action) => action.tone === "positive" || action.tone === "primary");
}

/**
 * 行标题位身份（inbox-triage-workbench §4.2.1）：按 layer 组合，不用 item.title 常量。
 * - demand → 需求名；task → 任务名；project → 项目名；无 layer（告警类）→ title。
 * 缺失走 missingObjectLabel，不回落 kind 常量、不裸 UUID。
 */
export function inboxItemIdentityTitle(item: InboxItem): string {
  const layer = typeof item.layer === "string" ? item.layer : "";
  switch (layer) {
    case "demand": {
      const demandLabel = primaryDemandLabel(item);
      if (demandLabel) return demandLabel;
      const demandId =
        readContextText(item.context ?? {}, ["primary_demand_id", "demand_id"]) ??
        item.source_id;
      return missingObjectLabel("demand", demandId);
    }
    case "task": {
      const taskName =
        item.source_task_name?.trim() ||
        primaryTaskLabel(item) ||
        readContextText(item.context ?? {}, ["task_title"]);
      if (taskName) return taskName;
      return missingObjectLabel("task", item.source_task_id ?? item.source_id);
    }
    case "project": {
      const projectName =
        item.source_project_name?.trim() ||
        readContextText(item.context ?? {}, ["project_name", "project", "project_title"]);
      if (projectName) return projectName;
      return missingObjectLabel("project", item.source_project_id ?? item.source_id);
    }
    default:
      return item.title;
  }
}

/**
 * 详情栏说明段落：summary 与 why 并列，trim 后相同只保留一段
 * （服务端未登记 kind 会把 summary 回填到 why，见 humanTaskWhy）。
 * 列表侧已不再渲染 why（§4.2.2 密度压缩）。
 */
export function inboxListDescriptionParagraphs(
  item: Pick<InboxItem, "summary" | "why">,
): string[] {
  const summaryText = item.summary?.trim() ?? "";
  const whyText = item.why?.trim() ?? "";
  const paragraphs: string[] = [];
  if (summaryText) {
    paragraphs.push(summaryText);
  }
  if (whyText && whyText !== summaryText) {
    paragraphs.push(whyText);
  }
  return paragraphs;
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
  /** 终态需求含 completed / failed / cancelled，展示处必须区分,不得一律当已完成。 */
  status?: string;
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
      const rawTitle =
        typeof record.title === "string" && record.title.trim()
          ? record.title.trim()
          : "";
      // 标题缺失或等于 id（服务端/历史载荷把 UUID 当 title）时走 D3 兜底，禁止裸 UUID。
      let title = "";
      if (rawTitle && rawTitle !== id) {
        title = rawTitle;
      } else if (id) {
        title = missingObjectLabel("demand", id);
      }
      if (!title) continue;
      const taskTitles: string[] = [];
      if (Array.isArray(record.task_titles)) {
        for (const taskTitle of record.task_titles) {
          if (typeof taskTitle === "string" && taskTitle.trim()) {
            taskTitles.push(taskTitle.trim());
          }
        }
      }
      const status =
        typeof record.status === "string" && record.status.trim()
          ? record.status.trim()
          : undefined;
      refs.push({ id, title, status, taskTitles });
    }
    if (refs.length > 0) {
      // 服务端 primary_demand_id 是"闭合本项目的那条需求"的唯一口径(结项卡上必为
      // 已完成需求);refs[0] 是 headline 与 refs 消费点的默认取值,必须与它一致,
      // 否则标题会挂到刚被取消的需求上。
      const primaryId = readContextText(context, ["primary_demand_id"]);
      const primaryIndex = primaryId ? refs.findIndex((ref) => ref.id === primaryId) : -1;
      if (primaryIndex > 0) {
        const [primary] = refs.splice(primaryIndex, 1);
        refs.unshift(primary);
      }
      return refs;
    }
  }

  const demandId =
    readContextText(context, ["primary_demand_id", "demand_id"]) ?? undefined;
  const demandTitle = readContextText(context, ["demand_title"]);
  if (demandTitle || demandId) {
    let title: string;
    if (demandTitle && demandTitle !== demandId) {
      title = demandTitle;
    } else if (demandId) {
      title = missingObjectLabel("demand", demandId);
    } else {
      title = demandTitle ?? "";
    }
    return [
      {
        id: demandId,
        title,
        taskTitles: [],
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

  // 服务端读时补名优先于 context 快照；名缺失走 D3 missingObjectLabel，禁止全 UUID 主文本。
  const projectName =
    item.source_project_name?.trim() ||
    readContextText(item.context, ["project_name", "project", "project_title"]);
  const sourceName =
    readContextText(item.context, ["source_title", "approval_title", "task_title"]) ??
    (item.source_task_name?.trim() || undefined);

  if (projectName && sourceName) {
    return `${projectName} / ${sourceName}`;
  }
  if (projectName || sourceName) {
    return projectName ?? sourceName;
  }
  if (item.source_project_id) {
    return missingObjectLabel("project", item.source_project_id);
  }
  return undefined;
}

export function formatItemType(item: InboxItem) {
  return itemTypeLabel[item.item_type] ?? item.item_type;
}

export function formatSourceType(item: InboxItem) {
  return sourceTypeLabel[item.source_type] ?? item.source_type;
}

/**
 * 只读真实工作流节点字段。列表 meta 用此函数——无值时不渲染节点段，
 * 避免与分组表头（同 kind）或类型 pill 重复。
 */
export function formatRealCurrentNode(item: InboxItem): string | undefined {
  return readContextText(item.context, [
    "current_node",
    "node_title",
    "workflow_node",
    "stage",
  ]);
}

/**
 * 详情面板「当前节点」：真实节点优先，再回退 kind / decision_type / itemType。
 * 详情无分组表头，kind 回退有价值。
 */
export function formatCurrentNode(item: InboxItem) {
  const node = formatRealCurrentNode(item);
  if (node) {
    return node;
  }
  // §6.1/§12 + F3：禁止裸英文技术枚举。规范化 kind 优先,再退 decision_type,最后 itemType。
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
