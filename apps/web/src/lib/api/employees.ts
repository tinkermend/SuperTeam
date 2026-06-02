import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type DigitalEmployeeStatus = "draft" | "ready" | "active" | "disabled" | "error";

export type DigitalEmployee = {
  id: string;
  name: string;
  role: string;
  description?: string;
  status: DigitalEmployeeStatus;
  permission_policy?: Record<string, unknown>;
  context_policy?: Record<string, unknown>;
  approval_policy?: Record<string, unknown>;
  risk_level?: string;
  metadata?: Record<string, unknown>;
  execution_instance?: {
    id: string;
    runtime_node_id: string;
    provider_type: string;
    agent_home_dir?: string;
    session_policy?: Record<string, unknown>;
    status: string;
  };
  created_at?: string;
  updated_at?: string;
};

export type CreateDigitalEmployeeInput = {
  name: string;
  role: string;
  description?: string;
};

export async function listDigitalEmployees(options: ApiClientOptions): Promise<DigitalEmployee[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/digital-employees"), {
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
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/digital-employees"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  return parseJson<DigitalEmployee>(response, "create digital employee");
}
