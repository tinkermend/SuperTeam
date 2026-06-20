package teamlending

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

// HandlerService 借调 HTTP 处理依赖的服务接口。
type HandlerService interface {
	GetPolicy(ctx context.Context, tenantID, teamID uuid.UUID) (*Policy, error)
	UpsertPolicy(ctx context.Context, input UpsertPolicyInput) (*Policy, error)
	ListRequestsByTeam(ctx context.Context, tenantID, teamID uuid.UUID, status RequestStatus, limit, offset int32) ([]*Request, error)
	ListRequestsByProject(ctx context.Context, tenantID, projectID uuid.UUID, status RequestStatus, limit, offset int32) ([]*Request, error)
	CreateRequest(ctx context.Context, input CreateRequestInput) (*Request, error)
	ApproveRequest(ctx context.Context, input DecideRequestInput) (*Request, error)
	RejectRequest(ctx context.Context, input DecideRequestInput) (*Request, error)
	RevokeRequest(ctx context.Context, input DecideRequestInput) (*Request, error)
}

// HTTPHandler 团队借调 HTTP 处理器。
type HTTPHandler struct {
	service    HandlerService
	authorizer authz.Authorizer
}

// NewHandler 构造借调 HTTP 处理器。
func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

// SetAuthorizer 注入授权器（团队侧策略/裁决动作鉴权）。
func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

// GetLendingPolicy GET /api/v1/teams/{teamId}/lending-policy
func (h *HTTPHandler) GetLendingPolicy(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamLendingPolicyRead, "team lending policy read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	policy, err := service.GetPolicy(r.Context(), tenantID, teamID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policyResponseFromDomain(policy))
}

// UpsertLendingPolicy PUT /api/v1/teams/{teamId}/lending-policy
func (h *HTTPHandler) UpsertLendingPolicy(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		AllowLending      bool           `json:"allow_lending"`
		ApprovalMode      string         `json:"approval_mode"`
		BudgetCeiling     string         `json:"budget_ceiling"`
		CapabilityCeiling map[string]any `json:"capability_ceiling"`
		ProjectMatch      map[string]any `json:"project_match"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamLendingPolicyEdit, "team lending policy update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	policy, err := service.UpsertPolicy(r.Context(), UpsertPolicyInput{
		TenantID:          tenantID,
		TeamID:            teamID,
		ActorUserID:       middleware.GetUserID(r.Context()),
		AllowLending:      req.AllowLending,
		ApprovalMode:      ApprovalMode(req.ApprovalMode),
		BudgetCeiling:     req.BudgetCeiling,
		CapabilityCeiling: req.CapabilityCeiling,
		ProjectMatch:      req.ProjectMatch,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policyResponseFromDomain(policy))
}

// ListTeamLendingRequests GET /api/v1/teams/{teamId}/lending-requests
func (h *HTTPHandler) ListTeamLendingRequests(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamLendingRequestRead, "team lending requests read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	limit, ok := nonNegativeInt32QueryParam(w, r, "limit")
	if !ok {
		return
	}
	offset, ok := nonNegativeInt32QueryParam(w, r, "offset")
	if !ok {
		return
	}
	requests, err := service.ListRequestsByTeam(r.Context(), tenantID, teamID, RequestStatus(r.URL.Query().Get("status")), limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lendingRequestResponses(requests))
}

// decideLendingRequest 处理 approve/reject/revoke（POST .../lending-requests/{requestId}/{decision}）。
func (h *HTTPHandler) decideLendingRequest(w http.ResponseWriter, r *http.Request, decide func(ctx context.Context, service HandlerService, input DecideRequestInput) (*Request, error)) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	requestID, ok := lendingRequestIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		DecisionReason    string         `json:"decision_reason"`
		GrantedBudget     string         `json:"granted_budget"`
		GrantedCapability map[string]any `json:"granted_capability"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamLendingRequestDecide, "team lending request decide")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	request, err := decide(r.Context(), service, DecideRequestInput{
		TenantID:          tenantID,
		TeamID:            teamID,
		RequestID:         requestID,
		DecidedByUserID:   middleware.GetUserID(r.Context()),
		DecisionReason:    req.DecisionReason,
		GrantedBudget:     req.GrantedBudget,
		GrantedCapability: req.GrantedCapability,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lendingRequestResponseFromDomain(request))
}

// ApproveLendingRequest POST /api/v1/teams/{teamId}/lending-requests/{requestId}/approve
func (h *HTTPHandler) ApproveLendingRequest(w http.ResponseWriter, r *http.Request) {
	h.decideLendingRequest(w, r, func(ctx context.Context, service HandlerService, input DecideRequestInput) (*Request, error) {
		return service.ApproveRequest(ctx, input)
	})
}

// RejectLendingRequest POST /api/v1/teams/{teamId}/lending-requests/{requestId}/reject
func (h *HTTPHandler) RejectLendingRequest(w http.ResponseWriter, r *http.Request) {
	h.decideLendingRequest(w, r, func(ctx context.Context, service HandlerService, input DecideRequestInput) (*Request, error) {
		return service.RejectRequest(ctx, input)
	})
}

// RevokeLendingRequest POST /api/v1/teams/{teamId}/lending-requests/{requestId}/revoke
func (h *HTTPHandler) RevokeLendingRequest(w http.ResponseWriter, r *http.Request) {
	h.decideLendingRequest(w, r, func(ctx context.Context, service HandlerService, input DecideRequestInput) (*Request, error) {
		return service.RevokeRequest(ctx, input)
	})
}

// CreateProjectLendingRequest POST /api/v1/projects/{projectId}/lending-requests（需求侧发起）。
func (h *HTTPHandler) CreateProjectLendingRequest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, userID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	var req struct {
		TeamID              uuid.UUID      `json:"team_id"`
		RequestReason       string         `json:"request_reason"`
		RequestedBudget     string         `json:"requested_budget"`
		RequestedCapability map[string]any `json:"requested_capability"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.TeamID == uuid.Nil {
		http.Error(w, "team_id is required", http.StatusBadRequest)
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	request, err := service.CreateRequest(r.Context(), CreateRequestInput{
		TenantID:            tenantID,
		TeamID:              req.TeamID,
		ProjectID:           projectID,
		RequestedByUserID:   userID,
		RequestReason:       req.RequestReason,
		RequestedBudget:     req.RequestedBudget,
		RequestedCapability: req.RequestedCapability,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, lendingRequestResponseFromDomain(request))
}

// ListProjectLendingRequests GET /api/v1/projects/{projectId}/lending-requests（需求侧视角）。
func (h *HTTPHandler) ListProjectLendingRequests(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, _, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	limit, ok := nonNegativeInt32QueryParam(w, r, "limit")
	if !ok {
		return
	}
	offset, ok := nonNegativeInt32QueryParam(w, r, "offset")
	if !ok {
		return
	}
	requests, err := service.ListRequestsByProject(r.Context(), tenantID, projectID, RequestStatus(r.URL.Query().Get("status")), limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lendingRequestResponses(requests))
}

// ---- 鉴权 / 参数辅助 ----

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "team lending service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func (h *HTTPHandler) authorizeTeamAction(w http.ResponseWriter, r *http.Request, teamID uuid.UUID, action, auditReason string) (uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "team authorization is not configured", http.StatusForbidden)
		return uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, false
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor:       authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
		Action:      action,
		Resource:    authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()},
		TenantID:    tenantID,
		TeamID:      &teamID,
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

func consoleIdentity(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

func teamIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	teamID, err := uuid.Parse(chi.URLParam(r, "teamId"))
	if err != nil || teamID == uuid.Nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return teamID, true
}

func projectIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectId"))
	if err != nil || projectID == uuid.Nil {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return projectID, true
}

func lendingRequestIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	requestID, err := uuid.Parse(chi.URLParam(r, "requestId"))
	if err != nil || requestID == uuid.Nil {
		http.Error(w, "invalid request id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return requestID, true
}

func nonNegativeInt32QueryParam(w http.ResponseWriter, r *http.Request, name string) (int32, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || parsed < 0 {
		http.Error(w, "invalid "+name, http.StatusBadRequest)
		return 0, false
	}
	return int32(parsed), true
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrDuplicateRequest), errors.Is(err, ErrInvalidTransition):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrPolicyNotFound), errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ---- 响应映射 ----

type policyResponse struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenant_id"`
	TeamID            string         `json:"team_id"`
	AllowLending      bool           `json:"allow_lending"`
	ApprovalMode      string         `json:"approval_mode"`
	BudgetCeiling     string         `json:"budget_ceiling,omitempty"`
	CapabilityCeiling map[string]any `json:"capability_ceiling"`
	ProjectMatch      map[string]any `json:"project_match"`
	Status            string         `json:"status"`
	CreatedByUserID   string         `json:"created_by_user_id,omitempty"`
	UpdatedByUserID   string         `json:"updated_by_user_id,omitempty"`
	CreatedAt         string         `json:"created_at,omitempty"`
	UpdatedAt         string         `json:"updated_at,omitempty"`
}

type lendingRequestResponse struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	TeamID              string         `json:"team_id"`
	ProjectID           string         `json:"project_id"`
	Status              string         `json:"status"`
	RequestedByUserID   string         `json:"requested_by_user_id"`
	RequestReason       string         `json:"request_reason"`
	RequestedBudget     string         `json:"requested_budget,omitempty"`
	RequestedCapability map[string]any `json:"requested_capability"`
	GrantedBudget       string         `json:"granted_budget,omitempty"`
	GrantedCapability   map[string]any `json:"granted_capability"`
	IsException         bool           `json:"is_exception"`
	DecidedByUserID     string         `json:"decided_by_user_id,omitempty"`
	DecidedAt           string         `json:"decided_at,omitempty"`
	DecisionReason      string         `json:"decision_reason,omitempty"`
	CreatedAt           string         `json:"created_at,omitempty"`
	UpdatedAt           string         `json:"updated_at,omitempty"`
}

func policyResponseFromDomain(policy *Policy) policyResponse {
	resp := policyResponse{
		ID:                policy.ID.String(),
		TenantID:          policy.TenantID.String(),
		TeamID:            policy.TeamID.String(),
		AllowLending:      policy.AllowLending,
		ApprovalMode:      string(policy.ApprovalMode),
		BudgetCeiling:     policy.BudgetCeiling,
		CapabilityCeiling: policy.CapabilityCeiling,
		ProjectMatch:      policy.ProjectMatch,
		Status:            policy.Status,
		CreatedAt:         formatTime(policy.CreatedAt),
		UpdatedAt:         formatTime(policy.UpdatedAt),
	}
	if policy.CreatedByUserID != nil {
		resp.CreatedByUserID = policy.CreatedByUserID.String()
	}
	if policy.UpdatedByUserID != nil {
		resp.UpdatedByUserID = policy.UpdatedByUserID.String()
	}
	return resp
}

func lendingRequestResponseFromDomain(request *Request) lendingRequestResponse {
	resp := lendingRequestResponse{
		ID:                  request.ID.String(),
		TenantID:            request.TenantID.String(),
		TeamID:              request.TeamID.String(),
		ProjectID:           request.ProjectID.String(),
		Status:              string(request.Status),
		RequestedByUserID:   request.RequestedByUserID.String(),
		RequestReason:       request.RequestReason,
		RequestedBudget:     request.RequestedBudget,
		RequestedCapability: request.RequestedCapability,
		GrantedBudget:       request.GrantedBudget,
		GrantedCapability:   request.GrantedCapability,
		IsException:         request.IsException,
		DecisionReason:      request.DecisionReason,
		CreatedAt:           formatTime(request.CreatedAt),
		UpdatedAt:           formatTime(request.UpdatedAt),
	}
	if request.DecidedByUserID != nil {
		resp.DecidedByUserID = request.DecidedByUserID.String()
	}
	if request.DecidedAt != nil {
		resp.DecidedAt = formatTime(*request.DecidedAt)
	}
	return resp
}

func lendingRequestResponses(requests []*Request) []lendingRequestResponse {
	responses := make([]lendingRequestResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, lendingRequestResponseFromDomain(request))
	}
	return responses
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
