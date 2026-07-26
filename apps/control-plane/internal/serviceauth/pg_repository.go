package serviceauth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) *PgRepository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateServiceToken(ctx context.Context, tenantID uuid.UUID, serviceName, tokenHash string) (ServiceToken, error) {
	row, err := r.q.CreateServiceToken(ctx, queries.CreateServiceTokenParams{
		TenantID:    tenantID,
		ServiceName: serviceName,
		TokenHash:   tokenHash,
	})
	if err != nil {
		return ServiceToken{}, err
	}
	return serviceTokenFromRow(row), nil
}

func (r *PgRepository) ListActiveServiceTokensByName(ctx context.Context, serviceName string) ([]ServiceToken, error) {
	rows, err := r.q.ListActiveServiceTokensByName(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	tokens := make([]ServiceToken, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, serviceTokenFromRow(row))
	}
	return tokens, nil
}

func (r *PgRepository) ListServiceTokensByTenant(ctx context.Context, tenantID uuid.UUID) ([]ServiceToken, error) {
	rows, err := r.q.ListServiceTokensByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	tokens := make([]ServiceToken, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, serviceTokenFromRow(row))
	}
	return tokens, nil
}

func (r *PgRepository) TouchServiceTokenLastUsed(ctx context.Context, id uuid.UUID) error {
	return r.q.TouchServiceTokenLastUsed(ctx, id)
}

func (r *PgRepository) RevokeServiceToken(ctx context.Context, tenantID, id uuid.UUID) (ServiceToken, error) {
	row, err := r.q.RevokeServiceToken(ctx, queries.RevokeServiceTokenParams{TenantID: tenantID, ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceToken{}, ErrTokenNotFound
		}
		return ServiceToken{}, err
	}
	return serviceTokenFromRow(row), nil
}

func serviceTokenFromRow(row queries.AuthServiceToken) ServiceToken {
	return ServiceToken{
		ID:          row.ID,
		TenantID:    row.TenantID,
		ServiceName: row.ServiceName,
		TokenHash:   row.TokenHash,
		Status:      row.Status,
		LastUsedAt:  timePtr(row.LastUsedAt),
		CreatedAt:   row.CreatedAt.Time,
		RevokedAt:   timePtr(row.RevokedAt),
	}
}

func timePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}
