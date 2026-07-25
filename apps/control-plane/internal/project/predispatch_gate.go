package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	PreDispatchGateStatusPassed         = "passed"
	PreDispatchGateStatusWaitingHuman   = "waiting_human"
	PreDispatchGateStatusBlocked        = "blocked"
	PreDispatchGateStatusRetryLater     = "retry_later"
	PreDispatchGateStatusReplanRequired = "replan_required"
)

const (
	DispatchReasonRootReady          = "root_ready"
	DispatchReasonDependencyUnlocked = "dependency_unlocked"
	DispatchReasonHumanResolved      = "human_resolved"
	DispatchReasonRetry              = "retry"
	DispatchReasonManual             = "manual"
)

const (
	PreDispatchHumanActionPermissionApproval = "permission_approval"
	PreDispatchHumanActionRiskApproval       = "risk_approval"
	PreDispatchHumanActionMissingContext     = "missing_context"
	PreDispatchHumanActionToolAuthorization  = "tool_authorization"
	PreDispatchHumanActionRuntimeRecovery    = "runtime_recovery"
	PreDispatchHumanActionBudgetApproval     = "budget_approval"
	PreDispatchHumanActionReplanDecision     = "replan_decision"
)

type PreDispatchGateInput struct {
	ProjectID              uuid.UUID
	ProjectTaskID          uuid.UUID
	AcceptedPlanRevisionID *uuid.UUID
	PlannedTaskKey         *string
	SelectedEmployeeID     uuid.UUID
	AttemptNo              int32
	DispatchReason         string
}

type PreDispatchGateSnapshot struct {
	Task          ProjectTask
	ActiveAttempt *PreDispatchAttemptSnapshot
	Dependencies  []PreDispatchDependencySnapshot
	Employee      PreDispatchEmployeeSnapshot
	Capabilities  PreDispatchCapabilitySnapshot
	Runtime       PreDispatchRuntimeSnapshot
	Budget        PreDispatchBudgetSnapshot
	Risk          PreDispatchRiskSnapshot
	Context       PreDispatchContextSnapshot
}

type PreDispatchAttemptSnapshot struct {
	ID     uuid.UUID
	Status string
}

type PreDispatchDependencySnapshot struct {
	TaskID              uuid.UUID
	Status              string
	AcceptanceSatisfied bool
	ResultVersion       string
}

type PreDispatchEmployeeSnapshot struct {
	ID                  uuid.UUID
	IsProjectExecutor   bool
	Status              string
	PolicyAllowed       bool
	RequiredLoadSlots   int32
	AvailableLoadSlots  int32
	ProfileSnapshotHash string
}

// PreDispatchCapabilitySnapshot carries the planner's capability reasoning for
// display and audit only. It is never a gate input: the keys are free text with
// no registry, no server-side validation, and no runtime effect, so a model can
// name anything here. See the 2026-07-10 plan-phase refactor spec, constraint 1.
type PreDispatchCapabilitySnapshot struct {
	Required []string
	Matched  []string
}

type PreDispatchRuntimeSnapshot struct {
	// PlacementPresent means the project has at least one runtime node in its
	// eligibility set (project_runtime_nodes). It no longer reflects the legacy
	// single active project_placement — Plan B's eligibility set is the source
	// of truth for whether a runtime binding exists at all.
	PlacementPresent bool
	// Pinned means the task's current attempt already has a hard-pinned runtime
	// node (from a prior successful dispatch). When true, NodeOnline reports the
	// status of that SPECIFIC pinned node (via the three-layer resolver), not of
	// the eligibility set in general — per the anti-drift rule, an offline
	// pinned node must never be papered over by some other node being online.
	Pinned bool
	// NodeOnline means the relevant node (the pinned node when Pinned is true,
	// otherwise a resolvable node from the online eligibility set) is currently
	// dispatchable.
	NodeOnline              bool
	ProviderAvailable       bool
	WorkspaceReady          bool
	SlotAvailable           bool
	ContractVersionAccepted bool
	RetryAfter              time.Time
}

type PreDispatchBudgetSnapshot struct {
	ProjectBudgetAllowed bool
	TaskBudgetPresent    bool
	NeedsApproval        bool
	ApprovalGranted      bool
	// TokenLimit / ConsumedTokens 记录 token 预算判定依据(P1-A);TokenLimit 为 nil
	// 表示项目未设额度(不限),此时 ProjectBudgetAllowed 恒为 true。
	TokenLimit     *int64
	ConsumedTokens int64
}

type PreDispatchRiskSnapshot struct {
	HumanApprovalRequired bool
	HumanApprovalGranted  bool
	Reason                string
}

type PreDispatchContextSnapshot struct {
	RequiredRefsResolved bool
	InjectionAllowed     bool
	MissingRefs          []string
}

type PreDispatchGateEvaluation struct {
	Status             string
	CheckedAt          time.Time
	Checks             []PreDispatchGateCheck
	Blockers           []PreDispatchGateBlocker
	HumanActionRequest *PreDispatchHumanActionRequest
	RetryAfter         *time.Time
	IdempotencyKey     string
	DispatchToken      string
	CreateRun          bool
}

type PreDispatchGateCheck struct {
	Key     string
	Status  string
	Details map[string]any
}

type PreDispatchGateBlocker struct {
	Key       string
	Severity  string
	Retryable bool
	Details   map[string]any
}

type PreDispatchHumanActionRequest struct {
	Type          string
	WaitingReason string
	DecisionType  string
	Title         string
	Summary       string
	RiskLevel     string
	Options       []any
	Context       map[string]any
}

type preDispatchGateBlockerWithStatus struct {
	blocker PreDispatchGateBlocker
	status  string
	order   int
}

func EvaluatePreDispatchGate(input PreDispatchGateInput, snapshot PreDispatchGateSnapshot, now time.Time) PreDispatchGateEvaluation {
	input.DispatchReason = normalizeDispatchReason(input.DispatchReason)
	result := PreDispatchGateEvaluation{
		Status:         PreDispatchGateStatusPassed,
		CheckedAt:      now,
		IdempotencyKey: PreDispatchGateIdempotencyKey(input),
		DispatchToken:  PreDispatchGateDispatchToken(input),
		CreateRun:      true,
	}
	pendingBlockers := make([]preDispatchGateBlockerWithStatus, 0)

	addCheck := func(key, status string, details map[string]any) {
		result.Checks = append(result.Checks, PreDispatchGateCheck{Key: key, Status: status, Details: sanitizeGateDetails(details)})
	}
	addBlocker := func(key, status, severity string, retryable bool, details map[string]any) {
		pendingBlockers = append(pendingBlockers, preDispatchGateBlockerWithStatus{
			blocker: PreDispatchGateBlocker{Key: key, Severity: severity, Retryable: retryable, Details: sanitizeGateDetails(details)},
			status:  status,
			order:   len(pendingBlockers),
		})
	}
	setStatus := func(status string) {
		if preDispatchGateStatusPriority(status) >= preDispatchGateStatusPriority(result.Status) {
			result.Status = status
		}
	}

	task := snapshot.Task
	if task.ID != input.ProjectTaskID || task.ProjectID != input.ProjectID {
		addCheck("task.identity", "failed", map[string]any{"reason": "task_project_mismatch"})
		addBlocker("task.identity_mismatch", PreDispatchGateStatusBlocked, "hard", false, nil)
		setStatus(PreDispatchGateStatusBlocked)
	} else {
		addCheck("task.identity", "passed", nil)
	}

	if input.AcceptedPlanRevisionID != nil {
		if task.AcceptedPlanRevisionID == nil || *task.AcceptedPlanRevisionID != *input.AcceptedPlanRevisionID {
			var taskRevisionID any
			if task.AcceptedPlanRevisionID != nil {
				taskRevisionID = task.AcceptedPlanRevisionID.String()
			}
			addCheck("task.accepted_plan_revision", "failed", map[string]any{
				"input_accepted_plan_revision_id": input.AcceptedPlanRevisionID.String(),
				"task_accepted_plan_revision_id":  taskRevisionID,
			})
			addBlocker("task.accepted_plan_revision_changed", PreDispatchGateStatusReplanRequired, "hard", false, nil)
			setStatus(PreDispatchGateStatusReplanRequired)
		} else {
			addCheck("task.accepted_plan_revision", "passed", map[string]any{"accepted_plan_revision_id": input.AcceptedPlanRevisionID.String()})
		}
	}

	if input.PlannedTaskKey != nil {
		if task.PlannedTaskKey == nil || *task.PlannedTaskKey != *input.PlannedTaskKey {
			var taskKey any
			if task.PlannedTaskKey != nil {
				taskKey = *task.PlannedTaskKey
			}
			addCheck("task.planned_task_key", "failed", map[string]any{
				"input_planned_task_key": *input.PlannedTaskKey,
				"task_planned_task_key":  taskKey,
			})
			addBlocker("task.planned_task_key_changed", PreDispatchGateStatusReplanRequired, "hard", false, nil)
			setStatus(PreDispatchGateStatusReplanRequired)
		} else {
			addCheck("task.planned_task_key", "passed", map[string]any{"planned_task_key": *input.PlannedTaskKey})
		}
	}

	if task.Status != ProjectTaskStatusPlanned && task.Status != ProjectTaskStatusWaitingHuman {
		addCheck("task.dispatchable", "failed", map[string]any{"status": task.Status})
		addBlocker("task.status_not_dispatchable", PreDispatchGateStatusBlocked, "hard", false, map[string]any{"status": task.Status})
		setStatus(PreDispatchGateStatusBlocked)
	} else {
		addCheck("task.dispatchable", "passed", map[string]any{"status": task.Status})
	}

	if snapshot.ActiveAttempt != nil && activeAttemptStatus(snapshot.ActiveAttempt.Status) {
		addCheck("task.active_attempt", "failed", map[string]any{"attempt_id": snapshot.ActiveAttempt.ID.String(), "status": snapshot.ActiveAttempt.Status})
		addBlocker("task.active_attempt_exists", PreDispatchGateStatusBlocked, "hard", false, map[string]any{"attempt_id": snapshot.ActiveAttempt.ID.String()})
		setStatus(PreDispatchGateStatusBlocked)
	} else {
		addCheck("task.active_attempt", "passed", nil)
	}

	if task.MaxAttempts != nil && input.AttemptNo > *task.MaxAttempts {
		addCheck("task.retry_policy", "failed", map[string]any{"attempt_no": input.AttemptNo, "max_attempts": *task.MaxAttempts})
		addBlocker("task.retry_exhausted", PreDispatchGateStatusBlocked, "hard", false, nil)
		setStatus(PreDispatchGateStatusBlocked)
	} else {
		addCheck("task.retry_policy", "passed", map[string]any{"attempt_no": input.AttemptNo})
	}

	dependencyFailed := false
	for _, dep := range snapshot.Dependencies {
		if dep.Status != ProjectTaskStatusCompleted || !dep.AcceptanceSatisfied {
			dependencyFailed = true
			addBlocker("dependency.not_ready", PreDispatchGateStatusBlocked, "hard", false, map[string]any{"project_task_id": dep.TaskID.String(), "status": dep.Status, "acceptance_satisfied": dep.AcceptanceSatisfied})
		}
	}
	if dependencyFailed {
		addCheck("dependency.ready", "failed", nil)
		setStatus(PreDispatchGateStatusBlocked)
	} else {
		addCheck("dependency.ready", "passed", map[string]any{"dependency_count": len(snapshot.Dependencies)})
	}

	if task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID != input.SelectedEmployeeID {
		addCheck("employee.selected", "failed", nil)
		addBlocker("employee.selection_changed", PreDispatchGateStatusReplanRequired, "hard", false, nil)
		setStatus(PreDispatchGateStatusReplanRequired)
	} else if snapshot.Employee.ID != input.SelectedEmployeeID {
		addCheck("employee.snapshot_identity", "failed", map[string]any{"snapshot_employee_id": snapshot.Employee.ID.String(), "selected_employee_id": input.SelectedEmployeeID.String()})
		addBlocker("employee.snapshot_mismatch", PreDispatchGateStatusReplanRequired, "hard", false, nil)
		setStatus(PreDispatchGateStatusReplanRequired)
	} else if !snapshot.Employee.IsProjectExecutor || !preDispatchEmployeeStatusDispatchable(snapshot.Employee.Status) || !snapshot.Employee.PolicyAllowed {
		addCheck("employee.dispatchable", "failed", map[string]any{"status": snapshot.Employee.Status, "project_executor": snapshot.Employee.IsProjectExecutor, "policy_allowed": snapshot.Employee.PolicyAllowed})
		addBlocker("employee.not_dispatchable", PreDispatchGateStatusReplanRequired, "hard", false, nil)
		setStatus(PreDispatchGateStatusReplanRequired)
	} else if snapshot.Employee.AvailableLoadSlots < snapshot.Employee.RequiredLoadSlots {
		addCheck("employee.load", "failed", map[string]any{"available": snapshot.Employee.AvailableLoadSlots, "required": snapshot.Employee.RequiredLoadSlots})
		addBlocker("employee.slot_unavailable", PreDispatchGateStatusRetryLater, "transient", true, nil)
		setStatus(PreDispatchGateStatusRetryLater)
	} else {
		addCheck("employee.dispatchable", "passed", map[string]any{"profile_snapshot_hash": snapshot.Employee.ProfileSnapshotHash})
	}

	if !snapshot.Runtime.PlacementPresent {
		addCheck("runtime.placement", "failed", nil)
		addBlocker("runtime.placement_missing", PreDispatchGateStatusBlocked, "hard", false, nil)
		setStatus(PreDispatchGateStatusBlocked)
	} else if snapshot.Runtime.Pinned && !snapshot.Runtime.NodeOnline {
		// The task is hard-pinned to a specific node (a prior attempt already
		// bound one) and that node is currently offline/out of capacity. Per the
		// anti-drift rule this must wait for that node, not be re-selected onto a
		// different one — so it gets its own blocker key and Blocked status
		// (parked, non-terminal) rather than the RetryLater active-poll status
		// used for "no node was ever pinned".
		addCheck("runtime.ready", "failed", map[string]any{"node_online": false, "pinned": true})
		addBlocker("runtime.pinned_node_offline", PreDispatchGateStatusBlocked, "hard", false, nil)
		setStatus(PreDispatchGateStatusBlocked)
	} else if !snapshot.Runtime.NodeOnline {
		addCheck("runtime.ready", "failed", map[string]any{"node_online": false})
		addBlocker("runtime.no_eligible_online_node", PreDispatchGateStatusRetryLater, "transient", true, nil)
		setStatus(PreDispatchGateStatusRetryLater)
	} else if !snapshot.Runtime.ProviderAvailable {
		addCheck("runtime.ready", "failed", map[string]any{"provider_available": false})
		addBlocker("runtime.provider_unavailable", PreDispatchGateStatusRetryLater, "transient", true, nil)
		setStatus(PreDispatchGateStatusRetryLater)
	} else if !snapshot.Runtime.WorkspaceReady {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionRuntimeRecovery, HumanWaitReasonRuntimeRecovery, "project_task_runtime_recovery", "执行工作区未就绪", "需要恢复 Runtime 工作区后才能分派任务", "medium")
		addCheck("runtime.workspace", "failed", nil)
		addBlocker("runtime.workspace_not_ready", PreDispatchGateStatusWaitingHuman, "human", false, nil)
		setStatus(PreDispatchGateStatusWaitingHuman)
	} else if !snapshot.Runtime.SlotAvailable {
		addCheck("runtime.ready", "failed", map[string]any{"slot_available": false})
		addBlocker("runtime.slot_unavailable", PreDispatchGateStatusRetryLater, "transient", true, nil)
		setStatus(PreDispatchGateStatusRetryLater)
	} else if !snapshot.Runtime.ContractVersionAccepted {
		addCheck("runtime.contract", "failed", nil)
		addBlocker("runtime.contract_version_unsupported", PreDispatchGateStatusReplanRequired, "hard", false, nil)
		setStatus(PreDispatchGateStatusReplanRequired)
	} else {
		addCheck("runtime.ready", "passed", nil)
	}

	if !snapshot.Budget.TaskBudgetPresent {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionBudgetApproval, HumanWaitReasonBudgetApproval, "project_task_budget_approval", "任务预算缺失", "需要确认任务预算和超时策略后才能分派任务", "medium")
		addCheck("budget.ready", "failed", map[string]any{"task_budget_present": false})
		addBlocker("budget.task_budget_missing", PreDispatchGateStatusWaitingHuman, "human", false, nil)
		setStatus(PreDispatchGateStatusWaitingHuman)
	} else if !snapshot.Budget.ProjectBudgetAllowed && snapshot.Budget.TokenLimit != nil {
		// Token 预算耗尽:提示提额而非泛泛「确认预算」。提额(改大 budget_token_limit)后
		// 下次派发 consumed < limit 自动放行,不是一次性审批。
		summary := fmt.Sprintf("项目 token 预算已耗尽（已用 %d / 上限 %d），提高额度后可继续派发任务", snapshot.Budget.ConsumedTokens, *snapshot.Budget.TokenLimit)
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionBudgetApproval, HumanWaitReasonBudgetApproval, "project_task_budget_approval", "项目 token 预算已耗尽", summary, "medium")
		addCheck("budget.ready", "failed", map[string]any{
			"project_budget_allowed": false,
			"token_limit":            *snapshot.Budget.TokenLimit,
			"consumed_tokens":        snapshot.Budget.ConsumedTokens,
		})
		addBlocker("budget.token_exhausted", PreDispatchGateStatusWaitingHuman, "human", false, nil)
		setStatus(PreDispatchGateStatusWaitingHuman)
	} else if !snapshot.Budget.ProjectBudgetAllowed || (snapshot.Budget.NeedsApproval && !snapshot.Budget.ApprovalGranted) {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionBudgetApproval, HumanWaitReasonBudgetApproval, "project_task_budget_approval", "项目预算需要确认", "需要人类确认预算后才能分派任务", "medium")
		addCheck("budget.ready", "failed", map[string]any{"project_budget_allowed": snapshot.Budget.ProjectBudgetAllowed})
		addBlocker("budget.approval_required", PreDispatchGateStatusWaitingHuman, "human", false, nil)
		setStatus(PreDispatchGateStatusWaitingHuman)
	} else {
		addCheck("budget.ready", "passed", nil)
	}

	if snapshot.Risk.HumanApprovalRequired && !snapshot.Risk.HumanApprovalGranted {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionRiskApproval, HumanWaitReasonApprovalRequired, "project_task_approval", "高风险动作需要确认", riskApprovalSummary(task, snapshot.Risk.Reason), "high")
		if title := strings.TrimSpace(task.Title); title != "" {
			result.HumanActionRequest.Context["task_title"] = title
		}
		if snapshot.Risk.Reason != "" {
			result.HumanActionRequest.Context["risk_reason"] = snapshot.Risk.Reason
			result.HumanActionRequest.Context["risk_reason_label"] = riskReasonLabel(snapshot.Risk.Reason)
		}
		addCheck("risk.approval", "failed", map[string]any{"reason": snapshot.Risk.Reason})
		addBlocker("risk.approval_required", PreDispatchGateStatusWaitingHuman, "human", false, nil)
		setStatus(PreDispatchGateStatusWaitingHuman)
	} else {
		addCheck("risk.approval", "passed", nil)
	}

	if !snapshot.Context.RequiredRefsResolved {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionMissingContext, HumanWaitReasonMissingContext, "project_task_missing_context", "任务缺少必要上下文", "需要补充上下文后才能分派任务", "medium")
		result.HumanActionRequest.Context["missing_refs"] = append([]string(nil), snapshot.Context.MissingRefs...)
		addCheck("context.ready", "failed", map[string]any{"missing_refs": append([]string(nil), snapshot.Context.MissingRefs...)})
		addBlocker("context.missing_required_refs", PreDispatchGateStatusWaitingHuman, "human", false, nil)
		setStatus(PreDispatchGateStatusWaitingHuman)
	} else if !snapshot.Context.InjectionAllowed {
		addCheck("context.policy", "failed", nil)
		addBlocker("context.injection_denied", PreDispatchGateStatusBlocked, "hard", false, nil)
		setStatus(PreDispatchGateStatusBlocked)
	} else {
		addCheck("context.ready", "passed", nil)
	}

	if result.Status != PreDispatchGateStatusPassed {
		result.CreateRun = false
	}
	if result.Status == PreDispatchGateStatusRetryLater && !snapshot.Runtime.RetryAfter.IsZero() {
		retryAfter := snapshot.Runtime.RetryAfter
		result.RetryAfter = &retryAfter
	}
	if len(pendingBlockers) > 0 {
		sort.SliceStable(pendingBlockers, func(i, j int) bool {
			leftStatusPriority := preDispatchGateStatusPriority(pendingBlockers[i].status)
			rightStatusPriority := preDispatchGateStatusPriority(pendingBlockers[j].status)
			if leftStatusPriority != rightStatusPriority {
				return leftStatusPriority > rightStatusPriority
			}
			leftSeverityPriority := preDispatchGateBlockerSeverityPriority(pendingBlockers[i].blocker.Severity)
			rightSeverityPriority := preDispatchGateBlockerSeverityPriority(pendingBlockers[j].blocker.Severity)
			if leftSeverityPriority != rightSeverityPriority {
				return leftSeverityPriority > rightSeverityPriority
			}
			return pendingBlockers[i].order < pendingBlockers[j].order
		})
		result.Blockers = make([]PreDispatchGateBlocker, 0, len(pendingBlockers))
		for _, pending := range pendingBlockers {
			result.Blockers = append(result.Blockers, pending.blocker)
		}
	}
	if result.Status != PreDispatchGateStatusWaitingHuman {
		result.HumanActionRequest = nil
	} else {
		sanitizeHumanActionRequest(result.HumanActionRequest)
	}
	sort.SliceStable(result.Checks, func(i, j int) bool { return result.Checks[i].Key < result.Checks[j].Key })
	return result
}

func (r PreDispatchGateEvaluation) CheckKeys() []string {
	keys := make([]string, 0, len(r.Checks))
	for _, check := range r.Checks {
		keys = append(keys, check.Key)
	}
	return keys
}

func PreDispatchGateIdempotencyKey(input PreDispatchGateInput) string {
	reason := normalizeDispatchReason(input.DispatchReason)
	return fmt.Sprintf("project-task:%s:reason:%s:attempt:%d:employee:%s", input.ProjectTaskID, reason, input.AttemptNo, input.SelectedEmployeeID)
}

func PreDispatchGateDispatchToken(input PreDispatchGateInput) string {
	sum := sha256.Sum256([]byte(PreDispatchGateIdempotencyKey(input)))
	return hex.EncodeToString(sum[:])
}

func normalizeDispatchReason(reason string) string {
	reason = strings.TrimSpace(reason)
	switch reason {
	case DispatchReasonRootReady, DispatchReasonDependencyUnlocked, DispatchReasonHumanResolved, DispatchReasonRetry, DispatchReasonManual:
		return reason
	default:
		return DispatchReasonRootReady
	}
}

func activeAttemptStatus(status string) bool {
	return status == ProjectTaskAttemptStatusQueued || status == ProjectTaskAttemptStatusRunning || status == ProjectTaskAttemptStatusWaitingHuman
}

func preDispatchEmployeeStatusDispatchable(status string) bool {
	return status == "ready" || status == "active"
}

func preDispatchGateStatusPriority(status string) int {
	switch status {
	case PreDispatchGateStatusRetryLater:
		return 1
	case PreDispatchGateStatusWaitingHuman:
		return 2
	case PreDispatchGateStatusReplanRequired:
		return 3
	case PreDispatchGateStatusBlocked:
		return 4
	default:
		return 0
	}
}

func preDispatchGateBlockerSeverityPriority(severity string) int {
	switch severity {
	case "hard":
		return 3
	case "human":
		return 2
	case "transient":
		return 1
	default:
		return 0
	}
}

func sanitizeGateDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return map[string]any{}
	}
	clean := make(map[string]any, len(details))
	for key, value := range details {
		if sensitiveGateKey(key) {
			clean[key] = "[redacted]"
			continue
		}
		clean[key] = sanitizeGateValue(value)
	}
	return clean
}

func sanitizeGateValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeGateDetails(typed)
	case []any:
		clean := make([]any, len(typed))
		for i, item := range typed {
			clean[i] = sanitizeGateValue(item)
		}
		return clean
	case []string:
		return append([]string(nil), typed...)
	case nil:
		return nil
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Map:
		if reflected.Type().Key().Kind() != reflect.String {
			return value
		}
		clean := make(map[string]any, reflected.Len())
		iter := reflected.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			if sensitiveGateKey(key) {
				clean[key] = "[redacted]"
				continue
			}
			clean[key] = sanitizeGateValue(iter.Value().Interface())
		}
		return clean
	case reflect.Slice, reflect.Array:
		clean := make([]any, reflected.Len())
		for i := 0; i < reflected.Len(); i++ {
			clean[i] = sanitizeGateValue(reflected.Index(i).Interface())
		}
		return clean
	default:
		return value
	}
}

func sensitiveGateKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "connection_string")
}

func sanitizeHumanActionRequest(request *PreDispatchHumanActionRequest) {
	if request == nil {
		return
	}
	request.Context = sanitizeGateDetails(request.Context)
}

// riskApprovalSummary makes the "high-risk action" card self-describing: the
// generic boilerplate never told the human WHICH action was gated (the task's
// title lived only in the raw context payload) nor WHY (an opaque reason code).
// It now names the concrete task and a readable trigger.
func riskApprovalSummary(task ProjectTask, reason string) string {
	summary := "需要人类确认高风险动作后才能分派任务"
	if title := strings.TrimSpace(task.Title); title != "" {
		summary = fmt.Sprintf("%s：「%s」", summary, title)
	}
	if label := riskReasonLabel(reason); label != "" {
		summary = fmt.Sprintf("%s（触发原因：%s）", summary, label)
	}
	return summary
}

// riskReasonLabel maps pre-dispatch risk reason codes to human-readable Chinese.
// Unknown codes fall back to the raw code so nothing is silently dropped.
func riskReasonLabel(reason string) string {
	switch strings.TrimSpace(reason) {
	case "":
		return ""
	case "task.requires_human_approval":
		return "任务被标记为需人工审批"
	default:
		return strings.TrimSpace(reason)
	}
}

func humanGateRequest(actionType, waitingReason, decisionType, title, summary, riskLevel string) *PreDispatchHumanActionRequest {
	return &PreDispatchHumanActionRequest{
		Type:          actionType,
		WaitingReason: waitingReason,
		DecisionType:  decisionType,
		Title:         title,
		Summary:       summary,
		RiskLevel:     riskLevel,
		Options:       []any{"approved", "rejected", "needs_more_evidence", "cancelled"},
		Context:       map[string]any{"source": "predispatch_gate"},
	}
}
