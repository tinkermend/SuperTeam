import type { ApiClientOptions } from "./client";
import { deleteJson, getJson, postJson, putJson } from "./client";

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
  context_policy: Record<string, unknown>;
  approval_policy: Record<string, unknown>;
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

export type DigitalEmployeeCapabilityOptions = {
  provider_types: string[];
  skills: string[];
  mcp_servers: string[];
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

export type DigitalEmployeeExecutionInstance = {
  id: string;
  digital_employee_id: string;
  runtime_node_id?: string;
  provider_type: string;
  agent_home_dir?: string;
  workspace_policy?: Record<string, unknown>;
  session_policy?: Record<string, unknown>;
  runtime_selector?: Record<string, unknown>;
  capacity_requirements?: Record<string, unknown>;
  fallback_policy?: Record<string, unknown>;
  status: string;
  created_at?: string;
  updated_at?: string;
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
  started_at?: string;
  completed_at?: string;
  finished_at?: string;
  created_at?: string;
  updated_at?: string;
  run_kind: DigitalEmployeeRunKind;
  resume_of_run_id?: string;
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
  | "pending_binding"
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
    pending_runtime_binding_count: number;
    pending_config_approval_count: number;
    failed_recent_run_count: number;
    operational_status_counts: Partial<Record<DigitalEmployeeOperationalStatus, number>>;
  };
  queue_summary: {
    pending_runtime_binding_count: number;
    stale_config_count: number;
    failed_recent_run_count: number;
  };
  items: DigitalEmployeeOverviewItem[];
  filters: {
    teams: OverviewFilterOption[];
    employee_types: OverviewFilterOption[];
    statuses: OverviewFilterOption[];
    providers: OverviewFilterOption[];
    runtime_nodes: OverviewFilterOption[];
    risk_levels: OverviewFilterOption[];
    execution_statuses: OverviewFilterOption[];
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
  identity_summary: {
    id: string;
    tenant_id: string;
    team_id?: string;
    team_name: string;
    owner_user_id: string;
    owner_display_name: string;
    employee_type: string;
    employee_type_label: string;
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
  context_policy?: Record<string, unknown>;
  approval_policy?: Record<string, unknown>;
  risk_level?: string;
  metadata?: Record<string, unknown>;
  persona_memory_markdown?: string;
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
  runtime_node_id?: string;
  risk_level?: string;
  execution_status?: DigitalEmployeeOverviewExecutionStatus;
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

export async function getDigitalEmployeeExecutionInstance(
  options: ApiClientOptions,
  employeeId: string,
): Promise<DigitalEmployeeExecutionInstance> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<DigitalEmployeeExecutionInstance>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/execution-instance`,
    "digital employee execution instance",
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
