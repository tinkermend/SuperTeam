import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type PromptTemplateScope = "GLOBAL" | "TENANT" | "TEAM" | "PERSONAL";

export interface PromptTemplateVariable {
  name: string;
  label?: string;
  description?: string;
  type: "string" | "text" | "select";
  required: boolean;
  default?: string;
  options?: string[];
}

export type PromptTemplate = {
  id: string;
  tenant_id: string;
  title: string;
  content: string;
  category_code: string;
  scope: PromptTemplateScope;
  team_id?: string;
  creator_id: string;
  variables?: PromptTemplateVariable[];
  use_count: number;
  created_at: string;
  updated_at: string;
};

export async function listPromptTemplates(
  options: ApiClientOptions,
): Promise<PromptTemplate[]> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/templates"), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });

  return parseJson<PromptTemplate[]>(response, "templates");
}

export async function applyPromptTemplate(
  options: ApiClientOptions,
  id: string,
): Promise<void> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/templates/${id}/apply`), {
    credentials: "include",
    method: "POST",
  });

  if (!response.ok) {
    // we just need it to not crash.
    throw new Error(`Failed to apply prompt template: ${response.statusText}`);
  }
}
