package permission

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/approval"
)

// fakeApprovalRepo is a minimal in-memory approval.Repository for permission tests.
type fakeApprovalRepo struct {
	mu       sync.Mutex
	requests map[uuid.UUID]approval.ApprovalRequest
}

func newFakeApprovalRepo() *fakeApprovalRepo {
	return &fakeApprovalRepo{requests: map[uuid.UUID]approval.ApprovalRequest{}}
}

func (r *fakeApprovalRepo) CreateApprovalRequest(_ context.Context, in approval.CreateRequestInput, status approval.ApprovalStatus) (approval.ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cat := in.Category
	if cat == "" {
		cat = approval.ApprovalCategoryProjectTask
	}
	now := time.Now().UTC()
	req := approval.ApprovalRequest{
		ID: uuid.New(), TenantID: in.TenantID, ResourceType: in.ResourceType, ResourceID: in.ResourceID,
		RequesterType: in.RequesterType, RequesterID: in.RequesterID, TargetUserID: in.TargetUserID,
		DecisionType: in.DecisionType, Title: in.Title, Status: status, Category: cat,
		ContextPayload: in.ContextPayload, CreatedAt: now, UpdatedAt: now,
	}
	r.requests[req.ID] = req
	return req, nil
}

func (r *fakeApprovalRepo) GetApprovalRequest(_ context.Context, tenantID, id uuid.UUID) (approval.ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.requests[id]
	if !ok || req.TenantID != tenantID {
		return approval.ApprovalRequest{}, approval.ErrApprovalNotFound
	}
	return req, nil
}

func (r *fakeApprovalRepo) GetApprovalRequestByResource(context.Context, uuid.UUID, string, uuid.UUID) (approval.ApprovalRequest, error) {
	return approval.ApprovalRequest{}, approval.ErrApprovalNotFound
}

func (r *fakeApprovalRepo) ResolveApprovalRequest(_ context.Context, in approval.ResolveRequestInput, status approval.ApprovalStatus) (approval.ApprovalRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.requests[in.ApprovalRequestID]
	if !ok || req.TenantID != in.TenantID {
		return approval.ApprovalRequest{}, approval.ErrApprovalNotFound
	}
	if req.Status != approval.ApprovalStatusPending {
		return approval.ApprovalRequest{}, approval.ErrApprovalAlreadyResolved
	}
	now := time.Now().UTC()
	req.Status = status
	req.UpdatedAt = now
	req.ResolvedAt = &now
	r.requests[req.ID] = req
	return req, nil
}

func (r *fakeApprovalRepo) CreateApprovalDecision(_ context.Context, in approval.ResolveRequestInput) (approval.ApprovalDecisionRecord, error) {
	return approval.ApprovalDecisionRecord{ID: uuid.New(), TenantID: in.TenantID, ApprovalRequestID: in.ApprovalRequestID, DecidedByUserID: in.DecidedByUserID, Decision: in.Decision, CreatedAt: time.Now().UTC()}, nil
}

func (r *fakeApprovalRepo) ListPermissionApprovals(context.Context, approval.ListPermissionApprovalsInput) ([]approval.ApprovalRequest, error) {
	return nil, nil
}

func (r *fakeApprovalRepo) PermissionApprovalSummary(context.Context, approval.PermissionApprovalSummaryInput) (approval.PermissionApprovalSummary, error) {
	return approval.PermissionApprovalSummary{}, nil
}

// fakeSubject records Apply calls and can be told to fail.
type fakeSubject struct {
	resourceType string
	applied      int
	failWith     error
}

func (f *fakeSubject) ResourceType() string { return f.resourceType }
func (f *fakeSubject) Actions() []Action    { return DefaultActions() }
func (f *fakeSubject) Apply(_ context.Context, _ ApplyInput) error {
	f.applied++
	return f.failWith
}

func newPermissionService(t *testing.T, subj Subject) (*Service, *approval.Service) {
	t.Helper()
	repo := newFakeApprovalRepo()
	appr, err := approval.NewService(repo)
	if err != nil {
		t.Fatalf("approval.NewService: %v", err)
	}
	reg := NewRegistry()
	if subj != nil {
		reg.Register(subj)
	}
	svc, err := NewService(appr, reg, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, appr
}

func createPermissionRequest(t *testing.T, appr *approval.Service, tenantID, target uuid.UUID, resourceType string) approval.ApprovalRequest {
	t.Helper()
	req, err := appr.CreateRequest(context.Background(), approval.CreateRequestInput{
		TenantID:      tenantID,
		ResourceType:  resourceType,
		ResourceID:    uuid.New(),
		RequesterType: "human_user",
		TargetUserID:  target,
		DecisionType:  "permission_grant",
		Title:         "grant",
		Category:      approval.ApprovalCategoryPermission,
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	return *req
}

func TestDecideApprovedTriggersApplyThenResolves(t *testing.T) {
	tenantID, target, decider := uuid.New(), uuid.New(), uuid.New()
	subj := &fakeSubject{resourceType: "team_privileged_role_request"}
	svc, appr := newPermissionService(t, subj)
	req := createPermissionRequest(t, appr, tenantID, target, subj.resourceType)

	view, err := svc.Decide(context.Background(), DecideInput{
		TenantID:   tenantID,
		ApprovalID: req.ID,
		DecidedBy:  decider,
		Decision:   string(approval.ApprovalDecisionApproved),
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if subj.applied != 1 {
		t.Fatalf("expected Apply called once, got %d", subj.applied)
	}
	if view.Request.Status != approval.ApprovalStatusApproved {
		t.Fatalf("expected approved, got %s", view.Request.Status)
	}
}

func TestDecideApplyFailureLeavesRequestPending(t *testing.T) {
	tenantID, target, decider := uuid.New(), uuid.New(), uuid.New()
	subj := &fakeSubject{resourceType: "team_privileged_role_request", failWith: errors.New("grant failed")}
	svc, appr := newPermissionService(t, subj)
	req := createPermissionRequest(t, appr, tenantID, target, subj.resourceType)

	if _, err := svc.Decide(context.Background(), DecideInput{
		TenantID:   tenantID,
		ApprovalID: req.ID,
		DecidedBy:  decider,
		Decision:   string(approval.ApprovalDecisionApproved),
	}); err == nil {
		t.Fatal("expected Decide to fail when Apply fails")
	}
	got, err := appr.GetRequest(context.Background(), tenantID, req.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got.Status != approval.ApprovalStatusPending {
		t.Fatalf("expected request still pending after Apply failure, got %s", got.Status)
	}
}

func TestDecideRejectedSkipsApply(t *testing.T) {
	tenantID, target, decider := uuid.New(), uuid.New(), uuid.New()
	subj := &fakeSubject{resourceType: "team_privileged_role_request"}
	svc, appr := newPermissionService(t, subj)
	req := createPermissionRequest(t, appr, tenantID, target, subj.resourceType)

	if _, err := svc.Decide(context.Background(), DecideInput{
		TenantID:   tenantID,
		ApprovalID: req.ID,
		DecidedBy:  decider,
		Decision:   string(approval.ApprovalDecisionRejected),
	}); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if subj.applied != 0 {
		t.Fatalf("expected Apply not called on reject, got %d", subj.applied)
	}
}

func TestDecideRejectsNonPermissionCategory(t *testing.T) {
	tenantID, target, decider := uuid.New(), uuid.New(), uuid.New()
	svc, appr := newPermissionService(t, &fakeSubject{resourceType: "x"})
	req, err := appr.CreateRequest(context.Background(), approval.CreateRequestInput{
		TenantID:      tenantID,
		ResourceType:  "project_task_dispatch_gate",
		ResourceID:    uuid.New(),
		RequesterType: "system",
		TargetUserID:  target,
		DecisionType:  "gate",
		Title:         "gate",
		Category:      approval.ApprovalCategoryProjectTask,
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if _, err := svc.Decide(context.Background(), DecideInput{
		TenantID:   tenantID,
		ApprovalID: req.ID,
		DecidedBy:  decider,
		Decision:   string(approval.ApprovalDecisionApproved),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-permission category, got %v", err)
	}
}
