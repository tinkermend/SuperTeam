#!/usr/bin/env node
// 入口包体积护栏（P1-D Step 4）：构建后校验入口 chunk 的 gzip 体积不超过阈值，
// 防止重依赖（xyflow/dicebear/monaco/recharts 等）被重新静态引入首屏而悄悄回潮。
//
// 阈值给了充足余量（当前入口约 85 KB gz），不是精确卡水位——只在发生「把大依赖塞回
// 入口」这类明显回归时报错。合法地让入口变大（如新增框架级依赖）时，连同这里一起调整
// 并在 PR 说明原因。

import { gzipSync } from "node:zlib";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const ENTRY_GZIP_LIMIT_KB = 150;

const here = dirname(fileURLToPath(import.meta.url));
const assetsDir = join(here, "..", "dist", "assets");

function fail(message) {
  process.stderr.write(`bundle-size guard: ${message}\n`);
  process.exit(1);
}

let files;
try {
  files = readdirSync(assetsDir);
} catch (err) {
  fail(`cannot read ${assetsDir} — run vite build first (${err})`);
}

const entryFiles = files.filter((name) => /^index-.*\.js$/.test(name));
if (entryFiles.length !== 1) {
  fail(`expected exactly one entry chunk index-*.js, found ${entryFiles.length}: ${entryFiles.join(", ")}`);
}

const entryPath = join(assetsDir, entryFiles[0]);
const gzipKb = gzipSync(readFileSync(entryPath)).length / 1024;

if (gzipKb > ENTRY_GZIP_LIMIT_KB) {
  fail(
    `entry chunk ${entryFiles[0]} is ${gzipKb.toFixed(1)} KB gzip, over the ${ENTRY_GZIP_LIMIT_KB} KB limit. ` +
      `A heavy dependency was likely pulled back into the first-screen bundle — lazy-load it (React.lazy / dynamic import) ` +
      `or, if the growth is intentional, raise ENTRY_GZIP_LIMIT_KB in scripts/check-bundle-size.mjs with a note.`,
  );
}

process.stdout.write(`bundle-size guard: entry ${entryFiles[0]} = ${gzipKb.toFixed(1)} KB gzip (limit ${ENTRY_GZIP_LIMIT_KB} KB) ✓\n`);
