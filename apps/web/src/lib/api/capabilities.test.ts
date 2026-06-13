import { describe, expect, it, vi } from "vitest";
import { ApiRequestError } from "./client";
import {
  createEmployeeMcpBinding,
  createTeamMcpServer,
  createUserCredential,
  deleteEmployeeMcpBinding,
  deleteTeamMcpServer,
  listEffectiveMcpServers,
  listEmployeeMcpBindings,
  listTeamMcpServers,
  listUserCredentials,
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
  it("creates a credential with secret input without expecting secret response fields", async () => {
    const credential = {
      id: "credential-1",
      tenant_id: "tenant-1",
      user_id: "user-1",
      name: "ops-token",
      credential_type: "mcp_token",
      last_four: "7890",
      status: "active",
    } satisfies UserCredential;
    const fetcher = vi.fn(async () => jsonResponse(credential, 201));

    const result = await createUserCredential(
      { baseUrl: "http://control-plane.local", fetcher },
      {
        name: "ops-token",
        credential_type: "mcp_token",
        credential_value: "sk-test-7890",
      },
    );

    expect(result).toEqual(credential);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/user-credentials",
      {
        body: JSON.stringify({
          name: "ops-token",
          credential_type: "mcp_token",
          credential_value: "sk-test-7890",
        }),
        credentials: "include",
        headers: { accept: "application/json", "content-type": "application/json" },
        method: "POST",
      },
    );
    expect(result).not.toHaveProperty("credential_value");
    expect(result).not.toHaveProperty("encrypted_value");
  });

  it("lists credentials filtered by credential type", async () => {
    const credentials = [
      {
        id: "credential-1",
        tenant_id: "tenant-1",
        user_id: "user-1",
        name: "ops-token",
        credential_type: "mcp_token",
        last_four: "7890",
        status: "active",
      },
    ] satisfies UserCredential[];
    const fetcher = vi.fn(async () => jsonResponse(credentials));

    await expect(
      listUserCredentials(
        { baseUrl: "http://control-plane.local", fetcher },
        "mcp_token",
      ),
    ).resolves.toEqual(credentials);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/user-credentials?credential_type=mcp_token",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("lists creates and deletes team MCP servers with encoded path segments", async () => {
    const teamServer = {
      id: "server 1/ops",
      tenant_id: "tenant-1",
      team_id: "team 1/ops",
      name: "ops-mcp",
      url: "https://mcp.example.com",
      credential_id: "credential-1",
      credential_name: "ops-token",
      credential_type: "mcp_token",
      credential_last_four: "7890",
      status: "active",
      source_scope: "team",
      inherited: true,
    } satisfies McpServer;
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "DELETE") {
        return new Response(null, { status: 204 });
      }

      if (init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toEqual({
          name: "ops-mcp",
          url: "https://mcp.example.com",
          credential_id: "credential-1",
        });
        return jsonResponse(teamServer, 201);
      }

      return jsonResponse([teamServer]);
    });

    await expect(
      listTeamMcpServers(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/ops",
      ),
    ).resolves.toEqual([teamServer]);
    await expect(
      createTeamMcpServer(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/ops",
        {
          name: "ops-mcp",
          url: "https://mcp.example.com",
          credential_id: "credential-1",
        },
      ),
    ).resolves.toEqual(teamServer);
    await expect(
      deleteTeamMcpServer(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/ops",
        "server 1/ops",
      ),
    ).resolves.toBeUndefined();

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      "http://control-plane.local/api/v1/teams/team%201%2Fops/mcp-servers",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      "http://control-plane.local/api/v1/teams/team%201%2Fops/mcp-servers",
      expect.objectContaining({
        credentials: "include",
        headers: { accept: "application/json", "content-type": "application/json" },
        method: "POST",
      }),
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      3,
      "http://control-plane.local/api/v1/teams/team%201%2Fops/mcp-servers/server%201%2Fops",
      {
        credentials: "include",
        method: "DELETE",
      },
    );
  });

  it("lists creates and deletes employee MCP bindings and lists effective MCP servers", async () => {
    const employeeServer = {
      id: "binding 1/primary",
      tenant_id: "tenant-1",
      digital_employee_id: "employee 1/primary",
      name: "personal-mcp",
      url: "https://personal.example.com",
      credential_id: "credential-1",
      status: "active",
      source_scope: "employee",
      inherited: false,
    } satisfies McpServer;
    const effectiveServer = {
      ...employeeServer,
      source_scope: "team",
      inherited: true,
      team_id: "team-1",
    } satisfies McpServer;
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      if (init?.method === "DELETE") {
        return new Response(null, { status: 204 });
      }

      if (init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toEqual({
          name: "personal-mcp",
          url: "https://personal.example.com",
          credential_id: "credential-1",
        });
        return jsonResponse(employeeServer, 201);
      }

      if (url.pathname.endsWith("/effective-mcp-servers")) {
        return jsonResponse([effectiveServer]);
      }

      return jsonResponse([employeeServer]);
    });

    await expect(
      listEmployeeMcpBindings(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
      ),
    ).resolves.toEqual([employeeServer]);
    await expect(
      createEmployeeMcpBinding(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
        {
          name: "personal-mcp",
          url: "https://personal.example.com",
          credential_id: "credential-1",
        },
      ),
    ).resolves.toEqual(employeeServer);
    await expect(
      deleteEmployeeMcpBinding(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
        "binding 1/primary",
      ),
    ).resolves.toBeUndefined();
    await expect(
      listEffectiveMcpServers(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
      ),
    ).resolves.toEqual([effectiveServer]);

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/mcp-bindings",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      3,
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/mcp-bindings/binding%201%2Fprimary",
      {
        credentials: "include",
        method: "DELETE",
      },
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      4,
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/effective-mcp-servers",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("parses DELETE errors through the shared JSON error handling", async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify({ error: "server not found" }), {
          headers: { "content-type": "application/json" },
          status: 404,
        }),
    );

    await expect(
      deleteTeamMcpServer(
        { baseUrl: "http://control-plane.local", fetcher },
        "team-1",
        "missing-server",
      ),
    ).rejects.toThrow(ApiRequestError);
    await expect(
      deleteTeamMcpServer(
        { baseUrl: "http://control-plane.local", fetcher },
        "team-1",
        "missing-server",
      ),
    ).rejects.toThrow("server not found");
  });
});
