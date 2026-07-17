package feishu

import (
	"context"
	"errors"

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

func (r *PgRepository) UpsertAppConfig(ctx context.Context, tenantID uuid.UUID, appID, secretSealed, status string) (AppConfig, error) {
	row, err := r.q.UpsertFeishuAppConfig(ctx, queries.UpsertFeishuAppConfigParams{
		TenantID:        tenantID,
		AppID:           appID,
		AppSecretSealed: secretSealed,
		Status:          pgtype.Text{String: status, Valid: status != ""},
	})
	if err != nil {
		return AppConfig{}, err
	}
	return appConfigFromRow(row), nil
}

func (r *PgRepository) ListActiveAppConfigs(ctx context.Context, tenantID uuid.UUID) ([]AppConfig, error) {
	rows, err := r.q.ListActiveFeishuAppConfigs(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	configs := make([]AppConfig, 0, len(rows))
	for _, row := range rows {
		configs = append(configs, appConfigFromRow(row))
	}
	return configs, nil
}

func (r *PgRepository) GetAppConfig(ctx context.Context, tenantID, id uuid.UUID) (AppConfig, error) {
	row, err := r.q.GetFeishuAppConfig(ctx, queries.GetFeishuAppConfigParams{TenantID: tenantID, ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AppConfig{}, ErrAppConfigNotFound
		}
		return AppConfig{}, err
	}
	return appConfigFromRow(row), nil
}

func (r *PgRepository) CreateIdentity(ctx context.Context, identity Identity) (Identity, error) {
	row, err := r.q.CreateFeishuIdentity(ctx, queries.CreateFeishuIdentityParams{
		TenantID:          identity.TenantID,
		AuthUserID:        identity.AuthUserID,
		FeishuAppConfigID: identity.FeishuAppConfigID,
		OpenID:            identity.OpenID,
		UnionID:           textOrNull(identity.UnionID),
		BoundVia:          identity.BoundVia,
	})
	if err != nil {
		return Identity{}, err
	}
	return identityFromRow(row), nil
}

func (r *PgRepository) GetIdentityByOpenID(ctx context.Context, appConfigID uuid.UUID, openID string) (Identity, error) {
	row, err := r.q.GetFeishuIdentityByOpenID(ctx, queries.GetFeishuIdentityByOpenIDParams{
		FeishuAppConfigID: appConfigID,
		OpenID:            openID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Identity{}, ErrIdentityNotFound
		}
		return Identity{}, err
	}
	return identityFromRow(row), nil
}

func (r *PgRepository) GetIdentityByUser(ctx context.Context, appConfigID, authUserID uuid.UUID) (Identity, error) {
	row, err := r.q.GetFeishuIdentityByUser(ctx, queries.GetFeishuIdentityByUserParams{
		FeishuAppConfigID: appConfigID,
		AuthUserID:        authUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Identity{}, ErrIdentityNotFound
		}
		return Identity{}, err
	}
	return identityFromRow(row), nil
}

func (r *PgRepository) ListIdentitiesByUsers(ctx context.Context, tenantID uuid.UUID, userIDs []uuid.UUID) ([]Identity, error) {
	rows, err := r.q.ListFeishuIdentitiesByUsers(ctx, queries.ListFeishuIdentitiesByUsersParams{
		TenantID:    tenantID,
		AuthUserIds: userIDs,
	})
	if err != nil {
		return nil, err
	}
	identities := make([]Identity, 0, len(rows))
	for _, row := range rows {
		identities = append(identities, identityFromRow(row))
	}
	return identities, nil
}

func (r *PgRepository) DeleteIdentityByUser(ctx context.Context, tenantID, appConfigID, authUserID uuid.UUID) error {
	return r.q.DeleteFeishuIdentityByUser(ctx, queries.DeleteFeishuIdentityByUserParams{
		TenantID:          tenantID,
		FeishuAppConfigID: appConfigID,
		AuthUserID:        authUserID,
	})
}

func appConfigFromRow(row queries.FeishuAppConfig) AppConfig {
	return AppConfig{
		ID:              row.ID,
		TenantID:        row.TenantID,
		AppID:           row.AppID,
		AppSecretSealed: row.AppSecretSealed,
		Status:          row.Status,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func identityFromRow(row queries.UserFeishuIdentity) Identity {
	identity := Identity{
		ID:                row.ID,
		TenantID:          row.TenantID,
		AuthUserID:        row.AuthUserID,
		FeishuAppConfigID: row.FeishuAppConfigID,
		OpenID:            row.OpenID,
		BoundVia:          row.BoundVia,
		CreatedAt:         row.CreatedAt.Time,
	}
	if row.UnionID.Valid {
		unionID := row.UnionID.String
		identity.UnionID = &unionID
	}
	return identity
}

func textOrNull(value *string) pgtype.Text {
	if value == nil || *value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
