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
  "data-display.md",
  "forms.md",
  "icons.md",
  "layout-density.md",
  "navigation.md",
  "overlays.md",
  "principles.md",
  "surfaces.md",
  "tokens.md",
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

async function verifyNoForbiddenPatterns(failures) {
  const checkedFiles = [
    path.join(repoRoot, "DESIGN.md"),
    ...(await Promise.all(requiredDesignDocs.map((docName) => path.join(designDir, docName)))),
    ...(await listPrototypePages()).map((pageName) => path.join(prototypeDir, pageName)),
    path.join(prototypeDir, "README.md"),
  ];

  const forbidden = [
    { pattern: /\bTODO\b|\bTBD\b|\bFIXME\b/, reason: "placeholder marker" },
    { pattern: /[ \t]$/m, reason: "trailing whitespace" },
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
}

async function main() {
  const failures = [];
  const warnings = [];

  await verifyDesignDocs(failures);
  await verifyPrototypeKit(failures, warnings);
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
