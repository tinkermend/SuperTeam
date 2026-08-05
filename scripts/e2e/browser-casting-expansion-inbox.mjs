/**
 * Browser E2E: casting_expansion inbox card — open decision, pick employee, approve.
 * Seeds via POST /casting-expansions (judge path not required for UI gate).
 *
 *   node scripts/e2e/browser-casting-expansion-inbox.mjs
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
const ROLE_KEY = process.env.SUPERTEAM_EXPANSION_ROLE || "tester";
const OUT = join(__dirname, "../../.scratch/e2e-browser-casting-expansion");
mkdirSync(OUT, { recursive: true });

const result = { gates: {}, errors: [] };
function log(m) {
  console.log(`[expansion-inbox] ${m}`);
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

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(30000);

  try {
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

    const cookies = await context.cookies();
    const cookieHeader = cookies.map((c) => `${c.name}=${c.value}`).join("; ");

    // Ensure ops has target role for candidates list
    let r = await api(`/api/v1/digital-employees/${EMP.ops}/roles`, {
      method: "PUT",
      body: { role_keys: ["collector", "analyst", ROLE_KEY] },
      cookieHeader,
    });
    log(`prep ops roles → ${r.status}`);

    // Pick a demand on the project (prefer executing, else any); seed if empty.
    r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, { cookieHeader });
    let demands = Array.isArray(r.json) ? r.json : r.json?.items ?? r.json?.demands ?? [];
    if (!demands.length) {
      r = await api(`/api/v1/projects/${PROJECT_ID}/demands`, {
        method: "POST",
        body: {
          title: "E2E扩编测试需求",
          content: "用于验证 casting_expansion 收件箱选人批准",
          scenario_template_key: "software_delivery",
          coordination_mode: "plan",
        },
        cookieHeader,
      });
      log(`seed demand → ${r.status} ${r.text.slice(0, 160)}`);
      if (r.status >= 400 || !r.json?.id) {
        throw new Error(`seed demand failed: ${r.status} ${r.text.slice(0, 200)}`);
      }
      demands = [r.json];
    }
    const demand =
      demands.find((d) => d.status === "executing") ||
      demands.find((d) => d.status === "planning_pending" || d.status === "accepted") ||
      demands[0];
    if (!demand?.id) {
      throw new Error(`could not pick demand from ${JSON.stringify(demands).slice(0, 300)}`);
    }
    const demandId = demand.id;
    const templateKey =
      demand.scenario_template_key ||
      demand.scenarioTemplateKey ||
      "software_delivery";
    log(`demand=${demandId} status=${demand.status} template=${templateKey}`);

    // Baseline casting without ROLE_KEY (so approve adds it)
    r = await api(`/api/v1/projects/${PROJECT_ID}/castings?template_key=${encodeURIComponent(templateKey)}`, {
      cookieHeader,
    });
    const existing = Array.isArray(r.json) ? r.json : [];
    const baseline = existing
      .filter((c) => c.role_key !== ROLE_KEY)
      .map((c) => ({
        role_key: c.role_key,
        digital_employee_id: c.digital_employee_id,
      }));
    // keep at least developer if empty
    if (baseline.length === 0) {
      baseline.push({ role_key: "developer", digital_employee_id: EMP.developer });
    }
    r = await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      body: { scenario_template_key: templateKey, assignments: baseline },
      cookieHeader,
    });
    log(`baseline castings → ${r.status} n=${Array.isArray(r.json) ? r.json.length : "?"}`);

    // Open casting expansion decision
    r = await api(`/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
      method: "POST",
      body: {
        demand_id: demandId,
        suggested_role_key: ROLE_KEY,
        reason: "E2E: 执行中途需要测试角色扩编",
        scenario_template_key: templateKey,
      },
      cookieHeader,
    });
    log(`open expansion → ${r.status} ${r.text.slice(0, 200)}`);
    if (r.status >= 400) {
      result.errors.push(`open expansion failed: ${r.text}`);
      throw new Error(`open expansion ${r.status}`);
    }
    const decisionId = r.json?.id;
    result.gates.open_decision = { pass: true, decision_id: decisionId };

    // Inbox UI
    await page.goto(`${WEB}/inbox`, { waitUntil: "domcontentloaded" });
    await page.getByText("收件箱").first().waitFor({ state: "visible", timeout: 15000 });
    // Prefer the new card
    const card = page.getByText("扩编请求").first();
    await card.waitFor({ state: "visible", timeout: 15000 });
    await card.click();
    await shot(page, "01-inbox-card");

    // Action: 批准并选人
    const approveBtn = page.getByRole("button", { name: /批准并选人|同意/ }).first();
    await approveBtn.waitFor({ state: "visible", timeout: 10000 });
    await approveBtn.click();

    // Dialog framing
    await page.getByText(/执行期扩编|选定扩编人员/).first().waitFor({ state: "visible", timeout: 10000 });
    await shot(page, "02-dialog-open");

    // Select employee (role pre-filled from suggested_role_key)
    const employeeTrigger = page.locator("#casting-expansion-employee");
    await employeeTrigger.waitFor({ state: "visible", timeout: 10000 });
    await employeeTrigger.scrollIntoViewIfNeeded();
    await employeeTrigger.click();
    // Prefer 运维-D if listed, else first enabled option
    const option = page.getByRole("option").filter({ hasText: /运维|tester|测试/ }).first();
    if (await option.count()) {
      await option.click();
    } else {
      const any = page.getByRole("option").filter({ hasNotText: /暂无/ }).first();
      await any.click();
    }
    await shot(page, "03-employee-picked");

    // Dialog can be tall (framing + pickers); force submit so footer outside viewport still works.
    const submitBtn = page.getByRole("button", { name: "提交" });
    await submitBtn.scrollIntoViewIfNeeded().catch(() => null);
    await submitBtn.click({ force: true });

    // Wait for success toast or dialog close
    await page
      .getByText(/决策已提交|操作已提交/)
      .first()
      .waitFor({ state: "visible", timeout: 20000 })
      .catch(() => null);
    await page.waitForTimeout(1500);
    await shot(page, "04-after-submit");

    // Verify casting includes ROLE_KEY
    r = await api(
      `/api/v1/projects/${PROJECT_ID}/castings?template_key=${encodeURIComponent(templateKey)}`,
      { cookieHeader },
    );
    const castings = Array.isArray(r.json) ? r.json : [];
    const hit = castings.find((c) => c.role_key === ROLE_KEY);
    const castPass = Boolean(hit?.digital_employee_id);
    result.gates.casting_written = {
      pass: castPass,
      role_key: ROLE_KEY,
      employee_id: hit?.digital_employee_id ?? null,
      castings: castings.map((c) => ({ role: c.role_key, emp: c.digital_employee_id })),
    };
    log(`casting_written pass=${castPass} emp=${hit?.digital_employee_id}`);

    // Inbox item for THIS decision should resolve (ignore leftover cards from prior runs).
    await page.waitForTimeout(800);
    r = await api(`/api/v1/inbox/items?view=mine&status=open&limit=50`, { cookieHeader });
    const openItems = r.json?.items ?? [];
    const stillOpen = openItems.some((it) => it.source_id === decisionId);
    // Also confirm resolved list contains it when open check is clean
    let resolvedHit = false;
    if (!stillOpen) {
      const rr = await api(`/api/v1/inbox/items?view=mine&status=resolved&limit=50`, {
        cookieHeader,
      });
      resolvedHit = (rr.json?.items ?? []).some((it) => it.source_id === decisionId);
    }
    result.gates.inbox_resolved = {
      pass: !stillOpen,
      still_open: stillOpen,
      resolved_hit: resolvedHit,
      decision_id: decisionId,
    };
    log(`inbox_resolved pass=${!stillOpen} resolved_hit=${resolvedHit}`);

    const allPass = Object.values(result.gates).every((g) => g.pass);
    result.ok = allPass && result.errors.length === 0;
    writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
    log(allPass ? "PASS" : "FAIL");
    console.log(JSON.stringify(result, null, 2));
    if (!allPass) process.exitCode = 1;
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
