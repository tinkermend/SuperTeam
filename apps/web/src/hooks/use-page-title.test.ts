import { describe, expect, it } from "vitest";
import { formatPageTitle } from "./use-page-title";

describe("formatPageTitle", () => {
  it("prefixes 炬枢", () => {
    expect(formatPageTitle("收件箱")).toBe("炬枢 · 收件箱");
    expect(formatPageTitle(" 技能市场 ")).toBe("炬枢 · 技能市场");
    expect(formatPageTitle("")).toBe("炬枢平台");
  });
});
