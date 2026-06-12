package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

func TestHandlerListItemsUsesConsoleIdentity(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	projectID := uuid.New()
	spoofedTargetID := uuid.New()
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	summary := "Needs review"
	risk := "high"
	priority := "urgent"
	service := &handlerService{
		listResult: ListItemsResult{
			Items: []Item{{
				ID:             uuid.New(),
				TenantID:       tenantID,
				TargetUserID:   userID,
				ItemType:       ItemTypeApproval,
				SourceType:     SourceTypeApprovalRequest,
				SourceID:       uuid.New(),
				Title:          "Approve rollout",
				Summary:        &summary,
				RiskLevel:      &risk,
				Priority:       &priority,
				Status:         StatusOpen,
				Actions:        DefaultActions(ItemTypeApproval),
				ContextPayload: map[string]any{"source": "test"},
				DeepLink:       map[string]any{"route": "/approvals"},
				LastActivityAt: now,
				CreatedAt:      now,
				UpdatedAt:      now,
			}},
			Limit:         25,
			Offset:        5,
			HasMore:       true,
			OpenCount:     3,
			HighRiskCount: 1,
		},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/inbox/items?view=mine&status=open&item_type=approval&risk_level=high&project_id="+projectID.String()+"&target_user_id="+spoofedTargetID.String()+"&limit=25&offset=5", nil)
	req = withConsoleIdentity(req, tenantID, userID)
	resp := httptest.NewRecorder()

	handler.ListItems(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.listReq.TenantID != tenantID || service.listReq.ActorUserID != userID {
		t.Fatalf("expected console tenant/user %s/%s, got %s/%s", tenantID, userID, service.listReq.TenantID, service.listReq.ActorUserID)
	}
	if service.listReq.View != ViewMine || service.listReq.Status != StatusOpen || service.listReq.Limit != 25 || service.listReq.Offset != 5 {
		t.Fatalf("expected parsed list filters, got %#v", service.listReq)
	}
	if service.listReq.ItemType == nil || *service.listReq.ItemType != ItemTypeApproval {
		t.Fatalf("expected item type filter, got %#v", service.listReq.ItemType)
	}
	if service.listReq.RiskLevel == nil || *service.listReq.RiskLevel != "high" {
		t.Fatalf("expected risk filter, got %#v", service.listReq.RiskLevel)
	}
	if service.listReq.ProjectID == nil || *service.listReq.ProjectID != projectID {
		t.Fatalf("expected project filter, got %#v", service.listReq.ProjectID)
	}
	if service.listReq.TargetUserID == nil || *service.listReq.TargetUserID != spoofedTargetID {
		t.Fatalf("expected target user filter to be parsed for service normalization, got %#v", service.listReq.TargetUserID)
	}

	var body struct {
		Items []struct {
			ID        string         `json:"id"`
			TenantID  string         `json:"tenant_id"`
			Title     string         `json:"title"`
			Status    Status         `json:"status"`
			Actions   []Action       `json:"actions"`
			Context   map[string]any `json:"context"`
			DeepLink  map[string]any `json:"deep_link"`
			CreatedAt string         `json:"created_at"`
		} `json:"items"`
		Pagination struct {
			Limit   int32 `json:"limit"`
			Offset  int32 `json:"offset"`
			HasMore bool  `json:"has_more"`
		} `json:"pagination"`
		Summary struct {
			OpenCount     int64 `json:"open_count"`
			HighRiskCount int64 `json:"high_risk_count"`
			BlockedCount  int64 `json:"blocked_count"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Title != "Approve rollout" || body.Items[0].TenantID != tenantID.String() {
		t.Fatalf("unexpected item response: %#v", body.Items)
	}
	if body.Items[0].Context["source"] != "test" || body.Items[0].DeepLink["route"] != "/approvals" || len(body.Items[0].Actions) == 0 {
		t.Fatalf("expected item details to be serialized, got %#v", body.Items[0])
	}
	if body.Pagination.Limit != 25 || body.Pagination.Offset != 5 || !body.Pagination.HasMore {
		t.Fatalf("unexpected pagination: %#v", body.Pagination)
	}
	if body.Summary.OpenCount != 3 || body.Summary.HighRiskCount != 1 || body.Summary.BlockedCount != 0 {
		t.Fatalf("unexpected summary: %#v", body.Summary)
	}
}

func TestHandlerBadgeUsesConsoleIdentity(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	service := &handlerService{
		badge: Badge{MineOpenCount: 4, TeamOpenCount: 9, HighRiskCount: 2},
	}
	handler := NewHandler(service)
	req := withConsoleIdentity(httptest.NewRequest(http.MethodGet, "/inbox/badge", nil), tenantID, userID)
	resp := httptest.NewRecorder()

	handler.GetBadge(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.badgeTenantID != tenantID || service.badgeActorUserID != userID || service.badgeIncludeTeam {
		t.Fatalf("expected badge to use console identity without team count, got tenant=%s user=%s includeTeam=%v", service.badgeTenantID, service.badgeActorUserID, service.badgeIncludeTeam)
	}
	var body struct {
		MineOpenCount int64 `json:"mine_open_count"`
		TeamOpenCount int64 `json:"team_open_count"`
		HighRiskCount int64 `json:"high_risk_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.MineOpenCount != 4 || body.TeamOpenCount != 0 || body.HighRiskCount != 2 {
		t.Fatalf("unexpected badge: %#v", body)
	}
}

func TestHandlerTeamListWithoutAuthorizerReturnsForbidden(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	service := &handlerService{}
	handler := NewHandler(service)
	req := withConsoleIdentity(httptest.NewRequest(http.MethodGet, "/inbox/items?view=team", nil), tenantID, userID)
	resp := httptest.NewRecorder()

	handler.ListItems(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.listCalled {
		t.Fatalf("expected unauthorized team view to stop before service, got %#v", service.listReq)
	}
}

func TestHandlerTeamListWithAuthorizerAllowedPassesTeamView(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	service := &handlerService{
		listResult: ListItemsResult{Limit: 50, Offset: 0},
	}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)
	req := withConsoleIdentity(httptest.NewRequest(http.MethodGet, "/inbox/items?view=team", nil), tenantID, userID)
	resp := httptest.NewRecorder()

	handler.ListItems(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !service.listCalled || service.listReq.View != ViewTeam || !service.listReq.TeamViewAllowed {
		t.Fatalf("expected authorized team view to reach service, got called=%v req=%#v", service.listCalled, service.listReq)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authz check, got %d", len(authorizer.checks))
	}
	check := authorizer.checks[0]
	if check.Actor.Type != authz.ActorUser || check.Actor.ID != userID.String() || check.Action != authz.ActionTeamRead || check.Resource.Type != authz.ResourceTenant || check.Resource.ID != tenantID.String() || check.TenantID != tenantID {
		t.Fatalf("unexpected authz check: %#v", check)
	}
}

func TestHandlerTeamListAuthorizerErrorReturnsInternalServerError(t *testing.T) {
	service := &handlerService{}
	handler := NewHandler(service)
	handler.SetAuthorizer(&handlerAuthorizer{err: errors.New("authz unavailable")})
	req := withConsoleIdentity(httptest.NewRequest(http.MethodGet, "/inbox/items?view=team", nil), uuid.New(), uuid.New())
	resp := httptest.NewRecorder()

	handler.ListItems(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.listCalled {
		t.Fatalf("expected authz error to stop before service, got %#v", service.listReq)
	}
}

func TestHandlerBadgeWithAuthorizerAllowedIncludesTeamCount(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	service := &handlerService{
		badge: Badge{MineOpenCount: 4, TeamOpenCount: 9, HighRiskCount: 2},
	}
	authorizer := &handlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)
	req := withConsoleIdentity(httptest.NewRequest(http.MethodGet, "/inbox/badge", nil), tenantID, userID)
	resp := httptest.NewRecorder()

	handler.GetBadge(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.badgeTenantID != tenantID || service.badgeActorUserID != userID || !service.badgeIncludeTeam {
		t.Fatalf("expected badge to include authorized team count, got tenant=%s user=%s includeTeam=%v", service.badgeTenantID, service.badgeActorUserID, service.badgeIncludeTeam)
	}
	var body struct {
		MineOpenCount int64 `json:"mine_open_count"`
		TeamOpenCount int64 `json:"team_open_count"`
		HighRiskCount int64 `json:"high_risk_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.MineOpenCount != 4 || body.TeamOpenCount != 9 || body.HighRiskCount != 2 {
		t.Fatalf("unexpected badge: %#v", body)
	}
}

func TestHandlerBadgeAuthorizerErrorReturnsInternalServerError(t *testing.T) {
	service := &handlerService{}
	handler := NewHandler(service)
	handler.SetAuthorizer(&handlerAuthorizer{err: errors.New("authz unavailable")})
	req := withConsoleIdentity(httptest.NewRequest(http.MethodGet, "/inbox/badge", nil), uuid.New(), uuid.New())
	resp := httptest.NewRecorder()

	handler.GetBadge(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.badgeCalled {
		t.Fatalf("expected authz error to stop before service, got includeTeam=%v", service.badgeIncludeTeam)
	}
}

func TestHandlerExecuteActionRejectsWrongUser(t *testing.T) {
	service := &handlerService{executeErr: ErrActionForbidden}
	handler := NewHandler(service)
	itemID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/inbox/items/"+itemID.String()+"/actions", strings.NewReader(`{"action":"approved"}`))
	req = withConsoleIdentity(req, uuid.New(), uuid.New())
	req = withItemRouteParam(req, itemID)
	resp := httptest.NewRecorder()

	handler.ExecuteAction(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.executeReq.ItemID != itemID {
		t.Fatalf("expected route item id %s, got %s", itemID, service.executeReq.ItemID)
	}
}

func TestHandlerExecuteActionReturnsSourceResult(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	itemID := uuid.New()
	sourceID := uuid.New()
	now := time.Date(2026, 6, 12, 11, 0, 0, 0, time.UTC)
	service := &handlerService{
		executeItem: Item{
			ID:             itemID,
			TenantID:       tenantID,
			TargetUserID:   userID,
			ItemType:       ItemTypeApproval,
			SourceType:     SourceTypeApprovalRequest,
			SourceID:       sourceID,
			Title:          "Approve deploy",
			Status:         StatusResolved,
			Actions:        DefaultActions(ItemTypeApproval),
			ContextPayload: map[string]any{},
			DeepLink:       map[string]any{},
			LastActivityAt: now,
			CreatedAt:      now,
			UpdatedAt:      now,
			ResolvedAt:     &now,
		},
		executeResult: SourceActionResult{SourceType: string(SourceTypeApprovalRequest), SourceID: sourceID, Status: "approved"},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/inbox/items/"+itemID.String()+"/actions", strings.NewReader(`{"action":"approved","comment":"ship it","payload":{"reason":"clear"}}`))
	req = withConsoleIdentity(req, tenantID, userID)
	req = withItemRouteParam(req, itemID)
	resp := httptest.NewRecorder()

	handler.ExecuteAction(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.executeReq.TenantID != tenantID || service.executeReq.ActorUserID != userID || service.executeReq.ItemID != itemID {
		t.Fatalf("expected execute action to use console identity and route item, got %#v", service.executeReq)
	}
	if service.executeReq.Action != "approved" || service.executeReq.Comment != "ship it" || service.executeReq.Payload["reason"] != "clear" {
		t.Fatalf("expected action body to be forwarded, got %#v", service.executeReq)
	}
	var body struct {
		Item struct {
			ID     string `json:"id"`
			Status Status `json:"status"`
		} `json:"item"`
		SourceResult struct {
			SourceType string `json:"source_type"`
			SourceID   string `json:"source_id"`
			Status     string `json:"status"`
		} `json:"source_result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Item.ID != itemID.String() || body.Item.Status != StatusResolved {
		t.Fatalf("unexpected item response: %#v", body.Item)
	}
	if body.SourceResult.SourceType != string(SourceTypeApprovalRequest) || body.SourceResult.SourceID != sourceID.String() || body.SourceResult.Status != "approved" {
		t.Fatalf("unexpected source result: %#v", body.SourceResult)
	}
}

func TestHandlerExecuteActionReturnsNormalizedSourceErrorStatus(t *testing.T) {
	itemID := uuid.New()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid source action", err: ErrInvalidAction, wantStatus: http.StatusBadRequest},
		{name: "source unavailable", err: ErrSourceUnavailable, wantStatus: http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &handlerService{executeErr: tt.err}
			handler := NewHandler(service)
			req := httptest.NewRequest(http.MethodPost, "/inbox/items/"+itemID.String()+"/actions", strings.NewReader(`{"action":"approved"}`))
			req = withConsoleIdentity(req, uuid.New(), uuid.New())
			req = withItemRouteParam(req, itemID)
			resp := httptest.NewRecorder()

			handler.ExecuteAction(resp, req)

			if resp.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tt.wantStatus, resp.Code, resp.Body.String())
			}
		})
	}
}

type handlerService struct {
	listCalled       bool
	listReq          ListItemsRequest
	listResult       ListItemsResult
	listErr          error
	badgeCalled      bool
	badgeTenantID    uuid.UUID
	badgeActorUserID uuid.UUID
	badgeIncludeTeam bool
	badge            Badge
	badgeErr         error
	executeReq       ExecuteActionRequest
	executeItem      Item
	executeResult    SourceActionResult
	executeErr       error
}

func (s *handlerService) ListItems(_ context.Context, req ListItemsRequest) (ListItemsResult, error) {
	s.listCalled = true
	s.listReq = req
	return s.listResult, s.listErr
}

func (s *handlerService) GetBadge(_ context.Context, tenantID, actorUserID uuid.UUID, includeTeam bool) (Badge, error) {
	s.badgeCalled = true
	s.badgeTenantID = tenantID
	s.badgeActorUserID = actorUserID
	s.badgeIncludeTeam = includeTeam
	badge := s.badge
	if !includeTeam {
		badge.TeamOpenCount = 0
	}
	return badge, s.badgeErr
}

func (s *handlerService) ExecuteAction(_ context.Context, req ExecuteActionRequest) (Item, SourceActionResult, error) {
	s.executeReq = req
	return s.executeItem, s.executeResult, s.executeErr
}

func withConsoleIdentity(req *http.Request, tenantID, userID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	return req.WithContext(ctx)
}

func withItemRouteParam(req *http.Request, itemID uuid.UUID) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("itemId", itemID.String())
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

type handlerAuthorizer struct {
	allowed bool
	err     error
	checks  []authz.CheckRequest
}

func (a *handlerAuthorizer) Check(_ context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	if a.err != nil {
		return authz.Decision{}, a.err
	}
	if a.allowed {
		return authz.Decision{Allowed: true, Reason: authz.ReasonAllowed}, nil
	}
	return authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership}, nil
}
