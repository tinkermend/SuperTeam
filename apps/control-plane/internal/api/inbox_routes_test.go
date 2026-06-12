package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/inbox"
)

func TestInboxRoutesRequireConsoleAuth(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetInboxHandler(inbox.NewHandler(&routeInboxService{}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/inbox/badge", nil)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated inbox route to return 401, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestInboxRoutesAreRegisteredWhenHandlerIsSet(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeInboxService{badge: inbox.Badge{MineOpenCount: 2, TeamOpenCount: 5, HighRiskCount: 1}}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetInboxHandler(inbox.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/inbox/badge", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected inbox badge route to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	if service.badgeTenantID != expectedTenantID || service.badgeActorUserID != user.ID || service.badgeIncludeTeam {
		t.Fatalf("expected badge route to pass console identity, got tenant=%s user=%s includeTeam=%v", service.badgeTenantID, service.badgeActorUserID, service.badgeIncludeTeam)
	}
}

type routeInboxService struct {
	listReq          inbox.ListItemsRequest
	badgeTenantID    uuid.UUID
	badgeActorUserID uuid.UUID
	badgeIncludeTeam bool
	badge            inbox.Badge
	executeReq       inbox.ExecuteActionRequest
}

func (s *routeInboxService) ListItems(_ context.Context, req inbox.ListItemsRequest) (inbox.ListItemsResult, error) {
	s.listReq = req
	return inbox.ListItemsResult{}, nil
}

func (s *routeInboxService) GetBadge(_ context.Context, tenantID, actorUserID uuid.UUID, includeTeam bool) (inbox.Badge, error) {
	s.badgeTenantID = tenantID
	s.badgeActorUserID = actorUserID
	s.badgeIncludeTeam = includeTeam
	return s.badge, nil
}

func (s *routeInboxService) ExecuteAction(_ context.Context, req inbox.ExecuteActionRequest) (inbox.Item, inbox.SourceActionResult, error) {
	s.executeReq = req
	return inbox.Item{}, inbox.SourceActionResult{}, nil
}
