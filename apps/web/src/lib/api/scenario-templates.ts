import {
  type ApiClientOptions,
  getJson,
  patchJson,
  postJson,
} from "./client";

export type ScenarioTemplateRole = {
  key?: string;
  title?: string;
  required_capabilities?: string[];
  collapsible_with?: string[];
  independent_from?: string[];
};

export type ScenarioTemplateSkeletonStep = {
  step?: string;
  role?: string;
  depends_on?: string[];
  required_inputs_defaults?: string[];
  produces_defaults?: { name?: string; kind?: string }[];
};

export type ScenarioTemplate = {
  id: string;
  tenant_id: string;
  template_key: string;
  name: string;
  description: string;
  spec: Record<string, unknown>;
  status: string;
  active_version?: number;
  created_at: string;
  updated_at: string;
};

export type ScenarioTemplateVersion = {
  id: string;
  template_id: string;
  version: number;
  spec: Record<string, unknown>;
  created_at: string;
};

export type CreateScenarioTemplateInput = {
  template_key: string;
  name: string;
  description?: string;
  spec: Record<string, unknown>;
};

export type CreateScenarioTemplateVersionInput = {
  spec: Record<string, unknown>;
};

export type PatchScenarioTemplateInput = {
  status?: "active" | "disabled";
  name?: string;
  description?: string;
};

export function listScenarioTemplates(
  options: ApiClientOptions,
): Promise<ScenarioTemplate[]> {
  return getJson<ScenarioTemplate[]>(
    options,
    "/api/v1/scenario-templates",
    "scenario templates",
  );
}

export function createScenarioTemplate(
  options: ApiClientOptions,
  input: CreateScenarioTemplateInput,
): Promise<ScenarioTemplate> {
  return postJson<ScenarioTemplate>(
    options,
    "/api/v1/scenario-templates",
    input,
    "create scenario template",
  );
}

export function createScenarioTemplateVersion(
  options: ApiClientOptions,
  templateKey: string,
  input: CreateScenarioTemplateVersionInput,
): Promise<ScenarioTemplate> {
  return postJson<ScenarioTemplate>(
    options,
    `/api/v1/scenario-templates/${encodeURIComponent(templateKey)}/versions`,
    input,
    "create scenario template version",
  );
}

export function listScenarioTemplateVersions(
  options: ApiClientOptions,
  templateKey: string,
): Promise<ScenarioTemplateVersion[]> {
  return getJson<ScenarioTemplateVersion[]>(
    options,
    `/api/v1/scenario-templates/${encodeURIComponent(templateKey)}/versions`,
    "scenario template versions",
  );
}

export function patchScenarioTemplate(
  options: ApiClientOptions,
  templateKey: string,
  input: PatchScenarioTemplateInput,
): Promise<ScenarioTemplate> {
  return patchJson<ScenarioTemplate>(
    options,
    `/api/v1/scenario-templates/${encodeURIComponent(templateKey)}`,
    input,
    "patch scenario template",
  );
}

export function scenarioTemplateRoles(
  template: ScenarioTemplate,
): ScenarioTemplateRole[] {
  const roles = template.spec["roles"];
  return Array.isArray(roles) ? (roles as ScenarioTemplateRole[]) : [];
}

export function scenarioTemplateSkeleton(
  template: ScenarioTemplate,
): ScenarioTemplateSkeletonStep[] {
  const skeleton = template.spec["skeleton"];
  return Array.isArray(skeleton)
    ? (skeleton as ScenarioTemplateSkeletonStep[])
    : [];
}

export type AcceptanceCriterion =
  | string
  | { statement: string; applies_from_exit?: string };

export function scenarioTemplateAcceptanceCriteria(
  template: ScenarioTemplate,
): AcceptanceCriterion[] {
  const criteria = template.spec["default_acceptance_criteria"];
  return Array.isArray(criteria)
    ? criteria.filter(
        (item): item is AcceptanceCriterion =>
          typeof item === "string" ||
          (typeof item === "object" &&
            item !== null &&
            "statement" in item &&
            typeof item.statement === "string"),
      )
    : [];
}

export function scenarioTemplateExits(
  template: ScenarioTemplate,
): Array<{ deliverable: string; label: string }> {
  const exits = template.spec["exits"];
  return Array.isArray(exits)
    ? exits.filter(
        (item): item is { deliverable: string; label: string } =>
          typeof item === "object" &&
          item !== null &&
          "deliverable" in item &&
          "label" in item &&
          typeof item.deliverable === "string" &&
          typeof item.label === "string",
      )
    : [];
}
