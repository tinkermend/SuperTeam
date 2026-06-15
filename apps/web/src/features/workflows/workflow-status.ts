import type { Tone } from "@/components/superteam";
import type { WorkflowInstanceStatus } from "@/lib/api/projects";

export function workflowStatusLabel(status: WorkflowInstanceStatus): string {
  switch (status) {
    case "cancelled":
      return "已取消";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "planning":
      return "规划中";
    case "running":
      return "运行中";
    case "unknown":
      return "未知";
    case "waiting_human":
      return "等待人工";
    default:
      return status satisfies never;
  }
}

export function workflowStatusTone(status: WorkflowInstanceStatus): Tone {
  switch (status) {
    case "completed":
      return "success";
    case "cancelled":
    case "failed":
      return "danger";
    case "planning":
    case "waiting_human":
      return "warning";
    case "running":
      return "info";
    case "unknown":
      return "neutral";
    default:
      return status satisfies never;
  }
}
