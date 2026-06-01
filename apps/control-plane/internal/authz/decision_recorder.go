package authz

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

const (
	ModuleAuthz     = "authz"
	ResultSucceeded = "succeeded"
	ResultFailed    = "failed"
)

type OperationLogQueries interface {
	CreateWebOperationLog(ctx context.Context, params queries.CreateWebOperationLogParams) (queries.WebOperationLog, error)
}

type OperationLogDecisionRecorder struct {
	q OperationLogQueries
}

func NewOperationLogDecisionRecorder(q OperationLogQueries) *OperationLogDecisionRecorder {
	return &OperationLogDecisionRecorder{q: q}
}

func (r *OperationLogDecisionRecorder) RecordDecision(ctx context.Context, record DecisionRecord) error {
	if r == nil || r.q == nil {
		return nil
	}
	details, err := json.Marshal(map[string]any{
		"allowed":      record.Allowed,
		"reason":       record.Reason,
		"matched_rule": record.MatchedRule,
		"engine":       record.Engine,
		"snapshot":     record.Snapshot,
	})
	if err != nil {
		return err
	}
	_, err = r.q.CreateWebOperationLog(ctx, queries.CreateWebOperationLogParams{
		TenantID:     uuid.NullUUID{UUID: record.TenantID, Valid: record.TenantID != uuid.Nil},
		UserID:       userIDFromRecord(record),
		Username:     pgtype.Text{},
		Module:       ModuleAuthz,
		ResourceType: text(record.ResourceType),
		ResourceID:   text(record.ResourceID),
		Action:       record.Action,
		Result:       result(record.Allowed),
		RequestID:    pgtype.Text{},
		ClientIp:     pgtype.Text{},
		UserAgent:    pgtype.Text{},
		Details:      details,
	})
	return err
}

func userIDFromRecord(record DecisionRecord) uuid.NullUUID {
	if record.ActorType != ActorUser {
		return uuid.NullUUID{}
	}
	id, err := uuid.Parse(record.ActorID)
	if err != nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: id, Valid: true}
}

func text(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func result(allowed bool) string {
	if allowed {
		return ResultSucceeded
	}
	return ResultFailed
}
