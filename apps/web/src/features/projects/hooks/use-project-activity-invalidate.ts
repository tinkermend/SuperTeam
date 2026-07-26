import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";

/** 同一秒内多事件合并成一次 invalidate（spec 2026-07-27 §5 P2-E 节流口径）。 */
const INVALIDATE_THROTTLE_MS = 1_000;

export type ProjectActivityInvalidateOptions = {
  apiBaseUrl: string;
  /** 只响应该项目的活动事件（DigitalEmployeeActivityItem.project_id 过滤）。 */
  projectId: string;
  /** 测试注入 fetcher 时默认关流（与 run-overview 先例一致），显式给 factory 照常开。 */
  enabled?: boolean;
  /** 测试注入用；生产默认用带凭据的原生 EventSource。 */
  eventSourceFactory?: (url: string) => EventSource;
};

/**
 * SSE 驱动的需求流程图刷新（spec 2026-07-27 §5 P2-E）：复用既有跨员工活动流
 * `/api/v1/digital-employees/activity/stream`（run-overview 消费先例），事件带
 * 本项目 project_id 时 invalidate 图与 launch-detail 查询，让 30s 保底轮询之外
 * 的状态变化秒级到达。流断开由 EventSource 自动重连；节流为 leading+trailing
 * （窗口内首事件立即刷、其余合并成窗口末一次），不丢最后一拍。
 */
export function useProjectActivityInvalidate({
  apiBaseUrl,
  enabled = true,
  eventSourceFactory,
  projectId,
}: ProjectActivityInvalidateOptions) {
  const queryClient = useQueryClient();

  useEffect(() => {
    if (!enabled || !projectId) return;
    const factory =
      eventSourceFactory ?? ((url: string) => new EventSource(url, { withCredentials: true }));
    let source: EventSource | undefined;
    try {
      source = factory(`${apiBaseUrl}/api/v1/digital-employees/activity/stream`);
    } catch {
      // 环境不支持 EventSource 时静默降级：30s 保底轮询兜底。
      return;
    }

    let lastInvalidateMs = 0;
    let trailingTimer: number | undefined;
    const invalidate = () => {
      lastInvalidateMs = Date.now();
      void queryClient.invalidateQueries({ queryKey: ["project-task-graph", projectId] });
      void queryClient.invalidateQueries({ queryKey: ["workflow-detail"] });
    };
    const onActivity = (event: MessageEvent) => {
      let item: { project_id?: string } | undefined;
      try {
        item = JSON.parse(String(event.data)) as { project_id?: string };
      } catch {
        return;
      }
      if (item?.project_id !== projectId) return;
      const now = Date.now();
      const sinceLast = now - lastInvalidateMs;
      if (sinceLast >= INVALIDATE_THROTTLE_MS) {
        invalidate();
        return;
      }
      if (trailingTimer !== undefined) return;
      trailingTimer = window.setTimeout(() => {
        trailingTimer = undefined;
        invalidate();
      }, INVALIDATE_THROTTLE_MS - sinceLast);
    };

    source.addEventListener("activity", onActivity as EventListener);
    return () => {
      if (trailingTimer !== undefined) window.clearTimeout(trailingTimer);
      source?.removeEventListener("activity", onActivity as EventListener);
      source?.close();
    };
  }, [apiBaseUrl, enabled, eventSourceFactory, projectId, queryClient]);
}
