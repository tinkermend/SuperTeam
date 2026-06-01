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
	"github.com/superteam/control-plane/internal/authzcenter"
	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
)

const routeTaskID = "11111111-1111-1111-1111-111111111111"

func TestRuntimeRoutesAreRegistered(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
	)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/runtime/register", body: `{"node_id":"node-1","name":"node 1","supported_providers":["codex"],"max_slots":1}`},
		{method: http.MethodPost, path: "/api/v1/runtime/heartbeat", body: `{"current_load":0}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/claim"},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/" + routeTaskID + "/events", body: `{"events":[]}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/" + routeTaskID + "/complete", body: `{"result":{}}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/" + routeTaskID + "/fail", body: `{"error":"failed"}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/" + routeTaskID + "/lease"},
		{method: http.MethodGet, path: "/api/v1/runtime/nodes"},
		{method: http.MethodGet, path: "/api/v1/runtime/nodes/node-1"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			server.ServeHTTP(rr, req)

			if rr.Code == http.StatusNotFound {
				t.Fatalf("expected runtime route to be registered, got 404")
			}
		})
	}
}

func TestLegacyRuntimeClaimRouteIsNotRegistered(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/claim", nil)
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected legacy runtime claim route to be removed, got %d", rr.Code)
	}
}

func TestAuthRoutesAreRegistered(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	server := NewServerWithAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
	)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()

	server.ServeHTTP(loginResp, loginReq)

	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	cookies := loginResp.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != auth.SessionCookieName {
		t.Fatalf("expected session cookie, got %#v", cookies)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(cookies[0])
	meResp := httptest.NewRecorder()

	server.ServeHTTP(meResp, meReq)

	if meResp.Code != http.StatusOK {
		t.Fatalf("expected current user to succeed, got %d: %s", meResp.Code, meResp.Body.String())
	}

	logsReq := httptest.NewRequest(http.MethodGet, "/api/auth/login-logs?limit=10&offset=0", nil)
	logsReq.AddCookie(cookies[0])
	logsResp := httptest.NewRecorder()

	server.ServeHTTP(logsResp, logsReq)

	if logsResp.Code != http.StatusOK {
		t.Fatalf("expected login logs to succeed, got %d: %s", logsResp.Code, logsResp.Body.String())
	}
	var logsBody struct {
		Items []struct {
			EventType string `json:"event_type"`
			Username  string `json:"username"`
			Result    string `json:"result"`
		} `json:"items"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsBody); err != nil {
		t.Fatalf("decode login logs response: %v", err)
	}
	if len(logsBody.Items) != 1 {
		t.Fatalf("expected one login log, got %#v", logsBody.Items)
	}
	if logsBody.Items[0].EventType != auth.LoginEventSucceeded {
		t.Fatalf("expected login succeeded log, got %#v", logsBody.Items[0])
	}
}

func TestCurrentUserRequiresConsoleAuthorization(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	authorizer := &routeAuthorizer{allowed: false}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	server.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginResp.Code, loginResp.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(loginResp.Result().Cookies()[0])
	meResp := httptest.NewRecorder()
	server.ServeHTTP(meResp, meReq)

	if meResp.Code != http.StatusForbidden {
		t.Fatalf("expected current user to be forbidden, got %d: %s", meResp.Code, meResp.Body.String())
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authorization check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Action != authz.ActionConsoleAccess {
		t.Fatalf("expected console access action, got %q", check.Action)
	}
	if check.Resource.Type != authz.ResourceConsole {
		t.Fatalf("expected console resource type, got %q", check.Resource.Type)
	}
	if check.Resource.ID != "web" {
		t.Fatalf("expected web console resource ID, got %q", check.Resource.ID)
	}
	if check.Actor.Type != authz.ActorUser {
		t.Fatalf("expected user actor type, got %q", check.Actor.Type)
	}
	if check.Actor.ID != user.ID.String() {
		t.Fatalf("expected actor ID %q, got %q", user.ID.String(), check.Actor.ID)
	}
	if check.TenantID.String() != auth.DefaultTenantID {
		t.Fatalf("expected default tenant ID %q, got %q", auth.DefaultTenantID, check.TenantID.String())
	}
	if check.TeamID != nil {
		t.Fatalf("expected nil team ID, got %v", check.TeamID)
	}
	if check.AuditReason != "current user console access" {
		t.Fatalf("expected current user audit reason, got %q", check.AuditReason)
	}
}

func TestServerWithAuthzGatesRuntimeClaim(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	tenantID := uuid.MustParse(auth.DefaultTenantID)
	taskID := uuid.MustParse(routeTaskID)
	taskService := &routeTaskService{
		tasks: []*task.Task{{
			ID:           taskID,
			TenantID:     tenantID,
			ProviderType: "codex",
		}},
	}
	authorizer := &routeAuthorizer{allowed: false}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(taskService),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, taskService, &routePoller{}),
		authService,
		&routeRuntimeAuthService{},
		authorizer,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Node-ID", "node-1")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected denied runtime claim to return 204, got %d: %s", resp.Code, resp.Body.String())
	}
	if taskService.assignedTaskID != uuid.Nil {
		t.Fatalf("expected denied runtime claim not to assign task, got %s", taskService.assignedTaskID)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one runtime authz check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Actor.Type != authz.ActorRuntimeNode || check.Actor.ID != "node-1" {
		t.Fatalf("expected runtime node actor, got %#v", check.Actor)
	}
	if check.Action != authz.ActionTaskClaim {
		t.Fatalf("expected task claim action, got %q", check.Action)
	}
	if check.Resource.Type != authz.ResourceTask || check.Resource.ID != taskID.String() {
		t.Fatalf("expected task resource %s, got %#v", taskID, check.Resource)
	}
	if check.TenantID != tenantID {
		t.Fatalf("expected tenant %s, got %s", tenantID, check.TenantID)
	}
}

func TestAuthUserManagementRoutesAreRegistered(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	server := NewServerWithAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
	)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	server.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	cookie := loginResp.Result().Cookies()[0]

	createReq := httptest.NewRequest(http.MethodPost, "/api/auth/users", strings.NewReader(`{"username":"operator","password":"secret"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create user to succeed, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var createBody struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createBody); err != nil {
		t.Fatalf("decode created user response: %v", err)
	}
	operatorID, err := uuid.Parse(createBody.User.ID)
	if err != nil {
		t.Fatalf("expected created user ID to be UUID, got %q: %v", createBody.User.ID, err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/auth/users?limit=10&offset=0", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list users to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listBody struct {
		Items []struct {
			Username string `json:"username"`
			Status   string `json:"status"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list users response: %v", err)
	}
	if len(listBody.Items) != 2 {
		t.Fatalf("expected two users, got %#v", listBody.Items)
	}

	statusReq := httptest.NewRequest(http.MethodPatch, "/api/auth/users/"+operatorID.String()+"/status", strings.NewReader(`{"status":"disabled"}`))
	statusReq.Header.Set("Content-Type", "application/json")
	statusReq.AddCookie(cookie)
	statusResp := httptest.NewRecorder()
	server.ServeHTTP(statusResp, statusReq)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("expected status update to succeed, got %d: %s", statusResp.Code, statusResp.Body.String())
	}

	resetReq := httptest.NewRequest(http.MethodPost, "/api/auth/users/"+operatorID.String()+"/reset-password", strings.NewReader(`{"password":"new-secret"}`))
	resetReq.Header.Set("Content-Type", "application/json")
	resetReq.AddCookie(cookie)
	resetResp := httptest.NewRecorder()
	server.ServeHTTP(resetResp, resetReq)
	if resetResp.Code != http.StatusOK {
		t.Fatalf("expected password reset to succeed, got %d: %s", resetResp.Code, resetResp.Body.String())
	}

	if len(authRepo.operationLogs) != 3 {
		t.Fatalf("expected three operation logs, got %#v", authRepo.operationLogs)
	}
	if authRepo.operationLogs[0].Action != auth.OperationActionUserCreate {
		t.Fatalf("expected first operation to create user, got %#v", authRepo.operationLogs[0])
	}
	if authRepo.operationLogs[1].Action != auth.OperationActionUserDisable {
		t.Fatalf("expected second operation to disable user, got %#v", authRepo.operationLogs[1])
	}
	if authRepo.operationLogs[2].Action != auth.OperationActionUserResetPassword {
		t.Fatalf("expected third operation to reset password, got %#v", authRepo.operationLogs[2])
	}
}

func TestAuthUserManagementRejectsUnauthenticatedRequests(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	server := NewServerWithAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/users", nil)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected users route to reject unauthenticated request, got %d", resp.Code)
	}
}

func TestLoginLogsRejectUnauthenticatedRequests(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	server := NewServerWithAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/login-logs", nil)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected login logs to reject unauthenticated request, got %d", resp.Code)
	}
}

func TestAuthzCenterOverviewRejectsUnauthenticatedRequests(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	repo := &routeAuthzCenterRepo{}
	service := authzcenter.NewService(repo, &routeAuthorizer{allowed: true})
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
		authzcenter.NewHandler(service, authService),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/authz/overview", nil)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected overview to reject unauthenticated request, got %d", resp.Code)
	}
}

func TestAuthzCenterOverviewAllowsAuthenticatedAdmin(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	repo := &routeAuthzCenterRepo{}
	service := authzcenter.NewService(repo, &routeAuthorizer{allowed: true})
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
		authzcenter.NewHandler(service, authService),
	)
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/authz/overview", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected overview to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	var body authzcenter.AuthzOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode overview response: %v", err)
	}
	if body.Engine.Engine != "db" {
		t.Fatalf("expected db engine, got %#v", body.Engine)
	}
	if repo.lastTenantID != uuid.MustParse(auth.DefaultTenantID) {
		t.Fatalf("expected overview repository calls to use default tenant, got %s", repo.lastTenantID)
	}
}

func TestAuthzCenterOverviewDeniedAuthorizationReturnsForbidden(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "viewer", "viewer"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	repo := &routeAuthzCenterRepo{}
	authorizer := &routeAuthorizer{allowed: false}
	service := authzcenter.NewService(repo, authorizer)
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
		authzcenter.NewHandler(service, authService),
	)
	cookie := routeLogin(t, server, "viewer", "viewer")

	req := httptest.NewRequest(http.MethodGet, "/api/authz/overview", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected denied read to return forbidden, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionAuthzCenterRead {
		t.Fatalf("expected one authz center read check, got %#v", authorizer.checks)
	}
}

func TestAuthzCenterRuntimeScopeCreateRecordsCheckAndOperationLog(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	repo := &routeAuthzCenterRepo{}
	authorizer := &routeAuthorizer{allowed: true}
	service := authzcenter.NewService(repo, authorizer)
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
		authzcenter.NewHandler(service, authService),
	)
	cookie := routeLogin(t, server, "admin", "admin")
	tenantID := uuid.MustParse(auth.DefaultTenantID)
	nodeID := uuid.MustParse("00000000-0000-0000-0000-000000000201")

	req := httptest.NewRequest(http.MethodPost, "/api/authz/runtime-scopes", strings.NewReader(`{
		"tenant_id":"`+tenantID.String()+`",
		"runtime_node_id":"`+nodeID.String()+`",
		"scope_type":"tenant",
		"scope_value":"`+tenantID.String()+`"
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected runtime scope create to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authorization check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Action != authz.ActionRuntimeScopeManage {
		t.Fatalf("expected runtime scope manage check, got %q", check.Action)
	}
	if check.Actor.ID != user.ID.String() || check.Resource.Type != authz.ResourceTenant || check.Resource.ID != tenantID.String() {
		t.Fatalf("unexpected authorization check: %#v", check)
	}
	if len(repo.operationLogs) != 1 {
		t.Fatalf("expected one operation log, got %#v", repo.operationLogs)
	}
	if repo.operationLogs[0].Action != authzcenter.OperationActionRuntimeScopeCreate {
		t.Fatalf("expected runtime scope create operation, got %#v", repo.operationLogs[0])
	}
}

func TestRuntimeRoutesUseAuthenticatedNodeIdentity(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/heartbeat", strings.NewReader(`{"current_load":2}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-ID", "node-1")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected authenticated heartbeat to reach handler, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode heartbeat response: %v", err)
	}
	if body["node_id"] != "node-1" {
		t.Fatalf("expected node_id from auth context, got %#v", body["node_id"])
	}
}

func TestRuntimeRoutesRejectMissingRuntimeAuth(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/heartbeat", strings.NewReader(`{"current_load":2}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing runtime auth to be rejected, got %d", rr.Code)
	}
}

func TestRuntimeRegisterRejectsMismatchedAuthenticatedNodeIdentity(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/register", strings.NewReader(`{"node_id":"node-2","name":"node 2","supported_providers":["codex"],"max_slots":1}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-ID", "node-1")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected mismatched runtime node identity to be rejected, got %d: %s", rr.Code, rr.Body.String())
	}
}

type routeRuntimeService struct{}

func (s *routeRuntimeService) RegisterNode(ctx context.Context, req runtime.RegisterNodeRequest) (*runtime.Node, error) {
	return &runtime.Node{NodeID: req.NodeID, Name: req.Name, SupportedProviders: req.SupportedProviders, MaxSlots: req.MaxSlots}, nil
}

func (s *routeRuntimeService) UpdateHeartbeat(ctx context.Context, req runtime.UpdateHeartbeatRequest) (*runtime.Node, error) {
	return &runtime.Node{NodeID: req.NodeID, CurrentLoad: req.CurrentLoad}, nil
}

func (s *routeRuntimeService) GetNode(ctx context.Context, nodeID string) (*runtime.Node, error) {
	return &runtime.Node{NodeID: nodeID, SupportedProviders: []string{"codex"}}, nil
}

func (s *routeRuntimeService) ListNodes(ctx context.Context, filter runtime.ListNodesFilter) ([]*runtime.Node, error) {
	return []*runtime.Node{}, nil
}

type routeTaskService struct {
	tasks          []*task.Task
	assignedTaskID uuid.UUID
}

func (s *routeTaskService) CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error) {
	return &task.Task{ID: uuid.New(), Title: req.Title, ProviderType: req.ProviderType}, nil
}

func (s *routeTaskService) GetTask(ctx context.Context, taskID uuid.UUID) (*task.Task, error) {
	return &task.Task{ID: taskID, ProviderType: "codex"}, nil
}

func (s *routeTaskService) ListTasks(ctx context.Context, filter task.ListTasksFilter) ([]*task.Task, error) {
	if s.tasks == nil {
		return []*task.Task{}, nil
	}
	tasks := make([]*task.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if filter.ProviderType != nil && t.ProviderType != *filter.ProviderType {
			continue
		}
		if filter.Status != nil && t.Status != "" && t.Status != *filter.Status {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *routeTaskService) AppendTaskEvent(ctx context.Context, req task.AppendTaskEventRequest) (*task.TaskEvent, error) {
	return &task.TaskEvent{TaskID: req.TaskID, EventType: req.EventType, Payload: req.Payload}, nil
}

func (s *routeTaskService) UpdateTaskStatus(ctx context.Context, req task.UpdateTaskStatusRequest) (*task.Task, error) {
	return &task.Task{ID: req.TaskID, Status: req.NewStatus}, nil
}

func (s *routeTaskService) CancelTask(ctx context.Context, taskID uuid.UUID, cancelledBy *string, reason *string) (*task.Task, error) {
	return &task.Task{ID: taskID, Status: task.TaskStatusCancelled}, nil
}

func (s *routeTaskService) AssignTask(ctx context.Context, req task.AssignTaskRequest) (*task.Task, error) {
	s.assignedTaskID = req.TaskID
	return &task.Task{ID: req.TaskID, AssignedNodeID: &req.AssignedNodeID}, nil
}

type routePoller struct{}

func (p *routePoller) WaitForTask(ctx context.Context, nodeID string) (*task.Task, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type routeRuntimeAuthService struct{}

func (s *routeRuntimeAuthService) ValidateRuntimeToken(ctx context.Context, nodeID, token string) error {
	if nodeID != "node-1" || token != "test-token" {
		return context.Canceled
	}
	return nil
}

type routeAuthRepo struct {
	users         map[string]*auth.User
	usersByID     map[uuid.UUID]*auth.User
	sessions      map[string]*auth.Session
	loginLogs     []auth.LoginLog
	operationLogs []auth.CreateOperationLogParams
}

func newRouteAuthRepo() *routeAuthRepo {
	return &routeAuthRepo{
		users:         map[string]*auth.User{},
		usersByID:     map[uuid.UUID]*auth.User{},
		sessions:      map[string]*auth.Session{},
		loginLogs:     []auth.LoginLog{},
		operationLogs: []auth.CreateOperationLogParams{},
	}
}

func (r *routeAuthRepo) CreateUser(ctx context.Context, username, passwordHash string) (*auth.User, error) {
	user := &auth.User{ID: uuid.New(), Username: username, PasswordHash: passwordHash, Status: "active"}
	r.users[username] = user
	r.usersByID[user.ID] = user
	return user, nil
}

func (r *routeAuthRepo) GetUserByUsername(ctx context.Context, username string) (*auth.User, error) {
	user, ok := r.users[username]
	if !ok {
		return nil, auth.ErrInvalidCredentials
	}
	return user, nil
}

func (r *routeAuthRepo) ListUsers(ctx context.Context, filter auth.ListUsersFilter) ([]*auth.User, error) {
	users := make([]*auth.User, 0, len(r.usersByID))
	for _, user := range r.usersByID {
		if filter.Status != "" && user.Status != filter.Status {
			continue
		}
		users = append(users, user)
	}
	return users, nil
}

func (r *routeAuthRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*auth.User, error) {
	user, ok := r.usersByID[id]
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	return user, nil
}

func (r *routeAuthRepo) UpdateUserStatus(ctx context.Context, userID uuid.UUID, status string) (*auth.User, error) {
	user, ok := r.usersByID[userID]
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	user.Status = status
	return user, nil
}

func (r *routeAuthRepo) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (*auth.User, error) {
	user, ok := r.usersByID[userID]
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	user.PasswordHash = passwordHash
	return user, nil
}

func (r *routeAuthRepo) CreateRuntimeToken(ctx context.Context, nodeID, tokenHash string, expiresAt time.Time) error {
	return nil
}

func (r *routeAuthRepo) GetRuntimeTokenByNodeID(ctx context.Context, nodeID string) (*auth.RuntimeToken, error) {
	return nil, auth.ErrInvalidToken
}

func (r *routeAuthRepo) CreateSession(ctx context.Context, session *auth.Session, tokenHash string) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	r.sessions[tokenHash] = session
	return nil
}

func (r *routeAuthRepo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, error) {
	session, ok := r.sessions[tokenHash]
	if !ok {
		return nil, auth.ErrSessionNotFound
	}
	return session, nil
}

func (r *routeAuthRepo) DeleteSession(ctx context.Context, tokenHash string) error {
	delete(r.sessions, tokenHash)
	return nil
}

func (r *routeAuthRepo) UpdateSessionLastSeen(ctx context.Context, tokenHash string, lastSeenAt time.Time) error {
	session, ok := r.sessions[tokenHash]
	if !ok {
		return auth.ErrSessionNotFound
	}
	session.LastSeenAt = lastSeenAt
	return nil
}

func (r *routeAuthRepo) CreateLoginLog(ctx context.Context, params auth.CreateLoginLogParams) error {
	now := time.Now().UTC()
	r.loginLogs = append([]auth.LoginLog{
		{
			ID:            uuid.New(),
			EventType:     params.EventType,
			UserID:        params.UserID,
			Username:      params.Username,
			SessionID:     params.SessionID,
			ClientIP:      params.ClientIP,
			UserAgent:     params.UserAgent,
			Result:        params.Result,
			FailureReason: params.FailureReason,
			CreatedAt:     now,
		},
	}, r.loginLogs...)
	return nil
}

func (r *routeAuthRepo) ListLoginLogs(ctx context.Context, filter auth.ListLoginLogsFilter) ([]auth.LoginLog, error) {
	start := int(filter.Offset)
	if start >= len(r.loginLogs) {
		return []auth.LoginLog{}, nil
	}
	end := start + int(filter.Limit)
	if end > len(r.loginLogs) {
		end = len(r.loginLogs)
	}
	return append([]auth.LoginLog{}, r.loginLogs[start:end]...), nil
}

func (r *routeAuthRepo) CreateOperationLog(ctx context.Context, params auth.CreateOperationLogParams) error {
	r.operationLogs = append(r.operationLogs, params)
	return nil
}

func routeLogin(t *testing.T, server *Server, username, password string) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	cookies := resp.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie")
	}
	return cookies[0]
}

type routeAuthorizer struct {
	allowed bool
	checks  []authz.CheckRequest
}

func (a *routeAuthorizer) Check(ctx context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	if a.allowed {
		return authz.Decision{Allowed: true, Reason: authz.ReasonAllowed, MatchedRule: "test.allow"}, nil
	}
	return authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership, RequiresAudit: true}, nil
}

type routeAuthzCenterRepo struct {
	operationLogs []authzcenter.OperationLogInput
	lastTenantID  uuid.UUID
}

func (r *routeAuthzCenterRepo) CountDecisionsSince(ctx context.Context, tenantID uuid.UUID, since time.Time) (authzcenter.DecisionTotals, error) {
	r.lastTenantID = tenantID
	return authzcenter.DecisionTotals{}, nil
}

func (r *routeAuthzCenterRepo) ListTopDeniedActionsSince(ctx context.Context, tenantID uuid.UUID, since time.Time, limit int32) ([]authzcenter.ActionCount, error) {
	r.lastTenantID = tenantID
	return []authzcenter.ActionCount{}, nil
}

func (r *routeAuthzCenterRepo) ListDecisions(ctx context.Context, filter authzcenter.DecisionFilter) ([]authzcenter.DecisionRecord, error) {
	return []authzcenter.DecisionRecord{}, nil
}

func (r *routeAuthzCenterRepo) ListRuntimeScopeNodes(ctx context.Context, tenantID uuid.UUID) ([]authzcenter.RuntimeScopeNodeRecord, error) {
	r.lastTenantID = tenantID
	return []authzcenter.RuntimeScopeNodeRecord{}, nil
}

func (r *routeAuthzCenterRepo) CreateRuntimeScope(ctx context.Context, input authzcenter.RuntimeScopeInput) (authzcenter.RuntimeScopeRecord, error) {
	now := time.Now().UTC()
	return authzcenter.RuntimeScopeRecord{
		ID:            uuid.New(),
		TenantID:      input.TenantID,
		RuntimeNodeID: input.RuntimeNodeID,
		TeamID:        input.TeamID,
		ScopeType:     authzcenter.RuntimeScopeScopeType(input.ScopeType),
		ScopeValue:    input.ScopeValue,
		Status:        authzcenter.RuntimeScopeStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (r *routeAuthzCenterRepo) UpdateRuntimeScopeStatus(ctx context.Context, tenantID uuid.UUID, scopeID uuid.UUID, status string) (authzcenter.RuntimeScopeRecord, error) {
	now := time.Now().UTC()
	return authzcenter.RuntimeScopeRecord{
		ID:        scopeID,
		TenantID:  tenantID,
		Status:    authzcenter.RuntimeScopeStatus(status),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (r *routeAuthzCenterRepo) ListMembers(ctx context.Context, filter authzcenter.MemberFilter) ([]authzcenter.MemberRecord, error) {
	return []authzcenter.MemberRecord{}, nil
}

func (r *routeAuthzCenterRepo) RecordOperationLog(ctx context.Context, input authzcenter.OperationLogInput) error {
	r.operationLogs = append(r.operationLogs, input)
	return nil
}
