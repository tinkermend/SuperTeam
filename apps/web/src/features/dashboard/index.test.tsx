import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { Dashboard } from "@/features/dashboard";

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

vi.mock("@/features/auth/use-auth", () => ({
  useAuth: () => ({
    user: { username: "admin", display_name: "平台管理员" },
  }),
}));

vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local",
}));

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    if (url.pathname === "/health") {
      return jsonResponse({ status: "ok" });
    }
    if (url.pathname === "/api/auth/login-logs") {
      return jsonResponse({
    items: [
      {
        client_ip: "127.0.0.1",
        created_at: "2026-06-23T00:00:00Z",
        event_type: "login_succeeded",
        id: "log-1",
        result: "succeeded",
        session_id: "session-1",
        user_agent: "Chrome",
        user_id: "user-1",
        username: "admin",
      },
      {
        client_ip: "10.0.0.8",
        created_at: "2026-06-22T23:00:00Z",
        event_type: "login_failed",
        failure_reason: "bad_password",
        id: "log-2",
        result: "failed",
        username: "auditor",
      },
    ],
      });
    }
    return jsonResponse({ error: `unhandled ${url.pathname}` }, 500);
  }));
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
}

async function renderDashboard() {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <Dashboard />
    </QueryClientProvider>,
  );
}

describe("Dashboard", () => {
  it("renders the authenticated homepage with v3 soft-flat surfaces", async () => {
    const screen = await renderDashboard();

    await expect.element(screen.getByRole("heading", { name: "工作台" })).toBeVisible();
    await expect.element(screen.getByText("平台管理员")).toBeVisible();
    await expect.element(screen.getByText("Control Plane", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("最近登录日志")).toBeVisible();
    await expect.element(screen.getByText("事件", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("来源", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("登录失败")).toBeVisible();

    expect(document.body.querySelector('[data-slot="v3-page-header"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-signature-card"]')).not.toBeNull();
    expect(document.body.querySelectorAll('[data-slot="v3-soft-card"]').length).toBeGreaterThanOrEqual(2);
    expect(document.body.querySelector('[data-slot="v3-work-surface"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-table"]')).not.toBeNull();
  });
});
