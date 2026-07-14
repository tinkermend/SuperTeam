import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { RunDetailDrawer } from "./run-detail-drawer";
import type { DigitalEmployeeRunListItem } from "@/lib/api/employees";

const employeeId = "11111111-1111-4111-8111-111111111111";

const runningRun: DigitalEmployeeRunListItem = {
  id: "run-1",
  tenant_id: "tenant-1",
  task_id: "task-1",
  digital_employee_id: employeeId,
  execution_instance_id: "instance-1",
  runtime_node_id: "node-uuid-1",
  node_id: "node-a",
  command_id: "cmd-1",
  provider_type: "claude_code",
  status: "running",
  result: {},
  diagnostic: {},
  work_products: [],
  session_state: {},
  timed_out: false,
  run_kind: "task",
  task_title: "数据库迁移脚本校验",
  work_product_count: 0,
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

function createFetcher() {
  let current = runningRun;
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/runs/${current.id}/events` && method === "GET") {
      return jsonResponse([{ event_type: "text_delta", sequence_number: 1, payload: { text: "正在执行" } }]);
    }
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/runs/${current.id}/stop` && method === "POST") {
      current = { ...current, status: "cancelling" };
      return jsonResponse(current);
    }
    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;
}

describe("RunDetailDrawer", () => {
  it("renders nothing when no run is selected", async () => {
    const screen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RunDetailDrawer
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher: createFetcher() }}
          employeeId={employeeId}
          onOpenChange={vi.fn()}
          onStopped={vi.fn()}
          open
          run={undefined}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("数据库迁移脚本校验")).not.toBeInTheDocument();
  });

  it("shows events and stops an active run", async () => {
    const fetcher = createFetcher();
    const onStopped = vi.fn();
    const screen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RunDetailDrawer
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
          employeeId={employeeId}
          onOpenChange={vi.fn()}
          onStopped={onStopped}
          open
          run={runningRun}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText(/正在执行/)).toBeVisible();
    await expect.element(screen.getByText("更新时间")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "停止" }));
    await expect.element(screen.getByText("取消中")).toBeVisible();
    expect(onStopped).toHaveBeenCalled();
  });

  it("heals stale stopRun mutation state once parent passes a terminal run prop", async () => {
    // Regression guard: after stop succeeds, stopRun.data.status === "cancelling" with id === run.id.
    // If the parent's list refetch then returns status === "cancelled" and passes that refreshed
    // run prop, the drawer must show "已取消" (the prop), NOT stay stuck on "取消中" (stale mutation).
    const fetcher = createFetcher();
    const screen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RunDetailDrawer
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
          employeeId={employeeId}
          onOpenChange={vi.fn()}
          onStopped={vi.fn()}
          open
          run={runningRun}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText(/正在执行/)).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "停止" }));
    await expect.element(screen.getByText("取消中")).toBeVisible();

    // Parent refetch lands: same run id, now terminal. Drawer must trust the prop over the stale
    // stopRun.data and show "已取消".
    const cancelledRun: DigitalEmployeeRunListItem = { ...runningRun, status: "cancelled" };
    await screen.rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RunDetailDrawer
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
          employeeId={employeeId}
          onOpenChange={vi.fn()}
          onStopped={vi.fn()}
          open
          run={cancelledRun}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("已取消")).toBeVisible();
    // vitest-browser-react's `getBy*` queries throw when no match is found, so a successful
    // "已取消" lookup above is itself proof the stale "取消中" pill label is gone (the pill renders
    // exactly one label). If the bug regressed, this assertion would throw with "unable to find
    // element with text: 已取消".
  });

  it("renders completed result conclusion as prose with raw JSON collapsed", async () => {
    const completedRun: DigitalEmployeeRunListItem = {
      ...runningRun,
      status: "completed",
      result: { summary: "已生成验收报告", detail: { files: 3 } },
    };
    const screen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RunDetailDrawer
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher: createFetcher() }}
          employeeId={employeeId}
          onOpenChange={vi.fn()}
          onStopped={vi.fn()}
          open
          run={completedRun}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("已生成验收报告")).toBeVisible();
    await expect.element(screen.getByText("原始结果 JSON")).toBeVisible();
  });

  it("shows the truncation hint only when more than 50 events exist", async () => {
    const makeFetcher = (eventCount: number) =>
      vi.fn(async (input: RequestInfo | URL) => {
        const url = new URL(String(input));
        if (url.pathname.endsWith("/events")) {
          return jsonResponse(
            Array.from({ length: eventCount }, (_, index) => ({
              event_type: "text_delta",
              sequence_number: index + 1,
              payload: { text: `片段${index + 1} ` },
            })),
          );
        }
        return jsonResponse({ error: "unhandled" }, 404);
      }) as unknown as typeof fetch;

    const completedRun: DigitalEmployeeRunListItem = { ...runningRun, status: "completed", result: {} };

    // 恰好 50 条:没有被截断,不显示提示
    const exactScreen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RunDetailDrawer
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher: makeFetcher(50) }}
          employeeId={employeeId}
          onOpenChange={vi.fn()}
          onStopped={vi.fn()}
          open
          run={completedRun}
        />
      </QueryClientProvider>,
    );
    await expect.element(exactScreen.getByText("50 条", { exact: true })).toBeVisible();
    expect(exactScreen.getByText(/仅显示前/).query()).toBeNull();
    await exactScreen.unmount();

    // 51 条:超过上限,只显示前 50 条并给出提示
    const overScreen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RunDetailDrawer
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher: makeFetcher(51) }}
          employeeId={employeeId}
          onOpenChange={vi.fn()}
          onStopped={vi.fn()}
          open
          run={completedRun}
        />
      </QueryClientProvider>,
    );
    await expect.element(overScreen.getByText("仅显示前 50 条事件。")).toBeVisible();
    await expect.element(overScreen.getByText("50 条", { exact: true })).toBeVisible();
  });

  it("collapses expanded raw JSON when the drawer switches to a different run", async () => {
    const resultByRun: Record<string, Record<string, unknown>> = {
      "run-a": { summary: "A 运行结论", detail: "a" },
      "run-b": { summary: "B 运行结论", detail: "b" },
    };
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input));
      const match = url.pathname.match(/\/runs\/([^/]+)\/events$/);
      if (match) {
        return jsonResponse([
          { event_type: "turn_started", sequence_number: 1, payload: {} },
        ]);
      }
      return jsonResponse({ error: "unhandled" }, 404);
    }) as unknown as typeof fetch;

    const runA: DigitalEmployeeRunListItem = {
      ...runningRun,
      id: "run-a",
      status: "completed",
      result: resultByRun["run-a"],
      task_title: "A 任务",
    };
    const runB: DigitalEmployeeRunListItem = {
      ...runningRun,
      id: "run-b",
      status: "completed",
      result: resultByRun["run-b"],
      task_title: "B 任务",
    };

    const wrap = (run: DigitalEmployeeRunListItem) => (
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RunDetailDrawer
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
          employeeId={employeeId}
          onOpenChange={vi.fn()}
          onStopped={vi.fn()}
          open
          run={run}
        />
      </QueryClientProvider>
    );

    const screen = await render(wrap(runA));
    // 展开结果原始 JSON 与事件原始 JSON
    await screen.getByText("原始结果 JSON").click();
    await expect.element(screen.getByText(/"detail": "a"/)).toBeVisible();
    await screen.getByText("原始 JSON").click();
    await expect.element(screen.getByText(/"event_type": "turn_started"/)).toBeVisible();

    // 切换到另一条运行:展开态必须重置(折叠),且不残留上一条的内容
    await screen.rerender(wrap(runB));
    await expect.element(screen.getByText("B 运行结论", { exact: true })).toBeVisible();
    // 等 B 的事件流加载出折叠入口后再断言:确保不是"还没渲染"造成的假通过
    await expect.element(screen.getByText("原始 JSON")).toBeVisible();
    expect(screen.getByText(/"detail": "b"/).query()).toBeNull();
    expect(screen.getByText(/"detail": "a"/).query()).toBeNull();
    expect(screen.getByText(/"event_type": "turn_started"/).query()).toBeNull();
  });
});
