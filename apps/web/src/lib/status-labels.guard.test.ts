import { describe, expect, it } from "vitest";

/**
 * 中文优先显示护栏（DESIGN.md「面向用户文本与枚举显示」）：
 * 扫描 features 源码，拦截"机器枚举直接渲染给用户"的最常见写法——
 * 裸成员表达式 `{xxx.status}` / `{xxx.risk_level}` 作为 JSX 元素的直接文本子节点。
 * 经 statusLabel()/riskLevelLabel() 等映射函数包裹的调用不会命中。
 * 护栏只兜常见形态，不替代规范本身；误报时优先改代码走词表，确属例外再进 ALLOWLIST。
 */
const sources = import.meta.glob("/src/features/**/*.tsx", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

// 确属例外的文件路径（技术详情区等），加入前先在 review 里说明理由。
const ALLOWLIST: string[] = [];

// `>{x.status}<`：裸枚举成员作为元素的完整文本子节点。
const BARE_ENUM_CHILD = />\s*\{\s*[\w$]+(?:[.?!][\w$]+)*[.!]?\.(status|status_snapshot|risk_level)\s*\}\s*</g;

describe("status-labels guard", () => {
  it("扫描集非空（glob 失效时此断言先红，避免护栏假绿）", () => {
    expect(Object.keys(sources).length).toBeGreaterThan(50);
  });

  it("features 源码不存在裸枚举直渲（未经 status-labels 映射）", () => {
    const violations: string[] = [];
    for (const [path, source] of Object.entries(sources)) {
      if (path.endsWith(".test.tsx") || ALLOWLIST.includes(path)) {
        continue;
      }
      for (const match of source.matchAll(BARE_ENUM_CHILD)) {
        const line = source.slice(0, match.index).split("\n").length;
        violations.push(`${path}:${line} ${match[0].trim()}`);
      }
    }
    expect(
      violations,
      `发现未经 lib/status-labels.ts 映射的裸枚举直渲，请改用 statusLabel 系列函数：\n${violations.join("\n")}`,
    ).toEqual([]);
  });
});
