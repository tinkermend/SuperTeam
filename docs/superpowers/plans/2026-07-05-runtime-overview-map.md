# Runtime Overview Map Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first production-oriented `/run-overview` console page: a 3-floor runtime overview map with fixed 10-seat team workspaces, data-rendered employees/statuses, polling-first refresh from existing Control Plane read APIs, and backend guards that prevent teams from exceeding 10 digital employees.

**Architecture:** The first pass uses existing Control Plane read models rather than adding a new realtime channel: Web queries `digital-employees/overview` and `teams`, adapts them into a local `RuntimeOverviewDTO`, then renders the map with a shared logical coordinate system. The visual stack is `2.5D background layer -> SVG path layer -> HTML absolute team/seat/avatar layer -> HTML side panel`; SSE/WebSocket and Konva/Canvas are not first-pass dependencies. Backend capacity enforcement is added at the service layer for both team creation and digital employee creation.

**Tech Stack:** React 19, Vite, TanStack Router, TanStack Query, Tailwind CSS, existing `@/components/superteam` v3 components, SVG, HTML absolute positioning, Go Control Plane services, Vitest browser tests, Go tests.

---

## Global Constraints

- Read `DESIGN.md` before touching Web JSX. Use existing v3 components from `@/components/superteam`; do not introduce a parallel card, pill, or button style system.
- Keep `/run-overview` on the current Web stack: React + Vite + TanStack Router. Do not introduce Next.js.
- First-pass refresh is polling-first via React Query `refetchInterval: 10000`. Do not add SSE or WebSocket for this page.
- Each team has exactly 10 logical seats in the map and a hard service limit of 10 digital employees.
- Do not use the old `reference.png` screenshot as a production layer. It is only a visual QA reference.
- Stage only the files touched by each task. The checkout may contain unrelated dirty files.
- Web tests must run through `corepack pnpm --filter ./apps/web run test`, not direct `npx vitest`.
- Visible UI completion requires Chrome/browser verification against the actual app route. If the app is wired to real APIs, verify Web talks to current Control Plane; mock-only evidence is not real-chain proof.

## Known Boundaries (First Pass)

These are accepted limitations of this pass. Do not silently "fix" them mid-implementation; if one becomes blocking, surface it to the human owner.

- **Capacity guard is service-level, not a DB constraint.** It is a read-then-check before write: concurrent creations can still race past 10, and assignment paths outside `CreateTeam` / `CreateDigitalEmployee` (for example the in-progress team-lending flows, or direct repository-level assignment) are not guarded. State this in the PR description.
- **Overview query uses `limit=100`.** Tenants with more than 100 digital employees get truncated map data: seat assignment, `overCapacity`, and `capacityUsed` are computed from fetched items only (`employeeCount: items.length || team.digital_employee_count` falls back to the team count only when a team has zero fetched items). Acceptable for current dev-scale data.
- **Floor slots support at most 13 teams (6 + 4 + 3).** Teams beyond the 13th are appended to floor-3's `teamIds` but get no workspace, so their employees do not appear on the map. The table view remains complete.
- **Seat assignment is stable by API item order.** The adapter must not sort employees by status when assigning seats, otherwise avatars jump between desks on every poll; status is shown on the avatar badge only.

## File Structure

### Web Feature

- Create `apps/web/src/features/run-overview/runtime-overview-model.ts`
  - Defines the local DTO consumed by map components and the fixed 10-seat capacity constant.
- Create `apps/web/src/features/run-overview/runtime-overview-adapter.ts`
  - Converts existing `DigitalEmployeeOverview` and `TeamListItem[]` API payloads into `RuntimeOverviewDTO`.
- Create `apps/web/src/features/run-overview/runtime-overview-layout.ts`
  - Contains deterministic floor/team/seat coordinates for up to 3 floors.
- Create `apps/web/src/features/run-overview/runtime-overview-fixtures.ts`
  - Test-only/demo fixture used by component tests.
- Create `apps/web/src/features/run-overview/runtime-overview-adapter.test.ts`
  - Unit tests for status counts, 10-seat assignment, floor placement, and over-capacity handling.
- Create `apps/web/src/features/run-overview/index.tsx`
  - Route page and `RunOverviewView` query shell.
- Create `apps/web/src/features/run-overview/index.test.tsx`
  - Browser component tests for page fetches, map rendering, floor switch, selection, and polling config.
- Create `apps/web/src/features/run-overview/components/runtime-map-stage.tsx`
  - Shared logical-coordinate viewport, zoom controls, background/SVG/HTML layers.
- Create `apps/web/src/features/run-overview/components/runtime-map-svg-layer.tsx`
  - SVG paths, team boundaries, and selected-team outline.
- Create `apps/web/src/features/run-overview/components/team-workspace-renderer.tsx`
  - Team card, 10 seats, empty seat markers, and `+N 空闲`.
- Create `apps/web/src/features/run-overview/components/employee-avatar-node.tsx`
  - Avatar/status/selected rendering for one employee.
- Create `apps/web/src/features/run-overview/components/runtime-overview-side-panel.tsx`
  - Right summary and selected employee details.
- Create `apps/web/src/features/run-overview/components/runtime-overview-table.tsx`
  - Table view fallback.
- Modify `apps/web/src/routes/_authenticated/run-overview/index.tsx`
  - Replace `UnimplementedPage` with `RunOverviewPage`.

### Backend Capacity Guard

- Modify `apps/control-plane/internal/tenant/types.go`
  - Add shared `MaxDigitalEmployeesPerTeam = 10` constant in the tenant package.
- Modify `apps/control-plane/internal/tenant/service.go`
  - Reject team creation requests whose `InitialDigitalEmployeeIDs` length is greater than 10.
- Modify `apps/control-plane/internal/tenant/service_test.go`
  - Add a service test for the initial team capacity guard.
- Modify `apps/control-plane/internal/employee/service.go`
  - Reject `CreateDigitalEmployee` when `TeamID` is set and the target team already has 10 digital employees.
- Modify `apps/control-plane/internal/employee/service_test.go`
  - Add a service test for the existing-team capacity guard and extend `memoryRepository` with an overview count hook.

---

### Task 1: Web Runtime Overview Model And Adapter

**Files:**
- Create: `apps/web/src/features/run-overview/runtime-overview-model.ts`
- Create: `apps/web/src/features/run-overview/runtime-overview-layout.ts`
- Create: `apps/web/src/features/run-overview/runtime-overview-adapter.ts`
- Create: `apps/web/src/features/run-overview/runtime-overview-adapter.test.ts`

- [ ] **Step 1: Write the adapter tests first**

Create `apps/web/src/features/run-overview/runtime-overview-adapter.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import type { DigitalEmployeeOverview } from "@/lib/api/employees";
import type { TeamListItem } from "@/lib/api/teams";
import { TEAM_SEAT_CAPACITY, buildRuntimeOverview } from "./runtime-overview-adapter";

const teams: TeamListItem[] = [
  {
    id: "team-dev",
    tenant_id: "tenant-1",
    slug: "dev",
    name: "开发团队",
    status: "active",
    member_count: 2,
    digital_employee_count: 3,
    capability_count: 0,
    governance_status: "active",
    pending_draft_count: 0,
    risk_summary: "normal",
  },
  {
    id: "team-ops",
    tenant_id: "tenant-1",
    slug: "ops",
    name: "运维团队",
    status: "active",
    member_count: 2,
    digital_employee_count: 11,
    capability_count: 0,
    governance_status: "active",
    pending_draft_count: 0,
    risk_summary: "over capacity",
  },
];

const employees: DigitalEmployeeOverview = {
  summary: {
    total_count: 5,
    runnable_count: 3,
    running_count: 2,
    waiting_runtime_count: 0,
    error_count: 1,
    high_risk_count: 1,
    ready_count: 4,
    pending_runtime_binding_count: 0,
    pending_config_approval_count: 0,
    failed_recent_run_count: 1,
    operational_status_counts: {
      working: 2,
      idle: 1,
      waiting_human: 1,
      error: 1,
    },
  },
  queue_summary: {
    pending_runtime_binding_count: 0,
    stale_config_count: 0,
    failed_recent_run_count: 1,
  },
  filters: { teams: [], employee_types: [], statuses: [], providers: [], runtime_nodes: [], risk_levels: [], execution_statuses: [], run_statuses: [] },
  pagination: { limit: 100, offset: 0, total_count: 5 },
  items: [
    employee("emp-1", "高秀英", "运维工程师 AI", "team-ops", "运维团队", "working", "排查线上告警并生成修复计划"),
    employee("emp-2", "陆一鸣", "前端工程师 AI", "team-dev", "开发团队", "working", "实现运行态组件"),
    employee("emp-3", "沈嘉", "后端工程师 AI", "team-dev", "开发团队", "idle"),
    employee("emp-4", "赵宁", "测试工程师 AI", "team-dev", "开发团队", "waiting_human", "等待人工确认"),
    employee("emp-5", "季敏", "告警分析 AI", "team-ops", "运维团队", "error", "定位异常实例"),
  ],
};

function employee(
  id: string,
  name: string,
  role: string,
  teamId: string,
  teamName: string,
  status: DigitalEmployeeOverview["items"][number]["operational_state"]["status"],
  title = "",
): DigitalEmployeeOverview["items"][number] {
  return {
    identity_summary: {
      id,
      tenant_id: "tenant-1",
      team_id: teamId,
      team_name: teamName,
      owner_user_id: "owner-1",
      owner_display_name: "Owner",
      employee_type: "engineer",
      employee_type_label: "工程师",
      name,
      role,
      status: "ready",
      risk_level: status === "error" ? "high" : "medium",
      avatar_asset: {
        id: `${id}-avatar`,
        label: name,
        gender: "unknown",
        age_range: "adult",
        style: "2.5d",
        image_url: `https://example.com/${id}.png`,
        thumbnail_url: `https://example.com/${id}-thumb.png`,
        source: "fixture",
        license: "internal",
        status: "active",
      },
    },
    execution_summary: {
      status: "ready",
      node_id: "local-dev-node",
      runtime_name: "local-dev-node",
      runtime_status: "online",
      provider_type: "codex",
      provider_status: "healthy",
      health_status: "healthy",
      agent_home_dir_available: true,
    },
    workbench_status: "ready",
    operational_state: { status, reasons: [], can_dispatch: status !== "error" },
    recent_events: [{ label: title || "暂无任务", status, occurred_at: "2026-07-05T10:00:00Z" }],
    latest_run_summary: title ? {
      run_id: `${id}-run`,
      task_id: `${id}-task`,
      status: status === "working" ? "running" : "none",
      title,
      error_message: "",
      token_usage: 128,
    } : null,
    governance_summary: {
      status: "active",
      skills_count: 0,
      mcp_servers_count: 0,
      constitution_ref: "team",
    },
    budget_summary: {
      run_count_30d: 1,
      currency: "CNY",
      source: "runtime",
      usage_tokens_today: 128,
      limit_exceeded: false,
    },
  };
}

describe("buildRuntimeOverview", () => {
  it("maps existing Control Plane read models into fixed-capacity floor workspaces", () => {
    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees,
      generatedAt: "2026-07-05T10:00:00Z",
      teams,
    });

    expect(overview.summary.employeeCount).toBe(5);
    expect(overview.summary.capacityTotal).toBe(teams.length * TEAM_SEAT_CAPACITY);
    expect(overview.summary.workingCount).toBe(2);
    expect(overview.floors).toHaveLength(3);
    expect(overview.teams.find((team) => team.teamId === "team-ops")?.overCapacity).toBe(true);
    expect(overview.floors[0].layout.teamWorkspaces[0].seats).toHaveLength(TEAM_SEAT_CAPACITY);
    expect(overview.employees.find((item) => item.employeeId === "emp-1")?.seatId).toBe("team-ops-seat-1");
  });

  it("does not assign a map seat beyond the tenth employee in a team", () => {
    const elevenOps = Array.from({ length: 11 }, (_, index) =>
      employee(`ops-${index + 1}`, `运维 ${index + 1}`, "运维工程师 AI", "team-ops", "运维团队", "idle"),
    );
    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: { ...employees, items: elevenOps, pagination: { limit: 100, offset: 0, total_count: 11 } },
      generatedAt: "2026-07-05T10:00:00Z",
      teams,
    });

    expect(overview.employees.filter((item) => item.teamId === "team-ops" && item.seatId).length).toBe(10);
    expect(overview.employees.find((item) => item.employeeId === "ops-11")?.seatId).toBeUndefined();
    expect(overview.teams.find((team) => team.teamId === "team-ops")?.overCapacity).toBe(true);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `corepack pnpm --filter ./apps/web run test -- runtime-overview-adapter`

Expected: FAIL because `runtime-overview-adapter.ts` does not exist.

- [ ] **Step 3: Add the model types**

Create `apps/web/src/features/run-overview/runtime-overview-model.ts`:

```ts
import type { DigitalEmployeeOperationalStatus } from "@/lib/api/employees";

export const TEAM_SEAT_CAPACITY = 10 as const;

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
  polygon: Array<{ x: number; y: number }>;
  cardAnchor: { x: number; y: number };
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
  capacity: typeof TEAM_SEAT_CAPACITY;
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
```

- [ ] **Step 4: Add deterministic layout config**

Create `apps/web/src/features/run-overview/runtime-overview-layout.ts`:

```ts
import { TEAM_SEAT_CAPACITY, type RuntimeOverviewFloor, type RuntimeOverviewFloorId, type RuntimeOverviewTeamWorkspace } from "./runtime-overview-model";

export const RUNTIME_OVERVIEW_CANVAS = {
  width: 1200,
  height: 760,
};

const floorTeamSlots: Record<RuntimeOverviewFloorId, Array<Omit<RuntimeOverviewTeamWorkspace, "teamId" | "seats">>> = {
  "floor-1": [
    workspace([70, 115, 315, 115, 370, 255, 110, 285], 115, 70, "standard"),
    workspace([440, 105, 625, 105, 690, 230, 465, 260], 455, 75, "lab"),
    workspace([760, 110, 1060, 110, 1125, 285, 800, 320], 785, 70, "ops"),
    workspace([85, 405, 320, 380, 380, 570, 125, 600], 110, 360, "review"),
    workspace([450, 405, 655, 385, 715, 555, 485, 590], 465, 355, "standard"),
    workspace([760, 420, 1015, 395, 1085, 590, 810, 635], 785, 370, "data"),
  ],
  "floor-2": [
    workspace([115, 135, 390, 95, 455, 260, 155, 310], 145, 75, "lab"),
    workspace([530, 120, 830, 120, 895, 290, 565, 325], 555, 80, "standard"),
    workspace([205, 425, 470, 395, 530, 590, 245, 620], 235, 360, "ops"),
    workspace([690, 420, 980, 390, 1045, 585, 735, 625], 715, 360, "review"),
  ],
  "floor-3": [
    workspace([105, 150, 350, 115, 410, 285, 145, 325], 135, 90, "data"),
    workspace([475, 115, 760, 105, 825, 285, 515, 325], 500, 80, "standard"),
    workspace([800, 420, 1080, 390, 1135, 585, 835, 625], 825, 360, "lab"),
  ],
};

function workspace(
  polygonValues: number[],
  cardX: number,
  cardY: number,
  decorationVariant: RuntimeOverviewTeamWorkspace["decorationVariant"],
) {
  return {
    polygon: toPoints(polygonValues),
    cardAnchor: { x: cardX, y: cardY },
    decorationVariant,
  };
}

function toPoints(values: number[]) {
  const points: Array<{ x: number; y: number }> = [];
  for (let index = 0; index < values.length; index += 2) {
    points.push({ x: values[index], y: values[index + 1] });
  }
  return points;
}

function buildSeats(teamId: string, slotIndex: number): RuntimeOverviewTeamWorkspace["seats"] {
  const baseX = [150, 515, 845, 160, 520, 850][slotIndex] ?? 150;
  const baseY = [205, 195, 210, 500, 500, 520][slotIndex] ?? 205;
  return Array.from({ length: TEAM_SEAT_CAPACITY }, (_, index) => {
    const column = index % 5;
    const row = Math.floor(index / 5);
    return {
      seatId: `${teamId}-seat-${index + 1}`,
      x: baseX + column * 46,
      y: baseY + row * 54,
      rotation: row === 0 ? -8 : 8,
    };
  });
}

export function buildFloorLayouts(teamIdsByFloor: Record<RuntimeOverviewFloorId, string[]>): RuntimeOverviewFloor[] {
  return (Object.keys(floorTeamSlots) as RuntimeOverviewFloorId[]).map((floorId, floorIndex) => {
    const teamIds = teamIdsByFloor[floorId] ?? [];
    const slots = floorTeamSlots[floorId];
    const workspaces = teamIds.slice(0, slots.length).map((teamId, index) => ({
      ...slots[index],
      teamId,
      seats: buildSeats(teamId, index),
    }));
    return {
      floorId,
      label: `${floorIndex + 1}层`,
      teamIds,
      summary: {
        teamCount: teamIds.length,
        errorCount: 0,
        capacityUsed: 0,
        capacityTotal: teamIds.length * TEAM_SEAT_CAPACITY,
      },
      layout: {
        canvasWidth: RUNTIME_OVERVIEW_CANVAS.width,
        canvasHeight: RUNTIME_OVERVIEW_CANVAS.height,
        paths: [
          { id: `${floorId}-main-path`, tone: "primary", points: [{ x: 240, y: 345 }, { x: 460, y: 400 }, { x: 705, y: 395 }, { x: 905, y: 335 }] },
        ],
        teamWorkspaces: workspaces,
      },
    };
  });
}
```

- [ ] **Step 5: Add the adapter implementation**

Create `apps/web/src/features/run-overview/runtime-overview-adapter.ts`:

```ts
import type { DigitalEmployeeOverview, DigitalEmployeeOverviewItem, DigitalEmployeeOperationalStatus } from "@/lib/api/employees";
import type { TeamListItem } from "@/lib/api/teams";
import {
  TEAM_SEAT_CAPACITY,
  type RuntimeOverviewDTO,
  type RuntimeOverviewEmployee,
  type RuntimeOverviewFloor,
  type RuntimeOverviewFloorId,
  type RuntimeOverviewTeam,
} from "./runtime-overview-model";
import { buildFloorLayouts } from "./runtime-overview-layout";

export { TEAM_SEAT_CAPACITY } from "./runtime-overview-model";

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
  const workspaceByTeamId = new Map(
    floors.flatMap((floor) =>
      floor.layout.teamWorkspaces.map((workspace) => [workspace.teamId, { floorId: floor.floorId, workspace }] as const),
    ),
  );
  const employeesByTeam = groupEmployeesByTeam(input.employees.items);
  const overviewTeams = activeTeams.map((team): RuntimeOverviewTeam => {
    const items = employeesByTeam.get(team.id) ?? [];
    return {
      teamId: team.id,
      floorId: workspaceByTeamId.get(team.id)?.floorId ?? "floor-1",
      name: team.name,
      capacity: TEAM_SEAT_CAPACITY,
      employeeCount: items.length || team.digital_employee_count,
      workingCount: countStatus(items, "working"),
      idleCount: countStatus(items, "idle"),
      waitingHumanCount: countStatus(items, "waiting_human"),
      queuedCount: countStatus(items, "queued"),
      errorCount: countStatus(items, "error"),
      overCapacity: (items.length || team.digital_employee_count) > TEAM_SEAT_CAPACITY,
    };
  });
  const overviewEmployees = input.employees.items.map((item): RuntimeOverviewEmployee => {
    const teamId = item.identity_summary.team_id ?? "unassigned";
    const workspace = workspaceByTeamId.get(teamId);
    const teamItems = employeesByTeam.get(teamId) ?? [];
    const seatIndex = teamItems.findIndex((employee) => employee.identity_summary.id === item.identity_summary.id);
    const seat = seatIndex >= 0 && seatIndex < TEAM_SEAT_CAPACITY ? workspace?.workspace.seats[seatIndex] : undefined;
    return employeePresenceFromItem(item, workspace?.floorId ?? "floor-1", seat?.seatId);
  });
  const floorsWithSummary = enrichFloorSummaries(floors, overviewTeams);
  return {
    activeFloorId: input.activeFloorId,
    employees: overviewEmployees,
    floors: floorsWithSummary,
    generatedAt: input.generatedAt,
    selectedEmployeeId: overviewEmployees[0]?.employeeId,
    summary: {
      teamCount: activeTeams.length,
      employeeCount: input.employees.pagination.total_count,
      capacityUsed: overviewEmployees.length,
      capacityTotal: activeTeams.length * TEAM_SEAT_CAPACITY,
      workingCount: input.employees.summary.operational_status_counts.working ?? countEmployees(overviewEmployees, "working"),
      idleCount: input.employees.summary.operational_status_counts.idle ?? countEmployees(overviewEmployees, "idle"),
      waitingHumanCount: input.employees.summary.operational_status_counts.waiting_human ?? countEmployees(overviewEmployees, "waiting_human"),
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
    const floorId: RuntimeOverviewFloorId = index < 6 ? "floor-1" : index < 10 ? "floor-2" : "floor-3";
    floors[floorId].push(team.id);
  });
  return floors;
}

// Seat assignment must stay stable across polls: keep API item order and never
// sort by status, otherwise avatars jump between desks whenever a status changes.
// Status is reflected on the avatar badge only.
function groupEmployeesByTeam(items: DigitalEmployeeOverviewItem[]) {
  const grouped = new Map<string, DigitalEmployeeOverviewItem[]>();
  for (const item of items) {
    const teamId = item.identity_summary.team_id ?? "unassigned";
    const current = grouped.get(teamId) ?? [];
    current.push(item);
    grouped.set(teamId, current);
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
    avatarAsset: item.identity_summary.avatar_asset ? {
      id: item.identity_summary.avatar_asset.id,
      url: item.identity_summary.avatar_asset.thumbnail_url || item.identity_summary.avatar_asset.image_url,
      fallbackLabel: item.identity_summary.name.slice(0, 1),
    } : undefined,
    status: item.operational_state.status,
    currentTask: latestRun ? {
      taskId: latestRun.task_id,
      title: latestRun.title,
      priority: item.identity_summary.risk_level === "high" ? "high" : "medium",
    } : undefined,
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
        capacityTotal: floorTeams.length * TEAM_SEAT_CAPACITY,
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
```

- [ ] **Step 6: Run the adapter test**

Run: `corepack pnpm --filter ./apps/web run test -- runtime-overview-adapter`

Expected: PASS.

- [ ] **Step 7: Commit Task 1**

```bash
git add apps/web/src/features/run-overview/runtime-overview-model.ts apps/web/src/features/run-overview/runtime-overview-layout.ts apps/web/src/features/run-overview/runtime-overview-adapter.ts apps/web/src/features/run-overview/runtime-overview-adapter.test.ts
git commit -m "feat(web): add runtime overview map model"
```

---

### Task 2: Run Overview Query Shell And Route

**Files:**
- Create: `apps/web/src/features/run-overview/index.tsx`
- Create: `apps/web/src/features/run-overview/runtime-overview-fixtures.ts`
- Create: `apps/web/src/features/run-overview/index.test.tsx`
- Modify: `apps/web/src/routes/_authenticated/run-overview/index.tsx`

- [ ] **Step 1: Write the page test first**

Create `apps/web/src/features/run-overview/index.test.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { RunOverviewView } from "@/features/run-overview";
import { digitalEmployeeOverviewFixture, teamListFixture } from "./runtime-overview-fixtures";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status: 200,
  });
}

function createFetcher() {
  const requests: Array<{ pathname: string; search: string }> = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    requests.push({ pathname: url.pathname, search: url.search });
    if (url.pathname === "/api/v1/digital-employees/overview") {
      return jsonResponse(digitalEmployeeOverviewFixture);
    }
    if (url.pathname === "/api/v1/teams") {
      return jsonResponse(teamListFixture);
    }
    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), { status: 404 });
  }) as unknown as typeof fetch;
  return { fetcher, requests };
}

function queryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
}

async function renderPage(fetcher: typeof fetch) {
  return await render(
    <QueryClientProvider client={queryClient()}>
      <RunOverviewView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("RunOverviewView", () => {
  it("renders the runtime overview map from existing Control Plane read APIs", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("heading", { name: "运行总览" })).toBeVisible();
    // Task 3/4 会在地图卡片、工位 sr-only 文本等多处渲染团队名与容量文案，
    // 统一用 .first() 让这些断言在后续任务阶段保持稳定。
    await expect.element(screen.getByText("开发团队").first()).toBeVisible();
    await expect.element(screen.getByText("运维团队").first()).toBeVisible();
    await expect.element(screen.getByText("高秀英").first()).toBeVisible();
    await expect.element(screen.getByText("容量 2/10").first()).toBeVisible();
    expect(requests.some((request) => request.pathname === "/api/v1/digital-employees/overview")).toBe(true);
    expect(requests.some((request) => request.pathname === "/api/v1/teams")).toBe(true);
  });

  it("switches floors without refetching layout data", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("button", { name: "1层" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "2层" }));
    await expect.element(screen.getByText("当前楼层：2层")).toBeVisible();
    expect(requests.filter((request) => request.pathname === "/api/v1/digital-employees/overview").length).toBe(1);
  });
});
```

- [ ] **Step 2: Add the fixture**

Create `apps/web/src/features/run-overview/runtime-overview-fixtures.ts`:

```ts
import type { DigitalEmployeeOverview } from "@/lib/api/employees";
import type { TeamListItem } from "@/lib/api/teams";

export const teamListFixture: TeamListItem[] = [
  {
    id: "team-dev",
    tenant_id: "tenant-1",
    slug: "dev",
    name: "开发团队",
    status: "active",
    member_count: 2,
    digital_employee_count: 3,
    capability_count: 3,
    governance_status: "active",
    pending_draft_count: 0,
    risk_summary: "normal",
  },
  {
    id: "team-ops",
    tenant_id: "tenant-1",
    slug: "ops",
    name: "运维团队",
    status: "active",
    member_count: 2,
    digital_employee_count: 2,
    capability_count: 4,
    governance_status: "active",
    pending_draft_count: 0,
    risk_summary: "normal",
  },
];

// 两个团队的员工数刻意不同（dev=3、ops=2），保证“容量 x/10”之类的文案
// 在页面上不会出现完全相同的重复文本，避免 locator 严格模式多匹配。
export const digitalEmployeeOverviewFixture: DigitalEmployeeOverview = {
  summary: {
    total_count: 5,
    runnable_count: 5,
    running_count: 2,
    waiting_runtime_count: 0,
    error_count: 0,
    high_risk_count: 1,
    ready_count: 5,
    pending_runtime_binding_count: 0,
    pending_config_approval_count: 0,
    failed_recent_run_count: 0,
    operational_status_counts: {
      working: 2,
      idle: 2,
      waiting_human: 1,
    },
  },
  queue_summary: {
    pending_runtime_binding_count: 0,
    stale_config_count: 0,
    failed_recent_run_count: 0,
  },
  filters: {
    teams: [],
    employee_types: [],
    statuses: [],
    providers: [],
    runtime_nodes: [],
    risk_levels: [],
    execution_statuses: [],
    run_statuses: [],
  },
  pagination: { limit: 100, offset: 0, total_count: 5 },
  items: [
    employee("emp-ops-1", "高秀英", "运维工程师 AI", "team-ops", "运维团队", "working", "排查线上告警并生成修复计划"),
    employee("emp-ops-2", "罗明", "发布工程师 AI", "team-ops", "运维团队", "waiting_human", "等待发布窗口确认"),
    employee("emp-dev-1", "陆一鸣", "前端工程师 AI", "team-dev", "开发团队", "working", "实现运行态组件"),
    employee("emp-dev-2", "沈嘉", "后端工程师 AI", "team-dev", "开发团队", "idle"),
    employee("emp-dev-3", "许静", "数据工程师 AI", "team-dev", "开发团队", "idle"),
  ],
};

function employee(
  id: string,
  name: string,
  role: string,
  teamId: string,
  teamName: string,
  status: DigitalEmployeeOverview["items"][number]["operational_state"]["status"],
  title = "",
): DigitalEmployeeOverview["items"][number] {
  return {
    identity_summary: {
      id,
      tenant_id: "tenant-1",
      team_id: teamId,
      team_name: teamName,
      owner_user_id: "owner-1",
      owner_display_name: "Owner",
      employee_type: "engineer",
      employee_type_label: "工程师",
      name,
      role,
      status: "ready",
      risk_level: status === "waiting_human" ? "high" : "medium",
      avatar_asset: {
        id: `${id}-avatar`,
        label: name,
        gender: "unknown",
        age_range: "adult",
        style: "2.5d",
        image_url: `https://example.com/${id}.png`,
        thumbnail_url: `https://example.com/${id}-thumb.png`,
        source: "fixture",
        license: "internal",
        status: "active",
      },
    },
    execution_summary: {
      execution_instance_id: `${id}-instance`,
      status: "ready",
      runtime_node_id: "local-dev-node",
      node_id: "local-dev-node",
      runtime_name: "local-dev-node",
      runtime_status: "online",
      provider_type: "codex",
      provider_status: "healthy",
      health_status: "healthy",
      agent_home_dir_available: true,
    },
    workbench_status: "ready",
    operational_state: { status, reasons: [], can_dispatch: status !== "waiting_human" },
    // 事件标签刻意与任务标题不同：侧栏会同时渲染“当前任务”和事件列表，
    // 两处出现相同文本会触发 locator 严格模式多匹配。
    recent_events: [{ label: title ? "已领取任务" : "暂无任务", status, occurred_at: "2026-07-05T10:00:00Z" }],
    latest_run_summary: title ? {
      run_id: `${id}-run`,
      task_id: `${id}-task`,
      status: status === "working" ? "running" : "none",
      title,
      error_message: "",
      token_usage: 128,
    } : null,
    governance_summary: {
      status: "active",
      skills_count: 1,
      mcp_servers_count: 1,
      constitution_ref: "team",
    },
    budget_summary: {
      run_count_30d: 1,
      currency: "CNY",
      source: "runtime",
      usage_tokens_today: 128,
      daily_token_limit: 10000,
      limit_exceeded: false,
    },
  };
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `corepack pnpm --filter ./apps/web run test -- run-overview`

Expected: FAIL because `RunOverviewView` does not exist.

- [ ] **Step 4: Add the query shell**

Create `apps/web/src/features/run-overview/index.tsx`:

```tsx
import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity, RefreshCw } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { V3Button, V3ErrorState, V3LoadingState } from "@/components/superteam";
import { getDigitalEmployeeOverview } from "@/lib/api/employees";
import { listTeamSummaries } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { buildRuntimeOverview } from "./runtime-overview-adapter";
import type { RuntimeOverviewFloorId } from "./runtime-overview-model";

export function RunOverviewPage() {
  return <RunOverviewView apiBaseUrl={resolveControlPlaneUrl()} />;
}

type RunOverviewViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function RunOverviewView({ apiBaseUrl, fetcher }: RunOverviewViewProps) {
  const [activeFloorId, setActiveFloorId] = useState<RuntimeOverviewFloorId>("floor-1");
  const employees = useQuery({
    queryKey: ["run-overview", "digital-employees"],
    queryFn: () => getDigitalEmployeeOverview({ baseUrl: apiBaseUrl, fetcher }, { limit: 100 }),
    refetchInterval: 10_000,
  });
  const teams = useQuery({
    queryKey: ["run-overview", "teams"],
    queryFn: () => listTeamSummaries({ baseUrl: apiBaseUrl, fetcher }, { limit: 100, status: "active" }),
    refetchInterval: 10_000,
  });
  const overview = useMemo(() => {
    if (!employees.data || !teams.data) return undefined;
    return buildRuntimeOverview({
      activeFloorId,
      employees: employees.data,
      generatedAt: new Date().toISOString(),
      teams: teams.data,
    });
  }, [activeFloorId, employees.data, teams.data]);

  const isLoading = employees.isPending || teams.isPending;
  const error = employees.error ?? teams.error;

  return (
    <Main className="min-w-0">
      <ShellPageHeader
        icon={<Activity className="size-4" />}
        title="运行总览"
        subtitle="按楼层展示团队运行态、数字员工状态和容量占用。"
        actions={
          <V3Button
            type="button"
            variant="outline"
            onClick={() => {
              void employees.refetch();
              void teams.refetch();
            }}
          >
            <RefreshCw className="size-4" />
            刷新
          </V3Button>
        }
      />
      {isLoading ? <V3LoadingState label="正在加载运行总览" /> : null}
      {error ? <V3ErrorState title="运行总览加载失败" description={error.message} /> : null}
      {overview ? (
        <section aria-label="运行总览地图" className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
          <div>
            <div className="mb-3 flex gap-2">
              {overview.floors.map((floor) => (
                <V3Button
                  key={floor.floorId}
                  type="button"
                  variant={activeFloorId === floor.floorId ? "primary" : "outline"}
                  onClick={() => setActiveFloorId(floor.floorId)}
                >
                  {floor.label}
                </V3Button>
              ))}
            </div>
            <p className="text-sm text-v3-ink-2">当前楼层：{overview.floors.find((floor) => floor.floorId === activeFloorId)?.label}</p>
            <pre className="sr-only" data-testid="runtime-overview-json">{JSON.stringify(overview)}</pre>
          </div>
          <aside className="rounded-[22px] border border-v3-line bg-white p-4 shadow-v3-card">
            <div className="text-sm text-v3-ink-2">团队</div>
            <div className="text-2xl font-semibold text-v3-ink">{overview.summary.teamCount}</div>
            <div className="mt-4 text-sm text-v3-ink-2">容量使用</div>
            <div className="text-2xl font-semibold text-v3-ink">{overview.summary.capacityUsed} / {overview.summary.capacityTotal}</div>
            {overview.teams.map((team) => (
              <div key={team.teamId} className="mt-3 rounded-xl border border-v3-line p-3">
                <div className="font-semibold text-v3-ink">{team.name}</div>
                <div className="text-sm text-v3-ink-2">容量 {team.employeeCount}/10</div>
              </div>
            ))}
            {overview.employees.map((employee) => (
              <div key={employee.employeeId} className="mt-2 text-sm text-v3-ink">
                {employee.name}
              </div>
            ))}
          </aside>
        </section>
      ) : null}
    </Main>
  );
}
```

This temporary view intentionally renders simple cards first so the query/data contract can pass before the map components are introduced in Task 3.

- [ ] **Step 5: Wire the route**

Replace `apps/web/src/routes/_authenticated/run-overview/index.tsx` with:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { RunOverviewPage } from "@/features/run-overview";

export const Route = createFileRoute("/_authenticated/run-overview/")({
  component: RunOverviewPage,
});
```

- [ ] **Step 6: Run the page test**

Run: `corepack pnpm --filter ./apps/web run test -- run-overview`

Expected: PASS.

- [ ] **Step 7: Commit Task 2**

```bash
git add apps/web/src/features/run-overview/index.tsx apps/web/src/features/run-overview/index.test.tsx apps/web/src/features/run-overview/runtime-overview-fixtures.ts apps/web/src/routes/_authenticated/run-overview/index.tsx
git commit -m "feat(web): wire runtime overview page"
```

---

### Task 3: HTML/SVG Runtime Map Components

**Files:**
- Create: `apps/web/src/features/run-overview/components/runtime-map-stage.tsx`
- Create: `apps/web/src/features/run-overview/components/runtime-map-svg-layer.tsx`
- Create: `apps/web/src/features/run-overview/components/team-workspace-renderer.tsx`
- Create: `apps/web/src/features/run-overview/components/employee-avatar-node.tsx`
- Modify: `apps/web/src/features/run-overview/index.tsx`
- Modify: `apps/web/src/features/run-overview/index.test.tsx`

- [ ] **Step 1: Extend the browser test**

In `apps/web/src/features/run-overview/index.test.tsx`, add:

```tsx
  it("renders fixed ten-seat team workspaces and selectable employee avatars", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    expect(screen.container.querySelectorAll("[data-runtime-seat='team-dev']").length).toBe(10);
    expect(screen.container.querySelectorAll("[data-runtime-seat='team-ops']").length).toBe(10);

    await userEvent.click(screen.getByRole("button", { name: /高秀英/ }));
    await expect.element(screen.getByText("当前选择：高秀英")).toBeVisible();
  });
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `corepack pnpm --filter ./apps/web run test -- run-overview`

Expected: FAIL because the map stage and seat markers do not exist.

- [ ] **Step 3: Add `EmployeeAvatarNode`**

Create `apps/web/src/features/run-overview/components/employee-avatar-node.tsx`:

```tsx
import type { RuntimeOverviewEmployee } from "../runtime-overview-model";

const statusClass: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "bg-red-500",
  idle: "bg-slate-300",
  needs_configuration: "bg-slate-400",
  queued: "bg-blue-500",
  unavailable: "bg-slate-400",
  waiting_human: "bg-orange-500",
  working: "bg-emerald-500",
};

type EmployeeAvatarNodeProps = {
  employee: RuntimeOverviewEmployee;
  selected: boolean;
  x: number;
  y: number;
  onSelect: (employeeId: string) => void;
};

export function EmployeeAvatarNode({ employee, onSelect, selected, x, y }: EmployeeAvatarNodeProps) {
  return (
    <button
      type="button"
      aria-label={`${employee.name}，${employee.roleLabel}`}
      className="absolute z-20 grid size-12 place-items-center rounded-full border border-white bg-white shadow-v3-card transition-transform hover:scale-105 focus:outline-none focus:ring-2 focus:ring-v3-brand"
      style={{ left: x - 24, top: y - 58 }}
      data-employee-id={employee.employeeId}
      onClick={() => onSelect(employee.employeeId)}
    >
      <span className={`absolute -inset-1 rounded-full ${selected ? "bg-v3-brand/25" : "bg-transparent"}`} />
      {employee.avatarAsset?.url ? (
        <img className="relative size-10 rounded-full object-cover" src={employee.avatarAsset.url} alt="" />
      ) : (
        <span className="relative grid size-10 place-items-center rounded-full bg-v3-brand-soft text-sm font-semibold text-v3-brand-deep">
          {employee.avatarAsset?.fallbackLabel ?? employee.name.slice(0, 1)}
        </span>
      )}
      <span className={`absolute bottom-1 right-1 size-3 rounded-full border-2 border-white ${statusClass[employee.status]}`} />
    </button>
  );
}
```

- [ ] **Step 4: Add SVG layer**

Create `apps/web/src/features/run-overview/components/runtime-map-svg-layer.tsx`:

```tsx
import type { RuntimeOverviewFloor } from "../runtime-overview-model";

type RuntimeMapSvgLayerProps = {
  floor: RuntimeOverviewFloor;
  selectedTeamId?: string;
};

export function RuntimeMapSvgLayer({ floor, selectedTeamId }: RuntimeMapSvgLayerProps) {
  return (
    <svg
      aria-hidden="true"
      className="absolute inset-0 z-10 h-full w-full overflow-visible"
      viewBox={`0 0 ${floor.layout.canvasWidth} ${floor.layout.canvasHeight}`}
    >
      {floor.layout.paths.map((path) => (
        <polyline
          key={path.id}
          fill="none"
          points={path.points.map((point) => `${point.x},${point.y}`).join(" ")}
          stroke={path.tone === "primary" ? "#2F5FFF" : "#94a3b8"}
          strokeDasharray="8 8"
          strokeLinecap="round"
          strokeWidth="2"
        />
      ))}
      {floor.layout.teamWorkspaces.map((workspace) => (
        <polygon
          key={workspace.teamId}
          fill={workspace.teamId === selectedTeamId ? "rgba(47,95,255,0.08)" : "rgba(255,255,255,0.58)"}
          points={workspace.polygon.map((point) => `${point.x},${point.y}`).join(" ")}
          stroke={workspace.teamId === selectedTeamId ? "#2F5FFF" : "#cbd5e1"}
          strokeDasharray="6 5"
          strokeWidth={workspace.teamId === selectedTeamId ? 2 : 1}
        />
      ))}
    </svg>
  );
}
```

- [ ] **Step 5: Add team workspace renderer**

Create `apps/web/src/features/run-overview/components/team-workspace-renderer.tsx`:

```tsx
import type { RuntimeOverviewEmployee, RuntimeOverviewTeam, RuntimeOverviewTeamWorkspace } from "../runtime-overview-model";
import { EmployeeAvatarNode } from "./employee-avatar-node";

type TeamWorkspaceRendererProps = {
  employees: RuntimeOverviewEmployee[];
  onSelectEmployee: (employeeId: string) => void;
  selectedEmployeeId?: string;
  selectedTeamId?: string;
  team: RuntimeOverviewTeam;
  workspace: RuntimeOverviewTeamWorkspace;
};

export function TeamWorkspaceRenderer({
  employees,
  onSelectEmployee,
  selectedEmployeeId,
  selectedTeamId,
  team,
  workspace,
}: TeamWorkspaceRendererProps) {
  const employeesBySeat = new Map(employees.map((employee) => [employee.seatId, employee]));
  const idleSeats = workspace.seats.filter((seat) => !employeesBySeat.has(seat.seatId)).length;
  return (
    <div className="absolute inset-0">
      <article
        className={`absolute z-30 w-[220px] rounded-[16px] border bg-white/95 p-4 shadow-v3-card backdrop-blur ${selectedTeamId === team.teamId ? "border-v3-brand" : "border-v3-line"}`}
        style={{ left: workspace.cardAnchor.x, top: workspace.cardAnchor.y }}
      >
        <h3 className="text-base font-semibold text-v3-ink">{team.name} · {team.employeeCount} 人</h3>
        <p className="mt-2 text-xs text-v3-ink-2">
          容量 {team.employeeCount}/10 · 异常 <span className="text-red-500">{team.errorCount}</span> · 工作中 <span className="text-emerald-600">{team.workingCount}</span>
        </p>
        {team.overCapacity ? <p className="mt-1 text-xs font-semibold text-red-600">团队人数超过 10 人上限</p> : null}
      </article>
      {workspace.seats.map((seat) => {
        const employee = employeesBySeat.get(seat.seatId);
        return (
          <div
            key={seat.seatId}
            className="absolute z-10 h-8 w-12 rounded-[6px] border border-slate-200 bg-white shadow-sm"
            style={{ left: seat.x - 24, top: seat.y - 16, transform: `rotate(${seat.rotation ?? 0}deg)` }}
            data-runtime-seat={team.teamId}
          >
            <span className="sr-only">{team.name} 工位</span>
            {employee ? (
              <EmployeeAvatarNode
                employee={employee}
                selected={employee.employeeId === selectedEmployeeId}
                x={seat.x}
                y={seat.y}
                onSelect={onSelectEmployee}
              />
            ) : null}
          </div>
        );
      })}
      {idleSeats > 0 ? (
        <div
          className="absolute z-20 rounded-full border border-v3-line bg-white px-3 py-1 text-xs font-semibold text-v3-ink-2 shadow-sm"
          style={{ left: workspace.cardAnchor.x + 150, top: workspace.cardAnchor.y + 250 }}
        >
          +{idleSeats} 空闲
        </div>
      ) : null}
    </div>
  );
}
```

- [ ] **Step 6: Add map stage**

Create `apps/web/src/features/run-overview/components/runtime-map-stage.tsx`:

```tsx
import { useLayoutEffect, useMemo, useRef, useState } from "react";
import type { RuntimeOverviewDTO, RuntimeOverviewEmployee } from "../runtime-overview-model";
import { RuntimeMapSvgLayer } from "./runtime-map-svg-layer";
import { TeamWorkspaceRenderer } from "./team-workspace-renderer";

type RuntimeMapStageProps = {
  activeFloorId: RuntimeOverviewDTO["activeFloorId"];
  onSelectEmployee: (employeeId: string) => void;
  overview: RuntimeOverviewDTO;
  selectedEmployeeId?: string;
};

export function RuntimeMapStage({ activeFloorId, onSelectEmployee, overview, selectedEmployeeId }: RuntimeMapStageProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  const floor = overview.floors.find((item) => item.floorId === activeFloorId) ?? overview.floors[0];
  const selectedEmployee = overview.employees.find((employee) => employee.employeeId === selectedEmployeeId);
  const employeesByTeam = useMemo(() => {
    const map = new Map<string, RuntimeOverviewEmployee[]>();
    for (const employee of overview.employees.filter((item) => item.floorId === floor.floorId)) {
      map.set(employee.teamId, [...(map.get(employee.teamId) ?? []), employee]);
    }
    return map;
  }, [floor.floorId, overview.employees]);

  useLayoutEffect(() => {
    const node = containerRef.current;
    if (!node) return;
    const updateScale = () => {
      const nextScale = Math.min(1, Math.max(0.58, node.clientWidth / floor.layout.canvasWidth));
      setScale(Number(nextScale.toFixed(3)));
    };
    updateScale();
    const observer = new ResizeObserver(updateScale);
    observer.observe(node);
    return () => observer.disconnect();
  }, [floor.layout.canvasWidth]);

  return (
    <section className="overflow-hidden rounded-[22px] border border-v3-line bg-[#f8fafc] shadow-v3-card" aria-label="运行总览地图画布">
      <div ref={containerRef} className="relative aspect-[1200/760] min-h-[520px] w-full">
        <div className="absolute inset-0 bg-[linear-gradient(135deg,#ffffff_0%,#f4f7fb_55%,#eef3f8_100%)]" />
        {/* SVG 层必须与 HTML 工位层同处一个被缩放的 wrapper 内，共用同一 transform。
            若 SVG 单独随容器 viewBox 缩放而 HTML 层被 clamp，两层在容器宽度
            超出 [696, 1200] 区间时会错位。 */}
        <div
          className="absolute left-1/2 top-1/2 origin-center"
          style={{
            width: floor.layout.canvasWidth,
            height: floor.layout.canvasHeight,
            transform: `translate(-50%, -50%) scale(${scale})`,
          }}
        >
          <RuntimeMapSvgLayer floor={floor} selectedTeamId={selectedEmployee?.teamId} />
          {floor.layout.teamWorkspaces.map((workspace) => {
            const team = overview.teams.find((item) => item.teamId === workspace.teamId);
            if (!team) return null;
            return (
              <TeamWorkspaceRenderer
                key={workspace.teamId}
                employees={employeesByTeam.get(workspace.teamId) ?? []}
                onSelectEmployee={onSelectEmployee}
                selectedEmployeeId={selectedEmployeeId}
                selectedTeamId={selectedEmployee?.teamId}
                team={team}
                workspace={workspace}
              />
            );
          })}
        </div>
      </div>
    </section>
  );
}
```

- [ ] **Step 7: Integrate the map into `RunOverviewView`**

In `apps/web/src/features/run-overview/index.tsx`, import `RuntimeMapStage`, add `selectedEmployeeId` state, and replace the temporary left column with:

```tsx
<RuntimeMapStage
  activeFloorId={activeFloorId}
  overview={overview}
  selectedEmployeeId={selectedEmployeeId ?? overview.selectedEmployeeId}
  onSelectEmployee={setSelectedEmployeeId}
/>
<p className="mt-3 text-sm text-v3-ink-2">
  当前选择：{overview.employees.find((employee) => employee.employeeId === (selectedEmployeeId ?? overview.selectedEmployeeId))?.name ?? "未选择"}
</p>
```

- [ ] **Step 8: Run the page test**

Run: `corepack pnpm --filter ./apps/web run test -- run-overview`

Expected: PASS.

- [ ] **Step 9: Commit Task 3**

```bash
git add apps/web/src/features/run-overview/components apps/web/src/features/run-overview/index.tsx apps/web/src/features/run-overview/index.test.tsx
git commit -m "feat(web): render runtime overview map"
```

---

### Task 4: Side Panel, Table View, And UX States

**Files:**
- Create: `apps/web/src/features/run-overview/components/runtime-overview-side-panel.tsx`
- Create: `apps/web/src/features/run-overview/components/runtime-overview-table.tsx`
- Modify: `apps/web/src/features/run-overview/index.tsx`
- Modify: `apps/web/src/features/run-overview/index.test.tsx`

- [ ] **Step 1: Extend tests for side panel and table view**

Add to `apps/web/src/features/run-overview/index.test.tsx`:

```tsx
  it("shows selected employee details and table view fallback", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await userEvent.click(screen.getByRole("button", { name: /高秀英/ }));
    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();
    await expect.element(screen.getByText("排查线上告警并生成修复计划")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "表格视图" }));
    const table = screen.getByRole("table", { name: "运行总览表格" });
    await expect.element(table).toBeVisible();
    // 侧栏同时显示选中员工的角色文案，作用域限定到表格内避免多匹配。
    await expect.element(table.getByText("运维工程师 AI")).toBeVisible();
  });
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `corepack pnpm --filter ./apps/web run test -- run-overview`

Expected: FAIL because the side panel and table toggle do not exist.

- [ ] **Step 3: Add side panel**

Create `apps/web/src/features/run-overview/components/runtime-overview-side-panel.tsx`:

```tsx
import { SoftCard, StatusPill, V3MetricCard } from "@/components/superteam";
import type { RuntimeOverviewDTO, RuntimeOverviewEmployee } from "../runtime-overview-model";

const statusLabel: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "异常",
  idle: "空闲",
  needs_configuration: "待配置",
  queued: "排队",
  unavailable: "不可用",
  waiting_human: "待确认",
  working: "工作中",
};

export function RuntimeOverviewSidePanel({ overview, selectedEmployeeId }: { overview: RuntimeOverviewDTO; selectedEmployeeId?: string }) {
  const selected = overview.employees.find((employee) => employee.employeeId === selectedEmployeeId) ?? overview.employees[0];
  return (
    <div className="flex min-w-0 flex-col gap-4">
      <SoftCard className="p-4">
        <h2 className="text-sm font-semibold text-v3-ink">运行概况</h2>
        <div className="mt-4 grid grid-cols-2 gap-3">
          <V3MetricCard label="团队" value={overview.summary.teamCount} />
          <V3MetricCard label="数字员工" value={overview.summary.employeeCount} />
          <V3MetricCard label="容量使用" value={`${overview.summary.capacityUsed}/${overview.summary.capacityTotal}`} />
          <V3MetricCard label="异常" value={overview.summary.errorCount} />
        </div>
      </SoftCard>
      {selected ? (
        <SoftCard className="p-4">
          <div className="flex items-start gap-3">
            {selected.avatarAsset?.url ? <img className="size-12 rounded-full" src={selected.avatarAsset.url} alt="" /> : <div className="grid size-12 place-items-center rounded-full bg-v3-brand-soft font-semibold text-v3-brand-deep">{selected.name.slice(0, 1)}</div>}
            <div className="min-w-0">
              <h2 className="truncate text-lg font-semibold text-v3-ink">{selected.name}</h2>
              <p className="text-sm text-v3-ink-2">{selected.roleLabel}</p>
            </div>
            <StatusPill tone={selected.status === "error" ? "danger" : selected.status === "working" ? "ok" : selected.status === "waiting_human" ? "warn" : "mute"}>
              {statusLabel[selected.status]}
            </StatusPill>
          </div>
          <div className="mt-5">
            <div className="text-xs font-semibold text-v3-ink-2">当前任务</div>
            <p className="mt-1 text-sm font-semibold text-v3-ink">{selected.currentTask?.title ?? "暂无进行中的任务"}</p>
          </div>
          <div className="mt-5">
            <div className="text-xs font-semibold text-v3-ink-2">命令 / 日志</div>
            <ul className="mt-2 space-y-2">
              {selected.recentEvents.slice(0, 3).map((event, index) => (
                <li key={`${event.label}-${index}`} className="text-sm text-v3-ink-2">{event.label}</li>
              ))}
            </ul>
          </div>
        </SoftCard>
      ) : null}
    </div>
  );
}
```

- [ ] **Step 4: Add table fallback**

Create `apps/web/src/features/run-overview/components/runtime-overview-table.tsx`:

```tsx
import { StatusPill, V3Table, V3Td, V3Th, V3Tr, WorkSurface } from "@/components/superteam";
import type { RuntimeOverviewDTO, RuntimeOverviewEmployee } from "../runtime-overview-model";

const statusLabel: Record<RuntimeOverviewEmployee["status"], string> = {
  error: "异常",
  idle: "空闲",
  needs_configuration: "待配置",
  queued: "排队",
  unavailable: "不可用",
  waiting_human: "待确认",
  working: "工作中",
};

export function RuntimeOverviewTable({ overview }: { overview: RuntimeOverviewDTO }) {
  return (
    <WorkSurface>
      <V3Table aria-label="运行总览表格">
        <thead>
          <tr>
            <V3Th>数字员工</V3Th>
            <V3Th>角色</V3Th>
            <V3Th>状态</V3Th>
            <V3Th>当前任务</V3Th>
          </tr>
        </thead>
        <tbody>
          {overview.employees.map((employee) => (
            <V3Tr key={employee.employeeId} tone={employee.status === "error" ? "danger" : undefined}>
              <V3Td>{employee.name}</V3Td>
              <V3Td>{employee.roleLabel}</V3Td>
              <V3Td><StatusPill tone={employee.status === "error" ? "danger" : employee.status === "working" ? "ok" : "mute"}>{statusLabel[employee.status]}</StatusPill></V3Td>
              <V3Td>{employee.currentTask?.title ?? "暂无"}</V3Td>
            </V3Tr>
          ))}
        </tbody>
      </V3Table>
    </WorkSurface>
  );
}
```

- [ ] **Step 5: Integrate view toggle and side panel**

In `apps/web/src/features/run-overview/index.tsx`, add `viewMode` state:

```ts
const [viewMode, setViewMode] = useState<"map" | "table">("map");
```

Import the new components:

```ts
import { RuntimeMapStage } from "./components/runtime-map-stage";
import { RuntimeOverviewSidePanel } from "./components/runtime-overview-side-panel";
import { RuntimeOverviewTable } from "./components/runtime-overview-table";
```

Add view buttons near floor controls:

```tsx
<V3Button type="button" variant={viewMode === "map" ? "primary" : "outline"} onClick={() => setViewMode("map")}>地图视图</V3Button>
<V3Button type="button" variant={viewMode === "table" ? "primary" : "outline"} onClick={() => setViewMode("table")}>表格视图</V3Button>
```

Replace the `overview ? (...) : null` content with:

```tsx
<section aria-label="运行总览地图" className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
  <div className="min-w-0">
    <div className="mb-3 flex flex-wrap items-center gap-2">
      {overview.floors.map((floor) => (
        <V3Button
          key={floor.floorId}
          type="button"
          variant={activeFloorId === floor.floorId ? "primary" : "outline"}
          onClick={() => setActiveFloorId(floor.floorId)}
        >
          {floor.label}
        </V3Button>
      ))}
      <span className="mx-1 h-5 w-px bg-v3-line" aria-hidden />
      <V3Button type="button" variant={viewMode === "map" ? "primary" : "outline"} onClick={() => setViewMode("map")}>
        地图视图
      </V3Button>
      <V3Button type="button" variant={viewMode === "table" ? "primary" : "outline"} onClick={() => setViewMode("table")}>
        表格视图
      </V3Button>
    </div>
    <p className="mb-3 text-sm text-v3-ink-2">
      当前楼层：{overview.floors.find((floor) => floor.floorId === activeFloorId)?.label}
    </p>
    {viewMode === "map" ? (
      <>
        <RuntimeMapStage
          activeFloorId={activeFloorId}
          overview={overview}
          selectedEmployeeId={selectedEmployeeId ?? overview.selectedEmployeeId}
          onSelectEmployee={setSelectedEmployeeId}
        />
        <p className="mt-3 text-sm text-v3-ink-2">
          当前选择：{overview.employees.find((employee) => employee.employeeId === (selectedEmployeeId ?? overview.selectedEmployeeId))?.name ?? "未选择"}
        </p>
      </>
    ) : (
      <RuntimeOverviewTable overview={overview} />
    )}
  </div>
  <RuntimeOverviewSidePanel overview={overview} selectedEmployeeId={selectedEmployeeId ?? overview.selectedEmployeeId} />
</section>
```

- [ ] **Step 6: Run page tests**

Run: `corepack pnpm --filter ./apps/web run test -- run-overview`

Expected: PASS.

- [ ] **Step 7: Commit Task 4**

```bash
git add apps/web/src/features/run-overview
git commit -m "feat(web): add runtime overview side panel"
```

---

### Task 5: Enforce Team Capacity On Team Creation

**Files:**
- Modify: `apps/control-plane/internal/tenant/types.go`
- Modify: `apps/control-plane/internal/tenant/service.go`
- Modify: `apps/control-plane/internal/tenant/service_test.go`

- [ ] **Step 1: Write the failing service test**

In `apps/control-plane/internal/tenant/service_test.go`, add:

```go
func TestCreateTeamRejectsMoreThanTenInitialDigitalEmployees(t *testing.T) {
	svc, err := NewServiceWithoutAuditForTest(&memoryRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ownerID := uuid.New()
	employeeIDs := make([]uuid.UUID, MaxDigitalEmployeesPerTeam+1)
	for i := range employeeIDs {
		employeeIDs[i] = uuid.New()
	}
	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:                  uuid.New(),
		ActorUserID:               ownerID,
		Slug:                      "runtime-map",
		Name:                      "Runtime Map",
		HumanOwnerUserIDs:         []uuid.UUID{ownerID},
		InitialDigitalEmployeeIDs: employeeIDs,
	})
	if err == nil || !strings.Contains(err.Error(), "digital employee capacity") {
		t.Fatalf("expected digital employee capacity error, got %v", err)
	}
}
```

If `strings` is not already imported in this test file, add it to the import list.

- [ ] **Step 2: Run the tenant service test to verify it fails**

Run: `go test ./apps/control-plane/internal/tenant -run TestCreateTeamRejectsMoreThanTenInitialDigitalEmployees`

Expected: FAIL because `MaxDigitalEmployeesPerTeam` and the validation do not exist.

- [ ] **Step 3: Add the constant and validation**

In `apps/control-plane/internal/tenant/types.go`, add near team role constants:

```go
const MaxDigitalEmployeesPerTeam = 10
```

In `apps/control-plane/internal/tenant/service.go`, after status validation and before `normalizeInitialMembers`, add:

```go
if len(req.InitialDigitalEmployeeIDs) > MaxDigitalEmployeesPerTeam {
	return nil, fmt.Errorf("%w: digital employee capacity is limited to %d per team", ErrInvalidInput, MaxDigitalEmployeesPerTeam)
}
```

- [ ] **Step 4: Run the tenant tests**

Run: `go test ./apps/control-plane/internal/tenant`

Expected: PASS.

- [ ] **Step 5: Commit Task 5**

```bash
git add apps/control-plane/internal/tenant/types.go apps/control-plane/internal/tenant/service.go apps/control-plane/internal/tenant/service_test.go
git commit -m "feat(control-plane): limit team digital employees"
```

---

### Task 6: Enforce Team Capacity On Digital Employee Creation

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`

- [ ] **Step 1: Write the failing employee service test**

In `apps/control-plane/internal/employee/service_test.go`, add:

```go
func TestCreateDigitalEmployeeRejectsFullTeam(t *testing.T) {
	svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	repo.overviewTotalCountByTeam[*req.TeamID] = maxDigitalEmployeesPerTeam

	_, err := svc.CreateDigitalEmployee(context.Background(), CreateDigitalEmployeeRequest{
		TenantID:      req.TenantID,
		TeamID:        req.TeamID,
		OwnerUserID:   req.OwnerUserID,
		EmployeeType:  req.EmployeeType,
		Name:          "第十一个数字员工",
		AvatarAssetID: req.AvatarAssetID,
		ProviderType:  req.ProviderType,
	})
	if err == nil || !strings.Contains(err.Error(), "digital employee capacity") {
		t.Fatalf("expected digital employee capacity error, got %v", err)
	}
	if repo.createdEmployeeCount != 0 || repo.transactionCount != 0 {
		t.Fatalf("expected full team check before creation, employees=%d transactions=%d", repo.createdEmployeeCount, repo.transactionCount)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected full team check not to dispatch command, got %#v", dispatcher.commands)
	}
}
```

Extend the existing `memoryRepository` test fixture in the same file:

```go
type memoryRepository struct {
	// existing fields...
	// int32 匹配 OverviewPagination.TotalCount 的字段类型，避免赋值时的类型转换。
	overviewTotalCountByTeam map[uuid.UUID]int32
}
```

Initialize it in `newMemoryRepository`:

```go
overviewTotalCountByTeam: make(map[uuid.UUID]int32),
```

Update the existing `GetDigitalEmployeeOverview` method on `memoryRepository`:

```go
func (r *memoryRepository) GetDigitalEmployeeOverview(_ context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error) {
	var totalCount int32
	if req.TeamID != nil {
		totalCount = r.overviewTotalCountByTeam[*req.TeamID]
	}
	return &DigitalEmployeeOverview{
		Summary:    DigitalEmployeeOverviewSummary{},
		Items:      []DigitalEmployeeOverviewItem{},
		Filters:    DigitalEmployeeOverviewFilters{},
		Pagination: OverviewPagination{Limit: req.Limit, Offset: req.Offset, TotalCount: totalCount},
	}, nil
}
```

- [ ] **Step 2: Run the employee test to verify it fails**

Run: `go test ./apps/control-plane/internal/employee -run TestCreateDigitalEmployeeRejectsFullTeam`

Expected: FAIL because no capacity check exists.

- [ ] **Step 3: Add local capacity constant and check**

In `apps/control-plane/internal/employee/service.go`, add `maxDigitalEmployeesPerTeam` to the existing `const (...)` block next to `maxWorkspaceFileInlineBytes`:

```go
const (
	defaultProvisioningTimeout      = 10 * time.Second
	defaultProvisioningPollInterval = 250 * time.Millisecond
	maxWorkspaceFileInlineBytes     = 10 * 1024 * 1024
	maxDigitalEmployeesPerTeam      = 10
)
```

Then in `CreateDigitalEmployee`, immediately after `EnsureTeamExists` succeeds and before loading current team config, add:

```go
if err := s.ensureTeamDigitalEmployeeCapacity(ctx, normalized.TenantID, teamID); err != nil {
	return nil, err
}
```

Add this helper near other validation helpers:

```go
func (s *Service) ensureTeamDigitalEmployeeCapacity(ctx context.Context, tenantID, teamID uuid.UUID) error {
	overview, err := s.repository.GetDigitalEmployeeOverview(ctx, GetDigitalEmployeeOverviewRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
		Limit:    1,
	})
	if err != nil {
		return fmt.Errorf("get team digital employee count: %w", err)
	}
	if overview != nil && overview.Pagination.TotalCount >= maxDigitalEmployeesPerTeam {
		return fmt.Errorf("%w: digital employee capacity is limited to %d per team", ErrInvalidInput, maxDigitalEmployeesPerTeam)
	}
	return nil
}
```

- [ ] **Step 4: Run employee tests**

Run: `go test ./apps/control-plane/internal/employee`

Expected: PASS.

- [ ] **Step 5: Commit Task 6**

```bash
git add apps/control-plane/internal/employee/service.go apps/control-plane/internal/employee/service_test.go
git commit -m "feat(control-plane): guard full team employee creation"
```

---

### Task 7: Final Verification And Browser QA

**Files:**
- No implementation files expected unless QA finds a defect.

- [ ] **Step 1: Run focused local gates**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- run-overview
corepack pnpm --filter ./apps/web run typecheck
go test ./apps/control-plane/internal/tenant ./apps/control-plane/internal/employee
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 2: Start or restart relevant services**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```

Expected: Control Plane and Web are running from current code. If the script reports services it does not manage, inspect before killing anything.

- [ ] **Step 3: Browser verify the route**

Using Chrome plug/browser automation, open the running Web route:

```text
http://127.0.0.1:3000/run-overview
```

Verify:

- The page title is `运行总览`.
- The route is no longer `UnimplementedPage`.
- The map view renders at least one team card and 10 seats per visible team.
- Clicking a visible employee updates the right detail panel.
- `1层 / 2层 / 3层` switches do not produce overlapping text or broken layout.
- `表格视图` shows the same employees in a table.

- [ ] **Step 4: Browser verify responsive alignment**

Use Chrome viewport checks:

- `1536 x 1024`
- `1366 x 768`
- `1920 x 1080`

Expected: SVG paths and HTML seats/avatars remain aligned; no horizontal page overflow; right panel remains usable or stacks according to layout.

- [ ] **Step 5: Real API smoke**

With authenticated browser session or authenticated curl, confirm the route calls:

- `GET /api/v1/digital-employees/overview?limit=100`
- `GET /api/v1/teams?status=active&limit=100`

Expected: both return non-5xx responses from current Control Plane. If auth is unavailable, report that real-chain verification is blocked and do not claim the feature is usable end-to-end.

- [ ] **Step 6: Capacity guard smoke**

Use an API test or curl with real auth to attempt creating a team with 11 `initial_digital_employee_ids`, or creating an 11th digital employee in an already full team.

Expected: Control Plane returns a 4xx response with a digital employee capacity error. If no safe tenant/team fixture exists, document the blocker and rely only on Go tests for this guard.

- [ ] **Step 7: Final commit if QA fixes were needed**

If Step 3-6 required fixes, stage only those files:

```bash
git add <fixed-files>
git commit -m "fix(web): polish runtime overview map verification"
```

If no fixes were needed, do not create an empty commit.
