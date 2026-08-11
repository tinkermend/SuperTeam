/**
 * Predispatch gate zombie E2E (spec 2026-08-11).
 *
 * Strategy: stop runtime-agent so risk flag can be forced before a successful
 * run start; create demand → approve plan → set requires_human_approval →
 * approve real project_task_approval → assert release leaves approval_required
 * trap and never mints zero-approval project_task_approval zombies.
 *
 * Usage:
 *   # agent must be stopped by the harness below, or pre-stopped
 *   node scripts/e2e/predispatch-gate-zombie-e2e.mjs
 */
import { login, api, apiOk, assert } from "./lib/cp-client.mjs";
import { execFileSync } from "node:child_process";

const PROJECT = process.env.SUPERTEAM_E2E_PROJECT_ID || "5c40c4fb-d9c1-4a51-afbb-2d64cbce9877";
const PGURL =
  process.env.SUPERTEAM_E2E_DB_URL ||
  "postgres://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@115.190.247.9:35432/superteam?sslmode=disable";

function psql(sql) {
  for (let i = 0; i < 6; i++) {
    try {
      return execFileSync("psql", [PGURL, "-tAc", sql], { encoding: "utf8" }).trim();
    } catch (e) {
      if (i === 5) throw e;
      execFileSync("sleep", ["2"]);
    }
  }
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function main() {
  const cookie = await login();
  const r = await api(cookie, `/api/v1/projects/${PROJECT}/demands`, {
    method: "POST",
    body: {
      title: `predispatch-gate-zombie-e2e ${Date.now()}`,
      content: "请创建 predispatch-gate-zombie-e2e.txt，内容仅为 ok。",
      source_type: "console",
      coordination_mode: "plan",
    },
  });
  assert(r.ok, `create demand ${r.status} ${r.text}`);
  const demandId = r.json.id;
  console.log("demand", demandId);

  for (let i = 0; i < 45; i++) {
    const dec = await apiOk(cookie, `/api/v1/projects/${PROJECT}/decisions`);
    const list = Array.isArray(dec) ? dec : dec.items || [];
    const plan = list.find((d) => d.decision_type === "plan_review" && d.status_snapshot === "pending");
    if (plan) {
      const res = await api(cookie, `/api/v1/projects/${PROJECT}/decisions/${plan.id}/resolve`, {
        method: "POST",
        body: { decision: "approved", comment: "gate zombie e2e plan" },
      });
      assert(res.ok, `plan resolve ${res.status} ${res.text}`);
      console.log("plan approved", plan.id);
      break;
    }
    await sleep(2000);
  }

  let gateId = "";
  for (let i = 0; i < 40; i++) {
    psql(
      `UPDATE project_tasks SET requires_human_approval=true
       WHERE demand_id='${demandId}' AND status IN ('planned','waiting_human')`,
    );
    gateId = psql(
      `SELECT d.id::text FROM project_decision_requests d
       JOIN project_tasks t ON t.id=d.project_task_id
       WHERE t.demand_id='${demandId}' AND d.decision_type='project_task_approval'
         AND lower(d.status_snapshot)='pending' LIMIT 1`,
    );
    if (gateId) break;
    await sleep(2000);
  }
  assert(gateId, "project_task_approval gate card did not appear");
  const ar = psql(`SELECT approval_request_id::text FROM project_decision_requests WHERE id='${gateId}'`);
  assert(ar && ar !== "00000000-0000-0000-0000-000000000000", `gate missing real approval_request_id: ${ar}`);
  console.log("gate card", gateId, "approval", ar);

  const res = await api(cookie, `/api/v1/projects/${PROJECT}/decisions/${gateId}/resolve`, {
    method: "POST",
    body: { decision: "approved", comment: "gate zombie e2e approve" },
  });
  assert(res.ok, `gate resolve ${res.status} ${res.text}`);
  console.log("gate approved");

  for (let j = 0; j < 45; j++) {
    const row = psql(
      `SELECT t.status||'|'||t.attempt_count||'|'||coalesce(t.waiting_reason,'')||'|'||coalesce(t.waiting_request_id::text,'')
       FROM project_tasks t
       JOIN project_decision_requests d ON d.project_task_id=t.id
       WHERE d.id='${gateId}' LIMIT 1`,
    );
    console.log("post-approve", row);
    const [st, att, reason, ptr] = row.split("|");
    if (ptr) {
      const ps = psql(`SELECT status_snapshot FROM project_decision_requests WHERE id='${ptr}'`);
      if (st === "waiting_human" && reason === "approval_required" && ps === "approved") {
        throw new Error(`STUCK waiting_human approval_required → approved card ${ptr}`);
      }
    }
    const z = psql(
      `SELECT count(*)::text FROM project_decision_requests d
       JOIN project_tasks t ON t.id=d.project_task_id
       WHERE t.demand_id='${demandId}' AND d.decision_type='project_task_approval'
         AND d.approval_request_id='00000000-0000-0000-0000-000000000000'
         AND lower(d.status_snapshot) IN ('pending','waiting','open','requested')`,
    );
    assert(z === "0", `pending zero-approval project_task_approval zombies=${z}`);
    if (
      st === "planned" ||
      st === "queued" ||
      st === "running" ||
      Number(att) >= 1 ||
      (st === "waiting_human" && reason !== "approval_required")
    ) {
      console.log("PASS", JSON.stringify({ demandId, st, att, reason, gateId, ar }, null, 2));
      return;
    }
    await sleep(2000);
  }
  throw new Error(`timeout after gate approve demand=${demandId}`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
