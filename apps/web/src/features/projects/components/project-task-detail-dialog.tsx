import { useState, type ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Activity, ArrowUpRight, ClipboardList, GitBranch, Inbox } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { StatusPill, IconTile } from "@/components/superteam";
import { cn } from "@/lib/utils";
import type {
  ProjectDecisionRequest,
  ProjectDemand,
  ProjectOverview,
  ProjectTask,
  ProjectTaskGraph
} from "@/lib/api/projects";
import { riskLevelLabel, runStatusLabel, taskStatusLabel } from "@/lib/status-labels";
import { isAwaitingHumanApproval } from "@/lib/task-status";
import { formatDateTime, formatRelativeTime, formatRunDuration } from "@/lib/format-time";
import {
  formatBlocker,
  taskFieldKeyLabel,
  taskStatusTone
} from "@/features/flow-graph/inspector-primitives";
import { providerDisplayName } from "@/features/employees/provider-label";
import {
  coordinationModeLabel,
  resolveTaskMode
} from "../lib/project-ops-home";
import { BlockerActions, modeToneClass } from "./project-ops-home";

type ProjectTaskDetailDialogProps = {
  taskId?: string;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  tasks: ProjectTask[];
  demands: ProjectDemand[];
  decisionRequests: ProjectDecisionRequest[];
  taskGraph?: ProjectTaskGraph;
  overview?: ProjectOverview;
  principalNamesById?: ReadonlyMap<string, string>;
  onResolveDecision: (decisionId: string, decision: string) => void;
  /**
   * 按 demand 懒查执行图：页面只预载最新 demand 的图，历史需求的任务打开弹层时
   * 用任务自己的 demand_id 补一次查询（queryKey 与页面同族，最新 demand 直接命中缓存）。
   */
  fetchTaskGraph?: (demandId: string) => Promise<ProjectTaskGraph>;
};

/**
 * 项目视角的任务详情弹层：任务自身事实 + 当前执行图切片 + 该任务的待决事项。
 * 纯投影已加载的项目详情数据（tasks/overview/taskGraph/decisionRequests），不发新请求；
 * 决策处理复用工作台同一条 onResolveDecision 出口，与收件箱 any-of-N 语义一致。
 */
export function ProjectTaskDetailDialog({
  taskId,
  onOpenChange,
  projectId,
  tasks,
  demands,
  decisionRequests,
  taskGraph,
  overview,
  principalNamesById,
  onResolveDecision,
  fetchTaskGraph
}: ProjectTaskDetailDialogProps) {
  const preloadedNode = taskGraph?.nodes.find((node) => node.id === taskId);
  const task =
    preloadedNode ?? tasks.find((item) => item.id === taskId);
  const open = Boolean(taskId && task);
  const lazyDemandId = !preloadedNode ? task?.demand_id : undefined;
  const lazyGraphQuery = useQuery({
    enabled: Boolean(open && lazyDemandId && fetchTaskGraph),
    // 与页面预载查询同 key 族：任务属最新 demand 时直接命中缓存，不发请求。
    queryKey: ["project-task-graph", projectId, lazyDemandId],
    queryFn: () => fetchTaskGraph!(lazyDemandId as string),
    staleTime: 30_000
});
  if (!task) return null;

  const activeGraph = preloadedNode
    ? taskGraph
    : lazyGraphQuery.data?.nodes.some((node) => node.id === task.id)
      ? lazyGraphQuery.data
      : undefined;
  const graphNode =
    preloadedNode ?? activeGraph?.nodes.find((node) => node.id === task.id);
  const isGraphLoading = !graphNode && lazyGraphQuery.isFetching;

  const demandsById = new Map(demands.map((demand) => [demand.id, demand]));
  const mode = resolveTaskMode(task, demandsById);
  const run = activeGraph?.runs.find((item) => item.project_task_id === task.id);
  const result = activeGraph?.execution_summaries.find(
    (item) => item.project_task_id === task.id,
  );
  const openDecisionStatuses = new Set(["pending", "waiting", "requested", "open"]);
  const pendingDecisions = decisionRequests.filter((item) => {
    if (item.project_task_id !== task.id) return false;
    return openDecisionStatuses.has((item.status_snapshot ?? "").trim().toLowerCase());
  });
  const isWaitingHuman =
    (task.status ?? "").trim().toLowerCase() === "waiting_human" ||
    isAwaitingHumanApproval(task);
  const orphanHumanWait = isWaitingHuman && pendingDecisions.length === 0;
  const employeeName = task.assigned_digital_employee_id
    ? (activeGraph?.employees.find(
        (item) => item.digital_employee_id === task.assigned_digital_employee_id,
      )?.display_name ??
      principalNamesById?.get(task.assigned_digital_employee_id) ??
      overview?.digital_employee_pool?.find(
        (item) => item.principal_id === task.assigned_digital_employee_id,
      )?.display_name_snapshot ??
      "数字员工")
    : undefined;
  const timelineEntries = [
    { label: "创建", at: task.created_at },
    { label: "开始", at: graphNode?.started_at },
    { label: "结束", at: graphNode?.finished_at },
    {
      label: "更新",
      at: graphNode?.finished_at ? undefined : task.updated_at
},
  ].filter((entry): entry is { label: string; at: string } => Boolean(entry.at));
  const inputChips = valueChips(graphNode?.input_requirements);
  const outputChips = valueChips(graphNode?.expected_outputs);
  const contractLines = contractEntries(graphNode?.handoff_contract);
  const blockerText = graphNode ? formatBlocker(graphNode) : "";
  const hasBlocker = Boolean(graphNode?.current_blocker);

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent
        className="flex max-h-[85vh] w-full flex-col gap-0 p-0 sm:max-w-xl"
        data-testid="project-task-detail-dialog"
      >
        <DialogHeader className="shrink-0 border-b border-line px-5 py-4 text-left">
          <div className="flex items-start gap-3 pr-6">
            <IconTile tone="brand">
              <ClipboardList />
            </IconTile>
            <div className="min-w-0">
              <DialogTitle className="text-[15px] font-bold leading-6 tracking-normal text-ink">
                {task.title}
              </DialogTitle>
              <DialogDescription className="sr-only">
                查看该任务的状态、编排上下文与待决事项
              </DialogDescription>
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                <StatusPill tone={taskStatusTone(task.status)}>
                  {taskStatusLabel(task.status)}
                </StatusPill>
                <span
                  className={cn(
                    "rounded-md px-1.5 py-0.5 text-[11px] font-bold",
                    modeToneClass(mode),
                  )}
                >
                  {coordinationModeLabel(mode)}
                </span>
                {task.risk_level ? (
                  <span className="rounded-md bg-card-soft px-1.5 py-0.5 text-[11px] font-semibold text-ink-2">
                    {riskLevelLabel(task.risk_level)}
                  </span>
                ) : null}
                {isAwaitingHumanApproval(task) ? (
                  <span className="rounded-md bg-card-soft px-1.5 py-0.5 text-[11px] font-semibold text-ink-2">
                    需人工审批
                  </span>
                ) : null}
              </div>
            </div>
          </div>
        </DialogHeader>

        <div className="min-h-0 space-y-4 overflow-y-auto px-5 py-4">
          {task.summary ? (
            <p className="text-[13px] leading-6 text-ink-2">{task.summary}</p>
          ) : null}

          <div className="grid gap-2 sm:grid-cols-2">
            <div className="rounded-[10px] bg-card-soft px-3 py-2.5">
              <div className="text-[10.5px] font-bold text-ink-3">负责员工</div>
              {task.assigned_digital_employee_id && employeeName ? (
                <Link
                  aria-label={`查看${employeeName}详情`}
                  className="mt-1 inline-flex items-center gap-1 text-[13px] font-bold text-brand-deep hover:text-brand"
                  params={{ employeeId: task.assigned_digital_employee_id }}
                  to="/employees/$employeeId"
                >
                  {employeeName}
                  <ArrowUpRight className="size-3.5 shrink-0" />
                </Link>
              ) : (
                <div className="mt-1 text-[13px] text-ink-3">未分配</div>
              )}
            </div>
            <div className="rounded-[10px] bg-card-soft px-3 py-2.5">
              <div className="text-[10.5px] font-bold text-ink-3">时间</div>
              {timelineEntries.length > 0 ? (
                <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[12px] text-ink-2">
                  {timelineEntries.map((entry) => (
                    <span className="whitespace-nowrap" key={entry.label}>
                      <span className="text-ink-3">{entry.label}</span>{" "}
                      <time className="tabular-nums" dateTime={entry.at}>
                        {formatDateTime(entry.at)}
                      </time>
                    </span>
                  ))}
                </div>
              ) : (
                <div className="mt-1 text-[13px] text-ink-3">—</div>
              )}
            </div>
          </div>

          <section className="space-y-2">
            <div className="flex items-center justify-between gap-2">
              <SectionEyebrow icon={<GitBranch />} label="编排" />
              {task.demand_id ? (
                <Link
                  aria-label="查看该任务所在需求流程"
                  className="inline-flex items-center gap-1 text-[12px] font-semibold text-brand hover:opacity-80"
                  params={{ projectId }}
                  search={{ demand: task.demand_id, tab: "demands" }}
                  to="/projects/$projectId"
                >
                  查看需求流程
                  <ArrowUpRight className="size-3.5" />
                </Link>
              ) : null}
            </div>
            {graphNode ? (
              <div className="space-y-2.5">
                <div className="flex items-start gap-1.5 text-[12.5px] leading-5">
                  <span
                    className={cn(
                      "mt-1.5 size-1.5 shrink-0 rounded-full",
                      hasBlocker ? "bg-warn" : "bg-ink-3/50",
                    )}
                  />
                  <span className={hasBlocker ? "text-ink" : "text-ink-3"}>
                    {blockerText}
                  </span>
                </div>
                <FieldChips label="输入" chips={inputChips} />
                <FieldChips label="预期输出" chips={outputChips} />
                {contractLines.length > 0 ? (
                  <div>
                    <div className="mb-1 text-[10.5px] font-bold text-ink-3">
                      交接契约
                    </div>
                    <div className="space-y-1 rounded-[10px] bg-card-soft px-3 py-2 text-[11.5px] leading-5 text-ink-2">
                      {contractLines.map((line, index) => (
                        <p className="break-words" key={`${line.label}-${index}`}>
                          {line.label ? (
                            <span className="font-semibold text-ink">{line.label}</span>
                          ) : null}
                          {line.label ? " " : ""}
                          {line.text}
                        </p>
                      ))}
                    </div>
                  </div>
                ) : null}
              </div>
            ) : isGraphLoading ? (
              <p className="text-[12.5px] leading-5 text-ink-3">正在加载编排数据…</p>
            ) : (
              <p className="text-[12.5px] leading-5 text-ink-3">
                {lazyGraphQuery.isError
                  ? "编排数据加载失败，可稍后重开本弹层重试"
                  : "当前执行图未包含该任务（可能来自历史需求或已重新规划）"}
              </p>
            )}
          </section>

          <section className="space-y-2">
            <SectionEyebrow icon={<Activity />} label="运行与结论" />
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[12.5px]">
              {run ? (
                <>
                  <span className="inline-flex items-center gap-1.5 text-ink">
                    <span
                      className={cn(
                        "size-1.5 rounded-full",
                        runDotClass(run.status),
                      )}
                    />
                    {runStatusLabel(run.status)}
                  </span>
                  <span className="text-ink-3">
                    {[providerDisplayName(run.provider_type), run.runtime_node_summary]
                      .filter(Boolean)
                      .join(" · ")}
                  </span>
                  <Link
                    aria-label={`查看${task.title}执行轨迹`}
                    className="inline-flex items-center gap-0.5 text-[12px] font-semibold text-brand hover:opacity-80"
                    params={{ projectId }}
                    search={{ tab: "trace", task: task.id }}
                    to="/projects/$projectId"
                  >
                    查看执行轨迹
                    <ArrowUpRight className="size-3" />
                  </Link>
                </>
              ) : (
                <span className="text-ink-3">暂无运行记录</span>
              )}
            </div>
            {run?.started_at ? (
              <p className="text-[12px] tabular-nums text-ink-2">
                <span className="text-ink-3">起</span>{" "}
                <time dateTime={run.started_at}>{formatDateTime(run.started_at)}</time>
                {run.finished_at ? (
                  <>
                    {" "}
                    <span className="text-ink-3">止</span>{" "}
                    <time dateTime={run.finished_at}>
                      {formatDateTime(run.finished_at)}
                    </time>
                  </>
                ) : null}
                {formatRunDuration(run.started_at, run.finished_at) ? (
                  <>
                    {" "}
                    <span className="text-ink-3">耗时</span>{" "}
                    {formatRunDuration(run.started_at, run.finished_at)}
                  </>
                ) : null}
              </p>
            ) : null}
            {run?.error_message ? (
              <RunErrorMessage message={run.error_message} />
            ) : null}
            {result ? (
              <blockquote className="border-l-2 border-brand/40 pl-3 text-[13px] leading-6 text-ink">
                {result.conclusion}
              </blockquote>
            ) : (
              <p className="text-[12.5px] text-ink-3">暂无执行结论</p>
            )}
          </section>

          <section className="space-y-2" data-testid="task-detail-decisions">
            <div className="flex items-center justify-between gap-2">
              <SectionEyebrow
                icon={<Inbox />}
                label={
                  pendingDecisions.length > 0
                    ? `待决事项 · ${pendingDecisions.length}`
                    : "待决事项"
                }
              />
              <Link
                aria-label="前往收件箱处理"
                className="inline-flex items-center gap-1 text-[12px] font-semibold text-brand hover:opacity-80"
                to="/inbox"
              >
                收件箱
                <ArrowUpRight className="size-3.5" />
              </Link>
            </div>
            {orphanHumanWait ? (
              <div
                className="rounded-[10px] bg-warn/10 px-3 py-2.5 shadow-[inset_2px_0_0_var(--warn)]"
                data-testid="task-detail-orphan-human-wait"
              >
                <p className="text-[12.5px] font-semibold leading-5 text-ink">
                  任务停在「等待人工」，但没有关联的待决决策卡
                </p>
                <p className="mt-1 text-[12px] leading-5 text-ink-2">
                  常见原因：派发闸门阻塞、决策已处理但任务未释放、或等待请求未挂到本任务。
                  {hasBlocker ? ` 当前编排阻塞：${blockerText}。` : " 可在编排区与执行轨迹核对原因。"}
                </p>
                <div className="mt-2 flex flex-wrap gap-2">
                  <Link
                    className="inline-flex items-center gap-1 text-[12px] font-semibold text-brand hover:opacity-80"
                    params={{ projectId }}
                    search={{ tab: "trace", task: task.id }}
                    to="/projects/$projectId"
                  >
                    查看执行轨迹
                    <ArrowUpRight className="size-3.5" />
                  </Link>
                  <Link
                    className="inline-flex items-center gap-1 text-[12px] font-semibold text-brand hover:opacity-80"
                    to="/inbox"
                  >
                    去收件箱核对
                    <ArrowUpRight className="size-3.5" />
                  </Link>
                </div>
              </div>
            ) : pendingDecisions.length === 0 ? (
              <p className="text-[12.5px] text-ink-3">暂无待决事项</p>
            ) : (
              <ul className="space-y-2">
                {pendingDecisions.map((decision) => (
                  <li
                    className="rounded-[10px] bg-card-soft px-3 py-2.5 shadow-[inset_2px_0_0_var(--warn)]"
                    key={decision.id}
                  >
                    <p className="text-[12.5px] font-bold leading-5 text-ink">
                      {decision.title_snapshot}
                    </p>
                    {decision.summary_snapshot &&
                    decision.summary_snapshot !== decision.title_snapshot ? (
                      <p className="mt-0.5 line-clamp-2 text-[11.5px] leading-4 text-ink-2">
                        {decision.summary_snapshot}
                      </p>
                    ) : null}
                    {decision.created_at ? (
                      <time
                        className="mt-1 block text-[11px] tabular-nums text-ink-3"
                        dateTime={decision.created_at}
                        title={decision.created_at}
                      >
                        {formatRelativeTime(decision.created_at)}
                      </time>
                    ) : null}
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
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}

/** 失败 run 的错误摘要（服务端已脱敏）：默认三行 clamp，长文本可展开。 */
function RunErrorMessage({ message }: { message: string }) {
  const [expanded, setExpanded] = useState(false);
  const isLong = message.length > 160 || message.split("\n").length > 3;

  return (
    <div
      className="rounded-[10px] bg-danger/5 px-3 py-2.5 shadow-[inset_2px_0_0_var(--danger)]"
      data-testid="task-run-error"
    >
      <div className="text-[10.5px] font-bold text-ink-3">错误摘要</div>
      <p
        className={cn(
          "mt-1 whitespace-pre-wrap break-words text-[12px] leading-5 text-ink",
          !expanded && "line-clamp-3",
        )}
      >
        {message}
      </p>
      {isLong ? (
        <button
          className="mt-1 text-[11.5px] font-semibold text-brand hover:opacity-80"
          onClick={() => setExpanded((current) => !current)}
          type="button"
        >
          {expanded ? "收起" : "展开全文"}
        </button>
      ) : null}
    </div>
  );
}

function SectionEyebrow({ icon, label }: { icon: ReactNode; label: string }) {
  return (
    <h4 className="flex items-center gap-1.5 text-[11px] font-extrabold text-ink-3 [&>svg]:size-3.5">
      {icon}
      {label}
    </h4>
  );
}

function FieldChips({ chips, label }: { chips: string[]; label: string }) {
  return (
    <div>
      <div className="mb-1 text-[10.5px] font-bold text-ink-3">{label}</div>
      {chips.length === 0 ? (
        <p className="text-[12px] text-ink-3">暂无</p>
      ) : (
        <div className="flex flex-wrap gap-1">
          {chips.map((chip, index) => (
            <span
              className="max-w-full break-words rounded-md bg-card-soft px-1.5 py-0.5 text-[11.5px] leading-5 text-ink-2"
              key={`${chip}-${index}`}
            >
              {chip}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

function leafText(item: unknown): string {
  if (item === undefined || item === null || item === "") return "";
  if (typeof item === "string") return item;
  if (typeof item === "number" || typeof item === "boolean") return String(item);
  try {
    return JSON.stringify(item);
  } catch {
    return "无法显示";
  }
}

/** 值为空（含空数组/空对象）时视为无内容，不产出 chip。 */
function isEmptyValue(value: unknown): boolean {
  if (value === undefined || value === null || value === "") return true;
  if (Array.isArray(value)) return value.length === 0;
  if (typeof value === "object") return Object.keys(value).length === 0;
  return false;
}

/** 数组/对象字段拆成短 chips；空结构不裸漏（`foo: []` 一律折成「暂无」）。 */
function valueChips(value: unknown): string[] {
  if (isEmptyValue(value)) return [];
  if (Array.isArray(value)) {
    return value.filter((item) => !isEmptyValue(item)).map(leafText);
  }
  if (value && typeof value === "object") {
    return Object.entries(value)
      .filter(([, item]) => !isEmptyValue(item))
      .map(([key, item]) => `${taskFieldKeyLabel(key)}：${listText(item)}`);
  }
  return [leafText(value)];
}

/** 数组值渲染为条目列表（顿号衔接），标量沿用 leafText；避免 JSON 数组直出。 */
function listText(value: unknown): string {
  if (Array.isArray(value)) {
    const items = value.filter((item) => !isEmptyValue(item)).map(leafText);
    return items.length > 0 ? items.join("、") : "暂无";
  }
  return leafText(value);
}

/** 交接契约按顶层键逐行展示，避免多键拼在一行难读。 */
function contractEntries(value: unknown): Array<{ label?: string; text: string }> {
  if (isEmptyValue(value)) return [];
  if (Array.isArray(value)) {
    return value
      .filter((item) => !isEmptyValue(item))
      .map((item) => ({ text: leafText(item) }));
  }
  if (value && typeof value === "object") {
    return Object.entries(value)
      .filter(([, item]) => !isEmptyValue(item))
      .map(([key, item]) => ({ label: taskFieldKeyLabel(key), text: listText(item) }));
  }
  return [{ text: leafText(value) }];
}

function runDotClass(status: string): string {
  switch (status) {
    case "completed":
      return "bg-ok";
    case "failed":
    case "timed_out":
      return "bg-danger";
    case "running":
    case "dispatching":
    case "cancelling":
      return "bg-warn";
    default:
      return "bg-ink-3/60";
  }
}
