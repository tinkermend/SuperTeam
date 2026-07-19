import { type ReactNode, useState } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ChatPanel,
  type ChatPanelProps,
  type ConvertToTaskPayload,
} from "@/features/task-launches/components/chat-panel";
import type { DigitalEmployee, DigitalEmployeeRun } from "@/lib/api/employees";
import type { Project } from "@/lib/api/projects";

/** Test harness: ChatPanel's project selection is now controlled by the parent
 * (mirrors how index.tsx owns `selectedProjectId` and passes it down), so tests
 * hold that bit of state locally instead of re-fetching a projects list. */
function ControlledChatPanel({
  apiOptions,
  initialProjectId = "project-1",
  onConvertToTask,
  onProjectChange,
  projects,
}: {
  apiOptions: ChatPanelProps["apiOptions"];
  initialProjectId?: string;
  onConvertToTask: (payload: ConvertToTaskPayload) => void;
  onProjectChange?: (projectId: string) => void;
  projects: Project[];
}) {
  const [projectId, setProjectId] = useState(initialProjectId);
  return (
    <ChatPanel
      apiOptions={apiOptions}
      onConvertToTask={onConvertToTask}
      onProjectChange={(nextProjectId) => {
        setProjectId(nextProjectId);
        onProjectChange?.(nextProjectId);
      }}
      projectId={projectId}
      projects={projects}
    />
  );
}

function makeProject(id = "project-1", name = "客户接入项目"): Project {
  return {
    approval_policy: {},
    coordination_policy: {},
    coordination_status: "registered",
    coordination_workflow_id: `project-coordinator:${id}`,
    evidence_policy: {},
    goal: "完成一次任务发起",
    human_owner_user_id: "owner-1",
    id,
    name,
    status: "running",
    tenant_id: "tenant-1",
  };
}

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const mountedRoots: Root[] = [];

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
      value,
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
      children,
    }: {
      "aria-label"?: string;
      children: ReactNode;
    }) => (
      <button aria-label={ariaLabel} type="button">
        {children}
      </button>
    ),
    SelectValue: ({ placeholder }: { placeholder?: string }) => <span>{placeholder}</span>,
  };
});

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function makeEmployee(): DigitalEmployee {
  return {
    employee_type: "generalist",
    id: "emp-1",
    name: "Ada",
    owner_user_id: "owner-1",
    permission_policy: {},
    provider_type: "claude_code",
    risk_level: "low",
    role: "客服助手",
    status: "active",
    tenant_id: "tenant-1",
  };
}

function baseRunFields(runId: string, employeeId: string): Partial<DigitalEmployeeRun> {
  return {
    command_id: `cmd-${runId}`,
    digital_employee_id: employeeId,
    execution_instance_id: "instance-1",
    id: runId,
    node_id: "node-1",
    provider_type: "claude_code",
    run_kind: "chat",
    runtime_node_id: "node-1",
    session_state: {},
    task_id: `task-${runId}`,
    tenant_id: "tenant-1",
    timed_out: false,
    diagnostic: {},
    result: {},
    work_products: [],
  };
}

function emptyRunListResponse() {
  return jsonResponse({
    filters: { projects: [], statuses: [] },
    items: [],
    total_count: 0,
  });
}

/** 参与门禁：ChatPanel 现在按项目成员过滤员工，测试 fetcher 统一把
 * mock 员工全部投影为锚点项目的 active digital_employee 成员。 */
function projectMembersRouteResponse(
  path: string,
  method: string,
  employees: DigitalEmployee[],
): Response | null {
  const membersMatch = path.match(/^\/api\/v1\/projects\/([^/]+)\/members$/);
  if (!membersMatch || method !== "GET") {
    return null;
  }
  return jsonResponse(
    employees.map((employee, index) => ({
      id: `member-${index + 1}`,
      tenant_id: "tenant-1",
      project_id: membersMatch[1],
      principal_type: "digital_employee",
      principal_id: employee.id,
      project_role: "executor",
      status: "active",
    })),
  );
}

function createChatFetcher() {
  const employees = [makeEmployee()];
  const runScripts = new Map<string, Array<Partial<DigitalEmployeeRun>>>();
  const runGetCallCounts = new Map<string, number>();
  let runCounter = 0;

  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const path = url.pathname;

    if (path === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(employees);
    }

    const membersResponse = projectMembersRouteResponse(path, method, employees);
    if (membersResponse) {
      return membersResponse;
    }

    const createMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
    if (createMatch && method === "GET") {
      // 会话恢复查询:这些场景不预置历史会话,返回空列表即"无可恢复内容"。
      return emptyRunListResponse();
    }
    if (createMatch && method === "POST") {
      runCounter += 1;
      const runId = `run-${runCounter}`;
      const employeeId = createMatch[1];
      const body = JSON.parse(String(init?.body)) as {
        objective: string;
        run_kind?: string;
        resume_of_run_id?: string;
      };
      return jsonResponse(
        {
          ...baseRunFields(runId, employeeId),
          status: "queued",
          ...(body.resume_of_run_id ? { resume_of_run_id: body.resume_of_run_id } : {}),
        },
        201,
      );
    }

    const getMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs\/([^/]+)$/);
    if (getMatch && method === "GET") {
      const employeeId = getMatch[1];
      const runId = getMatch[2];
      const callIndex = runGetCallCounts.get(runId) ?? 0;
      runGetCallCounts.set(runId, callIndex + 1);
      const script = runScripts.get(runId) ?? [];
      const step = script[Math.min(callIndex, script.length - 1)] ?? { status: "running" };
      return jsonResponse({
        ...baseRunFields(runId, employeeId),
        status: "running",
        ...step,
      });
    }

    return jsonResponse({ message: `Unhandled ${method} ${path}` }, 404);
  });

  return {
    fetcher,
    setRunScript: (runId: string, script: Array<Partial<DigitalEmployeeRun>>) =>
      runScripts.set(runId, script),
  };
}

function createFailingSendFetcher() {
  const employees = [makeEmployee()];

  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const path = url.pathname;

    if (path === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(employees);
    }

    const membersResponse = projectMembersRouteResponse(path, method, employees);
    if (membersResponse) {
      return membersResponse;
    }

    const createMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
    if (createMatch && method === "GET") {
      // 会话恢复查询:这些场景不预置历史会话,返回空列表即"无可恢复内容"。
      return emptyRunListResponse();
    }
    if (createMatch && method === "POST") {
      return jsonResponse({ message: "员工繁忙，暂时无法接单" }, 409);
    }

    return jsonResponse({ message: `Unhandled ${method} ${path}` }, 404);
  });

  return { fetcher };
}

function createRetryDeferredFetcher() {
  const employees = [makeEmployee()];
  let createCallCount = 0;
  let resolveRetry: ((response: Response) => void) | null = null;
  const retryResponse = new Promise<Response>((resolve) => {
    resolveRetry = resolve;
  });

  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const path = url.pathname;

    if (path === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(employees);
    }

    const membersResponse = projectMembersRouteResponse(path, method, employees);
    if (membersResponse) {
      return membersResponse;
    }

    const createMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
    if (createMatch && method === "GET") {
      // 会话恢复查询:这些场景不预置历史会话,返回空列表即"无可恢复内容"。
      return emptyRunListResponse();
    }
    if (createMatch && method === "POST") {
      createCallCount += 1;
      const runId = `run-${createCallCount}`;
      const employeeId = createMatch[1];
      if (createCallCount === 1) {
        return jsonResponse(
          {
            ...baseRunFields(runId, employeeId),
            status: "failed",
            error_message: "第一次失败",
          },
          201,
        );
      }
      // Second (and any further) create call is the retry: keep it pending until
      // the test explicitly resolves it, so it can assert only one retry POST fired.
      return retryResponse;
    }

    const getMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs\/([^/]+)$/);
    if (getMatch && method === "GET") {
      const employeeId = getMatch[1];
      const runId = getMatch[2];
      return jsonResponse({
        ...baseRunFields(runId, employeeId),
        status: "failed",
        error_message: "第一次失败",
      });
    }

    return jsonResponse({ message: `Unhandled ${method} ${path}` }, 404);
  });

  return {
    fetcher,
    getCreateCallCount: () => createCallCount,
    resolveRetry: () =>
      resolveRetry?.(
        jsonResponse({ ...baseRunFields("run-2", "emp-1"), status: "queued" }, 201),
      ),
  };
}

/** First message succeeds and completes; the resumed second send is rejected with a
 * 400 (server treats the resume session as invalid/lost), and the degraded resend
 * (no resume_of_run_id) succeeds. */
function createResumeDegradeFetcher() {
  const employees = [makeEmployee()];
  let createCallCount = 0;

  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const path = url.pathname;

    if (path === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(employees);
    }

    const membersResponse = projectMembersRouteResponse(path, method, employees);
    if (membersResponse) {
      return membersResponse;
    }

    const createMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
    if (createMatch && method === "GET") {
      // 会话恢复查询:这些场景不预置历史会话,返回空列表即"无可恢复内容"。
      return emptyRunListResponse();
    }
    if (createMatch && method === "POST") {
      createCallCount += 1;
      const employeeId = createMatch[1];
      const body = JSON.parse(String(init?.body)) as {
        objective: string;
        resume_of_run_id?: string;
      };
      if (createCallCount === 1) {
        return jsonResponse(
          { ...baseRunFields("run-1", employeeId), status: "queued" },
          201,
        );
      }
      if (createCallCount === 2 && body.resume_of_run_id) {
        return jsonResponse({ message: "会话已失效，无法继续上下文" }, 400);
      }
      return jsonResponse(
        { ...baseRunFields("run-2", employeeId), status: "queued" },
        201,
      );
    }

    const getMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs\/([^/]+)$/);
    if (getMatch && method === "GET") {
      const employeeId = getMatch[1];
      const runId = getMatch[2];
      return jsonResponse({
        ...baseRunFields(runId, employeeId),
        status: "completed",
        result: { output: `回答-${runId}` },
      });
    }

    return jsonResponse({ message: `Unhandled ${method} ${path}` }, 404);
  });

  return { fetcher, getCreateCallCount: () => createCallCount };
}

/** First message succeeds and completes; the resumed second send fails with a
 * non-400 error, which must NOT trigger an automatic no-resume resend. */
function createNonResumableFailureFetcher() {
  const employees = [makeEmployee()];
  let createCallCount = 0;

  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const path = url.pathname;

    if (path === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(employees);
    }

    const membersResponse = projectMembersRouteResponse(path, method, employees);
    if (membersResponse) {
      return membersResponse;
    }

    const createMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
    if (createMatch && method === "GET") {
      // 会话恢复查询:这些场景不预置历史会话,返回空列表即"无可恢复内容"。
      return emptyRunListResponse();
    }
    if (createMatch && method === "POST") {
      createCallCount += 1;
      const employeeId = createMatch[1];
      if (createCallCount === 1) {
        return jsonResponse(
          { ...baseRunFields("run-1", employeeId), status: "queued" },
          201,
        );
      }
      return jsonResponse({ message: "员工繁忙，暂时无法接单" }, 409);
    }

    const getMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs\/([^/]+)$/);
    if (getMatch && method === "GET") {
      const employeeId = getMatch[1];
      const runId = getMatch[2];
      return jsonResponse({
        ...baseRunFields(runId, employeeId),
        status: "completed",
        result: { output: "首轮回答" },
      });
    }

    return jsonResponse({ message: `Unhandled ${method} ${path}` }, 404);
  });

  return { fetcher, getCreateCallCount: () => createCallCount };
}

type RestoreThreadItem = Partial<DigitalEmployeeRun> & {
  task_title: string;
  work_product_count?: number;
};

/** Serves a persisted chat conversation for the restore-on-mount queries:
 * GET /runs?run_kind=chat&limit=1 returns the newest run, GET /runs?chat_thread_id=…
 * returns the whole thread (both created_at-desc, as the server does). POST /runs
 * and GET /runs/:id behave like createChatFetcher's immediate-completion runs. */
function createRestoreFetcher(threadItemsAsc: RestoreThreadItem[]) {
  const employees = [makeEmployee()];
  let runCounter = 100;

  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    const path = url.pathname;

    if (path === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(employees);
    }

    const membersResponse = projectMembersRouteResponse(path, method, employees);
    if (membersResponse) {
      return membersResponse;
    }

    const runsMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
    if (runsMatch && method === "GET") {
      const employeeId = runsMatch[1];
      const desc = [...threadItemsAsc]
        .reverse()
        .map((item) => ({ ...baseRunFields(String(item.id), employeeId), ...item }));
      const items = url.searchParams.get("chat_thread_id") ? desc : desc.slice(0, 1);
      return jsonResponse({
        filters: { projects: [], statuses: [] },
        items,
        total_count: desc.length,
      });
    }
    if (runsMatch && method === "POST") {
      runCounter += 1;
      const body = JSON.parse(String(init?.body)) as { resume_of_run_id?: string };
      return jsonResponse(
        {
          ...baseRunFields(`run-${runCounter}`, runsMatch[1]),
          status: "queued",
          ...(body.resume_of_run_id ? { resume_of_run_id: body.resume_of_run_id } : {}),
        },
        201,
      );
    }

    const getMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs\/([^/]+)$/);
    if (getMatch && method === "GET") {
      return jsonResponse({
        ...baseRunFields(getMatch[2], getMatch[1]),
        status: "completed",
        result: { output: `轮询回答-${getMatch[2]}` },
      });
    }

    return jsonResponse({ message: `Unhandled ${method} ${path}` }, 404);
  });

  return { fetcher };
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

describe("ChatPanel", () => {
  afterEach(() => {
    for (const root of mountedRoots.splice(0)) {
      act(() => {
        root.unmount();
      });
    }
    document.body.innerHTML = "";
  });

  it("lists mock employees by name and role, sends a first question without resume_of_run_id, renders the completed answer, sends a follow-up with resume_of_run_id, converts to a task draft, and retries a failed run resuming the last completed turn", async () => {
    const { fetcher, setRunScript } = createChatFetcher();
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    // 1. employee select lists mock employees (name/role)
    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    // 2. send first question -> POST without resume_of_run_id
    setRunScript("run-1", [
      { status: "running" },
      { status: "completed", result: { output: "这是第一轮的回答内容" } },
    ]);
    await typeInLabeledField("对话问题", "第一个问题");
    await clickButton("发送");

    await waitFor(() => {
      const body = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs")[0];
      expect(body).toEqual({
        objective: "第一个问题",
        run_kind: "chat",
        project_id: "project-1",
      });
    });

    // 3. running -> completed; answer renders inside chat-thread
    await waitFor(() => expect(chatThread().textContent).toContain("数字员工思考中"));
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("这是第一轮的回答内容"));

    // 4. follow-up question -> second POST carries resume_of_run_id: run-1
    setRunScript("run-2", [
      { status: "running" },
      { status: "failed", error_message: "对话执行失败，请重试" },
    ]);
    await typeInLabeledField("对话问题", "第二个问题");
    await clickButton("发送");

    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies[1]).toEqual({
        objective: "第二个问题",
        run_kind: "chat",
        project_id: "project-1",
        resume_of_run_id: "run-1",
      });
    });

    // 5. convert first (completed) answer to a task draft
    await clickButton("转为任务");
    expect(onConvertToTask).toHaveBeenCalledTimes(1);
    const payload = onConvertToTask.mock.calls[0][0] as ConvertToTaskPayload;
    expect(payload.draft).toContain("第一个问题");
    expect(payload.draft).toContain("这是第一轮的回答内容");
    expect(payload.chatRunId).toBe("run-1");
    expect(payload.digitalEmployeeId).toBe("emp-1");
    expect(payload.anchorProjectId).toBe("project-1");

    // 6. second run fails -> error card + retry; retry stays on the current
    // conversation by resuming the last completed turn (run-1)
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("对话执行失败，请重试"));

    await clickButton("重试");

    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies).toHaveLength(3);
      expect(bodies[2]).toEqual({
        objective: "第二个问题",
        run_kind: "chat",
        project_id: "project-1",
        resume_of_run_id: "run-1",
      });
    });
  });

  it("keeps the typed question in the textarea and re-enables send after a failed create-run request", async () => {
    const { fetcher } = createFailingSendFetcher();
    const onConvertToTask = vi.fn();
    await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    await typeInLabeledField("对话问题", "会失败的问题");
    await clickButton("发送");

    // the failing POST fires and rejects
    await waitFor(() => {
      expect(postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs")).toHaveLength(1);
    });

    // the typed question is preserved rather than cleared on error
    await waitFor(() => {
      const textarea = getByLabelText("对话问题") as HTMLTextAreaElement;
      expect(textarea.value).toBe("会失败的问题");
    });

    // send becomes enabled again once the mutation settles (not pending)
    await waitFor(() => expect(getButton("发送").disabled).toBe(false));
  });

  it("guards against a fast double-click on retry: only one retry POST is issued while pending", async () => {
    const { fetcher, getCreateCallCount, resolveRetry } = createRetryDeferredFetcher();
    const onConvertToTask = vi.fn();
    await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    await typeInLabeledField("对话问题", "第一次问题");
    await clickButton("发送");

    await waitFor(() => expect(chatThread().textContent).toContain("对话失败"));
    expect(getCreateCallCount()).toBe(1);

    // fast double-click the retry button before the (deliberately delayed) retry
    // POST resolves; the in-flight guard must prevent a second POST. Grab a single
    // element reference so a second click is issued even if the retry affordance
    // gets hidden/removed once the mutation is pending.
    const retryButton = getButton("重试");
    await act(async () => {
      retryButton.click();
      retryButton.click();
    });

    expect(getCreateCallCount()).toBe(2);

    await act(async () => {
      resolveRetry();
      await Promise.resolve();
    });

    await waitFor(() => expect(getCreateCallCount()).toBe(2));
  });

  it("auto-resends once without resume_of_run_id when a resumed send is rejected with 400, and surfaces a context-not-continued notice", async () => {
    const { fetcher, getCreateCallCount } = createResumeDegradeFetcher();
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    await typeInLabeledField("对话问题", "第一个问题");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("回答-run-1"));

    await typeInLabeledField("对话问题", "第二个问题");
    await clickButton("发送");

    await waitFor(() => expect(getCreateCallCount()).toBe(3));
    const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
    expect(bodies[1]).toEqual({
      objective: "第二个问题",
      run_kind: "chat",
      project_id: "project-1",
      resume_of_run_id: "run-1",
    });
    expect(bodies[2]).toEqual({
      objective: "第二个问题",
      run_kind: "chat",
      project_id: "project-1",
    });

    await waitFor(() => expect(chatThread().textContent).toContain("上下文未延续"));
  });

  it("does not auto-resend when a resumed send fails with a non-400 error", async () => {
    const { fetcher, getCreateCallCount } = createNonResumableFailureFetcher();
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    await typeInLabeledField("对话问题", "第一个问题");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("首轮回答"));

    await typeInLabeledField("对话问题", "第二个问题");
    await clickButton("发送");

    await waitFor(() => expect(document.body.textContent).toContain("员工繁忙，暂时无法接单"));
    expect(getCreateCallCount()).toBe(2);
    expect(document.body.textContent).not.toContain("上下文未延续");
  });

  it("renders a required 项目 chip and keeps send disabled until a project is selected", async () => {
    const { fetcher } = createChatFetcher();
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        initialProjectId=""
        onConvertToTask={onConvertToTask}
        projects={[makeProject("project-1", "客户接入项目"), makeProject("project-2", "生产巡检项目")]}
      />,
    );

    // 参与门禁：未选项目时员工下拉只出占位，不出任何候选员工
    await waitFor(() => expect(getByText("请先选择项目")).toBeTruthy());
    expect(queryByText("Ada · 客服助手")).toBeNull();
    expect(getByLabelText("项目")).toBeTruthy();

    await typeInLabeledField("对话问题", "第一个问题");
    expect(getButton("发送").disabled).toBe(true);

    // selecting a project arms the anchor and loads its member employees; send
    // stays gated until the anchor's conversation restore settles (empty here)
    await clickButton("生产巡检项目");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());
    // 员工自动选中后锚点会话恢复需要再走一轮查询才能落定
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(getButton("发送").disabled).toBe(false));
  });

  it("clears the thread when the project changes mid-conversation, and the next send has no resume_of_run_id", async () => {
    const { fetcher, setRunScript } = createChatFetcher();
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject("project-1", "客户接入项目"), makeProject("project-2", "生产巡检项目")]}
      />,
    );

    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    setRunScript("run-1", [{ status: "completed", result: { output: "第一轮回答" } }]);
    await typeInLabeledField("对话问题", "第一个问题");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("第一轮回答"));

    await clickButton("生产巡检项目");

    // switching the project anchor clears the prior Q/A, same as switching employee
    expect(chatThread().textContent).not.toContain("第一个问题");
    expect(chatThread().textContent).not.toContain("第一轮回答");

    // 参与门禁：新锚点项目的成员列表加载完成后员工重新可选
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());

    setRunScript("run-2", [{ status: "completed", result: { output: "第二轮回答" } }]);
    await typeInLabeledField("对话问题", "第二个问题");
    await clickButton("发送");

    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies[1]).toEqual({
        objective: "第二个问题",
        run_kind: "chat",
        project_id: "project-2",
      });
    });
  });

  it("restores the anchor's latest conversation on mount and resumes it on follow-up", async () => {
    const { fetcher } = createRestoreFetcher([
      {
        chat_thread_id: "run-a",
        id: "run-a",
        result: { output: "历史回答一" },
        status: "completed",
        task_title: "历史问题一",
      },
      {
        chat_thread_id: "run-a",
        id: "run-b",
        resume_of_run_id: "run-a",
        result: { output: "历史回答二" },
        status: "completed",
        task_title: "历史问题二",
      },
    ]);
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());
    await act(async () => {
      await queryClient.refetchQueries();
    });

    // both turns come back from the server, oldest first
    await waitFor(() => {
      const text = chatThread().textContent ?? "";
      expect(text).toContain("历史问题一");
      expect(text).toContain("历史回答一");
      expect(text).toContain("历史问题二");
      expect(text).toContain("历史回答二");
      expect(text.indexOf("历史问题一")).toBeLessThan(text.indexOf("历史问题二"));
    });

    // a follow-up resumes the restored conversation's last completed turn
    await typeInLabeledField("对话问题", "恢复后的追问");
    await clickButton("发送");
    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies[0]).toEqual({
        objective: "恢复后的追问",
        run_kind: "chat",
        project_id: "project-1",
        resume_of_run_id: "run-b",
      });
    });
  });

  it("renders an expired-content placeholder for a restored completed turn whose result was cleared", async () => {
    const { fetcher } = createRestoreFetcher([
      {
        chat_thread_id: "run-a",
        id: "run-a",
        result: {},
        status: "completed",
        task_title: "被清理的历史问题",
      },
    ]);
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());
    await act(async () => {
      await queryClient.refetchQueries();
    });

    await waitFor(() => {
      const text = chatThread().textContent ?? "";
      expect(text).toContain("被清理的历史问题");
      expect(text).toContain("（内容已过期或无结果）");
    });
  });

  it("starts a fresh conversation via 新对话: clears the restored thread and the next send carries no resume_of_run_id", async () => {
    const { fetcher } = createRestoreFetcher([
      {
        chat_thread_id: "run-a",
        id: "run-a",
        result: { output: "历史回答一" },
        status: "completed",
        task_title: "历史问题一",
      },
    ]);
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("历史问题一"));

    await clickButton("新对话");
    expect(chatThread().textContent).not.toContain("历史问题一");

    await typeInLabeledField("对话问题", "全新会话的问题");
    await clickButton("发送");
    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies[0]).toEqual({
        objective: "全新会话的问题",
        run_kind: "chat",
        project_id: "project-1",
      });
    });
  });

  it("resumes polling for a restored in-flight run until it completes", async () => {
    const { fetcher } = createRestoreFetcher([
      {
        chat_thread_id: "run-a",
        id: "run-a",
        status: "running",
        task_title: "离开前发出的问题",
      },
    ]);
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada · 客服助手")).toBeTruthy());
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("离开前发出的问题"));
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("轮询回答-run-a"));
  });
});

function chatThread() {
  const element = document.querySelector<HTMLElement>('[data-testid="chat-thread"]');
  if (!element) {
    throw new Error("Unable to find chat-thread");
  }
  return element;
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

function getByLabelText(label: string) {
  const element = document.querySelector<HTMLElement>(`[aria-label="${label}"]`);
  if (!element) {
    throw new Error(`Unable to find label: ${label}`);
  }
  return element;
}

function postBodies(fetcher: ReturnType<typeof createChatFetcher>["fetcher"], path: string) {
  return fetcher.mock.calls
    .filter(
      ([url, init]) =>
        new URL(String(url)).pathname === path &&
        ((init as RequestInit | undefined)?.method ?? "GET") === "POST",
    )
    .map(([, init]) => JSON.parse(String((init as RequestInit | undefined)?.body)) as Record<string, unknown>);
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
    (item) => item.textContent?.trim() === name,
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
