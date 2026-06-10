import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectsView } from "@/features/projects";
import type { Project } from "@/lib/api/projects";

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

vi.mock("@tanstack/react-router", () => {
  type MockLinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
    children: ReactNode;
    params?: Record<string, string>;
    to: string;
  };
  const Link = forwardRef<HTMLAnchorElement, MockLinkProps>(
    ({ children, params, to, ...props }, ref) => (
      <a
        {...props}
        href={
          params?.projectId
            ? to.replace("$projectId", encodeURIComponent(params.projectId))
            : to
        }
        ref={ref}
      >
        {children}
      </a>
    ),
  );
  Link.displayName = "MockRouterLink";

  return { Link };
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

function makeProject(id: string, name: string, status: Project["status"] = "running"): Project {
  return {
    approval_policy: { high_risk: "human" },
    coordination_policy: { cadence: "daily" },
    coordination_status: "registered",
    coordination_workflow_id: `project-coordinator:${id}`,
    evidence_policy: { archive: "required" },
    goal: `${name} 闭环目标`,
    human_owner_user_id: "human-owner-1",
    id,
    name,
    status,
    tenant_id: "tenant-1",
  };
}

function createProjectFetcher(options: { slowFilteredList?: boolean } = {}) {
  const projects = [
    makeProject("project-1", "客户接入验收"),
    makeProject("project-2", "生产巡检整改"),
  ];

  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/projects" && method === "GET") {
      const q = url.searchParams.get("q") ?? "";
      if (q && options.slowFilteredList) {
        await new Promise((resolve) => setTimeout(resolve, 120));
      }
      return jsonResponse(
        q
          ? projects.filter((project) => project.name.includes(q) || project.goal.includes(q))
          : projects,
      );
    }

    if (url.pathname === "/api/v1/projects/project-1/overview" && method === "GET") {
      return jsonResponse({
        active_tasks: [
          {
            id: "task-1",
            project_id: "project-1",
            requires_human_approval: true,
            status: "running",
            tenant_id: "tenant-1",
            title: "整理接入证据",
          },
        ],
        coordination_workflow: {
          status: "registered",
          workflow_id: "project-coordinator:project-1",
        },
        digital_employee_pool: [
          {
            id: "member-2",
            principal_id: "de-1",
            principal_type: "digital_employee",
            project_id: "project-1",
            project_role: "executor",
            settings: {},
            status: "active",
            tenant_id: "tenant-1",
          },
        ],
        human_roles: [
          {
            id: "member-1",
            principal_id: "human-owner-1",
            principal_type: "human_user",
            project_id: "project-1",
            project_role: "owner",
            settings: {},
            status: "active",
            tenant_id: "tenant-1",
          },
        ],
        project: projects[0],
        recent_events: [
          {
            actor_id: "human-owner-1",
            actor_type: "human_user",
            event_type: "project.created",
            id: "event-1",
            payload: {},
            project_id: "project-1",
            resource_id: "project-1",
            resource_type: "project",
            sequence_number: 1,
            summary: "项目已创建",
            tenant_id: "tenant-1",
          },
        ],
        status_summary: { current_phase: "running", is_archived: false },
        task_summary: {
          active_tasks: 1,
          completed_tasks: 0,
          failed_tasks: 0,
          pending_human_tasks: 1,
        },
      });
    }

    if (url.pathname.endsWith("/overview") && method === "GET") {
      const id = url.pathname.split("/")[4];
      return jsonResponse({
        active_tasks: [],
        coordination_workflow: {
          status: "registered",
          workflow_id: `project-coordinator:${id}`,
        },
        digital_employee_pool: [],
        human_roles: [],
        project: projects.find((project) => project.id === id) ?? projects[0],
        recent_events: [],
        status_summary: { current_phase: "running", is_archived: false },
        task_summary: {
          active_tasks: 0,
          completed_tasks: 0,
          failed_tasks: 0,
          pending_human_tasks: 0,
        },
      });
    }

    if (url.pathname.endsWith("/tasks") && method === "GET") {
      return jsonResponse([]);
    }
    if (url.pathname.endsWith("/events") && method === "GET") {
      return jsonResponse([]);
    }
    if (url.pathname === "/api/v1/projects/project-1/demands" && method === "GET") {
      return jsonResponse([
        {
          attachments: [],
          id: "demand-1",
          project_id: "project-1",
          source_refs: {},
          source_type: "manual",
          status: "submitted",
          submitted_by_user_id: "human-owner-1",
          tenant_id: "tenant-1",
          title: "补充上线验收说明",
        },
      ]);
    }
    if (url.pathname.endsWith("/demands") && method === "GET") {
      return jsonResponse([]);
    }
    if (url.pathname === "/api/v1/projects/project-1/demands" && method === "POST") {
      return jsonResponse({
        attachments: ["s3://evidence/report.md"],
        id: "demand-2",
        project_id: "project-1",
        source_refs: { ticket: "SUP-42" },
        source_type: "ticket",
        status: "submitted",
        submitted_by_user_id: "human-owner-1",
        tenant_id: "tenant-1",
        title: "补充回归证据",
      });
    }
    if (url.pathname === "/api/v1/projects/project-1/archive" && method === "POST") {
      return jsonResponse({ ...projects[0], status: "archived" });
    }

    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 500);
  });
}

function fetchCalls(fetcher: typeof fetch) {
  return (
    fetcher as unknown as {
      mock: { calls: [RequestInfo | URL, RequestInit | undefined][] };
    }
  ).mock.calls;
}

async function renderProjects(fetcher: typeof fetch, routeProjectId?: string) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <ProjectsView
        apiBaseUrl="http://control-plane.test"
        fetcher={fetcher}
        routeProjectId={routeProjectId}
      />
    </QueryClientProvider>,
  );
}

describe("ProjectsView", () => {
  it("renders the project list and selected overview", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();
    await expect.element(screen.getByText("项目已创建")).toBeInTheDocument();
    await expect.element(screen.getByText("整理接入证据")).toBeInTheDocument();
    await expect.element(screen.getByText("补充上线验收说明")).toBeInTheDocument();
  });

  it("submits a demand to the current project", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");

    await userEvent.click(screen.getByRole("button", { name: "提交需求" }));
    await userEvent.fill(screen.getByLabelText("需求标题"), "补充回归证据");
    await userEvent.fill(screen.getByLabelText("来源引用 JSON"), '{"ticket":"SUP-42"}');
    await userEvent.fill(screen.getByLabelText("附件引用"), "s3://evidence/report.md");
    await userEvent.click(screen.getByRole("button", { name: "提交" }));

    await vi.waitFor(() => {
      const postCall = fetchCalls(fetcher).find(([url, init]) => {
        return (
          String(url).endsWith("/api/v1/projects/project-1/demands") &&
          init?.method === "POST"
        );
      });
      expect(postCall).toBeTruthy();
      expect(JSON.parse(String(postCall?.[1]?.body))).toMatchObject({
        attachments: ["s3://evidence/report.md"],
        source_refs: { ticket: "SUP-42" },
        title: "补充回归证据",
      });
    });
  });

  it("keeps previous list content visible while a filter request is refreshing", async () => {
    const fetcher = createProjectFetcher({ slowFilteredList: true });
    const screen = await renderProjects(fetcher);

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();
    await userEvent.fill(screen.getByLabelText("搜索项目"), "巡检");

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();
    await expect
      .element(screen.getByRole("heading", { name: "生产巡检整改" }))
      .toBeInTheDocument();
  });

  it("posts to the archive route", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");

    await userEvent.click(screen.getByRole("button", { name: "归档" }));

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith("/api/v1/projects/project-1/archive") &&
            init?.method === "POST"
          );
        }),
      ).toBe(true);
    });
  });
});
