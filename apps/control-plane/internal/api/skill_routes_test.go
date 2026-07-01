package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/platform"
	"github.com/superteam/control-plane/internal/skill"
)

func TestSkillRoutesUseConsoleTenantAndMultipartUpload(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeSkillService{}
	authorizer := &routeAuthorizer{allowed: true}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	server.SetSkillHandler(skill.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	expectedTenantID := platform.DefaultTenantID

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list skills to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listReq.TenantID != expectedTenantID {
		t.Fatalf("expected list tenant %s, got %s", expectedTenantID, service.listReq.TenantID)
	}
	var listed []struct {
		Name             string   `json:"name"`
		Tags             []string `json:"tags"`
		ArchiveObjectRef string   `json:"archive_object_ref"`
		ArchiveFileCount int      `json:"archive_file_count"`
		AgentBindings    []struct {
			AgentName string `json:"agent_name"`
		} `json:"agent_bindings"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list skills: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "diagnose" || listed[0].Tags[0] != "诊断" || listed[0].ArchiveObjectRef != "s3://bucket/skills/diagnose.zip" || listed[0].ArchiveFileCount != 3 || listed[0].AgentBindings[0].AgentName != "需求澄清 Agent" {
		t.Fatalf("expected skill response with tags, archive metadata and agent binding, got %#v", listed)
	}

	teamID := uuid.New()
	body, contentType := buildSkillUploadMultipart(t, map[string]string{
		"name":          "diagnose-plus",
		"description":   "上传诊断技能",
		"tags":          "诊断,自动化",
		"team_ids":      teamID.String(),
		"runtime_tools": "gh,kubectl",
		"runtime_env":   "GH_TOKEN",
	}, map[string]string{
		"diagnose-plus/SKILL.md":            "# diagnose-plus\n",
		"diagnose-plus/scripts/collect.py":  "print('collect')\n",
		"diagnose-plus/references/check.md": "检查项\n",
	})
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v1/skills/uploads", bytes.NewReader(body))
	uploadReq.Header.Set("Content-Type", contentType)
	uploadReq.AddCookie(cookie)
	uploadResp := httptest.NewRecorder()
	server.ServeHTTP(uploadResp, uploadReq)
	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("expected upload skill to succeed, got %d: %s", uploadResp.Code, uploadResp.Body.String())
	}
	if service.uploadReq.TenantID != expectedTenantID || service.uploadReq.ActorUserID != user.ID || service.uploadReq.Name != "diagnose-plus" {
		t.Fatalf("expected upload tenant actor and name, got %#v", service.uploadReq)
	}
	if !stringSlicesEqual(service.uploadReq.Tags, []string{"诊断", "自动化"}) {
		t.Fatalf("expected parsed upload tags, got %#v", service.uploadReq.Tags)
	}
	if len(service.uploadReq.TeamIDs) != 1 || service.uploadReq.TeamIDs[0] != teamID {
		t.Fatalf("expected parsed team binding, got %#v", service.uploadReq.TeamIDs)
	}
	if len(service.uploadReq.Archive) == 0 || service.uploadReq.Filename != "skill.zip" {
		t.Fatalf("expected uploaded archive bytes and filename, got filename=%q size=%d", service.uploadReq.Filename, len(service.uploadReq.Archive))
	}
	if !stringSlicesEqual(service.uploadReq.RuntimeDependencies.Tools, []string{"gh", "kubectl"}) {
		t.Fatalf("expected parsed runtime tools, got %#v", service.uploadReq.RuntimeDependencies.Tools)
	}
	if !stringSlicesEqual(service.uploadReq.RuntimeDependencies.Env, []string{"GH_TOKEN"}) {
		t.Fatalf("expected parsed runtime env, got %#v", service.uploadReq.RuntimeDependencies.Env)
	}
	var uploadBody struct {
		RuntimeDependencies skill.SkillRuntimeDependencies `json:"runtime_dependencies"`
	}
	if err := json.NewDecoder(uploadResp.Body).Decode(&uploadBody); err != nil {
		t.Fatalf("decode upload skill response: %v", err)
	}
	if !stringSlicesEqual(uploadBody.RuntimeDependencies.Tools, []string{"gh", "kubectl"}) {
		t.Fatalf("response tools mismatch: %#v", uploadBody.RuntimeDependencies.Tools)
	}

	routeTeamID := uuid.New()
	teamListReq := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+routeTeamID.String()+"/skills", nil)
	teamListReq.AddCookie(cookie)
	teamListResp := httptest.NewRecorder()
	server.ServeHTTP(teamListResp, teamListReq)
	if teamListResp.Code != http.StatusOK {
		t.Fatalf("expected list team skills to succeed, got %d: %s", teamListResp.Code, teamListResp.Body.String())
	}
	if service.teamListReq.TenantID != expectedTenantID || service.teamListReq.TeamID != routeTeamID {
		t.Fatalf("expected list team skills tenant/team, got %#v", service.teamListReq)
	}
	var teamSkills []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(teamListResp.Body).Decode(&teamSkills); err != nil {
		t.Fatalf("decode team skills: %v", err)
	}
	if len(teamSkills) != 1 || teamSkills[0].Name != "team-diagnose" {
		t.Fatalf("expected team skill response, got %#v", teamSkills)
	}
	assertLastSkillAuthzCheck(t, authorizer, authz.ActionTeamRead, authz.ResourceTeam, routeTeamID.String(), expectedTenantID, &routeTeamID)

	bindSkillID := uuid.New()
	teamBindReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+routeTeamID.String()+"/skills", strings.NewReader(`{"skill_id":"`+bindSkillID.String()+`"}`))
	teamBindReq.Header.Set("Content-Type", "application/json")
	teamBindReq.AddCookie(cookie)
	teamBindResp := httptest.NewRecorder()
	server.ServeHTTP(teamBindResp, teamBindReq)
	if teamBindResp.Code != http.StatusCreated {
		t.Fatalf("expected bind team skill to succeed, got %d: %s", teamBindResp.Code, teamBindResp.Body.String())
	}
	if service.teamBindReq.TenantID != expectedTenantID || service.teamBindReq.TeamID != routeTeamID || service.teamBindReq.SkillID != bindSkillID {
		t.Fatalf("expected bind team skill request, got %#v", service.teamBindReq)
	}
	var teamBindBody struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(teamBindResp.Body).Decode(&teamBindBody); err != nil {
		t.Fatalf("decode team bind skill response: %v", err)
	}
	if teamBindBody.ID != bindSkillID.String() || teamBindBody.Name != "bound-team-skill" {
		t.Fatalf("expected bound team skill response, got %#v", teamBindBody)
	}
	assertLastSkillAuthzCheck(t, authorizer, authz.ActionTeamCapabilityBind, authz.ResourceTeam, routeTeamID.String(), expectedTenantID, &routeTeamID)

	teamUnbindReq := httptest.NewRequest(http.MethodDelete, "/api/v1/teams/"+routeTeamID.String()+"/skills/"+bindSkillID.String(), nil)
	teamUnbindReq.AddCookie(cookie)
	teamUnbindResp := httptest.NewRecorder()
	server.ServeHTTP(teamUnbindResp, teamUnbindReq)
	if teamUnbindResp.Code != http.StatusNoContent {
		t.Fatalf("expected unbind team skill to succeed, got %d: %s", teamUnbindResp.Code, teamUnbindResp.Body.String())
	}
	if service.teamUnbindReq.TenantID != expectedTenantID || service.teamUnbindReq.TeamID != routeTeamID || service.teamUnbindReq.SkillID != bindSkillID {
		t.Fatalf("expected unbind team skill request, got %#v", service.teamUnbindReq)
	}
	assertLastSkillAuthzCheck(t, authorizer, authz.ActionTeamCapabilityUnbind, authz.ResourceTeam, routeTeamID.String(), expectedTenantID, &routeTeamID)

	employeeID := uuid.New()
	employeeListReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/skills", nil)
	employeeListReq.AddCookie(cookie)
	employeeListResp := httptest.NewRecorder()
	server.ServeHTTP(employeeListResp, employeeListReq)
	if employeeListResp.Code != http.StatusOK {
		t.Fatalf("expected list effective employee skills to succeed, got %d: %s", employeeListResp.Code, employeeListResp.Body.String())
	}
	if service.effectiveListReq.TenantID != expectedTenantID || service.effectiveListReq.DigitalEmployeeID != employeeID {
		t.Fatalf("expected effective skill list request, got %#v", service.effectiveListReq)
	}
	var effectiveSkills []struct {
		Skill struct {
			Name string `json:"name"`
		} `json:"skill"`
		SourceScope string `json:"source_scope"`
		Inherited   bool   `json:"inherited"`
		ReadOnly    bool   `json:"read_only"`
	}
	if err := json.NewDecoder(employeeListResp.Body).Decode(&effectiveSkills); err != nil {
		t.Fatalf("decode effective employee skills: %v", err)
	}
	if len(effectiveSkills) != 2 || effectiveSkills[0].Skill.Name != "team-diagnose" || effectiveSkills[0].SourceScope != "team" || !effectiveSkills[0].Inherited || !effectiveSkills[0].ReadOnly || effectiveSkills[1].SourceScope != "employee" {
		t.Fatalf("expected effective employee skill response with inherited team first, got %#v", effectiveSkills)
	}
	assertLastSkillAuthzCheck(t, authorizer, authz.ActionEmployeeRead, authz.ResourceEmployee, employeeID.String(), expectedTenantID, nil)

	employeeBindReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+employeeID.String()+"/skills", strings.NewReader(`{"skill_id":"`+bindSkillID.String()+`"}`))
	employeeBindReq.Header.Set("Content-Type", "application/json")
	employeeBindReq.AddCookie(cookie)
	employeeBindResp := httptest.NewRecorder()
	server.ServeHTTP(employeeBindResp, employeeBindReq)
	if employeeBindResp.Code != http.StatusCreated {
		t.Fatalf("expected bind employee skill to succeed, got %d: %s", employeeBindResp.Code, employeeBindResp.Body.String())
	}
	if service.employeeBindReq.TenantID != expectedTenantID || service.employeeBindReq.DigitalEmployeeID != employeeID || service.employeeBindReq.SkillID != bindSkillID {
		t.Fatalf("expected bind employee skill request, got %#v", service.employeeBindReq)
	}
	var employeeBindBody struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(employeeBindResp.Body).Decode(&employeeBindBody); err != nil {
		t.Fatalf("decode employee bind skill response: %v", err)
	}
	if employeeBindBody.ID != bindSkillID.String() || employeeBindBody.Name != "bound-employee-skill" {
		t.Fatalf("expected bound employee skill response, got %#v", employeeBindBody)
	}
	assertLastSkillAuthzCheck(t, authorizer, authz.ActionEmployeeConfigCreate, authz.ResourceEmployee, employeeID.String(), expectedTenantID, nil)

	installSkillID := uuid.New()
	installEmployeeID := uuid.New()
	installReq := httptest.NewRequest(http.MethodPost, "/api/v1/skills/"+installSkillID.String()+"/install", strings.NewReader(`{"target_scope":"employee","digital_employee_id":"`+installEmployeeID.String()+`","timeout_sec":7}`))
	installReq.Header.Set("Content-Type", "application/json")
	installReq.AddCookie(cookie)
	installResp := httptest.NewRecorder()
	server.ServeHTTP(installResp, installReq)
	if installResp.Code != http.StatusCreated {
		t.Fatalf("expected install skill to succeed, got %d: %s", installResp.Code, installResp.Body.String())
	}
	if service.installReq.TenantID != expectedTenantID || service.installReq.SkillID != installSkillID || service.installReq.DigitalEmployeeID != installEmployeeID || service.installReq.ActorUserID != user.ID || service.installReq.Timeout != 7_000_000_000 {
		t.Fatalf("expected install skill request, got %#v", service.installReq)
	}
	assertLastSkillAuthzCheck(t, authorizer, authz.ActionSkillInstall, authz.ResourceSkill, installSkillID.String(), expectedTenantID, nil)

	listInstallationsReq := httptest.NewRequest(http.MethodGet, "/api/v1/skills/"+installSkillID.String()+"/installations", nil)
	listInstallationsReq.AddCookie(cookie)
	listInstallationsResp := httptest.NewRecorder()
	server.ServeHTTP(listInstallationsResp, listInstallationsReq)
	if listInstallationsResp.Code != http.StatusOK {
		t.Fatalf("expected list skill installations to succeed, got %d: %s", listInstallationsResp.Code, listInstallationsResp.Body.String())
	}
	if service.listInstallationsReq.TenantID != expectedTenantID || service.listInstallationsReq.SkillID != installSkillID {
		t.Fatalf("expected list skill installations request, got %#v", service.listInstallationsReq)
	}
	var installationRows []struct {
		SkillID           string `json:"skill_id"`
		DigitalEmployeeID string `json:"digital_employee_id"`
		EmployeeName      string `json:"employee_name"`
		ProviderType      string `json:"provider_type"`
		InstalledPath     string `json:"installed_path"`
	}
	if err := json.NewDecoder(listInstallationsResp.Body).Decode(&installationRows); err != nil {
		t.Fatalf("decode skill installations: %v", err)
	}
	if len(installationRows) != 1 || installationRows[0].SkillID != installSkillID.String() || installationRows[0].EmployeeName != "需求澄清 Agent" || installationRows[0].ProviderType != "codex" || installationRows[0].InstalledPath == "" {
		t.Fatalf("expected installation row response, got %#v", installationRows)
	}
	assertLastSkillAuthzCheck(t, authorizer, authz.ActionSkillRead, authz.ResourceSkill, installSkillID.String(), expectedTenantID, nil)

	deniedService := &routeSkillService{}
	deniedAuthorizer := &routeAuthorizer{allowed: true, denyActions: map[string]bool{authz.ActionSkillInstall: true}}
	deniedServer := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		deniedAuthorizer,
	)
	deniedServer.SetSkillHandler(skill.NewHandler(deniedService))
	deniedReq := httptest.NewRequest(http.MethodPost, "/api/v1/skills/"+installSkillID.String()+"/install", strings.NewReader(`{"target_scope":"employee","digital_employee_id":"`+installEmployeeID.String()+`"}`))
	deniedReq.Header.Set("Content-Type", "application/json")
	deniedReq.AddCookie(cookie)
	deniedResp := httptest.NewRecorder()
	deniedServer.ServeHTTP(deniedResp, deniedReq)
	if deniedResp.Code != http.StatusForbidden {
		t.Fatalf("expected denied install to return 403, got %d: %s", deniedResp.Code, deniedResp.Body.String())
	}
	if deniedService.installCalled {
		t.Fatal("expected denied install route not to call service")
	}
	assertLastSkillAuthzCheck(t, deniedAuthorizer, authz.ActionSkillInstall, authz.ResourceSkill, installSkillID.String(), expectedTenantID, nil)

	deniedListService := &routeSkillService{}
	deniedListAuthorizer := &routeAuthorizer{allowed: true, denyActions: map[string]bool{authz.ActionSkillRead: true}}
	deniedListServer := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		deniedListAuthorizer,
	)
	deniedListServer.SetSkillHandler(skill.NewHandler(deniedListService))
	deniedListReq := httptest.NewRequest(http.MethodGet, "/api/v1/skills/"+installSkillID.String()+"/installations", nil)
	deniedListReq.AddCookie(cookie)
	deniedListResp := httptest.NewRecorder()
	deniedListServer.ServeHTTP(deniedListResp, deniedListReq)
	if deniedListResp.Code != http.StatusForbidden {
		t.Fatalf("expected denied installation list to return 403, got %d: %s", deniedListResp.Code, deniedListResp.Body.String())
	}
	if deniedListService.listInstallationsCalled {
		t.Fatal("expected denied installation list route not to call service")
	}
	assertLastSkillAuthzCheck(t, deniedListAuthorizer, authz.ActionSkillRead, authz.ResourceSkill, installSkillID.String(), expectedTenantID, nil)

	employeeUnbindReq := httptest.NewRequest(http.MethodDelete, "/api/v1/digital-employees/"+employeeID.String()+"/skills/"+bindSkillID.String(), nil)
	employeeUnbindReq.AddCookie(cookie)
	employeeUnbindResp := httptest.NewRecorder()
	server.ServeHTTP(employeeUnbindResp, employeeUnbindReq)
	if employeeUnbindResp.Code != http.StatusNoContent {
		t.Fatalf("expected unbind employee skill to succeed, got %d: %s", employeeUnbindResp.Code, employeeUnbindResp.Body.String())
	}
	if service.employeeUnbindReq.TenantID != expectedTenantID || service.employeeUnbindReq.DigitalEmployeeID != employeeID || service.employeeUnbindReq.SkillID != bindSkillID {
		t.Fatalf("expected unbind employee skill request, got %#v", service.employeeUnbindReq)
	}
	assertLastSkillAuthzCheck(t, authorizer, authz.ActionEmployeeConfigCreate, authz.ResourceEmployee, employeeID.String(), expectedTenantID, nil)
}

func TestSkillBindRoutesPropagateMissingTargets(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeSkillService{
		teamBindErr:     skill.ErrNotFound,
		employeeBindErr: skill.ErrNotFound,
	}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetSkillHandler(skill.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	bindSkillID := uuid.New()

	teamID := uuid.New()
	teamBindReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+teamID.String()+"/skills", strings.NewReader(`{"skill_id":"`+bindSkillID.String()+`"}`))
	teamBindReq.Header.Set("Content-Type", "application/json")
	teamBindReq.AddCookie(cookie)
	teamBindResp := httptest.NewRecorder()
	server.ServeHTTP(teamBindResp, teamBindReq)
	if teamBindResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing team bind target to return 404, got %d: %s", teamBindResp.Code, teamBindResp.Body.String())
	}

	employeeID := uuid.New()
	employeeBindReq := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+employeeID.String()+"/skills", strings.NewReader(`{"skill_id":"`+bindSkillID.String()+`"}`))
	employeeBindReq.Header.Set("Content-Type", "application/json")
	employeeBindReq.AddCookie(cookie)
	employeeBindResp := httptest.NewRecorder()
	server.ServeHTTP(employeeBindResp, employeeBindReq)
	if employeeBindResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing employee bind target to return 404, got %d: %s", employeeBindResp.Code, employeeBindResp.Body.String())
	}
}

type routeSkillService struct {
	listReq                 skill.ListSkillsRequest
	deleteReq               skill.DeleteSkillRequest
	uploadReq               skill.UploadSkillRequest
	teamListReq             skill.ListTeamSkillsRequest
	teamBindReq             skill.BindTeamSkillRequest
	teamUnbindReq           skill.BindTeamSkillRequest
	effectiveListReq        skill.ListEffectiveEmployeeSkillsRequest
	employeeBindReq         skill.BindEmployeeSkillRequest
	employeeUnbindReq       skill.BindEmployeeSkillRequest
	installReq              skill.InstallSkillRequest
	listInstallationsReq    skill.ListSkillInstallationsRequest
	installCalled           bool
	listInstallationsCalled bool
	teamBindErr             error
	employeeBindErr         error
	skillID                 uuid.UUID
}

func (s *routeSkillService) ListSkills(_ context.Context, req skill.ListSkillsRequest) ([]*skill.Skill, error) {
	s.listReq = req
	s.skillID = uuid.New()
	return []*skill.Skill{{
		ID:               s.skillID,
		TenantID:         req.TenantID,
		Slug:             "diagnose",
		Name:             "diagnose",
		Description:      "诊断流程",
		Version:          "v1.0.0",
		Tags:             []string{"诊断", "测试"},
		ArchiveObjectRef: "s3://bucket/skills/diagnose.zip",
		ArchiveFileCount: 3,
		AgentBindings: []*skill.SkillAgentBinding{{
			AgentID:   uuid.New(),
			AgentName: "需求澄清 Agent",
			TeamName:  "产品与需求团队",
			Status:    "enabled",
		}},
	}}, nil
}

func (s *routeSkillService) GetSkill(context.Context, skill.GetSkillRequest) (*skill.Skill, error) {
	return nil, nil
}

func (s *routeSkillService) DeleteSkill(_ context.Context, req skill.DeleteSkillRequest) error {
	s.deleteReq = req
	return nil
}

func (s *routeSkillService) UploadSkill(_ context.Context, req skill.UploadSkillRequest) (*skill.Skill, error) {
	s.uploadReq = req
	return &skill.Skill{
		ID:                  uuid.New(),
		TenantID:            req.TenantID,
		Name:                req.Name,
		Description:         req.Description,
		Tags:                req.Tags,
		RuntimeDependencies: req.RuntimeDependencies,
	}, nil
}

func (s *routeSkillService) BindSkillToTeam(_ context.Context, req skill.BindTeamSkillRequest) (*skill.Skill, error) {
	s.teamBindReq = req
	if s.teamBindErr != nil {
		return nil, s.teamBindErr
	}
	return &skill.Skill{
		ID:       req.SkillID,
		TenantID: req.TenantID,
		Slug:     "bound-team-skill",
		Name:     "bound-team-skill",
	}, nil
}

func (s *routeSkillService) UnbindSkillFromTeam(_ context.Context, req skill.BindTeamSkillRequest) error {
	s.teamUnbindReq = req
	return nil
}

func (s *routeSkillService) ListTeamSkills(_ context.Context, req skill.ListTeamSkillsRequest) ([]*skill.Skill, error) {
	s.teamListReq = req
	return []*skill.Skill{{
		ID:       uuid.New(),
		TenantID: req.TenantID,
		Slug:     "team-diagnose",
		Name:     "team-diagnose",
	}}, nil
}

func (s *routeSkillService) BindSkillToEmployee(_ context.Context, req skill.BindEmployeeSkillRequest) (*skill.Skill, error) {
	s.employeeBindReq = req
	if s.employeeBindErr != nil {
		return nil, s.employeeBindErr
	}
	return &skill.Skill{
		ID:       req.SkillID,
		TenantID: req.TenantID,
		Slug:     "bound-employee-skill",
		Name:     "bound-employee-skill",
	}, nil
}

func (s *routeSkillService) UnbindSkillFromEmployee(_ context.Context, req skill.BindEmployeeSkillRequest) error {
	s.employeeUnbindReq = req
	return nil
}

func (s *routeSkillService) ListEffectiveEmployeeSkills(_ context.Context, req skill.ListEffectiveEmployeeSkillsRequest) ([]skill.EffectiveEmployeeSkill, error) {
	s.effectiveListReq = req
	return []skill.EffectiveEmployeeSkill{
		{
			Skill:       skill.Skill{ID: uuid.New(), TenantID: req.TenantID, Slug: "team-diagnose", Name: "team-diagnose"},
			SourceScope: "team",
			Inherited:   true,
			ReadOnly:    true,
		},
		{
			Skill:       skill.Skill{ID: uuid.New(), TenantID: req.TenantID, Slug: "personal-review", Name: "personal-review"},
			SourceScope: "employee",
			Inherited:   false,
			ReadOnly:    false,
		},
	}, nil
}

func (s *routeSkillService) InstallSkill(_ context.Context, req skill.InstallSkillRequest) (skill.InstallSkillResult, error) {
	s.installCalled = true
	s.installReq = req
	return skill.InstallSkillResult{
		SkillID:           req.SkillID,
		TargetScope:       req.TargetScope,
		TeamID:            req.TeamID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		InstalledCount:    1,
		Installations: []skill.SkillInstallation{{
			ID:                uuid.New(),
			TenantID:          req.TenantID,
			SkillID:           req.SkillID,
			TargetScope:       req.TargetScope,
			TeamID:            req.TeamID,
			DigitalEmployeeID: req.DigitalEmployeeID,
			ProviderType:      "codex",
			InstalledPath:     "/home/agent/.agents/skills/diagnose",
			InstalledBy:       req.ActorUserID,
		}},
	}, nil
}

func (s *routeSkillService) ListSkillInstallations(_ context.Context, req skill.ListSkillInstallationsRequest) ([]skill.SkillInstallation, error) {
	s.listInstallationsCalled = true
	s.listInstallationsReq = req
	return []skill.SkillInstallation{{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		SkillID:           req.SkillID,
		TargetScope:       skill.SkillInstallTargetEmployee,
		DigitalEmployeeID: uuid.New(),
		EmployeeName:      "需求澄清 Agent",
		RuntimeNodeID:     uuid.New(),
		NodeID:            "node-a",
		ProviderType:      "codex",
		InstalledPath:     "/home/agent/.agents/skills/diagnose",
	}}, nil
}

func assertLastSkillAuthzCheck(t *testing.T, authorizer *routeAuthorizer, action, resourceType, resourceID string, tenantID uuid.UUID, teamID *uuid.UUID) {
	t.Helper()
	if len(authorizer.checks) == 0 {
		t.Fatal("expected authorization check")
	}
	check := authorizer.checks[len(authorizer.checks)-1]
	if check.Action != action || check.Resource.Type != resourceType || check.Resource.ID != resourceID || check.TenantID != tenantID {
		t.Fatalf("unexpected authorization check: %#v", check)
	}
	if teamID == nil {
		if check.TeamID != nil {
			t.Fatalf("expected no team context, got %#v", check.TeamID)
		}
		return
	}
	if check.TeamID == nil || *check.TeamID != *teamID {
		t.Fatalf("expected team context %s, got %#v", *teamID, check.TeamID)
	}
}

func buildSkillUploadMultipart(t *testing.T, fields map[string]string, files map[string]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	for path, content := range files {
		w, err := zipWriter.Create(path)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", path, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", path, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	part, err := writer.CreateFormFile("file", "skill.zip")
	if err != nil {
		t.Fatalf("create upload file part: %v", err)
	}
	if _, err := part.Write(archive.Bytes()); err != nil {
		t.Fatalf("write upload file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
