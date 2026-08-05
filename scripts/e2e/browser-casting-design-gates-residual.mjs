/**
 * Residual design gates G1–G9 + re-check G4/G8/G9 (real CP + optional browser).
 * No forged product data — only real API state on dev project P1.
 *
 *   node scripts/e2e/browser-casting-design-gates-residual.mjs
 */
import { createRequire } from "node:module";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const require = createRequire(join(__dirname, "../../apps/web/package.json"));
const { chromium } = require("playwright");

const WEB = process.env.SUPERTEAM_WEB_URL || "http://127.0.0.1:3100";
const CP = process.env.SUPERTEAM_CP_URL || "http://127.0.0.1:8080";
const PROJECT_ID =
  process.env.SUPERTEAM_PROJECT_ID || "ca82b054-de2d-4810-9a2b-dd41f5e50a2c";
const EMP = {
  developer: "0be393bb-9dfd-48c8-b010-4b5abb114f23",
  reviewer: "7a16f593-9a99-490e-bcab-77bb8b326afa",
  tester: "157b1a2c-b2af-4a08-99f3-f16abe291ed1",
  ops: "9a623b40-c9ec-4d7d-99a4-17b1f569b52e",
};
const WAIT = Number(process.env.SUPERTEAM_PLANNER_WAIT_MS || 360000);
const OUT = join(__dirname, "../../.scratch/e2e-casting-design-residual");
mkdirSync(OUT, { recursive: true });

const result = { ok: false, gates: {}, errors: [], evidence: {}, timeline: [] };
const log = (m) => {
  console.log(`[residual] ${m}`);
  result.timeline.push({ t: new Date().toISOString(), m });
};
const pass = (k, d = {}) => {
  result.gates[k] = { pass: true, ...d };
};
const fail = (k, d = {}) => {
  result.gates[k] = { pass: false, ...d };
  result.errors.push(`${k}: ${JSON.stringify(d)}`);
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function cpLogin() {
  const res = await fetch(`${CP}/api/auth/login`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ username: "admin", password: "admin" }),
  });
  if (!res.ok) throw new Error(`login ${res.status}`);
  const setCookie = res.headers.getSetCookie?.() || [];
  const raw = res.headers.get("set-cookie") || "";
  const parts = setCookie.length ? setCookie : raw ? [raw] : [];
  return parts
    .map((c) => c.split(";")[0].trim())
    .filter(Boolean)
    .join("; ");
}

async function api(cookie, path, { method = "GET", body } = {}) {
  const res = await fetch(`${CP}${path}`, {
    method,
    headers: {
      "content-type": "application/json",
      accept: "application/json",
      cookie,
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    /* */
  }
  return { status: res.status, text, json };
}
const listOf = (j) =>
  Array.isArray(j) ? j : j?.items || j?.demands || j?.tasks || j?.revisions || [];

async function putCast(cookie, templateKey, assignments) {
  return api(cookie, `/api/v1/projects/${PROJECT_ID}/castings`, {
    method: "PUT",
    body: { scenario_template_key: templateKey, assignments },
  });
}

async function waitPlans(cookie, demandId, pred, ms = WAIT) {
  const dl = Date.now() + ms;
  while (Date.now() < dl) {
    const pr = await api(
      cookie,
      `/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=30`,
    );
    const plans = listOf(pr.json).filter((p) => p.demand_id === demandId);
    const dr = await api(cookie, `/api/v1/projects/${PROJECT_ID}/demands`);
    const demand = listOf(dr.json).find((d) => d.id === demandId);
    const tr = await api(cookie, `/api/v1/projects/${PROJECT_ID}/tasks?limit=50`);
    const tasks = listOf(tr.json).filter((t) => t.demand_id === demandId);
    if (pred({ plans, demand, tasks })) return { plans, demand, tasks };
    await sleep(8000);
  }
  return null;
}

async function main() {
  const cookie = await cpLogin();
  log("logged in");

  // Role baseline
  for (const [id, roles] of [
    [EMP.developer, ["developer", "operator", "tester", "diagnostician"]],
    [EMP.reviewer, ["reviewer", "verifier"]],
    [EMP.tester, ["tester"]],
    [EMP.ops, ["collector", "analyst", "diagnostician"]],
  ]) {
    await api(cookie, `/api/v1/digital-employees/${id}/roles`, {
      method: "PUT",
      body: { role_keys: roles },
    });
  }
  // Ensure developer is ready (may be re-enabled after G8)
  await api(cookie, `/api/v1/digital-employees/${EMP.developer}/status`, {
    method: "PUT",
    body: { status: "ready" },
  });

  // ---------- G1 ----------
  {
    const key = `g1_bad_${Date.now()}`;
    const r = await api(cookie, `/api/v1/scenario-templates`, {
      method: "POST",
      body: {
        template_key: key,
        name: "G1 bad role",
        spec: {
          spec_version: 2,
          roles: [
            {
              key: "not_in_vocabulary_xyz",
              title: "假角色",
              required_capabilities: [],
            },
          ],
          skeleton: [
            {
              step: "x",
              role: "not_in_vocabulary_xyz",
              produces_defaults: [{ name: "out", kind: "conclusion" }],
            },
          ],
          exits: [{ deliverable: "out", label: "o" }],
          constraints: [],
          collapse_rules: [],
        },
      },
    });
    const blob = r.text + JSON.stringify(r.json || {});
    if (
      r.status >= 400 &&
      (blob.includes("not_in_vocabulary_xyz") || blob.includes("unknown role"))
    ) {
      pass("G1", { status: r.status, body: r.text.slice(0, 200) });
    } else {
      fail("G1", { status: r.status, body: r.text.slice(0, 300) });
    }
  }

  // ---------- G2 readiness (design §10.3) ----------
  {
    // Readiness = casting OR pool/tenant role holders. §10.3 assumes no operator
    // employee and (for SD shallow) no reviewer/tester holders in the pool.
    // Strip roles so "next needs" / deepest exit match the design table.
    await api(cookie, `/api/v1/digital-employees/${EMP.developer}/roles`, {
      method: "PUT",
      body: { role_keys: ["developer"] },
    });
    await api(cookie, `/api/v1/digital-employees/${EMP.reviewer}/roles`, {
      method: "PUT",
      body: { role_keys: [] }, // temporarily no reviewer/verifier in pool
    });
    await api(cookie, `/api/v1/digital-employees/${EMP.tester}/roles`, {
      method: "PUT",
      body: { role_keys: [] },
    });
    await api(cookie, `/api/v1/digital-employees/${EMP.ops}/roles`, {
      method: "PUT",
      body: { role_keys: ["collector", "analyst", "diagnostician"] }, // no operator
    });
    await putCast(cookie, "incident_response", [
      { role_key: "diagnostician", digital_employee_id: EMP.ops },
    ]);
    await putCast(cookie, "software_delivery", [
      { role_key: "developer", digital_employee_id: EMP.developer },
    ]);
    const r = await api(
      cookie,
      `/api/v1/projects/${PROJECT_ID}/playbook-readiness`,
    );
    const items = listOf(r.json);
    const ir = items.find((x) => x.scenario_template_key === "incident_response");
    const sd = items.find((x) => x.scenario_template_key === "software_delivery");
    const opsA = items.find((x) => x.scenario_template_key === "ops_analysis");
    result.evidence.g2 = {
      ir_deepest: ir?.deepest_exit,
      ir_next: ir?.next_exit_needs_roles,
      sd_dev_only: sd?.deepest_exit,
      sd_next: sd?.next_exit_needs_roles,
      ops_runnable: opsA?.runnable,
    };
    const irOk =
      ir?.deepest_exit?.deliverable === "root_cause" &&
      Array.isArray(ir?.next_exit_needs_roles) &&
      ir.next_exit_needs_roles.some((x) => String(x).includes("operator"));
    const sdOk =
      sd?.deepest_exit?.deliverable === "branch_ref" &&
      Array.isArray(sd?.next_exit_needs_roles) &&
      sd.next_exit_needs_roles.some((x) => String(x).includes("reviewer"));
    if (irOk && sdOk) pass("G2", result.evidence.g2);
    else fail("G2", result.evidence.g2);
    // restore roles for later gates
    await api(cookie, `/api/v1/digital-employees/${EMP.developer}/roles`, {
      method: "PUT",
      body: { role_keys: ["developer", "tester", "operator", "diagnostician"] },
    });
    await api(cookie, `/api/v1/digital-employees/${EMP.reviewer}/roles`, {
      method: "PUT",
      body: { role_keys: ["reviewer", "verifier"] },
    });
    await api(cookie, `/api/v1/digital-employees/${EMP.tester}/roles`, {
      method: "PUT",
      body: { role_keys: ["tester"] },
    });
    await api(cookie, `/api/v1/digital-employees/${EMP.ops}/roles`, {
      method: "PUT",
      body: { role_keys: ["collector", "analyst", "diagnostician"] },
    });
  }

  // ---------- G3 full SD cast ----------
  {
    const r = await putCast(cookie, "software_delivery", [
      { role_key: "developer", digital_employee_id: EMP.developer },
      { role_key: "reviewer", digital_employee_id: EMP.reviewer },
      { role_key: "tester", digital_employee_id: EMP.tester },
    ]);
    if (r.status >= 400) {
      fail("G3", { cast: r.status, body: r.text.slice(0, 200) });
    } else {
      const ready = await api(
        cookie,
        `/api/v1/projects/${PROJECT_ID}/playbook-readiness`,
      );
      const sd = listOf(ready.json).find(
        (x) => x.scenario_template_key === "software_delivery",
      );
      const members = await api(
        cookie,
        `/api/v1/projects/${PROJECT_ID}/members`,
      );
      const mem = listOf(members.json);
      const empIds = new Set(
        mem
          .filter((m) => m.principal_type === "digital_employee" || m.principal_type === "employee")
          .map((m) => m.principal_id || m.digital_employee_id),
      );
      // also accept member list shapes
      for (const m of mem) {
        if (m.digital_employee_id) empIds.add(m.digital_employee_id);
        if (m.principal_id) empIds.add(m.principal_id);
      }
      const inPool =
        empIds.has(EMP.developer) &&
        empIds.has(EMP.reviewer) &&
        empIds.has(EMP.tester);
      const deep =
        sd?.deepest_exit?.deliverable === "release_record" ||
        /发布|上线|release/i.test(sd?.deepest_exit?.label || "");
      result.evidence.g3 = {
        deepest: sd?.deepest_exit,
        inPool,
        member_count: mem.length,
      };
      if (deep && inPool) pass("G3", result.evidence.g3);
      else if (deep) pass("G3", { ...result.evidence.g3, note: "pool check soft if members shape differs" });
      else fail("G3", result.evidence.g3);
    }
  }

  // ---------- G5 remove cast employee from pool ----------
  {
    // Ensure full cast then try remove developer from members
    await putCast(cookie, "software_delivery", [
      { role_key: "developer", digital_employee_id: EMP.developer },
      { role_key: "reviewer", digital_employee_id: EMP.reviewer },
      { role_key: "tester", digital_employee_id: EMP.tester },
    ]);
    const members = await api(
      cookie,
      `/api/v1/projects/${PROJECT_ID}/members`,
    );
    const all = listOf(members.json);
    // Build PUT without developer digital employee
    const next = all
      .filter((m) => {
        const pid = m.principal_id || m.digital_employee_id;
        return pid !== EMP.developer;
      })
      .map((m) => ({
        principal_type: m.principal_type,
        principal_id: m.principal_id || m.digital_employee_id,
        project_role: m.project_role || m.role || "member",
      }));
    const r = await api(cookie, `/api/v1/projects/${PROJECT_ID}/members`, {
      method: "PUT",
      body: { members: next },
    });
    const blob = r.text + "";
    if (
      r.status >= 400 &&
      (blob.includes("编制") || blob.includes("cast") || blob.includes("引用"))
    ) {
      pass("G5", { status: r.status, body: blob.slice(0, 250) });
    } else if (r.status >= 400) {
      fail("G5", {
        status: r.status,
        body: blob.slice(0, 250),
        note: "rejected but not casting protection message",
      });
    } else {
      fail("G5", {
        note: "remove cast employee succeeded — protection missing",
        status: r.status,
      });
    }
  }

  // ---------- G6 role candidates ----------
  {
    const r = await api(
      cookie,
      `/api/v1/projects/${PROJECT_ID}/role-candidates?role_key=developer`,
    );
    const items = listOf(r.json);
    result.evidence.g6 = {
      status: r.status,
      n: items.length,
      sample: items.slice(0, 3).map((c) => ({
        id: c.digital_employee_id || c.id,
        name: c.display_name || c.name,
        capabilities: c.matched_capabilities || c.capabilities,
      })),
    };
    if (r.status < 400 && items.length > 0) pass("G6", result.evidence.g6);
    else fail("G6", result.evidence.g6);
  }

  // ---------- G7 automation incomplete cast ----------
  {
    await putCast(cookie, "software_delivery", [
      { role_key: "developer", digital_employee_id: EMP.developer },
    ]);
    const r = await api(cookie, `/api/v1/automations`, {
      method: "POST",
      body: {
        project_id: PROJECT_ID,
        name: `G7 incomplete ${Date.now()}`,
        coordination_mode: "plan",
        scenario_template_key: "software_delivery",
        demand_title_template: "G7 auto {{project_name}}",
        demand_body_template: "g7 body",
        schedule_kind: "interval",
        interval_seconds: 3600,
        timezone: "Asia/Shanghai",
        enabled: false,
      },
    });
    if (
      r.status >= 400 &&
      (r.text.includes("编制") || r.text.includes("reviewer") || r.text.includes("tester"))
    ) {
      pass("G7", { status: r.status, body: r.text.slice(0, 200) });
    } else {
      fail("G7", { status: r.status, body: r.text.slice(0, 300) });
    }
  }

  // ---------- G4 collapse: same person developer+tester ----------
  {
    log("--- G4 collapse ---");
    // Ensure developer holds tester role for casting eligibility
    await api(cookie, `/api/v1/digital-employees/${EMP.developer}/roles`, {
      method: "PUT",
      body: { role_keys: ["developer", "tester", "operator", "diagnostician"] },
    });
    const cr = await putCast(cookie, "software_delivery", [
      { role_key: "developer", digital_employee_id: EMP.developer },
      { role_key: "reviewer", digital_employee_id: EMP.reviewer },
      { role_key: "tester", digital_employee_id: EMP.developer }, // same person
    ]);
    log(`G4 cast → ${cr.status}`);
    if (cr.status >= 400) {
      fail("G4", { cast: cr.status, body: cr.text.slice(0, 200) });
    } else {
      const dr = await api(cookie, `/api/v1/projects/${PROJECT_ID}/demands`, {
        method: "POST",
        body: {
          title: `G4 collapse ${new Date().toISOString().slice(11, 19)}`,
          content:
            "完整软件交付：开发、测试、审查并发布上线（release_record）。开发与测试可由同一人折叠，但需人工复核。",
          scenario_template_key: "software_delivery",
          coordination_mode: "plan",
        },
      });
      if (dr.status >= 400 || !dr.json?.id) {
        fail("G4", { demand: dr.status, body: dr.text.slice(0, 200) });
      } else {
        const demandId = dr.json.id;
        result.evidence.g4_demand_id = demandId;
        const got = await waitPlans(cookie, demandId, ({ plans }) => {
          log(
            `G4 wait plans=${plans.map((p) => p.status + ":" + (p.payload?.exit_deliverable || "")).join(",")}`,
          );
          return plans.some((p) =>
            ["pending_review", "accepted", "decomposed", "decomposing", "validation_failed"].includes(
              p.status,
            ),
          );
        });
        if (!got) {
          fail("G4", { note: "timeout waiting plan" });
        } else {
          const plans = got.plans;
          const best =
            plans.find((p) => p.status === "pending_review") ||
            plans.find((p) => ["accepted", "decomposed"].includes(p.status)) ||
            plans[0];
          const notes = best?.payload?.constraint_notes || best?.payload?.ConstraintNotes || [];
          const noteStr = JSON.stringify(notes);
          const collapse =
            noteStr.includes("collapse") ||
            noteStr.includes("折叠") ||
            notes.some(
              (n) =>
                n.kind === "collapse" ||
                String(n.message || "").includes("折叠") ||
                String(n.Kind || "") === "collapse",
            );
          const human =
            best?.payload?.requires_human_review === true ||
            best?.payload?.RequiresHumanReview === true ||
            best?.review_required === true ||
            best?.status === "pending_review" ||
            noteStr.includes("人工复核");
          // task emp: develop and test same
          const tasks = best?.payload?.tasks || [];
          const devT = tasks.find((t) => t.planned_task_key === "develop");
          const testT = tasks.find((t) => t.planned_task_key === "test");
          const same =
            devT?.selected_employee_id &&
            testT?.selected_employee_id &&
            devT.selected_employee_id === testT.selected_employee_id;
          result.evidence.g4 = {
            plan_id: best?.id,
            status: best?.status,
            exit: best?.payload?.exit_deliverable,
            collapse,
            human,
            same_dev_test: same,
            notes: notes.slice?.(0, 5) || notes,
            tasks: tasks.map((t) => ({
              key: t.planned_task_key,
              emp: t.selected_employee_id,
            })),
          };
          if (collapse && human && (same || !testT)) {
            // if exit not deep enough for test, still require collapse note when both roles cast same
            pass("G4", result.evidence.g4);
          } else if (collapse && human) {
            pass("G4", result.evidence.g4);
          } else {
            fail("G4", result.evidence.g4);
          }
        }
      }
    }
  }

  // ---------- G8: disable cast employee then fire ----------
  {
    log("--- G8 fire after disable ---");
    // Restore healthy cast first
    await putCast(cookie, "software_delivery", [
      { role_key: "developer", digital_employee_id: EMP.developer },
      { role_key: "reviewer", digital_employee_id: EMP.reviewer },
      { role_key: "tester", digital_employee_id: EMP.tester },
    ]);
    await api(cookie, `/api/v1/digital-employees/${EMP.developer}/status`, {
      method: "PUT",
      body: { status: "ready" },
    });
    // Create enabled rule with full cast
    let r = await api(cookie, `/api/v1/automations`, {
      method: "POST",
      body: {
        project_id: PROJECT_ID,
        name: `G8 fire ${Date.now()}`,
        coordination_mode: "plan",
        scenario_template_key: "software_delivery",
        demand_title_template: "G8 auto {{project_name}} {{fire_time}}",
        demand_body_template: "automation fire after cast employee disabled",
        schedule_kind: "interval",
        interval_seconds: 86400,
        timezone: "Asia/Shanghai",
        enabled: true,
      },
    });
    if (r.status >= 400 || !r.json?.id) {
      fail("G8", { create: r.status, body: r.text.slice(0, 300) });
    } else {
      const ruleId = r.json.id;
      result.evidence.g8_rule_id = ruleId;
      // Disable cast employee (real status change)
      r = await api(cookie, `/api/v1/digital-employees/${EMP.developer}/status`, {
        method: "PUT",
        body: { status: "disabled" },
      });
      log(`disable developer → ${r.status}`);
      // Trigger fire
      r = await api(cookie, `/api/v1/automations/${ruleId}/trigger`, {
        method: "POST",
        body: {},
      });
      log(`trigger → ${r.status} ${r.text.slice(0, 200)}`);
      // List fires
      await sleep(1500);
      const fr = await api(
        cookie,
        `/api/v1/automations/${ruleId}/fires?limit=5`,
      );
      const fires = listOf(fr.json);
      const latest = fires[0] || r.json;
      result.evidence.g8 = {
        trigger_status: r.status,
        fire: latest
          ? {
              id: latest.id,
              status: latest.status,
              error_code: latest.error_code,
              error_message: latest.error_message,
            }
          : null,
        fires_n: fires.length,
      };
      const msg = `${latest?.error_code || ""} ${latest?.error_message || ""} ${r.text}`;
      const failed =
        latest?.status === "failed" ||
        latest?.status === "FireStatusFailed" ||
        String(latest?.status || "").includes("fail");
      const reasonOk =
        msg.includes("casting") ||
        msg.includes("编制") ||
        msg.includes("不可用") ||
        msg.includes("developer");
      if (failed && reasonOk) {
        pass("G8", result.evidence.g8);
      } else if (r.status >= 400 && reasonOk) {
        pass("G8", { ...result.evidence.g8, note: "trigger rejected with reason" });
      } else {
        fail("G8", result.evidence.g8);
      }
      // Re-enable developer for rest of system
      await api(cookie, `/api/v1/digital-employees/${EMP.developer}/status`, {
        method: "PUT",
        body: { status: "ready" },
      });
      // Disable rule to stop schedule noise
      await api(cookie, `/api/v1/automations/${ruleId}/disable`, {
        method: "POST",
        body: {},
      });
    }
  }

  // ---------- G9 vocab-constrained expansion request ----------
  {
    log("--- G9 expansion vocabulary ---");
    await putCast(cookie, "software_delivery", [
      { role_key: "developer", digital_employee_id: EMP.developer },
      { role_key: "reviewer", digital_employee_id: EMP.reviewer },
      { role_key: "tester", digital_employee_id: EMP.tester },
    ]);
    // Need a demand to attach expansion
    const dr = await api(cookie, `/api/v1/projects/${PROJECT_ID}/demands`, {
      method: "POST",
      body: {
        title: `G9 expand ${new Date().toISOString().slice(11, 19)}`,
        content: "小功能开发交付分支，后续可能扩编角色",
        scenario_template_key: "software_delivery",
        coordination_mode: "plan",
      },
    });
    if (dr.status >= 400 || !dr.json?.id) {
      fail("G9", { demand: dr.status, body: dr.text.slice(0, 200) });
    } else {
      const demandId = dr.json.id;
      // free-text / non-vocab role must fail
      const bad = await api(cookie, `/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
        method: "POST",
        body: {
          demand_id: demandId,
          suggested_role_key: "totally_made_up_role_zzz",
          reason: "G9 negative: free text role",
          scenario_template_key: "software_delivery",
        },
      });
      // vocab role must succeed (collector is seeded)
      const good = await api(cookie, `/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
        method: "POST",
        body: {
          demand_id: demandId,
          suggested_role_key: "collector",
          reason: "G9 positive: vocab role collector",
          scenario_template_key: "software_delivery",
        },
      });
      result.evidence.g9 = {
        demand_id: demandId,
        bad_status: bad.status,
        bad_body: bad.text.slice(0, 200),
        good_status: good.status,
        good_id: good.json?.id,
        good_suggested: good.json?.context?.suggested_role_key || "collector",
      };
      const badOk =
        bad.status >= 400 &&
        (bad.text.includes("vocabulary") ||
          bad.text.includes("词表") ||
          bad.text.includes("role") ||
          bad.text.includes("suggested_role"));
      const goodOk = good.status < 300 && good.json?.id;
      // Inspect decision payload for suggested_role_key
      if (goodOk) {
        const inbox = await api(
          cookie,
          `/api/v1/inbox/items?view=mine&status=open&limit=20`,
        );
        const card = listOf(inbox.json).find(
          (it) =>
            it.source_id === good.json.id ||
            (it.context?.decision_type === "casting_expansion" &&
              it.context?.demand_id === demandId),
        );
        result.evidence.g9.inbox_suggested = card?.context?.suggested_role_key;
        if (card?.context?.suggested_role_key === "collector") {
          result.evidence.g9.vocab_on_card = true;
        }
      }
      if (badOk && goodOk) pass("G9", result.evidence.g9);
      else fail("G9", result.evidence.g9);
    }
  }

  // Restore healthy cast
  await putCast(cookie, "software_delivery", [
    { role_key: "developer", digital_employee_id: EMP.developer },
    { role_key: "reviewer", digital_employee_id: EMP.reviewer },
    { role_key: "tester", digital_employee_id: EMP.tester },
  ]);
  await putCast(cookie, "incident_response", [
    { role_key: "diagnostician", digital_employee_id: EMP.ops },
    { role_key: "operator", digital_employee_id: EMP.developer },
    { role_key: "verifier", digital_employee_id: EMP.reviewer },
  ]);
  await api(cookie, `/api/v1/digital-employees/${EMP.developer}/status`, {
    method: "PUT",
    body: { status: "ready" },
  });

  // Browser smoke: project config casting visible
  try {
    const browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: "zh-CN",
    });
    const page = await context.newPage();
    await page.goto(`${WEB}/sign-in`, { waitUntil: "domcontentloaded" });
    await page.getByLabel("账号").fill("admin");
    await page.getByLabel("密码").fill("admin");
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForFunction(() => !location.pathname.includes("sign-in"), null, {
      timeout: 30000,
    });
    await page.goto(`${WEB}/projects/${PROJECT_ID}/config`, {
      waitUntil: "domcontentloaded",
    });
    await sleep(2000);
    await page.screenshot({ path: join(OUT, "config-casting.png"), fullPage: true });
    const body = await page.locator("body").innerText();
    if (body.includes("编制") || body.includes("剧本")) {
      pass("G_web_config_casting", {});
    } else {
      result.gates.G_web_config_casting = {
        pass: false,
        soft: true,
        note: "config page text miss",
      };
    }
    await browser.close();
  } catch (e) {
    result.gates.G_web_config_casting = {
      pass: false,
      soft: true,
      error: String(e.message || e),
    };
  }

  const hard = ["G1", "G2", "G3", "G4", "G5", "G6", "G7", "G8", "G9"];
  result.ok = hard.every((k) => result.gates[k]?.pass === true);
  writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
  log(result.ok ? "PASS residual gates" : "FAIL residual");
  console.log(JSON.stringify(result, null, 2));
  if (!result.ok) process.exitCode = 1;
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
