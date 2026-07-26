import { describe, expect, it } from "vitest";
import type { ProjectTaskGraph, ProjectTaskGraphNode } from "@/lib/api/projects";
import {
  applyReplayToGraph,
  buildReplayTimeline,
  replayVirtualStatuses,
} from "./flow-replay";

function taskNode(overrides: Partial<ProjectTaskGraphNode>): ProjectTaskGraphNode {
  return {
    demand_id: "demand-1",
    expected_outputs: [],
    handoff_contract: {},
    id: "task-a",
    input_requirements: {},
    planner_metadata: {},
    project_id: "project-1",
    requires_human_approval: false,
    stage_index: 0,
    status: "completed",
    tenant_id: "tenant-1",
    title: "任务",
    ...overrides,
  };
}

function graphWith(
  nodes: ProjectTaskGraphNode[],
  runs: ProjectTaskGraph["runs"] = [],
): ProjectTaskGraph {
  return {
    blocking_facts: [],
    decision_requests: [],
    edges: [],
    employees: [],
    execution_summaries: [],
    nodes,
    recent_events: [],
    runs,
  };
}

/** 8 分钟串行链：a 0–4 分钟完成，b 4–8 分钟失败。 */
function timedGraph(): ProjectTaskGraph {
  return graphWith([
    taskNode({
      finished_at: "2026-07-27T00:04:00Z",
      id: "task-a",
      started_at: "2026-07-27T00:00:00Z",
      status: "completed",
    }),
    taskNode({
      finished_at: "2026-07-27T00:08:00Z",
      id: "task-b",
      started_at: "2026-07-27T00:04:00Z",
      status: "failed",
    }),
  ]);
}

describe("buildReplayTimeline", () => {
  it("returns undefined when no node has any timing data (replay disabled)", () => {
    expect(buildReplayTimeline(graphWith([taskNode({})]))).toBeUndefined();
  });

  it("builds offsets from the earliest start and prefers run projections over node times", () => {
    const graph = graphWith(
      [
        taskNode({
          // 节点自带时间被 runs[] 投影覆盖（与 adapter 时间区同一口径）。
          finished_at: "2026-07-27T09:00:00Z",
          id: "task-a",
          started_at: "2026-07-27T08:00:00Z",
          status: "completed",
        }),
        taskNode({ id: "task-b", status: "pending" }),
      ],
      [
        {
          project_task_id: "task-a",
          provider_type: "claude_code",
          runtime_node_summary: "节点",
          started_at: "2026-07-27T00:00:00Z",
          finished_at: "2026-07-27T00:01:00Z",
          status: "completed",
        },
      ],
    );

    const timeline = buildReplayTimeline(graph);
    expect(timeline).toBeDefined();
    expect(timeline?.t0Iso).toBe("2026-07-27T00:00:00.000Z");
    expect(timeline?.tEndIso).toBe("2026-07-27T00:01:00.000Z");
    expect(timeline?.totalDurationMs).toBe(60_000);
    const entryA = timeline?.entries.find((entry) => entry.taskId === "task-a");
    expect(entryA?.startOffsetMs).toBe(0);
    expect(entryA?.finishOffsetMs).toBe(60_000);
    // 无时间数据的节点：startOffset undefined，整段回放呈排队中。
    const entryB = timeline?.entries.find((entry) => entry.taskId === "task-b");
    expect(entryB?.startOffsetMs).toBeUndefined();
  });

  it("falls back missing finished_at to the end of the window", () => {
    const timeline = buildReplayTimeline(
      graphWith([
        taskNode({
          finished_at: "2026-07-27T00:10:00Z",
          id: "task-a",
          started_at: "2026-07-27T00:00:00Z",
        }),
        taskNode({
          id: "task-b",
          started_at: "2026-07-27T00:05:00Z",
          status: "running",
        }),
      ]),
    );

    const entryB = timeline?.entries.find((entry) => entry.taskId === "task-b");
    expect(entryB?.finishOffsetMs).toBe(timeline?.totalDurationMs);
  });
});

describe("replayVirtualStatuses", () => {
  it("moves nodes through queued → running → real terminal status along progress", () => {
    const timeline = buildReplayTimeline(timedGraph());
    if (!timeline) throw new Error("timeline should exist");

    // t=0：a 进入运行窗口，b 未到 started_at 排队。
    const atStart = replayVirtualStatuses(timeline, 0);
    expect(atStart.get("task-a")).toEqual({ phase: "running", status: "running" });
    expect(atStart.get("task-b")).toEqual({ phase: "queued", status: "queued" });

    // 中点过后：a 已过 finished_at 落真实终态，b 进入运行窗口。
    const midway = replayVirtualStatuses(timeline, 0.6);
    expect(midway.get("task-a")).toEqual({ phase: "finished", status: "completed" });
    expect(midway.get("task-b")).toEqual({ phase: "running", status: "running" });

    // 结尾：全部落真实终态（failed 原样呈现，不发明状态）。
    const atEnd = replayVirtualStatuses(timeline, 1);
    expect(atEnd.get("task-a")).toEqual({ phase: "finished", status: "completed" });
    expect(atEnd.get("task-b")).toEqual({ phase: "finished", status: "failed" });
  });

  it("clamps out-of-range progress", () => {
    const timeline = buildReplayTimeline(timedGraph());
    if (!timeline) throw new Error("timeline should exist");

    expect(replayVirtualStatuses(timeline, -1).get("task-b")?.phase).toBe("queued");
    expect(replayVirtualStatuses(timeline, 2).get("task-b")?.status).toBe("failed");
  });
});

describe("applyReplayToGraph", () => {
  it("virtualizes node statuses, empties runs and strips times before the finished phase", () => {
    const graph = timedGraph();
    graph.runs = [
      {
        project_task_id: "task-a",
        provider_type: "claude_code",
        runtime_node_summary: "节点",
        status: "completed",
      },
    ];
    const timeline = buildReplayTimeline(graph);
    if (!timeline) throw new Error("timeline should exist");

    const virtual = applyReplayToGraph(graph, timeline, 0);
    // runs 置空：真实 run 状态/时间不得污染回放时刻的边推导与节点徽章。
    expect(virtual.runs).toEqual([]);
    const nodeA = virtual.nodes.find((node) => node.id === "task-a");
    expect(nodeA?.status).toBe("running");
    // 运行窗口内不保留真实起止：避免节点卡按真实墙钟显示"已运行 X 分"。
    expect(nodeA?.started_at).toBeUndefined();
    expect(nodeA?.finished_at).toBeUndefined();

    const done = applyReplayToGraph(graph, timeline, 1);
    const doneA = done.nodes.find((node) => node.id === "task-a");
    expect(doneA?.status).toBe("completed");
    expect(doneA?.started_at).toBe("2026-07-27T00:00:00Z");
    expect(doneA?.finished_at).toBe("2026-07-27T00:04:00Z");
    // 原图不被就地修改。
    expect(graph.nodes[0]?.status).toBe("completed");
    expect(graph.runs).toHaveLength(1);
  });
});
