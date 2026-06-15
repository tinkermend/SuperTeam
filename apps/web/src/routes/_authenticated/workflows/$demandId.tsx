import { createFileRoute } from "@tanstack/react-router";
import { WorkflowPage } from "@/features/workflows";

export const Route = createFileRoute("/_authenticated/workflows/$demandId")({
  component: WorkflowDemandRoute,
});

function WorkflowDemandRoute() {
  const { demandId } = Route.useParams();

  return <WorkflowPage demandId={demandId} />;
}
