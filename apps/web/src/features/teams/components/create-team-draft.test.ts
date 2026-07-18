import { describe, expect, it } from "vitest";
import { slugify } from "./create-team-draft";

describe("slugify", () => {
  it("纯中文名称按拼音逐字转写", () => {
    expect(slugify("研发团队")).toBe("yan-fa-tuan-dui");
    expect(slugify("安全响应组")).toBe("an-quan-xiang-ying-zu");
  });

  it("中英混排保留英文段并用连字符衔接", () => {
    expect(slugify("AI 平台组")).toBe("ai-ping-tai-zu");
    expect(slugify("研发Ops")).toBe("yan-fa-ops");
  });

  it("英文名称保持原有 kebab 行为", () => {
    expect(slugify("Platform Engineering")).toBe("platform-engineering");
    expect(slugify("  Ops--Team  ")).toBe("ops-team");
  });

  it("ü 转写为 v 以满足 slug 字符集", () => {
    expect(slugify("绿洲")).toBe("lv-zhou");
  });

  it("无可转写字符时返回空串", () => {
    expect(slugify("🎉🎉")).toBe("");
    expect(slugify("")).toBe("");
  });
});
