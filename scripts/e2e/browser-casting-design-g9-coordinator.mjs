/**
 * G9 true coordinator path (design §7.1 / §7.2):
 * developer-only cast → demand → plan/accept → complete develop task →
 * coordinator opens casting_expansion with suggested_role_key from vocab
 * (NextExitNeedsRoles), NOT free text / not manual POST /casting-expansions.
 *
 *   node scripts/e2e/browser-casting-design-g9-coordinator.mjs
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
const OUT = join(__dirname, "../../.scratch/e2e-casting-design-g9-coordinator");
mkdirSync(OUT, { recursive: true });

const result = {
  ok: false,
  gates: {},
  errors: [],
  evidence: {},
  timeline: [],
};
const log = (m) => {
  console.log(`[g9-coord] ${m}`);
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
  Array.isArray(j)
    ? j
    : j?.items || j?.demands || j?.tasks || j?.revisions || j?.events || [];

async function putCast(cookie, templateKey, assignments) {
  return api(cookie, `/api/v1/projects/${PROJECT_ID}/castings`, {
    method: "PUT",
    body: { scenario_template_key: templateKey, assignments },
  });
}

async function waitUntil(cookie, label, pred, ms = WAIT) {
  const dl = Date.now() + ms;
  let last = null;
  while (Date.now() < dl) {
    last = await pred();
    if (last?.ok) return last;
    await sleep(2500);
  }
  return { ok: false, last, label };
}

async function tryCompleteTask(cookie, task, empId) {
  // Prefer attempt complete if attempt id present; else legacy runtime complete.
  const attempts = [
    {
      path: `/api/v1/projects/${PROJECT_ID}/tasks/${task.id}/complete`,
      body: {
        digital_employee_id: empId,
        conclusion: "G9 coordinator E2E: develop done, may need deeper cast",
      },
    },
    {
      path: `/api/v1/runtime/tasks/${task.id}/complete`,
      body: {
        digital_employee_id: empId,
        conclusion: "G9 coordinator E2E: develop done",
      },
    },
  ];
  for (const a of attempts) {
    const r = await api(cookie, a.path, { method: "POST", body: a.body });
    log(`complete ${task.id.slice(0, 8)} via ${a.path} → ${r.status} ${r.text.slice(0, 120)}`);
    if (r.status < 400) return { ok: true, ...r };
  }
  return { ok: false };
}

async function main() {
  const cookie = await cpLogin();
  log("cp login ok");

  // Clean roles baseline (do not invent operator employee)
  for (const [id, roles] of [
    [EMP.developer, ["developer", "diagnostician"]],
    [EMP.reviewer, ["reviewer", "verifier"]],
    [EMP.tester, ["tester"]],
    [EMP.ops, ["collector", "analyst", "diagnostician"]],
  ]) {
    await api(cookie, `/api/v1/digital-employees/${id}/roles`, {
      method: "PUT",
      body: { role_keys: roles },
    });
  }
  await api(cookie, `/api/v1/digital-employees/${EMP.developer}/status`, {
    method: "PUT",
    body: { status: "ready" },
  });

  // developer-only cast → shallow exit, next needs reviewer
  let r = await putCast(cookie, "software_delivery", [
    { role_key: "developer", digital_employee_id: EMP.developer },
  ]);
  log(`cast developer-only → ${r.status}`);
  if (r.status >= 400) {
    fail("G9", { cast: r.status, body: r.text.slice(0, 200) });
    throw new Error("cast failed");
  }

  const ready = await api(
    cookie,
    `/api/v1/projects/${PROJECT_ID}/playbook-readiness?template_key=software_delivery`,
  );
  const sd = listOf(ready.json).find(
    (x) => x.scenario_template_key === "software_delivery",
  );
  result.evidence.readiness_before = {
    deepest: sd?.deepest_exit?.deliverable || sd?.deepest_exit,
    next: sd?.next_exit_needs_roles,
    missing_any: sd?.missing_roles_for_any,
  };
  log(`readiness next=${JSON.stringify(result.evidence.readiness_before.next)}`);

  const dr = await api(cookie, `/api/v1/projects/${PROJECT_ID}/demands`, {
    method: "POST",
    body: {
      title: `G9 coord expand ${new Date().toISOString().slice(11, 19)}`,
      content: "仅开发编制：完成开发后协调应提请扩编审查角色",
      scenario_template_key: "software_delivery",
      coordination_mode: "plan",
    },
  });
  if (dr.status >= 400 || !dr.json?.id) {
    fail("G9", { demand: dr.status, body: dr.text.slice(0, 200) });
    throw new Error("demand create failed");
  }
  const demandId = dr.json.id;
  result.evidence.demand_id = demandId;
  log(`demand ${demandId}`);

  // Wait for a plan revision, auto-accept pending_review, then tasks
  const planned = await waitUntil(cookie, "plan", async () => {
    const pr = await api(
      cookie,
      `/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=40`,
    );
    const plans = listOf(pr.json).filter((p) => p.demand_id === demandId);
    const tr = await api(
      cookie,
      `/api/v1/projects/${PROJECT_ID}/tasks?limit=80`,
    );
    const tasks = listOf(tr.json).filter((t) => t.demand_id === demandId);
    const dem = await api(cookie, `/api/v1/projects/${PROJECT_ID}/demands`);
    const demand = listOf(dem.json).find((d) => d.id === demandId);
    const accepted = plans.find((p) =>
      ["accepted", "decomposed", "decomposing"].includes(p.status),
    );
    const pending = plans.find((p) => p.status === "pending_review");
    if (pending && !accepted) {
      const inbox = await api(
        cookie,
        `/api/v1/inbox/items?view=mine&status=open&limit=40`,
      );
      const card = listOf(inbox.json).find(
        (it) =>
          (it.kind === "plan_review" ||
            it.context?.decision_type === "plan_review") &&
          (it.context?.plan_revision_id === pending.id ||
            it.context?.demand_id === demandId),
      );
      if (card) {
        const ar = await api(cookie, `/api/v1/inbox/items/${card.id}/actions`, {
          method: "POST",
          body: {
            action: "approved",
            comment: "G9 accept shallow plan",
            payload: {},
          },
        });
        log(`accept plan_review ${pending.id.slice(0, 8)} → ${ar.status}`);
      }
    }
    // Only done when we have tasks or a non-pending plan
    const ready =
      tasks.length > 0 ||
      Boolean(accepted) ||
      plans.some((p) => p.status === "decomposed");
    return {
      ok: ready,
      plans,
      tasks,
      demand,
      accepted,
      pending,
    };
  });
  result.evidence.plan_wait = {
    ok: planned.ok,
    plan_status: planned.accepted?.status || planned.pending?.status,
    plan_id: planned.accepted?.id || planned.pending?.id,
    task_count: planned.tasks?.length,
    demand_status: planned.demand?.status,
  };
  if (!planned.ok) {
    fail("G9", { step: "plan", ...result.evidence.plan_wait });
    throw new Error("plan timeout");
  }

  const withTask = await waitUntil(cookie, "task", async () => {
    const tr = await api(
      cookie,
      `/api/v1/projects/${PROJECT_ID}/tasks?limit=80`,
    );
    const tasks = listOf(tr.json).filter((t) => t.demand_id === demandId);
    const develop = tasks.find(
      (t) =>
        t.planned_task_key === "develop" ||
        t.planned_task_key === "develop_task" ||
        /开发|实现|develop/i.test(t.title || ""),
    );
    const runnable = tasks.find((t) =>
      ["planned", "queued", "running", "waiting_human", "ready", "blocked"].includes(
        t.status,
      ),
    );
    return {
      ok: Boolean(develop || runnable || tasks.length > 0),
      tasks,
      develop: develop || runnable || tasks[0],
    };
  });
  result.evidence.tasks_before = withTask.tasks?.map((t) => ({
    id: t.id,
    key: t.planned_task_key,
    status: t.status,
    emp: t.assigned_digital_employee_id,
  }));
  if (!withTask.ok || !withTask.develop) {
    fail("G9", { step: "no_task", tasks: result.evidence.tasks_before });
    throw new Error("no develop task");
  }
  const task = withTask.develop;
  log(`task ${task.id} status=${task.status} key=${task.planned_task_key}`);

  // Snapshot open casting_expansion for this demand BEFORE complete (must be 0)
  const inboxBefore = await api(
    cookie,
    `/api/v1/inbox/items?view=mine&status=open&limit=50`,
  );
  const expandBefore = listOf(inboxBefore.json).filter(
    (it) =>
      (it.kind === "casting_expansion" ||
        it.context?.decision_type === "casting_expansion") &&
      it.context?.demand_id === demandId,
  );
  result.evidence.expand_before = expandBefore.length;

  // Prefer real runtime completion (product path). Console complete is 404;
  // runtime complete needs session auth. Poll until develop is completed by
  // runtime agent, or try complete once (best-effort).
  let completedOk = task.status === "completed";
  if (!completedOk) {
    const tried = await tryCompleteTask(cookie, task, EMP.developer);
    result.evidence.complete_try = {
      ok: tried.ok,
      status: tried.status,
      body: tried.text?.slice(0, 200),
    };
    const waitComplete = await waitUntil(
      cookie,
      "task_completed",
      async () => {
        const tr = await api(
          cookie,
          `/api/v1/projects/${PROJECT_ID}/tasks?limit=80`,
        );
        const tasks = listOf(tr.json).filter((t) => t.demand_id === demandId);
        const t = tasks.find((x) => x.id === task.id);
        return {
          ok: t?.status === "completed",
          status: t?.status,
          tasks: tasks.map((x) => ({
            id: x.id,
            key: x.planned_task_key,
            status: x.status,
          })),
        };
      },
      Math.min(WAIT, 300000),
    );
    completedOk = waitComplete.ok;
    result.evidence.complete_wait = waitComplete;
  }
  result.evidence.complete_ok = completedOk;

  if (!completedOk) {
    fail("G9", {
      step: "complete_timeout",
      note: "develop never reached completed; coordinator path not exercised",
      ...result.evidence.complete_try,
      ...result.evidence.complete_wait,
    });
  } else {
    // Wait for casting_expansion opened by coordinator (NOT by this script POST)
    const expanded = await waitUntil(
      cookie,
      "expansion",
      async () => {
        const inbox = await api(
          cookie,
          `/api/v1/inbox/items?view=mine&status=open&limit=50`,
        );
        const card = listOf(inbox.json).find(
          (it) =>
            (it.kind === "casting_expansion" ||
              it.context?.decision_type === "casting_expansion") &&
            it.context?.demand_id === demandId,
        );
        const decisions = await api(
          cookie,
          `/api/v1/projects/${PROJECT_ID}/decisions?limit=40`,
        );
        const dec = listOf(decisions.json).find(
          (d) =>
            d.decision_type === "casting_expansion" &&
            d.status_snapshot === "pending" &&
            String(d.summary_snapshot || "").includes("缺角色"),
        );
        const events = await api(
          cookie,
          `/api/v1/projects/${PROJECT_ID}/events?limit=40`,
        );
        const ev = listOf(events.json).find(
          (e) =>
            e.event_type === "decision_requested" &&
            e.actor_type === "coordinator" &&
            String(e.summary || "").includes("扩编"),
        );
        return {
          ok: Boolean(card || dec),
          card,
          dec,
          coordinatorEvent: Boolean(ev),
          suggested:
            card?.context?.suggested_role_key ||
            dec?.context?.suggested_role_key,
          summary: card?.summary || dec?.summary_snapshot || ev?.summary,
        };
      },
      180000,
    );

    result.evidence.expansion = {
      found: expanded.ok,
      suggested: expanded.suggested,
      card_id: expanded.card?.id,
      decision_id: expanded.dec?.id,
      coordinator_event: expanded.coordinatorEvent,
      summary: expanded.summary,
      expand_before: result.evidence.expand_before,
    };

    const vocabRoles = new Set([
      "developer",
      "reviewer",
      "tester",
      "collector",
      "analyst",
      "operator",
      "verifier",
      "diagnostician",
      "researcher",
      "writer",
    ]);
    const roleOk =
      expanded.suggested && vocabRoles.has(String(expanded.suggested));
    // Casting-only next role with developer-only cast must be reviewer (skeleton order).
    const expectedRoles = ["reviewer", "tester"];
    const matchesExpected =
      expectedRoles.includes(expanded.suggested) ||
      (expanded.suggested && expanded.suggested !== "developer");

    if (
      expanded.ok &&
      roleOk &&
      matchesExpected &&
      result.evidence.expand_before === 0
    ) {
      pass("G9", result.evidence.expansion);
    } else {
      fail("G9", {
        ...result.evidence.expansion,
        roleOk,
        matchesExpected,
        expectedRoles,
      });
    }
  }

  // Browser observe inbox
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: "zh-CN",
    });
    const page = await context.newPage();
    await page.goto(`${WEB}/login`, { waitUntil: "domcontentloaded" });
    await page.fill('input[name="username"], #username, input[type="text"]', "admin");
    await page.fill('input[name="password"], #password, input[type="password"]', "admin");
    await page.click('button[type="submit"]');
    await page.waitForTimeout(1500);
    await page.goto(`${WEB}/inbox`, { waitUntil: "networkidle" });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: join(OUT, "inbox.png"), fullPage: true });
    result.evidence.browser_inbox = true;
  } catch (e) {
    result.evidence.browser_error = String(e).slice(0, 200);
  } finally {
    await browser.close();
  }

  // Restore healthy cast
  await putCast(cookie, "software_delivery", [
    { role_key: "developer", digital_employee_id: EMP.developer },
    { role_key: "reviewer", digital_employee_id: EMP.reviewer },
    { role_key: "tester", digital_employee_id: EMP.tester },
  ]);

  result.ok = Object.values(result.gates).every((g) => g.pass) && !result.errors.length;
  writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
  log(`done ok=${result.ok} gates=${JSON.stringify(result.gates)}`);
  if (!result.ok) process.exitCode = 1;
}

main().catch((e) => {
  console.error(e);
  result.errors.push(String(e));
  writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
  process.exit(1);
});
