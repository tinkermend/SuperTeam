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

	// 项目级 MCP 绑定（迁移 072，目录与能力投影修订 spec §3.2）。
	PutProjectMCPBindings(ctx context.Context, tenantID, projectID, createdBy uuid.UUID, items []ProjectMCPBindingInput) ([]MCPBinding, error)
	ListProjectMCPBindings(ctx context.Context, req ProjectScopedRequest) ([]MCPBinding, error)
	ListEffectiveProjectMCPServers(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) ([]EffectiveMCPServer, error)

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

// PutProjectMCPBindings declaratively replaces a project's MCP bindings with the desired set
// after validating each referenced server exists, is active and belongs to the tenant.
// 与 team 绑定一致：不校验 project 本体存在性（team 版同样不校验 team 存在），归属
// 边界由 handler 层的项目级 authz（ResourceProject）承担；env-var 预检同样是 advisory，
// 因为凭据值由各员工自己的环境变量提供。
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
	return s.repository.PutProjectMCPBindings(ctx, req.TenantID, req.ProjectID, req.UserID, items)
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
// ListEffectiveMCPConfig but skips the user-scoped validation. projectID 可选（目录与能力
// 投影修订 spec §3.2）：非 nil 时结果 = 员工侧集合（team 继承 + 个人）∪ 项目绑定集合，
// 同 server_key 时项目绑定优先——两侧均在治理链内，冲突在治理体系内以项目侧为准。
// 项目绑定的 MissingEnvVars 同样按该员工的已配置 env 集合判定，调用方对全量结果做
// 统一的 env-satisfied 过滤即可。
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
	if len(projectSide) == 0 {
		return employeeSide, nil
	}
	projectKeys := make(map[string]struct{}, len(projectSide))
	for _, server := range projectSide {
		projectKeys[server.ServerKey] = struct{}{}
	}
	merged := make([]EffectiveMCPServer, 0, len(employeeSide)+len(projectSide))
	for _, server := range employeeSide {
		if _, overridden := projectKeys[server.ServerKey]; overridden {
			continue
		}
		merged = append(merged, server)
	}
	merged = append(merged, projectSide...)
	return merged, nil
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
