package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

func TestHandlerCreateCredentialRedactsSecretsAndPassesRawValue(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	credentialID := uuid.New()
	service := &handlerService{
		credential: Credential{
			ID:             credentialID,
			TenantID:       tenantID,
			UserID:         userID,
			Name:           "ops-token",
			CredentialType: CredentialTypeMCPToken,
			EncryptedValue: "sealed-secret",
			LastFour:       "7890",
			Status:         "active",
		},
	}
	handler := NewHandler(service)
	authorizer := &handlerAuthorizer{allowed: true}
	handler.SetAuthorizer(authorizer)

	body := bytes.NewBufferString(`{"name":"ops-token","credential_type":"mcp_token","credential_value":"sk-test-1234567890"}`)
	req := requestWithConsoleIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/user-credentials", body), tenantID, userID)
	resp := httptest.NewRecorder()

	handler.CreateCredential(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.createCredentialReq.TenantID != tenantID || service.createCredentialReq.UserID != userID {
		t.Fatalf("unexpected credential identity: %#v", service.createCredentialReq)
	}
	if service.createCredentialReq.CredentialType != CredentialTypeMCPToken || service.createCredentialReq.CredentialValue != "sk-test-1234567890" {
		t.Fatalf("expected raw credential value and typed credential type, got %#v", service.createCredentialReq)
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionCredentialCreate || authorizer.checks[0].Resource.Type != authz.ResourceCredential || authorizer.checks[0].Resource.ID != userID.String() {
		t.Fatalf("unexpected authz check: %#v", authorizer.checks)
	}
	var response map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := response["credential_value"]; ok {
		t.Fatalf("credential response exposed credential_value: %#v", response)
	}
	if _, ok := response["encrypted_value"]; ok {
		t.Fatalf("credential response exposed encrypted_value: %#v", response)
	}
	if response["credential_type"] != string(CredentialTypeMCPToken) || response["last_four"] != "7890" {
		t.Fatalf("unexpected credential response: %#v", response)
	}
}

func TestHandlerTeamMCPRoutesUseManageActionAndTypedResponses(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	teamID := uuid.New()
	credentialID := uuid.New()
	serverID := uuid.New()
	service := &handlerService{
		mcpServer: MCPServer{
			ID:                 serverID,
			TenantID:           tenantID,
			TeamID:             &teamID,
			Name:               "ops-mcp",
			URL:                "https://mcp.example.com",
			CredentialID:       &credentialID,
			CredentialName:     "ops-token",
			CredentialType:     CredentialTypeMCPToken,
			CredentialLastFour: "7890",
			Status:             "active",
			SourceScope:        "team",
		},
	}
	handler := NewHandler(service)
	authorizer := &handlerAuthorizer{allowed: true}
	handler.SetAuthorizer(authorizer)

	createReq := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodPost, "/teams/"+teamID.String()+"/mcp-servers", bytes.NewBufferString(`{"name":"ops-mcp","url":"https://mcp.example.com","credential_id":"`+credentialID.String()+`"}`)), map[string]string{"teamId": teamID.String()}),
		tenantID,
		userID,
	)
	createResp := httptest.NewRecorder()
	handler.CreateTeamMCPServer(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	if service.createTeamReq.TenantID != tenantID || service.createTeamReq.UserID != userID || service.createTeamReq.TeamID != teamID || service.createTeamReq.CredentialID == nil || *service.createTeamReq.CredentialID != credentialID {
		t.Fatalf("unexpected create team mcp request: %#v", service.createTeamReq)
	}
	var created map[string]any
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created mcp server: %v", err)
	}
	if created["credential_type"] != string(CredentialTypeMCPToken) {
		t.Fatalf("expected credential_type string, got %#v", created)
	}
	if _, ok := created["encrypted_value"]; ok {
		t.Fatalf("mcp response exposed encrypted_value: %#v", created)
	}

	deleteReq := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodDelete, "/teams/"+teamID.String()+"/mcp-servers/"+serverID.String(), nil), map[string]string{"teamId": teamID.String(), "serverId": serverID.String()}),
		tenantID,
		userID,
	)
	deleteResp := httptest.NewRecorder()
	handler.DeleteTeamMCPServer(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("expected delete status 204, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
	if service.deleteTeamReq.TenantID != tenantID || service.deleteTeamReq.TeamID != teamID || service.deleteTeamReq.ServerID != serverID {
		t.Fatalf("unexpected delete team mcp request: %#v", service.deleteTeamReq)
	}
	for _, check := range authorizer.checks {
		if check.Action != authz.ActionTeamCapabilityManage || check.Resource.Type != authz.ResourceTeam || check.Resource.ID != teamID.String() || check.TeamID == nil || *check.TeamID != teamID {
			t.Fatalf("unexpected team mcp authz check: %#v", check)
		}
	}
}

func TestHandlerEmployeeMCPRoutesUseEditAndReadActions(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	employeeID := uuid.New()
	bindingID := uuid.New()
	service := &handlerService{
		mcpServer: MCPServer{
			ID:                bindingID,
			TenantID:          tenantID,
			DigitalEmployeeID: &employeeID,
			Name:              "personal-mcp",
			URL:               "https://personal.example.com",
			CredentialType:    CredentialTypeMCPToken,
			Status:            "active",
			SourceScope:       "employee",
		},
	}
	handler := NewHandler(service)
	authorizer := &handlerAuthorizer{allowed: true}
	handler.SetAuthorizer(authorizer)

	listReq := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodGet, "/digital-employees/"+employeeID.String()+"/mcp-bindings", nil), map[string]string{"employeeId": employeeID.String()}),
		tenantID,
		userID,
	)
	listResp := httptest.NewRecorder()
	handler.ListEmployeeMCPBindings(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listEmployeeReq.TenantID != tenantID || service.listEmployeeReq.UserID != userID || service.listEmployeeReq.DigitalEmployeeID != employeeID {
		t.Fatalf("unexpected list employee request: %#v", service.listEmployeeReq)
	}

	effectiveReq := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodGet, "/digital-employees/"+employeeID.String()+"/effective-mcp-servers", nil), map[string]string{"employeeId": employeeID.String()}),
		tenantID,
		userID,
	)
	effectiveResp := httptest.NewRecorder()
	handler.ListEffectiveMCPServers(effectiveResp, effectiveReq)
	if effectiveResp.Code != http.StatusOK {
		t.Fatalf("expected effective status 200, got %d: %s", effectiveResp.Code, effectiveResp.Body.String())
	}

	if len(authorizer.checks) != 2 {
		t.Fatalf("expected two authz checks, got %#v", authorizer.checks)
	}
	if authorizer.checks[0].Action != authz.ActionEmployeeCapabilityEdit || authorizer.checks[0].Resource.Type != authz.ResourceEmployee || authorizer.checks[0].Resource.ID != employeeID.String() {
		t.Fatalf("unexpected employee binding authz check: %#v", authorizer.checks[0])
	}
	if authorizer.checks[1].Action != authz.ActionEmployeeRead || authorizer.checks[1].Resource.Type != authz.ResourceEmployee || authorizer.checks[1].Resource.ID != employeeID.String() {
		t.Fatalf("unexpected effective mcp authz check: %#v", authorizer.checks[1])
	}
}

func TestHandlerMapsCapabilityErrors(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid input", err: ErrInvalidInput, want: http.StatusBadRequest},
		{name: "missing key", err: ErrCredentialKeyMissing, want: http.StatusBadRequest},
		{name: "invalid credential type", err: ErrCredentialTypeInvalid, want: http.StatusBadRequest},
		{name: "not found", err: ErrNotFound, want: http.StatusNotFound},
		{name: "default", err: errors.New("database DSN leaked"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &handlerService{err: tt.err}
			handler := NewHandler(service)
			handler.SetAuthorizer(&handlerAuthorizer{allowed: true})
			req := requestWithConsoleIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/user-credentials", nil), tenantID, userID)
			resp := httptest.NewRecorder()

			handler.ListCredentials(resp, req)

			if resp.Code != tt.want {
				t.Fatalf("expected status %d, got %d: %s", tt.want, resp.Code, resp.Body.String())
			}
			if tt.want == http.StatusInternalServerError && bytes.Contains(resp.Body.Bytes(), []byte("DSN")) {
				t.Fatalf("internal error leaked detail: %s", resp.Body.String())
			}
		})
	}
}

type handlerService struct {
	err error

	credential       Credential
	mcpServer        MCPServer
	mcpDefinition    MCPDefinition
	mcpBinding       MCPBinding
	effectiveServers []EffectiveMCPServer

	createCredentialReq CreateCredentialRequest
	listCredentialsReq  ListCredentialsRequest
	createTeamReq       CreateTeamMCPServerRequest
	listTeamReq         TeamScopedRequest
	deleteTeamReq       DeleteTeamMCPServerRequest
	createEmployeeReq   CreateEmployeeMCPBindingRequest
	listEmployeeReq     EmployeeScopedRequest
	deleteEmployeeReq   DeleteEmployeeMCPBindingRequest
	effectiveReq        EmployeeScopedRequest

	createDefinitionReq        CreateMCPServerDefinitionRequest
	listDefinitionsReq         ListMCPServerDefinitionsRequest
	deleteDefinitionReq        DeleteMCPServerDefinitionRequest
	createTeamBindingReq       CreateTeamMCPBindingRequest
	createEmployeeBindingV2Req CreateEmployeeMCPBindingV2Request
	effectiveConfigReq         EmployeeScopedRequest

	skillMCPDependencies      []SkillMCPDependency
	dependentSkills           []DependentSkill
	listSkillDependenciesReq  ListSkillMCPDependenciesRequest
	replaceSkillDependencyReq ReplaceSkillMCPDependenciesRequest
	listDependentSkillsReq    ListDependentSkillsRequest
}

func (s *handlerService) CreateCredential(_ context.Context, req CreateCredentialRequest) (Credential, error) {
	s.createCredentialReq = req
	return s.credential, s.err
}

func (s *handlerService) ListCredentials(_ context.Context, req ListCredentialsRequest) ([]Credential, error) {
	s.listCredentialsReq = req
	return []Credential{s.credential}, s.err
}

func (s *handlerService) CreateTeamMCPServer(_ context.Context, req CreateTeamMCPServerRequest) (MCPServer, error) {
	s.createTeamReq = req
	return s.mcpServer, s.err
}

func (s *handlerService) ListTeamMCPServers(_ context.Context, req TeamScopedRequest) ([]MCPServer, error) {
	s.listTeamReq = req
	return []MCPServer{s.mcpServer}, s.err
}

func (s *handlerService) DeleteTeamMCPServer(_ context.Context, req DeleteTeamMCPServerRequest) error {
	s.deleteTeamReq = req
	return s.err
}

func (s *handlerService) CreateEmployeeMCPBinding(_ context.Context, req CreateEmployeeMCPBindingRequest) (MCPServer, error) {
	s.createEmployeeReq = req
	return s.mcpServer, s.err
}

func (s *handlerService) ListEmployeeMCPBindings(_ context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	s.listEmployeeReq = req
	return []MCPServer{s.mcpServer}, s.err
}

func (s *handlerService) DeleteEmployeeMCPBinding(_ context.Context, req DeleteEmployeeMCPBindingRequest) error {
	s.deleteEmployeeReq = req
	return s.err
}

func (s *handlerService) ListEffectiveMCPServers(_ context.Context, req EmployeeScopedRequest) ([]MCPServer, error) {
	s.effectiveReq = req
	return []MCPServer{s.mcpServer}, s.err
}

func (s *handlerService) CreateMCPServerDefinition(_ context.Context, req CreateMCPServerDefinitionRequest) (MCPDefinition, error) {
	s.createDefinitionReq = req
	return s.mcpDefinition, s.err
}

func (s *handlerService) ListMCPServerDefinitions(_ context.Context, req ListMCPServerDefinitionsRequest) ([]MCPDefinition, error) {
	s.listDefinitionsReq = req
	return []MCPDefinition{s.mcpDefinition}, s.err
}

func (s *handlerService) DeleteMCPServerDefinition(_ context.Context, req DeleteMCPServerDefinitionRequest) error {
	s.deleteDefinitionReq = req
	return s.err
}

func (s *handlerService) CreateTeamMCPBinding(_ context.Context, req CreateTeamMCPBindingRequest) (MCPBinding, error) {
	s.createTeamBindingReq = req
	return s.mcpBinding, s.err
}

func (s *handlerService) ListTeamMCPBindings(_ context.Context, _ TeamScopedRequest) ([]MCPBinding, error) {
	return []MCPBinding{s.mcpBinding}, s.err
}

func (s *handlerService) DeleteTeamMCPBinding(_ context.Context, _ DeleteTeamMCPBindingRequest) error {
	return s.err
}

func (s *handlerService) CreateEmployeeMCPBindingV2(_ context.Context, req CreateEmployeeMCPBindingV2Request) (MCPBinding, error) {
	s.createEmployeeBindingV2Req = req
	return s.mcpBinding, s.err
}

func (s *handlerService) ListEmployeeMCPBindingsV2(_ context.Context, _ EmployeeScopedRequest) ([]MCPBinding, error) {
	return []MCPBinding{s.mcpBinding}, s.err
}

func (s *handlerService) DeleteEmployeeMCPBindingV2(_ context.Context, _ DeleteEmployeeMCPBindingV2Request) error {
	return s.err
}

func (s *handlerService) ListEffectiveMCPConfig(_ context.Context, req EmployeeScopedRequest) ([]EffectiveMCPServer, error) {
	s.effectiveConfigReq = req
	return s.effectiveServers, s.err
}

func (s *handlerService) ListSkillMCPDependencies(_ context.Context, req ListSkillMCPDependenciesRequest) ([]SkillMCPDependency, error) {
	s.listSkillDependenciesReq = req
	return s.skillMCPDependencies, s.err
}

func (s *handlerService) ReplaceSkillMCPDependencies(_ context.Context, req ReplaceSkillMCPDependenciesRequest) ([]SkillMCPDependency, error) {
	s.replaceSkillDependencyReq = req
	return s.skillMCPDependencies, s.err
}

func (s *handlerService) ListDependentSkills(_ context.Context, req ListDependentSkillsRequest) ([]DependentSkill, error) {
	s.listDependentSkillsReq = req
	return s.dependentSkills, s.err
}

type handlerAuthorizer struct {
	allowed bool
	checks  []authz.CheckRequest
}

func (a *handlerAuthorizer) Check(_ context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	if a.allowed {
		return authz.Decision{Allowed: true, Reason: authz.ReasonAllowed}, nil
	}
	return authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership}, nil
}

func (a *handlerAuthorizer) CheckBulkTeamActions(_ context.Context, _ authz.BulkTeamActionsRequest) ([]string, error) {
	return nil, nil
}

func requestWithConsoleIdentity(req *http.Request, tenantID, userID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	return req.WithContext(ctx)
}

func requestWithChiParams(req *http.Request, params map[string]string) *http.Request {
	routeCtx := chi.NewRouteContext()
	for key, value := range params {
		routeCtx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func TestHandlerReplaceSkillMCPDependenciesUsesManageAction(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	skillID := uuid.New()
	depServerID := uuid.New()
	depID := uuid.New()
	service := &handlerService{
		skillMCPDependencies: []SkillMCPDependency{
			{
				ID:           depID,
				TenantID:     tenantID,
				SkillID:      skillID,
				MCPServerID:  depServerID,
				Note:         "api",
				ServerKey:    "ops-mcp",
				ServerName:   "Ops MCP",
				AuthStrategy: MCPAuthStrategy("bearer"),
				RiskLevel:    "low",
				ServerStatus: "active",
			},
		},
	}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	body := strings.NewReader(`{"items":[{"mcp_server_id":"` + depServerID.String() + `","note":"api"}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/skills/"+skillID.String()+"/mcp-dependencies", body)
	req = requestWithConsoleIdentity(req, tenantID, userID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("skillId", skillID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	resp := httptest.NewRecorder()
	handler.ReplaceSkillMCPDependencies(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	if authorizer.checks[0].Action != authz.ActionMCPRegistryManage {
		t.Fatalf("expected manage action, got %s", authorizer.checks[0].Action)
	}

	if service.replaceSkillDependencyReq.TenantID != tenantID || service.replaceSkillDependencyReq.UserID != userID || service.replaceSkillDependencyReq.SkillID != skillID {
		t.Fatalf("unexpected replace request identity: %#v", service.replaceSkillDependencyReq)
	}
	if len(service.replaceSkillDependencyReq.Items) != 1 {
		t.Fatalf("expected one parsed item, got %#v", service.replaceSkillDependencyReq.Items)
	}
	if service.replaceSkillDependencyReq.Items[0].MCPServerID != depServerID || service.replaceSkillDependencyReq.Items[0].Note != "api" {
		t.Fatalf("expected note to round-trip with parsed server id, got %#v", service.replaceSkillDependencyReq.Items[0])
	}

	var response []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("expected one dependency in response, got %#v", response)
	}
	got := response[0]
	if got["id"] != depID.String() || got["skill_id"] != skillID.String() || got["mcp_server_id"] != depServerID.String() ||
		got["note"] != "api" || got["server_key"] != "ops-mcp" || got["server_name"] != "Ops MCP" ||
		got["auth_strategy"] != "bearer" || got["risk_level"] != "low" || got["server_status"] != "active" {
		t.Fatalf("unexpected replace response fields: %#v", got)
	}
}

func TestHandlerReplaceSkillMCPDependenciesInvalidJSONReturns400(t *testing.T) {
	service := &handlerService{}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	skillID := uuid.New()
	body := strings.NewReader(`{"items":[`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/skills/"+skillID.String()+"/mcp-dependencies", body)
	req = requestWithConsoleIdentity(req, uuid.New(), uuid.New())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("skillId", skillID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	resp := httptest.NewRecorder()
	handler.ReplaceSkillMCPDependencies(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json body, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestHandlerReplaceSkillMCPDependenciesInvalidServerIDReturns400(t *testing.T) {
	service := &handlerService{}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	skillID := uuid.New()
	body := strings.NewReader(`{"items":[{"mcp_server_id":"not-a-uuid","note":"api"}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/skills/"+skillID.String()+"/mcp-dependencies", body)
	req = requestWithConsoleIdentity(req, uuid.New(), uuid.New())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("skillId", skillID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	resp := httptest.NewRecorder()
	handler.ReplaceSkillMCPDependencies(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid mcp_server_id, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestHandlerListSkillMCPDependenciesReturnsRecordsWithReadAction(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	skillID := uuid.New()
	depServerID := uuid.New()
	depID := uuid.New()
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	service := &handlerService{
		skillMCPDependencies: []SkillMCPDependency{
			{
				ID:           depID,
				TenantID:     tenantID,
				SkillID:      skillID,
				MCPServerID:  depServerID,
				Note:         "needed for search",
				CreatedAt:    createdAt,
				ServerKey:    "ops-mcp",
				ServerName:   "Ops MCP",
				AuthStrategy: MCPAuthStrategy("bearer"),
				RiskLevel:    "medium",
				ServerStatus: "active",
			},
		},
	}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	req := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodGet, "/api/v1/skills/"+skillID.String()+"/mcp-dependencies", nil), map[string]string{"skillId": skillID.String()}),
		tenantID,
		userID,
	)
	resp := httptest.NewRecorder()
	handler.ListSkillMCPDependencies(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionMCPRegistryRead {
		t.Fatalf("expected one read action check, got %#v", authorizer.checks)
	}
	if service.listSkillDependenciesReq.TenantID != tenantID || service.listSkillDependenciesReq.UserID != userID || service.listSkillDependenciesReq.SkillID != skillID {
		t.Fatalf("unexpected list skill dependencies request: %#v", service.listSkillDependenciesReq)
	}

	var response []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("expected one dependency, got %#v", response)
	}
	got := response[0]
	if got["id"] != depID.String() {
		t.Fatalf("unexpected id: %#v", got)
	}
	if got["skill_id"] != skillID.String() {
		t.Fatalf("unexpected skill_id: %#v", got)
	}
	if got["mcp_server_id"] != depServerID.String() {
		t.Fatalf("unexpected mcp_server_id: %#v", got)
	}
	if got["note"] != "needed for search" {
		t.Fatalf("unexpected note: %#v", got)
	}
	if got["server_key"] != "ops-mcp" {
		t.Fatalf("unexpected server_key: %#v", got)
	}
	if got["server_name"] != "Ops MCP" {
		t.Fatalf("unexpected server_name: %#v", got)
	}
	if got["auth_strategy"] != "bearer" {
		t.Fatalf("unexpected auth_strategy: %#v", got)
	}
	if got["risk_level"] != "medium" {
		t.Fatalf("unexpected risk_level: %#v", got)
	}
	if got["server_status"] != "active" {
		t.Fatalf("unexpected server_status: %#v", got)
	}
	if got["created_at"] != createdAt.Format(time.RFC3339) {
		t.Fatalf("unexpected created_at: %#v", got)
	}
}

func TestHandlerListDependentSkillsReturnsRecordsWithReadAction(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	dependentSkillID := uuid.New()
	service := &handlerService{
		dependentSkills: []DependentSkill{
			{
				SkillID: dependentSkillID,
				Slug:    "search-helper",
				Name:    "Search Helper",
			},
		},
	}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	req := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodGet, "/api/v1/mcp-servers/"+serverID.String()+"/dependent-skills", nil), map[string]string{"serverId": serverID.String()}),
		tenantID,
		userID,
	)
	resp := httptest.NewRecorder()
	handler.ListDependentSkills(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionMCPRegistryRead {
		t.Fatalf("expected one read action check, got %#v", authorizer.checks)
	}
	if service.listDependentSkillsReq.TenantID != tenantID || service.listDependentSkillsReq.UserID != userID || service.listDependentSkillsReq.ServerID != serverID {
		t.Fatalf("unexpected list dependent skills request: %#v", service.listDependentSkillsReq)
	}

	var response []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("expected one dependent skill, got %#v", response)
	}
	got := response[0]
	if got["skill_id"] != dependentSkillID.String() {
		t.Fatalf("unexpected skill_id: %#v", got)
	}
	if got["slug"] != "search-helper" {
		t.Fatalf("unexpected slug: %#v", got)
	}
	if got["name"] != "Search Helper" {
		t.Fatalf("unexpected name: %#v", got)
	}
}

func TestHandlerDeleteMCPServerDefinitionConflictMapsTo409(t *testing.T) {
	service := &handlerService{err: fmt.Errorf("%w: mcp server is required by skills: a", ErrConflict)}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	serverID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/mcp-servers/"+serverID.String(), nil)
	req = requestWithConsoleIdentity(req, uuid.New(), uuid.New())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("serverId", serverID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	resp := httptest.NewRecorder()
	handler.DeleteMCPServerDefinition(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.Code)
	}
}
