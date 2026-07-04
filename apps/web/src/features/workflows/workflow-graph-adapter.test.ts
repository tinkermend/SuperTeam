import { describe, expect, it } from "vitest";

import {
  PLAN_TASK_GRAPH_LAYOUT,
  buildPlanTaskGraphElements,
  buildWorkflowGraphElements,
  selectInitialWorkflowNodeId,
  stageLabelNodeId,
} from "./workflow-graph-adapter";
import type {
  WorkflowStageLabelNodeData,
  WorkflowTaskNodeData,
} from "./workflow-graph-adapter";
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
        employee_role: "应用运维工程师·工程部",
        avatar_asset: {
          id: "avatar-1",
          label: "Adventurer 1",
          image_url: "https://example.com/avatar-1.png",
          thumbnail_url: "https://example.com/avatar-1-thumb.png",
        },
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
    blocking_facts: [],
  };
}

describe("workflow graph adapter", () => {
  it("maps project task graph nodes and edges into xyflow elements", () => {
    const result = buildWorkflowGraphElements(makeGraph());
    const runTaskData = taskData(result, "task:task-run");

    expect(result.nodes.map((node) => node.id)).toEqual([
      "stage-label:1",
      "task:task-root",
      "stage-label:2",
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
    const rootTask = result.nodes.find((node) => node.id === "task:task-root");
    const runTask = result.nodes.find((node) => node.id === "task:task-run");
    const stageOneLabel = result.nodes.find((node) => node.id === stageLabelNodeId(1));
    const stageTwoLabel = result.nodes.find((node) => node.id === stageLabelNodeId(2));

    expect(rootTask?.position.y).toBeGreaterThan(stageOneLabel?.position.y ?? 0);
    expect(runTask?.position.y).toBeGreaterThan(stageTwoLabel?.position.y ?? 0);
    expect(runTask?.position.y).toBeGreaterThan(rootTask?.position.y ?? 0);
    expect(runTaskData.employeeName).toBe("应用运维工程师");
    expect(runTaskData.runStatus).toBe("assigned");
    expect(runTaskData.employeeRole).toBe("应用运维工程师·工程部");
    expect(runTaskData.avatarAsset?.thumbnailUrl).toBe("https://example.com/avatar-1-thumb.png");
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

    expect(unstagedNode?.position.y).toBe(stageThreeNode?.position.y);
    expect(unstagedNode?.position.y).toBeGreaterThan(stageOneNode?.position.y ?? 0);
    expect(unstagedNode?.position.x).not.toBe(stageThreeNode?.position.x);
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

    expect(stageZeroNode?.position.x).toBe(-180);
    expect(stageZeroNode?.position.y).toBeGreaterThan(0);
    expect(stageOneNode?.position.y).toBeGreaterThan(stageZeroNode?.position.y ?? 0);
    expect(unstagedNode?.position.y).toBe(stageOneNode?.position.y);
    expect(unstagedNode?.position.x).not.toBe(stageOneNode?.position.x);
  });

  it("wraps same-stage tasks into columns and keeps stage 1 before stage 2", () => {
    const graph = makeGraph();
    graph.nodes = [
      makeTask("task-stage-2", "planned", { stage_index: 2 }),
      makeTask("task-stage-1-a", "planned", { stage_index: 1 }),
      makeTask("task-stage-1-b", "planned", { stage_index: 1 }),
    ];

    const result = buildWorkflowGraphElements(graph);
    const stageTwoNode = result.nodes.find((node) => node.id === "task:task-stage-2");
    const stageOneFirstNode = result.nodes.find(
      (node) => node.id === "task:task-stage-1-a",
    );
    const stageOneSecondNode = result.nodes.find(
      (node) => node.id === "task:task-stage-1-b",
    );

    expect(stageTwoNode).toBeDefined();
    expect(stageOneFirstNode).toBeDefined();
    expect(stageOneSecondNode).toBeDefined();
    expect(stageOneFirstNode?.position.y).toBe(stageOneSecondNode?.position.y);
    expect(stageOneFirstNode?.position.x).not.toBe(stageOneSecondNode?.position.x);
    expect(stageTwoNode?.position.y).toBeGreaterThan(
      stageOneFirstNode?.position.y ?? 0,
    );
  });

  it("uses the shared dynamic desktop layout for workflow task nodes", () => {
    const graph = makeGraph();
    graph.nodes = Array.from({ length: 10 }, (_, index) =>
      makeTask(`task-${index + 1}`, "planned", {
        assigned_digital_employee_id: `employee-${index + 1}`,
        planned_task_key: `task-${index + 1}`,
        stage_index: 0,
      }),
    );
    graph.edges = graph.nodes.slice(1).map((task, index) => ({
      blocker_task_id: graph.nodes[index].id,
      dependent_task_id: task.id,
      edge_status: "planned",
    }));
    graph.employees = graph.nodes.map((task, index) => ({
      digital_employee_id: task.assigned_digital_employee_id ?? `employee-${index + 1}`,
      display_name: `数字员工 ${index + 1}`,
      project_role: "executor",
      status: "active",
    }));

    const result = buildWorkflowGraphElements(graph);
    const taskNodes = result.nodes.filter((node) => node.type === "workflowTask");
    const rowsByY = new Map<number, typeof taskNodes>();
    for (const node of taskNodes) {
      const row = rowsByY.get(node.position.y) ?? [];
      row.push(node);
      rowsByY.set(node.position.y, row);
    }

    expect(rowsByY.size).toBe(5);
    expect([...rowsByY.values()].map((row) => row.length)).toEqual([2, 2, 2, 2, 2]);
    expect(workflowTaskOverlapPairs(result)).toEqual([]);
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

describe("plan task graph adapter", () => {
  it("publishes one desktop layout spec shared by dynamic plan graph renderers", () => {
    expect(PLAN_TASK_GRAPH_LAYOUT.maxTasks).toBe(10);
    expect(PLAN_TASK_GRAPH_LAYOUT.tasksPerRow).toBe(2);
    expect(PLAN_TASK_GRAPH_LAYOUT.taskColumnWidth).toBeGreaterThan(
      PLAN_TASK_GRAPH_LAYOUT.taskNodeWidth,
    );
    expect(PLAN_TASK_GRAPH_LAYOUT.taskRowHeight).toBeGreaterThan(
      PLAN_TASK_GRAPH_LAYOUT.taskEstimatedHeight,
    );
  });

  it("lays out one row per stage, centered on a shared x=0 axis, with a stage label per row", () => {
    const graph = makeGraph();
    graph.nodes = [
      makeTask("task-a", "planned", { stage_index: 0 }),
      makeTask("task-b", "planned", {
        assigned_digital_employee_id: "employee-1",
        stage_index: 1,
      }),
      makeTask("task-c", "planned", {
        assigned_digital_employee_id: "employee-1",
        stage_index: 1,
      }),
    ];
    graph.edges = [];
    graph.employees = [
      {
        digital_employee_id: "employee-1",
        display_name: "同一执行人",
        project_role: "executor",
        status: "active",
      },
    ];

    const result = buildPlanTaskGraphElements(graph);

    const stageZeroLabel = result.nodes.find((node) => node.id === stageLabelNodeId(0));
    const stageOneLabel = result.nodes.find((node) => node.id === stageLabelNodeId(1));
    const taskA = result.nodes.find((node) => node.id === "task:task-a");
    const taskB = result.nodes.find((node) => node.id === "task:task-b");
    const taskC = result.nodes.find((node) => node.id === "task:task-c");

    expect(stageZeroLabel).toBeDefined();
    expect(stageOneLabel).toBeDefined();
    expect((stageZeroLabel?.data as WorkflowStageLabelNodeData).taskCount).toBe(1);
    expect((stageOneLabel?.data as WorkflowStageLabelNodeData).taskCount).toBe(2);
    expect((stageOneLabel?.data as WorkflowStageLabelNodeData).employeeCount).toBe(1);

    expect((stageZeroLabel?.position.x ?? 0) + 84).toBe(0);
    expect((stageOneLabel?.position.x ?? 0) + 84).toBe(0);
    expect((taskA?.position.x ?? 0) + 180).toBe(0);
    expect((taskB?.position.x ?? 0) + (taskC?.position.x ?? 0) + 360).toBe(0);
    expect(taskB?.position.x).not.toBe(taskC?.position.x);

    expect(taskB?.position.y ?? 0).toBeGreaterThan(taskA?.position.y ?? 0);
    expect((stageOneLabel?.position.y ?? 0) - (stageZeroLabel?.position.y ?? 0)).toBeGreaterThanOrEqual(
      300,
    );
    expect(stageOneLabel?.position.y ?? 0).toBeLessThan(taskB?.position.y ?? 0);
    expect((taskB?.position.x ?? 0) - (taskA?.position.x ?? 0)).toBeLessThan(-170);
    expect((taskC?.position.x ?? 0) - (taskA?.position.x ?? 0)).toBeGreaterThan(170);
    expect((taskC?.position.x ?? 0) - (taskB?.position.x ?? 0) - 360).toBeGreaterThanOrEqual(
      120,
    );
  });

  it("falls back to a generated stage title when stage_summaries has no matching entry", () => {
    const graph = makeGraph();
    graph.nodes = [makeTask("task-a", "planned", { stage_index: 4 })];
    graph.edges = [];
    graph.employees = [];
    graph.stage_summaries = [];

    const result = buildPlanTaskGraphElements(graph);
    const label = result.nodes.find((node) => node.id === stageLabelNodeId(4));

    expect((label?.data as WorkflowStageLabelNodeData).title).toBe("第 5 阶段");
  });

  it("uses the stage title from stage_summaries when present", () => {
    const graph = makeGraph();
    graph.nodes = [makeTask("task-a", "planned", { stage_index: 0 })];
    graph.edges = [];
    graph.employees = [];
    graph.stage_summaries = [
      {
        stage_index: 0,
        title: "并行审查",
        total_nodes: 1,
        completed_nodes: 0,
        running_nodes: 0,
        waiting_human_nodes: 0,
        blocked_nodes: 0,
      },
    ];

    const result = buildPlanTaskGraphElements(graph);
    const label = result.nodes.find((node) => node.id === stageLabelNodeId(0));

    expect((label?.data as WorkflowStageLabelNodeData).title).toBe("并行审查");
  });

  it("passes styled unlabeled dependency edges to the plan view", () => {
    const graph = makeGraph();
    graph.nodes = [
      makeTask("task-a", "planned", { stage_index: 0 }),
      makeTask("task-b", "planned", { stage_index: 1 }),
    ];
    graph.edges = [
      { blocker_task_id: "task-a", dependent_task_id: "task-b", edge_status: "planned" },
    ];
    graph.employees = [];

    const result = buildPlanTaskGraphElements(graph);

    expect(result.edges[0]).toEqual(
      expect.objectContaining({
        id: "edge:task-a:task-b",
        label: undefined,
        markerEnd: expect.objectContaining({ type: "arrowclosed" }),
        source: "task:task-a",
        style: expect.objectContaining({ strokeWidth: 1.6 }),
        target: "task:task-b",
        type: "smoothstep",
      }),
    );
  });

  it("wraps a large planned stage into breathable draggable desktop rows", () => {
    const graph = makeGraph();
    graph.nodes = Array.from({ length: 10 }, (_, index) =>
      makeTask(`task-${index + 1}`, "planned", {
        assigned_digital_employee_id: `employee-${index + 1}`,
        planned_task_key: `task-${index + 1}`,
        stage_index: 0,
      }),
    );
    graph.edges = [];
    graph.employees = graph.nodes.map((task, index) => ({
      digital_employee_id: task.assigned_digital_employee_id ?? `employee-${index + 1}`,
      display_name: `数字员工 ${index + 1}`,
      project_role: "executor",
      status: "active",
    }));

    const result = buildPlanTaskGraphElements(graph);
    const taskNodes = result.nodes.filter((node) => node.type === "workflowTask");
    const rowsByY = new Map<number, typeof taskNodes>();
    for (const node of taskNodes) {
      const row = rowsByY.get(node.position.y) ?? [];
      row.push(node);
      rowsByY.set(node.position.y, row);
    }

    expect(rowsByY.size).toBe(5);

    const rows = [...rowsByY.entries()].sort(([a], [b]) => a - b);
    expect(rows.map(([, row]) => row.length)).toEqual([2, 2, 2, 2, 2]);

    for (const [, row] of rows) {
      const minX = Math.min(...row.map((node) => node.position.x));
      const maxRight = Math.max(...row.map((node) => node.position.x + 360));

      expect(minX).toBeGreaterThanOrEqual(-520);
      expect(maxRight).toBeLessThanOrEqual(520);

      const ordered = [...row].sort((a, b) => a.position.x - b.position.x);
      for (let index = 1; index < ordered.length; index += 1) {
        const previousRight = ordered[index - 1].position.x + 360;
        expect(ordered[index].position.x - previousRight).toBeGreaterThanOrEqual(220);
      }
    }

    for (let index = 1; index < rows.length; index += 1) {
      expect(rows[index][0] - rows[index - 1][0]).toBeGreaterThanOrEqual(400);
    }
  });

  it("keeps every supported dynamic generated plan layout non-overlapping", () => {
    for (let taskCount = 1; taskCount <= PLAN_TASK_GRAPH_LAYOUT.maxTasks; taskCount += 1) {
      const graph = makeGraph();
      graph.nodes = Array.from({ length: taskCount }, (_, index) =>
        makeTask(`same-stage-task-${index + 1}`, "planned", {
          assigned_digital_employee_id: `employee-${index + 1}`,
          planned_task_key: `task-${index + 1}`,
          stage_index: 0,
        }),
      );
      graph.edges = graph.nodes.slice(1).map((task, index) => ({
        blocker_task_id: graph.nodes[index].id,
        dependent_task_id: task.id,
        edge_status: "planned",
      }));
      graph.employees = graph.nodes.map((task, index) => ({
        digital_employee_id: task.assigned_digital_employee_id ?? `employee-${index + 1}`,
        display_name: `数字员工 ${index + 1}`,
        project_role: "executor",
        status: "active",
      }));

      const result = buildPlanTaskGraphElements(graph);

      expect(taskOverlapPairs(result)).toEqual([]);
      expect(result.edges).toHaveLength(Math.max(0, taskCount - 1));
    }

    const multiStageGraph = makeGraph();
    multiStageGraph.nodes = Array.from({ length: PLAN_TASK_GRAPH_LAYOUT.maxTasks }, (_, index) =>
      makeTask(`multi-stage-task-${index + 1}`, "planned", {
        assigned_digital_employee_id: `employee-${index + 1}`,
        planned_task_key: `task-${index + 1}`,
        stage_index: Math.floor(index / 3),
      }),
    );
    multiStageGraph.edges = multiStageGraph.nodes.slice(1).map((task, index) => ({
      blocker_task_id: multiStageGraph.nodes[index].id,
      dependent_task_id: task.id,
      edge_status: "planned",
    }));
    multiStageGraph.employees = multiStageGraph.nodes.map((task, index) => ({
      digital_employee_id: task.assigned_digital_employee_id ?? `employee-${index + 1}`,
      display_name: `数字员工 ${index + 1}`,
      project_role: "executor",
      status: "active",
    }));

    const multiStageResult = buildPlanTaskGraphElements(multiStageGraph);

    expect(taskOverlapPairs(multiStageResult)).toEqual([]);
    expect(multiStageResult.edges).toHaveLength(PLAN_TASK_GRAPH_LAYOUT.maxTasks - 1);
  });

  it("orders planned tasks by their natural planned task key before wrapping", () => {
    const graph = makeGraph();
    graph.nodes = [
      makeTask("id-10", "planned", { planned_task_key: "task-10", stage_index: 0 }),
      makeTask("id-2", "planned", { planned_task_key: "task-2", stage_index: 0 }),
      makeTask("id-1", "planned", { planned_task_key: "task-1", stage_index: 0 }),
      makeTask("id-11", "planned", { planned_task_key: "task-11", stage_index: 0 }),
      makeTask("id-3", "planned", { planned_task_key: "task-3", stage_index: 0 }),
    ];
    graph.edges = [];
    graph.employees = [];

    const result = buildPlanTaskGraphElements(graph);
    const taskPositions = result.nodes
      .filter((node) => node.type === "workflowTask")
      .map((node) => ({ id: node.id, x: node.position.x, y: node.position.y }))
      .sort((a, b) => a.y - b.y || a.x - b.x);

    expect(taskPositions.map((node) => node.id)).toEqual([
      "task:id-1",
      "task:id-2",
      "task:id-3",
      "task:id-10",
      "task:id-11",
    ]);
  });
});

function makeTask(
  id: string,
  status: string,
  overrides: Partial<ProjectTaskGraph["nodes"][number]> = {},
): ProjectTaskGraph["nodes"][number] {
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
    ...overrides,
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

function taskOverlapPairs(result: ReturnType<typeof buildPlanTaskGraphElements>): string[] {
  const taskRects = result.nodes
    .filter((node) => node.type === "workflowTask")
    .map((node) => ({
      bottom: node.position.y + PLAN_TASK_GRAPH_LAYOUT.taskEstimatedHeight,
      id: node.id,
      left: node.position.x,
      right: node.position.x + PLAN_TASK_GRAPH_LAYOUT.taskNodeWidth,
      top: node.position.y,
    }));

  const pairs: string[] = [];
  for (let leftIndex = 0; leftIndex < taskRects.length; leftIndex += 1) {
    for (let rightIndex = leftIndex + 1; rightIndex < taskRects.length; rightIndex += 1) {
      const left = taskRects[leftIndex];
      const right = taskRects[rightIndex];
      const overlapX =
        Math.min(left.right, right.right) - Math.max(left.left, right.left);
      const overlapY =
        Math.min(left.bottom, right.bottom) - Math.max(left.top, right.top);

      if (overlapX > 0 && overlapY > 0) {
        pairs.push(`${left.id}:${right.id}`);
      }
    }
  }

  return pairs;
}

function workflowTaskOverlapPairs(
  result: ReturnType<typeof buildWorkflowGraphElements>,
): string[] {
  const taskRects = result.nodes
    .filter((node) => node.type === "workflowTask")
    .map((node) => ({
      bottom: node.position.y + PLAN_TASK_GRAPH_LAYOUT.taskEstimatedHeight,
      id: node.id,
      left: node.position.x,
      right: node.position.x + PLAN_TASK_GRAPH_LAYOUT.taskNodeWidth,
      top: node.position.y,
    }));

  const pairs: string[] = [];
  for (let leftIndex = 0; leftIndex < taskRects.length; leftIndex += 1) {
    for (let rightIndex = leftIndex + 1; rightIndex < taskRects.length; rightIndex += 1) {
      const left = taskRects[leftIndex];
      const right = taskRects[rightIndex];
      const overlapX =
        Math.min(left.right, right.right) - Math.max(left.left, right.left);
      const overlapY =
        Math.min(left.bottom, right.bottom) - Math.max(left.top, right.top);

      if (overlapX > 0 && overlapY > 0) {
        pairs.push(`${left.id}:${right.id}`);
      }
    }
  }

  return pairs;
}
