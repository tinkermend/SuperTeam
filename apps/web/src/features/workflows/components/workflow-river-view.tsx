import { useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Activity, AlertTriangle, CheckCircle2, Clock3, GitBranch, Timer } from "lucide-react";
import {
  SoftCard,
  StatusPill,
  V3Chip,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  WorkSurface,
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import type { WorkflowInstanceSummary } from "@/lib/api/projects";
import { workflowStatusLabel, workflowStatusTone } from "../workflow-status";

type WorkflowRiverViewProps = {
  instances: WorkflowInstanceSummary[];
  isError: boolean;
  isLoading: boolean;
};

type RiverCategory = "attention" | "active" | "done";

type RiverFilter = "all" | RiverCategory;

const filterChips: Array<{ label: string; value: RiverFilter }> = [
  { label: "全部", value: "all" },
  { label: "需要介入", value: "attention" },
  { label: "进行中", value: "active" },
  { label: "已完成", value: "done" },
];

const riverGroups: Array<{
  categories: RiverCategory[];
  key: string;
  label: string;
  tone: "danger" | "brand" | "ok";
}> = [
  { categories: ["attention"], key: "attention", label: "需要介入", tone: "danger" },
  { categories: ["active"], key: "active", label: "进行中", tone: "brand" },
  { categories: ["done"], key: "done", label: "已完成", tone: "ok" },
];

/** 河道分段桶：按执行顺序 已完成→运行→等待人工→阻断→待执行。 */
type RiverBucket = {
  key: "done" | "run" | "wait" | "block" | "pend";
  count: number;
  label: string;
};

const MS_MIN = 60_000;
const MS_HOUR = 60 * MS_MIN;
const FALLBACK_MAX_MS = 3 * MS_HOUR;

/** 单个分组默认展示条数，超过则折叠并显示"查看更多"。 */
const GROUP_COLLAPSE_THRESHOLD = 5;

export function WorkflowRiverView({
  instances,
  isError,
  isLoading,
}: WorkflowRiverViewProps) {
  const [filter, setFilter] = useState<RiverFilter>("all");
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({});

  const toggleGroup = (key: string) =>
    setExpandedGroups((prev) => ({ ...prev, [key]: !prev[key] }));

  if (isError && instances.length === 0) {
    return (
      <SoftCard className="p-5">
        <V3ErrorState
          description="暂时无法读取流程编排入口，请稍后重试。"
          title="流程实例加载失败"
        />
      </SoftCard>
    );
  }

  if (isLoading && instances.length === 0) {
    return (
      <SoftCard>
        <V3LoadingState label="正在加载流程实例" />
      </SoftCard>
    );
  }

  if (instances.length === 0) {
    return (
      <SoftCard>
        <V3EmptyState
          description="有需求进入协调线程后，会在这里显示全局流程状态。"
          icon={<GitBranch />}
          title="暂无可见流程实例"
        />
      </SoftCard>
    );
  }

  const maxMs = Math.max(
    FALLBACK_MAX_MS,
    ...instances.map((instance) => instanceDurationMs(instance)),
  );
  const metrics = riverMetrics(instances);
  const visibleInstances =
    filter === "all"
      ? instances
      : instances.filter((instance) => riverCategory(instance) === filter);
  const groups = riverGroups
    .map((group) => ({
      ...group,
      instances: sortRiverInstances(
        visibleInstances.filter((instance) =>
          group.categories.includes(riverCategory(instance)),
        ),
      ),
    }))
    .filter((group) => group.instances.length > 0);

  const collapsibleGroups = groups.filter(
    (group) => group.instances.length > GROUP_COLLAPSE_THRESHOLD,
  );
  const anyCollapsible = collapsibleGroups.length > 0;
  const allExpanded =
    anyCollapsible && collapsibleGroups.every((group) => expandedGroups[group.key]);
  const toggleAllGroups = () => {
    const nextAllExpanded = !allExpanded;
    setExpandedGroups((prev) => {
      const next = { ...prev };
      for (const group of collapsibleGroups) next[group.key] = nextAllExpanded;
      return next;
    });
  };

  return (
    <div className="flex min-w-0 flex-col gap-5">
      {/* 指标带 */}
      <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        <V3MetricCard
          icon={<Activity />}
          iconTone="brand"
          label="运行中"
          meta="含规划中"
          value={metrics.active}
        />
        <V3MetricCard
          icon={<Timer />}
          iconTone="warn"
          label="等待人工"
          meta={metrics.attentionWaiting > 0 ? "需负责人确认" : "暂无"}
          value={metrics.waiting}
        />
        <V3MetricCard
          icon={<AlertTriangle />}
          iconTone="danger"
          label="阻断 / 失败"
          loud={metrics.blocked > 0}
          meta={metrics.blocked > 0 ? `P0 ${metrics.p0}` : "暂无"}
          value={metrics.blocked}
        />
        <V3MetricCard
          icon={<CheckCircle2 />}
          iconTone="ok"
          label="已完成"
          meta="本视图范围"
          value={metrics.done}
        />
        <V3MetricCard
          icon={<AlertTriangle />}
          iconTone="danger"
          label="SLA 超时"
          loud={metrics.slaBreached > 0}
          meta={metrics.slaBreached > 0 ? "需优先处理" : "全部在控"}
          value={metrics.slaBreached}
        />
        <V3MetricCard
          icon={<Clock3 />}
          iconTone="mute"
          label="最久已持续"
          meta="最长未完成实例"
          value={formatDuration(maxMs)}
        />
      </section>

      {/* 河道主体 */}
      <WorkSurface>
        {/* 顶部：标题 + 筛选 */}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-v3-line px-4 py-3">
          <h2 className="text-[15px] font-bold text-v3-ink">流程实例 · 时间河道</h2>
          {isLoading ? <StatusPill tone="mute">同步中</StatusPill> : null}
          {isError ? <StatusPill tone="danger">刷新失败</StatusPill> : null}
          <span className="ml-auto text-[11px] text-v3-ink-3">
            河长=已持续时长 · 段宽=节点数比例 · 段色=状态
          </span>
        </div>

        {/* 筛选 */}
        <div className="flex flex-wrap items-center gap-1.5 border-b border-v3-line bg-v3-card-soft px-4 py-2.5">
          {filterChips.map((chip) => (
            <V3Chip
              active={filter === chip.value}
              count={
                chip.value === "all"
                  ? instances.length
                  : instances.filter(
                      (instance) => riverCategory(instance) === chip.value,
                    ).length
              }
              key={chip.value}
              onClick={() => setFilter(chip.value)}
            >
              {chip.label}
            </V3Chip>
          ))}
          <span className="ml-auto text-[11px] tabular-nums text-v3-ink-3">
            显示 {visibleInstances.length} / {instances.length}
          </span>
          {anyCollapsible ? (
            <button
              className="rounded-md border border-v3-line bg-v3-card px-2.5 py-1 text-[11px] font-semibold text-v3-brand-deep transition-colors hover:bg-v3-brand-soft"
              onClick={toggleAllGroups}
              type="button"
            >
              {allExpanded ? "全部收起" : "全部展开"}
            </button>
          ) : null}
        </div>

        {/* 时间刻度轴 */}
        <RiverTimeAxis maxMs={maxMs} />

        {/* 河道分组 */}
        {groups.length === 0 ? (
          <V3EmptyState
            className="py-10"
            description="切换上方筛选查看其他状态的实例。"
            title="该筛选下暂无实例"
          />
        ) : (
          <div aria-label="流程实例列表" role="list">
            {groups.map((group) => {
              const expanded = expandedGroups[group.key];
              const overThreshold =
                group.instances.length > GROUP_COLLAPSE_THRESHOLD;
              const visibleGroupInstances =
                expanded || !overThreshold
                  ? group.instances
                  : group.instances.slice(0, GROUP_COLLAPSE_THRESHOLD);
              return (
                <section key={group.key}>
                  <header className="flex items-center gap-2 border-b border-v3-line bg-v3-card-soft px-4 py-1.5 text-xs font-semibold text-v3-ink-2">
                    <span
                      aria-hidden
                      className={cn("size-1.5 rounded-full", groupDotClass[group.tone])}
                    />
                    {group.label}
                    <span className="tabular-nums text-v3-ink-3">
                      {group.instances.length}
                    </span>
                  </header>
                  {visibleGroupInstances.map((instance) => (
                    <RiverLane
                      instance={instance}
                      key={instance.demand_id}
                      maxMs={maxMs}
                    />
                  ))}
                  {overThreshold ? (
                    <button
                      className="flex w-full items-center justify-center gap-1.5 border-b border-v3-line bg-v3-card-soft py-2 text-xs font-semibold text-v3-brand-deep transition-colors hover:bg-v3-brand-soft"
                      onClick={() => toggleGroup(group.key)}
                      type="button"
                    >
                      {expanded
                        ? "收起"
                        : `查看全部 ${group.instances.length} 条（已折叠 ${group.instances.length - GROUP_COLLAPSE_THRESHOLD} 条）`}
                      <span aria-hidden>{expanded ? "▴" : "▾"}</span>
                    </button>
                  ) : null}
                </section>
              );
            })}
          </div>
        )}
      </WorkSurface>
    </div>
  );
}

const groupDotClass = {
  danger: "bg-v3-danger",
  brand: "bg-v3-brand",
  ok: "bg-v3-ok",
} as const;

/* ------------------------------------------------------------------ */
/* 时间刻度轴                                                          */
/* ------------------------------------------------------------------ */

function RiverTimeAxis({ maxMs }: { maxMs: number }) {
  const ticks = [0, 0.25, 0.5, 0.75].map((ratio) => ({
    label: ratio === 0 ? "开始" : formatDuration(maxMs * ratio),
    left: `${ratio * 100}%`,
  }));
  return (
    <div className="flex items-center border-b border-v3-line bg-v3-card-inner px-4 py-2.5">
      <div className="w-[210px] shrink-0 text-[11px] font-semibold uppercase tracking-wide text-v3-ink-3">
        已持续时长 →
      </div>
      <div className="relative h-6 flex-1">
        {ticks.map((tick, index) => (
          <span
            className="absolute bottom-0 flex -translate-x-1/2 flex-col items-center"
            key={index}
            style={{ left: tick.left }}
          >
            <span className="whitespace-nowrap font-mono text-[10.5px] font-semibold text-v3-ink-3">
              {tick.label}
            </span>
            <span className="mt-0.5 h-2 w-px bg-v3-line-strong" />
          </span>
        ))}
        <span className="absolute bottom-0 flex -translate-x-1/2 flex-col items-center text-v3-ink-4" style={{ left: "100%" }}>
          <span className="whitespace-nowrap font-mono text-[10.5px] font-semibold">→ 开放</span>
          <span className="mt-0.5 h-2 w-px bg-v3-line-strong" />
        </span>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* 单条河道                                                            */
/* ------------------------------------------------------------------ */

function RiverLane({
  instance,
  maxMs,
}: {
  instance: WorkflowInstanceSummary;
  maxMs: number;
}) {
  const navigate = useNavigate();
  const category = riverCategory(instance);
  const duration = instanceDurationMs(instance);
  const trackPct = Math.max((duration / maxMs) * 100, 1.5); // 最小可见宽度
  const isClosed = category === "done";
  const buckets = riverBuckets(instance);
  const nowPct = riverNowPct(instance);
  const sla = riverSlaTag(instance);
  const event = riverEventSummary(instance);
  const summary = riverBlockerSummary(instance);

  return (
    <div
      aria-label={instance.title}
      className={cn(
        "flex cursor-pointer items-stretch border-b border-v3-line transition-colors last:border-b-0 hover:bg-v3-card-inner",
      )}
      onClick={() =>
        void navigate({
          params: { demandId: instance.demand_id },
          to: "/workflows/$demandId",
        })
      }
      role="listitem"
    >
      {/* 左侧标签 */}
      <div className="flex w-[210px] shrink-0 flex-col justify-center gap-1 border-r border-v3-line px-4 py-3">
        <div className="flex flex-wrap items-center gap-1.5">
          <StatusPill tone={workflowStatusTone(instance.status)}>
            {workflowStatusLabel(instance.status)}
          </StatusPill>
          {instance.priority ? (
            <PriorityTag value={instance.priority.value} label={instance.priority.label} />
          ) : null}
          {instance.risk ? <RiskTag level={instance.risk.level} label={instance.risk.label} /> : null}
        </div>
        <Link
          className="block min-w-0 truncate text-[13px] font-bold text-v3-ink outline-none hover:text-v3-brand-deep focus-visible:ring-2 focus-visible:ring-v3-brand/60"
          onClick={(event) => event.stopPropagation()}
          params={{ demandId: instance.demand_id }}
          to="/workflows/$demandId"
        >
          {instance.title}
        </Link>
        <span className="truncate font-mono text-[11px] text-v3-ink-3">
          {instance.project_name}
        </span>
        <span className="text-[10.5px] text-v3-ink-3">
          提交 <span className="font-semibold text-v3-ink-2">{instance.submitted_by_display_name || "—"}</span>
        </span>
      </div>

      {/* 河道主体 */}
      <div className="flex min-w-0 flex-1 flex-col gap-1.5 px-4 py-2.5">
        {/* 顶栏：时长 + 创建时间 + SLA */}
        <div className="flex items-center gap-2 font-mono text-[10.5px] text-v3-ink-3">
          <span className="font-semibold text-v3-ink-2">
            {isClosed ? `运行 ${formatDuration(duration)} · 已结束` : `已持续 ${formatDuration(duration)}`}
          </span>
          <span className="text-v3-ink-4">·</span>
          <span>创建 {formatTime(instance.created_at)}</span>
          {sla ? (
            <span
              className={cn(
                "ml-auto inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[10px] font-bold",
                sla.className,
              )}
            >
              {sla.text}
            </span>
          ) : null}
        </div>

        {/* 河道 */}
        <div className="relative flex h-6 items-center">
          <div
            className="relative flex h-5 items-stretch overflow-visible rounded-md border border-v3-line bg-v3-card-soft"
            style={{ width: `${trackPct}%` }}
          >
            {buckets.map((bucket) => (
              <RiverSegment bucket={bucket} key={bucket.key} />
            ))}
            {/* NOW 标记 */}
            {!isClosed ? (
              <span
                aria-hidden
                className="absolute top-1/2 z-10 h-7 w-0.5 -translate-y-1/2 rounded bg-v3-brand"
                style={{ left: `${nowPct}%` }}
              >
                <span className="absolute -top-3.5 left-1/2 -translate-x-1/2 whitespace-nowrap rounded bg-v3-brand px-1 py-px font-mono text-[8.5px] font-bold text-white shadow-sm">
                  当前
                </span>
              </span>
            ) : null}
            {/* 完成标记 */}
            {isClosed ? (
              <span className="absolute top-1/2 z-10 flex size-4 -translate-y-1/2 translate-x-1/2 items-center justify-center rounded-full bg-v3-ok text-[10px] font-bold text-white shadow ring-2 ring-v3-ok-soft" style={{ left: "100%" }}>
                ✓
              </span>
            ) : null}
            {/* 人工门禁 */}
            {instance.progress.waiting_human_nodes > 0 ? (
              <RiverGate
                kind="human"
                leftPct={riverGateLeftPct(instance, "wait")}
              />
            ) : null}
            {/* 阻断门禁 */}
            {instance.current_blocker ? (
              <RiverGate
                kind="block"
                leftPct={riverGateLeftPct(instance, "block")}
              />
            ) : null}
          </div>
          {/* 未来开放段 */}
          {!isClosed ? (
            <div
              aria-hidden
              className="ml-0.5 h-5 shrink-0 rounded-r-md border border-l-0 border-dashed border-v3-line-strong"
              style={{
                width: "8%",
                minWidth: "18px",
                backgroundImage:
                  "repeating-linear-gradient(135deg, transparent, transparent 4px, var(--v3-line) 4px, var(--v3-line) 8px)",
              }}
            />
          ) : null}
        </div>

        {/* 底栏：计数 + 事件 */}
        <div className="flex items-center gap-2.5 text-[10.5px]">
          <span className="font-mono font-semibold tabular-nums text-v3-ink-2">
            <span className="text-v3-ink">{instance.progress.completed_nodes}</span>
            <span className="text-v3-ink-3">/{instance.progress.total_nodes}</span>
            {instance.progress.running_nodes > 0 ? (
              <span className="ml-1 text-v3-brand">· 运行 {instance.progress.running_nodes}</span>
            ) : null}
            {instance.progress.waiting_human_nodes > 0 ? (
              <span className="ml-1 text-v3-warn">· 人工 {instance.progress.waiting_human_nodes}</span>
            ) : null}
            {instance.progress.blocked_nodes > 0 ? (
              <span className="ml-1 text-v3-danger">· 阻断 {instance.progress.blocked_nodes}</span>
            ) : null}
          </span>
          {summary ? (
            <span className="flex min-w-0 flex-1 items-center gap-1.5 text-v3-danger">
              <span aria-hidden className="size-1.5 shrink-0 rounded-full bg-v3-danger" />
              <span className="truncate">{summary}</span>
              {event.ts ? <span className="ml-auto shrink-0 font-mono text-v3-ink-4">{event.ts}</span> : null}
            </span>
          ) : event.text ? (
            <span className="flex min-w-0 flex-1 items-center gap-1.5 text-v3-ink-2">
              <span aria-hidden className="size-1.5 shrink-0 rounded-full bg-v3-mute" />
              <span className="truncate">{event.text}</span>
              {event.ts ? <span className="ml-auto shrink-0 font-mono text-v3-ink-4">{event.ts}</span> : null}
            </span>
          ) : null}
        </div>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* 河道分段                                                            */
/* ------------------------------------------------------------------ */

function RiverSegment({ bucket }: { bucket: RiverBucket }) {
  const showText = bucket.count > 0;
  return (
    <div
      aria-label={`${bucket.label} ${bucket.count}`}
      className={cn(
        "flex items-center justify-center overflow-hidden font-mono text-[10px] font-bold text-white",
        bucket.key === "pend" &&
          "border border-dashed border-v3-line-strong bg-v3-card-soft text-v3-ink-3",
        bucket.key === "done" && "bg-v3-ok",
        bucket.key === "run" && "bg-v3-brand",
        bucket.key === "wait" && "bg-v3-warn",
        bucket.key === "block" && "bg-v3-danger",
      )}
      style={{ flexGrow: bucket.count, flexBasis: 0, minWidth: bucket.count > 0 ? 4 : 0 }}
      title={`${bucket.label} ${bucket.count}`}
    >
      {showText ? <span className="px-1">{bucket.count}</span> : null}
    </div>
  );
}

function RiverGate({
  kind,
  leftPct,
}: {
  kind: "human" | "block";
  leftPct: number;
}) {
  const color = kind === "human" ? "var(--v3-warn)" : "var(--v3-danger)";
  const soft = kind === "human" ? "var(--v3-warn-soft)" : "var(--v3-danger-soft)";
  const label = kind === "human" ? "人" : "!";
  return (
    <span
      aria-hidden
      className="absolute top-1/2 z-20 h-8 w-0.5 -translate-y-1/2"
      style={{ left: `${leftPct}%`, background: color }}
    >
      <span
        className="absolute top-1/2 left-1/2 size-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 bg-v3-card"
        style={{ borderColor: color, backgroundColor: soft }}
      />
      <span
        className="absolute -bottom-3.5 left-1/2 -translate-x-1/2 rounded border border-v3-line bg-v3-card px-1 text-[9px] font-bold"
        style={{ color }}
      >
        {label}
      </span>
    </span>
  );
}

/* ------------------------------------------------------------------ */
/* 标签：优先级 / 风险 / SLA                                           */
/* ------------------------------------------------------------------ */

function PriorityTag({ value, label }: { value: string; label: string }) {
  const normalized = value.toLowerCase();
  const tone = ["critical", "p0", "p1", "urgent"].includes(normalized)
    ? "bg-v3-danger-soft text-v3-danger-text"
    : ["high", "p2"].includes(normalized)
      ? "bg-v3-warn-soft text-v3-warn-text"
      : "bg-v3-mute-soft text-v3-mute-text";
  return (
    <span className={cn("rounded border border-v3-line px-1.5 py-px font-mono text-[9.5px] font-bold", tone)}>
      {label}
    </span>
  );
}

function RiskTag({ level, label }: { level: string; label: string }) {
  const normalized = level.toLowerCase();
  const tone = ["critical", "high", "severe"].includes(normalized)
    ? "bg-v3-danger-soft text-v3-danger-text"
    : ["medium", "moderate"].includes(normalized)
      ? "bg-v3-warn-soft text-v3-warn-text"
      : null;
  if (!tone) return null;
  return (
    <span className={cn("rounded border border-v3-line px-1.5 py-px font-mono text-[9.5px] font-bold", tone)}>
      {label}
    </span>
  );
}

/* ------------------------------------------------------------------ */
/* 计算辅助                                                            */
/* ------------------------------------------------------------------ */

function instanceDurationMs(instance: WorkflowInstanceSummary): number {
  const start = new Date(instance.created_at).getTime();
  if (Number.isNaN(start)) return 0;
  const isClosed =
    instance.status === "completed" ||
    instance.status === "failed" ||
    instance.status === "cancelled";
  const end = isClosed
    ? new Date(instance.updated_at).getTime()
    : Date.now();
  if (Number.isNaN(end)) return 0;
  return Math.max(end - start, 0);
}

function riverCategory(instance: WorkflowInstanceSummary): RiverCategory {
  if (
    instance.progress.blocked_nodes > 0 ||
    instance.status === "failed" ||
    instance.status === "cancelled" ||
    instance.status === "waiting_human"
  ) {
    return "attention";
  }
  if (instance.status === "running" || instance.status === "planning") {
    return "active";
  }
  return "done";
}

function sortRiverInstances(instances: WorkflowInstanceSummary[]) {
  return [...instances].sort((a, b) => {
    const aAtt = riverCategory(a) === "attention" ? 0 : 1;
    const bAtt = riverCategory(b) === "attention" ? 0 : 1;
    if (aAtt !== bAtt) return aAtt - bAtt;
    return instanceDurationMs(b) - instanceDurationMs(a);
  });
}

function riverBuckets(instance: WorkflowInstanceSummary): RiverBucket[] {
  const p = instance.progress;
  const known =
    p.completed_nodes + p.running_nodes + p.waiting_human_nodes + p.blocked_nodes;
  const pending = Math.max(p.total_nodes - known, 0);
  const buckets: RiverBucket[] = [
    { key: "done", count: p.completed_nodes, label: "已完成" },
    { key: "run", count: p.running_nodes, label: "运行中" },
    { key: "wait", count: p.waiting_human_nodes, label: "等待人工" },
    { key: "block", count: p.blocked_nodes, label: "阻断" },
    { key: "pend", count: pending, label: "待执行" },
  ];
  return buckets.filter((bucket) => bucket.count > 0);
}

/** NOW 位置 = 已处理节点 / total，相对河道宽度。 */
function riverNowPct(instance: WorkflowInstanceSummary): number {
  const p = instance.progress;
  if (p.total_nodes === 0) return 0;
  const processed =
    p.completed_nodes + p.running_nodes + p.waiting_human_nodes + p.blocked_nodes;
  return Math.min((processed / p.total_nodes) * 100, 100);
}

/** 门禁位置 = 对应段中点，相对河道宽度。 */
function riverGateLeftPct(
  instance: WorkflowInstanceSummary,
  kind: "wait" | "block",
): number {
  const p = instance.progress;
  if (p.total_nodes === 0) return 0;
  const before =
    p.completed_nodes + p.running_nodes + (kind === "block" ? p.waiting_human_nodes : 0);
  const segCount = kind === "wait" ? p.waiting_human_nodes : p.blocked_nodes;
  const center = before + segCount / 2;
  return Math.min((center / p.total_nodes) * 100, 100);
}

function riverMetrics(instances: WorkflowInstanceSummary[]) {
  let active = 0;
  let waiting = 0;
  let blocked = 0;
  let done = 0;
  let attentionWaiting = 0;
  let slaBreached = 0;
  let p0 = 0;
  for (const instance of instances) {
    const category = riverCategory(instance);
    if (category === "active") active += 1;
    if (instance.status === "waiting_human") {
      waiting += 1;
      attentionWaiting += 1;
    }
    if (
      instance.progress.blocked_nodes > 0 ||
      instance.status === "failed" ||
      instance.status === "cancelled"
    ) {
      blocked += 1;
    }
    if (instance.status === "completed") done += 1;
    if (instance.sla?.breached) slaBreached += 1;
    if (
      instance.priority &&
      ["critical", "p0", "p1", "urgent"].includes(
        instance.priority.value.toLowerCase(),
      )
    ) {
      p0 += 1;
    }
  }
  return { active, waiting, blocked, done, attentionWaiting, slaBreached, p0 };
}

function riverSlaTag(instance: WorkflowInstanceSummary): { text: string; className: string } | null {
  const sla = instance.sla;
  if (!sla) return null;
  const text = sla.breached ? `⚠ ${sla.label}` : sla.label;
  let className: string;
  if (sla.breached) {
    className = "border-v3-danger/30 bg-v3-danger-soft text-v3-danger-text";
  } else if (
    sla.remaining_seconds !== undefined &&
    sla.remaining_seconds !== null &&
    sla.remaining_seconds / 60 <= 30
  ) {
    className = "border-v3-warn/30 bg-v3-warn-soft text-v3-warn-text";
  } else {
    className = "border-v3-ok/25 bg-v3-ok-soft text-v3-ok-text";
  }
  return { text, className };
}

function riverBlockerSummary(instance: WorkflowInstanceSummary): string | null {
  if (instance.current_blocker?.title) {
    return `阻塞：${instance.current_blocker.title}`;
  }
  return null;
}

function riverEventSummary(instance: WorkflowInstanceSummary): { text: string | null; ts: string | null } {
  if (instance.recent_event?.summary) {
    return {
      text: `最新事件：${instance.recent_event.summary}`,
      ts: instance.recent_event.occurred_at ? formatRelativeTime(instance.recent_event.occurred_at) : null,
    };
  }
  if (instance.status_reason) {
    return { text: instance.status_reason, ts: null };
  }
  return { text: null, ts: null };
}

/* ------------------------------------------------------------------ */
/* 格式化                                                              */
/* ------------------------------------------------------------------ */

export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "—";
  const totalMin = Math.round(ms / MS_MIN);
  if (totalMin < 1) return "<1分钟";
  if (totalMin < 60) return `${totalMin}分钟`;
  const hours = Math.floor(totalMin / 60);
  const mins = totalMin % 60;
  if (hours < 24) return mins > 0 ? `${hours}小时${String(mins).padStart(2, "0")}分` : `${hours}小时`;
  const days = Math.floor(hours / 24);
  return `${days}天${hours % 24}小时`;
}

function formatRelativeTime(iso: string): string {
  const ts = new Date(iso).getTime();
  if (Number.isNaN(ts)) return "";
  const diff = Date.now() - ts;
  if (diff < MS_MIN) return "刚刚";
  if (diff < MS_HOUR) return `${Math.round(diff / MS_MIN)}分钟前`;
  if (diff < 24 * MS_HOUR) return `${Math.round(diff / MS_HOUR)}小时前`;
  return `${Math.round(diff / (24 * MS_HOUR))}天前`;
}

function formatTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
