import { describe, expect, it, vi } from "vitest";
import { listRuntimeNodes } from "./runtime";

describe("listRuntimeNodes", () => {
  it("calls the runtime nodes endpoint and parses JSON", async () => {
    const nodes = [
      {
        node_id: "node-1",
        name: "developer-machine",
        status: "online",
        current_load: 1,
        max_slots: 4,
      },
    ];
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(nodes), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listRuntimeNodes({
        baseUrl: "http://control-plane.local/",
        fetcher,
      }),
    ).resolves.toEqual(nodes);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/runtime/nodes",
      {
        headers: {
          accept: "application/json",
        },
        method: "GET",
      },
    );
  });

  it("throws when the runtime nodes endpoint returns a non-ok response", async () => {
    const fetcher = vi.fn(async () => new Response("unavailable", { status: 503 }));

    await expect(
      listRuntimeNodes({
        baseUrl: "http://control-plane.local",
        fetcher,
      }),
    ).rejects.toThrow("runtime nodes request failed with status 503");
  });
});
