package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

const SessionCookieName = "session_token"

type HTTPHandler struct {
	service *Service
}

func NewHandler(service *Service) *HTTPHandler {
	return &HTTPHandler{service: service}
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
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	_, user, err := h.service.GetUserBySessionToken(r.Context(), cookie.Value)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, CurrentUserResponse{User: toGeneratedUserSummary(user)})
}

func (h *HTTPHandler) ListLoginLogs(w http.ResponseWriter, r *http.Request, params ListLoginLogsParams) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if _, _, err := h.service.GetUserBySessionToken(r.Context(), cookie.Value); err != nil {
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

func (h *HTTPHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		_ = h.service.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, clearSessionCookie())
	writeJSON(w, http.StatusOK, map[string]string{"message": "logout success"})
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
		Id:            log.ID,
		Result:        LoginLogRecordResult(log.Result),
		SessionId:     optionalString(log.SessionID),
		UserAgent:     optionalString(log.UserAgent),
		UserId:        log.UserID,
		Username:      log.Username,
	}
}

func toGeneratedUserSummary(user *User) UserSummary {
	return UserSummary{
		Id:       uuid.NewSHA1(uuid.NameSpaceOID, []byte(strconv.FormatInt(user.ID, 10))),
		Status:   UserSummaryStatus(user.Status),
		Username: user.Username,
	}
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
