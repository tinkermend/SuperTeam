package tenant

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
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error)
	ListTeams(ctx context.Context, req ListTeamsRequest) ([]*Team, error)
	GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (*Team, error)
	CreateConfigRevision(ctx context.Context, req CreateTeamConfigRevisionRequest) (*TeamConfigRevision, error)
	GetCurrentConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamConfigRevision, error)
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

func (h *HTTPHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeTenantTeamAction(w, r, authz.ActionTeamRead, "team list")
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
	status := TeamStatus(r.URL.Query().Get("status"))

	teams, err := service.ListTeams(r.Context(), ListTeamsRequest{
		TenantID: tenantID,
		Status:   status,
		Offset:   offset,
		Limit:    limit,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamResponses(teams))
}

func (h *HTTPHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeTenantTeamAction(w, r, authz.ActionTeamCreate, "team create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Slug             string         `json:"slug"`
		Name             string         `json:"name"`
		Status           TeamStatus     `json:"status"`
		HumanOwnerUserID *uuid.UUID     `json:"human_owner_user_id"`
		Metadata         map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	team, err := service.CreateTeam(r.Context(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             req.Slug,
		Name:             req.Name,
		Status:           req.Status,
		HumanOwnerUserID: req.HumanOwnerUserID,
		Metadata:         req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, teamResponseFromDomain(team))
}

func (h *HTTPHandler) GetTeam(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamRead, "team read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	team, err := service.GetTeam(r.Context(), tenantID, teamID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamResponseFromDomain(team))
}

func (h *HTTPHandler) CreateTeamConfigRevision(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		Constitution                map[string]any           `json:"constitution"`
		CapabilityPolicy            map[string]any           `json:"capability_policy"`
		ContextPolicy               map[string]any           `json:"context_policy"`
		ApprovalPolicy              map[string]any           `json:"approval_policy"`
		ArtifactContract            map[string]any           `json:"artifact_contract"`
		InternalCollaborationPolicy map[string]any           `json:"internal_collaboration_policy"`
		RuntimeScopePolicy          map[string]any           `json:"runtime_scope_policy"`
		HumanOwnerUserID            *uuid.UUID               `json:"human_owner_user_id"`
		Status                      TeamConfigRevisionStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := authz.ActionTeamGovernanceApprove
	if req.Status == TeamConfigRevisionStatusDraft {
		action = authz.ActionTeamGovernanceEdit
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, action, "team config revision create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	approvedBy := middleware.GetUserID(r.Context())
	revision, err := service.CreateConfigRevision(r.Context(), CreateTeamConfigRevisionRequest{
		TenantID:                    tenantID,
		TeamID:                      teamID,
		Constitution:                req.Constitution,
		CapabilityPolicy:            req.CapabilityPolicy,
		ContextPolicy:               req.ContextPolicy,
		ApprovalPolicy:              req.ApprovalPolicy,
		ArtifactContract:            req.ArtifactContract,
		InternalCollaborationPolicy: req.InternalCollaborationPolicy,
		RuntimeScopePolicy:          req.RuntimeScopePolicy,
		HumanOwnerUserID:            req.HumanOwnerUserID,
		Status:                      req.Status,
		ApprovedBy:                  &approvedBy,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, configRevisionResponseFromDomain(revision))
}

func (h *HTTPHandler) GetCurrentTeamConfigRevision(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamGovernanceRead, "team config revision current read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	revision, err := service.GetCurrentConfigRevision(r.Context(), tenantID, teamID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configRevisionResponseFromDomain(revision))
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "tenant service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func (h *HTTPHandler) authorizeTenantTeamAction(w http.ResponseWriter, r *http.Request, action, auditReason string) (uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	return h.authorizeTeamRequest(w, r, action, authz.ResourceRef{
		Type: authz.ResourceTenant,
		ID:   tenantID.String(),
	}, nil, auditReason)
}

func (h *HTTPHandler) authorizeTeamAction(w http.ResponseWriter, r *http.Request, teamID uuid.UUID, action, auditReason string) (uuid.UUID, bool) {
	return h.authorizeTeamRequest(w, r, action, authz.ResourceRef{
		Type: authz.ResourceTeam,
		ID:   teamID.String(),
	}, &teamID, auditReason)
}

func (h *HTTPHandler) authorizeTeamRequest(w http.ResponseWriter, r *http.Request, action string, resource authz.ResourceRef, teamID *uuid.UUID, auditReason string) (uuid.UUID, bool) {
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
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   userID.String(),
		},
		Action:      action,
		Resource:    resource,
		TenantID:    tenantID,
		TeamID:      teamID,
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

type teamResponse struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenant_id"`
	Slug             string         `json:"slug"`
	Name             string         `json:"name"`
	Status           TeamStatus     `json:"status"`
	HumanOwnerUserID *string        `json:"human_owner_user_id,omitempty"`
	Metadata         map[string]any `json:"metadata"`
	CreatedAt        string         `json:"created_at,omitempty"`
	UpdatedAt        string         `json:"updated_at,omitempty"`
}

type configRevisionResponse struct {
	ID                          string                   `json:"id"`
	TenantID                    string                   `json:"tenant_id"`
	TeamID                      string                   `json:"team_id"`
	RevisionNumber              int32                    `json:"revision_number"`
	Constitution                map[string]any           `json:"constitution"`
	CapabilityPolicy            map[string]any           `json:"capability_policy"`
	ContextPolicy               map[string]any           `json:"context_policy"`
	ApprovalPolicy              map[string]any           `json:"approval_policy"`
	ArtifactContract            map[string]any           `json:"artifact_contract"`
	InternalCollaborationPolicy map[string]any           `json:"internal_collaboration_policy"`
	RuntimeScopePolicy          map[string]any           `json:"runtime_scope_policy"`
	HumanOwnerUserID            *string                  `json:"human_owner_user_id,omitempty"`
	Status                      TeamConfigRevisionStatus `json:"status"`
	ApprovedBy                  *string                  `json:"approved_by,omitempty"`
	ApprovedAt                  *string                  `json:"approved_at,omitempty"`
	CreatedAt                   string                   `json:"created_at,omitempty"`
	UpdatedAt                   string                   `json:"updated_at,omitempty"`
}

func teamIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	teamID, err := uuid.Parse(chi.URLParam(r, "teamId"))
	if err != nil || teamID == uuid.Nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return teamID, true
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func teamResponses(teams []*Team) []teamResponse {
	responses := make([]teamResponse, 0, len(teams))
	for _, team := range teams {
		responses = append(responses, teamResponseFromDomain(team))
	}
	return responses
}

func teamResponseFromDomain(team *Team) teamResponse {
	return teamResponse{
		ID:               team.ID.String(),
		TenantID:         team.TenantID.String(),
		Slug:             team.Slug,
		Name:             team.Name,
		Status:           team.Status,
		HumanOwnerUserID: uuidStringPtr(team.HumanOwnerUserID),
		Metadata:         cloneMap(team.Metadata),
		CreatedAt:        timeString(team.CreatedAt),
		UpdatedAt:        timeString(team.UpdatedAt),
	}
}

func configRevisionResponseFromDomain(revision *TeamConfigRevision) configRevisionResponse {
	return configRevisionResponse{
		ID:                          revision.ID.String(),
		TenantID:                    revision.TenantID.String(),
		TeamID:                      revision.TeamID.String(),
		RevisionNumber:              revision.RevisionNumber,
		Constitution:                cloneMap(revision.Constitution),
		CapabilityPolicy:            cloneMap(revision.CapabilityPolicy),
		ContextPolicy:               cloneMap(revision.ContextPolicy),
		ApprovalPolicy:              cloneMap(revision.ApprovalPolicy),
		ArtifactContract:            cloneMap(revision.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(revision.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(revision.RuntimeScopePolicy),
		HumanOwnerUserID:            uuidStringPtr(revision.HumanOwnerUserID),
		Status:                      revision.Status,
		ApprovedBy:                  uuidStringPtr(revision.ApprovedBy),
		ApprovedAt:                  timeStringPtr(revision.ApprovedAt),
		CreatedAt:                   timeString(revision.CreatedAt),
		UpdatedAt:                   timeString(revision.UpdatedAt),
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
