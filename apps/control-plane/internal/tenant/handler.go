package tenant

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
	"github.com/superteam/control-plane/internal/audit"
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error)
	ListTeamSummaries(ctx context.Context, req ListTeamsRequest) ([]*TeamListItem, error)
	GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (*Team, error)
	GetOverview(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamOverview, error)
	UpdateTeam(ctx context.Context, req UpdateTeamRequest) (*Team, error)
	ChangeTeamStatus(ctx context.Context, req ChangeTeamStatusRequest) (*Team, error)
	CreateConfigRevision(ctx context.Context, req CreateTeamConfigRevisionRequest) (*TeamConfigRevision, error)
	GetCurrentConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamConfigRevision, error)
	ListGovernanceDrafts(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*TeamConfigRevision, error)
	CreateGovernanceDraft(ctx context.Context, req CreateTeamConfigRevisionRequest) (*TeamConfigRevision, error)
	UpdateGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID, input GovernanceDraftInput) (*TeamConfigRevision, error)
	ApproveGovernanceDraft(ctx context.Context, tenantID, teamID, draftID, approvedBy uuid.UUID) (*TeamConfigRevision, error)
	RejectGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*TeamConfigRevision, error)
	PreviewGovernanceDiff(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*GovernanceDiffSummary, error)
	ListTeamMembers(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*TeamMember, error)
	AddTeamMember(ctx context.Context, req AddTeamMemberRequest) (*TeamMember, error)
	RemoveTeamMember(ctx context.Context, req RemoveTeamMemberRequest) error
	CreateRoleRequest(ctx context.Context, req CreateRoleRequestRequest) (*TeamMemberRoleRequest, error)
	ListRoleRequests(ctx context.Context, tenantID, teamID uuid.UUID, status TeamMemberRoleRequestStatus, limit, offset int32) ([]*TeamMemberRoleRequest, error)
	ApproveRoleRequest(ctx context.Context, req DecideRoleRequestRequest) (*TeamMemberRoleRequest, error)
	RejectRoleRequest(ctx context.Context, req DecideRoleRequestRequest) (*TeamMemberRoleRequest, error)
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
	q := r.URL.Query().Get("q")

	teams, err := service.ListTeamSummaries(r.Context(), ListTeamsRequest{
		TenantID: tenantID,
		Status:   status,
		Q:        q,
		Offset:   offset,
		Limit:    limit,
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
		Slug             string         `json:"slug"`
		Name             string         `json:"name"`
		HumanOwnerUserID *uuid.UUID     `json:"human_owner_user_id"`
		Metadata         map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	team, err := service.UpdateTeam(r.Context(), UpdateTeamRequest{
		TenantID:         tenantID,
		TeamID:           teamID,
		Slug:             req.Slug,
		Name:             req.Name,
		HumanOwnerUserID: req.HumanOwnerUserID,
		Metadata:         req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamResponseFromDomain(team))
}

func (h *HTTPHandler) DisableTeam(w http.ResponseWriter, r *http.Request) {
	h.changeTeamStatus(w, r, TeamStatusDisabled, authz.ActionTeamDisable, "team disable")
}

func (h *HTTPHandler) ArchiveTeam(w http.ResponseWriter, r *http.Request) {
	h.changeTeamStatus(w, r, TeamStatusArchived, authz.ActionTeamArchive, "team archive")
}

func (h *HTTPHandler) RestoreTeam(w http.ResponseWriter, r *http.Request) {
	h.changeTeamStatus(w, r, TeamStatusActive, authz.ActionTeamRestore, "team restore")
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

func (h *HTTPHandler) ListGovernanceDrafts(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamGovernanceRead, "team governance drafts read")
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
	drafts, err := service.ListGovernanceDrafts(r.Context(), tenantID, teamID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configRevisionResponses(drafts))
}

func (h *HTTPHandler) CreateGovernanceDraft(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamGovernanceEdit, "team governance draft create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	input, ok := governanceDraftInputFromRequest(w, r)
	if !ok {
		return
	}
	revision, err := service.CreateGovernanceDraft(r.Context(), CreateTeamConfigRevisionRequest{
		TenantID:                    tenantID,
		TeamID:                      teamID,
		Constitution:                input.Constitution,
		CapabilityPolicy:            input.CapabilityPolicy,
		ContextPolicy:               input.ContextPolicy,
		ApprovalPolicy:              input.ApprovalPolicy,
		ArtifactContract:            input.ArtifactContract,
		InternalCollaborationPolicy: input.InternalCollaborationPolicy,
		RuntimeScopePolicy:          input.RuntimeScopePolicy,
		HumanOwnerUserID:            input.HumanOwnerUserID,
		Status:                      TeamConfigRevisionStatusDraft,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, configRevisionResponseFromDomain(revision))
}

func (h *HTTPHandler) UpdateGovernanceDraft(w http.ResponseWriter, r *http.Request) {
	teamID, draftID, ok := teamAndDraftIDsFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamGovernanceEdit, "team governance draft update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	input, ok := governanceDraftInputFromRequest(w, r)
	if !ok {
		return
	}
	revision, err := service.UpdateGovernanceDraft(r.Context(), tenantID, teamID, draftID, input)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configRevisionResponseFromDomain(revision))
}

func (h *HTTPHandler) ApproveGovernanceDraft(w http.ResponseWriter, r *http.Request) {
	teamID, draftID, ok := teamAndDraftIDsFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamGovernanceApprove, "team governance draft approve")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	approvedBy := middleware.GetUserID(r.Context())
	revision, err := service.ApproveGovernanceDraft(r.Context(), tenantID, teamID, draftID, approvedBy)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configRevisionResponseFromDomain(revision))
}

func (h *HTTPHandler) RejectGovernanceDraft(w http.ResponseWriter, r *http.Request) {
	teamID, draftID, ok := teamAndDraftIDsFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamGovernanceApprove, "team governance draft reject")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	revision, err := service.RejectGovernanceDraft(r.Context(), tenantID, teamID, draftID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configRevisionResponseFromDomain(revision))
}

func (h *HTTPHandler) PreviewGovernanceDiff(w http.ResponseWriter, r *http.Request) {
	teamID, draftID, ok := teamAndDraftIDsFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamGovernanceRead, "team governance draft diff read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	diff, err := service.PreviewGovernanceDiff(r.Context(), tenantID, teamID, draftID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, governanceDiffSummaryResponseFromDomain(diff))
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

func (h *HTTPHandler) CreateTeamMemberRoleRequest(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		TargetUserID  uuid.UUID `json:"target_user_id"`
		RequestedRole string    `json:"requested_role"`
		Reason        string    `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeTeamActionWithContext(w, r, teamID, authz.ActionTeamMemberRequestPrivilegedRole, "team member privileged role request", map[string]any{"target_role": req.RequestedRole})
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	requestedBy := middleware.GetUserID(r.Context())
	request, err := service.CreateRoleRequest(r.Context(), CreateRoleRequestRequest{
		TenantID:      tenantID,
		TeamID:        teamID,
		TargetUserID:  req.TargetUserID,
		RequestedRole: req.RequestedRole,
		RequestedBy:   requestedBy,
		Reason:        req.Reason,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, roleRequestResponseFromDomain(request))
}

func (h *HTTPHandler) ListTeamMemberRoleRequests(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamRead, "team member privileged role requests read")
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
	requests, err := service.ListRoleRequests(r.Context(), tenantID, teamID, TeamMemberRoleRequestStatus(r.URL.Query().Get("status")), limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roleRequestResponses(requests))
}

func (h *HTTPHandler) ApproveTeamMemberRoleRequest(w http.ResponseWriter, r *http.Request) {
	h.decideTeamMemberRoleRequest(w, r, true)
}

func (h *HTTPHandler) RejectTeamMemberRoleRequest(w http.ResponseWriter, r *http.Request) {
	h.decideTeamMemberRoleRequest(w, r, false)
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
	authz.ActionTeamDisable,
	authz.ActionTeamArchive,
	authz.ActionTeamRestore,
	authz.ActionTeamMemberAdd,
	authz.ActionTeamMemberRequestPrivilegedRole,
	authz.ActionTeamGovernanceEdit,
	authz.ActionTeamGovernanceApprove,
	authz.ActionTeamCapabilityBind,
	authz.ActionTeamCapabilityUnbind,
	authz.ActionTeamAuditRead,
}

func (h *HTTPHandler) changeTeamStatus(w http.ResponseWriter, r *http.Request, status TeamStatus, action, auditReason string) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, action, auditReason)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	team, err := service.ChangeTeamStatus(r.Context(), ChangeTeamStatusRequest{
		TenantID: tenantID,
		TeamID:   teamID,
		Status:   status,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teamResponseFromDomain(team))
}

func (h *HTTPHandler) decideTeamMemberRoleRequest(w http.ResponseWriter, r *http.Request, approve bool) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	requestID, ok := roleRequestIDFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		DecisionReason string `json:"decision_reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tenantID, ok := h.authorizeTeamAction(w, r, teamID, authz.ActionTeamMemberApprovePrivilegedRole, "team member privileged role decide")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	decidedBy := middleware.GetUserID(r.Context())
	decisionReq := DecideRoleRequestRequest{
		TenantID:       tenantID,
		TeamID:         teamID,
		RequestID:      requestID,
		DecidedBy:      decidedBy,
		DecisionReason: req.DecisionReason,
	}
	var (
		request *TeamMemberRoleRequest
		err     error
	)
	if approve {
		request, err = service.ApproveRoleRequest(r.Context(), decisionReq)
	} else {
		request, err = service.RejectRoleRequest(r.Context(), decisionReq)
	}
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roleRequestResponseFromDomain(request))
}

func (h *HTTPHandler) allowedTeamActions(r *http.Request, tenantID, teamID uuid.UUID) []AllowedTeamAction {
	if h == nil || h.authorizer == nil {
		return []AllowedTeamAction{}
	}
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil || teamID == uuid.Nil {
		return []AllowedTeamAction{}
	}
	allowed := make([]AllowedTeamAction, 0, len(overviewActions))
	for _, action := range overviewActions {
		decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
			Actor: authz.ActorRef{
				Type: authz.ActorUser,
				ID:   userID.String(),
			},
			Action: action,
			Resource: authz.ResourceRef{
				Type: authz.ResourceTeam,
				ID:   teamID.String(),
			},
			TenantID:    tenantID,
			TeamID:      &teamID,
			AuditReason: "team overview allowed action",
		})
		if err == nil && decision.Allowed {
			allowed = append(allowed, AllowedTeamAction(action))
		}
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

type teamListItemResponse struct {
	ID                   string                  `json:"id"`
	TenantID             string                  `json:"tenant_id"`
	Slug                 string                  `json:"slug"`
	Name                 string                  `json:"name"`
	Status               TeamStatus              `json:"status"`
	HumanOwnerUserID     *string                 `json:"human_owner_user_id,omitempty"`
	Metadata             map[string]any          `json:"metadata"`
	CreatedAt            string                  `json:"created_at,omitempty"`
	UpdatedAt            string                  `json:"updated_at,omitempty"`
	MemberCount          int32                   `json:"member_count"`
	DigitalEmployeeCount int32                   `json:"digital_employee_count"`
	CapabilityCount      int32                   `json:"capability_count"`
	GovernanceStatus     GovernanceSummaryStatus `json:"governance_status"`
	CurrentRevision      *int32                  `json:"current_revision,omitempty"`
	PendingDraftCount    int32                   `json:"pending_draft_count"`
	RiskSummary          string                  `json:"risk_summary"`
}

type teamOverviewResponse struct {
	Team                 teamResponse            `json:"team"`
	MemberCount          int32                   `json:"member_count"`
	DigitalEmployeeCount int32                   `json:"digital_employee_count"`
	CapabilityCount      int32                   `json:"capability_count"`
	CurrentRevision      *configRevisionResponse `json:"current_revision,omitempty"`
	PendingDraftCount    int32                   `json:"pending_draft_count"`
	PendingItemCount     int32                   `json:"pending_item_count"`
	AllowedActions       []AllowedTeamAction     `json:"allowed_actions"`
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

type governanceDiffSummaryResponse struct {
	AddedHardRules       int32                     `json:"added_hard_rules"`
	ChangedCapabilities  int32                     `json:"changed_capabilities"`
	ChangedApprovalRules int32                     `json:"changed_approval_rules"`
	Warnings             []validationIssueResponse `json:"warnings"`
	BlockingErrors       []validationIssueResponse `json:"blocking_errors"`
}

type validationIssueResponse struct {
	Field    string `json:"field"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type teamMemberResponse struct {
	MembershipID     string `json:"membership_id"`
	TenantID         string `json:"tenant_id"`
	TeamID           string `json:"team_id"`
	UserID           string `json:"user_id"`
	Username         string `json:"username"`
	DisplayName      string `json:"display_name"`
	Email            string `json:"email"`
	AccountStatus    string `json:"account_status"`
	Role             string `json:"role"`
	MembershipStatus string `json:"membership_status"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

type roleRequestResponse struct {
	ID             string                      `json:"id"`
	TenantID       string                      `json:"tenant_id"`
	TeamID         string                      `json:"team_id"`
	TargetUserID   string                      `json:"target_user_id"`
	RequestedRole  string                      `json:"requested_role"`
	RequestedBy    string                      `json:"requested_by"`
	Status         TeamMemberRoleRequestStatus `json:"status"`
	Reason         string                      `json:"reason"`
	DecidedBy      *string                     `json:"decided_by,omitempty"`
	DecidedAt      *string                     `json:"decided_at,omitempty"`
	DecisionReason string                      `json:"decision_reason"`
	CreatedAt      string                      `json:"created_at,omitempty"`
	UpdatedAt      string                      `json:"updated_at,omitempty"`
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

func teamAndDraftIDsFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	draftID, err := uuid.Parse(chi.URLParam(r, "draftId"))
	if err != nil || draftID == uuid.Nil {
		http.Error(w, "invalid draft id", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	return teamID, draftID, true
}

func governanceDraftInputFromRequest(w http.ResponseWriter, r *http.Request) (GovernanceDraftInput, bool) {
	var req struct {
		Constitution                map[string]any `json:"constitution"`
		CapabilityPolicy            map[string]any `json:"capability_policy"`
		ContextPolicy               map[string]any `json:"context_policy"`
		ApprovalPolicy              map[string]any `json:"approval_policy"`
		ArtifactContract            map[string]any `json:"artifact_contract"`
		InternalCollaborationPolicy map[string]any `json:"internal_collaboration_policy"`
		RuntimeScopePolicy          map[string]any `json:"runtime_scope_policy"`
		HumanOwnerUserID            *uuid.UUID     `json:"human_owner_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return GovernanceDraftInput{}, false
	}
	return GovernanceDraftInput{
		Constitution:                req.Constitution,
		CapabilityPolicy:            req.CapabilityPolicy,
		ContextPolicy:               req.ContextPolicy,
		ApprovalPolicy:              req.ApprovalPolicy,
		ArtifactContract:            req.ArtifactContract,
		InternalCollaborationPolicy: req.InternalCollaborationPolicy,
		RuntimeScopePolicy:          req.RuntimeScopePolicy,
		HumanOwnerUserID:            req.HumanOwnerUserID,
	}, true
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

func teamListItemResponseFromDomain(item *TeamListItem) teamListItemResponse {
	return teamListItemResponse{
		ID:                   item.ID.String(),
		TenantID:             item.TenantID.String(),
		Slug:                 item.Slug,
		Name:                 item.Name,
		Status:               item.Status,
		HumanOwnerUserID:     uuidStringPtr(item.HumanOwnerUserID),
		Metadata:             cloneMap(item.Metadata),
		CreatedAt:            timeString(item.CreatedAt),
		UpdatedAt:            timeString(item.UpdatedAt),
		MemberCount:          item.MemberCount,
		DigitalEmployeeCount: item.DigitalEmployeeCount,
		CapabilityCount:      item.CapabilityCount,
		GovernanceStatus:     item.GovernanceStatus,
		CurrentRevision:      cloneInt32Ptr(item.CurrentRevision),
		PendingDraftCount:    item.PendingDraftCount,
		RiskSummary:          item.RiskSummary,
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
	if overview.CurrentRevision != nil {
		revision := configRevisionResponseFromDomain(overview.CurrentRevision)
		response.CurrentRevision = &revision
	}
	return response
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

func configRevisionResponses(revisions []*TeamConfigRevision) []configRevisionResponse {
	responses := make([]configRevisionResponse, 0, len(revisions))
	for _, revision := range revisions {
		responses = append(responses, configRevisionResponseFromDomain(revision))
	}
	return responses
}

func governanceDiffSummaryResponseFromDomain(summary *GovernanceDiffSummary) governanceDiffSummaryResponse {
	if summary == nil {
		return governanceDiffSummaryResponse{
			Warnings:       []validationIssueResponse{},
			BlockingErrors: []validationIssueResponse{},
		}
	}
	return governanceDiffSummaryResponse{
		AddedHardRules:       summary.AddedHardRules,
		ChangedCapabilities:  summary.ChangedCapabilities,
		ChangedApprovalRules: summary.ChangedApprovalRules,
		Warnings:             validationIssueResponses(summary.Warnings),
		BlockingErrors:       validationIssueResponses(summary.BlockingErrors),
	}
}

func validationIssueResponses(issues []ValidationIssue) []validationIssueResponse {
	responses := make([]validationIssueResponse, 0, len(issues))
	for _, issue := range issues {
		responses = append(responses, validationIssueResponse{
			Field:    issue.Field,
			Message:  issue.Message,
			Severity: issue.Severity,
		})
	}
	return responses
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
		Role:             member.Role,
		MembershipStatus: member.MembershipStatus,
		CreatedAt:        timeString(member.CreatedAt),
		UpdatedAt:        timeString(member.UpdatedAt),
	}
}

func roleRequestResponseFromDomain(request *TeamMemberRoleRequest) roleRequestResponse {
	return roleRequestResponse{
		ID:             request.ID.String(),
		TenantID:       request.TenantID.String(),
		TeamID:         request.TeamID.String(),
		TargetUserID:   request.TargetUserID.String(),
		RequestedRole:  request.RequestedRole,
		RequestedBy:    request.RequestedBy.String(),
		Status:         request.Status,
		Reason:         request.Reason,
		DecidedBy:      uuidStringPtr(request.DecidedBy),
		DecidedAt:      timeStringPtr(request.DecidedAt),
		DecisionReason: request.DecisionReason,
		CreatedAt:      timeString(request.CreatedAt),
		UpdatedAt:      timeString(request.UpdatedAt),
	}
}

func roleRequestResponses(requests []*TeamMemberRoleRequest) []roleRequestResponse {
	responses := make([]roleRequestResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, roleRequestResponseFromDomain(request))
	}
	return responses
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
