const DEFAULT_CONTROL_PLANE_PORT = "8080";
const DEFAULT_CONTROL_PLANE_URL = `http://localhost:${DEFAULT_CONTROL_PLANE_PORT}`;

type BrowserLocationLike = Pick<Location, "hostname" | "protocol">;

export function resolveControlPlaneUrl(
  configuredUrl = import.meta.env.VITE_CONTROL_PLANE_URL?.trim(),
  locationLike: BrowserLocationLike | undefined = getBrowserLocation(),
) {
  if (!locationLike) {
    return configuredUrl || DEFAULT_CONTROL_PLANE_URL;
  }

  if (configuredUrl) {
    return resolveBrowserControlPlaneUrl(configuredUrl, locationLike);
  }

  return `${locationLike.protocol}//${locationLike.hostname}:${DEFAULT_CONTROL_PLANE_PORT}`;
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
