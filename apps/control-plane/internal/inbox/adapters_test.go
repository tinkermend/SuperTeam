package inbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)

func TestApprovalProjectorAdapterUpsertsApprovalRequest(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewApprovalProjectorAdapter(service)
	summary := "Approve high risk deployment"
	risk := "high"
	createdAt := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	request := approval.ApprovalRequest{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ResourceType:   "project",
		ResourceID:     uuid.New(),
		TargetUserID:   uuid.New(),
		DecisionType:   "deploy",
		Title:          "Deployment approval",
		Summary:        &summary,
		RiskLevel:      &risk,
		Status:         approval.ApprovalStatusPending,
		ContextPayload: map[string]any{"change": "deploy"},
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}

	if err := adapter.UpsertApprovalRequest(context.Background(), request); err != nil {
		t.Fatalf("upsert approval request: %v", err)
	}
	item, err := repo.GetItem(context.Background(), request.TenantID, repo.itemsByApproval[request.ID])
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusOpen || item.ItemType != ItemTypeApproval || item.SourceType != SourceTypeApprovalRequest {
		t.Fatalf("unexpected projected item: %#v", item)
	}
	if item.SourceID != request.ID || item.SourceApprovalRequestID == nil || *item.SourceApprovalRequestID != request.ID {
		t.Fatalf("expected source approval request id %s, got source=%s approval=%#v", request.ID, item.SourceID, item.SourceApprovalRequestID)
	}
	if item.Summary == nil || *item.Summary != summary || item.RiskLevel == nil || *item.RiskLevel != risk {
		t.Fatalf("expected summary and risk from request, got summary=%#v risk=%#v", item.Summary, item.RiskLevel)
	}
	if !item.LastActivityAt.Equal(updatedAt) {
		t.Fatalf("expected last activity %s, got %s", updatedAt, item.LastActivityAt)
	}
	if item.DeepLink["route"] != "/approvals" || item.DeepLink["approval_request_id"] != request.ID.String() {
		t.Fatalf("unexpected deep link: %#v", item.DeepLink)
	}
	if item.ContextPayload["change"] != "deploy" || len(item.Actions) == 0 {
		t.Fatalf("expected context payload and default actions, got %#v actions=%#v", item.ContextPayload, item.Actions)
	}
}

func TestApprovalProjectorAdapterResolvesApprovalRequest(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewApprovalProjectorAdapter(service)
	resolvedAt := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	request := approval.ApprovalRequest{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ResourceID:     uuid.New(),
		TargetUserID:   uuid.New(),
		DecisionType:   "deploy",
		Title:          "Deployment approval",
		Status:         approval.ApprovalStatusApproved,
		ContextPayload: map[string]any{},
		CreatedAt:      resolvedAt.Add(-time.Hour),
		UpdatedAt:      resolvedAt,
		ResolvedAt:     &resolvedAt,
	}

	if err := adapter.ResolveApprovalRequest(context.Background(), request); err != nil {
		t.Fatalf("resolve approval request: %v", err)
	}
	item, err := repo.GetItem(context.Background(), request.TenantID, repo.itemsByApproval[request.ID])
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusResolved || item.ResolvedAt == nil || !item.ResolvedAt.Equal(resolvedAt) {
		t.Fatalf("expected resolved item at %s, got status=%s resolved=%#v", resolvedAt, item.Status, item.ResolvedAt)
	}
}

func TestDecisionProjectorAdapterUpsertsProjectDecision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	summary := "Needs owner approval"
	risk := "medium"
	taskID := uuid.New()
	approvalID := uuid.New()
	createdAt := time.Date(2026, 6, 12, 13, 0, 0, 0, time.UTC)
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		ApprovalRequestID: approvalID,
		ProjectTaskID:     &taskID,
		TargetUserID:      uuid.New(),
		DecisionType:      "route",
		TitleSnapshot:     "Review route decision",
		SummarySnapshot:   &summary,
		RiskLevelSnapshot: &risk,
		StatusSnapshot:    "pending",
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt.Add(time.Minute),
	}

	if err := adapter.UpsertProjectDecisionRequest(context.Background(), decision); err != nil {
		t.Fatalf("upsert project decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusOpen || item.ItemType != ItemTypeProjectDecision || item.SourceType != SourceTypeProjectDecisionRequest {
		t.Fatalf("unexpected projected item: %#v", item)
	}
	if item.SourceProjectID == nil || *item.SourceProjectID != decision.ProjectID || item.SourceTaskID == nil || *item.SourceTaskID != taskID {
		t.Fatalf("expected project/task source ids, got project=%#v task=%#v", item.SourceProjectID, item.SourceTaskID)
	}
	if item.SourceApprovalRequestID == nil || *item.SourceApprovalRequestID != approvalID {
		t.Fatalf("expected approval source id %s, got %#v", approvalID, item.SourceApprovalRequestID)
	}
	if item.DeepLink["route"] != "/projects/"+decision.ProjectID.String() || item.DeepLink["anchor"] != decision.ID.String() {
		t.Fatalf("unexpected deep link: %#v", item.DeepLink)
	}
	if item.Summary == nil || *item.Summary != summary || item.RiskLevel == nil || *item.RiskLevel != risk || len(item.Actions) == 0 {
		t.Fatalf("expected decision details, got summary=%#v risk=%#v actions=%#v", item.Summary, item.RiskLevel, item.Actions)
	}
}

func TestDecisionProjectorAdapterResolvesProjectDecision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	resolvedAt := time.Date(2026, 6, 12, 14, 0, 0, 0, time.UTC)
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		ApprovalRequestID: uuid.New(),
		TargetUserID:      uuid.New(),
		DecisionType:      "route",
		TitleSnapshot:     "Review route decision",
		StatusSnapshot:    "approved",
		CreatedAt:         resolvedAt.Add(-time.Hour),
		UpdatedAt:         resolvedAt,
		ResolvedAt:        &resolvedAt,
	}

	if err := adapter.ResolveProjectDecisionRequest(context.Background(), decision); err != nil {
		t.Fatalf("resolve project decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusResolved || item.ResolvedAt == nil || !item.ResolvedAt.Equal(resolvedAt) {
		t.Fatalf("expected resolved item at %s, got status=%s resolved=%#v", resolvedAt, item.Status, item.ResolvedAt)
	}
}

func TestApprovalActionAdapterResolvesApproval(t *testing.T) {
	repo := &approvalActionRepository{}
	service, err := approval.NewService(repo)
	if err != nil {
		t.Fatalf("new approval service: %v", err)
	}
	adapter := NewApprovalActionAdapter(service)
	sourceID := uuid.New()
	req := SourceActionRequest{
		TenantID:    uuid.New(),
		ActorUserID: uuid.New(),
		SourceID:    sourceID,
		Action:      "approved",
		Comment:     "ok",
		Payload:     map[string]any{"reason": "clear"},
	}

	result, err := adapter.ResolveApprovalAction(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve approval action: %v", err)
	}
	if repo.resolveInput.TenantID != req.TenantID || repo.resolveInput.ApprovalRequestID != sourceID || repo.resolveInput.DecidedByUserID != req.ActorUserID {
		t.Fatalf("unexpected approval resolve input: %#v", repo.resolveInput)
	}
	if repo.resolveInput.Decision != approval.ApprovalDecisionApproved || repo.resolveInput.Comment != "ok" || repo.resolveInput.Payload["reason"] != "clear" {
		t.Fatalf("unexpected approval decision input: %#v", repo.resolveInput)
	}
	if result.SourceType != string(SourceTypeApprovalRequest) || result.SourceID != sourceID || result.Status != "approved" {
		t.Fatalf("unexpected source result: %#v", result)
	}
}

func TestApprovalActionAdapterReturnsSourceUnavailableWithoutService(t *testing.T) {
	adapter := NewApprovalActionAdapter(nil)
	_, err := adapter.ResolveApprovalAction(context.Background(), SourceActionRequest{SourceID: uuid.New(), Action: "approved"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("expected source unavailable, got %v", err)
	}
}

func TestApprovalActionAdapterNormalizesSourceErrors(t *testing.T) {
	unknownErr := errors.New("approval store unavailable")
	tests := []struct {
		name    string
		source  error
		wantErr error
	}{
		{name: "invalid approval request", source: approval.ErrInvalidApprovalRequest, wantErr: ErrInvalidAction},
		{name: "approval not found", source: approval.ErrApprovalNotFound, wantErr: ErrSourceUnavailable},
		{name: "approval already resolved", source: approval.ErrApprovalAlreadyResolved, wantErr: ErrInvalidAction},
		{name: "unknown", source: unknownErr, wantErr: unknownErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &approvalActionRepository{resolveErr: tt.source}
			service, err := approval.NewService(repo)
			if err != nil {
				t.Fatalf("new approval service: %v", err)
			}
			adapter := NewApprovalActionAdapter(service)

			_, err = adapter.ResolveApprovalAction(context.Background(), SourceActionRequest{
				TenantID:    uuid.New(),
				ActorUserID: uuid.New(),
				SourceID:    uuid.New(),
				Action:      "approved",
			})

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestProjectDecisionActionAdapterResolvesDecision(t *testing.T) {
	projectID := uuid.New()
	decisionID := uuid.New()
	repo := &projectActionRepository{
		project: project.Project{
			ID:               projectID,
			TenantID:         uuid.New(),
			Name:             "Customer rollout",
			Goal:             "Ship safely",
			HumanOwnerUserID: uuid.New(),
		},
		decision: project.DecisionRequest{
			ID:                decisionID,
			ProjectID:         projectID,
			ApprovalRequestID: uuid.New(),
			TargetUserID:      uuid.New(),
			DecisionType:      "route",
			TitleSnapshot:     "Review route decision",
			StatusSnapshot:    "pending",
		},
	}
	repo.decision.TenantID = repo.project.TenantID
	service, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, project.NoopCoordinatorSignalClient{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("new project service: %v", err)
	}
	adapter := NewProjectDecisionActionAdapter(service)
	req := SourceActionRequest{
		TenantID:        repo.project.TenantID,
		ActorUserID:     uuid.New(),
		SourceID:        decisionID,
		SourceProjectID: &projectID,
		Action:          "approved",
		Comment:         "approved",
		Payload:         map[string]any{"reason": "clear"},
	}

	result, err := adapter.ResolveProjectDecisionAction(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve project decision action: %v", err)
	}
	if repo.resolveReq.TenantID != req.TenantID || repo.resolveReq.ProjectID != projectID || repo.resolveReq.ID != decisionID || repo.resolveReq.StatusSnapshot != "approved" {
		t.Fatalf("unexpected project resolve request: %#v", repo.resolveReq)
	}
	if repo.event.ActorID != req.ActorUserID.String() || repo.event.Payload["decision"] != "approved" {
		t.Fatalf("unexpected decision event: %#v", repo.event)
	}
	if result.SourceType != string(SourceTypeProjectDecisionRequest) || result.SourceID != decisionID || result.Status != "approved" {
		t.Fatalf("unexpected source result: %#v", result)
	}
}

func TestProjectDecisionActionAdapterReturnsSourceUnavailableWithoutProjectID(t *testing.T) {
	service, err := project.NewServiceWithCoordinator(&projectActionRepository{}, project.NoopCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new project service: %v", err)
	}
	adapter := NewProjectDecisionActionAdapter(service)
	_, err = adapter.ResolveProjectDecisionAction(context.Background(), SourceActionRequest{SourceID: uuid.New(), Action: "approved"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("expected source unavailable, got %v", err)
	}
}

func TestProjectDecisionActionAdapterNormalizesSourceErrors(t *testing.T) {
	unknownErr := errors.New("project store unavailable")
	tests := []struct {
		name    string
		mutate  func(*projectActionRepository)
		wantErr error
	}{
		{
			name: "invalid decision request",
			mutate: func(repo *projectActionRepository) {
				repo.getDecisionErr = project.ErrInvalidProject
			},
			wantErr: ErrInvalidAction,
		},
		{
			name: "project not found",
			mutate: func(repo *projectActionRepository) {
				repo.getProjectErr = project.ErrProjectNotFound
			},
			wantErr: ErrSourceUnavailable,
		},
		{
			name: "unknown",
			mutate: func(repo *projectActionRepository) {
				repo.getDecisionErr = unknownErr
			},
			wantErr: unknownErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectID := uuid.New()
			decisionID := uuid.New()
			repo := &projectActionRepository{
				project: project.Project{
					ID:               projectID,
					TenantID:         uuid.New(),
					Name:             "Customer rollout",
					Goal:             "Ship safely",
					HumanOwnerUserID: uuid.New(),
				},
				decision: project.DecisionRequest{
					ID:                decisionID,
					ProjectID:         projectID,
					ApprovalRequestID: uuid.New(),
					TargetUserID:      uuid.New(),
					DecisionType:      "route",
					TitleSnapshot:     "Review route decision",
					StatusSnapshot:    "pending",
				},
			}
			repo.decision.TenantID = repo.project.TenantID
			tt.mutate(repo)
			service, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, project.NoopCoordinatorSignalClient{}, nil, nil, nil)
			if err != nil {
				t.Fatalf("new project service: %v", err)
			}
			adapter := NewProjectDecisionActionAdapter(service)

			_, err = adapter.ResolveProjectDecisionAction(context.Background(), SourceActionRequest{
				TenantID:        repo.project.TenantID,
				ActorUserID:     uuid.New(),
				SourceID:        decisionID,
				SourceProjectID: &projectID,
				Action:          "approved",
			})

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

type approvalActionRepository struct {
	resolveInput approval.ResolveRequestInput
	resolveErr   error
}

func (r *approvalActionRepository) CreateApprovalRequest(_ context.Context, input approval.CreateRequestInput, status approval.ApprovalStatus) (approval.ApprovalRequest, error) {
	return approval.ApprovalRequest{
		ID:           uuid.New(),
		TenantID:     input.TenantID,
		ResourceID:   input.ResourceID,
		TargetUserID: input.TargetUserID,
		Title:        input.Title,
		Status:       status,
	}, nil
}

func (r *approvalActionRepository) GetApprovalRequest(_ context.Context, tenantID, requestID uuid.UUID) (approval.ApprovalRequest, error) {
	return approval.ApprovalRequest{ID: requestID, TenantID: tenantID}, nil
}

func (r *approvalActionRepository) ResolveApprovalRequest(_ context.Context, input approval.ResolveRequestInput, status approval.ApprovalStatus) (approval.ApprovalRequest, error) {
	if r.resolveErr != nil {
		return approval.ApprovalRequest{}, r.resolveErr
	}
	r.resolveInput = input
	return approval.ApprovalRequest{
		ID:           input.ApprovalRequestID,
		TenantID:     input.TenantID,
		TargetUserID: input.DecidedByUserID,
		Title:        "Resolved",
		Status:       status,
	}, nil
}

func (r *approvalActionRepository) CreateApprovalDecision(_ context.Context, input approval.ResolveRequestInput) (approval.ApprovalDecisionRecord, error) {
	return approval.ApprovalDecisionRecord{
		ID:                uuid.New(),
		TenantID:          input.TenantID,
		ApprovalRequestID: input.ApprovalRequestID,
		DecidedByUserID:   input.DecidedByUserID,
		Decision:          input.Decision,
		Payload:           input.Payload,
	}, nil
}

type projectActionRepository struct {
	project.Repository
	project        project.Project
	decision       project.DecisionRequest
	event          project.AppendProjectEventRequest
	resolveReq     project.ResolveDecisionRequestRepositoryRequest
	getProjectErr  error
	getDecisionErr error
	resolveErr     error
}

func (r projectActionRepository) GetProject(_ context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	if r.getProjectErr != nil {
		return project.Project{}, r.getProjectErr
	}
	if r.project.TenantID != tenantID || r.project.ID != projectID {
		return project.Project{}, project.ErrProjectNotFound
	}
	return r.project, nil
}

func (r projectActionRepository) GetDecisionRequest(_ context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (project.DecisionRequest, error) {
	if r.getDecisionErr != nil {
		return project.DecisionRequest{}, r.getDecisionErr
	}
	if r.decision.TenantID != tenantID || r.decision.ProjectID != projectID || r.decision.ID != decisionRequestID {
		return project.DecisionRequest{}, project.ErrInvalidProject
	}
	return r.decision, nil
}

func (r *projectActionRepository) AppendProjectEvent(_ context.Context, event project.AppendProjectEventRequest) (project.ProjectEvent, error) {
	r.event = event
	return project.ProjectEvent{
		ID:        uuid.New(),
		TenantID:  event.TenantID,
		ProjectID: event.ProjectID,
		EventType: event.EventType,
		ActorType: event.ActorType,
		ActorID:   event.ActorID,
		Payload:   event.Payload,
	}, nil
}

func (r *projectActionRepository) ResolveDecisionRequest(_ context.Context, req project.ResolveDecisionRequestRepositoryRequest) (project.DecisionRequest, error) {
	if r.resolveErr != nil {
		return project.DecisionRequest{}, r.resolveErr
	}
	r.resolveReq = req
	resolved := r.decision
	resolved.StatusSnapshot = req.StatusSnapshot
	resolved.ResolvedEventID = req.ResolvedEventID
	now := time.Now().UTC()
	resolved.ResolvedAt = &now
	return resolved, nil
}
