package artifact

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

const activeRetentionHoldStatus = "active"

type pgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &pgRepository{q: q}
}

func (r *pgRepository) CreateRetentionHold(ctx context.Context, req CreateRetentionHoldRequest) (RetentionHold, error) {
	row, err := r.q.CreateArtifactRetentionHold(ctx, queries.CreateArtifactRetentionHoldParams{
		TenantID:       req.TenantID,
		ArtifactID:     req.ArtifactID,
		HoldType:       req.HoldType,
		ResourceType:   req.ResourceType,
		ResourceID:     req.ResourceID,
		Reason:         textOrNull(req.Reason),
		Status:         activeRetentionHoldStatus,
		CreatedEventID: nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return RetentionHold{}, err
	}
	return RetentionHold{
		ID:         row.ID,
		TenantID:   row.TenantID,
		ArtifactID: row.ArtifactID,
		Status:     row.Status,
	}, nil
}

func (r *pgRepository) CountActiveRetentionHolds(ctx context.Context, tenantID, artifactID uuid.UUID) (int32, error) {
	return r.q.CountActiveArtifactRetentionHolds(ctx, queries.CountActiveArtifactRetentionHoldsParams{
		TenantID:   tenantID,
		ArtifactID: artifactID,
	})
}

func textOrNull(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func nullUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}
