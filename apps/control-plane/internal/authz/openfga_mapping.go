package authz

import (
	"strings"

	"github.com/google/uuid"
)

const (
	OpenFGARelationOwner            = "owner"
	OpenFGARelationAdmin            = "admin"
	OpenFGARelationApprover         = "approver"
	OpenFGARelationMember           = "member"
	OpenFGARelationViewer           = "viewer"
	OpenFGARelationProjectScopeUser = "project_scope_user"
)

type OpenFGACheck struct {
	User     string
	Relation string
	Object   string
}

type OpenFGATuple struct {
	User     string
	Relation string
	Object   string
}

func OpenFGACheckForRequest(req CheckRequest) (OpenFGACheck, bool) {
	if req.Actor.Type != ActorUser || strings.TrimSpace(req.Actor.ID) == "" {
		return OpenFGACheck{}, false
	}
	relation, ok := openFGARelationForAction(req.Action)
	if !ok {
		return OpenFGACheck{}, false
	}
	object, ok := openFGAObjectForRequest(req)
	if !ok {
		return OpenFGACheck{}, false
	}
	return OpenFGACheck{
		User:     openFGAUser(ActorUser, req.Actor.ID),
		Relation: relation,
		Object:   object,
	}, true
}

func OpenFGATupleForMembership(membership Membership) (OpenFGATuple, bool) {
	if membership.PrincipalType != ActorUser || membership.PrincipalID == uuid.Nil || membership.TenantID == uuid.Nil {
		return OpenFGATuple{}, false
	}
	relation, ok := openFGARelationForRole(membership.Role)
	if !ok || membership.Status != "active" {
		return OpenFGATuple{}, false
	}
	object := openFGAObject(ResourceTenant, membership.TenantID.String())
	if membership.TeamID != nil && *membership.TeamID != uuid.Nil {
		object = openFGAObject(ResourceTeam, membership.TeamID.String())
	}
	return OpenFGATuple{
		User:     openFGAUser(ActorUser, membership.PrincipalID.String()),
		Relation: relation,
		Object:   object,
	}, true
}

func OpenFGATupleForProjectTeamScope(tenantID, userID, teamID uuid.UUID, status string) (OpenFGATuple, bool) {
	if tenantID == uuid.Nil || userID == uuid.Nil || teamID == uuid.Nil || status != "active" {
		return OpenFGATuple{}, false
	}
	return OpenFGATuple{
		User:     openFGAUser(ActorUser, userID.String()),
		Relation: OpenFGARelationProjectScopeUser,
		Object:   openFGAObject(ResourceTeam, teamID.String()),
	}, true
}

func openFGARelationForAction(action string) (string, bool) {
	switch action {
	case ActionConsoleAccess, ActionTenantAccess, ActionTeamAccess, ActionTeamRead, ActionTeamGovernanceRead, ActionTeamLendingPolicyRead, ActionTeamLendingRequestRead:
		return OpenFGARelationViewer, true
	case ActionRuntimeScopeManage, ActionAuthzCenterRead, ActionUserProjectTeamScopeRead, ActionUserProjectTeamScopeManage, ActionEmployeeCreate, ActionEmployeeDelete, ActionTeamCreate,
		ActionTeamUpdate, ActionTeamDisable, ActionTeamArchive, ActionTeamRestore, ActionTeamDelete, ActionTeamMemberAdd, ActionTeamMemberRemove, ActionTeamMemberChangeRole,
		ActionTeamMemberRequestPrivilegedRole, ActionTeamCapabilityBind, ActionTeamCapabilityUnbind, ActionTeamCapabilityManage, ActionTeamAuditRead,
		ActionTeamLendingPolicyEdit, ActionTeamLendingRequestDecide, ActionSkillInstall:
		return OpenFGARelationAdmin, true
	case ActionTeamMemberApprovePrivilegedRole:
		return OpenFGARelationOwner, true
	case ActionTeamGovernanceApprove:
		return OpenFGARelationApprover, true
	default:
		return "", false
	}
}

func openFGARelationForRole(role string) (string, bool) {
	switch role {
	case RoleOwner:
		return OpenFGARelationOwner, true
	case RoleAdmin:
		return OpenFGARelationAdmin, true
	case RoleApprover:
		return OpenFGARelationApprover, true
	case RoleMember:
		return OpenFGARelationMember, true
	case RoleViewer:
		return OpenFGARelationViewer, true
	default:
		return "", false
	}
}

func openFGAObjectForRequest(req CheckRequest) (string, bool) {
	switch req.Resource.Type {
	case ResourceTenant:
		if req.Resource.ID == "" {
			return "", false
		}
		return openFGAObject(ResourceTenant, req.Resource.ID), true
	case ResourceTeam:
		if req.Resource.ID == "" {
			return "", false
		}
		return openFGAObject(ResourceTeam, req.Resource.ID), true
	case ResourceSkill:
		if req.Action == ActionSkillInstall {
			if req.TenantID == uuid.Nil {
				return "", false
			}
			return openFGAObject(ResourceTenant, req.TenantID.String()), true
		}
		if req.Resource.ID == "" {
			return "", false
		}
		return openFGAObject(ResourceSkill, req.Resource.ID), true
	case ResourceConsole:
		if req.TenantID == uuid.Nil {
			return "", false
		}
		return openFGAObject(ResourceTenant, req.TenantID.String()), true
	default:
		return "", false
	}
}

func openFGAUser(actorType, actorID string) string {
	return actorType + ":" + actorID
}

func openFGAObject(resourceType, resourceID string) string {
	return resourceType + ":" + resourceID
}
