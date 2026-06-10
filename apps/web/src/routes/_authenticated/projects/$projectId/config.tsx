import { createFileRoute } from "@tanstack/react-router";
import { ProjectConfigPage } from "@/features/projects";

export const Route = createFileRoute("/_authenticated/projects/$projectId/config")({
  component: ProjectConfigRoute,
});

function ProjectConfigRoute() {
  const { projectId } = Route.useParams();

  return <ProjectConfigPage projectId={projectId} />;
}
