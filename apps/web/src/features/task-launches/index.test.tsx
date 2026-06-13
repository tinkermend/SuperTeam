import { type ReactNode } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskLaunchDetailView, TaskLaunchView } from "@/features/task-launches";
import {
  resolveDefaultReviewer,
  type ReviewerDefaultResolution,
} from "@/features/task-launches/components/task-launch-form";
import type { Project, ProjectMember } from "@/lib/api/projects";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
}));
const mountedRoots: Root[] = [];

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

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    params,
    to,
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
}));

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

function makeProject(id = "project-1", status: Project["status"] = "running"): Project {
  return {
    approval_policy: {},
    coordination_policy: {},
    coordination_status: "registered",
    coordination_workflow_id: `project-coordinator:${id}`,
    evidence_policy: {},
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
  };
}

function makeMember(
  id: string,
  overrides: Partial<ProjectMember> = {},
): ProjectMember {
  return {
    display_name_snapshot: id,
    id,
    principal_id: id,
    principal_type: "human_user",
    project_id: "project-1",
    project_role: "reviewer",
    settings: {},
    status: "active",
    tenant_id: "tenant-1",
    ...overrides,
  };
}

function createTaskLaunchFetcher({
  includeSecondProject = false,
  launchDetail = false,
  multipleReviewers = false,
  emptyFacts = false,
}: {
  emptyFacts?: boolean;
  includeSecondProject?: boolean;
  launchDetail?: boolean;
  multipleReviewers?: boolean;
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
      selection_reason: "project_reviewer_default",
    },
    source_refs: {},
    source_type: "manual",
    status: "submitted",
    submitted_by_user_id: "owner-1",
    tenant_id: "tenant-1",
    title: "审查这个开源项目的 PR，并按数量分配数字员工",
  };
  const projectOneMembers = [
    makeMember("owner-member", {
      display_name_snapshot: "负责人",
      principal_id: "owner-1",
      project_role: "owner",
    }),
    makeMember("reviewer-member-1", {
      display_name_snapshot: "王审核",
      principal_id: "reviewer-1",
    }),
    makeMember("executor-member-1", {
      display_name_snapshot: "数字员工",
      principal_id: "employee-1",
      principal_type: "digital_employee",
      project_role: "reviewer",
    }),
    ...(multipleReviewers
      ? [
          makeMember("reviewer-member-2", {
            display_name_snapshot: "李审核",
            principal_id: "reviewer-2",
          }),
        ]
      : []),
  ];
  const projectTwoMembers = [
    makeMember("project-2-owner-member", {
      display_name_snapshot: "第二项目负责人",
      principal_id: "owner-2",
      project_id: "project-2",
      project_role: "owner",
    }),
    makeMember("project-2-reviewer-member", {
      display_name_snapshot: "赵审核",
      principal_id: "reviewer-2",
      project_id: "project-2",
    }),
  ];

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
    if (url.pathname === "/api/v1/projects/project-1/members" && method === "GET") {
      return jsonResponse(projectOneMembers);
    }
    if (url.pathname === "/api/v1/projects/project-2/members" && method === "GET") {
      return jsonResponse(projectTwoMembers);
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
            selection_reason: "project_reviewer_default",
          },
          title: body.title,
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

function makeLaunchDetail({
  demandId = "demand-1",
  emptyFacts = false,
  title = "审查 PR",
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
      selection_reason: "project_reviewer_default",
    },
    source_refs: {},
    source_type: "manual",
    status: "submitted",
    submitted_by_user_id: "owner-1",
    tenant_id: "tenant-1",
    title,
  };
  const empty = {
    coordination_jobs: [],
    decision_requests: [],
    project_tasks: [],
    route_decisions: [],
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
              workflow_id: "project-coordinator:project-1",
            },
          ],
          decision_requests: [
            {
              id: "decision-1",
              project_id: "project-1",
              status_snapshot: "pending",
              target_user_id: "reviewer-1",
              tenant_id: "tenant-1",
              title_snapshot: "确认路由",
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
              title: "整理审查清单",
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
              tenant_id: "tenant-1",
            },
          ],
        }),
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

async function rerenderWithQueryClient(
  root: Root,
  queryClient: QueryClient,
  children: ReactNode,
) {
  await act(async () => {
    root.render(<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>);
  });
}

function expectReviewerResolution(
  resolution: ReviewerDefaultResolution | undefined,
  expected: Partial<ReviewerDefaultResolution> & { principalId?: string },
) {
  expect(resolution?.reason).toBe(expected.reason);
  expect(resolution?.requiresChoice).toBe(expected.requiresChoice);
  expect(resolution?.member?.principal_id).toBe(expected.principalId);
}

describe("resolveDefaultReviewer", () => {
  it("uses the only active human reviewer as the project reviewer default", () => {
    const resolution = resolveDefaultReviewer(makeProject(), [
      makeMember("employee-1", {
        principal_type: "digital_employee",
        project_role: "reviewer",
      }),
      makeMember("reviewer-1", { principal_id: "reviewer-1" }),
    ]);

    expectReviewerResolution(resolution, {
      principalId: "reviewer-1",
      reason: "project_reviewer_default",
      requiresChoice: false,
    });
  });

  it("requires explicit user selection when multiple active human reviewers exist", () => {
    const resolution = resolveDefaultReviewer(makeProject(), [
      makeMember("reviewer-1", { principal_id: "reviewer-1" }),
      makeMember("reviewer-2", { principal_id: "reviewer-2" }),
    ]);

    expectReviewerResolution(resolution, {
      reason: "user_selected",
      requiresChoice: true,
    });
  });

  it("falls back to the active human owner when there is no human reviewer", () => {
    const resolution = resolveDefaultReviewer(makeProject(), [
      makeMember("employee-1", {
        principal_id: "employee-1",
        principal_type: "digital_employee",
        project_role: "reviewer",
      }),
      makeMember("owner-1", {
        principal_id: "owner-1",
        project_role: "owner",
      }),
    ]);

    expectReviewerResolution(resolution, {
      principalId: "owner-1",
      reason: "project_human_owner_fallback",
      requiresChoice: false,
    });
  });
});

describe("TaskLaunchView", () => {
  afterEach(() => {
    for (const root of mountedRoots.splice(0)) {
      act(() => {
        root.unmount();
      });
    }
    document.body.innerHTML = "";
  });

  it("defaults reviewer to the only active human reviewer and submits demand", async () => {
    mocks.navigate.mockClear();
    const fetcher = createTaskLaunchFetcher();
    await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await typeInLabeledField("需求描述", "审查这个开源项目的 PR，并按数量分配数字员工");

    await vi.waitFor(() => expect(getByText("王审核 · reviewer")).toBeTruthy());
    await clickButton("提交需求");

    expect(postBody(fetcher, "/api/v1/projects/project-1/demands")).toEqual({
      title: "审查这个开源项目的 PR，并按数量分配数字员工",
      content: "审查这个开源项目的 PR，并按数量分配数字员工",
      source_type: "manual",
      source_refs: {},
      attachments: [],
      reviewer_user_id: "reviewer-1",
      reviewer_selection_reason: "project_reviewer_default",
    });
    await vi.waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        params: { demandId: "demand-1" },
        to: "/task-launches/$demandId",
      });
    });
  });

  it("uses the selected project's reviewer after project changes", async () => {
    mocks.navigate.mockClear();
    const fetcher = createTaskLaunchFetcher({ includeSecondProject: true });
    await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await vi.waitFor(() => expect(getByText("生产巡检项目")).toBeTruthy());
    await clickButton("生产巡检项目");
    await typeInLabeledField("需求描述", "处理第二个项目的巡检问题");

    await vi.waitFor(() => expect(getByText("赵审核 · reviewer")).toBeTruthy());
    await clickButton("提交需求");

    expect(postBody(fetcher, "/api/v1/projects/project-2/demands")).toMatchObject({
      content: "处理第二个项目的巡检问题",
      reviewer_selection_reason: "project_reviewer_default",
      reviewer_user_id: "reviewer-2",
      title: "处理第二个项目的巡检问题",
    });
    await vi.waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        params: { demandId: "demand-2" },
        to: "/task-launches/$demandId",
      });
    });
  });

  it("requires explicit reviewer selection when the selected project has multiple active human reviewers", async () => {
    const fetcher = createTaskLaunchFetcher({ multipleReviewers: true });
    await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await typeInLabeledField("需求描述", "需要两位审核人项目确认");
    await vi.waitFor(() => expect(getByText("王审核 · reviewer")).toBeTruthy());
    await vi.waitFor(() => expect(getByText("李审核 · reviewer")).toBeTruthy());
    expect(textOrder("王审核 · reviewer")).toBeLessThan(textOrder("负责人 · owner"));
    await clickButton("提交需求");

    await vi.waitFor(() => expect(getByText("请选择审核人")).toBeTruthy());
  });

  it("renders the pre-submit launch composer without orchestration state controls", async () => {
    const fetcher = createTaskLaunchFetcher();
    await renderWithQueryClient(
      <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await vi.waitFor(() => expect(getByText("提交后会发生什么")).toBeTruthy());

    expect(getByText("01")).toBeTruthy();
    expect(getByText("写入项目需求")).toBeTruthy();
    expect(getByText("02")).toBeTruthy();
    expect(getByText("启动协调线程")).toBeTruthy();
    expect(getByText("03")).toBeTruthy();
    expect(getByText("生成编排决策")).toBeTruthy();
    expect(getByText("04")).toBeTruthy();
    expect(getByText("进入运行视图")).toBeTruthy();
    expect(getByText("提交前确认")).toBeTruthy();
    expect(getByText("保存草稿")).toBeTruthy();
    expect(document.querySelector('[data-testid="task-launch-parameters"]')).toBeTruthy();

    expect(queryByText("上下文边界")).toBeNull();
    expect(queryByText("备注")).toBeNull();
    expect(queryByText("待提交")).toBeNull();
    expect(queryByText("待生成")).toBeNull();
    expect(queryByText("已完成")).toBeNull();
    expect(queryByText("运行中")).toBeNull();
  });
});

describe("TaskLaunchDetailView", () => {
  afterEach(() => {
    for (const root of mountedRoots.splice(0)) {
      act(() => {
        root.unmount();
      });
    }
    document.body.innerHTML = "";
  });

  it("renders launch detail coordination facts", async () => {
    const fetcher = createTaskLaunchFetcher({ launchDetail: true });
    await renderWithQueryClient(
      <TaskLaunchDetailView
        apiBaseUrl="http://control-plane.local"
        demandId="demand-1"
        fetcher={fetcher}
      />,
    );

    await vi.waitFor(() => expect(getByText("审查 PR")).toBeTruthy());
    expect(getByText("协调 Job")).toBeTruthy();
    expect(getByText("按能力分派")).toBeTruthy();
    expect(getByText("整理审查清单")).toBeTruthy();
    expect(getByText("确认路由")).toBeTruthy();
  });

  it("shows waiting state when coordination facts are empty", async () => {
    const fetcher = createTaskLaunchFetcher({ emptyFacts: true, launchDetail: true });
    await renderWithQueryClient(
      <TaskLaunchDetailView
        apiBaseUrl="http://control-plane.local"
        demandId="demand-1"
        fetcher={fetcher}
      />,
    );

    await vi.waitFor(() => expect(getByText("等待项目协调线程处理")).toBeTruthy());
  });

  it("does not show the previous demand detail while switching demand ids", async () => {
    let resolveDemand2!: () => void;
    const demand2Ready = new Promise<void>((resolve) => {
      resolveDemand2 = resolve;
    });
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";

      if (
        url.pathname === "/api/v1/project-demands/demand-1/launch-detail" &&
        method === "GET"
      ) {
        return jsonResponse(
          makeLaunchDetail({ demandId: "demand-1", title: "旧需求" }),
        );
      }
      if (
        url.pathname === "/api/v1/project-demands/demand-2/launch-detail" &&
        method === "GET"
      ) {
        await demand2Ready;
        return jsonResponse(
          makeLaunchDetail({ demandId: "demand-2", title: "新需求" }),
        );
      }

      return jsonResponse({ message: `Unhandled ${method} ${url.pathname}` }, 404);
    });
    const { queryClient, root } = await renderWithQueryClient(
      <TaskLaunchDetailView
        apiBaseUrl="http://control-plane.local"
        demandId="demand-1"
        fetcher={fetcher}
      />,
    );

    await vi.waitFor(() => expect(getByText("旧需求")).toBeTruthy());
    await rerenderWithQueryClient(
      root,
      queryClient,
      <TaskLaunchDetailView
        apiBaseUrl="http://control-plane.local"
        demandId="demand-2"
        fetcher={fetcher}
      />,
    );

    await vi.waitFor(() => expect(getByText("正在加载发起详情")).toBeTruthy());
    expect(queryByText("旧需求")).toBeNull();

    await act(async () => {
      resolveDemand2();
    });
    await vi.waitFor(() => expect(getByText("新需求")).toBeTruthy());
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

function textOrder(text: string) {
  const element = getByText(text);
  return Array.from(document.body.querySelectorAll<HTMLElement>("*")).indexOf(element);
}

function postBody(fetcher: ReturnType<typeof createTaskLaunchFetcher>, path: string) {
  const call = fetcher.mock.calls.find(([url]) => {
    const parsed = new URL(String(url));
    return parsed.pathname === path;
  });
  expect(call).toBeDefined();
  return JSON.parse(String(call?.[1]?.body)) as Record<string, unknown>;
}

async function typeInLabeledField(label: string, value: string) {
  await vi.waitFor(() => expect(getByLabelText(label)).toBeTruthy());
  const input = getByLabelText(label) as HTMLInputElement | HTMLTextAreaElement;
  await act(async () => {
    setInputValue(input, value);
  });
}

async function clickButton(name: string) {
  const button = Array.from(document.body.querySelectorAll<HTMLButtonElement>("button")).find(
    (item) => item.textContent === name,
  );
  if (!button) {
    throw new Error(`Unable to find button: ${name}`);
  }
  await act(async () => {
    button.click();
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
