#!/usr/bin/env node
/**
 * 技能包替换 + 只读预览真链（本地 RustFS）：
 * 上传 → 预览 SKILL.md → 替换 → checksum 变、绑定数组长度不变
 * → 旧对象仍在桶里 → 同 slug 新建 409 → 穿越 zip 400 → 清理
 */
import { readFileSync, unlinkSync, writeFileSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { login, CP, assert } from "../e2e/lib/cp-client.mjs";

const stamp = Date.now();
const skillName = `archive-replace-${stamp}`;
const slug = skillName;

function zipDir(dir, zipPath) {
  const zip = spawnSync("zip", ["-r", zipPath, "."], { cwd: dir, encoding: "utf8" });
  if (zip.status !== 0) {
    throw new Error(`zip failed: ${zip.stderr || zip.stdout}`);
  }
}

function buildPackage(rootName, version, body) {
  const dir = join(tmpdir(), `${rootName}-${stamp}`);
  mkdirSync(dir, { recursive: true });
  writeFileSync(
    join(dir, "SKILL.md"),
    `---\nname: ${rootName}\ndescription: archive replace smoke\nversion: ${version}\n---\n\n# ${rootName}\n\n${body}\n`,
    "utf8",
  );
  writeFileSync(join(dir, "notes.txt"), `notes ${version}\n`, "utf8");
  const zipPath = join(tmpdir(), `${rootName}-${version}.zip`);
  zipDir(dir, zipPath);
  rmSync(dir, { recursive: true, force: true });
  return zipPath;
}

function buildTraversalZip() {
  const py = `
import zipfile, os, tempfile
path = os.path.join(tempfile.gettempdir(), "skill-traversal-${stamp}.zip")
with zipfile.ZipFile(path, "w") as z:
    z.writestr("${slug}/SKILL.md", "---\\nname: ${slug}\\n---\\n# x\\n")
    z.writestr("../escape.sh", "echo pwn\\n")
print(path)
`;
  const out = spawnSync("python3", ["-c", py], { encoding: "utf8" });
  if (out.status !== 0) {
    throw new Error(`python zip failed: ${out.stderr || out.stdout}`);
  }
  return out.stdout.trim();
}

async function postZip(cookie, url, zipPath, extra = {}) {
  const form = new FormData();
  form.append("file", new Blob([readFileSync(zipPath)]), `${skillName}.zip`);
  for (const [k, v] of Object.entries(extra)) {
    form.append(k, v);
  }
  const res = await fetch(url, {
    method: "POST",
    headers: { cookie, accept: "application/json" },
    body: form,
  });
  const text = await res.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = text;
  }
  return { status: res.status, json, text };
}

function objectKeyFromRef(ref) {
  const s = String(ref || "");
  if (s.startsWith("s3://")) {
    const rest = s.slice("s3://".length);
    const slash = rest.indexOf("/");
    return slash >= 0 ? rest.slice(slash + 1) : rest;
  }
  return s;
}

function headObject(key) {
  const py = `
import datetime, hashlib, hmac, http.client, sys
access, secret = "rustfsadmin", "rustfsadmin"
region, service, host = "us-east-1", "s3", "127.0.0.1:9000"
bucket, key = "superteam-artifacts", sys.argv[1]
t = datetime.datetime.now(datetime.UTC)
amzdate, datestamp = t.strftime("%Y%m%dT%H%M%SZ"), t.strftime("%Y%m%d")
canonical_uri = f"/{bucket}/{key}"
payload_hash = hashlib.sha256(b"").hexdigest()
canonical_headers = f"host:{host}\\nx-amz-content-sha256:{payload_hash}\\nx-amz-date:{amzdate}\\n"
signed_headers = "host;x-amz-content-sha256;x-amz-date"
canonical_request = "\\n".join(["HEAD", canonical_uri, "", canonical_headers, signed_headers, payload_hash])
scope = f"{datestamp}/{region}/{service}/aws4_request"
string_to_sign = "\\n".join(["AWS4-HMAC-SHA256", amzdate, scope, hashlib.sha256(canonical_request.encode()).hexdigest()])
kDate = hmac.new(("AWS4"+secret).encode(), datestamp.encode(), hashlib.sha256).digest()
kRegion = hmac.new(kDate, region.encode(), hashlib.sha256).digest()
kService = hmac.new(kRegion, service.encode(), hashlib.sha256).digest()
kSigning = hmac.new(kService, b"aws4_request", hashlib.sha256).digest()
sig = hmac.new(kSigning, string_to_sign.encode(), hashlib.sha256).hexdigest()
auth = f"AWS4-HMAC-SHA256 Credential={access}/{scope}, SignedHeaders={signed_headers}, Signature={sig}"
conn = http.client.HTTPConnection("127.0.0.1", 9000, timeout=5)
conn.request("HEAD", canonical_uri, headers={"Host": host, "x-amz-date": amzdate, "x-amz-content-sha256": payload_hash, "Authorization": auth})
res = conn.getresponse(); res.read(); conn.close()
raise SystemExit(0 if res.status == 200 else 1)
`;
  const r = spawnSync("python3", ["-c", py, key], { encoding: "utf8" });
  return r.status === 0;
}

async function main() {
  console.log(`CP=${CP}`);
  const cookie = await login();
  console.log("login ok");

  const v1 = buildPackage(slug, "v0.1.0", "first package body");
  const v2 = buildPackage(slug, "v0.2.0", "second package body");
  const traversal = buildTraversalZip();
  let skillId = "";
  try {
    const created = await postZip(cookie, `${CP}/api/v1/skills/uploads`, v1, {
      name: skillName,
      description: "archive replace smoke",
      risk_level: "low",
    });
    assert(created.status === 201 || created.status === 200, `upload ${created.status} ${created.text}`);
    const skill = created.json;
    skillId = skill.id;
    const oldChecksum = skill.archive_checksum_sha256;
    const oldKey = objectKeyFromRef(skill.archive_object_ref);
    console.log("upload", { id: skillId, checksum: oldChecksum, ref: skill.archive_object_ref });

    const entriesRes = await fetch(`${CP}/api/v1/skills/${skillId}/archive/entries`, {
      headers: { cookie, accept: "application/json" },
    });
    const entries = await entriesRes.json();
    assert(entriesRes.status === 200, `entries ${entriesRes.status} ${JSON.stringify(entries)}`);
    const list = Array.isArray(entries) ? entries : entries.entries;
    assert(
      list.some((e) => e.path === "SKILL.md"),
      `SKILL.md missing in ${JSON.stringify(list)}`,
    );

    const contentRes = await fetch(
      `${CP}/api/v1/skills/${skillId}/archive/content?path=${encodeURIComponent("SKILL.md")}`,
      { headers: { cookie, accept: "application/json" } },
    );
    const content = await contentRes.json();
    assert(contentRes.status === 200, `content ${contentRes.status} ${JSON.stringify(content)}`);
    assert(String(content.content).includes("first package body"), "preview missing v1 body");

    const missing = await fetch(
      `${CP}/api/v1/skills/${skillId}/archive/content?path=${encodeURIComponent("../secret")}`,
      { headers: { cookie, accept: "application/json" } },
    );
    assert(missing.status === 404, `whitelist 404 expected, got ${missing.status}`);

    const png = await fetch(
      `${CP}/api/v1/skills/${skillId}/archive/content?path=${encodeURIComponent("notes.txt")}`,
      { headers: { cookie, accept: "application/json" } },
    );
    assert(png.status === 200, `notes preview ${png.status}`);

    const before = await fetch(`${CP}/api/v1/skills/${skillId}`, {
      headers: { cookie, accept: "application/json" },
    }).then((r) => r.json());
    const teamN = (before.team_bindings || []).length;
    const agentN = (before.agent_bindings || []).length;
    const projectN = (before.project_bindings || []).length;

    const replaced = await postZip(cookie, `${CP}/api/v1/skills/${skillId}/archive`, v2);
    assert(replaced.status === 200, `replace ${replaced.status} ${replaced.text}`);
    assert(replaced.json.id === skillId, "skill id changed");
    assert(replaced.json.version === "v0.2.0", `version ${replaced.json.version}`);
    assert(
      replaced.json.archive_checksum_sha256 !== oldChecksum,
      "checksum did not change",
    );
    assert((replaced.json.team_bindings || []).length === teamN, "team bindings changed");
    assert((replaced.json.agent_bindings || []).length === agentN, "agent bindings changed");
    assert((replaced.json.project_bindings || []).length === projectN, "project bindings changed");

    const content2 = await fetch(
      `${CP}/api/v1/skills/${skillId}/archive/content?path=${encodeURIComponent("SKILL.md")}`,
      { headers: { cookie, accept: "application/json" } },
    ).then((r) => r.json());
    assert(String(content2.content).includes("second package body"), "preview missing v2 body");

    const oldStillThere = headObject(oldKey);
    assert(oldStillThere, `old object missing in rustfs: ${oldKey}`);
    console.log("old object still in bucket:", oldKey);

    const conflict = await postZip(cookie, `${CP}/api/v1/skills/uploads`, v1, {
      name: skillName,
      description: "should 409",
      risk_level: "low",
    });
    assert(conflict.status === 409, `expected 409, got ${conflict.status} ${conflict.text}`);
    assert(conflict.json?.skill_id === skillId, `409 skill_id ${JSON.stringify(conflict.json)}`);

    const bad = await postZip(cookie, `${CP}/api/v1/skills/${skillId}/archive`, traversal);
    assert(bad.status === 400, `traversal replace expected 400, got ${bad.status} ${bad.text}`);
    assert(
      String(bad.text).includes("skill_archive_unsafe_path") ||
        String(bad.text).includes("不安全"),
      `traversal body ${bad.text}`,
    );

    const afterBad = await fetch(`${CP}/api/v1/skills/${skillId}`, {
      headers: { cookie, accept: "application/json" },
    }).then((r) => r.json());
    assert(
      afterBad.archive_checksum_sha256 === replaced.json.archive_checksum_sha256,
      "traversal replace mutated archive",
    );

    console.log("SMOKE_OK skill archive replace + preview", {
      skillId,
      oldChecksum,
      newChecksum: replaced.json.archive_checksum_sha256,
      oldObjectKept: oldStillThere,
    });
  } finally {
    if (skillId) {
      const del = await fetch(`${CP}/api/v1/skills/${skillId}`, {
        method: "DELETE",
        headers: { cookie },
      });
      console.log(`cleanup delete ${skillId} -> ${del.status}`);
    }
    for (const p of [v1, v2, traversal]) {
      try {
        unlinkSync(p);
      } catch {
        /* ignore */
      }
    }
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
