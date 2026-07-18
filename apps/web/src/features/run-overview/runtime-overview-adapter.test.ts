import { describe, expect, it } from "vitest";
import type { DigitalEmployeeOverview } from "@/lib/api/employees";
import type { TeamListItem } from "@/lib/api/teams";
import { buildRuntimeOverview } from "./runtime-overview-adapter";

const teams: TeamListItem[] = [
  {
    id: "team-dev",
    tenant_id: "tenant-1",
    slug: "dev",
    name: "开发团队",
    status: "active",
    constitution: {},
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
    constitution: {},
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
    operational_state: { status, reasons: [], can_dispatch: status !== "error" },
    recent_events: [{ label: title || "暂无任务", status, occurred_at: "2026-07-05T10:00:00Z" }],
    latest_run_summary: title
      ? {
          run_id: `${id}-run`,
          task_id: `${id}-task`,
          status: status === "working" ? "running" : "none",
          title,
          error_message: "",
          token_usage: 128,
        }
      : null,
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
    project_summary: title
      ? {
          project_count: 2,
          projects: [
            {
              project_id: `${id}-project`,
              name: `${teamName}项目`,
              status: "active",
              is_member: true,
              active_task_count: 2,
              working_task_count: 1,
              total_task_count: 4,
              last_activity_at: "2026-07-05T09:30:00Z",
            },
            {
              project_id: "project-shared",
              name: "共享项目",
              status: "active",
              is_member: false,
              active_task_count: 1,
              working_task_count: 0,
              total_task_count: 1,
              last_activity_at: "2026-07-04T09:30:00Z",
            },
          ],
        }
      : { project_count: 0, projects: [] },
  };
}

describe("buildRuntimeOverview", () => {
  it("maps existing Control Plane read models into office-zone capacity workspaces", () => {
    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees,
      generatedAt: "2026-07-05T10:00:00Z",
      teams,
    });

    expect(overview.summary.employeeCount).toBe(5);
    expect(overview.summary.capacityTotal).toBe(7);
    expect(overview.summary.workingCount).toBe(2);
    expect(overview.floors).toHaveLength(3);
    expect(overview.summary.todayTokensTotal).toBe(5 * 128);
    // emp-1 / emp-2 / emp-4 / emp-5 带任务：各自专属项目 4 个 + 共享项目 1 个（去重）。
    expect(overview.summary.linkedProjectCount).toBe(5);
    const emp1 = overview.employees.find((item) => item.employeeId === "emp-1");
    expect(emp1?.projectCount).toBe(2);
    expect(emp1?.projects.map((project) => project.projectId)).toEqual(["emp-1-project", "project-shared"]);
    expect(emp1?.statusReasons).toEqual([]);
    // 无 started_at 时回退到最近活动时间（recent_events）。
    expect(emp1?.statusSince).toBe("2026-07-05T10:00:00Z");
    expect(emp1?.latestRunErrorMessage).toBeUndefined();
    expect(overview.recentActivity[0]?.employeeName).toBeTruthy();
    expect(overview.recentActivity.length).toBeGreaterThan(0);
    expect(overview.teams.find((team) => team.teamId === "team-ops")?.employeeCount).toBe(2);
    expect(overview.teams.find((team) => team.teamId === "team-ops")?.overCapacity).toBe(false);
    expect(overview.teams.find((team) => team.teamId === "team-dev")?.capacity).toBe(3);
    expect(overview.teams.find((team) => team.teamId === "team-ops")?.capacity).toBe(4);
    expect(overview.floors[0].layout.teamWorkspaces[0].seats).toHaveLength(3);
    expect(overview.floors[0].layout.teamWorkspaces[1].seats).toHaveLength(4);
    expect(overview.employees.find((item) => item.employeeId === "emp-1")?.seatId).toBe("team-ops-seat-1");
  });

  it("moves a growing team to a workspace that can hold the current employees", () => {
    const fiveOps = Array.from({ length: 5 }, (_, index) =>
      employee(`ops-${index + 1}`, `运维 ${index + 1}`, "运维工程师 AI", "team-ops", "运维团队", "idle"),
    );
    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: { ...employees, items: fiveOps, pagination: { limit: 100, offset: 0, total_count: 5 } },
      generatedAt: "2026-07-05T10:00:00Z",
      teams,
    });

    expect(overview.teams.find((team) => team.teamId === "team-ops")?.capacity).toBeGreaterThanOrEqual(5);
    expect(overview.employees.filter((item) => item.teamId === "team-ops" && item.seatId).length).toBe(5);
    expect(overview.teams.find((team) => team.teamId === "team-ops")?.overCapacity).toBe(false);
  });

  it("assigns a newly populated larger team to a workspace that can hold its employees", () => {
    const newTeam: TeamListItem = {
      id: "team-large",
      tenant_id: "tenant-1",
      slug: "large",
      name: "我来也团队",
      status: "active",
      constitution: {},
      member_count: 0,
      digital_employee_count: 8,
      capability_count: 0,
      governance_status: "active",
      pending_draft_count: 0,
      risk_summary: "normal",
    };
    const largeTeamEmployees = Array.from({ length: 8 }, (_, index) =>
      employee(`large-${index + 1}`, `大团队员工 ${index + 1}`, "数字员工", "team-large", "我来也团队", "idle"),
    );

    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: {
        ...employees,
        items: largeTeamEmployees,
        pagination: { limit: 100, offset: 0, total_count: 8 },
      },
      generatedAt: "2026-07-05T10:00:00Z",
      teams: [newTeam, ...teams],
    });

    const largeTeam = overview.teams.find((team) => team.teamId === "team-large");
    expect(largeTeam?.capacity).toBeGreaterThanOrEqual(8);
    expect(largeTeam?.capacityUsed).toBe(8);
    expect(largeTeam?.overCapacity).toBe(false);
    expect(overview.employees.filter((item) => item.teamId === "team-large" && item.seatId).length).toBe(8);
  });

  it("keeps zero-employee teams beyond floor slots on floor three without assigning a workspace", () => {
    const manyTeams = Array.from({ length: 25 }, (_, index): TeamListItem => ({
      id: `team-${index + 1}`,
      tenant_id: "tenant-1",
      slug: `team-${index + 1}`,
      name: `团队 ${index + 1}`,
      status: "active",
      constitution: {},
      member_count: 0,
      digital_employee_count: 0,
      capability_count: 0,
      governance_status: "active",
      pending_draft_count: 0,
      risk_summary: "normal",
    }));

    const overview = buildRuntimeOverview({
      activeFloorId: "floor-3",
      employees: { ...employees, items: [], pagination: { limit: 100, offset: 0, total_count: 0 } },
      generatedAt: "2026-07-05T10:00:00Z",
      teams: manyTeams,
    });

    expect(overview.floors.find((floor) => floor.floorId === "floor-3")?.teamIds).toContain("team-25");
    expect(overview.floors.find((floor) => floor.floorId === "floor-3")?.layout.teamWorkspaces).toHaveLength(8);
    expect(overview.teams.find((team) => team.teamId === "team-25")?.floorId).toBe("floor-3");
  });

  it("reserves a large workspace for a populated team even when smaller empty teams are listed first", () => {
    const smallTeams = Array.from({ length: 24 }, (_, index): TeamListItem => ({
      id: `team-small-${index + 1}`,
      tenant_id: "tenant-1",
      slug: `team-small-${index + 1}`,
      name: `空团队 ${index + 1}`,
      status: "active",
      constitution: {},
      member_count: 0,
      digital_employee_count: 0,
      capability_count: 0,
      governance_status: "active",
      pending_draft_count: 0,
      risk_summary: "normal",
    }));
    const largeTeam: TeamListItem = {
      id: "team-late-large",
      tenant_id: "tenant-1",
      slug: "late-large",
      name: "后置大团队",
      status: "active",
      constitution: {},
      member_count: 0,
      digital_employee_count: 8,
      capability_count: 0,
      governance_status: "active",
      pending_draft_count: 0,
      risk_summary: "normal",
    };
    const largeTeamEmployees = Array.from({ length: 8 }, (_, index) =>
      employee(`late-large-${index + 1}`, `后置员工 ${index + 1}`, "数字员工", "team-late-large", "后置大团队", "idle"),
    );

    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: {
        ...employees,
        items: largeTeamEmployees,
        pagination: { limit: 100, offset: 0, total_count: 8 },
      },
      generatedAt: "2026-07-05T10:00:00Z",
      teams: [...smallTeams, largeTeam],
    });

    const mappedLargeTeam = overview.teams.find((team) => team.teamId === "team-late-large");
    expect(mappedLargeTeam?.capacity).toBeGreaterThanOrEqual(8);
    expect(mappedLargeTeam?.capacityUsed).toBe(8);
    expect(mappedLargeTeam?.overCapacity).toBe(false);
    expect(overview.employees.filter((item) => item.teamId === "team-late-large" && item.seatId).length).toBe(8);
    expect(overview.floors.flatMap((floor) => floor.layout.teamWorkspaces).some((workspace) => workspace.teamId === "team-small-24")).toBe(false);
  });

  it("fills eight office zones per floor before assigning teams to the next floor", () => {
    const manyTeams = Array.from({ length: 16 }, (_, index): TeamListItem => ({
      id: `team-${index + 1}`,
      tenant_id: "tenant-1",
      slug: `team-${index + 1}`,
      name: `团队 ${index + 1}`,
      status: "active",
      constitution: {},
      member_count: 0,
      digital_employee_count: 0,
      capability_count: 0,
      governance_status: "active",
      pending_draft_count: 0,
      risk_summary: "normal",
    }));

    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: { ...employees, items: [], pagination: { limit: 100, offset: 0, total_count: 0 } },
      generatedAt: "2026-07-05T10:00:00Z",
      teams: manyTeams,
    });

    expect(overview.floors.find((floor) => floor.floorId === "floor-1")?.teamIds).toHaveLength(8);
    expect(overview.floors.find((floor) => floor.floorId === "floor-2")?.teamIds).toHaveLength(8);
    expect(overview.floors.find((floor) => floor.floorId === "floor-3")?.teamIds).toHaveLength(0);
    expect(overview.teams.find((team) => team.teamId === "team-9")?.floorId).toBe("floor-2");
  });

  it("uses mapped employees, not fallback team counts, when computing map seat usage", () => {
    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: {
        ...employees,
        items: employees.items.filter((item) => item.identity_summary.team_id === "team-dev"),
        pagination: { limit: 100, offset: 0, total_count: 3 },
      },
      generatedAt: "2026-07-05T10:00:00Z",
      teams,
    });

    expect(overview.teams.find((team) => team.teamId === "team-ops")?.employeeCount).toBe(11);
    expect(overview.teams.find((team) => team.teamId === "team-ops")?.capacityUsed).toBe(0);
    expect(overview.summary.capacityUsed).toBe(3);
    expect(overview.summary.capacityTotal).toBe(13);
  });

  it("assigns seats by API item order instead of operational status", () => {
    const orderedEmployees = [
      employee("ordered-error", "先到异常", "运维工程师 AI", "team-ops", "运维团队", "error"),
      employee("ordered-working", "后到工作", "运维工程师 AI", "team-ops", "运维团队", "working"),
      employee("ordered-idle", "最后空闲", "运维工程师 AI", "team-ops", "运维团队", "idle"),
    ];

    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: { ...employees, items: orderedEmployees, pagination: { limit: 100, offset: 0, total_count: 3 } },
      generatedAt: "2026-07-05T10:00:00Z",
      teams,
    });

    expect(overview.employees.map((item) => [item.employeeId, item.seatId])).toEqual([
      ["ordered-error", "team-ops-seat-1"],
      ["ordered-working", "team-ops-seat-2"],
      ["ordered-idle", "team-ops-seat-3"],
    ]);
  });

  it("uses the shared stable avatar fallback when overview items do not include avatar assets", () => {
    const [itemWithoutAvatar] = [employee("no-avatar-1", "无头像员工", "运维工程师 AI", "team-ops", "运维团队", "idle")];
    delete itemWithoutAvatar.identity_summary.avatar_asset;

    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: { ...employees, items: [itemWithoutAvatar], pagination: { limit: 100, offset: 0, total_count: 1 } },
      generatedAt: "2026-07-05T10:00:00Z",
      teams,
    });

    const mapped = overview.employees.find((item) => item.employeeId === "no-avatar-1");
    expect(mapped?.avatarAsset?.url).toMatch(/^\/images\/digital-employee-avatars\/engineer-[mf]-\d{2}-256\.webp$/);
  });

  it("seats unassigned employees in the floor-one lobby without touching team capacity", () => {
    const withUnassigned = {
      ...employees,
      items: [...employees.items, unassignedEmployee("emp-free-1", "赵新")],
      pagination: { limit: 100, offset: 0, total_count: 6 },
    };
    const base = buildRuntimeOverview({ activeFloorId: "floor-1", employees, generatedAt: "2026-07-05T10:00:00Z", teams });
    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: withUnassigned,
      generatedAt: "2026-07-05T10:00:00Z",
      teams,
    });

    const mapped = overview.employees.find((item) => item.employeeId === "emp-free-1");
    expect(mapped?.teamId).toBe("unassigned");
    expect(mapped?.floorId).toBe("floor-1");
    expect(mapped?.seatId).toBe("unassigned-seat-1");
    // 候岗不占团队容量：容量汇总与无候岗员工时完全一致，也不出现在团队列表。
    expect(overview.summary.capacityTotal).toBe(base.summary.capacityTotal);
    expect(overview.summary.capacityUsed).toBe(base.summary.capacityUsed);
    expect(overview.teams.some((team) => team.teamId === "unassigned")).toBe(false);
  });

  it("leaves lobby overflow employees visible in the list but without a seat", () => {
    const lobbyCrowd = [1, 2, 3, 4].map((index) => unassignedEmployee(`emp-free-${index}`, `候岗${index}`));
    const overview = buildRuntimeOverview({
      activeFloorId: "floor-1",
      employees: { ...employees, items: lobbyCrowd, pagination: { limit: 100, offset: 0, total_count: 4 } },
      generatedAt: "2026-07-05T10:00:00Z",
      teams,
    });

    const seatIds = overview.employees.map((item) => item.seatId);
    expect(seatIds.slice(0, 3)).toEqual(["unassigned-seat-1", "unassigned-seat-2", "unassigned-seat-3"]);
    expect(seatIds[3]).toBeUndefined();
  });
});

function unassignedEmployee(id: string, name: string) {
  const item = employee(id, name, "分析师 AI", "team-x", "", "idle");
  item.identity_summary.team_id = undefined;
  return item;
}
