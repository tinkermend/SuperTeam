package permission

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/tenant"
)

// ErrNoApprover means no eligible approver could be resolved in the request's
// scope (e.g. a team with no approver/owner/admin other than the requester).
var ErrNoApprover = errors.New("permission: no eligible approver in scope")

// TeamMemberLister is the minimal tenant surface the approver router needs;
// tenant.Service satisfies it.
type TeamMemberLister interface {
	ListTeamMembers(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*tenant.TeamMember, error)
}

// ApproverRouter resolves who should decide a permission request (§4.5, v1).
// v1 is single-approver routing: the primary approver in scope, with a
// owner/admin fallback. Candidate-set routing (target_user_ids) is a followup.
type ApproverRouter struct {
	members TeamMemberLister
}

func NewApproverRouter(members TeamMemberLister) *ApproverRouter {
	return &ApproverRouter{members: members}
}

// rolePriority orders team roles from most to least preferred as an approver.
var approverRolePriority = []string{tenant.TeamRoleApprover, tenant.TeamRoleOwner, tenant.TeamRoleAdmin}

// ResolveTeamApprover picks the primary approver for a team-scoped permission
// request. It prefers an active approver-role member, then owner, then admin,
// excluding the requester (no self-approval). Returns ErrNoApprover when the
// scope has no eligible non-requester approver.
func (r *ApproverRouter) ResolveTeamApprover(ctx context.Context, tenantID, teamID, requester uuid.UUID) (uuid.UUID, error) {
	members, err := r.members.ListTeamMembers(ctx, tenantID, teamID, 200, 0)
	if err != nil {
		return uuid.Nil, err
	}
	byRole := map[string][]uuid.UUID{}
	for _, m := range members {
		if m == nil || m.UserID == uuid.Nil || m.UserID == requester {
			continue
		}
		if m.MembershipStatus != "" && m.MembershipStatus != "active" {
			continue
		}
		byRole[m.Role] = append(byRole[m.Role], m.UserID)
	}
	for _, role := range approverRolePriority {
		if ids := byRole[role]; len(ids) > 0 {
			return ids[0], nil
		}
	}
	return uuid.Nil, ErrNoApprover
}
