package project

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
)

type HandlerService interface {
	CreateProject(ctx context.Context, req CreateProjectRequest) (*CreateProjectResult, error)
	GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (*Project, error)
	ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error)
	UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (*Project, error)
	ArchiveProject(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) (*Project, error)
	ReplaceProjectMembers(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error)
	ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error)
	ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error)
	ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error)
	RetryWorkflowSignal(ctx context.Context, req RetryWorkflowSignalRequest) (*ProjectEvent, error)
	SubmitDemand(ctx context.Context, req SubmitProjectDemandRequest) (*ProjectDemand, error)
	ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error)
	GetOverview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectOverview, error)
	ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error)
	ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error)
	ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error)
	ResolveDecision(ctx context.Context, req ResolveDecisionRequest) (*DecisionRequest, error)
	ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error)
	ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error)
	CompleteProjectTask(ctx context.Context, req CompleteProjectTaskRequest) (*ExecutionSummary, error)
	FailProjectTask(ctx context.Context, req FailProjectTaskRequest) (*ProjectTask, error)
	RequestProjectTaskTransfer(ctx context.Context, req RequestProjectTaskTransferRequest) (*TransferRequest, error)
}

type HTTPHandler struct {
	service HandlerService
}

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	_ = actorID
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

func (h *HTTPHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
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
		TenantID:           tenantID,
		TeamID:             req.TeamID,
		ActorUserID:        actorID,
		Name:               req.Name,
		Description:        req.Description,
		Goal:               req.Goal,
		HumanOwnerUserID:   req.HumanOwnerUserID,
		LeaderUserID:       req.LeaderUserID,
		AcceptanceUserID:   req.AcceptanceUserID,
		Members:            req.Members,
		CoordinationPolicy: req.CoordinationPolicy,
		ApprovalPolicy:     req.ApprovalPolicy,
		EvidencePolicy:     req.EvidencePolicy,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, createProjectResponseFromDomain(created))
}

func (h *HTTPHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	projectID, ok := projectIDFromRequest(w, r)
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
	writeJSON(w, http.StatusOK, projectResponseFromDomain(*project))
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
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: actorID,
		Title:             req.Title,
		Content:           req.Content,
		SourceType:        req.SourceType,
		SourceRefs:        req.SourceRefs,
		Attachments:       req.Attachments,
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
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          body.Decision,
		Comment:           body.Comment,
		Payload:           body.Payload,
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

func (h *HTTPHandler) CompleteProjectTask(w http.ResponseWriter, r *http.Request) {
	tenantID, runtimeNodeID, taskID, service, ok := h.runtimeProjectTaskContext(w, r)
	if !ok {
		return
	}
	var body completeProjectTaskBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	summary, err := service.CompleteProjectTask(r.Context(), CompleteProjectTaskRequest{
		TenantID:              tenantID,
		RuntimeNodeID:         runtimeNodeID,
		ProjectTaskID:         taskID,
		DigitalEmployeeID:     body.DigitalEmployeeID,
		Conclusion:            body.Conclusion,
		EvidenceRefs:          body.EvidenceRefs,
		ArtifactRefs:          body.ArtifactRefs,
		ConfidenceFactors:     body.ConfidenceFactors,
		Uncertainty:           body.Uncertainty,
		MissingInformation:    body.MissingInformation,
		RecommendedNextAction: body.RecommendedNextAction,
		RequiresHumanReview:   body.RequiresHumanReview,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, executionSummaryResponseFromDomain(*summary))
}

func (h *HTTPHandler) FailProjectTask(w http.ResponseWriter, r *http.Request) {
	tenantID, runtimeNodeID, taskID, service, ok := h.runtimeProjectTaskContext(w, r)
	if !ok {
		return
	}
	var body failProjectTaskBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	task, err := service.FailProjectTask(r.Context(), FailProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: body.DigitalEmployeeID,
		FailureSummary:    body.FailureSummary,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, taskResponseFromDomain(*task))
}

func (h *HTTPHandler) RequestProjectTaskTransfer(w http.ResponseWriter, r *http.Request) {
	tenantID, runtimeNodeID, taskID, service, ok := h.runtimeProjectTaskContext(w, r)
	if !ok {
		return
	}
	var body requestProjectTaskTransferBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	transfer, err := service.RequestProjectTaskTransfer(r.Context(), RequestProjectTaskTransferRequest{
		TenantID:                    tenantID,
		RuntimeNodeID:               runtimeNodeID,
		ProjectTaskID:               taskID,
		DigitalEmployeeID:           body.DigitalEmployeeID,
		Reason:                      body.Reason,
		SuggestedEmployeeType:       body.SuggestedEmployeeType,
		SuggestedDigitalEmployeeIDs: body.SuggestedDigitalEmployeeIDs,
		MissingContextRefs:          body.MissingContextRefs,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, transferRequestResponseFromDomain(*transfer))
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
		LeaderUserID:       req.LeaderUserID,
		AcceptanceUserID:   req.AcceptanceUserID,
		Members:            req.Members,
		CoordinationPolicy: req.CoordinationPolicy,
		ApprovalPolicy:     req.ApprovalPolicy,
		EvidencePolicy:     req.EvidencePolicy,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectResponseFromDomain(*updated))
}

func (h *HTTPHandler) projectRouteContext(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, HandlerService, bool) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	projectID, ok := projectIDFromRequest(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	return tenantID, actorID, projectID, service, true
}

func (h *HTTPHandler) runtimeProjectTaskContext(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, HandlerService, bool) {
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
	taskID, ok := projectTaskIDFromRequest(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, false
	}
	return tenantID, runtimeNodeID, taskID, service, true
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "project service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func consoleIdentity(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
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

func projectTaskIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	taskID, err := uuid.Parse(chi.URLParam(r, "projectTaskId"))
	if err != nil || taskID == uuid.Nil {
		http.Error(w, "invalid project task id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return taskID, true
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

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidProject), errors.Is(err, ErrInvalidProjectMember):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrProjectNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrProjectTaskForbidden):
		http.Error(w, "project task forbidden", http.StatusForbidden)
	case errors.Is(err, ErrProjectArchived):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type createProjectBody struct {
	TenantID           uuid.UUID            `json:"tenant_id,omitempty"`
	ActorUserID        uuid.UUID            `json:"actor_user_id,omitempty"`
	TeamID             *uuid.UUID           `json:"team_id"`
	Name               string               `json:"name"`
	Description        string               `json:"description"`
	Goal               string               `json:"goal"`
	HumanOwnerUserID   uuid.UUID            `json:"human_owner_user_id"`
	LeaderUserID       *uuid.UUID           `json:"leader_user_id"`
	AcceptanceUserID   *uuid.UUID           `json:"acceptance_user_id"`
	Members            []ProjectMemberInput `json:"members"`
	CoordinationPolicy map[string]any       `json:"coordination_policy"`
	ApprovalPolicy     map[string]any       `json:"approval_policy"`
	EvidencePolicy     map[string]any       `json:"evidence_policy"`
}

type updateProjectBody struct {
	TenantID           uuid.UUID             `json:"tenant_id,omitempty"`
	ActorUserID        uuid.UUID             `json:"actor_user_id,omitempty"`
	ProjectID          uuid.UUID             `json:"project_id,omitempty"`
	Name               string                `json:"name"`
	Description        string                `json:"description"`
	Goal               string                `json:"goal"`
	HumanOwnerUserID   uuid.UUID             `json:"human_owner_user_id"`
	LeaderUserID       *uuid.UUID            `json:"leader_user_id"`
	AcceptanceUserID   *uuid.UUID            `json:"acceptance_user_id"`
	Members            *[]ProjectMemberInput `json:"members"`
	CoordinationPolicy map[string]any        `json:"coordination_policy"`
	ApprovalPolicy     map[string]any        `json:"approval_policy"`
	EvidencePolicy     map[string]any        `json:"evidence_policy"`
}

type submitDemandBody struct {
	TenantID          uuid.UUID        `json:"tenant_id,omitempty"`
	ProjectID         uuid.UUID        `json:"project_id,omitempty"`
	SubmittedByUserID uuid.UUID        `json:"submitted_by_user_id,omitempty"`
	Title             string           `json:"title"`
	Content           string           `json:"content"`
	SourceType        DemandSourceType `json:"source_type"`
	SourceRefs        map[string]any   `json:"source_refs"`
	Attachments       []any            `json:"attachments"`
}

type resolveDecisionBody struct {
	Decision string         `json:"decision"`
	Comment  string         `json:"comment"`
	Payload  map[string]any `json:"payload"`
}

type completeProjectTaskBody struct {
	DigitalEmployeeID     uuid.UUID      `json:"digital_employee_id"`
	Conclusion            string         `json:"conclusion"`
	EvidenceRefs          []any          `json:"evidence_refs"`
	ArtifactRefs          []any          `json:"artifact_refs"`
	ConfidenceFactors     map[string]any `json:"confidence_factors"`
	Uncertainty           string         `json:"uncertainty"`
	MissingInformation    []any          `json:"missing_information"`
	RecommendedNextAction string         `json:"recommended_next_action"`
	RequiresHumanReview   bool           `json:"requires_human_review"`
}

type failProjectTaskBody struct {
	DigitalEmployeeID uuid.UUID `json:"digital_employee_id"`
	FailureSummary    string    `json:"failure_summary"`
}

type requestProjectTaskTransferBody struct {
	DigitalEmployeeID           uuid.UUID   `json:"digital_employee_id"`
	Reason                      string      `json:"reason"`
	SuggestedEmployeeType       string      `json:"suggested_employee_type"`
	SuggestedDigitalEmployeeIDs []uuid.UUID `json:"suggested_digital_employee_ids"`
	MissingContextRefs          []any       `json:"missing_context_refs"`
}

type projectResponse struct {
	ID                     string         `json:"id"`
	TenantID               string         `json:"tenant_id"`
	TeamID                 *string        `json:"team_id,omitempty"`
	Name                   string         `json:"name"`
	Description            *string        `json:"description,omitempty"`
	Goal                   string         `json:"goal"`
	Status                 ProjectStatus  `json:"status"`
	HumanOwnerUserID       string         `json:"human_owner_user_id"`
	LeaderUserID           *string        `json:"leader_user_id,omitempty"`
	AcceptanceUserID       *string        `json:"acceptance_user_id,omitempty"`
	CoordinationWorkflowID string         `json:"coordination_workflow_id"`
	CoordinationStatus     string         `json:"coordination_status"`
	CoordinationPolicy     map[string]any `json:"coordination_policy"`
	ApprovalPolicy         map[string]any `json:"approval_policy"`
	EvidencePolicy         map[string]any `json:"evidence_policy"`
	ArchivedAt             *string        `json:"archived_at,omitempty"`
	CreatedAt              string         `json:"created_at,omitempty"`
	UpdatedAt              string         `json:"updated_at,omitempty"`
}

type createProjectResponse struct {
	Project projectResponse         `json:"project"`
	Members []projectMemberResponse `json:"members"`
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
	ID                        string  `json:"id"`
	TenantID                  string  `json:"tenant_id"`
	ProjectID                 string  `json:"project_id"`
	DemandID                  *string `json:"demand_id,omitempty"`
	Title                     string  `json:"title"`
	Summary                   *string `json:"summary,omitempty"`
	Status                    string  `json:"status"`
	AssignedDigitalEmployeeID *string `json:"assigned_digital_employee_id,omitempty"`
	RiskLevel                 *string `json:"risk_level,omitempty"`
	RequiresHumanApproval     bool    `json:"requires_human_approval"`
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
	ID                string              `json:"id"`
	TenantID          string              `json:"tenant_id"`
	ProjectID         string              `json:"project_id"`
	SubmittedByUserID string              `json:"submitted_by_user_id"`
	Title             string              `json:"title"`
	Content           *string             `json:"content,omitempty"`
	SourceType        DemandSourceType    `json:"source_type"`
	SourceRefs        map[string]any      `json:"source_refs"`
	Attachments       []any               `json:"attachments"`
	Status            ProjectDemandStatus `json:"status"`
	CreatedEventID    *string             `json:"created_event_id,omitempty"`
}

type projectConfigRevisionResponse struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	ProjectID       string         `json:"project_id"`
	RevisionNumber  int32          `json:"revision_number"`
	ConfigSnapshot  map[string]any `json:"config_snapshot"`
	ChangeSummary   *string        `json:"change_summary,omitempty"`
	CreatedByUserID string         `json:"created_by_user_id"`
	CreatedEventID  *string        `json:"created_event_id,omitempty"`
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
		LeaderUserID:           stringPtr(project.LeaderUserID),
		AcceptanceUserID:       stringPtr(project.AcceptanceUserID),
		CoordinationWorkflowID: project.CoordinationWorkflowID,
		CoordinationStatus:     project.CoordinationStatus,
		CoordinationPolicy:     mapOrEmpty(project.CoordinationPolicy),
		ApprovalPolicy:         mapOrEmpty(project.ApprovalPolicy),
		EvidencePolicy:         mapOrEmpty(project.EvidencePolicy),
		ArchivedAt:             timePtr(project.ArchivedAt),
		CreatedAt:              timeValue(project.CreatedAt),
		UpdatedAt:              timeValue(project.UpdatedAt),
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
	}
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
		ID:                demand.ID.String(),
		TenantID:          demand.TenantID.String(),
		ProjectID:         demand.ProjectID.String(),
		SubmittedByUserID: demand.SubmittedByUserID.String(),
		Title:             demand.Title,
		Content:           demand.Content,
		SourceType:        demand.SourceType,
		SourceRefs:        mapOrEmpty(demand.SourceRefs),
		Attachments:       sliceOrEmpty(demand.Attachments),
		Status:            demand.Status,
		CreatedEventID:    stringPtr(demand.CreatedEventID),
	}
}

func configRevisionResponseFromDomain(revision ProjectConfigRevision) projectConfigRevisionResponse {
	return projectConfigRevisionResponse{
		ID:              revision.ID.String(),
		TenantID:        revision.TenantID.String(),
		ProjectID:       revision.ProjectID.String(),
		RevisionNumber:  revision.RevisionNumber,
		ConfigSnapshot:  mapOrEmpty(revision.ConfigSnapshot),
		ChangeSummary:   revision.ChangeSummary,
		CreatedByUserID: revision.CreatedByUserID.String(),
		CreatedEventID:  stringPtr(revision.CreatedEventID),
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

func uuidStrings(values []uuid.UUID) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.String())
	}
	return result
}
