import { describe, expect, it, vi } from "vitest";
import { getHealth } from "./health";

describe("getHealth", () => {
  it("fetches and parses the Control Plane health response", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          status: "ok",
          service: "control-plane",
        }),
        {
          status: 200,
          headers: {
            "content-type": "application/json",
          },
        },
      ),
    );

    await expect(
      getHealth({
        baseUrl: "http://control-plane.local",
        fetcher,
      }),
    ).resolves.toEqual({
      status: "ok",
      service: "control-plane",
    });

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/health",
      expect.objectContaining({
        headers: expect.objectContaining({
          accept: "application/json",
        }),
      }),
    );
  });
});

describe("control-plane OpenAPI contract", () => {
  it("declares the health operation used by the generated client boundary", async () => {
    const { readFile } = await import("node:fs/promises");
    const contractUrl = new URL("../../../contracts/control-plane/openapi.yaml", import.meta.url);
    const contract = await readFile(contractUrl, "utf8");

    expect(contract).toContain("/health:");
    expect(contract).toContain("operationId: getHealth");
    expect(contract).toContain("HealthResponse");
  });
});
