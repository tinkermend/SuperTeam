package employee

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"
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

func TestEnvironmentServiceEncryptsAndDecryptsOnlyForRuntime(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetEnvironmentCodec(testEnvironmentCodec(t))
	tenantID := uuid.New()
	teamID := uuid.New()
	employeeID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:           employeeID,
		TenantID:     tenantID,
		TeamID:       &teamID,
		OwnerUserID:  uuid.New(),
		EmployeeType: "database_admin",
		Name:         "Database administrator",
		Role:         "database_admin",
		Status:       DigitalEmployeeStatusReady,
	}

	summary, err := svc.UpsertEnvironmentVariable(context.Background(), UpsertEnvironmentVariableRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Name:              " GH_TOKEN ",
		Value:             "ghp_secret_value",
		Sensitive:         true,
	})
	if err != nil {
		t.Fatalf("upsert env var: %v", err)
	}
	if summary.Name != "GH_TOKEN" || !summary.Configured || summary.Fingerprint == "" || summary.Sensitive != true {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if strings.Contains(summary.Fingerprint, "ghp_secret_value") {
		t.Fatalf("summary leaked plaintext: %#v", summary)
	}
	if len(repo.envVars) != 1 {
		t.Fatalf("expected one stored env var, got %#v", repo.envVars)
	}
	for _, record := range repo.envVars {
		if record.EncryptedValue == "" || strings.Contains(record.EncryptedValue, "ghp_secret_value") {
			t.Fatalf("stored value leaked plaintext: %#v", record)
		}
	}

	list, err := svc.ListEnvironmentVariables(context.Background(), ListEnvironmentVariablesRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		t.Fatalf("list env vars: %v", err)
	}
	if len(list) != 1 || list[0].Name != "GH_TOKEN" || list[0].Fingerprint == "" {
		t.Fatalf("unexpected env summary list: %#v", list)
	}

	runtimeVars, err := svc.ListRuntimeEnvironmentVariablesForRuntime(context.Background(), tenantID, employeeID)
	if err != nil {
		t.Fatalf("list runtime env vars: %v", err)
	}
	if len(runtimeVars) != 1 || runtimeVars[0].Name != "GH_TOKEN" || runtimeVars[0].Value != "ghp_secret_value" {
		t.Fatalf("unexpected runtime env vars: %#v", runtimeVars)
	}

	if err := svc.DeleteEnvironmentVariable(context.Background(), DeleteEnvironmentVariableRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Name:              "GH_TOKEN",
	}); err != nil {
		t.Fatalf("delete env var: %v", err)
	}
	list, err = svc.ListEnvironmentVariables(context.Background(), ListEnvironmentVariablesRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected env var to be deleted, got %#v", list)
	}
}

func testEnvironmentCodec(t *testing.T) *EnvironmentValueCodec {
	t.Helper()
	codec, err := NewEnvironmentValueCodec(EnvironmentValueCodecConfig{
		Keys:        "v1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)),
		ActiveKeyID: "v1",
	})
	if err != nil {
		t.Fatalf("new env codec: %v", err)
	}
	return codec
}
