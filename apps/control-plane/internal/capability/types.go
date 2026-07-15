package capability

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type CredentialType string

const (
	CredentialTypeMCPToken CredentialType = "mcp_token"
)

var (
	ErrInvalidInput          = errors.New("invalid capability input")
	ErrNotFound              = errors.New("capability not found")
	ErrCredentialKeyMissing  = errors.New("credential encryption key is required")
	ErrCredentialTypeInvalid = errors.New("invalid credential type")
	ErrConflict              = errors.New("conflict")
)

type Credential struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	UserID         uuid.UUID
	Name           string
	CredentialType CredentialType
	EncryptedValue string
	LastFour       string
	Status         string
	DisabledAt     time.Time
	DeletedAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type MCPServer struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	TeamID             *uuid.UUID
	DigitalEmployeeID  *uuid.UUID
	Name               string
	URL                string
	CredentialID       *uuid.UUID
	CredentialName     string
	CredentialType     CredentialType
	CredentialLastFour string
	Status             string
	SourceScope        string
	Inherited          bool
	CreatedBy          *uuid.UUID
	DisabledAt         time.Time
	DeletedAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CreateCredentialRequest struct {
	TenantID        uuid.UUID
	UserID          uuid.UUID
	Name            string
	CredentialType  CredentialType
	CredentialValue string
}

type CreateCredentialStoreRequest struct {
	TenantID       uuid.UUID
	UserID         uuid.UUID
	Name           string
	CredentialType CredentialType
	EncryptedValue string
	LastFour       string
}

type ListCredentialsRequest struct {
	TenantID       uuid.UUID
	UserID         uuid.UUID
	CredentialType CredentialType
}

type ResolveCredentialRequest struct {
	TenantID     uuid.UUID
	UserID       uuid.UUID
	CredentialID uuid.UUID
}

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

type CreateTeamMCPServerRequest struct {
	TenantID     uuid.UUID
	TeamID       uuid.UUID
	UserID       uuid.UUID
	Name         string
	URL          string
	CredentialID *uuid.UUID
}

type DeleteTeamMCPServerRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	ServerID uuid.UUID
}

type CreateEmployeeMCPBindingRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	UserID            uuid.UUID
	Name              string
	URL               string
	CredentialID      *uuid.UUID
}

type DeleteEmployeeMCPBindingRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	BindingID         uuid.UUID
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
// binding whose MCP requires env vars the target employee has not configured. The DB status
// stays "active"; Runtime projection excludes blocked bindings.
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
	Status             string
	CreatedBy          *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// MCPBinding represents a team or employee binding to a registered MCP definition, enriched
// with the definition's projection fields and a preflight (MissingEnvVars) computed against
// the target employee's configured env vars where applicable.
type MCPBinding struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            *uuid.UUID
	DigitalEmployeeID *uuid.UUID
	MCPServerID       uuid.UUID
	CredentialEnvVar  string
	Status            string
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
func (b MCPBinding) PreflightStatus() string {
	if len(b.MissingEnvVars) > 0 {
		return MCPBindingStatusBlockedMissingEnv
	}
	if b.Status == "" {
		return MCPBindingStatusActive
	}
	return b.Status
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
	ServerStatus string
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
