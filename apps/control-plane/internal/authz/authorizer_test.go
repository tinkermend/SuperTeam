package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type memoryRepository struct {
	tenantRoles    map[string]string
	teamRoles      map[string]string
	employeeScopes map[uuid.UUID]DigitalEmployeeAuthzScope
	runtimeOK      bool
	taskID         uuid.UUID
	err            error
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

func (r *memoryRepository) GetDigitalEmployeeAuthzScope(ctx context.Context, params DigitalEmployeeAuthzScopeParams) (DigitalEmployeeAuthzScope, error) {
	if r.err != nil {
		return DigitalEmployeeAuthzScope{}, r.err
	}
	scope, ok := r.employeeScopes[params.EmployeeID]
	if !ok || scope.TenantID != params.TenantID {
		return DigitalEmployeeAuthzScope{}, ErrNoMembership
	}
	return scope, nil
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
	err     error
}

func (r *memoryRecorder) RecordDecision(ctx context.Context, record DecisionRecord) error {
	if r.err != nil {
		return r.err
	}
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

func TestDBAuthorizerRecordsAllowedDecision(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000011")
	recorder := &memoryRecorder{}
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo, recorder)

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
	if len(recorder.records) != 1 {
		t.Fatalf("expected one decision record, got %#v", recorder.records)
	}
	record := recorder.records[0]
	if !record.Allowed {
		t.Fatalf("expected allowed decision record, got %#v", record)
	}
	if record.MatchedRule != "tenant.owner" {
		t.Fatalf("expected tenant.owner rule, got %q", record.MatchedRule)
	}
	if record.TenantID != tenantID || record.TeamID != nil {
		t.Fatalf("unexpected tenant/team context: %#v", record)
	}
	if record.ActorType != ActorUser || record.ActorID != userID.String() {
		t.Fatalf("unexpected actor context: %#v", record)
	}
	if record.Action != ActionConsoleAccess || record.ResourceType != ResourceConsole || record.ResourceID != "web" {
		t.Fatalf("unexpected action/resource context: %#v", record)
	}
}

func TestDBAuthorizerPropagatesRecorderError(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000012")
	recorderErr := errors.New("record decision failed")
	recorder := &memoryRecorder{err: recorderErr}
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo, recorder)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if !errors.Is(err, recorderErr) {
		t.Fatalf("expected recorder error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected zero decision on recorder error, got %#v", decision)
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

func TestDBAuthorizerRuntimeScopeManageRequiresTenantOwnerOrAdmin(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	tests := []struct {
		name    string
		role    string
		allowed bool
	}{
		{name: "owner", role: RoleOwner, allowed: true},
		{name: "admin", role: RoleAdmin, allowed: true},
		{name: "member", role: RoleMember, allowed: false},
		{name: "viewer", role: RoleViewer, allowed: false},
		{name: "no membership", role: "", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			repo := &memoryRepository{tenantRoles: map[string]string{}}
			if tt.role != "" {
				repo.tenantRoles[tenantID.String()+":user:"+userID.String()] = tt.role
			}
			authorizer := NewDBAuthorizer(repo)

			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
				Action:   ActionRuntimeScopeManage,
				Resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
				TenantID: tenantID,
			})

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if decision.Allowed != tt.allowed {
				t.Fatalf("expected allowed=%v, got %#v", tt.allowed, decision)
			}
			if !tt.allowed && decision.Reason != ReasonNoMembership {
				t.Fatalf("expected no membership denial for %s, got %q", tt.role, decision.Reason)
			}
		})
	}
}

func TestDBAuthorizerAuthzCenterReadRequiresTenantOwnerOrAdmin(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	tests := []struct {
		name    string
		role    string
		allowed bool
	}{
		{name: "owner", role: RoleOwner, allowed: true},
		{name: "admin", role: RoleAdmin, allowed: true},
		{name: "member", role: RoleMember, allowed: false},
		{name: "viewer", role: RoleViewer, allowed: false},
		{name: "no membership", role: "", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			repo := &memoryRepository{tenantRoles: map[string]string{}}
			if tt.role != "" {
				repo.tenantRoles[tenantID.String()+":user:"+userID.String()] = tt.role
			}
			authorizer := NewDBAuthorizer(repo)

			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
				Action:   ActionAuthzCenterRead,
				Resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
				TenantID: tenantID,
			})

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if decision.Allowed != tt.allowed {
				t.Fatalf("expected allowed=%v, got %#v", tt.allowed, decision)
			}
			if !tt.allowed && decision.Reason != ReasonNoMembership {
				t.Fatalf("expected no membership denial for %s, got %q", tt.role, decision.Reason)
			}
		})
	}
}

func TestDBAuthorizerEmployeeActionsUseBusinessActionSurface(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	employeeID := uuid.MustParse("00000000-0000-0000-0000-000000000201")

	tests := []struct {
		name         string
		action       string
		resource     ResourceRef
		tenantRole   string
		allowed      bool
		matchedRule  string
		denyReason   string
		resourceType string
		resourceID   string
	}{
		{name: "owner creates employee at tenant", action: ActionEmployeeCreate, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, tenantRole: RoleOwner, allowed: true, matchedRule: "tenant.owner", resourceType: ResourceTenant, resourceID: tenantID.String()},
		{name: "admin lists employees at tenant", action: ActionEmployeeRead, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, tenantRole: RoleAdmin, allowed: true, matchedRule: "tenant.admin", resourceType: ResourceTenant, resourceID: tenantID.String()},
		{name: "admin reads employee resource", action: ActionEmployeeRead, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleAdmin, allowed: true, matchedRule: "tenant.admin", resourceType: ResourceEmployee, resourceID: employeeID.String()},
		{name: "admin updates employee status", action: ActionEmployeeStatusUpdate, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleAdmin, allowed: true, matchedRule: "tenant.admin", resourceType: ResourceEmployee, resourceID: employeeID.String()},
		{name: "admin binds execution instance", action: ActionEmployeeExecutionBind, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleAdmin, allowed: true, matchedRule: "tenant.admin", resourceType: ResourceEmployee, resourceID: employeeID.String()},
		{name: "admin creates employee config", action: ActionEmployeeConfigCreate, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleAdmin, allowed: true, matchedRule: "tenant.admin", resourceType: ResourceEmployee, resourceID: employeeID.String()},
		{name: "admin edits employee capability", action: ActionEmployeeCapabilityEdit, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleAdmin, allowed: true, matchedRule: "tenant.admin", resourceType: ResourceEmployee, resourceID: employeeID.String()},
		{name: "admin previews employee config", action: ActionEmployeeConfigPreview, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleAdmin, allowed: true, matchedRule: "tenant.admin", resourceType: ResourceEmployee, resourceID: employeeID.String()},
		{name: "owner approves employee config", action: ActionEmployeeConfigApprove, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleOwner, allowed: true, matchedRule: "tenant.owner", resourceType: ResourceEmployee, resourceID: employeeID.String()},
		{name: "member cannot manage employees", action: ActionEmployeeCreate, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, tenantRole: RoleMember, denyReason: ReasonNoMembership, resourceType: ResourceTenant, resourceID: tenantID.String()},
		{name: "employee action rejects invalid resource", action: ActionEmployeeStatusUpdate, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, tenantRole: RoleOwner, denyReason: ReasonInvalidResource, resourceType: ResourceTenant, resourceID: tenantID.String()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			recorder := &memoryRecorder{}
			repo := &memoryRepository{tenantRoles: map[string]string{}}
			if tt.tenantRole != "" {
				repo.tenantRoles[tenantID.String()+":user:"+userID.String()] = tt.tenantRole
			}
			authorizer := NewDBAuthorizer(repo, recorder)

			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
				Action:   tt.action,
				Resource: tt.resource,
				TenantID: tenantID,
			})

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if decision.Allowed != tt.allowed {
				t.Fatalf("expected allowed=%v, got %#v", tt.allowed, decision)
			}
			if tt.matchedRule != "" && decision.MatchedRule != tt.matchedRule {
				t.Fatalf("expected matched rule %s, got %#v", tt.matchedRule, decision)
			}
			if tt.denyReason != "" && decision.Reason != tt.denyReason {
				t.Fatalf("expected deny reason %s, got %#v", tt.denyReason, decision)
			}
			if len(recorder.records) != 1 {
				t.Fatalf("expected one decision record, got %#v", recorder.records)
			}
			record := recorder.records[0]
			if record.Action != tt.action || record.ResourceType != tt.resourceType || record.ResourceID != tt.resourceID {
				t.Fatalf("unexpected decision record: %#v", record)
			}
		})
	}
}

func TestDBAuthorizerEmployeeOwnerCanUsePersonalEmployeeActions(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	unrelatedID := uuid.New()
	ownerResource := ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}

	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + ownerID.String():     RoleMember,
			tenantID.String() + ":user:" + unrelatedID.String(): RoleMember,
		},
		employeeScopes: map[uuid.UUID]DigitalEmployeeAuthzScope{
			employeeID: {
				TenantID:    tenantID,
				EmployeeID:  employeeID,
				OwnerUserID: ownerID,
				TeamID:      &teamID,
			},
		},
	}
	authorizer := NewDBAuthorizer(repo)

	ownerActions := []string{
		ActionEmployeeRead,
		ActionEmployeeConfigCreate,
		ActionEmployeeConfigPreview,
		ActionEmployeeConfigApprove,
		ActionEmployeeCapabilityEdit,
	}
	for _, action := range ownerActions {
		t.Run("owner "+action, func(t *testing.T) {
			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: ownerID.String()},
				Action:   action,
				Resource: ownerResource,
				TenantID: tenantID,
			})
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !decision.Allowed || decision.MatchedRule != "employee.owner" {
				t.Fatalf("expected owner to be allowed by employee.owner, got %#v", decision)
			}
		})
	}

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: unrelatedID.String()},
		Action:   ActionEmployeeCapabilityEdit,
		Resource: ownerResource,
		TenantID: tenantID,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed || decision.Reason != ReasonNoMembership {
		t.Fatalf("expected unrelated member to be denied, got %#v", decision)
	}

	delete(repo.tenantRoles, tenantID.String()+":user:"+ownerID.String())
	decision, err = authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: ownerID.String()},
		Action:   ActionEmployeeCapabilityEdit,
		Resource: ownerResource,
		TenantID: tenantID,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed || decision.Reason != ReasonNoMembership {
		t.Fatalf("expected employee owner without tenant membership to be denied, got %#v", decision)
	}
	repo.tenantRoles[tenantID.String()+":user:"+ownerID.String()] = RoleMember

	decision, err = authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: ownerID.String()},
		Action:   ActionEmployeeStatusUpdate,
		Resource: ownerResource,
		TenantID: tenantID,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected owner not to get status updates through personal owner rule, got %#v", decision)
	}
}

func TestDBAuthorizerCredentialActionsAllowSelfResource(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	otherUserID := uuid.New()
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleMember,
		},
	}
	authorizer := NewDBAuthorizer(repo)

	for _, action := range []string{ActionCredentialRead, ActionCredentialCreate, ActionCredentialDelete} {
		t.Run("self "+action, func(t *testing.T) {
			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
				Action:   action,
				Resource: ResourceRef{Type: ResourceCredential, ID: userID.String()},
				TenantID: tenantID,
			})
			if err != nil {
				t.Fatalf("expected credential action to be supported, got %v", err)
			}
			if !decision.Allowed || decision.MatchedRule != "credential.self" {
				t.Fatalf("expected self credential resource to be allowed, got %#v", decision)
			}
		})
	}

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionCredentialRead,
		Resource: ResourceRef{Type: ResourceCredential, ID: otherUserID.String()},
		TenantID: tenantID,
	})
	if err != nil {
		t.Fatalf("expected credential action to be supported, got %v", err)
	}
	if decision.Allowed || decision.Reason != ReasonNoMembership {
		t.Fatalf("expected another user's credential resource to be denied, got %#v", decision)
	}

	delete(repo.tenantRoles, tenantID.String()+":user:"+userID.String())
	decision, err = authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionCredentialCreate,
		Resource: ResourceRef{Type: ResourceCredential, ID: userID.String()},
		TenantID: tenantID,
	})
	if err != nil {
		t.Fatalf("expected credential action to be supported, got %v", err)
	}
	if decision.Allowed || decision.Reason != ReasonNoMembership {
		t.Fatalf("expected self credential without tenant membership to be denied, got %#v", decision)
	}
}

func TestDBAuthorizerCredentialActionsRequireTenantAdmin(t *testing.T) {
	tenantID := uuid.New()
	credentialID := uuid.New()

	tests := []struct {
		name       string
		action     string
		resource   ResourceRef
		role       string
		allowed    bool
		wantReason string
	}{
		{name: "admin reads credential", action: ActionCredentialRead, resource: ResourceRef{Type: ResourceCredential, ID: credentialID.String()}, role: RoleAdmin, allowed: true},
		{name: "owner creates credential for user id resource", action: ActionCredentialCreate, resource: ResourceRef{Type: ResourceCredential, ID: uuid.New().String()}, role: RoleOwner, allowed: true},
		{name: "admin deletes credential", action: ActionCredentialDelete, resource: ResourceRef{Type: ResourceCredential, ID: credentialID.String()}, role: RoleAdmin, allowed: true},
		{name: "member cannot read credential", action: ActionCredentialRead, resource: ResourceRef{Type: ResourceCredential, ID: credentialID.String()}, role: RoleMember, wantReason: ReasonNoMembership},
		{name: "credential rejects tenant resource", action: ActionCredentialRead, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, role: RoleOwner, wantReason: ReasonInvalidResource},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			repo := &memoryRepository{tenantRoles: map[string]string{}}
			if tt.role != "" {
				repo.tenantRoles[tenantID.String()+":user:"+userID.String()] = tt.role
			}
			authorizer := NewDBAuthorizer(repo)

			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
				Action:   tt.action,
				Resource: tt.resource,
				TenantID: tenantID,
			})

			if err != nil {
				t.Fatalf("expected credential action to be supported, got %v", err)
			}
			if decision.Allowed != tt.allowed {
				t.Fatalf("expected allowed=%v, got %#v", tt.allowed, decision)
			}
			if tt.wantReason != "" && decision.Reason != tt.wantReason {
				t.Fatalf("expected reason %q, got %#v", tt.wantReason, decision)
			}
		})
	}
}

func TestDBAuthorizerTeamManagementActionsUseOpenFGAReadyRoles(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")

	tests := []struct {
		name         string
		action       string
		resource     ResourceRef
		teamID       *uuid.UUID
		tenantRole   string
		teamRole     string
		context      map[string]any
		allowed      bool
		matchedRule  string
		denyReason   string
		resourceTeam bool
	}{
		{
			name:        "tenant owner creates team at tenant resource",
			action:      ActionTeamCreate,
			resource:    ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
			tenantRole:  RoleOwner,
			allowed:     true,
			matchedRule: "tenant.owner",
		},
		{
			name:       "tenant member cannot create team",
			action:     ActionTeamCreate,
			resource:   ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
			tenantRole: RoleMember,
			denyReason: ReasonNoMembership,
		},
		{
			name:         "team member reads own team",
			action:       ActionTeamRead,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleMember,
			allowed:      true,
			matchedRule:  "team.member",
			resourceTeam: true,
		},
		{
			name:        "tenant admin lists teams at tenant resource",
			action:      ActionTeamRead,
			resource:    ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
			tenantRole:  RoleAdmin,
			allowed:     true,
			matchedRule: "tenant.admin",
		},
		{
			name:       "tenant member cannot list all teams at tenant resource",
			action:     ActionTeamRead,
			resource:   ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
			tenantRole: RoleMember,
			denyReason: ReasonNoMembership,
		},
		{
			name:         "team admin updates own team",
			action:       ActionTeamUpdate,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			allowed:      true,
			matchedRule:  "team.admin",
			resourceTeam: true,
		},
		{
			name:         "team viewer cannot update team",
			action:       ActionTeamUpdate,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleViewer,
			denyReason:   ReasonNoMembership,
			resourceTeam: true,
		},
		{
			name:         "team owner disables own team",
			action:       ActionTeamDisable,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleOwner,
			allowed:      true,
			matchedRule:  "team.owner",
			resourceTeam: true,
		},
		{
			name:         "team admin archives own team",
			action:       ActionTeamArchive,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			allowed:      true,
			matchedRule:  "team.admin",
			resourceTeam: true,
		},
		{
			name:         "team owner restores own team",
			action:       ActionTeamRestore,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleOwner,
			allowed:      true,
			matchedRule:  "team.owner",
			resourceTeam: true,
		},
		{
			name:         "team admin adds ordinary member",
			action:       ActionTeamMemberAdd,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			context:      map[string]any{"target_role": RoleMember},
			allowed:      true,
			matchedRule:  "team.admin",
			resourceTeam: true,
		},
		{
			name:         "team admin cannot directly add privileged role",
			action:       ActionTeamMemberAdd,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			context:      map[string]any{"target_role": RoleOwner},
			denyReason:   ReasonPrivilegedRoleRequiresApproval,
			resourceTeam: true,
		},
		{
			name:         "team admin changes ordinary member role",
			action:       ActionTeamMemberChangeRole,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			context:      map[string]any{"target_role": RoleViewer},
			allowed:      true,
			matchedRule:  "team.admin",
			resourceTeam: true,
		},
		{
			name:         "team admin cannot directly promote privileged role",
			action:       ActionTeamMemberChangeRole,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			context:      map[string]any{"target_role": RoleApprover},
			denyReason:   ReasonPrivilegedRoleRequiresApproval,
			resourceTeam: true,
		},
		{
			name:         "team admin removes ordinary member",
			action:       ActionTeamMemberRemove,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			context:      map[string]any{"target_role": RoleMember},
			allowed:      true,
			matchedRule:  "team.admin",
			resourceTeam: true,
		},
		{
			name:         "last team owner cannot be removed",
			action:       ActionTeamMemberRemove,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleOwner,
			context:      map[string]any{"target_role": RoleOwner, "last_team_owner": true},
			denyReason:   ReasonLastTeamOwner,
			resourceTeam: true,
		},
		{
			name:         "team admin requests privileged role change",
			action:       ActionTeamMemberRequestPrivilegedRole,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			context:      map[string]any{"target_role": RoleApprover},
			allowed:      true,
			matchedRule:  "team.admin",
			resourceTeam: true,
		},
		{
			name:         "team owner approves privileged role change",
			action:       ActionTeamMemberApprovePrivilegedRole,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleOwner,
			context:      map[string]any{"target_role": RoleAdmin},
			allowed:      true,
			matchedRule:  "team.owner",
			resourceTeam: true,
		},
		{
			name:         "team admin cannot approve privileged role change",
			action:       ActionTeamMemberApprovePrivilegedRole,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			context:      map[string]any{"target_role": RoleOwner},
			denyReason:   ReasonNoMembership,
			resourceTeam: true,
		},
		{
			name:         "team viewer reads governance",
			action:       ActionTeamGovernanceRead,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleViewer,
			allowed:      true,
			matchedRule:  "team.viewer",
			resourceTeam: true,
		},
		{
			name:         "team admin edits governance draft",
			action:       ActionTeamGovernanceEdit,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			allowed:      true,
			matchedRule:  "team.admin",
			resourceTeam: true,
		},
		{
			name:         "team approver approves governance",
			action:       ActionTeamGovernanceApprove,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleApprover,
			allowed:      true,
			matchedRule:  "team.approver",
			resourceTeam: true,
		},
		{
			name:         "team admin cannot approve governance without approver role",
			action:       ActionTeamGovernanceApprove,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			denyReason:   ReasonNoMembership,
			resourceTeam: true,
		},
		{
			name:         "team owner binds capability",
			action:       ActionTeamCapabilityBind,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleOwner,
			allowed:      true,
			matchedRule:  "team.owner",
			resourceTeam: true,
		},
		{
			name:         "team admin unbinds capability",
			action:       ActionTeamCapabilityUnbind,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleAdmin,
			allowed:      true,
			matchedRule:  "team.admin",
			resourceTeam: true,
		},
		{
			name:         "team owner reads audit",
			action:       ActionTeamAuditRead,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleOwner,
			allowed:      true,
			matchedRule:  "team.owner",
			resourceTeam: true,
		},
		{
			name:         "team viewer cannot read audit",
			action:       ActionTeamAuditRead,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			teamRole:     RoleViewer,
			denyReason:   ReasonNoMembership,
			resourceTeam: true,
		},
		{
			name:         "tenant admin manages any team",
			action:       ActionTeamMemberChangeRole,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			tenantRole:   RoleAdmin,
			context:      map[string]any{"target_role": RoleMember},
			allowed:      true,
			matchedRule:  "tenant.admin",
			resourceTeam: true,
		},
		{
			name:         "missing membership denies team action",
			action:       ActionTeamUpdate,
			resource:     ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:       &teamID,
			denyReason:   ReasonNoMembership,
			resourceTeam: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			repo := &memoryRepository{
				tenantRoles: map[string]string{},
				teamRoles:   map[string]string{},
			}
			if tt.tenantRole != "" {
				repo.tenantRoles[tenantID.String()+":user:"+userID.String()] = tt.tenantRole
			}
			if tt.teamRole != "" {
				repo.teamRoles[tenantID.String()+":"+teamID.String()+":user:"+userID.String()] = tt.teamRole
			}
			authorizer := NewDBAuthorizer(repo)

			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
				Action:   tt.action,
				Resource: tt.resource,
				TenantID: tenantID,
				TeamID:   tt.teamID,
				Context:  tt.context,
			})

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if decision.Allowed != tt.allowed {
				t.Fatalf("expected allowed=%v, got %#v", tt.allowed, decision)
			}
			if tt.matchedRule != "" && decision.MatchedRule != tt.matchedRule {
				t.Fatalf("expected matched rule %q, got %q", tt.matchedRule, decision.MatchedRule)
			}
			if tt.denyReason != "" && decision.Reason != tt.denyReason {
				t.Fatalf("expected deny reason %q, got %q", tt.denyReason, decision.Reason)
			}
		})
	}
}

func TestDBAuthorizerTeamManagementActionsDenyWrongResource(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000013")
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleOwner,
		},
		teamRoles: map[string]string{
			tenantID.String() + ":" + teamID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo)

	tests := []struct {
		name     string
		action   string
		resource ResourceRef
		teamID   *uuid.UUID
	}{
		{
			name:     "team create requires tenant resource",
			action:   ActionTeamCreate,
			resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()},
			teamID:   &teamID,
		},
		{
			name:     "team update requires matching team resource",
			action:   ActionTeamUpdate,
			resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
			teamID:   &teamID,
		},
		{
			name:     "team governance approve requires team id",
			action:   ActionTeamGovernanceApprove,
			resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
				Action:   tt.action,
				Resource: tt.resource,
				TenantID: tenantID,
				TeamID:   tt.teamID,
			})

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if decision.Allowed {
				t.Fatalf("expected invalid resource denial, got %#v", decision)
			}
			if decision.Reason != ReasonInvalidResource {
				t.Fatalf("expected invalid resource reason, got %q", decision.Reason)
			}
		})
	}
}

func TestDBAuthorizerTeamManagementActionsDenyWithoutMembership(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")

	tests := []struct {
		name     string
		action   string
		resource ResourceRef
		teamID   *uuid.UUID
		context  map[string]any
	}{
		{name: "team create", action: ActionTeamCreate, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}},
		{name: "team collection read", action: ActionTeamRead, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}},
		{name: "team read", action: ActionTeamRead, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "team update", action: ActionTeamUpdate, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "team disable", action: ActionTeamDisable, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "team archive", action: ActionTeamArchive, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "team restore", action: ActionTeamRestore, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "member add", action: ActionTeamMemberAdd, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, context: map[string]any{"target_role": RoleMember}},
		{name: "member remove", action: ActionTeamMemberRemove, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, context: map[string]any{"target_role": RoleMember}},
		{name: "member change role", action: ActionTeamMemberChangeRole, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, context: map[string]any{"target_role": RoleViewer}},
		{name: "member request privileged role", action: ActionTeamMemberRequestPrivilegedRole, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, context: map[string]any{"target_role": RoleApprover}},
		{name: "member approve privileged role", action: ActionTeamMemberApprovePrivilegedRole, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, context: map[string]any{"target_role": RoleAdmin}},
		{name: "governance read", action: ActionTeamGovernanceRead, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "governance edit", action: ActionTeamGovernanceEdit, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "governance approve", action: ActionTeamGovernanceApprove, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "capability bind", action: ActionTeamCapabilityBind, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "capability unbind", action: ActionTeamCapabilityUnbind, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "audit read", action: ActionTeamAuditRead, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewDBAuthorizer(&memoryRepository{
				tenantRoles: map[string]string{},
				teamRoles:   map[string]string{},
			})
			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: uuid.New().String()},
				Action:   tt.action,
				Resource: tt.resource,
				TenantID: tenantID,
				TeamID:   tt.teamID,
				Context:  tt.context,
			})

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if decision.Allowed || decision.Reason != ReasonNoMembership {
				t.Fatalf("expected no membership denial, got %#v", decision)
			}
		})
	}
}

func TestDBAuthorizerTeamManagementActionsValidateResourceShape(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	otherTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000015")
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleOwner,
		},
		teamRoles: map[string]string{
			tenantID.String() + ":" + teamID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo)

	tests := []struct {
		name     string
		action   string
		resource ResourceRef
		teamID   *uuid.UUID
	}{
		{name: "team create rejects team resource", action: ActionTeamCreate, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID},
		{name: "team collection read rejects wrong tenant", action: ActionTeamRead, resource: ResourceRef{Type: ResourceTenant, ID: otherTenantID.String()}},
		{name: "team read rejects tenant resource with team context", action: ActionTeamRead, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "team update rejects tenant resource", action: ActionTeamUpdate, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "team disable rejects tenant resource", action: ActionTeamDisable, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "team archive rejects tenant resource", action: ActionTeamArchive, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "team restore rejects tenant resource", action: ActionTeamRestore, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "member add rejects tenant resource", action: ActionTeamMemberAdd, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "member remove rejects tenant resource", action: ActionTeamMemberRemove, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "member change role rejects tenant resource", action: ActionTeamMemberChangeRole, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "member request privileged role rejects tenant resource", action: ActionTeamMemberRequestPrivilegedRole, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "member approve privileged role rejects tenant resource", action: ActionTeamMemberApprovePrivilegedRole, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "governance read rejects tenant resource", action: ActionTeamGovernanceRead, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "governance edit rejects tenant resource", action: ActionTeamGovernanceEdit, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "governance approve rejects tenant resource", action: ActionTeamGovernanceApprove, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "capability bind rejects tenant resource", action: ActionTeamCapabilityBind, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "capability unbind rejects tenant resource", action: ActionTeamCapabilityUnbind, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
		{name: "audit read rejects tenant resource", action: ActionTeamAuditRead, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, teamID: &teamID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
				Action:   tt.action,
				Resource: tt.resource,
				TenantID: tenantID,
				TeamID:   tt.teamID,
			})

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if decision.Allowed || decision.Reason != ReasonInvalidResource {
				t.Fatalf("expected invalid resource denial, got %#v", decision)
			}
		})
	}
}

func TestDBAuthorizerRecordsTeamManagementDecision(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000014")
	recorder := &memoryRecorder{}
	repo := &memoryRepository{
		teamRoles: map[string]string{
			tenantID.String() + ":" + teamID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo, recorder)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionTeamGovernanceApprove,
		Resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()},
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allowed decision, got %#v", decision)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("expected one decision record, got %#v", recorder.records)
	}
	record := recorder.records[0]
	if record.Action != ActionTeamGovernanceApprove || record.ResourceType != ResourceTeam || record.ResourceID != teamID.String() {
		t.Fatalf("unexpected action/resource record: %#v", record)
	}
	if record.TeamID == nil || *record.TeamID != teamID {
		t.Fatalf("expected team context in record, got %#v", record)
	}
	if !record.Allowed || record.MatchedRule != "team.owner" {
		t.Fatalf("unexpected decision record: %#v", record)
	}
}

func TestDBAuthorizerRecordsAllTeamManagementActions(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")

	tests := []struct {
		name         string
		action       string
		resource     ResourceRef
		teamID       *uuid.UUID
		tenantRole   string
		teamRole     string
		context      map[string]any
		matchedRule  string
		resourceTeam bool
	}{
		{name: "team create", action: ActionTeamCreate, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, tenantRole: RoleOwner, matchedRule: "tenant.owner"},
		{name: "team collection read", action: ActionTeamRead, resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()}, tenantRole: RoleAdmin, matchedRule: "tenant.admin"},
		{name: "team read", action: ActionTeamRead, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleMember, matchedRule: "team.member", resourceTeam: true},
		{name: "team update", action: ActionTeamUpdate, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleAdmin, matchedRule: "team.admin", resourceTeam: true},
		{name: "team disable", action: ActionTeamDisable, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleOwner, matchedRule: "team.owner", resourceTeam: true},
		{name: "team archive", action: ActionTeamArchive, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleAdmin, matchedRule: "team.admin", resourceTeam: true},
		{name: "team restore", action: ActionTeamRestore, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleOwner, matchedRule: "team.owner", resourceTeam: true},
		{name: "member add", action: ActionTeamMemberAdd, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleAdmin, context: map[string]any{"target_role": RoleMember}, matchedRule: "team.admin", resourceTeam: true},
		{name: "member remove", action: ActionTeamMemberRemove, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleAdmin, context: map[string]any{"target_role": RoleMember}, matchedRule: "team.admin", resourceTeam: true},
		{name: "member change role", action: ActionTeamMemberChangeRole, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleAdmin, context: map[string]any{"target_role": RoleViewer}, matchedRule: "team.admin", resourceTeam: true},
		{name: "member request privileged role", action: ActionTeamMemberRequestPrivilegedRole, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleAdmin, context: map[string]any{"target_role": RoleApprover}, matchedRule: "team.admin", resourceTeam: true},
		{name: "member approve privileged role", action: ActionTeamMemberApprovePrivilegedRole, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleOwner, context: map[string]any{"target_role": RoleAdmin}, matchedRule: "team.owner", resourceTeam: true},
		{name: "governance read", action: ActionTeamGovernanceRead, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleViewer, matchedRule: "team.viewer", resourceTeam: true},
		{name: "governance edit", action: ActionTeamGovernanceEdit, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleAdmin, matchedRule: "team.admin", resourceTeam: true},
		{name: "governance approve", action: ActionTeamGovernanceApprove, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleApprover, matchedRule: "team.approver", resourceTeam: true},
		{name: "capability bind", action: ActionTeamCapabilityBind, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleOwner, matchedRule: "team.owner", resourceTeam: true},
		{name: "capability unbind", action: ActionTeamCapabilityUnbind, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleAdmin, matchedRule: "team.admin", resourceTeam: true},
		{name: "capability manage", action: ActionTeamCapabilityManage, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleAdmin, matchedRule: "team.admin", resourceTeam: true},
		{name: "audit read", action: ActionTeamAuditRead, resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()}, teamID: &teamID, teamRole: RoleOwner, matchedRule: "team.owner", resourceTeam: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			recorder := &memoryRecorder{}
			repo := &memoryRepository{
				tenantRoles: map[string]string{},
				teamRoles:   map[string]string{},
			}
			if tt.tenantRole != "" {
				repo.tenantRoles[tenantID.String()+":user:"+userID.String()] = tt.tenantRole
			}
			if tt.teamRole != "" {
				repo.teamRoles[tenantID.String()+":"+teamID.String()+":user:"+userID.String()] = tt.teamRole
			}
			authorizer := NewDBAuthorizer(repo, recorder)

			decision, err := authorizer.Check(context.Background(), CheckRequest{
				Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
				Action:   tt.action,
				Resource: tt.resource,
				TenantID: tenantID,
				TeamID:   tt.teamID,
				Context:  tt.context,
			})

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !decision.Allowed {
				t.Fatalf("expected allowed decision, got %#v", decision)
			}
			if len(recorder.records) != 1 {
				t.Fatalf("expected one decision record, got %#v", recorder.records)
			}
			record := recorder.records[0]
			if record.Action != tt.action || record.ResourceType != tt.resource.Type || record.ResourceID != tt.resource.ID {
				t.Fatalf("unexpected action/resource record: %#v", record)
			}
			if record.MatchedRule != tt.matchedRule || !record.Allowed {
				t.Fatalf("unexpected decision record: %#v", record)
			}
			if tt.resourceTeam {
				if record.TeamID == nil || *record.TeamID != teamID {
					t.Fatalf("expected team context in record, got %#v", record)
				}
			} else if record.TeamID != nil {
				t.Fatalf("expected no team context in record, got %#v", record)
			}
		})
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
