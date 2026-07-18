import { describe, expect, it, vi } from "vitest";
import { ApiRequestError } from "./client";
import {
  bindEmployeeMcpServer,
  createMcpServerDefinition,
  deleteMcpServerDefinition,
  listMcpServerDefinitions,
  type McpServer,
  type UserCredential,
} from "./capabilities";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

describe("capabilities API", () => {
  it("parses DELETE errors through the shared JSON error handling", async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify({ error: "server not found" }), {
          headers: { "content-type": "application/json" },
          status: 404,
        }),
    );

    await expect(
      deleteMcpServerDefinition(
        { baseUrl: "http://control-plane.local", fetcher },
        "missing-server",
      ),
    ).rejects.toThrow(ApiRequestError);
    await expect(
      deleteMcpServerDefinition(
        { baseUrl: "http://control-plane.local", fetcher },
        "missing-server",
      ),
    ).rejects.toThrow("server not found");
  });

  it("creates an MCP registry entry with required env vars", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse(
        {
          id: "mcp-github",
          tenant_id: "tenant-1",
          name: "GitHub MCP",
          server_key: "github",
          description: "",
          transport: "streamable_http",
          url: "https://api.githubcopilot.com/mcp/",
          auth_strategy: "bearer_env",
          required_env_vars: ["GITHUB_TOKEN"],
          optional_env_vars: [],
          tool_allowlist: [],
          risk_level: "medium",
          status: "active",
        },
        201,
      ),
    );

    const created = await createMcpServerDefinition(
      { baseUrl: "http://control-plane.local", fetcher },
      {
        name: "GitHub MCP",
        server_key: "github",
        transport: "streamable_http",
        url: "https://api.githubcopilot.com/mcp/",
        auth_strategy: "bearer_env",
        required_env_vars: ["GITHUB_TOKEN"],
      },
    );

    expect(created.server_key).toBe("github");
    expect(created.required_env_vars).toEqual(["GITHUB_TOKEN"]);
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/mcp-servers",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("lists MCP registry entries", async () => {
    const fetcher = vi.fn(async () => jsonResponse([]));
    await listMcpServerDefinitions({
      baseUrl: "http://control-plane.local",
      fetcher,
    });
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/mcp-servers",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("binds a registered MCP to an employee via the v2 endpoint", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse(
        {
          id: "binding-1",
          tenant_id: "tenant-1",
          digital_employee_id: "employee-1",
          mcp_server_id: "mcp-github",
          status: "blocked_missing_env",
          missing_env_vars: ["GITHUB_TOKEN"],
        },
        201,
      ),
    );

    const binding = await bindEmployeeMcpServer(
      { baseUrl: "http://control-plane.local", fetcher },
      "employee-1",
      { mcp_server_id: "mcp-github", credential_env_var: "GITHUB_TOKEN" },
    );

    expect(binding.missing_env_vars).toEqual(["GITHUB_TOKEN"]);
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee-1/mcp-bindings-v2",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("deletes an MCP registry entry", async () => {
    const fetcher = vi.fn(async () => new Response(null, { status: 204 }));
    await deleteMcpServerDefinition(
      { baseUrl: "http://control-plane.local", fetcher },
      "mcp-github",
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/mcp-servers/mcp-github",
      expect.objectContaining({ method: "DELETE" }),
    );
  });
});
