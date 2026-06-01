import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { PermissionsCenter } from "@/features/permissions";

const defaultTenantId = "00000000-0000-4000-8000-000000000001";
const teamId = "00000000-0000-4000-8000-000000000101";
const runtimeNodeId = "11111111-1111-4111-8111-111111111111";
const scopeId = "22222222-2222-4222-8222-222222222222";
const actorId = "33333333-3333-4333-8333-333333333333";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>,
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>,
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>,
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>,
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        retry: false,
      },
    },
  });
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: {
      "content-type": "application/json",
    },
    status: 200,
  });
}

type RecordedRequest = {
  body: unknown;
  method: string;
  pathname: string;
};

type AuthzFetcherOptions = {
  runtimeScopeNodes?: unknown[];
};

function createNotFoundResponse(pathname: string) {
  return new Response(JSON.stringify({ error: `unhandled ${pathname}` }), {
    headers: {
      "content-type": "application/json",
    },
    status: 404,
  });
}

function parseRequestBody(init?: RequestInit) {
  if (typeof init?.body !== "string") {
    return undefined;
  }

  return JSON.parse(init.body) as unknown;
}

function createAuthzFetcher(options: AuthzFetcherOptions = {}) {
  const requests: RecordedRequest[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    requests.push({
      body: parseRequestBody(init),
      method,
      pathname: url.pathname,
    });

    switch (url.pathname) {
      case "/api/authz/overview":
        if (method !== "GET") {
          return createNotFoundResponse(url.pathname);
        }
        return jsonResponse({
          engine: {
            engine: "db",
            status: "ok",
          },
          totals: {
            allowed: 0,
            denied: 0,
            denied_rate: 0,
            total: 0,
          },
          recent_events: [],
          top_denied_actions: [],
        });
      case "/api/authz/decisions":
        if (method !== "GET") {
          return createNotFoundResponse(url.pathname);
        }
        return jsonResponse({ items: [] });
      case "/api/authz/runtime-scopes":
        if (method === "GET") {
          return jsonResponse({ nodes: options.runtimeScopeNodes ?? [] });
        }
        if (method === "POST") {
          return jsonResponse({
            scope: {
              id: scopeId,
              ...(parseRequestBody(init) as Record<string, unknown>),
              created_at: "2026-06-01T00:00:00Z",
              status: "active",
              updated_at: "2026-06-01T00:00:00Z",
            },
          });
        }
        return createNotFoundResponse(url.pathname);
      case "/api/authz/members":
        if (method !== "GET") {
          return createNotFoundResponse(url.pathname);
        }
        return jsonResponse({ items: [] });
      case "/api/authz/check":
        if (method !== "POST") {
          return createNotFoundResponse(url.pathname);
        }
        return jsonResponse({
          allowed: true,
          engine: "db",
          matched_rule: "tenant.owner",
          reason: "allowed",
          snapshot: {},
        });
      default:
        if (/^\/api\/authz\/runtime-scopes\/[^/]+$/.test(url.pathname) && method === "PATCH") {
          return jsonResponse({
            scope: {
              id: scopeId,
              tenant_id: defaultTenantId,
              runtime_node_id: runtimeNodeId,
              team_id: null,
              scope_type: "tenant",
              scope_value: defaultTenantId,
              status: (parseRequestBody(init) as { status?: string } | undefined)?.status ?? "disabled",
              created_at: "2026-06-01T00:00:00Z",
              updated_at: "2026-06-01T00:01:00Z",
            },
          });
        }
        return createNotFoundResponse(url.pathname);
    }
  }) as unknown as typeof fetch;

  return { fetcher, requests };
}

async function renderPermissionsCenter(fetcher = createAuthzFetcher().fetcher) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <PermissionsCenter apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("PermissionsCenter", () => {
  it("renders five tabs", async () => {
    const screen = await renderPermissionsCenter();

    await expect.element(screen.getByRole("heading", { name: "权限中心" })).toBeVisible();

    const tabNames = ["授权概览", "授权审计", "Runtime 范围", "成员角色", "权限诊断"];
    await vi.waitFor(() => {
      expect(screen.getByRole("tab").elements()).toHaveLength(5);
    });

    for (const tabName of tabNames) {
      await expect.element(screen.getByRole("tab", { name: tabName })).toBeVisible();
    }

    await expect.element(screen.getByText("db")).toBeVisible();
  });

  it("submits permission diagnostics", async () => {
    const { fetcher, requests } = createAuthzFetcher();
    const screen = await renderPermissionsCenter(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "权限诊断" }));
    await userEvent.fill(screen.getByLabelText("Actor ID"), actorId);
    await userEvent.fill(screen.getByLabelText("租户 ID"), defaultTenantId);
    await userEvent.click(screen.getByRole("button", { name: "开始诊断" }));

    await expect.element(screen.getByText("允许")).toBeVisible();

    const checkRequest = requests.find((request) => request.pathname === "/api/authz/check");
    expect(checkRequest).toMatchObject({
      body: {
        actor: {
          id: actorId,
          type: "user",
        },
        action: "console.access",
        resource: {
          id: "web",
          type: "console",
        },
        tenant_id: defaultTenantId,
      },
      method: "POST",
    });
  });

  it("derives tenant resources when diagnostics action targets tenant authorization", async () => {
    const { fetcher, requests } = createAuthzFetcher();
    const screen = await renderPermissionsCenter(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "权限诊断" }));
    await userEvent.fill(screen.getByLabelText("Actor ID"), actorId);
    await userEvent.fill(screen.getByLabelText("租户 ID"), defaultTenantId);
    await userEvent.click(screen.getByRole("combobox", { name: "动作" }));
    await userEvent.click(screen.getByRole("option", { name: "runtime_scope.manage" }));
    await userEvent.click(screen.getByRole("button", { name: "开始诊断" }));

    await expect.element(screen.getByText("允许")).toBeVisible();

    const checkRequest = requests.find(
      (request) => request.pathname === "/api/authz/check" && request.method === "POST",
    );
    expect(checkRequest?.body).toMatchObject({
      action: "runtime_scope.manage",
      resource: {
        id: defaultTenantId,
        type: "tenant",
      },
      tenant_id: defaultTenantId,
    });
  });

  it("creates tenant runtime scopes with backend-derived scope value", async () => {
    const { fetcher, requests } = createAuthzFetcher();
    const screen = await renderPermissionsCenter(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "Runtime 范围" }));
    await userEvent.fill(screen.getByLabelText("Runtime Node ID"), runtimeNodeId);
    await userEvent.fill(screen.getByLabelText("租户 ID"), defaultTenantId);
    await expect.element(screen.getByLabelText("范围值")).toHaveValue(defaultTenantId);
    await userEvent.click(screen.getByRole("button", { name: "新增" }));

    await vi.waitFor(() => {
      const createRequest = requests.find((request) => request.pathname === "/api/authz/runtime-scopes" && request.method === "POST");
      expect(createRequest?.body).toEqual({
        runtime_node_id: runtimeNodeId,
        tenant_id: defaultTenantId,
        scope_type: "tenant",
        scope_value: defaultTenantId,
      });
    });
  });

  it("creates team runtime scopes with team-derived scope value", async () => {
    const { fetcher, requests } = createAuthzFetcher();
    const screen = await renderPermissionsCenter(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "Runtime 范围" }));
    await userEvent.fill(screen.getByLabelText("Runtime Node ID"), runtimeNodeId);
    await userEvent.fill(screen.getByLabelText("租户 ID"), defaultTenantId);
    await userEvent.click(screen.getByRole("combobox", { name: "范围类型" }));
    await userEvent.click(screen.getByRole("option", { name: "团队" }));
    await userEvent.fill(screen.getByLabelText("团队 ID"), teamId);
    await expect.element(screen.getByLabelText("范围值")).toHaveValue(teamId);
    await userEvent.click(screen.getByRole("button", { name: "新增" }));

    await vi.waitFor(() => {
      const createRequest = requests.find((request) => request.pathname === "/api/authz/runtime-scopes" && request.method === "POST");
      expect(createRequest?.body).toEqual({
        runtime_node_id: runtimeNodeId,
        tenant_id: defaultTenantId,
        team_id: teamId,
        scope_type: "team",
        scope_value: teamId,
      });
    });
  });

  it("validates team runtime scopes before calling create", async () => {
    const { fetcher, requests } = createAuthzFetcher();
    const screen = await renderPermissionsCenter(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "Runtime 范围" }));
    await userEvent.fill(screen.getByLabelText("Runtime Node ID"), runtimeNodeId);
    await userEvent.fill(screen.getByLabelText("租户 ID"), defaultTenantId);
    await userEvent.click(screen.getByRole("combobox", { name: "范围类型" }));
    await userEvent.click(screen.getByRole("option", { name: "团队" }));
    await userEvent.click(screen.getByRole("button", { name: "新增" }));

    await expect.element(screen.getByText("团队范围需要填写团队 ID。")).toBeVisible();
    expect(
      requests.find((request) => request.pathname === "/api/authz/runtime-scopes" && request.method === "POST"),
    ).toBeUndefined();
  });

  it("requires confirmation before disabling a runtime scope", async () => {
    const { fetcher, requests } = createAuthzFetcher({
      runtimeScopeNodes: [
        {
          runtime_node_id: runtimeNodeId,
          node_id: "runtime-node-1",
          name: "Runtime Node 1",
          supported_providers: ["codex"],
          max_slots: 4,
          current_load: 1,
          status: "online",
          scopes: [
            {
              id: scopeId,
              tenant_id: defaultTenantId,
              runtime_node_id: runtimeNodeId,
              team_id: null,
              scope_type: "tenant",
              scope_value: defaultTenantId,
              status: "active",
              disabled_at: null,
              created_at: "2026-06-01T00:00:00Z",
              updated_at: "2026-06-01T00:00:00Z",
            },
          ],
        },
      ],
    });
    const screen = await renderPermissionsCenter(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "Runtime 范围" }));
    await userEvent.click(screen.getByRole("button", { name: "禁用" }));

    expect(requests.filter((request) => request.method === "PATCH")).toHaveLength(0);
    await expect.element(screen.getByRole("alertdialog", { name: "确认禁用 Runtime 范围" })).toBeVisible();
    await expect.element(screen.getByText(/后续任务领取行为/)).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "确认禁用" }));

    await vi.waitFor(() => {
      const patchRequest = requests.find((request) => request.method === "PATCH");
      expect(patchRequest).toMatchObject({
        body: {
          status: "disabled",
        },
        pathname: `/api/authz/runtime-scopes/${scopeId}`,
      });
    });
  });
});
