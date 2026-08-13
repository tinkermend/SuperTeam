package externalintegration

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateIntegration(ctx context.Context, integration Integration) (Integration, error)
	GetIntegration(ctx context.Context, tenantID, integrationID uuid.UUID) (Integration, error)
	ListIntegrations(ctx context.Context, tenantID uuid.UUID, projectID *uuid.UUID) ([]Integration, error)
	UpdateIntegration(ctx context.Context, integration Integration) (Integration, error)
	// ConsumeBudget atomically counts one call in the fixed hourly window.
	// Returns false (no error) when the window is full — the caller maps that
	// to ErrOverBudget. Disabled/missing rows also return false.
	ConsumeBudget(ctx context.Context, tenantID, integrationID uuid.UUID) (bool, error)

	CreateToken(ctx context.Context, tenantID, integrationID uuid.UUID, tokenSHA256 string) (Token, error)
	GetActiveTokenBySHA(ctx context.Context, tokenSHA256 string) (Token, error)
	ListTokens(ctx context.Context, tenantID, integrationID uuid.UUID) ([]Token, error)
	TouchTokenLastUsed(ctx context.Context, tokenID uuid.UUID) error
	RevokeToken(ctx context.Context, tenantID, integrationID, tokenID uuid.UUID) (Token, error)
}
