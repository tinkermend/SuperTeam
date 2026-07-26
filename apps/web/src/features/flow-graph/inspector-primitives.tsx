import type { ReactNode } from "react";
import type { Tone } from "@/components/superteam";
import type { ProjectTaskGraph, ProjectTaskGraphNode } from "@/lib/api/projects";
import { blockerResourceTypeLabel } from "@/lib/status-labels";

/**
 * 任务节点详情的展示基元（spec 2026-07-26 §4.2/§4.3）。
 * 原 WorkflowNodeInspector 组件已由 ProjectTaskDetailDialog 取代，这里只保留
 * 项目详情与流程编排详情共用的状态色、阻塞语义与字段格式化算法（唯一出口）。
 */

/** 任务图内部 schema 键 → 面向用户中文标签；未知键原样返回（技术兜底）。 */
const TASK_FIELD_KEY_LABELS: Record<string, string> = {
  acceptance_criteria: "验收判据",
  expected_outputs: "预期输出",
  required_inputs: "所需输入",
};

export function taskFieldKeyLabel(key: string): string {
  return TASK_FIELD_KEY_LABELS[key.trim().toLowerCase()] ?? key;
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

export function taskStatusTone(status: string): Tone {
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

/**
 * 阻塞行文案。CP 对 failed/blocked 任务写入 resource_id 即任务自身的自指 fact，
 * 语义是「流程当前停驻在本节点」而非「任务被自己阻塞」——按停驻语义显示，
 * 不再渲染「标题·类型·UUID」自指串；类型判别字经词表映射，不英文直出。
 */
export function formatBlocker(task: ProjectTaskGraphNode): string {
  const blocker = task.current_blocker;
  if (!blocker) return "暂无阻塞";

  if (blocker.resource_id && blocker.resource_id === task.id) {
    const reason = task.status_reason?.trim();
    return reason ? `流程停驻在本节点：${reason}` : "流程停驻在本节点";
  }

  return (
    [blocker.title, blockerResourceTypeLabel(blocker.type)].filter(Boolean).join(" · ") ||
    "暂无阻塞"
  );
}

export function InspectorRow({
  action,
  label,
  value
}: {
  action?: ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="grid gap-2 py-3">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs font-bold text-ink-3">{label}</span>
        {action}
      </div>
      <p className="min-w-0 whitespace-pre-wrap break-words text-sm leading-6 text-ink">
        {value}
      </p>
    </div>
  );
}

export function formatValue(value: unknown): string {
  if (Array.isArray(value)) {
    if (value.length === 0) return "暂无";
    return value.map((item) => formatLeaf(item)).join(" · ");
  }

  if (value && typeof value === "object") {
    const entries = Object.entries(value);
    if (entries.length === 0) return "暂无";

    return entries
      .slice(0, 4)
      .map(([key, item]) => `${taskFieldKeyLabel(key)}：${formatEntryItems(item)}`)
      .join(" · ");
  }

  return formatLeaf(value);
}

/** 已知键的条目列表渲染：空数组「暂无」，条目用中文顿号衔接。 */
function formatEntryItems(value: unknown): string {
  if (Array.isArray(value)) {
    if (value.length === 0) return "暂无";
    return value.map((item) => formatLeaf(item)).join("、");
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
