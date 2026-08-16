#!/usr/bin/env node
/**
 * 已绑定技能替换 + Runtime checksum 收敛真链：
 * 团队/员工/项目三表行数替换前后不变 → 第一次派发物化 v1 → 换包 → 第二次派发物化 v2。
 */
import { readFileSync, writeFileSync, mkdirSync, rmSync, existsSync, readdirSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync, execFileSync } from "node:child_process";
import { login, CP, api, apiOk, assert } from "../e2e/lib/cp-client.mjs";
import { resolveFixtures } from "../e2e/lib/fixtures.mjs";

const stamp = Date.now();
const slug = `bind-replace-${stamp}`;
const PGURL =
  process.env.SUPERTEAM_E2E_DB_URL ||
  "postgres://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@115.190.247.9:35432/superteam?sslmode=disable";
const RUNTIME_LOG =
  process.env.SUPERTEAM_RUNTIME_LOG ||
  "/Users/tinker/src/singe/SuperTeam/.scratch/dev-services/logs/runtime-agent.log";
const WORKSPACE_BASE = process.env.SUPERTEAM_WORKSPACE_BASE || "/var/superteam/workspaces";

function psql(sql) {
  let last = "";
  for (let i = 0; i < 4; i++) {
    try {
      return execFileSync("psql", [PGURL, "-tAc", sql], { encoding: "utf8" }).trim();
    } catch (err) {
      last = String(err.stderr || err.message || err);
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 400 * (i + 1));
    }
  }
  throw new Error(`psql failed: ${last}`);
}

function zipDir(dir, zipPath) {
  const zip = spawnSync("zip", ["-r", zipPath, "."], { cwd: dir, encoding: "utf8" });
  if (zip.status !== 0) throw new Error(`zip failed: ${zip.stderr || zip.stdout}`);
}

function buildPackage(version, body) {
  const dir = join(tmpdir(), `${slug}-${version}`);
  mkdirSync(dir, { recursive: true });
  writeFileSync(
    join(dir, "SKILL.md"),
    `---\nname: ${slug}\ndescription: bind replace runtime smoke\nversion: ${version}\n---\n\n# ${slug}\n\n${body}\n`,
    "utf8",
  );
  const zipPath = join(tmpdir(), `${slug}-${version}.zip`);
  zipDir(dir, zipPath);
  rmSync(dir, { recursive: true, force: true });
  return zipPath;
}

async function postZip(cookie, url, zipPath) {
  const form = new FormData();
  form.append("file", new Blob([readFileSync(zipPath)]), `${slug}.zip`);
  form.append("name", slug);
  form.append("description", "bind replace runtime smoke");
  form.append("risk_level", "low");
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

function bindingCounts(skillId) {
  const team = psql(`SELECT count(*) FROM team_skill_bindings WHERE skill_id='${skillId}'`);
  const agent = psql(`SELECT count(*) FROM skill_agent_bindings WHERE skill_id='${skillId}'`);
  const project = psql(`SELECT count(*) FROM project_skill_bindings WHERE skill_id='${skillId}'`);
  return { team: Number(team), agent: Number(agent), project: Number(project) };
}

function logOffset() {
  if (!existsSync(RUNTIME_LOG)) return 0;
  return statSync(RUNTIME_LOG).size;
}

function readLogSince(offset) {
  if (!existsSync(RUNTIME_LOG)) return "";
  const buf = readFileSync(RUNTIME_LOG);
  return buf.slice(offset).toString("utf8");
}

function walkFindChecksum(root, slugName, limit = 40) {
  const found = [];
  const stack = [root];
  while (stack.length && found.length < limit) {
    const dir = stack.pop();
    let entries;
    try {
      entries = readdirSync(dir, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const ent of entries) {
      const p = join(dir, ent.name);
      if (ent.isDirectory()) {
        if (ent.name === slugName) {
          const marker = join(p, ".skill-checksum");
          const md = join(p, "SKILL.md");
          if (existsSync(marker)) found.push({ dir: p, marker, md });
        }
        if (!ent.name.startsWith(".") || ent.name === ".claude" || ent.name === ".opencode" || ent.name === ".agents") {
          stack.push(p);
        }
      }
    }
  }
  return found;
}

async function sleep(ms) {
  await new Promise((r) => setTimeout(r, ms));
}

async function dispatchAndWaitConvergence(cookie, employeeId, projectId, skillId, label) {
  const offset = logOffset();
  const created = await api(cookie, `/api/v1/digital-employees/${employeeId}/runs`, {
    method: "POST",
    body: {
      objective: `${label}: reply with the word ping and stop.`,
      prompt: `${label}: 只回复 ping，不要改文件。`,
      run_kind: "chat",
      project_id: projectId,
      skill_ids: [skillId],
      interactive_confirmed: true,
    },
  });
  assert(created.ok, `create run ${label} ${created.status} ${created.text}`);
  const run = created.json;
  console.log(label, "run", run.id, "provider", run.provider_type, "command", run.command_id);

  let snippet = "";
  for (let i = 0; i < 60; i++) {
    snippet = readLogSince(offset);
    const needle = `command ${run.command_id}: skill convergence`;
    const idx = snippet.indexOf(needle);
    if (idx >= 0) {
      const line = snippet.slice(idx, snippet.indexOf("\n", idx));
      console.log(label, line);
      await api(cookie, `/api/v1/digital-employees/${employeeId}/runs/${run.id}/stop`, {
        method: "POST",
        body: { reason: "skill convergence smoke stop" },
      });
      return { run, line };
    }
    await sleep(1000);
  }
  await api(cookie, `/api/v1/digital-employees/${employeeId}/runs/${run.id}/stop`, {
    method: "POST",
    body: { reason: "skill convergence smoke timeout stop" },
  });
  throw new Error(`${label}: no skill convergence log in 60s. tail=\n${snippet.slice(-1500)}`);
}

async function waitRunIdle(cookie, employeeId, runId) {
  for (let i = 0; i < 40; i++) {
    const run = await apiOk(cookie, `/api/v1/digital-employees/${employeeId}/runs/${runId}`);
    if (run.status && !["queued", "running", "starting", "dispatched"].includes(run.status)) {
      console.log("run idle", runId, run.status);
      return run;
    }
    await sleep(500);
  }
  console.warn("run still not idle", runId);
}

async function main() {
  const cookie = await login();
  const fx = await resolveFixtures(cookie);
  assert(fx.developer?.id, "no developer employee fixture");
  assert(fx.projectId, "no project fixture");
  const employee = fx.developer;
  const teamId = employee.team_id;
  assert(teamId, `employee ${employee.name} has no team_id`);
  const personalEmployee = fx.reviewer || (fx.employees || []).find((e) => e.id !== employee.id);
  assert(personalEmployee?.id, "need a second employee for skill_agent_bindings row");
  console.log("fixture", {
    employee: employee.name,
    id: employee.id,
    teamId,
    personalEmployee: personalEmployee.name,
    personalTeam: personalEmployee.team_id,
    project: fx.project.name,
    projectId: fx.projectId,
  });

  const v1 = buildPackage("v0.1.0", "first package body");
  const v2 = buildPackage("v0.2.0", "second package body");
  let skillId = "";
  let originalProjectItems = null;
  try {
    const created = await postZip(cookie, `${CP}/api/v1/skills/uploads`, v1);
    assert(created.status === 200 || created.status === 201, `upload ${created.status} ${created.text}`);
    skillId = created.json.id;
    const checksum1 = created.json.archive_checksum_sha256;
    console.log("uploaded", skillId, checksum1);

    const teamBind = await api(cookie, `/api/v1/teams/${teamId}/skills`, {
      method: "POST",
      body: { skill_id: skillId },
    });
    assert(teamBind.ok, `team bind ${teamBind.status} ${teamBind.text}`);

    const empBind = await api(cookie, `/api/v1/digital-employees/${personalEmployee.id}/skills`, {
      method: "POST",
      body: { skill_id: skillId },
    });
    if (!empBind.ok) {
      // 同团队时控制台拒绝「团队已提供再绑员工」；为了分表断言仍插入一行 agent binding。
      const tenantId = personalEmployee.tenant_id || employee.tenant_id;
      assert(tenantId, "missing tenant_id for agent binding insert");
      psql(`INSERT INTO skill_agent_bindings (tenant_id, skill_id, digital_employee_id, status)
            SELECT '${tenantId}', '${skillId}', '${personalEmployee.id}', 'enabled'
            WHERE NOT EXISTS (
              SELECT 1 FROM skill_agent_bindings
              WHERE tenant_id='${tenantId}' AND skill_id='${skillId}' AND digital_employee_id='${personalEmployee.id}'
            )`);
      console.log("employee bind via SQL after", empBind.status, empBind.json?.code || empBind.text);
    }

    const currentBindings = await apiOk(cookie, `/api/v1/projects/${fx.projectId}/skill-bindings`);
    const currentItems = Array.isArray(currentBindings) ? currentBindings : currentBindings.items || [];
    originalProjectItems = currentItems
      .map((b) => ({ skill_id: b.skill_id || b.skill?.id }))
      .filter((i) => i.skill_id);
    const already = originalProjectItems.some((i) => i.skill_id === skillId);
    if (!already) {
      const put = await api(cookie, `/api/v1/projects/${fx.projectId}/skill-bindings`, {
        method: "PUT",
        body: { items: [...originalProjectItems, { skill_id: skillId }] },
      });
      assert(put.ok, `project bind ${put.status} ${put.text}`);
    }

    const before = bindingCounts(skillId);
    console.log("binding counts before replace", before);
    assert(before.team >= 1, "team_skill_bindings empty");
    assert(before.agent >= 1, "skill_agent_bindings empty");
    assert(before.project >= 1, "project_skill_bindings empty");

    const first = await dispatchAndWaitConvergence(cookie, employee.id, fx.projectId, skillId, "v1");
    assert(/materialized=\d+/.test(first.line), "missing convergence counters");
    await waitRunIdle(cookie, employee.id, first.run.id);

    const homes1 = walkFindChecksum(WORKSPACE_BASE, slug);
    assert(homes1.length > 0, `no materialized skill dir for ${slug} under ${WORKSPACE_BASE}`);
    const marker1 = readFileSync(homes1[0].marker, "utf8").trim();
    const md1 = readFileSync(homes1[0].md, "utf8");
    console.log("v1 marker", marker1, "dir", homes1[0].dir);
    assert(marker1.toLowerCase() === checksum1.toLowerCase(), `marker ${marker1} != ${checksum1}`);
    assert(md1.includes("first package body"), "v1 SKILL.md missing first body");

    const replaced = await postZip(cookie, `${CP}/api/v1/skills/${skillId}/archive`, v2);
    assert(replaced.status === 200, `replace ${replaced.status} ${replaced.text}`);
    const checksum2 = replaced.json.archive_checksum_sha256;
    assert(checksum2 !== checksum1, "checksum unchanged");
    const after = bindingCounts(skillId);
    console.log("binding counts after replace", after, "checksum", checksum2);
    assert(after.team === before.team, `team bindings ${before.team}→${after.team}`);
    assert(after.agent === before.agent, `agent bindings ${before.agent}→${after.agent}`);
    assert(after.project === before.project, `project bindings ${before.project}→${after.project}`);

    const skill = await apiOk(cookie, `/api/v1/skills/${skillId}`);
    assert((skill.team_bindings || []).length === after.team, "GET skill team_bindings drifted");
    assert((skill.agent_bindings || []).length === after.agent, "GET skill agent_bindings drifted");
    assert((skill.project_bindings || []).length === after.project, "GET skill project_bindings drifted");

    const second = await dispatchAndWaitConvergence(cookie, employee.id, fx.projectId, skillId, "v2");
    assert(/stamp_hit=false/.test(second.line) || /materialized=[1-9]/.test(second.line), `expected rematerialize, got ${second.line}`);
    await waitRunIdle(cookie, employee.id, second.run.id);

    const homes2 = walkFindChecksum(WORKSPACE_BASE, slug);
    const marker2 = readFileSync(homes2[0].marker, "utf8").trim();
    const md2 = readFileSync(homes2[0].md, "utf8");
    console.log("v2 marker", marker2, "dir", homes2[0].dir);
    assert(marker2.toLowerCase() === checksum2.toLowerCase(), `marker ${marker2} != ${checksum2}`);
    assert(md2.includes("second package body"), "v2 SKILL.md missing second body");

    console.log("SMOKE_OK skill replace bindings + runtime convergence", {
      skillId,
      checksum1,
      checksum2,
      before,
      after,
    });
  } finally {
    if (originalProjectItems && fx.projectId) {
      const restore = await api(cookie, `/api/v1/projects/${fx.projectId}/skill-bindings`, {
        method: "PUT",
        body: { items: originalProjectItems },
      });
      console.log("restore project bindings", restore.status);
    }
    if (skillId && teamId) {
      const u = await api(cookie, `/api/v1/teams/${teamId}/skills/${skillId}`, { method: "DELETE" });
      console.log("unbind team", u.status);
    }
    if (skillId && personalEmployee?.id) {
      const u = await api(
        cookie,
        `/api/v1/digital-employees/${personalEmployee.id}/skills/${skillId}`,
        { method: "DELETE" },
      );
      console.log("unbind employee", u.status);
    }
    if (skillId) {
      const del = await fetch(`${CP}/api/v1/skills/${skillId}`, { method: "DELETE", headers: { cookie } });
      console.log("delete skill", del.status);
    }
    for (const p of [v1, v2]) {
      try {
        rmSync(p, { force: true });
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
