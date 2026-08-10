/**
 * Focused E2E: transient failure → attempt #2 produces runtime command →
 * after max_attempts, waiting_human with primary error_code not runtime_recovery.
 */
import { login, api, apiOk, assert } from "./lib/cp-client.mjs";
import { execFileSync } from "node:child_process";

const PROJECT = process.env.SUPERTEAM_E2E_PROJECT_ID || "5c40c4fb-d9c1-4a51-afbb-2d64cbce9877";
const PGURL = process.env.SUPERTEAM_E2E_DB_URL ||
  "postgres://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@115.190.247.9:35432/superteam?sslmode=disable";

function psql(sql) {
  return execFileSync("psql", [PGURL, "-tAc", sql], { encoding: "utf8" }).trim();
}

async function createAndApprove(cookie, title) {
  const r = await api(cookie, `/api/v1/projects/${PROJECT}/demands`, {
    method: "POST",
    body: {
      title,
      content: "请创建文件 provider-retry-e2e.txt，内容仅为 ok。",
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
        { method: "POST", body: { decision: "approved", comment: "retry redispatch e2e" } },
      );
      assert(res.ok, `approve ${res.status} ${res.text}`);
      console.log("plan approved", plans[0].id);
      return demandId;
    }
    // Also approve predispatch risk if present
    const risk = list
      .filter((d) => (d.decision_type || "").includes("approval") && d.status_snapshot === "pending")
      .sort((a, b) => String(b.created_at).localeCompare(String(a.created_at)));
    if (risk[0] && i > 5) {
      console.log("pending non-plan decision", risk[0].decision_type, risk[0].id);
    }
    await new Promise((r) => setTimeout(r, 3000));
  }
  throw new Error("plan_review not ready in 180s");
}

async function main() {
  // Preflight: single agent
  const agents = execFileSync("bash", ["-lc", "ps -ax | grep 'runtime-agent --config' | grep -v grep | wc -l"], { encoding: "utf8" }).trim();
  assert(agents === "1", `expected 1 runtime-agent, got ${agents}`);

  const cookie = await login();
  const demandId = await createAndApprove(cookie, `retry-redispatch e2e ${Date.now()}`);

  // Wait until attempt #1 has terminal PROVIDER_NO_TERMINAL_EVENT
  let taskId = "";
  let firstAttemptId = "";
  for (let i = 0; i < 40; i++) {
    const row = psql(
      `SELECT pt.id||'|'||pta.id||'|'||coalesce(pta.error_code,'-')||'|'||coalesce(pta.failure_family,'-')||'|'||coalesce(pta.retryable::text,'-')||'|'||pta.attempt_no||'|'||pt.attempt_count||'|'||pt.status
       FROM project_tasks pt
       JOIN project_task_attempts pta ON pta.project_task_id=pt.id
       WHERE pt.demand_id='${demandId}' AND pta.error_code IS NOT NULL
       ORDER BY pta.attempt_no ASC LIMIT 1`,
    );
    if (row) {
      const parts = row.split("|");
      taskId = parts[0];
      firstAttemptId = parts[1];
      console.log("first terminal attempt", row);
      assert(parts[2] === "PROVIDER_NO_TERMINAL_EVENT", `code ${parts[2]}`);
      assert(parts[3] === "transient_provider", `family ${parts[3]}`);
      assert(parts[4] === "true", `retryable ${parts[4]}`);
      break;
    }
    await new Promise((r) => setTimeout(r, 3000));
  }
  assert(taskId, "no first terminal attempt");

  // Discriminator: wait for attempt #2 to get a NEW runtime command_event
  // (runtime_events linked via runtime_task_id of attempt #2, or count command events for task's runs)
  let secondCmd = false;
  let sawAttempt2 = false;
  for (let i = 0; i < 40; i++) {
    const attempts = psql(
      `SELECT string_agg(attempt_no::text||':'||status||':'||coalesce(runtime_task_id::text,'null')||':'||coalesce(digital_employee_run_id::text,'null')||':'||coalesce(error_code,'-'), ' ; ' ORDER BY attempt_no)
       FROM project_task_attempts WHERE project_task_id='${taskId}'`,
    );
    console.log("attempts", attempts);
    if (attempts.includes("2:")) sawAttempt2 = true;

    // Distinct digital_employee_run_id bindings = distinct dispatches.
    const runs = psql(
      `SELECT count(DISTINCT pta.digital_employee_run_id)::text
       FROM project_task_attempts pta
       WHERE pta.project_task_id='${taskId}' AND pta.digital_employee_run_id IS NOT NULL`,
    );
    const cmds = psql(
      `SELECT count(*)::text FROM runtime_events
       WHERE event_type IN ('command_event','command_failed','command_completed')
         AND created_at > now() - interval '30 minutes'`,
    );
    console.log("distinct runs with binding", runs, "recent command events", cmds);

    if (parseInt(runs, 10) >= 2) {
      secondCmd = true;
      console.log("PASS: attempt #2 got a new digital_employee_run_id (redispatch happened)");
      break;
    }

    const taskRow = psql(`SELECT status||'|'||attempt_count||'|'||coalesce(waiting_reason,'-') FROM project_tasks WHERE id='${taskId}'`);
    console.log("task", taskRow);
    if (taskRow.startsWith("waiting_human") || taskRow.startsWith("failed")) {
      // finished without 2 runs — fail
      break;
    }
    await new Promise((r) => setTimeout(r, 5000));
  }

  assert(sawAttempt2, "never saw attempt #2");
  assert(secondCmd, "attempt #2 never got a new run binding — redispatch still broken");

  // Wait for exhaustion → waiting_human with primary attribution on decision card
  let finalOk = false;
  for (let i = 0; i < 60; i++) {
    const taskRow = psql(
      `SELECT status||'|'||attempt_count||'|'||coalesce(waiting_reason,'-') FROM project_tasks WHERE id='${taskId}'`,
    );
    console.log("task final poll", taskRow);
    if (taskRow.startsWith("waiting_human")) {
      const summary = psql(
        `SELECT coalesce(summary_snapshot,'') FROM project_decision_requests
         WHERE project_task_id='${taskId}' AND status_snapshot='pending'
         ORDER BY created_at DESC LIMIT 1`,
      );
      console.log("decision summary:", summary);
      assert(
        summary.includes("PROVIDER_NO_TERMINAL_EVENT") || summary.includes("执行器启动或运行失败"),
        `expected primary attribution in summary, got: ${summary}`,
      );
      assert(
        !summary.includes("Runtime did not acknowledge"),
        `summary still dominated by watchdog noise: ${summary}`,
      );
      finalOk = true;
      break;
    }
    await new Promise((r) => setTimeout(r, 5000));
  }
  assert(finalOk, "task never reached waiting_human with primary attribution");
  console.log("ALL PASS");
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
