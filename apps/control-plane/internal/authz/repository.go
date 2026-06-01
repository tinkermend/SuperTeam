package authz

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	GetActiveTenantMembership(ctx context.Context, params TenantMembershipParams) (Membership, error)
	GetActiveTeamMembership(ctx context.Context, params TeamMembershipParams) (Membership, error)
	RuntimeNodeCoversTaskScope(ctx context.Context, params RuntimeScopeParams) (bool, error)
}

type TenantMembershipParams struct {
	TenantID      uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
}

type TeamMembershipParams struct {
	TenantID      uuid.UUID
	TeamID        uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
}

type RuntimeScopeParams struct {
	TenantID uuid.UUID
	TeamID   *uuid.UUID
	NodeID   string
}
