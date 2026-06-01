package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/storage/queries"
)

type fakeAuthzQueryStore struct {
	tenantParams  queries.GetActiveTenantMembershipParams
	tenantMember  queries.TenantMember
	tenantErr     error
	teamParams    queries.GetActiveTeamMembershipParams
	teamMember    queries.TenantMember
	teamErr       error
	runtimeParams queries.RuntimeNodeCoversTaskScopeParams
	runtimeOK     bool
	runtimeErr    error
}

func (s *fakeAuthzQueryStore) GetActiveTenantMembership(ctx context.Context, params queries.GetActiveTenantMembershipParams) (queries.TenantMember, error) {
	s.tenantParams = params
	return s.tenantMember, s.tenantErr
}

func (s *fakeAuthzQueryStore) GetActiveTeamMembership(ctx context.Context, params queries.GetActiveTeamMembershipParams) (queries.TenantMember, error) {
	s.teamParams = params
	return s.teamMember, s.teamErr
}

func (s *fakeAuthzQueryStore) RuntimeNodeCoversTaskScope(ctx context.Context, params queries.RuntimeNodeCoversTaskScopeParams) (bool, error) {
	s.runtimeParams = params
	return s.runtimeOK, s.runtimeErr
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
