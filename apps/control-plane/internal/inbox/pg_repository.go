package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) UpsertItem(ctx context.Context, req UpsertItemRequest) (Item, error) {
	params, err := upsertParams(req)
	if err != nil {
		return Item{}, err
	}
	row, err := r.q.UpsertInboxItem(ctx, params)
	if err != nil {
		return Item{}, err
	}
	return itemFromRecord(row)
}

func (r *PgRepository) UpsertItemByApprovalSource(ctx context.Context, req UpsertItemRequest) (Item, error) {
	params, err := upsertApprovalParams(req)
	if err != nil {
		return Item{}, err
	}
	row, err := r.q.UpsertInboxItemByApprovalSource(ctx, params)
	if err != nil {
		return Item{}, err
	}
	return itemFromRecord(row)
}

func (r *PgRepository) GetItem(ctx context.Context, tenantID, itemID uuid.UUID) (Item, error) {
	row, err := r.q.GetInboxItem(ctx, queries.GetInboxItemParams{TenantID: tenantID, ID: itemID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Item{}, ErrItemNotFound
		}
		return Item{}, err
	}
	return itemFromRecord(row)
}

func (r *PgRepository) ListItems(ctx context.Context, req ListItemsRequest) ([]Item, error) {
	rows, err := r.q.ListInboxItems(ctx, queries.ListInboxItemsParams{
		TenantID:        req.TenantID,
		Status:          string(req.Status),
		TargetUserID:    nullUUID(req.TargetUserID),
		ItemType:        textFromItemType(req.ItemType),
		RiskLevel:       textFromStringPtr(req.RiskLevel),
		SourceProjectID: nullUUID(req.ProjectID),
		Offset:          req.Offset,
		Limit:           req.Limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		item, err := itemFromRecord(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PgRepository) CountOpenItems(ctx context.Context, tenantID uuid.UUID, targetUserID *uuid.UUID) (int64, error) {
	return r.q.CountInboxItems(ctx, queries.CountInboxItemsParams{
		TenantID:        tenantID,
		Status:          string(StatusOpen),
		TargetUserID:    nullUUID(targetUserID),
		ItemType:        pgtype.Text{},
		RiskLevel:       pgtype.Text{},
		SourceProjectID: uuid.NullUUID{},
	})
}

func (r *PgRepository) CountHighRiskOpenItems(ctx context.Context, tenantID uuid.UUID, targetUserID *uuid.UUID) (int64, error) {
	return r.q.CountHighRiskInboxItems(ctx, queries.CountHighRiskInboxItemsParams{
		TenantID:     tenantID,
		TargetUserID: nullUUID(targetUserID),
	})
}

func upsertParams(req UpsertItemRequest) (queries.UpsertInboxItemParams, error) {
	actionSchema, contextPayload, deepLink, err := marshalItemJSON(req)
	if err != nil {
		return queries.UpsertInboxItemParams{}, err
	}
	return queries.UpsertInboxItemParams{
		TenantID:                req.TenantID,
		TeamID:                  nullUUID(req.TeamID),
		TargetUserID:            req.TargetUserID,
		Scope:                   req.Scope,
		ItemType:                string(req.ItemType),
		SourceType:              string(req.SourceType),
		SourceID:                req.SourceID,
		SourceProjectID:         nullUUID(req.SourceProjectID),
		SourceTaskID:            nullUUID(req.SourceTaskID),
		SourceApprovalRequestID: nullUUID(req.SourceApprovalRequestID),
		Title:                   req.Title,
		Summary:                 textFromString(req.Summary),
		RiskLevel:               textFromString(req.RiskLevel),
		Priority:                textFromString(req.Priority),
		Status:                  string(req.Status),
		ActionSchema:            actionSchema,
		ContextPayload:          contextPayload,
		DeepLink:                deepLink,
		ResolvedAt:              timestamptzFromPtr(req.ResolvedAt),
		LastActivityAt:          timestamptz(req.LastActivityAt),
	}, nil
}

func upsertApprovalParams(req UpsertItemRequest) (queries.UpsertInboxItemByApprovalSourceParams, error) {
	if req.SourceApprovalRequestID == nil || *req.SourceApprovalRequestID == uuid.Nil {
		return queries.UpsertInboxItemByApprovalSourceParams{}, ErrInvalidItem
	}
	actionSchema, contextPayload, deepLink, err := marshalItemJSON(req)
	if err != nil {
		return queries.UpsertInboxItemByApprovalSourceParams{}, err
	}
	return queries.UpsertInboxItemByApprovalSourceParams{
		TenantID:                req.TenantID,
		TeamID:                  nullUUID(req.TeamID),
		TargetUserID:            req.TargetUserID,
		Scope:                   req.Scope,
		ItemType:                string(req.ItemType),
		SourceType:              string(req.SourceType),
		SourceID:                req.SourceID,
		SourceProjectID:         nullUUID(req.SourceProjectID),
		SourceTaskID:            nullUUID(req.SourceTaskID),
		SourceApprovalRequestID: *req.SourceApprovalRequestID,
		Title:                   req.Title,
		Summary:                 textFromString(req.Summary),
		RiskLevel:               textFromString(req.RiskLevel),
		Priority:                textFromString(req.Priority),
		Status:                  string(req.Status),
		ActionSchema:            actionSchema,
		ContextPayload:          contextPayload,
		DeepLink:                deepLink,
		ResolvedAt:              timestamptzFromPtr(req.ResolvedAt),
		LastActivityAt:          timestamptz(req.LastActivityAt),
	}, nil
}

func itemFromRecord(row queries.InboxItem) (Item, error) {
	actions, err := actionsFromJSON(row.ActionSchema)
	if err != nil {
		return Item{}, fmt.Errorf("action_schema: %w", err)
	}
	contextPayload, err := mapFromJSON(row.ContextPayload)
	if err != nil {
		return Item{}, fmt.Errorf("context_payload: %w", err)
	}
	deepLink, err := mapFromJSON(row.DeepLink)
	if err != nil {
		return Item{}, fmt.Errorf("deep_link: %w", err)
	}
	return Item{
		ID:                      row.ID,
		TenantID:                row.TenantID,
		TeamID:                  ptrUUID(row.TeamID),
		TargetUserID:            row.TargetUserID,
		Scope:                   row.Scope,
		ItemType:                ItemType(row.ItemType),
		SourceType:              SourceType(row.SourceType),
		SourceID:                row.SourceID,
		SourceProjectID:         ptrUUID(row.SourceProjectID),
		SourceTaskID:            ptrUUID(row.SourceTaskID),
		SourceApprovalRequestID: ptrUUID(row.SourceApprovalRequestID),
		Title:                   row.Title,
		Summary:                 ptrText(row.Summary),
		RiskLevel:               ptrText(row.RiskLevel),
		Priority:                ptrText(row.Priority),
		Status:                  Status(row.Status),
		Actions:                 actions,
		ContextPayload:          contextPayload,
		DeepLink:                deepLink,
		ResolvedAt:              ptrTime(row.ResolvedAt),
		LastActivityAt:          timeFromTimestamptz(row.LastActivityAt),
		CreatedAt:               timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:               timeFromTimestamptz(row.UpdatedAt),
	}, nil
}

func marshalItemJSON(req UpsertItemRequest) ([]byte, []byte, []byte, error) {
	actionSchema, err := jsonbActions(req.Actions, "action_schema")
	if err != nil {
		return nil, nil, nil, err
	}
	contextPayload, err := jsonbObject(req.ContextPayload, "context_payload")
	if err != nil {
		return nil, nil, nil, err
	}
	deepLink, err := jsonbObject(req.DeepLink, "deep_link")
	if err != nil {
		return nil, nil, nil, err
	}
	return actionSchema, contextPayload, deepLink, nil
}

func jsonbActions(value []Action, field string) ([]byte, error) {
	if len(value) == 0 {
		return []byte("[]"), nil
	}
	return marshalJSON(value, field)
}

func jsonbObject(value map[string]any, field string) ([]byte, error) {
	if len(value) == 0 {
		return []byte("{}"), nil
	}
	return marshalJSON(value, field)
}

func actionsFromJSON(raw []byte) ([]Action, error) {
	if len(raw) == 0 {
		return []Action{}, nil
	}
	var actions []Action
	if err := json.Unmarshal(raw, &actions); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	if actions == nil {
		return []Action{}, nil
	}
	return actions, nil
}

func mapFromJSON(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	if value == nil {
		return map[string]any{}, nil
	}
	return value, nil
}

func marshalJSON(value any, field string) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal json: %w", field, err)
	}
	return raw, nil
}

func nullUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func ptrUUID(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := value.UUID
	return &id
}

func textFromString(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func textFromStringPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return textFromString(*value)
}

func textFromItemType(value *ItemType) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return textFromString(string(*value))
}

func ptrText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func timestamptzFromPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*value)
}

func ptrTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
