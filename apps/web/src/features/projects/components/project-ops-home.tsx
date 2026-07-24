import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { Bot, History, Inbox } from "lucide-react";
import {
  SoftCard,
  StatusPill,
  V3Button,
  type V3Tone,
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import type {
  Project,
  ProjectBudgetSummary,
  ProjectDecisionRequest,
  ProjectDemand,
  ProjectEvent,
  ProjectOverview,
  ProjectPlanRevision,
  ProjectTask,
  ProjectTaskGraph,
} from "@/lib/api/projects";
import {
  decisionStatusLabel,
  taskStatusLabel,
} from "@/lib/status-labels";
import { formatRelativeTime } from "@/lib/format-time";
import {
  buildWeekPulse,
  coordinationModeLabel,
  countWeekPulseActivity,
  employeeBusyLabel,
  filterOpsEvents,
  resolveTaskMode,
  selectActiveOrBlockedTasks,
  type OpsLaunchMode,
  type OpsPulseDay,
} from "../lib/project-ops-home";

type ProjectOpsHomeProps = {
  artifactsCount?: number;
  budgetSummary?: ProjectBudgetSummary;
  decisionRequests: ProjectDecisionRequest[];
  demands: ProjectDemand[];
  events: ProjectEvent[];
  isArchived?: boolean;
  onResolveDecision: (decisionId: string, decision: string) => void;
  onShowAllTasks: () => void;
  overview?: ProjectOverview;
  planRevisions: ProjectPlanRevision[];
  principalNamesById?: ReadonlyMap<string, string>;
  project: Project;
  riskLabel?: string;
  runtimePlacementPanel?: ReactNode;
  taskGraph?: ProjectTaskGraph;
  tasks: ProjectTask[];
};

const modePillClass: Record<OpsLaunchMode, string> = {
  plan: "bg-v3-brand-soft text-v3-brand-deep",
  loop: "bg-v3-artifact-soft text-v3-artifact-text",
  chat: "bg-emerald-50 text-emerald-700",
};

/** 日格高度按约 10–12 条芯片估，超出可滚；数据侧最多保留 15 条。 */
const PULSE_DAY_MIN_HEIGHT_CLASS = "min-h-[28rem]";

function modeToneClass(mode: OpsLaunchMode) {
  return modePillClass[mode] ?? modePillClass.plan;
}

function statusTone(status: string): V3Tone {
  switch (status) {
    case "completed":
    case "resolved":
    case "approved":
      return "ok";
    case "running":
    case "waiting_human":
    case "pending":
      return "warn";
    case "failed":
    case "rejected":
      return "danger";
    case "planned":
    case "queued":
      return "info";
    default:
      return "mute";
  }
}

export function ProjectOpsHome({
  artifactsCount,
  budgetSummary,
  decisionRequests,
  demands,
  events,
  isArchived,
  onResolveDecision,
  onShowAllTasks,
  overview,
  planRevisions,
  principalNamesById,
  project,
  riskLabel,
  runtimePlacementPanel,
  taskGraph,
  tasks,
}: ProjectOpsHomeProps) {
  const pool = overview?.digital_employee_pool ?? [];
  const sourceTasks = overview?.active_tasks?.length
    ? [...overview.active_tasks, ...tasks]
    : tasks;
  const dedupedTasks = dedupeTasks(sourceTasks);
  const sourceEvents = overview?.recent_events?.length
    ? overview.recent_events
    : events;

  const pulseDays = buildWeekPulse({
    demands,
    graphNodes: taskGraph?.nodes,
    tasks: dedupedTasks,
  });
  const pulseCount = countWeekPulseActivity(pulseDays);
  const runningSlice = selectActiveOrBlockedTasks(dedupedTasks, 3);
  const pendingBlockers = decisionRequests.filter(
    (d) => d.status_snapshot === "pending",
  );
  const opsEvents = filterOpsEvents(sourceEvents, 6);
  const latestPlan = [...planRevisions].sort(
    (a, b) => b.revision_number - a.revision_number,
  )[0];
  const demandsById = new Map(demands.map((d) => [d.id, d]));
  const hasPulseActivity = pulseCount > 0;
  const hasRunningSlice = runningSlice.length > 0;
  const railIsQuiet = pendingBlockers.length === 0 && opsEvents.length === 0;

  return (
    <div
      className="grid min-w-0 gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(260px,300px)] lg:items-stretch"
      data-testid="project-ops-home"
    >
      <div className="flex min-w-0 flex-col gap-3">
        <SoftCard
          className="flex min-h-0 flex-1 flex-col overflow-hidden p-3.5"
          data-testid="project-ops-pulse"
          id="project-overview-pulse"
        >
          <div className="mb-2 flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-extrabold text-v3-ink">运行脉搏</h3>
                <span className="rounded-md bg-v3-brand-soft px-1.5 py-0.5 text-[11px] font-bold text-v3-brand-deep">
                  本周
                </span>
              </div>
              {hasPulseActivity ? (
                <p className="mt-0.5 text-[11.5px] text-v3-ink-3">
                  时间 + Plan/Loop/对话 · {pulseCount} 次
                </p>
              ) : null}
            </div>
            <V3Button size="sm" type="button" variant="ghost" onClick={onShowAllTasks}>
              全部任务
            </V3Button>
          </div>

          {hasPulseActivity ? (
            <>
              <div className="mb-2 flex flex-wrap gap-1.5 text-[11px]">
                <span className={cn("rounded-md px-1.5 py-0.5 font-bold", modeToneClass("plan"))}>
                  Plan
                </span>
                <span className={cn("rounded-md px-1.5 py-0.5 font-bold", modeToneClass("loop"))}>
                  Loop
                </span>
                <span className={cn("rounded-md px-1.5 py-0.5 font-bold", modeToneClass("chat"))}>
                  对话
                </span>
              </div>
              <WeekPulseGrid days={pulseDays} />
            </>
          ) : (
            <div
              className={cn(
                "relative flex-1 overflow-hidden rounded-[12px] border border-v3-line bg-v3-card-soft/40",
                PULSE_DAY_MIN_HEIGHT_CLASS,
              )}
              data-testid="project-ops-pulse-empty"
            >
              <div
                aria-hidden
                className={cn(
                  "pointer-events-none grid h-full grid-cols-7 divide-x divide-v3-line/80",
                  PULSE_DAY_MIN_HEIGHT_CLASS,
                )}
              >
                {pulseDays.map((day) => (
                  <div
                    className={cn(
                      "flex min-w-0 flex-col px-1.5 py-1.5",
                      day.isToday && "bg-v3-brand-soft/35",
                    )}
                    key={day.dayKey}
                  >
                    <div className="flex items-baseline justify-between gap-1 text-[10.5px] text-v3-ink-3">
                      <span>
                        {day.weekdayLabel}
                        {day.isToday ? " · 今" : ""}
                      </span>
                      <b className="font-extrabold tabular-nums text-v3-ink/70">
                        {day.dayOfMonth}
                      </b>
                    </div>
                  </div>
                ))}
              </div>
              <div className="pointer-events-none absolute inset-0 flex items-center justify-center px-4">
                <p className="text-center text-base font-extrabold tracking-tight text-v3-ink sm:text-lg">
                  本周暂无任务活动
                </p>
              </div>
            </div>
          )}
        </SoftCard>

        {hasRunningSlice || hasPulseActivity ? (
          <SoftCard
            className="overflow-hidden p-3.5"
            data-testid="project-ops-running"
            id="project-overview-execution"
          >
            <div className="mb-2 flex flex-wrap items-start justify-between gap-2">
              <div>
                <h3 className="text-sm font-extrabold text-v3-ink">执行中与阻塞</h3>
                <p className="mt-0.5 text-[11.5px] text-v3-ink-3">最多 3 条</p>
              </div>
              <V3Button size="sm" type="button" variant="ghost" onClick={onShowAllTasks}>
                全部任务
              </V3Button>
            </div>
            {runningSlice.length === 0 ? (
              <p className="text-[12.5px] text-v3-ink-3">暂无执行中或阻塞任务</p>
            ) : (
              <ul className="divide-y divide-v3-line">
                {runningSlice.map((task) => {
                  const mode = resolveTaskMode(task, demandsById);
                  return (
                    <li
                      className="flex items-start justify-between gap-2 py-2 first:pt-0 last:pb-0"
                      key={task.id}
                    >
                      <div className="min-w-0">
                        <p className="truncate text-[12.5px] font-bold">{task.title}</p>
                        <p className="mt-0.5 line-clamp-1 text-[11.5px] text-v3-ink-2">
                          {task.summary || "等待系统推进"}
                        </p>
                        <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[11px] text-v3-ink-3">
                          <span
                            className={cn(
                              "rounded px-1 font-bold",
                              modeToneClass(mode),
                            )}
                          >
                            {coordinationModeLabel(mode)}
                          </span>
                          {task.assigned_digital_employee_id ? (
                            <span>
                              {principalNamesById?.get(
                                task.assigned_digital_employee_id,
                              ) || "数字员工"}
                            </span>
                          ) : null}
                        </div>
                      </div>
                      <StatusPill tone={statusTone(task.status)}>
                        {taskStatusLabel(task.status)}
                      </StatusPill>
                    </li>
                  );
                })}
              </ul>
            )}
          </SoftCard>
        ) : null}

        <div className="grid min-w-0 gap-3 sm:grid-cols-2">
          <SoftCard
            className="overflow-hidden px-3.5 py-3"
            data-testid="project-ops-employees"
          >
            {pool.length === 0 ? (
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2.5">
                  <div className="grid size-8 shrink-0 place-items-center rounded-[10px] bg-v3-card-soft text-v3-ink-2">
                    <Bot className="size-3.5" />
                  </div>
                  <div className="min-w-0">
                    <h3 className="text-sm font-extrabold text-v3-ink">项目数字员工</h3>
                    <p className="mt-0.5 text-[12px] text-v3-ink-3">尚未配置数字员工池</p>
                  </div>
                </div>
                {!isArchived ? (
                  <Link
                    className="shrink-0 text-[12px] font-semibold text-v3-brand hover:opacity-80"
                    params={{ projectId: project.id }}
                    to="/projects/$projectId/config"
                  >
                    配置池 →
                  </Link>
                ) : null}
              </div>
            ) : (
              <>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="min-w-0">
                    <h3 className="text-sm font-extrabold text-v3-ink">项目数字员工</h3>
                    <p className="mt-0.5 text-[11.5px] text-v3-ink-3">忙闲态势</p>
                  </div>
                  {!isArchived ? (
                    <Link
                      className="shrink-0 text-[12px] font-semibold text-v3-brand hover:opacity-80"
                      params={{ projectId: project.id }}
                      to="/projects/$projectId/config"
                    >
                      配置池 →
                    </Link>
                  ) : null}
                </div>
                <ul className="mt-2 divide-y divide-v3-line">
                  {pool.slice(0, 6).map((member) => {
                    const busy = employeeBusyLabel(member, dedupedTasks);
                    const name =
                      member.display_name_snapshot ||
                      principalNamesById?.get(member.principal_id) ||
                      "数字员工";
                    return (
                      <li
                        className="flex items-center gap-2 py-1.5 first:pt-0 last:pb-0"
                        key={member.id}
                      >
                        <div className="grid size-7 shrink-0 place-items-center rounded-full bg-v3-card-soft text-[10px] font-bold text-v3-ink-2">
                          {name.slice(0, 1)}
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-[12.5px] font-bold">
                            <Link
                              className="hover:underline"
                              params={{ employeeId: member.principal_id }}
                              to="/employees/$employeeId"
                            >
                              {name}
                            </Link>
                          </p>
                        </div>
                        <p
                          className={cn(
                            "max-w-[42%] truncate text-right text-[11.5px]",
                            busy.tone === "danger" && "text-v3-danger",
                            busy.tone === "warn" && "text-v3-ink-2",
                            busy.tone === "mute" && "text-v3-ink-3",
                          )}
                        >
                          {busy.label}
                        </p>
                      </li>
                    );
                  })}
                </ul>
              </>
            )}
          </SoftCard>

          <div className="min-w-0" data-testid="project-ops-runtime" id="project-overview-runtime">
            {runtimePlacementPanel ?? (
              <SoftCard className="overflow-hidden px-3.5 py-3">
                <h3 className="text-sm font-extrabold text-v3-ink">Runtime 节点</h3>
                <p className="mt-2 text-[12.5px] text-v3-ink-3">尚未绑定 Runtime</p>
              </SoftCard>
            )}
          </div>
        </div>
      </div>

      <aside
        className={cn(
          "flex min-w-0 flex-col gap-3",
          railIsQuiet ? "h-full" : "lg:sticky lg:top-4 lg:self-start",
        )}
      >
        {railIsQuiet ? (
          <SoftCard
            className="flex min-h-0 flex-1 flex-col overflow-hidden p-0"
            data-testid="project-ops-rail"
          >
            <div
              className="flex items-center justify-between gap-2 border-b border-v3-line px-3.5 py-2.5"
              data-testid="project-ops-blockers"
              id="project-overview-pending"
            >
              <span className="flex min-w-0 items-center gap-1.5 text-[12.5px]">
                <Inbox className="size-3.5 shrink-0 text-v3-ink-3" />
                <span className="font-bold text-v3-ink">阻塞 · 0</span>
                <span className="truncate text-v3-ink-3">当前无阻塞</span>
              </span>
              <Link
                className="shrink-0 text-[11.5px] font-semibold text-v3-brand hover:opacity-80"
                to="/inbox"
              >
                全部待办
              </Link>
            </div>
            <div
              className="flex items-center justify-between gap-2 border-b border-v3-line px-3.5 py-2.5"
              data-testid="project-ops-events"
            >
              <span className="flex min-w-0 items-center gap-1.5 text-[12.5px]">
                <History className="size-3.5 shrink-0 text-v3-ink-3" />
                <span className="font-bold text-v3-ink">事件</span>
                <span className="truncate text-v3-ink-3">暂无执行/审批事件</span>
              </span>
              <Link
                className="shrink-0 text-[11.5px] font-semibold text-v3-brand hover:opacity-80"
                search={{ project_id: project.id }}
                to="/audit"
              >
                审计
              </Link>
            </div>
            <div className="flex-1 px-3.5 py-3" data-testid="project-ops-metrics">
              <h3 className="mb-2 text-[12px] font-extrabold text-v3-ink">项目脉搏</h3>
              <div className="grid grid-cols-2 gap-1.5">
                <MetricCell
                  label="预算消耗"
                  value={
                    budgetSummary
                      ? `${budgetSummary.actual_cost} / ${budgetSummary.estimated_cost}`
                      : "—"
                  }
                />
                <MetricCell label="风险" value={riskLabel || "—"} />
                <MetricCell
                  label="计划版本"
                  value={latestPlan ? `v${latestPlan.revision_number}` : "—"}
                />
                <MetricCell
                  label="证据 / 工件"
                  value={artifactsCount != null ? String(artifactsCount) : "—"}
                />
              </div>
            </div>
          </SoftCard>
        ) : (
          <>
            <SoftCard
              className="overflow-hidden p-3.5"
              data-testid="project-ops-blockers"
              id="project-overview-pending"
            >
              <div className="mb-2 flex flex-wrap items-start justify-between gap-2">
                <div>
                  <h3 className="text-sm font-extrabold text-v3-ink">
                    本项目阻塞 · {pendingBlockers.length}
                  </h3>
                  <p className="mt-0.5 text-[11.5px] text-v3-ink-3">非第二收件箱</p>
                </div>
                <V3Button asChild size="sm" variant="ghost">
                  <Link to="/inbox">全部待办</Link>
                </V3Button>
              </div>
              {pendingBlockers.length === 0 ? (
                <p className="flex items-center gap-1.5 text-[12.5px] text-v3-ink-3">
                  <Inbox className="size-3.5" />
                  当前没有项目阻塞
                </p>
              ) : (
                <ul className="divide-y divide-v3-line">
                  {pendingBlockers.slice(0, 4).map((decision) => (
                    <li className="py-2 first:pt-0 last:pb-0" key={decision.id}>
                      <div className="min-w-0">
                        <p className="truncate text-[12.5px] font-bold">
                          {decision.title_snapshot}
                        </p>
                        <p className="mt-0.5 line-clamp-2 text-[11.5px] text-v3-ink-2">
                          {decision.summary_snapshot &&
                          decision.summary_snapshot !== decision.title_snapshot
                            ? decision.summary_snapshot
                            : "等待处理"}
                        </p>
                        <div className="mt-1 flex flex-wrap gap-2 text-[11px] text-v3-ink-3">
                          {decision.created_at ? (
                            <time dateTime={decision.created_at}>
                              {formatRelativeTime(decision.created_at)}
                            </time>
                          ) : null}
                          <StatusPill tone={statusTone(decision.status_snapshot)}>
                            {decisionStatusLabel(decision.status_snapshot)}
                          </StatusPill>
                        </div>
                      </div>
                      <div className="mt-2">
                        <BlockerActions
                          decision={decision}
                          onResolveDecision={onResolveDecision}
                        />
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </SoftCard>

            <SoftCard className="overflow-hidden p-3.5" data-testid="project-ops-events">
              <div className="mb-2 flex flex-wrap items-start justify-between gap-2">
                <div>
                  <h3 className="text-sm font-extrabold text-v3-ink">事件流</h3>
                  <p className="mt-0.5 text-[11.5px] text-v3-ink-3">执行 + 审批</p>
                </div>
                <V3Button asChild size="sm" variant="ghost">
                  <Link search={{ project_id: project.id }} to="/audit">
                    审计
                  </Link>
                </V3Button>
              </div>
              {opsEvents.length === 0 ? (
                <p className="flex items-center gap-1.5 text-[12.5px] text-v3-ink-3">
                  <History className="size-3.5" />
                  暂无项目事件
                </p>
              ) : (
                <ul className="divide-y divide-v3-line">
                  {opsEvents.map((event) => {
                    const display = opsEventTitle(event);
                    return (
                      <li
                        className="grid grid-cols-[auto_minmax(0,1fr)] gap-2 py-1.5 first:pt-0 last:pb-0"
                        key={event.id}
                      >
                        <time
                          className="shrink-0 text-[11.5px] tabular-nums text-v3-ink-3"
                          dateTime={event.created_at}
                          title={event.created_at}
                        >
                          {formatRelativeTime(event.created_at)}
                        </time>
                        <div className="min-w-0">
                          <p className="flex flex-wrap items-center gap-1 text-[12px] font-bold">
                            {display.title}
                            {display.kind ? (
                              <span className="rounded bg-v3-card-soft px-1 text-[10px] font-bold text-v3-ink-2">
                                {display.kind}
                              </span>
                            ) : null}
                          </p>
                          {display.summary ? (
                            <p className="mt-0.5 line-clamp-2 text-[11.5px] text-v3-ink-2">
                              {display.summary}
                            </p>
                          ) : null}
                        </div>
                      </li>
                    );
                  })}
                </ul>
              )}
            </SoftCard>

            <SoftCard className="overflow-hidden p-3.5" data-testid="project-ops-metrics">
              <h3 className="mb-2 text-sm font-extrabold text-v3-ink">项目脉搏</h3>
              <div className="grid grid-cols-2 gap-1.5">
                <MetricCell
                  label="预算消耗"
                  value={
                    budgetSummary
                      ? `${budgetSummary.actual_cost} / ${budgetSummary.estimated_cost}`
                      : "—"
                  }
                />
                <MetricCell label="风险" value={riskLabel || "—"} />
                <MetricCell
                  label="计划版本"
                  value={latestPlan ? `v${latestPlan.revision_number}` : "—"}
                />
                <MetricCell
                  label="证据 / 工件"
                  value={artifactsCount != null ? String(artifactsCount) : "—"}
                />
              </div>
            </SoftCard>
          </>
        )}
      </aside>
    </div>
  );
}

function WeekPulseGrid({ days }: { days: OpsPulseDay[] }) {
  return (
    <div className={cn("grid grid-cols-7 gap-1.5", PULSE_DAY_MIN_HEIGHT_CLASS)}>
      {days.map((day) => (
        <div
          className={cn(
            "flex min-h-0 min-w-0 flex-col gap-1 overflow-hidden rounded-[10px] bg-v3-card-soft p-1.5",
            PULSE_DAY_MIN_HEIGHT_CLASS,
            day.isToday && "border border-v3-brand/35 bg-v3-card",
          )}
          key={day.dayKey}
        >
          <div className="flex shrink-0 justify-between text-[10.5px] text-v3-ink-3">
            <span>
              {day.weekdayLabel}
              {day.isToday ? " · 今" : ""}
            </span>
            <b className="font-extrabold text-v3-ink tabular-nums">{day.dayOfMonth}</b>
          </div>
          <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto">
            {day.chips.length === 0 ? (
              <div className="grid flex-1 place-items-center text-[10.5px] text-v3-ink-3">
                —
              </div>
            ) : (
              day.chips.map((chip) => (
                <div
                  className="grid shrink-0 grid-cols-[6px_minmax(0,1fr)] gap-1 rounded-md border border-v3-line bg-v3-card px-1 py-0.5"
                  key={chip.taskId}
                  title={chip.title}
                >
                  <span
                    className={cn(
                      "mt-1 size-1.5 rounded-full",
                      chip.statusTone === "ok" && "bg-v3-ok",
                      chip.statusTone === "warn" && "bg-v3-warn",
                      chip.statusTone === "info" && "bg-v3-brand",
                      chip.statusTone === "danger" && "bg-v3-danger",
                      chip.statusTone === "mute" && "bg-v3-ink-3",
                    )}
                  />
                  <div className="min-w-0">
                    <p className="truncate text-[10.5px] font-bold leading-4">{chip.title}</p>
                    <div className="mt-0.5 flex flex-wrap items-center gap-1 text-[10px] text-v3-ink-3">
                      <span className="tabular-nums">{chip.timeLabel}</span>
                      <span
                        className={cn("rounded px-1 font-bold", modeToneClass(chip.mode))}
                      >
                        {coordinationModeLabel(chip.mode)}
                      </span>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      ))}
    </div>
  );
}

function BlockerActions({
  decision,
  onResolveDecision,
}: {
  decision: ProjectDecisionRequest;
  onResolveDecision: (decisionId: string, decision: string) => void;
}) {
  const actions =
    decision.decision_type === "task_failure_recovery"
      ? [
          { label: "重试", value: "retry" },
          { label: "取消下游", value: "cancel_downstream" },
        ]
      : [
          { label: "批准", value: "approved" },
          { label: "要求补证", value: "needs_more_evidence" },
        ];
  return (
    <div className="flex flex-wrap gap-1.5">
      {actions.map((action) => (
        <V3Button
          key={action.value}
          size="sm"
          type="button"
          variant={
            action.value === "retry" || action.value === "approved"
              ? "primary"
              : "outline"
          }
          onClick={() => onResolveDecision(decision.id, action.value)}
        >
          {action.label}
        </V3Button>
      ))}
    </div>
  );
}

function MetricCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[10px] bg-v3-card-soft px-2.5 py-2">
      <div className="text-[10.5px] text-v3-ink-3">{label}</div>
      <div className="mt-0.5 text-[13px] font-extrabold tracking-tight tabular-nums text-v3-ink">
        {value}
      </div>
    </div>
  );
}

function dedupeTasks(tasks: ProjectTask[]): ProjectTask[] {
  const map = new Map<string, ProjectTask>();
  for (const task of tasks) {
    if (!map.has(task.id)) map.set(task.id, task);
  }
  return [...map.values()];
}

function opsEventTitle(event: ProjectEvent): {
  kind?: string;
  summary?: string;
  title: string;
} {
  switch (event.event_type) {
    case "project_task.created":
      return { kind: "执行", title: "任务创建", summary: event.summary };
    case "project_task.dispatched":
      return { kind: "执行", title: "任务开始", summary: event.summary };
    case "project_task.completed":
      return { kind: "执行", title: "任务完成", summary: event.summary };
    case "project_task.failed":
      return { kind: "执行", title: "任务失败", summary: event.summary };
    case "decision.requested":
      return { kind: "审批", title: "审批创建", summary: event.summary };
    case "decision.submitted":
      return { kind: "审批", title: "审批已处理", summary: event.summary };
    case "project.acceptance.submitted":
      return { kind: "审批", title: "验收已提交", summary: event.summary };
    case "transfer.requested":
      return { kind: "审批", title: "转派请求", summary: event.summary };
    default:
      if (event.event_type.startsWith("project_task.")) {
        return { kind: "执行", title: "任务事件", summary: event.summary };
      }
      return { title: event.event_type, summary: event.summary };
  }
}
