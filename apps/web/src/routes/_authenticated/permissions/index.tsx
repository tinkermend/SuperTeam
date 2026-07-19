import { createFileRoute } from "@tanstack/react-router";
import { PermissionsCenter } from "@/features/permissions";

type PermissionsSearch = {
  tab?: string;
};

function PermissionsRoute() {
  const { tab } = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <PermissionsCenter
      activeTab={tab ?? "overview"}
      onTabChange={(next) =>
        navigate({
          search: (prev) => ({ ...prev, tab: next === "overview" ? undefined : next }),
          replace: true,
        })
      }
    />
  );
}

export const Route = createFileRoute("/_authenticated/permissions/")({
  component: PermissionsRoute,
  validateSearch: (search: Record<string, unknown>): PermissionsSearch => {
    const result: PermissionsSearch = {};
    if (typeof search.tab === "string" && search.tab) {
      result.tab = search.tab;
    }
    return result;
  },
});
