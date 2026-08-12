package auth

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/oplog"
	"github.com/superteam/control-plane/internal/platform"
)

type CreateUserRecordInput struct {
	Username      string
	DisplayName   string
	Email         string
	Mobile        string
	PasswordHash  string
	Avatar        UserAvatarConfig
	AvatarAssetID string
}

type Repository interface {
	WithTransaction(ctx context.Context, fn func(Repository) error) error
	CreateUser(ctx context.Context, input CreateUserRecordInput) (*User, error)
	ListUsers(ctx context.Context, filter ListUsersFilter) ([]*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateUserStatus(ctx context.Context, userID uuid.UUID, status string) (*User, error)
	UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (*User, error)
	UpdateUserProfile(ctx context.Context, userID uuid.UUID, input UpdateUserProfileInput) (*User, error)
	UpdateUserContact(ctx context.Context, userID uuid.UUID, input UpdateUserContactInput) (*User, error)
	SetUserAvatarSVG(ctx context.Context, userID uuid.UUID, svg string) error
	CreateRuntimeToken(ctx context.Context, nodeID, tokenHash string, expiresAt time.Time) error
	GetRuntimeTokenByNodeID(ctx context.Context, nodeID string) (*RuntimeToken, error)
	CreateSession(ctx context.Context, session *Session, tokenHash string) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	UpdateSessionLastSeen(ctx context.Context, tokenHash string, lastSeenAt time.Time) error
	CreateLoginLog(ctx context.Context, params CreateLoginLogParams) error
	ListLoginLogs(ctx context.Context, filter ListLoginLogsFilter) ([]LoginLog, error)
	CreateOperationLog(ctx context.Context, params CreateOperationLogParams) error
	ListOperationLogs(ctx context.Context, filter ListOperationLogsFilter) ([]OperationLog, error)
	CreateCaptchaChallenge(ctx context.Context, params CreateCaptchaChallengeParams) (*CaptchaChallengeRecord, error)
	GetCaptchaChallengeForUpdate(ctx context.Context, id uuid.UUID) (*CaptchaChallengeRecord, error)
	ConsumeCaptchaChallenge(ctx context.Context, id uuid.UUID, usedAt time.Time) error
	DeleteExpiredCaptchaChallenges(ctx context.Context, before time.Time) error
	ReplaceUserProjectTeamScopes(ctx context.Context, tenantID, userID, grantedByUserID uuid.UUID, teamIDs []uuid.UUID) ([]UserProjectTeamScopeSummary, error)
	ListUserProjectTeamScopes(ctx context.Context, tenantID, userID uuid.UUID) ([]UserProjectTeamScopeSummary, error)
	CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error)
	EnsureActiveUser(ctx context.Context, userID uuid.UUID) error
	ValidateActiveTenantTeamIDs(ctx context.Context, tenantID uuid.UUID, teamIDs []uuid.UUID) error
	GetActiveTenantMembership(ctx context.Context, tenantID, userID uuid.UUID) (*TenantLevelMembership, error)
	UpsertTenantMembership(ctx context.Context, tenantID, userID uuid.UUID, role string) (*TenantLevelMembership, error)
	DisableTenantMembership(ctx context.Context, tenantID, userID uuid.UUID) (*TenantLevelMembership, error)
	CountActiveTenantOwners(ctx context.Context, tenantID uuid.UUID) (int32, error)
}

type Service struct {
	repo                   Repository
	projectTeamScopeSyncer ProjectTeamScopeSyncer
	membershipSyncer       MembershipSyncer
	captchaEnabled         bool
	captchaSecret          []byte
	captchaTTL             time.Duration
	now                    func() time.Time
	sessionTTLResolver     func(ctx context.Context) time.Duration
	userDeactivatedHook    UserDeactivatedHook
}

// UserDeactivatedHook notifies dependents when a managed user is disabled.
// Optional; nil skips the hook.
type UserDeactivatedHook interface {
	OnUserDeactivated(ctx context.Context, userID uuid.UUID) error
}

func (s *Service) SetUserDeactivatedHook(hook UserDeactivatedHook) {
	s.userDeactivatedHook = hook
}

// defaultAuthSessionTTL 是登录会话 TTL 兜底默认值;生效值经 SetSessionTTLResolver
// 读系统配置中心(auth.session_ttl_seconds)。auth 包被 api/middleware 反向依赖,
// 不能直接 import systemconfig(会成环),由 app 装配层以闭包注入(登录先于租户
// 上下文,闭包内固定使用平台默认租户)。
const defaultAuthSessionTTL = 12 * time.Hour

// SetSessionTTLResolver 注入登录会话 TTL 解析器;未注入(测试)时使用兜底默认值。
func (s *Service) SetSessionTTLResolver(resolver func(ctx context.Context) time.Duration) {
	s.sessionTTLResolver = resolver
}

func (s *Service) sessionTTL(ctx context.Context) time.Duration {
	if s.sessionTTLResolver == nil {
		return defaultAuthSessionTTL
	}
	return s.sessionTTLResolver(ctx)
}

type ServiceOption func(*Service) error

type CaptchaOptions struct {
	Secret string
	TTL    time.Duration
	Now    func() time.Time
}

func WithCaptchaOptions(options CaptchaOptions) ServiceOption {
	return func(s *Service) error {
		if options.TTL <= 0 {
			options.TTL = 5 * time.Minute
		}
		if options.Now == nil {
			options.Now = func() time.Time { return time.Now().UTC() }
		}
		secret := strings.TrimSpace(options.Secret)
		if secret == "" {
			token, err := GenerateToken()
			if err != nil {
				return err
			}
			secret = token
			log.Println("auth captcha secret is not configured; using process-local secret")
		}
		s.captchaSecret = []byte(secret)
		s.captchaTTL = options.TTL
		s.now = options.Now
		return nil
	}
}

func WithCaptchaEnabled(enabled bool) ServiceOption {
	return func(s *Service) error {
		s.captchaEnabled = enabled
		return nil
	}
}

type ProjectTeamScopeSyncer interface {
	SyncProjectTeamScope(ctx context.Context, tenantID, userID, teamID uuid.UUID, status string) error
}

type MembershipSyncer interface {
	SyncMembership(ctx context.Context, membership authz.Membership) error
}

type CurrentUserContext struct {
	User     *User
	TenantID uuid.UUID
	TeamID   *uuid.UUID
}

func NewService(repo Repository, options ...ServiceOption) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	svc := &Service{
		repo:           repo,
		captchaEnabled: false,
		captchaTTL:     5 * time.Minute,
		now:            func() time.Time { return time.Now().UTC() },
	}
	if err := WithCaptchaOptions(CaptchaOptions{})(svc); err != nil {
		return nil, err
	}
	for _, option := range options {
		if err := option(svc); err != nil {
			return nil, err
		}
	}
	return svc, nil
}

func (s *Service) IsCaptchaEnabled() bool {
	return s != nil && s.captchaEnabled
}

func (s *Service) SetProjectTeamScopeSyncer(syncer ProjectTeamScopeSyncer) {
	if s != nil {
		s.projectTeamScopeSyncer = syncer
	}
}

func (s *Service) SetMembershipSyncer(syncer MembershipSyncer) {
	if s != nil {
		s.membershipSyncer = syncer
	}
}

func (s *Service) CreateUser(ctx context.Context, username, password string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return s.repo.CreateUser(ctx, CreateUserRecordInput{
		Username:     strings.TrimSpace(username),
		PasswordHash: string(hash),
	})
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
	input, err := normalizeManagedUserInput(input)
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, uuid.Nil, OperationActionUserCreate, OperationResultFailed)
		return nil, err
	}
	if err := s.authorizeTenantRoleGrant(ctx, actor, input.TenantID, input.TenantRole); err != nil {
		_ = s.recordUserOperation(ctx, actor, uuid.Nil, OperationActionUserCreate, OperationResultFailed)
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	var user *User
	var membership *TenantLevelMembership
	err = s.repo.WithTransaction(ctx, func(repo Repository) error {
		if err := repo.ValidateActiveTenantTeamIDs(ctx, input.TenantID, input.SelectableTeamIDs); err != nil {
			return err
		}
		created, err := repo.CreateUser(ctx, CreateUserRecordInput{
			Username:     input.Username,
			DisplayName:  input.DisplayName,
			Email:        input.Email,
			Mobile:       input.Mobile,
			PasswordHash: string(hash),
			Avatar:       input.Avatar,
		})
		if err != nil {
			return err
		}
		createdMembership, err := repo.UpsertTenantMembership(ctx, input.TenantID, created.ID, input.TenantRole)
		if err != nil {
			return err
		}
		if _, err := repo.ReplaceUserProjectTeamScopes(ctx, input.TenantID, created.ID, actor.UserID, input.SelectableTeamIDs); err != nil {
			return err
		}
		user = created
		membership = createdMembership
		return nil
	})
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, uuid.Nil, OperationActionUserCreate, OperationResultFailed)
		return nil, err
	}
	_ = s.recordUserOperation(ctx, actor, user.ID, OperationActionUserCreate, OperationResultSucceeded)
	s.syncTenantMembership(ctx, nil, membership)
	s.syncProjectTeamScopeChanges(ctx, input.TenantID, user.ID, nil, input.SelectableTeamIDs)
	return user, nil
}

func normalizeManagedUserInput(input CreateManagedUserInput) (CreateManagedUserInput, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.TrimSpace(input.Email)
	rawMobile := strings.TrimSpace(input.Mobile)
	input.Mobile = normalizeMobile(rawMobile)
	input.AvatarAssetID = strings.ToLower(strings.TrimSpace(input.AvatarAssetID))
	input.TenantRole = strings.TrimSpace(strings.ToLower(input.TenantRole))
	if input.TenantID == uuid.Nil {
		input.TenantID = platform.DefaultTenantID
	}
	if input.Username == "" ||
		input.DisplayName == "" ||
		input.Password == "" ||
		input.AvatarAssetID != "" ||
		!isValidTenantRole(input.TenantRole) ||
		!isExplicitSupportedUserAvatar(input.Avatar) {
		return input, ErrInvalidManagedUserInput
	}
	if input.Email != "" && !strings.Contains(input.Email, "@") {
		return input, ErrInvalidManagedUserInput
	}
	if rawMobile != "" && !isValidMobile(input.Mobile) {
		return input, ErrInvalidManagedUserInput
	}
	input.Avatar = normalizeUserAvatarConfig(input.Username, input.Avatar)
	if len(input.SelectableTeamIDs) > 0 {
		teamIDs, err := normalizeProjectTeamScopeIDs(input.SelectableTeamIDs)
		if err != nil {
			return input, err
		}
		input.SelectableTeamIDs = teamIDs
	}
	return input, nil
}

// normalizeMobile 去除空格/短横线等展示分隔;保留可选前导 + 与数字。
func normalizeMobile(mobile string) string {
	mobile = strings.TrimSpace(mobile)
	var b strings.Builder
	for i, r := range mobile {
		if r == '+' && i == 0 {
			b.WriteRune(r)
			continue
		}
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isValidMobile 宽松校验:可选 + 前缀 + 5..20 位数字(具体格式以飞书档案为准,
// 反查命不命中由 batch_get_id 决定,这里只拦明显非号码输入)。
func isValidMobile(mobile string) bool {
	digits := strings.TrimPrefix(mobile, "+")
	if len(digits) < 5 || len(digits) > 20 {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isValidTenantRole(role string) bool {
	switch role {
	case TenantRoleOwner, TenantRoleAdmin, TenantRoleMember, TenantRoleViewer:
		return true
	default:
		return false
	}
}

func isExplicitSupportedUserAvatar(avatar UserAvatarConfig) bool {
	return strings.TrimSpace(avatar.Provider) == "dicebear" &&
		strings.TrimSpace(avatar.Style) == "adventurer" &&
		strings.TrimSpace(avatar.Seed) != ""
}

func normalizeProjectTeamScopeIDs(teamIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(teamIDs) == 0 {
		return nil, ErrInvalidManagedUserInput
	}
	seen := make(map[uuid.UUID]bool, len(teamIDs))
	normalized := make([]uuid.UUID, 0, len(teamIDs))
	for _, teamID := range teamIDs {
		if teamID == uuid.Nil {
			return nil, ErrInvalidManagedUserInput
		}
		if seen[teamID] {
			continue
		}
		seen[teamID] = true
		normalized = append(normalized, teamID)
	}
	if len(normalized) == 0 {
		return nil, ErrInvalidManagedUserInput
	}
	return normalized, nil
}

func normalizeUserAvatarConfig(username string, avatar UserAvatarConfig) UserAvatarConfig {
	avatar.Provider = strings.TrimSpace(avatar.Provider)
	avatar.Style = strings.TrimSpace(avatar.Style)
	avatar.Seed = strings.TrimSpace(avatar.Seed)
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
	if status == UserStatusDisabled && s.userDeactivatedHook != nil {
		if hookErr := s.userDeactivatedHook.OnUserDeactivated(ctx, user.ID); hookErr != nil {
			log.Printf("user deactivated hook failed: user_id=%s err=%v", user.ID, hookErr)
		}
	}
	return user, nil
}

// UpdateManagedUserContact 管理员维护既有用户的联系方式(email/mobile)——它们是
// 飞书通讯录反查(ContactSync)的撞库键;没有这个入口,既有用户只能本人登录
// 自服务补资料才能被反查命中。nil=不改,空串=清除。
func (s *Service) UpdateManagedUserContact(ctx context.Context, actor Actor, userID uuid.UUID, input UpdateUserContactInput) (*User, error) {
	if input.Email != nil {
		email := strings.TrimSpace(*input.Email)
		if email != "" && !strings.Contains(email, "@") {
			return nil, ErrInvalidManagedUserInput
		}
		input.Email = &email
	}
	if input.Mobile != nil {
		raw := strings.TrimSpace(*input.Mobile)
		mobile := normalizeMobile(raw)
		// 原始输入非空但规整后不合法(含规整成空,如纯字母)必须报错,
		// 不得静默降级成"清除"。
		if raw != "" && !isValidMobile(mobile) {
			return nil, ErrInvalidManagedUserInput
		}
		input.Mobile = &mobile
	}
	user, err := s.repo.UpdateUserContact(ctx, userID, input)
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, userID, OperationActionUserUpdateContact, OperationResultFailed)
		return nil, err
	}
	_ = s.recordUserOperation(ctx, actor, user.ID, OperationActionUserUpdateContact, OperationResultSucceeded)
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

func (s *Service) ListUserProjectTeamScopes(ctx context.Context, tenantID, userID uuid.UUID) ([]UserProjectTeamScopeSummary, error) {
	if tenantID == uuid.Nil {
		tenantID = platform.DefaultTenantID
	}
	return s.repo.ListUserProjectTeamScopes(ctx, tenantID, userID)
}

func (s *Service) ReplaceUserProjectTeamScopes(ctx context.Context, actor Actor, tenantID, userID uuid.UUID, teamIDs []uuid.UUID) ([]UserProjectTeamScopeSummary, error) {
	if tenantID == uuid.Nil {
		tenantID = platform.DefaultTenantID
	}
	if userID == uuid.Nil {
		return nil, ErrManagedUserNotFound
	}
	normalizedTeamIDs, err := normalizeProjectTeamScopeIDs(teamIDs)
	if err != nil {
		return nil, ErrInvalidManagedUserInput
	}
	if err := s.repo.EnsureActiveUser(ctx, userID); err != nil {
		return nil, err
	}
	if err := s.repo.ValidateActiveTenantTeamIDs(ctx, tenantID, normalizedTeamIDs); err != nil {
		return nil, err
	}
	previous, previousErr := s.repo.ListUserProjectTeamScopes(ctx, tenantID, userID)
	if previousErr != nil {
		log.Printf("openfga project team scope sync skipped revoked tuples: tenant_id=%s user_id=%s err=%v", tenantID, userID, previousErr)
	}
	scopes, err := s.repo.ReplaceUserProjectTeamScopes(ctx, tenantID, userID, actor.UserID, normalizedTeamIDs)
	if err != nil {
		return nil, err
	}
	s.syncProjectTeamScopeChanges(ctx, tenantID, userID, previous, normalizedTeamIDs)
	return scopes, nil
}

func (s *Service) CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error) {
	if tenantID == uuid.Nil {
		tenantID = platform.DefaultTenantID
	}
	return s.repo.CanUseTeamForProject(ctx, tenantID, userID, teamID)
}

func (s *Service) GetUserTenantMembership(ctx context.Context, tenantID, userID uuid.UUID) (*TenantLevelMembership, error) {
	if tenantID == uuid.Nil {
		tenantID = platform.DefaultTenantID
	}
	membership, err := s.repo.GetActiveTenantMembership(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if membership == nil {
		return nil, ErrTenantMembershipNotFound
	}
	return membership, nil
}

func (s *Service) UpsertUserTenantMembership(ctx context.Context, actor Actor, tenantID, userID uuid.UUID, role string) (*TenantLevelMembership, error) {
	if tenantID == uuid.Nil {
		tenantID = platform.DefaultTenantID
	}
	role = strings.TrimSpace(strings.ToLower(role))
	if !isValidTenantRole(role) || userID == uuid.Nil {
		_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipUpsert, OperationResultFailed)
		return nil, ErrInvalidManagedUserInput
	}
	if err := s.authorizeTenantRoleGrant(ctx, actor, tenantID, role); err != nil {
		_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipUpsert, OperationResultFailed)
		return nil, err
	}
	if err := s.repo.EnsureActiveUser(ctx, userID); err != nil {
		_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipUpsert, OperationResultFailed)
		return nil, err
	}
	previous, _ := s.repo.GetActiveTenantMembership(ctx, tenantID, userID)
	if previous != nil && previous.Role == TenantRoleOwner && role != TenantRoleOwner {
		if err := s.ensureNotLastTenantOwner(ctx, tenantID); err != nil {
			_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipUpsert, OperationResultFailed)
			return nil, err
		}
	}
	membership, err := s.repo.UpsertTenantMembership(ctx, tenantID, userID, role)
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipUpsert, OperationResultFailed)
		return nil, err
	}
	_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipUpsert, OperationResultSucceeded)
	s.syncTenantMembership(ctx, previous, membership)
	return membership, nil
}

func (s *Service) DeleteUserTenantMembership(ctx context.Context, actor Actor, tenantID, userID uuid.UUID) error {
	if tenantID == uuid.Nil {
		tenantID = platform.DefaultTenantID
	}
	if userID == uuid.Nil {
		return ErrInvalidManagedUserInput
	}
	previous, err := s.repo.GetActiveTenantMembership(ctx, tenantID, userID)
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipDelete, OperationResultFailed)
		return err
	}
	if previous == nil {
		_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipDelete, OperationResultFailed)
		return ErrTenantMembershipNotFound
	}
	if previous.Role == TenantRoleOwner {
		if err := s.ensureNotLastTenantOwner(ctx, tenantID); err != nil {
			_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipDelete, OperationResultFailed)
			return err
		}
	}
	disabled, err := s.repo.DisableTenantMembership(ctx, tenantID, userID)
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipDelete, OperationResultFailed)
		return err
	}
	_ = s.recordUserOperation(ctx, actor, userID, OperationActionTenantMembershipDelete, OperationResultSucceeded)
	s.syncTenantMembership(ctx, previous, disabled)
	return nil
}

func (s *Service) authorizeTenantRoleGrant(ctx context.Context, actor Actor, tenantID uuid.UUID, role string) error {
	if role != TenantRoleOwner {
		return nil
	}
	actorMembership, err := s.repo.GetActiveTenantMembership(ctx, tenantID, actor.UserID)
	if err != nil {
		return err
	}
	if actorMembership == nil || actorMembership.Role != TenantRoleOwner {
		return ErrOwnerGrantForbidden
	}
	return nil
}

func (s *Service) ensureNotLastTenantOwner(ctx context.Context, tenantID uuid.UUID) error {
	count, err := s.repo.CountActiveTenantOwners(ctx, tenantID)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastTenantOwner
	}
	return nil
}

func (s *Service) syncTenantMembership(ctx context.Context, previous, current *TenantLevelMembership) {
	if s == nil || s.membershipSyncer == nil {
		return
	}
	if previous != nil && (current == nil || previous.Role != current.Role || previous.Status != current.Status) {
		if err := s.membershipSyncer.SyncMembership(ctx, authz.Membership{
			TenantID:      previous.TenantID,
			PrincipalType: authz.ActorUser,
			PrincipalID:   previous.UserID,
			Role:          previous.Role,
			Status:        "disabled",
		}); err != nil {
			log.Printf("openfga tenant membership sync failed: tenant_id=%s user_id=%s role=%s status=disabled err=%v", previous.TenantID, previous.UserID, previous.Role, err)
		}
	}
	if current != nil && current.Status == "active" {
		if err := s.membershipSyncer.SyncMembership(ctx, authz.Membership{
			TenantID:      current.TenantID,
			PrincipalType: authz.ActorUser,
			PrincipalID:   current.UserID,
			Role:          current.Role,
			Status:        current.Status,
		}); err != nil {
			log.Printf("openfga tenant membership sync failed: tenant_id=%s user_id=%s role=%s status=%s err=%v", current.TenantID, current.UserID, current.Role, current.Status, err)
		}
	}
}

func (s *Service) syncProjectTeamScopeChanges(ctx context.Context, tenantID, userID uuid.UUID, previous []UserProjectTeamScopeSummary, activeTeamIDs []uuid.UUID) {
	if s == nil || s.projectTeamScopeSyncer == nil {
		return
	}
	active := make(map[uuid.UUID]bool, len(activeTeamIDs))
	for _, teamID := range activeTeamIDs {
		active[teamID] = true
		if err := s.projectTeamScopeSyncer.SyncProjectTeamScope(ctx, tenantID, userID, teamID, "active"); err != nil {
			log.Printf("openfga project team scope sync failed: tenant_id=%s user_id=%s team_id=%s status=active err=%v", tenantID, userID, teamID, err)
		}
	}
	for _, scope := range previous {
		if active[scope.TeamID] {
			continue
		}
		if err := s.projectTeamScopeSyncer.SyncProjectTeamScope(ctx, tenantID, userID, scope.TeamID, "revoked"); err != nil {
			log.Printf("openfga project team scope sync failed: tenant_id=%s user_id=%s team_id=%s status=revoked err=%v", tenantID, userID, scope.TeamID, err)
		}
	}
}

// SetCurrentUserAvatarSVG 自愈写回当前用户预渲染头像 data-URI(P1-D 2b)。校验非空与
// 长度上限;仅写调用者自己(userID 由会话解析,不接受任意用户)。
func (s *Service) SetCurrentUserAvatarSVG(ctx context.Context, userID uuid.UUID, svg string) error {
	svg = strings.TrimSpace(svg)
	if svg == "" || len(svg) > 65536 {
		return ErrInvalidManagedUserInput
	}
	return s.repo.SetUserAvatarSVG(ctx, userID, svg)
}

func (s *Service) UpdateCurrentUserProfile(ctx context.Context, actor Actor, input UpdateUserProfileInput) (*User, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.TrimSpace(input.Email)
	input.Avatar = normalizeUserAvatarConfig(actor.Username, input.Avatar)
	user, err := s.repo.UpdateUserProfile(ctx, actor.UserID, input)
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, actor.UserID, OperationActionUserUpdateOwnProfile, OperationResultFailed)
		return nil, err
	}
	_ = s.recordUserOperation(ctx, actor, user.ID, OperationActionUserUpdateOwnProfile, OperationResultSucceeded)
	return user, nil
}

func (s *Service) ChangeCurrentUserPassword(ctx context.Context, actor Actor, currentPassword, newPassword string) (*User, error) {
	user, err := s.repo.GetUserByID(ctx, actor.UserID)
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, actor.UserID, OperationActionUserChangeOwnPassword, OperationResultFailed)
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		_ = s.recordUserOperation(ctx, actor, actor.UserID, OperationActionUserChangeOwnPassword, OperationResultFailed)
		return nil, ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdateUserPassword(ctx, actor.UserID, string(hash))
	if err != nil {
		_ = s.recordUserOperation(ctx, actor, actor.UserID, OperationActionUserChangeOwnPassword, OperationResultFailed)
		return nil, err
	}
	_ = s.recordUserOperation(ctx, actor, updated.ID, OperationActionUserChangeOwnPassword, OperationResultSucceeded)
	return updated, nil
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
		ClientIP:     oplog.MetaFromContext(ctx).ClientIP,
		UserAgent:    oplog.MetaFromContext(ctx).UserAgent,
		RequestID:    oplog.MetaFromContext(ctx).RequestID,
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
		ExpiresAt:  now.Add(s.sessionTTL(ctx)),
		LastSeenAt: now,
		ClientIP:   clientIP,
		UserAgent:  userAgent,
	}
	if err := s.repo.CreateSession(ctx, session, HashToken(token)); err != nil {
		return nil, "", err
	}
	return session, token, nil
}

const (
	// sessionLastSeenMinInterval skips last_seen writes on hot console paths so
	// remote Postgres RTT is not paid on every authenticated request.
	sessionLastSeenMinInterval = 60 * time.Second
)

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
	now := time.Now().UTC()
	if now.Sub(session.LastSeenAt) >= sessionLastSeenMinInterval {
		_ = s.repo.UpdateSessionLastSeen(ctx, tokenHash, now)
	}
	return session, user, nil
}

func (s *Service) GetCurrentUserContext(ctx context.Context, token string) (*CurrentUserContext, error) {
	_, user, err := s.GetUserBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	tenantID := platform.DefaultTenantID
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

func (s *Service) ListOperationLogs(ctx context.Context, filter ListOperationLogsFilter) ([]OperationLog, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repo.ListOperationLogs(ctx, filter)
}

func (s *Service) ListCurrentUserLoginLogs(ctx context.Context, actor Actor, filter ListLoginLogsFilter) ([]LoginLog, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.UserID = &actor.UserID
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
