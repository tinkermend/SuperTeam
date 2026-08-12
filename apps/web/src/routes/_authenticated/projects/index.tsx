import { createFileRoute } from "@tanstack/react-router";
import { ProjectsPage } from "@/features/projects";
import type { ProjectPortfolioSort } from "@/lib/api/projects";
import type { ProjectStatus } from "@/lib/api/projects";
import type { ProjectRiskFilter } from "@/features/projects/project-risk";
import type { ProjectTaskPortfolioBucketKey } from "@/lib/status-labels";

/** 列表态筛选 URL 事实源（§5.4.1 mine_only 等，刷新不丢）。 */
export type ProjectsListSearch = {
  q?: string;
  status?: ProjectStatus;
  risk?: ProjectRiskFilter;
  task_state?: ProjectTaskPortfolioBucketKey;
  sort?: ProjectPortfolioSort;
  mine_only?: boolean;
  page?: number;
  page_size?: number;
};

const PROJECT_STATUSES = new Set<string>([
  "draft",
  "configuring",
  "running",
  "paused",
  "acceptance",
  "archived",
]);

const RISK_FILTERS = new Set<string>([
  "all",
  "blocked",
  "human_decision",
  "execution_failed",
  "waiting_human",
  "evidence_required",
  "runtime_or_coordination",
  "sla_waiting",
]);

const TASK_STATES = new Set<string>([
  "pending",
  "queued",
  "running",
  "waiting_human",
  "blocked",
  "failed",
  "completed",
  "cancelled",
  "other",
]);

const SORTS = new Set<string>(["attention", "recent", "created"]);

function parsePositiveInt(raw: unknown, fallback: number, max: number): number {
  if (typeof raw === "number" && Number.isFinite(raw)) {
    const n = Math.floor(raw);
    if (n >= 1 && n <= max) return n;
  }
  if (typeof raw === "string" && raw.trim()) {
    const n = Number.parseInt(raw, 10);
    if (Number.isFinite(n) && n >= 1 && n <= max) return n;
  }
  return fallback;
}

export const Route = createFileRoute("/_authenticated/projects/")({
  validateSearch: (search: Record<string, unknown>): ProjectsListSearch => {
    const out: ProjectsListSearch = {};
    if (typeof search.q === "string" && search.q.trim()) {
      out.q = search.q;
    }
    if (typeof search.status === "string" && PROJECT_STATUSES.has(search.status)) {
      out.status = search.status as ProjectStatus;
    }
    if (typeof search.risk === "string" && RISK_FILTERS.has(search.risk) && search.risk !== "all") {
      out.risk = search.risk as ProjectRiskFilter;
    }
    if (typeof search.task_state === "string" && TASK_STATES.has(search.task_state)) {
      out.task_state = search.task_state as ProjectTaskPortfolioBucketKey;
    }
    if (typeof search.sort === "string" && SORTS.has(search.sort) && search.sort !== "attention") {
      out.sort = search.sort as ProjectPortfolioSort;
    }
    if (search.mine_only === true || search.mine_only === "true" || search.mine_only === "1") {
      out.mine_only = true;
    }
    const page = parsePositiveInt(search.page, 1, 10_000);
    if (page !== 1) out.page = page;
    // 默认 9（3×3 约 1.5 屏）；合法取值含 9/12/20/50。
    const pageSize = parsePositiveInt(search.page_size, 9, 50);
    if (pageSize !== 9) out.page_size = pageSize;
    return out;
  },
  component: ProjectsRoute,
});

function ProjectsRoute() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <ProjectsPage
      listSearch={search}
      onListSearchChange={(next) => {
        void navigate({
          search: next,
          replace: true,
        });
      }}
    />
  );
}
