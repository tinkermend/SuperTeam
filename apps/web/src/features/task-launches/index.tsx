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
import { HubContextRail } from "./components/hub-context-rail";
import {
  HubInstanceSummary,
  TaskHubInstanceRail,
} from "./components/task-hub-instance-rail";
import { type WorkflowInstancesFilters } from "./components/workflow-instances-view";
import "./components/task-hub-chat.css";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import {
  listProjects,
  submitProjectDemand,
  type Project,
  type SubmitProjectDemandInput,
  type WorkflowInstanceSummary,
} from "@/lib/api/projects";
import { missingObjectLabel } from "@/lib/status-labels";

const HUB_SUBTITLE = "对话协作与任务发起共用项目上下文；右栏挂项目目录现场";

type HubFace = "chat" | "task";

type TaskLaunchSearch = {
  mode?: LaunchMode;
  project?: string;
  /** 工作台面：chat（默认）| task。兼容旧深链 view=instances → task。 */
  face?: "chat" | "task";
  view?: "instances";
  q?: string;
  scope?: "archived";
};

type TaskLaunchPageProps = {
  fetcher?: typeof fetch;
  title?: string;
};

function resolveFace(search: TaskLaunchSearch): HubFace {
  if (search.face === "task" || search.face === "chat") {
    return search.face;
  }
  // 旧「流程实例」深链落到任务面。
  if (search.view === "instances") {
    return "task";
  }
  // 项目「提交需求」带 mode=plan|loop 且不带 face，落到任务面。
  if (search.mode === "plan" || search.mode === "loop") {
    return "task";
  }
  // 旧 mode=chat 深链落到对话面。
  if (search.mode === "chat") {
    return "chat";
  }
  return "chat";
}

export function TaskLaunchPage({
  fetcher,
  title = "任务中枢",
}: TaskLaunchPageProps) {
  const search = useSearch({ strict: false }) as TaskLaunchSearch;
  const navigate = useNavigate();
  const apiBaseUrl = resolveControlPlaneUrl();
  const face = resolveFace(search);

  const tabBar = (
    <PageTabs aria-label="任务中枢视图" role="tablist">
      <PageTabList>
        <PageTab
          id="task-hub-tab-chat"
          active={face === "chat"}
          aria-controls="task-hub-panel-chat"
          aria-selected={face === "chat"}
          onClick={() =>
            navigate({
              search: { project: search.project, face: "chat" },
              to: ".",
            })
          }
          role="tab"
          type="button"
        >
          对话
        </PageTab>
        <PageTab
          id="task-hub-tab-task"
          active={face === "task"}
          aria-controls="task-hub-panel-task"
          aria-selected={face === "task"}
          onClick={() =>
            navigate({
              search: {
                project: search.project,
                face: "task",
                mode:
                  search.mode === "loop" || search.mode === "plan"
                    ? search.mode
                    : "plan",
                q: search.q,
                scope: search.scope,
              },
              to: ".",
            })
          }
          role="tab"
          type="button"
        >
          任务
        </PageTab>
      </PageTabList>
    </PageTabs>
  );

  return (
    <TaskLaunchView
      apiBaseUrl={apiBaseUrl}
      face={face}
      fetcher={fetcher}
      initialMode={
        search.mode === "loop" || search.mode === "plan" ? search.mode : "plan"
      }
      initialProjectId={search.project}
      instanceFilters={{
        projectId: search.project,
        q: search.q,
        scope: search.scope === "archived" ? "archived" : "active",
      }}
      onFaceChange={(next) => {
        void navigate({
          search: {
            project: search.project,
            face: next,
            mode:
              next === "task"
                ? search.mode === "loop" || search.mode === "plan"
                  ? search.mode
                  : "plan"
                : undefined,
            q: next === "task" ? search.q : undefined,
            scope: next === "task" ? search.scope : undefined,
          },
          to: ".",
        });
      }}
      onInstanceFiltersChange={(next) => {
        void navigate({
          replace: true,
          search: {
            face: "task",
            mode:
              search.mode === "loop" || search.mode === "plan"
                ? search.mode
                : "plan",
            project: next.projectId,
            q: next.q,
            scope: next.scope === "archived" ? "archived" : undefined,
          },
          to: ".",
        });
      }}
      tabs={tabBar}
      title={title}
    />
  );
}

type TaskLaunchViewProps = {
  apiBaseUrl: string;
  face: HubFace;
  fetcher?: typeof fetch;
  initialMode?: Exclude<LaunchMode, "chat">;
  initialProjectId?: string;
  instanceFilters: WorkflowInstancesFilters;
  onFaceChange: (face: HubFace) => void;
  onInstanceFiltersChange: (next: WorkflowInstancesFilters) => void;
  tabs?: ReactNode;
  title?: string;
};

export function TaskLaunchView({
  apiBaseUrl,
  face,
  fetcher,
  initialMode = "plan",
  initialProjectId,
  instanceFilters,
  onFaceChange,
  onInstanceFiltersChange,
  tabs,
  title = "任务中枢",
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
  const [mode, setMode] = useState<Exclude<LaunchMode, "chat">>(initialMode);
  const [content, setContent] = useState("");
  const [chatSource, setChatSource] = useState<{
    chatRunId: string;
    digitalEmployeeId: string;
  } | null>(null);
  const [successResult, setSuccessResult] = useState<SubmitSuccessResult | null>(
    null,
  );
  const [submitError, setSubmitError] = useState("");
  const [selectedInstance, setSelectedInstance] =
    useState<WorkflowInstanceSummary | null>(null);

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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectsQuery.data]);

  useEffect(() => {
    if (initialMode === "plan" || initialMode === "loop") {
      setMode(initialMode);
    }
  }, [initialMode]);

  useEffect(() => {
    if (initialProjectId) {
      setSelectedProjectId(initialProjectId);
    }
  }, [initialProjectId]);

  useEffect(() => {
    if (projectsQuery.isLoading) {
      return;
    }
    if (!activeProjects.length) {
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
    setSelectedInstance(null);
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
    onFaceChange("task");
  }

  function handleModeChange(nextMode: Exclude<LaunchMode, "chat">) {
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
      width="wide"
    >
      {face === "chat" ? (
        <div
          id="task-hub-panel-chat"
          role="tabpanel"
          aria-labelledby="task-hub-tab-chat"
          className="hub-face"
        >
          <ChatPanel
            apiOptions={apiOptions}
            onConvertToTask={handleConvertToTask}
            onProjectChange={handleProjectChange}
            projectId={selectedProjectId}
            projects={activeProjects}
            projectsError={projectsQuery.isError}
            projectsLoading={projectsQuery.isLoading}
            resolvedProject={resolvedProject}
          />
        </div>
      ) : (
        <div
          id="task-hub-panel-task"
          role="tabpanel"
          aria-labelledby="task-hub-tab-task"
          className="hub-face hub-task"
        >
          <div className="hub-body">
            <TaskHubInstanceRail
              apiOptions={apiOptions}
              filters={{
                ...instanceFilters,
                projectId: selectedProjectId || instanceFilters.projectId,
              }}
              onFiltersChange={onInstanceFiltersChange}
              onSelect={setSelectedInstance}
              selectedDemandId={selectedInstance?.demand_id ?? null}
            />
            <section aria-label="任务工作面" className="hub-main">
              {selectedInstance ? (
                <div className="hub-task-sheet">
                  <HubInstanceSummary
                    instance={selectedInstance}
                    onBack={() => setSelectedInstance(null)}
                  />
                </div>
              ) : (
                <TaskLaunchForm
                  apiOptions={apiOptions}
                  content={content}
                  isSubmitting={submitMutation.isPending}
                  mode={mode}
                  onContentChange={setContent}
                  onModeChange={handleModeChange}
                  onProjectChange={handleProjectChange}
                  onSuccessDismiss={handleSuccessDismiss}
                  onSubmit={(projectId, input) => {
                    setSubmitError("");
                    const sourceRefs: Record<string, unknown> = {
                      ...(input.source_refs ?? {}),
                    };
                    if (chatSource) {
                      sourceRefs.chat_run_id = chatSource.chatRunId;
                      sourceRefs.digital_employee_id =
                        chatSource.digitalEmployeeId;
                    }
                    submitMutation.mutate({
                      projectId,
                      input: {
                        ...input,
                        coordination_mode: mode,
                        source_refs: sourceRefs,
                      },
                    });
                  }}
                  projects={activeProjects}
                  projectsLoading={projectsQuery.isLoading}
                  resolvedProject={resolvedProject}
                  selectedProjectId={selectedProjectId}
                  submitError={submitError}
                  successResult={successResult}
                />
              )}
            </section>
            <HubContextRail
              apiOptions={apiOptions}
              footnote="任务面看需求与阻塞。Plan / Loop 是需求策略，不是页面皮肤。"
              projectId={selectedProjectId}
            />
          </div>
        </div>
      )}
    </TaskLaunchShell>
  );
}
