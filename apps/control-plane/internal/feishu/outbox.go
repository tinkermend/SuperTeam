package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/superteam/control-plane/internal/storage/queries"
)

// OutboxMaxAttempts 投递失败重试上限:第 3 次失败后标 failed 终态,不再消费。
const OutboxMaxAttempts = 3

var ErrOutboxNotFound = errors.New("feishu outbox item not found or not pending")

type OutboxItem struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	ProjectID       *uuid.UUID
	Kind            string
	ResourceType    string
	ResourceID      uuid.UUID
	RecipientUserID uuid.UUID
	RecipientOpenID string
	Payload         map[string]any
	Status          string
	Attempts        int32
	FeishuMessageID *string
	CreatedAt       time.Time
}

type OutboxRepository interface {
	ListPendingOutbox(ctx context.Context, tenantID uuid.UUID, limit int32) ([]OutboxItem, error)
	MarkOutboxSent(ctx context.Context, tenantID, id uuid.UUID, feishuMessageID string) (OutboxItem, error)
	MarkOutboxFailed(ctx context.Context, tenantID, id uuid.UUID, reason string) (OutboxItem, error)
}

func (r *PgRepository) ListPendingOutbox(ctx context.Context, tenantID uuid.UUID, limit int32) ([]OutboxItem, error) {
	rows, err := r.q.ListPendingFeishuOutbox(ctx, queries.ListPendingFeishuOutboxParams{
		TenantID: tenantID,
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]OutboxItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, outboxItemFromRow(row))
	}
	return items, nil
}

func (r *PgRepository) MarkOutboxSent(ctx context.Context, tenantID, id uuid.UUID, feishuMessageID string) (OutboxItem, error) {
	row, err := r.q.MarkFeishuOutboxSent(ctx, queries.MarkFeishuOutboxSentParams{
		TenantID:        tenantID,
		ID:              id,
		FeishuMessageID: pgtype.Text{String: feishuMessageID, Valid: feishuMessageID != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OutboxItem{}, ErrOutboxNotFound
		}
		return OutboxItem{}, err
	}
	return outboxItemFromRow(row), nil
}

func (r *PgRepository) MarkOutboxFailed(ctx context.Context, tenantID, id uuid.UUID, reason string) (OutboxItem, error) {
	row, err := r.q.MarkFeishuOutboxFailed(ctx, queries.MarkFeishuOutboxFailedParams{
		TenantID:    tenantID,
		ID:          id,
		LastError:   reason,
		MaxAttempts: OutboxMaxAttempts,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OutboxItem{}, ErrOutboxNotFound
		}
		return OutboxItem{}, err
	}
	return outboxItemFromRow(row), nil
}

func outboxItemFromRow(row queries.FeishuOutbox) OutboxItem {
	item := OutboxItem{
		ID:              row.ID,
		TenantID:        row.TenantID,
		Kind:            row.Kind,
		ResourceType:    row.ResourceType,
		ResourceID:      row.ResourceID,
		RecipientUserID: row.RecipientUserID,
		RecipientOpenID: row.RecipientOpenID,
		Status:          row.Status,
		Attempts:        row.Attempts,
		CreatedAt:       row.CreatedAt.Time,
	}
	if row.ProjectID.Valid {
		projectID := row.ProjectID.UUID
		item.ProjectID = &projectID
	}
	if row.FeishuMessageID.Valid {
		messageID := row.FeishuMessageID.String
		item.FeishuMessageID = &messageID
	}
	if len(row.Payload) > 0 {
		payload := map[string]any{}
		if err := json.Unmarshal(row.Payload, &payload); err == nil {
			item.Payload = payload
		}
	}
	return item
}
