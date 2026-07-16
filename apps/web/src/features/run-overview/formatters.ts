export function formatNumber(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}

// 大数值紧凑显示：≥10 亿转 B、≥100 万转 M（保留最多两位小数，去尾零），其余按千分位原样。
export function formatCompactTokens(value: number) {
  const abs = Math.abs(value);
  if (abs >= 1_000_000_000) return `${trimDecimals(value / 1_000_000_000)}B`;
  if (abs >= 1_000_000) return `${trimDecimals(value / 1_000_000)}M`;
  return formatNumber(value);
}

function trimDecimals(value: number) {
  return String(Number(value.toFixed(2)));
}

// 持续时长：不到 1 分钟 → "不到 1 分钟"；分钟级 → "X 分钟"；小时级 → "X 小时 Y 分钟"；天级 → "X 天 Y 小时"。
export function formatDurationSince(since: string, now: number = Date.now()) {
  const start = new Date(since).getTime();
  if (Number.isNaN(start)) return "";
  const elapsedMinutes = Math.floor(Math.max(0, now - start) / 60_000);
  if (elapsedMinutes < 1) return "不到 1 分钟";
  if (elapsedMinutes < 60) return `${elapsedMinutes} 分钟`;
  const hours = Math.floor(elapsedMinutes / 60);
  if (hours < 24) {
    const minutes = elapsedMinutes % 60;
    return minutes > 0 ? `${hours} 小时 ${minutes} 分钟` : `${hours} 小时`;
  }
  const days = Math.floor(hours / 24);
  const remainderHours = hours % 24;
  return remainderHours > 0 ? `${days} 天 ${remainderHours} 小时` : `${days} 天`;
}

export function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
