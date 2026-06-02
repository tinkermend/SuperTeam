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

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list teams to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listReq.TenantID != expectedTenantID {
		t.Fatalf("expected list tenant %s, got %s", expectedTenantID, service.listReq.TenantID)
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
		name   string
		method string
		path   string
		body   string
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/teams"},
		{name: "create", method: http.MethodPost, path: "/api/v1/teams", body: `{"slug":"platform","name":"Platform","human_owner_user_id":"` + ownerID + `"}`},
		{name: "get", method: http.MethodGet, path: "/api/v1/teams/" + teamID},
		{name: "create config revision", method: http.MethodPost, path: "/api/v1/teams/" + teamID + "/config-revisions", body: `{"human_owner_user_id":"` + ownerID + `"}`},
		{name: "current config revision", method: http.MethodGet, path: "/api/v1/teams/" + teamID + "/config-revisions/current"},
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
	createRevisionReq    tenant.CreateTeamConfigRevisionRequest
	getTenantID          uuid.UUID
	getTeamID            uuid.UUID
	currentTenantID      uuid.UUID
	currentTeamID        uuid.UUID
	createCalled         bool
	listCalled           bool
	getCalled            bool
	createRevisionCalled bool
	currentCalled        bool
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

func (s *routeTeamService) ListTeams(ctx context.Context, req tenant.ListTeamsRequest) ([]*tenant.Team, error) {
	s.listCalled = true
	s.listReq = req
	if s.listErr != nil {
		return nil, s.listErr
	}
	return []*tenant.Team{}, nil
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

func (s *routeTeamService) called() bool {
	return s.createCalled ||
		s.listCalled ||
		s.getCalled ||
		s.createRevisionCalled ||
		s.currentCalled
}

var _ tenant.HandlerService = (*routeTeamService)(nil)
