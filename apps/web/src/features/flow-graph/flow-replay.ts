import type { ProjectTaskGraph, ProjectTaskGraphNode } from "@/lib/api/projects";

/** 回放窗口：真实执行时长线性压缩到固定 8 秒播放（spec 2026-07-27 §5 P2-R）。 */
export const REPLAY_WINDOW_MS = 8_000;

/** 回放时刻某任务所处阶段：未到 started_at 排队、窗口内运行、过 finished_at 落真实终态。 */
export type ReplayPhase = "queued" | "running" | "finished";

export type ReplayTimelineEntry = {
  taskId: string;
  /** 相对 t0 的开始偏移（ms）；无任何时间数据的节点为 undefined，整段回放呈排队中。 */
  startOffsetMs?: number;
  /** 相对 t0 的结束偏移（ms）；无 finished_at 时取总时长——终态只在回放末尾落定。 */
  finishOffsetMs: number;
  /** 权威终态 = 任务当前真实 status（含 running 等非终态），到点原样呈现，不发明状态。 */
  finalStatus: string;
};

export type ReplayTimeline = {
  entries: ReplayTimelineEntry[];
  /** 最早 started_at（ISO）；"压缩自 Y"标注的真实时间轴起点。 */
  t0Iso: string;
  /** 最晚 finished_at（缺失时退回 started_at）（ISO）；真实时间轴终点。 */
  tEndIso: string;
  /** 真实总时长（ms，最小 1 防零除）。 */
  totalDurationMs: number;
};

/** 回放时刻的虚拟状态：status 直接进既有 taskStatusLabel/taskStatusTone/边推导词表。 */
export type ReplayVirtualStatus = {
  phase: ReplayPhase;
  status: string;
};

/**
 * 从 task graph 构建确定性回放时间轴。时间取值口径与 adapter 节点时间区一致：
 * runs[] 投影优先，回退任务节点自身 started_at/finished_at。图中**没有任何**可解析
 * started_at 时返回 undefined（回放按钮禁用的判据）。
 */
export function buildReplayTimeline(graph: ProjectTaskGraph): ReplayTimeline | undefined {
  const runsByTaskId = new Map(graph.runs.map((run) => [run.project_task_id, run]));

  const rawEntries = graph.nodes.map((task) => {
    const run = runsByTaskId.get(task.id);
    return {
      finishMs: parseTimeMs(run?.finished_at ?? task.finished_at),
      startMs: parseTimeMs(run?.started_at ?? task.started_at),
      taskId: task.id,
      finalStatus: task.status,
    };
  });

  const startTimes = rawEntries
    .map((entry) => entry.startMs)
    .filter((value): value is number => value !== undefined);
  if (startTimes.length === 0) return undefined;

  const t0Ms = Math.min(...startTimes);
  const tEndMs = Math.max(
    ...rawEntries
      .map((entry) => entry.finishMs ?? entry.startMs)
      .filter((value): value is number => value !== undefined),
  );
  const totalDurationMs = Math.max(tEndMs - t0Ms, 1);

  return {
    entries: rawEntries.map((entry) => {
      const startOffsetMs = entry.startMs === undefined ? undefined : entry.startMs - t0Ms;
      // finished_at 缺失或倒挂（< started_at 的脏数据）都落到窗口末尾/开始点之后。
      const finishOffsetMs =
        entry.finishMs === undefined
          ? totalDurationMs
          : Math.max(entry.finishMs - t0Ms, startOffsetMs ?? 0);
      return {
        finalStatus: entry.finalStatus,
        finishOffsetMs,
        startOffsetMs,
        taskId: entry.taskId,
      };
    }),
    t0Iso: new Date(t0Ms).toISOString(),
    tEndIso: new Date(tEndMs).toISOString(),
    totalDurationMs,
  };
}

/**
 * 回放进度（0..1）→ 各任务的虚拟状态。"排队中"用词表既有 `queued`、运行窗口用
 * `running`、过 finishOffset 后回到真实终态——三者都是权威状态词，直接复用
 * adapter 的 deriveEdgeActivity/taskStatusLabel 推导，不复制逻辑。
 */
export function replayVirtualStatuses(
  timeline: ReplayTimeline,
  progress: number,
): Map<string, ReplayVirtualStatus> {
  const clamped = Math.min(Math.max(progress, 0), 1);
  const virtualNowMs = clamped * timeline.totalDurationMs;
  const statuses = new Map<string, ReplayVirtualStatus>();

  for (const entry of timeline.entries) {
    if (entry.startOffsetMs === undefined || virtualNowMs < entry.startOffsetMs) {
      statuses.set(entry.taskId, { phase: "queued", status: "queued" });
    } else if (virtualNowMs < entry.finishOffsetMs) {
      statuses.set(entry.taskId, { phase: "running", status: "running" });
    } else {
      statuses.set(entry.taskId, { phase: "finished", status: entry.finalStatus });
    }
  }

  return statuses;
}

/**
 * 把回放时刻的虚拟状态套回 graph，产出可直接喂给 buildFlowGraphElements 的
 * "虚拟图"——边活性/节点样式/降级分支全部走 adapter 既有推导（复用而非复制）。
 * 取舍：
 * - `runs` 置空：run 状态/时间与虚拟时刻不一致会污染边推导与节点 run 徽章；
 * - 起止时间仅在 finished 阶段保留：运行窗口内保留 started_at 会让节点卡的
 *   "已运行 X 分"按真实墙钟计算，误导压缩时间轴的观看者。
 */
export function applyReplayToGraph(
  graph: ProjectTaskGraph,
  timeline: ReplayTimeline,
  progress: number,
): ProjectTaskGraph {
  const statuses = replayVirtualStatuses(timeline, progress);

  return {
    ...graph,
    nodes: graph.nodes.map((task): ProjectTaskGraphNode => {
      const virtual = statuses.get(task.id);
      if (!virtual) return task;
      if (virtual.phase === "finished") {
        return { ...task, status: virtual.status };
      }
      return {
        ...task,
        finished_at: undefined,
        started_at: undefined,
        status: virtual.status,
      };
    }),
    runs: [],
  };
}

function parseTimeMs(value: string | undefined): number | undefined {
  if (!value) return undefined;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? undefined : parsed;
}
