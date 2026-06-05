package employee

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type DigitalEmployeeRunRepository interface {
	WithTransaction(ctx context.Context, fn func(DigitalEmployeeRunRepository) error) error
	GetRunPreflight(ctx context.Context, tenantID, employeeID uuid.UUID) (RunPreflight, error)
	GetActiveRun(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRun, error)
	GetRun(ctx context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error)
	GetRunByID(ctx context.Context, tenantID, runID uuid.UUID) (*DigitalEmployeeRun, error)
	GetRunByCommandID(ctx context.Context, tenantID uuid.UUID, commandID string) (*DigitalEmployeeRun, error)
	ListRuns(ctx context.Context, tenantID, employeeID uuid.UUID, limit, offset int32) ([]*DigitalEmployeeRun, error)
	ListRunEvents(ctx context.Context, tenantID, taskID, runID uuid.UUID, limit, offset int32) ([]RuntimeCommandEventWriteback, error)
	CreateRun(ctx context.Context, req CreateRunRecordRequest) (*DigitalEmployeeRun, error)
	UpdateRunStatus(ctx context.Context, req UpdateRunStatusRequest) (*DigitalEmployeeRun, error)
	HasRunEventSequence(ctx context.Context, tenantID, taskID, runID uuid.UUID, sequenceNumber int32) (bool, error)
	CreateTaskEventIfAbsent(ctx context.Context, req CreateRunEventRecordRequest) (bool, error)
	UpsertProviderSession(ctx context.Context, req UpsertProviderSessionRequest) (uuid.UUID, error)
	CreateProviderSessionEventIfAbsent(ctx context.Context, req CreateProviderSessionEventRecordRequest) error
	CreateCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error
	GetCommandReceipt(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error)
	GetCommandReceiptForUpdate(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error)
	UpdateCommandReceipt(ctx context.Context, req UpdateRuntimeCommandReceiptRequest) (*RuntimeCommandReceipt, error)
	UpdateExecutionInstanceStatus(ctx context.Context, tenantID, executionInstanceID uuid.UUID, status ExecutionInstanceStatus, errorMessage *string) (DigitalEmployeeExecutionInstanceRecord, error)
	UpdateDigitalEmployeeStatus(ctx context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error)
	DeleteExecutionInstance(ctx context.Context, tenantID, executionInstanceID uuid.UUID) error
	DeleteDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) error
}

type RunPreflight struct {
	TenantID                   uuid.UUID
	TeamID                     uuid.UUID
	DigitalEmployeeID          uuid.UUID
	DigitalEmployeeStatus      DigitalEmployeeStatus
	ExecutionInstanceID        uuid.UUID
	ExecutionStatus            ExecutionInstanceStatus
	RuntimeNodeID              uuid.UUID
	NodeID                     string
	ProviderType               string
	AgentHomeDir               string
	RuntimeSelector            map[string]any
	SessionPolicy              map[string]any
	WorkspacePolicy            map[string]any
	HasApprovedEffectiveConfig bool
	ProviderHealthy            bool
}

type CreateRunRecordRequest struct {
	IdempotencyKey         *string
	IdempotencyFingerprint *string
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	TeamID                 uuid.UUID
	Title                  string
	Description            *string
	Priority               int32
	ProviderType           string
	CreatorID              *uuid.UUID
	TargetNodeID           string
	WorkspacePath          *string
	Params                 map[string]any
	RiskLevel              *string
	NodeID                 string
	RuntimeNodeID          uuid.UUID
	ProviderSessionID      *string
	RunStatus              DigitalEmployeeRunStatus
	CommandID              string
	ExecutionInstanceID    uuid.UUID
	TimeoutSec             *int32
	GraceSec               *int32
}

type UpdateRunStatusRequest struct {
	TenantID                  uuid.UUID
	RunID                     uuid.UUID
	Status                    DigitalEmployeeRunStatus
	Result                    map[string]any
	ErrorMessage              *string
	Diagnostic                map[string]any
	LogRef                    *string
	RawResultRef              *string
	WorkProducts              []WorkProduct
	SessionState              map[string]any
	ErrorCode                 *string
	ErrorFamily               *string
	ExitCode                  *int32
	Signal                    *string
	ProviderSessionExternalID *string
	TimedOut                  bool
}

type CreateRunEventRecordRequest struct {
	TenantID       uuid.UUID
	TaskID         uuid.UUID
	RunID          uuid.UUID
	EventType      string
	SequenceNumber int32
	Payload        map[string]any
	CommandID      *string
	RawEventRef    *string
	LogRef         *string
	Metadata       map[string]any
}

type UpsertProviderSessionRequest struct {
	TenantID            uuid.UUID
	ProviderSessionID   string
	DigitalEmployeeID   uuid.UUID
	ExecutionInstanceID uuid.UUID
	RuntimeNodeID       uuid.UUID
	ProviderType        string
	Status              string
	Recoverable         bool
	SessionDisplayID    *string
	SessionParams       map[string]any
	SessionState        map[string]any
	LastSequenceNumber  int32
	LastCommandID       *string
	LastRunID           *uuid.UUID
	LastErrorFamily     *string
	Metadata            map[string]any
}

type CreateProviderSessionEventRecordRequest struct {
	TenantID            uuid.UUID
	ProviderSessionUUID uuid.UUID
	EventType           string
	SequenceNumber      int32
	Payload             map[string]any
	RequestID           *string
	CommandID           *string
	RawEventRef         *string
	LogRef              *string
	SessionStatePatch   map[string]any
	Metadata            map[string]any
}

type CreateRuntimeCommandReceiptRequest struct {
	TenantID      uuid.UUID
	CommandID     string
	CommandType   string
	RuntimeNodeID uuid.UUID
	NodeID        string
	ResourceType  string
	ResourceID    uuid.UUID
	Status        string
	Payload       map[string]any
	DispatchedAt  *time.Time
}

type UpdateRuntimeCommandReceiptRequest struct {
	TenantID     uuid.UUID
	CommandID    string
	Status       string
	Result       map[string]any
	ErrorMessage *string
}

type RuntimeCommandReceipt struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	CommandID     string
	CommandType   string
	RuntimeNodeID uuid.UUID
	NodeID        string
	ResourceType  string
	ResourceID    uuid.UUID
	Status        string
	Payload       map[string]any
	Result        map[string]any
	ErrorMessage  *string
	DispatchedAt  *time.Time
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
