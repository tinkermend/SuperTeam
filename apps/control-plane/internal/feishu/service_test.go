package feishu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeSealer struct{}

func (fakeSealer) Seal(plain string) (string, error) { return "sealed:" + plain, nil }
func (fakeSealer) Open(sealed string) (string, error) {
	if !strings.HasPrefix(sealed, "sealed:") {
		return "", errors.New("bad ciphertext")
	}
	return strings.TrimPrefix(sealed, "sealed:"), nil
}

type memoryRepo struct {
	configs    map[uuid.UUID]AppConfig
	identities []Identity
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{configs: map[uuid.UUID]AppConfig{}}
}

func (r *memoryRepo) UpsertAppConfig(_ context.Context, tenantID uuid.UUID, appID, secretSealed, status string) (AppConfig, error) {
	for id, cfg := range r.configs {
		if cfg.TenantID == tenantID && cfg.AppID == appID {
			cfg.AppSecretSealed = secretSealed
			cfg.Status = status
			r.configs[id] = cfg
			return cfg, nil
		}
	}
	cfg := AppConfig{ID: uuid.New(), TenantID: tenantID, AppID: appID, AppSecretSealed: secretSealed, Status: status}
	r.configs[cfg.ID] = cfg
	return cfg, nil
}

func (r *memoryRepo) ListActiveAppConfigs(_ context.Context, tenantID uuid.UUID) ([]AppConfig, error) {
	var out []AppConfig
	for _, cfg := range r.configs {
		if cfg.TenantID == tenantID && cfg.Status == "active" {
			out = append(out, cfg)
		}
	}
	return out, nil
}

func (r *memoryRepo) GetAppConfig(_ context.Context, tenantID, id uuid.UUID) (AppConfig, error) {
	cfg, ok := r.configs[id]
	if !ok || cfg.TenantID != tenantID {
		return AppConfig{}, ErrAppConfigNotFound
	}
	return cfg, nil
}

func (r *memoryRepo) CreateIdentity(_ context.Context, identity Identity) (Identity, error) {
	for _, existing := range r.identities {
		if existing.FeishuAppConfigID == identity.FeishuAppConfigID &&
			(existing.OpenID == identity.OpenID || existing.AuthUserID == identity.AuthUserID) {
			return Identity{}, errors.New("unique violation")
		}
	}
	identity.ID = uuid.New()
	r.identities = append(r.identities, identity)
	return identity, nil
}

func (r *memoryRepo) GetIdentityByOpenID(_ context.Context, appConfigID uuid.UUID, openID string) (Identity, error) {
	for _, identity := range r.identities {
		if identity.FeishuAppConfigID == appConfigID && identity.OpenID == openID {
			return identity, nil
		}
	}
	return Identity{}, ErrIdentityNotFound
}

func (r *memoryRepo) GetIdentityByUser(_ context.Context, appConfigID, authUserID uuid.UUID) (Identity, error) {
	for _, identity := range r.identities {
		if identity.FeishuAppConfigID == appConfigID && identity.AuthUserID == authUserID {
			return identity, nil
		}
	}
	return Identity{}, ErrIdentityNotFound
}

func (r *memoryRepo) ListIdentitiesByUsers(_ context.Context, tenantID uuid.UUID, userIDs []uuid.UUID) ([]Identity, error) {
	var out []Identity
	for _, identity := range r.identities {
		if identity.TenantID != tenantID {
			continue
		}
		for _, userID := range userIDs {
			if identity.AuthUserID == userID {
				out = append(out, identity)
			}
		}
	}
	return out, nil
}

func (r *memoryRepo) ListIdentitiesByTenant(_ context.Context, tenantID uuid.UUID) ([]Identity, error) {
	var out []Identity
	for _, identity := range r.identities {
		if identity.TenantID == tenantID {
			out = append(out, identity)
		}
	}
	return out, nil
}

func (r *memoryRepo) DeleteIdentityByUser(_ context.Context, tenantID, appConfigID, authUserID uuid.UUID) error {
	kept := r.identities[:0]
	for _, identity := range r.identities {
		if identity.TenantID == tenantID && identity.FeishuAppConfigID == appConfigID && identity.AuthUserID == authUserID {
			continue
		}
		kept = append(kept, identity)
	}
	r.identities = kept
	return nil
}

func TestUpsertAppConfigSealsSecret(t *testing.T) {
	repo := newMemoryRepo()
	service := NewService(repo, fakeSealer{})
	tenantID := uuid.New()

	cfg, err := service.UpsertAppConfig(context.Background(), tenantID, "cli_app", "raw-secret")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if cfg.AppSecretSealed != "sealed:raw-secret" {
		t.Fatalf("expected sealed secret stored, got %q", cfg.AppSecretSealed)
	}

	boots, err := service.BootstrapConfigs(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if len(boots) != 1 || boots[0].AppSecret != "raw-secret" || boots[0].AppID != "cli_app" {
		t.Fatalf("expected decrypted bootstrap config, got %#v", boots)
	}
}

func TestUpsertAppConfigRequiresSealer(t *testing.T) {
	service := NewService(newMemoryRepo(), nil)
	if _, err := service.UpsertAppConfig(context.Background(), uuid.New(), "cli_app", "s"); !errors.Is(err, ErrSealerRequired) {
		t.Fatalf("expected sealer required, got %v", err)
	}
}

func TestVerifyOnBehalfOf(t *testing.T) {
	repo := newMemoryRepo()
	service := NewService(repo, fakeSealer{})
	tenantID := uuid.New()
	appConfigID := uuid.New()
	userID := uuid.New()
	repo.identities = append(repo.identities, Identity{
		ID: uuid.New(), TenantID: tenantID, AuthUserID: userID,
		FeishuAppConfigID: appConfigID, OpenID: "ou_real", BoundVia: BoundViaOAuth,
	})

	if err := service.VerifyOnBehalfOf(context.Background(), tenantID, userID, "ou_real"); err != nil {
		t.Fatalf("expected match, got %v", err)
	}
	if err := service.VerifyOnBehalfOf(context.Background(), tenantID, userID, "ou_forged"); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("expected mismatch for forged open id, got %v", err)
	}
	if err := service.VerifyOnBehalfOf(context.Background(), tenantID, uuid.New(), "ou_real"); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("expected mismatch for unbound user, got %v", err)
	}
}

func TestBindAndRebindIdentity(t *testing.T) {
	repo := newMemoryRepo()
	service := NewService(repo, fakeSealer{})
	tenantID := uuid.New()
	appConfigID := uuid.New()
	userID := uuid.New()

	if _, err := service.BindIdentity(context.Background(), Identity{
		TenantID: tenantID, AuthUserID: userID, FeishuAppConfigID: appConfigID,
		OpenID: "ou_1", BoundVia: "manual",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid bound_via rejected, got %v", err)
	}

	first, err := service.BindIdentity(context.Background(), Identity{
		TenantID: tenantID, AuthUserID: userID, FeishuAppConfigID: appConfigID,
		OpenID: "ou_1", BoundVia: BoundViaContactSync,
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if first.OpenID != "ou_1" {
		t.Fatalf("unexpected identity %#v", first)
	}

	rebound, err := service.RebindIdentity(context.Background(), Identity{
		TenantID: tenantID, AuthUserID: userID, FeishuAppConfigID: appConfigID,
		OpenID: "ou_2", BoundVia: BoundViaOAuth,
	})
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if rebound.OpenID != "ou_2" {
		t.Fatalf("expected new open id, got %#v", rebound)
	}
	if _, err := service.ResolveIdentityByOpenID(context.Background(), appConfigID, "ou_1"); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("expected old binding removed, got %v", err)
	}
}
