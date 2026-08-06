/**
 * 图终态断言（收口批 C10 承重）。
 *
 * **数据源必须是 /task-graph 的 nodes+edges**，不是 /tasks。
 * `GET /projects/{id}/tasks` 的 ProjectTask 里根本没有依赖字段，用它做断言时
 * 「blocker 已全解却仍 blocked」这个条件永远无法被求值——真有滞留任务也抓不到，
 * 反而会把合法的 blocked 任务误报成滞留。批三就是靠零依赖断言「绿着」漏过去的。
 *
 * 边形状（实测）：{ dependent_task_id, blocker_task_id, edge_status }
 */

const TERMINAL = new Set([
  "completed",
  "done",
  "success",
  "cancelled",
  "failed",
  "planning_failed",
]);

function statusOf(node) {
  return String(node?.status ?? node?.Status ?? "").toLowerCase();
}

/**
 * 拉取一个 demand 的任务图。demandId 省略时按 coordination_job_id 取。
 * @returns {Promise<{nodes: object[], edges: object[]}>}
 */
export async function fetchTaskGraph(apiOk, cookie, projectId, { demandId, coordinationJobId } = {}) {
  const params = new URLSearchParams();
  if (demandId) params.set("demand_id", demandId);
  if (coordinationJobId) params.set("coordination_job_id", coordinationJobId);
  const qs = params.toString();
  const graph = await apiOk(
    cookie,
    `/api/v1/projects/${projectId}/task-graph${qs ? `?${qs}` : ""}`,
  );
  return { nodes: graph?.nodes ?? [], edges: graph?.edges ?? [] };
}

/**
 * 断言：不存在「所有 blocker 都已终态、自己却还 blocked」的任务。
 *
 * @param {{nodes: object[], edges: object[]}} graph
 * @param {{ label?: string }} [opts]
 */
export function assertGraphTerminal(graph, opts = {}) {
  const label = opts.label || "graph";
  const nodes = graph?.nodes ?? [];
  const edges = graph?.edges ?? [];
  if (!Array.isArray(nodes) || !Array.isArray(edges)) {
    throw new Error(`[assert-graph:${label}] graph must carry nodes[] and edges[]`);
  }

  const byId = new Map();
  for (const n of nodes) {
    if (n?.id) byId.set(n.id, n);
  }
  const blockersOf = new Map();
  for (const e of edges) {
    const dep = e?.dependent_task_id ?? e?.dependentTaskId;
    const blocker = e?.blocker_task_id ?? e?.blockerTaskId;
    if (!dep || !blocker) continue;
    if (!blockersOf.has(dep)) blockersOf.set(dep, []);
    blockersOf.get(dep).push(blocker);
  }

  const stale = [];
  for (const node of nodes) {
    if (statusOf(node) !== "blocked") continue;
    const blockers = blockersOf.get(node.id) ?? [];
    if (blockers.length === 0) {
      // blocked 却没有任何入边：解锁信号永远不会到来。
      stale.push({ id: node.id, title: node.title, reason: "blocked_without_edges" });
      continue;
    }
    const unresolved = blockers.filter((bid) => {
      const b = byId.get(bid);
      if (!b) return true; // blocker 不在图里 → 保守视为未解
      return !TERMINAL.has(statusOf(b));
    });
    if (unresolved.length === 0) {
      stale.push({
        id: node.id,
        title: node.title,
        reason: "blockers_all_terminal",
        blockers,
      });
    }
  }

  if (stale.length > 0) {
    const detail = stale
      .map((s) => `${s.id}(${s.title ?? ""}):${s.reason}`)
      .join("; ");
    throw new Error(`[assert-graph:${label}] stale blocked task(s): ${detail}`);
  }
  return { ok: true, nodes: nodes.length, edges: edges.length };
}

/**
 * 反向自检夹具：把一个真实图改造成「blocker 已完成、下游仍 blocked」的形态。
 * 用真实边结构（dependent_task_id / blocker_task_id），不是臆造字段名——
 * 断言库只有在能吃真实形状时，绿灯才有意义。
 */
export function plantStaleBlockedFixture(graph) {
  const blockerId = "00000000-0000-0000-0000-0000000000aa";
  const blockedId = "00000000-0000-0000-0000-0000000000bb";
  return {
    nodes: [
      ...(graph?.nodes ?? []),
      { id: blockerId, title: "反向夹具·上游", status: "completed" },
      { id: blockedId, title: "反向夹具·滞留下游", status: "blocked" },
    ],
    edges: [
      ...(graph?.edges ?? []),
      {
        dependent_task_id: blockedId,
        blocker_task_id: blockerId,
        edge_status: "completed",
      },
    ],
  };
}

export function assertNoSilentTaskLoss(nodes, expectedKeys, failureEvents = [], opts = {}) {
  const label = opts.label || "dispatch";
  if (!expectedKeys || expectedKeys.length === 0) return { ok: true };
  const present = new Set(
    (nodes || [])
      .map((t) => t.planned_task_key ?? t.plannedTaskKey)
      .filter(Boolean),
  );
  const failed = new Set(
    (failureEvents || [])
      .map((e) => e.planned_task_key ?? e.plannedTaskKey ?? e.key)
      .filter(Boolean),
  );
  const missing = expectedKeys.filter((k) => !present.has(k) && !failed.has(k));
  if (missing.length > 0) {
    throw new Error(
      `[assert-graph:${label}] silent task loss for keys: ${missing.join(", ")}`,
    );
  }
  return { ok: true };
}
