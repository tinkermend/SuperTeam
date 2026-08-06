package capability

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/teamguard"
)

// envNamePattern matches a POSIX-style environment variable name.
var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// serverKeyPattern matches the stable MCP key rendered into provider config (e.g. "github").
var serverKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Repository interface {
	// MCP HTTP capability registry (migration 037).
	CreateMCPServerDefinition(ctx context.Context, req CreateMCPServerDefinitionRequest) (MCPDefinition, error)
	ListMCPServerDefinitions(ctx context.Context, req ListMCPServerDefinitionsRequest) ([]MCPDefinition, error)
	GetMCPServerDefinition(ctx context.Context, tenantID, serverID uuid.UUID) (MCPDefinition, error)
	DeleteMCPServerDefinition(ctx context.Context, req DeleteMCPServerDefinitionRequest) error
	CreateTeamMCPBinding(ctx context.Context, req CreateTeamMCPBindingRequest) (MCPBinding, error)
	ListTeamMCPBindings(ctx context.Context, req TeamScopedRequest) ([]MCPBinding, error)
	DeleteTeamMCPBinding(ctx context.Context, req DeleteTeamMCPBindingRequest) error
	CreateEmployeeMCPBindingV2(ctx context.Context, req CreateEmployeeMCPBindingV2Request) (MCPBinding, error)
	// 团队接管收敛（spec §5.2.1）：同一 MCP 在团队与员工两维度只留一份，团队胜出。
	TeamProvidesMCPServer(ctx context.Context, tenantID, employeeID, mcpServerID uuid.UUID) (bool, error)
	ListTeamMCPTakeoverTargets(ctx context.Context, tenantID, teamID, mcpServerID uuid.UUID) ([]MCPTakeover, error)
	ListTeamMCPReadiness(ctx context.Context, tenantID, teamID uuid.UUID) ([]TeamMCPReadinessEntry, error)
	ListEmployeeMCPBindingsV2(ctx context.Context, req EmployeeScopedRequest) ([]MCPBinding, error)
	DeleteEmployeeMCPBindingV2(ctx context.Context, req DeleteEmployeeMCPBindingV2Request) error
	ListEffectiveMCPBindingsV2(ctx context.Context, req EmployeeScopedRequest) ([]EffectiveMCPServer, error)
	PutProjectMCPBindings(ctx context.Context, tenantID, projectID, createdBy uuid.UUID, items []ProjectMCPBindingInput) ([]MCPBinding, error)
	ListProjectMCPBindings(ctx context.Context, req ProjectScopedRequest) ([]MCPBinding, error)
	ListEffectiveProjectMCPServers(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) ([]EffectiveMCPServer, error)
	ListConfiguredEmployeeEnvVarNames(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]string, error)

	// Skill <-> MCP registry dependency declarations (migration 062).
	SkillExistsForTenant(ctx context.Context, tenantID, skillID uuid.UUID) (bool, error)
	ListSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID) ([]SkillMCPDependency, error)
	ReplaceSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID, items []SkillMCPDependencyInput) ([]SkillMCPDependency, error)
	ListDependentSkills(ctx context.Context, tenantID, serverID uuid.UUID) ([]DependentSkill, error)
	ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error)
	ListSkillMCPDependenciesIncludingMissing(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error)
}

// EmployeeRuntimeSkillLister resolves the runtime-effective skill set for a digital employee.
// It is satisfied by an adapter over the skill module (see app.go) so this package does not
// import skill directly.
type EmployeeRuntimeSkillLister interface {
	ListEmployeeRuntimeSkillRefs(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeSkillRef, error)
}

// CapabilityBindingEventRecorder records project capability binding changes (best-effort).
type CapabilityBindingEventRecorder interface {
	RecordProjectCapabilityBindingChanged(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, bindingKind string, resourceIDs []uuid.UUID) error
}

type Service struct {
	repository                 Repository
	sealer                     CredentialSealer
	employeeRuntimeSkillLister EmployeeRuntimeSkillLister
	eventRecorder              CapabilityBindingEventRecorder
}

func NewService(repository Repository, sealer CredentialSealer) *Service {
	return &Service{repository: repository, sealer: sealer}
}

// SetEmployeeRuntimeSkillLister wires the skill-module adapter used by
// EvaluateEmployeeSkillMCPDependencies. Optional: if unset, that method returns an empty result.
func (s *Service) SetEmployeeRuntimeSkillLister(l EmployeeRuntimeSkillLister) {
	s.employeeRuntimeSkillLister = l
}

func (s *Service) SetCapabilityBindingEventRecorder(r CapabilityBindingEventRecorder) {
	s.eventRecorder = r
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
// GetMCPServerDefinitionForRuntime loads an MCP definition without console user validation.
func (s *Service) GetMCPServerDefinitionForRuntime(ctx context.Context, tenantID, serverID uuid.UUID) (MCPDefinition, error) {
	if err := s.requireRepository(); err != nil {
		return MCPDefinition{}, err
	}
	if tenantID == uuid.Nil || serverID == uuid.Nil {
		return MCPDefinition{}, fmt.Errorf("%w: tenant_id and server_id are required", ErrInvalidInput)
	}
	return s.repository.GetMCPServerDefinition(ctx, tenantID, serverID)
}

func (s *Service) ListSkillMCPDependenciesIncludingMissing(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	return s.repository.ListSkillMCPDependenciesIncludingMissing(ctx, tenantID, skillIDs)
}

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
	// 团队已提供同一 MCP 时写时拒绝。此前是照写不误、再由读路径 NOT EXISTS 静默
	// 屏蔽，结果员工页「个人 MCP 绑定」和「生效 MCP 配置」自相矛盾（spec §5.2.1）。
	provided, err := s.repository.TeamProvidesMCPServer(ctx, req.TenantID, req.DigitalEmployeeID, req.MCPServerID)
	if err != nil {
		return MCPBinding{}, err
	}
	if provided {
		return MCPBinding{}, teamguard.CapabilityProvidedByTeamError("MCP", definition.Name)
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
// (runtime) context where there is no console user. When projectID is non-nil, project-bound
// MCP servers are unioned in; same server_key prefers the project side (capability supply
// three-layer model §5). Dependency-closure MCP entries are merged by the run-service after
// skill projection (source_scope=dependency_closure).
func (s *Service) ListEffectiveMCPConfigForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID, projectID *uuid.UUID) ([]EffectiveMCPServer, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if digitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	employeeSide, err := s.repository.ListEffectiveMCPBindingsV2(ctx, EmployeeScopedRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	if projectID == nil || *projectID == uuid.Nil {
		return employeeSide, nil
	}
	projectSide, err := s.repository.ListEffectiveProjectMCPServers(ctx, tenantID, *projectID, digitalEmployeeID)
	if err != nil {
		return nil, err
	}
	return mergeEffectiveMCPServers(employeeSide, projectSide), nil
}

// PutProjectMCPBindings declaratively replaces a project's MCP bindings after validating each
// referenced server exists and belongs to the tenant. Empty items clears all bindings.
func (s *Service) PutProjectMCPBindings(ctx context.Context, req PutProjectMCPBindingsRequest) ([]MCPBinding, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.ProjectID == uuid.Nil {
		return nil, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	seen := map[uuid.UUID]struct{}{}
	items := make([]ProjectMCPBindingInput, 0, len(req.Items))
	for _, item := range req.Items {
		if item.MCPServerID == uuid.Nil {
			return nil, fmt.Errorf("%w: mcp_server_id is required", ErrInvalidInput)
		}
		if _, dup := seen[item.MCPServerID]; dup {
			return nil, fmt.Errorf("%w: duplicate mcp_server_id %s", ErrInvalidInput, item.MCPServerID)
		}
		seen[item.MCPServerID] = struct{}{}
		if err := validateCredentialEnvVar(item.CredentialEnvVar); err != nil {
			return nil, err
		}
		if _, err := s.requireActiveMCPDefinition(ctx, req.TenantID, item.MCPServerID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("%w: mcp server %s not found", ErrInvalidInput, item.MCPServerID)
			}
			return nil, err
		}
		item.CredentialEnvVar = strings.TrimSpace(item.CredentialEnvVar)
		items = append(items, item)
	}
	bound, err := s.repository.PutProjectMCPBindings(ctx, req.TenantID, req.ProjectID, req.UserID, items)
	if err != nil {
		return nil, err
	}
	if s.eventRecorder != nil {
		ids := make([]uuid.UUID, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.MCPServerID)
		}
		_ = s.eventRecorder.RecordProjectCapabilityBindingChanged(ctx, req.TenantID, req.ProjectID, req.UserID, "mcp", ids)
	}
	return bound, nil
}

func (s *Service) ListProjectMCPBindings(ctx context.Context, req ProjectScopedRequest) ([]MCPBinding, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if req.ProjectID == uuid.Nil {
		return nil, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	return s.repository.ListProjectMCPBindings(ctx, req)
}

// mergeEffectiveMCPServers unions employee-side and project-side MCP sets. Same server_key
// prefers project. Project-only keys are appended after employee order is preserved for
// non-overridden entries.
func mergeEffectiveMCPServers(employeeSide, projectSide []EffectiveMCPServer) []EffectiveMCPServer {
	byKey := make(map[string]EffectiveMCPServer, len(employeeSide)+len(projectSide))
	order := make([]string, 0, len(employeeSide)+len(projectSide))
	for _, server := range employeeSide {
		key := strings.TrimSpace(server.ServerKey)
		if key == "" {
			key = server.ServerID.String()
		}
		if _, exists := byKey[key]; !exists {
			order = append(order, key)
		}
		byKey[key] = server
	}
	for _, server := range projectSide {
		key := strings.TrimSpace(server.ServerKey)
		if key == "" {
			key = server.ServerID.String()
		}
		if _, exists := byKey[key]; !exists {
			order = append(order, key)
		}
		// project always wins on collision
		byKey[key] = server
	}
	out := make([]EffectiveMCPServer, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	return out
}

// MergeDependencyClosureMCPServers appends MCP servers required by projected skills that are
// not already present. Added entries keep source_scope=dependency_closure for audit.
func MergeDependencyClosureMCPServers(base []EffectiveMCPServer, closure []EffectiveMCPServer) []EffectiveMCPServer {
	if len(closure) == 0 {
		return base
	}
	present := make(map[uuid.UUID]struct{}, len(base))
	presentKey := make(map[string]struct{}, len(base))
	for _, server := range base {
		present[server.ServerID] = struct{}{}
		if key := strings.TrimSpace(server.ServerKey); key != "" {
			presentKey[key] = struct{}{}
		}
	}
	out := append([]EffectiveMCPServer{}, base...)
	for _, server := range closure {
		if _, ok := present[server.ServerID]; ok {
			continue
		}
		if key := strings.TrimSpace(server.ServerKey); key != "" {
			if _, ok := presentKey[key]; ok {
				continue
			}
			presentKey[key] = struct{}{}
		}
		present[server.ServerID] = struct{}{}
		server.SourceScope = "dependency_closure"
		out = append(out, server)
	}
	return out
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
	// 员工面板无项目上下文，projectID 传 nil：只评估员工自身携带的绑定集合。
	effective, err := s.ListEffectiveMCPConfigForRuntime(ctx, req.TenantID, req.DigitalEmployeeID, nil)
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
	// 注册表定义只有创建/删除两态（迁移 087）：Get 按 deleted_at 过滤，取到即活跃。
	return s.repository.GetMCPServerDefinition(ctx, tenantID, serverID)
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

// TeamCapabilityConflict 团队绑定某能力前的接管预览：这次绑定会收敛掉哪些成员的
// 个人绑定，其中哪些人原先用的是不同的凭据变量名（接管后会立刻缺变量）。
// 预览与执行走同一条查询，"看到的"就是"接管的"。
type TeamCapabilityConflict struct {
	MCPServerID uuid.UUID
	Takeovers   []MCPTakeover
}

// PreviewTeamMCPTakeover 返回团队绑定该 MCP 时将接管的成员个人绑定清单。
func (s *Service) PreviewTeamMCPTakeover(ctx context.Context, tenantID, teamID, mcpServerID uuid.UUID) (TeamCapabilityConflict, error) {
	if err := s.requireRepository(); err != nil {
		return TeamCapabilityConflict{}, err
	}
	if tenantID == uuid.Nil || teamID == uuid.Nil || mcpServerID == uuid.Nil {
		return TeamCapabilityConflict{}, fmt.Errorf("%w: tenant_id, team_id and mcp_server_id are required", ErrInvalidInput)
	}
	takeovers, err := s.repository.ListTeamMCPTakeoverTargets(ctx, tenantID, teamID, mcpServerID)
	if err != nil {
		return TeamCapabilityConflict{}, err
	}
	return TeamCapabilityConflict{MCPServerID: mcpServerID, Takeovers: takeovers}, nil
}

// TeamMCPReadinessEntry 团队某个 MCP 在某名成员上的就绪情况。
type TeamMCPReadinessEntry struct {
	MCPServerID       uuid.UUID
	ServerKey         string
	ServerName        string
	RequiredEnvVars   []string
	DigitalEmployeeID uuid.UUID
	EmployeeName      string
	MissingEnvVars    []string
}

// ListTeamMCPReadiness 团队 MCP 就绪矩阵。变量名由绑定/注册表定义、值只存在员工级，
// 所以"就绪"天然是逐员工的：这里一次性把 MCP × 成员的缺失情况铺开，避免控制台
// 逐员工 N+1 查询。
func (s *Service) ListTeamMCPReadiness(ctx context.Context, tenantID, teamID uuid.UUID) ([]TeamMCPReadinessEntry, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if tenantID == uuid.Nil || teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	return s.repository.ListTeamMCPReadiness(ctx, tenantID, teamID)
}
