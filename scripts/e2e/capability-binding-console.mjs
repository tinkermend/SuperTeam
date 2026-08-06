/**
 * 能力绑定控制台 GATE（U1–U9），spec `2026-08-06-capability-binding-console-design.md` §8。
 *
 *   node scripts/e2e/capability-binding-console.mjs
 *   node scripts/e2e/capability-binding-console.mjs --project=<uuid>
 *
 * 不进 verify:*——需要真实 Web + CP。改了 Web/CP 代码要先 restart 对应服务。
 */
import { createRequire } from "node:module";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { WEB, CP } from "./lib/cp-client.mjs";
import { launchLoggedIn, SAFE_WAIT_UNTIL } from "./lib/browser.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const require = createRequire(join(__dirname, "../../apps/web/package.json"));
const { chromium } = require("playwright");

const OUT = join(__dirname, "../../.scratch/e2e-capability-binding-console");
mkdirSync(OUT, { recursive: true });

const PROJECT =
  process.argv.find((a) => a.startsWith("--project="))?.slice("--project=".length) ||
  process.env.SUPERTEAM_PROJECT_ID ||
  "56de8016-ce14-43d9-95bf-3fca89849b0a";

const results = [];
const rec = (id, ok, detail) => {
  results.push({ id, ok, detail });
  console.log(`${ok ? "✓" : "✗"} ${id}${detail ? " — " + detail : ""}`);
};

const { browser, page, realErrors } = await launchLoggedIn({ chromium });

try {
  // ---- U1 / U1b：tab + 两区 + 说明条 ----
  await page.goto(`${WEB}/projects/${PROJECT}/config?tab=capabilities`, {
    waitUntil: SAFE_WAIT_UNTIL,
  });
  await page.waitForTimeout(2500);
  const tabVisible = await page
    .getByRole("tab", { name: "能力绑定" })
    .isVisible()
    .catch(() => false);
  let body = await page.locator("body").innerText();
  rec(
    "U1 tab + 技能/MCP 两区",
    tabVisible && /项目技能/.test(body) && /项目 ?MCP/.test(body),
    `tab=${tabVisible}`,
  );
  rec("U1b 说明条常驻", /只在本项目的任务中投影/.test(body));
  await page.screenshot({ path: join(OUT, "u1-capabilities-tab.png"), fullPage: true });

  // ---- U3 / U3b：依赖闭包预览（对**已保存**的绑定行也必须显示）----
  rec("U3 闭包预览（已保存行）", /任务派发时会一并投影/.test(body),
    (body.match(/该技能依赖[^。]*。/)?.[0] ?? "").slice(0, 70));
  rec(
    "U3b 预览带 env 前提半句",
    /若执行者已配齐所需环境变量/.test(body),
    "缺这半句会被读成「绑了就有凭据」",
  );

  // ---- U2：草稿态不即时写入（承重）----
  const rowsBefore = (body.match(/依赖 MCP \d+/g) || []).length;
  const addTrigger = page.getByRole("combobox", { name: "从技能市场添加" });
  if (await addTrigger.isVisible().catch(() => false)) {
    await addTrigger.click();
    await page.waitForTimeout(600);
    const opt = page.locator('[role="option"]').first();
    if (await opt.isVisible().catch(() => false)) {
      await opt.click();
      await page.waitForTimeout(300);
      await page.getByRole("button", { name: "添加到列表" }).click();
      await page.waitForTimeout(600);
      const afterAdd = await page.locator("body").innerText();
      await page.screenshot({ path: join(OUT, "u2-draft.png"), fullPage: true });

      await page.reload({ waitUntil: SAFE_WAIT_UNTIL });
      await page.waitForTimeout(2500);
      const afterReload = await page.locator("body").innerText();
      const rowsAfter = (afterReload.match(/依赖 MCP \d+/g) || []).length;
      rec(
        "U2 草稿不即时写入",
        /未保存/.test(afterAdd) &&
          /整体替换/.test(afterAdd) &&
          rowsAfter === rowsBefore &&
          !/未保存/.test(afterReload),
        `pill+alert 出现；刷新后行数 ${rowsBefore}→${rowsAfter}`,
      );
    } else {
      rec("U2 草稿不即时写入", false, "技能市场无可选项（可能已全部绑定）");
    }
  } else {
    rec("U2 草稿不即时写入", false, "找不到「从技能市场添加」");
  }

  // ---- U7 / U8：员工侧场地标记 ----
  // 不靠"页面上出现某个词"来断言——那会随租户数据漂移（全部技能都被项目绑定时
  // 就永远看不到「通用」，反之亦然）。改为**拿 API 真值逐条对照 UI**：
  //   project_bindings 为空 → 必须显示「通用」；非空 → 必须显示「限 N 个项目」。
  const empRes = await page.request.get(`${CP}/api/v1/digital-employees?limit=20`);
  const empJson = await empRes.json();
  const employees = Array.isArray(empJson) ? empJson : empJson.items ?? [];
  let checked = 0;
  let sawUniversal = false;
  let sawScoped = false;
  const mismatches = [];
  for (const emp of employees.slice(0, 8)) {
    const skRes = await page.request.get(
      `${CP}/api/v1/digital-employees/${emp.id}/skills`,
    );
    const skJson = await skRes.json();
    const entries = Array.isArray(skJson) ? skJson : skJson.items ?? [];
    if (entries.length === 0) continue;

    await page.goto(`${WEB}/employees/${emp.id}/config`, { waitUntil: SAFE_WAIT_UNTIL });
    await page.waitForTimeout(2200);
    const t = await page.locator("body").innerText();
    if (!/已生效技能/.test(t)) continue;

    for (const entry of entries) {
      const bindings = entry.skill?.project_bindings ?? [];
      checked += 1;
      if (bindings.length === 0) {
        sawUniversal = true;
        if (!/通用/.test(t)) mismatches.push(`${emp.name}/${entry.skill?.slug}: 期望「通用」未出现`);
      } else {
        sawScoped = true;
        if (!new RegExp(`限 ${bindings.length} 个项目`).test(t)) {
          mismatches.push(
            `${emp.name}/${entry.skill?.slug}: 期望「限 ${bindings.length} 个项目」未出现`,
          );
        }
      }
    }
    if (!sawScoped || !sawUniversal) continue;
    await page.screenshot({ path: join(OUT, "u7-employee-venue.png"), fullPage: true });
    break;
  }
  rec(
    "U7/U8 场地标记与 API 一致",
    checked > 0 && mismatches.length === 0,
    mismatches.length
      ? mismatches.slice(0, 3).join(" | ")
      : `逐条比对 ${checked} 项（通用=${sawUniversal} 限定=${sawScoped}）`,
  );
  if (checked > 0 && !(sawUniversal && sawScoped)) {
    console.log(
      `  ⓘ 当前租户数据只覆盖了${sawUniversal ? "「通用」" : "「限定」"}一种形态；` +
        `另一形态未被验到（不是缺陷，是数据状态）。要全覆盖：让某技能不绑任何项目 / 或给某技能加项目绑定。`,
    );
  }

  // ---- U9：技能详情页 ----
  const skRes = await page.request.get(`${CP}/api/v1/skills?limit=5`);
  const skJson = await skRes.json();
  const skills = Array.isArray(skJson) ? skJson : skJson.items ?? [];
  if (skills.length) {
    await page.goto(`${WEB}/skills/${skills[0].id}`, { waitUntil: SAFE_WAIT_UNTIL });
    await page.waitForTimeout(2200);
    const t = await page.locator("body").innerText();
    rec(
      "U9 技能详情页",
      /项目绑定/.test(t) && /依赖分两种情况/.test(t) && !/依赖不会自动为员工开通能力/.test(t),
      "项目绑定卡 + 新依赖文案 + 旧文案零残留",
    );
    await page.screenshot({ path: join(OUT, "u9-skill-detail.png"), fullPage: true });
  } else {
    rec("U9 技能详情页", false, "租户内无技能");
  }

  // ---- 控制台：滤掉已知良性噪音后必须为零 ----
  const noise = realErrors();
  rec("控制台无真实错误", noise.length === 0, noise.slice(0, 3).join(" | ") || "无（已滤 /api/auth/me 401）");
} catch (err) {
  rec("EXCEPTION", false, String(err?.message || err).slice(0, 300));
} finally {
  await browser.close();
  const failed = results.filter((r) => !r.ok);
  writeFileSync(
    join(OUT, "result.json"),
    JSON.stringify({ ok: failed.length === 0, project: PROJECT, results }, null, 2),
  );
  console.log(`\n结果：${results.length - failed.length}/${results.length} 通过；产物 ${OUT}`);
  if (failed.length) process.exitCode = 1;
}
