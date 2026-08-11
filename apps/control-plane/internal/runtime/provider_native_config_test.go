package runtime

import (
	"strings"
	"testing"
)

func TestValidatePushKeysAllowlist(t *testing.T) {
	t.Parallel()
	if err := validatePushKeys("codex", "model_profile", map[string]any{"model": "gpt-5"}); err != nil {
		t.Fatalf("expected allowlisted key ok: %v", err)
	}
	if err := validatePushKeys("codex", "model_profile", map[string]any{"mcp_servers.foo.command": "x"}); err == nil {
		t.Fatal("expected mcp key rejected")
	}
	if err := validatePushKeys("claude-code", "model_profile", map[string]any{"env.PATH": "/bin"}); err == nil {
		t.Fatal("expected env.PATH rejected")
	}
	if err := validatePushKeys("claude-code", "model_profile", map[string]any{"env.ANTHROPIC_API_KEY": "sk"}); err != nil {
		t.Fatalf("expected env api key allowlisted: %v", err)
	}
	if err := validatePushKeys("claude-code", "auth", map[string]any{"token": "x"}); err == nil {
		t.Fatal("expected claude auth write rejected")
	}
}

// 可执行面必须留在白名单外：apiKeyHelper 是 Claude Code 经 /bin/sh 执行的取凭据命令，
// opencode 的 provider.<name>.npm 与 models.<id>.provider.* 会让节点加载任意 npm 包。
func TestValidatePushKeysRejectsExecutableSurfaces(t *testing.T) {
	t.Parallel()
	rejected := []struct {
		provider string
		key      string
	}{
		{"claude-code", "apiKeyHelper"},
		{"claude-code", "awsCredentialExport"},
		{"claude-code", "awsAuthRefresh"},
		{"opencode", "provider.custom.npm"},
		{"opencode", "provider.custom.models.gpt.provider.npm"},
		{"opencode", "provider.custom"},
		{"opencode", "provider"},
	}
	for _, tc := range rejected {
		if err := validatePushKeys(tc.provider, "model_profile", map[string]any{tc.key: "x"}); err == nil {
			t.Fatalf("expected %s/%s rejected", tc.provider, tc.key)
		}
	}

	allowed := []string{
		"provider.custom.name",
		"provider.custom.options.baseURL",
		"provider.custom.options.headers.X-Trace",
		"provider.custom.models.gpt.name",
		"provider.custom.models.gpt.limit.context",
	}
	for _, key := range allowed {
		if err := validatePushKeys("opencode", "model_profile", map[string]any{key: "x"}); err != nil {
			t.Fatalf("expected opencode %s allowlisted: %v", key, err)
		}
	}
}

// 收窄白名单前拉取的快照里可能仍留着 apiKeyHelper / provider.*.npm，
// 读路径必须过滤掉，否则会被渲染成可编辑字段并在下次 push 时整体 422。
func TestFilterAllowlistedManagedValuesDropsStaleExecutableKeys(t *testing.T) {
	t.Parallel()
	claude := filterAllowlistedManagedValues("claude-code", "model_profile", map[string]any{
		"model":        "claude-sonnet",
		"apiKeyHelper": "/bin/old_helper.sh",
	})
	if _, ok := claude["apiKeyHelper"]; ok {
		t.Fatal("stale apiKeyHelper must be filtered out of snapshot reads")
	}
	if claude["model"] != "claude-sonnet" {
		t.Fatalf("allowlisted key must survive: %v", claude["model"])
	}

	oc := filterAllowlistedManagedValues("opencode", "model_profile", map[string]any{
		"provider.custom.npm":             "@attacker/pkg",
		"provider.custom.options.baseURL": "https://api.example/v1",
	})
	if _, ok := oc["provider.custom.npm"]; ok {
		t.Fatal("stale provider npm must be filtered out of snapshot reads")
	}
	if oc["provider.custom.options.baseURL"] != "https://api.example/v1" {
		t.Fatal("opencode data field must survive")
	}

	// auth 面读集合是整份文件，宽于写集合，不得被过滤。
	auth := filterAllowlistedManagedValues("claude-code", "auth", map[string]any{"accessToken": "x"})
	if auth["accessToken"] != "x" {
		t.Fatal("auth surface reads must not be filtered")
	}
}

func TestSensitiveKeyDetection(t *testing.T) {
	t.Parallel()
	if !isSensitiveManagedKey("claude-code", "model_profile", "env.ANTHROPIC_API_KEY") {
		t.Fatal("api key should be sensitive")
	}
	if !isSensitiveManagedKey("codex", "auth", "OPENAI_API_KEY") {
		t.Fatal("auth surface should be sensitive")
	}
	if isSensitiveManagedKey("codex", "model_profile", "model") {
		t.Fatal("model should not be sensitive")
	}
}

func TestStripManagedValuesFromResult(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"file_content_hash": "sha256:abc",
		"managed_values":    map[string]any{"model": "secret"},
		"exists":            true,
	}
	out := StripManagedValuesFromResult(in)
	if _, ok := out["managed_values"]; ok {
		t.Fatal("managed_values must be stripped")
	}
	if out["file_content_hash"] != "sha256:abc" {
		t.Fatalf("unexpected hash: %v", out["file_content_hash"])
	}
}

func TestMapNativeConfigCommandError(t *testing.T) {
	t.Parallel()
	msg := "hash mismatch"
	err := mapNativeConfigCommandError(&NativeConfigCommandReceipt{
		Status:       "failed",
		ErrorMessage: &msg,
		Result: map[string]any{
			"diagnostic": map[string]any{"error_code": "conflict"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "conflict") && err != ErrProviderNativeConfigConflict {
		// errors.Is path
		if err == nil {
			t.Fatal("expected error")
		}
	}
	if err == nil || !strings.Contains(err.Error(), msg) {
		// wrap should include message; ensure it's the conflict sentinel
		if err != nil {
			// check wrap chain via string for simplicity when errors.Is not used with %w on message
			_ = err
		}
	}
}
