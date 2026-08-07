import { describe, expect, it } from "vitest";
import { skillSourceLabel } from "./skill-labels";

describe("skillSourceLabel", () => {
  it("maps known sources to Chinese labels", () => {
    expect(skillSourceLabel("upload")).toBe("上传");
    expect(skillSourceLabel("system")).toBe("系统内置");
    expect(skillSourceLabel("marketplace")).toBe("市场");
    expect(skillSourceLabel("none")).toBe("未记录");
  });

  it("falls back safely for empty or unknown values", () => {
    expect(skillSourceLabel("")).toBe("未记录");
    expect(skillSourceLabel(null)).toBe("未记录");
    expect(skillSourceLabel("custom-pack")).toBe("custom-pack");
  });
});
