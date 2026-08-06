import {
  type ApiClientOptions,
  getJson,
  patchJson,
  postJson,
  putJson,
} from "@/lib/api/client";

export type ProjectPlaybookCasting = {
  id: string;
  tenant_id: string;
  project_id: string;
  scenario_template_key: string;
  role_key: string;
  digital_employee_id: string;
  cast_by_user_id: string;
  created_at: string;
  updated_at: string;
};

export type CastingAssignment = {
  role_key: string;
  digital_employee_id: string;
};

export type RoleCandidate = {
  digital_employee_id: string;
  name: string;
  team_id?: string;
  team_name: string;
  role_keys: string[];
  matched_capabilities: string[];
  missing_capabilities: string[];
  capability_fit: "matched" | "partial" | "missing";
};

export type PlaybookExitReach = {
  deliverable: string;
  label: string;
  reachable: boolean;
  required_roles: string[];
  missing_roles: string[];
};

export type PlaybookReadiness = {
  scenario_template_key: string;
  template_name: string;
  runnable: boolean;
  deepest_exit: PlaybookExitReach | null;
  next_exit_needs_roles: string[];
  missing_roles_for_any: string[];
  exits: PlaybookExitReach[];
};

export type RoleVocabularyEntry = {
  id: string;
  tenant_id: string;
  role_key: string;
  title: string;
  description: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export function listRoleVocabulary(
  options: ApiClientOptions,
): Promise<RoleVocabularyEntry[]> {
  return getJson<RoleVocabularyEntry[]>(
    options,
    "/api/v1/role-vocabulary",
    "role vocabulary",
  );
}

export type CreateRoleVocabularyInput = {
  role_key: string;
  title: string;
  description?: string;
  status?: "active" | "disabled";
};

export type PatchRoleVocabularyInput = {
  title?: string;
  description?: string;
  status?: "active" | "disabled";
  confirm_impact?: boolean;
};

export type RoleVocabularyTemplateRef = {
  key: string;
  name: string;
};

export type RoleVocabularyEmployeeRef = {
  id: string;
  name: string;
};

export type RoleVocabularyReferences = {
  scenario_templates: RoleVocabularyTemplateRef[];
  employees: RoleVocabularyEmployeeRef[];
  employee_count: number;
  casting_count: number;
};

export type ScenarioTemplateRoleViewRole = {
  role_key: string;
  title: string;
  required_capabilities: string[];
  holder_count: number;
};

export type ScenarioTemplateRoleIndependencePair = {
  roles: string[];
};

export type ScenarioTemplateRoleViewExit = {
  deliverable: string;
  label: string;
  required_roles: string[];
  role_independence_pairs: ScenarioTemplateRoleIndependencePair[];
};

export type ScenarioTemplateRoleView = {
  template_key: string;
  name: string;
  roles: ScenarioTemplateRoleViewRole[];
  exits: ScenarioTemplateRoleViewExit[];
};

export function createRoleVocabulary(
  options: ApiClientOptions,
  input: CreateRoleVocabularyInput,
): Promise<RoleVocabularyEntry> {
  return postJson<RoleVocabularyEntry>(
    options,
    "/api/v1/role-vocabulary",
    input,
    "create role vocabulary",
  );
}

export function patchRoleVocabulary(
  options: ApiClientOptions,
  roleKey: string,
  input: PatchRoleVocabularyInput,
): Promise<RoleVocabularyEntry> {
  return patchJson<RoleVocabularyEntry>(
    options,
    `/api/v1/role-vocabulary/${encodeURIComponent(roleKey)}`,
    input,
    "patch role vocabulary",
  );
}

export function getRoleVocabularyReferences(
  options: ApiClientOptions,
  roleKey: string,
): Promise<RoleVocabularyReferences> {
  return getJson<RoleVocabularyReferences>(
    options,
    `/api/v1/role-vocabulary/${encodeURIComponent(roleKey)}/references`,
    "role vocabulary references",
  );
}

export function getScenarioTemplateRoleView(
  options: ApiClientOptions,
  templateKey: string,
): Promise<ScenarioTemplateRoleView> {
  return getJson<ScenarioTemplateRoleView>(
    options,
    `/api/v1/scenario-templates/${encodeURIComponent(templateKey)}/role-view`,
    "scenario template role view",
  );
}

export function listProjectCastings(
  options: ApiClientOptions,
  projectId: string,
  templateKey?: string,
): Promise<ProjectPlaybookCasting[]> {
  const q = templateKey
    ? `?template_key=${encodeURIComponent(templateKey)}`
    : "";
  return getJson<ProjectPlaybookCasting[]>(
    options,
    `/api/v1/projects/${projectId}/castings${q}`,
    "project castings",
  );
}

export function putProjectCastings(
  options: ApiClientOptions,
  projectId: string,
  input: { scenario_template_key: string; assignments: CastingAssignment[] },
): Promise<ProjectPlaybookCasting[]> {
  return putJson<ProjectPlaybookCasting[]>(
    options,
    `/api/v1/projects/${projectId}/castings`,
    input,
    "put project castings",
  );
}

export function listRoleCandidates(
  options: ApiClientOptions,
  projectId: string,
  roleKey: string,
  requiredCapabilities: string[] = [],
): Promise<RoleCandidate[]> {
  const params = new URLSearchParams({ role_key: roleKey });
  if (requiredCapabilities.length > 0) {
    params.set("required_capabilities", requiredCapabilities.join(","));
  }
  return getJson<RoleCandidate[]>(
    options,
    `/api/v1/projects/${projectId}/role-candidates?${params}`,
    "role candidates",
  );
}

export function getPlaybookReadiness(
  options: ApiClientOptions,
  projectId: string,
  templateKey?: string,
): Promise<PlaybookReadiness[]> {
  const q = templateKey
    ? `?template_key=${encodeURIComponent(templateKey)}`
    : "";
  return getJson<PlaybookReadiness[]>(
    options,
    `/api/v1/projects/${projectId}/playbook-readiness${q}`,
    "playbook readiness",
  );
}

export type RequestCastingExpansionInput = {
  demand_id: string;
  suggested_role_key?: string;
  needs_external_role?: boolean;
  reason?: string;
  scenario_template_key?: string;
};

export type CastingExpansionDecision = {
  id: string;
  project_id: string;
  decision_type: string;
  status_snapshot: string;
  title_snapshot?: string;
  summary_snapshot?: string;
};

export function requestCastingExpansion(
  options: ApiClientOptions,
  projectId: string,
  input: RequestCastingExpansionInput,
): Promise<CastingExpansionDecision> {
  return postJson<CastingExpansionDecision>(
    options,
    `/api/v1/projects/${projectId}/casting-expansions`,
    input,
    "request casting expansion",
  );
}
