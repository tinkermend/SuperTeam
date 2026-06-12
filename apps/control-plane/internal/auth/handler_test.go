package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHTTPHandlerUpdatesCurrentUserProfile(t *testing.T) {
	repo, svc, handler, token := newAuthenticatedHandler(t)

	request := httptest.NewRequest(http.MethodPatch, "/api/auth/account/profile", bytes.NewBufferString(`{
		"display_name": "值班负责人",
		"email": "operator@example.com",
		"avatar": {
			"provider": "dicebear",
			"style": "adventurer",
			"seed": "operator-v2",
			"options": {"backgroundColor": "b6e3f4"}
		}
	}`))
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.UpdateCurrentUserProfile(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response UserResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.User.DisplayName == nil || *response.User.DisplayName != "值班负责人" {
		t.Fatalf("expected display name in response, got %#v", response.User)
	}
	if response.User.Email == nil || *response.User.Email != "operator@example.com" {
		t.Fatalf("expected email in response, got %#v", response.User)
	}
	if response.User.Avatar.Seed != "operator-v2" {
		t.Fatalf("expected avatar seed update, got %#v", response.User.Avatar)
	}
	if repo.operationLogs[0].Action != OperationActionUserUpdateOwnProfile {
		t.Fatalf("expected profile operation log, got %#v", repo.operationLogs)
	}
	_ = svc
}

func TestHTTPHandlerChangesCurrentUserPassword(t *testing.T) {
	_, svc, handler, token := newAuthenticatedHandler(t)

	request := httptest.NewRequest(http.MethodPost, "/api/auth/account/password", bytes.NewBufferString(`{
		"current_password": "secret",
		"password": "new-secret"
	}`))
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.ChangeCurrentUserPassword(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, err := svc.AuthenticateUser(request.Context(), "operator", "secret"); err == nil {
		t.Fatal("old password should not authenticate after password change")
	}
	if _, err := svc.AuthenticateUser(request.Context(), "operator", "new-secret"); err != nil {
		t.Fatalf("new password should authenticate: %v", err)
	}
}

func TestHTTPHandlerRejectsWrongCurrentPassword(t *testing.T) {
	_, _, handler, token := newAuthenticatedHandler(t)

	request := httptest.NewRequest(http.MethodPost, "/api/auth/account/password", bytes.NewBufferString(`{
		"current_password": "wrong",
		"password": "new-secret"
	}`))
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.ChangeCurrentUserPassword(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestHTTPHandlerListsCurrentUserLoginLogs(t *testing.T) {
	repo, _, handler, token := newAuthenticatedHandler(t)
	current := repo.users["operator"]
	other := &User{
		ID:           uuid.New(),
		Username:     "auditor",
		PasswordHash: "hash",
		Status:       UserStatusActive,
		Avatar:       defaultUserAvatarConfig("auditor"),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	repo.users[other.Username] = other
	repo.usersByID[other.ID] = other
	repo.loginLogs = append(repo.loginLogs, LoginLog{
		ID:        uuid.New(),
		EventType: LoginEventSucceeded,
		UserID:    &other.ID,
		Username:  other.Username,
		Result:    LoginResultSucceeded,
		CreatedAt: time.Now(),
	})

	request := httptest.NewRequest(http.MethodGet, "/api/auth/account/login-logs?limit=20&offset=0", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.ListCurrentUserLoginLogs(recorder, request, ListCurrentUserLoginLogsParams{Limit: ptrInt32(20), Offset: ptrInt32(0)})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response LoginLogListResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Items) == 0 {
		t.Fatal("expected at least one current user login log")
	}
	for _, item := range response.Items {
		if item.UserId == nil || uuid.UUID(*item.UserId) != current.ID {
			t.Fatalf("expected only current user log items, got %#v", response.Items)
		}
	}
}

func newAuthenticatedHandler(t *testing.T) (*mockRepo, *Service, *HTTPHandler, string) {
	t.Helper()
	repo := newMockRepo()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateUser(t.Context(), "operator", "secret"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, _, token, err := svc.Login(t.Context(), "operator", "secret", "127.0.0.1", "Chrome 125")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return repo, svc, NewHandler(svc), token
}

func ptrInt32(value int32) *int32 {
	return &value
}
