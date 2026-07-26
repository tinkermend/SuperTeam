import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { SendHorizontal } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { PageTabs, PageTabList, PageTab } from "@/components/superteam";
import { TaskLaunchShell } from "./components/task-launch-shell";
import { TaskLaunchForm, type LaunchMode } from "./components/task-launch-form";
import { ChatPanel, type ConvertToTaskPayload } from "./components/chat-panel";
import {
  WorkflowInstancesView,
  type WorkflowInstancesFilters
} from "./components/workflow-instances-view";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listProjects,
  submitProjectDemand,
  type SubmitProjectDemandInput
} from "@/lib/api/projects";

type TaskLaunchSearch = {
  mode?: LaunchMode;
  project?: string;
  /** 页签深链：?view=instances 打开「流程实例」；缺省为「提出任务」。 */
  view?: string;
  /** 流程实例页签的服务端关键词（防抖落定值）。 */
  q?: string;
  /** 流程实例页签的口径：archived（已结束）；缺省 active。 */
  scope?: string;
};

type TaskLaunchPageProps = {
  fetcher?: typeof fetch;
  title?: string;
};

export function TaskLaunchPage({ fetcher, title = "任务发起" }: TaskLaunchPageProps) {
  const search = useSearch({ strict: false }) as TaskLaunchSearch;
  const navigate = useNavigate();
  const apiBaseUrl = resolveControlPlaneUrl();
  const view = search.view === "instances" ? "instances" : "compose";

  const tabBar = (
    <PageTabs aria-label="任务中枢视图" role="tablist">
      <PageTabList>
        <PageTab
          active={view === "compose"}
          aria-selected={view === "compose"}
          onClick={() =>
            navigate({
              search: { mode: search.mode, project: search.project },
              to: "."
            })
          }
          role="tab"
          type="button"
        >
          提出任务
        </PageTab>
        <PageTab
          active={view === "instances"}
          aria-selected={view === "instances"}
          onClick={() =>
            navigate({
              search: { project: search.project, view: "instances" },
              to: "."
            })
          }
          role="tab"
          type="button"
        >
          流程实例
        </PageTab>
      </PageTabList>
    </PageTabs>
  );

  if (view === "instances") {
    const filters: WorkflowInstancesFilters = {
      projectId: search.project,
      q: search.q,
      scope: search.scope === "archived" ? "archived" : "active"
    };
    const handleFiltersChange = (next: WorkflowInstancesFilters) => {
      void navigate({
        replace: true,
        search: {
          mode: search.mode,
          project: next.projectId,
          q: next.q,
          scope: next.scope === "archived" ? "archived" : undefined,
          view: "instances"
        },
        to: "."
      });
    };
    return (
      <>
        <ShellPageHeader
          icon={<SendHorizontal />}
          iconTone="brand"
          subtitle="跨项目监控流程实例的运行、阻塞与结果"
          title={title}
        />
        <Main className="min-w-0 overflow-x-hidden" width="wide">
          <div className="flex min-w-0 flex-col gap-5">
            {tabBar}
            <WorkflowInstancesView
              apiOptions={{ baseUrl: apiBaseUrl, fetcher }}
              filters={filters}
              onFiltersChange={handleFiltersChange}
            />
          </div>
        </Main>
      </>
    );
  }

  return (
    <TaskLaunchView
      apiBaseUrl={apiBaseUrl}
      fetcher={fetcher}
      initialMode={search.mode}
      initialProjectId={search.project}
      tabs={tabBar}
      title={title}
    />
  );
}

type TaskLaunchViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  initialMode?: LaunchMode;
  initialProjectId?: string;
  /** 页签条（由 TaskLaunchPage 提供；直接渲染 TaskLaunchView 的测试场景可省略）。 */
  tabs?: ReactNode;
  title?: string;
};

export function TaskLaunchView({
  apiBaseUrl,
  fetcher,
  initialMode,
  initialProjectId,
  tabs,
  title = "任务发起"
}: TaskLaunchViewProps) {
  const navigate = useNavigate();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const [selectedProjectId, setSelectedProjectId] = useState(initialProjectId ?? "");
  const [mode, setMode] = useState<LaunchMode>(initialMode ?? "plan");
  const [content, setContent] = useState("");
  const [chatSource, setChatSource] = useState<{
    chatRunId: string;
    digitalEmployeeId: string;
  } | null>(null);
  const projectsQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listProjects(apiOptions, { limit: 50, offset: 0 }),
    queryKey: ["task-launch-projects", apiBaseUrl]
});
  const activeProjects = useMemo(
    () => projectsQuery.data?.filter((project) => project.status !== "archived") ?? [],
    [projectsQuery.data],
  );

  useEffect(() => {
    if (initialMode === "plan" || initialMode === "loop" || initialMode === "chat") {
      setMode(initialMode);
    }
  }, [initialMode]);

  useEffect(() => {
    if (initialProjectId) {
      setSelectedProjectId(initialProjectId);
    }
  }, [initialProjectId]);

  useEffect(() => {
    if (!activeProjects.length) {
      if (selectedProjectId) {
        setSelectedProjectId("");
      }
      return;
    }
    if (!activeProjects.some((project) => project.id === selectedProjectId)) {
      if (
        initialProjectId &&
        activeProjects.some((project) => project.id === initialProjectId)
      ) {
        setSelectedProjectId(initialProjectId);
        return;
      }
      setSelectedProjectId(activeProjects[0].id);
    }
  }, [activeProjects, initialProjectId, selectedProjectId]);

  const submitMutation = useMutation({
    mutationFn: ({
      input,
      projectId
}: {
      input: SubmitProjectDemandInput;
      projectId: string;
    }) => submitProjectDemand(apiOptions, projectId, input),
    onSuccess: (demand, variables) => {
      setChatSource(null);
      navigate({
        params: { projectId: variables.projectId },
        search: { demand: demand.id, tab: "demands" },
        to: "/projects/$projectId"
});
    }
});

  function handleConvertToTask({
    anchorProjectId,
    draft,
    chatRunId,
    digitalEmployeeId
}: ConvertToTaskPayload) {
    setMode("plan");
    setContent(draft);
    setChatSource({ chatRunId, digitalEmployeeId });
    setSelectedProjectId(anchorProjectId);
  }

  function handleModeChange(nextMode: LaunchMode) {
    if (nextMode === "chat") {
      setChatSource(null);
    }
    setMode(nextMode);
  }

  return (
    <TaskLaunchShell
      tabs={tabs}
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
                      digital_employee_id: chatSource.digitalEmployeeId
}
}
                : {})
},
            projectId
})
        }
        projects={projectsQuery.data ?? []}
        selectedProjectId={selectedProjectId}
      />
    </TaskLaunchShell>
  );
}
