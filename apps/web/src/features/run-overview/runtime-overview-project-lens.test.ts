import { describe, expect, it } from "vitest";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import type { RuntimeOverviewDTO, RuntimeOverviewEmployee, RuntimeOverviewFloor } from "./runtime-overview-model";
import {
  aggregateLensProjectOptions,
  buildProjectLens,
  lensParticipantFloorIds,
  projectLensForFloor,
} from "./runtime-overview-project-lens";
import { projectTaskGraphFixture } from "./runtime-overview-fixtures";

function graph(partial: Partial<ProjectTaskGraph>): ProjectTaskGraph {
  return {
    nodes: [],
    edges: [],
    employees: [],
    runs: [],
    execution_summaries: [],
    recent_events: [],
    decision_requests: [],
    blocking_facts: [],
    ...partial,
  };
}

function node(
  id: string,
  status: string,
  assignee?: string,
  currentBlocker?: { reason_code: string; message: string },
): ProjectTaskGraph["nodes"][number] {
  return {
    id,
    tenant_id: "tenant-1",
    project_id: "project-1",
    title: id,
    status,
    assigned_digital_employee_id: assignee,
    requires_human_approval: false,
    expected_outputs: [],
    input_requirements: {},
    handoff_contract: {},
    planner_metadata: {},
    current_blocker: currentBlocker as ProjectTaskGraph["nodes"][number]["current_blocker"],
  };
}

function edge(blockerTaskId: string, dependentTaskId: string): ProjectTaskGraph["edges"][number] {
  return { dependent_task_id: dependentTaskId, blocker_task_id: blockerTaskId, edge_status: "pending" };
}

describe("buildProjectLens", () => {
  it("projects handoff edges between assignees and counts unassigned tasks", () => {
    const lens = buildProjectLens("project-lens-1", projectTaskGraphFixture);

    expect(lens.participantEmployeeIds.sort()).toEqual(["emp-dev-1", "emp-dev-2", "emp-ops-1"]);
    expect(lens.edges).toHaveLength(2);
    const active = lens.edges.find((item) => item.id === "emp-dev-1->emp-ops-1");
    expect(active?.tone).toBe("primary");
    const upcoming = lens.edges.find((item) => item.id === "emp-ops-1->emp-dev-2");
    expect(upcoming?.tone).toBe("muted");
    expect(lens.unassignedTaskCount).toBe(1);
    expect(lens.totalTaskCount).toBe(4);
    expect(lens.stopEmployeeIds).toEqual(["emp-ops-1"]);
  });

  it("merges same-endpoint edges with warning tone taking priority", () => {
    const lens = buildProjectLens(
      "project-1",
      graph({
        nodes: [
          node("a1", "completed", "emp-a"),
          node("a2", "completed", "emp-a"),
          node("b1", "running", "emp-b"),
          node("b2", "failed", "emp-b"),
        ],
        edges: [edge("a1", "b1"), edge("a2", "b2")],
      }),
    );

    expect(lens.edges).toHaveLength(1);
    expect(lens.edges[0]).toMatchObject({ id: "emp-a->emp-b", taskCount: 2, tone: "warning" });
    expect(lens.blockedTaskCount).toBe(1);
  });

  it("skips self edges and edges with unassigned endpoints", () => {
    const lens = buildProjectLens(
      "project-1",
      graph({
        nodes: [node("a", "completed", "emp-a"), node("b", "running", "emp-a"), node("c", "pending")],
        edges: [edge("a", "b"), edge("b", "c")],
      }),
    );

    expect(lens.edges).toHaveLength(0);
    expect(lens.unassignedTaskCount).toBe(1);
  });

  it("marks employees with blocked tasks as chain stops", () => {
    const lens = buildProjectLens(
      "project-1",
      graph({
        nodes: [node("a", "pending", "emp-a", { reason_code: "missing_input", message: "缺少输入" })],
      }),
    );

    expect(lens.stopEmployeeIds).toEqual(["emp-a"]);
    expect(lens.blockedTaskCount).toBe(1);
  });
});

// —— 楼层投影 ——

function floor(
  floorId: RuntimeOverviewFloor["floorId"],
  seats: Array<{ seatId: string; x: number; y: number }>,
): RuntimeOverviewFloor {
  return {
    floorId,
    label: floorId,
    teamIds: ["team-1"],
    summary: { teamCount: 1, errorCount: 0, capacityUsed: 0, capacityTotal: seats.length },
    layout: {
      backgroundImageUrl: "/x.png",
      canvasWidth: 1672,
      canvasHeight: 941,
      paths: [],
      teamWorkspaces: [
        {
          teamId: `team-${floorId}`,
          capacity: 3,
          polygon: [],
          cardAnchor: { x: 0, y: 0 },
          calloutTarget: { x: 0, y: 0 },
          seats,
          decorationVariant: "standard",
        },
      ],
    },
  };
}

function overviewEmployee(employeeId: string, floorId: RuntimeOverviewFloor["floorId"], seatId?: string): RuntimeOverviewEmployee {
  return {
    employeeId,
    teamId: `team-${floorId}`,
    floorId,
    seatId,
    name: employeeId,
    roleLabel: "工程师 AI",
    status: "working",
    statusReasons: [],
    recentEvents: [],
    projects: [],
    projectCount: 0,
    artifacts: [],
  };
}

function overviewWith(floors: RuntimeOverviewFloor[], employees: RuntimeOverviewEmployee[]): RuntimeOverviewDTO {
  return {
    generatedAt: "2026-07-18T00:00:00Z",
    activeFloorId: "floor-1",
    summary: {
      teamCount: 1,
      employeeCount: employees.length,
      capacityUsed: 0,
      capacityTotal: 0,
      workingCount: 0,
      idleCount: 0,
      waitingHumanCount: 0,
      queuedCount: 0,
      needsConfigurationCount: 0,
      unavailableCount: 0,
      errorCount: 0,
      cumulativeTaskCount: 0,
      linkedProjectCount: 0,
      todayTokensTotal: 0,
    },
    floors,
    teams: [],
    employees,
    recentActivity: [],
  };
}

describe("projectLensForFloor", () => {
  const floors = [
    floor("floor-1", [
      { seatId: "s1", x: 100, y: 200 },
      { seatId: "s2", x: 300, y: 200 },
    ]),
    floor("floor-2", [{ seatId: "s3", x: 500, y: 400 }]),
  ];
  const employees = [
    overviewEmployee("emp-a", "floor-1", "s1"),
    overviewEmployee("emp-b", "floor-1", "s2"),
    overviewEmployee("emp-c", "floor-2", "s3"),
    overviewEmployee("emp-d", "floor-1", undefined),
  ];
  const overview = overviewWith(floors, employees);
  const lens = buildProjectLens(
    "project-1",
    graph({
      nodes: [
        node("a", "completed", "emp-a"),
        node("b", "running", "emp-b"),
        node("c", "pending", "emp-c"),
        node("d", "pending", "emp-d"),
      ],
      edges: [edge("a", "b"), edge("b", "c"), edge("c", "d")],
    }),
  );

  it("draws same-floor edges with seat coordinates and lifts cross-floor edges into portals", () => {
    const projection = projectLensForFloor(lens, overview, "floor-1");

    expect(projection.fullEdges).toHaveLength(1);
    expect(projection.fullEdges[0]).toMatchObject({
      id: "emp-a->emp-b",
      from: { x: 100, y: 200 },
      to: { x: 300, y: 200 },
    });
    expect(projection.portals).toHaveLength(1);
    expect(projection.portals[0]).toMatchObject({
      employeeId: "emp-b",
      targetFloorId: "floor-2",
      direction: "outgoing",
    });
    // emp-c -> emp-d：emp-d 无座位（候岗溢出），不画线只计数。
    expect(projection.unlocatedEdgeCount).toBe(1);
  });

  it("shows the incoming portal on the destination floor", () => {
    const projection = projectLensForFloor(lens, overview, "floor-2");

    expect(projection.fullEdges).toHaveLength(0);
    expect(projection.portals).toHaveLength(1);
    expect(projection.portals[0]).toMatchObject({
      employeeId: "emp-c",
      targetFloorId: "floor-1",
      direction: "incoming",
    });
  });

  it("lists the floors hosting lens participants", () => {
    expect(lensParticipantFloorIds(lens, overview).sort()).toEqual(["floor-1", "floor-2"]);
  });
});

describe("aggregateLensProjectOptions", () => {
  it("groups employee project summaries and sorts by activity", () => {
    const employees = [
      {
        ...overviewEmployee("emp-a", "floor-1", "s1"),
        projects: [
          {
            projectId: "p-1",
            name: "项目一",
            status: "active",
            isMember: true,
            activeTaskCount: 2,
            workingTaskCount: 1,
            totalTaskCount: 3,
            lastActivityAt: "2026-07-18T01:00:00Z",
          },
        ],
      },
      {
        ...overviewEmployee("emp-b", "floor-1", "s2"),
        projects: [
          {
            projectId: "p-1",
            name: "项目一",
            status: "active",
            isMember: true,
            activeTaskCount: 1,
            workingTaskCount: 0,
            totalTaskCount: 3,
            lastActivityAt: "2026-07-18T02:00:00Z",
          },
          {
            projectId: "p-2",
            name: "项目二",
            status: "active",
            isMember: false,
            activeTaskCount: 0,
            workingTaskCount: 0,
            totalTaskCount: 1,
            lastActivityAt: "2026-07-17T00:00:00Z",
          },
        ],
      },
    ];
    const options = aggregateLensProjectOptions(overviewWith([], employees));

    expect(options.map((option) => option.projectId)).toEqual(["p-1", "p-2"]);
    expect(options[0]).toMatchObject({
      participantCount: 2,
      activeTaskCount: 3,
      workingTaskCount: 1,
      lastActivityAt: "2026-07-18T02:00:00Z",
    });
  });
});
