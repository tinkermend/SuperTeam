import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useMemo } from "react";
import { WorkflowDetail } from "./components/workflow-detail";
import { WorkflowInstanceList } from "./components/workflow-instance-list";
import { WorkflowShell } from "./components/workflow-shell";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  getProjectDemandLaunchDetail,
  getProjectTaskGraph,
  listWorkflowInstances,
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
  const fallbackSelected = !demandId ? instances[0] : undefined;
  const selected = routeSelected ?? fallbackSelected;
  const selectedDemandId = selected?.demand_id;
  const fallbackDemandId = instances[0]?.demand_id;

  useEffect(() => {
    if (!fallbackDemandId) {
      return;
    }

    if (demandId && routeSelected) {
      return;
    }

    void navigate({
      params: { demandId: fallbackDemandId },
      replace: true,
      to: "/workflows/$demandId",
    });
  }, [demandId, fallbackDemandId, navigate, routeSelected]);

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

  return (
    <WorkflowShell>
      <div className="grid items-start gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
        <WorkflowInstanceList
          instances={instances}
          selectedDemandId={selectedDemandId}
        />
        <WorkflowDetail
          detail={currentDetail}
          graph={graphQuery.data}
          instance={selected}
          isError={listQuery.isError || detailQuery.isError || graphQuery.isError}
        />
      </div>
    </WorkflowShell>
  );
}
