package project

import (
	"time"

	"github.com/google/uuid"
)

const (
	PlanRevisionStatusDraft            = "draft"
	PlanRevisionStatusValidationFailed = "validation_failed"
	PlanRevisionStatusPendingReview    = "pending_review"
	PlanRevisionStatusAccepted         = "accepted"
	PlanRevisionStatusRejected         = "rejected"
	PlanRevisionStatusSuperseded       = "superseded"
	PlanRevisionStatusDecomposing      = "decomposing"
	PlanRevisionStatusDecomposed       = "decomposed"
)

const (
	PlanReviewDecisionAccept         = "approved"
	PlanReviewDecisionReject         = "rejected"
	PlanReviewDecisionRequestChanges = "request_changes"
	PlanReviewDecisionCancel         = "cancelled"
)

const (
	// DecisionTypePlanningGap is the human-decision type opened when a demand's
	// route cannot be planned because the executor pool has a structural gap.
	DecisionTypePlanningGap = "planning_gap"
	// PlanningGapDecisionRestaffed resolves a planning_gap decision by declaring
	// the pool has been supplemented; the coordinator reopens the demand and
	// replans it. Like request_changes it is decision-type-scoped vocabulary.
	PlanningGapDecisionRestaffed = "restaffed"
)

const (
	PlanDecompositionClaimStatusInFlight  = "in_flight"
	PlanDecompositionClaimStatusCompleted = "completed"
	PlanDecompositionClaimStatusFailed    = "failed"
)

type PlanRevision struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            *uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID *uuid.UUID
	RouteDecisionID   *uuid.UUID
	RevisionNumber    int32
	Status            string

	Payload          map[string]any
	PlannerProvider  *string
	PlannerModel     *string
	PlannerInputHash *string
	PlanFingerprint  string

	ValidationErrors   []string
	ValidationWarnings []string
	ReviewRequired     bool
	ReviewReason       *string

	AcceptedBy      *uuid.UUID
	AcceptedAt      *time.Time
	RejectedBy      *uuid.UUID
	RejectedAt      *time.Time
	RejectionReason *string

	SupersededByRevisionID *uuid.UUID
	DecompositionClaimID   *uuid.UUID
	CreatedTaskIDs         []uuid.UUID
	CreatedEventID         *uuid.UUID

	// CoordinationMode is the demand's coordination_mode ("plan"/"loop") frozen onto this
	// revision at persist time. NULL means the source demand could not be read (legacy/missing)
	// and is interpreted as "loop" downstream.
	CoordinationMode *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type PlanDecompositionClaim struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	DemandID               uuid.UUID
	AcceptedPlanRevisionID uuid.UUID
	PlanFingerprint        string
	Status                 string
	CreatedTaskIDs         []uuid.UUID
	Error                  map[string]any
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func CanTransitionPlanRevisionStatus(from, to string) bool {
	switch from {
	case PlanRevisionStatusDraft:
		return to == PlanRevisionStatusValidationFailed ||
			to == PlanRevisionStatusPendingReview ||
			to == PlanRevisionStatusAccepted
	case PlanRevisionStatusValidationFailed:
		return to == PlanRevisionStatusSuperseded
	case PlanRevisionStatusPendingReview:
		return to == PlanRevisionStatusAccepted ||
			to == PlanRevisionStatusRejected ||
			to == PlanRevisionStatusSuperseded
	case PlanRevisionStatusAccepted:
		return to == PlanRevisionStatusDecomposing
	case PlanRevisionStatusDecomposing:
		return to == PlanRevisionStatusDecomposed
	case PlanRevisionStatusDecomposed:
		return to == PlanRevisionStatusDecomposed
	default:
		return false
	}
}

func IsAcceptedPlanRevisionStatus(status string) bool {
	switch status {
	case PlanRevisionStatusAccepted, PlanRevisionStatusDecomposing, PlanRevisionStatusDecomposed:
		return true
	default:
		return false
	}
}

func IsMutablePlanRevisionStatus(status string) bool {
	switch status {
	case PlanRevisionStatusDraft, PlanRevisionStatusValidationFailed, PlanRevisionStatusPendingReview:
		return true
	default:
		return false
	}
}
