import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useControlPlaneEventStream } from "@/hooks/use-control-plane-event-stream";

/** 同一秒内多事件合并成一次 invalidate(spec 2026-07-27 §5 P2-E 节流口径)。 */
const INVALIDATE_THROTTLE_MS = 1_000;

export type ProjectActivityInvalidateOptions = {
  apiBaseUrl: string;
  /** 只响应该项目的活动事件(DigitalEmployeeActivityItem.project_id 过滤)。 */
  projectId: string;
  /** 测试注入 fetcher 时默认关流(与 run-overview 先例一致),显式给 factory 照常开。 */
  enabled?: boolean;
  /** 测试注入用;生产默认用带凭据的原生 EventSource。 */
  eventSourceFactory?: (url: string) => EventSource;
};

/**
 * SSE 驱动的需求流程图刷新(spec 2026-07-27 §5 P2-E):复用既有跨员工活动流
 * `/api/v1/digital-employees/activity/stream`(run-overview 消费先例),事件带
 * 本项目 project_id 时 invalidate 卷宗、图与 launch-detail 查询,让 30s 保底轮询之外
 * 的状态变化秒级到达。流断开由 EventSource 自动重连;节流为 leading+trailing
 * (窗口内首事件立即刷、其余合并成窗口末一次),不丢最后一拍。
 */
export function useProjectActivityInvalidate({
  apiBaseUrl,
  enabled = true,
  eventSourceFactory,
  projectId,
}: ProjectActivityInvalidateOptions) {
  const queryClient = useQueryClient();
  // 节流窗口跨渲染收敛,状态挂 ref 而非回调闭包(闭包随渲染重建)。
  const lastInvalidateRef = useRef(0);
  const trailingTimerRef = useRef<number | undefined>(undefined);

  useEffect(
    () => () => {
      if (trailingTimerRef.current !== undefined) window.clearTimeout(trailingTimerRef.current);
    },
    [],
  );

  const invalidate = () => {
    lastInvalidateRef.current = Date.now();
    void queryClient.invalidateQueries({ queryKey: ["project-task-graph", projectId] });
    void queryClient.invalidateQueries({ queryKey: ["workflow-detail"] });
    // 一单卷宗是需求处所的主读模型;漏掉它会让时间线/待你处理停在旧值,
    // 只能靠 30s 兜底轮询才追上(真实 E2E 揪出)。
    void queryClient.invalidateQueries({ queryKey: ["demand-dossier"] });
  };

  useControlPlaneEventStream({
    apiBaseUrl,
    enabled: enabled && Boolean(projectId),
    eventSourceFactory,
    path: "/api/v1/digital-employees/activity/stream",
    onEvent: (event) => {
      let item: { project_id?: string } | undefined;
      try {
        item = JSON.parse(String(event.data)) as { project_id?: string };
      } catch {
        return;
      }
      if (item?.project_id !== projectId) return;
      const now = Date.now();
      const sinceLast = now - lastInvalidateRef.current;
      if (sinceLast >= INVALIDATE_THROTTLE_MS) {
        invalidate();
        return;
      }
      if (trailingTimerRef.current !== undefined) return;
      trailingTimerRef.current = window.setTimeout(() => {
        trailingTimerRef.current = undefined;
        invalidate();
      }, INVALIDATE_THROTTLE_MS - sinceLast);
    },
  });
}
