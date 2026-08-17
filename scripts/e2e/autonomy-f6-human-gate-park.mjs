/**
 * Focused retest: full_auto + software_delivery human_gate must park at
 * project_task_approval (not policy auto-sign). Quota must be healthy.
 */
import { login, api, apiOk, assert, CP } from "./lib/cp-client.mjs";
import { resolveFixtures } from "./lib/fixtures.mjs";

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const listOf = (p) => (Array.isArray(p) ? p : Array.isArray(p?.items) ? p.items : []);

async function main() {
  const cookie = await login();
  const fixtures = await resolveFixtures(cookie);
  const projectId = fixtures.projectId;
  console.log(`[ok] login CP=${CP} project=${fixtures.project.name}`);

  const stamp = Date.now();
  const created = await api(cookie, "/api/v1/automations", {
    method: "POST",
    body: {
      project_id: projectId,
      name: `F6 hg-park ${stamp}`,
      coordination_mode: "loop",
      scenario_template_key: "software_delivery",
      schedule_kind: "interval",
      interval_seconds: 3600,
      timezone: "Asia/Shanghai",
      demand_title_template: `F6 hg-park ${stamp}`,
      demand_body_template: "retest human_gate park under full_auto after provider quota restore",
      enabled: true,
      autonomy_tier: "full_auto",
      pinned_exit_deliverable: "release_record",
    },
  });
  assert(
    created.status === 200 || created.status === 201,
    `create rule ${created.status} ${String(created.text).slice(0, 300)}`,
  );
  const ruleId = created.json.id;
  console.log(`[ok] rule ${ruleId}`);

  try {
    const triggered = await api(cookie, `/api/v1/automations/${ruleId}/trigger`, {
      method: "POST",
      body: {},
    });
    assert(triggered.ok, `trigger ${triggered.status} ${String(triggered.text).slice(0, 200)}`);

    let demand = null;
    for (let i = 0; i < 40 && !demand; i += 1) {
      const demands = listOf(await apiOk(cookie, `/api/v1/projects/${projectId}/demands?limit=50`));
      demand =
        demands.find(
          (d) =>
            d.source_refs?.automation_rule_id === ruleId ||
            String(d.title || "").includes(`F6 hg-park ${stamp}`),
        ) || null;
      if (!demand) await sleep(1000);
    }
    assert(demand, "demand not created");
    assert(demand.source_refs?.autonomy_tier_snapshot === "full_auto", "missing tier snapshot");
    console.log(`[ok] demand ${demand.id} snapshot=full_auto`);

    // Wait until release human gate parks, or hard-fail on quota/recovery loops.
    let parked = null;
    let lastNote = "";
    for (let i = 0; i < 180; i += 1) {
      const decisions = listOf(
        await apiOk(cookie, `/api/v1/projects/${projectId}/decisions?limit=100`),
      );
      const stamped = decisions.find((d) => {
        const type = d.decision_type;
        const status = String(d.status_snapshot || "").toLowerCase();
        return (
          type === "project_task_approval" &&
          (status === "pending" || status === "requested") &&
          String(d.title_snapshot || "").includes(`F6 hg-park ${stamp}`)
        );
      });
      if (stamped) {
        parked = stamped;
        break;
      }

      const tfr = decisions.find(
        (d) =>
          d.decision_type === "task_failure_recovery" &&
          String(d.status_snapshot || "").toLowerCase() === "pending" &&
          String(d.title_snapshot || "").includes(`F6 hg-park ${stamp}`),
      );
      const summary = String(tfr?.summary_snapshot || "");
      if (tfr && (summary.includes("429") || summary.includes("使用上限"))) {
        throw new Error(`provider quota still blocking: ${summary.slice(0, 200)}`);
      }

      if (parked && String(parked.title_snapshot || "").includes(`F6 hg-park ${stamp}`)) {
        break;
      }
      // Also accept any pending project_task_approval created after fire if demand executing
      // and we can correlate via project_task later.
      if (i % 12 === 11) {
        const demands = listOf(
          await apiOk(cookie, `/api/v1/projects/${projectId}/demands?limit=50`),
        );
        const dem = demands.find((d) => d.id === demand.id);
        lastNote = `demand.status=${dem?.status || "missing"}`;
        console.log(`[..] wait park ${i + 1}/180 ${lastNote}`);
      }
      await sleep(5000);
    }

    // Correlate via tasks API if title match failed.
    if (!parked || !String(parked.title_snapshot || "").includes(`F6 hg-park ${stamp}`)) {
      const tasks = listOf(
        await apiOk(cookie, `/api/v1/projects/${projectId}/tasks?limit=100`),
      ).filter((t) => t.demand_id === demand.id || String(t.title || "").includes(`F6 hg-park ${stamp}`));
      const rha = tasks.filter((t) => t.requires_human_approval);
      console.log(
        `[..] tasks=${tasks.length} rha=${rha.map((t) => `${t.status}:${t.title}`).join(" | ") || "none"}`,
      );
      const decisions = listOf(
        await apiOk(cookie, `/api/v1/projects/${projectId}/decisions?limit=100`),
      );
      const taskIds = new Set(tasks.map((t) => t.id));
      parked = decisions.find(
        (d) =>
          d.decision_type === "project_task_approval" &&
          taskIds.has(d.project_task_id) &&
          ["pending", "requested"].includes(String(d.status_snapshot || "").toLowerCase()),
      );
    }

    assert(parked, `human_gate park not observed (${lastNote})`);
    console.log(
      `[ok] parked project_task_approval ${parked.id} status=${parked.status_snapshot} title=${parked.title_snapshot}`,
    );

    // Hold window: full_auto must NOT policy-resolve risk_approval.
    await sleep(45000);
    const decisions = listOf(
      await apiOk(cookie, `/api/v1/projects/${projectId}/decisions?limit=100`),
    );
    const again = decisions.find((d) => d.id === parked.id);
    assert(again, "parked decision disappeared");
    const status = String(again.status_snapshot || "").toLowerCase();
    assert(
      status === "pending" || status === "requested",
      `expected still pending under full_auto, got ${status}`,
    );
    if (again.resolved_event_id) {
      const events = listOf(
        await apiOk(cookie, `/api/v1/projects/${projectId}/events?limit=100`),
      );
      const ev = events.find((e) => e.id === again.resolved_event_id);
      const resolvedBy = ev?.payload?.payload?.resolved_by || ev?.payload?.comment || "";
      assert(
        !String(resolvedBy).startsWith("policy:"),
        `must not be policy auto-signed, got ${resolvedBy}`,
      );
    }
    console.log(`[ok] still parked after 45s (full_auto did not blind-sign human_gate)`);
  } finally {
    await api(cookie, `/api/v1/automations/${ruleId}`, { method: "DELETE" });
  }
}

main().catch((err) => {
  console.error(`[fail] ${err?.stack || err}`);
  process.exit(1);
});
