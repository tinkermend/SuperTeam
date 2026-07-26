// Package serviceauth 提供外部服务凭据的签发与验证:服务(如 feishu-connector)以
// Bearer token + X-Service-Name 认证;业务动作的判权仍以 on-behalf-of 绑定用户为
// 行为人,服务凭据只解决"这是谁家的进程"的认证问题。
package serviceauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidServiceToken = errors.New("invalid service token")
	ErrTokenNotFound       = errors.New("service token not found")
)

type ServiceToken struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	ServiceName string
	TokenHash   string
	Status      string
	LastUsedAt  *time.Time
	CreatedAt   time.Time
	RevokedAt   *time.Time
}

type Repository interface {
	CreateServiceToken(ctx context.Context, tenantID uuid.UUID, serviceName, tokenHash string) (ServiceToken, error)
	ListActiveServiceTokensByName(ctx context.Context, serviceName string) ([]ServiceToken, error)
	ListServiceTokensByTenant(ctx context.Context, tenantID uuid.UUID) ([]ServiceToken, error)
	TouchServiceTokenLastUsed(ctx context.Context, id uuid.UUID) error
	RevokeServiceToken(ctx context.Context, tenantID, id uuid.UUID) (ServiceToken, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// IssueToken 生成随机凭据,存 bcrypt 哈希,明文只返回一次。
func (s *Service) IssueToken(ctx context.Context, tenantID uuid.UUID, serviceName string) (string, ServiceToken, error) {
	serviceName = strings.TrimSpace(serviceName)
	if tenantID == uuid.Nil || serviceName == "" {
		return "", ServiceToken{}, ErrInvalidServiceToken
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", ServiceToken{}, err
	}
	plaintext := "svc_" + hex.EncodeToString(raw)
	// bcrypt 输入上限 72 字节:svc_ 前缀 + 64 hex = 68 字节,留有余量。
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", ServiceToken{}, err
	}
	token, err := s.repo.CreateServiceToken(ctx, tenantID, serviceName, string(hash))
	if err != nil {
		return "", ServiceToken{}, err
	}
	return plaintext, token, nil
}

// ValidateServiceToken 对该服务名下全部 active 凭据做 bcrypt 比对(服务凭据数量
// 以个位数计,逐行比对成本可忽略)。
func (s *Service) ValidateServiceToken(ctx context.Context, serviceName, token string) (ServiceToken, error) {
	serviceName = strings.TrimSpace(serviceName)
	token = strings.TrimSpace(token)
	if serviceName == "" || token == "" {
		return ServiceToken{}, ErrInvalidServiceToken
	}
	candidates, err := s.repo.ListActiveServiceTokensByName(ctx, serviceName)
	if err != nil {
		return ServiceToken{}, err
	}
	for _, candidate := range candidates {
		if bcrypt.CompareHashAndPassword([]byte(candidate.TokenHash), []byte(token)) == nil {
			_ = s.repo.TouchServiceTokenLastUsed(ctx, candidate.ID)
			return candidate, nil
		}
	}
	return ServiceToken{}, ErrInvalidServiceToken
}

func (s *Service) RevokeToken(ctx context.Context, tenantID, id uuid.UUID) (ServiceToken, error) {
	return s.repo.RevokeServiceToken(ctx, tenantID, id)
}

// ListTokens 管理面列表(含 revoked;不回显 hash/明文)。
func (s *Service) ListTokens(ctx context.Context, tenantID uuid.UUID) ([]ServiceToken, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidServiceToken
	}
	return s.repo.ListServiceTokensByTenant(ctx, tenantID)
}
