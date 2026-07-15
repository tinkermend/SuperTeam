import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useMemo } from "react";
import { WorkflowDetail } from "./components/workflow-detail";
import { WorkflowRiverView } from "./components/workflow-river-view";
import { WorkflowShell } from "./components/workflow-shell";
import { SoftCard, StatusPill } from "@/components/superteam";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import {
  getProjectDemandLaunchDetail,
  getProjectTaskGraph,
  listProjectEvents,
  listWorkflowInstances,
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
