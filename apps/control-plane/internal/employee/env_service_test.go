package employee

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestEnvironmentCryptoRoundTripsAndHidesPlaintext(t *testing.T) {
	codec, err := NewEnvironmentValueCodec(EnvironmentValueCodecConfig{
		Keys:        "v1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
		ActiveKeyID: "v1",
	})
	if err != nil {
		t.Fatalf("codec config: %v", err)
	}

	encrypted, err := codec.Encrypt("GH_TOKEN", "ghp_secret_value")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if encrypted.EncryptedValue == "" || strings.Contains(encrypted.EncryptedValue, "ghp_secret_value") {
		t.Fatalf("encrypted value leaked plaintext: %q", encrypted.EncryptedValue)
	}
	if encrypted.KeyID != "v1" {
		t.Fatalf("key id mismatch: %s", encrypted.KeyID)
	}
	if encrypted.Fingerprint == "" {
		t.Fatal("fingerprint is required")
	}

	plain, err := codec.Decrypt("GH_TOKEN", encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plain != "ghp_secret_value" {
		t.Fatalf("plain mismatch: %q", plain)
	}
}

func TestEnvironmentCryptoRejectsMissingActiveKey(t *testing.T) {
	_, err := NewEnvironmentValueCodec(EnvironmentValueCodecConfig{
		Keys:        "v1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)),
		ActiveKeyID: "v2",
	})
	if err == nil || !strings.Contains(err.Error(), "active key") {
		t.Fatalf("expected active key error, got %v", err)
	}
}
