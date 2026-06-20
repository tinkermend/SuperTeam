package teamlending

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// fakeRepository 内存版仓储，用于覆盖借调核心裁决逻辑（不依赖真实数据库）。
type fakeRepository struct {
	mu           sync.Mutex
	policies     map[string]Policy
	requests     map[uuid.UUID]Request
	activeKey    map[string]bool // (tenant,project,team) 有效借调占用
	ownerUserIDs []uuid.UUID
}

func newFakeRepository(owners ...uuid.UUID) *fakeRepository {
	return &fakeRepository{
		policies:     map[string]Policy{},
		requests:     map[uuid.UUID]Request{},
		activeKey:    map[string]bool{},
		ownerUserIDs: owners,
	}
}

func teamPolicyKey(tenantID, teamID uuid.UUID) string {
	return fmt.Sprintf("%s|%s", tenantID, teamID)
}

func activeRequestKey(tenantID, projectID, teamID uuid.UUID) string {
	return fmt.Sprintf("%s|%s|%s", tenantID, projectID, teamID)
}

func (r *fakeRepository) UpsertPolicy(_ context.Context, params UpsertPolicyParams) (Policy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	policy := Policy{
		ID:                uuid.New(),
		TenantID:          params.TenantID,
		TeamID:            params.TeamID,
		AllowLending:      params.AllowLending,
		ApprovalMode:      params.ApprovalMode,
		BudgetCeiling:     params.BudgetCeiling,
		CapabilityCeiling: params.CapabilityCeiling,
		ProjectMatch:      params.ProjectMatch,
		Status:            "active",
		CreatedByUserID:   &params.ActorUserID,
		UpdatedByUserID:   &params.ActorUserID,
	}
	r.policies[teamPolicyKey(params.TenantID, params.TeamID)] = policy
	return policy, nil
}

func (r *fakeRepository) GetPolicy(_ context.Context, tenantID, teamID uuid.UUID) (Policy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	policy, ok := r.policies[teamPolicyKey(tenantID, teamID)]
	if !ok {
		return Policy{}, ErrNotFound
	}
	return policy, nil
}

func (r *fakeRepository) CreateRequest(_ context.Context, params CreateRequestParams) (Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := activeRequestKey(params.TenantID, params.ProjectID, params.TeamID)
	if r.activeKey[key] {
		return Request{}, fmt.Errorf("%w: duplicate", ErrDuplicateRequest)
	}
	request := Request{
		ID:                  uuid.New(),
		TenantID:            params.TenantID,
		TeamID:              params.TeamID,
		ProjectID:           params.ProjectID,
		Status:              params.Status,
		RequestedByUserID:   params.RequestedByUserID,
		RequestReason:       params.RequestReason,
		RequestedBudget:     params.RequestedBudget,
		RequestedCapability: params.RequestedCapability,
		GrantedBudget:       params.GrantedBudget,
		GrantedCapability:   params.GrantedCapability,
		IsException:         params.IsException,
	}
	r.requests[request.ID] = request
	if request.Status == RequestStatusPending || request.Status == RequestStatusAutoApproved || request.Status == RequestStatusApproved {
		r.activeKey[key] = true
	}
	return request, nil
}

func (r *fakeRepository) GetRequest(_ context.Context, tenantID, requestID uuid.UUID) (Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[requestID]
	if !ok || request.TenantID != tenantID {
		return Request{}, ErrNotFound
	}
	return request, nil
}

func (r *fakeRepository) ListRequestsByTeam(_ context.Context, params ListTeamRequestsParams) ([]Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Request{}
	for _, request := range r.requests {
		if request.TenantID == params.TenantID && request.TeamID == params.TeamID {
			if params.Status == "" || request.Status == params.Status {
				out = append(out, request)
			}
		}
	}
	return out, nil
}

func (r *fakeRepository) ListRequestsByProject(_ context.Context, params ListProjectRequestsParams) ([]Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Request{}
	for _, request := range r.requests {
		if request.TenantID == params.TenantID && request.ProjectID == params.ProjectID {
			if params.Status == "" || request.Status == params.Status {
				out = append(out, request)
			}
		}
	}
	return out, nil
}

func (r *fakeRepository) ApproveRequest(_ context.Context, params DecideRequestParams) (Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[params.RequestID]
	if !ok || request.TenantID != params.TenantID || request.TeamID != params.TeamID {
		return Request{}, ErrNotFound
	}
	if request.Status != RequestStatusPending {
		return Request{}, ErrInvalidTransition
	}
	request.Status = RequestStatusApproved
	request.DecidedByUserID = &params.DecidedByUserID
	request.DecisionReason = params.DecisionReason
	if params.GrantedBudget != "" {
		request.GrantedBudget = params.GrantedBudget
	} else {
		request.GrantedBudget = request.RequestedBudget
	}
	r.requests[request.ID] = request
	return request, nil
}

func (r *fakeRepository) RejectRequest(_ context.Context, params DecideRequestParams) (Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[params.RequestID]
	if !ok || request.TenantID != params.TenantID || request.TeamID != params.TeamID {
		return Request{}, ErrNotFound
	}
	if request.Status != RequestStatusPending {
		return Request{}, ErrInvalidTransition
	}
	request.Status = RequestStatusRejected
	request.DecidedByUserID = &params.DecidedByUserID
	request.DecisionReason = params.DecisionReason
	key := activeRequestKey(request.TenantID, request.ProjectID, request.TeamID)
	delete(r.activeKey, key)
	r.requests[request.ID] = request
	return request, nil
}

func (r *fakeRepository) RevokeRequest(_ context.Context, params DecideRequestParams) (Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[params.RequestID]
	if !ok || request.TenantID != params.TenantID || request.TeamID != params.TeamID {
		return Request{}, ErrNotFound
	}
	if request.Status != RequestStatusApproved && request.Status != RequestStatusAutoApproved {
		return Request{}, ErrInvalidTransition
	}
	request.Status = RequestStatusRevoked
	request.DecidedByUserID = &params.DecidedByUserID
	request.DecisionReason = params.DecisionReason
	key := activeRequestKey(request.TenantID, request.ProjectID, request.TeamID)
	delete(r.activeKey, key)
	r.requests[request.ID] = request
	return request, nil
}

func (r *fakeRepository) GetTeamOwnerUserIDs(_ context.Context, _, _ uuid.UUID) ([]uuid.UUID, error) {
	return r.ownerUserIDs, nil
}

func (r *fakeRepository) ListEffectiveLendingTeams(_ context.Context, tenantID, projectID uuid.UUID) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[uuid.UUID]bool{}
	teams := make([]uuid.UUID, 0)
	for _, request := range r.requests {
		if request.TenantID != tenantID || request.ProjectID != projectID {
			continue
		}
		if request.Status != RequestStatusApproved && request.Status != RequestStatusAutoApproved {
			continue
		}
		if seen[request.TeamID] {
			continue
		}
		seen[request.TeamID] = true
		teams = append(teams, request.TeamID)
	}
	return teams, nil
}

func TestEffectiveLendingTeams(t *testing.T) {
	tenantID, projectID := uuid.New(), uuid.New()
	approvedTeam, autoTeam, rejectedTeam, otherProjectTeam := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepository()
	seed := func(teamID, project uuid.UUID, status RequestStatus) {
		id := uuid.New()
		repo.requests[id] = Request{ID: id, TenantID: tenantID, TeamID: teamID, ProjectID: project, Status: status}
	}
	seed(approvedTeam, projectID, RequestStatusApproved)
	seed(autoTeam, projectID, RequestStatusAutoApproved)
	seed(rejectedTeam, projectID, RequestStatusRejected)
	seed(otherProjectTeam, uuid.New(), RequestStatusApproved)

	service := mustService(t, repo)
	granted, err := service.EffectiveLendingTeams(context.Background(), tenantID, projectID)
	if err != nil {
		t.Fatalf("effective lending teams: %v", err)
	}
	if !granted[approvedTeam] || !granted[autoTeam] {
		t.Fatalf("approved and auto_approved teams must be granted: %#v", granted)
	}
	if granted[rejectedTeam] || granted[otherProjectTeam] {
		t.Fatalf("rejected or other-project grants must not appear: %#v", granted)
	}
	if len(granted) != 2 {
		t.Fatalf("expected exactly two granted teams, got %d", len(granted))
	}
}

func mustService(t *testing.T, repo Repository) *Service {
	t.Helper()
	service, err := NewService(repo, nil, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

func TestCreateRequest_AutoApprovesWithinCeiling(t *testing.T) {
	tenantID, teamID, projectID, requester, actor := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepository(actor)
	service := mustService(t, repo)
	if _, err := service.UpsertPolicy(context.Background(), UpsertPolicyInput{
		TenantID: tenantID, TeamID: teamID, ActorUserID: actor,
		AllowLending: true, ApprovalMode: ApprovalModeAuto, BudgetCeiling: "1000",
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID: tenantID, TeamID: teamID, ProjectID: projectID, RequestedByUserID: requester,
		RequestedBudget: "500",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if request.Status != RequestStatusAutoApproved {
		t.Fatalf("expected auto_approved, got %s", request.Status)
	}
	if request.IsException {
		t.Fatalf("auto-approved within ceiling must not be exception")
	}
	if request.GrantedBudget != "500" {
		t.Fatalf("expected granted budget 500, got %q", request.GrantedBudget)
	}
}

func TestCreateRequest_AutoExceedingBudgetForcesHumanException(t *testing.T) {
	tenantID, teamID, projectID, requester, actor := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepository(actor)
	service := mustService(t, repo)
	if _, err := service.UpsertPolicy(context.Background(), UpsertPolicyInput{
		TenantID: tenantID, TeamID: teamID, ActorUserID: actor,
		AllowLending: true, ApprovalMode: ApprovalModeAuto, BudgetCeiling: "1000",
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID: tenantID, TeamID: teamID, ProjectID: projectID, RequestedByUserID: requester,
		RequestedBudget: "1500",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if request.Status != RequestStatusPending {
		t.Fatalf("expected pending (over ceiling), got %s", request.Status)
	}
	if !request.IsException {
		t.Fatalf("over-ceiling auto request must be flagged as exception")
	}
}

func TestCreateRequest_ManualModeAlwaysPending(t *testing.T) {
	tenantID, teamID, projectID, requester, actor := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepository(actor)
	service := mustService(t, repo)
	if _, err := service.UpsertPolicy(context.Background(), UpsertPolicyInput{
		TenantID: tenantID, TeamID: teamID, ActorUserID: actor,
		AllowLending: true, ApprovalMode: ApprovalModeManual, BudgetCeiling: "1000",
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID: tenantID, TeamID: teamID, ProjectID: projectID, RequestedByUserID: requester,
		RequestedBudget: "100",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if request.Status != RequestStatusPending {
		t.Fatalf("manual mode must produce pending, got %s", request.Status)
	}
	if request.IsException {
		t.Fatalf("manual mode pending is not an exception")
	}
}

func TestCreateRequest_NoPolicyIsPending(t *testing.T) {
	tenantID, teamID, projectID, requester := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepository()
	service := mustService(t, repo)
	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID: tenantID, TeamID: teamID, ProjectID: projectID, RequestedByUserID: requester,
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if request.Status != RequestStatusPending {
		t.Fatalf("missing policy must produce pending, got %s", request.Status)
	}
}

func TestApproveThenRevokeTransition(t *testing.T) {
	tenantID, teamID, projectID, requester, owner := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepository(owner)
	service := mustService(t, repo)
	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID: tenantID, TeamID: teamID, ProjectID: projectID, RequestedByUserID: requester,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	approved, err := service.ApproveRequest(context.Background(), DecideRequestInput{
		TenantID: tenantID, TeamID: teamID, RequestID: request.ID, DecidedByUserID: owner,
	})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != RequestStatusApproved {
		t.Fatalf("expected approved, got %s", approved.Status)
	}
	// 再次 approve 已 approved 的请求应拒绝（状态机不允许）。
	if _, err := service.ApproveRequest(context.Background(), DecideRequestInput{
		TenantID: tenantID, TeamID: teamID, RequestID: request.ID, DecidedByUserID: owner,
	}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition re-approving, got %v", err)
	}
	// approved 之后可撤销。
	revoked, err := service.RevokeRequest(context.Background(), DecideRequestInput{
		TenantID: tenantID, TeamID: teamID, RequestID: request.ID, DecidedByUserID: owner,
	})
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.Status != RequestStatusRevoked {
		t.Fatalf("expected revoked, got %s", revoked.Status)
	}
}

func TestDuplicateActiveRequestRejected(t *testing.T) {
	tenantID, teamID, projectID, requester := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepository()
	service := mustService(t, repo)
	if _, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID: tenantID, TeamID: teamID, ProjectID: projectID, RequestedByUserID: requester,
	}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID: tenantID, TeamID: teamID, ProjectID: projectID, RequestedByUserID: requester,
	})
	if !errors.Is(err, ErrDuplicateRequest) {
		t.Fatalf("expected ErrDuplicateRequest, got %v", err)
	}
}

func TestCapabilityCeilingKeyPresence(t *testing.T) {
	policy := Policy{CapabilityCeiling: map[string]any{"code": true, "search": true}}
	input := CreateRequestInput{RequestedCapability: map[string]any{"code": true}}
	if requestExceedsCeiling(input, policy) {
		t.Fatalf("requested key present in ceiling must not exceed")
	}
	input.RequestedCapability = map[string]any{"deploy": true}
	if !requestExceedsCeiling(input, policy) {
		t.Fatalf("requested key absent from ceiling must exceed")
	}
}
