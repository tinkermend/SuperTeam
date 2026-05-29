import { describe, expect, it } from "vitest";
import { summarizeTask } from "./task-summary";

describe("summarizeTask", () => {
  it("normalizes snake_case task data into the console summary shape", () => {
    expect(
      summarizeTask({
        id: 42,
        title: "Analyze requirements",
        status: "pending",
        provider_type: "codex",
      }),
    ).toEqual({
      id: "42",
      title: "Analyze requirements",
      status: "pending",
      providerLabel: "codex",
    });
  });

  it("uses camelCase provider type and defaults missing provider to unknown", () => {
    expect(
      summarizeTask({
        id: "task-1",
        title: "Implement boundary",
        status: "running",
        providerType: "opencode",
      }),
    ).toEqual({
      id: "task-1",
      title: "Implement boundary",
      status: "running",
      providerLabel: "opencode",
    });

    expect(
      summarizeTask({
        id: "task-2",
        title: "Review result",
        status: "completed",
      }),
    ).toEqual({
      id: "task-2",
      title: "Review result",
      status: "completed",
      providerLabel: "unknown",
    });
  });
});
