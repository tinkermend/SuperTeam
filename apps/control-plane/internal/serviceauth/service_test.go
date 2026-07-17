package serviceauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type memoryRepo struct {
	tokens []ServiceToken
}

func (r *memoryRepo) CreateServiceToken(_ context.Context, tenantID uuid.UUID, serviceName, tokenHash string) (ServiceToken, error) {
	token := ServiceToken{
		ID:          uuid.New(),
		TenantID:    tenantID,
		ServiceName: serviceName,
		TokenHash:   tokenHash,
		Status:      "active",
		CreatedAt:   time.Now(),
	}
	r.tokens = append(r.tokens, token)
	return token, nil
}

func (r *memoryRepo) ListActiveServiceTokensByName(_ context.Context, serviceName string) ([]ServiceToken, error) {
	var out []ServiceToken
	for _, t := range r.tokens {
		if t.ServiceName == serviceName && t.Status == "active" {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *memoryRepo) TouchServiceTokenLastUsed(_ context.Context, id uuid.UUID) error { return nil }

func (r *memoryRepo) RevokeServiceToken(_ context.Context, tenantID, id uuid.UUID) (ServiceToken, error) {
	for i, t := range r.tokens {
		if t.ID == id && t.TenantID == tenantID && t.Status == "active" {
			r.tokens[i].Status = "revoked"
			return r.tokens[i], nil
		}
	}
	return ServiceToken{}, ErrTokenNotFound
}

func TestIssueAndValidateServiceToken(t *testing.T) {
	repo := &memoryRepo{}
	service := NewService(repo)
	tenantID := uuid.New()

	plaintext, issued, err := service.IssueToken(context.Background(), tenantID, "feishu-connector")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if plaintext == "" || issued.TokenHash == plaintext {
		t.Fatalf("expected plaintext distinct from stored hash")
	}

	validated, err := service.ValidateServiceToken(context.Background(), "feishu-connector", plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if validated.TenantID != tenantID {
		t.Fatalf("expected tenant %s, got %s", tenantID, validated.TenantID)
	}
}

func TestValidateRejectsWrongTokenAndWrongService(t *testing.T) {
	repo := &memoryRepo{}
	service := NewService(repo)
	plaintext, _, err := service.IssueToken(context.Background(), uuid.New(), "feishu-connector")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	if _, err := service.ValidateServiceToken(context.Background(), "feishu-connector", "wrong-"+plaintext); !errors.Is(err, ErrInvalidServiceToken) {
		t.Fatalf("expected invalid token, got %v", err)
	}
	if _, err := service.ValidateServiceToken(context.Background(), "other-service", plaintext); !errors.Is(err, ErrInvalidServiceToken) {
		t.Fatalf("expected invalid for wrong service, got %v", err)
	}
	if _, err := service.ValidateServiceToken(context.Background(), "feishu-connector", ""); !errors.Is(err, ErrInvalidServiceToken) {
		t.Fatalf("expected invalid for empty token, got %v", err)
	}
}

func TestValidateRejectsRevokedToken(t *testing.T) {
	repo := &memoryRepo{}
	service := NewService(repo)
	tenantID := uuid.New()
	plaintext, issued, err := service.IssueToken(context.Background(), tenantID, "feishu-connector")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := service.RevokeToken(context.Background(), tenantID, issued.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := service.ValidateServiceToken(context.Background(), "feishu-connector", plaintext); !errors.Is(err, ErrInvalidServiceToken) {
		t.Fatalf("expected invalid after revoke, got %v", err)
	}
}
