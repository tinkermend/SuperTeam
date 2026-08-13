package employee

import (
	"context"
	"encoding/json"
	"github.com/superteam/control-plane/internal/capabilityprojection"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type RunHandlerService interface {
	CreateRun(ctx context.Context, req CreateDigitalEmployeeRunRequest) (*DigitalEmployeeRun, error)
	ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error)
	ListChatThreads(ctx context.Context, tenantID, employeeID, projectID uuid.UUID) ([]DigitalEmployeeChatThread, error)
	RenameChatThread(ctx context.Context, tenantID, employeeID, threadID, actorUserID uuid.UUID, title string) (*DigitalEmployeeChatThread, error)
	GetRunCalendar(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) (*DigitalEmployeeRunCalendarResult, error)
	GetRun(ctx context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error)
	ListRunEvents(ctx context.Context, tenantID, employeeID, runID uuid.UUID, limit, offset int32) ([]RuntimeCommandEventWriteback, error)
	StopRun(ctx context.Context, req StopDigitalEmployeeRunRequest) (*DigitalEmployeeRun, error)
	GetRunStats(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRunStats, error)
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
		Objective            string           `json:"objective"`
		Prompt               string           `json:"prompt"`
		ContextRefs          []map[string]any `json:"context_refs"`
		ArtifactRefs         []map[string]any `json:"artifact_refs"`
		OutputSchema         map[string]any   `json:"output_schema"`
		AllowedActions       []string         `json:"allowed_actions"`
		ForbiddenActions     []string         `json:"forbidden_actions"`
		SecretRefs           []string         `json:"secret_refs"`
		IdempotencyKey       *string          `json:"idempotency_key"`
		TimeoutSec           *int32           `json:"timeout_sec"`
		GraceSec             *int32           `json:"grace_sec"`
		Metadata             map[string]any   `json:"metadata"`
		RunKind              string           `json:"run_kind"`
		ResumeOfRunID        *uuid.UUID       `json:"resume_of_run_id"`
		ChatThreadID         *uuid.UUID       `json:"chat_thread_id"`
		ProjectID            *uuid.UUID       `json:"project_id"`
		SkillIDs             []uuid.UUID      `json:"skill_ids"`
		InteractiveConfirmed bool             `json:"interactive_confirmed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	run, err := service.CreateRun(r.Context(), CreateDigitalEmployeeRunRequest{
		TenantID:             tenantID,
		UserID:               middleware.GetUserID(r.Context()),
		DigitalEmployeeID:    employeeID,
		Objective:            req.Objective,
		Prompt:               req.Prompt,
		ContextRefs:          req.ContextRefs,
		ArtifactRefs:         req.ArtifactRefs,
		OutputSchema:         req.OutputSchema,
		AllowedActions:       req.AllowedActions,
		ForbiddenActions:     req.ForbiddenActions,
		SecretRefs:           req.SecretRefs,
		IdempotencyKey:       req.IdempotencyKey,
		TimeoutSec:           req.TimeoutSec,
		GraceSec:             req.GraceSec,
		Metadata:             req.Metadata,
		RunKind:              req.RunKind,
		ResumeOfRunID:        req.ResumeOfRunID,
		ChatThreadID:         req.ChatThreadID,
		ProjectID:            req.ProjectID,
		SkillIDs:             req.SkillIDs,
		InteractiveConfirmed: req.InteractiveConfirmed,
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
	filter, parseErr := parseRunListFilter(r)
	if parseErr != "" {
		http.Error(w, parseErr, http.StatusBadRequest)
		return
	}
	result, err := service.ListRunsDetailed(r.Context(), tenantID, employeeID, filter)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runListResponseFromDomain(result))
}

func (h *HTTPHandler) ListDigitalEmployeeChatThreads(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee chat thread list")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	projectID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("project_id")))
	if err != nil || projectID == uuid.Nil {
		http.Error(w, "project_id must be a valid uuid", http.StatusBadRequest)
		return
	}
	items, err := service.ListChatThreads(r.Context(), tenantID, employeeID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chatThreadListResponseFromDomain(items))
}

func (h *HTTPHandler) PatchDigitalEmployeeChatThread(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRunCreate, &employeeID, "digital employee chat thread rename")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	threadID, err := uuid.Parse(chi.URLParam(r, "threadId"))
	if err != nil || threadID == uuid.Nil {
		http.Error(w, "threadId must be a valid uuid", http.StatusBadRequest)
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	thread, err := service.RenameChatThread(r.Context(), tenantID, employeeID, threadID, middleware.GetUserID(r.Context()), req.Title)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chatThreadResponseFromDomain(thread))
}

// parseRunListFilter parses the run list query parameters: pagination (limit/offset),
// comma-separated status filter, project_id, and RFC3339 from/to time window. An empty
// error string means success; a non-empty string is an HTTP 400 message.
func parseRunListFilter(r *http.Request) (DigitalEmployeeRunListFilter, string) {
	limit, offset, parseErr := parseRunPagination(r)
	if parseErr != "" {
		return DigitalEmployeeRunListFilter{}, parseErr
	}
	query := r.URL.Query()
	filter := DigitalEmployeeRunListFilter{Limit: limit, Offset: offset}
	if raw := query.Get("status"); raw != "" {
		filter.Statuses = strings.Split(raw, ",")
	}
	if raw := query.Get("project_id"); raw != "" {
		projectID, err := uuid.Parse(raw)
		if err != nil {
			return DigitalEmployeeRunListFilter{}, "project_id must be a valid uuid"
		}
		filter.ProjectID = &projectID
	}
	if raw := query.Get("from"); raw != "" {
		from, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return DigitalEmployeeRunListFilter{}, "from must be an RFC3339 timestamp"
		}
		filter.From = &from
	}
	if raw := query.Get("to"); raw != "" {
		to, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return DigitalEmployeeRunListFilter{}, "to must be an RFC3339 timestamp"
		}
		filter.To = &to
	}
	if raw := query.Get("run_kind"); raw != "" {
		if raw != RunKindTask && raw != RunKindChat {
			return DigitalEmployeeRunListFilter{}, ErrInvalidRunKind.Error()
		}
		filter.RunKind = &raw
	}
	if raw := query.Get("chat_thread_id"); raw != "" {
		threadID, err := uuid.Parse(raw)
		if err != nil {
			return DigitalEmployeeRunListFilter{}, "chat_thread_id must be a valid uuid"
		}
		filter.ChatThreadID = &threadID
	}
	return filter, ""
}

func (h *HTTPHandler) GetDigitalEmployeeRunStats(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee run stats read")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	stats, err := service.GetRunStats(r.Context(), tenantID, employeeID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runStatsResponseFromDomain(stats))
}

func (h *HTTPHandler) GetDigitalEmployeeRunCalendar(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee run calendar read")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	from, to, parseErr := parseRunCalendarWindow(r)
	if parseErr != "" {
		http.Error(w, parseErr, http.StatusBadRequest)
		return
	}
	result, err := service.GetRunCalendar(r.Context(), tenantID, employeeID, from, to)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runCalendarResponseFromDomain(result))
}

func parseRunCalendarWindow(r *http.Request) (time.Time, time.Time, string) {
	query := r.URL.Query()
	rawFrom := query.Get("from")
	rawTo := query.Get("to")
	if rawFrom == "" || rawTo == "" {
		return time.Time{}, time.Time{}, "from and to are required RFC3339 timestamps"
	}
	from, err := time.Parse(time.RFC3339, rawFrom)
	if err != nil {
		return time.Time{}, time.Time{}, "from must be an RFC3339 timestamp"
	}
	to, err := time.Parse(time.RFC3339, rawTo)
	if err != nil {
		return time.Time{}, time.Time{}, "to must be an RFC3339 timestamp"
	}
	return from, to, ""
}

type digitalEmployeeRunStatsResponse struct {
	TotalCount     int64    `json:"total_count"`
	SucceededCount int64    `json:"succeeded_count"`
	FailedCount    int64    `json:"failed_count"`
	CancelledCount int64    `json:"cancelled_count"`
	SuccessRate    *float64 `json:"success_rate"`
	AvgDurationSec *float64 `json:"avg_duration_sec"`
	P90DurationSec *float64 `json:"p90_duration_sec"`
	Last7dCount    int64    `json:"last_7d_count"`
	Prev7dCount    int64    `json:"prev_7d_count"`
}

func runStatsResponseFromDomain(stats *DigitalEmployeeRunStats) digitalEmployeeRunStatsResponse {
	var successRate *float64
	if stats.TotalCount > 0 {
		rate := float64(stats.SucceededCount) / float64(stats.TotalCount)
		successRate = &rate
	}
	return digitalEmployeeRunStatsResponse{
		TotalCount:     stats.TotalCount,
		SucceededCount: stats.SucceededCount,
		FailedCount:    stats.FailedCount,
		CancelledCount: stats.CancelledCount,
		SuccessRate:    successRate,
		AvgDurationSec: stats.AvgDurationSec,
		P90DurationSec: stats.P90DurationSec,
		Last7dCount:    stats.Last7dCount,
		Prev7dCount:    stats.Prev7dCount,
	}
}

type digitalEmployeeRunCalendarItemResponse struct {
	ID             string `json:"id"`
	TaskTitle      string `json:"task_title"`
	Status         string `json:"status"`
	RunKind        string `json:"run_kind"`
	CreatedAt      string `json:"created_at"`
	ProjectID      string `json:"project_id,omitempty"`
	ProjectName    string `json:"project_name,omitempty"`
	ProjectDeleted bool   `json:"project_deleted,omitempty"`
}

type digitalEmployeeRunCalendarResponse struct {
	From       string                                   `json:"from"`
	To         string                                   `json:"to"`
	TotalCount int64                                    `json:"total_count"`
	Truncated  bool                                     `json:"truncated"`
	Items      []digitalEmployeeRunCalendarItemResponse `json:"items"`
}

func runCalendarResponseFromDomain(result *DigitalEmployeeRunCalendarResult) digitalEmployeeRunCalendarResponse {
	items := make([]digitalEmployeeRunCalendarItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		row := digitalEmployeeRunCalendarItemResponse{
			ID:        item.ID.String(),
			TaskTitle: item.TaskTitle,
			Status:    string(item.Status),
			RunKind:   item.RunKind,
			CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if item.ProjectID != nil {
			row.ProjectID = item.ProjectID.String()
		}
		if item.ProjectName != nil {
			row.ProjectName = *item.ProjectName
		}
		row.ProjectDeleted = item.ProjectDeleted
		items = append(items, row)
	}
	return digitalEmployeeRunCalendarResponse{
		From:       result.From.UTC().Format(time.RFC3339Nano),
		To:         result.To.UTC().Format(time.RFC3339Nano),
		TotalCount: result.TotalCount,
		Truncated:  result.Truncated,
		Items:      items,
	}
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
	events, err := service.ListRunEvents(r.Context(), tenantID, employeeID, runID, limit, offset)
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

func parseRunPagination(r *http.Request) (int32, int32, string) {
	query := r.URL.Query()
	limit := int32(defaultRunPageLimit)
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			return 0, 0, "limit must be an integer"
		}
		if parsed <= 0 {
			return 0, 0, "limit must be greater than 0"
		}
		if parsed > maxRunPageLimit {
			parsed = maxRunPageLimit
		}
		limit = int32(parsed)
	}

	var offset int32
	if raw := query.Get("offset"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			return 0, 0, "offset must be an integer"
		}
		if parsed < 0 {
			return 0, 0, "offset must be greater than or equal to 0"
		}
		offset = int32(parsed)
	}
	return limit, offset, ""
}

type digitalEmployeeRunResponse struct {
	ID                        string                                             `json:"id"`
	TenantID                  string                                             `json:"tenant_id"`
	TaskID                    string                                             `json:"task_id"`
	DigitalEmployeeID         string                                             `json:"digital_employee_id"`
	ExecutionInstanceID       string                                             `json:"execution_instance_id"`
	RuntimeNodeID             string                                             `json:"runtime_node_id"`
	NodeID                    string                                             `json:"node_id"`
	CommandID                 string                                             `json:"command_id"`
	ProviderType              string                                             `json:"provider_type"`
	ProviderSessionID         *string                                            `json:"provider_session_id,omitempty"`
	ProviderSessionExternalID *string                                            `json:"provider_session_external_id,omitempty"`
	RunKind                   string                                             `json:"run_kind"`
	ResumeOfRunID             *string                                            `json:"resume_of_run_id,omitempty"`
	ChatThreadID              *string                                            `json:"chat_thread_id,omitempty"`
	CreatorUserID             *string                                            `json:"creator_user_id,omitempty"`
	CreatorDisplayName        *string                                            `json:"creator_display_name,omitempty"`
	Status                    DigitalEmployeeRunStatus                           `json:"status"`
	Result                    map[string]any                                     `json:"result"`
	Diagnostic                map[string]any                                     `json:"diagnostic"`
	LogRef                    *string                                            `json:"log_ref,omitempty"`
	RawResultRef              *string                                            `json:"raw_result_ref,omitempty"`
	WorkProducts              []WorkProduct                                      `json:"work_products"`
	SessionState              map[string]any                                     `json:"session_state"`
	ErrorMessage              *string                                            `json:"error_message,omitempty"`
	ErrorCode                 *string                                            `json:"error_code,omitempty"`
	ErrorFamily               *string                                            `json:"error_family,omitempty"`
	ExitCode                  *int32                                             `json:"exit_code,omitempty"`
	Signal                    *string                                            `json:"signal,omitempty"`
	TimedOut                  bool                                               `json:"timed_out"`
	IdempotencyKey            *string                                            `json:"idempotency_key,omitempty"`
	TimeoutSec                *int32                                             `json:"timeout_sec,omitempty"`
	GraceSec                  *int32                                             `json:"grace_sec,omitempty"`
	FailureAcknowledgedAt     *string                                            `json:"failure_acknowledged_at,omitempty"`
	StartedAt                 string                                             `json:"started_at,omitempty"`
	CompletedAt               *string                                            `json:"completed_at,omitempty"`
	FinishedAt                *string                                            `json:"finished_at,omitempty"`
	CreatedAt                 string                                             `json:"created_at,omitempty"`
	UpdatedAt                 string                                             `json:"updated_at,omitempty"`
	ProjectID                 *string                                            `json:"project_id,omitempty"`
	ProjectName               *string                                            `json:"project_name,omitempty"`
	ProjectDeleted            bool                                               `json:"project_deleted,omitempty"`
	CapabilityProjection      *capabilityprojection.CapabilityProjectionSnapshot `json:"capability_projection,omitempty"`
}

func runResponseFromDomain(run *DigitalEmployeeRun) digitalEmployeeRunResponse {
	response := digitalEmployeeRunResponse{
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
		RunKind:                   run.RunKind,
		ResumeOfRunID:             uuidStringPtr(run.ResumeOfRunID),
		ChatThreadID:              uuidStringPtr(run.ChatThreadID),
		CreatorUserID:             uuidStringPtr(run.CreatorUserID),
		CreatorDisplayName:        run.CreatorDisplayName,
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
		FailureAcknowledgedAt:     timeStringPtr(run.FailureAcknowledgedAt),
		StartedAt:                 timeString(run.StartedAt),
		CompletedAt:               timeStringPtr(run.CompletedAt),
		FinishedAt:                timeStringPtr(run.FinishedAt),
		CreatedAt:                 timeString(run.CreatedAt),
		UpdatedAt:                 timeString(run.UpdatedAt),
		ProjectName:               run.ProjectName,
		ProjectDeleted:            run.ProjectDeleted,
		CapabilityProjection:      run.CapabilityProjection,
	}
	if run.ProjectID != nil {
		id := run.ProjectID.String()
		response.ProjectID = &id
	}
	return response
}

// digitalEmployeeRunListItemResponse embeds the base run response and adds the joined
// task/project context, work-product count, and finished-run duration surfaced by the
// detailed list query.
type digitalEmployeeRunListItemResponse struct {
	digitalEmployeeRunResponse
	TaskTitle        string   `json:"task_title"`
	ProjectID        *string  `json:"project_id,omitempty"`
	ProjectName      *string  `json:"project_name,omitempty"`
	ProjectDeleted   bool     `json:"project_deleted,omitempty"`
	WorkProductCount int32    `json:"work_product_count"`
	DurationSec      *float64 `json:"duration_sec,omitempty"`
}

// runFilterOption is a single selectable filter value with a localized label, used for
// the statuses and projects filter dropdowns in the run list response.
type runFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// digitalEmployeeRunListResponse is the new run list payload shape: a paginated items
// array, the total count of runs matching the active filters (independent of limit/
// offset), and the filter options (statuses is a fixed enum; projects is scoped to the
// employee's runs).
type digitalEmployeeRunListResponse struct {
	Items      []digitalEmployeeRunListItemResponse `json:"items"`
	TotalCount int64                                `json:"total_count"`
	Filters    struct {
		Statuses []runFilterOption `json:"statuses"`
		Projects []runFilterOption `json:"projects"`
	} `json:"filters"`
}

// digitalEmployeeRunStatusLabels maps each run status enum value to its localized
// display label for the run list filter dropdown. The order of the underlying
// runStatusFilterOrder slice defines the rendered order.
var digitalEmployeeRunStatusLabels = map[DigitalEmployeeRunStatus]string{
	DigitalEmployeeRunStatusQueued:      "排队中",
	DigitalEmployeeRunStatusDispatching: "调度中",
	DigitalEmployeeRunStatusRunning:     "执行中",
	DigitalEmployeeRunStatusCancelling:  "取消中",
	DigitalEmployeeRunStatusCompleted:   "已完成",
	DigitalEmployeeRunStatusFailed:      "失败",
	DigitalEmployeeRunStatusCancelled:   "已取消",
	DigitalEmployeeRunStatusTimedOut:    "已超时",
}

// runStatusFilterOrder fixes the display order of the status filter options so the
// frontend renders a stable, semantically grouped list regardless of map iteration.
var runStatusFilterOrder = []DigitalEmployeeRunStatus{
	DigitalEmployeeRunStatusQueued,
	DigitalEmployeeRunStatusDispatching,
	DigitalEmployeeRunStatusRunning,
	DigitalEmployeeRunStatusCancelling,
	DigitalEmployeeRunStatusCompleted,
	DigitalEmployeeRunStatusFailed,
	DigitalEmployeeRunStatusCancelled,
	DigitalEmployeeRunStatusTimedOut,
}

func runListResponseFromDomain(result *DigitalEmployeeRunListResult) digitalEmployeeRunListResponse {
	response := digitalEmployeeRunListResponse{TotalCount: result.TotalCount}
	response.Items = make([]digitalEmployeeRunListItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		entry := digitalEmployeeRunListItemResponse{
			digitalEmployeeRunResponse: runResponseFromDomain(item.Run),
			TaskTitle:                  item.TaskTitle,
			WorkProductCount:           item.WorkProductCount,
			DurationSec:                item.DurationSec,
		}
		if item.ProjectID != nil {
			id := item.ProjectID.String()
			entry.ProjectID = &id
		}
		entry.ProjectName = item.ProjectName
		entry.ProjectDeleted = item.ProjectDeleted
		response.Items = append(response.Items, entry)
	}
	for _, status := range runStatusFilterOrder {
		response.Filters.Statuses = append(response.Filters.Statuses, runFilterOption{
			Value: string(status),
			Label: digitalEmployeeRunStatusLabels[status],
		})
	}
	for _, project := range result.Projects {
		label := project.Name
		if project.Deleted {
			label = project.Name + "（已删除）"
		}
		response.Filters.Projects = append(response.Filters.Projects, runFilterOption{
			Value: project.ID.String(),
			Label: label,
		})
	}
	return response
}

func chatThreadResponseFromDomain(thread *DigitalEmployeeChatThread) map[string]any {
	if thread == nil {
		return map[string]any{}
	}
	body := map[string]any{
		"chat_thread_id":            thread.ChatThreadID.String(),
		"title":                     thread.Title,
		"initiator_user_id":         thread.InitiatorUserID.String(),
		"initiator_display_name":    thread.InitiatorDisplayName,
		"last_speaker_user_id":      thread.LastSpeakerUserID.String(),
		"last_speaker_display_name": thread.LastSpeakerDisplayName,
		"last_prompt":               thread.LastPrompt,
		"last_active_at":            thread.LastActiveAt.UTC().Format(time.RFC3339Nano),
		"has_active_run":            thread.HasActiveRun,
	}
	if thread.ActiveRunnerUserID != nil {
		body["active_runner_user_id"] = thread.ActiveRunnerUserID.String()
	}
	if thread.ActiveRunnerDisplayName != nil {
		body["active_runner_display_name"] = *thread.ActiveRunnerDisplayName
	}
	return body
}

func chatThreadListResponseFromDomain(items []DigitalEmployeeChatThread) map[string]any {
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		out = append(out, chatThreadResponseFromDomain(&items[i]))
	}
	return map[string]any{"items": out}
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
