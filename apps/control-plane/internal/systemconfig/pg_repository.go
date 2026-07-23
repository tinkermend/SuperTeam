package systemconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) ListOverrides(ctx context.Context, tenantID uuid.UUID) ([]Override, error) {
	rows, err := r.q.ListSystemConfigOverrides(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Override, 0, len(rows))
	for _, row := range rows {
		o, err := decodeOverride(row.ConfigKey, row.Value)
		if err != nil {
			return nil, fmt.Errorf("decode override %s: %w", row.ConfigKey, err)
		}
		o.UpdatedAt = row.UpdatedAt.Time
		if row.UpdatedBy.Valid {
			updatedBy := row.UpdatedBy.UUID
			o.UpdatedBy = &updatedBy
		}
		o.UpdatedByName = strings.TrimSpace(row.UpdatedByDisplayName.String)
		if o.UpdatedByName == "" {
			o.UpdatedByName = strings.TrimSpace(row.UpdatedByUsername.String)
		}
		out = append(out, o)
	}
	return out, nil
}

func (r *PgRepository) GetOverride(ctx context.Context, tenantID uuid.UUID, key string) (*Override, error) {
	row, err := r.q.GetSystemConfigOverride(ctx, queries.GetSystemConfigOverrideParams{
		TenantID:  tenantID,
		ConfigKey: key,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	o, err := decodeOverride(row.ConfigKey, row.Value)
	if err != nil {
		return nil, fmt.Errorf("decode override %s: %w", key, err)
	}
	o.UpdatedAt = row.UpdatedAt.Time
	if row.UpdatedBy.Valid {
		updatedBy := row.UpdatedBy.UUID
		o.UpdatedBy = &updatedBy
	}
	return &o, nil
}

func (r *PgRepository) UpsertOverride(ctx context.Context, tenantID uuid.UUID, key string, value any, updatedBy uuid.UUID) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = r.q.UpsertSystemConfigOverride(ctx, queries.UpsertSystemConfigOverrideParams{
		TenantID:  tenantID,
		ConfigKey: key,
		Value:     encoded,
		UpdatedBy: uuid.NullUUID{UUID: updatedBy, Valid: updatedBy != uuid.Nil},
	})
	return err
}

func (r *PgRepository) DeleteOverride(ctx context.Context, tenantID uuid.UUID, key string) (bool, error) {
	affected, err := r.q.DeleteSystemConfigOverride(ctx, queries.DeleteSystemConfigOverrideParams{
		TenantID:  tenantID,
		ConfigKey: key,
	})
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// decodeOverride 按 JSONB 实际类型解码:number → Value,string → StringValue。
// 定义类型由服务层在读路径对齐;存储层只负责忠实还原。
func decodeOverride(key string, raw []byte) (Override, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Override{}, err
	}
	o := Override{ConfigKey: key}
	switch v := decoded.(type) {
	case float64:
		o.Value = int64(v)
	case string:
		o.StringValue = v
	default:
		return Override{}, fmt.Errorf("unsupported JSONB value type %T", decoded)
	}
	return o, nil
}
