/**
 * Provider semantic fail-classification E2E (opt-in).
 *
 * Requires:
 * - Control Plane :8080 + Runtime Agent with fake provider binaries
 * - SUPERTEAM_DEV_* env for shared pid/config when restarting (optional)
 *
 * Env:
 *   SUPERTEAM_CP_URL (default http://127.0.0.1:8080)
 *   SUPERTEAM_USER / SUPERTEAM_PASS (default admin/admin)
 *   SUPERTEAM_E2E_PROJECT_ID (default provider-semantic-e2e-p1 project)
 *   SUPERTEAM_E2E_EMPLOYEE_ID (default 开发-A)
 *   SUPERTEAM_E2E_DB_URL (postgres URL; if set, swaps employee.provider_type for opencode leg)
 *   SUPERTEAM_E2E_SKIP_OPENCODE=1 to only run claude-code leg
 *
 * Not part of verify:* — real services + optional DB rewrite.
 *
 * Usage:
 *   node scripts/e2e/provider-semantic-fail-classification.mjs
 */

import { login, api, apiOk, assert } from "./lib/cp-client.mjs";
import { execFileSync } from "node:child_process";

const CP = process.env.SUPERTEAM_CP_URL || "http://127.0.0.1:8080";
const PROJECT =
  process.env.SUPERTEAM_E2E_PROJECT_ID || "5c40c4fb-d9c1-4a51-afbb-2d64cbce9877";
const EMPLOYEE =
  process.env.SUPERTEAM_E2E_EMPLOYEE_ID || "0be393bb-9dfd-48c8-b010-4b5abb114f23";
const DB_URL = process.env.SUPERTEAM_E2E_DB_URL || "";
const SKIP_OPENCODE = process.env.SUPERTEAM_E2E_SKIP_OPENCODE === "1";

async function healthOk() {
  const res = await fetch(`${CP}/health`);
  if (!res.ok) throw new Error(`CP health ${res.status}`);
  const body = await res.json();
  assert(body.status === "ok", "cp status");
  if (body.provider_contract) {
    console.log(
      "cp provider_contract",
      JSON.stringify(body.provider_contract),
    );
  }
}

function psql(sql) {
  if (!DB_URL) throw new Error("SUPERTEAM_E2E_DB_URL required for opencode leg / attempt asserts");
  return execFileSync("psql", [DB_URL, "-tAc", sql], { encoding: "utf8" }).trim();
}

async function createAndApproveDemand(cookie, title) {
  const r = await api(cookie, `/api/v1/projects/${PROJECT}/demands`, {
    method: "POST",
    body: {
      title,
      content: "请创建文件 provider-semantic-e2e.txt，内容仅为 ok。",
      source_type: "console",
      coordination_mode: "plan",
    },
  });
  assert(r.ok, `create demand ${r.status}`);
  const demandId = r.json.id;
  for (let i = 0; i < 40; i++) {
    const dec = await apiOk(cookie, `/api/v1/projects/${PROJECT}/decisions`);
    const list = Array.isArray(dec) ? dec : dec.items || [];
    const plans = list
      .filter(
        (d) =>
          d.decision_type === "plan_review" && d.status_snapshot === "pending",
      )
      .sort((a, b) => String(b.created_at).localeCompare(String(a.created_at)));
    if (plans[0]) {
      const res = await api(
        cookie,
        `/api/v1/projects/${PROJECT}/decisions/${plans[0].id}/resolve`,
        {
          method: "POST",
          body: { decision: "approved", comment: "provider-semantic e2e" },
        },
      );
      assert(res.ok, `approve ${res.status}`);
      return demandId;
    }
    await new Promise((r) => setTimeout(r, 3000));
  }
  throw new Error("plan_review not ready");
}

async function waitFail(demandId, expectProviderType) {
  for (let i = 0; i < 30; i++) {
    const row = psql(
      `SELECT pta.id||'|'||pta.error_code||'|'||pta.failure_family||'|'||pta.retryable::text
       FROM project_task_attempts pta
       JOIN project_tasks pt ON pt.id=pta.project_task_id
       WHERE pt.demand_id='${demandId}' AND pta.error_code IS NOT NULL
       ORDER BY pta.created_at LIMIT 1`,
    );
    if (row) {
      const [id, code, family, retryable] = row.split("|");
      assert(code === "RATE_LIMIT", `code ${code}`);
      assert(family === "transient_provider", `family ${family}`);
      assert(retryable === "t" || retryable === "true", `retryable ${retryable}`);
      const meta = psql(
        `SELECT metadata::text FROM execution_ledger_events
         WHERE project_task_attempt_id='${id}'
           AND event_type='provider.event'
           AND metadata::text LIKE '%RATE_LIMIT%'
         LIMIT 1`,
      );
      const parsed = JSON.parse(meta);
      const err = parsed.error || {};
      assert(
        err.provider_type === expectProviderType,
        `envelope provider_type ${err.provider_type} != ${expectProviderType}`,
      );
      assert(err.code === "RATE_LIMIT", `envelope code ${err.code}`);
      console.log("PASS", expectProviderType, id, code, family);
      return id;
    }
    await new Promise((r) => setTimeout(r, 4000));
  }
  throw new Error(`timeout waiting for fail demand=${demandId}`);
}

function setEmployeeProvider(type) {
  psql(
    `UPDATE digital_employees SET provider_type='${type}', updated_at=now() WHERE id='${EMPLOYEE}'`,
  );
  const got = psql(
    `SELECT provider_type FROM digital_employees WHERE id='${EMPLOYEE}'`,
  );
  assert(got === type, `employee provider_type ${got}`);
}

async function main() {
  console.log("health…");
  await healthOk();
  if (!DB_URL) {
    console.error(
      "SUPERTEAM_E2E_DB_URL is required (attempt error_code + envelope assert).",
    );
    process.exit(2);
  }

  const cookie = await login();
  console.log("=== claude-code leg ===");
  setEmployeeProvider("claude-code");
  const d1 = await createAndApproveDemand(
    cookie,
    `provider-semantic claude ${Date.now()}`,
  );
  await waitFail(d1, "claude-code");

  if (!SKIP_OPENCODE) {
    console.log("=== opencode leg ===");
    try {
      setEmployeeProvider("opencode");
      const d2 = await createAndApproveDemand(
        cookie,
        `provider-semantic opencode ${Date.now()}`,
      );
      await waitFail(d2, "opencode");
    } finally {
      setEmployeeProvider("claude-code");
      console.log("employee restored to claude-code");
    }
  }

  console.log("provider-semantic-fail-classification: ALL PASS");
}

main().catch((err) => {
  console.error(err);
  try {
    if (DB_URL) setEmployeeProvider("claude-code");
  } catch {
    /* ignore */
  }
  process.exit(1);
});
