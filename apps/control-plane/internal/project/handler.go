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
	GetLatestProjectConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectConfigRevision, error)
	SubmitDemand(ctx context.Context, req SubmitProjectDemandRequest) (*ProjectDemand, error)
	ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error)
	GetOverview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectOverview, error)
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

func (h *HTTPHandler) GetProjectConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	revision, err := service.GetLatestProjectConfigRevision(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configRevisionResponseFromDomain(*revision))
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
		responses = append(responses, projectTaskResponse{
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
		})
	}
	return responses
}

func eventResponses(events []ProjectEvent) []projectEventResponse {
	responses := make([]projectEventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, projectEventResponse{
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
		})
	}
	return responses
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
