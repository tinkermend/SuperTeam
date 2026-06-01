import { describe, expect, it, vi } from "vitest";
import {
  checkPermission,
  createRuntimeScope,
  listAuthzDecisions,
  listAuthzMembers,
  listRuntimeScopes,
  updateRuntimeScope,
} from "./authz";

const tenantId = "00000000-0000-4000-8000-000000000001";
const teamId = "00000000-0000-4000-8000-000000000101";
const runtimeNodeId = "11111111-1111-4111-8111-111111111111";
const scopeId = "22222222-2222-4222-8222-222222222222";
const userId = "33333333-3333-4333-8333-333333333333";

describe("authz center api client", () => {
  it("lists authz decisions with filters and cookie credentials", async () => {
    const responseBody = {
      items: [
        {
          id: "44444444-4444-4444-8444-444444444444",
          tenant_id: tenantId,
          module: "authz",
          action: "task.claim",
          result: "failed",
          actor_type: "runtime_node",
          actor_id: runtimeNodeId,
          resource_type: "task",
          resource_id: "task-1",
          reason: "runtime scope does not cover task",
          created_at: "2026-06-01T00:00:00Z",
        },
      ],
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listAuthzDecisions({
        baseUrl: "http://control-plane.local/",
        fetcher,
        result: "failed",
        action: "task.claim",
        limit: 10,
        offset: 5,
      }),
    ).resolves.toEqual(responseBody);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/authz/decisions?result=failed&action=task.claim&limit=10&offset=5",
      {
        credentials: "include",
        headers: {
          accept: "application/json",
        },
        method: "GET",
      },
    );
  });

  it("creates runtime scopes with JSON body and cookie credentials", async () => {
    const input = {
      runtime_node_id: runtimeNodeId,
      tenant_id: tenantId,
      team_id: teamId,
      scope_type: "team" as const,
      scope_value: teamId,
    };
    const responseBody = {
      scope: {
        id: scopeId,
        tenant_id: tenantId,
        runtime_node_id: runtimeNodeId,
        team_id: teamId,
        scope_type: "team",
        scope_value: teamId,
        status: "active",
        disabled_at: null,
        created_at: "2026-06-01T00:00:00Z",
        updated_at: "2026-06-01T00:00:00Z",
      },
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 201,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(createRuntimeScope({ baseUrl: "http://control-plane.local/", fetcher }, input)).resolves.toEqual(
      responseBody,
    );

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/authz/runtime-scopes", {
      body: JSON.stringify(input),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });

  it("updates runtime scopes with JSON body and cookie credentials", async () => {
    const responseBody = {
      scope: {
        id: scopeId,
        tenant_id: tenantId,
        runtime_node_id: runtimeNodeId,
        team_id: null,
        scope_type: "tenant",
        scope_value: tenantId,
        status: "disabled",
        disabled_at: "2026-06-01T00:01:00Z",
        created_at: "2026-06-01T00:00:00Z",
        updated_at: "2026-06-01T00:01:00Z",
      },
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(updateRuntimeScope({ baseUrl: "http://control-plane.local/", fetcher }, scopeId, "disabled")).resolves.toEqual(
      responseBody,
    );

    expect(fetcher).toHaveBeenCalledWith(`http://control-plane.local/api/authz/runtime-scopes/${scopeId}`, {
      body: JSON.stringify({ status: "disabled" }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "PATCH",
    });
  });

  it("parses runtime scope list responses", async () => {
    const responseBody = {
      nodes: [
        {
          runtime_node_id: runtimeNodeId,
          node_id: "runtime-node-1",
          name: "Runtime Node 1",
          supported_providers: ["codex"],
          max_slots: 4,
          current_load: 1,
          status: "online",
          last_heartbeat_at: "2026-06-01T00:01:00Z",
          recent_denied_reason: null,
          scopes: [
            {
              id: scopeId,
              tenant_id: tenantId,
              runtime_node_id: runtimeNodeId,
              team_id: null,
              scope_type: "tenant",
              scope_value: tenantId,
              status: "active",
              disabled_at: null,
              created_at: "2026-06-01T00:00:00Z",
              updated_at: "2026-06-01T00:00:00Z",
            },
          ],
        },
      ],
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(listRuntimeScopes({ baseUrl: "http://control-plane.local/", fetcher })).resolves.toEqual(responseBody);
  });

  it("parses member list responses", async () => {
    const responseBody = {
      items: [
        {
          user_id: userId,
          username: "operator",
          email: "operator@example.com",
          display_name: "Operator",
          account_status: "active",
          console_access: true,
          recent_denied_reason: null,
          memberships: [
            {
              tenant_id: tenantId,
              team_id: teamId,
              principal_type: "user",
              principal_id: userId,
              role: "admin",
              status: "active",
            },
          ],
        },
      ],
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listAuthzMembers({
        baseUrl: "http://control-plane.local/",
        fetcher,
        limit: 20,
        offset: 0,
      }),
    ).resolves.toEqual(responseBody);
  });

  it("parses permission check responses", async () => {
    const input = {
      actor: {
        type: "user",
        id: userId,
      },
      action: "runtime_scope.manage" as const,
      resource: {
        type: "tenant",
        id: tenantId,
      },
      tenant_id: tenantId,
    };
    const responseBody = {
      allowed: true,
      reason: "allowed",
      matched_rule: "tenant.admin",
      engine: "db",
      snapshot: {
        engine_version: "db-authorizer-v1",
      },
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(checkPermission({ baseUrl: "http://control-plane.local/", fetcher }, input)).resolves.toEqual(responseBody);
  });
});
