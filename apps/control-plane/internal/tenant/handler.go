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
	"github.com/superteam/control-plane/internal/audit"
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	CreateTeam(ctx context.Context, req CreateTeamRequest) (*TeamOverview, error)
	ListTeamSummaries(ctx context.Context, req ListTeamsRequest) ([]*TeamListItem, error)
	GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (*Team, error)
	GetOverview(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamOverview, error)
	UpdateTeam(ctx context.Context, req UpdateTeamRequest) (*Team, error)
	UpdateTeamConstitution(ctx context.Context, tenantID, teamID uuid.UUID, constitution map[string]any) (*Team, error)
	DeleteTeam(ctx context.Context, req DeleteTeamRequest) error
	ListPendingDeleteTeams(ctx context.Context, tenantID uuid.UUID) ([]PendingDeleteTeamRecord, error)
	RestorePendingDeleteTeam(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) (*Team, error)
	ConfirmTeamDelete(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) error
	ListTeamMembers(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*TeamMember, error)
	AddTeamMember(ctx context.Context, req AddTeamMemberRequest) (*TeamMember, error)
	BindTeamDigitalEmployee(ctx context.Context, req BindTeamDigitalEmployeeRequest) error
	RemoveTeamMember(ctx context.Context, req RemoveTeamMemberRequest) error
	ListTeamAuditEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*audit.Event, error)
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
	governanceStatus := GovernanceSummaryStatus(r.URL.Query().Get("governance_status"))
	q := r.URL.Query().Get("q")

	teams, err := service.ListTeamSummaries(r.Context(), ListTeamsRequest{
		TenantID:         tenantID,
		Status:           status,
		GovernanceStatus: governanceStatus,
		Q:                q,
		Offset:           offset,
		Limit:            limit,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamListItemResponses(teams))
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
		Slug                      string                   `json:"slug"`
		Name                      string                   `json:"name"`
		Description               string                   `json:"description"`
		Status                    TeamStatus               `json:"status"`
		HumanOwnerUserIDs         []uuid.UUID              `json:"human_owner_user_ids,omitempty"`
		InitialMembers            []InitialTeamMemberInput `json:"initial_members"`
		InitialDigitalEmployeeIDs []uuid.UUID              `json:"initial_digital_employee_ids"`
		Metadata                  map[string]any           `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	overview, err := service.CreateTeam(r.Context(), CreateTeamRequest{
		TenantID:                  tenantID,
		ActorUserID:               middleware.GetUserID(r.Context()),
		Slug:                      req.Slug,
		Name:                      req.Name,
		Description:               req.Description,
		Status:                    req.Status,
		HumanOwnerUserIDs:         req.HumanOwnerUserIDs,
		InitialMembers:            req.InitialMembers,
		InitialDigitalEmployeeIDs: req.InitialDigitalEmployeeIDs,
		Metadata:                  req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	overview.AllowedActions = h.allowedTeamActions(r, tenantID, overview.Team.ID)
	writeJSON(w, http.StatusCreated, teamOverviewResponseFromDomain(overview))
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

func (h *HTTPHandler) GetTeamOverview(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamRead, "team overview read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	overview, err := service.GetOverview(r.Context(), tenantID, teamID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	overview.AllowedActions = h.allowedTeamActions(r, tenantID, teamID)
	writeJSON(w, http.StatusOK, teamOverviewResponseFromDomain(overview))
}

func (h *HTTPHandler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamUpdate, "team update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Slug              string         `json:"slug"`
		Name              string         `json:"name"`
		Description       *string        `json:"description,omitempty"`
		HumanOwnerUserIDs []uuid.UUID    `json:"human_owner_user_ids,omitempty"`
		Metadata          map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	team, err := service.UpdateTeam(r.Context(), UpdateTeamRequest{
		TenantID:          tenantID,
		TeamID:            teamID,
		Slug:              req.Slug,
		Name:              req.Name,
		Description:       req.Description,
		HumanOwnerUserIDs: req.HumanOwnerUserIDs,
		Metadata:          req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamResponseFromDomain(team))
}

func (h *HTTPHandler) UpdateTeamConstitution(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamGovernanceEdit, "team constitution update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var constitution map[string]any
	if err := json.NewDecoder(r.Body).Decode(&constitution); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	team, err := service.UpdateTeamConstitution(r.Context(), tenantID, teamID, constitution)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamResponseFromDomain(team))
}

// 待确认删除队列:读与团队列表同权(tenant 级 team.read);恢复/确认两个决策动作
// 走 team 级 team.delete(与删除同权)。spec 2026-07-18-team-lifecycle-convergence §2。
func (h *HTTPHandler) ListPendingDeleteTeams(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeTenantTeamAction(w, r, authz.ActionTeamRead, "team pending delete list")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	records, err := service.ListPendingDeleteTeams(r.Context(), tenantID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pendingDeleteTeamResponses(records))
}

func (h *HTTPHandler) RestorePendingDeleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamDelete, "team pending delete restore")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	team, err := service.RestorePendingDeleteTeam(r.Context(), tenantID, teamID, middleware.GetUserID(r.Context()))
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamResponseFromDomain(team))
}

func (h *HTTPHandler) ConfirmTeamDelete(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamDelete, "team pending delete confirm")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.ConfirmTeamDelete(r.Context(), tenantID, teamID, middleware.GetUserID(r.Context())); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamDelete, "team delete")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.DeleteTeam(r.Context(), DeleteTeamRequest{
		TenantID:    tenantID,
		TeamID:      teamID,
		ActorUserID: middleware.GetUserID(r.Context()),
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) ListTeamMembers(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamRead, "team members read")
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
	members, err := service.ListTeamMembers(r.Context(), tenantID, teamID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamMemberResponses(members))
}

func (h *HTTPHandler) AddTeamMember(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		UserID uuid.UUID `json:"user_id"`
		Role   string    `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeTeamActionWithContext(w, r, teamID, authz.ActionTeamMemberAdd, "team member add", map[string]any{"target_role": req.Role})
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	member, err := service.AddTeamMember(r.Context(), AddTeamMemberRequest{
		TenantID: tenantID,
		TeamID:   teamID,
		UserID:   req.UserID,
		Role:     req.Role,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, teamMemberResponseFromDomain(member))
}

// BindTeamDigitalEmployee 收编候岗数字员工进本团队（POST /teams/{teamId}/digital-employees）。
func (h *HTTPHandler) BindTeamDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		DigitalEmployeeID uuid.UUID `json:"digital_employee_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamUpdate, "team digital employee bind")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.BindTeamDigitalEmployee(r.Context(), BindTeamDigitalEmployeeRequest{
		TenantID:    tenantID,
		TeamID:      teamID,
		EmployeeID:  req.DigitalEmployeeID,
		ActorUserID: middleware.GetUserID(r.Context()),
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"digital_employee_id": req.DigitalEmployeeID.String(),
		"team_id":             teamID.String(),
	})
}

func (h *HTTPHandler) RemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	membershipID, ok := memberIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamMemberRemove, "team member remove")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.RemoveTeamMember(r.Context(), RemoveTeamMemberRequest{
		TenantID:     tenantID,
		TeamID:       teamID,
		MembershipID: membershipID,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) ListTeamAudit(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamAuditRead, "team audit read")
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
	events, err := service.ListTeamAuditEvents(r.Context(), tenantID, teamID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamAuditEventResponses(events))
}

var overviewActions = []string{
	authz.ActionTeamUpdate,
	authz.ActionTeamDelete,
	authz.ActionTeamMemberAdd,
	authz.ActionTeamMemberRemove,
	authz.ActionTeamMemberRequestPrivilegedRole,
	authz.ActionTeamGovernanceEdit,
	authz.ActionTeamGovernanceApprove,
	authz.ActionTeamCapabilityBind,
	authz.ActionTeamCapabilityUnbind,
	authz.ActionTeamAuditRead,
}


func (h *HTTPHandler) allowedTeamActions(r *http.Request, tenantID, teamID uuid.UUID) []AllowedTeamAction {
	if h == nil || h.authorizer == nil {
		return []AllowedTeamAction{}
	}
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil || teamID == uuid.Nil {
		return []AllowedTeamAction{}
	}
	actions, err := h.authorizer.CheckBulkTeamActions(r.Context(), authz.BulkTeamActionsRequest{
		Actor:    authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
		TenantID: tenantID,
		TeamID:   teamID,
		Actions:  overviewActions,
	})
	if err != nil {
		return []AllowedTeamAction{}
	}
	allowed := make([]AllowedTeamAction, 0, len(actions))
	for _, a := range actions {
		allowed = append(allowed, AllowedTeamAction(a))
	}
	return allowed
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
	return h.authorizeTeamActionWithContext(w, r, teamID, action, auditReason, nil)
}

func (h *HTTPHandler) authorizeTeamActionWithContext(w http.ResponseWriter, r *http.Request, teamID uuid.UUID, action, auditReason string, context map[string]any) (uuid.UUID, bool) {
	return h.authorizeTeamRequest(w, r, action, authz.ResourceRef{
		Type: authz.ResourceTeam,
		ID:   teamID.String(),
	}, &teamID, auditReason, context)
}

func (h *HTTPHandler) authorizeTeamRequest(w http.ResponseWriter, r *http.Request, action string, resource authz.ResourceRef, teamID *uuid.UUID, auditReason string, requestContext ...map[string]any) (uuid.UUID, bool) {
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
		Context:     firstContext(requestContext),
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
	ID                string                   `json:"id"`
	TenantID          string                   `json:"tenant_id"`
	Slug              string                   `json:"slug"`
	Name              string                   `json:"name"`
	Description       string                   `json:"description"`
	Status            TeamStatus               `json:"status"`
	HumanOwnerUserIDs []string                 `json:"human_owner_user_ids,omitempty"`
	HumanOwners       []teamHumanOwnerResponse `json:"human_owners,omitempty"`
	Constitution      map[string]any           `json:"constitution"`
	Metadata          map[string]any           `json:"metadata"`
	CreatedAt         string                   `json:"created_at,omitempty"`
	UpdatedAt         string                   `json:"updated_at,omitempty"`
}

type teamListItemResponse struct {
	ID                   string                   `json:"id"`
	TenantID             string                   `json:"tenant_id"`
	Slug                 string                   `json:"slug"`
	Name                 string                   `json:"name"`
	Description          string                   `json:"description"`
	Status               TeamStatus               `json:"status"`
	HumanOwnerUserIDs    []string                 `json:"human_owner_user_ids,omitempty"`
	HumanOwners          []teamHumanOwnerResponse `json:"human_owners,omitempty"`
	Metadata             map[string]any           `json:"metadata"`
	CreatedAt            string                   `json:"created_at,omitempty"`
	UpdatedAt            string                   `json:"updated_at,omitempty"`
	MemberCount          int32                    `json:"member_count"`
	DigitalEmployeeCount int32                    `json:"digital_employee_count"`
	CapabilityCount      int32                    `json:"capability_count"`
	GovernanceStatus     GovernanceSummaryStatus  `json:"governance_status"`
	PendingDraftCount    int32                    `json:"pending_draft_count"`
	RiskSummary          string                   `json:"risk_summary"`
}

type teamHumanOwnerResponse struct {
	UserID      string              `json:"user_id"`
	Username    string              `json:"username"`
	DisplayName string              `json:"display_name"`
	Email       string              `json:"email"`
	Status      string              `json:"status"`
	Avatar      *userAvatarResponse `json:"avatar,omitempty"`
}

type teamOverviewResponse struct {
	Team                 teamResponse        `json:"team"`
	MemberCount          int32               `json:"member_count"`
	DigitalEmployeeCount int32               `json:"digital_employee_count"`
	CapabilityCount      int32               `json:"capability_count"`
	PendingDraftCount    int32               `json:"pending_draft_count"`
	PendingItemCount     int32               `json:"pending_item_count"`
	AllowedActions       []AllowedTeamAction `json:"allowed_actions"`
}

type validationIssueResponse struct {
	Field    string `json:"field"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type teamMemberResponse struct {
	MembershipID     string              `json:"membership_id"`
	TenantID         string              `json:"tenant_id"`
	TeamID           string              `json:"team_id"`
	UserID           string              `json:"user_id"`
	Username         string              `json:"username"`
	DisplayName      string              `json:"display_name"`
	Email            string              `json:"email"`
	AccountStatus    string              `json:"account_status"`
	Avatar           *userAvatarResponse `json:"avatar,omitempty"`
	Role             string              `json:"role"`
	MembershipStatus string              `json:"membership_status"`
	CreatedAt        string              `json:"created_at,omitempty"`
	UpdatedAt        string              `json:"updated_at,omitempty"`
}

type userAvatarResponse struct {
	Provider string         `json:"provider"`
	Style    string         `json:"style"`
	Seed     string         `json:"seed"`
	Options  map[string]any `json:"options,omitempty"`
}

type teamAuditEventResponse struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	EventType    string         `json:"event_type"`
	ActorType    string         `json:"actor_type"`
	ActorID      string         `json:"actor_id"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Action       string         `json:"action"`
	Details      map[string]any `json:"details"`
	IPAddress    string         `json:"ip_address"`
	CreatedAt    string         `json:"created_at,omitempty"`
}

func teamIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	teamID, err := uuid.Parse(chi.URLParam(r, "teamId"))
	if err != nil || teamID == uuid.Nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return teamID, true
}

func memberIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	memberID, err := uuid.Parse(chi.URLParam(r, "memberId"))
	if err != nil || memberID == uuid.Nil {
		http.Error(w, "invalid member id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return memberID, true
}

func roleRequestIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	requestID, err := uuid.Parse(chi.URLParam(r, "requestId"))
	if err != nil || requestID == uuid.Nil {
		http.Error(w, "invalid request id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return requestID, true
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

func teamListItemResponses(teams []*TeamListItem) []teamListItemResponse {
	responses := make([]teamListItemResponse, 0, len(teams))
	for _, team := range teams {
		responses = append(responses, teamListItemResponseFromDomain(team))
	}
	return responses
}

type pendingDeleteTeamResponse struct {
	teamResponse
	DeletedAt         string `json:"deleted_at"`
	DeleteRequestedBy string `json:"delete_requested_by,omitempty"`
}

func pendingDeleteTeamResponses(records []PendingDeleteTeamRecord) []pendingDeleteTeamResponse {
	responses := make([]pendingDeleteTeamResponse, 0, len(records))
	for i := range records {
		team := records[i].Team
		response := pendingDeleteTeamResponse{
			teamResponse: teamResponseFromDomain(&team),
			DeletedAt:    records[i].DeletedAt.UTC().Format(time.RFC3339),
		}
		if records[i].DeleteRequestedBy != nil {
			response.DeleteRequestedBy = records[i].DeleteRequestedBy.String()
		}
		responses = append(responses, response)
	}
	return responses
}

func teamResponseFromDomain(team *Team) teamResponse {
	return teamResponse{
		ID:                team.ID.String(),
		TenantID:          team.TenantID.String(),
		Slug:              team.Slug,
		Name:              team.Name,
		Description:       team.Description,
		Status:            team.Status,
		HumanOwnerUserIDs: uuidStringSlice(team.HumanOwnerUserIDs),
		HumanOwners:       teamHumanOwnersResponseFromDomain(team.HumanOwners),
		Constitution:      cloneMap(team.Constitution),
		Metadata:          cloneMap(team.Metadata),
		CreatedAt:         timeString(team.CreatedAt),
		UpdatedAt:         timeString(team.UpdatedAt),
	}
}

func teamListItemResponseFromDomain(item *TeamListItem) teamListItemResponse {
	return teamListItemResponse{
		ID:                   item.ID.String(),
		TenantID:             item.TenantID.String(),
		Slug:                 item.Slug,
		Name:                 item.Name,
		Description:          item.Description,
		Status:               item.Status,
		HumanOwnerUserIDs:    uuidStringSlice(item.HumanOwnerUserIDs),
		HumanOwners:          teamHumanOwnersResponseFromDomain(item.HumanOwners),
		Metadata:             cloneMap(item.Metadata),
		CreatedAt:            timeString(item.CreatedAt),
		UpdatedAt:            timeString(item.UpdatedAt),
		MemberCount:          item.MemberCount,
		DigitalEmployeeCount: item.DigitalEmployeeCount,
		CapabilityCount:      item.CapabilityCount,
		GovernanceStatus:     item.GovernanceStatus,
		PendingDraftCount:    item.PendingDraftCount,
		RiskSummary:          item.RiskSummary,
	}
}

func teamHumanOwnersResponseFromDomain(owners []TeamHumanOwner) []teamHumanOwnerResponse {
	if owners == nil {
		return nil
	}
	var res []teamHumanOwnerResponse
	for _, owner := range owners {
		res = append(res, teamHumanOwnerResponse{
			UserID:      owner.UserID.String(),
			Username:    owner.Username,
			DisplayName: owner.DisplayName,
			Email:       owner.Email,
			Status:      owner.Status,
			Avatar:      userAvatarResponseFromDomain(owner.Avatar),
		})
	}
	return res
}

func userAvatarResponseFromDomain(avatar *UserAvatarConfig) *userAvatarResponse {
	if avatar == nil {
		return nil
	}
	return &userAvatarResponse{
		Provider: avatar.Provider,
		Style:    avatar.Style,
		Seed:     avatar.Seed,
		Options:  cloneMap(avatar.Options),
	}
}

func teamOverviewResponseFromDomain(overview *TeamOverview) teamOverviewResponse {
	response := teamOverviewResponse{
		MemberCount:          overview.MemberCount,
		DigitalEmployeeCount: overview.DigitalEmployeeCount,
		CapabilityCount:      overview.CapabilityCount,
		PendingDraftCount:    overview.PendingDraftCount,
		PendingItemCount:     overview.PendingItemCount,
		AllowedActions:       append([]AllowedTeamAction{}, overview.AllowedActions...),
	}
	if overview.Team != nil {
		response.Team = teamResponseFromDomain(overview.Team)
	}
	return response
}

func teamMemberResponses(members []*TeamMember) []teamMemberResponse {
	responses := make([]teamMemberResponse, 0, len(members))
	for _, member := range members {
		responses = append(responses, teamMemberResponseFromDomain(member))
	}
	return responses
}

func teamAuditEventResponses(events []*audit.Event) []teamAuditEventResponse {
	responses := make([]teamAuditEventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, teamAuditEventResponseFromDomain(event))
	}
	return responses
}

func teamMemberResponseFromDomain(member *TeamMember) teamMemberResponse {
	return teamMemberResponse{
		MembershipID:     member.MembershipID.String(),
		TenantID:         member.TenantID.String(),
		TeamID:           member.TeamID.String(),
		UserID:           member.UserID.String(),
		Username:         member.Username,
		DisplayName:      member.DisplayName,
		Email:            member.Email,
		AccountStatus:    member.AccountStatus,
		Avatar:           userAvatarResponseFromDomain(member.Avatar),
		Role:             member.Role,
		MembershipStatus: member.MembershipStatus,
		CreatedAt:        timeString(member.CreatedAt),
		UpdatedAt:        timeString(member.UpdatedAt),
	}
}

func firstContext(values []map[string]any) map[string]any {
	if len(values) == 0 || values[0] == nil {
		return nil
	}
	return values[0]
}

func teamAuditEventResponseFromDomain(event *audit.Event) teamAuditEventResponse {
	return teamAuditEventResponse{
		ID:           event.ID.String(),
		TenantID:     event.TenantID.String(),
		EventType:    event.EventType,
		ActorType:    event.ActorType,
		ActorID:      event.ActorID,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Action:       event.Action,
		Details:      cloneMap(event.Details),
		IPAddress:    event.IPAddress,
		CreatedAt:    timeString(event.CreatedAt),
	}
}

func uuidStringSlice(values []uuid.UUID) []string {
	if values == nil {
		return nil
	}
	var res []string
	for _, v := range values {
		res = append(res, v.String())
	}
	return res
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
