package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/audit"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/project"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
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

	configReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+service.projectID.String()+"/config", nil)
	configReq.AddCookie(cookie)
	configResp := httptest.NewRecorder()
	server.ServeHTTP(configResp, configReq)
	if configResp.Code != http.StatusOK {
		t.Fatalf("expected current config to succeed without config revision, got %d: %s", configResp.Code, configResp.Body.String())
	}
	if service.latestConfigRevisionCalls != 0 {
		t.Fatalf("expected get config not to call latest revision, got %d calls", service.latestConfigRevisionCalls)
	}
	if service.overviewCalls < 2 {
		t.Fatalf("expected overview to serve both overview and config routes, got %d calls", service.overviewCalls)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+service.projectID.String()+"/events?limit=10&offset=2", nil)
	eventsReq.AddCookie(cookie)
	eventsResp := httptest.NewRecorder()
	server.ServeHTTP(eventsResp, eventsReq)
	if eventsResp.Code != http.StatusOK {
		t.Fatalf("expected events route to succeed, got %d: %s", eventsResp.Code, eventsResp.Body.String())
	}
	if service.eventsTenantID != expectedTenantID || service.eventsProjectID != service.projectID || service.eventsLimit != 10 || service.eventsOffset != 2 {
		t.Fatalf("expected events tenant/project/page from route, got tenant=%s project=%s limit=%d offset=%d", service.eventsTenantID, service.eventsProjectID, service.eventsLimit, service.eventsOffset)
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

	launchDemandID := uuid.New()
	launchDetailReq := httptest.NewRequest(http.MethodGet, "/api/v1/project-demands/"+launchDemandID.String()+"/launch-detail", nil)
	launchDetailReq.AddCookie(cookie)
	launchDetailResp := httptest.NewRecorder()
	server.ServeHTTP(launchDetailResp, launchDetailReq)
	if launchDetailResp.Code != http.StatusOK {
		t.Fatalf("expected launch detail to succeed, got %d: %s", launchDetailResp.Code, launchDetailResp.Body.String())
	}
	if service.launchDetailTenantID != expectedTenantID || service.launchDetailDemandID != launchDemandID {
		t.Fatalf("expected launch detail tenant/demand from route, got tenant=%s demand=%s", service.launchDetailTenantID, service.launchDetailDemandID)
	}

	routeDecisionReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+service.projectID.String()+"/route-decisions?limit=7", nil)
	routeDecisionReq.AddCookie(cookie)
	routeDecisionResp := httptest.NewRecorder()
	server.ServeHTTP(routeDecisionResp, routeDecisionReq)
	if routeDecisionResp.Code != http.StatusOK {
		t.Fatalf("expected route decisions to succeed, got %d: %s", routeDecisionResp.Code, routeDecisionResp.Body.String())
	}
	if service.routeDecisionTenantID != expectedTenantID || service.routeDecisionProjectID != service.projectID || service.routeDecisionLimit != 7 {
		t.Fatalf("expected route decision tenant/project/page from route, got tenant=%s project=%s limit=%d", service.routeDecisionTenantID, service.routeDecisionProjectID, service.routeDecisionLimit)
	}

	decisionID := uuid.New()
	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+service.projectID.String()+"/decisions/"+decisionID.String()+"/resolve", strings.NewReader(`{"decision":"approved","comment":"同意"}`))
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveReq.AddCookie(cookie)
	resolveResp := httptest.NewRecorder()
	server.ServeHTTP(resolveResp, resolveReq)
	if resolveResp.Code != http.StatusOK {
		t.Fatalf("expected decision resolve to succeed, got %d: %s", resolveResp.Code, resolveResp.Body.String())
	}
	if service.resolveDecisionReq.TenantID != expectedTenantID || service.resolveDecisionReq.ProjectID != service.projectID || service.resolveDecisionReq.DecisionRequestID != decisionID || service.resolveDecisionReq.DecidedByUserID != user.ID {
		t.Fatalf("expected resolve decision context/path/user, got %#v", service.resolveDecisionReq)
	}

	evidenceID := uuid.New()
	evidenceReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+service.projectID.String()+"/evidence", strings.NewReader(`{
		"tenant_id":"`+spoofedTenantID.String()+`",
		"project_id":"`+spoofedProjectID.String()+`",
		"actor_user_id":"`+spoofedActorID.String()+`",
		"submitted_by_id":"`+spoofedActorID.String()+`",
		"evidence_type":"test_report",
		"title":"验收测试报告",
		"source_type":"artifact",
		"source_ref":"s3://bucket/report.md",
		"metadata":{"suite":"routes"}
	}`))
	evidenceReq.Header.Set("Content-Type", "application/json")
	evidenceReq.AddCookie(cookie)
	evidenceResp := httptest.NewRecorder()
	server.ServeHTTP(evidenceResp, evidenceReq)
	if evidenceResp.Code != http.StatusCreated {
		t.Fatalf("expected create evidence to succeed, got %d: %s", evidenceResp.Code, evidenceResp.Body.String())
	}
	if service.createEvidenceReq.TenantID != expectedTenantID || service.createEvidenceReq.ProjectID != service.projectID || service.createEvidenceReq.ActorID != user.ID {
		t.Fatalf("expected evidence context/path/user, got %#v", service.createEvidenceReq)
	}
	if service.createEvidenceReq.SubmittedByID == nil || *service.createEvidenceReq.SubmittedByID != user.ID {
		t.Fatalf("expected evidence submitted_by to use console user, got %#v", service.createEvidenceReq.SubmittedByID)
	}

	patchEvidenceReq := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+service.projectID.String()+"/evidence/"+evidenceID.String(), strings.NewReader(`{
		"tenant_id":"`+spoofedTenantID.String()+`",
		"project_id":"`+spoofedProjectID.String()+`",
		"evidence_id":"`+uuid.New().String()+`",
		"actor_user_id":"`+spoofedActorID.String()+`",
		"verification_status":"verified",
		"metadata":{"review":"manual"}
	}`))
	patchEvidenceReq.Header.Set("Content-Type", "application/json")
	patchEvidenceReq.AddCookie(cookie)
	patchEvidenceResp := httptest.NewRecorder()
	server.ServeHTTP(patchEvidenceResp, patchEvidenceReq)
	if patchEvidenceResp.Code != http.StatusOK {
		t.Fatalf("expected patch evidence to succeed, got %d: %s", patchEvidenceResp.Code, patchEvidenceResp.Body.String())
	}
	if service.patchEvidenceReq.TenantID != expectedTenantID || service.patchEvidenceReq.ProjectID != service.projectID || service.patchEvidenceReq.EvidenceID != evidenceID || service.patchEvidenceReq.ActorUserID != user.ID {
		t.Fatalf("expected patch evidence context/path/user, got %#v", service.patchEvidenceReq)
	}

	budgetSummaryReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+service.projectID.String()+"/budget-summary", nil)
	budgetSummaryReq.AddCookie(cookie)
	budgetSummaryResp := httptest.NewRecorder()
	server.ServeHTTP(budgetSummaryResp, budgetSummaryReq)
	if budgetSummaryResp.Code != http.StatusOK {
		t.Fatalf("expected budget summary to succeed, got %d: %s", budgetSummaryResp.Code, budgetSummaryResp.Body.String())
	}
	if service.budgetSummaryTenantID != expectedTenantID || service.budgetSummaryProjectID != service.projectID {
		t.Fatalf("expected budget summary context/path, got tenant=%s project=%s", service.budgetSummaryTenantID, service.budgetSummaryProjectID)
	}

	acceptanceReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+service.projectID.String()+"/acceptance", strings.NewReader(`{
		"tenant_id":"`+spoofedTenantID.String()+`",
		"project_id":"`+spoofedProjectID.String()+`",
		"accepted_by_user_id":"`+spoofedActorID.String()+`",
		"status":"accepted",
		"conclusion":"通过",
		"evidence_ref_ids":["`+evidenceID.String()+`"],
		"report_ref_ids":["`+uuid.New().String()+`"]
	}`))
	acceptanceReq.Header.Set("Content-Type", "application/json")
	acceptanceReq.AddCookie(cookie)
	acceptanceResp := httptest.NewRecorder()
	server.ServeHTTP(acceptanceResp, acceptanceReq)
	if acceptanceResp.Code != http.StatusCreated {
		t.Fatalf("expected acceptance create to succeed, got %d: %s", acceptanceResp.Code, acceptanceResp.Body.String())
	}
	if service.createAcceptanceReq.TenantID != expectedTenantID || service.createAcceptanceReq.ProjectID != service.projectID || service.createAcceptanceReq.AcceptedByUserID != user.ID {
		t.Fatalf("expected acceptance context/path/user, got %#v", service.createAcceptanceReq)
	}

	archiveSnapshotReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+service.projectID.String()+"/archive-snapshot", strings.NewReader(`{
		"tenant_id":"`+spoofedTenantID.String()+`",
		"project_id":"`+spoofedProjectID.String()+`",
		"created_by_user_id":"`+spoofedActorID.String()+`",
		"snapshot_type":"final",
		"summary":"最终归档",
		"object_ref":"s3://bucket/archive.json"
	}`))
	archiveSnapshotReq.Header.Set("Content-Type", "application/json")
	archiveSnapshotReq.AddCookie(cookie)
	archiveSnapshotResp := httptest.NewRecorder()
	server.ServeHTTP(archiveSnapshotResp, archiveSnapshotReq)
	if archiveSnapshotResp.Code != http.StatusCreated {
		t.Fatalf("expected archive snapshot create to succeed, got %d: %s", archiveSnapshotResp.Code, archiveSnapshotResp.Body.String())
	}
	if service.createArchiveReq.TenantID != expectedTenantID || service.createArchiveReq.ProjectID != service.projectID || service.createArchiveReq.CreatedByUserID != user.ID {
		t.Fatalf("expected archive snapshot context/path/user, got %#v", service.createArchiveReq)
	}

	revisionID := uuid.New()
	configRevisionReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+service.projectID.String()+"/config-revisions/"+revisionID.String(), nil)
	configRevisionReq.AddCookie(cookie)
	configRevisionResp := httptest.NewRecorder()
	server.ServeHTTP(configRevisionResp, configRevisionReq)
	if configRevisionResp.Code != http.StatusOK {
		t.Fatalf("expected config revision route to succeed, got %d: %s", configRevisionResp.Code, configRevisionResp.Body.String())
	}
	if service.configRevisionTenantID != expectedTenantID || service.configRevisionProjectID != service.projectID || service.configRevisionID != revisionID {
		t.Fatalf("expected config revision context/path, got tenant=%s project=%s revision=%s", service.configRevisionTenantID, service.configRevisionProjectID, service.configRevisionID)
	}
}

func TestProjectDemandLaunchDetailRouteUsesDemandID(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
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
	demandID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/project-demands/"+demandID.String()+"/launch-detail", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected launch detail to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.launchDetailTenantID != uuid.MustParse(auth.DefaultTenantID) || service.launchDetailDemandID != demandID {
		t.Fatalf("expected service to receive tenant/demand from route, got tenant=%s demand=%s", service.launchDetailTenantID, service.launchDetailDemandID)
	}
}

func TestAuditEventsRouteUsesConsoleTenantAndProjectResource(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeAuditService{}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetAuditHandler(audit.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?tenant_id="+uuid.New().String()+"&resource_type=project&resource_id="+projectID.String()+"&limit=25&offset=5", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected audit events route to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	if service.tenantID != expectedTenantID || service.projectID != projectID || service.limit != 25 || service.offset != 5 {
		t.Fatalf("expected console tenant/project/page, got tenant=%s project=%s limit=%d offset=%d", service.tenantID, service.projectID, service.limit, service.offset)
	}

	var events []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &events); err != nil {
		t.Fatalf("expected audit JSON array: %v", err)
	}
	if len(events) != 1 || events[0]["resource_type"] != "project" || events[0]["resource_id"] != projectID.String() {
		t.Fatalf("expected project audit response, got %#v", events)
	}
}

func TestAuditEventsRouteRejectsInvalidResourceFilters(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeAuditService{}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetAuditHandler(audit.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")

	tests := []string{
		"/api/v1/audit/events?resource_type=team&resource_id=" + uuid.New().String(),
		"/api/v1/audit/events?resource_type=project",
		"/api/v1/audit/events?resource_type=project&resource_id=not-a-uuid",
	}
	for _, path := range tests {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(cookie)
		resp := httptest.NewRecorder()

		server.ServeHTTP(resp, req)

		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected %s to fail with 400, got %d: %s", path, resp.Code, resp.Body.String())
		}
	}
	if service.called {
		t.Fatal("expected invalid filters not to call audit service")
	}
}

func TestRuntimeProjectTaskWritebackRoutesUseRuntimeSessionAuth(t *testing.T) {
	runtimeAuth := &routeRuntimeSessionAuth{
		tenantID:      uuid.MustParse(auth.DefaultTenantID),
		runtimeNodeID: uuid.New(),
		sessionID:     uuid.New(),
		nodeID:        "runtime-node-1",
		token:         "runtime-session-token",
	}
	service := &routeProjectService{}
	server := NewServerWithAuthzAndRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		nil,
		nil,
		runtimeAuth,
		&routeAuthorizer{allowed: true},
	)
	server.SetProjectHandler(project.NewHandler(service))
	projectTaskID := uuid.New()
	employeeID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-tasks/"+projectTaskID.String()+"/complete", strings.NewReader(`{
		"digital_employee_id":"`+employeeID.String()+`",
		"conclusion":"证据充分",
		"evidence_refs":["s3://bucket/report.md"],
		"artifact_refs":[],
		"confidence_factors":{"tests":"passed"}
	}`))
	req.Header.Set("Authorization", "Bearer runtime-session-token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected project task complete to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.completeTaskReq.TenantID != runtimeAuth.tenantID || service.completeTaskReq.RuntimeNodeID != runtimeAuth.runtimeNodeID || service.completeTaskReq.ProjectTaskID != projectTaskID || service.completeTaskReq.DigitalEmployeeID != employeeID {
		t.Fatalf("expected runtime context/path/body in complete request, got %#v", service.completeTaskReq)
	}
}

func TestProjectWorkflowSignalRetryRouteUsesConsoleAuth(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeProjectService{projectID: uuid.New()}
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetProjectHandler(project.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	eventID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+service.projectID.String()+"/events/"+eventID.String()+"/retry-workflow-signal", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected retry workflow signal to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.retryWorkflowSignalReq.TenantID != expectedTenantID || service.retryWorkflowSignalReq.ProjectID != service.projectID || service.retryWorkflowSignalReq.EventID != eventID || service.retryWorkflowSignalReq.ActorID != user.ID {
		t.Fatalf("expected retry context/path/user, got %#v", service.retryWorkflowSignalReq)
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
	projectID                 uuid.UUID
	createReq                 project.CreateProjectRequest
	listReq                   project.ListProjectsRequest
	overviewTenantID          uuid.UUID
	overviewProjectID         uuid.UUID
	overviewCalls             int
	eventsTenantID            uuid.UUID
	eventsProjectID           uuid.UUID
	eventsLimit               int32
	eventsOffset              int32
	latestConfigRevisionCalls int
	submitDemandReq           project.SubmitProjectDemandRequest
	routeDecisionTenantID     uuid.UUID
	routeDecisionProjectID    uuid.UUID
	routeDecisionLimit        int32
	resolveDecisionReq        project.ResolveDecisionRequest
	retryWorkflowSignalReq    project.RetryWorkflowSignalRequest
	completeTaskReq           project.CompleteProjectTaskRequest
	createEvidenceReq         project.CreateEvidenceRefServiceRequest
	patchEvidenceReq          project.PatchEvidenceRequest
	budgetSummaryTenantID     uuid.UUID
	budgetSummaryProjectID    uuid.UUID
	createAcceptanceReq       project.CreateAcceptanceServiceRequest
	createArchiveReq          project.CreateArchiveSnapshotServiceRequest
	configRevisionTenantID    uuid.UUID
	configRevisionProjectID   uuid.UUID
	configRevisionID          uuid.UUID
	launchDetailTenantID      uuid.UUID
	launchDetailDemandID      uuid.UUID
	archiveErr                error
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
	s.eventsTenantID = tenantID
	s.eventsProjectID = projectID
	s.eventsLimit = limit
	s.eventsOffset = offset
	return []project.ProjectEvent{{
		ID:             uuid.New(),
		TenantID:       tenantID,
		ProjectID:      projectID,
		SequenceNumber: 1,
		EventType:      project.ProjectEventCreated,
		ActorType:      "human_user",
		ActorID:        uuid.New().String(),
		Payload:        map[string]any{},
	}}, nil
}

func (s *routeProjectService) RetryWorkflowSignal(ctx context.Context, req project.RetryWorkflowSignalRequest) (*project.ProjectEvent, error) {
	s.retryWorkflowSignalReq = req
	return &project.ProjectEvent{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		SequenceNumber: 2,
		EventType:      project.ProjectEventWorkflowSignaled,
		ActorType:      "human_user",
		ActorID:        req.ActorID.String(),
		Payload:        map[string]any{"status": "sent"},
	}, nil
}

func (s *routeProjectService) GetLatestProjectConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (*project.ProjectConfigRevision, error) {
	s.latestConfigRevisionCalls++
	return nil, project.ErrProjectNotFound
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
	s.overviewCalls++
	s.overviewTenantID = tenantID
	s.overviewProjectID = projectID
	projectValue := routeProject(tenantID, projectID, uuid.New())
	return &project.ProjectOverview{
		Project:              projectValue,
		StatusSummary:        project.ProjectStatusSummary{CurrentPhase: string(projectValue.Status)},
		CoordinationWorkflow: project.ProjectCoordinationWorkflow{WorkflowID: projectValue.CoordinationWorkflowID, Status: projectValue.CoordinationStatus},
	}, nil
}

func (s *routeProjectService) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.RouteDecision, error) {
	s.routeDecisionTenantID = tenantID
	s.routeDecisionProjectID = projectID
	s.routeDecisionLimit = limit
	return []project.RouteDecision{{
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

func (s *routeProjectService) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.CoordinationJob, error) {
	return nil, nil
}

func (s *routeProjectService) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.DecisionRequest, error) {
	return nil, nil
}

func (s *routeProjectService) GetDemandLaunchDetail(ctx context.Context, tenantID, demandID uuid.UUID) (*project.DemandLaunchDetail, error) {
	s.launchDetailTenantID = tenantID
	s.launchDetailDemandID = demandID
	projectID := s.ensureProjectID()
	projectValue := routeProject(tenantID, projectID, uuid.New())
	return &project.DemandLaunchDetail{
		Demand:       project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: uuid.New(), Title: "补充验收证据", SourceType: project.DemandSourceManual, Status: project.ProjectDemandStatusPlanningPending},
		Project:      projectValue,
		ProjectTasks: []project.ProjectTask{{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, DemandID: &demandID, Title: "补充验收证据", Status: "pending"}},
	}, nil
}

func (s *routeProjectService) ResolveDecision(ctx context.Context, req project.ResolveDecisionRequest) (*project.DecisionRequest, error) {
	s.resolveDecisionReq = req
	decision := project.DecisionRequest{
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

func (s *routeProjectService) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.ExecutionSummary, error) {
	return nil, nil
}

func (s *routeProjectService) ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.TransferRequest, error) {
	return nil, nil
}

func (s *routeProjectService) CompleteProjectTask(ctx context.Context, req project.CompleteProjectTaskRequest) (*project.ExecutionSummary, error) {
	s.completeTaskReq = req
	summary := project.ExecutionSummary{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         uuid.New(),
		ProjectTaskID:     req.ProjectTaskID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Conclusion:        req.Conclusion,
		EvidenceRefs:      req.EvidenceRefs,
		ArtifactRefs:      req.ArtifactRefs,
		ConfidenceFactors: req.ConfidenceFactors,
	}
	return &summary, nil
}

func (s *routeProjectService) FailProjectTask(ctx context.Context, req project.FailProjectTaskRequest) (*project.ProjectTask, error) {
	return &project.ProjectTask{ID: req.ProjectTaskID, TenantID: req.TenantID, ProjectID: uuid.New(), Status: "failed"}, nil
}

func (s *routeProjectService) RequestProjectTaskTransfer(ctx context.Context, req project.RequestProjectTaskTransferRequest) (*project.TransferRequest, error) {
	return &project.TransferRequest{ID: uuid.New(), TenantID: req.TenantID, ProjectID: uuid.New(), ProjectTaskID: req.ProjectTaskID, RequestedByDigitalEmployeeID: req.DigitalEmployeeID, Reason: req.Reason, Status: "requested"}, nil
}

func (s *routeProjectService) ListEvidence(ctx context.Context, tenantID, projectID uuid.UUID, status *project.EvidenceVerificationStatus, limit, offset int32) ([]project.ProjectEvidenceRef, error) {
	return []project.ProjectEvidenceRef{routeEvidence(tenantID, projectID, uuid.New())}, nil
}

func (s *routeProjectService) CreateEvidence(ctx context.Context, req project.CreateEvidenceRefServiceRequest) (*project.ProjectEvidenceRef, error) {
	s.createEvidenceReq = req
	evidence := routeEvidence(req.TenantID, req.ProjectID, req.ActorID)
	evidence.EvidenceType = req.EvidenceType
	evidence.Title = req.Title
	evidence.SourceType = req.SourceType
	evidence.SourceRef = req.SourceRef
	evidence.SubmittedByType = req.SubmittedByType
	evidence.SubmittedByID = req.SubmittedByID
	evidence.Metadata = req.Metadata
	return &evidence, nil
}

func (s *routeProjectService) PatchEvidence(ctx context.Context, req project.PatchEvidenceRequest) (*project.ProjectEvidenceRef, error) {
	s.patchEvidenceReq = req
	evidence := routeEvidence(req.TenantID, req.ProjectID, req.ActorUserID)
	evidence.ID = req.EvidenceID
	evidence.VerificationStatus = req.VerificationStatus
	evidence.Metadata = req.Metadata
	return &evidence, nil
}

func (s *routeProjectService) ListArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.ProjectArtifactRef, error) {
	return []project.ProjectArtifactRef{{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactType: "log", Title: "执行日志", ObjectRef: "s3://bucket/run.log", RetentionStatus: "retained", Metadata: map[string]any{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}}, nil
}

func (s *routeProjectService) ListReports(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.ProjectReportRef, error) {
	return []project.ProjectReportRef{{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ReportType: "final", Title: "最终报告", ObjectRef: "s3://bucket/final.md", Format: "markdown", GeneratedByType: "human_user", CreatedAt: time.Now().UTC()}}, nil
}

func (s *routeProjectService) ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.ProjectBudgetLedgerEntry, error) {
	return []project.ProjectBudgetLedgerEntry{{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CostType: "tokens", EstimatedCost: "1.00", ActualCost: "0.80", Source: "runtime", CreatedAt: time.Now().UTC()}}, nil
}

func (s *routeProjectService) GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (*project.ProjectBudgetSummary, error) {
	s.budgetSummaryTenantID = tenantID
	s.budgetSummaryProjectID = projectID
	return &project.ProjectBudgetSummary{EstimatedTokens: 1000, ActualTokens: 800, EstimatedCost: "1.00", ActualCost: "0.80", LedgerCount: 1}, nil
}

func (s *routeProjectService) CreateAcceptance(ctx context.Context, req project.CreateAcceptanceServiceRequest) (*project.ProjectAcceptanceRecord, error) {
	s.createAcceptanceReq = req
	record := routeAcceptance(req.TenantID, req.ProjectID, req.AcceptedByUserID)
	record.Status = req.Status
	record.Conclusion = req.Conclusion
	record.EvidenceRefIDs = req.EvidenceRefIDs
	record.ReportRefIDs = req.ReportRefIDs
	record.UnresolvedRisks = req.UnresolvedRisks
	return &record, nil
}

func (s *routeProjectService) GetAcceptance(ctx context.Context, tenantID, projectID uuid.UUID) (*project.ProjectAcceptanceRecord, error) {
	record := routeAcceptance(tenantID, projectID, uuid.New())
	return &record, nil
}

func (s *routeProjectService) GetArchivePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*project.ProjectArchivePreview, error) {
	return &project.ProjectArchivePreview{ProjectID: projectID, EvidenceCount: 1, ArtifactCount: 1, ReportCount: 1, BlockedReasons: []any{}, EstimatedObjectRefs: []any{"s3://bucket/final.md"}}, nil
}

func (s *routeProjectService) CreateArchiveSnapshot(ctx context.Context, req project.CreateArchiveSnapshotServiceRequest) (*project.ProjectArchiveSnapshot, error) {
	s.createArchiveReq = req
	snapshot := routeArchiveSnapshot(req.TenantID, req.ProjectID, req.CreatedByUserID)
	snapshot.SnapshotType = req.SnapshotType
	snapshot.ObjectRef = routeStringPtr(req.ObjectRef)
	snapshot.Summary = routeStringPtr(req.Summary)
	return &snapshot, nil
}

func (s *routeProjectService) ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.ProjectArchiveSnapshot, error) {
	return []project.ProjectArchiveSnapshot{routeArchiveSnapshot(tenantID, projectID, uuid.New())}, nil
}

func (s *routeProjectService) ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.ProjectConfigRevision, error) {
	return []project.ProjectConfigRevision{routeConfigRevision(tenantID, projectID, uuid.New())}, nil
}

func (s *routeProjectService) GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*project.ProjectConfigRevision, error) {
	s.configRevisionTenantID = tenantID
	s.configRevisionProjectID = projectID
	s.configRevisionID = revisionID
	revision := routeConfigRevision(tenantID, projectID, uuid.New())
	revision.ID = revisionID
	return &revision, nil
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

func routeEvidence(tenantID, projectID, userID uuid.UUID) project.ProjectEvidenceRef {
	now := time.Now().UTC()
	return project.ProjectEvidenceRef{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		ProjectID:          projectID,
		EvidenceType:       "test_report",
		Title:              "验收测试报告",
		SourceType:         "artifact",
		SourceRef:          "s3://bucket/report.md",
		SubmittedByType:    "human_user",
		SubmittedByID:      &userID,
		VerificationStatus: project.EvidenceVerificationStatusSubmitted,
		Metadata:           map[string]any{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func routeAcceptance(tenantID, projectID, userID uuid.UUID) project.ProjectAcceptanceRecord {
	return project.ProjectAcceptanceRecord{
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

func routeArchiveSnapshot(tenantID, projectID, userID uuid.UUID) project.ProjectArchiveSnapshot {
	return project.ProjectArchiveSnapshot{
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

func routeConfigRevision(tenantID, projectID, userID uuid.UUID) project.ProjectConfigRevision {
	return project.ProjectConfigRevision{
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

func routeStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

type routeAuditService struct {
	called    bool
	tenantID  uuid.UUID
	projectID uuid.UUID
	limit     int
	offset    int
}

func (s *routeAuditService) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int) ([]*audit.Event, error) {
	s.called = true
	s.tenantID = tenantID
	s.projectID = projectID
	s.limit = limit
	s.offset = offset
	return []*audit.Event{
		{
			ID:           uuid.New(),
			TenantID:     tenantID,
			EventType:    "project.created",
			ActorType:    "human_user",
			ActorID:      uuid.New().String(),
			ResourceType: "project",
			ResourceID:   projectID.String(),
			Action:       "project.create",
			Details:      map[string]any{"source": "route-test"},
			IPAddress:    "127.0.0.1",
			CreatedAt:    time.Now().UTC(),
		},
	}, nil
}

type routeRuntimeSessionAuth struct {
	tenantID      uuid.UUID
	runtimeNodeID uuid.UUID
	sessionID     uuid.UUID
	nodeID        string
	token         string
}

func (s *routeRuntimeSessionAuth) ValidateRuntimeSession(ctx context.Context, token string) (*runtimepkg.RuntimeSessionValidation, error) {
	if token != s.token {
		return nil, context.Canceled
	}
	return &runtimepkg.RuntimeSessionValidation{
		SessionID:     s.sessionID,
		TenantID:      s.tenantID,
		RuntimeNodeID: s.runtimeNodeID,
		NodeID:        s.nodeID,
		ExpiresAt:     time.Now().Add(time.Hour),
	}, nil
}
