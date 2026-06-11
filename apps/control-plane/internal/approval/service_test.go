package approval

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewServiceRequiresRepository(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("expected nil repository to fail")
	}
}

func TestApprovalServiceCreatesAndResolvesRequest(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	targetUserID := uuid.New()
	resourceID := uuid.New()

	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID:       tenantID,
		ResourceType:   "project_decision",
		ResourceID:     resourceID,
		RequesterType:  "project_coordinator",
		TargetUserID:   targetUserID,
		DecisionType:   "route_review",
		Title:          "确认高风险路由",
		Summary:        "需要负责人确认是否继续",
		RiskLevel:      "high",
		Options:        []any{"approved", "rejected", "needs_more_evidence"},
		ContextPayload: map[string]any{"project_id": resourceID.String()},
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if request.Status != ApprovalStatusPending {
		t.Fatalf("expected pending request, got %s", request.Status)
	}
	if request.Summary == nil || *request.Summary != "需要负责人确认是否继续" {
		t.Fatalf("expected summary snapshot, got %#v", request.Summary)
	}

	decision, err := service.ResolveRequest(context.Background(), ResolveRequestInput{
		TenantID:          tenantID,
		ApprovalRequestID: request.ID,
		DecidedByUserID:   targetUserID,
		Decision:          ApprovalDecisionApproved,
		Comment:           "同意继续",
		Payload:           map[string]any{"accepted": true},
	})
	if err != nil {
		t.Fatalf("resolve request: %v", err)
	}
	if decision.Decision != ApprovalDecisionApproved {
		t.Fatalf("expected approved decision, got %s", decision.Decision)
	}
	resolved, err := service.GetRequest(context.Background(), tenantID, request.ID)
	if err != nil {
		t.Fatalf("get resolved request: %v", err)
	}
	if resolved.Status != ApprovalStatusApproved {
		t.Fatalf("expected approved request status, got %s", resolved.Status)
	}
	if resolved.ResolvedAt == nil {
		t.Fatal("expected resolved timestamp")
	}
}

func TestApprovalServiceRejectsInvalidAndDuplicateResolution(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID: uuid.New(),
		Title:    "缺少字段",
	})
	if !errors.Is(err, ErrInvalidApprovalRequest) {
		t.Fatalf("expected invalid request, got %v", err)
	}

	tenantID := uuid.New()
	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID:      tenantID,
		ResourceType:  "project_decision",
		ResourceID:    uuid.New(),
		RequesterType: "project_coordinator",
		TargetUserID:  uuid.New(),
		DecisionType:  "route_review",
		Title:         "确认",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	input := ResolveRequestInput{
		TenantID:          tenantID,
		ApprovalRequestID: request.ID,
		DecidedByUserID:   request.TargetUserID,
		Decision:          ApprovalDecisionRejected,
	}
	if _, err := service.ResolveRequest(context.Background(), input); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if _, err := service.ResolveRequest(context.Background(), input); !errors.Is(err, ErrApprovalAlreadyResolved) {
		t.Fatalf("expected duplicate resolve error, got %v", err)
	}
}

func TestApprovalServiceIsTenantScoped(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID:      uuid.New(),
		ResourceType:  "project_decision",
		ResourceID:    uuid.New(),
		RequesterType: "project_coordinator",
		TargetUserID:  uuid.New(),
		DecisionType:  "route_review",
		Title:         "确认",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	if _, err := service.GetRequest(context.Background(), uuid.New(), request.ID); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("expected tenant scoped not found, got %v", err)
	}
	if _, err := service.ResolveRequest(context.Background(), ResolveRequestInput{
		TenantID:          uuid.New(),
		ApprovalRequestID: request.ID,
		DecidedByUserID:   request.TargetUserID,
		Decision:          ApprovalDecisionApproved,
	}); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("expected tenant scoped resolve not found, got %v", err)
	}
}

type memoryRepository struct {
	mu        sync.Mutex
	requests  map[uuid.UUID]ApprovalRequest
	decisions []ApprovalDecisionRecord
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		requests: make(map[uuid.UUID]ApprovalRequest),
	}
}

func (r *memoryRepository) CreateApprovalRequest(_ context.Context, input CreateRequestInput, status ApprovalStatus) (ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	request := ApprovalRequest{
		ID:             uuid.New(),
		TenantID:       input.TenantID,
		ResourceType:   input.ResourceType,
		ResourceID:     input.ResourceID,
		RequesterType:  input.RequesterType,
		RequesterID:    input.RequesterID,
		TargetUserID:   input.TargetUserID,
		DecisionType:   input.DecisionType,
		Title:          input.Title,
		Summary:        optionalString(input.Summary),
		RiskLevel:      optionalString(input.RiskLevel),
		Status:         status,
		Options:        input.Options,
		ContextPayload: input.ContextPayload,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	r.requests[request.ID] = request
	return request, nil
}

func (r *memoryRepository) GetApprovalRequest(_ context.Context, tenantID, requestID uuid.UUID) (ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	request, ok := r.requests[requestID]
	if !ok || request.TenantID != tenantID {
		return ApprovalRequest{}, ErrApprovalNotFound
	}
	return request, nil
}

func (r *memoryRepository) ResolveApprovalRequest(_ context.Context, input ResolveRequestInput, status ApprovalStatus) (ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	request, ok := r.requests[input.ApprovalRequestID]
	if !ok || request.TenantID != input.TenantID {
		return ApprovalRequest{}, ErrApprovalNotFound
	}
	if request.Status != ApprovalStatusPending {
		return ApprovalRequest{}, ErrApprovalAlreadyResolved
	}
	now := time.Now().UTC()
	request.Status = status
	request.UpdatedAt = now
	request.ResolvedAt = &now
	r.requests[request.ID] = request
	return request, nil
}

func (r *memoryRepository) CreateApprovalDecision(_ context.Context, input ResolveRequestInput) (ApprovalDecisionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	decision := ApprovalDecisionRecord{
		ID:                uuid.New(),
		TenantID:          input.TenantID,
		ApprovalRequestID: input.ApprovalRequestID,
		DecidedByUserID:   input.DecidedByUserID,
		Decision:          input.Decision,
		Comment:           optionalString(input.Comment),
		Payload:           input.Payload,
		CreatedAt:         time.Now().UTC(),
	}
	r.decisions = append(r.decisions, decision)
	return decision, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
