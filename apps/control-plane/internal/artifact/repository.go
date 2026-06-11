package artifact

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateRetentionHold(ctx context.Context, req CreateRetentionHoldRequest) (RetentionHold, error)
	CountActiveRetentionHolds(ctx context.Context, tenantID, artifactID uuid.UUID) (int32, error)
}

type CreateRetentionHoldRequest struct {
	TenantID       uuid.UUID
	ArtifactID     uuid.UUID
	HoldType       string
	ResourceType   string
	ResourceID     uuid.UUID
	Reason         string
	CreatedEventID *uuid.UUID
}

type RetentionHold struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	ArtifactID uuid.UUID
	Status     string
}
