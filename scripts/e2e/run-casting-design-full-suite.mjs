/**
 * Clean P1 → sequential design suite:
 *   residual (G1–G9 API) → G9-coordinator → G10–G12 → G9→G10 chain → G13
 *
 * Between product E2E stages: hard SQL clean + restore castings.
 * G13 is layered verify only (no P1 dependency).
 *
 *   node scripts/e2e/run-casting-design-full-suite.mjs
 *   SKIP_G13=1 node scripts/e2e/run-casting-design-full-suite.mjs
 */
import { spawnSync } from "node:child_process";
import { mkdirSync, writeFileSync, readFileSync, existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, "../..");
const OUT = join(ROOT, ".scratch/e2e-casting-design-full-suite");
mkdirSync(OUT, { recursive: true });

const PROJECT_ID =
  process.env.SUPERTEAM_PROJECT_ID || "ca82b054-de2d-4810-9a2b-dd41f5e50a2c";
const CP = process.env.SUPERTEAM_CP_URL || "http://127.0.0.1:8080";
const EMP = {
  developer: "0be393bb-9dfd-48c8-b010-4b5abb114f23",
  reviewer: "7a16f593-9a99-490e-bcab-77bb8b326afa",
  tester: "157b1a2c-b2af-4a08-99f3-f16abe291ed1",
  ops: "9a623b40-c9ec-4d7d-99a4-17b1f569b52e",
};

const suite = {
  ok: false,
  started_at: new Date().toISOString(),
  stages: {},
  errors: [],
};

function log(m) {
  console.log(`[full-suite] ${m}`);
}

function run(cmd, args, opts = {}) {
  log(`$ ${cmd} ${args.join(" ")}`);
  const r = spawnSync(cmd, args, {
    cwd: ROOT,
    encoding: "utf8",
    env: { ...process.env, PATH: process.env.PATH },
    maxBuffer: 20 * 1024 * 1024,
    timeout: opts.timeout ?? 0,
    shell: false,
  });
  const out = (r.stdout || "") + (r.stderr || "");
  if (opts.logFile) {
    writeFileSync(join(OUT, opts.logFile), out);
  }
  return { code: r.status ?? 1, out, signal: r.signal };
}

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

async function api(cookie, path, { method = "GET", body } = {}) {
  const res = await fetch(`${CP}${path}`, {
    method,
    headers: {
      "content-type": "application/json",
      accept: "application/json",
      cookie,
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

async function restoreBaseline() {
  const cookie = await cpLogin();
  for (const [id, roles] of [
    [EMP.developer, ["developer", "diagnostician"]],
    [EMP.reviewer, ["reviewer", "verifier"]],
    [EMP.tester, ["tester"]],
    [EMP.ops, ["collector", "analyst", "diagnostician"]],
  ]) {
    await api(cookie, `/api/v1/digital-employees/${id}/roles`, {
      method: "PUT",
      body: { role_keys: roles },
    });
  }
  await api(cookie, `/api/v1/digital-employees/${EMP.developer}/status`, {
    method: "PUT",
    body: { status: "ready" },
  });
  await api(cookie, `/api/v1/digital-employees/${EMP.reviewer}/status`, {
    method: "PUT",
    body: { status: "ready" },
  });
  await api(cookie, `/api/v1/digital-employees/${EMP.tester}/status`, {
    method: "PUT",
    body: { status: "ready" },
  });
  // full healthy cast for residual start; individual scripts may overwrite
  await api(cookie, `/api/v1/projects/${PROJECT_ID}/castings`, {
    method: "PUT",
    body: {
      scenario_template_key: "software_delivery",
      assignments: [
        { role_key: "developer", digital_employee_id: EMP.developer },
        { role_key: "reviewer", digital_employee_id: EMP.reviewer },
        { role_key: "tester", digital_employee_id: EMP.tester },
      ],
    },
  });
  await api(cookie, `/api/v1/projects/${PROJECT_ID}/castings`, {
    method: "PUT",
    body: {
      scenario_template_key: "incident_response",
      assignments: [
        { role_key: "diagnostician", digital_employee_id: EMP.ops },
        { role_key: "operator", digital_employee_id: EMP.developer },
        { role_key: "verifier", digital_employee_id: EMP.reviewer },
      ],
    },
  });
  const dem = await api(cookie, `/api/v1/projects/${PROJECT_ID}/demands`);
  const items = Array.isArray(dem.json) ? dem.json : dem.json?.items || [];
  const inbox = await api(
    cookie,
    `/api/v1/inbox/items?view=mine&status=open&limit=20`,
  );
  const open = Array.isArray(inbox.json)
    ? inbox.json
    : inbox.json?.items || [];
  log(`baseline: demands=${items.length} open_inbox=${open.length}`);
  return { demands: items.length, open_inbox: open.length };
}

function hardClean(retries = 3) {
  log("--- hard clean P1 ---");
  let last = null;
  for (let i = 1; i <= retries; i++) {
    const r = run("bash", ["scripts/e2e/cleanup-p1-hard-sql.sh"], {
      logFile: `clean-try${i}.log`,
      timeout: 120000,
    });
    last = r;
    if (r.code === 0) return;
    log(`hard clean attempt ${i}/${retries} failed: ${r.out.slice(-200)}`);
    spawnSync("sleep", ["2"], { encoding: "utf8" });
  }
  throw new Error(`hard clean failed after ${retries}: ${last?.out?.slice(-500)}`);
}

function readJson(path) {
  if (!existsSync(path)) return null;
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch {
    return null;
  }
}

async function stage(name, fn) {
  log(`======== STAGE ${name} ========`);
  const started = Date.now();
  try {
    const detail = await fn();
    suite.stages[name] = {
      pass: true,
      ms: Date.now() - started,
      ...(detail && typeof detail === "object" ? detail : {}),
    };
    log(`STAGE ${name} PASS (${suite.stages[name].ms}ms)`);
    return true;
  } catch (e) {
    suite.stages[name] = {
      pass: false,
      ms: Date.now() - started,
      error: String(e).slice(0, 500),
    };
    suite.errors.push(`${name}: ${e}`);
    log(`STAGE ${name} FAIL: ${e}`);
    return false;
  }
}

async function main() {
  // 0. clean + baseline
  await stage("clean_baseline", async () => {
    hardClean();
    return restoreBaseline();
  });

  // 1. residual G1-G9
  const residualOk = await stage("residual", async () => {
    hardClean();
    await restoreBaseline();
    const r = run(
      "node",
      ["scripts/e2e/browser-casting-design-gates-residual.mjs"],
      { logFile: "residual.log", timeout: 900000 },
    );
    const res = readJson(
      join(ROOT, ".scratch/e2e-casting-design-residual/result.json"),
    );
    writeFileSync(
      join(OUT, "residual-result.json"),
      JSON.stringify(res, null, 2),
    );
    if (r.code !== 0 || !res?.ok) {
      throw new Error(
        `residual exit=${r.code} ok=${res?.ok} gates=${JSON.stringify(res?.gates)}`,
      );
    }
    return { gates: res.gates, ok: res.ok };
  });

  // 2. G9 coordinator
  const g9Ok = await stage("g9_coordinator", async () => {
    hardClean();
    await restoreBaseline();
    const r = run(
      "node",
      ["scripts/e2e/browser-casting-design-g9-coordinator.mjs"],
      { logFile: "g9-coordinator.log", timeout: 900000 },
    );
    const res = readJson(
      join(ROOT, ".scratch/e2e-casting-design-g9-coordinator/result.json"),
    );
    writeFileSync(
      join(OUT, "g9-coordinator-result.json"),
      JSON.stringify(res, null, 2),
    );
    if (r.code !== 0 || !res?.ok) {
      throw new Error(
        `g9-coord exit=${r.code} ok=${res?.ok} gates=${JSON.stringify(res?.gates)}`,
      );
    }
    return { gates: res.gates, ok: res.ok };
  });

  // 3. G10-G12 design
  const g1012Ok = await stage("g10_g11_g12", async () => {
    hardClean();
    await restoreBaseline();
    const r = run(
      "node",
      ["scripts/e2e/browser-casting-design-g10-g11-g12.mjs"],
      { logFile: "g10-g11-g12.log", timeout: 1200000 },
    );
    const res = readJson(
      join(ROOT, ".scratch/e2e-browser-casting-design-gates/result.json"),
    );
    writeFileSync(
      join(OUT, "g10-g11-g12-result.json"),
      JSON.stringify(res, null, 2),
    );
    if (r.code !== 0 || !res?.ok) {
      throw new Error(
        `g10-12 exit=${r.code} ok=${res?.ok} gates=${JSON.stringify(res?.gates)}`,
      );
    }
    return { gates: res.gates, ok: res.ok };
  });

  // 4. G9→G10 full chain (coordinator auto → human approve → replan)
  const chainOk = await stage("g9_to_g10_chain", async () => {
    hardClean();
    await restoreBaseline();
    const r = run(
      "node",
      ["scripts/e2e/browser-casting-design-g9-to-g10-chain.mjs"],
      { logFile: "g9-g10-chain.log", timeout: 1200000 },
    );
    const res = readJson(
      join(ROOT, ".scratch/e2e-casting-design-g9-g10-chain/result.json"),
    );
    writeFileSync(
      join(OUT, "g9-g10-chain-result.json"),
      JSON.stringify(res, null, 2),
    );
    if (r.code !== 0 || !res?.ok) {
      throw new Error(
        `chain exit=${r.code} ok=${res?.ok} gates=${JSON.stringify(res?.gates)}`,
      );
    }
    return { gates: res.gates, ok: res.ok };
  });

  // 5. G13
  let g13Ok = true;
  if (process.env.SKIP_G13 === "1") {
    suite.stages.g13 = { pass: true, skipped: true };
    log("STAGE g13 SKIPPED (SKIP_G13=1)");
  } else {
    g13Ok = await stage("g13", async () => {
      const results = {};
      for (const [name, cmd, args] of [
        ["verify_contracts", "corepack", ["pnpm", "verify:contracts"]],
        ["verify_control_plane", "corepack", ["pnpm", "verify:control-plane"]],
        ["verify_web", "corepack", ["pnpm", "verify:web"]],
        [
          "migrate_validate",
          "make",
          ["-C", "apps/control-plane", "migrate-validate"],
        ],
      ]) {
        log(`g13 ${name}...`);
        const r = run(cmd, args, {
          logFile: `g13-${name}.log`,
          timeout: name === "verify_web" ? 1200000 : 900000,
        });
        results[name] = { code: r.code };
        if (r.code !== 0) {
          throw new Error(`${name} exit ${r.code}`);
        }
      }
      return results;
    });
  }

  // final clean
  try {
    hardClean();
    await restoreBaseline();
  } catch (e) {
    log(`final clean warn: ${e}`);
  }

  suite.finished_at = new Date().toISOString();
  suite.ok =
    residualOk && g9Ok && g1012Ok && chainOk && g13Ok && suite.errors.length === 0;
  writeFileSync(join(OUT, "suite-result.json"), JSON.stringify(suite, null, 2));
  log(`======== SUITE ok=${suite.ok} ========`);
  log(JSON.stringify(suite.stages, null, 2));
  if (!suite.ok) process.exit(1);
}

main().catch((e) => {
  console.error(e);
  suite.errors.push(String(e));
  suite.ok = false;
  suite.finished_at = new Date().toISOString();
  writeFileSync(join(OUT, "suite-result.json"), JSON.stringify(suite, null, 2));
  process.exit(1);
});
