import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import { EmployeesView } from "@/features/employees";

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

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, params, to }: { children: ReactNode; params?: Record<string, string>; to: string }) => (
    <a href={params?.employeeId ? to.replace("$employeeId", encodeURIComponent(params.employeeId)) : to}>{children}</a>
  ),
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function createEmployeesFetcher() {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    if (url.pathname === "/api/v1/digital-employees/overview" && (init?.method ?? "GET") === "GET") {
      const twoMinutesAgo = new Date(Date.now() - 2 * 60 * 1000).toISOString();
      return new Response(
        JSON.stringify({
          queue_summary: {
            pending_runtime_binding_count: 2,
            stale_config_count: 4,
            failed_recent_run_count: 1,
          },
          summary: {
            total_count: 18,
            runnable_count: 14,
            running_count: 5,
            waiting_runtime_count: 2,
            error_count: 1,
            high_risk_count: 3,
            ready_count: 14,
            pending_runtime_binding_count: 2,
            pending_config_approval_count: 4,
            failed_recent_run_count: 1,
          },
          items: [
            {
              workbench_status: "ready",
              recent_events: [
                {
                  label: "命令已下发",
                  status: "dispatching",
                  occurred_at: twoMinutesAgo,
                },
              ],
              identity_summary: {
                id: "11111111-1111-4111-8111-111111111111",
                tenant_id: "tenant-1",
                team_id: "team-1",
                team_name: "产品组",
                owner_user_id: "owner-1",
                owner_display_name: "王产品",
                employee_type: "requirements_analyst",
                employee_type_label: "需求分析",
                name: "需求分析员工",
                role: "需求分析师",
                description: "负责需求拆解和交付风险识别",
                status: "active",
                risk_level: "medium",
                avatar_asset: {
                  id: "engineer-f-01",
                  label: "工程师头像 F01",
                  gender: "female",
                  age_range: "24-30",
                  style: "photorealistic_2d",
                  image_url: "/images/digital-employee-avatars/engineer-f-01.webp",
                  thumbnail_url: "/images/digital-employee-avatars/engineer-f-01-256.webp",
                  source: "ai_generated_internal_pack",
                  license: "internal_product_asset",
                  status: "active",
                },
              },
              execution_summary: {
                execution_instance_id: "22222222-2222-4222-8222-222222222222",
                status: "active",
                runtime_node_id: "33333333-3333-4333-8333-333333333333",
                node_id: "local-dev-node",
                runtime_name: "华东执行节点",
                runtime_status: "online",
                provider_type: "claude_code",
                provider_status: "ready",
                health_status: "healthy",
                agent_home_dir_available: true,
              },
              latest_run_summary: {
                run_id: "44444444-4444-4444-8444-444444444444",
                task_id: "task-1",
                status: "completed",
                title: "审查需求",
                started_at: "2026-06-07T08:00:00Z",
                finished_at: twoMinutesAgo,
                updated_at: twoMinutesAgo,
                duration_sec: 480,
                token_usage: 3200,
                error_message: "",
              },
              governance_summary: {
                effective_config_id: "55555555-5555-4555-8555-555555555555",
                status: "approved",
                team_revision_number: 3,
                employee_revision_number: 2,
                skills_count: 8,
                mcp_servers_count: 3,
                constitution_ref: "constitution://requirements/v2",
              },
              budget_summary: {
                usage_tokens_30d: 16000,
                run_count_30d: 12,
                cost_amount_30d: 28.5,
                currency: "CNY",
                source: "runtime_usage",
                daily_token_limit: null,
                usage_tokens_today: 0,
                usage_percent_today: null,
                limit_exceeded: false,
              },
            },
          ],
          filters: {
            teams: [{ value: "team-1", label: "产品组" }],
            employee_types: [{ value: "requirements_analyst", label: "需求分析" }],
            statuses: [
              { value: "active", label: "运行中" },
              { value: "ready", label: "就绪" },
            ],
            providers: [{ value: "codex", label: "codex" }],
            runtime_nodes: [{ value: "33333333-3333-4333-8333-333333333333", label: "runtime-cn-01" }],
            risk_levels: [{ value: "medium", label: "中风险" }],
            execution_statuses: [{ value: "active", label: "执行中" }],
            run_statuses: [{ value: "running", label: "运行中" }],
          },
          pagination: {
            limit: 50,
            offset: 0,
            total_count: 18,
          },
        }),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        },
      );
    }

    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), {
      headers: { "content-type": "application/json" },
      status: 404,
    });
  }) as unknown as typeof fetch;

  return fetcher;
}

function fetchCalls(fetcher: typeof fetch) {
  return (
    fetcher as unknown as {
      mock: { calls: [RequestInfo | URL, RequestInit | undefined][] };
    }
  ).mock.calls;
}

async function renderEmployeesView(fetcher = createEmployeesFetcher()) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <EmployeesView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("EmployeesView", () => {
  it("renders the digital employee workbench overview", async () => {
    const fetcher = createEmployeesFetcher();
    const screen = await renderEmployeesView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "数字员工" })).toBeVisible();
    await expect.element(screen.getByText("就绪").first()).toBeVisible();
    await expect.element(screen.getByText("待绑定").first()).toBeVisible();
    await expect.element(screen.getByText("异常").first()).toBeVisible();
    await expect.element(screen.getByText("配置待审批")).toBeVisible();
    await expect.element(screen.getByText("运行失败").first()).toBeVisible();
    await expect.element(screen.getByText("需求分析员工").first()).toBeVisible();
    await expect.element(screen.getByAltText("需求分析员工 的头像").first()).toHaveAttribute(
      "src",
      "/images/digital-employee-avatars/engineer-f-01-256.webp",
    );
    await expect.element(screen.getByText("产品组").first()).toBeVisible();
    await expect.element(screen.getByText("local-dev-node · Claude Code").first()).toBeVisible();
    await expect.element(screen.getByText("成功 · 2 分钟前")).toBeVisible();
    await expect.element(screen.getByText("无预算上限")).toBeVisible();
    await expect.element(screen.getByText("待处理队列")).toBeVisible();
    await expect.element(screen.getByText("最近运行失败")).toBeVisible();
    await expect.element(screen.getByText("命令已下发")).toBeVisible();
    await expect.element(screen.getByText("配置 v2 已审批 · skills 8 · MCP 3").first()).toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "详情" }))
      .toHaveAttribute("href", "/employees/11111111-1111-4111-8111-111111111111");
    await expect.element(screen.getByText("执行实例 ready")).not.toBeInTheDocument();
    await expect.element(screen.getByText("Server")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: /启动|停止/ })).not.toBeInTheDocument();
  });

  it("links the primary create action to the creation wizard", async () => {
    const screen = await renderEmployeesView();

    await expect
      .element(screen.getByRole("link", { name: "创建数字员工" }))
      .toHaveAttribute("href", "/employees/new");
  });

  it("requests the overview endpoint with selected status filter", async () => {
    const fetcher = createEmployeesFetcher();
    const screen = await renderEmployeesView(fetcher);

    await userEvent.click(screen.getByRole("combobox", { name: "状态" }));
    await userEvent.click(screen.getByRole("option", { name: "运行中" }));
    await expect.element(screen.getByText("需求分析员工").first()).toBeVisible();

    expect(
      fetchCalls(fetcher).some(([input]) => {
        const url = new URL(String(input));
        return url.pathname === "/api/v1/digital-employees/overview" && url.searchParams.get("status") === "active";
      }),
    ).toBe(true);
  });
});
