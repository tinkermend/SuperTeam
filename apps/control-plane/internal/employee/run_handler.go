package employee

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type RunHandlerService interface {
	CreateRun(ctx context.Context, req CreateDigitalEmployeeRunRequest) (*DigitalEmployeeRun, error)
	ListRuns(ctx context.Context, tenantID, employeeID uuid.UUID, limit, offset int32) ([]*DigitalEmployeeRun, error)
	GetRun(ctx context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error)
	ListRunEvents(ctx context.Context, tenantID, employeeID, runID uuid.UUID, limit, offset int32) ([]RuntimeCommandEventWriteback, error)
	StopRun(ctx context.Context, req StopDigitalEmployeeRunRequest) (*DigitalEmployeeRun, error)
}

const (
	defaultRunPageLimit = 50
	maxRunPageLimit     = 100
)

func (h *HTTPHandler) CreateDigitalEmployeeRun(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRunCreate, &employeeID, "digital employee run create")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Objective        string           `json:"objective"`
		Prompt           string           `json:"prompt"`
		ContextRefs      []map[string]any `json:"context_refs"`
		ArtifactRefs     []map[string]any `json:"artifact_refs"`
		OutputSchema     map[string]any   `json:"output_schema"`
		AllowedActions   []string         `json:"allowed_actions"`
		ForbiddenActions []string         `json:"forbidden_actions"`
		SecretRefs       []string         `json:"secret_refs"`
		IdempotencyKey   *string          `json:"idempotency_key"`
		TimeoutSec       *int32           `json:"timeout_sec"`
		GraceSec         *int32           `json:"grace_sec"`
		Metadata         map[string]any   `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	run, err := service.CreateRun(r.Context(), CreateDigitalEmployeeRunRequest{
		TenantID:          tenantID,
		UserID:            middleware.GetUserID(r.Context()),
		DigitalEmployeeID: employeeID,
		Objective:         req.Objective,
		Prompt:            req.Prompt,
		ContextRefs:       req.ContextRefs,
		ArtifactRefs:      req.ArtifactRefs,
		OutputSchema:      req.OutputSchema,
		AllowedActions:    req.AllowedActions,
		ForbiddenActions:  req.ForbiddenActions,
		SecretRefs:        req.SecretRefs,
		IdempotencyKey:    req.IdempotencyKey,
		TimeoutSec:        req.TimeoutSec,
		GraceSec:          req.GraceSec,
		Metadata:          req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, runResponseFromDomain(run))
}

func (h *HTTPHandler) ListDigitalEmployeeRuns(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee run read")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	limit, offset, parseErr := parseRunPagination(r)
	if parseErr != "" {
		http.Error(w, parseErr, http.StatusBadRequest)
		return
	}
	runs, err := service.ListRuns(r.Context(), tenantID, employeeID, int32(limit), int32(offset))
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runResponses(runs))
}

func (h *HTTPHandler) GetDigitalEmployeeRun(w http.ResponseWriter, r *http.Request) {
	employeeID, runID, ok := employeeAndRunIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee run read")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	run, err := service.GetRun(r.Context(), tenantID, employeeID, runID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runResponseFromDomain(run))
}

func (h *HTTPHandler) ListDigitalEmployeeRunEvents(w http.ResponseWriter, r *http.Request) {
	employeeID, runID, ok := employeeAndRunIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee run events read")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	limit, offset, parseErr := parseRunPagination(r)
	if parseErr != "" {
		http.Error(w, parseErr, http.StatusBadRequest)
		return
	}
	events, err := service.ListRunEvents(r.Context(), tenantID, employeeID, runID, int32(limit), int32(offset))
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *HTTPHandler) StopDigitalEmployeeRun(w http.ResponseWriter, r *http.Request) {
	employeeID, runID, ok := employeeAndRunIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRunStop, &employeeID, "digital employee run stop")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	run, err := service.StopRun(r.Context(), StopDigitalEmployeeRunRequest{
		TenantID:          tenantID,
		UserID:            middleware.GetUserID(r.Context()),
		DigitalEmployeeID: employeeID,
		RunID:             runID,
		Reason:            req.Reason,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runResponseFromDomain(run))
}

func (h *HTTPHandler) runServiceFromRequest(w http.ResponseWriter) (RunHandlerService, bool) {
	if h == nil || h.runService == nil {
		http.Error(w, "digital employee run service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.runService, true
}

func parseRunPagination(r *http.Request) (int, int, string) {
	query := r.URL.Query()
	limit := defaultRunPageLimit
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, "limit must be an integer"
		}
		if parsed <= 0 {
			return 0, 0, "limit must be greater than 0"
		}
		if parsed > maxRunPageLimit {
			parsed = maxRunPageLimit
		}
		limit = parsed
	}

	offset := 0
	if raw := query.Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, "offset must be an integer"
		}
		if parsed < 0 {
			return 0, 0, "offset must be greater than or equal to 0"
		}
		offset = parsed
	}
	return limit, offset, ""
}

type digitalEmployeeRunResponse struct {
	ID                        string                   `json:"id"`
	TenantID                  string                   `json:"tenant_id"`
	TaskID                    string                   `json:"task_id"`
	DigitalEmployeeID         string                   `json:"digital_employee_id"`
	ExecutionInstanceID       string                   `json:"execution_instance_id"`
	RuntimeNodeID             string                   `json:"runtime_node_id"`
	NodeID                    string                   `json:"node_id"`
	CommandID                 string                   `json:"command_id"`
	ProviderType              string                   `json:"provider_type"`
	ProviderSessionID         *string                  `json:"provider_session_id,omitempty"`
	ProviderSessionExternalID *string                  `json:"provider_session_external_id,omitempty"`
	Status                    DigitalEmployeeRunStatus `json:"status"`
	Result                    map[string]any           `json:"result"`
	Diagnostic                map[string]any           `json:"diagnostic"`
	LogRef                    *string                  `json:"log_ref,omitempty"`
	RawResultRef              *string                  `json:"raw_result_ref,omitempty"`
	WorkProducts              []WorkProduct            `json:"work_products"`
	SessionState              map[string]any           `json:"session_state"`
	ErrorMessage              *string                  `json:"error_message,omitempty"`
	ErrorCode                 *string                  `json:"error_code,omitempty"`
	ErrorFamily               *string                  `json:"error_family,omitempty"`
	ExitCode                  *int32                   `json:"exit_code,omitempty"`
	Signal                    *string                  `json:"signal,omitempty"`
	TimedOut                  bool                     `json:"timed_out"`
	IdempotencyKey            *string                  `json:"idempotency_key,omitempty"`
	TimeoutSec                *int32                   `json:"timeout_sec,omitempty"`
	GraceSec                  *int32                   `json:"grace_sec,omitempty"`
	StartedAt                 string                   `json:"started_at,omitempty"`
	CompletedAt               *string                  `json:"completed_at,omitempty"`
	FinishedAt                *string                  `json:"finished_at,omitempty"`
	CreatedAt                 string                   `json:"created_at,omitempty"`
	UpdatedAt                 string                   `json:"updated_at,omitempty"`
}

func runResponses(runs []*DigitalEmployeeRun) []digitalEmployeeRunResponse {
	responses := make([]digitalEmployeeRunResponse, 0, len(runs))
	for _, run := range runs {
		responses = append(responses, runResponseFromDomain(run))
	}
	return responses
}

func runResponseFromDomain(run *DigitalEmployeeRun) digitalEmployeeRunResponse {
	return digitalEmployeeRunResponse{
		ID:                        run.ID.String(),
		TenantID:                  run.TenantID.String(),
		TaskID:                    run.TaskID.String(),
		DigitalEmployeeID:         run.DigitalEmployeeID.String(),
		ExecutionInstanceID:       run.ExecutionInstanceID.String(),
		RuntimeNodeID:             run.RuntimeNodeID.String(),
		NodeID:                    run.NodeID,
		CommandID:                 run.CommandID,
		ProviderType:              run.ProviderType,
		ProviderSessionID:         run.ProviderSessionID,
		ProviderSessionExternalID: run.ProviderSessionExternalID,
		Status:                    run.Status,
		Result:                    cloneMap(run.Result),
		Diagnostic:                cloneMap(run.Diagnostic),
		LogRef:                    run.LogRef,
		RawResultRef:              run.RawResultRef,
		WorkProducts:              run.WorkProducts,
		SessionState:              cloneMap(run.SessionState),
		ErrorMessage:              run.ErrorMessage,
		ErrorCode:                 run.ErrorCode,
		ErrorFamily:               run.ErrorFamily,
		ExitCode:                  run.ExitCode,
		Signal:                    run.Signal,
		TimedOut:                  run.TimedOut,
		IdempotencyKey:            run.IdempotencyKey,
		TimeoutSec:                run.TimeoutSec,
		GraceSec:                  run.GraceSec,
		StartedAt:                 timeString(run.StartedAt),
		CompletedAt:               timeStringPtr(run.CompletedAt),
		FinishedAt:                timeStringPtr(run.FinishedAt),
		CreatedAt:                 timeString(run.CreatedAt),
		UpdatedAt:                 timeString(run.UpdatedAt),
	}
}

func employeeAndRunIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runId"))
	if err != nil || runID == uuid.Nil {
		http.Error(w, "invalid run id", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	return employeeID, runID, true
}
