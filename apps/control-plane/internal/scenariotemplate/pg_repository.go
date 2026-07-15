package scenariotemplate

import (
	"context"
	"encoding/json"
	"errors"

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

func (r *PgRepository) ListScenarioTemplates(ctx context.Context, tenantID uuid.UUID) ([]ScenarioTemplate, error) {
	rows, err := r.q.ListScenarioTemplates(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	templates := make([]ScenarioTemplate, 0, len(rows))
	for _, row := range rows {
		templates = append(templates, scenarioTemplateFromRow(row))
	}
	return templates, nil
}

func (r *PgRepository) GetScenarioTemplateByKey(ctx context.Context, tenantID uuid.UUID, key string) (ScenarioTemplate, error) {
	row, err := r.q.GetScenarioTemplateByKey(ctx, queries.GetScenarioTemplateByKeyParams{
		TenantID:    tenantID,
		TemplateKey: key,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScenarioTemplate{}, ErrScenarioTemplateNotFound
		}
		return ScenarioTemplate{}, err
	}
	return scenarioTemplateFromRow(row), nil
}

func scenarioTemplateFromRow(row queries.ScenarioTemplate) ScenarioTemplate {
	spec := map[string]any{}
	if len(row.Spec) > 0 {
		// A malformed spec degrades to an empty object rather than failing the
		// read: the registry row stays listable and the defect stays visible.
		_ = json.Unmarshal(row.Spec, &spec)
	}
	return ScenarioTemplate{
		ID:            row.ID,
		TenantID:      row.TenantID,
		Key:           row.TemplateKey,
		Name:          row.Name,
		Description:   row.Description,
		Spec:          spec,
		Status:        row.Status,
		ActiveVersion: int(row.ActiveVersion),
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}
