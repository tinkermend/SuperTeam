import { describe, expect, it } from "vitest";
import { createHealthSummary } from "./health-summary";

describe("createHealthSummary", () => {
  it("formats the Control Plane health response for the console", () => {
    expect(
      createHealthSummary({
        status: "ok",
        service: "control-plane",
      }),
    ).toBe("control-plane is ok");
  });
});
