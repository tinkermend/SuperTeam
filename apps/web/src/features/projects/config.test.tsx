import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectConfigView } from "@/features/projects/components/project-config-page";
import type { ProjectConfig, ProjectConfigRevision } from "@/lib/api/projects";

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
    acceptance_user_id: "acceptance-user-1",
    human_owner_user_id: "human-owner-1",
    id: "project-1",
    leader_user_id: "leader-user-1",
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

function makeConfigRevisions(): ProjectConfigRevision[] {
  return [
    {
      changed_sections: ["coordination_policy"],
      change_summary: "提高协调频率",
      config_snapshot: {
        approval_policy: { high_risk: "human" },
        coordination_policy: { cadence: "continuous" },
        evidence_policy: { retention_days: 120 },
      },
      created_at: "2026-01-03T08:00:00Z",
      created_by_user_id: "human-owner-1",
      diff_summary: { coordination_policy: "changed" },
      id: "revision-3",
      policy_fingerprint: "policy-fingerprint-3",
      project_id: "project-1",
      revision_number: 3,
      tenant_id: "tenant-1",
      previous_revision_id: "revision-2",
    },
    {
      changed_sections: ["approvalPolicy", "evidence_policy"],
      change_summary: "补充审批和证据规则",
      config_snapshot: {
        project: {
          approvalPolicy: { highRisk: "human_review" },
          coordination_policy: { cadence: "hourly" },
          evidence_policy: { archive_mode: "locked", retention_days: 90 },
        },
      },
      created_at: "2026-01-02T08:00:00Z",
      created_by_user_id: "leader-user-1",
      diff_summary: { approvalPolicy: "changed", evidence_policy: "changed" },
      id: "revision-2",
      policy_fingerprint: "policy-fingerprint-2",
      project_id: "project-1",
      revision_number: 2,
      tenant_id: "tenant-1",
      previous_revision_id: "revision-1",
    },
    {
      changed_sections: [],
      change_summary: "初始配置",
      config_snapshot: {
        approval_policy: { high_risk: "human" },
        coordination_policy: { cadence: "daily" },
        evidence_policy: { retention_days: 30 },
      },
      created_at: "2026-01-01T08:00:00Z",
      created_by_user_id: "human-owner-1",
      diff_summary: {},
      id: "revision-1",
      project_id: "project-1",
      revision_number: 1,
      tenant_id: "tenant-1",
    },
  ];
}

function createConfigFetcher(
  status: "running" | "archived" = "running",
  configs: ProjectConfig[] = [makeConfig(status)],
  revisions: ProjectConfigRevision[] = makeConfigRevisions(),
) {
  let requestCount = 0;
  const latestConfig = () => configs[Math.min(requestCount, configs.length - 1)];
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/projects/project-1/config" && method === "GET") {
      const config = configs[Math.min(requestCount, configs.length - 1)];
      requestCount += 1;
      return jsonResponse(config);
    }
    if (url.pathname === "/api/v1/projects/project-1/config" && method === "PUT") {
      return jsonResponse(latestConfig().project);
    }
    if (url.pathname === "/api/v1/projects/project-1/members" && method === "PUT") {
      return jsonResponse(latestConfig().members);
    }
    if (url.pathname === "/api/v1/projects/project-1/tasks" && method === "GET") {
      return jsonResponse([
        {
          id: "task-history-1",
          project_id: "project-1",
          requires_human_approval: false,
          status: "completed",
          summary: "完成验收材料整理",
          tenant_id: "tenant-1",
          title: "整理历史任务",
        },
      ]);
    }
    if (
      url.pathname === "/api/v1/projects/project-1/config-revisions" &&
      method === "GET"
    ) {
      return jsonResponse(revisions);
    }
    if (
      url.pathname.startsWith("/api/v1/projects/project-1/config-revisions/") &&
      method === "GET"
    ) {
      const revisionId = decodeURIComponent(
        url.pathname.replace("/api/v1/projects/project-1/config-revisions/", ""),
      );
      const revision = revisions.find((candidate) => candidate.id === revisionId);
      return revision
        ? jsonResponse(revision)
        : jsonResponse({ error: "revision not found" }, 404);
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
  const queryClient = createQueryClient();
  return await render(
    <QueryClientProvider client={queryClient}>
      <ProjectConfigView
        apiBaseUrl="http://control-plane.test"
        fetcher={fetcher}
        projectId="project-1"
      />
    </QueryClientProvider>,
  );
}

async function renderConfigWithClient(fetcher: typeof fetch) {
  const queryClient = createQueryClient();
  const screen = await render(
    <QueryClientProvider client={queryClient}>
      <ProjectConfigView
        apiBaseUrl="http://control-plane.test"
        fetcher={fetcher}
        projectId="project-1"
      />
    </QueryClientProvider>,
  );

  return { queryClient, screen };
}

describe("ProjectConfigView", () => {
  it("renders config revision history and switches policy comparison details", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await expect.element(screen.getByText("配置修订历史")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "查看 revision #3" }))
      .toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "查看 revision #2" }));

    await expect
      .element(screen.getByRole("heading", { name: "协调策略" }))
      .toBeInTheDocument();
    await expect
      .element(screen.getByRole("heading", { name: "审批策略" }))
      .toBeInTheDocument();
    await expect
      .element(screen.getByRole("heading", { name: "证据归档规则" }))
      .toBeInTheDocument();
    await expect.element(screen.getByText(/human_review/)).toBeInTheDocument();
    await expect.element(screen.getByText(/archive_mode/)).toBeInTheDocument();
  });

  it("renders config tabs and saves current project policy", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();
    await expect.element(screen.getByRole("tab", { name: "任务历史" })).toBeInTheDocument();
    await userEvent.fill(screen.getByLabelText("人类 Owner 用户 ID"), "human-owner-2");
    await userEvent.fill(screen.getByLabelText("Leader 用户 ID"), "leader-user-2");
    await userEvent.fill(screen.getByLabelText("验收人用户 ID"), "acceptance-user-2");
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
        acceptance_user_id: "acceptance-user-2",
        coordination_policy: { cadence: "hourly" },
        human_owner_user_id: "human-owner-2",
        leader_user_id: "leader-user-2",
      });
    });
  });

  it("replaces members from the members tab", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await userEvent.fill(
      screen.getByLabelText("项目成员 JSON"),
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

  it("shows workflow impact notice when coordination policy or members are dirty", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "协调策略" }));
    await userEvent.fill(screen.getByLabelText("协调策略 JSON"), '{"cadence":"hourly"}');

    await expect
      .element(screen.getByText("保存后会向当前项目协调 Workflow 发送策略变更 signal"))
      .toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await userEvent.fill(screen.getByLabelText("项目成员 JSON"), "[]");

    await expect
      .element(screen.getByText("保存成员后会向当前项目协调 Workflow 发送成员变更 signal"))
      .toBeInTheDocument();
  });

  it("preserves dirty config and member drafts during background refetch", async () => {
    const refreshedConfig = makeConfig();
    refreshedConfig.coordination_policy = { cadence: "weekly" };
    refreshedConfig.members = [
      {
        display_name_snapshot: "后台刷新成员",
        id: "member-3",
        principal_id: "de-refetch",
        principal_type: "digital_employee",
        project_id: "project-1",
        project_role: "executor",
        settings: {},
        status: "active",
        tenant_id: "tenant-1",
      },
    ];
    refreshedConfig.digital_employee_pool = refreshedConfig.members;
    refreshedConfig.human_roles = [];
    const fetcher = createConfigFetcher("running", [makeConfig(), refreshedConfig]);
    const { queryClient, screen } = await renderConfigWithClient(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "协调策略" }));
    await userEvent.fill(
      screen.getByLabelText("协调策略 JSON"),
      '{"cadence":"hourly"}',
    );
    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await userEvent.fill(
      screen.getByLabelText("项目成员 JSON"),
      JSON.stringify([
        {
          principal_id: "human-owner-1",
          principal_type: "human_user",
          project_role: "owner",
        },
      ]),
    );

    await queryClient.refetchQueries({ queryKey: ["project-config", "project-1"] });

    await userEvent.click(screen.getByRole("tab", { name: "协调策略" }));
    await expect
      .element(screen.getByLabelText("协调策略 JSON"))
      .toHaveValue('{"cadence":"hourly"}');
    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await expect
      .element(screen.getByLabelText("项目成员 JSON"))
      .toHaveValue(
        JSON.stringify([
          {
            principal_id: "human-owner-1",
            principal_type: "human_user",
            project_role: "owner",
          },
        ]),
      );
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
