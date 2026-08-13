import type { ApiClientOptions } from "./client";
import { deleteJson, getJson, patchJson, postJson, postJsonWithoutBody } from "./client";

export type ExternalIntegrationAutonomyTier = "pause_at_gate" | "full_auto";
export type ExternalIntegrationStatus = "active" | "disabled";

export type ExternalIntegration = {
  id: string;
  tenant_id: string;
  project_id: string;
  digital_employee_id: string;
  name: string;
  description: string;
  allow_chat_run: boolean;
  allow_demand_submit: boolean;
  skill_ids: string[];
  scenario_template_key?: string | null;
  autonomy_tier: ExternalIntegrationAutonomyTier;
  max_calls_per_hour: number;
  status: ExternalIntegrationStatus;
  created_by_user_id: string;
  created_at: string;
  updated_at: string;
};

export type ExternalIntegrationToken = {
  id: string;
  integration_id: string;
  status: "active" | "revoked";
  created_at: string;
  last_used_at?: string | null;
  revoked_at?: string | null;
};

export type CreateExternalIntegrationInput = {
  project_id: string;
  digital_employee_id: string;
  name: string;
  description?: string;
  allow_chat_run?: boolean;
  allow_demand_submit?: boolean;
  skill_ids?: string[];
  scenario_template_key?: string;
  autonomy_tier?: ExternalIntegrationAutonomyTier;
  max_calls_per_hour?: number;
};

export type UpdateExternalIntegrationInput = {
  name?: string;
  description?: string;
  allow_chat_run?: boolean;
  allow_demand_submit?: boolean;
  skill_ids?: string[];
  scenario_template_key?: string | null;
  autonomy_tier?: ExternalIntegrationAutonomyTier;
  max_calls_per_hour?: number;
  status?: ExternalIntegrationStatus;
};

export function listExternalIntegrations(
  options: ApiClientOptions,
  params?: { project_id?: string },
): Promise<{ integrations: ExternalIntegration[] }> {
  const search = new URLSearchParams();
  if (params?.project_id) search.set("project_id", params.project_id);
  const qs = search.toString();
  return getJson(
    options,
    `/api/v1/external-integrations${qs ? `?${qs}` : ""}`,
    "external integrations",
  );
}

export function createExternalIntegration(
  options: ApiClientOptions,
  input: CreateExternalIntegrationInput,
): Promise<ExternalIntegration> {
  return postJson(options, "/api/v1/external-integrations", input, "external integration");
}

export function updateExternalIntegration(
  options: ApiClientOptions,
  integrationId: string,
  input: UpdateExternalIntegrationInput,
): Promise<ExternalIntegration> {
  return patchJson(
    options,
    `/api/v1/external-integrations/${encodeURIComponent(integrationId)}`,
    input,
    "external integration",
  );
}

export function listExternalIntegrationTokens(
  options: ApiClientOptions,
  integrationId: string,
): Promise<{ tokens: ExternalIntegrationToken[] }> {
  return getJson(
    options,
    `/api/v1/external-integrations/${encodeURIComponent(integrationId)}/tokens`,
    "integration tokens",
  );
}

export function issueExternalIntegrationToken(
  options: ApiClientOptions,
  integrationId: string,
): Promise<{ id: string; integration_id: string; token: string }> {
  return postJsonWithoutBody(
    options,
    `/api/v1/external-integrations/${encodeURIComponent(integrationId)}/tokens`,
    "integration token",
  );
}

export function revokeExternalIntegrationToken(
  options: ApiClientOptions,
  integrationId: string,
  tokenId: string,
): Promise<void> {
  return deleteJson(
    options,
    `/api/v1/external-integrations/${encodeURIComponent(integrationId)}/tokens/${encodeURIComponent(tokenId)}`,
    "integration token",
  );
}
