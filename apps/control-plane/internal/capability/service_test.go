package capability

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type serviceRepo struct {
	mcpDefinition       MCPDefinition
	createdDefinition   CreateMCPServerDefinitionRequest
	configuredEnvVars   []string
	createdV2Binding    CreateEmployeeMCPBindingV2Request
	createdV2BindingHit bool

	definitions          map[uuid.UUID]MCPDefinition
	dependentSkills      map[uuid.UUID][]DependentSkill
	skillMCPDependencies map[uuid.UUID][]SkillMCPDependency // keyed by skill ID
	effectiveByEmployee  map[uuid.UUID][]EffectiveMCPServer // keyed by digital employee ID
	existingSkills       map[uuid.UUID]bool                 // keyed by skill ID, for SkillExistsForTenant

}

func (r *serviceRepo) CreateMCPServerDefinition(_ context.Context, req CreateMCPServerDefinitionRequest) (MCPDefinition, error) {
	r.createdDefinition = req
	return MCPDefinition{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		Name:            req.Name,
		ServerKey:       req.ServerKey,
		Transport:       req.Transport,
		URL:             req.URL,
		AuthStrategy:    req.AuthStrategy,
		RequiredEnvVars: req.RequiredEnvVars,
	}, nil
}

func (r *serviceRepo) ListMCPServerDefinitions(context.Context, ListMCPServerDefinitionsRequest) ([]MCPDefinition, error) {
	return nil, nil
}

func (r *serviceRepo) GetMCPServerDefinition(_ context.Context, _, serverID uuid.UUID) (MCPDefinition, error) {
	if def, ok := r.definitions[serverID]; ok {
		return def, nil
	}
	if r.mcpDefinition.ID != uuid.Nil && r.mcpDefinition.ID == serverID {
		return r.mcpDefinition, nil
	}
	return MCPDefinition{}, ErrNotFound
}

func (r *serviceRepo) DeleteMCPServerDefinition(context.Context, DeleteMCPServerDefinitionRequest) error {
	return nil
}

func (r *serviceRepo) CreateTeamMCPBinding(_ context.Context, req CreateTeamMCPBindingRequest) (MCPBinding, error) {
	teamID := req.TeamID
	return MCPBinding{ID: uuid.New(), TenantID: req.TenantID, TeamID: &teamID, MCPServerID: req.MCPServerID, SourceScope: "team"}, nil
}

func (r *serviceRepo) ListTeamMCPBindings(context.Context, TeamScopedRequest) ([]MCPBinding, error) {
	return nil, nil
}

func (r *serviceRepo) DeleteTeamMCPBinding(context.Context, DeleteTeamMCPBindingRequest) error {
	return nil
}

func (r *serviceRepo) CreateEmployeeMCPBindingV2(_ context.Context, req CreateEmployeeMCPBindingV2Request) (MCPBinding, error) {
	r.createdV2Binding = req
	r.createdV2BindingHit = true
	employeeID := req.DigitalEmployeeID
	return MCPBinding{ID: uuid.New(), TenantID: req.TenantID, DigitalEmployeeID: &employeeID, MCPServerID: req.MCPServerID, SourceScope: "employee"}, nil
}

func (r *serviceRepo) ListEmployeeMCPBindingsV2(context.Context, EmployeeScopedRequest) ([]MCPBinding, error) {
	return nil, nil
}

func (r *serviceRepo) DeleteEmployeeMCPBindingV2(context.Context, DeleteEmployeeMCPBindingV2Request) error {
	return nil
}

func (r *serviceRepo) ListEffectiveMCPBindingsV2(_ context.Context, req EmployeeScopedRequest) ([]EffectiveMCPServer, error) {
	return r.effectiveByEmployee[req.DigitalEmployeeID], nil
}

func (r *serviceRepo) ListConfiguredEmployeeEnvVarNames(context.Context, uuid.UUID, uuid.UUID) ([]string, error) {
	return r.configuredEnvVars, nil
}

func (r *serviceRepo) SkillExistsForTenant(_ context.Context, _, skillID uuid.UUID) (bool, error) {
	return r.existingSkills[skillID], nil
}

func (r *serviceRepo) ListSkillMCPDependencies(context.Context, uuid.UUID, uuid.UUID) ([]SkillMCPDependency, error) {
	return nil, nil
}

func (r *serviceRepo) ReplaceSkillMCPDependencies(context.Context, uuid.UUID, uuid.UUID, []SkillMCPDependencyInput) ([]SkillMCPDependency, error) {
	return nil, nil
}

func (r *serviceRepo) ListDependentSkills(_ context.Context, _, serverID uuid.UUID) ([]DependentSkill, error) {
	return r.dependentSkills[serverID], nil
}

func (r *serviceRepo) ListSkillMCPDependenciesForSkills(_ context.Context, _ uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error) {
	var out []SkillMCPDependency
	for _, skillID := range skillIDs {
		out = append(out, r.skillMCPDependencies[skillID]...)
	}
	return out, nil
}

// seedDefinition registers an in-memory MCP definition and returns its generated ID, for
// tests that need GetMCPServerDefinition to resolve a real server (e.g. dependency replace
// validation, delete-protection).
func (r *serviceRepo) seedDefinition(tenantID uuid.UUID, serverKey string) uuid.UUID {
	id := uuid.New()
	if r.definitions == nil {
		r.definitions = map[uuid.UUID]MCPDefinition{}
	}
	r.definitions[id] = MCPDefinition{
		ID:        id,
		TenantID:  tenantID,
		Name:      serverKey,
		ServerKey: serverKey,
	}
	return id
}

// seedDependency records a dependent-skill row keyed by server ID, for
// ListDependentSkills / delete-protection tests. It also records the corresponding
// SkillMCPDependency (keyed by skill ID, enriched from any definition seeded via
// seedDefinition) for ListSkillMCPDependenciesForSkills / EvaluateEmployeeSkillMCPDependencies
// tests.
func (r *serviceRepo) seedDependency(tenantID, skillID, serverID uuid.UUID) {
	if r.dependentSkills == nil {
		r.dependentSkills = map[uuid.UUID][]DependentSkill{}
	}
	r.dependentSkills[serverID] = append(r.dependentSkills[serverID], DependentSkill{
		SkillID: skillID,
		Slug:    "dependent-skill",
		Name:    "Dependent Skill",
	})
	def := r.definitions[serverID]
	if r.skillMCPDependencies == nil {
		r.skillMCPDependencies = map[uuid.UUID][]SkillMCPDependency{}
	}
	r.skillMCPDependencies[skillID] = append(r.skillMCPDependencies[skillID], SkillMCPDependency{
		ID:          uuid.New(),
		TenantID:    tenantID,
		SkillID:     skillID,
		MCPServerID: serverID,
		ServerKey:   def.ServerKey,
		ServerName:  def.Name,
	})
}

// seedSkill marks a skill ID as existing for the tenant, for SkillExistsForTenant tests
// gating ReplaceSkillMCPDependencies / ListSkillMCPDependencies.
func (r *serviceRepo) seedSkill(skillID uuid.UUID) {
	if r.existingSkills == nil {
		r.existingSkills = map[uuid.UUID]bool{}
	}
	r.existingSkills[skillID] = true
}

// seedEffective records an effective (bound) MCP server for an employee, for
// EvaluateEmployeeSkillMCPDependencies tests. A non-empty missingEnvVars marks the binding
// blocked_missing_env.
func (r *serviceRepo) seedEffective(employeeID, serverID uuid.UUID, missingEnvVars []string) {
	if r.effectiveByEmployee == nil {
		r.effectiveByEmployee = map[uuid.UUID][]EffectiveMCPServer{}
	}
	r.effectiveByEmployee[employeeID] = append(r.effectiveByEmployee[employeeID], EffectiveMCPServer{
		ServerID:       serverID,
		MissingEnvVars: missingEnvVars,
	})
}

type fakeRuntimeSkillLister struct {
	refs []RuntimeSkillRef
	err  error
}

func (f fakeRuntimeSkillLister) ListEmployeeRuntimeSkillRefs(context.Context, uuid.UUID, uuid.UUID) ([]RuntimeSkillRef, error) {
	return f.refs, f.err
}

func TestServiceReplaceSkillMCPDependenciesValidatesServerExists(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo, nil)
	tenantID, userID, skillID := uuid.New(), uuid.New(), uuid.New()
	repo.seedSkill(skillID)
	_, err := svc.ReplaceSkillMCPDependencies(context.Background(), ReplaceSkillMCPDependenciesRequest{
		TenantID: tenantID, UserID: userID, SkillID: skillID,
		Items: []SkillMCPDependencyInput{{MCPServerID: uuid.New()}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for unknown mcp server, got %v", err)
	}
}

func TestServiceReplaceSkillMCPDependenciesReturnsNotFoundForUnknownSkill(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo, nil)
	tenantID, userID, skillID := uuid.New(), uuid.New(), uuid.New()
	_, err := svc.ReplaceSkillMCPDependencies(context.Background(), ReplaceSkillMCPDependenciesRequest{
		TenantID: tenantID, UserID: userID, SkillID: skillID,
		Items: []SkillMCPDependencyInput{{MCPServerID: uuid.New()}},
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown skill, got %v", err)
	}
}

func TestServiceListSkillMCPDependenciesReturnsNotFoundForUnknownSkill(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo, nil)
	tenantID, userID, skillID := uuid.New(), uuid.New(), uuid.New()
	_, err := svc.ListSkillMCPDependencies(context.Background(), ListSkillMCPDependenciesRequest{
		TenantID: tenantID, UserID: userID, SkillID: skillID,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown skill, got %v", err)
	}
}

func TestServiceDeleteMCPServerDefinitionBlockedByDependentSkills(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo, nil)
	tenantID, userID := uuid.New(), uuid.New()
	serverID := repo.seedDefinition(tenantID, "github-mcp")
	repo.seedDependency(tenantID, uuid.New(), serverID)
	err := svc.DeleteMCPServerDefinition(context.Background(), DeleteMCPServerDefinitionRequest{
		TenantID: tenantID, UserID: userID, ServerID: serverID,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict when skills depend on server, got %v", err)
	}
}

func TestServiceCreateMCPServerDefinitionValidatesHTTPOnlyAndEnvVars(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	svc := NewService(&serviceRepo{}, nil)

	_, err := svc.CreateMCPServerDefinition(context.Background(), CreateMCPServerDefinitionRequest{
		TenantID:        tenantID,
		UserID:          userID,
		Name:            "Local Stdio MCP",
		ServerKey:       "local-stdio",
		Transport:       "stdio",
		URL:             "file:///tmp/mcp",
		AuthStrategy:    MCPAuthStrategyNone,
		RequiredEnvVars: []string{"BAD-NAME"},
	})

	if err == nil || !strings.Contains(err.Error(), "transport must be http") {
		t.Fatalf("expected http-only validation error, got %v", err)
	}
}

func TestServiceCreateEmployeeMCPBindingV2ComputesMissingEnvPreflight(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	serverID := uuid.New()
	repo := &serviceRepo{
		mcpDefinition: MCPDefinition{
			ID:              serverID,
			TenantID:        tenantID,
			Name:            "GitHub MCP",
			ServerKey:       "github",
			Transport:       MCPTransportStreamableHTTP,
			URL:             "https://api.githubcopilot.com/mcp/",
			AuthStrategy:    MCPAuthStrategyBearerEnv,
			RequiredEnvVars: []string{"GITHUB_TOKEN"},
		},
		configuredEnvVars: nil, // employee has not configured GITHUB_TOKEN
	}
	svc := NewService(repo, nil)

	binding, err := svc.CreateEmployeeMCPBindingV2(context.Background(), CreateEmployeeMCPBindingV2Request{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		UserID:            uuid.New(),
		MCPServerID:       serverID,
		CredentialEnvVar:  "GITHUB_TOKEN",
	})
	if err != nil {
		t.Fatalf("create v2 binding: %v", err)
	}
	if !repo.createdV2BindingHit {
		t.Fatalf("expected binding to be persisted")
	}
	if len(binding.MissingEnvVars) != 1 || binding.MissingEnvVars[0] != "GITHUB_TOKEN" {
		t.Fatalf("expected missing GITHUB_TOKEN, got %#v", binding.MissingEnvVars)
	}
	if binding.PreflightStatus() != MCPBindingStatusBlockedMissingEnv {
		t.Fatalf("expected blocked_missing_env, got %q", binding.PreflightStatus())
	}
}

func TestServiceEvaluatesEmployeeSkillMCPDependencyStatus(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo, nil)
	tenantID, userID, employeeID, skillID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	boundServer := repo.seedDefinition(tenantID, "bound-mcp") // 已绑定且 env 满足
	envBlocked := repo.seedDefinition(tenantID, "env-mcp")    // 已绑定但缺 env（fake effective 返回 MissingEnvVars）
	unbound := repo.seedDefinition(tenantID, "unbound-mcp")   // 未绑定
	repo.seedDependency(tenantID, skillID, boundServer)
	repo.seedDependency(tenantID, skillID, envBlocked)
	repo.seedDependency(tenantID, skillID, unbound)
	repo.seedEffective(employeeID, boundServer, nil)
	repo.seedEffective(employeeID, envBlocked, []string{"GH_TOKEN"})
	svc.SetEmployeeRuntimeSkillLister(fakeRuntimeSkillLister{refs: []RuntimeSkillRef{{ID: skillID, Slug: "deploy-helper"}}})

	statuses, err := svc.EvaluateEmployeeSkillMCPDependencies(context.Background(), EvaluateEmployeeSkillMCPDependenciesRequest{TenantID: tenantID, UserID: userID, DigitalEmployeeID: employeeID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 || len(statuses[0].Dependencies) != 3 {
		t.Fatalf("expected 1 skill with 3 dependencies, got %+v", statuses)
	}
	if statuses[0].SkillID != skillID || statuses[0].SkillSlug != "deploy-helper" {
		t.Fatalf("unexpected skill identity: %+v", statuses[0])
	}
	byKey := map[string]string{}
	missingEnvByKey := map[string][]string{}
	for _, dep := range statuses[0].Dependencies {
		byKey[dep.ServerKey] = dep.Status
		missingEnvByKey[dep.ServerKey] = dep.MissingEnvVars
	}
	if byKey["bound-mcp"] != "satisfied" || byKey["env-mcp"] != "blocked_missing_env" || byKey["unbound-mcp"] != "missing_binding" {
		t.Fatalf("unexpected statuses: %v", byKey)
	}
	if len(missingEnvByKey["bound-mcp"]) != 0 || len(missingEnvByKey["unbound-mcp"]) != 0 {
		t.Fatalf("missing_env_vars must be empty outside blocked_missing_env: %v", missingEnvByKey)
	}
	if len(missingEnvByKey["env-mcp"]) != 1 || missingEnvByKey["env-mcp"][0] != "GH_TOKEN" {
		t.Fatalf("expected missing_env_vars [GH_TOKEN] for env-mcp, got %v", missingEnvByKey["env-mcp"])
	}
}

func TestServiceEvaluateEmployeeSkillMCPDependenciesReturnsEmptyWhenListerUnset(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo, nil)
	tenantID, userID, employeeID := uuid.New(), uuid.New(), uuid.New()

	statuses, err := svc.EvaluateEmployeeSkillMCPDependencies(context.Background(), EvaluateEmployeeSkillMCPDependenciesRequest{TenantID: tenantID, UserID: userID, DigitalEmployeeID: employeeID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 0 {
		t.Fatalf("expected empty result when no lister is wired, got %+v", statuses)
	}
}

func TestServiceEvaluateEmployeeSkillMCPDependenciesReturnsEmptyWhenEmployeeHasNoSkills(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo, nil)
	svc.SetEmployeeRuntimeSkillLister(fakeRuntimeSkillLister{refs: nil})
	tenantID, userID, employeeID := uuid.New(), uuid.New(), uuid.New()

	statuses, err := svc.EvaluateEmployeeSkillMCPDependencies(context.Background(), EvaluateEmployeeSkillMCPDependenciesRequest{TenantID: tenantID, UserID: userID, DigitalEmployeeID: employeeID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 0 {
		t.Fatalf("expected empty result when employee has no runtime skills, got %+v", statuses)
	}
}

// ----------------------------------------------------------------------------
// TestServiceRuntimeProjectionIgnoresProjectID verifies that after project MCP binding
// retirement, ListEffectiveMCPConfigForRuntime always returns employee-side servers only,
// regardless of whether a projectID is provided.
func TestServiceRuntimeProjectionIgnoresProjectID(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	projectID := uuid.New()
	employeeServerID := uuid.New()

	repo := &serviceRepo{}
	repo.effectiveByEmployee = map[uuid.UUID][]EffectiveMCPServer{
		employeeID: {
			{ServerID: employeeServerID, ServerKey: "search-mcp", CredentialEnvVar: "SEARCH_TOKEN", SourceScope: "employee"},
		},
	}
	service := NewService(repo, nil)

	// With projectID provided: should still return only employee side.
	withProject, err := service.ListEffectiveMCPConfigForRuntime(context.Background(), tenantID, employeeID, &projectID)
	if err != nil {
		t.Fatalf("list effective mcp config with project: %v", err)
	}
	if len(withProject) != 1 || withProject[0].ServerID != employeeServerID {
		t.Fatalf("expected employee-side only, got %#v", withProject)
	}

	// Without projectID: same result.
	withoutProject, err := service.ListEffectiveMCPConfigForRuntime(context.Background(), tenantID, employeeID, nil)
	if err != nil {
		t.Fatalf("list effective mcp config without project: %v", err)
	}
	if len(withoutProject) != 1 || withoutProject[0].ServerID != employeeServerID {
		t.Fatalf("expected employee-side only, got %#v", withoutProject)
	}
}
