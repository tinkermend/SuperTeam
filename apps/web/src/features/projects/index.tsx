import { useEffect, useMemo, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
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
  getProjectOverview,
  listProjectArchiveSnapshots,
  listProjectArtifacts,
  listProjectBudgetLedger,
  listProjectCoordinationJobs,
  listProjectDecisionRequests,
  listProjectDemands,
  listProjectEvidence,
  listProjectEvents,
  listProjectExecutionSummaries,
  listProjectReports,
  listProjectRouteDecisions,
  listProjects,
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
  type ProjectEvidenceVerificationStatus,
  type ProjectStatus,
  type SubmitProjectDemandInput,
} from "@/lib/api/projects";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Button } from "@/components/ui/button";
import { ProjectManagementShell } from "./components/project-management-shell";
import {
  ProjectErrorState,
  ProjectLoadingState,
} from "./components/project-empty-states";
import {
  ProjectSwitcherPane,
  type ProjectListFilters as UiProjectListFilters,
} from "./components/project-switcher-pane";
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
  const projectCoordinationJobs = (coordinationJobsQuery.data ?? []).filter(
    (job) => job.project_id === effectiveProjectId,
  );
  const projectDecisionRequests = (decisionRequestsQuery.data ?? []).filter(
    (decision) => decision.project_id === effectiveProjectId,
  );
  const projectExecutionSummaries = (executionSummariesQuery.data ?? []).filter(
    (summary) => summary.project_id === effectiveProjectId,
  );
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

  return (
    <ProjectManagementShell
      title="项目管理"
      description="项目事实、成员池、任务、事件和需求记录"
      actions={
        <Button type="button" onClick={() => setCreateOpen(true)}>
          新建项目
        </Button>
      }
    >
      {isInitialLoading ? <ProjectLoadingState /> : null}
      {projectsQuery.isError ? (
        <ProjectErrorState onRetry={() => void projectsQuery.refetch()} />
      ) : null}
      {!isInitialLoading && !projectsQuery.isError ? (
        <div className="grid items-start gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
          <ProjectSwitcherPane
            filters={filters}
            isFetching={projectsQuery.isFetching}
            onCreateProject={() => setCreateOpen(true)}
            onFiltersChange={setFilters}
            onSelectProject={setSelectedProjectId}
            projects={projects}
            selectedProjectId={effectiveProjectId}
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
            evidence={projectEvidence}
            events={projectEvents}
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
            onResolveDecision={(decisionId, decision) => {
              if (effectiveProjectId) {
                resolveDecisionMutation.mutate({ decisionId, decision });
              }
            }}
            onSubmitDemand={() => setDemandOpen(true)}
            overview={overview}
            project={displayedProject}
            reports={projectReports}
            routeDecisions={projectRouteDecisions}
            tasks={projectTasks}
            transferRequests={projectTransferRequests}
          />
        </div>
      ) : null}
      <CreateProjectDrawer
        isSubmitting={createMutation.isPending}
        open={createOpen}
        submitError={createMutation.error?.message}
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
    </ProjectManagementShell>
  );
}
