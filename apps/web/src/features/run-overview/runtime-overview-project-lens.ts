import type { ProjectTaskGraph, ProjectTaskGraphNode } from "@/lib/api/projects";
import type { RuntimeOverviewDTO, RuntimeOverviewFloorId } from "./runtime-overview-model";
import { runtimeOverviewLobbyPositions } from "./runtime-overview-layout";

// 项目透镜：把项目任务图投影成地图上的"参与者高亮 + 座位间交接连线"。
// 连线语义是任务依赖/交接，不承诺执行时序（并行分支下依赖边 ≠ 实际先后）。
export type ProjectLensEdgeTone = "muted" | "primary" | "warning";

export type ProjectLensEdge = {
  id: string;
  fromEmployeeId: string;
  toEmployeeId: string;
  tone: ProjectLensEdgeTone;
  // 同端点对的依赖边合并计数，避免重线。
  taskCount: number;
};

export type ProjectLens = {
  projectId: string;
  participantEmployeeIds: string[];
  // 当前停留环节的员工：有运行中或被阻塞任务，地图上加脉冲标记。
  stopEmployeeIds: string[];
  edges: ProjectLensEdge[];
  totalTaskCount: number;
  unassignedTaskCount: number;
  blockedTaskCount: number;
};

// 状态桶沿用 plan-task-graph 的分类口径。
const COMPLETED_STATUSES = new Set(["completed", "accepted", "approved", "done", "success"]);
const BLOCKED_STATUSES = new Set(["failed", "rejected", "cancelled", "blocked"]);
const ACTIVE_STATUSES = new Set(["dispatchable", "running", "in_progress", "waiting_human"]);

const TONE_PRIORITY: Record<ProjectLensEdgeTone, number> = { muted: 0, primary: 1, warning: 2 };

function isBlockedNode(node: ProjectTaskGraphNode): boolean {
  return Boolean(node.current_blocker) || BLOCKED_STATUSES.has(node.status);
}

// 边色调看下游任务：阻塞 warning > 活跃 primary > 其余（已完成/未开始）muted。
function edgeTone(dependent: ProjectTaskGraphNode): ProjectLensEdgeTone {
  if (isBlockedNode(dependent)) return "warning";
  if (ACTIVE_STATUSES.has(dependent.status)) return "primary";
  return "muted";
}

export function buildProjectLens(projectId: string, graph: ProjectTaskGraph): ProjectLens {
  const nodesById = new Map(graph.nodes.map((node) => [node.id, node]));
  const edgesByPair = new Map<string, ProjectLensEdge>();
  for (const edge of graph.edges) {
    const blocker = nodesById.get(edge.blocker_task_id);
    const dependent = nodesById.get(edge.dependent_task_id);
    const from = blocker?.assigned_digital_employee_id;
    const to = dependent?.assigned_digital_employee_id;
    // 端点未派发或自环（同员工承接上下游）不画线；未派发任务另行计数呈现。
    if (!dependent || !from || !to || from === to) continue;
    const key = `${from}->${to}`;
    const tone = edgeTone(dependent);
    const existing = edgesByPair.get(key);
    if (existing) {
      existing.taskCount += 1;
      if (TONE_PRIORITY[tone] > TONE_PRIORITY[existing.tone]) existing.tone = tone;
    } else {
      edgesByPair.set(key, { id: key, fromEmployeeId: from, toEmployeeId: to, tone, taskCount: 1 });
    }
  }

  const participantIds = new Set<string>();
  for (const employee of graph.employees) participantIds.add(employee.digital_employee_id);
  const stopIds = new Set<string>();
  let unassignedTaskCount = 0;
  let blockedTaskCount = 0;
  for (const node of graph.nodes) {
    const assignee = node.assigned_digital_employee_id;
    if (assignee) participantIds.add(assignee);
    else if (!COMPLETED_STATUSES.has(node.status)) unassignedTaskCount += 1;
    if (isBlockedNode(node)) blockedTaskCount += 1;
    if (assignee && (isBlockedNode(node) || ACTIVE_STATUSES.has(node.status))) stopIds.add(assignee);
  }

  return {
    projectId,
    participantEmployeeIds: [...participantIds],
    stopEmployeeIds: [...stopIds],
    edges: [...edgesByPair.values()],
    totalTaskCount: graph.nodes.length,
    unassignedTaskCount,
    blockedTaskCount,
  };
}

// —— 座位坐标解算：把员工级的边落到当前楼层的可绘制线段/跨层出口 ——

type LensPoint = { x: number; y: number };

export type ProjectLensDrawableEdge = {
  id: string;
  from: LensPoint;
  to: LensPoint;
  tone: ProjectLensEdgeTone;
  taskCount: number;
};

export type ProjectLensPortal = {
  id: string;
  at: LensPoint;
  employeeId: string;
  targetFloorId: RuntimeOverviewFloorId;
  direction: "outgoing" | "incoming";
  tone: ProjectLensEdgeTone;
};

export type ProjectLensFloorProjection = {
  fullEdges: ProjectLensDrawableEdge[];
  portals: ProjectLensPortal[];
  // 端点员工不在总览列表内或无座位（候岗溢出）的边：不画线，留计数供文案兜底。
  unlocatedEdgeCount: number;
};

type SeatLocation = LensPoint & { floorId: RuntimeOverviewFloorId };

function seatLocationsByEmployee(overview: RuntimeOverviewDTO): Map<string, SeatLocation> {
  const seatById = new Map<string, SeatLocation>();
  for (const floor of overview.floors) {
    for (const workspace of floor.layout.teamWorkspaces) {
      for (const seat of workspace.seats) {
        seatById.set(seat.seatId, { x: seat.x, y: seat.y, floorId: floor.floorId });
      }
    }
  }
  const byEmployee = new Map<string, SeatLocation>();
  // 候岗员工没有 seatId：按其在大厅在场顺序对应 LobbyWorkspaceRenderer 的展示锚点
  // （同源同序），使连线端点落在头像实际位置；超出锚点数的溢出员工保持无定位。
  let lobbyIndex = 0;
  for (const employee of overview.employees) {
    if (employee.floorId === "lobby") {
      const anchor = runtimeOverviewLobbyPositions[lobbyIndex];
      lobbyIndex += 1;
      if (anchor) byEmployee.set(employee.employeeId, { x: anchor.x, y: anchor.y, floorId: "lobby" });
      continue;
    }
    if (!employee.seatId) continue;
    const location = seatById.get(employee.seatId);
    if (location) byEmployee.set(employee.employeeId, location);
  }
  return byEmployee;
}

export function projectLensForFloor(
  lens: ProjectLens,
  overview: RuntimeOverviewDTO,
  floorId: RuntimeOverviewFloorId,
): ProjectLensFloorProjection {
  const locations = seatLocationsByEmployee(overview);
  const fullEdges: ProjectLensDrawableEdge[] = [];
  const portalsByKey = new Map<string, ProjectLensPortal>();
  let unlocatedEdgeCount = 0;
  for (const edge of lens.edges) {
    const from = locations.get(edge.fromEmployeeId);
    const to = locations.get(edge.toEmployeeId);
    if (!from || !to) {
      unlocatedEdgeCount += 1;
      continue;
    }
    if (from.floorId === floorId && to.floorId === floorId) {
      fullEdges.push({ id: edge.id, from, to, tone: edge.tone, taskCount: edge.taskCount });
      continue;
    }
    // 跨楼层：在场端点收一个出口徽标（同员工同目标层去重，色调取更高优先级）。
    const onFloorEnd = from.floorId === floorId ? from : to.floorId === floorId ? to : undefined;
    if (!onFloorEnd) continue;
    const direction = from.floorId === floorId ? "outgoing" : "incoming";
    const employeeId = direction === "outgoing" ? edge.fromEmployeeId : edge.toEmployeeId;
    const targetFloorId = direction === "outgoing" ? to.floorId : from.floorId;
    const key = `${employeeId}:${targetFloorId}:${direction}`;
    const existing = portalsByKey.get(key);
    if (existing) {
      if (TONE_PRIORITY[edge.tone] > TONE_PRIORITY[existing.tone]) existing.tone = edge.tone;
    } else {
      portalsByKey.set(key, {
        id: key,
        at: { x: onFloorEnd.x, y: onFloorEnd.y },
        employeeId,
        targetFloorId,
        direction,
        tone: edge.tone,
      });
    }
  }
  return { fullEdges, portals: [...portalsByKey.values()], unlocatedEdgeCount };
}

// 项目选择器数据源：从总览员工的项目摘要反向聚合。只有关联到可见员工的项目才可能
// 在地图上点亮，所以不额外拉项目列表；计数是员工侧视角的求和，仅用于排序与提示。
export type LensProjectOption = {
  projectId: string;
  name: string;
  status: string;
  participantCount: number;
  activeTaskCount: number;
  workingTaskCount: number;
  lastActivityAt?: string;
};

export function aggregateLensProjectOptions(overview: RuntimeOverviewDTO): LensProjectOption[] {
  const byProject = new Map<string, LensProjectOption>();
  for (const employee of overview.employees) {
    for (const project of employee.projects) {
      const existing = byProject.get(project.projectId);
      if (existing) {
        existing.participantCount += 1;
        existing.activeTaskCount += project.activeTaskCount;
        existing.workingTaskCount += project.workingTaskCount;
        if (project.lastActivityAt && (!existing.lastActivityAt || project.lastActivityAt > existing.lastActivityAt)) {
          existing.lastActivityAt = project.lastActivityAt;
        }
      } else {
        byProject.set(project.projectId, {
          projectId: project.projectId,
          name: project.name,
          status: project.status,
          participantCount: 1,
          activeTaskCount: project.activeTaskCount,
          workingTaskCount: project.workingTaskCount,
          lastActivityAt: project.lastActivityAt,
        });
      }
    }
  }
  return [...byProject.values()].sort(
    (a, b) =>
      b.workingTaskCount - a.workingTaskCount ||
      b.activeTaskCount - a.activeTaskCount ||
      (b.lastActivityAt ?? "").localeCompare(a.lastActivityAt ?? "") ||
      a.name.localeCompare(b.name),
  );
}

export function lensParticipantFloorIds(lens: ProjectLens, overview: RuntimeOverviewDTO): RuntimeOverviewFloorId[] {
  const participantIds = new Set(lens.participantEmployeeIds);
  const floorIds = new Set<RuntimeOverviewFloorId>();
  for (const employee of overview.employees) {
    if (participantIds.has(employee.employeeId)) floorIds.add(employee.floorId);
  }
  return [...floorIds];
}
