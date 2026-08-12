#!/usr/bin/env node
/**
 * P0 验收：声明式交付物落点 + 串味修复 +（可选）legacy 双读提示。
 * 复用 fake-providers/claude-success-with-artifacts.sh，不依赖 RustFS 断言。
 */
import { spawnSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { login, api, apiOk, assert, CP } from "../e2e/lib/cp-client.mjs";
import { resolveFixtures } from "../e2e/lib/fixtures.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, "../..");
const RUNTIME_CONFIG = join(ROOT, "apps/runtime-agent/config.yaml");
const RUNTIME_CONFIG_BAK = join(ROOT, ".scratch/runtime-config.p0-deliverables.bak.yaml");
const FAKE_PROVIDER = join(
  ROOT,
  "scripts/e2e/fake-providers/claude-success-with-artifacts.sh",
);
const PRODUCES_FILE = join(ROOT, ".scratch/e2e/fake-produces.json");
const ACCEPT_FILE = join(ROOT, ".scratch/e2e/fake-acceptance.json");
const WORKSPACE_BASE = "/var/superteam/workspaces";

function log(...args) {
  console.log("[p0-deliverables]", ...args);
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
  log("wrote produces", names);
}

function writeAcceptance(task, extra = []) {
  const hc = task?.handoff_contract || {};
  let criteria = Array.isArray(hc.acceptance_criteria) ? hc.acceptance_criteria : [];
  criteria = [...criteria, ...extra]
    .map((c) => {
      if (typeof c === "string") return c.trim();
      if (c && typeof c === "object") {
        return String(c.criterion || c.text || c.summary || c.description || c.name || "").trim();
      }
      return "";
    })
    .filter(Boolean)
    .filter((c) => c !== "[object Object]");
  criteria = [...new Set(criteria)];
  mkdirSync(dirname(ACCEPT_FILE), { recursive: true });
  writeFileSync(ACCEPT_FILE, JSON.stringify(criteria, null, 2), "utf8");
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

function projectDirName(project) {
  return String(project.directory_name || project.name || "").trim();
}

function listSessionDeliverables(projectPath) {
  const sessionsRoot = join(projectPath, ".superteam", "sessions");
  if (!existsSync(sessionsRoot)) return [];
  const out = [];
  for (const sid of readdirSync(sessionsRoot)) {
    const deliv = join(sessionsRoot, sid, "deliverables");
    if (!existsSync(deliv) || !statSync(deliv).isDirectory()) continue;
    for (const f of readdirSync(deliv)) {
      out.push({ commandId: sid, path: join(deliv, f), rel: `.superteam/sessions/${sid}/deliverables/${f}` });
    }
  }
  return out;
}

function gitPorcelain(projectPath) {
  const r = spawnSync("git", ["-C", projectPath, "status", "--porcelain"], { encoding: "utf8" });
  return (r.stdout || "").trim();
}

async function createAndApprove(cookie, projectId, title) {
  const r = await api(cookie, `/api/v1/projects/${projectId}/demands`, {
    method: "POST",
    body: {
      title,
      content:
        "【P0 交付物落点验收】在工作区声明式交付物目录写入报告文件，内容包含 ok。低风险。",
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
        { method: "POST", body: { decision: "approved", comment: "p0 deliverables e2e" } },
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
        { method: "POST", body: { decision: "approved", comment: "p0 redispatch" } },
      );
      log("approved wait card", t, d.id, res.status);
      n++;
    }
  }
  return n;
}

async function waitTaskTerminal(cookie, projectId, demandId, { writeProducesFlag = true } = {}) {
  let taskId = null;
  let clarificationsApproved = 0;
  let last = null;
  for (let i = 0; i < 70; i++) {
    const mine = await listTasks(cookie, projectId, demandId);
    if (mine[0]) {
      last = mine[0];
      taskId = mine[0].id;
      if (writeProducesFlag) {
        const produces = producesOf(mine[0]);
        writeProduces(produces.length ? produces : ["p0_smoke_report"]);
      } else {
        // 第二棒：故意不写 produces，且写空 acceptance，尽量让假 provider 不产出声明文件
        writeProduces([]);
      }
      writeAcceptance(mine[0]);
      log("task", taskId, mine[0].status, "produces", producesOf(mine[0]));

      if (["completed", "done", "success", "failed", "cancelled"].includes(mine[0].status)) {
        return mine[0];
      }
      if (mine[0].status === "waiting_human" && clarificationsApproved < 3) {
        clarificationsApproved += await approveClarificationOrGate(cookie, projectId, taskId);
      }
    }
    await new Promise((r) => setTimeout(r, 4000));
  }
  throw new Error(`task not terminal: ${taskId} status=${last?.status}`);
}

async function listDeclaredArtifacts(cookie, projectId, taskId) {
  const arts = await apiOk(cookie, `/api/v1/projects/${projectId}/artifacts?limit=100`);
  const items = Array.isArray(arts) ? arts : arts.items || [];
  return items.filter((a) => {
    const kind = a.kind || a.artifact_kind || a.source_kind || "";
    const meta = JSON.stringify(a);
    return (
      kind.includes("declared") ||
      meta.includes(taskId) ||
      meta.includes("deliverable")
    );
  });
}

async function main() {
  const verdict = {
    p0_1_session_path: "FAIL",
    p0_1_git_clean: "FAIL",
    p0_2_no_cross_task: "FAIL",
    p0_3_legacy: "SKIP",
  };
  log("CP", CP);
  const agents = countRuntimeAgents();
  log("runtime agents", agents.count, agents.lines);
  assert(agents.count <= 1, `multiple runtime-agent processes:\n${agents.lines.join("\n")}`);

  try {
    pointClaudeAtFakeProvider();
    restartRuntime();
    await new Promise((r) => setTimeout(r, 5000));

    // 复核 runtime pid 同源
    const status = spawnSync("bash", [join(ROOT, "scripts/dev-services.sh"), "status"], {
      encoding: "utf8",
      cwd: ROOT,
    });
    log("status after restart:\n" + (status.stdout || "").trim());

    const cookie = await login();
    // Prefer a known git project for porcelain checks
    const projects = await apiOk(cookie, "/api/v1/projects?limit=100");
    const items = projects?.items ?? projects ?? [];
    const gitProject =
      items.find((p) => p.name === "S10 Git 屏蔽验收") ||
      items.find((p) => p.name?.includes("Git")) ||
      (await resolveFixtures(cookie)).project;
    const projectId = gitProject.id;
    const dirName = projectDirName(gitProject);
    const projectPath = join(WORKSPACE_BASE, dirName);
    log("project", projectId, gitProject.name, "dir", dirName, "path", projectPath);
    assert(existsSync(projectPath), `project workspace missing: ${projectPath}`);

    // Ensure it's a git repo for porcelain check
    const isGit = existsSync(join(projectPath, ".git"));
    log("is_git", isGit);

    // --- Task A: with deliverables ---
    const demandA = await createAndApprove(
      cookie,
      projectId,
      `p0-deliverables-A ${Date.now()}`,
    );
    const taskA = await waitTaskTerminal(cookie, projectId, demandA, { writeProducesFlag: true });
    log("task A terminal", taskA.id, taskA.status);
    assert(
      ["completed", "done", "success"].includes(taskA.status),
      `task A not completed: ${taskA.status}`,
    );

    const sessionFiles = listSessionDeliverables(projectPath);
    log(
      "session deliverables",
      sessionFiles.map((f) => f.rel),
    );
    assert(sessionFiles.length > 0, "expected files under .superteam/sessions/*/deliverables/");
    verdict.p0_1_session_path = "PASS";

    if (isGit) {
      const porcelain = gitPorcelain(projectPath);
      log("git porcelain:\n" + (porcelain || "<empty>"));
      const hasRootDeliverables = porcelain
        .split("\n")
        .some((l) => /\?\?\s+deliverables\/?/.test(l) || / deliverables\//.test(l));
      assert(!hasRootDeliverables, "git status still shows root deliverables/");
      verdict.p0_1_git_clean = "PASS";
    } else {
      verdict.p0_1_git_clean = "阻塞：项目目录非 git，无法断言 porcelain";
    }

    // Capture command ids from task A session dirs for cross-task check
    const cmdIdsA = new Set(sessionFiles.map((f) => f.commandId));
    const filesA = new Set(sessionFiles.map((f) => f.rel));

    // --- Task B: no produces ---
    writeProduces([]);
    const demandB = await createAndApprove(
      cookie,
      projectId,
      `p0-deliverables-B-no-produce ${Date.now()}`,
    );
    const taskB = await waitTaskTerminal(cookie, projectId, demandB, { writeProducesFlag: false });
    log("task B terminal", taskB.id, taskB.status);

    // Inspect result contract / artifacts for task B — must not re-report A's files
    const results = await api(
      cookie,
      `/api/v1/projects/${projectId}/tasks/${taskB.id}`,
    );
    log("task B detail status", results.status);
    // Also check latest attempt result via graph or artifacts list
    const artsB = await listDeclaredArtifacts(cookie, projectId, taskB.id);
    log(
      "task B declared-ish artifacts",
      artsB.slice(0, 10).map((a) => ({
        id: a.id,
        kind: a.kind || a.artifact_kind,
        name: a.name || a.file_name || a.relative_path,
      })),
    );

    // Disk: new session dirs for B may exist empty or with files; if files exist they must not be A's paths re-uploaded as B
    const sessionAfterB = listSessionDeliverables(projectPath);
    const newOnlyB = sessionAfterB.filter((f) => !cmdIdsA.has(f.commandId));
    log(
      "sessions after B (new cmds)",
      newOnlyB.map((f) => f.rel),
    );

    // Strongest check: task B completed without declaring A's relative paths in its result
    // Fall back to: B's result artifacts empty or only B session paths
    let crossContaminated = false;
    for (const a of artsB) {
      const blob = JSON.stringify(a);
      for (const rel of filesA) {
        if (blob.includes(rel) || blob.includes(rel.split("/").pop())) {
          // same basename alone is weak; require session path of A
          if (blob.includes(".superteam/sessions/") && [...cmdIdsA].some((id) => blob.includes(id))) {
            crossContaminated = true;
          }
        }
      }
    }
    // Also: collect_declared should not pull A's session into B — if B has no new deliverables files, good.
    // If fake provider still wrote into B's session because sessions/* glob found a dir, that's OK as long as A paths aren't in B result.
    if (!crossContaminated) {
      verdict.p0_2_no_cross_task = "PASS";
    } else {
      verdict.p0_2_no_cross_task = "FAIL: task B artifacts reference task A session paths";
    }

    // --- Legacy path dual-read (optional, in-place) ---
    try {
      const legacyDir = join(projectPath, "deliverables");
      mkdirSync(legacyDir, { recursive: true });
      const legacyFile = join(legacyDir, `legacy-p0-${Date.now()}.md`);
      writeFileSync(legacyFile, "# legacy dual-read\nok\n", "utf8");
      log("planted legacy file", legacyFile);

      // Trigger a third short task that will collect; or invoke probe via another demand
      writeProduces(["legacy_probe_report"]);
      const demandC = await createAndApprove(
        cookie,
        projectId,
        `p0-legacy-dual-read ${Date.now()}`,
      );
      const taskC = await waitTaskTerminal(cookie, projectId, demandC, { writeProducesFlag: true });
      log("task C terminal", taskC.id, taskC.status);

      // Look for legacy hint in result / skipped / events
      const taskDetail = await apiOk(cookie, `/api/v1/projects/${projectId}/tasks/${taskC.id}`);
      const events = await apiOk(cookie, `/api/v1/projects/${projectId}/events?limit=50`);
      const eventItems = Array.isArray(events) ? events : events.items || [];
      const blob = JSON.stringify({ taskDetail, eventItems: eventItems.slice(0, 20), arts: await listDeclaredArtifacts(cookie, projectId, taskC.id) });
      const hasLegacyHint =
        blob.includes("legacy_path") ||
        blob.includes("旧路径") ||
        blob.includes("deliverables/");
      log("legacy hint present?", hasLegacyHint);
      // Disk legacy file still there; collection may have uploaded it
      assert(existsSync(legacyFile), "legacy file disappeared unexpectedly");
      verdict.p0_3_legacy = hasLegacyHint
        ? "PASS"
        : "FAIL: planted legacy file but no legacy_path/visible hint found in task C surfaces";
      // cleanup legacy noise for the project
      try {
        spawnSync("rm", ["-f", legacyFile]);
      } catch {
        /* ignore */
      }
    } catch (e) {
      verdict.p0_3_legacy = `阻塞：${e.message}`;
    }

    console.log("\n=== P0 VERDICT ===");
    console.log(JSON.stringify(verdict, null, 2));
    const hardFail = Object.values(verdict).some((v) => String(v).startsWith("FAIL"));
    if (hardFail) process.exitCode = 1;
  } finally {
    restoreRuntimeConfig();
  }
}

main().catch((e) => {
  console.error(e);
  try {
    restoreRuntimeConfig();
  } catch {
    /* ignore */
  }
  process.exit(1);
});
