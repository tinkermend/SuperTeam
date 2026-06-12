package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/superteam/control-plane/internal/authz"
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

func (h *HTTPHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body LoginJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
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
	current, err := h.currentUserContext(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	if h.authorizer != nil {
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
			AuditReason: "current user console access",
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if !decision.Allowed {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	writeJSON(w, http.StatusOK, CurrentUserResponse{User: toGeneratedUserSummary(current.User)})
}

func (h *HTTPHandler) ListLoginLogs(w http.ResponseWriter, r *http.Request, params ListLoginLogsParams) {
	if _, _, err := h.currentSessionUser(r); err != nil {
		h.writeAuthError(w, err)
		return
	}

	logs, err := h.service.ListLoginLogs(r.Context(), ListLoginLogsFilter{
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

	var body CreateUserJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := h.service.CreateManagedUser(r.Context(), toActor(actorUser), CreateManagedUserInput{
		Username: body.Username,
		Password: body.Password,
		Avatar:   userAvatarFromGenerated(body.Avatar),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, UserResponse{User: toGeneratedUserSummary(user)})
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
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrUnauthorized), errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrSessionExpired):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
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

func toGeneratedUserSummary(user *User) UserSummary {
	return UserSummary{
		Avatar:      toGeneratedUserAvatar(user.Avatar),
		DisplayName: optionalString(user.DisplayName),
		Email:       optionalString(user.Email),
		Id:          openapiUUID(user.ID),
		Status:      UserSummaryStatus(user.Status),
		Username:    user.Username,
	}
}

func toGeneratedUserAvatar(avatar UserAvatarConfig) UserAvatar {
	options := avatar.Options
	if options == nil {
		options = map[string]any{}
	}
	return UserAvatar{
		Options:  &options,
		Provider: UserAvatarProvider(avatar.Provider),
		Seed:     avatar.Seed,
		Style:    UserAvatarStyle(avatar.Style),
	}
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
