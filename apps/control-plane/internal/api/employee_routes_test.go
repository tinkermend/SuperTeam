package api

import (
	"context"
	"encoding/json"
	"errors"
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
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
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
	teamID := uuid.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees", strings.NewReader(`{"team_id":"`+teamID.String()+`","name":"Requirements analyst","role":"requirements_analyst"}`))
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
	if service.createReq.TeamID == nil || *service.createReq.TeamID != teamID {
		t.Fatalf("expected create team %s, got %#v", teamID, service.createReq.TeamID)
	}
	var created struct {
		ID               string         `json:"id"`
		TenantID         string         `json:"tenant_id"`
		TeamID           string         `json:"team_id"`
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
	if created.TeamID != teamID.String() {
		t.Fatalf("expected response team %s, got %s", teamID, created.TeamID)
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

	spoofedConfigApproverID := uuid.New()
	configReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+created.ID+"/config-revisions", strings.NewReader(`{"role_profile":{"title":"requirements analyst"},"capability_selection":{"enabled_skills":["incident-diagnosis"]},"approved_by":"`+spoofedConfigApproverID.String()+`"}`))
	configReq.Header.Set("Content-Type", "application/json")
	configReq.AddCookie(cookie)
	configResp := httptest.NewRecorder()
	server.ServeHTTP(configResp, configReq)
	if configResp.Code != http.StatusCreated {
		t.Fatalf("expected create config revision to succeed, got %d: %s", configResp.Code, configResp.Body.String())
	}
	employeeID := uuid.MustParse(created.ID)
	if service.configRevisionReq.TenantID != expectedTenantID || service.configRevisionReq.DigitalEmployeeID != employeeID {
		t.Fatalf("expected config revision tenant/employee %s/%s, got %s/%s", expectedTenantID, employeeID, service.configRevisionReq.TenantID, service.configRevisionReq.DigitalEmployeeID)
	}
	if service.configRevisionReq.RoleProfile["title"] != "requirements analyst" {
		t.Fatalf("expected role profile from request, got %#v", service.configRevisionReq.RoleProfile)
	}
	if service.configRevisionReq.ApprovedBy != nil {
		t.Fatalf("expected handler not to forward client approved_by %s for draft config revision, got %#v", spoofedConfigApproverID, service.configRevisionReq.ApprovedBy)
	}

	teamConfigRevisionID := uuid.New()
	employeeConfigRevisionID := uuid.New()
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+created.ID+"/effective-configs/preview", strings.NewReader(`{"team_config":{"id":"`+teamConfigRevisionID.String()+`"},"employee_config":{"id":"`+employeeConfigRevisionID.String()+`"}}`))
	previewReq.Header.Set("Content-Type", "application/json")
	previewReq.AddCookie(cookie)
	previewResp := httptest.NewRecorder()
	server.ServeHTTP(previewResp, previewReq)
	if previewResp.Code != http.StatusOK {
		t.Fatalf("expected preview effective config to succeed, got %d: %s", previewResp.Code, previewResp.Body.String())
	}
	if service.previewReq.TenantID != expectedTenantID || service.previewReq.DigitalEmployeeID != employeeID || service.previewReq.TeamConfigRevisionID != teamConfigRevisionID || service.previewReq.EmployeeConfigRevisionID != employeeConfigRevisionID {
		t.Fatalf("unexpected preview request mapping: %#v", service.previewReq)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+created.ID+"/effective-configs/approve", strings.NewReader(`{"preview":{"team_config":{"id":"`+teamConfigRevisionID.String()+`"},"employee_config":{"id":"`+employeeConfigRevisionID.String()+`"}}}`))
	approveReq.Header.Set("Content-Type", "application/json")
	approveReq.AddCookie(cookie)
	approveResp := httptest.NewRecorder()
	server.ServeHTTP(approveResp, approveReq)
	if approveResp.Code != http.StatusCreated {
		t.Fatalf("expected approve effective config to succeed, got %d: %s", approveResp.Code, approveResp.Body.String())
	}
	if service.approveReq.TenantID != expectedTenantID || service.approveReq.DigitalEmployeeID != employeeID || service.approveReq.ApprovedBy != user.ID {
		t.Fatalf("unexpected approve request mapping: %#v", service.approveReq)
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
		{name: "create", method: http.MethodPost, path: "/api/v1/digital-employees", body: `{"team_id":"` + uuid.New().String() + `","name":"Requirements analyst","role":"requirements_analyst"}`},
		{name: "get", method: http.MethodGet, path: "/api/v1/digital-employees/" + employeeID},
		{name: "status", method: http.MethodPut, path: "/api/v1/digital-employees/" + employeeID + "/status", body: `{"status":"active"}`},
		{name: "get execution instance", method: http.MethodGet, path: "/api/v1/digital-employees/" + employeeID + "/execution-instance"},
		{name: "upsert execution instance", method: http.MethodPut, path: "/api/v1/digital-employees/" + employeeID + "/execution-instance", body: `{"runtime_node_id":"` + runtimeNodeID + `","provider_type":"codex","agent_home_dir":"/srv/agents/requirements"}`},
		{name: "create config revision", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/config-revisions", body: `{"role_profile":{"title":"analyst"}}`},
		{name: "preview effective config", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/effective-configs/preview", body: `{"team_config":{"id":"` + uuid.New().String() + `"},"employee_config":{"id":"` + uuid.New().String() + `"}}`},
		{name: "approve effective config", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/effective-configs/approve", body: `{"preview":{"team_config":{"id":"` + uuid.New().String() + `"},"employee_config":{"id":"` + uuid.New().String() + `"}}}`},
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

func TestDigitalEmployeeRouteSanitizesInternalServiceError(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeEmployeeService{listErr: errors.New("sensitive database password leaked")}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetEmployeeHandler(employee.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected internal service error to return 500, got %d: %s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "sensitive") || !strings.Contains(resp.Body.String(), "internal server error") {
		t.Fatalf("expected sanitized internal service error, got %q", resp.Body.String())
	}
}

func TestDigitalEmployeeRouteSanitizesAuthorizationBackendError(t *testing.T) {
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
		&routeAuthorizer{allowed: true, err: errors.New("sensitive policy backend failure")},
	)
	server.SetEmployeeHandler(employee.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected authz backend error to return 500, got %d: %s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "sensitive") || !strings.Contains(resp.Body.String(), "internal server error") {
		t.Fatalf("expected sanitized authz backend error, got %q", resp.Body.String())
	}
	if service.called() {
		t.Fatalf("expected authz backend error not to call employee service")
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
	configRevisionReq   employee.CreateDigitalEmployeeConfigRevisionRequest
	previewReq          employee.PreviewEffectiveConfigByRevisionIDsRequest
	approveReq          employee.ApproveEffectiveConfigRequest
	configCalled        bool
	previewCalled       bool
	approveCalled       bool
	createdID           uuid.UUID
	listErr             error
}

func (s *routeEmployeeService) CreateDraft(ctx context.Context, req employee.CreateDraftRequest) (*employee.DigitalEmployee, error) {
	s.createCalled = true
	s.createReq = req
	s.createdID = uuid.New()
	now := time.Now().UTC()
	return &employee.DigitalEmployee{
		ID:               s.createdID,
		TenantID:         req.TenantID,
		TeamID:           req.TeamID,
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
	if s.listErr != nil {
		return nil, s.listErr
	}
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

func (s *routeEmployeeService) CreateConfigRevision(ctx context.Context, req employee.CreateDigitalEmployeeConfigRevisionRequest) (*employee.DigitalEmployeeConfigRevision, error) {
	s.configCalled = true
	s.configRevisionReq = req
	now := time.Now().UTC()
	return &employee.DigitalEmployeeConfigRevision{
		ID:                     uuid.New(),
		TenantID:               req.TenantID,
		DigitalEmployeeID:      req.DigitalEmployeeID,
		RevisionNumber:         1,
		RoleProfile:            req.RoleProfile,
		ConstitutionAddendum:   req.ConstitutionAddendum,
		CapabilitySelection:    req.CapabilitySelection,
		ContextPolicyOverride:  req.ContextPolicyOverride,
		ApprovalPolicyOverride: req.ApprovalPolicyOverride,
		OutputContractAddendum: req.OutputContractAddendum,
		Status:                 employee.ConfigRevisionStatusDraft,
		CreatedAt:              now,
		UpdatedAt:              now,
	}, nil
}

func (s *routeEmployeeService) PreviewEffectiveConfigByRevisionIDs(ctx context.Context, req employee.PreviewEffectiveConfigByRevisionIDsRequest) (*employee.EffectiveConfigPreview, error) {
	s.previewCalled = true
	s.previewReq = req
	return &employee.EffectiveConfigPreview{
		TeamConfigRevisionID:     req.TeamConfigRevisionID,
		EmployeeConfigRevisionID: req.EmployeeConfigRevisionID,
		EffectiveConfig:          map[string]any{"team_config_revision_id": req.TeamConfigRevisionID.String()},
		Validation:               employee.EffectiveConfigValidation{BlockingErrors: []employee.ValidationIssue{}, Warnings: []employee.ValidationIssue{}},
	}, nil
}

func (s *routeEmployeeService) ApproveEffectiveConfig(ctx context.Context, req employee.ApproveEffectiveConfigRequest) (*employee.DigitalEmployeeEffectiveConfig, error) {
	s.approveCalled = true
	s.approveReq = req
	now := time.Now().UTC()
	return &employee.DigitalEmployeeEffectiveConfig{
		ID:                       uuid.New(),
		TenantID:                 req.TenantID,
		DigitalEmployeeID:        req.DigitalEmployeeID,
		TeamConfigRevisionID:     req.TeamConfigRevisionID,
		EmployeeConfigRevisionID: req.EmployeeConfigRevisionID,
		EffectiveConfig:          map[string]any{"approved": true},
		ValidationResult:         map[string]any{"blocking_errors": []any{}},
		Status:                   employee.EffectiveConfigStatusApproved,
		ApprovedBy:               &req.ApprovedBy,
		ApprovedAt:               &now,
		CreatedAt:                now,
		UpdatedAt:                now,
	}, nil
}

func (s *routeEmployeeService) called() bool {
	return s.createCalled ||
		s.listCalled ||
		s.getCalled ||
		s.updateCalled ||
		s.getInstanceCalled ||
		s.bindCalled ||
		s.configCalled ||
		s.previewCalled ||
		s.approveCalled
}

var _ employee.HandlerService = (*routeEmployeeService)(nil)
