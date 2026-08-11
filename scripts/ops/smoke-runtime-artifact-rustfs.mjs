#!/usr/bin/env node
/**
 * Runtime → CP presign → 本地 RustFS 真链路冒烟。
 *
 * 1. 单 runtime-agent；临时把 claude binary 指到 fake-providers/claude-success-with-artifacts.sh
 * 2. 登录 → 建 demand → 批 plan_review
 * 3. 任务出现后把 planner produces 写入 .scratch/e2e/fake-produces.json
 * 4. 若因 handoff 缺交付物而 waiting_human：对 clarification 卡 decision=approved 触发再派发
 * 5. 等待 completed；用 mc 断言本地桶中有本 attempt 的 runs/ 与 artifacts/ 对象
 * 6. 还原 runtime config 并重启 agent
 */
import { execFileSync, spawnSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { login, api, apiOk, assert, CP } from "../e2e/lib/cp-client.mjs";
import { resolveFixtures } from "../e2e/lib/fixtures.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, "../..");
// 本机 docker 常是 shell alias→podman；node spawn 看不到 alias
const CONTAINER_CLI =
  process.env.SUPERTEAM_CONTAINER_CLI ||
  (existsSync("/opt/homebrew/bin/podman")
    ? "/opt/homebrew/bin/podman"
    : existsSync("/usr/local/bin/podman")
      ? "/usr/local/bin/podman"
      : "docker");
const RUNTIME_CONFIG = join(ROOT, "apps/runtime-agent/config.yaml");
const RUNTIME_CONFIG_BAK = join(ROOT, ".scratch/runtime-config.artifact-smoke.bak.yaml");
const FAKE_PROVIDER = join(
  ROOT,
  "scripts/e2e/fake-providers/claude-success-with-artifacts.sh",
);
const PRODUCES_FILE = join(ROOT, ".scratch/e2e/fake-produces.json");
const ACCEPT_FILE = join(ROOT, ".scratch/e2e/fake-acceptance.json");

const PGURL =
  process.env.SUPERTEAM_E2E_DB_URL ||
  "postgres://superteam:83ab1f233b790e580ba5dae3a26998d78095f780d7067b32@115.190.247.9:35432/superteam?sslmode=disable";

function log(...args) {
  console.log("[artifact-rustfs]", ...args);
}

function psql(sql) {
  try {
    return execFileSync("psql", [PGURL, "-tAc", sql], { encoding: "utf8" }).trim();
  } catch (e) {
    log("psql warn", e.message?.slice?.(0, 200) || e);
    return "";
  }
}

function countRuntimeAgents() {
  const out = spawnSync(
    "bash",
    [
      "-lc",
      "ps -ax -o pid=,command= | grep -E 'target/.*/runtime-agent|runtime-agent --config' | grep -v grep || true",
    ],
    { encoding: "utf8" },
  );
  const lines = (out.stdout || "")
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .filter((l) => !l.includes("pnpm") && !l.includes("node "));
  return { lines, count: lines.length };
}

function restartRuntime() {
  const r = spawnSync("bash", [join(ROOT, "scripts/dev-services.sh"), "restart", "runtime-agent"], {
    encoding: "utf8",
    cwd: ROOT,
  });
  if (r.status !== 0) {
    throw new Error(`restart runtime-agent failed: ${r.stderr || r.stdout}`);
  }
  log("runtime-agent restarted");
}

function pointClaudeAtFakeProvider() {
  if (!existsSync(FAKE_PROVIDER)) throw new Error(`missing fake provider: ${FAKE_PROVIDER}`);
  spawnSync("chmod", ["+x", FAKE_PROVIDER]);
  if (!existsSync(RUNTIME_CONFIG)) throw new Error(`missing ${RUNTIME_CONFIG}`);
  copyFileSync(RUNTIME_CONFIG, RUNTIME_CONFIG_BAK);
  let text = readFileSync(RUNTIME_CONFIG, "utf8");
  text = text.replace(
    /(claude_code:\s*\n(?:[ \t]+.+\n)*?[ \t]+binary_path:\s*).+/,
    `$1${FAKE_PROVIDER}`,
  );
  writeFileSync(RUNTIME_CONFIG, text, "utf8");
  log("pointed claude_code.binary_path ->", FAKE_PROVIDER);
}

function restoreRuntimeConfig() {
  if (process.env.SUPERTEAM_SKIP_RUNTIME_RESTORE === "1") {
    log("SKIP restore runtime config");
    return;
  }
  if (existsSync(RUNTIME_CONFIG_BAK)) {
    copyFileSync(RUNTIME_CONFIG_BAK, RUNTIME_CONFIG);
    log("restored runtime config from bak");
    restartRuntime();
  }
}

function writeProduces(names) {
  mkdirSync(dirname(PRODUCES_FILE), { recursive: true });
  writeFileSync(PRODUCES_FILE, JSON.stringify(names, null, 2), "utf8");
  log("wrote produces file", PRODUCES_FILE, names);
}

function criterionText(c) {
  if (typeof c === "string") return c.trim();
  if (c && typeof c === "object") {
    return String(
      c.criterion || c.text || c.summary || c.description || c.name || "",
    ).trim();
  }
  return "";
}

function writeAcceptance(task, extraCriteria = []) {
  const hc = task?.handoff_contract || {};
  let criteria = hc.acceptance_criteria;
  if (!Array.isArray(criteria)) criteria = [];
  criteria = [...criteria, ...extraCriteria]
    .map(criterionText)
    .filter(Boolean)
    // 丢掉 String(object) 噪声
    .filter((c) => c !== "[object Object]");
  criteria = [...new Set(criteria)];
  mkdirSync(dirname(ACCEPT_FILE), { recursive: true });
  writeFileSync(ACCEPT_FILE, JSON.stringify(criteria, null, 2), "utf8");
  log("wrote acceptance file", ACCEPT_FILE, criteria);
}

/** 从 validation_errors 抽出 acceptance_result_missing:* 的判据原文 */
function criteriaFromValidationErrors(errorsJson) {
  let arr = errorsJson;
  if (typeof errorsJson === "string") {
    try {
      arr = JSON.parse(errorsJson);
    } catch {
      arr = [errorsJson];
    }
  }
  if (!Array.isArray(arr)) return [];
  const out = [];
  for (const e of arr) {
    const s = String(e);
    const m = s.match(/^acceptance_result_missing:(.+)$/);
    if (m) out.push(m[1]);
  }
  return out;
}

async function createAndApprove(cookie, projectId, title) {
  const r = await api(cookie, `/api/v1/projects/${projectId}/demands`, {
    method: "POST",
    body: {
      title,
      content:
        "【对象存储冒烟】在工作区 deliverables/ 下写入报告文件，内容包含 ok。低风险、无需改生产系统。",
      source_type: "console",
      coordination_mode: "plan",
    },
  });
  assert(r.ok, `create demand ${r.status} ${r.text}`);
  const demandId = r.json.id;
  log("demand", demandId);

  for (let i = 0; i < 80; i++) {
    const dec = await apiOk(cookie, `/api/v1/projects/${projectId}/decisions`);
    const list = Array.isArray(dec) ? dec : dec.items || [];
    const plans = list
      .filter((d) => d.decision_type === "plan_review" && d.status_snapshot === "pending")
      .sort((a, b) => String(b.created_at).localeCompare(String(a.created_at)));
    if (plans[0]) {
      const mine =
        plans.find((d) => d.demand_id === demandId || d.context?.demand_id === demandId) ||
        plans[0];
      const res = await api(
        cookie,
        `/api/v1/projects/${projectId}/decisions/${mine.id}/resolve`,
        {
          method: "POST",
          body: { decision: "approved", comment: "rustfs artifact e2e" },
        },
      );
      assert(res.ok, `approve ${res.status} ${res.text}`);
      log("plan approved", mine.id);
      return demandId;
    }
    await new Promise((r) => setTimeout(r, 3000));
  }
  throw new Error("plan_review not ready");
}

async function listTasks(cookie, projectId, demandId) {
  const tasks = await apiOk(cookie, `/api/v1/projects/${projectId}/tasks`);
  const items = Array.isArray(tasks) ? tasks : tasks.items || [];
  return items.filter((t) => t.demand_id === demandId);
}

function producesOf(task) {
  const raw = task?.planner_metadata?.produces;
  if (!Array.isArray(raw)) return [];
  return raw.map((x) => String(x).trim()).filter(Boolean);
}

/** 仅批 clarification / 预检 approval；绝不批 recovery（会制造僵尸卡风暴）。 */
async function approveClarificationOrGate(cookie, projectId, taskId) {
  const dec = await apiOk(cookie, `/api/v1/projects/${projectId}/decisions`);
  const list = Array.isArray(dec) ? dec : dec.items || [];
  let n = 0;
  for (const d of list) {
    if (d.status_snapshot !== "pending") continue;
    const t = d.decision_type || d.kind || "";
    if (t.includes("recovery")) continue;
    const linked =
      d.project_task_id === taskId ||
      d.resource_id === taskId ||
      JSON.stringify(d).includes(taskId);
    if (
      linked &&
      (t === "project_task_clarification" ||
        t === "project_task_approval" ||
        t.includes("clarification") ||
        t.includes("approval"))
    ) {
      const res = await api(
        cookie,
        `/api/v1/projects/${projectId}/decisions/${d.id}/resolve`,
        {
          method: "POST",
          body: { decision: "approved", comment: "rustfs smoke redispatch" },
        },
      );
      log("approved wait card", t, d.id, res.status);
      n++;
    }
  }
  return n;
}

function latestAttemptIdWithObjects(taskId) {
  // 优先有 raw 上传的 attempt（status 多为 cancelled=validation 后）；按 created_at 倒序
  const row = psql(
    `SELECT pta.id::text || '|' || coalesce(ptr.validation_errors::text,'[]')
     FROM project_task_attempts pta
     LEFT JOIN project_task_results ptr ON ptr.attempt_id = pta.id
     WHERE pta.project_task_id='${taskId}'
     ORDER BY pta.created_at ASC`,
  );
  const lines = row.split("\n").filter(Boolean);
  // 返回全部 attempt id，供 mc 探测
  return lines.map((l) => {
    const [id, ...rest] = l.split("|");
    return { id, validation_errors: rest.join("|") };
  });
}

function assertRustfsHasAttemptObjects(attemptIds) {
  const ids = Array.isArray(attemptIds) ? attemptIds : [attemptIds];
  assert(ids.length > 0, "attempt id required for S3 check");
  let lastOut = "";
  let okId = null;
  // raw 收尾可能略晚于 task completed；短暂重试
  for (let attempt = 0; attempt < 8 && !okId; attempt++) {
    if (attempt > 0) {
      spawnSync("sleep", ["2"]);
    }
    for (const attemptId of ids) {
      if (!attemptId) continue;
      const script = `
mc alias set rustfs http://host.containers.internal:9000 rustfsadmin rustfsadmin >/dev/null
mc stat "rustfs/superteam-artifacts/runs/00000000-0000-0000-0000-000000000001/${attemptId}/raw.part-0001.jsonl" && \
mc cat "rustfs/superteam-artifacts/runs/00000000-0000-0000-0000-000000000001/${attemptId}/raw.part-0001.jsonl"
`;
      const r = spawnSync(
        CONTAINER_CLI,
        ["run", "--rm", "--entrypoint", "/bin/sh", "minio/mc:latest", "-c", script],
        { encoding: "utf8" },
      );
      lastOut = (r.stdout || "") + (r.stderr || "");
      if (
        r.status === 0 &&
        (lastOut.includes("ses_e2e_artifact") || lastOut.includes("result_contract"))
      ) {
        okId = attemptId;
        break;
      }
    }
  }
  log("mc probe out:\n" + lastOut.slice(0, 1500));
  assert(okId, `no attempt raw with ses_e2e_artifact among ${ids.join(",")}`);
  log("rustfs raw ok for attempt", okId);

  const arts = spawnSync(
    CONTAINER_CLI,
    [
      "run",
      "--rm",
      "--entrypoint",
      "/bin/sh",
      "minio/mc:latest",
      "-c",
      `mc alias set rustfs http://host.containers.internal:9000 rustfsadmin rustfsadmin >/dev/null; mc ls rustfs/superteam-artifacts/artifacts/00000000-0000-0000-0000-000000000001/sha256/ | wc -l`,
    ],
    { encoding: "utf8" },
  );
  const artCount = parseInt(String(arts.stdout || "0").trim(), 10) || 0;
  log("artifact object count", artCount);
  assert(artCount > 0, "expected content-addressed artifacts in local rustfs");
  return okId;
}

async function main() {
  log("CP", CP);
  const health = await fetch(`${CP}/health`).then((r) => r.json());
  log("cp", health.status);
  const rustfs = await fetch("http://127.0.0.1:9000/health").then((r) => r.json());
  log("rustfs", rustfs.status, rustfs.ready);

  const agents = countRuntimeAgents();
  log("runtime agents", agents.count, agents.lines);
  if (agents.count > 1) {
    throw new Error(`multiple runtime-agent processes:\n${agents.lines.join("\n")}`);
  }

  let demandId = null;
  try {
    pointClaudeAtFakeProvider();
    restartRuntime();
    await new Promise((r) => setTimeout(r, 5000));

    const cookie = await login();
    const fx = await resolveFixtures(cookie);
    const projectId = process.env.SUPERTEAM_E2E_PROJECT_ID || fx.projectId;
    log("project", projectId, fx.project?.name);

    demandId = await createAndApprove(
      cookie,
      projectId,
      `rustfs-artifact-smoke ${Date.now()}`,
    );

    let taskId = null;
    let completed = false;
    let clarificationsApproved = 0;

    for (let i = 0; i < 60; i++) {
      const mine = await listTasks(cookie, projectId, demandId);
      if (mine[0]) {
        taskId = mine[0].id;
        const produces = producesOf(mine[0]);
        if (produces.length) writeProduces(produces);
        else writeProduces(["smoke_report"]);
        writeAcceptance(mine[0]);

        // 从已失败 attempt 的 validation_errors 反哺 acceptance 判据
        const attempts = latestAttemptIdWithObjects(taskId);
        for (const a of attempts) {
          const missing = criteriaFromValidationErrors(a.validation_errors);
          if (missing.length) writeAcceptance(mine[0], missing);
        }

        log("task", taskId, mine[0].status, "produces", produces, "attempts", attempts.length);

        if (["completed", "done", "success"].includes(mine[0].status)) {
          completed = true;
          break;
        }

        if (mine[0].status === "waiting_human" && clarificationsApproved < 2) {
          const n = await approveClarificationOrGate(cookie, projectId, taskId);
          clarificationsApproved += n;
          log("clarificationsApproved total", clarificationsApproved);
        }

        // 已有至少 1 次 attempt 结果（含 validation_failed）→ 对象存储可断言
        if (attempts.length >= 1 && i >= 4) {
          if (clarificationsApproved >= 1 && mine[0].status === "waiting_human" && i >= 12) {
            break;
          }
          if (attempts.length >= 2) break;
        }
      }
      await new Promise((r) => setTimeout(r, 4000));
    }

    assert(taskId, "no task created");
    const attempts = latestAttemptIdWithObjects(taskId);
    const attemptIds = attempts.map((a) => a.id);
    log("attempts", attempts, "completed=", completed);
    assert(attemptIds.length > 0, "no attempts recorded");

    // 主断言：DB 已有落在本地桶键上的 artifact 行（presign 上传成功的业务事实）
    const artRows = psql(
      `SELECT string_agg(artifact_type||'|'||coalesce(title,'')||'|'||object_ref, E'\\n' ORDER BY created_at)
       FROM project_artifact_refs
       WHERE project_task_id='${taskId}'
         AND object_ref LIKE 'artifacts/%'`,
    );
    log("db artifacts:\n" + (artRows || "(none)"));
    assert(artRows && artRows.includes("artifacts/"), "expected project_artifact_refs with local object keys");
    assert(
      artRows.includes("declared") || artRows.includes("execution_output") || artRows.includes("execution_transcript"),
      "expected declared/execution_output/transcript artifact types",
    );

    // 辅断言：从桶读回一条 declared/transcript 对象内容
    const firstKey = artRows
      .split("\n")
      .map((l) => l.split("|")[2])
      .find((k) => k && k.includes("sha256"));
    assert(firstKey, "no sha256 object key");
    const mc = spawnSync(
      CONTAINER_CLI,
      [
        "run",
        "--rm",
        "--entrypoint",
        "/bin/sh",
        "minio/mc:latest",
        "-c",
        `mc alias set rustfs http://host.containers.internal:9000 rustfsadmin rustfsadmin >/dev/null; mc cat rustfs/superteam-artifacts/${firstKey} | head -c 400; echo`,
      ],
      { encoding: "utf8" },
    );
    log("mc cat status", mc.status, "body", (mc.stdout || "").slice(0, 300), (mc.stderr || "").slice(0, 200));
    if (mc.status !== 0) {
      // 桶读偶发失败时不掩盖 DB 事实；再试 raw 目录
      try {
        assertRustfsHasAttemptObjects(attemptIds);
      } catch (e) {
        log("mc secondary probe failed:", e.message);
        assert(false, `mc cat failed for ${firstKey}: ${mc.stderr || mc.stdout}`);
      }
    } else {
      assert(
        (mc.stdout || "").length > 0,
        "mc cat returned empty body",
      );
    }

    assert(completed, "expected task completed after fake provider with produces+acceptance");

    log("SMOKE_OK runtime→presign→local RustFS");
    log("DEMAND", demandId, "TASK", taskId, "ATTEMPTS", attemptIds.join(","));
  } finally {
    restoreRuntimeConfig();
  }
}

main().catch((e) => {
  console.error("FAIL", e.message || e);
  try {
    restoreRuntimeConfig();
  } catch (re) {
    console.error("restore failed", re);
  }
  process.exit(1);
});
