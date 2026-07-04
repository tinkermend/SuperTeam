import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useMemo } from "react";
import { WorkflowDetail } from "./components/workflow-detail";
import { WorkflowEntrance } from "./components/workflow-entrance";
import { WorkflowShell } from "./components/workflow-shell";
import { SoftCard, StatusPill } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
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
  const routeSelected = demandId
    ? instances.find((instance) => instance.demand_id === demandId)
    : undefined;
  const selected = routeSelected;
  const selectedDemandId = selected?.demand_id;
  const fallbackDemandId = instances[0]?.demand_id;

  useEffect(() => {
    if (!demandId || !fallbackDemandId || !listQuery.isSuccess) {
      return;
    }

    if (routeSelected) {
      return;
    }

    void navigate({
      params: { demandId: fallbackDemandId },
      replace: true,
      to: "/workflows/$demandId",
    });
  }, [demandId, fallbackDemandId, listQuery.isSuccess, navigate, routeSelected]);

  const detailQuery = useQuery({
    enabled: Boolean(selectedDemandId),
    placeholderData: keepPreviousData,
    queryFn: () => getProjectDemandLaunchDetail(apiOptions, selectedDemandId ?? ""),
    queryKey: ["workflow-detail", apiBaseUrl, selectedDemandId],
    refetchInterval: 5000,
  });
  const currentDetail =
    detailQuery.data?.demand.id === selectedDemandId ? detailQuery.data : undefined;

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
        <WorkflowEntrance
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
        instance={selected}
        isError={listQuery.isError || detailQuery.isError || graphQuery.isError}
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
    <SoftCard className="mb-4 border-v3-warning/25 bg-v3-warning/5 p-4">
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
