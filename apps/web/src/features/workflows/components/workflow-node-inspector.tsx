import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { ExternalLink } from "lucide-react";
import { StatusPill, V3Button, type V3Tone } from "@/components/superteam";
import type { ProjectTaskGraph, ProjectTaskGraphNode } from "@/lib/api/projects";

type WorkflowNodeInspectorProps = {
  graph: ProjectTaskGraph;
  selectedTask: ProjectTaskGraphNode | undefined;
  variant?: "card" | "dialog";
};

export function WorkflowNodeInspector({
  graph,
  selectedTask,
}: WorkflowNodeInspectorProps) {
  if (!selectedTask) {
    return null;
  }

  const run = graph.runs.find((item) => item.project_task_id === selectedTask.id);
  const result = graph.execution_summaries.find(
    (item) => item.project_task_id === selectedTask.id,
  );
  const decisions = graph.decision_requests.filter(
    (item) => item.project_task_id === selectedTask.id,
  );
  const ownerName = employeeNameForTask(graph, selectedTask);

  return (
    <>
      <div className="flex items-start justify-between gap-3">
        <h3 className="line-clamp-3 text-base font-bold leading-6 tracking-normal text-v3-ink">
          {selectedTask.title}
        </h3>
        <StatusPill className="shrink-0" tone={taskStatusTone(selectedTask.status)}>
          {selectedTask.status}
        </StatusPill>
      </div>

      <div className="mt-2 divide-y divide-v3-line text-sm">
        <InspectorRow label="负责人" value={ownerName} />
        <InspectorRow label="阻塞" value={formatBlocker(selectedTask)} />
        <InspectorRow label="输入" value={formatValue(selectedTask.input_requirements)} />
        <InspectorRow label="输出" value={formatValue(selectedTask.expected_outputs)} />
        <InspectorRow
          label="交接契约"
          value={formatValue(selectedTask.handoff_contract)}
        />
        <InspectorRow
          action={
            run?.runtime_task_id ? (
              <V3Button asChild size="sm" variant="outline">
                <Link aria-label={`查看${selectedTask.title} Runtime`} to="/runtime">
                  <ExternalLink className="size-3.5" />
                  Runtime
                </Link>
              </V3Button>
            ) : null
          }
          label="Run"
          value={
            run
              ? [run.status, run.provider_type, run.runtime_node_summary]
                  .filter(Boolean)
                  .join(" · ")
              : "暂无运行记录"
          }
        />
        <InspectorRow
          label="结果"
          value={result ? result.conclusion : "暂无执行结果"}
        />
        <InspectorRow
          action={
            decisions.length > 0 ? (
              <V3Button asChild size="sm" variant="outline">
                <Link aria-label={`查看${selectedTask.title}审批`} to="/approvals">
                  <ExternalLink className="size-3.5" />
                  审批
                </Link>
              </V3Button>
            ) : null
          }
          label="人工决策"
          value={
            decisions.length > 0
              ? decisions.map((decision) => decision.status_snapshot).join(" · ")
              : "暂无人工决策"
          }
        />
      </div>
    </>
  );
}

export function employeeNameForTask(
  graph: ProjectTaskGraph,
  task: ProjectTaskGraphNode,
): string {
  if (!task.assigned_digital_employee_id) return "未分配";

  return (
    graph.employees.find(
      (employee) => employee.digital_employee_id === task.assigned_digital_employee_id,
    )?.display_name ?? "未分配"
  );
}

export function taskStatusTone(status: string): V3Tone {
  const normalized = status.toLowerCase();

  if (["completed", "accepted", "approved", "done", "success"].includes(normalized)) {
    return "ok";
  }
  if (["failed", "rejected", "cancelled", "blocked"].includes(normalized)) {
    return "danger";
  }
  if (
    ["pending", "waiting", "planning", "planning_pending", "planned", "waiting_human"].includes(
      normalized,
    )
  ) {
    return "warn";
  }
  if (["assigned", "dispatchable", "running", "in_progress"].includes(normalized)) {
    return "info";
  }

  return "mute";
}

function formatBlocker(task: ProjectTaskGraphNode): string {
  if (!task.current_blocker) return "暂无阻塞";

  return [
    task.current_blocker.title,
    task.current_blocker.type,
    task.current_blocker.resource_id,
  ]
    .filter(Boolean)
    .join(" · ");
}

function InspectorRow({
  action,
  label,
  value,
}: {
  action?: ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="grid gap-2 py-3">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs font-bold text-v3-ink-3">{label}</span>
        {action}
      </div>
      <p className="min-w-0 whitespace-pre-wrap break-words text-sm leading-6 text-v3-ink">
        {value}
      </p>
    </div>
  );
}

function formatValue(value: unknown): string {
  if (Array.isArray(value)) {
    if (value.length === 0) return "暂无";
    return value.map((item) => formatLeaf(item)).join(" · ");
  }

  if (value && typeof value === "object") {
    const entries = Object.entries(value);
    if (entries.length === 0) return "暂无";

    return entries
      .slice(0, 4)
      .map(([key, item]) => `${key}: ${formatLeaf(item)}`)
      .join(" · ");
  }

  return formatLeaf(value);
}

function formatLeaf(value: unknown): string {
  if (value === undefined || value === null || value === "") return "暂无";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);

  try {
    return JSON.stringify(value);
  } catch {
    return "无法显示";
  }
}
