import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectsView } from "@/features/projects";
import type { Project } from "@/lib/api/projects";

const CURRENT_USER_ID = "current-user-1";
const TEAM_AUTHORIZED_ID = "team-authorized-1";
const TEAM_REVIEW_ID = "team-review-1";
const TEAM_REVOKED_ID = "team-revoked-1";
const TEAM_DISABLED_ID = "team-disabled-1";
const TEAM_UNAUTHORIZED_ID = "team-unauthorized-1";

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
      const path = params?.projectId
        ? to.replace("$projectId", encodeURIComponent(params.projectId))
        : to;
      const query = search ? `?${new URLSearchParams(search).toString()}` : "";

      return (
        <a {...props} href={`${path}${query}`} ref={ref}>
          {children}
        </a>
      );
    },
  );
  Link.displayName = "MockRouterLink";

  return { Link };
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

function createProjectFetcher(
  options: {
    currentUserDeferred?: ReturnType<typeof makeDeferred<Response>>;
    currentUserStatus?: "default" | "error" | "loading";
    projectTeamScopesDeferred?: ReturnType<typeof makeDeferred<Response>>;
    projectTeamScopesStatus?: "default" | "empty" | "error" | "loading";
    project2OverviewGate?: Promise<void>;
    slowFilteredList?: boolean;
  } = {},
) {
  const projects = [
    makeProject("project-1", "客户接入验收"),
    makeProject("project-2", "生产巡检整改"),
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
      created.acceptance_user_id = body.acceptance_user_id;
      created.description = body.description;
      created.goal = body.goal ?? created.goal;
      created.human_owner_user_id = body.human_owner_user_id ?? created.human_owner_user_id;
      created.leader_user_id = body.leader_user_id;
      projects.unshift(created);
      return jsonResponse({
        members: [],
        project: created,
      });
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

    if (url.pathname.endsWith("/tasks") && method === "GET") {
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

  it("renders the project list and selected overview", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();
    // `当前需求`/`当前执行`/`待负责人处理` each render in both a metric tile and
    // a panel header, so these locators resolve to multiple elements; use
    // `.first()` to avoid strict-locator multi-match failures.
    await expect.element(screen.getByText("当前需求").first()).toBeInTheDocument();
    await expect.element(screen.getByText("补充上线验收说明").first()).toBeInTheDocument();
    await expect.element(screen.getByText("当前执行").first()).toBeInTheDocument();
    await expect.element(screen.getByText("整理接入证据").first()).toBeInTheDocument();
    await expect.element(screen.getByText("待负责人处理").first()).toBeInTheDocument();
    await expect.element(screen.getByText("需要负责人确认")).toBeInTheDocument();
    await expect.element(screen.getByText("项目负责人组")).toBeInTheDocument();
    await expect.element(screen.getByText("负责人甲")).toBeInTheDocument();
    await expect.element(screen.getByText("负责人乙")).toBeInTheDocument();
    await expect.element(screen.getByText("项目服务池")).toBeInTheDocument();
    await expect.element(screen.getByText("验收执行员工")).toBeInTheDocument();
    await expect.element(screen.getByText("最新结果")).toBeInTheDocument();
    await expect.element(screen.getByText("证据充分")).toBeInTheDocument();
    await expect.element(screen.getByText("高级项目事实")).toBeInTheDocument();

    expect(pageText()).not.toContain("Dispatch gate 技术详情");
    expect(pageText()).not.toContain("路由决策");
    expect(pageText()).not.toContain("协调任务");
    expect(pageText()).not.toContain("人类角色");
    expect(pageText()).not.toContain("数字员工池");
    expect(pageText()).not.toContain("协调线程");

    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

    await expect.element(screen.getByText("路由决策")).toBeInTheDocument();
    await expect.element(screen.getByText("协调任务")).toBeInTheDocument();
    // Execution trace panel keeps its existing title `执行证据链`; this plan does
    // not rename it.
    await expect
      .element(screen.getByRole("heading", { name: "执行证据链" }))
      .toBeInTheDocument();
    expect(pageText()).toContain("runtime.node_offline");
    expect(pageText()).toContain("runtime.inspect、codebase.analysis");

    await userEvent.click(
      screen.getByRole("button", { name: "要求修改计划版本 v2" }),
    );

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith(
              "/api/v1/projects/project-1/decisions/decision-plan-1/resolve",
            ) &&
            init?.method === "POST" &&
            JSON.parse(String(init.body)).decision === "request_changes"
          );
        }),
      ).toBe(true);
    });
  });

  it("renders the project list as a v3 work surface table", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();

    const listSurface = screen.container.querySelector('[data-testid="projects-v3-list"]');
    expect(listSurface).toBeTruthy();
    expect(listSurface?.querySelector('[data-slot="v3-work-surface"]')).toBeTruthy();
    expect(listSurface?.querySelector('[data-slot="v3-table"]')).toBeTruthy();
    expect(listSurface?.querySelectorAll("thead th").length).toBeGreaterThanOrEqual(5);
  });

  it("keeps project filtering and v3 pagination controls working", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await expect
      .element(screen.getByRole("heading", { name: "客户接入验收" }))
      .toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText("每页条数"), "1");

    expect(screen.container.querySelector('[data-testid="projects-v3-list"]')?.textContent).toContain(
      "客户接入验收",
    );
    expect(
      screen.container.querySelector('[data-testid="projects-v3-list"]')?.textContent,
    ).not.toContain("生产巡检整改");

    await userEvent.click(screen.getByRole("button", { name: "下一页" }));

    await vi.waitFor(() => {
      const listText = screen.container.querySelector('[data-testid="projects-v3-list"]')?.textContent;
      expect(listText).toContain("生产巡检整改");
      expect(listText).not.toContain("客户接入验收");
    });

    await userEvent.fill(screen.getByLabelText("搜索项目"), "巡检");

    await vi.waitFor(() => {
      const listText = screen.container.querySelector('[data-testid="projects-v3-list"]')?.textContent;
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
    await expect.element(screen.getByText("Retry later")).toBeInTheDocument();
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

  it("fetches current user and authorized project teams when opening the create drawer", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "新建项目" }));

    await expect.element(screen.getByLabelText("可选团队")).toBeInTheDocument();
    await expect.element(screen.getByRole("option", { name: "平台运营" })).toBeInTheDocument();
    await expect.element(screen.getByRole("option", { name: "风控审查" })).toBeInTheDocument();
    await expect.element(screen.getByRole("option", { name: "已撤销团队" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("option", { name: "停用团队" })).not.toBeInTheDocument();

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
    const screen = await renderProjects(fetcher, undefined, queryClient);

    await userEvent.click(screen.getByRole("button", { name: "新建项目" }));
    await userEvent.fill(screen.getByLabelText("项目名称"), "缓存授权项目");
    await userEvent.fill(screen.getByLabelText("目标"), "等待授权范围刷新后才能提交");

    try {
      await expect.element(screen.getByText("正在加载可选团队...")).toBeInTheDocument();
      await expect.element(screen.getByRole("button", { name: "创建项目" })).toBeDisabled();
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

  it("creates a project with a selected authorized team and the current user as owner", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "新建项目" }));
    await userEvent.fill(screen.getByLabelText("项目名称"), "客户验收推进");
    await userEvent.fill(screen.getByLabelText("目标"), "完成客户验收闭环");
    await userEvent.selectOptions(screen.getByLabelText("可选团队"), TEAM_REVIEW_ID);
    await userEvent.fill(screen.getByLabelText("Leader 用户 ID"), "leader-user-id");
    await userEvent.fill(screen.getByLabelText("验收人用户 ID"), "acceptance-user-id");
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }));

    await vi.waitFor(() => {
      const postCall = fetchCalls(fetcher).find(([url, init]) => {
        return String(url).endsWith("/api/v1/projects") && init?.method === "POST";
      });
      expect(postCall).toBeTruthy();
      expect(JSON.parse(String(postCall?.[1]?.body))).toMatchObject({
        acceptance_user_id: "acceptance-user-id",
        human_owner_user_id: CURRENT_USER_ID,
        leader_user_id: "leader-user-id",
        name: "客户验收推进",
        team_id: TEAM_REVIEW_ID,
      });
    });
  });

  it("does not submit a project when the selected team is not authorized", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjects(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "新建项目" }));
    await expect.element(screen.getByLabelText("可选团队")).toBeInTheDocument();

    const teamSelect = document.querySelector(
      'select[aria-label="可选团队"]',
    ) as HTMLSelectElement | null;
    expect(teamSelect).toBeTruthy();

    const unauthorizedOption = document.createElement("option");
    unauthorizedOption.value = TEAM_UNAUTHORIZED_ID;
    unauthorizedOption.textContent = "未授权团队";
    teamSelect?.append(unauthorizedOption);

    await userEvent.fill(screen.getByLabelText("项目名称"), "越权团队项目");
    await userEvent.fill(screen.getByLabelText("目标"), "尝试提交未授权团队");
    await userEvent.selectOptions(screen.getByLabelText("可选团队"), TEAM_UNAUTHORIZED_ID);

    await expect.element(screen.getByRole("button", { name: "创建项目" })).toBeDisabled();
    expect(
      fetchCalls(fetcher).some(([url, init]) => {
        return String(url).endsWith("/api/v1/projects") && init?.method === "POST";
      }),
    ).toBe(false);
  });

  it("shows an empty state and disables project creation when no teams are selectable", async () => {
    const fetcher = createProjectFetcher({ projectTeamScopesStatus: "empty" });
    const screen = await renderProjects(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "新建项目" }));
    await userEvent.fill(screen.getByLabelText("项目名称"), "客户验收推进");
    await userEvent.fill(screen.getByLabelText("目标"), "完成客户验收闭环");

    await expect.element(screen.getByText("暂无可选团队")).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "创建项目" })).toBeDisabled();
  });

  it("keeps project creation disabled while the current user is loading", async () => {
    const deferred = makeDeferred<Response>();
    const fetcher = createProjectFetcher({
      currentUserDeferred: deferred,
      currentUserStatus: "loading",
    });
    const screen = await renderProjects(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "新建项目" }));

    try {
      await expect.element(screen.getByText("正在加载当前用户...")).toBeInTheDocument();
      await expect.element(screen.getByRole("button", { name: "创建项目" })).toBeDisabled();
    } finally {
      deferred.resolve(jsonResponse({
        user: {
          avatar: { provider: "dicebear", seed: "current-user", style: "adventurer" },
          avatar_asset_id: null,
          display_name: "当前用户",
          email: "current@example.com",
          id: CURRENT_USER_ID,
          status: "active",
          username: "current-user",
        },
      }));
    }
  });

  it("keeps project creation disabled when the current user fails to load", async () => {
    const fetcher = createProjectFetcher({ currentUserStatus: "error" });
    const screen = await renderProjects(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "新建项目" }));

    await expect.element(screen.getByText("加载当前用户失败")).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "创建项目" })).toBeDisabled();
  });

  it("keeps project creation disabled while selectable teams are loading", async () => {
    const deferred = makeDeferred<Response>();
    const fetcher = createProjectFetcher({
      projectTeamScopesDeferred: deferred,
      projectTeamScopesStatus: "loading",
    });
    const screen = await renderProjects(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "新建项目" }));

    try {
      await expect.element(screen.getByText("正在加载可选团队...")).toBeInTheDocument();
      await expect.element(screen.getByRole("button", { name: "创建项目" })).toBeDisabled();
    } finally {
      deferred.resolve(jsonResponse(userProjectTeamScopesResponse()));
    }
  });

  it("keeps project creation disabled when selectable teams fail to load", async () => {
    const fetcher = createProjectFetcher({ projectTeamScopesStatus: "error" });
    const screen = await renderProjects(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "新建项目" }));

    await expect.element(screen.getByText("加载可选团队失败")).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "创建项目" })).toBeDisabled();
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

    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));
    await userEvent.click(screen.getByRole("button", { name: "归档项目" }));

    await vi.waitFor(() => {
      const archiveCall = fetchCalls(fetcher).find(([url, init]) => {
        return (
          String(url).endsWith("/api/v1/projects/project-1/archive") &&
          init?.method === "POST"
        );
      });
      expect(archiveCall).toBeTruthy();
      expect(archiveCall?.[1]?.body).toBeUndefined();
      expect(archiveCall?.[1]?.headers).toEqual({ accept: "application/json" });
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
      screen.getByRole("button", { name: "批准：需要负责人确认" }),
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

  it("does not keep stale project detail actions after selecting another project", async () => {
    let releaseProject2Overview!: () => void;
    const project2OverviewGate = new Promise<void>((resolve) => {
      releaseProject2Overview = resolve;
    });
    const fetcher = createProjectFetcher({ project2OverviewGate });
    const screen = await renderProjects(fetcher);

    await expect.element(screen.getByText("需要负责人确认")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /生产巡检整改/ }));

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url]) =>
          String(url).includes("/api/v1/projects/project-2/overview"),
        ),
      ).toBe(true);
    });

    try {
      const detailHeadings = Array.from(
        screen.container.querySelectorAll("h2"),
        (heading) => heading.textContent?.trim(),
      );
      expect(detailHeadings).toContain("生产巡检整改");
      expect(
        screen.container.querySelector(
          'button[aria-label="批准：需要负责人确认"]',
        ),
      ).toBeNull();
    } finally {
      releaseProject2Overview();
    }

    await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));
    await userEvent.click(screen.getByRole("button", { name: "归档项目" }));

    await vi.waitFor(() => {
      expect(
        fetchCalls(fetcher).some(([url, init]) => {
          return (
            String(url).endsWith("/api/v1/projects/project-2/archive") &&
            init?.method === "POST"
          );
        }),
      ).toBe(true);
    });
  });
});
