import { describe, expect, it } from "vitest";
import { createTaskSummary } from "./task-summary";

describe("createTaskSummary", () => {
  it("normalizes the task id to string", () => {
    expect(
      createTaskSummary({
        id: 42,
        provider_type: "codex",
      }),
    ).toEqual({
      id: "42",
      providerLabel: "codex",
    });
  });

  it("uses camelCase provider type and defaults missing provider to unknown", () => {
    expect(
      createTaskSummary({
        id: "task-1",
        providerType: "opencode",
      }),
    ).toEqual({
      id: "task-1",
      providerLabel: "opencode",
    });

    expect(
      createTaskSummary({
        id: "task-2",
      }),
    ).toEqual({
      id: "task-2",
      providerLabel: "unknown",
    });
  });
});
