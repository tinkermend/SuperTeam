package prompttemplate

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/superteam/control-plane/internal/storage/queries"
)

type pgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &pgRepository{q: q}
}

func (r *pgRepository) List(ctx context.Context, tenantID uuid.UUID, teamIDs []uuid.UUID, userID uuid.UUID) ([]PromptTemplate, error) {
	dbTemplates, err := r.q.ListPromptTemplates(ctx, queries.ListPromptTemplatesParams{
		TenantID: tenantID,
		TeamIds:  teamIDs,
		UserID:   userID,
	})
	if err != nil {
		return nil, err
	}

	templates := make([]PromptTemplate, len(dbTemplates))
	for i, dbT := range dbTemplates {
		t, err := mapToDomain(dbT)
		if err != nil {
			return nil, err
		}
		templates[i] = t
	}
	return templates, nil
}

func (r *pgRepository) Create(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error) {
	varsJSON, err := json.Marshal(input.Variables)
	if err != nil {
		return PromptTemplate{}, err
	}

	var catCode pgtype.Text
	if input.CategoryCode != "" {
		catCode = pgtype.Text{String: input.CategoryCode, Valid: true}
	} else {
		// Even if empty, we can just omit it or pass empty since DB does COALESCE if null,
		// but since input.CategoryCode might be deliberately empty, we can just pass not valid for default.
		catCode = pgtype.Text{Valid: false}
	}

	var teamID uuid.NullUUID
	if input.TeamID != nil {
		teamID = uuid.NullUUID{UUID: *input.TeamID, Valid: true}
	}

	dbT, err := r.q.CreatePromptTemplate(ctx, queries.CreatePromptTemplateParams{
		TenantID:     input.TenantID,
		Title:        input.Title,
		Content:      input.Content,
		CategoryCode: catCode,
		Scope:        input.Scope,
		TeamID:       teamID,
		CreatorID:    input.CreatorID,
		Variables:    varsJSON,
	})
	if err != nil {
		return PromptTemplate{}, err
	}

	return mapToDomain(dbT)
}

func (r *pgRepository) IncrementUseCount(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	return r.q.IncrementPromptTemplateUseCount(ctx, queries.IncrementPromptTemplateUseCountParams{
		ID:       id,
		TenantID: tenantID,
	})
}

func mapToDomain(dbT queries.TaskPromptTemplate) (PromptTemplate, error) {
	var vars []PromptTemplateVariable
	if len(dbT.Variables) > 0 {
		if err := json.Unmarshal(dbT.Variables, &vars); err != nil {
			return PromptTemplate{}, err
		}
	} else {
		vars = []PromptTemplateVariable{}
	}

	var teamID *uuid.UUID
	if dbT.TeamID.Valid {
		teamID = &dbT.TeamID.UUID
	}

	return PromptTemplate{
		ID:           dbT.ID,
		TenantID:     dbT.TenantID,
		Title:        dbT.Title,
		Content:      dbT.Content,
		CategoryCode: dbT.CategoryCode,
		Scope:        dbT.Scope,
		TeamID:       teamID,
		CreatorID:    dbT.CreatorID,
		Variables:    vars,
		UseCount:     dbT.UseCount,
		CreatedAt:    dbT.CreatedAt.Time,
		UpdatedAt:    dbT.UpdatedAt.Time,
	}, nil
}
