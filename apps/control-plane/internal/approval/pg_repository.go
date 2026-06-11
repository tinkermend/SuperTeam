package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateApprovalRequest(ctx context.Context, input CreateRequestInput, status ApprovalStatus) (ApprovalRequest, error) {
	options, err := jsonbArray(input.Options, "options")
	if err != nil {
		return ApprovalRequest{}, err
	}
	payload, err := jsonbObject(input.ContextPayload, "context_payload")
	if err != nil {
		return ApprovalRequest{}, err
	}
	row, err := r.q.CreateApprovalRequest(ctx, queries.CreateApprovalRequestParams{
		TenantID:       input.TenantID,
		ResourceType:   input.ResourceType,
		ResourceID:     input.ResourceID,
		RequesterType:  input.RequesterType,
		RequesterID:    nullUUID(input.RequesterID),
		TargetUserID:   input.TargetUserID,
		DecisionType:   input.DecisionType,
		Title:          input.Title,
		Summary:        textOrNull(input.Summary),
		RiskLevel:      textOrNull(input.RiskLevel),
		Status:         string(status),
		Options:        options,
		ContextPayload: payload,
	})
	if err != nil {
		return ApprovalRequest{}, err
	}
	return requestFromRecord(row)
}

func (r *PgRepository) GetApprovalRequest(ctx context.Context, tenantID, requestID uuid.UUID) (ApprovalRequest, error) {
	row, err := r.q.GetApprovalRequest(ctx, queries.GetApprovalRequestParams{TenantID: tenantID, ID: requestID})
	if err != nil {
		return ApprovalRequest{}, err
	}
	return requestFromRecord(row)
}

func (r *PgRepository) ResolveApprovalRequest(ctx context.Context, input ResolveRequestInput, status ApprovalStatus) (ApprovalRequest, error) {
	row, err := r.q.ResolveApprovalRequest(ctx, queries.ResolveApprovalRequestParams{
		TenantID: input.TenantID,
		ID:       input.ApprovalRequestID,
		Status:   string(status),
	})
	if err != nil {
		return ApprovalRequest{}, err
	}
	return requestFromRecord(row)
}

func (r *PgRepository) CreateApprovalDecision(ctx context.Context, input ResolveRequestInput) (ApprovalDecisionRecord, error) {
	payload, err := jsonbObject(input.Payload, "payload")
	if err != nil {
		return ApprovalDecisionRecord{}, err
	}
	row, err := r.q.CreateApprovalDecision(ctx, queries.CreateApprovalDecisionParams{
		TenantID:          input.TenantID,
		ApprovalRequestID: input.ApprovalRequestID,
		DecidedByUserID:   input.DecidedByUserID,
		Decision:          string(input.Decision),
		Comment:           textOrNull(input.Comment),
		Payload:           payload,
	})
	if err != nil {
		return ApprovalDecisionRecord{}, err
	}
	return decisionFromRecord(row)
}

func requestFromRecord(row queries.ApprovalRequest) (ApprovalRequest, error) {
	options := []any{}
	if len(row.Options) > 0 {
		if err := json.Unmarshal(row.Options, &options); err != nil {
			return ApprovalRequest{}, fmt.Errorf("options: %w", err)
		}
		if options == nil {
			options = []any{}
		}
	}
	payload, err := mapFromJSON(row.ContextPayload)
	if err != nil {
		return ApprovalRequest{}, fmt.Errorf("context_payload: %w", err)
	}
	return ApprovalRequest{
		ID:             row.ID,
		TenantID:       row.TenantID,
		ResourceType:   row.ResourceType,
		ResourceID:     row.ResourceID,
		RequesterType:  row.RequesterType,
		RequesterID:    ptrUUID(row.RequesterID),
		TargetUserID:   row.TargetUserID,
		DecisionType:   row.DecisionType,
		Title:          row.Title,
		Summary:        ptrText(row.Summary),
		RiskLevel:      ptrText(row.RiskLevel),
		Status:         ApprovalStatus(row.Status),
		Options:        options,
		ContextPayload: payload,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
		ResolvedAt:     ptrTime(row.ResolvedAt),
	}, nil
}

func decisionFromRecord(row queries.ApprovalDecision) (ApprovalDecisionRecord, error) {
	payload, err := mapFromJSON(row.Payload)
	if err != nil {
		return ApprovalDecisionRecord{}, fmt.Errorf("payload: %w", err)
	}
	return ApprovalDecisionRecord{
		ID:                row.ID,
		TenantID:          row.TenantID,
		ApprovalRequestID: row.ApprovalRequestID,
		DecidedByUserID:   row.DecidedByUserID,
		Decision:          ApprovalDecision(row.Decision),
		Comment:           ptrText(row.Comment),
		Payload:           payload,
		CreatedAt:         row.CreatedAt.Time,
	}, nil
}

func textOrNull(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func ptrText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
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

func ptrTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func jsonbObject(value map[string]any, field string) ([]byte, error) {
	if len(value) == 0 {
		return []byte("{}"), nil
	}
	return marshalJSON(value, field)
}

func jsonbArray(value []any, field string) ([]byte, error) {
	if len(value) == 0 {
		return []byte("[]"), nil
	}
	return marshalJSON(value, field)
}

func mapFromJSON(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
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
