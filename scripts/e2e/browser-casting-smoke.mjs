/**
 * Real-browser E2E smoke for 批二：角色词表与编制
 * Against running Web :3100 + Control Plane :8080 (admin/admin).
 *
 * Run: node scripts/e2e/browser-casting-smoke.mjs
 */
import { createRequire } from "node:module";
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const require = createRequire(join(__dirname, "../../apps/web/package.json"));
const { chromium } = require("playwright");

const WEB = process.env.SUPERTEAM_WEB_URL || "http://127.0.0.1:3100";
const PROJECT_ID = process.env.SUPERTEAM_PROJECT_ID || "ca82b054-de2d-4810-9a2b-dd41f5e50a2c";
const OUT = join(__dirname, "../../.scratch/e2e-browser-casting");
mkdirSync(OUT, { recursive: true });

const steps = [];
function log(msg) {
  const line = `[e2e] ${msg}`;
  console.log(line);
  steps.push(line);
}

async function shot(page, name) {
  const path = join(OUT, `${name}.png`);
  await page.screenshot({ path, fullPage: true });
  log(`screenshot ${path}`);
}

async function main() {
  const browser = await chromium.launch({
    headless: true,
    // Prefer system cache if PLAYWRIGHT_BROWSERS_PATH unset
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(20000);

  try {
    // --- Login ---
    log(`goto ${WEB}/sign-in`);
    await page.goto(`${WEB}/sign-in`, { waitUntil: "domcontentloaded" });
    await page.getByLabel("账号").waitFor({ state: "visible", timeout: 15000 });
    await shot(page, "01-sign-in");

    // Labels: 账号 / 密码
    await page.getByLabel("账号").fill("admin");
    await page.getByLabel("密码").fill("admin");
    await page.getByRole("button", { name: "登录" }).click();
    // Wait until session is established (not /sign-in or /login).
    await page.waitForFunction(
      () => {
        const p = location.pathname;
        return !p.includes("sign-in") && p !== "/login" && !p.endsWith("/login");
      },
      null,
      { timeout: 30000 },
    );
    // Shell chrome visible
    await page.getByText("项目管理").first().waitFor({ state: "visible", timeout: 15000 });
    log(`logged in → ${page.url()}`);
    await shot(page, "02-after-login");

    // --- Project config: 剧本编制 ---
    const configUrl = `${WEB}/projects/${PROJECT_ID}/config`;
    log(`goto ${configUrl}`);
    await page.goto(configUrl, { waitUntil: "domcontentloaded", timeout: 60000 });
    await page.waitForTimeout(2500);
    // If auth bounced back to login, fail clearly
    if (page.url().includes("login") || page.url().includes("sign-in")) {
      throw new Error(`not authenticated after navigation: ${page.url()}`);
    }
    await shot(page, "03-project-config");

    // Tab 剧本编制 (SoftTabs uses Radix TabsTrigger → role=tab)
    const castingTab = page
      .getByRole("tab", { name: "剧本编制" })
      .or(page.getByText("剧本编制", { exact: true }));
    await castingTab.first().waitFor({ state: "visible", timeout: 20000 });
    await castingTab.first().click();
    await page.waitForTimeout(1500);
    await shot(page, "04-casting-tab");

    // Expect panel title
    const heading = page.getByRole("heading", { name: "剧本编制" }).or(page.getByText("剧本编制", { exact: true }).first());
    await heading.first().waitFor({ state: "visible", timeout: 10000 });
    log("casting panel visible");

    // Readiness lines (may load async)
    await page.waitForTimeout(2000);
    const bodyText = await page.locator("body").innerText();
    const checks = {
      hasCastingPanel: bodyText.includes("剧本编制"),
      hasSoftwareOrIncident:
        bodyText.includes("软件开发") ||
        bodyText.includes("故障排查") ||
        bodyText.includes("software_delivery") ||
        bodyText.includes("incident_response"),
      hasOperatorHint:
        bodyText.includes("operator") ||
        bodyText.includes("修复") ||
        bodyText.includes("缺"),
      hasTemplateSelect: bodyText.includes("场景模板") || bodyText.includes("选择要编制的剧本"),
    };
    log(`readiness/panel checks: ${JSON.stringify(checks)}`);

    // Select software_delivery if combobox present
    const templateCombo = page.getByRole("combobox").first();
    if (await templateCombo.isVisible().catch(() => false)) {
      await templateCombo.click();
      await page.waitForTimeout(400);
      // Prefer software delivery option
      const opt = page.getByRole("option").filter({ hasText: /软件开发|software_delivery/ }).first();
      if (await opt.isVisible().catch(() => false)) {
        await opt.click();
        log("selected software_delivery template");
        await page.waitForTimeout(1500);
        await shot(page, "05-template-selected");

        // Role rows may show developer/reviewer/tester
        const after = await page.locator("body").innerText();
        checks.hasDeveloperRole = after.includes("developer") || after.includes("开发");
        checks.hasReviewerRole = after.includes("reviewer") || after.includes("审查");
        log(`role rows: developer=${checks.hasDeveloperRole} reviewer=${checks.hasReviewerRole}`);

        // Try save casting if button enabled and candidates exist
        const saveBtn = page.getByRole("button", { name: /保存编制/ });
        if (await saveBtn.isVisible().catch(() => false)) {
          // Fill first open select per role if any empty
          const combos = page.getByRole("combobox");
          const n = await combos.count();
          for (let i = 1; i < n; i++) {
            // skip template combo index 0
            const c = combos.nth(i);
            const text = (await c.innerText().catch(() => "")).trim();
            if (!text || text.includes("选择") || text.includes("加载")) {
              await c.click();
              await page.waitForTimeout(300);
              const firstOpt = page.getByRole("option").filter({ hasNotText: /暂无/ }).first();
              if (await firstOpt.isVisible().catch(() => false)) {
                await firstOpt.click();
                await page.waitForTimeout(200);
              } else {
                await page.keyboard.press("Escape");
              }
            }
          }
          await shot(page, "06-roles-picked");
          // Wait for candidate dropdowns to leave "加载候选人"
          await page.waitForFunction(() => {
            const t = document.body.innerText;
            return !t.includes("加载候选人");
          }, null, { timeout: 15000 }).catch(() => {});
          if (await saveBtn.isEnabled()) {
            await saveBtn.click();
            await page.waitForTimeout(2500);
            log("clicked 保存编制");
            await shot(page, "07-after-save-casting");
            const afterSave = await page.locator("body").innerText();
            if (afterSave.includes("Cannot read") || afterSave.includes("request failed") || afterSave.includes("replace")) {
              throw new Error("save casting UI error: " + afterSave.match(/.{0,40}(Cannot|request failed|replace).{0,80}/)?.[0]);
            }
            checks.savedCasting = true;
            checks.noSaveError = true;
          } else {
            log("保存编制 disabled (incomplete picks?)");
            checks.savedCasting = false;
          }
        }
      } else {
        await page.keyboard.press("Escape");
        log("software_delivery option not found in list");
      }
    }

    // --- Members tab: verify pool may include cast employees ---
    const membersTab = page.getByRole("tab", { name: "成员" });
    if (await membersTab.isVisible().catch(() => false)) {
      await membersTab.click();
      await page.waitForTimeout(1000);
      await shot(page, "08-members-tab");
      const membersText = await page.locator("body").innerText();
      checks.membersShowDeveloper =
        membersText.includes("开发-A") || membersText.includes("开发");
      log(`members show developer-ish: ${checks.membersShowDeveloper}`);
    }

    // --- API cross-check via page.evaluate fetch (same origin cookies may not apply to :8080) ---
    // Use browser fetch to CP with credentials if CORS allows; else skip.
    const readiness = await page.evaluate(async (projectId) => {
      try {
        const r = await fetch(`http://127.0.0.1:8080/api/v1/projects/${projectId}/playbook-readiness`, {
          credentials: "include",
        });
        return { status: r.status, body: await r.text() };
      } catch (e) {
        return { error: String(e) };
      }
    }, PROJECT_ID);
    log(`browser-fetch readiness: ${JSON.stringify(readiness).slice(0, 400)}`);

    const failed = [];
    if (!checks.hasCastingPanel) failed.push("casting panel missing");
    if (!checks.hasSoftwareOrIncident && !checks.hasTemplateSelect) {
      failed.push("no readiness lines and no template select");
    }

    await shot(page, "09-final");
    if (failed.length) {
      console.error("E2E FAILED:", failed.join("; "));
      console.error("checks:", checks);
      process.exitCode = 1;
    } else {
      log("E2E PASS (browser casting smoke)");
      console.log(JSON.stringify({ ok: true, checks, out: OUT }, null, 2));
    }
  } catch (err) {
    console.error("E2E ERROR:", err);
    try {
      await shot(page, "99-error");
    } catch {
      /* ignore */
    }
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}

main();
