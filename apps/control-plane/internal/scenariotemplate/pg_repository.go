package scenariotemplate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (r *PgRepository) CreateScenarioTemplate(ctx context.Context, params CreateScenarioTemplateParams) (ScenarioTemplate, error) {
	specJSON, err := json.Marshal(params.Spec)
	if err != nil {
		return ScenarioTemplate{}, fmt.Errorf("marshal spec: %w", err)
	}
	row, err := r.q.CreateScenarioTemplate(ctx, queries.CreateScenarioTemplateParams{
		TenantID:    params.TenantID,
		TemplateKey: params.Key,
		Name:        params.Name,
		Description: params.Description,
		Spec:        specJSON,
		CreatedBy:   nullUUIDFromPtr(params.CreatedBy),
	})
	if err != nil {
		return ScenarioTemplate{}, mapConstraintError(err)
	}
	return scenarioTemplateFromRow(row), nil
}

func (r *PgRepository) CreateScenarioTemplateVersion(ctx context.Context, params CreateScenarioTemplateVersionParams) (ScenarioTemplateVersion, error) {
	specJSON, err := json.Marshal(params.Spec)
	if err != nil {
		return ScenarioTemplateVersion{}, fmt.Errorf("marshal spec: %w", err)
	}
	row, err := r.q.CreateScenarioTemplateVersion(ctx, queries.CreateScenarioTemplateVersionParams{
		TenantID:   params.TenantID,
		TemplateID: params.TemplateID,
		Version:    int32(params.Version),
		Spec:       specJSON,
		CreatedBy:  nullUUIDFromPtr(params.CreatedBy),
	})
	if err != nil {
		return ScenarioTemplateVersion{}, err
	}
	return scenarioTemplateVersionFromRow(row), nil
}

func (r *PgRepository) UpdateScenarioTemplateActiveSpec(ctx context.Context, params UpdateScenarioTemplateActiveSpecParams) (ScenarioTemplate, error) {
	specJSON, err := json.Marshal(params.Spec)
	if err != nil {
		return ScenarioTemplate{}, fmt.Errorf("marshal spec: %w", err)
	}
	row, err := r.q.UpdateScenarioTemplateActiveSpec(ctx, queries.UpdateScenarioTemplateActiveSpecParams{
		TenantID:      params.TenantID,
		ID:            params.TemplateID,
		Spec:          specJSON,
		ActiveVersion: int32(params.ActiveVersion),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScenarioTemplate{}, ErrScenarioTemplateNotFound
		}
		return ScenarioTemplate{}, err
	}
	return scenarioTemplateFromRow(row), nil
}

func (r *PgRepository) UpdateScenarioTemplateStatus(ctx context.Context, params UpdateScenarioTemplateStatusParams) (ScenarioTemplate, error) {
	row, err := r.q.UpdateScenarioTemplateStatus(ctx, queries.UpdateScenarioTemplateStatusParams{
		TenantID:    params.TenantID,
		ID:          params.TemplateID,
		Status:      params.Status,
		Name:        params.Name,
		Description: params.Description,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScenarioTemplate{}, ErrScenarioTemplateNotFound
		}
		return ScenarioTemplate{}, err
	}
	return scenarioTemplateFromRow(row), nil
}

func (r *PgRepository) ListScenarioTemplateVersions(ctx context.Context, tenantID, templateID uuid.UUID) ([]ScenarioTemplateVersion, error) {
	rows, err := r.q.ListScenarioTemplateVersions(ctx, queries.ListScenarioTemplateVersionsParams{
		TenantID:   tenantID,
		TemplateID: templateID,
	})
	if err != nil {
		return nil, err
	}
	versions := make([]ScenarioTemplateVersion, 0, len(rows))
	for _, row := range rows {
		versions = append(versions, scenarioTemplateVersionFromRow(row))
	}
	return versions, nil
}

func scenarioTemplateVersionFromRow(row queries.ScenarioTemplateVersion) ScenarioTemplateVersion {
	spec := map[string]any{}
	if len(row.Spec) > 0 {
		_ = json.Unmarshal(row.Spec, &spec)
	}
	return ScenarioTemplateVersion{
		ID:         row.ID,
		TenantID:   row.TenantID,
		TemplateID: row.TemplateID,
		Version:    int(row.Version),
		Spec:       spec,
		CreatedBy:  uuidPtrFromNull(row.CreatedBy),
		CreatedAt:  row.CreatedAt.Time,
	}
}

func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid || value.UUID == uuid.Nil {
		return nil
	}
	copied := value.UUID
	return &copied
}

// mapConstraintError maps a Postgres unique-violation on
// uq_scenario_templates_tenant_key_active to ErrConflict. The service layer
// already pre-checks for an existing key; this is a race-condition backstop.
func mapConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: template key already exists", ErrConflict)
	}
	return err
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
