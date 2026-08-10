import { createFileRoute } from "@tanstack/react-router";
import { InboxPage } from "@/features/inbox";

type InboxSearch = {
  /** §4.4.2：只进 URL，不持久化。非法值忽略回落 risk。 */
  sort?: "risk" | "oldest";
};

export const Route = createFileRoute("/_authenticated/inbox/")({
  validateSearch: (search: Record<string, unknown>): InboxSearch => {
    const raw = typeof search.sort === "string" ? search.sort : undefined;
    if (raw === "oldest") return { sort: "oldest" };
    if (raw === "risk") return { sort: "risk" };
    return {};
  },
  component: InboxRoute,
});

function InboxRoute() {
  const { sort } = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <InboxPage
      initialSort={sort}
      onSortChange={(next) => {
        void navigate({
          search: next === "oldest" ? { sort: "oldest" } : {},
          replace: true,
        });
      }}
    />
  );
}