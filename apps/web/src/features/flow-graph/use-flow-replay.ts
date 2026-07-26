import { useEffect, useMemo, useState } from "react";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import {
  applyReplayToGraph,
  buildReplayTimeline,
  REPLAY_WINDOW_MS,
  type ReplayTimeline,
} from "./flow-replay";

/** 进度推进粒度：状态只在离散转折点变化，200ms tick 足够平滑且不压 CPU。 */
const REPLAY_TICK_MS = 200;

type ReplaySession = {
  /**
   * 回放开始瞬间的图快照：回放期间轮询/SSE 到达的新数据只更新 react-query
   * 缓存（即"数据到达先缓存"），画布始终渲染该快照；回放结束丢弃快照，
   * 最新数据自然应用——无须暂停查询本身。
   */
  graph: ProjectTaskGraph;
  startedAtMs: number;
  timeline: ReplayTimeline;
};

/**
 * 回放控制器（spec 2026-07-27 §5 P2-R）：把真实执行时长线性压缩到 8 秒窗口，
 * 按虚拟时刻产出虚拟状态图；播放结束（进度到 1）自动回实时。纯推导在
 * flow-replay.ts，本 hook 只管会话与计时。
 */
export function useFlowReplay(graph: ProjectTaskGraph, enabled: boolean) {
  const [session, setSession] = useState<ReplaySession | undefined>(undefined);
  const [progress, setProgress] = useState(0);

  // 无任何节点时间数据时不可回放（按钮禁用判据）。
  const available = useMemo(
    () => (enabled ? buildReplayTimeline(graph) !== undefined : false),
    [enabled, graph],
  );

  useEffect(() => {
    if (!session) return;
    const timer = window.setInterval(() => {
      const next = (Date.now() - session.startedAtMs) / REPLAY_WINDOW_MS;
      if (next >= 1) {
        // 播放结束自动回实时。
        setSession(undefined);
        setProgress(0);
        return;
      }
      setProgress(next);
    }, REPLAY_TICK_MS);
    return () => window.clearInterval(timer);
  }, [session]);

  const start = () => {
    const timeline = buildReplayTimeline(graph);
    if (!timeline) return;
    setProgress(0);
    setSession({ graph, startedAtMs: Date.now(), timeline });
  };

  const stop = () => {
    setSession(undefined);
    setProgress(0);
  };

  const replayGraph = useMemo(
    () =>
      session
        ? applyReplayToGraph(session.graph, session.timeline, progress)
        : undefined,
    [progress, session],
  );

  return {
    available,
    isReplaying: Boolean(session),
    progress,
    /** 回放中的虚拟图；非回放为 undefined（调用方回退实时 graph）。 */
    replayGraph,
    start,
    stop,
    /** 回放中的时间轴（进度标注用）；非回放为 undefined。 */
    timeline: session?.timeline,
  };
}
