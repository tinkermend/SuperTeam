import { afterEach, describe, expect, it, vi } from "vitest";

describe("resolveControlPlaneUrl", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
  });

  it("uses the configured remote control plane URL when provided", async () => {
    vi.stubEnv("NEXT_PUBLIC_CONTROL_PLANE_URL", "http://control-plane.local:8080");
    vi.stubGlobal("window", {
      location: {
        hostname: "127.0.0.1",
        protocol: "http:",
      },
    });

    const { resolveControlPlaneUrl } = await import("./control-plane-url");

    expect(resolveControlPlaneUrl()).toBe("http://control-plane.local:8080");
  });

  it("keeps a configured local API URL on the current browser host", async () => {
    vi.stubEnv("NEXT_PUBLIC_CONTROL_PLANE_URL", "http://localhost:8080");
    vi.stubGlobal("window", {
      location: {
        hostname: "127.0.0.1",
        protocol: "http:",
      },
    });

    const { resolveControlPlaneUrl } = await import("./control-plane-url");

    expect(resolveControlPlaneUrl()).toBe("http://127.0.0.1:8080");
  });

  it("derives the local API URL from the current browser host", async () => {
    vi.stubGlobal("window", {
      location: {
        hostname: "127.0.0.1",
        protocol: "http:",
      },
    });

    const { resolveControlPlaneUrl } = await import("./control-plane-url");

    expect(resolveControlPlaneUrl()).toBe("http://127.0.0.1:8080");
  });

  it("falls back to localhost when no browser location is available", async () => {
    vi.stubGlobal("window", undefined);

    const { resolveControlPlaneUrl } = await import("./control-plane-url");

    expect(resolveControlPlaneUrl()).toBe("http://localhost:8080");
  });
});
