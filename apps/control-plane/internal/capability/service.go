package capability

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

type Repository interface {
	CreateCredential(ctx context.Context, req CreateCredentialStoreRequest) (Credential, error)
	ListCredentials(ctx context.Context, req ListCredentialsRequest) ([]Credential, error)
	GetCredential(ctx context.Context, req ResolveCredentialRequest) (Credential, error)
	CreateTeamMCPServer(ctx context.Context, req CreateTeamMCPServerRequest) (MCPServer, error)
	ListTeamMCPServers(ctx context.Context, req TeamScopedRequest) ([]MCPServer, error)
	DeleteTeamMCPServer(ctx context.Context, req DeleteTeamMCPServerRequest) error
	CreateEmployeeMCPBinding(ctx context.Context, req CreateEmployeeMCPBindingRequest) (MCPServer, error)
	ListEmployeeMCPBindings(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error)
	DeleteEmployeeMCPBinding(ctx context.Context, req DeleteEmployeeMCPBindingRequest) error
	ListEffectiveMCPServers(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error)
}

type Service struct {
	repository Repository
	sealer     CredentialSealer
}

func NewService(repository Repository, sealer CredentialSealer) *Service {
	return &Service{repository: repository, sealer: sealer}
}

func (s *Service) CreateCredential(ctx context.Context, req CreateCredentialRequest) (Credential, error) {
	if err := s.requireRepository(); err != nil {
		return Credential{}, err
	}
	if err := s.requireSealer(); err != nil {
		return Credential{}, err
	}
	if err := validateCredentialRequest(req, true); err != nil {
		return Credential{}, err
	}

	req.Name = strings.TrimSpace(req.Name)
	sealed, err := s.sealer.Seal(req.CredentialValue)
	if err != nil {
		return Credential{}, err
	}
	created, err := s.repository.CreateCredential(ctx, CreateCredentialStoreRequest{
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		Name:           req.Name,
		CredentialType: req.CredentialType,
		EncryptedValue: sealed,
		LastFour:       lastFour(req.CredentialValue),
	})
	if err != nil {
		return Credential{}, err
	}
	return redactCredential(created), nil
}

func (s *Service) ListCredentials(ctx context.Context, req ListCredentialsRequest) ([]Credential, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if req.CredentialType != "" && req.CredentialType != CredentialTypeMCPToken {
		return nil, fmt.Errorf("%w: %s", ErrCredentialTypeInvalid, req.CredentialType)
	}
	credentials, err := s.repository.ListCredentials(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range credentials {
		credentials[i] = redactCredential(credentials[i])
	}
	return credentials, nil
}

func (s *Service) BuildMCPAuthorizationHeader(ctx context.Context, req ResolveCredentialRequest) (string, error) {
	if err := s.requireRepository(); err != nil {
		return "", err
	}
	if err := s.requireSealer(); err != nil {
		return "", err
	}
	if err := validateResolveCredentialRequest(req); err != nil {
		return "", err
	}
	credential, err := s.repository.GetCredential(ctx, req)
	if err != nil {
		return "", err
	}
	if credential.CredentialType != CredentialTypeMCPToken {
		return "", fmt.Errorf("%w: %s", ErrCredentialTypeInvalid, credential.CredentialType)
	}
	if err := validateCredentialActive(credential); err != nil {
		return "", err
	}
	plain, err := s.sealer.Open(credential.EncryptedValue)
	if err != nil {
		return "", err
	}
	return "Bearer " + plain, nil
}

func (s *Service) CreateTeamMCPServer(ctx context.Context, req CreateTeamMCPServerRequest) (MCPServer, error) {
	if err := s.requireRepository(); err != nil {
		return MCPServer{}, err
	}
	if err := validateMCPInput(req.TenantID, req.UserID, req.Name, req.URL, req.CredentialID); err != nil {
		return MCPServer{}, err
	}
	if req.TeamID == uuid.Nil {
		return MCPServer{}, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if err := s.validateMCPBindingCredential(ctx, req.TenantID, req.UserID, req.CredentialID); err != nil {
		return MCPServer{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	return s.repository.CreateTeamMCPServer(ctx, req)
}

func (s *Service) ListTeamMCPServers(ctx context.Context, req TeamScopedRequest) ([]MCPServer, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if err := validateTeamScopedRequest(req); err != nil {
		return nil, err
	}
	return s.repository.ListTeamMCPServers(ctx, req)
}

func (s *Service) DeleteTeamMCPServer(ctx context.Context, req DeleteTeamMCPServerRequest) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.ServerID == uuid.Nil {
		return fmt.Errorf("%w: server_id is required", ErrInvalidInput)
	}
	return s.repository.DeleteTeamMCPServer(ctx, req)
}

func (s *Service) CreateEmployeeMCPBinding(ctx context.Context, req CreateEmployeeMCPBindingRequest) (MCPServer, error) {
	if err := s.requireRepository(); err != nil {
		return MCPServer{}, err
	}
	if err := validateMCPInput(req.TenantID, req.UserID, req.Name, req.URL, req.CredentialID); err != nil {
		return MCPServer{}, err
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return MCPServer{}, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if err := s.validateMCPBindingCredential(ctx, req.TenantID, req.UserID, req.CredentialID); err != nil {
		return MCPServer{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	return s.repository.CreateEmployeeMCPBinding(ctx, req)
}

func (s *Service) ListEmployeeMCPBindings(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if err := validateEmployeeScopedRequest(req); err != nil {
		return nil, err
	}
	return s.repository.ListEmployeeMCPBindings(ctx, req)
}

func (s *Service) DeleteEmployeeMCPBinding(ctx context.Context, req DeleteEmployeeMCPBindingRequest) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.BindingID == uuid.Nil {
		return fmt.Errorf("%w: binding_id is required", ErrInvalidInput)
	}
	return s.repository.DeleteEmployeeMCPBinding(ctx, req)
}

func (s *Service) ListEffectiveMCPServers(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if err := validateEmployeeScopedRequest(req); err != nil {
		return nil, err
	}
	return s.repository.ListEffectiveMCPServers(ctx, req)
}

func (s *Service) requireRepository() error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("%w: capability repository is required", ErrInvalidInput)
	}
	return nil
}

func (s *Service) requireSealer() error {
	if s == nil || s.sealer == nil {
		return ErrCredentialKeyMissing
	}
	return nil
}

func (s *Service) validateMCPBindingCredential(ctx context.Context, tenantID, userID uuid.UUID, credentialID *uuid.UUID) error {
	if credentialID == nil {
		return nil
	}
	credential, err := s.repository.GetCredential(ctx, ResolveCredentialRequest{
		TenantID:     tenantID,
		UserID:       userID,
		CredentialID: *credentialID,
	})
	if err != nil {
		return err
	}
	if credential.CredentialType != CredentialTypeMCPToken {
		return fmt.Errorf("%w: %s", ErrCredentialTypeInvalid, credential.CredentialType)
	}
	return validateCredentialActive(credential)
}

func validateCredentialActive(credential Credential) error {
	if credential.Status != "active" || !credential.DisabledAt.IsZero() {
		return fmt.Errorf("%w: credential is not active", ErrInvalidInput)
	}
	return nil
}

func validateCredentialRequest(req CreateCredentialRequest, requireValue bool) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: credential name is required", ErrInvalidInput)
	}
	if req.CredentialType != CredentialTypeMCPToken {
		return fmt.Errorf("%w: %s", ErrCredentialTypeInvalid, req.CredentialType)
	}
	if requireValue && strings.TrimSpace(req.CredentialValue) == "" {
		return fmt.Errorf("%w: credential value is required", ErrInvalidInput)
	}
	return nil
}

func validateResolveCredentialRequest(req ResolveCredentialRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if req.CredentialID == uuid.Nil {
		return fmt.Errorf("%w: credential_id is required", ErrInvalidInput)
	}
	return nil
}

func validateMCPInput(tenantID, userID uuid.UUID, name, rawURL string, credentialID *uuid.UUID) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if userID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: mcp server name is required", ErrInvalidInput)
	}
	trimmedURL := strings.TrimSpace(rawURL)
	if trimmedURL == "" {
		return fmt.Errorf("%w: mcp server url is required", ErrInvalidInput)
	}
	parsedURL, err := url.Parse(trimmedURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("%w: mcp server url must include http(s) scheme and host", ErrInvalidInput)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("%w: mcp server url scheme must be http or https", ErrInvalidInput)
	}
	if credentialID != nil && *credentialID == uuid.Nil {
		return fmt.Errorf("%w: credential_id is required", ErrInvalidInput)
	}
	return nil
}

func validateTeamScopedRequest(req TeamScopedRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	return nil
}

func validateEmployeeScopedRequest(req EmployeeScopedRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	return nil
}

func redactCredential(credential Credential) Credential {
	credential.EncryptedValue = ""
	return credential
}

func lastFour(value string) string {
	runes := []rune(value)
	if len(runes) <= 4 {
		return value
	}
	return string(runes[len(runes)-4:])
}
