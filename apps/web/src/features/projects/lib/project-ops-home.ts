import type {
  ProjectCoordinationMode,
  ProjectDemand,
  ProjectEvent,
  ProjectMember,
  ProjectTask,
  ProjectTaskGraphNode,
} from "@/lib/api/projects";

/** 脉搏/展示用模式：含任务中枢对话（不进 demand coordination_mode 时仍可标）。 */
export type OpsLaunchMode = ProjectCoordinationMode | "chat";

export type OpsPulseChip = {
  dayKey: string;
  taskId: string;
  title: string;
  status: string;
  statusTone: "ok" | "warn" | "info" | "danger" | "mute";
  timeLabel: string;
  at: string;
  mode: OpsLaunchMode;
};

export type OpsPulseDay = {
  dayKey: string;
  weekdayLabel: string;
  dayOfMonth: number;
  isToday: boolean;
  chips: OpsPulseChip[];
};

const ACTIVE_OR_BLOCKED = new Set([
  "planned",
  "queued",
  "running",
  "waiting_human",
  "failed",
]);

const OPS_EVENT_TYPES = new Set([
  "project_task.created",
  "project_task.dispatched",
  "project_task.completed",
  "project_task.failed",
  "project_task.dispatch_blocked",
  "project_task.dispatch_gate.blocked",
  "project_task.dispatch_gate.checked",
  "project_task.dispatch_gate.replan_required",
  "project_task.dispatch_gate.retry_later",
  "project_task.dispatch_gate.waiting_human",
  "decision.requested",
  "decision.submitted",
  "transfer.requested",
  "project.acceptance.submitted",
]);

const WEEKDAY_LABELS = ["日", "一", "二", "三", "四", "五", "六"];

export function coordinationModeLabel(mode: OpsLaunchMode): string {
  switch (mode) {
    case "loop":
      return "Loop";
    case "chat":
      return "对话";
    default:
      return "Plan";
  }
}

export function resolveTaskMode(
  task: ProjectTask,
  demandsById: ReadonlyMap<string, ProjectDemand>,
): OpsLaunchMode {
  if (!task.demand_id) return "plan";
  const demand = demandsById.get(task.demand_id);
  const mode = demand?.coordination_mode;
  if (mode === "loop" || mode === "plan") return mode;
  return "plan";
}

export function taskActivityAt(
  task: ProjectTask,
  graphNodesById?: ReadonlyMap<string, ProjectTaskGraphNode>,
): string | undefined {
  const node = graphNodesById?.get(task.id);
  return node?.started_at || node?.finished_at || task.updated_at || task.created_at;
}

export function taskStatusTone(
  status: string,
): OpsPulseChip["statusTone"] {
  switch (status) {
    case "completed":
      return "ok";
    case "running":
    case "waiting_human":
      return "warn";
    case "planned":
    case "queued":
      return "info";
    case "failed":
      return "danger";
    default:
      return "mute";
  }
}

function startOfLocalDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

function dayKeyFromDate(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

function formatClock(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

/** 以「今日」为锚的本周（周一到周日）脉搏。 */
export function buildWeekPulse(input: {
  tasks: ProjectTask[];
  demands: ProjectDemand[];
  graphNodes?: ProjectTaskGraphNode[];
  now?: Date;
}): OpsPulseDay[] {
  const now = input.now ?? new Date();
  const today = startOfLocalDay(now);
  const weekday = today.getDay(); // 0=Sun
  const mondayOffset = weekday === 0 ? -6 : 1 - weekday;
  const monday = new Date(today);
  monday.setDate(today.getDate() + mondayOffset);

  const demandsById = new Map(input.demands.map((d) => [d.id, d]));
  const graphNodesById = new Map(
    (input.graphNodes ?? []).map((node) => [node.id, node]),
  );

  const chipsByDay = new Map<string, OpsPulseChip[]>();
  for (const task of input.tasks) {
    if (task.dismissed_at) continue;
    const at = taskActivityAt(task, graphNodesById);
    if (!at) continue;
    const atDate = new Date(at);
    if (Number.isNaN(atDate.getTime())) continue;
    const key = dayKeyFromDate(atDate);
    const weekEnd = new Date(monday);
    weekEnd.setDate(monday.getDate() + 7);
    if (atDate < monday || atDate >= weekEnd) continue;

    const list = chipsByDay.get(key) ?? [];
    list.push({
      at,
      dayKey: key,
      mode: resolveTaskMode(task, demandsById),
      status: task.status,
      statusTone: taskStatusTone(task.status),
      taskId: task.id,
      timeLabel: formatClock(at),
      title: task.title,
    });
    chipsByDay.set(key, list);
  }

  for (const [, chips] of chipsByDay) {
    chips.sort((a, b) => a.at.localeCompare(b.at));
  }

  const days: OpsPulseDay[] = [];
  for (let i = 0; i < 7; i += 1) {
    const date = new Date(monday);
    date.setDate(monday.getDate() + i);
    const key = dayKeyFromDate(date);
    days.push({
      chips: (chipsByDay.get(key) ?? []).slice(0, 4),
      dayKey: key,
      dayOfMonth: date.getDate(),
      isToday: key === dayKeyFromDate(today),
      weekdayLabel: WEEKDAY_LABELS[date.getDay()] ?? "",
    });
  }
  return days;
}

export function selectActiveOrBlockedTasks(
  tasks: ProjectTask[],
  limit = 3,
): ProjectTask[] {
  return tasks
    .filter((task) => !task.dismissed_at && ACTIVE_OR_BLOCKED.has(task.status))
    .sort((a, b) => {
      const aAt = a.updated_at || a.created_at || "";
      const bAt = b.updated_at || b.created_at || "";
      return bAt.localeCompare(aAt);
    })
    .slice(0, limit);
}

export function filterOpsEvents(events: ProjectEvent[], limit = 6): ProjectEvent[] {
  return events
    .filter((event) => OPS_EVENT_TYPES.has(event.event_type))
    .slice(0, limit);
}

export function employeeCurrentTaskTitle(
  member: ProjectMember,
  tasks: ProjectTask[],
): string | undefined {
  if (member.principal_type !== "digital_employee") return undefined;
  const active = tasks.find(
    (task) =>
      !task.dismissed_at &&
      task.assigned_digital_employee_id === member.principal_id &&
      ACTIVE_OR_BLOCKED.has(task.status) &&
      task.status !== "failed",
  );
  if (active) return active.title;
  const failed = tasks.find(
    (task) =>
      !task.dismissed_at &&
      task.assigned_digital_employee_id === member.principal_id &&
      task.status === "failed",
  );
  return failed ? failed.title : undefined;
}

export function employeeBusyLabel(
  member: ProjectMember,
  tasks: ProjectTask[],
): { label: string; tone: "ok" | "warn" | "danger" | "mute" } {
  if (member.principal_type !== "digital_employee") {
    return { label: "—", tone: "mute" };
  }
  const assigned = tasks.filter(
    (task) =>
      !task.dismissed_at &&
      task.assigned_digital_employee_id === member.principal_id &&
      ACTIVE_OR_BLOCKED.has(task.status),
  );
  const failed = assigned.find((task) => task.status === "failed");
  if (failed) return { label: "失败待恢复", tone: "danger" };
  const running = assigned.find(
    (task) => task.status === "running" || task.status === "waiting_human",
  );
  if (running) return { label: running.title, tone: "warn" };
  const queued = assigned.find(
    (task) => task.status === "queued" || task.status === "planned",
  );
  if (queued) return { label: `${queued.title} · 待启`, tone: "mute" };
  return { label: "空闲", tone: "mute" };
}

export function countWeekPulseActivity(days: OpsPulseDay[]): number {
  return days.reduce((sum, day) => sum + day.chips.length, 0);
}
