import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { TaskLaunchShell } from "./components/task-launch-shell";
import { TaskLaunchForm } from "./components/task-launch-form";
import { TaskLaunchDetail } from "./components/task-launch-detail";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  getProjectDemandLaunchDetail,
  listProjectMembers,
  listProjects,
  submitProjectDemand,
  type SubmitProjectDemandInput,
} from "@/lib/api/projects";

export function TaskLaunchPage() {
  return <TaskLaunchView apiBaseUrl={resolveControlPlaneUrl()} />;
}

type TaskLaunchViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function TaskLaunchView({ apiBaseUrl, fetcher }: TaskLaunchViewProps) {
  const navigate = useNavigate();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const [selectedProjectId, setSelectedProjectId] = useState("");
  const projectsQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listProjects(apiOptions, { limit: 50, offset: 0 }),
    queryKey: ["task-launch-projects", apiBaseUrl],
  });
  const activeProjects = useMemo(
    () => projectsQuery.data?.filter((project) => project.status !== "archived") ?? [],
    [projectsQuery.data],
  );

  useEffect(() => {
    if (!activeProjects.length) {
      if (selectedProjectId) {
        setSelectedProjectId("");
      }
      return;
    }
    if (!activeProjects.some((project) => project.id === selectedProjectId)) {
      setSelectedProjectId(activeProjects[0].id);
    }
  }, [activeProjects, selectedProjectId]);

  const membersQuery = useQuery({
    enabled: Boolean(selectedProjectId),
    placeholderData: keepPreviousData,
    queryFn: () => listProjectMembers(apiOptions, selectedProjectId),
    queryKey: ["task-launch-project-members", apiBaseUrl, selectedProjectId],
  });
  const hasSelectedProjectMembers = (membersQuery.data ?? []).some(
    (member) => member.project_id === selectedProjectId,
  );
  const isReviewerLoading =
    Boolean(selectedProjectId) && membersQuery.isFetching && !hasSelectedProjectMembers;
  const submitMutation = useMutation({
    mutationFn: ({
      input,
      projectId,
    }: {
      input: SubmitProjectDemandInput;
      projectId: string;
    }) => submitProjectDemand(apiOptions, projectId, input),
    onSuccess: (demand) =>
      navigate({
        params: { demandId: demand.id },
        to: "/task-launches/$demandId",
      }),
  });

  return (
    <TaskLaunchShell
      title="任务发起"
      description="提交需求到项目，由项目协调线程编排后续任务"
    >
      <TaskLaunchForm
        isSubmitting={submitMutation.isPending}
        isReviewerLoading={isReviewerLoading}
        members={membersQuery.data ?? []}
        onProjectChange={setSelectedProjectId}
        onSubmit={(projectId, input) => submitMutation.mutate({ input, projectId })}
        projects={projectsQuery.data ?? []}
        selectedProjectId={selectedProjectId}
      />
    </TaskLaunchShell>
  );
}

export function TaskLaunchDetailPage({ demandId }: { demandId: string }) {
  return <TaskLaunchDetailView apiBaseUrl={resolveControlPlaneUrl()} demandId={demandId} />;
}

type TaskLaunchDetailViewProps = {
  apiBaseUrl: string;
  demandId: string;
  fetcher?: typeof fetch;
};

export function TaskLaunchDetailView({
  apiBaseUrl,
  demandId,
  fetcher,
}: TaskLaunchDetailViewProps) {
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const detailQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => getProjectDemandLaunchDetail(apiOptions, demandId),
    queryKey: ["task-launch-detail", apiBaseUrl, demandId],
  });
  const currentDetail =
    detailQuery.data?.demand.id === demandId ? detailQuery.data : undefined;

  return (
    <TaskLaunchShell title="发起详情" description="查看一次任务发起触发的协调事实">
      {detailQuery.isError ? (
        <div className="text-sm text-destructive">发起详情加载失败</div>
      ) : currentDetail ? (
        <TaskLaunchDetail detail={currentDetail} />
      ) : (
        <div className="text-sm text-muted-foreground">正在加载发起详情</div>
      )}
    </TaskLaunchShell>
  );
}
