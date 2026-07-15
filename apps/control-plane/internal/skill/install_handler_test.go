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
				NodeID:            "node-a",
				ReasonCode:        "runtime_not_connected",
				Message:           "绑定的 Runtime 节点已失活，请先重新 provision 数字员工",
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
			NodeID            string `json:"node_id"`
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
	if body.BlockedTargets[0].NodeID != "node-a" || body.BlockedTargets[0].Message != "绑定的 Runtime 节点已失活，请先重新 provision 数字员工" {
		t.Fatalf("unexpected blocker detail: %#v", body.BlockedTargets[0])
	}
}

func TestListSkillInstallationsHandlerAuthorizesReadAndReturnsRows(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	installationID := uuid.New()
	service := &installHandlerService{
		installations: []SkillInstallation{{
			ID:                    installationID,
			TenantID:              tenantID,
			SkillID:               skillID,
			TargetScope:           SkillInstallTargetEmployee,
			DigitalEmployeeID:     employeeID,
			EmployeeName:          "Review Agent",
			RuntimeNodeID:         uuid.New(),
			NodeID:                "node-a",
			ProviderType:          "codex",
			InstalledPath:         "/home/agent/.agents/skills/review",
			ArchiveChecksumSHA256: "sha256-review",
			InstalledBy:           userID,
			InstalledAt:           time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC),
			Metadata:              map[string]any{"command_id": "cmd-1"},
		}},
	}
	authorizer := &installHandlerAuthorizer{allowed: true}
	handler := NewHandler(service)
	handler.SetAuthorizer(authorizer)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/"+skillID.String()+"/installations", nil)
	req = withSkillRouteContext(req, skillID)
	req = req.WithContext(withInstallConsoleIdentity(req.Context(), tenantID, userID))
	rec := httptest.NewRecorder()

	handler.ListSkillInstallations(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if service.listInstallationsReq.TenantID != tenantID || service.listInstallationsReq.SkillID != skillID {
		t.Fatalf("expected tenant skill request, got %#v", service.listInstallationsReq)
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionSkillRead || authorizer.checks[0].Resource.Type != authz.ResourceSkill || authorizer.checks[0].Resource.ID != skillID.String() {
		t.Fatalf("expected skill read authorization, got %#v", authorizer.checks)
	}
	var body []struct {
		ID                    string         `json:"id"`
		DigitalEmployeeID     string         `json:"digital_employee_id"`
		EmployeeName          string         `json:"employee_name"`
		NodeID                string         `json:"node_id"`
		ProviderType          string         `json:"provider_type"`
		InstalledPath         string         `json:"installed_path"`
		ArchiveChecksumSHA256 string         `json:"archive_checksum_sha256"`
		Metadata              map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 1 || body[0].ID != installationID.String() || body[0].DigitalEmployeeID != employeeID.String() || body[0].EmployeeName != "Review Agent" || body[0].NodeID != "node-a" || body[0].ProviderType != "codex" || body[0].InstalledPath == "" || body[0].ArchiveChecksumSHA256 != "sha256-review" || body[0].Metadata["command_id"] != "cmd-1" {
		t.Fatalf("unexpected installation response: %#v", body)
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

func TestListSkillInstallationsDelegatesToRepository(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	repository := &serviceInstallRepository{
		installations: []SkillInstallation{{
			ID:       uuid.New(),
			TenantID: tenantID,
			SkillID:  skillID,
		}},
	}
	service := NewService(repository, nil)

	installations, err := service.ListSkillInstallations(context.Background(), ListSkillInstallationsRequest{
		TenantID: tenantID,
		SkillID:  skillID,
	})

	if err != nil {
		t.Fatalf("expected repository success, got %v", err)
	}
	if repository.listInstallationsReq.TenantID != tenantID || repository.listInstallationsReq.SkillID != skillID {
		t.Fatalf("repository received wrong request: %#v", repository.listInstallationsReq)
	}
	if len(installations) != 1 || installations[0].SkillID != skillID {
		t.Fatalf("unexpected installations: %#v", installations)
	}
}

type installHandlerService struct {
	installReq           InstallSkillRequest
	listInstallationsReq ListSkillInstallationsRequest
	result               InstallSkillResult
	installations        []SkillInstallation
	installErr           error
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
func (s *installHandlerService) ListSkillInstallations(_ context.Context, req ListSkillInstallationsRequest) ([]SkillInstallation, error) {
	s.listInstallationsReq = req
	return s.installations, nil
}

type installHandlerAuthorizer struct {
	allowed bool
	checks  []authz.CheckRequest
}

func (a *installHandlerAuthorizer) Check(_ context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	return authz.Decision{Allowed: a.allowed}, nil
}

func (a *installHandlerAuthorizer) CheckBulkTeamActions(_ context.Context, req authz.BulkTeamActionsRequest) ([]string, error) {
	if !a.allowed {
		return nil, nil
	}
	return req.Actions, nil
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

type serviceInstallRepository struct {
	listInstallationsReq ListSkillInstallationsRequest
	installations        []SkillInstallation
}

func (r *serviceInstallRepository) ListSkills(context.Context, ListSkillsRequest) ([]*Skill, error) {
	return nil, nil
}
func (r *serviceInstallRepository) GetSkill(context.Context, GetSkillRequest) (*Skill, error) {
	return nil, nil
}
func (r *serviceInstallRepository) UpsertSkillPackage(context.Context, UpsertSkillPackageRequest) (*Skill, error) {
	return nil, nil
}
func (r *serviceInstallRepository) DeleteSkill(context.Context, DeleteSkillRequest) error { return nil }
func (r *serviceInstallRepository) BindSkillToTeam(context.Context, BindTeamSkillRequest) (*Skill, error) {
	return nil, nil
}
func (r *serviceInstallRepository) UnbindSkillFromTeam(context.Context, BindTeamSkillRequest) error {
	return nil
}
func (r *serviceInstallRepository) ListTeamSkills(context.Context, ListTeamSkillsRequest) ([]*Skill, error) {
	return nil, nil
}
func (r *serviceInstallRepository) BindSkillToEmployee(context.Context, BindEmployeeSkillRequest) (*Skill, error) {
	return nil, nil
}
func (r *serviceInstallRepository) UnbindSkillFromEmployee(context.Context, BindEmployeeSkillRequest) error {
	return nil
}
func (r *serviceInstallRepository) ListEffectiveEmployeeSkills(context.Context, ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error) {
	return nil, nil
}
func (r *serviceInstallRepository) ListSkillsForRuntime(context.Context, uuid.UUID, uuid.UUID) ([]SkillRuntimeRecord, error) {
	return nil, nil
}
func (r *serviceInstallRepository) IsSkillBoundToEmployeeTeam(context.Context, BindEmployeeSkillRequest) (bool, error) {
	return false, nil
}
func (r *serviceInstallRepository) DeleteSkillMCPDependencies(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (r *serviceInstallRepository) ListSkillInstallations(_ context.Context, req ListSkillInstallationsRequest) ([]SkillInstallation, error) {
	r.listInstallationsReq = req
	return r.installations, nil
}
