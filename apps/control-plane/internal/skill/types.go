package skill

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput         = errors.New("invalid skill input")
	ErrNotFound             = errors.New("skill not found")
	ErrTeamAlreadyInherited = errors.New("team already inherited this skill")
)

type Skill struct {
	ID                  uuid.UUID
	TenantID            uuid.UUID
	Slug                string
	Name                string
	Description         string
	Version             string
	Source              string
	RiskLevel           string
	IconKey             string
	ColorToken          string
	Tags                []string
	TeamIDs             []uuid.UUID
	Metadata            map[string]any
	RuntimeDependencies SkillRuntimeDependencies
	ArchiveObjectRef    string
	ArchiveFilename     string
	ArchiveSizeBytes    int64
	ArchiveChecksum     string
	ArchiveFileCount    int
	CreatedBy           uuid.UUID
	CreatedByName       string
	TeamBindings        []*SkillTeamBinding
	AgentBindings       []*SkillAgentBinding
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type SkillTeamBinding struct {
	TeamID   uuid.UUID
	TeamName string
}

type SkillAgentBinding struct {
	AgentID                 uuid.UUID
	AgentName               string
	TeamID                  *uuid.UUID
	TeamName                string
	Status                  string
	RuntimeDependencyStatus *SkillRuntimeDependencyStatus
}

type SkillRuntimeRecord struct {
	ID                  uuid.UUID
	Slug                string
	Metadata            map[string]any
	RuntimeDependencies SkillRuntimeDependencies
	ArchiveObjectRef    string
	ArchiveChecksum     string
	ArchiveSizeBytes    int64
	ArchiveFileCount    int
}

type SkillRuntimeDependencyStatus struct {
	LoadStatus   string   `json:"load_status"`
	MissingTools []string `json:"missing_tools"`
	MissingEnv   []string `json:"missing_env"`
}

type RuntimeDependencies struct {
	Tools []string `json:"tools,omitempty"`
	Env   []string `json:"env,omitempty"`
}

type SkillRuntimeDependencies = RuntimeDependencies

type ListSkillsRequest struct {
	TenantID uuid.UUID
	Q        string
}

type GetSkillRequest struct {
	TenantID uuid.UUID
	SkillID  uuid.UUID
}

type DeleteSkillRequest struct {
	TenantID uuid.UUID
	SkillID  uuid.UUID
}

type UploadSkillRequest struct {
	TenantID            uuid.UUID
	ActorUserID         uuid.UUID
	Name                string
	Description         string
	Tags                []string
	TeamIDs             []uuid.UUID
	RiskLevel           string
	RuntimeDependencies SkillRuntimeDependencies
	Archive             []byte
	Filename            string
}

type UpsertSkillPackageRequest struct {
	TenantID            uuid.UUID
	ActorUserID         uuid.UUID
	Slug                string
	Name                string
	Description         string
	Version             string
	Source              string
	RiskLevel           string
	IconKey             string
	ColorToken          string
	Tags                []string
	TeamIDs             []uuid.UUID
	Metadata            map[string]any
	RuntimeDependencies SkillRuntimeDependencies
	ArchiveObjectRef    string
	ArchiveFilename     string
	ArchiveSizeBytes    int64
	ArchiveChecksum     string
	ArchiveFileCount    int
}

type BindTeamSkillRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	SkillID  uuid.UUID
}

type ListTeamSkillsRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
}

type BindEmployeeSkillRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	SkillID           uuid.UUID
}

type ListEffectiveEmployeeSkillsRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type EffectiveEmployeeSkill struct {
	Skill       Skill
	SourceScope string
	Inherited   bool
	ReadOnly    bool
}

type SkillInstallTargetScope string

const (
	SkillInstallTargetTeam     SkillInstallTargetScope = "team"
	SkillInstallTargetEmployee SkillInstallTargetScope = "employee"
)

type InstallFailurePhase string

const (
	InstallFailurePhasePreflight      InstallFailurePhase = "preflight"
	InstallFailurePhaseRuntimeInstall InstallFailurePhase = "runtime_install"
	InstallFailurePhaseTimeout        InstallFailurePhase = "timeout"
)

type InstallSkillRequest struct {
	TenantID          uuid.UUID
	SkillID           uuid.UUID
	TargetScope       SkillInstallTargetScope
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	ActorUserID       uuid.UUID
	Timeout           time.Duration
}

type SkillInstallTarget struct {
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	EmployeeName      string
	RuntimeNodeID     uuid.UUID
	NodeID            string
	ProviderType      string
	AgentHomeDir      string
}

type SkillInstallation struct {
	ID                    uuid.UUID
	TenantID              uuid.UUID
	SkillID               uuid.UUID
	TargetScope           SkillInstallTargetScope
	TeamID                uuid.UUID
	DigitalEmployeeID     uuid.UUID
	EmployeeName          string
	RuntimeNodeID         uuid.UUID
	NodeID                string
	ProviderType          string
	InstalledPath         string
	ArchiveChecksumSHA256 string
	InstalledBy           uuid.UUID
	InstalledAt           time.Time
	Metadata              map[string]any
}

type InstallSkillResult struct {
	SkillID           uuid.UUID
	TargetScope       SkillInstallTargetScope
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	InstalledCount    int
	Installations     []SkillInstallation
	BlockedTargets    []SkillInstallBlockedTarget
}

type SkillInstallBlockedTarget struct {
	DigitalEmployeeID uuid.UUID `json:"digital_employee_id"`
	EmployeeName      string    `json:"employee_name,omitempty"`
	ProviderType      string    `json:"provider_type,omitempty"`
	RuntimeNodeID     uuid.UUID `json:"runtime_node_id,omitempty"`
	NodeID            string    `json:"node_id,omitempty"`
	ReasonCode        string    `json:"reason_code"`
	Message           string    `json:"message"`
}

type InstallSkillError struct {
	Phase          InstallFailurePhase
	Message        string
	BlockedTargets []SkillInstallBlockedTarget
}

func (e *InstallSkillError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type ListSkillInstallTargetsRequest struct {
	TenantID          uuid.UUID
	SkillID           uuid.UUID
	TargetScope       SkillInstallTargetScope
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type CreateSkillInstallCommandReceiptRequest struct {
	TenantID      uuid.UUID
	CommandID     string
	CommandType   string
	RuntimeNodeID uuid.UUID
	NodeID        string
	ResourceID    uuid.UUID
	Payload       map[string]any
}

type RuntimeInstallCommandReceipt struct {
	CommandID     string
	Status        string
	RuntimeNodeID uuid.UUID
	NodeID        string
	Payload       map[string]any
	Result        map[string]any
	ErrorMessage  string
}

type PersistSkillInstallSuccessRequest struct {
	TenantID      uuid.UUID
	SkillID       uuid.UUID
	TargetScope   SkillInstallTargetScope
	TeamID        uuid.UUID
	InstalledBy   uuid.UUID
	Installations []SkillInstallation
}

type SkillInstallFailureLog struct {
	TenantID          uuid.UUID
	SkillID           uuid.UUID
	TargetScope       SkillInstallTargetScope
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	RuntimeNodeID     uuid.UUID
	ProviderType      string
	Phase             InstallFailurePhase
	ReasonCode        string
	Message           string
	CommandID         string
	Details           map[string]any
}
