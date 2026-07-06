import type { DigitalEmployeeOperationalStatus } from "@/lib/api/employees";

export type RuntimeOverviewWorkspaceCapacity = 3 | 4 | 6 | 8 | 10;

export type RuntimeOverviewFloorId = "floor-1" | "floor-2" | "floor-3";

export type RuntimeOverviewSummary = {
  teamCount: number;
  employeeCount: number;
  capacityUsed: number;
  capacityTotal: number;
  workingCount: number;
  idleCount: number;
  waitingHumanCount: number;
  queuedCount: number;
  errorCount: number;
  cumulativeTaskCount: number;
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
  capacity: RuntimeOverviewWorkspaceCapacity;
  polygon: Array<{ x: number; y: number }>;
  cardAnchor: { x: number; y: number };
  calloutTarget: { x: number; y: number };
  seats: RuntimeOverviewSeat[];
  decorationVariant: "standard" | "lab" | "ops" | "review" | "data";
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
  currentTask?: {
    taskId: string;
    title: string;
    priority?: "low" | "medium" | "high";
  };
  runtime?: {
    nodeId?: string;
    providerType?: string;
    sessionId?: string;
  };
  recentEvents: Array<{ label: string; status: string; occurredAt?: string }>;
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
  selectedEmployeeId?: string;
};
