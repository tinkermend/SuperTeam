import { createFileRoute } from "@tanstack/react-router";
import { TaskLaunchPage } from "@/features/task-launches";

export const Route = createFileRoute("/_authenticated/task-launches/")({
  component: TaskLaunchPage,
});
