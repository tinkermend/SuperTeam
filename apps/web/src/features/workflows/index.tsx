import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { WorkflowDetail } from "./components/workflow-detail";
import { WorkflowRiverView } from "./components/workflow-river-view";
import { WorkflowShell } from "./components/workflow-shell";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { IconTile, SoftCard, StatusPill, V3Button } from "@/components/superteam";
import { StaffGapDialog } from "@/features/projects/components/staff-gap-dialog";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import {
  getProjectDemandLaunchDetail,
  getProjectTaskGraph,
  listProjectEvents,
  listWorkflowInstances,
  resolveProjectDecision,
  type ProjectEvent,
  type ProjectTaskGraph,
} from "@/lib/api/projects";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export function WorkflowPage({ demandId }: { demandId?: string }) {
  return <WorkflowView apiBaseUrl={resolveControlPlaneUrl()} demandId={demandId} />;
}

type WorkflowViewProps = {
  apiBaseUrl: string;
  demandId?: string;
  fetcher?: typeof fetch;
};

export function WorkflowView({ apiBaseUrl, demandId, fetcher }: WorkflowViewProps) {
  const navigate = useNavigate();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const listQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listWorkflowInstances(apiOptions, { limit: 50, offset: 0 }),
    queryKey: ["workflow-instances", apiBaseUrl],
    refetchInterval: 5000,
  });
  const instances = listQuery.data ?? [];
  // 直链可达性：selectedDemandId 直接取路由参数，不再要求该 demand 出现在首页 50 条列表里，
  // 否则失败需求排在河道底部时，直链会被下面的兜底重定向劫持到别的需求。
  const selectedDemandId = demandId;
  // 列表命中的 instance 仅用于详情头部展示（状态/进度 pill），不参与是否能看到详情的判断。
  const listMatchedInstance = demandId
    ? instances.find((instance) => instance.demand_id === demandId)
    : undefined;
  const fallbackDemandId = instances[0]?.demand_id;

  const detailQuery = useQuery({
    enabled: Boolean(selectedDemandId),
    placeholderData: keepPreviousData,
    queryFn: () => getProjectDemandLaunchDetail(apiOptions, selectedDemandId ?? ""),
    queryKey: ["workflow-detail", apiBaseUrl, selectedDemandId],
    refetchInterval: 5000,
  });
  const currentDetail =
    detailQuery.data?.demand.id === selectedDemandId ? detailQuery.data : undefined;
  const detailNotFound =
    detailQuery.isError &&
    detailQuery.error instanceof ApiRequestError &&
    detailQuery.error.status === 404;

  useEffect(() => {
    // 仅当按 id 拉取详情真的 404（需求不存在/不可见）时才兜底重定向到列表第一条；
    // 不能仅凭 demandId 不在首页列表命中就重定向，否则直链会被劫持。
    if (!demandId || !fallbackDemandId || !listQuery.isSuccess) {
      return;
    }

    if (!detailNotFound) {
      return;
    }

    void navigate({
      params: { demandId: fallbackDemandId },
      replace: true,
      to: "/workflows/$demandId",
    });
  }, [demandId, detailNotFound, fallbackDemandId, listQuery.isSuccess, navigate]);

  const graphQuery = useQuery({
    enabled: Boolean(currentDetail?.project.id && selectedDemandId),
    placeholderData: keepPreviousData,
    queryFn: () =>
      getProjectTaskGraph(apiOptions, currentDetail?.project.id ?? "", {
        demandId: selectedDemandId ?? "",
      }),
    queryKey: ["workflow-task-graph", apiBaseUrl, currentDetail?.project.id, selectedDemandId],
    refetchInterval: 5000,
  });
  const currentGraph = currentTaskGraph(
    graphQuery.data,
    selectedDemandId,
    graphQuery.isPlaceholderData,
  );

  const eventsQuery = useQuery({
    enabled: Boolean(currentDetail?.project.id),
    placeholderData: keepPreviousData,
    queryFn: () =>
      listProjectEvents(apiOptions, currentDetail?.project.id ?? "", { limit: 30 }),
    queryKey: ["workflow-project-events", apiBaseUrl, currentDetail?.project.id],
    refetchInterval: 5000,
  });
  const dispatchBlocker = currentWorkflowDispatchBlocker(
    eventsQuery.data,
    currentGraph,
    selectedDemandId,
  );

  if (!demandId) {
    return (
      <WorkflowShell>
        <WorkflowRiverView
          instances={instances}
          isError={listQuery.isError}
          isLoading={listQuery.isLoading}
        />
      </WorkflowShell>
    );
  }

  return (
    <WorkflowShell>
      <WorkflowBlockingBanner graph={currentGraph} />
      {currentDetail && selectedDemandId ? (
        <WorkflowGapPanel
          apiOptions={apiOptions}
          graph={currentGraph}
          projectId={currentDetail.project.id}
        />
      ) : null}
      <WorkflowDispatchBlockerBanner event={dispatchBlocker} />
      <WorkflowDetail
        detail={currentDetail}
        graph={currentGraph}
        instance={listMatchedInstance}
        isError={listQuery.isError || (detailQuery.isError && !detailNotFound) || graphQuery.isError}
      />
    </WorkflowShell>
  );
}

function WorkflowBlockingBanner({ graph }: { graph: ProjectTaskGraph | undefined }) {
  const fact = graph?.blocking_facts[0];
  if (!fact) return null;

  return (
    <SoftCard className="mb-4 border-v3-danger/25 bg-v3-danger/5 p-4">
      <div className="flex flex-col gap-2 text-sm leading-6 text-v3-ink sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="font-bold">协调已阻塞：{fact.message}</p>
          {fact.recommended_action ? (
            <p className="mt-1 text-v3-ink-2">下一步：{fact.recommended_action}</p>
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
 * 规划缺口面板：coordination.blocked 事件携带结构化 gap（RejectDemandPlanning →
 * 任务 4-6 的 planning_gap 决策通道）时，在阻塞横幅下方给出三条处置动作——一键补员、
 * 豁免约束重规划、发起借调（仅链接）。没有 gap 就不渲染面板（非结构性诊断没有可执行
 * 的结构化处置项）。
 */
function WorkflowGapPanel({
  apiOptions,
  graph,
  projectId,
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
        decision: "exempted",
      });
    },
    onError: (error: unknown) => {
      toast.error(error instanceof Error ? error.message : "豁免失败");
    },
    onSuccess: async () => {
      toast.success("已豁免约束，重新规划已触发");
      setExemptDialogOpen(false);
      await invalidateWorkflowGapQueries(queryClient);
    },
  });

  if (!gap) return null;

  return (
    <SoftCard className="mb-4 p-4" data-testid="workflow-gap-panel">
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start gap-3">
          <IconTile tone="warn">
            <span aria-hidden className="text-sm font-bold">
              缺
            </span>
          </IconTile>
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-v3-ink">
              规划缺口：{gapConstraintLabel(gap.constraint_kind)}
            </p>
            <p className="mt-1 text-xs leading-5 text-v3-ink-2">
              涉及角色：{gap.roles.length > 0 ? gap.roles.join("、") : "—"} · 当前可调度员工{" "}
              {gap.active_executor_count} 名
            </p>
            {gap.required_capabilities.length > 0 ? (
              <p className="text-xs leading-5 text-v3-ink-2">
                所需能力：{gap.required_capabilities.join("、")}
              </p>
            ) : null}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <V3Button
            disabled={!decisionRequestId}
            onClick={() => setStaffDialogOpen(true)}
            variant="primary"
          >
            从标准模板补员
          </V3Button>
          <V3Button
            disabled={!decisionRequestId}
            onClick={() => setExemptDialogOpen(true)}
            variant="outline"
          >
            豁免并重规划
          </V3Button>
          <V3Button asChild variant="ghost">
            <Link params={{ projectId }} to="/projects/$projectId/config">
              发起借调
            </Link>
          </V3Button>
        </div>
      </div>
      {decisionRequestId ? (
        <StaffGapDialog
          apiOptions={apiOptions}
          decisionRequestId={decisionRequestId}
          gap={gap}
          onOpenChange={setStaffDialogOpen}
          onStaffed={() => {
            void invalidateWorkflowGapQueries(queryClient);
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

function invalidateWorkflowGapQueries(
  queryClient: ReturnType<typeof useQueryClient>,
): Promise<unknown> {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: ["workflow-task-graph"] }),
    queryClient.invalidateQueries({ queryKey: ["workflow-detail"] }),
    queryClient.invalidateQueries({ queryKey: ["workflow-project-events"] }),
  ]);
}

function gapConstraintLabel(constraintKind: string): string {
  if (constraintKind === "role_independence") return "审查独立性约束";
  return constraintKind || "结构性约束";
}

function WorkflowDispatchBlockerBanner({ event }: { event: ProjectEvent | undefined }) {
  if (!event) return null;
  const label = workflowDispatchBlockerLabel(event.event_type);

  return (
    <SoftCard className="mb-4 border-v3-warn/25 bg-v3-warn/5 p-4">
      <div className="flex flex-col gap-2 text-sm leading-6 text-v3-ink sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="font-bold">{label.title}</p>
          <p className="mt-1 text-v3-ink-2">{label.summary}</p>
        </div>
        <StatusPill className="shrink-0 self-start" tone={label.tone}>
          {event.event_type}
        </StatusPill>
      </div>
    </SoftCard>
  );
}

function currentTaskGraph(
  graph: ProjectTaskGraph | undefined,
  selectedDemandId: string | undefined,
  isPlaceholderData = false,
): ProjectTaskGraph | undefined {
  if (!graph) return undefined;
  if (graph.nodes.length === 0) {
    if (graph.blocking_facts.length > 0 && isPlaceholderData) {
      return undefined;
    }

    return graph;
  }
  if (!selectedDemandId) return undefined;

  return graph.nodes.every(
    (node) => !node.demand_id || node.demand_id === selectedDemandId,
  )
    ? graph
    : undefined;
}

function currentWorkflowDispatchBlocker(
  events: ProjectEvent[] | undefined,
  graph: ProjectTaskGraph | undefined,
  selectedDemandId: string | undefined,
): ProjectEvent | undefined {
  const taskIds = new Set(graph?.nodes.map((node) => node.id) ?? []);
  const matchingEvents = events?.filter((event) => {
    if (!isWorkflowDispatchBlockerEvent(event.event_type)) return false;
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
      workflowDispatchBlockerPriority(a.event_type) -
      workflowDispatchBlockerPriority(b.event_type),
  )[0];
}

function isWorkflowDispatchBlockerEvent(eventType: ProjectEvent["event_type"]) {
  return (
    eventType === "project_task.dispatch_gate.replan_required" ||
    eventType === "project_task.dispatch_gate.blocked" ||
    eventType === "project_task.dispatch_gate.retry_later" ||
    eventType === "project_task.dispatch_gate.waiting_human" ||
    eventType === "project_task.dispatch_blocked"
  );
}

function workflowDispatchBlockerPriority(eventType: ProjectEvent["event_type"]) {
  if (eventType === "project_task.dispatch_gate.replan_required") return 0;
  if (eventType === "project_task.dispatch_gate.blocked") return 1;
  if (eventType === "project_task.dispatch_gate.waiting_human") return 2;
  if (eventType === "project_task.dispatch_gate.retry_later") return 3;
  return 4;
}

function workflowDispatchBlockerLabel(eventType: ProjectEvent["event_type"]): {
  summary: string;
  title: string;
  tone: "danger" | "warn";
} {
  if (eventType === "project_task.dispatch_gate.replan_required") {
    return {
      summary: "当前计划不再满足执行条件，需要重新编排后继续。",
      title: "计划需要调整",
      tone: "danger",
    };
  }

  if (eventType === "project_task.dispatch_gate.waiting_human") {
    return {
      summary: "当前执行需要负责人确认后继续。",
      title: "等待负责人确认",
      tone: "warn",
    };
  }

  if (eventType === "project_task.dispatch_gate.retry_later") {
    return {
      summary: "运行条件暂不可用，系统会稍后重试。",
      title: "稍后重试执行",
      tone: "warn",
    };
  }

  return {
    summary: "当前执行条件未满足，系统已记录阻塞原因。",
    title: "执行条件未满足",
    tone: "danger",
  };
}
