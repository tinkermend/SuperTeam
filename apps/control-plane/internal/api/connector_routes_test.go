package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/feishu"
)

type routeServiceAuth struct {
	tenantID uuid.UUID
}

func (a routeServiceAuth) ValidateServiceToken(_ context.Context, serviceName, token string) (middleware.ServiceIdentity, error) {
	if serviceName == "feishu-connector" && token == "svc_valid" {
		return middleware.ServiceIdentity{TokenID: uuid.New(), TenantID: a.tenantID}, nil
	}
	return middleware.ServiceIdentity{}, feishu.ErrIdentityMismatch
}

func newConnectorTestServer(t *testing.T, tenantID uuid.UUID) *Server {
	t.Helper()
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
	)
	feishuService := feishu.NewService(newConnectorRouteRepo(tenantID), staticSealer{})
	server.SetFeishuHandlers(feishu.NewConnectorHTTPHandler(feishuService), feishu.NewAdminHTTPHandler(feishuService))
	server.SetServiceAuth(routeServiceAuth{tenantID: tenantID}, feishuService)
	return server
}

type staticSealer struct{}

func (staticSealer) Seal(plain string) (string, error)  { return "sealed:" + plain, nil }
func (staticSealer) Open(sealed string) (string, error) { return sealed[len("sealed:"):], nil }

type connectorRouteRepo struct {
	tenantID uuid.UUID
	config   feishu.AppConfig
}

func newConnectorRouteRepo(tenantID uuid.UUID) *connectorRouteRepo {
	return &connectorRouteRepo{
		tenantID: tenantID,
		config: feishu.AppConfig{
			ID:              uuid.New(),
			TenantID:        tenantID,
			AppID:           "cli_test",
			AppSecretSealed: "sealed:shh",
			Status:          "active",
		},
	}
}

func (r *connectorRouteRepo) UpsertAppConfig(_ context.Context, tenantID uuid.UUID, appID, secretSealed, status string) (feishu.AppConfig, error) {
	return r.config, nil
}

func (r *connectorRouteRepo) ListActiveAppConfigs(_ context.Context, tenantID uuid.UUID) ([]feishu.AppConfig, error) {
	if tenantID != r.tenantID {
		return nil, nil
	}
	return []feishu.AppConfig{r.config}, nil
}

func (r *connectorRouteRepo) GetAppConfig(_ context.Context, tenantID, id uuid.UUID) (feishu.AppConfig, error) {
	return r.config, nil
}

func (r *connectorRouteRepo) CreateIdentity(_ context.Context, identity feishu.Identity) (feishu.Identity, error) {
	return identity, nil
}

func (r *connectorRouteRepo) GetIdentityByOpenID(_ context.Context, appConfigID uuid.UUID, openID string) (feishu.Identity, error) {
	return feishu.Identity{}, feishu.ErrIdentityNotFound
}

func (r *connectorRouteRepo) GetIdentityByUser(_ context.Context, appConfigID, authUserID uuid.UUID) (feishu.Identity, error) {
	return feishu.Identity{}, feishu.ErrIdentityNotFound
}

func (r *connectorRouteRepo) ListIdentitiesByUsers(_ context.Context, tenantID uuid.UUID, userIDs []uuid.UUID) ([]feishu.Identity, error) {
	return nil, nil
}

func (r *connectorRouteRepo) ListIdentitiesByTenant(_ context.Context, tenantID uuid.UUID) ([]feishu.Identity, error) {
	return nil, nil
}

func (r *connectorRouteRepo) DeleteIdentityByUser(_ context.Context, tenantID, appConfigID, authUserID uuid.UUID) error {
	return nil
}

func TestConnectorRoutesRequireServiceAuth(t *testing.T) {
	server := newConnectorTestServer(t, uuid.New())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connector/bootstrap", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without service token, got %d", rec.Code)
	}
}

func TestConnectorBootstrapReturnsDecryptedConfigs(t *testing.T) {
	tenantID := uuid.New()
	server := newConnectorTestServer(t, tenantID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connector/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer svc_valid")
	req.Header.Set("X-Service-Name", "feishu-connector")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"cli_test", "shh", tenantID.String()} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected bootstrap body to contain %q, got %s", want, body)
		}
	}
}

func TestConnectorIdentityUnboundReturns404(t *testing.T) {
	server := newConnectorTestServer(t, uuid.New())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connector/identity?app_config_id="+uuid.NewString()+"&open_id=ou_x", nil)
	req.Header.Set("Authorization", "Bearer svc_valid")
	req.Header.Set("X-Service-Name", "feishu-connector")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unbound open id, got %d", rec.Code)
	}
}
