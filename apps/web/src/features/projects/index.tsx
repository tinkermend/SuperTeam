import { useEffect, useMemo, useRef, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient
} from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import {
  AlertTriangle,
  Check,
  Copy,
  FolderKanban,
  Plus
} from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  MasterDetailLayout,
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  WorkSurface,
  Callout
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import { apiErrorMessage } from "@/lib/api/api-error";
import {
  getCurrentUser,
  listUserProjectTeamScopes,
  type UserProjectTeamScope
} from "@/lib/api";
import { deleteBlockerTypeLabel, statusLabel, archiveReadinessCodeLabel } from "@/lib/status-labels";
import {
  addProjectRuntimeNode,
  markProjectWorkspaceReady,
  recloneProjectWorkspace,
  archiveProject,
  unarchiveProject,
  createProject,
  createProjectArchiveSnapshot,
  createProjectEvidence,
  deleteProject,
  getProject,
  getProjectAcceptance,
  getProjectArchivePreview,
  getProjectBudgetSummary,
  getProjectDeletePreview,
  getProjectExecutionTrace,
  getProjectOverview,
  getProjectRuntimeReadiness,
  removeProjectRuntimeNode,
  listProjectArchiveSnapshots,
  listProjectArtifacts,
  listProjectBudgetLedger,
  getProjectTaskGraph,
  listProjectCoordinationJobs,
  listProjectDecisionRequests,
  listProjectDemands,
  listProjectEvidence,
  listProjectEvents,
  listProjectExecutionSummaries,
  listProjectPlanRevisions,
  listProjectReports,
  listProjectRouteDecisions,
  listProjects,
  listProjectTaskDispatchGates,
  listProjectTasks,
  listProjectTransferRequests,
  listProjectRunSummaries,
  patchProjectEvidence,
  dismissProjectTask,
  resolveProjectDecision,
  submitProjectDemand,
  type CreateProjectArchiveSnapshotInput,
  type CreateProjectEvidenceInput,
  type CreateProjectInput,
  type ListProjectsFilters,
  type ProjectDeleteBlockedErrorResponse,
  type ProjectDeleteBlocker,
  type ProjectDeletePreview,
  type ProjectEvidenceVerificationStatus,
  type ProjectRunSummaryItem,
  type ProjectExecutionTrace,
  type ProjectTask,
  type ProjectStatus,
  type SubmitProjectDemandInput
} from "@/lib/api/projects";
import { listDigitalEmployees } from "@/lib/api/employees";
import { listUsers, type UserSummary } from "@/lib/api/auth";
import { listRuntimeNodes } from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack
} from "@/components/layout/shell-page-header";
import { ProjectOperationalDetail } from "./components/project-operational-detail";
import { ProjectRuntimePlacementPanel } from "./components/project-runtime-placement-panel";
import { CreateProjectShell } from "./components/create-project";
import { SubmitDemandDialog } from "./components/submit-demand-dialog";
import { ProjectConfigView } from "./components/project-config-page";
import {
  ProjectPortfolioPerspectivePanel,
  ProjectPortfolioSummaryBar,
  ProjectRiskQueue,
  ProjectTriagePanel
} from "./components/project-risk-home";
import { useProjectRiskSignals } from "./hooks/use-project-risk-signals";
import {
  buildProjectPortfolioCounts,
  buildProjectRiskSummaryFromCounts,
  buildRiskCounts,
  emptyProjectRiskSummary,
  matchesProjectRiskFilter,
  sortProjectsByRisk,
  type ProjectRiskFilter,
  type ProjectRiskSummaryMap
} from "./project-risk";

type ProjectsPageProps = {
  fetcher?: typeof fetch;
};

type ProjectsViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  routeProjectId?: string;
};

type UiProjectListFilters = {
  q: string;
  risk: ProjectRiskFilter;
  status: "all" | ProjectStatus;
};

export function ProjectsPage({ fetcher }: ProjectsPageProps = {}) {
  return <ProjectsView apiBaseUrl={resolveControlPlaneUrl()} fetcher={fetcher} />;
}

export function ProjectDetailPage({
  fetcher,
  projectId
}: ProjectsPageProps & { projectId: string }) {
  return (
    <ProjectsView
      apiBaseUrl={resolveControlPlaneUrl()}
      fetcher={fetcher}
      routeProjectId={projectId}
    />
  );
}

export function ProjectConfigPage({
  fetcher,
  initialTab,
  projectId
}: ProjectsPageProps & {
  projectId: string;
  initialTab?: "overview" | "members" | "casting" | "capabilities" | "coordination";
}) {
  return (
    <ProjectConfigView
      apiBaseUrl={resolveControlPlaneUrl()}
      fetcher={fetcher}
      initialTab={initialTab}
      projectId={projectId}
    />
  );
}

export function CreateProjectPage({ fetcher }: ProjectsPageProps = {}) {
  return (
    <CreateProjectView
      apiBaseUrl={resolveControlPlaneUrl()}
      fetcher={fetcher}
    />
  );
}

export function CreateProjectView({
  apiBaseUrl,
  fetcher
}: Omit<ProjectsViewProps, "routeProjectId">) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );

  const currentUserQuery = useQuery({
    queryKey: ["auth", "current-user", "project-create"],
    queryFn: () => getCurrentUser(apiOptions),
    refetchOnMount: "always",
    staleTime: 0
});
  const currentUser = currentUserQuery.data?.user;
  const currentUserId = currentUser?.id;

  const projectTeamScopesQuery = useQuery({
    enabled: Boolean(currentUserId),
    queryKey: ["auth", "users", currentUserId, "project-team-scopes", "project-create"],
    queryFn: () => listUserProjectTeamScopes(apiOptions, currentUserId as string),
    refetchOnMount: "always",
    staleTime: 0
});

  const availableProjectTeamScopes = useMemo<UserProjectTeamScope[]>(
    () =>
      (projectTeamScopesQuery.data?.items ?? []).filter(
        (scope) =>
          scope.status === "active" &&
          !scope.revoked_at &&
          scope.team.status === "active",
      ),
    [projectTeamScopesQuery.data?.items],
  );

  const createMutation = useMutation({
    mutationFn: (input: CreateProjectInput) => createProject(apiOptions, input),
    onSuccess: async (response) => {
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      queryClient.setQueryData(["project", response.project.id], response.project);
      void navigate({
        params: { projectId: response.project.id },
        to: "/projects/$projectId"
});
    }
});

  return (
    <>
      <ShellPageHeader
        back={<ShellPageHeaderBack ariaLabel="返回项目管理" to="/projects" />}
        title="新建项目工作台"
        subtitle="建立项目事实容器，配置负责人、团队、数字员工池与协调策略。"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <CreateProjectShell
          apiBaseUrl={apiBaseUrl}
          availableTeams={availableProjectTeamScopes}
          currentUser={currentUser}
          currentUserError={currentUserQuery.error?.message}
          fetcher={fetcher}
          isCurrentUserLoading={currentUserQuery.isFetching}
          isSubmitting={createMutation.isPending}
          isTeamsLoading={projectTeamScopesQuery.isFetching}
          submitError={
            createMutation.error
              ? createProjectSubmitErrorMessage(createMutation.error)
              : undefined
          }
          teamsError={projectTeamScopesQuery.error?.message}
          showHeading={false}
          onCancel={() => void navigate({ to: "/projects" })}
          onSubmit={(input) => createMutation.mutate(input)}
        />
      </Main>
    </>
  );
}

export function ProjectsView({
  apiBaseUrl,
  fetcher,
  routeProjectId
}: ProjectsViewProps) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as {
    /** 需求流程区选中需求（?tab=demands&demand=<id>）。 */
    demand?: string;
    focus?: string;
    tab?: string;
    /** 执行轨迹面板按任务过滤（?tab=trace&task=<id>）。 */
    task?: string;
    /** 一单卷宗中栏视图（?tab=demands&demand=<id>&view=timeline|graph）。 */
    view?: string;
  };
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const [filters, setFilters] = useState<UiProjectListFilters>({
    q: "",
    risk: "all",
    status: "all"
});
  const [selectedRuntimeNodeId, setSelectedRuntimeNodeId] = useState("");
  const [selectedQueueProjectId, setSelectedQueueProjectId] = useState("");
  const [demandOpen, setDemandOpen] = useState(false);
  const [archiveDialogOpen, setArchiveDialogOpen] = useState(false);
  const [unarchiveDialogOpen, setUnarchiveDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [deleteBlocked, setDeleteBlocked] =
    useState<ProjectDeleteBlockedErrorResponse | undefined>(undefined);
  const [projectListPage, setProjectListPage] = useState(1);
  // 默认每页 10 条：与分页器选项一致（此前默认 5 不在 [10,20] 选项内，
  // 分页器显示 10 实际渲染 5），也让队列在常规桌面高度下填满首屏。
  const [projectListPageSize, setProjectListPageSize] = useState(10);

  const listFilters = useMemo<ListProjectsFilters>(() => {
    const request: ListProjectsFilters = { limit: 50, offset: 0 };
    if (filters.q.trim()) {
      request.q = filters.q.trim();
    }
    if (filters.status !== "all") {
      request.status = filters.status as ProjectStatus;
    }
    return request;
  }, [filters.q, filters.status]);

  const projectsQuery = useQuery({
    queryKey: ["projects", listFilters],
    queryFn: () => listProjects(apiOptions, listFilters),
    placeholderData: keepPreviousData
});
  // limit 500：与 CP normalizeRunSummaryLimit 上限一致，覆盖 listProjects(≤50) 全集，
  // 避免 ORDER BY 风险优先 + 双端 limit 50 时 join 漏项被当成「暂无阻塞」。
  const runSummaryQuery = useQuery({
    enabled: !routeProjectId,
    queryKey: ["projects", "run-summary", { limit: 500 }],
    queryFn: () => listProjectRunSummaries(apiOptions, { limit: 500 }),
    placeholderData: keepPreviousData,
  });
  // 数字员工 / 用户目录：成员快照缺名时回退真实名称，避免项目列表/详情裸显负责人 UUID。
  // 列表页也要拉用户目录——风险队列负责人行依赖 principalNamesById，不能仅在进详情后启用。
  const digitalEmployeesQuery = useQuery({
    queryKey: ["digital-employees", "project-name-map"],
    queryFn: () => listDigitalEmployees(apiOptions),
    placeholderData: keepPreviousData,
    staleTime: 60_000
});
  const usersQuery = useQuery({
    queryKey: ["auth-users", "member-name-lookup"],
    queryFn: () => listUsers({ ...apiOptions, limit: 200 }),
    placeholderData: keepPreviousData,
    staleTime: 60_000
});
  const principalNamesById = useMemo(() => {
    const names = new Map<string, string>();
    for (const employee of digitalEmployeesQuery.data ?? []) {
      if (employee.name?.trim()) {
        names.set(employee.id, employee.name.trim());
      }
    }
    for (const user of usersQuery.data?.items ?? []) {
      const name = user.display_name?.trim() || user.username?.trim();
      if (name) {
        names.set(user.id, name);
      }
    }
    return names;
  }, [digitalEmployeesQuery.data, usersQuery.data]);
  const usersById = useMemo(() => {
    const map = new Map<string, UserSummary>();
    for (const user of usersQuery.data?.items ?? []) {
      map.set(user.id, user);
    }
    return map;
  }, [usersQuery.data]);
  const employeeNamesById = principalNamesById;
  const projects = projectsQuery.data ?? [];

  const runSummaryByProjectId = useMemo(() => {
    // 用契约类型而非就地字面量：新增计数列时自动跟随，不会漏字段。
    const map = new Map<string, ProjectRunSummaryItem>();
    for (const item of runSummaryQuery.data?.items ?? []) {
      map.set(item.project_id, item);
    }
    return map;
  }, [runSummaryQuery.data]);

  const listRiskSummaries = useMemo<ProjectRiskSummaryMap>(() => {
    const map: ProjectRiskSummaryMap = {};
    const runSummaryFailed = runSummaryQuery.isError;
    const runSummaryPending =
      runSummaryQuery.isLoading ||
      (runSummaryQuery.isFetching && runSummaryQuery.data === undefined);
    for (const project of projects) {
      const archived =
        project.status === "archived" || Boolean(project.archived_at);
      // 归档不在 run-summary 宇宙内（服务端 status != archived），空 ready 合法。
      if (archived) {
        map[project.id] = emptyProjectRiskSummary(project, { state: "ready" });
        continue;
      }
      if (runSummaryFailed) {
        map[project.id] = emptyProjectRiskSummary(project, { state: "error" });
        continue;
      }
      const item = runSummaryByProjectId.get(project.id);
      if (!item) {
        // 成功但无行 = 截断/join 漏项，不得冒充「暂无阻塞」。
        map[project.id] = emptyProjectRiskSummary(project, {
          state: runSummaryPending ? "pending" : "error",
        });
        continue;
      }
      const ownerId = project.human_owner_user_id?.trim();
      const ownerName = ownerId
        ? employeeNamesById.get(ownerId)?.trim()
        : undefined;
      map[project.id] = buildProjectRiskSummaryFromCounts(
        project,
        {
          open_decision_count: item.open_decision_count,
          // 与 open_decision_count 并列展示，必须用 orphan 口径，否则同一次人工动作双计。
          waiting_human_unlinked_count: item.waiting_human_unlinked_count,
          failed_count: item.failed_count,
          evidence_pending_count: item.evidence_pending_count,
          running_count: item.running_count,
          unassigned_count: item.unassigned_count,
          last_activity_at: item.last_activity_at,
        },
        {
          state: "ready",
          owner: ownerId
            ? {
                id: ownerId,
                label: ownerName || ownerId,
                principalType: "human_user",
              }
            : undefined,
        },
      );
    }
    return map;
  }, [
    employeeNamesById,
    projects,
    runSummaryByProjectId,
    runSummaryQuery.data,
    runSummaryQuery.isError,
    runSummaryQuery.isFetching,
    runSummaryQuery.isLoading,
  ]);

  const triageDetailEnabled =
    Boolean(selectedQueueProjectId) && !routeProjectId;
  const triageProjects = useMemo(
    () =>
      triageDetailEnabled
        ? projects.filter((project) => project.id === selectedQueueProjectId)
        : [],
    [projects, selectedQueueProjectId, triageDetailEnabled],
  );
  const triageRiskSignals = useProjectRiskSignals({
    apiOptions,
    enabled: triageDetailEnabled && triageProjects.length > 0,
    principalNamesById: employeeNamesById,
    projects: triageProjects,
  });

  const displayedRiskSummaries = useMemo<ProjectRiskSummaryMap>(() => {
    const map: ProjectRiskSummaryMap = { ...listRiskSummaries };
    if (
      selectedQueueProjectId &&
      triageRiskSignals.summaries[selectedQueueProjectId]?.state === "ready"
    ) {
      const detail = triageRiskSignals.summaries[selectedQueueProjectId];
      const list = listRiskSummaries[selectedQueueProjectId];
      // 明细 ready 后不得再挂列表 countBuckets：否则 breakdown 优先读桶，
      // 与 reasons 去重口径（decision 覆盖 waiting_human）分叉。
      map[selectedQueueProjectId] = {
        ...detail,
        lastActivityAt: list?.lastActivityAt ?? detail.lastActivityAt,
        runningCount: list?.runningCount ?? detail.runningCount,
      };
    }
    return map;
  }, [listRiskSummaries, selectedQueueProjectId, triageRiskSignals.summaries]);

  const isListRiskSettling = !routeProjectId && runSummaryQuery.isLoading;

  // 关注 chip 计数 + 风险筛选：基于已加载全量（run-summary 真值），再客户端分页。
  const loadedRiskCounts = useMemo(
    () =>
      buildRiskCounts(
        projects.map(
          (project) =>
            listRiskSummaries[project.id] ?? emptyProjectRiskSummary(project),
        ),
      ),
    [listRiskSummaries, projects],
  );
  const riskFilteredProjects = useMemo(
    () =>
      sortProjectsByRisk(projects, listRiskSummaries).filter((project) => {
        const summary =
          listRiskSummaries[project.id] ?? emptyProjectRiskSummary(project);
        return matchesProjectRiskFilter(summary, filters.risk);
      }),
    [filters.risk, listRiskSummaries, projects],
  );
  const projectListPageCount = Math.max(
    1,
    Math.ceil(riskFilteredProjects.length / projectListPageSize),
  );
  const activeProjectListPage = Math.min(projectListPage, projectListPageCount);
  const pagedProjects = riskFilteredProjects.slice(
    (activeProjectListPage - 1) * projectListPageSize,
    activeProjectListPage * projectListPageSize,
  );

  useEffect(() => {
    setProjectListPage(1);
  }, [filters.q, filters.risk, filters.status]);

  const portfolioCounts = useMemo(
    () => buildProjectPortfolioCounts(projects),
    [projects],
  );
  // 选中项在已加载全量中查找（风险筛选/分页后仍可保留 triage）。
  const selectedQueueProject = projects.find(
    (project) => project.id === selectedQueueProjectId,
  );
  const selectedQueueSummary = selectedQueueProject
    ? displayedRiskSummaries[selectedQueueProject.id]
    : undefined;
  const isProjectPortfolioEmpty =
    !routeProjectId &&
    projects.length === 0 &&
    filters.q.trim() === "" &&
    filters.status === "all" &&
    filters.risk === "all";

  const effectiveProjectId = routeProjectId;
  const selectedProjectFromList = effectiveProjectId
    ? projects.find((project) => project.id === effectiveProjectId)
    : undefined;

  const selectedProjectQuery = useQuery({
    // Always fetch detail on the project route so allowed_actions (archive/delete)
    // are present; list items do not include them.
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project", effectiveProjectId],
    queryFn: () => getProject(apiOptions, effectiveProjectId as string),
    placeholderData: keepPreviousData
});

  const selectedProject =
    selectedProjectQuery.data?.id === effectiveProjectId
      ? selectedProjectQuery.data
      : selectedProjectFromList;

  const overviewQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-overview", effectiveProjectId],
    queryFn: () => getProjectOverview(apiOptions, effectiveProjectId as string),
    placeholderData: keepPreviousData
});

  const tasksQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-tasks", effectiveProjectId],
    queryFn: () => listProjectTasks(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData
});

  const eventsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-events", effectiveProjectId],
    queryFn: () => listProjectEvents(apiOptions, effectiveProjectId as string, { limit: 30 }),
    placeholderData: keepPreviousData
});

  const demandsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-demands", effectiveProjectId],
    queryFn: () => listProjectDemands(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData
});

  const routeDecisionsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-route-decisions", effectiveProjectId],
    queryFn: () =>
      listProjectRouteDecisions(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData
});

  const runtimeReadinessQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-runtime-readiness", effectiveProjectId],
    queryFn: () => getProjectRuntimeReadiness(apiOptions, effectiveProjectId as string)
});

  const runtimeNodesQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["runtime-nodes", "project-placement"],
    queryFn: () => listRuntimeNodes({ ...apiOptions, limit: 100 }),
    placeholderData: keepPreviousData
});
  const currentProjectRuntimeReadiness =
    runtimeReadinessQuery.isSuccess ? runtimeReadinessQuery.data : undefined;

  const latestDemandId = demandsQuery.data?.[0]?.id;
  const planRevisionsQuery = useQuery({
    enabled: Boolean(effectiveProjectId) && Boolean(latestDemandId),
    queryKey: ["project-plan-revisions", effectiveProjectId, latestDemandId],
    queryFn: () =>
      listProjectPlanRevisions(apiOptions, effectiveProjectId as string, {
        demandId: latestDemandId as string,
        limit: 10
}),
    placeholderData: keepPreviousData
});

  const taskGraphQuery = useQuery({
    enabled: Boolean(effectiveProjectId) && Boolean(latestDemandId),
    queryKey: ["project-task-graph", effectiveProjectId, latestDemandId],
    queryFn: () =>
      getProjectTaskGraph(apiOptions, effectiveProjectId as string, {
        demandId: latestDemandId as string
}),
    placeholderData: keepPreviousData
});

  useEffect(() => {
    const placedRuntimeNodeId = runtimeReadinessQuery.data?.runtime_node_id;
    if (placedRuntimeNodeId) {
      setSelectedRuntimeNodeId(placedRuntimeNodeId);
      return;
    }
    const nodes = runtimeNodesQuery.data ?? [];
    const firstOnlineNode = nodes.find((node) => node.status === "online");
    const firstNode = firstOnlineNode ?? nodes[0];
    setSelectedRuntimeNodeId(firstNode?.runtime_node_id ?? firstNode?.node_id ?? "");
  }, [runtimeNodesQuery.data, runtimeReadinessQuery.data?.runtime_node_id]);

  const dispatchGateTask = selectDispatchGateTask({
    fallbackTasks: tasksQuery.data ?? [],
    graphTasks: taskGraphQuery.data?.nodes ?? [],
    projectId: effectiveProjectId
});
  const dispatchGateTaskId = dispatchGateTask?.id;

  const dispatchGatesQuery = useQuery({
    enabled: Boolean(effectiveProjectId) && Boolean(dispatchGateTaskId),
    queryKey: ["project-task-dispatch-gates", effectiveProjectId, dispatchGateTaskId],
    queryFn: () =>
      listProjectTaskDispatchGates(
        apiOptions,
        effectiveProjectId as string,
        dispatchGateTaskId as string,
      ),
    placeholderData: keepPreviousData
});

  const coordinationJobsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-coordination-jobs", effectiveProjectId],
    queryFn: () =>
      listProjectCoordinationJobs(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData
});

  const decisionRequestsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-decisions", effectiveProjectId],
    queryFn: () =>
      listProjectDecisionRequests(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData
});

  const executionSummariesQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-execution-summaries", effectiveProjectId],
    queryFn: () =>
      listProjectExecutionSummaries(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData
});

  const executionTraceQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-execution-trace", effectiveProjectId],
    queryFn: () =>
      getProjectExecutionTrace(apiOptions, effectiveProjectId as string, { limit: 100 }),
    placeholderData: keepPreviousData
});

  const transferRequestsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-transfer-requests", effectiveProjectId],
    queryFn: () =>
      listProjectTransferRequests(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData
});

  const evidenceQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-evidence", effectiveProjectId],
    queryFn: () => listProjectEvidence(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData
});

  const artifactsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-artifacts", effectiveProjectId],
    queryFn: () => listProjectArtifacts(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData
});

  const reportsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-reports", effectiveProjectId],
    queryFn: () => listProjectReports(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData
});

  const budgetLedgerQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-budget-ledger", effectiveProjectId],
    queryFn: () =>
      listProjectBudgetLedger(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData
});

  const budgetSummaryQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-budget-summary", effectiveProjectId],
    queryFn: async () => {
      const projectId = effectiveProjectId as string;
      const summary = await getProjectBudgetSummary(apiOptions, projectId);
      return { projectId, summary };
    },
    placeholderData: keepPreviousData
});

  const acceptanceQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-acceptance", effectiveProjectId],
    queryFn: async () => {
      const projectId = effectiveProjectId as string;
      try {
        return await getProjectAcceptance(apiOptions, projectId);
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 404) {
          return null;
        }
        throw error;
      }
    },
    placeholderData: keepPreviousData
});

  const archivePreviewQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-archive-preview", effectiveProjectId],
    queryFn: () => getProjectArchivePreview(apiOptions, effectiveProjectId as string),
    placeholderData: keepPreviousData
});

  const archiveSnapshotsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-archive-snapshots", effectiveProjectId],
    queryFn: () =>
      listProjectArchiveSnapshots(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData
});

  const deletePreviewQuery = useQuery({
    enabled: deleteDialogOpen && Boolean(effectiveProjectId),
    queryKey: ["project-delete-preview", effectiveProjectId],
    queryFn: () => getProjectDeletePreview(apiOptions, effectiveProjectId as string)
});

  const invalidateRuntimePlacementSurfaces = async (projectId?: string) => {
    if (!projectId) {
      return;
    }
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["project-runtime-readiness", projectId] }),
      queryClient.invalidateQueries({ queryKey: ["project-task-graph", projectId] }),
      queryClient.invalidateQueries({ queryKey: ["project-events", projectId] }),
      queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
    ]);
  };

  const bindRuntimePlacementMutation = useMutation({
    mutationFn: (runtimeNodeId: string) =>
      addProjectRuntimeNode(apiOptions, effectiveProjectId as string, runtimeNodeId, {
        reason: "project_runtime_placement_panel"
}),
    onSuccess: async () => {
      await invalidateRuntimePlacementSurfaces(effectiveProjectId);
    }
});

  const releaseRuntimePlacementMutation = useMutation({
    mutationFn: (runtimeNodeId: string) =>
      removeProjectRuntimeNode(apiOptions, effectiveProjectId as string, runtimeNodeId),
    onSuccess: async () => {
      await invalidateRuntimePlacementSurfaces(effectiveProjectId);
    }
});

  const archiveMutation = useMutation({
    mutationFn: (projectId: string) => archiveProject(apiOptions, projectId),
    onSuccess: async (project) => {
      setArchiveDialogOpen(false);
      queryClient.setQueryData(["project", project.id], project);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["project-archive-preview", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["inbox"] }),
      ]);
    }
  });

  const unarchiveMutation = useMutation({
    mutationFn: (projectId: string) => unarchiveProject(apiOptions, projectId),
    onSuccess: async (project) => {
      setUnarchiveDialogOpen(false);
      queryClient.setQueryData(["project", project.id], project);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["project-config", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["project-archive-preview", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", project.id] }),
      ]);
    }
  });

  const recloneWorkspaceMutation = useMutation({
    mutationFn: (projectId: string) =>
      recloneProjectWorkspace(apiOptions, projectId, "console reclone"),
    onSuccess: async (project) => {
      queryClient.setQueryData(["project", project.id], project);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", project.id] }),
      ]);
    }
});

  const markWorkspaceReadyMutation = useMutation({
    mutationFn: (projectId: string) =>
      markProjectWorkspaceReady(apiOptions, projectId, "console mark ready"),
    onSuccess: async (project) => {
      queryClient.setQueryData(["project", project.id], project);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", project.id] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", project.id] }),
      ]);
    }
});

  const submitDemandMutation = useMutation({
    mutationFn: (input: SubmitProjectDemandInput) =>
      submitProjectDemand(apiOptions, effectiveProjectId as string, input),
    onSuccess: async (demand) => {
      const projectId = demand.project_id || effectiveProjectId;
      setDemandOpen(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-demands", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
      ]);
    }
});

  const resolveDecisionMutation = useMutation({
    mutationFn: (input: {
      decisionId: string;
      decision: string;
      targetExitDeliverable?: string;
    }) =>
      resolveProjectDecision(
        apiOptions,
        effectiveProjectId as string,
        input.decisionId,
        {
          decision: input.decision,
          target_exit_deliverable: input.targetExitDeliverable
},
      ),
    onSuccess: async (decisionRequest) => {
      const projectId = decisionRequest.project_id || effectiveProjectId;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-decisions", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-tasks", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-plan-revisions", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-task-graph", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-task-dispatch-gates", projectId] }),
      ]);
    }
});

  const dismissTaskMutation = useMutation({
    mutationFn: (taskId: string) =>
      dismissProjectTask(apiOptions, effectiveProjectId as string, taskId),
    onSuccess: async (task) => {
      const projectId = task.project_id || effectiveProjectId;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-tasks", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-task-graph", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-risk-signals"] }),
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
      ]);
    }
});

  const createEvidenceMutation = useMutation({
    mutationFn: (input: CreateProjectEvidenceInput) =>
      createProjectEvidence(apiOptions, effectiveProjectId as string, input),
    onSuccess: async (evidence) => {
      const projectId = evidence.project_id || effectiveProjectId;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-evidence", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["project-archive-preview", projectId]
}),
      ]);
    }
});

  const patchEvidenceMutation = useMutation({
    mutationFn: (input: {
      evidenceId: string;
      verificationStatus: ProjectEvidenceVerificationStatus;
    }) =>
      patchProjectEvidence(apiOptions, effectiveProjectId as string, input.evidenceId, {
        verification_status: input.verificationStatus
}),
    onSuccess: async (evidence) => {
      const projectId = evidence.project_id || effectiveProjectId;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-evidence", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["project-archive-preview", projectId]
}),
      ]);
    }
});

  const createArchiveSnapshotMutation = useMutation({
    mutationFn: (input: CreateProjectArchiveSnapshotInput) =>
      createProjectArchiveSnapshot(apiOptions, effectiveProjectId as string, input),
    onSuccess: async (snapshot) => {
      const projectId = snapshot.project_id || effectiveProjectId;
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ["project-archive-snapshots", projectId]
}),
        queryClient.invalidateQueries({
          queryKey: ["project-archive-preview", projectId]
}),
        queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
      ]);
    }
});

  const deleteProjectMutation = useMutation({
    mutationFn: (projectId: string) => deleteProject(apiOptions, projectId),
    onMutate: () => {
      setDeleteBlocked(undefined);
    },
    onSuccess: async () => {
      const projectId = effectiveProjectId;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
        queryClient.invalidateQueries({ queryKey: ["workflow-instances"] }),
        ...(projectId
          ? [queryClient.invalidateQueries({ queryKey: ["project", projectId] })]
          : []),
      ]);
      handleDeleteDialogOpenChange(false);
      await navigate({ to: "/projects" });
    },
    onError: (error) => {
      if (isProjectDeleteBlockedError(error)) {
        setDeleteBlocked(error.payload);
      }
    }
});

  const isInitialLoading = projectsQuery.isLoading && !projectsQuery.data;
  const overview =
    overviewQuery.data?.project.id === effectiveProjectId
      ? overviewQuery.data
      : undefined;
  const selectedProjectDetail =
    selectedProject?.id === effectiveProjectId ? selectedProject : undefined;
  // Prefer overview.project for live status fields, but keep allowed_actions from
  // getProject — overview payloads do not include action gates.
  const displayedProject = overview?.project
    ? {
        ...overview.project,
        allowed_actions:
          selectedProjectDetail?.allowed_actions ?? overview.project.allowed_actions
}
    : selectedProjectDetail;
  const isArchived = displayedProject?.status === "archived";
  const projectRouteDecisions = (routeDecisionsQuery.data ?? []).filter(
    (decision) => decision.project_id === effectiveProjectId,
  );
  const projectPlanRevisions = (planRevisionsQuery.data ?? []).filter(
    (revision) => revision.project_id === effectiveProjectId,
  );
  const projectCoordinationJobs = (coordinationJobsQuery.data ?? []).filter(
    (job) => job.project_id === effectiveProjectId,
  );
  const projectDecisionRequests = (decisionRequestsQuery.data ?? []).filter(
    (decision) => decision.project_id === effectiveProjectId,
  );
  const projectExecutionSummaries = (executionSummariesQuery.data ?? []).filter(
    (summary) => summary.project_id === effectiveProjectId,
  );
  const hasExecutionTraceForSelectedProject =
    executionTraceQuery.data?.project_id === effectiveProjectId;
  const projectExecutionTrace: ProjectExecutionTrace | undefined =
    hasExecutionTraceForSelectedProject
      ? executionTraceQuery.data
      : undefined;
  const projectExecutionTraceIsLoading =
    Boolean(effectiveProjectId) &&
    !hasExecutionTraceForSelectedProject &&
    (executionTraceQuery.isLoading || executionTraceQuery.isFetching);
  const projectExecutionTraceIsError =
    Boolean(effectiveProjectId) &&
    !hasExecutionTraceForSelectedProject &&
    executionTraceQuery.isError;
  const projectExecutionTraceErrorMessage = projectExecutionTraceIsError
    ? queryErrorMessage(executionTraceQuery.error)
    : undefined;
  const projectTransferRequests = (transferRequestsQuery.data ?? []).filter(
    (request) => request.project_id === effectiveProjectId,
  );
  const projectTasks = (tasksQuery.data ?? []).filter(
    (task) => task.project_id === effectiveProjectId,
  );
  const projectEvents = (eventsQuery.data ?? []).filter(
    (event) => event.project_id === effectiveProjectId,
  );
  const projectDemands = (demandsQuery.data ?? []).filter(
    (demand) => demand.project_id === effectiveProjectId,
  );
  const projectEvidence = (evidenceQuery.data ?? []).filter(
    (evidence) => evidence.project_id === effectiveProjectId,
  );
  const projectArtifacts = (artifactsQuery.data ?? []).filter(
    (artifact) => artifact.project_id === effectiveProjectId,
  );
  const projectReports = (reportsQuery.data ?? []).filter(
    (report) => report.project_id === effectiveProjectId,
  );
  const projectBudgetLedger = (budgetLedgerQuery.data ?? []).filter(
    (entry) => entry.project_id === effectiveProjectId,
  );
  const budgetSummaryData = budgetSummaryQuery.data;
  const projectBudgetSummary =
    effectiveProjectId &&
    budgetSummaryData &&
    budgetSummaryData.projectId === effectiveProjectId
      ? budgetSummaryData.summary
      : undefined;
  const acceptanceData = acceptanceQuery.data;
  const projectAcceptance =
    effectiveProjectId &&
    acceptanceData &&
    acceptanceData.project_id === effectiveProjectId
      ? acceptanceData
      : undefined;
  const projectArchivePreview =
    archivePreviewQuery.data?.project_id === effectiveProjectId
      ? archivePreviewQuery.data
      : undefined;
  const projectArchiveSnapshots = (archiveSnapshotsQuery.data ?? []).filter(
    (snapshot) => snapshot.project_id === effectiveProjectId,
  );
  const projectDispatchGates = (dispatchGatesQuery.data?.items ?? []).filter(
    (gate) => gate.project_task_id === dispatchGateTaskId,
  );
  const projectRuntimeNodes = runtimeNodesQuery.data ?? [];
  const projectHeaderBack = routeProjectId ? (
    <ShellPageHeaderBack ariaLabel="返回项目管理" to="/projects" />
  ) : undefined;
  const projectCreateAction = routeProjectId ? null : (
    <Button asChild className="h-11 self-start px-5">
      <Link to="/projects/new">
        <Plus data-icon="inline-start" />
        新建项目
      </Link>
    </Button>
  );
  const projectName = displayedProject?.name ?? "";
  const deletePreview = deletePreviewQuery.data;
  const deletePreviewBlocked = deletePreview?.can_delete === false;
  const deleteConfirmReady = deleteConfirmation === projectName;
  const genericDeleteError =
    deleteProjectMutation.isError && !deleteBlocked
      ? getProjectDeleteErrorMessage(deleteProjectMutation.error)
      : undefined;

  const handleDeleteDialogOpenChange = (open: boolean) => {
    setDeleteDialogOpen(open);
    if (!open) {
      setDeleteConfirmation("");
      setDeleteBlocked(undefined);
      deleteProjectMutation.reset();
    }
  };

  return (
    <>
      <ShellPageHeader
        back={projectHeaderBack}
        icon={routeProjectId ? undefined : <FolderKanban />}
        iconTone="brand"
        title={routeProjectId ? "项目详情" : "项目管理"}
        subtitle={
          routeProjectId
            ? undefined
            : "查看项目组合状态与关注信号，进入项目推进闭环。"
        }
      />
      <Main
        className="min-w-0 overflow-x-hidden"
        width={routeProjectId ? "wide" : "canvas"}
      >
        <div className="flex min-w-0 flex-col gap-5">
          {isInitialLoading ? (
            <WorkSurface>
              <LoadingState label="加载项目列表…" />
            </WorkSurface>
          ) : projectsQuery.isError ? (
            <WorkSurface className="p-4">
              <ErrorState
                title="项目列表加载失败"
                description={queryErrorMessage(projectsQuery.error)}
                onRetry={() => void projectsQuery.refetch()}
              />
            </WorkSurface>
          ) : isProjectPortfolioEmpty ? (
            <WorkSurface className="p-8">
              <EmptyState
                action={
                  <Button asChild className="h-11 px-5">
                    <Link to="/projects/new">
                      <Plus data-icon="inline-start" />
                      新建首个项目
                    </Link>
                  </Button>
                }
                description="项目是目标、负责人、任务、证据、预算和验收结论的业务闭环容器。创建后可在此队列按需要介入程度巡检推进。"
                icon={<FolderKanban />}
                title="还没有项目"
              />
            </WorkSurface>
          ) : (
            <div
              className={
                routeProjectId
                  ? "contents"
                  : "grid min-w-0 gap-4"
              }
              data-testid={routeProjectId ? undefined : "projects-compact-control-surface"}
            >
              {!routeProjectId ? (
                <ProjectPortfolioSummaryBar
                  portfolioCounts={portfolioCounts}
                  totalLabel="已加载"
                />
              ) : null}

              {!routeProjectId ? (
                <MasterDetailLayout
                  data-testid="projects-risk-home-layout"
                  detail={
                    selectedQueueProject ? (
                      <ProjectTriagePanel
                        detailState={
                          triageRiskSignals.summaries[selectedQueueProject.id]
                            ?.state ?? "pending"
                        }
                        onClose={() => setSelectedQueueProjectId("")}
                        principalNamesById={principalNamesById}
                        project={selectedQueueProject}
                        summary={selectedQueueSummary}
                      />
                    ) : (
                      <ProjectPortfolioPerspectivePanel
                        completedTodayCount={
                          runSummaryQuery.data?.today_completed_run_count
                        }
                        onSelectProject={setSelectedQueueProjectId}
                        projects={projects}
                        riskSummaries={displayedRiskSummaries}
                        runSummaryItems={runSummaryQuery.data?.items}
                      />
                    )
                  }
                  detailLabel="项目组合透视"
                  narrowDetail={selectedQueueProject ? "sheet" : "stack"}
                  onDetailDismiss={() => setSelectedQueueProjectId("")}
                  master={
                    <ProjectRiskQueue
                      activePage={activeProjectListPage}
                      createAction={
                        <div className="flex flex-wrap items-center gap-2">
                          {/* 弱链：不带数字，计数唯一出口是侧栏收件箱角标。 */}
                          <Button asChild className="h-11 px-4" variant="ghost">
                            <Link to="/inbox">我的待办</Link>
                          </Button>
                          {projectCreateAction}
                        </div>
                      }
                      filters={filters}
                      isFetching={
                        projectsQuery.isFetching ||
                        runSummaryQuery.isFetching ||
                        isListRiskSettling
                      }
                      listCapped={projects.length >= 50}
                      loadedRiskCounts={loadedRiskCounts}
                      onFiltersChange={setFilters}
                      onPageChange={setProjectListPage}
                      onPageSizeChange={(size) => {
                        setProjectListPageSize(size);
                        setProjectListPage(1);
                      }}
                      onSelectProject={setSelectedQueueProjectId}
                      pageCount={projectListPageCount}
                      pageSize={projectListPageSize}
                      principalNamesById={principalNamesById}
                      projects={pagedProjects}
                      // 队列恒用列表（run-summary）口径：选中项若换成明细摘要，该行会
                      // 单独多出「等待超时」等明细专有信号，同一张表里行与行不可比。
                      riskSummaries={listRiskSummaries}
                      selectedProjectId={selectedQueueProjectId}
                      total={projects.length}
                      visibleTotal={riskFilteredProjects.length}
                    />
                  }
                  rail="lg"
                />
              ) : (
                <div
                  className="grid min-w-0 items-start gap-5"
                  data-testid="projects-risk-home-layout"
                >
                  <ProjectOperationalDetail
                    acceptance={projectAcceptance}
                    apiBaseUrl={apiBaseUrl}
                    apiOptions={apiOptions}
                    archivePreview={projectArchivePreview}
                    archiveSnapshots={projectArchiveSnapshots}
                    artifacts={projectArtifacts}
                    budgetLedger={projectBudgetLedger}
                    budgetSummary={projectBudgetSummary}
                    coordinationJobs={projectCoordinationJobs}
                    decisionRequests={projectDecisionRequests}
                    demands={projectDemands}
                    dispatchGateTaskTitle={dispatchGateTask?.title}
                    dispatchGates={projectDispatchGates}
                    evidence={projectEvidence}
                    events={projectEvents}
                    executionTrace={projectExecutionTrace}
                    executionTraceErrorMessage={projectExecutionTraceErrorMessage}
                    executionTraceIsError={projectExecutionTraceIsError}
                    executionTraceIsLoading={projectExecutionTraceIsLoading}
                    executionSummaries={projectExecutionSummaries}
                    fetchTaskGraph={(demandId) =>
                      getProjectTaskGraph(apiOptions, effectiveProjectId as string, {
                        demandId
})
                    }
                    focusDecisionId={search.focus}
                    demandView={search.view === "graph" ? "graph" : "timeline"}
                    initialDemandId={search.demand}
                    onDemandViewChange={(view) => {
                      // 视图进 URL：刷新不丢、深链能直接指到图。
                      void navigate({
                        params: { projectId: effectiveProjectId as string },
                        search: (prev: Record<string, unknown>) => ({
                          ...prev,
                          tab: "demands",
                          view: view === "graph" ? "graph" : undefined
                        }),
                        to: "/projects/$projectId"
                      });
                    }}
                    initialTab={isProjectOperationalTab(search.tab) ? search.tab : undefined}
                    traceTaskId={search.task}
                    isArchived={isArchived}
                    onArchiveProject={() => {
                      archiveMutation.reset();
                      setArchiveDialogOpen(true);
                      void archivePreviewQuery.refetch();
                    }}
                    onUnarchiveProject={() => {
                      unarchiveMutation.reset();
                      setUnarchiveDialogOpen(true);
                    }}
                    onDeleteProject={() => setDeleteDialogOpen(true)}
                    onRecloneWorkspace={() => {
                      if (effectiveProjectId) {
                        recloneWorkspaceMutation.mutate(effectiveProjectId);
                      }
                    }}
                    onMarkWorkspaceReady={() => {
                      if (effectiveProjectId) {
                        markWorkspaceReadyMutation.mutate(effectiveProjectId);
                      }
                    }}
                    workspaceActionPending={
                      recloneWorkspaceMutation.isPending ||
                      markWorkspaceReadyMutation.isPending
                    }
                    onCreateArchiveSnapshot={(input) => {
                      if (effectiveProjectId) {
                        createArchiveSnapshotMutation.mutate(input);
                      }
                    }}
                    onCreateEvidence={(input) => {
                      if (effectiveProjectId) {
                        createEvidenceMutation.mutate(input);
                      }
                    }}
                    onPatchEvidence={(evidenceId, verificationStatus) => {
                      if (effectiveProjectId) {
                        patchEvidenceMutation.mutate({ evidenceId, verificationStatus });
                      }
                    }}
                    onRetryExecutionTrace={() => {
                      void executionTraceQuery.refetch();
                    }}
                    onResolveDecision={(decisionId, decision, targetExitDeliverable) => {
                      if (effectiveProjectId) {
                        resolveDecisionMutation.mutate({
                          decisionId,
                          decision,
                          targetExitDeliverable
});
                      }
                    }}
                    dismissTaskPending={dismissTaskMutation.isPending}
                    onDismissTask={(taskId) => {
                      if (effectiveProjectId) {
                        dismissTaskMutation.mutate(taskId);
                      }
                    }}
                    onSubmitDemand={() => setDemandOpen(true)}
                    overview={overview}
                    principalNamesById={principalNamesById}
                    usersById={usersById}
                    project={displayedProject}
                    reports={projectReports}
                    planRevisions={projectPlanRevisions}
                    routeDecisions={projectRouteDecisions}
                    runtimePlacementPanel={
                      <ProjectRuntimePlacementPanel
                        isBinding={bindRuntimePlacementMutation.isPending}
                        isReadinessLoading={
                          runtimeReadinessQuery.isLoading || runtimeReadinessQuery.isFetching
                        }
                        isReleasing={releaseRuntimePlacementMutation.isPending}
                        readiness={currentProjectRuntimeReadiness}
                        runtimeNodes={projectRuntimeNodes}
                        selectedRuntimeNodeId={selectedRuntimeNodeId}
                        onBindRuntime={() => {
                          if (effectiveProjectId && selectedRuntimeNodeId) {
                            bindRuntimePlacementMutation.mutate(selectedRuntimeNodeId);
                          }
                        }}
                        onReleaseRuntime={() => {
                          const boundNodeId =
                            currentProjectRuntimeReadiness?.runtime_node_id ?? selectedRuntimeNodeId;
                          if (effectiveProjectId && boundNodeId) {
                            releaseRuntimePlacementMutation.mutate(boundNodeId);
                          }
                        }}
                        onSelectedRuntimeNodeIdChange={setSelectedRuntimeNodeId}
                      />
                    }
                    taskGraph={taskGraphQuery.data}
                    tasks={projectTasks}
                    transferRequests={projectTransferRequests}
                  />
                </div>
              )}
            </div>
          )}
      <SubmitDemandDialog
        isSubmitting={submitDemandMutation.isPending}
        open={demandOpen}
        projectId={selectedProject?.id}
        projectName={selectedProject?.name}
        submitError={submitDemandMutation.error?.message}
        onOpenChange={setDemandOpen}
        onSubmit={(input) => submitDemandMutation.mutate(input)}
      />
      {displayedProject ? (
        <ConfirmDialog
          cancelBtnText="取消"
          confirmText="确认归档"
          desc={
            <div className="space-y-2">
              <p>
                确认归档项目「{displayedProject.name}
                」？需无未完结任务、未结需求与待决决策；归档后停止推进、配置与需求提交将被禁用，可从菜单「恢复项目」重新打开（不复活归档时取消的待办）。
              </p>
              {archivePreviewQuery.isError ? (
                <p className="text-sm text-danger">
                  无法加载归档预检：{queryErrorMessage(archivePreviewQuery.error)}
                </p>
              ) : null}
              {archivePreviewQuery.isFetching && !archivePreviewQuery.data ? (
                <p className="text-sm text-ink-2">正在检查归档条件…</p>
              ) : null}
              {archivePreviewQuery.data?.message ? (
                <p className="text-sm text-ink-2">{archivePreviewQuery.data.message}</p>
              ) : null}
              {(archivePreviewQuery.data?.blockers?.length ?? 0) > 0 ? (
                <ul className="list-disc space-y-1 pl-5 text-sm text-danger">
                  {archivePreviewQuery.data?.blockers.map((blocker) => (
                    <li key={`archive-blocker-${blocker.code}-${blocker.count}`}>
                      {blocker.message || archiveReadinessCodeLabel(blocker.code)}
                    </li>
                  ))}
                </ul>
              ) : null}
              {(archivePreviewQuery.data?.warnings?.length ?? 0) > 0 ? (
                <ul className="list-disc space-y-1 pl-5 text-sm text-ink-2">
                  {archivePreviewQuery.data?.warnings.map((warning) => (
                    <li key={`archive-warning-${warning.code}-${warning.count}`}>
                      {warning.message || archiveReadinessCodeLabel(warning.code)}
                    </li>
                  ))}
                </ul>
              ) : null}
              {archiveMutation.isError ? (
                <p className="text-sm text-danger">
                  {archiveErrorMessage(archiveMutation.error)}
                </p>
              ) : null}
            </div>
          }
          destructive
          disabled={
            archiveMutation.isPending ||
            archivePreviewQuery.isError ||
            // 首屏尚无 preview 时禁止；已有 can_archive=true 时允许点（后台 refetch 不挡）。
            // 前两个条件已完全覆盖 isLoading（加载中必然无 data），多余的第三条会把
            // query 联合类型收敛成 never。
            !archivePreviewQuery.data ||
            archivePreviewQuery.data.can_archive !== true
          }
          handleConfirm={() => {
            if (effectiveProjectId && archivePreviewQuery.data?.can_archive === true) {
              archiveMutation.mutate(effectiveProjectId);
            }
          }}
          isLoading={archiveMutation.isPending}
          onOpenChange={setArchiveDialogOpen}
          open={archiveDialogOpen}
          title="归档项目"
        />
      ) : null}
      {displayedProject ? (
        <ConfirmDialog
          cancelBtnText="取消"
          confirmText="确认恢复"
          desc={
            <div className="space-y-2">
              <p>
                确认将项目「{displayedProject.name}
                」从归档恢复为运行中？仅恢复项目状态，不重开历史需求/任务，也不复活归档时取消的收件箱待办。
              </p>
              {unarchiveMutation.isError ? (
                <p className="text-sm text-danger">
                  {queryErrorMessage(unarchiveMutation.error)}
                </p>
              ) : null}
            </div>
          }
          disabled={unarchiveMutation.isPending}
          handleConfirm={() => {
            if (effectiveProjectId) {
              unarchiveMutation.mutate(effectiveProjectId);
            }
          }}
          isLoading={unarchiveMutation.isPending}
          onOpenChange={setUnarchiveDialogOpen}
          open={unarchiveDialogOpen}
          title="恢复项目"
        />
      ) : null}
      {displayedProject ? (
        <ConfirmDialog
          cancelBtnText="取消"
          className="sm:max-w-xl"
          confirmText="确认删除"
          desc={
            <ProjectDeleteDialogDescription
              isLoading={deletePreviewQuery.isLoading}
              preview={deletePreview}
            />
          }
          destructive
          disabled={
            !deleteConfirmReady ||
            deleteProjectMutation.isPending ||
            deletePreviewQuery.isLoading ||
            deletePreviewBlocked
          }
          form="delete-project-form"
          isLoading={deleteProjectMutation.isPending}
          onOpenChange={handleDeleteDialogOpenChange}
          open={deleteDialogOpen}
          title="删除项目"
        >
          <form
            className="space-y-4"
            id="delete-project-form"
            onSubmit={(event) => {
              event.preventDefault();
              if (deleteConfirmReady && effectiveProjectId && !deletePreviewBlocked) {
                deleteProjectMutation.mutate(effectiveProjectId);
              }
            }}
          >
            <div className="space-y-2">
              <div className="space-y-1.5">
                <p className="text-sm font-medium text-ink">项目名称</p>
                <CopyableProjectName name={projectName} />
                <p className="text-xs text-ink-3">点击上方名称可复制，粘贴或手动输入后确认删除。</p>
              </div>
              <Label htmlFor="delete-project-confirmation">输入项目名称确认删除</Label>
              <Input
                autoComplete="off"
                id="delete-project-confirmation"
                onChange={(event) => {
                  setDeleteConfirmation(event.currentTarget.value);
                  if (deleteBlocked) setDeleteBlocked(undefined);
                }}
                placeholder="粘贴或输入完整项目名称"
                value={deleteConfirmation}
              />
            </div>
            {deleteBlocked ? <ProjectDeleteBlockedAlert blocked={deleteBlocked} /> : null}
            {deletePreviewBlocked && deletePreview ? (
              <ProjectDeleteBlockedAlert
                blocked={{
                  blockers: deletePreview.blockers,
                  code: "project_delete_blocked",
                  message: deletePreview.message
}}
              />
            ) : null}
            {genericDeleteError ? (
              <p className="text-sm text-danger">{genericDeleteError}</p>
            ) : null}
          </form>
        </ConfirmDialog>
      ) : null}
        </div>
      </Main>
    </>
  );
}

function archiveErrorMessage(error: unknown): string {
  if (error instanceof ApiRequestError) {
    const payload = error.payload as
      | { message?: string; blockers?: Array<{ message?: string; code?: string }> }
      | undefined;
    if (error.code === "project_archive_blocked" || payload?.blockers?.length) {
      const parts = (payload?.blockers ?? [])
        .map((item) => item.message || item.code)
        .filter(Boolean);
      if (parts.length > 0) {
        return parts.join("；");
      }
      if (error.detail?.trim()) {
        return error.detail;
      }
      return "项目未达归档条件，无法归档";
    }
  }
  return queryErrorMessage(error);
}

function queryErrorMessage(error: unknown) {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return "执行证据链加载失败";
}

/** 创建项目失败时的用户文案；同名冲突映射为明确中文提示。 */
function createProjectSubmitErrorMessage(error: unknown): string {
  if (error instanceof ApiRequestError) {
    const detail = error.detail ?? "";
    if (
      error.status === 409 &&
      (detail.includes("project name already exists") ||
        detail.includes("已被使用") ||
        /项目名.+已被使用/.test(detail))
    ) {
      return "项目名已被使用，请更换为全局唯一的目录名。";
    }
    if (detail.trim()) {
      return detail;
    }
  }
  return apiErrorMessage(error, "创建项目失败，请稍后重试");
}

function isProjectOperationalTab(value: string | undefined): boolean {
  return (
    value === "workbench" ||
    value === "overview" ||
    value === "tasks" ||
    value === "artifacts" ||
    value === "approval" ||
    value === "budget" ||
    value === "acceptance" ||
    value === "closure" ||
    value === "config" ||
    value === "assets" ||
    value === "trace" ||
    value === "demands"
  );
}

function selectDispatchGateTask({
  fallbackTasks,
  graphTasks,
  projectId
}: {
  fallbackTasks: ProjectTask[];
  graphTasks: ProjectTask[];
  projectId?: string;
}) {
  if (!projectId) {
    return undefined;
  }
  const candidates = [fallbackTasks, graphTasks]
    .flat()
    .filter((task) => task.project_id === projectId);
  return (
    candidates.find((task) => dispatchGateCandidateStatus(task.status)) ?? candidates[0]
  );
}

function dispatchGateCandidateStatus(status: string) {
  return !["accepted", "approved", "cancelled", "completed", "done", "success"].includes(
    status,
  );
}

function isProjectDeleteBlockedError(
  error: unknown,
): error is ApiRequestError & { payload: ProjectDeleteBlockedErrorResponse } {
  return (
    error instanceof ApiRequestError &&
    error.status === 409 &&
    error.code === "project_delete_blocked" &&
    isProjectDeleteBlockedPayload(error.payload)
  );
}

function isProjectDeleteBlockedPayload(
  payload: unknown,
): payload is ProjectDeleteBlockedErrorResponse {
  if (!payload || typeof payload !== "object") return false;
  const value = payload as Partial<ProjectDeleteBlockedErrorResponse>;
  return (
    value.code === "project_delete_blocked" &&
    typeof value.message === "string" &&
    Array.isArray(value.blockers)
  );
}

function getProjectDeleteErrorMessage(error: unknown) {
  if (error instanceof ApiRequestError && error.detail) return error.detail;
  if (error instanceof Error) return error.message;
  return "删除失败，请稍后重试。";
}

function CopyableProjectName({ name }: { name: string }) {
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (resetTimer.current) {
        clearTimeout(resetTimer.current);
      }
    },
    [],
  );

  if (!name) {
    return (
      <span className="block rounded-inner border border-dashed border-line bg-card-soft px-3 py-2 text-sm text-ink-3">
        加载项目名称…
      </span>
    );
  }

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(name);
      setCopied(true);
      if (resetTimer.current) {
        clearTimeout(resetTimer.current);
      }
      resetTimer.current = setTimeout(() => setCopied(false), 1500);
    } catch {
      // 剪贴板不可用时静默降级，用户仍可手动选中输入
    }
  };

  return (
    <button
      type="button"
      title={copied ? "已复制" : "点击复制项目名称"}
      aria-label={`复制项目名称 ${name}`}
      onClick={() => void copy()}
      className={cn(
        "flex w-full min-w-0 items-center justify-between gap-2 rounded-inner border border-line bg-card-soft px-3 py-2 text-left text-sm text-ink-3 transition-colors",
        "hover:border-line-strong hover:text-ink-2",
      )}
    >
      <span className="min-w-0 break-all select-all">{name}</span>
      {copied ? (
        <Check aria-hidden className="size-3.5 shrink-0 text-ok" />
      ) : (
        <Copy aria-hidden className="size-3.5 shrink-0 opacity-70" />
      )}
    </button>
  );
}

function ProjectDeleteDialogDescription({
  isLoading,
  preview
}: {
  isLoading: boolean;
  preview?: ProjectDeletePreview;
}) {
  if (isLoading) {
    return <p>正在加载删除影响预览…</p>;
  }

  const warnings = preview ? formatProjectDeleteWarnings(preview) : [];

  return (
    <div className="space-y-2">
      <p>
        删除后项目会从项目列表中隐藏；历史任务、工件、证据和审计记录会按策略保留，协调线程会终止。
      </p>
      {warnings.length > 0 ? (
        <ul className="list-disc space-y-1 pl-5">
          {warnings.map((warning) => (
            <li key={warning}>{warning}</li>
          ))}
        </ul>
      ) : null}
      {preview?.message && preview.can_delete !== false ? <p>{preview.message}</p> : null}
    </div>
  );
}

function formatProjectDeleteWarnings(preview: ProjectDeletePreview) {
  const warnings = preview.warnings;
  const items: string[] = [];
  if (warnings.pending_decision_count) {
    items.push(`仍有 ${warnings.pending_decision_count} 项待处理决策`);
  }
  if (warnings.waiting_human_task_count) {
    items.push(`仍有 ${warnings.waiting_human_task_count} 个等待人工处理的任务`);
  }
  if (warnings.open_inbox_count) {
    items.push(`仍有 ${warnings.open_inbox_count} 条未处理收件箱事项`);
  }
  if (warnings.active_member_count) {
    items.push(`仍有 ${warnings.active_member_count} 位活跃项目成员`);
  }
  if (warnings.digital_employee_member_count) {
    items.push(`仍有 ${warnings.digital_employee_member_count} 位数字员工在项目服务池中`);
  }
  if (warnings.runtime_node_binding_count) {
    items.push(`仍绑定 ${warnings.runtime_node_binding_count} 个运行节点`);
  }
  if (warnings.affinity_count) {
    items.push(`仍有 ${warnings.affinity_count} 条员工亲和绑定`);
  }
  if (warnings.automation_rule_count) {
    items.push(`仍有 ${warnings.automation_rule_count} 条自动化规则将一并删除`);
  }
  return items;
}

function ProjectDeleteBlockedAlert({
  blocked
}: {
  blocked: ProjectDeleteBlockedErrorResponse;
}) {
  return (
    <Callout tone="danger" title="删除被阻断" icon={<AlertTriangle aria-hidden className="size-4" />}>
      <p>{blocked.message}</p>
      <ul className="mt-3 space-y-2">
          {blocked.blockers.map((blocker) => (
            <ProjectDeleteBlockerItem blocker={blocker} key={`${blocker.type}:${blocker.id}`} />
          ))}
        </ul>
      </Callout>
  );
}

function ProjectDeleteBlockerItem({ blocker }: { blocker: ProjectDeleteBlocker }) {
  return (
    <li className="rounded-inner border border-danger/25 bg-card px-3 py-2 text-ink">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold">{blocker.title}</span>
        <StatusPill tone="danger">{`${deleteBlockerTypeLabel(blocker.type)} · ${statusLabel(blocker.status)}`}</StatusPill>
      </div>
      <p className="mt-1 break-all font-mono text-[11px] text-ink-3">id {blocker.id}</p>
    </li>
  );
}
