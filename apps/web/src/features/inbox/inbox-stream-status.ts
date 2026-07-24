/**
 * 全局收件箱 SSE 连接态：由 AuthenticatedLayout 上的 useInboxChangeStream 写入，
 * 收件箱列表与侧栏角标据此切换快/慢轮询兜底（断流时 5s，连上后降频）。
 */
export type InboxStreamConnection = "connecting" | "connected" | "disconnected";

export type InboxStreamStatus = {
  connection: InboxStreamConnection;
  /** 最近一次收到 inbox-changed 或成功 onopen 追平的时间戳（ms）。 */
  lastSyncedAt: number | null;
};

const INITIAL_STATUS: InboxStreamStatus = {
  connection: "disconnected",
  lastSyncedAt: null,
};

let status: InboxStreamStatus = INITIAL_STATUS;
const listeners = new Set<() => void>();

export function getInboxStreamStatus(): InboxStreamStatus {
  return status;
}

export function subscribeInboxStreamStatus(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function setInboxStreamStatus(patch: Partial<InboxStreamStatus>): void {
  status = { ...status, ...patch };
  for (const listener of listeners) {
    listener();
  }
}

export function resetInboxStreamStatus(): void {
  status = INITIAL_STATUS;
  for (const listener of listeners) {
    listener();
  }
}

/** 列表页：连上时低频兜底；断流/重连中加速，避免 5s 游标回看漏项后人守空屏。 */
export const INBOX_LIST_REFETCH_CONNECTED_MS = 15_000;
export const INBOX_LIST_REFETCH_DISCONNECTED_MS = 5_000;

/** 侧栏角标：连上时 60s；断流时与列表同频快拉。 */
export const INBOX_BADGE_REFETCH_CONNECTED_MS = 60_000;
export const INBOX_BADGE_REFETCH_DISCONNECTED_MS = 5_000;

export function inboxListRefetchInterval(connection: InboxStreamConnection): number {
  return connection === "connected"
    ? INBOX_LIST_REFETCH_CONNECTED_MS
    : INBOX_LIST_REFETCH_DISCONNECTED_MS;
}

export function inboxBadgeRefetchInterval(connection: InboxStreamConnection): number {
  return connection === "connected"
    ? INBOX_BADGE_REFETCH_CONNECTED_MS
    : INBOX_BADGE_REFETCH_DISCONNECTED_MS;
}
