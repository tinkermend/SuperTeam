import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useMemo } from "react";
import { WorkflowDetail } from "./components/workflow-detail";
import { WorkflowEntrance } from "./components/workflow-entrance";
import { WorkflowShell } from "./components/workflow-shell";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  getProjectDemandLaunchDetail,
  getProjectTaskGraph,
  listWorkflowInstances,
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
  const currentGraph = currentTaskGraph(graphQuery.data, selectedDemandId);

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
      <WorkflowDetail
        detail={currentDetail}
        graph={currentGraph}
        instance={selected}
        isError={listQuery.isError || detailQuery.isError || graphQuery.isError}
      />
    </WorkflowShell>
  );
}

function currentTaskGraph(
  graph: ProjectTaskGraph | undefined,
  selectedDemandId: string | undefined,
): ProjectTaskGraph | undefined {
  if (!graph) return undefined;
  if (graph.nodes.length === 0) return graph;
  if (!selectedDemandId) return undefined;

  return graph.nodes.every(
    (node) => !node.demand_id || node.demand_id === selectedDemandId,
  )
    ? graph
    : undefined;
}
