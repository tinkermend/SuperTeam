package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	memberID := uuid.New()
	viewerID := uuid.New()

	createBody := `{
		"slug":"platform",
		"name":"Platform",
		"human_owner_user_id":"` + ownerID.String() + `",
		"initial_members":[
			{"user_id":"` + memberID.String() + `","role":"member"},
			{"user_id":"` + viewerID.String() + `","role":"viewer"}
		],
		"metadata":{"cost_center":"r-and-d"}
	}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(createBody))
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
	if service.createReq.ActorUserID != user.ID {
		t.Fatalf("expected actor user %s, got %s", user.ID, service.createReq.ActorUserID)
	}
	if !reflect.DeepEqual(service.createReq.InitialMembers, []tenant.InitialTeamMemberInput{
		{UserID: memberID, Role: tenant.TeamRoleMember},
		{UserID: viewerID, Role: tenant.TeamRoleViewer},
	}) {
		t.Fatalf("expected initial members in create request, got %#v", service.createReq.InitialMembers)
	}
	var created struct {
		Team struct {
			ID               string         `json:"id"`
			TenantID         string         `json:"tenant_id"`
			HumanOwnerUserID string         `json:"human_owner_user_id"`
			Metadata         map[string]any `json:"metadata"`
		} `json:"team"`
		AllowedActions []string `json:"allowed_actions"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created team: %v", err)
	}
	if created.Team.TenantID != expectedTenantID.String() || created.Team.HumanOwnerUserID != ownerID.String() {
		t.Fatalf("expected response tenant/owner %s/%s, got %#v", expectedTenantID, ownerID, created)
	}
	if created.Team.Metadata["cost_center"] != "r-and-d" {
		t.Fatalf("expected metadata in response, got %#v", created.Team.Metadata)
	}
	if len(created.AllowedActions) == 0 {
		t.Fatalf("expected create response to include allowed actions, got %#v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams?status=active&q=ops&governance_status=draft_pending", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list teams to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listReq.TenantID != expectedTenantID {
		t.Fatalf("expected list tenant %s, got %s", expectedTenantID, service.listReq.TenantID)
	}
	if service.listReq.Status != tenant.TeamStatusActive || service.listReq.Q != "ops" || service.listReq.GovernanceStatus != tenant.GovernanceSummaryDraftPending {
		t.Fatalf("expected list filters active/ops/draft_pending, got %#v", service.listReq)
	}
	var listed []struct {
		HumanOwner *struct {
			UserID      string `json:"user_id"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
			Status      string `json:"status"`
		} `json:"human_owner"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed teams: %v", err)
	}
	if len(listed) != 1 || listed[0].HumanOwner == nil || listed[0].HumanOwner.Username != "owner" || listed[0].HumanOwner.DisplayName != "Owner Person" || listed[0].HumanOwner.Email != "owner@example.com" {
		t.Fatalf("expected list response to include human owner summary, got %#v", listed)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.Team.ID, nil)
	getReq.AddCookie(cookie)
	getResp := httptest.NewRecorder()
	server.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected get team to succeed, got %d: %s", getResp.Code, getResp.Body.String())
	}
	if service.getTenantID != expectedTenantID {
		t.Fatalf("expected get tenant %s, got %s", expectedTenantID, service.getTenantID)
	}

	overviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.Team.ID+"/overview", nil)
	overviewReq.AddCookie(cookie)
	overviewResp := httptest.NewRecorder()
	server.ServeHTTP(overviewResp, overviewReq)
	if overviewResp.Code != http.StatusOK {
		t.Fatalf("expected overview to succeed, got %d: %s", overviewResp.Code, overviewResp.Body.String())
	}
	if service.overviewTenantID != expectedTenantID || service.overviewTeamID.String() != created.Team.ID {
		t.Fatalf("expected overview tenant/team %s/%s, got %s/%s", expectedTenantID, created.Team.ID, service.overviewTenantID, service.overviewTeamID)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.Team.ID, strings.NewReader(`{"slug":"platform-sre","name":"Platform SRE","human_owner_user_id":"`+ownerID.String()+`","metadata":{"cost_center":"ops"}}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(cookie)
	updateResp := httptest.NewRecorder()
	server.ServeHTTP(updateResp, updateReq)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update team to succeed, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	if service.updateReq.TenantID != expectedTenantID || service.updateReq.TeamID.String() != created.Team.ID || service.updateReq.Name != "Platform SRE" {
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.Team.ID+tt.path, nil)
			req.AddCookie(cookie)
			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("expected %s to succeed, got %d: %s", tt.name, resp.Code, resp.Body.String())
			}
			if service.changeStatusReq.TenantID != expectedTenantID || service.changeStatusReq.TeamID.String() != created.Team.ID || service.changeStatusReq.Status != tt.status {
				t.Fatalf("expected %s status request %#v, got %#v", tt.name, tt.status, service.changeStatusReq)
			}
		})
	}

	clientApprovedBy := uuid.New()
	revisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.Team.ID+"/config-revisions", strings.NewReader(`{"human_owner_user_id":"`+ownerID.String()+`","approved_by":"`+clientApprovedBy.String()+`","constitution":{"principle":"review"}}`))
	revisionReq.Header.Set("Content-Type", "application/json")
	revisionReq.AddCookie(cookie)
	revisionResp := httptest.NewRecorder()
	server.ServeHTTP(revisionResp, revisionReq)
	if revisionResp.Code != http.StatusCreated {
		t.Fatalf("expected create config revision to succeed, got %d: %s", revisionResp.Code, revisionResp.Body.String())
	}
	if service.createRevisionReq.TenantID != expectedTenantID || service.createRevisionReq.TeamID.String() != created.Team.ID {
		t.Fatalf("expected revision tenant/team %s/%s, got %s/%s", expectedTenantID, created.Team.ID, service.createRevisionReq.TenantID, service.createRevisionReq.TeamID)
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

	currentReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.Team.ID+"/config-revisions/current", nil)
	currentReq.AddCookie(cookie)
	currentResp := httptest.NewRecorder()
	server.ServeHTTP(currentResp, currentReq)
	if currentResp.Code != http.StatusOK {
		t.Fatalf("expected current config revision to succeed, got %d: %s", currentResp.Code, currentResp.Body.String())
	}
	if service.currentTenantID != expectedTenantID || service.currentTeamID.String() != created.Team.ID {
		t.Fatalf("expected current revision tenant/team %s/%s, got %s/%s", expectedTenantID, created.Team.ID, service.currentTenantID, service.currentTeamID)
	}

	governanceCurrentReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.Team.ID+"/governance/current", nil)
	governanceCurrentReq.AddCookie(cookie)
	governanceCurrentResp := httptest.NewRecorder()
	server.ServeHTTP(governanceCurrentResp, governanceCurrentReq)
	if governanceCurrentResp.Code != http.StatusOK {
		t.Fatalf("expected current governance revision to succeed, got %d: %s", governanceCurrentResp.Code, governanceCurrentResp.Body.String())
	}
	if service.currentTenantID != expectedTenantID || service.currentTeamID.String() != created.Team.ID {
		t.Fatalf("expected current governance tenant/team %s/%s, got %s/%s", expectedTenantID, created.Team.ID, service.currentTenantID, service.currentTeamID)
	}

	listDraftsReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.Team.ID+"/governance/drafts?limit=25&offset=5", nil)
	listDraftsReq.AddCookie(cookie)
	listDraftsResp := httptest.NewRecorder()
	server.ServeHTTP(listDraftsResp, listDraftsReq)
	if listDraftsResp.Code != http.StatusOK {
		t.Fatalf("expected list governance drafts to succeed, got %d: %s", listDraftsResp.Code, listDraftsResp.Body.String())
	}
	if service.listDraftsTenantID != expectedTenantID || service.listDraftsTeamID.String() != created.Team.ID || service.listDraftsLimit != 25 || service.listDraftsOffset != 5 {
		t.Fatalf("expected list drafts tenant/team/pagination %s/%s/25/5, got %s/%s/%d/%d", expectedTenantID, created.Team.ID, service.listDraftsTenantID, service.listDraftsTeamID, service.listDraftsLimit, service.listDraftsOffset)
	}

	createDraftReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.Team.ID+"/governance/drafts", strings.NewReader(`{"human_owner_user_id":"`+ownerID.String()+`","approved_by":"`+uuid.New().String()+`","constitution":{"hard_rules":["review before deploy"]}}`))
	createDraftReq.Header.Set("Content-Type", "application/json")
	createDraftReq.AddCookie(cookie)
	createDraftResp := httptest.NewRecorder()
	server.ServeHTTP(createDraftResp, createDraftReq)
	if createDraftResp.Code != http.StatusCreated {
		t.Fatalf("expected create governance draft to succeed, got %d: %s", createDraftResp.Code, createDraftResp.Body.String())
	}
	if service.createDraftReq.TenantID != expectedTenantID || service.createDraftReq.TeamID.String() != created.Team.ID {
		t.Fatalf("expected create draft tenant/team %s/%s, got %s/%s", expectedTenantID, created.Team.ID, service.createDraftReq.TenantID, service.createDraftReq.TeamID)
	}
	if service.createDraftReq.ApprovedBy != nil {
		t.Fatalf("expected create draft to ignore client approved_by, got %#v", service.createDraftReq.ApprovedBy)
	}

	draftID := uuid.New()
	updateDraftReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.Team.ID+"/governance/drafts/"+draftID.String(), strings.NewReader(`{"human_owner_user_id":"`+ownerID.String()+`","constitution":{"hard_rules":["keep audit trail"]},"capability_policy":{"bindings":["runtime:read"]}}`))
	updateDraftReq.Header.Set("Content-Type", "application/json")
	updateDraftReq.AddCookie(cookie)
	updateDraftResp := httptest.NewRecorder()
	server.ServeHTTP(updateDraftResp, updateDraftReq)
	if updateDraftResp.Code != http.StatusOK {
		t.Fatalf("expected update governance draft to succeed, got %d: %s", updateDraftResp.Code, updateDraftResp.Body.String())
	}
	if service.updateDraftTenantID != expectedTenantID || service.updateDraftTeamID.String() != created.Team.ID || service.updateDraftID != draftID {
		t.Fatalf("expected update draft tenant/team/draft %s/%s/%s, got %s/%s/%s", expectedTenantID, created.Team.ID, draftID, service.updateDraftTenantID, service.updateDraftTeamID, service.updateDraftID)
	}
	if service.updateDraftInput.HumanOwnerUserID == nil || *service.updateDraftInput.HumanOwnerUserID != ownerID {
		t.Fatalf("expected update draft human owner %s, got %#v", ownerID, service.updateDraftInput.HumanOwnerUserID)
	}

	approveDraftReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.Team.ID+"/governance/drafts/"+draftID.String()+"/approve", strings.NewReader(`{"approved_by":"`+uuid.New().String()+`"}`))
	approveDraftReq.Header.Set("Content-Type", "application/json")
	approveDraftReq.AddCookie(cookie)
	approveDraftResp := httptest.NewRecorder()
	server.ServeHTTP(approveDraftResp, approveDraftReq)
	if approveDraftResp.Code != http.StatusOK {
		t.Fatalf("expected approve governance draft to succeed, got %d: %s", approveDraftResp.Code, approveDraftResp.Body.String())
	}
	if service.approveDraftTenantID != expectedTenantID || service.approveDraftTeamID.String() != created.Team.ID || service.approveDraftID != draftID {
		t.Fatalf("expected approve draft tenant/team/draft %s/%s/%s, got %s/%s/%s", expectedTenantID, created.Team.ID, draftID, service.approveDraftTenantID, service.approveDraftTeamID, service.approveDraftID)
	}
	if service.approveDraftApprovedBy != user.ID {
		t.Fatalf("expected approve draft to use current user %s, got %s", user.ID, service.approveDraftApprovedBy)
	}

	rejectDraftReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.Team.ID+"/governance/drafts/"+draftID.String()+"/reject", nil)
	rejectDraftReq.AddCookie(cookie)
	rejectDraftResp := httptest.NewRecorder()
	server.ServeHTTP(rejectDraftResp, rejectDraftReq)
	if rejectDraftResp.Code != http.StatusOK {
		t.Fatalf("expected reject governance draft to succeed, got %d: %s", rejectDraftResp.Code, rejectDraftResp.Body.String())
	}
	if service.rejectDraftTenantID != expectedTenantID || service.rejectDraftTeamID.String() != created.Team.ID || service.rejectDraftID != draftID {
		t.Fatalf("expected reject draft tenant/team/draft %s/%s/%s, got %s/%s/%s", expectedTenantID, created.Team.ID, draftID, service.rejectDraftTenantID, service.rejectDraftTeamID, service.rejectDraftID)
	}

	diffReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.Team.ID+"/governance/drafts/"+draftID.String()+"/diff", nil)
	diffReq.AddCookie(cookie)
	diffResp := httptest.NewRecorder()
	server.ServeHTTP(diffResp, diffReq)
	if diffResp.Code != http.StatusOK {
		t.Fatalf("expected preview governance diff to succeed, got %d: %s", diffResp.Code, diffResp.Body.String())
	}
	if service.diffTenantID != expectedTenantID || service.diffTeamID.String() != created.Team.ID || service.diffDraftID != draftID {
		t.Fatalf("expected diff tenant/team/draft %s/%s/%s, got %s/%s/%s", expectedTenantID, created.Team.ID, draftID, service.diffTenantID, service.diffTeamID, service.diffDraftID)
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
		"/api/v1/teams/" + uuid.New().String() + "/governance/drafts?limit=bad",
		"/api/v1/teams/" + uuid.New().String() + "/governance/drafts?offset=-1",
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
	if service.listDraftsCalled {
		t.Fatalf("expected invalid draft pagination not to call tenant service")
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
		targetRole   string
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
		{name: "current governance", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/governance/current", action: authz.ActionTeamGovernanceRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "list governance drafts", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/governance/drafts", action: authz.ActionTeamGovernanceRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "create governance draft", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/governance/drafts", body: `{"human_owner_user_id":"` + ownerID + `"}`, action: authz.ActionTeamGovernanceEdit, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "update governance draft", method: http.MethodPatch, path: "/api/v1/teams/" + teamID + "/governance/drafts/" + uuid.New().String(), body: `{"constitution":{"hard_rules":["review"]}}`, action: authz.ActionTeamGovernanceEdit, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "approve governance draft", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/governance/drafts/" + uuid.New().String() + "/approve", action: authz.ActionTeamGovernanceApprove, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "reject governance draft", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/governance/drafts/" + uuid.New().String() + "/reject", action: authz.ActionTeamGovernanceApprove, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "diff governance draft", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/governance/drafts/" + uuid.New().String() + "/diff", action: authz.ActionTeamGovernanceRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "list members", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/members", action: authz.ActionTeamRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "add member", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/members", body: `{"user_id":"` + uuid.New().String() + `","role":"member"}`, action: authz.ActionTeamMemberAdd, resourceType: authz.ResourceTeam, resourceID: teamID, targetRole: "member"},
		{name: "remove member", method: http.MethodDelete, path: "/api/v1/teams/" + teamID + "/members/" + uuid.New().String(), action: authz.ActionTeamMemberRemove, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "list role requests", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/member-role-requests", action: authz.ActionTeamRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "create role request", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/member-role-requests", body: `{"target_user_id":"` + uuid.New().String() + `","requested_role":"admin","reason":"需要维护成员"}`, action: authz.ActionTeamMemberRequestPrivilegedRole, resourceType: authz.ResourceTeam, resourceID: teamID, targetRole: "admin"},
		{name: "approve role request", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/member-role-requests/" + uuid.New().String() + "/approve", body: `{"decision_reason":"允许"}`, action: authz.ActionTeamMemberApprovePrivilegedRole, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "reject role request", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/member-role-requests/" + uuid.New().String() + "/reject", body: `{"decision_reason":"拒绝"}`, action: authz.ActionTeamMemberApprovePrivilegedRole, resourceType: authz.ResourceTeam, resourceID: teamID},
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
		if expected.targetRole != "" {
			if check.Context["target_role"] != expected.targetRole {
				t.Fatalf("expected target role %s for %s, got %#v", expected.targetRole, expected.name, check.Context)
			}
		}
	}
}

func TestTeamMemberRoutesUseConsoleTenant(t *testing.T) {
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
	teamID := uuid.New()
	targetUserID := uuid.New()

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+teamID.String()+"/members?limit=25&offset=5", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list members to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listMembersTenantID != expectedTenantID || service.listMembersTeamID != teamID || service.listMembersLimit != 25 || service.listMembersOffset != 5 {
		t.Fatalf("unexpected list members args: tenant=%s team=%s limit=%d offset=%d", service.listMembersTenantID, service.listMembersTeamID, service.listMembersLimit, service.listMembersOffset)
	}

	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+teamID.String()+"/members", strings.NewReader(`{"user_id":"`+targetUserID.String()+`","role":"viewer"}`))
	addReq.Header.Set("Content-Type", "application/json")
	addReq.AddCookie(cookie)
	addResp := httptest.NewRecorder()
	server.ServeHTTP(addResp, addReq)
	if addResp.Code != http.StatusCreated {
		t.Fatalf("expected add member to succeed, got %d: %s", addResp.Code, addResp.Body.String())
	}
	if service.addMemberReq.TenantID != expectedTenantID || service.addMemberReq.TeamID != teamID || service.addMemberReq.UserID != targetUserID || service.addMemberReq.Role != tenant.TeamRoleViewer {
		t.Fatalf("unexpected add member request: %#v", service.addMemberReq)
	}

	memberID := uuid.New()
	removeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/teams/"+teamID.String()+"/members/"+memberID.String(), nil)
	removeReq.AddCookie(cookie)
	removeResp := httptest.NewRecorder()
	server.ServeHTTP(removeResp, removeReq)
	if removeResp.Code != http.StatusNoContent {
		t.Fatalf("expected remove member to succeed, got %d: %s", removeResp.Code, removeResp.Body.String())
	}
	if service.removeMemberReq.TenantID != expectedTenantID || service.removeMemberReq.TeamID != teamID || service.removeMemberReq.MembershipID != memberID {
		t.Fatalf("unexpected remove member request: %#v", service.removeMemberReq)
	}

	createRoleReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+teamID.String()+"/member-role-requests", strings.NewReader(`{"target_user_id":"`+targetUserID.String()+`","requested_role":"admin","reason":"需要维护成员"}`))
	createRoleReq.Header.Set("Content-Type", "application/json")
	createRoleReq.AddCookie(cookie)
	createRoleResp := httptest.NewRecorder()
	server.ServeHTTP(createRoleResp, createRoleReq)
	if createRoleResp.Code != http.StatusCreated {
		t.Fatalf("expected create role request to succeed, got %d: %s", createRoleResp.Code, createRoleResp.Body.String())
	}
	if service.createRoleReq.TenantID != expectedTenantID || service.createRoleReq.TeamID != teamID || service.createRoleReq.TargetUserID != targetUserID || service.createRoleReq.RequestedBy != user.ID {
		t.Fatalf("unexpected role request: %#v", service.createRoleReq)
	}

	listRoleReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+teamID.String()+"/member-role-requests?status=pending", nil)
	listRoleReq.AddCookie(cookie)
	listRoleResp := httptest.NewRecorder()
	server.ServeHTTP(listRoleResp, listRoleReq)
	if listRoleResp.Code != http.StatusOK {
		t.Fatalf("expected list role requests to succeed, got %d: %s", listRoleResp.Code, listRoleResp.Body.String())
	}
	if !service.listRoleRequestsCalled || service.listRoleRequestsID != teamID {
		t.Fatalf("expected list role requests to call service for team %s", teamID)
	}

	roleRequestID := uuid.New()
	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+teamID.String()+"/member-role-requests/"+roleRequestID.String()+"/approve", strings.NewReader(`{"decision_reason":"允许"}`))
	approveReq.Header.Set("Content-Type", "application/json")
	approveReq.AddCookie(cookie)
	approveResp := httptest.NewRecorder()
	server.ServeHTTP(approveResp, approveReq)
	if approveResp.Code != http.StatusOK {
		t.Fatalf("expected approve role request to succeed, got %d: %s", approveResp.Code, approveResp.Body.String())
	}
	if service.decideRoleReq.TenantID != expectedTenantID || service.decideRoleReq.TeamID != teamID || service.decideRoleReq.RequestID != roleRequestID || service.decideRoleReq.DecidedBy != user.ID {
		t.Fatalf("unexpected approve request: %#v", service.decideRoleReq)
	}

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+teamID.String()+"/member-role-requests/"+roleRequestID.String()+"/reject", strings.NewReader(`{"decision_reason":"拒绝"}`))
	rejectReq.Header.Set("Content-Type", "application/json")
	rejectReq.AddCookie(cookie)
	rejectResp := httptest.NewRecorder()
	server.ServeHTTP(rejectResp, rejectReq)
	if rejectResp.Code != http.StatusOK {
		t.Fatalf("expected reject role request to succeed, got %d: %s", rejectResp.Code, rejectResp.Body.String())
	}
	if !service.rejectRoleCalled {
		t.Fatalf("expected reject route to call RejectRoleRequest")
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
	createReq              tenant.CreateTeamRequest
	listReq                tenant.ListTeamsRequest
	updateReq              tenant.UpdateTeamRequest
	changeStatusReq        tenant.ChangeTeamStatusRequest
	createRevisionReq      tenant.CreateTeamConfigRevisionRequest
	createDraftReq         tenant.CreateTeamConfigRevisionRequest
	updateDraftInput       tenant.GovernanceDraftInput
	addMemberReq           tenant.AddTeamMemberRequest
	removeMemberReq        tenant.RemoveTeamMemberRequest
	createRoleReq          tenant.CreateRoleRequestRequest
	decideRoleReq          tenant.DecideRoleRequestRequest
	getTenantID            uuid.UUID
	getTeamID              uuid.UUID
	overviewTenantID       uuid.UUID
	overviewTeamID         uuid.UUID
	currentTenantID        uuid.UUID
	currentTeamID          uuid.UUID
	listDraftsTenantID     uuid.UUID
	listDraftsTeamID       uuid.UUID
	listDraftsLimit        int32
	listDraftsOffset       int32
	updateDraftTenantID    uuid.UUID
	updateDraftTeamID      uuid.UUID
	updateDraftID          uuid.UUID
	approveDraftTenantID   uuid.UUID
	approveDraftTeamID     uuid.UUID
	approveDraftID         uuid.UUID
	approveDraftApprovedBy uuid.UUID
	rejectDraftTenantID    uuid.UUID
	rejectDraftTeamID      uuid.UUID
	rejectDraftID          uuid.UUID
	diffTenantID           uuid.UUID
	diffTeamID             uuid.UUID
	diffDraftID            uuid.UUID
	auditTenantID          uuid.UUID
	auditTeamID            uuid.UUID
	auditLimit             int32
	auditOffset            int32
	listMembersTenantID    uuid.UUID
	listMembersTeamID      uuid.UUID
	listMembersLimit       int32
	listMembersOffset      int32
	listRoleRequestsID     uuid.UUID
	createCalled           bool
	listCalled             bool
	getCalled              bool
	overviewCalled         bool
	updateCalled           bool
	changeStatusCalled     bool
	createRevisionCalled   bool
	currentCalled          bool
	listDraftsCalled       bool
	createDraftCalled      bool
	updateDraftCalled      bool
	approveDraftCalled     bool
	rejectDraftCalled      bool
	diffCalled             bool
	auditCalled            bool
	listMembersCalled      bool
	addMemberCalled        bool
	removeMemberCalled     bool
	createRoleCalled       bool
	listRoleRequestsCalled bool
	approveRoleCalled      bool
	rejectRoleCalled       bool
	createdID              uuid.UUID
	rejectMissingOwner     bool
	listErr                error
}

func (s *routeTeamService) CreateTeam(ctx context.Context, req tenant.CreateTeamRequest) (*tenant.TeamOverview, error) {
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
	team := &tenant.Team{
		ID:               s.createdID,
		TenantID:         req.TenantID,
		Slug:             req.Slug,
		Name:             req.Name,
		Status:           status,
		HumanOwnerUserID: req.HumanOwnerUserID,
		Metadata:         req.Metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	return &tenant.TeamOverview{
		Team:                 team,
		MemberCount:          int32(len(req.InitialMembers) + 1),
		DigitalEmployeeCount: 0,
		CapabilityCount:      0,
		PendingDraftCount:    0,
		PendingItemCount:     0,
		AllowedActions:       []tenant.AllowedTeamAction{tenant.AllowedTeamAction(authz.ActionTeamUpdate)},
	}, nil
}

func (s *routeTeamService) ListTeamSummaries(ctx context.Context, req tenant.ListTeamsRequest) ([]*tenant.TeamListItem, error) {
	s.listCalled = true
	s.listReq = req
	if s.listErr != nil {
		return nil, s.listErr
	}
	ownerID := uuid.New()
	return []*tenant.TeamListItem{
		{
			Team: tenant.Team{
				ID:               uuid.New(),
				TenantID:         req.TenantID,
				Slug:             "ops",
				Name:             "Ops",
				Status:           tenant.TeamStatusActive,
				HumanOwnerUserID: &ownerID,
				HumanOwner: &tenant.TeamHumanOwner{
					UserID:      ownerID,
					Username:    "owner",
					DisplayName: "Owner Person",
					Email:       "owner@example.com",
					Status:      "active",
				},
				Metadata:  map[string]any{},
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			GovernanceStatus: tenant.GovernanceSummaryDraftPending,
		},
	}, nil
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

func (s *routeTeamService) ListGovernanceDrafts(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*tenant.TeamConfigRevision, error) {
	s.listDraftsCalled = true
	s.listDraftsTenantID = tenantID
	s.listDraftsTeamID = teamID
	s.listDraftsLimit = limit
	s.listDraftsOffset = offset
	return []*tenant.TeamConfigRevision{s.configRevision(tenantID, teamID, tenant.TeamConfigRevisionStatusDraft)}, nil
}

func (s *routeTeamService) CreateGovernanceDraft(ctx context.Context, req tenant.CreateTeamConfigRevisionRequest) (*tenant.TeamConfigRevision, error) {
	s.createDraftCalled = true
	s.createDraftReq = req
	if s.rejectMissingOwner && req.HumanOwnerUserID == nil {
		return nil, tenant.ErrInvalidInput
	}
	return s.configRevision(req.TenantID, req.TeamID, tenant.TeamConfigRevisionStatusDraft), nil
}

func (s *routeTeamService) UpdateGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID, input tenant.GovernanceDraftInput) (*tenant.TeamConfigRevision, error) {
	s.updateDraftCalled = true
	s.updateDraftTenantID = tenantID
	s.updateDraftTeamID = teamID
	s.updateDraftID = draftID
	s.updateDraftInput = input
	revision := s.configRevision(tenantID, teamID, tenant.TeamConfigRevisionStatusDraft)
	revision.ID = draftID
	revision.HumanOwnerUserID = input.HumanOwnerUserID
	revision.Constitution = input.Constitution
	revision.CapabilityPolicy = input.CapabilityPolicy
	return revision, nil
}

func (s *routeTeamService) ApproveGovernanceDraft(ctx context.Context, tenantID, teamID, draftID, approvedBy uuid.UUID) (*tenant.TeamConfigRevision, error) {
	s.approveDraftCalled = true
	s.approveDraftTenantID = tenantID
	s.approveDraftTeamID = teamID
	s.approveDraftID = draftID
	s.approveDraftApprovedBy = approvedBy
	revision := s.configRevision(tenantID, teamID, tenant.TeamConfigRevisionStatusActive)
	revision.ID = draftID
	revision.ApprovedBy = &approvedBy
	return revision, nil
}

func (s *routeTeamService) RejectGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*tenant.TeamConfigRevision, error) {
	s.rejectDraftCalled = true
	s.rejectDraftTenantID = tenantID
	s.rejectDraftTeamID = teamID
	s.rejectDraftID = draftID
	revision := s.configRevision(tenantID, teamID, tenant.TeamConfigRevisionStatusRejected)
	revision.ID = draftID
	return revision, nil
}

func (s *routeTeamService) PreviewGovernanceDiff(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*tenant.GovernanceDiffSummary, error) {
	s.diffCalled = true
	s.diffTenantID = tenantID
	s.diffTeamID = teamID
	s.diffDraftID = draftID
	return &tenant.GovernanceDiffSummary{
		AddedHardRules:       1,
		ChangedCapabilities:  1,
		ChangedApprovalRules: 0,
		Warnings: []tenant.ValidationIssue{{
			Field:    "constitution.hard_rules",
			Message:  "new hard rule requires review",
			Severity: "warning",
		}},
		BlockingErrors: []tenant.ValidationIssue{},
	}, nil
}

func (s *routeTeamService) configRevision(tenantID, teamID uuid.UUID, status tenant.TeamConfigRevisionStatus) *tenant.TeamConfigRevision {
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
		Status:                      status,
		ApprovedAt:                  &now,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}
}

func (s *routeTeamService) ListTeamMembers(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*tenant.TeamMember, error) {
	s.listMembersCalled = true
	s.listMembersTenantID = tenantID
	s.listMembersTeamID = teamID
	s.listMembersLimit = limit
	s.listMembersOffset = offset
	now := time.Now().UTC()
	return []*tenant.TeamMember{
		{
			MembershipID:     uuid.New(),
			TenantID:         tenantID,
			TeamID:           teamID,
			UserID:           uuid.New(),
			Username:         "member",
			DisplayName:      "Member",
			Email:            "member@example.com",
			AccountStatus:    "active",
			Role:             tenant.TeamRoleMember,
			MembershipStatus: "active",
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}, nil
}

func (s *routeTeamService) AddTeamMember(ctx context.Context, req tenant.AddTeamMemberRequest) (*tenant.TeamMember, error) {
	s.addMemberCalled = true
	s.addMemberReq = req
	now := time.Now().UTC()
	return &tenant.TeamMember{
		MembershipID:     uuid.New(),
		TenantID:         req.TenantID,
		TeamID:           req.TeamID,
		UserID:           req.UserID,
		Username:         "member",
		AccountStatus:    "active",
		Role:             req.Role,
		MembershipStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *routeTeamService) RemoveTeamMember(ctx context.Context, req tenant.RemoveTeamMemberRequest) error {
	s.removeMemberCalled = true
	s.removeMemberReq = req
	return nil
}

func (s *routeTeamService) CreateRoleRequest(ctx context.Context, req tenant.CreateRoleRequestRequest) (*tenant.TeamMemberRoleRequest, error) {
	s.createRoleCalled = true
	s.createRoleReq = req
	now := time.Now().UTC()
	return &tenant.TeamMemberRoleRequest{
		ID:            uuid.New(),
		TenantID:      req.TenantID,
		TeamID:        req.TeamID,
		TargetUserID:  req.TargetUserID,
		RequestedRole: req.RequestedRole,
		RequestedBy:   req.RequestedBy,
		Status:        tenant.TeamMemberRoleRequestStatusPending,
		Reason:        req.Reason,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (s *routeTeamService) ListRoleRequests(ctx context.Context, tenantID, teamID uuid.UUID, status tenant.TeamMemberRoleRequestStatus, limit, offset int32) ([]*tenant.TeamMemberRoleRequest, error) {
	s.listRoleRequestsCalled = true
	s.listRoleRequestsID = teamID
	now := time.Now().UTC()
	return []*tenant.TeamMemberRoleRequest{
		{
			ID:            uuid.New(),
			TenantID:      tenantID,
			TeamID:        teamID,
			TargetUserID:  uuid.New(),
			RequestedRole: tenant.TeamRoleAdmin,
			RequestedBy:   uuid.New(),
			Status:        tenant.TeamMemberRoleRequestStatusPending,
			Reason:        "需要维护成员",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}, nil
}

func (s *routeTeamService) ApproveRoleRequest(ctx context.Context, req tenant.DecideRoleRequestRequest) (*tenant.TeamMemberRoleRequest, error) {
	s.approveRoleCalled = true
	s.decideRoleReq = req
	now := time.Now().UTC()
	return &tenant.TeamMemberRoleRequest{
		ID:             req.RequestID,
		TenantID:       req.TenantID,
		TeamID:         req.TeamID,
		Status:         tenant.TeamMemberRoleRequestStatusApproved,
		DecidedBy:      &req.DecidedBy,
		DecidedAt:      &now,
		DecisionReason: req.DecisionReason,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func (s *routeTeamService) RejectRoleRequest(ctx context.Context, req tenant.DecideRoleRequestRequest) (*tenant.TeamMemberRoleRequest, error) {
	s.rejectRoleCalled = true
	s.decideRoleReq = req
	now := time.Now().UTC()
	return &tenant.TeamMemberRoleRequest{
		ID:             req.RequestID,
		TenantID:       req.TenantID,
		TeamID:         req.TeamID,
		Status:         tenant.TeamMemberRoleRequestStatusRejected,
		DecidedBy:      &req.DecidedBy,
		DecidedAt:      &now,
		DecisionReason: req.DecisionReason,
		CreatedAt:      now,
		UpdatedAt:      now,
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
		s.listDraftsCalled ||
		s.createDraftCalled ||
		s.updateDraftCalled ||
		s.approveDraftCalled ||
		s.rejectDraftCalled ||
		s.diffCalled ||
		s.listMembersCalled ||
		s.addMemberCalled ||
		s.removeMemberCalled ||
		s.createRoleCalled ||
		s.listRoleRequestsCalled ||
		s.approveRoleCalled ||
		s.rejectRoleCalled ||
		s.auditCalled
}

var _ tenant.HandlerService = (*routeTeamService)(nil)
