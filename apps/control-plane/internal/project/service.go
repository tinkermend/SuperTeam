package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repository  Repository
	coordinator CoordinatorSignalClient
	approvals   ApprovalResolver
}

type latestConfigRevisionRepository interface {
	GetLatestConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectConfigRevision, error)
}

type ApprovalResolver interface {
	ResolveApproval(ctx context.Context, req ResolveApprovalRequest) error
}

func NewService(repository Repository) (*Service, error) {
	return NewServiceWithCoordinator(repository, NoopCoordinatorSignalClient{})
}

func NewServiceWithCoordinator(repository Repository, coordinator CoordinatorSignalClient) (*Service, error) {
	return NewServiceWithCoordinatorAndApprovals(repository, coordinator, nil)
}

func NewServiceWithCoordinatorAndApprovals(repository Repository, coordinator CoordinatorSignalClient, approvals ApprovalResolver) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("project repository is required")
	}
	if coordinator == nil {
		coordinator = NoopCoordinatorSignalClient{}
	}
	return &Service{repository: repository, coordinator: coordinator, approvals: approvals}, nil
}

func (s *Service) CreateProject(ctx context.Context, req CreateProjectRequest) (*CreateProjectResult, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Goal = strings.TrimSpace(req.Goal)
	if req.TenantID == uuid.Nil || req.ActorUserID == uuid.Nil || req.HumanOwnerUserID == uuid.Nil || req.Name == "" || req.Goal == "" {
		return nil, ErrInvalidProject
	}
	if err := validateMembers(req.Members); err != nil {
		return nil, err
	}

	projectID := uuid.New()
	workflowID := fmt.Sprintf("project-coordinator:%s", projectID)
	project, err := s.repository.CreateProject(ctx, req, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	members, err := s.repository.ReplaceProjectMembers(ctx, req.TenantID, project.ID, ensureOwnerMember(req))
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: project.ID,
		EventType: ProjectEventCreated,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "项目已创建",
		Payload:   map[string]any{"name": project.Name},
	}); err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: project.ID,
		EventType: ProjectEventConfigChanged,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "项目配置已初始化",
		Payload:   map[string]any{"member_count": len(members)},
	}); err != nil {
		return nil, err
	}
	if err := s.coordinator.EnsureProjectCoordinator(ctx, ProjectCoordinatorSignal{
		TenantID:   req.TenantID,
		ProjectID:  project.ID,
		WorkflowID: project.CoordinationWorkflowID,
	}); err != nil {
		return nil, err
	}

	return &CreateProjectResult{Project: project, Members: members}, nil
}

func (s *Service) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (*Project, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *Service) ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error) {
	if req.TenantID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	return s.repository.ListProjects(ctx, req)
}

func (s *Service) UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (*Project, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.Status == ProjectStatusArchived || project.ArchivedAt != nil {
		return nil, ErrProjectArchived
	}
	if req.Name != "" {
		req.Name = strings.TrimSpace(req.Name)
	}
	if req.Goal != "" {
		req.Goal = strings.TrimSpace(req.Goal)
	}
	if req.Members != nil {
		if err := validateMembers(*req.Members); err != nil {
			return nil, err
		}
	}

	updated, err := s.repository.UpdateProjectConfig(ctx, req)
	if err != nil {
		return nil, err
	}
	if req.Members != nil {
		if _, err := s.repository.ReplaceProjectMembers(ctx, req.TenantID, req.ProjectID, *req.Members); err != nil {
			return nil, err
		}
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventConfigChanged,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "项目配置已更新",
		Payload:   map[string]any{"name": updated.Name},
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.CreateConfigRevision(ctx, req, updated, event.ID); err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalProjectPolicyChanged(ctx, ProjectPolicyChangedSignal{
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		ChangedEventID: event.ID,
		WorkflowID:     updated.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, "ProjectPolicyChanged", "failed", err, map[string]any{
			"changed_event_id": event.ID.String(),
		})
		return nil, err
	}
	return &updated, nil
}

func (s *Service) ArchiveProject(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) (*Project, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.ArchiveProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventArchived,
		ActorType: "human_user",
		ActorID:   actorUserID.String(),
		Summary:   "项目已归档",
		Payload:   map[string]any{"status": string(project.Status)},
	}); err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *Service) ReplaceProjectMembers(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if err := validateMembers(members); err != nil {
		return nil, err
	}
	replaced, err := s.repository.ReplaceProjectMembers(ctx, tenantID, projectID, members)
	if err != nil {
		return nil, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventConfigChanged,
		ActorType: "human_user",
		ActorID:   actorUserID.String(),
		Summary:   "项目成员已更新",
		Payload:   map[string]any{"member_count": len(replaced)},
	})
	if err != nil {
		return nil, err
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	changedMemberIDs := make([]uuid.UUID, 0, len(replaced))
	for _, member := range replaced {
		changedMemberIDs = append(changedMemberIDs, member.ID)
	}
	if err := s.coordinator.SignalProjectMemberChanged(ctx, ProjectMemberChangedSignal{
		TenantID:         tenantID,
		ProjectID:        projectID,
		ChangedMemberIDs: changedMemberIDs,
		ChangedEventID:   event.ID,
		WorkflowID:       project.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, tenantID, projectID, "ProjectMemberChanged", "failed", err, map[string]any{
			"changed_event_id":     event.ID.String(),
			"changed_member_ids":   uuidStrings(changedMemberIDs),
			"changed_member_count": len(changedMemberIDs),
		})
		return nil, err
	}
	return replaced, nil
}

func (s *Service) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	return s.repository.ListProjectMembers(ctx, tenantID, projectID)
}

func (s *Service) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListProjectTasks(ctx, tenantID, projectID, status, limit, offset)
}

func (s *Service) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListProjectEvents(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) SubmitDemand(ctx context.Context, req SubmitProjectDemandRequest) (*ProjectDemand, error) {
	req.Title = strings.TrimSpace(req.Title)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.SubmittedByUserID == uuid.Nil || req.Title == "" {
		return nil, ErrInvalidProject
	}
	if req.SourceType == "" {
		req.SourceType = DemandSourceManual
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.Status == ProjectStatusArchived || project.ArchivedAt != nil {
		return nil, ErrProjectArchived
	}

	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventDemandSubmitted,
		ActorType: "human_user",
		ActorID:   req.SubmittedByUserID.String(),
		Summary:   "需求已提交到当前项目",
		Payload:   map[string]any{"title": req.Title},
	})
	if err != nil {
		return nil, err
	}
	demand, err := s.repository.CreateProjectDemand(ctx, req, ProjectDemandStatusPlanningPending, &event.ID)
	if err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalDemandSubmitted(ctx, DemandSubmittedSignal{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		DemandID:          demand.ID,
		SubmittedByUserID: req.SubmittedByUserID,
		CreatedEventID:    event.ID,
		WorkflowID:        project.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, "DemandSubmitted", "failed", err, map[string]any{
			"demand_id":        demand.ID.String(),
			"created_event_id": event.ID.String(),
		})
		return nil, err
	}
	return &demand, nil
}

func (s *Service) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListProjectDemands(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListRouteDecisions(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListCoordinationJobs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListDecisionRequests(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListExecutionSummaries(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListTransferRequests(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) CompleteProjectTask(ctx context.Context, req CompleteProjectTaskRequest) (*ExecutionSummary, error) {
	req.Conclusion = strings.TrimSpace(req.Conclusion)
	if req.TenantID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil || req.Conclusion == "" {
		return nil, ErrInvalidProject
	}
	task, projectRecord, err := s.taskAndProjectForWriteback(ctx, req.TenantID, req.RuntimeNodeID, req.ProjectTaskID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.CompleteProjectTaskWriteback(ctx, CompleteProjectTaskWritebackRequest{
		Task: task,
		Summary: CreateExecutionSummaryRequest{
			TenantID:              req.TenantID,
			ProjectID:             task.ProjectID,
			ProjectTaskID:         task.ID,
			DigitalEmployeeID:     req.DigitalEmployeeID,
			Conclusion:            req.Conclusion,
			EvidenceRefs:          sliceOrEmptyAny(req.EvidenceRefs),
			ArtifactRefs:          sliceOrEmptyAny(req.ArtifactRefs),
			ConfidenceFactors:     mapOrEmptyAny(req.ConfidenceFactors),
			Uncertainty:           strings.TrimSpace(req.Uncertainty),
			MissingInformation:    sliceOrEmptyAny(req.MissingInformation),
			RecommendedNextAction: strings.TrimSpace(req.RecommendedNextAction),
			RequiresHumanReview:   req.RequiresHumanReview,
		},
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    task.ProjectID,
			EventType:    ProjectEventTaskCompleted,
			ActorType:    "digital_employee",
			ActorID:      req.DigitalEmployeeID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(task.ID.String()),
			Summary:      "项目任务已完成",
			Payload: map[string]any{
				"project_task_id": task.ID.String(),
			},
		},
		AllowedCurrentStatuses: runtimeWritebackProjectTaskStatuses(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalEmployeeTaskCompleted(ctx, EmployeeTaskCompletedSignal{
		TenantID:           req.TenantID,
		ProjectID:          task.ProjectID,
		ProjectTaskID:      task.ID,
		ExecutionSummaryID: result.Summary.ID,
		CompletedEventID:   result.Event.ID,
		WorkflowID:         projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTaskCompleted", "failed", err, map[string]any{
			"project_task_id":      task.ID.String(),
			"execution_summary_id": result.Summary.ID.String(),
			"completed_event_id":   result.Event.ID.String(),
		})
		return nil, err
	}
	return &result.Summary, nil
}

func (s *Service) FailProjectTask(ctx context.Context, req FailProjectTaskRequest) (*ProjectTask, error) {
	req.FailureSummary = strings.TrimSpace(req.FailureSummary)
	if req.TenantID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil || req.FailureSummary == "" {
		return nil, ErrInvalidProject
	}
	task, projectRecord, err := s.taskAndProjectForWriteback(ctx, req.TenantID, req.RuntimeNodeID, req.ProjectTaskID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.FailProjectTaskWriteback(ctx, FailProjectTaskWritebackRequest{
		Task: task,
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    task.ProjectID,
			EventType:    ProjectEventTaskFailed,
			ActorType:    "digital_employee",
			ActorID:      req.DigitalEmployeeID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(task.ID.String()),
			Summary:      "项目任务执行失败",
			Payload: map[string]any{
				"project_task_id": task.ID.String(),
				"failure_summary": req.FailureSummary,
			},
		},
		AllowedCurrentStatuses: runtimeWritebackProjectTaskStatuses(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalEmployeeTaskFailed(ctx, EmployeeTaskFailedSignal{
		TenantID:       req.TenantID,
		ProjectID:      task.ProjectID,
		ProjectTaskID:  task.ID,
		FailureSummary: req.FailureSummary,
		FailedEventID:  result.Event.ID,
		WorkflowID:     projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTaskFailed", "failed", err, map[string]any{
			"project_task_id": task.ID.String(),
			"failed_event_id": result.Event.ID.String(),
			"failure_summary": req.FailureSummary,
		})
		return nil, err
	}
	return &result.Task, nil
}

func (s *Service) RequestProjectTaskTransfer(ctx context.Context, req RequestProjectTaskTransferRequest) (*TransferRequest, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	if req.TenantID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil || req.Reason == "" {
		return nil, ErrInvalidProject
	}
	task, projectRecord, err := s.taskAndProjectForWriteback(ctx, req.TenantID, req.RuntimeNodeID, req.ProjectTaskID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.RequestProjectTaskTransferWriteback(ctx, RequestProjectTaskTransferWritebackRequest{
		Task: task,
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    task.ProjectID,
			EventType:    ProjectEventTransferRequested,
			ActorType:    "digital_employee",
			ActorID:      req.DigitalEmployeeID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(task.ID.String()),
			Summary:      "数字员工请求转派",
			Payload:      map[string]any{"project_task_id": task.ID.String(), "reason": req.Reason},
		},
		Transfer: CreateTransferRequestRequest{
			TenantID:                     req.TenantID,
			ProjectID:                    task.ProjectID,
			ProjectTaskID:                task.ID,
			RequestedByDigitalEmployeeID: req.DigitalEmployeeID,
			Reason:                       req.Reason,
			SuggestedEmployeeType:        strings.TrimSpace(req.SuggestedEmployeeType),
			SuggestedDigitalEmployeeIDs:  req.SuggestedDigitalEmployeeIDs,
			MissingContextRefs:           sliceOrEmptyAny(req.MissingContextRefs),
			Status:                       "requested",
		},
		AllowedCurrentStatuses: runtimeWritebackProjectTaskStatuses(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalEmployeeTransferRequested(ctx, EmployeeTransferRequestedSignal{
		TenantID:          req.TenantID,
		ProjectID:         task.ProjectID,
		ProjectTaskID:     task.ID,
		TransferRequestID: result.Transfer.ID,
		RequestedEventID:  result.Event.ID,
		WorkflowID:        projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTransferRequested", "failed", err, map[string]any{
			"project_task_id":     task.ID.String(),
			"transfer_request_id": result.Transfer.ID.String(),
			"requested_event_id":  result.Event.ID.String(),
		})
		return nil, err
	}
	return &result.Transfer, nil
}

func (s *Service) ResolveDecision(ctx context.Context, req ResolveDecisionRequest) (*DecisionRequest, error) {
	req.Decision = strings.TrimSpace(req.Decision)
	req.Comment = strings.TrimSpace(req.Comment)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.DecisionRequestID == uuid.Nil || req.DecidedByUserID == uuid.Nil || !validHumanDecision(req.Decision) {
		return nil, ErrInvalidProject
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	decision, err := s.findDecisionRequest(ctx, req.TenantID, req.ProjectID, req.DecisionRequestID)
	if err != nil {
		return nil, err
	}
	if s.approvals != nil {
		if err := s.approvals.ResolveApproval(ctx, ResolveApprovalRequest{
			TenantID:          req.TenantID,
			ApprovalRequestID: decision.ApprovalRequestID,
			DecidedByUserID:   req.DecidedByUserID,
			Decision:          req.Decision,
			Comment:           req.Comment,
			Payload:           mapOrEmptyAny(req.Payload),
		}); err != nil {
			return nil, err
		}
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    req.ProjectID,
		EventType:    ProjectEventDecisionSubmitted,
		ActorType:    "human_user",
		ActorID:      req.DecidedByUserID.String(),
		ResourceType: strPtr("decision_request"),
		ResourceID:   strPtr(req.DecisionRequestID.String()),
		Summary:      "人类决策已提交",
		Payload:      map[string]any{"decision": req.Decision, "comment": req.Comment},
	})
	if err != nil {
		return nil, err
	}
	resolved, err := s.repository.ResolveDecisionRequest(ctx, ResolveDecisionRequestRepositoryRequest{
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ID:              req.DecisionRequestID,
		StatusSnapshot:  req.Decision,
		ResolvedEventID: &event.ID,
	})
	if err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalHumanDecisionSubmitted(ctx, HumanDecisionSubmittedSignal{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: decision.ApprovalRequestID,
		DecisionRequestID: req.DecisionRequestID,
		Decision:          req.Decision,
		ResolvedEventID:   event.ID,
		WorkflowID:        projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, "HumanDecisionSubmitted", "failed", err, map[string]any{
			"approval_request_id": decision.ApprovalRequestID.String(),
			"decision_request_id": req.DecisionRequestID.String(),
			"resolved_event_id":   event.ID.String(),
			"decision":            req.Decision,
		})
		return nil, err
	}
	return &resolved, nil
}

func (s *Service) RetryWorkflowSignal(ctx context.Context, req RetryWorkflowSignalRequest) (*ProjectEvent, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.EventID == uuid.Nil || req.ActorID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	event, err := s.repository.GetProjectEvent(ctx, req.TenantID, req.ProjectID, req.EventID)
	if err != nil {
		return nil, err
	}
	if event.EventType != ProjectEventWorkflowSignaled {
		return nil, ErrInvalidProject
	}
	signalName, _ := event.Payload["signal_name"].(string)
	status, _ := event.Payload["status"].(string)
	retryable, _ := event.Payload["retryable"].(bool)
	if signalName == "" || status != "failed" || !retryable {
		return nil, ErrInvalidProject
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if err := s.retryWorkflowSignal(ctx, projectRecord, signalName, event.Payload); err != nil {
		retryPayload := cloneMap(event.Payload)
		retryPayload["retry_of_event_id"] = req.EventID.String()
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, signalName, "failed", err, retryPayload)
		return nil, err
	}
	retryEvent, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventWorkflowSignaled,
		ActorType: "human_user",
		ActorID:   req.ActorID.String(),
		Summary:   "Workflow signal 已重试",
		Payload: map[string]any{
			"signal_name":       signalName,
			"status":            "sent",
			"retryable":         false,
			"retry_of_event_id": req.EventID.String(),
		},
	})
	if err != nil {
		return nil, err
	}
	return &retryEvent, nil
}

func (s *Service) retryWorkflowSignal(ctx context.Context, projectRecord Project, signalName string, payload map[string]any) error {
	switch signalName {
	case "DemandSubmitted":
		demandID, err := uuidFromPayload(payload, "demand_id")
		if err != nil {
			return err
		}
		demand, err := s.repository.GetProjectDemand(ctx, projectRecord.TenantID, demandID)
		if err != nil {
			return err
		}
		if demand.CreatedEventID == nil {
			return ErrInvalidProject
		}
		return s.coordinator.SignalDemandSubmitted(ctx, DemandSubmittedSignal{
			TenantID:          projectRecord.TenantID,
			ProjectID:         projectRecord.ID,
			DemandID:          demand.ID,
			SubmittedByUserID: demand.SubmittedByUserID,
			CreatedEventID:    *demand.CreatedEventID,
			WorkflowID:        projectRecord.CoordinationWorkflowID,
		})
	case "ProjectPolicyChanged":
		changedEventID, err := uuidFromPayload(payload, "changed_event_id")
		if err != nil {
			return err
		}
		return s.coordinator.SignalProjectPolicyChanged(ctx, ProjectPolicyChangedSignal{
			TenantID:       projectRecord.TenantID,
			ProjectID:      projectRecord.ID,
			ChangedEventID: changedEventID,
			WorkflowID:     projectRecord.CoordinationWorkflowID,
		})
	case "ProjectMemberChanged":
		changedEventID, err := uuidFromPayload(payload, "changed_event_id")
		if err != nil {
			return err
		}
		return s.coordinator.SignalProjectMemberChanged(ctx, ProjectMemberChangedSignal{
			TenantID:         projectRecord.TenantID,
			ProjectID:        projectRecord.ID,
			ChangedMemberIDs: uuidSliceFromPayload(payload, "changed_member_ids"),
			ChangedEventID:   changedEventID,
			WorkflowID:       projectRecord.CoordinationWorkflowID,
		})
	case "EmployeeTaskCompleted":
		projectTaskID, err := uuidFromPayload(payload, "project_task_id")
		if err != nil {
			return err
		}
		executionSummaryID, err := uuidFromPayload(payload, "execution_summary_id")
		if err != nil {
			return err
		}
		completedEventID, err := uuidFromPayload(payload, "completed_event_id")
		if err != nil {
			return err
		}
		return s.coordinator.SignalEmployeeTaskCompleted(ctx, EmployeeTaskCompletedSignal{
			TenantID:           projectRecord.TenantID,
			ProjectID:          projectRecord.ID,
			ProjectTaskID:      projectTaskID,
			ExecutionSummaryID: executionSummaryID,
			CompletedEventID:   completedEventID,
			WorkflowID:         projectRecord.CoordinationWorkflowID,
		})
	case "EmployeeTaskFailed":
		projectTaskID, err := uuidFromPayload(payload, "project_task_id")
		if err != nil {
			return err
		}
		failedEventID, err := uuidFromPayload(payload, "failed_event_id")
		if err != nil {
			return err
		}
		failureSummary, _ := payload["failure_summary"].(string)
		return s.coordinator.SignalEmployeeTaskFailed(ctx, EmployeeTaskFailedSignal{
			TenantID:       projectRecord.TenantID,
			ProjectID:      projectRecord.ID,
			ProjectTaskID:  projectTaskID,
			FailureSummary: failureSummary,
			FailedEventID:  failedEventID,
			WorkflowID:     projectRecord.CoordinationWorkflowID,
		})
	case "EmployeeTransferRequested":
		projectTaskID, err := uuidFromPayload(payload, "project_task_id")
		if err != nil {
			return err
		}
		transferRequestID, err := uuidFromPayload(payload, "transfer_request_id")
		if err != nil {
			return err
		}
		requestedEventID, err := uuidFromPayload(payload, "requested_event_id")
		if err != nil {
			return err
		}
		return s.coordinator.SignalEmployeeTransferRequested(ctx, EmployeeTransferRequestedSignal{
			TenantID:          projectRecord.TenantID,
			ProjectID:         projectRecord.ID,
			ProjectTaskID:     projectTaskID,
			TransferRequestID: transferRequestID,
			RequestedEventID:  requestedEventID,
			WorkflowID:        projectRecord.CoordinationWorkflowID,
		})
	case "HumanDecisionSubmitted":
		approvalRequestID, err := uuidFromPayload(payload, "approval_request_id")
		if err != nil {
			return err
		}
		decisionRequestID, err := uuidFromPayload(payload, "decision_request_id")
		if err != nil {
			return err
		}
		resolvedEventID, err := uuidFromPayload(payload, "resolved_event_id")
		if err != nil {
			return err
		}
		decision, _ := payload["decision"].(string)
		return s.coordinator.SignalHumanDecisionSubmitted(ctx, HumanDecisionSubmittedSignal{
			TenantID:          projectRecord.TenantID,
			ProjectID:         projectRecord.ID,
			ApprovalRequestID: approvalRequestID,
			DecisionRequestID: decisionRequestID,
			Decision:          decision,
			ResolvedEventID:   resolvedEventID,
			WorkflowID:        projectRecord.CoordinationWorkflowID,
		})
	default:
		return ErrInvalidProject
	}
}

func (s *Service) GetLatestProjectConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectConfigRevision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	repository, ok := s.repository.(latestConfigRevisionRepository)
	if !ok {
		return nil, fmt.Errorf("project repository does not support latest config revision")
	}
	revision, err := repository.GetLatestConfigRevision(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &revision, nil
}

func (s *Service) GetProjectOverview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectOverview, error) {
	return s.GetOverview(ctx, tenantID, projectID)
}

func (s *Service) GetOverview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectOverview, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	members, err := s.repository.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	limit, offset := normalizePagination(20, 0)
	tasks, err := s.repository.ListProjectTasks(ctx, tenantID, projectID, nil, limit, offset)
	if err != nil {
		return nil, err
	}
	events, err := s.repository.ListProjectEvents(ctx, tenantID, projectID, limit, offset)
	if err != nil {
		return nil, err
	}

	overview := ProjectOverview{
		Project: project,
		StatusSummary: ProjectStatusSummary{
			CurrentPhase: string(project.Status),
			IsArchived:   project.Status == ProjectStatusArchived || project.ArchivedAt != nil,
		},
		ActiveTasks:  tasks,
		RecentEvents: events,
		CoordinationWorkflow: ProjectCoordinationWorkflow{
			WorkflowID: project.CoordinationWorkflowID,
			Status:     project.CoordinationStatus,
		},
	}
	for _, member := range members {
		switch member.PrincipalType {
		case PrincipalTypeHumanUser:
			overview.HumanRoles = append(overview.HumanRoles, member)
		case PrincipalTypeDigitalEmployee:
			overview.DigitalEmployeePool = append(overview.DigitalEmployeePool, member)
		}
	}
	for _, task := range tasks {
		switch task.Status {
		case "completed":
			overview.TaskSummary.CompletedTasks++
		case "failed":
			overview.TaskSummary.FailedTasks++
		case "waiting_human":
			overview.TaskSummary.PendingHumanTasks++
			overview.TaskSummary.ActiveTasks++
		default:
			overview.TaskSummary.ActiveTasks++
		}
	}
	return &overview, nil
}

func (s *Service) taskAndProjectForWriteback(ctx context.Context, tenantID, runtimeNodeID, projectTaskID, digitalEmployeeID uuid.UUID) (ProjectTask, Project, error) {
	if runtimeNodeID == uuid.Nil {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	task, err := s.repository.GetProjectTask(ctx, tenantID, projectTaskID)
	if err != nil {
		return ProjectTask{}, Project{}, err
	}
	if task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID != digitalEmployeeID {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	if task.DigitalEmployeeRunID == nil {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	runRepository, ok := s.repository.(ProjectTaskRuntimeBindingRepository)
	if !ok {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	taskRuntimeNodeID, err := runRepository.GetProjectTaskRunRuntimeNodeID(ctx, tenantID, task.ID, *task.DigitalEmployeeRunID)
	if err != nil {
		return ProjectTask{}, Project{}, err
	}
	if taskRuntimeNodeID != runtimeNodeID {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	if !projectTaskAcceptsRuntimeWriteback(task.Status) {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	projectRecord, err := s.repository.GetProject(ctx, tenantID, task.ProjectID)
	if err != nil {
		return ProjectTask{}, Project{}, err
	}
	return task, projectRecord, nil
}

func projectTaskAcceptsRuntimeWriteback(status string) bool {
	switch status {
	case "assigned", "running":
		return true
	default:
		return false
	}
}

func runtimeWritebackProjectTaskStatuses() []string {
	return []string{"assigned", "running"}
}

func (s *Service) projectTaskWritebackRepository() (ProjectTaskWritebackRepository, error) {
	repository, ok := s.repository.(ProjectTaskWritebackRepository)
	if !ok {
		return nil, fmt.Errorf("project repository does not support atomic project task writeback")
	}
	return repository, nil
}

func (s *Service) findDecisionRequest(ctx context.Context, tenantID, projectID, decisionID uuid.UUID) (DecisionRequest, error) {
	return s.repository.GetDecisionRequest(ctx, tenantID, projectID, decisionID)
}

func (s *Service) appendWorkflowSignalEvent(ctx context.Context, tenantID, projectID uuid.UUID, signalName, status string, signalErr error, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["signal_name"] = signalName
	payload["status"] = status
	payload["retryable"] = signalErr != nil
	if signalErr != nil {
		payload["error"] = signalErr.Error()
	}
	_, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventWorkflowSignaled,
		ActorType: "control_plane",
		ActorID:   "project_service",
		Summary:   "Workflow signal 状态已记录",
		Payload:   payload,
	})
	return err
}

func uuidFromPayload(payload map[string]any, key string) (uuid.UUID, error) {
	value, ok := payload[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return uuid.Nil, ErrInvalidProject
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, ErrInvalidProject
	}
	return id, nil
}

func uuidSliceFromPayload(payload map[string]any, key string) []uuid.UUID {
	switch raw := payload[key].(type) {
	case []string:
		ids := make([]uuid.UUID, 0, len(raw))
		for _, value := range raw {
			id, err := uuid.Parse(value)
			if err == nil {
				ids = append(ids, id)
			}
		}
		return ids
	case []any:
		ids := make([]uuid.UUID, 0, len(raw))
		for _, item := range raw {
			value, ok := item.(string)
			if !ok {
				continue
			}
			id, err := uuid.Parse(value)
			if err == nil {
				ids = append(ids, id)
			}
		}
		return ids
	default:
		return nil
	}
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func validHumanDecision(decision string) bool {
	switch decision {
	case "approved", "rejected", "needs_more_evidence":
		return true
	default:
		return false
	}
}

func validateMembers(members []ProjectMemberInput) error {
	for _, member := range members {
		if member.PrincipalID == uuid.Nil {
			return ErrInvalidProjectMember
		}
		if member.ProjectRole == ProjectRole("coordinator") {
			return ErrInvalidProjectMember
		}
		if member.ProjectRole == ProjectRoleExecutor && member.PrincipalType != PrincipalTypeDigitalEmployee {
			return ErrInvalidProjectMember
		}
		if (member.ProjectRole == ProjectRoleOwner || member.ProjectRole == ProjectRoleLeader || member.ProjectRole == ProjectRoleAcceptance) && member.PrincipalType != PrincipalTypeHumanUser {
			return ErrInvalidProjectMember
		}
	}
	return nil
}

func ensureOwnerMember(req CreateProjectRequest) []ProjectMemberInput {
	members := append([]ProjectMemberInput{}, req.Members...)
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == req.HumanOwnerUserID && member.ProjectRole == ProjectRoleOwner {
			return members
		}
	}
	return append(members, ProjectMemberInput{
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   req.HumanOwnerUserID,
		ProjectRole:   ProjectRoleOwner,
	})
}

func normalizePagination(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func strPtr(value string) *string {
	return &value
}

func mapOrEmptyAny(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func sliceOrEmptyAny(value []any) []any {
	if value == nil {
		return []any{}
	}
	return value
}
