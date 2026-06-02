package employee

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid employee input")
	ErrNotFound     = errors.New("employee not found")
)

type DigitalEmployeeStatus string

const (
	DigitalEmployeeStatusDraft    DigitalEmployeeStatus = "draft"
	DigitalEmployeeStatusReady    DigitalEmployeeStatus = "ready"
	DigitalEmployeeStatusActive   DigitalEmployeeStatus = "active"
	DigitalEmployeeStatusDisabled DigitalEmployeeStatus = "disabled"
	DigitalEmployeeStatusError    DigitalEmployeeStatus = "error"
)

func (s DigitalEmployeeStatus) IsValid() bool {
	switch s {
	case DigitalEmployeeStatusDraft, DigitalEmployeeStatusReady, DigitalEmployeeStatusActive, DigitalEmployeeStatusDisabled, DigitalEmployeeStatusError:
		return true
	default:
		return false
	}
}

type ExecutionInstanceStatus string

const (
	ExecutionInstanceStatusProvisioning ExecutionInstanceStatus = "provisioning"
	ExecutionInstanceStatusReady        ExecutionInstanceStatus = "ready"
	ExecutionInstanceStatusActive       ExecutionInstanceStatus = "active"
	ExecutionInstanceStatusDisabled     ExecutionInstanceStatus = "disabled"
	ExecutionInstanceStatusError        ExecutionInstanceStatus = "error"
)

func (s ExecutionInstanceStatus) IsValid() bool {
	switch s {
	case ExecutionInstanceStatusProvisioning, ExecutionInstanceStatusReady, ExecutionInstanceStatusActive, ExecutionInstanceStatusDisabled, ExecutionInstanceStatusError:
		return true
	default:
		return false
	}
}

type DigitalEmployee struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	TeamID           *uuid.UUID
	Name             string
	Role             string
	Description      *string
	Status           DigitalEmployeeStatus
	PermissionPolicy map[string]any
	ContextPolicy    map[string]any
	ApprovalPolicy   map[string]any
	RiskLevel        string
	Metadata         map[string]any
	DisabledAt       *time.Time
	ArchivedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type DigitalEmployeeExecutionInstance struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	DigitalEmployeeID    uuid.UUID
	RuntimeNodeID        uuid.UUID
	ProviderType         string
	AgentHomeDir         string
	WorkspacePolicy      map[string]any
	SessionPolicy        map[string]any
	RuntimeSelector      map[string]any
	CapacityRequirements map[string]any
	FallbackPolicy       map[string]any
	Status               ExecutionInstanceStatus
	ReadyAt              *time.Time
	DisabledAt           *time.Time
	ErrorAt              *time.Time
	ErrorMessage         *string
	Metadata             map[string]any
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type CreateDraftRequest struct {
	TenantID         uuid.UUID
	TeamID           *uuid.UUID
	Name             string
	Role             string
	Description      *string
	PermissionPolicy map[string]any
	ContextPolicy    map[string]any
	ApprovalPolicy   map[string]any
	RiskLevel        string
	Metadata         map[string]any
}

type ListDigitalEmployeesRequest struct {
	TenantID uuid.UUID
	TeamID   *uuid.UUID
	Status   DigitalEmployeeStatus
	Offset   int32
	Limit    int32
}

type UpdateStatusRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	Status            DigitalEmployeeStatus
}

type BindExecutionInstanceRequest struct {
	TenantID             uuid.UUID
	DigitalEmployeeID    uuid.UUID
	RuntimeNodeID        uuid.UUID
	ProviderType         string
	AgentHomeDir         string
	WorkspacePolicy      map[string]any
	SessionPolicy        map[string]any
	RuntimeSelector      map[string]any
	CapacityRequirements map[string]any
	FallbackPolicy       map[string]any
	Metadata             map[string]any
}
