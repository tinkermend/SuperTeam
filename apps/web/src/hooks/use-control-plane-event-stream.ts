import { useEffect, useRef } from "react";

export type ControlPlaneEventStreamOptions = {
  apiBaseUrl: string;
  /** API 路径(以 / 开头),拼接前会剥掉 base 末尾的斜杠。 */
  path: string;
  enabled?: boolean;
  /** 测试注入用;生产默认带凭据的原生 EventSource。 */
  eventSourceFactory?: (url: string) => EventSource;
  /** 监听的服务端事件名,默认 "activity"。 */
  eventName?: string;
  onOpen?: () => void;
  onError?: () => void;
  onEvent?: (event: MessageEvent) => void;
};

/**
 * Control Plane SSE 长连接的公共骨架:建流、open/error/业务事件接线、卸载清理。
 * 业务侧只给回调,不自己摸 EventSource;断流重连交给 EventSource 原生行为。
 * 回调经 latest-ref 转发、不进连接 effect 依赖——业务回调每次渲染新建也不会重建连接。
 */
export function useControlPlaneEventStream({
  apiBaseUrl,
  path,
  enabled = true,
  eventSourceFactory,
  eventName = "activity",
  onOpen,
  onError,
  onEvent,
}: ControlPlaneEventStreamOptions) {
  const handlersRef = useRef({ onOpen, onError, onEvent });
  useEffect(() => {
    handlersRef.current = { onOpen, onError, onEvent };
  });

  useEffect(() => {
    const base = apiBaseUrl.replace(/\/+$/, "");
    if (!enabled || !path || !base) return;
    const factory =
      eventSourceFactory ?? ((url: string) => new EventSource(url, { withCredentials: true }));
    let source: EventSource | undefined;
    try {
      source = factory(`${base}${path}`);
    } catch {
      // 环境不支持 EventSource 时静默降级,由调用方的轮询兜底。
      return;
    }
    const handleOpen = () => handlersRef.current.onOpen?.();
    const handleError = () => handlersRef.current.onError?.();
    const handleEvent = (event: Event) => handlersRef.current.onEvent?.(event as MessageEvent);
    source.addEventListener("open", handleOpen);
    source.addEventListener("error", handleError);
    source.addEventListener(eventName, handleEvent);
    return () => {
      source?.removeEventListener("open", handleOpen);
      source?.removeEventListener("error", handleError);
      source?.removeEventListener(eventName, handleEvent);
      source?.close();
    };
  }, [apiBaseUrl, path, enabled, eventName, eventSourceFactory]);
}
