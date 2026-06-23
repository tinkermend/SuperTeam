import { describe, expect, it } from "vitest";

const sourceModules = import.meta.glob<string>("./**/*.{ts,tsx}", {
  eager: true,
  import: "default",
  query: "?raw",
});

const allowedNativeHrefFiles = new Map<string, string>([
  ["components/skip-to-main.tsx", "same-page accessibility skip link"],
]);

const ignoredPathPrefixes = [
  "components/ui/",
];

describe("navigation rules", () => {
  it("uses TanStack Router Link or navigate for in-app navigation", () => {
    const violations = sourceFiles()
      .flatMap(({ path, source }) => findNativeHrefViolations(path, source));

    expect(violations).toEqual([]);
  });

  it("does not hard reload internal routes through location APIs", () => {
    const violations = sourceFiles()
      .flatMap(({ path, source }) => findInternalLocationNavigation(path, source));

    expect(violations).toEqual([]);
  });

  it("does not mount TanStack development corner badges", () => {
    const violations = sourceFiles()
      .flatMap(({ path, source }) => findTanStackDevtoolsMounts(path, source));

    expect(violations).toEqual([]);
  });
});

function sourceFiles() {
  return Object.entries(sourceModules)
    .map(([modulePath, source]) => ({
      path: normalizeModulePath(modulePath),
      source,
    }))
    .filter(({ path }) => {
      if (path.includes(".test.") || path === "routeTree.gen.ts") {
        return false;
      }

      return !ignoredPathPrefixes.some((prefix) => path.startsWith(prefix));
    });
}

function findNativeHrefViolations(filePath: string, source: string): string[] {
  if (!filePath.endsWith(".tsx")) {
    return [];
  }

  if (allowedNativeHrefFiles.has(filePath)) {
    return [];
  }

  const violations: string[] = [];

  for (const match of source.matchAll(/<a\b[\s\S]*?>/g)) {
    const tag = match[0];
    if (!/\bhref\s*=/.test(tag)) {
      continue;
    }

    if (isAllowedNativeAnchor(tag)) {
      continue;
    }

    violations.push(`${filePath}:${lineNumberAt(source, match.index ?? 0)} ${compactTag(tag)}`);
  }

  return violations;
}

function findInternalLocationNavigation(filePath: string, source: string): string[] {
  const patterns = [
    /\b(?:window\.)?location\.href\s*=\s*["']\//g,
    /\b(?:window\.)?location\.assign\(\s*["']\//g,
    /\b(?:window\.)?location\.replace\(\s*["']\//g,
  ];

  return patterns.flatMap((pattern) =>
    Array.from(source.matchAll(pattern), (match) => (
      `${filePath}:${lineNumberAt(source, match.index ?? 0)} ${match[0].trim()}`
    )),
  );
}

function findTanStackDevtoolsMounts(filePath: string, source: string): string[] {
  const patterns = [
    /@tanstack\/react-query-devtools/g,
    /@tanstack\/react-router-devtools/g,
    /<ReactQueryDevtools\b/g,
    /<TanStackRouterDevtools\b/g,
  ];

  return patterns.flatMap((pattern) =>
    Array.from(source.matchAll(pattern), (match) => (
      `${filePath}:${lineNumberAt(source, match.index ?? 0)} ${match[0].trim()}`
    )),
  );
}

function isAllowedNativeAnchor(tag: string) {
  if (/\bdownload(?:\s|=|>)/.test(tag)) {
    return true;
  }

  const href = readLiteralHref(tag);
  if (!href) {
    return hasBlankTarget(tag);
  }

  if (href.startsWith("#")) {
    return true;
  }

  if (/^(?:https?:|mailto:|tel:)/.test(href)) {
    return true;
  }

  return false;
}

function readLiteralHref(tag: string) {
  const direct = tag.match(/\bhref\s*=\s*(["'])(.*?)\1/);
  if (direct) {
    return direct[2];
  }

  const expression = tag.match(/\bhref\s*=\s*{\s*(["'])(.*?)\1\s*}/);
  return expression?.[2];
}

function hasBlankTarget(tag: string) {
  return /\btarget\s*=\s*(?:(["'])_blank\1|{\s*(["'])_blank\2\s*})/.test(tag);
}

function compactTag(tag: string) {
  return tag.replace(/\s+/g, " ").trim();
}

function lineNumberAt(source: string, index: number) {
  return source.slice(0, index).split("\n").length;
}

function normalizeModulePath(modulePath: string) {
  return modulePath.replace(/^\.\//, "");
}
