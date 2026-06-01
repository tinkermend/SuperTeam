package authzcenter

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	CountDecisionsSince(ctx context.Context, since time.Time) (DecisionTotals, error)
	ListTopDeniedActionsSince(ctx context.Context, since time.Time, limit int32) ([]ActionCount, error)
	ListDecisions(ctx context.Context, filter DecisionFilter) ([]DecisionRecord, error)
	ListRuntimeScopeNodes(ctx context.Context) ([]RuntimeScopeNodeRecord, error)
	CreateRuntimeScope(ctx context.Context, input RuntimeScopeInput) (RuntimeScopeRecord, error)
	UpdateRuntimeScopeStatus(ctx context.Context, scopeID uuid.UUID, status string) (RuntimeScopeRecord, error)
	ListMembers(ctx context.Context, filter MemberFilter) ([]MemberRecord, error)
	RecordOperationLog(ctx context.Context, input OperationLogInput) error
}
