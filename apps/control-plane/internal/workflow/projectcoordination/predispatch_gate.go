package projectcoordination

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)

type PreDispatchGateDecision struct {
	Gate          project.PreDispatchGateResult
	AllowRunStart bool
	Retryable     bool
	Terminal      bool
}

var (
	errPreDispatchGateApprovalCreatorRequired = errors.New("pre-dispatch gate waiting-human action requires approval creator")
	errPreDispatchGateApprovalRequestRequired = errors.New("pre-dispatch gate waiting-human action requires non-empty approval request")
)

const preDispatchGateApprovalResourceType = "project_task_dispatch_gate"

type gateApprovalDecisionRequestFinder interface {
	GetDecisionRequestByApprovalAndTask(ctx context.Context, tenantID, projectID, approvalRequestID, projectTaskID uuid.UUID) (project.DecisionRequest, error)
}

func (s *ProjectStore) RunPreDispatchGate(ctx context.Context, input DispatchProjectTaskInput) (PreDispatchGateDecision, error) {
	if s.repository == nil {
		return PreDispatchGateDecision{}, ErrActivityStoreRequired
	}
	task, err := s.repository.GetProjectTask(ctx, input.TenantID, input.TaskID)
	if err != nil {
		return PreDispatchGateDecision{}, err
	}
	if task.ProjectID != input.ProjectID {
		return PreDispatchGateDecision{}, project.ErrProjectNotFound
	}
	gateInput := project.PreDispatchGateInput{
		ProjectID:              input.ProjectID,
		ProjectTaskID:          input.TaskID,
		AcceptedPlanRevisionID: task.AcceptedPlanRevisionID,
		PlannedTaskKey:         task.PlannedTaskKey,
		SelectedEmployeeID:     selectedEmployeeID(task),
		AttemptNo:              task.AttemptCount + 1,
		DispatchReason:         project.DispatchReasonRootReady,
	}
	snapshot, err := s.loadPreDispatchGateSnapshot(ctx, input, task)
	if err != nil {
		return PreDispatchGateDecision{}, err
	}
	evaluation := project.EvaluatePreDispatchGate(gateInput, snapshot, s.now())
	if evaluation.Status == project.PreDispatchGateStatusWaitingHuman && evaluation.HumanActionRequest != nil && s.approvals == nil {
		return PreDispatchGateDecision{}, errPreDispatchGateApprovalCreatorRequired
	}
	gate, err := s.recordEvaluatedGate(ctx, input, gateInput, evaluation)
	if err != nil {
		return PreDispatchGateDecision{}, err
	}
	if gate.Status == project.PreDispatchGateStatusWaitingHuman && evaluation.HumanActionRequest != nil {
		gate, err = s.createGateHumanAction(ctx, input, task, gate, evaluation.HumanActionRequest)
		if err != nil {
			return PreDispatchGateDecision{}, err
		}
	}
	return PreDispatchGateDecision{
		Gate:          gate,
		AllowRunStart: gate.Status == project.PreDispatchGateStatusPassed,
		Retryable:     gate.Status == project.PreDispatchGateStatusRetryLater,
		Terminal:      gate.Status == project.PreDispatchGateStatusBlocked || gate.Status == project.PreDispatchGateStatusReplanRequired,
	}, nil
}

func (s *ProjectStore) loadPreDispatchGateSnapshot(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask) (project.PreDispatchGateSnapshot, error) {
	snapshot := project.PreDispatchGateSnapshot{
		Task: task,
		Employee: project.PreDispatchEmployeeSnapshot{
			ID:                 selectedEmployeeID(task),
			PolicyAllowed:      true,
			RequiredLoadSlots:  1,
			AvailableLoadSlots: 1,
		},
		Runtime: project.PreDispatchRuntimeSnapshot{
			NodeOnline:              true,
			ProviderAvailable:       true,
			WorkspaceReady:          true,
			SlotAvailable:           true,
			ContractVersionAccepted: true,
		},
		Budget: project.PreDispatchBudgetSnapshot{
			ProjectBudgetAllowed: true,
			TaskBudgetPresent:    true,
			ApprovalGranted:      true,
		},
		Context: project.PreDispatchContextSnapshot{
			RequiredRefsResolved: true,
			InjectionAllowed:     true,
		},
	}
	if attempt, err := s.repository.GetCurrentProjectTaskAttempt(ctx, input.TenantID, input.TaskID); err == nil {
		snapshot.ActiveAttempt = &project.PreDispatchAttemptSnapshot{ID: attempt.ID, Status: attempt.Status}
	} else if !errors.Is(err, project.ErrProjectNotFound) {
		return project.PreDispatchGateSnapshot{}, err
	}
	dependencies, err := s.repository.ListProjectTaskDependencies(ctx, input.TenantID, input.ProjectID, []uuid.UUID{input.TaskID})
	if err != nil {
		return project.PreDispatchGateSnapshot{}, err
	}
	snapshot.Dependencies = make([]project.PreDispatchDependencySnapshot, 0, len(dependencies))
	for _, dependency := range dependencies {
		blocker, err := s.repository.GetProjectTask(ctx, input.TenantID, dependency.BlockerTaskID)
		if err != nil {
			return project.PreDispatchGateSnapshot{}, err
		}
		if blocker.ProjectID != input.ProjectID {
			return project.PreDispatchGateSnapshot{}, project.ErrProjectNotFound
		}
		snapshot.Dependencies = append(snapshot.Dependencies, project.PreDispatchDependencySnapshot{
			TaskID:              blocker.ID,
			Status:              blocker.Status,
			AcceptanceSatisfied: blocker.Status == project.ProjectTaskStatusCompleted,
			ResultVersion:       dependencyResultVersion(blocker),
		})
	}
	if task.AssignedDigitalEmployeeID != nil && *task.AssignedDigitalEmployeeID != uuid.Nil {
		members, err := s.repository.ListProjectMembers(ctx, input.TenantID, input.ProjectID)
		if err != nil {
			return project.PreDispatchGateSnapshot{}, err
		}
		for _, member := range members {
			if member.PrincipalType != project.PrincipalTypeDigitalEmployee || member.PrincipalID != *task.AssignedDigitalEmployeeID {
				continue
			}
			snapshot.Employee.Status = member.Status
			snapshot.Employee.IsProjectExecutor = member.Status == "active" && isRoutableDigitalProjectRole(member.ProjectRole)
			break
		}
	}
	applyGateTaskMetadata(&snapshot, task)
	if task.RequiresHumanApproval {
		snapshot.Risk.HumanApprovalRequired = true
		snapshot.Risk.HumanApprovalGranted = false
		snapshot.Risk.Reason = "task.requires_human_approval"
	}
	if s.employeeReader != nil && task.AssignedDigitalEmployeeID != nil && *task.AssignedDigitalEmployeeID != uuid.Nil {
		employee, runtime, err := s.employeeReader.GetEmployeeRuntimeSnapshot(ctx, input.TenantID, input.ProjectID, *task.AssignedDigitalEmployeeID)
		if err != nil {
			return project.PreDispatchGateSnapshot{}, err
		}
		snapshot.Employee = mergePreDispatchEmployeeSnapshot(snapshot.Employee, employee)
		snapshot.Runtime = runtime
	}
	if s.capabilityReader != nil && task.AssignedDigitalEmployeeID != nil && *task.AssignedDigitalEmployeeID != uuid.Nil {
		capabilities, tools, err := s.capabilityReader.GetEmployeeCapabilitySnapshot(ctx, input.TenantID, *task.AssignedDigitalEmployeeID, task)
		if err != nil {
			return project.PreDispatchGateSnapshot{}, err
		}
		snapshot.Capabilities = capabilities
		snapshot.Tools = tools
	}
	return snapshot, nil
}

func (s *ProjectStore) recordEvaluatedGate(ctx context.Context, input DispatchProjectTaskInput, gateInput project.PreDispatchGateInput, evaluation project.PreDispatchGateEvaluation) (project.PreDispatchGateResult, error) {
	gate, err := s.repository.RecordPreDispatchGateResult(ctx, project.RecordPreDispatchGateResultRequest{
		TenantID:               input.TenantID,
		ProjectID:              input.ProjectID,
		ProjectTaskID:          input.TaskID,
		AcceptedPlanRevisionID: gateInput.AcceptedPlanRevisionID,
		PlannedTaskKey:         gateInput.PlannedTaskKey,
		SelectedEmployeeID:     gateInput.SelectedEmployeeID,
		AttemptNo:              gateInput.AttemptNo,
		DispatchReason:         gateInput.DispatchReason,
		IdempotencyKey:         evaluation.IdempotencyKey,
		DispatchToken:          evaluation.DispatchToken,
		Status:                 evaluation.Status,
		CheckedAt:              evaluation.CheckedAt,
		Checks:                 evaluation.Checks,
		Blockers:               evaluation.Blockers,
		HumanActionRequest:     humanActionMap(evaluation.HumanActionRequest),
		RetryAfter:             evaluation.RetryAfter,
	})
	if err != nil {
		return project.PreDispatchGateResult{}, err
	}
	if gate.CreatedEventID != nil {
		return gate, nil
	}
	eventType := gateEventType(gate.Status)
	exists, err := s.repository.ProjectTaskEventExists(ctx, input.TenantID, input.ProjectID, eventType, input.TaskID.String())
	if err != nil {
		return project.PreDispatchGateResult{}, err
	}
	if exists {
		return gate, nil
	}
	_, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, eventType, input.TaskID.String(), gateEventSummary(gate.Status), map[string]any{
		"project_task_id":      input.TaskID.String(),
		"status":               gate.Status,
		"selected_employee_id": gate.SelectedEmployeeID.String(),
		"attempt_no":           gate.AttemptNo,
		"dispatch_reason":      gate.DispatchReason,
		"idempotency_key":      gate.IdempotencyKey,
		"dispatch_token":       gate.DispatchToken,
		"check_count":          len(gate.Checks),
		"blocker_count":        len(gate.Blockers),
		"human_action":         cloneHumanActionRequest(gate.HumanActionRequest),
		"retry_after":          timePtrString(gate.RetryAfter),
	}))
	if err != nil {
		return project.PreDispatchGateResult{}, err
	}
	return gate, nil
}

func (s *ProjectStore) createGateHumanAction(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask, gate project.PreDispatchGateResult, action *project.PreDispatchHumanActionRequest) (project.PreDispatchGateResult, error) {
	actionPayload := humanActionMap(action)
	linkedGate := gate
	var decision project.DecisionRequest
	var err error
	switch {
	case gate.DecisionRequestID != nil:
		decision, err = s.repository.GetDecisionRequest(ctx, input.TenantID, input.ProjectID, *gate.DecisionRequestID)
		if err != nil {
			return project.PreDispatchGateResult{}, err
		}
		if err := validateGateDecisionTask(input, decision); err != nil {
			return project.PreDispatchGateResult{}, err
		}
		if decision.DispatchGateResultID == nil || *decision.DispatchGateResultID != gate.ID {
			linkedGate, err = s.repository.LinkPreDispatchGateDecisionRequest(ctx, project.LinkPreDispatchGateDecisionRequest{
				TenantID:          input.TenantID,
				ProjectID:         input.ProjectID,
				ProjectTaskID:     input.TaskID,
				GateResultID:      gate.ID,
				DecisionRequestID: *gate.DecisionRequestID,
			})
			if err != nil {
				return project.PreDispatchGateResult{}, err
			}
			decision.DispatchGateResultID = &gate.ID
		}
	case task.WaitingRequestID != nil && *task.WaitingRequestID != uuid.Nil:
		decision, err = s.repository.GetDecisionRequest(ctx, input.TenantID, input.ProjectID, *task.WaitingRequestID)
		if err != nil {
			return project.PreDispatchGateResult{}, err
		}
		if err := validateGateDecisionTask(input, decision); err != nil {
			return project.PreDispatchGateResult{}, err
		}
		linkedGate, err = s.repository.LinkPreDispatchGateDecisionRequest(ctx, project.LinkPreDispatchGateDecisionRequest{
			TenantID:          input.TenantID,
			ProjectID:         input.ProjectID,
			ProjectTaskID:     input.TaskID,
			GateResultID:      gate.ID,
			DecisionRequestID: *task.WaitingRequestID,
		})
		if err != nil {
			return project.PreDispatchGateResult{}, err
		}
		decision.DispatchGateResultID = &gate.ID
	default:
		if s.approvals == nil {
			return project.PreDispatchGateResult{}, errPreDispatchGateApprovalCreatorRequired
		}
		projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
		if err != nil {
			return project.PreDispatchGateResult{}, err
		}
		approvalRequest, err := s.findOrCreateGateApprovalRequest(ctx, input, task, gate, action, actionPayload, projectRecord.HumanOwnerUserID)
		if err != nil {
			return project.PreDispatchGateResult{}, err
		}
		decision, err = s.findGateApprovalDecisionRequest(ctx, input, gate, approvalRequest.ID)
		if err != nil && !errors.Is(err, project.ErrProjectNotFound) {
			return project.PreDispatchGateResult{}, err
		}
		if errors.Is(err, project.ErrProjectNotFound) {
			projectTaskID := input.TaskID
			decision, err = s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
				TenantID:          input.TenantID,
				ProjectID:         input.ProjectID,
				ApprovalRequestID: approvalRequest.ID,
				ProjectTaskID:     &projectTaskID,
				TargetUserID:      projectRecord.HumanOwnerUserID,
				DecisionType:      action.DecisionType,
				TitleSnapshot:     action.Title,
				SummarySnapshot:   action.Summary,
				RiskLevelSnapshot: action.RiskLevel,
				StatusSnapshot:    "pending",
			})
			if err != nil {
				return project.PreDispatchGateResult{}, err
			}
		}
		if gate.DecisionRequestID == nil || *gate.DecisionRequestID != decision.ID || decision.DispatchGateResultID == nil || *decision.DispatchGateResultID != gate.ID {
			linkedGate, err = s.repository.LinkPreDispatchGateDecisionRequest(ctx, project.LinkPreDispatchGateDecisionRequest{
				TenantID:          input.TenantID,
				ProjectID:         input.ProjectID,
				ProjectTaskID:     input.TaskID,
				GateResultID:      gate.ID,
				DecisionRequestID: decision.ID,
			})
			if err != nil {
				return project.PreDispatchGateResult{}, err
			}
		}
		decision.DispatchGateResultID = &gate.ID
	}
	eventID, err := s.ensureGateDecisionRequestedEvent(ctx, input, linkedGate, decision, actionPayload)
	if err != nil {
		return project.PreDispatchGateResult{}, err
	}
	if task.Status != project.ProjectTaskStatusWaitingHuman || task.WaitingRequestID == nil || *task.WaitingRequestID != decision.ID {
		if _, err := s.repository.MoveProjectTaskToWaitingHumanForPreDispatchGate(ctx, project.MoveProjectTaskToWaitingHumanForPreDispatchGateRequest{
			TenantID:          input.TenantID,
			ProjectID:         input.ProjectID,
			ProjectTaskID:     input.TaskID,
			GateResultID:      linkedGate.ID,
			DecisionRequestID: decision.ID,
			EventID:           eventID,
			WaitingReason:     action.WaitingReason,
		}); err != nil {
			return project.PreDispatchGateResult{}, err
		}
	}
	if s.inbox != nil {
		if decision.DispatchGateResultID == nil || *decision.DispatchGateResultID != linkedGate.ID {
			decision.DispatchGateResultID = &linkedGate.ID
		}
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return project.PreDispatchGateResult{}, err
		}
	}
	return linkedGate, nil
}

func (s *ProjectStore) findOrCreateGateApprovalRequest(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask, gate project.PreDispatchGateResult, action *project.PreDispatchHumanActionRequest, actionPayload project.HumanActionRequest, targetUserID uuid.UUID) (*approval.ApprovalRequest, error) {
	request, err := s.approvals.GetRequestByResource(ctx, input.TenantID, preDispatchGateApprovalResourceType, gate.ID)
	if err == nil {
		if request == nil || request.ID == uuid.Nil {
			return nil, errPreDispatchGateApprovalRequestRequired
		}
		return request, nil
	}
	if !errors.Is(err, approval.ErrApprovalNotFound) {
		return nil, err
	}
	request, err = s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:       input.TenantID,
		ResourceType:   preDispatchGateApprovalResourceType,
		ResourceID:     gate.ID,
		RequesterType:  "project_coordinator",
		TargetUserID:   targetUserID,
		DecisionType:   action.DecisionType,
		Title:          action.Title,
		Summary:        action.Summary,
		RiskLevel:      action.RiskLevel,
		Options:        append([]any(nil), action.Options...),
		ContextPayload: gateHumanActionContext(input, task, gate, actionPayload),
	})
	if err != nil {
		return nil, err
	}
	if request == nil || request.ID == uuid.Nil {
		return nil, errPreDispatchGateApprovalRequestRequired
	}
	return request, nil
}

func (s *ProjectStore) findGateApprovalDecisionRequest(ctx context.Context, input DispatchProjectTaskInput, gate project.PreDispatchGateResult, approvalRequestID uuid.UUID) (project.DecisionRequest, error) {
	finder, ok := s.repository.(gateApprovalDecisionRequestFinder)
	if !ok {
		return project.DecisionRequest{}, project.ErrProjectNotFound
	}
	decision, err := finder.GetDecisionRequestByApprovalAndTask(ctx, input.TenantID, input.ProjectID, approvalRequestID, input.TaskID)
	if err != nil {
		return project.DecisionRequest{}, err
	}
	if decision.DispatchGateResultID != nil && *decision.DispatchGateResultID != gate.ID {
		return project.DecisionRequest{}, project.ErrProjectNotFound
	}
	return decision, nil
}

func validateGateDecisionTask(input DispatchProjectTaskInput, decision project.DecisionRequest) error {
	if decision.ProjectTaskID == nil || *decision.ProjectTaskID != input.TaskID {
		return project.ErrProjectNotFound
	}
	return nil
}

func (s *ProjectStore) ensureGateDecisionRequestedEvent(ctx context.Context, input DispatchProjectTaskInput, gate project.PreDispatchGateResult, decision project.DecisionRequest, actionPayload project.HumanActionRequest) (*uuid.UUID, error) {
	existing, err := s.repository.GetProjectEventByTypeAndActor(ctx, input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, gate.ID.String())
	if err == nil {
		return &existing.ID, nil
	}
	if !errors.Is(err, project.ErrProjectNotFound) {
		return nil, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, gate.ID.String(), "Dispatch gate requires human action", map[string]any{
		"approval_request_id": decision.ApprovalRequestID.String(),
		"decision_type":       decision.DecisionType,
		"dispatch_gate_id":    gate.ID.String(),
		"project_task_id":     input.TaskID.String(),
		"target_user_id":      decision.TargetUserID.String(),
		"human_action":        actionPayload,
	}))
	if err != nil {
		return nil, err
	}
	return &event.ID, nil
}

func mergePreDispatchEmployeeSnapshot(base, update project.PreDispatchEmployeeSnapshot) project.PreDispatchEmployeeSnapshot {
	merged := update
	if merged.ID == uuid.Nil {
		merged.ID = base.ID
	}
	if !merged.IsProjectExecutor && base.IsProjectExecutor {
		merged.IsProjectExecutor = true
	}
	if strings.TrimSpace(merged.Status) == "" {
		merged.Status = base.Status
	}
	if !merged.PolicyAllowed && base.PolicyAllowed && update.ID == uuid.Nil && update.RequiredLoadSlots == 0 && update.AvailableLoadSlots == 0 && strings.TrimSpace(update.Status) == "" {
		merged.PolicyAllowed = true
	}
	if merged.RequiredLoadSlots == 0 && merged.AvailableLoadSlots == 0 {
		merged.RequiredLoadSlots = base.RequiredLoadSlots
		merged.AvailableLoadSlots = base.AvailableLoadSlots
	}
	if strings.TrimSpace(merged.ProfileSnapshotHash) == "" {
		merged.ProfileSnapshotHash = base.ProfileSnapshotHash
	}
	return merged
}

func gateEventType(status string) project.ProjectEventType {
	switch status {
	case project.PreDispatchGateStatusPassed:
		return project.ProjectEventTaskDispatchGateChecked
	case project.PreDispatchGateStatusWaitingHuman:
		return project.ProjectEventTaskDispatchGateWaitingHuman
	case project.PreDispatchGateStatusRetryLater:
		return project.ProjectEventTaskDispatchGateRetryLater
	case project.PreDispatchGateStatusReplanRequired:
		return project.ProjectEventTaskDispatchGateReplanRequired
	case project.PreDispatchGateStatusBlocked:
		return project.ProjectEventTaskDispatchGateBlocked
	default:
		return project.ProjectEventTaskDispatchGateBlocked
	}
}

func gateEventSummary(status string) string {
	switch status {
	case project.PreDispatchGateStatusPassed:
		return "Dispatch gate passed"
	case project.PreDispatchGateStatusWaitingHuman:
		return "Dispatch gate waiting for human action"
	case project.PreDispatchGateStatusRetryLater:
		return "Dispatch gate retry scheduled"
	case project.PreDispatchGateStatusReplanRequired:
		return "Dispatch gate requires replanning"
	default:
		return "Dispatch gate blocked"
	}
}

func selectedEmployeeID(task project.ProjectTask) uuid.UUID {
	if task.AssignedDigitalEmployeeID == nil {
		return uuid.Nil
	}
	return *task.AssignedDigitalEmployeeID
}

func humanActionMap(action *project.PreDispatchHumanActionRequest) project.HumanActionRequest {
	if action == nil {
		return nil
	}
	result := project.HumanActionRequest{
		"type":           action.Type,
		"waiting_reason": action.WaitingReason,
		"decision_type":  action.DecisionType,
		"title":          action.Title,
		"summary":        action.Summary,
		"risk_level":     action.RiskLevel,
		"options":        append([]any(nil), action.Options...),
		"context":        cloneAnyMap(action.Context),
	}
	for key, value := range action.Context {
		if _, exists := result[key]; !exists {
			result[key] = value
		}
	}
	return result
}

func cloneHumanActionRequest(input project.HumanActionRequest) project.HumanActionRequest {
	if input == nil {
		return nil
	}
	clone := make(project.HumanActionRequest, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}

func gateHumanActionContext(input DispatchProjectTaskInput, task project.ProjectTask, gate project.PreDispatchGateResult, action map[string]any) map[string]any {
	contextPayload := map[string]any{
		"project_id":       input.ProjectID.String(),
		"project_task_id":  input.TaskID.String(),
		"dispatch_gate_id": gate.ID.String(),
		"gate_status":      gate.Status,
		"task_title":       task.Title,
		"human_action":     cloneAnyMap(action),
	}
	if task.DemandID != nil {
		contextPayload["demand_id"] = task.DemandID.String()
	}
	return contextPayload
}

func applyGateTaskMetadata(snapshot *project.PreDispatchGateSnapshot, task project.ProjectTask) {
	metadata := task.PlannerMetadata
	inputRequirements := task.InputRequirements
	handoff := task.HandoffContract
	employeeSelection, _ := metadata["employee_selection"].(map[string]any)
	snapshot.Employee.ProfileSnapshotHash = firstNonEmptyString(
		stringFromAny(employeeSelection["planning_profile_snapshot_hash"]),
		stringFromAny(metadata["planning_profile_snapshot_hash"]),
	)
	snapshot.Capabilities.Required = firstNonEmptyStrings(
		stringsFromAny(inputRequirements["required_capabilities"]),
		stringsFromAny(inputRequirements["capabilities"]),
		stringsFromAny(employeeSelection["required_capabilities"]),
	)
	snapshot.Capabilities.Matched = firstNonEmptyStrings(
		stringsFromAny(employeeSelection["matched_capabilities"]),
		stringsFromAny(metadata["matched_capabilities"]),
	)
	snapshot.Capabilities.HardMissing = firstNonEmptyStrings(
		stringsFromAny(inputRequirements["hard_missing_capabilities"]),
		stringsFromAny(inputRequirements["missing_capabilities"]),
		stringsFromAny(employeeSelection["missing_capabilities"]),
	)
	snapshot.Capabilities.Unknown = stringsFromAny(inputRequirements["unknown_capabilities"])
	snapshot.Tools.MissingBindings = firstNonEmptyStrings(
		stringsFromAny(inputRequirements["missing_tool_bindings"]),
		stringsFromAny(inputRequirements["required_tool_bindings_missing"]),
	)
	snapshot.Tools.ExpiredAuthorizations = stringsFromAny(inputRequirements["expired_tool_authorizations"])
	snapshot.Tools.RetryableUnavailable = stringsFromAny(inputRequirements["retryable_unavailable_tools"])
	missingRefs := firstNonEmptyStrings(
		stringsFromAny(inputRequirements["missing_context_refs"]),
		stringsFromAny(handoff["missing_context_refs"]),
	)
	if len(missingRefs) > 0 {
		snapshot.Context.RequiredRefsResolved = false
		snapshot.Context.MissingRefs = missingRefs
	}
	if value, ok := boolFromAny(inputRequirements["context_injection_allowed"]); ok {
		snapshot.Context.InjectionAllowed = value
	}
	applyBudgetMetadata(&snapshot.Budget, metadata)
	applyBudgetMetadata(&snapshot.Budget, inputRequirements)
	applyBudgetMetadata(&snapshot.Budget, handoff)
}

func applyBudgetMetadata(budget *project.PreDispatchBudgetSnapshot, values map[string]any) {
	if len(values) == 0 {
		return
	}
	if value, ok := boolFromAny(values["project_budget_allowed"]); ok {
		budget.ProjectBudgetAllowed = value
	}
	if value, ok := boolFromAny(values["task_budget_present"]); ok {
		budget.TaskBudgetPresent = value
	}
	if value, ok := boolFromAny(values["budget_required"]); ok && value {
		budget.TaskBudgetPresent = true
	}
	if value, ok := boolFromAny(values["needs_budget_approval"]); ok {
		budget.NeedsApproval = value
	}
	if value, ok := boolFromAny(values["budget_approval_granted"]); ok {
		budget.ApprovalGranted = value
	}
}

func dependencyResultVersion(task project.ProjectTask) string {
	if !task.UpdatedAt.IsZero() {
		return task.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return task.ID.String()
}

func timePtrString(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyStrings(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func stringsFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return cleanStrings(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringFromAny(item); text != "" {
				values = append(values, text)
			}
		}
		return cleanStrings(values)
	case string:
		return cleanStrings(strings.Split(typed, ","))
	default:
		return nil
	}
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func stringFromAny(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func boolFromAny(value any) (bool, bool) {
	typed, ok := value.(bool)
	return typed, ok
}
