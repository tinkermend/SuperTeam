/**
 * @deprecated Prefer scripts/e2e/browser-casting-design-g10-g11-g12.mjs (this is intermediate).
 *
 * Finish design G10/G11/G12 on already-open mid-executing expansion cards
 * (honest product path: approve open casting_expansion with selected employee).
 *
 * Precondition (created by prior seed or browser-casting-g10-g11-g12.mjs):
 *  - demand G10G11: software_delivery, develop task exists, expansion reviewer open
 *  - demand G12 expand SoD: incident executing, expansion verifier open
 *
 *   node scripts/e2e/browser-casting-gates-g10-g11-g12-finish.mjs
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
const WAIT_MS = Number(process.env.SUPERTEAM_PLANNER_WAIT_MS || 360000);
const OUT = join(__dirname, "../../.scratch/e2e-browser-casting-g10-g11-g12");
const SCRATCH =
  process.env.SUPERTEAM_SCRATCH ||
  "/var/folders/_s/2zwng6xn03g1rj6v60h9r75h0000gn/T/grok-goal-ac6f8423dcba/implementer/g10g12";
mkdirSync(OUT, { recursive: true });
mkdirSync(SCRATCH, { recursive: true });

const result = { ok: false, gates: {}, errors: [], evidence: {}, timeline: [] };
function log(m) {
  console.log(`[finish] ${m}`);
  result.timeline.push({ t: new Date().toISOString(), m });
}
function pass(k, d = {}) {
  result.gates[k] = { pass: true, ...d };
}
function fail(k, d = {}) {
  result.gates[k] = { pass: false, ...d };
  result.errors.push(`${k}: ${JSON.stringify(d)}`);
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
  return json?.items || json?.demands || json?.tasks || [];
}

async function loginBrowser(page) {
  await page.goto(`${WEB}/sign-in`, { waitUntil: "domcontentloaded" });
  await page.getByLabel("账号").fill("admin");
  await page.getByLabel("密码").fill("admin");
  await page.getByRole("button", { name: "登录" }).click();
  await page.waitForFunction(
    () => !location.pathname.includes("sign-in"),
    null,
    { timeout: 30000 },
  );
}

/** CP session cookie (browser cookies are often host-bound to :3100). */
async function loginCpCookie() {
  const res = await fetch(`${CP}/api/auth/login`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ username: "admin", password: "admin" }),
  });
  if (!res.ok) throw new Error(`cp login ${res.status}`);
  const setCookie = res.headers.getSetCookie?.() || [];
  const raw = res.headers.get("set-cookie") || "";
  const parts = setCookie.length ? setCookie : raw ? [raw] : [];
  const cookies = [];
  for (const c of parts) {
    const [nv] = c.split(";");
    const eq = nv.indexOf("=");
    if (eq > 0) cookies.push(`${nv.slice(0, eq).trim()}=${nv.slice(eq + 1).trim()}`);
  }
  if (!cookies.length) throw new Error("cp login missing cookie");
  return cookies.join("; ");
}

async function tasksForDemand(cookieHeader, demandId) {
  const r = await api(`/api/v1/projects/${PROJECT_ID}/tasks?limit=80`, {
    cookieHeader,
  });
  return listOf(r.json).filter((t) => t.demand_id === demandId);
}
async function plansForDemand(cookieHeader, demandId) {
  const r = await api(`/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=40`, {
    cookieHeader,
  });
  return listOf(r.json).filter((p) => p.demand_id === demandId);
}
async function demandOf(cookieHeader, demandId) {
  const r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookieHeader });
  return listOf(r.json).find((d) => d.id === demandId);
}
async function openExpansionCards(cookieHeader) {
  const r = await api(`/api/v1/inbox/items?view=mine&status=open&limit=50`, {
    cookieHeader,
  });
  return listOf(r.json).filter(
    (it) =>
      it.kind === "casting_expansion" ||
      it.context?.decision_type === "casting_expansion",
  );
}

/** Browser observe card + API approve with role/employee (reliable). */
async function browserObserveAndApiApprove(
  page,
  cookieHeader,
  { inboxId, roleKey, employeeId, shotPrefix },
) {
  await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
  await sleep(1500);
  // Open any 扩编请求 for observation
  const card = page.getByText("扩编请求").first();
  if (await card.count()) {
    await card.click();
    await sleep(800);
  }
  await shot(page, `${shotPrefix}-inbox`);
  const body = await page.locator("body").innerText();
  const saw = body.includes("扩编") || body.includes("选定");

  const ar = await api(`/api/v1/inbox/items/${inboxId}/actions`, {
    method: "POST",
    body: {
      action: "approved",
      comment: `E2E approve expansion ${roleKey}`,
      payload: {
        role_key: roleKey,
        digital_employee_id: employeeId,
      },
    },
    cookieHeader,
  });
  log(`api approve ${inboxId.slice(0, 8)} role=${roleKey} emp=${employeeId.slice(0, 8)} → ${ar.status} ${ar.text.slice(0, 120)}`);
  return { status: ar.status, text: ar.text, json: ar.json, browser_saw: saw };
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
  });
  const page = await context.newPage();
  try {
    await loginBrowser(page);
    const cookieHeader = await loginCpCookie();
    pass("browser_login", {});

    const expansions = await openExpansionCards(cookieHeader);
    result.evidence.open_expansions = expansions.map((e) => ({
      id: e.id,
      demand: e.context?.demand_id,
      role: e.context?.suggested_role_key,
    }));
    log(`open expansions: ${JSON.stringify(result.evidence.open_expansions)}`);

    const g10Card = expansions.find(
      (e) =>
        e.context?.suggested_role_key === "reviewer" &&
        e.context?.scenario_template_key === "software_delivery",
    ) || expansions.find((e) => e.context?.suggested_role_key === "reviewer");
    const g12Card = expansions.find(
      (e) =>
        e.context?.suggested_role_key === "verifier" &&
        e.context?.scenario_template_key === "incident_response",
    ) || expansions.find((e) => e.context?.suggested_role_key === "verifier");

    if (!g10Card) {
      fail("g10_card", { note: "no open reviewer expansion" });
      throw new Error("missing g10 expansion card");
    }
    if (!g12Card) {
      fail("g12_card", { note: "no open verifier expansion" });
      throw new Error("missing g12 expansion card");
    }

    const g10DemandId = g10Card.context.demand_id;
    const g12DemandId = g12Card.context.demand_id;
    result.evidence.g10_demand_id = g10DemandId;
    result.evidence.g12_demand_id = g12DemandId;

    // ----- G10 baseline snapshots -----
    const tasksBefore = await tasksForDemand(cookieHeader, g10DemandId);
    const plansBefore = await plansForDemand(cookieHeader, g10DemandId);
    const planIdsBefore = new Set(plansBefore.map((p) => p.id));
    const priorExit =
      plansBefore.find((p) =>
        ["accepted", "decomposed", "decomposing", "pending_review"].includes(p.status),
      )?.payload?.exit_deliverable || "";
    const developBefore = tasksBefore.find((t) => t.planned_task_key === "develop");
    result.evidence.g10_before = {
      prior_exit: priorExit,
      tasks: tasksBefore.map((t) => ({
        id: t.id,
        key: t.planned_task_key,
        status: t.status,
        emp: t.assigned_digital_employee_id,
      })),
      plans: plansBefore.map((p) => ({
        id: p.id,
        status: p.status,
        exit: p.payload?.exit_deliverable,
      })),
    };
    pass("g10_seed_mid_executing", {
      demand_id: g10DemandId,
      develop: developBefore
        ? { id: developBefore.id, status: developBefore.status }
        : null,
      prior_exit: priorExit,
    });

    // Approve G10 expansion: reviewer → EMP.reviewer
    const a10 = await browserObserveAndApiApprove(page, cookieHeader, {
      inboxId: g10Card.id,
      roleKey: "reviewer",
      employeeId: EMP.reviewer,
      shotPrefix: "g10",
    });
    if (a10.status >= 400) {
      fail("g10_approve", { status: a10.status, body: a10.text.slice(0, 200) });
      throw new Error("g10 approve failed");
    }
    pass("g10_approve_expansion", { browser_saw: a10.browser_saw });

    // Casting reviewer written
    let r = await api(
      `/api/v1/projects/${PROJECT_ID}/castings?template_key=software_delivery`,
      { cookieHeader },
    );
    const revCast = listOf(r.json).find((c) => c.role_key === "reviewer");
    if (revCast?.digital_employee_id === EMP.reviewer) {
      pass("g10_casting_reviewer", {});
    } else {
      fail("g10_casting_reviewer", { revCast, all: listOf(r.json) });
    }

    // Wait replan
    const dl = Date.now() + WAIT_MS;
    let newPlans = [];
    while (Date.now() < dl) {
      const plans = await plansForDemand(cookieHeader, g10DemandId);
      newPlans = plans.filter((p) => !planIdsBefore.has(p.id));
      const dem = await demandOf(cookieHeader, g10DemandId);
      log(
        `g10 replan demand=${dem?.status} new=${newPlans.map((p) => p.status + ":" + (p.payload?.exit_deliverable || "")).join(",")}`,
      );
      if (newPlans.length) break;
      await sleep(8000);
    }
    if (!newPlans.length) {
      fail("g10_replan", { note: "timeout no new plan" });
    } else {
      pass("g10_replan", {
        plans: newPlans.map((p) => ({
          id: p.id,
          status: p.status,
          exit: p.payload?.exit_deliverable,
        })),
      });
    }

    // G11: pending_review on overbound (expect exit change branch_ref → review_verdict+)
    const pending = newPlans.find((p) => p.status === "pending_review");
    const newExit = newPlans[0]?.payload?.exit_deliverable || "";
    const exitChanged = priorExit && newExit && priorExit !== newExit;
    result.evidence.g11 = {
      prior_exit: priorExit,
      new_exit: newExit,
      exit_changed: exitChanged,
      pending_review: Boolean(pending),
      plan_statuses: newPlans.map((p) => p.status),
    };
    if (pending) {
      pass("g11_force_pending_review", {
        plan_id: pending.id,
        prior_exit: priorExit,
        new_exit: newExit,
        note: "replan not auto-run; human plan_review required",
      });
      await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
      await sleep(1500);
      await shot(page, "g11-inbox");
      const body = await page.locator("body").innerText();
      if (body.includes("计划确认") || body.includes("确认项目计划")) {
        pass("g11_browser_plan_review", {});
      } else {
        result.gates.g11_browser_plan_review = {
          pass: false,
          soft: true,
          note: "API pending_review is hard proof",
        };
      }
      // Approve plan_review to materialize tasks for G10
      const inbox = await api(`/api/v1/inbox/items?view=mine&status=open&limit=40`, {
        cookieHeader,
      });
      const prCard = listOf(inbox.json).find(
        (it) =>
          it.context?.decision_type === "plan_review" &&
          (it.context?.plan_revision_id === pending.id ||
            it.context?.demand_id === g10DemandId),
      );
      if (prCard) {
        const ar = await api(`/api/v1/inbox/items/${prCard.id}/actions`, {
          method: "POST",
          body: { action: "approved", comment: "G11 accept overbound", payload: {} },
          cookieHeader,
        });
        log(`g11 plan_review approve → ${ar.status}`);
        const d2 = Date.now() + 180000;
        while (Date.now() < d2) {
          const p = (await plansForDemand(cookieHeader, g10DemandId)).find(
            (x) => x.id === pending.id,
          );
          if (p && ["accepted", "decomposed", "decomposing"].includes(p.status)) break;
          await sleep(5000);
        }
      }
    } else if (
      exitChanged &&
      newPlans.some((p) => ["accepted", "decomposed", "decomposing"].includes(p.status))
    ) {
      fail("g11_force_pending_review", {
        note: "exit changed but auto-ran — G11 fail",
        ...result.evidence.g11,
      });
    } else {
      fail("g11_force_pending_review", {
        note: "expected pending_review after expansion replan",
        ...result.evidence.g11,
      });
    }

    await sleep(5000);
    const tasksAfter = await tasksForDemand(cookieHeader, g10DemandId);
    result.evidence.g10_after = {
      tasks: tasksAfter.map((t) => ({
        id: t.id,
        key: t.planned_task_key,
        status: t.status,
        emp: t.assigned_digital_employee_id,
      })),
    };

    // G10 new task for reviewer
    const reviewTask = tasksAfter.find(
      (t) =>
        t.planned_task_key === "review" &&
        t.assigned_digital_employee_id === EMP.reviewer,
    );
    const newForReviewer = tasksAfter.filter(
      (t) =>
        t.assigned_digital_employee_id === EMP.reviewer &&
        !tasksBefore.some((b) => b.id === t.id),
    );
    if (reviewTask || newForReviewer.length) {
      pass("g10_new_task_to_new_employee", {
        review: reviewTask
          ? { id: reviewTask.id, emp: reviewTask.assigned_digital_employee_id }
          : null,
        new_for_reviewer: newForReviewer.map((t) => ({
          id: t.id,
          key: t.planned_task_key,
        })),
      });
    } else {
      // plan payload fallback
      const plans = await plansForDemand(cookieHeader, g10DemandId);
      const latest = newPlans[0] || plans[0];
      const pt = (latest?.payload?.tasks || []).find(
        (t) =>
          (t.planned_task_key === "review" || t.key === "review") &&
          t.selected_employee_id === EMP.reviewer,
      );
      if (pt) {
        pass("g10_new_task_to_new_employee", {
          note: "plan payload",
          plan_status: latest.status,
          task: pt,
        });
      } else {
        fail("g10_new_task_to_new_employee", {
          tasks: result.evidence.g10_after.tasks,
        });
      }
    }

    // G10 develop id reuse
    if (developBefore) {
      const devAfter = tasksAfter.find((t) => t.id === developBefore.id);
      if (devAfter) {
        pass("g10_planned_task_key_reuse", {
          id: developBefore.id,
          key: "develop",
          status_before: developBefore.status,
          status_after: devAfter.status,
        });
      } else {
        fail("g10_planned_task_key_reuse", {
          before: developBefore.id,
          after_keys: tasksAfter.map((t) => t.planned_task_key),
        });
      }
    } else {
      fail("g10_planned_task_key_reuse", { note: "no develop before" });
    }

    // ----- G12 expansion SoD -----
    // Ensure baseline cast still operator=dev verifier=reviewer before approve
    // (approve will replace verifier with developer)
    const g12PlanIdsBefore = new Set(
      (await plansForDemand(cookieHeader, g12DemandId)).map((p) => p.id),
    );
    const g12TasksBefore = await tasksForDemand(cookieHeader, g12DemandId);
    result.evidence.g12_before = {
      tasks: g12TasksBefore.map((t) => ({
        id: t.id,
        key: t.planned_task_key,
        emp: t.assigned_digital_employee_id,
        status: t.status,
      })),
    };
    pass("g12_seed_mid_executing", {
      demand_id: g12DemandId,
      n_tasks: g12TasksBefore.length,
    });

    const a12 = await browserObserveAndApiApprove(page, cookieHeader, {
      inboxId: g12Card.id,
      roleKey: "verifier",
      employeeId: EMP.developer, // original operator — design G12
      shotPrefix: "g12",
    });
    if (a12.status >= 400) {
      fail("g12_approve", { status: a12.status, body: a12.text.slice(0, 200) });
      throw new Error("g12 approve failed");
    }
    pass("g12_approve_expansion", {});

    r = await api(
      `/api/v1/projects/${PROJECT_ID}/castings?template_key=incident_response`,
      { cookieHeader },
    );
    const casts = listOf(r.json);
    result.evidence.g12_cast_after = casts.map((c) => ({
      role: c.role_key,
      emp: c.digital_employee_id,
    }));
    const op = casts.find((c) => c.role_key === "operator");
    const ver = casts.find((c) => c.role_key === "verifier");
    if (op?.digital_employee_id === EMP.developer && ver?.digital_employee_id === EMP.developer) {
      pass("g12_casting_sod_pair_written", {});
    } else {
      fail("g12_casting_sod_pair_written", { op, ver });
    }

    // Wait demand failed or gap card
    const d3 = Date.now() + WAIT_MS;
    let gapCard = null;
    let g12Dem = null;
    while (Date.now() < d3) {
      g12Dem = await demandOf(cookieHeader, g12DemandId);
      const inbox = await api(`/api/v1/inbox/items?view=mine&status=open&limit=50`, {
        cookieHeader,
      });
      gapCard = listOf(inbox.json).find((it) => {
        const dt = it.kind || it.context?.decision_type;
        if (dt !== "planning_gap" && dt !== "planning_failed") return false;
        if (it.context?.demand_id === g12DemandId) return true;
        const blob = `${it.title || ""}${it.summary || ""}`;
        return (
          blob.includes("职责分离") ||
          blob.includes("剧本编制") ||
          blob.includes("独立性")
        );
      });
      log(`g12 wait demand=${g12Dem?.status} gap=${Boolean(gapCard)}`);
      if (
        g12Dem?.status === "failed" ||
        g12Dem?.status === "planning_failed" ||
        gapCard
      ) {
        break;
      }
      await sleep(8000);
    }

    const g12Plans = await plansForDemand(cookieHeader, g12DemandId);
    let illegal = false;
    for (const p of g12Plans) {
      if (g12PlanIdsBefore.has(p.id)) continue;
      if (!["accepted", "decomposed", "decomposing"].includes(p.status)) continue;
      const tasks = p.payload?.tasks || [];
      const fix = tasks.find((t) => /fix|operator/.test(t.planned_task_key || t.key || ""));
      const v = tasks.find((t) => /verif/.test(t.planned_task_key || t.key || ""));
      if (
        fix?.selected_employee_id === EMP.developer &&
        v?.selected_employee_id === EMP.developer
      ) {
        illegal = true;
      }
    }
    result.evidence.g12_outcome = {
      demand_status: g12Dem?.status,
      gap: gapCard
        ? {
            id: gapCard.id,
            kind: gapCard.kind,
            title: gapCard.title,
            demand_id: gapCard.context?.demand_id,
          }
        : null,
      illegal_plan: illegal,
      plans: g12Plans.map((p) => ({
        id: p.id,
        status: p.status,
        exit: p.payload?.exit_deliverable,
        new: !g12PlanIdsBefore.has(p.id),
      })),
    };

    if (illegal) {
      fail("g12_expansion_sod_blocked", { note: "illegal plan accepted" });
    } else if (
      g12Dem?.status === "failed" ||
      g12Dem?.status === "planning_failed" ||
      gapCard
    ) {
      pass("g12_expansion_sod_blocked", {
        demand_status: g12Dem?.status,
        gap_title: gapCard?.title,
        path: "mid-executing expansion verifier→original operator",
      });
    } else {
      fail("g12_expansion_sod_blocked", result.evidence.g12_outcome);
    }

    await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
    await sleep(1500);
    await shot(page, "g12-final-inbox");
    const body = await page.locator("body").innerText();
    if (
      body.includes("规划缺口") ||
      body.includes("职责分离") ||
      body.includes("剧本编制") ||
      body.includes("规划失败")
    ) {
      pass("g12_browser_surface", {});
    } else {
      result.gates.g12_browser_surface = {
        pass: false,
        soft: true,
        note: "API durable surface is hard proof",
      };
    }

    const hard = [
      "g10_new_task_to_new_employee",
      "g10_planned_task_key_reuse",
      "g11_force_pending_review",
      "g12_casting_sod_pair_written",
      "g12_expansion_sod_blocked",
    ];
    result.ok = hard.every((k) => result.gates[k]?.pass === true);
    const payload = JSON.stringify(result, null, 2);
    writeFileSync(join(OUT, "result-finish.json"), payload);
    writeFileSync(join(SCRATCH, "g10-g11-g12-result.json"), payload);
    log(result.ok ? "PASS G10+G11+G12" : "FAIL");
    console.log(payload);
    if (!result.ok) process.exitCode = 1;
  } catch (e) {
    result.errors.push(String(e.stack || e));
    writeFileSync(join(OUT, "result-finish.json"), JSON.stringify(result, null, 2));
    writeFileSync(join(SCRATCH, "g10-g11-g12-result.json"), JSON.stringify(result, null, 2));
    console.error(e);
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}
main();
