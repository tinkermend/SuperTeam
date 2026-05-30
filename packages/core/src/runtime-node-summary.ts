import type { SummaryTone } from "./task-summary";

type RuntimeNodeSummaryBaseInput = {
  name: string;
  status: string;
  current_load?: number;
  currentLoad?: number;
  max_slots?: number;
  maxSlots?: number;
};

export type RuntimeNodeSummaryInput =
  | (RuntimeNodeSummaryBaseInput & {
      node_id: string | number;
      nodeId?: string | number;
    })
  | (RuntimeNodeSummaryBaseInput & {
      node_id?: string | number;
      nodeId: string | number;
    });

export type RuntimeNodeSummary = {
  id: string;
  name: string;
  status: string;
  statusTone: SummaryTone;
  loadLabel: string;
  loadPercent: number;
};

function toCount(value: unknown): number {
  return typeof value === "number" ? value : 0;
}

function toRuntimeNodeStatusTone(status: string): SummaryTone {
  return status === "online" ? "success" : "neutral";
}

function toLoadPercent(currentLoad: number, maxSlots: number): number {
  if (maxSlots <= 0) {
    return 0;
  }

  return Math.min(100, Math.round((currentLoad / maxSlots) * 100));
}

export function summarizeRuntimeNode(raw: RuntimeNodeSummaryInput): RuntimeNodeSummary {
  const currentLoad = toCount(raw.current_load ?? raw.currentLoad);
  const maxSlots = toCount(raw.max_slots ?? raw.maxSlots);

  return {
    id: String(raw.node_id ?? raw.nodeId),
    name: raw.name,
    status: raw.status,
    statusTone: toRuntimeNodeStatusTone(raw.status),
    loadLabel: `${currentLoad}/${maxSlots}`,
    loadPercent: toLoadPercent(currentLoad, maxSlots),
  };
}
