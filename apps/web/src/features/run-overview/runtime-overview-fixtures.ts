import type { DigitalEmployeeActivity, DigitalEmployeeOverview } from "@/lib/api/employees";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import type { TeamListItem } from "@/lib/api/teams";

export const digitalEmployeeActivityFixture: DigitalEmployeeActivity = {
  items: [
    {
      event_id: "evt-1",
      event_type: "run_completed",
      label: "运行完成",
      status: "completed",
      occurred_at: "2026-07-05T10:05:00Z",
      run_id: "emp-ops-1-run",
      task_id: "emp-ops-1-task",
      task_title: "排查线上告警并生成修复计划",
      digital_employee_id: "emp-ops-1",
      digital_employee_name: "高秀英",
      team_id: "team-ops",
      project_id: "emp-ops-1-project",
      project_name: "运维团队交付项目",
    },
    {
      event_id: "evt-2",
      event_type: "tool_started",
      label: "开始调用工具",
      status: "running",
      occurred_at: "2026-07-05T10:04:00Z",
      run_id: "emp-dev-1-run",
      task_id: "emp-dev-1-task",
      task_title: "实现运行态组件",
      digital_employee_id: "emp-dev-1",
      digital_employee_name: "陆一鸣",
      team_id: "team-dev",
      project_name: "",
    },
  ],
  next_since: "2026-07-05T10:05:00Z|evt-1",
};

export const teamListFixture: TeamListItem[] = [
  {
    id: "team-dev",
    tenant_id: "tenant-1",
    slug: "dev",
    name: "开发团队",
    status: "active",
    constitution: {},
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
    constitution: {},
    member_count: 2,
    digital_employee_count: 2,
    capability_count: 4,
    governance_status: "active",
    pending_draft_count: 0,
    risk_summary: "normal",
  },
];

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

// 未归属团队的员工：验证候岗区落座与"未归属团队"文案。
export const digitalEmployeeOverviewWithUnassignedFixture: DigitalEmployeeOverview = {
  ...digitalEmployeeOverviewFixture,
  summary: {
    ...digitalEmployeeOverviewFixture.summary,
    total_count: 6,
    operational_status_counts: { working: 2, idle: 3, waiting_human: 1 },
  },
  pagination: { limit: 100, offset: 0, total_count: 6 },
  items: [...digitalEmployeeOverviewFixture.items, employee("emp-free-1", "赵新", "分析师 AI", null, "", "idle")],
};

// 项目透镜 fixture：陆一鸣(emp-dev-1) → 高秀英(emp-ops-1) → 沈嘉(emp-dev-2) 的交接链；
// 中段任务运行中(primary)、首段已完成(muted)，另有一条未派发任务。
export const projectTaskGraphFixture: ProjectTaskGraph = {
  nodes: [
    graphNode("task-a", "整理告警上下文", "completed", "emp-dev-1", 0),
    graphNode("task-b", "定位根因并出修复计划", "running", "emp-ops-1", 1),
    graphNode("task-c", "复核修复计划", "pending", "emp-dev-2", 2),
    graphNode("task-d", "编写回归清单", "pending", undefined, 2),
  ],
  edges: [
    { dependent_task_id: "task-b", blocker_task_id: "task-a", edge_status: "satisfied" },
    { dependent_task_id: "task-c", blocker_task_id: "task-b", edge_status: "blocking" },
  ],
  employees: [
    { digital_employee_id: "emp-dev-1", display_name: "陆一鸣", project_role: "executor", status: "active" },
    { digital_employee_id: "emp-ops-1", display_name: "高秀英", project_role: "executor", status: "active" },
    { digital_employee_id: "emp-dev-2", display_name: "沈嘉", project_role: "executor", status: "active" },
  ],
  runs: [],
  execution_summaries: [],
  recent_events: [],
  decision_requests: [],
  blocking_facts: [],
};

function graphNode(
  id: string,
  title: string,
  status: string,
  assignedDigitalEmployeeId: string | undefined,
  stageIndex: number,
): ProjectTaskGraph["nodes"][number] {
  return {
    id,
    tenant_id: "tenant-1",
    project_id: "project-lens-1",
    title,
    status,
    assigned_digital_employee_id: assignedDigitalEmployeeId,
    requires_human_approval: false,
    stage_index: stageIndex,
    expected_outputs: [],
    input_requirements: {},
    handoff_contract: {},
    planner_metadata: {},
  };
}

function employee(
  id: string,
  name: string,
  role: string,
  teamId: string | null,
  teamName: string,
  status: DigitalEmployeeOverview["items"][number]["operational_state"]["status"],
  title = "",
): DigitalEmployeeOverview["items"][number] {
  return {
    identity_summary: {
      id,
      tenant_id: "tenant-1",
      team_id: teamId ?? undefined,
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
    operational_state: {
      status,
      reasons: status === "waiting_human" ? [{ code: "approval_blocked", message: "等待人工确认后继续执行" }] : [],
      can_dispatch: status !== "waiting_human",
    },
    recent_events: [{ label: title ? "已领取任务" : "暂无任务", status, occurred_at: "2026-07-05T10:00:00Z" }],
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
    project_summary: title
      ? {
          project_count: 1,
          projects: [
            {
              project_id: `${id}-project`,
              name: `${teamName}交付项目`,
              status: "active",
              is_member: true,
              active_task_count: status === "working" ? 2 : 1,
              working_task_count: status === "working" ? 1 : 0,
              total_task_count: 3,
              last_activity_at: "2026-07-05T10:00:00Z",
            },
          ],
        }
      : { project_count: 0, projects: [] },
  };
}
