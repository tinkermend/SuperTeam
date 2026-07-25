package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/platform"
)

const SessionCookieName = "session_token"

type HTTPHandler struct {
	service    *Service
	authorizer authz.Authorizer
}

func NewHandler(service *Service, authorizer ...authz.Authorizer) *HTTPHandler {
	var az authz.Authorizer
	if len(authorizer) > 0 {
		az = authorizer[0]
	}
	return &HTTPHandler{service: service, authorizer: az}
}

func (h *HTTPHandler) CreateCaptcha(w http.ResponseWriter, r *http.Request) {
	challenge, err := h.service.CreateCaptcha(r.Context(), clientIP(r), r.UserAgent())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !challenge.Enabled {
		writeJSON(w, http.StatusOK, CaptchaChallengeResponse{
			Enabled: false,
		})
		return
	}
	captchaID := openapi_types.UUID(challenge.ID)
	imageDataURL := challenge.ImageDataURL
	expiresAt := challenge.ExpiresAt
	writeJSON(w, http.StatusOK, CaptchaChallengeResponse{
		CaptchaId:    &captchaID,
		Enabled:      true,
		ImageDataUrl: &imageDataURL,
		ExpiresAt:    &expiresAt,
	})
}

func (h *HTTPHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body LoginJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if h.service.IsCaptchaEnabled() {
		captchaID, captchaCode, ok := captchaInput(body)
		if !ok {
			writeError(w, http.StatusBadRequest, "captcha is required")
			return
		}
		if err := h.service.ValidateAndConsumeCaptcha(r.Context(), captchaID, captchaCode, body.Username, clientIP(r), r.UserAgent()); err != nil {
			h.writeAuthError(w, err)
			return
		}
	}

	session, user, token, err := h.service.Login(r.Context(), body.Username, body.Password, clientIP(r), r.UserAgent())
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	http.SetCookie(w, sessionCookie(token, int(session.ExpiresAt.Sub(session.LastSeenAt).Seconds())))
	writeJSON(w, http.StatusOK, LoginResponse{User: toGeneratedUserSummary(user)})
}

func (h *HTTPHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	current, ok := h.requireConsoleAccess(w, r, "current user console access")
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, CurrentUserResponse{User: toGeneratedUserSummary(current.User)})
}

func (h *HTTPHandler) ListLoginLogs(w http.ResponseWriter, r *http.Request, params ListLoginLogsParams) {
	if _, ok := h.requireConsoleAccess(w, r, "login log read"); !ok {
		return
	}

	filter := ListLoginLogsFilter{
		Limit:  valueOrDefault(params.Limit, 20),
		Offset: valueOrDefault(params.Offset, 0),
	}
	if params.EventType != nil {
		filter.EventType = string(*params.EventType)
	}
	if params.Result != nil {
		filter.Result = string(*params.Result)
	}
	logs, err := h.service.ListLoginLogs(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	items := make([]LoginLogRecord, 0, len(logs))
	for _, log := range logs {
		items = append(items, toGeneratedLoginLogRecord(log))
	}
	writeJSON(w, http.StatusOK, LoginLogListResponse{Items: items})
}

func (h *HTTPHandler) ListOperationLogs(w http.ResponseWriter, r *http.Request, params ListOperationLogsParams) {
	if _, ok := h.requireConsoleAccess(w, r, "operation log read"); !ok {
		return
	}

	filter := ListOperationLogsFilter{
		Limit:  valueOrDefault(params.Limit, 20),
		Offset: valueOrDefault(params.Offset, 0),
	}
	if params.Module != nil {
		filter.Module = *params.Module
	}
	if params.Action != nil {
		filter.Action = *params.Action
	}
	if params.Result != nil {
		filter.Result = string(*params.Result)
	}

	logs, err := h.service.ListOperationLogs(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	items := make([]OperationLogRecord, 0, len(logs))
	for _, log := range logs {
		items = append(items, toGeneratedOperationLogRecord(log))
	}
	writeJSON(w, http.StatusOK, OperationLogListResponse{Items: items})
}

func captchaInput(body LoginJSONRequestBody) (uuid.UUID, string, bool) {
	if body.CaptchaId == nil || body.CaptchaCode == nil {
		return uuid.Nil, "", false
	}
	captchaID := uuid.UUID(*body.CaptchaId)
	captchaCode := strings.TrimSpace(*body.CaptchaCode)
	return captchaID, captchaCode, captchaID != uuid.Nil && captchaCode != ""
}

func (h *HTTPHandler) ListCurrentUserLoginLogs(w http.ResponseWriter, r *http.Request, params ListCurrentUserLoginLogsParams) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	logs, err := h.service.ListCurrentUserLoginLogs(r.Context(), toActor(actorUser), ListLoginLogsFilter{
		Limit:  valueOrDefault(params.Limit, 20),
		Offset: valueOrDefault(params.Offset, 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	items := make([]LoginLogRecord, 0, len(logs))
	for _, log := range logs {
		items = append(items, toGeneratedLoginLogRecord(log))
	}
	writeJSON(w, http.StatusOK, LoginLogListResponse{Items: items})
}

// SetCurrentUserAvatarSVG 自愈写回当前用户预渲染头像(P1-D 2b)。仅写当前登录用户自己。
func (h *HTTPHandler) SetCurrentUserAvatarSVG(w http.ResponseWriter, r *http.Request) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	var body SetCurrentUserAvatarSVGJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.service.SetCurrentUserAvatarSVG(r.Context(), actorUser.ID, body.Svg); err != nil {
		if errors.Is(err, ErrInvalidManagedUserInput) {
			writeError(w, http.StatusBadRequest, "invalid avatar svg")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) UpdateCurrentUserProfile(w http.ResponseWriter, r *http.Request) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	var body UpdateCurrentUserProfileJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	input := UpdateUserProfileInput{
		DisplayName: actorUser.DisplayName,
		Email:       actorUser.Email,
		Avatar:      actorUser.Avatar,
	}
	if body.DisplayName != nil {
		input.DisplayName = *body.DisplayName
	}
	if body.Email != nil {
		input.Email = string(*body.Email)
	}
	if body.Avatar != nil {
		input.Avatar = userAvatarFromGenerated(body.Avatar)
	}
	user, err := h.service.UpdateCurrentUserProfile(r.Context(), toActor(actorUser), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, UserResponse{User: toGeneratedUserSummary(user)})
}

func (h *HTTPHandler) ChangeCurrentUserPassword(w http.ResponseWriter, r *http.Request) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	var body ChangeCurrentUserPasswordJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := h.service.ChangeCurrentUserPassword(r.Context(), toActor(actorUser), body.CurrentPassword, body.Password)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, UserResponse{User: toGeneratedUserSummary(user)})
}

func (h *HTTPHandler) ListUsers(w http.ResponseWriter, r *http.Request, params ListUsersParams) {
	if _, _, err := h.currentSessionUser(r); err != nil {
		h.writeAuthError(w, err)
		return
	}

	status := ""
	if params.Status != nil {
		status = string(*params.Status)
	}
	q := ""
	if params.Q != nil {
		q = *params.Q
	}
	users, err := h.service.ListUsers(r.Context(), ListUsersFilter{
		Q:      q,
		Status: status,
		Limit:  valueOrDefault(params.Limit, 20),
		Offset: valueOrDefault(params.Offset, 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	items := make([]UserSummary, 0, len(users))
	for _, user := range users {
		items = append(items, toGeneratedUserSummary(user))
	}
	writeJSON(w, http.StatusOK, UserListResponse{Items: items})
}

func (h *HTTPHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	if !h.authorizeUserProjectTeamScope(w, r, actorUser, authz.ActionUserProjectTeamScopeManage, "user create with project team scope manage") {
		return
	}

	var body CreateUserJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var selectableTeamIDs []openapi_types.UUID
	if body.SelectableTeamIds != nil {
		selectableTeamIDs = *body.SelectableTeamIds
	}
	user, err := h.service.CreateManagedUser(r.Context(), toActor(actorUser), CreateManagedUserInput{
		TenantID:          platform.DefaultTenantID,
		Username:          body.Username,
		DisplayName:       body.DisplayName,
		Password:          body.Password,
		Avatar:            userAvatarFromGenerated(&body.Avatar),
		TenantRole:        string(body.TenantRole),
		SelectableTeamIDs: uuidSliceFromOpenAPI(selectableTeamIDs),
	})
	if err != nil {
		h.writeManagedUserError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, UserResponse{User: toGeneratedUserSummary(user)})
}

func (h *HTTPHandler) GetUserTenantMembership(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	if !h.authorizeUserProjectTeamScope(w, r, actorUser, authz.ActionUserProjectTeamScopeRead, "user tenant membership read") {
		return
	}
	membership, err := h.service.GetUserTenantMembership(r.Context(), platform.DefaultTenantID, uuid.UUID(id))
	if err != nil {
		h.writeManagedUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, TenantMembershipResponse{Membership: toGeneratedTenantMembership(membership)})
}

func (h *HTTPHandler) UpsertUserTenantMembership(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	if !h.authorizeUserProjectTeamScope(w, r, actorUser, authz.ActionUserProjectTeamScopeManage, "user tenant membership upsert") {
		return
	}
	var body UpsertUserTenantMembershipJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	membership, err := h.service.UpsertUserTenantMembership(r.Context(), toActor(actorUser), platform.DefaultTenantID, uuid.UUID(id), string(body.Role))
	if err != nil {
		h.writeManagedUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, TenantMembershipResponse{Membership: toGeneratedTenantMembership(membership)})
}

func (h *HTTPHandler) DeleteUserTenantMembership(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	if !h.authorizeUserProjectTeamScope(w, r, actorUser, authz.ActionUserProjectTeamScopeManage, "user tenant membership delete") {
		return
	}
	if err := h.service.DeleteUserTenantMembership(r.Context(), toActor(actorUser), platform.DefaultTenantID, uuid.UUID(id)); err != nil {
		h.writeManagedUserError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) ListUserProjectTeamScopes(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	if !h.authorizeUserProjectTeamScope(w, r, actorUser, authz.ActionUserProjectTeamScopeRead, "user project team scope read") {
		return
	}

	scopes, err := h.service.ListUserProjectTeamScopes(r.Context(), platform.DefaultTenantID, uuid.UUID(id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, UserProjectTeamScopeListResponse{Items: toGeneratedUserProjectTeamScopes(scopes)})
}

func (h *HTTPHandler) ReplaceUserProjectTeamScopes(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	if !h.authorizeUserProjectTeamScope(w, r, actorUser, authz.ActionUserProjectTeamScopeManage, "user project team scope manage") {
		return
	}

	var body ReplaceUserProjectTeamScopesJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	scopes, err := h.service.ReplaceUserProjectTeamScopes(r.Context(), toActor(actorUser), platform.DefaultTenantID, uuid.UUID(id), uuidSliceFromOpenAPI(body.TeamIds))
	if err != nil {
		h.writeManagedUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, UserProjectTeamScopeListResponse{Items: toGeneratedUserProjectTeamScopes(scopes)})
}

func (h *HTTPHandler) authorizeUserProjectTeamScope(w http.ResponseWriter, r *http.Request, actorUser *User, action, auditReason string) bool {
	if h.authorizer == nil {
		return true
	}
	tenantID := platform.DefaultTenantID
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   actorUser.ID.String(),
		},
		Action: action,
		Resource: authz.ResourceRef{
			Type: authz.ResourceTenant,
			ID:   tenantID.String(),
		},
		TenantID:    tenantID,
		AuditReason: auditReason,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return false
	}
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func (h *HTTPHandler) UpdateUserStatus(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	var body UpdateUserStatusJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := h.service.UpdateManagedUserStatus(r.Context(), toActor(actorUser), uuid.UUID(id), string(body.Status))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, UserResponse{User: toGeneratedUserSummary(user)})
}

func (h *HTTPHandler) ResetUserPassword(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	_, actorUser, err := h.currentSessionUser(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	var body ResetUserPasswordJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := h.service.ResetManagedUserPassword(r.Context(), toActor(actorUser), uuid.UUID(id), body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, UserResponse{User: toGeneratedUserSummary(user)})
}

func (h *HTTPHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		_ = h.service.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, clearSessionCookie())
	writeJSON(w, http.StatusOK, map[string]string{"message": "logout success"})
}

func (h *HTTPHandler) currentSessionUser(r *http.Request) (*Session, *User, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, nil, ErrUnauthorized
	}
	return h.service.GetUserBySessionToken(r.Context(), cookie.Value)
}

func (h *HTTPHandler) currentUserContext(r *http.Request) (*CurrentUserContext, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return h.service.GetCurrentUserContext(r.Context(), cookie.Value)
}

func (h *HTTPHandler) requireConsoleAccess(w http.ResponseWriter, r *http.Request, auditReason string) (*CurrentUserContext, bool) {
	current, err := h.currentUserContext(r)
	if err != nil {
		h.writeAuthError(w, err)
		return nil, false
	}
	if h.authorizer == nil {
		return current, true
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   current.User.ID.String(),
		},
		Action: authz.ActionConsoleAccess,
		Resource: authz.ResourceRef{
			Type: authz.ResourceConsole,
			ID:   "web",
		},
		TenantID:    current.TenantID,
		TeamID:      current.TeamID,
		AuditReason: auditReason,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return nil, false
	}
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, "forbidden")
		return nil, false
	}
	return current, true
}

func toActor(user *User) Actor {
	return Actor{
		UserID:   user.ID,
		Username: user.Username,
	}
}

func (h *HTTPHandler) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUserDisabled):
		writeError(w, http.StatusForbidden, "user account is disabled")
	case errors.Is(err, ErrCaptchaInvalid), errors.Is(err, ErrCaptchaExpired), errors.Is(err, ErrCaptchaUsed):
		writeError(w, http.StatusUnauthorized, "验证码不正确或已过期")
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrUnauthorized), errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrSessionExpired):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *HTTPHandler) writeManagedUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidManagedUserInput):
		writeError(w, http.StatusBadRequest, "invalid managed user input")
	case errors.Is(err, ErrManagedUserNotFound), errors.Is(err, ErrTenantMembershipNotFound):
		writeError(w, http.StatusNotFound, "managed user not found")
	case errors.Is(err, ErrOwnerGrantForbidden):
		writeError(w, http.StatusForbidden, "only a tenant owner can grant the owner role")
	case errors.Is(err, ErrLastTenantOwner):
		writeError(w, http.StatusConflict, "cannot demote or revoke the last tenant owner")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func toGeneratedTenantMembership(membership *TenantLevelMembership) TenantMembership {
	return TenantMembership{
		Id:        openapi_types.UUID(membership.ID),
		TenantId:  openapi_types.UUID(membership.TenantID),
		UserId:    openapi_types.UUID(membership.UserID),
		Role:      TenantRole(membership.Role),
		Status:    TenantMembershipStatus(membership.Status),
		CreatedAt: membership.CreatedAt,
		UpdatedAt: membership.UpdatedAt,
	}
}

func toGeneratedLoginLogRecord(log LoginLog) LoginLogRecord {
	return LoginLogRecord{
		ClientIp:      optionalString(log.ClientIP),
		CreatedAt:     log.CreatedAt,
		EventType:     LoginLogRecordEventType(log.EventType),
		FailureReason: optionalString(log.FailureReason),
		Id:            openapiUUID(log.ID),
		Result:        LoginLogRecordResult(log.Result),
		SessionId:     optionalOpenAPIUUID(log.SessionID),
		UserAgent:     optionalString(log.UserAgent),
		UserId:        optionalOpenAPIUUID(log.UserID),
		Username:      log.Username,
	}
}

func toGeneratedOperationLogRecord(log OperationLog) OperationLogRecord {
	return OperationLogRecord{
		Action:       log.Action,
		ClientIp:     optionalString(log.ClientIP),
		CreatedAt:    log.CreatedAt,
		Id:           openapiUUID(log.ID),
		Module:       log.Module,
		RequestId:    optionalString(log.RequestID),
		ResourceId:   optionalString(log.ResourceID),
		ResourceType: optionalString(log.ResourceType),
		Result:       OperationLogRecordResult(log.Result),
		UserAgent:    optionalString(log.UserAgent),
		UserId:       optionalOpenAPIUUID(log.UserID),
		Username:     optionalString(log.Username),
	}
}

func toGeneratedUserSummary(user *User) UserSummary {
	return UserSummary{
		Avatar:        toGeneratedUserAvatar(user.Avatar),
		AvatarAssetId: optionalString(user.AvatarAssetID),
		DisplayName:   optionalString(user.DisplayName),
		Email:         optionalString(user.Email),
		Id:            openapiUUID(user.ID),
		Status:        UserSummaryStatus(user.Status),
		Username:      user.Username,
	}
}

func toGeneratedUserProjectTeamScopes(scopes []UserProjectTeamScopeSummary) []UserProjectTeamScope {
	items := make([]UserProjectTeamScope, 0, len(scopes))
	for _, scope := range scopes {
		items = append(items, toGeneratedUserProjectTeamScope(scope))
	}
	return items
}

func toGeneratedUserProjectTeamScope(scope UserProjectTeamScopeSummary) UserProjectTeamScope {
	return UserProjectTeamScope{
		CreatedAt:       scope.CreatedAt,
		GrantedByUserId: optionalOpenAPIUUID(scope.GrantedByUserID),
		Id:              openapiUUID(scope.ID),
		RevokedAt:       scope.RevokedAt,
		Status:          scope.Status,
		Team:            toGeneratedUserProjectTeamSummary(scope.Team),
		TeamId:          openapiUUID(scope.TeamID),
		TenantId:        openapiUUID(scope.TenantID),
		UpdatedAt:       scope.UpdatedAt,
		UserId:          openapiUUID(scope.UserID),
	}
}

func toGeneratedUserProjectTeamSummary(team UserProjectTeamScopeTeamSummary) UserProjectTeamSummary {
	return UserProjectTeamSummary{
		CurrentRevision:      team.CurrentRevision,
		DigitalEmployeeCount: team.DigitalEmployeeCount,
		GovernanceStatus:     team.GovernanceStatus,
		HumanOwners:          toGeneratedUserProjectTeamOwners(team.HumanOwners),
		Id:                   openapiUUID(team.ID),
		Name:                 team.Name,
		PendingDraftCount:    team.PendingDraftCount,
		RiskSummary:          team.RiskSummary,
		Slug:                 team.Slug,
		Status:               team.Status,
	}
}

func toGeneratedUserProjectTeamOwners(owners []UserProjectTeamScopeOwnerSummary) []UserProjectTeamOwner {
	items := make([]UserProjectTeamOwner, 0, len(owners))
	for _, owner := range owners {
		items = append(items, UserProjectTeamOwner{
			Avatar:        toGeneratedUserAvatar(owner.Avatar),
			AvatarAssetId: optionalString(owner.AvatarAssetID),
			DisplayName:   optionalString(owner.DisplayName),
			Email:         optionalString(owner.Email),
			Id:            openapiUUID(owner.ID),
			Status:        owner.Status,
			Username:      owner.Username,
		})
	}
	return items
}

func toGeneratedUserAvatar(avatar UserAvatarConfig) UserAvatar {
	options := avatar.Options
	if options == nil {
		options = map[string]any{}
	}
	generated := UserAvatar{
		Options:  &options,
		Provider: UserAvatarProvider(avatar.Provider),
		Seed:     avatar.Seed,
		Style:    UserAvatarStyle(avatar.Style),
	}
	if avatar.SVG != "" {
		svg := avatar.SVG
		generated.Svg = &svg
	}
	return generated
}

func userAvatarFromGenerated(avatar *UserAvatar) UserAvatarConfig {
	if avatar == nil {
		return UserAvatarConfig{}
	}
	options := map[string]any{}
	if avatar.Options != nil {
		options = *avatar.Options
	}
	return UserAvatarConfig{
		Provider: string(avatar.Provider),
		Style:    string(avatar.Style),
		Seed:     avatar.Seed,
		Options:  options,
	}
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

func uuidSliceFromOpenAPI(values []openapi_types.UUID) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		ids = append(ids, uuid.UUID(value))
	}
	return ids
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func valueOrDefault(value *int32, fallback int32) int32 {
	if value == nil {
		return fallback
	}
	return *value
}

func sessionCookie(token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func clearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

func clientIP(r *http.Request) string {
	if forwardedFor := r.Header.Get("x-forwarded-for"); forwardedFor != "" {
		host, _, err := net.SplitHostPort(forwardedFor)
		if err == nil {
			return host
		}
		return forwardedFor
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
