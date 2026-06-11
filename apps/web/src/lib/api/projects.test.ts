import { describe, expect, it, vi } from "vitest";
import {
  archiveProject,
  createProject,
  getProjectConfig,
  getProjectOverview,
  listProjectRouteDecisions,
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
        body: JSON.stringify({}),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
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
});
