/**
 * Casting E2E suite (收口批 D)：单一入口，取代 12 个 browser-casting-* 脚本。
 *
 *   node scripts/e2e/casting-suite.mjs
 *   node scripts/e2e/casting-suite.mjs --stage=smoke,role-impact,cascade,graph-assert
 *   node scripts/e2e/casting-suite.mjs --stage=sod            # 需真实 planner，较慢
 *   SUPERTEAM_ASSERT_GRAPH_REVERSE=1 node scripts/e2e/casting-suite.mjs --stage=graph-assert
 *
 * 不进 verify:*——需要真实 CP + DB（sod 还需 planner）。见 scripts/e2e/README.md。
 */
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { login, api, apiOk, assert, CP } from "./lib/cp-client.mjs";
import { resolveFixtures } from "./lib/fixtures.mjs";
import {
  assertGraphTerminal,
  fetchTaskGraph,
  plantStaleBlockedFixture,
} from "./lib/assert-graph.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const OUT = join(__dirname, "../../.scratch/e2e-casting-suite");
mkdirSync(OUT, { recursive: true });

const ALL_STAGES = [
  "smoke",
  "role-impact",
  "cascade",
  "graph-assert",
  "automation-fire",
];
// sod 依赖真实 planner，跑一次数分钟，默认不在全量里；显式 --stage=sod 才跑。
const OPT_IN_STAGES = ["sod"];

const SOD_TEMPLATE_KEY = "e2e_sod_probe";

function parseStages() {
  const arg = process.argv.find((a) => a.startsWith("--stage="));
  if (!arg) return ALL_STAGES;
  const raw = arg.slice("--stage=".length).trim();
  if (!raw || raw === "all") return ALL_STAGES;
  if (raw === "everything") return [...ALL_STAGES, ...OPT_IN_STAGES];
  return raw.split(",").map((s) => s.trim()).filter(Boolean);
}

const log = (m) => console.log(`[casting-suite] ${m}`);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const result = {
  ok: false,
  started_at: new Date().toISOString(),
  stages: {},
  errors: [],
};

function listOf(payload) {
  if (Array.isArray(payload)) return payload;
  return payload?.items ?? [];
}

async function listCastings(cookie, projectId, templateKey) {
  const path = templateKey
    ? `/api/v1/projects/${projectId}/castings?template_key=${encodeURIComponent(templateKey)}`
    : `/api/v1/projects/${projectId}/castings`;
  return listOf(await apiOk(cookie, path));
}

async function openCastingAlerts(cookie, projectId) {
  const items = listOf(await apiOk(cookie, "/api/v1/inbox/items?limit=200"));
  return items.filter(
    (i) =>
      i.item_type === "casting_invalidated" &&
      i.status === "open" &&
      (!projectId || i.source_project_id === projectId || i.source_id === projectId),
  );
}

async function stageSmoke(ctx) {
  assert(ctx.cookie, "cookie");
  assert(ctx.fixtures.projectId, "projectId");
  assert(ctx.fixtures.developer, "developer fixture by name/role");
  log(`project=${ctx.fixtures.project.name} (${ctx.fixtures.projectId})`);
  log(`developer=${ctx.fixtures.developer.name} (${ctx.fixtures.developer.id})`);
  return { projectId: ctx.fixtures.projectId };
}

/** C1 预检端点 + C2 未确认拒绝（不改状态）。 */
async function stageRoleImpact(ctx) {
  const developer = ctx.fixtures.developer;
  assert(developer, "developer fixture required");
  const empId = developer.id;
  const held = developer.role_keys ?? [];
  assert(held.includes("developer"), "fixture developer must hold developer role");

  const impact = await apiOk(
    ctx.cookie,
    `/api/v1/digital-employees/${empId}/role-impact?role_keys=developer`,
  );
  assert(Array.isArray(impact.affected_castings), "affected_castings must be an array");
  assert(typeof impact.affected_count === "number", "affected_count must be a number");
  assert(
    impact.affected_count === impact.affected_castings.length,
    "affected_count must match affected_castings length",
  );
  log(`C1 role-impact affected_count=${impact.affected_count}`);

  if (impact.affected_count === 0) {
    throw new Error(
      "C2 需要至少一条 developer 编制作为判别条件；请先在任一项目为 developer 编制该员工",
    );
  }

  const keep = held.filter((k) => k !== "developer");
  const rejected = await api(ctx.cookie, `/api/v1/digital-employees/${empId}/roles`, {
    method: "PUT",
    body: { role_keys: keep, confirm_impact: false },
  });
  assert(rejected.status === 400, `C2 want 400 got ${rejected.status}`);
  assert(
    rejected.json?.code === "casting_impact_requires_confirm",
    `C2 body must carry code, got ${JSON.stringify(rejected.json).slice(0, 200)}`,
  );
  assert(
    (rejected.json?.affected_castings ?? []).length === impact.affected_count,
    "C2 body impact must match the preview endpoint",
  );

  // 未确认的请求必须零副作用。
  const after = await apiOk(ctx.cookie, `/api/v1/digital-employees/${empId}`);
  assert(
    (after.role_keys ?? []).includes("developer"),
    "C2 rejection must not mutate role bindings",
  );
  log("C2 unconfirmed remove rejected with impact, no state change");
  return { affected_count: impact.affected_count };
}

/**
 * C3 完整级联闭环：确认移除 → 编制被删 + 负责人收到告警 → 恢复编制 → 告警关闭。
 * 自恢复：结束时角色绑定与编制回到进入前的状态。
 */
async function stageCascade(ctx) {
  const empId = ctx.fixtures.developer.id;
  const before = await apiOk(ctx.cookie, `/api/v1/digital-employees/${empId}`);
  const heldBefore = before.role_keys ?? [];
  assert(heldBefore.includes("developer"), "cascade stage needs the developer role held");

  const impact = await apiOk(
    ctx.cookie,
    `/api/v1/digital-employees/${empId}/role-impact?role_keys=developer`,
  );
  assert(impact.affected_count > 0, "cascade stage needs at least one affected casting");

  // 逐个受影响 (project, template) 快照现有编制，供恢复。
  const snapshots = [];
  for (const row of impact.affected_castings) {
    const rows = await listCastings(ctx.cookie, row.project_id, row.scenario_template_key);
    snapshots.push({
      projectId: row.project_id,
      templateKey: row.scenario_template_key,
      assignments: rows.map((c) => ({
        role_key: c.role_key,
        digital_employee_id: c.digital_employee_id,
      })),
    });
  }

  const keep = heldBefore.filter((k) => k !== "developer");
  await apiOk(ctx.cookie, `/api/v1/digital-employees/${empId}/roles`, {
    method: "PUT",
    body: { role_keys: keep, confirm_impact: true },
  });
  log(`C3 confirmed removal cascaded ${impact.affected_count} casting row(s)`);

  try {
    for (const snap of snapshots) {
      const rows = await listCastings(ctx.cookie, snap.projectId, snap.templateKey);
      const stillCast = rows.some(
        (c) => c.role_key === "developer" && c.digital_employee_id === empId,
      );
      assert(!stillCast, `C3 casting row must be gone for project ${snap.projectId}`);

      // 读路径自证：可达收口必须把该角色算成缺口。
      const readiness = listOf(
        await apiOk(ctx.cookie, `/api/v1/projects/${snap.projectId}/playbook-readiness`),
      );
      const entry = readiness.find((r) => r.scenario_template_key === snap.templateKey);
      if (entry) {
        const gaps = [
          ...(entry.missing_roles_for_any ?? []),
          ...(entry.next_exit_needs_roles ?? []),
          ...(entry.exits ?? []).flatMap((e) => e.missing_roles ?? []),
        ];
        assert(
          gaps.includes("developer"),
          `C4 readiness must surface developer as a gap for ${snap.templateKey}, got ${JSON.stringify(gaps)}`,
        );
      }

      const alerts = await openCastingAlerts(ctx.cookie, snap.projectId);
      assert(
        alerts.length > 0,
        `C3 owners must get an 编制失效 alert for project ${snap.projectId}`,
      );
      assert(
        (alerts[0].summary ?? "").includes("角色"),
        "C3 alert summary must name the roles",
      );
    }
  } finally {
    // 恢复：先还角色，再还编制。
    await apiOk(ctx.cookie, `/api/v1/digital-employees/${empId}/roles`, {
      method: "PUT",
      body: { role_keys: heldBefore, confirm_impact: true },
    });
    for (const snap of snapshots) {
      await apiOk(ctx.cookie, `/api/v1/projects/${snap.projectId}/castings`, {
        method: "PUT",
        body: {
          scenario_template_key: snap.templateKey,
          assignments: snap.assignments,
        },
      });
    }
  }

  // 重新编制是告警的唯一关闭者——不关就永久滞留。
  for (const snap of snapshots) {
    const stillOpen = await openCastingAlerts(ctx.cookie, snap.projectId);
    assert(
      stillOpen.length === 0,
      `C8 re-casting must resolve 编制失效 alerts for ${snap.projectId}, still open: ${stillOpen.length}`,
    );
  }
  log("C3/C8 cascade → alert → re-cast → alert resolved");
  return { affected: impact.affected_count, projects: snapshots.length };
}

/** C10 图终态：必须吃 task-graph 的 nodes+edges，/tasks 没有依赖字段。 */
async function stageGraphAssert(ctx) {
  const projectId = ctx.fixtures.projectId;
  const demands = listOf(
    await apiOk(ctx.cookie, `/api/v1/projects/${projectId}/demands?limit=50`),
  );

  const reverse = process.env.SUPERTEAM_ASSERT_GRAPH_REVERSE === "1";
  if (reverse) {
    const base = demands.length
      ? await fetchTaskGraph(apiOk, ctx.cookie, projectId, { demandId: demands[0].id })
      : { nodes: [], edges: [] };
    let threw = false;
    try {
      assertGraphTerminal(plantStaleBlockedFixture(base), { label: "reverse" });
    } catch (err) {
      threw = true;
      log(`C10 reverse correctly failed: ${err.message}`);
    }
    assert(threw, "C10 reverse: assert-graph MUST fail on a stale blocked task");
    return { reverse: true, threw: true };
  }

  // 单任务 demand 没有边，只查夹具项目会让断言真空通过。扫全租户的项目，
  // 直到至少见过一条依赖边——「检查过 N 个图但一条边都没有」不算通过。
  const projects = listOf(await apiOk(ctx.cookie, "/api/v1/projects?limit=100"));
  const ordered = [
    ...projects.filter((p) => p.id === projectId),
    ...projects.filter((p) => p.id !== projectId),
  ];
  let nodes = 0;
  let edges = 0;
  let graphs = 0;
  for (const proj of ordered.slice(0, 20)) {
    const projDemands =
      proj.id === projectId
        ? demands
        : listOf(await apiOk(ctx.cookie, `/api/v1/projects/${proj.id}/demands?limit=20`));
    for (const demand of projDemands) {
      const graph = await fetchTaskGraph(apiOk, ctx.cookie, proj.id, {
        demandId: demand.id,
      });
      const r = assertGraphTerminal(graph, { label: `demand:${demand.id}` });
      nodes += r.nodes;
      edges += r.edges;
      graphs += 1;
    }
  }
  log(`graph-assert ok graphs=${graphs} nodes=${nodes} edges=${edges}`);
  assert(graphs > 0, "graph-assert needs at least one demand graph");
  assert(
    edges > 0,
    "graph-assert saw zero edges across every graph — dependency data missing, the assertion would be vacuous",
  );
  return { graphs, nodes, edges };
}

/**
 * G8：规则保存时编制完整，运行期编制失效 → fire 失败 + 负责人告警（写明原因）。
 * 自恢复：结束时删掉探针规则并还原编制。
 */
async function stageAutomationFire(ctx) {
  const projectId = ctx.fixtures.projectId;
  const empId = ctx.fixtures.developer.id;
  const templateKey = "software_delivery";
  const before = await apiOk(ctx.cookie, `/api/v1/digital-employees/${empId}`);
  const heldBefore = before.role_keys ?? [];
  const castingBefore = await listCastings(ctx.cookie, projectId, templateKey);
  assert(
    castingBefore.some((c) => c.digital_employee_id === empId),
    "G8 needs the fixture employee cast on software_delivery",
  );
  // 移除角色会级联到**所有**引用它的项目，不只是本 stage 的项目。只还一个项目
  // 等于把别的项目的编制留在被删状态——探针不得把共享 dev 库改坏。
  const impact = await apiOk(
    ctx.cookie,
    `/api/v1/digital-employees/${empId}/role-impact?role_keys=developer`,
  );
  const affectedSnapshots = [];
  for (const row of impact.affected_castings ?? []) {
    const rows = await listCastings(ctx.cookie, row.project_id, row.scenario_template_key);
    affectedSnapshots.push({
      projectId: row.project_id,
      templateKey: row.scenario_template_key,
      assignments: rows.map((c) => ({
        role_key: c.role_key,
        digital_employee_id: c.digital_employee_id,
      })),
    });
  }

  const ruleName = `E2E G8 编制失效 ${Date.now()}`;
  const created = await api(ctx.cookie, "/api/v1/automations", {
    method: "POST",
    body: {
      project_id: projectId,
      name: ruleName,
      coordination_mode: "plan",
      scenario_template_key: templateKey,
      schedule_kind: "cron",
      cron_expr: "0 3 * * *",
      timezone: "Asia/Shanghai",
      demand_title_template: "E2E G8 探针 {{date}}",
      demand_body_template: "自动化探针，不需要真实执行。",
      enabled: true,
    },
  });
  assert(
    created.status === 200 || created.status === 201,
    `G7 rule save with complete casting must succeed, got ${created.status} ${String(created.text).slice(0, 300)}`,
  );
  const ruleId = created.json?.id;
  assert(ruleId, "rule id");
  log(`G7 rule saved with complete casting (${ruleId})`);

  try {
    // 运行期让编制失效：移除该员工的角色（级联删编制）。
    const keep = heldBefore.filter((k) => k !== "developer");
    await apiOk(ctx.cookie, `/api/v1/digital-employees/${empId}/roles`, {
      method: "PUT",
      body: { role_keys: keep, confirm_impact: true },
    });

    const fired = await api(ctx.cookie, `/api/v1/automations/${ruleId}/trigger`, {
      method: "POST",
      body: {},
    });
    assert(fired.ok, `trigger failed: ${fired.status} ${String(fired.text).slice(0, 200)}`);

    let fire = null;
    for (let i = 0; i < 20 && !fire; i += 1) {
      const fires = listOf(await apiOk(ctx.cookie, `/api/v1/automations/${ruleId}/fires`));
      fire = fires.find((f) => f.status === "failed") ?? null;
      if (!fire) await sleep(500);
    }
    assert(fire, "G8 fire must fail when casting is invalid at run time");
    assert(
      fire.error_code === "casting_incomplete",
      `G8 error_code=${fire.error_code} want casting_incomplete`,
    );
    assert(
      /developer/.test(fire.error_message ?? ""),
      `G8 message must name the missing role, got ${fire.error_message}`,
    );
    log(`G8 fire failed with reason: ${fire.error_message}`);

    let alert = null;
    for (let i = 0; i < 20 && !alert; i += 1) {
      const items = listOf(await apiOk(ctx.cookie, "/api/v1/inbox/items?limit=200"));
      alert =
        items.find(
          (it) =>
            it.item_type === "automation_alert" &&
            it.status === "open" &&
            (it.context_payload?.rule_id === ruleId || (it.title ?? "").includes(ruleName)),
        ) ?? null;
      if (!alert) await sleep(500);
    }
    assert(alert, "G8 owners must receive an automation_alert inbox item");
    assert(
      /developer/.test(alert.summary ?? ""),
      `G8 alert must name the failing role, got ${alert.summary}`,
    );
    log("G8 owner alert opened with structured reason");
    return { ruleId, fireId: fire.id, alertId: alert.id };
  } finally {
    await api(ctx.cookie, `/api/v1/digital-employees/${empId}/roles`, {
      method: "PUT",
      body: { role_keys: heldBefore, confirm_impact: true },
    });
    for (const snap of affectedSnapshots) {
      await api(ctx.cookie, `/api/v1/projects/${snap.projectId}/castings`, {
        method: "PUT",
        body: {
          scenario_template_key: snap.templateKey,
          assignments: snap.assignments,
        },
      });
    }
    if (ruleId) {
      // 删规则同时会关掉它的失败告警（否则死链告警永久滞留）。
      await api(ctx.cookie, `/api/v1/automations/${ruleId}`, { method: "DELETE" });
      const leftovers = listOf(await apiOk(ctx.cookie, "/api/v1/inbox/items?limit=200")).filter(
        (it) =>
          it.item_type === "automation_alert" &&
          it.status === "open" &&
          it.context_payload?.rule_id === ruleId,
      );
      assert(
        leftovers.length === 0,
        `deleting a rule must close its alerts, still open: ${leftovers.length}`,
      );
    }
  }
}

/**
 * G12：把同一个人编制到 role_independence 的两个角色上，平台必须给出**耐久的**
 * 产品面（planning_gap 待办 / demand 终态失败），不能只在活动日志里留一行。
 *
 * 需要专用探针剧本：现网 incident_response 的 SoD 对是 operator+verifier，而
 * 全库无人持有 operator（批三 G2 判别条件，不得补发）；software_delivery 的
 * developer+reviewer 已迁到 adversarial_review，不再走经典 role_independence。
 * 探针用 diagnostician+verifier（两侧都有持有人，且有人同时持有两者）。
 */
async function stageSod(ctx) {
  const projectId = ctx.fixtures.projectId;
  const both = ctx.fixtures.employees.find(
    (e) =>
      (e.role_keys ?? []).includes("diagnostician") &&
      (e.role_keys ?? []).includes("verifier"),
  );
  assert(
    both,
    "G12 needs one employee holding BOTH diagnostician and verifier (违规夹具)",
  );

  await ensureSodProbeTemplate(ctx.cookie);

  const castingBefore = await listCastings(ctx.cookie, projectId, SOD_TEMPLATE_KEY);
  await apiOk(ctx.cookie, `/api/v1/projects/${projectId}/castings`, {
    method: "PUT",
    body: {
      scenario_template_key: SOD_TEMPLATE_KEY,
      assignments: [
        { role_key: "diagnostician", digital_employee_id: both.id },
        { role_key: "verifier", digital_employee_id: both.id },
      ],
    },
  });
  log(`G12 cast ${both.name} onto BOTH sides of the role_independence pair`);

  try {
    const demand = await apiOk(ctx.cookie, `/api/v1/projects/${projectId}/demands`, {
      method: "POST",
      body: {
        title: `E2E G12 SoD 探针 ${Date.now()}`,
        content: "诊断一个问题并独立验证结论。",
        coordination_mode: "plan",
        scenario_template_key: SOD_TEMPLATE_KEY,
        source_type: "manual",
      },
    });
    const demandId = demand?.id;
    assert(demandId, "demand id");

    // 只认「耐久且点名 SoD」的产品面：任何 failed 都算通过的话，这条断言就
    // 退化成「随便什么理由挂掉都算 PASS」——正是本批要清掉的假绿。
    const sodWording = /职责分离|role_independence|同一数字员工/;
    const deadlineMs = Number(process.env.SUPERTEAM_SOD_TIMEOUT_MS || 300000);
    const started = Date.now();
    let evidence = null;
    let lastStatus = "";
    while (Date.now() - started < deadlineMs && !evidence) {
      const dossier = await apiOk(
        ctx.cookie,
        `/api/v1/project-demands/${demandId}/dossier`,
      );
      lastStatus = dossier?.demand?.status ?? lastStatus;
      const gap = (dossier?.pending_actions ?? []).find(
        (a) => a.kind === "planning_gap" && sodWording.test(a.title ?? ""),
      );
      if (gap) {
        evidence = {
          kind: "planning_gap",
          demand_status: lastStatus,
          title: gap.title,
        };
        break;
      }
      await sleep(5000);
    }
    assert(
      evidence,
      `G12 no durable SoD surface within ${deadlineMs}ms (demand status=${lastStatus}) — cast-locked role_independence must produce a planning_gap naming 职责分离, not log-only`,
    );
    assert(
      ["failed", "planning_failed"].includes(evidence.demand_status),
      `G12 demand must be terminal, got ${evidence.demand_status}`,
    );
    log(`G12 durable SoD surface: ${evidence.title}`);
    return { demandId, evidence };
  } finally {
    if (castingBefore.length > 0) {
      await api(ctx.cookie, `/api/v1/projects/${projectId}/castings`, {
        method: "PUT",
        body: {
          scenario_template_key: SOD_TEMPLATE_KEY,
          assignments: castingBefore.map((c) => ({
            role_key: c.role_key,
            digital_employee_id: c.digital_employee_id,
          })),
        },
      });
    }
    // 探针留下的 planning_gap 待办要收掉,否则每跑一次就在共享收件箱多压一张卡。
    await closeProbePlanningGaps(ctx.cookie, projectId);
  }
}

/** 收掉 SoD 探针留下的 planning_gap 决策（planning_gap 的封闭值域含 rejected）。 */
async function closeProbePlanningGaps(cookie, projectId) {
  // 注意:该列表端点不按 status 过滤,未决与已决同列——用 resolved_at 判别。
  const decisions = listOf(
    await apiOk(cookie, `/api/v1/projects/${projectId}/decisions?limit=100`).catch(() => []),
  );
  for (const d of decisions) {
    const isProbeGap =
      d.decision_type === "planning_gap" &&
      !d.resolved_at &&
      /职责分离|同一数字员工/.test(d.title_snapshot ?? d.title ?? "");
    if (!isProbeGap) continue;
    await api(cookie, `/api/v1/projects/${projectId}/decisions/${d.id}/resolve`, {
      method: "POST",
      body: { decision: "rejected", comment: "E2E SoD 探针清理" },
    });
  }
}

/** 幂等创建 SoD 探针剧本（已存在则复用）。 */
async function ensureSodProbeTemplate(cookie) {
  const existing = await api(cookie, `/api/v1/scenario-templates/${SOD_TEMPLATE_KEY}`);
  if (existing.ok) return existing.json;
  const spec = {
    spec_version: 2,
    roles: [
      { key: "diagnostician", title: "诊断", required_capabilities: ["incident_triage"] },
      { key: "verifier", title: "验证", required_capabilities: ["incident_triage"] },
    ],
    skeleton: [
      {
        step: "diagnose",
        role: "diagnostician",
        produces_defaults: [{ kind: "conclusion", name: "root_cause" }],
      },
      {
        step: "verify",
        role: "verifier",
        depends_on: ["diagnose"],
        required_inputs_defaults: ["root_cause"],
        produces_defaults: [{ kind: "conclusion", name: "verification_result" }],
      },
    ],
    exits: [{ deliverable: "verification_result", label: "诊断并独立验证" }],
    // 无 when → 该约束始终生效，探针不依赖出口选择。
    constraints: [{ kind: "role_independence", roles: ["diagnostician", "verifier"] }],
  };
  const created = await api(cookie, "/api/v1/scenario-templates", {
    method: "POST",
    body: {
      template_key: SOD_TEMPLATE_KEY,
      name: "E2E SoD 探针",
      description:
        "收口批 G12 专用：验证者与诊断者必须不同人。现网剧本已无可用 SoD 夹具，故独立建探针。",
      spec,
    },
  });
  assert(
    created.ok,
    `create sod probe template failed: ${created.status} ${String(created.text).slice(0, 300)}`,
  );
  log(`G12 probe template ${SOD_TEMPLATE_KEY} created`);
  return created.json;
}

const STAGES = {
  smoke: stageSmoke,
  "role-impact": stageRoleImpact,
  cascade: stageCascade,
  "graph-assert": stageGraphAssert,
  "automation-fire": stageAutomationFire,
  sod: stageSod,
};

async function main() {
  const stages = parseStages();
  log(`stages=${stages.join(",")}`);
  const cookie = await login();
  const fixtures = await resolveFixtures(cookie);
  const ctx = { cookie, fixtures };

  for (const name of stages) {
    const fn = STAGES[name];
    if (!fn) {
      result.errors.push(`unknown stage ${name}`);
      result.stages[name] = { ok: false, error: "unknown stage" };
      continue;
    }
    try {
      log(`→ ${name}`);
      const detail = await fn(ctx);
      result.stages[name] = { ok: true, detail };
      log(`✓ ${name}`);
    } catch (err) {
      result.stages[name] = { ok: false, error: String(err?.message || err) };
      result.errors.push(`${name}: ${err?.message || err}`);
      log(`✗ ${name}: ${err?.message || err}`);
    }
  }

  result.ok = result.errors.length === 0;
  result.finished_at = new Date().toISOString();
  result.cp = CP;
  writeFileSync(join(OUT, "result.json"), JSON.stringify(result, null, 2));
  log(`done ok=${result.ok} → ${join(OUT, "result.json")}`);
  if (!result.ok) process.exitCode = 1;
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
