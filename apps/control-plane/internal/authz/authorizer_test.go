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
	taskID      uuid.UUID
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
	r.taskID = params.TaskID
	return r.runtimeOK, nil
}

type memoryRecorder struct {
	records []DecisionRecord
}

func (r *memoryRecorder) RecordDecision(ctx context.Context, record DecisionRecord) error {
	r.records = append(r.records, record)
	return nil
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

func TestDBAuthorizerRecordsDeniedDecision(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000004")
	recorder := &memoryRecorder{}
	authorizer := NewDBAuthorizer(&memoryRepository{tenantRoles: map[string]string{}}, recorder)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected denial, got %#v", decision)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("expected one decision record, got %#v", recorder.records)
	}
	record := recorder.records[0]
	if record.ActorType != ActorUser || record.Action != ActionConsoleAccess || record.Allowed {
		t.Fatalf("unexpected decision record: %#v", record)
	}
}

func TestDBAuthorizerDeniesConsoleAccessWithNonConsoleResource(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000004")
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceTask, ID: "task-1"},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected console access to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonInvalidResource {
		t.Fatalf("expected invalid resource reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerDeniesTenantAccessWithMismatchedTenantResourceID(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	otherTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000007")
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionTenantAccess,
		Resource: ResourceRef{Type: ResourceTenant, ID: otherTenantID.String()},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected tenant access to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonInvalidResource {
		t.Fatalf("expected invalid resource reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerDeniesTeamAccessWithMismatchedTeamResourceID(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	otherTeamID := uuid.MustParse("00000000-0000-0000-0000-000000000102")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000008")
	repo := &memoryRepository{
		teamRoles: map[string]string{
			tenantID.String() + ":" + teamID.String() + ":user:" + userID.String(): RoleMember,
		},
	}
	authorizer := NewDBAuthorizer(repo)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionTeamAccess,
		Resource: ResourceRef{Type: ResourceTeam, ID: otherTeamID.String()},
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected team access to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonInvalidResource {
		t.Fatalf("expected invalid resource reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerDeniesTeamAccessWithNilTeamID(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000009")
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionTeamAccess,
		Resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected team access to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonInvalidResource {
		t.Fatalf("expected invalid resource reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerAllowsRuntimeClaimWhenScopeCoversTask(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	taskID := uuid.MustParse("00000000-0000-0000-0000-000000001001")
	repo := &memoryRepository{runtimeOK: true}
	authorizer := NewDBAuthorizer(repo)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorRuntimeNode, ID: "node-1"},
		Action:   ActionTaskClaim,
		Resource: ResourceRef{Type: ResourceTask, ID: taskID.String()},
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
	if repo.taskID != taskID {
		t.Fatalf("expected task ID %s to reach repository, got %s", taskID, repo.taskID)
	}
}

func TestDBAuthorizerDeniesRuntimeClaimWithNonTaskResource(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	authorizer := NewDBAuthorizer(&memoryRepository{runtimeOK: true})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorRuntimeNode, ID: "node-1"},
		Action:   ActionTaskClaim,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected runtime claim to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonInvalidResource {
		t.Fatalf("expected invalid resource reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerDeniesRuntimeClaimWhenScopeDoesNotCoverTask(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	taskID := uuid.MustParse("00000000-0000-0000-0000-000000001002")
	authorizer := NewDBAuthorizer(&memoryRepository{runtimeOK: false})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorRuntimeNode, ID: "node-1"},
		Action:   ActionTaskClaim,
		Resource: ResourceRef{Type: ResourceTask, ID: taskID.String()},
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

func TestDBAuthorizerDeniesRuntimeClaimWithInvalidTaskResourceID(t *testing.T) {
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
	if decision.Allowed {
		t.Fatalf("expected runtime claim to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonInvalidResource {
		t.Fatalf("expected invalid resource reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerDeniesInvalidUserActorID(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	authorizer := NewDBAuthorizer(&memoryRepository{tenantRoles: map[string]string{}})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: "not-a-uuid"},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected invalid actor to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonInvalidActor {
		t.Fatalf("expected invalid actor reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerDeniesNilRepository(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000005")
	authorizer := NewDBAuthorizer(nil)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected nil repository to be denied, got %#v", decision)
	}
	if !decision.RequiresAudit {
		t.Fatalf("expected nil repository denial to require audit, got %#v", decision)
	}
}

func TestDBAuthorizerUnsupportedActionReturnsErrorAndDenyDecision(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000006")
	authorizer := NewDBAuthorizer(&memoryRepository{})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   "task.delete",
		Resource: ResourceRef{Type: ResourceTask, ID: "task-1"},
		TenantID: tenantID,
	})

	if !errors.Is(err, ErrUnsupportedAction) {
		t.Fatalf("expected unsupported action error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected unsupported action to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonUnsupportedAction {
		t.Fatalf("expected unsupported action reason, got %q", decision.Reason)
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
