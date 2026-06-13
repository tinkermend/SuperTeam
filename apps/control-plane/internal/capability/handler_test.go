package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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

	credential Credential
	mcpServer  MCPServer

	createCredentialReq CreateCredentialRequest
	listCredentialsReq  ListCredentialsRequest
	createTeamReq       CreateTeamMCPServerRequest
	listTeamReq         TeamScopedRequest
	deleteTeamReq       DeleteTeamMCPServerRequest
	createEmployeeReq   CreateEmployeeMCPBindingRequest
	listEmployeeReq     EmployeeScopedRequest
	deleteEmployeeReq   DeleteEmployeeMCPBindingRequest
	effectiveReq        EmployeeScopedRequest
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
