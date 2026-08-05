/**
 * @deprecated Prefer scripts/e2e/browser-casting-design-g10-g11-g12.mjs
 * Early G10+G12; G12 software_delivery path is migratable adversarial_review.
 *
 * Real G10 + G12 for casting expansion (design §7 / gates).
 *
 * G10: demand already has completed planned_task_key → expand → replan →
 *      completed keys must not reappear as new open tasks on THIS demand.
 * G12: expand by casting original developer into reviewer (role_independence) →
 *      replan must not freely run with shared employee (blocked / gap / overbound).
 *
 *   node scripts/e2e/browser-casting-g10-g12.mjs
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
// Prefer demand that already has completed work (override via env)
const DEMAND_ID = process.env.SUPERTEAM_DEMAND_ID || "";
const EMP = {
  developer: "0be393bb-9dfd-48c8-b010-4b5abb114f23",
  reviewer: "7a16f593-9a99-490e-bcab-77bb8b326afa",
  tester: "157b1a2c-b2af-4a08-99f3-f16abe291ed1",
  ops: "9a623b40-c9ec-4d7d-99a4-17b1f569b52e",
};
const WAIT_MS = Number(process.env.SUPERTEAM_PLANNER_WAIT_MS || 300000);
const OUT = join(__dirname, "../../.scratch/e2e-browser-casting-g10-g12");
mkdirSync(OUT, { recursive: true });

const result = { ok: false, gates: {}, errors: [], evidence: {}, timeline: [] };
function log(m) {
  console.log(`[g10g12] ${m}`);
  result.timeline.push({ t: new Date().toISOString(), m });
}
function pass(k, d = {}) {
  result.gates[k] = { pass: true, ...d };
}
function fail(k, d = {}) {
  result.gates[k] = { pass: false, ...d };
  result.errors.push(`${k}: ${JSON.stringify(d)}`);
}
function skip(k, reason, d = {}) {
  result.gates[k] = { pass: null, skipped: true, reason, ...d };
}
async function sleep(ms) {
  await new Promise((r) => setTimeout(r, ms));
}
async function shot(page, name) {
  await page.screenshot({ path: join(OUT, `${name}.png`), fullPage: true });
}
async function api(path, { method = "GET", body, cookieHeader } = {}) {
  const res = await fetch(`${CP}${path}`, {
    method,
    headers: {
      "content-type": "application/json",
      accept: "application/json",
      ...(cookieHeader ? { cookie: cookieHeader } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    /* ignore */
  }
  return { status: res.status, text, json };
}
function listOf(json) {
  if (Array.isArray(json)) return json;
  return json?.items || json?.tasks || json?.demands || [];
}

async function loginBrowser(page) {
  await page.goto(`${WEB}/sign-in`, { waitUntil: "domcontentloaded" });
  await page.getByLabel("账号").fill("admin");
  await page.getByLabel("密码").fill("admin");
  await page.getByRole("button", { name: "登录" }).click();
  await page.waitForFunction(
    () => !location.pathname.includes("sign-in") && !location.pathname.endsWith("/login"),
    null,
    { timeout: 30000 },
  );
}

async function browserApproveExpansion(page, roleKey, preferName) {
  await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
  // page may not have 收件箱 label in nav; wait for list
  await page.waitForTimeout(1500);
  const card = page.getByText("扩编请求").first();
  await card.waitFor({ state: "visible", timeout: 20000 });
  await card.click();
  await page.getByRole("button", { name: /批准并选人|同意/ }).first().click();
  await page.locator("#casting-expansion-employee").waitFor({ state: "visible", timeout: 15000 });
  await page.locator("#casting-expansion-employee").click();
  const opt = page.getByRole("option").filter({ hasText: preferName }).first();
  if (await opt.count()) await opt.click();
  else await page.getByRole("option").filter({ hasNotText: /暂无/ }).first().click();
  await page.getByRole("button", { name: "提交" }).click({ force: true });
  await page
    .getByText(/决策已提交|操作已提交/)
    .first()
    .waitFor({ state: "visible", timeout: 25000 })
    .catch(() => null);
  await sleep(1500);
}

async function pickDemandWithCompleted(cookieHeader) {
  if (DEMAND_ID) {
    const dr = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookieHeader });
    const d = listOf(dr.json).find((x) => x.id === DEMAND_ID);
    if (!d) throw new Error(`env demand ${DEMAND_ID} not found`);
    return d;
  }
  const tr = await api(`/api/v1/projects/${PROJECT_ID}/tasks?limit=50`, { cookieHeader });
  const tasks = listOf(tr.json);
  const completedByDemand = new Map();
  for (const t of tasks) {
    const did = t.demand_id || t.project_demand_id;
    if (!did) continue;
    if (!["completed", "done", "success"].includes(t.status)) continue;
    if (!t.planned_task_key) continue;
    const set = completedByDemand.get(did) || new Set();
    set.add(t.planned_task_key);
    completedByDemand.set(did, set);
  }
  const dr = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookieHeader });
  const demands = listOf(dr.json);
  // Prefer executing with completed keys
  for (const d of demands) {
    if (d.status !== "executing") continue;
    const keys = completedByDemand.get(d.id);
    if (keys && keys.size > 0) return { ...d, _completedKeys: [...keys] };
  }
  for (const d of demands) {
    const keys = completedByDemand.get(d.id);
    if (keys && keys.size > 0) return { ...d, _completedKeys: [...keys] };
  }
  return null;
}

async function tasksForDemand(cookieHeader, demandId) {
  const tr = await api(`/api/v1/projects/${PROJECT_ID}/tasks?limit=50`, { cookieHeader });
  return listOf(tr.json).filter(
    (t) => (t.demand_id || t.project_demand_id) === demandId,
  );
}

async function waitForReplan(cookieHeader, demandId, planIdsBefore, sinceMs) {
  const deadline = Date.now() + WAIT_MS;
  while (Date.now() < deadline) {
    const pr = await api(`/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`, {
      cookieHeader,
    });
    const plans = listOf(pr.json).filter((p) => p.demand_id === demandId);
    const newer = plans.filter((p) => !planIdsBefore.has(p.id));
    const er = await api(`/api/v1/projects/${PROJECT_ID}/events?limit=30`, { cookieHeader });
    const events = listOf(er.json).filter((e) => {
      const t = Date.parse(e.created_at || "");
      return !Number.isFinite(t) || t >= sinceMs - 3000;
    });
    const job = events.find(
      (e) =>
        (e.event_type || e.type) === "coordination_job.created" &&
        String(e.payload?.job_type || "").includes("casting_expansion"),
    );
    const fail = events.find(
      (e) =>
        String(e.summary || "").toLowerCase().includes("role_independence") ||
        String(e.summary || "").includes("职责") ||
        String(e.event_type || "").includes("planning_failed") ||
        String(e.summary || "").includes("constraint"),
    );
    log(
      `replan wait newer=${newer.map((p) => p.status).join(",")} job=${Boolean(job)} failEvt=${fail?.summary?.slice(0, 60) || ""}`,
    );
    if (newer.some((p) => ["accepted", "decomposed", "pending_review", "decomposing"].includes(p.status))) {
      return { plans: newer, job: Boolean(job), fail };
    }
    if (fail && job) return { plans: newer, job: true, fail };
    await sleep(8000);
  }
  return null;
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(30000);

  try {
    await loginBrowser(page);
    log(`logged in ${page.url()}`);
    await shot(page, "00-home");
    const cookieHeader = (await context.cookies()).map((c) => `${c.name}=${c.value}`).join("; ");

    // Ensure ops has collector+developer for candidates / G12
    await api(`/api/v1/digital-employees/${EMP.ops}/roles`, {
      method: "PUT",
      body: { role_keys: ["collector", "analyst", "developer"] },
      cookieHeader,
    });
    await api(`/api/v1/digital-employees/${EMP.developer}/roles`, {
      method: "PUT",
      body: { role_keys: ["developer"] },
      cookieHeader,
    });

    // ---------- G10 seed ----------
    let demand = await pickDemandWithCompleted(cookieHeader);
    if (!demand) {
      fail("g10_seed", { note: "no demand with completed planned_task_key" });
      throw new Error("G10 seed missing — need completed task on a demand");
    }
    const demandId = demand.id;
    let tasksBefore = await tasksForDemand(cookieHeader, demandId);
    const completedKeysBefore = new Set(
      tasksBefore
        .filter((t) => ["completed", "done", "success"].includes(t.status) && t.planned_task_key)
        .map((t) => t.planned_task_key),
    );
    if (completedKeysBefore.size === 0) {
      fail("g10_seed", { demandId, tasks: tasksBefore.map((t) => t.status) });
      throw new Error("no completed keys");
    }
    pass("g10_seed", {
      demand_id: demandId,
      demand_status: demand.status,
      completed_keys: [...completedKeysBefore],
    });
    result.evidence.g10_demand_id = demandId;
    result.evidence.completed_keys_before = [...completedKeysBefore];
    result.evidence.tasks_before = tasksBefore.map((t) => ({
      id: t.id,
      status: t.status,
      key: t.planned_task_key,
      emp: t.assigned_digital_employee_id || t.digital_employee_id,
    }));

    // Snapshot plans
    let pr = await api(`/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`, {
      cookieHeader,
    });
    const planIdsBefore = new Set(
      listOf(pr.json).filter((p) => p.demand_id === demandId).map((p) => p.id),
    );

    // Baseline casting: developer/reviewer/tester distinct (no SoD yet)
    await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      body: {
        scenario_template_key: "software_delivery",
        assignments: [
          { role_key: "developer", digital_employee_id: EMP.developer },
          { role_key: "reviewer", digital_employee_id: EMP.reviewer },
          { role_key: "tester", digital_employee_id: EMP.tester },
        ],
      },
      cookieHeader,
    });

    // G10 expand: add collector (ops) — new role not required by shallow plan, but replan runs
    let r = await api(`/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
      method: "POST",
      body: {
        demand_id: demandId,
        suggested_role_key: "collector",
        reason: "G10: 扩编 collector，验证已完成 develop 不被重复创建",
        scenario_template_key: "software_delivery",
      },
      cookieHeader,
    });
    log(`G10 open expansion → ${r.status}`);
    if (r.status >= 400) {
      fail("g10_open", { status: r.status, body: r.text.slice(0, 200) });
      throw new Error("g10 open expansion failed");
    }
    const g10DecisionId = r.json.id;
    pass("g10_open", { decision_id: g10DecisionId });

    const expandAt = Date.now();
    await browserApproveExpansion(page, "collector", /运维/);
    await shot(page, "01-g10-approved");
    pass("g10_browser_approve", {});

    // casting has collector
    r = await api(
      `/api/v1/projects/${PROJECT_ID}/castings?template_key=software_delivery`,
      { cookieHeader },
    );
    const cast = listOf(r.json).find((c) => c.role_key === "collector");
    if (!cast?.digital_employee_id) fail("g10_casting", { castings: r.json });
    else pass("g10_casting", { employee_id: cast.digital_employee_id });

    const replan = await waitForReplan(cookieHeader, demandId, planIdsBefore, expandAt);
    if (!replan) {
      fail("g10_replan", { note: "timeout waiting for replan plan" });
    } else {
      pass("g10_replan", {
        plans: replan.plans.map((p) => ({ id: p.id, status: p.status })),
      });
    }

    // If pending_review, approve via API (same product path as inbox)
    if (replan?.plans?.some((p) => p.status === "pending_review")) {
      const pending = replan.plans.find((p) => p.status === "pending_review");
      const inbox = await api(`/api/v1/inbox/items?view=mine&status=open&limit=30`, {
        cookieHeader,
      });
      const card = listOf(inbox.json).find(
        (it) =>
          it.context?.plan_revision_id === pending.id ||
          (it.context?.decision_type === "plan_review" &&
            it.context?.demand_id === demandId),
      );
      if (card) {
        r = await api(`/api/v1/inbox/items/${card.id}/actions`, {
          method: "POST",
          body: { action: "approved", comment: "G10 accept replan", payload: {} },
          cookieHeader,
        });
        log(`G10 plan_review approve → ${r.status}`);
        // wait decompose
        const dl = Date.now() + 120000;
        while (Date.now() < dl) {
          pr = await api(`/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`, {
            cookieHeader,
          });
          const p = listOf(pr.json).find((x) => x.id === pending.id);
          if (p && ["accepted", "decomposed", "decomposing"].includes(p.status)) break;
          await sleep(5000);
        }
      }
    }

    await sleep(3000);
    const tasksAfter = await tasksForDemand(cookieHeader, demandId);
    result.evidence.tasks_after_g10 = tasksAfter.map((t) => ({
      id: t.id,
      status: t.status,
      key: t.planned_task_key,
      emp: t.assigned_digital_employee_id || t.digital_employee_id,
    }));

    // G10 hard check: no open/running task with a key that was already completed before expand
    // (unless it is the SAME task id reused)
    const beforeByKey = new Map();
    for (const t of tasksBefore) {
      if (t.planned_task_key && ["completed", "done", "success"].includes(t.status)) {
        beforeByKey.set(t.planned_task_key, t.id);
      }
    }
    const bad = [];
    for (const t of tasksAfter) {
      if (!t.planned_task_key || !beforeByKey.has(t.planned_task_key)) continue;
      const oldId = beforeByKey.get(t.planned_task_key);
      // New non-terminal task with same key as previously completed → FAIL
      if (
        t.id !== oldId &&
        !["completed", "done", "success", "cancelled"].includes(t.status)
      ) {
        bad.push({ key: t.planned_task_key, oldId, newId: t.id, status: t.status });
      }
      // Duplicate completed rows with same key different ids also bad
      if (
        t.id !== oldId &&
        ["completed", "done", "success"].includes(t.status)
      ) {
        // allow only if old is still completed and this is not a recreate — still fail G10
        bad.push({
          key: t.planned_task_key,
          oldId,
          newId: t.id,
          status: t.status,
          note: "duplicate completed key",
        });
      }
    }
    // Count open tasks with completed keys
    const openDup = tasksAfter.filter(
      (t) =>
        t.planned_task_key &&
        completedKeysBefore.has(t.planned_task_key) &&
        !["completed", "done", "success", "cancelled"].includes(t.status) &&
        !beforeByKey.has(t.planned_task_key) === false &&
        t.id !== beforeByKey.get(t.planned_task_key),
    );

    if (bad.length === 0 && openDup.length === 0) {
      // Ensure original completed tasks still present
      const stillCompleted = [...completedKeysBefore].every((k) =>
        tasksAfter.some(
          (t) =>
            t.planned_task_key === k &&
            t.id === beforeByKey.get(k) &&
            ["completed", "done", "success"].includes(t.status),
        ),
      );
      if (!stillCompleted) {
        fail("g10_no_recreate_completed", {
          note: "original completed task missing or status changed",
          completedKeysBefore: [...completedKeysBefore],
          tasksAfter: result.evidence.tasks_after_g10,
        });
      } else {
        pass("g10_no_recreate_completed", {
          completed_keys: [...completedKeysBefore],
          reused_ids: [...beforeByKey.entries()].map(([k, id]) => ({ key: k, id })),
        });
      }
    } else {
      fail("g10_no_recreate_completed", { bad, openDup });
    }

    // ---------- G12: cast developer as reviewer via expansion ----------
    pr = await api(`/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`, {
      cookieHeader,
    });
    const planIdsBeforeG12 = new Set(
      listOf(pr.json).filter((p) => p.demand_id === demandId).map((p) => p.id),
    );

    // Ensure developer is developer; open expansion for reviewer with developer id
    await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      body: {
        scenario_template_key: "software_delivery",
        assignments: [
          { role_key: "developer", digital_employee_id: EMP.developer },
          { role_key: "reviewer", digital_employee_id: EMP.reviewer },
          { role_key: "tester", digital_employee_id: EMP.tester },
        ],
      },
      cookieHeader,
    });

    r = await api(`/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
      method: "POST",
      body: {
        demand_id: demandId,
        suggested_role_key: "reviewer",
        reason: "G12: 扩编 reviewer 指定为原 developer，应触发职责分离",
        scenario_template_key: "software_delivery",
      },
      cookieHeader,
    });
    log(`G12 open expansion → ${r.status}`);
    if (r.status >= 400) {
      fail("g12_open", { status: r.status, body: r.text.slice(0, 200) });
    } else {
      pass("g12_open", { decision_id: r.json.id });
      // API resolve with developer as reviewer (product path payload)
      const inbox = await api(`/api/v1/inbox/items?view=mine&status=open&limit=30`, {
        cookieHeader,
      });
      const expCard = listOf(inbox.json).find(
        (it) =>
          it.source_id === r.json.id ||
          (it.context?.decision_type === "casting_expansion" && it.status === "open"),
      );
      const g12At = Date.now();
      if (expCard) {
        const ar = await api(`/api/v1/inbox/items/${expCard.id}/actions`, {
          method: "POST",
          body: {
            action: "approved",
            comment: "G12 cast developer as reviewer",
            payload: {
              digital_employee_id: EMP.developer,
              role_key: "reviewer",
            },
          },
          cookieHeader,
        });
        log(`G12 approve expansion → ${ar.status} ${ar.text.slice(0, 120)}`);
        if (ar.status >= 400) fail("g12_approve", { status: ar.status, body: ar.text.slice(0, 200) });
        else pass("g12_approve", {});
      } else {
        // direct decision resolve
        const ar = await api(
          `/api/v1/projects/${PROJECT_ID}/decisions/${r.json.id}/resolve`,
          {
            method: "POST",
            body: {
              decision: "approved",
              comment: "G12",
              payload: {
                digital_employee_id: EMP.developer,
                role_key: "reviewer",
              },
            },
            cookieHeader,
          },
        );
        log(`G12 resolve → ${ar.status}`);
        if (ar.status >= 400) fail("g12_approve", { status: ar.status, body: ar.text.slice(0, 200) });
        else pass("g12_approve", { via: "resolve" });
      }

      // casting must show same emp for developer+reviewer
      r = await api(
        `/api/v1/projects/${PROJECT_ID}/castings?template_key=software_delivery`,
        { cookieHeader },
      );
      const castings = listOf(r.json);
      const dev = castings.find((c) => c.role_key === "developer");
      const rev = castings.find((c) => c.role_key === "reviewer");
      const same =
        dev?.digital_employee_id &&
        rev?.digital_employee_id &&
        dev.digital_employee_id === rev.digital_employee_id &&
        dev.digital_employee_id === EMP.developer;
      if (!same) {
        fail("g12_casting_same_person", { castings });
      } else {
        pass("g12_casting_same_person", { employee_id: EMP.developer });
      }

      // Wait for replan outcome under SoD
      const replan12 = await waitForReplan(cookieHeader, demandId, planIdsBeforeG12, g12At);
      result.evidence.g12_replan = replan12;

      // Check: either no free-running deep plan with shared employee, or
      // governance event / pending_review with overbound / planning_gap
      const er = await api(`/api/v1/projects/${PROJECT_ID}/events?limit=40`, {
        cookieHeader,
      });
      const events = listOf(er.json);
      const sodHit = events.some((e) => {
        const s = `${e.summary || ""} ${JSON.stringify(e.payload || {})}`;
        return (
          s.includes("role_independence") ||
          s.includes("职责分离") ||
          s.includes("share employee") ||
          s.includes("constraint role_independence")
        );
      });

      // Effective plan after g12: if decomposed with developer==reviewer on tasks for deep exit → FAIL
      pr = await api(`/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`, {
        cookieHeader,
      });
      const demandPlans = listOf(pr.json).filter((p) => p.demand_id === demandId);
      const latest = demandPlans.sort(
        (a, b) => Date.parse(b.created_at || 0) - Date.parse(a.created_at || 0),
      )[0];
      result.evidence.g12_latest_plan = latest
        ? {
            id: latest.id,
            status: latest.status,
            exit: latest.payload?.exit_deliverable,
            tasks: (latest.payload?.tasks || []).map((t) => ({
              key: t.planned_task_key,
              emp: t.selected_employee_id,
            })),
          }
        : null;

      let illegalDeepPlan = false;
      if (latest && ["accepted", "decomposed", "decomposing"].includes(latest.status)) {
        const exit = latest.payload?.exit_deliverable || "";
        // SoD applies at review_verdict and beyond for software_delivery
        const deep = ["review_verdict", "release_record"].includes(exit);
        if (deep) {
          const emps = (latest.payload?.tasks || []).map((t) => t.selected_employee_id);
          // if plan has both develop and review tasks with same emp
          const byKey = Object.fromEntries(
            (latest.payload?.tasks || []).map((t) => [t.planned_task_key, t.selected_employee_id]),
          );
          if (
            byKey.develop &&
            byKey.review &&
            byKey.develop === byKey.review &&
            byKey.develop === EMP.developer
          ) {
            illegalDeepPlan = true;
          }
        }
      }

      if (illegalDeepPlan) {
        fail("g12_sod_enforced", {
          note: "deep plan accepted with same employee on developer+reviewer tasks",
          plan: result.evidence.g12_latest_plan,
        });
      } else if (
        sodHit ||
        latest?.status === "pending_review" ||
        latest?.status === "validation_failed" ||
        replan12?.fail ||
        // shallow plan auto-accepted is OK only if exit is below SoD threshold
        (latest &&
          ["accepted", "decomposed"].includes(latest.status) &&
          !["review_verdict", "release_record"].includes(latest.payload?.exit_deliverable || ""))
      ) {
        pass("g12_sod_enforced", {
          sod_event: sodHit,
          latest_status: latest?.status,
          exit: latest?.payload?.exit_deliverable,
          note: sodHit
            ? "role_independence observed in events"
            : "no illegal deep shared-employee plan accepted",
        });
      } else {
        fail("g12_sod_enforced", {
          note: "could not confirm SoD enforcement",
          latest: result.evidence.g12_latest_plan,
          sodHit,
        });
      }
    }

    // overall
    const hard = Object.entries(result.gates).filter(
      ([, g]) => g && g.pass !== null && g.pass !== undefined && !g.skipped,
    );
    result.ok =
      hard.every(([, g]) => g.pass === true) &&
      result.gates.g10_no_recreate_completed?.pass === true &&
      result.gates.g12_sod_enforced?.pass === true;

    writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
    log(result.ok ? "PASS G10+G12" : "FAIL — see gates");
    console.log(JSON.stringify(result, null, 2));
    if (!result.ok) process.exitCode = 1;
  } catch (err) {
    result.ok = false;
    result.errors.push(String(err?.stack || err));
    writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
    try {
      await shot(page, "error");
    } catch {
      /* ignore */
    }
    console.error(err);
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}

main();
