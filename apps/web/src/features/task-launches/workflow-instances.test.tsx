import { useState, type ReactNode } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskLaunchPage } from "@/features/task-launches";
import {
  WorkflowInstancesView,
  type WorkflowInstancesFilters
} from "@/features/task-launches/components/workflow-instances-view";
import type {
  Project,
  WorkflowInstanceStatus,
  WorkflowInstanceSummary
} from "@/lib/api/projects";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  search: {} as Record<string, unknown>
}));
const mountedRoots: Root[] = [];

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

vi.mock("@/components/ui/select", async () => {
  const React = await import("react");
  type SelectContextValue = {
    onValueChange?: (value: string) => void;
    value?: string;
  };
  const SelectContext = React.createContext<SelectContextValue>({});

  return {
    Select: ({
      children,
      onValueChange,
      value
    }: {
      children: ReactNode;
      onValueChange?: (value: string) => void;
      value?: string;
    }) => (
      <SelectContext value={{ onValueChange, value }}>
        <div data-select-value={value}>{children}</div>
      </SelectContext>
    ),
    SelectContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    SelectGroup: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    SelectItem: ({ children, value }: { children: ReactNode; value: string }) => {
      const { onValueChange, value: selectedValue } = React.useContext(SelectContext);
      return (
        <button
          aria-pressed={selectedValue === value}
          type="button"
          onClick={() => onValueChange?.(value)}
        >
          {children}
        </button>
      );
    },
    SelectLabel: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    SelectSeparator: () => <hr />,
    SelectTrigger: ({
      "aria-label": ariaLabel,
      children
    }: {
      "aria-label"?: string;
      children: ReactNode;
    }) => (
      <button aria-label={ariaLabel} type="button">
        {children}
      </button>
    ),
    SelectValue: ({ placeholder }: { placeholder?: string }) => <span>{placeholder}</span>
  };
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useNavigate: () => mocks.navigate,
  useSearch: () => mocks.search
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } }
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status
  });
}

function makeProject(id: string, name: string): Project {
  return {
    coordination_policy: {},
    coordination_status: "registered",
    coordination_workflow_id: `project-coordinator:${id}`,
    directory_name: `${id}-dir`,
    goal: "完成一次任务发起",
    human_owner_user_id: "owner-1",
    id,
    name,
    status: "running",
    tenant_id: "tenant-1",
    workspace_ready_status: "ready"
  };
}

function makeInstance(
  id: string,
  status: WorkflowInstanceStatus = "running",
): WorkflowInstanceSummary {
  return {
    created_at: "2026-07-25T10:00:00Z",
    demand_id: id,
    progress: {
      blocked_nodes: 0,
      completed_nodes: 1,
      running_nodes: status === "running" ? 1 : 0,
      total_nodes: 3,
      waiting_human_nodes: 0
    },
    project_id: "project-1",
    project_name: "客户接入项目",
    status,
    status_reason: "",
    submitted_by_display_name: "张三",
    submitted_by_user_id: "owner-1",
    title: `需求 ${id}`,
    updated_at: "2026-07-25T11:00:00Z"
  };
}

function createInstancesFetcher() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    if (url.pathname === "/api/v1/workflow-instances") {
      return jsonResponse([
        makeInstance("demand-1", "running"),
        makeInstance("demand-2", "completed"),
      ]);
    }
    if (url.pathname === "/api/v1/projects") {
      return jsonResponse([
        makeProject("project-1", "客户接入项目"),
        makeProject("project-2", "生产巡检项目"),
      ]);
    }
    return jsonResponse({ message: `Unhandled ${url.pathname}` }, 404);
  });
}

/** 收集 fetcher 收到的 workflow-instances 请求 URL（含 query），用于断言服务端过滤参数。 */
function instancesRequestUrls(fetcher: ReturnType<typeof createInstancesFetcher>) {
  return fetcher.mock.calls
    .map((call) => new URL(String(call[0])))
    .filter((url) => url.pathname === "/api/v1/workflow-instances");
}

function InstancesHarness({
  fetcher,
  initialFilters = {}
}: {
  fetcher: typeof fetch;
  initialFilters?: WorkflowInstancesFilters;
}) {
  const [filters, setFilters] = useState<WorkflowInstancesFilters>(initialFilters);
  return (
    <WorkflowInstancesView
      apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
      filters={filters}
      onFiltersChange={setFilters}
      searchDebounceMs={0}
    />
  );
}

async function renderWithQueryClient(children: ReactNode) {
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  const queryClient = createQueryClient();
  mountedRoots.push(root);

  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>,
    );
  });

  return { container, queryClient, root };
}

function queryByText(text: string) {
  return (
    Array.from(document.body.querySelectorAll<HTMLElement>("*")).find(
      (item) =>
        item.textContent === text &&
        Array.from(item.children).every((child) => child.textContent !== text),
    ) ?? null
  );
}

function getByText(text: string) {
  const element = queryByText(text);
  if (!element) {
    throw new Error(`Unable to find text: ${text}`);
  }
  return element;
}

function getButton(name: string) {
  const button = Array.from(
    document.body.querySelectorAll<HTMLButtonElement>("button"),
  ).find(
    (item) => item.textContent === name || item.getAttribute("aria-label") === name,
  );
  if (!button) {
    throw new Error(`Unable to find button: ${name}`);
  }
  return button;
}

async function clickButton(name: string) {
  await waitFor(() => expect(getButton(name).disabled).toBe(false));
  const button = getButton(name);
  await act(async () => {
    button.click();
  });
}

function setInputValue(input: HTMLInputElement, value: string) {
  const valueSetter = Object.getOwnPropertyDescriptor(input, "value")?.set;
  const prototypeValueSetter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  if (prototypeValueSetter && valueSetter !== prototypeValueSetter) {
    prototypeValueSetter.call(input, value);
  } else {
    valueSetter?.call(input, value);
  }
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

async function waitFor(assertion: () => void) {
  await act(async () => {
    await vi.waitFor(assertion);
  });
}

afterEach(() => {
  for (const root of mountedRoots.splice(0)) {
    act(() => {
      root.unmount();
    });
  }
  document.body.innerHTML = "";
  mocks.navigate.mockClear();
  mocks.search = {};
});

describe("TaskLaunchPage tabs", () => {
  it("renders the compose tab by default and navigates to the instances view on tab click", async () => {
    mocks.search = {};
    const fetcher = createInstancesFetcher();
    await renderWithQueryClient(
      <TaskLaunchPage fetcher={fetcher as typeof fetch} title="任务中枢" />,
    );

    expect(getButton("提出任务").getAttribute("aria-selected")).toBe("true");
    expect(getButton("流程实例").getAttribute("aria-selected")).toBe("false");
    // 默认页签仍是提交表单，不渲染河道。
    expect(queryByText("流程实例 · 时间河道")).toBeNull();

    await clickButton("流程实例");
    expect(mocks.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        search: expect.objectContaining({ view: "instances" }),
        to: "."
      }),
    );
  });

  it("renders the river with ?view=instances, requests scope=active and hides the completed KPI", async () => {
    mocks.search = { view: "instances" };
    const fetcher = createInstancesFetcher();
    await renderWithQueryClient(
      <TaskLaunchPage fetcher={fetcher as typeof fetch} title="任务中枢" />,
    );

    await waitFor(() => expect(getByText("流程实例 · 时间河道")).toBeTruthy());
    expect(getButton("流程实例").getAttribute("aria-selected")).toBe("true");

    const urls = instancesRequestUrls(fetcher);
    expect(urls.length).toBeGreaterThan(0);
    expect(urls[0].searchParams.get("scope")).toBe("active");
    expect(urls[0].searchParams.get("q")).toBeNull();
    expect(urls[0].searchParams.get("project_id")).toBeNull();

    // 运行中口径：服务端排除终态，「已完成」KPI 恒零，隐藏（其 meta 文案不出现）。
    expect(queryByText("本视图范围")).toBeNull();
    expect(getByText("最长未完成实例")).toBeTruthy();
    // 口径页签更名：已结束（不再叫已归档）。
    expect(getButton("已结束")).toBeTruthy();
    expect(queryByText("已归档")).toBeNull();
  });

  it("carries the q deep-link param into the server query", async () => {
    mocks.search = { q: "回归", view: "instances" };
    const fetcher = createInstancesFetcher();
    await renderWithQueryClient(
      <TaskLaunchPage fetcher={fetcher as typeof fetch} title="任务中枢" />,
    );

    await waitFor(() => {
      const urls = instancesRequestUrls(fetcher);
      expect(urls.length).toBeGreaterThan(0);
      expect(urls[0].searchParams.get("q")).toBe("回归");
    });
  });
});

describe("WorkflowInstancesView server-side filtering", () => {
  it("sends the debounced search keyword as the q query param", async () => {
    const fetcher = createInstancesFetcher();
    await renderWithQueryClient(<InstancesHarness fetcher={fetcher as typeof fetch} />);

    await waitFor(() => expect(getByText("流程实例 · 时间河道")).toBeTruthy());

    const input = document.querySelector<HTMLInputElement>(
      '[aria-label="搜索流程实例"]',
    );
    expect(input).toBeTruthy();
    await act(async () => {
      setInputValue(input as HTMLInputElement, "验收回归");
    });

    await waitFor(() => {
      const withQ = instancesRequestUrls(fetcher).filter(
        (url) => url.searchParams.get("q") === "验收回归",
      );
      expect(withQ.length).toBeGreaterThan(0);
      expect(withQ[0].searchParams.get("scope")).toBe("active");
    });
  });

  it("sends the selected project as the project_id query param", async () => {
    const fetcher = createInstancesFetcher();
    await renderWithQueryClient(<InstancesHarness fetcher={fetcher as typeof fetch} />);

    await waitFor(() => expect(getByText("流程实例 · 时间河道")).toBeTruthy());
    // 项目下拉候选来自 listProjects。
    await clickButton("生产巡检项目");

    await waitFor(() => {
      const withProject = instancesRequestUrls(fetcher).filter(
        (url) => url.searchParams.get("project_id") === "project-2",
      );
      expect(withProject.length).toBeGreaterThan(0);
    });
  });

  it("requests scope=archived on the 已结束 tab and restores the completed KPI with ended-view duration copy", async () => {
    const fetcher = createInstancesFetcher();
    await renderWithQueryClient(<InstancesHarness fetcher={fetcher as typeof fetch} />);

    await waitFor(() => expect(getByText("流程实例 · 时间河道")).toBeTruthy());
    await clickButton("已结束");

    await waitFor(() => {
      const archived = instancesRequestUrls(fetcher).filter(
        (url) => url.searchParams.get("scope") === "archived",
      );
      expect(archived.length).toBeGreaterThan(0);
    });
    // 已结束口径恢复「已完成」KPI，且时长卡副文案不再暗示未完成。
    await waitFor(() => expect(getByText("本视图范围")).toBeTruthy());
    expect(getByText("最长历时实例")).toBeTruthy();
    expect(queryByText("最长未完成实例")).toBeNull();
  });
});
