package feishu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeAPIClient struct {
	openIDsByEmail  map[string]string
	openIDsByMobile map[string]string
	oauthOpenID     string
	oauthUnionID    string
	failCode        string
	failToken       bool
}

func (f *fakeAPIClient) TenantAccessToken(_ context.Context, appID, appSecret string) (string, error) {
	if f.failToken {
		return "", errors.New("app secret invalid")
	}
	if appSecret == "" {
		return "", errors.New("no secret")
	}
	return "t-token", nil
}

func (f *fakeAPIClient) BatchGetOpenIDs(_ context.Context, tenantToken string, emails, mobiles []string) (map[string]string, map[string]string, error) {
	emailOut := map[string]string{}
	for _, email := range emails {
		if openID, ok := f.openIDsByEmail[email]; ok {
			emailOut[email] = openID
		}
	}
	mobileOut := map[string]string{}
	for _, mobile := range mobiles {
		if openID, ok := f.openIDsByMobile[mobile]; ok {
			mobileOut[mobile] = openID
		}
	}
	return emailOut, mobileOut, nil
}

func (f *fakeAPIClient) ProbeContactDirectory(_ context.Context, tenantToken string) (bool, int, string, error) {
	return true, 0, "ok", nil
}

func (f *fakeAPIClient) ProbeBotInfo(_ context.Context, tenantToken string) (bool, int, string, error) {
	return true, 0, "ok", nil
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
	users []UserContact
}

func (l staticUserLister) ListActiveUsersWithContact(_ context.Context) ([]UserContact, error) {
	return l.users, nil
}

func setupBindingService(t *testing.T) (*Service, *memoryRepo, uuid.UUID, AppConfig) {
	t.Helper()
	repo := newMemoryRepo()
	service := NewService(repo, fakeSealer{})
	service.SetClient(&fakeAPIClient{})
	tenantID := uuid.New()
	cfg, _, err := service.UpsertAppConfig(context.Background(), tenantID, "cli_app", "secret")
	if err != nil {
		t.Fatalf("seed app config: %v", err)
	}
	service.SetOAuthOrigins("http://cp.local:8081", "http://web.local:3000")
	return service, repo, tenantID, cfg
}

func TestContactSyncBindsMatchedUsers(t *testing.T) {
	service, repo, tenantID, cfg := setupBindingService(t)
	alice, bob, carol := uuid.New(), uuid.New(), uuid.New()
	service.SetUserLister(staticUserLister{users: []UserContact{
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

// TestContactSyncMobileLegAndConflict 钉住手机号腿(spec 2026-07-27 §6 提前项):
// 邮箱缺失手机号命中→绑定;邮箱与手机号命中不同 open_id→conflict 不静默绑任一边。
func TestContactSyncMobileLegAndConflict(t *testing.T) {
	service, _, tenantID, cfg := setupBindingService(t)
	mobileOnly, conflicted, both := uuid.New(), uuid.New(), uuid.New()
	service.SetUserLister(staticUserLister{users: []UserContact{
		{UserID: mobileOnly, Mobile: "+8613800138000"},
		{UserID: conflicted, Email: "mixed@corp.com", Mobile: "+8613900139000"},
		{UserID: both, Email: "dana@corp.com", Mobile: "+8613700137000"},
	}})
	service.SetClient(&fakeAPIClient{
		openIDsByEmail: map[string]string{
			"mixed@corp.com": "ou_email_person",
			"dana@corp.com":  "ou_dana",
		},
		openIDsByMobile: map[string]string{
			"+8613800138000": "ou_mobile_only",
			"+8613900139000": "ou_other_person",
			"+8613700137000": "ou_dana",
		},
	})

	reports, err := service.ContactSync(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("contact sync: %v", err)
	}
	r := reports[0]
	if r.Bound != 2 || r.Conflicts != 1 || r.Unmatched != 0 {
		t.Fatalf("unexpected report %#v", r)
	}
	identity, err := service.repo.GetIdentityByUser(context.Background(), cfg.ID, mobileOnly)
	if err != nil || identity.OpenID != "ou_mobile_only" {
		t.Fatalf("expected mobile-only user bound, got %#v err=%v", identity, err)
	}
	if identity.BoundVia != BoundViaContactSync {
		t.Fatalf("expected contact_sync via, got %s", identity.BoundVia)
	}
	// 双键一致的用户正常绑定。
	if identity, err := service.repo.GetIdentityByUser(context.Background(), cfg.ID, both); err != nil || identity.OpenID != "ou_dana" {
		t.Fatalf("expected dual-key user bound, got %#v err=%v", identity, err)
	}
	// 冲突用户不绑任何一边。
	if _, err := service.repo.GetIdentityByUser(context.Background(), cfg.ID, conflicted); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("conflicted user must stay unbound, got err=%v", err)
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
