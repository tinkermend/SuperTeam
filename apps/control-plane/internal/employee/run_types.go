package employee

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type DigitalEmployeeRunStatus string

const (
	DigitalEmployeeRunStatusQueued      DigitalEmployeeRunStatus = "queued"
	DigitalEmployeeRunStatusDispatching DigitalEmployeeRunStatus = "dispatching"
	DigitalEmployeeRunStatusRunning     DigitalEmployeeRunStatus = "running"
	DigitalEmployeeRunStatusCancelling  DigitalEmployeeRunStatus = "cancelling"
	DigitalEmployeeRunStatusCompleted   DigitalEmployeeRunStatus = "completed"
	DigitalEmployeeRunStatusFailed      DigitalEmployeeRunStatus = "failed"
	DigitalEmployeeRunStatusCancelled   DigitalEmployeeRunStatus = "cancelled"
	DigitalEmployeeRunStatusTimedOut    DigitalEmployeeRunStatus = "timed_out"
)

func (s DigitalEmployeeRunStatus) IsTerminal() bool {
	switch s {
	case DigitalEmployeeRunStatusCompleted, DigitalEmployeeRunStatusFailed, DigitalEmployeeRunStatusCancelled, DigitalEmployeeRunStatusTimedOut:
		return true
	default:
		return false
	}
}

func (s DigitalEmployeeRunStatus) IsActive() bool {
	switch s {
	case DigitalEmployeeRunStatusQueued, DigitalEmployeeRunStatusDispatching, DigitalEmployeeRunStatusRunning, DigitalEmployeeRunStatusCancelling:
		return true
	default:
		return false
	}
}

type WorkProduct struct {
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Summary   string         `json:"summary,omitempty"`
	Ref       string         `json:"ref,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
}

type DigitalEmployeeRun struct {
	ID                        uuid.UUID
	TenantID                  uuid.UUID
	TaskID                    uuid.UUID
	DigitalEmployeeID         uuid.UUID
	ExecutionInstanceID       uuid.UUID
	RuntimeNodeID             uuid.UUID
	NodeID                    string
	CommandID                 string
	ProviderType              string
	ProviderSessionID         *string
	ProviderSessionExternalID *string
	Status                    DigitalEmployeeRunStatus
	Result                    map[string]any
	Diagnostic                map[string]any
	LogRef                    *string
	RawResultRef              *string
	WorkProducts              []WorkProduct
	SessionState              map[string]any
	ErrorMessage              *string
	ErrorCode                 *string
	ErrorFamily               *string
	ExitCode                  *int32
	Signal                    *string
	TimedOut                  bool
	IdempotencyKey            *string
	IdempotencyFingerprint    *string
	TimeoutSec                *int32
	GraceSec                  *int32
	StartedAt                 time.Time
	CompletedAt               *time.Time
	FinishedAt                *time.Time
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type CreateDigitalEmployeeRunRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Objective         string
	Prompt            string
	ContextRefs       []map[string]any
	ArtifactRefs      []map[string]any
	OutputSchema      map[string]any
	AllowedActions    []string
	ForbiddenActions  []string
	SecretRefs        []string
	IdempotencyKey    *string
	TimeoutSec        *int32
	GraceSec          *int32
	Metadata          map[string]any
}

type StopDigitalEmployeeRunRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	RunID             uuid.UUID
	Reason            string
}

type RuntimeCommandWritebackIdentity struct {
	TenantID      uuid.UUID
	RuntimeNodeID uuid.UUID
	NodeID        string
}

type RuntimeEventRecordRequest struct {
	TenantID        uuid.UUID
	RuntimeNodeID   uuid.UUID
	NodeID          string
	EventType       string
	Severity        string
	Source          string
	Title           string
	Description     string
	ProviderType    string
	CorrelationType string
	CorrelationID   string
	Payload         map[string]any
}

type RuntimeEventRecorder interface {
	RecordRuntimeEvent(ctx context.Context, req RuntimeEventRecordRequest) error
}

type RuntimeCommandEventWriteback struct {
	EventType                 string         `json:"event_type"`
	SequenceNumber            int32          `json:"sequence_number"`
	Payload                   map[string]any `json:"payload"`
	ProviderSessionExternalID *string        `json:"provider_session_external_id,omitempty"`
	SessionStatePatch         map[string]any `json:"session_state_patch,omitempty"`
	LogRef                    *string        `json:"log_ref,omitempty"`
	RawEventRef               *string        `json:"raw_event_ref,omitempty"`
	Metadata                  map[string]any `json:"metadata,omitempty"`
}

type RuntimeCommandTerminalWriteback struct {
	Status                    DigitalEmployeeRunStatus `json:"status"`
	Summary                   string                   `json:"summary,omitempty"`
	Result                    map[string]any           `json:"result,omitempty"`
	Diagnostic                map[string]any           `json:"diagnostic,omitempty"`
	WorkProducts              []WorkProduct            `json:"work_products,omitempty"`
	ProviderSessionExternalID *string                  `json:"provider_session_external_id,omitempty"`
	SessionStatePatch         map[string]any           `json:"session_state_patch,omitempty"`
	LogRef                    *string                  `json:"log_ref,omitempty"`
	RawResultRef              *string                  `json:"raw_result_ref,omitempty"`
	ErrorMessage              *string                  `json:"error_message,omitempty"`
	ErrorCode                 *string                  `json:"error_code,omitempty"`
	ErrorFamily               *string                  `json:"error_family,omitempty"`
	ExitCode                  *int32                   `json:"exit_code,omitempty"`
	Signal                    *string                  `json:"signal,omitempty"`
	TimedOut                  bool                     `json:"timed_out,omitempty"`
}
