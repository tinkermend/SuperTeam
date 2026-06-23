import { useEffect, useMemo, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Archive,
  CircleDot,
  ClipboardList,
  FolderKanban,
  ListChecks,
  Plus,
  type LucideIcon,
} from "lucide-react";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import {
  getCurrentUser,
  listUserProjectTeamScopes,
  type UserProjectTeamScope,
} from "@/lib/api";
import {
  archiveProject,
  createProject,
  createProjectAcceptance,
  createProjectArchiveSnapshot,
  createProjectEvidence,
  getProject,
  getProjectAcceptance,
  getProjectArchivePreview,
  getProjectBudgetSummary,
  getProjectExecutionTrace,
  getProjectOverview,
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
  patchProjectEvidence,
  resolveProjectDecision,
  submitProjectDemand,
  type CreateProjectAcceptanceInput,
  type CreateProjectArchiveSnapshotInput,
  type CreateProjectEvidenceInput,
  type CreateProjectInput,
  type ListProjectsFilters,
  type Project,
  type ProjectEvidenceVerificationStatus,
  type ProjectExecutionTrace,
  type ProjectTask,
  type ProjectStatus,
  type SubmitProjectDemandInput,
} from "@/lib/api/projects";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import {
  IconTile,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  V3Pagination,
  V3Table,
  V3Td,
  V3Th,
  V3ToolbarSearch,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import { ProjectOperationalDetail } from "./components/project-operational-detail";
import { CreateProjectDrawer } from "./components/create-project-drawer";
import { SubmitDemandDialog } from "./components/submit-demand-dialog";
import { ProjectConfigView } from "./components/project-config-page";

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
  status: "all" | ProjectStatus;
};

const statusOptions: Array<{ label: string; value: UiProjectListFilters["status"] }> = [
  { label: "全部状态", value: "all" },
  { label: "运行中", value: "running" },
  { label: "配置中", value: "configuring" },
  { label: "验收中", value: "acceptance" },
  { label: "已暂停", value: "paused" },
  { label: "已归档", value: "archived" },
];

export function ProjectsPage({ fetcher }: ProjectsPageProps = {}) {
  return <ProjectsView apiBaseUrl={resolveControlPlaneUrl()} fetcher={fetcher} />;
}

export function ProjectDetailPage({
  fetcher,
  projectId,
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
  projectId,
}: ProjectsPageProps & { projectId: string }) {
  return (
    <ProjectConfigView
      apiBaseUrl={resolveControlPlaneUrl()}
      fetcher={fetcher}
      projectId={projectId}
    />
  );
}

export function ProjectsView({
  apiBaseUrl,
  fetcher,
  routeProjectId,
}: ProjectsViewProps) {
  const queryClient = useQueryClient();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const [filters, setFilters] = useState<UiProjectListFilters>({
    q: "",
    status: "all",
  });
  const [selectedProjectId, setSelectedProjectId] = useState(routeProjectId);
  const [createOpen, setCreateOpen] = useState(false);
  const [demandOpen, setDemandOpen] = useState(false);
  const [projectListPage, setProjectListPage] = useState(1);
  const [projectListPageSize, setProjectListPageSize] = useState(10);

  useEffect(() => {
    if (routeProjectId) {
      setSelectedProjectId(routeProjectId);
    }
  }, [routeProjectId]);

  const listFilters = useMemo<ListProjectsFilters>(() => {
    const request: ListProjectsFilters = { limit: 50, offset: 0 };
    if (filters.q.trim()) {
      request.q = filters.q.trim();
    }
    if (filters.status !== "all") {
      request.status = filters.status as ProjectStatus;
    }
    return request;
  }, [filters]);

  const projectsQuery = useQuery({
    queryKey: ["projects", listFilters],
    queryFn: () => listProjects(apiOptions, listFilters),
    placeholderData: keepPreviousData,
  });
  const projects = projectsQuery.data ?? [];
  const projectListPageCount = Math.max(1, Math.ceil(projects.length / projectListPageSize));
  const activeProjectListPage = Math.min(projectListPage, projectListPageCount);
  const pagedProjects = projects.slice(
    (activeProjectListPage - 1) * projectListPageSize,
    activeProjectListPage * projectListPageSize,
  );
  const projectStats = useMemo(() => buildProjectStats(projects), [projects]);

  useEffect(() => {
    setProjectListPage(1);
  }, [filters.q, filters.status]);

  const selectedProjectFromList = selectedProjectId
    ? projects.find((project) => project.id === selectedProjectId)
    : undefined;

  const selectedProjectQuery = useQuery({
    enabled: Boolean(selectedProjectId) && !selectedProjectFromList,
    queryKey: ["project", selectedProjectId],
    queryFn: () => getProject(apiOptions, selectedProjectId as string),
    placeholderData: keepPreviousData,
  });

  const selectedProject =
    selectedProjectFromList ??
    (selectedProjectQuery.data?.id === selectedProjectId
      ? selectedProjectQuery.data
      : undefined);
  const effectiveProjectId = selectedProjectId;

  const currentUserQuery = useQuery({
    enabled: createOpen,
    queryKey: ["auth", "current-user", "project-create"],
    queryFn: () => getCurrentUser(apiOptions),
    refetchOnMount: "always",
    staleTime: 0,
  });
  const currentUserId = currentUserQuery.data?.user.id;

  const projectTeamScopesQuery = useQuery({
    enabled: createOpen && Boolean(currentUserId),
    queryKey: ["auth", "users", currentUserId, "project-team-scopes", "project-create"],
    queryFn: () => listUserProjectTeamScopes(apiOptions, currentUserId as string),
    refetchOnMount: "always",
    staleTime: 0,
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

  useEffect(() => {
    if (routeProjectId || projects.length === 0) {
      if (!routeProjectId && projects.length === 0) {
        setSelectedProjectId(undefined);
      }
      return;
    }

    if (!selectedProjectId || !projects.some((project) => project.id === selectedProjectId)) {
      setSelectedProjectId(projects[0].id);
    }
  }, [projects, routeProjectId, selectedProjectId]);

  const overviewQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-overview", effectiveProjectId],
    queryFn: () => getProjectOverview(apiOptions, effectiveProjectId as string),
    placeholderData: keepPreviousData,
  });

  const tasksQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-tasks", effectiveProjectId],
    queryFn: () => listProjectTasks(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData,
  });

  const eventsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-events", effectiveProjectId],
    queryFn: () => listProjectEvents(apiOptions, effectiveProjectId as string, { limit: 30 }),
    placeholderData: keepPreviousData,
  });

  const demandsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-demands", effectiveProjectId],
    queryFn: () => listProjectDemands(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData,
  });

  const routeDecisionsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-route-decisions", effectiveProjectId],
    queryFn: () =>
      listProjectRouteDecisions(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData,
  });

  const latestDemandId = demandsQuery.data?.[0]?.id;
  const planRevisionsQuery = useQuery({
    enabled: Boolean(effectiveProjectId) && Boolean(latestDemandId),
    queryKey: ["project-plan-revisions", effectiveProjectId, latestDemandId],
    queryFn: () =>
      listProjectPlanRevisions(apiOptions, effectiveProjectId as string, {
        demandId: latestDemandId as string,
        limit: 10,
      }),
    placeholderData: keepPreviousData,
  });

  const taskGraphQuery = useQuery({
    enabled: Boolean(effectiveProjectId) && Boolean(latestDemandId),
    queryKey: ["project-task-graph", effectiveProjectId, latestDemandId],
    queryFn: () =>
      getProjectTaskGraph(apiOptions, effectiveProjectId as string, {
        demandId: latestDemandId as string,
    }),
    placeholderData: keepPreviousData,
  });

  const dispatchGateTask = selectDispatchGateTask({
    activeTasks: overviewQuery.data?.active_tasks ?? [],
    fallbackTasks: tasksQuery.data ?? [],
    graphTasks: taskGraphQuery.data?.nodes ?? [],
    projectId: effectiveProjectId,
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
    placeholderData: keepPreviousData,
  });

  const coordinationJobsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-coordination-jobs", effectiveProjectId],
    queryFn: () =>
      listProjectCoordinationJobs(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData,
  });

  const decisionRequestsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-decisions", effectiveProjectId],
    queryFn: () =>
      listProjectDecisionRequests(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData,
  });

  const executionSummariesQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-execution-summaries", effectiveProjectId],
    queryFn: () =>
      listProjectExecutionSummaries(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData,
  });

  const executionTraceQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-execution-trace", effectiveProjectId],
    queryFn: () =>
      getProjectExecutionTrace(apiOptions, effectiveProjectId as string, { limit: 100 }),
    placeholderData: keepPreviousData,
  });

  const transferRequestsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-transfer-requests", effectiveProjectId],
    queryFn: () =>
      listProjectTransferRequests(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData,
  });

  const evidenceQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-evidence", effectiveProjectId],
    queryFn: () => listProjectEvidence(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData,
  });

  const artifactsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-artifacts", effectiveProjectId],
    queryFn: () => listProjectArtifacts(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData,
  });

  const reportsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-reports", effectiveProjectId],
    queryFn: () => listProjectReports(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData,
  });

  const budgetLedgerQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-budget-ledger", effectiveProjectId],
    queryFn: () =>
      listProjectBudgetLedger(apiOptions, effectiveProjectId as string, { limit: 20 }),
    placeholderData: keepPreviousData,
  });

  const budgetSummaryQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-budget-summary", effectiveProjectId],
    queryFn: async () => {
      const projectId = effectiveProjectId as string;
      const summary = await getProjectBudgetSummary(apiOptions, projectId);
      return { projectId, summary };
    },
    placeholderData: keepPreviousData,
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
    placeholderData: keepPreviousData,
  });

  const archivePreviewQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-archive-preview", effectiveProjectId],
    queryFn: () => getProjectArchivePreview(apiOptions, effectiveProjectId as string),
    placeholderData: keepPreviousData,
  });

  const archiveSnapshotsQuery = useQuery({
    enabled: Boolean(effectiveProjectId),
    queryKey: ["project-archive-snapshots", effectiveProjectId],
    queryFn: () =>
      listProjectArchiveSnapshots(apiOptions, effectiveProjectId as string, { limit: 10 }),
    placeholderData: keepPreviousData,
  });

  const createMutation = useMutation({
    mutationFn: (input: CreateProjectInput) => createProject(apiOptions, input),
    onSuccess: async (response) => {
      setCreateOpen(false);
      setSelectedProjectId(response.project.id);
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      queryClient.setQueryData(["project", response.project.id], response.project);
    },
  });

  const archiveMutation = useMutation({
    mutationFn: (projectId: string) => archiveProject(apiOptions, projectId),
    onSuccess: async (project) => {
      queryClient.setQueryData(["project", project.id], project);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", project.id] }),
      ]);
    },
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
    },
  });

  const resolveDecisionMutation = useMutation({
    mutationFn: (input: { decisionId: string; decision: string }) =>
      resolveProjectDecision(
        apiOptions,
        effectiveProjectId as string,
        input.decisionId,
        { decision: input.decision },
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
    },
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
          queryKey: ["project-archive-preview", projectId],
        }),
      ]);
    },
  });

  const patchEvidenceMutation = useMutation({
    mutationFn: (input: {
      evidenceId: string;
      verificationStatus: ProjectEvidenceVerificationStatus;
    }) =>
      patchProjectEvidence(apiOptions, effectiveProjectId as string, input.evidenceId, {
        verification_status: input.verificationStatus,
      }),
    onSuccess: async (evidence) => {
      const projectId = evidence.project_id || effectiveProjectId;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-evidence", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["project-archive-preview", projectId],
        }),
      ]);
    },
  });

  const createAcceptanceMutation = useMutation({
    mutationFn: (input: CreateProjectAcceptanceInput) =>
      createProjectAcceptance(apiOptions, effectiveProjectId as string, input),
    onSuccess: async (acceptance) => {
      const projectId = acceptance.project_id || effectiveProjectId;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-acceptance", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["project-archive-preview", projectId],
        }),
      ]);
    },
  });

  const createArchiveSnapshotMutation = useMutation({
    mutationFn: (input: CreateProjectArchiveSnapshotInput) =>
      createProjectArchiveSnapshot(apiOptions, effectiveProjectId as string, input),
    onSuccess: async (snapshot) => {
      const projectId = snapshot.project_id || effectiveProjectId;
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ["project-archive-snapshots", projectId],
        }),
        queryClient.invalidateQueries({
          queryKey: ["project-archive-preview", projectId],
        }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
      ]);
    },
  });

  const isInitialLoading = projectsQuery.isLoading && !projectsQuery.data;
  const overview =
    overviewQuery.data?.project.id === effectiveProjectId
      ? overviewQuery.data
      : undefined;
  const displayedProject =
    overview?.project ??
    (selectedProject?.id === effectiveProjectId ? selectedProject : undefined);
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

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-6">
          <header className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="flex min-w-0 items-start gap-3">
              <IconTile tone="brand" size="lg">
                <FolderKanban />
              </IconTile>
              <div className="min-w-0">
                <h1 className="text-[1.7rem] font-extrabold tracking-tight text-v3-ink">
                  项目管理
                </h1>
                <p className="mt-1 text-sm text-v3-ink-2">
                  项目事实、成员池、任务、事件和需求记录
                </p>
              </div>
            </div>
            <V3Button
              className="h-11 self-start px-5"
              type="button"
              onClick={() => setCreateOpen(true)}
            >
              <Plus data-icon="inline-start" />
              新建项目
            </V3Button>
          </header>

          {isInitialLoading ? (
            <WorkSurface>
              <V3LoadingState label="加载项目列表…" />
            </WorkSurface>
          ) : projectsQuery.isError ? (
            <WorkSurface className="p-4">
              <V3ErrorState
                title="项目列表加载失败"
                description={queryErrorMessage(projectsQuery.error)}
                onRetry={() => void projectsQuery.refetch()}
              />
            </WorkSurface>
          ) : (
            <>
              <section
                aria-label="项目管理指标"
                className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4"
              >
                {projectStats.map((metric) => {
                  const Icon = metric.icon;
                  return (
                    <V3MetricCard
                      key={metric.label}
                      icon={<Icon />}
                      iconTone={metric.tone}
                      label={metric.label}
                      loud={metric.loud}
                      value={metric.value}
                    />
                  );
                })}
              </section>

              <div className="grid min-w-0 items-start gap-5 2xl:grid-cols-[minmax(720px,1.05fr)_minmax(0,1fr)]">
                <ProjectsV3List
                  activePage={activeProjectListPage}
                  filters={filters}
                  isFetching={projectsQuery.isFetching}
                  onCreateProject={() => setCreateOpen(true)}
                  onFiltersChange={setFilters}
                  onPageChange={setProjectListPage}
                  onPageSizeChange={(size) => {
                    setProjectListPageSize(size);
                    setProjectListPage(1);
                  }}
                  onSelectProject={setSelectedProjectId}
                  pageCount={projectListPageCount}
                  pageSize={projectListPageSize}
                  projects={pagedProjects}
                  selectedProjectId={effectiveProjectId}
                  total={projects.length}
                />
                <ProjectOperationalDetail
                  acceptance={projectAcceptance}
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
                  isArchived={isArchived}
                  onArchiveProject={() => {
                    if (effectiveProjectId) {
                      archiveMutation.mutate(effectiveProjectId);
                    }
                  }}
                  onCreateAcceptance={(input) => {
                    if (effectiveProjectId) {
                      createAcceptanceMutation.mutate(input);
                    }
                  }}
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
                  onResolveDecision={(decisionId, decision) => {
                    if (effectiveProjectId) {
                      resolveDecisionMutation.mutate({ decisionId, decision });
                    }
                  }}
                  onSubmitDemand={() => setDemandOpen(true)}
                  overview={overview}
                  project={displayedProject}
                  reports={projectReports}
                  planRevisions={projectPlanRevisions}
                  routeDecisions={projectRouteDecisions}
                  taskGraph={taskGraphQuery.data}
                  tasks={projectTasks}
                  transferRequests={projectTransferRequests}
                />
              </div>
            </>
          )}
      <CreateProjectDrawer
        availableTeams={availableProjectTeamScopes}
        currentUserError={currentUserQuery.error?.message}
        currentUserId={currentUserId}
        isCurrentUserLoading={currentUserQuery.isFetching}
        isSubmitting={createMutation.isPending}
        isTeamsLoading={projectTeamScopesQuery.isFetching}
        open={createOpen}
        submitError={createMutation.error?.message}
        teamsError={projectTeamScopesQuery.error?.message}
        onOpenChange={setCreateOpen}
        onSubmit={(input) => createMutation.mutate(input)}
      />
      <SubmitDemandDialog
        isSubmitting={submitDemandMutation.isPending}
        open={demandOpen}
        projectName={selectedProject?.name}
        submitError={submitDemandMutation.error?.message}
        onOpenChange={setDemandOpen}
        onSubmit={(input) => submitDemandMutation.mutate(input)}
      />
        </div>
      </Main>
    </>
  );
}

function ProjectsV3List({
  activePage,
  filters,
  isFetching,
  onCreateProject,
  onFiltersChange,
  onPageChange,
  onPageSizeChange,
  onSelectProject,
  pageCount,
  pageSize,
  projects,
  selectedProjectId,
  total,
}: {
  activePage: number;
  filters: UiProjectListFilters;
  isFetching: boolean;
  onCreateProject: () => void;
  onFiltersChange: (filters: UiProjectListFilters) => void;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  onSelectProject: (projectId: string) => void;
  pageCount: number;
  pageSize: number;
  projects: Project[];
  selectedProjectId?: string;
  total: number;
}) {
  return (
    <section data-testid="projects-v3-list" className="min-w-0" aria-label="项目列表">
      <WorkSurface className="min-w-0">
        <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line p-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <h2 className="text-base font-extrabold text-v3-ink">项目列表</h2>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <StatusPill tone={isFetching ? "info" : "mute"}>
                {isFetching ? "正在刷新" : `${total} 个项目`}
              </StatusPill>
              {filters.status !== "all" ? (
                <StatusPill tone={projectStatusTone(filters.status)}>
                  {projectStatusLabel(filters.status)}
                </StatusPill>
              ) : null}
            </div>
          </div>
          <V3Button type="button" variant="outline" onClick={onCreateProject}>
            <Plus data-icon="inline-start" />
            新建
          </V3Button>
        </div>

        <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line p-4 lg:flex-row lg:items-center">
          <V3ToolbarSearch
            aria-label="搜索项目"
            onChange={(event) => onFiltersChange({ ...filters, q: event.target.value })}
            placeholder="搜索项目、目标"
            type="search"
            value={filters.q}
          />
          <select
            aria-label="项目状态筛选"
            className="h-10 rounded-xl border border-v3-line-strong bg-v3-card-soft px-3 text-[13px] font-semibold text-v3-ink-2 outline-none focus-visible:ring-2 focus-visible:ring-v3-brand/60 lg:w-[136px]"
            value={filters.status}
            onChange={(event) =>
              onFiltersChange({
                ...filters,
                status: event.target.value as UiProjectListFilters["status"],
              })
            }
          >
            {statusOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>

        <ProjectsV3Table
          onSelectProject={onSelectProject}
          projects={projects}
          selectedProjectId={selectedProjectId}
        />
        <V3Pagination
          page={activePage}
          pageCount={pageCount}
          pageSize={pageSize}
          pageSizeOptions={[1, 10, 20, 50]}
          total={total}
          onPageChange={onPageChange}
          onPageSizeChange={onPageSizeChange}
        />
      </WorkSurface>
    </section>
  );
}

function ProjectsV3Table({
  onSelectProject,
  projects,
  selectedProjectId,
}: {
  onSelectProject: (projectId: string) => void;
  projects: Project[];
  selectedProjectId?: string;
}) {
  return (
    <V3Table>
      <thead>
        <tr>
          <V3Th className="min-w-[260px]" role="columnheader">
            项目
          </V3Th>
          <V3Th className="min-w-24" role="columnheader">
            状态
          </V3Th>
          <V3Th className="min-w-36" role="columnheader">
            协调线程
          </V3Th>
          <V3Th className="min-w-36" role="columnheader">
            负责人
          </V3Th>
          <V3Th className="min-w-[260px]" role="columnheader">
            目标
          </V3Th>
          <V3Th className="min-w-32 text-right" role="columnheader">
            操作
          </V3Th>
        </tr>
      </thead>
      <tbody>
        {projects.map((project) => (
          <ProjectsV3TableRow
            key={project.id}
            onSelectProject={onSelectProject}
            project={project}
            selected={project.id === selectedProjectId}
          />
        ))}
        {projects.length === 0 ? (
          <tr>
            <V3Td colSpan={6}>
              <V3EmptyState
                icon={<FolderKanban />}
                title="没有符合筛选条件的项目"
                description="调整搜索关键词或项目状态筛选后重试。"
              />
            </V3Td>
          </tr>
        ) : null}
      </tbody>
    </V3Table>
  );
}

function ProjectsV3TableRow({
  onSelectProject,
  project,
  selected,
}: {
  onSelectProject: (projectId: string) => void;
  project: Project;
  selected: boolean;
}) {
  const rowTone =
    project.status === "acceptance" || project.status === "paused" ? "warn" : undefined;

  return (
    <V3Tr
      className={cn(selected && "[&>td]:bg-v3-brand-soft/60")}
      data-state={selected ? "selected" : undefined}
      tone={rowTone}
    >
      <V3Td className="whitespace-normal">
        <button
          aria-current={selected ? "true" : undefined}
          aria-label={`选择项目 ${project.name}`}
          className="flex min-w-0 items-start gap-3 text-left"
          type="button"
          onClick={() => onSelectProject(project.id)}
        >
          <IconTile tone={projectStatusTone(project.status)} size="sm">
            {project.status === "archived" ? <Archive /> : <CircleDot />}
          </IconTile>
          <span className="min-w-0">
            <span className="block truncate font-bold text-v3-ink">{project.name}</span>
            <span className="mt-0.5 block font-mono text-[12px] text-v3-ink-3">
              {project.id}
            </span>
          </span>
        </button>
      </V3Td>
      <V3Td>
        <StatusPill tone={projectStatusTone(project.status)}>
          {projectStatusLabel(project.status)}
        </StatusPill>
      </V3Td>
      <V3Td>
        <span className="font-mono text-[12px] text-v3-ink-2">
          {project.coordination_status || "registered"}
        </span>
      </V3Td>
      <V3Td>
        <span className="font-mono text-[12px] text-v3-ink-2">
          {project.human_owner_user_id || "未设置"}
        </span>
      </V3Td>
      <V3Td className="whitespace-normal">
        <span className="line-clamp-2 text-[13px] leading-5 text-v3-ink-2">
          {project.goal || "暂无目标说明"}
        </span>
      </V3Td>
      <V3Td>
        <div className="flex justify-end gap-2">
          <V3Button
            aria-label="选择项目"
            onClick={() => onSelectProject(project.id)}
            size="sm"
            type="button"
            variant={selected ? "primary" : "outline"}
          >
            选择
          </V3Button>
          <V3Button asChild size="sm" variant="ghost">
            <Link params={{ projectId: project.id }} to="/projects/$projectId">
              详情
            </Link>
          </V3Button>
        </div>
      </V3Td>
    </V3Tr>
  );
}

function buildProjectStats(projects: Project[]): Array<{
  icon: LucideIcon;
  label: string;
  loud?: boolean;
  tone: V3Tone;
  value: number;
}> {
  const running = projects.filter((project) => project.status === "running").length;
  const acceptance = projects.filter((project) => project.status === "acceptance").length;
  const archived = projects.filter((project) => project.status === "archived").length;
  return [
    {
      icon: ClipboardList,
      label: "项目总数",
      tone: "brand",
      value: projects.length,
    },
    {
      icon: CircleDot,
      label: "运行中",
      tone: "ok",
      value: running,
    },
    {
      icon: ListChecks,
      label: "验收中",
      loud: acceptance > 0,
      tone: "warn",
      value: acceptance,
    },
    {
      icon: Archive,
      label: "已归档",
      tone: "mute",
      value: archived,
    },
  ];
}

function projectStatusLabel(status: ProjectStatus | string) {
  const labels: Record<string, string> = {
    acceptance: "验收中",
    archived: "已归档",
    configuring: "配置中",
    draft: "草稿",
    paused: "已暂停",
    running: "运行中",
  };
  return labels[status] ?? status;
}

function projectStatusTone(status: ProjectStatus | string): V3Tone {
  if (status === "running") return "ok";
  if (status === "archived") return "mute";
  if (status === "paused" || status === "acceptance") return "warn";
  if (status === "configuring" || status === "draft") return "info";
  return "mute";
}

function queryErrorMessage(error: unknown) {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return "执行证据链加载失败";
}

function selectDispatchGateTask({
  activeTasks,
  fallbackTasks,
  graphTasks,
  projectId,
}: {
  activeTasks: ProjectTask[];
  fallbackTasks: ProjectTask[];
  graphTasks: ProjectTask[];
  projectId?: string;
}) {
  if (!projectId) {
    return undefined;
  }
  const candidates = [activeTasks, fallbackTasks, graphTasks]
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
