package permission

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/approval"
)

// HTTPHandler serves the 权限中心「权限审批」read + decision endpoints. It reads the
// approval domain directly (category=permission), never via the inbox.
type HTTPHandler struct {
	service  *Service
	producer *PrivilegedRoleProducer
}

func NewHandler(service *Service, producer *PrivilegedRoleProducer) *HTTPHandler {
	return &HTTPHandler{service: service, producer: producer}
}

// ListApprovals: GET /api/v1/permission-approvals
func (h *HTTPHandler) ListApprovals(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	limit, ok := int32QueryParam(w, r, "limit")
	if !ok {
		return
	}
	offset, ok := int32QueryParam(w, r, "offset")
	if !ok {
		return
	}
	q := r.URL.Query()
	views, summary, hasMore, err := h.service.List(r.Context(), ListInput{
		TenantID:     tenantID,
		ActorUserID:  actorID,
		View:         q.Get("view"),
		Status:       q.Get("status"),
		RiskLevel:    q.Get("risk_level"),
		ResourceType: q.Get("resource_type"),
		Limit:        limit,
		Offset:       offset,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	if limit <= 0 {
		limit = 50
	}
	items := make([]approvalResponse, 0, len(views))
	for _, v := range views {
		items = append(items, responseFromView(v))
	}
	writeJSON(w, http.StatusOK, listResponse{
		Items: items,
		Pagination: paginationResponse{
			Limit:   limit,
			Offset:  offset,
			HasMore: hasMore,
		},
		Summary: summaryResponse{
			OpenCount:     summary.OpenCount,
			HighRiskCount: summary.HighRiskCount,
			BlockedCount:  summary.BlockedCount,
		},
	})
}

// Decide: POST /api/v1/permission-approvals/{id}/decision
func (h *HTTPHandler) Decide(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || id == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid permission approval id")
		return
	}
	var body decisionBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	view, err := h.service.Decide(r.Context(), DecideInput{
		TenantID:     tenantID,
		ApprovalID:   id,
		DecidedBy:    actorID,
		Decision:     body.Decision,
		Note:         body.Note,
		EvidenceRefs: body.EvidenceRefs,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, responseFromView(*view))
}

// RequestPrivilegedRole: POST /api/v1/teams/{teamId}/privileged-role-requests (S2)
func (h *HTTPHandler) RequestPrivilegedRole(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	if h.producer == nil {
		writeJSONError(w, http.StatusNotImplemented, "privileged role requests unavailable")
		return
	}
	teamID, err := uuid.Parse(chi.URLParam(r, "teamId"))
	if err != nil || teamID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid team id")
		return
	}
	var body requestPrivilegedRoleBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	targetUserID, err := uuid.Parse(body.TargetUserID)
	if err != nil || targetUserID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid target_user_id")
		return
	}
	view, err := h.producer.Request(r.Context(), PrivilegedRoleRequestInput{
		TenantID:      tenantID,
		TeamID:        teamID,
		RequestedBy:   actorID,
		TargetUserID:  targetUserID,
		RequestedRole: body.RequestedRole,
		Reason:        body.Reason,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, responseFromView(*view))
}

// --- request/response DTOs (match contract schemas) ---

type decisionBody struct {
	Decision     string   `json:"decision"`
	Note         string   `json:"note"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type requestPrivilegedRoleBody struct {
	TargetUserID  string `json:"target_user_id"`
	RequestedRole string `json:"requested_role"`
	Reason        string `json:"reason"`
}

type actionResponse struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Tone  string `json:"tone,omitempty"`
}

type approvalResponse struct {
	ID            string           `json:"id"`
	TenantID      string           `json:"tenant_id"`
	Category      string           `json:"category"`
	ResourceType  string           `json:"resource_type"`
	ResourceID    string           `json:"resource_id"`
	RequesterType string           `json:"requester_type"`
	RequesterID   *string          `json:"requester_id,omitempty"`
	RequesterName string           `json:"requester_name,omitempty"`
	TargetUserID  *string          `json:"target_user_id,omitempty"`
	DecisionType  string           `json:"decision_type"`
	Title         string           `json:"title"`
	Summary       string           `json:"summary,omitempty"`
	RiskLevel     string           `json:"risk_level,omitempty"`
	Status        string           `json:"status"`
	Actions       []actionResponse `json:"actions"`
	Context       map[string]any   `json:"context"`
	CreatedAt     string           `json:"created_at"`
	UpdatedAt     string           `json:"updated_at"`
	ResolvedAt    *string          `json:"resolved_at,omitempty"`
}

type listResponse struct {
	Items      []approvalResponse `json:"items"`
	Pagination paginationResponse `json:"pagination"`
	Summary    summaryResponse    `json:"summary"`
}

type paginationResponse struct {
	Limit   int32 `json:"limit"`
	Offset  int32 `json:"offset"`
	HasMore bool  `json:"has_more"`
}

type summaryResponse struct {
	OpenCount     int64 `json:"open_count"`
	HighRiskCount int64 `json:"high_risk_count"`
	BlockedCount  int64 `json:"blocked_count"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func responseFromView(v View) approvalResponse {
	req := v.Request
	actions := make([]actionResponse, 0, len(v.Actions))
	for _, a := range v.Actions {
		actions = append(actions, actionResponse{Key: a.Key, Label: a.Label, Tone: a.Tone})
	}
	ctx := req.ContextPayload
	if ctx == nil {
		ctx = map[string]any{}
	}
	resp := approvalResponse{
		ID:            req.ID.String(),
		TenantID:      req.TenantID.String(),
		Category:      string(req.Category),
		ResourceType:  req.ResourceType,
		ResourceID:    req.ResourceID.String(),
		RequesterType: req.RequesterType,
		RequesterName: v.RequesterName,
		DecisionType:  req.DecisionType,
		Title:         req.Title,
		Status:        string(req.Status),
		Actions:       actions,
		Context:       ctx,
		CreatedAt:     req.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     req.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if req.RequesterID != nil && *req.RequesterID != uuid.Nil {
		s := req.RequesterID.String()
		resp.RequesterID = &s
	}
	if req.TargetUserID != uuid.Nil {
		s := req.TargetUserID.String()
		resp.TargetUserID = &s
	}
	if req.Summary != nil {
		resp.Summary = *req.Summary
	}
	if req.RiskLevel != nil {
		resp.RiskLevel = *req.RiskLevel
	}
	if req.ResolvedAt != nil {
		s := req.ResolvedAt.UTC().Format(time.RFC3339)
		resp.ResolvedAt = &s
	}
	return resp
}

// --- shared helpers (mirrors the inbox handler conventions) ---

func consoleIdentity(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		writeJSONError(w, http.StatusForbidden, "console identity not found in context")
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

func int32QueryParam(w http.ResponseWriter, r *http.Request, name string) (int32, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, name+" must be an integer")
		return 0, false
	}
	return int32(value), true
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrInvalidDecision):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrAlreadyResolved):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNoApprover):
		writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, approval.ErrInvalidApprovalRequest):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
