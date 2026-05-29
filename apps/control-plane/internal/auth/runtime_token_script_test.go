package auth

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateRuntimeTokenScriptMatchesRuntimeTokenSchema(t *testing.T) {
	content, err := os.ReadFile("../../../../scripts/generate-runtime-token.sh")
	if err != nil {
		t.Fatalf("read generate-runtime-token script: %v", err)
	}

	script := string(content)
	for _, want := range []string{
		"INSERT INTO auth_runtime_tokens (node_id, token_hash, expires_at)",
		"NOW() + :'ttl'::interval",
		"ON CONFLICT (node_id)",
		"-v node_id=",
		"-v token_hash=",
		"-v ttl=",
		"<<SQL",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected script to contain %q", want)
		}
	}
	for _, stale := range []string{
		"auth_runtime_tokens (" + "name",
		"ON CONFLICT (" + "name)",
		"VALUES (:'node_id', :'token_hash', " + "NULL)",
		"-c \"$SQL\"",
		"updated_at",
	} {
		if strings.Contains(script, stale) {
			t.Fatalf("script still contains stale runtime token schema fragment %q", stale)
		}
	}
}
