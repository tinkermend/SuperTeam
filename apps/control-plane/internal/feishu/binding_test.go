package feishu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeAPIClient struct {
	openIDsByEmail map[string]string
	oauthOpenID    string
	oauthUnionID   string
	failCode       string
}

func (f *fakeAPIClient) TenantAccessToken(_ context.Context, appID, appSecret string) (string, error) {
	if appSecret == "" {
		return "", errors.New("no secret")
	}
	return "t-token", nil
}

func (f *fakeAPIClient) BatchGetOpenIDsByEmail(_ context.Context, tenantToken string, emails []string) (map[string]string, error) {
	out := map[string]string{}
	for _, email := range emails {
		if openID, ok := f.openIDsByEmail[email]; ok {
			out[email] = openID
		}
	}
	return out, nil
}

func (f *fakeAPIClient) AuthorizeURL(appID, redirectURI, state string) string {
	return "https://feishu.example/authorize?app_id=" + appID + "&state=" + state + "&redirect_uri=" + redirectURI
}

func (f *fakeAPIClient) OAuthUserIdentity(_ context.Context, appID, appSecret, code, redirectURI string) (string, string, error) {
	if code == f.failCode || code == "" {
		return "", "", errors.New("bad code")
	}
	return f.oauthOpenID, f.oauthUnionID, nil
}

type staticUserLister struct {
	users []UserEmail
}

func (l staticUserLister) ListActiveUsersWithEmail(_ context.Context) ([]UserEmail, error) {
	return l.users, nil
}

func setupBindingService(t *testing.T) (*Service, *memoryRepo, uuid.UUID, AppConfig) {
	t.Helper()
	repo := newMemoryRepo()
	service := NewService(repo, fakeSealer{})
	tenantID := uuid.New()
	cfg, err := service.UpsertAppConfig(context.Background(), tenantID, "cli_app", "secret")
	if err != nil {
		t.Fatalf("seed app config: %v", err)
	}
	service.SetOAuthOrigins("http://cp.local:8081", "http://web.local:3000")
	return service, repo, tenantID, cfg
}

func TestContactSyncBindsMatchedUsers(t *testing.T) {
	service, repo, tenantID, cfg := setupBindingService(t)
	alice, bob, carol := uuid.New(), uuid.New(), uuid.New()
	service.SetUserLister(staticUserLister{users: []UserEmail{
		{UserID: alice, Email: "Alice@corp.com"},
		{UserID: bob, Email: "bob@corp.com"},
		{UserID: carol, Email: ""},
	}})
	service.SetClient(&fakeAPIClient{openIDsByEmail: map[string]string{"alice@corp.com": "ou_alice"}})

	reports, err := service.ContactSync(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("contact sync: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected one report, got %#v", reports)
	}
	r := reports[0]
	if r.Bound != 1 || r.Matched != 1 || r.Unmatched != 1 {
		t.Fatalf("unexpected report %#v", r)
	}
	identity, err := service.repo.GetIdentityByUser(context.Background(), cfg.ID, alice)
	if err != nil || identity.OpenID != "ou_alice" || identity.BoundVia != BoundViaContactSync {
		t.Fatalf("expected alice bound via contact_sync, got %#v err=%v", identity, err)
	}
	_ = repo

	// 幂等:再跑一遍,alice 已绑定不重复。
	reports, err = service.ContactSync(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if reports[0].AlreadyBound != 1 || reports[0].Bound != 0 {
		t.Fatalf("expected idempotent second run, got %#v", reports[0])
	}
}

func TestOAuthStartAndComplete(t *testing.T) {
	service, _, tenantID, cfg := setupBindingService(t)
	client := &fakeAPIClient{oauthOpenID: "ou_bob", oauthUnionID: "on_bob"}
	service.SetClient(client)
	userID := uuid.New()

	authorizeURL, err := service.StartOAuth(context.Background(), tenantID, userID, uuid.Nil, "/users")
	if err != nil {
		t.Fatalf("start oauth: %v", err)
	}
	if !strings.Contains(authorizeURL, "app_id=cli_app") {
		t.Fatalf("unexpected authorize url %s", authorizeURL)
	}
	state := authorizeURL[strings.Index(authorizeURL, "state=")+len("state=") : strings.Index(authorizeURL, "&redirect_uri=")]

	returnTo, err := service.CompleteOAuth(context.Background(), "authcode", state)
	if err != nil {
		t.Fatalf("complete oauth: %v", err)
	}
	if returnTo != "http://web.local:3000/users" {
		t.Fatalf("unexpected return_to %s", returnTo)
	}
	identity, err := service.repo.GetIdentityByUser(context.Background(), cfg.ID, userID)
	if err != nil || identity.OpenID != "ou_bob" || identity.BoundVia != BoundViaOAuth {
		t.Fatalf("expected oauth binding, got %#v err=%v", identity, err)
	}
	if identity.UnionID == nil || *identity.UnionID != "on_bob" {
		t.Fatalf("expected union id persisted, got %#v", identity.UnionID)
	}

	// state 一次性:重放同一 state 必须失败。
	if _, err := service.CompleteOAuth(context.Background(), "authcode", state); !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("expected state replay rejected, got %v", err)
	}
}

func TestOAuthRebindReplacesExisting(t *testing.T) {
	service, _, tenantID, cfg := setupBindingService(t)
	client := &fakeAPIClient{oauthOpenID: "ou_new"}
	service.SetClient(client)
	userID := uuid.New()
	if _, err := service.BindIdentity(context.Background(), Identity{
		TenantID: tenantID, AuthUserID: userID, FeishuAppConfigID: cfg.ID,
		OpenID: "ou_old", BoundVia: BoundViaContactSync,
	}); err != nil {
		t.Fatalf("seed old binding: %v", err)
	}

	authorizeURL, err := service.StartOAuth(context.Background(), tenantID, userID, cfg.ID, "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	state := authorizeURL[strings.Index(authorizeURL, "state=")+len("state=") : strings.Index(authorizeURL, "&redirect_uri=")]
	if _, err := service.CompleteOAuth(context.Background(), "authcode", state); err != nil {
		t.Fatalf("complete: %v", err)
	}
	identity, err := service.repo.GetIdentityByUser(context.Background(), cfg.ID, userID)
	if err != nil || identity.OpenID != "ou_new" {
		t.Fatalf("expected rebind to ou_new, got %#v err=%v", identity, err)
	}
}

func TestSanitizeReturnToBlocksOpenRedirect(t *testing.T) {
	service, _, _, _ := setupBindingService(t)
	cases := map[string]string{
		"":                              "http://web.local:3000/users",
		"/settings":                     "http://web.local:3000/settings",
		"//evil.com/x":                  "http://web.local:3000/users",
		"https://evil.com/phish":        "http://web.local:3000/users",
		"http://web.local:3000/profile": "http://web.local:3000/profile",
	}
	for input, want := range cases {
		if got := service.sanitizeReturnTo(input); got != want {
			t.Fatalf("sanitize(%q) = %q, want %q", input, got, want)
		}
	}
}
