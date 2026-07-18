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
		{name: "not found", err: ErrNotFound, want: http.StatusNotFound},
		{name: "default", err: errors.New("database DSN leaked"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &handlerService{err: tt.err}
			handler := NewHandler(service)
			handler.SetAuthorizer(&handlerAuthorizer{allowed: true})
			req := requestWithConsoleIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/mcp-servers", nil), tenantID, userID)
			resp := httptest.NewRecorder()

			handler.ListMCPServerDefinitions(resp, req)

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

	mcpDefinition    MCPDefinition
	mcpBinding       MCPBinding
	effectiveServers []EffectiveMCPServer

	createDefinitionReq        CreateMCPServerDefinitionRequest
	listDefinitionsReq         ListMCPServerDefinitionsRequest
	deleteDefinitionReq        DeleteMCPServerDefinitionRequest
	createTeamBindingReq       CreateTeamMCPBindingRequest
	createEmployeeBindingV2Req CreateEmployeeMCPBindingV2Request
	effectiveConfigReq         EmployeeScopedRequest
	listProjectBindingsReq     ProjectScopedRequest
	putProjectBindingsReq      PutProjectMCPBindingsRequest

	skillMCPDependencies      []SkillMCPDependency
	dependentSkills           []DependentSkill
	listSkillDependenciesReq  ListSkillMCPDependenciesRequest
	replaceSkillDependencyReq ReplaceSkillMCPDependenciesRequest
	listDependentSkillsReq    ListDependentSkillsRequest

	employeeSkillMCPDependencyStatuses []EmployeeSkillMCPDependencyStatus
	evaluateEmployeeSkillMCPDepsReq    EvaluateEmployeeSkillMCPDependenciesRequest
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

func (s *handlerService) ListProjectMCPBindings(_ context.Context, req ProjectScopedRequest) ([]MCPBinding, error) {
	s.listProjectBindingsReq = req
	return []MCPBinding{s.mcpBinding}, s.err
}

func (s *handlerService) PutProjectMCPBindings(_ context.Context, req PutProjectMCPBindingsRequest) ([]MCPBinding, error) {
	s.putProjectBindingsReq = req
	return []MCPBinding{s.mcpBinding}, s.err
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

func (s *handlerService) EvaluateEmployeeSkillMCPDependencies(_ context.Context, req EvaluateEmployeeSkillMCPDependenciesRequest) ([]EmployeeSkillMCPDependencyStatus, error) {
	s.evaluateEmployeeSkillMCPDepsReq = req
	return s.employeeSkillMCPDependencyStatuses, s.err
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

func TestHandlerListEmployeeSkillMCPDependencyStatusReturnsRecordsWithReadAction(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	employeeID := uuid.New()
	skillID := uuid.New()
	serverID := uuid.New()
	service := &handlerService{
		employeeSkillMCPDependencyStatuses: []EmployeeSkillMCPDependencyStatus{
			{
				SkillID:   skillID,
				SkillSlug: "deploy-helper",
				Dependencies: []EmployeeSkillMCPDependencyItem{
					{
						MCPServerID:    serverID,
						ServerKey:      "github-mcp",
						ServerName:     "GitHub MCP",
						Status:         "missing_binding",
						MissingEnvVars: []string{},
					},
				},
			},
		},
	}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	req := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/skill-mcp-dependency-status", nil), map[string]string{"employeeId": employeeID.String()}),
		tenantID,
		userID,
	)
	resp := httptest.NewRecorder()
	handler.ListEmployeeSkillMCPDependencyStatus(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionMCPRegistryRead || authorizer.checks[0].Resource.Type != authz.ResourceTenant || authorizer.checks[0].Resource.ID != tenantID.String() {
		t.Fatalf("expected one tenant-scoped mcp_registry.read check, got %#v", authorizer.checks)
	}
	if service.evaluateEmployeeSkillMCPDepsReq.TenantID != tenantID || service.evaluateEmployeeSkillMCPDepsReq.UserID != userID || service.evaluateEmployeeSkillMCPDepsReq.DigitalEmployeeID != employeeID {
		t.Fatalf("unexpected evaluate request: %#v", service.evaluateEmployeeSkillMCPDepsReq)
	}

	var response []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("expected one skill status, got %#v", response)
	}
	got := response[0]
	if got["skill_id"] != skillID.String() || got["skill_slug"] != "deploy-helper" {
		t.Fatalf("unexpected skill status: %#v", got)
	}
	deps, ok := got["dependencies"].([]any)
	if !ok || len(deps) != 1 {
		t.Fatalf("expected one dependency, got %#v", got)
	}
	dep := deps[0].(map[string]any)
	if dep["mcp_server_id"] != serverID.String() || dep["server_key"] != "github-mcp" || dep["server_name"] != "GitHub MCP" || dep["status"] != "missing_binding" {
		t.Fatalf("unexpected dependency: %#v", dep)
	}
	if missingEnvVars, ok := dep["missing_env_vars"].([]any); !ok || len(missingEnvVars) != 0 {
		t.Fatalf("expected empty missing_env_vars, got %#v", dep["missing_env_vars"])
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

// TestHandlerProjectMCPBindingRoutesUseProjectConfigActions 验证项目绑定端点的 authz
// 形状：读走 project.config.read、写走 project.config.edit，资源都是 ResourceProject
// （项目归属裁决在 authorizer 的 checkProjectAccess 内完成，不携带 team 维度）。
func TestHandlerProjectMCPBindingRoutesUseProjectConfigActions(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	projectID := uuid.New()
	serverID := uuid.New()
	service := &handlerService{
		mcpBinding: MCPBinding{
			ID:          uuid.New(),
			TenantID:    tenantID,
			ProjectID:   &projectID,
			MCPServerID: serverID,
			ServerKey:   "github-mcp",
			Status:      "active",
			SourceScope: "project",
		},
	}
	handler := NewHandler(service)
	authorizer := &handlerAuthorizer{allowed: true}
	handler.SetAuthorizer(authorizer)

	listReq := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodGet, "/projects/"+projectID.String()+"/mcp-bindings", nil), map[string]string{"projectId": projectID.String()}),
		tenantID,
		userID,
	)
	listResp := httptest.NewRecorder()
	handler.ListProjectMCPBindings(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listProjectBindingsReq.TenantID != tenantID || service.listProjectBindingsReq.UserID != userID || service.listProjectBindingsReq.ProjectID != projectID {
		t.Fatalf("unexpected list project bindings request: %#v", service.listProjectBindingsReq)
	}
	var listed []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode project binding list: %v", err)
	}
	if len(listed) != 1 || listed[0]["project_id"] != projectID.String() || listed[0]["source_scope"] != "project" {
		t.Fatalf("expected project-scoped binding response, got %#v", listed)
	}

	putReq := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodPut, "/projects/"+projectID.String()+"/mcp-bindings", bytes.NewBufferString(`{"items":[{"mcp_server_id":"`+serverID.String()+`","credential_env_var":"GH_TOKEN"}]}`)), map[string]string{"projectId": projectID.String()}),
		tenantID,
		userID,
	)
	putResp := httptest.NewRecorder()
	handler.PutProjectMCPBindings(putResp, putReq)
	if putResp.Code != http.StatusOK {
		t.Fatalf("expected put status 200, got %d: %s", putResp.Code, putResp.Body.String())
	}
	if service.putProjectBindingsReq.TenantID != tenantID || service.putProjectBindingsReq.UserID != userID || service.putProjectBindingsReq.ProjectID != projectID {
		t.Fatalf("unexpected put project bindings request: %#v", service.putProjectBindingsReq)
	}
	if len(service.putProjectBindingsReq.Items) != 1 || service.putProjectBindingsReq.Items[0].MCPServerID != serverID || service.putProjectBindingsReq.Items[0].CredentialEnvVar != "GH_TOKEN" {
		t.Fatalf("unexpected put project bindings items: %#v", service.putProjectBindingsReq.Items)
	}

	if len(authorizer.checks) != 2 {
		t.Fatalf("expected two authz checks, got %#v", authorizer.checks)
	}
	if authorizer.checks[0].Action != authz.ActionProjectConfigRead || authorizer.checks[0].Resource.Type != authz.ResourceProject || authorizer.checks[0].Resource.ID != projectID.String() || authorizer.checks[0].TeamID != nil {
		t.Fatalf("unexpected project binding read authz check: %#v", authorizer.checks[0])
	}
	if authorizer.checks[1].Action != authz.ActionProjectConfigEdit || authorizer.checks[1].Resource.Type != authz.ResourceProject || authorizer.checks[1].Resource.ID != projectID.String() || authorizer.checks[1].TeamID != nil {
		t.Fatalf("unexpected project binding edit authz check: %#v", authorizer.checks[1])
	}
}

func TestHandlerPutProjectMCPBindingsRejectsMalformedBody(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	projectID := uuid.New()

	tests := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `{"items":`},
		{name: "invalid server id", body: `{"items":[{"mcp_server_id":"not-a-uuid"}]}`},
		// 键名手滑不得被宽容为清空(残债交接 §3):缺 items 必须 400。
		{name: "missing items key", body: `{"bindings":[]}`},
		{name: "empty object", body: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &handlerService{}
			handler := NewHandler(service)
			handler.SetAuthorizer(&handlerAuthorizer{allowed: true})
			req := requestWithConsoleIdentity(
				requestWithChiParams(httptest.NewRequest(http.MethodPut, "/projects/"+projectID.String()+"/mcp-bindings", bytes.NewBufferString(tt.body)), map[string]string{"projectId": projectID.String()}),
				tenantID,
				userID,
			)
			resp := httptest.NewRecorder()
			handler.PutProjectMCPBindings(resp, req)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d: %s", resp.Code, resp.Body.String())
			}
			if service.putProjectBindingsReq.ProjectID != uuid.Nil {
				t.Fatalf("service must not be called for malformed body")
			}
		})
	}
}

func TestHandlerProjectMCPBindingRoutesDenyWhenAuthorizerRejects(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	projectID := uuid.New()
	service := &handlerService{}
	handler := NewHandler(service)
	handler.SetAuthorizer(&handlerAuthorizer{allowed: false})

	req := requestWithConsoleIdentity(
		requestWithChiParams(httptest.NewRequest(http.MethodPut, "/projects/"+projectID.String()+"/mcp-bindings", bytes.NewBufferString(`{"items":[]}`)), map[string]string{"projectId": projectID.String()}),
		tenantID,
		userID,
	)
	resp := httptest.NewRecorder()
	handler.PutProjectMCPBindings(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.putProjectBindingsReq.ProjectID != uuid.Nil {
		t.Fatalf("service must not be called when authorization is denied")
	}
}
