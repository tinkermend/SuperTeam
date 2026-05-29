import { describe, expect, it } from "vitest";
import { createRuntimeNodeSummary } from "./runtime-node-summary";

describe("createRuntimeNodeSummary", () => {
  it("uses snake_case node id and load fields", () => {
    expect(
      createRuntimeNodeSummary({
        node_id: "node-1",
        name: "builder",
        current_load: 2,
        max_slots: 8,
      }),
    ).toEqual({
      id: "node-1",
      loadLabel: "2/8",
    });
  });

  it("uses camelCase node id and load fields", () => {
    expect(
      createRuntimeNodeSummary({
        nodeId: "node-2",
        currentLoad: 1,
        maxSlots: 4,
      }),
    ).toEqual({
      id: "node-2",
      loadLabel: "1/4",
    });
  });

  it("falls back to name for id and defaults missing counts to zero", () => {
    expect(
      createRuntimeNodeSummary({
        name: "local-runtime",
      }),
    ).toEqual({
      id: "local-runtime",
      loadLabel: "0/0",
    });
  });
});
