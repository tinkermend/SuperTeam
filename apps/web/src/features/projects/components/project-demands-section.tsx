import { lazy, Suspense, useCallback, useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ClipboardList, FileText } from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  Button,
  EmptyState,
  ErrorState,
  IconTile,
  LoadingState,
  MasterDetailLayout,
  notifyError,
  notifySuccess,
  SoftCard,
  StatusPill
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  getProjectDemandDossier,
  resolveProjectDecision,
  type ProjectDemand,
  type ProjectDemandDossier,
  type ProjectTaskGraph,
  type ProjectTaskGraphDispatchGate
} from "@/lib/api/projects";
import { demandStatusLabel } from "@/lib/status-labels";
import { formatRelativeTime } from "@/lib/format-time";
import { isTerminalTaskStatus } from "@/lib/task-status";
import { taskIdFromNodeId, taskNodeId } from "@/features/flow-graph/flow-graph-adapter";
import { useProjectActivityInvalidate } from "../hooks/use-project-activity-invalidate";
import { DemandCriteriaPanel } from "./demand-criteria-panel";
import {
  readStoredDossierDensity,
  resolveDossierDensity,
  writeStoredDossierDensity,
  type DossierDensity
} from "./demand-dossier-density";
import {
  DemandDossierHeader,
  demandStatusTone,
  type DemandDossierView
} from "./demand-dossier-header";
import { DemandDossierRail } from "./demand-dossier-rail";
import { DemandDossierTimeline } from "./demand-dossier-timeline";
import { StaffGapDialog } from "./staff-gap-dialog";
import { DemandContinueDialog } from "./demand-continue-dialog";
import { findChainOf, foldDemandChains } from "./demand-chains";

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
  /** 测试注入用（SSE 活动流）；生产默认用带凭据的原生 EventSource。 */
  eventSourceFactory?: (url: string) => EventSource;
  /** 按 demand 拉执行图；queryKey 与页面预载同族（project-task-graph）复用缓存。 */
  fetchTaskGraph?: (demandId: string) => Promise<ProjectTaskGraph>;
  onClearTask?: () => void;
  onOpenTask: (taskId: string) => void;
  projectId: string;
  /** ?demand= 深链选中的需求；缺省回退最新需求。 */
  selectedDemandId?: string;
  /** ?view= 中栏视图；缺省时间线（叙事优先，图为副视图）。 */
  view?: DemandDossierView;
  onViewChange?: (view: DemandDossierView) => void;
};

/**
 * 卷宗内已补名的任务显示名字典，供验收面板展示"哪个任务满足了判据"。
 * 服务端已在读路径补名，这里只是把两处已有的名字归并，不做逐行请求。
 */
function dossierTaskNames(dossier: ProjectDemandDossier): Map<string, string> {
  const names = new Map<string, string>();
  for (const assessment of dossier.handoff_summary.assessments) {
    if (assessment.project_task_name) {
      names.set(assessment.project_task_id, assessment.project_task_name);
    }
  }
  for (const slot of dossier.rail.slots) {
    for (const item of slot.items) {
      if (item.project_task_id && item.project_task_name) {
        names.set(item.project_task_id, item.project_task_name);
      }
    }
  }
  return names;
}

/**
 * 项目详情「这一单」处所（spec 2026-07-29 R2 一单卷宗）。
 *
 * 左：本项目需求列表（带待你处理角标）；中：协调时间线为主叙事、权威流程图可切；
 * 右：按剧本 produces kind 分槽的交付事实轨 + 交付判定 + 验收摘要。
 *
 * 数据来自 demand 级只读聚合 `getProjectDemandDossier`——时间线归一、补名、
 * 剧本解析都在服务端做完，前端不解析原始 event_type、不拼多接口。
 * 图视图继续独立拉 task-graph（与页面预载共享缓存 key）。
 */
export function ProjectDemandsSection({
  apiBaseUrl,
  apiOptions,
  demands,
  detailTaskId,
  eventSourceFactory,
  fetchTaskGraph,
  onClearTask,
  onOpenTask,
  projectId,
  selectedDemandId,
  view = "timeline",
  onViewChange
}: ProjectDemandsSectionProps) {
  const selectedDemand =
    demands.find((demand) => demand.id === selectedDemandId) ?? demands[0];
  const navigate = useNavigate();
  const sectionQueryClient = useQueryClient();
  const [continueOpen, setContinueOpen] = useState(false);
  // 左轨按接续链折叠：一条链只占一行（spec 2026-08-01 §8.2）。不折叠的话，
  // 每接续一次列表就多出一行看似无关的单。
  const chains = useMemo(() => foldDemandChains(demands), [demands]);
  const selectedChain = findChainOf(chains, selectedDemand?.id);
  const goToDemand = useCallback(
    (demandId: string) => {
      void navigate({
        params: { projectId },
        search: (prev: Record<string, unknown>) => ({ ...prev, demand: demandId, tab: "demands" }),
        to: "/projects/$projectId",
      });
    },
    [navigate, projectId],
  );

  // 数据活性升级（spec 2026-07-27 §5 P2-E）：主通道是既有跨员工活动 SSE 的
  // 项目维度事件驱动 invalidate；组件测试注入 fetcher 时默认不开真实流（避免
  // 连不上的重连噪音，run-overview 先例），显式给 factory 则照常开。
  useProjectActivityInvalidate({
    apiBaseUrl,
    enabled: !(apiOptions.fetcher && !eventSourceFactory),
    eventSourceFactory,
    projectId
});

  const dossierQuery = useQuery({
    enabled: Boolean(selectedDemand),
    placeholderData: keepPreviousData,
    queryFn: () =>
      getProjectDemandDossier(apiOptions, selectedDemand!.id, { siblingPending: true }),
    queryKey: ["demand-dossier", apiBaseUrl, selectedDemand?.id],
    // 兜底轮询只在 drive 密度下开：卷宗是重聚合（launch facts + 交接判定 +
    // 证据/工件 + 补名），巡检态还每 30s 重拉一遍纯属浪费。秒级活性由 SSE
    // invalidate 承担，这里只兜 SSE 不可用的情况。
    refetchInterval: (query) => {
      const signals = query.state.data?.signals;
      return resolveDossierDensity(signals, undefined) === "drive" ? 30_000 : false;
    }
});
  // 只认"确实是当前这一单"的响应（切单期间 keepPreviousData 会留着上一单的数据）。
  // 对缺字段的响应做保底填充：卷宗是只读读模型，半截 payload 不该把整页打崩。
  const dossier = useMemo(() => {
    const data = dossierQuery.data;
    if (!data?.demand?.id || data.demand.id !== selectedDemand?.id) {
      return undefined;
    }
    return {
      ...data,
      handoff_summary: data.handoff_summary ?? {
        assessments: [],
        fulfilled: 0,
        partial: 0,
        unfulfilled: 0,
        unknown: 0
},
      pending_actions: data.pending_actions ?? [],
      rail: data.rail ?? { slots: [] },
      timeline: data.timeline ?? { items: [], truncated: false }
};
  }, [dossierQuery.data, selectedDemand?.id]);

  const [densityOverride, setDensityOverride] = useState<DossierDensity | undefined>(() =>
    readStoredDossierDensity(typeof window === "undefined" ? undefined : window.localStorage),
  );
  const density = resolveDossierDensity(dossier?.signals, densityOverride);
  const handleDensityChange = (next: DossierDensity) => {
    setDensityOverride(next);
    writeStoredDossierDensity(
      typeof window === "undefined" ? undefined : window.localStorage,
      next,
    );
  };

  // 图数据保持常拉（与改版前一致）：除图视图外，派发闸横幅、阻塞横幅与规划缺口
  // 面板都读 graph.blocking_facts / dispatch_gates。改成"仅图视图才拉"会让默认
  // 时间线视图下这些告警静默消失——省一次请求换掉一条告警不是优化。
  const graphQuery = useQuery({
    enabled: Boolean(selectedDemand && fetchTaskGraph),
    placeholderData: keepPreviousData,
    queryFn: () => fetchTaskGraph!(selectedDemand!.id),
    // 与页面预载/任务弹层同 key 族：最新需求直接命中缓存。
    queryKey: ["project-task-graph", projectId, selectedDemand?.id],
    refetchInterval: 30_000
});
  const graph = graphQuery.data;

  const pendingByDemand = useMemo(() => {
    const map = new Map<string, number>();
    for (const sibling of dossier?.sibling_pending ?? []) {
      map.set(sibling.demand_id, sibling.open_decisions);
    }
    return map;
  }, [dossier]);

  const [acceptanceOpen, setAcceptanceOpen] = useState(false);

  if (demands.length === 0) {
    return (
      <SoftCard className="p-8">
        <EmptyState
          description="向项目提交需求后，这里按需求查看协调时间线、交付事实与待你处理事项。"
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
          <p className="mt-0.5 text-[11.5px] text-ink-3">{chains.length} 条 · 最新在前</p>
        </div>
        <nav aria-label="需求列表" className="max-h-[520px] divide-y divide-line overflow-y-auto">
          {chains.map((chain) => {
            // 行代表整条链，落点是链上最新一单；选中链内任一单都算本行选中。
            const demand = chain.latest;
            const isSelected = chain === selectedChain;
            const continuationCount = chain.members.length - 1;
            const pendingCount = chain.members.reduce(
              (total, member) => total + (pendingByDemand.get(member.id) ?? 0),
              0,
            );
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
                <div className="flex items-start justify-between gap-2">
                  <p
                    className={cn(
                      "min-w-0 flex-1 truncate text-[13px] font-semibold",
                      isSelected ? "text-brand-deep" : "text-ink",
                    )}
                  >
                    {demand.title}
                  </p>
                  {pendingCount > 0 ? (
                    <span
                      aria-label={`待你处理 ${pendingCount} 项`}
                      className="shrink-0 rounded-full bg-warn-soft px-1.5 py-0.5 text-[10.5px] font-semibold tabular-nums text-warn-text"
                      data-testid={`demand-list-pending-${demand.id}`}
                    >
                      {pendingCount}
                    </span>
                  ) : null}
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-1.5">
                  <StatusPill tone={demandStatusTone(demand.status)}>
                    {demandStatusLabel(demand.status)}
                  </StatusPill>
                  {continuationCount > 0 ? (
                    <span
                      className="rounded-full bg-card-soft px-1.5 py-0.5 text-[10.5px] text-ink-3"
                      data-testid={`demand-list-continuations-${demand.id}`}
                    >
                      接续 {continuationCount} 次
                    </span>
                  ) : null}
                  {demand.created_at ? (
                    <time
                      className="text-[11px] tabular-nums text-ink-3"
                      dateTime={demand.created_at}
                      title={demand.created_at}
                    >
                      {formatRelativeTime(demand.created_at)}
                    </time>
                  ) : null}
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
              已随重规划清掉，此时继续渲染红色阻塞条会误导负责人。demand.status 是权威来源。
              这两块读 task-graph，故在图视图外也需要图数据时按需拉取。 */}
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
          <DemandDispatchBlockerBanner gate={currentDemandDispatchBlocker(graph)} />

          {dossierQuery.isError ? (
            <SoftCard className="p-6" data-testid="demand-dossier-error">
              <ErrorState
                description="一单卷宗加载失败，可稍后重试。"
                onRetry={() => void dossierQuery.refetch()}
                title="卷宗加载失败"
              />
            </SoftCard>
          ) : !dossier ? (
            <LoadingState label="正在加载这一单…" />
          ) : (
            <MasterDetailLayout
              detail={
                <DemandDossierRail
                  acceptance={dossier.acceptance}
                  acceptanceDetail={
                    <DemandCriteriaPanel
                      apiBaseUrl={apiBaseUrl}
                      apiOptions={apiOptions}
                      demandId={selectedDemand.id}
                      taskNamesById={dossierTaskNames(dossier)}
                    />
                  }
                  acceptanceOpen={acceptanceOpen}
                  handoffSummary={dossier.handoff_summary}
                  onAcceptanceToggle={() => setAcceptanceOpen((value) => !value)}
                  slots={dossier.rail.slots}
                />
              }
              master={
                <div className="grid min-w-0 gap-4">
                  <DemandDossierHeader
                    density={density}
                    dossier={dossier}
                    onContinue={() => setContinueOpen(true)}
                    onDensityChange={handleDensityChange}
                    onSelectDemand={goToDemand}
                    onViewChange={onViewChange ?? (() => undefined)}
                    view={view}
                  />
                  {view === "graph" ? (
                    <section className="grid gap-2" data-testid="demand-flow-graph-section">
                      <div className="flex items-center gap-2 px-1">
                        <ClipboardList className="size-4 text-ink-2" />
                        <h3 className="text-sm font-semibold tracking-normal">权威流程图</h3>
                      </div>
                      {graph && graph.nodes.length > 0 ? (
                        <Suspense fallback={<LoadingState />}>
                          {/* key 按需求重挂画布：切需求即终止上一需求可能在进行的回放会话。 */}
                          <FlowGraphCanvas
                            key={selectedDemand.id}
                            graph={graph}
                            live
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
                  ) : (
                    <DemandDossierTimeline
                      density={density}
                      onOpenTask={onOpenTask}
                      projectId={projectId}
                      timeline={dossier.timeline}
                    />
                  )}
                </div>
              }
              narrowDetail="stack"
              rail="md"
            />
          )}
        </div>
      ) : null}
      {selectedDemand ? (
        <DemandContinueDialog
          apiOptions={apiOptions}
          demandId={selectedDemand.id}
          demandTitle={selectedDemand.title}
          onContinued={(created) => {
            // 必须显式失效父页需求列表：不刷新的话左轨里没有这条新单，
            // 跳过去会落在一个列表不认识的 demand 上（链也折不出来）。
            // 不能只依赖 SSE——它是尽力而为的，而这里是刚发生的确定事实。
            void sectionQueryClient.invalidateQueries({
              queryKey: ["project-demands", projectId],
            });
            // 落到新一单的卷宗：接续的价值就在"接着往下看"，停在旧单上等于没接。
            goToDemand(created.id);
          }}
          onOpenChange={setContinueOpen}
          open={continueOpen}
        />
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

function DemandDispatchBlockerBanner({ gate }: { gate: ProjectTaskGraphDispatchGate | undefined }) {
  if (!gate) return null;
  const label = demandDispatchBlockerLabel(gate.status);

  return (
    <SoftCard className="border-warn/25 bg-warn/5 p-4" data-testid="demand-dispatch-blocker">
      <div className="flex flex-col gap-2 text-sm leading-6 text-ink sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="font-bold">{label.title}</p>
          <p className="mt-1 text-ink-2">{label.summary}</p>
        </div>
        <StatusPill className="shrink-0 self-start" tone={label.tone}>
          {dispatchGateStatusLabel(gate.status)}
        </StatusPill>
      </div>
    </SoftCard>
  );
}

/**
 * 挑出当前**仍然成立**的派发闸门 blocker。
 *
 * 数据源必须是闸门结果记录（`graph.dispatch_gates`，服务端每任务只给最新一条），
 * 不能用 `project_events` 的闸门事件推断：闸门事件按 (任务, 事件类型) 至多发一次
 * （predispatch_gate.go 的 ProjectTaskEventExists 去重），任务重试后二次卡人工不会
 * 再产生 waiting_human 事件，按事件流推断会永远看不到第二次阻塞（漏报）。
 * 闸门结果则按 (任务, 分派原因+尝试序号) 唯一，重评估必新增一行，最新一行即当前裁决。
 *
 * 判据两条同时成立：
 * 1. 该任务最新闸门裁决不是 passed；
 * 2. 该任务未到终态（终态任务不可能还在等派发；闸门记录不会因任务收尾而回写）。
 */
function currentDemandDispatchBlocker(
  graph: ProjectTaskGraph | undefined,
): ProjectTaskGraphDispatchGate | undefined {
  const nodesById = new Map(graph?.nodes.map((node) => [node.id, node]) ?? []);
  const blocking = (graph?.dispatch_gates ?? []).filter((gate) => {
    if (gate.status === "passed") return false;
    const task = nodesById.get(gate.project_task_id);
    if (!task) return false;
    return !isTerminalTaskStatus(task.status);
  });

  return blocking.sort(
    (a, b) => demandDispatchBlockerPriority(a.status) - demandDispatchBlockerPriority(b.status),
  )[0];
}

function demandDispatchBlockerPriority(status: string) {
  if (status === "replan_required") return 0;
  if (status === "blocked") return 1;
  if (status === "waiting_human") return 2;
  if (status === "retry_later") return 3;
  return 4;
}

function dispatchGateStatusLabel(status: string): string {
  return DISPATCH_GATE_STATUS_LABELS[status] ?? status;
}

const DISPATCH_GATE_STATUS_LABELS: Record<string, string> = {
  blocked: "执行条件未满足",
  replan_required: "需要重新编排",
  retry_later: "稍后重试",
  waiting_human: "等待人工确认"
};

function demandDispatchBlockerLabel(status: string): {
  summary: string;
  title: string;
  tone: "danger" | "warn";
} {
  if (status === "replan_required") {
    return {
      summary: "当前计划不再满足执行条件，需要重新编排后继续。",
      title: "计划需要调整",
      tone: "danger"
};
  }

  if (status === "waiting_human") {
    return {
      summary: "当前执行需要负责人确认后继续。",
      title: "等待负责人确认",
      tone: "warn"
};
  }

  if (status === "retry_later") {
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
