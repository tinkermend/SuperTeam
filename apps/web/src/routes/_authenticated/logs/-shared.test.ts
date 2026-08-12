import { describe, expect, it } from "vitest";
import { logEmptyCopy, resourceCaption, sinceWindowLabel } from "./-shared";

describe("logEmptyCopy", () => {
  it("explains time window when there is no extra filter", () => {
    expect(
      logEmptyCopy({
        noun: "操作日志",
        sinceWindow: "24h",
        hasExtraFilter: false,
        fallbackDescription: "fallback",
      }),
    ).toEqual({
      title: "近 24 小时暂无操作日志",
      description:
        "仅展示近 24 小时内的记录。若要找更早的，可切换时间范围；新产生的记录会出现在这里。",
    });
  });

  it("hints expanding the window when filters empty a bounded range", () => {
    const copy = logEmptyCopy({
      noun: "操作日志",
      sinceWindow: "24h",
      hasExtraFilter: true,
      fallbackDescription: "fallback",
    });
    expect(copy.title).toBe("近 24 小时内无匹配的操作日志");
    expect(copy.description).toContain("全部时间");
    expect(copy.description).toContain("清除筛选");
  });

  it("uses clear-filters copy when already on all time", () => {
    expect(
      logEmptyCopy({
        noun: "平台事件",
        sinceWindow: "all",
        hasExtraFilter: true,
        fallbackDescription: "fallback",
      }).title,
    ).toBe("筛选后无平台事件");
  });

  it("maps since window labels", () => {
    expect(sinceWindowLabel("24h")).toBe("近 24 小时");
    expect(sinceWindowLabel("all")).toBe("全部时间");
  });
});

describe("resourceCaption", () => {
  it("prefers human resource names over type:uuid captions", () => {
    expect(
      resourceCaption("decision_request", "fe5b585a-1111-2222-3333-444444444444", "是否批准扩编"),
    ).toBe("是否批准扩编");
    expect(resourceCaption("system_config", "team.constitution_max_chars")).toBe(
      "team.constitution_max_chars",
    );
  });
});
