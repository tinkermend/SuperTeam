import { useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Clock3, GitBranch } from "lucide-react";
import {
  SoftCard,
  StatusPill,
  Chip,
  EmptyState,
  ErrorState,
  LoadingState,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import type { WorkflowInstanceSummary } from "@/lib/api/projects";
import { workflowStatusLabel, workflowStatusTone } from "../workflow-status";

type WorkflowEntranceProps = {
  instances: WorkflowInstanceSummary[];
  isError: boolean;
  isLoading: boolean;
};

type WorkflowCategory = "active" | "blocked" | "done" | "other" | "waiting";
type WorkflowFilter = "all" | Exclude<WorkflowCategory, "other">;

const filterChips: Array<{ label: string; value: WorkflowFilter }> = [
  { label: "全部", value: "all" },
  { label: "进行中", value: "active" },
  { label: "等待人工", value: "waiting" },
  { label: "阻断", value: "blocked" },
  { label: "已完成", value: "done" },
];

/** 分诊分组：需要介入（阻断优先）→ 进行中 → 已完成 → 其他。 */
const runGroups: Array<{
  categories: WorkflowCategory[];
  key: string;
  label: string;
  tone: "danger" | "mute" | "ok" | "warn";
}> = [
  { categories: ["blocked", "waiting"], key: "attention", label: "需要介入", tone: "warn" },
  { categories: ["active"], key: "active", label: "进行中", tone: "mute" },
  { categories: ["done"], key: "done", label: "已完成", tone: "ok" },
  { categories: ["other"], key: "other", label: "其他", tone: "mute" },
];

export function WorkflowEntrance({
  instances,
  isError,
  isLoading
}: WorkflowEntranceProps) {
  const [filter, setFilter] = useState<WorkflowFilter>("all");

  if (isError && instances.length === 0) {
    return (
      <SoftCard className="p-5">
        <ErrorState
          description="暂时无法读取流程编排入口，请稍后重试。"
          title="流程实例加载失败"
        />
      </SoftCard>
    );
  }

  if (isLoading && instances.length === 0) {
    return (
      <SoftCard>
        <LoadingState label="正在加载流程实例" />
      </SoftCard>
    );
  }

  if (instances.length === 0) {
    return (
      <SoftCard>
        <EmptyState
          description="有需求进入协调线程后，会在这里显示全局流程状态。"
          icon={<GitBranch />}
          title="暂无可见流程实例"
        />
      </SoftCard>
    );
  }

  const counts = workflowCategoryCounts(instances);
  const visibleInstances =
    filter === "all"
      ? instances
      : instances.filter((instance) => workflowCategory(instance) === filter);
  const groups = runGroups
    .map((group) => ({
      ...group,
      instances: sortGroupInstances(
        visibleInstances.filter((instance) =>
          group.categories.includes(workflowCategory(instance)),
        ),
      )
}))
    .filter((group) => group.instances.length > 0);

  return (
    <WorkSurface>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-line px-4 py-3">
        <h2 className="text-[15px] font-bold text-ink">流程实例</h2>
        {isLoading ? <StatusPill tone="mute">同步中</StatusPill> : null}
        {isError ? <StatusPill tone="danger">刷新失败</StatusPill> : null}
        <div className="ml-auto flex flex-wrap gap-1.5">
          {filterChips.map((chip) => (
            <Chip
              active={filter === chip.value}
              className="px-2.5 py-1 text-xs"
              count={chip.value === "all" ? instances.length : counts[chip.value]}
              key={chip.value}
              onClick={() => setFilter(chip.value)}
            >
              {chip.label}
            </Chip>
          ))}
        </div>
      </div>
      {groups.length === 0 ? (
        <EmptyState
          className="py-10"
          description="切换上方筛选查看其他状态的实例。"
          title="该筛选下暂无实例"
        />
      ) : (
        <div aria-label="流程实例列表" role="list">
          {groups.map((group) => (
            <section key={group.key}>
              <header className="flex items-center gap-2 border-b border-line bg-card-soft px-4 py-1.5 text-xs font-semibold text-ink-2">
                <span
                  aria-hidden
                  className={cn("size-1.5 rounded-full", groupDotClass[group.tone])}
                />
                {group.label}
                <span className="tabular-nums text-ink-3">
                  {group.instances.length}
                </span>
              </header>
              {group.instances.map((instance) => (
                <WorkflowRunRow instance={instance} key={instance.demand_id} />
              ))}
            </section>
          ))}
        </div>
      )}
    </WorkSurface>
  );
}

const groupDotClass = {
  danger: "bg-danger",
  mute: "bg-mute",
  ok: "bg-ok",
  warn: "bg-warn"
} as const;

/** 单条流水线运行行：状态 + 标题行 / 节点链 / 摘要行。整行可点进详情。 */
function WorkflowRunRow({ instance }: { instance: WorkflowInstanceSummary }) {
  const navigate = useNavigate();
  const category = workflowCategory(instance);
  const summary = workflowInstanceSummary(instance);
  const optionalBadges = workflowOptionalBadges(instance);
  const nodeCounters = workflowNodeCounters(instance);
  const completed = instance.progress.completed_nodes;
  const total = instance.progress.total_nodes;

  return (
    <div
      className={cn(
        "cursor-pointer border-b border-line px-4 py-3 transition-colors last:border-b-0 hover:bg-card-inner",
        category === "blocked" &&
          "bg-danger-soft/40 shadow-[inset_3px_0_0_var(--danger)]",
      )}
      onClick={() =>
        void navigate({
          params: { demandId: instance.demand_id },
          to: "/workflows/$demandId"
})
      }
      role="listitem"
    >
      <div className="flex items-start justify-between gap-4">
        <Link
          className="group block min-w-0 flex-1 rounded-inner outline-none focus-visible:ring-2 focus-visible:ring-brand/60"
          onClick={(event) => event.stopPropagation()}
          params={{ demandId: instance.demand_id }}
          to="/workflows/$demandId"
        >
          <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            <StatusPill tone={workflowStatusTone(instance.status)}>
              {workflowStatusLabel(instance.status)}
            </StatusPill>
            <span className="min-w-0 truncate text-[13px] font-bold text-ink group-hover:text-brand-deep">
              {instance.title}
            </span>
            <span className="truncate text-xs text-ink-3">
              {instance.project_name}
            </span>
            {optionalBadges.map((badge) => (
              <StatusPill key={badge.key} showDot={false} tone={badge.tone}>
                {badge.label}
              </StatusPill>
            ))}
          </span>
          <span className="mt-2.5 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
            <WorkflowNodeChain progress={instance.progress} />
            <span className="text-xs font-semibold tabular-nums text-ink-2">
              {completed}/{total}
            </span>
            {nodeCounters.length > 0 ? (
              <span className="text-xs text-ink-2">
                {nodeCounters.map((counter, index) => (
                  <span key={counter.key}>
                    {index > 0 ? <span className="text-ink-3"> · </span> : null}
                    <span
                      className={
                        counter.key === "blocked"
                          ? "font-semibold text-danger-text"
                          : undefined
                      }
                    >
                      {counter.label} {counter.value}
                    </span>
                  </span>
                ))}
              </span>
            ) : null}
          </span>
          {summary.text ? (
            <span className="mt-1.5 flex items-start gap-2 text-[13px] leading-5 text-ink-2">
              <span
                aria-hidden
                className={cn(
                  "mt-1.5 size-1.5 shrink-0 rounded-full",
                  summary.blocked ? "bg-danger" : "bg-mute",
                )}
              />
              <span className="line-clamp-1">{summary.text}</span>
            </span>
          ) : null}
        </Link>
        <span className="inline-flex shrink-0 items-center gap-1.5 pt-1 text-xs tabular-nums text-ink-2">
          <Clock3 className="size-3.5" />
          {formatWorkflowTime(instance.updated_at)}
        </span>
      </div>
    </div>
  );
}

const chainBuckets = [
  { dotClass: "bg-brand", key: "completed", label: "已完成" },
  { dotClass: "bg-info", key: "running", label: "运行中" },
  { dotClass: "bg-warn", key: "waiting", label: "等待人工" },
  { dotClass: "bg-danger", key: "blocked", label: "阻断" },
] as const;

/**
 * mini 节点链：按 已完成 → 运行 → 等待人工 → 阻断 → 待执行 渲染节点点位。
 * 节点数多于 14 时退化为等宽分段条，保持行高稳定。
 */
function WorkflowNodeChain({
  progress
}: {
  progress: WorkflowInstanceSummary["progress"];
}) {
  const values: Record<(typeof chainBuckets)[number]["key"], number> = {
    blocked: progress.blocked_nodes,
    completed: progress.completed_nodes,
    running: progress.running_nodes,
    waiting: progress.waiting_human_nodes
};
  const known = Object.values(values).reduce((sum, count) => sum + count, 0);
  const pending = Math.max(progress.total_nodes - known, 0);
  const total = known + pending;

  if (total === 0) {
    return (
      <span className="inline-flex items-center gap-2" aria-hidden>
        <span className="h-px w-16 border-t border-dashed border-line-strong" />
        <span className="text-xs text-ink-3">待规划</span>
      </span>
    );
  }

  const segments = [
    ...chainBuckets.map((bucket) => ({ ...bucket, count: values[bucket.key] })),
    { count: pending, dotClass: "", key: "pending", label: "待执行" },
  ].filter((segment) => segment.count > 0);

  if (total > 14) {
    return (
      <span
        aria-label={chainAriaLabel(segments)}
        className="inline-flex h-1.5 w-40 overflow-hidden rounded-full bg-card-soft"
        role="img"
      >
        {segments.map((segment) => (
          <span
            className={cn(
              "h-full",
              segment.key === "pending" ? "bg-line" : segment.dotClass,
            )}
            key={segment.key}
            style={{ width: `${(segment.count / total) * 100}%` }}
          />
        ))}
      </span>
    );
  }

  const dots = segments.flatMap((segment) =>
    Array.from({ length: segment.count }, (_, index) => ({
      dotClass: segment.dotClass,
      key: `${segment.key}-${index}`,
      pending: segment.key === "pending"
})),
  );

  return (
    <span
      aria-label={chainAriaLabel(segments)}
      className="inline-flex items-center"
      role="img"
    >
      {dots.map((dot, index) => (
        <span className="inline-flex items-center" key={dot.key}>
          {index > 0 ? <span className="h-px w-2.5 bg-line-strong" /> : null}
          <span
            className={cn(
              "size-2 rounded-full",
              dot.pending
                ? "border border-line-strong bg-transparent"
                : dot.dotClass,
            )}
          />
        </span>
      ))}
    </span>
  );
}

function chainAriaLabel(
  segments: Array<{ count: number; label: string }>,
): string {
  return `节点：${segments
    .map((segment) => `${segment.label} ${segment.count}`)
    .join("，")}`;
}

function workflowCategory(instance: WorkflowInstanceSummary): WorkflowCategory {
  if (
    instance.progress.blocked_nodes > 0 ||
    instance.status === "failed" ||
    instance.status === "cancelled"
  ) {
    return "blocked";
  }
  if (instance.status === "waiting_human") {
    return "waiting";
  }
  if (instance.status === "running" || instance.status === "planning") {
    return "active";
  }
  if (instance.status === "completed") {
    return "done";
  }
  return "other";
}

/** 组内排序：阻断类置顶，其余按更新时间倒序。 */
function sortGroupInstances(instances: WorkflowInstanceSummary[]) {
  return [...instances].sort((a, b) => {
    const aBlocked = workflowCategory(a) === "blocked" ? 0 : 1;
    const bBlocked = workflowCategory(b) === "blocked" ? 0 : 1;
    if (aBlocked !== bBlocked) return aBlocked - bBlocked;
    return b.updated_at.localeCompare(a.updated_at);
  });
}

function workflowCategoryCounts(instances: WorkflowInstanceSummary[]) {
  const counts = { active: 0, blocked: 0, done: 0, other: 0, waiting: 0 };
  for (const instance of instances) {
    counts[workflowCategory(instance)] += 1;
  }
  return counts;
}

function workflowNodeCounters(instance: WorkflowInstanceSummary) {
  return [
    { key: "running", label: "运行", value: instance.progress.running_nodes },
    {
      key: "waiting",
      label: "人工",
      value: instance.progress.waiting_human_nodes
},
    { key: "blocked", label: "阻断", value: instance.progress.blocked_nodes },
  ].filter((counter) => counter.value > 0);
}

function workflowInstanceSummary(instance: WorkflowInstanceSummary) {
  if (instance.current_blocker?.title) {
    // 红点只给真正的阻断类实例；等待人工等行保持安静，文字自带「阻塞：」前缀
    return {
      blocked: workflowCategory(instance) === "blocked",
      text: `阻塞：${instance.current_blocker.title}`
};
  }

  if (instance.recent_event?.summary) {
    return { blocked: false, text: `最新事件：${instance.recent_event.summary}` };
  }

  return {
    blocked: false,
    text: instance.status_reason || "等待协调线程更新状态"
};
}

function workflowOptionalBadges(instance: WorkflowInstanceSummary) {
  return [
    instance.priority
      ? {
          key: "priority",
          label: instance.priority.label,
          tone: priorityTone(instance.priority.value)
}
      : undefined,
    instance.risk
      ? {
          key: "risk",
          label: instance.risk.label,
          tone: riskTone(instance.risk.level)
}
      : undefined,
    instance.sla
      ? {
          key: "sla",
          label: instance.sla.label,
          tone: instance.sla.breached ? "danger" : "warn"
}
      : undefined,
  ].filter(
    (badge): badge is { key: string; label: string; tone: Tone } =>
      // 只保留有信息量的标记：low 优先级 / 低风险等 mute 档不渲染，保持行安静
      Boolean(badge) && badge?.tone !== "mute",
  );
}

function priorityTone(priority: string): Tone {
  const normalized = priority.toLowerCase();
  if (["critical", "p0", "p1", "urgent"].includes(normalized)) {
    return "danger";
  }
  if (["high", "p2"].includes(normalized)) {
    return "warn";
  }
  return "mute";
}

function riskTone(risk: string): Tone {
  const normalized = risk.toLowerCase();
  if (["critical", "high", "severe"].includes(normalized)) {
    return "danger";
  }
  if (["medium", "moderate"].includes(normalized)) {
    return "warn";
  }
  return "mute";
}

function formatWorkflowTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "未知";
  }

  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit"
}).format(date);
}
