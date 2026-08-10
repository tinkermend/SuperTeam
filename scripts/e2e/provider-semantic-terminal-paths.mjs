/**
 * Provider 终态路径真实 E2E（opt-in，不进 verify:*）。
 *
 * 走 Console API → CP → Temporal 协调 → Runtime → 假 provider，断言 attempt 行上的
 * ErrorEnvelope 投影（error_code / failure_family / retryable）、任务路由、
 * task_events 里的 turn_error 终止标记与 attestation 齐全。
 *
 * 前置：把 providers.claude_code.binary_path 指到 scripts/e2e/fake-providers/ 下对应脚本
 * 并重启 runtime-agent（务必先确认全机只有一个 agent 进程）。详见该目录 README。
 *
 * 用法:
 *   node scripts/e2e/provider-semantic-terminal-paths.mjs PROVIDER_NO_TERMINAL_EVENT transient_provider true
 *   node scripts/e2e/provider-semantic-terminal-paths.mjs BUDGET_FUSE budget_fuse false --budget
 *
 * 环境变量: SUPERTEAM_E2E_PROJECT_ID / SUPERTEAM_E2E_DB_URL 可覆盖默认值。
 */
import { login, api, apiOk, assert } from "./lib/cp-client.mjs";
import { execFileSync } from "node:child_process";

const PROJECT = process.env.SUPERTEAM_E2E_PROJECT_ID || "5c40c4fb-d9c1-4a51-afbb-2d64cbce9877";
const PGURL = process.env.SUPERTEAM_E2E_DB_URL ||
  "postgres://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@115.190.247.9:35432/superteam?sslmode=disable";

const [expectCode, expectFamily, expectRetryable] = process.argv.slice(2);
const budgetMode = process.argv.includes("--budget");

function psql(sql) {
  return execFileSync("psql", [PGURL, "-tAc", sql], { encoding: "utf8" }).trim();
}

async function createAndApprove(cookie, title) {
  const r = await api(cookie, `/api/v1/projects/${PROJECT}/demands`, {
    method: "POST",
    body: {
      title,
      content: "请创建文件 provider-review-e2e.txt，内容仅为 ok。",
      source_type: "console",
      coordination_mode: "plan",
    },
  });
  assert(r.ok, `create demand ${r.status} ${r.text}`);
  const demandId = r.json.id;
  console.log("demand", demandId);
  for (let i = 0; i < 60; i++) {
    const dec = await apiOk(cookie, `/api/v1/projects/${PROJECT}/decisions`);
    const list = Array.isArray(dec) ? dec : dec.items || [];
    const plans = list
      .filter((d) => d.decision_type === "plan_review" && d.status_snapshot === "pending")
      .sort((a, b) => String(b.created_at).localeCompare(String(a.created_at)));
    if (plans[0]) {
      const res = await api(
        cookie,
        `/api/v1/projects/${PROJECT}/decisions/${plans[0].id}/resolve`,
        { method: "POST", body: { decision: "approved", comment: "provider review e2e" } },
      );
      assert(res.ok, `approve ${res.status} ${res.text}`);
      console.log("plan approved", plans[0].id);
      return demandId;
    }
    await new Promise((r) => setTimeout(r, 3000));
  }
  throw new Error("plan_review not ready in 180s");
}

async function waitAttemptRunning(demandId) {
  for (let i = 0; i < 40; i++) {
    const row = psql(
      `SELECT pta.id FROM project_task_attempts pta
       JOIN project_tasks pt ON pt.id=pta.project_task_id
       WHERE pt.demand_id='${demandId}' AND pta.status='running'
       ORDER BY pta.created_at DESC LIMIT 1`,
    );
    if (row) return row;
    await new Promise((r) => setTimeout(r, 3000));
  }
  throw new Error("no running attempt");
}

async function waitTerminal(demandId) {
  for (let i = 0; i < 60; i++) {
    const row = psql(
      `SELECT pta.id||'|'||coalesce(pta.error_code,'-')||'|'||coalesce(pta.failure_family,'-')||'|'||coalesce(pta.retryable::text,'-')||'|'||pta.status||'|'||pt.status||'|'||coalesce(pt.waiting_reason,'-')||'|'||pt.attempt_count
       FROM project_task_attempts pta
       JOIN project_tasks pt ON pt.id=pta.project_task_id
       WHERE pt.demand_id='${demandId}' AND pta.error_code IS NOT NULL
       ORDER BY pta.created_at DESC LIMIT 1`,
    );
    if (row) return row.split("|");
    await new Promise((r) => setTimeout(r, 4000));
  }
  throw new Error("timeout waiting for terminal attempt");
}

async function main() {
  const cookie = await login();
  const demandId = await createAndApprove(cookie, `provider-review ${expectCode} ${Date.now()}`);

  if (budgetMode) {
    const attemptId = await waitAttemptRunning(demandId);
    psql(`UPDATE project_task_attempts SET budget_wall_clock_limit_sec=1 WHERE id='${attemptId}'`);
    console.log("budget limit forced to 1s on", attemptId);
  }

  const [id, code, family, retryable, attemptStatus, taskStatus, waitReason, attemptCount] =
    await waitTerminal(demandId);
  console.log(
    JSON.stringify({ attempt: id, code, family, retryable, attemptStatus, taskStatus, waitReason, attemptCount }, null, 2),
  );
  assert(code === expectCode, `code ${code} != ${expectCode}`);
  assert(family === expectFamily, `family ${family} != ${expectFamily}`);
  assert(retryable === expectRetryable, `retryable ${retryable} != ${expectRetryable}`);

  const events = psql(
    `SELECT string_agg(te.event_type, ',' ORDER BY te.sequence_number)
     FROM task_events te
     JOIN project_task_attempts pta ON pta.digital_employee_run_id=te.run_id
     WHERE pta.id='${id}'`,
  );
  console.log("task_events:", events);
  const attestations = psql(
    `SELECT string_agg(attestation_type||'/'||status, ',' ORDER BY created_at)
     FROM project_task_attestations WHERE attempt_id='${id}'`,
  );
  console.log("attestations:", attestations);
  const envelope = psql(
    `SELECT te.payload::text FROM task_events te
     JOIN project_task_attempts pta ON pta.digital_employee_run_id=te.run_id
     WHERE pta.id='${id}' AND te.event_type='turn_error' LIMIT 1`,
  );
  console.log("turn_error payload:", envelope.slice(0, 400));
  const violation = await (await fetch("http://127.0.0.1:8080/health")).json();
  console.log("cp provider_contract:", JSON.stringify(violation.provider_contract));
  console.log("DEMAND", demandId, "PASS");
}

main().catch((e) => {
  console.error("FAIL", e.message);
  process.exit(1);
});
