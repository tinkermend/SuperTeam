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
- `apps/control-plane/internal/employee/run_service.go` - dependency evaluation, skill payload, env decrypt payload, and new `RuntimeSkillLister` / `RuntimeCapabilityLister` / `RuntimeEnvironmentLister` collaborator interfaces + setters.
- `apps/control-plane/internal/employee/run_repository.go` - no interface change required; the run service consumes the three listers above instead of growing `DigitalEmployeeRunRepository`.
- `apps/control-plane/internal/app/app.go` (`NewContainerWithConfig`) - construct the env `EnvironmentValueCodec` from `cfg.EmployeeEnv` (fail fast on bad/missing keys), inject it into the employee service, and wire the run-service lister setters (`SetSkillLister(skillService)` / `SetRuntimeCapabilityLister(runtimeService)` / `SetEnvironmentLister(employeeService)`).
- `apps/control-plane/internal/config/config.go` - new `EmployeeEnvConfig` section (`keys`, `activeKeyId`) on `Config`, env overlay, and startup validation.
- `apps/control-plane/config/config.example.yaml` - documented `employeeEnv:` template section (the real `config.yaml` is gitignored).
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

- [ ] **Step 6b: Load encryption keys from `config.yaml` (`employeeEnv` section)**

This project's convention is that configuration lives in `apps/control-plane/config/config.yaml` (gitignored; `config.example.yaml` is the committed template), loaded by `config.LoadFromFile` → `yaml.Unmarshal` into `Config`, then `applyEnv` overlays env-var overrides. The encryption keys follow the same pattern as `objectStore.secretAccessKey` and `planner.apiKey` — a yaml section, not env-only.

In `apps/control-plane/internal/config/config.go`:

- Add a section type and wire it onto `Config`:

```go
type EmployeeEnvConfig struct {
    Keys        string `yaml:"keys"`        // comma-separated "keyId:base64(32-byte-key)"
    ActiveKeyID string `yaml:"activeKeyId"` // keyId used to encrypt new values
}
```

```go
// inside type Config:
EmployeeEnv EmployeeEnvConfig `yaml:"employeeEnv"`
```

- In `applyEnv`, add env overrides so the yaml field can still be overridden by the legacy env var names (consistent with every other section):

```go
cfg.EmployeeEnv.Keys = envOrDefault("SUPERTEAM_ENV_ENCRYPTION_KEYS", cfg.EmployeeEnv.Keys)
cfg.EmployeeEnv.ActiveKeyID = envOrDefault("SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID", cfg.EmployeeEnv.ActiveKeyID)
```

- In `validate()`, fail startup with an actionable message when the section is incomplete (design §11: missing key ⇒ startup failure). The "active key id not in key list" check is enforced by `NewEnvironmentValueCodec` at boot (Task 2 wiring), so `validate()` only guards presence:

```go
if strings.TrimSpace(cfg.EmployeeEnv.Keys) == "" {
    return errors.New("employeeEnv.keys is required (set employeeEnv in config.yaml or SUPERTEAM_ENV_ENCRYPTION_KEYS)")
}
if strings.TrimSpace(cfg.EmployeeEnv.ActiveKeyID) == "" {
    return errors.New("employeeEnv.activeKeyId is required (set employeeEnv in config.yaml or SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID)")
}
```

In `apps/control-plane/config/config.example.yaml`, add the documented template (operators fill real values in their gitignored `config.yaml`):

```yaml
employeeEnv:
  # Digital-employee environment-variable encryption. Comma-separated
  # "keyId:base64(32-byte-key)" entries; activeKeyId selects the key used to
  # encrypt new values (all listed keys can decrypt, to support rotation).
  # Generate a key with:  openssl rand -base64 32
  # Keep config.yaml out of git (it already is). Env overrides:
  #   SUPERTEAM_ENV_ENCRYPTION_KEYS / SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID
  keys: "v1:replace-with-base64-32-byte-key"
  activeKeyId: "v1"
```

The codec stays env-agnostic: `NewEnvironmentValueCodec(EnvironmentValueCodecConfig{Keys: cfg.EmployeeEnv.Keys, ActiveKeyID: cfg.EmployeeEnv.ActiveKeyID})` is constructed in `app.go` (Task 2 wiring) and injected into the employee service. Do not read these keys directly from `os.Getenv` inside the employee package.

- [ ] **Step 7: Run task tests**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeEnvironmentVariablesMigration -count=1
go test ./apps/control-plane/internal/employee -run 'TestEnvironmentCrypto' -count=1
go test ./apps/control-plane/internal/config -count=1
git diff --check
```

Add a `config_test.go` case asserting `employeeEnv` parses from yaml and that `validate()` fails when `keys` or `activeKeyId` is missing. Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/032_digital_employee_env_and_skill_dependencies.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/migrations_test.go \
  apps/control-plane/internal/employee/env_crypto.go \
  apps/control-plane/internal/employee/env_service_test.go \
  apps/control-plane/internal/config/config.go apps/control-plane/internal/config/config_test.go \
  apps/control-plane/config/config.example.yaml
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
- Modify: `apps/control-plane/internal/app/app.go` (construct env codec from `cfg.EmployeeEnv`, inject via `employeeService.SetEnvironmentCodec`, save initial env vars in the create flow)
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

// InitialEnvironmentVariable is used by the digital-employee create flow to save
// env vars at create time. Plaintext lives only in the request; the service
// encrypts before any write.
type InitialEnvironmentVariable struct {
	Name      string
	Value     string
	Sensitive bool
}

// RuntimeEnvironmentVariablePayload is the decrypted shape handed to the run
// service for the Runtime command payload. Control Plane decrypts; Runtime
// Agent never sees ciphertext or keys.
type RuntimeEnvironmentVariablePayload struct {
	Name      string
	Value     string
	Sensitive bool
}
```

Extend `CreateDigitalEmployeeRequest` (in `employee/types.go`) with:

```go
EnvironmentVariables []InitialEnvironmentVariable
```

This field is the create-time entry point referenced by Task 5's create-flow wiring and by the Web create wizard (Task 7).

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

Add a runtime-decrypt method used by the run service (Task 5). It reads active encrypted records and decrypts through `EnvironmentValueCodec`, returning `[]RuntimeEnvironmentVariablePayload`. It must never log plaintext:

```go
func (s *Service) ListRuntimeEnvironmentVariablesForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeEnvironmentVariablePayload, error)
```

This method is the concrete implementation of the `RuntimeEnvironmentLister` interface consumed by `DigitalEmployeeRunService` in Task 5. Keep decryption inside the env service so the codec never leaks into the run service.

The env service methods need the codec. Add an `envCodec *EnvironmentValueCodec` field and a `SetEnvironmentCodec(*EnvironmentValueCodec)` setter onto `employee.Service` (the env methods live on `*Service`), mirroring the `SetRunService` / `SetAuthorizer` pattern. In `app.go` `NewContainerWithConfig`, construct the codec from the config loaded in Task 1 Step 6b and inject it, failing the container build on a codec error (bad/missing/unknown active key — design §11):

```go
envCodec, err := employee.NewEnvironmentValueCodec(employee.EnvironmentValueCodecConfig{
    Keys:        cfg.EmployeeEnv.Keys,
    ActiveKeyID: cfg.EmployeeEnv.ActiveKeyID,
})
if err != nil {
    return nil, fmt.Errorf("build env encryption codec: %w", err)
}
employeeService.SetEnvironmentCodec(envCodec)
```

`employeeService` is already constructed in `NewContainerWithConfig`; this is also where Task 5 wires the run-service listers.

- [ ] **Step 5b: Save initial env vars in the create flow**

The digital-employee create flow currently saves child records inside `s.repository.WithTransaction(...)` → `createLocalReadyEmployeeFacts` (`service.go`). Add a new step in that transaction that encrypts and persists each entry of `req.EnvironmentVariables` via the env repository's upsert path (`UpsertEnvironmentVariableStoreRequest`), using the same `EnvironmentValueCodec`, after the employee row exists so `digital_employee_id` is valid. The team id comes from the create request. If encryption fails, the whole create transaction must roll back (no half-written secrets). Normalize each name with `normalizeEnvName`; reject the create request with `ErrInvalidInput` on any invalid name.

Also parse the new field in `HTTPHandler.CreateDigitalEmployee` (`handler.go`) — the handler decodes an inline anonymous struct into `CreateDigitalEmployeeRequest`; add an `EnvironmentVariables []struct { Name, Value string; Sensitive bool }` field there and map it onto the request. Authz stays on the existing `authz.ActionEmployeeCreate` action for create.

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
- extend the create-employee request schema (the body of `POST /api/v1/digital-employees`) with an optional `environment_variables` array of `{ name, value, sensitive }` so the create-flow field from Step 5b is contract-covered. The `DigitalEmployee` detail response must NOT gain any env field; env summaries come only from the dedicated list endpoint.

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

- [ ] **Step 9: Run task tests**

```bash
go test ./apps/control-plane/internal/employee -run 'TestEnvironment|TestCreateDigitalEmployee' -count=1
go test ./apps/control-plane/internal/api -run TestDigitalEmployeeEnvironmentVariableRoutes -count=1
corepack pnpm verify:contracts
git diff --check
```

Expected: all pass. Add a service test asserting `CreateDigitalEmployee` with `EnvironmentVariables` persists exactly one encrypted row per entry (use a fake env repository in the employee service test) and rolls back when a name is invalid.

- [ ] **Step 10: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api/server.gen.go \
  apps/control-plane/internal/api/server.go apps/control-plane/internal/api/employee_routes_test.go \
  apps/control-plane/internal/employee/types.go apps/control-plane/internal/employee/repository.go \
  apps/control-plane/internal/employee/pg_repository.go apps/control-plane/internal/employee/env_repository.go \
  apps/control-plane/internal/employee/env_service.go apps/control-plane/internal/employee/env_service_test.go \
  apps/control-plane/internal/employee/handler.go apps/control-plane/internal/employee/service.go \
  apps/control-plane/internal/employee/service_test.go apps/control-plane/internal/app/app.go
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

Note: `Skill` currently has **no** `Metadata` field and `skillSelectColumns` (`skill/pg_repository.go:22`) does **not** include `s.metadata`. `RuntimeDependencies` is a typed projection over the JSONB `metadata` column, not a separate column. So also add a `Metadata map[string]any` field (or `json.RawMessage`) to `Skill` so the raw blob can be scanned; the service/repository layer unmarshals `metadata.runtime_dependencies` into `Skill.RuntimeDependencies` on every read (and back into `metadata` on write). The same applies to `SkillRuntimeRecord` (`skill/types.go:55`) because the run service reads dependencies via runtime skill records in Task 5.

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

- [ ] **Step 4: Persist and read dependencies via skills.metadata**

`skill/pg_repository.go` uses a hand-written column constant (`skillSelectColumns`) consumed by every skill SELECT and scan site. Changing the column list is therefore **not** a one-liner — every read path must be updated together:

- Add `s.metadata` to `skillSelectColumns`.
- Add a scan destination for the metadata column at every `Scan(...)` / row-mapping site that reads skills (list, get, runtime skills, agent bindings). Scan into `[]byte` then `json.Unmarshal` into `map[string]any` (guard NULL/empty with `COALESCE(s.metadata, '{}'::jsonb)` so existing rows scan cleanly).
- Populate `Skill.Metadata` from the unmarshaled map and project `Skill.RuntimeDependencies` by decoding the `runtime_dependencies` sub-key (tolerate missing key → empty `SkillRuntimeDependencies`). Do the same projection for `SkillRuntimeRecord` so Task 5 can read deps from runtime skill records.
- Marshal dependency metadata during `UpsertSkillPackage`: merge `runtime_dependencies` into the existing metadata map (never overwrite unrelated metadata keys), then write the merged JSONB. Update `UpsertSkillPackageParams` and the underlying query so the `metadata` column is included in INSERT and UPDATE. Preserve the column's existing `'{}'::jsonb` default for rows created without dependencies.
- The `ALTER TABLE skills ALTER COLUMN metadata SET DEFAULT '{}'::jsonb` from Task 1 is redundant (migration `009_skill_management.sql` already sets it) — keep it only as an idempotent no-op or drop it; do not rely on it.

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

### Task 4: Dynamic CLI Tool Capability Probing

The probe set is **dynamic and database-driven**, not a static config list. The Control Plane scans the database once to compute, per runtime node, the CLI tools required by the digital employees mounted on that node (their bound skills' `runtime_dependencies.tools`), injects that list into the node's heartbeat response, and the Runtime Agent periodically probes exactly that list on PATH. PATH is never scanned exhaustively; there is no global static probe list. (Operator can optionally keep a small always-probe baseline; it is additive, not authoritative.)

Depends on Task 3 (skills carry `runtime_dependencies`). Task 5's preflight consumes the `tool` capabilities reported here.

**Files (Control Plane):**
- Modify: `apps/control-plane/internal/storage/queries/runtime_events.sql` (+ generated `.sql.go`) — relax `ListRuntimeCapabilitiesForNode` from `capability_type='provider'` to `capability_type IN ('provider','tool')`. Latent bug today: tool caps are upserted by the agent but never returned by the list query, so preflight could not see them.
- Create query + service method `ListRequiredToolsForNode(tenantID, nodeID)` (Step A3) — one DB scan.
- Modify: `apps/control-plane/internal/runtime/service.go` (+ handler/models) — compute `required_tools` and include it in the heartbeat response.
- Modify: `apps/control-plane/internal/app/app.go` — wire the required-tools resolver into the runtime heartbeat path.
- Modify: `contracts/control-plane/openapi.yaml` — add `required_tools` to the heartbeat response schema.
- Tests: `apps/control-plane/internal/runtime/*_test.go`, `apps/control-plane/internal/api/runtime_routes_test.go`.

**Files (Runtime Agent):**
- Create: `apps/runtime-agent/src/tools.rs` — PATH probe helper.
- Modify: `apps/runtime-agent/src/controlplane/models.rs` — add `required_tools: Vec<String>` to `HeartbeatResponse`.
- Modify: `apps/runtime-agent/src/daemon.rs` — make `build_capabilities` `pub` and accept the probe set; in `heartbeat_loop` recompute from `baseline ∪ required_tools` and `upsert_capabilities` on change.
- Modify: `apps/runtime-agent/src/config.rs` — `tools.probe_names` becomes an OPTIONAL always-probe baseline (default empty); no longer authoritative.
- Modify: `apps/runtime-agent/tests/daemon_test.rs`.

#### Phase A — Control Plane: scan the DB for required tools and inject into heartbeat

- [ ] **Step A1: Write failing tests**

In `apps/control-plane/internal/runtime/` add a test (and an `api/runtime_routes_test.go` case) asserting:
- `ListRequiredToolsForNode(tenantID, nodeN)` returns the sorted, de-duped union of `runtime_dependencies.tools` over skills bound to DEs whose active execution instance is on node N. A DE on a different node (skill needs `helm`) does NOT contribute. A DE on node N with no skills / skills without deps contributes nothing.
- `ListRuntimeCapabilitiesForNode(...)` now returns rows with `capability_type='tool'` (it did not before the Step A4 filter change).

- [ ] **Step A2: Run, expect failure** — method/query absent; capability filter still provider-only.

- [ ] **Step A3: Implement `ListRequiredToolsForNode` as a single DB scan**

Add a sqlc query (e.g. `apps/control-plane/internal/storage/queries/skill.sql`) that scans the DB once — execution instances on the node → enabled skill bindings (agent + team, mirroring `ListSkillsForRuntime`) → `skills.metadata.runtime_dependencies.tools`:

```sql
-- name: ListRequiredToolsForNode :many
WITH mounted_skills AS (
    SELECT s.metadata
    FROM digital_employee_execution_instances dei
    JOIN skill_agent_bindings sab
      ON sab.tenant_id = dei.tenant_id
     AND sab.digital_employee_id = dei.digital_employee_id
     AND sab.status = 'enabled'
    JOIN skills s ON s.tenant_id = sab.tenant_id AND s.id = sab.skill_id
    WHERE dei.tenant_id = @tenant_id
      AND dei.runtime_node_id = @runtime_node_id
      AND dei.deleted_at IS NULL
      AND dei.status IN ('provisioning','ready','active')
    UNION
    SELECT s.metadata
    FROM digital_employee_execution_instances dei
    JOIN digital_employees de ON de.tenant_id = dei.tenant_id AND de.id = dei.digital_employee_id
    JOIN skill_team_bindings stb ON stb.tenant_id = de.tenant_id AND stb.team_id = de.team_id
    JOIN skills s ON s.tenant_id = stb.tenant_id AND s.id = stb.skill_id
    WHERE dei.tenant_id = @tenant_id
      AND dei.runtime_node_id = @runtime_node_id
      AND dei.deleted_at IS NULL
      AND dei.status IN ('provisioning','ready','active')
)
SELECT DISTINCT tool
FROM mounted_skills ms,
     LATERAL jsonb_array_elements_text(
        COALESCE(ms.metadata->'runtime_dependencies', '{}'::jsonb)->'tools'
     ) AS tool
ORDER BY tool;
```

Regenerate sqlc. Expose via `func (s *Service) ListRequiredToolsForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]string, error)` — resolve the agent string `nodeID` → `runtime_nodes.id` exactly the way `ListRuntimeCapabilitiesForNode` does (it joins on `rn.node_id = $node_id`).

- [ ] **Step A4: Relax the capability-list filter**

In `apps/control-plane/internal/storage/queries/runtime_events.sql`, change `ListRuntimeCapabilitiesForNode`'s `capability_type = 'provider'` to `capability_type IN ('provider', 'tool')`. Regenerate sqlc. This also surfaces `tool` capabilities on the node-detail UI (design §9.4).

- [ ] **Step A5: Inject `required_tools` into the heartbeat response**

The runtime heartbeat handler already resolves the node (tenant + node id). Add a collaborator interface on the runtime service and compute the list on each heartbeat:

```go
type RequiredToolsResolver interface {
    ListRequiredToolsForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]string, error)
}
```

Wire it via `SetRequiredToolsResolver` at `app.go` `NewContainerWithConfig` (point at the skill service where Step A3's method lives). Add `required_tools []string` to the heartbeat response model and to the OpenAPI heartbeat-response schema, then run `corepack pnpm generate:control-plane && corepack pnpm verify:contracts`.

- [ ] **Step A6: Run Phase A tests + commit**

```bash
go test ./apps/control-plane/internal/runtime -count=1
go test ./apps/control-plane/internal/api -run RuntimeRoutes -count=1
corepack pnpm verify:contracts
git diff --check
git add apps/control-plane/internal/storage/queries/skill.sql \
  apps/control-plane/internal/storage/queries/runtime_events.sql \
  apps/control-plane/internal/storage/queries/*.sql.go \
  apps/control-plane/internal/skill/service.go apps/control-plane/internal/runtime/service.go \
  apps/control-plane/internal/runtime/models.go apps/control-plane/internal/app/app.go \
  contracts/control-plane/openapi.yaml apps/control-plane/internal/api/server.gen.go \
  apps/control-plane/internal/runtime/*_test.go apps/control-plane/internal/api/runtime_routes_test.go
git commit -m "feat: compute per-node required CLI tools from mounted employees"
```

#### Phase B — Runtime Agent: periodically probe the injected list

- [ ] **Step B1: Write failing tests**

In `apps/runtime-agent/tests/daemon_test.rs`:

```rust
#[test]
fn config_probe_baseline_defaults_empty() {
    let cfg = RuntimeConfig::new("node-a").expect("config");
    assert!(cfg.tools.probe_names.is_empty(),
        "baseline must default to empty; the authoritative probe set is injected by the control plane");
}

#[tokio::test]
async fn build_capabilities_probes_required_tool_set() {
    let cfg = RuntimeConfig::new("node-a").expect("config");
    let caps = superteam_runtime_agent::daemon::build_capabilities(
        &cfg,
        &["definitely-missing-superteam-tool".to_string()],
    ).await;
    let tool = caps.iter()
        .find(|c| c.capability_type == "tool" && c.capability_key == "definitely-missing-superteam-tool")
        .expect("tool capability probed from required set");
    assert!(!tool.available);
    assert_eq!(tool.status, "missing");
}
```

- [ ] **Step B2: Run, expect failure** — `tools` section / new `build_capabilities` signature absent.

- [ ] **Step B3: Probe helper + optional baseline config**

Create `apps/runtime-agent/src/tools.rs` with `pub fn probe_tool(name: &str) -> ToolProbeResult { name, binary_path, available }` (PATH lookup only; never an exhaustive scan). In `config.rs` keep `tools.probe_names: Vec<String>` but **default it to `vec![]`**; document it as an optional always-probe baseline (env `RUNTIME_AGENT_TOOL_PROBE_NAMES` still overrides for operators who want a persistent baseline). It is no longer the authoritative source.

- [ ] **Step B4: Probe the injected list and re-report on heartbeat**

Make `build_capabilities` `pub` and accept the probe set explicitly (it is **not** `#[cfg(test)]` — integration tests in `tests/` would otherwise fail to link):

```rust
pub async fn build_capabilities(config: &RuntimeConfig, probe_tools: &[String]) -> Vec<RuntimeCapabilityInput>
```

`probe_tools` is the full set to probe (baseline ∪ required). Tool capabilities are emitted from it; provider/workspace/capability capabilities are unchanged. At enrollment call `build_capabilities(&cfg, &cfg.tools.probe_names)` (baseline only — required set is unknown until the first heartbeat).

Add `required_tools: Vec<String>` to `HeartbeatResponse` (`controlplane/models.rs`). In `heartbeat_loop` (`daemon.rs`): read `required_tools` from the response; when it differs from the last seen set, recompute `probe_tools = baseline ∪ required_tools`, call `build_capabilities(&cfg, &probe_tools)`, and `client.upsert_capabilities(&node_id, caps)`. Store the last seen required set on the daemon so subsequent equal heartbeats are a no-op.

- [ ] **Step B5: Run Phase B tests + commit**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml config_probe_baseline_defaults_empty build_capabilities_probes_required_tool_set
cargo test --manifest-path apps/runtime-agent/Cargo.toml daemon_test
git diff --check
git add apps/runtime-agent/src/tools.rs apps/runtime-agent/src/config.rs apps/runtime-agent/src/daemon.rs \
  apps/runtime-agent/src/controlplane/models.rs apps/runtime-agent/tests/daemon_test.rs
git commit -m "feat: probe required CLI tools injected via heartbeat"
```

Notes:
- Stale `tool` capabilities (a tool no longer required by any mounted employee) are harmless — they just record "tool X exists on this node" — so v1 does not GC them; the reported set always reflects the latest probe of `baseline ∪ required`.
- A freshly enrolled node has no required-tools until its first heartbeat (~30s); until then Task 5's preflight reports `pending_runtime`, which is the correct state (design §7.4).

---

### Task 5: Dependency Evaluation And Start Session Payload

**Files:**
- Modify: `apps/control-plane/internal/employee/run_service.go`
- Modify: `apps/control-plane/internal/employee/run_service_test.go`
- Modify: `apps/control-plane/internal/skill/types.go`
- Modify: `apps/control-plane/internal/skill/service.go`
- Modify: `apps/control-plane/internal/skill/pg_repository.go`
- Modify: `apps/control-plane/internal/app/app.go` (`NewContainerWithConfig`: wire the three run-service lister setters — `SetSkillLister(skillService)`, `SetRuntimeCapabilityLister(runtimeService)`, `SetEnvironmentLister(employeeService)`)
- No change: `apps/control-plane/internal/runtime/repository.go` (`ListRuntimeCapabilitiesForNode` already exists and is consumed via the new collaborator)

- [ ] **Step 1: Add lister interfaces, fake wiring, and failing run preflight tests**

The run service does **not** get these reads from its `DigitalEmployeeRunRepository`. `ListSkillsForRuntime` lives on `skill.PgRepository` / the existing `SkillLister` interface (`employee/service.go:20`), and `ListRuntimeCapabilitiesForNode` lives on `runtime.RuntimeCapabilityReadRepository` (`runtime/repository.go:60`). So introduce three collaborator interfaces on `DigitalEmployeeRunService` (Step 5) and wire them via setters, mirroring the existing `SetRunService` / `SetAuthorizer` pattern.

In `apps/control-plane/internal/employee/run_service_test.go`, extend `fakeRunServiceRepository` so it satisfies all three lister interfaces with one fake (the env lister returns the **decrypted** payload shape, since decryption stays in the env service in production):

```go
runtimeSkills []skill.SkillRuntimeRecord
runtimeCapabilities []cpruntime.RuntimeCapability
runtimeEnv []RuntimeEnvironmentVariablePayload
// An empty runtimeCapabilities slice models a node that has not reported any
// capability yet; the evaluator treats that as pending_runtime.
```

Add lister methods on the fake repository:

```go
func (f *fakeRunServiceRepository) ListSkillsForRuntime(context.Context, uuid.UUID, uuid.UUID) ([]skill.SkillRuntimeRecord, error) {
	return f.runtimeSkills, nil
}

func (f *fakeRunServiceRepository) ListRuntimeCapabilitiesForNode(context.Context, uuid.UUID, string) ([]cpruntime.RuntimeCapability, error) {
	return f.runtimeCapabilities, nil
}

func (f *fakeRunServiceRepository) ListRuntimeEnvironmentVariablesForRuntime(context.Context, uuid.UUID, uuid.UUID) ([]RuntimeEnvironmentVariablePayload, error) {
	return f.runtimeEnv, nil
}
```

Add a test helper that wires the fake as all three listers (the run service gains `SetSkillLister` / `SetRuntimeCapabilityLister` / `SetEnvironmentLister` setters in Step 5):

```go
func newRunServiceWithListers(t *testing.T, repo *fakeRunServiceRepository, dispatcher RuntimeCommandDispatcher) *DigitalEmployeeRunService {
	service := mustNewRunService(t, repo, dispatcher)
	service.SetSkillLister(repo)
	service.SetRuntimeCapabilityLister(repo)
	service.SetEnvironmentLister(repo)
	return service
}
```

Use `newRunServiceWithListers` (not bare `mustNewRunService`) in the two tests below so the dependency-evaluation path has non-nil listers. Existing tests that do not exercise dependency evaluation keep using `mustNewRunService` unchanged.

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
	repo.runtimeCapabilities = []cpruntime.RuntimeCapability{{
		CapabilityType: "tool",
		CapabilityKey:  "git",
		Available:      true,
	}} // node has reported capabilities (so not pending), but gh is missing
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := newRunServiceWithListers(t, repo, dispatcher)

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
	repo.runtimeEnv = []RuntimeEnvironmentVariablePayload{{
		Name: "GH_TOKEN",
		Value: "plain-token",
		Sensitive: true,
	}}
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := newRunServiceWithListers(t, repo, dispatcher)

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

func TestRunServiceCreateRunReportsPendingRuntimeWhenNodeHasNotReported(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.runtimeSkills = []skill.SkillRuntimeRecord{{
		ID: uuid.New(), Slug: "github", ArchiveObjectRef: "s3://bucket/github.zip",
		ArchiveChecksum: strings.Repeat("c", 64), ArchiveSizeBytes: 10, ArchiveFileCount: 1,
		RuntimeDependencies: skill.SkillRuntimeDependencies{Tools: []string{"gh"}},
	}}
	// runtimeCapabilities left empty: the node has not reported any capability yet.
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := newRunServiceWithListers(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())
	if err == nil || !strings.Contains(err.Error(), "pending_runtime") {
		t.Fatalf("expected pending_runtime failure, got %v", err)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected pending runtime not to dispatch, got %#v", dispatcher.commands)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./apps/control-plane/internal/employee -run 'TestRunService(CreateRunRejectsSkillWithMissingToolDependency|StartSessionPayloadIncludesLoadableSkillsAndEnvironment|CreateRunReportsPendingRuntimeWhenNodeHasNotReported)' -count=1
```

Expected: fail because dependency evaluation, pending_runtime handling, and the env payload are missing.

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

// evaluateSkillDependencies computes the load status of one skill.
//   - nodeReportedAnyCapability is true when the runtime node has reported at
//     least one capability of any kind. When false, the node is treated as
//     pending_runtime (capability reporting not yet received) — per design §7.4.
//   - Status priority is: pending_runtime > missing_tools > missing_env >
//     loadable. Both MissingTools and MissingEnv are always fully populated
//     regardless of the chosen status so the error/UI can list everything.
func evaluateSkillDependencies(
	skill skill.SkillRuntimeRecord,
	availableTools map[string]struct{},
	availableEnv map[string]struct{},
	nodeReportedAnyCapability bool,
) SkillDependencyEvaluation {
	missingTools := []string{}
	for _, tool := range skill.RuntimeDependencies.Tools {
		if _, ok := availableTools[tool]; !ok {
			missingTools = append(missingTools, tool)
		}
	}
	missingEnv := []string{}
	for _, name := range skill.RuntimeDependencies.Env {
		if _, ok := availableEnv[name]; !ok {
			missingEnv = append(missingEnv, name)
		}
	}

	status := "loadable"
	switch {
	case !nodeReportedAnyCapability && (len(missingTools) > 0 || len(missingEnv) > 0):
		status = "pending_runtime"
	case len(missingTools) > 0:
		status = "missing_tools"
	case len(missingEnv) > 0:
		status = "missing_env"
	}
	return SkillDependencyEvaluation{LoadStatus: status, MissingTools: missingTools, MissingEnv: missingEnv}
}
```

Status semantics match the design: `loadable` (deps satisfied), `missing_tools` (Runtime node lacks a required CLI), `missing_env` (employee lacks a required env var), `pending_runtime` (node has not reported capabilities yet so loadability cannot be decided). When both tools and env are missing and the node has reported, status is `missing_tools` and both arrays are returned. `pending_runtime` only applies when there is something missing AND the node has reported nothing — a node that reported but lacks the tool is `missing_tools`, not pending.

- [ ] **Step 5: Add lister collaborators and wire the preflight**

Add three collaborator interfaces near `DigitalEmployeeRunService` (the existing `SkillLister` in `employee/service.go:20` already covers the skill signature; define the other two here and reuse the skill one by signature match):

```go
type RuntimeSkillLister interface {
	ListSkillsForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]skill.SkillRuntimeRecord, error)
}
type RuntimeCapabilityLister interface {
	ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]cpruntime.RuntimeCapability, error)
}
type RuntimeEnvironmentLister interface {
	ListRuntimeEnvironmentVariablesForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeEnvironmentVariablePayload, error)
}
```

Add three fields to `DigitalEmployeeRunService` and three setters mirroring the existing `SetRunService` / `SetAuthorizer` pattern:

```go
func (s *DigitalEmployeeRunService) SetSkillLister(l RuntimeSkillLister)
func (s *DigitalEmployeeRunService) SetRuntimeCapabilityLister(l RuntimeCapabilityLister)
func (s *DigitalEmployeeRunService) SetEnvironmentLister(l RuntimeEnvironmentLister)
```

At the composition root — `app.go` `NewContainerWithConfig(stores, cfg)` (where `runService := employee.NewDigitalEmployeeRunService(...)` is already called) — call the setters with the real implementations already constructed in that function: `skillService`, `runtimeService`, and `employeeService` (which owns `ListRuntimeEnvironmentVariablesForRuntime` and the injected codec from Task 2). Concretely: `runService.SetSkillLister(skillService)`, `runService.SetRuntimeCapabilityLister(runtimeService)`, `runService.SetEnvironmentLister(employeeService)`. Do **not** add these methods to `DigitalEmployeeRunRepository`; the run service orchestrates the three collaborator interfaces instead.

Before `dispatchStartSession`, read through the listers (never `s.repository`):

```go
runtimeSkills, _ := s.skillLister.ListSkillsForRuntime(ctx, req.TenantID, req.DigitalEmployeeID)
capabilities, _ := s.capabilityLister.ListRuntimeCapabilitiesForNode(ctx, req.TenantID, preflight.RuntimeNodeID)
runtimeEnv, _ := s.envLister.ListRuntimeEnvironmentVariablesForRuntime(ctx, req.TenantID, req.DigitalEmployeeID)

availableTools := map[string]struct{}{}
for _, cap := range capabilities {
	if cap.CapabilityType == "tool" && cap.Available {
		availableTools[cap.CapabilityKey] = struct{}{}
	}
}
availableEnv := map[string]struct{}{}
for _, env := range runtimeEnv {
	availableEnv[env.Name] = struct{}{}
}
nodeReportedAnyCapability := len(capabilities) > 0
```

Evaluate each skill. If any active skill is not `loadable`, fail the run closed with a code that distinguishes the three failure shapes so the UI/API can route the user to the right fix:

```go
// status "missing_tools"  -> skill_dependencies_not_satisfied
// status "missing_env"    -> skill_dependencies_not_satisfied
// status "pending_runtime"-> skill_dependencies_pending_runtime
fmt.Errorf("%w: %s: %s", ErrInvalidInput, code, strings.Join(messages, "; "))
```

Each message names the skill and the missing tools/env (or "等待 Runtime 上报" for pending). Never include env values in the message. Pass `runtimeSkills` and `runtimeEnv` (decrypted) into `buildStartSessionPayload` (Step 6) only when all skills are loadable.

- [ ] **Step 6: Fill start session payload**

`buildStartSessionPayload` currently emits `"skills": emptyRuntimeSkillsPayload()` (`run_service.go:672`). Replace that with the real skills and add the environment block. Change the signature to accept:

```go
runtimeSkills []skill.SkillRuntimeRecord
runtimeEnv []RuntimeEnvironmentVariablePayload
```

(`RuntimeEnvironmentVariablePayload` is defined in Task 2; do not redefine it here.) Add:

```go
"skills":      runtimeSkillsPayload(runtimeSkills),
"environment": runtimeEnvironmentPayload(runtimeEnv),
```

`runtimeEnvironmentPayload` returns `[]map[string]any` with `name`, `value`, and `sensitive`. The values are already plaintext at this point (decrypted by the env lister in Step 5); Runtime Agent receives them as-is.

- [ ] **Step 7: Redact environment values in persisted runtime events**

Event/command redaction is centralized in `redactRuntimeEventPayload` (`pg_run_repository.go:523`), which today redacts keys like `token`/`secret` but would otherwise persist the dispatched `environment` plaintext. Add an explicit branch so the `environment` array is scrubbed before any runtime command event or provider session event is stored:

```go
case "environment":
    redacted[key] = redactEnvironmentPayload(value)
```

`redactEnvironmentPayload` walks each entry and replaces `value` with `"[redacted]"` while preserving `name` and `sensitive`. Expected redacted shape:

```json
{"name":"GH_TOKEN","value":"[redacted]","sensitive":true}
```

This covers the Control-Plane side. Runtime-Agent-side log/snapshot redaction is a separate requirement (Task 6, design §8.3) because Runtime Agent has no existing redaction layer.

- [ ] **Step 8: Run task tests**

```bash
go test ./apps/control-plane/internal/employee -run 'Skill|Environment|StartSessionPayload|CreateRun' -count=1
git diff --check
```

- [ ] **Step 9: Commit**

```bash
git add apps/control-plane/internal/employee/run_service.go \
  apps/control-plane/internal/employee/run_service_test.go apps/control-plane/internal/skill/types.go \
  apps/control-plane/internal/skill/service.go apps/control-plane/internal/skill/pg_repository.go \
  apps/control-plane/internal/app/app.go
git commit -m "feat: validate skill dependencies before employee runs"
```

(`runtime/repository.go` needs no change — `ListRuntimeCapabilitiesForNode` already exists there and is consumed via the new `RuntimeCapabilityLister` collaborator.)

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

In `apps/runtime-agent/src/runs.rs` (`RunSpec` derives `Serialize`/`Deserialize` and existing fields carry `#[serde(...)]` attributes, so the new field must default or older payloads/snapshots will fail to deserialize):

```rust
#[serde(default)]
pub environment: std::collections::BTreeMap<String, String>,
```

In `apps/runtime-agent/src/providers/mod.rs`:

```rust
#[serde(default)]
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

- [ ] **Step 7: Mask environment values in Runtime logs and snapshots**

Runtime Agent has **no existing redaction layer** (confirmed: no redact/mask/sanitiz logic outside path-traversal sanitization). Per design §8.3, plaintext env must never appear in Runtime logs, provider events, run snapshots, or error messages. Now that plaintext env flows through `RuntimeSessionCommandPayload` → `RunSpec` → `ProviderRequest`, add a focused redaction guard:

- `RunSnapshot` (`runs.rs:50`) must **not** gain an `environment` field — env lives only on `RunSpec`/`ProviderRequest` for the subprocess and is not persisted.
- Add a `redacted_environment_view` helper (e.g. in `runs.rs`) that returns a log-safe, serializable view of the payload/spec with every `environment[].value` replaced by `"[redacted]"` (keeping `name` and `sensitive`). Use it at every site that logs, traces, or serializes the command payload or `RunSpec`/`ProviderRequest` for diagnostics (search for `tracing::`, `dbg!`, `info!`, `debug!`, `serde_json::to_string` over these types).
- Ensure provider error/failure paths that surface command context go through the same redacted view (design §11: provider failure output must be env-redacted).

Add a test in `apps/runtime-agent/tests/runtime_command_payload_test.rs` asserting a redacted payload view does not contain the plaintext value:

```rust
#[test]
fn redacted_environment_view_hides_plaintext_values() {
    let mut payload = valid_payload();
    payload["environment"] = json!([
        {"name": "GH_TOKEN", "value": "plain-token", "sensitive": true}
    ]);
    let parsed = RuntimeSessionCommandPayload::from_command(&command(payload)).expect("valid");
    let view = superteam_runtime_agent::runs::redacted_environment_view(&parsed.environment);
    let rendered = serde_json::to_string(&view).unwrap();
    assert!(!rendered.contains("plain-token"));
    assert!(rendered.contains("GH_TOKEN"));
    assert!(rendered.contains("[redacted]"));
}
```

- [ ] **Step 8: Run task tests**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml parses_environment_variables_for_session_payload providers_inject_runtime_environment redacted_environment_view_hides_plaintext_values
cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_command_payload_test provider_command_test
git diff --check
```

- [ ] **Step 9: Commit**

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

Use the real helpers in `apps/web/src/features/skills/index.test.tsx`: `renderSkillsView(fetcher)` takes a **fetcher** (not an options object), there is **no** `skillFactory` (use the inline `skillsFixture`), and interaction is via `userEvent.*` directly (there is no `const user`). Mirror the existing upload test at `index.test.tsx:102-108`, which captures the upload `FormData` through the fetcher and asserts field names — extend that same capture to record `runtime_tools` and `runtime_env`:

```tsx
it("submits runtime dependency fields when uploading a skill", async () => {
  const captured: Record<string, string | null> = {};
  const fetcher = createSkillsFetcher();
  // Extend the existing upload-capture branch (the one that today asserts
  // formData.get("name") / "risk_level" / "file") to also record the new fields.
  fetcher.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/skills/uploads") && init?.body instanceof FormData) {
      captured.runtime_tools = (init.body.get("runtime_tools") as string | null) ?? null;
      captured.runtime_env = (init.body.get("runtime_env") as string | null) ?? null;
      return jsonResponse({ ...skillsFixture[0], runtime_dependencies: { tools: ["gh"], env: ["GH_TOKEN"] } });
    }
    return createSkillsFetcher()(input, init);
  });

  renderSkillsView(fetcher);

  await userEvent.click(await screen.findByRole("button", { name: /上传技能/ }));
  await userEvent.type(screen.getByLabelText("技能名称"), "GitHub Skill");
  await userEvent.type(screen.getByLabelText("运行依赖 CLI"), "gh");
  await userEvent.type(screen.getByLabelText("运行依赖环境变量"), "GH_TOKEN");
  await userEvent.upload(screen.getByLabelText("技能 zip 包"), new File(["zip"], "skill.zip", { type: "application/zip" }));
  await userEvent.click(screen.getByRole("button", { name: "上传" }));

  expect(captured.runtime_tools).toBe("gh");
  expect(captured.runtime_env).toBe("GH_TOKEN");
});
```

If the existing capture helper has a different shape than `fetcher.mockImplementation((input, init) => ...)`, adapt the capture to that shape but keep the two assertions on `runtime_tools` / `runtime_env`. Apply the same correction to the employee create test (`create.test.tsx`): no factory, `userEvent` direct, and assert the create payload includes `environment_variables`.

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

`apps/web/src/features/employees/create.tsx` is a **4-step wizard** (`身份` / `能力` / `治理` / `运行`), not a single form, and it assembles `CreateDigitalEmployeeInput` across steps. Add the repeatable env-var editor in the **`运行` step** (alongside runtime node / provider / session policy), and thread the collected rows into the final assembled `CreateDigitalEmployeeInput`. Each row has:

- name input
- value password input
- sensitive checkbox default checked
- remove button

Payload field (added to `CreateDigitalEmployeeInput` in `apps/web/src/lib/api/employees.ts`):

```ts
environment_variables: envRows
  .filter((row) => row.name.trim() && row.value)
  .map((row) => ({ name: row.name.trim(), value: row.value, sensitive: row.sensitive })),
```

Add the matching `environment_variables?: { name: string; value: string; sensitive: boolean }[]` field to the `CreateDigitalEmployeeInput` type. Use the wizard's existing step-state pattern (do not introduce a parallel form store).

- [ ] **Step 6: Add employee detail env summary**

`EmployeeCapabilitiesPanel` (`apps/web/src/features/employees/components/employee-capabilities-panel.tsx`) is currently **orphaned** — `detail.tsx` does not render it. Before adding env UI, mount the panel on the detail page: render `<EmployeeCapabilitiesPanel apiOptions={apiOptions} employeeId={employeeId} />` inside `EmployeeDetailView` (it fetches its own data by `employeeId`). Then add a compact environment-variable section to the panel near the existing 个人技能 section. If the panel becomes hard to read, create a local child component in the same file named `EmployeeEnvironmentPanel`.

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
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/skills/index.test.tsx
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/employees/create.test.tsx
git diff --check
```

(`vitest-run.mjs` already forces run mode, so `--run` is redundant; the repo convention is `run test -- <file>`.) Expected: tests pass.

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
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/skills/index.test.tsx apps/web/src/features/employees/create.test.tsx
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

1. Configure env encryption in `apps/control-plane/config/config.yaml` under the `employeeEnv:` section (`keys: "v1:<base64-32-byte-key>"`, `activeKeyId: "v1"`); generate the key with `openssl rand -base64 32`. (Env overrides `SUPERTEAM_ENV_ENCRYPTION_KEYS` / `SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID` still work but yaml is the canonical source.)
2. Do NOT set a static tool list — leave `RUNTIME_AGENT_TOOL_PROBE_NAMES` unset (baseline defaults empty). Instead assign a digital employee to the node whose bound skill declares `runtime_dependencies.tools=["gh"]`; the Control Plane injects `gh` into the heartbeat response and the Runtime Agent probes it within one heartbeat (~30s).
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
  - Create-time env saving (Step 5b): Task 2.
  - Run preflight dependency validation, `pending_runtime`, and payload filling: Task 5.
  - Runtime Provider env injection **and** Runtime-side redaction (design §8.3): Task 6.
  - Console flows (incl. mounting the previously-orphaned capabilities panel): Task 7.
  - Verification and real smoke: Task 8.
- Codebase alignment (verified before revision):
  - The run service reads skills/capabilities/env via three **collaborator interfaces** wired with setters (`RuntimeSkillLister` / `RuntimeCapabilityLister` / `RuntimeEnvironmentLister`) — these methods are **not** added to `DigitalEmployeeRunRepository`. `ListSkillsForRuntime` is reused from `skill.PgRepository`/`SkillLister`; `ListRuntimeCapabilitiesForNode` is reused from `runtime.RuntimeCapabilityReadRepository`.
  - Composition root is `app.go` `NewContainerWithConfig(stores, cfg)` — this is where the env codec is built from `cfg.EmployeeEnv` and injected into `employeeService`, and where the three run-service lister setters are called.
  - Tool probing is **dynamic**, not a static config list: CP scans the DB once (`ListRequiredToolsForNode`) for the CLI tools required by skills bound to the DEs mounted on the node, injects the list into the heartbeat response, and the agent re-probes `baseline ∪ required` each heartbeat and re-upserts. `tools.probe_names` is an optional always-probe baseline only (default empty). `ListRuntimeCapabilitiesForNode` is relaxed to include `capability_type='tool'` (it was provider-only — a latent bug that would have hidden all tool caps from preflight).
  - Encryption keys live in `config.yaml` as an `employeeEnv:` section (`keys`, `activeKeyId`), loaded via `config.LoadFromFile`/`applyEnv` exactly like `objectStore`/`planner`; `SUPERTEAM_ENV_ENCRYPTION_KEYS` / `SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID` remain as env overrides, not the canonical source. Startup fails if the section is missing or the active key is unknown.
  - Skill dependencies are a projection over the JSONB `metadata` column: `Skill`/`SkillRuntimeRecord` gain a `Metadata` field and `RuntimeDependencies`; every scan site and `UpsertSkillPackage` are updated (Task 3 Step 4). The `ALTER TABLE ... metadata SET DEFAULT` is redundant vs migration `009`.
  - `build_capabilities` is made `pub` (not `#[cfg(test)]`) so integration tests in `tests/` can call it (Task 4 Step 5).
  - New `RunSpec`/`ProviderRequest` `environment` fields carry `#[serde(default)]`; `RunSnapshot` gains no env field.
  - `RuntimeEnvironmentVariablePayload` is defined once (Task 2) and reused by the env service, run service, and Runtime payload.
- Placeholder scan: no unresolved placeholders remain — the earlier `<composition root>` marker is resolved to `apps/control-plane/internal/app/app.go` (`NewContainerWithConfig`).
- Type consistency: use `runtime_dependencies`, `environment`, `EnvironmentVariableSummary`, `SkillRuntimeDependencies`, and `RuntimeEnvironmentVariablePayload` consistently across tasks.
- Scope check: project source-directory access, CLI installation UI, CLI version checks, MCP merge logic, and KMS/Vault integration stay outside this plan.
