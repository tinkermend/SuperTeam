import { createFileRoute } from "@tanstack/react-router";
import { TaskLaunchPage } from "@/features/task-launches";

/** 任务中枢搜索参数：face 选工作面（对话/任务），mode/project 供任务面，
 * q/scope 供在途实例；view=instances / mode=chat 为旧深链兼容。 */
export type TaskHubSearch = {
  face?: "chat" | "task";
  mode?: "plan" | "loop" | "chat";
  project?: string;
  view?: "instances";
  q?: string;
  scope?: "archived";
};

export const Route = createFileRoute("/_authenticated/")({
  component: () => <TaskLaunchPage title="任务中枢" />,
  validateSearch: (search: Record<string, unknown>): TaskHubSearch => {
    const result: TaskHubSearch = {};
    if (typeof search.project === "string" && search.project) {
      result.project = search.project;
    }
    if (search.face === "chat" || search.face === "task") {
      result.face = search.face;
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
