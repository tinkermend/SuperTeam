/**
 * Clean P1 project junk left by casting E2E (open inbox + terminalize failed planning).
 * Does NOT delete history rows; closes human-task cards and closes failed planning demands.
 *
 *   node scripts/e2e/cleanup-p1-inbox-demands.mjs
 */
const CP = process.env.SUPERTEAM_CP_URL || "http://127.0.0.1:8080";
const PROJECT_ID =
  process.env.SUPERTEAM_PROJECT_ID || "ca82b054-de2d-4810-9a2b-dd41f5e50a2c";
const EMP = {
  developer: "0be393bb-9dfd-48c8-b010-4b5abb114f23",
  reviewer: "7a16f593-9a99-490e-bcab-77bb8b326afa",
  tester: "157b1a2c-b2af-4a08-99f3-f16abe291ed1",
  ops: "9a623b40-c9ec-4d7d-99a4-17b1f569b52e",
};

async function login() {
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

function listOf(j) {
  if (Array.isArray(j)) return j;
  return j?.items || [];
}

function isP1(it) {
  return (
    it.source_project_id === PROJECT_ID ||
    (it.context?.project_id && it.context.project_id === PROJECT_ID) ||
    String(it.primary_surface || "").includes(PROJECT_ID) ||
    String(it.deep_link?.route || "").includes(PROJECT_ID)
  );
}

function kindOf(it) {
  return it.kind || it.context?.decision_type || "";
}

/** Prefer close/reject verbs; never restaff/approve e2e junk. */
function closeAction(it) {
  const k = kindOf(it);
  const keys = (it.actions || []).map((a) => a.key);
  if (k === "planning_failed" && keys.includes("close_demand")) {
    return {
      action: "close_demand",
      comment: "P1 cleanup: close e2e planning_failed demand",
      payload: {},
    };
  }
  if (k === "planning_gap" && keys.includes("rejected")) {
    return {
      action: "rejected",
      comment: "P1 cleanup: close e2e planning_gap",
      payload: {},
    };
  }
  if (k === "casting_expansion" && keys.includes("rejected")) {
    return {
      action: "rejected",
      comment: "P1 cleanup: reject stale casting_expansion",
      payload: {},
    };
  }
  if (k === "plan_review" && keys.includes("rejected")) {
    return {
      action: "rejected",
      comment: "P1 cleanup: reject stale plan_review",
      payload: {},
    };
  }
  if (
    (k === "dispatch_release" || k === "downstream_release") &&
    keys.includes("rejected")
  ) {
    return {
      action: "rejected",
      comment: "P1 cleanup: reject e2e dispatch gate",
      payload: {},
    };
  }
  if (k === "acceptance_sign" && keys.includes("rejected")) {
    return {
      action: "rejected",
      comment: "P1 cleanup: reject e2e acceptance",
      payload: {},
    };
  }
  if (k === "closure_confirm" && keys.includes("rejected")) {
    return {
      action: "rejected",
      comment: "P1 cleanup: reject premature closure_confirm",
      payload: {},
    };
  }
  if (
    (k === "project_task_recovery" || k === "task_failure_recovery") &&
    keys.includes("rejected")
  ) {
    return {
      action: "rejected",
      comment: "P1 cleanup: close recovery card",
      payload: {},
    };
  }
  if (k === "project_task_clarification") {
    if (keys.includes("rejected")) {
      return {
        action: "rejected",
        comment: "P1 cleanup: close clarification",
        payload: {},
      };
    }
    if (keys.includes("cancelled")) {
      return {
        action: "cancelled",
        comment: "P1 cleanup",
        payload: {},
      };
    }
  }
  // generic fallback
  if (keys.includes("rejected")) {
    return {
      action: "rejected",
      comment: "P1 cleanup",
      payload: {},
    };
  }
  if (keys.includes("close_demand")) {
    return {
      action: "close_demand",
      comment: "P1 cleanup",
      payload: {},
    };
  }
  return null;
}

async function main() {
  const cookie = await login();
  const report = { closed: [], failed: [], skipped: [], castings: null };

  // Reset castings to healthy baseline
  let r = await api(cookie, `/api/v1/projects/${PROJECT_ID}/castings`, {
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
  console.log(`cast software_delivery → ${r.status}`);
  r = await api(cookie, `/api/v1/projects/${PROJECT_ID}/castings`, {
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
  console.log(`cast incident_response (distinct SoD) → ${r.status}`);
  report.castings = { software_delivery: "ok", incident_response: "distinct" };

  // Role baseline
  await api(cookie, `/api/v1/digital-employees/${EMP.developer}/roles`, {
    method: "PUT",
    body: { role_keys: ["developer", "operator", "diagnostician"] },
  });
  await api(cookie, `/api/v1/digital-employees/${EMP.reviewer}/roles`, {
    method: "PUT",
    body: { role_keys: ["reviewer", "verifier"] },
  });
  await api(cookie, `/api/v1/digital-employees/${EMP.tester}/roles`, {
    method: "PUT",
    body: { role_keys: ["tester"] },
  });
  await api(cookie, `/api/v1/digital-employees/${EMP.ops}/roles`, {
    method: "PUT",
    body: { role_keys: ["collector", "analyst", "diagnostician"] },
  });

  // Multi-pass close inbox (some closes open follow-ups)
  for (let pass = 0; pass < 5; pass++) {
    r = await api(cookie, `/api/v1/inbox/items?view=mine&status=open&limit=100`);
    const open = listOf(r.json).filter(isP1);
    console.log(`pass ${pass + 1}: P1 open inbox ${open.length}`);
    if (!open.length) break;
    let progressed = 0;
    for (const it of open) {
      const act = closeAction(it);
      if (!act) {
        report.skipped.push({
          id: it.id,
          kind: kindOf(it),
          title: it.title,
          actions: (it.actions || []).map((a) => a.key),
        });
        continue;
      }
      const ar = await api(cookie, `/api/v1/inbox/items/${it.id}/actions`, {
        method: "POST",
        body: act,
      });
      const row = {
        id: it.id,
        kind: kindOf(it),
        action: act.action,
        status: ar.status,
        title: (it.title || "").slice(0, 60),
      };
      if (ar.status < 400) {
        report.closed.push(row);
        progressed++;
        console.log(`  closed ${row.kind} ${row.id.slice(0, 8)} → ${act.action}`);
      } else {
        report.failed.push({ ...row, body: ar.text.slice(0, 160) });
        console.log(
          `  FAIL ${row.kind} ${row.id.slice(0, 8)} → ${ar.status} ${ar.text.slice(0, 100)}`,
        );
      }
    }
    if (!progressed) break;
  }

  r = await api(cookie, `/api/v1/projects/${PROJECT_ID}/demands`);
  const demands = listOf(r.json);
  report.demands_after = demands.map((d) => ({
    id: d.id,
    status: d.status,
    title: (d.title || "").slice(0, 50),
  }));

  r = await api(cookie, `/api/v1/inbox/items?view=mine&status=open&limit=100`);
  report.inbox_open_after = listOf(r.json)
    .filter(isP1)
    .map((it) => ({
      id: it.id,
      kind: kindOf(it),
      title: (it.title || "").slice(0, 50),
    }));

  console.log(JSON.stringify(report, null, 2));
  console.log(
    `summary closed=${report.closed.length} failed=${report.failed.length} skipped=${report.skipped.length} inbox_left=${report.inbox_open_after.length}`,
  );
  if (report.failed.length) process.exitCode = 1;
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
