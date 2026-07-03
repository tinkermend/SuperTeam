import type { Edge, Node } from "@xyflow/react";

import type {
  ProjectDecisionRequest,
  ProjectTaskGraph,
  ProjectTaskGraphEmployee,
  ProjectTaskGraphNode,
} from "@/lib/api/projects";
import { taskStatusLabel } from "@/lib/status-labels";

const STAGE_X = 360;
const ROW_Y = 170;
const ATTACHMENT_OFFSET_Y = 116;

const INITIAL_STATUS_PRIORITY = [
  "failed",
  "blocked",
  "waiting_human",
  "pending",
  "planned",
  "assigned",
  "running",
  "in_progress",
];

const PENDING_DECISION_STATUSES = new Set(["pending", "open", "requested"]);
const ANIMATED_EDGE_STATUSES = new Set(["unblocked", "ready", "completed"]);

export type WorkflowTaskNodeData = {
  avatarAsset: WorkflowTaskNodeAvatarAsset | undefined;
  employeeName: string | undefined;
  employeeRole: string | undefined;
  expectedOutputs: unknown[];
  hasPendingDecision: boolean;
  requiresHumanApproval: boolean;
  riskLevel: string | undefined;
  runStatus: string | undefined;
  status: string;
  summary: string | undefined;
  task: ProjectTaskGraphNode;
  title: string;
};

export type WorkflowTaskNodeAvatarAsset = {
  id: string;
  imageUrl: string;
  label: string;
  thumbnailUrl: string;
};

export type WorkflowAttachmentNodeData = {
  status: string;
  title: string;
  type: "decision";
};

type WorkflowTaskNode = Node<WorkflowTaskNodeData, "workflowTask">;
type WorkflowAttachmentNode = Node<WorkflowAttachmentNodeData, "workflowAttachment">;
type WorkflowStageLabelNode = Node<WorkflowStageLabelNodeData, "workflowStageLabel">;
type WorkflowGraphNode = WorkflowTaskNode | WorkflowAttachmentNode;

export type WorkflowGraphElements = {
  nodes: WorkflowGraphNode[];
  edges: Edge[];
};

export type WorkflowStageLabelNodeData = {
  employeeCount: number;
  stageIndex: number;
  taskCount: number;
  title: string;
};

export type PlanWorkflowGraphElements = {
  edges: Edge[];
  nodes: (WorkflowTaskNode | WorkflowStageLabelNode)[];
};

export const PLAN_TASK_GRAPH_LAYOUT = {
  maxTasks: 10,
  stageLabelHeight: 76,
  stageLabelWidth: 168,
  stageRowHeight: 430,
  taskColumnWidth: 600,
  taskEstimatedHeight: 320,
  taskNodeWidth: 360,
  taskRowHeight: 430,
  tasksPerRow: 2,
} as const;

export function taskNodeId(taskId: string): string {
  return `task:${taskId}`;
}

export function stageLabelNodeId(stageIndex: number): string {
  return `stage-label:${stageIndex}`;
}

function toWorkflowTaskNodeAvatarAsset(
  asset: ProjectTaskGraphEmployee["avatar_asset"],
): WorkflowTaskNodeAvatarAsset | undefined {
  if (!asset) return undefined;
  return {
    id: asset.id,
    imageUrl: asset.image_url,
    label: asset.label,
    thumbnailUrl: asset.thumbnail_url,
  };
}

export function buildWorkflowGraphElements(graph: ProjectTaskGraph): WorkflowGraphElements {
  const taskIds = new Set(graph.nodes.map((task) => task.id));
  const employeesById = new Map(
    graph.employees.map((employee) => [employee.digital_employee_id, employee]),
  );
  const runStatusByTaskId = new Map(graph.runs.map((run) => [run.project_task_id, run.status]));
  const pendingDecisions = graph.decision_requests.filter(
    (decision) => isPendingTaskDecision(decision) && taskIds.has(decision.project_task_id ?? ""),
  );
  const pendingDecisionsByTaskId = groupDecisionsByTaskId(pendingDecisions);
  const stageRange = knownStageRange(graph.nodes);
  const rowsByStage = new Map<number, number>();

  const taskNodes: WorkflowTaskNode[] = graph.nodes.map((task) => {
    const stage = finiteStage(task.stage_index) ?? stageRange.max;
    const row = rowsByStage.get(stage) ?? 0;
    rowsByStage.set(stage, row + 1);

    return {
      id: taskNodeId(task.id),
      type: "workflowTask",
      position: {
        x: stageColumnX(stage, stageRange),
        y: row * ROW_Y,
      },
      data: {
        avatarAsset: toWorkflowTaskNodeAvatarAsset(
          task.assigned_digital_employee_id
            ? employeesById.get(task.assigned_digital_employee_id)?.avatar_asset
            : undefined,
        ),
        employeeName: task.assigned_digital_employee_id
          ? employeesById.get(task.assigned_digital_employee_id)?.display_name
          : undefined,
        employeeRole: task.assigned_digital_employee_id
          ? employeesById.get(task.assigned_digital_employee_id)?.employee_role
          : undefined,
        expectedOutputs: task.expected_outputs,
        hasPendingDecision: pendingDecisionsByTaskId.has(task.id),
        requiresHumanApproval: task.requires_human_approval,
        riskLevel: task.risk_level,
        runStatus: runStatusByTaskId.get(task.id),
        status: task.status,
        summary: task.summary,
        task,
        title: task.title,
      },
    };
  });

  const attachmentCountsByTaskId = new Map<string, number>();
  const attachmentNodes: WorkflowAttachmentNode[] = pendingDecisions.map((decision) => {
    const taskId = decision.project_task_id ?? "";
    const attachmentIndex = attachmentCountsByTaskId.get(taskId) ?? 0;
    attachmentCountsByTaskId.set(taskId, attachmentIndex + 1);

    return {
      id: `attachment:decision:${decision.id}`,
      type: "workflowAttachment",
      parentId: taskNodeId(taskId),
      position: {
        x: 0,
        y: ATTACHMENT_OFFSET_Y * (attachmentIndex + 1),
      },
      data: {
        status: decision.status_snapshot,
        title: decision.title_snapshot,
        type: "decision",
      },
    };
  });

  return {
    nodes: [...taskNodes, ...attachmentNodes],
    edges: buildTaskDependencyEdges(graph, taskIds, { includeLabel: true }),
  };
}

function buildTaskDependencyEdges(
  graph: ProjectTaskGraph,
  taskIds: Set<string>,
  options: Partial<Pick<Edge, "markerEnd" | "style">> & { includeLabel: boolean },
): Edge[] {
  return graph.edges
    .filter((edge) => taskIds.has(edge.blocker_task_id) && taskIds.has(edge.dependent_task_id))
    .map((edge) => ({
      id: `edge:${edge.blocker_task_id}:${edge.dependent_task_id}`,
      source: taskNodeId(edge.blocker_task_id),
      target: taskNodeId(edge.dependent_task_id),
      type: "smoothstep",
      label: options.includeLabel ? taskStatusLabel(edge.edge_status) : undefined,
      animated: ANIMATED_EDGE_STATUSES.has(normalizeStatus(edge.edge_status)),
      markerEnd: options.markerEnd,
      style: options.style,
    }));
}

export function buildPlanTaskGraphElements(graph: ProjectTaskGraph): PlanWorkflowGraphElements {
  const taskIds = new Set(graph.nodes.map((task) => task.id));
  const employeesById = new Map(
    graph.employees.map((employee) => [employee.digital_employee_id, employee]),
  );
  const runStatusByTaskId = new Map(graph.runs.map((run) => [run.project_task_id, run.status]));
  const pendingDecisions = graph.decision_requests.filter(
    (decision) => isPendingTaskDecision(decision) && taskIds.has(decision.project_task_id ?? ""),
  );
  const pendingDecisionsByTaskId = groupDecisionsByTaskId(pendingDecisions);
  const stageTitleByIndex = new Map(
    (graph.stage_summaries ?? []).map((stage) => [stage.stage_index, stage.title]),
  );
  const stageRange = knownStageRange(graph.nodes);
  const inputOrderByTaskId = new Map(graph.nodes.map((task, index) => [task.id, index]));

  const tasksByStage = new Map<number, ProjectTaskGraphNode[]>();
  for (const task of graph.nodes) {
    const stage = finiteStage(task.stage_index) ?? stageRange.max;
    const list = tasksByStage.get(stage) ?? [];
    list.push(task);
    tasksByStage.set(stage, list);
  }
  const orderedStages = [...tasksByStage.keys()].sort((a, b) => a - b);

  const nodes: (WorkflowTaskNode | WorkflowStageLabelNode)[] = [];
  let stageTopY = 0;
  orderedStages.forEach((stage) => {
    const tasks = sortPlanStageTasks(tasksByStage.get(stage) ?? [], inputOrderByTaskId);
    const assignedEmployeeIds = new Set(
      tasks.flatMap((task) =>
        task.assigned_digital_employee_id ? [task.assigned_digital_employee_id] : [],
      ),
    );
    const taskRows = chunkTasks(tasks, PLAN_TASK_GRAPH_LAYOUT.tasksPerRow);
    const rowY = stageTopY;

    nodes.push({
      id: stageLabelNodeId(stage),
      type: "workflowStageLabel",
      draggable: false,
      selectable: false,
      position: { x: -PLAN_TASK_GRAPH_LAYOUT.stageLabelWidth / 2, y: rowY },
      data: {
        employeeCount: assignedEmployeeIds.size,
        stageIndex: stage,
        taskCount: tasks.length,
        title: stageTitleByIndex.get(stage) || `第 ${stage + 1} 阶段`,
      },
    });

    taskRows.forEach((rowTasks, rowIndex) => {
      rowTasks.forEach((task, taskOrder) => {
        const employee = task.assigned_digital_employee_id
          ? employeesById.get(task.assigned_digital_employee_id)
          : undefined;
        const offset =
          (taskOrder - (rowTasks.length - 1) / 2) *
          PLAN_TASK_GRAPH_LAYOUT.taskColumnWidth;

        nodes.push({
          id: taskNodeId(task.id),
          type: "workflowTask",
          position: {
            x: offset - PLAN_TASK_GRAPH_LAYOUT.taskNodeWidth / 2,
            y:
              rowY +
              PLAN_TASK_GRAPH_LAYOUT.stageLabelHeight +
              rowIndex * PLAN_TASK_GRAPH_LAYOUT.taskRowHeight,
          },
          data: {
            avatarAsset: toWorkflowTaskNodeAvatarAsset(employee?.avatar_asset),
            employeeName: employee?.display_name,
            employeeRole: employee?.employee_role,
            expectedOutputs: task.expected_outputs,
            hasPendingDecision: pendingDecisionsByTaskId.has(task.id),
            requiresHumanApproval: task.requires_human_approval,
            riskLevel: task.risk_level,
            runStatus: runStatusByTaskId.get(task.id),
            status: task.status,
            summary: task.summary,
            task,
            title: task.title,
          },
        });
      });
    });

    stageTopY +=
      PLAN_TASK_GRAPH_LAYOUT.stageRowHeight +
      Math.max(0, taskRows.length - 1) * PLAN_TASK_GRAPH_LAYOUT.taskRowHeight;
  });

  return {
    edges: buildTaskDependencyEdges(graph, taskIds, {
      includeLabel: false,
      markerEnd: {
        color: "rgb(47 95 255 / 0.45)",
        height: 18,
        type: "arrowclosed",
        width: 18,
      },
      style: {
        stroke: "rgb(47 95 255 / 0.32)",
        strokeWidth: 1.6,
      },
    }),
    nodes,
  };
}

function chunkTasks(
  tasks: ProjectTaskGraphNode[],
  maxTasksPerRow: number,
): ProjectTaskGraphNode[][] {
  const rows: ProjectTaskGraphNode[][] = [];
  for (let index = 0; index < tasks.length; index += maxTasksPerRow) {
    rows.push(tasks.slice(index, index + maxTasksPerRow));
  }
  return rows.length > 0 ? rows : [[]];
}

function sortPlanStageTasks(
  tasks: ProjectTaskGraphNode[],
  inputOrderByTaskId: Map<string, number>,
): ProjectTaskGraphNode[] {
  return [...tasks].sort((a, b) => {
    const keyComparison = naturalCompare(a.planned_task_key, b.planned_task_key);
    if (keyComparison !== 0) return keyComparison;
    return inputOrder(a, inputOrderByTaskId) - inputOrder(b, inputOrderByTaskId);
  });
}

function naturalCompare(left: string | undefined, right: string | undefined): number {
  if (left && right) return left.localeCompare(right, undefined, { numeric: true });
  if (left) return -1;
  if (right) return 1;
  return 0;
}

function inputOrder(task: ProjectTaskGraphNode, inputOrderByTaskId: Map<string, number>): number {
  return inputOrderByTaskId.get(task.id) ?? 0;
}

export function selectInitialWorkflowNodeId(graph: ProjectTaskGraph): string | undefined {
  for (const status of INITIAL_STATUS_PRIORITY) {
    const task = graph.nodes.find((node) => normalizeStatus(node.status) === status);
    if (task) return taskNodeId(task.id);
  }

  return graph.nodes[0] ? taskNodeId(graph.nodes[0].id) : undefined;
}

function groupDecisionsByTaskId(
  decisions: ProjectDecisionRequest[],
): Map<string, ProjectDecisionRequest[]> {
  const decisionsByTaskId = new Map<string, ProjectDecisionRequest[]>();

  for (const decision of decisions) {
    if (!decision.project_task_id) continue;

    const current = decisionsByTaskId.get(decision.project_task_id) ?? [];
    decisionsByTaskId.set(decision.project_task_id, [...current, decision]);
  }

  return decisionsByTaskId;
}

function isPendingTaskDecision(decision: ProjectDecisionRequest): boolean {
  return Boolean(
    decision.project_task_id &&
      PENDING_DECISION_STATUSES.has(normalizeStatus(decision.status_snapshot)),
  );
}

function knownStageRange(nodes: ProjectTaskGraphNode[]): { min: number; max: number } {
  let min = Number.POSITIVE_INFINITY;
  let max = Number.NEGATIVE_INFINITY;

  for (const node of nodes) {
    const stage = finiteStage(node.stage_index);
    if (stage === undefined) continue;

    min = Math.min(min, stage);
    max = Math.max(max, stage);
  }

  if (!Number.isFinite(min) || !Number.isFinite(max)) return { min: 0, max: 0 };

  return { min, max };
}

function stageColumnX(stage: number, stageRange: { min: number; max: number }): number {
  return Math.max(stage - stageRange.min, 0) * STAGE_X;
}

function finiteStage(stage: number | undefined): number | undefined {
  return typeof stage === "number" && Number.isFinite(stage) ? stage : undefined;
}

function normalizeStatus(status: string): string {
  return status.toLowerCase();
}
