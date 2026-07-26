import { describe, expect, it } from "vitest";
import { formatElapsedSince, formatRunDuration } from "./format-time";

describe("formatRunDuration", () => {
  it("formats seconds, minutes and hours in Chinese", () => {
    expect(
      formatRunDuration("2026-07-27T02:00:00Z", "2026-07-27T02:00:42Z"),
    ).toBe("42 秒");
    expect(
      formatRunDuration("2026-07-27T02:00:00Z", "2026-07-27T02:12:00Z"),
    ).toBe("12 分钟");
    expect(
      formatRunDuration("2026-07-27T02:00:00Z", "2026-07-27T02:12:30Z"),
    ).toBe("12 分 30 秒");
    expect(
      formatRunDuration("2026-07-27T02:00:00Z", "2026-07-27T04:05:00Z"),
    ).toBe("2 小时 5 分");
  });

  it("returns undefined for missing, invalid, or reversed times", () => {
    expect(formatRunDuration("2026-07-27T02:00:00Z", undefined)).toBeUndefined();
    expect(formatRunDuration("not-a-date", "2026-07-27T02:00:00Z")).toBeUndefined();
    expect(
      formatRunDuration("2026-07-27T03:00:00Z", "2026-07-27T02:00:00Z"),
    ).toBeUndefined();
  });
});

describe("formatElapsedSince", () => {
  const startMs = Date.parse("2026-07-27T02:00:00Z");

  it("rolls up minutes and hours as 已运行 labels", () => {
    expect(formatElapsedSince("2026-07-27T02:00:00Z", startMs + 20_000)).toBe(
      "已运行不足 1 分钟",
    );
    expect(
      formatElapsedSince("2026-07-27T02:00:00Z", startMs + 7 * 60_000),
    ).toBe("已运行 7 分钟");
    expect(
      formatElapsedSince("2026-07-27T02:00:00Z", startMs + 60 * 60_000),
    ).toBe("已运行 1 小时");
    expect(
      formatElapsedSince("2026-07-27T02:00:00Z", startMs + 95 * 60_000),
    ).toBe("已运行 1 小时 35 分");
  });

  it("returns undefined for invalid or future start times", () => {
    expect(formatElapsedSince("not-a-date")).toBeUndefined();
    expect(
      formatElapsedSince("2026-07-27T02:00:00Z", startMs - 1000),
    ).toBeUndefined();
  });
});
