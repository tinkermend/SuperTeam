import { describe, expect, it } from "vitest";

/**
 * 两轴分离护栏（spec 2026-08-11 projects-home-status-color-and-drilldown-remediation §5.1）：
 *
 * `StatusPill.dotClassName` 只为**分类轴**（项目生命周期阶段）而开，值必须来自
 * `--phase-*` 族。用它塞语义色（ok/warn/danger/info/brand/mute/artifact）等于绕过
 * `tone` 偷渡第二个语义色编码，会重新打破 DESIGN.md「一行或一卡最多 1 个语义色状态编码」。
 *
 * 组件层测不出这条——`dotClassName` 的类型就是 `string`，JSDoc 只是君子协定。
 * 所以按仓库既有做法（`lib/status-labels.guard.test.ts`）扫源码拦截。
 * 护栏只兜「字面量里出现语义色 token」这一常见形态，不替代规范本身。
 */
const sources = {
  ...(import.meta.glob("/src/features/**/*.{ts,tsx}", {
    query: "?raw",
    import: "default",
    eager: true,
  }) as Record<string, string>),
  ...(import.meta.glob("/src/components/**/*.{ts,tsx}", {
    query: "?raw",
    import: "default",
    eager: true,
  }) as Record<string, string>),
  ...(import.meta.glob("/src/routes/**/*.{ts,tsx}", {
    query: "?raw",
    import: "default",
    eager: true,
  }) as Record<string, string>),
};

/** `dotClassName=` 之后到该 prop 结束前的取值片段（属性字符串或单层 {} 表达式）。 */
const DOT_CLASS_VALUE = /dotClassName=(?:"([^"]*)"|\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\})/g;

/** 语义色 solid/soft/text 前缀；`bg-phase-*` 不在其列。 */
const SEMANTIC_TOKEN =
  /\b(?:bg|text|border)-(?:ok|warn|danger|info|brand|mute|artifact)\b/;

function isTestFile(path: string): boolean {
  return path.includes(".test.ts") || path.includes(".test.tsx");
}

/**
 * 取出每个 `<StatusPill …>` 开标签的文本。
 *
 * 只能扫 StatusPill 自己的属性区——`dotClassName` 是个普通的 prop 名，别的本地组件
 * （如收件箱 `TimelineItem`）也在用，扫全文件会把它们全误报成违规。
 * 按花括号深度前进到深度 0 的 `>`，避开属性里 `=>`、嵌套 `{}` 带来的截断。
 */
export function statusPillOpenTags(source: string): string[] {
  const tags: string[] = [];
  const TAG_START = /<StatusPill\b/g;
  for (const start of source.matchAll(TAG_START)) {
    const from = start.index ?? 0;
    let depth = 0;
    for (let i = from; i < source.length; i += 1) {
      const ch = source[i];
      if (ch === "{") depth += 1;
      else if (ch === "}") depth -= 1;
      else if (ch === ">" && depth === 0) {
        tags.push(source.slice(from, i + 1));
        break;
      }
    }
  }
  return tags;
}

describe("StatusPill.dotClassName guard", () => {
  it("dotClassName 不得承载语义色 class（阶段轴专用）", () => {
    const violations: string[] = [];

    for (const [path, source] of Object.entries(sources)) {
      if (isTestFile(path)) continue;
      for (const tag of statusPillOpenTags(source)) {
        for (const match of tag.matchAll(DOT_CLASS_VALUE)) {
          const value = (match[1] ?? match[2] ?? "").trim();
          if (!value) continue;
          if (SEMANTIC_TOKEN.test(value)) {
            violations.push(`${path}: dotClassName={${value}}`);
          }
        }
      }
    }

    expect(
      violations,
      `dotClassName 只允许 phase 分类色（bg-phase-* 或 projectPhaseDotClass()）。\n` +
        `要表达紧迫度请改用 tone，不要从圆点偷渡语义色：\n${violations.join("\n")}`,
    ).toEqual([]);
  });

  it("护栏本身有效：识别违规、放过 phase、不误伤同名 prop 的别家组件", () => {
    // 防止正则/扫描被改坏后变成永远通过的空护栏。
    const flagged = (src: string) =>
      statusPillOpenTags(src).flatMap((tag) =>
        [...tag.matchAll(DOT_CLASS_VALUE)].filter((m) =>
          SEMANTIC_TOKEN.test((m[1] ?? m[2] ?? "").trim()),
        ),
      );

    expect(
      flagged('<StatusPill tone="mute" dotClassName="bg-danger">x</StatusPill>'),
    ).toHaveLength(1);

    expect(
      flagged('<StatusPill dotClassName={projectPhaseDotClass(s)} tone="mute" />'),
    ).toHaveLength(0);

    // 别家组件的同名 prop 不归本护栏管（收件箱 TimelineItem 就是这形态）。
    expect(flagged('<TimelineItem dotClassName="bg-ok-soft text-ok" />')).toHaveLength(0);

    // 属性里有箭头函数时开标签不能被 `>` 提前截断。
    expect(
      flagged(
        '<StatusPill onClick={() => go()} dotClassName="bg-warn" tone="mute">x</StatusPill>',
      ),
    ).toHaveLength(1);
  });
});
