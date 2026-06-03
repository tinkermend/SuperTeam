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
  const fetcher = options.fetcher ?? fetch;
  const encodedEmployeeId = encodeURIComponent(employeeId);
  const response = await fetcher(
    buildApiUrl(options.baseUrl, `/api/v1/digital-employees/${encodedEmployeeId}/execution-instance`),
    {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    },
  );

  return parseJson<DigitalEmployeeExecutionInstance>(response, "digital employee execution instance");
}

export function createDigitalEmployeeConfigRevision(
  options: ApiClientOptions,
  employeeId: string,
  input: CreateDigitalEmployeeConfigRevisionInput,
): Promise<DigitalEmployeeConfigRevision> {
  const encodedEmployeeId = encodeURIComponent(employeeId);
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
  const encodedEmployeeId = encodeURIComponent(employeeId);
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
  const encodedEmployeeId = encodeURIComponent(employeeId);
  return postJson<DigitalEmployeeEffectiveConfig>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/effective-configs/approve`,
    input,
    "approve digital employee effective config",
  );
}
