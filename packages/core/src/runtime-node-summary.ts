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
  loadLabel: string;
};

function toCount(value: unknown): number {
  return typeof value === "number" ? value : 0;
}

export function summarizeRuntimeNode(raw: RuntimeNodeSummaryInput): RuntimeNodeSummary {
  return {
    id: String(raw.node_id ?? raw.nodeId),
    name: raw.name,
    status: raw.status,
    loadLabel: `${toCount(raw.current_load ?? raw.currentLoad)}/${toCount(
      raw.max_slots ?? raw.maxSlots,
    )}`,
  };
}
