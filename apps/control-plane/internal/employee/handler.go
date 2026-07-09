package employee

import (
	"context"
	"encoding/json"
	"errors"
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
	GetCreateOptions(ctx context.Context, req CreateOptionsRequest) (*CreateOptions, error)
	CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error)
	ListDigitalEmployees(ctx context.Context, req ListDigitalEmployeesRequest) ([]*DigitalEmployee, error)
	GetOverview(ctx context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error)
	ListWorkspaceFiles(ctx context.Context, req ListWorkspaceFilesRequest) ([]WorkspaceFile, error)
	UpsertWorkspaceFile(ctx context.Context, req UpsertWorkspaceFileRequest) (WorkspaceFile, error)
	ListEnvironmentVariables(ctx context.Context, req ListEnvironmentVariablesRequest) ([]EnvironmentVariableSummary, error)
	UpsertEnvironmentVariable(ctx context.Context, req UpsertEnvironmentVariableRequest) (EnvironmentVariableSummary, error)
	DeleteEnvironmentVariable(ctx context.Context, req DeleteEnvironmentVariableRequest) error
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployee, error)
	DeleteDigitalEmployee(ctx context.Context, req DeleteDigitalEmployeeRequest) error
	UpdateStatus(ctx context.Context, req UpdateStatusRequest) (*DigitalEmployee, error)
	GetExecutionInstance(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeExecutionInstance, error)
	BindExecutionInstance(ctx context.Context, req BindExecutionInstanceRequest) (*DigitalEmployeeExecutionInstance, error)
	CreateConfigRevision(ctx context.Context, req CreateDigitalEmployeeConfigRevisionRequest) (*DigitalEmployeeConfigRevision, error)
	GetSchedulingReadiness(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeSchedulingReadiness, error)
	ListEmployeeTemplates(ctx context.Context, tenantID uuid.UUID) ([]EmployeeTemplateRecord, error)
	GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error)
	CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error)
	UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error)
	SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error)
	DeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error
}

type HTTPHandler struct {
	service    HandlerService
	runService RunHandlerService
	authorizer authz.Authorizer
}

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func NewHandlerWithRunService(service HandlerService, runService RunHandlerService) *HTTPHandler {
	return &HTTPHandler{service: service, runService: runService}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *HTTPHandler) SetRunService(runService RunHandlerService) {
	h.runService = runService
}

func (h *HTTPHandler) ListDigitalEmployees(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, nil, "digital employee read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	status := DigitalEmployeeStatus(r.URL.Query().Get("status"))
	assignment := r.URL.Query().Get("assignment")
	var teamID *uuid.UUID
	if rawTeamID := r.URL.Query().Get("team_id"); rawTeamID != "" {
		parsedTeamID, err := uuid.Parse(rawTeamID)
		if err != nil {
			http.Error(w, "invalid team_id", http.StatusBadRequest)
			return
		}
		teamID = &parsedTeamID
	}

	if assignment == "unassigned" && teamID != nil {
		http.Error(w, "cannot specify both team_id and assignment=unassigned", http.StatusBadRequest)
		return
	}

	employees, err := service.ListDigitalEmployees(r.Context(), ListDigitalEmployeesRequest{
		TenantID:   tenantID,
		TeamID:     teamID,
		Status:     status,
		Assignment: assignment,
		Offset:     int32(offset),
		Limit:      int32(limit),
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeResponses(employees))
}

func (h *HTTPHandler) ListDigitalEmployeeAvatarAssets(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, nil, "digital employee avatar assets read"); !ok {
		return
	}
	writeJSON(w, http.StatusOK, ListDigitalEmployeeAvatarAssets())
}

func (h *HTTPHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, nil, "digital employee overview read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	req, parseErr := overviewRequestFromQuery(tenantID, r)
	if parseErr != "" {
		http.Error(w, parseErr, http.StatusBadRequest)
		return
	}
	overview, err := service.GetOverview(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overviewResponseFromDomain(overview))
}

func (h *HTTPHandler) GetCreateOptions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "digital employee create options")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	rawTeamID := r.URL.Query().Get("team_id")
	var teamID *uuid.UUID
	if rawTeamID != "" {
		parsedTeamID, err := uuid.Parse(rawTeamID)
		if err != nil || parsedTeamID == uuid.Nil {
			http.Error(w, "invalid team_id", http.StatusBadRequest)
			return
		}
		teamID = &parsedTeamID
	}
	options, err := service.GetCreateOptions(r.Context(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, createOptionsResponseFromDomain(options))
}

func (h *HTTPHandler) CreateDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "digital employee create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		TeamID                *uuid.UUID     `json:"team_id"`
		EmployeeType          string         `json:"employee_type"`
		Name                  string         `json:"name"`
		AvatarAssetID         string         `json:"avatar_asset_id"`
		Role                  string         `json:"role"`
		Description           *string        `json:"description"`
		PermissionPolicy      map[string]any `json:"permission_policy"`
		ContextPolicy         map[string]any `json:"context_policy"`
		ApprovalPolicy        map[string]any `json:"approval_policy"`
		RiskLevel             string         `json:"risk_level"`
		Metadata              map[string]any `json:"metadata"`
		PersonaMemoryMarkdown string         `json:"persona_memory_markdown"`
		CapabilityBindings    map[string]any `json:"capability_bindings"`
		BudgetPolicy          map[string]any `json:"budget_policy"`
		RuntimeNodeID         uuid.UUID      `json:"runtime_node_id"`
		ProviderType          string         `json:"provider_type"`
		SessionPolicy         map[string]any `json:"session_policy"`
		WorkspacePolicy       map[string]any `json:"workspace_policy"`
		EnvironmentVariables  []struct {
			Name      string `json:"name"`
			Value     string `json:"value"`
			Sensitive *bool  `json:"sensitive"`
		} `json:"environment_variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	environmentVariables := make([]InitialEnvironmentVariable, 0, len(req.EnvironmentVariables))
	for _, item := range req.EnvironmentVariables {
		environmentVariables = append(environmentVariables, InitialEnvironmentVariable{
			Name:      item.Name,
			Value:     item.Value,
			Sensitive: sensitiveOrDefault(item.Sensitive),
		})
	}
	employee, err := service.CreateDigitalEmployee(r.Context(), CreateDigitalEmployeeRequest{
		TenantID:              tenantID,
		TeamID:                req.TeamID,
		OwnerUserID:           middleware.GetUserID(r.Context()),
		EmployeeType:          req.EmployeeType,
		Name:                  req.Name,
		AvatarAssetID:         req.AvatarAssetID,
		Role:                  req.Role,
		Description:           req.Description,
		PermissionPolicy:      req.PermissionPolicy,
		ContextPolicy:         req.ContextPolicy,
		ApprovalPolicy:        req.ApprovalPolicy,
		RiskLevel:             req.RiskLevel,
		Metadata:              req.Metadata,
		PersonaMemoryMarkdown: req.PersonaMemoryMarkdown,
		CapabilityBindings:    req.CapabilityBindings,
		BudgetPolicy:          req.BudgetPolicy,
		RuntimeNodeID:         req.RuntimeNodeID,
		ProviderType:          req.ProviderType,
		SessionPolicy:         req.SessionPolicy,
		WorkspacePolicy:       req.WorkspacePolicy,
		EnvironmentVariables:  environmentVariables,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, employeeResponseFromDomain(employee))
}

func (h *HTTPHandler) GetDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	employee, err := service.GetDigitalEmployee(r.Context(), tenantID, employeeID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	response := employeeResponseFromDomain(employee)
	response.AllowedActions = h.allowedEmployeeActions(r.Context(), tenantID, employeeID)
	writeJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) DeleteDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeDelete, &employeeID, "digital employee delete")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	actorUserID := middleware.GetUserID(r.Context())
	if err := service.DeleteDigitalEmployee(r.Context(), DeleteDigitalEmployeeRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, ActorUserID: actorUserID}); err != nil {
		writeDeleteDigitalEmployeeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) GetSchedulingReadiness(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee scheduling readiness read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	readiness, err := service.GetSchedulingReadiness(r.Context(), tenantID, employeeID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	if readiness == nil {
		http.Error(w, "digital employee scheduling readiness unavailable", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, schedulingReadinessResponseFromDomain(readiness))
}

func (h *HTTPHandler) ListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee workspace files read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	files, err := service.ListWorkspaceFiles(r.Context(), ListWorkspaceFilesRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspaceFileResponses(files))
}

func (h *HTTPHandler) UpsertWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeConfigCreate, &employeeID, "digital employee workspace file upsert")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Path       string  `json:"path"`
		Content    string  `json:"content"`
		FileRole   string  `json:"file_role"`
		MimeType   string  `json:"mime_type"`
		SyncPolicy string  `json:"sync_policy"`
		ChangeNote *string `json:"change_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updatedBy := middleware.GetUserID(r.Context())
	file, err := service.UpsertWorkspaceFile(r.Context(), UpsertWorkspaceFileRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Path:              req.Path,
		Content:           req.Content,
		FileRole:          req.FileRole,
		MimeType:          req.MimeType,
		SyncPolicy:        req.SyncPolicy,
		ChangeNote:        req.ChangeNote,
		UpdatedBy:         &updatedBy,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspaceFileResponseFromDomain(file))
}

func (h *HTTPHandler) ListEnvironmentVariables(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee environment variables read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	vars, err := service.ListEnvironmentVariables(r.Context(), ListEnvironmentVariablesRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, environmentVariableSummaryResponses(vars))
}

func (h *HTTPHandler) UpsertEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeConfigCreate, &employeeID, "digital employee environment variable upsert")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Value     string `json:"value"`
		Sensitive *bool  `json:"sensitive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updatedBy := middleware.GetUserID(r.Context())
	summary, err := service.UpsertEnvironmentVariable(r.Context(), UpsertEnvironmentVariableRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Name:              chi.URLParam(r, "envName"),
		Value:             req.Value,
		Sensitive:         sensitiveOrDefault(req.Sensitive),
		ActorUserID:       &updatedBy,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, environmentVariableSummaryResponseFromDomain(summary))
}

func sensitiveOrDefault(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func (h *HTTPHandler) DeleteEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeConfigCreate, &employeeID, "digital employee environment variable delete")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.DeleteEnvironmentVariable(r.Context(), DeleteEnvironmentVariableRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Name:              chi.URLParam(r, "envName"),
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) UpdateDigitalEmployeeStatus(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeStatusUpdate, &employeeID, "digital employee status update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Status DigitalEmployeeStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	employee, err := service.UpdateStatus(r.Context(), UpdateStatusRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Status:            req.Status,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeResponseFromDomain(employee))
}

func (h *HTTPHandler) GetDigitalEmployeeExecutionInstance(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee execution instance read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	instance, err := service.GetExecutionInstance(r.Context(), tenantID, employeeID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, executionInstanceResponseFromDomain(instance))
}

func (h *HTTPHandler) UpsertDigitalEmployeeExecutionInstance(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeExecutionBind, &employeeID, "digital employee execution instance bind")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		RuntimeNodeID        uuid.UUID      `json:"runtime_node_id"`
		ProviderType         string         `json:"provider_type"`
		AgentHomeDir         string         `json:"agent_home_dir"`
		WorkspacePolicy      map[string]any `json:"workspace_policy"`
		SessionPolicy        map[string]any `json:"session_policy"`
		RuntimeSelector      map[string]any `json:"runtime_selector"`
		CapacityRequirements map[string]any `json:"capacity_requirements"`
		FallbackPolicy       map[string]any `json:"fallback_policy"`
		Metadata             map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	instance, err := service.BindExecutionInstance(r.Context(), BindExecutionInstanceRequest{
		TenantID:             tenantID,
		DigitalEmployeeID:    employeeID,
		RuntimeNodeID:        req.RuntimeNodeID,
		ProviderType:         req.ProviderType,
		AgentHomeDir:         req.AgentHomeDir,
		WorkspacePolicy:      req.WorkspacePolicy,
		SessionPolicy:        req.SessionPolicy,
		RuntimeSelector:      req.RuntimeSelector,
		CapacityRequirements: req.CapacityRequirements,
		FallbackPolicy:       req.FallbackPolicy,
		Metadata:             req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, executionInstanceResponseFromDomain(instance))
}

func (h *HTTPHandler) CreateDigitalEmployeeConfigRevision(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeConfigCreate, &employeeID, "digital employee config revision create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, field := range []string{
		"role_profile",
		"constitution_addendum",
		"capability_selection",
		"context_policy_override",
		"approval_policy_override",
		"output_contract_addendum",
	} {
		if _, exists := raw[field]; exists {
			http.Error(w, field+" is no longer supported", http.StatusBadRequest)
			return
		}
	}
	var req struct {
		PersonaMemoryMarkdown *string              `json:"persona_memory_markdown"`
		CapabilityBindings    map[string]any       `json:"capability_bindings"`
		BudgetPolicy          map[string]any       `json:"budget_policy"`
		Status                ConfigRevisionStatus `json:"status"`
	}
	payload, _ := json.Marshal(raw)
	if err := json.Unmarshal(payload, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	revision, err := service.CreateConfigRevision(r.Context(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:              tenantID,
		DigitalEmployeeID:     employeeID,
		PersonaMemoryMarkdown: req.PersonaMemoryMarkdown,
		CapabilityBindings:    req.CapabilityBindings,
		BudgetPolicy:          req.BudgetPolicy,
		Status:                req.Status,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, configRevisionResponseFromDomain(revision))
}

const (
	defaultOverviewPageLimit = 50
	maxOverviewPageLimit     = 100
)

func overviewRequestFromQuery(tenantID uuid.UUID, r *http.Request) (GetDigitalEmployeeOverviewRequest, string) {
	query := r.URL.Query()
	req := GetDigitalEmployeeOverviewRequest{
		TenantID:        tenantID,
		Query:           strings.TrimSpace(query.Get("q")),
		Status:          DigitalEmployeeStatus(strings.TrimSpace(query.Get("status"))),
		EmployeeType:    strings.TrimSpace(query.Get("employee_type")),
		ProviderType:    strings.TrimSpace(query.Get("provider_type")),
		RiskLevel:       strings.TrimSpace(query.Get("risk_level")),
		ExecutionStatus: OverviewExecutionStatus(strings.TrimSpace(query.Get("execution_status"))),
		RunStatus:       OverviewRunStatus(strings.TrimSpace(query.Get("run_status"))),
		Limit:           defaultOverviewPageLimit,
	}
	if req.Status != "" && !req.Status.IsValid() {
		return GetDigitalEmployeeOverviewRequest{}, "invalid status"
	}
	if !req.ExecutionStatus.IsValid() {
		return GetDigitalEmployeeOverviewRequest{}, "invalid execution_status"
	}
	if !req.RunStatus.IsValid() {
		return GetDigitalEmployeeOverviewRequest{}, "invalid run_status"
	}
	if rawTeamID := strings.TrimSpace(query.Get("team_id")); rawTeamID != "" {
		teamID, err := uuid.Parse(rawTeamID)
		if err != nil || teamID == uuid.Nil {
			return GetDigitalEmployeeOverviewRequest{}, "invalid team_id"
		}
		req.TeamID = &teamID
	}
	if rawRuntimeNodeID := strings.TrimSpace(query.Get("runtime_node_id")); rawRuntimeNodeID != "" {
		runtimeNodeID, err := uuid.Parse(rawRuntimeNodeID)
		if err != nil || runtimeNodeID == uuid.Nil {
			return GetDigitalEmployeeOverviewRequest{}, "invalid runtime_node_id"
		}
		req.RuntimeNodeID = &runtimeNodeID
	}
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		limit, err := strconv.ParseInt(rawLimit, 10, 32)
		if err != nil {
			return GetDigitalEmployeeOverviewRequest{}, "limit must be an integer"
		}
		if limit <= 0 {
			return GetDigitalEmployeeOverviewRequest{}, "limit must be greater than 0"
		}
		if limit > maxOverviewPageLimit {
			limit = maxOverviewPageLimit
		}
		req.Limit = int32(limit)
	}
	if rawOffset := strings.TrimSpace(query.Get("offset")); rawOffset != "" {
		offset, err := strconv.ParseInt(rawOffset, 10, 32)
		if err != nil {
			return GetDigitalEmployeeOverviewRequest{}, "offset must be an integer"
		}
		if offset < 0 {
			return GetDigitalEmployeeOverviewRequest{}, "offset must be greater than or equal to 0"
		}
		req.Offset = int32(offset)
	}
	return req, ""
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "employee service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func (h *HTTPHandler) authorizeDigitalEmployeeManagement(w http.ResponseWriter, r *http.Request, action string, employeeID *uuid.UUID, auditReason string) (uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "digital employee authorization is not configured", http.StatusForbidden)
		return uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, false
	}
	resource := authz.ResourceRef{
		Type: authz.ResourceTenant,
		ID:   tenantID.String(),
	}
	if employeeID != nil {
		resource = authz.ResourceRef{
			Type: authz.ResourceEmployee,
			ID:   employeeID.String(),
		}
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   userID.String(),
		},
		Action:      action,
		Resource:    resource,
		TenantID:    tenantID,
		AuditReason: auditReason,
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, false
	}
	return tenantID, true
}

func (h *HTTPHandler) allowedEmployeeActions(ctx context.Context, tenantID, employeeID uuid.UUID) []string {
	if h == nil || h.authorizer == nil {
		return nil
	}
	userID := middleware.GetUserID(ctx)
	if userID == uuid.Nil {
		return nil
	}
	actions := []string{authz.ActionEmployeeDelete}
	allowed := make([]string, 0, len(actions))
	for _, action := range actions {
		decision, err := h.authorizer.Check(ctx, authz.CheckRequest{
			Actor:    authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
			Action:   action,
			Resource: authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()},
			TenantID: tenantID,
		})
		if err == nil && decision.Allowed {
			allowed = append(allowed, action)
		}
	}
	return allowed
}

type digitalEmployeeResponse struct {
	ID               string                `json:"id"`
	TenantID         string                `json:"tenant_id"`
	TeamID           *string               `json:"team_id,omitempty"`
	OwnerUserID      string                `json:"owner_user_id"`
	EmployeeType     string                `json:"employee_type"`
	ProviderType     string                `json:"provider_type"`
	Name             string                `json:"name"`
	Role             string                `json:"role"`
	Description      *string               `json:"description,omitempty"`
	Status           DigitalEmployeeStatus `json:"status"`
	PermissionPolicy map[string]any        `json:"permission_policy"`
	ContextPolicy    map[string]any        `json:"context_policy"`
	ApprovalPolicy   map[string]any        `json:"approval_policy"`
	RiskLevel        string                `json:"risk_level"`
	Metadata         map[string]any        `json:"metadata"`
	DisabledAt       *string               `json:"disabled_at,omitempty"`
	ArchivedAt       *string               `json:"archived_at,omitempty"`
	AllowedActions   []string              `json:"allowed_actions,omitempty"`
	CreatedAt        string                `json:"created_at,omitempty"`
	UpdatedAt        string                `json:"updated_at,omitempty"`
}

type digitalEmployeeOverviewResponse struct {
	Summary      digitalEmployeeOverviewSummaryResponse      `json:"summary"`
	QueueSummary digitalEmployeeOverviewQueueSummaryResponse `json:"queue_summary"`
	Items        []digitalEmployeeOverviewItemResponse       `json:"items"`
	Filters      digitalEmployeeOverviewFiltersResponse      `json:"filters"`
	Pagination   overviewPaginationResponse                  `json:"pagination"`
}

type digitalEmployeeOverviewSummaryResponse struct {
	TotalCount                 int32            `json:"total_count"`
	RunnableCount              int32            `json:"runnable_count"`
	RunningCount               int32            `json:"running_count"`
	WaitingRuntimeCount        int32            `json:"waiting_runtime_count"`
	ErrorCount                 int32            `json:"error_count"`
	HighRiskCount              int32            `json:"high_risk_count"`
	ReadyCount                 int32            `json:"ready_count"`
	PendingRuntimeBindingCount int32            `json:"pending_runtime_binding_count"`
	PendingConfigApprovalCount int32            `json:"pending_config_approval_count"`
	FailedRecentRunCount       int32            `json:"failed_recent_run_count"`
	OperationalStatusCounts    map[string]int32 `json:"operational_status_counts"`
}

type digitalEmployeeOverviewQueueSummaryResponse struct {
	PendingRuntimeBindingCount int32 `json:"pending_runtime_binding_count"`
	StaleConfigCount           int32 `json:"stale_config_count"`
	FailedRecentRunCount       int32 `json:"failed_recent_run_count"`
}

type digitalEmployeeOverviewItemResponse struct {
	IdentitySummary   digitalEmployeeIdentitySummaryResponse      `json:"identity_summary"`
	ExecutionSummary  digitalEmployeeExecutionSummaryResponse     `json:"execution_summary"`
	LatestRunSummary  *digitalEmployeeLatestRunSummaryResponse    `json:"latest_run_summary"`
	GovernanceSummary digitalEmployeeGovernanceSummaryResponse    `json:"governance_summary"`
	BudgetSummary     digitalEmployeeBudgetSummaryResponse        `json:"budget_summary"`
	WorkbenchStatus   WorkbenchStatus                             `json:"workbench_status"`
	OperationalState  digitalEmployeeOperationalStateResponse     `json:"operational_state"`
	RecentEvents      []digitalEmployeeRecentEventSummaryResponse `json:"recent_events"`
}

type digitalEmployeeOperationalReasonResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type digitalEmployeeOperationalStateResponse struct {
	Status      string                                     `json:"status"`
	Reasons     []digitalEmployeeOperationalReasonResponse `json:"reasons"`
	CanDispatch bool                                       `json:"can_dispatch"`
}

type digitalEmployeeIdentitySummaryResponse struct {
	ID                string                      `json:"id"`
	TenantID          string                      `json:"tenant_id"`
	TeamID            *string                     `json:"team_id,omitempty"`
	TeamName          string                      `json:"team_name"`
	OwnerUserID       string                      `json:"owner_user_id"`
	OwnerDisplayName  string                      `json:"owner_display_name"`
	EmployeeType      string                      `json:"employee_type"`
	EmployeeTypeLabel string                      `json:"employee_type_label"`
	Name              string                      `json:"name"`
	Role              string                      `json:"role"`
	Description       *string                     `json:"description,omitempty"`
	Status            DigitalEmployeeStatus       `json:"status"`
	RiskLevel         string                      `json:"risk_level"`
	AvatarAsset       *DigitalEmployeeAvatarAsset `json:"avatar_asset,omitempty"`
}

type digitalEmployeeExecutionSummaryResponse struct {
	ExecutionInstanceID   *string                 `json:"execution_instance_id,omitempty"`
	Status                OverviewExecutionStatus `json:"status"`
	RuntimeNodeID         *string                 `json:"runtime_node_id,omitempty"`
	NodeID                string                  `json:"node_id"`
	RuntimeName           string                  `json:"runtime_name"`
	RuntimeStatus         string                  `json:"runtime_status"`
	ProviderType          string                  `json:"provider_type"`
	ProviderStatus        string                  `json:"provider_status"`
	HealthStatus          string                  `json:"health_status"`
	AgentHomeDirAvailable bool                    `json:"agent_home_dir_available"`
}

type digitalEmployeeLatestRunSummaryResponse struct {
	RunID        string            `json:"run_id"`
	TaskID       string            `json:"task_id"`
	Status       OverviewRunStatus `json:"status"`
	Title        string            `json:"title"`
	StartedAt    *string           `json:"started_at,omitempty"`
	UpdatedAt    *string           `json:"updated_at,omitempty"`
	FinishedAt   *string           `json:"finished_at,omitempty"`
	DurationSec  *int32            `json:"duration_sec,omitempty"`
	TokenUsage   *int32            `json:"token_usage,omitempty"`
	ErrorMessage string            `json:"error_message"`
}

type digitalEmployeeGovernanceSummaryResponse struct {
	EffectiveConfigID      *string `json:"effective_config_id,omitempty"`
	Status                 string  `json:"status"`
	TeamRevisionNumber     *int32  `json:"team_revision_number,omitempty"`
	EmployeeRevisionNumber *int32  `json:"employee_revision_number,omitempty"`
	SkillsCount            int32   `json:"skills_count"`
	MCPServersCount        int32   `json:"mcp_servers_count"`
	ConstitutionRef        string  `json:"constitution_ref"`
}

type digitalEmployeeBudgetSummaryResponse struct {
	DailyTokenLimit   *int32   `json:"daily_token_limit,omitempty"`
	UsageTokensToday  int32    `json:"usage_tokens_today"`
	UsagePercentToday *int32   `json:"usage_percent_today,omitempty"`
	LimitExceeded     bool     `json:"limit_exceeded"`
	UsageTokens30d    *int32   `json:"usage_tokens_30d,omitempty"`
	RunCount30d       int32    `json:"run_count_30d"`
	CostAmount30d     *float64 `json:"cost_amount_30d,omitempty"`
	Currency          string   `json:"currency"`
	Source            string   `json:"source"`
}

type digitalEmployeeRecentEventSummaryResponse struct {
	Label      string  `json:"label"`
	Status     string  `json:"status"`
	OccurredAt *string `json:"occurred_at,omitempty"`
}

type digitalEmployeeOverviewFiltersResponse struct {
	Teams             []overviewFilterOptionResponse `json:"teams"`
	Statuses          []overviewFilterOptionResponse `json:"statuses"`
	EmployeeTypes     []overviewFilterOptionResponse `json:"employee_types"`
	Providers         []overviewFilterOptionResponse `json:"providers"`
	RuntimeNodes      []overviewFilterOptionResponse `json:"runtime_nodes"`
	RiskLevels        []overviewFilterOptionResponse `json:"risk_levels"`
	ExecutionStatuses []overviewFilterOptionResponse `json:"execution_statuses"`
	RunStatuses       []overviewFilterOptionResponse `json:"run_statuses"`
}

type overviewFilterOptionResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type overviewPaginationResponse struct {
	Limit      int32 `json:"limit"`
	Offset     int32 `json:"offset"`
	TotalCount int32 `json:"total_count"`
}

type workspaceFileResponse struct {
	ID                string  `json:"id"`
	TeamID            *string `json:"team_id,omitempty"`
	Path              string  `json:"path"`
	FileRole          string  `json:"file_role"`
	MimeType          string  `json:"mime_type"`
	SyncPolicy        string  `json:"sync_policy"`
	Status            string  `json:"status"`
	CurrentRevisionID string  `json:"current_revision_id"`
	RevisionNumber    int32   `json:"revision_number"`
	Content           string  `json:"content"`
	ContentHash       string  `json:"content_hash"`
	SizeBytes         int32   `json:"size_bytes"`
	StorageBackend    string  `json:"storage_backend"`
	ObjectKey         *string `json:"object_key,omitempty"`
	ChangeNote        *string `json:"change_note,omitempty"`
	CreatedAt         string  `json:"created_at,omitempty"`
	UpdatedAt         string  `json:"updated_at,omitempty"`
}

type environmentVariableSummaryResponse struct {
	ID                string                    `json:"id,omitempty"`
	TenantID          string                    `json:"tenant_id,omitempty"`
	TeamID            string                    `json:"team_id,omitempty"`
	DigitalEmployeeID string                    `json:"digital_employee_id,omitempty"`
	Name              string                    `json:"name"`
	Configured        bool                      `json:"configured"`
	Fingerprint       string                    `json:"fingerprint,omitempty"`
	Sensitive         bool                      `json:"sensitive"`
	Status            EnvironmentVariableStatus `json:"status"`
	UpdatedAt         string                    `json:"updated_at,omitempty"`
}

type createOptionsResponse struct {
	TeamConfig             teamConfigCreateOptionResponse  `json:"team_config"`
	EmployeeTypes          []employeeTypeOptionResponse    `json:"employee_types"`
	CapabilityOptions      capabilityOptionsResponse       `json:"capability_options"`
	RuntimeProviderOptions []runtimeProviderOptionResponse `json:"runtime_provider_options"`
	CreationChecks         []createOptionCheckResponse     `json:"creation_checks"`
	PolicyDefaults         policyDefaultsResponse          `json:"policy_defaults"`
}

type createOptionCheckResponse struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type teamConfigCreateOptionResponse struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	TeamID       *string        `json:"team_id,omitempty"`
	Constitution map[string]any `json:"constitution"`
	Skills       []string       `json:"skills"`
	MCPServers   []string       `json:"mcp_servers"`
}

type employeeTypeOptionResponse struct {
	Type                         string         `json:"type"`
	Label                        string         `json:"label"`
	Description                  string         `json:"description"`
	DefaultRole                  string         `json:"default_role"`
	RecommendedSkills            []string       `json:"recommended_skills"`
	RecommendedMCPServers        []string       `json:"recommended_mcp_servers"`
	RecommendedProviderTypes     []string       `json:"recommended_provider_types"`
	DefaultCapabilitySelection   map[string]any `json:"default_capability_selection"`
	DefaultContextPolicyOverride map[string]any `json:"default_context_policy_override"`
	DefaultApprovalPolicy        map[string]any `json:"default_approval_policy"`
	Metadata                     map[string]any `json:"metadata"`
}

type capabilityOptionsResponse struct {
	ProviderTypes []string `json:"provider_types"`
	Skills        []string `json:"skills"`
	MCPServers    []string `json:"mcp_servers"`
}

type runtimeProviderOptionResponse struct {
	RuntimeNodeID         string `json:"runtime_node_id"`
	NodeID                string `json:"node_id"`
	RuntimeName           string `json:"runtime_name"`
	ProviderType          string `json:"provider_type"`
	RuntimeStatus         string `json:"runtime_status"`
	ProviderStatus        string `json:"provider_status"`
	HealthStatus          string `json:"health_status"`
	CurrentLoad           int32  `json:"current_load"`
	MaxSlots              int32  `json:"max_slots"`
	AgentHomeDir          string `json:"agent_home_dir"`
	AgentHomeDirAvailable bool   `json:"agent_home_dir_available"`
	Available             bool   `json:"available"`
	DisabledReason        string `json:"disabled_reason,omitempty"`
}

type policyDefaultsResponse struct {
	PermissionPolicy      map[string]any `json:"permission_policy"`
	ContextPolicyOverride map[string]any `json:"context_policy_override"`
	ApprovalPolicy        map[string]any `json:"approval_policy"`
	CapabilitySelection   map[string]any `json:"capability_selection"`
	RuntimeSelector       map[string]any `json:"runtime_selector"`
	WorkspacePolicy       map[string]any `json:"workspace_policy"`
	SessionPolicy         map[string]any `json:"session_policy"`
	Metadata              map[string]any `json:"metadata"`
}

type executionInstanceResponse struct {
	ID                   string                  `json:"id"`
	TenantID             string                  `json:"tenant_id"`
	DigitalEmployeeID    string                  `json:"digital_employee_id"`
	RuntimeNodeID        string                  `json:"runtime_node_id"`
	ProviderType         string                  `json:"provider_type"`
	AgentHomeDir         string                  `json:"agent_home_dir"`
	WorkspacePolicy      map[string]any          `json:"workspace_policy"`
	SessionPolicy        map[string]any          `json:"session_policy"`
	RuntimeSelector      map[string]any          `json:"runtime_selector"`
	CapacityRequirements map[string]any          `json:"capacity_requirements"`
	FallbackPolicy       map[string]any          `json:"fallback_policy"`
	Status               ExecutionInstanceStatus `json:"status"`
	ReadyAt              *string                 `json:"ready_at,omitempty"`
	DisabledAt           *string                 `json:"disabled_at,omitempty"`
	ErrorAt              *string                 `json:"error_at,omitempty"`
	ErrorMessage         *string                 `json:"error_message,omitempty"`
	Metadata             map[string]any          `json:"metadata"`
	CreatedAt            string                  `json:"created_at,omitempty"`
	UpdatedAt            string                  `json:"updated_at,omitempty"`
}

type configRevisionResponse struct {
	ID                    string               `json:"id"`
	TenantID              string               `json:"tenant_id"`
	DigitalEmployeeID     string               `json:"digital_employee_id"`
	RevisionNumber        int32                `json:"revision_number"`
	PersonaMemoryMarkdown string               `json:"persona_memory_markdown"`
	CapabilityBindings    map[string]any       `json:"capability_bindings"`
	BudgetPolicy          map[string]any       `json:"budget_policy"`
	Status                ConfigRevisionStatus `json:"status"`
	ApprovedBy            *string              `json:"approved_by,omitempty"`
	ApprovedAt            *string              `json:"approved_at,omitempty"`
	ArchivedAt            *string              `json:"archived_at,omitempty"`
	CreatedAt             string               `json:"created_at,omitempty"`
	UpdatedAt             string               `json:"updated_at,omitempty"`
}

type schedulingReadinessResponse struct {
	EmployeeID                string                                  `json:"employee_id"`
	Status                    DigitalEmployeeStatus                   `json:"status"`
	ReadyForProjectScheduling bool                                    `json:"ready_for_project_scheduling"`
	ProjectExecutionSource    string                                  `json:"project_execution_source"`
	Checks                    []schedulingReadinessCheckResponse      `json:"checks"`
	Capabilities              schedulingReadinessCapabilitiesResponse `json:"capabilities"`
}

type schedulingReadinessCheckResponse struct {
	Code    string               `json:"code"`
	Status  ReadinessCheckStatus `json:"status"`
	Label   string               `json:"label"`
	Message string               `json:"message"`
}

type schedulingReadinessCapabilitiesResponse struct {
	Skills               schedulingReadinessSkillSummaryResponse       `json:"skills"`
	MCPServers           schedulingReadinessMCPSummaryResponse         `json:"mcp_servers"`
	EnvironmentVariables schedulingReadinessEnvironmentSummaryResponse `json:"environment_variables"`
}

type schedulingReadinessSkillSummaryResponse struct {
	PersonalCount   int      `json:"personal_count"`
	InheritedCount  int      `json:"inherited_count"`
	MissingRequired []string `json:"missing_required"`
}

type schedulingReadinessMCPSummaryResponse struct {
	PersonalCount  int `json:"personal_count"`
	InheritedCount int `json:"inherited_count"`
}

type schedulingReadinessEnvironmentSummaryResponse struct {
	ConfiguredCount int      `json:"configured_count"`
	MissingNames    []string `json:"missing_names"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type digitalEmployeeDeleteBlockedResponse struct {
	Code     string                                 `json:"code"`
	Message  string                                 `json:"message"`
	Blockers []digitalEmployeeDeleteBlockerResponse `json:"blockers"`
}

type digitalEmployeeDeleteBlockerResponse struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	RunID     string `json:"run_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func employeeIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	employeeID, err := uuid.Parse(chi.URLParam(r, "employeeId"))
	if err != nil || employeeID == uuid.Nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return employeeID, true
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRuntimeUnavailable), errors.Is(err, ErrProviderUnavailable):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrConflict):
		http.Error(w, "conflict", http.StatusConflict)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeDeleteDigitalEmployeeError(w http.ResponseWriter, err error) {
	var blocked *DigitalEmployeeDeleteBlockedError
	if errors.As(err, &blocked) {
		writeJSON(w, http.StatusConflict, digitalEmployeeDeleteBlockedResponse{
			Code:     DigitalEmployeeDeleteBlockedCode,
			Message:  "该数字员工仍有排队或执行中的工作，停止或完成后再删除。",
			Blockers: deleteBlockerResponses(blocked.Blockers),
		})
		return
	}
	writeHandlerError(w, err)
}

func deleteBlockerResponses(blockers []DigitalEmployeeDeleteBlocker) []digitalEmployeeDeleteBlockerResponse {
	responses := make([]digitalEmployeeDeleteBlockerResponse, 0, len(blockers))
	for _, blocker := range blockers {
		responses = append(responses, digitalEmployeeDeleteBlockerResponse{
			Type:      string(blocker.Type),
			ID:        blocker.ID.String(),
			Status:    blocker.Status,
			Title:     blocker.Title,
			RunID:     uuidStringValue(blocker.RunID),
			ProjectID: uuidStringValue(blocker.ProjectID),
		})
	}
	return responses
}

func uuidStringValue(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}

func writeJSONError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, errorResponse{
		Code:    code,
		Message: message,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func employeeResponses(employees []*DigitalEmployee) []digitalEmployeeResponse {
	responses := make([]digitalEmployeeResponse, 0, len(employees))
	for _, employee := range employees {
		responses = append(responses, employeeResponseFromDomain(employee))
	}
	return responses
}

func employeeResponseFromDomain(employee *DigitalEmployee) digitalEmployeeResponse {
	return digitalEmployeeResponse{
		ID:               employee.ID.String(),
		TenantID:         employee.TenantID.String(),
		TeamID:           uuidStringPtr(employee.TeamID),
		OwnerUserID:      employee.OwnerUserID.String(),
		EmployeeType:     employee.EmployeeType,
		ProviderType:     employee.ProviderType,
		Name:             employee.Name,
		Role:             employee.Role,
		Description:      employee.Description,
		Status:           employee.Status,
		PermissionPolicy: cloneMap(employee.PermissionPolicy),
		ContextPolicy:    cloneMap(employee.ContextPolicy),
		ApprovalPolicy:   cloneMap(employee.ApprovalPolicy),
		RiskLevel:        employee.RiskLevel,
		Metadata:         cloneMap(employee.Metadata),
		DisabledAt:       timeStringPtr(employee.DisabledAt),
		ArchivedAt:       timeStringPtr(employee.ArchivedAt),
		CreatedAt:        timeString(employee.CreatedAt),
		UpdatedAt:        timeString(employee.UpdatedAt),
	}
}

func schedulingReadinessResponseFromDomain(readiness *DigitalEmployeeSchedulingReadiness) schedulingReadinessResponse {
	checks := make([]schedulingReadinessCheckResponse, 0, len(readiness.Checks))
	for _, check := range readiness.Checks {
		checks = append(checks, schedulingReadinessCheckResponse{
			Code:    check.Code,
			Status:  check.Status,
			Label:   check.Label,
			Message: check.Message,
		})
	}
	return schedulingReadinessResponse{
		EmployeeID:                readiness.EmployeeID.String(),
		Status:                    readiness.Status,
		ReadyForProjectScheduling: readiness.ReadyForProjectScheduling,
		ProjectExecutionSource:    readiness.ProjectExecutionSource,
		Checks:                    checks,
		Capabilities: schedulingReadinessCapabilitiesResponse{
			Skills: schedulingReadinessSkillSummaryResponse{
				PersonalCount:   readiness.Capabilities.Skills.PersonalCount,
				InheritedCount:  readiness.Capabilities.Skills.InheritedCount,
				MissingRequired: stringSliceForJSON(readiness.Capabilities.Skills.MissingRequired),
			},
			MCPServers: schedulingReadinessMCPSummaryResponse{
				PersonalCount:  readiness.Capabilities.MCPServers.PersonalCount,
				InheritedCount: readiness.Capabilities.MCPServers.InheritedCount,
			},
			EnvironmentVariables: schedulingReadinessEnvironmentSummaryResponse{
				ConfiguredCount: readiness.Capabilities.EnvironmentVariables.ConfiguredCount,
				MissingNames:    stringSliceForJSON(readiness.Capabilities.EnvironmentVariables.MissingNames),
			},
		},
	}
}

func workspaceFileResponses(files []WorkspaceFile) []workspaceFileResponse {
	responses := make([]workspaceFileResponse, 0, len(files))
	for _, file := range files {
		responses = append(responses, workspaceFileResponseFromDomain(file))
	}
	return responses
}

func workspaceFileResponseFromDomain(file WorkspaceFile) workspaceFileResponse {
	return workspaceFileResponse{
		ID:                file.ID.String(),
		TeamID:            uuidStringPtr(file.TeamID),
		Path:              file.Path,
		FileRole:          file.FileRole,
		MimeType:          file.MimeType,
		SyncPolicy:        file.SyncPolicy,
		Status:            file.Status,
		CurrentRevisionID: file.CurrentRevisionID.String(),
		RevisionNumber:    file.RevisionNumber,
		Content:           file.Content,
		ContentHash:       file.ContentHash,
		SizeBytes:         file.SizeBytes,
		StorageBackend:    file.StorageBackend,
		ObjectKey:         file.ObjectKey,
		ChangeNote:        file.ChangeNote,
		CreatedAt:         timeString(file.CreatedAt),
		UpdatedAt:         timeString(file.UpdatedAt),
	}
}

func environmentVariableSummaryResponses(vars []EnvironmentVariableSummary) []environmentVariableSummaryResponse {
	responses := make([]environmentVariableSummaryResponse, 0, len(vars))
	for _, item := range vars {
		responses = append(responses, environmentVariableSummaryResponseFromDomain(item))
	}
	return responses
}

func environmentVariableSummaryResponseFromDomain(item EnvironmentVariableSummary) environmentVariableSummaryResponse {
	return environmentVariableSummaryResponse{
		ID:                optionalUUIDString(item.ID),
		TenantID:          optionalUUIDString(item.TenantID),
		TeamID:            optionalUUIDStringPtr(item.TeamID),
		DigitalEmployeeID: optionalUUIDString(item.DigitalEmployeeID),
		Name:              item.Name,
		Configured:        item.Configured,
		Fingerprint:       item.Fingerprint,
		Sensitive:         item.Sensitive,
		Status:            item.Status,
		UpdatedAt:         timeString(item.UpdatedAt),
	}
}

func overviewResponseFromDomain(overview *DigitalEmployeeOverview) digitalEmployeeOverviewResponse {
	if overview == nil {
		return digitalEmployeeOverviewResponse{
			Summary: digitalEmployeeOverviewSummaryResponse{
				OperationalStatusCounts: operationalStatusCountsResponseFromDomain(nil),
			},
			Items:   []digitalEmployeeOverviewItemResponse{},
			Filters: overviewFiltersResponseFromDomain(DigitalEmployeeOverviewFilters{}),
		}
	}
	return digitalEmployeeOverviewResponse{
		Summary: digitalEmployeeOverviewSummaryResponse{
			TotalCount:                 overview.Summary.TotalCount,
			RunnableCount:              overview.Summary.RunnableCount,
			RunningCount:               overview.Summary.RunningCount,
			WaitingRuntimeCount:        overview.Summary.WaitingRuntimeCount,
			ErrorCount:                 overview.Summary.ErrorCount,
			HighRiskCount:              overview.Summary.HighRiskCount,
			ReadyCount:                 overview.Summary.ReadyCount,
			PendingRuntimeBindingCount: overview.Summary.PendingRuntimeBindingCount,
			PendingConfigApprovalCount: overview.Summary.PendingConfigApprovalCount,
			FailedRecentRunCount:       overview.Summary.FailedRecentRunCount,
			OperationalStatusCounts:    operationalStatusCountsResponseFromDomain(overview.Summary.OperationalStatusCounts),
		},
		QueueSummary: digitalEmployeeOverviewQueueSummaryResponse{
			PendingRuntimeBindingCount: overview.QueueSummary.PendingRuntimeBindingCount,
			StaleConfigCount:           overview.QueueSummary.StaleConfigCount,
			FailedRecentRunCount:       overview.QueueSummary.FailedRecentRunCount,
		},
		Items:      overviewItemResponses(overview.Items),
		Filters:    overviewFiltersResponseFromDomain(overview.Filters),
		Pagination: overviewPaginationResponseFromDomain(overview.Pagination),
	}
}

func overviewItemResponses(items []DigitalEmployeeOverviewItem) []digitalEmployeeOverviewItemResponse {
	responses := make([]digitalEmployeeOverviewItemResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, digitalEmployeeOverviewItemResponse{
			IdentitySummary:   identitySummaryResponseFromDomain(item.IdentitySummary),
			ExecutionSummary:  executionSummaryResponseFromDomain(item.ExecutionSummary),
			LatestRunSummary:  latestRunSummaryResponseFromDomain(item.LatestRunSummary),
			GovernanceSummary: governanceSummaryResponseFromDomain(item.GovernanceSummary),
			BudgetSummary:     budgetSummaryResponseFromDomain(item.BudgetSummary),
			WorkbenchStatus:   item.WorkbenchStatus,
			OperationalState:  operationalStateResponseFromDomain(item.OperationalState),
			RecentEvents:      recentEventSummaryResponses(item.RecentEvents),
		})
	}
	return responses
}

func operationalStatusCountsResponseFromDomain(counts map[DigitalEmployeeOperationalStatus]int32) map[string]int32 {
	response := make(map[string]int32, len(counts))
	for status, count := range counts {
		response[string(status)] = count
	}
	return response
}

func operationalStateResponseFromDomain(state DigitalEmployeeOperationalState) digitalEmployeeOperationalStateResponse {
	return digitalEmployeeOperationalStateResponse{
		Status:      string(state.Status),
		Reasons:     operationalReasonResponsesFromDomain(state.Reasons),
		CanDispatch: state.CanDispatch,
	}
}

func operationalReasonResponsesFromDomain(reasons []DigitalEmployeeOperationalReason) []digitalEmployeeOperationalReasonResponse {
	responses := make([]digitalEmployeeOperationalReasonResponse, 0, len(reasons))
	for _, reason := range reasons {
		responses = append(responses, digitalEmployeeOperationalReasonResponse{
			Code:    reason.Code,
			Message: reason.Message,
		})
	}
	return responses
}

func identitySummaryResponseFromDomain(summary DigitalEmployeeIdentitySummary) digitalEmployeeIdentitySummaryResponse {
	return digitalEmployeeIdentitySummaryResponse{
		ID:                summary.ID.String(),
		TenantID:          summary.TenantID.String(),
		TeamID:            uuidStringPtr(summary.TeamID),
		TeamName:          summary.TeamName,
		OwnerUserID:       summary.OwnerUserID.String(),
		OwnerDisplayName:  summary.OwnerDisplayName,
		EmployeeType:      summary.EmployeeType,
		EmployeeTypeLabel: summary.EmployeeTypeLabel,
		Name:              summary.Name,
		Role:              summary.Role,
		Description:       summary.Description,
		Status:            summary.Status,
		RiskLevel:         summary.RiskLevel,
		AvatarAsset:       summary.AvatarAsset,
	}
}

func executionSummaryResponseFromDomain(summary DigitalEmployeeExecutionSummary) digitalEmployeeExecutionSummaryResponse {
	return digitalEmployeeExecutionSummaryResponse{
		ExecutionInstanceID:   uuidStringPtr(summary.ExecutionInstanceID),
		Status:                summary.Status,
		RuntimeNodeID:         uuidStringPtr(summary.RuntimeNodeID),
		NodeID:                summary.NodeID,
		RuntimeName:           summary.RuntimeName,
		RuntimeStatus:         summary.RuntimeStatus,
		ProviderType:          summary.ProviderType,
		ProviderStatus:        summary.ProviderStatus,
		HealthStatus:          summary.HealthStatus,
		AgentHomeDirAvailable: summary.AgentHomeDirAvailable,
	}
}

func latestRunSummaryResponseFromDomain(summary *DigitalEmployeeLatestRunSummary) *digitalEmployeeLatestRunSummaryResponse {
	if summary == nil {
		return nil
	}
	return &digitalEmployeeLatestRunSummaryResponse{
		RunID:        summary.RunID.String(),
		TaskID:       summary.TaskID.String(),
		Status:       summary.Status,
		Title:        summary.Title,
		StartedAt:    timeStringPtr(summary.StartedAt),
		UpdatedAt:    timeStringPtr(summary.UpdatedAt),
		FinishedAt:   timeStringPtr(summary.FinishedAt),
		DurationSec:  summary.DurationSec,
		TokenUsage:   summary.TokenUsage,
		ErrorMessage: summary.ErrorMessage,
	}
}

func governanceSummaryResponseFromDomain(summary DigitalEmployeeGovernanceSummary) digitalEmployeeGovernanceSummaryResponse {
	return digitalEmployeeGovernanceSummaryResponse{
		EffectiveConfigID:      uuidStringPtr(summary.EffectiveConfigID),
		Status:                 summary.Status,
		TeamRevisionNumber:     summary.TeamRevisionNumber,
		EmployeeRevisionNumber: summary.EmployeeRevisionNumber,
		SkillsCount:            summary.SkillsCount,
		MCPServersCount:        summary.MCPServersCount,
		ConstitutionRef:        summary.ConstitutionRef,
	}
}

func budgetSummaryResponseFromDomain(summary DigitalEmployeeBudgetSummary) digitalEmployeeBudgetSummaryResponse {
	return digitalEmployeeBudgetSummaryResponse{
		DailyTokenLimit:   summary.DailyTokenLimit,
		UsageTokensToday:  summary.UsageTokensToday,
		UsagePercentToday: summary.UsagePercentToday,
		LimitExceeded:     summary.LimitExceeded,
		UsageTokens30d:    summary.UsageTokens30d,
		RunCount30d:       summary.RunCount30d,
		CostAmount30d:     summary.CostAmount30d,
		Currency:          summary.Currency,
		Source:            summary.Source,
	}
}

func recentEventSummaryResponses(events []DigitalEmployeeRecentEventSummary) []digitalEmployeeRecentEventSummaryResponse {
	responses := make([]digitalEmployeeRecentEventSummaryResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, digitalEmployeeRecentEventSummaryResponse{
			Label:      event.Label,
			Status:     event.Status,
			OccurredAt: timeStringPtr(event.OccurredAt),
		})
	}
	return responses
}

func overviewFiltersResponseFromDomain(filters DigitalEmployeeOverviewFilters) digitalEmployeeOverviewFiltersResponse {
	return digitalEmployeeOverviewFiltersResponse{
		Teams:             overviewFilterOptionResponses(filters.Teams),
		Statuses:          overviewFilterOptionResponses(filters.Statuses),
		EmployeeTypes:     overviewFilterOptionResponses(filters.EmployeeTypes),
		Providers:         overviewFilterOptionResponses(filters.Providers),
		RuntimeNodes:      overviewFilterOptionResponses(filters.RuntimeNodes),
		RiskLevels:        overviewFilterOptionResponses(filters.RiskLevels),
		ExecutionStatuses: overviewFilterOptionResponses(filters.ExecutionStatuses),
		RunStatuses:       overviewFilterOptionResponses(filters.RunStatuses),
	}
}

func overviewFilterOptionResponses(options []OverviewFilterOption) []overviewFilterOptionResponse {
	responses := make([]overviewFilterOptionResponse, 0, len(options))
	for _, option := range options {
		responses = append(responses, overviewFilterOptionResponse{
			Value: option.Value,
			Label: option.Label,
		})
	}
	return responses
}

func overviewPaginationResponseFromDomain(pagination OverviewPagination) overviewPaginationResponse {
	return overviewPaginationResponse{
		Limit:      pagination.Limit,
		Offset:     pagination.Offset,
		TotalCount: pagination.TotalCount,
	}
}

func createOptionsResponseFromDomain(options *CreateOptions) createOptionsResponse {
	runtimeOptions := make([]runtimeProviderOptionResponse, 0, len(options.RuntimeProviderOptions))
	for _, option := range options.RuntimeProviderOptions {
		runtimeOptions = append(runtimeOptions, runtimeProviderOptionResponse{
			RuntimeNodeID:         option.RuntimeNodeID.String(),
			NodeID:                option.NodeID,
			RuntimeName:           option.RuntimeName,
			ProviderType:          option.ProviderType,
			RuntimeStatus:         option.RuntimeStatus,
			ProviderStatus:        option.ProviderStatus,
			HealthStatus:          option.HealthStatus,
			CurrentLoad:           option.CurrentLoad,
			MaxSlots:              option.MaxSlots,
			AgentHomeDir:          option.AgentHomeDir,
			AgentHomeDirAvailable: option.AgentHomeDirAvailable,
			Available:             option.Available,
			DisabledReason:        option.DisabledReason,
		})
	}
	employeeTypes := make([]employeeTypeOptionResponse, 0, len(options.EmployeeTypes))
	for _, definition := range options.EmployeeTypes {
		employeeTypes = append(employeeTypes, employeeTypeOptionResponse{
			Type:                         definition.Type,
			Label:                        definition.Label,
			Description:                  definition.Description,
			DefaultRole:                  definition.DefaultRole,
			RecommendedSkills:            stringSliceForJSON(definition.RecommendedSkills),
			RecommendedMCPServers:        stringSliceForJSON(definition.RecommendedMCPServers),
			RecommendedProviderTypes:     stringSliceForJSON(definition.RecommendedProviderTypes),
			DefaultCapabilitySelection:   cloneMap(definition.DefaultCapabilitySelection),
			DefaultContextPolicyOverride: cloneMap(definition.DefaultContextPolicyOverride),
			DefaultApprovalPolicy:        cloneMap(definition.DefaultApprovalPolicy),
			Metadata:                     cloneMap(definition.Metadata),
		})
	}
	domainChecks := options.CreationChecks
	if len(domainChecks) == 0 {
		domainChecks = createOptionChecks(options.TeamConfig, options.EmployeeTypes, options.CapabilityOptions, options.RuntimeProviderOptions)
	}
	creationChecks := make([]createOptionCheckResponse, 0, len(domainChecks))
	for _, check := range domainChecks {
		creationChecks = append(creationChecks, createOptionCheckResponse{
			Key:     check.Key,
			Label:   check.Label,
			Status:  check.Status,
			Message: check.Message,
		})
	}
	return createOptionsResponse{
		TeamConfig: teamConfigCreateOptionResponse{
			ID:           options.TeamConfig.ID.String(),
			TenantID:     options.TeamConfig.TenantID.String(),
			TeamID:       uuidStringPtr(options.TeamConfig.TeamID),
			Constitution: cloneMap(options.TeamConfig.Constitution),
			Skills:       stringSliceForJSON(options.TeamConfig.Skills),
			MCPServers:   stringSliceForJSON(options.TeamConfig.MCPServers),
		},
		EmployeeTypes: employeeTypes,
		CapabilityOptions: capabilityOptionsResponse{
			ProviderTypes: stringSliceForJSON(options.CapabilityOptions.ProviderTypes),
			Skills:        stringSliceForJSON(options.CapabilityOptions.Skills),
			MCPServers:    stringSliceForJSON(options.CapabilityOptions.MCPServers),
		},
		RuntimeProviderOptions: runtimeOptions,
		CreationChecks:         creationChecks,
		PolicyDefaults: policyDefaultsResponse{
			PermissionPolicy:      cloneMap(options.PolicyDefaults.PermissionPolicy),
			ContextPolicyOverride: cloneMap(options.PolicyDefaults.ContextPolicyOverride),
			ApprovalPolicy:        cloneMap(options.PolicyDefaults.ApprovalPolicy),
			CapabilitySelection:   cloneMap(options.PolicyDefaults.CapabilitySelection),
			RuntimeSelector:       cloneMap(options.PolicyDefaults.RuntimeSelector),
			WorkspacePolicy:       cloneMap(options.PolicyDefaults.WorkspacePolicy),
			SessionPolicy:         cloneMap(options.PolicyDefaults.SessionPolicy),
			Metadata:              cloneMap(options.PolicyDefaults.Metadata),
		},
	}
}

func stringSliceForJSON(values []string) []string {
	if values == nil {
		return []string{}
	}
	return cloneStringSlice(values)
}

func executionInstanceResponseFromDomain(instance *DigitalEmployeeExecutionInstance) executionInstanceResponse {
	return executionInstanceResponse{
		ID:                   instance.ID.String(),
		TenantID:             instance.TenantID.String(),
		DigitalEmployeeID:    instance.DigitalEmployeeID.String(),
		RuntimeNodeID:        instance.RuntimeNodeID.String(),
		ProviderType:         instance.ProviderType,
		AgentHomeDir:         instance.AgentHomeDir,
		WorkspacePolicy:      cloneMap(instance.WorkspacePolicy),
		SessionPolicy:        cloneMap(instance.SessionPolicy),
		RuntimeSelector:      cloneMap(instance.RuntimeSelector),
		CapacityRequirements: cloneMap(instance.CapacityRequirements),
		FallbackPolicy:       cloneMap(instance.FallbackPolicy),
		Status:               instance.Status,
		ReadyAt:              timeStringPtr(instance.ReadyAt),
		DisabledAt:           timeStringPtr(instance.DisabledAt),
		ErrorAt:              timeStringPtr(instance.ErrorAt),
		ErrorMessage:         instance.ErrorMessage,
		Metadata:             cloneMap(instance.Metadata),
		CreatedAt:            timeString(instance.CreatedAt),
		UpdatedAt:            timeString(instance.UpdatedAt),
	}
}

func configRevisionResponseFromDomain(revision *DigitalEmployeeConfigRevision) configRevisionResponse {
	return configRevisionResponse{
		ID:                    revision.ID.String(),
		TenantID:              revision.TenantID.String(),
		DigitalEmployeeID:     revision.DigitalEmployeeID.String(),
		RevisionNumber:        revision.RevisionNumber,
		PersonaMemoryMarkdown: revision.PersonaMemoryMarkdown,
		CapabilityBindings:    cloneMap(revision.CapabilityBindings),
		BudgetPolicy:          cloneMap(revision.BudgetPolicy),
		Status:                revision.Status,
		ApprovedBy:            uuidStringPtr(revision.ApprovedBy),
		ApprovedAt:            timeStringPtr(revision.ApprovedAt),
		ArchivedAt:            timeStringPtr(revision.ArchivedAt),
		CreatedAt:             timeString(revision.CreatedAt),
		UpdatedAt:             timeString(revision.UpdatedAt),
	}
}

func uuidStringPtr(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func optionalUUIDString(value uuid.UUID) string {
	if value == uuid.Nil {
		return ""
	}
	return value.String()
}

func optionalUUIDStringPtr(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}

func timeStringPtr(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	text := value.UTC().Format(time.RFC3339Nano)
	return &text
}

func timeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
