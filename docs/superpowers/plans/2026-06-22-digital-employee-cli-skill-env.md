# Digital Employee CLI Skill Env Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the CLI-first skill dependency and encrypted digital-employee environment-variable runtime path described in `docs/superpowers/specs/2026-06-22-digital-employee-cli-skill-env-design.md`.

**Architecture:** Keep Skill as the single user-facing skill model, with `runtime_dependencies` stored in `skills.metadata`. Store digital employee environment variables in a dedicated encrypted table owned by Control Plane; Runtime Agent only receives per-command plaintext env values. Runtime Agent reports `tool` capabilities from configured PATH probes, and Control Plane evaluates skill loadability before dispatching runs.

**Tech Stack:** Go Control Plane, PostgreSQL + Atlas/sqlc, OpenAPI/oapi-codegen, Rust Runtime Agent, React/TypeScript Web, Vitest, Go tests, Cargo tests.

---

## File Structure

Create:

- `apps/control-plane/internal/storage/migrations/032_digital_employee_env_and_skill_dependencies.sql` - DB table for encrypted employee env vars and skill metadata defaults.
- `apps/control-plane/internal/employee/env_crypto.go` - key parsing, AES-GCM encryption, decryption, fingerprinting.
- `apps/control-plane/internal/employee/env_repository.go` - repository interface additions are kept near employee repository contracts.
- `apps/control-plane/internal/employee/env_service.go` - env var validation, create/update/delete/list, decrypt for runtime.
- `apps/control-plane/internal/employee/env_service_test.go` - crypto and service tests.
- `apps/runtime-agent/src/tools.rs` - PATH tool probing helpers.

Modify:

- `contracts/control-plane/openapi.yaml` - add request/response fields and employee env endpoints.
- `apps/control-plane/internal/api/server.go` - route employee env endpoints.
- `apps/control-plane/internal/api/employee_routes_test.go` - route coverage for env summary and authz.
- `apps/control-plane/internal/api/skill_routes_test.go` - route coverage for `runtime_dependencies`.
- `apps/control-plane/internal/employee/types.go` - env request/response/domain types and create request field.
- `apps/control-plane/internal/employee/handler.go` - env handlers and create request parsing.
- `apps/control-plane/internal/employee/pg_repository.go` - env table CRUD and runtime env queries.
- `apps/control-plane/internal/employee/repository.go` - env repository methods.
- `apps/control-plane/internal/employee/service.go` - create flow saves initial env vars.
- `apps/control-plane/internal/employee/run_service.go` - dependency evaluation, skill payload, env decrypt payload.
- `apps/control-plane/internal/employee/service_test.go` - create/run preflight tests.
- `apps/control-plane/internal/skill/types.go` - `SkillRuntimeDependencies`, dependency status types.
- `apps/control-plane/internal/skill/handler.go` - upload form parse and response fields.
- `apps/control-plane/internal/skill/service.go` - dependency normalization on upload.
- `apps/control-plane/internal/skill/pg_repository.go` - scan and persist `skills.metadata`.
- `apps/control-plane/internal/skill/repository.go` - dependency-aware runtime record contracts.
- `apps/control-plane/internal/skill/service_test.go` - dependency normalization and upload tests.
- `apps/control-plane/internal/runtime/models.go` - no schema change, but tests should assert `tool` capability is accepted.
- `apps/runtime-agent/src/config.rs` - `tools.probe_names` config and env override.
- `apps/runtime-agent/src/daemon.rs` - append `tool` capabilities from probe names.
- `apps/runtime-agent/src/commands/payload.rs` - parse `environment`.
- `apps/runtime-agent/src/runs.rs` - carry env in `RunSpec`.
- `apps/runtime-agent/src/providers/mod.rs` - carry env in `ProviderRequest`.
- `apps/runtime-agent/src/providers/claude.rs` - inject env.
- `apps/runtime-agent/src/providers/codex.rs` - inject env.
- `apps/runtime-agent/src/providers/opencode.rs` - inject env.
- `apps/runtime-agent/tests/daemon_test.rs` - config/tool capability tests.
- `apps/runtime-agent/tests/runtime_command_payload_test.rs` - environment payload parsing tests.
- `apps/runtime-agent/tests/provider_command_test.rs` - env injection tests.
- `apps/web/src/lib/api/skills.ts` - skill dependency types and upload fields.
- `apps/web/src/lib/api/employees.ts` - env summary and mutation APIs.
- `apps/web/src/features/skills/index.tsx` - runtime dependency inputs and display.
- `apps/web/src/features/skills/index.test.tsx` - dependency form tests.
- `apps/web/src/features/employees/create.tsx` - initial env vars in employee create flow.
- `apps/web/src/features/employees/create.test.tsx` - env create tests.
- `apps/web/src/features/employees/detail.tsx` - employee detail page entry and layout.
- `apps/web/src/features/employees/components/employee-capabilities-panel.tsx` - employee skill/MCP panel; add env summary and skill load status here or split a focused child component from it.
- `CHANGELOG.md` - timestamped entry after implementation is complete.

Generated:

- `apps/control-plane/internal/api/server.gen.go` - regenerate from OpenAPI.
- Any OpenAPI-generated types changed by `corepack pnpm generate:control-plane`.

---

### Task 1: Control Plane Encrypted Environment Foundation

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/032_digital_employee_env_and_skill_dependencies.sql`
- Create: `apps/control-plane/internal/employee/env_crypto.go`
- Create: `apps/control-plane/internal/employee/env_service_test.go`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/employee/types.go`

- [ ] **Step 1: Write migration test first**

Add a migration test in `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestDigitalEmployeeEnvironmentVariablesMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/032_digital_employee_env_and_skill_dependencies.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE digital_employee_environment_variables",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"team_id UUID NOT NULL",
		"digital_employee_id UUID NOT NULL",
		"name TEXT NOT NULL",
		"encrypted_value TEXT NOT NULL",
		"encryption_key_id TEXT NOT NULL",
		"value_fingerprint TEXT NOT NULL",
		"sensitive BOOLEAN NOT NULL DEFAULT true",
		"status VARCHAR(50) NOT NULL DEFAULT 'active'",
		"metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE UNIQUE INDEX digital_employee_env_unique_active_name",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"plain_value",
		"plaintext",
		"decrypted_value",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration must not store plaintext env values, found %q", forbidden)
		}
	}
}
```

- [ ] **Step 2: Run migration test to verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeEnvironmentVariablesMigration -count=1
```

Expected: fail because `digital_employee_environment_variables` does not exist.

- [ ] **Step 3: Add migration**

Create `apps/control-plane/internal/storage/migrations/032_digital_employee_env_and_skill_dependencies.sql`:

```sql
CREATE TABLE digital_employee_environment_variables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    team_id UUID NOT NULL REFERENCES tenant_teams(id),
    digital_employee_id UUID NOT NULL REFERENCES digital_employees(id),
    name TEXT NOT NULL,
    encrypted_value TEXT NOT NULL,
    encryption_key_id TEXT NOT NULL,
    value_fingerprint TEXT NOT NULL,
    sensitive BOOLEAN NOT NULL DEFAULT true,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_by UUID REFERENCES auth_users(id),
    updated_by UUID REFERENCES auth_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT digital_employee_env_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT digital_employee_env_encrypted_value_not_blank CHECK (btrim(encrypted_value) <> ''),
    CONSTRAINT digital_employee_env_key_id_not_blank CHECK (btrim(encryption_key_id) <> ''),
    CONSTRAINT digital_employee_env_status_supported CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX digital_employee_env_unique_active_name
    ON digital_employee_environment_variables (tenant_id, digital_employee_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX digital_employee_env_employee_idx
    ON digital_employee_environment_variables (tenant_id, digital_employee_id)
    WHERE deleted_at IS NULL;

CREATE INDEX digital_employee_env_team_idx
    ON digital_employee_environment_variables (tenant_id, team_id)
    WHERE deleted_at IS NULL;

ALTER TABLE skills
    ALTER COLUMN metadata SET DEFAULT '{}'::jsonb;
```

Run Atlas hash update after the file is final:

```bash
atlas migrate hash --dir file://apps/control-plane/internal/storage/migrations
```

- [ ] **Step 4: Add env crypto tests**

Create `apps/control-plane/internal/employee/env_service_test.go` with these tests:

```go
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
```

- [ ] **Step 5: Run crypto tests to verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestEnvironmentCrypto' -count=1
```

Expected: fail because `NewEnvironmentValueCodec` and related types do not exist.

- [ ] **Step 6: Implement env crypto**

Create `apps/control-plane/internal/employee/env_crypto.go` with:

```go
package employee

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

type EnvironmentValueCodecConfig struct {
	Keys        string
	ActiveKeyID string
}

type EnvironmentEncryptedValue struct {
	KeyID          string
	EncryptedValue string
	Fingerprint   string
}

type EnvironmentValueCodec struct {
	activeKeyID string
	keys        map[string][]byte
}

func NewEnvironmentValueCodec(cfg EnvironmentValueCodecConfig) (*EnvironmentValueCodec, error) {
	keys := map[string][]byte{}
	for _, item := range strings.Split(cfg.Keys, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("%w: invalid environment encryption key entry", ErrInvalidInput)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("%w: decode environment encryption key: %v", ErrInvalidInput, err)
		}
		if len(raw) != 32 {
			return nil, fmt.Errorf("%w: environment encryption key must be 32 bytes", ErrInvalidInput)
		}
		keys[strings.TrimSpace(parts[0])] = raw
	}
	active := strings.TrimSpace(cfg.ActiveKeyID)
	if active == "" {
		return nil, fmt.Errorf("%w: active environment encryption key id is required", ErrInvalidInput)
	}
	if _, ok := keys[active]; !ok {
		return nil, fmt.Errorf("%w: active environment encryption key is not configured", ErrInvalidInput)
	}
	return &EnvironmentValueCodec{activeKeyID: active, keys: keys}, nil
}

func (c *EnvironmentValueCodec) Encrypt(name, value string) (EnvironmentEncryptedValue, error) {
	key := c.keys[c.activeKeyID]
	block, err := aes.NewCipher(key)
	if err != nil {
		return EnvironmentEncryptedValue{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EnvironmentEncryptedValue{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return EnvironmentEncryptedValue{}, err
	}
	aad := []byte(strings.TrimSpace(name))
	ciphertext := gcm.Seal(nil, nonce, []byte(value), aad)
	sealed := append(nonce, ciphertext...)
	return EnvironmentEncryptedValue{
		KeyID:          c.activeKeyID,
		EncryptedValue: base64.StdEncoding.EncodeToString(sealed),
		Fingerprint:   fingerprintValue(key, name, value),
	}, nil
}

func (c *EnvironmentValueCodec) Decrypt(name string, value EnvironmentEncryptedValue) (string, error) {
	key, ok := c.keys[value.KeyID]
	if !ok {
		return "", fmt.Errorf("%w: environment encryption key is not configured", ErrInvalidInput)
	}
	sealed, err := base64.StdEncoding.DecodeString(value.EncryptedValue)
	if err != nil {
		return "", fmt.Errorf("%w: decode encrypted environment value: %v", ErrInvalidInput, err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", fmt.Errorf("%w: encrypted environment value is truncated", ErrInvalidInput)
	}
	nonce := sealed[:gcm.NonceSize()]
	ciphertext := sealed[gcm.NonceSize():]
	aad := []byte(strings.TrimSpace(name))
	plain, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", fmt.Errorf("%w: decrypt environment value", ErrInvalidInput)
	}
	return string(plain), nil
}

func fingerprintValue(key []byte, name, value string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(strings.TrimSpace(name)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum)[:12]
}
```

- [ ] **Step 7: Run task tests**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeEnvironmentVariablesMigration -count=1
go test ./apps/control-plane/internal/employee -run 'TestEnvironmentCrypto' -count=1
git diff --check
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/032_digital_employee_env_and_skill_dependencies.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/migrations_test.go \
  apps/control-plane/internal/employee/env_crypto.go \
  apps/control-plane/internal/employee/env_service_test.go
git commit -m "feat: add encrypted employee environment storage foundation"
```

---

### Task 2: Employee Environment API And Repository

**Files:**
- Create: `apps/control-plane/internal/employee/env_repository.go`
- Create: `apps/control-plane/internal/employee/env_service.go`
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [ ] **Step 1: Add route tests first**

In `apps/control-plane/internal/api/employee_routes_test.go`, add route coverage:

```go
func TestDigitalEmployeeEnvironmentVariableRoutes(t *testing.T) {
	service := &routeEmployeeService{}
	server := newTestServer(t)
	server.SetEmployeeHandler(employee.NewHandler(service))
	employeeID := uuid.New()

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String()+"/environment-variables", nil)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list env vars to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}

	upsertReq := httptest.NewRequest(http.MethodPut, "/api/v1/digital-employees/"+employeeID.String()+"/environment-variables/GH_TOKEN", strings.NewReader(`{"value":"secret","sensitive":true}`))
	upsertResp := httptest.NewRecorder()
	server.ServeHTTP(upsertResp, upsertReq)
	if upsertResp.Code != http.StatusOK {
		t.Fatalf("expected upsert env var to succeed, got %d: %s", upsertResp.Code, upsertResp.Body.String())
	}
	if strings.Contains(upsertResp.Body.String(), "secret") {
		t.Fatalf("response leaked plaintext: %s", upsertResp.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/digital-employees/"+employeeID.String()+"/environment-variables/GH_TOKEN", nil)
	deleteResp := httptest.NewRecorder()
	server.ServeHTTP(deleteResp, deleteReq)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("expected delete env var to succeed, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
}
```

Extend `routeEmployeeService` to satisfy new methods:

```go
func (s *routeEmployeeService) ListEnvironmentVariables(ctx context.Context, req employee.ListEnvironmentVariablesRequest) ([]employee.EnvironmentVariableSummary, error) {
	s.listEnvReq = req
	return []employee.EnvironmentVariableSummary{{Name: "GH_TOKEN", Configured: true, Fingerprint: "abc123", Sensitive: true, Status: "active"}}, nil
}

func (s *routeEmployeeService) UpsertEnvironmentVariable(ctx context.Context, req employee.UpsertEnvironmentVariableRequest) (employee.EnvironmentVariableSummary, error) {
	s.upsertEnvReq = req
	return employee.EnvironmentVariableSummary{Name: req.Name, Configured: true, Fingerprint: "abc123", Sensitive: true, Status: "active"}, nil
}

func (s *routeEmployeeService) DeleteEnvironmentVariable(ctx context.Context, req employee.DeleteEnvironmentVariableRequest) error {
	s.deleteEnvReq = req
	return nil
}
```

- [ ] **Step 2: Run route test to verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestDigitalEmployeeEnvironmentVariableRoutes -count=1
```

Expected: compile or route failure because handlers and service methods do not exist.

- [ ] **Step 3: Add employee env domain types**

In `apps/control-plane/internal/employee/types.go`, add:

```go
type EnvironmentVariableStatus string

const (
	EnvironmentVariableStatusActive   EnvironmentVariableStatus = "active"
	EnvironmentVariableStatusDisabled EnvironmentVariableStatus = "disabled"
)

type EnvironmentVariableSummary struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	Configured        bool
	Fingerprint       string
	Sensitive         bool
	Status            EnvironmentVariableStatus
	UpdatedAt         time.Time
}

type EnvironmentVariableRecord struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	EncryptedValue    string
	EncryptionKeyID   string
	ValueFingerprint  string
	Sensitive         bool
	Status            EnvironmentVariableStatus
	CreatedBy         *uuid.UUID
	UpdatedBy         *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ListEnvironmentVariablesRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type UpsertEnvironmentVariableRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	Value             string
	Sensitive         bool
	ActorUserID       *uuid.UUID
}

type DeleteEnvironmentVariableRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
}
```

- [ ] **Step 4: Implement service/repository contracts**

Add methods to `apps/control-plane/internal/employee/repository.go` or the new `env_repository.go`:

```go
type EnvironmentVariableRepository interface {
	ListEnvironmentVariables(ctx context.Context, req ListEnvironmentVariablesRequest) ([]EnvironmentVariableRecord, error)
	UpsertEnvironmentVariable(ctx context.Context, req UpsertEnvironmentVariableStoreRequest) (EnvironmentVariableRecord, error)
	DeleteEnvironmentVariable(ctx context.Context, req DeleteEnvironmentVariableRequest) error
	ListRuntimeEnvironmentVariables(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]EnvironmentVariableRecord, error)
}

type UpsertEnvironmentVariableStoreRequest struct {
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Name              string
	EncryptedValue    string
	EncryptionKeyID   string
	ValueFingerprint  string
	Sensitive         bool
	UpdatedBy         *uuid.UUID
}
```

Make the concrete employee repository satisfy this interface.

- [ ] **Step 5: Implement env service**

Create `apps/control-plane/internal/employee/env_service.go` with validation:

```go
var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func normalizeEnvName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !envNamePattern.MatchString(name) {
		return "", fmt.Errorf("%w: invalid environment variable name", ErrInvalidInput)
	}
	return name, nil
}

func environmentSummaryFromRecord(record EnvironmentVariableRecord) EnvironmentVariableSummary {
	return EnvironmentVariableSummary{
		ID:                record.ID,
		TenantID:          record.TenantID,
		TeamID:            record.TeamID,
		DigitalEmployeeID: record.DigitalEmployeeID,
		Name:              record.Name,
		Configured:        record.EncryptedValue != "",
		Fingerprint:       record.ValueFingerprint,
		Sensitive:         record.Sensitive,
		Status:            record.Status,
		UpdatedAt:         record.UpdatedAt,
	}
}
```

Add methods on `Service`:

```go
func (s *Service) ListEnvironmentVariables(ctx context.Context, req ListEnvironmentVariablesRequest) ([]EnvironmentVariableSummary, error)
func (s *Service) UpsertEnvironmentVariable(ctx context.Context, req UpsertEnvironmentVariableRequest) (EnvironmentVariableSummary, error)
func (s *Service) DeleteEnvironmentVariable(ctx context.Context, req DeleteEnvironmentVariableRequest) error
```

Each method must verify tenant and employee IDs, normalize name, confirm employee exists with `GetDigitalEmployee`, encrypt values through `EnvironmentValueCodec`, and never return plaintext.

- [ ] **Step 6: Implement PgRepository methods**

In `apps/control-plane/internal/employee/pg_repository.go`, add SQL using the existing pgx style:

```sql
INSERT INTO digital_employee_environment_variables (
    tenant_id, team_id, digital_employee_id, name,
    encrypted_value, encryption_key_id, value_fingerprint, sensitive,
    status, created_by, updated_by, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active',$9,$9,NOW())
ON CONFLICT (tenant_id, digital_employee_id, name) WHERE deleted_at IS NULL
DO UPDATE SET
    encrypted_value = EXCLUDED.encrypted_value,
    encryption_key_id = EXCLUDED.encryption_key_id,
    value_fingerprint = EXCLUDED.value_fingerprint,
    sensitive = EXCLUDED.sensitive,
    status = 'active',
    updated_by = EXCLUDED.updated_by,
    updated_at = NOW()
RETURNING ...
```

Use soft delete:

```sql
UPDATE digital_employee_environment_variables
SET deleted_at = NOW(), updated_at = NOW()
WHERE tenant_id = $1 AND digital_employee_id = $2 AND name = $3 AND deleted_at IS NULL
```

- [ ] **Step 7: Add HTTP handlers and routes**

Extend `HandlerService` in `apps/control-plane/internal/employee/handler.go` with env methods. Add handlers:

```go
func (h *HTTPHandler) ListEnvironmentVariables(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) UpsertEnvironmentVariable(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) DeleteEnvironmentVariable(w http.ResponseWriter, r *http.Request)
```

Routes in `apps/control-plane/internal/api/server.go`:

```go
r.Get("/digital-employees/{employeeId}/environment-variables", s.employeeHandler.ListEnvironmentVariables)
r.Put("/digital-employees/{employeeId}/environment-variables/{envName}", s.employeeHandler.UpsertEnvironmentVariable)
r.Delete("/digital-employees/{employeeId}/environment-variables/{envName}", s.employeeHandler.DeleteEnvironmentVariable)
```

Use `authz.ActionEmployeeRead` for list and `authz.ActionEmployeeConfigCreate` for upsert/delete.

- [ ] **Step 8: Update OpenAPI**

In `contracts/control-plane/openapi.yaml`, add:

- `GET /api/v1/digital-employees/{employeeId}/environment-variables`
- `PUT /api/v1/digital-employees/{employeeId}/environment-variables/{envName}`
- `DELETE /api/v1/digital-employees/{employeeId}/environment-variables/{envName}`
- schemas `DigitalEmployeeEnvironmentVariableSummary` and `UpsertEnvironmentVariableRequest`

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

- [ ] **Step 9: Run task tests**

```bash
go test ./apps/control-plane/internal/employee -run 'TestEnvironment' -count=1
go test ./apps/control-plane/internal/api -run TestDigitalEmployeeEnvironmentVariableRoutes -count=1
corepack pnpm verify:contracts
git diff --check
```

Expected: all pass.

- [ ] **Step 10: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api/server.gen.go \
  apps/control-plane/internal/api/server.go apps/control-plane/internal/api/employee_routes_test.go \
  apps/control-plane/internal/employee/types.go apps/control-plane/internal/employee/repository.go \
  apps/control-plane/internal/employee/pg_repository.go apps/control-plane/internal/employee/env_repository.go \
  apps/control-plane/internal/employee/env_service.go apps/control-plane/internal/employee/env_service_test.go
git commit -m "feat: manage encrypted employee environment variables"
```

---

### Task 3: Skill Runtime Dependencies

**Files:**
- Modify: `apps/control-plane/internal/skill/types.go`
- Modify: `apps/control-plane/internal/skill/service.go`
- Modify: `apps/control-plane/internal/skill/handler.go`
- Modify: `apps/control-plane/internal/skill/pg_repository.go`
- Modify: `apps/control-plane/internal/skill/service_test.go`
- Modify: `apps/control-plane/internal/api/skill_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [ ] **Step 1: Write skill dependency service test first**

In `apps/control-plane/internal/skill/service_test.go`, add:

```go
func TestServiceUploadSkillStoresRuntimeDependencies(t *testing.T) {
	repo := &serviceTestRepository{}
	service := newTestService(repo)
	archive := buildSkillZip(t, map[string]string{
		"SKILL.md": "# GitHub Skill\n\nUses gh.",
	})

	item, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID: uuid.New(),
		Name:     "GitHub Skill",
		Archive:  archive,
		Filename: "github-skill.zip",
		RuntimeDependencies: SkillRuntimeDependencies{
			Tools: []string{" gh ", "gh", "kubectl"},
			Env:   []string{"GH_TOKEN", " GH_TOKEN "},
		},
	})
	if err != nil {
		t.Fatalf("upload skill: %v", err)
	}

	if !stringSlicesEqual(repo.upsertReq.RuntimeDependencies.Tools, []string{"gh", "kubectl"}) {
		t.Fatalf("tools mismatch: %#v", repo.upsertReq.RuntimeDependencies.Tools)
	}
	if !stringSlicesEqual(repo.upsertReq.RuntimeDependencies.Env, []string{"GH_TOKEN"}) {
		t.Fatalf("env mismatch: %#v", repo.upsertReq.RuntimeDependencies.Env)
	}
	if !stringSlicesEqual(item.RuntimeDependencies.Tools, []string{"gh", "kubectl"}) {
		t.Fatalf("returned tools mismatch: %#v", item.RuntimeDependencies.Tools)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./apps/control-plane/internal/skill -run TestServiceUploadSkillStoresRuntimeDependencies -count=1
```

Expected: compile failure because `RuntimeDependencies` does not exist.

- [ ] **Step 3: Add domain types and normalization**

In `apps/control-plane/internal/skill/types.go`, add:

```go
type SkillRuntimeDependencies struct {
	Tools []string `json:"tools"`
	Env   []string `json:"env"`
}

type SkillRuntimeDependencyStatus struct {
	LoadStatus  string   `json:"load_status"`
	MissingTools []string `json:"missing_tools"`
	MissingEnv   []string `json:"missing_env"`
}
```

Add `RuntimeDependencies SkillRuntimeDependencies` to `Skill`, `UploadSkillRequest`, and `UpsertSkillPackageRequest`. Add `RuntimeDependencyStatus *SkillRuntimeDependencyStatus` to `SkillAgentBinding` or `EffectiveEmployeeSkill`, whichever is easier for the existing response shape.

In `apps/control-plane/internal/skill/service.go`, add:

```go
var skillToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
var skillEnvNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func normalizeRuntimeDependencies(input SkillRuntimeDependencies) (SkillRuntimeDependencies, error) {
	tools, err := normalizeDependencyList(input.Tools, skillToolNamePattern, "tool")
	if err != nil {
		return SkillRuntimeDependencies{}, err
	}
	env, err := normalizeDependencyList(input.Env, skillEnvNamePattern, "env")
	if err != nil {
		return SkillRuntimeDependencies{}, err
	}
	return SkillRuntimeDependencies{Tools: tools, Env: env}, nil
}
```

Use sorted unique output.

- [ ] **Step 4: Persist dependencies in skills.metadata**

In `apps/control-plane/internal/skill/pg_repository.go`:

- Add `s.metadata` to `skillSelectColumns`.
- Scan it as `[]byte` or `map[string]any`.
- Marshal dependency metadata during `UpsertSkillPackage`.
- Insert/update `metadata`.

Metadata shape:

```json
{
  "runtime_dependencies": {
    "tools": ["gh"],
    "env": ["GH_TOKEN"]
  }
}
```

- [ ] **Step 5: Parse upload form**

In `apps/control-plane/internal/skill/handler.go`, accept either:

```text
runtime_tools=gh,kubectl
runtime_env=GH_TOKEN
```

or JSON:

```text
runtime_dependencies={"tools":["gh"],"env":["GH_TOKEN"]}
```

Use `splitFormList` for comma lists and validate in service.

- [ ] **Step 6: Update route test**

In `apps/control-plane/internal/api/skill_routes_test.go`, extend upload multipart helper to include:

```go
_ = writer.WriteField("runtime_tools", "gh,kubectl")
_ = writer.WriteField("runtime_env", "GH_TOKEN")
```

Assert response includes:

```go
if got := response.RuntimeDependencies.Tools; !slices.Equal(got, []string{"gh", "kubectl"}) {
	t.Fatalf("tools mismatch: %#v", got)
}
```

- [ ] **Step 7: Update OpenAPI**

Add `runtime_dependencies` to Skill schema and upload request schema in `contracts/control-plane/openapi.yaml`, then run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

- [ ] **Step 8: Run task tests**

```bash
go test ./apps/control-plane/internal/skill -run RuntimeDependencies -count=1
go test ./apps/control-plane/internal/api -run SkillRoutes -count=1
corepack pnpm verify:contracts
git diff --check
```

- [ ] **Step 9: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api/server.gen.go \
  apps/control-plane/internal/skill/types.go apps/control-plane/internal/skill/service.go \
  apps/control-plane/internal/skill/handler.go apps/control-plane/internal/skill/pg_repository.go \
  apps/control-plane/internal/skill/service_test.go apps/control-plane/internal/api/skill_routes_test.go
git commit -m "feat: add skill runtime dependency metadata"
```

---

### Task 4: Runtime Agent Tool Capability Probe

**Files:**
- Create: `apps/runtime-agent/src/tools.rs`
- Modify: `apps/runtime-agent/src/lib.rs`
- Modify: `apps/runtime-agent/src/config.rs`
- Modify: `apps/runtime-agent/src/daemon.rs`
- Modify: `apps/runtime-agent/tests/daemon_test.rs`

- [ ] **Step 1: Add failing config and daemon tests**

In `apps/runtime-agent/tests/daemon_test.rs`, add a config test:

```rust
#[test]
fn config_loads_tool_probe_names_from_env() {
    let cfg = RuntimeConfig::load_with_env(
        None,
        [
            ("RUNTIME_AGENT_NODE_ID", "node-a"),
            ("RUNTIME_AGENT_BOOTSTRAP_KEY", "bootstrap"),
            ("RUNTIME_AGENT_TOOL_PROBE_NAMES", "git, gh,kubectl"),
        ],
        RuntimeConfigOverrides::default(),
    )
    .expect("config");

    assert_eq!(cfg.tools.probe_names, vec!["git", "gh", "kubectl"]);
}
```

Add a capability test near existing daemon capability tests:

```rust
#[tokio::test]
async fn runtime_reports_configured_tool_capabilities() {
    let mut cfg = RuntimeConfig::new("node-a").expect("config");
    cfg.tools.probe_names = vec!["definitely-missing-superteam-tool".to_string()];

    let capabilities = superteam_runtime_agent::daemon::build_capabilities_for_test(&cfg).await;
    let tool = capabilities
        .iter()
        .find(|cap| cap.capability_type == "tool" && cap.capability_key == "definitely-missing-superteam-tool")
        .expect("tool capability");

    assert!(!tool.available);
    assert_eq!(tool.status, "missing");
    assert_eq!(tool.provider_type, "tool");
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml config_loads_tool_probe_names_from_env runtime_reports_configured_tool_capabilities
```

Expected: compile failure because `tools` config and test helper do not exist.

- [ ] **Step 3: Add tools config**

In `apps/runtime-agent/src/config.rs`, add:

```rust
pub tools: ToolsSection,

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ToolsSection {
    pub probe_names: Vec<String>,
}
```

Add file config:

```rust
tools: Option<FileToolsSection>,

#[derive(Debug, Deserialize, Default)]
struct FileToolsSection {
    probe_names: Option<Vec<String>>,
}
```

Default:

```rust
tools: ToolsSection {
    probe_names: vec!["git".to_string(), "gh".to_string()],
},
```

Env parsing:

```rust
"RUNTIME_AGENT_TOOL_PROBE_NAMES" => {
    self.tools.probe_names = parse_csv(value);
}
```

Add `parse_csv`:

```rust
fn parse_csv(value: &str) -> Vec<String> {
    let mut values: Vec<String> = value
        .split(',')
        .map(str::trim)
        .filter(|item| !item.is_empty())
        .map(ToString::to_string)
        .collect();
    values.sort();
    values.dedup();
    values
}
```

- [ ] **Step 4: Add tool probe helper**

Create `apps/runtime-agent/src/tools.rs`:

```rust
use std::env;
use std::path::PathBuf;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ToolProbeResult {
    pub name: String,
    pub binary_path: Option<PathBuf>,
    pub available: bool,
}

pub fn probe_tool(name: &str) -> ToolProbeResult {
    let binary_path = env::var_os("PATH")
        .and_then(|path| {
            env::split_paths(&path)
                .map(|dir| dir.join(name))
                .find(|candidate| candidate.is_file())
        });
    ToolProbeResult {
        name: name.to_string(),
        available: binary_path.is_some(),
        binary_path,
    }
}
```

- [ ] **Step 5: Append tool capabilities**

In `apps/runtime-agent/src/daemon.rs`, expose a test helper:

```rust
#[cfg(test)]
pub async fn build_capabilities_for_test(config: &RuntimeConfig) -> Vec<RuntimeCapabilityInput> {
    build_capabilities(config).await
}
```

Append tool capabilities in `build_capabilities`:

```rust
for name in &config.tools.probe_names {
    let probe = crate::tools::probe_tool(name);
    capabilities.push(RuntimeCapabilityInput {
        capability_type: "tool".to_string(),
        capability_key: probe.name,
        provider_type: "tool".to_string(),
        provider_version: None,
        binary_path: probe.binary_path.map(|path| path.display().to_string()),
        available: probe.available,
        workspace_base_dir: None,
        capacity: None,
        labels: None,
        status: if probe.available { "available" } else { "missing" }.to_string(),
        details: None,
        health_status: if probe.available { "configured" } else { "missing" }.to_string(),
        metadata: None,
    });
}
```

- [ ] **Step 6: Run task tests**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml config_loads_tool_probe_names_from_env runtime_reports_configured_tool_capabilities
git diff --check
```

- [ ] **Step 7: Commit**

```bash
git add apps/runtime-agent/src/lib.rs apps/runtime-agent/src/tools.rs apps/runtime-agent/src/config.rs \
  apps/runtime-agent/src/daemon.rs apps/runtime-agent/tests/daemon_test.rs
git commit -m "feat: report runtime CLI tool capabilities"
```

---

### Task 5: Dependency Evaluation And Start Session Payload

**Files:**
- Modify: `apps/control-plane/internal/employee/run_service.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/run_service_test.go`
- Modify: `apps/control-plane/internal/skill/types.go`
- Modify: `apps/control-plane/internal/skill/service.go`
- Modify: `apps/control-plane/internal/skill/pg_repository.go`
- Modify: `apps/control-plane/internal/runtime/repository.go`

- [ ] **Step 1: Add failing run preflight tests**

In `apps/control-plane/internal/employee/run_service_test.go`, extend `fakeRunServiceRepository` with:

```go
runtimeSkills []skill.SkillRuntimeRecord
runtimeCapabilities []cpruntime.RuntimeCapability
runtimeEnv []EnvironmentVariableRecord
```

Add methods on the fake repository:

```go
func (f *fakeRunServiceRepository) ListSkillsForRuntime(context.Context, uuid.UUID, uuid.UUID) ([]skill.SkillRuntimeRecord, error) {
	return f.runtimeSkills, nil
}

func (f *fakeRunServiceRepository) ListRuntimeCapabilitiesForNode(context.Context, uuid.UUID, string) ([]cpruntime.RuntimeCapability, error) {
	return f.runtimeCapabilities, nil
}

func (f *fakeRunServiceRepository) ListRuntimeEnvironmentVariables(context.Context, uuid.UUID, uuid.UUID) ([]EnvironmentVariableRecord, error) {
	return f.runtimeEnv, nil
}
```

Then add:

```go
func TestRunServiceCreateRunRejectsSkillWithMissingToolDependency(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.runtimeSkills = []skill.SkillRuntimeRecord{{
		ID: uuid.New(), Slug: "github", ArchiveObjectRef: "s3://bucket/github.zip",
		ArchiveChecksum: strings.Repeat("a", 64), ArchiveSizeBytes: 10, ArchiveFileCount: 1,
		RuntimeDependencies: skill.SkillRuntimeDependencies{Tools: []string{"gh"}},
	}}
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())
	if err == nil || !strings.Contains(err.Error(), "gh") {
		t.Fatalf("expected missing gh dependency, got %v", err)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected missing dependency not to dispatch, got %#v", dispatcher.commands)
	}
}

func TestRunServiceStartSessionPayloadIncludesLoadableSkillsAndEnvironment(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.runtimeCapabilities = []cpruntime.RuntimeCapability{{
		CapabilityType: "tool",
		CapabilityKey: "gh",
		Available: true,
	}}
	repo.runtimeSkills = []skill.SkillRuntimeRecord{{
		ID: uuid.New(), Slug: "github", ArchiveObjectRef: "s3://bucket/github.zip",
		ArchiveChecksum: strings.Repeat("b", 64), ArchiveSizeBytes: 10, ArchiveFileCount: 1,
		RuntimeDependencies: skill.SkillRuntimeDependencies{Tools: []string{"gh"}, Env: []string{"GH_TOKEN"}},
	}}
	repo.runtimeEnv = []EnvironmentVariableRecord{{
		TenantID: repo.preflight.TenantID,
		TeamID: repo.preflight.TeamID,
		DigitalEmployeeID: repo.preflight.DigitalEmployeeID,
		Name: "GH_TOKEN",
		EncryptedValue: "encrypted",
		EncryptionKeyID: "v1",
		ValueFingerprint: "abc123",
		Sensitive: true,
		Status: EnvironmentVariableStatusActive,
	}}
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	run, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %#v", dispatcher.commands)
	}
	payload := commandPayload(t, dispatcher.commands[0].command)
	environment, ok := payload["environment"].([]any)
	if !ok || len(environment) != 1 {
		t.Fatalf("expected one env payload, got %#v", payload["environment"])
	}
	env := environment[0].(map[string]any)
	if env["name"] != "GH_TOKEN" || env["sensitive"] != true {
		t.Fatalf("unexpected env payload: %#v", env)
	}
	if got := payload["skills"].([]any); len(got) != 1 {
		t.Fatalf("expected one skill payload, got %#v", payload["skills"])
	}
	if run.CommandID == "" {
		t.Fatal("expected command id")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./apps/control-plane/internal/employee -run 'TestRunService(CreateRunRejectsSkillWithMissingToolDependency|StartSessionPayloadIncludesLoadableSkillsAndEnvironment)' -count=1
```

Expected: fail because dependency evaluation and env payload are missing.

- [ ] **Step 3: Extend runtime skill records**

In `apps/control-plane/internal/skill/types.go`, add `RuntimeDependencies` to `SkillRuntimeRecord`:

```go
RuntimeDependencies SkillRuntimeDependencies
```

Update repository queries that return runtime skills to scan dependencies from metadata.

- [ ] **Step 4: Add dependency evaluator**

In `apps/control-plane/internal/employee/run_service.go`, add:

```go
type SkillDependencyEvaluation struct {
	LoadStatus   string
	MissingTools []string
	MissingEnv   []string
}

func evaluateSkillDependencies(
	skill skill.SkillRuntimeRecord,
	availableTools map[string]struct{},
	availableEnv map[string]struct{},
) SkillDependencyEvaluation {
	var missingTools []string
	for _, tool := range skill.RuntimeDependencies.Tools {
		if _, ok := availableTools[tool]; !ok {
			missingTools = append(missingTools, tool)
		}
	}
	var missingEnv []string
	for _, name := range skill.RuntimeDependencies.Env {
		if _, ok := availableEnv[name]; !ok {
			missingEnv = append(missingEnv, name)
		}
	}
	status := "loadable"
	if len(missingTools) > 0 {
		status = "missing_tools"
	}
	if len(missingEnv) > 0 {
		status = "missing_env"
	}
	return SkillDependencyEvaluation{LoadStatus: status, MissingTools: missingTools, MissingEnv: missingEnv}
}
```

If both tools and env are missing, return `missing_tools` and include both arrays in the error details.

- [ ] **Step 5: Wire preflight**

Before `dispatchStartSession`, query:

- effective employee skills
- `ListRuntimeCapabilitiesForNode`
- active environment variables

Build:

```go
availableTools := map[string]struct{}{}
availableEnv := map[string]struct{}{}
```

For tools, include only capabilities where:

```go
cap.CapabilityType == "tool" && cap.Available
```

For env, include active encrypted env records.

If any active skill has missing deps, return an error with code/message:

```go
fmt.Errorf("%w: skill_dependencies_not_satisfied: %s", ErrInvalidInput, strings.Join(messages, "; "))
```

- [ ] **Step 6: Fill start session payload**

Change `buildStartSessionPayload` signature to accept:

```go
runtimeSkills []skill.SkillRuntimeRecord
runtimeEnv []RuntimeEnvironmentVariablePayload
```

Add:

```go
"skills": runtimeSkillsPayload(runtimeSkills),
"environment": runtimeEnvironmentPayload(runtimeEnv),
```

Define:

```go
type RuntimeEnvironmentVariablePayload struct {
	Name      string
	Value     string
	Sensitive bool
}
```

`runtimeEnvironmentPayload` returns `[]map[string]any` with `name`, `value`, and `sensitive`.

- [ ] **Step 7: Redaction check**

Ensure the command receipt/event payload redaction path redacts `"environment"` values. If redaction is centralized in `redactRuntimeEventPayload`, add:

```go
case "environment":
    redacted[key] = redactEnvironmentPayload(value)
```

Expected redacted shape:

```json
{"name":"GH_TOKEN","value":"[redacted]","sensitive":true}
```

- [ ] **Step 8: Run task tests**

```bash
go test ./apps/control-plane/internal/employee -run 'Skill|Environment|StartSessionPayload|CreateRun' -count=1
git diff --check
```

- [ ] **Step 9: Commit**

```bash
git add apps/control-plane/internal/employee/run_service.go apps/control-plane/internal/employee/service.go \
  apps/control-plane/internal/employee/run_service_test.go apps/control-plane/internal/skill/types.go \
  apps/control-plane/internal/skill/service.go apps/control-plane/internal/skill/pg_repository.go \
  apps/control-plane/internal/runtime/repository.go
git commit -m "feat: validate skill dependencies before employee runs"
```

---

### Task 6: Runtime Session Environment Injection

**Files:**
- Modify: `apps/runtime-agent/src/commands/payload.rs`
- Modify: `apps/runtime-agent/src/runs.rs`
- Modify: `apps/runtime-agent/src/providers/mod.rs`
- Modify: `apps/runtime-agent/src/providers/claude.rs`
- Modify: `apps/runtime-agent/src/providers/codex.rs`
- Modify: `apps/runtime-agent/src/providers/opencode.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_payload_test.rs`
- Modify: `apps/runtime-agent/tests/provider_command_test.rs`

- [ ] **Step 1: Add failing payload test**

In `apps/runtime-agent/tests/runtime_command_payload_test.rs`, add:

```rust
#[test]
fn parses_environment_variables_for_session_payload() {
    let mut payload = valid_payload();
    payload["environment"] = json!([
        {"name": "GH_TOKEN", "value": "plain-token", "sensitive": true}
    ]);

    let parsed = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect("valid environment payload");

    assert_eq!(parsed.environment.len(), 1);
    assert_eq!(parsed.environment[0].name, "GH_TOKEN");
    assert_eq!(parsed.environment[0].value, "plain-token");
    assert!(parsed.environment[0].sensitive);
}
```

- [ ] **Step 2: Add failing provider command test**

In `apps/runtime-agent/tests/provider_command_test.rs`, change helper `request` to include env:

```rust
environment: BTreeMap::from([("GH_TOKEN".to_string(), "plain-token".to_string())]),
```

Add:

```rust
#[test]
fn providers_inject_runtime_environment() {
    for command in [
        ClaudeProvider::new("claude").build_command(&request(None, false)),
        OpenCodeProvider::new("opencode").build_command(&request(None, false)),
        CodexProvider::new("codex").build_command(&request(None, false)),
    ] {
        let envs: std::collections::HashMap<_, _> = command
            .as_std()
            .get_envs()
            .filter_map(|(key, value)| {
                value.map(|value| (key.to_string_lossy().to_string(), value.to_string_lossy().to_string()))
            })
            .collect();
        assert_eq!(envs.get("GH_TOKEN").map(String::as_str), Some("plain-token"));
    }
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml parses_environment_variables_for_session_payload providers_inject_runtime_environment
```

Expected: compile failure because environment fields do not exist.

- [ ] **Step 4: Add payload type**

In `apps/runtime-agent/src/commands/payload.rs`, add:

```rust
#[derive(Debug, Clone, Deserialize, PartialEq, Eq)]
pub struct RuntimeEnvironmentVariablePayload {
    pub name: String,
    pub value: String,
    #[serde(default)]
    pub sensitive: bool,
}
```

Add to `RuntimeSessionCommandPayload`:

```rust
#[serde(default)]
pub environment: Vec<RuntimeEnvironmentVariablePayload>,
```

Validate env names:

```rust
fn valid_env_name(name: &str) -> bool {
    let mut chars = name.chars();
    match chars.next() {
        Some(c) if c == '_' || c.is_ascii_alphabetic() => {}
        _ => return false,
    }
    chars.all(|c| c == '_' || c.is_ascii_alphanumeric())
}
```

In `validate`, reject invalid names with `anyhow::bail!("invalid environment variable name: {}", env.name)`.

- [ ] **Step 5: Carry env through RunSpec and ProviderRequest**

In `apps/runtime-agent/src/runs.rs`:

```rust
pub environment: std::collections::BTreeMap<String, String>,
```

In `apps/runtime-agent/src/providers/mod.rs`:

```rust
pub environment: std::collections::BTreeMap<String, String>,
```

In `apps/runtime-agent/src/commands/executor.rs`, when building `RunSpec`:

```rust
environment: payload
    .environment
    .iter()
    .map(|env| (env.name.clone(), env.value.clone()))
    .collect(),
```

In `provider_request`:

```rust
environment: spec.environment.clone(),
```

- [ ] **Step 6: Inject env in provider adapters**

Add helper in `apps/runtime-agent/src/providers/mod.rs`:

```rust
pub fn apply_environment(command: &mut tokio::process::Command, request: &ProviderRequest) {
    for (name, value) in &request.environment {
        command.env(name, value);
    }
}
```

Call it in each adapter after `current_dir`:

```rust
crate::providers::apply_environment(&mut command, request);
```

- [ ] **Step 7: Run task tests**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml parses_environment_variables_for_session_payload providers_inject_runtime_environment
cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_command_payload_test provider_command_test
git diff --check
```

- [ ] **Step 8: Commit**

```bash
git add apps/runtime-agent/src/commands/payload.rs apps/runtime-agent/src/runs.rs \
  apps/runtime-agent/src/providers/mod.rs apps/runtime-agent/src/providers/claude.rs \
  apps/runtime-agent/src/providers/codex.rs apps/runtime-agent/src/providers/opencode.rs \
  apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/tests/runtime_command_payload_test.rs \
  apps/runtime-agent/tests/provider_command_test.rs
git commit -m "feat: inject employee environment into provider processes"
```

---

### Task 7: Web Skill Dependencies And Employee Environment UI

**Files:**
- Modify: `apps/web/src/lib/api/skills.ts`
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/features/skills/index.tsx`
- Modify: `apps/web/src/features/skills/index.test.tsx`
- Modify: `apps/web/src/features/employees/create.tsx`
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/detail.tsx`
- Modify: `apps/web/src/features/employees/components/employee-capabilities-panel.tsx`

- [ ] **Step 1: Read design rules before UI edits**

Run:

```bash
sed -n '1,220p' DESIGN.md
```

Apply existing SuperTeam dense admin UI patterns. Do not add a marketing page or decorative cards.

- [ ] **Step 2: Add API types first**

In `apps/web/src/lib/api/skills.ts`, add:

```ts
export type SkillRuntimeDependencies = {
  tools: string[];
  env: string[];
};
```

Add `runtime_dependencies: SkillRuntimeDependencies` to `Skill`, and `runtime_dependencies?: SkillRuntimeDependencies` to `UploadSkillInput`. In `uploadSkill`, send:

```ts
if (input.runtime_dependencies?.tools?.length) {
  formData.set("runtime_tools", input.runtime_dependencies.tools.join(","));
}
if (input.runtime_dependencies?.env?.length) {
  formData.set("runtime_env", input.runtime_dependencies.env.join(","));
}
```

In `apps/web/src/lib/api/employees.ts`, add:

```ts
export type DigitalEmployeeEnvironmentVariableSummary = {
  name: string;
  configured: boolean;
  fingerprint: string;
  sensitive: boolean;
  status: "active" | "disabled";
  updated_at?: string;
};

export type UpsertDigitalEmployeeEnvironmentVariableInput = {
  value: string;
  sensitive?: boolean;
};
```

Add `listEmployeeEnvironmentVariables`, `upsertEmployeeEnvironmentVariable`, and `deleteEmployeeEnvironmentVariable` using the routes from Task 2.

- [ ] **Step 3: Add skill upload form tests**

In `apps/web/src/features/skills/index.test.tsx`, add a test that uploads a skill with tools/env:

```tsx
it("submits runtime dependency fields when uploading a skill", async () => {
  const uploadSkill = vi.fn().mockResolvedValue(skillFactory({
    runtime_dependencies: { tools: ["gh"], env: ["GH_TOKEN"] },
  }));
  renderSkillsView({ uploadSkill });

  await user.click(screen.getByRole("button", { name: /上传技能/ }));
  await user.type(screen.getByLabelText("技能名称"), "GitHub Skill");
  await user.type(screen.getByLabelText("运行依赖 CLI"), "gh");
  await user.type(screen.getByLabelText("运行依赖环境变量"), "GH_TOKEN");
  await user.upload(screen.getByLabelText("技能 zip 包"), new File(["zip"], "skill.zip", { type: "application/zip" }));
  await user.click(screen.getByRole("button", { name: /^上传$/ }));

  expect(uploadSkill).toHaveBeenCalledWith(expect.anything(), expect.objectContaining({
    runtime_dependencies: { tools: ["gh"], env: ["GH_TOKEN"] },
  }));
});
```

- [ ] **Step 4: Implement skill UI**

In `SkillUploadDialog`, add state:

```ts
const [runtimeTools, setRuntimeTools] = useState("");
const [runtimeEnv, setRuntimeEnv] = useState("");
```

Add inputs:

```tsx
<div className="flex flex-col gap-2">
  <Label htmlFor="skill-runtime-tools">运行依赖 CLI</Label>
  <Input id="skill-runtime-tools" onChange={(event) => setRuntimeTools(event.target.value)} placeholder="gh,kubectl" value={runtimeTools} />
</div>
<div className="flex flex-col gap-2">
  <Label htmlFor="skill-runtime-env">运行依赖环境变量</Label>
  <Input id="skill-runtime-env" onChange={(event) => setRuntimeEnv(event.target.value)} placeholder="GH_TOKEN" value={runtimeEnv} />
</div>
```

Submit:

```ts
runtime_dependencies: {
  tools: splitDependencyInput(runtimeTools),
  env: splitDependencyInput(runtimeEnv),
},
```

Define:

```ts
function splitDependencyInput(value: string): string[] {
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}
```

- [ ] **Step 5: Add employee create env UI**

In `apps/web/src/features/employees/create.tsx`, add repeatable env rows with:

- name input
- value password input
- sensitive checkbox default checked
- remove button

Payload field:

```ts
environment_variables: envRows
  .filter((row) => row.name.trim() && row.value)
  .map((row) => ({ name: row.name.trim(), value: row.value, sensitive: row.sensitive })),
```

Add matching type field to `CreateDigitalEmployeeInput`.

- [ ] **Step 6: Add employee detail env summary**

In `apps/web/src/features/employees/components/employee-capabilities-panel.tsx`, add a compact environment-variable section near the personal skills section. If the component becomes hard to read, create a local child component in the same file named `EmployeeEnvironmentPanel`.

Render this table:

```text
名称 | 状态 | 指纹 | 更新时间 | 操作
GH_TOKEN | 已配置 | fp:a13f09c2 | 2026-06-22 10:00 | 替换 / 删除
```

Use password input for replacement. Never render plaintext from API responses.

- [ ] **Step 7: Add skill load status display**

Extend employee skills UI to render dependency status returned by backend:

```text
可加载
缺少 Runtime 工具：gh
缺少员工环境变量：GH_TOKEN
等待 Runtime 上报
```

Use a compact badge or inline status text consistent with existing dashboard style.

- [ ] **Step 8: Run Web tests**

```bash
corepack pnpm --filter ./apps/web run test -- --run apps/web/src/features/skills/index.test.tsx
corepack pnpm --filter ./apps/web run test -- --run apps/web/src/features/employees/create.test.tsx
git diff --check
```

Expected: tests pass.

- [ ] **Step 9: Commit**

```bash
git add apps/web/src/lib/api/skills.ts apps/web/src/lib/api/employees.ts \
  apps/web/src/features/skills/index.tsx apps/web/src/features/skills/index.test.tsx \
  apps/web/src/features/employees/create.tsx apps/web/src/features/employees/create.test.tsx \
  apps/web/src/features/employees/detail.tsx \
  apps/web/src/features/employees/components/employee-capabilities-panel.tsx
git commit -m "feat: manage skill dependencies and employee env in console"
```

---

### Task 8: Integration Verification, Changelog, And Real Smoke

**Files:**
- Modify: `CHANGELOG.md`
- Modify: any docs that need command examples after implementation, only if tests reveal missing operator guidance.

- [ ] **Step 1: Run focused test suites**

```bash
go test ./apps/control-plane/internal/storage -run 'EnvironmentVariables|SkillManagement' -count=1
go test ./apps/control-plane/internal/skill -count=1
go test ./apps/control-plane/internal/employee -run 'Environment|Skill|CreateRun|StartSession' -count=1
go test ./apps/control-plane/internal/api -run 'SkillRoutes|DigitalEmployeeEnvironmentVariableRoutes' -count=1
cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_command_payload_test provider_command_test daemon_test
corepack pnpm --filter ./apps/web run test -- --run apps/web/src/features/skills/index.test.tsx apps/web/src/features/employees/create.test.tsx
corepack pnpm verify:contracts
git diff --check
```

Expected: all pass.

- [ ] **Step 2: Run broader verification**

```bash
corepack pnpm test:go
corepack pnpm test:rust
corepack pnpm --filter ./apps/web run test
```

Expected: all pass. If unrelated dirty-worktree tests fail, capture the exact failing package/test and verify whether the failure is caused by this feature before proceeding.

- [ ] **Step 3: Run migration status or apply against intended dev DB**

Only run this when `DATABASE_URL` points to the intended local development database:

```bash
DATABASE_URL="$DATABASE_URL" make -C apps/control-plane migrate-status
DATABASE_URL="$DATABASE_URL" make -C apps/control-plane migrate-up
```

Expected: migration `032_digital_employee_env_and_skill_dependencies` applies and `digital_employee_environment_variables` exists.

- [ ] **Step 4: Real Runtime/Provider smoke**

Start current services:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart runtime-agent
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```

Smoke path:

1. Configure `SUPERTEAM_ENV_ENCRYPTION_KEYS` and `SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID` for Control Plane.
2. Configure Runtime with `RUNTIME_AGENT_TOOL_PROBE_NAMES=git,gh`.
3. Confirm `GET /api/v1/runtime/nodes/{nodeId}/capabilities` shows `capability_type=tool` for `gh`.
4. Upload a skill with `runtime_dependencies.tools=["gh"]` and `runtime_dependencies.env=["GH_TOKEN"]`.
5. Create or edit a digital employee with `GH_TOKEN`.
6. Bind the skill and confirm load status is `loadable`.
7. Launch a small run whose Provider command prints whether `GH_TOKEN` exists without printing its value, for example: `printf 'GH_TOKEN present: %s\n' "${GH_TOKEN:+yes}"`.
8. Confirm run logs and Runtime command receipts do not contain the token value.

Expected: Provider sees the env var, and logs/API responses do not leak plaintext.

- [ ] **Step 5: Add changelog entry**

Use exact timestamp command:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add one concise entry to `CHANGELOG.md` with that timestamp and the feature summary:

```markdown
- 2026-06-22 HH:MM: Added CLI runtime dependency checks for skills, Runtime tool capability reporting, and encrypted digital employee environment variable injection.
```

- [ ] **Step 6: Final completion check**

Read and apply the project completion skill:

```bash
sed -n '1,260p' .codex/skills/superteam-completion-check/SKILL.md
```

Run:

```bash
git status --short
git diff --check
```

Report verification using the required shape:

```text
真实链路验证：...
```

If real Runtime/Provider smoke cannot run because Provider credentials or service state are missing, report:

```text
阻塞：...；尚不能声明完成
```

- [ ] **Step 7: Commit final verification docs/changelog**

```bash
git add CHANGELOG.md
git commit -m "docs: note CLI skill runtime dependency support"
```

---

## Self-Review Checklist

- Spec coverage:
  - Skill `runtime_dependencies`: Task 3 and Task 7.
  - Runtime `tool` capability: Task 4.
  - Encrypted employee env storage and key management: Task 1 and Task 2.
  - Run preflight dependency validation and payload filling: Task 5.
  - Runtime Provider env injection: Task 6.
  - Console flows: Task 7.
  - Verification and real smoke: Task 8.
- Placeholder scan: no unresolved placeholder markers remain in the plan.
- Type consistency: use `runtime_dependencies`, `environment`, `EnvironmentVariableSummary`, `SkillRuntimeDependencies`, and `RuntimeEnvironmentVariablePayload` consistently across tasks.
- Scope check: project source-directory access, CLI installation UI, CLI version checks, MCP merge logic, and KMS/Vault integration stay outside this plan.
