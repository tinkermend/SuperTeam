/**
 * Full chain: coordinator auto casting_expansion → human approve+pick employee
 * → mid-exec replan → new tasks for hire + completed task key reuse.
 *
 * Does NOT POST /casting-expansions. Expansion must come from MaybeRequest
 * after real task completion (G9 coordinator path).
 *
 *   node scripts/e2e/browser-casting-design-g9-to-g10-chain.mjs
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
const OUT = join(__dirname, "../../.scratch/e2e-casting-design-g9-g10-chain");
mkdirSync(OUT, { recursive: true });

const result = { ok: false, gates: {}, errors: [], evidence: {}, timeline: [] };
const log = (m) => {
  console.log(`[g9-g10-chain] ${m}`);
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
const listOf = (j) =>
  Array.isArray(j)
    ? j
    : j?.items || j?.demands || j?.tasks || j?.revisions || j?.events || [];

async function cpLogin(retries = 5) {
  let last = null;
  for (let i = 1; i <= retries; i++) {
    try {
      const res = await fetch(`${CP}/api/auth/login`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ username: "admin", password: "admin" }),
      });
      if (!res.ok) {
        last = `login ${res.status}`;
        await sleep(1500 * i);
        continue;
      }
      const setCookie = res.headers.getSetCookie?.() || [];
      const raw = res.headers.get("set-cookie") || "";
      const parts = setCookie.length ? setCookie : raw ? [raw] : [];
      return parts
        .map((c) => c.split(";")[0].trim())
        .filter(Boolean)
        .join("; ");
    } catch (e) {
      last = String(e);
      await sleep(1500 * i);
    }
  }
  throw new Error(last || "login failed");
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

async function waitUntil(pred, ms = WAIT, every = 4000) {
  const dl = Date.now() + ms;
  let last = null;
  while (Date.now() < dl) {
    last = await pred();
    if (last?.ok || last === true) return last === true ? { ok: true } : last;
    await sleep(every);
  }
  return { ok: false, last };
}

async function putCast(cookie, templateKey, assignments) {
  return api(cookie, `/api/v1/projects/${PROJECT_ID}/castings`, {
    method: "PUT",
    body: { scenario_template_key: templateKey, assignments },
  });
}

async function tasks(cookie, demandId) {
  const r = await api(cookie, `/api/v1/projects/${PROJECT_ID}/tasks?limit=80`);
  return listOf(r.json).filter((t) => t.demand_id === demandId);
}
async function plans(cookie, demandId) {
  const r = await api(
    cookie,
    `/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=40`,
  );
  return listOf(r.json).filter((p) => p.demand_id === demandId);
}

async function approvePlanReview(cookie, demandId, planId) {
  const r = await api(
    cookie,
    `/api/v1/inbox/items?view=mine&status=open&limit=50`,
  );
  const card = listOf(r.json).find(
    (it) =>
      (it.kind === "plan_review" ||
        it.context?.decision_type === "plan_review") &&
      (it.context?.plan_revision_id === planId ||
        it.context?.demand_id === demandId),
  );
  if (!card) return { status: 0, note: "no card" };
  return api(cookie, `/api/v1/inbox/items/${card.id}/actions`, {
    method: "POST",
    body: { action: "approved", comment: "chain accept plan", payload: {} },
  });
}

async function approveExpansion(cookie, inboxId, roleKey, empId) {
  return api(cookie, `/api/v1/inbox/items/${inboxId}/actions`, {
    method: "POST",
    body: {
      action: "approved",
      comment: `chain approve expansion ${roleKey}`,
      payload: { role_key: roleKey, digital_employee_id: empId },
    },
  });
}

async function main() {
  const cookie = await cpLogin();
  log("cp login ok");

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

  // developer-only cast
  let r = await putCast(cookie, "software_delivery", [
    { role_key: "developer", digital_employee_id: EMP.developer },
  ]);
  if (r.status >= 400) throw new Error(`cast ${r.text}`);
  log("cast developer-only");

  r = await api(cookie, `/api/v1/projects/${PROJECT_ID}/demands`, {
    method: "POST",
    body: {
      title: `G9→G10 chain ${new Date().toISOString().slice(11, 19)}`,
      content:
        "仅开发编制：完成开发后协调应自动提请扩编审查；人批准后重规划派审查任务。",
      scenario_template_key: "software_delivery",
      coordination_mode: "plan",
    },
  });
  if (r.status >= 400 || !r.json?.id) throw new Error(`demand ${r.text}`);
  const demandId = r.json.id;
  result.evidence.demand_id = demandId;
  pass("chain_demand", { demand_id: demandId });
  log(`demand ${demandId}`);

  // plan + accept (keep polling until tasks exist)
  const planned = await waitUntil(async () => {
    const p = await plans(cookie, demandId);
    const t = await tasks(cookie, demandId);
    const pend = p.find((x) => x.status === "pending_review");
    if (pend) {
      const ar = await approvePlanReview(cookie, demandId, pend.id);
      if (ar?.status) log(`accept plan_review → ${ar.status}`);
    }
    const hasDevelop = t.some(
      (x) =>
        x.planned_task_key === "develop" ||
        x.planned_task_key === "develop_task" ||
        /开发|develop/i.test(x.title || ""),
    );
    return {
      ok:
        hasDevelop ||
        t.length > 0 ||
        p.some((x) =>
          ["decomposed", "accepted", "decomposing"].includes(x.status),
        ),
      plans: p,
      tasks: t,
    };
  });
  if (!planned.ok) {
    fail("chain_plan", { note: "plan/task timeout" });
    throw new Error("plan timeout");
  }
  pass("chain_plan", {
    tasks: (await tasks(cookie, demandId)).map((t) => ({
      key: t.planned_task_key,
      status: t.status,
    })),
  });

  const beforeTasks = await tasks(cookie, demandId);
  const develop = beforeTasks.find(
    (t) =>
      t.planned_task_key === "develop" ||
      t.planned_task_key === "develop_task" ||
      /开发|develop/i.test(t.title || ""),
  );
  if (!develop) {
    fail("chain_develop_seed", { tasks: beforeTasks });
    throw new Error("no develop");
  }
  const developId = develop.id;
  result.evidence.develop_before = {
    id: developId,
    status: develop.status,
    key: develop.planned_task_key,
  };
  pass("chain_develop_seed", result.evidence.develop_before);

  const planIdsBefore = new Set((await plans(cookie, demandId)).map((p) => p.id));

  // Wait runtime complete develop (no forge POST expansion)
  // Provider may take several minutes; wait full planner budget.
  const completed = await waitUntil(async () => {
    const t = (await tasks(cookie, demandId)).find((x) => x.id === developId);
    return { ok: t?.status === "completed", status: t?.status };
  }, Math.max(WAIT, 600000), 8000);
  result.evidence.develop_complete = completed;
  if (!completed.ok) {
    fail("chain_develop_complete", completed);
    throw new Error("develop not completed by runtime");
  }
  pass("chain_develop_complete", { id: developId });
  log("develop completed");

  // Wait coordinator open casting_expansion (must NOT be seeded by us)
  const expansion = await waitUntil(async () => {
    const inbox = await api(
      cookie,
      `/api/v1/inbox/items?view=mine&status=open&limit=40`,
    );
    const card = listOf(inbox.json).find(
      (it) =>
        (it.kind === "casting_expansion" ||
          it.context?.decision_type === "casting_expansion") &&
        it.context?.demand_id === demandId,
    );
    return {
      ok: Boolean(card?.context?.suggested_role_key),
      card,
      suggested: card?.context?.suggested_role_key,
      summary: card?.summary || card?.context?.reason,
    };
  }, 180000);
  result.evidence.coordinator_expansion = {
    suggested: expansion.suggested,
    card_id: expansion.card?.id,
    summary: expansion.summary,
  };
  if (
    !expansion.ok ||
    !expansion.suggested ||
    expansion.suggested === "totally_made_up_role_zzz"
  ) {
    fail("chain_g9_coordinator", result.evidence.coordinator_expansion);
    throw new Error("coordinator did not open casting_expansion");
  }
  pass("chain_g9_coordinator", result.evidence.coordinator_expansion);
  log(`coordinator expansion suggested=${expansion.suggested}`);

  // Human approve + pick reviewer (or suggested role employee)
  const roleKey = expansion.suggested;
  const empId =
    roleKey === "tester"
      ? EMP.tester
      : roleKey === "reviewer"
        ? EMP.reviewer
        : EMP.reviewer;
  const ar = await approveExpansion(
    cookie,
    expansion.card.id,
    roleKey,
    empId,
  );
  log(`approve expansion → ${ar.status}`);
  if (ar.status >= 400) {
    fail("chain_approve_expansion", { status: ar.status, body: ar.text.slice(0, 200) });
    throw new Error("approve failed");
  }
  pass("chain_approve_expansion", { role: roleKey, emp: empId });

  // casting written
  const cast = await api(
    cookie,
    `/api/v1/projects/${PROJECT_ID}/castings?template_key=software_delivery`,
  );
  const castRows = listOf(cast.json);
  const hasHire = castRows.some(
    (c) => c.role_key === roleKey && c.digital_employee_id === empId,
  );
  result.evidence.casting_after = castRows.map((c) => ({
    role: c.role_key,
    emp: c.digital_employee_id,
  }));
  if (!hasHire) {
    fail("chain_casting_written", result.evidence.casting_after);
  } else {
    pass("chain_casting_written", { role: roleKey, emp: empId });
  }

  // replan
  const replan = await waitUntil(async () => {
    const p = await plans(cookie, demandId);
    const neu = p.filter((x) => !planIdsBefore.has(x.id));
    log(`replan new=${neu.map((x) => x.status + ":" + (x.payload?.exit_deliverable || "")).join(",")}`);
    if (!neu.length) return { ok: false };
    const pending = neu.find((x) => x.status === "pending_review");
    if (pending) {
      await approvePlanReview(cookie, demandId, pending.id);
    }
    const progressed = neu.some((x) =>
      ["accepted", "decomposed", "decomposing", "pending_review"].includes(
        x.status,
      ),
    );
    return { ok: progressed, neu };
  }, WAIT);
  result.evidence.replan = replan.neu?.map((p) => ({
    id: p.id,
    status: p.status,
    exit: p.payload?.exit_deliverable,
  }));
  if (!replan.ok) {
    fail("chain_replan", { note: "no replan" });
  } else {
    pass("chain_replan", result.evidence.replan);
  }

  // wait tasks after replan decompose
  const after = await waitUntil(async () => {
    const t = await tasks(cookie, demandId);
    const hireTasks = t.filter(
      (x) => x.assigned_digital_employee_id === empId && x.id !== developId,
    );
    const developSame = t.find((x) => x.id === developId);
    return {
      ok: hireTasks.length > 0 && developSame?.id === developId,
      hireTasks,
      develop: developSame,
      all: t.map((x) => ({
        id: x.id,
        key: x.planned_task_key,
        status: x.status,
        emp: x.assigned_digital_employee_id,
      })),
    };
  }, WAIT);
  result.evidence.tasks_after = after.all;
  result.evidence.hire_tasks = after.hireTasks?.map((t) => ({
    id: t.id,
    key: t.planned_task_key,
    emp: t.assigned_digital_employee_id,
  }));
  result.evidence.develop_reuse = {
    same_id: after.develop?.id === developId,
    id: after.develop?.id,
    status: after.develop?.status,
  };

  if (after.ok && after.hireTasks?.length) {
    pass("chain_g10_new_task_to_hire", result.evidence.hire_tasks);
  } else {
    fail("chain_g10_new_task_to_hire", {
      hire: result.evidence.hire_tasks,
      all: after.all,
    });
  }
  if (result.evidence.develop_reuse.same_id) {
    pass("chain_g10_develop_id_reuse", result.evidence.develop_reuse);
  } else {
    fail("chain_g10_develop_id_reuse", result.evidence.develop_reuse);
  }

  // browser observe (best-effort)
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
    await page.waitForTimeout(2000);
    await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(1500);
    await page.screenshot({ path: join(OUT, "inbox.png"), fullPage: true });
    await browser.close();
    pass("chain_browser", {});
  } catch (e) {
    result.evidence.browser_error = String(e).slice(0, 200);
    // non-fatal for chain hard gates
  }

  // restore full cast
  await putCast(cookie, "software_delivery", [
    { role_key: "developer", digital_employee_id: EMP.developer },
    { role_key: "reviewer", digital_employee_id: EMP.reviewer },
    { role_key: "tester", digital_employee_id: EMP.tester },
  ]);

  const hard = Object.entries(result.gates).filter(
    ([k, v]) =>
      !k.startsWith("chain_browser") && v && v.pass === false,
  );
  result.ok = hard.length === 0 && result.errors.filter((e) => !e.startsWith("chain_browser")).length === 0;
  // recompute: all required gates pass
  const required = [
    "chain_demand",
    "chain_plan",
    "chain_develop_seed",
    "chain_develop_complete",
    "chain_g9_coordinator",
    "chain_approve_expansion",
    "chain_casting_written",
    "chain_replan",
    "chain_g10_new_task_to_hire",
    "chain_g10_develop_id_reuse",
  ];
  result.ok = required.every((k) => result.gates[k]?.pass);
  writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
  log(`done ok=${result.ok} gates=${JSON.stringify(Object.fromEntries(Object.entries(result.gates).map(([k,v])=>[k,v.pass])))}`);
  if (!result.ok) process.exitCode = 1;
}

main().catch((e) => {
  console.error(e);
  result.errors.push(String(e));
  writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
  process.exit(1);
});
