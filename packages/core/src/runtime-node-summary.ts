export type RuntimeNodeSummaryInput = {
  node_id?: unknown;
  nodeId?: unknown;
  name?: unknown;
  current_load?: unknown;
  currentLoad?: unknown;
  max_slots?: unknown;
  maxSlots?: unknown;
};

export type RuntimeNodeSummary = {
  id: string;
  loadLabel: string;
};

function toCount(value: unknown): number {
  return typeof value === "number" ? value : 0;
}

export function createRuntimeNodeSummary(node: RuntimeNodeSummaryInput): RuntimeNodeSummary {
  return {
    id: String(node.node_id ?? node.nodeId ?? node.name),
    loadLabel: `${toCount(node.current_load ?? node.currentLoad)}/${toCount(
      node.max_slots ?? node.maxSlots,
    )}`,
  };
}
