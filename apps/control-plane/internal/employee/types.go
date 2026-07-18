package employee

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput                 = errors.New("invalid employee input")
	ErrNotFound                     = errors.New("employee not found")
	ErrConflict                     = errors.New("employee conflict")
	ErrDigitalEmployeeDeleteBlocked = errors.New("digital employee delete blocked")
	ErrRuntimeUnavailable           = errors.New("employee runtime unavailable")
	ErrProviderUnavailable          = errors.New("employee provider unavailable")
	ErrRuntimeIdentityMismatch      = errors.New("employee runtime identity mismatch")
)

const DigitalEmployeeDeleteBlockedCode = "digital_employee_delete_blocked"

type DeleteDigitalEmployeeRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	ActorUserID       uuid.UUID
}

type DigitalEmployeeDeleteBlockerType string

const (
	DigitalEmployeeDeleteBlockerTypeRun         DigitalEmployeeDeleteBlockerType = "run"
	DigitalEmployeeDeleteBlockerTypeProjectTask DigitalEmployeeDeleteBlockerType = "project_task"
)

type DigitalEmployeeDeleteBlocker struct {
	Type      DigitalEmployeeDeleteBlockerType
	ID        uuid.UUID
	Status    string
	Title     string
	RunID     *uuid.UUID
	ProjectID *uuid.UUID
}

type DigitalEmployeeDeleteBlockedError struct {
	Blockers []DigitalEmployeeDeleteBlocker
}

func (e *DigitalEmployeeDeleteBlockedError) Error() string {
	return ErrDigitalEmployeeDeleteBlocked.Error()
}

func (e *DigitalEmployeeDeleteBlockedError) Unwrap() error {
	return ErrDigitalEmployeeDeleteBlocked
}

type SoftDeleteDigitalEmployeeCascadeParams struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	DeletedAt         time.Time
}

type DigitalEmployeeDeleteCascadeResult struct {
	ExecutionInstances   int64
	EnvironmentVariables int64
	MCPBindingsV2        int64
	SkillBindings        int64
	ConfigRevisions      int64
	ProjectAffinities    int64
	MCPBindingV2IDs      []uuid.UUID
	SkillBindingIDs      []uuid.UUID
	ExecutionInstanceID  *uuid.UUID
	RuntimeNodeID        *uuid.UUID
	AgentHomeDir         string
	ProviderType         string
}

type DigitalEmployeeDeleteAuditEventParams struct {
	TenantID      uuid.UUID
	ActorUserID   uuid.UUID
	Employee      DigitalEmployeeRecord
	CascadeResult DigitalEmployeeDeleteCascadeResult
	DeletedAt     time.Time
}

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

type ConfigRevisionStatus string

const (
	ConfigRevisionStatusDraft  ConfigRevisionStatus = "draft"
	ConfigRevisionStatusActive ConfigRevisionStatus = "active"
)

type ValidationIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type EffectiveConfigValidation struct {
	BlockingErrors []ValidationIssue `json:"blocking_errors"`
	Warnings       []ValidationIssue `json:"warnings"`
}

type ReadinessCheckStatus string

const (
	ReadinessCheckPassed  ReadinessCheckStatus = "passed"
	ReadinessCheckWarning ReadinessCheckStatus = "warning"
	ReadinessCheckBlocked ReadinessCheckStatus = "blocked"
	ReadinessCheckInfo    ReadinessCheckStatus = "info"
)

type SchedulingReadinessCheck struct {
	Code    string
	Status  ReadinessCheckStatus
	Label   string
	Message string
}

type SchedulingReadinessSkillSummary struct {
	PersonalCount   int
	InheritedCount  int
	MissingRequired []string
}

type SchedulingReadinessMCPSummary struct {
	PersonalCount  int
	InheritedCount int
}

type SchedulingReadinessEnvironmentSummary struct {
	ConfiguredCount int
	MissingNames    []string
}

type SchedulingReadinessCapabilities struct {
	Skills               SchedulingReadinessSkillSummary
	MCPServers           SchedulingReadinessMCPSummary
	EnvironmentVariables SchedulingReadinessEnvironmentSummary
}

type DigitalEmployeeSchedulingReadiness struct {
	EmployeeID                uuid.UUID
	Status                    DigitalEmployeeStatus
	ReadyForProjectScheduling bool
	Checks                    []SchedulingReadinessCheck
	Capabilities              SchedulingReadinessCapabilities
	ProjectExecutionSource    string
}

type SchedulingCapabilityFacts struct {
	PersonalSkillCount      int
	InheritedSkillCount     int
	MissingRequiredSkills   []string
	PersonalMCPServerCount  int
	InheritedMCPServerCount int
	ConfiguredEnvVarCount   int
	MissingEnvironmentNames []string
}

type TeamBaseline struct {
	Constitution map[string]any
	Skills       []string
	MCPServers   []string
}

type DigitalEmployee struct {
	ID                    uuid.UUID
	TenantID              uuid.UUID
	TeamID                *uuid.UUID
	OwnerUserID           uuid.UUID
	EmployeeType          string
	ProviderType          string
	Name                  string
	Role                  string
	Description           *string
	Status                DigitalEmployeeStatus
	PermissionPolicy      map[string]any
	ContextPolicy         map[string]any
	ApprovalPolicy        map[string]any
	RiskLevel             string
	Metadata              map[string]any
	PersonaMemoryMarkdown string
	CapabilityBindings    map[string]any
	BudgetPolicy          map[string]any
	DisabledAt            *time.Time
	ArchivedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type EnvironmentVariableStatus string

const (
	EnvironmentVariableStatusActive   EnvironmentVariableStatus = "active"
	EnvironmentVariableStatusDisabled EnvironmentVariableStatus = "disabled"
)

type EnvironmentVariableSummary struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            *uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	Configured        bool
	Fingerprint       string
	Sensitive         bool
	Status            EnvironmentVariableStatus
	UpdatedAt         time.Time
}

type EnvironmentVariableRecord struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            *uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	EncryptedValue    string
	EncryptionKeyID   string
	ValueFingerprint  string
	Sensitive         bool
	Status            EnvironmentVariableStatus
	CreatedBy         *uuid.UUID
	UpdatedBy         *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ListEnvironmentVariablesRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type UpsertEnvironmentVariableRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	Value             string
	Sensitive         bool
	ActorUserID       *uuid.UUID
}

type DeleteEnvironmentVariableRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
}

// InitialEnvironmentVariable is used by the digital-employee create flow to save
// env vars at create time. Plaintext lives only in the request; the service
// encrypts before any write.
type InitialEnvironmentVariable struct {
	Name      string
	Value     string
	Sensitive bool
}

// RuntimeEnvironmentVariablePayload is the decrypted shape handed to the run
// service for the Runtime command payload. Control Plane decrypts; Runtime
// Agent never sees ciphertext or keys.
type RuntimeEnvironmentVariablePayload struct {
	Name      string
	Value     string
	Sensitive bool
}

// RuntimeMCPServerPayload is one effective MCP server projected to the Runtime Agent. It
// carries env-var names (not values); the Runtime materializes provider config that
// references these names. Only env-satisfied bindings are projected.
type RuntimeMCPServerPayload struct {
	ServerID         string
	ServerKey        string
	Name             string
	Transport        string
	URL              string
	AuthStrategy     string
	CredentialEnvVar string
	RequiredEnvVars  []string
	HeadersEnv       map[string]string
	SourceScope      string
	PermissionScope  map[string]any
}

// SkillMCPDependencyRecord is the run-service projection of a skill's MCP dependency.
// MCPServerID is a string to compare directly against RuntimeMCPServerPayload.ServerID.
type SkillMCPDependencyRecord struct {
	SkillID     uuid.UUID
	MCPServerID string
	ServerKey   string
}

type EmployeeTypeDefinition struct {
	Type                     string
	Label                    string
	Description              string
	DefaultRole              string
	RecommendedSkills        []string
	RecommendedMCPServers    []string
	RecommendedProviderTypes []string
	PersonaMemoryMarkdown    string
	CapabilityBindings       map[string]any
	BudgetPolicy             map[string]any
	DefaultApprovalPolicy    map[string]any
	Metadata                 map[string]any
}

type TeamConfigInput struct {
	ID                          uuid.UUID
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
}

type TeamConfigCreateOption struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	TeamID       *uuid.UUID
	Constitution map[string]any
	Skills       []string
	MCPServers   []string
}

type CapabilityOptions struct {
	ProviderTypes []string
	Skills        []CapabilityOptionItem
	MCPServers    []CapabilityOptionItem
}

// CapabilityOptionItem is one selectable capability candidate for employee
// creation. Key is the skill slug or MCP server key; Available marks whether
// the key exists in the tenant registry (template-recommended keys missing
// from the registry are returned with Available=false and cannot be bound).
type CapabilityOptionItem struct {
	Key         string
	ID          *uuid.UUID
	Label       string
	Description string
	Recommended bool
	Available   bool
	RiskLevel   string
}

// CapabilityRegistryOption is a bindable capability from the tenant registry
// (skills table or mcp_servers table).
type CapabilityRegistryOption struct {
	ID          uuid.UUID
	Key         string
	Label       string
	Description string
	RiskLevel   string
}

type RuntimeProviderOption struct {
	RuntimeNodeID         uuid.UUID
	NodeID                string
	RuntimeName           string
	ProviderType          string
	RuntimeStatus         string
	ProviderStatus        string
	HealthStatus          string
	CurrentLoad           int32
	MaxSlots              int32
	AgentHomeDir          string
	AgentHomeDirAvailable bool
	Available             bool
	DisabledReason        string
}

type CreateOptionCheck struct {
	Key     string
	Label   string
	Status  string
	Message string
}

type PolicyDefaults struct {
	PermissionPolicy map[string]any
	ApprovalPolicy   map[string]any
	WorkspacePolicy  map[string]any
	SessionPolicy    map[string]any
	Metadata         map[string]any
}

type BudgetPolicy struct {
	DailyTokenLimit *int32
}

type EmployeeConfigInput struct {
	ID                    uuid.UUID
	TenantID              uuid.UUID
	DigitalEmployeeID     uuid.UUID
	RevisionNumber        int32
	PersonaMemoryMarkdown string
	CapabilityBindings    map[string]any
	BudgetPolicy          map[string]any
}

type DigitalEmployeeConfigRevision struct {
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

type EffectiveConfigPreview struct {
	EmployeeConfigRevisionID uuid.UUID
	EffectiveConfig          map[string]any
	Validation               EffectiveConfigValidation
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

type RuntimeProvisioningPreflight struct {
	TenantID              uuid.UUID
	TeamID                uuid.UUID
	RuntimeNodeID         uuid.UUID
	NodeID                string
	AgentHomeDir          string
	GovernanceSnapshot    map[string]any
	HasActiveTeamConfig   bool
	RuntimeOnline         bool
	EnrollmentApproved    bool
	RuntimeSessionActive  bool
	ProviderAvailable     bool
	ProviderPolicyAllowed bool
	RuntimePolicyAllowed  bool
}

type CreateOptionsRequest struct {
	TenantID uuid.UUID
	TeamID   *uuid.UUID
}

type CreateOptions struct {
	TeamConfig             TeamConfigCreateOption
	EmployeeTypes          []EmployeeTypeDefinition
	CapabilityOptions      CapabilityOptions
	RuntimeProviderOptions []RuntimeProviderOption
	CreationChecks         []CreateOptionCheck
	PolicyDefaults         PolicyDefaults
}

type CreateDigitalEmployeeRequest struct {
	TenantID              uuid.UUID
	TeamID                *uuid.UUID
	OwnerUserID           uuid.UUID
	EmployeeType          string
	Name                  string
	AvatarAssetID         string
	Role                  string
	Description           *string
	PermissionPolicy      map[string]any
	ContextPolicy         map[string]any
	ApprovalPolicy        map[string]any
	RiskLevel             string
	Metadata              map[string]any
	PersonaMemoryMarkdown string
	Skills                []string
	MCPServers            []string
	CapabilityBindings    map[string]any
	BudgetPolicy          map[string]any
	ProviderType          string
	EnvironmentVariables  []InitialEnvironmentVariable
}

type CreateDigitalEmployeeConfigRevisionRequest struct {
	TenantID              uuid.UUID
	DigitalEmployeeID     uuid.UUID
	PersonaMemoryMarkdown *string
	CapabilityBindings    map[string]any
	BudgetPolicy          map[string]any
	Status                ConfigRevisionStatus
	ApprovedBy            *uuid.UUID
}

type PreviewEffectiveConfigRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	TeamConfig        TeamConfigInput
	EmployeeConfig    EmployeeConfigInput
}

type ListDigitalEmployeesRequest struct {
	TenantID   uuid.UUID
	TeamID     *uuid.UUID
	Status     DigitalEmployeeStatus
	Assignment string
	Offset     int32
	Limit      int32
}

type OverviewExecutionStatus string

const (
	OverviewExecutionStatusMissing      OverviewExecutionStatus = "missing"
	OverviewExecutionStatusProvisioning OverviewExecutionStatus = "provisioning"
	OverviewExecutionStatusReady        OverviewExecutionStatus = "ready"
	OverviewExecutionStatusActive       OverviewExecutionStatus = "active"
	OverviewExecutionStatusDisabled     OverviewExecutionStatus = "disabled"
	OverviewExecutionStatusError        OverviewExecutionStatus = "error"
)

func (s OverviewExecutionStatus) IsValid() bool {
	switch s {
	case "", OverviewExecutionStatusMissing, OverviewExecutionStatusProvisioning, OverviewExecutionStatusReady, OverviewExecutionStatusActive, OverviewExecutionStatusDisabled, OverviewExecutionStatusError:
		return true
	default:
		return false
	}
}

type OverviewRunStatus string

const (
	OverviewRunStatusNone        OverviewRunStatus = "none"
	OverviewRunStatusQueued      OverviewRunStatus = "queued"
	OverviewRunStatusDispatching OverviewRunStatus = "dispatching"
	OverviewRunStatusRunning     OverviewRunStatus = "running"
	OverviewRunStatusCancelling  OverviewRunStatus = "cancelling"
	OverviewRunStatusCompleted   OverviewRunStatus = "completed"
	OverviewRunStatusFailed      OverviewRunStatus = "failed"
	OverviewRunStatusCancelled   OverviewRunStatus = "cancelled"
	OverviewRunStatusTimedOut    OverviewRunStatus = "timed_out"
)

func (s OverviewRunStatus) IsValid() bool {
	switch s {
	case "", OverviewRunStatusNone, OverviewRunStatusQueued, OverviewRunStatusDispatching, OverviewRunStatusRunning, OverviewRunStatusCancelling, OverviewRunStatusCompleted, OverviewRunStatusFailed, OverviewRunStatusCancelled, OverviewRunStatusTimedOut:
		return true
	default:
		return false
	}
}

type WorkbenchStatus string

const (
	WorkbenchStatusReady          WorkbenchStatus = "ready"
	WorkbenchStatusPendingBinding WorkbenchStatus = "pending_binding"
	WorkbenchStatusError          WorkbenchStatus = "error"
)

type GetDigitalEmployeeOverviewRequest struct {
	TenantID        uuid.UUID
	Query           string
	TeamID          *uuid.UUID
	Status          DigitalEmployeeStatus
	EmployeeType    string
	ProviderType    string
	RuntimeNodeID   *uuid.UUID
	RiskLevel       string
	ExecutionStatus OverviewExecutionStatus
	RunStatus       OverviewRunStatus
	// OperationalStatus 按计算态运行状态过滤（如轮播只拉非空闲员工）。
	// 状态在 operational facts 上由 Go 状态机裁决后回填为 ID 过滤条件，非 SQL 直接判定。
	OperationalStatus []DigitalEmployeeOperationalStatus
	Offset            int32
	Limit             int32
}

type DigitalEmployeeOverview struct {
	Summary      DigitalEmployeeOverviewSummary
	QueueSummary DigitalEmployeeOverviewQueueSummary
	Items        []DigitalEmployeeOverviewItem
	Filters      DigitalEmployeeOverviewFilters
	Pagination   OverviewPagination
}

type DigitalEmployeeOverviewSummary struct {
	TotalCount                 int32
	RunnableCount              int32
	RunningCount               int32
	WaitingRuntimeCount        int32
	ErrorCount                 int32
	HighRiskCount              int32
	ReadyCount                 int32
	PendingRuntimeBindingCount int32
	PendingConfigApprovalCount int32
	FailedRecentRunCount       int32
	OperationalStatusCounts    map[DigitalEmployeeOperationalStatus]int32
}

type DigitalEmployeeOverviewQueueSummary struct {
	PendingRuntimeBindingCount int32
	StaleConfigCount           int32
	FailedRecentRunCount       int32
}

type DigitalEmployeeOverviewItem struct {
	IdentitySummary   DigitalEmployeeIdentitySummary
	ExecutionSummary  DigitalEmployeeExecutionSummary
	LatestRunSummary  *DigitalEmployeeLatestRunSummary
	GovernanceSummary DigitalEmployeeGovernanceSummary
	BudgetSummary     DigitalEmployeeBudgetSummary
	WorkbenchStatus   WorkbenchStatus
	OperationalState  DigitalEmployeeOperationalState
	RecentEvents      []DigitalEmployeeRecentEventSummary
	ProjectSummary    DigitalEmployeeProjectSummary
}

// DigitalEmployeeProjectSummary 描述数字员工与项目的关联情况：
// 活跃成员身份（project_members）或任务分派（project_tasks）都算关联。
type DigitalEmployeeProjectSummary struct {
	ProjectCount int32
	Projects     []DigitalEmployeeProjectLinkSummary
}

type DigitalEmployeeProjectLinkSummary struct {
	ProjectID        uuid.UUID
	Name             string
	Status           string
	IsMember         bool
	ActiveTaskCount  int32
	WorkingTaskCount int32
	TotalTaskCount   int32
	LastActivityAt   *time.Time
}

type DigitalEmployeeIdentitySummary struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            *uuid.UUID
	TeamName          string
	OwnerUserID       uuid.UUID
	OwnerDisplayName  string
	EmployeeType      string
	EmployeeTypeLabel string
	Name              string
	Role              string
	Description       *string
	Status            DigitalEmployeeStatus
	RiskLevel         string
	AvatarAsset       *DigitalEmployeeAvatarAsset
}

type DigitalEmployeeExecutionSummary struct {
	ExecutionInstanceID   *uuid.UUID
	Status                OverviewExecutionStatus
	RuntimeNodeID         *uuid.UUID
	NodeID                string
	RuntimeName           string
	RuntimeStatus         string
	ProviderType          string
	ProviderStatus        string
	HealthStatus          string
	AgentHomeDirAvailable bool
}

type DigitalEmployeeLatestRunSummary struct {
	RunID        uuid.UUID
	TaskID       uuid.UUID
	Status       OverviewRunStatus
	Title        string
	StartedAt    *time.Time
	UpdatedAt    *time.Time
	FinishedAt   *time.Time
	DurationSec  *int32
	TokenUsage   *int32
	ErrorMessage string
}

type DigitalEmployeeGovernanceSummary struct {
	EffectiveConfigID      *uuid.UUID
	Status                 string
	TeamRevisionNumber     *int32
	EmployeeRevisionNumber *int32
	SkillsCount            int32
	MCPServersCount        int32
	ConstitutionRef        string
}

type DigitalEmployeeBudgetSummary struct {
	DailyTokenLimit   *int32
	UsageTokensToday  int32
	UsagePercentToday *int32
	LimitExceeded     bool
	UsageTokens30d    *int32
	RunCount30d       int32
	CostAmount30d     *float64
	Currency          string
	Source            string
}

type DigitalEmployeeRecentEventSummary struct {
	Label      string
	Status     string
	OccurredAt *time.Time
}

type DigitalEmployeeOverviewFilters struct {
	Teams             []OverviewFilterOption
	Statuses          []OverviewFilterOption
	EmployeeTypes     []OverviewFilterOption
	Providers         []OverviewFilterOption
	RuntimeNodes      []OverviewFilterOption
	RiskLevels        []OverviewFilterOption
	ExecutionStatuses []OverviewFilterOption
	RunStatuses       []OverviewFilterOption
}

type OverviewFilterOption struct {
	Value string
	Label string
}

type OverviewPagination struct {
	Limit      int32
	Offset     int32
	TotalCount int32
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
