import { describe, expect, it, vi } from "vitest";
import type {
  RuntimeEventList,
  RuntimeEventSeverity,
  RuntimeNodeResponse,
  RuntimeNodeStatus,
  RuntimeOverview,
} from "./runtime";
import {
  approveRuntimeEnrollment,
  getRuntimeNode,
  getRuntimeOverview,
  listRuntimeEnrollments,
  listRuntimeEvents,
  listRuntimeNodes,
  rejectRuntimeEnrollment,
} from "./runtime";

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

describe("getRuntimeOverview", () => {
  it("calls the runtime overview endpoint and parses summary JSON", async () => {
    const overview: RuntimeOverview = {
      summary: {
        online_nodes: 2,
        total_nodes: 3,
        pending_enrollments: 1,
        active_provider_sessions: 4,
        blocked_events: 0,
      },
      pending_enrollments: [],
      nodes: [],
      provider_capabilities: [
        {
          provider_type: "codex",
          node_count: 2,
          available_count: 2,
          healthy_count: 1,
          last_seen_at: "2026-06-05T08:00:00Z",
        },
      ],
      recent_events: [],
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(overview), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      getRuntimeOverview({
        baseUrl: "http://control-plane.local/",
        fetcher,
      }),
    ).resolves.toEqual(overview);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/runtime/overview",
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

describe("listRuntimeEvents", () => {
  it("calls the runtime events endpoint with filters in deterministic order and parses JSON", async () => {
    const events: RuntimeEventList = {
      items: [
        {
          id: "22222222-2222-4222-8222-222222222222",
          tenant_id: "33333333-3333-4333-8333-333333333333",
          event_type: "command_failed",
          severity: "error",
          source: "runtime_command",
          title: "Command failed",
          description: "Provider returned an error",
          node_id: "node-1",
          provider_type: "codex",
          correlation_type: "runtime_command",
          correlation_id: "command-1",
          payload: {
            exit_code: 1,
          },
          created_at: "2026-06-05T08:00:00Z",
        },
      ],
      limit: 25,
      offset: 50,
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(events), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listRuntimeEvents({
        baseUrl: "http://control-plane.local/root/",
        fetcher,
        limit: 25,
        offset: 50,
        event_type: "command_failed",
        severity: "error",
        node_id: "node-1",
        provider_type: "codex",
      }),
    ).resolves.toEqual(events);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/root/api/v1/runtime/events?limit=25&offset=50&event_type=command_failed&severity=error&node_id=node-1&provider_type=codex",
      {
        credentials: "include",
        headers: {
          accept: "application/json",
        },
        method: "GET",
      },
    );
  });

  it("skips empty string filters while preserving zero pagination values", async () => {
    const events: RuntimeEventList = {
      items: [],
      limit: 0,
      offset: 0,
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(events), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listRuntimeEvents({
        baseUrl: "http://control-plane.local",
        fetcher,
        limit: 0,
        offset: 0,
        event_type: "",
        severity: "" as RuntimeEventSeverity,
        node_id: "",
        provider_type: "",
      }),
    ).resolves.toEqual(events);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/runtime/events?limit=0&offset=0",
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

describe("runtime enrollments", () => {
  it("lists runtime enrollments with cookie credentials", async () => {
    const enrollments = [
      {
        id: "11111111-1111-4111-8111-111111111111",
        node_id: "node-1",
        status: "pending",
      },
    ];
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(enrollments), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    await expect(
      listRuntimeEnrollments({
        baseUrl: "http://control-plane.local",
        fetcher,
      }),
    ).resolves.toEqual(enrollments);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/runtime/enrollments",
      expect.objectContaining({ method: "GET", credentials: "include" }),
    );
  });

  it("approves runtime enrollment with cookie credentials", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "11111111-1111-4111-8111-111111111111",
          node_id: "node-1",
          status: "approved",
        }),
        {
          status: 200,
          headers: { "content-type": "application/json" },
        },
      ),
    );

    await approveRuntimeEnrollment(
      { baseUrl: "http://control-plane.local", fetcher },
      "11111111-1111-4111-8111-111111111111",
    );

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/runtime/enrollments/11111111-1111-4111-8111-111111111111/approve",
      expect.objectContaining({ method: "POST", credentials: "include" }),
    );
  });

  it("rejects runtime enrollment with a reason body and cookie credentials", async () => {
    const enrollment = {
      id: "11111111-1111-4111-8111-111111111111",
      node_id: "node-1",
      status: "rejected",
    };
    const fetcher = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(enrollment), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    await expect(
      rejectRuntimeEnrollment(
        { baseUrl: "http://control-plane.local", fetcher },
        "11111111-1111-4111-8111-111111111111",
        "节点身份无法核验",
      ),
    ).resolves.toEqual(enrollment);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/runtime/enrollments/11111111-1111-4111-8111-111111111111/reject",
      {
        body: JSON.stringify({ reason: "节点身份无法核验" }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
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
