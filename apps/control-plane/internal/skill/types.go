package skill

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type SkillStatus string

const (
	SkillStatusInstalled SkillStatus = "installed"
	SkillStatusAvailable SkillStatus = "available"
)

type SkillFileType string

const (
	SkillFileTypeFile SkillFileType = "file"
)

var (
	ErrInvalidInput = errors.New("invalid skill input")
	ErrNotFound     = errors.New("skill not found")
)

type Skill struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	Slug          string
	Name          string
	Description   string
	Version       string
	Source        string
	RiskLevel     string
	Status        SkillStatus
	IconKey       string
	ColorToken    string
	Tags          []string
	TeamIDs       []uuid.UUID
	Files         []*SkillFile
	TeamBindings  []*SkillTeamBinding
	AgentBindings []*SkillAgentBinding
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SkillFile struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	SkillID        uuid.UUID
	Path           string
	FileType       SkillFileType
	Content        string
	SizeBytes      int64
	ChecksumSHA256 string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type SkillTeamBinding struct {
	TeamID   uuid.UUID
	TeamName string
}

type SkillAgentBinding struct {
	AgentID   uuid.UUID
	AgentName string
	TeamID    *uuid.UUID
	TeamName  string
	Status    string
}

type ListSkillsRequest struct {
	TenantID uuid.UUID
	Status   SkillStatus
	Q        string
}

type GetSkillRequest struct {
	TenantID uuid.UUID
	SkillID  uuid.UUID
}

type UploadSkillRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	Name        string
	Description string
	Tags        []string
	TeamIDs     []uuid.UUID
	RiskLevel   string
	Archive     []byte
	Filename    string
}

type UpsertSkillPackageRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	Slug        string
	Name        string
	Description string
	Version     string
	Source      string
	RiskLevel   string
	Status      SkillStatus
	IconKey     string
	ColorToken  string
	Tags        []string
	TeamIDs     []uuid.UUID
	Files       []*SkillFile
}

type UpdateSkillFileRequest struct {
	TenantID uuid.UUID
	SkillID  uuid.UUID
	Path     string
	Content  string
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
