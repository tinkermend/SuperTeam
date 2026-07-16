import { describe, expect, it } from "vitest";
import { formatCompactTokens, formatDurationSince } from "./formatters";

describe("formatCompactTokens", () => {
  it("keeps values below one million as grouped numbers", () => {
    expect(formatCompactTokens(0)).toBe("0");
    expect(formatCompactTokens(999_999)).toBe("999,999");
  });

  it("converts millions to M with up to two trimmed decimals", () => {
    expect(formatCompactTokens(1_000_000)).toBe("1M");
    expect(formatCompactTokens(3_234_767)).toBe("3.23M");
    expect(formatCompactTokens(120_500_000)).toBe("120.5M");
  });

  it("converts billions to B", () => {
    expect(formatCompactTokens(1_000_000_000)).toBe("1B");
    expect(formatCompactTokens(12_340_000_000)).toBe("12.34B");
  });
});

describe("formatDurationSince", () => {
  const now = new Date("2026-07-16T12:00:00Z").getTime();

  it("covers minute, hour and day granularity", () => {
    expect(formatDurationSince("2026-07-16T11:59:40Z", now)).toBe("不到 1 分钟");
    expect(formatDurationSince("2026-07-16T11:48:00Z", now)).toBe("12 分钟");
    expect(formatDurationSince("2026-07-16T09:35:00Z", now)).toBe("2 小时 25 分钟");
    expect(formatDurationSince("2026-07-16T09:00:00Z", now)).toBe("3 小时");
    expect(formatDurationSince("2026-07-13T08:00:00Z", now)).toBe("3 天 4 小时");
    expect(formatDurationSince("2026-07-13T12:00:00Z", now)).toBe("3 天");
  });

  it("returns empty for invalid timestamps", () => {
    expect(formatDurationSince("not-a-date", now)).toBe("");
  });
});
