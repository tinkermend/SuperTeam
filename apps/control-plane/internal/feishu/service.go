// Package feishu 承载飞书集成的控制平面侧:应用配置(secret 加密存储)、
// 用户身份绑定(open_id 只经可信来源写入)、connector bootstrap 与 on-behalf-of 核验。
// connector 本体是独立进程,本包只提供其依赖的 API 面与数据。
package feishu

import (
	"context"
	"errors"
	"fmt"
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

const (
	AppConfigStatusActive     = "active"
	AppConfigStatusUnverified = "unverified"
	AppConfigStatusDisabled   = "disabled"
)

// ConnectivityProbe 是保存配置时的单项连通探测结果(中文 hint,供 Console 直接展示)。
type ConnectivityProbe struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	OK      bool   `json:"ok"`
	Hint    string `json:"hint"`
	Code    int    `json:"code,omitempty"`
	RawMsg  string `json:"raw_msg,omitempty"`
}

// ConnectivityReport 是一次完整自检报告。
type ConnectivityReport struct {
	TokenOK bool                `json:"token_ok"`
	OK      bool                `json:"ok"`
	Probes  []ConnectivityProbe `json:"probes"`
	Summary string              `json:"summary"`
}

type Repository interface {
	UpsertAppConfig(ctx context.Context, tenantID uuid.UUID, appID, secretSealed, status string) (AppConfig, error)
	ListActiveAppConfigs(ctx context.Context, tenantID uuid.UUID) ([]AppConfig, error)
	ListAppConfigs(ctx context.Context, tenantID uuid.UUID) ([]AppConfig, error)
	UpdateAppConfigStatus(ctx context.Context, tenantID, id uuid.UUID, status string) (AppConfig, error)
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
	repo           Repository
	sealer         CredentialSealer
	client         APIClient
	userLister     UserLister
	oauthStates    oauthStateStore
	publicOrigin   string
	webOrigin      string
	heartbeatStore heartbeatStore // Redis 探活权威；未注入则健康摘要为 missing
}

func NewService(repo Repository, sealer CredentialSealer) *Service {
	// 默认内存 store 仅供单测;生产在 app 装配层注入 Redis。
	return &Service{repo: repo, sealer: sealer, oauthStates: newMemoryOAuthStateStore()}
}

// SetOAuthStateStore 注入 OAuth state 存储(生产 = Redis one-shot)。
func (s *Service) SetOAuthStateStore(store oauthStateStore) {
	if store == nil {
		return
	}
	s.oauthStates = store
}

// UpsertAppConfig 加密写入(或轮换)租户飞书应用配置,并做连通性自检。
// secret 明文只在请求中出现;自检失败仍保存,状态标 unverified(管理员可先填后开权限)。
func (s *Service) UpsertAppConfig(ctx context.Context, tenantID uuid.UUID, appID, appSecret string) (AppConfig, ConnectivityReport, error) {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	if tenantID == uuid.Nil || appID == "" || appSecret == "" {
		return AppConfig{}, ConnectivityReport{}, ErrInvalidInput
	}
	if s.sealer == nil {
		return AppConfig{}, ConnectivityReport{}, ErrSealerRequired
	}
	report := s.VerifyAppCredentials(ctx, appID, appSecret)
	status := AppConfigStatusUnverified
	if report.OK {
		status = AppConfigStatusActive
	}
	sealed, err := s.sealer.Seal(appSecret)
	if err != nil {
		return AppConfig{}, report, err
	}
	cfg, err := s.repo.UpsertAppConfig(ctx, tenantID, appID, sealed, status)
	if err != nil {
		return AppConfig{}, report, err
	}
	return cfg, report, nil
}

// ListAppConfigs 管理面列表(含 unverified/disabled)。
func (s *Service) ListAppConfigs(ctx context.Context, tenantID uuid.UUID) ([]AppConfig, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repo.ListAppConfigs(ctx, tenantID)
}

// SetAppConfigStatus 启停通道。disabled 的配置不再下发给 connector bootstrap。
func (s *Service) SetAppConfigStatus(ctx context.Context, tenantID, id uuid.UUID, status string) (AppConfig, error) {
	status = strings.TrimSpace(status)
	switch status {
	case AppConfigStatusActive, AppConfigStatusUnverified, AppConfigStatusDisabled:
	default:
		return AppConfig{}, ErrInvalidInput
	}
	if tenantID == uuid.Nil || id == uuid.Nil {
		return AppConfig{}, ErrInvalidInput
	}
	// 重新启用时若未重跑自检,最多回到 unverified,避免把未验证凭据直接标 active。
	if status == AppConfigStatusActive {
		status = AppConfigStatusUnverified
	}
	return s.repo.UpdateAppConfigStatus(ctx, tenantID, id, status)
}

// VerifyAppCredentials 用明文凭据直打飞书,返回结构化中文报告(不落库)。
// 探测项对齐平台真实调用面:取 token / 通讯录反查 / bot 信息(发消息前置)。
func (s *Service) VerifyAppCredentials(ctx context.Context, appID, appSecret string) ConnectivityReport {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	report := ConnectivityReport{Probes: make([]ConnectivityProbe, 0, 3)}
	if s.client == nil {
		report.Probes = append(report.Probes, ConnectivityProbe{
			Key:   "client",
			Label: "飞书客户端",
			OK:    false,
			Hint:  "控制平面未配置飞书 HTTP 客户端,无法做连通自检",
		})
		report.Summary = "无法自检:飞书客户端未配置"
		return report
	}

	token, err := s.client.TenantAccessToken(ctx, appID, appSecret)
	tokenProbe := ConnectivityProbe{Key: "tenant_access_token", Label: "应用凭证(App ID / Secret)"}
	if err != nil {
		tokenProbe.OK = false
		tokenProbe.Hint = "无法获取 tenant_access_token:请核对 App ID 与 App Secret 是否与开放平台一致,或应用是否已启用"
		tokenProbe.RawMsg = err.Error()
		report.Probes = append(report.Probes, tokenProbe)
		report.Summary = "应用凭证无效,配置已保存为「未验证」"
		return report
	}
	tokenProbe.OK = true
	tokenProbe.Hint = "已成功获取 tenant_access_token"
	report.TokenOK = true
	report.Probes = append(report.Probes, tokenProbe)

	contactOK, code, msg, err := s.client.ProbeContactDirectory(ctx, token)
	contactProbe := ConnectivityProbe{Key: "contact_directory", Label: "通讯录反查权限", Code: code, RawMsg: msg}
	if err != nil {
		contactProbe.OK = false
		contactProbe.Hint = "通讯录探测请求失败:网络或飞书服务异常"
		contactProbe.RawMsg = err.Error()
	} else if contactOK {
		contactProbe.OK = true
		contactProbe.Hint = "batch_get_id 可用。若同步仍无人命中,请到开放平台把「通讯录授权范围」设为全部成员并发布版本"
	} else {
		contactProbe.OK = false
		contactProbe.Hint = fmt.Sprintf("通讯录反查被拒(code=%d)。请在开放平台开通 contact 相关权限,并将「通讯录授权范围」设为全部成员后发布", code)
	}
	report.Probes = append(report.Probes, contactProbe)

	botOK, botCode, botMsg, err := s.client.ProbeBotInfo(ctx, token)
	botProbe := ConnectivityProbe{Key: "bot_info", Label: "机器人/发消息能力", Code: botCode, RawMsg: botMsg}
	if err != nil {
		botProbe.OK = false
		botProbe.Hint = "机器人信息探测失败:网络或飞书服务异常"
		botProbe.RawMsg = err.Error()
	} else if botOK {
		botProbe.OK = true
		botProbe.Hint = "机器人接口可用。若发卡报 230013,请到开放平台把「应用可用范围」设为全部成员并发布版本"
	} else {
		botProbe.OK = false
		botProbe.Hint = fmt.Sprintf("机器人接口被拒(code=%d)。请确认应用已启用 Bot、开通 im:message 等发消息权限,并设置「应用可用范围」", botCode)
	}
	report.Probes = append(report.Probes, botProbe)

	report.OK = tokenProbe.OK && contactProbe.OK && botProbe.OK
	if report.OK {
		report.Summary = "连通自检通过,配置已标记为可用。请确认通讯录授权范围与应用可用范围覆盖目标成员"
	} else {
		failed := 0
		for _, p := range report.Probes {
			if !p.OK {
				failed++
			}
		}
		report.Summary = fmt.Sprintf("连通自检未全部通过(%d 项失败),配置已保存为「未验证」——可先保留凭据,到开放平台补权限/范围后再保存一次重检", failed)
	}
	return report
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
