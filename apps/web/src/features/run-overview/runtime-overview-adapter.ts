import type {
  DigitalEmployeeOperationalStatus,
  DigitalEmployeeOverview,
  DigitalEmployeeOverviewItem,
} from "@/lib/api/employees";
import type { TeamListItem } from "@/lib/api/teams";
import { overviewAvatarAsset } from "@/features/employees/avatar-library";
import {
  type RuntimeOverviewActivityItem,
  type RuntimeOverviewDTO,
  type RuntimeOverviewEmployee,
  type RuntimeOverviewFloor,
  type RuntimeOverviewFloorId,
  type RuntimeOverviewTeam,
  type RuntimeOverviewWorkspaceCapacity,
} from "./runtime-overview-model";
import { buildFloorLayouts, runtimeOverviewSlotCapacities } from "./runtime-overview-layout";

const FALLBACK_TEAM_CAPACITY: RuntimeOverviewWorkspaceCapacity = 10;

type BuildRuntimeOverviewInput = {
  activeFloorId: RuntimeOverviewFloorId;
  employees: DigitalEmployeeOverview;
  generatedAt: string;
  teams: TeamListItem[];
};

export function buildRuntimeOverview(input: BuildRuntimeOverviewInput): RuntimeOverviewDTO {
  const activeTeams = input.teams.filter((team) => team.status === "active");
  const employeesByTeam = groupEmployeesByTeam(input.employees.items);
  const teamIdsByFloor = distributeTeamsByFloor(activeTeams, employeesByTeam);
  const floors = buildFloorLayouts(teamIdsByFloor);
  const floorIdByTeamId = new Map(
    floors.flatMap((floor) => floor.teamIds.map((teamId) => [teamId, floor.floorId] as const)),
  );
  const workspaceByTeamId = new Map(
    floors.flatMap((floor) =>
      floor.layout.teamWorkspaces.map((workspace) => [workspace.teamId, { floorId: floor.floorId, workspace }] as const),
    ),
  );
  const overviewTeams = activeTeams.map((team): RuntimeOverviewTeam => {
    const items = employeesByTeam.get(team.id) ?? [];
    const employeeCount = items.length || team.digital_employee_count;
    const capacity = workspaceByTeamId.get(team.id)?.workspace.capacity ?? FALLBACK_TEAM_CAPACITY;
    const capacityUsed = Math.min(items.length, capacity);
    return {
      teamId: team.id,
      floorId: floorIdByTeamId.get(team.id) ?? "floor-1",
      name: team.name,
      capacity,
      capacityUsed,
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
  const capacityUsed = overviewTeams.reduce((sum, team) => sum + team.capacityUsed, 0);
  const capacityTotal = overviewTeams.reduce((sum, team) => sum + team.capacity, 0);
  const linkedProjectIds = new Set(
    input.employees.items.flatMap((item) => item.project_summary.projects.map((project) => project.project_id)),
  );
  const todayTokensTotal = input.employees.items.reduce((sum, item) => sum + item.budget_summary.usage_tokens_today, 0);

  return {
    activeFloorId: input.activeFloorId,
    employees: overviewEmployees,
    floors: enrichFloorSummaries(floors, overviewTeams),
    generatedAt: input.generatedAt,
    recentActivity: buildRecentActivity(input.employees.items),
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
      needsConfigurationCount:
        input.employees.summary.operational_status_counts.needs_configuration ??
        countEmployees(overviewEmployees, "needs_configuration"),
      unavailableCount:
        input.employees.summary.operational_status_counts.unavailable ?? countEmployees(overviewEmployees, "unavailable"),
      errorCount: input.employees.summary.operational_status_counts.error ?? countEmployees(overviewEmployees, "error"),
      cumulativeTaskCount: input.employees.items.filter((item) => item.latest_run_summary).length,
      linkedProjectCount: linkedProjectIds.size,
      todayTokensTotal,
    },
    teams: overviewTeams,
  };
}

function buildRecentActivity(items: DigitalEmployeeOverviewItem[]): RuntimeOverviewActivityItem[] {
  return items
    .flatMap((item) =>
      item.recent_events.map((event) => ({
        employeeId: item.identity_summary.id,
        employeeName: item.identity_summary.name,
        teamId: item.identity_summary.team_id ?? "unassigned",
        label: event.label,
        status: event.status,
        occurredAt: event.occurred_at,
      })),
    )
    .sort((a, b) => {
      if (!a.occurredAt) return 1;
      if (!b.occurredAt) return -1;
      return b.occurredAt.localeCompare(a.occurredAt);
    })
    .slice(0, 8);
}

function distributeTeamsByFloor(
  teams: TeamListItem[],
  employeesByTeam: Map<string, DigitalEmployeeOverviewItem[]>,
): Record<RuntimeOverviewFloorId, string[]> {
  const floors: Record<RuntimeOverviewFloorId, string[]> = { "floor-1": [], "floor-2": [], "floor-3": [] };
  const capacities = runtimeOverviewSlotCapacities();
  const slots = (Object.keys(capacities) as RuntimeOverviewFloorId[]).flatMap((floorId) =>
    capacities[floorId].map((capacity, index) => ({ floorId, index, capacity })),
  );
  const teamsByRequiredCapacity = teams
    .map((team, index) => ({ index, requiredCapacity: teamRequiredCapacity(team, employeesByTeam), team }))
    .sort((a, b) => b.requiredCapacity - a.requiredCapacity || a.index - b.index);

  for (const { team, requiredCapacity } of teamsByRequiredCapacity) {
    if (slots.length === 0) {
      floors["floor-3"].push(team.id);
      continue;
    }
    const matchingSlotIndex = slots.findIndex((slot) => slot.capacity >= requiredCapacity);
    const slotIndex = matchingSlotIndex >= 0 ? matchingSlotIndex : largestAvailableSlotIndex(slots);
    const [slot] = slots.splice(slotIndex, 1);
    if (!slot) break;
    floors[slot.floorId][slot.index] = team.id;
  }
  return floors;
}

function teamRequiredCapacity(team: TeamListItem, employeesByTeam: Map<string, DigitalEmployeeOverviewItem[]>) {
  const fetchedEmployeeCount = employeesByTeam.get(team.id)?.length ?? 0;
  return fetchedEmployeeCount > 0 ? fetchedEmployeeCount : team.digital_employee_count;
}

function largestAvailableSlotIndex(slots: Array<{ capacity: RuntimeOverviewWorkspaceCapacity }>) {
  return slots.reduce((largestIndex, slot, index) => (slot.capacity > slots[largestIndex].capacity ? index : largestIndex), 0);
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
  const avatarAsset = overviewAvatarAsset(item);
  return {
    employeeId: item.identity_summary.id,
    teamId: item.identity_summary.team_id ?? "unassigned",
    floorId,
    seatId,
    name: item.identity_summary.name,
    roleLabel: item.identity_summary.role,
    avatarAsset: {
      id: avatarAsset.id,
      url: avatarAsset.thumbnail_url || avatarAsset.image_url,
      fallbackLabel: item.identity_summary.name.slice(0, 1),
    },
    status: item.operational_state.status,
    statusReasons: item.operational_state.reasons.map((reason) => reason.message),
    statusSince: statusSinceFromItem(item),
    latestRunErrorMessage: latestRun?.error_message || undefined,
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
    projects: item.project_summary.projects.map((project) => ({
      projectId: project.project_id,
      name: project.name,
      status: project.status,
      isMember: project.is_member,
      activeTaskCount: project.active_task_count,
      workingTaskCount: project.working_task_count,
      totalTaskCount: project.total_task_count,
      lastActivityAt: project.last_activity_at,
    })),
    projectCount: item.project_summary.project_count,
    artifacts: [],
    usage: {
      taskTokens: latestRun?.token_usage,
      dailyTokens: item.budget_summary.usage_tokens_today,
      dailyTokenLimit: item.budget_summary.daily_token_limit ?? undefined,
    },
  };
}

// 状态起始时间近似：working 用运行开始时间；其余状态没有精确的状态迁移时间戳，
// 用"最近一次活动"（事件/运行结束/更新时间的最大值）近似。
function statusSinceFromItem(item: DigitalEmployeeOverviewItem): string | undefined {
  const run = item.latest_run_summary;
  if (item.operational_state.status === "working" && run?.started_at) return run.started_at;
  const candidates = [
    ...item.recent_events.map((event) => event.occurred_at),
    run?.finished_at,
    run?.updated_at,
    run?.started_at,
  ].filter((value): value is string => Boolean(value));
  if (candidates.length === 0) return undefined;
  return candidates.sort()[candidates.length - 1];
}

function enrichFloorSummaries(floors: RuntimeOverviewFloor[], teams: RuntimeOverviewTeam[]): RuntimeOverviewFloor[] {
  return floors.map((floor) => {
    const floorTeams = teams.filter((team) => floor.teamIds.includes(team.teamId));
    return {
      ...floor,
      summary: {
        teamCount: floorTeams.length,
        errorCount: floorTeams.reduce((sum, team) => sum + team.errorCount, 0),
        capacityUsed: floorTeams.reduce((sum, team) => sum + team.capacityUsed, 0),
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
