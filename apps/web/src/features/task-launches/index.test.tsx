import { type ReactNode } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskLaunchView } from "@/features/task-launches";
import type { Project } from "@/lib/api/projects";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const mocks = vi.hoisted(() => ({
  navigate: vi.fn()
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
    SelectScrollDownButton: ({ children }: { children?: ReactNode }) => (
      <button type="button">{children}</button>
    ),
    SelectScrollUpButton: ({ children }: { children?: ReactNode }) => (
      <button type="button">{children}</button>
    ),
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
  Link: ({
    children,
    params,
    to
}: {
    children: ReactNode;
    params?: Record<string, string>;
    to: string;
  }) => {
    const href = params?.projectId
      ? to.replace("$projectId", encodeURIComponent(params.projectId))
      : to;
    return <a href={href}>{children}</a>;
  },
  useNavigate: () => mocks.navigate,
  useSearch: () => ({})
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

function makeProject(id = "project-1", status: Project["status"] = "running"): Project {
  return {
    coordination_policy: {},
    coordination_status: "registered",
    coordination_workflow_id: `project-coordinator:${id}`,
    directory_name: `${id}-dir`,
    goal: "完成一次任务发起",
    human_owner_user_id: id === "project-2" ? "owner-2" : "owner-1",
    id,
    name:
      id === "project-1"
        ? "客户接入项目"
        : id === "project-2"
          ? "生产巡检项目"
          : "归档项目",
    status,
    tenant_id: "tenant-1",
    workspace_ready_status: "ready"
};
}

function createTaskLaunchFetcher({
  includeSecondProject = false,
  launchDetail = false,
  emptyFacts = false
}: {
  emptyFacts?: boolean;
  includeSecondProject?: boolean;
  launchDetail?: boolean;
} = {}) {
  const submittedDemand = {
    attachments: [],
    content: "审查这个开源项目的 PR，并按数量分配数字员工",
    id: "demand-1",
    project_id: "project-1",
    reviewer: {
      project_role: "reviewer",
      resolved_from_rule: true,
      reviewer_user_id: "reviewer-1",
      selection_reason: "project_reviewer_default"
},
    source_refs: {},
    source_type: "manual",
    status: "submitted",
    submitted_by_user_id: "owner-1",
    tenant_id: "tenant-1",
    title: "审查这个开源项目的 PR，并按数量分配数字员工"
};
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (
      url.pathname === "/api/v1/projects" &&
      url.searchParams.get("limit") === "50" &&
      url.searchParams.get("offset") === "0" &&
      method === "GET"
    ) {
      return jsonResponse([
        makeProject("archived-project", "archived"),
        makeProject(),
        ...(includeSecondProject ? [makeProject("project-2")] : []),
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/demands" && method === "POST") {
      return jsonResponse(submittedDemand, 201);
    }
    if (url.pathname === "/api/v1/projects/project-2/demands" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { title?: string; content?: string };
      return jsonResponse(
        {
          ...submittedDemand,
          content: body.content,
          id: "demand-2",
          project_id: "project-2",
          reviewer: {
            project_role: "reviewer",
            resolved_from_rule: true,
            reviewer_user_id: "reviewer-2",
            selection_reason: "project_reviewer_default"
},
          title: body.title
},
        201,
      );
    }
    if (
      launchDetail &&
      url.pathname === "/api/v1/project-demands/demand-1/launch-detail" &&
      method === "GET"
    ) {
      return jsonResponse(makeLaunchDetail({ emptyFacts }));
    }

    return jsonResponse({ message: `Unhandled ${method} ${url.pathname}` }, 404);
  });
}

/** Extends the base task-launch fetcher with chat-mode endpoints (a single digital
 * employee and a run that completes immediately) so tests can drive the
 * chat -> 转为任务 -> submit flow and assert the resulting demand's source_refs. */
function createTaskLaunchFetcherWithChat({
  includeSecondProject = false
}: { includeSecondProject?: boolean } = {}) {
  const base = createTaskLaunchFetcher({ includeSecondProject });

  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const path = url.pathname;

    if (path === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse([
        {
                context_policy: {},
          employee_type: "generalist",
          id: "emp-1",
          name: "Ada",
          owner_user_id: "owner-1",
          permission_policy: {},
          provider_type: "claude_code",
          risk_level: "low",
          role: "客服助手",
          status: "active",
          tenant_id: "tenant-1"
},
      ]);
    }
    // 参与门禁:chat 员工按项目成员过滤,emp-1 投影为锚点项目的 active 成员。
    const membersMatch = path.match(/^\/api\/v1\/projects\/([^/]+)\/members$/);
    if (membersMatch && method === "GET") {
      return jsonResponse([
        {
          id: "member-1",
          tenant_id: "tenant-1",
          project_id: membersMatch[1],
          principal_type: "digital_employee",
          principal_id: "emp-1",
          project_role: "executor",
          status: "active"
},
      ]);
    }
    if (path === "/api/v1/digital-employees/emp-1/runs" && method === "GET") {
      // 会话恢复查询:页面级用例不预置历史会话,返回空列表。
      return jsonResponse({
        filters: { projects: [], statuses: [] },
        items: [],
        total_count: 0
});
    }
    if (path === "/api/v1/digital-employees/emp-1/runs" && method === "POST") {
      return jsonResponse(
        {
          command_id: "cmd-run-1",
          digital_employee_id: "emp-1",
          execution_instance_id: "instance-1",
          id: "run-1",
          node_id: "node-1",
          provider_type: "claude_code",
          run_kind: "chat",
          runtime_node_id: "node-1",
          session_state: {},
          status: "queued",
          task_id: "task-run-1",
          tenant_id: "tenant-1",
          timed_out: false,
          diagnostic: {},
          result: {},
          work_products: []
},
        201,
      );
    }
    if (path === "/api/v1/digital-employees/emp-1/runs/run-1" && method === "GET") {
      return jsonResponse({
        command_id: "cmd-run-1",
        digital_employee_id: "emp-1",
        execution_instance_id: "instance-1",
        id: "run-1",
        node_id: "node-1",
        provider_type: "claude_code",
        run_kind: "chat",
        runtime_node_id: "node-1",
        session_state: {},
        status: "completed",
        task_id: "task-run-1",
        tenant_id: "tenant-1",
        timed_out: false,
        diagnostic: {},
        result: { output: "这是对话回答" },
        work_products: []
});
    }

    return base(input, init);
  });
}

function makeLaunchDetail({
  demandId = "demand-1",
  emptyFacts = false,
  title = "审查 PR"
}: {
  demandId?: string;
  emptyFacts?: boolean;
  title?: string;
} = {}) {
  const baseDemand = {
    attachments: [],
    content: "统计 PR 数量，生成审查分工",
    id: demandId,
    project_id: "project-1",
    reviewer: {
      display_name: "王审核",
      project_role: "reviewer",
      resolved_from_rule: true,
      reviewer_user_id: "reviewer-1",
      selection_reason: "project_reviewer_default"
},
    source_refs: {},
    source_type: "manual",
    status: "submitted",
    submitted_by_user_id: "owner-1",
    tenant_id: "tenant-1",
    title
};
  const empty = {
    coordination_jobs: [],
    decision_requests: [],
    project_tasks: [],
    route_decisions: []
};

  return {
    demand: baseDemand,
    project: makeProject(),
    reviewer: baseDemand.reviewer,
    recent_events: [],
    ...(emptyFacts
      ? empty
      : {
          coordination_jobs: [
            {
              created_at: "2026-06-12T09:00:00Z",
              demand_id: demandId,
              id: "job-1",
              job_type: "demand_intake",
              project_id: "project-1",
              status: "running",
              tenant_id: "tenant-1",
              workflow_id: "project-coordinator:project-1"
},
          ],
          decision_requests: [
            {
              id: "decision-1",
              project_id: "project-1",
              status_snapshot: "pending",
              target_user_id: "reviewer-1",
              tenant_id: "tenant-1",
              title_snapshot: "确认路由"
},
          ],
          project_tasks: [
            {
              demand_id: demandId,
              id: "task-1",
              project_id: "project-1",
              requires_human_approval: true,
              status: "pending",
              summary: "汇总 PR 并输出分派建议",
              tenant_id: "tenant-1",
              title: "整理审查清单"
},
          ],
          route_decisions: [
            {
              budget_estimate: {},
              candidate_digital_employee_ids: ["employee-1"],
              coordination_job_id: "job-1",
              demand_id: demandId,
              expected_outputs: [],
              id: "route-1",
              input_requirements: {},
              project_id: "project-1",
              reason: "按能力分派",
              requires_human_review: true,
              selected_digital_employee_ids: ["employee-1"],
              tenant_id: "tenant-1"
},
          ]
})
};
}

async function renderWithQueryClient(children: ReactNode) {
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  const queryClient = createQueryClient();
  mountedRoots.push(root);

  await act(async () => {
    root.render(<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>);
  });

  return { container, queryClient, root };
}

describe("TaskLaunchView", () => {
  afterEach(() => {
    for (const root of mountedRoots.splice(0)) {
      act(() => {
        root.unmount();
      });
    }
    document.body.innerHTML = "";
  });

  it("submits demand without reviewer fields", async () => {
    mocks.navigate.mockClear();
    const fetcher = createTaskLaunchFetcher();
    await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await typeInLabeledField("需求描述", "审查这个开源项目的 PR，并按数量分配数字员工");
    await waitFor(() => expect(getByText("客户接入项目")).toBeTruthy());

    await clickButton("提交任务");

    await waitFor(() => {
      expect(postBody(fetcher, "/api/v1/projects/project-1/demands")).toEqual({
        title: "审查这个开源项目的 PR，并按数量分配数字员工",
        content: "审查这个开源项目的 PR，并按数量分配数字员工",
        source_type: "manual",
        source_refs: {},
        attachments: [],
        coordination_mode: "plan"
});
    });
    expect(fetchPaths(fetcher)).not.toContain("/api/v1/projects/project-1/members");
    await waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        params: { projectId: "project-1" },
        search: { demand: "demand-1", tab: "demands" },
        to: "/projects/$projectId"
});
    });
  });

  it("submits demand for the selected project without reviewer fields", async () => {
    mocks.navigate.mockClear();
    const fetcher = createTaskLaunchFetcher({ includeSecondProject: true });
    await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await clickButton("项目");
    await waitFor(() => expect(getByText("生产巡检项目")).toBeTruthy());
    await clickButton("生产巡检项目");
    await typeInLabeledField("需求描述", "处理第二个项目的巡检问题");

    await clickButton("提交任务");

    const body = postBody(fetcher, "/api/v1/projects/project-2/demands");
    expect(body).toMatchObject({
      content: "处理第二个项目的巡检问题",
      title: "处理第二个项目的巡检问题"
});
    expect(body).not.toHaveProperty("reviewer_user_id");
    expect(body).not.toHaveProperty("reviewer_selection_reason");
    expect(fetchPaths(fetcher)).not.toContain("/api/v1/projects/project-2/members");
    await waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        params: { projectId: "project-2" },
        search: { demand: "demand-2", tab: "demands" },
        to: "/projects/$projectId"
});
    });
  });

  it("switches to loop mode and submits coordination_mode loop", async () => {
    mocks.navigate.mockClear();
    const fetcher = createTaskLaunchFetcher();
    await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await waitFor(() => expect(getByText("客户接入项目")).toBeTruthy());
    await clickButton("Loop 任务");
    await typeInLabeledField("需求描述", "遇到阻塞时自动补做上游任务");

    await clickButton("提交任务");

    await waitFor(() => {
      expect(postBody(fetcher, "/api/v1/projects/project-1/demands")).toMatchObject({
        coordination_mode: "loop"
});
    });
  });

  it("renders the pre-submit launch composer without orchestration state controls", async () => {
    const fetcher = createTaskLaunchFetcher();
    await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await waitFor(() => expect(getByText("中枢指令区")).toBeTruthy());

    expect(getByText("提出任务")).toBeTruthy();
    expect(queryByText("提交后由协调线程动态编排")).toBeNull();
    expect(getByText("中枢指令区")).toBeTruthy();
    expect(getByText("命令中心")).toBeTruthy();
    expect(getByText("保存草稿")).toBeTruthy();
    expect(getByLabelText("项目")).toBeTruthy();
    expect(() => getByLabelText("审核人")).toThrow();
    expect(queryByText("审核人")).toBeNull();
    expect(() => getByLabelText("优先级")).toThrow();
    expect(() => getByLabelText("风险级别")).toThrow();
    expect(queryByText("优先级")).toBeNull();
    expect(queryByText("风险级别")).toBeNull();
    expect(document.querySelector('[data-testid="task-launch-parameters"]')).toBeTruthy();
    expect(document.querySelector(".glass")).toBeTruthy();
    expect(document.querySelector(".tl-btn-send")).toBeTruthy();

    expect(queryByText("Command Center")).toBeNull();
    expect(queryByText("Project routing")).toBeNull();
    expect(queryByText("提交后会发生什么")).toBeNull();
    expect(queryByText("写入项目需求")).toBeNull();
    expect(queryByText("启动协调线程")).toBeNull();
    expect(queryByText("生成编排决策")).toBeNull();
    expect(queryByText("进入运行视图")).toBeNull();
    expect(queryByText("提交前确认")).toBeNull();
    expect(queryByText("协同资料")).toBeNull();
    expect(queryByText("添加附件")).toBeNull();
    expect(queryByText("关联链接")).toBeNull();
    expect(queryByText("导入资料")).toBeNull();
    expect(queryByText("上下文边界")).toBeNull();
    expect(queryByText("备注")).toBeNull();
    expect(queryByText("待提交")).toBeNull();
    expect(queryByText("待生成")).toBeNull();
    expect(queryByText("已完成")).toBeNull();
    expect(queryByText("运行中")).toBeNull();
  });

  it("switches to chat mode: hides the task-form parameter grid but keeps a required 项目 chip inside the chat panel slot", async () => {
    const fetcher = createTaskLaunchFetcher();
    await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await waitFor(() => expect(getByText("客户接入项目")).toBeTruthy());
    await clickButton("对话");

    await waitFor(() => {
      expect(document.querySelector('[data-testid="chat-panel-slot"]')).toBeTruthy();
    });
    expect(document.querySelector('[data-testid="task-launch-parameters"]')).toBeNull();
    // the 项目 chip moved into the chat panel (same aria-label/select pattern as
    // task mode) rather than disappearing: chat runs are anchored to a project.
    expect(getByLabelText("项目")).toBeTruthy();
  });

  it("converts a chat answer to a task and submits it with chat source_refs lineage", async () => {
    mocks.navigate.mockClear();
    const fetcher = createTaskLaunchFetcherWithChat();
    const { queryClient } = await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await waitFor(() => expect(getByText("客户接入项目")).toBeTruthy());
    await clickButton("对话");
    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    await typeInLabeledField("对话问题", "如何配置这个项目？");
    await clickButton("发送");

    await waitFor(() => {
      expect(postBody(fetcher, "/api/v1/digital-employees/emp-1/runs")).toMatchObject({
        project_id: "project-1"
});
    });

    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(getByText("这是对话回答")).toBeTruthy());

    await clickButton("转为任务");

    // conversion switches the composer back to plan mode with the draft prefilled
    await waitFor(() => expect(getByLabelText("项目")).toBeTruthy());

    await clickButton("提交任务");

    await waitFor(() => {
      expect(postBody(fetcher, "/api/v1/projects/project-1/demands")).toMatchObject({
        source_refs: { chat_run_id: "run-1", digital_employee_id: "emp-1" }
});
    });
    await waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        params: { projectId: "project-1" },
        search: { demand: "demand-1", tab: "demands" },
        to: "/projects/$projectId"
});
    });
  });

  it("defaults the post-conversion demand to the chat run's anchor project (not the form's original default)", async () => {
    mocks.navigate.mockClear();
    const fetcher = createTaskLaunchFetcherWithChat({ includeSecondProject: true });
    const { queryClient } = await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await waitFor(() => expect(getByText("客户接入项目")).toBeTruthy());
    await clickButton("对话");
    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    // anchor the chat conversation to the second project before asking anything
    await clickButton("项目");
    await clickButton("生产巡检项目");
    // 参与门禁：换锚点项目后成员列表与会话恢复各需一轮查询落定
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());
    await act(async () => {
      await queryClient.refetchQueries();
    });

    await typeInLabeledField("对话问题", "第二个项目怎么配置？");
    await clickButton("发送");

    await waitFor(() => {
      expect(postBody(fetcher, "/api/v1/digital-employees/emp-1/runs")).toMatchObject({
        project_id: "project-2"
});
    });

    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(getByText("这是对话回答")).toBeTruthy());

    await clickButton("转为任务");
    await waitFor(() => expect(getByLabelText("项目")).toBeTruthy());

    // no manual project change here: conversion must have pre-selected project-2
    // (the chat anchor), overriding the form's original project-1 default
    await clickButton("提交任务");

    await waitFor(() => {
      expect(postBody(fetcher, "/api/v1/projects/project-2/demands")).toMatchObject({
        source_refs: { chat_run_id: "run-1", digital_employee_id: "emp-1" }
});
    });
    await waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        params: { projectId: "project-2" },
        search: { demand: "demand-2", tab: "demands" },
        to: "/projects/$projectId"
});
    });
  });

  it("clears chat source lineage when switching back to chat mode before submitting", async () => {
    mocks.navigate.mockClear();
    const fetcher = createTaskLaunchFetcherWithChat();
    const { queryClient } = await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await waitFor(() => expect(getByText("客户接入项目")).toBeTruthy());
    await clickButton("对话");
    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    await typeInLabeledField("对话问题", "如何配置这个项目？");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(getByText("这是对话回答")).toBeTruthy());

    await clickButton("转为任务");
    await waitFor(() => expect(getByLabelText("项目")).toBeTruthy());

    // switch back to chat, then to plan for an unrelated demand: lineage must not
    // leak onto this later submit
    await clickButton("对话");
    await clickButton("Plan 任务");
    await typeInLabeledField("需求描述", "一个与对话无关的新需求");

    await clickButton("提交任务");

    await waitFor(() => {
      expect(postBody(fetcher, "/api/v1/projects/project-1/demands")).toMatchObject({
        source_refs: {}
});
    });
  });
});

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

function getByLabelText(label: string) {
  const element = document.querySelector<HTMLElement>(`[aria-label="${label}"]`);
  if (!element) {
    throw new Error(`Unable to find label: ${label}`);
  }
  return element;
}

function postBody(fetcher: ReturnType<typeof createTaskLaunchFetcher>, path: string) {
  const call = fetcher.mock.calls.find(([url, init]) => {
    const parsed = new URL(String(url));
    return parsed.pathname === path && ((init as RequestInit | undefined)?.method ?? "GET") === "POST";
  });
  expect(call).toBeDefined();
  return JSON.parse(String(call?.[1]?.body)) as Record<string, unknown>;
}

function fetchPaths(fetcher: ReturnType<typeof createTaskLaunchFetcher>) {
  return fetcher.mock.calls.map(([url]) => new URL(String(url)).pathname);
}

async function typeInLabeledField(label: string, value: string) {
  await waitFor(() => expect(getByLabelText(label)).toBeTruthy());
  const input = getByLabelText(label) as HTMLInputElement | HTMLTextAreaElement;
  await act(async () => {
    setInputValue(input, value);
  });
}

async function clickButton(name: string) {
  await waitFor(() => expect(getButton(name).disabled).toBe(false));
  const button = getButton(name);
  await act(async () => {
    button.click();
  });
}

function getButton(name: string) {
  const button = Array.from(document.body.querySelectorAll<HTMLButtonElement>("button")).find(
    (item) => item.textContent === name || item.getAttribute("aria-label") === name,
  );
  if (!button) {
    throw new Error(`Unable to find button: ${name}`);
  }
  return button;
}

async function waitFor(assertion: () => void) {
  await act(async () => {
    await vi.waitFor(assertion);
  });
}

function setInputValue(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const valueSetter = Object.getOwnPropertyDescriptor(input, "value")?.set;
  const prototype = input instanceof HTMLTextAreaElement ? HTMLTextAreaElement : HTMLInputElement;
  const prototypeValueSetter = Object.getOwnPropertyDescriptor(prototype.prototype, "value")?.set;

  if (prototypeValueSetter && valueSetter !== prototypeValueSetter) {
    prototypeValueSetter.call(input, value);
  } else {
    valueSetter?.call(input, value);
  }
  input.dispatchEvent(new Event("input", { bubbles: true }));
}
