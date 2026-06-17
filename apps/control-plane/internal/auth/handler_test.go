package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
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

func TestHTTPHandlerCreatesManagedUserWithSelectableTeams(t *testing.T) {
	_, _, handler, token := newAuthenticatedHandler(t)
	teamA := uuid.New()
	teamB := uuid.New()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(fmt.Sprintf(`{
		"username":"zhoumin",
		"display_name":"周敏",
		"password":"secret",
		"avatar":{"provider":"dicebear","style":"adventurer","seed":"user:zhoumin"},
		"selectable_team_ids":["%s","%s"]
	}`, teamA, teamB)))
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.CreateUser(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response UserResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.User.DisplayName == nil || *response.User.DisplayName != "周敏" {
		t.Fatalf("expected display name in response, got %#v", response.User)
	}
	if response.User.Avatar.Seed != "user:zhoumin" {
		t.Fatalf("expected human avatar config in response, got %#v", response.User)
	}
}

func TestHTTPHandlerDeniedCreateUserWithSelectableTeamsDoesNotCreateUserOrScopes(t *testing.T) {
	repo, svc, _, token := newAuthenticatedHandler(t)
	authorizer := &recordingAuthorizer{allowed: false}
	handler := NewHandler(svc, authorizer)
	teamID := uuid.New()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(fmt.Sprintf(`{
		"username":"denied-create",
		"display_name":"Denied Create",
		"password":"secret",
		"avatar":{"provider":"dicebear","style":"adventurer","seed":"user:denied-create"},
		"selectable_team_ids":["%s"]
	}`, teamID)))
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.CreateUser(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, ok := repo.users["denied-create"]; ok {
		t.Fatalf("expected denied create not to persist user, got %#v", repo.users["denied-create"])
	}
	for userID, teamIDs := range repo.scopeTeamIDs {
		t.Fatalf("expected denied create not to persist scopes, got %s => %#v", userID, teamIDs)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authz check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Action != authz.ActionUserProjectTeamScopeManage || check.Resource.Type != authz.ResourceTenant || check.Resource.ID != DefaultTenantID {
		t.Fatalf("unexpected authz check: %#v", check)
	}
}

func TestHTTPHandlerListsUserProjectTeamScopes(t *testing.T) {
	repo, svc, handler, token := newAuthenticatedHandler(t)
	target, err := svc.CreateUser(t.Context(), "scope-target", "secret")
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}
	teamA := uuid.New()
	teamB := uuid.New()
	repo.scopeTeamIDs[target.ID] = []uuid.UUID{teamA, teamB}

	request := httptest.NewRequest(http.MethodGet, "/api/auth/users/"+target.ID.String()+"/project-team-scopes", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.ListUserProjectTeamScopes(recorder, request, openapiUUID(target.ID))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response UserProjectTeamScopeListResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Items) != 2 || uuid.UUID(response.Items[0].TeamId) != teamA || uuid.UUID(response.Items[1].TeamId) != teamB {
		t.Fatalf("expected listed scopes for both teams, got %#v", response.Items)
	}
}

func TestHTTPHandlerDeniedListUserProjectTeamScopesDoesNotReadScopes(t *testing.T) {
	repo, svc, _, token := newAuthenticatedHandler(t)
	target, err := svc.CreateUser(t.Context(), "denied-list-target", "secret")
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}
	teamID := uuid.New()
	repo.scopeTeamIDs[target.ID] = []uuid.UUID{teamID}
	authorizer := &recordingAuthorizer{allowed: false}
	handler := NewHandler(svc, authorizer)

	request := httptest.NewRequest(http.MethodGet, "/api/auth/users/"+target.ID.String()+"/project-team-scopes", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.ListUserProjectTeamScopes(recorder, request, openapiUUID(target.ID))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authz check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Action != authz.ActionUserProjectTeamScopeRead || check.Resource.Type != authz.ResourceTenant || check.Resource.ID != DefaultTenantID {
		t.Fatalf("unexpected authz check: %#v", check)
	}
}

func TestHTTPHandlerReplacesUserProjectTeamScopes(t *testing.T) {
	repo, svc, handler, token := newAuthenticatedHandler(t)
	target, err := svc.CreateUser(t.Context(), "replace-target", "secret")
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}
	teamA := uuid.New()
	teamB := uuid.New()
	request := httptest.NewRequest(http.MethodPut, "/api/auth/users/"+target.ID.String()+"/project-team-scopes", bytes.NewBufferString(fmt.Sprintf(`{
		"team_ids":["%s","%s"]
	}`, teamA, teamB)))
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.ReplaceUserProjectTeamScopes(recorder, request, openapiUUID(target.ID))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := repo.scopeTeamIDs[target.ID]; len(got) != 2 || got[0] != teamA || got[1] != teamB {
		t.Fatalf("expected replacement scopes to be persisted, got %#v", got)
	}
	var response UserProjectTeamScopeListResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Items) != 2 {
		t.Fatalf("expected response scopes, got %#v", response.Items)
	}
}

func TestHTTPHandlerDeniedReplaceUserProjectTeamScopesDoesNotMutateScopes(t *testing.T) {
	repo, svc, _, token := newAuthenticatedHandler(t)
	target, err := svc.CreateUser(t.Context(), "denied-replace-target", "secret")
	if err != nil {
		t.Fatalf("create target user: %v", err)
	}
	existingTeamID := uuid.New()
	repo.scopeTeamIDs[target.ID] = []uuid.UUID{existingTeamID}
	authorizer := &recordingAuthorizer{allowed: false}
	handler := NewHandler(svc, authorizer)
	newTeamID := uuid.New()
	request := httptest.NewRequest(http.MethodPut, "/api/auth/users/"+target.ID.String()+"/project-team-scopes", bytes.NewBufferString(fmt.Sprintf(`{
		"team_ids":["%s"]
	}`, newTeamID)))
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.ReplaceUserProjectTeamScopes(recorder, request, openapiUUID(target.ID))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := repo.scopeTeamIDs[target.ID]; len(got) != 1 || got[0] != existingTeamID {
		t.Fatalf("expected scopes not to be mutated, got %#v", got)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authz check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Action != authz.ActionUserProjectTeamScopeManage || check.Resource.Type != authz.ResourceTenant || check.Resource.ID != DefaultTenantID {
		t.Fatalf("unexpected authz check: %#v", check)
	}
}

func TestHTTPHandlerReplaceUserProjectTeamScopesMissingTargetUserReturnsNotFound(t *testing.T) {
	_, _, handler, token := newAuthenticatedHandler(t)
	missingUserID := uuid.New()
	teamID := uuid.New()
	request := httptest.NewRequest(http.MethodPut, "/api/auth/users/"+missingUserID.String()+"/project-team-scopes", bytes.NewBufferString(fmt.Sprintf(`{
		"team_ids":["%s"]
	}`, teamID)))
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	recorder := httptest.NewRecorder()

	handler.ReplaceUserProjectTeamScopes(recorder, request, openapiUUID(missingUserID))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
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

type recordingAuthorizer struct {
	allowed bool
	err     error
	checks  []authz.CheckRequest
}

func (a *recordingAuthorizer) Check(ctx context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	if a.err != nil {
		return authz.Decision{}, a.err
	}
	if a.allowed {
		return authz.Decision{Allowed: true, Reason: authz.ReasonAllowed, MatchedRule: "test.allow"}, nil
	}
	return authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership, RequiresAudit: true}, nil
}
