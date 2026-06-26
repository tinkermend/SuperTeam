package authz

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type QueryStore interface {
	GetActiveTenantMembership(ctx context.Context, params queries.GetActiveTenantMembershipParams) (queries.TenantMember, error)
	GetActiveTeamMembership(ctx context.Context, params queries.GetActiveTeamMembershipParams) (queries.TenantMember, error)
	GetDigitalEmployeeAuthzScope(ctx context.Context, params queries.GetDigitalEmployeeAuthzScopeParams) (queries.GetDigitalEmployeeAuthzScopeRow, error)
	GetProjectAuthzFacts(ctx context.Context, params queries.GetProjectAuthzFactsParams) (queries.GetProjectAuthzFactsRow, error)
	ListOpenFGAMembers(ctx context.Context) ([]queries.ListOpenFGAMembersRow, error)
	ListOpenFGAProjectTeamScopes(ctx context.Context) ([]queries.ListOpenFGAProjectTeamScopesRow, error)
	RuntimeNodeCoversTaskScope(ctx context.Context, params queries.RuntimeNodeCoversTaskScopeParams) (bool, error)
}

type PgRepository struct {
	q QueryStore
}

func NewPgRepository(q QueryStore) *PgRepository {
	return &PgRepository{q: q}
}

func (r *PgRepository) GetActiveTenantMembership(ctx context.Context, params TenantMembershipParams) (Membership, error) {
	member, err := r.q.GetActiveTenantMembership(ctx, queries.GetActiveTenantMembershipParams{
		TenantID:      params.TenantID,
		PrincipalType: params.PrincipalType,
		PrincipalID:   params.PrincipalID,
	})
	if err != nil {
		return Membership{}, mapMembershipError(err)
	}
	return membershipFromTenantMember(member), nil
}

func (r *PgRepository) GetActiveTeamMembership(ctx context.Context, params TeamMembershipParams) (Membership, error) {
	member, err := r.q.GetActiveTeamMembership(ctx, queries.GetActiveTeamMembershipParams{
		TenantID:      params.TenantID,
		TeamID:        params.TeamID,
		PrincipalType: params.PrincipalType,
		PrincipalID:   params.PrincipalID,
	})
	if err != nil {
		return Membership{}, mapMembershipError(err)
	}
	return membershipFromTenantMember(member), nil
}

func (r *PgRepository) GetDigitalEmployeeAuthzScope(ctx context.Context, params DigitalEmployeeAuthzScopeParams) (DigitalEmployeeAuthzScope, error) {
	scope, err := r.q.GetDigitalEmployeeAuthzScope(ctx, queries.GetDigitalEmployeeAuthzScopeParams{
		TenantID:   params.TenantID,
		EmployeeID: params.EmployeeID,
	})
	if err != nil {
		return DigitalEmployeeAuthzScope{}, mapMembershipError(err)
	}
	var teamID *uuid.UUID
	if scope.TeamID.Valid {
		teamID = &scope.TeamID.UUID
	}
	return DigitalEmployeeAuthzScope{
		TenantID:    scope.TenantID,
		EmployeeID:  scope.EmployeeID,
		OwnerUserID: scope.OwnerUserID,
		TeamID:      teamID,
	}, nil
}

func (r *PgRepository) GetProjectAuthzFacts(ctx context.Context, params ProjectAuthzParams) (ProjectAuthzFacts, error) {
	row, err := r.q.GetProjectAuthzFacts(ctx, queries.GetProjectAuthzFactsParams{
		TenantID:      params.TenantID,
		ProjectID:     params.ProjectID,
		UserID:        params.UserID,
		PrincipalType: ActorUser,
	})
	if err != nil {
		return ProjectAuthzFacts{}, mapMembershipError(err)
	}
	var teamID *uuid.UUID
	if row.TeamID.Valid {
		teamID = &row.TeamID.UUID
	}
	return ProjectAuthzFacts{
		HumanOwnerUserID: row.HumanOwnerUserID,
		IsMember:         row.IsMember,
		TeamID:           teamID,
	}, nil
}

func (r *PgRepository) RuntimeNodeCoversTaskScope(ctx context.Context, params RuntimeScopeParams) (bool, error) {
	teamID := uuid.NullUUID{}
	if params.TeamID != nil {
		teamID = uuid.NullUUID{UUID: *params.TeamID, Valid: true}
	}
	return r.q.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID:           params.TenantID,
		TeamID:             teamID,
		TaskID:             params.TaskID,
		NodeID:             params.NodeID,
		LastHeartbeatAfter: timestamptz(time.Now().Add(-runtimepkg.HeartbeatTimeout)),
	})
}

func (r *PgRepository) ListOpenFGATuples(ctx context.Context) ([]OpenFGATuple, error) {
	if r == nil || r.q == nil {
		return nil, nil
	}
	members, err := r.q.ListOpenFGAMembers(ctx)
	if err != nil {
		return nil, err
	}
	scopes, err := r.q.ListOpenFGAProjectTeamScopes(ctx)
	if err != nil {
		return nil, err
	}
	tuples := make([]OpenFGATuple, 0, len(members)+len(scopes))
	for _, member := range members {
		if tuple, ok := OpenFGATupleForMembership(membershipFromOpenFGAMember(member)); ok {
			tuples = append(tuples, tuple)
		}
	}
	for _, scope := range scopes {
		if tuple, ok := OpenFGATupleForProjectTeamScope(scope.TenantID, scope.UserID, scope.TeamID, scope.Status); ok {
			tuples = append(tuples, tuple)
		}
	}
	return tuples, nil
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func membershipFromTenantMember(member queries.TenantMember) Membership {
	var teamID *uuid.UUID
	if member.TeamID.Valid {
		teamID = &member.TeamID.UUID
	}
	return Membership{
		TenantID:      member.TenantID,
		TeamID:        teamID,
		PrincipalType: member.PrincipalType,
		PrincipalID:   member.PrincipalID,
		Role:          member.Role,
		Status:        member.Status,
	}
}

func membershipFromOpenFGAMember(member queries.ListOpenFGAMembersRow) Membership {
	var teamID *uuid.UUID
	if member.TeamID.Valid {
		teamID = &member.TeamID.UUID
	}
	return Membership{
		TenantID:      member.TenantID,
		TeamID:        teamID,
		PrincipalType: member.PrincipalType,
		PrincipalID:   member.PrincipalID,
		Role:          member.Role,
		Status:        member.Status,
	}
}

func mapMembershipError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoMembership
	}
	return err
}
