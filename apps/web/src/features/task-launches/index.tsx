import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { TaskLaunchShell } from "./components/task-launch-shell";
import { TaskLaunchForm, type LaunchMode } from "./components/task-launch-form";
import { ChatPanel, type ConvertToTaskPayload } from "./components/chat-panel";
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
  const [content, setContent] = useState("");
  const [chatSource, setChatSource] = useState<{
    chatRunId: string;
    digitalEmployeeId: string;
  } | null>(null);
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
    onSuccess: (demand) => {
      setChatSource(null);
      navigate({
        params: { demandId: demand.id },
        to: "/workflows/$demandId",
      });
    },
  });

  function handleConvertToTask({
    anchorProjectId,
    draft,
    chatRunId,
    digitalEmployeeId,
  }: ConvertToTaskPayload) {
    setMode("plan");
    setContent(draft);
    setChatSource({ chatRunId, digitalEmployeeId });
    // Default the task form's project to the chat run's anchor; user can still
    // change it before submitting.
    setSelectedProjectId(anchorProjectId);
  }

  function handleModeChange(nextMode: LaunchMode) {
    if (nextMode === "chat") {
      // Prevents stale lineage: a chat-sourced demand's source_refs must not
      // leak onto a later, unrelated demand submitted after switching back to
      // chat mode and never converting anything from it.
      setChatSource(null);
    }
    setMode(nextMode);
  }

  return (
    <TaskLaunchShell
      title={title}
      description="提交需求到项目，由项目协调线程编排后续任务"
    >
      <TaskLaunchForm
        chatPanel={
          <ChatPanel
            apiOptions={apiOptions}
            onConvertToTask={handleConvertToTask}
            onProjectChange={setSelectedProjectId}
            projectId={selectedProjectId}
            projects={activeProjects}
          />
        }
        content={content}
        isSubmitting={submitMutation.isPending}
        mode={mode}
        onContentChange={setContent}
        onModeChange={handleModeChange}
        onProjectChange={setSelectedProjectId}
        onSubmit={(projectId, input) =>
          submitMutation.mutate({
            input: {
              ...input,
              coordination_mode: mode === "loop" ? "loop" : "plan",
              ...(chatSource
                ? {
                    source_refs: {
                      chat_run_id: chatSource.chatRunId,
                      digital_employee_id: chatSource.digitalEmployeeId,
                    },
                  }
                : {}),
            },
            projectId,
          })
        }
        projects={projectsQuery.data ?? []}
        selectedProjectId={selectedProjectId}
      />
    </TaskLaunchShell>
  );
}
