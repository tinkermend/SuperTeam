import type { Edge, Node } from "@xyflow/react";

import type {
  ProjectDecisionRequest,
  ProjectTaskGraph,
  ProjectTaskGraphEmployee,
  ProjectTaskGraphNode,
  ProjectTaskGraphRun,
} from "@/lib/api/projects";
import { taskStatusLabel } from "@/lib/status-labels";

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

// —— 活性边推导用状态集（spec 2026-07-27 §1.1：视觉状态严格从权威任务/运行状态推导）。
const FAILED_TASK_STATUSES = new Set(["failed"]);
const CANCELLED_TASK_STATUSES = new Set(["cancelled"]);
const RUNNING_TASK_STATUSES = new Set(["running", "in_progress"]);
const RUNNING_RUN_STATUSES = new Set(["running", "in_progress", "started"]);
const COMPLETED_TASK_STATUSES = new Set(["completed", "done", "accepted"]);
/** 下游"正在消费交接物"的活跃态：已在跑或已被派上执行位。 */
const DEPENDENT_ACTIVE_STATUSES = new Set([
  "running",
  "in_progress",
  "assigned",
  "dispatchable",
]);

/** 活性边的四态：flowing 粒子流动 / failed 红停流 / done 静态通电 / idle 灰待命。 */
export type FlowLiveEdgeActivity = "flowing" | "failed" | "done" | "idle";

/**
 * 活图动画规模阈值（spec 2026-07-27 §5 P2-S）：live 模式下渲染元素（节点+边）
 * 总数**超过**该值时，flowing 边自动从粒子流降级为呼吸边（与 prefers-reduced-motion
 * 走同一降级渲染分支，两条件取或）。导出可调；阈值内行为零变化。
 */
export const LIVE_ANIMATION_MAX_ELEMENTS = 40;

export type FlowLiveEdgeData = {
  activity: FlowLiveEdgeActivity;
  blockerTaskId: string;
  dependentTaskId: string;
  /** 大图性能降级（spec §5 P2-S）：仅在 live 且元素总数超阈值时置 true。 */
  scaleDegraded?: boolean;
};

export type WorkflowTaskNodeData = {
  avatarAsset: WorkflowTaskNodeAvatarAsset | undefined;
  employeeName: string | undefined;
  employeeRole: string | undefined;
  expectedOutputs: unknown[];
  hasPendingDecision: boolean;
  requiresHumanApproval: boolean;
  riskLevel: string | undefined;
  runStatus: string | undefined;
  /** 运行起止（优先 runs[] 投影，回退任务节点自身投影）；live 模式节点卡时间区数据源。 */
  runStartedAt: string | undefined;
  runFinishedAt: string | undefined;
  /** live 模式（spec 2026-07-27 §1.2）才渲染时间区，既有消费方零行为变化。 */
  showTiming: boolean;
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

export type WorkflowBlockingNodeData = {
  reasonCode: string;
  message: string;
  recommendedAction: string | undefined;
};

type WorkflowTaskNode = Node<WorkflowTaskNodeData, "workflowTask">;
type WorkflowAttachmentNode = Node<WorkflowAttachmentNodeData, "workflowAttachment">;
type WorkflowStageLabelNode = Node<WorkflowStageLabelNodeData, "workflowStageLabel">;
type WorkflowBlockingNode = Node<WorkflowBlockingNodeData, "workflowBlocking">;
type FlowGraphNode =
  | WorkflowTaskNode
  | WorkflowAttachmentNode
  | WorkflowStageLabelNode
  | WorkflowBlockingNode;

export type FlowGraphElements = {
  nodes: FlowGraphNode[];
  edges: Edge<FlowLiveEdgeData>[];
  /** 大图性能降级判定（spec §5 P2-S）：live 且节点+边总数超阈值；非 live 恒 false。 */
  scaleDegraded: boolean;
};

export type WorkflowStageLabelNodeData = {
  employeeCount: number;
  stageIndex: number;
  taskCount: number;
  title: string;
};

export type FlowGraphBuildOptions = {
  /** 空图但存在 blocking_facts 时渲染协调阻塞兜底节点（默认开）。 */
  includeBlockingFallback?: boolean;
  /** pending 人工决策渲染为任务挂饰节点（默认开）。 */
  includeDecisionAttachments?: boolean;
  /**
   * 活图模式（spec 2026-07-27）：边切换为 flowLive 自定义 edge（粒子流动），
   * 节点卡开启时间区。默认关——既有消费方零行为变化。
   */
  live?: boolean;
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

/** 画布节点 id 还原任务 id；非任务节点（阶段标签/阻塞兜底/挂饰）返回 undefined。 */
export function taskIdFromNodeId(nodeId: string): string | undefined {
  return nodeId.startsWith("task:") ? nodeId.slice("task:".length) : undefined;
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

/**
 * task graph → xyflow 元素的唯一权威 build 函数（spec 2026-07-26 §4.2）。
 * 项目详情与流程编排详情消费同一投影：同一布局、同一状态词/色、同一挂饰语义；
 * blocking/attachment 渲染由参数开关（默认全开）。
 */
export function buildFlowGraphElements(
  graph: ProjectTaskGraph,
  {
    includeBlockingFallback = true,
    includeDecisionAttachments = true,
    live = false,
  }: FlowGraphBuildOptions = {},
): FlowGraphElements {
  if (graph.nodes.length === 0 && graph.blocking_facts.length > 0) {
    if (!includeBlockingFallback) {
      return { nodes: [], edges: [], scaleDegraded: false };
    }
    const fact = graph.blocking_facts[0];
    return {
      nodes: [
        {
          id: `blocking-${fact.reason_code}`,
          type: "workflowBlocking",
          position: { x: -180, y: 96 },
          data: {
            reasonCode: fact.reason_code,
            message: fact.message,
            recommendedAction: fact.recommended_action,
          },
        },
      ],
      edges: [],
      scaleDegraded: false,
    };
  }

  const taskIds = new Set(graph.nodes.map((task) => task.id));
  const employeesById = new Map(
    graph.employees.map((employee) => [employee.digital_employee_id, employee]),
  );
  const runsByTaskId = new Map(graph.runs.map((run) => [run.project_task_id, run]));
  const pendingDecisions = graph.decision_requests.filter(
    (decision) => isPendingTaskDecision(decision) && taskIds.has(decision.project_task_id ?? ""),
  );
  const pendingDecisionsByTaskId = groupDecisionsByTaskId(pendingDecisions);
  const layoutNodes = buildDynamicTaskLayoutNodes(graph, {
    employeesById,
    live,
    pendingDecisionsByTaskId,
    runsByTaskId,
  });

  const attachmentCountsByTaskId = new Map<string, number>();
  const attachmentNodes: WorkflowAttachmentNode[] = includeDecisionAttachments
    ? pendingDecisions.map((decision) => {
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
        } satisfies WorkflowAttachmentNode;
      })
    : [];

  const nodes = [...layoutNodes, ...attachmentNodes];
  const edges = buildTaskDependencyEdges(graph, taskIds, { live, runsByTaskId });
  // 大图性能分层（spec §5 P2-S）：live 模式渲染元素总数超阈值时，全部边打降级位，
  // flowing 边在 FlowLiveEdge 内走与 reduced-motion 相同的呼吸边分支。
  const scaleDegraded =
    live && nodes.length + edges.length > LIVE_ANIMATION_MAX_ELEMENTS;

  return {
    nodes,
    edges: scaleDegraded
      ? edges.map((edge) =>
          edge.data
            ? { ...edge, data: { ...edge.data, scaleDegraded: true } }
            : edge,
        )
      : edges,
    scaleDegraded,
  };
}

function buildTaskDependencyEdges(
  graph: ProjectTaskGraph,
  taskIds: Set<string>,
  {
    live,
    runsByTaskId,
  }: { live: boolean; runsByTaskId: Map<string, ProjectTaskGraphRun> },
): Edge<FlowLiveEdgeData>[] {
  const statusByTaskId = new Map(graph.nodes.map((task) => [task.id, task.status]));
  return graph.edges
    .filter((edge) => taskIds.has(edge.blocker_task_id) && taskIds.has(edge.dependent_task_id))
    .map((edge) => ({
      id: `edge:${edge.blocker_task_id}:${edge.dependent_task_id}`,
      source: taskNodeId(edge.blocker_task_id),
      target: taskNodeId(edge.dependent_task_id),
      type: live ? "flowLive" : "smoothstep",
      label: taskStatusLabel(edge.edge_status),
      animated: !live && ANIMATED_EDGE_STATUSES.has(normalizeStatus(edge.edge_status)),
      data: {
        activity: deriveEdgeActivity({
          blockerRunStatus: runsByTaskId.get(edge.blocker_task_id)?.status,
          blockerStatus: statusByTaskId.get(edge.blocker_task_id),
          dependentStatus: statusByTaskId.get(edge.dependent_task_id),
        }),
        blockerTaskId: edge.blocker_task_id,
        dependentTaskId: edge.dependent_task_id,
      },
    }));
}

/**
 * 活性边四态推导（spec 2026-07-27 §1.1 拍板默认）：纯从权威任务/运行状态推导，
 * 不引入"交接不符"等新边语义。规则按优先级：
 * 1. 任一端 failed → failed（红停流）；
 * 2. 任一端 cancelled → idle（灰）；
 * 3. 上游任务 running/in_progress（或其 run 在跑）→ flowing（出边粒子流动）；
 * 4. 上游已完成且下游活跃（running/in_progress/assigned/dispatchable）→ flowing；
 * 5. 两端均完成 → done（静态已通电）；
 * 6. 其余 → idle。
 */
export function deriveEdgeActivity({
  blockerRunStatus,
  blockerStatus,
  dependentStatus,
}: {
  blockerRunStatus: string | undefined;
  blockerStatus: string | undefined;
  dependentStatus: string | undefined;
}): FlowLiveEdgeActivity {
  const blocker = normalizeStatus(blockerStatus ?? "");
  const dependent = normalizeStatus(dependentStatus ?? "");
  const blockerRun = normalizeStatus(blockerRunStatus ?? "");

  if (FAILED_TASK_STATUSES.has(blocker) || FAILED_TASK_STATUSES.has(dependent)) {
    return "failed";
  }
  if (CANCELLED_TASK_STATUSES.has(blocker) || CANCELLED_TASK_STATUSES.has(dependent)) {
    return "idle";
  }
  if (RUNNING_TASK_STATUSES.has(blocker) || RUNNING_RUN_STATUSES.has(blockerRun)) {
    return "flowing";
  }
  if (COMPLETED_TASK_STATUSES.has(blocker)) {
    if (DEPENDENT_ACTIVE_STATUSES.has(dependent)) return "flowing";
    if (COMPLETED_TASK_STATUSES.has(dependent)) return "done";
  }
  return "idle";
}

function buildDynamicTaskLayoutNodes(
  graph: ProjectTaskGraph,
  {
    employeesById,
    live,
    pendingDecisionsByTaskId,
    runsByTaskId,
  }: {
    employeesById: Map<string, ProjectTaskGraphEmployee>;
    live: boolean;
    pendingDecisionsByTaskId: Map<string, ProjectDecisionRequest[]>;
    runsByTaskId: Map<string, ProjectTaskGraphRun>;
  },
): (WorkflowTaskNode | WorkflowStageLabelNode)[] {
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
            runStatus: runsByTaskId.get(task.id)?.status,
            runStartedAt: runsByTaskId.get(task.id)?.started_at ?? task.started_at,
            runFinishedAt: runsByTaskId.get(task.id)?.finished_at ?? task.finished_at,
            showTiming: live,
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

  return nodes;
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

function finiteStage(stage: number | undefined): number | undefined {
  return typeof stage === "number" && Number.isFinite(stage) ? stage : undefined;
}

function normalizeStatus(status: string): string {
  return status.toLowerCase();
}
