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
