package authz

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	GetActiveTenantMembership(ctx context.Context, params TenantMembershipParams) (Membership, error)
	GetActiveTeamMembership(ctx context.Context, params TeamMembershipParams) (Membership, error)
	GetDigitalEmployeeAuthzScope(ctx context.Context, params DigitalEmployeeAuthzScopeParams) (DigitalEmployeeAuthzScope, error)
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
	TaskID   uuid.UUID
	NodeID   string
}

type DigitalEmployeeAuthzScopeParams struct {
	TenantID   uuid.UUID
	EmployeeID uuid.UUID
}

type DigitalEmployeeAuthzScope struct {
	TenantID    uuid.UUID
	EmployeeID  uuid.UUID
	OwnerUserID uuid.UUID
	TeamID      *uuid.UUID
}
