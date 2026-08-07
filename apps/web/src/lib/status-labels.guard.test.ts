import { describe, expect, it } from "vitest";

/**
 * 中文优先显示护栏（DESIGN.md「面向用户文本与枚举显示」）：
 * 扫描 features 源码，拦截"机器枚举直接渲染给用户"的最常见写法——
 * 裸成员表达式 `{xxx.status}` / `{xxx.risk_level}` 作为 JSX 元素的直接文本子节点。
 * 经 statusLabel()/riskLevelLabel() 等映射函数包裹的调用不会命中。
 * 另：拦截关联 meta 等 API 字段名作为 JSX 可见字符串字面量（跨页体验共性问题 P0）。
 * 护栏只兜常见形态，不替代规范本身；误报时优先改代码走词表，确属例外再进 ALLOWLIST。
 */
const featureSources = import.meta.glob("/src/features/**/*.{ts,tsx}", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const routeSources = import.meta.glob("/src/routes/**/*.{ts,tsx}", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const sources = { ...featureSources, ...routeSources };

// 确属例外的路径片段（配置标识面、字段表、测试）。加入前先在 review 说明理由。
const ALLOWLIST_SUBSTRINGS = [
  "/role-vocabulary/",
  "/scenario-templates/",
  "inbox-action-dialog.tsx", // 动作弹窗字段表含 demand_id 等配置标签键
  ".test.ts",
  ".test.tsx",
];

// `>{x.status}<`：裸枚举成员作为元素的完整文本子节点。
const BARE_ENUM_CHILD =
  />\s*\{\s*[\w$]+(?:[.?!][\w$]+)*[.!]?\.(status|status_snapshot|risk_level)\s*\}\s*</g;

// 工作面禁止作为可见字符串字面量的 API 字段名 meta（含 ↗ 形态）。
const FORBIDDEN_META_LITERALS = [
  "demand_id ↗",
  "source_task_id ↗",
  "source_project_id ↗",
  "source_approval_request_id ↗",
];

function isAllowlisted(path: string): boolean {
  return ALLOWLIST_SUBSTRINGS.some((part) => path.includes(part));
}

describe("status-labels guard", () => {
  it("扫描集非空（glob 失效时此断言先红，避免护栏假绿）", () => {
    expect(Object.keys(sources).length).toBeGreaterThan(50);
  });

  it("features 源码不存在裸枚举直渲（未经 status-labels 映射）", () => {
    const violations: string[] = [];
    for (const [path, source] of Object.entries(sources)) {
      if (isAllowlisted(path)) {
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

  it("工作面不把关联 API 字段名作为 JSX/可见字符串字面量", () => {
    const violations: string[] = [];
    for (const [path, source] of Object.entries(sources)) {
      if (isAllowlisted(path)) {
        continue;
      }
      for (const literal of FORBIDDEN_META_LITERALS) {
        if (source.includes(`"${literal}"`) || source.includes(`'${literal}'`) || source.includes(`\`${literal}\``)) {
          const idx = source.indexOf(literal);
          const line = source.slice(0, idx).split("\n").length;
          violations.push(`${path}:${line} "${literal}"`);
        }
      }
      // meta: "task_title" 字段名作关联 meta（勿扫 context 键数组里的 "task_title"）。
      for (const match of source.matchAll(/\bmeta\s*:\s*(["'`])task_title\1/g)) {
        const line = source.slice(0, match.index ?? 0).split("\n").length;
        violations.push(`${path}:${line} ${match[0]}`);
      }
    }
    expect(
      violations,
      `发现 API 字段名作为可见字面量，请改用 relatedRefMetaLabel / 中文 meta：\n${violations.join("\n")}`,
    ).toEqual([]);
  });
});
