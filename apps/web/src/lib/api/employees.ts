import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type DigitalEmployeeStatus = "draft" | "ready" | "active" | "disabled" | "error";

export type DigitalEmployee = {
  id: string;
  tenant_id?: string;
  team_id?: string;
  name: string;
  role: string;
  description?: string;
  status: DigitalEmployeeStatus;
  permission_policy?: Record<string, unknown>;
  context_policy?: Record<string, unknown>;
  approval_policy?: Record<string, unknown>;
  risk_level?: string;
  metadata?: Record<string, unknown> & {
    effective_config_label?: string;
    effective_config_status?: "approved" | "draft" | "stale" | "missing" | string;
  };
  created_at?: string;
  updated_at?: string;
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

export type RunPagination = {
  limit?: number;
  offset?: number;
};

export type StopDigitalEmployeeRunInput = {
  reason: string;
};

export type CreateDigitalEmployeeInput = {
  team_id: string;
  name: string;
  role: string;
  description?: string;
};

export type ListDigitalEmployeesFilters = {
  team_id?: string;
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

async function postJson<T>(options: ApiClientOptions, path: string, input: unknown, resource: string): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  return parseJson<T>(response, resource);
}

async function getJson<T>(options: ApiClientOptions, path: string, resource: string): Promise<T> {
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

export function getDigitalEmployee(options: ApiClientOptions, employeeId: string): Promise<DigitalEmployee> {
  return getJson<DigitalEmployee>(
    options,
    `/api/v1/digital-employees/${encodePathSegment(employeeId)}`,
    "digital employee",
  );
}

export async function createDigitalEmployee(
  options: ApiClientOptions,
  input: CreateDigitalEmployeeInput,
): Promise<DigitalEmployee> {
  return postJson<DigitalEmployee>(options, "/api/v1/digital-employees", input, "create digital employee");
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
