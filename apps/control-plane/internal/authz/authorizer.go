package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Authorizer interface {
	Check(ctx context.Context, req CheckRequest) (Decision, error)
}

type DBAuthorizer struct {
	repository Repository
	recorder   DecisionRecorder
}

func NewDBAuthorizer(repository Repository, recorder ...DecisionRecorder) *DBAuthorizer {
	var r DecisionRecorder
	if len(recorder) > 0 {
		r = recorder[0]
	}
	return &DBAuthorizer{repository: repository, recorder: r}
}

func (a *DBAuthorizer) Check(ctx context.Context, req CheckRequest) (Decision, error) {
	var decision Decision
	var err error
	if a == nil || a.repository == nil {
		return Decision{Allowed: false, Reason: "authorizer is not configured", RequiresAudit: true}, nil
	}
	switch req.Action {
	case ActionConsoleAccess:
		if !validResource(req.Resource, ResourceConsole) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAccess(ctx, req)
	case ActionTenantAccess:
		if !resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAccess(ctx, req)
	case ActionTeamAccess:
		if req.TeamID == nil || !resourceMatchesUUID(req.Resource, ResourceTeam, *req.TeamID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTeamAccess(ctx, req)
	case ActionTaskClaim:
		if !validResource(req.Resource, ResourceTask) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkRuntimeTaskClaim(ctx, req)
	case ActionRuntimeScopeManage:
		if !resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
	case ActionAuthzCenterRead:
		if !resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
	case ActionEmployeeCreate:
		if !resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
	case ActionEmployeeRead:
		if resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
			decision, err = a.checkTenantAdminAccess(ctx, req)
			break
		}
		if !validUUIDResource(req.Resource, ResourceEmployee) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkEmployeeOwnerAction(ctx, req)
	case ActionEmployeeStatusUpdate,
		ActionEmployeeExecutionBind,
		ActionEmployeeRunCreate,
		ActionEmployeeRunStop,
		ActionEmployeeRunLogRead:
		if !validUUIDResource(req.Resource, ResourceEmployee) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
	case ActionEmployeeConfigCreate,
		ActionEmployeeConfigPreview,
		ActionEmployeeConfigApprove,
		ActionEmployeeCapabilityEdit:
		if !validUUIDResource(req.Resource, ResourceEmployee) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkEmployeeOwnerAction(ctx, req)
	case ActionCredentialRead,
		ActionCredentialCreate,
		ActionCredentialDelete:
		if !validUUIDResource(req.Resource, ResourceCredential) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkCredentialSelfOrTenantAdmin(ctx, req)
	case ActionSkillRead:
		if resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
			decision, err = a.checkTenantAdminAccess(ctx, req)
			break
		}
		if !validUUIDResource(req.Resource, ResourceSkill) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
	case ActionSkillUpload:
		if !resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
	case ActionSkillUpdate:
		if !validUUIDResource(req.Resource, ResourceSkill) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
	case ActionTeamCreate:
		if !resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
	case ActionTeamRead:
		if resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) && req.TeamID == nil {
			decision, err = a.checkTenantAdminAccess(ctx, req)
			break
		}
		if req.TeamID == nil || !resourceMatchesUUID(req.Resource, ResourceTeam, *req.TeamID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTeamManagementAction(ctx, req)
	case ActionTeamUpdate,
		ActionTeamDisable,
		ActionTeamArchive,
		ActionTeamRestore,
		ActionTeamMemberAdd,
		ActionTeamMemberRemove,
		ActionTeamMemberChangeRole,
		ActionTeamMemberRequestPrivilegedRole,
		ActionTeamMemberApprovePrivilegedRole,
		ActionTeamGovernanceRead,
		ActionTeamGovernanceEdit,
		ActionTeamGovernanceApprove,
		ActionTeamCapabilityBind,
		ActionTeamCapabilityUnbind,
		ActionTeamCapabilityManage,
		ActionTeamAuditRead:
		if req.TeamID == nil || !resourceMatchesUUID(req.Resource, ResourceTeam, *req.TeamID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTeamManagementAction(ctx, req)
	default:
		return Decision{Allowed: false, Reason: ReasonUnsupportedAction, RequiresAudit: true}, ErrUnsupportedAction
	}
	if err != nil {
		return decision, err
	}
	if recordErr := a.record(ctx, req, decision); recordErr != nil {
		return Decision{}, recordErr
	}
	return decision, nil
}

func (a *DBAuthorizer) record(ctx context.Context, req CheckRequest, decision Decision) error {
	if a.recorder == nil {
		return nil
	}
	return a.recorder.RecordDecision(ctx, DecisionRecord{
		TenantID:     req.TenantID,
		TeamID:       req.TeamID,
		ActorType:    req.Actor.Type,
		ActorID:      req.Actor.ID,
		Action:       req.Action,
		ResourceType: req.Resource.Type,
		ResourceID:   req.Resource.ID,
		Allowed:      decision.Allowed,
		Reason:       decision.Reason,
		MatchedRule:  decision.MatchedRule,
		Engine:       "db",
		Snapshot:     decision.Snapshot,
	})
}

func (a *DBAuthorizer) checkTenantAccess(ctx context.Context, req CheckRequest) (Decision, error) {
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	membership, err := a.repository.GetActiveTenantMembership(ctx, TenantMembershipParams{
		TenantID:      req.TenantID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	if err != nil {
		if errors.Is(err, ErrNoMembership) {
			return deny(ReasonNoMembership), nil
		}
		return Decision{}, err
	}
	if roleAllowsTenantAccess(membership.Role) {
		return allow("tenant."+membership.Role, membership.Role), nil
	}
	return deny(ReasonNoMembership), nil
}

func (a *DBAuthorizer) checkTeamAccess(ctx context.Context, req CheckRequest) (Decision, error) {
	if req.TeamID == nil {
		return a.checkTenantAccess(ctx, req)
	}
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	membership, err := a.repository.GetActiveTeamMembership(ctx, TeamMembershipParams{
		TenantID:      req.TenantID,
		TeamID:        *req.TeamID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	if err == nil && roleAllowsTenantAccess(membership.Role) {
		return allow("team."+membership.Role, membership.Role), nil
	}
	if err != nil && !errors.Is(err, ErrNoMembership) {
		return Decision{}, err
	}
	return a.checkTenantAccess(ctx, req)
}

func (a *DBAuthorizer) checkTenantAdminAccess(ctx context.Context, req CheckRequest) (Decision, error) {
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	membership, err := a.repository.GetActiveTenantMembership(ctx, TenantMembershipParams{
		TenantID:      req.TenantID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	if err != nil {
		if errors.Is(err, ErrNoMembership) {
			return deny(ReasonNoMembership), nil
		}
		return Decision{}, err
	}
	if roleAllowsTenantAdminAccess(membership.Role) {
		return allow("tenant."+membership.Role, membership.Role), nil
	}
	return deny(ReasonNoMembership), nil
}

func (a *DBAuthorizer) checkEmployeeOwnerAction(ctx context.Context, req CheckRequest) (Decision, error) {
	adminDecision, err := a.checkTenantAdminAccess(ctx, req)
	if err != nil || adminDecision.Allowed || adminDecision.Reason != ReasonNoMembership {
		return adminDecision, err
	}
	tenantDecision, err := a.checkTenantAccess(ctx, req)
	if err != nil || !tenantDecision.Allowed {
		return tenantDecision, err
	}
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	employeeID, err := uuid.Parse(req.Resource.ID)
	if err != nil {
		return deny(ReasonInvalidResource), nil
	}
	scope, err := a.repository.GetDigitalEmployeeAuthzScope(ctx, DigitalEmployeeAuthzScopeParams{
		TenantID:   req.TenantID,
		EmployeeID: employeeID,
	})
	if err != nil {
		if errors.Is(err, ErrNoMembership) {
			return deny(ReasonNoMembership), nil
		}
		return Decision{}, err
	}
	if scope.OwnerUserID == principalID {
		return allow("employee.owner", "owner"), nil
	}
	return deny(ReasonNoMembership), nil
}

func (a *DBAuthorizer) checkCredentialSelfOrTenantAdmin(ctx context.Context, req CheckRequest) (Decision, error) {
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	credentialResourceID, err := uuid.Parse(req.Resource.ID)
	if err != nil {
		return deny(ReasonInvalidResource), nil
	}
	if credentialResourceID == principalID {
		tenantDecision, err := a.checkTenantAccess(ctx, req)
		if err != nil || !tenantDecision.Allowed {
			return tenantDecision, err
		}
		return allow("credential.self", "self"), nil
	}
	return a.checkTenantAdminAccess(ctx, req)
}

func (a *DBAuthorizer) checkTeamManagementAction(ctx context.Context, req CheckRequest) (Decision, error) {
	if req.TeamID == nil {
		return deny(ReasonInvalidResource), nil
	}
	if teamActionRequiresOrdinaryRoleTarget(req.Action) && isPrivilegedTargetRole(req.Context) {
		return deny(ReasonPrivilegedRoleRequiresApproval), nil
	}
	if teamActionCanRemoveOwner(req.Action) && contextBool(req.Context, "last_team_owner") {
		return deny(ReasonLastTeamOwner), nil
	}
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	tenantMembership, err := a.repository.GetActiveTenantMembership(ctx, TenantMembershipParams{
		TenantID:      req.TenantID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	if err == nil && roleAllowsTenantAdminAccess(tenantMembership.Role) {
		return allow("tenant."+tenantMembership.Role, tenantMembership.Role), nil
	}
	if err != nil && !errors.Is(err, ErrNoMembership) {
		return Decision{}, err
	}
	teamMembership, err := a.repository.GetActiveTeamMembership(ctx, TeamMembershipParams{
		TenantID:      req.TenantID,
		TeamID:        *req.TeamID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	if err != nil {
		if errors.Is(err, ErrNoMembership) {
			return deny(ReasonNoMembership), nil
		}
		return Decision{}, err
	}
	if roleAllowsTeamAction(req.Action, teamMembership.Role) {
		return allow("team."+teamMembership.Role, teamMembership.Role), nil
	}
	return deny(ReasonNoMembership), nil
}

func (a *DBAuthorizer) checkRuntimeTaskClaim(ctx context.Context, req CheckRequest) (Decision, error) {
	if req.Actor.Type != ActorRuntimeNode || req.Actor.ID == "" {
		return deny(ReasonInvalidActor), nil
	}
	taskID, err := uuid.Parse(req.Resource.ID)
	if err != nil {
		return deny(ReasonInvalidResource), nil
	}
	covered, err := a.repository.RuntimeNodeCoversTaskScope(ctx, RuntimeScopeParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
		TaskID:   taskID,
		NodeID:   req.Actor.ID,
	})
	if err != nil {
		return Decision{}, err
	}
	if !covered {
		return deny(ReasonRuntimeScopeMissing), nil
	}
	return Decision{
		Allowed:     true,
		Reason:      ReasonAllowed,
		MatchedRule: "runtime.scope",
		Snapshot: map[string]any{
			"engine": "db",
			"action": req.Action,
		},
	}, nil
}

func parseUUIDActor(actor ActorRef, expectedType string) (uuid.UUID, bool) {
	if actor.Type != expectedType {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(actor.ID)
	return id, err == nil
}

func validResource(resource ResourceRef, expectedType string) bool {
	return resource.Type == expectedType && resource.ID != ""
}

func resourceMatchesUUID(resource ResourceRef, expectedType string, expectedID uuid.UUID) bool {
	if resource.Type != expectedType {
		return false
	}
	id, err := uuid.Parse(resource.ID)
	return err == nil && id == expectedID
}

func validUUIDResource(resource ResourceRef, expectedType string) bool {
	if resource.Type != expectedType {
		return false
	}
	_, err := uuid.Parse(resource.ID)
	return err == nil
}

func roleAllowsTenantAccess(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleMember, RoleViewer:
		return true
	default:
		return false
	}
}

func roleAllowsTenantAdminAccess(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin:
		return true
	default:
		return false
	}
}

func roleAllowsTeamAction(action, role string) bool {
	switch action {
	case ActionTeamRead, ActionTeamGovernanceRead:
		return roleAllowsTeamRead(role)
	case ActionTeamUpdate,
		ActionTeamDisable,
		ActionTeamArchive,
		ActionTeamRestore,
		ActionTeamMemberAdd,
		ActionTeamMemberRemove,
		ActionTeamMemberChangeRole,
		ActionTeamMemberRequestPrivilegedRole,
		ActionTeamGovernanceEdit,
		ActionTeamCapabilityBind,
		ActionTeamCapabilityUnbind,
		ActionTeamCapabilityManage,
		ActionTeamAuditRead:
		return roleAllowsTeamManagement(role)
	case ActionTeamMemberApprovePrivilegedRole:
		return role == RoleOwner
	case ActionTeamGovernanceApprove:
		return role == RoleOwner || role == RoleApprover
	default:
		return false
	}
}

func roleAllowsTeamRead(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleApprover, RoleMember, RoleViewer:
		return true
	default:
		return false
	}
}

func roleAllowsTeamManagement(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin:
		return true
	default:
		return false
	}
}

func teamActionRequiresOrdinaryRoleTarget(action string) bool {
	switch action {
	case ActionTeamMemberAdd, ActionTeamMemberChangeRole:
		return true
	default:
		return false
	}
}

func teamActionCanRemoveOwner(action string) bool {
	switch action {
	case ActionTeamMemberRemove, ActionTeamMemberChangeRole:
		return true
	default:
		return false
	}
}

func isPrivilegedTargetRole(context map[string]any) bool {
	role, ok := contextString(context, "target_role")
	if !ok {
		return false
	}
	switch role {
	case RoleOwner, RoleAdmin, RoleApprover:
		return true
	default:
		return false
	}
}

func contextString(context map[string]any, key string) (string, bool) {
	if context == nil {
		return "", false
	}
	value, ok := context[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func contextBool(context map[string]any, key string) bool {
	if context == nil {
		return false
	}
	value, ok := context[key]
	if !ok {
		return false
	}
	enabled, ok := value.(bool)
	return ok && enabled
}

func allow(rule string, role string) Decision {
	return Decision{
		Allowed:     true,
		Reason:      ReasonAllowed,
		MatchedRule: rule,
		Snapshot: map[string]any{
			"engine": "db",
			"role":   role,
		},
	}
}

func deny(reason string) Decision {
	return Decision{
		Allowed:       false,
		Reason:        reason,
		RequiresAudit: true,
		Snapshot: map[string]any{
			"engine": "db",
		},
	}
}
