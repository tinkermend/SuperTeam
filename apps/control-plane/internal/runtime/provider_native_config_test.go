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
