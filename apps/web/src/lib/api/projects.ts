import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type ProjectStatus =
  | "draft"
  | "configuring"
  | "running"
  | "paused"
  | "acceptance"
  | "archived";

export type ProjectPrincipalType = "human_user" | "digital_employee" | "team";
export type ProjectRole =
  | "owner"
  | "leader"
  | "acceptance"
  | "executor"
  | "reviewer"
  | "observer";
export type ProjectEventType =
  | "project.created"
  | "project.config.changed"
  | "project.archived"
  | "demand.submitted";
export type ProjectDemandSourceType =
  | "manual"
  | "github"
  | "ticket"
  | "document"
  | "log";
export type ProjectDemandStatus =
  | "submitted"
  | "recorded"
  | "planning_pending"
  | "cancelled";

export type Project = {
  id: string;
  tenant_id: string;
  team_id?: string;
  name: string;
  description?: string;
  goal: string;
  status: ProjectStatus;
  human_owner_user_id: string;
  leader_user_id?: string;
  acceptance_user_id?: string;
  coordination_workflow_id: string;
  coordination_status: string;
  coordination_policy: Record<string, unknown>;
  approval_policy: Record<string, unknown>;
  evidence_policy: Record<string, unknown>;
  archived_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type ProjectMember = {
  id: string;
  tenant_id: string;
  project_id: string;
  principal_type: ProjectPrincipalType;
  principal_id: string;
  project_role: ProjectRole;
  display_name_snapshot?: string;
  status: string;
  settings: Record<string, unknown>;
};

export type ProjectMemberInput = {
  principal_type: ProjectPrincipalType;
  principal_id: string;
  project_role: ProjectRole;
  display_name_snapshot?: string;
  settings?: Record<string, unknown>;
};

export type ProjectTask = {
  id: string;
  tenant_id: string;
  project_id: string;
  demand_id?: string;
  title: string;
  summary?: string;
  status: string;
  assigned_digital_employee_id?: string;
  risk_level?: string;
  requires_human_approval: boolean;
};

export type ProjectEvent = {
  id: string;
  tenant_id: string;
  project_id: string;
  sequence_number: number;
  event_type: ProjectEventType;
  actor_type: string;
  actor_id: string;
  resource_type?: string;
  resource_id?: string;
  summary?: string;
  payload: Record<string, unknown>;
};

export type ProjectDemand = {
  id: string;
  tenant_id: string;
  project_id: string;
  submitted_by_user_id: string;
  title: string;
  content?: string;
  source_type: ProjectDemandSourceType;
  source_refs: Record<string, unknown>;
  attachments: unknown[];
  status: ProjectDemandStatus;
  created_event_id?: string;
};

export type ProjectStatusSummary = {
  current_phase: string;
  is_archived: boolean;
};

export type ProjectTaskSummary = {
  active_tasks: number;
  pending_human_tasks: number;
  completed_tasks: number;
  failed_tasks: number;
};

export type ProjectCoordinationWorkflow = {
  workflow_id: string;
  status: string;
};

export type ProjectOverview = {
  project: Project;
  human_roles: ProjectMember[];
  digital_employee_pool: ProjectMember[];
  status_summary: ProjectStatusSummary;
  task_summary: ProjectTaskSummary;
  active_tasks: ProjectTask[];
  recent_events: ProjectEvent[];
  coordination_workflow: ProjectCoordinationWorkflow;
};

export type ProjectConfig = {
  project: Project;
  human_roles: ProjectMember[];
  digital_employee_pool: ProjectMember[];
  members: ProjectMember[];
  coordination_policy: Record<string, unknown>;
  approval_policy: Record<string, unknown>;
  evidence_policy: Record<string, unknown>;
  coordination_workflow: ProjectCoordinationWorkflow;
};

export type CreateProjectInput = {
  team_id?: string;
  name: string;
  description?: string;
  goal: string;
  human_owner_user_id: string;
  leader_user_id?: string;
  acceptance_user_id?: string;
  members?: ProjectMemberInput[];
  coordination_policy?: Record<string, unknown>;
  approval_policy?: Record<string, unknown>;
  evidence_policy?: Record<string, unknown>;
};

export type CreateProjectResponse = {
  project: Project;
  members: ProjectMember[];
};

export type UpdateProjectConfigInput = {
  name?: string;
  description?: string;
  goal?: string;
  human_owner_user_id?: string;
  leader_user_id?: string;
  acceptance_user_id?: string;
  members?: ProjectMemberInput[];
  coordination_policy?: Record<string, unknown>;
  approval_policy?: Record<string, unknown>;
  evidence_policy?: Record<string, unknown>;
};

export type SubmitProjectDemandInput = {
  title: string;
  content?: string;
  source_type?: ProjectDemandSourceType;
  source_refs?: Record<string, unknown>;
  attachments?: unknown[];
};

export type ListProjectsFilters = {
  status?: ProjectStatus;
  q?: string;
  limit?: number;
  offset?: number;
};

export type ListProjectTasksFilters = {
  status?: string;
  limit?: number;
  offset?: number;
};

export type PaginationFilters = {
  limit?: number;
  offset?: number;
};

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

async function patchJson<T>(
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
    method: "PATCH",
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

function projectPath(projectId: string, suffix = ""): string {
  return `/api/v1/projects/${encodeURIComponent(projectId)}${suffix}`;
}

function projectListPath(filters: ListProjectsFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.status) {
    params.set("status", filters.status);
  }
  const q = filters.q?.trim();
  if (q) {
    params.set("q", q);
  }
  if (filters.limit !== undefined) {
    params.set("limit", String(filters.limit));
  }
  if (filters.offset !== undefined) {
    params.set("offset", String(filters.offset));
  }
  const query = params.toString();
  return query ? `/api/v1/projects?${query}` : "/api/v1/projects";
}

function paginationQuery(filters: PaginationFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.limit !== undefined) {
    params.set("limit", String(filters.limit));
  }
  if (filters.offset !== undefined) {
    params.set("offset", String(filters.offset));
  }
  const query = params.toString();
  return query ? `?${query}` : "";
}

function taskQuery(filters: ListProjectTasksFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.status) {
    params.set("status", filters.status);
  }
  if (filters.limit !== undefined) {
    params.set("limit", String(filters.limit));
  }
  if (filters.offset !== undefined) {
    params.set("offset", String(filters.offset));
  }
  const query = params.toString();
  return query ? `?${query}` : "";
}

export function listProjects(
  options: ApiClientOptions,
  filters: ListProjectsFilters = {},
): Promise<Project[]> {
  return getJson<Project[]>(options, projectListPath(filters), "projects");
}

export function createProject(
  options: ApiClientOptions,
  input: CreateProjectInput,
): Promise<CreateProjectResponse> {
  return postJson<CreateProjectResponse>(
    options,
    "/api/v1/projects",
    input,
    "create project",
  );
}

export function getProject(
  options: ApiClientOptions,
  projectId: string,
): Promise<Project> {
  return getJson<Project>(options, projectPath(projectId), "project");
}

export function updateProject(
  options: ApiClientOptions,
  projectId: string,
  input: UpdateProjectConfigInput,
): Promise<Project> {
  return patchJson<Project>(
    options,
    projectPath(projectId),
    input,
    "update project",
  );
}

export function archiveProject(
  options: ApiClientOptions,
  projectId: string,
): Promise<Project> {
  return postJson<Project>(
    options,
    projectPath(projectId, "/archive"),
    {},
    "archive project",
  );
}

export function getProjectOverview(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectOverview> {
  return getJson<ProjectOverview>(
    options,
    projectPath(projectId, "/overview"),
    "project overview",
  );
}

export function getProjectConfig(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectConfig> {
  return getJson<ProjectConfig>(
    options,
    projectPath(projectId, "/config"),
    "project config",
  );
}

export function updateProjectConfig(
  options: ApiClientOptions,
  projectId: string,
  input: UpdateProjectConfigInput,
): Promise<Project> {
  return putJson<Project>(
    options,
    projectPath(projectId, "/config"),
    input,
    "update project config",
  );
}

export function replaceProjectMembers(
  options: ApiClientOptions,
  projectId: string,
  members: ProjectMemberInput[],
): Promise<ProjectMember[]> {
  return putJson<ProjectMember[]>(
    options,
    projectPath(projectId, "/members"),
    { members },
    "replace project members",
  );
}

export function listProjectMembers(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectMember[]> {
  return getJson<ProjectMember[]>(
    options,
    projectPath(projectId, "/members"),
    "project members",
  );
}

export function listProjectTasks(
  options: ApiClientOptions,
  projectId: string,
  filters: ListProjectTasksFilters = {},
): Promise<ProjectTask[]> {
  return getJson<ProjectTask[]>(
    options,
    projectPath(projectId, `/tasks${taskQuery(filters)}`),
    "project tasks",
  );
}

export function listProjectEvents(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectEvent[]> {
  return getJson<ProjectEvent[]>(
    options,
    projectPath(projectId, `/events${paginationQuery(filters)}`),
    "project events",
  );
}

export function listProjectDemands(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectDemand[]> {
  return getJson<ProjectDemand[]>(
    options,
    projectPath(projectId, `/demands${paginationQuery(filters)}`),
    "project demands",
  );
}

export function submitProjectDemand(
  options: ApiClientOptions,
  projectId: string,
  input: SubmitProjectDemandInput,
): Promise<ProjectDemand> {
  return postJson<ProjectDemand>(
    options,
    projectPath(projectId, "/demands"),
    input,
    "submit project demand",
  );
}
