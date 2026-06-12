import { createFileRoute } from "@tanstack/react-router";
import { TaskLaunchDetailPage } from "@/features/task-launches";

export const Route = createFileRoute("/_authenticated/task-launches/$demandId")({
  component: TaskLaunchDetailRoute,
});

function TaskLaunchDetailRoute() {
  const { demandId } = Route.useParams();
  return <TaskLaunchDetailPage demandId={demandId} />;
}
