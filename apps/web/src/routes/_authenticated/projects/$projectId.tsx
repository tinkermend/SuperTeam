import { createFileRoute } from "@tanstack/react-router";
import { ProjectDetailPage } from "@/features/projects";

export const Route = createFileRoute("/_authenticated/projects/$projectId")({
  component: ProjectDetailRoute,
});

function ProjectDetailRoute() {
  const { projectId } = Route.useParams();

  return <ProjectDetailPage projectId={projectId} />;
}
