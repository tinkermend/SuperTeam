import type {
  DigitalEmployeeOperationalStatus,
  DigitalEmployeeOverview,
  DigitalEmployeeOverviewItem,
} from "@/lib/api/employees";
import type { TeamListItem } from "@/lib/api/teams";
import {
  type RuntimeOverviewDTO,
  type RuntimeOverviewEmployee,
  type RuntimeOverviewFloor,
  type RuntimeOverviewFloorId,
  type RuntimeOverviewTeam,
  type RuntimeOverviewWorkspaceCapacity,
} from "./runtime-overview-model";
import { buildFloorLayouts } from "./runtime-overview-layout";

const FALLBACK_TEAM_CAPACITY: RuntimeOverviewWorkspaceCapacity = 10;

type BuildRuntimeOverviewInput = {
  activeFloorId: RuntimeOverviewFloorId;
  employees: DigitalEmployeeOverview;
  generatedAt: string;
  teams: TeamListItem[];
};

export function buildRuntimeOverview(input: BuildRuntimeOverviewInput): RuntimeOverviewDTO {
  const activeTeams = input.teams.filter((team) => team.status === "active");
  const teamIdsByFloor = distributeTeamsByFloor(activeTeams);
  const floors = buildFloorLayouts(teamIdsByFloor);
  const floorIdByTeamId = new Map(
    floors.flatMap((floor) => floor.teamIds.map((teamId) => [teamId, floor.floorId] as const)),
  );
  const workspaceByTeamId = new Map(
    floors.flatMap((floor) =>
      floor.layout.teamWorkspaces.map((workspace) => [workspace.teamId, { floorId: floor.floorId, workspace }] as const),
    ),
  );
  const employeesByTeam = groupEmployeesByTeam(input.employees.items);
  const overviewTeams = activeTeams.map((team): RuntimeOverviewTeam => {
    const items = employeesByTeam.get(team.id) ?? [];
    const employeeCount = items.length || team.digital_employee_count;
    const capacity = workspaceByTeamId.get(team.id)?.workspace.capacity ?? FALLBACK_TEAM_CAPACITY;
    return {
      teamId: team.id,
      floorId: floorIdByTeamId.get(team.id) ?? "floor-1",
      name: team.name,
      capacity,
      employeeCount,
      workingCount: countStatus(items, "working"),
      idleCount: countStatus(items, "idle"),
      waitingHumanCount: countStatus(items, "waiting_human"),
      queuedCount: countStatus(items, "queued"),
      errorCount: countStatus(items, "error"),
      overCapacity: employeeCount > capacity,
    };
  });
  const overviewEmployees = input.employees.items.map((item): RuntimeOverviewEmployee => {
    const teamId = item.identity_summary.team_id ?? "unassigned";
    const workspace = workspaceByTeamId.get(teamId);
    const floorId = floorIdByTeamId.get(teamId) ?? workspace?.floorId ?? "floor-1";
    const teamItems = employeesByTeam.get(teamId) ?? [];
    const seatIndex = teamItems.findIndex((employee) => employee.identity_summary.id === item.identity_summary.id);
    const seat = seatIndex >= 0 ? workspace?.workspace.seats[seatIndex] : undefined;
    return employeePresenceFromItem(item, floorId, seat?.seatId);
  });
  const capacityUsed = overviewTeams.reduce((sum, team) => sum + team.employeeCount, 0);
  const capacityTotal = overviewTeams.reduce((sum, team) => sum + team.capacity, 0);

  return {
    activeFloorId: input.activeFloorId,
    employees: overviewEmployees,
    floors: enrichFloorSummaries(floors, overviewTeams),
    generatedAt: input.generatedAt,
    selectedEmployeeId: overviewEmployees[0]?.employeeId,
    summary: {
      teamCount: activeTeams.length,
      employeeCount: input.employees.pagination.total_count,
      capacityUsed,
      capacityTotal,
      workingCount: input.employees.summary.operational_status_counts.working ?? countEmployees(overviewEmployees, "working"),
      idleCount: input.employees.summary.operational_status_counts.idle ?? countEmployees(overviewEmployees, "idle"),
      waitingHumanCount:
        input.employees.summary.operational_status_counts.waiting_human ?? countEmployees(overviewEmployees, "waiting_human"),
      queuedCount: input.employees.summary.operational_status_counts.queued ?? countEmployees(overviewEmployees, "queued"),
      errorCount: input.employees.summary.operational_status_counts.error ?? countEmployees(overviewEmployees, "error"),
      cumulativeTaskCount: input.employees.items.filter((item) => item.latest_run_summary).length,
    },
    teams: overviewTeams,
  };
}

function distributeTeamsByFloor(teams: TeamListItem[]): Record<RuntimeOverviewFloorId, string[]> {
  const floors: Record<RuntimeOverviewFloorId, string[]> = { "floor-1": [], "floor-2": [], "floor-3": [] };
  teams.forEach((team, index) => {
    const floorId: RuntimeOverviewFloorId = index < 8 ? "floor-1" : index < 16 ? "floor-2" : "floor-3";
    floors[floorId].push(team.id);
  });
  return floors;
}

function groupEmployeesByTeam(items: DigitalEmployeeOverviewItem[]) {
  const grouped = new Map<string, DigitalEmployeeOverviewItem[]>();
  for (const item of items) {
    const teamId = item.identity_summary.team_id ?? "unassigned";
    grouped.set(teamId, [...(grouped.get(teamId) ?? []), item]);
  }
  return grouped;
}

function employeePresenceFromItem(item: DigitalEmployeeOverviewItem, floorId: RuntimeOverviewFloorId, seatId?: string): RuntimeOverviewEmployee {
  const latestRun = item.latest_run_summary;
  return {
    employeeId: item.identity_summary.id,
    teamId: item.identity_summary.team_id ?? "unassigned",
    floorId,
    seatId,
    name: item.identity_summary.name,
    roleLabel: item.identity_summary.role,
    avatarAsset: item.identity_summary.avatar_asset
      ? {
          id: item.identity_summary.avatar_asset.id,
          url: item.identity_summary.avatar_asset.thumbnail_url || item.identity_summary.avatar_asset.image_url,
          fallbackLabel: item.identity_summary.name.slice(0, 1),
        }
      : undefined,
    status: item.operational_state.status,
    currentTask: latestRun
      ? {
          taskId: latestRun.task_id,
          title: latestRun.title,
          priority: item.identity_summary.risk_level === "high" ? "high" : "medium",
        }
      : undefined,
    runtime: {
      nodeId: item.execution_summary.node_id,
      providerType: item.execution_summary.provider_type,
    },
    recentEvents: item.recent_events.map((event) => ({
      label: event.label,
      status: event.status,
      occurredAt: event.occurred_at,
    })),
    artifacts: [],
    usage: {
      taskTokens: latestRun?.token_usage,
      dailyTokens: item.budget_summary.usage_tokens_today,
      dailyTokenLimit: item.budget_summary.daily_token_limit ?? undefined,
    },
  };
}

function enrichFloorSummaries(floors: RuntimeOverviewFloor[], teams: RuntimeOverviewTeam[]): RuntimeOverviewFloor[] {
  return floors.map((floor) => {
    const floorTeams = teams.filter((team) => floor.teamIds.includes(team.teamId));
    return {
      ...floor,
      summary: {
        teamCount: floorTeams.length,
        errorCount: floorTeams.reduce((sum, team) => sum + team.errorCount, 0),
        capacityUsed: floorTeams.reduce((sum, team) => sum + team.employeeCount, 0),
        capacityTotal: floorTeams.reduce((sum, team) => sum + team.capacity, 0),
      },
    };
  });
}

function countStatus(items: DigitalEmployeeOverviewItem[], status: DigitalEmployeeOperationalStatus) {
  return items.filter((item) => item.operational_state.status === status).length;
}

function countEmployees(items: RuntimeOverviewEmployee[], status: DigitalEmployeeOperationalStatus) {
  return items.filter((item) => item.status === status).length;
}
