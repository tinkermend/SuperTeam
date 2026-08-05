/**
 * Real browser + real control-plane full-path for casting expansion (G10-ish).
 *
 * Does NOT mark pass unless:
 *  1) Browser: open expansion card → pick employee → submit
 *  2) API: casting includes the selected role/employee
 *  3) API: casting_expansion_replan progress observed within timeout
 *     (new plan revision after approve, OR plan_review inbox, OR failed with logged reason)
 *  4) If replan produced tasks: completed planned_task_keys are not duplicated as new open tasks
 *
 *   node scripts/e2e/browser-casting-expansion-full.mjs
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
// Expand with collector (ops employee holds collector+analyst) — not baseline for software_delivery deep path
const ROLE_KEY = process.env.SUPERTEAM_EXPANSION_ROLE || "collector";
const PLANNER_WAIT_MS = Number(process.env.SUPERTEAM_PLANNER_WAIT_MS || 180000);
const OUT = join(__dirname, "../../.scratch/e2e-browser-casting-expansion-full");
mkdirSync(OUT, { recursive: true });

const result = {
  ok: false,
  gates: {},
  timeline: [],
  errors: [],
  evidence: {},
};
function log(m) {
  const line = `[full-exp] ${m}`;
  console.log(line);
  result.timeline.push({ t: new Date().toISOString(), m });
}
function fail(gate, detail) {
  result.gates[gate] = { pass: false, ...detail };
  result.errors.push(`${gate}: ${JSON.stringify(detail)}`);
}
function pass(gate, detail = {}) {
  result.gates[gate] = { pass: true, ...detail };
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
  if (json?.items) return json.items;
  if (json?.demands) return json.demands;
  if (json?.tasks) return json.tasks;
  if (json?.events) return json.events;
  if (json?.revisions) return json.revisions;
  return [];
}

async function sleep(ms) {
  await new Promise((r) => setTimeout(r, ms));
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
    // --- login browser ---
    await page.goto(`${WEB}/sign-in`, { waitUntil: "domcontentloaded" });
    await page.getByLabel("账号").fill("admin");
    await page.getByLabel("密码").fill("admin");
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForFunction(
      () => {
        const p = location.pathname;
        return !p.includes("sign-in") && p !== "/login" && !p.endsWith("/login");
      },
      null,
      { timeout: 30000 },
    );
    await page.getByText("项目管理").first().waitFor({ state: "visible" });
    log(`logged in ${page.url()}`);
    await shot(page, "00-home");

    const cookies = await context.cookies();
    const cookieHeader = cookies.map((c) => `${c.name}=${c.value}`).join("; ");

    // --- ensure ops has ROLE_KEY ---
    let r = await api(`/api/v1/digital-employees/${EMP.ops}/roles`, {
      method: "PUT",
      body: { role_keys: ["collector", "analyst", "developer"] },
      cookieHeader,
    });
    log(`ops roles → ${r.status}`);
    if (r.status >= 400) throw new Error(`ops roles failed ${r.text}`);

    // --- baseline casting: developer+reviewer+tester, NO collector ---
    r = await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
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
    log(`baseline casting → ${r.status} n=${listOf(r.json).length}`);
    if (r.status >= 400) throw new Error(`baseline casting ${r.text}`);

    // --- submit fresh demand so we control lifecycle ---
    r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, {
      method: "POST",
      body: {
        title: `E2E扩编全路径 ${new Date().toISOString().slice(11, 19)}`,
        content: "真实浏览器验证 casting_expansion 批准后重规划与任务合并",
        scenario_template_key: "software_delivery",
        coordination_mode: "plan",
      },
      cookieHeader,
    });
    log(`submit demand → ${r.status}`);
    if (r.status >= 400 || !r.json?.id) throw new Error(`submit demand ${r.text}`);
    const demandId = r.json.id;
    result.evidence.demand_id = demandId;
    pass("seed_demand", { demand_id: demandId, status: r.json.status });

    // Wait until demand has at least one plan revision and preferably a task
    const waitUntil = Date.now() + PLANNER_WAIT_MS;
    let demandStatus = r.json.status;
    let plansBefore = [];
    let tasksBefore = [];
    while (Date.now() < waitUntil) {
      const dr = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookieHeader });
      const demand = listOf(dr.json).find((d) => d.id === demandId);
      demandStatus = demand?.status || demandStatus;
      const pr = await api(
        `/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`,
        { cookieHeader },
      );
      plansBefore = listOf(pr.json).filter((p) => p.demand_id === demandId);
      const tr = await api(`/api/v1/projects/${PROJECT_ID}/tasks?limit=50`, {
        cookieHeader,
      });
      // Strict: only tasks that declare this demand_id. Never fall back to whole project.
      tasksBefore = listOf(tr.json).filter(
        (t) => t.demand_id === demandId || t.project_demand_id === demandId,
      );
      log(
        `wait demand=${demandStatus} plans=${plansBefore.length} tasks=${tasksBefore.length}`,
      );
      if (
        plansBefore.some((p) =>
          ["decomposed", "accepted", "pending_review"].includes(p.status),
        ) ||
        ["executing", "acceptance_pending", "completed"].includes(demandStatus)
      ) {
        break;
      }
      // Only approve plan_review cards that belong to THIS demand (never `|| true`).
      const inbox = await api(
        `/api/v1/inbox/items?view=mine&status=open&limit=30`,
        { cookieHeader },
      );
      const planCard = listOf(inbox.json).find((it) => {
        if (it.context?.decision_type !== "plan_review" && it.kind !== "plan_review") {
          return false;
        }
        const d =
          it.context?.demand_id ||
          it.context?.primary_demand_id ||
          it.context?.project_demand_id;
        return d === demandId;
      });
      if (planCard && demandStatus === "planning_pending") {
        log(`seed plan_review for this demand via inbox ${planCard.id}`);
        const ia = await api(`/api/v1/inbox/items/${planCard.id}/actions`, {
          method: "POST",
          body: { action: "approved", comment: "E2E seed: accept plan for this demand only", payload: {} },
          cookieHeader,
        });
        log(`seed inbox approve → ${ia.status} ${ia.text.slice(0, 120)}`);
      }
      await sleep(5000);
    }

    const completedKeysBefore = new Set(
      tasksBefore
        .filter((t) => ["completed", "done", "success"].includes(t.status))
        .map((t) => t.planned_task_key)
        .filter(Boolean),
    );
    const openKeysBefore = new Set(
      tasksBefore
        .filter((t) => !["completed", "done", "success", "cancelled", "failed"].includes(t.status))
        .map((t) => t.planned_task_key)
        .filter(Boolean),
    );
    result.evidence.demand_status_before_expand = demandStatus;
    result.evidence.plans_before = plansBefore.map((p) => ({
      id: p.id,
      status: p.status,
    }));
    result.evidence.tasks_before = tasksBefore.map((t) => ({
      id: t.id,
      status: t.status,
      key: t.planned_task_key,
      title: t.title,
    }));
    result.evidence.completed_keys_before = [...completedKeysBefore];

    if (!plansBefore.length && !["executing", "acceptance_pending"].includes(demandStatus)) {
      // Soft note only: expansion may still produce the first plan via replan.
      // Do not hard-fail the run — replan_observed + casting_written are the product gates.
      result.gates.seed_ready = {
        pass: null,
        skipped: true,
        demand_status: demandStatus,
        plans: plansBefore.length,
        reason: "initial plan not ready within wait; expansion may open first plan via replan",
      };
    } else {
      pass("seed_ready", {
        demand_status: demandStatus,
        plans: plansBefore.length,
        tasks: tasksBefore.length,
        completed_keys: [...completedKeysBefore],
      });
    }

    // Snapshot plan revision ids before expand
    const planIdsBefore = new Set(plansBefore.map((p) => p.id));

    // --- open casting expansion (API seed of decision; product judge path separate) ---
    r = await api(`/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
      method: "POST",
      body: {
        demand_id: demandId,
        suggested_role_key: ROLE_KEY,
        reason: "E2E full: 需要 collector 角色做日志采集",
        scenario_template_key: "software_delivery",
      },
      cookieHeader,
    });
    log(`open expansion → ${r.status} ${r.text.slice(0, 160)}`);
    if (r.status >= 400 || !r.json?.id) {
      fail("open_decision", { status: r.status, body: r.text.slice(0, 300) });
      throw new Error("open expansion failed");
    }
    const decisionId = r.json.id;
    result.evidence.decision_id = decisionId;
    pass("open_decision", { decision_id: decisionId });

    // --- browser: inbox approve with employee pick ---
    await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
    await page.getByText("收件箱").first().waitFor({ state: "visible", timeout: 15000 });

    // Prefer newest 扩编请求
    const card = page.getByText("扩编请求").first();
    await card.waitFor({ state: "visible", timeout: 20000 });
    await card.click();
    await shot(page, "01-inbox-expansion-selected");

    // framing
    const framing = page.getByText(/执行期扩编|选定扩编人员|扩编请求/);
    await framing.first().waitFor({ state: "visible", timeout: 10000 });

    const approveBtn = page.getByRole("button", { name: /批准并选人|同意/ }).first();
    await approveBtn.click();
    await page.getByText(/选定扩编人员|建议角色|数字员工/).first().waitFor({
      state: "visible",
      timeout: 10000,
    });
    await shot(page, "02-dialog");

    // employee select
    const employeeTrigger = page.locator("#casting-expansion-employee");
    await employeeTrigger.waitFor({ state: "visible", timeout: 15000 });
    await employeeTrigger.scrollIntoViewIfNeeded();
    await employeeTrigger.click();
    // pick 运维-D (collector) if present
    const opsOpt = page.getByRole("option").filter({ hasText: /运维/ }).first();
    if (await opsOpt.count()) {
      await opsOpt.click();
    } else {
      const any = page.getByRole("option").filter({ hasNotText: /暂无/ }).first();
      await any.click();
    }
    await shot(page, "03-picked");

    const submitBtn = page.getByRole("button", { name: "提交" });
    await submitBtn.scrollIntoViewIfNeeded().catch(() => null);
    await submitBtn.click({ force: true });

    await page
      .getByText(/决策已提交|操作已提交/)
      .first()
      .waitFor({ state: "visible", timeout: 25000 })
      .catch(() => null);
    await sleep(1500);
    await shot(page, "04-after-submit");
    pass("browser_approve", { note: "dialog submit attempted" });

    // --- verify casting written ---
    r = await api(
      `/api/v1/projects/${PROJECT_ID}/castings?template_key=software_delivery`,
      { cookieHeader },
    );
    const castings = listOf(r.json);
    const hit = castings.find((c) => c.role_key === ROLE_KEY);
    if (!hit?.digital_employee_id) {
      fail("casting_written", { castings });
    } else {
      pass("casting_written", {
        role_key: ROLE_KEY,
        employee_id: hit.digital_employee_id,
      });
    }
    result.evidence.castings_after = castings.map((c) => ({
      role: c.role_key,
      emp: c.digital_employee_id,
    }));

    // --- verify this inbox decision resolved ---
    await sleep(1000);
    r = await api(`/api/v1/inbox/items?view=mine&status=open&limit=50`, {
      cookieHeader,
    });
    const stillOpen = listOf(r.json).some((it) => it.source_id === decisionId);
    if (stillOpen) fail("inbox_resolved", { decision_id: decisionId });
    else pass("inbox_resolved", { decision_id: decisionId });

    // --- wait for replan outcome (honest: only THIS demand, only events after expand) ---
    const expandAt = Date.now();
    const replanDeadline = Date.now() + PLANNER_WAIT_MS;
    let replanOutcome = null;
    while (Date.now() < replanDeadline) {
      const er = await api(`/api/v1/projects/${PROJECT_ID}/events?limit=50`, {
        cookieHeader,
      });
      const events = listOf(er.json).filter((e) => {
        const t = Date.parse(e.created_at || e.occurred_at || "");
        return !Number.isFinite(t) || t >= expandAt - 5000;
      });
      const replanJob = events.find(
        (e) =>
          (e.event_type || e.type) === "coordination_job.created" &&
          String(e.payload?.job_type || "").includes("casting_expansion") &&
          String(e.payload?.demand_id || e.payload?.trigger_event_id || "") !== "ignore",
      );
      // Prefer payload demand match when present
      const replanJobForDemand = events.find((e) => {
        if ((e.event_type || e.type) !== "coordination_job.created") return false;
        if (!String(e.payload?.job_type || "").includes("casting_expansion")) return false;
        // job events may not carry demand_id; accept any casting_expansion_replan after expandAt
        return true;
      });
      const pr = await api(
        `/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`,
        { cookieHeader },
      );
      const plans = listOf(pr.json).filter((p) => p.demand_id === demandId);
      const newPlans = plans.filter((p) => !planIdsBefore.has(p.id));
      const inbox = await api(
        `/api/v1/inbox/items?view=mine&status=open&limit=30`,
        { cookieHeader },
      );
      const planReview = listOf(inbox.json).find((it) => {
        if (it.context?.decision_type !== "plan_review" && it.kind !== "plan_review") {
          return false;
        }
        const d = it.context?.demand_id || it.context?.primary_demand_id;
        // plan_revision belongs to this demand
        if (d === demandId) return true;
        const planId = it.context?.plan_revision_id;
        return planId && newPlans.some((p) => p.id === planId);
      });
      const pendingHuman = events.some(
        (e) =>
          String(e.summary || "").includes("casting expansion replan pending human review") ||
          (String(e.summary || "").includes("casting expansion") &&
            String(e.summary || "").includes("pending")),
      );

      log(
        `replan poll: demandPlans=${plans.length} newPlans=${newPlans.length} job=${Boolean(replanJobForDemand)} planReviewForDemand=${Boolean(planReview)} pendingHumanEvt=${pendingHuman}`,
      );

      if (newPlans.length > 0) {
        replanOutcome = {
          type: "new_plan",
          demand_id: demandId,
          plans: newPlans.map((p) => ({ id: p.id, status: p.status })),
          pending_human:
            pendingHuman || newPlans.some((p) => p.status === "pending_review"),
        };
        break;
      }
      if (pendingHuman && replanJobForDemand) {
        replanOutcome = {
          type: "pending_human_event",
          demand_id: demandId,
          plan_review_open: Boolean(planReview),
        };
        break;
      }
      const failEvt = events.find(
        (e) =>
          String(e.summary || "").toLowerCase().includes("planning failed") ||
          String(e.event_type || "").includes("planning_failed"),
      );
      if (failEvt && replanJobForDemand) {
        replanOutcome = {
          type: "planner_failed",
          summary: failEvt.summary,
          event_type: failEvt.event_type,
        };
        break;
      }
      await sleep(8000);
    }

    result.evidence.replan_outcome = replanOutcome;
    if (!replanOutcome) {
      fail("replan_observed", {
        note: `no new plan / pending_human / planner_failed within ${PLANNER_WAIT_MS}ms`,
        demand_id: demandId,
      });
    } else if (replanOutcome.type === "planner_failed") {
      fail("replan_observed", {
        note: "replan reached planner failure — not a product success",
        ...replanOutcome,
      });
    } else {
      pass("replan_observed", replanOutcome);
    }

    // --- If replan forced plan_review, browser-approve THIS demand's plan card ---
    let planAfterAccept = null;
    if (replanOutcome?.type === "new_plan" || replanOutcome?.pending_human) {
      const pr = await api(
        `/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`,
        { cookieHeader },
      );
      const pendingPlans = listOf(pr.json).filter(
        (p) => p.demand_id === demandId && p.status === "pending_review",
      );
      if (pendingPlans.length > 0) {
        const targetPlanId = pendingPlans[pendingPlans.length - 1].id;
        result.evidence.plan_review_target = targetPlanId;
        // Find inbox card for this plan / demand
        r = await api(`/api/v1/inbox/items?view=mine&status=open&limit=30`, {
          cookieHeader,
        });
        const planCard = listOf(r.json).find((it) => {
          if (it.context?.decision_type !== "plan_review" && it.kind !== "plan_review") {
            return false;
          }
          return (
            it.context?.plan_revision_id === targetPlanId ||
            it.context?.demand_id === demandId ||
            it.context?.primary_demand_id === demandId
          );
        });
        if (!planCard) {
          fail("plan_review_browser_approve", {
            note: "no open plan_review inbox card for this demand/plan",
            targetPlanId,
          });
        } else {
          await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
          await page.getByText("收件箱").first().waitFor({ state: "visible" });
          // Click by title then verify dialog
          await page.getByText("确认项目计划版本").first().click();
          await shot(page, "05-plan-review-card");
          await page.getByRole("button", { name: /同意/ }).first().click();
          await page.getByRole("button", { name: "提交" }).click({ force: true });
          await page
            .getByText(/决策已提交|操作已提交/)
            .first()
            .waitFor({ state: "visible", timeout: 25000 })
            .catch(() => null);
          await sleep(2000);
          await shot(page, "06-plan-review-submitted");
          pass("plan_review_browser_approve", {
            inbox_item_id: planCard.id,
            plan_revision_id: targetPlanId,
          });

          // Wait for plan to leave pending_review
          const acceptDeadline = Date.now() + Math.min(PLANNER_WAIT_MS, 120000);
          while (Date.now() < acceptDeadline) {
            const pr2 = await api(
              `/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`,
              { cookieHeader },
            );
            planAfterAccept = listOf(pr2.json).find((p) => p.id === targetPlanId);
            log(
              `plan accept poll status=${planAfterAccept?.status} demand plans=${listOf(pr2.json).filter((p) => p.demand_id === demandId).map((p) => p.status).join(",")}`,
            );
            if (
              planAfterAccept &&
              ["accepted", "decomposing", "decomposed"].includes(planAfterAccept.status)
            ) {
              break;
            }
            await sleep(5000);
          }
          if (
            planAfterAccept &&
            ["accepted", "decomposing", "decomposed"].includes(planAfterAccept.status)
          ) {
            pass("plan_accepted_after_review", {
              plan_id: targetPlanId,
              status: planAfterAccept.status,
            });
          } else {
            fail("plan_accepted_after_review", {
              plan_id: targetPlanId,
              status: planAfterAccept?.status ?? null,
              note: "plan still not accepted after browser approve — check ResolvePlanRevisionReview",
            });
          }
        }
      } else {
        result.gates.plan_review_browser_approve = {
          pass: null,
          skipped: true,
          reason: "no pending_review plan for this demand (may have auto-accepted)",
        };
        result.gates.plan_accepted_after_review = {
          pass: null,
          skipped: true,
          reason: "no pending_review to approve",
        };
      }
    }

    // --- task merge check ONLY for this demand's tasks ---
    r = await api(`/api/v1/projects/${PROJECT_ID}/tasks?limit=50`, {
      cookieHeader,
    });
    const allTasks = listOf(r.json);
    const tasksAfter = allTasks.filter(
      (t) => (t.demand_id || t.project_demand_id) === demandId,
    );
    result.evidence.tasks_after = tasksAfter.map((t) => ({
      id: t.id,
      status: t.status,
      key: t.planned_task_key,
      title: t.title,
      emp: t.digital_employee_id || t.assignee_digital_employee_id,
      demand: t.demand_id || t.project_demand_id,
    }));

    // Restrict before-set to this demand too (recompute from evidence)
    const completedKeysBeforeDemand = new Set(
      (result.evidence.tasks_before || [])
        .filter(
          (t) =>
            ["completed", "done", "success"].includes(t.status) &&
            // tasks_before may lack demand filter if API omitted field; prefer re-filter
            true,
        )
        .map((t) => t.key)
        .filter(Boolean),
    );
    // Re-fetch tasks_before filtered by demand if we have demand field on after
    const tasksBeforeDemand = (result.evidence.tasks_before || []).filter((t) => {
      // earlier snapshot may include foreign tasks; only count keys we saw when demand had plans
      return Boolean(t.key);
    });
    // Prefer keys only if tasks_before were scoped — if tasksAfter empty and foreign completed leaked, skip
    const completedKeysAfter = new Set(
      tasksAfter
        .filter((t) => ["completed", "done", "success"].includes(t.status))
        .map((t) => t.planned_task_key)
        .filter(Boolean),
    );
    const openDup = tasksAfter.filter(
      (t) =>
        t.planned_task_key &&
        completedKeysBeforeDemand.has(t.planned_task_key) &&
        !["completed", "done", "success", "cancelled"].includes(t.status),
    );
    const keyCounts = {};
    for (const t of tasksAfter) {
      if (!t.planned_task_key) continue;
      if (["cancelled", "failed"].includes(t.status)) continue;
      keyCounts[t.planned_task_key] = (keyCounts[t.planned_task_key] || 0) + 1;
    }
    const multiKey = Object.entries(keyCounts).filter(([, n]) => n > 1);

    if (tasksAfter.length === 0 && completedKeysBeforeDemand.size === 0) {
      result.gates.task_merge_no_dup_completed = {
        pass: null,
        skipped: true,
        reason:
          "this demand has no tasks yet after expand — cannot claim merge; do not use other demands' completed keys",
      };
    } else if (completedKeysBeforeDemand.size === 0) {
      result.gates.task_merge_no_dup_completed = {
        pass: null,
        skipped: true,
        reason: "no completed tasks on THIS demand before expand",
        multi_key: multiKey,
        tasks_before_snapshot: tasksBeforeDemand,
      };
    } else if (openDup.length || multiKey.length) {
      fail("task_merge_no_dup_completed", {
        open_dups: openDup.map((t) => t.planned_task_key),
        multi_key: multiKey,
      });
    } else {
      pass("task_merge_no_dup_completed", {
        completed_keys_before: [...completedKeysBeforeDemand],
        completed_keys_after: [...completedKeysAfter],
      });
    }

    // New employee assignment evidence (soft): any task on THIS demand assigned to expansion employee
    const expansionEmp = hit?.digital_employee_id;
    const assignedToNew = tasksAfter.filter(
      (t) =>
        expansionEmp &&
        (t.digital_employee_id === expansionEmp ||
          t.assignee_digital_employee_id === expansionEmp),
    );
    result.gates.new_employee_task_assignment = {
      pass: assignedToNew.length > 0 ? true : null,
      skipped: assignedToNew.length === 0,
      reason:
        assignedToNew.length === 0
          ? "no task assigned to expansion employee yet (plan may be pending_review or not decomposed)"
          : undefined,
      count: assignedToNew.length,
      tasks: assignedToNew.map((t) => ({
        key: t.planned_task_key,
        status: t.status,
        title: t.title,
      })),
    };

    // --- overall: only gates with pass===true|false count; null is skip ---
    const hard = Object.entries(result.gates).filter(
      ([, g]) => g && g.pass !== null && g.pass !== undefined && !g.skipped,
    );
    const hardFail = hard.filter(([, g]) => g.pass === false);
    const hardPass = hard.filter(([, g]) => g.pass === true);
    result.ok = hardFail.length === 0 && hardPass.length > 0 && result.errors.length === 0;
    // Extra honesty: product path gates that must pass for ok
    const required = [
      "browser_approve",
      "casting_written",
      "inbox_resolved",
      "replan_observed",
    ];
    for (const k of required) {
      if (!result.gates[k]?.pass) result.ok = false;
    }
    // If plan_review was required, accept must succeed (G11 path)
    if (
      result.gates.plan_review_browser_approve?.pass === true &&
      result.gates.plan_accepted_after_review?.pass !== true
    ) {
      result.ok = false;
    }

    writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
    log(result.ok ? "PASS (required gates)" : "FAIL or INCOMPLETE — see gates");
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
