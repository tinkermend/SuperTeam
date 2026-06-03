package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type mockRepo struct {
	users         map[string]*User
	usersByID     map[uuid.UUID]*User
	runtimeTokens map[string]*RuntimeToken
	sessions      map[string]*Session
	loginLogs     []mockLoginLog
	operationLogs []mockOperationLog
	lastListUsersFilter ListUsersFilter
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:         make(map[string]*User),
		usersByID:     make(map[uuid.UUID]*User),
		runtimeTokens: make(map[string]*RuntimeToken),
		sessions:      make(map[string]*Session),
		loginLogs:     []mockLoginLog{},
		operationLogs: []mockOperationLog{},
	}
}

type mockLoginLog struct {
	EventType     string
	UserID        *uuid.UUID
	Username      string
	SessionID     *uuid.UUID
	ClientIP      string
	UserAgent     string
	Result        string
	FailureReason string
}

type mockOperationLog struct {
	UserID       *uuid.UUID
	Username     string
	Module       string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
}

func (m *mockRepo) CreateUser(ctx context.Context, username, passwordHash string) (*User, error) {
	user := &User{
		ID:           uuid.New(),
		Username:     username,
		PasswordHash: passwordHash,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	m.users[username] = user
	m.usersByID[user.ID] = user
	return user, nil
}

func (m *mockRepo) ListUsers(ctx context.Context, filter ListUsersFilter) ([]*User, error) {
	m.lastListUsersFilter = filter
	users := make([]*User, 0, len(m.usersByID))
	for _, user := range m.usersByID {
		if filter.Status != "" && user.Status != filter.Status {
			continue
		}
		users = append(users, user)
	}
	return users, nil
}

func (m *mockRepo) UpdateUserStatus(ctx context.Context, userID uuid.UUID, status string) (*User, error) {
	user, ok := m.usersByID[userID]
	if !ok {
		return nil, errors.New("user not found")
	}
	user.Status = status
	user.UpdatedAt = time.Now()
	return user, nil
}

func (m *mockRepo) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (*User, error) {
	user, ok := m.usersByID[userID]
	if !ok {
		return nil, errors.New("user not found")
	}
	user.PasswordHash = passwordHash
	user.UpdatedAt = time.Now()
	return user, nil
}

func (m *mockRepo) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	user, ok := m.users[username]
	if !ok {
		return nil, errors.New("user not found")
	}
	return user, nil
}

func (m *mockRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	user, ok := m.usersByID[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return user, nil
}

func (m *mockRepo) CreateRuntimeToken(ctx context.Context, nodeID, tokenHash string, expiresAt time.Time) error {
	m.runtimeTokens[nodeID] = &RuntimeToken{
		NodeID:    nodeID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	return nil
}

func (m *mockRepo) GetRuntimeTokenByNodeID(ctx context.Context, nodeID string) (*RuntimeToken, error) {
	token, ok := m.runtimeTokens[nodeID]
	if !ok {
		return nil, errors.New("token not found")
	}
	return token, nil
}

func (m *mockRepo) CreateSession(ctx context.Context, session *Session, token string) error {
	session.ID = uuid.New()
	m.sessions[token] = session
	return nil
}

func (m *mockRepo) GetSessionByTokenHash(ctx context.Context, token string) (*Session, error) {
	session, ok := m.sessions[token]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

func (m *mockRepo) DeleteSession(ctx context.Context, token string) error {
	delete(m.sessions, token)
	return nil
}

func (m *mockRepo) UpdateSessionLastSeen(ctx context.Context, token string, lastSeenAt time.Time) error {
	session, ok := m.sessions[token]
	if !ok {
		return ErrSessionNotFound
	}
	session.LastSeenAt = lastSeenAt
	return nil
}

func (m *mockRepo) CreateLoginLog(ctx context.Context, params CreateLoginLogParams) error {
	m.loginLogs = append(m.loginLogs, mockLoginLog{
		EventType:     params.EventType,
		UserID:        params.UserID,
		Username:      params.Username,
		SessionID:     params.SessionID,
		ClientIP:      params.ClientIP,
		UserAgent:     params.UserAgent,
		Result:        params.Result,
		FailureReason: params.FailureReason,
	})
	return nil
}

func (m *mockRepo) ListLoginLogs(ctx context.Context, filter ListLoginLogsFilter) ([]LoginLog, error) {
	return []LoginLog{}, nil
}

func (m *mockRepo) CreateOperationLog(ctx context.Context, params CreateOperationLogParams) error {
	m.operationLogs = append(m.operationLogs, mockOperationLog{
		UserID:       params.UserID,
		Username:     params.Username,
		Module:       params.Module,
		ResourceType: params.ResourceType,
		ResourceID:   params.ResourceID,
		Action:       params.Action,
		Result:       params.Result,
	})
	return nil
}

func TestNewService(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("expected error with nil repo")
	}
	if _, err := NewService(newMockRepo()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateUser(t *testing.T) {
	svc, _ := NewService(newMockRepo())
	user, err := svc.CreateUser(context.Background(), "test", "password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "test" {
		t.Errorf("expected username test, got %s", user.Username)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("password")); err != nil {
		t.Error("password hash mismatch")
	}
}

func TestListUsersUsesStatusFilter(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	activeUser, err := svc.CreateUser(context.Background(), "active-user", "password")
	if err != nil {
		t.Fatalf("create active user: %v", err)
	}
	disabledUser, err := svc.CreateUser(context.Background(), "disabled-user", "password")
	if err != nil {
		t.Fatalf("create disabled user: %v", err)
	}
	if _, err := repo.UpdateUserStatus(context.Background(), disabledUser.ID, UserStatusDisabled); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	users, err := svc.ListUsers(context.Background(), ListUsersFilter{Status: UserStatusActive})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 || users[0].ID != activeUser.ID {
		t.Fatalf("expected only active user %s, got %#v", activeUser.ID, users)
	}
}

func TestListUsersNormalizesSearchQuery(t *testing.T) {
	repo := &mockRepo{}
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := svc.ListUsers(context.Background(), ListUsersFilter{
		Q:      "  owner@example.com  ",
		Status: UserStatusActive,
		Limit:  200,
		Offset: -5,
	}); err != nil {
		t.Fatalf("list users: %v", err)
	}

	if repo.lastListUsersFilter.Q != "owner@example.com" {
		t.Fatalf("expected trimmed query, got %q", repo.lastListUsersFilter.Q)
	}
	if repo.lastListUsersFilter.Status != UserStatusActive {
		t.Fatalf("expected active status filter, got %q", repo.lastListUsersFilter.Status)
	}
	if repo.lastListUsersFilter.Limit != 20 || repo.lastListUsersFilter.Offset != 0 {
		t.Fatalf("expected normalized pagination 20/0, got %d/%d", repo.lastListUsersFilter.Limit, repo.lastListUsersFilter.Offset)
	}
}

func TestCreateManagedUserRecordsOperationLog(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	created, err := svc.CreateManagedUser(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, CreateManagedUserInput{
		Username: "operator",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("create managed user: %v", err)
	}
	if created.Username != "operator" || created.Status != UserStatusActive {
		t.Fatalf("unexpected created user: %#v", created)
	}
	if len(repo.operationLogs) != 1 {
		t.Fatalf("expected operation log, got %d", len(repo.operationLogs))
	}
	log := repo.operationLogs[0]
	if log.Action != OperationActionUserCreate || log.ResourceID != created.ID.String() || log.Result != OperationResultSucceeded {
		t.Fatalf("unexpected operation log: %#v", log)
	}
	if log.UserID == nil || *log.UserID != actor.ID || log.Username != actor.Username {
		t.Fatalf("expected actor in operation log, got %#v", log)
	}
}

func TestUpdateManagedUserStatusRecordsOperationLog(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	target, err := svc.CreateUser(context.Background(), "operator", "secret")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	disabled, err := svc.UpdateManagedUserStatus(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, target.ID, UserStatusDisabled)
	if err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if disabled.Status != UserStatusDisabled {
		t.Fatalf("expected disabled status, got %q", disabled.Status)
	}
	enabled, err := svc.UpdateManagedUserStatus(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, target.ID, UserStatusActive)
	if err != nil {
		t.Fatalf("enable user: %v", err)
	}
	if enabled.Status != UserStatusActive {
		t.Fatalf("expected active status, got %q", enabled.Status)
	}
	if len(repo.operationLogs) != 2 {
		t.Fatalf("expected two operation logs, got %d", len(repo.operationLogs))
	}
	if repo.operationLogs[0].Action != OperationActionUserDisable {
		t.Fatalf("expected disable operation, got %#v", repo.operationLogs[0])
	}
	if repo.operationLogs[1].Action != OperationActionUserEnable {
		t.Fatalf("expected enable operation, got %#v", repo.operationLogs[1])
	}
}

func TestResetManagedUserPasswordRecordsOperationLog(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	target, err := svc.CreateUser(context.Background(), "operator", "old-secret")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	updated, err := svc.ResetManagedUserPassword(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, target.ID, "new-secret")
	if err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("new-secret")); err != nil {
		t.Fatalf("expected new password hash, got %v", err)
	}
	if len(repo.operationLogs) != 1 {
		t.Fatalf("expected operation log, got %d", len(repo.operationLogs))
	}
	if repo.operationLogs[0].Action != OperationActionUserResetPassword {
		t.Fatalf("expected reset password operation, got %#v", repo.operationLogs[0])
	}
}

func TestAuthenticateUser(t *testing.T) {
	svc, _ := NewService(newMockRepo())
	svc.CreateUser(context.Background(), "test", "password")

	user, err := svc.AuthenticateUser(context.Background(), "test", "password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "test" {
		t.Errorf("expected username test, got %s", user.Username)
	}

	if _, err := svc.AuthenticateUser(context.Background(), "test", "wrong"); err != ErrInvalidCredentials {
		t.Error("expected invalid credentials error")
	}
}

func TestLoginCreatesSessionAndReturnsCurrentUser(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	createdUser, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	session, user, token, err := svc.Login(context.Background(), "admin", "admin", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" {
		t.Fatal("expected login token")
	}
	if user.ID != createdUser.ID {
		t.Fatalf("expected user %s, got %s", createdUser.ID, user.ID)
	}
	if session.UserID != createdUser.ID {
		t.Fatalf("expected session user %s, got %s", createdUser.ID, session.UserID)
	}
	if session.ID == uuid.Nil {
		t.Fatal("expected session id to be assigned by repository")
	}
	if session.ClientIP != "127.0.0.1" {
		t.Fatalf("expected client ip to be recorded, got %q", session.ClientIP)
	}
	if _, ok := repo.sessions[token]; ok {
		t.Fatal("expected session repository key to be a token hash, got raw token")
	}

	currentSession, currentUser, err := svc.GetUserBySessionToken(context.Background(), token)
	if err != nil {
		t.Fatalf("get current user: %v", err)
	}
	if currentSession.ID != session.ID {
		t.Fatalf("expected session %q, got %q", session.ID, currentSession.ID)
	}
	if currentUser.Username != "admin" {
		t.Fatalf("expected admin user, got %q", currentUser.Username)
	}

	if len(repo.loginLogs) != 1 {
		t.Fatalf("expected one login log, got %d", len(repo.loginLogs))
	}
	log := repo.loginLogs[0]
	if log.EventType != LoginEventSucceeded {
		t.Fatalf("expected event type %q, got %q", LoginEventSucceeded, log.EventType)
	}
	if log.UserID == nil || *log.UserID != createdUser.ID {
		t.Fatalf("expected user id %s, got %#v", createdUser.ID, log.UserID)
	}
	if log.Username != "admin" {
		t.Fatalf("expected username admin, got %q", log.Username)
	}
	if log.SessionID == nil || *log.SessionID != session.ID {
		t.Fatalf("expected session id %q, got %q", session.ID, log.SessionID)
	}
	if log.ClientIP != "127.0.0.1" {
		t.Fatalf("expected client ip 127.0.0.1, got %q", log.ClientIP)
	}
	if log.UserAgent != "test-agent" {
		t.Fatalf("expected user agent test-agent, got %q", log.UserAgent)
	}
	if log.Result != LoginResultSucceeded {
		t.Fatalf("expected result %q, got %q", LoginResultSucceeded, log.Result)
	}
}

func TestLogoutDeletesSession(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	if _, err := svc.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, _, token, err := svc.Login(context.Background(), "admin", "admin", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := svc.Logout(context.Background(), token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, _, err := svc.GetUserBySessionToken(context.Background(), token); err != ErrSessionNotFound {
		t.Fatalf("expected deleted session to be unavailable, got %v", err)
	}
	if len(repo.loginLogs) != 2 {
		t.Fatalf("expected login and logout logs, got %d", len(repo.loginLogs))
	}
	log := repo.loginLogs[1]
	if log.EventType != LoginEventLogoutSucceeded {
		t.Fatalf("expected event type %q, got %q", LoginEventLogoutSucceeded, log.EventType)
	}
	if log.Result != LoginResultSucceeded {
		t.Fatalf("expected result %q, got %q", LoginResultSucceeded, log.Result)
	}
	if log.ClientIP != "127.0.0.1" {
		t.Fatalf("expected logout client ip 127.0.0.1, got %q", log.ClientIP)
	}
}

func TestLoginRecordsFailedAttempt(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	if _, err := svc.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, _, _, err := svc.Login(context.Background(), "admin", "wrong", "127.0.0.1", "test-agent")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials, got %v", err)
	}

	if len(repo.loginLogs) != 1 {
		t.Fatalf("expected one failed login log, got %d", len(repo.loginLogs))
	}
	log := repo.loginLogs[0]
	if log.EventType != LoginEventFailed {
		t.Fatalf("expected event type %q, got %q", LoginEventFailed, log.EventType)
	}
	if log.UserID != nil {
		t.Fatalf("expected failed log to avoid binding a user id, got %#v", log.UserID)
	}
	if log.Username != "admin" {
		t.Fatalf("expected attempted username admin, got %q", log.Username)
	}
	if log.Result != LoginResultFailed {
		t.Fatalf("expected result %q, got %q", LoginResultFailed, log.Result)
	}
	if log.FailureReason != LoginFailureInvalidCredentials {
		t.Fatalf("expected failure reason %q, got %q", LoginFailureInvalidCredentials, log.FailureReason)
	}
}

func TestGenerateRuntimeToken(t *testing.T) {
	svc, _ := NewService(newMockRepo())
	expiresAt := time.Now().Add(24 * time.Hour)
	token, err := svc.GenerateRuntimeToken(context.Background(), "node1", expiresAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestValidateRuntimeToken(t *testing.T) {
	svc, _ := NewService(newMockRepo())
	expiresAt := time.Now().Add(24 * time.Hour)
	token, _ := svc.GenerateRuntimeToken(context.Background(), "node1", expiresAt)

	if err := svc.ValidateRuntimeToken(context.Background(), "node1", token); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := svc.ValidateRuntimeToken(context.Background(), "node1", "invalid"); err != ErrInvalidToken {
		t.Error("expected invalid token error")
	}
}
