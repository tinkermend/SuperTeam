package scenariotemplate

import (
	"context"
	"encoding/json"
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

func (s *stubService) Create(_ context.Context, req CreateScenarioTemplateRequest) (ScenarioTemplate, error) {
	template := ScenarioTemplate{Key: req.Key, Name: req.Name, Description: req.Description, Spec: req.Spec, Status: "active", ActiveVersion: 1}
	s.templates = append(s.templates, template)
	return template, nil
}

func (s *stubService) CreateVersion(_ context.Context, req CreateScenarioTemplateVersionRequest) (ScenarioTemplate, error) {
	return ScenarioTemplate{Key: req.Key, Spec: req.Spec, Status: "active", ActiveVersion: 2}, nil
}

func (s *stubService) ListVersions(_ context.Context, _ uuid.UUID, _ string) ([]ScenarioTemplateVersion, error) {
	return nil, nil
}

func (s *stubService) Patch(_ context.Context, req PatchScenarioTemplateRequest) (ScenarioTemplate, error) {
	for _, template := range s.templates {
		if template.Key == req.Key {
			return template, nil
		}
	}
	return ScenarioTemplate{}, ErrScenarioTemplateNotFound
}

func (s *stubService) RoleView(_ context.Context, _ uuid.UUID, key string) (RoleView, error) {
	for _, template := range s.templates {
		if template.Key == key {
			return RoleView{TemplateKey: template.Key, Name: template.Name, Roles: []RoleViewRole{}, Exits: []RoleViewExit{}}, nil
		}
	}
	return RoleView{}, ErrScenarioTemplateNotFound
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

func TestCreateScenarioTemplateForbiddenWithoutManageAuthz(t *testing.T) {
	handler := NewHandler(&stubService{})
	authorizer := &stubAuthorizer{allowed: false}
	handler.SetAuthorizer(authorizer)

	body := `{"template_key":"ops_review","name":"运维评审","spec":{}}`
	req := identityRequest(httptest.NewRequest(http.MethodPost, "/api/v1/scenario-templates", strings.NewReader(body)), uuid.New(), uuid.New())
	resp := httptest.NewRecorder()
	handler.CreateScenarioTemplate(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionScenarioTemplateManage {
		t.Fatalf("unexpected authz checks: %#v", authorizer.checks)
	}
}

func TestCreateScenarioTemplateVersionForbiddenWithoutManageAuthz(t *testing.T) {
	handler := NewHandler(&stubService{})
	authorizer := &stubAuthorizer{allowed: false}
	handler.SetAuthorizer(authorizer)

	body := `{"spec":{}}`
	req := identityRequest(httptest.NewRequest(http.MethodPost, "/api/v1/scenario-templates/ops_review/versions", strings.NewReader(body)), uuid.New(), uuid.New())
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("templateKey", "ops_review")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	resp := httptest.NewRecorder()
	handler.CreateScenarioTemplateVersion(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionScenarioTemplateManage {
		t.Fatalf("unexpected authz checks: %#v", authorizer.checks)
	}
}

func TestPatchScenarioTemplateForbiddenWithoutManageAuthz(t *testing.T) {
	handler := NewHandler(&stubService{})
	authorizer := &stubAuthorizer{allowed: false}
	handler.SetAuthorizer(authorizer)

	body := `{"status":"disabled"}`
	req := identityRequest(httptest.NewRequest(http.MethodPatch, "/api/v1/scenario-templates/ops_review", strings.NewReader(body)), uuid.New(), uuid.New())
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("templateKey", "ops_review")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	resp := httptest.NewRecorder()
	handler.PatchScenarioTemplate(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionScenarioTemplateManage {
		t.Fatalf("unexpected authz checks: %#v", authorizer.checks)
	}
}

func TestCreateScenarioTemplateSuccess(t *testing.T) {
	handler := NewHandler(&stubService{})
	handler.SetAuthorizer(&stubAuthorizer{allowed: true})

	body := `{"template_key":"ops_review","name":"运维评审","description":"desc","spec":{"spec_version":2,"roles":[]}}`
	req := identityRequest(httptest.NewRequest(http.MethodPost, "/api/v1/scenario-templates", strings.NewReader(body)), uuid.New(), uuid.New())
	resp := httptest.NewRecorder()
	handler.CreateScenarioTemplate(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["template_key"] != "ops_review" {
		t.Fatalf("unexpected body: %#v", out)
	}
}
