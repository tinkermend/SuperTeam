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
      statusTone: "success",
      loadLabel: "2/8",
      loadPercent: 25,
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
      statusTone: "neutral",
      loadLabel: "1/4",
      loadPercent: 25,
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
      statusTone: "success",
      loadLabel: "0/0",
      loadPercent: 0,
    });
  });

  it.each([
    ["online", "success"],
    ["offline", "neutral"],
    ["draining", "neutral"],
  ] as const)("maps %s runtime status to %s tone", (status, statusTone) => {
    expect(
      summarizeRuntimeNode({
        node_id: "node-tone",
        name: "worker",
        status,
      }),
    ).toMatchObject({
      statusTone,
    });
  });

  it("caps load percent at 100 and returns zero when capacity is not positive", () => {
    expect(
      summarizeRuntimeNode({
        node_id: "node-overloaded",
        name: "worker",
        status: "online",
        current_load: 9,
        max_slots: 4,
      }),
    ).toMatchObject({
      loadPercent: 100,
    });

    expect(
      summarizeRuntimeNode({
        node_id: "node-zero-capacity",
        name: "worker",
        status: "online",
        currentLoad: 3,
        maxSlots: 0,
      }),
    ).toMatchObject({
      loadPercent: 0,
    });
  });
});
