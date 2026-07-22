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
	GetDigitalEmployeeDetailAffiliation(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeDetailAffiliation, error)
	GetDigitalEmployeeOperationalState(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeOperationalState, error)
	GetDigitalEmployeeForDelete(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error)
	ListDigitalEmployeeDeleteBlockers(ctx context.Context, tenantID, employeeID uuid.UUID) ([]DigitalEmployeeDeleteBlocker, error)
	SoftDeleteDigitalEmployeeCascade(ctx context.Context, params SoftDeleteDigitalEmployeeCascadeParams) (DigitalEmployeeDeleteCascadeResult, error)
	CreateDigitalEmployeeDeleteAuditEvent(ctx context.Context, params DigitalEmployeeDeleteAuditEventParams) error
	GetDigitalEmployeeOverview(ctx context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error)
	GetDigitalEmployeeActivity(ctx context.Context, req GetDigitalEmployeeActivityRequest) ([]DigitalEmployeeActivityItem, error)
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
	UpdateDigitalEmployeeStatus(ctx context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error)
	// UpdateDigitalEmployeeRolePermission 写回经权限中心批准的 role/permission_policy(方案2:
	// 权限变更不进 config_revision,值由审批请求 ContextPayload 承载,批准时落库员工行)。
	UpdateDigitalEmployeeRolePermission(ctx context.Context, tenantID, employeeID uuid.UUID, role string, permissionPolicy map[string]any) (DigitalEmployeeRecord, error)
	GetDigitalEmployeeOperationalSignals(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]OperationalSignals, error)
	CreateRuntimeCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error
	WaitForRuntimeCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeCommandReceipt, error)
	CreateDigitalEmployeeConfigRevision(ctx context.Context, params CreateConfigRevisionParams) (DigitalEmployeeConfigRevisionRecord, error)
	// ArchivePriorActiveConfigRevisions 归档某员工当前所有 active 修订,让位给新激活的修订(维持
	// 每员工至多一条 active 的偏唯一索引)。应与创建新 active 修订同事务调用。
	ArchivePriorActiveConfigRevisions(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) error
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

// DigitalEmployeeDetailAffiliation is the detail-page team name + project binding summary.
type DigitalEmployeeDetailAffiliation struct {
	TeamName       string
	ProjectSummary DigitalEmployeeProjectSummary
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
