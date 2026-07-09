import type { ApiClientOptions } from "./client";
import type { BudgetPolicy, CapabilityBindings } from "./employees";
import { deleteJson, getJson, patchJson, postJson } from "./client";

export type EmployeeTemplateStatus = "active" | "disabled";

export type EmployeeTemplate = {
  id: string;
  tenant_id: string;
  type: string;
  label: string;
  description: string;
  default_role: string;
  recommended_skills: string[];
  recommended_mcp_servers: string[];
  recommended_provider_types: string[];
  persona_memory_markdown?: string;
  capability_bindings?: CapabilityBindings;
  default_approval_policy: Record<string, unknown>;
  budget_policy?: BudgetPolicy;
  metadata: Record<string, unknown>;
  status: EmployeeTemplateStatus;
  is_system: boolean;
  created_at: string;
  updated_at: string;
};

export type CreateEmployeeTemplateInput = {
  type: string;
  label: string;
  description?: string;
  default_role?: string;
  recommended_skills?: string[];
  recommended_mcp_servers?: string[];
  recommended_provider_types?: string[];
  persona_memory_markdown?: string;
  capability_bindings?: CapabilityBindings;
  default_approval_policy?: Record<string, unknown>;
  budget_policy?: BudgetPolicy;
  metadata?: Record<string, unknown>;
};

export type UpdateEmployeeTemplateInput = Omit<CreateEmployeeTemplateInput, "type">;

export async function listEmployeeTemplates(
  options: ApiClientOptions,
): Promise<EmployeeTemplate[]> {
  return getJson<EmployeeTemplate[]>(options, "/api/v1/digital-employee-templates", "employee templates");
}

export async function getEmployeeTemplate(
  options: ApiClientOptions,
  templateId: string,
): Promise<EmployeeTemplate> {
  return getJson<EmployeeTemplate>(
    options,
    `/api/v1/digital-employee-templates/${templateId}`,
    "employee template",
  );
}

export async function createEmployeeTemplate(
  options: ApiClientOptions,
  input: CreateEmployeeTemplateInput,
): Promise<EmployeeTemplate> {
  return postJson<EmployeeTemplate>(
    options,
    "/api/v1/digital-employee-templates",
    input,
    "employee template",
  );
}

export async function updateEmployeeTemplate(
  options: ApiClientOptions,
  templateId: string,
  input: UpdateEmployeeTemplateInput,
): Promise<EmployeeTemplate> {
  return patchJson<EmployeeTemplate>(
    options,
    `/api/v1/digital-employee-templates/${templateId}`,
    input,
    "employee template",
  );
}

export async function setEmployeeTemplateStatus(
  options: ApiClientOptions,
  templateId: string,
  status: EmployeeTemplateStatus,
): Promise<EmployeeTemplate> {
  return patchJson<EmployeeTemplate>(
    options,
    `/api/v1/digital-employee-templates/${templateId}/status`,
    { status },
    "employee template",
  );
}

export async function deleteEmployeeTemplate(
  options: ApiClientOptions,
  templateId: string,
): Promise<void> {
  await deleteJson(options, `/api/v1/digital-employee-templates/${templateId}`, "employee template");
}
