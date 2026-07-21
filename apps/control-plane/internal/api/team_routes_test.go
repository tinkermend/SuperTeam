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
	"github.com/superteam/control-plane/internal/capability"
	"github.com/superteam/control-plane/internal/platform"
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
	expectedTenantID := platform.DefaultTenantID
	ownerID := uuid.New()
	memberID := uuid.New()
	viewerID := uuid.New()

	createBody := `{
		"slug":"platform",
		"name":"Platform",
		"description":"提供平台工程与可靠性交付能力",
		"human_owner_user_ids":["` + ownerID.String() + `"],
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
	if service.createReq.HumanOwnerUserIDs == nil || service.createReq.HumanOwnerUserIDs[0] != ownerID {
		t.Fatalf("expected request human owner %s, got %#v", ownerID, service.createReq.HumanOwnerUserIDs)
	}
	if service.createReq.ActorUserID != user.ID {
		t.Fatalf("expected actor user %s, got %s", user.ID, service.createReq.ActorUserID)
	}
	if service.createReq.Description != "提供平台工程与可靠性交付能力" {
		t.Fatalf("expected description in create request, got %q", service.createReq.Description)
	}
	if !reflect.DeepEqual(service.createReq.InitialMembers, []tenant.InitialTeamMemberInput{
		{UserID: memberID, Role: tenant.TeamRoleMember},
		{UserID: viewerID, Role: tenant.TeamRoleViewer},
	}) {
		t.Fatalf("expected initial members in create request, got %#v", service.createReq.InitialMembers)
	}
	var created struct {
		Team struct {
			ID                string         `json:"id"`
			TenantID          string         `json:"tenant_id"`
			Description       string         `json:"description"`
			HumanOwnerUserIDs []string       `json:"human_owner_user_ids"`
			Metadata          map[string]any `json:"metadata"`
		} `json:"team"`
		AllowedActions []string `json:"allowed_actions"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created team: %v", err)
	}
	if created.Team.TenantID != expectedTenantID.String() || len(created.Team.HumanOwnerUserIDs) == 0 || created.Team.HumanOwnerUserIDs[0] != ownerID.String() {
		t.Fatalf("expected response tenant/owner %s/%s, got %#v", expectedTenantID, ownerID, created)
	}
	if created.Team.Metadata["cost_center"] != "r-and-d" {
		t.Fatalf("expected metadata in response, got %#v", created.Team.Metadata)
	}
	if created.Team.Description != "提供平台工程与可靠性交付能力" {
		t.Fatalf("expected description in response, got %q", created.Team.Description)
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
		Description string `json:"description"`
		HumanOwners []struct {
			UserID      string `json:"user_id"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
			Status      string `json:"status"`
			Avatar      *struct {
				Seed string `json:"seed"`
			} `json:"avatar"`
		} `json:"human_owners"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed teams: %v", err)
	}
	if len(listed) != 1 || len(listed[0].HumanOwners) == 0 || listed[0].HumanOwners[0].Username != "owner" || listed[0].HumanOwners[0].DisplayName != "Owner Person" || listed[0].HumanOwners[0].Email != "owner@example.com" {
		t.Fatalf("expected list response to include human owner summary, got %#v", listed)
	}
	if listed[0].HumanOwners[0].Avatar == nil || listed[0].HumanOwners[0].Avatar.Seed != "user:owner" {
		t.Fatalf("expected list response to include human owner avatar, got %#v", listed)
	}
	if listed[0].Description != "负责日常平台运行与发布保障" {
		t.Fatalf("expected list response to include description, got %#v", listed)
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

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.Team.ID, strings.NewReader(`{"slug":"platform-sre","name":"Platform SRE","description":"负责平台可靠性与工程效率","human_owner_user_ids":["`+ownerID.String()+`"],"metadata":{"cost_center":"ops"}}`))
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
	if service.updateReq.Description == nil || *service.updateReq.Description != "负责平台可靠性与工程效率" {
		t.Fatalf("expected description in update request, got %#v", service.updateReq.Description)
	}

	constitutionReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.Team.ID+"/constitution", strings.NewReader(`{"hard_rules":["review before deploy"]}`))
	constitutionReq.Header.Set("Content-Type", "application/json")
	constitutionReq.AddCookie(cookie)
	constitutionResp := httptest.NewRecorder()
	server.ServeHTTP(constitutionResp, constitutionReq)
	if constitutionResp.Code != http.StatusOK {
		t.Fatalf("expected update constitution to succeed, got %d: %s", constitutionResp.Code, constitutionResp.Body.String())
	}
	if service.updateConstitutionTenantID != expectedTenantID || service.updateConstitutionTeamID.String() != created.Team.ID {
		t.Fatalf("expected constitution tenant/team %s/%s, got %s/%s", expectedTenantID, created.Team.ID, service.updateConstitutionTenantID, service.updateConstitutionTeamID)
	}
	if !reflect.DeepEqual(service.updateConstitution, map[string]any{"hard_rules": []any{"review before deploy"}}) {
		t.Fatalf("expected constitution payload, got %#v", service.updateConstitution)
	}
	var constitutionBody struct {
		Constitution map[string]any `json:"constitution"`
	}
	if err := json.NewDecoder(constitutionResp.Body).Decode(&constitutionBody); err != nil {
		t.Fatalf("decode constitution response: %v", err)
	}
	if !reflect.DeepEqual(constitutionBody.Constitution, map[string]any{"hard_rules": []any{"review before deploy"}}) {
		t.Fatalf("expected response constitution, got %#v", constitutionBody.Constitution)
	}
}

type routeCapabilityService struct {
	mcpDefinition    capability.MCPDefinition
	mcpBinding       capability.MCPBinding
	effectiveServers []capability.EffectiveMCPServer

	createDefinitionReq        capability.CreateMCPServerDefinitionRequest
	listDefinitionsReq         capability.ListMCPServerDefinitionsRequest
	deleteDefinitionReq        capability.DeleteMCPServerDefinitionRequest
	createTeamBindingReq       capability.CreateTeamMCPBindingRequest
	createEmployeeBindingV2Req capability.CreateEmployeeMCPBindingV2Request
	effectiveConfigReq         capability.EmployeeScopedRequest

	skillMCPDependencies      []capability.SkillMCPDependency
	dependentSkills           []capability.DependentSkill
	listSkillDependenciesReq  capability.ListSkillMCPDependenciesRequest
	replaceSkillDependencyReq capability.ReplaceSkillMCPDependenciesRequest
	listDependentSkillsReq    capability.ListDependentSkillsRequest
}

func (s *routeCapabilityService) CreateMCPServerDefinition(ctx context.Context, req capability.CreateMCPServerDefinitionRequest) (capability.MCPDefinition, error) {
	s.createDefinitionReq = req
	return s.mcpDefinition, nil
}

func (s *routeCapabilityService) ListMCPServerDefinitions(ctx context.Context, req capability.ListMCPServerDefinitionsRequest) ([]capability.MCPDefinition, error) {
	s.listDefinitionsReq = req
	return []capability.MCPDefinition{s.mcpDefinition}, nil
}

func (s *routeCapabilityService) DeleteMCPServerDefinition(ctx context.Context, req capability.DeleteMCPServerDefinitionRequest) error {
	s.deleteDefinitionReq = req
	return nil
}

func (s *routeCapabilityService) CreateTeamMCPBinding(ctx context.Context, req capability.CreateTeamMCPBindingRequest) (capability.MCPBinding, error) {
	s.createTeamBindingReq = req
	return s.mcpBinding, nil
}

func (s *routeCapabilityService) ListTeamMCPBindings(context.Context, capability.TeamScopedRequest) ([]capability.MCPBinding, error) {
	return []capability.MCPBinding{s.mcpBinding}, nil
}

func (s *routeCapabilityService) DeleteTeamMCPBinding(context.Context, capability.DeleteTeamMCPBindingRequest) error {
	return nil
}

func (s *routeCapabilityService) CreateEmployeeMCPBindingV2(ctx context.Context, req capability.CreateEmployeeMCPBindingV2Request) (capability.MCPBinding, error) {
	s.createEmployeeBindingV2Req = req
	return s.mcpBinding, nil
}

func (s *routeCapabilityService) ListEmployeeMCPBindingsV2(context.Context, capability.EmployeeScopedRequest) ([]capability.MCPBinding, error) {
	return []capability.MCPBinding{s.mcpBinding}, nil
}

func (s *routeCapabilityService) DeleteEmployeeMCPBindingV2(context.Context, capability.DeleteEmployeeMCPBindingV2Request) error {
	return nil
}

func (s *routeCapabilityService) ListEffectiveMCPConfig(ctx context.Context, req capability.EmployeeScopedRequest) ([]capability.EffectiveMCPServer, error) {
	s.effectiveConfigReq = req
	return s.effectiveServers, nil
}

func (s *routeCapabilityService) ListSkillMCPDependencies(ctx context.Context, req capability.ListSkillMCPDependenciesRequest) ([]capability.SkillMCPDependency, error) {
	s.listSkillDependenciesReq = req
	return s.skillMCPDependencies, nil
}

func (s *routeCapabilityService) ReplaceSkillMCPDependencies(ctx context.Context, req capability.ReplaceSkillMCPDependenciesRequest) ([]capability.SkillMCPDependency, error) {
	s.replaceSkillDependencyReq = req
	return s.skillMCPDependencies, nil
}

func (s *routeCapabilityService) ListDependentSkills(ctx context.Context, req capability.ListDependentSkillsRequest) ([]capability.DependentSkill, error) {
	s.listDependentSkillsReq = req
	return s.dependentSkills, nil
}

func (s *routeCapabilityService) EvaluateEmployeeSkillMCPDependencies(ctx context.Context, req capability.EvaluateEmployeeSkillMCPDependenciesRequest) ([]capability.EmployeeSkillMCPDependencyStatus, error) {
	return nil, nil
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
		targetRole   string
		teamID       *uuid.UUID
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/teams", action: authz.ActionTeamRead, resourceType: authz.ResourceTenant},
		{name: "create", method: http.MethodPost, path: "/api/v1/teams", body: `{"slug":"platform","name":"Platform","human_owner_user_ids":["` + ownerID + `"]}`, action: authz.ActionTeamCreate, resourceType: authz.ResourceTenant},
		{name: "get", method: http.MethodGet, path: "/api/v1/teams/" + teamID, action: authz.ActionTeamRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "overview", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/overview", action: authz.ActionTeamRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "update", method: http.MethodPatch, path: "/api/v1/teams/" + teamID, body: `{"slug":"platform","name":"Platform"}`, action: authz.ActionTeamUpdate, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "update constitution", method: http.MethodPatch, path: "/api/v1/teams/" + teamID + "/constitution", body: `{"constitution":{"hard_rules":["review"]}}`, action: authz.ActionTeamGovernanceEdit, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "list members", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/members", action: authz.ActionTeamRead, resourceType: authz.ResourceTeam, resourceID: teamID},
		{name: "add member", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/members", body: `{"user_id":"` + uuid.New().String() + `","role":"member"}`, action: authz.ActionTeamMemberAdd, resourceType: authz.ResourceTeam, resourceID: teamID, targetRole: "member"},
		{name: "remove member", method: http.MethodDelete, path: "/api/v1/teams/" + teamID + "/members/" + uuid.New().String(), action: authz.ActionTeamMemberRemove, resourceType: authz.ResourceTeam, resourceID: teamID},
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
	expectedTenantID := platform.DefaultTenantID
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
	expectedTenantID := platform.DefaultTenantID
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
	var listedMembers []struct {
		Avatar *struct {
			Seed string `json:"seed"`
		} `json:"avatar"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listedMembers); err != nil {
		t.Fatalf("decode listed members: %v", err)
	}
	if len(listedMembers) != 1 || listedMembers[0].Avatar == nil || listedMembers[0].Avatar.Seed != "user:member" {
		t.Fatalf("expected list members response to include member avatar, got %#v", listedMembers)
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
}

func TestUpdateTeamConstitutionUsesGovernanceEditAuthorization(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+teamID.String()+"/constitution", strings.NewReader(`{"hard_rules":["review before deploy"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected update constitution to succeed, got %d: %s", resp.Code, resp.Body.String())
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
	expectedTenantID := platform.DefaultTenantID
	if service.updateConstitutionTenantID != expectedTenantID || service.updateConstitutionTeamID != teamID {
		t.Fatalf("expected service tenant/team %s/%s, got %s/%s", expectedTenantID, teamID, service.updateConstitutionTenantID, service.updateConstitutionTeamID)
	}
	if !reflect.DeepEqual(service.updateConstitution, map[string]any{
		"hard_rules": []any{"review before deploy"},
	}) {
		t.Fatalf("expected raw constitution body, got %#v", service.updateConstitution)
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
	expectedTenantID := platform.DefaultTenantID
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
			authz.ActionTeamDelete: true,
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
	if containsString(body.AllowedActions, authz.ActionTeamDelete) {
		t.Fatalf("expected denied delete action to be filtered, got %#v", body.AllowedActions)
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
	if service.createReq.HumanOwnerUserIDs != nil {
		t.Fatalf("expected handler not to substitute console user %s as team owner, got %#v", user.ID, service.createReq.HumanOwnerUserIDs)
	}
}

type routeTeamService struct {
	createReq                  tenant.CreateTeamRequest
	listReq                    tenant.ListTeamsRequest
	updateReq                  tenant.UpdateTeamRequest
	updateConstitution         map[string]any
	addMemberReq               tenant.AddTeamMemberRequest
	removeMemberReq            tenant.RemoveTeamMemberRequest
	getTenantID                uuid.UUID
	getTeamID                  uuid.UUID
	overviewTenantID           uuid.UUID
	overviewTeamID             uuid.UUID
	updateConstitutionTenantID uuid.UUID
	updateConstitutionTeamID   uuid.UUID
	auditTenantID              uuid.UUID
	auditTeamID                uuid.UUID
	auditLimit                 int32
	auditOffset                int32
	listMembersTenantID        uuid.UUID
	listMembersTeamID          uuid.UUID
	listMembersLimit           int32
	listMembersOffset          int32
	createCalled               bool
	listCalled                 bool
	getCalled                  bool
	overviewCalled             bool
	updateCalled               bool
	updateConstitutionCalled   bool
	auditCalled                bool
	listMembersCalled          bool
	addMemberCalled            bool
	removeMemberCalled         bool
	createdID                  uuid.UUID
	rejectMissingOwner         bool
	listErr                    error
}

func (s *routeTeamService) CreateTeam(ctx context.Context, req tenant.CreateTeamRequest) (*tenant.TeamOverview, error) {
	s.createCalled = true
	s.createReq = req
	if s.rejectMissingOwner && req.HumanOwnerUserIDs == nil {
		return nil, tenant.ErrInvalidInput
	}
	s.createdID = uuid.New()
	now := time.Now().UTC()
	status := req.Status
	if status == "" {
		status = tenant.TeamStatusActive
	}
	team := &tenant.Team{
		ID:                s.createdID,
		TenantID:          req.TenantID,
		Slug:              req.Slug,
		Name:              req.Name,
		Description:       req.Description,
		Status:            status,
		HumanOwnerUserIDs: req.HumanOwnerUserIDs,
		Metadata:          req.Metadata,
		CreatedAt:         now,
		UpdatedAt:         now,
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
				ID:                uuid.New(),
				TenantID:          req.TenantID,
				Slug:              "ops",
				Name:              "Ops",
				Description:       "负责日常平台运行与发布保障",
				Status:            tenant.TeamStatusActive,
				HumanOwnerUserIDs: []uuid.UUID{ownerID},
				HumanOwners: []tenant.TeamHumanOwner{{
					UserID:      ownerID,
					Username:    "owner",
					DisplayName: "Owner Person",
					Email:       "owner@example.com",
					Status:      "active",
					Avatar: &tenant.UserAvatarConfig{
						Provider: "dicebear",
						Style:    "adventurer",
						Seed:     "user:owner",
						Options:  map[string]any{"backgroundColor": []any{"e6fbf5"}},
					},
				}},
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
		ID:                teamID,
		TenantID:          tenantID,
		Slug:              "platform",
		Name:              "Platform",
		Status:            tenant.TeamStatusActive,
		HumanOwnerUserIDs: []uuid.UUID{},
		Metadata:          map[string]any{},
		CreatedAt:         now,
		UpdatedAt:         now,
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
			tenant.AllowedTeamAction(authz.ActionTeamDelete),
		},
	}, nil
}

func (s *routeTeamService) UpdateTeam(ctx context.Context, req tenant.UpdateTeamRequest) (*tenant.Team, error) {
	s.updateCalled = true
	s.updateReq = req
	now := time.Now().UTC()
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	return &tenant.Team{
		ID:                req.TeamID,
		TenantID:          req.TenantID,
		Slug:              req.Slug,
		Name:              req.Name,
		Description:       description,
		Status:            tenant.TeamStatusActive,
		HumanOwnerUserIDs: req.HumanOwnerUserIDs,
		Metadata:          req.Metadata,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (s *routeTeamService) UpdateTeamConstitution(ctx context.Context, tenantID, teamID uuid.UUID, constitution map[string]any) (*tenant.Team, error) {
	s.updateConstitutionCalled = true
	s.updateConstitutionTenantID = tenantID
	s.updateConstitutionTeamID = teamID
	s.updateConstitution = constitution
	now := time.Now().UTC()
	return &tenant.Team{
		ID:           teamID,
		TenantID:     tenantID,
		Slug:         "platform",
		Name:         "Platform",
		Status:       tenant.TeamStatusActive,
		Constitution: constitution,
		Metadata:     map[string]any{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (s *routeTeamService) DeleteTeam(_ context.Context, _ tenant.DeleteTeamRequest) error {
	return nil
}

func (s *routeTeamService) ListPendingDeleteTeams(_ context.Context, _ uuid.UUID) ([]tenant.PendingDeleteTeamRecord, error) {
	return nil, nil
}

func (s *routeTeamService) RestorePendingDeleteTeam(_ context.Context, _, _, _ uuid.UUID) (*tenant.Team, error) {
	return &tenant.Team{}, nil
}

func (s *routeTeamService) ConfirmTeamDelete(_ context.Context, _, _, _ uuid.UUID) error {
	return nil
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
			MembershipID:  uuid.New(),
			TenantID:      tenantID,
			TeamID:        teamID,
			UserID:        uuid.New(),
			Username:      "member",
			DisplayName:   "Member",
			Email:         "member@example.com",
			AccountStatus: "active",
			Avatar: &tenant.UserAvatarConfig{
				Provider: "dicebear",
				Style:    "adventurer",
				Seed:     "user:member",
				Options:  map[string]any{"backgroundColor": []any{"e6fbf5"}},
			},
			Role:             tenant.TeamRoleMember,
			MembershipStatus: "active",
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}, nil
}

func (s *routeTeamService) BindTeamDigitalEmployee(ctx context.Context, req tenant.BindTeamDigitalEmployeeRequest) error {
	return nil
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
		s.listMembersCalled ||
		s.addMemberCalled ||
		s.removeMemberCalled ||
		s.auditCalled
}

var _ tenant.HandlerService = (*routeTeamService)(nil)
