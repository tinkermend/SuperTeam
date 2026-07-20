import { forwardRef, useState, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { page, userEvent } from "vitest/browser";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { CreateProjectView, ProjectsView } from "@/features/projects";
import type { Project } from "@/lib/api/projects";

const CURRENT_USER_ID = "current-user-1";
const TEAM_AUTHORIZED_ID = "team-authorized-1";
const TEAM_REVIEW_ID = "team-review-1";
const TEAM_REVOKED_ID = "team-revoked-1";
const TEAM_DISABLED_ID = "team-disabled-1";
const LEADER_USER_ID = "leader-user-1";
const ACCEPTANCE_USER_ID = "acceptance-user-1";
const REVIEWER_USER_ID = "reviewer-user-1";
const EMPLOYEE_PLATFORM_ID = "employee-platform-1";
const EMPLOYEE_ASSISTANT_ID = "employee-assistant-1";
const EMPLOYEE_QA_ID = "employee-qa-1";

const routerMock = vi.hoisted(() => ({
  navigate: vi.fn(),
}));

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
    search?: Record<string, string>;
    to: string;
  };
  const Link = forwardRef<HTMLAnchorElement, MockLinkProps>(
    ({ children, params, search, to, ...props }, ref) => {
      let path = to;
      if (params?.projectId) {
        path = path.replace("$projectId", encodeURIComponent(params.projectId));
      }
      if (params?.demandId) {
        path = path.replace("$demandId", encodeURIComponent(params.demandId));
      }
      const query = search ? `?${new URLSearchParams(search).toString()}` : "";

      return (
        <a {...props} href={`${path}${query}`} ref={ref}>
          {children}
        </a>
      );
    },
  );
  Link.displayName = "MockRouterLink";

  return {
    Link,
    useNavigate: () => routerMock.navigate,
    useSearch: () => ({}),
  };
});

beforeEach(() => {
  routerMock.navigate.mockClear();
});

function createQueryClient(options: { staleTime?: number } = {}) {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: options.staleTime,
      },
    },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function makeProject(id: string, name: string, status: Project["status"] = "running"): Project {
  const updatedAt = id === "project-2" ? "2026-06-05T02:28:13Z" : "2026-06-04T02:28:13Z";
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
    updated_at: updatedAt,
  };
}

function makeDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, reject, resolve };
}

function userProjectTeamScopesResponse() {
  return {
    items: [
      {
        created_at: "2026-06-04T02:28:13Z",
        granted_by_user_id: "admin-user-1",
        id: "scope-authorized-1",
        revoked_at: null,
        status: "active",
        team: {
          current_revision: 2,
          digital_employee_count: 4,
          governance_status: "active",
          human_owners: [],
          id: TEAM_AUTHORIZED_ID,
          name: "平台运营",
          pending_draft_count: 0,
          risk_summary: "低风险",
          slug: "ops",
          status: "active",
        },
        team_id: TEAM_AUTHORIZED_ID,
        tenant_id: "tenant-1",
        updated_at: "2026-06-04T02:28:13Z",
        user_id: CURRENT_USER_ID,
      },
      {
        created_at: "2026-06-04T02:28:13Z",
        granted_by_user_id: "admin-user-1",
        id: "scope-review-1",
        revoked_at: null,
        status: "active",
        team: {
          current_revision: 1,
          digital_employee_count: 2,
          governance_status: "active",
          human_owners: [],
          id: TEAM_REVIEW_ID,
          name: "风控审查",
          pending_draft_count: 0,
          risk_summary: "中风险",
          slug: "review",
          status: "active",
        },
        team_id: TEAM_REVIEW_ID,
        tenant_id: "tenant-1",
        updated_at: "2026-06-04T02:28:13Z",
        user_id: CURRENT_USER_ID,
      },
      {
        created_at: "2026-06-04T02:28:13Z",
        granted_by_user_id: "admin-user-1",
        id: "scope-revoked-1",
        revoked_at: "2026-06-05T02:28:13Z",
        status: "revoked",
        team: {
          current_revision: 1,
          digital_employee_count: 1,
          governance_status: "active",
          human_owners: [],
          id: TEAM_REVOKED_ID,
          name: "已撤销团队",
          pending_draft_count: 0,
          risk_summary: "低风险",
          slug: "revoked",
          status: "active",
        },
        team_id: TEAM_REVOKED_ID,
        tenant_id: "tenant-1",
        updated_at: "2026-06-05T02:28:13Z",
        user_id: CURRENT_USER_ID,
      },
      {
        created_at: "2026-06-04T02:28:13Z",
        granted_by_user_id: "admin-user-1",
        id: "scope-disabled-1",
        revoked_at: null,
        status: "active",
        team: {
          current_revision: 1,
          digital_employee_count: 1,
          governance_status: "active",
          human_owners: [],
          id: TEAM_DISABLED_ID,
          name: "停用团队",
          pending_draft_count: 0,
          risk_summary: "低风险",
          slug: "disabled",
          status: "disabled",
        },
        team_id: TEAM_DISABLED_ID,
        tenant_id: "tenant-1",
        updated_at: "2026-06-04T02:28:13Z",
        user_id: CURRENT_USER_ID,
      },
    ],
  };
}

function usersResponse(q?: string) {
  const allUsers = [
    {
      avatar: { provider: "dicebear", seed: "leader", style: "adventurer" },
      avatar_asset_id: null,
      display_name: "李娜",
      email: "leader@example.com",
      id: LEADER_USER_ID,
      status: "active",
      username: "leader",
    },
    {
      avatar: { provider: "dicebear", seed: "acceptance", style: "adventurer" },
      avatar_asset_id: null,
      display_name: "王磊",
      email: "acceptance@example.com",
      id: ACCEPTANCE_USER_ID,
      status: "active",
      username: "acceptance",
    },
    {
      avatar: { provider: "dicebear", seed: "reviewer", style: "adventurer" },
      avatar_asset_id: null,
      display_name: "赵明",
      email: "reviewer@example.com",
      id: REVIEWER_USER_ID,
      status: "active",
      username: "reviewer",
    },
  ];

  return {
    items: q ? allUsers.filter(u => u.display_name.includes(q) || u.username.includes(q)) : allUsers,
  };
}

function digitalEmployeesResponse(teamId?: string | null) {
  const employees = [
    {
      approval_policy: {},
      context_policy: {},
      created_at: "2026-06-01T00:00:00Z",
      employee_type: "ops",
      id: EMPLOYEE_PLATFORM_ID,
      metadata: { effective_config_label: "平台值守" },
      name: "平台值守员",
      owner_user_id: CURRENT_USER_ID,
      permission_policy: {},
      risk_level: "low",
      role: "platform-operator",
      status: "active",
      team_id: TEAM_AUTHORIZED_ID,
      tenant_id: "tenant-1",
    },
    {
      approval_policy: {},
      context_policy: {},
      created_at: "2026-06-01T00:00:00Z",
      employee_type: "delivery",
      id: EMPLOYEE_ASSISTANT_ID,
      metadata: { effective_config_label: "代码实现" },
      name: "研发助手",
      owner_user_id: CURRENT_USER_ID,
      permission_policy: {},
      risk_level: "medium",
      role: "rd-assistant",
      status: "active",
      team_id: TEAM_REVIEW_ID,
      tenant_id: "tenant-1",
    },
    {
      approval_policy: {},
      context_policy: {},
      created_at: "2026-06-01T00:00:00Z",
      employee_type: "qa",
      id: EMPLOYEE_QA_ID,
      metadata: { effective_config_label: "自动化测试" },
      name: "测试工程师",
      owner_user_id: CURRENT_USER_ID,
      permission_policy: {},
      risk_level: "low",
      role: "qa-engineer",
      status: "active",
      team_id: TEAM_REVIEW_ID,
      tenant_id: "tenant-1",
    },
  ];
  return teamId ? employees.filter((employee) => employee.team_id === teamId) : employees;
}

function createProjectFetcher(
  options: {
    currentUserDeferred?: ReturnType<typeof makeDeferred<Response>>;
    currentUserStatus?: "default" | "error" | "loading";
    projectTeamScopesDeferred?: ReturnType<typeof makeDeferred<Response>>;
    projectTeamScopesStatus?: "default" | "empty" | "error" | "loading";
    project2OverviewGate?: Promise<void>;
    project2RiskSignalGate?: Promise<void>;
    project2RuntimeReadinessGate?: Promise<void>;
    riskSignalFailureProjectId?: string;
    slowFilteredList?: boolean;
    runtimeReadinessStatus?: "missing" | "ready";
    projectAllowedActions?: string[];
    deletePreviewCanDelete?: boolean;
    deleteStatus?: number;
    deletePayload?: unknown;
    includeArchivedProject?: boolean;
  } = {},
) {
  const projects = [
    {
      ...makeProject("project-1", "客户接入验收"),
      allowed_actions: options.projectAllowedActions ?? ["project.archive", "project.delete"],
    },
    makeProject("project-2", "生产巡检整改"),
    ...(options.includeArchivedProject
      ? [makeProject("project-3", "已归档走查项目", "archived")]
      : []),
  ];

  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/auth/me" && method === "GET") {
      if (options.currentUserStatus === "loading" && options.currentUserDeferred) {
        return options.currentUserDeferred.promise;
      }
      if (options.currentUserStatus === "error") {
        return jsonResponse({ error: "current user load failed" }, 500);
      }
      return jsonResponse({
        user: {
          avatar: { provider: "dicebear", seed: "current-user", style: "adventurer" },
          avatar_asset_id: null,
          display_name: "当前用户",
          email: "current@example.com",
          id: CURRENT_USER_ID,
          status: "active",
          username: "current-user",
        },
      });
    }

    if (
      url.pathname === `/api/auth/users/${CURRENT_USER_ID}/project-team-scopes` &&
      method === "GET"
    ) {
      if (options.projectTeamScopesStatus === "loading" && options.projectTeamScopesDeferred) {
        return options.projectTeamScopesDeferred.promise;
      }
      if (options.projectTeamScopesStatus === "error") {
        return jsonResponse({ error: "scope load failed" }, 500);
      }
      if (options.projectTeamScopesStatus === "empty") {
        return jsonResponse({ items: [] });
      }
      return jsonResponse(userProjectTeamScopesResponse());
    }

    if (url.pathname === "/api/auth/users" && method === "GET") {
      return jsonResponse(usersResponse(url.searchParams.get("q") ?? undefined));
    }

    if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(digitalEmployeesResponse(url.searchParams.get("team_id")));
    }

    if (url.pathname === "/api/v1/runtime/nodes" && method === "GET") {
      return jsonResponse([
        {
          command_channel_connected: true,
          current_load: 0,
          max_slots: 2,
          name: "local-dev-node",
          node_id: "local-dev-node",
          runtime_node_id: "runtime-node-1",
          status: "online",
          supported_providers: ["codex"],
        },
      ]);
    }

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
    if (url.pathname === "/api/v1/projects" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as Partial<Project>;
      const created = makeProject("project-3", body.name ?? "新建项目");
      created.description = body.description;
      created.goal = body.goal ?? created.goal;
      created.human_owner_user_id = body.human_owner_user_id ?? created.human_owner_user_id;
      projects.unshift(created);
      return jsonResponse({
        members: [],
        project: created,
      });
    }
    if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse([]);
    }

    if (url.pathname === "/api/v1/inbox/items" && method === "GET") {
      return jsonResponse({
        items: [
          {
            actions: [],
            context: {},
            created_at: "2026-06-05T01:00:00Z",
            deep_link: {},
            id: "inbox-item-1",
            item_type: "project_decision",
            last_activity_at: "2026-06-05T01:30:00Z",
            risk_level: "high",
            source_id: "decision-1",
            source_project_id: "project-1",
            source_type: "project_decision_request",
            status: "open",
            target_user_id: CURRENT_USER_ID,
            tenant_id: "tenant-1",
            title: "需要负责人确认上线计划",
            updated_at: "2026-06-05T01:30:00Z",
          },
          {
            actions: [],
            context: {},
            created_at: "2026-06-05T02:00:00Z",
            deep_link: {},
            id: "inbox-item-2",
            item_type: "approval",
            last_activity_at: "2026-06-05T02:15:00Z",
            source_id: "approval-1",
            source_type: "approval_request",
            status: "open",
            target_user_id: CURRENT_USER_ID,
            tenant_id: "tenant-1",
            title: "生产环境发布审批",
            updated_at: "2026-06-05T02:15:00Z",
          },
        ],
        pagination: { has_more: false, limit: 8, offset: 0 },
        summary: { blocked_count: 0, high_risk_count: 1, open_count: 2 },
      });
    }

    if (url.pathname === "/api/v1/inbox/badge" && method === "GET") {
      return jsonResponse({
        high_risk_count: 1,
        mine_open_count: 2,
        team_open_count: 3,
      });
    }

    if (url.pathname === "/api/v1/workflow-instances" && method === "GET") {
      return jsonResponse([
        {
          created_at: "2026-06-04T02:30:00Z",
          demand_id: "demand-1",
          progress: {
            blocked_nodes: 0,
            completed_nodes: 1,
            running_nodes: 1,
            total_nodes: 2,
            waiting_human_nodes: 1,
          },
          project_id: "project-1",
          project_name: "客户接入验收",
          status: "waiting_human",
          status_reason: "需要负责人确认",
          submitted_by_display_name: "负责人甲",
          submitted_by_user_id: "human-owner-1",
          title: "补充上线验收说明",
          updated_at: "2026-06-04T03:00:00Z",
        },
        {
          created_at: "2026-06-05T02:10:00Z",
          demand_id: "demand-2",
          progress: {
            blocked_nodes: 1,
            completed_nodes: 0,
            running_nodes: 0,
            total_nodes: 1,
            waiting_human_nodes: 0,
          },
          project_id: "project-2",
          project_name: "生产巡检整改",
          status: "failed",
          status_reason: "任务执行失败",
          submitted_by_display_name: "负责人甲",
          submitted_by_user_id: "human-owner-1",
          title: "分析生产接口 5xx 激增原因并输出处置建议",
          updated_at: "2026-06-05T02:40:00Z",
        },
      ]);
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
            display_name_snapshot: "验收执行员工",
            id: "member-employee-1",
            principal_id: "de-1",
            principal_type: "digital_employee",
            project_id: "project-1",
            project_role: "executor",
            settings: { source_team_name: "平台运营" },
            status: "active",
            tenant_id: "tenant-1",
          },
        ],
        human_roles: [
          {
            display_name_snapshot: "负责人甲",
            id: "member-owner-1",
            principal_id: "human-owner-1",
            principal_type: "human_user",
            project_id: "project-1",
            project_role: "owner",
            settings: {},
            status: "active",
            tenant_id: "tenant-1",
          },
          {
            display_name_snapshot: "负责人乙",
            id: "member-owner-2",
            principal_id: "human-owner-2",
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
          {
            actor_id: "system",
            actor_type: "system",
            event_type: "route_decision.created",
            id: "event-2",
            payload: {},
            project_id: "project-1",
            resource_id: "route-decision-1",
            resource_type: "route_decision",
            sequence_number: 2,
            summary: "路由决策已生成",
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

    if (url.pathname === "/api/v1/projects/project-1/runtime-readiness" && method === "GET") {
      if (options.runtimeReadinessStatus === "ready") {
        return jsonResponse({
          blocking_reasons: [],
          command_channel_connected: true,
          employee_readiness: [
            {
              can_dispatch: true,
              can_plan: true,
              digital_employee_id: "de-1",
              display_name: "验收执行员工",
              provider_type: "codex",
            },
          ],
          next_actions: [],
          placement_status: "ready",
          provider_capabilities: ["codex"],
          required_provider_types: ["codex"],
          runtime_node_id: "runtime-node-1",
          runtime_node_name: "local-dev-node",
        });
      }

      return jsonResponse({
        blocking_reasons: [
          {
            code: "runtime_placement_missing",
            message: "项目尚未绑定运行节点",
            resource_id: "project-1",
            resource_type: "project",
          },
        ],
        command_channel_connected: false,
        employee_readiness: [
          {
            can_dispatch: false,
            can_plan: true,
            digital_employee_id: "de-1",
            display_name: "验收执行员工",
            provider_type: "codex",
            reason_code: "runtime_placement_missing",
            reason_message: "项目尚未绑定运行节点",
          },
        ],
        next_actions: [
          {
            code: "bind_runtime_node",
            label: "绑定运行节点",
          },
        ],
        placement_status: "missing",
        provider_capabilities: [],
        required_provider_types: ["codex"],
      });
    }

    if (
      url.pathname.startsWith("/api/v1/projects/project-1/runtime-nodes/") &&
      method === "PUT"
    ) {
      const runtimeNodeId = url.pathname.split("/").pop();
      return jsonResponse({ runtime_node_id: runtimeNodeId });
    }

    if (
      url.pathname.startsWith("/api/v1/projects/project-1/runtime-nodes/") &&
      method === "DELETE"
    ) {
      return new Response(null, { status: 204 });
    }

    if (url.pathname.endsWith("/overview") && method === "GET") {
      const id = url.pathname.split("/")[4];
      if (id === "project-2" && options.project2OverviewGate) {
        await options.project2OverviewGate;
      }
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

    if (url.pathname.endsWith("/runtime-readiness") && method === "GET") {
      const id = url.pathname.split("/")[4];
      if (id === "project-2" && options.project2RuntimeReadinessGate) {
        await options.project2RuntimeReadinessGate;
      }
      return jsonResponse({
        blocking_reasons: [],
        command_channel_connected: false,
        employee_readiness: [],
        next_actions: [],
        placement_status: "missing",
        provider_capabilities: [],
        required_provider_types: [],
      });
    }

    if (
      options.riskSignalFailureProjectId &&
      url.pathname.startsWith(`/api/v1/projects/${options.riskSignalFailureProjectId}/`) &&
      ["/tasks", "/decisions", "/evidence", "/events", "/members"].some((suffix) =>
        url.pathname.endsWith(suffix),
      ) &&
      method === "GET"
    ) {
      return jsonResponse({ error: "risk signal load failed" }, 500);
    }

    if (url.pathname === "/api/v1/projects/project-1/members" && method === "GET") {
      return jsonResponse([
        {
          display_name_snapshot: "验收执行员工",
          id: "member-employee-1",
          principal_id: "de-1",
          principal_type: "digital_employee",
          project_id: "project-1",
          project_role: "executor",
          settings: { source_team_name: "平台运营" },
          status: "active",
          tenant_id: "tenant-1",
        },
        {
          display_name_snapshot: "负责人甲",
          id: "member-owner-1",
          principal_id: "human-owner-1",
          principal_type: "human_user",
          project_id: "project-1",
          project_role: "owner",
          settings: {},
          status: "active",
          tenant_id: "tenant-1",
        },
      ]);
    }

    if (url.pathname === "/api/v1/projects/project-2/members" && method === "GET") {
      if (options.project2RiskSignalGate) {
        await options.project2RiskSignalGate;
      }
      return jsonResponse([
        {
          display_name_snapshot: "运维检索员工",
          id: "member-ops-1",
          principal_id: "de-ops-1",
          principal_type: "digital_employee",
          project_id: "project-2",
          project_role: "executor",
          settings: { source_team_name: "平台运营" },
          status: "active",
          tenant_id: "tenant-1",
        },
      ]);
    }

    if (url.pathname === "/api/v1/projects/project-2/tasks" && method === "GET") {
      if (options.project2RiskSignalGate) {
        await options.project2RiskSignalGate;
      }
      return jsonResponse([
        {
          id: "task-project-2-failed",
          project_id: "project-2",
          requires_human_approval: false,
          assigned_digital_employee_id: "de-ops-1",
          task_kind: "data_source.log_search",
          status: "failed",
          tenant_id: "tenant-1",
          title: "数据源: 日志检索",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-2/decisions" && method === "GET") {
      if (options.project2RiskSignalGate) {
        await options.project2RiskSignalGate;
      }
      return jsonResponse([]);
    }
    if (url.pathname === "/api/v1/projects/project-2/evidence" && method === "GET") {
      if (options.project2RiskSignalGate) {
        await options.project2RiskSignalGate;
      }
      return jsonResponse([]);
    }
    if (url.pathname === "/api/v1/projects/project-2/events" && method === "GET") {
      return jsonResponse([
        {
          actor_id: "runtime",
          actor_type: "system",
          event_type: "project_task.failed",
          id: "event-project-2-failed",
          payload: {},
          project_id: "project-2",
          sequence_number: 3,
          summary: "巡检脚本失败",
          tenant_id: "tenant-1",
        },
      ]);
    }

    if (url.pathname.endsWith("/tasks") && method === "GET") {
      const projectId = url.pathname.split("/")[4];
      if (projectId === "project-1") {
        return jsonResponse([
          {
            assigned_digital_employee_id: "de-1",
            id: "task-1",
            project_id: "project-1",
            requires_human_approval: true,
            status: "running",
            task_kind: "human_review",
            tenant_id: "tenant-1",
            title: "人工: 负责人确认",
          },
        ]);
      }
      return jsonResponse([]);
    }
    if (
      url.pathname === "/api/v1/projects/project-1/tasks/task-1/dispatch-gates" &&
      method === "GET"
    ) {
      return jsonResponse({
        items: [
          {
            attempt_no: 1,
            blockers: [
              {
                details: {},
                key: "runtime.node_offline",
                retryable: true,
                severity: "transient",
              },
            ],
            checked_at: "2026-06-21T12:00:00Z",
            checks: [],
            dispatch_reason: "root_ready",
            human_action_request: {},
            id: "dispatch-gate-1",
            project_task_id: "task-1",
            retry_after: "2026-06-21T12:02:00Z",
            selected_employee_id: "de-1",
            status: "retry_later",
          },
        ],
      });
    }
    if (url.pathname.endsWith("/dispatch-gates") && method === "GET") {
      return jsonResponse({ items: [] });
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
    if (url.pathname === "/api/v1/projects/project-1/task-graph" && method === "GET") {
      return jsonResponse({
        decision_requests: [],
        edges: [],
        employees: [],
        execution_summaries: [],
        nodes: [
          {
            expected_outputs: [],
            handoff_contract: {},
            id: "task-old",
            input_requirements: {},
            planner_metadata: {},
            project_id: "project-1",
            requires_human_approval: false,
            stage_index: 0,
            status: "completed",
            tenant_id: "tenant-1",
            title: "完成环境准备",
          },
          {
            expected_outputs: [],
            handoff_contract: {},
            id: "task-1",
            input_requirements: {},
            planner_metadata: {},
            project_id: "project-1",
            requires_human_approval: true,
            stage_index: 1,
            status: "running",
            tenant_id: "tenant-1",
            title: "整理接入证据",
          },
        ],
        recent_events: [],
        runs: [],
        stage_summaries: [],
      });
    }
    if (url.pathname === "/api/v1/projects/project-1/route-decisions" && method === "GET") {
      return jsonResponse([
        {
          budget_estimate: {},
          candidate_digital_employee_ids: ["de-1"],
          coordination_job_id: "job-1",
          expected_outputs: ["执行摘要"],
          id: "route-1",
          input_requirements: {},
          project_id: "project-1",
          reason: "选择项目数字员工池中的 active executor",
          requires_human_review: false,
          selected_digital_employee_ids: ["de-1"],
          tenant_id: "tenant-1",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/plan-revisions" && method === "GET") {
      return jsonResponse([
        {
          coordination_job_id: "job-1",
          created_task_ids: [],
          demand_id: "demand-1",
          id: "plan-revision-1",
          payload: {
            risk_assessment: { highest_risk_level: "high" },
            summary: "复核生产巡检计划",
            tasks: [
              {
                expected_outputs: ["巡检结论", "风险说明"],
                matched_capabilities: ["codebase.analysis"],
                planned_task_key: "inspect-runtime",
                required_capabilities: ["runtime.inspect"],
                risk_level: "high",
                title: "检查 Runtime 状态",
              },
            ],
          },
          plan_fingerprint: "fingerprint",
          project_id: "project-1",
          review_required: true,
          revision_number: 2,
          status: "pending_review",
          tenant_id: "tenant-1",
          validation_errors: [],
          validation_warnings: [],
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/coordination-jobs" && method === "GET") {
      return jsonResponse([
        {
          id: "job-1",
          input_snapshot_ref: { demand_id: "demand-1" },
          job_type: "demand_route",
          output_event_ids: [],
          project_id: "project-1",
          status: "completed",
          tenant_id: "tenant-1",
          workflow_id: "project-coordinator:project-1",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/decisions" && method === "GET") {
      return jsonResponse([
        {
          approval_request_id: "approval-1",
          decision_type: "route_review",
          id: "decision-1",
          project_id: "project-1",
          status_snapshot: "pending",
          summary_snapshot: "需要负责人确认",
          target_user_id: "human-owner-1",
          tenant_id: "tenant-1",
          title_snapshot: "需要负责人确认",
        },
        {
          approval_request_id: "approval-plan-1",
          coordination_job_id: "job-1",
          decision_type: "plan_review",
          id: "decision-plan-1",
          project_id: "project-1",
          status_snapshot: "pending",
          summary_snapshot: "复核生产巡检计划",
          target_user_id: "human-owner-1",
          tenant_id: "tenant-1",
          title_snapshot: "计划版本 v2 需要复核",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/execution-summaries" && method === "GET") {
      return jsonResponse([
        {
          artifact_refs: [],
          confidence_factors: {},
          conclusion: "证据充分",
          digital_employee_id: "de-1",
          evidence_refs: [],
          id: "summary-1",
          missing_information: [],
          project_id: "project-1",
          project_task_id: "task-1",
          requires_human_review: false,
          tenant_id: "tenant-1",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/transfer-requests" && method === "GET") {
      return jsonResponse([
        {
          id: "transfer-1",
          missing_context_refs: [],
          project_id: "project-1",
          project_task_id: "task-1",
          reason: "需要安全专家补充上线窗口评估",
          requested_by_digital_employee_id: "de-1",
          status: "requested",
          suggested_digital_employee_ids: [],
          tenant_id: "tenant-1",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/evidence" && method === "GET") {
      return jsonResponse([
        {
          evidence_type: "acceptance_check",
          id: "evidence-1",
          metadata: { severity: "medium" },
          project_id: "project-1",
          source_ref: "ticket://SUP-42",
          source_type: "ticket",
          submitted_by_id: "de-1",
          submitted_by_type: "digital_employee",
          summary: "接入链路验收证据已整理",
          tenant_id: "tenant-1",
          title: "上线验收证据",
          verification_status: "verified",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/evidence" && method === "POST") {
      const body = JSON.parse(String(init?.body));
      return jsonResponse(
        {
          created_event_id: "event-evidence-created",
          evidence_type: body.evidence_type,
          id: "evidence-created",
          metadata: body.metadata ?? {},
          project_id: "project-1",
          source_ref: body.source_ref,
          source_type: body.source_type,
          submitted_by_id: "human-owner-1",
          submitted_by_type: "human_user",
          summary: body.summary,
          tenant_id: "tenant-1",
          title: body.title,
          verification_status: "submitted",
        },
        201,
      );
    }
    if (
      url.pathname === "/api/v1/projects/project-1/evidence/evidence-1" &&
      method === "PATCH"
    ) {
      const body = JSON.parse(String(init?.body));
      return jsonResponse({
        evidence_type: "acceptance_check",
        id: "evidence-1",
        metadata: body.metadata ?? {},
        project_id: "project-1",
        source_ref: "ticket://SUP-42",
        source_type: "ticket",
        submitted_by_id: "de-1",
        submitted_by_type: "digital_employee",
        tenant_id: "tenant-1",
        title: "上线验收证据",
        verification_status: body.verification_status,
      });
    }
    if (url.pathname === "/api/v1/projects/project-1/artifacts" && method === "GET") {
      return jsonResponse([
        {
          artifact_type: "log_bundle",
          content_type: "application/gzip",
          id: "artifact-1",
          metadata: {},
          object_ref: "s3://superteam/project-1/logs.tgz",
          project_id: "project-1",
          retention_status: "retained",
          size_bytes: 2048,
          tenant_id: "tenant-1",
          title: "回归日志包",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/reports" && method === "GET") {
      return jsonResponse([
        {
          format: "markdown",
          generated_by_id: "de-1",
          generated_by_type: "digital_employee",
          id: "report-1",
          object_ref: "s3://superteam/project-1/acceptance.md",
          project_id: "project-1",
          report_type: "acceptance",
          summary: "验收摘要已生成",
          tenant_id: "tenant-1",
          title: "客户接入验收报告",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/budget-ledger" && method === "GET") {
      return jsonResponse([
        {
          actual_cost: "2.50",
          actual_tokens: 4800,
          cost_type: "llm",
          digital_employee_id: "de-1",
          estimated_cost: "3.00",
          estimated_tokens: 6000,
          id: "budget-1",
          project_id: "project-1",
          reason: "整理验收证据与报告",
          source: "execution_summary",
          tenant_id: "tenant-1",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/budget-summary" && method === "GET") {
      return jsonResponse({
        actual_cost: "2.50",
        actual_tokens: 4800,
        estimated_cost: "3.00",
        estimated_tokens: 6000,
        ledger_count: 1,
      });
    }
    if (url.pathname === "/api/v1/projects/project-1/acceptance" && method === "GET") {
      return jsonResponse({
        accepted_by_user_id: "human-owner-1",
        conclusion: "同意客户接入验收通过",
        evidence_ref_ids: ["evidence-1"],
        id: "acceptance-1",
        project_id: "project-1",
        report_ref_ids: ["report-1"],
        status: "accepted",
        summary: "证据、工件和报告均已归档",
        tenant_id: "tenant-1",
        unresolved_risks: [],
      });
    }
    if (url.pathname === "/api/v1/projects/project-1/acceptance" && method === "POST") {
      const body = JSON.parse(String(init?.body));
      return jsonResponse(
        {
          accepted_by_user_id: "human-owner-1",
          conclusion: body.conclusion,
          evidence_ref_ids: body.evidence_ref_ids,
          id: "acceptance-created",
          project_id: "project-1",
          report_ref_ids: body.report_ref_ids,
          status: body.status,
          summary: body.summary,
          tenant_id: "tenant-1",
          unresolved_risks: body.unresolved_risks ?? [],
        },
        201,
      );
    }
    if (url.pathname === "/api/v1/projects/project-1/archive-snapshots" && method === "GET") {
      return jsonResponse([
        {
          created_by_user_id: "human-owner-1",
          id: "snapshot-1",
          included_counts: {
            artifacts: 1,
            evidence: 1,
            reports: 1,
          },
          object_ref: "s3://superteam/project-1/archive.json",
          project_id: "project-1",
          retained_artifact_ids: ["artifact-1"],
          snapshot_type: "project_archive",
          status: "completed",
          summary: "当前项目归档快照",
          tenant_id: "tenant-1",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-1/archive-snapshot" && method === "POST") {
      const body = JSON.parse(String(init?.body));
      return jsonResponse(
        {
          created_by_user_id: "human-owner-1",
          id: "archive-created",
          included_counts: { evidence: 1, reports: 1 },
          object_ref: body.object_ref,
          project_id: "project-1",
          retained_artifact_ids: [],
          snapshot_type: body.snapshot_type,
          status: "archived",
          summary: body.summary,
          tenant_id: "tenant-1",
        },
        201,
      );
    }
    if (url.pathname === "/api/v1/projects/project-1/archive-preview" && method === "GET") {
      return jsonResponse({
        artifact_count: 1,
        blocked_reasons: [],
        estimated_object_refs: [
          "s3://superteam/project-1/logs.tgz",
          "s3://superteam/project-1/acceptance.md",
        ],
        evidence_count: 1,
        project_id: "project-1",
        report_count: 1,
        retention_pending: false,
      });
    }
    if (
      [
        "/plan-revisions",
        "/evidence",
        "/artifacts",
        "/reports",
        "/budget-ledger",
        "/archive-snapshots",
      ].some((suffix) => url.pathname.endsWith(suffix)) &&
      method === "GET"
    ) {
      return jsonResponse([]);
    }
    if (url.pathname.endsWith("/budget-summary") && method === "GET") {
      return jsonResponse({
        actual_cost: "0",
        actual_tokens: 0,
        estimated_cost: "0",
        estimated_tokens: 0,
        ledger_count: 0,
      });
    }
    if (url.pathname.endsWith("/acceptance") && method === "GET") {
      return jsonResponse({ error: "acceptance not found" }, 404);
    }
    if (url.pathname.endsWith("/archive-preview") && method === "GET") {
      const id = url.pathname.split("/")[4];
      return jsonResponse({
        artifact_count: 0,
        blocked_reasons: [],
        estimated_object_refs: [],
        evidence_count: 0,
        project_id: id,
        report_count: 0,
        retention_pending: false,
      });
    }
    if (
      url.pathname === "/api/v1/projects/project-1/decisions/decision-1/resolve" &&
      method === "POST"
    ) {
      return jsonResponse({
        approval_request_id: "approval-1",
        decision_type: "route_review",
        id: "decision-1",
        project_id: "project-1",
        status_snapshot: "approved",
        target_user_id: "human-owner-1",
        tenant_id: "tenant-1",
        title_snapshot: "需要负责人确认",
      });
    }
    if (
      url.pathname === "/api/v1/projects/project-1/decisions/decision-plan-1/resolve" &&
      method === "POST"
    ) {
      const body = JSON.parse(String(init?.body));
      return jsonResponse({
        approval_request_id: "approval-plan-1",
        coordination_job_id: "job-1",
        decision_type: "plan_review",
        id: "decision-plan-1",
        project_id: "project-1",
        status_snapshot: body.decision,
        target_user_id: "human-owner-1",
        tenant_id: "tenant-1",
        title_snapshot: "计划版本 v2 需要复核",
      });
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
    if (url.pathname === "/api/v1/projects/project-2/archive" && method === "POST") {
      return jsonResponse({ ...projects[1], status: "archived" });
    }
    if (url.pathname === "/api/v1/projects/project-1/delete-preview" && method === "GET") {
      const canDelete = options.deletePreviewCanDelete ?? true;
      return jsonResponse({
        blockers: canDelete
          ? []
          : [
              {
                id: "task-1",
                status: "running",
                title: "运行中的接入任务",
                type: "project_task",
              },
            ],
        can_delete: canDelete,
        message: canDelete
          ? "确认后将终止协调线程并软删除项目。"
          : "该项目仍有进行中的任务，完成或取消后再删除。",
        project_id: "project-1",
        project_name: "客户接入验收",
        warnings: {
          active_member_count: 2,
          digital_employee_member_count: 1,
          pending_decision_count: 1,
          waiting_human_task_count: 1,
        },
      });
    }
    if (url.pathname === "/api/v1/projects/project-1" && method === "DELETE") {
      if (options.deleteStatus === 409) {
        return jsonResponse(
          options.deletePayload ?? {
            blockers: [
              {
                id: "task-1",
                status: "running",
                title: "运行中的接入任务",
                type: "project_task",
              },
            ],
            code: "project_delete_blocked",
            message: "该项目仍有进行中的任务，完成或取消后再删除。",
          },
          409,
        );
      }
      projects.splice(
        projects.findIndex((project) => project.id === "project-1"),
        1,
      );
      return new Response(null, { status: 204 });
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

function pageText() {
  return document.body.textContent ?? "";
}

async function renderProjects(
  fetcher: typeof fetch,
  routeProjectId?: string,
  queryClient = createQueryClient(),
) {
  return await render(
    <QueryClientProvider client={queryClient}>
      <ProjectsView
        apiBaseUrl="http://control-plane.test"
        fetcher={fetcher}
        routeProjectId={routeProjectId}
      />
    </QueryClientProvider>,
  );
}

async function renderProjectCreate(
  fetcher: typeof fetch,
  queryClient = createQueryClient(),
) {
  return await render(
    <QueryClientProvider client={queryClient}>
      <CreateProjectView
        apiBaseUrl="http://control-plane.test"
        fetcher={fetcher}
      />
    </QueryClientProvider>,
  );
}

async function renderProjectRouteSwitcher(
  fetcher: typeof fetch,
  queryClient = createQueryClient(),
) {
  function ProjectRouteSwitcher() {
    const [projectId, setProjectId] = useState("project-1");

    return (
      <>
        <button type="button" onClick={() => setProjectId("project-2")}>
          切换到项目二
        </button>
        <ProjectsView
          apiBaseUrl="http://control-plane.test"
          fetcher={fetcher}
          routeProjectId={projectId}
        />
      </>
    );
  }

  return await render(
    <QueryClientProvider client={queryClient}>
      <ProjectRouteSwitcher />
    </QueryClientProvider>,
  );
}

describe("ProjectsView", () => {
  it("filters project plan revisions to the latest demand", async () => {
    const fetcher = createProjectFetcher();
    await renderProjects(fetcher, "project-1");

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          const target = new URL(String(url));
          return (
            target.pathname === "/api/v1/projects/project-1/plan-revisions" &&
            init?.method === "GET" &&
            target.searchParams.get("demand_id") === "demand-1" &&
            target.searchParams.get("limit") === "10"
          );
        }),
      ).toBe(true);
    });
  });

  it("renders the project homepage as a project-first master-detail triage queue", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeInTheDocument();
    expect(screen.getByTestId("project-risk-queue").element().textContent).toContain(
      "客户接入验收",
    );
    // Portfolio truth bar (loaded-list scope), not the old page-scoped risk metric bar.
    await expect.element(screen.getByLabelText("项目组合概览（已加载列表）")).toBeInTheDocument();
    // 详情层按需渲染：未选中时不保留空态占位栏，队列独占全宽。
    expect(
      screen.container.querySelector('[data-testid="project-selected-context-panel"]'),
    ).toBeNull();
    // Project-first columns surface owner name, risk label and current handler.
    await expect.element(screen.getByText("负责人甲")).toBeInTheDocument();
    await expect.element(screen.getByText("等待人工决策").first()).toBeInTheDocument();
    await expect.element(screen.getByText("验收执行员工")).toBeInTheDocument();
    await expect.element(screen.getByText("运维检索员工")).toBeInTheDocument();
    const queueText = screen.getByTestId("project-risk-queue").element().textContent ?? "";
    // Raw workflow/task detail no longer clutters the row; it moves to project detail.
    expect(queueText).not.toContain("需要负责人确认");
    expect(queueText).not.toContain("数据源: 日志检索");
    expect(queueText).not.toContain("de-1");
    expect(queueText).not.toContain("de-ops-1");

    expect(pageText()).not.toContain("Dispatch gate 技术详情");
    expect(pageText()).not.toContain("路由决策");
    expect(pageText()).not.toContain("协调任务");
    expect(pageText()).not.toContain("高级项目事实");
    expect(pageText()).not.toContain("人类角色");
    expect(pageText()).not.toContain("数字员工池");
    expect(pageText()).not.toContain("协调线程");
  });

  it("links project creation to the standalone route", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect
      .element(screen.getByRole("link", { name: "新建项目" }))
      .toHaveAttribute("href", "/projects/new");
    await expect
      .element(screen.getByRole("link", { exact: true, name: "新建" }))
      .not.toBeInTheDocument();
  });

  it("renders project creation as a standalone page without project management content behind it", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);

    await expect
      .element(screen.getByRole("heading", { name: "新建项目" }))
      .toBeInTheDocument();
    expect(screen.container.querySelector('[data-testid="project-create-page"]')).toBeTruthy();
    expect(screen.container.querySelector('[data-testid="project-risk-queue"]')).toBeNull();
    expect(
      screen.container.querySelector('[aria-label="项目管理指标"]'),
    ).toBeNull();
    await expect
      .element(screen.getByRole("heading", { name: "项目管理" }))
      .not.toBeInTheDocument();
  });

  it("does not count a source team as ready until the user selects one", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith(`/api/auth/users/${CURRENT_USER_ID}/project-team-scopes`) &&
            init?.method === "GET"
          );
        }),
      ).toBe(true);
    });
    await expect.element(screen.getByText("必备项 1 / 3 已就绪")).toBeInTheDocument();
    await expect.element(screen.getByText("未选择")).toBeInTheDocument();

    await userEvent.fill(screen.getByLabelText("项目名称 *"), "授权检查");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "确认来源团队可见");
    await expect.element(screen.getByText("必备项 2 / 3 已就绪")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "平台运营" }));

    await expect.element(screen.getByText("必备项 3 / 3 已就绪")).toBeInTheDocument();
  });

  it("renders the project risk queue as a v3 work surface table", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeInTheDocument();

    const listSurface = screen.container.querySelector('[data-testid="project-risk-queue"]');
    expect(listSurface).toBeTruthy();
    expect(listSurface?.querySelector('[data-slot="v3-work-surface"]')).toBeTruthy();
    expect(listSurface?.querySelector('[data-slot="v3-table"]')).toBeTruthy();
    const headers = Array.from(
      listSurface?.querySelectorAll("thead th") ?? [],
      (header) => header.textContent?.trim(),
    );
    expect(headers).toEqual([
      "项目",
      "待处理",
      "当前处理者",
      "最近活动",
      "操作",
    ]);

    const table = listSurface?.querySelector('[data-slot="v3-table"]');
    expect(table?.className).toContain("table-fixed");
    const columns = Array.from(table?.querySelectorAll("colgroup col") ?? []);
    expect(columns.map((column) => column.getAttribute("data-column"))).toEqual([
      "project",
      "pending",
      "handler",
      "last-run",
      "action",
    ]);
    const projectTitle = listSurface?.querySelector(
      '[data-testid="project-queue-project-title"]',
    );
    expect(projectTitle?.className).toContain("line-clamp-2");
    expect(projectTitle?.className).toContain("break-words");
    expect(projectTitle?.className).toContain("max-h-10");
    expect(
      listSurface?.querySelector('[data-testid="project-queue-pending"]'),
    ).toBeTruthy();
    const handler = listSurface?.querySelector(
      '[data-testid="project-queue-current-handler"]',
    );
    expect(handler?.className).toContain("line-clamp-2");
    expect(handler?.className).toContain("break-words");
    expect(handler?.className).toContain("max-h-10");
  });

  it("splits the projects index into a queue and an on-demand triage panel (wide: in-flow rail)", async () => {
    await page.viewport(1600, 900);
    try {
      const fetcher = createProjectFetcher();
      const screen = await renderProjects(fetcher);

      await expect.element(screen.getByText("项目队列")).toBeInTheDocument();

      const layout = screen.getByTestId("projects-risk-home-layout").element();
      expect(layout.className).toContain("@container/master-detail");
      // 未选中：右栏为驾驶舱面板（待我决策 + 最近运行动态），无 triage 面板。
      expect(layout.firstElementChild?.className).toContain(
        "@5xl/master-detail:grid-cols-[minmax(0,1fr)_minmax(min(100%,18rem),var(--v3-layout-rail-lg))]",
      );
      await vi.waitFor(() => {
        expect(
          document.querySelector('[data-testid="projects-dashboard-rail"]'),
        ).toBeTruthy();
      });
      const rail = document.querySelector(
        '[data-testid="projects-dashboard-rail"]',
      ) as HTMLElement;
      await vi.waitFor(() => {
        expect(rail.textContent).toContain("待我决策");
        expect(rail.textContent).toContain("需要负责人确认上线计划");
        expect(rail.textContent).toContain("生产环境发布审批");
        expect(rail.textContent).toContain("最近运行动态");
      });
      // 决策行深链到项目审批 Tab
      const decisionLink = Array.from(rail.querySelectorAll("a")).find((anchor) =>
        anchor.textContent?.includes("需要负责人确认上线计划"),
      );
      expect(decisionLink?.getAttribute("href")).toContain("/projects/project-1");
      expect(decisionLink?.getAttribute("href")).toContain("tab=approval");
      expect(decisionLink?.getAttribute("href")).toContain("focus=decision-1");
      // KPI 带新增「待我决策」真值卡（inbox badge）
      const kpiBand = screen.getByLabelText("项目组合概览（已加载列表）").element();
      expect(kpiBand.textContent).toContain("待我决策");
      expect(
        document.querySelector('[data-testid="project-selected-context-panel"]'),
      ).toBeNull();

      // Selecting the top queue row (project-1) swaps the rail to the in-flow triage panel
      // with deep-link actions, reusing the already-fetched page risk signals.
      await userEvent.click(screen.getByTestId("project-queue-project-title").first());

      await vi.waitFor(() => {
        expect(
          document.querySelector('[data-testid="project-selected-context-panel"]'),
        ).toBeTruthy();
      });
      expect(layout.firstElementChild?.className).toContain(
        "@5xl/master-detail:grid-cols-[minmax(0,1fr)_minmax(min(100%,18rem),var(--v3-layout-rail-lg))]",
      );
      // 宽容器下面板在栅格内 in-flow，不走 Sheet 抽屉。
      expect(document.querySelector('[data-slot="sheet-content"]')).toBeNull();

      const panel = document.querySelector(
        '[data-testid="project-selected-context-panel"]',
      ) as HTMLElement;
      await vi.waitFor(() => {
        expect(panel.textContent).toContain("客户接入验收");
        expect(panel.textContent).toContain("处理决策");
      });
      // 右栏受视口高度约束、内部滚动：长待办/证据列表不得把页面撑长。
      expect(panel.className).toContain("@5xl/master-detail:max-h-[calc(100svh-2rem)]");
      expect(panel.className).toContain("@5xl/master-detail:overflow-y-auto");
      const decisionAction = Array.from(panel.querySelectorAll("a")).find((anchor) =>
        anchor.textContent?.includes("处理决策"),
      );
      expect(decisionAction?.getAttribute("href")).toContain("tab=approval");

      // 选中期间驾驶舱面板让位给 triage；点关闭钮返回驾驶舱右栏。
      expect(
        document.querySelector('[data-testid="projects-dashboard-rail"]'),
      ).toBeNull();
      await userEvent.click(
        screen.getByRole("button", { name: "关闭项目待办详情" }),
      );
      await vi.waitFor(() => {
        expect(
          document.querySelector('[data-testid="project-selected-context-panel"]'),
        ).toBeNull();
        expect(
          document.querySelector('[data-testid="projects-dashboard-rail"]'),
        ).toBeTruthy();
      });
    } finally {
      await page.viewport(414, 896);
    }
  });

  it("opens the triage panel as a right sheet on narrow containers and clears selection on dismiss", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeInTheDocument();
    expect(document.querySelector('[data-slot="sheet-content"]')).toBeNull();

    await userEvent.click(screen.getByTestId("project-queue-project-title").first());

    await vi.waitFor(() => {
      const sheet = document.querySelector('[data-slot="sheet-content"]');
      expect(sheet).toBeTruthy();
      expect(sheet?.textContent).toContain("客户接入验收");
    });

    const close = document.querySelector(
      '[data-slot="sheet-content"] button',
    ) as HTMLElement;
    close.click();
    await vi.waitFor(() => {
      expect(document.querySelector('[data-slot="sheet-content"]')).toBeNull();
      expect(
        document.querySelector('[data-testid="project-selected-context-panel"]'),
      ).toBeNull();
    });
  });

  it("keeps the projects index free of horizontal overflow across viewport tiers", async () => {
    try {
      const fetcher = createProjectFetcher();
      const screen = await renderProjects(fetcher);
      await expect.element(screen.getByText("项目队列")).toBeInTheDocument();

      for (const [width, height] of [
        [1280, 900],
        [1536, 960],
        [1920, 1080],
      ] as const) {
        await page.viewport(width, height);
        await userEvent.click(screen.getByTestId("project-queue-project-title").first());
        await vi.waitFor(() => {
          expect(
            document.querySelector('[data-testid="project-selected-context-panel"]'),
          ).toBeTruthy();
        });
        const doc = document.documentElement;
        expect(
          doc.scrollWidth,
          `viewport ${width} 不应出现横向溢出`,
        ).toBeLessThanOrEqual(doc.clientWidth);
        const panel = document.querySelector(
          '[data-testid="project-selected-context-panel"]',
        ) as HTMLElement;
        expect(
          Math.round(panel.getBoundingClientRect().right),
          `viewport ${width} 右栏必须完整可见`,
        ).toBeLessThanOrEqual(width);
      }
    } finally {
      await page.viewport(414, 896);
    }
  });

  it("keeps the project detail route full-width for the plan graph", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();

    const layout = screen.getByTestId("projects-risk-home-layout").element();
    expect(layout.className).not.toContain("grid-cols");
    expect(screen.container.querySelector('[data-testid="project-risk-queue"]')).toBeNull();
    expect(screen.container.querySelector('[data-testid="plan-graph-canvas"]')).toBeTruthy();
  });

  it("keeps project filtering and v3 pagination controls working", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeInTheDocument();
    expect(screen.container.querySelector('[data-testid="project-risk-queue"]')?.textContent).toContain(
      "客户接入验收",
    );
    expect(
      screen.container.querySelector('[data-testid="project-risk-queue"]')?.textContent,
    ).toContain("生产巡检整改");

    await userEvent.fill(screen.getByLabelText("搜索项目"), "巡检");

    await vi.waitFor(() => {
      const listText = screen.container.querySelector('[data-testid="project-risk-queue"]')?.textContent;
      expect(listText).toContain("生产巡检整改");
      expect(listText).not.toContain("客户接入验收");
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          const target = new URL(String(url));
          return (
            target.pathname === "/api/v1/projects" &&
            init?.method === "GET" &&
            target.searchParams.get("q") === "巡检"
          );
        }),
      ).toBe(true);
    });
  });

  it("shows the latest pre-dispatch gate status for a selected project task", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");

    await expect.element(screen.getByText("当前阻塞")).toBeInTheDocument();
    await expect.element(screen.getByText("运行节点暂不可用，系统会稍后重试")).toBeInTheDocument();
    expect(pageText()).not.toContain("Dispatch gate 技术详情");
    expect(pageText()).not.toContain("runtime.node_offline");

    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

    await expect.element(screen.getByText("Dispatch gate 技术详情")).toBeInTheDocument();
    await expect
      .element(screen.getByText("稍后重试", { exact: true }))
      .toBeInTheDocument();
    await expect.element(screen.getByText("runtime.node_offline")).toBeInTheDocument();

    await vi.waitFor(() => {
      const calls = fetchCalls(fetcher);
      expect(
        calls.some(([url, init]) => {
          return (
            String(url).endsWith(
              "/api/v1/projects/project-1/tasks/task-1/dispatch-gates",
            ) && init?.method === "GET"
          );
        }),
      ).toBe(true);
      expect(
        calls.some(([url]) =>
          String(url).endsWith(
            "/api/v1/projects/project-1/tasks/task-old/dispatch-gates",
          ),
        ),
      ).toBe(false);
    });
  });

  it("shows missing runtime placement readiness with a bind action", async () => {
    const fetcher = createProjectFetcher({ runtimeReadinessStatus: "missing" });
    const screen = await renderProjects(fetcher, "project-1");

    await expect.element(screen.getByText("运行落点")).toBeInTheDocument();
    await expect.element(screen.getByText("缺少运行节点")).toBeInTheDocument();
    await expect.element(screen.getByText("项目尚未绑定运行节点")).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "绑定运行节点" })).toBeInTheDocument();
    await expect
      .element(screen.getByRole("option", { name: "local-dev-node · 在线" }))
      .toBeInTheDocument();
    await expect.element(screen.getByText("codex")).toBeInTheDocument();
  });

  it("binds the selected runtime node and refreshes placement-dependent project data", async () => {
    const fetcher = createProjectFetcher({ runtimeReadinessStatus: "missing" });
    const screen = await renderProjects(fetcher, "project-1");

    await userEvent.selectOptions(screen.getByLabelText("选择运行节点"), "runtime-node-1");
    await userEvent.click(screen.getByRole("button", { name: "绑定运行节点" }));

    await vi.waitFor(() => {
      const putCall = fetchCalls(fetcher).find(([url, init]) => {
        return (
          String(url).endsWith("/api/v1/projects/project-1/runtime-nodes/runtime-node-1") &&
          init?.method === "PUT"
        );
      });
      expect(putCall).toBeTruthy();
    });

    await vi.waitFor(() => {
      const calls = fetchCalls(fetcher);
      expect(
        calls.filter(([url, init]) => {
          const target = new URL(String(url));
          return (
            target.pathname === "/api/v1/projects/project-1/runtime-readiness" &&
            init?.method === "GET"
          );
        }).length,
      ).toBeGreaterThanOrEqual(2);
      expect(
        calls.filter(([url, init]) => {
          const target = new URL(String(url));
          return (
            target.pathname === "/api/v1/projects/project-1/task-graph" &&
            init?.method === "GET"
          );
        }).length,
      ).toBeGreaterThanOrEqual(2);
      expect(
        calls.filter(([url, init]) => {
          const target = new URL(String(url));
          return (
            target.pathname === "/api/v1/projects/project-1/events" &&
            init?.method === "GET"
          );
        }).length,
      ).toBeGreaterThanOrEqual(2);
      expect(
        calls.filter(([url, init]) => {
          const target = new URL(String(url));
          return (
            target.pathname === "/api/v1/projects/project-1/overview" &&
            init?.method === "GET"
          );
        }).length,
      ).toBeGreaterThanOrEqual(2);
    });
  });

  it("does not show stale runtime readiness actions while switching projects", async () => {
    const project2RuntimeReadiness = makeDeferred<Response>();
    const fetcher = createProjectFetcher({
      project2RuntimeReadinessGate: project2RuntimeReadiness.promise.then(() => undefined),
      runtimeReadinessStatus: "ready",
    });
    const screen = await renderProjectRouteSwitcher(fetcher);

    await expect.element(screen.getByText("已就绪")).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "释放绑定" })).toBeEnabled();

    await userEvent.click(screen.getByRole("button", { name: "切换到项目二" }));

    await expect
      .element(screen.getByRole("heading", { name: "生产巡检整改" }))
      .toBeInTheDocument();
    await expect.element(screen.getByText("等待检查")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "释放绑定" }))
      .toBeDisabled();

    project2RuntimeReadiness.resolve(jsonResponse({}));
  });

  it("treats missing project acceptance as an optional empty state", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-2");

    await expect
      .element(screen.getByRole("heading", { name: "生产巡检整改" }))
      .toBeInTheDocument();

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith("/api/v1/projects/project-2/acceptance") &&
            init?.method === "GET"
          );
        }),
      ).toBe(true);
    });
  });

  it("renders governance tabs and archive preview metrics", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");
    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

    await expect.element(screen.getByRole("tab", { name: "证据链" })).toBeInTheDocument();
    await expect.element(screen.getByRole("tab", { name: "预算流水" })).toBeInTheDocument();
    await expect.element(screen.getByRole("tab", { name: "验收结论" })).toBeInTheDocument();
    await expect.element(screen.getByRole("tab", { name: "归档预览" })).toBeInTheDocument();
    await expect.element(screen.getByText("上线验收证据")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "归档预览" }));

    await vi.waitFor(() => {
      expect(screen.container.textContent).toContain("需求数");
      expect(screen.container.textContent).toContain("保留工件");
      expect(screen.container.textContent).toContain("当前项目");
    });
  });

  it("does not surface audit and cost links in the default project hero", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");

    await expect
      .element(screen.getByRole("link", { name: "配置项目" }))
      .toHaveAttribute("href", "/projects/project-1/config");

    const visibleLinkLabels = Array.from(screen.container.querySelectorAll("a")).map((link) =>
      link.textContent?.trim(),
    );
    expect(visibleLinkLabels).not.toContain("审计");
    expect(visibleLinkLabels).not.toContain("成本");
  });

  it("creates and verifies project evidence from the governance tab", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");
    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

    await userEvent.fill(screen.getByLabelText("证据标题"), "补充验收附件");
    await userEvent.fill(
      screen.getByLabelText("来源引用"),
      "s3://superteam/project-1/additional.md",
    );
    await userEvent.click(screen.getByRole("button", { name: "新增证据" }));
    await userEvent.click(screen.getByRole("button", { name: "标记已验证：上线验收证据" }));

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith("/api/v1/projects/project-1/evidence") &&
            init?.method === "POST" &&
            JSON.parse(String(init.body)).title === "补充验收附件"
          );
        }),
      ).toBe(true);
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith("/api/v1/projects/project-1/evidence/evidence-1") &&
            init?.method === "PATCH" &&
            JSON.parse(String(init.body)).verification_status === "verified"
          );
        }),
      ).toBe(true);
    });
  });

  it("submits project acceptance and creates an archive snapshot", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");
    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

    await userEvent.click(screen.getByRole("tab", { name: "验收结论" }));
    await userEvent.fill(
      screen.getByRole("textbox", { name: "验收结论" }),
      "证据完整，同意归档",
    );
    await userEvent.click(screen.getByRole("button", { name: "提交验收" }));

    await userEvent.click(screen.getByRole("tab", { name: "归档预览" }));
    await userEvent.fill(
      screen.getByLabelText("快照 Object Ref"),
      "s3://superteam/project-1/final-archive.json",
    );
    await userEvent.click(screen.getByRole("button", { name: "生成归档快照" }));

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith("/api/v1/projects/project-1/acceptance") &&
            init?.method === "POST" &&
            JSON.parse(String(init.body)).conclusion === "证据完整，同意归档"
          );
        }),
      ).toBe(true);
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith("/api/v1/projects/project-1/archive-snapshot") &&
            init?.method === "POST" &&
            JSON.parse(String(init.body)).object_ref ===
              "s3://superteam/project-1/final-archive.json"
          );
        }),
      ).toBe(true);
    });
  });

  it("fetches current user and authorized project teams on the create route", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "授权检查");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "确认来源团队可见");

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("button", { name: "平台运营" })).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "风控审查" })).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "已撤销团队" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "停用团队" })).not.toBeInTheDocument();

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return String(url).endsWith("/api/auth/me") && init?.method === "GET";
        }),
      ).toBe(true);
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith(`/api/auth/users/${CURRENT_USER_ID}/project-team-scopes`) &&
            init?.method === "GET"
          );
        }),
      ).toBe(true);
    });
  });

  it("refetches cached authorization and blocks stale team submit while scopes refresh", async () => {
    const deferred = makeDeferred<Response>();
    const fetcher = createProjectFetcher({
      projectTeamScopesDeferred: deferred,
      projectTeamScopesStatus: "loading",
    });
    const queryClient = createQueryClient({ staleTime: 10_000 });
    queryClient.setQueryData(["auth", "current-user", "project-create"], {
      user: {
        avatar: { provider: "dicebear", seed: "current-user", style: "adventurer" },
        avatar_asset_id: null,
        display_name: "当前用户",
        email: "current@example.com",
        id: CURRENT_USER_ID,
        status: "active",
        username: "current-user",
      },
    });
    queryClient.setQueryData(
      ["auth", "users", CURRENT_USER_ID, "project-team-scopes", "project-create"],
      userProjectTeamScopesResponse(),
    );
    const screen = await renderProjectCreate(fetcher, queryClient);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "缓存授权项目");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "等待授权范围刷新后才能提交");

    try {
      await userEvent.click(screen.getByRole("button", { name: "下一步" }));
      await userEvent.click(screen.getByRole("button", { name: "下一步" }));
      await userEvent.click(screen.getByRole("button", { name: "下一步" }));
      await userEvent.click(screen.getByRole("button", { name: "下一步" }));
      await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith(`/api/auth/users/${CURRENT_USER_ID}/project-team-scopes`) &&
            init?.method === "GET"
          );
        }),
      ).toBe(true);
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return String(url).endsWith("/api/v1/projects") && init?.method === "POST";
        }),
      ).toBe(false);
    } finally {
      deferred.resolve(jsonResponse(userProjectTeamScopesResponse()));
    }
  });

  it("creates a project from the split console with human owners, source teams, employees, and policies", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "客户验收推进");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "完成客户验收闭环");

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.fill(screen.getByLabelText("搜索项目人类负责人"), "李娜");
    await userEvent.click(screen.getByRole("button", { name: "选择 leader" }).first());

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "平台运营" }));
    await expect.element(screen.getByText("平台值守员")).toBeInTheDocument();
    await expect.element(screen.getByText("研发助手")).not.toBeInTheDocument();
    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          const target = new URL(String(url));
          return (
            target.pathname === "/api/v1/digital-employees" &&
            target.searchParams.get("team_id") === TEAM_AUTHORIZED_ID &&
            init?.method === "GET"
          );
        }),
      ).toBe(true);
    });

    await userEvent.click(screen.getByRole("button", { name: "风控审查" }));
    await userEvent.click(screen.getByRole("button", { name: "平台运营" }));
    await expect.element(screen.getByText("平台值守员")).not.toBeInTheDocument();
    await expect.element(screen.getByText("研发助手")).toBeInTheDocument();
    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          const target = new URL(String(url));
          return (
            target.pathname === "/api/v1/digital-employees" &&
            target.searchParams.get("team_id") === TEAM_REVIEW_ID &&
            init?.method === "GET"
          );
        }),
      ).toBe(true);
    });

    await userEvent.click(screen.getByText("研发助手"));
    await userEvent.click(screen.getByText("测试工程师"));

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByRole("checkbox", { name: "local-dev-node" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("checkbox", { name: "local-dev-node" }));

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: /高风险审批/ }));
    await userEvent.click(screen.getByRole("button", { name: "创建项目", exact: true }));

    await vi.waitFor(() => {
      const postCall = fetchCalls(fetcher).find(([url, init]) => {
        return String(url).endsWith("/api/v1/projects") && init?.method === "POST";
      });
      expect(postCall).toBeTruthy();
      const body = JSON.parse(String(postCall?.[1]?.body));
      expect(body).toMatchObject({
        human_owner_user_id: LEADER_USER_ID,
        human_owner_user_ids: [LEADER_USER_ID],
        name: "客户验收推进",
        team_id: TEAM_REVIEW_ID,
        runtime_node_ids: ["runtime-node-1"],
        approval_policy: {
          budget_overrun_requires_owner_approval: true,
          high_risk_action_requires_confirmation: true,
          new_demand_requires_human_confirmation: true,
          preset: "highRisk",
        },
        evidence_policy: {
          acceptance_requires_evidence: true,
          preset: "highRisk",
        },
      });
      expect(body).not.toHaveProperty("leader_user_id");
      expect(body).not.toHaveProperty("acceptance_user_id");
      expect(body.members).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            principal_id: LEADER_USER_ID,
            principal_type: "human_user",
            project_role: "owner",
          }),
          expect.objectContaining({
            principal_id: EMPLOYEE_ASSISTANT_ID,
            principal_type: "digital_employee",
            project_role: "executor",
          }),
        ]),
      );
      expect(body.members).not.toEqual(
        expect.arrayContaining([
          expect.objectContaining({ principal_type: "team" }),
          expect.objectContaining({ project_role: "leader" }),
          expect.objectContaining({ project_role: "acceptance" }),
          expect.objectContaining({ project_role: "reviewer" }),
        ]),
      );
    });
  });

  it("lists runtime nodes and lets the user select and deselect them", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "节点选择校验");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "验证可运行节点多选");

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByText("选择可运行节点")).toBeInTheDocument();
    const nodeCheckbox = screen.getByRole("checkbox", { name: "local-dev-node" });
    await expect.element(nodeCheckbox).toBeInTheDocument();
    await expect.element(nodeCheckbox).toHaveAttribute("aria-checked", "false");
    await expect.element(screen.getByText(/负载 0\/2/)).toBeInTheDocument();

    await userEvent.click(nodeCheckbox);
    await expect.element(nodeCheckbox).toHaveAttribute("aria-checked", "true");

    await userEvent.click(nodeCheckbox);
    await expect.element(nodeCheckbox).toHaveAttribute("aria-checked", "false");
  });

  it("requires selecting at least one runtime node before creating a project", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "缺少运行节点");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "未选择运行节点时不能创建项目");

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "平台运营" }));

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByRole("checkbox", { name: "local-dev-node" })).toHaveAttribute("aria-checked", "false");

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
    expect(
      fetchCalls(fetcher).some(([url, init]) => {
        return String(url).endsWith("/api/v1/projects") && init?.method === "POST";
      }),
    ).toBe(false);
  });

  it("does not offer unauthorized teams as digital-employee source teams", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "越权团队项目");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "尝试提交未授权团队");

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("button", { name: "平台运营" })).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "未授权团队" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
    expect(
      fetchCalls(fetcher).some(([url, init]) => {
        return String(url).endsWith("/api/v1/projects") && init?.method === "POST";
      }),
    ).toBe(false);
  });

  it("keeps source teams cleared after authorization scopes refresh", async () => {
    const fetcher = createProjectFetcher();
    const queryClient = createQueryClient();
    const screen = await renderProjectCreate(fetcher, queryClient);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "清空来源团队");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "刷新授权范围后仍保持清空");

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByRole("button", { name: "平台运营" })).toHaveAttribute("aria-pressed", "false");
    await expect.element(screen.getByText("请先选择数字员工来源团队")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "平台运营" }));
    await expect.element(screen.getByText("平台值守员")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "平台运营" }));
    await expect.element(screen.getByRole("button", { name: "平台运营" })).toHaveAttribute("aria-pressed", "false");
    const refreshedScopes = userProjectTeamScopesResponse();
    refreshedScopes.items[0].updated_at = "2026-06-04T02:29:13Z";
    queryClient.setQueryData(
      ["auth", "users", CURRENT_USER_ID, "project-team-scopes", "project-create"],
      refreshedScopes,
    );
    await expect.element(screen.getByRole("button", { name: "平台运营" })).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
    expect(
      fetchCalls(fetcher).some(([url, init]) => {
        return String(url).endsWith("/api/v1/projects") && init?.method === "POST";
      }),
    ).toBe(false);
  });

  it("shows an empty state and disables project creation when no teams are selectable", async () => {
    const fetcher = createProjectFetcher({ projectTeamScopesStatus: "empty" });
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "客户验收推进");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "完成客户验收闭环");

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByText("暂无可选团队")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
  });

  it("keeps project creation disabled while the current user is loading", async () => {
    const deferred = makeDeferred<Response>();
    const fetcher = createProjectFetcher({
      currentUserDeferred: deferred,
      currentUserStatus: "loading",
    });
    const screen = await renderProjectCreate(fetcher);

    try {
      await userEvent.click(screen.getByRole("button", { name: "下一步" }));
      await expect.element(screen.getByText("正在加载当前用户...")).toBeInTheDocument();
    } finally {
      deferred.resolve(
        jsonResponse({
          user: {
            avatar: { provider: "dicebear", seed: "current-user", style: "adventurer" },
            avatar_asset_id: null,
            display_name: "当前用户",
            email: "current@example.com",
            id: CURRENT_USER_ID,
            status: "active",
            username: "current-user",
          },
        }),
      );
    }
  });

  it("keeps project creation disabled when the current user fails to load", async () => {
    const fetcher = createProjectFetcher({ currentUserStatus: "error" });
    const screen = await renderProjectCreate(fetcher);

    await expect.element(screen.getByText("加载当前用户失败")).toBeInTheDocument();
  });

  it("keeps project creation disabled while selectable teams are loading", async () => {
    const deferred = makeDeferred<Response>();
    const fetcher = createProjectFetcher({
      projectTeamScopesDeferred: deferred,
      projectTeamScopesStatus: "loading",
    });
    const screen = await renderProjectCreate(fetcher);

    try {
      await userEvent.fill(screen.getByLabelText("项目名称 *"), "等待团队");
      await userEvent.fill(screen.getByLabelText("项目目标 *"), "团队加载中");
      await userEvent.click(screen.getByRole("button", { name: "下一步" }));
      await userEvent.click(screen.getByRole("button", { name: "下一步" }));
      await userEvent.click(screen.getByRole("button", { name: "下一步" }));
      await userEvent.click(screen.getByRole("button", { name: "下一步" }));
      await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
    } finally {
      deferred.resolve(jsonResponse(userProjectTeamScopesResponse()));
    }
  });

  it("keeps project creation disabled when selectable teams fail to load", async () => {
    const fetcher = createProjectFetcher({ projectTeamScopesStatus: "error" });
    const screen = await renderProjectCreate(fetcher);

    await expect.element(screen.getByText("加载可选团队失败")).toBeInTheDocument();
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

    await expect.element(screen.getByText("项目队列")).toBeInTheDocument();
    await userEvent.fill(screen.getByLabelText("搜索项目"), "巡检");

    const queueText = screen.getByTestId("project-risk-queue").element().textContent ?? "";
    expect(queueText).toContain("客户接入验收");
    expect(queueText).toContain("生产巡检整改");
  });

  it("requires confirmation before posting to the archive route", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");

    const archivePosts = () =>
      fetchCalls(fetcher).filter(([url, init]) => {
        return (
          String(url).endsWith("/api/v1/projects/project-1/archive") &&
          init?.method === "POST"
        );
      });

    // 取消路径：打开弹窗后取消，不得发出归档请求。
    await userEvent.click(screen.getByRole("button", { name: "归档项目" }));
    await expect
      .element(screen.getByRole("heading", { name: "归档项目" }))
      .toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(archivePosts()).toHaveLength(0);

    await userEvent.click(screen.getByRole("button", { name: "归档项目" }));
    await userEvent.click(screen.getByRole("button", { name: "确认归档" }));

    await vi.waitFor(() => {
      const archiveCall = archivePosts()[0];
      expect(archiveCall).toBeTruthy();
      expect(archiveCall?.[1]?.body).toBeUndefined();
      expect(archiveCall?.[1]?.headers).toEqual({ accept: "application/json" });
    });
  });

  it("shows 已归档 instead of 待调度 for archived projects in the queue handler column", async () => {
    const fetcher = createProjectFetcher({ includeArchivedProject: true });
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeInTheDocument();
    await expect
      .element(screen.getByTestId("project-risk-queue").getByText("已归档走查项目"))
      .toBeInTheDocument();

    await vi.waitFor(() => {
      const rows = Array.from(
        screen
          .getByTestId("project-risk-queue")
          .element()
          .querySelectorAll("tbody tr"),
      );
      const archivedRow = rows.find((row) =>
        row.textContent?.includes("已归档走查项目"),
      );
      expect(archivedRow).toBeTruthy();
      const handlerCell = archivedRow?.querySelector(
        '[data-testid="project-queue-current-handler"]',
      );
      expect(handlerCell?.textContent).toBe("已归档");
    });
  });

  it("renders route decisions, transfer requests, and resolves pending human decisions", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");

    await expect.element(screen.getByText("待负责人处理").first()).toBeInTheDocument();
    await expect.element(screen.getByText("需要负责人确认")).toBeInTheDocument();
    expect(pageText()).not.toContain("路由决策");
    expect(pageText()).not.toContain("转派请求");

    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

    await expect.element(screen.getByText("路由决策")).toBeInTheDocument();
    await expect
      .element(screen.getByText("选择项目数字员工池中的 active executor"))
      .toBeInTheDocument();
    await expect.element(screen.getByText("转派请求")).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: "批准 需要负责人确认" }),
    );

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith(
              "/api/v1/projects/project-1/decisions/decision-1/resolve",
            ) &&
            init?.method === "POST" &&
            JSON.parse(String(init.body)).decision === "approved"
          );
        }),
      ).toBe(true);
    });
  });

  it("does not mount project detail actions on the projects index", async () => {
    let releaseProject2Overview!: () => void;
    const project2OverviewGate = new Promise<void>((resolve) => {
      releaseProject2Overview = resolve;
    });
    const fetcher = createProjectFetcher({ project2OverviewGate });
    const screen = await renderProjects(fetcher);

    await expect
      .element(screen.getByTestId("project-risk-queue").getByText("生产巡检整改"))
      .toBeInTheDocument();
    // Selecting a project mounts the lightweight triage panel, but it must not
    // mount the full operational detail: no per-project overview fetch, no archive action.
    await userEvent.click(
      screen.getByTestId("project-risk-queue").getByText("生产巡检整改"),
    );
    await vi.waitFor(() => {
      expect(
        document.querySelector('[data-testid="project-selected-context-panel"]'),
      ).toBeTruthy();
    });
    expect(
      fetchCalls(fetcher).some(([url]) =>
        String(url).includes("/api/v1/projects/project-2/overview"),
      ),
    ).toBe(false);
    releaseProject2Overview();

    await expect
      .element(screen.getByRole("button", { name: "归档项目" }))
      .not.toBeInTheDocument();
  });

  it("surfaces current-page risk signals in the project queue", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "执行失败" }))
      .toBeVisible();

    const queue = screen.getByTestId("project-risk-queue");
    const queueText = (await queue.element().textContent) ?? "";
    expect(queueText).toContain("生产巡检整改");
    expect(queueText).toContain("客户接入验收");
    expect(queueText).toContain("执行失败");
  });

  it("keeps the queue in a risk-identifying state until current-page risk signals settle", async () => {
    const project2RiskGate = makeDeferred<void>();
    const fetcher = createProjectFetcher({
      project2RiskSignalGate: project2RiskGate.promise,
    });
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeVisible();
    // Portfolio bar is derived from cheap loaded-list fields, so it shows immediately.
    await expect.element(screen.getByLabelText("项目组合概览（已加载列表）")).toBeVisible();

    const pendingQueue = screen.getByTestId("project-risk-queue").element();
    const pendingRowsText = pendingQueue.querySelector("tbody")?.textContent ?? "";
    expect(pendingQueue.textContent).toContain("正在识别风险");
    expect(pendingRowsText).toContain("识别中");
    expect(pendingRowsText).not.toContain("等待人工决策");
    expect(pendingRowsText).not.toContain("执行失败");

    project2RiskGate.resolve();

    await vi.waitFor(() => {
      const settledQueue = screen.getByTestId("project-risk-queue").element();
      const settledRowsText = settledQueue.querySelector("tbody")?.textContent ?? "";
      expect(settledRowsText).toContain("执行失败");
      expect(settledRowsText).toContain("等待人工决策");
      expect(settledQueue.textContent).not.toContain("正在识别风险");
    });
  });

  it("filters the queue by risk category without changing the base project list request", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "执行失败" }));

    const queueText = screen.getByTestId("project-risk-queue").element().textContent ?? "";
    expect(queueText).toContain("生产巡检整改");
    expect(queueText).not.toContain("客户接入验收");

    const projectListCalls = fetchCalls(fetcher).filter(([input, init]) => {
      const url = new URL(String(input));
      return url.pathname === "/api/v1/projects" && init?.method === "GET";
    });
    expect(projectListCalls.length).toBeGreaterThan(0);
    expect(
      projectListCalls.every(([input]) => {
        const url = new URL(String(input));
        return !url.searchParams.has("risk");
      }),
    ).toBe(true);
  });

  it("makes project detail the primary row action and workflow orchestration secondary", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeVisible();
    await expect
      .element(screen.getByRole("link", { name: "进入项目 生产巡检整改" }))
      .toHaveAttribute("href", "/projects/project-2");
    // 详情层按需渲染：未选中时页面上没有 triage 面板。
    expect(
      document.querySelector('[data-testid="project-selected-context-panel"]'),
    ).toBeNull();
  });

  it("keeps the full operational detail on project detail routes", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher, "project-1");

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeVisible();
    await expect.element(screen.getByText("整理接入证据")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "提交需求" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "归档项目" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));
    await expect.element(screen.getByText("路由决策")).toBeVisible();
    await expect.element(screen.getByText("协调任务")).toBeVisible();
    expect(screen.container.querySelector('[aria-label="选中项目上下文"]')).toBeNull();
  });

  it("requires the project name before deleting and redirects after success", async () => {
    const fetcher = createProjectFetcher({
      projectAllowedActions: ["project.archive", "project.delete"],
    });
    const screen = await renderProjects(fetcher, "project-1");

    await userEvent.click(screen.getByRole("button", { name: "删除项目" }));
    await expect.element(screen.getByRole("heading", { name: "删除项目" })).toBeVisible();
    await expect.element(screen.getByText("仍有 1 项待处理决策")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "确认删除" })).toBeDisabled();

    await userEvent.fill(screen.getByLabelText("输入项目名称确认删除"), "客户");
    await expect.element(screen.getByRole("button", { name: "确认删除" })).toBeDisabled();

    await userEvent.fill(screen.getByLabelText("输入项目名称确认删除"), "客户接入验收");
    await userEvent.click(screen.getByRole("button", { name: "确认删除" }));

    await expect
      .poll(() =>
        fetchCalls(fetcher).some(([url, init]) => {
          const target = new URL(String(url));
          return target.pathname === "/api/v1/projects/project-1" && init?.method === "DELETE";
        }),
      )
      .toBe(true);
    expect(routerMock.navigate).toHaveBeenCalledWith({ to: "/projects" });
  });

  it("disables delete confirmation when preview reports blockers", async () => {
    const fetcher = createProjectFetcher({
      deletePreviewCanDelete: false,
      projectAllowedActions: ["project.archive", "project.delete"],
    });
    const screen = await renderProjects(fetcher, "project-1");

    await userEvent.click(screen.getByRole("button", { name: "删除项目" }));
    await expect.element(screen.getByText("删除被阻断")).toBeVisible();
    await expect.element(screen.getByText("运行中的接入任务")).toBeVisible();

    await userEvent.fill(screen.getByLabelText("输入项目名称确认删除"), "客户接入验收");
    await expect.element(screen.getByRole("button", { name: "确认删除" })).toBeDisabled();
  });

  it("keeps the dialog open and renders delete blockers on 409", async () => {
    const fetcher = createProjectFetcher({
      deleteStatus: 409,
      projectAllowedActions: ["project.archive", "project.delete"],
    });
    const screen = await renderProjects(fetcher, "project-1");

    await userEvent.click(screen.getByRole("button", { name: "删除项目" }));
    await userEvent.fill(screen.getByLabelText("输入项目名称确认删除"), "客户接入验收");
    await userEvent.click(screen.getByRole("button", { name: "确认删除" }));

    await expect.element(screen.getByText("该项目仍有进行中的任务，完成或取消后再删除。")).toBeVisible();
    await expect.element(screen.getByText("运行中的接入任务")).toBeVisible();
    await expect.element(screen.getByText("项目任务 · 运行中")).toBeVisible();
    expect(routerMock.navigate).not.toHaveBeenCalled();
  });

  it("keeps the base project list usable when one project's risk enrichment fails", async () => {
    const fetcher = createProjectFetcher({ riskSignalFailureProjectId: "project-2" });
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeVisible();
    await expect
      .element(screen.getByTestId("project-risk-queue").getByText("生产巡检整改"))
      .toBeVisible();
    await expect.element(screen.getByText("风险待确认")).toBeVisible();
    const queueText = screen.getByTestId("project-risk-queue").element().textContent ?? "";
    expect(queueText).toContain("进入项目");
  });

  it("renders the project index with the compact control surface structure", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("项目队列")).toBeVisible();

    expect(screen.getByTestId("projects-compact-control-surface").element()).toBeVisible();
    expect(screen.getByTestId("project-risk-queue").element()).toHaveAttribute(
      "data-density",
      "compact",
    );
    expect(screen.getByTestId("project-risk-queue").element().textContent).toContain(
      "按需要介入程度排序",
    );
  });

  it("renders project creation with a compact workbench and tabular review", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);

    await expect.element(screen.getByRole("heading", { name: "新建项目工作台" })).toBeVisible();

    expect(screen.getByTestId("project-create-page").element()).toHaveAttribute(
      "data-variant",
      "compact-control-surface",
    );
    expect(screen.getByTestId("project-create-review-table").element()).toBeVisible();
    expect(screen.getByTestId("project-create-review-table").element().textContent).toContain(
      "项目事实",
    );
  });
});
