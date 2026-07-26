import { lazy, Suspense, useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowUpRight, ClipboardList, FileText, Inbox } from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  Button,
  EmptyState,
  IconTile,
  LoadingState,
  notifyError,
  notifySuccess,
  SoftCard,
  StatusPill
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  getProjectDemandLaunchDetail,
  resolveProjectDecision,
  type ProjectDemand,
  type ProjectEvent,
  type ProjectTaskGraph
} from "@/lib/api/projects";
import { decisionStatusLabel, demandStatusLabel } from "@/lib/status-labels";
import { formatRelativeTime } from "@/lib/format-time";
import { taskIdFromNodeId, taskNodeId } from "@/features/flow-graph/flow-graph-adapter";
import { DemandCriteriaPanel } from "./demand-criteria-panel";
import { StaffGapDialog } from "./staff-gap-dialog";

// 与工作台执行图同一权威画布（@xyflow/react 重依赖，懒加载同一 chunk）。
const FlowGraphCanvas = lazy(() =>
  import("@/features/flow-graph/flow-graph-canvas").then((m) => ({
    default: m.FlowGraphCanvas
})),
);

type ProjectDemandsSectionProps = {
  apiBaseUrl: string;
  apiOptions: ApiClientOptions;
  demands: ProjectDemand[];
  /** 当前打开的任务详情弹层任务 id（画布选中态与弹层同源）。 */
  detailTaskId?: string;
  /** 项目事件（父页已拉取）；用于派发闸阻塞横幅（原流程编排详情职责迁入）。 */
  events?: ProjectEvent[];
  /** 按 demand 拉执行图；queryKey 与页面预载同族（project-task-graph）复用缓存。 */
  fetchTaskGraph?: (demandId: string) => Promise<ProjectTaskGraph>;
  onClearTask?: () => void;
  onOpenTask: (taskId: string) => void;
  projectId: string;
  /** ?demand= 深链选中的需求；缺省回退最新需求。 */
  selectedDemandId?: string;
};

function demandStatusTone(status: string) {
  if (status === "completed") return "ok" as const;
  if (status === "failed" || status === "cancelled") return "danger" as const;
  if (status === "acceptance_pending") return "warn" as const;
  if (status === "executing" || status === "planned") return "info" as const;
  return "mute" as const;
}

/**
 * 项目详情「需求流程」区（IA Phase 2 P2a-1）：左侧需求切换器 + 右侧选中需求的
 * 状态头 / 权威流程图 / 验收血缘 / 待决面板。选中态由 ?demand= 查询参数持久化，
 * 无参数默认最新需求；切换用 Link 改 URL，数据经 keepPreviousData 不闪空。
 */
export function ProjectDemandsSection({
  apiBaseUrl,
  apiOptions,
  demands,
  detailTaskId,
  events,
  fetchTaskGraph,
  onClearTask,
  onOpenTask,
  projectId,
  selectedDemandId
}: ProjectDemandsSectionProps) {
  const selectedDemand =
    demands.find((demand) => demand.id === selectedDemandId) ?? demands[0];

  const graphQuery = useQuery({
    enabled: Boolean(selectedDemand && fetchTaskGraph),
    placeholderData: keepPreviousData,
    queryFn: () => fetchTaskGraph!(selectedDemand!.id),
    // 与页面预载/任务弹层同 key 族：最新需求直接命中缓存。
    queryKey: ["project-task-graph", projectId, selectedDemand?.id]
});
  const graph = graphQuery.data;

  // 需求维度的待决事项：launch detail 由服务端按 demand 收敛（协调 job + 任务双路），
  // 覆盖 plan_review 等没有 project_task_id 的需求级决策。key 与流程编排详情共享缓存。
  const launchDetailQuery = useQuery({
    enabled: Boolean(selectedDemand),
    placeholderData: keepPreviousData,
    queryFn: () => getProjectDemandLaunchDetail(apiOptions, selectedDemand!.id),
    queryKey: ["workflow-detail", apiBaseUrl, selectedDemand?.id]
});
  const launchDetail =
    launchDetailQuery.data?.demand.id === selectedDemand?.id
      ? launchDetailQuery.data
      : undefined;
  const pendingDecisions = (launchDetail?.decision_requests ?? []).filter(
    (decision) => decision.status_snapshot === "pending",
  );

  const taskNamesById = useMemo(
    () => new Map((graph?.nodes ?? []).map((node) => [node.id, node.title])),
    [graph],
  );

  if (demands.length === 0) {
    return (
      <SoftCard className="p-8">
        <EmptyState
          description="向项目提交需求后，这里按需求查看权威流程图、验收血缘与待决事项。"
          icon={<FileText />}
          title="暂无需求"
        />
      </SoftCard>
    );
  }

  return (
    <div
      className="grid min-w-0 items-start gap-4 lg:grid-cols-[260px_minmax(0,1fr)]"
      data-testid="project-demands-section"
    >
      <SoftCard className="overflow-hidden lg:sticky lg:top-4">
        <div className="border-b border-line px-4 py-3">
          <h3 className="text-sm font-semibold text-ink">需求</h3>
          <p className="mt-0.5 text-[11.5px] text-ink-3">{demands.length} 条 · 最新在前</p>
        </div>
        <nav aria-label="需求列表" className="max-h-[520px] divide-y divide-line overflow-y-auto">
          {demands.map((demand) => {
            const isSelected = demand.id === selectedDemand?.id;
            return (
              <Link
                aria-current={isSelected ? "true" : undefined}
                className={cn(
                  "block px-4 py-3 transition-colors hover:bg-card-soft",
                  isSelected && "bg-brand-soft shadow-[inset_2px_0_0_var(--brand)]",
                )}
                data-testid={`demand-list-item-${demand.id}`}
                key={demand.id}
                params={{ projectId }}
                search={{ demand: demand.id, tab: "demands" }}
                to="/projects/$projectId"
              >
                <p
                  className={cn(
                    "truncate text-[13px] font-semibold",
                    isSelected ? "text-brand-deep" : "text-ink",
                  )}
                >
                  {demand.title}
                </p>
                <div className="mt-1 flex flex-wrap items-center gap-1.5">
                  <StatusPill tone={demandStatusTone(demand.status)}>
                    {demandStatusLabel(demand.status)}
                  </StatusPill>
                </div>
              </Link>
            );
          })}
        </nav>
      </SoftCard>

      {selectedDemand ? (
        <div className="grid min-w-0 gap-4">
          {/* coordination.blocked 横幅 + 缺口面板只在需求终态 failed 时渲染（原流程编排详情
              的语义原样迁入）：reopen 重规划后 demand 会先回非终态，旧 blocking_facts 未必
              已随重规划清掉，此时继续渲染红色阻塞条会误导负责人。demand.status 是权威来源。 */}
          {selectedDemand.status === "failed" ? (
            <>
              <DemandBlockingBanner graph={graph} />
              <DemandGapPanel
                apiOptions={apiOptions}
                graph={graph}
                projectId={projectId}
              />
            </>
          ) : null}
          <DemandDispatchBlockerBanner
            event={currentDemandDispatchBlocker(events, graph, selectedDemand.id)}
          />
          <SoftCard className="p-4" data-testid="demand-status-header">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="min-w-0 truncate text-base font-semibold text-ink">
                    {selectedDemand.title}
                  </h3>
                  <StatusPill tone={demandStatusTone(selectedDemand.status)}>
                    {demandStatusLabel(selectedDemand.status)}
                  </StatusPill>
                </div>
                {selectedDemand.content &&
                selectedDemand.content !== selectedDemand.title ? (
                  <p className="mt-1.5 line-clamp-3 text-[13px] leading-6 text-ink-2">
                    {selectedDemand.content}
                  </p>
                ) : null}
              </div>
            </div>
          </SoftCard>

          <section className="grid gap-2" data-testid="demand-flow-graph-section">
            <div className="flex items-center gap-2 px-1">
              <ClipboardList className="size-4 text-ink-2" />
              <h3 className="text-sm font-semibold tracking-normal">权威流程图</h3>
            </div>
            {graph && graph.nodes.length > 0 ? (
              <Suspense fallback={<LoadingState />}>
                <FlowGraphCanvas
                  graph={graph}
                  onNodeOpen={(nodeId) => {
                    const taskId = taskIdFromNodeId(nodeId);
                    if (taskId) onOpenTask(taskId);
                  }}
                  onSelectedNodeChange={(nodeId) => {
                    if (!nodeId) onClearTask?.();
                  }}
                  selectedNodeId={detailTaskId ? taskNodeId(detailTaskId) : undefined}
                />
              </Suspense>
            ) : graphQuery.isFetching || graphQuery.isLoading ? (
              <LoadingState label="正在加载执行图…" />
            ) : (
              <SoftCard className="p-6">
                <EmptyState
                  description={
                    graphQuery.isError
                      ? "执行图加载失败，可稍后重试。"
                      : "该需求还没有生成执行任务图（可能仍在规划中）。"
                  }
                  title="暂无执行图"
                />
              </SoftCard>
            )}
          </section>

          <DemandCriteriaPanel
            apiBaseUrl={apiBaseUrl}
            apiOptions={apiOptions}
            demandId={selectedDemand.id}
            taskNamesById={taskNamesById}
          />

          <SoftCard className="p-4" data-testid="demand-pending-decisions">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <Inbox className="size-4 text-ink-2" />
                <h3 className="text-sm font-semibold text-ink">
                  {pendingDecisions.length > 0
                    ? `待决事项 · ${pendingDecisions.length}`
                    : "待决事项"}
                </h3>
              </div>
              <Link
                aria-label="前往收件箱处理待决事项"
                className="inline-flex items-center gap-1 text-[12px] font-semibold text-brand hover:opacity-80"
                to="/inbox"
              >
                收件箱
                <ArrowUpRight className="size-3.5" />
              </Link>
            </div>
            {pendingDecisions.length === 0 ? (
              <p className="mt-2 text-[12.5px] text-ink-3">
                {launchDetail
                  ? "该需求当前没有待决事项"
                  : launchDetailQuery.isError
                    ? "待决事项加载失败，可稍后重试"
                    : "正在加载待决事项…"}
              </p>
            ) : (
              <ul className="mt-3 space-y-2">
                {pendingDecisions.map((decision) => (
                  <li
                    className="rounded-[10px] bg-card-soft px-3 py-2.5 shadow-[inset_2px_0_0_var(--warn)]"
                    data-testid={`demand-pending-decision-${decision.id}`}
                    key={decision.id}
                  >
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <p className="min-w-0 text-[12.5px] font-bold leading-5 text-ink">
                        {decision.title_snapshot}
                      </p>
                      <StatusPill tone="warn">
                        {decisionStatusLabel(decision.status_snapshot)}
                      </StatusPill>
                    </div>
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
                  </li>
                ))}
              </ul>
            )}
          </SoftCard>
        </div>
      ) : null}
    </div>
  );
}

// ———— 以下为原 features/workflows/index.tsx 的阻塞横幅/缺口面板/派发闸横幅，
// 随流程编排页退役（IA Phase 2 P2c）原语义迁入需求维度。 ————

function DemandBlockingBanner({ graph }: { graph: ProjectTaskGraph | undefined }) {
  const fact = graph?.blocking_facts[0];
  if (!fact) return null;

  return (
    <SoftCard className="border-danger/25 bg-danger/5 p-4" data-testid="demand-blocking-banner">
      <div className="flex flex-col gap-2 text-sm leading-6 text-ink sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="font-bold">协调已阻塞：{fact.message}</p>
          {fact.recommended_action ? (
            <p className="mt-1 text-ink-2">下一步：{fact.recommended_action}</p>
          ) : null}
        </div>
        <StatusPill className="shrink-0 self-start" tone="danger">
          {fact.reason_code}
        </StatusPill>
      </div>
    </SoftCard>
  );
}

/**
 * 规划缺口面板：coordination.blocked 携带结构化 gap（planning_gap 决策通道）时，
 * 给出一键补员与豁免约束重规划两条处置动作；无 gap 不渲染。
 */
function DemandGapPanel({
  apiOptions,
  graph,
  projectId
}: {
  apiOptions: ApiClientOptions;
  graph: ProjectTaskGraph | undefined;
  projectId: string;
}) {
  const queryClient = useQueryClient();
  const [staffDialogOpen, setStaffDialogOpen] = useState(false);
  const [exemptDialogOpen, setExemptDialogOpen] = useState(false);
  const fact = graph?.blocking_facts[0];
  const gap = fact?.gap;
  const decisionRequestId = fact?.decision_request_id;

  const exemptMutation = useMutation({
    mutationFn: () => {
      if (!decisionRequestId) {
        throw new Error("缺少决策 ID，无法豁免");
      }
      return resolveProjectDecision(apiOptions, projectId, decisionRequestId, {
        decision: "exempted"
});
    },
    onError: (error: unknown) => {
      notifyError(error instanceof Error ? error.message : "豁免失败");
    },
    onSuccess: async () => {
      notifySuccess("已豁免约束，重新规划已触发");
      setExemptDialogOpen(false);
      await invalidateDemandGapQueries(queryClient);
    }
});

  if (!gap) return null;

  return (
    <SoftCard className="p-4" data-testid="demand-gap-panel">
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start gap-3">
          <IconTile tone="warn">
            <span aria-hidden className="text-sm font-bold">
              缺
            </span>
          </IconTile>
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-ink">
              规划缺口：{gapConstraintLabel(gap.constraint_kind)}
            </p>
            <p className="mt-1 text-xs leading-5 text-ink-2">
              涉及角色：{gap.roles.length > 0 ? gap.roles.join("、") : "—"} · 当前可调度员工{" "}
              {gap.active_executor_count} 名
            </p>
            {gap.required_capabilities.length > 0 ? (
              <p className="text-xs leading-5 text-ink-2">
                所需能力：{gap.required_capabilities.join("、")}
              </p>
            ) : null}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            disabled={!decisionRequestId}
            onClick={() => setStaffDialogOpen(true)}
            variant="primary"
          >
            从标准模板补员
          </Button>
          <Button
            disabled={!decisionRequestId}
            onClick={() => setExemptDialogOpen(true)}
            variant="outline"
          >
            豁免并重规划
          </Button>
        </div>
      </div>
      {decisionRequestId ? (
        <StaffGapDialog
          apiOptions={apiOptions}
          decisionRequestId={decisionRequestId}
          gap={gap}
          onOpenChange={setStaffDialogOpen}
          onStaffed={() => {
            void invalidateDemandGapQueries(queryClient);
          }}
          open={staffDialogOpen}
          projectId={projectId}
        />
      ) : null}
      <ConfirmDialog
        cancelBtnText="取消"
        confirmText="确认豁免"
        desc="豁免后，审查独立性等约束将对该需求不再生效，同一数字员工可能身兼多个角色（如既实现又审查）。该操作会记录为人类负责人的一等决策，并立即触发重新规划。"
        destructive
        handleConfirm={() => exemptMutation.mutate()}
        isLoading={exemptMutation.isPending}
        onOpenChange={setExemptDialogOpen}
        open={exemptDialogOpen}
        title="豁免约束并重新规划"
      />
    </SoftCard>
  );
}

function invalidateDemandGapQueries(
  queryClient: ReturnType<typeof useQueryClient>,
): Promise<unknown> {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: ["project-task-graph"] }),
    queryClient.invalidateQueries({ queryKey: ["workflow-detail"] }),
    queryClient.invalidateQueries({ queryKey: ["project-events"] }),
  ]);
}

function gapConstraintLabel(constraintKind: string): string {
  if (constraintKind === "role_independence") return "审查独立性约束";
  return constraintKind || "结构性约束";
}

function DemandDispatchBlockerBanner({ event }: { event: ProjectEvent | undefined }) {
  if (!event) return null;
  const label = demandDispatchBlockerLabel(event.event_type);

  return (
    <SoftCard className="border-warn/25 bg-warn/5 p-4" data-testid="demand-dispatch-blocker">
      <div className="flex flex-col gap-2 text-sm leading-6 text-ink sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="font-bold">{label.title}</p>
          <p className="mt-1 text-ink-2">{label.summary}</p>
        </div>
        <StatusPill className="shrink-0 self-start" tone={label.tone}>
          {event.event_type}
        </StatusPill>
      </div>
    </SoftCard>
  );
}

function currentDemandDispatchBlocker(
  events: ProjectEvent[] | undefined,
  graph: ProjectTaskGraph | undefined,
  selectedDemandId: string | undefined,
): ProjectEvent | undefined {
  const taskIds = new Set(graph?.nodes.map((node) => node.id) ?? []);
  const matchingEvents = events?.filter((event) => {
    if (!isDemandDispatchBlockerEvent(event.event_type)) return false;
    const taskId =
      typeof event.payload.project_task_id === "string"
        ? event.payload.project_task_id
        : typeof event.payload.task_id === "string"
          ? event.payload.task_id
          : undefined;
    const eventDemandId =
      typeof event.payload.demand_id === "string" ? event.payload.demand_id : undefined;
    if (taskId && taskIds.has(taskId)) return true;
    return Boolean(eventDemandId && eventDemandId === selectedDemandId);
  });
  return matchingEvents?.sort(
    (a, b) =>
      demandDispatchBlockerPriority(a.event_type) -
      demandDispatchBlockerPriority(b.event_type),
  )[0];
}

function isDemandDispatchBlockerEvent(eventType: ProjectEvent["event_type"]) {
  return (
    eventType === "project_task.dispatch_gate.replan_required" ||
    eventType === "project_task.dispatch_gate.blocked" ||
    eventType === "project_task.dispatch_gate.retry_later" ||
    eventType === "project_task.dispatch_gate.waiting_human" ||
    eventType === "project_task.dispatch_blocked"
  );
}

function demandDispatchBlockerPriority(eventType: ProjectEvent["event_type"]) {
  if (eventType === "project_task.dispatch_gate.replan_required") return 0;
  if (eventType === "project_task.dispatch_gate.blocked") return 1;
  if (eventType === "project_task.dispatch_gate.waiting_human") return 2;
  if (eventType === "project_task.dispatch_gate.retry_later") return 3;
  return 4;
}

function demandDispatchBlockerLabel(eventType: ProjectEvent["event_type"]): {
  summary: string;
  title: string;
  tone: "danger" | "warn";
} {
  if (eventType === "project_task.dispatch_gate.replan_required") {
    return {
      summary: "当前计划不再满足执行条件，需要重新编排后继续。",
      title: "计划需要调整",
      tone: "danger"
};
  }

  if (eventType === "project_task.dispatch_gate.waiting_human") {
    return {
      summary: "当前执行需要负责人确认后继续。",
      title: "等待负责人确认",
      tone: "warn"
};
  }

  if (eventType === "project_task.dispatch_gate.retry_later") {
    return {
      summary: "运行条件暂不可用，系统会稍后重试。",
      title: "稍后重试执行",
      tone: "warn"
};
  }

  return {
    summary: "当前执行条件未满足，系统已记录阻塞原因。",
    title: "执行条件未满足",
    tone: "danger"
};
}
