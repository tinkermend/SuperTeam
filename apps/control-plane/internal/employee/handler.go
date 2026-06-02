package employee

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
	CreateDraft(ctx context.Context, req CreateDraftRequest) (*DigitalEmployee, error)
	ListDigitalEmployees(ctx context.Context, req ListDigitalEmployeesRequest) ([]*DigitalEmployee, error)
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployee, error)
	UpdateStatus(ctx context.Context, req UpdateStatusRequest) (*DigitalEmployee, error)
	GetExecutionInstance(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeExecutionInstance, error)
	BindExecutionInstance(ctx context.Context, req BindExecutionInstanceRequest) (*DigitalEmployeeExecutionInstance, error)
}

type HTTPHandler struct {
	service HandlerService
}

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) ListDigitalEmployees(w http.ResponseWriter, r *http.Request) {
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	tenantID, ok := tenantIDFromContext(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	status := DigitalEmployeeStatus(r.URL.Query().Get("status"))

	employees, err := service.ListDigitalEmployees(r.Context(), ListDigitalEmployeesRequest{
		TenantID: tenantID,
		Status:   status,
		Offset:   int32(offset),
		Limit:    int32(limit),
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeResponses(employees))
}

func (h *HTTPHandler) CreateDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	tenantID, ok := tenantIDFromContext(w, r)
	if !ok {
		return
	}
	var req struct {
		TeamID           *uuid.UUID     `json:"team_id"`
		Name             string         `json:"name"`
		Role             string         `json:"role"`
		Description      *string        `json:"description"`
		PermissionPolicy map[string]any `json:"permission_policy"`
		ContextPolicy    map[string]any `json:"context_policy"`
		ApprovalPolicy   map[string]any `json:"approval_policy"`
		RiskLevel        string         `json:"risk_level"`
		Metadata         map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	employee, err := service.CreateDraft(r.Context(), CreateDraftRequest{
		TenantID:         tenantID,
		TeamID:           req.TeamID,
		Name:             req.Name,
		Role:             req.Role,
		Description:      req.Description,
		PermissionPolicy: req.PermissionPolicy,
		ContextPolicy:    req.ContextPolicy,
		ApprovalPolicy:   req.ApprovalPolicy,
		RiskLevel:        req.RiskLevel,
		Metadata:         req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, employeeResponseFromDomain(employee))
}

func (h *HTTPHandler) GetDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	tenantID, employeeID, ok := tenantAndEmployeeIDFromRequest(w, r)
	if !ok {
		return
	}
	employee, err := service.GetDigitalEmployee(r.Context(), tenantID, employeeID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeResponseFromDomain(employee))
}

func (h *HTTPHandler) UpdateDigitalEmployeeStatus(w http.ResponseWriter, r *http.Request) {
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	tenantID, employeeID, ok := tenantAndEmployeeIDFromRequest(w, r)
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
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	tenantID, employeeID, ok := tenantAndEmployeeIDFromRequest(w, r)
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
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	tenantID, employeeID, ok := tenantAndEmployeeIDFromRequest(w, r)
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

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "employee service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

type digitalEmployeeResponse struct {
	ID               string                `json:"id"`
	TenantID         string                `json:"tenant_id"`
	TeamID           *string               `json:"team_id,omitempty"`
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
	CreatedAt        string                `json:"created_at,omitempty"`
	UpdatedAt        string                `json:"updated_at,omitempty"`
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

func tenantAndEmployeeIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	tenantID, ok := tenantIDFromContext(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	employeeID, err := uuid.Parse(chi.URLParam(r, "employeeId"))
	if err != nil || employeeID == uuid.Nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, employeeID, true
}

func tenantIDFromContext(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "tenant_id not found in context", http.StatusUnauthorized)
		return uuid.Nil, false
	}
	return tenantID, true
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

func uuidStringPtr(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
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
