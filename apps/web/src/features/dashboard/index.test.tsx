import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { TaskLaunchView } from "@/features/task-launches";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>
}));

vi.mock("@/features/auth/use-auth", () => ({
  useAuth: () => ({
    user: { username: "admin", display_name: "平台管理员" }
})
}));

vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local"
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to
  }: {
    children: ReactNode;
    to: string;
  }) => <a href={to}>{children}</a>,
  useNavigate: () => vi.fn(),
  useSearch: () => ({}),
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
    if (url.pathname === "/api/v1/projects") {
      return jsonResponse([
        {
          coordination_policy: {},
          coordination_status: "registered",
          coordination_workflow_id: "project-coordinator:project-1",
          directory_name: "project-1-dir",
          goal: "首页任务中枢冒烟",
          human_owner_user_id: "owner-1",
          id: "project-1",
          name: "首页验收项目",
          status: "running",
          tenant_id: "tenant-1",
          workspace_ready_status: "ready",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/budget-summary") {
      return jsonResponse({ exhausted: false, consumed_tokens: 0 });
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
        username: "admin"
},
      {
        client_ip: "10.0.0.8",
        created_at: "2026-06-22T23:00:00Z",
        event_type: "login_failed",
        failure_reason: "bad_password",
        id: "log-2",
        result: "failed",
        username: "auditor"
},
    ]
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
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } }
});
}

async function renderHomepageTaskHub() {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <TaskLaunchView apiBaseUrl="http://control-plane.local" title="任务中枢" />
    </QueryClientProvider>,
  );
}

describe("Homepage task hub", () => {
  it("renders the task launch experience as the authenticated homepage", async () => {
    const screen = await renderHomepageTaskHub();

    await expect.element(screen.getByRole("heading", { name: "任务中枢" })).toBeVisible();
    await expect.element(screen.getByText("提交需求并跟踪流程实例的运行与阻塞")).toBeVisible();
  });
});
