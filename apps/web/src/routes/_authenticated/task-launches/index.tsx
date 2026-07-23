import { createFileRoute } from "@tanstack/react-router";
import { TaskLaunchPage } from "@/features/task-launches";

export type TaskLaunchSearch = {
  mode?: "plan" | "loop" | "chat";
  project?: string;
};

export const Route = createFileRoute("/_authenticated/task-launches/")({
  component: TaskLaunchPage,
  validateSearch: (search: Record<string, unknown>): TaskLaunchSearch => {
    const result: TaskLaunchSearch = {};
    if (typeof search.project === "string" && search.project) {
      result.project = search.project;
    }
    if (
      search.mode === "plan" ||
      search.mode === "loop" ||
      search.mode === "chat"
    ) {
      result.mode = search.mode;
    }
    return result;
  },
});
