package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository            Repository
	coordinator           CoordinatorSignalClient
	approvals             ApprovalResolver
	inbox                 DecisionInboxProjector
	archiveArtifactLocker ArchiveArtifactLocker
	teamScopeAuthorizer   ProjectTeamScopeAuthorizer
}

const (
	projectTaskAttemptStartReadinessAttempts = 25
	projectTaskAttemptStartReadinessBackoff  = 200 * time.Millisecond
)

type latestConfigRevisionRepository interface {
	GetLatestConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectConfigRevision, error)
}

type ProjectTeamScopeAuthorizer interface {
	CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error)
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
	return NewServiceWithCoordinatorApprovalsAndArchiveArtifactLocker(repository, coordinator, approvals, nil)
}

func NewServiceWithArchiveArtifactLocker(repository Repository, locker ArchiveArtifactLocker) (*Service, error) {
	return NewServiceWithCoordinatorApprovalsAndArchiveArtifactLocker(repository, NoopCoordinatorSignalClient{}, nil, locker)
}

func NewServiceWithCoordinatorApprovalsAndArchiveArtifactLocker(repository Repository, coordinator CoordinatorSignalClient, approvals ApprovalResolver, locker ArchiveArtifactLocker) (*Service, error) {
	return NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repository, coordinator, approvals, nil, locker)
}

func NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repository Repository, coordinator CoordinatorSignalClient, approvals ApprovalResolver, inbox DecisionInboxProjector, locker ArchiveArtifactLocker) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("project repository is required")
	}
	if coordinator == nil {
		coordinator = NoopCoordinatorSignalClient{}
	}
	teamScopeAuthorizer, _ := repository.(ProjectTeamScopeAuthorizer)
	return &Service{
		repository:            repository,
		coordinator:           coordinator,
		approvals:             approvals,
		inbox:                 inbox,
		archiveArtifactLocker: locker,
		teamScopeAuthorizer:   teamScopeAuthorizer,
	}, nil
}

func (s *Service) SetTeamScopeAuthorizer(authorizer ProjectTeamScopeAuthorizer) {
	if s != nil {
		s.teamScopeAuthorizer = authorizer
	}
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
	if err := s.validateProjectTeamScopes(ctx, req); err != nil {
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

func (s *Service) validateProjectTeamScopes(ctx context.Context, req CreateProjectRequest) error {
	return s.validateProjectTeamScopeAccess(ctx, req.TenantID, req.ActorUserID, req.TeamID, req.Members)
}

func (s *Service) validateProjectTeamScopeAccess(ctx context.Context, tenantID, actorUserID uuid.UUID, projectTeamID *uuid.UUID, members []ProjectMemberInput) error {
	teamIDs := make(map[uuid.UUID]struct{})
	orderedTeamIDs := make([]uuid.UUID, 0)
	addTeamID := func(teamID uuid.UUID) {
		if teamID == uuid.Nil {
			return
		}
		if _, ok := teamIDs[teamID]; ok {
			return
		}
		teamIDs[teamID] = struct{}{}
		orderedTeamIDs = append(orderedTeamIDs, teamID)
	}
	if projectTeamID != nil && *projectTeamID != uuid.Nil {
		addTeamID(*projectTeamID)
	}
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeTeam && member.PrincipalID != uuid.Nil {
			addTeamID(member.PrincipalID)
		}
	}
	if len(teamIDs) == 0 {
		return nil
	}
	if s.teamScopeAuthorizer == nil {
		return ErrUnauthorizedProjectTeamScope
	}
	sort.Slice(orderedTeamIDs, func(i, j int) bool {
		return orderedTeamIDs[i].String() < orderedTeamIDs[j].String()
	})
	for _, teamID := range orderedTeamIDs {
		allowed, err := s.teamScopeAuthorizer.CanUseTeamForProject(ctx, tenantID, actorUserID, teamID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrUnauthorizedProjectTeamScope
		}
	}
	return nil
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

func (s *Service) QueueProjectTask(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return QueueProjectTaskResult{}, ErrInvalidProject
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.LeaseToken = strings.TrimSpace(req.LeaseToken)
	if req.IdempotencyKey == "" || req.LeaseToken == "" {
		return QueueProjectTaskResult{}, ErrInvalidProject
	}
	if len(req.ExecutionContextPacket) == 0 {
		task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		if task.ProjectID != req.ProjectID {
			return QueueProjectTaskResult{}, ErrProjectNotFound
		}
		packet, err := s.BuildProjectTaskExecutionPacket(ctx, task)
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		req.ExecutionContextPacket = projectTaskExecutionPacketMap(packet)
		req.ExecutionContextPacketVersion = packet.Version
	}
	if strings.TrimSpace(req.ExecutionContextPacketVersion) == "" {
		req.ExecutionContextPacketVersion = "v1"
	}
	return s.repository.QueueProjectTaskWithAttempt(ctx, req)
}

func (s *Service) BuildProjectTaskExecutionPacket(ctx context.Context, task ProjectTask) (ProjectTaskExecutionPacket, error) {
	if task.TenantID == uuid.Nil || task.ProjectID == uuid.Nil || task.ID == uuid.Nil {
		return ProjectTaskExecutionPacket{}, ErrInvalidProject
	}
	summary := ""
	if task.Summary != nil {
		summary = *task.Summary
	}
	riskLevel := ""
	if task.RiskLevel != nil {
		riskLevel = *task.RiskLevel
	}
	packet := ProjectTaskExecutionPacket{
		Version:              "v1",
		ProjectID:            task.ProjectID.String(),
		ProjectTaskID:        task.ID.String(),
		Title:                task.Title,
		Summary:              summary,
		ExpectedOutputs:      append([]any(nil), task.ExpectedOutputs...),
		InputRequirements:    cloneMap(mapOrEmptyAny(task.InputRequirements)),
		HandoffContract:      cloneMap(mapOrEmptyAny(task.HandoffContract)),
		ForbiddenScopes:      []string{},
		RiskLevel:            riskLevel,
		StopForHumanCriteria: []string{HumanWaitReasonMissingContext, HumanWaitReasonApprovalRequired, HumanWaitReasonPermissionRequired, HumanWaitReasonPlanInvalid},
	}
	if len(task.BlockedByTaskIDs) > 0 {
		summaries, err := s.repository.ListExecutionSummaries(ctx, task.TenantID, task.ProjectID, 200, 0)
		if err != nil {
			return ProjectTaskExecutionPacket{}, err
		}
		blockedBy := make(map[uuid.UUID]struct{}, len(task.BlockedByTaskIDs))
		for _, blockerID := range task.BlockedByTaskIDs {
			blockedBy[blockerID] = struct{}{}
		}
		for _, summary := range summaries {
			if _, ok := blockedBy[summary.ProjectTaskID]; !ok {
				continue
			}
			packet.DependencyOutputs = append(packet.DependencyOutputs, ProjectTaskDependencyOutput{
				ProjectTaskID: summary.ProjectTaskID.String(),
				Conclusion:    summary.Conclusion,
				EvidenceRefs:  append([]any(nil), summary.EvidenceRefs...),
				ArtifactRefs:  append([]any(nil), summary.ArtifactRefs...),
			})
		}
	}
	decisions, err := s.repository.ListDecisionRequests(ctx, task.TenantID, task.ProjectID, 200, 0)
	if err != nil {
		return ProjectTaskExecutionPacket{}, err
	}
	for _, decision := range decisions {
		if decision.ProjectTaskID == nil || *decision.ProjectTaskID != task.ID {
			continue
		}
		packet.HumanDecisionRefs = append(packet.HumanDecisionRefs, ProjectTaskHumanDecisionRef{
			DecisionRequestID: decision.ID.String(),
			DecisionType:      decision.DecisionType,
			StatusSnapshot:    decision.StatusSnapshot,
		})
	}
	return packet, nil
}

func (s *Service) RecordAttemptContextUpdate(ctx context.Context, req RecordAttemptContextUpdateRequest) (ProjectTaskAttemptContextUpdate, error) {
	req.UpdateKind = strings.TrimSpace(req.UpdateKind)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.UpdateKind == "" || req.Payload == nil {
		return ProjectTaskAttemptContextUpdate{}, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return ProjectTaskAttemptContextUpdate{}, err
	}
	if task.ProjectID != req.ProjectID {
		return ProjectTaskAttemptContextUpdate{}, ErrProjectNotFound
	}
	attemptID := req.AttemptID
	if attemptID == nil {
		attemptID = task.CurrentAttemptID
	}
	deliveryMode := projectTaskContextUpdateDeliveryMode(task, req.UpdateKind)
	return s.repository.RecordProjectTaskAttemptContextUpdate(ctx, RecordProjectTaskAttemptContextUpdateRepositoryRequest{
		TenantID:      req.TenantID,
		ProjectTaskID: req.ProjectTaskID,
		AttemptID:     attemptID,
		UpdateKind:    req.UpdateKind,
		Payload:       cloneMap(req.Payload),
		DeliveryMode:  deliveryMode,
	})
}

func (s *Service) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	if req.TenantID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Limit, req.Offset = normalizeWorkflowInstancePagination(req.Limit, req.Offset)
	items, err := s.repository.ListWorkflowInstances(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Status = normalizeWorkflowInstanceStatus(items[i])
	}
	sort.SliceStable(items, func(i, j int) bool {
		leftRank := workflowInstanceAttentionRank(items[i].Status)
		rightRank := workflowInstanceAttentionRank(items[j].Status)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	if req.Status != nil {
		filtered := make([]WorkflowInstanceSummary, 0, len(items))
		for _, item := range items {
			if item.Status == *req.Status {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return items, nil
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
		if err := s.validateProjectTeamScopeAccess(ctx, req.TenantID, req.ActorUserID, nil, *req.Members); err != nil {
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

func (s *Service) ListProjectTaskLiveness(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskLiveness, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	tasks, err := s.repository.ListProjectTasks(ctx, tenantID, projectID, nil, 1000, 0)
	if err != nil {
		return nil, err
	}
	taskIDs := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
	}
	unresolvedByTask := map[uuid.UUID][]uuid.UUID{}
	if len(taskIDs) > 0 {
		readiness, err := s.repository.ListUnresolvedBlockersForTasks(ctx, tenantID, projectID, taskIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range readiness {
			unresolvedByTask[item.DependentTaskID] = append(unresolvedByTask[item.DependentTaskID], item.BlockerTaskID)
		}
	}
	now := time.Now().UTC()
	items := make([]ProjectTaskLiveness, 0, len(tasks))
	for _, task := range tasks {
		item := ProjectTaskLiveness{
			ProjectTaskID:         task.ID,
			BlockingDependencyIDs: append([]uuid.UUID(nil), unresolvedByTask[task.ID]...),
			CurrentAttemptID:      task.CurrentAttemptID,
			WaitingRequestID:      task.WaitingRequestID,
			RetryNotBefore:        task.RetryNotBefore,
		}
		if task.CurrentAttemptID != nil {
			attempt, err := s.repository.GetCurrentProjectTaskAttempt(ctx, tenantID, task.ID)
			if err != nil && !errors.Is(err, ErrProjectNotFound) {
				return nil, err
			}
			if err == nil {
				item.AttemptStatus = attempt.Status
				item.LeaseExpiresAt = attempt.LeaseExpiresAt
			}
		}
		classifyProjectTaskLiveness(&item, task, now)
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListProjectEvents(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefServiceRequest) (*ProjectEvidenceRef, error) {
	req.ActorType = strings.TrimSpace(req.ActorType)
	req.EvidenceType = strings.TrimSpace(req.EvidenceType)
	req.Title = strings.TrimSpace(req.Title)
	req.Summary = strings.TrimSpace(req.Summary)
	req.SourceType = strings.TrimSpace(req.SourceType)
	req.SourceRef = strings.TrimSpace(req.SourceRef)
	req.SubmittedByType = strings.TrimSpace(req.SubmittedByType)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorID == uuid.Nil || req.ActorType == "" || req.EvidenceType == "" || req.Title == "" || req.SourceType == "" || req.SourceRef == "" || req.SubmittedByType == "" || req.SubmittedByID == nil || *req.SubmittedByID == uuid.Nil {
		return nil, ErrInvalidProjectEvidence
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if projectArchived(project) {
		return nil, ErrProjectArchived
	}
	result, err := s.repository.CreateEvidenceRefWithEvent(ctx, CreateEvidenceRefWithEventRequest{
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventEvidenceLinked,
			ActorType:    req.ActorType,
			ActorID:      req.ActorID.String(),
			ResourceType: strPtr("project_evidence_ref"),
			Summary:      "项目证据已关联",
			Payload: map[string]any{
				"evidence_type": req.EvidenceType,
				"title":         req.Title,
				"source_type":   req.SourceType,
			},
		},
		Evidence: CreateEvidenceRefRequest{
			TenantID:           req.TenantID,
			ProjectID:          req.ProjectID,
			ProjectTaskID:      req.ProjectTaskID,
			RouteDecisionID:    req.RouteDecisionID,
			ExecutionSummaryID: req.ExecutionSummaryID,
			EvidenceType:       req.EvidenceType,
			Title:              req.Title,
			Summary:            req.Summary,
			SourceType:         req.SourceType,
			SourceRef:          req.SourceRef,
			ArtifactRefID:      req.ArtifactRefID,
			SubmittedByType:    req.SubmittedByType,
			SubmittedByID:      req.SubmittedByID,
			VerificationStatus: EvidenceVerificationStatusSubmitted,
			Metadata:           mapOrEmptyAny(req.Metadata),
		},
	})
	if err != nil {
		return nil, err
	}
	return &result.Evidence, nil
}

func (s *Service) ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListEvidenceRefs(ctx, tenantID, projectID, status, limit, offset)
}

type PatchEvidenceRequest struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	EvidenceID         uuid.UUID
	ActorUserID        uuid.UUID
	VerificationStatus EvidenceVerificationStatus
	Metadata           map[string]any
}

func (s *Service) ListEvidence(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	return s.ListEvidenceRefs(ctx, tenantID, projectID, status, limit, offset)
}

func (s *Service) CreateEvidence(ctx context.Context, req CreateEvidenceRefServiceRequest) (*ProjectEvidenceRef, error) {
	return s.CreateEvidenceRef(ctx, req)
}

func (s *Service) PatchEvidence(ctx context.Context, req PatchEvidenceRequest) (*ProjectEvidenceRef, error) {
	req.VerificationStatus = EvidenceVerificationStatus(strings.TrimSpace(string(req.VerificationStatus)))
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.EvidenceID == uuid.Nil || req.ActorUserID == uuid.Nil || !validEvidenceVerificationStatus(req.VerificationStatus) {
		return nil, ErrInvalidProjectEvidence
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if projectArchived(project) {
		return nil, ErrProjectArchived
	}
	result, err := s.repository.UpdateEvidenceVerificationStatusWithEvent(ctx, UpdateEvidenceVerificationStatusWithEventRequest{
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventEvidenceVerified,
			ActorType:    "human_user",
			ActorID:      req.ActorUserID.String(),
			ResourceType: strPtr("project_evidence_ref"),
			ResourceID:   strPtr(req.EvidenceID.String()),
			Summary:      "项目证据校验状态已更新",
			Payload: map[string]any{
				"verification_status": string(req.VerificationStatus),
			},
		},
		Evidence: UpdateEvidenceVerificationStatusRequest{
			TenantID:           req.TenantID,
			ProjectID:          req.ProjectID,
			ID:                 req.EvidenceID,
			VerificationStatus: req.VerificationStatus,
			Metadata:           req.Metadata,
		},
	})
	if err != nil {
		return nil, err
	}
	return &result.Evidence, nil
}

func (s *Service) ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListArtifactRefs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	return s.ListArtifactRefs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListReportRefs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListReports(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	return s.ListReportRefs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListBudgetLedger(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectBudgetSummary, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if _, err := s.repository.GetProject(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	summary, err := s.repository.GetBudgetSummary(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *Service) CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceServiceRequest) (*ProjectAcceptanceRecord, error) {
	req.Status = strings.TrimSpace(req.Status)
	req.Conclusion = strings.TrimSpace(req.Conclusion)
	req.Summary = strings.TrimSpace(req.Summary)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.AcceptedByUserID == uuid.Nil || req.Conclusion == "" || !validAcceptanceStatus(req.Status) {
		return nil, ErrInvalidProjectAcceptance
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if projectArchived(project) {
		return nil, ErrProjectArchived
	}
	if req.AcceptedByUserID != project.HumanOwnerUserID && (project.AcceptanceUserID == nil || req.AcceptedByUserID != *project.AcceptanceUserID) {
		return nil, ErrInvalidProjectAcceptance
	}
	if req.Status == "accepted" && (len(req.EvidenceRefIDs) == 0 || len(req.ReportRefIDs) == 0) {
		return nil, ErrInvalidProjectAcceptance
	}
	if req.Status == "accepted" {
		if err := s.validateAcceptanceRefs(ctx, req.TenantID, req.ProjectID, req.EvidenceRefIDs, req.ReportRefIDs); err != nil {
			return nil, err
		}
	}
	result, err := s.repository.CreateAcceptanceRecordWithEvent(ctx, CreateAcceptanceRecordWithEventRequest{
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventAcceptanceSubmitted,
			ActorType:    "human_user",
			ActorID:      req.AcceptedByUserID.String(),
			ResourceType: strPtr("project_acceptance_record"),
			Summary:      "项目验收结论已提交",
			Payload: map[string]any{
				"status":             req.Status,
				"evidence_ref_count": len(req.EvidenceRefIDs),
				"report_ref_count":   len(req.ReportRefIDs),
			},
		},
		Acceptance: CreateAcceptanceRecordRequest{
			TenantID:         req.TenantID,
			ProjectID:        req.ProjectID,
			AcceptedByUserID: req.AcceptedByUserID,
			Status:           req.Status,
			Conclusion:       req.Conclusion,
			Summary:          req.Summary,
			EvidenceRefIDs:   req.EvidenceRefIDs,
			ReportRefIDs:     req.ReportRefIDs,
			UnresolvedRisks:  sliceOrEmptyAny(req.UnresolvedRisks),
		},
	})
	if err != nil {
		return nil, err
	}
	return &result.Acceptance, nil
}

func (s *Service) CreateAcceptance(ctx context.Context, req CreateAcceptanceServiceRequest) (*ProjectAcceptanceRecord, error) {
	return s.CreateAcceptanceRecord(ctx, req)
}

func (s *Service) GetAcceptance(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectAcceptanceRecord, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	record, err := s.repository.GetLatestAcceptanceRecord(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Service) BuildArchivePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectArchivePreview, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	pageSize, _ := normalizePagination(100, 0)
	evidenceRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectEvidenceRef, error) {
		return s.repository.ListEvidenceRefs(ctx, tenantID, projectID, nil, limit, offset)
	})
	if err != nil {
		return nil, err
	}
	artifactRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectArtifactRef, error) {
		return s.repository.ListArtifactRefs(ctx, tenantID, projectID, limit, offset)
	})
	if err != nil {
		return nil, err
	}
	reportRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectReportRef, error) {
		return s.repository.ListReportRefs(ctx, tenantID, projectID, limit, offset)
	})
	if err != nil {
		return nil, err
	}
	budgetSummary, err := s.repository.GetBudgetSummary(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	blockedReasons := make([]any, 0)
	if len(reportRefs) == 0 {
		blockedReasons = append(blockedReasons, "missing_final_report")
	}
	if len(evidenceRefs) == 0 {
		blockedReasons = append(blockedReasons, "missing_evidence")
	}
	retentionPending := false
	estimatedObjectRefs := make([]any, 0, len(artifactRefs)+len(reportRefs))
	for _, artifact := range artifactRefs {
		if strings.TrimSpace(artifact.ObjectRef) != "" {
			estimatedObjectRefs = append(estimatedObjectRefs, artifact.ObjectRef)
		}
		if artifact.RetentionStatus == "" || artifact.RetentionStatus == "pending" || artifact.RetentionStatus == "retention_pending" {
			retentionPending = true
		}
	}
	for _, report := range reportRefs {
		if strings.TrimSpace(report.ObjectRef) != "" {
			estimatedObjectRefs = append(estimatedObjectRefs, report.ObjectRef)
		}
	}
	if projectArchived(project) {
		blockedReasons = append(blockedReasons, "project_already_archived")
	}
	if budgetSummary.LedgerCount > 0 {
		estimatedObjectRefs = append(estimatedObjectRefs, map[string]any{
			"budget_ledger_count": budgetSummary.LedgerCount,
			"actual_cost":         budgetSummary.ActualCost,
		})
	}
	return &ProjectArchivePreview{
		ProjectID:           projectID,
		EvidenceCount:       int64(len(evidenceRefs)),
		ArtifactCount:       int64(len(artifactRefs)),
		ReportCount:         int64(len(reportRefs)),
		RetentionPending:    retentionPending,
		BlockedReasons:      blockedReasons,
		EstimatedObjectRefs: estimatedObjectRefs,
	}, nil
}

func (s *Service) GetArchivePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectArchivePreview, error) {
	return s.BuildArchivePreview(ctx, tenantID, projectID)
}

func (s *Service) CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotServiceRequest) (*ProjectArchiveSnapshot, error) {
	req.SnapshotType = strings.TrimSpace(req.SnapshotType)
	req.Summary = strings.TrimSpace(req.Summary)
	req.ObjectRef = strings.TrimSpace(req.ObjectRef)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.CreatedByUserID == uuid.Nil || req.SnapshotType == "" {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if projectArchived(project) {
		return nil, ErrProjectArchived
	}
	preview, err := s.BuildArchivePreview(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	artifactIDs, err := s.collectArchiveArtifactIDs(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}

	status := "archived"
	retainedArtifactIDs := []uuid.UUID(nil)
	var retentionLockEventID *uuid.UUID
	if len(artifactIDs) > 0 {
		if s.archiveArtifactLocker == nil {
			status = "archive_pending_retention"
		} else {
			lockResult, lockErr := s.archiveArtifactLocker.LockProjectArtifacts(ctx, req.TenantID, req.ProjectID, artifactIDs)
			if lockErr != nil {
				status = "archive_pending_retention"
				retainedArtifactIDs = lockResult.ArtifactIDs
				retentionLockEventID = lockResult.EventID
			} else {
				retainedArtifactIDs = lockResult.ArtifactIDs
				retentionLockEventID = lockResult.EventID
			}
		}
	}

	includedCounts := archiveSnapshotIncludedCounts(preview)
	snapshotReq := CreateArchiveSnapshotWithEventRequest{
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventArchiveSnapshotCreated,
			ActorType:    "human_user",
			ActorID:      req.CreatedByUserID.String(),
			ResourceType: strPtr("project_archive_snapshot"),
			Summary:      "项目归档快照已创建",
			Payload: map[string]any{
				"snapshot_type": req.SnapshotType,
				"status":        status,
				"included_counts": map[string]any{
					"evidence": preview.EvidenceCount,
					"artifact": preview.ArtifactCount,
					"report":   preview.ReportCount,
				},
			},
		},
		Snapshot: CreateArchiveSnapshotRequest{
			TenantID:             req.TenantID,
			ProjectID:            req.ProjectID,
			SnapshotType:         req.SnapshotType,
			Status:               status,
			ObjectRef:            req.ObjectRef,
			Summary:              req.Summary,
			IncludedCounts:       includedCounts,
			RetainedArtifactIDs:  retainedArtifactIDs,
			RetentionLockEventID: retentionLockEventID,
			CreatedByUserID:      req.CreatedByUserID,
		},
	}
	var result ProjectArchiveSnapshotWriteResult
	if status == "archived" {
		result, err = s.repository.CreateArchiveSnapshotWithEventAndArchiveProject(ctx, snapshotReq)
	} else {
		result, err = s.repository.CreateArchiveSnapshotWithEvent(ctx, snapshotReq)
	}
	if err != nil {
		return nil, err
	}
	return &result.Snapshot, nil
}

func (s *Service) ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListArchiveSnapshots(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListConfigRevisions(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*ProjectConfigRevision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || revisionID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	revision, err := s.repository.GetConfigRevision(ctx, tenantID, projectID, revisionID)
	if err != nil {
		return nil, err
	}
	return &revision, nil
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
	preference, reviewerSourceRefs, err := s.resolveDemandReviewer(ctx, req, project)
	if err != nil {
		return nil, err
	}
	req.SourceRefs = mergeReviewerSourceRefs(req.SourceRefs, reviewerSourceRefs)

	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventDemandSubmitted,
		ActorType: "human_user",
		ActorID:   req.SubmittedByUserID.String(),
		Summary:   "需求已提交到当前项目",
		Payload: map[string]any{
			"title":                     req.Title,
			"reviewer_user_id":          preference.ReviewerUserID.String(),
			"reviewer_selection_reason": string(preference.SelectionReason),
		},
	})
	if err != nil {
		return nil, err
	}
	demand, err := s.repository.CreateProjectDemand(ctx, req, ProjectDemandStatusPlanningPending, &event.ID)
	if err != nil {
		return nil, err
	}
	demand.ReviewerPreference = preference
	if err := s.ensureProjectCoordinator(ctx, project); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, "DemandSubmitted", "failed", err, map[string]any{
			"demand_id":        demand.ID.String(),
			"created_event_id": event.ID.String(),
		})
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

func (s *Service) resolveDemandReviewer(ctx context.Context, req SubmitProjectDemandRequest, project Project) (*ReviewerPreference, map[string]any, error) {
	members, err := s.repository.ListProjectMembers(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	selected, reason, resolvedFromRule, err := selectReviewer(req.ReviewerUserID, req.ReviewerSelectionReason, project, members)
	if err != nil {
		return nil, nil, err
	}
	preference := &ReviewerPreference{
		ReviewerUserID:   selected.PrincipalID,
		SelectionReason:  reason,
		DisplayName:      selected.DisplayNameSnapshot,
		ProjectRole:      selected.ProjectRole,
		ResolvedFromRule: resolvedFromRule,
	}
	sourceRefs := map[string]any{
		"reviewer_user_id":            preference.ReviewerUserID.String(),
		"reviewer_selection_reason":   string(preference.SelectionReason),
		"reviewer_project_role":       string(preference.ProjectRole),
		"reviewer_resolved_from_rule": preference.ResolvedFromRule,
	}
	if preference.DisplayName != nil {
		sourceRefs["reviewer_display_name"] = *preference.DisplayName
	}
	return preference, sourceRefs, nil
}

func selectReviewer(explicit *uuid.UUID, explicitReason ReviewerSelectionReason, project Project, members []ProjectMember) (ProjectMember, ReviewerSelectionReason, bool, error) {
	if explicit != nil {
		reason, err := normalizeReviewerSelectionReason(explicitReason)
		if err != nil {
			return ProjectMember{}, "", false, err
		}
		for _, member := range members {
			if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == *explicit && member.Status == "active" {
				return member, reason, false, nil
			}
		}
		return ProjectMember{}, "", false, ErrInvalidProjectMember
	}
	reviewers := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeHumanUser && member.ProjectRole == ProjectRoleReviewer && member.Status == "active" {
			reviewers = append(reviewers, member)
		}
	}
	if len(reviewers) == 1 {
		return reviewers[0], ReviewerSelectionProjectReviewerDefault, true, nil
	}
	if len(reviewers) > 1 {
		return ProjectMember{}, "", false, ErrInvalidProjectMember
	}
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == project.HumanOwnerUserID && member.ProjectRole == ProjectRoleOwner && member.Status == "active" {
			return member, ReviewerSelectionProjectHumanOwnerFallback, true, nil
		}
	}
	return ProjectMember{}, "", false, ErrInvalidProjectMember
}

func normalizeReviewerSelectionReason(reason ReviewerSelectionReason) (ReviewerSelectionReason, error) {
	if reason == "" {
		return ReviewerSelectionUserSelected, nil
	}
	if isValidReviewerSelectionReason(reason) {
		return reason, nil
	}
	return "", ErrInvalidProjectMember
}

func isValidReviewerSelectionReason(reason ReviewerSelectionReason) bool {
	switch reason {
	case ReviewerSelectionProjectReviewerDefault, ReviewerSelectionProjectHumanOwnerFallback, ReviewerSelectionUserSelected:
		return true
	default:
		return false
	}
}

func mergeReviewerSourceRefs(sourceRefs map[string]any, reviewer map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range sourceRefs {
		if strings.HasPrefix(key, "reviewer_") {
			continue
		}
		merged[key] = value
	}
	for key, value := range reviewer {
		merged[key] = value
	}
	return merged
}

func (s *Service) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListProjectDemands(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) GetDemandLaunchDetail(ctx context.Context, tenantID, demandID uuid.UUID) (*DemandLaunchDetail, error) {
	if tenantID == uuid.Nil || demandID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	demand, err := s.repository.GetProjectDemand(ctx, tenantID, demandID)
	if err != nil {
		return nil, err
	}
	project, err := s.repository.GetProject(ctx, tenantID, demand.ProjectID)
	if err != nil {
		return nil, err
	}
	jobs, err := s.repository.ListDemandLaunchCoordinationJobs(ctx, tenantID, demand.ProjectID, demand.ID, demand.CreatedEventID, 100)
	if err != nil {
		return nil, err
	}
	routes, err := s.repository.ListDemandLaunchRouteDecisions(ctx, tenantID, demand.ProjectID, demand.ID, 100)
	if err != nil {
		return nil, err
	}
	tasks, err := s.repository.ListDemandLaunchProjectTasks(ctx, tenantID, demand.ProjectID, demand.ID, 100)
	if err != nil {
		return nil, err
	}
	taskIDs := projectTaskIDs(tasks)
	decisions, err := s.repository.ListDemandLaunchDecisionRequests(ctx, tenantID, demand.ProjectID, coordinationJobIDs(jobs), taskIDs, 100)
	if err != nil {
		return nil, err
	}
	events, err := s.repository.ListDemandLaunchEvents(ctx, tenantID, demand.ProjectID, demand.ID, demand.CreatedEventID, taskIDs, decisionRequestIDs(decisions), 50)
	if err != nil {
		return nil, err
	}
	return &DemandLaunchDetail{
		Demand:           demand,
		Project:          project,
		Reviewer:         demand.ReviewerPreference,
		CoordinationJobs: jobs,
		RouteDecisions:   routes,
		ProjectTasks:     tasks,
		DecisionRequests: decisions,
		RecentEvents:     events,
	}, nil
}

func (s *Service) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (*ProjectTaskGraph, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || (req.CoordinationJobID == nil && req.DemandID == nil) {
		return nil, ErrInvalidProject
	}
	graph, err := s.repository.GetProjectTaskGraph(ctx, req)
	if err != nil {
		return nil, err
	}
	normalizeProjectTaskGraph(&graph)
	return &graph, nil
}

func (s *Service) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListRouteDecisions(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	return s.repository.ListPlanRevisions(ctx, req)
}

func (s *Service) ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	return s.repository.ListPreDispatchGateResults(ctx, req)
}

func (s *Service) GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*PlanRevision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || revisionID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	revision, err := s.repository.GetPlanRevision(ctx, tenantID, projectID, revisionID)
	if err != nil {
		return nil, err
	}
	return &revision, nil
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

func coordinationJobIDs(jobs []CoordinationJob) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}
	return ids
}

func projectTaskIDs(tasks []ProjectTask) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func decisionRequestIDs(decisions []DecisionRequest) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(decisions))
	for _, decision := range decisions {
		ids = append(ids, decision.ID)
	}
	return ids
}

func filterJobsForDemand(jobs []CoordinationJob, demand ProjectDemand) []CoordinationJob {
	filtered := []CoordinationJob{}
	for _, job := range jobs {
		if demand.CreatedEventID != nil && job.TriggerEventID != nil && *job.TriggerEventID == *demand.CreatedEventID {
			filtered = append(filtered, job)
			continue
		}
		if rawDemandID, ok := job.InputSnapshotRef["demand_id"].(string); ok && rawDemandID == demand.ID.String() {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func filterRoutesForDemand(routes []RouteDecision, demandID uuid.UUID) []RouteDecision {
	filtered := []RouteDecision{}
	for _, route := range routes {
		if route.DemandID != nil && *route.DemandID == demandID {
			filtered = append(filtered, route)
		}
	}
	return filtered
}

func filterTasksForDemand(tasks []ProjectTask, demandID uuid.UUID) []ProjectTask {
	filtered := []ProjectTask{}
	for _, task := range tasks {
		if task.DemandID != nil && *task.DemandID == demandID {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func filterDecisionsForDemand(decisions []DecisionRequest, jobs []CoordinationJob, tasks []ProjectTask) []DecisionRequest {
	jobIDs := map[uuid.UUID]struct{}{}
	for _, job := range jobs {
		jobIDs[job.ID] = struct{}{}
	}
	taskIDs := map[uuid.UUID]struct{}{}
	for _, task := range tasks {
		taskIDs[task.ID] = struct{}{}
	}
	filtered := []DecisionRequest{}
	for _, decision := range decisions {
		if decision.CoordinationJobID != nil {
			if _, ok := jobIDs[*decision.CoordinationJobID]; ok {
				filtered = append(filtered, decision)
				continue
			}
		}
		if decision.ProjectTaskID != nil {
			if _, ok := taskIDs[*decision.ProjectTaskID]; ok {
				filtered = append(filtered, decision)
			}
		}
	}
	return filtered
}

func filterEventsForDemand(events []ProjectEvent, demand ProjectDemand, tasks []ProjectTask, decisions []DecisionRequest) []ProjectEvent {
	taskIDs := map[string]struct{}{}
	for _, task := range tasks {
		taskIDs[task.ID.String()] = struct{}{}
	}
	decisionIDs := map[string]struct{}{}
	for _, decision := range decisions {
		decisionIDs[decision.ID.String()] = struct{}{}
	}
	filtered := []ProjectEvent{}
	for _, event := range events {
		if demand.CreatedEventID != nil && event.ID == *demand.CreatedEventID {
			filtered = append(filtered, event)
			continue
		}
		if event.ResourceID != nil {
			if *event.ResourceID == demand.ID.String() {
				filtered = append(filtered, event)
				continue
			}
			if _, ok := taskIDs[*event.ResourceID]; ok {
				filtered = append(filtered, event)
				continue
			}
			if _, ok := decisionIDs[*event.ResourceID]; ok {
				filtered = append(filtered, event)
				continue
			}
		}
		if rawDemandID, ok := event.Payload["demand_id"].(string); ok && rawDemandID == demand.ID.String() {
			filtered = append(filtered, event)
			continue
		}
		if rawProjectTaskID, ok := event.Payload["project_task_id"].(string); ok {
			if _, exists := taskIDs[rawProjectTaskID]; exists {
				filtered = append(filtered, event)
				continue
			}
		}
		if rawDecisionRequestID, ok := event.Payload["decision_request_id"].(string); ok {
			if _, exists := decisionIDs[rawDecisionRequestID]; exists {
				filtered = append(filtered, event)
			}
		}
	}
	return filtered
}

func normalizeProjectTaskGraph(graph *ProjectTaskGraph) {
	if graph.Nodes == nil {
		graph.Nodes = []ProjectTaskGraphNode{}
	}
	if graph.Edges == nil {
		graph.Edges = []ProjectTaskGraphEdge{}
	}
	if graph.Employees == nil {
		graph.Employees = []ProjectTaskGraphEmployee{}
	}
	if graph.Runs == nil {
		graph.Runs = []ProjectTaskGraphRun{}
	}
	if graph.ExecutionSummaries == nil {
		graph.ExecutionSummaries = []ExecutionSummary{}
	}
	if graph.RecentEvents == nil {
		graph.RecentEvents = []ProjectEvent{}
	}
	if graph.DecisionRequests == nil {
		graph.DecisionRequests = []DecisionRequest{}
	}
	if graph.StageSummaries == nil {
		graph.StageSummaries = buildProjectTaskGraphStageSummaries(graph.Nodes)
	}
}

func buildProjectTaskGraphStageSummaries(nodes []ProjectTaskGraphNode) []ProjectTaskGraphStageSummary {
	type mutableSummary struct {
		summary ProjectTaskGraphStageSummary
	}
	byStage := map[int32]*mutableSummary{}
	for _, node := range nodes {
		stage := int32(-1)
		if node.Task.StageIndex != nil {
			stage = *node.Task.StageIndex
		}
		entry := byStage[stage]
		if entry == nil {
			title := "未分阶段"
			if stage >= 0 {
				title = fmt.Sprintf("第 %d 阶段", stage)
			}
			entry = &mutableSummary{summary: ProjectTaskGraphStageSummary{StageIndex: stage, Title: title}}
			byStage[stage] = entry
		}
		entry.summary.TotalNodes++
		switch normalizeTaskStatusForSummary(node.Task.Status) {
		case "completed":
			entry.summary.CompletedNodes++
		case "running":
			entry.summary.RunningNodes++
		case "waiting_human":
			entry.summary.WaitingHumanNodes++
		case "blocked":
			entry.summary.BlockedNodes++
		}
	}
	stages := make([]int, 0, len(byStage))
	for stage := range byStage {
		stages = append(stages, int(stage))
	}
	sort.Ints(stages)
	result := make([]ProjectTaskGraphStageSummary, 0, len(stages))
	for _, stage := range stages {
		result = append(result, byStage[int32(stage)].summary)
	}
	return result
}

func normalizeTaskStatusForSummary(status string) string {
	switch strings.ToLower(status) {
	case "completed", "done", "success":
		return "completed"
	case "assigned", "running", "in_progress":
		return "running"
	case "waiting_human", "pending_review":
		return "waiting_human"
	case "blocked":
		return "blocked"
	default:
		return "other"
	}
}

func (s *Service) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListExecutionSummaries(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) GetExecutionTrace(ctx context.Context, req GetExecutionTraceRequest) (*ProjectExecutionTrace, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	if req.Limit < 100 {
		req.Limit = 100
	}
	attempts, err := s.repository.ListProjectTaskAttemptsForExecutionTrace(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	attempts = filterExecutionTraceAttempts(attempts, req)
	events, err := s.repository.ListProjectExecutionLedgerEvents(ctx, req)
	if err != nil {
		return nil, err
	}
	summaryEventType := ExecutionLedgerEventSummaryCreated
	summaryMappingReq := req
	summaryMappingReq.EventType = &summaryEventType
	summaryMappingReq.ErrorFamily = nil
	summaryMappingReq.Limit = 1000
	summaryMappingReq.Offset = 0
	summaryMappingEvents, err := s.repository.ListProjectExecutionLedgerEvents(ctx, summaryMappingReq)
	if err != nil {
		return nil, err
	}
	summaries, err := s.repository.ListExecutionSummaries(ctx, req.TenantID, req.ProjectID, 1000, 0)
	if err != nil {
		return nil, err
	}
	trace := buildProjectExecutionTrace(req.ProjectID, attempts, events, summaryMappingEvents, summaries)
	return &trace, nil
}

func filterExecutionTraceAttempts(attempts []ProjectTaskAttempt, req GetExecutionTraceRequest) []ProjectTaskAttempt {
	if req.ProjectTaskID == nil && req.ProjectTaskAttemptID == nil {
		return attempts
	}
	filtered := make([]ProjectTaskAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		if req.ProjectTaskID != nil && attempt.ProjectTaskID != *req.ProjectTaskID {
			continue
		}
		if req.ProjectTaskAttemptID != nil && attempt.ID != *req.ProjectTaskAttemptID {
			continue
		}
		filtered = append(filtered, attempt)
	}
	return filtered
}

func buildProjectExecutionTrace(projectID uuid.UUID, attempts []ProjectTaskAttempt, visibleEvents []ExecutionLedgerEvent, summaryMappingEvents []ExecutionLedgerEvent, summaries []ExecutionSummary) ProjectExecutionTrace {
	trace := ProjectExecutionTrace{
		ProjectID: projectID,
		Attempts:  make([]ProjectExecutionTraceAttempt, 0, len(attempts)),
	}
	attemptIndexes := make(map[uuid.UUID]int, len(attempts))
	latestAttemptIndexByTaskID := make(map[uuid.UUID]int, len(attempts))
	for _, attempt := range attempts {
		attemptIndexes[attempt.ID] = len(trace.Attempts)
		trace.Attempts = append(trace.Attempts, ProjectExecutionTraceAttempt{
			ProjectTaskID:     attempt.ProjectTaskID,
			AttemptID:         attempt.ID,
			AttemptNo:         attempt.AttemptNo,
			Status:            attempt.Status,
			RuntimeNodeID:     attempt.RuntimeNodeID,
			ProviderSessionID: attempt.ProviderSessionID,
			StartedAt:         attempt.StartedAt,
			FinishedAt:        attempt.FinishedAt,
			FailureFamily:     attempt.FailureFamily,
			Retryable:         attempt.Retryable,
			Events:            []ExecutionLedgerEvent{},
		})
		trace.Summary.AttemptCount++
		if isFailedExecutionTraceAttempt(attempt.Status) {
			trace.Summary.FailedAttemptCount++
		}
		latestIndex, ok := latestAttemptIndexByTaskID[attempt.ProjectTaskID]
		if !ok || executionTraceAttemptAfter(attempt, attempts[latestIndex]) {
			latestAttemptIndexByTaskID[attempt.ProjectTaskID] = attemptIndexes[attempt.ID]
		}
	}

	summaryByID := make(map[string]ExecutionSummary, len(summaries))
	latestSummaryByTaskID := make(map[uuid.UUID]ExecutionSummary, len(summaries))
	tasksWithMatchedSummaryEvent := make(map[uuid.UUID]bool, len(summaries))
	attachedSummaryIDs := make(map[uuid.UUID]bool, len(summaries))
	refCounter := newExecutionTraceRefCounter()
	for _, summary := range summaries {
		summaryByID[summary.ID.String()] = summary
		latest, ok := latestSummaryByTaskID[summary.ProjectTaskID]
		if !ok || summary.CreatedAt.After(latest.CreatedAt) {
			latestSummaryByTaskID[summary.ProjectTaskID] = summary
		}
	}

	for _, event := range summaryMappingEvents {
		if event.EventType != ExecutionLedgerEventSummaryCreated {
			continue
		}
		summary, ok := summaryByID[event.SourceID]
		if !ok {
			continue
		}
		tasksWithMatchedSummaryEvent[summary.ProjectTaskID] = true
		if event.ProjectTaskAttemptID == nil {
			continue
		}
		attemptIndex, attemptOK := attemptIndexes[*event.ProjectTaskAttemptID]
		if attemptOK && !attachedSummaryIDs[summary.ID] && trace.Attempts[attemptIndex].Summary == nil {
			attachExecutionTraceSummary(&trace, attemptIndex, summary, refCounter)
			attachedSummaryIDs[summary.ID] = true
		}
	}

	var latestErrorEvent *ExecutionLedgerEvent
	for _, event := range visibleEvents {
		if event.ErrorFamily != nil && (latestErrorEvent == nil || executionTraceEventAfter(event, *latestErrorEvent)) {
			latestEvent := event
			latestErrorEvent = &latestEvent
			errorFamily := *event.ErrorFamily
			trace.Summary.LatestErrorFamily = &errorFamily
		}
		if event.ProjectTaskAttemptID == nil {
			continue
		}
		attemptIndex, attemptOK := attemptIndexes[*event.ProjectTaskAttemptID]
		if !attemptOK {
			continue
		}
		clonedEvent := cloneExecutionLedgerEvent(event)
		trace.Attempts[attemptIndex].Events = append(trace.Attempts[attemptIndex].Events, clonedEvent)
		trace.Summary.ArtifactRefCount += refCounter.addArtifactRefs(clonedEvent.ArtifactRefs)
		trace.Summary.EvidenceRefCount += refCounter.addEvidenceRefs(clonedEvent.EvidenceRefs)
		if trace.Attempts[attemptIndex].ProviderType == nil && clonedEvent.ProviderType != nil {
			trace.Attempts[attemptIndex].ProviderType = clonedEvent.ProviderType
		}
	}

	for taskID, summary := range latestSummaryByTaskID {
		if tasksWithMatchedSummaryEvent[taskID] {
			continue
		}
		attemptIndex, ok := latestAttemptIndexByTaskID[taskID]
		if !ok || trace.Attempts[attemptIndex].Summary != nil || attachedSummaryIDs[summary.ID] {
			continue
		}
		attachExecutionTraceSummary(&trace, attemptIndex, summary, refCounter)
		attachedSummaryIDs[summary.ID] = true
	}
	return trace
}

func isFailedExecutionTraceAttempt(status string) bool {
	switch status {
	case ProjectTaskAttemptStatusFailed, ProjectTaskAttemptStatusLost, ProjectTaskAttemptStatusTimedOut:
		return true
	default:
		return false
	}
}

func executionTraceEventAfter(left, right ExecutionLedgerEvent) bool {
	if !left.OccurredAt.Equal(right.OccurredAt) {
		return left.OccurredAt.After(right.OccurredAt)
	}
	return left.CreatedAt.After(right.CreatedAt)
}

func executionTraceAttemptAfter(left, right ProjectTaskAttempt) bool {
	leftTime := executionTraceAttemptSortTime(left)
	rightTime := executionTraceAttemptSortTime(right)
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	if left.AttemptNo != right.AttemptNo {
		return left.AttemptNo > right.AttemptNo
	}
	return left.ID.String() > right.ID.String()
}

func executionTraceAttemptSortTime(attempt ProjectTaskAttempt) time.Time {
	if attempt.FinishedAt != nil {
		return *attempt.FinishedAt
	}
	if attempt.StartedAt != nil {
		return *attempt.StartedAt
	}
	return attempt.CreatedAt
}

type executionTraceRefCounter struct {
	artifactRefs map[string]struct{}
	evidenceRefs map[string]struct{}
}

func newExecutionTraceRefCounter() *executionTraceRefCounter {
	return &executionTraceRefCounter{
		artifactRefs: map[string]struct{}{},
		evidenceRefs: map[string]struct{}{},
	}
}

func (c *executionTraceRefCounter) addArtifactRefs(refs []any) int32 {
	return addExecutionTraceRefs(c.artifactRefs, refs)
}

func (c *executionTraceRefCounter) addEvidenceRefs(refs []any) int32 {
	return addExecutionTraceRefs(c.evidenceRefs, refs)
}

func addExecutionTraceRefs(seen map[string]struct{}, refs []any) int32 {
	var added int32
	for _, ref := range sliceOrEmptyAny(refs) {
		key := executionTraceRefKey(ref)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		added++
	}
	return added
}

func executionTraceRefKey(ref any) string {
	encoded, err := json.Marshal(ref)
	if err == nil {
		return "json:" + string(encoded)
	}
	return fmt.Sprintf("fmt:%#v", ref)
}

func attachExecutionTraceSummary(trace *ProjectExecutionTrace, attemptIndex int, summary ExecutionSummary, refCounter *executionTraceRefCounter) {
	if trace.Attempts[attemptIndex].Summary != nil {
		return
	}
	artifactRefs := sliceOrEmptyAny(summary.ArtifactRefs)
	evidenceRefs := sliceOrEmptyAny(summary.EvidenceRefs)
	trace.Attempts[attemptIndex].Summary = &ProjectExecutionTraceAttemptSummary{
		ExecutionSummaryID:  summary.ID,
		Conclusion:          summary.Conclusion,
		RequiresHumanReview: summary.RequiresHumanReview,
		ArtifactRefs:        append([]any(nil), artifactRefs...),
		EvidenceRefs:        append([]any(nil), evidenceRefs...),
		CreatedAt:           summary.CreatedAt,
	}
	if summary.RequiresHumanReview {
		trace.Summary.HumanReviewRequiredCount++
	}
	trace.Summary.ArtifactRefCount += refCounter.addArtifactRefs(artifactRefs)
	trace.Summary.EvidenceRefCount += refCounter.addEvidenceRefs(evidenceRefs)
}

func cloneExecutionLedgerEvent(event ExecutionLedgerEvent) ExecutionLedgerEvent {
	event.ArtifactRefs = append([]any(nil), sliceOrEmptyAny(event.ArtifactRefs)...)
	event.EvidenceRefs = append([]any(nil), sliceOrEmptyAny(event.EvidenceRefs)...)
	event.Metadata = cloneMap(mapOrEmptyAny(event.Metadata))
	return event
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
	runWorkProducts, err := s.projectTaskRunWorkProducts(ctx, req.TenantID, task)
	if err != nil {
		return nil, err
	}
	contract := validateProjectTaskCompletionContract(task, req, runWorkProducts)
	if !contract.Satisfied() {
		if err := s.appendProjectTaskContractMissingEvent(ctx, task, req, contract); err != nil {
			return nil, err
		}
		return nil, ErrInvalidProjectEvidence
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
	// Materialize the task's structured evidence/artifacts into the project read
	// models so /evidence and /artifacts surface them. Best-effort: a materialization
	// failure must not roll back an already-completed task, so it is audited, not returned.
	if err := s.materializeTaskCompletionEvidence(ctx, task, req, result.Summary.ID); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EvidenceMaterialization", "failed", err, map[string]any{
			"project_task_id":      task.ID.String(),
			"execution_summary_id": result.Summary.ID.String(),
		})
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

func (s *Service) StartProjectTaskAttempt(ctx context.Context, req StartProjectTaskAttemptRequest) (*ProjectTaskAttempt, error) {
	var lastErr error
	for attempt := 0; attempt < projectTaskAttemptStartReadinessAttempts; attempt++ {
		started, err := s.startProjectTaskAttemptOnce(ctx, req)
		if err == nil {
			return started, nil
		}
		lastErr = err
		if !errors.Is(err, ErrProjectConflict) && !errors.Is(err, ErrProjectNotFound) {
			return nil, err
		}
		if !s.projectTaskAttemptStartMayBeAheadOfQueue(ctx, req.ProjectTaskAttemptRuntimeRequest) {
			return nil, err
		}
		if attempt == projectTaskAttemptStartReadinessAttempts-1 {
			break
		}
		timer := time.NewTimer(projectTaskAttemptStartReadinessBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func (s *Service) projectTaskAttemptStartMayBeAheadOfQueue(ctx context.Context, req ProjectTaskAttemptRuntimeRequest) bool {
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return errors.Is(err, ErrProjectNotFound)
	}
	if task.CurrentAttemptID == nil {
		return task.Status == ProjectTaskStatusPlanned || task.Status == ProjectTaskStatusWaitingHuman
	}
	if *task.CurrentAttemptID != req.AttemptID {
		return task.Status == ProjectTaskStatusPlanned || task.Status == ProjectTaskStatusWaitingHuman
	}
	if !projectTaskAcceptsRuntimeWriteback(task.Status) {
		return false
	}
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID)
	if err != nil {
		return errors.Is(err, ErrProjectNotFound)
	}
	if attempt.ProjectTaskID != req.ProjectTaskID || attempt.LeaseToken != req.LeaseToken {
		return false
	}
	if attempt.RuntimeNodeID != nil && *attempt.RuntimeNodeID != req.RuntimeNodeID {
		return false
	}
	return attempt.Status == ProjectTaskAttemptStatusQueued
}

func (s *Service) startProjectTaskAttemptOnce(ctx context.Context, req StartProjectTaskAttemptRequest) (*ProjectTaskAttempt, error) {
	if _, _, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest); err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.StartProjectTaskAttemptWriteback(ctx, req)
	if err != nil {
		return nil, err
	}
	_, _ = s.repository.CreateExecutionLedgerEvent(ctx, CreateExecutionLedgerEventRequest{
		TenantID:             req.TenantID,
		ProjectID:            result.Task.ProjectID,
		ProjectTaskID:        &req.ProjectTaskID,
		ProjectTaskAttemptID: &req.AttemptID,
		EventType:            ExecutionLedgerEventAttemptStarted,
		SourceType:           "project_task_attempt",
		SourceID:             req.AttemptID.String(),
		ActorType:            "runtime_node",
		ActorID:              strPtr(req.RuntimeNodeID.String()),
		RuntimeNodeID:        &req.RuntimeNodeID,
		ProviderSessionID:    req.ProviderSessionID,
		InputSummary:         "Runtime started project task attempt",
		Metadata: map[string]any{
			"project_task_id": req.ProjectTaskID.String(),
			"idempotency_key": req.IdempotencyKey,
		},
		IdempotencyKey: "project_task_attempt:" + req.AttemptID.String() + ":attempt.started",
	})
	return &result.Attempt, nil
}

func (s *Service) RenewProjectTaskAttemptLease(ctx context.Context, req RenewProjectTaskAttemptLeaseRequest) error {
	if _, _, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest); err != nil {
		return err
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return err
	}
	_, err = writebackRepository.RenewProjectTaskAttemptLeaseWriteback(ctx, req)
	return err
}

func (s *Service) CompleteProjectTaskAttempt(ctx context.Context, req CompleteProjectTaskAttemptRequest) (*ExecutionSummary, error) {
	req.Conclusion = strings.TrimSpace(req.Conclusion)
	if req.Conclusion == "" {
		return nil, ErrInvalidProject
	}
	task, _, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest)
	if err != nil {
		return nil, err
	}
	digitalEmployeeID, err := digitalEmployeeIDForProjectTask(task)
	if err != nil {
		return nil, err
	}
	req.DigitalEmployeeID = digitalEmployeeID
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, task.ProjectID)
	if err != nil {
		return nil, err
	}
	runWorkProducts, err := s.projectTaskRunWorkProducts(ctx, req.TenantID, task)
	if err != nil {
		return nil, err
	}
	contract := validateProjectTaskCompletionContract(task, CompleteProjectTaskRequest{
		TenantID:              req.TenantID,
		RuntimeNodeID:         req.RuntimeNodeID,
		ProjectTaskID:         req.ProjectTaskID,
		DigitalEmployeeID:     digitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          req.EvidenceRefs,
		ArtifactRefs:          req.ArtifactRefs,
		ConfidenceFactors:     req.ConfidenceFactors,
		Uncertainty:           req.Uncertainty,
		MissingInformation:    req.MissingInformation,
		RecommendedNextAction: req.RecommendedNextAction,
		RequiresHumanReview:   req.RequiresHumanReview,
	}, runWorkProducts)
	if !contract.Satisfied() {
		if err := s.appendProjectTaskContractMissingEvent(ctx, task, CompleteProjectTaskRequest{
			TenantID:              req.TenantID,
			RuntimeNodeID:         req.RuntimeNodeID,
			ProjectTaskID:         req.ProjectTaskID,
			DigitalEmployeeID:     digitalEmployeeID,
			Conclusion:            req.Conclusion,
			EvidenceRefs:          req.EvidenceRefs,
			ArtifactRefs:          req.ArtifactRefs,
			ConfidenceFactors:     req.ConfidenceFactors,
			Uncertainty:           req.Uncertainty,
			MissingInformation:    req.MissingInformation,
			RecommendedNextAction: req.RecommendedNextAction,
			RequiresHumanReview:   req.RequiresHumanReview,
		}, contract); err != nil {
			return nil, err
		}
		return nil, ErrInvalidProjectEvidence
	}
	var resultRecordReq *RecordProjectTaskResultRequest
	if req.ResultContract != nil {
		validation := ValidateTaskResultContract(task, *req.ResultContract)
		if !validation.Valid {
			return nil, ErrInvalidProjectEvidence
		}
		recordReq := projectTaskAttemptResultRecordRequest(task, req.ProjectTaskAttemptRuntimeRequest, nil, nil, *req.ResultContract, validation)
		resultRecordReq = &recordReq
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	if projectTaskRequiresAcceptance(task, req) {
		acceptanceReq := CompleteProjectTaskAttemptAcceptanceWritebackRequest{
			Task:     task,
			Complete: req,
			Decision: CreateDecisionRequestRequest{
				TenantID:          req.TenantID,
				ProjectID:         task.ProjectID,
				ApprovalRequestID: uuid.Nil,
				CoordinationJobID: task.CoordinationJobID,
				ProjectTaskID:     &task.ID,
				TargetUserID:      projectRecord.HumanOwnerUserID,
				DecisionType:      projectTaskHumanWaitDecisionType(HumanWaitReasonAcceptanceRequired),
				TitleSnapshot:     task.Title,
				SummarySnapshot:   req.Conclusion,
				RiskLevelSnapshot: stringValue(task.RiskLevel),
				StatusSnapshot:    "pending",
			},
		}
		var result ProjectTaskWritebackResult
		var err error
		if resultRecordReq != nil {
			result, err = writebackRepository.CompleteProjectTaskAttemptAcceptanceResultWriteback(ctx, CompleteProjectTaskAttemptAcceptanceResultWritebackRequest{
				Acceptance: acceptanceReq,
				Result:     *resultRecordReq,
			})
		} else {
			result, err = writebackRepository.CompleteProjectTaskAttemptAcceptanceWriteback(ctx, acceptanceReq)
		}
		if err != nil {
			return nil, err
		}
		if s.inbox != nil {
			if err := s.inbox.UpsertProjectDecisionRequest(ctx, result.Decision); err != nil {
				return nil, err
			}
		}
		if err := s.materializeTaskCompletionEvidence(ctx, task, CompleteProjectTaskRequest{
			TenantID:              req.TenantID,
			RuntimeNodeID:         req.RuntimeNodeID,
			ProjectTaskID:         req.ProjectTaskID,
			DigitalEmployeeID:     digitalEmployeeID,
			Conclusion:            req.Conclusion,
			EvidenceRefs:          req.EvidenceRefs,
			ArtifactRefs:          req.ArtifactRefs,
			ConfidenceFactors:     req.ConfidenceFactors,
			Uncertainty:           req.Uncertainty,
			MissingInformation:    req.MissingInformation,
			RecommendedNextAction: req.RecommendedNextAction,
			RequiresHumanReview:   req.RequiresHumanReview,
		}, result.Summary.ID); err != nil {
			_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EvidenceMaterialization", "failed", err, map[string]any{
				"project_task_id":      task.ID.String(),
				"execution_summary_id": result.Summary.ID.String(),
			})
		}
		return &result.Summary, nil
	}
	var result ProjectTaskWritebackResult
	if resultRecordReq != nil {
		result, err = writebackRepository.CompleteProjectTaskAttemptResultWriteback(ctx, CompleteProjectTaskAttemptResultWritebackRequest{
			Complete: req,
			Result:   *resultRecordReq,
		})
	} else {
		result, err = writebackRepository.CompleteProjectTaskAttemptWriteback(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	if err := s.materializeTaskCompletionEvidence(ctx, task, CompleteProjectTaskRequest{
		TenantID:              req.TenantID,
		RuntimeNodeID:         req.RuntimeNodeID,
		ProjectTaskID:         req.ProjectTaskID,
		DigitalEmployeeID:     digitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          req.EvidenceRefs,
		ArtifactRefs:          req.ArtifactRefs,
		ConfidenceFactors:     req.ConfidenceFactors,
		Uncertainty:           req.Uncertainty,
		MissingInformation:    req.MissingInformation,
		RecommendedNextAction: req.RecommendedNextAction,
		RequiresHumanReview:   req.RequiresHumanReview,
	}, result.Summary.ID); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EvidenceMaterialization", "failed", err, map[string]any{
			"project_task_id":      task.ID.String(),
			"execution_summary_id": result.Summary.ID.String(),
		})
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

func (s *Service) SubmitProjectTaskAttemptResult(ctx context.Context, req SubmitProjectTaskAttemptResultRequest) (*ExecutionSummary, error) {
	if req.ResultContract.Status == TaskResultStatusCompleted {
		return s.CompleteProjectTaskAttempt(ctx, CompleteProjectTaskAttemptRequest{
			ProjectTaskAttemptRuntimeRequest: req.ProjectTaskAttemptRuntimeRequest,
			Conclusion:                       req.ResultContract.Summary,
			EvidenceRefs:                     taskResultRefsToAny(req.ResultContract.EvidenceRefs),
			ArtifactRefs:                     taskResultRefsToAny(req.ResultContract.ArtifactRefs),
			RecommendedNextAction:            firstTaskResultFollowUpSummary(req.ResultContract.FollowUpRequests),
			RequiresHumanReview:              req.ResultContract.HumanReviewRequest != nil,
			ResultContract:                   &req.ResultContract,
		})
	}

	task, _, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest)
	if err != nil {
		return nil, err
	}
	if _, err := s.recordProjectTaskAttemptResult(ctx, task, req.ProjectTaskAttemptRuntimeRequest, nil, nil, req.ResultContract); err != nil {
		return nil, err
	}
	return &ExecutionSummary{
		TenantID:      req.TenantID,
		ProjectID:     task.ProjectID,
		ProjectTaskID: req.ProjectTaskID,
		Conclusion:    req.ResultContract.Summary,
	}, nil
}

func (s *Service) recordProjectTaskAttemptResult(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, summaryID, eventID *uuid.UUID, contract TaskResultContract) (ProjectTaskResult, error) {
	validation := ValidateTaskResultContract(task, contract)
	result, err := s.repository.RecordProjectTaskResult(ctx, projectTaskAttemptResultRecordRequest(task, runtimeReq, summaryID, eventID, contract, validation))
	if err != nil {
		return ProjectTaskResult{}, err
	}
	if _, err := s.repository.LinkProjectTaskLatestResult(ctx, runtimeReq.TenantID, task.ProjectID, runtimeReq.ProjectTaskID, result.ID); err != nil {
		return ProjectTaskResult{}, err
	}
	if !validation.Valid {
		return result, ErrInvalidProjectEvidence
	}
	return result, nil
}

func projectTaskAttemptResultRecordRequest(task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, summaryID, eventID *uuid.UUID, contract TaskResultContract, validation TaskResultValidation) RecordProjectTaskResultRequest {
	validationStatus := "accepted"
	if !validation.Valid {
		validationStatus = "rejected"
	}
	return RecordProjectTaskResultRequest{
		TenantID:           runtimeReq.TenantID,
		ProjectID:          task.ProjectID,
		ProjectTaskID:      runtimeReq.ProjectTaskID,
		AttemptID:          &runtimeReq.AttemptID,
		ExecutionSummaryID: summaryID,
		ResultStatus:       contract.Status,
		ValidationStatus:   validationStatus,
		Decision:           validation.Decision,
		Contract:           contract,
		ValidationErrors:   taskResultValidationErrors(validation.Errors),
		ValidationWarnings: validation.Warnings,
		IdempotencyKey:     "project_task_attempt:" + runtimeReq.AttemptID.String() + ":result:" + runtimeReq.IdempotencyKey,
		CreatedEventID:     eventID,
	}
}

func taskResultValidationErrors(errors []TaskResultValidationError) []string {
	values := make([]string, 0, len(errors))
	for _, err := range errors {
		values = append(values, string(err))
	}
	return values
}

func taskResultRefsToAny(refs []TaskResultRef) []any {
	values := make([]any, 0, len(refs))
	for _, ref := range refs {
		value := map[string]any{}
		if ref.Type != "" {
			value["type"] = ref.Type
		}
		if ref.Ref != "" {
			value["ref"] = ref.Ref
		}
		if ref.Summary != "" {
			value["summary"] = ref.Summary
		}
		values = append(values, value)
	}
	return values
}

func firstTaskResultFollowUpSummary(requests []TaskResultFollowUpRequest) string {
	for _, request := range requests {
		if summary := strings.TrimSpace(request.Summary); summary != "" {
			return summary
		}
	}
	return ""
}

type parsedEvidenceRef struct {
	EvidenceType string
	Title        string
	Summary      string
	SourceType   string
	SourceRef    string
}

type parsedArtifactRef struct {
	ArtifactType string
	Title        string
	ObjectRef    string
	ContentType  string
	Checksum     string
}

// materializeTaskCompletionEvidence turns the structured evidence_refs/artifact_refs a
// digital employee returns on completion into ProjectEvidenceRef / ProjectArtifactRef
// read-model rows, reusing the same create paths as the manual evidence/artifact APIs.
// Re-completion is blocked by the writeback status guard, so this runs once per task.
// Returns the first error encountered; the caller treats it as best-effort.
func (s *Service) materializeTaskCompletionEvidence(ctx context.Context, task ProjectTask, req CompleteProjectTaskRequest, summaryID uuid.UUID) error {
	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, raw := range req.ArtifactRefs {
		parsed, ok := parseArtifactRefElement(raw)
		if !ok {
			continue
		}
		artifact, err := s.repository.CreateArtifactRef(ctx, CreateArtifactRefRequest{
			TenantID:        req.TenantID,
			ProjectID:       task.ProjectID,
			ProjectTaskID:   &task.ID,
			ArtifactType:    parsed.ArtifactType,
			Title:           parsed.Title,
			ObjectRef:       parsed.ObjectRef,
			ContentType:     parsed.ContentType,
			Checksum:        parsed.Checksum,
			RetentionStatus: "pending",
		})
		if err != nil {
			record(err)
			continue
		}
		_, err = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    task.ProjectID,
			EventType:    ProjectEventArtifactLinked,
			ActorType:    "digital_employee",
			ActorID:      req.DigitalEmployeeID.String(),
			ResourceType: strPtr("project_artifact_ref"),
			ResourceID:   strPtr(artifact.ID.String()),
			Summary:      "项目工件已关联",
			Payload: map[string]any{
				"artifact_type":   parsed.ArtifactType,
				"title":           parsed.Title,
				"project_task_id": task.ID.String(),
			},
		})
		record(err)
	}
	submittedBy := req.DigitalEmployeeID
	for _, raw := range req.EvidenceRefs {
		parsed, ok := parseEvidenceRefElement(raw)
		if !ok {
			continue
		}
		_, err := s.CreateEvidenceRef(ctx, CreateEvidenceRefServiceRequest{
			TenantID:           req.TenantID,
			ProjectID:          task.ProjectID,
			ActorType:          "digital_employee",
			ActorID:            req.DigitalEmployeeID,
			ProjectTaskID:      &task.ID,
			RouteDecisionID:    task.RouteDecisionID,
			ExecutionSummaryID: &summaryID,
			EvidenceType:       parsed.EvidenceType,
			Title:              parsed.Title,
			Summary:            parsed.Summary,
			SourceType:         parsed.SourceType,
			SourceRef:          parsed.SourceRef,
			SubmittedByType:    "digital_employee",
			SubmittedByID:      &submittedBy,
		})
		record(err)
	}
	return firstErr
}

// parseEvidenceRefElement maps a completion evidence_ref element into a parsedEvidenceRef.
// Elements are either a plain string ref or a map[string]any with ref/id/title/type keys
// (matching addReferenceTokens). ok is false when no usable source ref is present.
func parseEvidenceRefElement(value any) (parsedEvidenceRef, bool) {
	parsed := parsedEvidenceRef{EvidenceType: "execution_evidence", SourceType: "runtime_output"}
	switch typed := value.(type) {
	case string:
		parsed.SourceRef = strings.TrimSpace(typed)
	case map[string]any:
		parsed.SourceRef = firstRefString(typed, "source_ref", "ref", "id")
		parsed.Title = firstRefString(typed, "title")
		parsed.Summary = firstRefString(typed, "summary")
		if t := firstRefString(typed, "evidence_type", "type"); t != "" {
			parsed.EvidenceType = t
		}
		if st := firstRefString(typed, "source_type"); st != "" {
			parsed.SourceType = st
		}
	default:
		return parsedEvidenceRef{}, false
	}
	if parsed.SourceRef == "" {
		return parsedEvidenceRef{}, false
	}
	if parsed.Title == "" {
		parsed.Title = parsed.SourceRef
	}
	return parsed, true
}

// parseArtifactRefElement maps a completion artifact_ref element into a parsedArtifactRef.
func parseArtifactRefElement(value any) (parsedArtifactRef, bool) {
	parsed := parsedArtifactRef{ArtifactType: "execution_artifact"}
	switch typed := value.(type) {
	case string:
		parsed.ObjectRef = strings.TrimSpace(typed)
	case map[string]any:
		parsed.ObjectRef = firstRefString(typed, "object_ref", "ref", "id")
		parsed.Title = firstRefString(typed, "title")
		parsed.ContentType = firstRefString(typed, "content_type")
		parsed.Checksum = firstRefString(typed, "checksum")
		if t := firstRefString(typed, "artifact_type", "type"); t != "" {
			parsed.ArtifactType = t
		}
	default:
		return parsedArtifactRef{}, false
	}
	if parsed.ObjectRef == "" {
		return parsedArtifactRef{}, false
	}
	if parsed.Title == "" {
		parsed.Title = parsed.ObjectRef
	}
	return parsed, true
}

func firstRefString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := m[key].(string); ok {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

type completionContractValidation struct {
	MissingOutputs     []string
	MissingHandoffRefs []string
}

func (v completionContractValidation) Satisfied() bool {
	return len(v.MissingOutputs) == 0 && len(v.MissingHandoffRefs) == 0
}

func (s *Service) projectTaskRunWorkProducts(ctx context.Context, tenantID uuid.UUID, task ProjectTask) ([]any, error) {
	if task.DigitalEmployeeRunID == nil {
		return []any{}, nil
	}
	runRepository, ok := s.repository.(ProjectTaskRunWorkProductRepository)
	if !ok {
		return []any{}, nil
	}
	workProducts, err := runRepository.GetProjectTaskRunWorkProducts(ctx, tenantID, *task.DigitalEmployeeRunID)
	if errors.Is(err, ErrProjectNotFound) {
		return []any{}, nil
	}
	return workProducts, err
}

func validateProjectTaskCompletionContract(task ProjectTask, req CompleteProjectTaskRequest, runWorkProducts []any) completionContractValidation {
	required := stringSetFromAny(task.ExpectedOutputs)
	missingOutputs := make([]string, 0)
	if required["execution_summary"] && strings.TrimSpace(req.Conclusion) == "" {
		missingOutputs = append(missingOutputs, "execution_summary")
	}
	if required["evidence_refs"] && len(req.EvidenceRefs) == 0 {
		missingOutputs = append(missingOutputs, "evidence_refs")
	}
	if required["artifact_refs"] && len(req.ArtifactRefs) == 0 {
		missingOutputs = append(missingOutputs, "artifact_refs")
	}
	if required["recommended_next_action"] && strings.TrimSpace(req.RecommendedNextAction) == "" {
		missingOutputs = append(missingOutputs, "recommended_next_action")
	}
	if required["missing_information"] && req.MissingInformation == nil {
		missingOutputs = append(missingOutputs, "missing_information")
	}
	if required["work_products"] && len(runWorkProducts) == 0 {
		missingOutputs = append(missingOutputs, "work_products")
	}
	return completionContractValidation{
		MissingOutputs:     missingOutputs,
		MissingHandoffRefs: missingRequiredHandoffRefs(task.HandoffContract, req, runWorkProducts),
	}
}

func (s *Service) appendProjectTaskContractMissingEvent(ctx context.Context, task ProjectTask, req CompleteProjectTaskRequest, validation completionContractValidation) error {
	payload := map[string]any{
		"project_task_id": task.ID.String(),
		"missing_outputs": stringsToAny(validation.MissingOutputs),
	}
	if len(validation.MissingHandoffRefs) > 0 {
		payload["missing_handoff_refs"] = stringsToAny(validation.MissingHandoffRefs)
	}
	_, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    task.ProjectID,
		EventType:    ProjectEventTaskContractMissing,
		ActorType:    "digital_employee",
		ActorID:      req.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务完成输出未满足交接契约",
		Payload:      payload,
	})
	return err
}

func stringSetFromAny(values []any) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			result[text] = true
		}
	}
	return result
}

func missingRequiredHandoffRefs(contract map[string]any, req CompleteProjectTaskRequest, runWorkProducts []any) []string {
	requiredRefs := requiredHandoffRefs(contract)
	if len(requiredRefs) == 0 {
		return []string{}
	}
	available := referenceTokenSet(req.EvidenceRefs, req.ArtifactRefs, runWorkProducts)
	missing := make([]string, 0)
	for _, ref := range requiredRefs {
		if !available[ref] {
			missing = append(missing, ref)
		}
	}
	return missing
}

func requiredHandoffRefs(contract map[string]any) []string {
	raw, ok := contract["required_refs"]
	if !ok {
		return []string{}
	}
	switch refs := raw.(type) {
	case []string:
		return normalizedStringRefs(refs)
	case []any:
		result := make([]string, 0, len(refs))
		for _, ref := range refs {
			if text, ok := ref.(string); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					result = append(result, text)
				}
			}
		}
		return result
	default:
		return []string{}
	}
}

func normalizedStringRefs(refs []string) []string {
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			result = append(result, ref)
		}
	}
	return result
}

func referenceTokenSet(groups ...[]any) map[string]bool {
	tokens := map[string]bool{}
	for _, group := range groups {
		for _, value := range group {
			addReferenceTokens(tokens, value)
		}
	}
	return tokens
}

func addReferenceTokens(tokens map[string]bool, value any) {
	switch typed := value.(type) {
	case string:
		addReferenceToken(tokens, typed)
	case map[string]any:
		for _, key := range []string{"ref", "id", "title", "type"} {
			if text, ok := typed[key].(string); ok {
				addReferenceToken(tokens, text)
			}
		}
	}
}

func addReferenceToken(tokens map[string]bool, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		tokens[value] = true
	}
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
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

func (s *Service) FailProjectTaskAttempt(ctx context.Context, req FailProjectTaskAttemptRequest) (*ProjectTask, error) {
	req.FailureSummary = strings.TrimSpace(req.FailureSummary)
	req.FailureFamily = strings.TrimSpace(req.FailureFamily)
	if req.FailureSummary == "" || req.FailureFamily == "" {
		return nil, ErrInvalidProject
	}
	task, attempt, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest)
	if err != nil {
		return nil, err
	}
	digitalEmployeeID, err := digitalEmployeeIDForProjectTask(task)
	if err != nil {
		return nil, err
	}
	req.DigitalEmployeeID = digitalEmployeeID
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	action := projectTaskFailureAction(task, req.FailureFamily, req.Retryable)
	result, err := writebackRepository.RecoverProjectTaskAttemptFailureWriteback(ctx, RecoverProjectTaskAttemptFailureWritebackRequest{
		Task:                  task,
		Attempt:               attempt,
		Failure:               req,
		AttemptTerminalStatus: projectTaskAttemptFailureStatus(req.FailureFamily),
		TaskTargetStatus:      action,
		WaitingReason:         humanWaitReasonForFailureFamily(req.FailureFamily),
		RetryAttemptID:        uuid.New(),
		RetryLeaseToken:       "retry-" + uuid.NewString(),
		RetryIdempotencyKey:   projectTaskRetryIdempotencyKey(task, req.IdempotencyKey),
	})
	if err != nil {
		return nil, err
	}
	if result.Task.Status != ProjectTaskStatusFailed {
		return &result.Task, nil
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, task.ProjectID)
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

func (s *Service) WaitHumanProjectTaskAttempt(ctx context.Context, req WaitHumanProjectTaskAttemptRequest) (*ProjectTask, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	req.Summary = strings.TrimSpace(req.Summary)
	if req.Reason == "" || req.Summary == "" || !validHumanWaitReason(req.Reason) {
		return nil, ErrInvalidProject
	}
	task, attempt, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest)
	if err != nil {
		return nil, err
	}
	digitalEmployeeID, err := digitalEmployeeIDForProjectTask(task)
	if err != nil {
		return nil, err
	}
	if req.DigitalEmployeeID != uuid.Nil && req.DigitalEmployeeID != digitalEmployeeID {
		return nil, ErrProjectTaskForbidden
	}
	req.DigitalEmployeeID = digitalEmployeeID
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, task.ProjectID)
	if err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.WaitHumanProjectTaskAttemptWriteback(ctx, WaitHumanProjectTaskAttemptWritebackRequest{
		Task:    task,
		Attempt: attempt,
		Wait:    req,
		Decision: CreateDecisionRequestRequest{
			TenantID:          req.TenantID,
			ProjectID:         task.ProjectID,
			ApprovalRequestID: uuid.Nil,
			CoordinationJobID: task.CoordinationJobID,
			ProjectTaskID:     &task.ID,
			TargetUserID:      projectRecord.HumanOwnerUserID,
			DecisionType:      projectTaskHumanWaitDecisionType(req.Reason),
			TitleSnapshot:     task.Title,
			SummarySnapshot:   req.Summary,
			RiskLevelSnapshot: stringValue(task.RiskLevel),
			StatusSnapshot:    "pending",
		},
	})
	if err != nil {
		return nil, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, result.Decision); err != nil {
			return nil, err
		}
	}
	return &result.Task, nil
}

func (s *Service) ResolveProjectTaskHumanWait(ctx context.Context, req ResolveProjectTaskHumanWaitRequest) (*ProjectTask, error) {
	req.Resolution = strings.TrimSpace(req.Resolution)
	req.ResponseSummary = strings.TrimSpace(req.ResponseSummary)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.ActorUserID == uuid.Nil || req.ResponseSummary == "" || !validHumanWaitResolution(req.Resolution) {
		return nil, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	if task.ProjectID != req.ProjectID || task.Status != ProjectTaskStatusWaitingHuman {
		return nil, ErrProjectConflict
	}
	if req.Resolution == HumanWaitResolutionApprove && (task.WaitingReason == nil || *task.WaitingReason != HumanWaitReasonAcceptanceRequired) {
		return nil, ErrProjectConflict
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if req.ActorUserID != projectRecord.HumanOwnerUserID {
		return nil, ErrProjectTaskForbidden
	}
	var currentAttempt ProjectTaskAttempt
	if task.CurrentAttemptID != nil {
		currentAttempt, _ = s.repository.GetProjectTaskAttempt(ctx, req.TenantID, *task.CurrentAttemptID)
	}
	targetStatus := projectTaskHumanWaitResolutionStatus(req.Resolution)
	resolutionRepository, err := s.projectTaskHumanWaitResolutionRepository()
	if err != nil {
		return nil, err
	}
	result, err := resolutionRepository.ResolveProjectTaskHumanWaitWriteback(ctx, ResolveProjectTaskHumanWaitWritebackRequest{
		Task:                task,
		CurrentAttempt:      currentAttempt,
		Resolve:             req,
		TargetStatus:        targetStatus,
		RetryAttemptID:      uuid.New(),
		RetryLeaseToken:     "human-wait-" + uuid.NewString(),
		RetryIdempotencyKey: fmt.Sprintf("project-task:%s:attempt:%d:human-wait:%s", task.ID, task.AttemptCount+1, req.Resolution),
	})
	if err != nil {
		return nil, err
	}
	return &result.Task, nil
}

func projectTaskRequiresAcceptance(task ProjectTask, req CompleteProjectTaskAttemptRequest) bool {
	if task.RequiresHumanApproval {
		return true
	}
	if task.RiskLevel != nil {
		switch strings.ToLower(strings.TrimSpace(*task.RiskLevel)) {
		case "high", "critical":
			return true
		}
	}
	return req.RequiresHumanReview
}

func projectTaskFailureAction(task ProjectTask, failureFamily string, retryable *bool) string {
	if retryable != nil && !*retryable {
		if failureFamily == FailureFamilyBusinessCancelled || failureFamily == FailureFamilyPlanInvalid || failureFamily == FailureFamilyRequirementChanged {
			return ProjectTaskStatusCancelled
		}
		return ProjectTaskStatusFailed
	}
	switch failureFamily {
	case FailureFamilyTransientRuntime, FailureFamilyTransientProvider, FailureFamilyTimeout:
		maxAttempts := int32(1)
		if task.MaxAttempts != nil {
			maxAttempts = *task.MaxAttempts
		}
		if task.AttemptCount < maxAttempts {
			return ProjectTaskStatusQueued
		}
		return ProjectTaskStatusWaitingHuman
	case FailureFamilyInvalidContract, FailureFamilyApprovalRequired, FailureFamilyPermissionRequired, FailureFamilyAcceptanceRequired:
		return ProjectTaskStatusWaitingHuman
	case FailureFamilyBusinessCancelled, FailureFamilyPlanInvalid, FailureFamilyRequirementChanged:
		return ProjectTaskStatusCancelled
	default:
		return ProjectTaskStatusFailed
	}
}

func projectTaskAttemptFailureStatus(failureFamily string) string {
	switch failureFamily {
	case FailureFamilyTimeout:
		return ProjectTaskAttemptStatusTimedOut
	case FailureFamilyTransientRuntime:
		return ProjectTaskAttemptStatusLost
	default:
		return ProjectTaskAttemptStatusFailed
	}
}

func humanWaitReasonForFailureFamily(failureFamily string) string {
	switch failureFamily {
	case FailureFamilyApprovalRequired:
		return HumanWaitReasonApprovalRequired
	case FailureFamilyPermissionRequired:
		return HumanWaitReasonPermissionRequired
	case FailureFamilyInvalidContract, FailureFamilyPlanInvalid:
		return HumanWaitReasonPlanInvalid
	case FailureFamilyAcceptanceRequired:
		return HumanWaitReasonAcceptanceRequired
	default:
		return HumanWaitReasonClarification
	}
}

func projectTaskRetryIdempotencyKey(task ProjectTask, failureIdempotencyKey string) string {
	return fmt.Sprintf("project-task:%s:attempt:%d:retry:%s", task.ID, task.AttemptCount+1, failureIdempotencyKey)
}

func validHumanWaitReason(reason string) bool {
	switch reason {
	case HumanWaitReasonMissingContext,
		HumanWaitReasonClarification,
		HumanWaitReasonApprovalRequired,
		HumanWaitReasonPermissionRequired,
		HumanWaitReasonPlanInvalid,
		HumanWaitReasonAcceptanceRequired,
		HumanWaitReasonRuntimeRecovery,
		HumanWaitReasonBudgetApproval:
		return true
	default:
		return false
	}
}

func validHumanWaitResolution(resolution string) bool {
	switch resolution {
	case HumanWaitResolutionApprove,
		HumanWaitResolutionResumeSameTask,
		HumanWaitResolutionCancelAndReplan,
		HumanWaitResolutionCancelWithoutPlan,
		HumanWaitResolutionMarkFailed:
		return true
	default:
		return false
	}
}

func projectTaskHumanWaitResolutionStatus(resolution string) string {
	switch resolution {
	case HumanWaitResolutionApprove:
		return ProjectTaskStatusCompleted
	case HumanWaitResolutionResumeSameTask:
		return ProjectTaskStatusQueued
	case HumanWaitResolutionCancelAndReplan, HumanWaitResolutionCancelWithoutPlan:
		return ProjectTaskStatusCancelled
	case HumanWaitResolutionMarkFailed:
		return ProjectTaskStatusFailed
	default:
		return ""
	}
}

func projectTaskHumanWaitDecisionType(reason string) string {
	switch reason {
	case HumanWaitReasonMissingContext:
		return "project_task_missing_context"
	case HumanWaitReasonClarification:
		return "project_task_clarification"
	case HumanWaitReasonApprovalRequired:
		return "project_task_approval"
	case HumanWaitReasonPermissionRequired:
		return "project_task_permission"
	case HumanWaitReasonPlanInvalid:
		return "project_task_plan_invalid"
	case HumanWaitReasonAcceptanceRequired:
		return "project_task_acceptance"
	case HumanWaitReasonRuntimeRecovery:
		return "project_task_runtime_recovery"
	case HumanWaitReasonBudgetApproval:
		return "project_task_budget_approval"
	default:
		return "project_task_human_wait"
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
	if decision.StatusSnapshot != "pending" {
		if decision.StatusSnapshot == req.Decision {
			if err := s.resolveProjectTaskAcceptanceDecision(ctx, decision, req); err != nil {
				return nil, err
			}
			return &decision, nil
		}
		return nil, ErrInvalidProject
	}
	if s.approvals != nil && decision.ApprovalRequestID != uuid.Nil {
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
		Payload:      map[string]any{"decision": req.Decision, "comment": req.Comment, "payload": mapOrEmptyAny(req.Payload)},
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
	if s.inbox != nil {
		if err := s.inbox.ResolveProjectDecisionRequest(ctx, resolved); err != nil {
			return nil, err
		}
	}
	if err := s.resolveProjectTaskAcceptanceDecision(ctx, resolved, req); err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalHumanDecisionSubmitted(ctx, HumanDecisionSubmittedSignal{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: decision.ApprovalRequestID,
		DecisionRequestID: req.DecisionRequestID,
		Decision:          req.Decision,
		Payload:           mapOrEmptyAny(req.Payload),
		ResolvedEventID:   event.ID,
		WorkflowID:        projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, "HumanDecisionSubmitted", "failed", err, map[string]any{
			"approval_request_id": decision.ApprovalRequestID.String(),
			"decision_request_id": req.DecisionRequestID.String(),
			"resolved_event_id":   event.ID.String(),
			"decision":            req.Decision,
			"payload":             mapOrEmptyAny(req.Payload),
		})
		return nil, err
	}
	return &resolved, nil
}

func (s *Service) resolveProjectTaskAcceptanceDecision(ctx context.Context, decision DecisionRequest, req ResolveDecisionRequest) error {
	if decision.DecisionType != "project_task_acceptance" || req.Decision != "approved" || decision.ProjectTaskID == nil {
		return nil
	}
	_, err := s.ResolveProjectTaskHumanWait(ctx, ResolveProjectTaskHumanWaitRequest{
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ProjectTaskID:   *decision.ProjectTaskID,
		ActorUserID:     req.DecidedByUserID,
		Resolution:      HumanWaitResolutionApprove,
		ResponseSummary: projectTaskAcceptanceResponseSummary(req.Comment),
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrProjectConflict) {
		task, getErr := s.repository.GetProjectTask(ctx, req.TenantID, *decision.ProjectTaskID)
		if getErr == nil && task.ProjectID == req.ProjectID && task.Status == ProjectTaskStatusCompleted {
			return nil
		}
	}
	return err
}

func projectTaskAcceptanceResponseSummary(comment string) string {
	comment = strings.TrimSpace(comment)
	if comment != "" {
		return comment
	}
	return "任务验收通过"
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
	if err := s.ensureProjectCoordinator(ctx, projectRecord); err != nil {
		retryPayload := cloneMap(event.Payload)
		retryPayload["retry_of_event_id"] = req.EventID.String()
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, signalName, "failed", err, retryPayload)
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

func (s *Service) ensureProjectCoordinator(ctx context.Context, projectRecord Project) error {
	return s.coordinator.EnsureProjectCoordinator(ctx, ProjectCoordinatorSignal{
		TenantID:   projectRecord.TenantID,
		ProjectID:  projectRecord.ID,
		WorkflowID: projectRecord.CoordinationWorkflowID,
	})
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
			Payload:           mapFromPayload(payload, "payload"),
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

func (s *Service) validateAttemptRuntimeRequest(ctx context.Context, req ProjectTaskAttemptRuntimeRequest) (ProjectTask, ProjectTaskAttempt, error) {
	req.LeaseToken = strings.TrimSpace(req.LeaseToken)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.TenantID == uuid.Nil || req.AttemptID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.RuntimeNodeID == uuid.Nil {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrInvalidProject
	}
	if req.LeaseToken == "" || req.IdempotencyKey == "" {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return ProjectTask{}, ProjectTaskAttempt{}, err
	}
	if task.CurrentAttemptID == nil || *task.CurrentAttemptID != req.AttemptID {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	if !projectTaskAcceptsRuntimeWriteback(task.Status) {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID)
	if err != nil {
		return ProjectTask{}, ProjectTaskAttempt{}, err
	}
	if attempt.ProjectTaskID != req.ProjectTaskID || attempt.LeaseToken != req.LeaseToken {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	if attempt.RuntimeNodeID != nil && *attempt.RuntimeNodeID != req.RuntimeNodeID {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	return task, attempt, nil
}

func digitalEmployeeIDForProjectTask(task ProjectTask) (uuid.UUID, error) {
	if task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID == uuid.Nil {
		return uuid.Nil, ErrProjectTaskForbidden
	}
	return *task.AssignedDigitalEmployeeID, nil
}

func projectTaskAcceptsRuntimeWriteback(status string) bool {
	switch status {
	case "assigned", "queued", "running":
		return true
	default:
		return false
	}
}

func runtimeWritebackProjectTaskStatuses() []string {
	return []string{"assigned", "queued", "running"}
}

func (s *Service) projectTaskWritebackRepository() (ProjectTaskWritebackRepository, error) {
	repository, ok := s.repository.(ProjectTaskWritebackRepository)
	if !ok {
		return nil, fmt.Errorf("project repository does not support atomic project task writeback")
	}
	return repository, nil
}

func (s *Service) projectTaskAttemptWritebackRepository() (ProjectTaskAttemptWritebackRepository, error) {
	repository, ok := s.repository.(ProjectTaskAttemptWritebackRepository)
	if !ok {
		return nil, fmt.Errorf("project repository does not support atomic project task attempt writeback")
	}
	return repository, nil
}

func (s *Service) projectTaskHumanWaitResolutionRepository() (ProjectTaskHumanWaitResolutionRepository, error) {
	repository, ok := s.repository.(ProjectTaskHumanWaitResolutionRepository)
	if !ok {
		return nil, fmt.Errorf("project repository does not support atomic project task human wait resolution")
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

func mapFromPayload(payload map[string]any, key string) map[string]any {
	value, ok := payload[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func projectTaskExecutionPacketMap(packet ProjectTaskExecutionPacket) map[string]any {
	dependencyOutputs := make([]any, 0, len(packet.DependencyOutputs))
	for _, output := range packet.DependencyOutputs {
		dependencyOutputs = append(dependencyOutputs, map[string]any{
			"project_task_id": output.ProjectTaskID,
			"conclusion":      output.Conclusion,
			"evidence_refs":   append([]any(nil), output.EvidenceRefs...),
			"artifact_refs":   append([]any(nil), output.ArtifactRefs...),
		})
	}
	humanDecisionRefs := make([]any, 0, len(packet.HumanDecisionRefs))
	for _, ref := range packet.HumanDecisionRefs {
		humanDecisionRefs = append(humanDecisionRefs, map[string]any{
			"decision_request_id": ref.DecisionRequestID,
			"decision_type":       ref.DecisionType,
			"status_snapshot":     ref.StatusSnapshot,
		})
	}
	forbiddenScopes := make([]any, 0, len(packet.ForbiddenScopes))
	for _, scope := range packet.ForbiddenScopes {
		forbiddenScopes = append(forbiddenScopes, scope)
	}
	stopForHumanCriteria := make([]any, 0, len(packet.StopForHumanCriteria))
	for _, criterion := range packet.StopForHumanCriteria {
		stopForHumanCriteria = append(stopForHumanCriteria, criterion)
	}
	return map[string]any{
		"version":                 packet.Version,
		"project_id":              packet.ProjectID,
		"project_task_id":         packet.ProjectTaskID,
		"title":                   packet.Title,
		"summary":                 packet.Summary,
		"expected_outputs":        append([]any(nil), packet.ExpectedOutputs...),
		"input_requirements":      cloneMap(mapOrEmptyAny(packet.InputRequirements)),
		"handoff_contract":        cloneMap(mapOrEmptyAny(packet.HandoffContract)),
		"dependency_outputs":      dependencyOutputs,
		"human_decision_refs":     humanDecisionRefs,
		"forbidden_scopes":        forbiddenScopes,
		"risk_level":              packet.RiskLevel,
		"stop_for_human_criteria": stopForHumanCriteria,
	}
}

func projectTaskContextUpdateDeliveryMode(task ProjectTask, updateKind string) string {
	switch strings.TrimSpace(updateKind) {
	case "requirement_changed", "plan_invalid", "scope_changed":
		return ContextUpdateDeliveryCancelAndReplan
	case "comment", "additional_context", "evidence_ref":
		if task.Status == ProjectTaskStatusWaitingHuman {
			return ContextUpdateDeliveryWaitingHuman
		}
		return ContextUpdateDeliveryNextAttempt
	default:
		return ContextUpdateDeliveryNextAttempt
	}
}

func classifyProjectTaskLiveness(item *ProjectTaskLiveness, task ProjectTask, now time.Time) {
	switch task.Status {
	case ProjectTaskStatusCompleted, ProjectTaskStatusFailed, ProjectTaskStatusCancelled:
		item.Liveness = ProjectTaskLivenessTerminal
		item.NextAction = "no-op terminal"
		return
	}
	if task.RetryNotBefore != nil && task.RetryNotBefore.After(now) {
		item.Liveness = ProjectTaskLivenessRetryScheduled
		item.NextAction = "retry wakeup"
		return
	}
	if len(item.BlockingDependencyIDs) > 0 {
		item.Liveness = ProjectTaskLivenessBlockedByDependency
		item.NextAction = "dependency completion"
		return
	}
	switch task.Status {
	case ProjectTaskStatusQueued:
		item.Liveness = ProjectTaskLivenessQueued
		item.NextAction = "runtime start"
	case ProjectTaskStatusRunning:
		if item.LeaseExpiresAt != nil && item.LeaseExpiresAt.Before(now) {
			item.Liveness = ProjectTaskLivenessLeaseLost
			item.NextAction = "recovery policy"
			return
		}
		item.Liveness = ProjectTaskLivenessRunning
		item.NextAction = "lease renew"
	case ProjectTaskStatusWaitingHuman:
		item.Liveness = ProjectTaskLivenessWaitingHuman
		item.NextAction = "human response"
		if task.WaitingReason != nil {
			item.Reason = *task.WaitingReason
		}
	default:
		item.Liveness = ProjectTaskLivenessReadyToDispatch
		item.NextAction = "dispatch"
	}
}

func validHumanDecision(decision string) bool {
	switch decision {
	case "approved", "rejected", "needs_more_evidence":
		return true
	default:
		return false
	}
}

func validAcceptanceStatus(status string) bool {
	switch status {
	case "accepted", "rejected", "needs_more_evidence", "partially_accepted":
		return true
	default:
		return false
	}
}

func validEvidenceVerificationStatus(status EvidenceVerificationStatus) bool {
	switch status {
	case EvidenceVerificationStatusSubmitted, EvidenceVerificationStatusLinked, EvidenceVerificationStatusVerified, EvidenceVerificationStatusRejected, EvidenceVerificationStatusSuperseded:
		return true
	default:
		return false
	}
}

func projectArchived(project Project) bool {
	return project.Status == ProjectStatusArchived || project.ArchivedAt != nil
}

func collectArchivePreviewPages[T any](ctx context.Context, pageSize int32, list func(limit, offset int32) ([]T, error)) ([]T, error) {
	pageSize, offset := normalizePagination(pageSize, 0)
	values := make([]T, 0)
	for page := 0; page < 10000; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows, err := list(pageSize, offset)
		if err != nil {
			return nil, err
		}
		values = append(values, rows...)
		if int32(len(rows)) < pageSize {
			return values, nil
		}
		nextOffset := offset + int32(len(rows))
		if nextOffset <= offset {
			return nil, ErrInvalidProject
		}
		offset = nextOffset
	}
	return nil, ErrInvalidProject
}

func (s *Service) collectArchiveArtifactIDs(ctx context.Context, tenantID, projectID uuid.UUID) ([]uuid.UUID, error) {
	pageSize, _ := normalizePagination(100, 0)
	artifactRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectArtifactRef, error) {
		return s.repository.ListArtifactRefs(ctx, tenantID, projectID, limit, offset)
	})
	if err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]struct{}, len(artifactRefs))
	artifactIDs := make([]uuid.UUID, 0, len(artifactRefs))
	for _, artifactRef := range artifactRefs {
		if artifactRef.ArtifactID == nil || *artifactRef.ArtifactID == uuid.Nil {
			continue
		}
		if _, ok := seen[*artifactRef.ArtifactID]; ok {
			continue
		}
		seen[*artifactRef.ArtifactID] = struct{}{}
		artifactIDs = append(artifactIDs, *artifactRef.ArtifactID)
	}
	return artifactIDs, nil
}

func archiveSnapshotIncludedCounts(preview *ProjectArchivePreview) map[string]any {
	if preview == nil {
		return map[string]any{}
	}
	return map[string]any{
		"evidence_ref_count": preview.EvidenceCount,
		"artifact_ref_count": preview.ArtifactCount,
		"report_ref_count":   preview.ReportCount,
	}
}

func (s *Service) validateAcceptanceRefs(ctx context.Context, tenantID, projectID uuid.UUID, evidenceRefIDs, reportRefIDs []uuid.UUID) error {
	for _, id := range evidenceRefIDs {
		if id == uuid.Nil {
			return ErrInvalidProjectAcceptance
		}
	}
	for _, id := range reportRefIDs {
		if id == uuid.Nil {
			return ErrInvalidProjectAcceptance
		}
	}
	pageSize, _ := normalizePagination(100, 0)
	evidenceRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectEvidenceRef, error) {
		return s.repository.ListEvidenceRefs(ctx, tenantID, projectID, nil, limit, offset)
	})
	if err != nil {
		return err
	}
	reportRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectReportRef, error) {
		return s.repository.ListReportRefs(ctx, tenantID, projectID, limit, offset)
	})
	if err != nil {
		return err
	}
	evidenceIDs := make(map[uuid.UUID]struct{}, len(evidenceRefs))
	for _, evidence := range evidenceRefs {
		evidenceIDs[evidence.ID] = struct{}{}
	}
	for _, id := range evidenceRefIDs {
		if _, ok := evidenceIDs[id]; !ok {
			return ErrInvalidProjectAcceptance
		}
	}
	reportIDs := make(map[uuid.UUID]struct{}, len(reportRefs))
	for _, report := range reportRefs {
		reportIDs[report.ID] = struct{}{}
	}
	for _, id := range reportRefIDs {
		if _, ok := reportIDs[id]; !ok {
			return ErrInvalidProjectAcceptance
		}
	}
	return nil
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

func normalizeWorkflowInstancePagination(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func normalizeWorkflowInstanceStatus(item WorkflowInstanceSummary) WorkflowInstanceStatus {
	switch item.Status {
	case WorkflowInstanceStatusFailed, WorkflowInstanceStatusCancelled:
		return item.Status
	}
	if item.Progress.WaitingHumanNodes > 0 {
		return WorkflowInstanceStatusWaitingHuman
	}
	if item.Progress.RunningNodes > 0 {
		return WorkflowInstanceStatusRunning
	}
	if item.Progress.TotalNodes == 0 {
		return WorkflowInstanceStatusPlanning
	}
	if item.Progress.CompletedNodes == item.Progress.TotalNodes {
		return WorkflowInstanceStatusCompleted
	}
	if item.Status != "" {
		return item.Status
	}
	return WorkflowInstanceStatusUnknown
}

func workflowInstanceAttentionRank(status WorkflowInstanceStatus) int {
	switch status {
	case WorkflowInstanceStatusWaitingHuman:
		return 0
	case WorkflowInstanceStatusFailed:
		return 1
	case WorkflowInstanceStatusRunning:
		return 2
	case WorkflowInstanceStatusPlanning:
		return 3
	case WorkflowInstanceStatusUnknown:
		return 4
	case WorkflowInstanceStatusCompleted:
		return 5
	case WorkflowInstanceStatusCancelled:
		return 6
	default:
		return 7
	}
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
