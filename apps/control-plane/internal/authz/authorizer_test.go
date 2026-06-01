package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type memoryRepository struct {
	tenantRoles map[string]string
	teamRoles   map[string]string
	runtimeOK   bool
	err         error
}

func (r *memoryRepository) GetActiveTenantMembership(ctx context.Context, params TenantMembershipParams) (Membership, error) {
	if r.err != nil {
		return Membership{}, r.err
	}
	role, ok := r.tenantRoles[params.TenantID.String()+":"+params.PrincipalType+":"+params.PrincipalID.String()]
	if !ok {
		return Membership{}, ErrNoMembership
	}
	return Membership{TenantID: params.TenantID, PrincipalType: params.PrincipalType, PrincipalID: params.PrincipalID, Role: role, Status: "active"}, nil
}

func (r *memoryRepository) GetActiveTeamMembership(ctx context.Context, params TeamMembershipParams) (Membership, error) {
	if r.err != nil {
		return Membership{}, r.err
	}
	role, ok := r.teamRoles[params.TenantID.String()+":"+params.TeamID.String()+":"+params.PrincipalType+":"+params.PrincipalID.String()]
	if !ok {
		return Membership{}, ErrNoMembership
	}
	return Membership{TenantID: params.TenantID, TeamID: &params.TeamID, PrincipalType: params.PrincipalType, PrincipalID: params.PrincipalID, Role: role, Status: "active"}, nil
}

func (r *memoryRepository) RuntimeNodeCoversTaskScope(ctx context.Context, params RuntimeScopeParams) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	return r.runtimeOK, nil
}

func TestDBAuthorizerAllowsTenantOwnerConsoleAccess(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected console access to be allowed, got %#v", decision)
	}
	if decision.MatchedRule != "tenant.owner" {
		t.Fatalf("expected tenant.owner rule, got %q", decision.MatchedRule)
	}
}

func TestDBAuthorizerDeniesConsoleAccessWithoutMembership(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	authorizer := NewDBAuthorizer(&memoryRepository{tenantRoles: map[string]string{}})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no repository error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected console access to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonNoMembership {
		t.Fatalf("expected no membership reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerAllowsRuntimeClaimWhenScopeCoversTask(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	authorizer := NewDBAuthorizer(&memoryRepository{runtimeOK: true})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorRuntimeNode, ID: "node-1"},
		Action:   ActionTaskClaim,
		Resource: ResourceRef{Type: ResourceTask, ID: "task-1"},
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected runtime claim to be allowed, got %#v", decision)
	}
	if decision.MatchedRule != "runtime.scope" {
		t.Fatalf("expected runtime.scope rule, got %q", decision.MatchedRule)
	}
}

func TestDBAuthorizerDeniesRuntimeClaimWhenScopeDoesNotCoverTask(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	authorizer := NewDBAuthorizer(&memoryRepository{runtimeOK: false})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorRuntimeNode, ID: "node-1"},
		Action:   ActionTaskClaim,
		Resource: ResourceRef{Type: ResourceTask, ID: "task-1"},
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected runtime claim to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonRuntimeScopeMissing {
		t.Fatalf("expected missing runtime scope reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerReturnsRepositoryErrors(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000003")
	repoErr := errors.New("db unavailable")
	authorizer := NewDBAuthorizer(&memoryRepository{err: repoErr})

	_, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if !errors.Is(err, repoErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
}
