import { useEffect, useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  ArrowUpRight,
  Check,
  CheckCircle2,
  ChevronDown,
  Clock,
  FileText,
  FolderKanban,
  Inbox,
  Layers,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  ShieldQuestion,
  SlidersHorizontal,
  Sparkles,
  X,
  Zap
} from "lucide-react";
import {
  ObjectRef,
  SoftCard,
  StatusPill,
  Button,
  StateSurface,
  PageTabs,
  PageTab
} from "@/components/superteam";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import type {
  InboxAction,
  InboxItem,
  InboxItemType,
  InboxListFilters,
  InboxListResponse,
  InboxStatus,
  InboxViewMode
} from "@/lib/api/inbox";
import { demandStatusLabel, missingObjectLabel, relatedRefMetaLabel } from "@/lib/status-labels";
import { cn } from "@/lib/utils";
import type { InboxStreamConnection } from "../inbox-stream-status";
import { formatInboxActionLabel } from "./action-format";
import {
  formatContext,
  formatCurrentNode,
  formatDateTime,
  formatElapsedDuration,
  formatItemType,
  formatRelativeTime,
  formatSourceType,
  formatWaitShort,
  InboxItemList,
  primaryDemandLabel,
  primaryTaskLabel,
  readDemandRefs,
  resolveInboxHref,
  riskLabel,
  riskTone
} from "./inbox-item-list";

export type InboxFilterKey = "status" | "item_type" | "risk_level" | "project_id" | "target_user_id";
export type InboxUuidFilterKey = Extract<InboxFilterKey, "project_id" | "target_user_id">;
export type InboxUuidFilterDrafts = Record<InboxUuidFilterKey, string>;
export type InboxUuidFilterErrors = Partial<Record<InboxUuidFilterKey, string | undefined>>;
export type InboxFilterChangeValue<Key extends InboxFilterKey> = {
  item_type: InboxItemType | "all";
  project_id: string;
  risk_level: string;
  status: InboxStatus | "all";
  target_user_id: string;
}[Key];
type InboxFilterChangeHandler = <Key extends InboxFilterKey>(
  key: Key,
  value: InboxFilterChangeValue<Key>,
) => void;

type InboxShellProps = {
  data?: InboxListResponse;
  /** React Query dataUpdatedAt（ms）；用于「同步于 …」提示。 */
  dataUpdatedAt?: number;
  error: Error | null;
  filters: InboxListFilters;
  isFetching?: boolean;
  isLoading: boolean;
  mutationError: Error | null;
  onAction: (item: InboxItem, action: InboxAction) => void;
  onFilterChange: InboxFilterChangeHandler;
  /** 显式刷新列表——勿与「重置筛选」混淆。 */
  onRefresh: () => void;
  onRetry: () => void;
  onResetFilters: () => void;
  onViewChange: (view: InboxViewMode) => void;
  streamConnection: InboxStreamConnection;
  uuidFilterDrafts: InboxUuidFilterDrafts;
  uuidFilterErrors: InboxUuidFilterErrors;
  view: InboxViewMode;
};

export function InboxShell({
  data,
  dataUpdatedAt,
  error,
  filters,
  isFetching = false,
  isLoading,
  mutationError,
  onAction,
  onFilterChange,
  onRefresh,
  onRetry,
  onResetFilters,
  onViewChange,
  streamConnection,
  uuidFilterDrafts,
  uuidFilterErrors,
  view
}: InboxShellProps) {
  const [selectedItemId, setSelectedItemId] = useState<string | null>(null);
  const hasItems = Boolean(data?.items.length);
  const selectedItem = useMemo(() => {
    return data?.items.find((item) => item.id === selectedItemId) ?? null;
  }, [data?.items, selectedItemId]);

  // 数据真实性修正 #1：第 4 指标卡「等待最久」= 前端基于 items[].created_at 取 max(now - created_at)
  const maxWaitMs = useMemo(() => {
    if (!data?.items.length) return 0;
    const now = Date.now();
    return data.items.reduce((max, item) => {
      const created = new Date(item.created_at).getTime();
      if (Number.isNaN(created)) return max;
      return Math.max(max, now - created);
    }, 0);
  }, [data?.items]);

  useEffect(() => {
    if (selectedItemId && data && !data.items.some((item) => item.id === selectedItemId)) {
      setSelectedItemId(null);
    }
  }, [data, selectedItemId]);

  return (
    <>
      <ShellPageHeader
        title="收件箱"
        subtitle="需要你处理、确认或继续追踪的事项。高风险与阻断项优先处理。"
        icon={<Inbox />}
        iconTone="brand"
      />
      <Main width="wide" fixed className="flex min-h-0 flex-col gap-3 py-4 text-ink">
        {/* 顶部：4 张对等小卡概览 + 视图分段 + 筛选工具条 */}
        <InboxSummaryCards summary={data?.summary} maxWaitMs={maxWaitMs} />
        <InboxToolbar
          dataUpdatedAt={dataUpdatedAt}
          view={view}
          onViewChange={onViewChange}
          filters={filters}
          isFetching={isFetching}
          onFilterChange={onFilterChange}
          onRefresh={onRefresh}
          onResetFilters={onResetFilters}
          streamConnection={streamConnection}
          uuidFilterDrafts={uuidFilterDrafts}
          uuidFilterErrors={uuidFilterErrors}
        />

        {mutationError ? (
          <div
            className="shrink-0 rounded-inner bg-danger-soft p-4 text-sm text-danger"
            role="alert"
          >
            <p className="font-bold">操作未完成</p>
            <p className="mt-1 text-ink-2">{mutationError.message}</p>
          </div>
        ) : null}

        {/* 工作台：三栏在桌面端填满视口，各自内部滚动；移动端整列纵向滚动 */}
        <div className="min-h-0 flex-1 overflow-y-auto xl:overflow-hidden">
          <StateSurface
            isLoading={isLoading && !data}
            isError={Boolean(error && !data)}
            error={error}
            empty={Boolean(data && !hasItems)}
            onRetry={onRetry}
            emptyState={
              <SoftCard>
                <div className="px-6 py-12 text-center text-sm text-ink-2">
                  当前没有需要处理的事项。
                </div>
              </SoftCard>
            }
          >
            {data && hasItems ? (
              <div className="grid min-h-0 gap-3 xl:h-full xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.15fr)_300px]">
                {/* 左栏：紧凑列表 */}
                <InboxItemList
                  items={data.items}
                  onSelect={(item) => setSelectedItemId(item.id)}
                  selectedItemId={selectedItemId}
                />
                {/* 中栏：详情 */}
                <InboxDetailPanel data={data} item={selectedItem} view={view} />
                {/* 右栏：操作面板 */}
                <InboxActionPanel item={selectedItem} onAction={onAction} view={view} />
              </div>
            ) : null}
          </StateSurface>
        </div>
      </Main>
    </>
  );
}

// ---------------------------------------------------------------------------
// 顶部概览：4 张对等小卡（照搬项目管理 KPI 卡：语义色 + 图标圆底 + 顶条 + hover 上浮）
// ---------------------------------------------------------------------------

type InboxSummaryTone = "brand" | "danger" | "warn" | "info" | "mute";

const summaryCardSoftBg: Record<InboxSummaryTone, string> = {
  brand: "bg-brand-soft",
  danger: "bg-danger-soft",
  warn: "bg-warn-soft",
  info: "bg-info-soft",
  mute: "bg-mute-soft"
};

// 与项目管理 KPI 卡一致：语义 text 层做数字与图标色（过 AA），保持色彩搭配
const summaryCardNumText: Record<InboxSummaryTone, string> = {
  brand: "text-brand-deep",
  danger: "text-danger-text",
  warn: "text-warn-text",
  info: "text-info-text",
  mute: "text-mute-text"
};

const summaryCardAccent: Record<InboxSummaryTone, string> = {
  brand: "bg-brand",
  danger: "bg-danger",
  warn: "bg-warn",
  info: "bg-info",
  mute: "bg-line-strong"
};

function InboxSummaryCards({
  summary,
  maxWaitMs
}: {
  summary?: InboxListResponse["summary"];
  maxWaitMs: number;
}) {
  const highRisk = summary?.high_risk_count ?? 0;
  const blocked = summary?.blocked_count ?? 0;
  const cards: Array<{
    icon: typeof Inbox;
    label: string;
    value: React.ReactNode;
    tone: InboxSummaryTone;
  }> = [
    {
      icon: Inbox,
      label: "开放事项",
      value: summary ? summary.open_count : "—",
      tone: "brand"
},
    {
      icon: AlertTriangle,
      label: "高风险",
      value: summary ? highRisk : "—",
      // 语义色只在需要人工介入（>0）时点亮，0 保持灰阶（对齐 DESIGN.md）
      tone: highRisk > 0 ? "danger" : "mute"
},
    {
      icon: ShieldCheck,
      label: "阻断",
      value: summary ? blocked : "—",
      tone: blocked > 0 ? "warn" : "mute"
},
    {
      icon: Clock,
      label: "等待最久",
      value: maxWaitMs > 0 ? formatWaitShort(maxWaitMs) : "—",
      tone: "info"
},
  ];

  return (
    <section
      aria-label="收件箱概览"
      className="grid shrink-0 grid-cols-2 gap-3 xl:grid-cols-4"
    >
      {cards.map((card) => {
        const Icon = card.icon;
        return (
          <SoftCard
            key={card.label}
            className="group relative flex flex-col gap-2 overflow-hidden px-4 py-3.5 transition-all duration-200 ease-out hover:-translate-y-0.5 hover:border-brand/40 hover:shadow-pop"
          >
            <span
              aria-hidden
              className={cn("absolute inset-x-0 top-0 h-0.5", summaryCardAccent[card.tone])}
            />
            <span
              className={cn(
                "flex size-8 items-center justify-center rounded-full transition-transform duration-200 ease-out group-hover:scale-110",
                summaryCardSoftBg[card.tone],
              )}
            >
              <Icon aria-hidden className={cn("size-[15px]", summaryCardNumText[card.tone])} />
            </span>
            <span
              className={cn(
                "text-2xl font-extrabold leading-none tabular-nums",
                summaryCardNumText[card.tone],
              )}
            >
              {card.value}
            </span>
            <p className="text-[11.5px] font-medium leading-none text-ink-3">{card.label}</p>
          </SoftCard>
        );
      })}
    </section>
  );
}

// ---------------------------------------------------------------------------
// 顶部工具条：视图分段 + 紧凑筛选
// ---------------------------------------------------------------------------

type InboxToolbarProps = {
  dataUpdatedAt?: number;
  view: InboxViewMode;
  onViewChange: (view: InboxViewMode) => void;
  filters: InboxListFilters;
  isFetching: boolean;
  onFilterChange: InboxFilterChangeHandler;
  onRefresh: () => void;
  onResetFilters: () => void;
  streamConnection: InboxStreamConnection;
  uuidFilterDrafts: InboxUuidFilterDrafts;
  uuidFilterErrors: InboxUuidFilterErrors;
};

function InboxToolbar({
  dataUpdatedAt,
  view,
  onViewChange,
  filters,
  isFetching,
  onFilterChange,
  onRefresh,
  onResetFilters,
  streamConnection,
  uuidFilterDrafts,
  uuidFilterErrors
}: InboxToolbarProps) {
  return (
    <SoftCard className="flex shrink-0 flex-wrap items-center gap-2 p-3">
      <PageTabs role="tablist" aria-label="收件箱视图">
        <PageTab
          type="button"
          role="tab"
          active={view === "mine"}
          aria-selected={view === "mine"}
          onClick={() => onViewChange("mine")}
        >
          我的待办
        </PageTab>
        <PageTab
          type="button"
          role="tab"
          active={view === "team"}
          aria-selected={view === "team"}
          onClick={() => onViewChange("team")}
        >
          团队待办
        </PageTab>
      </PageTabs>
      <span aria-hidden className="hidden h-6 w-px bg-line sm:block" />
      <InboxFilters
        filters={filters}
        onFilterChange={onFilterChange}
        onReset={onResetFilters}
        uuidFilterDrafts={uuidFilterDrafts}
        uuidFilterErrors={uuidFilterErrors}
      />
      <InboxSyncControls
        dataUpdatedAt={dataUpdatedAt}
        isFetching={isFetching}
        onRefresh={onRefresh}
        streamConnection={streamConnection}
      />
    </SoftCard>
  );
}

type InboxSyncControlsProps = {
  dataUpdatedAt?: number;
  isFetching: boolean;
  onRefresh: () => void;
  streamConnection: InboxStreamConnection;
};

function InboxSyncControls({
  dataUpdatedAt,
  isFetching,
  onRefresh,
  streamConnection
}: InboxSyncControlsProps) {
  const disconnected = streamConnection !== "connected";
  const syncedLabel =
    dataUpdatedAt && dataUpdatedAt > 0
      ? `同步于 ${formatRelativeTime(new Date(dataUpdatedAt).toISOString())}`
      : null;

  return (
    <div className="ml-auto flex shrink-0 items-center gap-2">
      <p
        className={cn(
          "hidden items-center gap-1.5 text-[11.5px] font-medium sm:flex",
          disconnected ? "text-warn-text" : "text-ink-3",
        )}
        role="status"
      >
        <span
          aria-hidden
          className={cn(
            "size-1.5 rounded-full",
            disconnected ? "bg-warn" : "bg-ok",
          )}
        />
        {disconnected ? "同步中断，正在重连…" : (syncedLabel ?? "实时同步")}
      </p>
      <Button
        aria-label="刷新收件箱"
        disabled={isFetching}
        onClick={onRefresh}
        size="sm"
        type="button"
        variant="ghost"
      >
        <RefreshCw className={cn("size-4", isFetching && "animate-spin")} />
        刷新
      </Button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// 中栏：详情面板
// ---------------------------------------------------------------------------

type InboxDetailPanelProps = {
  data: InboxListResponse;
  item: InboxItem | null;
  view: InboxViewMode;
};

function InboxDetailPanel({ data, item, view }: InboxDetailPanelProps) {
  if (!item) {
    return <InboxEmptyDetailPanel data={data} />;
  }

  return (
    <SoftCard className="flex min-h-0 flex-col overflow-hidden xl:h-full">
      {/* 详情头：KI 编号 + 更新时间 kicker + 标题 + pills（固定，不随正文滚动） */}
      <div className="shrink-0 border-b border-line px-5 py-4">
        {/* 数据真实性修正 #2：KI 编号 = item_type + source_id 前 8 位 */}
        <div className="mb-2 flex items-center justify-between gap-3">
          <p className="font-mono text-[11px] font-bold uppercase tracking-wider text-brand-deep">
            {formatKiNumber(item)}
          </p>
          <p className="shrink-0 font-mono text-[11px] text-ink-3">
            更新 {formatRelativeTime(item.last_activity_at)}
          </p>
        </div>
        <h2 className="text-lg font-extrabold leading-tight text-ink">{item.title}</h2>
        <div className="mt-2.5 flex flex-wrap gap-2">
          <StatusPill tone={item.item_type === "approval" ? "info" : "artifact"}>
            {formatItemType(item)}
          </StatusPill>
          {item.risk_level ? (
            <StatusPill tone={riskTone[item.risk_level] ?? "mute"}>
              {riskLabel[item.risk_level] ?? item.risk_level}
            </StatusPill>
          ) : null}
          {/* 状态徽标必须看 item.status——"已处理"过滤下选中的事项不再是待办 */}
          <StatusPill
            tone={item.status === "resolved" ? "ok" : item.status === "cancelled" ? "mute" : view === "mine" ? "warn" : "mute"}
          >
            {item.status === "resolved"
              ? "已处理"
              : item.status === "cancelled"
                ? "已取消"
                : view === "mine"
                  ? "待我处理"
                  : "团队只读"}
          </StatusPill>
        </div>
      </div>

      {/* 正文：内部滚动 */}
      <div className="min-h-0 flex-1 overflow-y-auto">
      {/* 为什么需要你处理 */}
      <section className="border-b border-line px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-ink">为什么需要你处理</h3>
        <dl className="mt-3 grid grid-cols-[5.5rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-[13px]">
          <dt className="font-semibold text-ink-3">关联对象</dt>
          <dd className="min-w-0 font-semibold text-ink">
            <RelatedObjectSummary item={item} />
          </dd>
          <dt className="font-semibold text-ink-3">当前节点</dt>
          <dd className="min-w-0 font-semibold text-ink">{formatCurrentNode(item)}</dd>
          <dt className="font-semibold text-ink-3">发起来源</dt>
          <dd className="min-w-0 font-semibold text-ink">{formatSourceType(item)}</dd>
          <dt className="font-semibold text-ink-3">更新时间</dt>
          <dd className="min-w-0 font-semibold text-ink">{formatDateTime(item.last_activity_at)}</dd>
          {item.status !== "open" ? (
            <>
              <dt className="font-semibold text-ink-3">处理结果</dt>
              <dd className="min-w-0 font-semibold text-ink">{resolvedTimelineTitle(item)}</dd>
              {(() => {
                const res = readResolution(item);
                if (!res) return null;
                const who = (res.resolved_by_name ?? "").trim();
                const channel = (res.channel_label ?? "").trim();
                if (!who && !channel) return null;
                return (
                  <>
                    {who ? (
                      <>
                        <dt className="font-semibold text-ink-3">处理人</dt>
                        <dd className="min-w-0 font-semibold text-ink">{who}</dd>
                      </>
                    ) : null}
                    {channel ? (
                      <>
                        <dt className="font-semibold text-ink-3">处理通道</dt>
                        <dd className="min-w-0 font-semibold text-ink">{channel}</dd>
                      </>
                    ) : null}
                  </>
                );
              })()}
            </>
          ) : null}
          {item.source_task_id ? (
            <>
              <dt className="font-semibold text-ink-3">关联任务</dt>
              <dd className="min-w-0 text-xs font-semibold text-ink-2">
                <ObjectRef kind="task" name={item.source_task_name} id={item.source_task_id} />
              </dd>
            </>
          ) : null}
        </dl>
        {item.summary ? (
          <p className="mt-3 rounded-inner bg-card-soft px-3 py-2 text-[13px] leading-5 text-ink-2">
            {item.summary}
          </p>
        ) : null}
      </section>

      {/* 过程记录 — 数据真实性修正 #3：仅 created_at / last_activity_at 两真实时间点 + 当前状态 */}
      <section className="border-b border-line px-5 py-4">
        <h3 className="flex items-center gap-2 text-[13px] font-extrabold text-ink">
          过程记录
          <span className="font-mono text-[10px] font-semibold text-ink-3">
            {item.status === "open" ? "2 时间点 · 进行中" : `${item.resolved_at ? 3 : 2} 时间点 · 已完结`}
          </span>
        </h3>
        <div className="mt-3 flex flex-col gap-3.5">
          <TimelineItem
            dot={<CheckCircle2 className="size-3" />}
            dotClassName="bg-ok-soft text-ok"
            title="事项已创建"
            description="事项进入收件箱，等待人工处理。"
            timestamp={`${formatDateTime(item.created_at)} · created_at`}
          />
          <TimelineItem
            dot={<CheckCircle2 className="size-3" />}
            dotClassName="bg-ok-soft text-ok"
            title="最近活动"
            description="来源对象更新了上下文快照。"
            timestamp={`${formatRelativeTime(item.last_activity_at)} · last_activity_at`}
          />
          {item.status === "open" ? (
            <TimelineItem
              dot={<Clock className="size-3" />}
              dotClassName="bg-brand-soft text-brand"
              title="等待人类决策"
              description={waitingDecisionDescription(item)}
              timestamp="进行中"
            />
          ) : (
            <TimelineItem
              dot={<CheckCircle2 className="size-3" />}
              dotClassName="bg-ok-soft text-ok"
              title={resolvedTimelineTitle(item)}
              description={resolvedTimelineDescription(item)}
              timestamp={item.resolved_at ? `${formatDateTime(item.resolved_at)} · resolved_at` : "—"}
            />
          )}
        </div>
      </section>

      {/* 关联引用 — 数据真实性修正 #4：source_*_id 跳转入口，不编造 artifacts */}
      <section className="px-5 py-4">
        <h3 className="flex items-center gap-2 text-[13px] font-extrabold text-ink">
          关联引用
          <span className="font-mono text-[10px] font-semibold text-ink-3">
            {countRelatedReferences(item)} 项 · 跳转
          </span>
        </h3>
        <div className="mt-3 grid gap-2">
          {buildRelatedReferences(item).map((ref) => (
            <RelatedReferenceRow key={ref.key} reference={ref} />
          ))}
        </div>
      </section>
      </div>
    </SoftCard>
  );
}

function TimelineItem({
  dot,
  dotClassName,
  title,
  description,
  timestamp
}: {
  dot: React.ReactNode;
  dotClassName: string;
  title: string;
  description: string;
  timestamp: string;
}) {
  return (
    <div className="flex gap-3">
      <span className={cn("mt-0.5 grid size-6 shrink-0 place-items-center rounded-full", dotClassName)}>
        {dot}
      </span>
      <div className="min-w-0">
        <p className="text-[13px] font-bold text-ink">{title}</p>
        <p className="mt-0.5 text-xs leading-5 text-ink-3">{description}</p>
        <p className="mt-0.5 font-mono text-[11px] text-ink-3">{timestamp}</p>
      </div>
    </div>
  );
}

type RelatedReference = {
  key: string;
  icon: React.ReactNode;
  label: string;
  meta: string;
  href?: string;
};

function RelatedObjectSummary({ item }: { item: InboxItem }) {
  const demandLabel = primaryDemandLabel(item);
  const taskLabel = primaryTaskLabel(item);
  const projectName =
    item.source_project_name ??
    readStringFromContext(item.context, ["project_name", "project", "project_title"]);

  if (demandLabel) {
    return (
      <div className="min-w-0 space-y-0.5">
        <p className="truncate">
          {taskLabel && taskLabel !== demandLabel
            ? `${demandLabel}（任务：${taskLabel}）`
            : demandLabel}
        </p>
        {projectName ? (
          <p className="truncate text-xs font-medium text-ink-3">项目 · {projectName}</p>
        ) : null}
      </div>
    );
  }

  return <>{formatContext(item) ?? item.source_id}</>;
}

function buildRelatedReferences(item: InboxItem): RelatedReference[] {
  const refs: RelatedReference[] = [];
  const demandRefs = readDemandRefs(item);
  const seenTaskTitles = new Set<string>();

  for (const demand of demandRefs) {
    refs.push({
      key: `demand-${demand.id ?? demand.title}`,
      icon: <Layers className="size-4 shrink-0 text-ink-3" />,
      // 结项卡会带上已取消/失败的终态需求,不标状态会与已完成的混为一谈。
      label: demand.status
        ? `关联需求 · ${demand.title}（${demandStatusLabel(demand.status)}）`
        : `关联需求 · ${demand.title}`,
      meta: relatedRefMetaLabel(demand.id ? "demand_open" : "demand"),
      // 一单卷宗的 canonical 落点是项目详情需求处所。收件箱事项自带
      // source_project_id 时直接指过去，省掉 /workflows/{id} 那一跳
      // （该跳仍保留：飞书历史卡片与不带项目身份的旧数据还靠它兜底）。
      href: demand.id
        ? item.source_project_id
          ? `/projects/${encodeURIComponent(item.source_project_id)}?demand=${encodeURIComponent(demand.id)}&tab=demands`
          : `/workflows/${encodeURIComponent(demand.id)}`
        : undefined
});
    for (const taskTitle of demand.taskTitles) {
      if (seenTaskTitles.has(taskTitle)) continue;
      seenTaskTitles.add(taskTitle);
      refs.push({
        key: `task-title-${taskTitle}`,
        icon: <FileText className="size-4 shrink-0 text-ink-3" />,
        label: `关联任务 · ${taskTitle}`,
        meta: relatedRefMetaLabel("task"),
        href: item.source_task_id ? resolveInboxHref(item) : undefined
});
    }
  }

  if (item.source_task_id && !seenTaskTitles.has(item.source_task_name ?? item.source_task_id)) {
    refs.push({
      key: "task",
      icon: <FileText className="size-4 shrink-0 text-ink-3" />,
      label: `关联任务 · ${item.source_task_name?.trim() || missingObjectLabel("task", item.source_task_id)}`,
      meta: relatedRefMetaLabel("task_open"),
      href: resolveInboxHref(item)
});
  }

  if (item.source_project_id) {
    refs.push({
      key: "project",
      icon: <FolderKanban className="size-4 shrink-0 text-ink-3" />,
      label: `关联项目 · ${item.source_project_name?.trim() || readStringFromContext(item.context, ["project_name", "project_title"]) || missingObjectLabel("project", item.source_project_id)}`,
      meta: relatedRefMetaLabel("project_open"),
      href: `/projects/${encodeURIComponent(item.source_project_id)}`
});
  }

  if (item.source_approval_request_id) {
    refs.push({
      key: "approval",
      icon: <ShieldQuestion className="size-4 shrink-0 text-ink-3" />,
      label: "关联审批请求",
      meta: relatedRefMetaLabel("approval_open"),
      href: resolveInboxHref(item)
});
  }

  refs.push({
    key: "audit",
    icon: <ShieldCheck className="size-4 shrink-0 text-ink-3" />,
    label: "操作将写入审计日志",
    meta: relatedRefMetaLabel("audit")
});

  return refs;
}

function countRelatedReferences(item: InboxItem): number {
  return buildRelatedReferences(item).length;
}

function RelatedReferenceRow({ reference }: { reference: RelatedReference }) {
  const content = (
    <>
      {reference.icon}
      <span className="min-w-0 flex-1 truncate font-semibold text-ink">{reference.label}</span>
      <span className="shrink-0 font-mono text-[11px] text-ink-3">{reference.meta}</span>
    </>
  );

  if (reference.href) {
    return (
      <Link
        className="flex min-w-0 items-center gap-2.5 rounded-inner border border-line bg-card-soft px-3 py-2 text-[13px] transition-colors hover:border-brand hover:bg-card"
        to={reference.href}
      >
        {content}
      </Link>
    );
  }

  return (
    <div className="flex min-w-0 items-center gap-2.5 rounded-inner border border-line bg-card-soft px-3 py-2 text-[13px]">
      {content}
    </div>
  );
}

// ---------------------------------------------------------------------------
// 右栏：操作面板
// ---------------------------------------------------------------------------

type InboxActionPanelProps = {
  item: InboxItem | null;
  onAction: (item: InboxItem, action: InboxAction) => void;
  view: InboxViewMode;
};

const actionToneVariant: Record<string, "primary" | "outline" | "danger"> = {
  danger: "danger",
  destructive: "danger",
  positive: "primary",
  primary: "primary",
  success: "outline",
  warning: "outline"
};

const actionToneClass: Record<string, string> = {
  primary: "",
  positive: "",
  success: "border-ok text-ok hover:bg-ok-soft",
  warning: "border-warn text-warn hover:bg-warn-soft"
};

function InboxActionPanel({ item, onAction, view }: InboxActionPanelProps) {
  if (!item) {
    return (
      <div className="flex min-h-0 flex-col gap-3 xl:h-full xl:overflow-y-auto">
        <SoftCard className="px-5 py-8 text-center">
          <Sparkles aria-hidden className="mx-auto mb-3 size-8 text-ink-3" />
          <p className="text-sm font-bold text-ink">选择事项后可操作</p>
          <p className="mt-1.5 text-xs text-ink-3">
            从左侧列表选择一条事项，这里会展示已等待时长、可执行动作和快速跳转。
          </p>
        </SoftCard>
      </div>
    );
  }

  const actions = Array.isArray(item.actions) ? item.actions : [];
  const detailHref = resolveInboxHref(item);
  const waitMs = computeWaitMs(item);

  return (
    <div className="flex min-h-0 flex-col gap-3 xl:h-full xl:overflow-y-auto">
      {/* 已等待/处理耗时 — 数据真实性修正 #5:open 用 now-created_at;终态用 resolved_at-created_at */}
      <SoftCard className="overflow-hidden">
        <div className="flex items-center gap-2 border-b border-line bg-card-soft px-4 py-3 text-[13px] font-bold text-ink">
          <Clock aria-hidden className="size-3.5" />
          {item.status === "open" ? "已等待时长" : "处理耗时"}
        </div>
        <div className="px-4 py-3.5">
          <WaitTimeRing waitMs={waitMs} riskLevel={item.risk_level} settled={item.status !== "open"} />
        </div>
      </SoftCard>

      {/* 可执行动作 — 仅 open 事项渲染按钮;终态事项动作已失效 */}
      <SoftCard className="overflow-hidden">
        <div className="flex items-center gap-2 border-b border-line bg-card-soft px-4 py-3 text-[13px] font-bold text-ink">
          <Zap aria-hidden className="size-3.5" />
          可执行动作
        </div>
        <div className="px-4 py-3.5">
          {item.status !== "open" ? (
            <p className="text-[13px] font-semibold text-ink-2">
              {item.status === "resolved"
                ? resolvedActionDisabledMessage(item)
                : "该事项已取消，无需处理。"}
            </p>
          ) : view === "mine" && actions.length > 0 ? (
            <>
              <div className="grid grid-cols-2 gap-2">
                {actions.map((action) => (
                  <Button
                    className={cn("justify-center", actionToneClass[action.tone])}
                    key={action.key}
                    onClick={() => onAction(item, action)}
                    type="button"
                    variant={actionToneVariant[action.tone] ?? "outline"}
                    size="sm"
                  >
                    {formatInboxActionLabel(action)}
                  </Button>
                ))}
              </div>
              <p className="mt-2.5 text-[11px] leading-5 text-ink-3">
                动作将写入审计日志，并推动流程进入下一节点。
              </p>
            </>
          ) : (
            <p className="text-[13px] font-semibold text-ink-2">
              {view === "mine" ? "该事项暂无可执行动作。" : "团队视图仅用于查看上下文。"}
            </p>
          )}
        </div>
      </SoftCard>

      {/* 快速跳转 */}
      <SoftCard className="overflow-hidden">
        <div className="flex items-center gap-2 border-b border-line bg-card-soft px-4 py-3 text-[13px] font-bold text-ink">
          <Layers aria-hidden className="size-3.5" />
          快速跳转
        </div>
        <div className="flex flex-col gap-0.5 px-2 py-1.5">
          {/* F3(§5.4.3): 唯一权威落点 detailHref 来自服务端 primary_surface。原"进入流程
              实例/查看流程编排"两个各自推导的入口已下线,避免同一待办多入口跳不同页。 */}
          <QuickLink to={detailHref} icon={<ArrowUpRight className="size-3.5" />}>
            查看完整详情
          </QuickLink>
          {item.source_task_id ? (
            <QuickLink to={detailHref} icon={<FileText className="size-3.5" />}>
              查看关联任务
            </QuickLink>
          ) : null}
        </div>
      </SoftCard>
    </div>
  );
}

function WaitTimeRing({ waitMs, riskLevel, settled }: { waitMs: number; riskLevel?: string; settled?: boolean }) {
  const clamped = Math.max(0, waitMs);
  const totalHours = clamped / 3600000;
  const ringPercentage = Math.min(100, Math.max(0, (totalHours / 24) * 100));
  // 终态事项没有紧迫度可言,统一降为非紧急配色。
  const isUrgent = !settled && (riskLevel === "blocked" || riskLevel === "high");
  const ringColor = isUrgent ? "var(--danger)" : "var(--warn)";
  const ringSoft = isUrgent ? "var(--danger-soft)" : "var(--warn-soft)";
  const shortLabel = totalHours >= 1 ? `${Math.floor(totalHours)}h` : `${Math.floor(clamped / 60000)}m`;

  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-inner border px-3.5 py-3",
        isUrgent
          ? "border-danger/20 bg-gradient-to-br from-danger-soft to-card"
          : "border-warn/20 bg-gradient-to-br from-warn-soft to-card",
      )}
    >
      <div
        className="relative grid size-12 shrink-0 place-items-center rounded-full text-[13px] font-extrabold"
        style={{
          background: `conic-gradient(${ringColor} ${ringPercentage}%, ${ringSoft} 0)`,
          color: isUrgent ? "var(--danger)" : "var(--warn)"
}}
      >
        <span className="absolute inset-1 rounded-full bg-card" />
        <span className="relative z-10">{shortLabel}</span>
      </div>
      <div className="min-w-0 flex-1">
        <p
          className={cn(
            "text-[11px] font-bold uppercase tracking-wide",
            isUrgent ? "text-danger-text" : "text-warn-text",
          )}
        >
          {settled ? "处理耗时" : "已等待"}
        </p>
        <p className="mt-0.5 text-lg font-extrabold tabular-nums text-ink">
          {formatElapsedDuration(clamped)}
        </p>
        <p className="mt-0.5 text-[11px] text-ink-3">
          {settled ? "created_at 至 resolved_at" : "基于 created_at 计算"}
          {isUrgent ? " · 阻断级建议尽快处理" : ""}
        </p>
      </div>
    </div>
  );
}

function QuickLink({
  to,
  icon,
  children
}: {
  to: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Link
      className="flex items-center gap-2.5 rounded-inner px-3 py-2.5 text-[13px] font-semibold text-ink-2 transition-colors hover:bg-card-soft hover:text-brand-deep"
      to={to}
    >
      <span className="text-ink-3">{icon}</span>
      <span className="flex-1 text-left">{children}</span>
      <ArrowUpRight aria-hidden className="size-3 text-ink-3" />
    </Link>
  );
}

// ---------------------------------------------------------------------------
function waitingDecisionDescription(item: InboxItem): string {
  const labels = (Array.isArray(item.actions) ? item.actions : [])
    .map((action) => formatInboxActionLabel(action).trim())
    .filter(Boolean);
  if (labels.length === 0) {
    return "完成决策后将推动流程进入下一节点。";
  }
  if (labels.length === 1) {
    return `选择「${labels[0]}」后将推动流程进入下一节点。`;
  }
  if (labels.length === 2) {
    return `选择「${labels[0]}」或「${labels[1]}」后将推动流程进入下一节点。`;
  }
  const head = labels.slice(0, -1).map((label) => `「${label}」`).join("、");
  const tail = labels[labels.length - 1];
  return `选择${head}或「${tail}」后将推动流程进入下一节点。`;
}

/** 终态 snapshot：resolve 时写入 context.resolution（who / channel / verb / comment）。 */
type InboxResolutionSnapshot = {
  decision?: string;
  decision_label?: string;
  resolved_by_user_id?: string;
  resolved_by_name?: string;
  channel?: string;
  channel_label?: string;
  comment?: string;
};

function readResolution(item: InboxItem): InboxResolutionSnapshot | null {
  const raw = item.context?.resolution;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
    return null;
  }
  return raw as InboxResolutionSnapshot;
}

function resolutionVerbPhrase(res: InboxResolutionSnapshot | null, status: InboxItem["status"]): string {
  if (status === "cancelled") {
    return "已取消";
  }
  const label = (res?.decision_label ?? "").trim();
  if (!label) {
    return "已处理";
  }
  // decision_label 多为「批准」「驳回」等动词词干；已含「已」时不再叠前缀。
  return label.startsWith("已") ? label : `已${label}`;
}

/** 过程记录终态节点标题：如「已批准」「已驳回」「已取消」。 */
export function resolvedTimelineTitle(item: InboxItem): string {
  return resolutionVerbPhrase(readResolution(item), item.status);
}

/** 过程记录终态说明：谁 · 经何通道 · 做了什么（可选备注）。 */
export function resolvedTimelineDescription(item: InboxItem): string {
  if (item.status === "cancelled") {
    return "该事项已取消，无需再处理。";
  }
  const res = readResolution(item);
  if (!res) {
    return "该事项已完成处理，无需再操作。";
  }
  const who = (res.resolved_by_name ?? "").trim() || "项目成员";
  const channel = (res.channel_label ?? "").trim() || "Console";
  const verb = resolutionVerbPhrase(res, item.status);
  const comment = (res.comment ?? "").trim();
  const base = `${who} 经 ${channel} ${verb}。`;
  return comment ? `${base} 备注：${comment}` : base;
}

/** 右侧动作面板终态禁用文案：谁经哪通道处理过。 */
export function resolvedActionDisabledMessage(item: InboxItem): string {
  const res = readResolution(item);
  if (!res) {
    return "该事项已处理完毕，无需再操作。";
  }
  const who = (res.resolved_by_name ?? "").trim() || "项目成员";
  const channel = (res.channel_label ?? "").trim() || "Console";
  const verb = resolutionVerbPhrase(res, item.status);
  return `已由 ${who} 经 ${channel} ${verb}，无需再操作。`;
}

// 空状态详情面板

// ---------------------------------------------------------------------------

function InboxEmptyDetailPanel({ data }: { data: InboxListResponse }) {
  return (
    <SoftCard className="min-h-0 overflow-y-auto xl:h-full">
      <div className="border-b border-line px-5 py-5">
        <h2 className="text-lg font-extrabold text-ink">选择一条事项查看详情</h2>
        <p className="mt-2 text-[13px] leading-5 text-ink-2">
          左侧列出所有需要你同意、审核、确认或验收的事项。选择任一事项后，这里会展示过程记录、关联引用和可执行动作。
        </p>
      </div>
      <section className="border-b border-line px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-ink">今日待处理摘要</h3>
        <div className="mt-3 grid grid-cols-3 gap-2">
          <MiniSummary label="待我处理" value={data.summary.open_count} />
          <MiniSummary label="高风险" value={data.summary.high_risk_count} tone="danger" />
          <MiniSummary label="阻断" value={data.summary.blocked_count} tone="warn" />
        </div>
      </section>
      <section className="border-b border-line px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-ink">处理顺序建议</h3>
        <ol className="mt-3 space-y-2 text-[13px] text-ink-2">
          <li className="flex gap-2"><span className="font-bold text-ink">1.</span>先处理阻断与高风险审批。</li>
          <li className="flex gap-2"><span className="font-bold text-ink">2.</span>再处理等待最久的事项。</li>
          <li className="flex gap-2"><span className="font-bold text-ink">3.</span>最后处理普通同意或复核事项。</li>
        </ol>
      </section>
      <section className="px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-ink">选择事项后可执行</h3>
        <div className="mt-3 grid gap-2 text-[13px] text-ink-2">
          {["查看完整详情", "查看过程记录", "查看关联引用", "按事项可用动作处理", "跳转到关联流程或项目"].map((label) => (
            <div className="flex items-center gap-2" key={label}>
              <CheckCircle2 className="size-4 text-ok" />
              <span>{label}</span>
            </div>
          ))}
        </div>
      </section>
    </SoftCard>
  );
}

function MiniSummary({
  label,
  tone = "info",
  value
}: {
  label: string;
  tone?: "danger" | "info" | "warn";
  value: number;
}) {
  const valueToneClass = {
    danger: "text-danger",
    info: "text-brand",
    warn: "text-warn"
}[tone];

  return (
    <div className="rounded-inner border border-line bg-card-soft px-3 py-2">
      <p className="text-[11px] font-bold text-ink-2">{label}</p>
      <p className={cn("mt-1 text-2xl font-extrabold tabular-nums", valueToneClass)}>{value}</p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

/** 数据真实性修正 #2：KI 编号 = item_type (大写) + source_id 前 8 位 */
function formatKiNumber(item: InboxItem): string {
  const sourceIdShort = item.source_id.slice(0, 8);
  return `${item.item_type.toUpperCase()} · ${sourceIdShort}`;
}

/** 数据真实性修正 #5：已等待时长 = now - created_at（毫秒），负值钳为 0 */
function computeWaitMs(item: InboxItem): number {
  const created = new Date(item.created_at).getTime();
  if (Number.isNaN(created)) return 0;
  // 终态事项的时长在处理时刻定格(处理耗时),不再随当前时间增长。
  if (item.status !== "open" && item.resolved_at) {
    const resolved = new Date(item.resolved_at).getTime();
    if (!Number.isNaN(resolved)) return Math.max(0, resolved - created);
  }
  return Math.max(0, Date.now() - created);
}

function readStringFromContext(context: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = context[key];
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return undefined;
}

// ---------------------------------------------------------------------------
// 筛选器（保留原有逻辑）
// ---------------------------------------------------------------------------

type InboxFiltersProps = {
  filters: InboxListFilters;
  onFilterChange: InboxFilterChangeHandler;
  onReset: () => void;
  uuidFilterDrafts: InboxUuidFilterDrafts;
  uuidFilterErrors: InboxUuidFilterErrors;
};

type SelectOption<Value extends string> = {
  label: string;
  value: Value;
};

const statusOptions = [
  { label: "开放", value: "open" },
  { label: "已处理", value: "resolved" },
  { label: "已取消", value: "cancelled" },
  { label: "所有", value: "all" },
] satisfies Array<SelectOption<InboxStatus | "all">>;

const itemTypeOptions = [
  { label: "全部类型", value: "all" },
  { label: "审批", value: "approval" },
  { label: "项目决策", value: "project_decision" },
  { label: "团队待删", value: "team_pending_delete" },
  { label: "通道告警", value: "channel_alert" },
  { label: "自动化告警", value: "automation_alert" },
  { label: "编制失效", value: "casting_invalidated" },
] satisfies Array<SelectOption<InboxItemType | "all">>;

const riskOptions = [
  { label: "全部风险", value: "all" },
  { label: "阻断", value: "blocked" },
  { label: "高风险", value: "high" },
  { label: "中风险", value: "medium" },
  { label: "低风险", value: "low" },
] satisfies Array<SelectOption<string>>;

function InboxFilters({
  filters,
  onFilterChange,
  onReset,
  uuidFilterDrafts,
  uuidFilterErrors
}: InboxFiltersProps) {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const hasUuidFilterError = Boolean(
    uuidFilterErrors.project_id || uuidFilterErrors.target_user_id,
  );
  const activeUuidCount = [
    uuidFilterDrafts.project_id.trim(),
    uuidFilterDrafts.target_user_id.trim(),
  ].filter(Boolean).length;

  return (
    <div className="flex flex-1 flex-wrap items-center gap-2">
      <FilterChip
        label="状态"
        options={statusOptions}
        value={filters.status ?? "all"}
        neutralValue="all"
        onValueChange={(value) => onFilterChange("status", value)}
      />
      <FilterChip
        label="事项类型"
        options={itemTypeOptions}
        value={filters.item_type ?? "all"}
        neutralValue="all"
        onValueChange={(value) => onFilterChange("item_type", value)}
      />
      <FilterChip
        label="风险等级"
        options={riskOptions}
        value={filters.risk_level ?? "all"}
        neutralValue="all"
        onValueChange={(value) => onFilterChange("risk_level", value)}
      />
      <MoreFiltersButton
        active={showAdvanced || activeUuidCount > 0}
        count={activeUuidCount}
        onToggle={() => setShowAdvanced((v) => !v)}
      />
      {showAdvanced ? (
        <div className="flex w-full flex-wrap items-center gap-2 border-t border-dashed border-line-strong pt-3">
          <FilterInput
            invalid={Boolean(uuidFilterErrors.project_id)}
            label="项目 ID"
            placeholder="项目 ID"
            value={uuidFilterDrafts.project_id}
            onValueChange={(value) => onFilterChange("project_id", value)}
          />
          <FilterInput
            invalid={Boolean(uuidFilterErrors.target_user_id)}
            label="目标用户 ID"
            placeholder="目标用户 ID"
            value={uuidFilterDrafts.target_user_id}
            onValueChange={(value) => onFilterChange("target_user_id", value)}
          />
          {hasUuidFilterError ? (
            <p className="text-xs font-semibold text-danger" role="alert">
              请输入有效 UUID
            </p>
          ) : null}
        </div>
      ) : null}
      <Button
        className="shrink-0"
        onClick={onReset}
        type="button"
        variant="ghost"
        size="sm"
      >
        <RotateCcw className="size-4" />
        重置
      </Button>
    </div>
  );
}

type FilterChipProps<Value extends string> = {
  label: string;
  neutralValue: Value;
  onValueChange: (value: Value) => void;
  options: ReadonlyArray<SelectOption<Value>>;
  value: Value;
};

function FilterChip<Value extends string>({
  label,
  neutralValue,
  onValueChange,
  options,
  value
}: FilterChipProps<Value>) {
  const isActive = value !== neutralValue;
  const selected = options.find((o) => o.value === value);
  const [open, setOpen] = useState(false);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <div
        className={cn(
          "inline-flex items-center gap-1 rounded-xl py-1.5 pl-3.5 pr-2 text-[13px] font-semibold transition-all duration-200 ease-out",
          isActive
            ? "bg-brand-soft text-brand-deep"
            : "border border-line bg-card text-ink-2 shadow-sm hover:-translate-y-0.5 hover:text-ink hover:shadow-md",
        )}
      >
        <PopoverTrigger asChild>
          <button
            type="button"
            aria-label={label}
            className="inline-flex items-center gap-1.5 outline-none"
          >
            <span>{selected?.label ?? label}</span>
            <ChevronDown aria-hidden className="size-3.5 opacity-50" />
          </button>
        </PopoverTrigger>
        {isActive ? (
          <button
            type="button"
            aria-label="清除"
            className="ml-0.5 inline-flex size-4 items-center justify-center rounded-full bg-black/10 transition-colors hover:bg-black/20"
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              onValueChange(neutralValue);
            }}
          >
            <X aria-hidden className="size-3" />
          </button>
        ) : null}
      </div>
      <PopoverContent align="start" className="min-w-[10rem] p-1.5">
        {options.map((opt) => (
          <button
            key={opt.value}
            type="button"
            className={cn(
              "flex w-full items-center justify-between rounded-lg px-3 py-2 text-[13px] font-medium transition-colors",
              opt.value === value
                ? "font-semibold text-brand-deep"
                : "text-ink-2 hover:bg-card-soft hover:text-ink",
            )}
            onClick={() => {
              onValueChange(opt.value);
              setOpen(false);
            }}
          >
            <span>{opt.label}</span>
            {opt.value === value ? <Check aria-hidden className="size-4" /> : null}
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}

function MoreFiltersButton({
  active,
  count,
  onToggle
}: {
  active: boolean;
  count: number;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-xl px-3.5 py-2 text-[13px] font-semibold transition-all",
        active
          ? "bg-brand-soft text-brand-deep"
          : "border border-dashed border-line-strong bg-transparent text-ink-3 hover:border-ink-3 hover:text-ink-2",
      )}
    >
      <SlidersHorizontal aria-hidden className="size-3.5" />
      更多筛选
      {count > 0 ? (
        <span className="inline-flex h-[17px] min-w-[17px] items-center justify-center rounded-full bg-brand px-1 text-[11px] font-bold text-white">
          {count}
        </span>
      ) : null}
    </button>
  );
}

type FilterInputProps = {
  invalid?: boolean;
  label: string;
  onValueChange: (value: string) => void;
  placeholder: string;
  value: string;
};

function FilterInput({ invalid = false, label, onValueChange, placeholder, value }: FilterInputProps) {
  const inputId = `inbox-filter-${label}`;

  return (
    <Input
      aria-invalid={invalid || undefined}
      aria-label={label}
      id={inputId}
      className="h-9 w-[10.5rem] rounded-xl border-line bg-card-soft text-[13px] text-ink shadow-none placeholder:text-ink-3 aria-invalid:border-danger"
      onChange={(event) => onValueChange(event.target.value)}
      placeholder={placeholder}
      value={value}
    />
  );
}
