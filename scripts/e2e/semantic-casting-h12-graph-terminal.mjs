/**
 * 批三 H12（design 2026-08-05 §7.0 ①②）：扩编 replan 后的图终态检查。
 *
 * 缺陷：planned_task_key 合并会把**已完成**的上游原样留在图里，挂它下面的新任务
 * 建出来就是 blocked，而解锁只由"上游完成"信号驱动 —— 那个信号早已发生过、不会
 * 再来，于是永久滞留。
 *
 * 本脚本对一条**已经有完成任务**的 demand 再走一次真实扩编批准链路，然后断言：
 *   G1  批准后确实触发了 replan（新协调作业）
 *   G2  replan 落地后，图里不存在"阻塞已全解却仍 blocked"的任务
 *   G3  原先滞留的任务被提回 planned 并进入派发（不再无人问津）
 *
 *   SUPERTEAM_PROJECT_ID=... SUPERTEAM_DEMAND_ID=... \
 *     node scripts/e2e/semantic-casting-h12-graph-terminal.mjs
 */
const CP = process.env.SUPERTEAM_CP_URL || "http://127.0.0.1:8080";
const PROJECT_ID =
  process.env.SUPERTEAM_PROJECT_ID || "56de8016-ce14-43d9-95bf-3fca89849b0a";
const DEMAND_ID =
  process.env.SUPERTEAM_DEMAND_ID || "207b5398-5dc2-46a5-8ea8-ee27f6ea3d1a";
const TEMPLATE = process.env.SUPERTEAM_TEMPLATE_KEY || "software_delivery";
const ROLE_KEY = process.env.SUPERTEAM_ROLE_KEY || "network_diagnostics";
const EMPLOYEE_ID =
  process.env.SUPERTEAM_EMPLOYEE_ID || "9a623b40-c9ec-4d7d-99a4-17b1f569b52e";
const WAIT = Number(process.env.SUPERTEAM_PLANNER_WAIT_MS || 420000);

const gates = {};
const log = (m) => console.log(`[h12] ${m}`);
const pass = (k, d) => {
  gates[k] = { ok: true, detail: d };
  log(`PASS ${k} ${d ?? ""}`);
};
const fail = (k, d) => {
  gates[k] = { ok: false, detail: d };
  log(`FAIL ${k} ${d ?? ""}`);
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

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
  return parts.map((c) => c.split(";")[0].trim()).filter(Boolean).join("; ");
}

async function api(cookie, path, { method = "GET", body } = {}) {
  const res = await fetch(`${CP}${path}`, {
    method,
    headers: { cookie, "content-type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = text;
  }
  if (!res.ok) throw new Error(`${method} ${path} -> ${res.status} ${text.slice(0, 300)}`);
  return json;
}

const listTasks = async (cookie) => {
  const r = await api(cookie, `/api/v1/projects/${PROJECT_ID}/tasks?limit=200`);
  const items = r?.items ?? r?.tasks ?? r ?? [];
  return items.filter((t) => (t.demand_id ?? t.demandId) === DEMAND_ID);
};

const terminalOk = (s) => ["completed", "done", "success", "cancelled"].includes(String(s).toLowerCase());

async function main() {
  const cookie = await cpLogin();
  log("cp login ok");

  const before = await listTasks(cookie);
  const strandedBefore = before.filter((t) => t.status === "blocked").map((t) => t.id);
  log(`before: ${before.length} tasks, blocked=${strandedBefore.length}`);

  // —— 提请扩编（真实 HTTP 链路）——
  const decision = await api(cookie, `/api/v1/projects/${PROJECT_ID}/casting-expansions`, {
    method: "POST",
    body: {
      demand_id: DEMAND_ID,
      suggested_role_key: ROLE_KEY,
      needs_external_role: false,
      reason: "H12 图终态回归：对已有完成任务的 demand 再走一次扩编 replan",
      scenario_template_key: TEMPLATE,
    },
  });
  const decisionID = decision.id ?? decision.decision_request_id;
  log(`expansion decision ${decisionID}`);

  // —— 人类批准并选人 ——
  await api(cookie, `/api/v1/projects/${PROJECT_ID}/decisions/${decisionID}/resolve`, {
    method: "POST",
    body: {
      decision: "approved",
      payload: { role_key: ROLE_KEY, digital_employee_id: EMPLOYEE_ID },
    },
  });
  log("approved");

  // —— 扩编越界会按 §7.4 转计划确认;必须替人类确认掉,decompose+派发才会发生。
  // 只认**本轮**开出来的那条:上一轮遗留的 plan_review 其计划版本已被 supersede,
  // 去 resolve 它只会拿到 project conflict(真实踩过)。——
  const approvedAt = Date.now();
  const planDeadline = approvedAt + WAIT;
  let reviewApproved = false;
  while (Date.now() < planDeadline) {
    await sleep(6000);
    const open = await api(cookie, `/api/v1/projects/${PROJECT_ID}/decisions?status=pending&limit=50`).catch(() => null);
    const items = open?.items ?? open?.decisions ?? open ?? [];
    const review = items
      .filter((d) => (d.decision_type ?? d.decisionType) === "plan_review")
      .filter((d) => new Date(d.created_at ?? d.createdAt).getTime() >= approvedAt - 60000)
      .sort((a, b) => new Date(b.created_at ?? b.createdAt) - new Date(a.created_at ?? a.createdAt))[0];
    if (!review) continue;
    await api(cookie, `/api/v1/projects/${PROJECT_ID}/decisions/${review.id}/resolve`, {
      method: "POST",
      body: { decision: "approved", payload: {} },
    });
    log(`plan_review ${review.id} approved`);
    reviewApproved = true;
    break;
  }

  // —— 等 replan 落地:以**新建任务**为准,不看状态抖动 ——
  const beforeIDs = new Set(before.map((t) => t.id));
  // 计划已确认时只需等图落定;没确认到才需要等满(直通路径)。
  const deadline = Date.now() + (reviewApproved ? 90000 : WAIT);
  let after = await listTasks(cookie);
  let replanned = false;
  while (Date.now() < deadline) {
    await sleep(6000);
    after = await listTasks(cookie);
    if (after.some((t) => !beforeIDs.has(t.id))) {
      replanned = true;
      break;
    }
  }
  // replan 全量复用既有任务是**正确**行为(planned_task_key 合并),所以不能以"新建
  // 任务"为准。越界路径以本轮新开的 plan_review 为证,直通路径以新任务为证。
  const newTasks = after.filter((t) => !beforeIDs.has(t.id)).length;
  reviewApproved || replanned
    ? pass("G1", `replan 落地(本轮 plan_review 已确认=${reviewApproved},新建任务=${newTasks})`)
    : fail("G1", `等待 ${WAIT}ms 未观察到 replan`);

  // —— G2：不得存在"阻塞已全解却仍 blocked"的任务 ——
  const graph = await api(cookie, `/api/v1/projects/${PROJECT_ID}/task-graph?demand_id=${DEMAND_ID}`);
  const deps = graph?.dependencies ?? graph?.edges ?? [];
  const byID = new Map(after.map((t) => [t.id, t]));
  const blockerIDOf = (d) => d.blocker_task_id ?? d.blockerTaskId ?? d.depends_on_task_id ?? d.dependsOnTaskId;
  const dependentIDOf = (d) => d.dependent_task_id ?? d.dependentTaskId;
  if (deps.length === 0) {
    // 读不到依赖边就不能宣称"无滞留" —— 上一版正是这样空过的。
    fail("G2", `拿不到依赖边(graph keys=${Object.keys(graph ?? {}).join(",")}),无法判定滞留`);
  } else {
    const stranded = after
      .filter((t) => t.status === "blocked")
      .filter((t) => {
        const blockers = deps
          .filter((d) => dependentIDOf(d) === t.id)
          .map((d) => byID.get(blockerIDOf(d)))
          .filter(Boolean);
        // 阻塞全部已终态 = 不会再有唤醒信号 = 永久滞留
        return blockers.length > 0 && blockers.every((b) => terminalOk(b.status));
      });
    stranded.length === 0
      ? pass("G2", `无阻塞已全解却仍 blocked 的任务(检查 ${deps.length} 条依赖边)`)
      : fail("G2", `永久滞留任务 ${stranded.length} 个: ${stranded.map((t) => `${t.title}(${t.id})`).join(", ")}`);
  }

  // —— G3：原先滞留的任务已离开 blocked ——
  const stillBlocked = strandedBefore.filter((id) => byID.get(id)?.status === "blocked");
  strandedBefore.length === 0
    ? pass("G3", "起始无滞留任务(跳过)")
    : stillBlocked.length === 0
      ? pass("G3", `原滞留 ${strandedBefore.length} 个任务已全部脱离 blocked`)
      : fail("G3", `仍滞留: ${stillBlocked.join(", ")}`);

  const ok = Object.values(gates).every((g) => g.ok);
  console.log(JSON.stringify({ ok, gates }, null, 2));
  process.exit(ok ? 0 : 1);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
