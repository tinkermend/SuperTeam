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
	Tools         PreDispatchToolSnapshot
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

type PreDispatchCapabilitySnapshot struct {
	Required    []string
	Matched     []string
	HardMissing []string
	Unknown     []string
}

type PreDispatchToolSnapshot struct {
	MissingBindings       []string
	ExpiredAuthorizations []string
	RetryableUnavailable  []string
}

type PreDispatchRuntimeSnapshot struct {
	PlacementPresent        bool
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

	if len(snapshot.Capabilities.HardMissing) > 0 {
		addCheck("capability.match", "failed", map[string]any{"hard_missing": append([]string(nil), snapshot.Capabilities.HardMissing...)})
		addBlocker("capability.hard_missing", PreDispatchGateStatusReplanRequired, "hard", false, map[string]any{"hard_missing": append([]string(nil), snapshot.Capabilities.HardMissing...)})
		setStatus(PreDispatchGateStatusReplanRequired)
	} else {
		addCheck("capability.match", "passed", map[string]any{"required": append([]string(nil), snapshot.Capabilities.Required...), "matched": append([]string(nil), snapshot.Capabilities.Matched...)})
	}

	if len(snapshot.Tools.ExpiredAuthorizations) > 0 {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionToolAuthorization, HumanWaitReasonPermissionRequired, "project_task_permission", "工具授权已失效", "需要人类重新授权后才能分派任务", "medium")
		addCheck("tool.authorization", "failed", map[string]any{"expired": append([]string(nil), snapshot.Tools.ExpiredAuthorizations...)})
		addBlocker("tool.authorization_expired", PreDispatchGateStatusWaitingHuman, "human", false, nil)
		setStatus(PreDispatchGateStatusWaitingHuman)
	} else if len(snapshot.Tools.MissingBindings) > 0 {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionToolAuthorization, HumanWaitReasonPermissionRequired, "project_task_permission", "任务缺少工具绑定", "需要补齐 MCP 或外部能力绑定后才能分派任务", "medium")
		addCheck("tool.binding", "failed", map[string]any{"missing": append([]string(nil), snapshot.Tools.MissingBindings...)})
		addBlocker("tool.binding_missing", PreDispatchGateStatusWaitingHuman, "human", false, nil)
		setStatus(PreDispatchGateStatusWaitingHuman)
	} else if len(snapshot.Tools.RetryableUnavailable) > 0 {
		addCheck("tool.available", "failed", map[string]any{"retryable_unavailable": append([]string(nil), snapshot.Tools.RetryableUnavailable...)})
		addBlocker("tool.retryable_unavailable", PreDispatchGateStatusRetryLater, "transient", true, nil)
		setStatus(PreDispatchGateStatusRetryLater)
	} else {
		addCheck("tool.available", "passed", nil)
	}

	if !snapshot.Runtime.PlacementPresent {
		addCheck("runtime.placement", "failed", nil)
		addBlocker("runtime.placement_missing", PreDispatchGateStatusBlocked, "hard", false, nil)
		setStatus(PreDispatchGateStatusBlocked)
	} else if !snapshot.Runtime.NodeOnline {
		addCheck("runtime.ready", "failed", map[string]any{"node_online": false})
		addBlocker("runtime.node_offline", PreDispatchGateStatusRetryLater, "transient", true, nil)
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
	} else if !snapshot.Budget.ProjectBudgetAllowed || (snapshot.Budget.NeedsApproval && !snapshot.Budget.ApprovalGranted) {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionBudgetApproval, HumanWaitReasonBudgetApproval, "project_task_budget_approval", "项目预算需要确认", "需要人类确认预算后才能分派任务", "medium")
		addCheck("budget.ready", "failed", map[string]any{"project_budget_allowed": snapshot.Budget.ProjectBudgetAllowed})
		addBlocker("budget.approval_required", PreDispatchGateStatusWaitingHuman, "human", false, nil)
		setStatus(PreDispatchGateStatusWaitingHuman)
	} else {
		addCheck("budget.ready", "passed", nil)
	}

	if snapshot.Risk.HumanApprovalRequired && !snapshot.Risk.HumanApprovalGranted {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionRiskApproval, HumanWaitReasonApprovalRequired, "project_task_approval", "高风险动作需要确认", "需要人类确认风险后才能分派任务", "high")
		if snapshot.Risk.Reason != "" {
			result.HumanActionRequest.Context["risk_reason"] = snapshot.Risk.Reason
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
