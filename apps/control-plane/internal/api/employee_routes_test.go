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
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/platform"
)

func derefTeamID(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func uuidPtrFromNilable(value uuid.UUID) *uuid.UUID {
	return &value
}

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
	runtimeNodeID := uuid.New()
	spoofedOwnerID := uuid.New()

	optionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/create-options?team_id="+teamID.String(), nil)
	optionsReq.AddCookie(cookie)
	optionsResp := httptest.NewRecorder()
	server.ServeHTTP(optionsResp, optionsReq)
	if optionsResp.Code != http.StatusOK {
		t.Fatalf("expected create options to succeed, got %d: %s", optionsResp.Code, optionsResp.Body.String())
	}
	expectedTenantID := platform.DefaultTenantID
	if service.createOptionsReq.TenantID != expectedTenantID || service.createOptionsReq.TeamID == nil || *service.createOptionsReq.TeamID != teamID {
		t.Fatalf("expected create options tenant/team %s/%s, got %#v", expectedTenantID, teamID, service.createOptionsReq)
	}
	var optionsBody struct {
		TeamConfig struct {
			TeamID     string   `json:"team_id"`
			Skills     []string `json:"skills"`
			MCPServers []string `json:"mcp_servers"`
		} `json:"team_config"`
		EmployeeTypes []struct {
			Type string `json:"type"`
		} `json:"employee_types"`
		CapabilityOptions struct {
			ProviderTypes []string `json:"provider_types"`
		} `json:"capability_options"`
		RuntimeProviderOptions []struct {
			RuntimeNodeID string `json:"runtime_node_id"`
			ProviderType  string `json:"provider_type"`
		} `json:"runtime_provider_options"`
		PolicyDefaults struct {
			SessionPolicy map[string]any `json:"session_policy"`
		} `json:"policy_defaults"`
		CreationChecks []struct {
			Key     string `json:"key"`
			Label   string `json:"label"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"creation_checks"`
	}
	if err := json.NewDecoder(optionsResp.Body).Decode(&optionsBody); err != nil {
		t.Fatalf("decode create options: %v", err)
	}
	if optionsBody.TeamConfig.TeamID != teamID.String() {
		t.Fatalf("expected team config team_id %s, got %#v", teamID, optionsBody.TeamConfig)
	}
	if len(optionsBody.TeamConfig.Skills) != 1 || optionsBody.TeamConfig.Skills[0] != "database_admin" {
		t.Fatalf("expected team config skills to reflect team baseline, got %#v", optionsBody.TeamConfig.Skills)
	}
	if len(optionsBody.TeamConfig.MCPServers) != 0 || optionsBody.TeamConfig.MCPServers == nil {
		t.Fatalf("expected team config mcp_servers to decode as [], got %#v", optionsBody.TeamConfig.MCPServers)
	}
	if len(optionsBody.EmployeeTypes) != 1 || optionsBody.EmployeeTypes[0].Type != "database_admin" {
		t.Fatalf("expected employee type options, got %#v", optionsBody.EmployeeTypes)
	}
	if len(optionsBody.CapabilityOptions.ProviderTypes) != 1 || optionsBody.CapabilityOptions.ProviderTypes[0] != "codex" {
		t.Fatalf("expected capability options, got %#v", optionsBody.CapabilityOptions)
	}
	if len(optionsBody.RuntimeProviderOptions) != 1 || optionsBody.RuntimeProviderOptions[0].RuntimeNodeID == "" || optionsBody.RuntimeProviderOptions[0].ProviderType != "codex" {
		t.Fatalf("expected runtime provider options, got %#v", optionsBody.RuntimeProviderOptions)
	}
	if optionsBody.PolicyDefaults.SessionPolicy["mode"] != "reuse_latest" {
		t.Fatalf("expected policy defaults, got %#v", optionsBody.PolicyDefaults)
	}
	assertCreateOptionCheck(t, optionsBody.CreationChecks, "team_baseline", "passed")
	assertCreateOptionCheck(t, optionsBody.CreationChecks, "employee_templates", "passed")
	assertCreateOptionCheck(t, optionsBody.CreationChecks, "runtime_provider", "passed")

	teamlessReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/create-options", nil)
	teamlessReq.AddCookie(cookie)
	teamlessResp := httptest.NewRecorder()
	server.ServeHTTP(teamlessResp, teamlessReq)
	if teamlessResp.Code != http.StatusOK {
		t.Fatalf("expected team-less create options to succeed, got %d: %s", teamlessResp.Code, teamlessResp.Body.String())
	}
	var teamlessBody struct {
		TeamConfig struct {
			TeamID *string `json:"team_id,omitempty"`
		} `json:"team_config"`
	}
	if err := json.NewDecoder(teamlessResp.Body).Decode(&teamlessBody); err != nil {
		t.Fatalf("decode team-less create options: %v", err)
	}
	if teamlessBody.TeamConfig.TeamID != nil || strings.Contains(teamlessResp.Body.String(), `"team_id"`) {
		t.Fatalf("expected team-less team_config to omit team_id, got %s", teamlessResp.Body.String())
	}

	avatarReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employee-avatar-assets", nil)
	avatarReq.AddCookie(cookie)
	avatarResp := httptest.NewRecorder()
	server.ServeHTTP(avatarResp, avatarReq)
	if avatarResp.Code != http.StatusOK {
		t.Fatalf("expected avatar assets to succeed, got %d: %s", avatarResp.Code, avatarResp.Body.String())
	}
	var avatarBody []struct {
		ID           string `json:"id"`
		ThumbnailURL string `json:"thumbnail_url"`
		Status       string `json:"status"`
	}
	if err := json.NewDecoder(avatarResp.Body).Decode(&avatarBody); err != nil {
		t.Fatalf("decode avatar assets: %v", err)
	}
	if len(avatarBody) == 0 || avatarBody[0].ID == "" || avatarBody[0].ThumbnailURL == "" || avatarBody[0].Status != "active" {
		t.Fatalf("expected active avatar assets, got %#v", avatarBody)
	}

	createBody := `{
		"team_id":"` + teamID.String() + `",
		"owner_user_id":"` + spoofedOwnerID.String() + `",
		"employee_type":"database_admin",
		"name":"Database administrator",
		"avatar_asset_id":"engineer-m-01",
		"role":"database_admin",
		"description":"Manages database operations",
		"permission_policy":{"allowed_actions":["read_context"]},
		"risk_level":"medium",
		"metadata":{"source":"route-test"},
		"persona_memory_markdown":"# Database administrator",
		"capability_bindings":{"skills":["incident-diagnosis"],"mcp_servers":["postgres-readonly"]},
		"budget_policy":{"daily_token_limit":12000},
		"provider_type":"codex"
	}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create digital employee to succeed, got %d: %s", createResp.Code, createResp.Body.String())
	}
	if service.createReq.TenantID != expectedTenantID {
		t.Fatalf("expected create tenant %s, got %s", expectedTenantID, service.createReq.TenantID)
	}
	if service.createReq.TeamID == nil || *service.createReq.TeamID != teamID {
		t.Fatalf("expected create team %s, got %#v", teamID, service.createReq.TeamID)
	}
	if service.createReq.OwnerUserID != user.ID || service.createReq.OwnerUserID == spoofedOwnerID {
		t.Fatalf("expected create owner from console user %s, got %s", user.ID, service.createReq.OwnerUserID)
	}
	if service.createReq.EmployeeType != "database_admin" {
		t.Fatalf("expected employee type from create body, got %q", service.createReq.EmployeeType)
	}
	if service.createReq.AvatarAssetID != "engineer-m-01" {
		t.Fatalf("expected avatar asset id from create body, got %q", service.createReq.AvatarAssetID)
	}
	if service.createReq.ProviderType != "codex" {
		t.Fatalf("expected create provider codex, got %q", service.createReq.ProviderType)
	}
	if service.createReq.PermissionPolicy["allowed_actions"] == nil || service.createReq.PersonaMemoryMarkdown != "# Database administrator" || service.createReq.CapabilityBindings["skills"] == nil {
		t.Fatalf("expected policy/config fields from create body, got %#v", service.createReq)
	}
	if service.createReq.BudgetPolicy["daily_token_limit"] != float64(12000) {
		t.Fatalf("expected budget policy from create body, got %#v", service.createReq.BudgetPolicy)
	}
	var created struct {
		ID               string         `json:"id"`
		TenantID         string         `json:"tenant_id"`
		TeamID           string         `json:"team_id"`
		OwnerUserID      string         `json:"owner_user_id"`
		EmployeeType     string         `json:"employee_type"`
		ProviderType     string         `json:"provider_type"`
		PermissionPolicy map[string]any `json:"permission_policy"`
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
	if created.OwnerUserID != user.ID.String() || created.EmployeeType != "database_admin" || created.ProviderType != "codex" {
		t.Fatalf("expected response owner/type/provider %s/database_admin/codex, got %#v", user.ID, created)
	}
	if created.PermissionPolicy == nil {
		t.Fatalf("expected permission policy object in response, got %#v", created)
	}

	removedCreateFieldReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees", strings.NewReader(`{"employee_type":"database_admin","name":"Legacy runtime placement","avatar_asset_id":"engineer-m-01","provider_type":"codex","runtime_node_id":"`+runtimeNodeID.String()+`"}`))
	removedCreateFieldReq.Header.Set("Content-Type", "application/json")
	removedCreateFieldReq.AddCookie(cookie)
	removedCreateFieldResp := httptest.NewRecorder()
	server.ServeHTTP(removedCreateFieldResp, removedCreateFieldReq)
	if removedCreateFieldResp.Code != http.StatusBadRequest || !strings.Contains(removedCreateFieldResp.Body.String(), "runtime_node_id is no longer supported") {
		t.Fatalf("expected employee-owned runtime placement to be rejected, got %d: %s", removedCreateFieldResp.Code, removedCreateFieldResp.Body.String())
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
	var listed []struct {
		OwnerUserID  string `json:"owner_user_id"`
		EmployeeType string `json:"employee_type"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed employees: %v", err)
	}
	if len(listed) != 1 || listed[0].OwnerUserID != user.ID.String() || listed[0].EmployeeType != "database_admin" {
		t.Fatalf("expected list response owner/type, got %#v", listed)
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
	var got struct {
		OwnerUserID  string `json:"owner_user_id"`
		EmployeeType string `json:"employee_type"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get employee: %v", err)
	}
	if got.OwnerUserID != user.ID.String() || got.EmployeeType != "database_admin" {
		t.Fatalf("expected get response owner/type, got %#v", got)
	}

	spoofedConfigApproverID := uuid.New()
	configReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+created.ID+"/config-revisions", strings.NewReader(`{"persona_memory_markdown":"# requirements analyst","capability_bindings":{"skills":["incident-diagnosis"]},"budget_policy":{"daily_token_limit":9000},"approved_by":"`+spoofedConfigApproverID.String()+`"}`))
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
	if service.configRevisionReq.PersonaMemoryMarkdown == nil || *service.configRevisionReq.PersonaMemoryMarkdown != "# requirements analyst" {
		t.Fatalf("expected persona memory from request, got %#v", service.configRevisionReq.PersonaMemoryMarkdown)
	}
	if service.configRevisionReq.BudgetPolicy["daily_token_limit"] != float64(9000) {
		t.Fatalf("expected budget policy from config request, got %#v", service.configRevisionReq.BudgetPolicy)
	}
	if service.configRevisionReq.ApprovedBy != nil {
		t.Fatalf("expected handler not to forward client approved_by %s for draft config revision, got %#v", spoofedConfigApproverID, service.configRevisionReq.ApprovedBy)
	}
	var configCreated struct {
		BudgetPolicy map[string]any `json:"budget_policy"`
	}
	if err := json.NewDecoder(configResp.Body).Decode(&configCreated); err != nil {
		t.Fatalf("decode created config revision: %v", err)
	}
	if configCreated.BudgetPolicy["daily_token_limit"] != float64(9000) {
		t.Fatalf("expected budget policy in config response, got %#v", configCreated.BudgetPolicy)
	}

	legacyConfigReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+created.ID+"/config-revisions", strings.NewReader(`{"role_profile":{"title":"legacy"},"persona_memory_markdown":"# legacy"}`))
	legacyConfigReq.Header.Set("Content-Type", "application/json")
	legacyConfigReq.AddCookie(cookie)
	legacyConfigResp := httptest.NewRecorder()
	server.ServeHTTP(legacyConfigResp, legacyConfigReq)
	if legacyConfigResp.Code != http.StatusBadRequest {
		t.Fatalf("expected legacy config fields to be rejected, got %d: %s", legacyConfigResp.Code, legacyConfigResp.Body.String())
	}
	if !strings.Contains(legacyConfigResp.Body.String(), "role_profile is no longer supported") {
		t.Fatalf("expected legacy field rejection body, got %s", legacyConfigResp.Body.String())
	}

	readinessReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+created.ID+"/scheduling-readiness", nil)
	readinessReq.AddCookie(cookie)
	readinessResp := httptest.NewRecorder()
	server.ServeHTTP(readinessResp, readinessReq)
	if readinessResp.Code != http.StatusOK {
		t.Fatalf("expected get scheduling readiness to succeed, got %d: %s", readinessResp.Code, readinessResp.Body.String())
	}
	if !service.getSchedulingReadinessCalled || service.getSchedulingReadinessTenant != expectedTenantID || service.getSchedulingReadinessEmployeeID != employeeID {
		t.Fatalf(
			"unexpected scheduling readiness request mapping: called=%v tenant=%s employee=%s",
			service.getSchedulingReadinessCalled,
			service.getSchedulingReadinessTenant,
			service.getSchedulingReadinessEmployeeID,
		)
	}
	var readinessBody struct {
		EmployeeID                uuid.UUID `json:"employee_id"`
		Status                    string    `json:"status"`
		ReadyForProjectScheduling bool      `json:"ready_for_project_scheduling"`
		ProjectExecutionSource    string    `json:"project_execution_source"`
		Checks                    []struct {
			Code    string `json:"code"`
			Status  string `json:"status"`
			Label   string `json:"label"`
			Message string `json:"message"`
		} `json:"checks"`
		Capabilities struct {
			Skills struct {
				PersonalCount   int      `json:"personal_count"`
				InheritedCount  int      `json:"inherited_count"`
				MissingRequired []string `json:"missing_required"`
			} `json:"skills"`
			MCPServers struct {
				PersonalCount  int `json:"personal_count"`
				InheritedCount int `json:"inherited_count"`
			} `json:"mcp_servers"`
			EnvironmentVariables struct {
				ConfiguredCount int      `json:"configured_count"`
				MissingNames    []string `json:"missing_names"`
			} `json:"environment_variables"`
		} `json:"capabilities"`
	}
	if err := json.NewDecoder(readinessResp.Body).Decode(&readinessBody); err != nil {
		t.Fatalf("decode scheduling readiness response: %v", err)
	}
	if readinessBody.EmployeeID != employeeID ||
		readinessBody.Status != string(employee.DigitalEmployeeStatusReady) ||
		!readinessBody.ReadyForProjectScheduling ||
		readinessBody.ProjectExecutionSource != "project_runtime_readiness" {
		t.Fatalf("unexpected scheduling readiness response identity/status: %#v", readinessBody)
	}
	if len(readinessBody.Checks) != 1 || readinessBody.Checks[0].Code != "employee_status" || readinessBody.Checks[0].Status != string(employee.ReadinessCheckPassed) {
		t.Fatalf("unexpected scheduling readiness checks: %#v", readinessBody.Checks)
	}
	if readinessBody.Capabilities.Skills.PersonalCount != 2 ||
		readinessBody.Capabilities.Skills.InheritedCount != 3 ||
		readinessBody.Capabilities.Skills.MissingRequired == nil ||
		len(readinessBody.Capabilities.Skills.MissingRequired) != 0 ||
		readinessBody.Capabilities.MCPServers.PersonalCount != 1 ||
		readinessBody.Capabilities.MCPServers.InheritedCount != 2 ||
		readinessBody.Capabilities.EnvironmentVariables.ConfiguredCount != 4 ||
		len(readinessBody.Capabilities.EnvironmentVariables.MissingNames) != 1 ||
		readinessBody.Capabilities.EnvironmentVariables.MissingNames[0] != "JIRA_TOKEN" {
		t.Fatalf("unexpected scheduling readiness capabilities: %#v", readinessBody.Capabilities)
	}
}

func TestCreateDigitalEmployeeRouteAcceptsProviderWithoutRuntime(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user := routeConsoleUser(t, authService, platform.DefaultTenantID)
	service := &routeEmployeeService{}
	server := NewServerWithAuthz(nil, nil, authService, nil, &routeAuthorizer{allowed: true})
	server.SetEmployeeHandler(employee.NewHandler(service))
	teamID := uuid.New()
	body := `{
		"team_id":"` + teamID.String() + `",
		"employee_type":"database_admin",
		"name":"Database administrator",
		"avatar_asset_id":"engineer-m-01",
		"role":"database_admin",
		"provider_type":"codex"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	withConsoleSessionCookie(req, user.SessionToken)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected create digital employee without runtime to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.createReq.ProviderType != "codex" {
		t.Fatalf("expected provider_type codex, got %q", service.createReq.ProviderType)
	}
	if strings.Contains(resp.Body.String(), "runtime_node_id") {
		t.Fatalf("expected create response not to expose employee runtime binding, got %s", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"provider_type":"codex"`) {
		t.Fatalf("expected provider_type in create response, got %s", resp.Body.String())
	}
}

func TestCreateDigitalEmployeeRouteRejectsLegacyFields(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user := routeConsoleUser(t, authService, platform.DefaultTenantID)
	service := &routeEmployeeService{}
	server := NewServerWithAuthz(nil, nil, authService, nil, &routeAuthorizer{allowed: true})
	server.SetEmployeeHandler(employee.NewHandler(service))

	body := `{
		"employee_type":"database_admin",
		"name":"Database administrator",
		"avatar_asset_id":"engineer-m-01",
		"role":"database_admin",
		"provider_type":"codex",
		"role_profile":{"title":"legacy"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	withConsoleSessionCookie(req, user.SessionToken)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected legacy create fields to be rejected, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "role_profile is no longer supported") {
		t.Fatalf("expected legacy field rejection body, got %s", resp.Body.String())
	}
	if service.createCalled {
		t.Fatalf("expected create service not to be called on legacy field rejection")
	}
}

func TestEmployeeRoutesDigitalEmployeeOverviewUsesConsoleTenantAndFilters(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("create auth service: %v", err)
	}
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	user := routeConsoleUser(t, authService, tenantID)
	authorizer := newRecordingAuthorizer()
	service := &routeEmployeeService{}
	server := NewServerWithAuthz(nil, nil, authService, nil, authorizer)
	server.SetEmployeeHandler(employee.NewHandler(service))

	teamID := uuid.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/digital-employees/overview?q=%E9%9C%80%E6%B1%82&team_id="+teamID.String()+"&status=active&employee_type=requirements_analyst&provider_type=codex&risk_level=medium&run_status=none&limit=25&offset=5",
		nil,
	)
	withConsoleSessionCookie(req, user.SessionToken)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected overview route to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.overviewReq.TenantID != tenantID || service.overviewReq.Query != "需求" {
		t.Fatalf("unexpected overview tenant/query: %#v", service.overviewReq)
	}
	if service.overviewReq.TeamID == nil || *service.overviewReq.TeamID != teamID {
		t.Fatalf("expected team filter %s, got %#v", teamID, service.overviewReq.TeamID)
	}
	if service.overviewReq.Status != employee.DigitalEmployeeStatusActive ||
		service.overviewReq.EmployeeType != "requirements_analyst" ||
		service.overviewReq.ProviderType != "codex" ||
		service.overviewReq.RiskLevel != "medium" ||
		service.overviewReq.RunStatus != employee.OverviewRunStatusNone ||
		service.overviewReq.Limit != 25 ||
		service.overviewReq.Offset != 5 {
		t.Fatalf("unexpected overview filters: %#v", service.overviewReq)
	}

	var body struct {
		Summary struct {
			TotalCount                 int32            `json:"total_count"`
			RunnableCount              int32            `json:"runnable_count"`
			RunningCount               int32            `json:"running_count"`
			WaitingRuntimeCount        int32            `json:"waiting_runtime_count"`
			ErrorCount                 int32            `json:"error_count"`
			HighRiskCount              int32            `json:"high_risk_count"`
			ReadyCount                 int32            `json:"ready_count"`
			NeedsConfigurationCount    int32            `json:"needs_configuration_count"`
			PendingConfigApprovalCount int32            `json:"pending_config_approval_count"`
			FailedRecentRunCount       int32            `json:"failed_recent_run_count"`
			OperationalStatusCounts    map[string]int32 `json:"operational_status_counts"`
		} `json:"summary"`
		QueueSummary struct {
			NeedsConfigurationCount int32 `json:"needs_configuration_count"`
			StaleConfigCount           int32 `json:"stale_config_count"`
			FailedRecentRunCount       int32 `json:"failed_recent_run_count"`
		} `json:"queue_summary"`
		Items []struct {
			IdentitySummary struct {
				ID                string `json:"id"`
				Name              string `json:"name"`
				TeamName          string `json:"team_name"`
				EmployeeTypeLabel string `json:"employee_type_label"`
				Status            string `json:"status"`
			} `json:"identity_summary"`
			ExecutionSummary struct {
				Status       string `json:"status"`
				NodeID       string `json:"node_id"`
				ProviderType string `json:"provider_type"`
			} `json:"execution_summary"`
			LatestRunSummary *struct {
				Status       string  `json:"status"`
				FinishedAt   *string `json:"finished_at"`
				ErrorMessage string  `json:"error_message"`
				TokenUsage   int32   `json:"token_usage"`
			} `json:"latest_run_summary"`
			GovernanceSummary struct {
				Status          string `json:"status"`
				SkillsCount     int32  `json:"skills_count"`
				MCPServersCount int32  `json:"mcp_servers_count"`
			} `json:"governance_summary"`
			BudgetSummary struct {
				DailyTokenLimit   *int32   `json:"daily_token_limit"`
				UsageTokensToday  int32    `json:"usage_tokens_today"`
				UsagePercentToday *int32   `json:"usage_percent_today"`
				LimitExceeded     bool     `json:"limit_exceeded"`
				RunCount30d       int32    `json:"run_count_30d"`
				CostAmount30d     *float64 `json:"cost_amount_30d"`
				Source            string   `json:"source"`
			} `json:"budget_summary"`
			WorkbenchStatus  string `json:"workbench_status"`
			OperationalState struct {
				Status  string `json:"status"`
				Reasons []struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"reasons"`
				CanDispatch bool `json:"can_dispatch"`
			} `json:"operational_state"`
			RecentEvents []struct {
				Label      string  `json:"label"`
				Status     string  `json:"status"`
				OccurredAt *string `json:"occurred_at"`
			} `json:"recent_events"`
		} `json:"items"`
		Filters struct {
			Teams []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"teams"`
			RunStatuses []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"run_statuses"`
			Providers []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"providers"`
			ProviderTypes []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"provider_types"`
		} `json:"filters"`
		Pagination struct {
			Limit      int32 `json:"limit"`
			Offset     int32 `json:"offset"`
			TotalCount int32 `json:"total_count"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode overview response: %v", err)
	}
	if body.Summary.TotalCount != 1 || body.Summary.RunnableCount != 1 || body.Summary.RunningCount != 1 {
		t.Fatalf("unexpected overview summary: %#v", body.Summary)
	}
	if body.Summary.ReadyCount != 1 ||
		body.Summary.NeedsConfigurationCount != 0 ||
		body.Summary.PendingConfigApprovalCount != 0 ||
		body.Summary.FailedRecentRunCount != 0 {
		t.Fatalf("unexpected workbench summary: %#v", body.Summary)
	}
	if body.Summary.OperationalStatusCounts == nil || body.Summary.OperationalStatusCounts["idle"] < 1 {
		t.Fatalf("expected operational status counts with idle count, got %#v", body.Summary.OperationalStatusCounts)
	}
	if body.QueueSummary.NeedsConfigurationCount != 0 || body.QueueSummary.StaleConfigCount != 0 || body.QueueSummary.FailedRecentRunCount != 0 {
		t.Fatalf("unexpected queue summary: %#v", body.QueueSummary)
	}
	if len(body.Items) != 1 || body.Items[0].IdentitySummary.Name != "需求分析员工" || body.Items[0].ExecutionSummary.ProviderType != "codex" {
		t.Fatalf("unexpected overview items: %#v", body.Items)
	}
	if body.Items[0].WorkbenchStatus != "ready" {
		t.Fatalf("expected ready workbench status, got %#v", body.Items[0].WorkbenchStatus)
	}
	if body.Items[0].OperationalState.Status != "idle" || !body.Items[0].OperationalState.CanDispatch {
		t.Fatalf("expected idle dispatchable operational state, got %#v", body.Items[0].OperationalState)
	}
	if body.Items[0].OperationalState.Reasons == nil || len(body.Items[0].OperationalState.Reasons) != 0 {
		t.Fatalf("expected operational state reasons to be an empty JSON array, got %#v", body.Items[0].OperationalState.Reasons)
	}
	if len(body.Items[0].RecentEvents) != 3 || body.Items[0].RecentEvents[0].Label != "命令已下发" {
		t.Fatalf("expected recent events, got %#v", body.Items[0].RecentEvents)
	}
	if body.Items[0].BudgetSummary.DailyTokenLimit == nil || *body.Items[0].BudgetSummary.DailyTokenLimit != 10000 {
		t.Fatalf("expected daily token limit, got %#v", body.Items[0].BudgetSummary)
	}
	if body.Items[0].LatestRunSummary == nil || body.Items[0].LatestRunSummary.TokenUsage != 1600 {
		t.Fatalf("expected latest run token usage, got %#v", body.Items[0].LatestRunSummary)
	}
	if body.Items[0].LatestRunSummary.FinishedAt == nil || body.Items[0].LatestRunSummary.ErrorMessage != "" {
		t.Fatalf("expected latest run finished/error fields, got %#v", body.Items[0].LatestRunSummary)
	}
	if body.Items[0].BudgetSummary.CostAmount30d == nil || *body.Items[0].BudgetSummary.CostAmount30d != 12.34 {
		t.Fatalf("expected budget cost amount, got %#v", body.Items[0].BudgetSummary)
	}
	if len(body.Filters.Teams) != 1 || body.Filters.Teams[0].Label != "产品组" {
		t.Fatalf("expected team filters, got %#v", body.Filters.Teams)
	}
	if len(body.Filters.Providers) != 1 || body.Filters.Providers[0].Value != "codex" || len(body.Filters.ProviderTypes) != 0 {
		t.Fatalf("expected providers filter key only, got providers=%#v provider_types=%#v", body.Filters.Providers, body.Filters.ProviderTypes)
	}
	if body.Pagination.Limit != 25 || body.Pagination.Offset != 5 || body.Pagination.TotalCount != 1 {
		t.Fatalf("unexpected pagination: %#v", body.Pagination)
	}
	lastCheck := authorizer.checks[len(authorizer.checks)-1]
	if lastCheck.Action != authz.ActionEmployeeRead || lastCheck.Resource.Type != authz.ResourceTenant || lastCheck.TenantID != tenantID {
		t.Fatalf("unexpected overview authz check: %#v", lastCheck)
	}
}

func TestDigitalEmployeeCreateOptionsUnrestrictedListsAreArrays(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	tenantID := platform.DefaultTenantID
	teamID := uuid.New()
	service := &routeEmployeeService{
		createOptions: &employee.CreateOptions{
			TeamConfig: employee.TeamConfigCreateOption{
				ID:           uuid.New(),
				TenantID:     tenantID,
				TeamID:       &teamID,
				Constitution: map[string]any{},
				Skills:       []string{},
				MCPServers:   []string{},
			},
			EmployeeTypes: []employee.EmployeeTypeDefinition{{
				Type:        "database_admin",
				Label:       "数据库管理",
				Description: "Manages database operations",
				DefaultRole: "database_admin",
			}},
			RuntimeProviderOptions: []employee.RuntimeProviderOption{{
				RuntimeNodeID:         uuid.New(),
				NodeID:                "offline-runtime",
				RuntimeName:           "离线执行机",
				ProviderType:          "codex",
				RuntimeStatus:         "offline",
				ProviderStatus:        "unhealthy",
				HealthStatus:          "unhealthy",
				CurrentLoad:           0,
				MaxSlots:              2,
				AgentHomeDir:          "/srv/agents/codex",
				AgentHomeDirAvailable: false,
				Available:             false,
				DisabledReason:        "runtime_session_inactive",
			}},
		},
	}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetEmployeeHandler(employee.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/create-options?team_id="+teamID.String(), nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected create options to succeed, got %d: %s", resp.Code, resp.Body.String())
	}

	var body struct {
		TeamConfig struct {
			TeamID       string         `json:"team_id"`
			Constitution map[string]any `json:"constitution"`
			Skills       []string       `json:"skills"`
			MCPServers   []string       `json:"mcp_servers"`
		} `json:"team_config"`
		EmployeeTypes []struct {
			RecommendedSkills        []string       `json:"recommended_skills"`
			RecommendedMCPServers    []string       `json:"recommended_mcp_servers"`
			RecommendedProviderTypes []string       `json:"recommended_provider_types"`
			PersonaMemoryMarkdown    string         `json:"persona_memory_markdown"`
			CapabilityBindings       map[string]any `json:"capability_bindings"`
			BudgetPolicy             map[string]any `json:"budget_policy"`
		} `json:"employee_types"`
		CapabilityOptions struct {
			ProviderTypes []string `json:"provider_types"`
			Skills        []string `json:"skills"`
			MCPServers    []string `json:"mcp_servers"`
		} `json:"capability_options"`
		RuntimeProviderOptions []struct {
			ProviderType string `json:"provider_type"`
			Available    bool   `json:"available"`
		} `json:"runtime_provider_options"`
		CreationChecks []struct {
			Key     string `json:"key"`
			Label   string `json:"label"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"creation_checks"`
		PolicyDefaults struct {
			PermissionPolicy map[string]any `json:"permission_policy"`
			ApprovalPolicy   map[string]any `json:"approval_policy"`
			WorkspacePolicy  map[string]any `json:"workspace_policy"`
			SessionPolicy    map[string]any `json:"session_policy"`
			Metadata         map[string]any `json:"metadata"`
		} `json:"policy_defaults"`
	}
	bodyJSON := resp.Body.String()
	if err := json.NewDecoder(strings.NewReader(bodyJSON)).Decode(&body); err != nil {
		t.Fatalf("decode create options: %v", err)
	}
	if body.TeamConfig.TeamID != teamID.String() {
		t.Fatalf("expected team_config.team_id %s, got %q", teamID, body.TeamConfig.TeamID)
	}
	assertNonNilEmptyStringSlice(t, "team_config.skills", body.TeamConfig.Skills)
	assertNonNilEmptyStringSlice(t, "team_config.mcp_servers", body.TeamConfig.MCPServers)
	if len(body.EmployeeTypes) != 1 {
		t.Fatalf("expected one employee type option, got %#v", body.EmployeeTypes)
	}
	assertNonNilEmptyStringSlice(t, "employee_types[0].recommended_skills", body.EmployeeTypes[0].RecommendedSkills)
	assertNonNilEmptyStringSlice(t, "employee_types[0].recommended_mcp_servers", body.EmployeeTypes[0].RecommendedMCPServers)
	assertNonNilEmptyStringSlice(t, "employee_types[0].recommended_provider_types", body.EmployeeTypes[0].RecommendedProviderTypes)
	if body.EmployeeTypes[0].PersonaMemoryMarkdown != "" {
		t.Fatalf("expected create-options employee type persona_memory_markdown default to be empty, got %#v", body.EmployeeTypes[0].PersonaMemoryMarkdown)
	}
	if body.EmployeeTypes[0].CapabilityBindings == nil {
		t.Fatalf("expected create-options employee type capability_bindings to decode as {}, got %#v", body.EmployeeTypes[0].CapabilityBindings)
	}
	if body.EmployeeTypes[0].BudgetPolicy == nil {
		t.Fatalf("expected create-options employee type budget_policy to decode as {}, got %#v", body.EmployeeTypes[0].BudgetPolicy)
	}
	assertNonNilEmptyStringSlice(t, "capability_options.provider_types", body.CapabilityOptions.ProviderTypes)
	assertNonNilEmptyStringSlice(t, "capability_options.skills", body.CapabilityOptions.Skills)
	assertNonNilEmptyStringSlice(t, "capability_options.mcp_servers", body.CapabilityOptions.MCPServers)
	if len(body.RuntimeProviderOptions) != 1 || body.RuntimeProviderOptions[0].ProviderType != "codex" || body.RuntimeProviderOptions[0].Available {
		t.Fatalf("expected runtime_provider_options dispatch preview to remain present and unavailable, got %#v", body.RuntimeProviderOptions)
	}
	assertCreateOptionCheck(t, body.CreationChecks, "employee_templates", "passed")
	if len(body.CreationChecks) <= 3 || body.CreationChecks[3].Key != "runtime_provider" {
		t.Fatalf("expected runtime_provider check at index 3, got %#v", body.CreationChecks)
	}
	if body.CreationChecks[3].Status == "blocked" {
		t.Fatalf("runtime_provider must be advisory, got %#v", body.CreationChecks[3])
	}
	if body.PolicyDefaults.PermissionPolicy == nil || body.PolicyDefaults.ApprovalPolicy == nil || body.PolicyDefaults.WorkspacePolicy == nil || body.PolicyDefaults.SessionPolicy == nil || body.PolicyDefaults.Metadata == nil {
		t.Fatalf("expected final policy_defaults fields to decode as objects, got %#v", body.PolicyDefaults)
	}
	if strings.Contains(bodyJSON, "\"default_capability_selection\"") ||
		strings.Contains(bodyJSON, "\"default_context_policy_override\"") ||
		strings.Contains(bodyJSON, "\"context_policy_override\"") ||
		strings.Contains(bodyJSON, "\"capability_selection\"") {
		t.Fatalf("expected legacy create-options keys to be absent, got %s", bodyJSON)
	}

	service.createOptions.EmployeeTypes = nil
	emptyTypesReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/create-options?team_id="+teamID.String(), nil)
	emptyTypesReq.AddCookie(cookie)
	emptyTypesResp := httptest.NewRecorder()
	server.ServeHTTP(emptyTypesResp, emptyTypesReq)
	if emptyTypesResp.Code != http.StatusOK {
		t.Fatalf("expected create options with no employee types to succeed, got %d: %s", emptyTypesResp.Code, emptyTypesResp.Body.String())
	}
	var emptyTypesBody struct {
		EmployeeTypes []struct{} `json:"employee_types"`
	}
	if err := json.NewDecoder(emptyTypesResp.Body).Decode(&emptyTypesBody); err != nil {
		t.Fatalf("decode create options with no employee types: %v", err)
	}
	if emptyTypesBody.EmployeeTypes == nil || len(emptyTypesBody.EmployeeTypes) != 0 {
		t.Fatalf("expected employee_types to decode as empty array, got %#v", emptyTypesBody.EmployeeTypes)
	}
}

func assertCreateOptionCheck(t *testing.T, checks []struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message"`
}, key string, status string) {
	t.Helper()
	for _, check := range checks {
		if check.Key == key {
			if check.Status != status {
				t.Fatalf("expected check %s status %s, got %s", key, status, check.Status)
			}
			if check.Label == "" || check.Message == "" {
				t.Fatalf("expected check %s to include label and message, got %#v", key, check)
			}
			return
		}
	}
	t.Fatalf("expected creation check %s in %#v", key, checks)
}

func assertNonNilEmptyStringSlice(t *testing.T, field string, values []string) {
	t.Helper()
	if values == nil || len(values) != 0 {
		t.Fatalf("expected %s to decode as empty array, got %#v", field, values)
	}
}

type routeConsoleSessionUser struct {
	*auth.User
	SessionToken string
}

func routeConsoleUser(t *testing.T, authService *auth.Service, tenantID uuid.UUID) routeConsoleSessionUser {
	t.Helper()
	if tenantID != platform.DefaultTenantID {
		t.Fatalf("route auth service only supports default tenant %s, got %s", platform.DefaultTenantID.String(), tenantID)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create console user: %v", err)
	}
	_, token, err := authService.CreateSession(context.Background(), user.ID, "127.0.0.1", "route-test")
	if err != nil {
		t.Fatalf("create console session: %v", err)
	}
	return routeConsoleSessionUser{User: user, SessionToken: token}
}

func withConsoleSessionCookie(req *http.Request, token string) {
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
}

func newRecordingAuthorizer() *routeAuthorizer {
	return &routeAuthorizer{allowed: true}
}

func newEmployeeRouteTestServer(t *testing.T, authorizer *routeAuthorizer, configure ...func(*routeEmployeeService)) (*Server, *routeEmployeeService, *http.Cookie) {
	t.Helper()
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeEmployeeService{}
	for _, fn := range configure {
		fn(service)
	}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	server.SetEmployeeHandler(employee.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	return server, service, cookie
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

func TestDigitalEmployeeSchedulingReadinessRouteRejectsNilServiceResponse(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeEmployeeService{returnNilSchedulingReadiness: true}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetEmployeeHandler(employee.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	employeeID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/scheduling-readiness", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected nil scheduling readiness to return 500, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "digital employee scheduling readiness unavailable") {
		t.Fatalf("expected scheduling readiness unavailable error, got %s", resp.Body.String())
	}
}

func TestEmployeeListAcceptsTeamFilter(t *testing.T) {
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
	teamID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees?team_id="+teamID.String(), nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected list digital employees to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.listReq.TeamID == nil || *service.listReq.TeamID != teamID {
		t.Fatalf("expected list team %s, got %#v", teamID, service.listReq.TeamID)
	}
}

func TestDigitalEmployeeRunRoutesCreateAndStop(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	employeeService := &routeEmployeeService{}
	runService := &routeEmployeeRunService{}
	authorizer := &routeAuthorizer{allowed: true}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	handler := employee.NewHandler(employeeService)
	handler.SetRunService(runService)
	server.SetEmployeeHandler(handler)
	cookie := routeLogin(t, server, "admin", "admin")
	tenantID := platform.DefaultTenantID
	employeeID := uuid.New()

	createBody := `{
		"objective":"审查需求",
		"prompt":"请输出风险点",
		"context_refs":[{"type":"doc","ref":"ctx://req"}],
		"artifact_refs":[{"type":"file","ref":"s3://bucket/input.md"}],
		"output_schema":{"type":"object"},
		"allowed_actions":["read_context"],
		"forbidden_actions":["deploy"],
		"secret_refs":["jira-token"],
		"idempotency_key":"idem-1",
		"timeout_sec":600,
		"grace_sec":30,
		"metadata":{"source":"route-test"}
	}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+employeeID.String()+"/runs", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create run to succeed, got %d: %s", createResp.Code, createResp.Body.String())
	}
	if runService.createReq.TenantID != tenantID || runService.createReq.UserID != user.ID || runService.createReq.DigitalEmployeeID != employeeID {
		t.Fatalf("unexpected create run identity mapping: %#v", runService.createReq)
	}
	if runService.createReq.Objective != "审查需求" || runService.createReq.Prompt != "请输出风险点" || runService.createReq.IdempotencyKey == nil || *runService.createReq.IdempotencyKey != "idem-1" {
		t.Fatalf("unexpected create run body mapping: %#v", runService.createReq)
	}
	if len(runService.createReq.ContextRefs) != 1 || runService.createReq.ContextRefs[0]["ref"] != "ctx://req" {
		t.Fatalf("expected context refs to map, got %#v", runService.createReq.ContextRefs)
	}
	if len(runService.createReq.AllowedActions) != 1 || runService.createReq.AllowedActions[0] != "read_context" {
		t.Fatalf("expected allowed actions to map, got %#v", runService.createReq.AllowedActions)
	}
	var createdRaw map[string]json.RawMessage
	if err := json.Unmarshal(createResp.Body.Bytes(), &createdRaw); err != nil {
		t.Fatalf("decode raw created run: %v", err)
	}
	if _, ok := createdRaw["idempotency_fingerprint"]; ok {
		t.Fatalf("run response must not expose idempotency_fingerprint: %s", string(createdRaw["idempotency_fingerprint"]))
	}
	if string(createdRaw["idempotency_key"]) != `"idem-route-test"` {
		t.Fatalf("expected run response to expose idempotency_key, got %s", string(createdRaw["idempotency_key"]))
	}
	var created struct {
		ID                string                 `json:"id"`
		TenantID          string                 `json:"tenant_id"`
		DigitalEmployeeID string                 `json:"digital_employee_id"`
		CommandID         string                 `json:"command_id"`
		Status            string                 `json:"status"`
		Result            map[string]any         `json:"result"`
		LogRef            *string                `json:"log_ref"`
		WorkProducts      []employee.WorkProduct `json:"work_products"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created run: %v", err)
	}
	if created.ID == "" || created.TenantID != tenantID.String() || created.DigitalEmployeeID != employeeID.String() || created.CommandID != "cmd-route-test" || created.Status != string(employee.DigitalEmployeeRunStatusDispatching) {
		t.Fatalf("unexpected created run response: %#v", created)
	}
	if created.Result["summary"] != "queued" || created.LogRef == nil || *created.LogRef != "s3://logs/run.log" || len(created.WorkProducts) != 1 {
		t.Fatalf("expected run response fields, got %#v", created)
	}

	defaultListReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs", nil)
	defaultListReq.AddCookie(cookie)
	defaultListResp := httptest.NewRecorder()
	server.ServeHTTP(defaultListResp, defaultListReq)
	if defaultListResp.Code != http.StatusOK {
		t.Fatalf("expected default list runs to succeed, got %d: %s", defaultListResp.Code, defaultListResp.Body.String())
	}
	if runService.listLimit != 50 || runService.listOffset != 0 {
		t.Fatalf("expected default list pagination limit=50 offset=0, got limit=%d offset=%d", runService.listLimit, runService.listOffset)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs?limit=25&offset=5", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list runs to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if runService.listTenantID != tenantID || runService.listEmployeeID != employeeID || runService.listLimit != 25 || runService.listOffset != 5 {
		t.Fatalf("unexpected list mapping: tenant=%s employee=%s limit=%d offset=%d", runService.listTenantID, runService.listEmployeeID, runService.listLimit, runService.listOffset)
	}
	var listRaw struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listRaw); err != nil {
		t.Fatalf("decode raw list runs: %v", err)
	}
	if len(listRaw.Items) != 1 {
		t.Fatalf("unexpected raw list runs response: %#v", listRaw.Items)
	}
	if _, ok := listRaw.Items[0]["idempotency_fingerprint"]; ok {
		t.Fatalf("list run response must not expose idempotency_fingerprint: %s", string(listRaw.Items[0]["idempotency_fingerprint"]))
	}
	if string(listRaw.Items[0]["idempotency_key"]) != `"idem-route-test"` {
		t.Fatalf("expected list run response to expose idempotency_key, got %s", string(listRaw.Items[0]["idempotency_key"]))
	}
	var listBody struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list runs: %v", err)
	}
	if len(listBody.Items) != 1 || listBody.Items[0].ID != created.ID {
		t.Fatalf("unexpected list runs response: %#v", listBody.Items)
	}

	clampListReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs?limit=500&offset=6", nil)
	clampListReq.AddCookie(cookie)
	clampListResp := httptest.NewRecorder()
	server.ServeHTTP(clampListResp, clampListReq)
	if clampListResp.Code != http.StatusOK {
		t.Fatalf("expected clamped list runs to succeed, got %d: %s", clampListResp.Code, clampListResp.Body.String())
	}
	if runService.listLimit != 100 || runService.listOffset != 6 {
		t.Fatalf("expected clamped list pagination limit=100 offset=6, got limit=%d offset=%d", runService.listLimit, runService.listOffset)
	}

	for _, query := range []string{"limit=bad", "offset=bad", "limit=0", "limit=-1", "offset=-1", "offset=2147483648"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs?"+query, nil)
		req.AddCookie(cookie)
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected list runs query %q to return 400, got %d: %s", query, resp.Code, resp.Body.String())
		}
	}

	runKindListReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs?run_kind=chat", nil)
	runKindListReq.AddCookie(cookie)
	runKindListResp := httptest.NewRecorder()
	server.ServeHTTP(runKindListResp, runKindListReq)
	if runKindListResp.Code != http.StatusOK {
		t.Fatalf("expected run_kind=chat list runs to succeed, got %d: %s", runKindListResp.Code, runKindListResp.Body.String())
	}
	if runService.listRunKind == nil || *runService.listRunKind != "chat" {
		t.Fatalf("expected list filter to forward run_kind=chat to the service, got %#v", runService.listRunKind)
	}

	runService.listCalled = false
	invalidRunKindReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs?run_kind=banana", nil)
	invalidRunKindReq.AddCookie(cookie)
	invalidRunKindResp := httptest.NewRecorder()
	server.ServeHTTP(invalidRunKindResp, invalidRunKindReq)
	if invalidRunKindResp.Code != http.StatusBadRequest {
		t.Fatalf("expected run_kind=banana list runs to return 400, got %d: %s", invalidRunKindResp.Code, invalidRunKindResp.Body.String())
	}
	if runService.listCalled {
		t.Fatalf("expected an invalid run_kind to be rejected before reaching the run service")
	}

	runID := uuid.MustParse(created.ID)
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs/"+runID.String(), nil)
	getReq.AddCookie(cookie)
	getResp := httptest.NewRecorder()
	server.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected get run to succeed, got %d: %s", getResp.Code, getResp.Body.String())
	}
	if runService.getTenantID != tenantID || runService.getEmployeeID != employeeID || runService.getRunID != runID {
		t.Fatalf("unexpected get mapping: tenant=%s employee=%s run=%s", runService.getTenantID, runService.getEmployeeID, runService.getRunID)
	}

	defaultEventsReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs/"+runID.String()+"/events", nil)
	defaultEventsReq.AddCookie(cookie)
	defaultEventsResp := httptest.NewRecorder()
	server.ServeHTTP(defaultEventsResp, defaultEventsReq)
	if defaultEventsResp.Code != http.StatusOK {
		t.Fatalf("expected default list run events to succeed, got %d: %s", defaultEventsResp.Code, defaultEventsResp.Body.String())
	}
	if runService.eventsLimit != 50 || runService.eventsOffset != 0 {
		t.Fatalf("expected default events pagination limit=50 offset=0, got limit=%d offset=%d", runService.eventsLimit, runService.eventsOffset)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs/"+runID.String()+"/events?limit=10&offset=2", nil)
	eventsReq.AddCookie(cookie)
	eventsResp := httptest.NewRecorder()
	server.ServeHTTP(eventsResp, eventsReq)
	if eventsResp.Code != http.StatusOK {
		t.Fatalf("expected list run events to succeed, got %d: %s", eventsResp.Code, eventsResp.Body.String())
	}
	if runService.eventsTenantID != tenantID || runService.eventsEmployeeID != employeeID || runService.eventsRunID != runID || runService.eventsLimit != 10 || runService.eventsOffset != 2 {
		t.Fatalf("unexpected events mapping: tenant=%s employee=%s run=%s limit=%d offset=%d", runService.eventsTenantID, runService.eventsEmployeeID, runService.eventsRunID, runService.eventsLimit, runService.eventsOffset)
	}
	var eventsBody []employee.RuntimeCommandEventWriteback
	if err := json.NewDecoder(eventsResp.Body).Decode(&eventsBody); err != nil {
		t.Fatalf("decode events response: %v", err)
	}
	if len(eventsBody) != 1 || eventsBody[0].EventType != "provider_output" || eventsBody[0].SequenceNumber != 7 {
		t.Fatalf("unexpected events response: %#v", eventsBody)
	}

	clampEventsReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs/"+runID.String()+"/events?limit=500&offset=8", nil)
	clampEventsReq.AddCookie(cookie)
	clampEventsResp := httptest.NewRecorder()
	server.ServeHTTP(clampEventsResp, clampEventsReq)
	if clampEventsResp.Code != http.StatusOK {
		t.Fatalf("expected clamped list run events to succeed, got %d: %s", clampEventsResp.Code, clampEventsResp.Body.String())
	}
	if runService.eventsLimit != 100 || runService.eventsOffset != 8 {
		t.Fatalf("expected clamped events pagination limit=100 offset=8, got limit=%d offset=%d", runService.eventsLimit, runService.eventsOffset)
	}

	for _, query := range []string{"limit=bad", "offset=bad", "limit=0", "limit=-1", "offset=-1", "offset=2147483648"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/runs/"+runID.String()+"/events?"+query, nil)
		req.AddCookie(cookie)
		resp := httptest.NewRecorder()
		server.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected list run events query %q to return 400, got %d: %s", query, resp.Code, resp.Body.String())
		}
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+employeeID.String()+"/runs/"+runID.String()+"/stop", strings.NewReader(`{"reason":"用户取消"}`))
	stopReq.Header.Set("Content-Type", "application/json")
	stopReq.AddCookie(cookie)
	stopResp := httptest.NewRecorder()
	server.ServeHTTP(stopResp, stopReq)
	if stopResp.Code != http.StatusOK {
		t.Fatalf("expected stop run to succeed, got %d: %s", stopResp.Code, stopResp.Body.String())
	}
	if runService.stopReq.TenantID != tenantID || runService.stopReq.UserID != user.ID || runService.stopReq.DigitalEmployeeID != employeeID || runService.stopReq.RunID != runID || runService.stopReq.Reason != "用户取消" {
		t.Fatalf("unexpected stop mapping: %#v", runService.stopReq)
	}
	var stopped struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(stopResp.Body).Decode(&stopped); err != nil {
		t.Fatalf("decode stopped run: %v", err)
	}
	if stopped.ID != runID.String() || stopped.Status != string(employee.DigitalEmployeeRunStatusCancelling) {
		t.Fatalf("unexpected stopped run response: %#v", stopped)
	}

	expectedChecks := []string{
		authz.ActionEmployeeRunCreate,
	}
	for i := 0; i < 21; i++ {
		expectedChecks = append(expectedChecks, authz.ActionEmployeeRead)
	}
	expectedChecks = append(expectedChecks, authz.ActionEmployeeRunStop)
	if len(authorizer.checks) < len(expectedChecks) {
		t.Fatalf("expected at least %d authorization checks, got %#v", len(expectedChecks), authorizer.checks)
	}
	runChecks := authorizer.checks[len(authorizer.checks)-len(expectedChecks):]
	for i, action := range expectedChecks {
		check := runChecks[i]
		if check.Action != action || check.Resource.Type != authz.ResourceEmployee || check.Resource.ID != employeeID.String() || check.TenantID != tenantID {
			t.Fatalf("unexpected run authz check at %d: %#v", i, check)
		}
	}
}

func TestEmployeeRoutesUseAuthzActions(t *testing.T) {
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

	tests := []struct {
		name         string
		method       string
		path         string
		body         string
		action       string
		resourceType string
		resourceID   string
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/digital-employees", action: authz.ActionEmployeeRead, resourceType: authz.ResourceTenant},
		{name: "avatar assets", method: http.MethodGet, path: "/api/v1/digital-employee-avatar-assets", action: authz.ActionEmployeeRead, resourceType: authz.ResourceTenant},
		{name: "create", method: http.MethodPost, path: "/api/v1/digital-employees", body: `{"team_id":"` + uuid.New().String() + `","name":"Requirements analyst","role":"requirements_analyst"}`, action: authz.ActionEmployeeCreate, resourceType: authz.ResourceTenant},
		{name: "create options", method: http.MethodGet, path: "/api/v1/digital-employees/create-options?team_id=" + uuid.New().String(), action: authz.ActionEmployeeCreate, resourceType: authz.ResourceTenant},
		{name: "get", method: http.MethodGet, path: "/api/v1/digital-employees/" + employeeID, action: authz.ActionEmployeeRead, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/digital-employees/" + employeeID, action: authz.ActionEmployeeDelete, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "status", method: http.MethodPut, path: "/api/v1/digital-employees/" + employeeID + "/status", body: `{"status":"active"}`, action: authz.ActionEmployeeStatusUpdate, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "profile", method: http.MethodPut, path: "/api/v1/digital-employees/" + employeeID + "/profile", body: `{"description":"负责需求拆解"}`, action: authz.ActionEmployeeProfileUpdate, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "create config revision", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/config-revisions", body: `{"role_profile":{"title":"analyst"}}`, action: authz.ActionEmployeeConfigCreate, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "get scheduling readiness", method: http.MethodGet, path: "/api/v1/digital-employees/" + employeeID + "/scheduling-readiness", action: authz.ActionEmployeeRead, resourceType: authz.ResourceEmployee, resourceID: employeeID},
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
	expectedTenantID := platform.DefaultTenantID
	for idx, check := range authorizer.checks {
		expected := tests[idx]
		if check.Action != expected.action {
			t.Fatalf("expected %s action, got %#v", expected.action, check)
		}
		if check.Actor.Type != authz.ActorUser {
			t.Fatalf("expected user actor, got %#v", check)
		}
		expectedResourceID := expected.resourceID
		if expectedResourceID == "" {
			expectedResourceID = expectedTenantID.String()
		}
		if check.Resource.Type != expected.resourceType || check.Resource.ID != expectedResourceID || check.TenantID != expectedTenantID {
			t.Fatalf("expected tenant resource %s, got %#v", expectedTenantID, check)
		}
	}
}

func TestDeleteDigitalEmployeeRouteReturnsNoContent(t *testing.T) {
	server, service, cookie := newEmployeeRouteTestServer(t, &routeAuthorizer{allowed: true})
	employeeID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/digital-employees/"+employeeID.String(), nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected delete digital employee to return 204, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.deleteReq.DigitalEmployeeID != employeeID {
		t.Fatalf("expected delete employee id %s, got %#v", employeeID, service.deleteReq)
	}
	if service.deleteReq.TenantID != platform.DefaultTenantID {
		t.Fatalf("expected default tenant %s, got %#v", platform.DefaultTenantID, service.deleteReq)
	}
	if service.deleteReq.ActorUserID == uuid.Nil {
		t.Fatalf("expected delete actor user id, got %#v", service.deleteReq)
	}
}

func TestUpdateDigitalEmployeeProfileRouteReturnsUpdatedEmployee(t *testing.T) {
	server, service, cookie := newEmployeeRouteTestServer(t, &routeAuthorizer{allowed: true})
	employeeID := uuid.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/digital-employees/"+employeeID.String()+"/profile", strings.NewReader(`{"description":"负责需求拆解和交付风险识别"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected profile update to return 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !service.updateProfileCalled {
		t.Fatal("expected UpdateProfile to be called")
	}
	if service.updateProfileReq.DigitalEmployeeID != employeeID {
		t.Fatalf("expected employee id %s, got %#v", employeeID, service.updateProfileReq)
	}
	if service.updateProfileReq.Description == nil || *service.updateProfileReq.Description != "负责需求拆解和交付风险识别" {
		t.Fatalf("expected description payload, got %#v", service.updateProfileReq.Description)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["description"] != "负责需求拆解和交付风险识别" {
		t.Fatalf("expected description in response, got %#v", body["description"])
	}
}

func TestUpdateDigitalEmployeeProfileRouteRejectsMissingDescription(t *testing.T) {
	server, service, cookie := newEmployeeRouteTestServer(t, &routeAuthorizer{allowed: true})
	employeeID := uuid.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/digital-employees/"+employeeID.String()+"/profile", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected missing description to return 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.updateProfileCalled {
		t.Fatal("expected UpdateProfile not to be called when description is missing")
	}
}

func TestDeleteDigitalEmployeeRouteReturnsBlockers(t *testing.T) {
	blockerID := uuid.New()
	projectID := uuid.New()
	blockedErr := &employee.DigitalEmployeeDeleteBlockedError{Blockers: []employee.DigitalEmployeeDeleteBlocker{{
		Type:      employee.DigitalEmployeeDeleteBlockerTypeProjectTask,
		ID:        blockerID,
		Status:    "queued",
		Title:     "项目任务 A",
		ProjectID: &projectID,
	}}}
	server, _, cookie := newEmployeeRouteTestServer(t, &routeAuthorizer{allowed: true}, func(s *routeEmployeeService) {
		s.deleteErr = blockedErr
	})
	employeeID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/digital-employees/"+employeeID.String(), nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected delete blockers to return 409, got %d: %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Code     string `json:"code"`
		Message  string `json:"message"`
		Blockers []struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			Status    string `json:"status"`
			Title     string `json:"title"`
			ProjectID string `json:"project_id"`
		} `json:"blockers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode delete blocker response: %v", err)
	}
	if body.Code != employee.DigitalEmployeeDeleteBlockedCode {
		t.Fatalf("expected blocked code, got %#v", body)
	}
	if !strings.Contains(body.Message, "仍有排队或执行中的工作") {
		t.Fatalf("expected Chinese blocker message, got %#v", body.Message)
	}
	if len(body.Blockers) != 1 || body.Blockers[0].Type != "project_task" || body.Blockers[0].Title != "项目任务 A" || body.Blockers[0].ProjectID != projectID.String() {
		t.Fatalf("unexpected blockers: %#v", body.Blockers)
	}
}

func TestGetDigitalEmployeeIncludesAllowedDeleteAction(t *testing.T) {
	server, _, cookie := newEmployeeRouteTestServer(t, &routeAuthorizer{allowed: true})
	employeeID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String(), nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected get digital employee to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	var body struct {
		AllowedActions []string `json:"allowed_actions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode get employee response: %v", err)
	}
	if !containsString(body.AllowedActions, authz.ActionEmployeeDelete) {
		t.Fatalf("expected allowed delete action, got %#v", body.AllowedActions)
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

func TestDigitalEmployeeEnvironmentVariableRoutes(t *testing.T) {
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
	employeeID := uuid.New()

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/environment-variables", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list env vars to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}

	upsertReq := httptest.NewRequest(http.MethodPut, "/api/v1/digital-employees/"+employeeID.String()+"/environment-variables/GH_TOKEN", strings.NewReader(`{"value":"secret","sensitive":true}`))
	upsertReq.AddCookie(cookie)
	upsertResp := httptest.NewRecorder()
	server.ServeHTTP(upsertResp, upsertReq)
	if upsertResp.Code != http.StatusOK {
		t.Fatalf("expected upsert env var to succeed, got %d: %s", upsertResp.Code, upsertResp.Body.String())
	}
	if strings.Contains(upsertResp.Body.String(), "secret") {
		t.Fatalf("response leaked plaintext: %s", upsertResp.Body.String())
	}
	if !service.upsertEnvReq.Sensitive {
		t.Fatalf("expected explicit sensitive flag to stay true, got %#v", service.upsertEnvReq)
	}

	defaultSensitiveReq := httptest.NewRequest(http.MethodPut, "/api/v1/digital-employees/"+employeeID.String()+"/environment-variables/GH_PAT", strings.NewReader(`{"value":"secret2"}`))
	defaultSensitiveReq.AddCookie(cookie)
	defaultSensitiveResp := httptest.NewRecorder()
	server.ServeHTTP(defaultSensitiveResp, defaultSensitiveReq)
	if defaultSensitiveResp.Code != http.StatusOK {
		t.Fatalf("expected upsert env var without sensitive flag to succeed, got %d: %s", defaultSensitiveResp.Code, defaultSensitiveResp.Body.String())
	}
	if strings.Contains(defaultSensitiveResp.Body.String(), "secret2") {
		t.Fatalf("response leaked plaintext: %s", defaultSensitiveResp.Body.String())
	}
	if !service.upsertEnvReq.Sensitive {
		t.Fatalf("expected omitted sensitive flag to default true, got %#v", service.upsertEnvReq)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/digital-employees/"+employeeID.String()+"/environment-variables/GH_TOKEN", nil)
	deleteReq.AddCookie(cookie)
	deleteResp := httptest.NewRecorder()
	server.ServeHTTP(deleteResp, deleteReq)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("expected delete env var to succeed, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
}

type routeEmployeeService struct {
	createOptionsReq                 employee.CreateOptionsRequest
	createOptions                    *employee.CreateOptions
	createOptionsErr                 error
	createReq                        employee.CreateDigitalEmployeeRequest
	listReq                          employee.ListDigitalEmployeesRequest
	overviewReq                      employee.GetDigitalEmployeeOverviewRequest
	listEnvReq                       employee.ListEnvironmentVariablesRequest
	upsertEnvReq                     employee.UpsertEnvironmentVariableRequest
	deleteEnvReq                     employee.DeleteEnvironmentVariableRequest
	deleteReq                        employee.DeleteDigitalEmployeeRequest
	deleteErr                        error
	updateReq                        employee.UpdateStatusRequest
	updateProfileReq                 employee.UpdateProfileRequest
	getTenantID                      uuid.UUID
	createCalled                     bool
	listCalled                       bool
	getCalled                        bool
	updateCalled                     bool
	updateProfileCalled              bool
	configRevisionReq                employee.CreateDigitalEmployeeConfigRevisionRequest
	configCalled                     bool
	getSchedulingReadinessCalled     bool
	getSchedulingReadinessTenant     uuid.UUID
	getSchedulingReadinessEmployeeID uuid.UUID
	returnNilSchedulingReadiness     bool
	createdID                        uuid.UUID
	listErr                          error
	overviewErr                      error
}

func (s *routeEmployeeService) ListAvatarAssets(ctx context.Context, tenantID uuid.UUID) ([]employee.DigitalEmployeeAvatarAsset, error) {
	return employee.ListDigitalEmployeeAvatarAssets(), nil
}

func (s *routeEmployeeService) GetCreateOptions(ctx context.Context, req employee.CreateOptionsRequest) (*employee.CreateOptions, error) {
	s.createOptionsReq = req
	if s.createOptionsErr != nil {
		return nil, s.createOptionsErr
	}
	if s.createOptions != nil {
		return s.createOptions, nil
	}
	return &employee.CreateOptions{
		TeamConfig: employee.TeamConfigCreateOption{
			ID:           uuid.New(),
			TenantID:     req.TenantID,
			TeamID:       req.TeamID,
			Constitution: map[string]any{},
			Skills:       baselineSkillsForRoute(req.TeamID),
			MCPServers:   []string{},
		},
		EmployeeTypes: []employee.EmployeeTypeDefinition{{
			Type:                     "database_admin",
			Label:                    "数据库管理",
			Description:              "Manages database operations",
			DefaultRole:              "database_admin",
			RecommendedProviderTypes: []string{"codex"},
		}},
		CapabilityOptions: employee.CapabilityOptions{
			ProviderTypes: []string{"codex"},
		},
		RuntimeProviderOptions: []employee.RuntimeProviderOption{{
			RuntimeNodeID:         uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			NodeID:                "local-dev-node",
			RuntimeName:           "Local Dev",
			ProviderType:          "codex",
			RuntimeStatus:         "online",
			ProviderStatus:        "healthy",
			HealthStatus:          "healthy",
			CurrentLoad:           1,
			MaxSlots:              4,
			AgentHomeDir:          "/srv/agents/database",
			AgentHomeDirAvailable: true,
			Available:             true,
		}},
		PolicyDefaults: employee.PolicyDefaults{
			PermissionPolicy: map[string]any{},
			ApprovalPolicy:   map[string]any{},
			WorkspacePolicy:  map[string]any{},
			SessionPolicy:    map[string]any{"mode": "reuse_latest"},
			Metadata:         map[string]any{},
		},
	}, nil
}

func baselineSkillsForRoute(teamID *uuid.UUID) []string {
	if teamID == nil {
		return []string{}
	}
	return []string{"database_admin"}
}

func (s *routeEmployeeService) CreateDigitalEmployee(ctx context.Context, req employee.CreateDigitalEmployeeRequest) (*employee.DigitalEmployee, error) {
	s.createCalled = true
	s.createReq = req
	s.createdID = uuid.New()
	now := time.Now().UTC()
	return &employee.DigitalEmployee{
		ID:               s.createdID,
		TenantID:         req.TenantID,
		TeamID:           req.TeamID,
		OwnerUserID:      req.OwnerUserID,
		EmployeeType:     req.EmployeeType,
		ProviderType:     req.ProviderType,
		Name:             req.Name,
		Role:             req.Role,
		Status:           employee.DigitalEmployeeStatusReady,
		PermissionPolicy: req.PermissionPolicy,
		RiskLevel:        req.RiskLevel,
		Metadata:         req.Metadata,
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
	now := time.Now().UTC()
	ownerUserID := s.createReq.OwnerUserID
	if ownerUserID == uuid.Nil {
		ownerUserID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	}
	return []*employee.DigitalEmployee{{
		ID:               s.createdID,
		TenantID:         req.TenantID,
		TeamID:           req.TeamID,
		OwnerUserID:      ownerUserID,
		EmployeeType:     "database_admin",
		Name:             "Database administrator",
		Role:             "database_admin",
		Status:           employee.DigitalEmployeeStatusReady,
		PermissionPolicy: map[string]any{},
		RiskLevel:        "medium",
		Metadata:         map[string]any{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}}, nil
}

func (s *routeEmployeeService) GetActivity(ctx context.Context, req employee.GetDigitalEmployeeActivityRequest) (*employee.DigitalEmployeeActivity, error) {
	return &employee.DigitalEmployeeActivity{Items: []employee.DigitalEmployeeActivityItem{}}, nil
}

func (s *routeEmployeeService) GetOverview(ctx context.Context, req employee.GetDigitalEmployeeOverviewRequest) (*employee.DigitalEmployeeOverview, error) {
	s.overviewReq = req
	if s.overviewErr != nil {
		return nil, s.overviewErr
	}
	return routeEmployeeOverview(req), nil
}

func (s *routeEmployeeService) GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*employee.DigitalEmployee, error) {
	s.getCalled = true
	s.getTenantID = tenantID
	now := time.Now().UTC()
	ownerUserID := s.createReq.OwnerUserID
	if ownerUserID == uuid.Nil {
		ownerUserID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	}
	return &employee.DigitalEmployee{
		ID:               employeeID,
		TenantID:         tenantID,
		OwnerUserID:      ownerUserID,
		EmployeeType:     "database_admin",
		Name:             "Database administrator",
		Role:             "database_admin",
		Status:           employee.DigitalEmployeeStatusReady,
		PermissionPolicy: map[string]any{},
		RiskLevel:        "medium",
		Metadata:         map[string]any{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeEmployeeService) ListEnvironmentVariables(ctx context.Context, req employee.ListEnvironmentVariablesRequest) ([]employee.EnvironmentVariableSummary, error) {
	s.listEnvReq = req
	return []employee.EnvironmentVariableSummary{{
		Name:        "GH_TOKEN",
		Configured:  true,
		Fingerprint: "abc123",
		Sensitive:   true,
		Status:      employee.EnvironmentVariableStatusActive,
	}}, nil
}

func (s *routeEmployeeService) UpsertEnvironmentVariable(ctx context.Context, req employee.UpsertEnvironmentVariableRequest) (employee.EnvironmentVariableSummary, error) {
	s.upsertEnvReq = req
	return employee.EnvironmentVariableSummary{
		Name:        req.Name,
		Configured:  true,
		Fingerprint: "abc123",
		Sensitive:   true,
		Status:      employee.EnvironmentVariableStatusActive,
	}, nil
}

func (s *routeEmployeeService) DeleteEnvironmentVariable(ctx context.Context, req employee.DeleteEnvironmentVariableRequest) error {
	s.deleteEnvReq = req
	return nil
}

func (s *routeEmployeeService) DeleteDigitalEmployee(ctx context.Context, req employee.DeleteDigitalEmployeeRequest) error {
	s.deleteReq = req
	return s.deleteErr
}

func (s *routeEmployeeService) ReassignTeam(ctx context.Context, req employee.ReassignDigitalEmployeeTeamRequest) (*employee.DigitalEmployee, error) {
	return nil, nil
}

func (s *routeEmployeeService) UpdateStatus(ctx context.Context, req employee.UpdateStatusRequest) (*employee.DigitalEmployee, error) {
	s.updateCalled = true
	s.updateReq = req
	now := time.Now().UTC()
	return &employee.DigitalEmployee{
		ID:               req.DigitalEmployeeID,
		TenantID:         req.TenantID,
		OwnerUserID:      uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		EmployeeType:     "database_admin",
		Name:             "Database administrator",
		Role:             "database_admin",
		Status:           req.Status,
		PermissionPolicy: map[string]any{},
		RiskLevel:        "medium",
		Metadata:         map[string]any{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeEmployeeService) UpdateProfile(ctx context.Context, req employee.UpdateProfileRequest) (*employee.DigitalEmployee, error) {
	s.updateProfileCalled = true
	s.updateProfileReq = req
	now := time.Now().UTC()
	return &employee.DigitalEmployee{
		ID:               req.DigitalEmployeeID,
		TenantID:         req.TenantID,
		OwnerUserID:      uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		EmployeeType:     "database_admin",
		Name:             "Database administrator",
		Role:             "database_admin",
		Description:      req.Description,
		Status:           employee.DigitalEmployeeStatusReady,
		PermissionPolicy: map[string]any{},
		RiskLevel:        "medium",
		Metadata:         map[string]any{},
		ProjectSummary:   employee.DigitalEmployeeProjectSummary{Projects: []employee.DigitalEmployeeProjectLinkSummary{}},
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeEmployeeService) SubmitPermissionChange(ctx context.Context, req employee.SubmitPermissionChangeRequest) (*approval.ApprovalRequest, error) {
	return nil, employee.ErrPermissionApprovalNotConfigured
}

func (s *routeEmployeeService) CreateConfigRevision(ctx context.Context, req employee.CreateDigitalEmployeeConfigRevisionRequest) (*employee.DigitalEmployeeConfigRevision, error) {
	s.configCalled = true
	s.configRevisionReq = req
	now := time.Now().UTC()
	persona := ""
	if req.PersonaMemoryMarkdown != nil {
		persona = *req.PersonaMemoryMarkdown
	}
	return &employee.DigitalEmployeeConfigRevision{
		ID:                    uuid.New(),
		TenantID:              req.TenantID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		RevisionNumber:        1,
		PersonaMemoryMarkdown: persona,
		CapabilityBindings:    req.CapabilityBindings,
		BudgetPolicy:          req.BudgetPolicy,
		Status:                employee.ConfigRevisionStatusDraft,
		CreatedAt:             now,
		UpdatedAt:             now,
	}, nil
}

func (s *routeEmployeeService) GetSchedulingReadiness(ctx context.Context, tenantID, employeeID uuid.UUID) (*employee.DigitalEmployeeSchedulingReadiness, error) {
	s.getSchedulingReadinessCalled = true
	s.getSchedulingReadinessTenant = tenantID
	s.getSchedulingReadinessEmployeeID = employeeID
	if s.returnNilSchedulingReadiness {
		return nil, nil
	}
	return &employee.DigitalEmployeeSchedulingReadiness{
		EmployeeID:                employeeID,
		Status:                    employee.DigitalEmployeeStatusReady,
		ReadyForProjectScheduling: true,
		ProjectExecutionSource:    "project_runtime_readiness",
		Checks: []employee.SchedulingReadinessCheck{{
			Code:    "employee_status",
			Status:  employee.ReadinessCheckPassed,
			Label:   "员工状态",
			Message: "员工状态为 ready，可进入项目调度池。",
		}},
		Capabilities: employee.SchedulingReadinessCapabilities{
			Skills: employee.SchedulingReadinessSkillSummary{
				PersonalCount:   2,
				InheritedCount:  3,
				MissingRequired: []string{},
			},
			MCPServers: employee.SchedulingReadinessMCPSummary{
				PersonalCount:  1,
				InheritedCount: 2,
			},
			EnvironmentVariables: employee.SchedulingReadinessEnvironmentSummary{
				ConfiguredCount: 4,
				MissingNames:    []string{"JIRA_TOKEN"},
			},
		},
	}, nil
}

func (s *routeEmployeeService) ListEmployeeTemplates(ctx context.Context, tenantID uuid.UUID) ([]employee.EmployeeTemplateRecord, error) {
	return nil, nil
}

func (s *routeEmployeeService) GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (employee.EmployeeTemplateRecord, error) {
	return employee.EmployeeTemplateRecord{}, nil
}

func (s *routeEmployeeService) CreateEmployeeTemplate(ctx context.Context, params employee.CreateEmployeeTemplateParams) (employee.EmployeeTemplateRecord, error) {
	return employee.EmployeeTemplateRecord{}, nil
}

func (s *routeEmployeeService) UpdateEmployeeTemplate(ctx context.Context, params employee.UpdateEmployeeTemplateParams) (employee.EmployeeTemplateRecord, error) {
	return employee.EmployeeTemplateRecord{}, nil
}

func (s *routeEmployeeService) SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (employee.EmployeeTemplateRecord, error) {
	return employee.EmployeeTemplateRecord{}, nil
}

func (s *routeEmployeeService) DeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error {
	return nil
}

func (s *routeEmployeeService) called() bool {
	return s.createCalled ||
		s.listCalled ||
		s.getCalled ||
		s.updateCalled ||
		s.updateProfileCalled ||
		s.configCalled ||
		s.getSchedulingReadinessCalled
}

var _ employee.HandlerService = (*routeEmployeeService)(nil)

type routeEmployeeRunService struct {
	createReq        employee.CreateDigitalEmployeeRunRequest
	stopReq          employee.StopDigitalEmployeeRunRequest
	listTenantID     uuid.UUID
	listEmployeeID   uuid.UUID
	listLimit        int32
	listOffset       int32
	listStatuses     []string
	listProjectID    *uuid.UUID
	listFrom         *time.Time
	listTo           *time.Time
	listRunKind      *string
	listCalled       bool
	getTenantID      uuid.UUID
	getEmployeeID    uuid.UUID
	getRunID         uuid.UUID
	eventsTenantID   uuid.UUID
	eventsEmployeeID uuid.UUID
	eventsRunID      uuid.UUID
	eventsLimit      int32
	eventsOffset     int32
	createdRun       *employee.DigitalEmployeeRun
	statsTenantID    uuid.UUID
	statsEmployeeID  uuid.UUID
	stats            *employee.DigitalEmployeeRunStats
	statsErr         error
}

func (s *routeEmployeeRunService) CreateRun(ctx context.Context, req employee.CreateDigitalEmployeeRunRequest) (*employee.DigitalEmployeeRun, error) {
	s.createReq = req
	run := routeEmployeeRun(req.TenantID, req.DigitalEmployeeID, employee.DigitalEmployeeRunStatusDispatching)
	s.createdRun = run
	return run, nil
}

func (s *routeEmployeeRunService) ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter employee.DigitalEmployeeRunListFilter) (*employee.DigitalEmployeeRunListResult, error) {
	s.listCalled = true
	s.listTenantID = tenantID
	s.listEmployeeID = employeeID
	s.listLimit = filter.Limit
	s.listOffset = filter.Offset
	s.listStatuses = filter.Statuses
	s.listProjectID = filter.ProjectID
	s.listFrom = filter.From
	s.listTo = filter.To
	s.listRunKind = filter.RunKind
	var run *employee.DigitalEmployeeRun
	if s.createdRun != nil {
		run = s.createdRun
	} else {
		run = routeEmployeeRun(tenantID, employeeID, employee.DigitalEmployeeRunStatusDispatching)
	}
	return &employee.DigitalEmployeeRunListResult{
		Items:      []employee.DigitalEmployeeRunListItem{{Run: run}},
		TotalCount: 1,
	}, nil
}

func (s *routeEmployeeRunService) GetRunCalendar(_ context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) (*employee.DigitalEmployeeRunCalendarResult, error) {
	return &employee.DigitalEmployeeRunCalendarResult{
		From:       from,
		To:         to,
		TotalCount: 0,
		Truncated:  false,
		Items:      []employee.DigitalEmployeeRunCalendarItem{},
	}, nil
}

func (s *routeEmployeeRunService) GetRun(ctx context.Context, tenantID, employeeID, runID uuid.UUID) (*employee.DigitalEmployeeRun, error) {
	s.getTenantID = tenantID
	s.getEmployeeID = employeeID
	s.getRunID = runID
	run := routeEmployeeRun(tenantID, employeeID, employee.DigitalEmployeeRunStatusDispatching)
	run.ID = runID
	return run, nil
}

func (s *routeEmployeeRunService) ListRunEvents(ctx context.Context, tenantID, employeeID, runID uuid.UUID, limit, offset int32) ([]employee.RuntimeCommandEventWriteback, error) {
	s.eventsTenantID = tenantID
	s.eventsEmployeeID = employeeID
	s.eventsRunID = runID
	s.eventsLimit = limit
	s.eventsOffset = offset
	return []employee.RuntimeCommandEventWriteback{{
		EventType:      "provider_output",
		SequenceNumber: 7,
		Payload:        map[string]any{"text": "running"},
		Metadata:       map[string]any{"source": "test"},
	}}, nil
}

func (s *routeEmployeeRunService) GetRunStats(ctx context.Context, tenantID, employeeID uuid.UUID) (*employee.DigitalEmployeeRunStats, error) {
	s.statsTenantID = tenantID
	s.statsEmployeeID = employeeID
	if s.statsErr != nil {
		return nil, s.statsErr
	}
	if s.stats != nil {
		return s.stats, nil
	}
	return &employee.DigitalEmployeeRunStats{}, nil
}

func (s *routeEmployeeRunService) StopRun(ctx context.Context, req employee.StopDigitalEmployeeRunRequest) (*employee.DigitalEmployeeRun, error) {
	s.stopReq = req
	run := routeEmployeeRun(req.TenantID, req.DigitalEmployeeID, employee.DigitalEmployeeRunStatusCancelling)
	run.ID = req.RunID
	return run, nil
}

func (s *routeEmployeeRunService) AcknowledgeFailedRun(_ context.Context, tenantID, employeeID, runID, _ uuid.UUID) (*employee.DigitalEmployeeRun, error) {
	run := routeEmployeeRun(tenantID, employeeID, employee.DigitalEmployeeRunStatusFailed)
	run.ID = runID
	now := time.Now().UTC()
	run.FailureAcknowledgedAt = &now
	return run, nil
}

func (s *routeEmployeeRunService) RetryFailedRun(_ context.Context, tenantID, employeeID, _, _ uuid.UUID) (*employee.DigitalEmployeeRun, error) {
	return routeEmployeeRun(tenantID, employeeID, employee.DigitalEmployeeRunStatusQueued), nil
}

func routeEmployeeOverview(req employee.GetDigitalEmployeeOverviewRequest) *employee.DigitalEmployeeOverview {
	employeeID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	teamID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	ownerID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	executionInstanceID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	runtimeNodeID := uuid.MustParse("55555555-5555-4555-8555-555555555555")
	runID := uuid.MustParse("66666666-6666-4666-8666-666666666666")
	taskID := uuid.MustParse("77777777-7777-4777-8777-777777777777")
	now := time.Date(2026, 6, 6, 10, 4, 0, 0, time.UTC)
	finishedAt := now.Add(10 * time.Minute)
	costAmount := 12.34
	return &employee.DigitalEmployeeOverview{
		Summary:      employee.DigitalEmployeeOverviewSummary{TotalCount: 1, RunnableCount: 1, RunningCount: 1, WaitingRuntimeCount: 0, ErrorCount: 0, HighRiskCount: 0, ReadyCount: 1, NeedsConfigurationCount: 0, PendingConfigApprovalCount: 0, FailedRecentRunCount: 0, OperationalStatusCounts: map[employee.DigitalEmployeeOperationalStatus]int32{employee.DigitalEmployeeOperationalStatusIdle: 1}},
		QueueSummary: employee.DigitalEmployeeOverviewQueueSummary{NeedsConfigurationCount: 0, StaleConfigCount: 0, FailedRecentRunCount: 0},
		Items: []employee.DigitalEmployeeOverviewItem{{
			IdentitySummary:   employee.DigitalEmployeeIdentitySummary{ID: employeeID, TenantID: req.TenantID, TeamID: &teamID, TeamName: "产品组", OwnerUserID: ownerID, OwnerDisplayName: "王佩", EmployeeType: "requirements_analyst", EmployeeTypeLabel: "需求分析", Name: "需求分析员工", Role: "requirements_analyst", Description: stringPtr("负责需求拆解和交付风险识别"), Status: employee.DigitalEmployeeStatusActive, RiskLevel: "medium"},
			ExecutionSummary:  employee.DigitalEmployeeExecutionSummary{ExecutionInstanceID: &executionInstanceID, Status: employee.OverviewExecutionStatusReady, RuntimeNodeID: &runtimeNodeID, NodeID: "runtime-cn-01", RuntimeName: "cn-01", RuntimeStatus: "online", ProviderType: "codex", ProviderStatus: "healthy", HealthStatus: "healthy", AgentHomeDirAvailable: true},
			LatestRunSummary:  &employee.DigitalEmployeeLatestRunSummary{RunID: runID, TaskID: taskID, Status: employee.OverviewRunStatusCompleted, Title: "审查需求", StartedAt: &now, UpdatedAt: &now, FinishedAt: &finishedAt, DurationSec: int32Ptr(240), TokenUsage: int32Ptr(1600), ErrorMessage: ""},
			GovernanceSummary: employee.DigitalEmployeeGovernanceSummary{Status: "approved", TeamRevisionNumber: int32Ptr(3), EmployeeRevisionNumber: int32Ptr(1), SkillsCount: 8, MCPServersCount: 3},
			BudgetSummary:     employee.DigitalEmployeeBudgetSummary{DailyTokenLimit: int32Ptr(10000), UsageTokensToday: 2500, UsagePercentToday: int32Ptr(25), LimitExceeded: false, UsageTokens30d: int32Ptr(16000), RunCount30d: 12, CostAmount30d: &costAmount, Currency: "USD", Source: "run_usage_projection"},
			WorkbenchStatus:   employee.WorkbenchStatusReady,
			OperationalState:  employee.DigitalEmployeeOperationalState{Status: employee.DigitalEmployeeOperationalStatusIdle, Reasons: []employee.DigitalEmployeeOperationalReason{}, CanDispatch: true},
			RecentEvents: []employee.DigitalEmployeeRecentEventSummary{
				{Label: "命令已下发", Status: "running", OccurredAt: &now},
				{Label: "Provider 输出中", Status: "running", OccurredAt: &now},
				{Label: "等待结果回写", Status: "completed", OccurredAt: &finishedAt},
			},
		}},
		Filters:    employee.DigitalEmployeeOverviewFilters{Teams: []employee.OverviewFilterOption{{Value: teamID.String(), Label: "产品组"}}, Providers: []employee.OverviewFilterOption{{Value: "codex", Label: "Codex"}}, RunStatuses: []employee.OverviewFilterOption{{Value: string(employee.OverviewRunStatusNone), Label: "暂无运行"}}},
		Pagination: employee.OverviewPagination{Limit: req.Limit, Offset: req.Offset, TotalCount: 1},
	}
}

func routeEmployeeRun(tenantID, employeeID uuid.UUID, status employee.DigitalEmployeeRunStatus) *employee.DigitalEmployeeRun {
	now := time.Now().UTC()
	logRef := "s3://logs/run.log"
	idempotencyKey := "idem-route-test"
	idempotencyFingerprint := "fingerprint-route-test"
	return &employee.DigitalEmployeeRun{
		ID:                  uuid.New(),
		TenantID:            tenantID,
		TaskID:              uuid.New(),
		DigitalEmployeeID:   employeeID,
		ExecutionInstanceID: uuid.New(),
		RuntimeNodeID:       uuid.New(),
		NodeID:              "runtime-node-1",
		CommandID:           "cmd-route-test",
		ProviderType:        "codex",
		Status:              status,
		Result:              map[string]any{"summary": "queued"},
		Diagnostic:          map[string]any{"phase": "dispatch"},
		LogRef:              &logRef,
		WorkProducts: []employee.WorkProduct{{
			Type:      "finding",
			Title:     "风险清单",
			Ref:       "artifact://risk-list",
			CreatedAt: now,
		}},
		SessionState:           map[string]any{"step": "dispatch"},
		IdempotencyKey:         &idempotencyKey,
		IdempotencyFingerprint: &idempotencyFingerprint,
		TimeoutSec:             int32Ptr(600),
		GraceSec:               int32Ptr(30),
		StartedAt:              now,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
}

func stringPtr(value string) *string {
	return &value
}

func int32Ptr(value int32) *int32 {
	return &value
}

var _ employee.RunHandlerService = (*routeEmployeeRunService)(nil)
