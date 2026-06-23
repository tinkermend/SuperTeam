package skill

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

func TestInstallSkillHandlerParsesEmployeeTargetAndReturnsCreated(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	installationID := uuid.New()
	service := &installHandlerService{
		result: InstallSkillResult{
			SkillID:           skillID,
			TargetScope:       SkillInstallTargetEmployee,
			DigitalEmployeeID: employeeID,
			InstalledCount:    1,
			Installations: []SkillInstallation{{
				ID:                installationID,
				TenantID:          tenantID,
				SkillID:           skillID,
				TargetScope:       SkillInstallTargetEmployee,
				DigitalEmployeeID: employeeID,
				EmployeeName:      "Review Agent",
				RuntimeNodeID:     uuid.New(),
				NodeID:            "node-a",
				ProviderType:      "codex",
				InstalledPath:     "/home/agent/.agents/skills/review",
				InstalledBy:       userID,
				InstalledAt:       time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC),
			}},
		},
	}
	handler := NewHandler(service)
	handler.SetAuthorizer(&installHandlerAuthorizer{allowed: true})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/skills/"+skillID.String()+"/install", strings.NewReader(`{
		"target_scope": "employee",
		"digital_employee_id": "`+employeeID.String()+`",
		"timeout_sec": 12
	}`))
	req = withSkillRouteContext(req, skillID)
	req = req.WithContext(withInstallConsoleIdentity(req.Context(), tenantID, userID))
	rec := httptest.NewRecorder()

	handler.InstallSkill(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if service.installReq.TenantID != tenantID || service.installReq.SkillID != skillID || service.installReq.ActorUserID != userID {
		t.Fatalf("expected tenant skill actor IDs, got %#v", service.installReq)
	}
	if service.installReq.TargetScope != SkillInstallTargetEmployee || service.installReq.DigitalEmployeeID != employeeID {
		t.Fatalf("expected employee target request, got %#v", service.installReq)
	}
	if service.installReq.Timeout != 12*time.Second {
		t.Fatalf("expected 12s timeout, got %s", service.installReq.Timeout)
	}
	var body struct {
		InstalledCount int `json:"installed_count"`
		Installations  []struct {
			ID                string `json:"id"`
			DigitalEmployeeID string `json:"digital_employee_id"`
			ProviderType      string `json:"provider_type"`
			InstalledPath     string `json:"installed_path"`
		} `json:"installations"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.InstalledCount != 1 || len(body.Installations) != 1 || body.Installations[0].ID != installationID.String() || body.Installations[0].DigitalEmployeeID != employeeID.String() || body.Installations[0].ProviderType != "codex" || body.Installations[0].InstalledPath == "" {
		t.Fatalf("unexpected install response: %#v", body)
	}
}

func TestInstallSkillHandlerMapsStructuredInstallErrorToConflict(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	service := &installHandlerService{
		installErr: &InstallSkillError{
			Phase:   InstallFailurePhasePreflight,
			Message: "skill install preflight failed",
			BlockedTargets: []SkillInstallBlockedTarget{{
				DigitalEmployeeID: employeeID,
				EmployeeName:      "Review Agent",
				ProviderType:      "codex",
				ReasonCode:        "runtime_not_connected",
				Message:           "Runtime is not connected",
			}},
		},
	}
	handler := NewHandler(service)
	handler.SetAuthorizer(&installHandlerAuthorizer{allowed: true})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/skills/"+skillID.String()+"/install", strings.NewReader(`{
		"target_scope": "employee",
		"digital_employee_id": "`+employeeID.String()+`"
	}`))
	req = withSkillRouteContext(req, skillID)
	req = req.WithContext(withInstallConsoleIdentity(req.Context(), tenantID, userID))
	rec := httptest.NewRecorder()

	handler.InstallSkill(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error          string `json:"error"`
		Phase          string `json:"phase"`
		Message        string `json:"message"`
		BlockedTargets []struct {
			DigitalEmployeeID string `json:"digital_employee_id"`
			ReasonCode        string `json:"reason_code"`
			Message           string `json:"message"`
		} `json:"blocked_targets"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error != "skill_install_failed" || body.Phase != string(InstallFailurePhasePreflight) || body.Message != "skill install preflight failed" {
		t.Fatalf("unexpected error envelope: %#v", body)
	}
	if len(body.BlockedTargets) != 1 || body.BlockedTargets[0].DigitalEmployeeID != employeeID.String() || body.BlockedTargets[0].ReasonCode != "runtime_not_connected" {
		t.Fatalf("unexpected blockers: %#v", body.BlockedTargets)
	}
}

func TestInstallSkillDelegatesToConfiguredInstaller(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	actorID := uuid.New()
	installer := &serviceInstallDelegate{
		result: InstallSkillResult{
			SkillID:           skillID,
			TargetScope:       SkillInstallTargetEmployee,
			DigitalEmployeeID: employeeID,
			InstalledCount:    1,
		},
	}
	service := NewService(nil, nil)
	service.SetInstallService(installer)

	result, err := service.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID:          tenantID,
		SkillID:           skillID,
		TargetScope:       SkillInstallTargetEmployee,
		DigitalEmployeeID: employeeID,
		ActorUserID:       actorID,
		Timeout:           5 * time.Second,
	})

	if err != nil {
		t.Fatalf("expected delegate success, got %v", err)
	}
	if installer.req.TenantID != tenantID || installer.req.SkillID != skillID || installer.req.DigitalEmployeeID != employeeID || installer.req.ActorUserID != actorID {
		t.Fatalf("delegate received wrong request: %#v", installer.req)
	}
	if result.InstalledCount != 1 || result.SkillID != skillID {
		t.Fatalf("unexpected result: %#v", result)
	}
}

type installHandlerService struct {
	installReq InstallSkillRequest
	result     InstallSkillResult
	installErr error
}

func (s *installHandlerService) ListSkills(context.Context, ListSkillsRequest) ([]*Skill, error) {
	return nil, nil
}
func (s *installHandlerService) GetSkill(context.Context, GetSkillRequest) (*Skill, error) {
	return nil, nil
}
func (s *installHandlerService) UploadSkill(context.Context, UploadSkillRequest) (*Skill, error) {
	return nil, nil
}
func (s *installHandlerService) DeleteSkill(context.Context, DeleteSkillRequest) error { return nil }
func (s *installHandlerService) BindSkillToTeam(context.Context, BindTeamSkillRequest) (*Skill, error) {
	return nil, nil
}
func (s *installHandlerService) UnbindSkillFromTeam(context.Context, BindTeamSkillRequest) error {
	return nil
}
func (s *installHandlerService) ListTeamSkills(context.Context, ListTeamSkillsRequest) ([]*Skill, error) {
	return nil, nil
}
func (s *installHandlerService) BindSkillToEmployee(context.Context, BindEmployeeSkillRequest) (*Skill, error) {
	return nil, nil
}
func (s *installHandlerService) UnbindSkillFromEmployee(context.Context, BindEmployeeSkillRequest) error {
	return nil
}
func (s *installHandlerService) ListEffectiveEmployeeSkills(context.Context, ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error) {
	return nil, nil
}
func (s *installHandlerService) InstallSkill(_ context.Context, req InstallSkillRequest) (InstallSkillResult, error) {
	s.installReq = req
	return s.result, s.installErr
}

type installHandlerAuthorizer struct {
	allowed bool
	checks  []authz.CheckRequest
}

func (a *installHandlerAuthorizer) Check(_ context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	return authz.Decision{Allowed: a.allowed}, nil
}

func withSkillRouteContext(req *http.Request, skillID uuid.UUID) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("skillId", skillID.String())
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func withInstallConsoleIdentity(ctx context.Context, tenantID, userID uuid.UUID) context.Context {
	ctx = context.WithValue(ctx, middleware.TenantIDKey, tenantID)
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	return ctx
}

type serviceInstallDelegate struct {
	req    InstallSkillRequest
	result InstallSkillResult
	err    error
}

func (d *serviceInstallDelegate) InstallSkill(_ context.Context, req InstallSkillRequest) (InstallSkillResult, error) {
	d.req = req
	return d.result, d.err
}
