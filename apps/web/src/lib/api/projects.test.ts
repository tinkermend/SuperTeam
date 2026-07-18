import { describe, expect, it, vi } from "vitest";
import {
  archiveProject,
  createProject,
  createProjectEvidence,
  deleteProject,
  getProjectArchivePreview,
  getProjectBudgetSummary,
  getProjectConfig,
  getProjectConfigRevision,
  getProjectDeletePreview,
  getProjectDemandLaunchDetail,
  getProjectOverview,
  getProjectPlanRevision,
  addProjectRuntimeNode,
  getProjectRuntimeReadiness,
  getProjectTaskGraph,
  listProjectTaskDispatchGates,
  listProjectConfigRevisions,
  listProjectEvidence,
  listProjectPlanRevisions,
  listProjectRouteDecisions,
  listWorkflowInstances,
  patchProjectEvidence,
  removeProjectRuntimeNode,
  replaceProjectMembers,
  resolveProjectDecision,
  submitProjectDemand,
} from "@/lib/api/projects";
import type {
  ProjectTaskGraph,
  WorkflowInstanceSummary,
} from "@/lib/api/projects";

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
      runtime_node_ids: ["44444444-4444-4444-8444-444444444444"],
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

  it("addProjectRuntimeNode puts node binding with reason", async () => {
    const binding = { runtime_node_id: "runtime-node-1" };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(binding), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      addProjectRuntimeNode(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        "runtime-node-1",
        { reason: "manual assignment" },
      ),
    ).resolves.toEqual(binding);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/runtime-nodes/runtime-node-1",
      {
        body: JSON.stringify({ reason: "manual assignment" }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "PUT",
      },
    );
  });

  it("removeProjectRuntimeNode deletes node binding", async () => {
    const fetcher = vi.fn(async () => new Response(null, { status: 204 }));

    await expect(
      removeProjectRuntimeNode(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        "runtime-node-1",
      ),
    ).resolves.toBeUndefined();

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/runtime-nodes/runtime-node-1",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "DELETE",
      },
    );
  });

  it("getProjectRuntimeReadiness encodes project id", async () => {
    const readiness = {
      placement_status: "ready",
      runtime_node_id: "runtime-node-1",
      runtime_node_name: "Runtime Node 1",
      command_channel_connected: true,
      provider_capabilities: ["codex", "claude"],
      required_provider_types: ["codex"],
      employee_readiness: [
        {
          digital_employee_id: "employee-1",
          display_name: "Planner",
          provider_type: "codex",
          can_plan: true,
          can_dispatch: true,
        },
      ],
      blocking_reasons: [],
      next_actions: [],
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(readiness), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getProjectRuntimeReadiness(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
      ),
    ).resolves.toEqual(readiness);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/runtime-readiness",
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

  it("lists workflow instances with encoded filters", async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify([]), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      listWorkflowInstances(
        { baseUrl: "http://control-plane.local", fetcher },
        {
          limit: 25,
          offset: 5,
          projectId: "project 1/primary",
          q: "支付 巡检",
          status: "running",
        },
      ),
    ).resolves.toEqual([]);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/workflow-instances?q=%E6%94%AF%E4%BB%98+%E5%B7%A1%E6%A3%80&project_id=project+1%2Fprimary&status=running&limit=25&offset=5",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("lists workflow instances with optional read model fields", async () => {
    const workflowInstances = [
      {
        demand_id: "55555555-5555-4555-8555-555555555555",
        project_id: "11111111-1111-4111-8111-111111111111",
        project_name: "客户接入",
        title: "完成接入验收",
        submitted_by_user_id: "33333333-3333-4333-8333-333333333333",
        submitted_by_display_name: "负责人",
        status: "waiting_human",
        status_reason: "等待负责人决策",
        created_at: "2026-06-16T01:00:00Z",
        updated_at: "2026-06-16T01:10:00Z",
        selected_coordination_job_id: "66666666-6666-4666-8666-666666666666",
        progress: {
          total_nodes: 3,
          completed_nodes: 1,
          running_nodes: 1,
          blocked_nodes: 0,
          waiting_human_nodes: 1,
          planned_nodes: 1,
          failed_nodes: 0,
          cancelled_nodes: 0,
        },
        current_blocker: {
          type: "decision_request",
          title: "确认上线窗口",
          resource_id: "77777777-7777-4777-8777-777777777777",
        },
        priority: {
          value: "p1",
          label: "P1",
          source: "policy",
        },
        risk: {
          level: "high",
          label: "高风险",
          source: "task_graph",
        },
        sla: {
          remaining_seconds: 1500,
          breached: false,
          label: "25 分钟",
          source: "sla_policy",
        },
        recent_event: {
          event_type: "decision.requested",
          summary: "需要负责人确认",
          occurred_at: "2026-06-16T01:09:00Z",
        },
      },
    ] satisfies WorkflowInstanceSummary[];
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(workflowInstances), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    const result = await listWorkflowInstances({
      baseUrl: "http://control-plane.local",
      fetcher,
    });

    expect(result[0]?.priority?.label).toBe("P1");
    expect(result[0]?.risk?.level).toBe("high");
    expect(result[0]?.sla?.remaining_seconds).toBe(1500);
    expect(result[0]?.recent_event?.event_type).toBe("decision.requested");
    expect(result[0]?.progress.planned_nodes).toBe(1);
    expect(result[0]?.progress.failed_nodes).toBe(0);
    expect(result[0]?.progress.cancelled_nodes).toBe(0);
    expect(result[0]?.current_blocker?.type).toBe("decision_request");
    expect(result[0]?.current_blocker?.title).toBe("确认上线窗口");
    expect(result[0]?.current_blocker?.resource_id).toBe(
      "77777777-7777-4777-8777-777777777777",
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/workflow-instances",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("gets project task graph by demand id", async () => {
    const graph = {
      nodes: [
        {
          id: "88888888-8888-4888-8888-888888888888",
          tenant_id: "22222222-2222-4222-8222-222222222222",
          project_id: "11111111-1111-4111-8111-111111111111",
          demand_id: "55555555-5555-4555-8555-555555555555",
          title: "确认上线窗口",
          status: "waiting_human",
          status_reason: "等待负责人决策",
          updated_at: "2026-06-16T01:10:00Z",
          requires_human_approval: true,
          stage_index: 1,
          expected_outputs: ["decision"],
          input_requirements: { approval: true },
          handoff_contract: { receiver: "human_owner" },
          planner_metadata: { source: "workflow_read_model" },
          current_blocker: {
            type: "decision_request",
            title: "确认上线窗口",
            resource_id: "77777777-7777-4777-8777-777777777777",
          },
        },
      ],
      edges: [],
      employees: [],
      runs: [],
      execution_summaries: [],
      recent_events: [],
      decision_requests: [],
      blocking_facts: [],
      stage_summaries: [
        {
          stage_index: 1,
          title: "验收",
          total_nodes: 2,
          completed_nodes: 1,
          running_nodes: 0,
          waiting_human_nodes: 1,
          blocked_nodes: 0,
        },
      ],
    } satisfies ProjectTaskGraph;
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(graph), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    const result = await getProjectTaskGraph(
      { baseUrl: "http://control-plane.local", fetcher },
      "project 1/primary",
      { demandId: "demand 1/primary" },
    );

    expect(result).toEqual(graph);
    expect(result.stage_summaries?.[0]).toMatchObject({
      stage_index: 1,
      title: "验收",
      total_nodes: 2,
      completed_nodes: 1,
      running_nodes: 0,
      waiting_human_nodes: 1,
      blocked_nodes: 0,
    });
    expect(result.nodes[0]?.status_reason).toBe("等待负责人决策");
    expect(result.nodes[0]?.updated_at).toBe("2026-06-16T01:10:00Z");
    expect(result.nodes[0]?.current_blocker?.type).toBe("decision_request");
    expect(result.nodes[0]?.current_blocker?.title).toBe("确认上线窗口");
    expect(result.nodes[0]?.current_blocker?.resource_id).toBe(
      "77777777-7777-4777-8777-777777777777",
    );

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/task-graph?demand_id=demand+1%2Fprimary",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("lists project task dispatch gates", async () => {
    const response = {
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
          id: "00000000-0000-0000-0000-000000000401",
          project_task_id: "00000000-0000-0000-0000-000000000402",
          retry_after: "2026-06-21T12:02:00Z",
          selected_employee_id: "00000000-0000-0000-0000-000000000403",
          status: "retry_later",
        },
      ],
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(response), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listProjectTaskDispatchGates(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        "task 1/primary",
      ),
    ).resolves.toEqual(response);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/tasks/task%201%2Fprimary/dispatch-gates",
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

  it("deletes a project with encoded project id", async () => {
    const fetcher = vi.fn(async () => new Response(null, { status: 204 }));

    await expect(
      deleteProject(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
      ),
    ).resolves.toBeUndefined();

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "DELETE",
      },
    );
  });

  it("surfaces project delete blockers from the API payload", async () => {
    const payload = {
      code: "project_delete_blocked",
      message: "该项目仍有进行中的任务，完成或取消后再删除。",
      blockers: [
        {
          type: "project_task",
          id: "task-1",
          status: "running",
          title: "接入验收",
        },
      ],
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(payload), {
          status: 409,
          headers: { "content-type": "application/json" },
        }),
    );

    await expect(
      deleteProject(
        { baseUrl: "http://control-plane.local", fetcher },
        "project-1",
      ),
    ).rejects.toMatchObject({
      status: 409,
      code: "project_delete_blocked",
      payload,
    });
  });

  it("gets project delete preview with encoded project id", async () => {
    const deletePreview = {
      project_id: "11111111-1111-4111-8111-111111111111",
      project_name: "客户接入",
      can_delete: false,
      blockers: [
        {
          type: "project_task",
          id: "task-1",
          status: "running",
          title: "接入验收",
        },
        {
          type: "run",
          id: "99999999-9999-4999-8999-999999999999",
          status: "running",
          title: "接入验收执行",
        },
      ],
      warnings: {
        pending_decision_count: 1,
        waiting_human_task_count: 2,
        open_inbox_count: 0,
        active_member_count: 3,
        digital_employee_member_count: 1,
        runtime_node_binding_count: 1,
        affinity_count: 0,
      },
      message: "该项目仍有进行中的任务，完成或取消后再删除。",
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(deletePreview), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getProjectDeletePreview(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
      ),
    ).resolves.toEqual(deletePreview);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/delete-preview",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
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

  it("lists and gets project plan revisions", async () => {
    const revision = {
      id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      project_id: "11111111-1111-4111-8111-111111111111",
      demand_id: "55555555-5555-4555-8555-555555555555",
      revision_number: 2,
      status: "pending_review",
      payload: { summary: "复核生产巡检计划" },
      plan_fingerprint: "fingerprint",
      validation_errors: [],
      validation_warnings: [],
      review_required: true,
      created_task_ids: [],
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify([revision]), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listProjectPlanRevisions(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        { demandId: "demand 1", limit: 5 },
      ),
    ).resolves.toEqual([revision]);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/plan-revisions?demand_id=demand+1&limit=5",
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
      getProjectPlanRevision(
        { baseUrl: "http://control-plane.local", fetcher },
        "project 1/primary",
        "revision 1",
      ),
    ).resolves.toEqual(revision);

    expect(fetcher).toHaveBeenLastCalledWith(
      "http://control-plane.local/api/v1/projects/project%201%2Fprimary/plan-revisions/revision%201",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
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
