import { useEffect, useMemo, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  archiveProject,
  createProject,
  getProject,
  getProjectOverview,
  listProjectCoordinationJobs,
  listProjectDecisionRequests,
  listProjectDemands,
  listProjectEvents,
  listProjectExecutionSummaries,
  listProjectRouteDecisions,
  listProjects,
  listProjectTasks,
  listProjectTransferRequests,
  resolveProjectDecision,
  submitProjectDemand,
  type CreateProjectInput,
  type ListProjectsFilters,
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

  const selectedProject = selectedProjectFromList ?? selectedProjectQuery.data;
  const effectiveProjectId = selectedProject?.id ?? selectedProjectId;

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
    onSuccess: async () => {
      setDemandOpen(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-demands", effectiveProjectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-events", effectiveProjectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", effectiveProjectId] }),
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

  const isInitialLoading = projectsQuery.isLoading && !projectsQuery.data;
  const displayedProject = overviewQuery.data?.project ?? selectedProject;
  const isArchived = displayedProject?.status === "archived";

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
            coordinationJobs={coordinationJobsQuery.data ?? []}
            decisionRequests={decisionRequestsQuery.data ?? []}
            demands={demandsQuery.data ?? []}
            events={eventsQuery.data ?? []}
            executionSummaries={executionSummariesQuery.data ?? []}
            isArchived={isArchived}
            onArchiveProject={() => {
              if (effectiveProjectId) {
                archiveMutation.mutate(effectiveProjectId);
              }
            }}
            onResolveDecision={(decisionId, decision) => {
              if (effectiveProjectId) {
                resolveDecisionMutation.mutate({ decisionId, decision });
              }
            }}
            onSubmitDemand={() => setDemandOpen(true)}
            overview={overviewQuery.data}
            project={displayedProject}
            routeDecisions={routeDecisionsQuery.data ?? []}
            tasks={tasksQuery.data ?? []}
            transferRequests={transferRequestsQuery.data ?? []}
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
