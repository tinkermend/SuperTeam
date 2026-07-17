package capability

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestServiceCreatesCredentialWithSealedValueAndRedactedResponse(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	sealer, err := NewAESGCMCredentialSealer(key)
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	repo := &serviceRepo{}
	service := NewService(repo, sealer)
	tenantID := uuid.New()
	userID := uuid.New()

	created, err := service.CreateCredential(context.Background(), CreateCredentialRequest{
		TenantID:        tenantID,
		UserID:          userID,
		Name:            "ops-token",
		CredentialType:  CredentialTypeMCPToken,
		CredentialValue: "sk-test-1234567890",
	})
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}

	if created.EncryptedValue != "" {
		t.Fatalf("credential response must not expose encrypted value")
	}
	if created.LastFour != "7890" {
		t.Fatalf("expected last four 7890, got %q", created.LastFour)
	}
	if repo.createdCredential.EncryptedValue == "" || strings.Contains(repo.createdCredential.EncryptedValue, "sk-test") {
		t.Fatalf("expected sealed credential value, got %q", repo.createdCredential.EncryptedValue)
	}
}

func TestServiceBuildsMCPAuthorizationHeaderFromCredential(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	sealer, err := NewAESGCMCredentialSealer(key)
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	sealed, err := sealer.Seal("mcp-secret-token")
	if err != nil {
		t.Fatalf("seal token: %v", err)
	}
	tenantID := uuid.New()
	userID := uuid.New()
	credentialID := uuid.New()
	repo := &serviceRepo{
		credential: Credential{
			ID:             credentialID,
			TenantID:       tenantID,
			UserID:         userID,
			Name:           "ops-token",
			CredentialType: CredentialTypeMCPToken,
			EncryptedValue: sealed,
			LastFour:       "oken",
			Status:         "active",
		},
	}
	service := NewService(repo, sealer)

	header, err := service.BuildMCPAuthorizationHeader(context.Background(), ResolveCredentialRequest{
		TenantID:     tenantID,
		UserID:       userID,
		CredentialID: credentialID,
	})
	if err != nil {
		t.Fatalf("build authorization header: %v", err)
	}
	if header != "Bearer mcp-secret-token" {
		t.Fatalf("expected bearer header, got %q", header)
	}
}

func TestServiceRejectsCredentialCreateWithoutSealer(t *testing.T) {
	service := NewService(&serviceRepo{}, nil)
	_, err := service.CreateCredential(context.Background(), CreateCredentialRequest{
		TenantID:        uuid.New(),
		UserID:          uuid.New(),
		Name:            "ops-token",
		CredentialType:  CredentialTypeMCPToken,
		CredentialValue: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "credential encryption key is required") {
		t.Fatalf("expected missing key error, got %v", err)
	}
}

func TestServiceValidatesTeamMCPCredentialReferenceType(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	teamID := uuid.New()
	credentialID := uuid.New()
	repo := &serviceRepo{
		credential: Credential{
			ID:             credentialID,
			TenantID:       tenantID,
			UserID:         userID,
			CredentialType: CredentialType("api_key"),
			Status:         "active",
		},
	}
	service := NewService(repo, nil)

	_, err := service.CreateTeamMCPServer(context.Background(), CreateTeamMCPServerRequest{
		TenantID:     tenantID,
		TeamID:       teamID,
		UserID:       userID,
		Name:         "ops-mcp",
		URL:          "https://mcp.example.com",
		CredentialID: &credentialID,
	})
	if !errors.Is(err, ErrCredentialTypeInvalid) {
		t.Fatalf("expected invalid credential type, got %v", err)
	}
	if len(repo.getCredentialRequests) != 1 {
		t.Fatalf("expected credential lookup, got %d", len(repo.getCredentialRequests))
	}
	got := repo.getCredentialRequests[0]
	if got.TenantID != tenantID || got.UserID != userID || got.CredentialID != credentialID {
		t.Fatalf("unexpected credential lookup request: %#v", got)
	}
	if repo.createdTeamMCPServer {
		t.Fatalf("repository create must not be called for invalid credential type")
	}
}

func TestServiceRejectsEmployeeMCPBindingWithInactiveCredential(t *testing.T) {
	tests := []struct {
		name       string
		credential Credential
	}{
		{
			name: "inactive status",
			credential: Credential{
				CredentialType: CredentialTypeMCPToken,
				Status:         "disabled",
			},
		},
		{
			name: "disabled at set",
			credential: Credential{
				CredentialType: CredentialTypeMCPToken,
				Status:         "active",
				DisabledAt:     time.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenantID := uuid.New()
			userID := uuid.New()
			employeeID := uuid.New()
			credentialID := uuid.New()
			tt.credential.ID = credentialID
			tt.credential.TenantID = tenantID
			tt.credential.UserID = userID
			repo := &serviceRepo{credential: tt.credential}
			service := NewService(repo, nil)

			_, err := service.CreateEmployeeMCPBinding(context.Background(), CreateEmployeeMCPBindingRequest{
				TenantID:          tenantID,
				DigitalEmployeeID: employeeID,
				UserID:            userID,
				Name:              "ops-mcp",
				URL:               "https://mcp.example.com",
				CredentialID:      &credentialID,
			})
			if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "credential is not active") {
				t.Fatalf("expected inactive credential error, got %v", err)
			}
			if repo.createdEmployeeMCPBinding {
				t.Fatalf("repository create must not be called for inactive credential")
			}
		})
	}
}

func TestServiceBuildMCPAuthorizationHeaderRejectsDisabledCredential(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	sealer, err := NewAESGCMCredentialSealer(key)
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	tenantID := uuid.New()
	userID := uuid.New()
	credentialID := uuid.New()
	repo := &serviceRepo{
		credential: Credential{
			ID:             credentialID,
			TenantID:       tenantID,
			UserID:         userID,
			CredentialType: CredentialTypeMCPToken,
			EncryptedValue: "not-sealed",
			Status:         "active",
			DisabledAt:     time.Now(),
		},
	}
	service := NewService(repo, sealer)

	_, err = service.BuildMCPAuthorizationHeader(context.Background(), ResolveCredentialRequest{
		TenantID:     tenantID,
		UserID:       userID,
		CredentialID: credentialID,
	})
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "credential is not active") {
		t.Fatalf("expected inactive credential error, got %v", err)
	}
}

func TestServiceRejectsInvalidMCPURLs(t *testing.T) {
	tests := []struct {
		name       string
		serverURL  string
		createFunc func(*Service, CreateTeamMCPServerRequest, CreateEmployeeMCPBindingRequest) error
	}{
		{
			name:      "team missing scheme and host",
			serverURL: "not-a-url",
			createFunc: func(service *Service, teamReq CreateTeamMCPServerRequest, _ CreateEmployeeMCPBindingRequest) error {
				_, err := service.CreateTeamMCPServer(context.Background(), teamReq)
				return err
			},
		},
		{
			name:      "team unsupported scheme",
			serverURL: "ftp://example.com",
			createFunc: func(service *Service, teamReq CreateTeamMCPServerRequest, _ CreateEmployeeMCPBindingRequest) error {
				_, err := service.CreateTeamMCPServer(context.Background(), teamReq)
				return err
			},
		},
		{
			name:      "employee missing scheme and host",
			serverURL: "not-a-url",
			createFunc: func(service *Service, _ CreateTeamMCPServerRequest, employeeReq CreateEmployeeMCPBindingRequest) error {
				_, err := service.CreateEmployeeMCPBinding(context.Background(), employeeReq)
				return err
			},
		},
		{
			name:      "employee unsupported scheme",
			serverURL: "ftp://example.com",
			createFunc: func(service *Service, _ CreateTeamMCPServerRequest, employeeReq CreateEmployeeMCPBindingRequest) error {
				_, err := service.CreateEmployeeMCPBinding(context.Background(), employeeReq)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &serviceRepo{}
			service := NewService(repo, nil)
			err := tt.createFunc(service,
				CreateTeamMCPServerRequest{
					TenantID: uuid.New(),
					TeamID:   uuid.New(),
					UserID:   uuid.New(),
					Name:     "ops-mcp",
					URL:      tt.serverURL,
				},
				CreateEmployeeMCPBindingRequest{
					TenantID:          uuid.New(),
					DigitalEmployeeID: uuid.New(),
					UserID:            uuid.New(),
					Name:              "ops-mcp",
					URL:               tt.serverURL,
				},
			)
			if err == nil || !strings.Contains(err.Error(), "invalid capability input") {
				t.Fatalf("expected invalid input error, got %v", err)
			}
			if repo.createdTeamMCPServer || repo.createdEmployeeMCPBinding {
				t.Fatalf("repository create must not be called for invalid URL")
			}
		})
	}
}

func TestServiceRequestTypesExposePlannedHandlerContract(t *testing.T) {
	userID := uuid.New()
	serverID := uuid.New()
	bindingID := uuid.New()

	teamScoped := TeamScopedRequest{
		TenantID: uuid.New(),
		UserID:   userID,
		TeamID:   uuid.New(),
	}
	employeeScoped := EmployeeScopedRequest{
		TenantID:          uuid.New(),
		UserID:            userID,
		DigitalEmployeeID: uuid.New(),
	}
	deleteTeam := DeleteTeamMCPServerRequest{
		TenantID: uuid.New(),
		TeamID:   uuid.New(),
		ServerID: serverID,
	}
	deleteEmployee := DeleteEmployeeMCPBindingRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		BindingID:         bindingID,
	}

	if teamScoped.UserID != userID || employeeScoped.UserID != userID {
		t.Fatalf("scoped requests must expose user_id")
	}
	if deleteTeam.ServerID != serverID {
		t.Fatalf("delete team request must expose server_id")
	}
	if deleteEmployee.BindingID != bindingID {
		t.Fatalf("delete employee request must expose binding_id")
	}
}

type serviceRepo struct {
	createdCredential         Credential
	credential                Credential
	getCredentialRequests     []ResolveCredentialRequest
	createdTeamMCPServer      bool
	createdEmployeeMCPBinding bool

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

	effectiveByProject     map[uuid.UUID][]EffectiveMCPServer // keyed by project ID, for runtime projection merge
	projectBindings        map[uuid.UUID][]MCPBinding         // keyed by project ID
	putProjectBindingsArgs []ProjectMCPBindingInput
	putProjectBindingsHit  bool
}

func (r *serviceRepo) CreateCredential(_ context.Context, req CreateCredentialStoreRequest) (Credential, error) {
	r.createdCredential = Credential{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		Name:           req.Name,
		CredentialType: req.CredentialType,
		EncryptedValue: req.EncryptedValue,
		LastFour:       req.LastFour,
		Status:         "active",
	}
	return r.createdCredential, nil
}

func (r *serviceRepo) ListCredentials(context.Context, ListCredentialsRequest) ([]Credential, error) {
	return nil, nil
}

func (r *serviceRepo) GetCredential(_ context.Context, req ResolveCredentialRequest) (Credential, error) {
	r.getCredentialRequests = append(r.getCredentialRequests, req)
	return r.credential, nil
}

func (r *serviceRepo) CreateTeamMCPServer(context.Context, CreateTeamMCPServerRequest) (MCPServer, error) {
	r.createdTeamMCPServer = true
	return MCPServer{}, nil
}

func (r *serviceRepo) ListTeamMCPServers(context.Context, TeamScopedRequest) ([]MCPServer, error) {
	return nil, nil
}

func (r *serviceRepo) DeleteTeamMCPServer(context.Context, DeleteTeamMCPServerRequest) error {
	return nil
}

func (r *serviceRepo) CreateEmployeeMCPBinding(context.Context, CreateEmployeeMCPBindingRequest) (MCPServer, error) {
	r.createdEmployeeMCPBinding = true
	return MCPServer{}, nil
}

func (r *serviceRepo) ListEmployeeMCPBindings(context.Context, EmployeeScopedRequest) ([]MCPServer, error) {
	return nil, nil
}

func (r *serviceRepo) DeleteEmployeeMCPBinding(context.Context, DeleteEmployeeMCPBindingRequest) error {
	return nil
}

func (r *serviceRepo) ListEffectiveMCPServers(context.Context, EmployeeScopedRequest) ([]MCPServer, error) {
	return nil, nil
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
		Status:          "active",
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
	return MCPBinding{ID: uuid.New(), TenantID: req.TenantID, TeamID: &teamID, MCPServerID: req.MCPServerID, Status: "active", SourceScope: "team"}, nil
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
	return MCPBinding{ID: uuid.New(), TenantID: req.TenantID, DigitalEmployeeID: &employeeID, MCPServerID: req.MCPServerID, Status: "active", SourceScope: "employee"}, nil
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

func (r *serviceRepo) PutProjectMCPBindings(_ context.Context, tenantID, projectID, _ uuid.UUID, items []ProjectMCPBindingInput) ([]MCPBinding, error) {
	r.putProjectBindingsHit = true
	r.putProjectBindingsArgs = items
	if r.projectBindings == nil {
		r.projectBindings = map[uuid.UUID][]MCPBinding{}
	}
	bindings := make([]MCPBinding, 0, len(items))
	for _, item := range items {
		pid := projectID
		bindings = append(bindings, MCPBinding{
			ID:               uuid.New(),
			TenantID:         tenantID,
			ProjectID:        &pid,
			MCPServerID:      item.MCPServerID,
			CredentialEnvVar: item.CredentialEnvVar,
			Status:           "active",
			SourceScope:      "project",
		})
	}
	r.projectBindings[projectID] = bindings
	return bindings, nil
}

func (r *serviceRepo) ListProjectMCPBindings(_ context.Context, req ProjectScopedRequest) ([]MCPBinding, error) {
	return r.projectBindings[req.ProjectID], nil
}

func (r *serviceRepo) ListEffectiveProjectMCPServers(_ context.Context, _, projectID, _ uuid.UUID) ([]EffectiveMCPServer, error) {
	return r.effectiveByProject[projectID], nil
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
		Status:    "active",
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
			Status:          "active",
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

func TestServiceCreateEmployeeMCPBindingV2RejectsDisabledMCP(t *testing.T) {
	tenantID := uuid.New()
	serverID := uuid.New()
	repo := &serviceRepo{
		mcpDefinition: MCPDefinition{ID: serverID, TenantID: tenantID, Status: "disabled"},
	}
	svc := NewService(repo, nil)

	_, err := svc.CreateEmployeeMCPBindingV2(context.Background(), CreateEmployeeMCPBindingV2Request{
		TenantID:          tenantID,
		DigitalEmployeeID: uuid.New(),
		UserID:            uuid.New(),
		MCPServerID:       serverID,
	})
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("expected not-active error, got %v", err)
	}
	if repo.createdV2BindingHit {
		t.Fatalf("disabled mcp must not create a binding")
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
// 项目级 MCP 绑定（迁移 072，目录与能力投影修订 spec §3.2）
// ----------------------------------------------------------------------------

func TestServicePutProjectMCPBindingsValidatesServersAndReplaces(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	userID := uuid.New()
	repo := &serviceRepo{}
	serverID := repo.seedDefinition(tenantID, "github-mcp")
	service := NewService(repo, nil)

	bindings, err := service.PutProjectMCPBindings(context.Background(), PutProjectMCPBindingsRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		UserID:    userID,
		Items:     []ProjectMCPBindingInput{{MCPServerID: serverID, CredentialEnvVar: "  GH_TOKEN  "}},
	})
	if err != nil {
		t.Fatalf("put project mcp bindings: %v", err)
	}
	if !repo.putProjectBindingsHit {
		t.Fatalf("expected repository replace to be called")
	}
	if len(repo.putProjectBindingsArgs) != 1 || repo.putProjectBindingsArgs[0].CredentialEnvVar != "GH_TOKEN" {
		t.Fatalf("expected trimmed credential env var, got %#v", repo.putProjectBindingsArgs)
	}
	if len(bindings) != 1 || bindings[0].ProjectID == nil || *bindings[0].ProjectID != projectID || bindings[0].SourceScope != "project" {
		t.Fatalf("expected project-scoped binding response, got %#v", bindings)
	}
}

func TestServicePutProjectMCPBindingsAllowsEmptySetAsClear(t *testing.T) {
	tenantID := uuid.New()
	repo := &serviceRepo{}
	service := NewService(repo, nil)

	bindings, err := service.PutProjectMCPBindings(context.Background(), PutProjectMCPBindingsRequest{
		TenantID:  tenantID,
		ProjectID: uuid.New(),
		UserID:    uuid.New(),
		Items:     nil,
	})
	if err != nil {
		t.Fatalf("put empty project mcp bindings: %v", err)
	}
	if !repo.putProjectBindingsHit {
		t.Fatalf("expected repository replace to be called for declarative clear")
	}
	if len(bindings) != 0 {
		t.Fatalf("expected empty binding set, got %#v", bindings)
	}
}

func TestServicePutProjectMCPBindingsRejectsInvalidItems(t *testing.T) {
	tenantID := uuid.New()
	repo := &serviceRepo{}
	activeID := repo.seedDefinition(tenantID, "github-mcp")
	disabledID := uuid.New()
	repo.definitions[disabledID] = MCPDefinition{ID: disabledID, TenantID: tenantID, ServerKey: "disabled-mcp", Status: "disabled"}
	service := NewService(repo, nil)

	tests := []struct {
		name    string
		items   []ProjectMCPBindingInput
		wantErr string
	}{
		{
			name:    "nil server id",
			items:   []ProjectMCPBindingInput{{MCPServerID: uuid.Nil}},
			wantErr: "mcp_server_id is required",
		},
		{
			name:    "duplicate server id",
			items:   []ProjectMCPBindingInput{{MCPServerID: activeID}, {MCPServerID: activeID}},
			wantErr: "duplicate mcp_server_id",
		},
		{
			name:    "unknown server",
			items:   []ProjectMCPBindingInput{{MCPServerID: uuid.New()}},
			wantErr: "not found",
		},
		{
			name:    "inactive server",
			items:   []ProjectMCPBindingInput{{MCPServerID: disabledID}},
			wantErr: "mcp server is not active",
		},
		{
			name:    "invalid credential env var",
			items:   []ProjectMCPBindingInput{{MCPServerID: activeID, CredentialEnvVar: "1BAD NAME"}},
			wantErr: "invalid credential env var name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo.putProjectBindingsHit = false
			_, err := service.PutProjectMCPBindings(context.Background(), PutProjectMCPBindingsRequest{
				TenantID:  tenantID,
				ProjectID: uuid.New(),
				UserID:    uuid.New(),
				Items:     tt.items,
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
			if repo.putProjectBindingsHit {
				t.Fatalf("repository replace must not run on validation failure")
			}
		})
	}
}

// TestServiceRuntimeProjectionMergesProjectBindingsWithPriority 覆盖投影合并语义：
// 结果 = 员工侧集合 ∪ 项目绑定集合，同 server_key 时项目绑定优先（员工侧同 key 条目
// 整条被替换，含 credential_env_var）；仅项目绑定的 server 也必须出现在结果里。
func TestServiceRuntimeProjectionMergesProjectBindingsWithPriority(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	projectID := uuid.New()
	sharedEmployeeServerID := uuid.New()
	sharedProjectServerID := uuid.New()
	employeeOnlyServerID := uuid.New()
	projectOnlyServerID := uuid.New()

	repo := &serviceRepo{}
	repo.seedEffective(employeeID, employeeOnlyServerID, nil)
	repo.effectiveByEmployee[employeeID] = []EffectiveMCPServer{
		{ServerID: employeeOnlyServerID, ServerKey: "search-mcp", CredentialEnvVar: "SEARCH_TOKEN", SourceScope: "employee"},
		{ServerID: sharedEmployeeServerID, ServerKey: "github-mcp", CredentialEnvVar: "EMPLOYEE_GH_TOKEN", SourceScope: "team"},
	}
	repo.effectiveByProject = map[uuid.UUID][]EffectiveMCPServer{
		projectID: {
			{ServerID: sharedProjectServerID, ServerKey: "github-mcp", CredentialEnvVar: "PROJECT_GH_TOKEN", SourceScope: "project"},
			{ServerID: projectOnlyServerID, ServerKey: "deploy-mcp", SourceScope: "project", MissingEnvVars: []string{"DEPLOY_TOKEN"}},
		},
	}
	service := NewService(repo, nil)

	merged, err := service.ListEffectiveMCPConfigForRuntime(context.Background(), tenantID, employeeID, &projectID)
	if err != nil {
		t.Fatalf("list effective mcp config for runtime: %v", err)
	}
	byKey := map[string]EffectiveMCPServer{}
	for _, server := range merged {
		byKey[server.ServerKey] = server
	}
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged servers, got %#v", merged)
	}
	if got := byKey["github-mcp"]; got.SourceScope != "project" || got.CredentialEnvVar != "PROJECT_GH_TOKEN" || got.ServerID != sharedProjectServerID {
		t.Fatalf("expected project binding to win on shared server_key, got %#v", got)
	}
	if got := byKey["search-mcp"]; got.SourceScope != "employee" {
		t.Fatalf("expected employee-only server preserved, got %#v", got)
	}
	if got := byKey["deploy-mcp"]; got.SourceScope != "project" || len(got.MissingEnvVars) != 1 {
		t.Fatalf("expected project-only server with env preflight preserved, got %#v", got)
	}

	// projectID 为 nil 时是纯员工侧结果，项目绑定不得渗入。
	employeeOnly, err := service.ListEffectiveMCPConfigForRuntime(context.Background(), tenantID, employeeID, nil)
	if err != nil {
		t.Fatalf("list effective mcp config without project: %v", err)
	}
	if len(employeeOnly) != 2 {
		t.Fatalf("expected employee-side servers only, got %#v", employeeOnly)
	}
	for _, server := range employeeOnly {
		if server.SourceScope == "project" {
			t.Fatalf("project binding leaked into nil-project projection: %#v", server)
		}
	}
}

func TestServiceListProjectMCPBindingsValidatesScope(t *testing.T) {
	repo := &serviceRepo{}
	service := NewService(repo, nil)
	_, err := service.ListProjectMCPBindings(context.Background(), ProjectScopedRequest{
		TenantID: uuid.New(),
		UserID:   uuid.New(),
	})
	if err == nil || !strings.Contains(err.Error(), "project_id is required") {
		t.Fatalf("expected project_id validation error, got %v", err)
	}
}
