package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

type Repository interface {
	CreateUser(ctx context.Context, username, passwordHash string, avatar UserAvatarConfig) (*User, error)
	ListUsers(ctx context.Context, filter ListUsersFilter) ([]*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateUserStatus(ctx context.Context, userID uuid.UUID, status string) (*User, error)
	UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (*User, error)
	CreateRuntimeToken(ctx context.Context, nodeID, tokenHash string, expiresAt time.Time) error
	GetRuntimeTokenByNodeID(ctx context.Context, nodeID string) (*RuntimeToken, error)
	CreateSession(ctx context.Context, session *Session, tokenHash string) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	UpdateSessionLastSeen(ctx context.Context, tokenHash string, lastSeenAt time.Time) error
	CreateLoginLog(ctx context.Context, params CreateLoginLogParams) error
	ListLoginLogs(ctx context.Context, filter ListLoginLogsFilter) ([]LoginLog, error)
	CreateOperationLog(ctx context.Context, params CreateOperationLogParams) error
}

type Service struct {
	repo Repository
}

type CurrentUserContext struct {
	User     *User
	TenantID uuid.UUID
	TeamID   *uuid.UUID
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) CreateUser(ctx context.Context, username, password string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return s.repo.CreateUser(ctx, username, string(hash), defaultUserAvatarConfig(username))
}

func (s *Service) ListUsers(ctx context.Context, filter ListUsersFilter) ([]*User, error) {
	filter.Q = strings.TrimSpace(filter.Q)
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repo.ListUsers(ctx, filter)
}

func (s *Service) CreateManagedUser(ctx context.Context, actor Actor, input CreateManagedUserInput) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user, err := s.repo.CreateUser(ctx, input.Username, string(hash), normalizeUserAvatarConfig(input.Username, input.Avatar))
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, uuid.Nil, OperationActionUserCreate, OperationResultFailed)
		return nil, err
	}
	_ = s.recordUserOperation(ctx, actor, user.ID, OperationActionUserCreate, OperationResultSucceeded)
	return user, nil
}

func normalizeUserAvatarConfig(username string, avatar UserAvatarConfig) UserAvatarConfig {
	if avatar.Provider == "" {
		avatar.Provider = "dicebear"
	}
	if avatar.Style == "" {
		avatar.Style = "adventurer"
	}
	if avatar.Seed == "" {
		avatar.Seed = "user:" + strings.TrimSpace(username)
	}
	if avatar.Options == nil {
		avatar.Options = map[string]any{}
	}
	return avatar
}

func defaultUserAvatarConfig(username string) UserAvatarConfig {
	return normalizeUserAvatarConfig(username, UserAvatarConfig{})
}

func (s *Service) UpdateManagedUserStatus(ctx context.Context, actor Actor, userID uuid.UUID, status string) (*User, error) {
	action := OperationActionUserEnable
	if status == UserStatusDisabled {
		action = OperationActionUserDisable
	}
	user, err := s.repo.UpdateUserStatus(ctx, userID, status)
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, userID, action, OperationResultFailed)
		return nil, err
	}
	_ = s.recordUserOperation(ctx, actor, user.ID, action, OperationResultSucceeded)
	return user, nil
}

func (s *Service) ResetManagedUserPassword(ctx context.Context, actor Actor, userID uuid.UUID, password string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user, err := s.repo.UpdateUserPassword(ctx, userID, string(hash))
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, userID, OperationActionUserResetPassword, OperationResultFailed)
		return nil, err
	}
	_ = s.recordUserOperation(ctx, actor, user.ID, OperationActionUserResetPassword, OperationResultSucceeded)
	return user, nil
}

func (s *Service) recordUserOperation(ctx context.Context, actor Actor, userID uuid.UUID, action, result string) error {
	resourceID := ""
	if userID != uuid.Nil {
		resourceID = userID.String()
	}
	return s.repo.CreateOperationLog(ctx, CreateOperationLogParams{
		UserID:       &actor.UserID,
		Username:     actor.Username,
		Module:       OperationModuleAuth,
		ResourceType: OperationResourceUser,
		ResourceID:   resourceID,
		Action:       action,
		Result:       result,
	})
}

func (s *Service) AuthenticateUser(ctx context.Context, username, password string) (*User, error) {
	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	if user.Status != "active" {
		return nil, ErrUserDisabled
	}
	return user, nil
}

func (s *Service) Login(ctx context.Context, username, password, clientIP, userAgent string) (*Session, *User, string, error) {
	user, err := s.AuthenticateUser(ctx, username, password)
	if err != nil {
		_ = s.repo.CreateLoginLog(ctx, CreateLoginLogParams{
			EventType:     LoginEventFailed,
			Username:      username,
			ClientIP:      clientIP,
			UserAgent:     userAgent,
			Result:        LoginResultFailed,
			FailureReason: loginFailureReason(err),
		})
		return nil, nil, "", err
	}
	session, token, err := s.CreateSession(ctx, user.ID, clientIP, userAgent)
	if err != nil {
		return nil, nil, "", err
	}
	_ = s.repo.CreateLoginLog(ctx, CreateLoginLogParams{
		EventType: LoginEventSucceeded,
		UserID:    &user.ID,
		Username:  user.Username,
		SessionID: &session.ID,
		ClientIP:  clientIP,
		UserAgent: userAgent,
		Result:    LoginResultSucceeded,
	})
	return session, user, token, nil
}

func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, clientIP, userAgent string) (*Session, string, error) {
	token, err := GenerateToken()
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	session := &Session{
		UserID:     userID,
		ExpiresAt:  now.Add(12 * time.Hour),
		LastSeenAt: now,
		ClientIP:   clientIP,
		UserAgent:  userAgent,
	}
	if err := s.repo.CreateSession(ctx, session, HashToken(token)); err != nil {
		return nil, "", err
	}
	return session, token, nil
}

func (s *Service) GetUserBySessionToken(ctx context.Context, token string) (*Session, *User, error) {
	if token == "" {
		return nil, nil, ErrUnauthorized
	}
	tokenHash := HashToken(token)
	session, err := s.repo.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, nil, ErrSessionNotFound
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		_ = s.repo.DeleteSession(ctx, tokenHash)
		return nil, nil, ErrSessionExpired
	}
	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, nil, ErrUnauthorized
	}
	if user.Status != "active" {
		return nil, nil, ErrUserDisabled
	}
	_ = s.repo.UpdateSessionLastSeen(ctx, tokenHash, time.Now().UTC())
	return session, user, nil
}

func (s *Service) GetCurrentUserContext(ctx context.Context, token string) (*CurrentUserContext, error) {
	_, user, err := s.GetUserBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	tenantID := uuid.MustParse(DefaultTenantID)
	return &CurrentUserContext{
		User:     user,
		TenantID: tenantID,
	}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	tokenHash := HashToken(token)
	session, sessionErr := s.repo.GetSessionByTokenHash(ctx, tokenHash)
	var user *User
	if sessionErr == nil {
		user, _ = s.repo.GetUserByID(ctx, session.UserID)
	}
	if err := s.repo.DeleteSession(ctx, tokenHash); err != nil {
		return err
	}
	if sessionErr == nil {
		username := ""
		if user != nil {
			username = user.Username
		}
		_ = s.repo.CreateLoginLog(ctx, CreateLoginLogParams{
			EventType: LoginEventLogoutSucceeded,
			UserID:    &session.UserID,
			Username:  username,
			SessionID: &session.ID,
			ClientIP:  session.ClientIP,
			UserAgent: session.UserAgent,
			Result:    LoginResultSucceeded,
		})
	}
	return nil
}

func (s *Service) ListLoginLogs(ctx context.Context, filter ListLoginLogsFilter) ([]LoginLog, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repo.ListLoginLogs(ctx, filter)
}

func loginFailureReason(err error) string {
	if errors.Is(err, ErrUserDisabled) {
		return LoginFailureUserDisabled
	}
	return LoginFailureInvalidCredentials
}

func (s *Service) GenerateRuntimeToken(ctx context.Context, nodeID string, expiresAt time.Time) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if err := s.repo.CreateRuntimeToken(ctx, nodeID, string(hash), expiresAt); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) ValidateRuntimeToken(ctx context.Context, nodeID, token string) error {
	rt, err := s.repo.GetRuntimeTokenByNodeID(ctx, nodeID)
	if err != nil {
		return ErrInvalidToken
	}
	if time.Now().After(rt.ExpiresAt) {
		return ErrInvalidToken
	}
	if err := bcrypt.CompareHashAndPassword([]byte(rt.TokenHash), []byte(token)); err != nil {
		return ErrInvalidToken
	}
	return nil
}
