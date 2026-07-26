package feishu

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrOAuthStateInvalid = errors.New("oauth state invalid or expired")
	ErrClientRequired    = errors.New("feishu client is not configured")
	ErrNoAppConfig       = errors.New("no active feishu app config")
)

// UserLister 提供通讯录反查所需的平台用户联系方式来源,由 app 层用 auth 服务适配。
type UserLister interface {
	ListActiveUsersWithContact(ctx context.Context) ([]UserContact, error)
}

type UserContact struct {
	UserID uuid.UUID
	Email  string
	Mobile string
}

// APIClient 是 Client 的接口视图(测试可替换)。
type APIClient interface {
	TenantAccessToken(ctx context.Context, appID, appSecret string) (string, error)
	BatchGetOpenIDs(ctx context.Context, tenantToken string, emails, mobiles []string) (emailMatches, mobileMatches map[string]string, err error)
	ProbeContactDirectory(ctx context.Context, tenantToken string) (ok bool, code int, msg string, err error)
	ProbeBotInfo(ctx context.Context, tenantToken string) (ok bool, code int, msg string, err error)
	AuthorizeURL(appID, redirectURI, state string) string
	OAuthUserIdentity(ctx context.Context, appID, appSecret, code, redirectURI string) (openID, unionID string, err error)
}

func (s *Service) SetClient(client APIClient) { s.client = client }

func (s *Service) SetUserLister(lister UserLister) { s.userLister = lister }

// SetOAuthOrigins 配置 OAuth 回调地址(控制平面对浏览器可达的 origin)与
// 授权完成后允许回跳的 Web origin。
func (s *Service) SetOAuthOrigins(publicOrigin, webOrigin string) {
	s.publicOrigin = strings.TrimRight(strings.TrimSpace(publicOrigin), "/")
	s.webOrigin = strings.TrimRight(strings.TrimSpace(webOrigin), "/")
}

type ContactSyncReport struct {
	AppID        string `json:"app_id"`
	Matched      int    `json:"matched"`
	Bound        int    `json:"bound"`
	AlreadyBound int    `json:"already_bound"`
	Unmatched    int    `json:"unmatched"`
	// Conflicts:同一用户的邮箱与手机号命中了不同 open_id(档案填串了或平台
	// 联系方式填错人),不静默绑任何一边,计数报出留人工裁决。
	Conflicts int `json:"conflicts"`
}

// ContactSync 按邮箱+手机号批量反查并绑定(零用户操作的初始化路径)。已绑定的
// 跳过;任一键命中即绑;双键命中不同人计 conflict;全失配留给 OAuth 补绑。
func (s *Service) ContactSync(ctx context.Context, tenantID uuid.UUID) ([]ContactSyncReport, error) {
	if s.client == nil {
		return nil, ErrClientRequired
	}
	if s.userLister == nil {
		return nil, errors.New("user lister is not configured")
	}
	if s.sealer == nil {
		return nil, ErrSealerRequired
	}
	configs, err := s.repo.ListActiveAppConfigs(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, ErrNoAppConfig
	}
	users, err := s.userLister.ListActiveUsersWithContact(ctx)
	if err != nil {
		return nil, err
	}

	reports := make([]ContactSyncReport, 0, len(configs))
	for _, cfg := range configs {
		report := ContactSyncReport{AppID: cfg.AppID}
		secret, err := s.sealer.Open(cfg.AppSecretSealed)
		if err != nil {
			return nil, err
		}
		token, err := s.client.TenantAccessToken(ctx, cfg.AppID, secret)
		if err != nil {
			return nil, err
		}

		type candidate struct {
			userID uuid.UUID
			email  string
			mobile string
		}
		var candidates []candidate
		var emails, mobiles []string
		for _, user := range users {
			email := strings.TrimSpace(strings.ToLower(user.Email))
			mobile := strings.TrimSpace(user.Mobile)
			if email == "" && mobile == "" {
				continue
			}
			if _, err := s.repo.GetIdentityByUser(ctx, cfg.ID, user.UserID); err == nil {
				report.AlreadyBound++
				continue
			} else if !errors.Is(err, ErrIdentityNotFound) {
				return nil, err
			}
			candidates = append(candidates, candidate{userID: user.UserID, email: email, mobile: mobile})
			if email != "" {
				emails = append(emails, email)
			}
			if mobile != "" {
				mobiles = append(mobiles, mobile)
			}
		}

		emailMatches, mobileMatches, err := s.client.BatchGetOpenIDs(ctx, token, emails, mobiles)
		if err != nil {
			return nil, err
		}
		for _, cand := range candidates {
			var emailOpenID, mobileOpenID string
			if cand.email != "" {
				emailOpenID = matchIgnoreCase(emailMatches, cand.email)
			}
			if cand.mobile != "" {
				mobileOpenID = mobileMatches[cand.mobile]
			}
			if emailOpenID == "" && mobileOpenID == "" {
				report.Unmatched++
				continue
			}
			report.Matched++
			if emailOpenID != "" && mobileOpenID != "" && emailOpenID != mobileOpenID {
				report.Conflicts++
				continue
			}
			openID := emailOpenID
			if openID == "" {
				openID = mobileOpenID
			}
			if _, err := s.BindIdentity(ctx, Identity{
				TenantID:          tenantID,
				AuthUserID:        cand.userID,
				FeishuAppConfigID: cfg.ID,
				OpenID:            openID,
				BoundVia:          BoundViaContactSync,
			}); err != nil {
				// open_id 撞唯一约束(比如已被他人绑定)不阻断整批,计入 unmatched。
				report.Unmatched++
				report.Matched--
				continue
			}
			report.Bound++
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// matchIgnoreCase 邮箱命中查找:飞书回带的 email 大小写可能与请求不一致,
// 先精确后小写比对。
func matchIgnoreCase(matches map[string]string, key string) string {
	if openID, ok := matches[key]; ok {
		return openID
	}
	for candidate, openID := range matches {
		if strings.EqualFold(candidate, key) {
			return openID
		}
	}
	return ""
}

type oauthState struct {
	TenantID    uuid.UUID
	UserID      uuid.UUID
	AppConfigID uuid.UUID
	ReturnTo    string
	ExpiresAt   time.Time
}

type oauthStateStore struct {
	mu     sync.Mutex
	states map[string]oauthState
}

func newOAuthStateStore() *oauthStateStore {
	return &oauthStateStore{states: map[string]oauthState{}}
}

func (s *oauthStateStore) put(state string, value oauthState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, existing := range s.states {
		if time.Now().After(existing.ExpiresAt) {
			delete(s.states, key)
		}
	}
	s.states[state] = value
}

func (s *oauthStateStore) take(state string) (oauthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.states[state]
	if !ok {
		return oauthState{}, false
	}
	delete(s.states, state)
	if time.Now().After(value.ExpiresAt) {
		return oauthState{}, false
	}
	return value, true
}

const oauthStateTTL = 10 * time.Minute

// StartOAuth 生成一次性 state 并返回飞书授权页地址。appConfigID 为空时取租户
// 首个 active 配置。state 在内存存续(单副本假设,P1 已知约束)。
func (s *Service) StartOAuth(ctx context.Context, tenantID, userID, appConfigID uuid.UUID, returnTo string) (string, error) {
	if s.client == nil {
		return "", ErrClientRequired
	}
	if s.publicOrigin == "" {
		return "", errors.New("oauth public origin is not configured")
	}
	if appConfigID == uuid.Nil {
		configs, err := s.repo.ListActiveAppConfigs(ctx, tenantID)
		if err != nil {
			return "", err
		}
		if len(configs) == 0 {
			return "", ErrNoAppConfig
		}
		appConfigID = configs[0].ID
	} else if _, err := s.repo.GetAppConfig(ctx, tenantID, appConfigID); err != nil {
		return "", err
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	state := hex.EncodeToString(raw)
	s.oauthStates.put(state, oauthState{
		TenantID:    tenantID,
		UserID:      userID,
		AppConfigID: appConfigID,
		ReturnTo:    s.sanitizeReturnTo(returnTo),
		ExpiresAt:   time.Now().Add(oauthStateTTL),
	})

	cfg, err := s.repo.GetAppConfig(ctx, tenantID, appConfigID)
	if err != nil {
		return "", err
	}
	return s.client.AuthorizeURL(cfg.AppID, s.oauthRedirectURI(), state), nil
}

// CompleteOAuth 消费 state,换取用户身份并(重)绑定,返回回跳地址。
func (s *Service) CompleteOAuth(ctx context.Context, code, state string) (string, error) {
	if s.client == nil {
		return "", ErrClientRequired
	}
	if s.sealer == nil {
		return "", ErrSealerRequired
	}
	value, ok := s.oauthStates.take(strings.TrimSpace(state))
	if !ok || strings.TrimSpace(code) == "" {
		return "", ErrOAuthStateInvalid
	}
	cfg, err := s.repo.GetAppConfig(ctx, value.TenantID, value.AppConfigID)
	if err != nil {
		return "", err
	}
	secret, err := s.sealer.Open(cfg.AppSecretSealed)
	if err != nil {
		return "", err
	}
	openID, unionID, err := s.client.OAuthUserIdentity(ctx, cfg.AppID, secret, code, s.oauthRedirectURI())
	if err != nil {
		return "", err
	}
	identity := Identity{
		TenantID:          value.TenantID,
		AuthUserID:        value.UserID,
		FeishuAppConfigID: value.AppConfigID,
		OpenID:            openID,
		BoundVia:          BoundViaOAuth,
	}
	if unionID != "" {
		identity.UnionID = &unionID
	}
	if _, err := s.RebindIdentity(ctx, identity); err != nil {
		return "", err
	}
	return value.ReturnTo, nil
}

func (s *Service) oauthRedirectURI() string {
	return s.publicOrigin + "/api/v1/auth/feishu/oauth-callback"
}

// sanitizeReturnTo 只允许相对路径或 webOrigin 前缀的绝对地址,防开放重定向。
func (s *Service) sanitizeReturnTo(returnTo string) string {
	returnTo = strings.TrimSpace(returnTo)
	fallback := s.webOrigin + "/users"
	if returnTo == "" {
		return fallback
	}
	if strings.HasPrefix(returnTo, "/") && !strings.HasPrefix(returnTo, "//") {
		return s.webOrigin + returnTo
	}
	if s.webOrigin != "" && strings.HasPrefix(returnTo, s.webOrigin+"/") {
		return returnTo
	}
	return fallback
}
