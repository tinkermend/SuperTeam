package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newFakeFeishu(t *testing.T) (*httptest.Server, *Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/open-apis/auth/v3/tenant_access_token/internal", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["app_id"] != "cli_app" || body["app_secret"] != "secret" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 10003, "msg": "invalid app credentials"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "t-token"})
	})
	mux.HandleFunc("/open-apis/contact/v3/users/batch_get_id", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t-token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 99991663, "msg": "invalid token"})
			return
		}
		// 回带条目按请求键构造:命中的邮箱/手机号各一条,ghost 无 user_id(未命中形态)。
		var body struct {
			Emails  []string `json:"emails"`
			Mobiles []string `json:"mobiles"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		userList := []map[string]string{}
		for _, email := range body.Emails {
			if email == "alice@corp.com" {
				userList = append(userList, map[string]string{"user_id": "ou_alice", "email": email})
			} else {
				userList = append(userList, map[string]string{"email": email})
			}
		}
		for _, mobile := range body.Mobiles {
			if mobile == "+8613800138000" {
				userList = append(userList, map[string]string{"user_id": "ou_mobile", "mobile": mobile})
			} else {
				userList = append(userList, map[string]string{"mobile": mobile})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"user_list": userList},
		})
	})
	mux.HandleFunc("/open-apis/authen/v2/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["code"] != "authcode" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 20050, "msg": "bad code"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "u-token"})
	})
	mux.HandleFunc("/open-apis/authen/v1/user_info", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer u-token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 99991663, "msg": "invalid token"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]string{"open_id": "ou_bob", "union_id": "on_bob"},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, NewClient(server.URL)
}

func TestTenantAccessToken(t *testing.T) {
	_, client := newFakeFeishu(t)
	token, err := client.TenantAccessToken(context.Background(), "cli_app", "secret")
	if err != nil || token != "t-token" {
		t.Fatalf("expected token, got %q err=%v", token, err)
	}
	if _, err := client.TenantAccessToken(context.Background(), "cli_app", "wrong"); err == nil {
		t.Fatalf("expected error for bad credentials")
	}
}

func TestBatchGetOpenIDs(t *testing.T) {
	_, client := newFakeFeishu(t)
	emailMatches, mobileMatches, err := client.BatchGetOpenIDs(context.Background(), "t-token",
		[]string{"alice@corp.com", "ghost@corp.com"}, []string{"+8613800138000", "+8600000000000"})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if emailMatches["alice@corp.com"] != "ou_alice" {
		t.Fatalf("expected alice matched, got %#v", emailMatches)
	}
	if _, ok := emailMatches["ghost@corp.com"]; ok {
		t.Fatalf("expected ghost unmatched (no user_id), got %#v", emailMatches)
	}
	if mobileMatches["+8613800138000"] != "ou_mobile" {
		t.Fatalf("expected mobile matched, got %#v", mobileMatches)
	}
	if _, ok := mobileMatches["+8600000000000"]; ok {
		t.Fatalf("expected unknown mobile unmatched, got %#v", mobileMatches)
	}
	emptyEmails, emptyMobiles, err := client.BatchGetOpenIDs(context.Background(), "t-token", nil, nil)
	if err != nil || len(emptyEmails) != 0 || len(emptyMobiles) != 0 {
		t.Fatalf("expected empty result for no keys, got %#v/%#v err=%v", emptyEmails, emptyMobiles, err)
	}
}

func TestOAuthUserIdentity(t *testing.T) {
	_, client := newFakeFeishu(t)
	openID, unionID, err := client.OAuthUserIdentity(context.Background(), "cli_app", "secret", "authcode", "https://cp/callback")
	if err != nil || openID != "ou_bob" || unionID != "on_bob" {
		t.Fatalf("expected identity, got %q/%q err=%v", openID, unionID, err)
	}
	if _, _, err := client.OAuthUserIdentity(context.Background(), "cli_app", "secret", "badcode", "https://cp/callback"); err == nil {
		t.Fatalf("expected error for bad code")
	}
}

func TestAuthorizeURL(t *testing.T) {
	client := NewClient("https://open.feishu.cn")
	u := client.AuthorizeURL("cli_app", "https://cp/callback", "state123")
	for _, want := range []string{"open-apis/authen/v1/authorize", "app_id=cli_app", "state=state123", "redirect_uri=https%3A%2F%2Fcp%2Fcallback"} {
		if !strings.Contains(u, want) {
			t.Fatalf("authorize url missing %q: %s", want, u)
		}
	}
}
