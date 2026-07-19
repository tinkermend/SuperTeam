package employee

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	EnvironmentVariableRepository

	WithTransaction(ctx context.Context, fn func(Repository) error) error
	CreateDigitalEmployee(ctx context.Context, params CreateDigitalEmployeeParams) (DigitalEmployeeRecord, error)
	ListDigitalEmployees(ctx context.Context, params ListDigitalEmployeesParams) ([]DigitalEmployeeRecord, error)
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error)
	GetDigitalEmployeeOperationalState(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeOperationalState, error)
	GetDigitalEmployeeForDelete(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error)
	ListDigitalEmployeeDeleteBlockers(ctx context.Context, tenantID, employeeID uuid.UUID) ([]DigitalEmployeeDeleteBlocker, error)
	SoftDeleteDigitalEmployeeCascade(ctx context.Context, params SoftDeleteDigitalEmployeeCascadeParams) (DigitalEmployeeDeleteCascadeResult, error)
	CreateDigitalEmployeeDeleteAuditEvent(ctx context.Context, params DigitalEmployeeDeleteAuditEventParams) error
	GetDigitalEmployeeOverview(ctx context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error)
	GetDigitalEmployeeActivity(ctx context.Context, req GetDigitalEmployeeActivityRequest) ([]DigitalEmployeeActivityItem, error)
	AreRuntimeReady(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	EnsureTeamExists(ctx context.Context, tenantID, teamID uuid.UUID) error
	GetTeamBaseline(ctx context.Context, tenantID, teamID uuid.UUID) (TeamBaseline, error)
	ListUsedAvatarAssetIDs(ctx context.Context, tenantID uuid.UUID) (map[string]struct{}, error)
	ListSkillCapabilityOptions(ctx context.Context, tenantID uuid.UUID) ([]CapabilityRegistryOption, error)
	ListMCPCapabilityOptions(ctx context.Context, tenantID uuid.UUID) ([]CapabilityRegistryOption, error)
	ResolveSkillIDsBySlugs(ctx context.Context, tenantID uuid.UUID, slugs []string) (map[string]uuid.UUID, error)
	ResolveMCPServerIDsByKeys(ctx context.Context, tenantID uuid.UUID, keys []string) (map[string]uuid.UUID, error)
	BindSkillsToEmployee(ctx context.Context, tenantID, employeeID uuid.UUID, skillIDs []uuid.UUID) error
	BindMCPServersToEmployee(ctx context.Context, tenantID, employeeID uuid.UUID, serverIDs []uuid.UUID) error
	ListRuntimeProviderOptionsForCreate(ctx context.Context, tenantID, teamID uuid.UUID) ([]RuntimeProviderOption, error)
	ListRuntimeProviderOptionsForTeamLessCreate(ctx context.Context, tenantID uuid.UUID) ([]RuntimeProviderOption, error)
	GetRuntimeProvisioningPreflight(ctx context.Context, tenantID, teamID, runtimeNodeID uuid.UUID, providerType string) (RuntimeProvisioningPreflight, error)
	GetRuntimeProvisioningPreflightTeamLess(ctx context.Context, tenantID, runtimeNodeID uuid.UUID, providerType string) (RuntimeProvisioningPreflight, error)
	UpdateDigitalEmployeeStatus(ctx context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error)
	UpsertDigitalEmployeeExecutionInstance(ctx context.Context, params UpsertExecutionInstanceParams) (DigitalEmployeeExecutionInstanceRecord, error)
	GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeExecutionInstanceRecord, error)
	GetDigitalEmployeeOperationalSignals(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]OperationalSignals, error)
	CreateRuntimeCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error
	WaitForRuntimeCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeCommandReceipt, error)
	AbortProvisionedDigitalEmployee(ctx context.Context, tenantID, employeeID, executionInstanceID uuid.UUID, reason string) error
	CreateDigitalEmployeeConfigRevision(ctx context.Context, params CreateConfigRevisionParams) (DigitalEmployeeConfigRevisionRecord, error)
	GetDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID, employeeConfigRevisionID uuid.UUID) (EmployeeConfigInput, error)
	GetLatestDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (EmployeeConfigInput, error)
	GetNextDigitalEmployeeConfigRevisionNumber(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (int32, error)
	GetSchedulingCapabilityFacts(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (SchedulingCapabilityFacts, error)
	GetDigitalEmployeeRunStats(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeRunStats, error)
	ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error)
	ListEmployeeTemplates(ctx context.Context, params ListEmployeeTemplatesParams) ([]EmployeeTemplateRecord, error)
	GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error)
	GetEmployeeTemplateByType(ctx context.Context, tenantID uuid.UUID, employeeType string) (EmployeeTemplateRecord, error)
	CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error)
	UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error)
	SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error)
	SoftDeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error
	ListEmployeeTemplateLabels(ctx context.Context, tenantID uuid.UUID) (map[string]string, error)
}

type CreateDigitalEmployeeParams struct {
	TenantID         uuid.UUID
	TeamID           *uuid.UUID
	OwnerUserID      uuid.UUID
	EmployeeType     string
	ProviderType     string
	Name             string
	Role             string
	Description      *string
	Status           DigitalEmployeeStatus
	PermissionPolicy map[string]any
	RiskLevel        string
	Metadata         map[string]any
}

type ListDigitalEmployeesParams struct {
	TenantID   uuid.UUID
	TeamID     *uuid.UUID
	Status     DigitalEmployeeStatus
	Assignment string
	Offset     int32
	Limit      int32
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
	TenantID              uuid.UUID
	DigitalEmployeeID     uuid.UUID
	RevisionNumber        int32
	PersonaMemoryMarkdown string
	CapabilityBindings    map[string]any
	BudgetPolicy          map[string]any
	Status                ConfigRevisionStatus
	ApprovedBy            *uuid.UUID
	ApprovedAt            *time.Time
}

type DigitalEmployeeRecord struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	TeamID           *uuid.UUID
	OwnerUserID      uuid.UUID
	EmployeeType     string
	ProviderType     string
	Name             string
	Role             string
	Description      *string
	Status           DigitalEmployeeStatus
	PermissionPolicy map[string]any
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
	ID                    uuid.UUID
	TenantID              uuid.UUID
	DigitalEmployeeID     uuid.UUID
	RevisionNumber        int32
	PersonaMemoryMarkdown string
	CapabilityBindings    map[string]any
	BudgetPolicy          map[string]any
	Status                ConfigRevisionStatus
	ApprovedBy            *uuid.UUID
	ApprovedAt            *time.Time
	ArchivedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}
