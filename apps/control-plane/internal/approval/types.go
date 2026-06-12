package approval

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidApprovalRequest  = errors.New("invalid approval request")
	ErrApprovalNotFound        = errors.New("approval request not found")
	ErrApprovalAlreadyResolved = errors.New("approval request already resolved")
)

type ApprovalStatus string

const (
	ApprovalStatusPending           ApprovalStatus = "pending"
	ApprovalStatusApproved          ApprovalStatus = "approved"
	ApprovalStatusRejected          ApprovalStatus = "rejected"
	ApprovalStatusNeedsMoreEvidence ApprovalStatus = "needs_more_evidence"
	ApprovalStatusCancelled         ApprovalStatus = "cancelled"
)

type ApprovalDecision string

const (
	ApprovalDecisionApproved          ApprovalDecision = "approved"
	ApprovalDecisionRejected          ApprovalDecision = "rejected"
	ApprovalDecisionNeedsMoreEvidence ApprovalDecision = "needs_more_evidence"
)

type ApprovalRequest struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	ResourceType   string
	ResourceID     uuid.UUID
	RequesterType  string
	RequesterID    *uuid.UUID
	TargetUserID   uuid.UUID
	DecisionType   string
	Title          string
	Summary        *string
	RiskLevel      *string
	Status         ApprovalStatus
	Options        []any
	ContextPayload map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ResolvedAt     *time.Time
}

type ApprovalDecisionRecord struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ApprovalRequestID uuid.UUID
	DecidedByUserID   uuid.UUID
	Decision          ApprovalDecision
	Comment           *string
	Payload           map[string]any
	CreatedAt         time.Time
}

type InboxProjector interface {
	UpsertApprovalRequest(ctx context.Context, request ApprovalRequest) error
	ResolveApprovalRequest(ctx context.Context, request ApprovalRequest) error
}

type CreateRequestInput struct {
	TenantID       uuid.UUID
	ResourceType   string
	ResourceID     uuid.UUID
	RequesterType  string
	RequesterID    *uuid.UUID
	TargetUserID   uuid.UUID
	DecisionType   string
	Title          string
	Summary        string
	RiskLevel      string
	Options        []any
	ContextPayload map[string]any
}

type ResolveRequestInput struct {
	TenantID          uuid.UUID
	ApprovalRequestID uuid.UUID
	DecidedByUserID   uuid.UUID
	Decision          ApprovalDecision
	Comment           string
	Payload           map[string]any
}
