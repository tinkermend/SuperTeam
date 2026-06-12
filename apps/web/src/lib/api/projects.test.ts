import { describe, expect, it, vi } from "vitest";
import {
  archiveProject,
  createProject,
  createProjectEvidence,
  getProjectArchivePreview,
  getProjectBudgetSummary,
  getProjectConfig,
  getProjectConfigRevision,
  getProjectDemandLaunchDetail,
  getProjectOverview,
  listProjectConfigRevisions,
  listProjectEvidence,
  listProjectRouteDecisions,
  patchProjectEvidence,
  replaceProjectMembers,
  resolveProjectDecision,
  submitProjectDemand,
} from "./projects";

const project = {
  id: "11111111-1111-4111-8111-111111111111",
  tenant_id: "22222222-2222-4222-8222-222222222222",
  name: "客户接入",
  goal: "完成 Runtime 接入验收",
  status: "running",
  human_owner_user_id: "33333333-3333-4333-8333-333333333333",
  coordination_workflow_id:
    "project-coordinator:11111111-1111-4111-8111-111111111111",
  coordination_status: "registered",
  coordination_policy: { cadence: "daily" },
  approval_policy: {},
  evidence_policy: {},
};

const ownerMember = {
  id: "44444444-4444-4444-8444-444444444444",
  tenant_id: "22222222-2222-4222-8222-222222222222",
  project_id: "11111111-1111-4111-8111-111111111111",
  principal_type: "human_user",
  principal_id: "33333333-3333-4333-8333-333333333333",
  project_role: "owner",
  status: "active",
  settings: {},
};

describe("project API", () => {
  it("creates project with JSON body and cookie credentials", async () => {
    const response = { project, members: [ownerMember] };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(response), {
          headers: { "content-type": "application/json" },
          status: 201,
        }),
    );
    const input = {
      name: "客户接入",
      goal: "完成 Runtime 接入验收",
      human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      members: [
        {
          principal_type: "human_user" as const,
          principal_id: "33333333-3333-4333-8333-333333333333",
          project_role: "owner" as const,
        },
      ],
      coordination_policy: { cadence: "daily" },
    };

    await expect(
      createProject(
        { baseUrl: "http://control-plane.local", fetcher },
        input,
      ),
    ).resolves.toEqual(response);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects",
      {
        body: JSON.stringify(input),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });

  it("gets project overview with encoded project id", async () => {
    const overview = {
      project,
      human_roles: [ownerMember],
      digital_employee_pool: [],
      status_summary: { current_phase: "running", is_archived: false },
      task_summary: {
        active_tasks: 0,
        pending_human_tasks: 0,
        completed_tasks: 0,
        failed_tasks: 0,
      },
      active_tasks: [],
      recent_events: [],
      coordination_workflow: {
        workflow_id:
          "project-coordinator:11111111-1111-4111-8111-111111111111",
        status: "registered",
      },
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(overview), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getProjectOverview(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
      ),
    ).resolves.toEqual(overview);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/overview",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("gets current project config shape from config path", async () => {
    const config = {
      project,
      human_roles: [ownerMember],
      digital_employee_pool: [],
      members: [ownerMember],
      coordination_policy: { cadence: "daily" },
      approval_policy: {},
      evidence_policy: {},
      coordination_workflow: {
        workflow_id:
          "project-coordinator:11111111-1111-4111-8111-111111111111",
        status: "registered",
      },
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(config), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getProjectConfig(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
      ),
    ).resolves.toEqual(config);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/config",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("submits demand with source refs and attachments", async () => {
    const demand = {
      id: "55555555-5555-4555-8555-555555555555",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      project_id: "11111111-1111-4111-8111-111111111111",
      submitted_by_user_id: "33333333-3333-4333-8333-333333333333",
      title: "补充验收证据",
      content: "上传执行日志",
      source_type: "manual",
      source_refs: { ticket: "SUP-1" },
      attachments: ["s3://bucket/log.txt"],
      status: "recorded",
      reviewer: null,
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(demand), {
          headers: { "content-type": "application/json" },
          status: 201,
        }),
    );
    const input = {
      title: "补充验收证据",
      content: "上传执行日志",
      source_refs: { ticket: "SUP-1" },
      attachments: ["s3://bucket/log.txt"],
    };

    await expect(
      submitProjectDemand(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        input,
      ),
    ).resolves.toEqual(demand);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/demands",
      {
        body: JSON.stringify(input),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });

  it("submits demand with reviewer preference", async () => {
    const demand = {
      id: "55555555-5555-4555-8555-555555555555",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      project_id: "11111111-1111-4111-8111-111111111111",
      submitted_by_user_id: "33333333-3333-4333-8333-333333333333",
      title: "审查 PR",
      content: "统计并审查 PR",
      source_type: "manual",
      source_refs: {},
      attachments: [],
      status: "planning_pending",
      reviewer: {
        reviewer_user_id: "33333333-3333-4333-8333-333333333333",
        selection_reason: "user_selected",
        project_role: "reviewer",
        resolved_from_rule: false,
      },
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(demand), {
          headers: { "content-type": "application/json" },
          status: 201,
        }),
    );
    const input = {
      title: "审查 PR",
      content: "统计并审查 PR",
      reviewer_user_id: "33333333-3333-4333-8333-333333333333",
      reviewer_selection_reason: "user_selected" as const,
    };

    await expect(
      submitProjectDemand(
        { baseUrl: "http://control-plane.local", fetcher },
        "project-1",
        input,
      ),
    ).resolves.toEqual(demand);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project-1/demands",
      expect.objectContaining({
        body: JSON.stringify(input),
        method: "POST",
      }),
    );
  });

  it("gets project demand launch detail with encoded demand id", async () => {
    const demand = {
      id: "55555555-5555-4555-8555-555555555555",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      project_id: "11111111-1111-4111-8111-111111111111",
      submitted_by_user_id: "33333333-3333-4333-8333-333333333333",
      title: "补充验收证据",
      source_type: "manual",
      source_refs: {},
      attachments: [],
      status: "recorded",
      reviewer: null,
    };
    const detail = {
      demand,
      project,
      reviewer: null,
      coordination_jobs: [],
      route_decisions: [],
      project_tasks: [],
      decision_requests: [],
      recent_events: [],
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(detail), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getProjectDemandLaunchDetail(
        { baseUrl: "http://control-plane.local", fetcher },
        "demand 1/primary",
      ),
    ).resolves.toEqual(detail);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/project-demands/demand%201%2Fprimary/launch-detail",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("replaces members through members wrapper", async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify([ownerMember]), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );
    const members = [
      {
        principal_type: "human_user" as const,
        principal_id: "33333333-3333-4333-8333-333333333333",
        project_role: "owner" as const,
        settings: { notifications: true },
      },
    ];

    await expect(
      replaceProjectMembers(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        members,
      ),
    ).resolves.toEqual([ownerMember]);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/members",
      {
        body: JSON.stringify({ members }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "PUT",
      },
    );
  });

  it("archives project through archive route", async () => {
    const archived = { ...project, status: "archived" };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(archived), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      archiveProject(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
      ),
    ).resolves.toEqual(archived);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/archive",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "POST",
      },
    );
  });

  it("lists route decisions and resolves project decisions", async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify([]), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listProjectRouteDecisions(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        { limit: 10 },
      ),
    ).resolves.toEqual([]);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/route-decisions?limit=10",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );

    fetcher.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ id: "decision-1", status_snapshot: "approved" }),
        {
          headers: { "content-type": "application/json" },
          status: 200,
        },
      ),
    );

    await resolveProjectDecision(
      { baseUrl: "http://control-plane.local", fetcher },
      "project 1/primary",
      "decision 1",
      { decision: "approved", comment: "同意继续" },
    );

    expect(fetcher).toHaveBeenLastCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/decisions/decision%201/resolve",
      expect.objectContaining({
        body: JSON.stringify({ decision: "approved", comment: "同意继续" }),
        method: "POST",
      }),
    );
  });

  it("creates evidence with encoded project id and cookie credentials", async () => {
    const evidence = {
      evidence_type: "test_report",
      id: "66666666-6666-4666-8666-666666666666",
      metadata: { suite: "regression" },
      project_id: "11111111-1111-4111-8111-111111111111",
      source_ref: "s3://bucket/report.md",
      source_type: "s3",
      submitted_by_type: "human_user",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      title: "回归测试报告",
      verification_status: "submitted",
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(evidence), {
          headers: { "content-type": "application/json" },
          status: 201,
        }),
    );
    const input = {
      evidence_type: "test_report",
      metadata: { suite: "regression" },
      source_ref: "s3://bucket/report.md",
      source_type: "s3",
      title: "回归测试报告",
    };

    await expect(
      createProjectEvidence(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        input,
      ),
    ).resolves.toEqual(evidence);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/evidence",
      {
        body: JSON.stringify(input),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });

  it("lists and patches project evidence through V2 evidence routes", async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify([]), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listProjectEvidence(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        { limit: 20, offset: 5, status: "verified" },
      ),
    ).resolves.toEqual([]);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/evidence?limit=20&offset=5&status=verified",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );

    const patched = {
      evidence_type: "test_report",
      id: "evidence 1",
      metadata: { reviewer: "owner" },
      project_id: "11111111-1111-4111-8111-111111111111",
      source_ref: "s3://bucket/report.md",
      source_type: "s3",
      submitted_by_type: "human_user",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      title: "回归测试报告",
      verification_status: "verified",
    };
    fetcher.mockResolvedValueOnce(
      new Response(JSON.stringify(patched), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      patchProjectEvidence(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        "evidence 1",
        { metadata: { reviewer: "owner" }, verification_status: "verified" },
      ),
    ).resolves.toEqual(patched);

    expect(fetcher).toHaveBeenLastCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/evidence/evidence%201",
      expect.objectContaining({
        body: JSON.stringify({
          metadata: { reviewer: "owner" },
          verification_status: "verified",
        }),
        credentials: "include",
        method: "PATCH",
      }),
    );
  });

  it("gets archive preview and budget summary with Task 6 response fields", async () => {
    const archivePreview = {
      artifact_count: 1,
      blocked_reasons: [],
      estimated_object_refs: ["s3://bucket/final.md"],
      evidence_count: 2,
      project_id: "11111111-1111-4111-8111-111111111111",
      report_count: 1,
      retention_pending: false,
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(archivePreview), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getProjectArchivePreview(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
      ),
    ).resolves.toEqual(archivePreview);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/archive-preview",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );

    const budgetSummary = {
      actual_cost: "0.80",
      actual_tokens: 800,
      estimated_cost: "1.00",
      estimated_tokens: 1000,
      ledger_count: 1,
    };
    fetcher.mockResolvedValueOnce(
      new Response(JSON.stringify(budgetSummary), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      getProjectBudgetSummary(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
      ),
    ).resolves.toEqual(budgetSummary);

    expect(fetcher).toHaveBeenLastCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/budget-summary",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("lists and gets project config revisions", async () => {
    const revision = {
      changed_sections: ["approval_policy"],
      config_snapshot: { approval_policy: { high_risk: "human" } },
      created_by_user_id: "33333333-3333-4333-8333-333333333333",
      diff_summary: { approval_policy: "changed" },
      id: "revision 1",
      project_id: "11111111-1111-4111-8111-111111111111",
      revision_number: 2,
      tenant_id: "22222222-2222-4222-8222-222222222222",
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify([revision]), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listProjectConfigRevisions(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        { limit: 10 },
      ),
    ).resolves.toEqual([revision]);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/config-revisions?limit=10",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );

    fetcher.mockResolvedValueOnce(
      new Response(JSON.stringify(revision), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      getProjectConfigRevision(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        "revision 1",
      ),
    ).resolves.toEqual(revision);

    expect(fetcher).toHaveBeenLastCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/config-revisions/revision%201",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });
});
