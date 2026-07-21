import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export type UseInboxChangeStreamOptions = {
  /** 覆盖默认 Control Plane base URL（测试注入）。 */
  apiBaseUrl?: string;
  /**
   * 测试注入用 EventSource 工厂。生产默认 `new EventSource(url, { withCredentials: true })`。
   * 组件测试注入了 fetcher、又不想开真实流时：不传 factory 且 `enabled: false`。
   */
  eventSourceFactory?: (url: string) => EventSource;
  /** 默认 true；未登录壳层或测试可关。 */
  enabled?: boolean;
};

/**
 * 全局收件箱 SSE 脏通知：服务端探测到可见范围内变更时推 `inbox-changed`，
 * 收到即 invalidate 列表与侧栏角标。流断开由 EventSource 自动重连。
 *
 * 应挂在已登录布局（每会话一条长连接），收件箱页不要再重复建流。
 */
export function useInboxChangeStream(options: UseInboxChangeStreamOptions = {}) {
  const queryClient = useQueryClient();
  const apiBaseUrl = options.apiBaseUrl ?? resolveControlPlaneUrl();
  const enabled = options.enabled ?? true;
  const eventSourceFactory = options.eventSourceFactory;

  useEffect(() => {
    if (!enabled) return;
    const factory =
      eventSourceFactory ?? ((url: string) => new EventSource(url, { withCredentials: true }));
    let source: EventSource | undefined;
    try {
      source = factory(`${apiBaseUrl}/api/v1/inbox/stream`);
    } catch {
      return;
    }
    const onChanged = () => {
      void queryClient.invalidateQueries({ queryKey: ["inbox-items"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox-badge"] });
    };
    source.addEventListener("inbox-changed", onChanged);
    return () => {
      source?.removeEventListener("inbox-changed", onChanged);
      source?.close();
    };
  }, [apiBaseUrl, enabled, eventSourceFactory, queryClient]);
}
