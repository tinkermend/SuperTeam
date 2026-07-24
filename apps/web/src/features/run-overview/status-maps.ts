import type { RuntimeOverviewEmployee } from "./runtime-overview-model";

export const employeeStatusLabel: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "异常",
  idle: "空闲",
  needs_configuration: "待配置",
  queued: "排队",
  unavailable: "不可用",
  waiting_human: "待确认",
  working: "工作中",
};

export const employeeStatusDotClass: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "bg-danger",
  idle: "bg-mute",
  needs_configuration: "bg-warn",
  queued: "bg-info",
  unavailable: "bg-mute",
  waiting_human: "bg-warn",
  working: "bg-ok",
};

export function employeeStatusTone(status: RuntimeOverviewEmployee["status"]) {
  if (status === "error") return "danger" as const;
  if (status === "working") return "ok" as const;
  if (status === "waiting_human" || status === "needs_configuration" || status === "queued") return "warn" as const;
  return "mute" as const;
}
