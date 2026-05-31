import { describe, expect, it, vi } from "vitest";
import type { RuntimeNodeResponse, RuntimeNodeStatus } from "./runtime";
import { getRuntimeNode, listRuntimeNodes } from "./runtime";

describe("listRuntimeNodes", () => {
  it("calls the runtime nodes endpoint and parses JSON", async () => {
    const status: RuntimeNodeStatus = "online";
    const nodes: RuntimeNodeResponse[] = [
      {
        node_id: "node-1",
        name: "developer-machine",
        supported_providers: ["codex", "opencode"],
        status,
        current_load: 1,
        max_slots: 4,
        metadata: {
          os: "darwin",
        },
        last_heartbeat_at: "2026-05-29T00:00:00Z",
        created_at: "2026-05-29T00:00:00Z",
        updated_at: "2026-05-29T00:01:00Z",
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
        credentials: "include",
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

  it("adds pagination query parameters in a deterministic order", async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify([]), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listRuntimeNodes({
        baseUrl: "http://control-plane.local/root/",
        fetcher,
        limit: 25,
        offset: 75,
      }),
    ).resolves.toEqual([]);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/root/api/v1/runtime/nodes?limit=25&offset=75",
      {
        credentials: "include",
        headers: {
          accept: "application/json",
        },
        method: "GET",
      },
    );
  });
});

describe("getRuntimeNode", () => {
  it("calls the runtime node detail endpoint with an encoded node id and parses JSON", async () => {
    const node: RuntimeNodeResponse = {
      node_id: "node 1/primary",
      name: "developer-machine",
      supported_providers: ["codex"],
      status: "online",
      current_load: 0,
      max_slots: 4,
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(node), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      getRuntimeNode(
        {
          baseUrl: "http://control-plane.local/",
          fetcher,
        },
        "node 1/primary",
      ),
    ).resolves.toEqual(node);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/runtime/nodes/node%201%2Fprimary",
      {
        credentials: "include",
        headers: {
          accept: "application/json",
        },
        method: "GET",
      },
    );
  });
});
