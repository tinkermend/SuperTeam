package authzcenter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
)

type serviceRepo struct {
	totalsSince time.Time
	topSince    time.Time
	topLimit    int32
	listFilter  DecisionFilter

	totals    DecisionTotals
	top       []ActionCount
	decisions []DecisionRecord

	createdScopes []RuntimeScopeInput
	scope         RuntimeScopeRecord
	operationLogs []OperationLogInput
}

func (r *serviceRepo) CountDecisionsSince(ctx context.Context, since time.Time) (DecisionTotals, error) {
	r.totalsSince = since
	return r.totals, nil
}

func (r *serviceRepo) ListTopDeniedActionsSince(ctx context.Context, since time.Time, limit int32) ([]ActionCount, error) {
	r.topSince = since
	r.topLimit = limit
	return r.top, nil
}

func (r *serviceRepo) ListDecisions(ctx context.Context, filter DecisionFilter) ([]DecisionRecord, error) {
	r.listFilter = filter
	return r.decisions, nil
}

func (r *serviceRepo) ListRuntimeScopeNodes(ctx context.Context) ([]RuntimeScopeNodeRecord, error) {
	return nil, nil
}

func (r *serviceRepo) CreateRuntimeScope(ctx context.Context, input RuntimeScopeInput) (RuntimeScopeRecord, error) {
	r.createdScopes = append(r.createdScopes, input)
	if r.scope.ID == uuid.Nil {
		r.scope = RuntimeScopeRecord{
			ID:            uuid.New(),
			TenantID:      input.TenantID,
			RuntimeNodeID: input.RuntimeNodeID,
			TeamID:        input.TeamID,
			ScopeType:     RuntimeScopeScopeType(input.ScopeType),
			ScopeValue:    input.ScopeValue,
			Status:        RuntimeScopeStatusActive,
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		}
	}
	return r.scope, nil
}

func (r *serviceRepo) UpdateRuntimeScopeStatus(ctx context.Context, scopeID uuid.UUID, status string) (RuntimeScopeRecord, error) {
	return RuntimeScopeRecord{ID: scopeID, Status: RuntimeScopeStatus(status)}, nil
}

func (r *serviceRepo) ListMembers(ctx context.Context, filter MemberFilter) ([]MemberRecord, error) {
	return nil, nil
}

func (r *serviceRepo) RecordOperationLog(ctx context.Context, input OperationLogInput) error {
	r.operationLogs = append(r.operationLogs, input)
	return nil
}

type serviceAuthorizer struct {
	decision authz.Decision
	err      error
	checks   []authz.CheckRequest
}

func (a *serviceAuthorizer) Check(ctx context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	if a.err != nil {
		return authz.Decision{}, a.err
	}
	return a.decision, nil
}

func TestServiceOverviewUsesDecisionDataAndDBEngine(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	recentID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	repo := &serviceRepo{
		totals: DecisionTotals{Total: 10, Allowed: 7, Denied: 3},
		top:    []ActionCount{{Action: authz.ActionTaskClaim, Count: 3}},
		decisions: []DecisionRecord{{
			ID:       recentID,
			TenantID: tenantID,
			Action:   authz.ActionTaskClaim,
			Result:   OperationResultFailed,
		}},
	}
	service := NewService(repo, &serviceAuthorizer{})

	overview, err := service.GetOverview(context.Background())

	if err != nil {
		t.Fatalf("expected overview, got error: %v", err)
	}
	if overview.Engine.Engine != "db" || overview.Engine.Status != "healthy" || overview.Engine.EngineVersion != "db-authorizer-v1" {
		t.Fatalf("expected db engine status, got %#v", overview.Engine)
	}
	if overview.Totals.Total != 10 || overview.Totals.Allowed != 7 || overview.Totals.Denied != 3 {
		t.Fatalf("unexpected totals: %#v", overview.Totals)
	}
	if got := overview.Totals.DeniedRate(); got != 0.3 {
		t.Fatalf("expected denied rate 0.3, got %v", got)
	}
	if repo.topLimit != 5 {
		t.Fatalf("expected top denied limit 5, got %d", repo.topLimit)
	}
	if repo.listFilter.Limit != 10 || repo.listFilter.Offset != 0 {
		t.Fatalf("expected recent events limit/offset 10/0, got %#v", repo.listFilter)
	}
	if time.Since(repo.totalsSince) > 25*time.Hour || time.Since(repo.topSince) > 25*time.Hour {
		t.Fatalf("expected overview to query roughly the last 24h")
	}
	if len(overview.TopDeniedActions) != 1 || len(overview.RecentEvents) != 1 {
		t.Fatalf("expected top denied and recent events to be populated, got %#v", overview)
	}
}

func TestServiceCreateRuntimeScopeRequiresAuthorizationAndWritesOperationLog(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	nodeID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	repo := &serviceRepo{}
	authorizer := &serviceAuthorizer{
		decision: authz.Decision{Allowed: true, Reason: authz.ReasonAllowed, MatchedRule: "tenant.admin"},
	}
	service := NewService(repo, authorizer)

	scope, err := service.CreateRuntimeScope(context.Background(), Actor{
		UserID:   userID,
		Username: "admin",
		TenantID: tenantID,
	}, RuntimeScopeInput{
		TenantID:      tenantID,
		RuntimeNodeID: nodeID,
		ScopeType:     "tenant",
		ScopeValue:    tenantID.String(),
	})

	if err != nil {
		t.Fatalf("expected scope to be created, got error: %v", err)
	}
	if scope.ID == uuid.Nil {
		t.Fatalf("expected created scope ID, got %#v", scope)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authorization check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Action != ActionRuntimeScopeManage {
		t.Fatalf("expected runtime scope manage action, got %q", check.Action)
	}
	if check.Resource.Type != authz.ResourceTenant || check.Resource.ID != tenantID.String() || check.TenantID != tenantID {
		t.Fatalf("expected tenant resource check, got %#v", check)
	}
	if len(repo.createdScopes) != 1 {
		t.Fatalf("expected repository create, got %#v", repo.createdScopes)
	}
	if len(repo.operationLogs) != 1 {
		t.Fatalf("expected one operation log, got %#v", repo.operationLogs)
	}
	log := repo.operationLogs[0]
	if log.Module != OperationModuleAuthz || log.Action != OperationActionRuntimeScopeCreate || log.Result != OperationResultSucceeded {
		t.Fatalf("unexpected operation log: %#v", log)
	}
	if log.Actor.UserID != userID || log.Actor.Username != "admin" {
		t.Fatalf("expected actor in operation log, got %#v", log.Actor)
	}
}

func TestServiceCreateRuntimeScopeDeniedWritesFailedOperationLog(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	nodeID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	repo := &serviceRepo{}
	authorizer := &serviceAuthorizer{
		decision: authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership},
	}
	service := NewService(repo, authorizer)

	_, err := service.CreateRuntimeScope(context.Background(), Actor{
		UserID:   userID,
		Username: "viewer",
		TenantID: tenantID,
	}, RuntimeScopeInput{
		TenantID:      tenantID,
		RuntimeNodeID: nodeID,
		ScopeType:     "tenant",
		ScopeValue:    tenantID.String(),
	})

	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if len(repo.createdScopes) != 0 {
		t.Fatalf("expected denied create not to call repository, got %#v", repo.createdScopes)
	}
	if len(repo.operationLogs) != 1 || repo.operationLogs[0].Result != OperationResultFailed {
		t.Fatalf("expected one failed operation log, got %#v", repo.operationLogs)
	}
}

func TestServiceCheckPermissionUsesDryRunAuditReason(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	authorizer := &serviceAuthorizer{
		decision: authz.Decision{Allowed: true, Reason: authz.ReasonAllowed, MatchedRule: "tenant.owner"},
	}
	service := NewService(&serviceRepo{}, authorizer)

	decision, err := service.CheckPermission(context.Background(), CheckPermissionInput{
		Actor:    authz.ActorRef{Type: authz.ActorUser, ID: uuid.NewString()},
		Action:   authz.ActionTenantAccess,
		Resource: authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected permission check, got error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allowed decision, got %#v", decision)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authorization check, got %#v", authorizer.checks)
	}
	if authorizer.checks[0].AuditReason != "authz center dry-run" {
		t.Fatalf("expected dry-run audit reason, got %q", authorizer.checks[0].AuditReason)
	}
}
