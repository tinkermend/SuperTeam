package employee

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput            = errors.New("invalid employee input")
	ErrNotFound                = errors.New("employee not found")
	ErrConflict                = errors.New("employee conflict")
	ErrRuntimeUnavailable      = errors.New("employee runtime unavailable")
	ErrProviderUnavailable     = errors.New("employee provider unavailable")
	ErrRuntimeIdentityMismatch = errors.New("employee runtime identity mismatch")
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
	ContextPolicy    map[string]any
	ApprovalPolicy   map[string]any
	RiskLevel        string
	Metadata         map[string]any
	DisabledAt       *time.Time
	ArchivedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type WorkspaceFile struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            *uuid.UUID
	DigitalEmployeeID uuid.UUID
	Path              string
	FileRole          string
	MimeType          string
	SyncPolicy        string
	Status            string
	CurrentRevisionID uuid.UUID
	RevisionNumber    int32
	Content           string
	ContentHash       string
	SizeBytes         int32
	StorageBackend    string
	ObjectKey         *string
	CreatedBy         *uuid.UUID
	ChangeNote        *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
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

type ListWorkspaceFilesRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type UpsertWorkspaceFileRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	Path              string
	Content           string
	FileRole          string
	MimeType          string
	SyncPolicy        string
	ChangeNote        *string
	UpdatedBy         *uuid.UUID
}

type UpsertWorkspaceFileStoreRequest struct {
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Path              string
	FileRole          string
	MimeType          string
	SyncPolicy        string
	Content           string
	ContentHash       string
	SizeBytes         int32
	ChangeNote        *string
	UpdatedBy         *uuid.UUID
}

type EmployeeTypeDefinition struct {
	Type                         string
	Label                        string
	Description                  string
	DefaultRole                  string
	RecommendedSkills            []string
	RecommendedMCPServers        []string
	RecommendedProviderTypes     []string
	DefaultCapabilitySelection   map[string]any
	DefaultContextPolicyOverride map[string]any
	DefaultApprovalPolicy        map[string]any
	Metadata                     map[string]any
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
	Skills        []string
	MCPServers    []string
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
	PermissionPolicy      map[string]any
	ContextPolicyOverride map[string]any
	ApprovalPolicy        map[string]any
	CapabilitySelection   map[string]any
	RuntimeSelector       map[string]any
	WorkspacePolicy       map[string]any
	SessionPolicy         map[string]any
	Metadata              map[string]any
}

type BudgetPolicy struct {
	DailyTokenLimit *int32
}

type EmployeeConfigInput struct {
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
}

type DigitalEmployeeConfigRevision struct {
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
	TenantID               uuid.UUID
	TeamID                 *uuid.UUID
	OwnerUserID            uuid.UUID
	EmployeeType           string
	Name                   string
	AvatarAssetID          string
	Role                   string
	Description            *string
	PermissionPolicy       map[string]any
	ContextPolicy          map[string]any
	ApprovalPolicy         map[string]any
	RiskLevel              string
	Metadata               map[string]any
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	BudgetPolicy           map[string]any
	OutputContractAddendum map[string]any
	RuntimeNodeID          uuid.UUID
	ProviderType           string
	SessionPolicy          map[string]any
	WorkspacePolicy        map[string]any
	EnvironmentVariables   []InitialEnvironmentVariable
}

type CreateDigitalEmployeeConfigRevisionRequest struct {
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	BudgetPolicy           map[string]any
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
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
	Offset          int32
	Limit           int32
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
