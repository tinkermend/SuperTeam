import { afterEach, describe, expect, it, vi } from "vitest";
import { resolveControlPlaneUrl } from "./control-plane-url";

const browserLocation = {
  hostname: "127.0.0.1",
  protocol: "http:",
} as Location;

describe("resolveControlPlaneUrl", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("uses the configured remote control plane URL when provided", () => {
    vi.stubEnv("VITE_CONTROL_PLANE_URL", "http://control-plane.local:8080");

    expect(resolveControlPlaneUrl(undefined, browserLocation)).toBe("http://control-plane.local:8080");
  });

  it("keeps a configured local API URL on the current browser host", () => {
    vi.stubEnv("VITE_CONTROL_PLANE_URL", "http://localhost:8080");

    expect(resolveControlPlaneUrl(undefined, browserLocation)).toBe("http://127.0.0.1:8080");
  });

  it("derives the local API URL from the current browser host", () => {
    vi.stubEnv("VITE_CONTROL_PLANE_URL", "");

    expect(resolveControlPlaneUrl(undefined, browserLocation)).toBe("http://127.0.0.1:8080");
  });

  it("falls back to localhost when no browser location is available", () => {
    vi.stubEnv("VITE_CONTROL_PLANE_URL", "");

    expect(resolveControlPlaneUrl(undefined, undefined)).toBe("http://localhost:8080");
  });
});
