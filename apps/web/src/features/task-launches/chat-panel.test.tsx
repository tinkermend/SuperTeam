import { type ReactNode, useState } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ChatPanel,
  chatTurnErrorTitle,
  durationSecFromRun,
  formatChatDuration,
  isJsonDumpAnswer,
  mergeChatThread,
  type ChatEntry,
  type ChatPanelProps,
  type ConvertToTaskPayload
} from "@/features/task-launches/components/chat-panel";
import { CHAT_HISTORY_WINDOW_SIZE, sliceChatHistoryWindow } from "@/features/task-launches/components/chat-history-window";
import { buildChatLiveItems, chatLiveStatusLine } from "@/features/task-launches/components/chat-live-process";
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
  projectsLoading = false,
}: {
  apiOptions: ChatPanelProps["apiOptions"];
  initialProjectId?: string;
  onConvertToTask: (payload: ConvertToTaskPayload) => void;
  onProjectChange?: (projectId: string) => void;
  projects: Project[];
  projectsLoading?: boolean;
}) {
  const [projectId, setProjectId] = useState(initialProjectId);
  const resolvedProject =
    projects.find((project) => project.id === projectId) ?? null;
  return (
    <ChatPanel
      apiOptions={apiOptions}
      onConvertToTask={onConvertToTask}
      onProjectChange={(project) => {
        setProjectId(project.id);
        onProjectChange?.(project.id);
      }}
      projectId={projectId}
      projects={projects}
      projectsLoading={projectsLoading}
      resolvedProject={resolvedProject}
    />
  );
}

function makeProject(id = "project-1", name = "客户接入项目"): Project {
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

function makeEmployee(id = "emp-1", name = "Ada"): DigitalEmployee {
  return {
    description: name === "Ada" ? "处理客户工单与常见问题解答" : "发布与本地提交",
    employee_type: "generalist",
    id,
    name,
    owner_user_id: "owner-1",
    permission_policy: {},
    provider_type: "claude_code",
    risk_level: "low",
    role: name === "Ada" ? "客服助手" : "发布工程师",
    status: "active",
    tenant_id: "tenant-1"
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
    work_products: []
};
}

function emptyRunListResponse() {
  return jsonResponse({
    filters: { projects: [], statuses: [] },
    items: [],
    total_count: 0
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
      status: "active"
})),
  );
}

/** P1 Chat rail: skill bindings + project detail (git). Empty by default. */
function chatAuxRoutesResponse(
  path: string,
  method: string,
  skillBindings: Array<{ skill_id: string; skill?: { id: string; name: string; slug: string } }> = [],
  project: Project = makeProject(),
  eventsByRunId: Map<string, unknown[]> = new Map(),
): Response | null {
  const bindingsMatch = path.match(/^\/api\/v1\/projects\/([^/]+)\/skill-bindings$/);
  if (bindingsMatch && method === "GET") {
    return jsonResponse(
      skillBindings.map((row, index) => ({
        id: `binding-${index + 1}`,
        tenant_id: "tenant-1",
        project_id: bindingsMatch[1],
        skill_id: row.skill_id,
        skill: row.skill ?? {
          id: row.skill_id,
          name: `Skill ${row.skill_id}`,
          slug: row.skill_id,
        },
      })),
    );
  }
  const projectMatch = path.match(/^\/api\/v1\/projects\/([^/]+)$/);
  if (projectMatch && method === "GET") {
    return jsonResponse({ ...project, id: projectMatch[1] });
  }
  if (path === "/api/auth/me" && method === "GET") {
    return jsonResponse({
      user: { id: "owner-1", username: "owner", display_name: "负责人", tenant_id: "tenant-1" },
    });
  }
  const eventsMatch = path.match(/^\/api\/v1\/digital-employees\/[^/]+\/runs\/([^/]+)\/events$/);
  if (eventsMatch && method === "GET") {
    return jsonResponse(eventsByRunId.get(eventsMatch[1]) ?? []);
  }
  const stopMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs\/([^/]+)\/stop$/);
  if (stopMatch && method === "POST") {
    return jsonResponse({
      ...baseRunFields(stopMatch[2], stopMatch[1]),
      status: "cancelling",
    });
  }
  return null;
}

function createChatFetcher(
  skillBindings: Array<{ skill_id: string; skill?: { id: string; name: string; slug: string } }> = [],
  project: Project = makeProject(),
) {
  const employees = [makeEmployee()];
  const runScripts = new Map<string, Array<Partial<DigitalEmployeeRun>>>();
  const runGetCallCounts = new Map<string, number>();
  const eventsByRunId = new Map<string, unknown[]>();
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
    const auxResponse = chatAuxRoutesResponse(path, method, skillBindings, project, eventsByRunId);
    if (auxResponse) {
      return auxResponse;
    }
    if (path.match(/^\/api\/v1\/digital-employees\/[^/]+\/chat-threads$/) && method === "GET") {
      return jsonResponse({ items: [] });
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
          chat_thread_id: body.resume_of_run_id ?? runId,
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
        ...step
});
    }

    return jsonResponse({ message: `Unhandled ${method} ${path}` }, 404);
  });

  return {
    fetcher,
    setRunEvents: (runId: string, events: unknown[]) => eventsByRunId.set(runId, events),
    setRunScript: (runId: string, script: Array<Partial<DigitalEmployeeRun>>) =>
      runScripts.set(runId, script)
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
    const auxResponse = chatAuxRoutesResponse(path, method);
    if (auxResponse) {
      return auxResponse;
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
    const auxResponse = chatAuxRoutesResponse(path, method);
    if (auxResponse) {
      return auxResponse;
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
            error_message: "第一次失败"
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
        error_message: "第一次失败"
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
      )
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
    const auxResponse = chatAuxRoutesResponse(path, method);
    if (auxResponse) {
      return auxResponse;
    }

    if (path.match(/^\/api\/v1\/digital-employees\/[^/]+\/chat-threads$/) && method === "GET") {
      return jsonResponse({ items: [] });
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
          {
            ...baseRunFields("run-1", employeeId),
            chat_thread_id: "run-1",
            status: "queued",
          },
          201,
        );
      }
      if (createCallCount === 2 && body.resume_of_run_id) {
        return jsonResponse({ message: "会话已失效，无法继续上下文" }, 400);
      }
      return jsonResponse(
        {
          ...baseRunFields("run-2", employeeId),
          chat_thread_id: "run-1",
          status: "queued",
        },
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
        result: { output: `回答-${runId}` }
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
    const auxResponse = chatAuxRoutesResponse(path, method);
    if (auxResponse) {
      return auxResponse;
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
        result: { output: "首轮回答" }
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
    const auxResponse = chatAuxRoutesResponse(path, method);
    if (auxResponse) {
      return auxResponse;
    }

    if (path.match(/^\/api\/v1\/digital-employees\/[^/]+\/chat-threads$/) && method === "GET") {
      const root = threadItemsAsc[0];
      if (!root) {
        return jsonResponse({ items: [] });
      }
      const last = threadItemsAsc[threadItemsAsc.length - 1]!;
      return jsonResponse({
        items: [
          {
            chat_thread_id: String(root.chat_thread_id ?? root.id),
            title: root.task_title,
            initiator_user_id: "owner-1",
            initiator_display_name: "负责人",
            last_speaker_user_id: "owner-1",
            last_speaker_display_name: "负责人",
            last_prompt: last.task_title,
            last_active_at: "2026-08-13T00:00:00Z",
            has_active_run: threadItemsAsc.some((item) =>
              ["queued", "dispatching", "running", "cancelling"].includes(String(item.status ?? "")),
            ),
          },
        ],
      });
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
        total_count: desc.length
});
    }
    if (runsMatch && method === "POST") {
      runCounter += 1;
      const body = JSON.parse(String(init?.body)) as { resume_of_run_id?: string };
      return jsonResponse(
        {
          ...baseRunFields(`run-${runCounter}`, runsMatch[1]),
          status: "queued",
          ...(body.resume_of_run_id ? { resume_of_run_id: body.resume_of_run_id } : {})
},
        201,
      );
    }

    const getMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs\/([^/]+)$/);
    if (getMatch && method === "GET") {
      return jsonResponse({
        ...baseRunFields(getMatch[2], getMatch[1]),
        status: "completed",
        result: { output: `轮询回答-${getMatch[2]}` }
});
    }

    return jsonResponse({ message: `Unhandled ${method} ${path}` }, 404);
  });

  return { fetcher };
}

function createTwoEmployeeThreadFetcher() {
  const employees = [makeEmployee("emp-1", "Ada"), makeEmployee("emp-2", "Bob")];
  const threads = new Map<string, RestoreThreadItem[]>([
    [
      "emp-1",
      [
        {
          chat_thread_id: "thread-a",
          id: "run-a",
          result: { output: "第一轮回答" },
          status: "completed",
          task_title: "第一轮问题",
        },
      ],
    ],
    ["emp-2", []],
  ]);
  const runScripts = new Map<string, Array<Partial<DigitalEmployeeRun>>>();
  const runGetCallCounts = new Map<string, number>();
  let runCounter = 200;

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
    const auxResponse = chatAuxRoutesResponse(path, method);
    if (auxResponse) {
      return auxResponse;
    }

    const threadMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/chat-threads$/);
    if (threadMatch && method === "GET") {
      const items = threads.get(threadMatch[1]) ?? [];
      const root = items[0];
      if (!root) {
        return jsonResponse({ items: [] });
      }
      const last = items[items.length - 1]!;
      return jsonResponse({
        items: [
          {
            chat_thread_id: String(root.chat_thread_id ?? root.id),
            title: root.task_title,
            initiator_user_id: "owner-1",
            initiator_display_name: "负责人",
            last_speaker_user_id: "owner-1",
            last_speaker_display_name: "负责人",
            last_prompt: last.task_title,
            last_active_at: "2026-08-16T00:00:00Z",
            has_active_run: items.some((item) =>
              ["queued", "dispatching", "running", "cancelling"].includes(String(item.status ?? "")),
            ),
          },
        ],
      });
    }

    const runsMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
    if (runsMatch && method === "GET") {
      const employeeId = runsMatch[1];
      const desc = [...(threads.get(employeeId) ?? [])]
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
      const employeeId = runsMatch[1];
      const body = JSON.parse(String(init?.body)) as {
        objective: string;
        resume_of_run_id?: string;
      };
      const existing = threads.get(employeeId) ?? [];
      const threadId = String(existing[0]?.chat_thread_id ?? `thread-${runCounter}`);
      const runId = `run-${runCounter}`;
      existing.push({
        chat_thread_id: threadId,
        id: runId,
        result: {},
        status: "queued",
        task_title: body.objective,
      });
      threads.set(employeeId, existing);
      return jsonResponse(
        {
          ...baseRunFields(runId, employeeId),
          chat_thread_id: threadId,
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
      const script = runScripts.get(runId) ?? [{ status: "completed", result: { output: `回答-${runId}` } }];
      const step = script[Math.min(callIndex, script.length - 1)] ?? script[script.length - 1];
      const stored = (threads.get(employeeId) ?? []).find((item) => item.id === runId);
      if (stored && step) {
        stored.status = (step.status as RestoreThreadItem["status"]) ?? stored.status;
        if (step.result) {
          stored.result = step.result as RestoreThreadItem["result"];
        }
      }
      return jsonResponse({
        ...baseRunFields(runId, employeeId),
        chat_thread_id: stored?.chat_thread_id,
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

describe("sliceChatHistoryWindow", () => {
  it("keeps a presentation window and reports hidden count", () => {
    const entries = Array.from({ length: CHAT_HISTORY_WINDOW_SIZE + 8 }, (_, index) => ({ id: String(index) }));
    const first = sliceChatHistoryWindow(entries, 0);
    expect(first.hidden).toBe(8);
    expect(first.visible).toHaveLength(CHAT_HISTORY_WINDOW_SIZE);
    expect(first.visible[0]).toEqual({ id: "8" });
    const next = sliceChatHistoryWindow(entries, CHAT_HISTORY_WINDOW_SIZE);
    expect(next.hidden).toBe(0);
    expect(next.visible).toHaveLength(entries.length);
  });
});

describe("buildChatLiveItems", () => {
  it("pairs tool start/complete and concatenates text deltas", () => {
    const { texts, tools } = buildChatLiveItems([
      { event_type: "text_delta", sequence_number: 1, payload: { text: "正在" } },
      {
        event_type: "tool_started",
        sequence_number: 2,
        payload: { tool_id: "t1", name: "Bash" },
      },
      { event_type: "text_delta", sequence_number: 3, payload: { text: "分析" } },
      {
        event_type: "tool_completed",
        sequence_number: 4,
        payload: { tool_id: "t1", is_error: false },
      },
    ]);
    expect(texts[0]?.text).toBe("正在分析");
    expect(tools).toEqual([expect.objectContaining({ name: "Bash", status: "ok" })]);
  });

  it("clips tool input/output excerpts", () => {
    const long = "x".repeat(900);
    const { tools } = buildChatLiveItems([
      {
        event_type: "tool_started",
        sequence_number: 1,
        payload: { tool_id: "t1", name: "Read", input_excerpt: long },
      },
      {
        event_type: "tool_completed",
        sequence_number: 2,
        payload: { tool_id: "t1", is_error: false, output_excerpt: "short out" },
      },
    ]);
    expect(tools[0]?.inputExcerpt?.endsWith("…")).toBe(true);
    expect(tools[0]?.inputExcerpt?.length).toBe(801);
    expect(tools[0]?.outputExcerpt).toBe("short out");
  });

  it("labels live status from tools and answer text, not a thinking row", () => {
    expect(chatLiveStatusLine([], "")).toBe("正在执行…");
    expect(
      chatLiveStatusLine([{ key: "t", name: "Bash", status: "running" }], ""),
    ).toBe("正在调用 Bash");
    expect(
      chatLiveStatusLine([{ key: "t", name: "Bash", status: "ok" }], "第一段"),
    ).toBe("正在写出回答…");
    expect(chatLiveStatusLine([{ key: "t", name: "Bash", status: "ok" }], "")).toBe(
      "正在整理回答…",
    );
  });
});

describe("mergeChatThread", () => {
  it("keeps later local turns when the server snapshot is stale", () => {
    const first: ChatEntry = { runId: "run-a", question: "q1", status: "completed", answer: "a1" };
    const second: ChatEntry = { runId: "run-b", question: "q2", status: "completed", answer: "a2" };
    expect(mergeChatThread([first], [first, second])).toEqual([first, second]);
  });

  it("appends a pending local turn after restored history instead of putting it first", () => {
    const first: ChatEntry = { runId: "run-a", question: "q1", status: "completed", answer: "a1" };
    const pending: ChatEntry = { runId: "pending:1", question: "新问题", status: "sending" };
    expect(mergeChatThread([first], [pending]).map((entry) => entry.runId)).toEqual([
      "run-a",
      "pending:1",
    ]);
  });

  it("prefers a locally completed overlay over a still-running server row", () => {
    const server: ChatEntry = { runId: "run-a", question: "q1", status: "running" };
    const local: ChatEntry = { runId: "run-a", question: "q1", status: "completed", answer: "done" };
    expect(mergeChatThread([server], [local])[0]).toMatchObject({ status: "completed", answer: "done" });
  });
});

describe("isJsonDumpAnswer", () => {
  it("accepts pretty-printed JSON objects and rejects markdown", () => {
    expect(isJsonDumpAnswer('{\n  "ok": true\n}')).toBe(true);
    expect(isJsonDumpAnswer("**加粗** 和一段说明")).toBe(false);
  });
});

describe("chat turn meta", () => {
  it("labels failures via failure_family and cancelled as stopped", () => {
    expect(chatTurnErrorTitle("cancelled")).toBe("对话已停止");
    expect(chatTurnErrorTitle("timed_out")).toBe("执行超时");
    expect(chatTurnErrorTitle("failed", "provider_configuration")).toBe("执行器配置有误");
    expect(chatTurnErrorTitle("failed")).toBe("对话失败");
  });

  it("computes duration from timestamps when duration_sec is absent", () => {
    expect(
      durationSecFromRun({
        started_at: "2026-08-16T03:00:00.000Z",
        completed_at: "2026-08-16T03:00:12.000Z",
      }),
    ).toBe(12);
    expect(formatChatDuration(12)).toBe("12秒");
    expect(formatChatDuration(72)).toBe("1分12秒");
  });
});

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

    // 1. employee roster lists mock employees with identity (avatar row: name/role),
    //    and the thread header surfaces the selected employee's role + description
    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    expect(getByText("客服助手")).toBeTruthy();
    const roster = document.querySelector('[aria-label="数字员工列表"]');
    expect(roster).toBeTruthy();
    const selectedRow = roster!.querySelector('button[aria-pressed="true"]');
    expect(selectedRow?.textContent).toContain("Ada");
    expect(document.body.textContent).toContain("处理客户工单与常见问题解答");

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
        project_id: "project-1"
});
    });

    // 3. running -> completed; answer renders inside chat-thread
    await waitFor(() => expect(chatThread().textContent).toContain("正在执行"));
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
        resume_of_run_id: "run-1"
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
        resume_of_run_id: "run-1"
});
    });
  });

  it("shows project skill chips and includes selected skill_ids on create-run", async () => {
    const skillId = "11111111-1111-1111-1111-111111111111";
    const { fetcher } = createChatFetcher([
      {
        skill_id: skillId,
        skill: { id: skillId, name: "调研助手", slug: "research" },
      },
    ]);
    await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(document.body.textContent).toContain("调研助手"));
    await waitFor(() => expect(document.body.textContent).toContain("将投影"));

    const chip = Array.from(document.body.querySelectorAll<HTMLButtonElement>("button")).find(
      (btn) => btn.textContent?.trim() === "调研助手",
    );
    expect(chip).toBeTruthy();
    await act(async () => {
      chip!.click();
    });
    expect(chip!.getAttribute("aria-pressed")).toBe("true");

    await typeInLabeledField("对话问题", "带技能提问");
    await clickButton("发送");

    await waitFor(() => expect(document.body.textContent).toContain("确认开跑"));
    await clickButton("确认开跑");

    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies).toHaveLength(1);
      expect(bodies[0]).toEqual({
        objective: "带技能提问",
        run_kind: "chat",
        project_id: "project-1",
        skill_ids: [skillId],
        interactive_confirmed: true,
      });
    });
  });

  it("opens SoftDialog when project autonomy_ceiling is pause_at_gate", async () => {
    const project = {
      ...makeProject(),
      coordination_policy: { autonomy_ceiling: "pause_at_gate" },
    };
    const { fetcher } = createChatFetcher([], project);
    await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[project]}
      />,
    );

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    await typeInLabeledField("对话问题", "ceiling 确认");
    await clickButton("发送");

    await waitFor(() => expect(document.body.textContent).toContain("确认开跑"));
    expect(document.body.textContent).toContain("遇闸暂停");
    expect(postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs")).toHaveLength(0);

    await clickButton("确认开跑");
    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies).toHaveLength(1);
      expect(bodies[0]).toEqual({
        objective: "ceiling 确认",
        run_kind: "chat",
        project_id: "project-1",
        interactive_confirmed: true,
      });
    });
  });

  it("recovers SoftDialog when create-run returns interactive confirm required", async () => {
    const employees = [makeEmployee()];
    let createCalls = 0;
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
      const auxResponse = chatAuxRoutesResponse(path, method);
      if (auxResponse) {
        return auxResponse;
      }
      const createMatch = path.match(/^\/api\/v1\/digital-employees\/([^/]+)\/runs$/);
      if (createMatch && method === "GET") {
        return emptyRunListResponse();
      }
      if (createMatch && method === "POST") {
        createCalls += 1;
        const body = JSON.parse(String(init?.body)) as { interactive_confirmed?: boolean };
        if (!body.interactive_confirmed) {
          return jsonResponse(
            {
              message:
                "invalid employee input: interactive light confirm required (set interactive_confirmed=true after user acknowledgment)",
            },
            400,
          );
        }
        return jsonResponse(
          { ...baseRunFields("run-confirm", createMatch[1]), status: "queued" },
          201,
        );
      }
      return jsonResponse({ message: `Unhandled ${method} ${path}` }, 404);
    });

    await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    await typeInLabeledField("对话问题", "服务端要求确认");
    await clickButton("发送");

    await waitFor(() => expect(document.body.textContent).toContain("确认开跑"));
    expect(createCalls).toBe(1);
    expect(document.body.textContent).not.toContain("interactive light confirm required");

    await clickButton("确认开跑");
    await waitFor(() => {
      expect(createCalls).toBe(2);
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies[1]).toMatchObject({
        objective: "服务端要求确认",
        interactive_confirmed: true,
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

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());

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

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());

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

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());

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
      resume_of_run_id: "run-1"
});
    expect(bodies[2]).toEqual({
      objective: "第二个问题",
      run_kind: "chat",
      project_id: "project-1",
      chat_thread_id: "run-1",
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

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());

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
    expect(queryByText("Ada")).toBeNull();
    expect(getByLabelText("项目")).toBeTruthy();

    await typeInLabeledField("对话问题", "第一个问题");
    expect(getButton("发送").disabled).toBe(true);

    // selecting a project arms the anchor and loads its member employees; send
    // stays gated until the anchor's conversation restore settles (empty here)
    await clickButton("项目");
    await clickButton("生产巡检项目");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
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

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());

    setRunScript("run-1", [{ status: "completed", result: { output: "第一轮回答" } }]);
    await typeInLabeledField("对话问题", "第一个问题");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("第一轮回答"));

    await clickButton("项目");
    await clickButton("生产巡检项目");

    // switching the project anchor clears the prior Q/A, same as switching employee
    expect(chatThread().textContent).not.toContain("第一个问题");
    expect(chatThread().textContent).not.toContain("第一轮回答");

    // 参与门禁：新锚点项目的成员列表加载完成后员工重新可选
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(getByText("Ada")).toBeTruthy());

    setRunScript("run-2", [{ status: "completed", result: { output: "第二轮回答" } }]);
    await typeInLabeledField("对话问题", "第二个问题");
    await clickButton("发送");

    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies[1]).toEqual({
        objective: "第二个问题",
        run_kind: "chat",
        project_id: "project-2"
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
        task_title: "历史问题一"
},
      {
        chat_thread_id: "run-a",
        id: "run-b",
        resume_of_run_id: "run-a",
        result: { output: "历史回答二" },
        status: "completed",
        task_title: "历史问题二"
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

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
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
        resume_of_run_id: "run-b"
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
        task_title: "被清理的历史问题"
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

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
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
        task_title: "历史问题一"
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

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("历史问题一"));

    await clickButton("新会话");
    expect(chatThread().textContent).not.toContain("历史问题一");

    await typeInLabeledField("对话问题", "全新会话的问题");
    await clickButton("发送");
    await waitFor(() => expect(chatThread().textContent).toContain("全新会话的问题"));
    await waitFor(() => {
      const bodies = postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs");
      expect(bodies[0]).toEqual({
        objective: "全新会话的问题",
        run_kind: "chat",
        project_id: "project-1"
});
    });
    expect(chatThread().querySelector(".hub-human-avatar, .hub-human-avatar-fallback")).toBeTruthy();
  });

  it("resumes polling for a restored in-flight run until it completes", async () => {
    const { fetcher } = createRestoreFetcher([
      {
        chat_thread_id: "run-a",
        id: "run-a",
        status: "running",
        task_title: "离开前发出的问题"
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

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("离开前发出的问题"));
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("轮询回答-run-a"));
  });

  it("does not treat an empty project list as no-projects while the parent is still loading", async () => {
    const { fetcher } = createChatFetcher();
    await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[]}
        projectsLoading
      />,
    );

    expect(document.body.textContent).toContain("加载项目…");
    expect(document.body.textContent).not.toContain("还没有可用项目，无法提交任务。");
  });

  it("sends the current question with Enter and leaves Shift+Enter as a newline", async () => {
    const { fetcher, setRunScript } = createChatFetcher();
    const onConvertToTask = vi.fn();
    await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={onConvertToTask}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    setRunScript("run-1", [
      { status: "running" },
      { status: "completed", result: { output: "快捷键回答" } },
    ]);
    await typeInLabeledField("对话问题", "用快捷键发送");

    const textarea = getByLabelText("对话问题") as HTMLTextAreaElement;
    await act(async () => {
      textarea.dispatchEvent(
        new KeyboardEvent("keydown", {
          bubbles: true,
          cancelable: true,
          key: "Enter",
          shiftKey: true,
        }),
      );
    });
    expect(postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs")).toHaveLength(0);

    await act(async () => {
      textarea.dispatchEvent(
        new KeyboardEvent("keydown", {
          bubbles: true,
          cancelable: true,
          key: "Enter",
        }),
      );
    });
    await waitFor(() => {
      expect(postBodies(fetcher, "/api/v1/digital-employees/emp-1/runs")[0]).toEqual({
        objective: "用快捷键发送",
        run_kind: "chat",
        project_id: "project-1"
});
    });
  });

  it("keeps later Ada turns after switching to Bob and back", async () => {
    const { fetcher, setRunScript } = createTwoEmployeeThreadFetcher();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("第一轮问题"));

    setRunScript("run-201", [{ status: "completed", result: { output: "第二轮回答" } }]);
    await typeInLabeledField("对话问题", "第二轮问题");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("第二轮回答"));

    const roster = document.querySelector('[aria-label="数字员工列表"]');
    const bob = Array.from(roster?.querySelectorAll("button") ?? []).find((button) =>
      button.textContent?.includes("Bob"),
    );
    expect(bob).toBeTruthy();
    await act(async () => {
      bob!.click();
    });
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).not.toContain("第一轮问题"));

    const ada = Array.from(roster?.querySelectorAll("button") ?? []).find((button) =>
      button.textContent?.includes("Ada"),
    );
    await act(async () => {
      ada!.click();
    });
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => {
      const text = chatThread().textContent ?? "";
      expect(text).toContain("第一轮问题");
      expect(text).toContain("第一轮回答");
      expect(text).toContain("第二轮问题");
      expect(text).toContain("第二轮回答");
    });
  });

  it("resumes an in-flight Ada run after switching to Bob and back", async () => {
    const { fetcher, setRunScript } = createTwoEmployeeThreadFetcher();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("第一轮问题"));

    setRunScript("run-201", [
      { status: "running" },
      { status: "running" },
      { status: "completed", result: { output: "切回后完成" } },
    ]);
    await typeInLabeledField("对话问题", "进行中的问题");
    await clickButton("发送");
    await waitFor(() => expect(chatThread().textContent).toContain("进行中的问题"));

    const roster = document.querySelector('[aria-label="数字员工列表"]');
    const bob = Array.from(roster?.querySelectorAll("button") ?? []).find((button) =>
      button.textContent?.includes("Bob"),
    );
    await act(async () => {
      bob!.click();
    });
    await act(async () => {
      await queryClient.refetchQueries();
    });

    const ada = Array.from(roster?.querySelectorAll("button") ?? []).find((button) =>
      button.textContent?.includes("Ada"),
    );
    await act(async () => {
      ada!.click();
    });
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("进行中的问题"));
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("切回后完成"));
  });

  it("renders completed answers as markdown and JSON dumps as preformatted text", async () => {
    const { fetcher, setRunScript } = createChatFetcher();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[makeProject()]}
      />,
    );

    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    setRunScript("run-1", [
      { status: "completed", result: { output: "结论：**必须加粗**" } },
    ]);
    await typeInLabeledField("对话问题", "请用 markdown 回答");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => {
      expect(chatThread().querySelector("strong")?.textContent).toBe("必须加粗");
    });
  });

  it("shows Stop while a run is active and posts the stop endpoint", async () => {
    const { fetcher, setRunScript } = createChatFetcher();
    await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[makeProject()]}
      />,
    );
    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    setRunScript("run-1", [{ status: "running" }]);
    await typeInLabeledField("对话问题", "长跑问题");
    await clickButton("发送");
    await waitFor(() => expect(getButton("停止")).toBeTruthy());
    await clickButton("停止");
    await waitFor(() => {
      const stopCalls = fetcher.mock.calls.filter(
        ([url, init]) =>
          String(url).includes("/runs/run-1/stop") &&
          ((init as RequestInit | undefined)?.method ?? "GET") === "POST",
      );
      expect(stopCalls).toHaveLength(1);
      expect(JSON.parse(String((stopCalls[0][1] as RequestInit).body))).toEqual({
        reason: "用户停止对话",
      });
    });
  });

  it("labels a failed turn with failure_family and keeps the raw error as detail", async () => {
    const { fetcher, setRunScript } = createChatFetcher();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[makeProject()]}
      />,
    );
    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    setRunScript("run-1", [
      {
        status: "failed",
        error_family: "provider_configuration",
        error_message: "missing ANTHROPIC_API_KEY",
      },
    ]);
    await typeInLabeledField("对话问题", "会配错的问题");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => {
      expect(chatThread().textContent).toContain("执行器配置有误");
      expect(chatThread().textContent).toContain("missing ANTHROPIC_API_KEY");
    });
  });

  it("copies the completed answer and shows duration on the footer", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    const { fetcher, setRunScript } = createChatFetcher();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[makeProject()]}
      />,
    );
    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    setRunScript("run-1", [
      {
        status: "completed",
        result: { output: "可复制的回答" },
        started_at: "2026-08-16T03:00:00.000Z",
        completed_at: "2026-08-16T03:00:12.000Z",
      },
    ]);
    await typeInLabeledField("对话问题", "请给一段回答");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("12秒"));
    await clickButton("复制");
    await waitFor(() => expect(writeText).toHaveBeenCalledWith("可复制的回答"));
  });

  it("collapses completed tool calls into a process chip", async () => {
    const { fetcher, setRunEvents, setRunScript } = createChatFetcher();
    const { queryClient } = await renderWithQueryClient(
      <ControlledChatPanel
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
        onConvertToTask={vi.fn()}
        projects={[makeProject()]}
      />,
    );
    await waitFor(() => expect(getByText("Ada")).toBeTruthy());
    setRunEvents("run-1", [
      {
        event_type: "tool_started",
        sequence_number: 1,
        payload: { tool_id: "t1", name: "Bash" },
      },
      {
        event_type: "tool_completed",
        sequence_number: 2,
        payload: { tool_id: "t1", is_error: false },
      },
    ]);
    setRunScript("run-1", [
      { status: "running" },
      { status: "completed", result: { output: "做完了" } },
    ]);
    await typeInLabeledField("对话问题", "改一下工作区");
    await clickButton("发送");
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await act(async () => {
      await queryClient.refetchQueries();
    });
    await waitFor(() => expect(chatThread().textContent).toContain("工具 · 1"));
    expect(chatThread().textContent).not.toContain("Bash");
    await clickButton("工具 · 1");
    await waitFor(() => expect(chatThread().textContent).toContain("Bash"));
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
    (item) => item.textContent?.trim() === name || item.getAttribute("aria-label") === name,
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
