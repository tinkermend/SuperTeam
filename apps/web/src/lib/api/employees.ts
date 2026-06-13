import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type DigitalEmployeeStatus =
  | "draft"
  | "ready"
  | "active"
  | "disabled"
  | "error";

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
  team_id: string;
  owner_user_id: string;
  employee_type: string;
  name: string;
  role: string;
  description?: string;
  status: DigitalEmployeeStatus;
  permission_policy: Record<string, unknown>;
  context_policy: Record<string, unknown>;
  approval_policy: Record<string, unknown>;
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
  created_at?: string;
  updated_at?: string;
};

export type DigitalEmployeeTypeOption = {
  type: string;
  label: string;
  description: string;
  default_role: string;
  recommended_skills?: string[];
  recommended_mcp_servers?: string[];
  recommended_provider_types?: string[];
  default_capability_selection?: Record<string, unknown>;
  default_context_policy_override?: Record<string, unknown>;
  default_approval_policy?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
};

export type DigitalEmployeeCapabilityOptions = {
  provider_types: string[];
  skills: string[];
  mcp_servers: string[];
  external_capabilities: string[];
};

export type DigitalEmployeeRuntimeProviderOption = {
  runtime_node_id: string;
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
  context_policy_override: Record<string, unknown>;
  approval_policy: Record<string, unknown>;
  capability_selection: Record<string, unknown>;
  runtime_selector: Record<string, unknown>;
  workspace_policy: Record<string, unknown>;
  session_policy: Record<string, unknown>;
  metadata: Record<string, unknown>;
};

export type DigitalEmployeeCreateOptions = {
  team_config: {
    id: string;
    tenant_id: string;
    team_id: string;
    revision_number: number;
    status: string;
    allowed_employee_types: string[];
    allowed_provider_types: string[];
    allowed_skills: string[];
    allowed_mcp_servers: string[];
    allowed_external_capabilities: string[];
    capability_policy: Record<string, unknown>;
    context_policy: Record<string, unknown>;
    approval_policy: Record<string, unknown>;
    artifact_contract: Record<string, unknown>;
    internal_collaboration_policy: Record<string, unknown>;
    runtime_scope_policy: Record<string, unknown>;
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
  runtime_node_id: string;
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
  team_id: string;
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
  role_profile?: Record<string, unknown>;
  constitution_addendum?: Record<string, unknown>;
  capability_selection?: Record<string, unknown>;
  context_policy_override?: Record<string, unknown>;
  approval_policy_override?: Record<string, unknown>;
  output_contract_addendum?: Record<string, unknown>;
  runtime_node_id: string;
  provider_type: string;
  session_policy?: Record<string, unknown>;
  workspace_policy?: Record<string, unknown>;
  budget_policy?: BudgetPolicy;
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
    !("runtime_node_id" in input) ||
    !input.runtime_node_id ||
    !("provider_type" in input) ||
    !input.provider_type
  ) {
    throw new Error(
      "digital employee ready creation requires employee_type, avatar_asset_id, runtime_node_id, and provider_type",
    );
  }
}

export type ListDigitalEmployeesFilters = {
  team_id?: string;
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
  role_profile: Record<string, unknown>;
  constitution_addendum: Record<string, unknown>;
  capability_selection: Record<string, unknown>;
  context_policy_override: Record<string, unknown>;
  approval_policy_override: Record<string, unknown>;
  output_contract_addendum: Record<string, unknown>;
  budget_policy: BudgetPolicy;
  status: "draft";
  approved_by?: string;
  approved_at?: string;
  archived_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateDigitalEmployeeConfigRevisionInput = {
  role_profile?: Record<string, unknown>;
  constitution_addendum?: Record<string, unknown>;
  capability_selection?: Record<string, unknown>;
  context_policy_override?: Record<string, unknown>;
  approval_policy_override?: Record<string, unknown>;
  output_contract_addendum?: Record<string, unknown>;
  budget_policy?: BudgetPolicy;
  status?: "draft";
};

export type ConfigRevisionRef = {
  id: string;
};

export type EffectiveConfigValidationIssue = {
  code: string;
  message: string;
  path?: string;
};

export type EffectiveConfigValidation = {
  blocking_errors: EffectiveConfigValidationIssue[];
  warnings: EffectiveConfigValidationIssue[];
};

export type EffectiveConfigPreview = {
  team_config_revision_id: string;
  employee_config_revision_id: string;
  effective_config: Record<string, unknown>;
  validation: EffectiveConfigValidation;
};

export type EffectiveConfigPreviewInput = {
  team_config: ConfigRevisionRef;
  employee_config: ConfigRevisionRef;
};

export type DigitalEmployeeEffectiveConfig = {
  id: string;
  tenant_id: string;
  digital_employee_id: string;
  team_config_revision_id: string;
  employee_config_revision_id: string;
  effective_config: Record<string, unknown>;
  validation_result: EffectiveConfigValidation;
  status: "pending_approval" | "approved" | "revoked";
  approved_by?: string;
  approved_at?: string;
  revoked_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type ApproveEffectiveConfigInput = {
  preview: EffectiveConfigPreviewInput;
};

export type WorkspaceFile = {
  id: string;
  team_id: string;
  path: string;
  file_role: string;
  mime_type: string;
  sync_policy: string;
  status: string;
  current_revision_id: string;
  revision_number: number;
  content: string;
  content_hash: string;
  size_bytes: number;
  storage_backend: string;
  object_key?: string;
  change_note?: string;
  created_at?: string;
  updated_at?: string;
};

export type UpsertWorkspaceFileInput = {
  path: string;
  content: string;
  file_role?: string;
  mime_type?: string;
  sync_policy?: string;
  change_note?: string;
};

async function postJson<T>(
  options: ApiClientOptions,
  path: string,
  input: unknown,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  return parseJson<T>(response, resource);
}

async function putJson<T>(
  options: ApiClientOptions,
  path: string,
  input: unknown,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "PUT",
  });

  return parseJson<T>(response, resource);
}

async function getJson<T>(
  options: ApiClientOptions,
  path: string,
  resource: string,
): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<T>(response, resource);
}

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
  const fetcher = options.fetcher ?? fetch;
  const searchParams = new URLSearchParams();
  if (filters.team_id) {
    searchParams.set("team_id", filters.team_id);
  }
  const query = searchParams.toString();
  const path = `/api/v1/digital-employees${query ? `?${query}` : ""}`;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<DigitalEmployee[]>(response, "digital employees");
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
  teamId: string,
): Promise<DigitalEmployeeCreateOptions> {
  const searchParams = new URLSearchParams();
  searchParams.set("team_id", teamId);

  return getJson<DigitalEmployeeCreateOptions>(
    options,
    `/api/v1/digital-employees/create-options?${searchParams.toString()}`,
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

export function previewDigitalEmployeeEffectiveConfig(
  options: ApiClientOptions,
  employeeId: string,
  input: EffectiveConfigPreviewInput,
): Promise<EffectiveConfigPreview> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  return postJson<EffectiveConfigPreview>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/effective-configs/preview`,
    input,
    "preview digital employee effective config",
  );
}

export function approveDigitalEmployeeEffectiveConfig(
  options: ApiClientOptions,
  employeeId: string,
  input: ApproveEffectiveConfigInput,
): Promise<DigitalEmployeeEffectiveConfig> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  return postJson<DigitalEmployeeEffectiveConfig>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/effective-configs/approve`,
    input,
    "approve digital employee effective config",
  );
}

export function listWorkspaceFiles(
  options: ApiClientOptions,
  employeeId: string,
): Promise<WorkspaceFile[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<WorkspaceFile[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/workspace-files`,
    "employee workspace files",
  );
}

export function upsertWorkspaceFile(
  options: ApiClientOptions,
  employeeId: string,
  input: UpsertWorkspaceFileInput,
): Promise<WorkspaceFile> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return putJson<WorkspaceFile>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/workspace-files`,
    input,
    "upsert employee workspace file",
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
  pagination: RunPagination = {},
): Promise<DigitalEmployeeRun[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<DigitalEmployeeRun[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs${paginationQuery(pagination)}`,
    "digital employee runs",
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
