package employee

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	WithTransaction(ctx context.Context, fn func(Repository) error) error
	CreateDigitalEmployee(ctx context.Context, params CreateDigitalEmployeeParams) (DigitalEmployeeRecord, error)
	ListDigitalEmployees(ctx context.Context, params ListDigitalEmployeesParams) ([]DigitalEmployeeRecord, error)
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error)
	GetDigitalEmployeeOverview(ctx context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error)
	EnsureTeamExists(ctx context.Context, tenantID, teamID uuid.UUID) error
	GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigInput, error)
	ListRuntimeProviderOptionsForCreate(ctx context.Context, tenantID, teamID uuid.UUID) ([]RuntimeProviderOption, error)
	GetRuntimeProvisioningPreflight(ctx context.Context, tenantID, teamID, runtimeNodeID uuid.UUID, providerType string) (RuntimeProvisioningPreflight, error)
	UpdateDigitalEmployeeStatus(ctx context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error)
	UpsertDigitalEmployeeExecutionInstance(ctx context.Context, params UpsertExecutionInstanceParams) (DigitalEmployeeExecutionInstanceRecord, error)
	GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeExecutionInstanceRecord, error)
	CreateRuntimeCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error
	WaitForRuntimeCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeCommandReceipt, error)
	AbortProvisionedDigitalEmployee(ctx context.Context, tenantID, employeeID, executionInstanceID uuid.UUID, reason string) error
	CreateDigitalEmployeeConfigRevision(ctx context.Context, params CreateConfigRevisionParams) (DigitalEmployeeConfigRevisionRecord, error)
	GetTeamConfigRevision(ctx context.Context, tenantID, teamConfigRevisionID uuid.UUID) (TeamConfigInput, error)
	GetDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID, employeeConfigRevisionID uuid.UUID) (EmployeeConfigInput, error)
	GetNextDigitalEmployeeConfigRevisionNumber(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (int32, error)
	GetCurrentDigitalEmployeeEffectiveConfig(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeEffectiveConfigRecord, error)
	CreateDigitalEmployeeEffectiveConfig(ctx context.Context, params CreateEffectiveConfigParams) (DigitalEmployeeEffectiveConfigRecord, error)
}

type CreateDigitalEmployeeParams struct {
	TenantID         uuid.UUID
	TeamID           *uuid.UUID
	OwnerUserID      uuid.UUID
	EmployeeType     string
	Name             string
	Role             string
	Description      *string
	Status           DigitalEmployeeStatus
	PermissionPolicy map[string]any
	ContextPolicy    map[string]any
	ApprovalPolicy   map[string]any
	RiskLevel        string
	Metadata         map[string]any
}

type ListDigitalEmployeesParams struct {
	TenantID uuid.UUID
	TeamID   *uuid.UUID
	Status   DigitalEmployeeStatus
	Offset   int32
	Limit    int32
}

type UpsertExecutionInstanceParams struct {
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
	Metadata             map[string]any
}

type CreateConfigRevisionParams struct {
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	BudgetPolicy           map[string]any
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
}

type CreateEffectiveConfigParams struct {
	TenantID                 uuid.UUID
	DigitalEmployeeID        uuid.UUID
	TeamConfigRevisionID     uuid.UUID
	EmployeeConfigRevisionID uuid.UUID
	EffectiveConfig          map[string]any
	ValidationResult         map[string]any
	Status                   EffectiveConfigStatus
	ApprovedBy               *uuid.UUID
	ApprovedAt               *time.Time
}

type DigitalEmployeeRecord struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	TeamID           *uuid.UUID
	OwnerUserID      uuid.UUID
	EmployeeType     string
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
	DeletedAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type DigitalEmployeeExecutionInstanceRecord struct {
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
	DeletedAt            *time.Time
	Metadata             map[string]any
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type DigitalEmployeeConfigRevisionRecord struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	BudgetPolicy           map[string]any
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
	ArchivedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type DigitalEmployeeEffectiveConfigRecord struct {
	ID                       uuid.UUID
	TenantID                 uuid.UUID
	DigitalEmployeeID        uuid.UUID
	TeamConfigRevisionID     uuid.UUID
	EmployeeConfigRevisionID uuid.UUID
	EffectiveConfig          map[string]any
	ValidationResult         map[string]any
	Status                   EffectiveConfigStatus
	ApprovedBy               *uuid.UUID
	ApprovedAt               *time.Time
	RevokedAt                *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}
