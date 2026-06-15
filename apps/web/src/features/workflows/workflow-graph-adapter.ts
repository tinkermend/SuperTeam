import type { Edge, Node } from "@xyflow/react";

import type {
  ProjectDecisionRequest,
  ProjectTaskGraph,
  ProjectTaskGraphNode,
} from "@/lib/api/projects";

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
  employeeName: string | undefined;
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

export type WorkflowAttachmentNodeData = {
  status: string;
  title: string;
  type: "decision";
};

type WorkflowTaskNode = Node<WorkflowTaskNodeData, "workflowTask">;
type WorkflowAttachmentNode = Node<WorkflowAttachmentNodeData, "workflowAttachment">;
type WorkflowGraphNode = WorkflowTaskNode | WorkflowAttachmentNode;

export type WorkflowGraphElements = {
  nodes: WorkflowGraphNode[];
  edges: Edge[];
};

export function taskNodeId(taskId: string): string {
  return `task:${taskId}`;
}

export function buildWorkflowGraphElements(graph: ProjectTaskGraph): WorkflowGraphElements {
  const taskIds = new Set(graph.nodes.map((task) => task.id));
  const employeesById = new Map(
    graph.employees.map((employee) => [employee.digital_employee_id, employee.display_name]),
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
        x: Math.max(stage - stageRange.min, 0) * STAGE_X,
        y: row * ROW_Y,
      },
      data: {
        employeeName: task.assigned_digital_employee_id
          ? employeesById.get(task.assigned_digital_employee_id)
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
    edges: graph.edges
      .filter(
        (edge) => taskIds.has(edge.blocker_task_id) && taskIds.has(edge.dependent_task_id),
      )
      .map((edge) => ({
        id: `edge:${edge.blocker_task_id}:${edge.dependent_task_id}`,
        source: taskNodeId(edge.blocker_task_id),
        target: taskNodeId(edge.dependent_task_id),
        type: "smoothstep",
        label: edge.edge_status,
        animated: ANIMATED_EDGE_STATUSES.has(normalizeStatus(edge.edge_status)),
      })),
  };
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

function finiteStage(stage: number | undefined): number | undefined {
  return typeof stage === "number" && Number.isFinite(stage) ? stage : undefined;
}

function normalizeStatus(status: string): string {
  return status.toLowerCase();
}
