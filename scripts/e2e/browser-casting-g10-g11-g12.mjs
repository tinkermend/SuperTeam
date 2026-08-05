/**
 * @deprecated Prefer scripts/e2e/browser-casting-design-g10-g11-g12.mjs (this is intermediate).
 *
 * Design-doc gates G10 / G11 / G12 (real browser + real CP).
 *
 * G10: mid-executing expand a skeleton role → new task assigned to new employee;
 *      previously present planned_task_key (esp. completed) keeps same task id.
 * G11: expansion replan that changes exit (or other overbound) → plan_revision
 *      pending_review with casting_expansion_overbound; NOT auto-decomposed.
 * G12: mid-executing expand verifier to the original operator employee →
 *      durable SoD block (planning_gap / demand failed), not log-only.
 *
 *   node scripts/e2e/browser-casting-g10-g11-g12.mjs
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

const result = {
  ok: false,
  gates: {},
  errors: [],
  evidence: {},
  timeline: [],
};
function log(m) {
  console.log(`[g101112] ${m}`);
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
  return json?.items || json?.demands || json?.tasks || json?.revisions || [];
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
  await page.getByText("项目管理").first().waitFor({ state: "visible" });
}

async function tasksForDemand(cookieHeader, demandId) {
  const r = await api(`/api/v1/projects/${PROJECT_ID}/tasks?limit=80`, {
    cookieHeader,
  });
  return listOf(r.json).filter(
    (t) => t.demand_id === demandId || t.project_demand_id === demandId,
  );
}

async function plansForDemand(cookieHeader, demandId) {
  const r = await api(`/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=30`, {
    cookieHeader,
  });
  return listOf(r.json).filter((p) => p.demand_id === demandId);
}

async function waitPlans(cookieHeader, demandId, pred, ms = WAIT_MS) {
  const dl = Date.now() + ms;
  while (Date.now() < dl) {
    const plans = await plansForDemand(cookieHeader, demandId);
    const tasks = await tasksForDemand(cookieHeader, demandId);
    const dr = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookieHeader });
    const demand = listOf(dr.json).find((d) => d.id === demandId);
    if (pred({ plans, tasks, demand })) return { plans, tasks, demand };
    await sleep(8000);
  }
  return null;
}

async function browserApproveExpansion(page, roleLabel, employeePattern) {
  await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
  await sleep(2000);
  const card = page.getByText("扩编请求").first();
  await card.waitFor({ state: "visible", timeout: 20000 });
  await card.click();
  await sleep(1000);
  // role select
  const roleTrigger = page.locator("#casting-expansion-role, [id*='casting-expansion-role']").first();
  if (await roleTrigger.count()) {
    await roleTrigger.click();
    await sleep(300);
    await page.getByRole("option").filter({ hasText: new RegExp(roleLabel, "i") }).first().click().catch(async () => {
      await page.getByText(roleLabel).first().click();
    });
  }
  const empTrigger = page.locator("#casting-expansion-employee, [id*='casting-expansion-employee']").first();
  if (await empTrigger.count()) {
    await empTrigger.click();
    await sleep(300);
    const opt = page.getByRole("option").filter({ hasText: employeePattern }).first();
    if (await opt.count()) await opt.click();
    else await page.getByRole("option").first().click();
  }
  // submit approve
  const approve = page.getByRole("button", { name: /批准|确认|提交/ }).first();
  await approve.click();
  await sleep(2000);
}

async function approvePlanReviewIfAny(cookieHeader, demandId, planId) {
  const inbox = await api(`/api/v1/inbox/items?view=mine&status=open&limit=40`, {
    cookieHeader,
  });
  const card = listOf(inbox.json).find(
    (it) =>
      it.context?.decision_type === "plan_review" &&
      (it.context?.plan_revision_id === planId ||
        it.context?.demand_id === demandId ||
        (it.context?.plan_revision_id && planId && it.context.plan_revision_id === planId)),
  );
  if (!card) return { status: 0, note: "no plan_review card" };
  return api(`/api/v1/inbox/items/${card.id}/actions`, {
    method: "POST",
    body: { action: "approved", comment: "E2E accept overbound replan", payload: {} },
    cookieHeader,
  });
}

async function tryCompleteProjectTask(cookieHeader, task, empId) {
  // Best-effort: attempt runtime/project complete; seed only if succeeds.
  const bodies = [
    {
      path: `/api/v1/runtime/tasks/${task.id}/complete`,
      method: "POST",
      body: {
        digital_employee_id: empId,
        conclusion: "E2E seed complete for G10",
      },
    },
  ];
  for (const b of bodies) {
    const r = await api(b.path, {
      method: b.method,
      body: b.body,
      cookieHeader,
    });
    log(`try complete ${task.id.slice(0, 8)} → ${r.status} ${r.text.slice(0, 80)}`);
    if (r.status < 400) return true;
  }
  return false;
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(45000);

  try {
    await loginBrowser(page);
    log(`login ${page.url()}`);
    await shot(page, "00-home");
    const cookieHeader = (await context.cookies())
      .map((c) => `${c.name}=${c.value}`)
      .join("; ");

    // Roles for all paths
    await api(`/api/v1/digital-employees/${EMP.developer}/roles`, {
      method: "PUT",
      body: { role_keys: ["developer", "operator", "diagnostician"] },
      cookieHeader,
    });
    await api(`/api/v1/digital-employees/${EMP.reviewer}/roles`, {
      method: "PUT",
      body: { role_keys: ["reviewer", "verifier"] },
      cookieHeader,
    });
    await api(`/api/v1/digital-employees/${EMP.tester}/roles`, {
      method: "PUT",
      body: { role_keys: ["tester"] },
      cookieHeader,
    });
    await api(`/api/v1/digital-employees/${EMP.ops}/roles`, {
      method: "PUT",
      body: { role_keys: ["collector", "analyst", "diagnostician"] },
      cookieHeader,
    });

    // ========== G10 + G11: software_delivery developer-only → expand reviewer ==========
    log("--- G10/G11 setup ---");
    let r = await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      body: {
        scenario_template_key: "software_delivery",
        assignments: [
          { role_key: "developer", digital_employee_id: EMP.developer },
        ],
      },
      cookieHeader,
    });
    log(`cast developer-only → ${r.status}`);
    if (r.status >= 400) throw new Error(`cast ${r.text}`);

    r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, {
      method: "POST",
      body: {
        title: `G10G11 ${new Date().toISOString().slice(11, 19)}`,
        content:
          "需要实现一个小功能并交付分支；先完成开发即可，后续可能补审查与测试",
        scenario_template_key: "software_delivery",
        coordination_mode: "plan",
      },
      cookieHeader,
    });
    if (r.status >= 400 || !r.json?.id) throw new Error(`demand ${r.text}`);
    const g10DemandId = r.json.id;
    result.evidence.g10_demand_id = g10DemandId;
    pass("g10_seed_demand", { demand_id: g10DemandId });
    log(`G10 demand ${g10DemandId}`);

    // Wait initial plan + develop task
    let seed = await waitPlans(
      cookieHeader,
      g10DemandId,
      ({ plans, tasks, demand }) => {
        log(
          `g10 seed wait demand=${demand?.status} plans=${plans.map((p) => p.status + ":" + (p.payload?.exit_deliverable || "")).join(",")} tasks=${tasks.map((t) => t.planned_task_key + ":" + t.status).join(",")}`,
        );
        return (
          tasks.some((t) => t.planned_task_key === "develop" || /develop|开发/.test(t.title || "")) ||
          plans.some((p) =>
            ["pending_review", "accepted", "decomposed", "decomposing"].includes(p.status),
          )
        );
      },
      WAIT_MS,
    );
    if (!seed) {
      fail("g10_initial_plan", { note: "timeout initial plan" });
      throw new Error("g10 initial plan timeout");
    }

    // Approve plan_review if needed for first plan
    let pending0 = seed.plans.find((p) => p.status === "pending_review");
    if (pending0) {
      const ar = await approvePlanReviewIfAny(cookieHeader, g10DemandId, pending0.id);
      log(`g10 initial plan_review → ${ar.status}`);
      seed = await waitPlans(
        cookieHeader,
        g10DemandId,
        ({ tasks, plans }) =>
          tasks.length > 0 ||
          plans.some((p) => ["accepted", "decomposed", "decomposing"].includes(p.status)),
        180000,
      );
    }

    let tasksBefore = await tasksForDemand(cookieHeader, g10DemandId);
    let plansBefore = await plansForDemand(cookieHeader, g10DemandId);
    const developBefore = tasksBefore.find(
      (t) => t.planned_task_key === "develop" || /开发/.test(t.title || ""),
    );
    result.evidence.g10_tasks_before = tasksBefore.map((t) => ({
      id: t.id,
      key: t.planned_task_key,
      status: t.status,
      emp: t.assigned_digital_employee_id,
    }));
    result.evidence.g10_plans_before = plansBefore.map((p) => ({
      id: p.id,
      status: p.status,
      exit: p.payload?.exit_deliverable,
      tasks: (p.payload?.tasks || []).map((t) => ({
        key: t.planned_task_key,
        emp: t.selected_employee_id,
      })),
    }));

    if (developBefore && !["completed", "done", "success"].includes(developBefore.status)) {
      await tryCompleteProjectTask(cookieHeader, developBefore, EMP.developer);
      await sleep(2000);
      tasksBefore = await tasksForDemand(cookieHeader, g10DemandId);
    }
    const completedBefore = tasksBefore.filter((t) =>
      ["completed", "done", "success"].includes(t.status),
    );
    result.evidence.g10_completed_before = completedBefore.map((t) => ({
      id: t.id,
      key: t.planned_task_key,
    }));

    const planIdsBefore = new Set(plansBefore.map((p) => p.id));
    const priorExit =
      plansBefore.find((p) =>
        ["accepted", "decomposed", "decomposing", "pending_review"].includes(p.status),
      )?.payload?.exit_deliverable ||
      plansBefore[0]?.payload?.exit_deliverable ||
      "";

    // Expand reviewer (skeleton role → should create/assign review tasks to EMP.reviewer)
    r = await api(`/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
      method: "POST",
      body: {
        demand_id: g10DemandId,
        suggested_role_key: "reviewer",
        reason: "G10/G11: 扩编审查角色，验证新任务派给新人 + 越界人工确认",
        scenario_template_key: "software_delivery",
      },
      cookieHeader,
    });
    log(`G10 open expansion → ${r.status}`);
    if (r.status >= 400) {
      fail("g10_open_expansion", { status: r.status, body: r.text.slice(0, 200) });
      throw new Error("open expansion failed");
    }
    pass("g10_open_expansion", { decision_id: r.json?.id });

    await browserApproveExpansion(page, "审查|reviewer", /审查|B/);
    await shot(page, "01-g10-expand-approved");
    pass("g10_browser_approve", {});

    // Casting must include reviewer → EMP.reviewer
    r = await api(
      `/api/v1/projects/${PROJECT_ID}/castings?template_key=software_delivery`,
      { cookieHeader },
    );
    const reviewCast = listOf(r.json).find((c) => c.role_key === "reviewer");
    if (reviewCast?.digital_employee_id === EMP.reviewer) {
      pass("g10_casting_reviewer", { employee: EMP.reviewer });
    } else {
      fail("g10_casting_reviewer", { cast: reviewCast, all: listOf(r.json) });
    }

    // Wait replan
    const replan = await waitPlans(
      cookieHeader,
      g10DemandId,
      ({ plans }) => plans.some((p) => !planIdsBefore.has(p.id)),
      WAIT_MS,
    );
    if (!replan) {
      fail("g10_replan", { note: "no new plan revision" });
    } else {
      const newPlans = replan.plans.filter((p) => !planIdsBefore.has(p.id));
      pass("g10_replan", {
        new_plans: newPlans.map((p) => ({
          id: p.id,
          status: p.status,
          exit: p.payload?.exit_deliverable,
        })),
      });
      result.evidence.g10_new_plans = newPlans.map((p) => ({
        id: p.id,
        status: p.status,
        exit: p.payload?.exit_deliverable,
        force_reasons: p.payload?.force_pending_review_reasons || p.force_pending_review_reasons,
        tasks: (p.payload?.tasks || []).map((t) => ({
          key: t.planned_task_key,
          emp: t.selected_employee_id,
        })),
      }));

      // ----- G11: overbound → pending_review -----
      const pending = newPlans.find((p) => p.status === "pending_review");
      const anyDecomposed = newPlans.some((p) =>
        ["accepted", "decomposed", "decomposing"].includes(p.status),
      );
      const newExit = newPlans[0]?.payload?.exit_deliverable || "";
      const exitChanged = priorExit && newExit && priorExit !== newExit;
      result.evidence.g11 = {
        prior_exit: priorExit,
        new_exit: newExit,
        exit_changed: exitChanged,
        pending_review: Boolean(pending),
        auto_ran: anyDecomposed && !pending,
      };

      if (pending) {
        // Durable G11: stuck in pending_review (not auto-run)
        const reasons =
          pending.payload?.force_pending_review_reasons ||
          pending.payload?.review_reasons ||
          pending.payload?.constraint_notes ||
          [];
        pass("g11_force_pending_review", {
          plan_id: pending.id,
          prior_exit: priorExit,
          new_exit: newExit,
          reasons,
        });
        // Browser: see plan confirm card
        await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
        await sleep(1500);
        await shot(page, "02-g11-inbox");
        const body = await page.locator("body").innerText();
        const saw =
          body.includes("计划确认") ||
          body.includes("确认项目计划") ||
          body.includes("计划版本");
        result.evidence.g11.browser_plan_review = saw;
        if (saw) pass("g11_browser_plan_review", {});
        else
          result.gates.g11_browser_plan_review = {
            pass: false,
            soft: true,
            note: "API pending_review is hard proof; browser list may group",
          };

        // Approve to continue G10 task materialization
        const ar = await approvePlanReviewIfAny(cookieHeader, g10DemandId, pending.id);
        log(`G11 plan_review approve → ${ar.status}`);
        await waitPlans(
          cookieHeader,
          g10DemandId,
          ({ plans }) => {
            const p = plans.find((x) => x.id === pending.id);
            return p && ["accepted", "decomposed", "decomposing"].includes(p.status);
          },
          180000,
        );
      } else if (exitChanged && anyDecomposed) {
        fail("g11_force_pending_review", {
          note: "exit changed but plan auto-ran without pending_review — G11 violation",
          prior_exit: priorExit,
          new_exit: newExit,
          plans: newPlans.map((p) => p.status),
        });
      } else if (!exitChanged) {
        // May still be overbound for other reasons; if auto-accepted with only new review tasks, G11 N/A
        result.gates.g11_force_pending_review = {
          pass: false,
          soft: true,
          note: "no exit change observed; G11 needs overbound construction",
          prior_exit: priorExit,
          new_exit: newExit,
          plans: newPlans.map((p) => ({ id: p.id, status: p.status })),
        };
      } else {
        fail("g11_force_pending_review", {
          note: "expected pending_review for overbound replan",
          plans: newPlans,
        });
      }
    }

    await sleep(5000);
    const tasksAfter = await tasksForDemand(cookieHeader, g10DemandId);
    result.evidence.g10_tasks_after = tasksAfter.map((t) => ({
      id: t.id,
      key: t.planned_task_key,
      status: t.status,
      emp: t.assigned_digital_employee_id,
    }));

    // G10: new task for reviewer employee
    const newForReviewer = tasksAfter.filter(
      (t) =>
        t.assigned_digital_employee_id === EMP.reviewer &&
        !tasksBefore.some((b) => b.id === t.id),
    );
    const reviewTask = tasksAfter.find(
      (t) =>
        (t.planned_task_key === "review" || /审查|review/i.test(t.title || "")) &&
        t.assigned_digital_employee_id === EMP.reviewer,
    );
    if (reviewTask || newForReviewer.length > 0) {
      pass("g10_new_task_to_new_employee", {
        review_task: reviewTask
          ? { id: reviewTask.id, key: reviewTask.planned_task_key, emp: reviewTask.assigned_digital_employee_id }
          : null,
        new_for_reviewer: newForReviewer.map((t) => ({
          id: t.id,
          key: t.planned_task_key,
        })),
      });
    } else {
      // Also check plan payload if tasks not yet decomposed
      const plansNow = await plansForDemand(cookieHeader, g10DemandId);
      const latest = plansNow.sort(
        (a, b) => new Date(b.created_at || 0) - new Date(a.created_at || 0),
      )[0];
      const planTasks = latest?.payload?.tasks || [];
      const planReview = planTasks.find(
        (t) =>
          (t.planned_task_key === "review" || t.key === "review") &&
          (t.selected_employee_id === EMP.reviewer ||
            t.assigned_digital_employee_id === EMP.reviewer),
      );
      if (planReview) {
        pass("g10_new_task_to_new_employee", {
          note: "from plan payload (task graph may still be decomposing)",
          plan_task: planReview,
          plan_status: latest.status,
        });
      } else {
        fail("g10_new_task_to_new_employee", {
          tasks_after: result.evidence.g10_tasks_after,
          plan_tasks: planTasks,
        });
      }
    }

    // G10: develop task id stable if existed before
    if (developBefore) {
      const developAfter = tasksAfter.find((t) => t.planned_task_key === "develop");
      if (developAfter && developAfter.id === developBefore.id) {
        pass("g10_planned_task_key_reuse", {
          key: "develop",
          id: developBefore.id,
          status_before: developBefore.status,
          status_after: developAfter.status,
        });
      } else if (!developAfter) {
        // still completed on old id?
        const still = tasksAfter.find((t) => t.id === developBefore.id);
        if (still) {
          pass("g10_planned_task_key_reuse", {
            key: developBefore.planned_task_key || "develop",
            id: still.id,
            note: "same id present after replan",
          });
        } else {
          fail("g10_planned_task_key_reuse", {
            before: developBefore,
            after: developAfter,
          });
        }
      } else {
        fail("g10_planned_task_key_reuse", {
          before_id: developBefore.id,
          after_id: developAfter?.id,
        });
      }
    } else {
      fail("g10_planned_task_key_reuse", { note: "no develop task before expand" });
    }

    // Completed not recreated
    const badCompleted = [];
    for (const c of completedBefore) {
      const same = tasksAfter.find((t) => t.id === c.id);
      if (!same || !["completed", "done", "success"].includes(same.status)) {
        badCompleted.push({ key: c.planned_task_key, id: c.id, same });
      }
      const dup = tasksAfter.filter(
        (t) =>
          t.planned_task_key === c.planned_task_key &&
          t.id !== c.id &&
          !["cancelled"].includes(t.status),
      );
      if (dup.length) badCompleted.push({ key: c.planned_task_key, dups: dup.map((d) => d.id) });
    }
    if (completedBefore.length === 0) {
      result.gates.g10_no_recreate_completed = {
        pass: false,
        soft: true,
        note: "no completed tasks before expand (runtime complete may have failed); key reuse still checked",
      };
    } else if (badCompleted.length === 0) {
      pass("g10_no_recreate_completed", {
        completed: completedBefore.map((c) => ({ id: c.id, key: c.planned_task_key })),
      });
    } else {
      fail("g10_no_recreate_completed", { badCompleted });
    }

    // ========== G12: expansion path SoD on incident_response ==========
    log("--- G12 expansion SoD ---");
    // Distinct cast: operator=developer, verifier=reviewer (not same)
    r = await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      body: {
        scenario_template_key: "incident_response",
        assignments: [
          { role_key: "diagnostician", digital_employee_id: EMP.ops },
          { role_key: "operator", digital_employee_id: EMP.developer },
          { role_key: "verifier", digital_employee_id: EMP.reviewer },
        ],
      },
      cookieHeader,
    });
    log(`G12 cast distinct SoD pair → ${r.status}`);
    if (r.status >= 400) throw new Error(`g12 cast ${r.text}`);

    r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, {
      method: "POST",
      body: {
        title: `G12 expand SoD ${new Date().toISOString().slice(11, 19)}`,
        content:
          "完整故障排查：诊断、修复、独立验证（verification_result）。执行中可能扩编验证角色。",
        scenario_template_key: "incident_response",
        coordination_mode: "plan",
      },
      cookieHeader,
    });
    if (r.status >= 400 || !r.json?.id) throw new Error(`g12 demand ${r.text}`);
    const g12DemandId = r.json.id;
    result.evidence.g12_demand_id = g12DemandId;
    pass("g12_seed_demand", { demand_id: g12DemandId });

    // Wait until executing / has tasks (mid-execution)
    let g12seed = await waitPlans(
      cookieHeader,
      g12DemandId,
      ({ plans, tasks, demand }) => {
        log(
          `g12 seed demand=${demand?.status} plans=${plans.map((p) => p.status + ":" + (p.payload?.exit_deliverable || "")).join(",")} nTasks=${tasks.length}`,
        );
        if (plans.some((p) => p.status === "pending_review")) return true;
        if (tasks.length > 0) return true;
        if (["executing", "planned", "acceptance_pending"].includes(demand?.status)) return true;
        return false;
      },
      WAIT_MS,
    );
    if (!g12seed) {
      fail("g12_seed_executing", { note: "timeout" });
      throw new Error("g12 seed timeout");
    }
    let pend = g12seed.plans.find((p) => p.status === "pending_review");
    if (pend) {
      const ar = await approvePlanReviewIfAny(cookieHeader, g12DemandId, pend.id);
      log(`g12 initial plan_review → ${ar.status}`);
      await waitPlans(
        cookieHeader,
        g12DemandId,
        ({ tasks, plans }) =>
          tasks.length > 0 ||
          plans.some((p) => ["accepted", "decomposed"].includes(p.status)),
        180000,
      );
    }

    // Snapshot operator employee from casting
    r = await api(
      `/api/v1/projects/${PROJECT_ID}/castings?template_key=incident_response`,
      { cookieHeader },
    );
    const opCast = listOf(r.json).find((c) => c.role_key === "operator");
    const verCast = listOf(r.json).find((c) => c.role_key === "verifier");
    result.evidence.g12_cast_before = listOf(r.json).map((c) => ({
      role: c.role_key,
      emp: c.digital_employee_id,
    }));
    if (opCast?.digital_employee_id !== EMP.developer || verCast?.digital_employee_id !== EMP.reviewer) {
      fail("g12_baseline_cast", { opCast, verCast });
    } else {
      pass("g12_baseline_cast_distinct", {});
    }

    const g12PlanIdsBefore = new Set(
      (await plansForDemand(cookieHeader, g12DemandId)).map((p) => p.id),
    );

    // Expand verifier → original operator (developer) — design G12
    r = await api(`/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
      method: "POST",
      body: {
        demand_id: g12DemandId,
        suggested_role_key: "verifier",
        reason: "G12: 扩编验证角色，指定原处置人（operator）触发职责分离",
        scenario_template_key: "incident_response",
      },
      cookieHeader,
    });
    log(`G12 open expansion → ${r.status}`);
    if (r.status >= 400) {
      fail("g12_open_expansion", { status: r.status, body: r.text.slice(0, 200) });
      throw new Error("g12 open failed");
    }
    pass("g12_open_expansion", { decision_id: r.json?.id });

    await browserApproveExpansion(page, "验证|verifier", /开发|A/);
    await shot(page, "03-g12-expand-approved");

    // Casting after: verifier should be developer
    await sleep(2000);
    r = await api(
      `/api/v1/projects/${PROJECT_ID}/castings?template_key=incident_response`,
      { cookieHeader },
    );
    const verAfter = listOf(r.json).find((c) => c.role_key === "verifier");
    const opAfter = listOf(r.json).find((c) => c.role_key === "operator");
    result.evidence.g12_cast_after = listOf(r.json).map((c) => ({
      role: c.role_key,
      emp: c.digital_employee_id,
    }));
    if (
      verAfter?.digital_employee_id === EMP.developer &&
      opAfter?.digital_employee_id === EMP.developer
    ) {
      pass("g12_casting_sod_pair_written", {
        operator: opAfter.digital_employee_id,
        verifier: verAfter.digital_employee_id,
      });
    } else {
      fail("g12_casting_sod_pair_written", { verAfter, opAfter });
    }

    // Wait durable SoD surface for THIS demand (failed / planning_failed)
    await waitPlans(
      cookieHeader,
      g12DemandId,
      ({ demand }) => {
        log(`g12 replan wait demand=${demand?.status}`);
        return demand?.status === "failed" || demand?.status === "planning_failed";
      },
      WAIT_MS,
    );

    // Also poll inbox for planning_gap on this demand
    let gapCard = null;
    for (let i = 0; i < 20; i++) {
      const inbox = await api(`/api/v1/inbox/items?view=mine&status=open&limit=50`, {
        cookieHeader,
      });
      gapCard = listOf(inbox.json).find((it) => {
        const dt = it.context?.decision_type || it.kind;
        if (dt !== "planning_gap" && dt !== "planning_failed") return false;
        if (it.context?.demand_id === g12DemandId) return true;
        const blob = `${it.title || ""} ${it.summary || ""}`;
        return (
          blob.includes("职责分离") ||
          blob.includes("剧本编制") ||
          blob.includes("role_independence") ||
          blob.includes("独立性")
        );
      });
      if (gapCard) break;
      const dr = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookieHeader });
      const dem = listOf(dr.json).find((d) => d.id === g12DemandId);
      log(`g12 wait demand=${dem?.status} gap=${Boolean(gapCard)}`);
      if (dem?.status === "failed" || dem?.status === "planning_failed") break;
      await sleep(8000);
    }

    const dr = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookieHeader });
    const g12Demand = listOf(dr.json).find((d) => d.id === g12DemandId);
    const g12Plans = await plansForDemand(cookieHeader, g12DemandId);
    result.evidence.g12_outcome = {
      demand_status: g12Demand?.status,
      gap_card: gapCard
        ? {
            id: gapCard.id,
            kind: gapCard.kind,
            title: gapCard.title,
            demand_id: gapCard.context?.demand_id,
            diagnosis: gapCard.context?.diagnosis || gapCard.summary,
          }
        : null,
      plans: g12Plans.map((p) => ({
        id: p.id,
        status: p.status,
        exit: p.payload?.exit_deliverable,
      })),
    };

    // Illegal plan check
    let illegal = false;
    for (const p of g12Plans) {
      if (!["accepted", "decomposed", "decomposing"].includes(p.status)) continue;
      if (g12PlanIdsBefore.has(p.id)) continue;
      const tasks = p.payload?.tasks || [];
      const fix = tasks.find((t) => /fix|operator/.test(t.planned_task_key || t.key || ""));
      const ver = tasks.find((t) => /verif/.test(t.planned_task_key || t.key || ""));
      if (
        fix?.selected_employee_id === EMP.developer &&
        ver?.selected_employee_id === EMP.developer
      ) {
        illegal = true;
      }
    }

    if (illegal) {
      fail("g12_expansion_sod_blocked", {
        note: "deep plan accepted with operator=verifier=same emp after expansion",
      });
    } else if (
      g12Demand?.status === "failed" ||
      g12Demand?.status === "planning_failed" ||
      (gapCard &&
        (String(gapCard.title || "").includes("职责分离") ||
          String(gapCard.title || "").includes("剧本编制") ||
          String(gapCard.context?.diagnosis || "").includes("职责分离") ||
          gapCard.context?.demand_id === g12DemandId ||
          String(gapCard.title || "").includes("独立性")))
    ) {
      pass("g12_expansion_sod_blocked", {
        demand_status: g12Demand?.status,
        gap: result.evidence.g12_outcome.gap_card,
        path: "mid-executing casting_expansion of verifier → original operator",
      });
    } else {
      fail("g12_expansion_sod_blocked", {
        note: "no durable fail/gap after expansion SoD cast",
        ...result.evidence.g12_outcome,
      });
    }

    // Browser G12
    await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
    await sleep(2000);
    try {
      const exceptionHdr = page.getByText("异常处理").first();
      if (await exceptionHdr.count()) await exceptionHdr.click().catch(() => {});
    } catch {
      /* ignore */
    }
    await shot(page, "04-g12-inbox");
    const g12Body = await page.locator("body").innerText();
    const browserG12 =
      g12Body.includes("规划缺口") ||
      g12Body.includes("职责分离") ||
      g12Body.includes("剧本编制") ||
      g12Body.includes("规划失败");
    result.evidence.g12_browser = { saw: browserG12, snippet: g12Body.slice(0, 1000) };
    if (browserG12) pass("g12_browser_surface", {});
    else
      result.gates.g12_browser_surface = {
        pass: false,
        soft: true,
        note: "API durable surface is hard proof",
      };

    // Aggregate
    const hard = [
      "g10_new_task_to_new_employee",
      "g10_planned_task_key_reuse",
      "g11_force_pending_review",
      "g12_expansion_sod_blocked",
      "g12_casting_sod_pair_written",
    ];
    result.ok = hard.every((k) => result.gates[k]?.pass === true);

    const payload = JSON.stringify(result, null, 2);
    writeFileSync(join(OUT, "result.json"), payload);
    writeFileSync(join(SCRATCH, "g10-g11-g12-result.json"), payload);
    log(result.ok ? "PASS G10+G11+G12" : "FAIL — see gates");
    console.log(payload);
    if (!result.ok) process.exitCode = 1;
  } catch (e) {
    result.errors.push(String(e.stack || e));
    writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
    writeFileSync(join(SCRATCH, "g10-g11-g12-result.json"), JSON.stringify(result, null, 2));
    console.error(e);
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}
main();
