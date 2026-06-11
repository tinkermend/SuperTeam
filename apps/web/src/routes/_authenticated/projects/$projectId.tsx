import { Outlet, createFileRoute, useRouterState } from "@tanstack/react-router";
import { ProjectDetailPage } from "@/features/projects";

export const Route = createFileRoute("/_authenticated/projects/$projectId")({
  component: ProjectDetailRoute,
});

function ProjectDetailRoute() {
  const { projectId } = Route.useParams();

  return <ProjectRouteContent projectId={projectId} />;
}

export function ProjectRouteContent({ projectId }: { projectId: string }) {
  const pathname = useRouterState({ select: (state) => state.location.pathname });

  if (pathname.endsWith("/config")) {
    return <Outlet />;
  }

  return <ProjectDetailPage projectId={projectId} />;
}
