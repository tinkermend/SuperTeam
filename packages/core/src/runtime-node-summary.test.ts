import { describe, expect, it } from "vitest";
import { summarizeRuntimeNode } from "./runtime-node-summary";

describe("summarizeRuntimeNode", () => {
  it("uses snake_case node id and load fields", () => {
    expect(
      summarizeRuntimeNode({
        node_id: "node-1",
        name: "builder",
        status: "online",
        current_load: 2,
        max_slots: 8,
      }),
    ).toEqual({
      id: "node-1",
      name: "builder",
      status: "online",
      loadLabel: "2/8",
    });
  });

  it("uses camelCase node id and load fields", () => {
    expect(
      summarizeRuntimeNode({
        nodeId: "node-2",
        name: "developer-machine",
        status: "offline",
        currentLoad: 1,
        maxSlots: 4,
      }),
    ).toEqual({
      id: "node-2",
      name: "developer-machine",
      status: "offline",
      loadLabel: "1/4",
    });
  });

  it("defaults missing counts to zero when an explicit node id is present", () => {
    expect(
      summarizeRuntimeNode({
        node_id: "node-3",
        name: "local-runtime",
        status: "online",
      }),
    ).toEqual({
      id: "node-3",
      name: "local-runtime",
      status: "online",
      loadLabel: "0/0",
    });
  });
});
