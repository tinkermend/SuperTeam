package authz

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOpenFGACheckForTenantAdminAction(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")

	check, ok := OpenFGACheckForRequest(CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionAuthzCenterRead,
		Resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
		TenantID: tenantID,
	})

	require.True(t, ok)
	require.Equal(t, "user:"+userID.String(), check.User)
	require.Equal(t, "admin", check.Relation)
	require.Equal(t, "tenant:"+tenantID.String(), check.Object)
}

func TestOpenFGACheckForTeamGovernanceRead(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-4000-8000-000000000010")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000002")

	check, ok := OpenFGACheckForRequest(CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionTeamGovernanceRead,
		Resource: ResourceRef{Type: ResourceTeam, ID: teamID.String()},
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	require.True(t, ok)
	require.Equal(t, "user:"+userID.String(), check.User)
	require.Equal(t, "viewer", check.Relation)
	require.Equal(t, "team:"+teamID.String(), check.Object)
}

func TestOpenFGATuplesForTenantMembershipAndProjectTeamScope(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-4000-8000-000000000010")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000003")

	tenantTuple, ok := OpenFGATupleForMembership(Membership{
		TenantID:      tenantID,
		PrincipalType: ActorUser,
		PrincipalID:   userID,
		Role:          RoleAdmin,
		Status:        "active",
	})
	require.True(t, ok)
	require.Equal(t, "user:"+userID.String(), tenantTuple.User)
	require.Equal(t, "admin", tenantTuple.Relation)
	require.Equal(t, "tenant:"+tenantID.String(), tenantTuple.Object)

	scopeTuple, ok := OpenFGATupleForProjectTeamScope(tenantID, userID, teamID, "active")
	require.True(t, ok)
	require.Equal(t, "user:"+userID.String(), scopeTuple.User)
	require.Equal(t, "project_scope_user", scopeTuple.Relation)
	require.Equal(t, "team:"+teamID.String(), scopeTuple.Object)

	_, ok = OpenFGATupleForProjectTeamScope(tenantID, userID, teamID, "revoked")
	require.False(t, ok)
}
