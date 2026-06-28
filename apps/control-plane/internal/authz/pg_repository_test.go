package authz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type fakeAuthzQueryStore struct {
	tenantParams         queries.GetActiveTenantMembershipParams
	tenantMember         queries.TenantMember
	tenantErr            error
	teamParams           queries.GetActiveTeamMembershipParams
	teamMember           queries.TenantMember
	teamErr              error
	employeeScopeParams  queries.GetDigitalEmployeeAuthzScopeParams
	employeeScope        queries.GetDigitalEmployeeAuthzScopeRow
	employeeScopeErr     error
	runtimeParams        queries.RuntimeNodeCoversTaskScopeParams
	runtimeOK            bool
	runtimeErr           error
	openFGAMembers       []queries.ListOpenFGAMembersRow
	openFGAMembersErr    error
	openFGAScopes        []queries.ListOpenFGAProjectTeamScopesRow
	openFGAScopesErr     error
	projectAuthzFacts    queries.GetProjectAuthzFactsRow
	projectAuthzFactsErr error
}

func (s *fakeAuthzQueryStore) GetActiveTenantMembership(ctx context.Context, params queries.GetActiveTenantMembershipParams) (queries.TenantMember, error) {
	s.tenantParams = params
	return s.tenantMember, s.tenantErr
}

func (s *fakeAuthzQueryStore) GetActiveTeamMembership(ctx context.Context, params queries.GetActiveTeamMembershipParams) (queries.TenantMember, error) {
	s.teamParams = params
	return s.teamMember, s.teamErr
}

func (s *fakeAuthzQueryStore) GetDigitalEmployeeAuthzScope(ctx context.Context, params queries.GetDigitalEmployeeAuthzScopeParams) (queries.GetDigitalEmployeeAuthzScopeRow, error) {
	s.employeeScopeParams = params
	return s.employeeScope, s.employeeScopeErr
}

func (s *fakeAuthzQueryStore) RuntimeNodeCoversTaskScope(ctx context.Context, params queries.RuntimeNodeCoversTaskScopeParams) (bool, error) {
	s.runtimeParams = params
	return s.runtimeOK, s.runtimeErr
}

func (s *fakeAuthzQueryStore) ListOpenFGAMembers(ctx context.Context) ([]queries.ListOpenFGAMembersRow, error) {
	return s.openFGAMembers, s.openFGAMembersErr
}

func (s *fakeAuthzQueryStore) ListOpenFGAProjectTeamScopes(ctx context.Context) ([]queries.ListOpenFGAProjectTeamScopesRow, error) {
	return s.openFGAScopes, s.openFGAScopesErr
}

func (s *fakeAuthzQueryStore) GetProjectAuthzFacts(ctx context.Context, params queries.GetProjectAuthzFactsParams) (queries.GetProjectAuthzFactsRow, error) {
	return s.projectAuthzFacts, s.projectAuthzFactsErr
}

func TestPgRepositoryMapsTenantMembership(t *testing.T) {
	tenantID := uuid.New()
	principalID := uuid.New()
	store := &fakeAuthzQueryStore{
		tenantMember: queries.TenantMember{
			TenantID:      tenantID,
			PrincipalType: ActorUser,
			PrincipalID:   principalID,
			Role:          RoleOwner,
			Status:        "active",
		},
	}
	repo := NewPgRepository(store)

	membership, err := repo.GetActiveTenantMembership(context.Background(), TenantMembershipParams{
		TenantID:      tenantID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	require.NoError(t, err)

	require.Equal(t, tenantID, store.tenantParams.TenantID)
	require.Equal(t, ActorUser, store.tenantParams.PrincipalType)
	require.Equal(t, principalID, store.tenantParams.PrincipalID)
	require.Equal(t, tenantID, membership.TenantID)
	require.Nil(t, membership.TeamID)
	require.Equal(t, RoleOwner, membership.Role)
	require.Equal(t, "active", membership.Status)
}

func TestPgRepositoryMapsTeamMembership(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	principalID := uuid.New()
	store := &fakeAuthzQueryStore{
		teamMember: queries.TenantMember{
			TenantID:      tenantID,
			TeamID:        uuid.NullUUID{UUID: teamID, Valid: true},
			PrincipalType: ActorUser,
			PrincipalID:   principalID,
			Role:          RoleMember,
			Status:        "active",
		},
	}
	repo := NewPgRepository(store)

	membership, err := repo.GetActiveTeamMembership(context.Background(), TeamMembershipParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	require.NoError(t, err)

	require.Equal(t, teamID, store.teamParams.TeamID)
	require.NotNil(t, membership.TeamID)
	require.Equal(t, teamID, *membership.TeamID)
	require.Equal(t, RoleMember, membership.Role)
}

func TestPgRepositoryListsOpenFGATuples(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	userID := uuid.New()
	store := &fakeAuthzQueryStore{
		openFGAMembers: []queries.ListOpenFGAMembersRow{
			{
				TenantID:      tenantID,
				PrincipalType: ActorUser,
				PrincipalID:   userID,
				Role:          RoleAdmin,
				Status:        "active",
			},
			{
				TenantID:      tenantID,
				TeamID:        uuid.NullUUID{UUID: teamID, Valid: true},
				PrincipalType: ActorUser,
				PrincipalID:   userID,
				Role:          RoleMember,
				Status:        "active",
			},
		},
		openFGAScopes: []queries.ListOpenFGAProjectTeamScopesRow{
			{TenantID: tenantID, UserID: userID, TeamID: teamID, Status: "active"},
		},
	}
	repo := NewPgRepository(store)

	tuples, err := repo.ListOpenFGATuples(context.Background())

	require.NoError(t, err)
	require.ElementsMatch(t, []OpenFGATuple{
		{User: "user:" + userID.String(), Relation: OpenFGARelationAdmin, Object: "tenant:" + tenantID.String()},
		{User: "user:" + userID.String(), Relation: OpenFGARelationMember, Object: "team:" + teamID.String()},
		{User: "user:" + userID.String(), Relation: OpenFGARelationProjectScopeUser, Object: "team:" + teamID.String()},
	}, tuples)
}

func TestPgRepositoryMapsDigitalEmployeeAuthzScope(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	ownerUserID := uuid.New()
	teamID := uuid.New()
	store := &fakeAuthzQueryStore{
		employeeScope: queries.GetDigitalEmployeeAuthzScopeRow{
			TenantID:    tenantID,
			EmployeeID:  employeeID,
			OwnerUserID: ownerUserID,
			TeamID:      uuid.NullUUID{UUID: teamID, Valid: true},
		},
	}
	repo := NewPgRepository(store)

	scope, err := repo.GetDigitalEmployeeAuthzScope(context.Background(), DigitalEmployeeAuthzScopeParams{
		TenantID:   tenantID,
		EmployeeID: employeeID,
	})
	require.NoError(t, err)

	require.Equal(t, tenantID, store.employeeScopeParams.TenantID)
	require.Equal(t, employeeID, store.employeeScopeParams.EmployeeID)
	require.Equal(t, tenantID, scope.TenantID)
	require.Equal(t, employeeID, scope.EmployeeID)
	require.Equal(t, ownerUserID, scope.OwnerUserID)
	require.NotNil(t, scope.TeamID)
	require.Equal(t, teamID, *scope.TeamID)
}

func TestPgRepositoryMapsNoRowsToNoMembership(t *testing.T) {
	store := &fakeAuthzQueryStore{tenantErr: pgx.ErrNoRows}
	repo := NewPgRepository(store)

	_, err := repo.GetActiveTenantMembership(context.Background(), TenantMembershipParams{
		TenantID:      uuid.New(),
		PrincipalType: ActorUser,
		PrincipalID:   uuid.New(),
	})
	require.ErrorIs(t, err, ErrNoMembership)
}

func TestPgRepositoryPassesRuntimeTaskScopeParams(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	taskID := uuid.New()
	store := &fakeAuthzQueryStore{runtimeOK: true}
	repo := NewPgRepository(store)

	covered, err := repo.RuntimeNodeCoversTaskScope(context.Background(), RuntimeScopeParams{
		TenantID: tenantID,
		TeamID:   &teamID,
		TaskID:   taskID,
		NodeID:   "node-1",
	})
	require.NoError(t, err)

	require.True(t, covered)
	require.Equal(t, tenantID, store.runtimeParams.TenantID)
	require.Equal(t, uuid.NullUUID{UUID: teamID, Valid: true}, store.runtimeParams.TeamID)
	require.Equal(t, taskID, store.runtimeParams.TaskID)
	require.Equal(t, "node-1", store.runtimeParams.NodeID)
	require.True(t, store.runtimeParams.LastHeartbeatAfter.Valid)
	expectedThreshold := time.Now().Add(-runtimepkg.HeartbeatTimeout)
	require.WithinDuration(t, expectedThreshold, store.runtimeParams.LastHeartbeatAfter.Time, 2*time.Second)
}

func TestPgRepositoryPreservesUnexpectedErrors(t *testing.T) {
	expected := errors.New("database unavailable")
	store := &fakeAuthzQueryStore{teamErr: expected}
	repo := NewPgRepository(store)

	_, err := repo.GetActiveTeamMembership(context.Background(), TeamMembershipParams{
		TenantID:      uuid.New(),
		TeamID:        uuid.New(),
		PrincipalType: ActorUser,
		PrincipalID:   uuid.New(),
	})
	require.ErrorIs(t, err, expected)
}
