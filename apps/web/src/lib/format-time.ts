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
