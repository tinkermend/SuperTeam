package projectcoordination

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)

type ProjectStore struct {
	repository project.Repository
	approvals  ApprovalCreator
	inbox      project.DecisionInboxProjector
	runStarter ProjectTaskRunStarter
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
	for _, member := range members {
		if member.PrincipalType != project.PrincipalTypeDigitalEmployee || member.Status != "active" || !isRoutableDigitalProjectRole(member.ProjectRole) {
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
	if _, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventCoordinationJobCreated, input.WorkflowID, "协调作业已创建", map[string]any{"coordination_job_id": job.ID.String()})); err != nil {
		return CoordinationJobResult{}, err
	}
	return CoordinationJobResult{ID: job.ID}, nil
}

func (s *ProjectStore) PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error) {
	if s.repository == nil {
		return RouteDecisionResult{}, ErrActivityStoreRequired
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventRouteDecisionCreated, input.JobID.String(), "路由决策已生成", map[string]any{
		"coordination_job_id": input.JobID.String(),
		"demand_id":           input.DemandID.String(),
	}))
	if err != nil {
		return RouteDecisionResult{}, err
	}
	demandID := input.DemandID
	decision, err := s.repository.CreateRouteDecision(ctx, project.CreateRouteDecisionRequest{
		TenantID:                    input.TenantID,
		ProjectID:                   input.ProjectID,
		CoordinationJobID:           input.JobID,
		DemandID:                    &demandID,
		CandidateDigitalEmployeeIDs: input.Decision.CandidateDigitalEmployeeIDs,
		SelectedDigitalEmployeeIDs:  input.Decision.SelectedDigitalEmployeeIDs,
		Reason:                      input.Decision.Reason,
		InputRequirements:           input.Decision.InputRequirements,
		ExpectedOutputs:             stringsToAny(input.Decision.ExpectedOutputs),
		BudgetEstimate:              input.Decision.BudgetEstimate,
		RequiresHumanReview:         input.Decision.RequiresHumanReview,
		CreatedEventID:              &event.ID,
	})
	if err != nil {
		return RouteDecisionResult{}, err
	}
	return RouteDecisionResult{ID: decision.ID, CreatedEventID: event.ID}, nil
}

func (s *ProjectStore) CreateProjectTasks(ctx context.Context, input CreateProjectTasksInput) ([]ProjectTaskResult, error) {
	if s.repository == nil {
		return nil, ErrActivityStoreRequired
	}
	results := make([]ProjectTaskResult, 0, len(input.Decision.SelectedDigitalEmployeeIDs))
	for _, employeeID := range input.Decision.SelectedDigitalEmployeeIDs {
		task, err := s.repository.CreateProjectTask(ctx, project.CreateProjectTaskRequest{
			TenantID:                  input.TenantID,
			ProjectID:                 input.ProjectID,
			DemandID:                  &input.DemandID,
			Title:                     input.Decision.TaskTitle,
			Summary:                   input.Decision.TaskSummary,
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
			RequiresHumanApproval:     input.Decision.RequiresHumanReview,
		})
		if err != nil {
			return nil, err
		}
		if _, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskCreated, task.ID.String(), "项目任务已创建", map[string]any{
			"project_task_id": task.ID.String(),
			"demand_id":       input.DemandID.String(),
		})); err != nil {
			return nil, err
		}
		results = append(results, ProjectTaskResult{ID: task.ID})
	}
	return results, nil
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
			"source":          "project_task_dispatch",
			"actor_type":      "project_coordinator",
			"project_id":      input.ProjectID.String(),
			"demand_id":       demand.ID.String(),
			"project_task_id": task.ID.String(),
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
	_, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String(), "项目任务已分派", map[string]any{
		"project_task_id":         input.TaskID.String(),
		"digital_employee_id":     task.AssignedDigitalEmployeeID.String(),
		"digital_employee_run_id": run.RunID.String(),
		"runtime_task_id":         run.RuntimeTaskID.String(),
		"runtime_node_id":         run.RuntimeNodeID.String(),
		"node_id":                 run.NodeID,
		"dispatch_actor_type":     "project_coordinator",
		"dispatch_user_id":        projectRecord.HumanOwnerUserID.String(),
	}))
	return err
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
		"请按项目任务要求执行，并在完成后通过项目任务回写端点提交结论、证据、工件引用和不确定性。"
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

func uuidStrings(values []uuid.UUID) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value.String())
	}
	return result
}

func routeReviewContext(input RequestRouteDecisionReviewInput) map[string]any {
	return map[string]any{
		"project_id":                    input.ProjectID.String(),
		"demand_id":                     input.DemandID.String(),
		"coordination_job_id":           input.CoordinationJobID.String(),
		"route_decision_id":             input.RouteDecisionID.String(),
		"project_task_ids":              uuidStrings(input.ProjectTaskIDs),
		"selected_digital_employee_ids": uuidStrings(input.Decision.SelectedDigitalEmployeeIDs),
		"reason":                        input.Decision.Reason,
		"route_created_event_id":        input.RouteCreatedEventID.String(),
	}
}

func isRoutableDigitalProjectRole(role project.ProjectRole) bool {
	return role == project.ProjectRoleExecutor || role == project.ProjectRoleReviewer
}
