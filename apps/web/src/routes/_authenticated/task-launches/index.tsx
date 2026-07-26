import { createFileRoute } from "@tanstack/react-router";
import { TaskLaunchPage } from "@/features/task-launches";

export type TaskLaunchSearch = {
  mode?: "plan" | "loop" | "chat";
  project?: string;
  view?: "instances";
  q?: string;
  scope?: "archived";
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
    if (search.view === "instances") {
      result.view = search.view;
    }
    if (typeof search.q === "string" && search.q) {
      result.q = search.q;
    }
    if (search.scope === "archived") {
      result.scope = search.scope;
    }
    return result;
  }
});
