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
	"github.com/superteam/control-plane/internal/audit"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/tenant"
)

func TestTeamRoutesUseConsoleTenant(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	ownerID := uuid.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"slug":"platform","name":"Platform","human_owner_user_id":"`+ownerID.String()+`","metadata":{"cost_center":"r-and-d"}}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create team to succeed, got %d: %s", createResp.Code, createResp.Body.String())
	}
	if service.createReq.TenantID != expectedTenantID {
		t.Fatalf("expected create tenant %s, got %s", expectedTenantID, service.createReq.TenantID)
	}
	if service.createReq.HumanOwnerUserID == nil || *service.createReq.HumanOwnerUserID != ownerID {
		t.Fatalf("expected request human owner %s, got %#v", ownerID, service.createReq.HumanOwnerUserID)
	}
	var created struct {
		ID               string         `json:"id"`
		TenantID         string         `json:"tenant_id"`
		HumanOwnerUserID string         `json:"human_owner_user_id"`
		Metadata         map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created team: %v", err)
	}
	if created.TenantID != expectedTenantID.String() || created.HumanOwnerUserID != ownerID.String() {
		t.Fatalf("expected response tenant/owner %s/%s, got %#v", expectedTenantID, ownerID, created)
	}
	if created.Metadata["cost_center"] != "r-and-d" {
		t.Fatalf("expected metadata in response, got %#v", created.Metadata)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams?status=active&q=ops", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list teams to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listReq.TenantID != expectedTenantID {
		t.Fatalf("expected list tenant %s, got %s", expectedTenantID, service.listReq.TenantID)
	}
	if service.listReq.Status != tenant.TeamStatusActive || service.listReq.Q != "ops" {
		t.Fatalf("expected list filters active/ops, got %#v", service.listReq)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.ID, nil)
	getReq.AddCookie(cookie)
	getResp := httptest.NewRecorder()
	server.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected get team to succeed, got %d: %s", getResp.Code, getResp.Body.String())
	}
	if service.getTenantID != expectedTenantID {
		t.Fatalf("expected get tenant %s, got %s", expectedTenantID, service.getTenantID)
	}

	overviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.ID+"/overview", nil)
	overviewReq.AddCookie(cookie)
	overviewResp := httptest.NewRecorder()
	server.ServeHTTP(overviewResp, overviewReq)
	if overviewResp.Code != http.StatusOK {
		t.Fatalf("expected overview to succeed, got %d: %s", overviewResp.Code, overviewResp.Body.String())
	}
	if service.overviewTenantID != expectedTenantID || service.overviewTeamID.String() != created.ID {
		t.Fatalf("expected overview tenant/team %s/%s, got %s/%s", expectedTenantID, created.ID, service.overviewTenantID, service.overviewTeamID)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID, strings.NewReader(`{"slug":"platform-sre","name":"Platform SRE","human_owner_user_id":"`+ownerID.String()+`","metadata":{"cost_center":"ops"}}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(cookie)
	updateResp := httptest.NewRecorder()
	server.ServeHTTP(updateResp, updateReq)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update team to succeed, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	if service.updateReq.TenantID != expectedTenantID || service.updateReq.TeamID.String() != created.ID || service.updateReq.Name != "Platform SRE" {
		t.Fatalf("expected update request for tenant/team/name, got %#v", service.updateReq)
	}

	for _, tt := range []struct {
		name   string
		path   string
		status tenant.TeamStatus
	}{
		{name: "disable", path: "/disable", status: tenant.TeamStatusDisabled},
		{name: "archive", path: "/archive", status: tenant.TeamStatusArchived},
		{name: "restore", path: "/restore", status: tenant.TeamStatusActive},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+tt.path, nil)
			req.AddCookie(cookie)
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("expected %s to succeed, got %d: %s", tt.name, resp.Code, resp.Body.String())
			}
			if service.changeStatusReq.TenantID != expectedTenantID || service.changeStatusReq.TeamID.String() != created.ID || service.changeStatusReq.Status != tt.status {
				t.Fatalf("expected %s status request %#v, got %#v", tt.name, tt.status, service.changeStatusReq)
			}
		})
	}

	clientApprovedBy := uuid.New()
	revisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/config-revisions", strings.NewReader(`{"human_owner_user_id":"`+ownerID.String()+`","approved_by":"`+clientApprovedBy.String()+`","constitution":{"principle":"review"}}`))
	revisionReq.Header.Set("Content-Type", "application/json")
	revisionReq.AddCookie(cookie)
	revisionResp := httptest.NewRecorder()
	server.ServeHTTP(revisionResp, revisionReq)
	if revisionResp.Code != http.StatusCreated {
		t.Fatalf("expected create config revision to succeed, got %d: %s", revisionResp.Code, revisionResp.Body.String())
	}
	if service.createRevisionReq.TenantID != expectedTenantID || service.createRevisionReq.TeamID.String() != created.ID {
		t.Fatalf("expected revision tenant/team %s/%s, got %s/%s", expectedTenantID, created.ID, service.createRevisionReq.TenantID, service.createRevisionReq.TeamID)
	}
	if service.createRevisionReq.HumanOwnerUserID == nil || *service.createRevisionReq.HumanOwnerUserID != ownerID {
		t.Fatalf("expected revision human owner %s, got %#v", ownerID, service.createRevisionReq.HumanOwnerUserID)
	}
	if service.createRevisionReq.ApprovedBy == nil || *service.createRevisionReq.ApprovedBy != user.ID {
		t.Fatalf("expected revision approved_by to use current console user %s, got %#v", user.ID, service.createRevisionReq.ApprovedBy)
	}
	if *service.createRevisionReq.ApprovedBy == clientApprovedBy {
		t.Fatalf("expected handler to ignore client supplied approved_by %s", clientApprovedBy)
	}

	currentReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.ID+"/config-revisions/current", nil)
	currentReq.AddCookie(cookie)
	currentResp := httptest.NewRecorder()
	server.ServeHTTP(currentResp, currentReq)
	if currentResp.Code != http.StatusOK {
		t.Fatalf("expected current config revision to succeed, got %d: %s", currentResp.Code, currentResp.Body.String())
	}
	if service.currentTenantID != expectedTenantID || service.currentTeamID.String() != created.ID {
		t.Fatalf("expected current revision tenant/team %s/%s, got %s/%s", expectedTenantID, created.ID, service.currentTenantID, service.currentTeamID)
	}
}

func TestTeamRoutesRequireConsoleAuth(t *testing.T) {
	service := &routeTeamService{}
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
	)
	server.SetTenantHandler(tenant.NewHandler(service))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated team route to return 401, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.called() {
		t.Fatalf("expected unauthenticated request not to call tenant service")
	}
}

func TestTeamRoutesRejectInvalidListPagination(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	tests := []string{
		"/api/v1/teams?limit=bad",
		"/api/v1/teams?offset=bad",
		"/api/v1/teams?limit=-1",
		"/api/v1/teams?offset=-1",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.AddCookie(cookie)
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected invalid pagination to return 400, got %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
	if service.listCalled {
		t.Fatalf("expected invalid pagination not to call tenant service")
	}
}

func TestTeamRoutesRequireManagementAuthorization(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{}
	authorizer := &routeAuthorizer{allowed: false}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	teamID := uuid.New().String()
	ownerID := uuid.New().String()

	tests := []struct {
		name         string
		method       string
		path         string
		body         string
		action       string
		resourceType string
		resourceID   string
		teamID       *uuid.UUID
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/teams", action: authz.ActionTeamRead, resourceType: authz.ResourceTenant},
		{name: "create", method: http.MethodPost, path: "/api/v1/teams", body: `{"slug":"platform","name":"Platform","human_owner_user_id":"` + ownerID + `"}`, action: authz.ActionTeamCreate, resourceType: authz.ResourceTenant},
		{name: "get", method: http.MethodGet, path: "/api/v1/teams/" + teamID, action: authz.ActionTeamRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "overview", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/overview", action: authz.ActionTeamRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "update", method: http.MethodPatch, path: "/api/v1/teams/" + teamID, body: `{"slug":"platform","name":"Platform"}`, action: authz.ActionTeamUpdate, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "disable", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/disable", action: authz.ActionTeamDisable, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "archive", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/archive", action: authz.ActionTeamArchive, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "restore", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/restore", action: authz.ActionTeamRestore, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "create config revision", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/config-revisions", body: `{"human_owner_user_id":"` + ownerID + `"}`, action: authz.ActionTeamGovernanceApprove, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "current config revision", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/config-revisions/current", action: authz.ActionTeamGovernanceRead, resourceType: authz.ResourceTeam, resourceID: teamID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookie)
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			if resp.Code != http.StatusForbidden {
				t.Fatalf("expected forbidden team route, got %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
	if service.called() {
		t.Fatalf("expected denied requests not to call tenant service")
	}
	if len(authorizer.checks) != len(tests) {
		t.Fatalf("expected one authorization check per request, got %#v", authorizer.checks)
	}
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	for i, check := range authorizer.checks {
		expected := tests[i]
		expectedResourceID := expected.resourceID
		if expectedResourceID == "" {
			expectedResourceID = expectedTenantID.String()
		}
		if check.Action != expected.action {
			t.Fatalf("expected action %s for %s, got %#v", expected.action, expected.name, check)
		}
		if check.Actor.Type != authz.ActorUser {
			t.Fatalf("expected user actor, got %#v", check)
		}
		if check.Resource.Type != expected.resourceType || check.Resource.ID != expectedResourceID || check.TenantID != expectedTenantID {
			t.Fatalf("expected resource %s/%s for %s, got %#v", expected.resourceType, expectedResourceID, expected.name, check)
		}
		if expected.resourceType == authz.ResourceTeam {
			expectedTeamID := uuid.MustParse(expected.resourceID)
			if check.TeamID == nil || *check.TeamID != expectedTeamID {
				t.Fatalf("expected team context %s for %s, got %#v", expectedTeamID, expected.name, check)
			}
		} else if check.TeamID != nil {
			t.Fatalf("expected no team context for %s, got %#v", expected.name, check)
		}
	}
}

func TestTeamConfigRevisionDraftUsesGovernanceEditAuthorization(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{}
	authorizer := &routeAuthorizer{allowed: true}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	teamID := uuid.New()
	ownerID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+teamID.String()+"/config-revisions", strings.NewReader(`{"human_owner_user_id":"`+ownerID.String()+`","status":"draft"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected draft config revision to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) == 0 {
		t.Fatalf("expected authorization check")
	}
	check := authorizer.checks[len(authorizer.checks)-1]
	if check.Action != authz.ActionTeamGovernanceEdit {
		t.Fatalf("expected governance edit action, got %#v", check)
	}
	if check.Resource.Type != authz.ResourceTeam || check.Resource.ID != teamID.String() || check.TeamID == nil || *check.TeamID != teamID {
		t.Fatalf("expected team resource %s, got %#v", teamID, check)
	}
}

func TestTeamAuditRouteUsesTeamAuditRead(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{}
	authorizer := &routeAuthorizer{allowed: true}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	teamID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+teamID.String()+"/audit", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected team audit route to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) == 0 {
		t.Fatalf("expected authorization check for team audit route, got status %d: %s", resp.Code, resp.Body.String())
	}
	check := authorizer.checks[len(authorizer.checks)-1]
	if check.Action != authz.ActionTeamAuditRead {
		t.Fatalf("expected team audit read action, got %#v", check)
	}
	if check.Resource.Type != authz.ResourceTeam || check.Resource.ID != teamID.String() || check.TeamID == nil || *check.TeamID != teamID {
		t.Fatalf("expected team resource %s, got %#v", teamID, check)
	}
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	if service.auditTenantID != expectedTenantID || service.auditTeamID != teamID {
		t.Fatalf("expected service tenant/team %s/%s, got %s/%s", expectedTenantID, teamID, service.auditTenantID, service.auditTeamID)
	}
}

func TestTeamOverviewAllowedActionsFilterDeniedDecisions(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{}
	authorizer := &routeAuthorizer{
		allowed: true,
		denyActions: map[string]bool{
			authz.ActionTeamArchive: true,
		},
	}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	teamID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+teamID.String()+"/overview", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected overview to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	var body struct {
		AllowedActions []string `json:"allowed_actions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if !containsString(body.AllowedActions, authz.ActionTeamUpdate) {
		t.Fatalf("expected allowed team update action, got %#v", body.AllowedActions)
	}
	if containsString(body.AllowedActions, authz.ActionTeamArchive) {
		t.Fatalf("expected denied archive action to be filtered, got %#v", body.AllowedActions)
	}
}

func TestTeamRoutesSanitizeInternalErrors(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{listErr: errors.New("database password leaked")}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected internal service error to return 500, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "internal server error") {
		t.Fatalf("expected generic internal server error body, got %q", resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "database password leaked") {
		t.Fatalf("expected internal service details to be hidden, got %q", resp.Body.String())
	}
}

func TestTeamRoutesSanitizeAuthorizationBackendErrors(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{err: errors.New("policy backend DSN leaked")},
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected authz backend error to return 500, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "internal server error") {
		t.Fatalf("expected generic internal server error body, got %q", resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "policy backend DSN leaked") {
		t.Fatalf("expected authz backend details to be hidden, got %q", resp.Body.String())
	}
	if service.called() {
		t.Fatalf("expected authz backend error not to call tenant service")
	}
}

func TestTeamRoutesRejectUnconfiguredAuthorizationBeforeService(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{}
	server := NewServerWithAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected unconfigured team authorization to return 403, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.called() {
		t.Fatalf("expected unconfigured authorization not to call tenant service")
	}
}

func TestTeamRouteRejectsUnconfiguredService(t *testing.T) {
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
	server.SetTenantHandler(tenant.NewHandler(nil))
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured tenant service to return 503, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestTeamRoutesDoNotSubstituteConsoleUserAsHumanOwner(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeTeamService{rejectMissingOwner: true}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetTenantHandler(tenant.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"slug":"platform","name":"Platform"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusBadRequest {
		t.Fatalf("expected missing team owner to fail through service validation, got %d: %s", createResp.Code, createResp.Body.String())
	}
	if service.createReq.HumanOwnerUserID != nil {
		t.Fatalf("expected handler not to substitute console user %s as team owner, got %#v", user.ID, service.createReq.HumanOwnerUserID)
	}

	teamID := uuid.New()
	revisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+teamID.String()+"/config-revisions", strings.NewReader(`{"constitution":{}}`))
	revisionReq.Header.Set("Content-Type", "application/json")
	revisionReq.AddCookie(cookie)
	revisionResp := httptest.NewRecorder()
	server.ServeHTTP(revisionResp, revisionReq)
	if revisionResp.Code != http.StatusBadRequest {
		t.Fatalf("expected missing revision owner to fail through service validation, got %d: %s", revisionResp.Code, revisionResp.Body.String())
	}
	if service.createRevisionReq.HumanOwnerUserID != nil {
		t.Fatalf("expected handler not to substitute console user %s as revision owner, got %#v", user.ID, service.createRevisionReq.HumanOwnerUserID)
	}
}

type routeTeamService struct {
	createReq            tenant.CreateTeamRequest
	listReq              tenant.ListTeamsRequest
	updateReq            tenant.UpdateTeamRequest
	changeStatusReq      tenant.ChangeTeamStatusRequest
	createRevisionReq    tenant.CreateTeamConfigRevisionRequest
	getTenantID          uuid.UUID
	getTeamID            uuid.UUID
	overviewTenantID     uuid.UUID
	overviewTeamID       uuid.UUID
	currentTenantID      uuid.UUID
	currentTeamID        uuid.UUID
	auditTenantID        uuid.UUID
	auditTeamID          uuid.UUID
	auditLimit           int32
	auditOffset          int32
	createCalled         bool
	listCalled           bool
	getCalled            bool
	overviewCalled       bool
	updateCalled         bool
	changeStatusCalled   bool
	createRevisionCalled bool
	currentCalled        bool
	auditCalled          bool
	createdID            uuid.UUID
	rejectMissingOwner   bool
	listErr              error
}

func (s *routeTeamService) CreateTeam(ctx context.Context, req tenant.CreateTeamRequest) (*tenant.Team, error) {
	s.createCalled = true
	s.createReq = req
	if s.rejectMissingOwner && req.HumanOwnerUserID == nil {
		return nil, tenant.ErrInvalidInput
	}
	s.createdID = uuid.New()
	now := time.Now().UTC()
	status := req.Status
	if status == "" {
		status = tenant.TeamStatusActive
	}
	return &tenant.Team{
		ID:               s.createdID,
		TenantID:         req.TenantID,
		Slug:             req.Slug,
		Name:             req.Name,
		Status:           status,
		HumanOwnerUserID: req.HumanOwnerUserID,
		Metadata:         req.Metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeTeamService) ListTeamSummaries(ctx context.Context, req tenant.ListTeamsRequest) ([]*tenant.TeamListItem, error) {
	s.listCalled = true
	s.listReq = req
	if s.listErr != nil {
		return nil, s.listErr
	}
	return []*tenant.TeamListItem{}, nil
}

func (s *routeTeamService) GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (*tenant.Team, error) {
	s.getCalled = true
	s.getTenantID = tenantID
	s.getTeamID = teamID
	now := time.Now().UTC()
	return &tenant.Team{
		ID:               teamID,
		TenantID:         tenantID,
		Slug:             "platform",
		Name:             "Platform",
		Status:           tenant.TeamStatusActive,
		HumanOwnerUserID: &uuid.UUID{},
		Metadata:         map[string]any{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeTeamService) GetOverview(ctx context.Context, tenantID, teamID uuid.UUID) (*tenant.TeamOverview, error) {
	s.overviewCalled = true
	s.overviewTenantID = tenantID
	s.overviewTeamID = teamID
	now := time.Now().UTC()
	return &tenant.TeamOverview{
		Team: &tenant.Team{
			ID:        teamID,
			TenantID:  tenantID,
			Slug:      "platform",
			Name:      "Platform",
			Status:    tenant.TeamStatusActive,
			Metadata:  map[string]any{},
			CreatedAt: now,
			UpdatedAt: now,
		},
		MemberCount:      3,
		CapabilityCount:  2,
		PendingItemCount: 1,
		AllowedActions: []tenant.AllowedTeamAction{
			tenant.AllowedTeamAction(authz.ActionTeamUpdate),
			tenant.AllowedTeamAction(authz.ActionTeamDisable),
		},
	}, nil
}

func (s *routeTeamService) UpdateTeam(ctx context.Context, req tenant.UpdateTeamRequest) (*tenant.Team, error) {
	s.updateCalled = true
	s.updateReq = req
	now := time.Now().UTC()
	return &tenant.Team{
		ID:               req.TeamID,
		TenantID:         req.TenantID,
		Slug:             req.Slug,
		Name:             req.Name,
		Status:           tenant.TeamStatusActive,
		HumanOwnerUserID: req.HumanOwnerUserID,
		Metadata:         req.Metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeTeamService) ChangeTeamStatus(ctx context.Context, req tenant.ChangeTeamStatusRequest) (*tenant.Team, error) {
	s.changeStatusCalled = true
	s.changeStatusReq = req
	now := time.Now().UTC()
	return &tenant.Team{
		ID:        req.TeamID,
		TenantID:  req.TenantID,
		Slug:      "platform",
		Name:      "Platform",
		Status:    req.Status,
		Metadata:  map[string]any{},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *routeTeamService) CreateConfigRevision(ctx context.Context, req tenant.CreateTeamConfigRevisionRequest) (*tenant.TeamConfigRevision, error) {
	s.createRevisionCalled = true
	s.createRevisionReq = req
	if s.rejectMissingOwner && req.HumanOwnerUserID == nil {
		return nil, tenant.ErrInvalidInput
	}
	now := time.Now().UTC()
	status := req.Status
	if status == "" {
		status = tenant.TeamConfigRevisionStatusActive
	}
	return &tenant.TeamConfigRevision{
		ID:                          uuid.New(),
		TenantID:                    req.TenantID,
		TeamID:                      req.TeamID,
		RevisionNumber:              1,
		Constitution:                req.Constitution,
		CapabilityPolicy:            map[string]any{},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		HumanOwnerUserID:            req.HumanOwnerUserID,
		Status:                      status,
		ApprovedBy:                  req.ApprovedBy,
		ApprovedAt:                  &now,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}, nil
}

func (s *routeTeamService) GetCurrentConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (*tenant.TeamConfigRevision, error) {
	s.currentCalled = true
	s.currentTenantID = tenantID
	s.currentTeamID = teamID
	now := time.Now().UTC()
	return &tenant.TeamConfigRevision{
		ID:                          uuid.New(),
		TenantID:                    tenantID,
		TeamID:                      teamID,
		RevisionNumber:              1,
		Constitution:                map[string]any{},
		CapabilityPolicy:            map[string]any{},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		Status:                      tenant.TeamConfigRevisionStatusActive,
		ApprovedAt:                  &now,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}, nil
}

func (s *routeTeamService) ListTeamAuditEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*audit.Event, error) {
	s.auditCalled = true
	s.auditTenantID = tenantID
	s.auditTeamID = teamID
	s.auditLimit = limit
	s.auditOffset = offset
	return []*audit.Event{}, nil
}

func (s *routeTeamService) called() bool {
	return s.createCalled ||
		s.listCalled ||
		s.getCalled ||
		s.overviewCalled ||
		s.updateCalled ||
		s.changeStatusCalled ||
		s.createRevisionCalled ||
		s.currentCalled ||
		s.auditCalled
}

var _ tenant.HandlerService = (*routeTeamService)(nil)
