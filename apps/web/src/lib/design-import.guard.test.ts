import { describe, expect, it } from "vitest";

/**
 * Soft-Flat 双轨收敛护栏（迁移 A2a）：
 * features/routes 不得【新增】对 ui/button|badge|card 的静态导入。
 * 严格模式：features/routes 不得导入 ui/button|badge|card。
 * 见 docs/design-system/migrations/2026-07-24-soft-flat-naming-unification/
 */

const sources = import.meta.glob(
  ["/src/features/**/*.{ts,tsx}", "/src/routes/**/*.{ts,tsx}"],
  {
    query: "?raw",
    import: "default",
    eager: true,
  },
) as Record<string, string>;

const IMPORT_RE: Record<"button" | "badge" | "card", RegExp> = {
  button: /from\s+['"]@\/components\/ui\/button['"]/,
  badge: /from\s+['"]@\/components\/ui\/badge['"]/,
  card: /from\s+['"]@\/components\/ui\/card['"]/,
};

/** allowlist paths relative to apps/web/src (no leading slash) */
const ALLOWLIST: Record<"button" | "badge" | "card", string[]> = {
  button: [],
  badge: [],
  card: [],
};

function toRel(globKey: string): string {
  const normalized = globKey.replace(/\\/g, "/");
  const marker = "/src/";
  const idx = normalized.lastIndexOf(marker);
  if (idx >= 0) return normalized.slice(idx + marker.length);
  if (normalized.startsWith("src/")) return normalized.slice(4);
  return normalized.replace(/^\//, "");
}

describe("design-import guard (strict zero (post Phase D))", () => {
  it("扫描集非空", () => {
    expect(Object.keys(sources).length).toBeGreaterThan(50);
  });

  for (const kind of ["button", "badge", "card"] as const) {
    it(`features/routes 不新增 ui/${kind} 导入`, () => {
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
        `新增了 ui/${kind} 业务导入（请改用 @/components/superteam）:\n${unexpected.join("\n")}`,
      ).toEqual([]);
      expect(
        stale,
        `allowlist 中的 ui/${kind} 路径已不再导入，请同步删减 ALLOWLIST:\n${stale.join("\n")}`,
      ).toEqual([]);
    });
  }
});
