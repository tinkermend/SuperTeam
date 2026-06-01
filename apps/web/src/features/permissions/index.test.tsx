import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { PermissionsCenter } from "@/features/permissions";

const defaultTenantId = "00000000-0000-4000-8000-000000000001";
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

function createAuthzFetcher() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));

    switch (url.pathname) {
      case "/api/authz/overview":
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
        return jsonResponse({ items: [] });
      case "/api/authz/runtime-scopes":
        return jsonResponse({ nodes: [] });
      case "/api/authz/members":
        return jsonResponse({ items: [] });
      case "/api/authz/check":
        return jsonResponse({
          allowed: true,
          engine: "db",
          matched_rule: "tenant.owner",
          reason: "allowed",
          snapshot: {},
        });
      default:
        return jsonResponse({ error: `unhandled ${url.pathname}` });
    }
  }) as typeof fetch;
}

async function renderPermissionsCenter(fetcher = createAuthzFetcher()) {
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
    const screen = await renderPermissionsCenter();

    await userEvent.click(screen.getByRole("tab", { name: "权限诊断" }));
    await userEvent.fill(screen.getByLabelText("Actor ID"), actorId);
    await userEvent.fill(screen.getByLabelText("租户 ID"), defaultTenantId);
    await userEvent.click(screen.getByRole("button", { name: "开始诊断" }));

    await expect.element(screen.getByText("允许")).toBeVisible();
  });
});
