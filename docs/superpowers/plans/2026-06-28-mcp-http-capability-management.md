# MCP HTTP Capability Management Implementation Plan
> 复核状态：已实现（mig 037 + registry/binding API + /mcp 页 E2E 验证）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build first-class MCP HTTP capability management so admins can register MCP endpoints once, bind them to teams or digital employees, enforce employee environment-variable requirements, and project the effective config into Codex, Claude Code, and OpenCode workspaces without running MCP servers on Runtime nodes.

**Architecture:** Control Plane is the source of truth for MCP definitions, bindings, credentials, required environment variables, and config revisions. Runtime Agent only materializes provider-specific config files from the effective employee payload using atomic writes; active provider sessions are treated as using the config they started with, so binding changes apply on the next provision/start unless a future provider-specific reload is added. Web adds a first-level MCP management surface and keeps team/employee pages as binding helper views.

**Tech Stack:** Go Control Plane, PostgreSQL migrations and sqlc, OpenAPI contract, Rust Runtime Agent, React/TanStack Router/Query Web console, `corepack pnpm` verification scripts.

---

## Existing Surfaces To Reuse

- `apps/control-plane/internal/storage/migrations/018_dual_layer_capability_management.sql` already creates `user_credentials`, `team_mcp_servers`, and `digital_employee_mcp_bindings`.
- `apps/control-plane/internal/capability/*` already handles user credentials, team MCP servers, employee MCP bindings, and effective MCP listing.
- `contracts/control-plane/openapi.yaml` already exposes team MCP, employee MCP, effective MCP, and employee environment-variable endpoints.
- `apps/web/src/lib/api/capabilities.ts` already wraps current credential and MCP binding APIs.
- `apps/web/src/features/teams/components/team-capabilities-tab.tsx` already has a team-local "公共 MCP" management panel.
- `apps/web/src/features/employees/components/employee-capabilities-panel.tsx` already uses employee MCP binding APIs.
- `apps/runtime-agent/src/commands/payload.rs` already contains `RuntimeMCPServerPayload`, and provision/start payloads already include `mcp_servers`, but Runtime does not yet materialize provider config.

## Target File Structure

### Control Plane

- Modify `apps/control-plane/internal/storage/migrations/037_mcp_http_capability_registry.sql`
  - Add the new tenant MCP registry and binding tables, keeping existing team/employee MCP tables readable during migration.
- Modify `apps/control-plane/internal/storage/queries/capability.sql`
  - Add sqlc queries for MCP registry CRUD, binding CRUD, required-env checks, and effective MCP payloads.
- Modify generated `apps/control-plane/internal/storage/queries/capability.sql.go` and `querier.go`
  - Regenerate if `sqlc` is available. If not, update generated code manually in the existing style and call that out in the final notes.
- Modify `apps/control-plane/internal/capability/types.go`
  - Add `MCPDefinition`, `MCPBinding`, `MCPBindingPreflight`, and provider projection fields.
- Modify `apps/control-plane/internal/capability/service.go`
  - Add registry validation, HTTP-only enforcement, required env-var checks, binding status calculation, and effective MCP resolution.
- Modify `apps/control-plane/internal/capability/pg_repository.go`
  - Implement repository methods for the new queries.
- Modify `apps/control-plane/internal/capability/handler.go`
  - Add tenant-level MCP definition routes and update binding routes to bind by `mcp_server_id`.
- Modify `apps/control-plane/internal/api/server.go`
  - Register new `/api/v1/mcp-servers` routes.
- Modify `contracts/control-plane/openapi.yaml`
  - Add schemas and endpoints for MCP registry, binding preflight, and effective provider config.
- Modify `apps/control-plane/internal/employee/run_service.go`
  - Replace `emptyRuntimeMCPServersPayload()` with effective MCP servers from capability service or repository.
- Modify `apps/control-plane/internal/employee/service.go`
  - Ensure provision-instance payload includes effective MCP config and does not invent local MCP processes.

### Runtime Agent

- Modify `apps/runtime-agent/src/commands/payload.rs`
  - Expand `RuntimeMCPServerPayload` with URL, required env vars, auth env var names, headers-by-env, source scope, and status.
- Create `apps/runtime-agent/src/mcp_config.rs`
  - Provider-specific config renderers for Codex, Claude Code, and OpenCode.
- Modify `apps/runtime-agent/src/commands/executor.rs`
  - Materialize MCP config during `provision_instance` and `sync_workspace_files` before command completion.
- Modify `apps/runtime-agent/src/workspace_files.rs`
  - Reuse or expose atomic write helpers for provider config files.
- Modify `apps/runtime-agent/src/lib.rs`
  - Export `mcp_config`.

### Web Console

- Modify `apps/web/src/components/layout/data/sidebar-data.ts`
  - Add first-level MCP menu item under "核心导航" or rename "外部能力" to a real capability hub with MCP as the first implemented tab.
- Create `apps/web/src/routes/_authenticated/mcp/index.tsx`
  - Route to the MCP management page.
- Create `apps/web/src/features/mcp/index.tsx`
  - Registry table, create/edit drawer, bind-to-team and bind-to-employee actions, env-var requirement status, credential status.
- Create `apps/web/src/features/mcp/index.test.tsx`
  - UI tests for list/create/bind/preflight/delete.
- Modify `apps/web/src/lib/api/capabilities.ts`
  - Add typed API wrappers for MCP registry and binding preflight.
- Modify `apps/web/src/features/teams/components/team-capabilities-tab.tsx`
  - Replace direct name+URL creation with selecting registered MCP entries.
- Modify `apps/web/src/features/employees/components/employee-capabilities-panel.tsx`
  - Show inherited vs personal MCP, missing env vars, and links to set employee environment variables.

---

## Database Model

Use the existing dual-layer tables as the starting point but move the product model to registry-plus-bindings.

New canonical tables:

```sql
CREATE TABLE IF NOT EXISTS mcp_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name TEXT NOT NULL,
    server_key TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    transport VARCHAR(40) NOT NULL DEFAULT 'streamable_http',
    url TEXT NOT NULL,
    auth_strategy VARCHAR(40) NOT NULL DEFAULT 'none',
    required_env_vars TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    optional_env_vars TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    provider_visibility JSONB NOT NULL DEFAULT '{"codex":true,"claude-code":true,"opencode":true}'::jsonb,
    tool_allowlist TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    risk_level VARCHAR(40) NOT NULL DEFAULT 'medium',
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_mcp_servers_transport_http_only
        CHECK (transport IN ('streamable_http', 'http')),
    CONSTRAINT ck_mcp_servers_auth_strategy
        CHECK (auth_strategy IN ('none', 'bearer_env', 'headers_env'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_mcp_servers_tenant_key_active
    ON mcp_servers(tenant_id, server_key)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS team_mcp_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    mcp_server_id UUID NOT NULL,
    credential_env_var TEXT,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_team_mcp_bindings_active
    ON team_mcp_bindings(tenant_id, team_id, mcp_server_id)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS digital_employee_mcp_bindings_v2 (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    mcp_server_id UUID NOT NULL,
    credential_env_var TEXT,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_employee_mcp_bindings_v2_active
    ON digital_employee_mcp_bindings_v2(tenant_id, digital_employee_id, mcp_server_id)
    WHERE deleted_at IS NULL;
```

Keep old `team_mcp_servers` and `digital_employee_mcp_bindings` until all routes and UI are migrated. The migration should backfill unique old rows into `mcp_servers` and bindings, deriving `server_key` from a normalized name plus a short hash of the URL.

**Decommission follow-up (must be scheduled, not left open-ended):** after Tasks 2–6 land and the real smoke in Task 7 passes, a later migration must drop `team_mcp_servers` and `digital_employee_mcp_bindings` and rename `digital_employee_mcp_bindings_v2` to a permanent name (e.g. `digital_employee_mcp_bindings`). Do not leave the `_v2` suffix as the canonical table and do not leave the legacy tables in place indefinitely — that is permanent duplicate schema. If the rename cannot happen in this effort, record an explicit decision that `_v2` is the permanent name and say why in the final report.

---

## Runtime Projection Rules

The effective employee MCP payload should include only active bindings whose required env vars are present on that employee. Blocked bindings remain visible in Console as `blocked_missing_env`, but Runtime payload excludes them.

Payload shape:

```json
{
  "server_id": "uuid",
  "server_key": "github",
  "name": "GitHub MCP",
  "transport": "streamable_http",
  "url": "https://api.githubcopilot.com/mcp/",
  "auth_strategy": "bearer_env",
  "credential_env_var": "GITHUB_TOKEN",
  "required_env_vars": ["GITHUB_TOKEN"],
  "headers_env": {},
  "source_scope": "team",
  "permission_scope": {
    "tool_allowlist": ["repos.search", "issues.create"]
  }
}
```

Provider files:

- Codex: write under the employee `agent_home_dir` using the repo's Codex home convention. `config.toml` also holds non-MCP Codex settings, so do **not** rewrite the whole file. Parse the existing `config.toml` (if present), replace only the `mcp_servers` table, and preserve every unrelated key — the same read-merge-preserve discipline used for OpenCode. Render MCP servers as remote `mcp_servers.<server_key>` entries with `url`, `bearer_token_env_var`, or `env_http_headers`.
- Claude Code: write project-level `.mcp.json` with HTTP MCP server definitions. Treat this as next-run effective; do not claim hot reload.
- OpenCode: write `opencode.json` `mcp` entries and preserve unrelated existing keys if the file already exists.

All writes must use temp-file plus atomic rename and must reject paths outside `agent_home_dir`.

---

## Tasks

### Task 1: Contract And Database Registry

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/037_mcp_http_capability_registry.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/queries/capability.sql`
- Modify: `contracts/control-plane/openapi.yaml`
- Modify if generated: `apps/control-plane/internal/storage/queries/capability.sql.go`
- Modify if generated: `apps/control-plane/internal/storage/queries/querier.go`
- Modify if generated: `apps/control-plane/internal/api/gen/control_plane.gen.go`

- [ ] **Step 1: Write migration coverage test**

Add assertions to `apps/control-plane/internal/storage/migrations_test.go` for migration `037_mcp_http_capability_registry.sql`:

```go
func TestMCPHTTPCapabilityRegistryMigration(t *testing.T) {
    body, err := os.ReadFile("migrations/037_mcp_http_capability_registry.sql")
    if err != nil {
        t.Fatalf("read migration 037: %v", err)
    }
    sql := string(body)

    assertMigrationContains(t, sql, "CREATE TABLE IF NOT EXISTS mcp_servers")
    assertMigrationContains(t, sql, "CREATE TABLE IF NOT EXISTS team_mcp_bindings")
    assertMigrationContains(t, sql, "CREATE TABLE IF NOT EXISTS digital_employee_mcp_bindings_v2")
    assertMigrationContains(t, sql, "CHECK (transport IN ('streamable_http', 'http'))")
    assertMigrationContains(t, sql, "required_env_vars TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[]")
    assertMigrationContains(t, sql, "COMMENT ON TABLE mcp_servers IS")
}
```

> **Note:** Migration tests in this repo read files directly with `os.ReadFile("migrations/<file>.sql")` and assert with the existing `assertMigrationContains` helper. There is no `readMigrationForTest` helper — do not invent one.

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestMCPHTTPCapabilityRegistryMigration -count=1
```

Expected: FAIL because the migration does not exist yet.

- [ ] **Step 2: Add migration**

Create `apps/control-plane/internal/storage/migrations/037_mcp_http_capability_registry.sql` with the canonical tables above, updated-at triggers, indexes, comments, and a backfill block:

```sql
INSERT INTO mcp_servers (
    tenant_id,
    name,
    server_key,
    url,
    auth_strategy,
    status,
    metadata,
    created_by,
    created_at,
    updated_at
)
SELECT DISTINCT ON (tenant_id, name, url)
    tenant_id,
    name,
    lower(regexp_replace(name, '[^a-zA-Z0-9]+', '-', 'g')) || '-' || substr(md5(url), 1, 8),
    url,
    CASE WHEN credential_id IS NULL THEN 'none' ELSE 'bearer_env' END,
    status,
    metadata || jsonb_build_object('legacy_source', 'team_mcp_servers'),
    created_by,
    created_at,
    updated_at
FROM team_mcp_servers
WHERE deleted_at IS NULL
ON CONFLICT DO NOTHING;
```

Add equivalent backfill for old employee MCP rows and binding inserts. When old rows have `credential_id`, set `credential_env_var` to `MCP_TOKEN_` plus an uppercased sanitized server key as a temporary migration value; later tasks will let users replace it with a real employee env var.

For any backfilled `mcp_servers` row whose `auth_strategy` is `bearer_env`, also populate `required_env_vars = ARRAY[<credential_env_var>]` (the same `MCP_TOKEN_<KEY>` value). A `bearer_env` server with an empty `required_env_vars` would make the env-var preflight treat migrated servers as needing nothing, silently bypassing the headline gate — keep the auth strategy and required env vars consistent.

- [ ] **Step 2b: Re-hash the Atlas migration directory**

Migrations are Atlas-managed: `make migrate-up` runs `atlas migrate apply`, which validates every file against `internal/storage/migrations/atlas.sum`. A new `037_*.sql` file without a matching checksum entry makes `atlas migrate` abort with a checksum-mismatch error. After writing the migration, regenerate the hash and commit `atlas.sum` alongside the migration:

```bash
atlas migrate hash --dir file://apps/control-plane/internal/storage/migrations
```

Expected: `atlas.sum` gains an entry for `037_mcp_http_capability_registry.sql`. If `atlas` is not installed locally, stop and report blocked — do not hand-edit `atlas.sum`.

- [ ] **Step 3: Add sqlc query tests before implementation**

Add a repository test in `apps/control-plane/internal/capability/pg_repository_test.go` that creates a registry MCP server with required env vars and verifies list output includes them.

Run:

```bash
go test ./apps/control-plane/internal/capability -run TestPgRepositoryMCPRegistry -count=1
```

Expected: FAIL because query methods do not exist.

- [ ] **Step 4: Add sqlc queries**

Append to `apps/control-plane/internal/storage/queries/capability.sql`:

```sql
-- name: CreateMCPServerDefinition :one
INSERT INTO mcp_servers (
    tenant_id,
    name,
    server_key,
    description,
    transport,
    url,
    auth_strategy,
    required_env_vars,
    optional_env_vars,
    provider_visibility,
    tool_allowlist,
    risk_level,
    metadata,
    created_by
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('name')::text,
    sqlc.arg('server_key')::text,
    COALESCE(sqlc.arg('description')::text, ''),
    sqlc.arg('transport')::varchar,
    sqlc.arg('url')::text,
    sqlc.arg('auth_strategy')::varchar,
    COALESCE(sqlc.arg('required_env_vars')::text[], ARRAY[]::TEXT[]),
    COALESCE(sqlc.arg('optional_env_vars')::text[], ARRAY[]::TEXT[]),
    COALESCE(sqlc.arg('provider_visibility')::jsonb, '{"codex":true,"claude-code":true,"opencode":true}'::jsonb),
    COALESCE(sqlc.arg('tool_allowlist')::text[], ARRAY[]::TEXT[]),
    COALESCE(sqlc.arg('risk_level')::varchar, 'medium'),
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
)
RETURNING *;

-- name: ListMCPServerDefinitions :many
SELECT *
FROM mcp_servers
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
ORDER BY created_at DESC, name ASC;

-- name: GetMCPServerDefinition :one
SELECT *
FROM mcp_servers
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: DeleteMCPServerDefinition :exec
UPDATE mcp_servers
SET deleted_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;
```

Add team and employee binding CRUD queries against `team_mcp_bindings` and `digital_employee_mcp_bindings_v2`, plus an effective query that joins employee env-var names from `digital_employee_environment_variables` and returns missing required env vars.

- [ ] **Step 5: Regenerate and verify**

Run:

```bash
cd apps/control-plane && make generate-sqlc
corepack pnpm verify:contracts
```

Expected: sqlc and OpenAPI generated files update cleanly. If `sqlc` is missing, manually update generated Go code in the existing style and document that in the final report.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/037_mcp_http_capability_registry.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/migrations_test.go \
  apps/control-plane/internal/storage/queries/capability.sql \
  apps/control-plane/internal/storage/queries/capability.sql.go \
  apps/control-plane/internal/storage/queries/querier.go \
  contracts/control-plane/openapi.yaml \
  apps/control-plane/internal/api/gen/control_plane.gen.go
git commit -m "feat: add mcp http capability registry schema"
```

### Task 2: Control Plane MCP Registry And Binding Service

**Files:**
- Modify: `apps/control-plane/internal/capability/types.go`
- Modify: `apps/control-plane/internal/capability/service.go`
- Modify: `apps/control-plane/internal/capability/pg_repository.go`
- Modify: `apps/control-plane/internal/capability/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify tests: `apps/control-plane/internal/capability/service_test.go`
- Modify tests: `apps/control-plane/internal/capability/handler_test.go`
- Modify tests: `apps/control-plane/internal/api/team_routes_test.go`
- Modify tests: `apps/control-plane/internal/api/employee_routes_test.go`

- [ ] **Step 1: Write service tests for HTTP-only registry validation**

Add to `apps/control-plane/internal/capability/service_test.go`:

```go
func TestServiceCreateMCPServerDefinitionValidatesHTTPOnlyAndEnvVars(t *testing.T) {
    tenantID := uuid.New()
    userID := uuid.New()
    repo := &serviceRepo{}
    svc := NewService(repo, nil)

    _, err := svc.CreateMCPServerDefinition(context.Background(), CreateMCPServerDefinitionRequest{
        TenantID:        tenantID,
        UserID:          userID,
        Name:            "Local Stdio MCP",
        ServerKey:       "local-stdio",
        Transport:       "stdio",
        URL:             "file:///tmp/mcp",
        AuthStrategy:    MCPAuthStrategyNone,
        RequiredEnvVars: []string{"BAD-NAME"},
    })

    if err == nil || !strings.Contains(err.Error(), "transport must be http") {
        t.Fatalf("expected http-only validation error, got %v", err)
    }
}
```

Run:

```bash
go test ./apps/control-plane/internal/capability -run TestServiceCreateMCPServerDefinitionValidatesHTTPOnlyAndEnvVars -count=1
```

Expected: FAIL because service method and types are not defined.

- [ ] **Step 2: Implement domain types**

Add explicit types:

```go
type MCPTransport string

const (
    MCPTransportStreamableHTTP MCPTransport = "streamable_http"
    MCPTransportHTTP           MCPTransport = "http"
)

type MCPAuthStrategy string

const (
    MCPAuthStrategyNone       MCPAuthStrategy = "none"
    MCPAuthStrategyBearerEnv  MCPAuthStrategy = "bearer_env"
    MCPAuthStrategyHeadersEnv MCPAuthStrategy = "headers_env"
)

type MCPDefinition struct {
    ID                 uuid.UUID
    TenantID           uuid.UUID
    Name               string
    ServerKey          string
    Description        string
    Transport          MCPTransport
    URL                string
    AuthStrategy       MCPAuthStrategy
    RequiredEnvVars    []string
    OptionalEnvVars    []string
    ProviderVisibility map[string]bool
    ToolAllowlist      []string
    RiskLevel          string
    Status             string
    CreatedBy          *uuid.UUID
    CreatedAt          time.Time
    UpdatedAt          time.Time
}
```

- [ ] **Step 3: Implement validation**

Add validation helpers to `service.go`:

```go
func validateMCPDefinitionInput(req CreateMCPServerDefinitionRequest) error {
    if req.TenantID == uuid.Nil {
        return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
    }
    if req.UserID == uuid.Nil {
        return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
    }
    if strings.TrimSpace(req.Name) == "" {
        return fmt.Errorf("%w: mcp name is required", ErrInvalidInput)
    }
    if strings.TrimSpace(req.ServerKey) == "" {
        return fmt.Errorf("%w: server_key is required", ErrInvalidInput)
    }
    if req.Transport != MCPTransportHTTP && req.Transport != MCPTransportStreamableHTTP {
        return fmt.Errorf("%w: transport must be http or streamable_http", ErrInvalidInput)
    }
    parsed, err := url.Parse(strings.TrimSpace(req.URL))
    if err != nil || parsed.Scheme == "" || parsed.Host == "" {
        return fmt.Errorf("%w: mcp url must be absolute http url", ErrInvalidInput)
    }
    if parsed.Scheme != "http" && parsed.Scheme != "https" {
        return fmt.Errorf("%w: mcp url must use http or https", ErrInvalidInput)
    }
    for _, name := range append(req.RequiredEnvVars, req.OptionalEnvVars...) {
        if !envNamePattern.MatchString(strings.TrimSpace(name)) {
            return fmt.Errorf("%w: invalid environment variable name %q", ErrInvalidInput, name)
        }
    }
    return nil
}
```

Use a local env-name regex in the capability package instead of importing employee internals.

- [ ] **Step 4: Add repository and service methods**

Extend `Repository` and `Service` with:

```go
CreateMCPServerDefinition(ctx context.Context, req CreateMCPServerDefinitionRequest) (MCPDefinition, error)
ListMCPServerDefinitions(ctx context.Context, req ListMCPServerDefinitionsRequest) ([]MCPDefinition, error)
DeleteMCPServerDefinition(ctx context.Context, req DeleteMCPServerDefinitionRequest) error
CreateTeamMCPBinding(ctx context.Context, req CreateTeamMCPBindingRequest) (MCPBinding, error)
CreateEmployeeMCPBindingV2(ctx context.Context, req CreateEmployeeMCPBindingV2Request) (MCPBinding, error)
ListEffectiveMCPConfig(ctx context.Context, req EmployeeScopedRequest) ([]EffectiveMCPServer, error)
```

The v2 binding create path must call `GetMCPServerDefinition` first and fail if the MCP server is disabled or deleted.

- [ ] **Step 5: Add binding preflight tests**

Add a service test that binds an MCP requiring `GITHUB_TOKEN` to an employee with no such environment variable and verifies the binding returns status `blocked_missing_env` and `missing_env_vars=["GITHUB_TOKEN"]`. This may require a capability repository method that checks configured env var names by employee.

- [ ] **Step 6: Add HTTP handlers and routes**

Add handlers:

```text
GET    /api/v1/mcp-servers
POST   /api/v1/mcp-servers
DELETE /api/v1/mcp-servers/{serverId}
POST   /api/v1/teams/{teamId}/mcp-bindings
POST   /api/v1/digital-employees/{employeeId}/mcp-bindings
GET    /api/v1/digital-employees/{employeeId}/effective-mcp-servers
```

Keep current `/teams/{teamId}/mcp-servers` responses backward compatible during the migration window if existing tests depend on them. New Web code should use registry and binding endpoints.

- [ ] **Step 7: Run targeted tests**

```bash
go test ./apps/control-plane/internal/capability -count=1
go test ./apps/control-plane/internal/api -run 'Test(Team|Employee).*MCP|TestRoutes' -count=1
corepack pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/capability \
  apps/control-plane/internal/api/server.go \
  apps/control-plane/internal/api/team_routes_test.go \
  apps/control-plane/internal/api/employee_routes_test.go \
  contracts/control-plane/openapi.yaml \
  apps/control-plane/internal/api/gen/control_plane.gen.go
git commit -m "feat: manage mcp registry and bindings in control plane"
```

### Task 3: Runtime Effective MCP Payload From Control Plane

**Files:**
- Modify: `apps/control-plane/internal/employee/run_service.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify tests: `apps/control-plane/internal/employee/run_service_test.go`
- Modify tests: `apps/control-plane/internal/employee/service_test.go`

- [ ] **Step 1: Write failing run payload test**

Add to `apps/control-plane/internal/employee/run_service_test.go`:

```go
func TestBuildStartSessionPayloadIncludesEffectiveMCPServers(t *testing.T) {
    run := &DigitalEmployeeRun{ID: uuid.New(), DigitalEmployeeID: uuid.New()}
    payload := buildStartSessionPayload(
        CreateDigitalEmployeeRunRequest{TenantID: uuid.New(), DigitalEmployeeID: run.DigitalEmployeeID},
        "Inspect repo",
        "Inspect repo",
        RunPreflight{ProviderType: "codex"},
        run,
        nil,
        nil,
        nil,
        []RuntimeMCPServerPayload{{
            ServerID:         "mcp-1",
            ServerKey:        "github",
            Name:             "GitHub MCP",
            Transport:        "streamable_http",
            URL:              "https://api.githubcopilot.com/mcp/",
            AuthStrategy:     "bearer_env",
            CredentialEnvVar: "GITHUB_TOKEN",
            RequiredEnvVars:  []string{"GITHUB_TOKEN"},
            SourceScope:      "employee",
        }},
    )

    servers, ok := payload["mcp_servers"].([]map[string]any)
    if !ok || len(servers) != 1 {
        t.Fatalf("expected one mcp server payload, got %#v", payload["mcp_servers"])
    }
    if servers[0]["server_key"] != "github" || servers[0]["credential_env_var"] != "GITHUB_TOKEN" {
        t.Fatalf("unexpected mcp payload: %#v", servers[0])
    }
}
```

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestBuildStartSessionPayloadIncludesEffectiveMCPServers -count=1
```

Expected: FAIL because `buildStartSessionPayload` currently emits empty MCP servers.

- [ ] **Step 2: Add runtime MCP payload type**

In `apps/control-plane/internal/employee/types.go`, define:

```go
type RuntimeMCPServerPayload struct {
    ServerID         string
    ServerKey        string
    Name             string
    Transport        string
    URL              string
    AuthStrategy     string
    CredentialEnvVar string
    RequiredEnvVars  []string
    HeadersEnv       map[string]string
    SourceScope      string
    PermissionScope  map[string]any
}
```

- [ ] **Step 3: Replace empty MCP payload**

Change `buildStartSessionPayload` to accept `runtimeMCPServers []RuntimeMCPServerPayload` and call:

```go
"mcp_servers": runtimeMCPServersPayload(runtimeMCPServers),
```

Implement:

```go
func runtimeMCPServersPayload(servers []RuntimeMCPServerPayload) []map[string]any {
    payload := make([]map[string]any, 0, len(servers))
    for _, server := range servers {
        payload = append(payload, map[string]any{
            "server_id":          server.ServerID,
            "server_key":         server.ServerKey,
            "name":               server.Name,
            "transport":          server.Transport,
            "url":                server.URL,
            "auth_strategy":      server.AuthStrategy,
            "credential_env_var": server.CredentialEnvVar,
            "required_env_vars":  stringSliceForRuntime(server.RequiredEnvVars),
            "headers_env":        stringMapForRuntime(server.HeadersEnv),
            "source_scope":       server.SourceScope,
            "permission_scope":   mapForRuntime(server.PermissionScope),
        })
    }
    return payload
}
```

- [ ] **Step 4: Wire effective MCP lookup into run dependencies**

Add an interface beside `RuntimeEnvironmentVariableLister`:

```go
type RuntimeMCPLister interface {
    ListRuntimeMCPServersForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeMCPServerPayload, error)
}
```

Set it on the run service using the capability service adapter. On start-session, load effective MCP after env vars; if a binding is missing env vars, exclude it and include a preflight warning in command metadata.

- [ ] **Step 5: Add redaction test**

Extend the existing redaction test around runtime command payloads so environment values are redacted but `credential_env_var` names remain visible. Expected redacted entry:

```json
{
  "server_key": "github",
  "credential_env_var": "GITHUB_TOKEN",
  "required_env_vars": ["GITHUB_TOKEN"]
}
```

- [ ] **Step 6: Run targeted tests**

```bash
go test ./apps/control-plane/internal/employee -run 'TestBuildStartSessionPayloadIncludesEffectiveMCPServers|TestStartDigitalEmployeeRun|TestRuntimeCommandPayloadRedacts' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/employee
git commit -m "feat: include effective mcp servers in runtime payload"
```

### Task 4: Runtime Agent Provider Config Materialization

**Files:**
- Modify: `apps/runtime-agent/src/commands/payload.rs`
- Create: `apps/runtime-agent/src/mcp_config.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/src/workspace_files.rs`
- Modify: `apps/runtime-agent/src/lib.rs`
- Test: `apps/runtime-agent/tests/runtime_command_executor_test.rs` or colocated Rust unit tests in `mcp_config.rs`

- [ ] **Step 1: Write renderer tests**

Create unit tests in `apps/runtime-agent/src/mcp_config.rs`:

```rust
#[test]
fn renders_codex_remote_mcp_with_bearer_env() {
    let servers = vec![RuntimeMCPServerPayload {
        server_id: "mcp-1".to_string(),
        server_key: "github".to_string(),
        name: Some("GitHub MCP".to_string()),
        transport: "streamable_http".to_string(),
        url: Some("https://api.githubcopilot.com/mcp/".to_string()),
        auth_strategy: Some("bearer_env".to_string()),
        credential_env_var: Some("GITHUB_TOKEN".to_string()),
        required_env_vars: vec!["GITHUB_TOKEN".to_string()],
        headers_env: Default::default(),
        source_scope: Some("employee".to_string()),
        config_ref: None,
        permission_scope: serde_json::json!({}),
    }];

    let rendered = render_codex_mcp_config(&servers).expect("render codex mcp");

    assert!(rendered.contains("[mcp_servers.github]"));
    assert!(rendered.contains("url = \"https://api.githubcopilot.com/mcp/\""));
    assert!(rendered.contains("bearer_token_env_var = \"GITHUB_TOKEN\""));
}
```

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml renders_codex_remote_mcp_with_bearer_env -- --nocapture
```

Expected: FAIL because renderer does not exist.

- [ ] **Step 2: Expand payload type**

Update `RuntimeMCPServerPayload`:

```rust
pub struct RuntimeMCPServerPayload {
    pub server_id: String,
    pub server_key: String,
    #[serde(default)]
    pub name: Option<String>,
    pub transport: String,
    #[serde(default)]
    pub url: Option<String>,
    #[serde(default)]
    pub auth_strategy: Option<String>,
    #[serde(default)]
    pub credential_env_var: Option<String>,
    #[serde(default)]
    pub required_env_vars: Vec<String>,
    #[serde(default)]
    pub headers_env: std::collections::BTreeMap<String, String>,
    #[serde(default)]
    pub source_scope: Option<String>,
    #[serde(default)]
    pub config_ref: Option<String>,
    #[serde(default = "default_metadata")]
    pub permission_scope: serde_json::Value,
}
```

Validation rules:

- `server_key` must match `^[A-Za-z0-9_-]+$`.
- `transport` must be `http` or `streamable_http`.
- `url` is required for HTTP transport.
- `required_env_vars`, `credential_env_var`, and `headers_env` values must be valid environment variable names.

- [ ] **Step 3: Implement renderer module**

Implement functions:

```rust
pub fn materialize_mcp_config(
    agent_home_dir: &Path,
    provider_type: &str,
    servers: &[RuntimeMCPServerPayload],
) -> Result<Vec<PathBuf>>
```

Provider targets:

```rust
match provider_type {
    "codex" => agent_home_dir.join(".codex").join("config.toml"),
    "claude-code" => agent_home_dir.join(".mcp.json"),
    "opencode" => agent_home_dir.join("opencode.json"),
    other => anyhow::bail!("unsupported provider_type for mcp config: {other}"),
}
```

For OpenCode, if `opencode.json` exists, parse it as JSON object and replace only its `mcp` field. Preserve unrelated keys.

For Codex, apply the same merge rule: if `.codex/config.toml` exists, parse it, replace only the `mcp_servers` table, and write back every other key unchanged. A whole-file rewrite would erase the employee's other Codex settings. `.mcp.json` (Claude Code) is MCP-only, so a full rewrite there is acceptable.

- [ ] **Step 4: Use atomic write**

Add or expose an `atomic_write_workspace_file` helper that writes to the same directory, syncs if current patterns do so, and renames into place. Do not follow symlinks for the target directory.

- [ ] **Step 5: Wire into provision**

In `handle_provision_instance`, after `materialize_workspace` and before command completion:

```rust
if !payload.mcp_servers.is_empty() {
    if let Err(error) = materialize_mcp_config(
        &PathBuf::from(&payload.agent_home_dir),
        &payload.provider_type,
        &payload.mcp_servers,
    ) {
        let error = self.recorded_error(&command.id, error);
        let message = error.to_string();
        self.write_provisioning_failure(&command.id, message).await?;
        return Err(error);
    }
}
```

- [ ] **Step 6: Add executor test**

Add a Runtime command executor test that provisions a Codex instance with one MCP server and verifies `.codex/config.toml` contains the server and no token value.

- [ ] **Step 7: Run targeted Runtime tests**

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml mcp_config -- --nocapture
cargo test --manifest-path apps/runtime-agent/Cargo.toml provision_instance --test runtime_command_executor_test -- --nocapture
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/runtime-agent/src/commands/payload.rs \
  apps/runtime-agent/src/mcp_config.rs \
  apps/runtime-agent/src/commands/executor.rs \
  apps/runtime-agent/src/workspace_files.rs \
  apps/runtime-agent/src/lib.rs \
  apps/runtime-agent/tests/runtime_command_executor_test.rs
git commit -m "feat: materialize mcp config for providers"
```

### Task 5: Web MCP First-Level Management

**Files:**
- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`
- Create: `apps/web/src/routes/_authenticated/mcp/index.tsx`
- Create: `apps/web/src/features/mcp/index.tsx`
- Create: `apps/web/src/features/mcp/index.test.tsx`
- Modify: `apps/web/src/lib/api/capabilities.ts`
- Modify: `apps/web/src/lib/api/capabilities.test.ts`
- Modify: `apps/web/src/routeTree.gen.ts` if route generation updates it

- [ ] **Step 1: Read design rules**

Read `DESIGN.md` before UI changes:

```bash
sed -n '1,220p' DESIGN.md
```

Expected: confirms v3 Soft-Flat is current baseline.

- [ ] **Step 2: Add API client tests**

Add tests in `apps/web/src/lib/api/capabilities.test.ts`:

```ts
it("creates an MCP registry entry with required env vars", async () => {
  const fetcher = vi.fn().mockResolvedValue(jsonResponse({
    id: "mcp-github",
    name: "GitHub MCP",
    server_key: "github",
    transport: "streamable_http",
    url: "https://api.githubcopilot.com/mcp/",
    auth_strategy: "bearer_env",
    required_env_vars: ["GITHUB_TOKEN"],
    status: "active",
  }));

  await createMcpServerDefinition({ baseUrl: "http://control-plane.local", fetcher }, {
    name: "GitHub MCP",
    server_key: "github",
    transport: "streamable_http",
    url: "https://api.githubcopilot.com/mcp/",
    auth_strategy: "bearer_env",
    required_env_vars: ["GITHUB_TOKEN"],
  });

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/mcp-servers",
    expect.objectContaining({ method: "POST" }),
  );
});
```

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/capabilities.test.ts
```

Expected: FAIL because API functions are missing.

- [ ] **Step 3: Implement API client types and functions**

Add:

```ts
export type McpAuthStrategy = "none" | "bearer_env" | "headers_env";
export type McpTransport = "http" | "streamable_http";

export type McpServerDefinition = {
  id: string;
  tenant_id: string;
  name: string;
  server_key: string;
  description: string;
  transport: McpTransport;
  url: string;
  auth_strategy: McpAuthStrategy;
  required_env_vars: string[];
  optional_env_vars: string[];
  tool_allowlist: string[];
  risk_level: string;
  status: string;
};
```

Functions:

```ts
listMcpServerDefinitions(options)
createMcpServerDefinition(options, input)
deleteMcpServerDefinition(options, serverId)
bindTeamMcpServer(options, teamId, input)
bindEmployeeMcpServer(options, employeeId, input)
```

- [ ] **Step 4: Add page test**

In `apps/web/src/features/mcp/index.test.tsx`, test:

- Page heading is `MCP 管理`.
- Existing registry entry shows URL, auth strategy, required env vars.
- Create drawer submits `server_key`, `transport`, `url`, `auth_strategy`, `required_env_vars`.
- Missing env binding displays `缺少环境变量 GITHUB_TOKEN`.

- [ ] **Step 5: Implement MCP page**

Use existing v3 components:

- `PageShell`
- `PageHeader`
- `WorkSurface`
- `V3Button`
- `V3Table`
- `StatusPill`
- `V3EmptyState`
- `V3ErrorState`
- lucide `Network`, `KeyRound`, `Plus`, `Trash2`, `Bot`, `UsersRound`

No marketing hero. The first viewport should be the management tool:

- top metrics: total MCP, active MCP, blocked bindings, required env vars
- registry table
- create/edit drawer or inline panel
- binding side panel: select team or employee, show required env preflight

- [ ] **Step 6: Add route and sidebar entry**

Add route:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { McpManagementPage } from "@/features/mcp";

export const Route = createFileRoute("/_authenticated/mcp/")({
  component: McpManagementPage,
});
```

In sidebar, add:

```ts
{
  title: "MCP 管理",
  url: "/mcp",
  icon: Network,
  iconTone: "neutral",
}
```

- [ ] **Step 7: Run Web tests**

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/capabilities.test.ts src/features/mcp/index.test.tsx src/navigation-rules.test.ts
corepack pnpm --filter ./apps/web typecheck
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/components/layout/data/sidebar-data.ts \
  apps/web/src/routes/_authenticated/mcp/index.tsx \
  apps/web/src/features/mcp \
  apps/web/src/lib/api/capabilities.ts \
  apps/web/src/lib/api/capabilities.test.ts \
  apps/web/src/routeTree.gen.ts
git commit -m "feat: add mcp management console"
```

### Task 6: Team And Employee Binding UX

**Files:**
- Modify: `apps/web/src/features/teams/components/team-capabilities-tab.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`
- Modify: `apps/web/src/features/employees/components/employee-capabilities-panel.tsx`
- Modify: `apps/web/src/features/employees/config.test.tsx`

- [ ] **Step 1: Write team UX regression test**

Update the team capabilities test so "公共 MCP" no longer asks for raw MCP URL first. It should select from registered MCP entries and post `{ "mcp_server_id": "mcp-github", "credential_env_var": "GITHUB_TOKEN" }`.

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx -t "manages team public skills and MCP servers"
```

Expected: FAIL until UI is updated.

- [ ] **Step 2: Update team panel**

Replace `mcpName` and `mcpUrl` inputs with:

- MCP select from `listMcpServerDefinitions`
- credential/env-var select or text input for `credential_env_var`
- required env-var badges from selected MCP
- warning when selected MCP requires env vars that are not available on target employees; for team binding this is advisory because each employee has its own env values

- [ ] **Step 3: Write employee UX regression test**

Update employee config test so personal MCP binding shows:

- inherited team MCP
- personal MCP
- missing required env vars
- a Link or button to environment-variable section for `GITHUB_TOKEN`

- [ ] **Step 4: Update employee panel**

Employee binding must run preflight before enabling:

```ts
const missingEnvVars = selectedDefinition.required_env_vars.filter(
  (name) => !configuredEnvNames.has(name),
);
const canBind = selectedDefinition && missingEnvVars.length === 0;
```

If missing env vars exist, keep the binding button disabled and display exact variable names.

- [ ] **Step 5: Run Web tests**

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx src/features/employees/config.test.tsx
corepack pnpm --filter ./apps/web typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/features/teams/components/team-capabilities-tab.tsx \
  apps/web/src/features/teams/index.test.tsx \
  apps/web/src/features/employees/components/employee-capabilities-panel.tsx \
  apps/web/src/features/employees/config.test.tsx
git commit -m "feat: bind registered mcp servers to teams and employees"
```

### Task 7: End-To-End MCP Config Smoke

**Files:**
- Modify if needed: `scripts/dev-services.sh`
- Add if useful: `apps/runtime-agent/tests/fixtures/mcp_http_server.js`
- Add if useful: `docs/superpowers/specs/2026-06-28-mcp-http-capability-management-smoke.md`

- [ ] **Step 1: Start or inspect services**

Run:

```bash
scripts/dev-services.sh status
```

If services are stale after code changes:

```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart runtime-agent
```

Expected: Control Plane, Web, Runtime Agent are running and connected.

- [ ] **Step 2: Apply migrations**

Use the repo migration truth:

```bash
cd apps/control-plane
make migrate-status DATABASE_URL="$DATABASE_URL"
make migrate-up DATABASE_URL="$DATABASE_URL"
```

Expected: migration 037 is applied. If `DATABASE_URL` is not configured, stop and report blocked. If `migrate-up` fails with an `atlas.sum` checksum mismatch, the re-hash from Task 1 Step 2b was not committed — run `atlas migrate hash --dir file://internal/storage/migrations` and re-run.

- [ ] **Step 3: Real API smoke**

Using the authenticated local session or existing test auth pattern:

1. Create an MCP definition:

```json
{
  "name": "GitHub MCP",
  "server_key": "github",
  "transport": "streamable_http",
  "url": "https://api.githubcopilot.com/mcp/",
  "auth_strategy": "bearer_env",
  "required_env_vars": ["GITHUB_TOKEN"],
  "risk_level": "medium"
}
```

2. Upsert employee env var `GITHUB_TOKEN`.
3. Bind MCP to the employee.
4. Start or provision the employee.
5. Verify Runtime command payload contains `mcp_servers`.
6. Verify Runtime workspace contains provider config with env-var references and no token value.

- [ ] **Step 4: Browser smoke**

Use Chrome plug or in-app browser:

- Navigate to `/mcp`.
- Create or view the MCP definition.
- Bind it to one digital employee.
- Open that employee config page.
- Verify the effective MCP list shows inherited/personal source and no missing env vars after env var setup.

- [ ] **Step 5: Full verification commands**

Run:

```bash
corepack pnpm verify:control-plane
corepack pnpm verify:runtime-agent
corepack pnpm --filter ./apps/web run test
corepack pnpm --filter ./apps/web typecheck
git diff --check
```

Expected: PASS. If Web browser smoke is blocked by auth or browser runtime, report it separately and do not claim end-to-end completion.

- [ ] **Step 6: Completion check**

Before claiming done, use project skill:

```bash
sed -n '1,260p' .codex/skills/superteam-completion-check/SKILL.md
```

Then complete its checklist with real evidence. Do not describe mock/unit/build-only checks as real MCP end-to-end verification.

- [ ] **Step 7: Commit final verification notes if docs changed**

```bash
git status --short
git diff --check
git commit -m "test: verify mcp http capability flow"
```

Only commit if there are actual verification docs or test fixtures to commit.

---

## Acceptance Criteria

- MCP management is a first-level Console surface, not only a team-local form.
- Control Plane stores MCP definitions once and stores team/employee bindings separately.
- MCP definitions only support HTTP/streamable HTTP transports; no Runtime-hosted MCP server process is modeled.
- Binding to an employee is strongly tied to employee environment variables. Missing required env vars are visible and block effective Runtime use.
- Runtime payload includes only active, env-satisfied effective MCP servers.
- Runtime Agent writes provider config files atomically and never writes credential values into config files.
- Codex, Claude Code, and OpenCode config rendering are covered by tests.
- Existing team-inherited plus employee-private MCP behavior remains intact.
- Real smoke verifies the current code through Control Plane, DB, Runtime Agent, and provider workspace config files before the task is called complete.

## Known Risks

- `sqlc` or `oapi-codegen` may be unavailable locally. If so, generated files must be updated manually in the existing style and flagged in the final report.
- Provider hot reload behavior differs. The product contract should say MCP changes apply on next run/provision unless the provider adapter later proves a safe reload.
- Migrating legacy raw URL MCP rows into a registry may produce duplicate definitions if names differ but URLs match. The backfill should dedupe by `(tenant_id, name, url)` for safety and the UI should allow later cleanup.
- Team-level required env vars cannot be fully validated until a target employee is known. Team binding can be saved, but each employee's effective config must still be gated by that employee's env vars.
- The rendered provider config references env-var *names* only (e.g. `bearer_token_env_var = "GITHUB_TOKEN"`); the token resolves at runtime only if the employee's env values are actually injected into the provider process environment. Task 3/Task 7 must confirm this projection already happens (effective env values reach the Runtime session env) — otherwise the bearer header resolves to nothing and the MCP call fails silently.
- Task 7 real smoke must assert on materialized config file contents (server present, no token value written) rather than live reachability of `https://api.githubcopilot.com/mcp/`. Prefer the optional `mcp_http_server.js` fixture as a deterministic local target so the smoke does not flake on network or GitHub credentials.

## Self-Review

- Spec coverage: the plan covers first-level MCP management, HTTP-only MCP, no Runtime-hosted server process, team/employee binding, employee env-var authentication, provider config projection, and real verification.
- Placeholder scan: no unresolved placeholder markers remain; every implementation area has concrete files, commands, and expected results.
- Type consistency: `MCPDefinition`, `MCPBinding`, `RuntimeMCPServerPayload`, `required_env_vars`, `credential_env_var`, and `server_key` are used consistently across Control Plane, Runtime, and Web.
