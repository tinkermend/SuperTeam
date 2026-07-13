package scenariotemplate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type stubService struct {
	templates []ScenarioTemplate
}

func (s *stubService) List(_ context.Context, _ uuid.UUID) ([]ScenarioTemplate, error) {
	return s.templates, nil
}

func (s *stubService) GetByKey(_ context.Context, _ uuid.UUID, key string) (ScenarioTemplate, error) {
	for _, template := range s.templates {
		if template.Key == key {
			return template, nil
		}
	}
	return ScenarioTemplate{}, ErrScenarioTemplateNotFound
}

type stubAuthorizer struct {
	allowed bool
	checks  []authz.CheckRequest
}

func (a *stubAuthorizer) Check(_ context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	if a.allowed {
		return authz.Decision{Allowed: true, Reason: authz.ReasonAllowed}, nil
	}
	return authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership}, nil
}

func (a *stubAuthorizer) CheckBulkTeamActions(_ context.Context, _ authz.BulkTeamActionsRequest) ([]string, error) {
	return nil, nil
}

func identityRequest(req *http.Request, tenantID, userID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	return req.WithContext(ctx)
}

func sampleTemplate() ScenarioTemplate {
	return ScenarioTemplate{
		ID:       uuid.New(),
		TenantID: uuid.New(),
		Key:      "ops_analysis",
		Name:     "运维分析",
		Spec: map[string]any{
			"roles": []any{map[string]any{"key": "analyst"}},
		},
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestListScenarioTemplatesReturnsRegistry(t *testing.T) {
	handler := NewHandler(&stubService{templates: []ScenarioTemplate{sampleTemplate()}})
	authorizer := &stubAuthorizer{allowed: true}
	handler.SetAuthorizer(authorizer)

	req := identityRequest(httptest.NewRequest(http.MethodGet, "/api/v1/scenario-templates", nil), uuid.New(), uuid.New())
	resp := httptest.NewRecorder()
	handler.ListScenarioTemplates(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0]["template_key"] != "ops_analysis" {
		t.Fatalf("unexpected body: %#v", body)
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionScenarioTemplateRead {
		t.Fatalf("unexpected authz checks: %#v", authorizer.checks)
	}
}

func TestGetScenarioTemplateUnknownKeyReturns404(t *testing.T) {
	handler := NewHandler(&stubService{})
	handler.SetAuthorizer(&stubAuthorizer{allowed: true})

	req := identityRequest(httptest.NewRequest(http.MethodGet, "/api/v1/scenario-templates/nope", nil), uuid.New(), uuid.New())
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("templateKey", "nope")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	resp := httptest.NewRecorder()
	handler.GetScenarioTemplate(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestListScenarioTemplatesForbiddenWithoutAuthz(t *testing.T) {
	handler := NewHandler(&stubService{})
	handler.SetAuthorizer(&stubAuthorizer{allowed: false})

	req := identityRequest(httptest.NewRequest(http.MethodGet, "/api/v1/scenario-templates", nil), uuid.New(), uuid.New())
	resp := httptest.NewRecorder()
	handler.ListScenarioTemplates(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.Code)
	}
}
