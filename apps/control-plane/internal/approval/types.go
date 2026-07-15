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
	// ApprovalDecisionRequestChanges records a plan-review "request changes"
	// resolution; the approval request itself closes as rejected because the
	// reviewed plan revision is superseded and a new review will be opened.
	ApprovalDecisionRequestChanges ApprovalDecision = "request_changes"
	// ApprovalDecisionRestaffed records a planning_gap "已补员，重新规划"
	// resolution; the approval request closes as rejected (statusFromDecision
	// default) because the demand is reopened and a fresh planning cycle — with
	// its own review — begins.
	ApprovalDecisionRestaffed ApprovalDecision = "restaffed"
	// ApprovalDecisionExempted records a planning_gap "豁免约束并重规划"
	// resolution; like restaffed, the approval request closes as rejected
	// (statusFromDecision default) because the demand is reopened and a fresh
	// planning cycle begins — this time with the exempted constraint skipped.
	ApprovalDecisionExempted ApprovalDecision = "exempted"
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
