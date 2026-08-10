import { useEffect, useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowUpRight,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronsUpDown,
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
  X,
  Zap
} from "lucide-react";
import {
  MasterDetailLayout,
  MetricCard,
  MetricGrid,
  ObjectRef,
  SoftCard,
  StatusPill,
  Button,
  StateSurface,
  PageTabs,
  PageTab,
  UserSearchSelect
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
import { getProject, listProjects, type Project } from "@/lib/api/projects";
import type { UserSummary } from "@/lib/api/auth";
import {
  demandStatusLabel,
  missingObjectLabel,
  relatedRefMetaLabel,
  shortObjectId,
} from "@/lib/status-labels";
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
  InboxProgressBar,
  inboxItemIdentityTitle,
  primaryDemandLabel,
  primaryTaskLabel,
  readDemandRefs,
  readInboxProgress,
  resolveInboxHref,
  riskLabel,
  riskTone
} from "./inbox-item-list";

export type InboxFilterKey =
  | "status"
  | "item_type"
  | "risk_level"
  | "project_id"
  | "target_user_id"
  | "sort";
export type InboxFilterChangeValue<Key extends InboxFilterKey> = {
  item_type: InboxItemType | "all";
  project_id: string;
  risk_level: string;
  sort: "risk" | "oldest";
  status: InboxStatus | "all";
  target_user_id: string;
}[Key];
type InboxFilterChangeHandler = <Key extends InboxFilterKey>(
  key: Key,
  value: InboxFilterChangeValue<Key>,
) => void;

type InboxShellProps = {
  apiBaseUrl: string;
  data?: InboxListResponse;
  /** React Query dataUpdatedAt（ms）；用于「同步于 …」提示。 */
  dataUpdatedAt?: number;
  error: Error | null;
  fetcher?: typeof fetch;
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
  onSelectItem: (itemId: string | null) => void;
  onViewChange: (view: InboxViewMode) => void;
  /** 选中态由父级持有，便于处理后自动前进与 SSE 清空语义分离。 */
  selectedItemId: string | null;
  streamConnection: InboxStreamConnection;
  view: InboxViewMode;
};

export function InboxShell({
  apiBaseUrl,
  data,
  dataUpdatedAt,
  error,
  fetcher,
  filters,
  isFetching = false,
  isLoading,
  mutationError,
  onAction,
  onFilterChange,
  onRefresh,
  onRetry,
  onResetFilters,
  onSelectItem,
  onViewChange,
  selectedItemId,
  streamConnection,
  view
}: InboxShellProps) {
  const hasItems = Boolean(data?.items.length);
  const selectedItem = useMemo(() => {
    return data?.items.find((item) => item.id === selectedItemId) ?? null;
  }, [data?.items, selectedItemId]);

  // 第 4 指标卡「等待最久」= 前端基于当前页 items[].created_at 取 max(now - created_at)。
  // U6（inbox-triage-workbench）：ListInboxItems 改为风险优先后，limit=50 截断集合
  // 从「最近活动 50 条」变成「风险最高 50 条」，该 KPI 静默变为「高风险页内等最久」
  // 而非「全部 open 里等最久」。本批按方案③：暂不处理，仅注释 + CHANGELOG 记录；
  // 后续若要与分页解耦，再把 max wait 并入服务端 summary（方案①）。
  const maxWaitMs = useMemo(() => {
    if (!data?.items.length) return 0;
    const now = Date.now();
    return data.items.reduce((max, item) => {
      const created = new Date(item.created_at).getTime();
      if (Number.isNaN(created)) return max;
      return Math.max(max, now - created);
    }, 0);
  }, [data?.items]);

  // 他人处理（SSE）导致选中项消失 → 清空选中。处理后自动前进只在提交成功回调里生效，
  // 不得与本 effect 互污（§4.3.2）。
  useEffect(() => {
    if (selectedItemId && data && !data.items.some((item) => item.id === selectedItemId)) {
      onSelectItem(null);
    }
  }, [data, onSelectItem, selectedItemId]);

  return (
    <>
      <ShellPageHeader
        title="收件箱"
        subtitle="需要你处理、确认或继续追踪的事项。高风险与阻断项优先处理。"
        icon={<Inbox />}
        iconTone="brand"
      />
      <Main width="wide" fixed className="flex min-h-0 flex-col gap-3 py-4 text-ink">
        <InboxSummaryCards summary={data?.summary} maxWaitMs={maxWaitMs} />
        <InboxToolbar
          apiBaseUrl={apiBaseUrl}
          dataUpdatedAt={dataUpdatedAt}
          fetcher={fetcher}
          view={view}
          onViewChange={onViewChange}
          filters={filters}
          isFetching={isFetching}
          onFilterChange={onFilterChange}
          onRefresh={onRefresh}
          onResetFilters={onResetFilters}
          streamConnection={streamConnection}
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

        {/* 主从：未选中列表独占全宽；选中后详情+动作合一右栏（裁决工作台）。
            外层 overflow-hidden 是承重的：它把 Main fixed 的高度封住，两列才能
            各自内部滚动。改回 overflow-y-auto 会让整个工作台变成一条长列——
            滚到下方选中卡片后，决策按钮会被顶到视口之上（实测 -863px）。 */}
        <div className="min-h-0 flex-1 overflow-hidden">
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
              <MasterDetailLayout
                className="min-h-0"
                fill
                rail="lg"
                detailLabel="事项详情"
                onDetailDismiss={() => onSelectItem(null)}
                master={
                  <InboxItemList
                    items={data.items}
                    onAction={onAction}
                    onClearSelection={() => onSelectItem(null)}
                    onSelect={(item) => onSelectItem(item.id)}
                    selectedItemId={selectedItemId}
                    sort={filters.sort ?? "risk"}
                    view={view}
                  />
                }
                detail={
                  selectedItem ? (
                    <InboxDetailWorkbench
                      item={selectedItem}
                      onAction={onAction}
                      view={view}
                    />
                  ) : undefined
                }
              />
            ) : null}
          </StateSurface>
        </div>
      </Main>
    </>
  );
}

// ---------------------------------------------------------------------------
// 顶部概览：MetricGrid + MetricCard（语义色仅 >0 点亮）
// ---------------------------------------------------------------------------

function InboxSummaryCards({
  summary,
  maxWaitMs
}: {
  summary?: InboxListResponse["summary"];
  maxWaitMs: number;
}) {
  const highRisk = summary?.high_risk_count ?? 0;
  const blocked = summary?.blocked_count ?? 0;

  return (
    <MetricGrid aria-label="收件箱概览" className="shrink-0">
      <MetricCard
        icon={<Inbox />}
        iconTone="brand"
        label="开放事项"
        value={summary ? summary.open_count : "—"}
      />
      <MetricCard
        icon={<AlertTriangle />}
        iconTone={highRisk > 0 ? "danger" : "mute"}
        label="高风险"
        // 语义色只在需要人工介入（>0）时点亮，0 保持灰阶（对齐 DESIGN.md）
        loud={highRisk > 0}
        value={summary ? highRisk : "—"}
      />
      <MetricCard
        icon={<ShieldCheck />}
        iconTone={blocked > 0 ? "warn" : "mute"}
        label="阻断"
        loud={blocked > 0}
        value={summary ? blocked : "—"}
      />
      <MetricCard
        icon={<Clock />}
        iconTone="info"
        label="等待最久"
        value={maxWaitMs > 0 ? formatWaitShort(maxWaitMs) : "—"}
      />
    </MetricGrid>
  );
}

// ---------------------------------------------------------------------------
// 顶部工具条：视图分段 + 紧凑筛选
// ---------------------------------------------------------------------------

type InboxToolbarProps = {
  apiBaseUrl: string;
  dataUpdatedAt?: number;
  fetcher?: typeof fetch;
  view: InboxViewMode;
  onViewChange: (view: InboxViewMode) => void;
  filters: InboxListFilters;
  isFetching: boolean;
  onFilterChange: InboxFilterChangeHandler;
  onRefresh: () => void;
  onResetFilters: () => void;
  streamConnection: InboxStreamConnection;
};

function InboxToolbar({
  apiBaseUrl,
  dataUpdatedAt,
  fetcher,
  view,
  onViewChange,
  filters,
  isFetching,
  onFilterChange,
  onRefresh,
  onResetFilters,
  streamConnection
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
      <span aria-hidden className="hidden h-6 w-px bg-line @md/content:block" />
      <InboxFilters
        apiBaseUrl={apiBaseUrl}
        fetcher={fetcher}
        filters={filters}
        onFilterChange={onFilterChange}
        onReset={onResetFilters}
        view={view}
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
          "hidden items-center gap-1.5 text-[11.5px] font-medium @md/content:flex",
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
// 详情工作台：动作置顶 + 详情正文（裁决工作台口径）
// ---------------------------------------------------------------------------

function InboxDetailWorkbench({
  item,
  onAction,
  view
}: {
  item: InboxItem;
  onAction: (item: InboxItem, action: InboxAction) => void;
  view: InboxViewMode;
}) {
  return (
    <div className="flex min-h-0 flex-col gap-3 @5xl/master-detail:h-full @5xl/master-detail:overflow-y-auto">
      <InboxActionPanel item={item} onAction={onAction} view={view} />
      <InboxDetailPanel item={item} view={view} />
    </div>
  );
}

type InboxDetailPanelProps = {
  item: InboxItem;
  view: InboxViewMode;
};

function InboxDetailPanel({ item, view }: InboxDetailPanelProps) {
  const waitMs = computeWaitMs(item);
  const waitLabel = item.status === "open" ? "已等待" : "处理耗时";

  return (
    <SoftCard className="flex min-h-0 flex-col overflow-hidden">
      {/* 详情头：KI 编号 + 等待时长 meta + 标题 + pills */}
      <div className="shrink-0 border-b border-line px-5 py-4">
        <div className="mb-2 flex items-center justify-between gap-3">
          <p className="font-mono text-[11px] font-bold uppercase tracking-wider text-brand-deep">
            {formatKiNumber(item)}
          </p>
          <p className="shrink-0 font-mono text-[11px] text-ink-3">
            {waitLabel} {formatElapsedDuration(waitMs)} · 更新 {formatRelativeTime(item.last_activity_at)}
          </p>
        </div>
        <h2 className="text-lg font-extrabold leading-tight text-ink">
          {inboxItemIdentityTitle(item)}
        </h2>
        <div className="mt-2.5 flex flex-wrap gap-2">
          <StatusPill tone={item.item_type === "approval" ? "info" : "artifact"}>
            {formatItemType(item)}
          </StatusPill>
          {item.risk_level ? (
            <StatusPill tone={riskTone[item.risk_level] ?? "mute"}>
              {riskLabel[item.risk_level] ?? item.risk_level}
            </StatusPill>
          ) : null}
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
      {/* 为什么需要你处理 — why 首行（从列表迁入）+ 进度条（从列表迁入，非删除） */}
      <section className="border-b border-line px-5 py-4">
        <h3 className="text-[13px] font-extrabold text-ink">为什么需要你处理</h3>
        {item.why?.trim() ? (
          <p className="mt-2 text-[13px] leading-5 text-ink-2">{item.why.trim()}</p>
        ) : null}
        <InboxProgressBar progress={readInboxProgress(item)} />
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

  return <>{formatContext(item) ?? missingObjectLabel("object", item.source_id)}</>;
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
  item: InboxItem;
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
  const actions = Array.isArray(item.actions) ? item.actions : [];
  const detailHref = resolveInboxHref(item);

  return (
    <div className="flex min-h-0 flex-col gap-3">
      {/* 可执行动作置顶 — 裁决工作台：选中即见可决断项 */}
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

      {/* 快速跳转：仅保留服务端 primary_surface 的唯一落点 */}
      <SoftCard className="overflow-hidden">
        <div className="flex items-center gap-2 border-b border-line bg-card-soft px-4 py-3 text-[13px] font-bold text-ink">
          <Layers aria-hidden className="size-3.5" />
          快速跳转
        </div>
        <div className="flex flex-col gap-0.5 px-2 py-1.5">
          <QuickLink to={detailHref} icon={<ArrowUpRight className="size-3.5" />}>
            查看完整详情
          </QuickLink>
        </div>
      </SoftCard>
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
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  filters: InboxListFilters;
  onFilterChange: InboxFilterChangeHandler;
  onReset: () => void;
  view: InboxViewMode;
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

// §4.4.2：默认风险优先；oldest 副文案写明不分组，避免惊吓。
const sortOptions = [
  { label: "风险优先（分诊）", value: "risk" },
  { label: "等待最久（不分组）", value: "oldest" },
] satisfies Array<SelectOption<"risk" | "oldest">>;

function InboxFilters({
  apiBaseUrl,
  fetcher,
  filters,
  onFilterChange,
  onReset,
  view
}: InboxFiltersProps) {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const showTargetUser = view === "team";
  const activeAdvancedCount = [
    filters.project_id?.trim(),
    showTargetUser ? filters.target_user_id?.trim() : undefined,
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
      <FilterChip
        label="排序"
        options={sortOptions}
        value={filters.sort === "oldest" ? "oldest" : "risk"}
        neutralValue="risk"
        onValueChange={(value) => onFilterChange("sort", value)}
      />
      <MoreFiltersButton
        active={showAdvanced || activeAdvancedCount > 0}
        count={activeAdvancedCount}
        onToggle={() => setShowAdvanced((v) => !v)}
      />
      {showAdvanced ? (
        <div className="flex w-full flex-wrap items-center gap-2 border-t border-dashed border-line-strong pt-3">
          <InboxProjectFilter
            apiBaseUrl={apiBaseUrl}
            fetcher={fetcher}
            value={filters.project_id ?? ""}
            onChange={(projectId) => onFilterChange("project_id", projectId)}
          />
          {showTargetUser ? (
            <InboxTargetUserFilter
              apiBaseUrl={apiBaseUrl}
              fetcher={fetcher}
              value={filters.target_user_id ?? ""}
              onChange={(userId) => onFilterChange("target_user_id", userId)}
            />
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

/** 按项目名称搜索筛选（无 UUID 输入；服务端 q 搜索，避免 limit 100 截断）。 */
function InboxProjectFilter({
  apiBaseUrl,
  fetcher,
  value,
  onChange
}: {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  value: string;
  onChange: (projectId: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  // 选中后缓存显示名，避免列表未命中时误显「全部项目」。
  const [selectedName, setSelectedName] = useState<string | undefined>();

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedQuery(query.trim());
    }, 280);
    return () => window.clearTimeout(timer);
  }, [query]);

  useEffect(() => {
    if (!value) setSelectedName(undefined);
  }, [value]);

  const apiOptions = useMemo(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );

  // 仅展开筛选时拉 browse 列表；已选项目靠 selectedName / selectedById 展示，避免进页就 listProjects。
  const browseQuery = useQuery({
    enabled: open,
    queryKey: ["inbox", "filter-projects", apiBaseUrl],
    queryFn: () => listProjects(apiOptions, { limit: 100, offset: 0 }),
    staleTime: 5 * 60 * 1000,
  });

  const searchEnabled = debouncedQuery.length > 0;
  const searchQuery = useQuery({
    enabled: open && searchEnabled,
    placeholderData: keepPreviousData,
    queryKey: ["inbox", "filter-projects-search", apiBaseUrl, debouncedQuery],
    queryFn: () =>
      listProjects(apiOptions, { limit: 50, offset: 0, q: debouncedQuery }),
    staleTime: 30 * 1000,
  });

  const list: Project[] = searchEnabled
    ? (searchQuery.data ?? [])
    : (browseQuery.data ?? []);
  const selectedFromList =
    list.find((project) => project.id === value) ??
    (browseQuery.data ?? []).find((project) => project.id === value);
  // 仅当列表与本地缓存都未解析到名称时才按 id 回查，避免每次筛选多打一枪。
  const needsSelectedById = Boolean(value) && !selectedFromList && !selectedName;
  const selectedByIdQuery = useQuery({
    enabled: needsSelectedById,
    queryKey: ["inbox", "filter-project", apiBaseUrl, value],
    queryFn: () => getProject(apiOptions, value),
    staleTime: 5 * 60 * 1000,
    retry: false,
  });

  const selected = selectedFromList ?? selectedByIdQuery.data;
  const triggerLabel = value
    ? selected?.name ||
      selectedName ||
      (selectedByIdQuery.isError
        ? missingObjectLabel("project", value)
        : selectedByIdQuery.isFetching
          ? `项目 (${shortObjectId(value)})`
          : missingObjectLabel("project", value))
    : "全部项目";
  // isLoading 在 keepPreviousData 下为 false；用 isFetching 表示搜索在飞，并弱提示旧结果。
  const listLoading = searchEnabled
    ? searchQuery.isFetching && list.length === 0
    : browseQuery.isLoading;
  const listStale = searchEnabled && searchQuery.isFetching && list.length > 0;

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) {
          setQuery("");
          setDebouncedQuery("");
        }
      }}
    >
      <PopoverTrigger asChild>
        <button
          aria-label="筛选项目"
          className={cn(
            "inline-flex max-w-[16rem] items-center gap-1.5 rounded-xl border px-3 py-1.5 text-[13px] font-semibold transition-all",
            value
              ? "border-brand/30 bg-brand-soft text-brand-deep"
              : "border-line bg-card text-ink-2 hover:text-ink",
          )}
          type="button"
        >
          <FolderKanban aria-hidden className="size-3.5 shrink-0" />
          <span className="min-w-0 truncate">{triggerLabel}</span>
          <ChevronsUpDown aria-hidden className="size-3.5 shrink-0 opacity-60" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-2">
        <Input
          aria-label="搜索项目"
          className="mb-2 h-8"
          onChange={(event) => setQuery(event.target.value)}
          placeholder="搜索项目名称…"
          value={query}
        />
        <div
          aria-busy={listStale || listLoading || undefined}
          className={cn(
            "flex max-h-56 flex-col gap-0.5 overflow-y-auto",
            listStale && "opacity-70",
          )}
          role="listbox"
          aria-label="项目列表"
        >
          <button
            className="rounded-inner px-2 py-1.5 text-left text-[13px] font-semibold text-ink-2 hover:bg-card-soft"
            onClick={() => {
              setSelectedName(undefined);
              onChange("");
              setOpen(false);
            }}
            role="option"
            type="button"
          >
            全部项目
          </button>
          {listLoading ? (
            <p className="px-2 py-1.5 text-xs text-ink-3">加载项目中…</p>
          ) : list.length === 0 ? (
            <p className="px-2 py-1.5 text-xs text-ink-3">无匹配项目</p>
          ) : (
            list.map((project: Project) => (
              <button
                aria-selected={value === project.id}
                className={cn(
                  "rounded-inner px-2 py-1.5 text-left text-[13px] font-semibold hover:bg-card-soft",
                  value === project.id ? "bg-brand-soft text-brand-deep" : "text-ink",
                )}
                key={project.id}
                onClick={() => {
                  setSelectedName(project.name);
                  onChange(project.id);
                  setOpen(false);
                }}
                role="option"
                type="button"
              >
                <span className="block truncate">{project.name}</span>
              </button>
            ))
          )}
          {listStale ? (
            <p className="px-2 py-1 text-[11px] text-ink-3">更新匹配结果中…</p>
          ) : null}
        </div>
      </PopoverContent>
    </Popover>
  );
}

/** 团队视图：按成员名筛选目标用户。 */
function InboxTargetUserFilter({
  apiBaseUrl,
  fetcher,
  value,
  onChange
}: {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  value: string;
  onChange: (userId: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<UserSummary | undefined>();

  useEffect(() => {
    if (!value) setSelected(undefined);
  }, [value]);

  // value 有 id 但尚未 onSelect 缓存时：短 id 明示已筛选，禁止假「全部用户」。
  const triggerLabel =
    selected?.display_name ||
    selected?.username ||
    (value ? `用户 (${shortObjectId(value)})` : "全部用户");

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          aria-label="筛选目标用户"
          className={cn(
            "inline-flex max-w-[14rem] items-center gap-1.5 rounded-xl border px-3 py-1.5 text-[13px] font-semibold transition-all",
            value
              ? "border-brand/30 bg-brand-soft text-brand-deep"
              : "border-line bg-card text-ink-2 hover:text-ink",
          )}
          type="button"
        >
          <span className="min-w-0 truncate">{triggerLabel}</span>
          <ChevronsUpDown aria-hidden className="size-3.5 shrink-0 opacity-60" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 p-3">
        {value ? (
          <Button
            className="mb-2 w-full"
            onClick={() => {
              onChange("");
              setSelected(undefined);
            }}
            size="sm"
            type="button"
            variant="ghost"
          >
            清除用户筛选
          </Button>
        ) : null}
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          fetcher={fetcher}
          inputLabel="搜索目标用户"
          onSelect={(user) => {
            setSelected(user);
            onChange(user.id);
            setOpen(false);
          }}
          placeholder="搜索用户名或显示名…"
          value={selected}
        />
      </PopoverContent>
    </Popover>
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
