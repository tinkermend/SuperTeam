package capability

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput         = errors.New("invalid capability input")
	ErrNotFound             = errors.New("capability not found")
	ErrCredentialKeyMissing = errors.New("credential encryption key is required")
	ErrConflict             = errors.New("conflict")
)

type TeamScopedRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	TeamID   uuid.UUID
}

type EmployeeScopedRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
}

// ----------------------------------------------------------------------------
// MCP HTTP capability registry (migration 037)
// ----------------------------------------------------------------------------

type MCPTransport string

const (
	MCPTransportStreamableHTTP MCPTransport = "streamable_http"
	MCPTransportHTTP           MCPTransport = "http"
)

type MCPAuthStrategy string

const (
	MCPAuthStrategyNone       MCPAuthStrategy = "none"
	MCPAuthStrategyBearerEnv  MCPAuthStrategy = "bearer_env"
	MCPAuthStrategyHeadersEnv MCPAuthStrategy = "headers_env"
)

// MCPBindingStatusBlockedMissingEnv is a derived (not stored) preflight status that marks a
// binding whose MCP requires env vars the target employee has not configured; Runtime
// projection excludes blocked bindings. 无存储 status（迁移 087 删除禁用生命周期）。
const (
	MCPBindingStatusActive            = "active"
	MCPBindingStatusBlockedMissingEnv = "blocked_missing_env"
)

// MCPDefinition is a tenant-level MCP HTTP capability definition.
type MCPDefinition struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	Name               string
	ServerKey          string
	Description        string
	Transport          MCPTransport
	URL                string
	AuthStrategy       MCPAuthStrategy
	RequiredEnvVars    []string
	OptionalEnvVars    []string
	ProviderVisibility map[string]bool
	ToolAllowlist      []string
	RiskLevel          string
	CreatedBy          *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// MCPBinding represents a team, employee or project binding to a registered MCP definition,
// enriched with the definition's projection fields and a preflight (MissingEnvVars) computed
// against the target employee's configured env vars where applicable. 项目绑定（迁移 072）
// 沿用同一行类型：team/employee/project 三个维度靠对应的 ID 指针与 SourceScope 区分，
// 不另设独立的 ProjectMCPBinding 结构（team 版也没有独立结构）。
type MCPBinding struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            *uuid.UUID
	DigitalEmployeeID *uuid.UUID
	ProjectID         *uuid.UUID
	MCPServerID       uuid.UUID
	CredentialEnvVar  string
	ServerName        string
	ServerKey         string
	URL               string
	Transport         MCPTransport
	AuthStrategy      MCPAuthStrategy
	RequiredEnvVars   []string
	RiskLevel         string
	SourceScope       string
	MissingEnvVars    []string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PreflightStatus returns the derived binding status accounting for missing env vars.
// 存储态 status 已随"禁用"生命周期一并删除（迁移 087）：绑定要么活着要么已删，
// 这里只剩 env 预检一种派生判定。
func (b MCPBinding) PreflightStatus() string {
	if len(b.MissingEnvVars) > 0 {
		return MCPBindingStatusBlockedMissingEnv
	}
	return MCPBindingStatusActive
}

// EffectiveMCPServer is one resolved effective MCP server for an employee, ready for runtime
// projection. Blocked (missing-env) entries are surfaced to Console but excluded from the
// Runtime payload by the caller.
type EffectiveMCPServer struct {
	ServerID         uuid.UUID
	ServerKey        string
	Name             string
	Transport        MCPTransport
	URL              string
	AuthStrategy     MCPAuthStrategy
	CredentialEnvVar string
	RequiredEnvVars  []string
	ToolAllowlist    []string
	RiskLevel        string
	SourceScope      string
	MissingEnvVars   []string
}

// BindingStatus reports active or blocked_missing_env for the effective server.
func (s EffectiveMCPServer) BindingStatus() string {
	if len(s.MissingEnvVars) > 0 {
		return MCPBindingStatusBlockedMissingEnv
	}
	return MCPBindingStatusActive
}

// SkillMCPDependency declares that a skill requires an MCP registry definition
// at load time. Validation-only: it never grants the MCP to an employee.
type SkillMCPDependency struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	SkillID      uuid.UUID
	MCPServerID  uuid.UUID
	Note         string
	CreatedAt    time.Time
	ServerKey    string
	ServerName   string
	AuthStrategy MCPAuthStrategy
	RiskLevel    string
}

// DependentSkill is a reverse lookup row: an active skill depending on an MCP definition.
type DependentSkill struct {
	SkillID uuid.UUID
	Slug    string
	Name    string
}

// SkillMCPDependencyInput is one desired dependency in a declarative replace.
type SkillMCPDependencyInput struct {
	MCPServerID uuid.UUID
	Note        string
}

type CreateMCPServerDefinitionRequest struct {
	TenantID           uuid.UUID
	UserID             uuid.UUID
	Name               string
	ServerKey          string
	Description        string
	Transport          MCPTransport
	URL                string
	AuthStrategy       MCPAuthStrategy
	RequiredEnvVars    []string
	OptionalEnvVars    []string
	ProviderVisibility map[string]bool
	ToolAllowlist      []string
	RiskLevel          string
}

type ListMCPServerDefinitionsRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
}

type DeleteMCPServerDefinitionRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	ServerID uuid.UUID
}

type CreateTeamMCPBindingRequest struct {
	TenantID         uuid.UUID
	TeamID           uuid.UUID
	UserID           uuid.UUID
	MCPServerID      uuid.UUID
	CredentialEnvVar string
}

type DeleteTeamMCPBindingRequest struct {
	TenantID  uuid.UUID
	TeamID    uuid.UUID
	UserID    uuid.UUID
	BindingID uuid.UUID
}

type ProjectScopedRequest struct {
	TenantID  uuid.UUID
	UserID    uuid.UUID
	ProjectID uuid.UUID
}

// ProjectMCPBindingInput is one desired project binding in a declarative replace.
type ProjectMCPBindingInput struct {
	MCPServerID      uuid.UUID
	CredentialEnvVar string
}

// PutProjectMCPBindingsRequest declaratively replaces a project's MCP bindings
// with the desired set（目录与能力投影修订 spec §3.2：项目公共 MCP 走注册表正门）.
type PutProjectMCPBindingsRequest struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	UserID    uuid.UUID
	Items     []ProjectMCPBindingInput
}

type ListSkillMCPDependenciesRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	SkillID  uuid.UUID
}

type ReplaceSkillMCPDependenciesRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	SkillID  uuid.UUID
	Items    []SkillMCPDependencyInput
}

type ListDependentSkillsRequest struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	ServerID uuid.UUID
}

type CreateEmployeeMCPBindingV2Request struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	UserID            uuid.UUID
	MCPServerID       uuid.UUID
	CredentialEnvVar  string
}

type DeleteEmployeeMCPBindingV2Request struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	UserID            uuid.UUID
	BindingID         uuid.UUID
}

// RuntimeSkillRef is a minimal skill reference (id + slug) surfaced by the skill module for
// runtime-facing consumers that only need identity, not the full skill record.
type RuntimeSkillRef struct {
	ID   uuid.UUID
	Slug string
}

type EvaluateEmployeeSkillMCPDependenciesRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
}

// EmployeeSkillMCPDependencyStatus groups the MCP dependency satisfaction status for one skill
// bound to a digital employee, for the employee panel data source.
type EmployeeSkillMCPDependencyStatus struct {
	SkillID      uuid.UUID
	SkillSlug    string
	Dependencies []EmployeeSkillMCPDependencyItem
}

// EmployeeSkillMCPDependencyItem is one skill -> MCP dependency evaluated against the
// employee's actual bindings and configured env vars.
type EmployeeSkillMCPDependencyItem struct {
	MCPServerID    uuid.UUID
	ServerKey      string
	ServerName     string
	Status         string // satisfied | missing_binding | blocked_missing_env
	MissingEnvVars []string
}
