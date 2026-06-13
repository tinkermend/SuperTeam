import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeDetailView } from "./detail";

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
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
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

const employee = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "需求分析员工",
  role: "requirements_analyst",
  description: "负责需求拆解和交付风险识别",
  status: "active",
  risk_level: "medium",
  metadata: {
    avatar: {
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
};

const executionInstance = {
  id: "22222222-2222-4222-8222-222222222222",
  digital_employee_id: employee.id,
  runtime_node_id: "33333333-3333-4333-8333-333333333333",
  provider_type: "codex",
  status: "ready",
};

function runFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: "44444444-4444-4444-8444-444444444444",
    tenant_id: "tenant-1",
    task_id: "task-1",
    digital_employee_id: employee.id,
    execution_instance_id: executionInstance.id,
    runtime_node_id: executionInstance.runtime_node_id,
    node_id: "node-a",
    command_id: "cmd-1",
    provider_type: "codex",
    status: "running",
    result: {},
    diagnostic: {},
    work_products: [],
    session_state: {},
    timed_out: false,
    created_at: "2026-06-05T01:00:00Z",
    updated_at: "2026-06-05T01:01:00Z",
    ...overrides,
  };
}

function createDetailFetcher({
  events = [
    {
      event_type: "provider.stdout",
      sequence_number: 1,
      payload: { text: "正在分析需求" },
      provider_session_external_id: "session-ext-1",
      session_state_patch: { phase: "analysis" },
      metadata: { source: "runtime" },
    },
  ],
  run = runFixture(),
  runs,
  eventsByRunId,
  executionInstanceStatus = 200,
  runsStatus = 200,
  runtimeOverview = {
    summary: {
      online_nodes: 1,
      total_nodes: 1,
      pending_enrollments: 0,
      active_provider_sessions: 0,
      blocked_events: 0,
    },
    pending_enrollments: [],
    nodes: [
      {
        runtime_node_id: executionInstance.runtime_node_id,
        node_id: "node-a",
        name: "node-a",
        supported_providers: ["codex"],
        max_slots: 3,
        current_load: 0,
        status: "online",
        command_channel_connected: true,
      },
    ],
    provider_capabilities: [],
    recent_events: [],
  },
}: {
  events?: Array<Record<string, unknown>>;
  run?: Record<string, unknown>;
  runs?: Array<Record<string, unknown>>;
  eventsByRunId?: Record<string, Array<Record<string, unknown>>>;
  executionInstanceStatus?: number;
  runsStatus?: number;
  runtimeOverview?: Record<string, unknown>;
} = {}) {
  let currentRun = run;
  let currentRuns = runs ?? [currentRun];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === `/api/v1/digital-employees/${employee.id}` && method === "GET") {
      return jsonResponse(employee);
    }

    if (url.pathname === `/api/v1/digital-employees/${employee.id}/execution-instance` && method === "GET") {
      if (executionInstanceStatus !== 200) {
        return jsonResponse({ error: "execution instance failed" }, executionInstanceStatus);
      }
      return jsonResponse(executionInstance);
    }

    if (url.pathname === `/api/v1/digital-employees/${employee.id}/runs` && method === "GET") {
      expect(url.searchParams.get("limit")).toBe("10");
      if (runsStatus !== 200) {
        return jsonResponse({ error: "runs failed" }, runsStatus);
      }
      return jsonResponse(currentRuns);
    }

    if (url.pathname === "/api/v1/runtime/overview" && method === "GET") {
      return jsonResponse(runtimeOverview);
    }

    if (url.pathname.startsWith(`/api/v1/digital-employees/${employee.id}/runs/`) && url.pathname.endsWith("/events") && method === "GET") {
      const runId = decodeURIComponent(url.pathname.split("/runs/")[1]?.replace("/events", "") ?? "");
      return jsonResponse(eventsByRunId?.[runId] ?? events);
    }

    if (url.pathname === `/api/v1/digital-employees/${employee.id}/runs/${currentRun.id}/stop` && method === "POST") {
      expect(JSON.parse(String(init?.body))).toEqual({ reason: "用户从 Web 停止" });
      currentRun = { ...currentRun, status: "cancelling" };
      currentRuns = currentRuns.map((runItem) => (runItem.id === currentRun.id ? currentRun : runItem));
      return jsonResponse(currentRun);
    }

    if (url.pathname === `/api/v1/digital-employees/${employee.id}/runs` && method === "POST") {
      expect(JSON.parse(String(init?.body))).toEqual({
        objective: "梳理上线风险",
        prompt: "请检查最近失败任务",
      });
      currentRun = runFixture({
        id: "55555555-5555-4555-8555-555555555555",
        objective: "梳理上线风险",
        status: "dispatching",
      });
      currentRuns = [currentRun, ...currentRuns];
      return jsonResponse(currentRun, 201);
    }

    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;

  return fetcher;
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function fetchCallCount(fetcher: typeof fetch, path: string, method: string) {
  return (
    fetcher as unknown as {
      mock: { calls: Array<[RequestInfo | URL, RequestInit | undefined]> };
    }
  ).mock.calls.filter(([input, init]) => {
    const url = new URL(String(input));

    return url.pathname === path && (init?.method ?? "GET") === method;
  }).length;
}

async function renderEmployeeDetail(fetcher = createDetailFetcher()) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <EmployeeDetailView
        apiBaseUrl="http://control-plane.local"
        employeeId="11111111-1111-4111-8111-111111111111"
        fetcher={fetcher}
      />
    </QueryClientProvider>,
  );
}

describe("EmployeeDetailView", () => {
  it("renders active run events and stops the run with refresh", async () => {
    const fetcher = createDetailFetcher();
    const screen = await renderEmployeeDetail(fetcher);

    await expect.element(screen.getByRole("heading", { name: "需求分析员工" })).toBeVisible();
    await expect.element(screen.getByAltText("需求分析员工 的头像")).toHaveAttribute(
      "src",
      "/images/digital-employee-avatars/engineer-f-01-256.webp",
    );
    await expect.element(screen.getByText("执行中")).toBeVisible();
    await expect.element(screen.getByText("provider.stdout")).toBeVisible();
    await expect.element(screen.getByText(/正在分析需求/)).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "停止" }));

    await expect.element(screen.getByText("取消中")).toBeVisible();
    expect(fetchCallCount(fetcher, `/api/v1/digital-employees/${employee.id}/runs`, "GET")).toBeGreaterThanOrEqual(2);
  });

  it("starts a task when there is no active run", async () => {
    const fetcher = createDetailFetcher({
      events: [],
      run: runFixture({
        status: "completed",
        result: { summary: "上一次已完成" },
      }),
    });
    const screen = await renderEmployeeDetail(fetcher);

    await userEvent.fill(screen.getByLabelText("任务目标"), "梳理上线风险");
    await userEvent.fill(screen.getByLabelText("任务提示"), "请检查最近失败任务");
    await userEvent.click(screen.getByRole("button", { name: "开始任务" }));

    await expect.element(screen.getByRole("button", { name: /调度中/ })).toBeVisible();
    expect(fetchCallCount(fetcher, `/api/v1/digital-employees/${employee.id}/runs`, "POST")).toBe(1);
  });

  it("keeps start disabled when runtime command channel is disconnected", async () => {
    const fetcher = createDetailFetcher({
      events: [],
      run: runFixture({ status: "completed" }),
      runtimeOverview: {
        summary: {
          online_nodes: 1,
          total_nodes: 1,
          pending_enrollments: 0,
          active_provider_sessions: 0,
          blocked_events: 0,
        },
        pending_enrollments: [],
        nodes: [
          {
            runtime_node_id: executionInstance.runtime_node_id,
            node_id: "node-a",
            name: "node-a",
            supported_providers: ["codex"],
            max_slots: 3,
            current_load: 0,
            status: "online",
            command_channel_connected: false,
          },
        ],
        provider_capabilities: [],
        recent_events: [],
      },
    });
    const screen = await renderEmployeeDetail(fetcher);

    await expect.element(screen.getByText("Runtime 命令通道未连接，暂不能开始任务")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "开始任务" })).toBeDisabled();
  });

  it("renders completed run result and failed run failure reason", async () => {
    const completedScreen = await renderEmployeeDetail(
      createDetailFetcher({
        run: runFixture({
          status: "completed",
          result: { summary: "已生成验收报告" },
        }),
      }),
    );

    await expect.element(completedScreen.getByText("已完成")).toBeVisible();
    await expect.element(completedScreen.getByText(/已生成验收报告/)).toBeVisible();

    const failedScreen = await renderEmployeeDetail(
      createDetailFetcher({
        run: runFixture({
          error_message: "Runtime 节点断开",
          status: "failed",
        }),
      }),
    );

    await expect.element(failedScreen.getByText("失败原因")).toBeVisible();
    await expect.element(failedScreen.getByText("Runtime 节点断开")).toBeVisible();
  });

  it("renders cancellation and timeout as failure reasons", async () => {
    const cancelledScreen = await renderEmployeeDetail(
      createDetailFetcher({
        run: runFixture({
          error_message: "用户停止执行",
          status: "cancelled",
        }),
      }),
    );

    await expect.element(cancelledScreen.getByText("已取消")).toBeVisible();
    await expect.element(cancelledScreen.getByText("用户停止执行")).toBeVisible();

    const timedOutScreen = await renderEmployeeDetail(
      createDetailFetcher({
        run: runFixture({
          diagnostic: { reason: "lease expired" },
          status: "timed_out",
        }),
      }),
    );

    await expect.element(timedOutScreen.getByText("已超时")).toBeVisible();
    await expect.element(timedOutScreen.getByText(/lease expired/)).toBeVisible();
  });

  it("keeps start disabled when run list cannot be trusted", async () => {
    const fetcher = createDetailFetcher({
      run: runFixture({ status: "completed" }),
      runsStatus: 500,
    });
    const screen = await renderEmployeeDetail(fetcher);

    await expect.element(screen.getByText("运行列表加载失败，暂不能开始新任务")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "开始任务" })).toBeDisabled();
  });

  it("keeps start disabled when execution instance is missing", async () => {
    const fetcher = createDetailFetcher({
      executionInstanceStatus: 404,
      run: runFixture({ status: "completed" }),
    });
    const screen = await renderEmployeeDetail(fetcher);

    await expect.element(screen.getByText("未绑定 Runtime，暂不能开始任务")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "开始任务" })).toBeDisabled();
  });

  it("switches event stream when selecting a historical run", async () => {
    const latestRun = runFixture({
      id: "latest-run",
      status: "completed",
      result: { summary: "最新执行完成" },
    });
    const previousRun = runFixture({
      id: "previous-run",
      status: "failed",
      error_message: "旧运行失败",
    });
    const fetcher = createDetailFetcher({
      runs: [latestRun, previousRun],
      eventsByRunId: {
        "latest-run": [{ event_type: "provider.stdout", sequence_number: 1, payload: { text: "最新事件" } }],
        "previous-run": [{ event_type: "provider.stderr", sequence_number: 2, payload: { text: "历史失败事件" } }],
      },
    });
    const screen = await renderEmployeeDetail(fetcher);

    await expect.element(screen.getByText(/最新事件/)).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: /失败.*previous/ }));

    await expect.element(screen.getByText("旧运行失败")).toBeVisible();
    await expect.element(screen.getByText(/历史失败事件/)).toBeVisible();
  });
});
