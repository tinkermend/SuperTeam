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
