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

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees", strings.NewReader(`{"team_id":"`+teamID.String()+`","name":"Requirements analyst","role":"requirements_analyst","runtime_node_id":"`+runtimeNodeID.String()+`","provider_type":"codex","session_policy":{"mode":"reuse_latest"},"workspace_policy":{"labels":{"tier":"standard"}}}`))
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
	if service.createReq.RuntimeNodeID != runtimeNodeID || service.createReq.ProviderType != "codex" {
		t.Fatalf("expected create runtime/provider %s/codex, got %s/%q", runtimeNodeID, service.createReq.RuntimeNodeID, service.createReq.ProviderType)
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
		name         string
		method       string
		path         string
		body         string
		action       string
		resourceType string
		resourceID   string
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/digital-employees", action: authz.ActionEmployeeRead, resourceType: authz.ResourceTenant},
		{name: "create", method: http.MethodPost, path: "/api/v1/digital-employees", body: `{"team_id":"` + uuid.New().String() + `","name":"Requirements analyst","role":"requirements_analyst"}`, action: authz.ActionEmployeeCreate, resourceType: authz.ResourceTenant},
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

func int32Ptr(value int32) *int32 {
	return &value
}

var _ employee.RunHandlerService = (*routeEmployeeRunService)(nil)
