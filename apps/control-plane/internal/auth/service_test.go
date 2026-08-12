package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/superteam/control-plane/internal/platform"
)

type mockRepo struct {
	users                      map[string]*User
	usersByID                  map[uuid.UUID]*User
	runtimeTokens              map[string]*RuntimeToken
	sessions                   map[string]*Session
	loginLogs                  []LoginLog
	operationLogs              []mockOperationLog
	captchaChallenges          map[uuid.UUID]*CaptchaChallengeRecord
	captchaConsumeCalls        []uuid.UUID
	lastCaptchaCleanup         *time.Time
	failCaptchaGet             error
	failCaptchaConsume         error
	failCaptchaCleanup         error
	failLoginLogInTx           bool
	scopeTeamIDs               map[uuid.UUID][]uuid.UUID
	invalidTeamIDs             map[uuid.UUID]bool
	failScopeReplace           error
	tenantMemberships          map[uuid.UUID]*TenantLevelMembership
	lastListUsersFilter        ListUsersFilter
	lastLoginLogFilter         ListLoginLogsFilter
	transactionCalls           int
	inTransaction              bool
	updateSessionLastSeenCalls int
}

type recordingProjectTeamScopeSyncer struct {
	calls []projectTeamScopeSyncCall
	err   error
}

type projectTeamScopeSyncCall struct {
	tenantID uuid.UUID
	userID   uuid.UUID
	teamID   uuid.UUID
	status   string
}

func (s *recordingProjectTeamScopeSyncer) SyncProjectTeamScope(ctx context.Context, tenantID, userID, teamID uuid.UUID, status string) error {
	s.calls = append(s.calls, projectTeamScopeSyncCall{
		tenantID: tenantID,
		userID:   userID,
		teamID:   teamID,
		status:   status,
	})
	return s.err
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:             make(map[string]*User),
		usersByID:         make(map[uuid.UUID]*User),
		runtimeTokens:     make(map[string]*RuntimeToken),
		sessions:          make(map[string]*Session),
		loginLogs:         []LoginLog{},
		operationLogs:     []mockOperationLog{},
		captchaChallenges: make(map[uuid.UUID]*CaptchaChallengeRecord),
		scopeTeamIDs:      make(map[uuid.UUID][]uuid.UUID),
		invalidTeamIDs:    make(map[uuid.UUID]bool),
	}
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

func (m *mockRepo) WithTransaction(ctx context.Context, fn func(Repository) error) error {
	m.transactionCalls++
	m.inTransaction = true
	defer func() { m.inTransaction = false }()
	users := make(map[string]*User, len(m.users))
	for key, value := range m.users {
		copied := *value
		users[key] = &copied
	}
	usersByID := make(map[uuid.UUID]*User, len(m.usersByID))
	for key, value := range m.usersByID {
		copied := *value
		usersByID[key] = &copied
	}
	scopeTeamIDs := make(map[uuid.UUID][]uuid.UUID, len(m.scopeTeamIDs))
	for key, value := range m.scopeTeamIDs {
		scopeTeamIDs[key] = append([]uuid.UUID(nil), value...)
	}
	captchaChallenges := make(map[uuid.UUID]*CaptchaChallengeRecord, len(m.captchaChallenges))
	for key, value := range m.captchaChallenges {
		copied := *value
		captchaChallenges[key] = &copied
	}
	tenantMemberships := make(map[uuid.UUID]*TenantLevelMembership, len(m.tenantMemberships))
	for key, value := range m.tenantMemberships {
		copied := *value
		tenantMemberships[key] = &copied
	}
	captchaConsumeCalls := append([]uuid.UUID(nil), m.captchaConsumeCalls...)
	if err := fn(m); err != nil {
		m.users = users
		m.usersByID = usersByID
		m.scopeTeamIDs = scopeTeamIDs
		m.captchaChallenges = captchaChallenges
		m.tenantMemberships = tenantMemberships
		m.captchaConsumeCalls = captchaConsumeCalls
		return err
	}
	return nil
}

func (m *mockRepo) CreateUser(ctx context.Context, input CreateUserRecordInput) (*User, error) {
	user := &User{
		ID:            uuid.New(),
		Username:      input.Username,
		DisplayName:   input.DisplayName,
		Email:         input.Email,
		PasswordHash:  input.PasswordHash,
		Status:        "active",
		Avatar:        normalizeUserAvatarConfig(input.Username, input.Avatar),
		AvatarAssetID: input.AvatarAssetID,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	m.users[input.Username] = user
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

func (m *mockRepo) SetUserAvatarSVG(ctx context.Context, userID uuid.UUID, svg string) error {
	user, ok := m.usersByID[userID]
	if !ok {
		return errors.New("user not found")
	}
	user.Avatar.SVG = svg
	return nil
}

func (m *mockRepo) UpdateUserProfile(ctx context.Context, userID uuid.UUID, input UpdateUserProfileInput) (*User, error) {
	user, ok := m.usersByID[userID]
	if !ok {
		return nil, errors.New("user not found")
	}
	user.DisplayName = input.DisplayName
	user.Email = input.Email
	user.Avatar = normalizeUserAvatarConfig(user.Username, input.Avatar)
	user.UpdatedAt = time.Now()
	return user, nil
}

func (m *mockRepo) UpdateUserContact(ctx context.Context, userID uuid.UUID, input UpdateUserContactInput) (*User, error) {
	user, ok := m.usersByID[userID]
	if !ok {
		return nil, errors.New("user not found")
	}
	if input.Email != nil {
		user.Email = *input.Email
	}
	if input.Mobile != nil {
		user.Mobile = *input.Mobile
	}
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
	m.updateSessionLastSeenCalls++
	session, ok := m.sessions[token]
	if !ok {
		return ErrSessionNotFound
	}
	session.LastSeenAt = lastSeenAt
	return nil
}

func (m *mockRepo) CreateLoginLog(ctx context.Context, params CreateLoginLogParams) error {
	if m.failLoginLogInTx && m.inTransaction {
		return errors.New("login log called inside transaction")
	}
	m.loginLogs = append(m.loginLogs, LoginLog{
		ID:            uuid.New(),
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
	m.lastLoginLogFilter = filter
	logs := make([]LoginLog, 0, len(m.loginLogs))
	for _, log := range m.loginLogs {
		if filter.UserID != nil {
			if log.UserID == nil || *log.UserID != *filter.UserID {
				continue
			}
		}
		logs = append(logs, log)
	}
	return logs, nil
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

func (m *mockRepo) ListOperationLogs(ctx context.Context, filter ListOperationLogsFilter) ([]OperationLog, error) {
	logs := make([]OperationLog, 0, len(m.operationLogs))
	for _, log := range m.operationLogs {
		if filter.UserID != nil {
			if log.UserID == nil || *log.UserID != *filter.UserID {
				continue
			}
		}
		if filter.Module != "" && log.Module != filter.Module {
			continue
		}
		if filter.ExcludeModule != "" && log.Module == filter.ExcludeModule {
			continue
		}
		if filter.Action != "" && log.Action != filter.Action {
			continue
		}
		if filter.Result != "" && log.Result != filter.Result {
			continue
		}
		logs = append(logs, OperationLog{
			UserID:       log.UserID,
			Username:     log.Username,
			Module:       log.Module,
			ResourceType: log.ResourceType,
			ResourceID:   log.ResourceID,
			Action:       log.Action,
			Result:       log.Result,
		})
	}
	return logs, nil
}

func (m *mockRepo) CreateCaptchaChallenge(ctx context.Context, params CreateCaptchaChallengeParams) (*CaptchaChallengeRecord, error) {
	record := &CaptchaChallengeRecord{
		ID:         params.ID,
		TenantID:   params.TenantID,
		AnswerHash: params.AnswerHash,
		ExpiresAt:  params.ExpiresAt,
		ClientIP:   params.ClientIP,
		UserAgent:  params.UserAgent,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	m.captchaChallenges[record.ID] = record
	return record, nil
}

func (m *mockRepo) GetCaptchaChallengeForUpdate(ctx context.Context, id uuid.UUID) (*CaptchaChallengeRecord, error) {
	if m.failCaptchaGet != nil {
		return nil, m.failCaptchaGet
	}
	record, ok := m.captchaChallenges[id]
	if !ok {
		return nil, ErrCaptchaInvalid
	}
	copied := *record
	return &copied, nil
}

func (m *mockRepo) ConsumeCaptchaChallenge(ctx context.Context, id uuid.UUID, usedAt time.Time) error {
	m.captchaConsumeCalls = append(m.captchaConsumeCalls, id)
	if m.failCaptchaConsume != nil {
		return m.failCaptchaConsume
	}
	record, ok := m.captchaChallenges[id]
	if !ok {
		return ErrCaptchaInvalid
	}
	if record.UsedAt != nil {
		return ErrCaptchaUsed
	}
	record.UsedAt = &usedAt
	record.UpdatedAt = usedAt
	return nil
}

func (m *mockRepo) DeleteExpiredCaptchaChallenges(ctx context.Context, before time.Time) error {
	m.lastCaptchaCleanup = &before
	if m.failCaptchaCleanup != nil {
		return m.failCaptchaCleanup
	}
	for id, record := range m.captchaChallenges {
		if record.ExpiresAt.Before(before) {
			delete(m.captchaChallenges, id)
		}
	}
	return nil
}

func (m *mockRepo) ReplaceUserProjectTeamScopes(ctx context.Context, tenantID, userID, grantedByUserID uuid.UUID, teamIDs []uuid.UUID) ([]UserProjectTeamScopeSummary, error) {
	if m.failScopeReplace != nil {
		return nil, m.failScopeReplace
	}
	copied := append([]uuid.UUID(nil), teamIDs...)
	m.scopeTeamIDs[userID] = copied
	scopes := make([]UserProjectTeamScopeSummary, 0, len(copied))
	now := time.Now()
	for _, teamID := range copied {
		scopes = append(scopes, UserProjectTeamScopeSummary{
			ID:              uuid.New(),
			TenantID:        tenantID,
			UserID:          userID,
			TeamID:          teamID,
			Status:          "active",
			GrantedByUserID: &grantedByUserID,
			CreatedAt:       now,
			UpdatedAt:       now,
			Team: UserProjectTeamScopeTeamSummary{
				ID:               teamID,
				Slug:             "team-" + teamID.String()[:8],
				Name:             "Team " + teamID.String()[:8],
				Status:           "active",
				GovernanceStatus: "not_configured",
				HumanOwners:      []UserProjectTeamScopeOwnerSummary{},
			},
		})
	}
	return scopes, nil
}

func (m *mockRepo) ListUserProjectTeamScopes(ctx context.Context, tenantID, userID uuid.UUID) ([]UserProjectTeamScopeSummary, error) {
	teamIDs := m.scopeTeamIDs[userID]
	scopes := make([]UserProjectTeamScopeSummary, 0, len(teamIDs))
	now := time.Now()
	for _, teamID := range teamIDs {
		scopes = append(scopes, UserProjectTeamScopeSummary{
			ID:        uuid.New(),
			TenantID:  tenantID,
			UserID:    userID,
			TeamID:    teamID,
			Status:    "active",
			CreatedAt: now,
			UpdatedAt: now,
			Team: UserProjectTeamScopeTeamSummary{
				ID:               teamID,
				Slug:             "team-" + teamID.String()[:8],
				Name:             "Team " + teamID.String()[:8],
				Status:           "active",
				GovernanceStatus: "not_configured",
				HumanOwners:      []UserProjectTeamScopeOwnerSummary{},
			},
		})
	}
	return scopes, nil
}

func (m *mockRepo) CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error) {
	for _, scopedTeamID := range m.scopeTeamIDs[userID] {
		if scopedTeamID == teamID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockRepo) EnsureActiveUser(ctx context.Context, userID uuid.UUID) error {
	user, ok := m.usersByID[userID]
	if !ok || user.Status != UserStatusActive {
		return ErrManagedUserNotFound
	}
	return nil
}

func (m *mockRepo) ValidateActiveTenantTeamIDs(ctx context.Context, tenantID uuid.UUID, teamIDs []uuid.UUID) error {
	if len(teamIDs) == 0 {
		return ErrInvalidManagedUserInput
	}
	for _, teamID := range teamIDs {
		if teamID == uuid.Nil || m.invalidTeamIDs[teamID] {
			return ErrInvalidManagedUserInput
		}
	}
	return nil
}

func (m *mockRepo) GetActiveTenantMembership(ctx context.Context, tenantID, userID uuid.UUID) (*TenantLevelMembership, error) {
	if m.tenantMemberships == nil {
		return nil, nil
	}
	membership, ok := m.tenantMemberships[userID]
	if !ok || membership.TenantID != tenantID || membership.Status != "active" {
		return nil, nil
	}
	copied := *membership
	return &copied, nil
}

func (m *mockRepo) UpsertTenantMembership(ctx context.Context, tenantID, userID uuid.UUID, role string) (*TenantLevelMembership, error) {
	if m.tenantMemberships == nil {
		m.tenantMemberships = make(map[uuid.UUID]*TenantLevelMembership)
	}
	now := time.Now().UTC()
	existing, _ := m.GetActiveTenantMembership(ctx, tenantID, userID)
	if existing != nil {
		existing.Role = role
		existing.Status = "active"
		existing.UpdatedAt = now
		m.tenantMemberships[userID] = existing
		copied := *existing
		return &copied, nil
	}
	membership := &TenantLevelMembership{
		ID:        uuid.New(),
		TenantID:  tenantID,
		UserID:    userID,
		Role:      role,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.tenantMemberships[userID] = membership
	copied := *membership
	return &copied, nil
}

func (m *mockRepo) DisableTenantMembership(ctx context.Context, tenantID, userID uuid.UUID) (*TenantLevelMembership, error) {
	existing, err := m.GetActiveTenantMembership(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrTenantMembershipNotFound
	}
	existing.Status = "disabled"
	existing.UpdatedAt = time.Now().UTC()
	m.tenantMemberships[userID] = existing
	copied := *existing
	return &copied, nil
}

func (m *mockRepo) CountActiveTenantOwners(ctx context.Context, tenantID uuid.UUID) (int32, error) {
	var count int32
	for _, membership := range m.tenantMemberships {
		if membership.TenantID == tenantID && membership.Role == TenantRoleOwner && membership.Status == "active" {
			count++
		}
	}
	return count, nil
}

func TestCaptchaEnabledDefaultsToFalse(t *testing.T) {
	repo := newMockRepo()
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if svc.IsCaptchaEnabled() {
		t.Fatal("expected captcha to be disabled by default")
	}
}

func TestValidateAndConsumeCaptchaSkipsValidationWhenDisabled(t *testing.T) {
	repo := newMockRepo()
	svc, err := NewService(repo,
		WithCaptchaOptions(CaptchaOptions{Secret: "test-captcha-secret"}),
		WithCaptchaEnabled(false),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.ValidateAndConsumeCaptcha(t.Context(), uuid.Nil, "", "operator", "127.0.0.1", "Chrome 125")

	if err != nil {
		t.Fatalf("expected disabled captcha validation to be skipped, got %v", err)
	}
	if repo.transactionCalls != 0 {
		t.Fatalf("expected no captcha transaction when disabled, got %d", repo.transactionCalls)
	}
	if len(repo.loginLogs) != 0 {
		t.Fatalf("expected no captcha failure log when disabled, got %#v", repo.loginLogs)
	}
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
		Username:          "operator",
		DisplayName:       "Operator",
		Password:          "secret",
		TenantRole:        TenantRoleMember,
		Avatar:            UserAvatarConfig{Provider: "dicebear", Style: "adventurer", Seed: "user:operator"},
		SelectableTeamIDs: []uuid.UUID{uuid.New()},
		TenantID:          platform.DefaultTenantID,
	})
	if err != nil {
		t.Fatalf("create managed user: %v", err)
	}
	if created.Username != "operator" || created.Status != UserStatusActive {
		t.Fatalf("unexpected created user: %#v", created)
	}
	if created.DisplayName != "Operator" || created.Avatar.Seed != "user:operator" || created.AvatarAssetID != "" {
		t.Fatalf("expected created user display name and human avatar to be preserved, got %#v", created)
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

func TestCreateManagedUserPersistsDisplayNameHumanAvatarAndScopes(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	teamA := uuid.New()
	teamB := uuid.New()

	created, err := svc.CreateManagedUser(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, CreateManagedUserInput{
		Username:          "zhoumin",
		DisplayName:       "周敏",
		Password:          "secret",
		TenantRole:        TenantRoleMember,
		Avatar:            UserAvatarConfig{Provider: "dicebear", Style: "adventurer", Seed: "user:zhoumin"},
		SelectableTeamIDs: []uuid.UUID{teamA, teamB},
		TenantID:          platform.DefaultTenantID,
	})
	if err != nil {
		t.Fatalf("create managed user: %v", err)
	}
	if created.DisplayName != "周敏" || created.Avatar.Seed != "user:zhoumin" || created.AvatarAssetID != "" {
		t.Fatalf("expected display name and human avatar, got %#v", created)
	}
	if got := repo.scopeTeamIDs[created.ID]; len(got) != 2 || got[0] != teamA || got[1] != teamB {
		t.Fatalf("expected scope team ids to be persisted, got %#v", got)
	}
}

func TestCreateManagedUserRollsBackCreatedUserWhenScopeReplacementFails(t *testing.T) {
	repo := newMockRepo()
	repo.failScopeReplace = errors.New("scope replace failed")
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	_, err = svc.CreateManagedUser(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, CreateManagedUserInput{
		Username:          "rollback-user",
		DisplayName:       "Rollback User",
		Password:          "secret",
		TenantRole:        TenantRoleMember,
		Avatar:            UserAvatarConfig{Provider: "dicebear", Style: "adventurer", Seed: "user:rollback-user"},
		SelectableTeamIDs: []uuid.UUID{uuid.New()},
		TenantID:          platform.DefaultTenantID,
	})
	if err == nil {
		t.Fatal("expected create managed user to fail")
	}
	if _, ok := repo.users["rollback-user"]; ok {
		t.Fatalf("expected user creation to be rolled back, got %#v", repo.users["rollback-user"])
	}
}

func TestCreateManagedUserRequiresSelectableTeams(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	_, err = svc.CreateManagedUser(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, CreateManagedUserInput{
		Username:    "empty-scope",
		DisplayName: "空范围",
		Password:    "secret",
		TenantRole:        TenantRoleMember,
		Avatar:      UserAvatarConfig{Provider: "dicebear", Style: "adventurer", Seed: "user:empty-scope"},
		TenantID:    platform.DefaultTenantID,
	})
	if !errors.Is(err, ErrInvalidManagedUserInput) {
		t.Fatalf("expected invalid managed user input, got %v", err)
	}
}

func TestReplaceUserProjectTeamScopesRejectsNilOrInvalidTeamIDs(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	target, err := svc.CreateUser(context.Background(), "target", "secret")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	tenantID := platform.DefaultTenantID
	invalidTeamID := uuid.New()
	repo.invalidTeamIDs[invalidTeamID] = true

	for _, tt := range []struct {
		name    string
		teamIDs []uuid.UUID
	}{
		{name: "nil team id", teamIDs: []uuid.UUID{uuid.Nil}},
		{name: "invalid team id", teamIDs: []uuid.UUID{invalidTeamID}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ReplaceUserProjectTeamScopes(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, tenantID, target.ID, tt.teamIDs)
			if !errors.Is(err, ErrInvalidManagedUserInput) {
				t.Fatalf("expected invalid managed user input, got %v", err)
			}
			if got := repo.scopeTeamIDs[target.ID]; len(got) != 0 {
				t.Fatalf("expected scopes not to be mutated, got %#v", got)
			}
		})
	}
}

func TestReplaceUserProjectTeamScopesSyncsOpenFGATuplesBestEffort(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	syncer := &recordingProjectTeamScopeSyncer{err: errors.New("openfga write failed")}
	svc.SetProjectTeamScopeSyncer(syncer)
	actor, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	target, err := svc.CreateUser(context.Background(), "target", "secret")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	tenantID := platform.DefaultTenantID
	oldTeamID := uuid.New()
	keptTeamID := uuid.New()
	newTeamID := uuid.New()
	repo.scopeTeamIDs[target.ID] = []uuid.UUID{oldTeamID, keptTeamID}

	scopes, err := svc.ReplaceUserProjectTeamScopes(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, tenantID, target.ID, []uuid.UUID{keptTeamID, newTeamID})

	if err != nil {
		t.Fatalf("replace scopes should keep DB result when OpenFGA sync fails: %v", err)
	}
	if len(scopes) != 2 {
		t.Fatalf("expected DB scopes to be returned, got %#v", scopes)
	}
	assertProjectTeamScopeSyncCall(t, syncer.calls, projectTeamScopeSyncCall{tenantID: tenantID, userID: target.ID, teamID: keptTeamID, status: "active"})
	assertProjectTeamScopeSyncCall(t, syncer.calls, projectTeamScopeSyncCall{tenantID: tenantID, userID: target.ID, teamID: newTeamID, status: "active"})
	assertProjectTeamScopeSyncCall(t, syncer.calls, projectTeamScopeSyncCall{tenantID: tenantID, userID: target.ID, teamID: oldTeamID, status: "revoked"})
}

func TestCreateManagedUserRejectsDigitalEmployeeAvatarAssetOnly(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	_, err = svc.CreateManagedUser(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, CreateManagedUserInput{
		Username:          "reviewer",
		DisplayName:       "Reviewer",
		Password:          "secret",
		TenantRole:        TenantRoleMember,
		AvatarAssetID:     " ENGINEER-F-01 ",
		SelectableTeamIDs: []uuid.UUID{uuid.New()},
		TenantID:          platform.DefaultTenantID,
	})
	if !errors.Is(err, ErrInvalidManagedUserInput) {
		t.Fatalf("expected invalid managed user input for digital employee avatar asset, got %v", err)
	}
}

func assertProjectTeamScopeSyncCall(t *testing.T, calls []projectTeamScopeSyncCall, want projectTeamScopeSyncCall) {
	t.Helper()
	for _, call := range calls {
		if call == want {
			return
		}
	}
	t.Fatalf("expected sync call %#v in %#v", want, calls)
}

func TestUpdateCurrentUserProfileOnlyMutatesActorAndRecordsOperation(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "operator", "secret")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	other, err := svc.CreateUser(context.Background(), "auditor", "secret")
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}

	updated, err := svc.UpdateCurrentUserProfile(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, UpdateUserProfileInput{
		DisplayName: "值班负责人",
		Email:       "operator@example.com",
		Avatar: UserAvatarConfig{
			Provider: "dicebear",
			Style:    "adventurer",
			Seed:     "operator-v2",
			Options:  map[string]any{"backgroundColor": "b6e3f4"},
		},
	})
	if err != nil {
		t.Fatalf("update current profile: %v", err)
	}
	if updated.ID != actor.ID || updated.DisplayName != "值班负责人" || updated.Email != "operator@example.com" {
		t.Fatalf("expected actor profile update, got %#v", updated)
	}
	if updated.Avatar.Seed != "operator-v2" || updated.Avatar.Options["backgroundColor"] != "b6e3f4" {
		t.Fatalf("expected avatar update, got %#v", updated.Avatar)
	}
	if repo.usersByID[other.ID].DisplayName != "" || repo.usersByID[other.ID].Email != "" {
		t.Fatalf("other user should not be mutated: %#v", repo.usersByID[other.ID])
	}
	if len(repo.operationLogs) != 1 {
		t.Fatalf("expected one operation log, got %d", len(repo.operationLogs))
	}
	log := repo.operationLogs[0]
	if log.Action != OperationActionUserUpdateOwnProfile || log.ResourceID != actor.ID.String() || log.Result != OperationResultSucceeded {
		t.Fatalf("unexpected operation log: %#v", log)
	}
}

func TestChangeCurrentUserPasswordRequiresCurrentPassword(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "operator", "old-secret")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	oldHash := actor.PasswordHash

	if _, err := svc.ChangeCurrentUserPassword(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, "wrong-secret", "new-secret"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials for wrong current password, got %v", err)
	}
	if actor.PasswordHash != oldHash {
		t.Fatal("password hash changed after invalid current password")
	}
	if len(repo.operationLogs) != 1 || repo.operationLogs[0].Action != OperationActionUserChangeOwnPassword || repo.operationLogs[0].Result != OperationResultFailed {
		t.Fatalf("expected failed password change operation log, got %#v", repo.operationLogs)
	}

	updated, err := svc.ChangeCurrentUserPassword(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, "old-secret", "new-secret")
	if err != nil {
		t.Fatalf("change current password: %v", err)
	}
	if updated.ID != actor.ID {
		t.Fatalf("expected actor user back, got %#v", updated)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("new-secret")); err != nil {
		t.Fatalf("new password hash mismatch: %v", err)
	}
	if len(repo.operationLogs) != 2 || repo.operationLogs[1].Action != OperationActionUserChangeOwnPassword || repo.operationLogs[1].Result != OperationResultSucceeded {
		t.Fatalf("expected succeeded password change operation log, got %#v", repo.operationLogs)
	}
}

func TestListCurrentUserLoginLogsFiltersToActor(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	actor, err := svc.CreateUser(context.Background(), "operator", "secret")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	other, err := svc.CreateUser(context.Background(), "auditor", "secret")
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	actorSessionID := uuid.New()
	otherSessionID := uuid.New()
	repo.loginLogs = append(repo.loginLogs,
		LoginLog{
			ID:        uuid.New(),
			EventType: LoginEventSucceeded,
			UserID:    &actor.ID,
			Username:  actor.Username,
			SessionID: &actorSessionID,
			Result:    LoginResultSucceeded,
			CreatedAt: time.Now(),
		},
		LoginLog{
			ID:        uuid.New(),
			EventType: LoginEventSucceeded,
			UserID:    &other.ID,
			Username:  other.Username,
			SessionID: &otherSessionID,
			Result:    LoginResultSucceeded,
			CreatedAt: time.Now(),
		},
	)

	logs, err := svc.ListCurrentUserLoginLogs(context.Background(), Actor{UserID: actor.ID, Username: actor.Username}, ListLoginLogsFilter{
		Limit:  200,
		Offset: -5,
	})
	if err != nil {
		t.Fatalf("list current user login logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Username != actor.Username {
		t.Fatalf("expected only actor login log, got %#v", logs)
	}
	if repo.lastLoginLogFilter.UserID == nil || *repo.lastLoginLogFilter.UserID != actor.ID {
		t.Fatalf("expected current user id filter, got %#v", repo.lastLoginLogFilter)
	}
	if repo.lastLoginLogFilter.Limit != 20 || repo.lastLoginLogFilter.Offset != 0 {
		t.Fatalf("expected normalized pagination 20/0, got %#v", repo.lastLoginLogFilter)
	}
}

func TestCreateCaptchaChallengeReturnsImageAndPersistsHash(t *testing.T) {
	repo := newMockRepo()
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		TTL:    5 * time.Minute,
		Now:    func() time.Time { return now },
	}), WithCaptchaEnabled(true))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	challenge, err := svc.CreateCaptcha(context.Background(), "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create captcha: %v", err)
	}
	if challenge.ID == uuid.Nil {
		t.Fatal("expected captcha id")
	}
	if !strings.HasPrefix(challenge.ImageDataURL, "data:image/png;base64,") {
		t.Fatalf("expected png data url, got %q", challenge.ImageDataURL)
	}
	if !challenge.ExpiresAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("expected ttl expiry, got %s", challenge.ExpiresAt)
	}
	if repo.lastCaptchaCleanup == nil || !repo.lastCaptchaCleanup.Equal(now) {
		t.Fatalf("expected captcha cleanup at %s, got %v", now, repo.lastCaptchaCleanup)
	}
	record := repo.captchaChallenges[challenge.ID]
	if record == nil {
		t.Fatal("expected persisted challenge")
	}
	if record.AnswerHash == "" || len(record.AnswerHash) < 32 {
		t.Fatalf("expected answer hash, got %q", record.AnswerHash)
	}
}

func TestCreateCaptchaContinuesWhenExpiredCleanupFails(t *testing.T) {
	repo := newMockRepo()
	repo.failCaptchaCleanup = errors.New("cleanup unavailable")
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		TTL:    5 * time.Minute,
		Now:    func() time.Time { return now },
	}), WithCaptchaEnabled(true))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	challenge, err := svc.CreateCaptcha(context.Background(), "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create captcha should ignore cleanup failure: %v", err)
	}
	if challenge.ID == uuid.Nil {
		t.Fatal("expected captcha id")
	}
	if repo.captchaChallenges[challenge.ID] == nil {
		t.Fatal("expected persisted challenge")
	}
	if repo.lastCaptchaCleanup == nil || !repo.lastCaptchaCleanup.Equal(now) {
		t.Fatalf("expected cleanup attempt at %s, got %v", now, repo.lastCaptchaCleanup)
	}
}

func TestCaptchaCodeGenerationIncludesDigitAndLetter(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := generateCaptchaCode()
		if err != nil {
			t.Fatalf("generate code: %v", err)
		}
		if len(code) != 4 {
			t.Fatalf("expected four characters, got %q", code)
		}
		if !captchaHasDigit(code) || !captchaHasLetter(code) {
			t.Fatalf("expected digit and letter in %q", code)
		}
	}
}

func TestValidateAndConsumeCaptchaIsCaseInsensitiveAndOneTime(t *testing.T) {
	repo := newMockRepo()
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		Now:    func() time.Time { return now },
	}), WithCaptchaEnabled(true))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	captchaID := uuid.New()
	record := &CaptchaChallengeRecord{
		ID:         captchaID,
		TenantID:   platform.DefaultTenantID,
		AnswerHash: svc.hashCaptchaAnswer(captchaID.String(), "A7K2"),
		ExpiresAt:  now.Add(time.Minute),
	}
	repo.captchaChallenges[record.ID] = record

	if err := svc.ValidateAndConsumeCaptcha(context.Background(), record.ID, "a7k2", "admin", "127.0.0.1", "agent"); err != nil {
		t.Fatalf("validate captcha: %v", err)
	}
	if repo.transactionCalls != 1 {
		t.Fatalf("expected validation in one transaction, got %d", repo.transactionCalls)
	}
	if err := svc.ValidateAndConsumeCaptcha(context.Background(), record.ID, "A7K2", "admin", "127.0.0.1", "agent"); !errors.Is(err, ErrCaptchaUsed) {
		t.Fatalf("expected used captcha, got %v", err)
	}
	assertLastLoginFailureReason(t, repo, LoginFailureCaptchaInvalid)
}

func TestValidateAndConsumeCaptchaConsumesExpiredAndWrongAnswers(t *testing.T) {
	repo := newMockRepo()
	repo.failLoginLogInTx = true
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		Now:    func() time.Time { return now },
	}), WithCaptchaEnabled(true))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	expiredID := uuid.New()
	repo.captchaChallenges[expiredID] = &CaptchaChallengeRecord{
		ID:         expiredID,
		TenantID:   platform.DefaultTenantID,
		AnswerHash: svc.hashCaptchaAnswer(expiredID.String(), "A7K2"),
		ExpiresAt:  now.Add(-time.Second),
	}
	if err := svc.ValidateAndConsumeCaptcha(context.Background(), expiredID, "A7K2", "admin", "127.0.0.1", "agent"); !errors.Is(err, ErrCaptchaExpired) {
		t.Fatalf("expected expired captcha, got %v", err)
	}
	if repo.captchaChallenges[expiredID].UsedAt == nil {
		t.Fatal("expected expired captcha to be consumed")
	}
	assertLastLoginFailureReason(t, repo, LoginFailureCaptchaExpired)

	wrongID := uuid.New()
	repo.captchaChallenges[wrongID] = &CaptchaChallengeRecord{
		ID:         wrongID,
		TenantID:   platform.DefaultTenantID,
		AnswerHash: svc.hashCaptchaAnswer(wrongID.String(), "B7K2"),
		ExpiresAt:  now.Add(time.Minute),
	}
	if err := svc.ValidateAndConsumeCaptcha(context.Background(), wrongID, "A7K2", "admin", "127.0.0.1", "agent"); !errors.Is(err, ErrCaptchaInvalid) {
		t.Fatalf("expected invalid captcha, got %v", err)
	}
	if repo.captchaChallenges[wrongID].UsedAt == nil {
		t.Fatal("expected wrong-answer captcha to be consumed")
	}
	assertLastLoginFailureReason(t, repo, LoginFailureCaptchaInvalid)
	if len(repo.captchaConsumeCalls) != 2 || repo.captchaConsumeCalls[0] != expiredID || repo.captchaConsumeCalls[1] != wrongID {
		t.Fatalf("expected expired and wrong captcha consume calls, got %#v", repo.captchaConsumeCalls)
	}
}

func TestValidateAndConsumeCaptchaPropagatesInfrastructureErrors(t *testing.T) {
	repo := newMockRepo()
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		Now:    func() time.Time { return now },
	}), WithCaptchaEnabled(true))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	getErr := errors.New("captcha store unavailable")
	repo.failCaptchaGet = getErr
	if err := svc.ValidateAndConsumeCaptcha(context.Background(), uuid.New(), "A7K2", "admin", "127.0.0.1", "agent"); !errors.Is(err, getErr) {
		t.Fatalf("expected get infrastructure error, got %v", err)
	}
	if len(repo.loginLogs) != 0 {
		t.Fatalf("expected no captcha failure login log for infrastructure error, got %#v", repo.loginLogs)
	}

	repo.failCaptchaGet = nil
	consumeErr := errors.New("captcha consume write failed")
	repo.failCaptchaConsume = consumeErr
	captchaID := uuid.New()
	repo.captchaChallenges[captchaID] = &CaptchaChallengeRecord{
		ID:         captchaID,
		TenantID:   platform.DefaultTenantID,
		AnswerHash: svc.hashCaptchaAnswer(captchaID.String(), "A7K2"),
		ExpiresAt:  now.Add(time.Minute),
	}
	if err := svc.ValidateAndConsumeCaptcha(context.Background(), captchaID, "A7K2", "admin", "127.0.0.1", "agent"); !errors.Is(err, consumeErr) {
		t.Fatalf("expected consume infrastructure error, got %v", err)
	}
}

func TestValidateAndConsumeCaptchaRejectsNilIDBeforeLookup(t *testing.T) {
	repo := newMockRepo()
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		Now:    func() time.Time { return now },
	}), WithCaptchaEnabled(true))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.ValidateAndConsumeCaptcha(context.Background(), uuid.Nil, "A7K2", "admin", "127.0.0.1", "agent"); !errors.Is(err, ErrCaptchaInvalid) {
		t.Fatalf("expected invalid captcha, got %v", err)
	}
	if repo.transactionCalls != 0 {
		t.Fatalf("expected nil captcha id to be rejected before transaction, got %d", repo.transactionCalls)
	}
	assertLastLoginFailureReason(t, repo, LoginFailureCaptchaInvalid)
}

func TestValidateAndConsumeCaptchaLogsUsedFailureAfterTransaction(t *testing.T) {
	repo := newMockRepo()
	repo.failLoginLogInTx = true
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		Now:    func() time.Time { return now },
	}), WithCaptchaEnabled(true))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	captchaID := uuid.New()
	usedAt := now.Add(-time.Second)
	repo.captchaChallenges[captchaID] = &CaptchaChallengeRecord{
		ID:         captchaID,
		TenantID:   platform.DefaultTenantID,
		AnswerHash: svc.hashCaptchaAnswer(captchaID.String(), "A7K2"),
		ExpiresAt:  now.Add(time.Minute),
		UsedAt:     &usedAt,
	}

	if err := svc.ValidateAndConsumeCaptcha(context.Background(), captchaID, "A7K2", "admin", "127.0.0.1", "agent"); !errors.Is(err, ErrCaptchaUsed) {
		t.Fatalf("expected used captcha, got %v", err)
	}
	if repo.transactionCalls != 1 {
		t.Fatalf("expected validation in one transaction, got %d", repo.transactionCalls)
	}
	assertLastLoginFailureReason(t, repo, LoginFailureCaptchaInvalid)
}

func TestValidateAndConsumeCaptchaRejectsInvalidAnswerLengthBeforeLookup(t *testing.T) {
	repo := newMockRepo()
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		Now:    func() time.Time { return now },
	}), WithCaptchaEnabled(true))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.ValidateAndConsumeCaptcha(context.Background(), uuid.New(), "A7K25", "admin", "127.0.0.1", "agent"); !errors.Is(err, ErrCaptchaInvalid) {
		t.Fatalf("expected invalid captcha, got %v", err)
	}
	if repo.transactionCalls != 0 {
		t.Fatalf("expected invalid answer length to be rejected before transaction, got %d", repo.transactionCalls)
	}
	if len(repo.captchaConsumeCalls) != 0 {
		t.Fatalf("expected no consume for invalid answer length, got %#v", repo.captchaConsumeCalls)
	}
}

func assertLastLoginFailureReason(t *testing.T, repo *mockRepo, reason string) {
	t.Helper()
	if len(repo.loginLogs) == 0 {
		t.Fatalf("expected login failure log with reason %q", reason)
	}
	log := repo.loginLogs[len(repo.loginLogs)-1]
	if log.EventType != LoginEventFailed || log.Result != LoginResultFailed || log.FailureReason != reason {
		t.Fatalf("expected login failure reason %q, got %#v", reason, log)
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

func TestGetUserBySessionTokenThrottlesLastSeenWrites(t *testing.T) {
	repo := newMockRepo()
	svc, _ := NewService(repo)
	if _, err := svc.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, _, token, err := svc.Login(context.Background(), "admin", "admin", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	tokenHash := HashToken(token)
	session := repo.sessions[tokenHash]
	session.LastSeenAt = time.Now().UTC().Add(-30 * time.Second)

	if _, _, err := svc.GetUserBySessionToken(context.Background(), token); err != nil {
		t.Fatalf("get current user: %v", err)
	}
	if repo.updateSessionLastSeenCalls != 0 {
		t.Fatalf("expected throttled last_seen write, got %d calls", repo.updateSessionLastSeenCalls)
	}

	session.LastSeenAt = time.Now().UTC().Add(-2 * time.Minute)
	if _, _, err := svc.GetUserBySessionToken(context.Background(), token); err != nil {
		t.Fatalf("get current user after stale last_seen: %v", err)
	}
	if repo.updateSessionLastSeenCalls != 1 {
		t.Fatalf("expected one last_seen write after throttle window, got %d", repo.updateSessionLastSeenCalls)
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

// TestUpdateManagedUserContact 钉住联系方式维护语义(飞书反查撞库键):
// nil=不改/空串=清除/手机号规整(去分隔)/非法值 400 族错误。
func TestUpdateManagedUserContact(t *testing.T) {
	repo := newMockRepo()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	user, err := svc.CreateUser(context.Background(), "contactuser", "pw123456")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	actor := Actor{UserID: uuid.New()}

	mobile := "+86 138-0013-8000"
	updated, err := svc.UpdateManagedUserContact(context.Background(), actor, user.ID, UpdateUserContactInput{Mobile: &mobile})
	if err != nil {
		t.Fatalf("update mobile: %v", err)
	}
	if updated.Mobile != "+8613800138000" {
		t.Fatalf("expected normalized mobile, got %q", updated.Mobile)
	}

	email := "second@corp.com"
	if _, err := svc.UpdateManagedUserContact(context.Background(), actor, user.ID, UpdateUserContactInput{Email: &email}); err != nil {
		t.Fatalf("update email: %v", err)
	}
	if repo.usersByID[user.ID].Mobile != "+8613800138000" {
		t.Fatalf("email-only update must not touch mobile")
	}

	empty := ""
	if updated, err := svc.UpdateManagedUserContact(context.Background(), actor, user.ID, UpdateUserContactInput{Mobile: &empty}); err != nil || updated.Mobile != "" {
		t.Fatalf("empty string should clear mobile, got %q err=%v", updated.Mobile, err)
	}

	bad := "not-a-phone"
	if _, err := svc.UpdateManagedUserContact(context.Background(), actor, user.ID, UpdateUserContactInput{Mobile: &bad}); !errors.Is(err, ErrInvalidManagedUserInput) {
		t.Fatalf("expected invalid input for bad mobile, got %v", err)
	}
	badEmail := "no-at-sign"
	if _, err := svc.UpdateManagedUserContact(context.Background(), actor, user.ID, UpdateUserContactInput{Email: &badEmail}); !errors.Is(err, ErrInvalidManagedUserInput) {
		t.Fatalf("expected invalid input for bad email, got %v", err)
	}
}
