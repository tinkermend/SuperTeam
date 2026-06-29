import { useMemo } from "react";
import { keepPreviousData, useQueries } from "@tanstack/react-query";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listProjectDecisionRequests,
  listProjectEvidence,
  listProjectTasks,
  type Project,
  type ProjectDecisionRequest,
  type ProjectEvidenceRef,
  type ProjectTask,
} from "@/lib/api/projects";
import {
  deriveProjectRiskSummary,
  emptyProjectRiskSummary,
  type ProjectRiskSummaryMap,
} from "../project-risk";

const fetcherIds = new WeakMap<typeof fetch, number>();
let nextFetcherId = 1;

// R3/R4: events no longer contribute to risk (runtime/coordination risk comes
// from coordination_status), so they are not fetched here: 3 requests/project.
type ProjectRiskSignalPayload = {
  projectId: string;
  tasks: ProjectTask[];
  decisions: ProjectDecisionRequest[];
  evidence: ProjectEvidenceRef[];
};

type UseProjectRiskSignalsInput = {
  apiOptions: ApiClientOptions;
  enabled?: boolean;
  projects: Project[];
};

export type UseProjectRiskSignalsResult = {
  isFetching: boolean;
  summaries: ProjectRiskSummaryMap;
};

export function useProjectRiskSignals({
  apiOptions,
  enabled = true,
  projects,
}: UseProjectRiskSignalsInput): UseProjectRiskSignalsResult {
  const apiBaseIdentity = apiOptions.baseUrl.replace(/\/+$/, "");
  const fetcherIdentity = apiOptions.fetcher
    ? getFetcherIdentity(apiOptions.fetcher)
    : "global";
  const queries = useQueries({
    queries: projects.map((project) => ({
      enabled,
      placeholderData: keepPreviousData,
      queryFn: async (): Promise<ProjectRiskSignalPayload> => {
        const [tasks, decisions, evidence] = await Promise.all([
          listProjectTasks(apiOptions, project.id, { limit: 20 }),
          listProjectDecisionRequests(apiOptions, project.id, { limit: 20 }),
          listProjectEvidence(apiOptions, project.id, { limit: 10 }),
        ]);

        return {
          decisions: decisions.filter((decision) => decision.project_id === project.id),
          evidence: evidence.filter((item) => item.project_id === project.id),
          projectId: project.id,
          tasks: tasks.filter((task) => task.project_id === project.id),
        };
      },
      queryKey: [
        "project-risk-signals",
        apiBaseIdentity,
        fetcherIdentity,
        project.id,
      ],
      retry: false,
      staleTime: 15_000,
    })),
  });

  return useMemo(() => {
    const summaries: ProjectRiskSummaryMap = {};

    projects.forEach((project, index) => {
      const query = queries[index];
      const payload = query?.data;

      if (payload?.projectId === project.id) {
        summaries[project.id] = deriveProjectRiskSummary({
          decisions: payload.decisions,
          evidence: payload.evidence,
          events: [],
          project,
          tasks: payload.tasks,
        });
        return;
      }

      summaries[project.id] = emptyProjectRiskSummary(project, {
        state: query?.isError ? "error" : "pending",
      });
    });

    return {
      isFetching: queries.some((query) => query.isFetching),
      summaries,
    };
  }, [projects, queries]);
}

function getFetcherIdentity(fetcher: typeof fetch): string {
  const existing = fetcherIds.get(fetcher);
  if (existing) {
    return `fetcher:${existing}`;
  }

  const id = nextFetcherId;
  nextFetcherId += 1;
  fetcherIds.set(fetcher, id);
  return `fetcher:${id}`;
}
