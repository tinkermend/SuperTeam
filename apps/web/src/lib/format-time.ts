/** Absolute short datetime for zh-CN lists (MM/DD HH:mm). */
export function formatDateTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
  }).format(date);
}

/** Relative time: "刚刚" / "X 分钟前" / "X 小时前" / "X 天前". */
export function formatRelativeTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  const diff = Date.now() - date.getTime();
  if (diff < 0) {
    return formatRelativeFuture(value);
  }
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "刚刚";
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.floor(hours / 24);
  return `${days} 天前`;
}

/** Relative future: "即将" / "X 分钟后" / "X 小时后" / "X 天后". */
export function formatRelativeFuture(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  const diff = date.getTime() - Date.now();
  if (diff <= 0) return "即将";
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "即将";
  if (minutes < 60) return `${minutes} 分钟后`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时后`;
  const days = Math.floor(hours / 24);
  if (days < 14) return `${days} 天后`;
  return formatDateTime(value);
}

/** run 起止 → 人读耗时；缺失/非法时间返回 undefined（未结束不显示耗时）。 */
export function formatRunDuration(
  startedAt: string,
  finishedAt: string | undefined,
): string | undefined {
  if (!finishedAt) return undefined;
  const startMs = Date.parse(startedAt);
  const endMs = Date.parse(finishedAt);
  if (Number.isNaN(startMs) || Number.isNaN(endMs) || endMs < startMs) {
    return undefined;
  }
  const totalSeconds = Math.round((endMs - startMs) / 1000);
  if (totalSeconds < 60) return `${totalSeconds} 秒`;
  const totalMinutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (totalMinutes < 60) {
    return seconds > 0 ? `${totalMinutes} 分 ${seconds} 秒` : `${totalMinutes} 分钟`;
  }
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return minutes > 0 ? `${hours} 小时 ${minutes} 分` : `${hours} 小时`;
}

/**
 * 运行中节点的滚动时长："已运行 X 分"（spec 2026-07-27 §1.2）。
 * 非法/未来时间返回 undefined；不足 1 分钟按分钟粒度显示「不足 1 分钟」。
 */
export function formatElapsedSince(
  startedAt: string,
  nowMs: number = Date.now(),
): string | undefined {
  const startMs = Date.parse(startedAt);
  if (Number.isNaN(startMs) || nowMs < startMs) return undefined;
  const totalMinutes = Math.floor((nowMs - startMs) / 60000);
  if (totalMinutes < 1) return "已运行不足 1 分钟";
  if (totalMinutes < 60) return `已运行 ${totalMinutes} 分钟`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return minutes > 0
    ? `已运行 ${hours} 小时 ${minutes} 分`
    : `已运行 ${hours} 小时`;
}

/** Compare ISO timestamps descending (newest first). Missing values sort last. */
export function compareIsoDesc(left?: string | null, right?: string | null): number {
  const leftMs = left ? Date.parse(left) : Number.NaN;
  const rightMs = right ? Date.parse(right) : Number.NaN;
  const leftOk = !Number.isNaN(leftMs);
  const rightOk = !Number.isNaN(rightMs);
  if (leftOk && rightOk) return rightMs - leftMs;
  if (leftOk) return -1;
  if (rightOk) return 1;
  return 0;
}
