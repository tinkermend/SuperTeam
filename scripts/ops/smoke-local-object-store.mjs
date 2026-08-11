#!/usr/bin/env node
/**
 * 本地 RustFS 对象存储冒烟（真实 CP 路径）：
 * 1) admin 登录
 * 2) multipart 上传最小 skill zip → CP 写入 objectStore
 * 3) 校验 archive_object_ref 非空
 * 4) 删除刚上传的 skill（清理）
 *
 * 前置：rustfs 在 9000、CP 指向本地 objectStore、桶已建。
 */
import { readFileSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { login, CP, assert } from "../e2e/lib/cp-client.mjs";

const skillName = `rustfs-smoke-${Date.now()}`;

function buildSkillZip(path) {
  const skillMd = `---
name: ${skillName}
description: local rustfs object-store smoke skill
---

# ${skillName}

E2E probe only.
`;
  // zip 根目录必须出现名为 SKILL.md 的条目（CP 解析约定）。
  const dir = join(tmpdir(), `${skillName}-skill-root`);
  spawnSync("mkdir", ["-p", dir], { encoding: "utf8" });
  const mdPath = join(dir, "SKILL.md");
  writeFileSync(mdPath, skillMd, "utf8");
  const zip = spawnSync("zip", ["-j", path, mdPath], { encoding: "utf8" });
  unlinkSync(mdPath);
  if (zip.status !== 0) {
    throw new Error(`zip failed: ${zip.stderr || zip.stdout}`);
  }
}

async function uploadSkill(cookie, zipPath) {
  const blob = new Blob([readFileSync(zipPath)]);
  const form = new FormData();
  form.append("file", blob, `${skillName}.zip`);
  form.append("name", skillName);
  form.append("description", "local rustfs object-store smoke");
  form.append("risk_level", "low");

  const res = await fetch(`${CP}/api/v1/skills/uploads`, {
    method: "POST",
    headers: { cookie, accept: "application/json" },
    body: form,
  });
  const text = await res.text();
  let json;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = text;
  }
  if (!res.ok) {
    throw new Error(`upload ${res.status}: ${String(text).slice(0, 500)}`);
  }
  return json;
}

async function main() {
  console.log(`CP=${CP}`);
  const cookie = await login();
  console.log("login ok");

  const zipPath = join(tmpdir(), `${skillName}.zip`);
  buildSkillZip(zipPath);
  try {
    const skill = await uploadSkill(cookie, zipPath);
    console.log("upload skill:", {
      id: skill?.id,
      name: skill?.name,
      archive_object_ref: skill?.archive_object_ref,
      archive_size_bytes: skill?.archive_size_bytes,
    });
    assert(skill?.id, "skill id missing");
    assert(
      skill?.archive_object_ref && String(skill.archive_object_ref).length > 0,
      "archive_object_ref empty — object store put likely failed",
    );
    assert(
      String(skill.archive_object_ref).includes("s3://") ||
        String(skill.archive_object_ref).includes("skills/"),
      `unexpected archive_object_ref: ${skill.archive_object_ref}`,
    );

    // cleanup
    const del = await fetch(`${CP}/api/v1/skills/${skill.id}`, {
      method: "DELETE",
      headers: { cookie },
    });
    if (!del.ok && del.status !== 204) {
      console.warn(`cleanup delete skill ${skill.id} -> ${del.status}`);
    } else {
      console.log("cleanup delete skill ok");
    }
    console.log("SMOKE_OK local object store skill upload");
  } finally {
    try {
      unlinkSync(zipPath);
    } catch {
      /* ignore */
    }
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
