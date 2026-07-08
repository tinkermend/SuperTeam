import type { ApiClientOptions } from "./client";
import {
  deleteJsonWithResponse,
  getJson,
  patchJson,
  postJson,
  postJsonWithoutBody,
  putJson,
} from "./client";

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
export type ReviewerSelectionReason =
  | "project_reviewer_default"
  | "project_human_owner_fallback"
  | "user_selected";
export type ProjectEventType =
  | "project.created"
  | "project.config.changed"
  | "project.archived"
  | "demand.submitted"
  | "workflow.signaled"
  | "workflow.coordination_failed"
  | "coordination_job.created"
  | "coordination.blocked"
  | "route_decision.created"
  | "project.runtime_placement.updated"
  | "project.runtime_placement.released"
  | "project_task.created"
  | "project_task.dispatched"
  | "project_task.dispatch_blocked"
  | "project_task.dispatch_gate.blocked"
  | "project_task.dispatch_gate.checked"
  | "project_task.dispatch_gate.replan_required"
  | "project_task.dispatch_gate.retry_later"
  | "project_task.dispatch_gate.waiting_human"
  | "project_task.completed"
  | "project_task.failed"
  | "transfer.requested"
  | "decision.requested"
  | "decision.submitted"
  | "project.evidence.linked"
  | "project.evidence.verified"
  | "project.artifact.linked"
  | "project.report.linked"
  | "project.budget.recorded"
  | "project.acceptance.submitted"
  | "project.archive_snapshot.created"
  | "project.archive.retention_pending";
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
export type WorkflowInstanceStatus =
  | "planning"
  | "running"
  | "waiting_human"
  | "failed"
  | "completed"
  | "cancelled"
  | "unknown";
export type WorkflowInstanceProgress = {
  total_nodes: number;
  completed_nodes: number;
  running_nodes: number;
  blocked_nodes: number;
  waiting_human_nodes: number;
  planned_nodes?: number;
  failed_nodes?: number;
  cancelled_nodes?: number;
};
export type WorkflowInstanceCurrentBlocker = {
  type: string;
  title: string;
  resource_id?: string;
};
export type WorkflowInstancePriority = {
  value: string;
  label: string;
  source: string;
};
export type WorkflowInstanceRisk = {
  level: string;
  label: string;
  source: string;
};
export type WorkflowInstanceSLA = {
  due_at?: string;
  remaining_seconds?: number;
  breached: boolean;
  label: string;
  source: string;
};
export type WorkflowInstanceRecentEvent = {
  event_type: string;
  summary: string;
  occurred_at: string;
};
export type WorkflowInstanceSummary = {
  demand_id: string;
  project_id: string;
  project_name: string;
  title: string;
  submitted_by_user_id: string;
  submitted_by_display_name: string;
  status: WorkflowInstanceStatus;
  status_reason: string;
  created_at: string;
  updated_at: string;
  selected_coordination_job_id?: string;
  progress: WorkflowInstanceProgress;
  current_blocker?: WorkflowInstanceCurrentBlocker;
  priority?: WorkflowInstancePriority;
  risk?: WorkflowInstanceRisk;
  sla?: WorkflowInstanceSLA;
  recent_event?: WorkflowInstanceRecentEvent;
};
export type ProjectEvidenceVerificationStatus =
  | "submitted"
  | "linked"
  | "verified"
  | "rejected"
  | "superseded";
export type ProjectAcceptanceStatus =
  | "accepted"
  | "rejected"
  | "needs_more_evidence"
  | "partially_accepted";

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

export type ReviewerPreference = {
  reviewer_user_id: string;
  selection_reason: ReviewerSelectionReason;
  display_name?: string;
  project_role: ProjectRole;
  resolved_from_rule: boolean;
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
  coordination_job_id?: string;
  route_decision_id?: string;
  planned_task_key?: string;
  task_kind?: string;
  stage_index?: number;
  expected_outputs?: unknown[];
  input_requirements?: Record<string, unknown>;
  handoff_contract?: Record<string, unknown>;
  planner_metadata?: Record<string, unknown>;
};

export type ProjectTaskGraphNode = ProjectTask & {
  expected_outputs: unknown[];
  input_requirements: Record<string, unknown>;
  handoff_contract: Record<string, unknown>;
  planner_metadata: Record<string, unknown>;
  status_reason?: string;
  updated_at?: string;
  started_at?: string;
  finished_at?: string;
  current_blocker?: WorkflowInstanceCurrentBlocker;
};

export type ProjectTaskGraphEdge = {
  dependent_task_id: string;
  blocker_task_id: string;
  coordination_job_id?: string;
  edge_status: string;
};

export type ProjectTaskGraphEmployeeAvatarAsset = {
  id: string;
  label: string;
  image_url: string;
  thumbnail_url: string;
};

export type ProjectTaskGraphEmployee = {
  digital_employee_id: string;
  display_name: string;
  employee_role?: string;
  avatar_asset?: ProjectTaskGraphEmployeeAvatarAsset;
  project_role: ProjectRole;
  status: string;
};

export type ProjectTaskGraphRun = {
  project_task_id: string;
  digital_employee_run_id?: string;
  runtime_task_id?: string;
  runtime_node_id?: string;
  runtime_node_summary: string;
  status: string;
  provider_type: string;
};

export type ProjectTaskGraphStageSummary = {
  stage_index: number;
  title: string;
  total_nodes: number;
  completed_nodes: number;
  running_nodes: number;
  waiting_human_nodes: number;
  blocked_nodes: number;
};

export type ProjectTaskGraphBlockingFact = {
  reason_code: string;
  message: string;
  resource_type?: string;
  resource_id?: string;
  recommended_action?: string;
  created_at?: string;
};

export type ProjectTaskGraph = {
  nodes: ProjectTaskGraphNode[];
  edges: ProjectTaskGraphEdge[];
  employees: ProjectTaskGraphEmployee[];
  runs: ProjectTaskGraphRun[];
  execution_summaries: ProjectExecutionSummary[];
  recent_events: ProjectEvent[];
  decision_requests: ProjectDecisionRequest[];
  stage_summaries?: ProjectTaskGraphStageSummary[];
  blocking_facts: ProjectTaskGraphBlockingFact[];
};

export type ProjectRuntimePlacementStatus =
  | "missing"
  | "ready"
  | "runtime_offline"
  | "command_channel_disconnected"
  | "provider_unavailable"
  | "capacity_full"
  | "workspace_pending"
  | "contract_mismatch";

export type ProjectRuntimePlacementState = "active" | "released" | "lost";

export type ProjectRuntimePlacement = {
  id: string;
  project_id: string;
  runtime_node_id: string;
  placement_status: ProjectRuntimePlacementState;
  placement_reason?: string;
  assigned_at: string;
  released_at?: string;
  created_at: string;
  updated_at: string;
};

export type ProjectReadinessReason = {
  code: string;
  message: string;
  resource_type?: string;
  resource_id?: string;
};

export type ProjectReadinessAction = {
  code: string;
  label: string;
  description?: string;
  resource_type?: string;
  resource_id?: string;
};

export type ProjectEmployeeReadiness = {
  digital_employee_id: string;
  display_name?: string;
  provider_type?: string;
  can_plan: boolean;
  can_dispatch: boolean;
  reason_code?: string;
  reason_message?: string;
};

export type ProjectRuntimePlacementReadiness = {
  placement_status: ProjectRuntimePlacementStatus;
  runtime_node_id?: string;
  runtime_node_name?: string;
  command_channel_connected: boolean;
  provider_capabilities?: string[];
  required_provider_types: string[];
  employee_readiness: ProjectEmployeeReadiness[];
  blocking_reasons: ProjectReadinessReason[];
  next_actions: ProjectReadinessAction[];
};

export type PutProjectRuntimePlacementInput = {
  runtime_node_id: string;
  reason?: string;
  expected_provider_types?: string[];
};

export type DispatchGateStatus =
  | "passed"
  | "waiting_human"
  | "blocked"
  | "retry_later"
  | "replan_required";

export type DispatchGateCheck = {
  key: string;
  status: string;
  details: Record<string, unknown>;
};

export type DispatchGateBlocker = {
  key: string;
  severity: string;
  retryable: boolean;
  details: Record<string, unknown>;
};

export type DispatchGateResult = {
  id: string;
  project_task_id: string;
  accepted_plan_revision_id?: string | null;
  planned_task_key?: string | null;
  selected_employee_id: string;
  attempt_no: number;
  dispatch_reason: string;
  status: DispatchGateStatus;
  checked_at: string;
  checks: DispatchGateCheck[];
  blockers: DispatchGateBlocker[];
  human_action_request: Record<string, unknown>;
  retry_after?: string | null;
  attempt_id?: string | null;
  decision_request_id?: string | null;
};

export type DispatchGateListResponse = {
  items: DispatchGateResult[];
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
  reviewer: ReviewerPreference | null;
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

export type ProjectDemandLaunchDetail = {
  demand: ProjectDemand;
  project: Project;
  reviewer: ReviewerPreference | null;
  coordination_jobs: ProjectCoordinationJob[];
  route_decisions: ProjectRouteDecision[];
  project_tasks: ProjectTask[];
  execution_summaries: ProjectExecutionSummary[];
  decision_requests: ProjectDecisionRequest[];
  recent_events: ProjectEvent[];
};

export type ProjectRouteDecision = {
  id: string;
  tenant_id: string;
  project_id: string;
  coordination_job_id: string;
  demand_id?: string;
  candidate_digital_employee_ids: string[];
  selected_digital_employee_ids: string[];
  reason: string;
  input_requirements: Record<string, unknown>;
  expected_outputs: unknown[];
  budget_estimate: Record<string, unknown>;
  requires_human_review: boolean;
  created_event_id?: string;
  created_at?: string;
};

export type ProjectPlanRevision = {
  id: string;
  tenant_id: string;
  team_id?: string;
  project_id: string;
  demand_id: string;
  coordination_job_id?: string;
  route_decision_id?: string;
  revision_number: number;
  status: string;
  payload: Record<string, unknown>;
  planner_provider?: string;
  planner_model?: string;
  planner_input_hash?: string;
  plan_fingerprint: string;
  validation_errors: string[];
  validation_warnings: string[];
  review_required: boolean;
  review_reason?: string;
  accepted_by?: string;
  accepted_at?: string;
  rejected_by?: string;
  rejected_at?: string;
  rejection_reason?: string;
  superseded_by_revision_id?: string;
  decomposition_claim_id?: string;
  created_task_ids: string[];
  created_at?: string;
  updated_at?: string;
};

export type ProjectCoordinationJob = {
  id: string;
  tenant_id: string;
  project_id: string;
  workflow_id: string;
  trigger_event_id?: string;
  job_type: string;
  status: string;
  input_snapshot_ref: Record<string, unknown>;
  output_event_ids: unknown[];
  started_at?: string;
  finished_at?: string;
  created_at?: string;
};

export type ProjectDecisionRequest = {
  id: string;
  tenant_id: string;
  project_id: string;
  approval_request_id: string;
  coordination_job_id?: string;
  project_task_id?: string;
  target_user_id: string;
  decision_type: string;
  title_snapshot: string;
  summary_snapshot?: string;
  risk_level_snapshot?: string;
  status_snapshot: string;
  created_event_id?: string;
  resolved_event_id?: string;
  created_at?: string;
  updated_at?: string;
  resolved_at?: string;
};

export type ProjectExecutionSummary = {
  id: string;
  tenant_id: string;
  project_id: string;
  project_task_id: string;
  digital_employee_id: string;
  conclusion: string;
  evidence_refs: unknown[];
  artifact_refs: unknown[];
  confidence_factors: Record<string, unknown>;
  uncertainty?: string;
  missing_information: unknown[];
  recommended_next_action?: string;
  requires_human_review: boolean;
  transfer_request_id?: string;
  created_event_id?: string;
  created_at?: string;
};

export type ExecutionLedgerEvent = {
  id: string;
  tenant_id: string;
  team_id?: string;
  project_id: string;
  project_task_id?: string;
  project_task_attempt_id?: string;
  event_type: string;
  source_type: string;
  source_id: string;
  actor_type: string;
  actor_id?: string;
  runtime_node_id?: string;
  provider_type?: string;
  provider_session_id?: string;
  input_summary?: string;
  output_summary?: string;
  error_family?: string;
  error_code?: string;
  error_message?: string;
  retryable?: boolean;
  artifact_refs: unknown[];
  evidence_refs: unknown[];
  metadata: Record<string, unknown>;
  occurred_at: string;
  created_at: string;
};

export type ProjectExecutionTraceSummary = {
  attempt_count: number;
  failed_attempt_count: number;
  human_review_required_count: number;
  artifact_ref_count: number;
  evidence_ref_count: number;
  latest_error_family?: string;
};

export type ProjectExecutionTraceAttemptSummary = {
  execution_summary_id: string;
  conclusion: string;
  requires_human_review: boolean;
  artifact_refs: unknown[];
  evidence_refs: unknown[];
  created_at: string;
};

export type ProjectExecutionTraceAttempt = {
  project_task_id: string;
  attempt_id: string;
  attempt_no: number;
  status: string;
  runtime_node_id?: string;
  provider_type?: string;
  provider_session_id?: string;
  started_at?: string;
  finished_at?: string;
  failure_family?: string;
  retryable?: boolean;
  events: ExecutionLedgerEvent[];
  summary?: ProjectExecutionTraceAttemptSummary;
};

export type ProjectExecutionTrace = {
  project_id: string;
  summary: ProjectExecutionTraceSummary;
  attempts: ProjectExecutionTraceAttempt[];
};

export type ProjectTransferRequest = {
  id: string;
  tenant_id: string;
  project_id: string;
  project_task_id: string;
  requested_by_digital_employee_id: string;
  reason: string;
  suggested_employee_type?: string;
  suggested_digital_employee_ids: string[];
  missing_context_refs: unknown[];
  status: string;
  created_event_id?: string;
  created_at?: string;
  updated_at?: string;
};

export type ProjectEvidenceRef = {
  id: string;
  tenant_id: string;
  project_id: string;
  project_task_id?: string;
  route_decision_id?: string;
  execution_summary_id?: string;
  evidence_type: string;
  title: string;
  summary?: string;
  source_type: string;
  source_ref: string;
  artifact_ref_id?: string;
  submitted_by_type: string;
  submitted_by_id?: string;
  verification_status: ProjectEvidenceVerificationStatus;
  metadata: Record<string, unknown>;
  created_event_id?: string;
  created_at?: string;
  updated_at?: string;
};

export type ProjectArtifactRef = {
  id: string;
  tenant_id: string;
  project_id: string;
  project_task_id?: string;
  artifact_id?: string;
  artifact_type: string;
  title: string;
  object_ref: string;
  content_type?: string;
  size_bytes?: number;
  checksum?: string;
  retention_status: string;
  retention_hold_id?: string;
  metadata: Record<string, unknown>;
  created_event_id?: string;
  created_at?: string;
  updated_at?: string;
};

export type ProjectReportRef = {
  id: string;
  tenant_id: string;
  project_id: string;
  report_type: string;
  title: string;
  summary?: string;
  object_ref: string;
  format: string;
  generated_by_type: string;
  generated_by_id?: string;
  created_event_id?: string;
  created_at?: string;
};

export type ProjectBudgetLedgerEntry = {
  id: string;
  tenant_id: string;
  project_id: string;
  coordination_job_id?: string;
  project_task_id?: string;
  digital_employee_id?: string;
  cost_type: string;
  estimated_tokens?: number;
  actual_tokens?: number;
  estimated_cost: string;
  actual_cost: string;
  source: string;
  reason?: string;
  created_event_id?: string;
  created_at?: string;
};

export type ProjectBudgetSummary = {
  estimated_tokens: number;
  actual_tokens: number;
  estimated_cost: string;
  actual_cost: string;
  ledger_count: number;
};

export type ProjectAcceptanceRecord = {
  id: string;
  tenant_id: string;
  project_id: string;
  accepted_by_user_id: string;
  status: ProjectAcceptanceStatus;
  conclusion: string;
  summary?: string;
  evidence_ref_ids: string[];
  report_ref_ids: string[];
  unresolved_risks: unknown[];
  created_event_id?: string;
  created_at?: string;
};

export type ProjectArchivePreview = {
  project_id: string;
  evidence_count: number;
  artifact_count: number;
  report_count: number;
  retention_pending: boolean;
  blocked_reasons: unknown[];
  estimated_object_refs: unknown[];
};

export type ProjectArchiveSnapshot = {
  id: string;
  tenant_id: string;
  project_id: string;
  snapshot_type: string;
  status: string;
  object_ref?: string;
  summary?: string;
  included_counts: Record<string, unknown>;
  retained_artifact_ids: string[];
  retention_lock_event_id?: string;
  created_by_user_id: string;
  created_event_id?: string;
  created_at?: string;
};

export type ProjectConfigRevision = {
  id: string;
  tenant_id: string;
  project_id: string;
  revision_number: number;
  config_snapshot: Record<string, unknown>;
  change_summary?: string;
  created_by_user_id: string;
  created_event_id?: string;
  created_at?: string;
  changed_sections: unknown[];
  previous_revision_id?: string;
  policy_fingerprint?: string;
  diff_summary: Record<string, unknown>;
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
  reviewer_user_id?: string;
  reviewer_selection_reason?: ReviewerSelectionReason;
};

export type CreateProjectEvidenceInput = {
  project_task_id?: string;
  route_decision_id?: string;
  execution_summary_id?: string;
  evidence_type: string;
  title: string;
  summary?: string;
  source_type: string;
  source_ref: string;
  artifact_ref_id?: string;
  metadata?: Record<string, unknown>;
};

export type PatchProjectEvidenceInput = {
  verification_status: ProjectEvidenceVerificationStatus;
  metadata?: Record<string, unknown>;
};

export type CreateProjectAcceptanceInput = {
  status: ProjectAcceptanceStatus;
  conclusion: string;
  summary?: string;
  evidence_ref_ids?: string[];
  report_ref_ids?: string[];
  unresolved_risks?: unknown[];
};

export type CreateProjectArchiveSnapshotInput = {
  snapshot_type: string;
  summary?: string;
  object_ref?: string;
};

export type ListProjectsFilters = {
  status?: ProjectStatus;
  q?: string;
  limit?: number;
  offset?: number;
};

export type ListWorkflowInstancesFilters = {
  q?: string;
  projectId?: string;
  status?: WorkflowInstanceStatus;
  limit?: number;
  offset?: number;
};

export type ListProjectTasksFilters = {
  status?: string;
  limit?: number;
  offset?: number;
};

export type GetProjectTaskGraphFilters =
  | { demandId: string; coordinationJobId?: string }
  | { coordinationJobId: string; demandId?: string };

export type PaginationFilters = {
  limit?: number;
  offset?: number;
};

export type ListProjectPlanRevisionsFilters = PaginationFilters & {
  demandId?: string;
};

export type ListProjectEvidenceFilters = PaginationFilters & {
  status?: ProjectEvidenceVerificationStatus;
};

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

function workflowInstancesPath(filters: ListWorkflowInstancesFilters = {}): string {
  const params = new URLSearchParams();
  const q = filters.q?.trim();
  if (q) {
    params.set("q", q);
  }
  if (filters.projectId) {
    params.set("project_id", filters.projectId);
  }
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
  return query
    ? `/api/v1/workflow-instances?${query}`
    : "/api/v1/workflow-instances";
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

function planRevisionsQuery(filters: ListProjectPlanRevisionsFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.demandId) {
    params.set("demand_id", filters.demandId);
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

function evidenceQuery(filters: ListProjectEvidenceFilters = {}): string {
  const pagination = paginationQuery(filters);
  const params = new URLSearchParams(pagination ? pagination.slice(1) : "");
  if (filters.status) {
    params.set("status", filters.status);
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

function taskGraphQuery(filters: GetProjectTaskGraphFilters): string {
  const params = new URLSearchParams();
  if (filters.demandId) {
    params.set("demand_id", filters.demandId);
  }
  if (filters.coordinationJobId) {
    params.set("coordination_job_id", filters.coordinationJobId);
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

export function listWorkflowInstances(
  options: ApiClientOptions,
  filters: ListWorkflowInstancesFilters = {},
): Promise<WorkflowInstanceSummary[]> {
  return getJson<WorkflowInstanceSummary[]>(
    options,
    workflowInstancesPath(filters),
    "workflow instances",
  );
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
  return postJsonWithoutBody<Project>(
    options,
    projectPath(projectId, "/archive"),
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

export function getProjectRuntimePlacement(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectRuntimePlacement> {
  return getJson<ProjectRuntimePlacement>(
    options,
    projectPath(projectId, "/runtime-placement"),
    "project runtime placement",
  );
}

export function putProjectRuntimePlacement(
  options: ApiClientOptions,
  projectId: string,
  input: PutProjectRuntimePlacementInput,
): Promise<ProjectRuntimePlacement> {
  return putJson<ProjectRuntimePlacement>(
    options,
    projectPath(projectId, "/runtime-placement"),
    input,
    "project runtime placement",
  );
}

export function releaseProjectRuntimePlacement(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectRuntimePlacement> {
  return deleteJsonWithResponse<ProjectRuntimePlacement>(
    options,
    projectPath(projectId, "/runtime-placement"),
    "project runtime placement",
  );
}

export function getProjectRuntimeReadiness(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectRuntimePlacementReadiness> {
  return getJson<ProjectRuntimePlacementReadiness>(
    options,
    projectPath(projectId, "/runtime-readiness"),
    "project runtime readiness",
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

export function getProjectTaskGraph(
  options: ApiClientOptions,
  projectId: string,
  filters: GetProjectTaskGraphFilters,
): Promise<ProjectTaskGraph> {
  return getJson<ProjectTaskGraph>(
    options,
    projectPath(projectId, `/task-graph${taskGraphQuery(filters)}`),
    "project task graph",
  );
}

export function listProjectTaskDispatchGates(
  options: ApiClientOptions,
  projectId: string,
  taskId: string,
): Promise<DispatchGateListResponse> {
  return getJson<DispatchGateListResponse>(
    options,
    projectPath(
      projectId,
      `/tasks/${encodeURIComponent(taskId)}/dispatch-gates`,
    ),
    "project task dispatch gates",
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

export function getProjectDemandLaunchDetail(
  options: ApiClientOptions,
  demandId: string,
): Promise<ProjectDemandLaunchDetail> {
  return getJson<ProjectDemandLaunchDetail>(
    options,
    `/api/v1/project-demands/${encodeURIComponent(demandId)}/launch-detail`,
    "project demand launch detail",
  );
}

export function listProjectRouteDecisions(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectRouteDecision[]> {
  return getJson<ProjectRouteDecision[]>(
    options,
    projectPath(projectId, `/route-decisions${paginationQuery(filters)}`),
    "project route decisions",
  );
}

export function listProjectPlanRevisions(
  options: ApiClientOptions,
  projectId: string,
  filters: ListProjectPlanRevisionsFilters = {},
): Promise<ProjectPlanRevision[]> {
  return getJson<ProjectPlanRevision[]>(
    options,
    projectPath(projectId, `/plan-revisions${planRevisionsQuery(filters)}`),
    "project plan revisions",
  );
}

export function getProjectPlanRevision(
  options: ApiClientOptions,
  projectId: string,
  revisionId: string,
): Promise<ProjectPlanRevision> {
  return getJson<ProjectPlanRevision>(
    options,
    projectPath(projectId, `/plan-revisions/${encodeURIComponent(revisionId)}`),
    "project plan revision",
  );
}

export function listProjectCoordinationJobs(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectCoordinationJob[]> {
  return getJson<ProjectCoordinationJob[]>(
    options,
    projectPath(projectId, `/coordination-jobs${paginationQuery(filters)}`),
    "project coordination jobs",
  );
}

export function listProjectDecisionRequests(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectDecisionRequest[]> {
  return getJson<ProjectDecisionRequest[]>(
    options,
    projectPath(projectId, `/decisions${paginationQuery(filters)}`),
    "project decisions",
  );
}

export function resolveProjectDecision(
  options: ApiClientOptions,
  projectId: string,
  decisionId: string,
  input: { decision: string; comment?: string; payload?: Record<string, unknown> },
): Promise<ProjectDecisionRequest> {
  return postJson<ProjectDecisionRequest>(
    options,
    projectPath(projectId, `/decisions/${encodeURIComponent(decisionId)}/resolve`),
    input,
    "resolve project decision",
  );
}

export function listProjectExecutionSummaries(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectExecutionSummary[]> {
  return getJson<ProjectExecutionSummary[]>(
    options,
    projectPath(projectId, `/execution-summaries${paginationQuery(filters)}`),
    "project execution summaries",
  );
}

export function getProjectExecutionTrace(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectExecutionTrace> {
  return getJson<ProjectExecutionTrace>(
    options,
    projectPath(projectId, `/execution-trace${paginationQuery(filters)}`),
    "project execution trace",
  );
}

export function listProjectTransferRequests(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectTransferRequest[]> {
  return getJson<ProjectTransferRequest[]>(
    options,
    projectPath(projectId, `/transfer-requests${paginationQuery(filters)}`),
    "project transfer requests",
  );
}

export function listProjectEvidence(
  options: ApiClientOptions,
  projectId: string,
  filters: ListProjectEvidenceFilters = {},
): Promise<ProjectEvidenceRef[]> {
  return getJson<ProjectEvidenceRef[]>(
    options,
    projectPath(projectId, `/evidence${evidenceQuery(filters)}`),
    "project evidence",
  );
}

export function createProjectEvidence(
  options: ApiClientOptions,
  projectId: string,
  input: CreateProjectEvidenceInput,
): Promise<ProjectEvidenceRef> {
  return postJson<ProjectEvidenceRef>(
    options,
    projectPath(projectId, "/evidence"),
    input,
    "create project evidence",
  );
}

export function patchProjectEvidence(
  options: ApiClientOptions,
  projectId: string,
  evidenceId: string,
  input: PatchProjectEvidenceInput,
): Promise<ProjectEvidenceRef> {
  return patchJson<ProjectEvidenceRef>(
    options,
    projectPath(projectId, `/evidence/${encodeURIComponent(evidenceId)}`),
    input,
    "patch project evidence",
  );
}

export function listProjectArtifacts(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectArtifactRef[]> {
  return getJson<ProjectArtifactRef[]>(
    options,
    projectPath(projectId, `/artifacts${paginationQuery(filters)}`),
    "project artifacts",
  );
}

export function listProjectReports(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectReportRef[]> {
  return getJson<ProjectReportRef[]>(
    options,
    projectPath(projectId, `/reports${paginationQuery(filters)}`),
    "project reports",
  );
}

export function listProjectBudgetLedger(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectBudgetLedgerEntry[]> {
  return getJson<ProjectBudgetLedgerEntry[]>(
    options,
    projectPath(projectId, `/budget-ledger${paginationQuery(filters)}`),
    "project budget ledger",
  );
}

export function getProjectBudgetSummary(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectBudgetSummary> {
  return getJson<ProjectBudgetSummary>(
    options,
    projectPath(projectId, "/budget-summary"),
    "project budget summary",
  );
}

export function createProjectAcceptance(
  options: ApiClientOptions,
  projectId: string,
  input: CreateProjectAcceptanceInput,
): Promise<ProjectAcceptanceRecord> {
  return postJson<ProjectAcceptanceRecord>(
    options,
    projectPath(projectId, "/acceptance"),
    input,
    "create project acceptance",
  );
}

export function getProjectAcceptance(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectAcceptanceRecord> {
  return getJson<ProjectAcceptanceRecord>(
    options,
    projectPath(projectId, "/acceptance"),
    "project acceptance",
  );
}

export function getProjectArchivePreview(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectArchivePreview> {
  return getJson<ProjectArchivePreview>(
    options,
    projectPath(projectId, "/archive-preview"),
    "project archive preview",
  );
}

export function createProjectArchiveSnapshot(
  options: ApiClientOptions,
  projectId: string,
  input: CreateProjectArchiveSnapshotInput,
): Promise<ProjectArchiveSnapshot> {
  return postJson<ProjectArchiveSnapshot>(
    options,
    projectPath(projectId, "/archive-snapshot"),
    input,
    "create project archive snapshot",
  );
}

export function listProjectArchiveSnapshots(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectArchiveSnapshot[]> {
  return getJson<ProjectArchiveSnapshot[]>(
    options,
    projectPath(projectId, `/archive-snapshots${paginationQuery(filters)}`),
    "project archive snapshots",
  );
}

export function listProjectConfigRevisions(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectConfigRevision[]> {
  return getJson<ProjectConfigRevision[]>(
    options,
    projectPath(projectId, `/config-revisions${paginationQuery(filters)}`),
    "project config revisions",
  );
}

export function getProjectConfigRevision(
  options: ApiClientOptions,
  projectId: string,
  revisionId: string,
): Promise<ProjectConfigRevision> {
  return getJson<ProjectConfigRevision>(
    options,
    projectPath(projectId, `/config-revisions/${encodeURIComponent(revisionId)}`),
    "project config revision",
  );
}
