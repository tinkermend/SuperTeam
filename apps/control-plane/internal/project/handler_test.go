package project

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

func TestProjectHandlerGetConfigUsesCurrentOverview(t *testing.T) {
	projectID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, uuid.New()))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectId", projectID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	resp := httptest.NewRecorder()

	handler.GetProjectConfig(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected current config to return 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.getOverviewCalls != 1 {
		t.Fatalf("expected get config to call overview once, got %d", service.getOverviewCalls)
	}
	var body struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
		CoordinationPolicy map[string]any `json:"coordination_policy"`
		HumanRoles         []any          `json:"human_roles"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	if body.Project.ID != projectID.String() || body.CoordinationPolicy == nil || len(body.HumanRoles) != 1 {
		t.Fatalf("unexpected config response: %#v", body)
	}
}

func TestProjectHandlerCreatesEvidenceFromConsoleContext(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	spoofedTenantID := uuid.New()
	spoofedProjectID := uuid.New()
	spoofedActorID := uuid.New()
	spoofedSubmitterID := uuid.New()
	taskID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/evidence", strings.NewReader(`{
		"tenant_id":"`+spoofedTenantID.String()+`",
		"project_id":"`+spoofedProjectID.String()+`",
		"actor_user_id":"`+spoofedActorID.String()+`",
		"submitted_by_id":"`+spoofedSubmitterID.String()+`",
		"project_task_id":"`+taskID.String()+`",
		"evidence_type":"test_report",
		"title":"验收测试报告",
		"summary":"全部通过",
		"source_type":"artifact",
		"source_ref":"s3://bucket/report.md",
		"metadata":{"suite":"go"}
	}`))
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.CreateEvidence(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected evidence create to return 201, got %d: %s", resp.Code, resp.Body.String())
	}
	got := service.createEvidenceReq
	if got.TenantID != tenantID || got.ProjectID != projectID || got.ActorID != actorID {
		t.Fatalf("expected evidence tenant/project/actor from context/path, got %#v", got)
	}
	if got.TenantID == spoofedTenantID || got.ProjectID == spoofedProjectID || got.ActorID == spoofedActorID {
		t.Fatalf("expected evidence create to ignore spoofed tenant/project/actor ids")
	}
	if got.SubmittedByID == nil || *got.SubmittedByID != actorID || got.SubmittedByType != "human_user" {
		t.Fatalf("expected submitted_by to use console actor, got type=%q id=%v", got.SubmittedByType, got.SubmittedByID)
	}
	if got.ProjectTaskID == nil || *got.ProjectTaskID != taskID || got.Metadata["suite"] != "go" {
		t.Fatalf("expected evidence body facts to be forwarded, got %#v", got)
	}
	var body struct {
		ProjectID          string `json:"project_id"`
		SubmittedByID      string `json:"submitted_by_id"`
		VerificationStatus string `json:"verification_status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode evidence response: %v", err)
	}
	if body.ProjectID != projectID.String() || body.SubmittedByID != actorID.String() || body.VerificationStatus != string(EvidenceVerificationStatusSubmitted) {
		t.Fatalf("unexpected evidence response: %#v", body)
	}
}

func TestProjectHandlerMapsGovernanceNotFound(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	service := &handlerTestService{
		patchEvidenceErr:     ErrProjectNotFound,
		getAcceptanceErr:     ErrProjectNotFound,
		getConfigRevisionErr: ErrProjectNotFound,
	}
	handler := NewHandler(service)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+projectID.String()+"/evidence/"+uuid.New().String(), strings.NewReader(`{"verification_status":"verified"}`))
	patchReq = withProjectRouteParams(patchReq, map[string]string{"projectId": projectID.String(), "evidenceId": uuid.New().String()})
	patchReq = withConsoleContext(patchReq, tenantID, actorID)
	patchResp := httptest.NewRecorder()
	handler.PatchEvidence(patchResp, patchReq)
	if patchResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing evidence to return 404, got %d: %s", patchResp.Code, patchResp.Body.String())
	}

	acceptanceReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/acceptance", nil)
	acceptanceReq = withProjectRouteParams(acceptanceReq, map[string]string{"projectId": projectID.String()})
	acceptanceReq = withConsoleContext(acceptanceReq, tenantID, actorID)
	acceptanceResp := httptest.NewRecorder()
	handler.GetAcceptance(acceptanceResp, acceptanceReq)
	if acceptanceResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing acceptance to return 404, got %d: %s", acceptanceResp.Code, acceptanceResp.Body.String())
	}

	revisionID := uuid.New()
	revisionReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/config-revisions/"+revisionID.String(), nil)
	revisionReq = withProjectRouteParams(revisionReq, map[string]string{"projectId": projectID.String(), "revisionId": revisionID.String()})
	revisionReq = withConsoleContext(revisionReq, tenantID, actorID)
	revisionResp := httptest.NewRecorder()
	handler.GetConfigRevision(revisionResp, revisionReq)
	if revisionResp.Code != http.StatusNotFound {
		t.Fatalf("expected missing config revision to return 404, got %d: %s", revisionResp.Code, revisionResp.Body.String())
	}
}

func TestProjectHandlerListsRouteDecisionsAndResolvesDecision(t *testing.T) {
	projectID := uuid.New()
	decisionID := uuid.New()
	tenantID := uuid.New()
	actorID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/route-decisions?limit=10&offset=2", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, actorID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectId", projectID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	resp := httptest.NewRecorder()

	handler.ListRouteDecisions(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected route decisions 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.routeDecisionTenantID != tenantID || service.routeDecisionProjectID != projectID || service.routeDecisionLimit != 10 || service.routeDecisionOffset != 2 {
		t.Fatalf("unexpected route decision query: tenant=%s project=%s limit=%d offset=%d", service.routeDecisionTenantID, service.routeDecisionProjectID, service.routeDecisionLimit, service.routeDecisionOffset)
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/decisions/"+decisionID.String()+"/resolve", strings.NewReader(`{"decision":"approved","comment":"同意","payload":{"source":"console"}}`))
	resolveReq = resolveReq.WithContext(context.WithValue(resolveReq.Context(), middleware.TenantIDKey, tenantID))
	resolveReq = resolveReq.WithContext(context.WithValue(resolveReq.Context(), middleware.UserIDKey, actorID))
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("projectId", projectID.String())
	rctx.URLParams.Add("decisionId", decisionID.String())
	resolveReq = resolveReq.WithContext(context.WithValue(resolveReq.Context(), chi.RouteCtxKey, rctx))
	resolveResp := httptest.NewRecorder()

	handler.ResolveDecision(resolveResp, resolveReq)

	if resolveResp.Code != http.StatusOK {
		t.Fatalf("expected decision resolve 200, got %d: %s", resolveResp.Code, resolveResp.Body.String())
	}
	if service.resolveDecisionReq.TenantID != tenantID || service.resolveDecisionReq.ProjectID != projectID || service.resolveDecisionReq.DecisionRequestID != decisionID || service.resolveDecisionReq.DecidedByUserID != actorID || service.resolveDecisionReq.Decision != "approved" {
		t.Fatalf("unexpected resolve request: %#v", service.resolveDecisionReq)
	}
	if service.resolveDecisionReq.Payload["source"] != "console" {
		t.Fatalf("expected payload to be decoded, got %#v", service.resolveDecisionReq.Payload)
	}
}

func TestProjectHandlerWithRealServiceE2ESimulation(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: errors.New("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	handler := NewHandler(service)
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "HTTP E2E 仿真项目",
		Goal:                   "验证接口到服务的项目协调闭环",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}

	submitReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/demands", strings.NewReader(`{
		"title":"验证 Runtime 执行回写",
		"content":"模拟 Workflow signal 短暂失败"
	}`))
	submitReq = withProjectRouteParams(submitReq, map[string]string{"projectId": projectID.String()})
	submitReq = withConsoleContext(submitReq, tenantID, ownerID)
	submitResp := httptest.NewRecorder()

	handler.SubmitDemand(submitResp, submitReq)

	if submitResp.Code != http.StatusInternalServerError {
		t.Fatalf("expected transient signal failure to surface as 500, got %d: %s", submitResp.Code, submitResp.Body.String())
	}
	if len(repo.demands) != 1 || countProjectEvents(repo.eventTypes, ProjectEventDemandSubmitted) != 1 {
		t.Fatalf("expected one demand persisted before signal retry, demands=%d events=%#v", len(repo.demands), repo.eventTypes)
	}
	if repo.demands[0].Content == nil || *repo.demands[0].Content != "模拟 Workflow signal 短暂失败" {
		t.Fatalf("expected demand content to be decoded and persisted, got %#v", repo.demands[0])
	}
	failedDemandSignalEvent := repo.events[len(repo.events)-1]
	if failedDemandSignalEvent.Payload["signal_name"] != "DemandSubmitted" || failedDemandSignalEvent.Payload["status"] != "failed" {
		t.Fatalf("expected failed demand signal event, got %#v", failedDemandSignalEvent)
	}

	coordinator.demandSignalErr = nil
	retryDemandReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/events/"+failedDemandSignalEvent.ID.String()+"/retry-workflow-signal", nil)
	retryDemandReq = withProjectRouteParams(retryDemandReq, map[string]string{"projectId": projectID.String(), "eventId": failedDemandSignalEvent.ID.String()})
	retryDemandReq = withConsoleContext(retryDemandReq, tenantID, ownerID)
	retryDemandResp := httptest.NewRecorder()

	handler.RetryWorkflowSignal(retryDemandResp, retryDemandReq)

	if retryDemandResp.Code != http.StatusAccepted {
		t.Fatalf("expected demand signal retry to return 202, got %d: %s", retryDemandResp.Code, retryDemandResp.Body.String())
	}
	if coordinator.demandSignals != 2 || len(repo.demands) != 1 {
		t.Fatalf("expected retry to resend demand signal without duplicate demand, signals=%d demands=%d", coordinator.demandSignals, len(repo.demands))
	}

	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理执行证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	wrongRuntimeReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-tasks/"+taskID.String()+"/complete", strings.NewReader(`{
		"digital_employee_id":"`+employeeID.String()+`",
		"conclusion":"错误 Runtime 尝试写回"
	}`))
	wrongRuntimeReq = withProjectRouteParams(wrongRuntimeReq, map[string]string{"projectTaskId": taskID.String()})
	wrongRuntimeReq = withRuntimeContext(wrongRuntimeReq, tenantID, uuid.New())
	wrongRuntimeResp := httptest.NewRecorder()

	handler.CompleteProjectTask(wrongRuntimeResp, wrongRuntimeReq)

	if wrongRuntimeResp.Code != http.StatusForbidden {
		t.Fatalf("expected wrong runtime writeback to return 403, got %d: %s", wrongRuntimeResp.Code, wrongRuntimeResp.Body.String())
	}
	if len(repo.executionSummaries) != 0 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 0 {
		t.Fatalf("expected wrong runtime writeback to have no side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}

	coordinator.completedSignalErr = errors.New("temporal unavailable")
	completeReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-tasks/"+taskID.String()+"/complete", strings.NewReader(`{
		"digital_employee_id":"`+employeeID.String()+`",
		"conclusion":"证据充分",
		"evidence_refs":["s3://bucket/e2e-report.md"],
		"artifact_refs":["artifact-runtime-log"],
		"confidence_factors":{"tests":"passed"},
		"recommended_next_action":"提交负责人验收"
	}`))
	completeReq = withProjectRouteParams(completeReq, map[string]string{"projectTaskId": taskID.String()})
	completeReq = withRuntimeContext(completeReq, tenantID, runtimeNodeID)
	completeResp := httptest.NewRecorder()

	handler.CompleteProjectTask(completeResp, completeReq)

	if completeResp.Code != http.StatusInternalServerError {
		t.Fatalf("expected completed signal failure to surface as 500, got %d: %s", completeResp.Code, completeResp.Body.String())
	}
	if repo.tasks[0].Status != "completed" || len(repo.executionSummaries) != 1 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected task writeback persisted before signal retry, task=%#v summaries=%d events=%#v", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes)
	}
	summary := repo.executionSummaries[0]
	if len(summary.EvidenceRefs) != 1 || summary.EvidenceRefs[0] != "s3://bucket/e2e-report.md" {
		t.Fatalf("expected evidence refs to be decoded, got %#v", summary.EvidenceRefs)
	}
	if len(summary.ArtifactRefs) != 1 || summary.ArtifactRefs[0] != "artifact-runtime-log" {
		t.Fatalf("expected artifact refs to be decoded, got %#v", summary.ArtifactRefs)
	}
	if summary.ConfidenceFactors["tests"] != "passed" {
		t.Fatalf("expected confidence factors to be decoded, got %#v", summary.ConfidenceFactors)
	}
	if summary.RecommendedNextAction == nil || *summary.RecommendedNextAction != "提交负责人验收" {
		t.Fatalf("expected recommended next action to be decoded, got %#v", summary.RecommendedNextAction)
	}
	failedCompletedSignalEvent := repo.events[len(repo.events)-1]
	if failedCompletedSignalEvent.Payload["signal_name"] != "EmployeeTaskCompleted" || failedCompletedSignalEvent.Payload["status"] != "failed" {
		t.Fatalf("expected failed completed signal event, got %#v", failedCompletedSignalEvent)
	}

	coordinator.completedSignalErr = nil
	retryCompletedReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/events/"+failedCompletedSignalEvent.ID.String()+"/retry-workflow-signal", nil)
	retryCompletedReq = withProjectRouteParams(retryCompletedReq, map[string]string{"projectId": projectID.String(), "eventId": failedCompletedSignalEvent.ID.String()})
	retryCompletedReq = withConsoleContext(retryCompletedReq, tenantID, ownerID)
	retryCompletedResp := httptest.NewRecorder()

	handler.RetryWorkflowSignal(retryCompletedResp, retryCompletedReq)

	if retryCompletedResp.Code != http.StatusAccepted {
		t.Fatalf("expected completed signal retry to return 202, got %d: %s", retryCompletedResp.Code, retryCompletedResp.Body.String())
	}
	if coordinator.completedSignals != 2 || len(repo.executionSummaries) != 1 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected retry to resend completed signal without duplicate facts, signals=%d summaries=%d events=%#v", coordinator.completedSignals, len(repo.executionSummaries), repo.eventTypes)
	}

	listSummariesReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/execution-summaries", nil)
	listSummariesReq = withProjectRouteParams(listSummariesReq, map[string]string{"projectId": projectID.String()})
	listSummariesReq = withConsoleContext(listSummariesReq, tenantID, ownerID)
	listSummariesResp := httptest.NewRecorder()

	handler.ListExecutionSummaries(listSummariesResp, listSummariesReq)

	if listSummariesResp.Code != http.StatusOK {
		t.Fatalf("expected execution summaries read model to return 200, got %d: %s", listSummariesResp.Code, listSummariesResp.Body.String())
	}
	var summaries []map[string]any
	if err := json.NewDecoder(listSummariesResp.Body).Decode(&summaries); err != nil {
		t.Fatalf("decode execution summaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0]["project_task_id"] != taskID.String() {
		t.Fatalf("unexpected execution summaries response: %#v", summaries)
	}
}

func withProjectRouteParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func withConsoleContext(req *http.Request, tenantID, userID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	return req.WithContext(ctx)
}

func withRuntimeContext(req *http.Request, tenantID, runtimeNodeID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
	ctx = context.WithValue(ctx, middleware.RuntimeNodeIDKey, runtimeNodeID)
	return req.WithContext(ctx)
}

type handlerTestService struct {
	createReq              CreateProjectRequest
	submitDemandReq        SubmitProjectDemandRequest
	submitDemandErr        error
	createEvidenceReq      CreateEvidenceRefServiceRequest
	patchEvidenceReq       PatchEvidenceRequest
	patchEvidenceErr       error
	createAcceptanceReq    CreateAcceptanceServiceRequest
	createArchiveReq       CreateArchiveSnapshotServiceRequest
	getAcceptanceErr       error
	getConfigRevisionErr   error
	getOverviewCalls       int
	routeDecisionTenantID  uuid.UUID
	routeDecisionProjectID uuid.UUID
	routeDecisionLimit     int32
	routeDecisionOffset    int32
	resolveDecisionReq     ResolveDecisionRequest
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
	s.getOverviewCalls++
	project := testProject(tenantID, projectID, uuid.New())
	project.CoordinationPolicy = map[string]any{"cadence": "daily"}
	owner := ProjectMember{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   project.HumanOwnerUserID,
		ProjectRole:   ProjectRoleOwner,
		Status:        "active",
		Settings:      map[string]any{},
	}
	return &ProjectOverview{Project: project, HumanRoles: []ProjectMember{owner}, CoordinationWorkflow: ProjectCoordinationWorkflow{WorkflowID: project.CoordinationWorkflowID, Status: project.CoordinationStatus}}, nil
}

func (s *handlerTestService) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	s.routeDecisionTenantID = tenantID
	s.routeDecisionProjectID = projectID
	s.routeDecisionLimit = limit
	s.routeDecisionOffset = offset
	return []RouteDecision{{
		ID:                          uuid.New(),
		TenantID:                    tenantID,
		ProjectID:                   projectID,
		CoordinationJobID:           uuid.New(),
		CandidateDigitalEmployeeIDs: []uuid.UUID{uuid.New()},
		SelectedDigitalEmployeeIDs:  []uuid.UUID{uuid.New()},
		Reason:                      "选择项目数字员工池中的 active executor",
		InputRequirements:           map[string]any{},
		ExpectedOutputs:             []any{"执行摘要"},
		BudgetEstimate:              map[string]any{},
	}}, nil
}

func (s *handlerTestService) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error) {
	return nil, nil
}

func (s *handlerTestService) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error) {
	return nil, nil
}

func (s *handlerTestService) ResolveDecision(ctx context.Context, req ResolveDecisionRequest) (*DecisionRequest, error) {
	s.resolveDecisionReq = req
	decision := DecisionRequest{
		ID:                req.DecisionRequestID,
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: uuid.New(),
		TargetUserID:      req.DecidedByUserID,
		DecisionType:      "route_review",
		TitleSnapshot:     "需要负责人确认",
		StatusSnapshot:    req.Decision,
	}
	return &decision, nil
}

func (s *handlerTestService) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error) {
	return nil, nil
}

func (s *handlerTestService) ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error) {
	return nil, nil
}

func (s *handlerTestService) RetryWorkflowSignal(ctx context.Context, req RetryWorkflowSignalRequest) (*ProjectEvent, error) {
	return &ProjectEvent{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		SequenceNumber: 1,
		EventType:      ProjectEventWorkflowSignaled,
		ActorType:      "human_user",
		ActorID:        req.ActorID.String(),
		Payload:        map[string]any{"status": "sent"},
	}, nil
}

func (s *handlerTestService) CompleteProjectTask(ctx context.Context, req CompleteProjectTaskRequest) (*ExecutionSummary, error) {
	return nil, nil
}

func (s *handlerTestService) FailProjectTask(ctx context.Context, req FailProjectTaskRequest) (*ProjectTask, error) {
	return nil, nil
}

func (s *handlerTestService) RequestProjectTaskTransfer(ctx context.Context, req RequestProjectTaskTransferRequest) (*TransferRequest, error) {
	return nil, nil
}

func (s *handlerTestService) ListEvidence(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	return []ProjectEvidenceRef{testEvidence(tenantID, projectID, uuid.New())}, nil
}

func (s *handlerTestService) CreateEvidence(ctx context.Context, req CreateEvidenceRefServiceRequest) (*ProjectEvidenceRef, error) {
	s.createEvidenceReq = req
	evidence := testEvidence(req.TenantID, req.ProjectID, req.ActorID)
	evidence.ProjectTaskID = req.ProjectTaskID
	evidence.RouteDecisionID = req.RouteDecisionID
	evidence.ExecutionSummaryID = req.ExecutionSummaryID
	evidence.EvidenceType = req.EvidenceType
	evidence.Title = req.Title
	evidence.Summary = stringPtrValue(req.Summary)
	evidence.SourceType = req.SourceType
	evidence.SourceRef = req.SourceRef
	evidence.ArtifactRefID = req.ArtifactRefID
	evidence.SubmittedByType = req.SubmittedByType
	evidence.SubmittedByID = req.SubmittedByID
	evidence.Metadata = req.Metadata
	return &evidence, nil
}

func (s *handlerTestService) PatchEvidence(ctx context.Context, req PatchEvidenceRequest) (*ProjectEvidenceRef, error) {
	s.patchEvidenceReq = req
	if s.patchEvidenceErr != nil {
		return nil, s.patchEvidenceErr
	}
	evidence := testEvidence(req.TenantID, req.ProjectID, req.ActorUserID)
	evidence.ID = req.EvidenceID
	evidence.VerificationStatus = req.VerificationStatus
	evidence.Metadata = req.Metadata
	return &evidence, nil
}

func (s *handlerTestService) ListArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	return []ProjectArtifactRef{testArtifact(tenantID, projectID)}, nil
}

func (s *handlerTestService) ListReports(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	return []ProjectReportRef{testReport(tenantID, projectID)}, nil
}

func (s *handlerTestService) ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error) {
	return []ProjectBudgetLedgerEntry{{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CostType: "tokens", EstimatedCost: "1.00", ActualCost: "0.80", Source: "runtime", CreatedAt: time.Now().UTC()}}, nil
}

func (s *handlerTestService) GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectBudgetSummary, error) {
	return &ProjectBudgetSummary{EstimatedTokens: 1000, ActualTokens: 800, EstimatedCost: "1.00", ActualCost: "0.80", LedgerCount: 1}, nil
}

func (s *handlerTestService) CreateAcceptance(ctx context.Context, req CreateAcceptanceServiceRequest) (*ProjectAcceptanceRecord, error) {
	s.createAcceptanceReq = req
	record := testAcceptance(req.TenantID, req.ProjectID, req.AcceptedByUserID)
	record.Status = req.Status
	record.Conclusion = req.Conclusion
	record.EvidenceRefIDs = req.EvidenceRefIDs
	record.ReportRefIDs = req.ReportRefIDs
	record.UnresolvedRisks = req.UnresolvedRisks
	return &record, nil
}

func (s *handlerTestService) GetAcceptance(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectAcceptanceRecord, error) {
	if s.getAcceptanceErr != nil {
		return nil, s.getAcceptanceErr
	}
	record := testAcceptance(tenantID, projectID, uuid.New())
	return &record, nil
}

func (s *handlerTestService) GetArchivePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectArchivePreview, error) {
	return &ProjectArchivePreview{ProjectID: projectID, EvidenceCount: 1, ArtifactCount: 1, ReportCount: 1, BlockedReasons: []any{}, EstimatedObjectRefs: []any{"s3://bucket/report.md"}}, nil
}

func (s *handlerTestService) CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotServiceRequest) (*ProjectArchiveSnapshot, error) {
	s.createArchiveReq = req
	snapshot := testArchiveSnapshot(req.TenantID, req.ProjectID, req.CreatedByUserID)
	snapshot.SnapshotType = req.SnapshotType
	snapshot.ObjectRef = stringPtrValue(req.ObjectRef)
	snapshot.Summary = stringPtrValue(req.Summary)
	return &snapshot, nil
}

func (s *handlerTestService) ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error) {
	return []ProjectArchiveSnapshot{testArchiveSnapshot(tenantID, projectID, uuid.New())}, nil
}

func (s *handlerTestService) ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error) {
	return []ProjectConfigRevision{testConfigRevision(tenantID, projectID, uuid.New())}, nil
}

func (s *handlerTestService) GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*ProjectConfigRevision, error) {
	if s.getConfigRevisionErr != nil {
		return nil, s.getConfigRevisionErr
	}
	revision := testConfigRevision(tenantID, projectID, uuid.New())
	revision.ID = revisionID
	return &revision, nil
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

func testEvidence(tenantID, projectID, userID uuid.UUID) ProjectEvidenceRef {
	now := time.Now().UTC()
	return ProjectEvidenceRef{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		ProjectID:          projectID,
		EvidenceType:       "test_report",
		Title:              "验收测试报告",
		SourceType:         "artifact",
		SourceRef:          "s3://bucket/report.md",
		SubmittedByType:    "human_user",
		SubmittedByID:      &userID,
		VerificationStatus: EvidenceVerificationStatusSubmitted,
		Metadata:           map[string]any{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func testArtifact(tenantID, projectID uuid.UUID) ProjectArtifactRef {
	now := time.Now().UTC()
	return ProjectArtifactRef{
		ID:              uuid.New(),
		TenantID:        tenantID,
		ProjectID:       projectID,
		ArtifactType:    "log",
		Title:           "执行日志",
		ObjectRef:       "s3://bucket/run.log",
		RetentionStatus: "retained",
		Metadata:        map[string]any{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func testReport(tenantID, projectID uuid.UUID) ProjectReportRef {
	return ProjectReportRef{
		ID:              uuid.New(),
		TenantID:        tenantID,
		ProjectID:       projectID,
		ReportType:      "final",
		Title:           "最终报告",
		ObjectRef:       "s3://bucket/final.md",
		Format:          "markdown",
		GeneratedByType: "human_user",
		CreatedAt:       time.Now().UTC(),
	}
}

func testAcceptance(tenantID, projectID, userID uuid.UUID) ProjectAcceptanceRecord {
	return ProjectAcceptanceRecord{
		ID:               uuid.New(),
		TenantID:         tenantID,
		ProjectID:        projectID,
		AcceptedByUserID: userID,
		Status:           "accepted",
		Conclusion:       "通过",
		EvidenceRefIDs:   []uuid.UUID{uuid.New()},
		ReportRefIDs:     []uuid.UUID{uuid.New()},
		UnresolvedRisks:  []any{},
		CreatedAt:        time.Now().UTC(),
	}
}

func testArchiveSnapshot(tenantID, projectID, userID uuid.UUID) ProjectArchiveSnapshot {
	return ProjectArchiveSnapshot{
		ID:                  uuid.New(),
		TenantID:            tenantID,
		ProjectID:           projectID,
		SnapshotType:        "final",
		Status:              "archived",
		IncludedCounts:      map[string]any{"evidence_ref_count": float64(1)},
		RetainedArtifactIDs: []uuid.UUID{},
		CreatedByUserID:     userID,
		CreatedAt:           time.Now().UTC(),
	}
}

func testConfigRevision(tenantID, projectID, userID uuid.UUID) ProjectConfigRevision {
	return ProjectConfigRevision{
		ID:              uuid.New(),
		TenantID:        tenantID,
		ProjectID:       projectID,
		RevisionNumber:  1,
		ConfigSnapshot:  map[string]any{"name": "项目"},
		CreatedByUserID: userID,
		CreatedAt:       time.Now().UTC(),
		ChangedSections: []any{},
		DiffSummary:     map[string]any{},
	}
}

func stringPtrValue(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
