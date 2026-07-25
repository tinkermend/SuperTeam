#!/usr/bin/env node
// 一次性回填 auth_users.avatar_svg（P1-D 2b）。
//
// 后端是 Go、跑不了 dicebear，故预渲染在此用 Node + @dicebear 完成（与前端同一确定性
// 算法/参数，见 src/lib/avatar-dicebear.ts）。读 stdin 的用户 JSON 数组，为每个用户按其
// seed 渲染头像 data-URI，向 stdout 发 UPDATE 语句，交给 psql 应用。不直接连库（web
// 无 pg 依赖），保持可审查。
//
// 用法（一次性运维）：
//   psql "$DB_URL" -tAc "select json_agg(json_build_object('id',id,'seed',
//       coalesce(nullif(avatar_seed,''),'user:'||username),'options',avatar_options))
//       from auth_users where avatar_svg is null and avatar_provider='dicebear'
//       and avatar_style='adventurer'" \
//     | node apps/web/scripts/backfill-avatar-svg.mjs \
//     | psql "$DB_URL"
//
// 幂等：只处理 avatar_svg IS NULL 的行（由上面的 WHERE 决定），重跑安全。

import { createAvatar } from "@dicebear/core";
import * as adventurer from "@dicebear/adventurer";

function sqlQuote(value) {
  return "'" + String(value).replaceAll("'", "''") + "'";
}

function buildDataUri(seed, options) {
  return createAvatar(adventurer, {
    backgroundColor: ["eef8f4", "e6fbf5", "dbeafe"],
    radius: 50,
    seed,
    size: 96,
    ...(options && typeof options === "object" ? options : {}),
  }).toDataUri();
}

async function main() {
  const raw = await new Promise((resolve, reject) => {
    let buf = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => (buf += chunk));
    process.stdin.on("end", () => resolve(buf));
    process.stdin.on("error", reject);
  });

  const trimmed = raw.trim();
  if (!trimmed || trimmed === "null") {
    process.stderr.write("no users to backfill\n");
    return;
  }

  let rows;
  try {
    rows = JSON.parse(trimmed);
  } catch (err) {
    process.stderr.write(`failed to parse input JSON: ${err}\n`);
    process.exit(1);
    return;
  }
  if (!Array.isArray(rows)) {
    process.stderr.write("input must be a JSON array of {id, seed, options}\n");
    process.exit(1);
    return;
  }

  let count = 0;
  process.stdout.write("BEGIN;\n");
  for (const row of rows) {
    if (!row || !row.id || !row.seed) {
      continue;
    }
    const dataUri = buildDataUri(row.seed, row.options);
    process.stdout.write(
      `UPDATE auth_users SET avatar_svg=${sqlQuote(dataUri)} WHERE id=${sqlQuote(row.id)} AND avatar_svg IS NULL;\n`,
    );
    count += 1;
  }
  process.stdout.write("COMMIT;\n");
  process.stderr.write(`emitted UPDATE for ${count} user(s)\n`);
}

main().catch((err) => {
  process.stderr.write(`${err}\n`);
  process.exit(1);
});
