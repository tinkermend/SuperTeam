import { createFileRoute } from "@tanstack/react-router";
import { ProjectConfigPage } from "@/features/projects";

export type ProjectConfigSearch = {
  tab?: "overview" | "members" | "casting" | "coordination";
};

export const Route = createFileRoute("/_authenticated/projects/$projectId/config")({
  component: ProjectConfigRoute,
  validateSearch: (search: Record<string, unknown>): ProjectConfigSearch => {
    const tab = search.tab;
    if (
      tab === "overview" ||
      tab === "members" ||
      tab === "casting" ||
      tab === "coordination"
    ) {
      return { tab };
    }
    return {};
  },
});

function ProjectConfigRoute() {
  const { projectId } = Route.useParams();
  const { tab } = Route.useSearch();
  return <ProjectConfigPage initialTab={tab} projectId={projectId} />;
}
