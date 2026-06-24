import { createFileRoute } from "@tanstack/react-router";
import { SkillDetailView } from "@/features/skills/detail";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export const Route = createFileRoute("/_authenticated/skills/$skillId")({
  component: SkillDetailRoute,
});

function SkillDetailRoute() {
  const { skillId } = Route.useParams();

  return <SkillDetailView apiBaseUrl={resolveControlPlaneUrl()} skillId={skillId} />;
}
