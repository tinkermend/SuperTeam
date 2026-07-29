/**
 * 任务状态判据的统一出口。
 *
 * 背景：`project_tasks.requires_human_approval` 是**粘性**标志——任务进入终态后
 * 它仍为 true。控制平面侧早已按此口径设了护栏（见
 * `apps/control-plane/internal/storage/queries/project.sql` 中 F5 的
 * `status NOT IN ('completed','done','success','cancelled','failed')`），
 * 前端各处若直接读该列，就会让已完成的任务永久显示"需要人工审批 / 等待人工决策"。
 *
 * 终态集与上述 SQL 判据保持同口径；新增终态时只改这里。
 */
const TERMINAL_TASK_STATUSES = new Set([
  "cancelled",
  "completed",
  "done",
  "failed",
  "success",
]);

/** 任务是否已到终态。 */
export function isTerminalTaskStatus(status: string | undefined | null): boolean {
  return TERMINAL_TASK_STATUSES.has((status ?? "").trim().toLowerCase());
}

/**
 * 任务是否**仍在**等待人工审批：粘性标志必须与非终态同时成立。
 * 所有渲染"需人工审批"类提示的调用点都应走这里，不要直接读 requires_human_approval。
 */
export function isAwaitingHumanApproval(task: {
  requires_human_approval?: boolean;
  status?: string;
}): boolean {
  return Boolean(task.requires_human_approval) && !isTerminalTaskStatus(task.status);
}
