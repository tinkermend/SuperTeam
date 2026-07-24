const DEFAULT_CONTROL_PLANE_PORT = "8080";
/** 无浏览器 location 时的兜底；本地规范宿主是 127.0.0.1（见 canonical-local-dev-host）。 */
const DEFAULT_CONTROL_PLANE_URL = `http://127.0.0.1:${DEFAULT_CONTROL_PLANE_PORT}`;

type BrowserLocationLike = Pick<Location, "hostname" | "protocol">;

export function resolveControlPlaneUrl(
  configuredUrl = import.meta.env.VITE_CONTROL_PLANE_URL?.trim(),
  /**
   * `undefined`：用浏览器 location；显式 `null`：无 location（SSR/测试兜底 DEFAULT）。
   * 注意：调用方传 `undefined` 时 JS 默认参数会触发 `getBrowserLocation()`，
   * 因此「无 location」必须传 `null`。
   */
  locationLike: BrowserLocationLike | null | undefined = undefined,
) {
  const location =
    locationLike === undefined ? getBrowserLocation() : locationLike;

  if (!location) {
    return configuredUrl || DEFAULT_CONTROL_PLANE_URL;
  }

  if (configuredUrl) {
    return resolveBrowserControlPlaneUrl(configuredUrl, location);
  }

  return `${location.protocol}//${location.hostname}:${DEFAULT_CONTROL_PLANE_PORT}`;
}

function getBrowserLocation() {
  if (typeof window === "undefined") {
    return undefined;
  }

  return window.location;
}

function resolveBrowserControlPlaneUrl(configuredUrl: string, locationLike: BrowserLocationLike) {
  let parsedUrl: URL;
  try {
    parsedUrl = new URL(configuredUrl);
  } catch {
    return configuredUrl;
  }

  if (isLocalHost(parsedUrl.hostname) && isLocalHost(locationLike.hostname)) {
    parsedUrl.hostname = locationLike.hostname;
    return trimTrailingSlash(parsedUrl.toString());
  }

  return trimTrailingSlash(configuredUrl);
}

function isLocalHost(hostname: string) {
  const normalizedHostname = hostname.replace(/^\[/, "").replace(/\]$/, "");
  return normalizedHostname === "localhost" || normalizedHostname === "127.0.0.1" || normalizedHostname === "::1";
}

function trimTrailingSlash(url: string) {
  return url.replace(/\/+$/, "");
}
