package approval

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateApprovalRequest(ctx context.Context, input CreateRequestInput, status ApprovalStatus) (ApprovalRequest, error)
	GetApprovalRequest(ctx context.Context, tenantID, requestID uuid.UUID) (ApprovalRequest, error)
	ResolveApprovalRequest(ctx context.Context, input ResolveRequestInput, status ApprovalStatus) (ApprovalRequest, error)
	CreateApprovalDecision(ctx context.Context, input ResolveRequestInput) (ApprovalDecisionRecord, error)
}
