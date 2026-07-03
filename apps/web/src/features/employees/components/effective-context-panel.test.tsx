import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EffectiveContextPanel } from "./effective-context-panel";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
}));

const employeeId = "11111111-1111-4111-8111-111111111111";

const employee = {
  id: employeeId,
  tenant_id: "tenant-1",
  owner_user_id: "user-1",
  employee_type: "backend_engineer",
  name: "后端实现员",
  role: "backend_engineer",
  status: "active" as const,
  permission_policy: {},
  context_policy: {},
  approval_policy: {},
  risk_level: "medium",
};

const executionInstance = {
  id: "instance-1",
  digital_employee_id: employeeId,
  runtime_node_id: "node-uuid-1",
  provider_type: "claude_code",
  agent_home_dir: ".superteam/workspaces/teams/backend/employees/backend-engineer",
  status: "ready",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

function createFetcher() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/effective-config`) {
      return jsonResponse({
        id: "config-1",
        tenant_id: "tenant-1",
        digital_employee_id: employeeId,
        team_config_revision_id: "team-rev-1",
        employee_config_revision_id: "employee-rev-1",
        effective_config: { constitution: { team: { rules: ["禁止删除生产数据"] }, addendum: {} } },
        validation_result: { blocking_errors: [], warnings: [] },
        status: "approved",
      });
    }
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/skills`) {
      return jsonResponse([
        { skill_id: "s1", inherited: false },
        { skill_id: "s2", inherited: true },
        { skill_id: "s3", inherited: true },
      ]);
    }
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/effective-mcp-config`) {
      return jsonResponse([{ server_id: "m1", source_scope: "team" }]);
    }
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/environment-variables`) {
      return jsonResponse([
        { name: "DATABASE_URL", configured: true, fingerprint: "a1", sensitive: true, status: "active" },
        { name: "REDIS_URL", configured: false, fingerprint: "", sensitive: true, status: "active" },
      ]);
    }
    return jsonResponse({ error: "unhandled" }, 404);
  }) as unknown as typeof fetch;
}

describe("EffectiveContextPanel", () => {
  it("renders skill/mcp counts, constitution, env vars and memory placeholder", async () => {
    const fetcher = createFetcher();
    const screen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <EffectiveContextPanel
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
          employee={employee}
          employeeId={employeeId}
          executionInstance={executionInstance}
          onManageCapabilities={vi.fn()}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("个人技能 1")).toBeVisible();
    await expect.element(screen.getByText("团队继承技能 2")).toBeVisible();
    await expect.element(screen.getByText("生效总数 3")).toBeVisible();
    await expect.element(screen.getByText("生效总数 1")).toBeVisible();
    await expect.element(screen.getByText("待接入")).toBeVisible();
    await expect.element(screen.getByText("已配置 1")).toBeVisible();
    await expect.element(screen.getByText("缺失 1")).toBeVisible();
    await expect.element(screen.getByText("REDIS_URL")).toBeVisible();
  });
});
