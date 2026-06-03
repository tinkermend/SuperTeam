import { createFileRoute } from "@tanstack/react-router";
import { TeamDetailPage } from "@/features/teams";

export const Route = createFileRoute("/_authenticated/teams/$teamId")({
  component: TeamDetailRoute,
});

function TeamDetailRoute() {
  const { teamId } = Route.useParams();

  return <TeamDetailPage teamId={teamId} />;
}
