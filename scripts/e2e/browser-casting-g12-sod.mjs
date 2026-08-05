/**
 * @deprecated Prefer scripts/e2e/browser-casting-design-g10-g11-g12.mjs
 * Early initiate-time cast-lock G12. Design G12 is mid-executing expansion-path.
 *
 * G12 real browser + CP: casting that puts the same employee on a classic
 * role_independence pair must still be blocked (design §7.3 / G12).
 *
 * software_delivery developer+reviewer is migrated to adversarial_review and may
 * allow a shared employee — use incident_response operator+verifier instead
 * (no "review" capability → classic enforceRoleIndependence).
 *
 * Durable product surface (required for PASS):
 *   demand.status === planning_failed | failed
 *   OR open inbox planning_failed / planning_gap for this demand
 *   with diagnosis_raw / payload mentioning role_independence or share employee
 *
 * Activity-log-only signals are recorded as evidence but do NOT alone pass.
 *
 *   node scripts/e2e/browser-casting-g12-sod.mjs
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
  developer: "0be393bb-9dfd-48c8-b010-4b5abb114f23", // operator + verifier (SoD pair)
  reviewer: "7a16f593-9a99-490e-bcab-77bb8b326afa",
  tester: "157b1a2c-b2af-4a08-99f3-f16abe291ed1",
  ops: "9a623b40-c9ec-4d7d-99a4-17b1f569b52e", // diagnostician only
};
// Cast-locked SoD escalates to planning_gap (ErrNoSuitableEmployee) without
// burning 3× planner retries; still allow headroom for planner latency.
const WAIT_MS = Number(process.env.SUPERTEAM_PLANNER_WAIT_MS || 300000);
const OUT = join(__dirname, "../../.scratch/e2e-browser-casting-g12-sod");
mkdirSync(OUT, { recursive: true });

const result = { ok: false, gates: {}, errors: [], evidence: {}, timeline: [] };
function log(m) {
  console.log(`[g12] ${m}`);
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
  return json?.items || json?.demands || json?.events || json?.revisions || [];
}

function sodMarker(blob) {
  const s = String(blob || "");
  return (
    s.includes("role_independence") ||
    s.includes("share employee") ||
    (s.includes("constraint") && s.includes("operator") && s.includes("verifier"))
  );
}

function forDemand(it, demandId) {
  if (!demandId) return false;
  const ctx = it?.context || {};
  // Strict: only this demand. Never match by title substring (would steal older G12 cards).
  return ctx.demand_id === demandId || it?.demand_id === demandId;
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
    // --- real browser login ---
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
    log(`browser login ok ${page.url()}`);
    await shot(page, "00-home");
    pass("browser_login", { url: page.url() });

    const cookies = await context.cookies();
    const cookieHeader = cookies.map((c) => `${c.name}=${c.value}`).join("; ");

    // Role setup: only developer holds operator+verifier (SoD pair)
    for (const [id, roles] of [
      [EMP.developer, ["developer", "diagnostician", "operator", "verifier"]],
      [EMP.ops, ["collector", "analyst", "diagnostician"]],
      [EMP.reviewer, ["reviewer"]],
      [EMP.tester, ["tester"]],
    ]) {
      const rr = await api(`/api/v1/digital-employees/${id}/roles`, {
        method: "PUT",
        body: { role_keys: roles },
        cookieHeader,
      });
      if (rr.status >= 400) throw new Error(`roles ${id} → ${rr.status} ${rr.text}`);
    }

    // Casting: diagnostician=ops, operator=verifier=developer (same person)
    let r = await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      body: {
        scenario_template_key: "incident_response",
        assignments: [
          { role_key: "diagnostician", digital_employee_id: EMP.ops },
          { role_key: "operator", digital_employee_id: EMP.developer },
          { role_key: "verifier", digital_employee_id: EMP.developer },
        ],
      },
      cookieHeader,
    });
    log(`cast SoD pair → ${r.status}`);
    if (r.status >= 400) {
      fail("cast", { status: r.status, body: r.text.slice(0, 200) });
      throw new Error("cast failed");
    }
    pass("cast_same_operator_verifier", { employee: EMP.developer });

    // Browser: project config casting surface (honest observation)
    await page.goto(`${WEB}/projects/${PROJECT_ID}/config`, {
      waitUntil: "domcontentloaded",
    });
    await sleep(1500);
    await shot(page, "01-project-config");
    const bodyText = await page.locator("body").innerText();
    result.evidence.config_page_has_casting =
      bodyText.includes("编制") || bodyText.includes("角色") || bodyText.includes("剧本");

    // Submit incident demand (API seed; browser observes outcome)
    const title = `G12 SoD ${new Date().toISOString().slice(11, 19)}`;
    r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, {
      method: "POST",
      body: {
        title,
        content:
          "需要完整故障排查：诊断根因、实施修复、并由独立角色验证修复结果（verification_result 收口）",
        scenario_template_key: "incident_response",
        coordination_mode: "plan",
      },
      cookieHeader,
    });
    log(`submit demand → ${r.status}`);
    if (r.status >= 400 || !r.json?.id) {
      fail("seed_demand", { status: r.status, body: r.text.slice(0, 200) });
      throw new Error("submit failed");
    }
    const demandId = r.json.id;
    result.evidence.demand_id = demandId;
    result.evidence.title = title;
    pass("seed_demand", { demand_id: demandId });

    // Wait for durable product surface (not activity log alone)
    const deadline = Date.now() + WAIT_MS;
    let outcome = null;
    while (Date.now() < deadline) {
      const dr = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookieHeader });
      const demand = listOf(dr.json).find((d) => d.id === demandId);
      const pr = await api(`/api/v1/projects/${PROJECT_ID}/plan-revisions?limit=20`, {
        cookieHeader,
      });
      const plans = listOf(pr.json).filter((p) => p.demand_id === demandId);

      const inbox = await api(`/api/v1/inbox/items?view=mine&status=open&limit=50`, {
        cookieHeader,
      });
      const items = listOf(inbox.json);
      // Match fail/gap for this demand. planning_gap historically omitted
      // demand_id on inbox context — also accept fresh gap cards with SoD title
      // when demand has just transitioned to failed (same poll window).
      let failCard = items.find(
        (it) =>
          forDemand(it, demandId) &&
          (it.context?.decision_type === "planning_failed" ||
            it.kind === "planning_failed" ||
            it.context?.decision_type === "planning_gap" ||
            it.kind === "planning_gap"),
      );
      if (!failCard && (demand?.status === "failed" || demand?.status === "planning_failed")) {
        failCard = items.find((it) => {
          const isGap =
            it.kind === "planning_gap" || it.context?.decision_type === "planning_gap";
          if (!isGap) return false;
          const blob = `${it.title || ""} ${it.summary || ""} ${JSON.stringify(it.context || {})}`;
          return (
            sodMarker(blob) ||
            blob.includes("职责分离") ||
            blob.includes("独立性") ||
            blob.includes("剧本编制")
          );
        });
      }
      const failBlob = failCard
        ? `${failCard.title} ${JSON.stringify(failCard.context || {})}`
        : "";
      const failHasSod =
        sodMarker(failBlob) ||
        failBlob.includes("职责分离") ||
        failBlob.includes("独立性") ||
        failBlob.includes("剧本编制");

      log(
        `wait demand=${demand?.status} plans=${plans.map((p) => p.status).join(",") || "-"} failCard=${Boolean(failCard)} sodInCard=${failHasSod}`,
      );

      // Illegal: deep plan accepted with same emp on operator+verifier
      for (const p of plans) {
        if (!["accepted", "decomposed", "decomposing"].includes(p.status)) continue;
        const exit = p.payload?.exit_deliverable || "";
        if (exit !== "verification_result" && exit !== "fix_record") continue;
        const tasks = p.payload?.tasks || [];
        const fixEmp =
          tasks.find((t) => /fix|operator|处置/.test(t.planned_task_key || t.title || ""))
            ?.selected_employee_id ||
          tasks.find((t) => (t.produces || []).includes("fix_record"))?.selected_employee_id;
        const verifyEmp =
          tasks.find((t) => /verif|验证/.test(t.planned_task_key || t.title || ""))
            ?.selected_employee_id ||
          tasks.find((t) => (t.produces || []).includes("verification_result"))
            ?.selected_employee_id;
        if (
          exit === "verification_result" &&
          fixEmp &&
          verifyEmp &&
          fixEmp === verifyEmp &&
          fixEmp === EMP.developer
        ) {
          outcome = {
            type: "illegal_plan",
            plan: p.id,
            status: p.status,
            exit,
            tasks: tasks.map((t) => ({
              key: t.planned_task_key,
              emp: t.selected_employee_id,
            })),
          };
          break;
        }
      }
      if (outcome?.type === "illegal_plan") break;

      // Durable block: demand failed OR fail/gap card with SoD diagnosis
      if (
        demand?.status === "planning_failed" ||
        demand?.status === "failed" ||
        (failCard && failHasSod)
      ) {
        outcome = {
          type: "blocked",
          demand_status: demand?.status,
          fail_card: failCard
            ? {
                id: failCard.id,
                type: failCard.context?.decision_type || failCard.kind,
                title: failCard.title,
                diagnosis: failCard.context?.diagnosis,
                diagnosis_raw: String(failCard.context?.diagnosis_raw || "").slice(0, 400),
                sod_in_card: failHasSod,
              }
            : null,
          plans: plans.map((p) => ({
            id: p.id,
            status: p.status,
            exit: p.payload?.exit_deliverable,
          })),
        };
        // Prefer card with SoD marker; if only status, keep waiting a bit for card
        if (failCard && failHasSod) break;
        if (
          (demand?.status === "planning_failed" || demand?.status === "failed") &&
          failCard
        ) {
          // card without explicit SoD string still counts if demand is planning_failed
          // after SoD cast — but require diagnosis_raw or why to mention invalid route
          const raw = String(failCard.context?.diagnosis_raw || failCard.context?.why || "");
          if (sodMarker(raw) || raw.includes("invalid route") || raw.includes("规划")) {
            outcome.fail_card.sod_in_card = sodMarker(raw);
            break;
          }
        }
        if (demand?.status === "planning_failed" || demand?.status === "failed") {
          // Status alone after forced SoD cast is the F6 surface; accept after one more poll
          await sleep(3000);
          break;
        }
      }

      if (plans.some((p) => p.status === "validation_failed")) {
        outcome = {
          type: "blocked",
          reason: "validation_failed",
          plans: plans.map((p) => ({ id: p.id, status: p.status })),
        };
        break;
      }

      // Auto-approve shallow plan_review for this demand only
      const planCard = items.find(
        (it) =>
          it.context?.decision_type === "plan_review" &&
          (it.context?.demand_id === demandId ||
            plans.some((p) => p.id === it.context?.plan_revision_id)),
      );
      if (planCard && demand?.status === "planning_pending") {
        const ar = await api(`/api/v1/inbox/items/${planCard.id}/actions`, {
          method: "POST",
          body: { action: "approved", comment: "G12 continue", payload: {} },
          cookieHeader,
        });
        log(`plan_review approve → ${ar.status}`);
      }

      await sleep(10000);
    }

    result.evidence.outcome = outcome;

    // --- Browser: open inbox and observe THIS demand's fail/gap card ---
    await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
    await sleep(2500);
    // 异常处理 bucket holds planning_failed / planning_gap
    try {
      const exceptionHdr = page.getByText("异常处理").first();
      if (await exceptionHdr.count()) {
        await exceptionHdr.click().catch(() => {});
        await sleep(500);
      }
    } catch {
      /* ignore */
    }
    await shot(page, "02-inbox");
    let browserSawFail = false;
    let browserCardText = "";
    const titleToken = title; // e.g. "G12 SoD 17:18:16" — unique per run
    try {
      // Prefer demand title (planning_failed · title); else SoD planning_gap copy.
      let card = page.getByText(titleToken, { exact: false }).first();
      if (!(await card.count())) {
        card = page.getByText(/规划缺口|职责分离|剧本编制|独立性约束/).first();
      }
      await card.waitFor({ state: "visible", timeout: 25000 });
      await card.click();
      await sleep(1500);
      await shot(page, "03-fail-card");
      browserCardText = await page.locator("body").innerText();
      const hasTitle = browserCardText.includes(titleToken);
      const hasFailSurface =
        browserCardText.includes("规划失败") ||
        browserCardText.includes("规划缺口") ||
        browserCardText.includes("补员") ||
        browserCardText.includes("独立性") ||
        browserCardText.includes("职责分离") ||
        browserCardText.includes("剧本编制") ||
        browserCardText.includes("role_independence") ||
        browserCardText.includes("重新规划") ||
        browserCardText.includes("豁免");
      // Accept either exact demand title or explicit SoD gap surface for this run.
      browserSawFail = hasFailSurface && (hasTitle || browserCardText.includes("规划缺口"));
      result.evidence.browser_inbox = {
        saw_fail_surface: browserSawFail,
        matched_title: titleToken,
        has_title: hasTitle,
        has_fail_surface: hasFailSurface,
        snippet: browserCardText.slice(0, 1200),
      };
    } catch (e) {
      result.evidence.browser_inbox = {
        saw_fail_surface: false,
        matched_title: titleToken,
        error: String(e.message || e),
      };
      await shot(page, "03-fail-card-miss");
    }

    if (!outcome) {
      fail("g12_sod_blocked", {
        note: "timeout without durable SoD surface (planning_failed/gap/card)",
        wait_ms: WAIT_MS,
      });
    } else if (outcome.type === "illegal_plan") {
      fail("g12_sod_blocked", {
        note: "deep plan accepted with single employee on SoD pair — constraint not enforced",
        ...outcome,
      });
    } else {
      // Require THIS demand planning_failed/failed OR fail card bound to demand_id with SoD marker.
      const cardOk =
        outcome.fail_card &&
        (outcome.fail_card.sod_in_card ||
          sodMarker(outcome.fail_card.diagnosis_raw || ""));
      const statusOk =
        outcome.demand_status === "planning_failed" ||
        outcome.demand_status === "failed";
      if (!(cardOk || statusOk)) {
        fail("g12_sod_blocked", {
          note: "blocked outcome without this-demand planning_failed or SoD fail card",
          ...outcome,
        });
      } else {
        pass("g12_sod_blocked", {
          ...outcome,
          browser_saw_fail: browserSawFail,
          proven_by: cardOk ? "fail_card_sod" : "demand_status",
        });
      }
    }

    if (browserSawFail) {
      pass("browser_planning_failed_surface", {
        demand_id: demandId,
      });
    } else if (result.gates.g12_sod_blocked?.pass) {
      // Honest soft: API durable surface is the gate; browser list can lag/group.
      result.gates.browser_planning_failed_surface = {
        pass: false,
        soft: true,
        note: "API proved SoD block; browser did not show this demand's card",
        evidence: result.evidence.browser_inbox,
      };
    } else {
      fail("browser_planning_failed_surface", {
        note: "no browser fail surface and no API block",
      });
    }

    // PASS only when THIS demand has durable SoD product surface (not stolen cards).
    result.ok = result.gates.g12_sod_blocked?.pass === true;
    writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
    log(result.ok ? "PASS G12" : "FAIL G12");
    console.log(JSON.stringify(result, null, 2));
    if (!result.ok) process.exitCode = 1;
  } catch (e) {
    result.errors.push(String(e.stack || e));
    writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
    console.error(e);
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}
main();
