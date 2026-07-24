import { describe, expect, it } from "vitest";

/**
 * Soft-Flat 导入边界护栏：
 * - 既有：features/routes 不得导入 ui/button|badge|card
 * - P0 扩展：不得导入 ui/tabs|ui/alert；不得直引 sonner（须 notify*）
 * - dialog/sheet 留 P2，本护栏不拦
 *
 * @see docs/design-system/migrations/2026-07-25-soft-flat-convergence-p0-p1/
 * @see docs/design-system/migrations/2026-07-24-soft-flat-naming-unification/
 */

const sources = import.meta.glob(
  ["/src/features/**/*.{ts,tsx}", "/src/routes/**/*.{ts,tsx}"],
  {
    query: "?raw",
    import: "default",
    eager: true,
  },
) as Record<string, string>;

type GuardKind = "button" | "badge" | "card" | "tabs" | "alert" | "sonner";

const IMPORT_RE: Record<GuardKind, RegExp> = {
  button: /from\s+['"]@\/components\/ui\/button['"]/,
  badge: /from\s+['"]@\/components\/ui\/badge['"]/,
  card: /from\s+['"]@\/components\/ui\/card['"]/,
  tabs: /from\s+['"]@\/components\/ui\/tabs['"]/,
  alert: /from\s+['"]@\/components\/ui\/alert['"]/,
  sonner: /from\s+['"]sonner['"]/,
};

/** allowlist：apps/web/src 下相对路径；命中即合法。空 = 必须零导入 */
const ALLOWLIST: Record<GuardKind, string[]> = {
  button: [],
  badge: [],
  card: [],
  tabs: [],
  alert: [],
  sonner: [],
};

const KIND_HINT: Record<GuardKind, string> = {
  button: "@/components/superteam Button",
  badge: "StatusPill / Chip（禁止用 ui/badge 表达状态）",
  card: "SoftCard / WorkSurface / GlassCard",
  tabs: "SoftTabs* 或 PageTabs",
  alert: "Callout",
  sonner: "notifySuccess / notifyError / notifyWarning / notifyInfo",
};

function toRel(globKey: string): string {
  const normalized = globKey.replace(/\\/g, "/");
  const marker = "/src/";
  const idx = normalized.lastIndexOf(marker);
  if (idx >= 0) return normalized.slice(idx + marker.length);
  if (normalized.startsWith("src/")) return normalized.slice(4);
  return normalized.replace(/^\//, "");
}

/**
 * 提取每个 `<ShellPageHeader …>` 开标签文本（含自闭合），避免扫到后续 EmptyState `action=` 等误报。
 */
function extractShellPageHeaderTags(source: string): string[] {
  const tags: string[] = [];
  let searchFrom = 0;
  while (searchFrom < source.length) {
    const start = source.indexOf("<ShellPageHeader", searchFrom);
    if (start < 0) break;
    const afterName = start + "<ShellPageHeader".length;
    if (/[A-Za-z0-9_]/.test(source[afterName] ?? "")) {
      searchFrom = afterName;
      continue;
    }
    let i = afterName;
    let depth = 1;
    while (i < source.length && depth > 0) {
      if (source.startsWith("/>", i)) {
        depth -= 1;
        i += 2;
        if (depth === 0) tags.push(source.slice(start, i));
        continue;
      }
      if (source.startsWith("</", i)) {
        const gt = source.indexOf(">", i);
        depth -= 1;
        i = gt < 0 ? source.length : gt + 1;
        if (depth === 0) tags.push(source.slice(start, i));
        continue;
      }
      if (source[i] === "<") {
        depth += 1;
        i += 1;
        continue;
      }
      i += 1;
    }
    searchFrom = start + 1;
  }
  return tags;
}

const SHELL_HEADER_ACTION_PROP_RE = /\bactions?\s*=/;

describe("design-import guard (Soft-Flat P0)", () => {
  it("扫描集非空", () => {
    expect(Object.keys(sources).length).toBeGreaterThan(50);
  });

  it("ShellPageHeader 不传 action/actions（主 CTA 放 Main）", () => {
    const found: string[] = [];
    for (const [key, source] of Object.entries(sources)) {
      const rel = toRel(key);
      if (rel.includes(".test.") || rel.includes("__screenshots__")) continue;
      for (const tag of extractShellPageHeaderTags(source)) {
        if (SHELL_HEADER_ACTION_PROP_RE.test(tag)) {
          found.push(rel);
          break;
        }
      }
    }
    expect(
      found.sort(),
      "ShellPageHeader 禁止 action/actions；新建/上传/刷新/分段等放 Main（page-archetypes 实体目录）:\n" +
        found.sort().join("\n"),
    ).toEqual([]);
  });

  for (const kind of Object.keys(IMPORT_RE) as GuardKind[]) {
    it(`features/routes 不导入 ${kind === "sonner" ? "sonner" : `ui/${kind}`}`, () => {
      const allow = new Set(ALLOWLIST[kind]);
      const found = new Set<string>();
      for (const [key, source] of Object.entries(sources)) {
        const rel = toRel(key);
        if (rel.includes(".test.") || rel.includes("__screenshots__")) continue;
        if (!IMPORT_RE[kind].test(source)) continue;
        found.add(rel);
      }
      const unexpected = [...found].filter((f) => !allow.has(f)).sort();
      const stale = [...allow].filter((f) => !found.has(f)).sort();
      expect(
        unexpected,
        `禁止的业务导入（请改用 ${KIND_HINT[kind]}）:\n${unexpected.join("\n")}`,
      ).toEqual([]);
      expect(
        stale,
        `allowlist 中的 ${kind} 路径已不再导入，请同步删减 ALLOWLIST:\n${stale.join("\n")}`,
      ).toEqual([]);
    });
  }
});
