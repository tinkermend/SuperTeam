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

	definitions     map[uuid.UUID]MCPDefinition
	dependentSkills map[uuid.UUID][]DependentSkill
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

func (r *serviceRepo) ListEffectiveMCPBindingsV2(context.Context, EmployeeScopedRequest) ([]EffectiveMCPServer, error) {
	return nil, nil
}

func (r *serviceRepo) ListConfiguredEmployeeEnvVarNames(context.Context, uuid.UUID, uuid.UUID) ([]string, error) {
	return r.configuredEnvVars, nil
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

func (r *serviceRepo) ListSkillMCPDependenciesForSkills(context.Context, uuid.UUID, []uuid.UUID) ([]SkillMCPDependency, error) {
	return nil, nil
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
// ListDependentSkills / delete-protection tests.
func (r *serviceRepo) seedDependency(tenantID, skillID, serverID uuid.UUID) {
	_ = tenantID
	if r.dependentSkills == nil {
		r.dependentSkills = map[uuid.UUID][]DependentSkill{}
	}
	r.dependentSkills[serverID] = append(r.dependentSkills[serverID], DependentSkill{
		SkillID: skillID,
		Slug:    "dependent-skill",
		Name:    "Dependent Skill",
	})
}

func TestServiceReplaceSkillMCPDependenciesValidatesServerExists(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo, nil)
	tenantID, userID, skillID := uuid.New(), uuid.New(), uuid.New()
	_, err := svc.ReplaceSkillMCPDependencies(context.Background(), ReplaceSkillMCPDependenciesRequest{
		TenantID: tenantID, UserID: userID, SkillID: skillID,
		Items: []SkillMCPDependencyInput{{MCPServerID: uuid.New()}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for unknown mcp server, got %v", err)
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
