package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type BulkTeamActionsRequest struct {
	Actor    ActorRef
	TenantID uuid.UUID
	TeamID   uuid.UUID
	Actions  []string
}

type Authorizer interface {
	Check(ctx context.Context, req CheckRequest) (Decision, error)
	CheckBulkTeamActions(ctx context.Context, req BulkTeamActionsRequest) ([]string, error)
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

func (a *DBAuthorizer) AuthzEngineStatus() EngineStatus {
	return EngineStatus{
		Engine:        "db",
		Status:        "healthy",
		EngineVersion: "db-authorizer-v1",
	}
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
	case ActionUserProjectTeamScopeRead, ActionUserProjectTeamScopeManage:
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
		ActionEmployeeTeamUpdate,
		ActionEmployeeDelete,
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
	case ActionMCPRegistryRead,
		ActionMCPRegistryManage,
		ActionScenarioTemplateRead,
		ActionScenarioTemplateManage:
		if !resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
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
	case ActionSkillDelete,
		ActionSkillInstall:
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
		ActionTeamDelete,
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
		ActionTeamAuditRead,
		ActionTeamLendingPolicyRead,
		ActionTeamLendingPolicyEdit,
		ActionTeamLendingRequestRead,
		ActionTeamLendingRequestDecide:
		if req.TeamID == nil || !resourceMatchesUUID(req.Resource, ResourceTeam, *req.TeamID) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTeamManagementAction(ctx, req)
	default:
		if isProjectAction(req.Action) {
			decision, err = a.checkProjectAccess(ctx, req)
			break
		}
		if isAuditAction(req.Action) {
			decision, err = a.checkAuditAccess(ctx, req)
			break
		}
		if isTaskAction(req.Action) {
			decision, err = a.checkTaskAccess(ctx, req)
			break
		}
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

// CheckBulkTeamActions resolves all team action permissions with a single pair of DB queries
// instead of one query per action. It fetches membership once and derives each permission
// from the resolved role in memory.
//
// ActionTeamDelete is intentionally absent from roleAllowsTeamAction — only tenant admins
// can delete teams. This function preserves that invariant.
func (a *DBAuthorizer) CheckBulkTeamActions(ctx context.Context, req BulkTeamActionsRequest) ([]string, error) {
	if a == nil || a.repository == nil {
		return nil, nil
	}
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok || req.TenantID == uuid.Nil || req.TeamID == uuid.Nil {
		return nil, nil
	}

	isTenantAdmin := false
	tenantMembership, err := a.repository.GetActiveTenantMembership(ctx, TenantMembershipParams{
		TenantID:      req.TenantID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	if err != nil && !errors.Is(err, ErrNoMembership) {
		return nil, err
	}
	if err == nil && roleAllowsTenantAdminAccess(tenantMembership.Role) {
		isTenantAdmin = true
	}

	teamRole := ""
	if !isTenantAdmin {
		teamMembership, teamErr := a.repository.GetActiveTeamMembership(ctx, TeamMembershipParams{
			TenantID:      req.TenantID,
			TeamID:        req.TeamID,
			PrincipalType: ActorUser,
			PrincipalID:   principalID,
		})
		if teamErr != nil && !errors.Is(teamErr, ErrNoMembership) {
			return nil, teamErr
		}
		if teamErr == nil {
			teamRole = teamMembership.Role
		}
	}

	allowed := make([]string, 0, len(req.Actions))
	for _, action := range req.Actions {
		if isTenantAdmin || roleAllowsTeamAction(action, teamRole) {
			allowed = append(allowed, action)
		}
	}
	return allowed, nil
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

func isProjectAction(action string) bool {
	switch action {
	case ActionProjectCreate, ActionProjectRead, ActionProjectUpdate, ActionProjectArchive, ActionProjectDelete,
		ActionProjectMemberRead, ActionProjectMemberManage,
		ActionProjectDemandRead, ActionProjectDemandSubmit,
		ActionProjectTaskRead, ActionProjectEventRead,
		ActionProjectDecisionRead, ActionProjectDecisionResolve,
		ActionProjectEvidenceRead, ActionProjectEvidenceCreate, ActionProjectEvidenceUpdate,
		ActionProjectArtifactRead, ActionProjectReportRead,
		ActionProjectBudgetRead, ActionProjectAcceptanceCreate, ActionProjectAcceptanceRead,
		ActionProjectConfigRead, ActionProjectConfigEdit:
		return true
	default:
		return false
	}
}

func isAuditAction(action string) bool {
	return action == ActionAuditRead
}

func isTaskAction(action string) bool {
	switch action {
	case ActionTaskRead, ActionTaskCreate, ActionTaskUpdate, ActionTaskCancel:
		return true
	default:
		return false
	}
}

func (a *DBAuthorizer) checkProjectAccess(ctx context.Context, req CheckRequest) (Decision, error) {
	// Tenant-scoped project actions (e.g. list/create projects) pass ResourceTenant;
	// any active tenant member is allowed.
	if req.Resource.Type == ResourceTenant {
		return a.checkTenantAccess(ctx, req)
	}
	if req.Resource.Type != ResourceProject || req.Resource.ID == "" {
		return deny(ReasonInvalidResource), nil
	}
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	tenantDecision, err := a.checkTenantAdminAccess(ctx, req)
	if err == nil && tenantDecision.Allowed {
		return tenantDecision, nil
	}
	if err != nil && !errors.Is(err, ErrNoMembership) {
		return Decision{}, err
	}
	projectID, err := uuid.Parse(req.Resource.ID)
	if err != nil {
		return deny(ReasonInvalidResource), nil
	}
	facts, err := a.repository.GetProjectAuthzFacts(ctx, ProjectAuthzParams{
		TenantID:  req.TenantID,
		ProjectID: projectID,
		UserID:    principalID,
	})
	if err != nil {
		if errors.Is(err, ErrNoMembership) {
			return deny(ReasonNoMembership), nil
		}
		return Decision{}, err
	}
	if facts.HumanOwnerUserID == principalID {
		return allow("project.owner", RoleOwner), nil
	}
	if facts.IsMember && projectActionAllowedForMember(req.Action) {
		return allow("project.member", RoleMember), nil
	}
	return deny(ReasonNoMembership), nil
}

func projectActionAllowedForMember(action string) bool {
	switch action {
	case ActionProjectRead, ActionProjectMemberRead,
		ActionProjectDemandRead, ActionProjectDemandSubmit,
		ActionProjectTaskRead, ActionProjectEventRead,
		ActionProjectDecisionRead, ActionProjectDecisionResolve,
		ActionProjectEvidenceRead, ActionProjectEvidenceCreate,
		ActionProjectArtifactRead, ActionProjectReportRead,
		ActionProjectBudgetRead, ActionProjectAcceptanceRead,
		ActionProjectConfigRead:
		return true
	default:
		return false
	}
}

func (a *DBAuthorizer) checkAuditAccess(ctx context.Context, req CheckRequest) (Decision, error) {
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	tenantDecision, err := a.checkTenantAdminAccess(ctx, req)
	if err == nil && tenantDecision.Allowed {
		return tenantDecision, nil
	}
	if err != nil && !errors.Is(err, ErrNoMembership) {
		return Decision{}, err
	}
	if req.Resource.Type == ResourceProject && req.Resource.ID != "" {
		projectID, err := uuid.Parse(req.Resource.ID)
		if err != nil {
			return deny(ReasonInvalidResource), nil
		}
		facts, err := a.repository.GetProjectAuthzFacts(ctx, ProjectAuthzParams{
			TenantID:  req.TenantID,
			ProjectID: projectID,
			UserID:    principalID,
		})
		if err != nil {
			if errors.Is(err, ErrNoMembership) {
				return deny(ReasonNoMembership), nil
			}
			return Decision{}, err
		}
		if facts.HumanOwnerUserID == principalID || facts.IsMember {
			return allow("audit.project_member", RoleMember), nil
		}
	}
	return deny(ReasonNoMembership), nil
}

func (a *DBAuthorizer) checkTaskAccess(ctx context.Context, req CheckRequest) (Decision, error) {
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
	case ActionTeamRead, ActionTeamGovernanceRead,
		ActionTeamLendingPolicyRead, ActionTeamLendingRequestRead:
		return roleAllowsTeamRead(role)
	case ActionTeamUpdate,
		ActionTeamMemberAdd,
		ActionTeamMemberRemove,
		ActionTeamMemberChangeRole,
		ActionTeamMemberRequestPrivilegedRole,
		ActionTeamGovernanceEdit,
		ActionTeamCapabilityBind,
		ActionTeamCapabilityUnbind,
		ActionTeamCapabilityManage,
		ActionTeamAuditRead,
		ActionTeamLendingPolicyEdit, ActionTeamLendingRequestDecide:
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
