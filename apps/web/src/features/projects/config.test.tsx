import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectConfigView } from "@/features/projects/components/project-config-page";
import type { ProjectConfig, ProjectConfigRevision } from "@/lib/api/projects";

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
    defaultOptions: { queries: { retry: false } }
});
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status
});
}

function makeConfig(status: "running" | "archived" = "running"): ProjectConfig {
  const project = {
    coordination_policy: { cadence: "daily" },
    coordination_status: "registered",
    coordination_workflow_id: "project-coordinator:project-1",
    description: "配置说明",
    directory_name: "customer-acceptance",
    goal: "完成客户接入验收",
    human_owner_user_id: "human-owner-1",
    human_owner_user_ids: ["human-owner-1"] as string[],
    id: "project-1",
    name: "客户接入验收",
    status,
    tenant_id: "tenant-1",
    workspace_ready_status: "ready" as const
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
    tenant_id: "tenant-1"
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
    tenant_id: "tenant-1"
} as const;

  return {
    coordination_policy: project.coordination_policy,
    coordination_workflow: {
      status: "registered",
      workflow_id: "project-coordinator:project-1"
},
    digital_employee_pool: [digitalMember],
    human_roles: [humanMember],
    members: [humanMember, digitalMember],
    project
};
}

function makeConfigRevisions(): ProjectConfigRevision[] {
  return [
    {
      changed_sections: ["coordination_policy"],
      change_summary: "提高协调频率",
      config_snapshot: {
        coordination_policy: { cadence: "continuous" }
},
      created_at: "2026-01-03T08:00:00Z",
      created_by_user_id: "human-owner-1",
      diff_summary: { coordination_policy: "changed" },
      id: "revision-3",
      policy_fingerprint: "policy-fingerprint-3",
      project_id: "project-1",
      revision_number: 3,
      tenant_id: "tenant-1",
      previous_revision_id: "revision-2"
},
    {
      changed_sections: ["coordinationPolicy"],
      change_summary: "调整协调策略",
      config_snapshot: {
        coordination_policy: { cadence: "hourly" }
},
      created_at: "2026-01-02T08:00:00Z",
      created_by_user_id: "leader-user-1",
      diff_summary: { coordinationPolicy: "changed" },
      id: "revision-2",
      policy_fingerprint: "policy-fingerprint-2",
      project_id: "project-1",
      revision_number: 2,
      tenant_id: "tenant-1",
      previous_revision_id: "revision-1"
},
    {
      changed_sections: [],
      change_summary: "初始配置",
      config_snapshot: {
        coordination_policy: { cadence: "daily" }
},
      created_at: "2026-01-01T08:00:00Z",
      created_by_user_id: "human-owner-1",
      diff_summary: {},
      id: "revision-1",
      project_id: "project-1",
      revision_number: 1,
      tenant_id: "tenant-1"
},
  ];
}

function makeLatestRevision(): ProjectConfigRevision {
  return {
    changed_sections: ["coordination_policy"],
    change_summary: "新增最新修订",
    config_snapshot: {
      coordination_policy: { cadence: "minute" }
},
    created_at: "2026-01-04T08:00:00Z",
    created_by_user_id: "human-owner-1",
    diff_summary: { coordination_policy: "changed" },
    id: "revision-4",
    policy_fingerprint: "policy-fingerprint-4",
    project_id: "project-1",
    revision_number: 4,
    tenant_id: "tenant-1",
    previous_revision_id: "revision-3"
};
}

function makeMalformedConfigRevision(): ProjectConfigRevision {
  return {
    changed_sections: null,
    change_summary: "历史异常 payload",
    config_snapshot: null,
    created_by_user_id: "legacy-import",
    diff_summary: null,
    id: "revision-malformed",
    project_id: "project-1",
    revision_number: 4,
    tenant_id: "tenant-1"
} as unknown as ProjectConfigRevision;
}

function createConfigFetcher(
  status: "running" | "archived" = "running",
  configs: ProjectConfig[] = [makeConfig(status)],
  revisions: ProjectConfigRevision[] = makeConfigRevisions(),
) {
  let requestCount = 0;
  // 成员 PUT 后写回最新 config 快照，避免 invalidate 再 GET 时冲掉 UI 已落库的成员改动。
  let memberState: ProjectConfig["members"] | undefined;
  const latestConfig = () => {
    const base = configs[Math.min(requestCount, configs.length - 1)];
    if (!memberState) return base;
    return {
      ...base,
      digital_employee_pool: memberState.filter(
        (member) => member.principal_type === "digital_employee",
      ),
      human_roles: memberState.filter((member) => member.principal_type === "human_user"),
      members: memberState
    };
  };
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/projects/project-1/config" && method === "GET") {
      const config = latestConfig();
      requestCount += 1;
      return jsonResponse(config);
    }
    if (url.pathname === "/api/v1/projects/project-1/config" && method === "PUT") {
      return jsonResponse(latestConfig().project);
    }
    if (url.pathname === "/api/v1/projects/project-1/members" && method === "PUT") {
      const body = JSON.parse(String(init?.body ?? "{}")) as {
        members?: Array<{
          display_name_snapshot?: string;
          principal_id: string;
          principal_type: string;
          project_role: string;
          settings?: Record<string, unknown>;
        }>;
      };
      const members = (body.members ?? []).map((member, index) => ({
        display_name_snapshot: member.display_name_snapshot,
        id: `member-put-${index + 1}`,
        principal_id: member.principal_id,
        principal_type: member.principal_type,
        project_id: "project-1",
        project_role: member.project_role,
        settings: member.settings ?? {},
        status: "active",
        tenant_id: "tenant-1"
      })) as ProjectConfig["members"];
      memberState = members;
      return jsonResponse(members);
    }
    if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse([
        {
          description: "已在项目池中",
          employee_type: "standard_executor",
          id: "de-1",
          name: "验收执行员工",
          owner_user_id: "human-owner-1",
          permission_policy: {},
          provider_type: "claude-code",
          risk_level: "medium",
          role: "executor",
          status: "active",
          team_id: "team-1",
          team_name: "交付一队",
          tenant_id: "tenant-1"
        },
        {
          description: "可加入项目",
          employee_type: "standard_reviewer",
          id: "de-candidate",
          name: "可添加审查员",
          owner_user_id: "human-owner-1",
          permission_policy: {},
          provider_type: "claude-code",
          risk_level: "low",
          role: "reviewer",
          status: "active",
          team_id: "team-1",
          team_name: "交付一队",
          tenant_id: "tenant-1"
        },
        {
          description: "无团队不能加入",
          employee_type: "standard_executor",
          id: "de-teamless",
          name: "无团队员工",
          owner_user_id: "human-owner-1",
          permission_policy: {},
          provider_type: "claude-code",
          risk_level: "low",
          role: "executor",
          status: "active",
          tenant_id: "tenant-1"
        }
      ]);
    }
    if (url.pathname === "/api/auth/users" && method === "GET") {
      return jsonResponse({ items: [] });
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
          title: "整理历史任务"
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

async function openMembersJsonEditor(screen: Awaited<ReturnType<typeof renderConfig>>) {
  await userEvent.click(
    screen.getByRole("button", { name: /高级：成员完整替换 JSON/ }),
  );
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
    await expect.element(screen.getByText(/hourly/)).toBeInTheDocument();
  });

  it("renders malformed config revision payloads with safe JSON fallbacks", async () => {
    const malformedRevision = makeMalformedConfigRevision();
    const fetcher = createConfigFetcher("running", [makeConfig()], [
      malformedRevision,
      ...makeConfigRevisions(),
    ]);
    const screen = await renderConfig(fetcher);

    await expect.element(screen.getByText("配置修订历史")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "查看 revision #4" }));

    await expect
      .element(screen.getByRole("heading", { name: "协调策略" }))
      .toBeInTheDocument();
    await expect.element(screen.getByText("0 个变更区块")).toBeInTheDocument();
    await expect.element(screen.getByText("null").first()).toBeInTheDocument();
  });

  it("keeps selected revisions across refetches and falls back when removed", async () => {
    const revisions = makeConfigRevisions();
    const fetcher = createConfigFetcher("running", [makeConfig()], revisions);
    const { queryClient, screen } = await renderConfigWithClient(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "查看 revision #2" }));
    await expect.element(screen.getByText(/hourly/)).toBeInTheDocument();

    revisions.unshift(makeLatestRevision());
    await queryClient.refetchQueries({
      queryKey: ["project-config-revisions", "project-1"]
});

    await expect.element(screen.getByText(/hourly/)).toBeInTheDocument();
    await expect.element(screen.getByText(/minute/)).not.toBeInTheDocument();

    const selectedRevisionIndex = revisions.findIndex(
      (revision) => revision.id === "revision-2",
    );
    revisions.splice(selectedRevisionIndex, 1);
    await queryClient.refetchQueries({
      queryKey: ["project-config-revisions", "project-1"]
});

    await expect.element(screen.getByText(/minute/)).toBeInTheDocument();
    await expect.element(screen.getByText(/hourly/)).not.toBeInTheDocument();
  });

  it("renders config tabs and saves current project policy", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();
    await expect.element(screen.getByRole("tab", { name: "任务历史" })).toBeInTheDocument();
    const container = screen.container;
    expect(container.querySelectorAll('[data-slot="soft-card"]').length).toBeGreaterThan(
      0,
    );
    expect(container.querySelectorAll('[data-slot="status-pill"]').length).toBeGreaterThan(
      0,
    );
    expect(container.querySelectorAll('[data-slot="page-tab"]').length).toBeGreaterThan(
      0,
    );
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
        coordination_policy: { cadence: "hourly" }
});
      // 负责人不再从配置页发送(改由成员管理),PUT body 不应含 human_owner_user_id
      expect(
        Object.prototype.hasOwnProperty.call(
          JSON.parse(String(putCall?.[1]?.body)),
          "human_owner_user_id",
        ),
      ).toBe(false);
    });
  });

  it("replaces members from the members tab", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await openMembersJsonEditor(screen);
    await userEvent.fill(
      screen.getByLabelText("项目成员 JSON"),
      JSON.stringify([
        {
          principal_id: "human-owner-1",
          principal_type: "human_user",
          project_role: "owner"
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
            project_role: "owner"
},
        ]
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
    await openMembersJsonEditor(screen);
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
        tenant_id: "tenant-1"
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
    await openMembersJsonEditor(screen);
    await userEvent.fill(
      screen.getByLabelText("项目成员 JSON"),
      JSON.stringify([
        {
          principal_id: "human-owner-1",
          principal_type: "human_user",
          project_role: "owner"
},
      ]),
    );

    await queryClient.refetchQueries({ queryKey: ["project-config", "project-1"] });

    await userEvent.click(screen.getByRole("tab", { name: "协调策略" }));
    await expect
      .element(screen.getByLabelText("协调策略 JSON"))
      .toHaveValue('{"cadence":"hourly"}');
    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await openMembersJsonEditor(screen);
    await expect
      .element(screen.getByLabelText("项目成员 JSON"))
      .toHaveValue(
        JSON.stringify([
          {
            principal_id: "human-owner-1",
            principal_type: "human_user",
            project_role: "owner"
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
    await openMembersJsonEditor(screen);
    await expect
      .element(screen.getByRole("button", { name: "保存成员池" }))
      .toBeDisabled();
  });

  it("renders humanized members and task history as v3 work surfaces", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "成员" }));

    expect(
      screen.container.querySelector('[data-slot="work-surface"]'),
    ).toBeTruthy();
    await expect
      .element(screen.getByRole("heading", { name: "人类成员" }))
      .toBeInTheDocument();
    await expect
      .element(screen.getByRole("heading", { name: "数字员工" }))
      .toBeInTheDocument();
    await expect.element(screen.getByText("负责人甲").first()).toBeInTheDocument();
    await expect.element(screen.getByText("验收执行员工")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "添加数字员工" }))
      .toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "任务历史" }));

    expect(screen.container.querySelector('[data-slot="data-table"]')).toBeTruthy();
    await expect.element(screen.getByText("整理历史任务")).toBeInTheDocument();
  });

  it("adds a digital employee from the picker dialog", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await userEvent.click(screen.getByRole("button", { name: "添加数字员工" }));

    await expect
      .element(screen.getByRole("heading", { name: "添加数字员工" }))
      .toBeInTheDocument();
    // 候选列表是 dialog portal 内的可切换行；已在池中 / 无团队的员工不得作为候选。
    await expect
      .element(screen.getByRole("button", { name: /可添加审查员/ }))
      .toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: /无团队员工/ }))
      .not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /可添加审查员/ }));
    await userEvent.click(screen.getByRole("button", { name: "加入项目" }));

    await vi.waitFor(() => {
      const putCall = fetchCalls(fetcher).find(([url, init]) => {
        return (
          String(url).endsWith("/api/v1/projects/project-1/members") &&
          init?.method === "PUT"
        );
      });
      expect(putCall).toBeTruthy();
      expect(JSON.parse(String(putCall?.[1]?.body))).toMatchObject({
        members: expect.arrayContaining([
          expect.objectContaining({
            principal_id: "human-owner-1",
            principal_type: "human_user",
            project_role: "owner"
          }),
          expect.objectContaining({
            principal_id: "de-1",
            principal_type: "digital_employee",
            project_role: "executor"
          }),
          expect.objectContaining({
            display_name_snapshot: "可添加审查员",
            principal_id: "de-candidate",
            principal_type: "digital_employee",
            project_role: "executor"
          })
        ])
      });
    });

    await expect
      .element(screen.getByRole("button", { name: "移除 可添加审查员" }))
      .toBeInTheDocument();
  });

  it("removes a digital employee from the members panel", async () => {
    const fetcher = createConfigFetcher();
    const screen = await renderConfig(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await userEvent.click(
      screen.getByRole("button", { name: "移除 验收执行员工" }),
    );

    await vi.waitFor(() => {
      const putCall = fetchCalls(fetcher).find(([url, init]) => {
        return (
          String(url).endsWith("/api/v1/projects/project-1/members") &&
          init?.method === "PUT"
        );
      });
      expect(putCall).toBeTruthy();
      expect(JSON.parse(String(putCall?.[1]?.body))).toEqual({
        members: [
          {
            display_name_snapshot: "负责人甲",
            principal_id: "human-owner-1",
            principal_type: "human_user",
            project_role: "owner",
            settings: {}
          }
        ]
      });
    });

    // toast 可能仍含员工名，以移除按钮与空态为准。
    await expect
      .element(screen.getByRole("button", { name: "移除 验收执行员工" }))
      .not.toBeInTheDocument();
    await expect
      .element(screen.getByText("暂无数字员工，点击右上角从目录加入项目执行池"))
      .toBeInTheDocument();
  });

  it("disables digital employee management for archived projects", async () => {
    const fetcher = createConfigFetcher("archived");
    const screen = await renderConfig(fetcher);

    await userEvent.click(screen.getByRole("tab", { name: "成员" }));
    await expect
      .element(screen.getByRole("button", { name: "添加数字员工" }))
      .toBeDisabled();
    await expect
      .element(screen.getByRole("button", { name: "移除 验收执行员工" }))
      .toBeDisabled();
  });
});
