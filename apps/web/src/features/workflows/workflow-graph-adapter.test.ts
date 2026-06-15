import { describe, expect, it } from "vitest";

import {
  buildWorkflowGraphElements,
  selectInitialWorkflowNodeId,
} from "./workflow-graph-adapter";
import type { WorkflowTaskNodeData } from "./workflow-graph-adapter";
import type { ProjectTaskGraph } from "@/lib/api/projects";

function makeGraph(): ProjectTaskGraph {
  return {
    nodes: [
      {
        id: "task-root",
        tenant_id: "tenant-1",
        project_id: "project-1",
        demand_id: "demand-1",
        title: "任务入口",
        summary: "整理需求和上下文",
        status: "completed",
        assigned_digital_employee_id: "employee-1",
        requires_human_approval: false,
        stage_index: 1,
        expected_outputs: ["需求清单"],
        input_requirements: {},
        handoff_contract: {},
        planner_metadata: {},
      },
      {
        id: "task-run",
        tenant_id: "tenant-1",
        project_id: "project-1",
        demand_id: "demand-1",
        title: "服务健康巡检",
        summary: "检查 payment-api 和队列",
        status: "assigned",
        assigned_digital_employee_id: "employee-2",
        requires_human_approval: true,
        stage_index: 2,
        expected_outputs: ["服务健康报告"],
        input_requirements: { scope: "payment" },
        handoff_contract: {},
        planner_metadata: {},
      },
    ],
    edges: [
      {
        blocker_task_id: "task-root",
        dependent_task_id: "task-run",
        edge_status: "unblocked",
      },
    ],
    employees: [
      {
        digital_employee_id: "employee-1",
        display_name: "需求分析师",
        project_role: "executor",
        status: "active",
      },
      {
        digital_employee_id: "employee-2",
        display_name: "应用运维工程师",
        project_role: "executor",
        status: "active",
      },
    ],
    runs: [
      {
        project_task_id: "task-run",
        runtime_node_summary: "runtime-east-1",
        status: "assigned",
        provider_type: "codex",
      },
    ],
    execution_summaries: [],
    recent_events: [],
    decision_requests: [
      {
        id: "decision-1",
        tenant_id: "tenant-1",
        project_id: "project-1",
        approval_request_id: "approval-1",
        project_task_id: "task-run",
        target_user_id: "reviewer-1",
        decision_type: "route_review",
        title_snapshot: "需要确认巡检范围",
        status_snapshot: "pending",
      },
    ],
  };
}

describe("workflow graph adapter", () => {
  it("maps project task graph nodes and edges into xyflow elements", () => {
    const result = buildWorkflowGraphElements(makeGraph());
    const runTaskData = taskData(result, "task:task-run");

    expect(result.nodes.map((node) => node.id)).toEqual([
      "task:task-root",
      "task:task-run",
      "attachment:decision:decision-1",
    ]);
    expect(result.edges).toEqual([
      expect.objectContaining({
        id: "edge:task-root:task-run",
        source: "task:task-root",
        target: "task:task-run",
      }),
    ]);
    expect(result.nodes[0].position.x).toBe(0);
    expect(result.nodes[1].position.x).toBeGreaterThan(result.nodes[0].position.x);
    expect(runTaskData.employeeName).toBe("应用运维工程师");
    expect(runTaskData.runStatus).toBe("assigned");
  });

  it("places unstaged tasks in the highest known finite stage", () => {
    const graph = makeGraph();
    graph.nodes[0].stage_index = 3;
    graph.nodes[1].stage_index = 1;
    graph.nodes.push({
      ...graph.nodes[1],
      id: "task-unstaged",
      title: "补充验证",
      stage_index: undefined,
    });

    const result = buildWorkflowGraphElements(graph);
    const stageThreeNode = result.nodes.find((node) => node.id === "task:task-root");
    const stageOneNode = result.nodes.find((node) => node.id === "task:task-run");
    const unstagedNode = result.nodes.find((node) => node.id === "task:task-unstaged");

    expect(unstagedNode?.position.x).toBe(stageThreeNode?.position.x);
    expect(unstagedNode?.position.x).toBeGreaterThan(stageOneNode?.position.x ?? 0);
  });

  it("keeps zero-based stages in separate columns and aligns unstaged tasks to max stage", () => {
    const graph = makeGraph();
    graph.nodes[0].stage_index = 0;
    graph.nodes[1].stage_index = 1;
    graph.nodes.push({
      ...graph.nodes[1],
      id: "task-unstaged",
      title: "补充验证",
      stage_index: undefined,
    });

    const result = buildWorkflowGraphElements(graph);
    const stageZeroNode = result.nodes.find((node) => node.id === "task:task-root");
    const stageOneNode = result.nodes.find((node) => node.id === "task:task-run");
    const unstagedNode = result.nodes.find((node) => node.id === "task:task-unstaged");

    expect(stageZeroNode?.position.x).toBe(0);
    expect(stageOneNode?.position.x).toBeGreaterThan(stageZeroNode?.position.x ?? 0);
    expect(unstagedNode?.position.x).toBe(stageOneNode?.position.x);
  });

  it("skips decision attachments when the parent task is missing", () => {
    const graph = makeGraph();
    graph.decision_requests.push({
      id: "decision-orphaned",
      tenant_id: "tenant-1",
      project_id: "project-1",
      approval_request_id: "approval-orphaned",
      project_task_id: "task-missing",
      target_user_id: "reviewer-1",
      decision_type: "route_review",
      title_snapshot: "孤立审批",
      status_snapshot: "pending",
    });

    const result = buildWorkflowGraphElements(graph);

    expect(result.nodes.map((node) => node.id)).toContain("attachment:decision:decision-1");
    expect(result.nodes.map((node) => node.id)).not.toContain(
      "attachment:decision:decision-orphaned",
    );
  });

  it("skips dependency edges when either endpoint task is missing", () => {
    const graph = makeGraph();
    graph.edges.push({
      blocker_task_id: "task-missing",
      dependent_task_id: "task-run",
      edge_status: "unblocked",
    });

    const result = buildWorkflowGraphElements(graph);

    expect(result.edges.map((edge) => edge.id)).toEqual(["edge:task-root:task-run"]);
  });

  it("animates ready, unblocked, and completed dependency edges", () => {
    const graph = makeGraph();
    graph.nodes.push(makeTask("task-review", "completed"));
    graph.edges = [
      {
        blocker_task_id: "task-root",
        dependent_task_id: "task-run",
        edge_status: "unblocked",
      },
      {
        blocker_task_id: "task-run",
        dependent_task_id: "task-review",
        edge_status: "ready",
      },
      {
        blocker_task_id: "task-review",
        dependent_task_id: "task-root",
        edge_status: "completed",
      },
    ];

    const result = buildWorkflowGraphElements(graph);

    expect(result.edges.map((edge) => [edge.id, edge.animated])).toEqual([
      ["edge:task-root:task-run", true],
      ["edge:task-run:task-review", true],
      ["edge:task-review:task-root", true],
    ]);
  });

  it("does not animate blocked or planned dependency edges", () => {
    const graph = makeGraph();
    graph.nodes.push(makeTask("task-review", "planned"));
    graph.edges = [
      {
        blocker_task_id: "task-root",
        dependent_task_id: "task-run",
        edge_status: "blocked",
      },
      {
        blocker_task_id: "task-run",
        dependent_task_id: "task-review",
        edge_status: "planned",
      },
    ];

    const result = buildWorkflowGraphElements(graph);

    expect(result.edges.map((edge) => [edge.id, edge.animated])).toEqual([
      ["edge:task-root:task-run", false],
      ["edge:task-run:task-review", false],
    ]);
  });

  it("selects the highest priority task status before falling back to the first node", () => {
    const graph = makeGraph();
    graph.nodes = [
      makeTask("task-completed", "completed"),
      makeTask("task-in-progress", "in_progress"),
      makeTask("task-running", "running"),
      makeTask("task-assigned", "assigned"),
      makeTask("task-planned", "planned"),
      makeTask("task-pending", "pending"),
      makeTask("task-waiting", "waiting_human"),
      makeTask("task-blocked", "blocked"),
      makeTask("task-failed", "failed"),
    ];

    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-failed");

    graph.nodes = graph.nodes.filter((node) => node.status !== "failed");
    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-blocked");

    graph.nodes = graph.nodes.filter((node) => node.status !== "blocked");
    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-waiting");

    graph.nodes = graph.nodes.filter((node) => node.status !== "waiting_human");
    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-pending");

    graph.nodes = graph.nodes.filter((node) => node.status !== "pending");
    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-planned");

    graph.nodes = graph.nodes.filter((node) => node.status !== "planned");
    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-assigned");

    graph.nodes = graph.nodes.filter((node) => node.status !== "assigned");
    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-running");

    graph.nodes = graph.nodes.filter((node) => node.status !== "running");
    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-in-progress");

    graph.nodes = graph.nodes.filter((node) => node.status !== "in_progress");
    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-completed");
  });

  it("selects a planned task before falling back to completed first node", () => {
    const graph = makeGraph();
    graph.nodes = [
      makeTask("task-completed", "completed"),
      makeTask("task-planned", "planned"),
    ];

    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-planned");
  });

  it("does not let pending decisions override status-priority selection", () => {
    const graph = makeGraph();
    graph.nodes[0].status = "completed";
    graph.nodes[1].status = "running";
    graph.decision_requests[0].project_task_id = "task-root";

    expect(selectInitialWorkflowNodeId(graph)).toBe("task:task-run");
  });
});

function makeTask(id: string, status: string): ProjectTaskGraph["nodes"][number] {
  return {
    id,
    tenant_id: "tenant-1",
    project_id: "project-1",
    demand_id: "demand-1",
    title: id,
    summary: id,
    status,
    requires_human_approval: false,
    expected_outputs: [],
    input_requirements: {},
    handoff_contract: {},
    planner_metadata: {},
  };
}

function taskData(
  result: ReturnType<typeof buildWorkflowGraphElements>,
  nodeId: string,
): WorkflowTaskNodeData {
  const node = result.nodes.find((candidate) => candidate.id === nodeId);
  expect(node?.type).toBe("workflowTask");
  return node?.data as WorkflowTaskNodeData;
}
