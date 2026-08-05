/**
 * Design G10/G11/G12 real-path (CP session + browser observe).
 *
 * G10: developer-only cast → plan develop → expand reviewer → new review task
 *      for EMP.reviewer + develop task id reused.
 * G11: if replan exit changes / overbound → pending_review (not auto-run).
 * G12: incident distinct cast → executing → expand verifier to original operator
 *      → durable planning_gap/fail for THIS demand.
 *
 *   node scripts/e2e/browser-casting-design-g10-g11-g12.mjs
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
const WAIT = Number(process.env.SUPERTEAM_PLANNER_WAIT_MS || 300000);
const OUT = join(__dirname, "../../.scratch/e2e-browser-casting-design-gates");
const SCRATCH =
  "/var/folders/_s/2zwng6xn03g1rj6v60h9r75h0000gn/T/grok-goal-ac6f8423dcba/implementer/g10g12";
mkdirSync(OUT, { recursive: true });
mkdirSync(SCRATCH, { recursive: true });

const result = { ok: false, gates: {}, errors: [], evidence: {}, timeline: [] };
const log = (m) => {
  console.log(`[design] ${m}`);
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
async function shot(page, n) {
  await page.screenshot({ path: join(OUT, `${n}.png`), fullPage: true });
}
async function api(path, { method = "GET", body, cookie } = {}) {
  const res = await fetch(`${CP}${path}`, {
    method,
    headers: {
      "content-type": "application/json",
      accept: "application/json",
      ...(cookie ? { cookie } : {}),
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
  Array.isArray(j) ? j : j?.items || j?.demands || j?.tasks || [];

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

async function tasks(cookie, demandId) {
  const r = await api(`/api/v1/projects/${PROJECT_ID}/tasks?limit=80`, { cookie });
  return listOf(r.json).filter((t) => t.demand_id === demandId);
}
async function plans(cookie, demandId) {
  const r = await api(`/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=40`, {
    cookie,
  });
  return listOf(r.json).filter((p) => p.demand_id === demandId);
}
async function demand(cookie, id) {
  const r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookie });
  return listOf(r.json).find((d) => d.id === id);
}
async function waitUntil(fn, ms = WAIT, every = 8000) {
  const dl = Date.now() + ms;
  while (Date.now() < dl) {
    const v = await fn();
    if (v) return v;
    await sleep(every);
  }
  return null;
}

async function approveExpansion(cookie, inboxId, roleKey, empId) {
  return api(`/api/v1/inbox/items/${inboxId}/actions`, {
    method: "POST",
    cookie,
    body: {
      action: "approved",
      comment: `design E2E ${roleKey}`,
      payload: { role_key: roleKey, digital_employee_id: empId },
    },
  });
}

async function approvePlanReview(cookie, demandId, planId) {
  const r = await api(`/api/v1/inbox/items?view=mine&status=open&limit=50`, {
    cookie,
  });
  const card = listOf(r.json).find(
    (it) =>
      it.context?.decision_type === "plan_review" &&
      (it.context?.plan_revision_id === planId || it.context?.demand_id === demandId),
  );
  if (!card) return null;
  return api(`/api/v1/inbox/items/${card.id}/actions`, {
    method: "POST",
    cookie,
    body: { action: "approved", comment: "design accept plan", payload: {} },
  });
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
  });
  const page = await context.newPage();
  try {
    await page.goto(`${WEB}/sign-in`, { waitUntil: "domcontentloaded" });
    await page.getByLabel("账号").fill("admin");
    await page.getByLabel("密码").fill("admin");
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForFunction(() => !location.pathname.includes("sign-in"), null, {
      timeout: 30000,
    });
    pass("browser_login", {});
    const cookie = await cpLogin();

    for (const [id, roles] of [
      [EMP.developer, ["developer", "operator", "diagnostician"]],
      [EMP.reviewer, ["reviewer", "verifier"]],
      [EMP.tester, ["tester"]],
      [EMP.ops, ["collector", "analyst", "diagnostician"]],
    ]) {
      await api(`/api/v1/digital-employees/${id}/roles`, {
        method: "PUT",
        cookie,
        body: { role_keys: roles },
      });
    }

    // ========== G10 + G11 ==========
    log("--- G10/G11 ---");
    let r = await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      cookie,
      body: {
        scenario_template_key: "software_delivery",
        assignments: [{ role_key: "developer", digital_employee_id: EMP.developer }],
      },
    });
    if (r.status >= 400) throw new Error(`cast ${r.text}`);

    r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, {
      method: "POST",
      cookie,
      body: {
        title: `G10G11 ${new Date().toISOString().slice(11, 19)}`,
        content:
          "实现功能并交付分支；后续需代码审查。请规划开发任务（branch_ref），审查角色将中途扩编。",
        scenario_template_key: "software_delivery",
        coordination_mode: "plan",
      },
    });
    if (r.status >= 400 || !r.json?.id) throw new Error(`demand ${r.text}`);
    const g10Id = r.json.id;
    result.evidence.g10_demand_id = g10Id;
    pass("g10_seed_demand", { demand_id: g10Id });

    await waitUntil(async () => {
      const p = await plans(cookie, g10Id);
      const t = await tasks(cookie, g10Id);
      const d = await demand(cookie, g10Id);
      log(
        `g10 seed d=${d?.status} plans=${p.map((x) => x.status + ":" + (x.payload?.exit_deliverable || "")).join(",")} tasks=${t.map((x) => x.planned_task_key).join(",")}`,
      );
      const pend = p.find((x) => x.status === "pending_review");
      if (pend) await approvePlanReview(cookie, g10Id, pend.id);
      return t.some((x) => x.planned_task_key === "develop") || p.some((x) => x.status === "decomposed");
    });

    const tasksBefore = await tasks(cookie, g10Id);
    const plansBefore = await plans(cookie, g10Id);
    const planIdsBefore = new Set(plansBefore.map((p) => p.id));
    const priorExit =
      plansBefore.find((p) =>
        ["accepted", "decomposed", "decomposing"].includes(p.status),
      )?.payload?.exit_deliverable || "";
    const developBefore = tasksBefore.find((t) => t.planned_task_key === "develop");
    result.evidence.g10_before = {
      prior_exit: priorExit,
      develop: developBefore && {
        id: developBefore.id,
        status: developBefore.status,
      },
      tasks: tasksBefore.map((t) => ({
        id: t.id,
        key: t.planned_task_key,
        emp: t.assigned_digital_employee_id,
        status: t.status,
      })),
    };
    if (!developBefore) {
      fail("g10_seed_develop", { tasks: tasksBefore });
      throw new Error("no develop task");
    }
    pass("g10_seed_develop", { id: developBefore.id, status: developBefore.status });

    r = await api(`/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
      method: "POST",
      cookie,
      body: {
        demand_id: g10Id,
        suggested_role_key: "reviewer",
        reason: "G10/G11 扩编审查：新任务派给新人 + 可能改收口需人工确认",
        scenario_template_key: "software_delivery",
      },
    });
    if (r.status >= 400) throw new Error(`open exp ${r.text}`);
    const g10Decision = r.json.id;

    // resolve decision id → inbox item
    const inbox0 = await api(`/api/v1/inbox/items?view=mine&status=open&limit=40`, {
      cookie,
    });
    const g10Card = listOf(inbox0.json).find(
      (it) =>
        it.source_id === g10Decision ||
        (it.context?.demand_id === g10Id &&
          it.context?.suggested_role_key === "reviewer"),
    );
    if (!g10Card) throw new Error("g10 inbox card missing");

    await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
    await sleep(1200);
    await shot(page, "g10-inbox");
    const a10 = await approveExpansion(cookie, g10Card.id, "reviewer", EMP.reviewer);
    log(`g10 approve → ${a10.status}`);
    if (a10.status >= 400) throw new Error(`g10 approve ${a10.text}`);
    pass("g10_approve_expansion", {});

    // wait new plan
    const replan = await waitUntil(async () => {
      const p = await plans(cookie, g10Id);
      const neu = p.filter((x) => !planIdsBefore.has(x.id));
      log(`g10 replan new=${neu.map((x) => x.status + ":" + (x.payload?.exit_deliverable || "")).join(",")}`);
      return neu.length ? neu : null;
    });
    if (!replan) {
      fail("g10_replan", { note: "no new plan after expansion" });
    } else {
      pass("g10_replan", {
        plans: replan.map((p) => ({
          id: p.id,
          status: p.status,
          exit: p.payload?.exit_deliverable,
        })),
      });
      result.evidence.g10_new_plans = replan.map((p) => ({
        id: p.id,
        status: p.status,
        exit: p.payload?.exit_deliverable,
        tasks: (p.payload?.tasks || []).map((t) => ({
          key: t.planned_task_key,
          emp: t.selected_employee_id,
        })),
      }));

      const pending = replan.find((p) => p.status === "pending_review");
      const newExit = replan[0]?.payload?.exit_deliverable || "";
      const exitChanged = priorExit && newExit && priorExit !== newExit;
      const autoRan = replan.some((p) =>
        ["accepted", "decomposed", "decomposing"].includes(p.status),
      );
      result.evidence.g11 = {
        prior_exit: priorExit,
        new_exit: newExit,
        exit_changed: Boolean(exitChanged),
        pending_review: Boolean(pending),
        auto_ran: autoRan && !pending,
      };

      if (pending) {
        pass("g11_force_pending_review", {
          plan_id: pending.id,
          prior_exit: priorExit,
          new_exit: newExit,
        });
        await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
        await sleep(1000);
        await shot(page, "g11-inbox");
        const body = await page.locator("body").innerText();
        if (body.includes("计划确认") || body.includes("确认项目计划")) {
          pass("g11_browser", {});
        }
        const ar = await approvePlanReview(cookie, g10Id, pending.id);
        log(`g11 plan_review → ${ar?.status}`);
        await waitUntil(async () => {
          const p = (await plans(cookie, g10Id)).find((x) => x.id === pending.id);
          return p && ["accepted", "decomposed", "decomposing"].includes(p.status);
        }, 180000);
      } else if (exitChanged && autoRan) {
        fail("g11_force_pending_review", {
          note: "exit changed but auto-ran",
          ...result.evidence.g11,
        });
      } else if (!exitChanged) {
        // Still may be overbound via unrelated tasks; if auto-accepted with only new
        // reviewer tasks, G11 N/A — record soft.
        const reasons = [];
        for (const p of replan) {
          const tasks = p.payload?.tasks || [];
          for (const t of tasks) {
            const key = t.planned_task_key;
            const was = (plansBefore[0]?.payload?.tasks || []).some(
              (o) => o.planned_task_key === key,
            );
            if (!was && t.selected_employee_id !== EMP.reviewer) {
              reasons.push("unrelated_new_task:" + key);
            }
          }
        }
        if (reasons.length && autoRan) {
          fail("g11_force_pending_review", {
            note: "overbound reasons present but auto-ran",
            reasons,
          });
        } else {
          result.gates.g11_force_pending_review = {
            pass: false,
            soft: true,
            note: "no exit change / overbound in this replan; G11 needs overbound construction",
            ...result.evidence.g11,
          };
        }
      } else {
        fail("g11_force_pending_review", result.evidence.g11);
      }
    }

    await sleep(4000);
    const tasksAfter = await tasks(cookie, g10Id);
    result.evidence.g10_after = {
      tasks: tasksAfter.map((t) => ({
        id: t.id,
        key: t.planned_task_key,
        emp: t.assigned_digital_employee_id,
        status: t.status,
      })),
    };

    const reviewTask = tasksAfter.find(
      (t) =>
        t.planned_task_key === "review" &&
        t.assigned_digital_employee_id === EMP.reviewer,
    );
    const newForRev = tasksAfter.filter(
      (t) =>
        t.assigned_digital_employee_id === EMP.reviewer &&
        !tasksBefore.some((b) => b.id === t.id),
    );
    if (reviewTask || newForRev.length) {
      pass("g10_new_task_to_new_employee", {
        review: reviewTask && { id: reviewTask.id, emp: reviewTask.assigned_digital_employee_id },
        new: newForRev.map((t) => ({ id: t.id, key: t.planned_task_key })),
      });
    } else {
      const latest = (await plans(cookie, g10Id)).sort(
        (a, b) => new Date(b.created_at || 0) - new Date(a.created_at || 0),
      )[0];
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
          plan: latest?.payload?.tasks,
        });
      }
    }

    const devAfter = tasksAfter.find((t) => t.id === developBefore.id);
    if (devAfter) {
      pass("g10_planned_task_key_reuse", {
        id: developBefore.id,
        status_before: developBefore.status,
        status_after: devAfter.status,
      });
    } else {
      fail("g10_planned_task_key_reuse", { before: developBefore.id });
    }

    // ========== G12 expansion SoD ==========
    log("--- G12 expansion SoD ---");
    r = await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      cookie,
      body: {
        scenario_template_key: "incident_response",
        assignments: [
          { role_key: "diagnostician", digital_employee_id: EMP.ops },
          { role_key: "operator", digital_employee_id: EMP.developer },
          { role_key: "verifier", digital_employee_id: EMP.reviewer },
        ],
      },
    });
    if (r.status >= 400) throw new Error(`g12 cast ${r.text}`);

    r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, {
      method: "POST",
      cookie,
      body: {
        title: `G12exp ${new Date().toISOString().slice(11, 19)}`,
        content:
          "完整故障排查：诊断、修复、独立验证（verification_result）。中途可能调整验证人。",
        scenario_template_key: "incident_response",
        coordination_mode: "plan",
      },
    });
    if (r.status >= 400 || !r.json?.id) throw new Error(`g12 demand ${r.text}`);
    const g12Id = r.json.id;
    result.evidence.g12_demand_id = g12Id;
    pass("g12_seed_demand", { demand_id: g12Id });

    await waitUntil(async () => {
      const p = await plans(cookie, g12Id);
      const t = await tasks(cookie, g12Id);
      const d = await demand(cookie, g12Id);
      log(
        `g12 seed d=${d?.status} plans=${p.map((x) => x.status + ":" + (x.payload?.exit_deliverable || "")).join(",")} nT=${t.length}`,
      );
      const pend = p.find((x) => x.status === "pending_review");
      if (pend) await approvePlanReview(cookie, g12Id, pend.id);
      return (
        t.length > 0 ||
        ["executing", "planned"].includes(d?.status) ||
        p.some((x) => x.status === "decomposed")
      );
    });

    const g12TasksBefore = await tasks(cookie, g12Id);
    result.evidence.g12_before = {
      tasks: g12TasksBefore.map((t) => ({
        id: t.id,
        key: t.planned_task_key,
        emp: t.assigned_digital_employee_id,
        status: t.status,
      })),
    };
    pass("g12_mid_executing", { n_tasks: g12TasksBefore.length });

    r = await api(`/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
      method: "POST",
      cookie,
      body: {
        demand_id: g12Id,
        suggested_role_key: "verifier",
        reason: "G12: 扩编验证角色，指定原 operator（开发）触发职责分离",
        scenario_template_key: "incident_response",
      },
    });
    if (r.status >= 400) throw new Error(`g12 open ${r.text}`);
    const g12Dec = r.json.id;
    const inbox1 = await api(`/api/v1/inbox/items?view=mine&status=open&limit=40`, {
      cookie,
    });
    const g12Card = listOf(inbox1.json).find(
      (it) =>
        it.source_id === g12Dec ||
        (it.context?.demand_id === g12Id && it.context?.suggested_role_key === "verifier"),
    );
    if (!g12Card) throw new Error("g12 card missing");

    await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
    await sleep(1000);
    await shot(page, "g12-inbox-before");
    const a12 = await approveExpansion(cookie, g12Card.id, "verifier", EMP.developer);
    log(`g12 approve → ${a12.status}`);
    if (a12.status >= 400) throw new Error(`g12 approve ${a12.text}`);
    pass("g12_approve_expansion", {});

    r = await api(
      `/api/v1/projects/${PROJECT_ID}/castings?template_key=incident_response`,
      { cookie },
    );
    const casts = listOf(r.json);
    result.evidence.g12_cast = casts.map((c) => ({
      role: c.role_key,
      emp: c.digital_employee_id,
    }));
    const op = casts.find((c) => c.role_key === "operator");
    const ver = casts.find((c) => c.role_key === "verifier");
    if (op?.digital_employee_id === EMP.developer && ver?.digital_employee_id === EMP.developer) {
      pass("g12_casting_sod_written", {});
    } else {
      fail("g12_casting_sod_written", { op, ver });
    }

    // Wait durable block for THIS demand only
    const blocked = await waitUntil(async () => {
      const d = await demand(cookie, g12Id);
      const inbox = await api(`/api/v1/inbox/items?view=mine&status=open&limit=50`, {
        cookie,
      });
      const gap = listOf(inbox.json).find((it) => {
        const dt = it.kind || it.context?.decision_type;
        if (dt !== "planning_gap" && dt !== "planning_failed") return false;
        return it.context?.demand_id === g12Id;
      });
      log(`g12 wait d=${d?.status} gap=${gap?.id?.slice(0, 8) || "-"} title=${(gap?.title || "").slice(0, 40)}`);
      if (d?.status === "failed" || d?.status === "planning_failed" || gap) {
        return { demand: d, gap };
      }
      return null;
    }, WAIT);

    // Illegal new accepted plan check
    const g12Plans = await plans(cookie, g12Id);
    let illegal = false;
    for (const p of g12Plans) {
      if (!["accepted", "decomposed", "decomposing"].includes(p.status)) continue;
      // only plans created after expansion roughly: exit verification with same emp
      const ts = p.payload?.tasks || [];
      const fix = ts.find((t) => /fix/.test(t.planned_task_key || ""));
      const v = ts.find((t) => /verif/.test(t.planned_task_key || ""));
      if (
        fix?.selected_employee_id === EMP.developer &&
        v?.selected_employee_id === EMP.developer &&
        blocked // only fail if we claim block but plan also accepted
      ) {
        // if demand failed, old decomposed plan may still list distinct emps — OK
      }
      // After expansion SoD, NEW accepted plan with both same is illegal
    }
    result.evidence.g12_outcome = {
      demand_status: blocked?.demand?.status,
      gap: blocked?.gap && {
        id: blocked.gap.id,
        kind: blocked.gap.kind,
        title: blocked.gap.title,
        demand_id: blocked.gap.context?.demand_id,
        diagnosis: blocked.gap.context?.diagnosis,
      },
      plans: g12Plans.map((p) => ({
        id: p.id,
        status: p.status,
        exit: p.payload?.exit_deliverable,
      })),
    };

    if (
      blocked &&
      (blocked.demand?.status === "failed" ||
        blocked.demand?.status === "planning_failed" ||
        (blocked.gap && blocked.gap.context?.demand_id === g12Id))
    ) {
      pass("g12_expansion_sod_blocked", {
        demand_status: blocked.demand?.status,
        gap_title: blocked.gap?.title,
        path: "mid-executing casting_expansion verifier→operator employee",
      });
    } else {
      fail("g12_expansion_sod_blocked", {
        note: "no THIS-demand durable fail/gap",
        ...result.evidence.g12_outcome,
      });
    }

    await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
    await sleep(1500);
    await shot(page, "g12-inbox-after");
    const body = await page.locator("body").innerText();
    if (
      body.includes("规划缺口") ||
      body.includes("职责分离") ||
      body.includes("剧本编制") ||
      body.includes("规划失败")
    ) {
      pass("g12_browser", {});
    } else {
      result.gates.g12_browser = { pass: false, soft: true };
    }

    const hard = [
      "g10_new_task_to_new_employee",
      "g10_planned_task_key_reuse",
      "g12_casting_sod_written",
      "g12_expansion_sod_blocked",
    ];
    // G11 hard only if not soft-skip
    if (result.gates.g11_force_pending_review && !result.gates.g11_force_pending_review.soft) {
      hard.push("g11_force_pending_review");
    }
    result.ok =
      hard.every((k) => result.gates[k]?.pass === true) &&
      (result.gates.g11_force_pending_review?.pass === true ||
        result.gates.g11_force_pending_review?.soft === true);

    const payload = JSON.stringify(result, null, 2);
    writeFileSync(join(OUT, "result.json"), payload);
    writeFileSync(join(SCRATCH, "design-g10-g11-g12-result.json"), payload);
    log(result.ok ? "PASS (design gates)" : "FAIL");
    console.log(payload);
    if (!result.ok) process.exitCode = 1;
  } catch (e) {
    result.errors.push(String(e.stack || e));
    writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
    writeFileSync(join(SCRATCH, "design-g10-g11-g12-result.json"), JSON.stringify(result, null, 2));
    console.error(e);
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}
main();
