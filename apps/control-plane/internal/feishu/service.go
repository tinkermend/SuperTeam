// Package feishu 承载飞书集成的控制平面侧:应用配置(secret 加密存储)、
// 用户身份绑定(open_id 只经可信来源写入)、connector bootstrap 与 on-behalf-of 核验。
// connector 本体是独立进程,本包只提供其依赖的 API 面与数据。
package feishu

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrAppConfigNotFound = errors.New("feishu app config not found")
	ErrIdentityNotFound  = errors.New("feishu identity not found")
	ErrIdentityMismatch  = errors.New("feishu identity mismatch")
	ErrInvalidInput      = errors.New("invalid feishu input")
	ErrSealerRequired    = errors.New("credential sealer is not configured")
)

const (
	BoundViaContactSync = "contact_sync"
	BoundViaOAuth       = "oauth"
)

type AppConfig struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	AppID           string
	AppSecretSealed string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BootstrapAppConfig 是 connector 启动配置(含解密后的 secret,只经 ServiceAuth 通道下发)。
type BootstrapAppConfig struct {
	ConfigID  uuid.UUID
	TenantID  uuid.UUID
	AppID     string
	AppSecret string
}

type Identity struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	AuthUserID        uuid.UUID
	FeishuAppConfigID uuid.UUID
	OpenID            string
	UnionID           *string
	BoundVia          string
	CreatedAt         time.Time
}

type Repository interface {
	UpsertAppConfig(ctx context.Context, tenantID uuid.UUID, appID, secretSealed, status string) (AppConfig, error)
	ListActiveAppConfigs(ctx context.Context, tenantID uuid.UUID) ([]AppConfig, error)
	GetAppConfig(ctx context.Context, tenantID, id uuid.UUID) (AppConfig, error)
	CreateIdentity(ctx context.Context, identity Identity) (Identity, error)
	GetIdentityByOpenID(ctx context.Context, appConfigID uuid.UUID, openID string) (Identity, error)
	GetIdentityByUser(ctx context.Context, appConfigID, authUserID uuid.UUID) (Identity, error)
	ListIdentitiesByUsers(ctx context.Context, tenantID uuid.UUID, userIDs []uuid.UUID) ([]Identity, error)
	ListIdentitiesByTenant(ctx context.Context, tenantID uuid.UUID) ([]Identity, error)
	DeleteIdentityByUser(ctx context.Context, tenantID, appConfigID, authUserID uuid.UUID) error
}

type CredentialSealer interface {
	Seal(plain string) (string, error)
	Open(sealed string) (string, error)
}

type Service struct {
	repo         Repository
	sealer       CredentialSealer
	client       APIClient
	userLister   UserLister
	oauthStates  *oauthStateStore
	publicOrigin string
	webOrigin    string
}

func NewService(repo Repository, sealer CredentialSealer) *Service {
	return &Service{repo: repo, sealer: sealer, oauthStates: newOAuthStateStore()}
}

// UpsertAppConfig 加密写入(或轮换)租户飞书应用配置。secret 明文只在请求中出现。
func (s *Service) UpsertAppConfig(ctx context.Context, tenantID uuid.UUID, appID, appSecret string) (AppConfig, error) {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	if tenantID == uuid.Nil || appID == "" || appSecret == "" {
		return AppConfig{}, ErrInvalidInput
	}
	if s.sealer == nil {
		return AppConfig{}, ErrSealerRequired
	}
	sealed, err := s.sealer.Seal(appSecret)
	if err != nil {
		return AppConfig{}, err
	}
	return s.repo.UpsertAppConfig(ctx, tenantID, appID, sealed, "active")
}

// BootstrapConfigs 返回租户全部 active 应用配置(解密 secret),供 connector 启动。
func (s *Service) BootstrapConfigs(ctx context.Context, tenantID uuid.UUID) ([]BootstrapAppConfig, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if s.sealer == nil {
		return nil, ErrSealerRequired
	}
	configs, err := s.repo.ListActiveAppConfigs(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]BootstrapAppConfig, 0, len(configs))
	for _, cfg := range configs {
		secret, err := s.sealer.Open(cfg.AppSecretSealed)
		if err != nil {
			return nil, err
		}
		out = append(out, BootstrapAppConfig{
			ConfigID:  cfg.ID,
			TenantID:  cfg.TenantID,
			AppID:     cfg.AppID,
			AppSecret: secret,
		})
	}
	return out, nil
}

// VerifyOnBehalfOf 核验 on-behalf-of 声明:acting user 在该租户任一 active 应用
// 下的绑定 open_id 必须与声明一致。绑定表是唯一事实源,声明不可信。
func (s *Service) VerifyOnBehalfOf(ctx context.Context, tenantID, actingUserID uuid.UUID, openID string) error {
	openID = strings.TrimSpace(openID)
	if tenantID == uuid.Nil || actingUserID == uuid.Nil || openID == "" {
		return ErrIdentityMismatch
	}
	identities, err := s.repo.ListIdentitiesByUsers(ctx, tenantID, []uuid.UUID{actingUserID})
	if err != nil {
		return err
	}
	for _, identity := range identities {
		if identity.OpenID == openID {
			return nil
		}
	}
	return ErrIdentityMismatch
}

// BindIdentity 写入绑定(换绑=删旧建新,由调用方决定是否先删)。
func (s *Service) BindIdentity(ctx context.Context, identity Identity) (Identity, error) {
	identity.OpenID = strings.TrimSpace(identity.OpenID)
	identity.BoundVia = strings.TrimSpace(identity.BoundVia)
	if identity.TenantID == uuid.Nil || identity.AuthUserID == uuid.Nil ||
		identity.FeishuAppConfigID == uuid.Nil || identity.OpenID == "" {
		return Identity{}, ErrInvalidInput
	}
	if identity.BoundVia != BoundViaContactSync && identity.BoundVia != BoundViaOAuth {
		return Identity{}, ErrInvalidInput
	}
	return s.repo.CreateIdentity(ctx, identity)
}

// RebindIdentity 删旧建新(用户换飞书账号)。
func (s *Service) RebindIdentity(ctx context.Context, identity Identity) (Identity, error) {
	if err := s.repo.DeleteIdentityByUser(ctx, identity.TenantID, identity.FeishuAppConfigID, identity.AuthUserID); err != nil {
		return Identity{}, err
	}
	return s.BindIdentity(ctx, identity)
}

// ResolveIdentityByOpenID 按 open_id 反查绑定用户(connector 入站用;未绑定返回 ErrIdentityNotFound)。
func (s *Service) ResolveIdentityByOpenID(ctx context.Context, appConfigID uuid.UUID, openID string) (Identity, error) {
	openID = strings.TrimSpace(openID)
	if appConfigID == uuid.Nil || openID == "" {
		return Identity{}, ErrInvalidInput
	}
	return s.repo.GetIdentityByOpenID(ctx, appConfigID, openID)
}

// ListIdentitiesByUsers 供 outbox 收件人展开使用。
func (s *Service) ListIdentitiesByUsers(ctx context.Context, tenantID uuid.UUID, userIDs []uuid.UUID) ([]Identity, error) {
	if tenantID == uuid.Nil || len(userIDs) == 0 {
		return nil, nil
	}
	return s.repo.ListIdentitiesByUsers(ctx, tenantID, userIDs)
}

// ListIdentitiesByTenant 全量绑定列表(Console 用户管理展示绑定状态)。
func (s *Service) ListIdentitiesByTenant(ctx context.Context, tenantID uuid.UUID) ([]Identity, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.ListIdentitiesByTenant(ctx, tenantID)
}
