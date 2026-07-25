import { describe, expect, it } from "vitest";
import fixture from "@human-task-kind-labels";
import { HUMAN_TASK_KIND_LABELS, humanTaskKindLabel } from "./status-labels";

/**
 * 2026-07-25 §5.4 option 2: Console kind → 中文 must match the shared fixture
 * key-by-key and value-by-value (not just key set).
 */
describe("human-task-kind-labels fixture guard", () => {
  it("matches contracts/control-plane/human-task-kind-labels.json", () => {
    expect(Object.keys(HUMAN_TASK_KIND_LABELS).sort()).toEqual(Object.keys(fixture).sort());
    for (const [kind, label] of Object.entries(fixture)) {
      expect(HUMAN_TASK_KIND_LABELS[kind]).toBe(label);
      expect(humanTaskKindLabel(kind)).toBe(label);
    }
  });

  it("uses 下游放行 for downstream_release", () => {
    expect(humanTaskKindLabel("downstream_release")).toBe("下游放行");
    expect(fixture.downstream_release).toBe("下游放行");
  });
});
