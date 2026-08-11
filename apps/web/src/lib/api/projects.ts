import type { ApiClientOptions } from "./client";
import {
  ApiRequestError,
  buildApiUrl,
  deleteJson,
  getJson,
  parseJson,
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

export type ProjectCoordinationMode = "plan" | "loop";

export type ProjectPrincipalType = "human_user" | "digital_employee" | "team";
export type ProjectRole = "owner" | "executor" | "reviewer" | "observer";
export type ReviewerSelectionReason =
  | "project_reviewer_default"
  | "project_human_owner_fallback"
  | "user_selected";
export type ProjectEventType =
  | "project.created"
  | "project.config.changed"
  | "project.archived"
  | "project.unarchived"
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
  | "project.archive.retention_pending"
  | "project.archive.auto_close_deferred";
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
  | "planned"
  | "executing"
  | "acceptance_pending"
  | "completed"
  | "failed"
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
  | "superseded"
  /** 数字员工自述(self_report):可读但不构成证据。 */
  | "unverified";
export type ProjectAcceptanceStatus =
  | "accepted"
  | "rejected"
  | "needs_more_evidence"
  | "partially_accepted";

export type WorkspaceReadyStatus = "pending" | "ready" | "error";

export type ProjectRepoBindingStatus = "bound" | "unbound";

export type ProjectRepoBinding = {
  status: ProjectRepoBindingStatus;
  url?: string;
  default_branch?: string;
  git_credential_ref?: string;
  scope?: string[];
};

export type Project = {
  id: string;
  tenant_id: string;
  team_id?: string;
  name: string;
  /** Runtime 工作区相对目录名（ASCII）；与展示 name 分离。 */
  directory_name: string;
  description?: string;
  goal: string;
  status: ProjectStatus;
  human_owner_user_id: string;
  human_owner_user_ids?: string[];
  coordination_workflow_id: string;
  coordination_status: string;
  coordination_policy: Record<string, unknown>;
  repo_binding?: ProjectRepoBinding;
  /** 工作区首启就绪；未就绪只挡派发。 */
  workspace_ready_status: WorkspaceReadyStatus;
  primary_runtime_node_id?: string;
  workspace_ready_error?: string;
  workspace_ready_at?: string;
  archived_at?: string;
  allowed_actions?: string[];
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
  created_at?: string;
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
  /** 已消耗的执行尝试次数 */
  attempt_count?: number;
  /** 瞬时失败自动重试预算；缺省时前端按平台默认 3 展示 */
  max_attempts?: number;
  expected_outputs?: unknown[];
  input_requirements?: Record<string, unknown>;
  handoff_contract?: Record<string, unknown>;
  planner_metadata?: Record<string, unknown>;
  dismissed_at?: string;
  dismissed_by?: string;
  created_at?: string;
  updated_at?: string;
};

export type ProjectTaskGraphNode = ProjectTask & {
  expected_outputs: unknown[];
  input_requirements: Record<string, unknown>;
  handoff_contract: Record<string, unknown>;
  planner_metadata: Record<string, unknown>;
  status_reason?: string;
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
  started_at?: string;
  finished_at?: string;
  error_message?: string;
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

export type ProjectTaskGraphBlockingFactGap = {
  constraint_kind: string;
  roles: string[];
  required_capabilities: string[];
  active_executor_count: number;
  options: string[];
};

export type ProjectTaskGraphBlockingFact = {
  reason_code: string;
  message: string;
  resource_type?: string;
  resource_id?: string;
  recommended_action?: string;
  created_at?: string;
  gap?: ProjectTaskGraphBlockingFactGap;
  decision_request_id?: string;
};

export type ProjectTaskGraphHandoffAssessmentStatus =
  | "fulfilled"
  | "partial"
  | "unfulfilled"
  | "unknown";

export type ProjectTaskGraphHandoffDeliverableVerdict = "delivered" | "missing";

export type ProjectTaskGraphHandoffDeliverable = {
  name: string;
  kind?: string;
  verdict: ProjectTaskGraphHandoffDeliverableVerdict;
  ref?: string;
  summary?: string;
};

/**
 * 结构化交接 verdict（纯读投影，spec 2026-07-27 §5 P2-V）：按声明交付物逐条
 * 核对 delivered/missing；status=unknown 表示无声明数据，前端维持"暂无"呈现，
 * 不得据此编造符合性判定。
 */
export type ProjectTaskGraphHandoffAssessment = {
  project_task_id: string;
  status: ProjectTaskGraphHandoffAssessmentStatus;
  deliverables: ProjectTaskGraphHandoffDeliverable[];
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
  handoff_assessments?: ProjectTaskGraphHandoffAssessment[];
  /** 每个任务当前的派发闸门裁决（服务端每任务只给最新一条）。 */
  dispatch_gates?: ProjectTaskGraphDispatchGate[];
};

/**
 * 任务当前的派发闸门裁决。判"是否仍被闸住"只能用它：闸门事件
 * （project_task.dispatch_gate.*）按 (任务, 事件类型) 至多发一次，任务重试后
 * 二次卡人工不会再有新事件，按事件流推断会漏报。
 */
export type ProjectTaskGraphDispatchGate = {
  project_task_id: string;
  status: DispatchGateStatus;
  checked_at: string;
  decision_request_id?: string;
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
  /**
   * 服务端渲染的用户可读叙事（唯一词表源 internal/project/event_narrative.go）。
   * 显示一律用 narrative.title；`event_type` 只作技术判别，不得当文案。
   * 老响应可能缺该字段（旧服务/缓存），调用方需自备中文兜底。
   */
  narrative?: {
    kind: string;
    title: string;
    severity?: DemandDossierSeverity;
    noise?: boolean;
  };
  created_at: string;
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
  /** 服务端已回写；OpenAPI ProjectDemand 尚未列入，读路径以 handler JSON 为准。 */
  coordination_mode?: ProjectCoordinationMode;
  scenario_template_key?: string;
  created_at?: string;
  updated_at?: string;
  /** 接续血缘：本单接着哪一单做；缺省为链头。 */
  continues_demand_id?: string;
};

export type ProjectStatusSummary = {
  current_phase: string;
  is_archived: boolean;
};

/**
 * 项目**全量**任务计数（服务端 SQL 聚合，排除 dismissed）。
 * 不要用 `ProjectOverview.active_tasks`（分页任务列表）的长度当计数。
 * total_tasks = active_tasks + completed_tasks + failed_tasks + cancelled_tasks。
 */
export type ProjectTaskSummary = {
  active_tasks: number;
  pending_human_tasks: number;
  completed_tasks: number;
  failed_tasks: number;
  running_tasks: number;
  cancelled_tasks: number;
  total_tasks: number;
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
  recent_events: ProjectEvent[];
  coordination_workflow: ProjectCoordinationWorkflow;
};

export type ProjectConfig = {
  project: Project;
  human_roles: ProjectMember[];
  digital_employee_pool: ProjectMember[];
  members: ProjectMember[];
  coordination_policy: Record<string, unknown>;
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

/**
 * 一单卷宗（spec 2026-07-29 R2）。中栏时间线是协调**叙事**（按关键节点归纳、
 * 噪音事件不入），不是完整审计流水；右轨按有效剧本的 produces kind 分槽给交付
 * 事实。只读模型，无写字段。
 */
export type DemandDossierTimelineKind =
  | "demand_submitted"
  | "coordination_started"
  | "plan_ready"
  | "plan_accepted"
  | "plan_rejected"
  | "plan_change_requested"
  | "task_created"
  | "task_dispatched"
  | "task_waiting_human"
  | "task_completed"
  | "task_failed"
  | "task_cancelled"
  | "result_recorded"
  | "result_accepted"
  | "result_rejected"
  | "decision_opened"
  | "decision_resolved"
  | "dispatch_blocked"
  | "staffing_gap"
  | "coordination_blocked"
  | "other";

export type DemandDossierSeverity = "info" | "success" | "warn" | "danger" | "mute";

export type DemandDossierTimelineItem = {
  id: string;
  occurred_at: string;
  kind: DemandDossierTimelineKind;
  /** 服务端已渲染的中文主文案；前端不得再解析原始 event_type。 */
  title: string;
  summary?: string;
  severity?: DemandDossierSeverity;
  actor_display_name?: string;
  entity?: {
    type: "task" | "decision" | "demand" | "job" | "event";
    id: string;
    name?: string;
  };
  open_target?: {
    type: "task_detail" | "decision" | "none";
    task_id?: string;
    decision_id?: string;
  };
};

export type DemandDossierRailItemState = "delivered" | "missing" | "unknown" | "info";

export type DemandDossierRailItem = {
  id: string;
  title: string;
  summary?: string;
  state: DemandDossierRailItemState;
  ref?: string;
  project_task_id?: string;
  project_task_name?: string;
};

export type DemandDossierRailSlot = {
  kind: string;
  title: string;
  items: DemandDossierRailItem[];
};

export type DemandDossierPendingAction = {
  id: string;
  kind: string;
  title: string;
  status: string;
  created_at?: string;
  href?: {
    type: "inbox" | "project_demand" | "decision";
    decision_id?: string;
    demand_id?: string;
    project_id?: string;
  };
};

export type DemandDossierHandoffAssessment = {
  project_task_id: string;
  project_task_name?: string;
  status: "fulfilled" | "partial" | "unfulfilled" | "unknown";
  deliverables: ProjectTaskGraphHandoffDeliverable[];
};

export type DemandContinuationReasonCode =
  | "ok"
  | "demand_not_settled"
  | "already_continued"
  | "chain_too_deep"
  | "project_archived";

/** 这一单所属的接续链。「一单」的用户身份是链而不是行。 */
export type ProjectDemandLineage = {
  continues_demand_id?: string;
  /** 本单是链上第几单，从 1 起 */
  chain_position: number;
  chain_length: number;
  /** 全链摘要，链头在前 */
  chain: {
    demand_id: string;
    title: string;
    status: string;
    created_at: string;
    is_current: boolean;
  }[];
  /** 能不能接续由服务端判定，前端不自己算 */
  continue_demand: {
    available: boolean;
    reason_code: DemandContinuationReasonCode;
    reason_message?: string;
  };
};

export type ProjectDemandDossier = {
  demand: ProjectDemand;
  project: {
    id: string;
    name: string;
    status?: string;
    scenario_template_key?: string | null;
  };
  lineage: ProjectDemandLineage;
  effective_playbook: {
    template_key?: string | null;
    source: "demand" | "project" | "none";
    name?: string;
    produce_kinds: string[];
    /** 本单收口：这一单走到哪一步。空串表示未规划或该计划无出口声明。 */
    exit_deliverable?: string;
    /** 收口中文标签（成案时快照，不随模板改版漂移）；可能为空。 */
    exit_label?: string;
    /** true = 收口来自尚未确认的计划，展示须标「拟」。 */
    exit_pending?: boolean;
  };
  /** 密度判定的原料，不是结论——密度由前端决定并允许用户切换。 */
  signals: {
    has_open_decisions: boolean;
    active_task_count: number;
    demand_terminal: boolean;
  };
  pending_actions: DemandDossierPendingAction[];
  timeline: {
    items: DemandDossierTimelineItem[];
    truncated: boolean;
  };
  rail: {
    slots: DemandDossierRailSlot[];
  };
  handoff_summary: {
    fulfilled: number;
    partial: number;
    unfulfilled: number;
    unknown: number;
    assessments: DemandDossierHandoffAssessment[];
  };
  acceptance?: {
    demand_status?: string;
    criteria_total?: number;
    pending_human_judgment?: number;
  };
  sibling_pending?: {
    demand_id: string;
    open_decisions: number;
    demand_title?: string;
    demand_status?: string;
  }[];
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


export type CapabilityProjectionSummary = {
  skill_count: number;
  mcp_count: number;
  conflict_count: number;
  by_source: Record<string, number>;
};

export type ProjectedSkillItem = {
  skill_id: string;
  skill_key: string;
  skill_name?: string;
  version?: string;
  source_scope: string;
};

export type ProjectedMcpItem = {
  server_id: string;
  server_key: string;
  server_name?: string;
  source_scope: string;
};

export type ProjectedSkillConflict = {
  slug: string;
  source: string;
  winning_skill_id?: string;
  dropped_skill_id?: string;
  winning_source?: string;
  dropped_source?: string;
  winning_skill_name?: string;
  dropped_skill_name?: string;
};

export type CapabilityProjectionSnapshot = {
  available: boolean;
  skills: ProjectedSkillItem[];
  mcp_servers: ProjectedMcpItem[];
  skill_conflicts: ProjectedSkillConflict[];
  summary: CapabilityProjectionSummary;
};

export type ProjectExecutionTraceAttempt = {
  project_task_id: string;
  attempt_id: string;
  attempt_no: number;
  status: string;
  runtime_node_id?: string;
  provider_type?: string;
  provider_session_id?: string;
  session_resume_status?: string;
  session_resume_label?: string;
  started_at?: string;
  finished_at?: string;
  failure_family?: string;
  /** Stable Provider ErrorEnvelope.code when present (Phase 4). */
  error_code?: string;
  retryable?: boolean;
  events: ExecutionLedgerEvent[];
  summary?: ProjectExecutionTraceAttemptSummary;
  capability_projection?: CapabilityProjectionSnapshot;
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
  /** token 预算上限；缺省表示不限（P1-A 熔断）。 */
  token_limit?: number | null;
  /** 项目下所有 attempt 心跳累加的 token 消耗之和。 */
  consumed_tokens: number;
  /** 已设额度且已消耗达到上限；为真时前端禁用发起、后端派发前闸拦新任务。 */
  exhausted: boolean;
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

export type ProjectArchiveBlockerCode =
  | "already_archived"
  | "active_tasks"
  | "open_demands"
  | "pending_decisions";

export type ProjectArchiveWarningCode =
  | "missing_evidence"
  | "open_inbox_will_cancel";

export type ProjectArchiveBlocker = {
  code: ProjectArchiveBlockerCode | string;
  message: string;
  count: number;
};

export type ProjectArchiveWarning = {
  code: ProjectArchiveWarningCode | string;
  message: string;
  count: number;
};

export type ProjectArchiveBlockedErrorResponse = {
  code: "project_archive_blocked";
  message: string;
  blockers: ProjectArchiveBlocker[];
};

export type ProjectArchivePreview = {
  project_id: string;
  can_archive: boolean;
  blockers: ProjectArchiveBlocker[];
  warnings: ProjectArchiveWarning[];
  message?: string;
  evidence_count: number;
  artifact_count: number;
  report_count: number;
  retention_pending: boolean;
  /** @deprecated use blockers/warnings */
  blocked_reasons: unknown[];
  estimated_object_refs: unknown[];
};

export type ProjectDeleteBlocker = {
  type: "run" | "project_task";
  id: string;
  status: string;
  title: string;
};

export type ProjectDeleteWarnings = {
  pending_decision_count?: number;
  waiting_human_task_count?: number;
  open_inbox_count?: number;
  active_member_count?: number;
  digital_employee_member_count?: number;
  runtime_node_binding_count?: number;
  affinity_count?: number;
  automation_rule_count?: number;
};

export type ProjectDeletePreview = {
  project_id: string;
  project_name: string;
  can_delete: boolean;
  blockers: ProjectDeleteBlocker[];
  warnings: ProjectDeleteWarnings;
  message: string;
};

export type ProjectDeleteBlockedErrorResponse = {
  code: "project_delete_blocked";
  message: string;
  blockers: ProjectDeleteBlocker[];
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
  /** Runtime 目录名；非 Git 必填；Git 可省略由服务端从 URL 推导。 */
  directory_name?: string;
  description?: string;
  goal: string;
  human_owner_user_id: string;
  human_owner_user_ids?: string[];
  members?: ProjectMemberInput[];
  coordination_policy?: Record<string, unknown>;
  runtime_node_ids: string[];
  /** 可选：绑定 Git 仓库，创建后异步 clone 到项目目录。 */
  repo_binding?: ProjectRepoBinding;
  scenario_template_key?: string;
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
  members?: ProjectMemberInput[];
  coordination_policy?: Record<string, unknown>;
};

export type SubmitProjectDemandInput = {
  title: string;
  content?: string;
  source_type?: ProjectDemandSourceType;
  source_refs?: Record<string, unknown>;
  attachments?: unknown[];
  reviewer_user_id?: string;
  reviewer_selection_reason?: ReviewerSelectionReason;
  coordination_mode?: ProjectCoordinationMode;
  scenario_template_key?: string;
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

export type WorkflowInstanceScope = "active" | "archived" | "all";

export type ListWorkflowInstancesFilters = {
  q?: string;
  projectId?: string;
  status?: WorkflowInstanceStatus;
  scope?: WorkflowInstanceScope;
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
  if (filters.scope) {
    params.set("scope", filters.scope);
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

/** 运行总览项目运行带 / 项目首页组合计数单项(契约 ProjectRunSummaryItem)。 */
export type ProjectRunSummaryItem = {
  project_id: string;
  name: string;
  status: ProjectStatus;
  running_count: number;
  queued_count: number;
  waiting_human_count: number;
  failed_count: number;
  unassigned_count: number;
  participant_employee_count: number;
  completed_today_count: number;
  /** 项目上开放决策数（pending/waiting/requested/open）。 */
  open_decision_count: number;
  /** 待核证据数（submitted/rejected）。 */
  evidence_pending_count: number;
  /**
   * 无 open 决策卡挂载的「等人」任务数（orphan）。
   * 与 `open_decision_count` 并列展示时用这个，用 `waiting_human_count` 会双计；
   * 只问「多少任务卡在人身上」（如运行总览大屏）时才用 `waiting_human_count`。
   */
  waiting_human_unlinked_count: number;
  last_activity_at?: string;
};

/** 契约 ProjectRunSummaryResponse:today_completed_run_count 为租户级(含归档项目当日完成)。 */
export type ProjectRunSummaryResponse = {
  items: ProjectRunSummaryItem[];
  today_completed_run_count: number;
};

export function listProjectRunSummaries(
  options: ApiClientOptions,
  params: { limit?: number } = {},
): Promise<ProjectRunSummaryResponse> {
  const search = new URLSearchParams();
  if (params.limit) search.set("limit", String(params.limit));
  const suffix = search.size > 0 ? `?${search.toString()}` : "";
  return getJson<ProjectRunSummaryResponse>(
    options,
    `/api/v1/projects/run-summary${suffix}`,
    "project run summaries",
  );
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

export function unarchiveProject(
  options: ApiClientOptions,
  projectId: string,
): Promise<Project> {
  return postJsonWithoutBody<Project>(
    options,
    projectPath(projectId, "/unarchive"),
    "unarchive project",
  );
}

export function recloneProjectWorkspace(
  options: ApiClientOptions,
  projectId: string,
  reason?: string,
): Promise<Project> {
  return postJson<Project>(
    options,
    projectPath(projectId, "/workspace/reclone"),
    reason ? { reason } : {},
    "reclone project workspace",
  );
}

export function markProjectWorkspaceReady(
  options: ApiClientOptions,
  projectId: string,
  reason?: string,
): Promise<Project> {
  return postJson<Project>(
    options,
    projectPath(projectId, "/workspace/mark-ready"),
    reason ? { reason } : {},
    "mark project workspace ready",
  );
}

export function deleteProject(
  options: ApiClientOptions,
  projectId: string,
): Promise<void> {
  return deleteJson(options, projectPath(projectId), "delete project");
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

export type ProjectRuntimeNodeBinding = {
  runtime_node_id: string;
};

export function addProjectRuntimeNode(
  options: ApiClientOptions,
  projectId: string,
  runtimeNodeId: string,
  input?: { reason?: string },
): Promise<ProjectRuntimeNodeBinding> {
  return putJson<ProjectRuntimeNodeBinding>(
    options,
    projectPath(projectId, `/runtime-nodes/${encodeURIComponent(runtimeNodeId)}`),
    input ?? {},
    "project runtime node",
  );
}

export function removeProjectRuntimeNode(
  options: ApiClientOptions,
  projectId: string,
  runtimeNodeId: string,
): Promise<void> {
  return deleteJson(
    options,
    projectPath(projectId, `/runtime-nodes/${encodeURIComponent(runtimeNodeId)}`),
    "project runtime node",
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

export function dismissProjectTask(
  options: ApiClientOptions,
  projectId: string,
  taskId: string,
): Promise<ProjectTask> {
  return postJsonWithoutBody<ProjectTask>(
    options,
    projectPath(projectId, `/tasks/${encodeURIComponent(taskId)}/dismiss`),
    "dismiss project task",
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

export function getProjectDemandDossier(
  options: ApiClientOptions,
  demandId: string,
  params?: { timelineLimit?: number; siblingPending?: boolean },
): Promise<ProjectDemandDossier> {
  const query = new URLSearchParams();
  if (params?.timelineLimit) query.set("timeline_limit", String(params.timelineLimit));
  if (params?.siblingPending) query.set("sibling_pending", "true");
  const suffix = query.size > 0 ? `?${query.toString()}` : "";
  return getJson<ProjectDemandDossier>(
    options,
    `/api/v1/project-demands/${encodeURIComponent(demandId)}/dossier${suffix}`,
    "project demand dossier",
  );
}

/**
 * 接续这一单：新开一单并接上血缘链。原单不动（终态永不回退）。
 * 派发时同一员工会回到自己在链上的既有会话。
 */
export function createProjectDemandContinuation(
  options: ApiClientOptions,
  demandId: string,
  body: { content: string; title?: string },
): Promise<ProjectDemand> {
  return postJson<ProjectDemand>(
    options,
    `/api/v1/project-demands/${encodeURIComponent(demandId)}/continuations`,
    body,
    "project demand continuation",
  );
}

export type DemandCriterionDeliverable = {
  artifact_ref_id: string;
  title: string;
  content_type?: string;
  size_bytes?: number;
};

export type DemandCriterionTaskSummary = {
  task_id: string;
  summary: string;
  deliverables: DemandCriterionDeliverable[];
};

export type DemandAcceptanceCriterionDetail = {
  criterion_id: string;
  statement: string;
  verification_method: string;
  severity: string;
  satisfied_by: string[];
  verdict:
    | "satisfied"
    | "unsatisfied"
    | "not_applicable"
    | "pending"
    | "escalate_human"
    | null;
  judge_type: "human" | "executor" | "adversarial" | "review_gate" | null;
  evidence_refs: string[];
  task_summaries: DemandCriterionTaskSummary[];
};

export type DemandAcceptanceCriteriaDetail = {
  demand_id: string;
  demand_status: string;
  criteria: DemandAcceptanceCriterionDetail[];
};

export function getDemandAcceptanceCriteria(
  options: ApiClientOptions,
  demandId: string,
): Promise<DemandAcceptanceCriteriaDetail> {
  return getJson<DemandAcceptanceCriteriaDetail>(
    options,
    `/api/v1/project-demands/${encodeURIComponent(demandId)}/acceptance-criteria`,
    "demand acceptance criteria",
  );
}

export type SignDemandCriterionVerdictInput = {
  criterion_id: string;
  verdict: "satisfied" | "unsatisfied";
  reason?: string;
  /** §5.3「通过并结项」：签署后项目可结项时直接归档，不产生结项确认卡。默认 false。 */
  also_close_project?: boolean;
};

export type SignDemandCriterionVerdictResult = {
  demand_id: string;
  demand_status: string;
  criterion_id: string;
  verdict: string;
  signed: number;
  total: number;
  remaining: number;
};

export function signDemandCriterionVerdict(
  options: ApiClientOptions,
  demandId: string,
  input: SignDemandCriterionVerdictInput,
): Promise<SignDemandCriterionVerdictResult> {
  return postJson<SignDemandCriterionVerdictResult>(
    options,
    `/api/v1/project-demands/${encodeURIComponent(demandId)}/criterion-verdicts`,
    input,
    "sign demand criterion verdict",
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
  input: {
    decision: string;
    comment?: string;
    payload?: Record<string, unknown>;
    target_exit_deliverable?: string;
  },
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

/**
 * 拉取工件文本内容用于预览,两步取回:①credentialed 向控制面拿 presigned
 * URL(format=json);②无凭证直取对象存储。不用 302 跟随——fetch 跨域重定向
 * 会把 Origin 置为 null(redirect taint),迫使桶 CORS 放行 null origin;
 * 两步取回下第二跳 Origin 干净,桶只需放行常规 web origin。
 */
export async function getArtifactContentText(
  options: ApiClientOptions,
  artifactRefId: string,
): Promise<string> {
  const fetcher = options.fetcher ?? fetch;
  const locationResponse = await fetcher(
    buildApiUrl(
      options.baseUrl,
      `/api/v1/artifacts/${encodeURIComponent(artifactRefId)}/content?format=json`,
    ),
    { credentials: "include", method: "GET" },
  );
  const location = await parseJson<{ url: string }>(
    locationResponse,
    "artifact content location",
  );
  const contentResponse = await fetcher(location.url, {
    credentials: "omit",
    method: "GET",
  });
  if (!contentResponse.ok) {
    throw new ApiRequestError("artifact content", contentResponse.status);
  }
  return contentResponse.text();
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

/** 提额/设限/清限（P1-A）。tokenLimit 传 null 清回不限。 */
export function setProjectBudget(
  options: ApiClientOptions,
  projectId: string,
  tokenLimit: number | null,
): Promise<ProjectBudgetSummary> {
  return putJson<ProjectBudgetSummary>(
    options,
    projectPath(projectId, "/budget-summary"),
    { token_limit: tokenLimit },
    "project budget",
  );
}

export async function getProjectAcceptance(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectAcceptanceRecord | null> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(
    buildApiUrl(options.baseUrl, projectPath(projectId, "/acceptance")),
    {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    },
  );
  // 204 = 项目存在但尚未提交验收结论。
  if (response.status === 204) {
    return null;
  }
  return parseJson<ProjectAcceptanceRecord>(response, "project acceptance");
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

export function getProjectDeletePreview(
  options: ApiClientOptions,
  projectId: string,
): Promise<ProjectDeletePreview> {
  return getJson<ProjectDeletePreview>(
    options,
    projectPath(projectId, "/delete-preview"),
    "project delete preview",
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
