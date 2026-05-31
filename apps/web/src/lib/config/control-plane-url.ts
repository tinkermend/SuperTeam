const DEFAULT_CONTROL_PLANE_PORT = "8080";
const DEFAULT_CONTROL_PLANE_URL = `http://localhost:${DEFAULT_CONTROL_PLANE_PORT}`;

export function resolveControlPlaneUrl(configuredUrl = import.meta.env.VITE_CONTROL_PLANE_URL?.trim()) {
  if (typeof window === "undefined") {
    return configuredUrl || DEFAULT_CONTROL_PLANE_URL;
  }

  if (configuredUrl) {
    return resolveBrowserControlPlaneUrl(configuredUrl);
  }

  return `${window.location.protocol}//${window.location.hostname}:${DEFAULT_CONTROL_PLANE_PORT}`;
}

function resolveBrowserControlPlaneUrl(configuredUrl: string) {
  let parsedUrl: URL;
  try {
    parsedUrl = new URL(configuredUrl);
  } catch {
    return configuredUrl;
  }

  if (isLocalHost(parsedUrl.hostname) && isLocalHost(window.location.hostname)) {
    parsedUrl.hostname = window.location.hostname;
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
