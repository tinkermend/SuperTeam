/**
 * 本地开发规范宿主：一律 127.0.0.1，避免 localhost / ::1 与 127.0.0.1
 * 分属不同 cookie jar，导致一边已登录、一边 SSE/badge 401 假「无待办」。
 *
 * 返回应跳转的完整 href；已是规范宿主或非本地则返回 null。
 */
export function canonicalLocalDevHref(href: string): string | null {
  let url: URL;
  try {
    url = new URL(href);
  } catch {
    return null;
  }

  const host = url.hostname.replace(/^\[/, "").replace(/\]$/, "");
  if (host !== "localhost" && host !== "::1") {
    return null;
  }

  url.hostname = "127.0.0.1";
  return url.toString();
}

/**
 * 浏览器入口：若当前是 localhost/::1，replace 到 127.0.0.1 并返回 true（调用方应停止渲染）。
 * 测试可注入 locationLike / assign。
 */
export function redirectToCanonicalLocalDevHost(
  locationLike: Pick<Location, "href"> | undefined =
    typeof window !== "undefined" ? window.location : undefined,
  assign: (url: string) => void = (url) => {
    window.location.replace(url);
  },
): boolean {
  if (!locationLike) {
    return false;
  }
  const next = canonicalLocalDevHref(locationLike.href);
  if (!next) {
    return false;
  }
  assign(next);
  return true;
}
