import { createFileRoute } from "@tanstack/react-router";
import { InboxPage } from "@/features/inbox";

type InboxSearch = {
  /** §4.4.2：只进 URL，不持久化。非法值忽略回落 risk。 */
  sort?: "risk" | "oldest";
  /** 项目内下钻次级出口：按项目过滤。 */
  project?: string;
  /** 按上游 source_id（决策/审批请求 id）定位条目。 */
  source?: string;
};

export const Route = createFileRoute("/_authenticated/inbox/")({
  validateSearch: (search: Record<string, unknown>): InboxSearch => {
    const out: InboxSearch = {};
    const raw = typeof search.sort === "string" ? search.sort : undefined;
    if (raw === "oldest") out.sort = "oldest";
    else if (raw === "risk") out.sort = "risk";
    if (typeof search.project === "string" && search.project.trim()) {
      out.project = search.project.trim();
    }
    if (typeof search.source === "string" && search.source.trim()) {
      out.source = search.source.trim();
    }
    return out;
  },
  component: InboxRoute,
});

function InboxRoute() {
  const { sort, project, source } = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <InboxPage
      initialProjectId={project}
      initialSort={sort}
      initialSourceId={source}
      onSortChange={(next) => {
        void navigate({
          search: (prev) => ({
            ...prev,
            sort: next === "oldest" ? "oldest" : undefined,
          }),
          replace: true,
        });
      }}
    />
  );
}