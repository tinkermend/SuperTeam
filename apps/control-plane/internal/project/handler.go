package project

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	CreateProject(ctx context.Context, req CreateProjectRequest) (*CreateProjectResult, error)
	GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (*Project, error)
	AddProjectRuntimeNode(ctx context.Context, req ModifyProjectRuntimeNodeRequest) (*ProjectRuntimeNode, error)
	RemoveProjectRuntimeNode(ctx context.Context, req ModifyProjectRuntimeNodeRequest) error
	GetProjectRuntimeReadiness(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectRuntimePlacementReadiness, error)
	ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error)
	ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error)
	UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (*Project, error)
	ArchiveProject(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) (*Project, error)
	GetProjectDeletePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectDeletePreview, error)
	DeleteProject(ctx context.Context, req DeleteProjectRequest) error
	ReplaceProjectMembers(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error)
	ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error)
	ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error)
	ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error)
	RetryWorkflowSignal(ctx context.Context, req RetryWorkflowSignalRequest) (*ProjectEvent, error)
	SubmitDemand(ctx context.Context, req SubmitProjectDemandRequest) (*ProjectDemand, error)
	ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error)
	GetDemandLaunchDetail(ctx context.Context, tenantID, demandID uuid.UUID) (*DemandLaunchDetail, error)
	ListDemandAcceptanceCriteriaDetail(ctx context.Context, tenantID, demandID uuid.UUID) (*DemandAcceptanceCriteriaDetail, error)
	SignDemandCriterionVerdict(ctx context.Context, req SignDemandCriterionVerdictRequest) (*SignDemandCriterionVerdictResult, error)
	GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (*ProjectTaskGraph, error)
	ListProjectTaskLiveness(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskLiveness, error)
	GetOverview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectOverview, error)
	ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error)
	ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error)
	GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*PlanRevision, error)
	ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error)
	ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error)
	ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error)
	ResolveDecision(ctx context.Context, req ResolveDecisionRequest) (*DecisionRequest, error)
	ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error)
	GetExecutionTrace(ctx context.Context, req GetExecutionTraceRequest) (*ProjectExecutionTrace, error)
	ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error)
	StartProjectTaskAttempt(ctx context.Context, req StartProjectTaskAttemptRequest) (*ProjectTaskAttempt, error)
	RenewProjectTaskAttemptLease(ctx context.Context, req RenewProjectTaskAttemptLeaseRequest) error
	CompleteProjectTaskAttempt(ctx context.Context, req CompleteProjectTaskAttemptRequest) (*ExecutionSummary, error)
	FailProjectTaskAttempt(ctx context.Context, req FailProjectTaskAttemptRequest) (*ProjectTask, error)
	WaitHumanProjectTaskAttempt(ctx context.Context, req WaitHumanProjectTaskAttemptRequest) (*ProjectTask, error)
	CreateProjectTaskAttestation(ctx context.Context, req CreateProjectTaskAttestationRequest) (*ProjectTaskAttestation, error)
	RecordProjectTaskAttemptBudgetHeartbeat(ctx context.Context, req RecordProjectTaskAttemptBudgetHeartbeatRequest) (*ProjectTaskAttemptBudgetHeartbeatResult, error)
	ListEvidence(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error)
	CreateEvidence(ctx context.Context, req CreateEvidenceRefServiceRequest) (*ProjectEvidenceRef, error)
	PatchEvidence(ctx context.Context, req PatchEvidenceRequest) (*ProjectEvidenceRef, error)
	ListArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error)
	ListReports(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error)
	ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error)
	GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectBudgetSummary, error)
	CreateAcceptance(ctx context.Context, req CreateAcceptanceServiceRequest) (*ProjectAcceptanceRecord, error)
	GetAcceptance(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectAcceptanceRecord, error)
	GetArchivePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectArchivePreview, error)
	CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotServiceRequest) (*ProjectArchiveSnapshot, error)
	ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error)
	ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error)
	GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*ProjectConfigRevision, error)
	ListProjectRuntimeNodes(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectRuntimeNode, error)
}

type projectTaskAttemptResultSubmitter interface {
	SubmitProjectTaskAttemptResult(ctx context.Context, req SubmitProjectTaskAttemptResultRequest) (*ExecutionSummary, error)
}

type HTTPHandler struct {
	service    HandlerService
	authorizer authz.Authorizer
}

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *HTTPHandler) authorizeProjectAction(w http.ResponseWriter, r *http.Request, action string) (uuid.UUID, uuid.UUID, bool) {
	if h.authorizer == nil {
		http.Error(w, "project authorization is not configured", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   userID.String(),
		},
		Action:   action,
		Resource: authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()},
		TenantID: tenantID,
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return uuid.Nil, uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

func (h *HTTPHandler) authorizeProjectScopedAction(w http.ResponseWriter, r *http.Request, action string) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	if h.authorizer == nil {
		http.Error(w, "project authorization is not configured", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	projectID, ok := projectIDFromRequest(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   userID.String(),
		},
		Action:   action,
		Resource: authz.ResourceRef{Type: authz.ResourceProject, ID: projectID.String()},
		TenantID: tenantID,
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, projectID, true
}

func (h *HTTPHandler) projectRouteContext(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, HandlerService, bool) {
	tenantID, actorID, projectID, ok := h.authorizeProjectScopedAction(w, r, authz.ActionProjectRead)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	return tenantID, actorID, projectID, service, true
}

func (h *HTTPHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeProjectAction(w, r, authz.ActionProjectRead)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	var status *ProjectStatus
	if raw := r.URL.Query().Get("status"); raw != "" {
		parsed := ProjectStatus(raw)
		status = &parsed
	}
	projects, err := service.ListProjects(r.Context(), ListProjectsRequest{
		TenantID: tenantID,
		Status:   status,
		Query:    r.URL.Query().Get("q"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectResponses(projects))
}

func (h *HTTPHandler) ListWorkflowInstances(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := h.authorizeProjectAction(w, r, authz.ActionProjectRead)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	req := ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Query:       r.URL.Query().Get("q"),
		Limit:       limit,
		Offset:      offset,
	}
	if raw := r.URL.Query().Get("project_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid project_id", http.StatusBadRequest)
			return
		}
		req.ProjectID = &id
	}
	if raw := r.URL.Query().Get("status"); raw != "" {
		status := WorkflowInstanceStatus(raw)
		req.Status = &status
	}
	if raw := r.URL.Query().Get("scope"); raw != "" {
		switch raw {
		case "active", "archived", "all":
			req.Scope = raw
		default:
			http.Error(w, "invalid scope", http.StatusBadRequest)
			return
		}
	}
	items, err := service.ListWorkflowInstances(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workflowInstanceResponses(items))
}

func (h *HTTPHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := h.authorizeProjectAction(w, r, authz.ActionProjectCreate)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req createProjectBody
	if !decodeJSONBody(w, r, &req) {
		return
	}
	created, err := service.CreateProject(r.Context(), CreateProjectRequest{
		TenantID:            tenantID,
		TeamID:              req.TeamID,
		ActorUserID:         actorID,
		Name:                req.Name,
		Description:         req.Description,
		Goal:                req.Goal,
		HumanOwnerUserID:    req.HumanOwnerUserID,
		Members:             req.Members,
		CoordinationPolicy:  req.CoordinationPolicy,
		ApprovalPolicy:      req.ApprovalPolicy,
		EvidencePolicy:      req.EvidencePolicy,
		RepoBinding:         req.RepoBinding,
		RuntimeNodeIDs:      req.RuntimeNodeIDs,
		ScenarioTemplateKey: req.ScenarioTemplateKey,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, createProjectResponseFromDomain(created))
}

func (h *HTTPHandler) ListProjectRuntimeNodes(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	nodes, err := service.ListProjectRuntimeNodes(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectRuntimeNodeResponses(nodes))
}

func (h *HTTPHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, ok := h.authorizeProjectScopedAction(w, r, authz.ActionProjectRead)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	project, err := service.GetProject(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	response := projectResponseFromDomain(*project)
	response.AllowedActions = h.allowedProjectActions(r.Context(), tenantID, projectID)
	writeJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) AddProjectRuntimeNode(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, ok := h.authorizeProjectScopedAction(w, r, authz.ActionProjectUpdate)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	runtimeNodeID, err := uuid.Parse(chi.URLParam(r, "runtimeNodeId"))
	if err != nil {
		http.Error(w, "invalid runtime node id", http.StatusBadRequest)
		return
	}
	var body modifyProjectRuntimeNodeBody
	if r.Body != nil && r.Body != http.NoBody {
		if !decodeOptionalJSONBody(w, r, &body) {
			return
		}
	}
	node, err := service.AddProjectRuntimeNode(r.Context(), ModifyProjectRuntimeNodeRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		RuntimeNodeID: runtimeNodeID,
		ActorUserID:   actorID,
		Reason:        body.Reason,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectRuntimeNodeResponse{RuntimeNodeID: node.RuntimeNodeID})
}

func (h *HTTPHandler) RemoveProjectRuntimeNode(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, ok := h.authorizeProjectScopedAction(w, r, authz.ActionProjectUpdate)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	runtimeNodeID, err := uuid.Parse(chi.URLParam(r, "runtimeNodeId"))
	if err != nil {
		http.Error(w, "invalid runtime node id", http.StatusBadRequest)
		return
	}
	var body modifyProjectRuntimeNodeBody
	if r.Body != nil && r.Body != http.NoBody {
		if !decodeOptionalJSONBody(w, r, &body) {
			return
		}
	}
	if err := service.RemoveProjectRuntimeNode(r.Context(), ModifyProjectRuntimeNodeRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		RuntimeNodeID: runtimeNodeID,
		ActorUserID:   actorID,
		Reason:        body.Reason,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) GetProjectRuntimeReadiness(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	readiness, err := service.GetProjectRuntimeReadiness(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, readiness)
}

func (h *HTTPHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	h.updateProjectConfig(w, r)
}

func (h *HTTPHandler) ArchiveProject(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	project, err := service.ArchiveProject(r.Context(), tenantID, projectID, actorID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectResponseFromDomain(*project))
}

func (h *HTTPHandler) GetProjectDeletePreview(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, ok := h.authorizeProjectScopedAction(w, r, authz.ActionProjectDelete)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	preview, err := service.GetProjectDeletePreview(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectDeletePreviewResponseFromDomain(*preview))
}

func (h *HTTPHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, ok := h.authorizeProjectScopedAction(w, r, authz.ActionProjectDelete)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.DeleteProject(r.Context(), DeleteProjectRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: actorID,
	}); err != nil {
		writeDeleteProjectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) allowedProjectActions(ctx context.Context, tenantID, projectID uuid.UUID) []string {
	if h == nil || h.authorizer == nil {
		return nil
	}
	userID := middleware.GetUserID(ctx)
	if userID == uuid.Nil {
		return nil
	}
	actions := []string{authz.ActionProjectArchive, authz.ActionProjectDelete}
	allowed := make([]string, 0, len(actions))
	for _, action := range actions {
		decision, err := h.authorizer.Check(ctx, authz.CheckRequest{
			Actor:    authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
			Action:   action,
			Resource: authz.ResourceRef{Type: authz.ResourceProject, ID: projectID.String()},
			TenantID: tenantID,
		})
		if err == nil && decision.Allowed {
			allowed = append(allowed, action)
		}
	}
	return allowed
}

func (h *HTTPHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	overview, err := service.GetOverview(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overviewResponseFromDomain(overview))
}

func (h *HTTPHandler) ListProjectMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	members, err := service.ListProjectMembers(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, memberResponses(members))
}

func (h *HTTPHandler) ReplaceProjectMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	var req struct {
		Members []ProjectMemberInput `json:"members"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	members, err := service.ReplaceProjectMembers(r.Context(), tenantID, projectID, actorID, req.Members)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, memberResponses(members))
}

func (h *HTTPHandler) ListProjectTasks(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	var status *string
	if raw := r.URL.Query().Get("status"); raw != "" {
		status = &raw
	}
	tasks, err := service.ListProjectTasks(r.Context(), tenantID, projectID, status, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskResponses(tasks))
}

func (h *HTTPHandler) GetProjectTaskGraph(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	req := GetProjectTaskGraphRequest{TenantID: tenantID, ProjectID: projectID}
	if raw := r.URL.Query().Get("coordination_job_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid coordination_job_id", http.StatusBadRequest)
			return
		}
		req.CoordinationJobID = &id
	}
	if raw := r.URL.Query().Get("demand_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid demand_id", http.StatusBadRequest)
			return
		}
		req.DemandID = &id
	}
	if req.CoordinationJobID == nil && req.DemandID == nil {
		writeHandlerError(w, ErrInvalidProject)
		return
	}
	graph, err := service.GetProjectTaskGraph(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskGraphResponseFromDomain(*graph))
}

func (h *HTTPHandler) GetProjectTaskLiveness(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	items, err := service.ListProjectTaskLiveness(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	for _, item := range items {
		if item.ProjectTaskID == taskID {
			writeJSON(w, http.StatusOK, taskLivenessResponseFromDomain(item))
			return
		}
	}
	writeHandlerError(w, ErrProjectNotFound)
}

func (h *HTTPHandler) ListProjectEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	events, err := service.ListProjectEvents(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, eventResponses(events))
}

func (h *HTTPHandler) RetryWorkflowSignal(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	eventID, ok := projectEventIDFromRequest(w, r)
	if !ok {
		return
	}
	event, err := service.RetryWorkflowSignal(r.Context(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   eventID,
		ActorID:   actorID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, eventResponseFromDomain(*event))
}

func (h *HTTPHandler) GetProjectConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	overview, err := service.GetOverview(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectConfigResponseFromDomain(overview))
}

func (h *HTTPHandler) UpdateProjectConfig(w http.ResponseWriter, r *http.Request) {
	h.updateProjectConfig(w, r)
}

func (h *HTTPHandler) SubmitDemand(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	var req submitDemandBody
	if !decodeJSONBody(w, r, &req) {
		return
	}
	demand, err := service.SubmitDemand(r.Context(), SubmitProjectDemandRequest{
		TenantID:                tenantID,
		ProjectID:               projectID,
		SubmittedByUserID:       actorID,
		Title:                   req.Title,
		Content:                 req.Content,
		SourceType:              req.SourceType,
		SourceRefs:              req.SourceRefs,
		Attachments:             req.Attachments,
		ReviewerUserID:          req.ReviewerUserID,
		ReviewerSelectionReason: req.ReviewerSelectionReason,
		CoordinationMode:        req.CoordinationMode,
		ScenarioTemplateKey:     req.ScenarioTemplateKey,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, demandResponseFromDomain(*demand))
}

func (h *HTTPHandler) ListProjectDemands(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	demands, err := service.ListProjectDemands(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, demandResponses(demands))
}

func (h *HTTPHandler) GetDemandLaunchDetail(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeProjectAction(w, r, authz.ActionProjectDemandRead)
	if !ok {
		return
	}
	demandID, err := uuid.Parse(chi.URLParam(r, "demandId"))
	if err != nil {
		writeHandlerError(w, ErrInvalidProject)
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	detail, err := service.GetDemandLaunchDetail(r.Context(), tenantID, demandID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, demandLaunchDetailResponseFromDomain(*detail))
}

func (h *HTTPHandler) ListDemandAcceptanceCriteria(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeProjectAction(w, r, authz.ActionProjectDemandRead)
	if !ok {
		return
	}
	demandID, err := uuid.Parse(chi.URLParam(r, "demandId"))
	if err != nil {
		writeHandlerError(w, ErrInvalidProject)
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	detail, err := service.ListDemandAcceptanceCriteriaDetail(r.Context(), tenantID, demandID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, demandAcceptanceCriteriaResponseFromDomain(*detail))
}

func (h *HTTPHandler) SignDemandCriterionVerdict(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := h.authorizeProjectAction(w, r, authz.ActionProjectDecisionResolve)
	if !ok {
		return
	}
	demandID, err := uuid.Parse(chi.URLParam(r, "demandId"))
	if err != nil {
		writeHandlerError(w, ErrInvalidProject)
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var body signDemandCriterionVerdictBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	result, err := service.SignDemandCriterionVerdict(r.Context(), SignDemandCriterionVerdictRequest{
		TenantID:    tenantID,
		DemandID:    demandID,
		ActorUserID: actorID,
		CriterionID: body.CriterionID,
		Verdict:     body.Verdict,
		Reason:      body.Reason,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, signDemandCriterionVerdictResponseFromDomain(*result))
}

func (h *HTTPHandler) ListRouteDecisions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	decisions, err := service.ListRouteDecisions(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, routeDecisionResponses(decisions))
}

func (h *HTTPHandler) ListPlanRevisions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	req := ListPlanRevisionsRequest{TenantID: tenantID, ProjectID: projectID, Limit: limit, Offset: offset}
	if raw := r.URL.Query().Get("demand_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			http.Error(w, "invalid demand_id", http.StatusBadRequest)
			return
		}
		req.DemandID = &id
	}
	revisions, err := service.ListPlanRevisions(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, planRevisionResponses(revisions))
}

func (h *HTTPHandler) GetPlanRevision(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	revisionID, ok := planRevisionIDFromRequest(w, r)
	if !ok {
		return
	}
	revision, err := service.GetPlanRevision(r.Context(), tenantID, projectID, revisionID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, planRevisionResponseFromDomain(*revision))
}

func (h *HTTPHandler) ListProjectTaskDispatchGates(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	results, err := service.ListPreDispatchGateResults(r.Context(), ListPreDispatchGateResultsRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskID,
		Limit:         50,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dispatchGateResponses(results)})
}

func (h *HTTPHandler) ListCoordinationJobs(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	jobs, err := service.ListCoordinationJobs(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, coordinationJobResponses(jobs))
}

func (h *HTTPHandler) ListDecisionRequests(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	decisions, err := service.ListDecisionRequests(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decisionRequestResponses(decisions))
}

func (h *HTTPHandler) ResolveDecision(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	decisionID, ok := decisionIDFromRequest(w, r)
	if !ok {
		return
	}
	var body resolveDecisionBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	decision, err := service.ResolveDecision(r.Context(), ResolveDecisionRequest{
		TenantID:              tenantID,
		ProjectID:             projectID,
		DecisionRequestID:     decisionID,
		DecidedByUserID:       actorID,
		Decision:              body.Decision,
		Comment:               body.Comment,
		Payload:               body.Payload,
		TargetExitDeliverable: body.TargetExitDeliverable,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decisionRequestResponseFromDomain(*decision))
}

func (h *HTTPHandler) ListExecutionSummaries(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	summaries, err := service.ListExecutionSummaries(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, executionSummaryResponses(summaries))
}

func (h *HTTPHandler) GetExecutionTrace(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	req := GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	}
	query := r.URL.Query()
	if raw := query.Get("project_task_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid project_task_id", http.StatusBadRequest)
			return
		}
		req.ProjectTaskID = &id
	}
	if raw := query.Get("attempt_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid attempt_id", http.StatusBadRequest)
			return
		}
		req.ProjectTaskAttemptID = &id
	}
	if value := strings.TrimSpace(query.Get("event_type")); value != "" {
		req.EventType = &value
	}
	if value := strings.TrimSpace(query.Get("error_family")); value != "" {
		req.ErrorFamily = &value
	}
	trace, err := service.GetExecutionTrace(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	if trace == nil {
		trace = &ProjectExecutionTrace{ProjectID: projectID}
	}
	writeJSON(w, http.StatusOK, executionTraceResponseFromDomain(*trace))
}

func (h *HTTPHandler) ListTransferRequests(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	transfers, err := service.ListTransferRequests(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, transferRequestResponses(transfers))
}

func (h *HTTPHandler) ListEvidence(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	var status *EvidenceVerificationStatus
	if raw := r.URL.Query().Get("status"); raw != "" {
		parsed := EvidenceVerificationStatus(raw)
		if !validEvidenceVerificationStatus(parsed) {
			http.Error(w, "invalid evidence status", http.StatusBadRequest)
			return
		}
		status = &parsed
	}
	evidence, err := service.ListEvidence(r.Context(), tenantID, projectID, status, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, evidenceResponses(evidence))
}

func (h *HTTPHandler) CreateEvidence(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	var body createEvidenceBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	evidence, err := service.CreateEvidence(r.Context(), CreateEvidenceRefServiceRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ActorType:          "human_user",
		ActorID:            actorID,
		ProjectTaskID:      body.ProjectTaskID,
		RouteDecisionID:    body.RouteDecisionID,
		ExecutionSummaryID: body.ExecutionSummaryID,
		EvidenceType:       body.EvidenceType,
		Title:              body.Title,
		Summary:            body.Summary,
		SourceType:         body.SourceType,
		SourceRef:          body.SourceRef,
		ArtifactRefID:      body.ArtifactRefID,
		SubmittedByType:    "human_user",
		SubmittedByID:      &actorID,
		Metadata:           body.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, evidenceResponseFromDomain(*evidence))
}

func (h *HTTPHandler) PatchEvidence(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	evidenceID, ok := evidenceIDFromRequest(w, r)
	if !ok {
		return
	}
	var body patchEvidenceBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	var metadata map[string]any
	if body.Metadata != nil {
		metadata = *body.Metadata
	}
	evidence, err := service.PatchEvidence(r.Context(), PatchEvidenceRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		EvidenceID:         evidenceID,
		ActorUserID:        actorID,
		VerificationStatus: body.VerificationStatus,
		Metadata:           metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, evidenceResponseFromDomain(*evidence))
}

func (h *HTTPHandler) ListArtifacts(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	artifacts, err := service.ListArtifacts(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, artifactResponses(artifacts))
}

func (h *HTTPHandler) ListReports(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	reports, err := service.ListReports(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reportResponses(reports))
}

func (h *HTTPHandler) ListBudgetLedger(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	ledger, err := service.ListBudgetLedger(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, budgetLedgerResponses(ledger))
}

func (h *HTTPHandler) GetBudgetSummary(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	summary, err := service.GetBudgetSummary(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, budgetSummaryResponseFromDomain(*summary))
}

func (h *HTTPHandler) CreateAcceptance(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	var body createAcceptanceBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	acceptance, err := service.CreateAcceptance(r.Context(), CreateAcceptanceServiceRequest{
		TenantID:         tenantID,
		ProjectID:        projectID,
		AcceptedByUserID: actorID,
		Status:           body.Status,
		Conclusion:       body.Conclusion,
		Summary:          body.Summary,
		EvidenceRefIDs:   body.EvidenceRefIDs,
		ReportRefIDs:     body.ReportRefIDs,
		UnresolvedRisks:  body.UnresolvedRisks,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, acceptanceResponseFromDomain(*acceptance))
}

func (h *HTTPHandler) GetAcceptance(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	acceptance, err := service.GetAcceptance(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	if acceptance == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, acceptanceResponseFromDomain(*acceptance))
}

func (h *HTTPHandler) GetArchivePreview(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	preview, err := service.GetArchivePreview(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, archivePreviewResponseFromDomain(*preview))
}

func (h *HTTPHandler) CreateArchiveSnapshot(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	var body createArchiveSnapshotBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	snapshot, err := service.CreateArchiveSnapshot(r.Context(), CreateArchiveSnapshotServiceRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		CreatedByUserID: actorID,
		SnapshotType:    body.SnapshotType,
		Summary:         body.Summary,
		ObjectRef:       body.ObjectRef,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, archiveSnapshotResponseFromDomain(*snapshot))
}

func (h *HTTPHandler) ListArchiveSnapshots(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	snapshots, err := service.ListArchiveSnapshots(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, archiveSnapshotResponses(snapshots))
}

func (h *HTTPHandler) ListConfigRevisions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	revisions, err := service.ListConfigRevisions(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configRevisionResponses(revisions))
}

func (h *HTTPHandler) GetConfigRevision(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	revisionID, ok := revisionIDFromRequest(w, r)
	if !ok {
		return
	}
	revision, err := service.GetConfigRevision(r.Context(), tenantID, projectID, revisionID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configRevisionResponseFromDomain(*revision))
}

func (h *HTTPHandler) StartProjectTaskAttempt(w http.ResponseWriter, r *http.Request) {
	tenantID, runtimeNodeID, attemptID, service, ok := h.runtimeProjectTaskAttemptContext(w, r)
	if !ok {
		return
	}
	var body startProjectTaskAttemptBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	runtimeReq, ok := projectTaskAttemptRuntimeRequestFromBody(w, tenantID, runtimeNodeID, attemptID, body.ProjectTaskAttemptRuntimeBody)
	if !ok {
		return
	}
	if _, err := service.StartProjectTaskAttempt(r.Context(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: runtimeReq,
		CommandID:                        strings.TrimSpace(body.CommandID),
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *HTTPHandler) RenewProjectTaskAttemptLease(w http.ResponseWriter, r *http.Request) {
	tenantID, runtimeNodeID, attemptID, service, ok := h.runtimeProjectTaskAttemptContext(w, r)
	if !ok {
		return
	}
	var body renewProjectTaskAttemptLeaseBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	runtimeReq, ok := projectTaskAttemptRuntimeRequestFromBody(w, tenantID, runtimeNodeID, attemptID, body.ProjectTaskAttemptRuntimeBody)
	if !ok {
		return
	}
	if err := service.RenewProjectTaskAttemptLease(r.Context(), RenewProjectTaskAttemptLeaseRequest{
		ProjectTaskAttemptRuntimeRequest: runtimeReq,
		LeaseExpiresAt:                   body.LeaseExpiresAt,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) CompleteProjectTaskAttempt(w http.ResponseWriter, r *http.Request) {
	tenantID, runtimeNodeID, attemptID, service, ok := h.runtimeProjectTaskAttemptContext(w, r)
	if !ok {
		return
	}
	var body completeProjectTaskAttemptBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	runtimeReq, ok := projectTaskAttemptRuntimeRequestFromBody(w, tenantID, runtimeNodeID, attemptID, body.ProjectTaskAttemptRuntimeBody)
	if !ok {
		return
	}
	if _, err := service.CompleteProjectTaskAttempt(r.Context(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: runtimeReq,
		Conclusion:                       body.Conclusion,
		EvidenceRefs:                     body.EvidenceRefs,
		ArtifactRefs:                     body.ArtifactRefs,
		ConfidenceFactors:                body.ConfidenceFactors,
		Uncertainty:                      body.Uncertainty,
		MissingInformation:               body.MissingInformation,
		RecommendedNextAction:            body.RecommendedNextAction,
		RequiresHumanReview:              body.RequiresHumanReview,
		ResultContract:                   body.ResultContract,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *HTTPHandler) SubmitProjectTaskAttemptResult(w http.ResponseWriter, r *http.Request) {
	tenantID, runtimeNodeID, attemptID, service, ok := h.runtimeProjectTaskAttemptContext(w, r)
	if !ok {
		return
	}
	submitter, ok := service.(projectTaskAttemptResultSubmitter)
	if !ok {
		http.Error(w, "project task result service is not configured", http.StatusServiceUnavailable)
		return
	}
	var body submitProjectTaskAttemptResultBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	runtimeReq, ok := projectTaskAttemptRuntimeRequestFromBody(w, tenantID, runtimeNodeID, attemptID, body.ProjectTaskAttemptRuntimeBody)
	if !ok {
		return
	}
	if _, err := submitter.SubmitProjectTaskAttemptResult(r.Context(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: runtimeReq,
		ResultContract:                   body.ResultContract,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *HTTPHandler) RecordProjectTaskAttemptBudgetHeartbeat(w http.ResponseWriter, r *http.Request) {
	tenantID, _, attemptID, service, ok := h.runtimeProjectTaskAttemptContext(w, r)
	if !ok {
		return
	}
	var body recordProjectTaskAttemptBudgetHeartbeatBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	result, err := service.RecordProjectTaskAttemptBudgetHeartbeat(r.Context(), RecordProjectTaskAttemptBudgetHeartbeatRequest{
		TenantID:             tenantID,
		ProjectID:            body.ProjectID,
		ProjectTaskID:        body.ProjectTaskID,
		AttemptID:            attemptID,
		ConsumedWallClockSec: body.ConsumedWallClockSec,
		ConsumedTokens:       body.ConsumedTokens,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectTaskAttemptBudgetHeartbeatResponse{
		Tripped:    result.Tripped,
		TripReason: nonEmptyStringPtr(result.TripReason),
	})
}

func (h *HTTPHandler) FailProjectTaskAttempt(w http.ResponseWriter, r *http.Request) {
	tenantID, runtimeNodeID, attemptID, service, ok := h.runtimeProjectTaskAttemptContext(w, r)
	if !ok {
		return
	}
	var body failProjectTaskAttemptBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	runtimeReq, ok := projectTaskAttemptRuntimeRequestFromBody(w, tenantID, runtimeNodeID, attemptID, body.ProjectTaskAttemptRuntimeBody)
	if !ok {
		return
	}
	if _, err := service.FailProjectTaskAttempt(r.Context(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: runtimeReq,
		FailureSummary:                   body.FailureSummary,
		FailureFamily:                    body.FailureFamily,
		Retryable:                        body.Retryable,
		ResultContract:                   body.ResultContract,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *HTTPHandler) WaitHumanProjectTaskAttempt(w http.ResponseWriter, r *http.Request) {
	tenantID, runtimeNodeID, attemptID, service, ok := h.runtimeProjectTaskAttemptContext(w, r)
	if !ok {
		return
	}
	var body waitHumanProjectTaskAttemptBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	runtimeReq, ok := projectTaskAttemptRuntimeRequestFromBody(w, tenantID, runtimeNodeID, attemptID, body.ProjectTaskAttemptRuntimeBody)
	if !ok {
		return
	}
	if _, err := service.WaitHumanProjectTaskAttempt(r.Context(), WaitHumanProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: runtimeReq,
		DigitalEmployeeID:                body.DigitalEmployeeID,
		Reason:                           body.Reason,
		Summary:                          body.Summary,
		MissingContextRefs:               body.MissingContextRefs,
		SuggestedResolutionOptions:       body.SuggestedResolutionOptions,
		ResultContract:                   body.ResultContract,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *HTTPHandler) CreateProjectTaskAttestation(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "tenant_id not found in context", http.StatusUnauthorized)
		return
	}
	runtimeNodeID := middleware.GetRuntimeNodeID(r.Context())
	if runtimeNodeID == uuid.Nil {
		http.Error(w, "runtime_node_id not found in context", http.StatusUnauthorized)
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var body createProjectTaskAttestationBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.RuntimeNodeID == uuid.Nil {
		http.Error(w, "runtime_node_id is required", http.StatusBadRequest)
		return
	}
	if body.RuntimeNodeID != runtimeNodeID {
		http.Error(w, "runtime_node_id does not match authenticated runtime node", http.StatusForbidden)
		return
	}
	providerAuthMode := body.ProviderAuthMode
	if providerAuthMode == "" {
		providerAuthMode = ProjectTaskAttestationProviderAuthModeHost
	}
	attestation, err := service.CreateProjectTaskAttestation(r.Context(), CreateProjectTaskAttestationRequest{
		TenantID:                  tenantID,
		ProjectID:                 body.ProjectID,
		ProjectTaskID:             body.ProjectTaskID,
		AttemptID:                 body.AttemptID,
		RuntimeNodeID:             runtimeNodeID,
		DigitalEmployeeID:         body.DigitalEmployeeID,
		CapabilityManifestVersion: body.CapabilityManifestVersion,
		ProviderAuthMode:          providerAuthMode,
		ProviderSessionID:         body.ProviderSessionID,
		AttestationType:           body.AttestationType,
		Status:                    body.Status,
		CommandArgv:               body.CommandArgv,
		ExitCode:                  body.ExitCode,
		DurationMs:                body.DurationMs,
		LogRef:                    body.LogRef,
		StdoutSha256:              body.StdoutSha256,
		StderrSha256:              body.StderrSha256,
		ArtifactRefs:              body.ArtifactRefs,
		ArtifactHashes:            body.ArtifactHashes,
		GitBranch:                 body.GitBranch,
		GitBaseRef:                body.GitBaseRef,
		GitHeadSha:                body.GitHeadSha,
		GitDiffSha256:             body.GitDiffSha256,
		Metadata:                  body.Metadata,
		IdempotencyKey:            body.IdempotencyKey,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, projectTaskAttestationResponseFromDomain(*attestation))
}

func (h *HTTPHandler) updateProjectConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	var req updateProjectBody
	if !decodeJSONBody(w, r, &req) {
		return
	}
	updated, err := service.UpdateProjectConfig(r.Context(), UpdateProjectConfigRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ActorUserID:        actorID,
		Name:               req.Name,
		Description:        req.Description,
		Goal:               req.Goal,
		HumanOwnerUserID:   req.HumanOwnerUserID,
		Members:            req.Members,
		CoordinationPolicy: req.CoordinationPolicy,
		ApprovalPolicy:     req.ApprovalPolicy,
		EvidencePolicy:     req.EvidencePolicy,
		RepoBinding:        req.RepoBinding,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectResponseFromDomain(*updated))
}

func (h *HTTPHandler) runtimeProjectTaskAttemptContext(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, HandlerService, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "tenant_id not found in context", http.StatusUnauthorized)
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	runtimeNodeID := middleware.GetRuntimeNodeID(r.Context())
	if runtimeNodeID == uuid.Nil {
		http.Error(w, "runtime_node_id not found in context", http.StatusUnauthorized)
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	attemptID, ok := attemptIDFromRequest(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	return tenantID, runtimeNodeID, attemptID, service, true
}

func projectTaskAttemptRuntimeRequestFromBody(w http.ResponseWriter, tenantID, runtimeNodeID, attemptID uuid.UUID, body ProjectTaskAttemptRuntimeBody) (ProjectTaskAttemptRuntimeRequest, bool) {
	if body.RuntimeNodeID == uuid.Nil {
		http.Error(w, "runtime_node_id is required", http.StatusBadRequest)
		return ProjectTaskAttemptRuntimeRequest{}, false
	}
	if body.RuntimeNodeID != runtimeNodeID {
		http.Error(w, "runtime_node_id does not match authenticated runtime node", http.StatusForbidden)
		return ProjectTaskAttemptRuntimeRequest{}, false
	}
	return ProjectTaskAttemptRuntimeRequest{
		TenantID:          tenantID,
		AttemptID:         attemptID,
		ProjectTaskID:     body.ProjectTaskID,
		RuntimeNodeID:     runtimeNodeID,
		LeaseToken:        body.LeaseToken,
		IdempotencyKey:    body.IdempotencyKey,
		ProviderSessionID: body.ProviderSessionID,
		RawLog:            body.RawLog.toRawLog(),
	}, true
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "project service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func projectIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectId"))
	if err != nil || projectID == uuid.Nil {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return projectID, true
}

func decisionIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	decisionID, err := uuid.Parse(chi.URLParam(r, "decisionId"))
	if err != nil || decisionID == uuid.Nil {
		http.Error(w, "invalid decision id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return decisionID, true
}

func projectEventIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	eventID, err := uuid.Parse(chi.URLParam(r, "eventId"))
	if err != nil || eventID == uuid.Nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return eventID, true
}

func attemptIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	attemptID, err := uuid.Parse(chi.URLParam(r, "attemptId"))
	if err != nil || attemptID == uuid.Nil {
		http.Error(w, "invalid project task attempt id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return attemptID, true
}

func taskIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskId"))
	if err != nil || taskID == uuid.Nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return taskID, true
}

func evidenceIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	evidenceID, err := uuid.Parse(chi.URLParam(r, "evidenceId"))
	if err != nil || evidenceID == uuid.Nil {
		http.Error(w, "invalid evidence id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return evidenceID, true
}

func revisionIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	revisionID, err := uuid.Parse(chi.URLParam(r, "revisionId"))
	if err != nil || revisionID == uuid.Nil {
		http.Error(w, "invalid config revision id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return revisionID, true
}

func planRevisionIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	revisionID, err := uuid.Parse(chi.URLParam(r, "planRevisionId"))
	if err != nil || revisionID == uuid.Nil {
		http.Error(w, "invalid plan revision id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return revisionID, true
}

func paginationFromRequest(w http.ResponseWriter, r *http.Request) (int32, int32, bool) {
	limit, ok := int32QueryParam(w, r, "limit")
	if !ok {
		return 0, 0, false
	}
	offset, ok := int32QueryParam(w, r, "offset")
	if !ok {
		return 0, 0, false
	}
	return limit, offset, true
}

func int32QueryParam(w http.ResponseWriter, r *http.Request, name string) (int32, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		http.Error(w, name+" must be an integer", http.StatusBadRequest)
		return 0, false
	}
	return int32(value), true
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func decodeOptionalJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTeamlessProjectMember):
		http.Error(w, "数字员工必须先归属团队才能加入项目："+strings.TrimSpace(strings.TrimPrefix(err.Error(), ErrTeamlessProjectMember.Error()+":")), http.StatusBadRequest)
	case errors.Is(err, ErrInvalidProject), errors.Is(err, ErrInvalidProjectMember), errors.Is(err, ErrInvalidProjectEvidence), errors.Is(err, ErrInvalidProjectAcceptance), errors.Is(err, ErrProjectRuntimeNodesRequired), errors.Is(err, ErrInvalidCoordinationMode):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrProjectNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrUnauthorizedProjectTeamScope):
		http.Error(w, "当前用户无权使用该团队创建项目。", http.StatusForbidden)
	case errors.Is(err, ErrProjectTaskForbidden), errors.Is(err, ErrProjectDecisionForbidden):
		http.Error(w, "project task forbidden", http.StatusForbidden)
	case errors.Is(err, ErrProjectArchived), errors.Is(err, ErrProjectArchiveBlocked), errors.Is(err, ErrProjectConflict):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		// 500 兜底不能吞错误细节——留一条服务端日志供排障（响应体仍不泄露内部信息）。
		log.Printf("project handler internal error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeDeleteProjectError(w http.ResponseWriter, err error) {
	var blocked *ProjectDeleteBlockedError
	if errors.As(err, &blocked) {
		writeJSON(w, http.StatusConflict, projectDeleteBlockedResponse{
			Code:     ProjectDeleteBlockedCode,
			Message:  "该项目仍有排队或执行中的任务，停止或完成后再删除。",
			Blockers: blocked.Blockers,
		})
		return
	}
	switch {
	case errors.Is(err, ErrInvalidProject):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrProjectNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"code":    "project_delete_terminate_failed",
			"message": "协调流程终止失败，请稍后重试。",
		})
	}
}

func projectDeletePreviewResponseFromDomain(preview ProjectDeletePreview) projectDeletePreviewResponse {
	return projectDeletePreviewResponse{
		ProjectID:   preview.ProjectID.String(),
		ProjectName: preview.ProjectName,
		CanDelete:   preview.CanDelete,
		Blockers:    append([]ProjectDeleteBlocker(nil), preview.Blockers...),
		Warnings:    preview.Warnings,
		Message:     preview.Message,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type createProjectBody struct {
	TenantID            uuid.UUID                `json:"tenant_id,omitempty"`
	ActorUserID         uuid.UUID                `json:"actor_user_id,omitempty"`
	TeamID              *uuid.UUID               `json:"team_id"`
	Name                string                   `json:"name"`
	Description         string                   `json:"description"`
	Goal                string                   `json:"goal"`
	HumanOwnerUserID    uuid.UUID                `json:"human_owner_user_id"`
	Members             []ProjectMemberInput     `json:"members"`
	CoordinationPolicy  map[string]any           `json:"coordination_policy"`
	ApprovalPolicy      map[string]any           `json:"approval_policy"`
	EvidencePolicy      map[string]any           `json:"evidence_policy"`
	RepoBinding         *ProjectRepoBindingInput `json:"repo_binding"`
	RuntimeNodeIDs      []uuid.UUID              `json:"runtime_node_ids"`
	ScenarioTemplateKey *string                  `json:"scenario_template_key"`
}

type projectRuntimeNodeResponse struct {
	RuntimeNodeID uuid.UUID `json:"runtime_node_id"`
}

func projectRuntimeNodeResponses(nodes []ProjectRuntimeNode) []projectRuntimeNodeResponse {
	responses := make([]projectRuntimeNodeResponse, 0, len(nodes))
	for _, node := range nodes {
		responses = append(responses, projectRuntimeNodeResponse{RuntimeNodeID: node.RuntimeNodeID})
	}
	return responses
}

type modifyProjectRuntimeNodeBody struct {
	Reason string `json:"reason"`
}

type updateProjectBody struct {
	TenantID           uuid.UUID                `json:"tenant_id,omitempty"`
	ActorUserID        uuid.UUID                `json:"actor_user_id,omitempty"`
	ProjectID          uuid.UUID                `json:"project_id,omitempty"`
	Name               string                   `json:"name"`
	Description        string                   `json:"description"`
	Goal               string                   `json:"goal"`
	HumanOwnerUserID   uuid.UUID                `json:"human_owner_user_id"`
	Members            *[]ProjectMemberInput    `json:"members"`
	CoordinationPolicy map[string]any           `json:"coordination_policy"`
	ApprovalPolicy     map[string]any           `json:"approval_policy"`
	EvidencePolicy     map[string]any           `json:"evidence_policy"`
	RepoBinding        *ProjectRepoBindingInput `json:"repo_binding"`
}

type submitDemandBody struct {
	TenantID                uuid.UUID               `json:"tenant_id,omitempty"`
	ProjectID               uuid.UUID               `json:"project_id,omitempty"`
	SubmittedByUserID       uuid.UUID               `json:"submitted_by_user_id,omitempty"`
	Title                   string                  `json:"title"`
	Content                 string                  `json:"content"`
	SourceType              DemandSourceType        `json:"source_type"`
	SourceRefs              map[string]any          `json:"source_refs"`
	Attachments             []any                   `json:"attachments"`
	ReviewerUserID          *uuid.UUID              `json:"reviewer_user_id"`
	ReviewerSelectionReason ReviewerSelectionReason `json:"reviewer_selection_reason"`
	CoordinationMode        string                  `json:"coordination_mode"`
	ScenarioTemplateKey     *string                 `json:"scenario_template_key"`
}

type resolveDecisionBody struct {
	Decision              string         `json:"decision"`
	Comment               string         `json:"comment"`
	Payload               map[string]any `json:"payload"`
	TargetExitDeliverable string         `json:"target_exit_deliverable,omitempty"`
}

type demandCriterionTaskSummaryResponse struct {
	TaskID       string                                `json:"task_id"`
	Summary      string                                `json:"summary"`
	Deliverables []demandCriterionDeliverableResponse `json:"deliverables"`
}

type demandCriterionDeliverableResponse struct {
	ArtifactRefID string `json:"artifact_ref_id"`
	Title         string `json:"title"`
	ContentType   string `json:"content_type,omitempty"`
	SizeBytes     *int64 `json:"size_bytes,omitempty"`
}

type demandAcceptanceCriterionDetailResponse struct {
	CriterionID        string                               `json:"criterion_id"`
	Statement          string                               `json:"statement"`
	VerificationMethod string                               `json:"verification_method"`
	Severity           string                               `json:"severity"`
	SatisfiedBy        []string                             `json:"satisfied_by"`
	Verdict            *string                              `json:"verdict"`
	JudgeType          *string                              `json:"judge_type"`
	EvidenceRefs       []string                             `json:"evidence_refs"`
	TaskSummaries      []demandCriterionTaskSummaryResponse `json:"task_summaries"`
}

type demandAcceptanceCriteriaResponse struct {
	DemandID     string                                    `json:"demand_id"`
	DemandStatus string                                    `json:"demand_status"`
	Criteria     []demandAcceptanceCriterionDetailResponse `json:"criteria"`
}

func demandAcceptanceCriteriaResponseFromDomain(detail DemandAcceptanceCriteriaDetail) demandAcceptanceCriteriaResponse {
	criteria := make([]demandAcceptanceCriterionDetailResponse, 0, len(detail.Criteria))
	for _, c := range detail.Criteria {
		satisfiedBy := c.SatisfiedBy
		if satisfiedBy == nil {
			satisfiedBy = []string{}
		}
		evidenceRefs := c.EvidenceRefs
		if evidenceRefs == nil {
			evidenceRefs = []string{}
		}
		summaries := make([]demandCriterionTaskSummaryResponse, 0, len(c.TaskSummaries))
		for _, s := range c.TaskSummaries {
			deliverables := make([]demandCriterionDeliverableResponse, 0, len(s.Deliverables))
			for _, d := range s.Deliverables {
				deliverables = append(deliverables, demandCriterionDeliverableResponse{
					ArtifactRefID: d.ArtifactRefID,
					Title:         d.Title,
					ContentType:   d.ContentType,
					SizeBytes:     d.SizeBytes,
				})
			}
			summaries = append(summaries, demandCriterionTaskSummaryResponse{
				TaskID:       s.TaskID,
				Summary:      s.Summary,
				Deliverables: deliverables,
			})
		}
		criteria = append(criteria, demandAcceptanceCriterionDetailResponse{
			CriterionID:        c.CriterionID,
			Statement:          c.Statement,
			VerificationMethod: c.VerificationMethod,
			Severity:           c.Severity,
			SatisfiedBy:        satisfiedBy,
			Verdict:            c.Verdict,
			JudgeType:          c.JudgeType,
			EvidenceRefs:       evidenceRefs,
			TaskSummaries:      summaries,
		})
	}
	return demandAcceptanceCriteriaResponse{
		DemandID:     detail.DemandID.String(),
		DemandStatus: string(detail.DemandStatus),
		Criteria:     criteria,
	}
}

type signDemandCriterionVerdictBody struct {
	CriterionID string `json:"criterion_id"`
	Verdict     string `json:"verdict"`
	Reason      string `json:"reason,omitempty"`
}

type signDemandCriterionVerdictResponse struct {
	DemandID     string `json:"demand_id"`
	DemandStatus string `json:"demand_status"`
	CriterionID  string `json:"criterion_id"`
	Verdict      string `json:"verdict"`
	Signed       int32  `json:"signed"`
	Total        int32  `json:"total"`
	Remaining    int32  `json:"remaining"`
}

func signDemandCriterionVerdictResponseFromDomain(result SignDemandCriterionVerdictResult) signDemandCriterionVerdictResponse {
	return signDemandCriterionVerdictResponse{
		DemandID:     result.DemandID.String(),
		DemandStatus: string(result.DemandStatus),
		CriterionID:  result.CriterionID,
		Verdict:      result.Verdict,
		Signed:       result.Signed,
		Total:        result.Total,
		Remaining:    result.Remaining,
	}
}

type ProjectTaskAttemptRuntimeBody struct {
	ProjectTaskID     uuid.UUID                     `json:"project_task_id"`
	LeaseToken        string                        `json:"lease_token"`
	RuntimeNodeID     uuid.UUID                     `json:"runtime_node_id"`
	IdempotencyKey    string                        `json:"idempotency_key"`
	ProviderSessionID *string                       `json:"provider_session_id"`
	RawLog            *projectTaskAttemptRawLogBody `json:"raw_log"`
}

type projectTaskAttemptRawLogBody struct {
	LogStore      string `json:"log_store"`
	LogRef        string `json:"log_ref"`
	LogBytes      int64  `json:"log_bytes"`
	LogSha256     string `json:"log_sha256"`
	LogCompressed bool   `json:"log_compressed"`
}

// toRawLog drops a malformed pointer rather than persisting a reference the
// control plane cannot resolve later.
func (b *projectTaskAttemptRawLogBody) toRawLog() *ProjectTaskAttemptRawLog {
	if b == nil {
		return nil
	}
	if b.LogStore == "" || b.LogRef == "" || b.LogSha256 == "" {
		return nil
	}
	return &ProjectTaskAttemptRawLog{
		LogStore:      b.LogStore,
		LogRef:        b.LogRef,
		LogBytes:      b.LogBytes,
		LogSha256:     b.LogSha256,
		LogCompressed: b.LogCompressed,
	}
}

type startProjectTaskAttemptBody struct {
	ProjectTaskAttemptRuntimeBody
	CommandID string `json:"command_id"`
}

type renewProjectTaskAttemptLeaseBody struct {
	ProjectTaskAttemptRuntimeBody
	LeaseExpiresAt *time.Time `json:"lease_expires_at"`
}

type completeProjectTaskAttemptBody struct {
	ProjectTaskAttemptRuntimeBody
	Conclusion            string              `json:"conclusion"`
	EvidenceRefs          []any               `json:"evidence_refs"`
	ArtifactRefs          []any               `json:"artifact_refs"`
	ConfidenceFactors     map[string]any      `json:"confidence_factors"`
	Uncertainty           string              `json:"uncertainty"`
	MissingInformation    []any               `json:"missing_information"`
	RecommendedNextAction string              `json:"recommended_next_action"`
	RequiresHumanReview   bool                `json:"requires_human_review"`
	ResultContract        *TaskResultContract `json:"result_contract"`
}

type submitProjectTaskAttemptResultBody struct {
	ProjectTaskAttemptRuntimeBody
	ResultContract TaskResultContract `json:"result_contract"`
}

type recordProjectTaskAttemptBudgetHeartbeatBody struct {
	ProjectID            uuid.UUID `json:"project_id"`
	ProjectTaskID        uuid.UUID `json:"project_task_id"`
	ConsumedWallClockSec int32     `json:"consumed_wall_clock_sec"`
	ConsumedTokens       int32     `json:"consumed_tokens"`
}

type failProjectTaskAttemptBody struct {
	ProjectTaskAttemptRuntimeBody
	FailureSummary string              `json:"failure_summary"`
	FailureFamily  string              `json:"failure_family"`
	Retryable      *bool               `json:"retryable"`
	ResultContract *TaskResultContract `json:"result_contract"`
}

type waitHumanProjectTaskAttemptBody struct {
	ProjectTaskAttemptRuntimeBody
	DigitalEmployeeID          uuid.UUID           `json:"digital_employee_id"`
	Reason                     string              `json:"reason"`
	Summary                    string              `json:"summary"`
	MissingContextRefs         []any               `json:"missing_context_refs"`
	SuggestedResolutionOptions []string            `json:"suggested_resolution_options"`
	ResultContract             *TaskResultContract `json:"result_contract"`
}

type createProjectTaskAttestationBody struct {
	ProjectID                 uuid.UUID                              `json:"project_id"`
	ProjectTaskID             uuid.UUID                              `json:"project_task_id"`
	AttemptID                 uuid.UUID                              `json:"attempt_id"`
	RuntimeNodeID             uuid.UUID                              `json:"runtime_node_id"`
	DigitalEmployeeID         uuid.UUID                              `json:"digital_employee_id"`
	CapabilityManifestVersion string                                 `json:"capability_manifest_version"`
	ProviderAuthMode          ProjectTaskAttestationProviderAuthMode `json:"provider_auth_mode"`
	ProviderSessionID         *string                                `json:"provider_session_id"`
	AttestationType           string                                 `json:"attestation_type"`
	Status                    ProjectTaskAttestationStatus           `json:"status"`
	CommandArgv               []any                                  `json:"command_argv"`
	ExitCode                  *int32                                 `json:"exit_code"`
	DurationMs                *int64                                 `json:"duration_ms"`
	LogRef                    *string                                `json:"log_ref"`
	StdoutSha256              *string                                `json:"stdout_sha256"`
	StderrSha256              *string                                `json:"stderr_sha256"`
	ArtifactRefs              []any                                  `json:"artifact_refs"`
	ArtifactHashes            map[string]any                         `json:"artifact_hashes"`
	GitBranch                 *string                                `json:"git_branch"`
	GitBaseRef                *string                                `json:"git_base_ref"`
	GitHeadSha                *string                                `json:"git_head_sha"`
	GitDiffSha256             *string                                `json:"git_diff_sha256"`
	Metadata                  map[string]any                         `json:"metadata"`
	IdempotencyKey            string                                 `json:"idempotency_key"`
}

type createEvidenceBody struct {
	TenantID           uuid.UUID      `json:"tenant_id,omitempty"`
	ProjectID          uuid.UUID      `json:"project_id,omitempty"`
	ActorUserID        uuid.UUID      `json:"actor_user_id,omitempty"`
	ActorID            uuid.UUID      `json:"actor_id,omitempty"`
	ProjectTaskID      *uuid.UUID     `json:"project_task_id"`
	RouteDecisionID    *uuid.UUID     `json:"route_decision_id"`
	ExecutionSummaryID *uuid.UUID     `json:"execution_summary_id"`
	EvidenceType       string         `json:"evidence_type"`
	Title              string         `json:"title"`
	Summary            string         `json:"summary"`
	SourceType         string         `json:"source_type"`
	SourceRef          string         `json:"source_ref"`
	ArtifactRefID      *uuid.UUID     `json:"artifact_ref_id"`
	SubmittedByType    string         `json:"submitted_by_type,omitempty"`
	SubmittedByID      *uuid.UUID     `json:"submitted_by_id,omitempty"`
	Metadata           map[string]any `json:"metadata"`
}

type patchEvidenceBody struct {
	TenantID           uuid.UUID                  `json:"tenant_id,omitempty"`
	ProjectID          uuid.UUID                  `json:"project_id,omitempty"`
	EvidenceID         uuid.UUID                  `json:"evidence_id,omitempty"`
	ActorUserID        uuid.UUID                  `json:"actor_user_id,omitempty"`
	VerificationStatus EvidenceVerificationStatus `json:"verification_status"`
	Metadata           *map[string]any            `json:"metadata"`
}

type createAcceptanceBody struct {
	TenantID         uuid.UUID   `json:"tenant_id,omitempty"`
	ProjectID        uuid.UUID   `json:"project_id,omitempty"`
	AcceptedByUserID uuid.UUID   `json:"accepted_by_user_id,omitempty"`
	Status           string      `json:"status"`
	Conclusion       string      `json:"conclusion"`
	Summary          string      `json:"summary"`
	EvidenceRefIDs   []uuid.UUID `json:"evidence_ref_ids"`
	ReportRefIDs     []uuid.UUID `json:"report_ref_ids"`
	UnresolvedRisks  []any       `json:"unresolved_risks"`
}

type createArchiveSnapshotBody struct {
	TenantID        uuid.UUID `json:"tenant_id,omitempty"`
	ProjectID       uuid.UUID `json:"project_id,omitempty"`
	CreatedByUserID uuid.UUID `json:"created_by_user_id,omitempty"`
	SnapshotType    string    `json:"snapshot_type"`
	Summary         string    `json:"summary"`
	ObjectRef       string    `json:"object_ref"`
}

type projectResponse struct {
	ID                     string                     `json:"id"`
	TenantID               string                     `json:"tenant_id"`
	TeamID                 *string                    `json:"team_id,omitempty"`
	Name                   string                     `json:"name"`
	Description            *string                    `json:"description,omitempty"`
	Goal                   string                     `json:"goal"`
	Status                 ProjectStatus              `json:"status"`
	HumanOwnerUserID       string                     `json:"human_owner_user_id"`
	CoordinationWorkflowID string                     `json:"coordination_workflow_id"`
	CoordinationStatus     string                     `json:"coordination_status"`
	CoordinationPolicy     map[string]any             `json:"coordination_policy"`
	ApprovalPolicy         map[string]any             `json:"approval_policy"`
	EvidencePolicy         map[string]any             `json:"evidence_policy"`
	RepoBinding            projectRepoBindingResponse `json:"repo_binding"`
	ScenarioTemplateKey    *string                    `json:"scenario_template_key,omitempty"`
	ArchivedAt             *string                    `json:"archived_at,omitempty"`
	AllowedActions         []string                   `json:"allowed_actions,omitempty"`
	CreatedAt              string                     `json:"created_at,omitempty"`
	UpdatedAt              string                     `json:"updated_at,omitempty"`
}

type projectDeletePreviewResponse struct {
	ProjectID   string                 `json:"project_id"`
	ProjectName string                 `json:"project_name"`
	CanDelete   bool                   `json:"can_delete"`
	Blockers    []ProjectDeleteBlocker `json:"blockers"`
	Warnings    ProjectDeleteWarnings  `json:"warnings"`
	Message     string                 `json:"message"`
}

type projectDeleteBlockedResponse struct {
	Code     string                 `json:"code"`
	Message  string                 `json:"message"`
	Blockers []ProjectDeleteBlocker `json:"blockers"`
}

type projectRepoBindingResponse struct {
	Status           ProjectRepoBindingStatus `json:"status"`
	URL              string                   `json:"url,omitempty"`
	DefaultBranch    string                   `json:"default_branch,omitempty"`
	GitCredentialRef *string                  `json:"git_credential_ref,omitempty"`
	Scope            []string                 `json:"scope"`
}

type createProjectResponse struct {
	Project projectResponse         `json:"project"`
	Members []projectMemberResponse `json:"members"`
}

type workflowInstanceResponse struct {
	DemandID                  string                                  `json:"demand_id"`
	ProjectID                 string                                  `json:"project_id"`
	ProjectName               string                                  `json:"project_name"`
	Title                     string                                  `json:"title"`
	SubmittedByUserID         string                                  `json:"submitted_by_user_id"`
	SubmittedByDisplayName    string                                  `json:"submitted_by_display_name"`
	Status                    string                                  `json:"status"`
	StatusReason              string                                  `json:"status_reason"`
	CreatedAt                 time.Time                               `json:"created_at"`
	UpdatedAt                 time.Time                               `json:"updated_at"`
	SelectedCoordinationJobID *string                                 `json:"selected_coordination_job_id,omitempty"`
	Progress                  workflowInstanceProgressResponse        `json:"progress"`
	CurrentBlocker            *workflowInstanceCurrentBlockerResponse `json:"current_blocker,omitempty"`
	Priority                  *workflowInstancePriorityResponse       `json:"priority,omitempty"`
	Risk                      *workflowInstanceRiskResponse           `json:"risk,omitempty"`
	SLA                       *workflowInstanceSLAResponse            `json:"sla,omitempty"`
	RecentEvent               *workflowInstanceRecentEventResponse    `json:"recent_event,omitempty"`
}

type workflowInstanceProgressResponse struct {
	TotalNodes        int32 `json:"total_nodes"`
	CompletedNodes    int32 `json:"completed_nodes"`
	RunningNodes      int32 `json:"running_nodes"`
	BlockedNodes      int32 `json:"blocked_nodes"`
	WaitingHumanNodes int32 `json:"waiting_human_nodes"`
	PlannedNodes      int32 `json:"planned_nodes,omitempty"`
	FailedNodes       int32 `json:"failed_nodes,omitempty"`
	CancelledNodes    int32 `json:"cancelled_nodes,omitempty"`
}

type workflowInstanceCurrentBlockerResponse struct {
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	ResourceID *string `json:"resource_id,omitempty"`
}

type workflowInstancePriorityResponse struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Source string `json:"source"`
}

type workflowInstanceRiskResponse struct {
	Level  string `json:"level"`
	Label  string `json:"label"`
	Source string `json:"source"`
}

type workflowInstanceSLAResponse struct {
	DueAt            *string `json:"due_at,omitempty"`
	RemainingSeconds *int32  `json:"remaining_seconds,omitempty"`
	Breached         bool    `json:"breached"`
	Label            string  `json:"label"`
	Source           string  `json:"source"`
}

type workflowInstanceRecentEventResponse struct {
	EventType  string `json:"event_type"`
	Summary    string `json:"summary"`
	OccurredAt string `json:"occurred_at"`
}

type projectMemberResponse struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	ProjectID           string         `json:"project_id"`
	PrincipalType       PrincipalType  `json:"principal_type"`
	PrincipalID         string         `json:"principal_id"`
	ProjectRole         ProjectRole    `json:"project_role"`
	DisplayNameSnapshot *string        `json:"display_name_snapshot,omitempty"`
	Status              string         `json:"status"`
	Settings            map[string]any `json:"settings"`
}

type projectTaskResponse struct {
	ID                        string         `json:"id"`
	TenantID                  string         `json:"tenant_id"`
	ProjectID                 string         `json:"project_id"`
	DemandID                  *string        `json:"demand_id,omitempty"`
	Title                     string         `json:"title"`
	Summary                   *string        `json:"summary,omitempty"`
	Status                    string         `json:"status"`
	AssignedDigitalEmployeeID *string        `json:"assigned_digital_employee_id,omitempty"`
	RiskLevel                 *string        `json:"risk_level,omitempty"`
	RequiresHumanApproval     bool           `json:"requires_human_approval"`
	CoordinationJobID         *string        `json:"coordination_job_id,omitempty"`
	RouteDecisionID           *string        `json:"route_decision_id,omitempty"`
	PlannedTaskKey            *string        `json:"planned_task_key,omitempty"`
	TaskKind                  *string        `json:"task_kind,omitempty"`
	StageIndex                *int32         `json:"stage_index,omitempty"`
	ExpectedOutputs           []any          `json:"expected_outputs"`
	InputRequirements         map[string]any `json:"input_requirements"`
	HandoffContract           map[string]any `json:"handoff_contract"`
	PlannerMetadata           map[string]any `json:"planner_metadata"`
	CreatedAt                 string         `json:"created_at"`
	UpdatedAt                 string         `json:"updated_at"`
}

type projectTaskGraphResponse struct {
	Nodes              []projectTaskGraphNodeResponse         `json:"nodes"`
	Edges              []projectTaskGraphEdgeResponse         `json:"edges"`
	Employees          []projectTaskGraphEmployeeResponse     `json:"employees"`
	Runs               []projectTaskGraphRunResponse          `json:"runs"`
	ExecutionSummaries []executionSummaryResponse             `json:"execution_summaries"`
	RecentEvents       []projectEventResponse                 `json:"recent_events"`
	DecisionRequests   []decisionRequestResponse              `json:"decision_requests"`
	StageSummaries     []projectTaskGraphStageSummaryResponse `json:"stage_summaries,omitempty"`
	BlockingFacts      []projectTaskGraphBlockingFactResponse `json:"blocking_facts"`
}

type projectTaskLivenessResponse struct {
	ProjectTaskID         string                                `json:"project_task_id"`
	Liveness              string                                `json:"liveness"`
	Reason                string                                `json:"reason,omitempty"`
	BlockingDependencyIDs []string                              `json:"blocking_dependency_ids"`
	CurrentAttemptID      *string                               `json:"current_attempt_id,omitempty"`
	WaitingRequestID      *string                               `json:"waiting_request_id,omitempty"`
	RetryNotBefore        *string                               `json:"retry_not_before,omitempty"`
	LeaseExpiresAt        *string                               `json:"lease_expires_at,omitempty"`
	NextAction            projectTaskLivenessNextActionResponse `json:"next_action"`
	IsTerminal            bool                                  `json:"is_terminal"`
	Attempt               projectTaskLivenessAttemptResponse    `json:"attempt"`
}

type projectTaskLivenessNextActionResponse struct {
	Source string `json:"source"`
}

type projectTaskLivenessAttemptResponse struct {
	ID     *string `json:"id,omitempty"`
	Status string  `json:"status"`
}

type projectTaskGraphNodeResponse struct {
	ID                        string                                  `json:"id"`
	TenantID                  string                                  `json:"tenant_id"`
	ProjectID                 string                                  `json:"project_id"`
	DemandID                  *string                                 `json:"demand_id,omitempty"`
	Title                     string                                  `json:"title"`
	Summary                   *string                                 `json:"summary,omitempty"`
	Status                    string                                  `json:"status"`
	AssignedDigitalEmployeeID *string                                 `json:"assigned_digital_employee_id,omitempty"`
	RiskLevel                 *string                                 `json:"risk_level,omitempty"`
	RequiresHumanApproval     bool                                    `json:"requires_human_approval"`
	CoordinationJobID         *string                                 `json:"coordination_job_id,omitempty"`
	RouteDecisionID           *string                                 `json:"route_decision_id,omitempty"`
	PlannedTaskKey            *string                                 `json:"planned_task_key,omitempty"`
	TaskKind                  *string                                 `json:"task_kind,omitempty"`
	StageIndex                *int32                                  `json:"stage_index,omitempty"`
	ExpectedOutputs           []any                                   `json:"expected_outputs"`
	InputRequirements         map[string]any                          `json:"input_requirements"`
	HandoffContract           map[string]any                          `json:"handoff_contract"`
	PlannerMetadata           map[string]any                          `json:"planner_metadata"`
	StatusReason              string                                  `json:"status_reason,omitempty"`
	CreatedAt                 string                                  `json:"created_at,omitempty"`
	UpdatedAt                 string                                  `json:"updated_at,omitempty"`
	StartedAt                 string                                  `json:"started_at,omitempty"`
	FinishedAt                string                                  `json:"finished_at,omitempty"`
	CurrentBlocker            *workflowInstanceCurrentBlockerResponse `json:"current_blocker,omitempty"`
}

type projectTaskGraphStageSummaryResponse struct {
	StageIndex        int32  `json:"stage_index"`
	Title             string `json:"title"`
	TotalNodes        int32  `json:"total_nodes"`
	CompletedNodes    int32  `json:"completed_nodes"`
	RunningNodes      int32  `json:"running_nodes"`
	WaitingHumanNodes int32  `json:"waiting_human_nodes"`
	BlockedNodes      int32  `json:"blocked_nodes"`
}

type projectTaskGraphBlockingFactResponse struct {
	ReasonCode        string                                   `json:"reason_code"`
	Message           string                                   `json:"message"`
	ResourceType      string                                   `json:"resource_type"`
	ResourceID        string                                   `json:"resource_id"`
	RecommendedAction string                                   `json:"recommended_action"`
	CreatedAt         string                                   `json:"created_at"`
	Gap               *projectTaskGraphBlockingFactGapResponse `json:"gap,omitempty"`
	DecisionRequestID string                                   `json:"decision_request_id,omitempty"`
}

type projectTaskGraphBlockingFactGapResponse struct {
	ConstraintKind       string   `json:"constraint_kind"`
	Roles                []string `json:"roles"`
	RequiredCapabilities []string `json:"required_capabilities"`
	ActiveExecutorCount  int      `json:"active_executor_count"`
	Options              []string `json:"options"`
}

type projectTaskGraphEdgeResponse struct {
	DependentTaskID   string  `json:"dependent_task_id"`
	BlockerTaskID     string  `json:"blocker_task_id"`
	CoordinationJobID *string `json:"coordination_job_id,omitempty"`
	EdgeStatus        string  `json:"edge_status"`
}

type projectTaskGraphEmployeeResponse struct {
	DigitalEmployeeID string                                       `json:"digital_employee_id"`
	DisplayName       string                                       `json:"display_name"`
	ProjectRole       ProjectRole                                  `json:"project_role"`
	EmployeeRole      string                                       `json:"employee_role,omitempty"`
	AvatarAsset       *projectTaskGraphEmployeeAvatarAssetResponse `json:"avatar_asset,omitempty"`
	Status            string                                       `json:"status"`
}

type projectTaskGraphEmployeeAvatarAssetResponse struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	ImageURL     string `json:"image_url"`
	ThumbnailURL string `json:"thumbnail_url"`
}

type projectTaskGraphRunResponse struct {
	ProjectTaskID        string  `json:"project_task_id"`
	DigitalEmployeeRunID *string `json:"digital_employee_run_id,omitempty"`
	RuntimeTaskID        *string `json:"runtime_task_id,omitempty"`
	RuntimeNodeID        *string `json:"runtime_node_id,omitempty"`
	RuntimeNodeSummary   string  `json:"runtime_node_summary"`
	Status               string  `json:"status"`
	ProviderType         string  `json:"provider_type"`
}

type coordinationJobResponse struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenant_id"`
	ProjectID        string         `json:"project_id"`
	WorkflowID       string         `json:"workflow_id"`
	TriggerEventID   *string        `json:"trigger_event_id,omitempty"`
	JobType          string         `json:"job_type"`
	Status           string         `json:"status"`
	InputSnapshotRef map[string]any `json:"input_snapshot_ref"`
	OutputEventIDs   []any          `json:"output_event_ids"`
	StartedAt        *string        `json:"started_at,omitempty"`
	FinishedAt       *string        `json:"finished_at,omitempty"`
	CreatedAt        string         `json:"created_at,omitempty"`
}

type routeDecisionResponse struct {
	ID                          string         `json:"id"`
	TenantID                    string         `json:"tenant_id"`
	ProjectID                   string         `json:"project_id"`
	CoordinationJobID           string         `json:"coordination_job_id"`
	DemandID                    *string        `json:"demand_id,omitempty"`
	CandidateDigitalEmployeeIDs []string       `json:"candidate_digital_employee_ids"`
	SelectedDigitalEmployeeIDs  []string       `json:"selected_digital_employee_ids"`
	Reason                      string         `json:"reason"`
	InputRequirements           map[string]any `json:"input_requirements"`
	ExpectedOutputs             []any          `json:"expected_outputs"`
	BudgetEstimate              map[string]any `json:"budget_estimate"`
	RequiresHumanReview         bool           `json:"requires_human_review"`
	CreatedEventID              *string        `json:"created_event_id,omitempty"`
	CreatedAt                   string         `json:"created_at,omitempty"`
}

type planRevisionResponse struct {
	ID                     string         `json:"id"`
	TenantID               string         `json:"tenant_id"`
	TeamID                 *string        `json:"team_id,omitempty"`
	ProjectID              string         `json:"project_id"`
	DemandID               string         `json:"demand_id"`
	CoordinationJobID      *string        `json:"coordination_job_id,omitempty"`
	RouteDecisionID        *string        `json:"route_decision_id,omitempty"`
	RevisionNumber         int32          `json:"revision_number"`
	Status                 string         `json:"status"`
	Payload                map[string]any `json:"payload"`
	PlannerProvider        *string        `json:"planner_provider,omitempty"`
	PlannerModel           *string        `json:"planner_model,omitempty"`
	PlannerInputHash       *string        `json:"planner_input_hash,omitempty"`
	PlanFingerprint        string         `json:"plan_fingerprint"`
	ValidationErrors       []string       `json:"validation_errors"`
	ValidationWarnings     []string       `json:"validation_warnings"`
	ReviewRequired         bool           `json:"review_required"`
	ReviewReason           *string        `json:"review_reason,omitempty"`
	AcceptedBy             *string        `json:"accepted_by,omitempty"`
	AcceptedAt             *string        `json:"accepted_at,omitempty"`
	RejectedBy             *string        `json:"rejected_by,omitempty"`
	RejectedAt             *string        `json:"rejected_at,omitempty"`
	RejectionReason        *string        `json:"rejection_reason,omitempty"`
	SupersededByRevisionID *string        `json:"superseded_by_revision_id,omitempty"`
	DecompositionClaimID   *string        `json:"decomposition_claim_id,omitempty"`
	CreatedTaskIDs         []string       `json:"created_task_ids"`
	CreatedAt              string         `json:"created_at,omitempty"`
	UpdatedAt              string         `json:"updated_at,omitempty"`
}

type dispatchGateResponse struct {
	ID                     string                        `json:"id"`
	ProjectTaskID          string                        `json:"project_task_id"`
	AcceptedPlanRevisionID *string                       `json:"accepted_plan_revision_id,omitempty"`
	PlannedTaskKey         *string                       `json:"planned_task_key,omitempty"`
	SelectedEmployeeID     string                        `json:"selected_employee_id"`
	AttemptNo              int32                         `json:"attempt_no"`
	DispatchReason         string                        `json:"dispatch_reason"`
	Status                 string                        `json:"status"`
	CheckedAt              time.Time                     `json:"checked_at"`
	Checks                 []dispatchGateCheckResponse   `json:"checks"`
	Blockers               []dispatchGateBlockerResponse `json:"blockers"`
	HumanActionRequest     map[string]any                `json:"human_action_request"`
	RetryAfter             *time.Time                    `json:"retry_after,omitempty"`
	AttemptID              *string                       `json:"attempt_id,omitempty"`
	DecisionRequestID      *string                       `json:"decision_request_id,omitempty"`
}

type dispatchGateCheckResponse struct {
	Key     string         `json:"key"`
	Status  string         `json:"status"`
	Details map[string]any `json:"details"`
}

type dispatchGateBlockerResponse struct {
	Key       string         `json:"key"`
	Severity  string         `json:"severity"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details"`
}

type executionSummaryResponse struct {
	ID                    string         `json:"id"`
	TenantID              string         `json:"tenant_id"`
	ProjectID             string         `json:"project_id"`
	ProjectTaskID         string         `json:"project_task_id"`
	DigitalEmployeeID     string         `json:"digital_employee_id"`
	Conclusion            string         `json:"conclusion"`
	EvidenceRefs          []any          `json:"evidence_refs"`
	ArtifactRefs          []any          `json:"artifact_refs"`
	ConfidenceFactors     map[string]any `json:"confidence_factors"`
	Uncertainty           *string        `json:"uncertainty,omitempty"`
	MissingInformation    []any          `json:"missing_information"`
	RecommendedNextAction *string        `json:"recommended_next_action,omitempty"`
	RequiresHumanReview   bool           `json:"requires_human_review"`
	TransferRequestID     *string        `json:"transfer_request_id,omitempty"`
	CreatedEventID        *string        `json:"created_event_id,omitempty"`
	CreatedAt             string         `json:"created_at,omitempty"`
}

type projectExecutionTraceResponse struct {
	ProjectID string                                 `json:"project_id"`
	Summary   projectExecutionTraceSummaryResponse   `json:"summary"`
	Attempts  []projectExecutionTraceAttemptResponse `json:"attempts"`
}

type projectExecutionTraceSummaryResponse struct {
	AttemptCount             int32   `json:"attempt_count"`
	FailedAttemptCount       int32   `json:"failed_attempt_count"`
	HumanReviewRequiredCount int32   `json:"human_review_required_count"`
	ArtifactRefCount         int32   `json:"artifact_ref_count"`
	EvidenceRefCount         int32   `json:"evidence_ref_count"`
	LatestErrorFamily        *string `json:"latest_error_family,omitempty"`
}

type projectExecutionTraceAttemptResponse struct {
	ProjectTaskID     string                                       `json:"project_task_id"`
	AttemptID         string                                       `json:"attempt_id"`
	AttemptNo         int32                                        `json:"attempt_no"`
	Status            string                                       `json:"status"`
	RuntimeNodeID     *string                                      `json:"runtime_node_id,omitempty"`
	ProviderType      *string                                      `json:"provider_type,omitempty"`
	ProviderSessionID *string                                      `json:"provider_session_id,omitempty"`
	StartedAt         *string                                      `json:"started_at,omitempty"`
	FinishedAt        *string                                      `json:"finished_at,omitempty"`
	FailureFamily     *string                                      `json:"failure_family,omitempty"`
	Retryable         *bool                                        `json:"retryable,omitempty"`
	Events            []executionLedgerEventResponse               `json:"events"`
	Summary           *projectExecutionTraceAttemptSummaryResponse `json:"summary,omitempty"`
}

type executionLedgerEventResponse struct {
	ID                   string         `json:"id"`
	TenantID             string         `json:"tenant_id"`
	TeamID               *string        `json:"team_id,omitempty"`
	ProjectID            string         `json:"project_id"`
	ProjectTaskID        *string        `json:"project_task_id,omitempty"`
	ProjectTaskAttemptID *string        `json:"project_task_attempt_id,omitempty"`
	EventType            string         `json:"event_type"`
	SourceType           string         `json:"source_type"`
	SourceID             string         `json:"source_id"`
	ActorType            string         `json:"actor_type"`
	ActorID              *string        `json:"actor_id,omitempty"`
	RuntimeNodeID        *string        `json:"runtime_node_id,omitempty"`
	ProviderType         *string        `json:"provider_type,omitempty"`
	ProviderSessionID    *string        `json:"provider_session_id,omitempty"`
	InputSummary         *string        `json:"input_summary,omitempty"`
	OutputSummary        *string        `json:"output_summary,omitempty"`
	ErrorFamily          *string        `json:"error_family,omitempty"`
	ErrorCode            *string        `json:"error_code,omitempty"`
	ErrorMessage         *string        `json:"error_message,omitempty"`
	Retryable            *bool          `json:"retryable,omitempty"`
	ArtifactRefs         []any          `json:"artifact_refs"`
	EvidenceRefs         []any          `json:"evidence_refs"`
	Metadata             map[string]any `json:"metadata"`
	OccurredAt           string         `json:"occurred_at"`
	CreatedAt            string         `json:"created_at"`
}

type projectExecutionTraceAttemptSummaryResponse struct {
	ExecutionSummaryID  string `json:"execution_summary_id"`
	Conclusion          string `json:"conclusion"`
	RequiresHumanReview bool   `json:"requires_human_review"`
	ArtifactRefs        []any  `json:"artifact_refs"`
	EvidenceRefs        []any  `json:"evidence_refs"`
	CreatedAt           string `json:"created_at"`
}

type transferRequestResponse struct {
	ID                           string   `json:"id"`
	TenantID                     string   `json:"tenant_id"`
	ProjectID                    string   `json:"project_id"`
	ProjectTaskID                string   `json:"project_task_id"`
	RequestedByDigitalEmployeeID string   `json:"requested_by_digital_employee_id"`
	Reason                       string   `json:"reason"`
	SuggestedEmployeeType        *string  `json:"suggested_employee_type,omitempty"`
	SuggestedDigitalEmployeeIDs  []string `json:"suggested_digital_employee_ids"`
	MissingContextRefs           []any    `json:"missing_context_refs"`
	Status                       string   `json:"status"`
	CreatedEventID               *string  `json:"created_event_id,omitempty"`
	CreatedAt                    string   `json:"created_at,omitempty"`
	UpdatedAt                    string   `json:"updated_at,omitempty"`
}

type decisionRequestResponse struct {
	ID                string  `json:"id"`
	TenantID          string  `json:"tenant_id"`
	ProjectID         string  `json:"project_id"`
	ApprovalRequestID string  `json:"approval_request_id"`
	CoordinationJobID *string `json:"coordination_job_id,omitempty"`
	ProjectTaskID     *string `json:"project_task_id,omitempty"`
	PlanRevisionID    *string `json:"plan_revision_id,omitempty"`
	TargetUserID      string  `json:"target_user_id"`
	DecisionType      string  `json:"decision_type"`
	TitleSnapshot     string  `json:"title_snapshot"`
	SummarySnapshot   *string `json:"summary_snapshot,omitempty"`
	RiskLevelSnapshot *string `json:"risk_level_snapshot,omitempty"`
	StatusSnapshot    string  `json:"status_snapshot"`
	CreatedEventID    *string `json:"created_event_id,omitempty"`
	ResolvedEventID   *string `json:"resolved_event_id,omitempty"`
	CreatedAt         string  `json:"created_at,omitempty"`
	UpdatedAt         string  `json:"updated_at,omitempty"`
	ResolvedAt        *string `json:"resolved_at,omitempty"`
}

type projectEventResponse struct {
	ID             string           `json:"id"`
	TenantID       string           `json:"tenant_id"`
	ProjectID      string           `json:"project_id"`
	SequenceNumber int64            `json:"sequence_number"`
	EventType      ProjectEventType `json:"event_type"`
	ActorType      string           `json:"actor_type"`
	ActorID        string           `json:"actor_id"`
	ResourceType   *string          `json:"resource_type,omitempty"`
	ResourceID     *string          `json:"resource_id,omitempty"`
	Summary        *string          `json:"summary,omitempty"`
	Payload        map[string]any   `json:"payload"`
}

type projectDemandResponse struct {
	ID                  string                      `json:"id"`
	TenantID            string                      `json:"tenant_id"`
	ProjectID           string                      `json:"project_id"`
	SubmittedByUserID   string                      `json:"submitted_by_user_id"`
	Title               string                      `json:"title"`
	Content             *string                     `json:"content,omitempty"`
	SourceType          DemandSourceType            `json:"source_type"`
	SourceRefs          map[string]any              `json:"source_refs"`
	Attachments         []any                       `json:"attachments"`
	Status              ProjectDemandStatus         `json:"status"`
	CreatedEventID      *string                     `json:"created_event_id,omitempty"`
	Reviewer            *reviewerPreferenceResponse `json:"reviewer"`
	CoordinationMode    string                      `json:"coordination_mode"`
	ScenarioTemplateKey *string                     `json:"scenario_template_key,omitempty"`
}

type demandLaunchDetailResponse struct {
	Demand             projectDemandResponse       `json:"demand"`
	Project            projectResponse             `json:"project"`
	Reviewer           *reviewerPreferenceResponse `json:"reviewer"`
	CoordinationJobs   []coordinationJobResponse   `json:"coordination_jobs"`
	RouteDecisions     []routeDecisionResponse     `json:"route_decisions"`
	ProjectTasks       []projectTaskResponse       `json:"project_tasks"`
	ExecutionSummaries []executionSummaryResponse  `json:"execution_summaries"`
	DecisionRequests   []decisionRequestResponse   `json:"decision_requests"`
	RecentEvents       []projectEventResponse      `json:"recent_events"`
}

type reviewerPreferenceResponse struct {
	ReviewerUserID   string                  `json:"reviewer_user_id"`
	SelectionReason  ReviewerSelectionReason `json:"selection_reason"`
	DisplayName      *string                 `json:"display_name,omitempty"`
	ProjectRole      ProjectRole             `json:"project_role"`
	ResolvedFromRule bool                    `json:"resolved_from_rule"`
}

type projectEvidenceResponse struct {
	ID                 string                     `json:"id"`
	TenantID           string                     `json:"tenant_id"`
	ProjectID          string                     `json:"project_id"`
	ProjectTaskID      *string                    `json:"project_task_id,omitempty"`
	RouteDecisionID    *string                    `json:"route_decision_id,omitempty"`
	ExecutionSummaryID *string                    `json:"execution_summary_id,omitempty"`
	EvidenceType       string                     `json:"evidence_type"`
	Title              string                     `json:"title"`
	Summary            *string                    `json:"summary,omitempty"`
	SourceType         string                     `json:"source_type"`
	SourceRef          string                     `json:"source_ref"`
	ArtifactRefID      *string                    `json:"artifact_ref_id,omitempty"`
	SubmittedByType    string                     `json:"submitted_by_type"`
	SubmittedByID      *string                    `json:"submitted_by_id,omitempty"`
	VerificationStatus EvidenceVerificationStatus `json:"verification_status"`
	Metadata           map[string]any             `json:"metadata"`
	CreatedEventID     *string                    `json:"created_event_id,omitempty"`
	CreatedAt          string                     `json:"created_at,omitempty"`
	UpdatedAt          string                     `json:"updated_at,omitempty"`
}

type projectArtifactResponse struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenant_id"`
	ProjectID         string         `json:"project_id"`
	ProjectTaskID     *string        `json:"project_task_id,omitempty"`
	AttemptID         *string        `json:"attempt_id,omitempty"`
	DigitalEmployeeID *string        `json:"digital_employee_id,omitempty"`
	ArtifactID        *string        `json:"artifact_id,omitempty"`
	ArtifactType      string         `json:"artifact_type"`
	Title             string         `json:"title"`
	ObjectRef         string         `json:"object_ref"`
	ContentType       *string        `json:"content_type,omitempty"`
	SizeBytes         *int64         `json:"size_bytes,omitempty"`
	Checksum          *string        `json:"checksum,omitempty"`
	RetentionStatus   string         `json:"retention_status"`
	RetentionHoldID   *string        `json:"retention_hold_id,omitempty"`
	Metadata          map[string]any `json:"metadata"`
	CreatedEventID    *string        `json:"created_event_id,omitempty"`
	CreatedAt         string         `json:"created_at,omitempty"`
	UpdatedAt         string         `json:"updated_at,omitempty"`
}

type projectReportResponse struct {
	ID              string  `json:"id"`
	TenantID        string  `json:"tenant_id"`
	ProjectID       string  `json:"project_id"`
	ReportType      string  `json:"report_type"`
	Title           string  `json:"title"`
	Summary         *string `json:"summary,omitempty"`
	ObjectRef       string  `json:"object_ref"`
	Format          string  `json:"format"`
	GeneratedByType string  `json:"generated_by_type"`
	GeneratedByID   *string `json:"generated_by_id,omitempty"`
	CreatedEventID  *string `json:"created_event_id,omitempty"`
	CreatedAt       string  `json:"created_at,omitempty"`
}

type projectBudgetLedgerResponse struct {
	ID                string  `json:"id"`
	TenantID          string  `json:"tenant_id"`
	ProjectID         string  `json:"project_id"`
	CoordinationJobID *string `json:"coordination_job_id,omitempty"`
	ProjectTaskID     *string `json:"project_task_id,omitempty"`
	DigitalEmployeeID *string `json:"digital_employee_id,omitempty"`
	CostType          string  `json:"cost_type"`
	EstimatedTokens   *int64  `json:"estimated_tokens,omitempty"`
	ActualTokens      *int64  `json:"actual_tokens,omitempty"`
	EstimatedCost     string  `json:"estimated_cost"`
	ActualCost        string  `json:"actual_cost"`
	Source            string  `json:"source"`
	Reason            *string `json:"reason,omitempty"`
	CreatedEventID    *string `json:"created_event_id,omitempty"`
	CreatedAt         string  `json:"created_at,omitempty"`
}

type projectBudgetSummaryResponse struct {
	EstimatedTokens int64  `json:"estimated_tokens"`
	ActualTokens    int64  `json:"actual_tokens"`
	EstimatedCost   string `json:"estimated_cost"`
	ActualCost      string `json:"actual_cost"`
	LedgerCount     int32  `json:"ledger_count"`
}

type projectAcceptanceResponse struct {
	ID               string   `json:"id"`
	TenantID         string   `json:"tenant_id"`
	ProjectID        string   `json:"project_id"`
	AcceptedByUserID string   `json:"accepted_by_user_id"`
	Status           string   `json:"status"`
	Conclusion       string   `json:"conclusion"`
	Summary          *string  `json:"summary,omitempty"`
	EvidenceRefIDs   []string `json:"evidence_ref_ids"`
	ReportRefIDs     []string `json:"report_ref_ids"`
	UnresolvedRisks  []any    `json:"unresolved_risks"`
	CreatedEventID   *string  `json:"created_event_id,omitempty"`
	CreatedAt        string   `json:"created_at,omitempty"`
}

type projectArchivePreviewResponse struct {
	ProjectID           string `json:"project_id"`
	EvidenceCount       int64  `json:"evidence_count"`
	ArtifactCount       int64  `json:"artifact_count"`
	ReportCount         int64  `json:"report_count"`
	RetentionPending    bool   `json:"retention_pending"`
	BlockedReasons      []any  `json:"blocked_reasons"`
	EstimatedObjectRefs []any  `json:"estimated_object_refs"`
}

type projectArchiveSnapshotResponse struct {
	ID                   string         `json:"id"`
	TenantID             string         `json:"tenant_id"`
	ProjectID            string         `json:"project_id"`
	SnapshotType         string         `json:"snapshot_type"`
	Status               string         `json:"status"`
	ObjectRef            *string        `json:"object_ref,omitempty"`
	Summary              *string        `json:"summary,omitempty"`
	IncludedCounts       map[string]any `json:"included_counts"`
	RetainedArtifactIDs  []string       `json:"retained_artifact_ids"`
	RetentionLockEventID *string        `json:"retention_lock_event_id,omitempty"`
	CreatedByUserID      string         `json:"created_by_user_id"`
	CreatedEventID       *string        `json:"created_event_id,omitempty"`
	CreatedAt            string         `json:"created_at,omitempty"`
}

type projectConfigRevisionResponse struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenant_id"`
	ProjectID          string         `json:"project_id"`
	RevisionNumber     int32          `json:"revision_number"`
	ConfigSnapshot     map[string]any `json:"config_snapshot"`
	ChangeSummary      *string        `json:"change_summary,omitempty"`
	CreatedByUserID    string         `json:"created_by_user_id"`
	CreatedEventID     *string        `json:"created_event_id,omitempty"`
	CreatedAt          string         `json:"created_at,omitempty"`
	ChangedSections    []any          `json:"changed_sections"`
	PreviousRevisionID *string        `json:"previous_revision_id,omitempty"`
	PolicyFingerprint  *string        `json:"policy_fingerprint,omitempty"`
	DiffSummary        map[string]any `json:"diff_summary"`
}

type projectTaskAttestationResponse struct {
	ID                        string                                 `json:"id"`
	TenantID                  string                                 `json:"tenant_id"`
	ProjectID                 string                                 `json:"project_id"`
	ProjectTaskID             string                                 `json:"project_task_id"`
	AttemptID                 string                                 `json:"attempt_id"`
	RuntimeNodeID             string                                 `json:"runtime_node_id"`
	DigitalEmployeeID         string                                 `json:"digital_employee_id"`
	CapabilityManifestVersion string                                 `json:"capability_manifest_version"`
	ProviderAuthMode          ProjectTaskAttestationProviderAuthMode `json:"provider_auth_mode"`
	ProviderSessionID         *string                                `json:"provider_session_id,omitempty"`
	AttestationType           string                                 `json:"attestation_type"`
	Status                    ProjectTaskAttestationStatus           `json:"status"`
	CommandArgv               []any                                  `json:"command_argv"`
	ExitCode                  *int32                                 `json:"exit_code,omitempty"`
	DurationMs                *int64                                 `json:"duration_ms,omitempty"`
	LogRef                    *string                                `json:"log_ref,omitempty"`
	StdoutSha256              *string                                `json:"stdout_sha256,omitempty"`
	StderrSha256              *string                                `json:"stderr_sha256,omitempty"`
	ArtifactRefs              []any                                  `json:"artifact_refs"`
	ArtifactHashes            map[string]any                         `json:"artifact_hashes"`
	GitBranch                 *string                                `json:"git_branch,omitempty"`
	GitBaseRef                *string                                `json:"git_base_ref,omitempty"`
	GitHeadSha                *string                                `json:"git_head_sha,omitempty"`
	GitDiffSha256             *string                                `json:"git_diff_sha256,omitempty"`
	Metadata                  map[string]any                         `json:"metadata"`
	IdempotencyKey            string                                 `json:"idempotency_key"`
	CreatedAt                 string                                 `json:"created_at,omitempty"`
	UpdatedAt                 string                                 `json:"updated_at,omitempty"`
}

type projectTaskAttemptBudgetHeartbeatResponse struct {
	Tripped    bool    `json:"tripped"`
	TripReason *string `json:"trip_reason,omitempty"`
}

type projectConfigResponse struct {
	Project              projectResponse             `json:"project"`
	HumanRoles           []projectMemberResponse     `json:"human_roles"`
	DigitalEmployeePool  []projectMemberResponse     `json:"digital_employee_pool"`
	Members              []projectMemberResponse     `json:"members"`
	CoordinationPolicy   map[string]any              `json:"coordination_policy"`
	ApprovalPolicy       map[string]any              `json:"approval_policy"`
	EvidencePolicy       map[string]any              `json:"evidence_policy"`
	CoordinationWorkflow ProjectCoordinationWorkflow `json:"coordination_workflow"`
}

type projectOverviewResponse struct {
	Project              projectResponse             `json:"project"`
	HumanRoles           []projectMemberResponse     `json:"human_roles"`
	DigitalEmployeePool  []projectMemberResponse     `json:"digital_employee_pool"`
	StatusSummary        ProjectStatusSummary        `json:"status_summary"`
	TaskSummary          ProjectTaskSummary          `json:"task_summary"`
	ActiveTasks          []projectTaskResponse       `json:"active_tasks"`
	RecentEvents         []projectEventResponse      `json:"recent_events"`
	CoordinationWorkflow ProjectCoordinationWorkflow `json:"coordination_workflow"`
}

func createProjectResponseFromDomain(result *CreateProjectResult) createProjectResponse {
	return createProjectResponse{Project: projectResponseFromDomain(result.Project), Members: memberResponses(result.Members)}
}

func projectResponses(projects []Project) []projectResponse {
	responses := make([]projectResponse, 0, len(projects))
	for _, project := range projects {
		responses = append(responses, projectResponseFromDomain(project))
	}
	return responses
}

func projectResponseFromDomain(project Project) projectResponse {
	return projectResponse{
		ID:                     project.ID.String(),
		TenantID:               project.TenantID.String(),
		TeamID:                 stringPtr(project.TeamID),
		Name:                   project.Name,
		Description:            project.Description,
		Goal:                   project.Goal,
		Status:                 project.Status,
		HumanOwnerUserID:       project.HumanOwnerUserID.String(),
		CoordinationWorkflowID: project.CoordinationWorkflowID,
		CoordinationStatus:     project.CoordinationStatus,
		CoordinationPolicy:     mapOrEmpty(project.CoordinationPolicy),
		ApprovalPolicy:         mapOrEmpty(project.ApprovalPolicy),
		EvidencePolicy:         mapOrEmpty(project.EvidencePolicy),
		RepoBinding:            projectRepoBindingResponseFromDomain(project.RepoBinding),
		ScenarioTemplateKey:    project.ScenarioTemplateKey,
		ArchivedAt:             timePtr(project.ArchivedAt),
		CreatedAt:              timeValue(project.CreatedAt),
		UpdatedAt:              timeValue(project.UpdatedAt),
	}
}

func projectRepoBindingResponseFromDomain(binding ProjectRepoBinding) projectRepoBindingResponse {
	if binding.Status != ProjectRepoBindingStatusBound {
		return projectRepoBindingResponse{Status: ProjectRepoBindingStatusUnbound, Scope: []string{}}
	}
	return projectRepoBindingResponse{
		Status:           binding.Status,
		URL:              binding.URL,
		DefaultBranch:    binding.DefaultBranch,
		GitCredentialRef: binding.GitCredentialRef,
		Scope:            append([]string(nil), binding.Scope...),
	}
}

func workflowInstanceResponses(items []WorkflowInstanceSummary) []workflowInstanceResponse {
	responses := make([]workflowInstanceResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, workflowInstanceResponseFromDomain(item))
	}
	return responses
}

func workflowInstanceResponseFromDomain(item WorkflowInstanceSummary) workflowInstanceResponse {
	var jobID *string
	if item.SelectedCoordinationJobID != nil {
		value := item.SelectedCoordinationJobID.String()
		jobID = &value
	}
	return workflowInstanceResponse{
		DemandID:                  item.DemandID.String(),
		ProjectID:                 item.ProjectID.String(),
		ProjectName:               item.ProjectName,
		Title:                     item.Title,
		SubmittedByUserID:         item.SubmittedByUserID.String(),
		SubmittedByDisplayName:    item.SubmittedByDisplayName,
		Status:                    string(item.Status),
		StatusReason:              item.StatusReason,
		CreatedAt:                 item.CreatedAt,
		UpdatedAt:                 item.UpdatedAt,
		SelectedCoordinationJobID: jobID,
		Progress: workflowInstanceProgressResponse{
			TotalNodes:        item.Progress.TotalNodes,
			CompletedNodes:    item.Progress.CompletedNodes,
			RunningNodes:      item.Progress.RunningNodes,
			BlockedNodes:      item.Progress.BlockedNodes,
			WaitingHumanNodes: item.Progress.WaitingHumanNodes,
			PlannedNodes:      item.Progress.PlannedNodes,
			FailedNodes:       item.Progress.FailedNodes,
			CancelledNodes:    item.Progress.CancelledNodes,
		},
		CurrentBlocker: workflowBlockerResponse(item.CurrentBlocker),
		Priority:       workflowPriorityResponse(item.Priority),
		Risk:           workflowRiskResponse(item.Risk),
		SLA:            workflowSLAResponse(item.SLA),
		RecentEvent:    workflowRecentEventResponse(item.RecentEvent),
	}
}

func workflowBlockerResponse(blocker *WorkflowInstanceCurrentBlocker) *workflowInstanceCurrentBlockerResponse {
	if blocker == nil {
		return nil
	}
	var resourceID *string
	if blocker.ResourceID != nil {
		value := blocker.ResourceID.String()
		resourceID = &value
	}
	return &workflowInstanceCurrentBlockerResponse{
		Type:       blocker.Type,
		Title:      blocker.Title,
		ResourceID: resourceID,
	}
}

func workflowPriorityResponse(priority *WorkflowInstancePriority) *workflowInstancePriorityResponse {
	if priority == nil {
		return nil
	}
	return &workflowInstancePriorityResponse{Value: priority.Value, Label: priority.Label, Source: priority.Source}
}

func workflowRiskResponse(risk *WorkflowInstanceRisk) *workflowInstanceRiskResponse {
	if risk == nil {
		return nil
	}
	return &workflowInstanceRiskResponse{Level: risk.Level, Label: risk.Label, Source: risk.Source}
}

func workflowSLAResponse(sla *WorkflowInstanceSLA) *workflowInstanceSLAResponse {
	if sla == nil {
		return nil
	}
	var due *string
	if sla.DueAt != nil {
		value := sla.DueAt.Format(time.RFC3339)
		due = &value
	}
	return &workflowInstanceSLAResponse{
		DueAt:            due,
		RemainingSeconds: sla.RemainingSeconds,
		Breached:         sla.Breached,
		Label:            sla.Label,
		Source:           sla.Source,
	}
}

func workflowRecentEventResponse(event *WorkflowInstanceRecentEvent) *workflowInstanceRecentEventResponse {
	if event == nil {
		return nil
	}
	return &workflowInstanceRecentEventResponse{
		EventType:  event.EventType,
		Summary:    event.Summary,
		OccurredAt: event.OccurredAt.Format(time.RFC3339),
	}
}

func memberResponses(members []ProjectMember) []projectMemberResponse {
	responses := make([]projectMemberResponse, 0, len(members))
	for _, member := range members {
		responses = append(responses, projectMemberResponse{
			ID:                  member.ID.String(),
			TenantID:            member.TenantID.String(),
			ProjectID:           member.ProjectID.String(),
			PrincipalType:       member.PrincipalType,
			PrincipalID:         member.PrincipalID.String(),
			ProjectRole:         member.ProjectRole,
			DisplayNameSnapshot: member.DisplayNameSnapshot,
			Status:              member.Status,
			Settings:            mapOrEmpty(member.Settings),
		})
	}
	return responses
}

func taskResponses(tasks []ProjectTask) []projectTaskResponse {
	responses := make([]projectTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		responses = append(responses, taskResponseFromDomain(task))
	}
	return responses
}

func taskResponseFromDomain(task ProjectTask) projectTaskResponse {
	return projectTaskResponse{
		ID:                        task.ID.String(),
		TenantID:                  task.TenantID.String(),
		ProjectID:                 task.ProjectID.String(),
		DemandID:                  stringPtr(task.DemandID),
		Title:                     task.Title,
		Summary:                   task.Summary,
		Status:                    task.Status,
		AssignedDigitalEmployeeID: stringPtr(task.AssignedDigitalEmployeeID),
		RiskLevel:                 task.RiskLevel,
		RequiresHumanApproval:     task.RequiresHumanApproval,
		CoordinationJobID:         stringPtr(task.CoordinationJobID),
		RouteDecisionID:           stringPtr(task.RouteDecisionID),
		PlannedTaskKey:            task.PlannedTaskKey,
		TaskKind:                  task.TaskKind,
		StageIndex:                task.StageIndex,
		ExpectedOutputs:           sliceOrEmpty(task.ExpectedOutputs),
		InputRequirements:         mapOrEmpty(task.InputRequirements),
		HandoffContract:           mapOrEmpty(task.HandoffContract),
		PlannerMetadata:           mapOrEmpty(task.PlannerMetadata),
		CreatedAt:                 timeValue(task.CreatedAt),
		UpdatedAt:                 timeValue(task.UpdatedAt),
	}
}

func taskGraphResponseFromDomain(graph ProjectTaskGraph) projectTaskGraphResponse {
	return projectTaskGraphResponse{
		Nodes:              taskGraphNodeResponses(graph.Nodes),
		Edges:              taskGraphEdgeResponses(graph.Edges),
		Employees:          taskGraphEmployeeResponses(graph.Employees),
		Runs:               taskGraphRunResponses(graph.Runs),
		ExecutionSummaries: executionSummaryResponses(graph.ExecutionSummaries),
		RecentEvents:       eventResponses(graph.RecentEvents),
		DecisionRequests:   decisionRequestResponses(graph.DecisionRequests),
		StageSummaries:     taskGraphStageSummaryResponses(graph.StageSummaries),
		BlockingFacts:      taskGraphBlockingFactResponses(graph.BlockingFacts),
	}
}

func taskLivenessResponseFromDomain(item ProjectTaskLiveness) projectTaskLivenessResponse {
	return projectTaskLivenessResponse{
		ProjectTaskID:         item.ProjectTaskID.String(),
		Liveness:              item.Liveness,
		Reason:                item.Reason,
		BlockingDependencyIDs: uuidStrings(item.BlockingDependencyIDs),
		CurrentAttemptID:      stringPtr(item.CurrentAttemptID),
		WaitingRequestID:      stringPtr(item.WaitingRequestID),
		RetryNotBefore:        timePtr(item.RetryNotBefore),
		LeaseExpiresAt:        timePtr(item.LeaseExpiresAt),
		NextAction:            projectTaskLivenessNextActionResponse{Source: item.NextAction},
		IsTerminal:            item.Liveness == ProjectTaskLivenessTerminal,
		Attempt: projectTaskLivenessAttemptResponse{
			ID:     stringPtr(item.CurrentAttemptID),
			Status: item.AttemptStatus,
		},
	}
}

func taskGraphStageSummaryResponses(items []ProjectTaskGraphStageSummary) []projectTaskGraphStageSummaryResponse {
	responses := make([]projectTaskGraphStageSummaryResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, projectTaskGraphStageSummaryResponse{
			StageIndex:        item.StageIndex,
			Title:             item.Title,
			TotalNodes:        item.TotalNodes,
			CompletedNodes:    item.CompletedNodes,
			RunningNodes:      item.RunningNodes,
			WaitingHumanNodes: item.WaitingHumanNodes,
			BlockedNodes:      item.BlockedNodes,
		})
	}
	return responses
}

func taskGraphBlockingFactResponses(items []ProjectTaskGraphBlockingFact) []projectTaskGraphBlockingFactResponse {
	responses := make([]projectTaskGraphBlockingFactResponse, 0, len(items))
	for _, item := range items {
		response := projectTaskGraphBlockingFactResponse{
			ReasonCode:        item.ReasonCode,
			Message:           item.Message,
			ResourceType:      item.ResourceType,
			ResourceID:        item.ResourceID,
			RecommendedAction: item.RecommendedAction,
			CreatedAt:         item.CreatedAt.Format(time.RFC3339),
			DecisionRequestID: item.DecisionRequestID,
		}
		if item.Gap != nil {
			response.Gap = &projectTaskGraphBlockingFactGapResponse{
				ConstraintKind:       item.Gap.ConstraintKind,
				Roles:                item.Gap.Roles,
				RequiredCapabilities: item.Gap.RequiredCapabilities,
				ActiveExecutorCount:  item.Gap.ActiveExecutorCount,
				Options:              item.Gap.Options,
			}
		}
		responses = append(responses, response)
	}
	return responses
}

func taskGraphNodeResponses(nodes []ProjectTaskGraphNode) []projectTaskGraphNodeResponse {
	responses := make([]projectTaskGraphNodeResponse, 0, len(nodes))
	for _, node := range nodes {
		responses = append(responses, taskGraphNodeResponseFromDomain(node))
	}
	return responses
}

func taskGraphNodeResponseFromDomain(node ProjectTaskGraphNode) projectTaskGraphNodeResponse {
	task := node.Task
	createdAt := ""
	if !task.CreatedAt.IsZero() {
		createdAt = task.CreatedAt.Format(time.RFC3339)
	}
	updatedAt := ""
	if node.UpdatedAt != nil {
		updatedAt = node.UpdatedAt.Format(time.RFC3339)
	} else if !task.UpdatedAt.IsZero() {
		updatedAt = task.UpdatedAt.Format(time.RFC3339)
	}
	startedAt := ""
	if node.StartedAt != nil {
		startedAt = node.StartedAt.Format(time.RFC3339)
	}
	finishedAt := ""
	if node.FinishedAt != nil {
		finishedAt = node.FinishedAt.Format(time.RFC3339)
	}
	return projectTaskGraphNodeResponse{
		ID:                        task.ID.String(),
		TenantID:                  task.TenantID.String(),
		ProjectID:                 task.ProjectID.String(),
		DemandID:                  stringPtr(task.DemandID),
		Title:                     task.Title,
		Summary:                   task.Summary,
		Status:                    task.Status,
		AssignedDigitalEmployeeID: stringPtr(task.AssignedDigitalEmployeeID),
		RiskLevel:                 task.RiskLevel,
		RequiresHumanApproval:     task.RequiresHumanApproval,
		CoordinationJobID:         stringPtr(task.CoordinationJobID),
		RouteDecisionID:           stringPtr(task.RouteDecisionID),
		PlannedTaskKey:            task.PlannedTaskKey,
		TaskKind:                  task.TaskKind,
		StageIndex:                task.StageIndex,
		ExpectedOutputs:           sliceOrEmpty(task.ExpectedOutputs),
		InputRequirements:         mapOrEmpty(task.InputRequirements),
		HandoffContract:           mapOrEmpty(task.HandoffContract),
		PlannerMetadata:           mapOrEmpty(task.PlannerMetadata),
		StatusReason:              node.StatusReason,
		CreatedAt:                 createdAt,
		UpdatedAt:                 updatedAt,
		StartedAt:                 startedAt,
		FinishedAt:                finishedAt,
		CurrentBlocker:            workflowBlockerResponse(node.CurrentBlocker),
	}
}

func taskGraphEdgeResponses(edges []ProjectTaskGraphEdge) []projectTaskGraphEdgeResponse {
	responses := make([]projectTaskGraphEdgeResponse, 0, len(edges))
	for _, edge := range edges {
		responses = append(responses, projectTaskGraphEdgeResponse{
			DependentTaskID:   edge.DependentTaskID.String(),
			BlockerTaskID:     edge.BlockerTaskID.String(),
			CoordinationJobID: stringPtr(edge.CoordinationJobID),
			EdgeStatus:        edge.EdgeStatus,
		})
	}
	return responses
}

func taskGraphEmployeeResponses(employees []ProjectTaskGraphEmployee) []projectTaskGraphEmployeeResponse {
	responses := make([]projectTaskGraphEmployeeResponse, 0, len(employees))
	for _, employee := range employees {
		responses = append(responses, projectTaskGraphEmployeeResponse{
			DigitalEmployeeID: employee.DigitalEmployeeID.String(),
			DisplayName:       employee.DisplayName,
			ProjectRole:       employee.ProjectRole,
			EmployeeRole:      employee.EmployeeRole,
			AvatarAsset:       taskGraphEmployeeAvatarAssetResponse(employee.AvatarAsset),
			Status:            employee.Status,
		})
	}
	return responses
}

func taskGraphEmployeeAvatarAssetResponse(asset *ProjectTaskGraphEmployeeAvatarAsset) *projectTaskGraphEmployeeAvatarAssetResponse {
	if asset == nil {
		return nil
	}
	return &projectTaskGraphEmployeeAvatarAssetResponse{
		ID:           asset.ID,
		Label:        asset.Label,
		ImageURL:     asset.ImageURL,
		ThumbnailURL: asset.ThumbnailURL,
	}
}

func taskGraphRunResponses(runs []ProjectTaskGraphRun) []projectTaskGraphRunResponse {
	responses := make([]projectTaskGraphRunResponse, 0, len(runs))
	for _, run := range runs {
		responses = append(responses, projectTaskGraphRunResponse{
			ProjectTaskID:        run.ProjectTaskID.String(),
			DigitalEmployeeRunID: stringPtr(run.DigitalEmployeeRunID),
			RuntimeTaskID:        stringPtr(run.RuntimeTaskID),
			RuntimeNodeID:        stringPtr(run.RuntimeNodeID),
			RuntimeNodeSummary:   run.RuntimeNodeSummary,
			Status:               run.Status,
			ProviderType:         run.ProviderType,
		})
	}
	return responses
}

func coordinationJobResponses(jobs []CoordinationJob) []coordinationJobResponse {
	responses := make([]coordinationJobResponse, 0, len(jobs))
	for _, job := range jobs {
		responses = append(responses, coordinationJobResponse{
			ID:               job.ID.String(),
			TenantID:         job.TenantID.String(),
			ProjectID:        job.ProjectID.String(),
			WorkflowID:       job.WorkflowID,
			TriggerEventID:   stringPtr(job.TriggerEventID),
			JobType:          job.JobType,
			Status:           job.Status,
			InputSnapshotRef: mapOrEmpty(job.InputSnapshotRef),
			OutputEventIDs:   sliceOrEmpty(job.OutputEventIDs),
			StartedAt:        timePtr(job.StartedAt),
			FinishedAt:       timePtr(job.FinishedAt),
			CreatedAt:        timeValue(job.CreatedAt),
		})
	}
	return responses
}

func routeDecisionResponses(decisions []RouteDecision) []routeDecisionResponse {
	responses := make([]routeDecisionResponse, 0, len(decisions))
	for _, decision := range decisions {
		responses = append(responses, routeDecisionResponse{
			ID:                          decision.ID.String(),
			TenantID:                    decision.TenantID.String(),
			ProjectID:                   decision.ProjectID.String(),
			CoordinationJobID:           decision.CoordinationJobID.String(),
			DemandID:                    stringPtr(decision.DemandID),
			CandidateDigitalEmployeeIDs: uuidStrings(decision.CandidateDigitalEmployeeIDs),
			SelectedDigitalEmployeeIDs:  uuidStrings(decision.SelectedDigitalEmployeeIDs),
			Reason:                      decision.Reason,
			InputRequirements:           mapOrEmpty(decision.InputRequirements),
			ExpectedOutputs:             sliceOrEmpty(decision.ExpectedOutputs),
			BudgetEstimate:              mapOrEmpty(decision.BudgetEstimate),
			RequiresHumanReview:         decision.RequiresHumanReview,
			CreatedEventID:              stringPtr(decision.CreatedEventID),
			CreatedAt:                   timeValue(decision.CreatedAt),
		})
	}
	return responses
}

func planRevisionResponses(revisions []PlanRevision) []planRevisionResponse {
	responses := make([]planRevisionResponse, 0, len(revisions))
	for _, revision := range revisions {
		responses = append(responses, planRevisionResponseFromDomain(revision))
	}
	return responses
}

func planRevisionResponseFromDomain(revision PlanRevision) planRevisionResponse {
	return planRevisionResponse{
		ID:                     revision.ID.String(),
		TenantID:               revision.TenantID.String(),
		TeamID:                 stringPtr(revision.TeamID),
		ProjectID:              revision.ProjectID.String(),
		DemandID:               revision.DemandID.String(),
		CoordinationJobID:      stringPtr(revision.CoordinationJobID),
		RouteDecisionID:        stringPtr(revision.RouteDecisionID),
		RevisionNumber:         revision.RevisionNumber,
		Status:                 revision.Status,
		Payload:                mapOrEmpty(revision.Payload),
		PlannerProvider:        revision.PlannerProvider,
		PlannerModel:           revision.PlannerModel,
		PlannerInputHash:       revision.PlannerInputHash,
		PlanFingerprint:        revision.PlanFingerprint,
		ValidationErrors:       stringSliceOrEmpty(revision.ValidationErrors),
		ValidationWarnings:     stringSliceOrEmpty(revision.ValidationWarnings),
		ReviewRequired:         revision.ReviewRequired,
		ReviewReason:           revision.ReviewReason,
		AcceptedBy:             stringPtr(revision.AcceptedBy),
		AcceptedAt:             timePtr(revision.AcceptedAt),
		RejectedBy:             stringPtr(revision.RejectedBy),
		RejectedAt:             timePtr(revision.RejectedAt),
		RejectionReason:        revision.RejectionReason,
		SupersededByRevisionID: stringPtr(revision.SupersededByRevisionID),
		DecompositionClaimID:   stringPtr(revision.DecompositionClaimID),
		CreatedTaskIDs:         uuidStrings(revision.CreatedTaskIDs),
		CreatedAt:              timeValue(revision.CreatedAt),
		UpdatedAt:              timeValue(revision.UpdatedAt),
	}
}

func dispatchGateResponses(results []PreDispatchGateResult) []dispatchGateResponse {
	responses := make([]dispatchGateResponse, 0, len(results))
	for _, result := range results {
		responses = append(responses, dispatchGateResponse{
			ID:                     result.ID.String(),
			ProjectTaskID:          result.ProjectTaskID.String(),
			AcceptedPlanRevisionID: stringPtr(result.AcceptedPlanRevisionID),
			PlannedTaskKey:         result.PlannedTaskKey,
			SelectedEmployeeID:     result.SelectedEmployeeID.String(),
			AttemptNo:              result.AttemptNo,
			DispatchReason:         result.DispatchReason,
			Status:                 result.Status,
			CheckedAt:              result.CheckedAt,
			Checks:                 dispatchGateCheckResponses(result.Checks),
			Blockers:               dispatchGateBlockerResponses(result.Blockers),
			HumanActionRequest:     mapOrEmpty(map[string]any(result.HumanActionRequest)),
			RetryAfter:             result.RetryAfter,
			AttemptID:              stringPtr(result.AttemptID),
			DecisionRequestID:      stringPtr(result.DecisionRequestID),
		})
	}
	return responses
}

func executionSummaryResponses(summaries []ExecutionSummary) []executionSummaryResponse {
	responses := make([]executionSummaryResponse, 0, len(summaries))
	for _, summary := range summaries {
		responses = append(responses, executionSummaryResponseFromDomain(summary))
	}
	return responses
}

func executionSummaryResponseFromDomain(summary ExecutionSummary) executionSummaryResponse {
	return executionSummaryResponse{
		ID:                    summary.ID.String(),
		TenantID:              summary.TenantID.String(),
		ProjectID:             summary.ProjectID.String(),
		ProjectTaskID:         summary.ProjectTaskID.String(),
		DigitalEmployeeID:     summary.DigitalEmployeeID.String(),
		Conclusion:            summary.Conclusion,
		EvidenceRefs:          sliceOrEmpty(summary.EvidenceRefs),
		ArtifactRefs:          sliceOrEmpty(summary.ArtifactRefs),
		ConfidenceFactors:     mapOrEmpty(summary.ConfidenceFactors),
		Uncertainty:           summary.Uncertainty,
		MissingInformation:    sliceOrEmpty(summary.MissingInformation),
		RecommendedNextAction: summary.RecommendedNextAction,
		RequiresHumanReview:   summary.RequiresHumanReview,
		TransferRequestID:     stringPtr(summary.TransferRequestID),
		CreatedEventID:        stringPtr(summary.CreatedEventID),
		CreatedAt:             timeValue(summary.CreatedAt),
	}
}

func projectTaskAttestationResponseFromDomain(attestation ProjectTaskAttestation) projectTaskAttestationResponse {
	return projectTaskAttestationResponse{
		ID:                        attestation.ID.String(),
		TenantID:                  attestation.TenantID.String(),
		ProjectID:                 attestation.ProjectID.String(),
		ProjectTaskID:             attestation.ProjectTaskID.String(),
		AttemptID:                 attestation.AttemptID.String(),
		RuntimeNodeID:             attestation.RuntimeNodeID.String(),
		DigitalEmployeeID:         attestation.DigitalEmployeeID.String(),
		CapabilityManifestVersion: attestation.CapabilityManifestVersion,
		ProviderAuthMode:          attestation.ProviderAuthMode,
		ProviderSessionID:         attestation.ProviderSessionID,
		AttestationType:           attestation.AttestationType,
		Status:                    attestation.Status,
		CommandArgv:               sliceOrEmpty(attestation.CommandArgv),
		ExitCode:                  attestation.ExitCode,
		DurationMs:                attestation.DurationMs,
		LogRef:                    attestation.LogRef,
		StdoutSha256:              attestation.StdoutSha256,
		StderrSha256:              attestation.StderrSha256,
		ArtifactRefs:              sliceOrEmpty(attestation.ArtifactRefs),
		ArtifactHashes:            mapOrEmpty(attestation.ArtifactHashes),
		GitBranch:                 attestation.GitBranch,
		GitBaseRef:                attestation.GitBaseRef,
		GitHeadSha:                attestation.GitHeadSha,
		GitDiffSha256:             attestation.GitDiffSha256,
		Metadata:                  mapOrEmpty(attestation.Metadata),
		IdempotencyKey:            attestation.IdempotencyKey,
		CreatedAt:                 timeValue(attestation.CreatedAt),
		UpdatedAt:                 timeValue(attestation.UpdatedAt),
	}
}

func executionTraceResponseFromDomain(trace ProjectExecutionTrace) projectExecutionTraceResponse {
	return projectExecutionTraceResponse{
		ProjectID: trace.ProjectID.String(),
		Summary: projectExecutionTraceSummaryResponse{
			AttemptCount:             trace.Summary.AttemptCount,
			FailedAttemptCount:       trace.Summary.FailedAttemptCount,
			HumanReviewRequiredCount: trace.Summary.HumanReviewRequiredCount,
			ArtifactRefCount:         trace.Summary.ArtifactRefCount,
			EvidenceRefCount:         trace.Summary.EvidenceRefCount,
			LatestErrorFamily:        trace.Summary.LatestErrorFamily,
		},
		Attempts: executionTraceAttemptResponses(trace.Attempts),
	}
}

func executionTraceAttemptResponses(attempts []ProjectExecutionTraceAttempt) []projectExecutionTraceAttemptResponse {
	responses := make([]projectExecutionTraceAttemptResponse, 0, len(attempts))
	for _, attempt := range attempts {
		responses = append(responses, executionTraceAttemptResponseFromDomain(attempt))
	}
	return responses
}

func executionTraceAttemptResponseFromDomain(attempt ProjectExecutionTraceAttempt) projectExecutionTraceAttemptResponse {
	return projectExecutionTraceAttemptResponse{
		ProjectTaskID:     attempt.ProjectTaskID.String(),
		AttemptID:         attempt.AttemptID.String(),
		AttemptNo:         attempt.AttemptNo,
		Status:            attempt.Status,
		RuntimeNodeID:     stringPtr(attempt.RuntimeNodeID),
		ProviderType:      attempt.ProviderType,
		ProviderSessionID: attempt.ProviderSessionID,
		StartedAt:         timePtr(attempt.StartedAt),
		FinishedAt:        timePtr(attempt.FinishedAt),
		FailureFamily:     attempt.FailureFamily,
		Retryable:         attempt.Retryable,
		Events:            executionLedgerEventResponses(attempt.Events),
		Summary:           executionTraceAttemptSummaryResponseFromDomain(attempt.Summary),
	}
}

func executionLedgerEventResponses(events []ExecutionLedgerEvent) []executionLedgerEventResponse {
	responses := make([]executionLedgerEventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, executionLedgerEventResponseFromDomain(event))
	}
	return responses
}

func executionLedgerEventResponseFromDomain(event ExecutionLedgerEvent) executionLedgerEventResponse {
	return executionLedgerEventResponse{
		ID:                   event.ID.String(),
		TenantID:             event.TenantID.String(),
		TeamID:               stringPtr(event.TeamID),
		ProjectID:            event.ProjectID.String(),
		ProjectTaskID:        stringPtr(event.ProjectTaskID),
		ProjectTaskAttemptID: stringPtr(event.ProjectTaskAttemptID),
		EventType:            event.EventType,
		SourceType:           event.SourceType,
		SourceID:             event.SourceID,
		ActorType:            event.ActorType,
		ActorID:              event.ActorID,
		RuntimeNodeID:        stringPtr(event.RuntimeNodeID),
		ProviderType:         event.ProviderType,
		ProviderSessionID:    event.ProviderSessionID,
		InputSummary:         event.InputSummary,
		OutputSummary:        event.OutputSummary,
		ErrorFamily:          event.ErrorFamily,
		ErrorCode:            event.ErrorCode,
		ErrorMessage:         event.ErrorMessage,
		Retryable:            event.Retryable,
		ArtifactRefs:         sliceOrEmpty(event.ArtifactRefs),
		EvidenceRefs:         sliceOrEmpty(event.EvidenceRefs),
		Metadata:             mapOrEmpty(event.Metadata),
		OccurredAt:           timeValue(event.OccurredAt),
		CreatedAt:            timeValue(event.CreatedAt),
	}
}

func executionTraceAttemptSummaryResponseFromDomain(summary *ProjectExecutionTraceAttemptSummary) *projectExecutionTraceAttemptSummaryResponse {
	if summary == nil {
		return nil
	}
	return &projectExecutionTraceAttemptSummaryResponse{
		ExecutionSummaryID:  summary.ExecutionSummaryID.String(),
		Conclusion:          summary.Conclusion,
		RequiresHumanReview: summary.RequiresHumanReview,
		ArtifactRefs:        sliceOrEmpty(summary.ArtifactRefs),
		EvidenceRefs:        sliceOrEmpty(summary.EvidenceRefs),
		CreatedAt:           timeValue(summary.CreatedAt),
	}
}

func transferRequestResponses(transfers []TransferRequest) []transferRequestResponse {
	responses := make([]transferRequestResponse, 0, len(transfers))
	for _, transfer := range transfers {
		responses = append(responses, transferRequestResponseFromDomain(transfer))
	}
	return responses
}

func transferRequestResponseFromDomain(transfer TransferRequest) transferRequestResponse {
	return transferRequestResponse{
		ID:                           transfer.ID.String(),
		TenantID:                     transfer.TenantID.String(),
		ProjectID:                    transfer.ProjectID.String(),
		ProjectTaskID:                transfer.ProjectTaskID.String(),
		RequestedByDigitalEmployeeID: transfer.RequestedByDigitalEmployeeID.String(),
		Reason:                       transfer.Reason,
		SuggestedEmployeeType:        transfer.SuggestedEmployeeType,
		SuggestedDigitalEmployeeIDs:  uuidStrings(transfer.SuggestedDigitalEmployeeIDs),
		MissingContextRefs:           sliceOrEmpty(transfer.MissingContextRefs),
		Status:                       transfer.Status,
		CreatedEventID:               stringPtr(transfer.CreatedEventID),
		CreatedAt:                    timeValue(transfer.CreatedAt),
		UpdatedAt:                    timeValue(transfer.UpdatedAt),
	}
}

func decisionRequestResponses(decisions []DecisionRequest) []decisionRequestResponse {
	responses := make([]decisionRequestResponse, 0, len(decisions))
	for _, decision := range decisions {
		responses = append(responses, decisionRequestResponseFromDomain(decision))
	}
	return responses
}

func decisionRequestResponseFromDomain(decision DecisionRequest) decisionRequestResponse {
	return decisionRequestResponse{
		ID:                decision.ID.String(),
		TenantID:          decision.TenantID.String(),
		ProjectID:         decision.ProjectID.String(),
		ApprovalRequestID: decision.ApprovalRequestID.String(),
		CoordinationJobID: stringPtr(decision.CoordinationJobID),
		ProjectTaskID:     stringPtr(decision.ProjectTaskID),
		PlanRevisionID:    stringPtr(decision.PlanRevisionID),
		TargetUserID:      decision.TargetUserID.String(),
		DecisionType:      decision.DecisionType,
		TitleSnapshot:     decision.TitleSnapshot,
		SummarySnapshot:   decision.SummarySnapshot,
		RiskLevelSnapshot: decision.RiskLevelSnapshot,
		StatusSnapshot:    decision.StatusSnapshot,
		CreatedEventID:    stringPtr(decision.CreatedEventID),
		ResolvedEventID:   stringPtr(decision.ResolvedEventID),
		CreatedAt:         timeValue(decision.CreatedAt),
		UpdatedAt:         timeValue(decision.UpdatedAt),
		ResolvedAt:        timePtr(decision.ResolvedAt),
	}
}

func eventResponses(events []ProjectEvent) []projectEventResponse {
	responses := make([]projectEventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, eventResponseFromDomain(event))
	}
	return responses
}

func eventResponseFromDomain(event ProjectEvent) projectEventResponse {
	return projectEventResponse{
		ID:             event.ID.String(),
		TenantID:       event.TenantID.String(),
		ProjectID:      event.ProjectID.String(),
		SequenceNumber: event.SequenceNumber,
		EventType:      event.EventType,
		ActorType:      event.ActorType,
		ActorID:        event.ActorID,
		ResourceType:   event.ResourceType,
		ResourceID:     event.ResourceID,
		Summary:        event.Summary,
		Payload:        mapOrEmpty(event.Payload),
	}
}

func demandResponses(demands []ProjectDemand) []projectDemandResponse {
	responses := make([]projectDemandResponse, 0, len(demands))
	for _, demand := range demands {
		responses = append(responses, demandResponseFromDomain(demand))
	}
	return responses
}

func demandResponseFromDomain(demand ProjectDemand) projectDemandResponse {
	return projectDemandResponse{
		ID:                  demand.ID.String(),
		TenantID:            demand.TenantID.String(),
		ProjectID:           demand.ProjectID.String(),
		SubmittedByUserID:   demand.SubmittedByUserID.String(),
		Title:               demand.Title,
		Content:             demand.Content,
		SourceType:          demand.SourceType,
		SourceRefs:          mapOrEmpty(demand.SourceRefs),
		Attachments:         sliceOrEmpty(demand.Attachments),
		Status:              demand.Status,
		CreatedEventID:      stringPtr(demand.CreatedEventID),
		Reviewer:            reviewerPreferenceResponseFromDomain(demand.ReviewerPreference),
		CoordinationMode:    demand.CoordinationMode,
		ScenarioTemplateKey: demand.ScenarioTemplateKey,
	}
}

func demandLaunchDetailResponseFromDomain(detail DemandLaunchDetail) demandLaunchDetailResponse {
	return demandLaunchDetailResponse{
		Demand:             demandResponseFromDomain(detail.Demand),
		Project:            projectResponseFromDomain(detail.Project),
		Reviewer:           reviewerPreferenceResponseFromDomain(detail.Reviewer),
		CoordinationJobs:   coordinationJobResponses(detail.CoordinationJobs),
		RouteDecisions:     routeDecisionResponses(detail.RouteDecisions),
		ProjectTasks:       taskResponses(detail.ProjectTasks),
		ExecutionSummaries: executionSummaryResponses(detail.ExecutionSummaries),
		DecisionRequests:   decisionRequestResponses(detail.DecisionRequests),
		RecentEvents:       eventResponses(detail.RecentEvents),
	}
}

func reviewerPreferenceResponseFromDomain(preference *ReviewerPreference) *reviewerPreferenceResponse {
	if preference == nil {
		return nil
	}
	return &reviewerPreferenceResponse{
		ReviewerUserID:   preference.ReviewerUserID.String(),
		SelectionReason:  preference.SelectionReason,
		DisplayName:      preference.DisplayName,
		ProjectRole:      preference.ProjectRole,
		ResolvedFromRule: preference.ResolvedFromRule,
	}
}

func evidenceResponses(evidence []ProjectEvidenceRef) []projectEvidenceResponse {
	responses := make([]projectEvidenceResponse, 0, len(evidence))
	for _, item := range evidence {
		responses = append(responses, evidenceResponseFromDomain(item))
	}
	return responses
}

func evidenceResponseFromDomain(evidence ProjectEvidenceRef) projectEvidenceResponse {
	return projectEvidenceResponse{
		ID:                 evidence.ID.String(),
		TenantID:           evidence.TenantID.String(),
		ProjectID:          evidence.ProjectID.String(),
		ProjectTaskID:      stringPtr(evidence.ProjectTaskID),
		RouteDecisionID:    stringPtr(evidence.RouteDecisionID),
		ExecutionSummaryID: stringPtr(evidence.ExecutionSummaryID),
		EvidenceType:       evidence.EvidenceType,
		Title:              evidence.Title,
		Summary:            evidence.Summary,
		SourceType:         evidence.SourceType,
		SourceRef:          evidence.SourceRef,
		ArtifactRefID:      stringPtr(evidence.ArtifactRefID),
		SubmittedByType:    evidence.SubmittedByType,
		SubmittedByID:      stringPtr(evidence.SubmittedByID),
		VerificationStatus: evidence.VerificationStatus,
		Metadata:           mapOrEmpty(evidence.Metadata),
		CreatedEventID:     stringPtr(evidence.CreatedEventID),
		CreatedAt:          timeValue(evidence.CreatedAt),
		UpdatedAt:          timeValue(evidence.UpdatedAt),
	}
}

func artifactResponses(artifacts []ProjectArtifactRef) []projectArtifactResponse {
	responses := make([]projectArtifactResponse, 0, len(artifacts))
	for _, artifact := range artifacts {
		responses = append(responses, artifactResponseFromDomain(artifact))
	}
	return responses
}

func artifactResponseFromDomain(artifact ProjectArtifactRef) projectArtifactResponse {
	return projectArtifactResponse{
		ID:                artifact.ID.String(),
		TenantID:          artifact.TenantID.String(),
		ProjectID:         artifact.ProjectID.String(),
		ProjectTaskID:     stringPtr(artifact.ProjectTaskID),
		AttemptID:         stringPtr(artifact.AttemptID),
		DigitalEmployeeID: stringPtr(artifact.DigitalEmployeeID),
		ArtifactID:        stringPtr(artifact.ArtifactID),
		ArtifactType:      artifact.ArtifactType,
		Title:             artifact.Title,
		ObjectRef:         artifact.ObjectRef,
		ContentType:       artifact.ContentType,
		SizeBytes:         artifact.SizeBytes,
		Checksum:          artifact.Checksum,
		RetentionStatus:   artifact.RetentionStatus,
		RetentionHoldID:   stringPtr(artifact.RetentionHoldID),
		Metadata:          mapOrEmpty(artifact.Metadata),
		CreatedEventID:    stringPtr(artifact.CreatedEventID),
		CreatedAt:         timeValue(artifact.CreatedAt),
		UpdatedAt:         timeValue(artifact.UpdatedAt),
	}
}

func reportResponses(reports []ProjectReportRef) []projectReportResponse {
	responses := make([]projectReportResponse, 0, len(reports))
	for _, report := range reports {
		responses = append(responses, reportResponseFromDomain(report))
	}
	return responses
}

func reportResponseFromDomain(report ProjectReportRef) projectReportResponse {
	return projectReportResponse{
		ID:              report.ID.String(),
		TenantID:        report.TenantID.String(),
		ProjectID:       report.ProjectID.String(),
		ReportType:      report.ReportType,
		Title:           report.Title,
		Summary:         report.Summary,
		ObjectRef:       report.ObjectRef,
		Format:          report.Format,
		GeneratedByType: report.GeneratedByType,
		GeneratedByID:   stringPtr(report.GeneratedByID),
		CreatedEventID:  stringPtr(report.CreatedEventID),
		CreatedAt:       timeValue(report.CreatedAt),
	}
}

func budgetLedgerResponses(ledger []ProjectBudgetLedgerEntry) []projectBudgetLedgerResponse {
	responses := make([]projectBudgetLedgerResponse, 0, len(ledger))
	for _, entry := range ledger {
		responses = append(responses, projectBudgetLedgerResponse{
			ID:                entry.ID.String(),
			TenantID:          entry.TenantID.String(),
			ProjectID:         entry.ProjectID.String(),
			CoordinationJobID: stringPtr(entry.CoordinationJobID),
			ProjectTaskID:     stringPtr(entry.ProjectTaskID),
			DigitalEmployeeID: stringPtr(entry.DigitalEmployeeID),
			CostType:          entry.CostType,
			EstimatedTokens:   entry.EstimatedTokens,
			ActualTokens:      entry.ActualTokens,
			EstimatedCost:     entry.EstimatedCost,
			ActualCost:        entry.ActualCost,
			Source:            entry.Source,
			Reason:            entry.Reason,
			CreatedEventID:    stringPtr(entry.CreatedEventID),
			CreatedAt:         timeValue(entry.CreatedAt),
		})
	}
	return responses
}

func budgetSummaryResponseFromDomain(summary ProjectBudgetSummary) projectBudgetSummaryResponse {
	return projectBudgetSummaryResponse{
		EstimatedTokens: summary.EstimatedTokens,
		ActualTokens:    summary.ActualTokens,
		EstimatedCost:   summary.EstimatedCost,
		ActualCost:      summary.ActualCost,
		LedgerCount:     summary.LedgerCount,
	}
}

func acceptanceResponseFromDomain(acceptance ProjectAcceptanceRecord) projectAcceptanceResponse {
	return projectAcceptanceResponse{
		ID:               acceptance.ID.String(),
		TenantID:         acceptance.TenantID.String(),
		ProjectID:        acceptance.ProjectID.String(),
		AcceptedByUserID: acceptance.AcceptedByUserID.String(),
		Status:           acceptance.Status,
		Conclusion:       acceptance.Conclusion,
		Summary:          acceptance.Summary,
		EvidenceRefIDs:   uuidStrings(acceptance.EvidenceRefIDs),
		ReportRefIDs:     uuidStrings(acceptance.ReportRefIDs),
		UnresolvedRisks:  sliceOrEmpty(acceptance.UnresolvedRisks),
		CreatedEventID:   stringPtr(acceptance.CreatedEventID),
		CreatedAt:        timeValue(acceptance.CreatedAt),
	}
}

func archivePreviewResponseFromDomain(preview ProjectArchivePreview) projectArchivePreviewResponse {
	return projectArchivePreviewResponse{
		ProjectID:           preview.ProjectID.String(),
		EvidenceCount:       preview.EvidenceCount,
		ArtifactCount:       preview.ArtifactCount,
		ReportCount:         preview.ReportCount,
		RetentionPending:    preview.RetentionPending,
		BlockedReasons:      sliceOrEmpty(preview.BlockedReasons),
		EstimatedObjectRefs: sliceOrEmpty(preview.EstimatedObjectRefs),
	}
}

func archiveSnapshotResponses(snapshots []ProjectArchiveSnapshot) []projectArchiveSnapshotResponse {
	responses := make([]projectArchiveSnapshotResponse, 0, len(snapshots))
	for _, snapshot := range snapshots {
		responses = append(responses, archiveSnapshotResponseFromDomain(snapshot))
	}
	return responses
}

func archiveSnapshotResponseFromDomain(snapshot ProjectArchiveSnapshot) projectArchiveSnapshotResponse {
	return projectArchiveSnapshotResponse{
		ID:                   snapshot.ID.String(),
		TenantID:             snapshot.TenantID.String(),
		ProjectID:            snapshot.ProjectID.String(),
		SnapshotType:         snapshot.SnapshotType,
		Status:               snapshot.Status,
		ObjectRef:            snapshot.ObjectRef,
		Summary:              snapshot.Summary,
		IncludedCounts:       mapOrEmpty(snapshot.IncludedCounts),
		RetainedArtifactIDs:  uuidStrings(snapshot.RetainedArtifactIDs),
		RetentionLockEventID: stringPtr(snapshot.RetentionLockEventID),
		CreatedByUserID:      snapshot.CreatedByUserID.String(),
		CreatedEventID:       stringPtr(snapshot.CreatedEventID),
		CreatedAt:            timeValue(snapshot.CreatedAt),
	}
}

func configRevisionResponses(revisions []ProjectConfigRevision) []projectConfigRevisionResponse {
	responses := make([]projectConfigRevisionResponse, 0, len(revisions))
	for _, revision := range revisions {
		responses = append(responses, configRevisionResponseFromDomain(revision))
	}
	return responses
}

func configRevisionResponseFromDomain(revision ProjectConfigRevision) projectConfigRevisionResponse {
	return projectConfigRevisionResponse{
		ID:                 revision.ID.String(),
		TenantID:           revision.TenantID.String(),
		ProjectID:          revision.ProjectID.String(),
		RevisionNumber:     revision.RevisionNumber,
		ConfigSnapshot:     mapOrEmpty(revision.ConfigSnapshot),
		ChangeSummary:      revision.ChangeSummary,
		CreatedByUserID:    revision.CreatedByUserID.String(),
		CreatedEventID:     stringPtr(revision.CreatedEventID),
		CreatedAt:          timeValue(revision.CreatedAt),
		ChangedSections:    sliceOrEmpty(revision.ChangedSections),
		PreviousRevisionID: stringPtr(revision.PreviousRevisionID),
		PolicyFingerprint:  revision.PolicyFingerprint,
		DiffSummary:        mapOrEmpty(revision.DiffSummary),
	}
}

func projectConfigResponseFromDomain(overview *ProjectOverview) projectConfigResponse {
	members := append([]ProjectMember{}, overview.HumanRoles...)
	members = append(members, overview.DigitalEmployeePool...)
	return projectConfigResponse{
		Project:              projectResponseFromDomain(overview.Project),
		HumanRoles:           memberResponses(overview.HumanRoles),
		DigitalEmployeePool:  memberResponses(overview.DigitalEmployeePool),
		Members:              memberResponses(members),
		CoordinationPolicy:   mapOrEmpty(overview.Project.CoordinationPolicy),
		ApprovalPolicy:       mapOrEmpty(overview.Project.ApprovalPolicy),
		EvidencePolicy:       mapOrEmpty(overview.Project.EvidencePolicy),
		CoordinationWorkflow: overview.CoordinationWorkflow,
	}
}

func overviewResponseFromDomain(overview *ProjectOverview) projectOverviewResponse {
	return projectOverviewResponse{
		Project:              projectResponseFromDomain(overview.Project),
		HumanRoles:           memberResponses(overview.HumanRoles),
		DigitalEmployeePool:  memberResponses(overview.DigitalEmployeePool),
		StatusSummary:        overview.StatusSummary,
		TaskSummary:          overview.TaskSummary,
		ActiveTasks:          taskResponses(overview.ActiveTasks),
		RecentEvents:         eventResponses(overview.RecentEvents),
		CoordinationWorkflow: overview.CoordinationWorkflow,
	}
}

func stringPtr(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func nonEmptyStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	text := value
	return &text
}

func timePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := value.Format(time.RFC3339)
	return &text
}

func timeValue(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func mapOrEmpty(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func sliceOrEmpty(value []any) []any {
	if value == nil {
		return []any{}
	}
	return value
}

func stringSliceOrEmpty(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func dispatchGateCheckResponses(checks []PreDispatchGateCheck) []dispatchGateCheckResponse {
	responses := make([]dispatchGateCheckResponse, 0, len(checks))
	for _, check := range checks {
		responses = append(responses, dispatchGateCheckResponse{
			Key:     check.Key,
			Status:  check.Status,
			Details: mapOrEmpty(check.Details),
		})
	}
	return responses
}

func dispatchGateBlockerResponses(blockers []PreDispatchGateBlocker) []dispatchGateBlockerResponse {
	responses := make([]dispatchGateBlockerResponse, 0, len(blockers))
	for _, blocker := range blockers {
		responses = append(responses, dispatchGateBlockerResponse{
			Key:       blocker.Key,
			Severity:  blocker.Severity,
			Retryable: blocker.Retryable,
			Details:   mapOrEmpty(blocker.Details),
		})
	}
	return responses
}

func uuidStrings(values []uuid.UUID) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.String())
	}
	return result
}
