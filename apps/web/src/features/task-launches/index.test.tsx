import { type ReactNode } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskLaunchView } from "@/features/task-launches";
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
    human_owner_user_id: "owner-1",
    id,
    name: id === "project-1" ? "客户接入项目" : "归档项目",
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
  multipleReviewers = false,
}: {
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
  const members = [
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

  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (
      url.pathname === "/api/v1/projects" &&
      url.searchParams.get("limit") === "50" &&
      url.searchParams.get("offset") === "0" &&
      method === "GET"
    ) {
      return jsonResponse([makeProject("archived-project", "archived"), makeProject()]);
    }
    if (url.pathname === "/api/v1/projects/project-1/members" && method === "GET") {
      return jsonResponse(members);
    }
    if (url.pathname === "/api/v1/projects/project-1/demands" && method === "POST") {
      return jsonResponse(submittedDemand, 201);
    }

    return jsonResponse({ message: `Unhandled ${method} ${url.pathname}` }, 404);
  });
}

async function renderWithQueryClient(children: ReactNode) {
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  mountedRoots.push(root);

  await act(async () => {
    root.render(
      <QueryClientProvider client={createQueryClient()}>{children}</QueryClientProvider>,
    );
  });

  return { container, root };
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
      root.unmount();
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
    await clickButton("发起任务");

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project-1/demands",
      expect.objectContaining({
        body: JSON.stringify({
          title: "审查这个开源项目的 PR，并按数量分配数字员工",
          content: "审查这个开源项目的 PR，并按数量分配数字员工",
          source_type: "manual",
          source_refs: {},
          attachments: [],
          reviewer_user_id: "reviewer-1",
          reviewer_selection_reason: "project_reviewer_default",
        }),
      }),
    );
    await vi.waitFor(() => {
      expect(mocks.navigate).toHaveBeenCalledWith({
        params: { demandId: "demand-1" },
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
    await clickButton("发起任务");

    await vi.waitFor(() => expect(getByText("请选择审核人")).toBeTruthy());
  });
});

function getByText(text: string) {
  const element = Array.from(document.body.querySelectorAll<HTMLElement>("*")).find(
    (item) =>
      item.textContent === text &&
      Array.from(item.children).every((child) => child.textContent !== text),
  );
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
