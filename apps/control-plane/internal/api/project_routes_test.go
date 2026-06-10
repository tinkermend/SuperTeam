package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/project"
)

func TestProjectRoutesUseConsoleAuthAndProjectService(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeProjectService{}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetProjectHandler(project.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	spoofedTenantID := uuid.New()
	spoofedActorID := uuid.New()
	ownerID := uuid.New()
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{
		"tenant_id":"`+spoofedTenantID.String()+`",
		"actor_user_id":"`+spoofedActorID.String()+`",
		"name":"客户接入",
		"goal":"完成 Runtime 接入验收",
		"human_owner_user_id":"`+ownerID.String()+`",
		"members":[{"principal_type":"human_user","principal_id":"`+ownerID.String()+`","project_role":"owner"}],
		"coordination_policy":{"cadence":"daily"},
		"approval_policy":{},
		"evidence_policy":{}
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create project to succeed, got %d: %s", createResp.Code, createResp.Body.String())
	}
	if service.createReq.TenantID != expectedTenantID || service.createReq.ActorUserID != user.ID {
		t.Fatalf("expected create tenant/user %s/%s, got %s/%s", expectedTenantID, user.ID, service.createReq.TenantID, service.createReq.ActorUserID)
	}
	if service.createReq.TenantID == spoofedTenantID || service.createReq.ActorUserID == spoofedActorID {
		t.Fatalf("expected create route to ignore client supplied tenant/user")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects?limit=25&offset=5&q=runtime", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list projects to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listReq.TenantID != expectedTenantID || service.listReq.Limit != 25 || service.listReq.Offset != 5 || service.listReq.Query != "runtime" {
		t.Fatalf("expected list request to use console tenant and query filters, got %#v", service.listReq)
	}

	overviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+service.projectID.String()+"/overview", nil)
	overviewReq.AddCookie(cookie)
	overviewResp := httptest.NewRecorder()
	server.ServeHTTP(overviewResp, overviewReq)
	if overviewResp.Code != http.StatusOK {
		t.Fatalf("expected overview to succeed, got %d: %s", overviewResp.Code, overviewResp.Body.String())
	}
	if service.overviewTenantID != expectedTenantID || service.overviewProjectID != service.projectID {
		t.Fatalf("expected overview tenant/project %s/%s, got %s/%s", expectedTenantID, service.projectID, service.overviewTenantID, service.overviewProjectID)
	}

	spoofedProjectID := uuid.New()
	spoofedSubmitterID := uuid.New()
	demandReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+service.projectID.String()+"/demands", strings.NewReader(`{
		"tenant_id":"`+spoofedTenantID.String()+`",
		"project_id":"`+spoofedProjectID.String()+`",
		"submitted_by_user_id":"`+spoofedSubmitterID.String()+`",
		"title":"补充验收证据",
		"content":"上传执行日志",
		"source_refs":{"ticket":"SUP-1"},
		"attachments":["s3://bucket/log.txt"]
	}`))
	demandReq.Header.Set("Content-Type", "application/json")
	demandReq.AddCookie(cookie)
	demandResp := httptest.NewRecorder()
	server.ServeHTTP(demandResp, demandReq)
	if demandResp.Code != http.StatusCreated {
		t.Fatalf("expected submit demand to succeed, got %d: %s", demandResp.Code, demandResp.Body.String())
	}
	if service.submitDemandReq.TenantID != expectedTenantID || service.submitDemandReq.ProjectID != service.projectID || service.submitDemandReq.SubmittedByUserID != user.ID {
		t.Fatalf("expected demand tenant/project/user from context/path, got %#v", service.submitDemandReq)
	}
	if service.submitDemandReq.SourceType != "" {
		t.Fatalf("expected omitted source_type to reach service default path, got %q", service.submitDemandReq.SourceType)
	}
	if service.submitDemandReq.SourceRefs["ticket"] != "SUP-1" || len(service.submitDemandReq.Attachments) != 1 {
		t.Fatalf("expected demand source refs and attachments, got %#v/%#v", service.submitDemandReq.SourceRefs, service.submitDemandReq.Attachments)
	}
}

func TestProjectRoutesRejectBadRequestsAndConflicts(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeProjectService{projectID: uuid.New(), archiveErr: project.ErrProjectArchived}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetProjectHandler(project.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	badIDReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/not-a-uuid", nil)
	badIDReq.AddCookie(cookie)
	badIDResp := httptest.NewRecorder()
	server.ServeHTTP(badIDResp, badIDReq)
	if badIDResp.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed project id to return 400, got %d: %s", badIDResp.Code, badIDResp.Body.String())
	}

	invalidJSONReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":`))
	invalidJSONReq.Header.Set("Content-Type", "application/json")
	invalidJSONReq.AddCookie(cookie)
	invalidJSONResp := httptest.NewRecorder()
	server.ServeHTTP(invalidJSONResp, invalidJSONReq)
	if invalidJSONResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid json to return 400, got %d: %s", invalidJSONResp.Code, invalidJSONResp.Body.String())
	}

	archiveReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+service.projectID.String()+"/archive", nil)
	archiveReq.AddCookie(cookie)
	archiveResp := httptest.NewRecorder()
	server.ServeHTTP(archiveResp, archiveReq)
	if archiveResp.Code != http.StatusConflict {
		t.Fatalf("expected archived conflict to return 409, got %d: %s", archiveResp.Code, archiveResp.Body.String())
	}
}

type routeProjectService struct {
	projectID         uuid.UUID
	createReq         project.CreateProjectRequest
	listReq           project.ListProjectsRequest
	overviewTenantID  uuid.UUID
	overviewProjectID uuid.UUID
	submitDemandReq   project.SubmitProjectDemandRequest
	archiveErr        error
}

func (s *routeProjectService) ensureProjectID() uuid.UUID {
	if s.projectID == uuid.Nil {
		s.projectID = uuid.New()
	}
	return s.projectID
}

func (s *routeProjectService) CreateProject(ctx context.Context, req project.CreateProjectRequest) (*project.CreateProjectResult, error) {
	s.createReq = req
	projectValue := routeProject(req.TenantID, s.ensureProjectID(), req.HumanOwnerUserID)
	projectValue.Name = req.Name
	projectValue.Goal = req.Goal
	return &project.CreateProjectResult{Project: projectValue}, nil
}

func (s *routeProjectService) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (*project.Project, error) {
	projectValue := routeProject(tenantID, projectID, uuid.New())
	return &projectValue, nil
}

func (s *routeProjectService) ListProjects(ctx context.Context, req project.ListProjectsRequest) ([]project.Project, error) {
	s.listReq = req
	return []project.Project{routeProject(req.TenantID, s.ensureProjectID(), uuid.New())}, nil
}

func (s *routeProjectService) UpdateProjectConfig(ctx context.Context, req project.UpdateProjectConfigRequest) (*project.Project, error) {
	projectValue := routeProject(req.TenantID, req.ProjectID, uuid.New())
	return &projectValue, nil
}

func (s *routeProjectService) ArchiveProject(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) (*project.Project, error) {
	if s.archiveErr != nil {
		return nil, s.archiveErr
	}
	projectValue := routeProject(tenantID, projectID, actorUserID)
	projectValue.Status = project.ProjectStatusArchived
	return &projectValue, nil
}

func (s *routeProjectService) ReplaceProjectMembers(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, members []project.ProjectMemberInput) ([]project.ProjectMember, error) {
	return nil, nil
}

func (s *routeProjectService) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]project.ProjectMember, error) {
	return nil, nil
}

func (s *routeProjectService) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]project.ProjectTask, error) {
	return nil, nil
}

func (s *routeProjectService) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.ProjectEvent, error) {
	return nil, nil
}

func (s *routeProjectService) GetLatestProjectConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (*project.ProjectConfigRevision, error) {
	revision := project.ProjectConfigRevision{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, RevisionNumber: 1, ConfigSnapshot: map[string]any{}, CreatedByUserID: uuid.New()}
	return &revision, nil
}

func (s *routeProjectService) SubmitDemand(ctx context.Context, req project.SubmitProjectDemandRequest) (*project.ProjectDemand, error) {
	s.submitDemandReq = req
	demand := project.ProjectDemand{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, SubmittedByUserID: req.SubmittedByUserID, Title: req.Title, SourceType: req.SourceType, SourceRefs: req.SourceRefs, Attachments: req.Attachments, Status: project.ProjectDemandStatusRecorded}
	return &demand, nil
}

func (s *routeProjectService) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.ProjectDemand, error) {
	return nil, nil
}

func (s *routeProjectService) GetOverview(ctx context.Context, tenantID, projectID uuid.UUID) (*project.ProjectOverview, error) {
	s.overviewTenantID = tenantID
	s.overviewProjectID = projectID
	projectValue := routeProject(tenantID, projectID, uuid.New())
	return &project.ProjectOverview{
		Project:              projectValue,
		StatusSummary:        project.ProjectStatusSummary{CurrentPhase: string(projectValue.Status)},
		CoordinationWorkflow: project.ProjectCoordinationWorkflow{WorkflowID: projectValue.CoordinationWorkflowID, Status: projectValue.CoordinationStatus},
	}, nil
}

func routeProject(tenantID, projectID, ownerID uuid.UUID) project.Project {
	now := time.Now().UTC()
	return project.Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 project.ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
		CoordinationPolicy:     map[string]any{},
		ApprovalPolicy:         map[string]any{},
		EvidencePolicy:         map[string]any{},
		CreatedAt:              now,
		UpdatedAt:              now,
	}
}
