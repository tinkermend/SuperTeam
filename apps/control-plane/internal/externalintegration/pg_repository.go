package externalintegration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

func (r *PgRepository) CreateIntegration(ctx context.Context, integration Integration) (Integration, error) {
	skillIDs, err := marshalSkillIDs(integration.SkillIDs)
	if err != nil {
		return Integration{}, err
	}
	row, err := r.q.CreateExternalIntegration(ctx, queries.CreateExternalIntegrationParams{
		TenantID:            integration.TenantID,
		ProjectID:           integration.ProjectID,
		DigitalEmployeeID:   integration.DigitalEmployeeID,
		Name:                integration.Name,
		Description:         integration.Description,
		AllowChatRun:        integration.AllowChatRun,
		AllowDemandSubmit:   integration.AllowDemandSubmit,
		SkillIds:            skillIDs,
		ScenarioTemplateKey: textPtr(integration.ScenarioTemplateKey),
		AutonomyTier:        integration.AutonomyTier,
		PinnedExitDeliverable: textPtr(integration.PinnedExitDeliverable),
		AcknowledgeExitTierSemantics: integration.AcknowledgeExitTierSemantics,
		MaxCallsPerHour:     integration.MaxCallsPerHour,
		Status:              integration.Status,
		CreatedByUserID:     integration.CreatedByUserID,
	})
	if err != nil {
		return Integration{}, fmt.Errorf("create external integration: %w", err)
	}
	return integrationFromRow(row)
}

func (r *PgRepository) GetIntegration(ctx context.Context, tenantID, integrationID uuid.UUID) (Integration, error) {
	row, err := r.q.GetExternalIntegration(ctx, queries.GetExternalIntegrationParams{
		TenantID: tenantID,
		ID:       integrationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Integration{}, ErrNotFound
		}
		return Integration{}, fmt.Errorf("get external integration: %w", err)
	}
	return integrationFromRow(row)
}

func (r *PgRepository) ListIntegrations(ctx context.Context, tenantID uuid.UUID, projectID *uuid.UUID) ([]Integration, error) {
	filter := uuid.NullUUID{}
	if projectID != nil {
		filter = uuid.NullUUID{UUID: *projectID, Valid: true}
	}
	rows, err := r.q.ListExternalIntegrationsByTenant(ctx, queries.ListExternalIntegrationsByTenantParams{
		TenantID:  tenantID,
		ProjectID: filter,
	})
	if err != nil {
		return nil, fmt.Errorf("list external integrations: %w", err)
	}
	out := make([]Integration, 0, len(rows))
	for _, row := range rows {
		integration, err := integrationFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, integration)
	}
	return out, nil
}

func (r *PgRepository) UpdateIntegration(ctx context.Context, integration Integration) (Integration, error) {
	skillIDs, err := marshalSkillIDs(integration.SkillIDs)
	if err != nil {
		return Integration{}, err
	}
	row, err := r.q.UpdateExternalIntegration(ctx, queries.UpdateExternalIntegrationParams{
		Name:                integration.Name,
		Description:         integration.Description,
		AllowChatRun:        integration.AllowChatRun,
		AllowDemandSubmit:   integration.AllowDemandSubmit,
		SkillIds:            skillIDs,
		ScenarioTemplateKey: textPtr(integration.ScenarioTemplateKey),
		AutonomyTier:        integration.AutonomyTier,
		PinnedExitDeliverable: textPtr(integration.PinnedExitDeliverable),
		AcknowledgeExitTierSemantics: integration.AcknowledgeExitTierSemantics,
		MaxCallsPerHour:     integration.MaxCallsPerHour,
		Status:              integration.Status,
		TenantID:            integration.TenantID,
		ID:                  integration.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Integration{}, ErrNotFound
		}
		return Integration{}, fmt.Errorf("update external integration: %w", err)
	}
	return integrationFromRow(row)
}

func (r *PgRepository) ConsumeBudget(ctx context.Context, tenantID, integrationID uuid.UUID) (bool, error) {
	_, err := r.q.ConsumeExternalIntegrationBudget(ctx, queries.ConsumeExternalIntegrationBudgetParams{
		TenantID: tenantID,
		ID:       integrationID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("consume integration budget: %w", err)
	}
	return true, nil
}

func (r *PgRepository) CreateToken(ctx context.Context, tenantID, integrationID uuid.UUID, tokenSHA256 string) (Token, error) {
	row, err := r.q.CreateExternalIntegrationToken(ctx, queries.CreateExternalIntegrationTokenParams{
		TenantID:      tenantID,
		IntegrationID: integrationID,
		TokenSha256:   tokenSHA256,
	})
	if err != nil {
		return Token{}, fmt.Errorf("create integration token: %w", err)
	}
	return tokenFromParts(row.ID, row.TenantID, row.IntegrationID, row.Status, row.CreatedAt, row.LastUsedAt, row.RevokedAt), nil
}

func (r *PgRepository) GetActiveTokenBySHA(ctx context.Context, tokenSHA256 string) (Token, error) {
	row, err := r.q.GetActiveExternalIntegrationTokenBySHA(ctx, tokenSHA256)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Token{}, ErrUnauthorized
		}
		return Token{}, fmt.Errorf("lookup integration token: %w", err)
	}
	return tokenFromParts(row.ID, row.TenantID, row.IntegrationID, row.Status, row.CreatedAt, row.LastUsedAt, row.RevokedAt), nil
}

func (r *PgRepository) ListTokens(ctx context.Context, tenantID, integrationID uuid.UUID) ([]Token, error) {
	rows, err := r.q.ListExternalIntegrationTokens(ctx, queries.ListExternalIntegrationTokensParams{
		TenantID:      tenantID,
		IntegrationID: integrationID,
	})
	if err != nil {
		return nil, fmt.Errorf("list integration tokens: %w", err)
	}
	out := make([]Token, 0, len(rows))
	for _, row := range rows {
		out = append(out, tokenFromParts(row.ID, row.TenantID, row.IntegrationID, row.Status, row.CreatedAt, row.LastUsedAt, row.RevokedAt))
	}
	return out, nil
}

func (r *PgRepository) TouchTokenLastUsed(ctx context.Context, tokenID uuid.UUID) error {
	return r.q.TouchExternalIntegrationTokenLastUsed(ctx, tokenID)
}

func (r *PgRepository) RevokeToken(ctx context.Context, tenantID, integrationID, tokenID uuid.UUID) (Token, error) {
	row, err := r.q.RevokeExternalIntegrationToken(ctx, queries.RevokeExternalIntegrationTokenParams{
		TenantID:      tenantID,
		IntegrationID: integrationID,
		ID:            tokenID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Token{}, ErrNotFound
		}
		return Token{}, fmt.Errorf("revoke integration token: %w", err)
	}
	return tokenFromParts(row.ID, row.TenantID, row.IntegrationID, row.Status, row.CreatedAt, row.LastUsedAt, row.RevokedAt), nil
}

func integrationFromRow(row queries.ExternalIntegration) (Integration, error) {
	skillIDs, err := unmarshalSkillIDs(row.SkillIds)
	if err != nil {
		return Integration{}, fmt.Errorf("integration %s skill_ids: %w", row.ID, err)
	}
	integration := Integration{
		ID:                row.ID,
		TenantID:          row.TenantID,
		ProjectID:         row.ProjectID,
		DigitalEmployeeID: row.DigitalEmployeeID,
		Name:              row.Name,
		Description:       row.Description,
		AllowChatRun:      row.AllowChatRun,
		AllowDemandSubmit: row.AllowDemandSubmit,
		SkillIDs:          skillIDs,
		AutonomyTier:      row.AutonomyTier,
		AcknowledgeExitTierSemantics: row.AcknowledgeExitTierSemantics,
		MaxCallsPerHour:   row.MaxCallsPerHour,
		Status:            row.Status,
		CreatedByUserID:   row.CreatedByUserID,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}
	if row.ScenarioTemplateKey.Valid {
		key := row.ScenarioTemplateKey.String
		integration.ScenarioTemplateKey = &key
	}
	if row.PinnedExitDeliverable.Valid {
		pin := row.PinnedExitDeliverable.String
		integration.PinnedExitDeliverable = &pin
	}
	return integration, nil
}

func tokenFromParts(id, tenantID, integrationID uuid.UUID, status string, createdAt, lastUsedAt, revokedAt pgtype.Timestamptz) Token {
	token := Token{
		ID:            id,
		TenantID:      tenantID,
		IntegrationID: integrationID,
		Status:        status,
		CreatedAt:     createdAt.Time,
	}
	if lastUsedAt.Valid {
		t := lastUsedAt.Time
		token.LastUsedAt = &t
	}
	if revokedAt.Valid {
		t := revokedAt.Time
		token.RevokedAt = &t
	}
	return token
}

func marshalSkillIDs(ids []uuid.UUID) ([]byte, error) {
	if ids == nil {
		ids = []uuid.UUID{}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("marshal skill_ids: %w", err)
	}
	return raw, nil
}

func unmarshalSkillIDs(raw []byte) ([]uuid.UUID, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var ids []uuid.UUID
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func textPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: trimmed, Valid: true}
}
