import type { DigitalEmployeeOperationalStatus } from "@/lib/api/employees";

// 未归属团队的伪团队 ID：这些员工固定落座候岗区，不参与团队容量统计。
export const UNASSIGNED_TEAM_ID = "unassigned";

export type RuntimeOverviewWorkspaceCapacity = 3 | 4 | 6 | 8 | 10;

// "lobby" 是独立候岗楼层：只承载未归属团队的员工，无团队工位，仅在有候岗员工时出现在楼层列表。
export type RuntimeOverviewFloorId = "floor-1" | "floor-2" | "floor-3" | "lobby";

export type RuntimeOverviewSummary = {
  teamCount: number;
  employeeCount: number;
  capacityUsed: number;
  capacityTotal: number;
  workingCount: number;
  idleCount: number;
  waitingHumanCount: number;
  queuedCount: number;
  needsConfigurationCount: number;
  unavailableCount: number;
  errorCount: number;
  cumulativeTaskCount: number;
  linkedProjectCount: number;
  todayTokensTotal: number;
};

export type RuntimeOverviewActivityItem = {
  employeeId: string;
  employeeName: string;
  teamId: string;
  label: string;
  status: string;
  occurredAt?: string;
  taskTitle?: string;
  projectName?: string;
};

export type RuntimeOverviewSeat = {
  seatId: string;
  x: number;
  y: number;
  rotation?: number;
  employeeId?: string;
};

export type RuntimeOverviewTeamWorkspace = {
  teamId: string;
  // 团队工位才有容量；大厅候岗区是动态承载区，不设物理席位容量。
  capacity: RuntimeOverviewWorkspaceCapacity | null;
  polygon: Array<{ x: number; y: number }>;
  cardAnchor: { x: number; y: number };
  calloutTarget: { x: number; y: number };
  seats: RuntimeOverviewSeat[];
  decorationVariant: "standard" | "lab" | "ops" | "review" | "data" | "lobby";
};

export type RuntimeOverviewPath = {
  id: string;
  points: Array<{ x: number; y: number }>;
  tone: "primary" | "muted" | "warning";
};

export type RuntimeOverviewFloor = {
  floorId: RuntimeOverviewFloorId;
  label: string;
  teamIds: string[];
  summary: {
    teamCount: number;
    errorCount: number;
    capacityUsed: number;
    capacityTotal: number;
  };
  layout: {
    backgroundImageUrl: string;
    canvasWidth: number;
    canvasHeight: number;
    paths: RuntimeOverviewPath[];
    teamWorkspaces: RuntimeOverviewTeamWorkspace[];
  };
};

export type RuntimeOverviewTeam = {
  teamId: string;
  floorId: RuntimeOverviewFloorId;
  name: string;
  capacity: RuntimeOverviewWorkspaceCapacity;
  capacityUsed: number;
  employeeCount: number;
  workingCount: number;
  idleCount: number;
  waitingHumanCount: number;
  queuedCount: number;
  errorCount: number;
  overCapacity: boolean;
};

export type RuntimeOverviewEmployeeProject = {
  projectId: string;
  name: string;
  status: string;
  isMember: boolean;
  activeTaskCount: number;
  workingTaskCount: number;
  totalTaskCount: number;
  lastActivityAt?: string;
};

export type RuntimeOverviewEmployee = {
  employeeId: string;
  teamId: string;
  floorId: RuntimeOverviewFloorId;
  seatId?: string;
  name: string;
  roleLabel: string;
  avatarAsset?: {
    id: string;
    url?: string;
    fallbackLabel?: string;
  };
  status: DigitalEmployeeOperationalStatus;
  // 状态原因（如"等待人工确认后继续执行"/"Provider 执行失败或不可用"），供运行快照卡解释当前状态。
  statusReasons: string[];
  // 当前状态的近似起始时间：working 取运行开始时间，其余取最近一次活动时间。
  statusSince?: string;
  latestRunErrorMessage?: string;
  currentTask?: {
    taskId: string;
    title: string;
    priority?: "low" | "medium" | "high";
    // working 成因所属项目(来自后端权威 current_work);用于座位卡精确深链到该项目。
    // 缺省(如仅有 latest_run 兜底)时无项目关联,座位卡回退到项目聚合启发式。
    projectId?: string;
    projectName?: string;
  };
  runtime?: {
    nodeId?: string;
    providerType?: string;
    sessionId?: string;
  };
  recentEvents: Array<{ label: string; status: string; occurredAt?: string }>;
  projects: RuntimeOverviewEmployeeProject[];
  projectCount: number;
  artifacts: Array<{ id: string; name: string; sizeLabel?: string; status?: string }>;
  usage?: {
    taskTokens?: number;
    dailyTokens?: number;
    dailyTokenLimit?: number;
  };
};

export type RuntimeOverviewDTO = {
  generatedAt: string;
  activeFloorId: RuntimeOverviewFloorId;
  summary: RuntimeOverviewSummary;
  floors: RuntimeOverviewFloor[];
  teams: RuntimeOverviewTeam[];
  employees: RuntimeOverviewEmployee[];
  recentActivity: RuntimeOverviewActivityItem[];
  selectedEmployeeId?: string;
};
