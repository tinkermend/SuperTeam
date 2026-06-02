package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/employee"
)

func TestDigitalEmployeeRoutesUseConsoleTenant(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeEmployeeService{}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetEmployeeHandler(employee.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees", strings.NewReader(`{"name":"Requirements analyst","role":"requirements_analyst"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create digital employee to succeed, got %d: %s", createResp.Code, createResp.Body.String())
	}
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	if service.createReq.TenantID != expectedTenantID {
		t.Fatalf("expected create tenant %s, got %s", expectedTenantID, service.createReq.TenantID)
	}
	var created struct {
		ID               string         `json:"id"`
		TenantID         string         `json:"tenant_id"`
		PermissionPolicy map[string]any `json:"permission_policy"`
		ContextPolicy    map[string]any `json:"context_policy"`
		ApprovalPolicy   map[string]any `json:"approval_policy"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created employee: %v", err)
	}
	if created.TenantID != expectedTenantID.String() {
		t.Fatalf("expected response tenant %s, got %s", expectedTenantID, created.TenantID)
	}
	if created.PermissionPolicy == nil || created.ContextPolicy == nil || created.ApprovalPolicy == nil {
		t.Fatalf("expected policy objects in response, got %#v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list digital employees to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listReq.TenantID != expectedTenantID {
		t.Fatalf("expected list tenant %s, got %s", expectedTenantID, service.listReq.TenantID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+created.ID, nil)
	getReq.AddCookie(cookie)
	getResp := httptest.NewRecorder()
	server.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected get digital employee to succeed, got %d: %s", getResp.Code, getResp.Body.String())
	}
	if service.getTenantID != expectedTenantID {
		t.Fatalf("expected get tenant %s, got %s", expectedTenantID, service.getTenantID)
	}

	runtimeNodeID := uuid.New()
	upsertReq := httptest.NewRequest(http.MethodPut, "/api/v1/digital-employees/"+created.ID+"/execution-instance", strings.NewReader(`{"runtime_node_id":"`+runtimeNodeID.String()+`","provider_type":"codex","agent_home_dir":"/srv/agents/requirements","workspace_policy":{},"session_policy":{}}`))
	upsertReq.Header.Set("Content-Type", "application/json")
	upsertReq.AddCookie(cookie)
	upsertResp := httptest.NewRecorder()
	server.ServeHTTP(upsertResp, upsertReq)
	if upsertResp.Code != http.StatusOK {
		t.Fatalf("expected upsert execution instance to succeed, got %d: %s", upsertResp.Code, upsertResp.Body.String())
	}
	if service.bindReq.TenantID != expectedTenantID || service.bindReq.RuntimeNodeID != runtimeNodeID {
		t.Fatalf("expected bind tenant/runtime %s/%s, got %s/%s", expectedTenantID, runtimeNodeID, service.bindReq.TenantID, service.bindReq.RuntimeNodeID)
	}
}

func TestDigitalEmployeeRoutesRequireConsoleAuth(t *testing.T) {
	service := &routeEmployeeService{}
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
	)
	server.SetEmployeeHandler(employee.NewHandler(service))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees", nil)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated digital employee route to return 401, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.listCalled {
		t.Fatalf("expected unauthenticated request not to call employee service")
	}
}

func TestDigitalEmployeeRoutesRequireManagementAuthorization(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeEmployeeService{}
	authorizer := &routeAuthorizer{allowed: false}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	server.SetEmployeeHandler(employee.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	employeeID := uuid.New().String()
	runtimeNodeID := uuid.New().String()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/digital-employees"},
		{name: "create", method: http.MethodPost, path: "/api/v1/digital-employees", body: `{"name":"Requirements analyst","role":"requirements_analyst"}`},
		{name: "get", method: http.MethodGet, path: "/api/v1/digital-employees/" + employeeID},
		{name: "status", method: http.MethodPut, path: "/api/v1/digital-employees/" + employeeID + "/status", body: `{"status":"active"}`},
		{name: "get execution instance", method: http.MethodGet, path: "/api/v1/digital-employees/" + employeeID + "/execution-instance"},
		{name: "upsert execution instance", method: http.MethodPut, path: "/api/v1/digital-employees/" + employeeID + "/execution-instance", body: `{"runtime_node_id":"` + runtimeNodeID + `","provider_type":"codex","agent_home_dir":"/srv/agents/requirements"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookie)
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			if resp.Code != http.StatusForbidden {
				t.Fatalf("expected forbidden digital employee route, got %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
	if service.called() {
		t.Fatalf("expected denied requests not to call employee service")
	}
	if len(authorizer.checks) != len(tests) {
		t.Fatalf("expected one authorization check per request, got %#v", authorizer.checks)
	}
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	for _, check := range authorizer.checks {
		if check.Action != authz.ActionRuntimeScopeManage {
			t.Fatalf("expected runtime scope management action, got %#v", check)
		}
		if check.Actor.Type != authz.ActorUser {
			t.Fatalf("expected user actor, got %#v", check)
		}
		if check.Resource.Type != authz.ResourceTenant || check.Resource.ID != expectedTenantID.String() || check.TenantID != expectedTenantID {
			t.Fatalf("expected tenant resource %s, got %#v", expectedTenantID, check)
		}
	}
}

func TestDigitalEmployeeRouteRejectsUnconfiguredService(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetEmployeeHandler(employee.NewHandler(nil))
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured employee service to return 503, got %d: %s", resp.Code, resp.Body.String())
	}
}

type routeEmployeeService struct {
	createReq           employee.CreateDraftRequest
	listReq             employee.ListDigitalEmployeesRequest
	bindReq             employee.BindExecutionInstanceRequest
	updateReq           employee.UpdateStatusRequest
	getTenantID         uuid.UUID
	getInstanceTenantID uuid.UUID
	createCalled        bool
	listCalled          bool
	getCalled           bool
	updateCalled        bool
	getInstanceCalled   bool
	bindCalled          bool
	createdID           uuid.UUID
}

func (s *routeEmployeeService) CreateDraft(ctx context.Context, req employee.CreateDraftRequest) (*employee.DigitalEmployee, error) {
	s.createCalled = true
	s.createReq = req
	s.createdID = uuid.New()
	now := time.Now().UTC()
	return &employee.DigitalEmployee{
		ID:               s.createdID,
		TenantID:         req.TenantID,
		Name:             req.Name,
		Role:             req.Role,
		Status:           employee.DigitalEmployeeStatusDraft,
		PermissionPolicy: map[string]any{},
		ContextPolicy:    map[string]any{},
		ApprovalPolicy:   map[string]any{},
		RiskLevel:        "medium",
		Metadata:         map[string]any{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeEmployeeService) ListDigitalEmployees(ctx context.Context, req employee.ListDigitalEmployeesRequest) ([]*employee.DigitalEmployee, error) {
	s.listCalled = true
	s.listReq = req
	return []*employee.DigitalEmployee{}, nil
}

func (s *routeEmployeeService) GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*employee.DigitalEmployee, error) {
	s.getCalled = true
	s.getTenantID = tenantID
	now := time.Now().UTC()
	return &employee.DigitalEmployee{
		ID:               employeeID,
		TenantID:         tenantID,
		Name:             "Requirements analyst",
		Role:             "requirements_analyst",
		Status:           employee.DigitalEmployeeStatusDraft,
		PermissionPolicy: map[string]any{},
		ContextPolicy:    map[string]any{},
		ApprovalPolicy:   map[string]any{},
		RiskLevel:        "medium",
		Metadata:         map[string]any{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeEmployeeService) UpdateStatus(ctx context.Context, req employee.UpdateStatusRequest) (*employee.DigitalEmployee, error) {
	s.updateCalled = true
	s.updateReq = req
	now := time.Now().UTC()
	return &employee.DigitalEmployee{
		ID:               req.DigitalEmployeeID,
		TenantID:         req.TenantID,
		Name:             "Requirements analyst",
		Role:             "requirements_analyst",
		Status:           req.Status,
		PermissionPolicy: map[string]any{},
		ContextPolicy:    map[string]any{},
		ApprovalPolicy:   map[string]any{},
		RiskLevel:        "medium",
		Metadata:         map[string]any{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeEmployeeService) GetExecutionInstance(ctx context.Context, tenantID, employeeID uuid.UUID) (*employee.DigitalEmployeeExecutionInstance, error) {
	s.getInstanceCalled = true
	s.getInstanceTenantID = tenantID
	now := time.Now().UTC()
	return &employee.DigitalEmployeeExecutionInstance{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		DigitalEmployeeID:    employeeID,
		RuntimeNodeID:        uuid.New(),
		ProviderType:         "codex",
		AgentHomeDir:         "/srv/agents/requirements",
		WorkspacePolicy:      map[string]any{},
		SessionPolicy:        map[string]any{},
		RuntimeSelector:      map[string]any{},
		CapacityRequirements: map[string]any{},
		FallbackPolicy:       map[string]any{},
		Status:               employee.ExecutionInstanceStatusReady,
		Metadata:             map[string]any{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}, nil
}

func (s *routeEmployeeService) BindExecutionInstance(ctx context.Context, req employee.BindExecutionInstanceRequest) (*employee.DigitalEmployeeExecutionInstance, error) {
	s.bindCalled = true
	s.bindReq = req
	now := time.Now().UTC()
	return &employee.DigitalEmployeeExecutionInstance{
		ID:                   uuid.New(),
		TenantID:             req.TenantID,
		DigitalEmployeeID:    req.DigitalEmployeeID,
		RuntimeNodeID:        req.RuntimeNodeID,
		ProviderType:         req.ProviderType,
		AgentHomeDir:         req.AgentHomeDir,
		WorkspacePolicy:      map[string]any{},
		SessionPolicy:        map[string]any{},
		RuntimeSelector:      map[string]any{},
		CapacityRequirements: map[string]any{},
		FallbackPolicy:       map[string]any{},
		Status:               employee.ExecutionInstanceStatusReady,
		Metadata:             map[string]any{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}, nil
}

func (s *routeEmployeeService) called() bool {
	return s.createCalled ||
		s.listCalled ||
		s.getCalled ||
		s.updateCalled ||
		s.getInstanceCalled ||
		s.bindCalled
}

var _ employee.HandlerService = (*routeEmployeeService)(nil)
