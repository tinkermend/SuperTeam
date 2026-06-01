package authzcenter

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
)

type HTTPHandler struct {
	service     *Service
	authService *auth.Service
}

func NewHandler(service *Service, authService *auth.Service) *HTTPHandler {
	return &HTTPHandler{service: service, authService: authService}
}

func (h *HTTPHandler) GetAuthzOverview(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentActor(r)
	if err != nil {
		h.writeError(w, err)
		return
	}
	overview, err := h.service.GetOverview(r.Context(), actor)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGeneratedOverview(overview))
}

func (h *HTTPHandler) ListAuthzDecisions(w http.ResponseWriter, r *http.Request, params ListAuthzDecisionsParams) {
	actor, err := h.currentActor(r)
	if err != nil {
		h.writeError(w, err)
		return
	}
	decisions, err := h.service.ListDecisions(r.Context(), actor, DecisionFilter{
		Result:       enumString(params.Result),
		Action:       stringValue(params.Action),
		ActorType:    stringValue(params.ActorType),
		ActorID:      stringValue(params.ActorId),
		ResourceType: stringValue(params.ResourceType),
		ResourceID:   stringValue(params.ResourceId),
		RequestID:    stringValue(params.RequestId),
		Limit:        int32Value(params.Limit, 20),
		Offset:       int32Value(params.Offset, 0),
	})
	if err != nil {
		h.writeError(w, err)
		return
	}
	items := make([]AuthzDecisionRecord, 0, len(decisions))
	for _, decision := range decisions {
		items = append(items, toGeneratedDecision(decision))
	}
	writeJSON(w, http.StatusOK, AuthzDecisionListResponse{Items: items})
}

func (h *HTTPHandler) ListRuntimeScopes(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentActor(r)
	if err != nil {
		h.writeError(w, err)
		return
	}
	nodes, err := h.service.ListRuntimeScopes(r.Context(), actor)
	if err != nil {
		h.writeError(w, err)
		return
	}
	items := make([]RuntimeScopeNode, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, toGeneratedRuntimeScopeNode(node))
	}
	writeJSON(w, http.StatusOK, RuntimeScopeListResponse{Nodes: items})
}

func (h *HTTPHandler) CreateRuntimeScope(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentActor(r)
	if err != nil {
		h.writeError(w, err)
		return
	}
	var body CreateRuntimeScopeJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	scope, err := h.service.CreateRuntimeScope(r.Context(), actor, RuntimeScopeInput{
		TenantID:      uuid.UUID(body.TenantId),
		RuntimeNodeID: uuid.UUID(body.RuntimeNodeId),
		TeamID:        uuidPtrFromOpenAPI(body.TeamId),
		ScopeType:     string(body.ScopeType),
		ScopeValue:    body.ScopeValue,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, RuntimeScopeResponse{Scope: toGeneratedRuntimeScope(scope)})
}

func (h *HTTPHandler) UpdateRuntimeScope(w http.ResponseWriter, r *http.Request, scopeId openapi_types.UUID) {
	actor, err := h.currentActor(r)
	if err != nil {
		h.writeError(w, err)
		return
	}
	var body UpdateRuntimeScopeJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	scope, err := h.service.UpdateRuntimeScopeStatus(r.Context(), actor, uuid.UUID(scopeId), string(body.Status))
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, RuntimeScopeResponse{Scope: toGeneratedRuntimeScope(scope)})
}

func (h *HTTPHandler) ListAuthzMembers(w http.ResponseWriter, r *http.Request, params ListAuthzMembersParams) {
	actor, err := h.currentActor(r)
	if err != nil {
		h.writeError(w, err)
		return
	}
	members, err := h.service.ListMembers(r.Context(), actor, MemberFilter{
		Limit:  int32Value(params.Limit, 20),
		Offset: int32Value(params.Offset, 0),
	})
	if err != nil {
		h.writeError(w, err)
		return
	}
	items := make([]AuthzMemberRecord, 0, len(members))
	for _, member := range members {
		items = append(items, toGeneratedMember(member))
	}
	writeJSON(w, http.StatusOK, AuthzMemberListResponse{Items: items})
}

func (h *HTTPHandler) CheckPermission(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentActor(r)
	if err != nil {
		h.writeError(w, err)
		return
	}
	var body CheckPermissionJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	decision, err := h.service.CheckPermission(r.Context(), actor, CheckPermissionInput{
		Actor: authz.ActorRef{
			Type: body.Actor.Type,
			ID:   body.Actor.Id,
		},
		Action: string(body.Action),
		Resource: authz.ResourceRef{
			Type: body.Resource.Type,
			ID:   body.Resource.Id,
		},
		TenantID: uuid.UUID(body.TenantId),
		TeamID:   uuidPtrFromOpenAPI(body.TeamId),
	})
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, CheckPermissionResponse{
		Allowed:     decision.Allowed,
		Engine:      "db",
		MatchedRule: decision.MatchedRule,
		Reason:      decision.Reason,
		Snapshot:    optionalMap(decision.Snapshot),
	})
}

func (h *HTTPHandler) currentActor(r *http.Request) (Actor, error) {
	if h == nil || h.authService == nil {
		return Actor{}, auth.ErrUnauthorized
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return Actor{}, auth.ErrUnauthorized
	}
	current, err := h.authService.GetCurrentUserContext(r.Context(), cookie.Value)
	if err != nil {
		return Actor{}, err
	}
	if current == nil || current.User == nil {
		return Actor{}, auth.ErrUnauthorized
	}
	return Actor{
		UserID:   current.User.ID,
		Username: current.User.Username,
		TenantID: current.TenantID,
		TeamID:   current.TeamID,
	}, nil
}

func (h *HTTPHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUserDisabled):
		writeError(w, http.StatusForbidden, "user account is disabled")
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrUnauthorized), errors.Is(err, auth.ErrSessionNotFound), errors.Is(err, auth.ErrSessionExpired):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid input")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func toGeneratedOverview(overview Overview) AuthzOverviewResponse {
	top := make([]AuthzActionCount, 0, len(overview.TopDeniedActions))
	for _, item := range overview.TopDeniedActions {
		top = append(top, AuthzActionCount{Action: item.Action, Count: item.Count})
	}
	recent := make([]AuthzDecisionRecord, 0, len(overview.RecentEvents))
	for _, item := range overview.RecentEvents {
		recent = append(recent, toGeneratedDecision(item))
	}
	return AuthzOverviewResponse{
		Engine: AuthzEngineStatus{
			Engine:        overview.Engine.Engine,
			Status:        overview.Engine.Status,
			EngineVersion: optionalString(overview.Engine.EngineVersion),
		},
		Totals: AuthzTotals{
			Total:      overview.Totals.Total,
			Allowed:    overview.Totals.Allowed,
			Denied:     overview.Totals.Denied,
			DeniedRate: overview.Totals.DeniedRate(),
		},
		TopDeniedActions: top,
		RecentEvents:     recent,
	}
}

func toGeneratedDecision(record DecisionRecord) AuthzDecisionRecord {
	return AuthzDecisionRecord{
		Action:       record.Action,
		ActorId:      record.ActorID,
		ActorType:    record.ActorType,
		CreatedAt:    record.CreatedAt,
		Details:      optionalMap(record.Details),
		Engine:       record.Engine,
		Id:           openapiUUID(record.ID),
		MatchedRule:  record.MatchedRule,
		Module:       record.Module,
		Reason:       record.Reason,
		RequestId:    record.RequestID,
		ResourceId:   record.ResourceID,
		ResourceType: record.ResourceType,
		Result:       AuthzDecisionRecordResult(record.Result),
		TenantId:     openapiUUID(record.TenantID),
		UserId:       optionalOpenAPIUUID(record.UserID),
		Username:     record.Username,
	}
}

func toGeneratedRuntimeScopeNode(record RuntimeScopeNodeRecord) RuntimeScopeNode {
	scopes := make([]RuntimeScope, 0, len(record.Scopes))
	for _, scope := range record.Scopes {
		scopes = append(scopes, toGeneratedRuntimeScope(scope))
	}
	return RuntimeScopeNode{
		CurrentLoad:        record.CurrentLoad,
		LastHeartbeatAt:    record.LastHeartbeatAt,
		MaxSlots:           record.MaxSlots,
		Name:               record.Name,
		NodeId:             record.NodeID,
		RecentDeniedReason: record.RecentDeniedReason,
		RuntimeNodeId:      openapiUUID(record.RuntimeNodeID),
		Scopes:             scopes,
		Status:             record.Status,
		SupportedProviders: record.SupportedProviders,
	}
}

func toGeneratedRuntimeScope(record RuntimeScopeRecord) RuntimeScope {
	return RuntimeScope{
		CreatedAt:     record.CreatedAt,
		DisabledAt:    record.DisabledAt,
		Id:            openapiUUID(record.ID),
		RuntimeNodeId: openapiUUID(record.RuntimeNodeID),
		ScopeType:     record.ScopeType,
		ScopeValue:    record.ScopeValue,
		Status:        record.Status,
		TeamId:        optionalOpenAPIUUID(record.TeamID),
		TenantId:      openapiUUID(record.TenantID),
		UpdatedAt:     record.UpdatedAt,
	}
}

func toGeneratedMember(record MemberRecord) AuthzMemberRecord {
	memberships := make([]AuthzMembershipRecord, 0, len(record.Memberships))
	for _, membership := range record.Memberships {
		memberships = append(memberships, AuthzMembershipRecord{
			PrincipalId:   openapiUUID(membership.PrincipalID),
			PrincipalType: membership.PrincipalType,
			Role:          membership.Role,
			Status:        membership.Status,
			TeamId:        optionalOpenAPIUUID(membership.TeamID),
			TenantId:      openapiUUID(membership.TenantID),
		})
	}
	return AuthzMemberRecord{
		AccountStatus:      record.AccountStatus,
		ConsoleAccess:      record.ConsoleAccess,
		DisplayName:        record.DisplayName,
		Email:              record.Email,
		Memberships:        memberships,
		RecentDeniedReason: record.RecentDeniedReason,
		UserId:             openapiUUID(record.UserID),
		Username:           record.Username,
	}
}

func enumString[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func int32Value[T ~int32](value *T, fallback int32) int32 {
	if value == nil {
		return fallback
	}
	return int32(*value)
}

func uuidPtrFromOpenAPI(value *openapi_types.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	id := uuid.UUID(*value)
	return &id
}

func openapiUUID(value uuid.UUID) openapi_types.UUID {
	return openapi_types.UUID(value)
}

func optionalOpenAPIUUID(value *uuid.UUID) *openapi_types.UUID {
	if value == nil {
		return nil
	}
	id := openapiUUID(*value)
	return &id
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalMap(value map[string]any) *map[string]interface{} {
	if value == nil {
		return nil
	}
	mapped := map[string]interface{}(value)
	return &mapped
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, Error{Error: message})
}
