import { useMemo } from "react";
import { keepPreviousData, useQueries } from "@tanstack/react-query";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listProjectDecisionRequests,
  listProjectEvidence,
  listProjectMembers,
  listProjectTasks,
  type Project,
  type ProjectDecisionRequest,
  type ProjectEvidenceRef,
  type ProjectMember,
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
// from coordination_status), so they are not fetched here: 4 requests/project.
type ProjectRiskSignalPayload = {
  projectId: string;
  tasks: ProjectTask[];
  decisions: ProjectDecisionRequest[];
  evidence: ProjectEvidenceRef[];
  members: ProjectMember[];
};

type UseProjectRiskSignalsInput = {
  apiOptions: ApiClientOptions;
  enabled?: boolean;
  projects: Project[];
  /** principal_id → 展示名（如全局数字员工目录），成员快照缺失时的名称回退。 */
  principalNamesById?: ReadonlyMap<string, string>;
};

export type UseProjectRiskSignalsResult = {
  isFetching: boolean;
  summaries: ProjectRiskSummaryMap;
};

export function useProjectRiskSignals({
  apiOptions,
  enabled = true,
  projects,
  principalNamesById,
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
        const [tasks, decisions, evidence, members] = await Promise.all([
          // limit 取服务端上限 100：这些窗口原本是为「当前页 10 项目 × 4 连发」压出来的，
          // 现在只对选中的单个项目发，取小反而让 triage 的「可行动 · N」比队列行的
          // run-summary 真值少（实测 18 vs 21、证据 10 vs 12）——同屏两个数字对不上。
          listProjectTasks(apiOptions, project.id, { limit: 100 }),
          listProjectDecisionRequests(apiOptions, project.id, { limit: 100 }),
          listProjectEvidence(apiOptions, project.id, { limit: 100 }),
          listProjectMembers(apiOptions, project.id),
        ]);

        return {
          decisions: decisions.filter((decision) => decision.project_id === project.id),
          evidence: evidence.filter((item) => item.project_id === project.id),
          members: members.filter((member) => member.project_id === project.id),
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
          members: payload.members,
          principalNamesById,
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
  }, [principalNamesById, projects, queries]);
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
