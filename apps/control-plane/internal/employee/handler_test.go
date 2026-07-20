package employee

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
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/authz"
)

// fakeTemplateHandlerService implements HandlerService for template_handler.go
// tests. It only needs meaningful behavior for the six employee-template
// methods; every other HandlerService method is a stub since the tests in
// this file never exercise the pre-existing digital-employee handlers.
type fakeTemplateHandlerService struct {
	templates []EmployeeTemplateRecord
	template  EmployeeTemplateRecord
	err       error

	activityBatches     [][]DigitalEmployeeActivityItem
	activityCall        int
	lastActivityRequest GetDigitalEmployeeActivityRequest

	listTenantID uuid.UUID

	getTenantID   uuid.UUID
	getTemplateID uuid.UUID

	createParams CreateEmployeeTemplateParams
	updateParams UpdateEmployeeTemplateParams

	statusTenantID   uuid.UUID
	statusTemplateID uuid.UUID
	statusValue      string

	deleteTenantID   uuid.UUID
	deleteTemplateID uuid.UUID
}

func (s *fakeTemplateHandlerService) GetCreateOptions(ctx context.Context, req CreateOptionsRequest) (*CreateOptions, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) ListAvatarAssets(ctx context.Context, tenantID uuid.UUID) ([]DigitalEmployeeAvatarAsset, error) {
	return ListDigitalEmployeeAvatarAssets(), nil
}


func (s *fakeTemplateHandlerService) ListDigitalEmployees(ctx context.Context, req ListDigitalEmployeesRequest) ([]*DigitalEmployee, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) DeleteDigitalEmployee(ctx context.Context, req DeleteDigitalEmployeeRequest) error {
	return nil
}

func (s *fakeTemplateHandlerService) GetOverview(ctx context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) GetActivity(ctx context.Context, req GetDigitalEmployeeActivityRequest) (*DigitalEmployeeActivity, error) {
	s.lastActivityRequest = req
	if s.activityCall < len(s.activityBatches) {
		batch := s.activityBatches[s.activityCall]
		s.activityCall++
		activity := &DigitalEmployeeActivity{Items: batch}
		if len(batch) > 0 && batch[0].OccurredAt != nil {
			activity.NextSince = encodeActivityCursor(*batch[0].OccurredAt, batch[0].EventID)
		}
		return activity, nil
	}
	return &DigitalEmployeeActivity{Items: []DigitalEmployeeActivityItem{}}, nil
}

func (s *fakeTemplateHandlerService) ListEnvironmentVariables(ctx context.Context, req ListEnvironmentVariablesRequest) ([]EnvironmentVariableSummary, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) UpsertEnvironmentVariable(ctx context.Context, req UpsertEnvironmentVariableRequest) (EnvironmentVariableSummary, error) {
	return EnvironmentVariableSummary{}, nil
}

func (s *fakeTemplateHandlerService) DeleteEnvironmentVariable(ctx context.Context, req DeleteEnvironmentVariableRequest) error {
	return nil
}

func (s *fakeTemplateHandlerService) GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployee, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) UpdateStatus(ctx context.Context, req UpdateStatusRequest) (*DigitalEmployee, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) ReassignTeam(ctx context.Context, req ReassignDigitalEmployeeTeamRequest) (*DigitalEmployee, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) GetExecutionInstance(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeExecutionInstance, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) BindExecutionInstance(ctx context.Context, req BindExecutionInstanceRequest) (*DigitalEmployeeExecutionInstance, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) CreateConfigRevision(ctx context.Context, req CreateDigitalEmployeeConfigRevisionRequest) (*DigitalEmployeeConfigRevision, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) SubmitPermissionChange(ctx context.Context, req SubmitPermissionChangeRequest) (*approval.ApprovalRequest, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) GetSchedulingReadiness(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeSchedulingReadiness, error) {
	return nil, nil
}

func (s *fakeTemplateHandlerService) ListEmployeeTemplates(ctx context.Context, tenantID uuid.UUID) ([]EmployeeTemplateRecord, error) {
	s.listTenantID = tenantID
	if s.err != nil {
		return nil, s.err
	}
	return s.templates, nil
}

func (s *fakeTemplateHandlerService) GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error) {
	s.getTenantID = tenantID
	s.getTemplateID = templateID
	if s.err != nil {
		return EmployeeTemplateRecord{}, s.err
	}
	return s.template, nil
}

func (s *fakeTemplateHandlerService) CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	s.createParams = params
	if s.err != nil {
		return EmployeeTemplateRecord{}, s.err
	}
	return s.template, nil
}

func (s *fakeTemplateHandlerService) UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	s.updateParams = params
	if s.err != nil {
		return EmployeeTemplateRecord{}, s.err
	}
	return s.template, nil
}

func (s *fakeTemplateHandlerService) SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error) {
	s.statusTenantID = tenantID
	s.statusTemplateID = templateID
	s.statusValue = status
	if s.err != nil {
		return EmployeeTemplateRecord{}, s.err
	}
	return s.template, nil
}

func (s *fakeTemplateHandlerService) DeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error {
	s.deleteTenantID = tenantID
	s.deleteTemplateID = templateID
	return s.err
}

// templateHandlerAuthorizer is a minimal authz.Authorizer test double,
// mirroring the allowed/denied fake-authorizer convention already used by
// other handler test files in this codebase (see e.g.
// internal/capability/handler_test.go's handlerAuthorizer and
// internal/auth/handler_test.go's recordingAuthorizer).
type templateHandlerAuthorizer struct {
	allowed bool
}

func (a *templateHandlerAuthorizer) Check(_ context.Context, _ authz.CheckRequest) (authz.Decision, error) {
	if a.allowed {
		return authz.Decision{Allowed: true, Reason: authz.ReasonAllowed}, nil
	}
	return authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership}, nil
}

func (a *templateHandlerAuthorizer) CheckBulkTeamActions(_ context.Context, req authz.BulkTeamActionsRequest) ([]string, error) {
	return req.Actions, nil
}

func newTemplateTestHandler(service HandlerService, allowed bool) *HTTPHandler {
	handler := NewHandler(service)
	handler.SetAuthorizer(&templateHandlerAuthorizer{allowed: allowed})
	return handler
}

func templateTestRequest(method, target string, body string, tenantID, userID uuid.UUID, routeParams map[string]string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	if len(routeParams) > 0 {
		routeCtx := chi.NewRouteContext()
		for key, value := range routeParams {
			routeCtx.URLParams.Add(key, value)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, routeCtx)
	}
	return req.WithContext(ctx)
}

func TestEmployeeTemplateHandlerListReturns200WithSerializedList(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	templateID := uuid.New()
	now := time.Now().UTC()
	service := &fakeTemplateHandlerService{
		templates: []EmployeeTemplateRecord{
			{
				ID:        templateID,
				TenantID:  tenantID,
				Type:      "sales_ops",
				Label:     "销售运营",
				Status:    "active",
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(http.MethodGet, "/api/v1/digital-employee-templates", "", tenantID, userID, nil)
	resp := httptest.NewRecorder()

	handler.ListEmployeeTemplates(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.listTenantID != tenantID {
		t.Fatalf("expected tenant %s forwarded to service, got %s", tenantID, service.listTenantID)
	}
	var body []employeeTemplateResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 1 || body[0].ID != templateID.String() || body[0].Label != "销售运营" {
		t.Fatalf("unexpected serialized list: %#v", body)
	}
}

func TestEmployeeTemplateHandlerCreateMissingLabelReturns400(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	service := &fakeTemplateHandlerService{err: ErrInvalidInput}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(http.MethodPost, "/api/v1/digital-employee-templates", `{"type":"sales_ops","label":""}`, tenantID, userID, nil)
	resp := httptest.NewRecorder()

	handler.CreateEmployeeTemplate(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.createParams.TenantID != tenantID {
		t.Fatalf("expected tenant forwarded to service, got %#v", service.createParams)
	}
}

func TestEmployeeTemplateHandlerDeleteUnknownTemplateReturns404(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	templateID := uuid.New()
	service := &fakeTemplateHandlerService{err: ErrNotFound}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(http.MethodDelete, "/api/v1/digital-employee-templates/"+templateID.String(), "", tenantID, userID, map[string]string{"templateId": templateID.String()})
	resp := httptest.NewRecorder()

	handler.DeleteEmployeeTemplate(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.deleteTenantID != tenantID || service.deleteTemplateID != templateID {
		t.Fatalf("unexpected delete forwarding: tenant=%s template=%s", service.deleteTenantID, service.deleteTemplateID)
	}
}

func TestEmployeeTemplateHandlerDeniedReturns403(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	service := &fakeTemplateHandlerService{}
	handler := newTemplateTestHandler(service, false)
	req := templateTestRequest(http.MethodGet, "/api/v1/digital-employee-templates", "", tenantID, userID, nil)
	resp := httptest.NewRecorder()

	handler.ListEmployeeTemplates(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestEmployeeTemplateHandlerGetInvalidTemplateIDReturns400(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	service := &fakeTemplateHandlerService{}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(http.MethodGet, "/api/v1/digital-employee-templates/not-a-uuid", "", tenantID, userID, map[string]string{"templateId": "not-a-uuid"})
	resp := httptest.NewRecorder()

	handler.GetEmployeeTemplate(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestEmployeeTemplateHandlerCreateReturns201(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	templateID := uuid.New()
	now := time.Now().UTC()
	service := &fakeTemplateHandlerService{
		template: EmployeeTemplateRecord{ID: templateID, TenantID: tenantID, Type: "sales_ops", Label: "销售运营", Status: "active", CreatedAt: now, UpdatedAt: now},
	}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(http.MethodPost, "/api/v1/digital-employee-templates", `{"type":"sales_ops","label":"销售运营"}`, tenantID, userID, nil)
	resp := httptest.NewRecorder()

	handler.CreateEmployeeTemplate(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.createParams.Type != "sales_ops" || service.createParams.Label != "销售运营" {
		t.Fatalf("unexpected create params: %#v", service.createParams)
	}
}

func TestEmployeeTemplateHandlerCreateRejectsLegacyTemplateFields(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	service := &fakeTemplateHandlerService{}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(
		http.MethodPost,
		"/api/v1/digital-employee-templates",
		`{"type":"sales_ops","label":"销售运营","default_capability_selection":{"enabled_skills":["sql-review"]}}`,
		tenantID,
		userID,
		nil,
	)
	resp := httptest.NewRecorder()

	handler.CreateEmployeeTemplate(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "default_capability_selection is no longer supported") {
		t.Fatalf("expected legacy field rejection, got %q", resp.Body.String())
	}
}

func TestEmployeeTemplateHandlerUpdateReturns200(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	templateID := uuid.New()
	now := time.Now().UTC()
	service := &fakeTemplateHandlerService{
		template: EmployeeTemplateRecord{ID: templateID, TenantID: tenantID, Label: "更新后的标签", Status: "active", CreatedAt: now, UpdatedAt: now},
	}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(http.MethodPatch, "/api/v1/digital-employee-templates/"+templateID.String(), `{"label":"更新后的标签"}`, tenantID, userID, map[string]string{"templateId": templateID.String()})
	resp := httptest.NewRecorder()

	handler.UpdateEmployeeTemplate(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.updateParams.ID != templateID || service.updateParams.Label != "更新后的标签" {
		t.Fatalf("unexpected update params: %#v", service.updateParams)
	}
}

func TestEmployeeTemplateHandlerUpdateRejectsLegacyTemplateFields(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	templateID := uuid.New()
	service := &fakeTemplateHandlerService{}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(
		http.MethodPatch,
		"/api/v1/digital-employee-templates/"+templateID.String(),
		`{"label":"更新后的标签","default_approval_policy":{"min_risk_for_human":"high"}}`,
		tenantID,
		userID,
		map[string]string{"templateId": templateID.String()},
	)
	resp := httptest.NewRecorder()

	handler.UpdateEmployeeTemplate(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "default_approval_policy is no longer supported") {
		t.Fatalf("expected legacy field rejection, got %q", resp.Body.String())
	}
}

func TestEmployeeTemplateHandlerSetStatusReturns200(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	templateID := uuid.New()
	now := time.Now().UTC()
	service := &fakeTemplateHandlerService{
		template: EmployeeTemplateRecord{ID: templateID, TenantID: tenantID, Status: "archived", CreatedAt: now, UpdatedAt: now},
	}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(http.MethodPatch, "/api/v1/digital-employee-templates/"+templateID.String()+"/status", `{"status":"archived"}`, tenantID, userID, map[string]string{"templateId": templateID.String()})
	resp := httptest.NewRecorder()

	handler.SetEmployeeTemplateStatus(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.statusTemplateID != templateID || service.statusValue != "archived" {
		t.Fatalf("unexpected status params: template=%s status=%s", service.statusTemplateID, service.statusValue)
	}
}

func TestEmployeeTemplateHandlerDeleteReturns204(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	templateID := uuid.New()
	service := &fakeTemplateHandlerService{}
	handler := newTemplateTestHandler(service, true)
	req := templateTestRequest(http.MethodDelete, "/api/v1/digital-employee-templates/"+templateID.String(), "", tenantID, userID, map[string]string{"templateId": templateID.String()})
	resp := httptest.NewRecorder()

	handler.DeleteEmployeeTemplate(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body.String())
	}
}
