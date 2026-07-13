import { type ReactNode } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChatPanel } from "@/features/task-launches/components/chat-panel";
import type { DigitalEmployee, DigitalEmployeeRun } from "@/lib/api/employees";

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
    approval_policy: {},
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

    const createMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
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

    const createMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
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

    const createMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
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

  it("lists mock employees by name and role, sends a first question without resume_of_run_id, renders the completed answer, sends a follow-up with resume_of_run_id, converts to a task draft, and retries a failed run without resume_of_run_id", async () => {
    const { fetcher, setRunScript } = createChatFetcher();
    const onConvertToTask = vi.fn();
    const { queryClient } = await renderWithQueryClient(
      <ChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
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
      expect(body).toEqual({ objective: "第一个问题", run_kind: "chat" });
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
        resume_of_run_id: "run-1",
      });
    });

    // 5. convert first (completed) answer to a task draft
    await clickButton("转为任务");
    expect(onConvertToTask).toHaveBeenCalledTimes(1);
    const payload = onConvertToTask.mock.calls[0][0] as {
      draft: string;
      chatRunId: string;
      digitalEmployeeId: string;
    };
    expect(payload.draft).toContain("第一个问题");
    expect(payload.draft).toContain("这是第一轮的回答内容");
    expect(payload.chatRunId).toBe("run-1");
    expect(payload.digitalEmployeeId).toBe("emp-1");

    // 6. second run fails -> error card + retry; retry resends without resume_of_run_id
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("对话执行失败，请重试"));

    await clickButton("重试");

    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies).toHaveLength(3);
      expect(bodies[2]).toEqual({ objective: "第二个问题", run_kind: "chat" });
    });
  });

  it("keeps the typed question in the textarea and re-enables send after a failed create-run request", async () => {
    const { fetcher } = createFailingSendFetcher();
    const onConvertToTask = vi.fn();
    await renderWithQueryClient(
      <ChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
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
      <ChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
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
    .filter(([url]) => new URL(String(url)).pathname === path)
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
