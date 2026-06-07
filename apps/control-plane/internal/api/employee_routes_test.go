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
	runtimeNodeID := uuid.New()
	spoofedOwnerID := uuid.New()

	optionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/create-options?team_id="+teamID.String(), nil)
	optionsReq.AddCookie(cookie)
	optionsResp := httptest.NewRecorder()
	server.ServeHTTP(optionsResp, optionsReq)
	if optionsResp.Code != http.StatusOK {
		t.Fatalf("expected create options to succeed, got %d: %s", optionsResp.Code, optionsResp.Body.String())
	}
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	if service.createOptionsReq.TenantID != expectedTenantID || service.createOptionsReq.TeamID != teamID {
		t.Fatalf("expected create options tenant/team %s/%s, got %#v", expectedTenantID, teamID, service.createOptionsReq)
	}
	var optionsBody struct {
		TeamConfig struct {
			AllowedEmployeeTypes []string `json:"allowed_employee_types"`
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
	}
	if err := json.NewDecoder(optionsResp.Body).Decode(&optionsBody); err != nil {
		t.Fatalf("decode create options: %v", err)
	}
	if len(optionsBody.TeamConfig.AllowedEmployeeTypes) != 1 || optionsBody.TeamConfig.AllowedEmployeeTypes[0] != "database_admin" {
		t.Fatalf("expected team config allowed employee types, got %#v", optionsBody.TeamConfig)
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
		"context_policy":{"scope":"task"},
		"approval_policy":{"required_for":["deploy"]},
		"risk_level":"medium",
		"metadata":{"source":"route-test"},
		"role_profile":{"title":"database administrator"},
		"constitution_addendum":{"tone":"concise"},
		"capability_selection":{"enabled_skills":["incident-diagnosis"]},
		"context_policy_override":{"redaction":"strict"},
		"approval_policy_override":{"require_owner":true},
		"budget_policy":{"daily_token_limit":12000},
		"output_contract_addendum":{"format":"markdown"},
		"runtime_node_id":"` + runtimeNodeID.String() + `",
		"provider_type":"codex",
		"session_policy":{"mode":"reuse_latest"},
		"workspace_policy":{"labels":{"tier":"standard"}}
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
	if service.createReq.RuntimeNodeID != runtimeNodeID || service.createReq.ProviderType != "codex" {
		t.Fatalf("expected create runtime/provider %s/codex, got %s/%q", runtimeNodeID, service.createReq.RuntimeNodeID, service.createReq.ProviderType)
	}
	if service.createReq.PermissionPolicy["allowed_actions"] == nil || service.createReq.RoleProfile["title"] != "database administrator" || service.createReq.CapabilitySelection["enabled_skills"] == nil {
		t.Fatalf("expected policy/config fields from create body, got %#v", service.createReq)
	}
	if service.createReq.ContextPolicyOverride["redaction"] != "strict" || service.createReq.ApprovalPolicyOverride["require_owner"] != true || service.createReq.OutputContractAddendum["format"] != "markdown" {
		t.Fatalf("expected override/addendum fields from create body, got %#v", service.createReq)
	}
	if service.createReq.BudgetPolicy["daily_token_limit"] != float64(12000) {
		t.Fatalf("expected budget policy from create body, got %#v", service.createReq.BudgetPolicy)
	}
	if service.createReq.SessionPolicy["mode"] != "reuse_latest" {
		t.Fatalf("expected session policy from create body, got %#v", service.createReq.SessionPolicy)
	}
	workspaceLabels, ok := service.createReq.WorkspacePolicy["labels"].(map[string]any)
	if !ok || workspaceLabels["tier"] != "standard" {
		t.Fatalf("expected workspace policy from create body, got %#v", service.createReq.WorkspacePolicy)
	}
	var created struct {
		ID               string         `json:"id"`
		TenantID         string         `json:"tenant_id"`
		TeamID           string         `json:"team_id"`
		OwnerUserID      string         `json:"owner_user_id"`
		EmployeeType     string         `json:"employee_type"`
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
	if created.OwnerUserID != user.ID.String() || created.EmployeeType != "database_admin" {
		t.Fatalf("expected response owner/type %s/database_admin, got %#v", user.ID, created)
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

	bindRuntimeNodeID := uuid.New()
	upsertReq := httptest.NewRequest(http.MethodPut, "/api/v1/digital-employees/"+created.ID+"/execution-instance", strings.NewReader(`{"runtime_node_id":"`+bindRuntimeNodeID.String()+`","provider_type":"codex","agent_home_dir":"/srv/agents/requirements","workspace_policy":{},"session_policy":{}}`))
	upsertReq.Header.Set("Content-Type", "application/json")
	upsertReq.AddCookie(cookie)
	upsertResp := httptest.NewRecorder()
	server.ServeHTTP(upsertResp, upsertReq)
	if upsertResp.Code != http.StatusOK {
		t.Fatalf("expected upsert execution instance to succeed, got %d: %s", upsertResp.Code, upsertResp.Body.String())
	}
	if service.bindReq.TenantID != expectedTenantID || service.bindReq.RuntimeNodeID != bindRuntimeNodeID {
		t.Fatalf("expected bind tenant/runtime %s/%s, got %s/%s", expectedTenantID, bindRuntimeNodeID, service.bindReq.TenantID, service.bindReq.RuntimeNodeID)
	}

	spoofedConfigApproverID := uuid.New()
	configReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+created.ID+"/config-revisions", strings.NewReader(`{"role_profile":{"title":"requirements analyst"},"capability_selection":{"enabled_skills":["incident-diagnosis"]},"budget_policy":{"daily_token_limit":9000},"approved_by":"`+spoofedConfigApproverID.String()+`"}`))
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

func TestDigitalEmployeeOverviewRouteUsesConsoleTenantAndFilters(t *testing.T) {
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
	runtimeNodeID := uuid.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/digital-employees/overview?q=%E9%9C%80%E6%B1%82&team_id="+teamID.String()+"&status=active&employee_type=requirements_analyst&provider_type=codex&runtime_node_id="+runtimeNodeID.String()+"&risk_level=medium&execution_status=missing&run_status=none&limit=25&offset=5",
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
		service.overviewReq.RuntimeNodeID == nil ||
		*service.overviewReq.RuntimeNodeID != runtimeNodeID ||
		service.overviewReq.RiskLevel != "medium" ||
		service.overviewReq.ExecutionStatus != employee.OverviewExecutionStatusMissing ||
		service.overviewReq.RunStatus != employee.OverviewRunStatusNone ||
		service.overviewReq.Limit != 25 ||
		service.overviewReq.Offset != 5 {
		t.Fatalf("unexpected overview filters: %#v", service.overviewReq)
	}

	var body struct {
		Summary struct {
			TotalCount          int32 `json:"total_count"`
			RunnableCount       int32 `json:"runnable_count"`
			RunningCount        int32 `json:"running_count"`
			WaitingRuntimeCount int32 `json:"waiting_runtime_count"`
			ErrorCount          int32 `json:"error_count"`
			HighRiskCount       int32 `json:"high_risk_count"`
		} `json:"summary"`
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
				RunCount30d   int32    `json:"run_count_30d"`
				CostAmount30d *float64 `json:"cost_amount_30d"`
				Source        string   `json:"source"`
			} `json:"budget_summary"`
		} `json:"items"`
		Filters struct {
			Teams []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"teams"`
			ExecutionStatuses []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"execution_statuses"`
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
	if len(body.Items) != 1 || body.Items[0].IdentitySummary.Name != "需求分析员工" || body.Items[0].ExecutionSummary.ProviderType != "codex" {
		t.Fatalf("unexpected overview items: %#v", body.Items)
	}
	if body.Items[0].LatestRunSummary == nil || body.Items[0].LatestRunSummary.TokenUsage != 1600 {
		t.Fatalf("expected latest run token usage, got %#v", body.Items[0].LatestRunSummary)
	}
	if body.Items[0].LatestRunSummary.FinishedAt == nil || body.Items[0].LatestRunSummary.ErrorMessage != "执行超时" {
		t.Fatalf("expected latest run finished/error fields, got %#v", body.Items[0].LatestRunSummary)
	}
	if body.Items[0].BudgetSummary.CostAmount30d == nil || *body.Items[0].BudgetSummary.CostAmount30d != 12.34 {
		t.Fatalf("expected budget cost amount, got %#v", body.Items[0].BudgetSummary)
	}
	if len(body.Filters.Teams) != 1 || body.Filters.Teams[0].Label != "产品组" {
		t.Fatalf("expected team filters, got %#v", body.Filters.Teams)
	}
	if len(body.Filters.ExecutionStatuses) == 0 || body.Filters.ExecutionStatuses[0].Value == "" {
		t.Fatalf("expected execution status filters, got %#v", body.Filters.ExecutionStatuses)
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
	tenantID := uuid.MustParse(auth.DefaultTenantID)
	teamID := uuid.New()
	service := &routeEmployeeService{
		createOptions: &employee.CreateOptions{
			TeamConfig: employee.TeamConfigCreateOption{
				ID:             uuid.New(),
				TenantID:       tenantID,
				TeamID:         teamID,
				RevisionNumber: 1,
				Status:         employee.TeamConfigRevisionStatusActive,
			},
			EmployeeTypes: []employee.EmployeeTypeDefinition{{
				Type:        "database_admin",
				Label:       "数据库管理",
				Description: "Manages database operations",
				DefaultRole: "database_admin",
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
			AllowedEmployeeTypes        []string `json:"allowed_employee_types"`
			AllowedProviderTypes        []string `json:"allowed_provider_types"`
			AllowedSkills               []string `json:"allowed_skills"`
			AllowedMCPServers           []string `json:"allowed_mcp_servers"`
			AllowedExternalCapabilities []string `json:"allowed_external_capabilities"`
		} `json:"team_config"`
		EmployeeTypes []struct {
			RecommendedSkills        []string `json:"recommended_skills"`
			RecommendedMCPServers    []string `json:"recommended_mcp_servers"`
			RecommendedProviderTypes []string `json:"recommended_provider_types"`
		} `json:"employee_types"`
		CapabilityOptions struct {
			ProviderTypes        []string `json:"provider_types"`
			Skills               []string `json:"skills"`
			MCPServers           []string `json:"mcp_servers"`
			ExternalCapabilities []string `json:"external_capabilities"`
		} `json:"capability_options"`
		RuntimeProviderOptions []struct{} `json:"runtime_provider_options"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode create options: %v", err)
	}
	assertNonNilEmptyStringSlice(t, "team_config.allowed_employee_types", body.TeamConfig.AllowedEmployeeTypes)
	assertNonNilEmptyStringSlice(t, "team_config.allowed_provider_types", body.TeamConfig.AllowedProviderTypes)
	assertNonNilEmptyStringSlice(t, "team_config.allowed_skills", body.TeamConfig.AllowedSkills)
	assertNonNilEmptyStringSlice(t, "team_config.allowed_mcp_servers", body.TeamConfig.AllowedMCPServers)
	assertNonNilEmptyStringSlice(t, "team_config.allowed_external_capabilities", body.TeamConfig.AllowedExternalCapabilities)
	if len(body.EmployeeTypes) != 1 {
		t.Fatalf("expected one employee type option, got %#v", body.EmployeeTypes)
	}
	assertNonNilEmptyStringSlice(t, "employee_types[0].recommended_skills", body.EmployeeTypes[0].RecommendedSkills)
	assertNonNilEmptyStringSlice(t, "employee_types[0].recommended_mcp_servers", body.EmployeeTypes[0].RecommendedMCPServers)
	assertNonNilEmptyStringSlice(t, "employee_types[0].recommended_provider_types", body.EmployeeTypes[0].RecommendedProviderTypes)
	assertNonNilEmptyStringSlice(t, "capability_options.provider_types", body.CapabilityOptions.ProviderTypes)
	assertNonNilEmptyStringSlice(t, "capability_options.skills", body.CapabilityOptions.Skills)
	assertNonNilEmptyStringSlice(t, "capability_options.mcp_servers", body.CapabilityOptions.MCPServers)
	assertNonNilEmptyStringSlice(t, "capability_options.external_capabilities", body.CapabilityOptions.ExternalCapabilities)
	if body.RuntimeProviderOptions == nil || len(body.RuntimeProviderOptions) != 0 {
		t.Fatalf("expected runtime_provider_options to decode as empty array, got %#v", body.RuntimeProviderOptions)
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
	if tenantID != uuid.MustParse(auth.DefaultTenantID) {
		t.Fatalf("route auth service only supports default tenant %s, got %s", auth.DefaultTenantID, tenantID)
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
	tenantID := uuid.MustParse(auth.DefaultTenantID)
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
	var listRaw []map[string]json.RawMessage
	if err := json.Unmarshal(listResp.Body.Bytes(), &listRaw); err != nil {
		t.Fatalf("decode raw list runs: %v", err)
	}
	if len(listRaw) != 1 {
		t.Fatalf("unexpected raw list runs response: %#v", listRaw)
	}
	if _, ok := listRaw[0]["idempotency_fingerprint"]; ok {
		t.Fatalf("list run response must not expose idempotency_fingerprint: %s", string(listRaw[0]["idempotency_fingerprint"]))
	}
	if string(listRaw[0]["idempotency_key"]) != `"idem-route-test"` {
		t.Fatalf("expected list run response to expose idempotency_key, got %s", string(listRaw[0]["idempotency_key"]))
	}
	var listBody []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list runs: %v", err)
	}
	if len(listBody) != 1 || listBody[0].ID != created.ID {
		t.Fatalf("unexpected list runs response: %#v", listBody)
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
	for i := 0; i < 19; i++ {
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

func TestDigitalEmployeeRouteAuthorizationDenial(t *testing.T) {
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
		{name: "status", method: http.MethodPut, path: "/api/v1/digital-employees/" + employeeID + "/status", body: `{"status":"active"}`, action: authz.ActionEmployeeStatusUpdate, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "get execution instance", method: http.MethodGet, path: "/api/v1/digital-employees/" + employeeID + "/execution-instance", action: authz.ActionEmployeeRead, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "upsert execution instance", method: http.MethodPut, path: "/api/v1/digital-employees/" + employeeID + "/execution-instance", body: `{"runtime_node_id":"` + runtimeNodeID + `","provider_type":"codex","agent_home_dir":"/srv/agents/requirements"}`, action: authz.ActionEmployeeExecutionBind, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "create config revision", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/config-revisions", body: `{"role_profile":{"title":"analyst"}}`, action: authz.ActionEmployeeConfigCreate, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "preview effective config", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/effective-configs/preview", body: `{"team_config":{"id":"` + uuid.New().String() + `"},"employee_config":{"id":"` + uuid.New().String() + `"}}`, action: authz.ActionEmployeeConfigPreview, resourceType: authz.ResourceEmployee, resourceID: employeeID},
		{name: "approve effective config", method: http.MethodPost, path: "/api/v1/digital-employees/" + employeeID + "/effective-configs/approve", body: `{"preview":{"team_config":{"id":"` + uuid.New().String() + `"},"employee_config":{"id":"` + uuid.New().String() + `"}}}`, action: authz.ActionEmployeeConfigApprove, resourceType: authz.ResourceEmployee, resourceID: employeeID},
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
	createOptionsReq    employee.CreateOptionsRequest
	createOptions       *employee.CreateOptions
	createReq           employee.CreateDigitalEmployeeRequest
	listReq             employee.ListDigitalEmployeesRequest
	overviewReq         employee.GetDigitalEmployeeOverviewRequest
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
	overviewErr         error
}

func (s *routeEmployeeService) GetCreateOptions(ctx context.Context, req employee.CreateOptionsRequest) (*employee.CreateOptions, error) {
	s.createOptionsReq = req
	if s.createOptions != nil {
		return s.createOptions, nil
	}
	return &employee.CreateOptions{
		TeamConfig: employee.TeamConfigCreateOption{
			ID:                   uuid.New(),
			TenantID:             req.TenantID,
			TeamID:               req.TeamID,
			RevisionNumber:       2,
			Status:               employee.TeamConfigRevisionStatusActive,
			AllowedEmployeeTypes: []string{"database_admin"},
			AllowedProviderTypes: []string{"codex"},
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
			PermissionPolicy:      map[string]any{},
			ContextPolicyOverride: map[string]any{},
			ApprovalPolicy:        map[string]any{},
			CapabilitySelection:   map[string]any{},
			RuntimeSelector:       map[string]any{},
			WorkspacePolicy:       map[string]any{},
			SessionPolicy:         map[string]any{"mode": "reuse_latest"},
			Metadata:              map[string]any{},
		},
	}, nil
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
		Name:             req.Name,
		Role:             req.Role,
		Status:           employee.DigitalEmployeeStatusReady,
		PermissionPolicy: req.PermissionPolicy,
		ContextPolicy:    req.ContextPolicy,
		ApprovalPolicy:   req.ApprovalPolicy,
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
		ContextPolicy:    map[string]any{},
		ApprovalPolicy:   map[string]any{},
		RiskLevel:        "medium",
		Metadata:         map[string]any{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}}, nil
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
		OwnerUserID:      uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		EmployeeType:     "database_admin",
		Name:             "Database administrator",
		Role:             "database_admin",
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
		BudgetPolicy:           req.BudgetPolicy,
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

type routeEmployeeRunService struct {
	createReq        employee.CreateDigitalEmployeeRunRequest
	stopReq          employee.StopDigitalEmployeeRunRequest
	listTenantID     uuid.UUID
	listEmployeeID   uuid.UUID
	listLimit        int32
	listOffset       int32
	getTenantID      uuid.UUID
	getEmployeeID    uuid.UUID
	getRunID         uuid.UUID
	eventsTenantID   uuid.UUID
	eventsEmployeeID uuid.UUID
	eventsRunID      uuid.UUID
	eventsLimit      int32
	eventsOffset     int32
	createdRun       *employee.DigitalEmployeeRun
}

func (s *routeEmployeeRunService) CreateRun(ctx context.Context, req employee.CreateDigitalEmployeeRunRequest) (*employee.DigitalEmployeeRun, error) {
	s.createReq = req
	run := routeEmployeeRun(req.TenantID, req.DigitalEmployeeID, employee.DigitalEmployeeRunStatusDispatching)
	s.createdRun = run
	return run, nil
}

func (s *routeEmployeeRunService) ListRuns(ctx context.Context, tenantID, employeeID uuid.UUID, limit, offset int32) ([]*employee.DigitalEmployeeRun, error) {
	s.listTenantID = tenantID
	s.listEmployeeID = employeeID
	s.listLimit = limit
	s.listOffset = offset
	if s.createdRun != nil {
		return []*employee.DigitalEmployeeRun{s.createdRun}, nil
	}
	return []*employee.DigitalEmployeeRun{routeEmployeeRun(tenantID, employeeID, employee.DigitalEmployeeRunStatusDispatching)}, nil
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

func (s *routeEmployeeRunService) StopRun(ctx context.Context, req employee.StopDigitalEmployeeRunRequest) (*employee.DigitalEmployeeRun, error) {
	s.stopReq = req
	run := routeEmployeeRun(req.TenantID, req.DigitalEmployeeID, employee.DigitalEmployeeRunStatusCancelling)
	run.ID = req.RunID
	return run, nil
}

func routeEmployeeOverview(req employee.GetDigitalEmployeeOverviewRequest) *employee.DigitalEmployeeOverview {
	employeeID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	teamID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	ownerID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	executionInstanceID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	runtimeNodeID := uuid.MustParse("55555555-5555-4555-8555-555555555555")
	runID := uuid.MustParse("66666666-6666-4666-8666-666666666666")
	taskID := uuid.MustParse("77777777-7777-4777-8777-777777777777")
	effectiveConfigID := uuid.MustParse("88888888-8888-4888-8888-888888888888")
	now := time.Date(2026, 6, 6, 10, 4, 0, 0, time.UTC)
	finishedAt := now.Add(10 * time.Minute)
	costAmount := 12.34
	return &employee.DigitalEmployeeOverview{
		Summary: employee.DigitalEmployeeOverviewSummary{TotalCount: 1, RunnableCount: 1, RunningCount: 1, WaitingRuntimeCount: 0, ErrorCount: 0, HighRiskCount: 0},
		Items: []employee.DigitalEmployeeOverviewItem{{
			IdentitySummary:   employee.DigitalEmployeeIdentitySummary{ID: employeeID, TenantID: req.TenantID, TeamID: &teamID, TeamName: "产品组", OwnerUserID: ownerID, OwnerDisplayName: "王佩", EmployeeType: "requirements_analyst", EmployeeTypeLabel: "需求分析", Name: "需求分析员工", Role: "requirements_analyst", Description: stringPtr("负责需求拆解和交付风险识别"), Status: employee.DigitalEmployeeStatusActive, RiskLevel: "medium"},
			ExecutionSummary:  employee.DigitalEmployeeExecutionSummary{ExecutionInstanceID: &executionInstanceID, Status: employee.OverviewExecutionStatusReady, RuntimeNodeID: &runtimeNodeID, NodeID: "runtime-cn-01", RuntimeName: "cn-01", RuntimeStatus: "online", ProviderType: "codex", ProviderStatus: "healthy", HealthStatus: "healthy", AgentHomeDirAvailable: true},
			LatestRunSummary:  &employee.DigitalEmployeeLatestRunSummary{RunID: runID, TaskID: taskID, Status: employee.OverviewRunStatusFailed, Title: "审查需求", StartedAt: &now, UpdatedAt: &now, FinishedAt: &finishedAt, DurationSec: int32Ptr(240), TokenUsage: int32Ptr(1600), ErrorMessage: "执行超时"},
			GovernanceSummary: employee.DigitalEmployeeGovernanceSummary{EffectiveConfigID: &effectiveConfigID, Status: "approved", TeamRevisionNumber: int32Ptr(3), EmployeeRevisionNumber: int32Ptr(1), SkillsCount: 8, MCPServersCount: 3, ConstitutionRef: "effective-config://88888888-8888-4888-8888-888888888888/constitution"},
			BudgetSummary:     employee.DigitalEmployeeBudgetSummary{UsageTokens30d: int32Ptr(16000), RunCount30d: 12, CostAmount30d: &costAmount, Currency: "USD", Source: "run_usage_projection"},
		}},
		Filters:    employee.DigitalEmployeeOverviewFilters{Teams: []employee.OverviewFilterOption{{Value: teamID.String(), Label: "产品组"}}, Providers: []employee.OverviewFilterOption{{Value: "codex", Label: "Codex"}}, ExecutionStatuses: []employee.OverviewFilterOption{{Value: string(employee.OverviewExecutionStatusMissing), Label: "未绑定 Runtime"}}, RunStatuses: []employee.OverviewFilterOption{{Value: string(employee.OverviewRunStatusNone), Label: "暂无运行"}}},
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
