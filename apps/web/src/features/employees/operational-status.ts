import { type Tone } from "@/components/superteam";
import { type DigitalEmployeeOperationalStatus } from "@/lib/api/employees";

// 数字员工运行态(operational_state.status)的中文标签与色调。单一事实源:运行总览、
// 员工列表、员工详情三处共用,保证同一运行态在各视图口径一致(跨视图一致性 P2 3.3a)。
export const operationalStatusLabel: Record<DigitalEmployeeOperationalStatus, string> = {
  working: "工作中",
  idle: "空闲",
  queued: "排队",
  waiting_human: "待人工确认",
  error: "异常",
  unavailable: "不可用",
  needs_configuration: "待配置",
};

export const operationalStatusTone: Record<DigitalEmployeeOperationalStatus, Tone> = {
  working: "info",
  idle: "ok",
  queued: "warn",
  waiting_human: "warn",
  error: "danger",
  unavailable: "mute",
  needs_configuration: "mute",
};

export type OperationalStatusPresentation = {
  label: string;
  tone: Tone;
};

export function isKnownOperationalStatus(status?: string): status is DigitalEmployeeOperationalStatus {
  return typeof status === "string" && Object.prototype.hasOwnProperty.call(operationalStatusLabel, status);
}

export function operationalStatusPresentation(status?: string): OperationalStatusPresentation {
  if (isKnownOperationalStatus(status)) {
    return {
      label: operationalStatusLabel[status],
      tone: operationalStatusTone[status],
    };
  }
  return { label: "状态未知", tone: "mute" };
}

// working/queued 视为"忙碌"(有在执行或排队的工作),供详情页判断能否发起新任务——
// 与总览的 working 判定同源,取代详情页此前基于 runs 列表本地计算的 hasActiveRun。
export function isBusyOperationalStatus(status?: string): boolean {
  return status === "working" || status === "queued";
}
