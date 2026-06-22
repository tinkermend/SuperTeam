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

func TestProjectHandlerMapsUnauthorizedTeamScope(t *testing.T) {
	service := &handlerTestService{createErr: ErrUnauthorizedProjectTeamScope}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{
		"name":"未授权团队项目",
		"goal":"验证团队授权边界",
		"human_owner_user_id":"`+uuid.New().String()+`",
		"team_id":"`+uuid.New().String()+`"
	}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, uuid.New()))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.New()))
	resp := httptest.NewRecorder()

	handler.CreateProject(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected unauthorized team scope to return 403, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "当前用户无权使用该团队创建项目。") {
		t.Fatalf("expected forbidden message in body, got %q", resp.Body.String())
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

func TestProjectHandlerSubmitsDemandReviewerPreference(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	reviewerID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/demands", strings.NewReader(`{
		"title":"审查 PR",
		"content":"统计并审查 PR",
		"reviewer_user_id":"`+reviewerID.String()+`",
		"reviewer_selection_reason":"user_selected"
	}`))
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.SubmitDemand(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected submit demand 201, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.submitDemandReq.ReviewerUserID == nil || *service.submitDemandReq.ReviewerUserID != reviewerID {
		t.Fatalf("expected reviewer user id to pass through, got %#v", service.submitDemandReq.ReviewerUserID)
	}
	if service.submitDemandReq.ReviewerSelectionReason != ReviewerSelectionUserSelected {
		t.Fatalf("expected reviewer reason to pass through, got %q", service.submitDemandReq.ReviewerSelectionReason)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode submit demand response: %v", err)
	}
	reviewer, ok := body["reviewer"].(map[string]any)
	if !ok {
		t.Fatalf("expected reviewer object in response, got %#v", body)
	}
	if reviewer["reviewer_user_id"] != reviewerID.String() || reviewer["selection_reason"] != string(ReviewerSelectionUserSelected) {
		t.Fatalf("unexpected reviewer response: %#v", reviewer)
	}
}

func TestProjectHandlerListsPlanRevisions(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	revisionID := uuid.New()
	service := &handlerTestService{planRevisions: []PlanRevision{{
		ID:              revisionID,
		TenantID:        tenantID,
		ProjectID:       projectID,
		DemandID:        demandID,
		RevisionNumber:  2,
		Status:          PlanRevisionStatusPendingReview,
		Payload:         map[string]any{"summary": "复核生产巡检计划"},
		PlanFingerprint: "fingerprint",
		ReviewRequired:  true,
		CreatedTaskIDs:  []uuid.UUID{},
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}}}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/plan-revisions?demand_id="+demandID.String()+"&limit=5", nil)
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.ListPlanRevisions(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected list plan revisions 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.planRevisionListReq.TenantID != tenantID || service.planRevisionListReq.ProjectID != projectID || service.planRevisionListReq.Limit != 5 {
		t.Fatalf("unexpected list request: %#v", service.planRevisionListReq)
	}
	if service.planRevisionListReq.DemandID == nil || *service.planRevisionListReq.DemandID != demandID {
		t.Fatalf("expected demand filter to pass through, got %#v", service.planRevisionListReq.DemandID)
	}
	var body []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 1 || body[0]["id"] != revisionID.String() || body[0]["status"] != PlanRevisionStatusPendingReview {
		t.Fatalf("unexpected response body: %#v", body)
	}
	payload, ok := body[0]["payload"].(map[string]any)
	if !ok || payload["summary"] != "复核生产巡检计划" {
		t.Fatalf("expected payload to be preserved, got %#v", body[0]["payload"])
	}
}

func TestProjectHandlerGetsPlanRevision(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	revisionID := uuid.New()
	service := &handlerTestService{planRevisions: []PlanRevision{{
		ID:              revisionID,
		TenantID:        tenantID,
		ProjectID:       projectID,
		DemandID:        uuid.New(),
		RevisionNumber:  1,
		Status:          PlanRevisionStatusAccepted,
		Payload:         map[string]any{},
		PlanFingerprint: "fingerprint",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}}}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/plan-revisions/"+revisionID.String(), nil)
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String(), "planRevisionId": revisionID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.GetPlanRevision(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected get plan revision 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.planRevisionTenantID != tenantID || service.planRevisionProjectID != projectID || service.planRevisionID != revisionID {
		t.Fatalf("unexpected get identifiers: tenant=%s project=%s revision=%s", service.planRevisionTenantID, service.planRevisionProjectID, service.planRevisionID)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["id"] != revisionID.String() || body["status"] != PlanRevisionStatusAccepted || body["plan_fingerprint"] != "fingerprint" {
		t.Fatalf("unexpected response body: %#v", body)
	}
}

func TestHandlerListProjectTaskDispatchGates(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	gateID := uuid.New()
	selectedEmployeeID := uuid.New()
	retryAfter := time.Date(2026, 6, 21, 12, 2, 0, 0, time.UTC)
	service := &handlerTestService{
		dispatchGates: []PreDispatchGateResult{
			{
				ID:                 gateID,
				TenantID:           tenantID,
				ProjectID:          projectID,
				ProjectTaskID:      taskID,
				SelectedEmployeeID: selectedEmployeeID,
				AttemptNo:          1,
				DispatchReason:     DispatchReasonRootReady,
				Status:             PreDispatchGateStatusRetryLater,
				CheckedAt:          time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
				Checks:             []PreDispatchGateCheck{{Key: "runtime.ready", Status: "failed", Details: map[string]any{"node_online": false}}},
				Blockers:           []PreDispatchGateBlocker{{Key: "runtime.node_offline", Severity: "transient", Retryable: true, Details: map[string]any{"node_id": "runtime-node-1"}}},
				HumanActionRequest: map[string]any{},
				RetryAfter:         &retryAfter,
			},
		},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/tasks/"+taskID.String()+"/dispatch-gates", nil)
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String(), "taskId": taskID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.ListProjectTaskDispatchGates(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected list dispatch gates 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.dispatchGateListReq.TenantID != tenantID ||
		service.dispatchGateListReq.ProjectID != projectID ||
		service.dispatchGateListReq.ProjectTaskID != taskID ||
		service.dispatchGateListReq.Limit != 50 {
		t.Fatalf("unexpected dispatch gate list request: %#v", service.dispatchGateListReq)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode dispatch gate response: %v", err)
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one dispatch gate item, got %#v", body["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object dispatch gate item, got %#v", items[0])
	}
	if item["id"] != gateID.String() || item["status"] != string(PreDispatchGateStatusRetryLater) || item["selected_employee_id"] != selectedEmployeeID.String() {
		t.Fatalf("unexpected dispatch gate item: %#v", item)
	}
	blockers, ok := item["blockers"].([]any)
	if !ok || len(blockers) != 1 {
		t.Fatalf("expected one blocker, got %#v", item["blockers"])
	}
	blocker := blockers[0].(map[string]any)
	if blocker["key"] != "runtime.node_offline" || blocker["retryable"] != true {
		t.Fatalf("unexpected blocker response: %#v", blocker)
	}
	if item["retry_after"] != retryAfter.Format(time.RFC3339Nano) {
		t.Fatalf("expected retry_after timestamp, got %#v", item["retry_after"])
	}
}

func TestProjectHandlerDemandResponseIncludesNullReviewerWhenAbsent(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/demands", strings.NewReader(`{"title":"补充验收证据"}`))
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.SubmitDemand(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected submit demand 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode submit demand response: %v", err)
	}
	reviewer, ok := body["reviewer"]
	if !ok {
		t.Fatalf("expected stable reviewer key in response: %#v", body)
	}
	if reviewer != nil {
		t.Fatalf("expected null reviewer when absent, got %#v", reviewer)
	}
}

func TestListWorkflowInstancesReturnsSummaries(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	demandID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	remaining := int32(600)
	dueAt := time.Now().UTC().Add(10 * time.Minute)
	service := &handlerTestService{
		workflowInstances: []WorkflowInstanceSummary{{
			DemandID:                  demandID,
			ProjectID:                 projectID,
			ProjectName:               "生产巡检",
			Title:                     "支付成功率下降",
			SubmittedByUserID:         actorID,
			SubmittedByDisplayName:    "张晓明",
			Status:                    WorkflowInstanceStatusRunning,
			StatusReason:              "任务执行中",
			CreatedAt:                 time.Now().UTC(),
			UpdatedAt:                 time.Now().UTC(),
			SelectedCoordinationJobID: &jobID,
			Progress: WorkflowInstanceProgress{
				TotalNodes:   2,
				RunningNodes: 1,
				PlannedNodes: 1,
			},
			CurrentBlocker: &WorkflowInstanceCurrentBlocker{
				Type:  "task",
				Title: "等待数据库巡检",
			},
			Priority: &WorkflowInstancePriority{Value: "p1", Label: "P1", Source: "source_refs.priority"},
			Risk:     &WorkflowInstanceRisk{Level: "high", Label: "高风险", Source: "project_tasks.risk_level"},
			SLA:      &WorkflowInstanceSLA{DueAt: &dueAt, RemainingSeconds: &remaining, Breached: false, Label: "剩余 10 分钟", Source: "source_refs.sla_due_at"},
			RecentEvent: &WorkflowInstanceRecentEvent{
				EventType:  string(ProjectEventDecisionRequested),
				Summary:    "已创建恢复决策请求",
				OccurredAt: time.Now().UTC(),
			},
		}},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances?status=running&limit=10&q=支付", nil)
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.ListWorkflowInstances(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected workflow instances 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.workflowInstancesReq.TenantID != tenantID || service.workflowInstancesReq.ActorUserID != actorID || service.workflowInstancesReq.Query != "支付" || service.workflowInstancesReq.Limit != 10 {
		t.Fatalf("unexpected workflow instance request: %#v", service.workflowInstancesReq)
	}
	if service.workflowInstancesReq.Status == nil || *service.workflowInstancesReq.Status != WorkflowInstanceStatusRunning {
		t.Fatalf("expected running status filter, got %#v", service.workflowInstancesReq.Status)
	}
	var body []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 1 || body[0]["demand_id"] != demandID.String() || body[0]["status"] != "running" {
		t.Fatalf("unexpected workflow instance body: %#v", body)
	}
	progress := body[0]["progress"].(map[string]any)
	if progress["total_nodes"].(float64) != 2 || progress["running_nodes"].(float64) != 1 {
		t.Fatalf("unexpected progress: %#v", progress)
	}
	if body[0]["priority"].(map[string]any)["label"] != "P1" {
		t.Fatalf("expected priority in response: %#v", body[0])
	}
	if body[0]["risk"].(map[string]any)["level"] != "high" {
		t.Fatalf("expected risk in response: %#v", body[0])
	}
	if body[0]["sla"].(map[string]any)["label"] != "剩余 10 分钟" {
		t.Fatalf("expected sla in response: %#v", body[0])
	}
	if body[0]["recent_event"].(map[string]any)["event_type"] != string(ProjectEventDecisionRequested) {
		t.Fatalf("expected recent event in response: %#v", body[0])
	}
	if progress["planned_nodes"].(float64) != 1 {
		t.Fatalf("expected planned_nodes in progress: %#v", progress)
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

func TestProjectHandlerListsExecutionTrace(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	eventID := uuid.New()
	nodeID := uuid.New()
	teamID := uuid.New()
	actor := uuid.New().String()
	providerType := "codex"
	providerSessionID := "session-123"
	inputSummary := "Run project task"
	outputSummary := "Task completed"
	errorFamily := "provider"
	errorCode := "E_PROVIDER"
	errorMessage := "provider failed"
	retryable := false
	attemptRetryable := true
	latestErrorFamily := "runtime"
	failureFamily := "timeout"
	startedAt := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(5 * time.Minute)
	occurredAt := finishedAt.Add(-time.Minute)
	createdAt := finishedAt
	summaryID := uuid.New()
	service := &handlerTestService{
		executionTrace: &ProjectExecutionTrace{
			ProjectID: projectID,
			Summary: ProjectExecutionTraceSummary{
				AttemptCount:             1,
				FailedAttemptCount:       0,
				HumanReviewRequiredCount: 0,
				ArtifactRefCount:         1,
				EvidenceRefCount:         1,
				LatestErrorFamily:        &latestErrorFamily,
			},
			Attempts: []ProjectExecutionTraceAttempt{{
				ProjectTaskID:     taskID,
				AttemptID:         attemptID,
				AttemptNo:         1,
				Status:            string(ProjectTaskAttemptStatusSucceeded),
				RuntimeNodeID:     &nodeID,
				ProviderType:      &providerType,
				ProviderSessionID: &providerSessionID,
				StartedAt:         &startedAt,
				FinishedAt:        &finishedAt,
				FailureFamily:     &failureFamily,
				Retryable:         &attemptRetryable,
				Events: []ExecutionLedgerEvent{{
					ID:                   eventID,
					TenantID:             tenantID,
					TeamID:               &teamID,
					ProjectID:            projectID,
					ProjectTaskID:        &taskID,
					ProjectTaskAttemptID: &attemptID,
					EventType:            ExecutionLedgerEventAttemptCompleted,
					SourceType:           "project_task_attempt",
					SourceID:             attemptID.String(),
					ActorType:            "digital_employee",
					ActorID:              &actor,
					RuntimeNodeID:        &nodeID,
					ProviderType:         &providerType,
					ProviderSessionID:    &providerSessionID,
					InputSummary:         &inputSummary,
					OutputSummary:        &outputSummary,
					ErrorFamily:          &errorFamily,
					ErrorCode:            &errorCode,
					ErrorMessage:         &errorMessage,
					Retryable:            &retryable,
					ArtifactRefs:         []any{"artifact-runtime-log"},
					EvidenceRefs:         []any{"s3://bucket/e2e-report.md"},
					Metadata:             map[string]any{"source": "runtime"},
					OccurredAt:           occurredAt,
					CreatedAt:            createdAt,
				}},
				Summary: &ProjectExecutionTraceAttemptSummary{
					ExecutionSummaryID:  summaryID,
					Conclusion:          "证据充分",
					RequiresHumanReview: false,
					ArtifactRefs:        []any{"artifact-runtime-log"},
					EvidenceRefs:        []any{"s3://bucket/e2e-report.md"},
					CreatedAt:           createdAt,
				},
			}},
		},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/execution-trace?project_task_id="+taskID.String()+"&attempt_id="+attemptID.String()+"&event_type=%20attempt.completed%20&error_family=%20provider%20&limit=7&offset=3", nil)
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.GetExecutionTrace(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected execution trace 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.executionTraceReq.TenantID != tenantID || service.executionTraceReq.ProjectID != projectID || service.executionTraceReq.Limit != 7 || service.executionTraceReq.Offset != 3 {
		t.Fatalf("unexpected execution trace request identity/page: %#v", service.executionTraceReq)
	}
	if service.executionTraceReq.ProjectTaskID == nil || *service.executionTraceReq.ProjectTaskID != taskID {
		t.Fatalf("expected project task filter, got %#v", service.executionTraceReq.ProjectTaskID)
	}
	if service.executionTraceReq.ProjectTaskAttemptID == nil || *service.executionTraceReq.ProjectTaskAttemptID != attemptID {
		t.Fatalf("expected attempt filter, got %#v", service.executionTraceReq.ProjectTaskAttemptID)
	}
	if service.executionTraceReq.EventType == nil || *service.executionTraceReq.EventType != ExecutionLedgerEventAttemptCompleted {
		t.Fatalf("expected trimmed event type filter, got %#v", service.executionTraceReq.EventType)
	}
	if service.executionTraceReq.ErrorFamily == nil || *service.executionTraceReq.ErrorFamily != "provider" {
		t.Fatalf("expected trimmed error family filter, got %#v", service.executionTraceReq.ErrorFamily)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode execution trace response: %v", err)
	}
	if body["project_id"] != projectID.String() {
		t.Fatalf("unexpected project id in response: %#v", body)
	}
	summary, ok := body["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary object, got %#v", body["summary"])
	}
	if summary["attempt_count"] != float64(1) || summary["artifact_ref_count"] != float64(1) || summary["latest_error_family"] != latestErrorFamily {
		t.Fatalf("unexpected trace summary: %#v", summary)
	}
	attempts, ok := body["attempts"].([]any)
	if !ok || len(attempts) != 1 {
		t.Fatalf("expected one attempt, got %#v", body["attempts"])
	}
	attempt, ok := attempts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected attempt object, got %#v", attempts[0])
	}
	if attempt["project_task_id"] != taskID.String() || attempt["attempt_id"] != attemptID.String() || attempt["status"] != string(ProjectTaskAttemptStatusSucceeded) {
		t.Fatalf("unexpected attempt response: %#v", attempt)
	}
	if attempt["started_at"] != startedAt.Format(time.RFC3339) || attempt["finished_at"] != finishedAt.Format(time.RFC3339) {
		t.Fatalf("unexpected attempt time response: %#v", attempt)
	}
	events, ok := attempt["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected one ledger event, got %#v", attempt["events"])
	}
	event, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("expected event object, got %#v", events[0])
	}
	if event["id"] != eventID.String() || event["event_type"] != ExecutionLedgerEventAttemptCompleted || event["project_task_attempt_id"] != attemptID.String() {
		t.Fatalf("unexpected event response: %#v", event)
	}
	if event["occurred_at"] != occurredAt.Format(time.RFC3339) || event["created_at"] != createdAt.Format(time.RFC3339) {
		t.Fatalf("unexpected event times: %#v", event)
	}
	artifactRefs, ok := event["artifact_refs"].([]any)
	if !ok || len(artifactRefs) != 1 || artifactRefs[0] != "artifact-runtime-log" {
		t.Fatalf("unexpected event artifact refs: %#v", event["artifact_refs"])
	}
	attemptSummary, ok := attempt["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected attempt summary object, got %#v", attempt["summary"])
	}
	if attemptSummary["execution_summary_id"] != summaryID.String() || attemptSummary["requires_human_review"] != false {
		t.Fatalf("unexpected attempt summary: %#v", attemptSummary)
	}
}

func TestProjectHandlerGetsDemandLaunchDetail(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	service := &handlerTestService{launchDetailProjectID: projectID}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/project-demands/"+demandID.String()+"/launch-detail", nil)
	req = withProjectRouteParams(req, map[string]string{"demandId": demandID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.GetDemandLaunchDetail(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected launch detail 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.launchDetailTenantID != tenantID || service.launchDetailDemandID != demandID {
		t.Fatalf("unexpected launch detail request: tenant=%s demand=%s", service.launchDetailTenantID, service.launchDetailDemandID)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode launch detail response: %v", err)
	}
	if body["demand"].(map[string]any)["id"] != demandID.String() {
		t.Fatalf("expected demand id in detail response: %#v", body)
	}
	if body["project"].(map[string]any)["id"] != projectID.String() {
		t.Fatalf("expected project id in detail response: %#v", body)
	}
	if _, ok := body["reviewer"]; !ok {
		t.Fatalf("expected reviewer key in launch detail response: %#v", body)
	}
	if len(body["project_tasks"].([]any)) != 1 {
		t.Fatalf("expected project tasks in launch detail: %#v", body)
	}
}

func TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	taskID := uuid.New()
	blockerID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	decisionID := uuid.New()
	eventID := uuid.New()
	stageIndex := int32(1)
	service := &handlerTestService{
		taskGraph: ProjectTaskGraph{
			Nodes: []ProjectTaskGraphNode{{
				Task: ProjectTask{
					ID:                        taskID,
					TenantID:                  tenantID,
					ProjectID:                 projectID,
					DemandID:                  &demandID,
					Title:                     "分析需求",
					Summary:                   strPtr("拆解任务图"),
					Status:                    "blocked",
					AssignedDigitalEmployeeID: &employeeID,
					RiskLevel:                 strPtr("medium"),
					RequiresHumanApproval:     true,
					CoordinationJobID:         &jobID,
					RouteDecisionID:           &routeID,
					PlannedTaskKey:            strPtr("t2"),
					TaskKind:                  strPtr("analysis"),
					StageIndex:                &stageIndex,
					ExpectedOutputs:           []any{"execution_summary"},
					InputRequirements:         map[string]any{"scope": "demand"},
					HandoffContract:           map[string]any{"required_refs": []any{"evidence"}},
					PlannerMetadata:           map[string]any{"provider": "deepseek"},
					UpdatedAt:                 time.Now().UTC(),
				},
				StatusReason: "等待上游任务完成",
				CurrentBlocker: &WorkflowInstanceCurrentBlocker{
					Type:       "project_task",
					Title:      "等待数据库巡检",
					ResourceID: &blockerID,
				},
			}},
			StageSummaries: []ProjectTaskGraphStageSummary{{
				StageIndex:     1,
				Title:          "第 1 阶段",
				TotalNodes:     1,
				BlockedNodes:   1,
				RunningNodes:   0,
				CompletedNodes: 0,
			}},
			Edges: []ProjectTaskGraphEdge{{
				DependentTaskID:   taskID,
				BlockerTaskID:     blockerID,
				CoordinationJobID: &jobID,
				EdgeStatus:        "blocked",
			}},
			Employees: []ProjectTaskGraphEmployee{{
				DigitalEmployeeID: employeeID,
				DisplayName:       "执行员工",
				ProjectRole:       ProjectRoleExecutor,
				Status:            "active",
			}},
			Runs: []ProjectTaskGraphRun{{
				ProjectTaskID:        taskID,
				DigitalEmployeeRunID: &runID,
				RuntimeTaskID:        &runtimeTaskID,
				Status:               "assigned",
				ProviderType:         "codex",
			}},
			ExecutionSummaries: []ExecutionSummary{{
				ID:                uuid.New(),
				TenantID:          tenantID,
				ProjectID:         projectID,
				ProjectTaskID:     taskID,
				DigitalEmployeeID: employeeID,
				Conclusion:        "已完成分析",
				EvidenceRefs:      []any{"evidence"},
				CreatedAt:         time.Now().UTC(),
			}},
			RecentEvents: []ProjectEvent{{
				ID:             eventID,
				TenantID:       tenantID,
				ProjectID:      projectID,
				SequenceNumber: 1,
				EventType:      ProjectEventTaskCreated,
				ActorType:      "project_coordinator",
				ActorID:        taskID.String(),
				Payload:        map[string]any{"project_task_id": taskID.String()},
				CreatedAt:      time.Now().UTC(),
			}},
			DecisionRequests: []DecisionRequest{{
				ID:                decisionID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				ApprovalRequestID: uuid.New(),
				CoordinationJobID: &jobID,
				ProjectTaskID:     &taskID,
				TargetUserID:      actorID,
				DecisionType:      "task_failure_recovery",
				TitleSnapshot:     "任务失败需要恢复决策",
				StatusSnapshot:    "pending",
				CreatedAt:         time.Now().UTC(),
				UpdatedAt:         time.Now().UTC(),
			}},
		},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/task-graph?coordination_job_id="+jobID.String(), nil)
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.GetProjectTaskGraph(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected task graph 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.taskGraphReq.TenantID != tenantID || service.taskGraphReq.ProjectID != projectID || service.taskGraphReq.CoordinationJobID == nil || *service.taskGraphReq.CoordinationJobID != jobID {
		t.Fatalf("unexpected graph request: %#v", service.taskGraphReq)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode task graph response: %v", err)
	}
	nodes := body["nodes"].([]any)
	edges := body["edges"].([]any)
	decisions := body["decision_requests"].([]any)
	if len(nodes) != 1 || len(edges) != 1 || len(decisions) != 1 {
		t.Fatalf("expected non-empty graph response, got %#v", body)
	}
	node := nodes[0].(map[string]any)
	if node["id"] != taskID.String() || node["planned_task_key"] != "t2" || node["coordination_job_id"] != jobID.String() || node["task_kind"] != "analysis" {
		t.Fatalf("unexpected node response: %#v", node)
	}
	if node["input_requirements"].(map[string]any)["scope"] != "demand" || node["planner_metadata"].(map[string]any)["provider"] != "deepseek" {
		t.Fatalf("expected task graph contracts and metadata, got %#v", node)
	}
	if node["status_reason"] != "等待上游任务完成" {
		t.Fatalf("expected status reason on graph node: %#v", node)
	}
	if node["current_blocker"].(map[string]any)["title"] != "等待数据库巡检" {
		t.Fatalf("expected current blocker on graph node: %#v", node)
	}
	stageSummaries := body["stage_summaries"].([]any)
	if len(stageSummaries) != 1 || stageSummaries[0].(map[string]any)["title"] != "第 1 阶段" {
		t.Fatalf("expected stage summaries in graph response: %#v", body)
	}
	edge := edges[0].(map[string]any)
	if edge["dependent_task_id"] != taskID.String() || edge["blocker_task_id"] != blockerID.String() || edge["edge_status"] != "blocked" {
		t.Fatalf("unexpected edge response: %#v", edge)
	}
	if len(body["employees"].([]any)) != 1 || len(body["runs"].([]any)) != 1 || len(body["execution_summaries"].([]any)) != 1 || len(body["recent_events"].([]any)) != 1 {
		t.Fatalf("expected graph sidecars in response, got %#v", body)
	}
}

func TestGetProjectTaskGraphRejectsMissingFilter(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/task-graph", nil)
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.GetProjectTaskGraph(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected missing graph filter to return 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.taskGraphCalls != 0 {
		t.Fatalf("expected missing graph filter not to call service, got %d calls", service.taskGraphCalls)
	}
}

func TestGetProjectTaskLivenessReturnsNextAction(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	service := &handlerTestService{
		taskLiveness: []ProjectTaskLiveness{{
			ProjectTaskID:    taskID,
			Liveness:         ProjectTaskLivenessWaitingHuman,
			Reason:           HumanWaitReasonMissingContext,
			CurrentAttemptID: &attemptID,
			AttemptStatus:    ProjectTaskAttemptStatusWaitingHuman,
			NextAction:       "human response",
		}},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/tasks/"+taskID.String()+"/liveness", nil)
	req = withProjectRouteParams(req, map[string]string{"projectId": projectID.String(), "taskId": taskID.String()})
	req = withConsoleContext(req, tenantID, actorID)
	resp := httptest.NewRecorder()

	handler.GetProjectTaskLiveness(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected task liveness 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.taskLivenessTenantID != tenantID || service.taskLivenessProjectID != projectID {
		t.Fatalf("unexpected liveness request tenant/project: %s/%s", service.taskLivenessTenantID, service.taskLivenessProjectID)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode liveness response: %v", err)
	}
	if body["project_task_id"] != taskID.String() || body["liveness"] != ProjectTaskLivenessWaitingHuman || body["reason"] != HumanWaitReasonMissingContext {
		t.Fatalf("unexpected liveness response: %#v", body)
	}
	if body["next_action"].(map[string]any)["source"] != "human response" {
		t.Fatalf("expected next action source, got %#v", body["next_action"])
	}
	if body["attempt"].(map[string]any)["status"] != ProjectTaskAttemptStatusWaitingHuman {
		t.Fatalf("expected attempt status, got %#v", body["attempt"])
	}
	if body["is_terminal"].(bool) {
		t.Fatalf("waiting human liveness must not be terminal: %#v", body)
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
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

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
		Status:                    ProjectTaskStatusQueued,
		AssignedDigitalEmployeeID: &employeeID,
	})
	runID := bindTaskToRuntimeRun(repo, 0, runtimeNodeID)
	attemptID := uuid.New()
	leaseToken := "lease-token-1"
	repo.tasks[0].CurrentAttemptID = &attemptID
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:                   attemptID,
		TenantID:             tenantID,
		ProjectTaskID:        taskID,
		AttemptNo:            1,
		Status:               ProjectTaskAttemptStatusQueued,
		DigitalEmployeeRunID: &runID,
		RuntimeNodeID:        &runtimeNodeID,
		LeaseToken:           leaseToken,
		IdempotencyKey:       "project-task:" + taskID.String(),
		CreatedAt:            time.Now().UTC(),
		UpdatedAt:            time.Now().UTC(),
	})

	wrongRuntimeReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/complete", strings.NewReader(`{
		"project_task_id":"`+taskID.String()+`",
		"lease_token":"`+leaseToken+`",
		"runtime_node_id":"`+runtimeNodeID.String()+`",
		"idempotency_key":"attempt-complete-wrong-runtime",
		"conclusion":"错误 Runtime 尝试写回"
	}`))
	wrongRuntimeReq = withProjectRouteParams(wrongRuntimeReq, map[string]string{"attemptId": attemptID.String()})
	wrongRuntimeReq = withRuntimeContext(wrongRuntimeReq, tenantID, uuid.New())
	wrongRuntimeResp := httptest.NewRecorder()

	handler.CompleteProjectTaskAttempt(wrongRuntimeResp, wrongRuntimeReq)

	if wrongRuntimeResp.Code != http.StatusForbidden {
		t.Fatalf("expected wrong runtime writeback to return 403, got %d: %s", wrongRuntimeResp.Code, wrongRuntimeResp.Body.String())
	}
	if len(repo.executionSummaries) != 0 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 0 {
		t.Fatalf("expected wrong runtime writeback to have no side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}

	coordinator.completedSignalErr = errors.New("temporal unavailable")
	completeReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/complete", strings.NewReader(`{
		"project_task_id":"`+taskID.String()+`",
		"lease_token":"`+leaseToken+`",
		"runtime_node_id":"`+runtimeNodeID.String()+`",
		"idempotency_key":"attempt-complete-success",
		"conclusion":"证据充分",
		"evidence_refs":["s3://bucket/e2e-report.md"],
		"artifact_refs":["artifact-runtime-log"],
		"confidence_factors":{"tests":"passed"},
		"recommended_next_action":"提交负责人验收"
	}`))
	completeReq = withProjectRouteParams(completeReq, map[string]string{"attemptId": attemptID.String()})
	completeReq = withRuntimeContext(completeReq, tenantID, runtimeNodeID)
	completeResp := httptest.NewRecorder()

	handler.CompleteProjectTaskAttempt(completeResp, completeReq)

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

func TestStartProjectTaskAttemptHandlerBuildsServiceRequest(t *testing.T) {
	tenantID := uuid.New()
	attemptID := uuid.New()
	taskID := uuid.New()
	nodeID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	body := strings.NewReader(`{
		"project_task_id":"` + taskID.String() + `",
		"runtime_node_id":"` + nodeID.String() + `",
		"lease_token":"lease-token-1",
		"idempotency_key":"attempt-start-1",
		"provider_session_id":"provider-session-1"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/started", body)
	req = withProjectRouteParams(req, map[string]string{"attemptId": attemptID.String()})
	req = withRuntimeContext(req, tenantID, nodeID)
	resp := httptest.NewRecorder()

	handler.StartProjectTaskAttempt(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected started writeback to return 202, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.startAttemptReq.AttemptID != attemptID || service.startAttemptReq.ProjectTaskID != taskID || service.startAttemptReq.RuntimeNodeID != nodeID {
		t.Fatalf("unexpected started request identity: %#v", service.startAttemptReq)
	}
	if service.startAttemptReq.LeaseToken != "lease-token-1" || service.startAttemptReq.IdempotencyKey != "attempt-start-1" {
		t.Fatalf("unexpected started request lease/idempotency: %#v", service.startAttemptReq)
	}
	if service.startAttemptReq.ProviderSessionID == nil || *service.startAttemptReq.ProviderSessionID != "provider-session-1" {
		t.Fatalf("expected provider session id to be forwarded, got %#v", service.startAttemptReq.ProviderSessionID)
	}
}

func TestRenewProjectTaskAttemptLeaseHandlerBuildsServiceRequest(t *testing.T) {
	tenantID := uuid.New()
	attemptID := uuid.New()
	taskID := uuid.New()
	nodeID := uuid.New()
	leaseExpiresAt := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Second)
	service := &handlerTestService{}
	handler := NewHandler(service)
	body := strings.NewReader(`{
		"project_task_id":"` + taskID.String() + `",
		"runtime_node_id":"` + nodeID.String() + `",
		"lease_token":"lease-token-2",
		"idempotency_key":"attempt-lease-1",
		"lease_expires_at":"` + leaseExpiresAt.Format(time.RFC3339) + `"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/lease", body)
	req = withProjectRouteParams(req, map[string]string{"attemptId": attemptID.String()})
	req = withRuntimeContext(req, tenantID, nodeID)
	resp := httptest.NewRecorder()

	handler.RenewProjectTaskAttemptLease(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected lease writeback to return 204, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.renewAttemptLeaseReq.AttemptID != attemptID || service.renewAttemptLeaseReq.ProjectTaskID != taskID || service.renewAttemptLeaseReq.RuntimeNodeID != nodeID {
		t.Fatalf("unexpected lease request identity: %#v", service.renewAttemptLeaseReq)
	}
	if service.renewAttemptLeaseReq.LeaseExpiresAt == nil || !service.renewAttemptLeaseReq.LeaseExpiresAt.Equal(leaseExpiresAt) {
		t.Fatalf("expected lease expiry %s, got %#v", leaseExpiresAt, service.renewAttemptLeaseReq.LeaseExpiresAt)
	}
}

func TestCompleteProjectTaskAttemptHandlerBuildsServiceRequest(t *testing.T) {
	tenantID := uuid.New()
	attemptID := uuid.New()
	taskID := uuid.New()
	nodeID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	body := strings.NewReader(`{
		"project_task_id":"` + taskID.String() + `",
		"runtime_node_id":"` + nodeID.String() + `",
		"lease_token":"lease-token-3",
		"idempotency_key":"attempt-complete-1",
		"conclusion":"done",
		"evidence_refs":["s3://bucket/report.md"],
		"artifact_refs":["artifact-runtime-log"],
		"confidence_factors":{"tests":"passed"},
		"uncertainty":"low",
		"missing_information":[],
		"recommended_next_action":"accept",
		"requires_human_review":true
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/complete", body)
	req = withProjectRouteParams(req, map[string]string{"attemptId": attemptID.String()})
	req = withRuntimeContext(req, tenantID, nodeID)
	resp := httptest.NewRecorder()

	handler.CompleteProjectTaskAttempt(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected complete writeback to return 202, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.completeAttemptReq.AttemptID != attemptID || service.completeAttemptReq.ProjectTaskID != taskID || service.completeAttemptReq.RuntimeNodeID != nodeID {
		t.Fatalf("unexpected complete request identity: %#v", service.completeAttemptReq)
	}
	if service.completeAttemptReq.Conclusion != "done" || !service.completeAttemptReq.RequiresHumanReview {
		t.Fatalf("unexpected complete request payload: %#v", service.completeAttemptReq)
	}
}

func TestCompleteProjectTaskAttemptResultRouteParsesTaskResultContract(t *testing.T) {
	tenantID := uuid.New()
	attemptID := uuid.MustParse("00000000-0000-0000-0000-000000000441")
	taskID := uuid.MustParse("00000000-0000-0000-0000-000000000442")
	nodeID := uuid.MustParse("00000000-0000-0000-0000-000000000443")
	service := &handlerTestService{}
	handler := NewHandler(service)
	body := strings.NewReader(`{
		"project_task_id":"00000000-0000-0000-0000-000000000442",
		"runtime_node_id":"00000000-0000-0000-0000-000000000443",
		"lease_token":"lease-token",
		"idempotency_key":"result-1",
		"result_contract":{
			"status":"completed",
			"summary":"完成分析",
			"acceptance_results":[{"criterion":"输出结论","status":"passed","evidence_refs":["artifact:report"]}],
			"evidence_refs":[{"type":"report","ref":"artifact:report"}],
			"artifact_refs":[],
			"verification":[{"type":"unit_test","status":"passed","summary":"测试通过"}],
			"risks":[]
		}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/result", body)
	req = withProjectRouteParams(req, map[string]string{"attemptId": attemptID.String()})
	req = withRuntimeContext(req, tenantID, nodeID)
	resp := httptest.NewRecorder()

	handler.SubmitProjectTaskAttemptResult(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected result writeback to return 202, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.submitProjectTaskAttemptResultReq.AttemptID != attemptID ||
		service.submitProjectTaskAttemptResultReq.ProjectTaskID != taskID ||
		service.submitProjectTaskAttemptResultReq.RuntimeNodeID != nodeID {
		t.Fatalf("unexpected result request identity: %#v", service.submitProjectTaskAttemptResultReq)
	}
	if service.submitProjectTaskAttemptResultReq.ResultContract.Status != TaskResultStatusCompleted {
		t.Fatalf("expected completed result status, got %#v", service.submitProjectTaskAttemptResultReq.ResultContract.Status)
	}
	if len(service.submitProjectTaskAttemptResultReq.ResultContract.AcceptanceResults) != 1 ||
		service.submitProjectTaskAttemptResultReq.ResultContract.AcceptanceResults[0].Criterion != "输出结论" ||
		service.submitProjectTaskAttemptResultReq.ResultContract.AcceptanceResults[0].Status != TaskResultCriterionStatusPassed {
		t.Fatalf("unexpected acceptance results: %#v", service.submitProjectTaskAttemptResultReq.ResultContract.AcceptanceResults)
	}
}

func TestFailProjectTaskAttemptHandlerBuildsServiceRequest(t *testing.T) {
	tenantID := uuid.New()
	attemptID := uuid.New()
	taskID := uuid.New()
	nodeID := uuid.New()
	retryable := true
	service := &handlerTestService{}
	handler := NewHandler(service)
	body := strings.NewReader(`{
		"project_task_id":"` + taskID.String() + `",
		"runtime_node_id":"` + nodeID.String() + `",
		"lease_token":"lease-token-4",
		"idempotency_key":"attempt-fail-1",
		"failure_summary":"provider crashed",
		"failure_family":"runtime_agent_failure",
		"retryable":true
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/fail", body)
	req = withProjectRouteParams(req, map[string]string{"attemptId": attemptID.String()})
	req = withRuntimeContext(req, tenantID, nodeID)
	resp := httptest.NewRecorder()

	handler.FailProjectTaskAttempt(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected fail writeback to return 202, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.failAttemptReq.AttemptID != attemptID || service.failAttemptReq.ProjectTaskID != taskID || service.failAttemptReq.RuntimeNodeID != nodeID {
		t.Fatalf("unexpected fail request identity: %#v", service.failAttemptReq)
	}
	if service.failAttemptReq.FailureSummary != "provider crashed" || service.failAttemptReq.FailureFamily != "runtime_agent_failure" {
		t.Fatalf("unexpected fail request payload: %#v", service.failAttemptReq)
	}
	if service.failAttemptReq.Retryable == nil || *service.failAttemptReq.Retryable != retryable {
		t.Fatalf("expected retryable true, got %#v", service.failAttemptReq.Retryable)
	}
}

func TestWaitHumanProjectTaskAttemptHandlerBuildsServiceRequest(t *testing.T) {
	tenantID := uuid.New()
	attemptID := uuid.New()
	taskID := uuid.New()
	nodeID := uuid.New()
	employeeID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	body := strings.NewReader(`{
		"project_task_id":"` + taskID.String() + `",
		"runtime_node_id":"` + nodeID.String() + `",
		"lease_token":"lease-token-5",
		"idempotency_key":"attempt-wait-human-1",
		"digital_employee_id":"` + employeeID.String() + `",
		"reason":"missing_context",
		"summary":"Need customer scope",
		"missing_context_refs":["customer_scope"],
		"suggested_resolution_options":["resume_same_task"]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/wait-human", body)
	req = withProjectRouteParams(req, map[string]string{"attemptId": attemptID.String()})
	req = withRuntimeContext(req, tenantID, nodeID)
	resp := httptest.NewRecorder()

	handler.WaitHumanProjectTaskAttempt(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected wait-human writeback to return 202, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.waitAttemptReq.AttemptID != attemptID || service.waitAttemptReq.ProjectTaskID != taskID || service.waitAttemptReq.RuntimeNodeID != nodeID {
		t.Fatalf("unexpected wait-human request identity: %#v", service.waitAttemptReq)
	}
	if service.waitAttemptReq.DigitalEmployeeID != employeeID || service.waitAttemptReq.Reason != HumanWaitReasonMissingContext || service.waitAttemptReq.Summary != "Need customer scope" {
		t.Fatalf("unexpected wait-human request payload: %#v", service.waitAttemptReq)
	}
	if len(service.waitAttemptReq.MissingContextRefs) != 1 || service.waitAttemptReq.MissingContextRefs[0] != "customer_scope" {
		t.Fatalf("unexpected missing context refs: %#v", service.waitAttemptReq.MissingContextRefs)
	}
	if len(service.waitAttemptReq.SuggestedResolutionOptions) != 1 || service.waitAttemptReq.SuggestedResolutionOptions[0] != HumanWaitResolutionResumeSameTask {
		t.Fatalf("unexpected suggested resolution options: %#v", service.waitAttemptReq.SuggestedResolutionOptions)
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
	createReq                         CreateProjectRequest
	createErr                         error
	submitDemandReq                   SubmitProjectDemandRequest
	submitDemandErr                   error
	workflowInstances                 []WorkflowInstanceSummary
	workflowInstancesReq              ListWorkflowInstancesRequest
	createEvidenceReq                 CreateEvidenceRefServiceRequest
	patchEvidenceReq                  PatchEvidenceRequest
	patchEvidenceErr                  error
	createAcceptanceReq               CreateAcceptanceServiceRequest
	createArchiveReq                  CreateArchiveSnapshotServiceRequest
	getAcceptanceErr                  error
	getConfigRevisionErr              error
	getOverviewCalls                  int
	routeDecisionTenantID             uuid.UUID
	routeDecisionProjectID            uuid.UUID
	routeDecisionLimit                int32
	routeDecisionOffset               int32
	planRevisions                     []PlanRevision
	planRevisionListReq               ListPlanRevisionsRequest
	planRevisionTenantID              uuid.UUID
	planRevisionProjectID             uuid.UUID
	planRevisionID                    uuid.UUID
	dispatchGates                     []PreDispatchGateResult
	dispatchGateListReq               ListPreDispatchGateResultsRequest
	resolveDecisionReq                ResolveDecisionRequest
	launchDetailTenantID              uuid.UUID
	launchDetailDemandID              uuid.UUID
	launchDetailProjectID             uuid.UUID
	taskGraph                         ProjectTaskGraph
	taskGraphReq                      GetProjectTaskGraphRequest
	taskGraphCalls                    int
	taskLiveness                      []ProjectTaskLiveness
	taskLivenessTenantID              uuid.UUID
	taskLivenessProjectID             uuid.UUID
	executionTrace                    *ProjectExecutionTrace
	executionTraceReq                 GetExecutionTraceRequest
	startAttemptReq                   StartProjectTaskAttemptRequest
	renewAttemptLeaseReq              RenewProjectTaskAttemptLeaseRequest
	completeAttemptReq                CompleteProjectTaskAttemptRequest
	submitProjectTaskAttemptResultReq SubmitProjectTaskAttemptResultRequest
	failAttemptReq                    FailProjectTaskAttemptRequest
	waitAttemptReq                    WaitHumanProjectTaskAttemptRequest
}

func (s *handlerTestService) CreateProject(ctx context.Context, req CreateProjectRequest) (*CreateProjectResult, error) {
	s.createReq = req
	if s.createErr != nil {
		return nil, s.createErr
	}
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

func (s *handlerTestService) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	s.workflowInstancesReq = req
	return s.workflowInstances, nil
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
	if req.ReviewerUserID != nil {
		demand.ReviewerPreference = &ReviewerPreference{
			ReviewerUserID:   *req.ReviewerUserID,
			SelectionReason:  req.ReviewerSelectionReason,
			ProjectRole:      ProjectRoleReviewer,
			ResolvedFromRule: false,
		}
	}
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

func (s *handlerTestService) ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error) {
	s.planRevisionListReq = req
	return s.planRevisions, nil
}

func (s *handlerTestService) GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*PlanRevision, error) {
	s.planRevisionTenantID = tenantID
	s.planRevisionProjectID = projectID
	s.planRevisionID = revisionID
	for _, revision := range s.planRevisions {
		if revision.ID == revisionID {
			return &revision, nil
		}
	}
	return nil, ErrProjectNotFound
}

func (s *handlerTestService) ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error) {
	s.dispatchGateListReq = req
	return s.dispatchGates, nil
}

func (s *handlerTestService) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error) {
	return nil, nil
}

func (s *handlerTestService) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error) {
	return nil, nil
}

func (s *handlerTestService) GetDemandLaunchDetail(ctx context.Context, tenantID, demandID uuid.UUID) (*DemandLaunchDetail, error) {
	s.launchDetailTenantID = tenantID
	s.launchDetailDemandID = demandID
	projectID := s.launchDetailProjectID
	if projectID == uuid.Nil {
		projectID = uuid.New()
	}
	project := testProject(tenantID, projectID, uuid.New())
	return &DemandLaunchDetail{
		Demand:       ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: uuid.New(), Title: "审查 PR", SourceType: DemandSourceManual, Status: ProjectDemandStatusPlanningPending},
		Project:      project,
		ProjectTasks: []ProjectTask{{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, DemandID: &demandID, Title: "审查 PR", Status: "pending"}},
	}, nil
}

func (s *handlerTestService) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (*ProjectTaskGraph, error) {
	s.taskGraphCalls++
	s.taskGraphReq = req
	return &s.taskGraph, nil
}

func (s *handlerTestService) ListProjectTaskLiveness(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskLiveness, error) {
	s.taskLivenessTenantID = tenantID
	s.taskLivenessProjectID = projectID
	return s.taskLiveness, nil
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

func (s *handlerTestService) GetExecutionTrace(ctx context.Context, req GetExecutionTraceRequest) (*ProjectExecutionTrace, error) {
	s.executionTraceReq = req
	if s.executionTrace != nil {
		return s.executionTrace, nil
	}
	return &ProjectExecutionTrace{ProjectID: req.ProjectID}, nil
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

func (s *handlerTestService) StartProjectTaskAttempt(ctx context.Context, req StartProjectTaskAttemptRequest) (*ProjectTaskAttempt, error) {
	s.startAttemptReq = req
	return &ProjectTaskAttempt{ID: req.AttemptID, TenantID: req.TenantID, ProjectTaskID: req.ProjectTaskID, Status: ProjectTaskAttemptStatusRunning}, nil
}

func (s *handlerTestService) RenewProjectTaskAttemptLease(ctx context.Context, req RenewProjectTaskAttemptLeaseRequest) error {
	s.renewAttemptLeaseReq = req
	return nil
}

func (s *handlerTestService) CompleteProjectTaskAttempt(ctx context.Context, req CompleteProjectTaskAttemptRequest) (*ExecutionSummary, error) {
	s.completeAttemptReq = req
	return &ExecutionSummary{ID: uuid.New(), TenantID: req.TenantID, ProjectTaskID: req.ProjectTaskID}, nil
}

func (s *handlerTestService) SubmitProjectTaskAttemptResult(ctx context.Context, req SubmitProjectTaskAttemptResultRequest) (*ExecutionSummary, error) {
	s.submitProjectTaskAttemptResultReq = req
	return &ExecutionSummary{ID: uuid.New(), TenantID: req.TenantID, ProjectTaskID: req.ProjectTaskID}, nil
}

func (s *handlerTestService) FailProjectTaskAttempt(ctx context.Context, req FailProjectTaskAttemptRequest) (*ProjectTask, error) {
	s.failAttemptReq = req
	return &ProjectTask{ID: req.ProjectTaskID, TenantID: req.TenantID, Status: ProjectTaskStatusFailed}, nil
}

func (s *handlerTestService) WaitHumanProjectTaskAttempt(ctx context.Context, req WaitHumanProjectTaskAttemptRequest) (*ProjectTask, error) {
	s.waitAttemptReq = req
	return &ProjectTask{ID: req.ProjectTaskID, TenantID: req.TenantID, Status: ProjectTaskStatusWaitingHuman}, nil
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
