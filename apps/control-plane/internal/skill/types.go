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
	ProjectBindings     []*SkillProjectBinding
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

// SkillProjectBinding is a project that has bound this skill (venue supply + venue filter).
type SkillProjectBinding struct {
	ProjectID   uuid.UUID
	ProjectName string
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
	SourceScope         string
	Version             string
}

// SkillRuntimeConflict records a same-slug multi-source resolution where the
// non-winning side was dropped. Source marks the winning side (e.g. project_binding).
type SkillRuntimeConflict struct {
	Slug            string
	WinningSkillID  uuid.UUID
	DroppedSkillID  uuid.UUID
	WinningSource   string
	DroppedSource   string
	Source          string // attestation marker, e.g. project_binding
}

// RuntimeSkillsResult is the control-plane projection of skills for one dispatch.
type RuntimeSkillsResult struct {
	Skills    []SkillRuntimeRecord
	Conflicts []SkillRuntimeConflict
}

type ProjectSkillBinding struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	SkillID         uuid.UUID
	CreatedByUserID *uuid.UUID
	CreatedAt       time.Time
	Skill           *Skill
}

type ListProjectSkillBindingsRequest struct {
	TenantID  uuid.UUID
	UserID    uuid.UUID
	ProjectID uuid.UUID
}

type PutProjectSkillBindingsRequest struct {
	TenantID  uuid.UUID
	UserID    uuid.UUID
	ProjectID uuid.UUID
	Items     []ProjectSkillBindingInput
}

type ProjectSkillBindingInput struct {
	SkillID uuid.UUID
}

type SkillRuntimeDependencyStatus struct {
	LoadStatus   string   `json:"load_status"`
	MissingTools []string `json:"missing_tools"`
	MissingEnv   []string `json:"missing_env"`
}

type RuntimeDependencies struct {
	Tools      []string                   `json:"tools"`
	Env        []string                   `json:"env"`
	MCPServers []SkillRuntimeMCPServerRef `json:"mcp_servers"`
}

// SkillRuntimeMCPServerRef is one skill→MCP dependency surfaced for bind-time preview
// and frontend closure calculation (capability supply three-layer §8.1).
type SkillRuntimeMCPServerRef struct {
	MCPServerID uuid.UUID `json:"mcp_server_id"`
	ServerKey   string    `json:"server_key"`
	ServerName  string    `json:"server_name"`
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
	// ActorUserID 用于团队审计事件的 actor；不参与业务判定，为空时审计记 uuid.Nil。
	ActorUserID uuid.UUID
}

// TeamSkillTakeover 团队接管某技能时，被物理收敛掉的成员个人绑定。
// 同一技能在团队与员工两个维度只留一份，团队胜出（spec §5.2.1）。
type TeamSkillTakeover struct {
	DigitalEmployeeID uuid.UUID
	EmployeeName      string
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

type InstallSkillRequest struct {
	TenantID          uuid.UUID
	SkillID           uuid.UUID
	TargetScope       SkillInstallTargetScope
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	ActorUserID       uuid.UUID
}

// InstallSkillResult reports the outcome of a logical skill bind.
// AlreadyBound is true when the target already had the skill (including via
// team inheritance for employee scope).
type InstallSkillResult struct {
	SkillID           uuid.UUID
	TargetScope       SkillInstallTargetScope
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	AlreadyBound      bool
	BoundAt           time.Time
}
