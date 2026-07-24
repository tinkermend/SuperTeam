import { Link } from "@tanstack/react-router";
import { ArrowRight, Inbox, ShieldCheck, UserCheck, Workflow } from "lucide-react";
import { cn } from "@/lib/utils";
import { IconTile, StatusPill, EmptyState, type Tone } from "@/components/superteam";
import type { InboxItem } from "@/lib/api/inbox";
import type {
  WorkflowInstanceStatus,
  WorkflowInstanceSummary
} from "@/lib/api/projects";
import { statusLabel } from "@/lib/status-labels";
import { compareIsoDesc, formatRelativeTime } from "@/lib/format-time";

/**
 * 项目驾驶舱右栏（未选中项目时的默认内容）：
 * ① 待我决策——跨项目审批/人工决策收件箱切片，首页只导流不承载审批操作；
 * ② 最近运行动态——复用页面已请求的 workflow instances，零额外请求。
 * 逐行扫读面，实底不透明；每行有主时间字段（相对时间 + tabular-nums）。
 */

const PANEL_CLASS =
  "flex min-w-0 flex-col gap-2 rounded-[14px] border border-line bg-card p-4 shadow-sm";

export function ProjectDashboardRail({
  decisionItems,
  decisionOpenCount,
  decisionsError,
  decisionsLoading,
  workflowInstances
}: {
  decisionItems: InboxItem[];
  decisionOpenCount: number;
  decisionsError?: string;
  decisionsLoading: boolean;
  workflowInstances: WorkflowInstanceSummary[];
}) {
  return (
    <aside
      aria-label="驾驶舱右栏"
      className="flex min-w-0 flex-col gap-4 @5xl/master-detail:sticky @5xl/master-detail:top-4 @5xl/master-detail:max-h-[calc(100svh-2rem)] @5xl/master-detail:overflow-y-auto"
      data-testid="projects-dashboard-rail"
    >
      <InboxDecisionsPanel
        error={decisionsError}
        isLoading={decisionsLoading}
        items={decisionItems}
        openCount={decisionOpenCount}
      />
      <RunActivityPanel instances={workflowInstances} />
    </aside>
  );
}

export function InboxDecisionsPanel({
  error,
  isLoading,
  items,
  openCount
}: {
  error?: string;
  isLoading: boolean;
  items: InboxItem[];
  openCount: number;
}) {
  return (
    <section
      aria-label="待我决策"
      className={PANEL_CLASS}
      data-testid="dashboard-decisions-panel"
    >
      <header className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-extrabold text-ink">
          待我决策{openCount > 0 ? ` · ${openCount}` : ""}
        </h3>
        <Link
          className="flex shrink-0 items-center gap-1 text-[12px] font-medium text-brand hover:opacity-75"
          to="/inbox"
        >
          查看收件箱
          <ArrowRight aria-hidden className="size-3" />
        </Link>
      </header>
      {error ? (
        <p className="text-[12px] leading-5 text-danger">{error}</p>
      ) : isLoading && items.length === 0 ? (
        <p className="text-[12px] leading-5 text-ink-3">正在加载决策事项…</p>
      ) : items.length === 0 ? (
        <EmptyState
          description="没有等待你处理的审批或人工决策。"
          icon={<Inbox />}
          title="决策已清空"
        />
      ) : (
        <ul className="flex min-w-0 flex-col gap-1.5">
          {items.map((item) => (
            <InboxDecisionRow item={item} key={item.id} />
          ))}
        </ul>
      )}
    </section>
  );
}

function InboxDecisionRow({ item }: { item: InboxItem }) {
  const isHighRisk = item.risk_level === "high";
  const Icon = item.item_type === "approval" ? ShieldCheck : UserCheck;
  const typeLabel = item.item_type === "approval" ? "审批" : "人工决策";
  const projectDecisionTarget =
    item.item_type === "project_decision" && item.source_project_id
      ? {
          params: { projectId: item.source_project_id },
          search: { focus: item.source_id, tab: "approval" },
          to: "/projects/$projectId" as const
}
      : null;

  const rowBody = (
    <>
      <IconTile size="sm" tone={isHighRisk ? "danger" : "brand"}>
        <Icon />
      </IconTile>
      <span className="min-w-0 flex-1">
        <span
          className="block min-w-0 line-clamp-2 break-words text-[12px] font-semibold leading-5 text-ink"
          title={item.title}
        >
          {item.title}
        </span>
        <span className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[11px] text-ink-3">
          <span className="shrink-0">{typeLabel}</span>
          {isHighRisk ? <StatusPill tone="danger">高危</StatusPill> : null}
          <time className="shrink-0 tabular-nums" dateTime={item.last_activity_at}>
            {formatRelativeTime(item.last_activity_at)}
          </time>
        </span>
      </span>
      <ArrowRight aria-hidden className="size-3.5 shrink-0 self-center text-ink-3" />
    </>
  );

  const rowClass =
    "flex min-w-0 items-start gap-2 rounded-[10px] bg-card-soft/60 px-2.5 py-2 transition-colors hover:bg-brand-soft/60";

  return (
    <li className="min-w-0">
      {projectDecisionTarget ? (
        <Link
          className={rowClass}
          params={projectDecisionTarget.params}
          search={projectDecisionTarget.search}
          to={projectDecisionTarget.to}
        >
          {rowBody}
        </Link>
      ) : (
        <Link className={rowClass} to="/inbox">
          {rowBody}
        </Link>
      )}
    </li>
  );
}

const runStatusTone: Partial<Record<WorkflowInstanceStatus, Tone>> = {
  cancelled: "mute",
  completed: "ok",
  failed: "danger",
  planning: "info",
  running: "info",
  waiting_human: "warn"
};

export function RunActivityPanel({
  instances
}: {
  instances: WorkflowInstanceSummary[];
}) {
  const recent = [...instances]
    .sort((left, right) => compareIsoDesc(left.updated_at, right.updated_at))
    .slice(0, 8);

  return (
    <section
      aria-label="最近运行动态"
      className={PANEL_CLASS}
      data-testid="dashboard-run-activity-panel"
    >
      <header className="flex items-center gap-2">
        <h3 className="text-sm font-extrabold text-ink">最近运行动态</h3>
      </header>
      {recent.length === 0 ? (
        <EmptyState
          description="项目产生运行实例后会在这里按时间倒序展示。"
          icon={<Workflow />}
          title="暂无运行记录"
        />
      ) : (
        <ul className="flex min-w-0 flex-col gap-1.5">
          {recent.map((instance) => (
            <li className="min-w-0" key={instance.demand_id}>
              <Link
                className="flex min-w-0 items-center gap-2 rounded-[10px] bg-card-soft/60 px-2.5 py-2 transition-colors hover:bg-brand-soft/60"
                params={{ projectId: instance.project_id }}
                to="/projects/$projectId"
              >
                <span className="min-w-0 flex-1">
                  <span
                    className="block min-w-0 truncate text-[12px] font-semibold leading-5 text-ink"
                    title={instance.project_name || instance.title}
                  >
                    {instance.project_name || instance.title}
                  </span>
                  <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11px] text-ink-3">
                    <StatusPill tone={runStatusTone[instance.status] ?? "mute"}>
                      {statusLabel(instance.status)}
                    </StatusPill>
                    <time
                      className={cn("shrink-0 tabular-nums")}
                      dateTime={instance.updated_at}
                    >
                      {formatRelativeTime(instance.updated_at)}
                    </time>
                  </span>
                </span>
                <ArrowRight
                  aria-hidden
                  className="size-3.5 shrink-0 text-ink-3"
                />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
