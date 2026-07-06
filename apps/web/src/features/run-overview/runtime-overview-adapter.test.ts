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
    expect(overview.teams.find((team) => team.teamId === "team-ops")?.employeeCount).toBe(2);
    expect(overview.teams.find((team) => team.teamId === "team-ops")?.overCapacity).toBe(false);
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

  it("keeps teams beyond floor slots on floor three without assigning a workspace", () => {
    const manyTeams = Array.from({ length: 14 }, (_, index): TeamListItem => ({
      id: `team-${index + 1}`,
      tenant_id: "tenant-1",
      slug: `team-${index + 1}`,
      name: `团队 ${index + 1}`,
      status: "active",
      member_count: 0,
      digital_employee_count: 0,
      capability_count: 0,
      governance_status: "active",
      pending_draft_count: 0,
      risk_summary: "normal",
    }));
    const overflowEmployee = employee("overflow-1", "周越", "协作工程师 AI", "team-14", "团队 14", "working");

    const overview = buildRuntimeOverview({
      activeFloorId: "floor-3",
      employees: { ...employees, items: [overflowEmployee], pagination: { limit: 100, offset: 0, total_count: 1 } },
      generatedAt: "2026-07-05T10:00:00Z",
      teams: manyTeams,
    });

    expect(overview.floors.find((floor) => floor.floorId === "floor-3")?.teamIds).toContain("team-14");
    expect(overview.floors.find((floor) => floor.floorId === "floor-3")?.layout.teamWorkspaces).toHaveLength(3);
    expect(overview.teams.find((team) => team.teamId === "team-14")?.floorId).toBe("floor-3");
    expect(overview.employees.find((item) => item.employeeId === "overflow-1")?.floorId).toBe("floor-3");
    expect(overview.employees.find((item) => item.employeeId === "overflow-1")?.seatId).toBeUndefined();
  });

  it("uses team counts for zero-fetched teams when computing capacity usage", () => {
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
    expect(overview.summary.capacityUsed).toBe(14);
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
});
