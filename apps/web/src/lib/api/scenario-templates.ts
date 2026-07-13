import { type ApiClientOptions, getJson } from "./client";

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
  created_at: string;
  updated_at: string;
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

export function scenarioTemplateAcceptanceCriteria(
  template: ScenarioTemplate,
): string[] {
  const criteria = template.spec["default_acceptance_criteria"];
  return Array.isArray(criteria)
    ? criteria.filter((item): item is string => typeof item === "string")
    : [];
}
