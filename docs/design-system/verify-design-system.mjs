#!/usr/bin/env node

import { createRequire } from "node:module";
import { promises as fs } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const designDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(designDir, "../..");
const prototypeDir = path.join(repoRoot, "docs/prototypes/design-system");
const requiredDesignDocs = [
  "actions.md",
  "a11y-and-dark.md",
  "data-display.md",
  "feedback.md",
  "form-flows.md",
  "forms.md",
  "icons.md",
  "layout-density.md",
  "navigation.md",
  "overlays.md",
  "page-archetypes.md",
  "principles.md",
  "surfaces.md",
  "tokens.md",
  "visual-language.md",
];

function relative(filePath) {
  return path.relative(repoRoot, filePath);
}

async function readText(filePath) {
  return fs.readFile(filePath, "utf8");
}

async function exists(filePath) {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

function resolveInstalledLucideVersion() {
  const require = createRequire(import.meta.url);
  try {
    const packagePath = require.resolve("lucide-react/package.json", {
      paths: [path.join(repoRoot, "apps/web")],
    });
    return require(packagePath).version;
  } catch {
    return null;
  }
}

async function listPrototypePages() {
  if (!(await exists(prototypeDir))) {
    return [];
  }

  const entries = await fs.readdir(prototypeDir, { withFileTypes: true });
  return entries
    .filter((entry) => entry.isFile() && entry.name.endsWith(".html") && !entry.name.startsWith("_"))
    .map((entry) => entry.name)
    .sort();
}

async function verifyDesignDocs(failures) {
  const designIndex = await readText(path.join(repoRoot, "DESIGN.md"));
  const referencedPaths = new Set(
    [...designIndex.matchAll(/`((?:docs|apps)\/[^`]+)`/g)].map((match) => match[1]),
  );

  for (const docName of requiredDesignDocs) {
    const docPath = path.join(designDir, docName);
    if (!(await exists(docPath))) {
      failures.push(`Missing design doc: ${relative(docPath)}`);
      continue;
    }

    if (!referencedPaths.has(`docs/design-system/${docName}`)) {
      failures.push(`DESIGN.md does not route to docs/design-system/${docName}`);
    }

    const doc = await readText(docPath);
    if (!doc.startsWith("# ")) {
      failures.push(`${relative(docPath)} must start with a level-1 title`);
    }
    if (!doc.includes("## 何时阅读")) {
      failures.push(`${relative(docPath)} must include "## 何时阅读"`);
    }
  }

  for (const referencedPath of referencedPaths) {
    const absolute = path.join(repoRoot, referencedPath);
    if (!(await exists(absolute))) {
      failures.push(`Referenced path does not exist: ${referencedPath}`);
    }
  }
}

async function verifyPrototypeKit(failures, warnings) {
  if (!(await exists(prototypeDir))) {
    warnings.push(`Prototype kit directory not found; skipped prototype verification: ${relative(prototypeDir)}`);
    return;
  }

  const readmePath = path.join(prototypeDir, "README.md");
  const cssPath = path.join(prototypeDir, "design-system-prototypes.css");
  const verifyPath = path.join(prototypeDir, "verify-prototypes.mjs");
  const gitignorePath = path.join(prototypeDir, "__screenshots__/.gitignore");

  for (const filePath of [readmePath, cssPath, verifyPath, gitignorePath]) {
    if (!(await exists(filePath))) {
      failures.push(`Missing prototype kit file: ${relative(filePath)}`);
    }
  }

  const pages = await listPrototypePages();
  if (pages.length === 0) {
    failures.push(`No prototype HTML files found in ${relative(prototypeDir)}`);
  }

  const installedLucideVersion = resolveInstalledLucideVersion();
  if (!installedLucideVersion) {
    warnings.push("Cannot resolve lucide-react from apps/web; skipped Lucide version pin check.");
  }

  for (const pageName of pages) {
    const pagePath = path.join(prototypeDir, pageName);
    const html = await readText(pagePath);
    if (!html.includes('<link rel="stylesheet" href="./design-system-prototypes.css" />')) {
      failures.push(`${relative(pagePath)} must use the shared prototype CSS`);
    }
    if (html.includes("lucide@latest")) {
      failures.push(`${relative(pagePath)} must not use lucide@latest`);
    }
    if (installedLucideVersion && !html.includes(`lucide@${installedLucideVersion}/`)) {
      failures.push(`${relative(pagePath)} must pin lucide@${installedLucideVersion}`);
    }
    if (!html.includes('<script src="./prototype-icons.js"></script>')) {
      failures.push(`${relative(pagePath)} must include prototype icon fallback`);
    }
    if (!html.includes("window.renderPrototypeIcons();")) {
      failures.push(`${relative(pagePath)} must render prototype icons`);
    }
  }
}

async function verifyTokenClassTable(failures) {
  const themeCssPath = path.join(repoRoot, "apps/web/src/styles/theme.css");
  const tokensDocPath = path.join(designDir, "tokens.md");
  if (!(await exists(themeCssPath)) || !(await exists(tokensDocPath))) {
    return;
  }

  const themeCss = await readText(themeCssPath);
  // Soft-Flat：@theme 暴露 --color-<name> / --radius-<name> / --shadow-<name>
  const colorTokens = new Set(
    [...themeCss.matchAll(/--color-([a-z][a-z0-9-]*)\s*:/g)].map((match) => match[1]),
  );
  const radiusTokens = new Set(
    [...themeCss.matchAll(/--radius-([a-z][a-z0-9-]*)\s*:/g)].map((match) => match[1]),
  );
  const shadowTokens = new Set(
    [...themeCss.matchAll(/--shadow-([a-z][a-z0-9-]*)\s*:/g)].map((match) => match[1]),
  );

  // 只校验文档表格行（以 | 开头）里出现的工具类，避免误伤散文里的反例说明。
  const tableRows = (await readText(tokensDocPath))
    .split("\n")
    .filter((line) => /^\s*\|/.test(line))
    .join("\n");

  const colorClassRe =
    /\b(?:bg|text|border|ring|fill|stroke|from|to|via|divide|outline)-([a-z][a-z0-9-]*)/g;
  for (const match of tableRows.matchAll(colorClassRe)) {
    const name = match[1];
    // 跳过非 token 类（如 text-sm）——仅校验已知 Soft-Flat / shadcn 色名风格：含连字符或在 theme 中
    if (!name.includes("-") && !colorTokens.has(name)) {
      continue;
    }
    if (!colorTokens.has(name)) {
      failures.push(
        `tokens.md uses Tailwind class for missing color token --color-${name} (not exposed in theme.css @theme inline)`,
      );
    }
  }
  for (const match of tableRows.matchAll(/\brounded-([a-z][a-z0-9-]*)/g)) {
    const name = match[1];
    // shadcn 也有 rounded-sm/md/lg/xl；仅强制校验 Soft-Flat 扩展名
    if (!["card", "inner"].includes(name)) {
      continue;
    }
    if (!radiusTokens.has(name)) {
      failures.push(
        `tokens.md uses rounded-${name} but --radius-${name} is not exposed in theme.css`,
      );
    }
  }
  for (const match of tableRows.matchAll(/\bshadow-([a-z][a-z0-9-]*)\b/g)) {
    const name = match[1];
    if (!["card", "pop"].includes(name)) {
      continue;
    }
    if (!shadowTokens.has(name)) {
      failures.push(
        `tokens.md uses shadow-${name} but --shadow-${name} is not exposed in theme.css`,
      );
    }
  }
}

async function verifyNoForbiddenPatterns(failures) {
  const checkedFiles = [
    path.join(repoRoot, "DESIGN.md"),
    ...requiredDesignDocs.map((docName) => path.join(designDir, docName)),
    ...(await listPrototypePages()).map((pageName) => path.join(prototypeDir, pageName)),
    path.join(prototypeDir, "README.md"),
  ];

  const forbidden = [
    { pattern: /\bTODO\b|\bTBD\b|\bFIXME\b/, reason: "placeholder marker" },
    { pattern: /[ \t]$/m, reason: "trailing whitespace" },
  ];

  // Soft-Flat 现行文档禁止旧 v3 路径 / token / 工具类（迁移目录与历史原型路径除外）。
  // 允许：design-direction-v3 目录名；tokens 反例 `v3-primary`；散文中的“历史上称 v3”。
  const softFlatDocFiles = [
    path.join(repoRoot, "DESIGN.md"),
    ...requiredDesignDocs.map((docName) => path.join(designDir, docName)),
  ];
  const softFlatForbidden = [
    {
      pattern: /v3-components(?:\.tsx)?/,
      reason: "stale path v3-components (use components/superteam/primitives.tsx)",
    },
    {
      pattern: /--v3-[a-z0-9-]+/,
      reason: "stale CSS variable prefix --v3-* (use semantic Soft-Flat tokens)",
    },
    {
      pattern:
        /\b(?:bg|text|border|ring|fill|stroke|from|to|via|divide|outline|rounded|shadow)-v3-[a-z0-9-]+\b/,
      reason: "stale Tailwind class with v3- prefix",
    },
    {
      pattern: /\.v3-glass\b/,
      reason: "stale class .v3-glass (use .glass)",
    },
    {
      pattern: /\bV3(?:Button|Table|EmptyState|ErrorState|LoadingState|Tone|Chip|Tabs)\b/,
      reason: "stale V3* component API name (use Soft-Flat names without V3 prefix)",
    },
  ];

  for (const filePath of checkedFiles) {
    if (!(await exists(filePath))) {
      continue;
    }
    const content = await readText(filePath);
    for (const rule of forbidden) {
      if (rule.pattern.test(content)) {
        failures.push(`${relative(filePath)} contains ${rule.reason}`);
      }
    }
  }

  for (const filePath of softFlatDocFiles) {
    if (!(await exists(filePath))) {
      continue;
    }
    const content = await readText(filePath);
    // Strip fenced code / inline backticks that are explicit anti-examples for hallucination names.
    // Keep checking the whole file for path/token drift; allow `v3-primary` anti-example only.
    const sanitized = content.replace(/`v3-primary`/g, "`«hallucination»`");
    for (const rule of softFlatForbidden) {
      if (rule.pattern.test(sanitized)) {
        failures.push(`${relative(filePath)} contains ${rule.reason}`);
      }
    }
  }
}

async function main() {
  const failures = [];
  const warnings = [];

  await verifyDesignDocs(failures);
  await verifyPrototypeKit(failures, warnings);
  await verifyTokenClassTable(failures);
  await verifyNoForbiddenPatterns(failures);

  for (const warning of warnings) {
    console.warn(`[design-system] warning: ${warning}`);
  }

  if (failures.length > 0) {
    console.error("Design system verification failed:");
    for (const failure of failures) {
      console.error(`- ${failure}`);
    }
    process.exitCode = 1;
    return;
  }

  console.log("Design system verification passed.");
}

main().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
