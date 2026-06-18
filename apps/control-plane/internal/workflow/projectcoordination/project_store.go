package projectcoordination

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)

type ProjectStore struct {
	repository project.Repository
	approvals  ApprovalCreator
	inbox      project.DecisionInboxProjector
	runStarter ProjectTaskRunStarter
	readiness  DigitalEmployeeReadinessChecker
}

// WithDigitalEmployeeReadiness attaches a runtime-readiness checker used to filter the
// coordinator's executor pool to runtime-ready digital employees.
func (s *ProjectStore) WithDigitalEmployeeReadiness(checker DigitalEmployeeReadinessChecker) *ProjectStore {
	s.readiness = checker
	return s
}

// runtimeReadyEmployeeIDs returns the set of runtime-ready digital-employee principal IDs
// among the given members. A nil/empty result means "do not filter" (no checker attached,
// no digital-employee candidates, or a checker error) so behavior stays backward-compatible.
func (s *ProjectStore) runtimeReadyEmployeeIDs(ctx context.Context, tenantID uuid.UUID, members []project.ProjectMember) map[uuid.UUID]bool {
	if s.readiness == nil {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		if member.PrincipalType == project.PrincipalTypeDigitalEmployee && member.PrincipalID != uuid.Nil {
			ids = append(ids, member.PrincipalID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	ready, err := s.readiness.AreRuntimeReady(ctx, tenantID, ids)
	if err != nil {
		// Fail open: a readiness lookup error must not block planning.
		return nil
	}
	return ready
}

func NewProjectStore(repository project.Repository) *ProjectStore {
	return NewProjectStoreWithApprovals(repository, nil)
}

type ApprovalCreator interface {
	CreateRequest(ctx context.Context, input approval.CreateRequestInput) (*approval.ApprovalRequest, error)
}

type ProjectTaskRunStarter interface {
	StartProjectTaskRun(ctx context.Context, req StartProjectTaskRunRequest) (StartProjectTaskRunResult, error)
}

// DigitalEmployeeReadinessChecker reports which digital employees are runtime-ready
// (bound to a healthy online runtime with an approved effective config). The coordinator
// uses it to filter its executor pool so the reasoning planner only proposes employees
// that can actually run, instead of stranding tasks on unbound ones.
type DigitalEmployeeReadinessChecker interface {
	AreRuntimeReady(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

type recoveryDependencyRepository interface {
	CreateProjectTaskDependency(ctx context.Context, req project.CreateProjectTaskDependencyRequest) (project.ProjectTaskDependency, error)
	RewireProjectTaskDependencies(ctx context.Context, req project.RewireProjectTaskDependenciesRequest) ([]project.ProjectTaskDependency, error)
}

func NewProjectStoreWithApprovals(repository project.Repository, approvals ApprovalCreator) *ProjectStore {
	return NewProjectStoreWithApprovalsAndInbox(repository, approvals, nil)
}

func NewProjectStoreWithApprovalsAndInbox(repository project.Repository, approvals ApprovalCreator, inbox project.DecisionInboxProjector) *ProjectStore {
	return NewProjectStoreWithApprovalsInboxAndRunStarter(repository, approvals, inbox, nil)
}

func NewProjectStoreWithApprovalsInboxAndRunStarter(repository project.Repository, approvals ApprovalCreator, inbox project.DecisionInboxProjector, runStarter ProjectTaskRunStarter) *ProjectStore {
	return &ProjectStore{repository: repository, approvals: approvals, inbox: inbox, runStarter: runStarter}
}

func (s *ProjectStore) LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error) {
	if s.repository == nil {
		return CoordinationSnapshot{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return CoordinationSnapshot{}, err
	}
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, input.DemandID)
	if err != nil {
		return CoordinationSnapshot{}, err
	}
	members, err := s.repository.ListProjectMembers(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return CoordinationSnapshot{}, err
	}
	pool := make([]ProjectMemberSnapshot, 0, len(members))
	readyEmployees := s.runtimeReadyEmployeeIDs(ctx, input.TenantID, members)
	for _, member := range members {
		if member.PrincipalType != project.PrincipalTypeDigitalEmployee || member.Status != "active" || !isRoutableDigitalProjectRole(member.ProjectRole) {
			continue
		}
		// Only runtime-ready digital employees are eligible executors; filtering here keeps
		// the reasoning planner from selecting employees whose runs cannot start.
		if readyEmployees != nil && !readyEmployees[member.PrincipalID] {
			continue
		}
		displayName := ""
		if member.DisplayNameSnapshot != nil {
			displayName = *member.DisplayNameSnapshot
		}
		pool = append(pool, ProjectMemberSnapshot{
			PrincipalID: member.PrincipalID,
			ProjectRole: string(member.ProjectRole),
			Status:      member.Status,
			DisplayName: displayName,
		})
	}
	content := ""
	if demand.Content != nil {
		content = *demand.Content
	}
	return CoordinationSnapshot{
		ProjectID:           projectRecord.ID,
		Demand:              DemandSnapshot{ID: demand.ID, Title: demand.Title, Content: content},
		DigitalEmployeePool: pool,
		CoordinationPolicy:  projectRecord.CoordinationPolicy,
	}, nil
}

func (s *ProjectStore) CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error) {
	if s.repository == nil {
		return CoordinationJobResult{}, ErrActivityStoreRequired
	}
	triggerEventID := input.TriggerEventID
	job, err := s.repository.CreateCoordinationJob(ctx, project.CreateCoordinationJobRequest{
		TenantID:       input.TenantID,
		ProjectID:      input.ProjectID,
		WorkflowID:     input.WorkflowID,
		TriggerEventID: &triggerEventID,
		JobType:        input.JobType,
		Status:         "running",
		InputSnapshotRef: map[string]any{
			"trigger_event_id": input.TriggerEventID.String(),
			"job_type":         input.JobType,
		},
	})
	if err != nil {
		return CoordinationJobResult{}, err
	}
	if _, err := s.ensureCoordinatorProjectEvent(ctx, input.TenantID, input.ProjectID, project.ProjectEventCoordinationJobCreated, job.ID.String(), "协调作业已创建", map[string]any{
		"coordination_job_id": job.ID.String(),
		"workflow_id":         input.WorkflowID,
		"trigger_event_id":    input.TriggerEventID.String(),
		"job_type":            input.JobType,
	}); err != nil {
		return CoordinationJobResult{}, err
	}
	return CoordinationJobResult{ID: job.ID}, nil
}

func (s *ProjectStore) PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error) {
	if s.repository == nil {
		return RouteDecisionResult{}, ErrActivityStoreRequired
	}
	existing, err := s.repository.GetRouteDecisionByCoordinationJob(ctx, input.TenantID, input.JobID)
	if err == nil {
		event, eventErr := s.ensureRouteDecisionCreatedEvent(ctx, input)
		if eventErr != nil {
			return RouteDecisionResult{}, eventErr
		}
		return RouteDecisionResult{ID: existing.ID, CreatedEventID: event.ID}, nil
	}
	if !errors.Is(err, project.ErrProjectNotFound) {
		return RouteDecisionResult{}, err
	}
	event, err := s.ensureRouteDecisionCreatedEvent(ctx, input)
	if err != nil {
		return RouteDecisionResult{}, err
	}
	demandID := input.DemandID
	aggregated := aggregateRouteDecisionFields(input.Decision)
	decision, err := s.repository.CreateRouteDecision(ctx, project.CreateRouteDecisionRequest{
		TenantID:                    input.TenantID,
		ProjectID:                   input.ProjectID,
		CoordinationJobID:           input.JobID,
		DemandID:                    &demandID,
		CandidateDigitalEmployeeIDs: aggregated.CandidateDigitalEmployeeIDs,
		SelectedDigitalEmployeeIDs:  aggregated.SelectedDigitalEmployeeIDs,
		Reason:                      input.Decision.Reason,
		InputRequirements:           aggregated.InputRequirements,
		ExpectedOutputs:             stringsToAny(aggregated.ExpectedOutputs),
		BudgetEstimate:              input.Decision.BudgetEstimate,
		RequiresHumanReview:         input.Decision.RequiresHumanReview,
		CreatedEventID:              &event.ID,
	})
	if err != nil {
		existing, existingErr := s.repository.GetRouteDecisionByCoordinationJob(ctx, input.TenantID, input.JobID)
		if existingErr == nil {
			return RouteDecisionResult{ID: existing.ID, CreatedEventID: event.ID}, nil
		}
		return RouteDecisionResult{}, err
	}
	return RouteDecisionResult{ID: decision.ID, CreatedEventID: event.ID}, nil
}

func (s *ProjectStore) ensureRouteDecisionCreatedEvent(ctx context.Context, input PersistRouteDecisionInput) (project.ProjectEvent, error) {
	return s.ensureCoordinatorProjectEvent(ctx, input.TenantID, input.ProjectID, project.ProjectEventRouteDecisionCreated, input.JobID.String(), "路由决策已生成", map[string]any{
		"coordination_job_id": input.JobID.String(),
		"demand_id":           input.DemandID.String(),
	})
}

func (s *ProjectStore) CreateProjectTasks(ctx context.Context, input CreateProjectTasksInput) ([]ProjectTaskResult, error) {
	if s.repository == nil {
		return nil, ErrActivityStoreRequired
	}
	graphTasks := make([]project.ProjectTaskGraphCreateTask, 0, len(input.Decision.Tasks))
	for _, plannedTask := range input.Decision.Tasks {
		status := "planned"
		if len(plannedTask.BlockedByKeys) > 0 {
			status = "blocked"
		}
		graphTasks = append(graphTasks, project.ProjectTaskGraphCreateTask{
			Key:                       plannedTask.Key,
			Title:                     plannedTask.Title,
			Summary:                   plannedTask.Summary,
			Status:                    status,
			AssignedDigitalEmployeeID: plannedTask.SelectedEmployeeID,
			TaskKind:                  plannedTask.TaskKind,
			StageIndex:                plannedTask.StageIndex,
			RiskLevel:                 plannedTask.RiskLevel,
			RequiresHumanApproval:     plannedTask.RequiresHumanApproval,
			ExpectedOutputs:           stringsToAny(plannedTask.ExpectedOutputs),
			InputRequirements:         plannedTask.InputRequirements,
			HandoffContract:           plannedTask.HandoffContract,
			PlannerMetadata:           input.Decision.PlannerMetadata,
			BlockedByKeys:             plannedTask.BlockedByKeys,
		})
	}
	graph, err := s.repository.CreateProjectTaskGraph(ctx, project.CreateProjectTaskGraphRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		DemandID:          input.DemandID,
		CoordinationJobID: input.CoordinationJobID,
		RouteDecisionID:   input.RouteDecisionID,
		Tasks:             graphTasks,
	})
	if err != nil {
		return nil, err
	}
	results := make([]ProjectTaskResult, 0, len(graph.Tasks))
	for _, task := range graph.Tasks {
		results = append(results, ProjectTaskResult{ID: task.ID})
	}
	return results, nil
}

func (s *ProjectStore) ListDispatchableTasks(ctx context.Context, input ListDispatchableTasksInput) ([]uuid.UUID, error) {
	if s.repository == nil {
		return nil, ErrActivityStoreRequired
	}
	tasks, err := s.repository.ListProjectTasksByCoordinationJob(ctx, input.TenantID, input.ProjectID, input.CoordinationJobID)
	if err != nil {
		return nil, err
	}
	candidates := make([]project.ProjectTask, 0, len(tasks))
	candidateIDs := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		if !projectTaskDispatchAllowed(task.Status) {
			continue
		}
		candidates = append(candidates, task)
		candidateIDs = append(candidateIDs, task.ID)
	}
	if len(candidates) == 0 {
		return []uuid.UUID{}, nil
	}
	unresolved, err := s.repository.ListUnresolvedBlockersForTasks(ctx, input.TenantID, input.ProjectID, candidateIDs)
	if err != nil {
		return nil, err
	}
	blockedByTaskID := unresolvedBlockersByDependent(unresolved)
	dispatchable := make([]uuid.UUID, 0, len(candidates))
	for _, task := range candidates {
		if _, blocked := blockedByTaskID[task.ID]; blocked {
			continue
		}
		dispatchable = append(dispatchable, task.ID)
	}
	return dispatchable, nil
}

func (s *ProjectStore) ResolveReadyDownstream(ctx context.Context, input ResolveReadyDownstreamInput) ([]uuid.UUID, error) {
	if s.repository == nil {
		return nil, ErrActivityStoreRequired
	}
	dependentIDs, err := s.repository.ListDependentsOfTask(ctx, input.TenantID, input.ProjectID, input.CompletedTaskID)
	if err != nil {
		return nil, err
	}
	if len(dependentIDs) == 0 {
		return []uuid.UUID{}, nil
	}
	unresolved, err := s.repository.ListUnresolvedBlockersForTasks(ctx, input.TenantID, input.ProjectID, dependentIDs)
	if err != nil {
		return nil, err
	}
	blockedByTaskID := unresolvedBlockersByDependent(unresolved)
	readyIDs := make([]uuid.UUID, 0, len(dependentIDs))
	for _, taskID := range dependentIDs {
		if _, blocked := blockedByTaskID[taskID]; blocked {
			continue
		}
		task, err := s.repository.GetProjectTask(ctx, input.TenantID, taskID)
		if err != nil {
			return nil, err
		}
		if task.ProjectID != input.ProjectID {
			return nil, project.ErrProjectNotFound
		}
		if task.Status != "blocked" {
			continue
		}
		updated, err := s.repository.UpdateProjectTaskStatus(ctx, input.TenantID, taskID, "planned", nil, []string{"blocked"})
		if err != nil {
			if errors.Is(err, project.ErrProjectConflict) {
				continue
			}
			return nil, err
		}
		readyIDs = append(readyIDs, updated.ID)
	}
	return readyIDs, nil
}

func (s *ProjectStore) HoldDownstreamForFailure(ctx context.Context, input HoldDownstreamForFailureInput) (DecisionRequestResult, error) {
	if s.repository == nil || s.approvals == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	failedTask, err := s.repository.GetProjectTask(ctx, input.TenantID, input.FailedTaskID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if failedTask.ProjectID != input.ProjectID {
		return DecisionRequestResult{}, project.ErrProjectNotFound
	}
	downstreamIDs, err := s.recursiveDownstreamTaskIDs(ctx, input.TenantID, input.ProjectID, input.FailedTaskID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	for _, taskID := range downstreamIDs {
		task, err := s.repository.GetProjectTask(ctx, input.TenantID, taskID)
		if err != nil {
			return DecisionRequestResult{}, err
		}
		if task.ProjectID != input.ProjectID {
			return DecisionRequestResult{}, project.ErrProjectNotFound
		}
		if projectTaskTerminalStatus(task.Status) || task.Status == "blocked" {
			continue
		}
		if _, err := s.repository.UpdateProjectTaskStatus(ctx, input.TenantID, taskID, "blocked", nil, failureHoldCurrentStatuses()); err != nil {
			if errors.Is(err, project.ErrProjectConflict) {
				continue
			}
			return DecisionRequestResult{}, err
		}
	}
	approvalRequest, err := s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:       input.TenantID,
		ResourceType:   "project_task",
		ResourceID:     input.FailedTaskID,
		RequesterType:  "project_coordinator",
		TargetUserID:   projectRecord.HumanOwnerUserID,
		DecisionType:   "task_failure_recovery",
		Title:          "处理项目任务失败",
		Summary:        input.FailureSummary,
		RiskLevel:      "high",
		Options:        []any{"approved", "rejected", "needs_more_evidence"},
		ContextPayload: failureRecoveryContext(input, downstreamIDs),
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, input.FailedTaskID.String(), "项目任务失败需要恢复决策", map[string]any{
		"approval_request_id": approvalRequest.ID.String(),
		"project_task_id":     input.FailedTaskID.String(),
		"failed_event_id":     input.FailedEventID.String(),
		"target_user_id":      projectRecord.HumanOwnerUserID.String(),
	}))
	if err != nil {
		return DecisionRequestResult{}, err
	}
	failedTaskID := input.FailedTaskID
	decision, err := s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ApprovalRequestID: approvalRequest.ID,
		ProjectTaskID:     &failedTaskID,
		TargetUserID:      projectRecord.HumanOwnerUserID,
		DecisionType:      "task_failure_recovery",
		TitleSnapshot:     "处理项目任务失败",
		SummarySnapshot:   input.FailureSummary,
		RiskLevelSnapshot: "high",
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return DecisionRequestResult{}, err
		}
	}
	return DecisionRequestResult{ID: decision.ID}, nil
}

func (s *ProjectStore) ApplyFailureRecoveryDecision(ctx context.Context, input ApplyFailureRecoveryDecisionInput) (ApplyFailureRecoveryDecisionResult, error) {
	if s.repository == nil {
		return ApplyFailureRecoveryDecisionResult{}, ErrActivityStoreRequired
	}
	decision, err := s.repository.GetDecisionRequest(ctx, input.TenantID, input.ProjectID, input.DecisionRequestID)
	if err != nil {
		return ApplyFailureRecoveryDecisionResult{}, err
	}
	if decision.DecisionType != "task_failure_recovery" || decision.ProjectTaskID == nil {
		return ApplyFailureRecoveryDecisionResult{}, project.ErrInvalidProject
	}
	action, err := parseFailureRecoveryAction(input.Decision, input.Payload)
	if err != nil {
		return ApplyFailureRecoveryDecisionResult{}, err
	}
	if action.Action == "needs_more_evidence" {
		return ApplyFailureRecoveryDecisionResult{ReadyTaskIDs: []uuid.UUID{}}, nil
	}
	source, err := s.repository.GetProjectTask(ctx, input.TenantID, *decision.ProjectTaskID)
	if err != nil {
		return ApplyFailureRecoveryDecisionResult{}, err
	}
	if source.ProjectID != input.ProjectID {
		return ApplyFailureRecoveryDecisionResult{}, project.ErrProjectNotFound
	}
	switch action.Action {
	case "retry":
		replacement, err := s.createRecoveryReplacementTask(ctx, input, decision, source, action)
		if err != nil {
			return ApplyFailureRecoveryDecisionResult{}, err
		}
		return s.recoveryReplacementReadyResult(ctx, input.TenantID, input.ProjectID, replacement)
	case "reassign":
		if action.NewDigitalEmployeeID == nil {
			return ApplyFailureRecoveryDecisionResult{}, project.ErrInvalidProject
		}
		if err := s.validateActiveDigitalProjectMember(ctx, input.TenantID, input.ProjectID, *action.NewDigitalEmployeeID); err != nil {
			return ApplyFailureRecoveryDecisionResult{}, err
		}
		replacement, err := s.createRecoveryReplacementTask(ctx, input, decision, source, action)
		if err != nil {
			return ApplyFailureRecoveryDecisionResult{}, err
		}
		return s.recoveryReplacementReadyResult(ctx, input.TenantID, input.ProjectID, replacement)
	case "cancel_downstream":
		if err := s.cancelFailureDownstream(ctx, input, source); err != nil {
			return ApplyFailureRecoveryDecisionResult{}, err
		}
		return ApplyFailureRecoveryDecisionResult{ReadyTaskIDs: []uuid.UUID{}}, nil
	default:
		return ApplyFailureRecoveryDecisionResult{}, project.ErrInvalidProject
	}
}

func parseFailureRecoveryAction(decision string, payload map[string]any) (FailureRecoveryAction, error) {
	switch decision {
	case "needs_more_evidence":
		return FailureRecoveryAction{Action: "needs_more_evidence"}, nil
	case "rejected":
		return FailureRecoveryAction{Action: "cancel_downstream"}, nil
	case "approved":
		raw, _ := payload["recovery_action"].(string)
		switch strings.TrimSpace(raw) {
		case "retry", "cancel_downstream":
			return FailureRecoveryAction{Action: strings.TrimSpace(raw)}, nil
		case "reassign":
			idText, _ := payload["new_digital_employee_id"].(string)
			id, err := uuid.Parse(strings.TrimSpace(idText))
			if err != nil {
				return FailureRecoveryAction{}, project.ErrInvalidProject
			}
			return FailureRecoveryAction{Action: "reassign", NewDigitalEmployeeID: &id}, nil
		default:
			return FailureRecoveryAction{}, project.ErrInvalidProject
		}
	default:
		return FailureRecoveryAction{}, project.ErrInvalidProject
	}
}

func (s *ProjectStore) createRecoveryReplacementTask(ctx context.Context, input ApplyFailureRecoveryDecisionInput, decision project.DecisionRequest, source project.ProjectTask, action FailureRecoveryAction) (project.ProjectTask, error) {
	assigneeID := source.AssignedDigitalEmployeeID
	if action.NewDigitalEmployeeID != nil {
		assigneeID = action.NewDigitalEmployeeID
	}
	if assigneeID == nil || source.DemandID == nil || source.CoordinationJobID == nil {
		return project.ProjectTask{}, project.ErrInvalidProject
	}
	replacementKey := recoveryReplacementTaskKey(source)
	sourceBlockers, err := s.repository.ListProjectTaskDependencies(ctx, input.TenantID, input.ProjectID, []uuid.UUID{source.ID})
	if err != nil {
		return project.ProjectTask{}, err
	}
	replacement, exists, err := s.findExistingRecoveryReplacement(ctx, input.TenantID, input.ProjectID, source, action, replacementKey)
	if err != nil {
		return project.ProjectTask{}, err
	}
	if !exists {
		status, err := s.recoveryReplacementStatus(ctx, input.TenantID, input.ProjectID, sourceBlockers)
		if err != nil {
			return project.ProjectTask{}, err
		}
		replacement, err = s.repository.CreateProjectTask(ctx, project.CreateProjectTaskRequest{
			TenantID:                  input.TenantID,
			ProjectID:                 input.ProjectID,
			DemandID:                  source.DemandID,
			Title:                     recoveryReplacementTitle(source.Title),
			Summary:                   stringPtrValue(source.Summary),
			Status:                    status,
			AssignedDigitalEmployeeID: assigneeID,
			RiskLevel:                 stringPtrValue(source.RiskLevel),
			RequiresHumanApproval:     source.RequiresHumanApproval,
			CoordinationJobID:         source.CoordinationJobID,
			RouteDecisionID:           source.RouteDecisionID,
			PlannedTaskKey:            &replacementKey,
			TaskKind:                  source.TaskKind,
			StageIndex:                source.StageIndex,
			ExpectedOutputs:           append([]any(nil), source.ExpectedOutputs...),
			InputRequirements:         cloneAnyMap(source.InputRequirements),
			HandoffContract:           cloneAnyMap(source.HandoffContract),
			PlannerMetadata:           recoveryPlannerMetadata(source, decision.ID, action),
		})
		if err != nil {
			existing, ok, findErr := s.findExistingRecoveryReplacement(ctx, input.TenantID, input.ProjectID, source, action, replacementKey)
			if findErr != nil {
				return project.ProjectTask{}, findErr
			}
			if !ok {
				return project.ProjectTask{}, err
			}
			replacement = existing
		}
	}
	if err := s.ensureRecoveryTaskCreatedEvent(ctx, input, decision.ID, source.ID, replacement.ID, action.Action); err != nil {
		return project.ProjectTask{}, err
	}
	if err := s.ensureReplacementBlockerDependencies(ctx, input.TenantID, input.ProjectID, replacement.ID, sourceBlockers); err != nil {
		return project.ProjectTask{}, err
	}
	if err := s.rewireRecoverableDependents(ctx, input.TenantID, input.ProjectID, source.ID, replacement.ID); err != nil {
		return project.ProjectTask{}, err
	}
	return replacement, nil
}

func (s *ProjectStore) ensureRecoveryTaskCreatedEvent(ctx context.Context, input ApplyFailureRecoveryDecisionInput, decisionRequestID, sourceTaskID, replacementTaskID uuid.UUID, action string) error {
	exists, err := s.repository.ProjectTaskEventExists(ctx, input.TenantID, input.ProjectID, project.ProjectEventTaskCreated, replacementTaskID.String())
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskCreated, replacementTaskID.String(), "恢复任务已创建", map[string]any{
		"project_task_id":        replacementTaskID.String(),
		"source_project_task_id": sourceTaskID.String(),
		"decision_request_id":    decisionRequestID.String(),
		"recovery_action":        action,
	}))
	return err
}

func (s *ProjectStore) findExistingRecoveryReplacement(ctx context.Context, tenantID, projectID uuid.UUID, source project.ProjectTask, action FailureRecoveryAction, replacementKey string) (project.ProjectTask, bool, error) {
	if source.CoordinationJobID == nil {
		return project.ProjectTask{}, false, nil
	}
	tasks, err := s.repository.ListProjectTasksByCoordinationJob(ctx, tenantID, projectID, *source.CoordinationJobID)
	if err != nil {
		return project.ProjectTask{}, false, err
	}
	for _, task := range tasks {
		if task.PlannedTaskKey == nil || *task.PlannedTaskKey != replacementKey {
			continue
		}
		if task.PlannerMetadata["source_task_id"] != source.ID.String() || task.PlannerMetadata["recovery_action"] != action.Action {
			return project.ProjectTask{}, false, project.ErrProjectConflict
		}
		if action.NewDigitalEmployeeID != nil && (task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID != *action.NewDigitalEmployeeID) {
			return project.ProjectTask{}, false, project.ErrProjectConflict
		}
		return task, true, nil
	}
	return project.ProjectTask{}, false, nil
}

func (s *ProjectStore) recoveryReplacementReadyResult(ctx context.Context, tenantID, projectID uuid.UUID, replacement project.ProjectTask) (ApplyFailureRecoveryDecisionResult, error) {
	result := ApplyFailureRecoveryDecisionResult{ReadyTaskIDs: []uuid.UUID{}}
	if !projectTaskDispatchAllowed(replacement.Status) {
		return result, nil
	}
	blockers, err := s.repository.ListUnresolvedBlockersForTasks(ctx, tenantID, projectID, []uuid.UUID{replacement.ID})
	if err != nil {
		return ApplyFailureRecoveryDecisionResult{}, err
	}
	if len(blockers) > 0 {
		return result, nil
	}
	result.ReadyTaskIDs = append(result.ReadyTaskIDs, replacement.ID)
	return result, nil
}

func (s *ProjectStore) recoveryReplacementStatus(ctx context.Context, tenantID, projectID uuid.UUID, sourceBlockers []project.ProjectTaskDependency) (string, error) {
	for _, dependency := range sourceBlockers {
		blocker, err := s.repository.GetProjectTask(ctx, tenantID, dependency.BlockerTaskID)
		if err != nil {
			return "", err
		}
		if blocker.ProjectID != projectID {
			return "", project.ErrProjectNotFound
		}
		if blocker.Status != "completed" {
			return "blocked", nil
		}
	}
	return "planned", nil
}

func (s *ProjectStore) ensureReplacementBlockerDependencies(ctx context.Context, tenantID, projectID, replacementID uuid.UUID, sourceBlockers []project.ProjectTaskDependency) error {
	if len(sourceBlockers) == 0 {
		return nil
	}
	dependencyRepository, ok := s.repository.(recoveryDependencyRepository)
	if !ok {
		return ErrActivityStoreRequired
	}
	existing, err := s.repository.ListProjectTaskDependencies(ctx, tenantID, projectID, []uuid.UUID{replacementID})
	if err != nil {
		return err
	}
	for _, sourceBlocker := range sourceBlockers {
		if dependencyExists(existing, replacementID, sourceBlocker.BlockerTaskID) {
			continue
		}
		if _, err := dependencyRepository.CreateProjectTaskDependency(ctx, project.CreateProjectTaskDependencyRequest{
			TenantID:          tenantID,
			ProjectID:         projectID,
			CoordinationJobID: sourceBlocker.CoordinationJobID,
			DependentTaskID:   replacementID,
			BlockerTaskID:     sourceBlocker.BlockerTaskID,
		}); err != nil {
			refreshed, refreshErr := s.repository.ListProjectTaskDependencies(ctx, tenantID, projectID, []uuid.UUID{replacementID})
			if refreshErr == nil && dependencyExists(refreshed, replacementID, sourceBlocker.BlockerTaskID) {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *ProjectStore) rewireRecoverableDependents(ctx context.Context, tenantID, projectID, oldBlockerID, newBlockerID uuid.UUID) error {
	dependentIDs, err := s.repository.ListDependentsOfTask(ctx, tenantID, projectID, oldBlockerID)
	if err != nil {
		return err
	}
	rewireIDs := make([]uuid.UUID, 0, len(dependentIDs))
	for _, taskID := range dependentIDs {
		task, err := s.repository.GetProjectTask(ctx, tenantID, taskID)
		if err != nil {
			return err
		}
		if task.ProjectID != projectID {
			return project.ErrProjectNotFound
		}
		if projectTaskTerminalStatus(task.Status) {
			continue
		}
		rewireIDs = append(rewireIDs, taskID)
	}
	if len(rewireIDs) == 0 {
		return nil
	}
	dependencyRepository, ok := s.repository.(recoveryDependencyRepository)
	if !ok {
		return ErrActivityStoreRequired
	}
	_, err = dependencyRepository.RewireProjectTaskDependencies(ctx, project.RewireProjectTaskDependenciesRequest{
		TenantID:         tenantID,
		ProjectID:        projectID,
		DependentTaskIDs: rewireIDs,
		OldBlockerTaskID: oldBlockerID,
		NewBlockerTaskID: newBlockerID,
	})
	return err
}

func (s *ProjectStore) cancelFailureDownstream(ctx context.Context, input ApplyFailureRecoveryDecisionInput, source project.ProjectTask) error {
	downstreamIDs, err := s.recursiveDownstreamTaskIDs(ctx, input.TenantID, input.ProjectID, source.ID)
	if err != nil {
		return err
	}
	for _, taskID := range downstreamIDs {
		task, err := s.repository.GetProjectTask(ctx, input.TenantID, taskID)
		if err != nil {
			return err
		}
		if task.ProjectID != input.ProjectID {
			return project.ErrProjectNotFound
		}
		if task.Status == "cancelled" {
			if err := s.ensureProjectTaskCancelledEvent(ctx, input, source.ID, task.ID); err != nil {
				return err
			}
			continue
		}
		if !projectTaskCancellationAllowed(task.Status) {
			continue
		}
		updated, err := s.repository.UpdateProjectTaskStatus(ctx, input.TenantID, taskID, "cancelled", nil, []string{"blocked", "planned", "pending"})
		if err != nil {
			if errors.Is(err, project.ErrProjectConflict) {
				continue
			}
			return err
		}
		if err := s.ensureProjectTaskCancelledEvent(ctx, input, source.ID, updated.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *ProjectStore) ensureProjectTaskCancelledEvent(ctx context.Context, input ApplyFailureRecoveryDecisionInput, sourceTaskID, cancelledTaskID uuid.UUID) error {
	exists, err := s.repository.ProjectTaskEventExists(ctx, input.TenantID, input.ProjectID, project.ProjectEventTaskCancelled, cancelledTaskID.String())
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskCancelled, cancelledTaskID.String(), "项目任务已取消", map[string]any{
		"project_task_id":        cancelledTaskID.String(),
		"source_project_task_id": sourceTaskID.String(),
		"decision_request_id":    input.DecisionRequestID.String(),
		"recovery_action":        "cancel_downstream",
	}))
	return err
}

func (s *ProjectStore) validateActiveDigitalProjectMember(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) error {
	members, err := s.repository.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	for _, member := range members {
		if member.PrincipalType == project.PrincipalTypeDigitalEmployee && member.PrincipalID == digitalEmployeeID && member.Status == "active" {
			return nil
		}
	}
	return project.ErrInvalidProject
}

func (s *ProjectStore) recursiveDownstreamTaskIDs(ctx context.Context, tenantID, projectID, failedTaskID uuid.UUID) ([]uuid.UUID, error) {
	seen := map[uuid.UUID]struct{}{failedTaskID: {}}
	queue := []uuid.UUID{failedTaskID}
	result := make([]uuid.UUID, 0)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		dependents, err := s.repository.ListDependentsOfTask(ctx, tenantID, projectID, current)
		if err != nil {
			return nil, err
		}
		for _, dependentID := range dependents {
			if _, exists := seen[dependentID]; exists {
				continue
			}
			seen[dependentID] = struct{}{}
			result = append(result, dependentID)
			queue = append(queue, dependentID)
		}
	}
	return result, nil
}

func projectTaskTerminalStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func projectTaskCancellationAllowed(status string) bool {
	return status == "blocked" || status == "planned" || status == "pending"
}

func failureHoldCurrentStatuses() []string {
	return []string{"planned", "pending", "assigned", "running", "waiting_human"}
}

func unresolvedBlockersByDependent(readiness []project.ProjectTaskDependencyReadiness) map[uuid.UUID]struct{} {
	blocked := map[uuid.UUID]struct{}{}
	for _, row := range readiness {
		blocked[row.DependentTaskID] = struct{}{}
	}
	return blocked
}

func (s *ProjectStore) RequestRouteDecisionReview(ctx context.Context, input RequestRouteDecisionReviewInput) (DecisionRequestResult, error) {
	if s.repository == nil || s.approvals == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	targetUserID, err := s.routeReviewTargetUserID(ctx, input, projectRecord)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	approvalRequest, err := s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:       input.TenantID,
		ResourceType:   "project_route_decision",
		ResourceID:     input.RouteDecisionID,
		RequesterType:  "project_coordinator",
		TargetUserID:   targetUserID,
		DecisionType:   "route_review",
		Title:          "确认项目路由决策",
		Summary:        input.Decision.Reason,
		RiskLevel:      "high",
		Options:        []any{"approved", "rejected", "needs_more_evidence"},
		ContextPayload: routeReviewContext(input),
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, input.CoordinationJobID.String(), "路由决策需要人类确认", map[string]any{
		"approval_request_id": approvalRequest.ID.String(),
		"route_decision_id":   input.RouteDecisionID.String(),
		"demand_id":           input.DemandID.String(),
		"target_user_id":      targetUserID.String(),
	}))
	if err != nil {
		return DecisionRequestResult{}, err
	}
	coordinationJobID := input.CoordinationJobID
	decision, err := s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ApprovalRequestID: approvalRequest.ID,
		CoordinationJobID: &coordinationJobID,
		TargetUserID:      targetUserID,
		DecisionType:      "route_review",
		TitleSnapshot:     "确认项目路由决策",
		SummarySnapshot:   input.Decision.Reason,
		RiskLevelSnapshot: "high",
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return DecisionRequestResult{}, err
		}
	}
	return DecisionRequestResult{ID: decision.ID}, nil
}

// IsProjectAcceptanceReady reports whether every demand of the project has reached a
// terminal state, i.e. the project is ready for human acceptance.
func (s *ProjectStore) IsProjectAcceptanceReady(ctx context.Context, input IsProjectAcceptanceReadyInput) (bool, error) {
	if s.repository == nil {
		return false, ErrActivityStoreRequired
	}
	return s.repository.AreAllProjectDemandsTerminal(ctx, input.TenantID, input.ProjectID)
}

// RequestProjectAcceptanceReview moves the project into the acceptance state and opens a
// human-decision item (approval + decision request + inbox) for the human owner. It is
// idempotent: the running→acceptance status transition is the guard — if the project is
// no longer running (already pending/terminal acceptance), it returns a zero result and
// the caller must not record a new pending handle.
func (s *ProjectStore) RequestProjectAcceptanceReview(ctx context.Context, input RequestProjectAcceptanceReviewInput) (DecisionRequestResult, error) {
	if s.repository == nil || s.approvals == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if _, err := s.repository.TransitionProjectStatus(ctx, input.TenantID, input.ProjectID, []string{string(project.ProjectStatusRunning)}, string(project.ProjectStatusAcceptance)); err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			// Already in acceptance/terminal: a review is already pending or resolved.
			return DecisionRequestResult{}, nil
		}
		return DecisionRequestResult{}, err
	}
	targetUserID := projectRecord.HumanOwnerUserID
	if projectRecord.AcceptanceUserID != nil && *projectRecord.AcceptanceUserID != uuid.Nil {
		targetUserID = *projectRecord.AcceptanceUserID
	}
	approvalRequest, err := s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:      input.TenantID,
		ResourceType:  "project",
		ResourceID:    input.ProjectID,
		RequesterType: "project_coordinator",
		TargetUserID:  targetUserID,
		DecisionType:  "project_acceptance",
		Title:         "验收项目交付",
		Summary:       "项目全部需求已完成,请确认验收",
		RiskLevel:     "high",
		Options:       []any{"accepted", "rejected", "needs_more_evidence"},
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, input.ProjectID.String(), "项目进入待验收,等待人类确认", map[string]any{
		"approval_request_id": approvalRequest.ID.String(),
		"project_id":          input.ProjectID.String(),
		"target_user_id":      targetUserID.String(),
	}))
	if err != nil {
		return DecisionRequestResult{}, err
	}
	decision, err := s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ApprovalRequestID: approvalRequest.ID,
		TargetUserID:      targetUserID,
		DecisionType:      "project_acceptance",
		TitleSnapshot:     "验收项目交付",
		SummarySnapshot:   "项目全部需求已完成,请确认验收",
		RiskLevelSnapshot: "high",
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return DecisionRequestResult{}, err
		}
	}
	return DecisionRequestResult{ID: decision.ID}, nil
}

// ApplyProjectAcceptanceDecision closes the acceptance loop: accept archives the project
// (and records an accepted acceptance conclusion); reject / needs_more_evidence reopens it
// to running for rework. The decision's conclusion, if provided in the payload, is recorded.
func (s *ProjectStore) ApplyProjectAcceptanceDecision(ctx context.Context, input ApplyProjectAcceptanceDecisionInput) error {
	if s.repository == nil {
		return ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return err
	}
	accepted := strings.EqualFold(strings.TrimSpace(input.Decision), "accepted")
	status := "rejected"
	if accepted {
		status = "accepted"
	} else if strings.EqualFold(strings.TrimSpace(input.Decision), "needs_more_evidence") {
		status = "needs_more_evidence"
	}
	conclusion := acceptanceConclusion(input.Payload, status)
	acceptedBy := projectRecord.HumanOwnerUserID
	if accepted {
		if _, err := s.repository.ArchiveProject(ctx, input.TenantID, input.ProjectID); err != nil {
			return err
		}
	} else {
		if _, err := s.repository.TransitionProjectStatus(ctx, input.TenantID, input.ProjectID, []string{string(project.ProjectStatusAcceptance)}, string(project.ProjectStatusRunning)); err != nil && !errors.Is(err, project.ErrProjectNotFound) {
			return err
		}
	}
	_, err = s.repository.CreateAcceptanceRecordWithEvent(ctx, project.CreateAcceptanceRecordWithEventRequest{
		Event: coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventAcceptanceSubmitted, input.DecisionRequestID.String(), acceptanceSummary(status), map[string]any{
			"decision_request_id": input.DecisionRequestID.String(),
			"decision":            status,
		}),
		Acceptance: project.CreateAcceptanceRecordRequest{
			TenantID:         input.TenantID,
			ProjectID:        input.ProjectID,
			AcceptedByUserID: acceptedBy,
			Status:           status,
			Conclusion:       conclusion,
		},
	})
	return err
}

func acceptanceConclusion(payload map[string]any, status string) string {
	if payload != nil {
		if text, ok := payload["conclusion"].(string); ok {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				return trimmed
			}
		}
	}
	switch status {
	case "accepted":
		return "项目交付已通过验收"
	case "needs_more_evidence":
		return "验收未通过,需要补充证据后重新交付"
	default:
		return "验收未通过,项目退回返工"
	}
}

func acceptanceSummary(status string) string {
	switch status {
	case "accepted":
		return "项目验收通过,已归档"
	case "needs_more_evidence":
		return "项目验收需补充证据,已退回"
	default:
		return "项目验收未通过,已退回返工"
	}
}

func (s *ProjectStore) routeReviewTargetUserID(ctx context.Context, input RequestRouteDecisionReviewInput, projectRecord project.Project) (uuid.UUID, error) {
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, input.DemandID)
	if err != nil {
		return uuid.Nil, err
	}
	if demand.ProjectID != input.ProjectID {
		return uuid.Nil, project.ErrProjectNotFound
	}
	if demand.ReviewerPreference != nil && demand.ReviewerPreference.ReviewerUserID != uuid.Nil {
		return demand.ReviewerPreference.ReviewerUserID, nil
	}
	return projectRecord.HumanOwnerUserID, nil
}

func (s *ProjectStore) AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error) {
	if s.repository == nil {
		return ProjectEventResult{}, ErrActivityStoreRequired
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventType(input.EventType), "project_coordinator", input.Summary, map[string]any{}))
	if err != nil {
		return ProjectEventResult{}, err
	}
	return ProjectEventResult{ID: event.ID}, nil
}

func (s *ProjectStore) ensureCoordinatorProjectEvent(ctx context.Context, tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID, summary string, payload map[string]any) (project.ProjectEvent, error) {
	existing, err := s.repository.GetProjectEventByTypeAndActor(ctx, tenantID, projectID, eventType, actorID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, project.ErrProjectNotFound) {
		return project.ProjectEvent{}, err
	}
	return s.repository.AppendProjectEvent(ctx, coordinatorEvent(tenantID, projectID, eventType, actorID, summary, payload))
}

func (s *ProjectStore) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	if s.repository == nil || s.runStarter == nil {
		return ErrActivityStoreRequired
	}
	task, err := s.repository.GetProjectTask(ctx, input.TenantID, input.TaskID)
	if err != nil {
		return err
	}
	if task.ProjectID != input.ProjectID {
		return s.recordDispatchFailure(ctx, input.TenantID, task.ProjectID, task, project.ErrProjectNotFound)
	}
	if task.DigitalEmployeeRunID != nil {
		if task.RuntimeTaskID == nil {
			return s.recordDispatchFailure(ctx, input.TenantID, task.ProjectID, task, project.ErrInvalidProject)
		}
		exists, err := s.repository.ProjectTaskEventExists(ctx, input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String())
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
		if err != nil {
			return err
		}
		_, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String(), "项目任务已分派", reemittedDispatchedPayload(task, projectRecord)))
		return err
	}
	if !projectTaskDispatchAllowed(task.Status) || task.AssignedDigitalEmployeeID == nil || task.DemandID == nil {
		return s.recordDispatchFailure(ctx, input.TenantID, task.ProjectID, task, project.ErrInvalidProject)
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return err
	}
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, *task.DemandID)
	if err != nil {
		return err
	}
	run, err := s.runStarter.StartProjectTaskRun(ctx, StartProjectTaskRunRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		DemandID:          demand.ID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		DispatchUserID:    projectRecord.HumanOwnerUserID,
		Objective:         task.Title,
		Prompt:            projectTaskRunPrompt(projectRecord, demand, task),
		IdempotencyKey:    projectTaskDispatchIdempotencyKey(task.ID),
		Metadata: map[string]any{
			"source":             "project_task_dispatch",
			"actor_type":         "project_coordinator",
			"project_id":         input.ProjectID.String(),
			"demand_id":          demand.ID.String(),
			"project_task_id":    task.ID.String(),
			"expected_outputs":   append([]any(nil), task.ExpectedOutputs...),
			"input_requirements": cloneAnyMap(task.InputRequirements),
			"handoff_contract":   projectTaskDispatchHandoffContract(task.HandoffContract),
		},
	})
	if err != nil {
		return s.recordDispatchFailure(ctx, input.TenantID, input.ProjectID, task, err)
	}
	if _, err := s.repository.BindProjectTaskRun(ctx, project.BindProjectTaskRunRequest{
		TenantID:             input.TenantID,
		ProjectTaskID:        input.TaskID,
		DigitalEmployeeRunID: run.RunID,
		RuntimeTaskID:        run.RuntimeTaskID,
		CurrentStatuses:      []string{"planned", "pending"},
	}); err != nil {
		return s.recordDispatchFailure(ctx, input.TenantID, input.ProjectID, task, err)
	}
	if _, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String(), "项目任务已分派", map[string]any{
		"project_task_id":         input.TaskID.String(),
		"digital_employee_id":     task.AssignedDigitalEmployeeID.String(),
		"digital_employee_run_id": run.RunID.String(),
		"runtime_task_id":         run.RuntimeTaskID.String(),
		"runtime_node_id":         run.RuntimeNodeID.String(),
		"node_id":                 run.NodeID,
		"dispatch_actor_type":     "project_coordinator",
		"dispatch_user_id":        projectRecord.HumanOwnerUserID.String(),
	})); err != nil {
		return err
	}
	return s.repository.AdvanceProjectDemandStatus(ctx, input.TenantID, input.ProjectID, demand.ID, project.ProjectDemandStatusExecuting)
}

func (s *ProjectStore) recordDispatchFailure(ctx context.Context, tenantID, projectID uuid.UUID, task project.ProjectTask, dispatchErr error) error {
	if _, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(tenantID, projectID, project.ProjectEventTaskDispatchFailed, task.ID.String(), "项目任务分派失败", dispatchFailurePayload(task, dispatchErr, dispatchErrorRetryable(dispatchErr)))); err != nil {
		return err
	}
	return &ProjectTaskDispatchError{FailureRecorded: true, Err: dispatchErr}
}

func projectTaskDispatchAllowed(status string) bool {
	return status == "planned" || status == "pending"
}

func projectTaskDispatchIdempotencyKey(taskID uuid.UUID) string {
	return "project-task:" + taskID.String()
}

func projectTaskRunPrompt(projectRecord project.Project, demand project.ProjectDemand, task project.ProjectTask) string {
	content := ""
	if demand.Content != nil {
		content = *demand.Content
	}
	summary := ""
	if task.Summary != nil {
		summary = *task.Summary
	}
	return "项目任务执行请求\n" +
		"项目ID: " + projectRecord.ID.String() + "\n" +
		"需求ID: " + demand.ID.String() + "\n" +
		"ProjectTask ID: " + task.ID.String() + "\n" +
		"需求标题: " + demand.Title + "\n" +
		"需求内容: " + content + "\n" +
		"任务标题: " + task.Title + "\n" +
		"任务摘要: " + summary + "\n" +
		"expected_outputs: " + taskContractJSON(task.ExpectedOutputs) + "\n" +
		"input_requirements: " + taskContractJSON(task.InputRequirements) + "\n" +
		"handoff_contract: " + taskContractJSON(task.HandoffContract) + "\n" +
		"请按项目任务要求执行，并直接输出结论、证据、工件引用和不确定性。" +
		"你只需要给出最终答案；Runtime Agent 会在本轮结束后记录该答案。"
}

func taskContractJSON(value any) string {
	if value == nil {
		return "null"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func dispatchFailurePayload(task project.ProjectTask, err error, retryable bool) map[string]any {
	digitalEmployeeID := ""
	if task.AssignedDigitalEmployeeID != nil {
		digitalEmployeeID = task.AssignedDigitalEmployeeID.String()
	}
	return map[string]any{
		"project_task_id":     task.ID.String(),
		"digital_employee_id": digitalEmployeeID,
		"error":               err.Error(),
		"error_family":        "project_task_dispatch",
		"retryable":           retryable,
		"dispatch_actor_type": "project_coordinator",
	}
}

func dispatchErrorRetryable(err error) bool {
	switch {
	case errors.Is(err, project.ErrProjectNotFound),
		errors.Is(err, project.ErrInvalidProject),
		errors.Is(err, project.ErrProjectConflict):
		return false
	}
	var startErr *ProjectTaskRunStartError
	if errors.As(err, &startErr) {
		return startErr.Retryable
	}
	return true
}

func reemittedDispatchedPayload(task project.ProjectTask, projectRecord project.Project) map[string]any {
	payload := map[string]any{
		"project_task_id":     task.ID.String(),
		"dispatch_actor_type": "project_coordinator",
		"dispatch_user_id":    projectRecord.HumanOwnerUserID.String(),
		"reemitted":           true,
	}
	if task.AssignedDigitalEmployeeID != nil {
		payload["digital_employee_id"] = task.AssignedDigitalEmployeeID.String()
	}
	if task.DigitalEmployeeRunID != nil {
		payload["digital_employee_run_id"] = task.DigitalEmployeeRunID.String()
	}
	if task.RuntimeTaskID != nil {
		payload["runtime_task_id"] = task.RuntimeTaskID.String()
	}
	return payload
}

func (s *ProjectStore) FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error {
	if s.repository == nil {
		return ErrActivityStoreRequired
	}
	outputEventIDs := make([]any, 0, len(input.OutputEventIDs))
	for _, id := range input.OutputEventIDs {
		outputEventIDs = append(outputEventIDs, id.String())
	}
	_, err := s.repository.FinishCoordinationJob(ctx, project.FinishCoordinationJobRequest{
		TenantID:       input.TenantID,
		ID:             input.JobID,
		Status:         input.Status,
		OutputEventIDs: outputEventIDs,
	})
	return err
}

func coordinatorEvent(tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID, summary string, payload map[string]any) project.AppendProjectEventRequest {
	if actorID == "" {
		actorID = "project_coordinator"
	}
	return project.AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: eventType,
		ActorType: "project_coordinator",
		ActorID:   actorID,
		Summary:   summary,
		Payload:   payload,
	}
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

type routeDecisionAggregate struct {
	CandidateDigitalEmployeeIDs []uuid.UUID
	SelectedDigitalEmployeeIDs  []uuid.UUID
	ExpectedOutputs             []string
	InputRequirements           map[string]any
}

func aggregateRouteDecisionFields(plan RouteDecisionPlan) routeDecisionAggregate {
	selected := make([]uuid.UUID, 0, len(plan.Tasks))
	seenEmployees := map[uuid.UUID]struct{}{}
	expectedOutputs := make([]string, 0)
	seenOutputs := map[string]struct{}{}
	taskSummaries := make([]any, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if task.SelectedEmployeeID != uuid.Nil {
			if _, seen := seenEmployees[task.SelectedEmployeeID]; !seen {
				seenEmployees[task.SelectedEmployeeID] = struct{}{}
				selected = append(selected, task.SelectedEmployeeID)
			}
		}
		for _, output := range task.ExpectedOutputs {
			output = strings.TrimSpace(output)
			if output == "" {
				continue
			}
			if _, seen := seenOutputs[output]; seen {
				continue
			}
			seenOutputs[output] = struct{}{}
			expectedOutputs = append(expectedOutputs, output)
		}
		taskSummaries = append(taskSummaries, aggregateTaskInputSummary(task))
	}
	return routeDecisionAggregate{
		CandidateDigitalEmployeeIDs: selected,
		SelectedDigitalEmployeeIDs:  selected,
		ExpectedOutputs:             expectedOutputs,
		InputRequirements:           map[string]any{"tasks": taskSummaries},
	}
}

func aggregateTaskInputSummary(task PlannedTask) map[string]any {
	summary := map[string]any{
		"key":                          task.Key,
		"title":                        task.Title,
		"selected_digital_employee_id": task.SelectedEmployeeID.String(),
		"expected_outputs":             stringsToAny(task.ExpectedOutputs),
		"input_requirement_keys":       stringsToAny(sortedMapKeys(task.InputRequirements)),
	}
	if task.TaskKind != "" {
		summary["task_kind"] = task.TaskKind
	}
	if task.StageIndex != nil {
		summary["stage_index"] = *task.StageIndex
	}
	if task.RiskLevel != "" {
		summary["risk_level"] = task.RiskLevel
	}
	if task.RequiresHumanApproval {
		summary["requires_human_approval"] = true
	}
	if len(task.BlockedByKeys) > 0 {
		summary["blocked_by_keys"] = stringsToAny(task.BlockedByKeys)
	}
	return summary
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func uuidStrings(values []uuid.UUID) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value.String())
	}
	return result
}

func routeReviewContext(input RequestRouteDecisionReviewInput) map[string]any {
	aggregated := aggregateRouteDecisionFields(input.Decision)
	return map[string]any{
		"project_id":                    input.ProjectID.String(),
		"demand_id":                     input.DemandID.String(),
		"coordination_job_id":           input.CoordinationJobID.String(),
		"route_decision_id":             input.RouteDecisionID.String(),
		"project_task_ids":              uuidStrings(input.ProjectTaskIDs),
		"selected_digital_employee_ids": uuidStrings(aggregated.SelectedDigitalEmployeeIDs),
		"reason":                        input.Decision.Reason,
		"route_created_event_id":        input.RouteCreatedEventID.String(),
	}
}

func recoveryReplacementTaskKey(source project.ProjectTask) string {
	base := source.ID.String()
	if source.PlannedTaskKey != nil && strings.TrimSpace(*source.PlannedTaskKey) != "" {
		base = strings.TrimSpace(*source.PlannedTaskKey)
	}
	if strings.HasSuffix(base, "#1") {
		base = strings.TrimSuffix(base, "#1")
	}
	key := base + "#2"
	if len(key) <= 100 {
		return key
	}
	return source.ID.String()[:8] + "#2"
}

func recoveryReplacementTitle(title string) string {
	if strings.Contains(title, "重试") {
		return title
	}
	return title + "（重试）"
}

func recoveryPlannerMetadata(source project.ProjectTask, decisionRequestID uuid.UUID, action FailureRecoveryAction) map[string]any {
	metadata := cloneAnyMap(source.PlannerMetadata)
	metadata["source_task_id"] = source.ID.String()
	metadata["decision_request_id"] = decisionRequestID.String()
	metadata["recovery_action"] = action.Action
	if source.CoordinationJobID != nil {
		metadata["parent_coordination_job_id"] = source.CoordinationJobID.String()
	}
	if action.NewDigitalEmployeeID != nil {
		metadata["new_digital_employee_id"] = action.NewDigitalEmployeeID.String()
	}
	return metadata
}

func dependencyExists(dependencies []project.ProjectTaskDependency, dependentTaskID, blockerTaskID uuid.UUID) bool {
	for _, dependency := range dependencies {
		if dependency.DependentTaskID == dependentTaskID && dependency.BlockerTaskID == blockerTaskID {
			return true
		}
	}
	return false
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// projectTaskDispatchHandoffContract clones a task's handoff contract and forces
// completion_path to "project_task_writeback". The runtime agent gates project-task
// completion writeback on this exact value; the planner does not always emit it, so
// the control-plane enforces it at dispatch to guarantee the run completes the task.
func projectTaskDispatchHandoffContract(contract map[string]any) map[string]any {
	cloned := cloneAnyMap(contract)
	cloned["completion_path"] = "project_task_writeback"
	return cloned
}

func failureRecoveryContext(input HoldDownstreamForFailureInput, downstreamTaskIDs []uuid.UUID) map[string]any {
	return map[string]any{
		"project_id":          input.ProjectID.String(),
		"failed_task_id":      input.FailedTaskID.String(),
		"failed_event_id":     input.FailedEventID.String(),
		"failure_summary":     input.FailureSummary,
		"downstream_task_ids": uuidStrings(downstreamTaskIDs),
	}
}

func isRoutableDigitalProjectRole(role project.ProjectRole) bool {
	return role == project.ProjectRoleExecutor || role == project.ProjectRoleReviewer
}
