import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { TaskLaunchShell } from "./components/task-launch-shell";
import { TaskLaunchForm, type LaunchMode } from "./components/task-launch-form";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listProjects,
  submitProjectDemand,
  type SubmitProjectDemandInput,
} from "@/lib/api/projects";

type TaskLaunchPageProps = {
  title?: string;
};

export function TaskLaunchPage({ title = "任务发起" }: TaskLaunchPageProps) {
  return <TaskLaunchView apiBaseUrl={resolveControlPlaneUrl()} title={title} />;
}

type TaskLaunchViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  title?: string;
};

export function TaskLaunchView({
  apiBaseUrl,
  fetcher,
  title = "任务发起",
}: TaskLaunchViewProps) {
  const navigate = useNavigate();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const [selectedProjectId, setSelectedProjectId] = useState("");
  const [mode, setMode] = useState<LaunchMode>("plan");
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
        to: "/workflows/$demandId",
      }),
  });

  return (
    <TaskLaunchShell
      title={title}
      description="提交需求到项目，由项目协调线程编排后续任务"
    >
      <TaskLaunchForm
        isSubmitting={submitMutation.isPending}
        mode={mode}
        onModeChange={setMode}
        onProjectChange={setSelectedProjectId}
        onSubmit={(projectId, input) =>
          submitMutation.mutate({
            input: { ...input, coordination_mode: mode === "loop" ? "loop" : "plan" },
            projectId,
          })
        }
        projects={projectsQuery.data ?? []}
        selectedProjectId={selectedProjectId}
      />
    </TaskLaunchShell>
  );
}
