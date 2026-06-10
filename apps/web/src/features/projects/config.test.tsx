import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectConfigView } from "@/features/projects/components/project-config-page";
import type { ProjectConfig } from "@/lib/api/projects";

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

function makeConfig(status: "running" | "archived" = "running"): ProjectConfig {
  const project = {
    approval_policy: { high_risk: "human" },
    coordination_policy: { cadence: "daily" },
    coordination_status: "registered",
    coordination_workflow_id: "project-coordinator:project-1",
    description: "配置说明",
    evidence_policy: { retention_days: 90 },
    goal: "完成客户接入验收",
    human_owner_user_id: "human-owner-1",
    id: "project-1",
    name: "客户接入验收",
    status,
    tenant_id: "tenant-1",
  } as const;
  const humanMember = {
    display_name_snapshot: "负责人甲",
    id: "member-1",
    principal_id: "human-owner-1",
    principal_type: "human_user",
    project_id: "project-1",
    project_role: "owner",
    settings: {},
    status: "active",
    tenant_id: "tenant-1",
  } as const;
  const digitalMember = {
    display_name_snapshot: "验收执行员工",
    id: "member-2",
    principal_id: "de-1",
    principal_type: "digital_employee",
    project_id: "project-1",
    project_role: "executor",
    settings: { lane: "qa" },
    status: "active",
    tenant_id: "tenant-1",
  } as const;

  return {
    approval_policy: project.approval_policy,
    coordination_policy: project.coordination_policy,
    coordination_workflow: {
      status: "registered",
      workflow_id: "project-coordinator:project-1",
    },
    digital_employee_pool: [digitalMember],
    evidence_policy: project.evidence_policy,
    human_roles: [humanMember],
    members: [humanMember, digitalMember],
    project,
  };
}

function createConfigFetcher(status: "running" | "archived" = "running") {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/projects/project-1/config" && method === "GET") {
      return jsonResponse(makeConfig(status));
    }
    if (url.pathname === "/api/v1/projects/project-1/config" && method === "PUT") {
      return jsonResponse(makeConfig(status).project);
    }
    if (url.pathname === "/api/v1/projects/project-1/members" && method === "PUT") {
      return jsonResponse(makeConfig(status).members);
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

async function renderConfig(fetcher: typeof fetch) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <ProjectConfigView
        apiBaseUrl="http://control-plane.test"
        fetcher={fetcher}
        projectId="project-1"
      />
    </QueryClientProvider>,
  );
}

describe("ProjectConfigView", () => {
  it("renders config tabs and saves current project policy", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();
    await userEvent.click(screen.getByRole("tab", { name: "协调策略" }));
    await expect.element(screen.getByLabelText("协调策略 JSON")).toBeInTheDocument();
    await userEvent.fill(screen.getByLabelText("协调策略 JSON"), '{"cadence":"hourly"}');
    await userEvent.click(screen.getByRole("button", { name: "保存配置" }));

    await vi.waitFor(() => {
      const putCall = fetchCalls(fetcher).find(([url, init]) => {
        return (
          String(url).endsWith("/api/v1/projects/project-1/config") &&
          init?.method === "PUT"
        );
      });
      expect(putCall).toBeTruthy();
      expect(JSON.parse(String(putCall?.[1]?.body))).toMatchObject({
        coordination_policy: { cadence: "hourly" },
      });
    });
  });

  it("replaces members from the members tab", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await userEvent.fill(
      screen.getByLabelText("成员 JSON"),
      JSON.stringify([
        {
          principal_id: "human-owner-1",
          principal_type: "human_user",
          project_role: "owner",
        },
      ]),
    );
    await userEvent.click(screen.getByRole("button", { name: "保存成员池" }));

    await vi.waitFor(() => {
      const putCall = fetchCalls(fetcher).find(([url, init]) => {
        return (
          String(url).endsWith("/api/v1/projects/project-1/members") &&
          init?.method === "PUT"
        );
      });
      expect(putCall).toBeTruthy();
      expect(JSON.parse(String(putCall?.[1]?.body))).toMatchObject({
        members: [
          {
            principal_id: "human-owner-1",
            principal_type: "human_user",
            project_role: "owner",
          },
        ],
      });
    });
  });

  it("disables saves for archived projects", async () => {
    const fetcher = createConfigFetcher("archived");
    const screen = await renderConfig(fetcher);

    await expect.element(screen.getByText("项目已归档")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "保存配置" }))
      .toBeDisabled();
    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await expect
      .element(screen.getByRole("button", { name: "保存成员池" }))
      .toBeDisabled();
  });
});
