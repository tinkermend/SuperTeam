import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useControlPlaneEventStream } from "@/hooks/use-control-plane-event-stream";
import type { DigitalEmployeeActivityItem } from "@/lib/api/employees";

export type UseChatActivityStreamOptions = {
  apiBaseUrl: string;
  employeeId: string;
  /** 当前进行中的 run;空则只刷新会话列表。 */
  runId?: string;
  enabled?: boolean;
  eventSourceFactory?: (url: string) => EventSource;
};

/**
 * 复用跨员工活动 SSE。匹配当前员工/run 时只刷新该 run 与事件,不整链作废 restore。
 */
export function useChatActivityStream({
  apiBaseUrl,
  employeeId,
  enabled = true,
  eventSourceFactory,
  runId,
}: UseChatActivityStreamOptions) {
  const queryClient = useQueryClient();
  const [live, setLive] = useState(false);

  const active = enabled && Boolean(employeeId) && Boolean(apiBaseUrl);
  useEffect(() => {
    if (!active) setLive(false);
  }, [active]);

  useControlPlaneEventStream({
    apiBaseUrl,
    enabled: active,
    eventSourceFactory,
    path: "/api/v1/digital-employees/activity/stream",
    onError: () => setLive(false),
    onOpen: () => setLive(true),
    onEvent: (event) => {
      let item: DigitalEmployeeActivityItem | undefined;
      try {
        item = JSON.parse(String(event.data)) as DigitalEmployeeActivityItem;
      } catch {
        return;
      }
      if (!item || item.digital_employee_id !== employeeId) {
        return;
      }
      void queryClient.invalidateQueries({ queryKey: ["chat-threads", employeeId] });
      if (runId && item.run_id !== runId) {
        return;
      }
      if (item.run_id) {
        void queryClient.invalidateQueries({ queryKey: ["chat-run", employeeId, item.run_id] });
        void queryClient.invalidateQueries({ queryKey: ["chat-run-events", employeeId, item.run_id] });
      }
    },
  });

  return live;
}
