import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import {
  notifySuccess,
  PageTab,
  PageTabList,
  PageTabs,
} from "@/components/superteam";
import { TaskLaunchShell } from "./components/task-launch-shell";
import {
  TaskLaunchForm,
  type LaunchMode,
  type SubmitSuccessResult,
} from "./components/task-launch-form";
import { ChatPanel, type ConvertToTaskPayload } from "./components/chat-panel";
import {
  WorkflowInstancesView,
  type WorkflowInstancesFilters,
} from "./components/workflow-instances-view";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import {
  listProjects,
  submitProjectDemand,
  type Project,
  type SubmitProjectDemandInput,
} from "@/lib/api/projects";
import { missingObjectLabel } from "@/lib/status-labels";

const HUB_SUBTITLE = "提交需求并跟踪流程实例的运行与阻塞";

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

export function TaskLaunchPage({
  fetcher,
  title = "任务发起",
}: TaskLaunchPageProps) {
  const search = useSearch({ strict: false }) as TaskLaunchSearch;
  const navigate = useNavigate();
  const apiBaseUrl = resolveControlPlaneUrl();
  const view = search.view === "instances" ? "instances" : "compose";

  const tabBar = (
    <PageTabs aria-label="任务中枢视图" role="tablist">
      <PageTabList>
        <PageTab
          id="task-hub-tab-compose"
          active={view === "compose"}
          aria-controls="task-hub-panel-compose"
          aria-selected={view === "compose"}
          onClick={() =>
            navigate({
              search: { mode: search.mode, project: search.project },
              to: ".",
            })
          }
          role="tab"
          type="button"
        >
          提出任务
        </PageTab>
        <PageTab
          id="task-hub-tab-instances"
          active={view === "instances"}
          aria-controls="task-hub-panel-instances"
          aria-selected={view === "instances"}
          onClick={() =>
            navigate({
              search: { project: search.project, view: "instances" },
              to: ".",
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
      scope: search.scope === "archived" ? "archived" : "active",
    };
    const handleFiltersChange = (next: WorkflowInstancesFilters) => {
      void navigate({
        replace: true,
        search: {
          mode: search.mode,
          project: next.projectId,
          q: next.q,
          scope: next.scope === "archived" ? "archived" : undefined,
          view: "instances",
        },
        to: ".",
      });
    };
    return (
      <TaskLaunchShell
        description={HUB_SUBTITLE}
        tabs={tabBar}
        title={title}
        width="wide"
      >
        <div
          id="task-hub-panel-instances"
          role="tabpanel"
          aria-labelledby="task-hub-tab-instances"
        >
          <WorkflowInstancesView
            apiOptions={{ baseUrl: apiBaseUrl, fetcher }}
            filters={filters}
            onFiltersChange={handleFiltersChange}
          />
        </div>
      </TaskLaunchShell>
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
  title = "任务发起",
}: TaskLaunchViewProps) {
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const [selectedProjectId, setSelectedProjectId] = useState(
    initialProjectId ?? "",
  );
  /** 搜索/点选过的项目实体缓存：id 可能不在 browse 首页 50 条内。 */
  const [projectById, setProjectById] = useState<Record<string, Project>>({});
  const [mode, setMode] = useState<LaunchMode>(initialMode ?? "plan");
  const [content, setContent] = useState("");
  const [chatSource, setChatSource] = useState<{
    chatRunId: string;
    digitalEmployeeId: string;
  } | null>(null);
  const [successResult, setSuccessResult] = useState<SubmitSuccessResult | null>(
    null,
  );
  const [submitError, setSubmitError] = useState("");

  const projectsQuery = useQuery({
    placeholderData: keepPreviousData,
    queryFn: () => listProjects(apiOptions, { limit: 50, offset: 0 }),
    queryKey: ["task-launch-projects", apiBaseUrl],
  });
  const activeProjects = useMemo(
    () =>
      projectsQuery.data?.filter((project) => project.status !== "archived") ??
      [],
    [projectsQuery.data],
  );

  function rememberProject(project: Project) {
    setProjectById((prev) =>
      prev[project.id] === project
        ? prev
        : { ...prev, [project.id]: project },
    );
  }

  useEffect(() => {
    for (const project of projectsQuery.data ?? []) {
      rememberProject(project);
    }
    // rememberProject 是稳定本地 setState 包装；依赖列表数据即可。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectsQuery.data]);

  useEffect(() => {
    if (
      initialMode === "plan" ||
      initialMode === "loop" ||
      initialMode === "chat"
    ) {
      setMode(initialMode);
    }
  }, [initialMode]);

  useEffect(() => {
    if (initialProjectId) {
      setSelectedProjectId(initialProjectId);
    }
  }, [initialProjectId]);

  // 仅在「尚无选中」时默认第一项；搜索选出的项目可能不在 browse 首页（limit 50），
  // 不得因不在 activeProjects 就强行改写选中。
  // 加载中 activeProjects 为空时不要清空已有选中（含 URL 深链）。
  useEffect(() => {
    if (projectsQuery.isLoading) {
      return;
    }
    if (!activeProjects.length) {
      // 真·零活跃项目：仅当没有缓存实体时才清空（避免误清深链 id）
      if (selectedProjectId && !projectById[selectedProjectId]) {
        setSelectedProjectId("");
      }
      return;
    }
    if (selectedProjectId) {
      return;
    }
    if (
      initialProjectId &&
      activeProjects.some((project) => project.id === initialProjectId)
    ) {
      setSelectedProjectId(initialProjectId);
      return;
    }
    setSelectedProjectId(activeProjects[0].id);
  }, [
    activeProjects,
    initialProjectId,
    projectById,
    projectsQuery.isLoading,
    selectedProjectId,
  ]);

  const resolvedProject =
    (selectedProjectId
      ? activeProjects.find((project) => project.id === selectedProjectId) ??
        projectById[selectedProjectId]
      : undefined) ?? null;

  const submitMutation = useMutation({
    mutationFn: ({
      input,
      projectId,
    }: {
      input: SubmitProjectDemandInput;
      projectId: string;
    }) => submitProjectDemand(apiOptions, projectId, input),
    onSuccess: (demand, variables) => {
      setChatSource(null);
      setContent("");
      setSubmitError("");
      const projectName =
        activeProjects.find((project) => project.id === variables.projectId)
          ?.name ??
        projectById[variables.projectId]?.name ??
        missingObjectLabel("project", variables.projectId);
      setSuccessResult({
        demandId: demand.id,
        mode,
        projectId: variables.projectId,
        projectName,
        title: demand.title || variables.input.title,
      });
      notifySuccess("需求已提交");
    },
    onError: (error) => {
      const message =
        error instanceof ApiRequestError
          ? error.message
          : error instanceof Error
            ? error.message
            : "提交失败，请重试";
      setSubmitError(message);
    },
  });

  function handleProjectChange(project: Project) {
    setSelectedProjectId(project.id);
    rememberProject(project);
  }

  function handleConvertToTask({
    anchorProjectId,
    draft,
    chatRunId,
    digitalEmployeeId,
  }: ConvertToTaskPayload) {
    setMode("plan");
    setContent(draft);
    setChatSource({ chatRunId, digitalEmployeeId });
    setSelectedProjectId(anchorProjectId);
    setSuccessResult(null);
  }

  function handleModeChange(nextMode: LaunchMode) {
    if (nextMode === "chat") {
      setChatSource(null);
    }
    setMode(nextMode);
  }

  function handleSuccessDismiss() {
    setSuccessResult(null);
    setSubmitError("");
  }

  return (
    <TaskLaunchShell
      tabs={tabs}
      title={title}
      description={HUB_SUBTITLE}
    >
      <div
        id="task-hub-panel-compose"
        role="tabpanel"
        aria-labelledby="task-hub-tab-compose"
      >
        <TaskLaunchForm
          apiOptions={apiOptions}
          chatPanel={
            <ChatPanel
              apiOptions={apiOptions}
              onConvertToTask={handleConvertToTask}
              onProjectChange={handleProjectChange}
              projectId={selectedProjectId}
              projects={activeProjects}
              resolvedProject={resolvedProject}
            />
          }
          content={content}
          isSubmitting={submitMutation.isPending}
          mode={mode}
          onContentChange={setContent}
          onModeChange={handleModeChange}
          onProjectChange={handleProjectChange}
          onSuccessDismiss={handleSuccessDismiss}
          onSubmit={(projectId, input) => {
            setSubmitError("");
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
            });
          }}
          projects={projectsQuery.data ?? []}
          projectsLoading={projectsQuery.isLoading}
          resolvedProject={resolvedProject}
          selectedProjectId={selectedProjectId}
          submitError={submitError}
          successResult={successResult}
        />
      </div>
    </TaskLaunchShell>
  );
}
