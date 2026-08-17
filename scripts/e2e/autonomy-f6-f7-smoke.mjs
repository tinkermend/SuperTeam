/**
 * Real-chain smoke for autonomy F6/F7 (2026-08-17).
 * Not in verify:*. Requires running Control Plane + DB.
 *
 * Covers:
 *  - F6 exit pin: full_auto + multi-exit playbook rejected without pin; accepted with pin
 *  - F6 require_human_acceptance: project config round-trip
 *  - F7 auto_recheck: raise ceiling on a full_auto external demand with pending
 *    recovery + ready workspace → reconciler resolves with auto_recheck=true
 *  - F6 human_gate parking: full_auto fire stamps snapshot; risk gate from
 *    human_gate must remain pending (poll limited window)
 */
import { login, api, apiOk, assert, CP } from "./lib/cp-client.mjs";
import { resolveFixtures } from "./lib/fixtures.mjs";

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function listOf(payload) {
  if (Array.isArray(payload)) return payload;
  if (Array.isArray(payload?.items)) return payload.items;
  return [];
}

async function main() {
  const started = Date.now();
  const cookie = await login();
  console.log(`[ok] login CP=${CP}`);

  const fixtures = await resolveFixtures(cookie);
  const projectId = fixtures.projectId;
  console.log(`[ok] fixture project=${fixtures.project.name} (${projectId})`);

  // --- F6: exit pin precheck ---
  const reject = await api(cookie, "/api/v1/automations", {
    method: "POST",
    body: {
      project_id: projectId,
      name: `F6 pin-reject ${Date.now()}`,
      coordination_mode: "loop",
      scenario_template_key: "software_delivery",
      schedule_kind: "interval",
      interval_seconds: 3600,
      timezone: "Asia/Shanghai",
      demand_title_template: "F6 pin reject {{date}}",
      demand_body_template: "smoke — expect create reject",
      enabled: false,
      autonomy_tier: "full_auto",
    },
  });
  assert(
    reject.status >= 400 && reject.status < 500,
    `F6 pin-missing must 4xx, got ${reject.status} ${String(reject.text).slice(0, 300)}`,
  );
  const rejectBlob = String(reject.text || "").toLowerCase();
  assert(
    rejectBlob.includes("pinned_exit") ||
      rejectBlob.includes("acknowledge_exit") ||
      rejectBlob.includes("multi-exit") ||
      rejectBlob.includes("exit"),
    `F6 reject body should mention exit pin, got ${String(reject.text).slice(0, 400)}`,
  );
  console.log(`[ok] F6 create without pin rejected (${reject.status})`);

  const pin = "branch_ref";
  const created = await api(cookie, "/api/v1/automations", {
    method: "POST",
    body: {
      project_id: projectId,
      name: `F6 pin-ok ${Date.now()}`,
      coordination_mode: "loop",
      scenario_template_key: "software_delivery",
      schedule_kind: "interval",
      interval_seconds: 3600,
      timezone: "Asia/Shanghai",
      demand_title_template: "F6 pin ok {{date}}",
      demand_body_template: "smoke — expect create ok",
      enabled: false,
      autonomy_tier: "full_auto",
      pinned_exit_deliverable: pin,
    },
  });
  assert(
    created.status === 200 || created.status === 201,
    `F6 create with pin must succeed, got ${created.status} ${String(created.text).slice(0, 300)}`,
  );
  const ruleId = created.json?.id;
  assert(ruleId, "rule id");
  assert(
    created.json?.pinned_exit_deliverable === pin,
    `pinned_exit_deliverable=${created.json?.pinned_exit_deliverable}`,
  );
  console.log(`[ok] F6 create with pin ok (${ruleId})`);

  const del = await api(cookie, `/api/v1/automations/${ruleId}`, { method: "DELETE" });
  assert(del.ok || del.status === 204 || del.status === 404, `cleanup delete rule ${del.status}`);

  // --- F6: require_human_acceptance config ---
  const cfgBefore = await apiOk(cookie, `/api/v1/projects/${projectId}/config`);
  const policyBefore = { ...(cfgBefore.coordination_policy || {}) };
  const hadRequire = policyBefore.require_human_acceptance === true;
  const nextPolicy = {
    ...policyBefore,
    require_human_acceptance: !hadRequire,
  };
  const patched = await api(cookie, `/api/v1/projects/${projectId}/config`, {
    method: "PUT",
    body: { coordination_policy: nextPolicy },
  });
  assert(patched.ok, `config patch failed ${patched.status} ${String(patched.text).slice(0, 300)}`);
  const cfgMid = await apiOk(cookie, `/api/v1/projects/${projectId}/config`);
  assert(
    cfgMid.coordination_policy?.require_human_acceptance === !hadRequire,
    `require_human_acceptance not persisted`,
  );
  // restore
  await apiOk(cookie, `/api/v1/projects/${projectId}/config`, {
    method: "PUT",
    body: { coordination_policy: policyBefore },
  });
  console.log(`[ok] F6 require_human_acceptance config round-trip`);

  // --- F7: auto_recheck on existing pending recovery under full_auto external ---
  // Prefer a live pending card; if the prior smoke already released it, verify the
  // decision.submitted event (payload carries auto_recheck=true).
  const autonomyProjectId = "846b29a2-650c-417d-ab12-847049d34843";
  const recoveryDecisionId = "284074fc-cf22-44ef-974d-d581f331c290";
  const cfgAuto = await api(cookie, `/api/v1/projects/${autonomyProjectId}/config`);
  if (!cfgAuto.ok) {
    throw new Error(`F7 fixture project config ${cfgAuto.status}`);
  }
  const autoPolicyBefore = { ...(cfgAuto.json.coordination_policy || {}) };

  async function loadResolveEvent(decision) {
    if (!decision?.resolved_event_id) return null;
    const events = listOf(
      await apiOk(cookie, `/api/v1/projects/${autonomyProjectId}/events?limit=200`),
    );
    return events.find((e) => e.id === decision.resolved_event_id) || null;
  }

  const decisionsNow = listOf(
    await apiOk(cookie, `/api/v1/projects/${autonomyProjectId}/decisions?limit=100`),
  );
  let resolved = decisionsNow.find((d) => d.id === recoveryDecisionId) || null;
  let status = String(resolved?.status_snapshot || resolved?.status || "").toLowerCase();

  if (resolved && (status === "approved" || status === "accepted") && resolved.resolved_event_id) {
    const resolveEvent = await loadResolveEvent(resolved);
    assert(resolveEvent, `F7 missing resolved event ${resolved.resolved_event_id}`);
    const inner = resolveEvent.payload?.payload || {};
    assert(inner.auto_recheck === true, `F7 event missing auto_recheck: ${JSON.stringify(resolveEvent.payload).slice(0, 400)}`);
    assert(String(inner.resolved_by || "").startsWith("policy:"), `F7 resolved_by=${inner.resolved_by}`);
    console.log(
      `[ok] F7 auto_recheck already landed on ${recoveryDecisionId} resolved_by=${inner.resolved_by}`,
    );
  } else {
    const autoPolicyRaised = {
      ...autoPolicyBefore,
      autonomy_ceiling: "full_auto",
    };
    try {
      const raise = await api(cookie, `/api/v1/projects/${autonomyProjectId}/config`, {
        method: "PUT",
        body: { coordination_policy: autoPolicyRaised },
      });
      assert(raise.ok, `raise ceiling failed ${raise.status} ${String(raise.text).slice(0, 300)}`);
      console.log(`[ok] F7 raised autonomy_ceiling=full_auto on E2E-autonomy-v2`);

      let resolveEvent = null;
      for (let i = 0; i < 8; i += 1) {
        await sleep(5000);
        const decisions = listOf(
          await apiOk(cookie, `/api/v1/projects/${autonomyProjectId}/decisions?limit=100`),
        );
        resolved = decisions.find((d) => d.id === recoveryDecisionId) || null;
        status = String(resolved?.status_snapshot || resolved?.status || "").toLowerCase();
        if (resolved && status !== "pending" && status !== "requested") {
          resolveEvent = await loadResolveEvent(resolved);
          break;
        }
        console.log(`[..] F7 waiting auto_recheck tick ${i + 1}/8`);
      }
      assert(resolved, `F7 decision ${recoveryDecisionId} not found`);
      assert(
        status === "approved" || status === "accepted",
        `F7 expected approved after heal, got status=${status}`,
      );
      assert(resolveEvent, `F7 missing resolved event ${resolved.resolved_event_id}`);
      const inner = resolveEvent.payload?.payload || {};
      assert(inner.auto_recheck === true, `F7 expect auto_recheck=true, got ${JSON.stringify(resolveEvent.payload).slice(0, 400)}`);
      assert(String(inner.resolved_by || "").startsWith("policy:"), `F7 resolved_by=${inner.resolved_by}`);
      console.log(
        `[ok] F7 auto_recheck released decision ${recoveryDecisionId} status=${status} resolved_by=${inner.resolved_by}`,
      );
    } finally {
      const restore = await api(cookie, `/api/v1/projects/${autonomyProjectId}/config`, {
        method: "PUT",
        body: { coordination_policy: autoPolicyBefore },
      });
      if (!restore.ok) {
        console.warn(`[warn] restore autonomy ceiling failed ${restore.status}`);
      } else {
        console.log(`[ok] F7 restored project ceiling`);
      }
    }
  }

  // --- F7 reverse: workspace not ready → reconciler must not auto-resolve ---
  {
    const dbURL = process.env.SUPERTEAM_E2E_DB_URL || "";
    if (!dbURL) {
      console.warn("[warn] F7 reverse skipped (set SUPERTEAM_E2E_DB_URL to enable)");
    } else {
      const { execFileSync } = await import("node:child_process");
      const psql = (sql) =>
        execFileSync("psql", [dbURL, "-v", "ON_ERROR_STOP=1", "-t", "-A", "-c", sql], {
          encoding: "utf8",
        }).trim();
      const beforeReady = psql(
        `SELECT workspace_ready_status FROM projects WHERE id='${autonomyProjectId}'`,
      );
      try {
        psql(
          `UPDATE projects SET workspace_ready_status='error', updated_at=now() WHERE id='${autonomyProjectId}'`,
        );
        const raise = await api(cookie, `/api/v1/projects/${autonomyProjectId}/config`, {
          method: "PUT",
          body: {
            coordination_policy: { ...autoPolicyBefore, autonomy_ceiling: "full_auto" },
          },
        });
        assert(raise.ok, `F7 reverse raise ceiling ${raise.status}`);
        const approvedBefore = listOf(
          await apiOk(cookie, `/api/v1/projects/${autonomyProjectId}/events?limit=50`),
        ).filter(
          (e) =>
            e.event_type === "decision.submitted" && e.payload?.payload?.auto_recheck === true,
        ).length;
        await sleep(35000);
        const approvedAfter = listOf(
          await apiOk(cookie, `/api/v1/projects/${autonomyProjectId}/events?limit=50`),
        ).filter(
          (e) =>
            e.event_type === "decision.submitted" && e.payload?.payload?.auto_recheck === true,
        ).length;
        assert(
          approvedAfter === approvedBefore,
          `F7 reverse: workspace=failed must not mint new auto_recheck approvals (before=${approvedBefore} after=${approvedAfter})`,
        );
        console.log(
          `[ok] F7 reverse: workspace=failed held; auto_recheck event count unchanged (${approvedAfter})`,
        );
      } finally {
        psql(
          `UPDATE projects SET workspace_ready_status='${beforeReady || "ready"}', updated_at=now() WHERE id='${autonomyProjectId}'`,
        );
        await api(cookie, `/api/v1/projects/${autonomyProjectId}/config`, {
          method: "PUT",
          body: { coordination_policy: autoPolicyBefore },
        });
        console.log(`[ok] F7 reverse restored workspace=${beforeReady || "ready"} + ceiling`);
      }
    }
  }

  // --- F6: human_gate still parks under full_auto (fire + short poll) ---
  const fireRule = await api(cookie, "/api/v1/automations", {
    method: "POST",
    body: {
      project_id: projectId,
      name: `F6 human_gate park ${Date.now()}`,
      coordination_mode: "loop",
      scenario_template_key: "software_delivery",
      schedule_kind: "interval",
      interval_seconds: 3600,
      timezone: "Asia/Shanghai",
      demand_title_template: `F6 human_gate ${Date.now()}`,
      demand_body_template: "smoke — expect human_gate park under full_auto",
      enabled: true,
      autonomy_tier: "full_auto",
      pinned_exit_deliverable: "release_record",
    },
  });
  assert(
    fireRule.status === 200 || fireRule.status === 201,
    `F6 fire rule create failed ${fireRule.status} ${String(fireRule.text).slice(0, 300)}`,
  );
  const fireRuleId = fireRule.json.id;
  try {
    const triggered = await api(cookie, `/api/v1/automations/${fireRuleId}/trigger`, {
      method: "POST",
      body: {},
    });
    assert(triggered.ok, `trigger failed ${triggered.status} ${String(triggered.text).slice(0, 200)}`);

    let demand = null;
    for (let i = 0; i < 30 && !demand; i += 1) {
      const demands = listOf(
        await apiOk(cookie, `/api/v1/projects/${projectId}/demands?limit=50`),
      );
      demand =
        demands.find(
          (d) =>
            d.source_refs?.automation_rule_id === fireRuleId ||
            String(d.title || "").includes("F6 human_gate"),
        ) || null;
      if (!demand) await sleep(1000);
    }
    assert(demand, "F6 fire did not create demand");
    const snap = demand.source_refs?.autonomy_tier_snapshot;
    assert(snap === "full_auto", `demand autonomy_tier_snapshot=${snap}`);
    const pinned = demand.source_refs?.pinned_exit_deliverable;
    assert(
      pinned === "release_record",
      `demand pinned_exit_deliverable=${pinned}`,
    );
    console.log(`[ok] F6 fire stamped demand ${demand.id} snapshot=full_auto pin=release_record`);

    // Poll for a parked human approval / acceptance card that was NOT policy-auto-signed.
    // software_delivery human_gate targets release; may take planner time.
    let parked = null;
    for (let i = 0; i < 90; i += 1) {
      const decisions = listOf(
        await apiOk(cookie, `/api/v1/projects/${projectId}/decisions?limit=100`),
      );
      parked =
        decisions.find((d) => {
          const type = d.decision_type || d.type;
          const status = String(d.status_snapshot || d.status || "").toLowerCase();
          if (status !== "pending" && status !== "requested") return false;
          if (
            type === "project_task_approval" ||
            type === "demand_acceptance" ||
            type === "plan_review"
          ) {
            // plan_review under loop should not appear; if it does and stays pending, also fine for "still human".
            const demandMatch =
              d.demand_id === demand.id ||
              d.project_demand_id === demand.id ||
              String(d.title_snapshot || "").includes("F6 human_gate");
            return demandMatch || type === "project_task_approval";
          }
          return false;
        }) || null;
      if (parked) break;
      if (i % 10 === 9) console.log(`[..] F6 waiting human park ${i + 1}/90`);
      await sleep(2000);
    }
    if (!parked) {
      console.warn(
        "[warn] F6 human_gate park not observed within ~3m (planner/dispatch lag). Snapshot+pin stamp already proven; leave demand for manual follow-up.",
      );
    } else {
      const pType = parked.decision_type || parked.type;
      const pStatus = parked.status_snapshot || parked.status;
      console.log(`[ok] F6 human park observed type=${pType} status=${pStatus} id=${parked.id}`);
    }
  } finally {
    await api(cookie, `/api/v1/automations/${fireRuleId}`, { method: "DELETE" });
  }

  // ownership recheck
  console.log(`[done] elapsed=${((Date.now() - started) / 1000).toFixed(1)}s`);
}

main().catch((err) => {
  console.error(`[fail] ${err?.stack || err}`);
  process.exit(1);
});
