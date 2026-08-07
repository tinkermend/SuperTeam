package project

import (
	"time"

	"github.com/google/uuid"
)

const (
	ExecutionLedgerEventAttemptStarted       = "attempt.started"
	ExecutionLedgerEventAttemptCompleted     = "attempt.completed"
	ExecutionLedgerEventAttemptFailed        = "attempt.failed"
	ExecutionLedgerEventAttemptWaitingHuman  = "attempt.waiting_human"
	ExecutionLedgerEventSummaryCreated       = "summary.created"
	ExecutionLedgerEventProviderSessionStart = "provider.session.started"
	ExecutionLedgerEventProviderEvent        = "provider.event"
	ExecutionLedgerEventToolCall             = "tool.call"
	ExecutionLedgerEventMCPToolCall          = "mcp.tool_call"
	ExecutionLedgerEventCapabilityInvocation = "capability.invocation"
	ExecutionLedgerEventArtifactLinked       = "artifact.linked"
	ExecutionLedgerEventEvidenceLinked       = "evidence.linked"
	ExecutionLedgerEventHandoffVerified      = "handoff.verified"
	ExecutionLedgerEventHandoffUnfulfilled   = "handoff.unfulfilled"
)

type ExecutionLedgerEvent struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	TeamID               *uuid.UUID
	ProjectID            uuid.UUID
	ProjectTaskID        *uuid.UUID
	ProjectTaskAttemptID *uuid.UUID
	EventType            string
	SourceType           string
	SourceID             string
	ActorType            string
	ActorID              *string
	RuntimeNodeID        *uuid.UUID
	ProviderType         *string
	ProviderSessionID    *string
	InputSummary         *string
	OutputSummary        *string
	ErrorFamily          *string
	ErrorCode            *string
	ErrorMessage         *string
	Retryable            *bool
	ArtifactRefs         []any
	EvidenceRefs         []any
	Metadata             map[string]any
	OccurredAt           time.Time
	IdempotencyKey       string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type CreateExecutionLedgerEventRequest struct {
	TenantID             uuid.UUID
	TeamID               *uuid.UUID
	ProjectID            uuid.UUID
	ProjectTaskID        *uuid.UUID
	ProjectTaskAttemptID *uuid.UUID
	EventType            string
	SourceType           string
	SourceID             string
	ActorType            string
	ActorID              *string
	RuntimeNodeID        *uuid.UUID
	ProviderType         *string
	ProviderSessionID    *string
	InputSummary         string
	OutputSummary        string
	ErrorFamily          string
	ErrorCode            string
	ErrorMessage         string
	Retryable            *bool
	ArtifactRefs         []any
	EvidenceRefs         []any
	Metadata             map[string]any
	OccurredAt           *time.Time
	IdempotencyKey       string
}

type GetExecutionTraceRequest struct {
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	ProjectTaskID        *uuid.UUID
	ProjectTaskAttemptID *uuid.UUID
	EventType            *string
	ErrorFamily          *string
	Limit                int32
	Offset               int32
}

type ProjectExecutionTrace struct {
	ProjectID uuid.UUID
	Summary   ProjectExecutionTraceSummary
	Attempts  []ProjectExecutionTraceAttempt
}

type ProjectExecutionTraceSummary struct {
	AttemptCount             int32
	FailedAttemptCount       int32
	HumanReviewRequiredCount int32
	ArtifactRefCount         int32
	EvidenceRefCount         int32
	LatestErrorFamily        *string
}

type ProjectExecutionTraceAttempt struct {
	ProjectTaskID     uuid.UUID
	AttemptID         uuid.UUID
	AttemptNo         int32
	Status            string
	RuntimeNodeID     *uuid.UUID
	ProviderType      *string
	ProviderSessionID *string
	// SessionResumeStatus/Label 来自 attempt.execution_context_packet（派发期固化）。
	SessionResumeStatus *string
	SessionResumeLabel  *string
	StartedAt           *time.Time
	FinishedAt          *time.Time
	FailureFamily       *string
	Retryable           *bool
	Events              []ExecutionLedgerEvent
	Summary             *ProjectExecutionTraceAttemptSummary
	// CapabilityProjection is the console-safe skill/MCP snapshot for this attempt (P3).
	// Always set by GetExecutionTrace (available=false when unresolvable).
	CapabilityProjection *CapabilityProjectionSnapshot
}

type ProjectExecutionTraceAttemptSummary struct {
	ExecutionSummaryID  uuid.UUID
	Conclusion          string
	RequiresHumanReview bool
	ArtifactRefs        []any
	EvidenceRefs        []any
	CreatedAt           time.Time
}
