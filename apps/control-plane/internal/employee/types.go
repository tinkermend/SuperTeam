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
	ErrEffectiveConfigRequired = errors.New("employee effective config required")
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

type TeamConfigRevisionStatus string

const (
	TeamConfigRevisionStatusDraft  TeamConfigRevisionStatus = "draft"
	TeamConfigRevisionStatusActive TeamConfigRevisionStatus = "active"
)

type EffectiveConfigStatus string

const (
	EffectiveConfigStatusPendingApproval EffectiveConfigStatus = "pending_approval"
	EffectiveConfigStatusApproved        EffectiveConfigStatus = "approved"
	EffectiveConfigStatusRevoked         EffectiveConfigStatus = "revoked"
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

type DigitalEmployee struct {
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
	CreatedAt        time.Time
	UpdatedAt        time.Time
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
	RevisionNumber              int32
	Status                      TeamConfigRevisionStatus
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
}

type TeamConfigCreateOption struct {
	ID                          uuid.UUID
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
	RevisionNumber              int32
	Status                      TeamConfigRevisionStatus
	AllowedEmployeeTypes        []string
	AllowedProviderTypes        []string
	AllowedSkills               []string
	AllowedMCPServers           []string
	AllowedExternalCaps         []string
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
}

type CapabilityOptions struct {
	ProviderTypes        []string
	Skills               []string
	MCPServers           []string
	ExternalCapabilities []string
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
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
	ArchivedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type EffectiveConfigPreview struct {
	TeamConfigRevisionID     uuid.UUID
	EmployeeConfigRevisionID uuid.UUID
	EffectiveConfig          map[string]any
	Validation               EffectiveConfigValidation
}

type DigitalEmployeeEffectiveConfig struct {
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
	TeamID   uuid.UUID
}

type CreateOptions struct {
	TeamConfig             TeamConfigCreateOption
	EmployeeTypes          []EmployeeTypeDefinition
	CapabilityOptions      CapabilityOptions
	RuntimeProviderOptions []RuntimeProviderOption
	PolicyDefaults         PolicyDefaults
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
	RuntimeNodeID    uuid.UUID
	ProviderType     string
	SessionPolicy    map[string]any
	WorkspacePolicy  map[string]any
}

type CreateDigitalEmployeeConfigRevisionRequest struct {
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
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

type PreviewEffectiveConfigByRevisionIDsRequest struct {
	TenantID                 uuid.UUID
	DigitalEmployeeID        uuid.UUID
	TeamConfigRevisionID     uuid.UUID
	EmployeeConfigRevisionID uuid.UUID
}

type ApproveEffectiveConfigRequest struct {
	TenantID                 uuid.UUID
	DigitalEmployeeID        uuid.UUID
	TeamConfigRevisionID     uuid.UUID
	EmployeeConfigRevisionID uuid.UUID
	ApprovedBy               uuid.UUID
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
