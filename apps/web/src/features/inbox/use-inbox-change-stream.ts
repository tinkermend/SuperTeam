import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { resetInboxStreamStatus, setInboxStreamStatus } from "./inbox-stream-status";

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

function invalidateInboxQueries(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: ["inbox-items"] });
  void queryClient.invalidateQueries({ queryKey: ["inbox-badge"] });
}

/**
 * 全局收件箱 SSE 脏通知：服务端探测到可见范围内变更时推 `inbox-changed`，
 * 收到即 invalidate 列表与侧栏角标。流断开由 EventSource 自动重连；
 * **每次 onopen（含重连）也立即 invalidate**，不依赖服务端 5s 游标回看——断窗超过
 * slack 的新建项否则会永久漏推，只能靠轮询或手动改筛选才能看见。
 *
 * 应挂在已登录布局（每会话一条长连接），收件箱页不要再重复建流。
 */
export function useInboxChangeStream(options: UseInboxChangeStreamOptions = {}) {
  const queryClient = useQueryClient();
  const apiBaseUrl = options.apiBaseUrl ?? resolveControlPlaneUrl();
  const enabled = options.enabled ?? true;
  const eventSourceFactory = options.eventSourceFactory;

  useEffect(() => {
    if (!enabled) {
      resetInboxStreamStatus();
      return;
    }
    const factory =
      eventSourceFactory ?? ((url: string) => new EventSource(url, { withCredentials: true }));
    let source: EventSource | undefined;
    setInboxStreamStatus({ connection: "connecting" });
    try {
      source = factory(`${apiBaseUrl}/api/v1/inbox/stream`);
    } catch {
      setInboxStreamStatus({ connection: "disconnected" });
      return;
    }
    const onOpen = () => {
      // 重连追平：强制重拉，覆盖 Peek 游标回看不足的断窗。
      setInboxStreamStatus({ connection: "connected", lastSyncedAt: Date.now() });
      invalidateInboxQueries(queryClient);
    };
    const onChanged = () => {
      setInboxStreamStatus({ lastSyncedAt: Date.now() });
      invalidateInboxQueries(queryClient);
    };
    const onError = () => {
      // EventSource 会自动重连；此处只切快轮询，等下次 onopen 再追平。
      setInboxStreamStatus({ connection: "disconnected" });
    };
    source.addEventListener("open", onOpen);
    source.addEventListener("inbox-changed", onChanged);
    source.addEventListener("error", onError);
    return () => {
      source?.removeEventListener("open", onOpen);
      source?.removeEventListener("inbox-changed", onChanged);
      source?.removeEventListener("error", onError);
      source?.close();
      resetInboxStreamStatus();
    };
  }, [apiBaseUrl, enabled, eventSourceFactory, queryClient]);
}
