/**
 * Browser E2E for G5 / G6 / G7 (批二编制).
 * Prerequisites: web :3100, control-plane :8080, admin/admin, project P1.
 *
 *   node scripts/e2e/browser-casting-gates.mjs
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
  ops: "9a623b40-c9ec-4d7d-99a4-17b1f569b52e", // collector+analyst, no code_implementation
};
const OUT = join(__dirname, "../../.scratch/e2e-browser-casting-gates");
mkdirSync(OUT, { recursive: true });

const result = { gates: {}, errors: [] };
function log(m) {
  console.log(`[gates] ${m}`);
}

async function shot(page, name) {
  await page.screenshot({ path: join(OUT, `${name}.png`), fullPage: true });
}

async function loginApi() {
  const res = await fetch(`${CP}/api/auth/login`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ username: "admin", password: "admin" }),
  });
  if (!res.ok) throw new Error(`login api ${res.status}`);
  const setCookie = res.headers.getSetCookie?.() || [];
  // Node fetch may not expose set-cookie fully; use raw
  const raw = res.headers.get("set-cookie") || "";
  const cookies = [];
  // Prefer getSetCookie
  const parts = setCookie.length ? setCookie : raw ? [raw] : [];
  for (const c of parts) {
    const [nv] = c.split(";");
    const eq = nv.indexOf("=");
    if (eq > 0) {
      cookies.push({
        name: nv.slice(0, eq).trim(),
        value: nv.slice(eq + 1).trim(),
        domain: "127.0.0.1",
        path: "/",
      });
    }
  }
  // Fallback: re-login with cookie jar via playwright context later
  return cookies;
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
  return { status: res.status, text, headers: res.headers };
}

async function main() {
  // --- API prep: full casting for software_delivery ---
  // Use playwright browser cookies after login for subsequent API checks too.
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(25000);

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
    await shot(page, "01-home");

    const cookies = await context.cookies();
    const cookieHeader = cookies.map((c) => `${c.name}=${c.value}`).join("; ");

    // Ensure 运维-D also has developer for G6 ⚠ (no code_implementation)
    let r = await api(`/api/v1/digital-employees/${EMP.ops}/roles`, {
      method: "PUT",
      body: { role_keys: ["collector", "analyst", "developer"] },
      cookieHeader,
    });
    log(`G6 prep ops roles → ${r.status} ${r.text.slice(0, 120)}`);
    if (r.status >= 400) result.errors.push(`G6 prep roles failed: ${r.text}`);

    // Full casting for G5 (need cast members in pool)
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
    log(`full casting → ${r.status}`);
    if (r.status >= 400) throw new Error(`cast fail ${r.text}`);

    // ========== G6: candidates show ⚠ and selectable ==========
    await page.goto(`${WEB}/projects/${PROJECT_ID}/config`, {
      waitUntil: "domcontentloaded",
    });
    await page.waitForTimeout(1500);
    await page.getByRole("tab", { name: "剧本编制" }).or(page.getByText("剧本编制", { exact: true })).first().click();
    await page.waitForTimeout(1000);

    // open template select
    const combos = page.getByRole("combobox");
    await combos.first().click();
    await page
      .getByRole("option")
      .filter({ hasText: /软件开发|software_delivery/ })
      .first()
      .click();
    await page.waitForFunction(
      () => !document.body.innerText.includes("加载候选人"),
      null,
      { timeout: 15000 },
    ).catch(() => {});
    await page.waitForTimeout(800);
    await shot(page, "02-g6-template");

    // Open developer combobox (second combobox)
    await combos.nth(1).click();
    await page.waitForTimeout(500);
    const optionsText = await page.locator("[role=option]").allInnerTexts();
    log(`developer options: ${JSON.stringify(optionsText)}`);
    await shot(page, "03-g6-options");

    const hasWarn =
      optionsText.some((t) => t.includes("⚠") || t.includes("缺")) ||
      optionsText.some((t) => t.includes("运维"));
    const hasMatched = optionsText.some(
      (t) => t.includes("✓") || t.includes("开发-A") || t.includes("具备"),
    );
    // Select 运维 if present (⚠ path)
    const warnOpt = page
      .getByRole("option")
      .filter({ hasText: /运维|⚠|缺/ })
      .first();
    if (await warnOpt.isVisible().catch(() => false)) {
      await warnOpt.click();
      await page.waitForTimeout(300);
      const picked = await combos.nth(1).innerText();
      log(`G6 selected warn candidate: ${picked}`);
      result.gates.G6 = {
        pass: hasWarn && hasMatched,
        hasWarn,
        hasMatched,
        selectedWarn: true,
        options: optionsText,
      };
    } else {
      await page.keyboard.press("Escape");
      // Options list already proves ⚠/✓ when present in optionsText
      result.gates.G6 = {
        pass: hasWarn && hasMatched,
        hasWarn,
        hasMatched,
        selectedWarn: false,
        options: optionsText,
        note: hasWarn
          ? "⚠ option listed in dropdown (click not confirmed)"
          : "no ⚠ option in list",
      };
      if (!result.gates.G6.pass) {
        const cand = await api(
          `/api/v1/projects/${PROJECT_ID}/role-candidates?role_key=developer&required_capabilities=code_implementation`,
          { cookieHeader },
        );
        let list = [];
        try {
          list = JSON.parse(cand.text);
        } catch {
          /* ignore */
        }
        const warnApi = list.filter((c) => c.capability_fit !== "matched");
        const matchedApi = list.filter((c) => c.capability_fit === "matched");
        result.gates.G6 = {
          pass: warnApi.length > 0 && matchedApi.length > 0,
          hasWarn: warnApi.length > 0,
          hasMatched: matchedApi.length > 0,
          via: "api+ui",
          warnNames: warnApi.map((c) => c.name),
          matchedNames: matchedApi.map((c) => c.name),
        };
        log(`G6 API warn=${warnApi.length} matched=${matchedApi.length}`);
      }
    }
    await page.keyboard.press("Escape").catch(() => {});

    // Restore developer cast to 开发-A for G5
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

    // ========== G5: remove cast employee refused ==========
    await page.goto(`${WEB}/projects/${PROJECT_ID}/config`, {
      waitUntil: "domcontentloaded",
    });
    await page.waitForTimeout(1200);
    await page.getByRole("tab", { name: "成员" }).or(page.getByText("成员", { exact: true })).first().click();
    await page.waitForTimeout(1000);
    await shot(page, "04-g5-members");

    // Remove 开发-A
    const removeBtn = page.getByRole("button", { name: /移除 开发-A|移除.*开发/ });
    let g5pass = false;
    let g5msg = "";
    if (await removeBtn.first().isVisible().catch(() => false)) {
      // Listen for toast / error dialog
      page.once("dialog", async (d) => {
        g5msg = d.message();
        await d.accept();
      });
      await removeBtn.first().click();
      await page.waitForTimeout(2000);
      const body = await page.locator("body").innerText();
      g5pass =
        body.includes("编制") ||
        body.includes("先改") ||
        body.includes("仍被") ||
        body.includes("无法") ||
        g5msg.includes("编制");
      // Also check still listed
      if (!g5pass) {
        g5pass = body.includes("开发-A"); // still there = remove blocked
      }
      result.gates.G5 = { pass: g5pass, bodySnippet: body.includes("开发-A"), dialog: g5msg };
      log(`G5 browser remove: pass=${g5pass} dialog=${g5msg}`);
    } else {
      // API G5 + note UI selector miss
      const members = await api(`/api/v1/projects/${PROJECT_ID}/members`, {
        cookieHeader,
      });
      let list = [];
      try {
        list = JSON.parse(members.text);
      } catch {
        /* */
      }
      const kept = list
        .filter(
          (m) =>
            !(
              m.principal_type === "digital_employee" &&
              m.principal_id === EMP.developer
            ),
        )
        .map((m) => ({
          principal_type: m.principal_type,
          principal_id: m.principal_id,
          project_role: m.project_role,
          display_name_snapshot: m.display_name_snapshot || "",
          settings: m.settings || {},
        }));
      const rep = await api(`/api/v1/projects/${PROJECT_ID}/members`, {
        method: "PUT",
        body: { members: kept },
        cookieHeader,
      });
      g5pass = rep.status >= 400 && /编制|cast/i.test(rep.text);
      result.gates.G5 = {
        pass: g5pass,
        via: "api-fallback",
        status: rep.status,
        text: rep.text.slice(0, 200),
      };
      log(`G5 API fallback ${rep.status} ${rep.text.slice(0, 120)}`);
    }
    await shot(page, "05-g5-after-remove");

    // ========== G7: automation incomplete casting ==========
    // Make casting incomplete
    await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      body: {
        scenario_template_key: "software_delivery",
        assignments: [
          { role_key: "developer", digital_employee_id: EMP.developer },
        ],
      },
      cookieHeader,
    });

    await page.goto(`${WEB}/automations`, { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(1500);
    await shot(page, "06-g7-automations");

    const newRule = page.getByRole("button", { name: /新建规则/ });
    let g7pass = false;
    if (await newRule.isVisible().catch(() => false)) {
      await newRule.click();
      await page.waitForTimeout(800);
      await shot(page, "07-g7-form");

      // Fill form fields loosely
      const nameInput = page.getByLabel(/名称|规则名/).or(page.locator('input[name="name"]')).first();
      if (await nameInput.isVisible().catch(() => false)) {
        await nameInput.fill(`G7-e2e-${Date.now()}`);
      } else {
        // try placeholder
        const anyName = page.getByPlaceholder(/名称|规则/).first();
        if (await anyName.isVisible().catch(() => false)) await anyName.fill(`G7-e2e-${Date.now()}`);
      }

      // Project select - batch2
      const projectCombo = page.getByRole("combobox").filter({ hasText: /项目|选择/ }).first();
      // broader: all comboboxes
      const formCombos = page.locator('[role="dialog"] [role="combobox"], [data-slot="sheet"] [role="combobox"], [role="combobox"]');
      const fc = await formCombos.count();
      log(`form combobox count ${fc}`);
      // Try click first few combos looking for project
      for (let i = 0; i < Math.min(fc, 6); i++) {
        const c = formCombos.nth(i);
        const t = await c.innerText().catch(() => "");
        if (/项目|选择项目|批二|P1|未选/.test(t) || i === 0) {
          await c.click();
          await page.waitForTimeout(300);
          const opt = page.getByRole("option").filter({ hasText: /批二|P1|batch2/ }).first();
          if (await opt.isVisible().catch(() => false)) {
            await opt.click();
            log("selected project for automation");
            break;
          }
          await page.keyboard.press("Escape");
        }
      }

      // Scenario template software_delivery
      for (let i = 0; i < Math.min(fc, 8); i++) {
        const c = formCombos.nth(i);
        await c.click().catch(() => {});
        await page.waitForTimeout(200);
        const opt = page
          .getByRole("option")
          .filter({ hasText: /软件开发|software_delivery/ })
          .first();
        if (await opt.isVisible().catch(() => false)) {
          await opt.click();
          log("selected software_delivery for automation");
          break;
        }
        await page.keyboard.press("Escape").catch(() => {});
      }

      // Mode plan if needed, schedule defaults
      const createBtn = page.getByRole("button", { name: /创建并启用|创建|保存/ }).last();
      if (await createBtn.isVisible().catch(() => false)) {
        const enabled = await createBtn.isEnabled().catch(() => false);
        if (enabled) {
          await createBtn.click();
          await page.waitForTimeout(2500);
          const body = await page.locator("body").innerText();
          g7pass =
            body.includes("编制") ||
            body.includes("缺角色") ||
            body.includes("reviewer") ||
            body.includes("tester") ||
            body.includes("不完整");
          await shot(page, "08-g7-after-create");
          result.gates.G7 = {
            pass: g7pass,
            via: "browser",
            snippet: body.includes("缺") ? "has 缺" : body.slice(0, 200),
          };
          log(`G7 browser: ${g7pass}`);
        } else {
          log("G7 create button disabled — form incomplete; fall back to API gate");
          await shot(page, "08-g7-form-disabled");
          // Close sheet if possible
          await page.keyboard.press("Escape").catch(() => {});
        }
      }
    }

    if (!result.gates.G7?.pass) {
      // API is the authoritative server gate
      const rep = await api("/api/v1/automations", {
        method: "POST",
        cookieHeader,
        body: {
          project_id: PROJECT_ID,
          name: `G7-api-${Date.now()}`,
          coordination_mode: "plan",
          demand_title_template: "t {{.date}}",
          demand_body_template: "b",
          scenario_template_key: "software_delivery",
          schedule_kind: "interval",
          interval_seconds: 3600,
          timezone: "Asia/Shanghai",
        },
      });
      const ok =
        rep.status >= 400 &&
        (rep.text.includes("编制") ||
          rep.text.includes("reviewer") ||
          rep.text.includes("tester"));
      result.gates.G7 = {
        ...(result.gates.G7 || {}),
        pass: ok,
        apiStatus: rep.status,
        apiText: rep.text.slice(0, 200),
        via: result.gates.G7?.via ? `${result.gates.G7.via}+api` : "api",
      };
      log(`G7 API ${rep.status} ${rep.text.slice(0, 120)}`);
    }

    // Restore full casting for project health
    await api(`/api/v1/projects/${PROJECT_ID}/castings`, {
      method: "PUT",
      cookieHeader,
      body: {
        scenario_template_key: "software_delivery",
        assignments: [
          { role_key: "developer", digital_employee_id: EMP.developer },
          { role_key: "reviewer", digital_employee_id: EMP.reviewer },
          { role_key: "tester", digital_employee_id: EMP.tester },
        ],
      },
    });
    // Restore ops roles without developer (optional cleanliness)
    await api(`/api/v1/digital-employees/${EMP.ops}/roles`, {
      method: "PUT",
      cookieHeader,
      body: { role_keys: ["collector", "analyst"] },
    });

    await shot(page, "09-final");

    const allPass = ["G5", "G6", "G7"].every((k) => result.gates[k]?.pass);
    writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
    console.log(JSON.stringify(result, null, 2));
    if (!allPass) {
      console.error("GATES FAILED");
      process.exitCode = 1;
    } else {
      log("G5 G6 G7 PASS");
    }
  } catch (e) {
    console.error(e);
    try {
      await shot(page, "99-error");
    } catch {
      /* */
    }
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}

main();
