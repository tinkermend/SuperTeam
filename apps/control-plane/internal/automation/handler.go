package automation

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
	ListRules(ctx context.Context, req ListRulesRequest) ([]Rule, error)
	GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error)
	CreateRule(ctx context.Context, req CreateRuleRequest) (Rule, error)
	UpdateRule(ctx context.Context, req UpdateRuleRequest) (Rule, error)
	DeleteRule(ctx context.Context, tenantID, ruleID, actorUserID uuid.UUID) error
	Enable(ctx context.Context, tenantID, ruleID, actorUserID uuid.UUID) (Rule, error)
	Disable(ctx context.Context, tenantID, ruleID, actorUserID uuid.UUID) (Rule, error)
	Trigger(ctx context.Context, req TriggerRequest) (Fire, error)
	ListFires(ctx context.Context, req ListFiresRequest) ([]Fire, error)
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

func (h *HTTPHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorize(w, r, authz.ActionProjectDemandRead, "automation list")
	if !ok {
		return
	}
	req := ListRulesRequest{TenantID: tenantID}
	if projectIDRaw := strings.TrimSpace(r.URL.Query().Get("project_id")); projectIDRaw != "" {
		projectID, err := uuid.Parse(projectIDRaw)
		if err != nil {
			http.Error(w, "invalid project_id", http.StatusBadRequest)
			return
		}
		req.ProjectID = &projectID
	}
	if enabledRaw := strings.TrimSpace(r.URL.Query().Get("enabled")); enabledRaw != "" {
		enabled, err := strconv.ParseBool(enabledRaw)
		if err != nil {
			http.Error(w, "invalid enabled", http.StatusBadRequest)
			return
		}
		req.Enabled = &enabled
	}
	req.Limit = queryInt32(r, "limit", DefaultListLimit)
	req.Offset = queryInt32(r, "offset", 0)
	rules, err := h.service.ListRules(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	responses := make([]ruleResponse, 0, len(rules))
	for _, rule := range rules {
		responses = append(responses, ruleResponseFrom(rule))
	}
	writeJSON(w, http.StatusOK, listRulesResponse{Items: responses})
}

func (h *HTTPHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectDemandSubmit, "automation create")
	if !ok {
		return
	}
	var body createRuleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rule, err := h.service.CreateRule(r.Context(), CreateRuleRequest{
		TenantID:              tenantID,
		ActorUserID:           userID,
		ProjectID:             body.ProjectID,
		Name:                  body.Name,
		CoordinationMode:      body.CoordinationMode,
		DemandTitleTemplate:   body.DemandTitleTemplate,
		DemandBodyTemplate:    body.DemandBodyTemplate,
		ScenarioTemplateKey:   body.ScenarioTemplateKey,
		DigitalEmployeeID:     body.DigitalEmployeeID,
		ChatObjectiveTemplate: body.ChatObjectiveTemplate,
		ScheduleKind:          body.ScheduleKind,
		CronExpr:              body.CronExpr,
		IntervalSeconds:       body.IntervalSeconds,
		Timezone:              body.Timezone,
		Enabled:               body.Enabled,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ruleResponseFrom(rule))
}

func (h *HTTPHandler) GetRule(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorize(w, r, authz.ActionProjectDemandRead, "automation get")
	if !ok {
		return
	}
	ruleID, ok := ruleIDFromRequest(w, r)
	if !ok {
		return
	}
	rule, err := h.service.GetRule(r.Context(), tenantID, ruleID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ruleResponseFrom(rule))
}

func (h *HTTPHandler) PatchRule(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectDemandSubmit, "automation patch")
	if !ok {
		return
	}
	ruleID, ok := ruleIDFromRequest(w, r)
	if !ok {
		return
	}
	var body patchRuleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rule, err := h.service.UpdateRule(r.Context(), UpdateRuleRequest{
		TenantID:              tenantID,
		RuleID:                ruleID,
		ActorUserID:           userID,
		Name:                  body.Name,
		DemandTitleTemplate:   body.DemandTitleTemplate,
		DemandBodyTemplate:    body.DemandBodyTemplate,
		ScenarioTemplateKey:   body.ScenarioTemplateKey,
		DigitalEmployeeID:     body.DigitalEmployeeID,
		ChatObjectiveTemplate: body.ChatObjectiveTemplate,
		ScheduleKind:          body.ScheduleKind,
		CronExpr:              body.CronExpr,
		IntervalSeconds:       body.IntervalSeconds,
		Timezone:              body.Timezone,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ruleResponseFrom(rule))
}

func (h *HTTPHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectDemandSubmit, "automation delete")
	if !ok {
		return
	}
	ruleID, ok := ruleIDFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.service.DeleteRule(r.Context(), tenantID, ruleID, userID); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) EnableRule(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectDemandSubmit, "automation enable")
	if !ok {
		return
	}
	ruleID, ok := ruleIDFromRequest(w, r)
	if !ok {
		return
	}
	rule, err := h.service.Enable(r.Context(), tenantID, ruleID, userID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ruleResponseFrom(rule))
}

func (h *HTTPHandler) DisableRule(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectDemandSubmit, "automation disable")
	if !ok {
		return
	}
	ruleID, ok := ruleIDFromRequest(w, r)
	if !ok {
		return
	}
	rule, err := h.service.Disable(r.Context(), tenantID, ruleID, userID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ruleResponseFrom(rule))
}

func (h *HTTPHandler) TriggerRule(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectDemandSubmit, "automation trigger")
	if !ok {
		return
	}
	ruleID, ok := ruleIDFromRequest(w, r)
	if !ok {
		return
	}
	fire, err := h.service.Trigger(r.Context(), TriggerRequest{
		TenantID:    tenantID,
		RuleID:      ruleID,
		ActorUserID: userID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fireResponseFrom(fire))
}

func (h *HTTPHandler) ListFires(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorize(w, r, authz.ActionProjectDemandRead, "automation fires list")
	if !ok {
		return
	}
	ruleID, ok := ruleIDFromRequest(w, r)
	if !ok {
		return
	}
	fires, err := h.service.ListFires(r.Context(), ListFiresRequest{
		TenantID: tenantID,
		RuleID:   ruleID,
		Limit:    queryInt32(r, "limit", DefaultListLimit),
		Offset:   queryInt32(r, "offset", 0),
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	responses := make([]fireResponse, 0, len(fires))
	for _, fire := range fires {
		responses = append(responses, fireResponseFrom(fire))
	}
	writeJSON(w, http.StatusOK, listFiresResponse{Items: responses})
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action string, auditReason string) (uuid.UUID, uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "automation authorization is not configured", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor:       authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
		Action:      action,
		Resource:    authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()},
		TenantID:    tenantID,
		AuditReason: auditReason,
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

func ruleIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "ruleId")
	id, err := uuid.Parse(raw)
	if err != nil {
		http.Error(w, "invalid ruleId", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func queryInt32(r *http.Request, key string, fallback int32) int32 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return int32(n)
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrProjectModeLocked):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrActorNotEligible):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, ErrConflict):
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

type createRuleBody struct {
	ProjectID             uuid.UUID  `json:"project_id"`
	Name                  string     `json:"name"`
	CoordinationMode      string     `json:"coordination_mode"`
	DemandTitleTemplate   *string    `json:"demand_title_template"`
	DemandBodyTemplate    *string    `json:"demand_body_template"`
	ScenarioTemplateKey   *string    `json:"scenario_template_key"`
	DigitalEmployeeID     *uuid.UUID `json:"digital_employee_id"`
	ChatObjectiveTemplate *string    `json:"chat_objective_template"`
	ScheduleKind          string     `json:"schedule_kind"`
	CronExpr              *string    `json:"cron_expr"`
	IntervalSeconds       *int32     `json:"interval_seconds"`
	Timezone              string     `json:"timezone"`
	Enabled               *bool      `json:"enabled"`
}

type patchRuleBody struct {
	Name                  *string    `json:"name"`
	DemandTitleTemplate   *string    `json:"demand_title_template"`
	DemandBodyTemplate    *string    `json:"demand_body_template"`
	ScenarioTemplateKey   *string    `json:"scenario_template_key"`
	DigitalEmployeeID     *uuid.UUID `json:"digital_employee_id"`
	ChatObjectiveTemplate *string    `json:"chat_objective_template"`
	ScheduleKind          *string    `json:"schedule_kind"`
	CronExpr              *string    `json:"cron_expr"`
	IntervalSeconds       *int32     `json:"interval_seconds"`
	Timezone              *string    `json:"timezone"`
}

type listRulesResponse struct {
	Items []ruleResponse `json:"items"`
}

type listFiresResponse struct {
	Items []fireResponse `json:"items"`
}

type ruleResponse struct {
	ID                      uuid.UUID     `json:"id"`
	TenantID                uuid.UUID     `json:"tenant_id"`
	TeamID                  uuid.UUID     `json:"team_id"`
	ProjectID               uuid.UUID     `json:"project_id"`
	ProjectName             string        `json:"project_name,omitempty"`
	Name                    string        `json:"name"`
	Enabled                 bool          `json:"enabled"`
	CoordinationMode        string        `json:"coordination_mode"`
	DemandTitleTemplate     *string       `json:"demand_title_template,omitempty"`
	DemandBodyTemplate      *string       `json:"demand_body_template,omitempty"`
	ScenarioTemplateKey     *string       `json:"scenario_template_key,omitempty"`
	DigitalEmployeeID       *uuid.UUID    `json:"digital_employee_id,omitempty"`
	ChatObjectiveTemplate   *string       `json:"chat_objective_template,omitempty"`
	ScheduleKind            string        `json:"schedule_kind"`
	CronExpr                *string       `json:"cron_expr,omitempty"`
	IntervalSeconds         *int32        `json:"interval_seconds,omitempty"`
	Timezone                string        `json:"timezone"`
	OverlapPolicy           string        `json:"overlap_policy"`
	ActorUserID             uuid.UUID     `json:"actor_user_id"`
	DisabledReason          *string       `json:"disabled_reason,omitempty"`
	ConsecutiveFailureCount int32         `json:"consecutive_failure_count"`
	TemporalScheduleID      *string       `json:"temporal_schedule_id,omitempty"`
	CreatedAt               time.Time     `json:"created_at"`
	UpdatedAt               time.Time     `json:"updated_at"`
	LatestFire              *fireResponse `json:"latest_fire,omitempty"`
}

type fireResponse struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        uuid.UUID  `json:"tenant_id"`
	RuleID          uuid.UUID  `json:"rule_id"`
	ScheduledFireAt time.Time  `json:"scheduled_fire_at"`
	IdempotencyKey  string     `json:"idempotency_key"`
	Status          string     `json:"status"`
	DemandID        *uuid.UUID `json:"demand_id,omitempty"`
	RunID           *uuid.UUID `json:"run_id,omitempty"`
	ErrorCode       *string    `json:"error_code,omitempty"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func ruleResponseFrom(rule Rule) ruleResponse {
	resp := ruleResponse{
		ID:                      rule.ID,
		TenantID:                rule.TenantID,
		TeamID:                  rule.TeamID,
		ProjectID:               rule.ProjectID,
		ProjectName:             rule.ProjectName,
		Name:                    rule.Name,
		Enabled:                 rule.Enabled,
		CoordinationMode:        rule.CoordinationMode,
		DemandTitleTemplate:     rule.DemandTitleTemplate,
		DemandBodyTemplate:      rule.DemandBodyTemplate,
		ScenarioTemplateKey:     rule.ScenarioTemplateKey,
		DigitalEmployeeID:       rule.DigitalEmployeeID,
		ChatObjectiveTemplate:   rule.ChatObjectiveTemplate,
		ScheduleKind:            rule.ScheduleKind,
		CronExpr:                rule.CronExpr,
		IntervalSeconds:         rule.IntervalSeconds,
		Timezone:                rule.Timezone,
		OverlapPolicy:           rule.OverlapPolicy,
		ActorUserID:             rule.ActorUserID,
		DisabledReason:          rule.DisabledReason,
		ConsecutiveFailureCount: rule.ConsecutiveFailureCount,
		TemporalScheduleID:      rule.TemporalScheduleID,
		CreatedAt:               rule.CreatedAt,
		UpdatedAt:               rule.UpdatedAt,
	}
	if rule.LatestFire != nil {
		fire := fireResponseFrom(*rule.LatestFire)
		resp.LatestFire = &fire
	}
	return resp
}

func fireResponseFrom(fire Fire) fireResponse {
	return fireResponse{
		ID:              fire.ID,
		TenantID:        fire.TenantID,
		RuleID:          fire.RuleID,
		ScheduledFireAt: fire.ScheduledFireAt,
		IdempotencyKey:  fire.IdempotencyKey,
		Status:          fire.Status,
		DemandID:        fire.DemandID,
		RunID:           fire.RunID,
		ErrorCode:       fire.ErrorCode,
		ErrorMessage:    fire.ErrorMessage,
		CreatedAt:       fire.CreatedAt,
	}
}
