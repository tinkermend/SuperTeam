import type { ApiClientOptions } from "./client";
import { buildApiUrl, deleteJson, getJson, parseJson, postJson, putJson } from "./client";

export type DigitalEmployeeStatus =
  | "draft"
  | "ready"
  | "active"
  | "disabled"
  | "error";

export type DigitalEmployeeRunKind = "task" | "chat";

export type DigitalEmployeeAvatarAsset = {
  id: string;
  label: string;
  gender: string;
  age_range: string;
  style: string;
  image_url: string;
  thumbnail_url: string;
  source: string;
  license: string;
  status: string;
  /** 头像独占：已被在册数字员工占用的头像不可再选（仅创建选项接口返回）。 */
  in_use?: boolean;
};

export type DigitalEmployee = {
  id: string;
  tenant_id: string;
  team_id?: string;
  owner_user_id: string;
  employee_type: string;
  provider_type: string;
  name: string;
  role: string;
  description?: string;
  status: DigitalEmployeeStatus;
  permission_policy: Record<string, unknown>;
  persona_memory_markdown?: string;
  capability_bindings?: CapabilityBindings;
  budget_policy?: BudgetPolicy;
  risk_level: string;
  metadata?: Record<string, unknown> & {
    avatar?: Record<string, unknown>;
    avatar_asset_id?: string;
    effective_config_label?: string;
    effective_config_status?:
      | "approved"
      | "draft"
      | "stale"
      | "missing"
      | string;
  };
  disabled_at?: string;
  archived_at?: string;
  allowed_actions?: string[];
  created_at?: string;
  updated_at?: string;
  // 与运行总览/员工列表同源的运行态裁决(跨视图一致性 P2 3.3a);详情页据此判断忙碌,
  // 取代前端本地基于 runs 列表的 hasActiveRun。后端单员工读路径填充,缺省时前端回退本地判断。
  operational_state?: DigitalEmployeeOperationalState;
};

export type DigitalEmployeeDeleteBlocker = {
  type: "run" | "project_task";
  id: string;
  status: string;
  title: string;
  run_id?: string;
  project_id?: string;
};

export type DigitalEmployeeDeleteBlockedErrorResponse = {
  code: "digital_employee_delete_blocked";
  message: string;
  blockers: DigitalEmployeeDeleteBlocker[];
};

export type DigitalEmployeeTypeOption = {
  type: string;
  label: string;
  description: string;
  default_role: string;
  recommended_skills?: string[];
  recommended_mcp_servers?: string[];
  recommended_provider_types?: string[];
  persona_memory_markdown?: string;
  capability_bindings?: Record<string, unknown>;
  budget_policy?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
};

export type DigitalEmployeeCapabilityOptionItem = {
  key: string;
  id?: string;
  label: string;
  description?: string;
  recommended: boolean;
  available: boolean;
  risk_level?: string;
};

export type DigitalEmployeeCapabilityOptions = {
  provider_types: string[];
  skills: DigitalEmployeeCapabilityOptionItem[];
  mcp_servers: DigitalEmployeeCapabilityOptionItem[];
};

export type DigitalEmployeeRuntimeProviderOption = {
  runtime_node_id?: string;
  node_id: string;
  runtime_name: string;
  provider_type: string;
  runtime_status: string;
  provider_status: string;
  health_status: string;
  current_load: number;
  max_slots: number;
  agent_home_dir: string;
  agent_home_dir_available: boolean;
  available: boolean;
  disabled_reason?: string;
};

export type DigitalEmployeeCreateOptionCheck = {
  key: string;
  label: string;
  status: "passed" | "warning" | "blocked";
  message: string;
};

export type DigitalEmployeePolicyDefaults = {
  permission_policy: Record<string, unknown>;
  approval_policy: Record<string, unknown>;
  workspace_policy: Record<string, unknown>;
  session_policy: Record<string, unknown>;
  metadata: Record<string, unknown>;
};

export type DigitalEmployeeCreateOptions = {
  team_config: {
    id: string;
    tenant_id: string;
    team_id?: string;
    constitution: Record<string, unknown>;
    skills: string[];
    mcp_servers: string[];
  };
  employee_types: DigitalEmployeeTypeOption[];
  capability_options: DigitalEmployeeCapabilityOptions;
  runtime_provider_options: DigitalEmployeeRuntimeProviderOption[];
  creation_checks: DigitalEmployeeCreateOptionCheck[];
  policy_defaults: DigitalEmployeePolicyDefaults;
};

export type SchedulingReadinessCheck = {
  code: string;
  status: "passed" | "warning" | "blocked" | "info";
  label: string;
  message: string;
};

export type SchedulingReadinessSkillSummary = {
  personal_count: number;
  inherited_count: number;
  missing_required: string[];
};

export type SchedulingReadinessMcpSummary = {
  personal_count: number;
  inherited_count: number;
};

export type SchedulingReadinessEnvironmentSummary = {
  configured_count: number;
  missing_names: string[];
};

export type SchedulingReadinessCapabilities = {
  skills: SchedulingReadinessSkillSummary;
  mcp_servers: SchedulingReadinessMcpSummary;
  environment_variables: SchedulingReadinessEnvironmentSummary;
};

export type DigitalEmployeeSchedulingReadiness = {
  employee_id: string;
  status: DigitalEmployeeStatus;
  ready_for_project_scheduling: boolean;
  project_execution_source: string;
  checks: SchedulingReadinessCheck[];
  capabilities: SchedulingReadinessCapabilities;
};

export type DigitalEmployeeRunStatus =
  | "queued"
  | "dispatching"
  | "running"
  | "cancelling"
  | "completed"
  | "failed"
  | "cancelled"
  | "timed_out";

export type DigitalEmployeeRunInput = {
  objective: string;
  prompt?: string;
  context_refs?: Array<Record<string, unknown>>;
  artifact_refs?: Array<Record<string, unknown>>;
  output_schema?: Record<string, unknown>;
  allowed_actions?: string[];
  forbidden_actions?: string[];
  secret_refs?: string[];
  idempotency_key?: string;
  timeout_sec?: number;
  grace_sec?: number;
  metadata?: Record<string, unknown>;
  run_kind?: DigitalEmployeeRunKind;
  resume_of_run_id?: string;
  /** Required when run_kind is "chat": anchors the chat run to a project for
   * node resolution, budget and policy boundaries. Ignored for task runs. */
  project_id?: string;
};

export type DigitalEmployeeRun = {
  id: string;
  tenant_id: string;
  task_id: string;
  digital_employee_id: string;
  execution_instance_id: string;
  runtime_node_id: string;
  node_id: string;
  command_id: string;
  provider_type: string;
  provider_session_id?: string;
  provider_session_external_id?: string;
  status: DigitalEmployeeRunStatus;
  result: Record<string, unknown>;
  diagnostic: Record<string, unknown>;
  log_ref?: string;
  raw_result_ref?: string;
  work_products: Array<Record<string, unknown>>;
  session_state: Record<string, unknown>;
  error_message?: string;
  error_code?: string;
  error_family?: string;
  exit_code?: number;
  signal?: string;
  timed_out: boolean;
  idempotency_key?: string;
  timeout_sec?: number;
  grace_sec?: number;
  failure_acknowledged_at?: string;
  started_at?: string;
  completed_at?: string;
  finished_at?: string;
  created_at?: string;
  updated_at?: string;
  run_kind: DigitalEmployeeRunKind;
  resume_of_run_id?: string;
  /** Effective chat conversation id (thread root run id); present on chat runs only. */
  chat_thread_id?: string;
};

export type DigitalEmployeeRunEvent = {
  event_type: string;
  sequence_number: number;
  payload: Record<string, unknown>;
  provider_session_external_id?: string;
  session_state_patch?: Record<string, unknown>;
  log_ref?: string;
  raw_event_ref?: string;
  metadata?: Record<string, unknown>;
};

export type DigitalEmployeeRunStats = {
  total_count: number;
  succeeded_count: number;
  failed_count: number;
  cancelled_count: number;
  success_rate: number | null;
  avg_duration_sec: number | null;
  p90_duration_sec: number | null;
  last_7d_count: number;
  prev_7d_count: number;
};

export type DigitalEmployeeRunFilterOption = {
  value: string;
  label: string;
};

export type DigitalEmployeeRunListItem = DigitalEmployeeRun & {
  task_title: string;
  project_id?: string;
  project_name?: string;
  work_product_count: number;
  duration_sec?: number;
};

export type DigitalEmployeeRunListResult = {
  items: DigitalEmployeeRunListItem[];
  total_count: number;
  filters: {
    statuses: DigitalEmployeeRunFilterOption[];
    projects: DigitalEmployeeRunFilterOption[];
  };
};

export type ListDigitalEmployeeRunsFilter = RunPagination & {
  status?: DigitalEmployeeRunStatus[];
  project_id?: string;
  from?: string;
  to?: string;
  run_kind?: DigitalEmployeeRunKind;
  /** Narrow to one chat conversation (thread root run id); matches the root
   * turn and its follow-ups, chat runs only. */
  chat_thread_id?: string;
};

export type DigitalEmployeeOverviewExecutionStatus =
  | "missing"
  | "provisioning"
  | "ready"
  | "active"
  | "disabled"
  | "error";

export type DigitalEmployeeOverviewRunStatus =
  | "none"
  | DigitalEmployeeRunStatus;

export type BudgetPolicy = {
  daily_token_limit?: number | null;
};

export type DigitalEmployeeWorkbenchStatus =
  | "ready"
  | "needs_configuration"
  | "error";

export type DigitalEmployeeOperationalStatus =
  | "working"
  | "idle"
  | "queued"
  | "waiting_human"
  | "error"
  | "unavailable"
  | "needs_configuration";

export type DigitalEmployeeOperationalReason = {
  code: string;
  message: string;
};

export type DigitalEmployeeOperationalState = {
  status: DigitalEmployeeOperationalStatus;
  reasons: DigitalEmployeeOperationalReason[];
  can_dispatch: boolean;
};

export type DigitalEmployeeRecentEventSummary = {
  label: string;
  status: string;
  occurred_at?: string;
};

export type DigitalEmployeeProjectLinkSummary = {
  project_id: string;
  name: string;
  status: string;
  is_member: boolean;
  active_task_count: number;
  working_task_count: number;
  total_task_count: number;
  last_activity_at?: string;
};

export type DigitalEmployeeProjectSummary = {
  project_count: number;
  projects: DigitalEmployeeProjectLinkSummary[];
};

export type DigitalEmployeeActivityItem = {
  event_id: string;
  event_type: string;
  label: string;
  status: string;
  occurred_at?: string;
  run_id: string;
  task_id: string;
  task_title: string;
  digital_employee_id: string;
  digital_employee_name: string;
  team_id?: string;
  project_id?: string;
  project_name: string;
};

export type DigitalEmployeeActivity = {
  items: DigitalEmployeeActivityItem[];
  next_since: string;
};

export type OverviewFilterOption = {
  value: string;
  label: string;
};

export type DigitalEmployeeOverview = {
  summary: {
    total_count: number;
    runnable_count: number;
    running_count: number;
    waiting_runtime_count: number;
    error_count: number;
    high_risk_count: number;
    ready_count: number;
    needs_configuration_count: number;
    pending_config_approval_count: number;
    failed_recent_run_count: number;
    operational_status_counts: Partial<Record<DigitalEmployeeOperationalStatus, number>>;
  };
  queue_summary: {
    needs_configuration_count: number;
    stale_config_count: number;
    failed_recent_run_count: number;
  };
  items: DigitalEmployeeOverviewItem[];
  filters: {
    teams: OverviewFilterOption[];
    employee_types: OverviewFilterOption[];
    statuses: OverviewFilterOption[];
    providers: OverviewFilterOption[];
    risk_levels: OverviewFilterOption[];
    run_statuses: OverviewFilterOption[];
  };
  pagination: {
    limit: number;
    offset: number;
    total_count: number;
  };
};

export type DigitalEmployeeOverviewItem = {
  workbench_status: DigitalEmployeeWorkbenchStatus;
  operational_state: DigitalEmployeeOperationalState;
  recent_events: DigitalEmployeeRecentEventSummary[];
  project_summary: DigitalEmployeeProjectSummary;
  identity_summary: {
    id: string;
    tenant_id: string;
    team_id?: string;
    team_name: string;
    owner_user_id: string;
    owner_display_name: string;
    employee_type: string;
    employee_type_label: string;
    /** 身份级主 Provider 类型(claude/codex/opencode);运行落点由项目派发动态解析,与本字段无关 */
    provider_type: string;
    name: string;
    role: string;
    description?: string;
    status: DigitalEmployeeStatus;
    risk_level: string;
    avatar_asset?: DigitalEmployeeAvatarAsset;
  };
  execution_summary: {
    execution_instance_id?: string;
    status: DigitalEmployeeOverviewExecutionStatus;
    runtime_node_id?: string;
    node_id: string;
    runtime_name: string;
    runtime_status: string;
    provider_type: string;
    provider_status: string;
    health_status: string;
    agent_home_dir_available: boolean;
  };
  latest_run_summary?: {
    run_id: string;
    task_id: string;
    status: DigitalEmployeeOverviewRunStatus;
    title: string;
    started_at?: string;
    finished_at?: string;
    updated_at?: string;
    duration_sec?: number;
    token_usage?: number;
    error_message: string;
  } | null;
  governance_summary: {
    effective_config_id?: string;
    status: string;
    team_revision_number?: number;
    employee_revision_number?: number;
    skills_count: number;
    mcp_servers_count: number;
    constitution_ref: string;
  };
  budget_summary: {
    usage_tokens_30d?: number;
    run_count_30d: number;
    cost_amount_30d?: number;
    currency: string;
    source: string;
    daily_token_limit?: number | null;
    usage_tokens_today: number;
    usage_percent_today?: number | null;
    limit_exceeded: boolean;
  };
  // working 状态的权威成因:当前 running/in_progress 的项目任务及其所属项目(与后端
  // operational working 判定同源)。用于座位卡精确显示"正在 X 项目做 Y 任务"并深链,
  // 替代从 latest_run(另一数据源)+ project_summary 聚合的启发式拼接。无正在执行任务时为 null。
  current_work?: {
    project_id: string;
    project_name: string;
    project_task_id: string;
    project_task_name: string;
  } | null;
};

export type RunPagination = {
  limit?: number;
  offset?: number;
};

export type StopDigitalEmployeeRunInput = {
  reason: string;
};

export type CreateDigitalEmployeeInput = {
  team_id?: string;
  employee_type: string;
  name: string;
  avatar_asset_id: string;
  role?: string;
  description?: string;
  permission_policy?: Record<string, unknown>;
  risk_level?: string;
  metadata?: Record<string, unknown>;
  persona_memory_markdown?: string;
  /** 技能注册表 slug 列表:创建即写逻辑绑定,派发时物化。 */
  skills?: string[];
  /** MCP 注册表 server_key 列表:同上。 */
  mcp_servers?: string[];
  /** 仅承载 external_capabilities/environment_variable_refs;skills/mcp_servers 已废弃并被服务端剥离。 */
  capability_bindings?: CapabilityBindings;
  provider_type: string;
  budget_policy?: BudgetPolicy;
  environment_variables?: Array<{ name: string; value: string; sensitive: boolean }>;
};

export type DigitalEmployeeEnvironmentVariableSummary = {
  name: string;
  configured: boolean;
  fingerprint: string;
  sensitive: boolean;
  status: "active" | "disabled";
  updated_at?: string;
};

export type UpsertDigitalEmployeeEnvironmentVariableInput = {
  value: string;
  sensitive?: boolean;
};

export type CapabilityBindings = {
  skills?: string[];
  mcp_servers?: string[];
  external_capabilities?: string[];
  environment_variable_refs?: string[];
  [key: string]: unknown;
};

type LegacyDraftDigitalEmployeeInput = {
  team_id: string;
  name: string;
  role: string;
  description?: string;
};

// Temporary compatibility for old inline create forms until the Task 6 creation wizard replaces them.
function assertReadyCreateInput(
  input: CreateDigitalEmployeeInput | LegacyDraftDigitalEmployeeInput,
): asserts input is CreateDigitalEmployeeInput {
  if (
    !("employee_type" in input) ||
    !input.employee_type ||
    !("avatar_asset_id" in input) ||
    !input.avatar_asset_id ||
    !("provider_type" in input) ||
    !input.provider_type
  ) {
    throw new Error(
      "digital employee ready creation requires employee_type, avatar_asset_id, and provider_type",
    );
  }
}

export type ListDigitalEmployeesFilters = {
  team_id?: string;
  assignment?: string;
};

export type DigitalEmployeeOverviewFilters = {
  q?: string;
  team_id?: string;
  status?: DigitalEmployeeStatus;
  employee_type?: string;
  provider_type?: string;
  risk_level?: string;
  run_status?: DigitalEmployeeOverviewRunStatus;
  limit?: number;
  offset?: number;
};

export type DigitalEmployeeConfigRevision = {
  id: string;
  tenant_id: string;
  digital_employee_id: string;
  revision_number: number;
  persona_memory_markdown: string;
  capability_bindings: CapabilityBindings;
  budget_policy: BudgetPolicy;
  status: "draft" | "active" | "archived";
  approved_by?: string;
  approved_at?: string;
  archived_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateDigitalEmployeeConfigRevisionInput = {
  persona_memory_markdown?: string;
  capability_bindings?: CapabilityBindings;
  budget_policy?: BudgetPolicy;
  status?: "draft" | "active" | "archived";
};

function encodePathSegment(value: string) {
  return encodeURIComponent(value);
}

function paginationQuery(pagination: RunPagination = {}) {
  const searchParams = new URLSearchParams();
  if (pagination.limit !== undefined) {
    searchParams.set("limit", String(pagination.limit));
  }
  if (pagination.offset !== undefined) {
    searchParams.set("offset", String(pagination.offset));
  }
  const query = searchParams.toString();

  return query ? `?${query}` : "";
}

export async function listDigitalEmployees(
  options: ApiClientOptions,
  filters: ListDigitalEmployeesFilters = {},
): Promise<DigitalEmployee[]> {
  const searchParams = new URLSearchParams();
  if (filters.team_id) {
    searchParams.set("team_id", filters.team_id);
  }
  if (filters.assignment) {
    searchParams.set("assignment", filters.assignment);
  }
  const query = searchParams.toString();
  const path = `/api/v1/digital-employees${query ? `?${query}` : ""}`;

  return getJson<DigitalEmployee[]>(options, path, "digital employees");
}

export async function getDigitalEmployeeOverview(
  options: ApiClientOptions,
  filters: DigitalEmployeeOverviewFilters = {},
): Promise<DigitalEmployeeOverview> {
  const searchParams = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== "") {
      searchParams.set(key, String(value));
    }
  }
  const query = searchParams.toString();
  return getJson<DigitalEmployeeOverview>(
    options,
    `/api/v1/digital-employees/overview${query ? `?${query}` : ""}`,
    "digital employee overview",
  );
}

export async function getDigitalEmployeeActivity(
  options: ApiClientOptions,
  params: { since?: string; limit?: number } = {},
): Promise<DigitalEmployeeActivity> {
  const searchParams = new URLSearchParams();
  if (params.since) searchParams.set("since", params.since);
  if (params.limit !== undefined) searchParams.set("limit", String(params.limit));
  const query = searchParams.toString();
  return getJson<DigitalEmployeeActivity>(
    options,
    `/api/v1/digital-employees/activity${query ? `?${query}` : ""}`,
    "digital employee activity",
  );
}

export function getDigitalEmployeeCreateOptions(
  options: ApiClientOptions,
  teamId?: string,
): Promise<DigitalEmployeeCreateOptions> {
  const searchParams = new URLSearchParams();
  if (teamId) {
    searchParams.set("team_id", teamId);
  }

  return getJson<DigitalEmployeeCreateOptions>(
    options,
    `/api/v1/digital-employees/create-options${searchParams.toString() ? `?${searchParams.toString()}` : ""}`,
    "digital employee create options",
  );
}

export function listDigitalEmployeeAvatarAssets(
  options: ApiClientOptions,
): Promise<DigitalEmployeeAvatarAsset[]> {
  return getJson<DigitalEmployeeAvatarAsset[]>(
    options,
    "/api/v1/digital-employee-avatar-assets",
    "digital employee avatar assets",
  );
}

export function getDigitalEmployee(
  options: ApiClientOptions,
  employeeId: string,
): Promise<DigitalEmployee> {
  return getJson<DigitalEmployee>(
    options,
    `/api/v1/digital-employees/${encodePathSegment(employeeId)}`,
    "digital employee",
  );
}

export function deleteDigitalEmployee(
  options: ApiClientOptions,
  employeeId: string,
): Promise<void> {
  return deleteJson(
    options,
    `/api/v1/digital-employees/${encodePathSegment(employeeId)}`,
    "delete digital employee",
  );
}

export async function createDigitalEmployee(
  options: ApiClientOptions,
  input: CreateDigitalEmployeeInput | LegacyDraftDigitalEmployeeInput,
): Promise<DigitalEmployee> {
  assertReadyCreateInput(input);
  return postJson<DigitalEmployee>(
    options,
    "/api/v1/digital-employees",
    input,
    "create digital employee",
  );
}

export function getDigitalEmployeeSchedulingReadiness(
  options: ApiClientOptions,
  employeeId: string,
): Promise<DigitalEmployeeSchedulingReadiness> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<DigitalEmployeeSchedulingReadiness>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/scheduling-readiness`,
    "digital employee scheduling readiness",
  );
}

export function listEmployeeEnvironmentVariables(
  options: ApiClientOptions,
  employeeId: string,
): Promise<DigitalEmployeeEnvironmentVariableSummary[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<DigitalEmployeeEnvironmentVariableSummary[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/environment-variables`,
    "employee environment variables",
  );
}

/**
 * 换队/首次归队（团队归属参与门禁的归队入口）。目标团队必须存在且 active。
 * 副作用：agent home dir 按 (team, employee) 键，换队后下次派发落新家目录；
 * 团队级技能与 MCP 继承随之切换。
 */
export function reassignDigitalEmployeeTeam(
  options: ApiClientOptions,
  employeeId: string,
  teamId: string,
): Promise<DigitalEmployee> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return putJson<DigitalEmployee>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/team`,
    { team_id: teamId },
    "reassign digital employee team",
  );
}

export function upsertEmployeeEnvironmentVariable(
  options: ApiClientOptions,
  employeeId: string,
  name: string,
  input: UpsertDigitalEmployeeEnvironmentVariableInput,
): Promise<DigitalEmployeeEnvironmentVariableSummary> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const envName = encodePathSegment(name);

  return putJson<DigitalEmployeeEnvironmentVariableSummary>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/environment-variables/${envName}`,
    input,
    "upsert employee environment variable",
  );
}

export async function deleteEmployeeEnvironmentVariable(
  options: ApiClientOptions,
  employeeId: string,
  name: string,
): Promise<void> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const envName = encodePathSegment(name);

  return deleteJson(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/environment-variables/${envName}`,
    "delete employee environment variable",
  );
}

export function createDigitalEmployeeConfigRevision(
  options: ApiClientOptions,
  employeeId: string,
  input: CreateDigitalEmployeeConfigRevisionInput,
): Promise<DigitalEmployeeConfigRevision> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  return postJson<DigitalEmployeeConfigRevision>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/config-revisions`,
    input,
    "create digital employee config revision",
  );
}

export type SubmitPermissionChangeInput = {
  /** 不改 role 时省略。 */
  role?: string;
  /** 不改 permission_policy 时省略;形状 {grants?: string[], allowed_actions?: string[]}。 */
  permission_policy?: Record<string, unknown>;
};

export type SubmitPermissionChangeResponse = {
  id: string;
  resource_type: string;
  status: string;
  category: string;
  target_user_id: string;
};

/** 提交 role/permission_policy 治理变更 → 产生权限中心审批请求,批准后写回员工行。 */
export function submitEmployeePermissionChange(
  options: ApiClientOptions,
  employeeId: string,
  input: SubmitPermissionChangeInput,
): Promise<SubmitPermissionChangeResponse> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  return postJson<SubmitPermissionChangeResponse>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/permission-changes`,
    input,
    "submit employee permission change",
  );
}

export function createDigitalEmployeeRun(
  options: ApiClientOptions,
  employeeId: string,
  input: DigitalEmployeeRunInput,
): Promise<DigitalEmployeeRun> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return postJson<DigitalEmployeeRun>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs`,
    input,
    "create digital employee run",
  );
}

export function listDigitalEmployeeRuns(
  options: ApiClientOptions,
  employeeId: string,
  filter: ListDigitalEmployeeRunsFilter = {},
): Promise<DigitalEmployeeRunListResult> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const params = new URLSearchParams();
  if (filter.limit !== undefined) params.set("limit", String(filter.limit));
  if (filter.offset !== undefined) params.set("offset", String(filter.offset));
  if (filter.status?.length) params.set("status", filter.status.join(","));
  if (filter.project_id) params.set("project_id", filter.project_id);
  if (filter.from) params.set("from", filter.from);
  if (filter.to) params.set("to", filter.to);
  if (filter.run_kind) params.set("run_kind", filter.run_kind);
  if (filter.chat_thread_id) params.set("chat_thread_id", filter.chat_thread_id);
  const query = params.toString();

  return getJson<DigitalEmployeeRunListResult>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs${query ? `?${query}` : ""}`,
    "digital employee runs",
  );
}

export function getDigitalEmployeeRunStats(
  options: ApiClientOptions,
  employeeId: string,
): Promise<DigitalEmployeeRunStats> {
  return getJson<DigitalEmployeeRunStats>(
    options,
    `/api/v1/digital-employees/${encodePathSegment(employeeId)}/run-stats`,
    "digital employee run stats",
  );
}

export function getDigitalEmployeeRun(
  options: ApiClientOptions,
  employeeId: string,
  runId: string,
): Promise<DigitalEmployeeRun> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const encodedRunId = encodePathSegment(runId);

  return getJson<DigitalEmployeeRun>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs/${encodedRunId}`,
    "digital employee run",
  );
}

export function listDigitalEmployeeRunEvents(
  options: ApiClientOptions,
  employeeId: string,
  runId: string,
  pagination: RunPagination = {},
): Promise<DigitalEmployeeRunEvent[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const encodedRunId = encodePathSegment(runId);

  return getJson<DigitalEmployeeRunEvent[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs/${encodedRunId}/events${paginationQuery(pagination)}`,
    "digital employee run events",
  );
}

export function stopDigitalEmployeeRun(
  options: ApiClientOptions,
  employeeId: string,
  runId: string,
  input: StopDigitalEmployeeRunInput,
): Promise<DigitalEmployeeRun> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const encodedRunId = encodePathSegment(runId);

  return postJson<DigitalEmployeeRun>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs/${encodedRunId}/stop`,
    input,
    "stop digital employee run",
  );
}

export function acknowledgeDigitalEmployeeRunFailure(
  options: ApiClientOptions,
  employeeId: string,
  runId: string,
): Promise<DigitalEmployeeRun> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const encodedRunId = encodePathSegment(runId);

  return postJson<DigitalEmployeeRun>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs/${encodedRunId}/acknowledge-failure`,
    {},
    "acknowledge digital employee run failure",
  );
}

export function retryDigitalEmployeeRunFailure(
  options: ApiClientOptions,
  employeeId: string,
  runId: string,
): Promise<DigitalEmployeeRun> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const encodedRunId = encodePathSegment(runId);

  return postJson<DigitalEmployeeRun>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs/${encodedRunId}/retry`,
    {},
    "retry digital employee run failure",
  );
}
