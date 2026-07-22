import { describe, expect, it } from "vitest";
import { computeNextFireAt } from "./schedule-next";

describe("computeNextFireAt", () => {
  it("returns null for disabled rules", () => {
    const next = computeNextFireAt({
      enabled: false,
      schedule_kind: "cron",
      cron_expr: "0 9 * * *",
      timezone: "Asia/Shanghai",
    });
    expect(next).toBeNull();
  });

  it("computes next daily 09:00 Asia/Shanghai fire", () => {
    // 2026-07-22 01:00 UTC = 09:00 Shanghai; next should be tomorrow 01:00 UTC
    const from = new Date("2026-07-22T01:30:00.000Z");
    const next = computeNextFireAt(
      {
        enabled: true,
        schedule_kind: "cron",
        cron_expr: "0 9 * * *",
        timezone: "Asia/Shanghai",
      },
      from,
    );
    expect(next?.toISOString()).toBe("2026-07-23T01:00:00.000Z");
  });

  it("computes next midnight Asia/Shanghai fire without hour-24 drift", () => {
    // 2026-07-22 16:30 UTC = 2026-07-23 00:30 Shanghai → next 00:00 is 2026-07-24 00:00 CST
    const from = new Date("2026-07-22T16:30:00.000Z");
    const next = computeNextFireAt(
      {
        enabled: true,
        schedule_kind: "cron",
        cron_expr: "0 0 * * *",
        timezone: "Asia/Shanghai",
      },
      from,
    );
    expect(next?.toISOString()).toBe("2026-07-23T16:00:00.000Z");
  });

  it("computes interval from latest fire with arithmetic jump", () => {
    const from = new Date("2026-07-22T12:00:00.000Z");
    const next = computeNextFireAt(
      {
        enabled: true,
        schedule_kind: "interval",
        interval_seconds: 3600,
        latest_fire: { scheduled_fire_at: "2026-07-22T11:30:00.000Z" },
      },
      from,
    );
    expect(next?.toISOString()).toBe("2026-07-22T12:30:00.000Z");
  });

  it("jumps many interval steps without looping", () => {
    const from = new Date("2026-07-22T12:00:00.000Z");
    const next = computeNextFireAt(
      {
        enabled: true,
        schedule_kind: "interval",
        interval_seconds: 3600,
        latest_fire: { scheduled_fire_at: "2026-07-20T10:00:00.000Z" },
      },
      from,
    );
    expect(next?.toISOString()).toBe("2026-07-22T13:00:00.000Z");
  });
});
