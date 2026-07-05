import type { V3Tone } from "@/components/superteam";
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

export function workflowStatusTone(status: WorkflowInstanceStatus): V3Tone {
  switch (status) {
    case "completed":
      return "ok";
    case "cancelled":
    case "failed":
      return "danger";
    case "waiting_human":
      return "warn";
    case "running":
      return "info";
    // 规划中是协调线程的正常工作态，不需要人介入，保持安静
    case "planning":
    case "unknown":
      return "mute";
    default:
      return status satisfies never;
  }
}
