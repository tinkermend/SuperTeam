import { createFileRoute } from "@tanstack/react-router";
import { TeamConfigPage } from "@/features/teams/components/team-config-page";

export const Route = createFileRoute("/_authenticated/teams/$teamId/config")({
  component: TeamConfigRoute
});

function TeamConfigRoute() {
  const { teamId } = Route.useParams();

  return <TeamConfigPage teamId={teamId} />;
}
