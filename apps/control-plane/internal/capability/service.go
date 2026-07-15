package capability

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// envNamePattern matches a POSIX-style environment variable name.
var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// serverKeyPattern matches the stable MCP key rendered into provider config (e.g. "github").
var serverKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

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

	// MCP HTTP capability registry (migration 037).
	CreateMCPServerDefinition(ctx context.Context, req CreateMCPServerDefinitionRequest) (MCPDefinition, error)
	ListMCPServerDefinitions(ctx context.Context, req ListMCPServerDefinitionsRequest) ([]MCPDefinition, error)
	GetMCPServerDefinition(ctx context.Context, tenantID, serverID uuid.UUID) (MCPDefinition, error)
	DeleteMCPServerDefinition(ctx context.Context, req DeleteMCPServerDefinitionRequest) error
	CreateTeamMCPBinding(ctx context.Context, req CreateTeamMCPBindingRequest) (MCPBinding, error)
	ListTeamMCPBindings(ctx context.Context, req TeamScopedRequest) ([]MCPBinding, error)
	DeleteTeamMCPBinding(ctx context.Context, req DeleteTeamMCPBindingRequest) error
	CreateEmployeeMCPBindingV2(ctx context.Context, req CreateEmployeeMCPBindingV2Request) (MCPBinding, error)
	ListEmployeeMCPBindingsV2(ctx context.Context, req EmployeeScopedRequest) ([]MCPBinding, error)
	DeleteEmployeeMCPBindingV2(ctx context.Context, req DeleteEmployeeMCPBindingV2Request) error
	ListEffectiveMCPBindingsV2(ctx context.Context, req EmployeeScopedRequest) ([]EffectiveMCPServer, error)
	ListConfiguredEmployeeEnvVarNames(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]string, error)

	// Skill <-> MCP registry dependency declarations (migration 062).
	SkillExistsForTenant(ctx context.Context, tenantID, skillID uuid.UUID) (bool, error)
	ListSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID) ([]SkillMCPDependency, error)
	ReplaceSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID, items []SkillMCPDependencyInput) ([]SkillMCPDependency, error)
	ListDependentSkills(ctx context.Context, tenantID, serverID uuid.UUID) ([]DependentSkill, error)
	ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error)
}

// EmployeeRuntimeSkillLister resolves the runtime-effective skill set for a digital employee.
// It is satisfied by an adapter over the skill module (see app.go) so this package does not
// import skill directly.
type EmployeeRuntimeSkillLister interface {
	ListEmployeeRuntimeSkillRefs(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeSkillRef, error)
}

type Service struct {
	repository                 Repository
	sealer                     CredentialSealer
	employeeRuntimeSkillLister EmployeeRuntimeSkillLister
}

func NewService(repository Repository, sealer CredentialSealer) *Service {
	return &Service{repository: repository, sealer: sealer}
}

// SetEmployeeRuntimeSkillLister wires the skill-module adapter used by
// EvaluateEmployeeSkillMCPDependencies. Optional: if unset, that method returns an empty result.
func (s *Service) SetEmployeeRuntimeSkillLister(l EmployeeRuntimeSkillLister) {
	s.employeeRuntimeSkillLister = l
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

// CreateMCPServerDefinition registers a tenant-level MCP HTTP definition after enforcing
// HTTP-only transport and valid env-var names.
func (s *Service) CreateMCPServerDefinition(ctx context.Context, req CreateMCPServerDefinitionRequest) (MCPDefinition, error) {
	if err := s.requireRepository(); err != nil {
		return MCPDefinition{}, err
	}
	if req.AuthStrategy == "" {
		req.AuthStrategy = MCPAuthStrategyNone
	}
	if strings.TrimSpace(req.RiskLevel) == "" {
		req.RiskLevel = "medium"
	}
	if err := validateMCPDefinitionInput(req); err != nil {
		return MCPDefinition{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.ServerKey = strings.TrimSpace(req.ServerKey)
	req.Description = strings.TrimSpace(req.Description)
	req.URL = strings.TrimSpace(req.URL)
	req.RequiredEnvVars = trimEnvNames(req.RequiredEnvVars)
	req.OptionalEnvVars = trimEnvNames(req.OptionalEnvVars)
	return s.repository.CreateMCPServerDefinition(ctx, req)
}

func (s *Service) ListMCPServerDefinitions(ctx context.Context, req ListMCPServerDefinitionsRequest) ([]MCPDefinition, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	return s.repository.ListMCPServerDefinitions(ctx, req)
}

func (s *Service) DeleteMCPServerDefinition(ctx context.Context, req DeleteMCPServerDefinitionRequest) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.ServerID == uuid.Nil {
		return fmt.Errorf("%w: server_id is required", ErrInvalidInput)
	}
	dependents, err := s.repository.ListDependentSkills(ctx, req.TenantID, req.ServerID)
	if err != nil {
		return err
	}
	if len(dependents) > 0 {
		slugs := make([]string, 0, len(dependents))
		for _, d := range dependents {
			slugs = append(slugs, d.Slug)
		}
		return fmt.Errorf("%w: mcp server is required by skills: %s", ErrConflict, strings.Join(slugs, ", "))
	}
	return s.repository.DeleteMCPServerDefinition(ctx, req)
}

// ListSkillMCPDependencies returns the MCP registry dependencies declared for a skill.
func (s *Service) ListSkillMCPDependencies(ctx context.Context, req ListSkillMCPDependenciesRequest) ([]SkillMCPDependency, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.SkillID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and skill_id are required", ErrInvalidInput)
	}
	exists, err := s.repository.SkillExistsForTenant(ctx, req.TenantID, req.SkillID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: skill %s not found", ErrNotFound, req.SkillID)
	}
	return s.repository.ListSkillMCPDependencies(ctx, req.TenantID, req.SkillID)
}

// ReplaceSkillMCPDependencies declaratively sets a skill's MCP registry dependencies after
// validating each referenced server exists and there are no duplicate references.
func (s *Service) ReplaceSkillMCPDependencies(ctx context.Context, req ReplaceSkillMCPDependenciesRequest) ([]SkillMCPDependency, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.SkillID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and skill_id are required", ErrInvalidInput)
	}
	exists, err := s.repository.SkillExistsForTenant(ctx, req.TenantID, req.SkillID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: skill %s not found", ErrNotFound, req.SkillID)
	}
	seen := map[uuid.UUID]struct{}{}
	for _, item := range req.Items {
		if item.MCPServerID == uuid.Nil {
			return nil, fmt.Errorf("%w: mcp_server_id is required", ErrInvalidInput)
		}
		if _, dup := seen[item.MCPServerID]; dup {
			return nil, fmt.Errorf("%w: duplicate mcp_server_id %s", ErrInvalidInput, item.MCPServerID)
		}
		seen[item.MCPServerID] = struct{}{}
		if _, err := s.repository.GetMCPServerDefinition(ctx, req.TenantID, item.MCPServerID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("%w: mcp server %s not found", ErrInvalidInput, item.MCPServerID)
			}
			return nil, err
		}
	}
	return s.repository.ReplaceSkillMCPDependencies(ctx, req.TenantID, req.SkillID, req.Items)
}

// ListDependentSkills reverse-looks-up active skills that depend on an MCP registry server.
func (s *Service) ListDependentSkills(ctx context.Context, req ListDependentSkillsRequest) ([]DependentSkill, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.ServerID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and server_id are required", ErrInvalidInput)
	}
	return s.repository.ListDependentSkills(ctx, req.TenantID, req.ServerID)
}

// ListSkillMCPDependenciesForRuntime skips user validation: it serves the run-service
// dispatch gate, mirroring ListEffectiveMCPConfigForRuntime.
func (s *Service) ListSkillMCPDependenciesForRuntime(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	return s.repository.ListSkillMCPDependenciesForSkills(ctx, tenantID, skillIDs)
}

// CreateTeamMCPBinding binds a registered MCP to a team. The MCP must exist and be active.
// Team-level env-var preflight is advisory because each employee carries its own env values.
func (s *Service) CreateTeamMCPBinding(ctx context.Context, req CreateTeamMCPBindingRequest) (MCPBinding, error) {
	if err := s.requireRepository(); err != nil {
		return MCPBinding{}, err
	}
	if req.TenantID == uuid.Nil {
		return MCPBinding{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return MCPBinding{}, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.MCPServerID == uuid.Nil {
		return MCPBinding{}, fmt.Errorf("%w: mcp_server_id is required", ErrInvalidInput)
	}
	if err := validateCredentialEnvVar(req.CredentialEnvVar); err != nil {
		return MCPBinding{}, err
	}
	if _, err := s.requireActiveMCPDefinition(ctx, req.TenantID, req.MCPServerID); err != nil {
		return MCPBinding{}, err
	}
	req.CredentialEnvVar = strings.TrimSpace(req.CredentialEnvVar)
	return s.repository.CreateTeamMCPBinding(ctx, req)
}

func (s *Service) ListTeamMCPBindings(ctx context.Context, req TeamScopedRequest) ([]MCPBinding, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if err := validateTeamScopedRequest(req); err != nil {
		return nil, err
	}
	return s.repository.ListTeamMCPBindings(ctx, req)
}

func (s *Service) DeleteTeamMCPBinding(ctx context.Context, req DeleteTeamMCPBindingRequest) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.BindingID == uuid.Nil {
		return fmt.Errorf("%w: binding_id is required", ErrInvalidInput)
	}
	return s.repository.DeleteTeamMCPBinding(ctx, req)
}

// CreateEmployeeMCPBindingV2 binds a registered MCP to a digital employee. The MCP must exist
// and be active; the returned binding carries a preflight (MissingEnvVars / blocked_missing_env)
// computed against the employee's configured env vars.
func (s *Service) CreateEmployeeMCPBindingV2(ctx context.Context, req CreateEmployeeMCPBindingV2Request) (MCPBinding, error) {
	if err := s.requireRepository(); err != nil {
		return MCPBinding{}, err
	}
	if req.TenantID == uuid.Nil {
		return MCPBinding{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return MCPBinding{}, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.MCPServerID == uuid.Nil {
		return MCPBinding{}, fmt.Errorf("%w: mcp_server_id is required", ErrInvalidInput)
	}
	if err := validateCredentialEnvVar(req.CredentialEnvVar); err != nil {
		return MCPBinding{}, err
	}
	definition, err := s.requireActiveMCPDefinition(ctx, req.TenantID, req.MCPServerID)
	if err != nil {
		return MCPBinding{}, err
	}
	req.CredentialEnvVar = strings.TrimSpace(req.CredentialEnvVar)
	binding, err := s.repository.CreateEmployeeMCPBindingV2(ctx, req)
	if err != nil {
		return MCPBinding{}, err
	}
	missing, err := s.missingEnvVarsForEmployee(ctx, req.TenantID, req.DigitalEmployeeID, definition.RequiredEnvVars)
	if err != nil {
		return MCPBinding{}, err
	}
	binding.MissingEnvVars = missing
	return binding, nil
}

func (s *Service) ListEmployeeMCPBindingsV2(ctx context.Context, req EmployeeScopedRequest) ([]MCPBinding, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if err := validateEmployeeScopedRequest(req); err != nil {
		return nil, err
	}
	bindings, err := s.repository.ListEmployeeMCPBindingsV2(ctx, req)
	if err != nil {
		return nil, err
	}
	configured, err := s.repository.ListConfiguredEmployeeEnvVarNames(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	configuredSet := stringSet(configured)
	for i := range bindings {
		bindings[i].MissingEnvVars = missingFromSet(bindings[i].RequiredEnvVars, configuredSet)
	}
	return bindings, nil
}

func (s *Service) DeleteEmployeeMCPBindingV2(ctx context.Context, req DeleteEmployeeMCPBindingV2Request) error {
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
	return s.repository.DeleteEmployeeMCPBindingV2(ctx, req)
}

// ListEffectiveMCPConfig resolves the effective MCP servers for an employee (team-inherited
// plus personal), each annotated with MissingEnvVars. Callers that build the Runtime payload
// must exclude entries whose BindingStatus is blocked_missing_env.
func (s *Service) ListEffectiveMCPConfig(ctx context.Context, req EmployeeScopedRequest) ([]EffectiveMCPServer, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if err := validateEmployeeScopedRequest(req); err != nil {
		return nil, err
	}
	return s.repository.ListEffectiveMCPBindingsV2(ctx, req)
}

// ListEffectiveMCPConfigForRuntime resolves effective MCP servers for an employee in a system
// (runtime) context where there is no console user. It performs the same resolution as
// ListEffectiveMCPConfig but skips the user-scoped validation.
func (s *Service) ListEffectiveMCPConfigForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]EffectiveMCPServer, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if digitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	return s.repository.ListEffectiveMCPBindingsV2(ctx, EmployeeScopedRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
}

// EvaluateEmployeeSkillMCPDependencies is the employee panel data source: for each skill
// runtime-effective on the employee, it resolves that skill's declared MCP dependencies and
// classifies each against the employee's actual bindings as satisfied, missing_binding (no
// binding at all) or blocked_missing_env (bound but required env vars are not configured).
func (s *Service) EvaluateEmployeeSkillMCPDependencies(ctx context.Context, req EvaluateEmployeeSkillMCPDependenciesRequest) ([]EmployeeSkillMCPDependencyStatus, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and employee_id are required", ErrInvalidInput)
	}
	if s.employeeRuntimeSkillLister == nil {
		return nil, nil
	}
	refs, err := s.employeeRuntimeSkillLister.ListEmployeeRuntimeSkillRefs(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, nil
	}
	skillIDs := make([]uuid.UUID, 0, len(refs))
	for _, ref := range refs {
		skillIDs = append(skillIDs, ref.ID)
	}
	deps, err := s.repository.ListSkillMCPDependenciesForSkills(ctx, req.TenantID, skillIDs)
	if err != nil {
		return nil, err
	}
	// ListEffectiveMCPConfigForRuntime resolves the same query as the console effective-config
	// path (ListEffectiveMCPBindingsV2): every bound server is returned, annotated with
	// MissingEnvVars rather than excluded. Confirmed against its implementation and against
	// runtimeMCPListerAdapter in app.go, which does its own post-hoc filtering on MissingEnvVars
	// before building the Runtime payload — so blocked bindings are not dropped here.
	effective, err := s.ListEffectiveMCPConfigForRuntime(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	missingEnvByServer := map[uuid.UUID][]string{}
	boundServers := map[uuid.UUID]struct{}{}
	for _, server := range effective {
		boundServers[server.ServerID] = struct{}{}
		if len(server.MissingEnvVars) > 0 {
			missingEnvByServer[server.ServerID] = server.MissingEnvVars
		}
	}
	bySkill := map[uuid.UUID]*EmployeeSkillMCPDependencyStatus{}
	ordered := make([]*EmployeeSkillMCPDependencyStatus, 0, len(refs))
	for _, ref := range refs {
		status := &EmployeeSkillMCPDependencyStatus{SkillID: ref.ID, SkillSlug: ref.Slug}
		bySkill[ref.ID] = status
		ordered = append(ordered, status)
	}
	for _, dep := range deps {
		item := EmployeeSkillMCPDependencyItem{
			MCPServerID:    dep.MCPServerID,
			ServerKey:      dep.ServerKey,
			ServerName:     dep.ServerName,
			Status:         "satisfied",
			MissingEnvVars: []string{},
		}
		if _, bound := boundServers[dep.MCPServerID]; !bound {
			item.Status = "missing_binding"
		} else if missing, blocked := missingEnvByServer[dep.MCPServerID]; blocked {
			item.Status = "blocked_missing_env"
			item.MissingEnvVars = missing
		}
		if status, ok := bySkill[dep.SkillID]; ok {
			status.Dependencies = append(status.Dependencies, item)
		}
	}
	out := make([]EmployeeSkillMCPDependencyStatus, 0, len(ordered))
	for _, status := range ordered {
		out = append(out, *status)
	}
	return out, nil
}

func (s *Service) requireActiveMCPDefinition(ctx context.Context, tenantID, serverID uuid.UUID) (MCPDefinition, error) {
	definition, err := s.repository.GetMCPServerDefinition(ctx, tenantID, serverID)
	if err != nil {
		return MCPDefinition{}, err
	}
	if definition.Status != "active" {
		return MCPDefinition{}, fmt.Errorf("%w: mcp server is not active", ErrInvalidInput)
	}
	return definition, nil
}

func (s *Service) missingEnvVarsForEmployee(ctx context.Context, tenantID, employeeID uuid.UUID, required []string) ([]string, error) {
	if len(required) == 0 {
		return nil, nil
	}
	configured, err := s.repository.ListConfiguredEmployeeEnvVarNames(ctx, tenantID, employeeID)
	if err != nil {
		return nil, err
	}
	return missingFromSet(required, stringSet(configured)), nil
}

func validateMCPDefinitionInput(req CreateMCPServerDefinitionRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: mcp name is required", ErrInvalidInput)
	}
	key := strings.TrimSpace(req.ServerKey)
	if key == "" {
		return fmt.Errorf("%w: server_key is required", ErrInvalidInput)
	}
	if !serverKeyPattern.MatchString(key) {
		return fmt.Errorf("%w: server_key must match [A-Za-z0-9_-]+", ErrInvalidInput)
	}
	if req.Transport != MCPTransportHTTP && req.Transport != MCPTransportStreamableHTTP {
		return fmt.Errorf("%w: transport must be http or streamable_http", ErrInvalidInput)
	}
	parsed, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: mcp url must be an absolute http url", ErrInvalidInput)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: mcp url must use http or https", ErrInvalidInput)
	}
	switch req.AuthStrategy {
	case MCPAuthStrategyNone, MCPAuthStrategyBearerEnv, MCPAuthStrategyHeadersEnv:
	default:
		return fmt.Errorf("%w: invalid auth_strategy %q", ErrInvalidInput, req.AuthStrategy)
	}
	for _, name := range append(append([]string{}, req.RequiredEnvVars...), req.OptionalEnvVars...) {
		if !envNamePattern.MatchString(strings.TrimSpace(name)) {
			return fmt.Errorf("%w: invalid environment variable name %q", ErrInvalidInput, name)
		}
	}
	return nil
}

func validateCredentialEnvVar(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	if !envNamePattern.MatchString(trimmed) {
		return fmt.Errorf("%w: invalid credential env var name %q", ErrInvalidInput, name)
	}
	return nil
}

func trimEnvNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func missingFromSet(required []string, configured map[string]struct{}) []string {
	missing := make([]string, 0)
	for _, name := range required {
		if _, ok := configured[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return missing
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
