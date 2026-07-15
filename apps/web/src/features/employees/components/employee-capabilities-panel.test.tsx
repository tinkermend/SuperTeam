import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, test, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeCapabilitiesPanel } from "./employee-capabilities-panel";

const employeeId = "emp-1";

const baseSkillEntry = {
  skill: {
    id: "skill-1",
    tenant_id: "tenant-1",
    slug: "deploy-helper",
    name: "Deploy Helper",
    description: "一键部署脚本",
    version: "1.0.0",
    source: "internal",
    risk_level: "low",
    icon_key: "boxes",
    color_token: "artifact",
    tags: [],
    archive_object_ref: "",
    archive_filename: "",
    archive_size_bytes: 0,
    archive_checksum_sha256: "",
    archive_file_count: 0,
    created_by: "user-1",
    created_by_name: "Tester",
    team_bindings: [],
    agent_bindings: [],
  },
  source_scope: "employee",
  inherited: false,
  read_only: false,
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

// createPanelFetcher stubs every query the panel fires. `overrides` keys are matched by
// pathname suffix (e.g. "/skill-mcp-dependency-status"); anything not in defaults or
// overrides falls back to an empty array so unrelated queries never crash the render.
//
// Stubbed panel query paths (defaults, all under /api/v1):
//   - /skills                                          (marketplace)
//   - /digital-employees/{id}/skills                   (installed skills; includes skill-1)
//   - /mcp-servers                                      (mcp server definitions)
//   - /digital-employees/{id}/mcp-bindings-v2           (personal mcp bindings)
//   - /digital-employees/{id}/effective-mcp-config      (registry-resolved effective config)
//   - /digital-employees/{id}/environment-variables     (employee env vars)
//   - /digital-employees/{id}/skill-mcp-dependency-status (Task 6 dependency status; override target)
function createPanelFetcher(overrides: Record<string, unknown> = {}) {
  const defaults: Record<string, unknown> = {
    [`/api/v1/skills`]: [],
    [`/api/v1/digital-employees/${employeeId}/skills`]: [baseSkillEntry],
    [`/api/v1/mcp-servers`]: [],
    [`/api/v1/digital-employees/${employeeId}/mcp-bindings-v2`]: [],
    [`/api/v1/digital-employees/${employeeId}/effective-mcp-config`]: [],
    [`/api/v1/digital-employees/${employeeId}/environment-variables`]: [],
    [`/api/v1/digital-employees/${employeeId}/skill-mcp-dependency-status`]: [],
  };
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    for (const [suffix, body] of Object.entries(overrides)) {
      if (url.pathname.endsWith(suffix)) return jsonResponse(body);
    }
    if (url.pathname in defaults) return jsonResponse(defaults[url.pathname]);
    return jsonResponse([]);
  }) as unknown as typeof fetch;
}

function withClient(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={client}>{ui}</QueryClientProvider>;
}

describe("EmployeeCapabilitiesPanel skill-level MCP dependency warnings", () => {
  test("shows missing mcp dependency warning under skill row", async () => {
    const fetcher = createPanelFetcher({
      "/skill-mcp-dependency-status": [
        {
          skill_id: "skill-1",
          skill_slug: "deploy-helper",
          dependencies: [
            {
              mcp_server_id: "srv-1",
              server_key: "github-mcp",
              server_name: "GitHub MCP",
              status: "missing_binding",
              missing_env_vars: [],
            },
          ],
        },
      ],
    });
    const screen = await render(
      withClient(<EmployeeCapabilitiesPanel apiOptions={{ baseUrl: "http://cp", fetcher }} employeeId={employeeId} />),
    );
    await expect.element(screen.getByText(/依赖 MCP github-mcp 未绑定/)).toBeVisible();
    await expect.element(screen.getByText("缺 MCP github-mcp")).toBeVisible();
  });

  test("blocked_missing_env warning lists the missing env var names", async () => {
    const fetcher = createPanelFetcher({
      "/skill-mcp-dependency-status": [
        {
          skill_id: "skill-1",
          skill_slug: "deploy-helper",
          dependencies: [
            {
              mcp_server_id: "srv-1",
              server_key: "github-mcp",
              server_name: "GitHub MCP",
              status: "blocked_missing_env",
              missing_env_vars: ["GITHUB_TOKEN", "GITHUB_ORG"],
            },
          ],
        },
      ],
    });
    const screen = await render(
      withClient(<EmployeeCapabilitiesPanel apiOptions={{ baseUrl: "http://cp", fetcher }} employeeId={employeeId} />),
    );
    await expect
      .element(screen.getByText(/依赖 MCP github-mcp 缺环境变量 GITHUB_TOKEN, GITHUB_ORG/))
      .toBeVisible();
  });

  test("satisfied dependency produces no warning and no extra pill", async () => {
    const fetcher = createPanelFetcher({
      "/skill-mcp-dependency-status": [
        {
          skill_id: "skill-1",
          skill_slug: "deploy-helper",
          dependencies: [
            {
              mcp_server_id: "srv-1",
              server_key: "github-mcp",
              server_name: "GitHub MCP",
              status: "satisfied",
              missing_env_vars: [],
            },
          ],
        },
      ],
    });
    const screen = await render(
      withClient(<EmployeeCapabilitiesPanel apiOptions={{ baseUrl: "http://cp", fetcher }} employeeId={employeeId} />),
    );
    await expect.element(screen.getByText("Deploy Helper", { exact: true })).toBeVisible();
    expect(screen.getByText(/依赖 MCP/).query()).toBeNull();
    expect(screen.getByText("缺 MCP github-mcp").query()).toBeNull();
  });

  test("no dependency data renders the skill row with no warning region at all", async () => {
    const fetcher = createPanelFetcher();
    const screen = await render(
      withClient(<EmployeeCapabilitiesPanel apiOptions={{ baseUrl: "http://cp", fetcher }} employeeId={employeeId} />),
    );
    await expect.element(screen.getByText("Deploy Helper", { exact: true })).toBeVisible();
    expect(screen.getByText(/任务派发将被阻断/).query()).toBeNull();
  });
});
