package project

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
)

func TestProjectHandlerRejectsBadProjectID(t *testing.T) {
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/not-a-uuid", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, uuid.New()))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectId", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	resp := httptest.NewRecorder()

	handler.GetProject(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad project id to return 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestProjectHandlerRejectsInvalidJSON(t *testing.T) {
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, uuid.New()))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.New()))
	resp := httptest.NewRecorder()

	handler.CreateProject(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid json to return 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestProjectHandlerMapsArchivedConflict(t *testing.T) {
	projectID := uuid.New()
	service := &handlerTestService{submitDemandErr: ErrProjectArchived}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/demands", strings.NewReader(`{"title":"需求"}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, uuid.New()))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectId", projectID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	resp := httptest.NewRecorder()

	handler.SubmitDemand(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("expected archived project to return 409, got %d: %s", resp.Code, resp.Body.String())
	}
}

type handlerTestService struct {
	createReq       CreateProjectRequest
	submitDemandReq SubmitProjectDemandRequest
	submitDemandErr error
}

func (s *handlerTestService) CreateProject(ctx context.Context, req CreateProjectRequest) (*CreateProjectResult, error) {
	s.createReq = req
	project := testProject(req.TenantID, uuid.New(), req.HumanOwnerUserID)
	project.Name = req.Name
	project.Goal = req.Goal
	return &CreateProjectResult{Project: project}, nil
}

func (s *handlerTestService) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (*Project, error) {
	project := testProject(tenantID, projectID, uuid.New())
	return &project, nil
}

func (s *handlerTestService) ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error) {
	return []Project{testProject(req.TenantID, uuid.New(), uuid.New())}, nil
}

func (s *handlerTestService) UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (*Project, error) {
	project := testProject(req.TenantID, req.ProjectID, uuid.New())
	return &project, nil
}

func (s *handlerTestService) ArchiveProject(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) (*Project, error) {
	project := testProject(tenantID, projectID, actorUserID)
	project.Status = ProjectStatusArchived
	return &project, nil
}

func (s *handlerTestService) ReplaceProjectMembers(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	return nil, nil
}

func (s *handlerTestService) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error) {
	return nil, nil
}

func (s *handlerTestService) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error) {
	return nil, nil
}

func (s *handlerTestService) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error) {
	return nil, nil
}

func (s *handlerTestService) GetLatestProjectConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectConfigRevision, error) {
	revision := ProjectConfigRevision{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, RevisionNumber: 1, ConfigSnapshot: map[string]any{}, CreatedByUserID: uuid.New()}
	return &revision, nil
}

func (s *handlerTestService) SubmitDemand(ctx context.Context, req SubmitProjectDemandRequest) (*ProjectDemand, error) {
	s.submitDemandReq = req
	if s.submitDemandErr != nil {
		return nil, s.submitDemandErr
	}
	demand := ProjectDemand{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, SubmittedByUserID: req.SubmittedByUserID, Title: req.Title, SourceType: req.SourceType, SourceRefs: req.SourceRefs, Attachments: req.Attachments, Status: ProjectDemandStatusRecorded}
	return &demand, nil
}

func (s *handlerTestService) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	return nil, nil
}

func (s *handlerTestService) GetOverview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectOverview, error) {
	project := testProject(tenantID, projectID, uuid.New())
	return &ProjectOverview{Project: project, CoordinationWorkflow: ProjectCoordinationWorkflow{WorkflowID: project.CoordinationWorkflowID, Status: project.CoordinationStatus}}, nil
}

func testProject(tenantID, projectID, ownerID uuid.UUID) Project {
	now := time.Now().UTC()
	return Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
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
