# Digital Employee Directory Management Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move digital employee execution from `agents/{execution_instance_id}` to stable team-owned employee homes, backed by DB-stored workspace files and provider-specific materialization.

**Architecture:** Control Plane remains the source of truth for employees, workspace files, file revisions, skill/MCP effective capability snapshots, provider sessions, runs, and audit. Runtime materializes a checked subset of that state into `{workspace_base_dir}/teams/{team_id}/employees/{digital_employee_id}` and starts Provider processes from that directory. Provider adapters own `.claude`, `.opencode`, and future provider private folders; project tasks and employee debug runs share the same employee home and differ only by run/session metadata.

**Tech Stack:** Go + pgx/sqlc + PostgreSQL migrations + existing Control Plane employee service; Rust Runtime Agent + serde + tokio + existing provider adapters; repo scripts via `corepack pnpm`.

---

## Scope And Guardrails

- This plan implements the first durable cut of the confirmed design in `docs/superpowers/specs/2026-06-13-digital-employee-directory-management-refactor-design.md`.
- Directory identity is `team_id + digital_employee_id`. `execution_instance_id` remains the run binding record and registry key where current runtime internals need it, but it is not the directory key.
- Project IDs never appear in the employee home path. Project task dispatch passes `project_id` and `project_task_id` only through metadata/context.
- Runtime does not create generic `state/`, `sessions/`, `runs/`, `artifacts/`, `skills/`, `mcp/`, or `context/` folders under the employee home. Provider adapters may create provider-owned private folders.
- First phase stores workspace file text in DB. Object-store columns are created now but remain inactive until a separate storage implementation is introduced.
- First phase creates and syncs `AGENTS.md`; it models future user-added root files through the same file/revision tables. The Console Instructions UI is outside this plan.
- `CLAUDE.md` is Runtime-generated compatibility material. It is not a user-editable workspace file.
- First phase carries `skills` and `mcp_servers` arrays in `provision_instance` and `sync_workspace_files` payloads. Empty arrays are valid while team MCP management is incomplete. Team knowledge base and external capability materialization are outside this plan.
- Provider type, file role, sync policy, and storage backend are strings validated in application code, not PostgreSQL enums.
- Do not rewrite unrelated existing worktree changes. Stage only files touched by this plan during implementation.

## File Structure

Control Plane storage:

- Create: `apps/control-plane/internal/storage/migrations/017_digital_employee_workspace_files.sql`
  - Adds `digital_employee_workspace_files`, `digital_employee_workspace_file_revisions`, and `digital_employee_workspace_file_syncs`.
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
  - Refresh after adding migration.
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
  - Adds static migration assertions for tables, indexes, comments, and DB-first/object-store-ready columns.
- Create: `apps/control-plane/internal/storage/queries/employee_workspace_files.sql`
  - sqlc queries for file identity, revisions, active revision listing, and sync projection updates.
- Generated: `apps/control-plane/internal/storage/queries/employee_workspace_files.sql.go`
- Modify generated sqlc package files as produced by `make -C apps/control-plane generate-sqlc`.

Control Plane employee domain:

- Modify: `apps/control-plane/internal/employee/repository.go`
  - Adds workspace file params/records and repository methods.
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
  - Implements workspace file repository methods and JSON/nullable conversions.
- Modify: `apps/control-plane/internal/employee/service.go`
  - Computes canonical employee home, creates default `AGENTS.md`, builds provisioning payload with files/capabilities.
- Modify: `apps/control-plane/internal/employee/service_test.go`
  - Covers default file creation, payload contents, and directory identity.
- Modify: `apps/control-plane/internal/employee/run_service.go`
  - Includes `team_id`, `agent_home_dir`, and target workspace file revisions in `start_session`.
- Modify: `apps/control-plane/internal/employee/run_service_test.go`
  - Covers debug/project run payloads sharing the same employee home and carrying metadata.

Runtime Agent domain:

- Modify: `apps/runtime-agent/src/controlplane/models.rs`
  - Adds `sync_workspace_files` command type and provision payload shape.
- Modify: `apps/runtime-agent/src/commands/payload.rs`
  - Adds typed payload structs for provision/session/sync workspace file materialization.
- Modify: `apps/runtime-agent/src/instances.rs`
  - Changes employee home calculation to `teams/{team_id}/employees/{digital_employee_id}` and removes generic subdir creation.
- Create: `apps/runtime-agent/src/workspace_files.rs`
  - Validates workspace paths, atomically writes DB-backed files, creates `CLAUDE.md` compatibility link/file, and initializes provider private dirs.
- Modify: `apps/runtime-agent/src/lib.rs`
  - Exports `workspace_files`.
- Modify: `apps/runtime-agent/Cargo.toml`
  - Adds `sha2 = "0.10"` for workspace file SHA-256 verification.
- Modify: `apps/runtime-agent/Cargo.lock`
  - Regenerated by Cargo after adding `sha2`.
- Modify: `apps/runtime-agent/src/commands/executor.rs`
  - Materializes provision/sync payloads and uses provisioned employee home as Provider `cwd`.
- Modify: `apps/runtime-agent/src/controlplane/ws.rs`
  - Updates command-loop tests for new provision/start payloads.
- Modify: `apps/runtime-agent/tests/instances_test.rs`
  - Covers new directory path and invalid IDs.
- Create: `apps/runtime-agent/tests/workspace_files_test.rs`
  - Covers path safety, provider private dirs, `AGENTS.md`, and `CLAUDE.md`.
- Modify: `apps/runtime-agent/tests/runtime_command_payload_test.rs`
  - Covers typed payload parsing and rejection.
- Modify: `apps/runtime-agent/tests/runtime_command_executor_test.rs`
  - Covers provision, sync, and start-session `cwd`.

No Web UI files are part of this first implementation plan.

---

### Task 1: Add Workspace File Storage Schema And Queries

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/017_digital_employee_workspace_files.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Create: `apps/control-plane/internal/storage/queries/employee_workspace_files.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Generated: `apps/control-plane/internal/storage/queries/employee_workspace_files.sql.go`

- [ ] **Step 1: Add the failing migration test**

Append this test to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestDigitalEmployeeWorkspaceFilesMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/017_digital_employee_workspace_files.sql")
	if err != nil {
		t.Fatalf("read digital employee workspace files migration: %v", err)
	}
	sql := string(body)

	required := []string{
		"CREATE TABLE IF NOT EXISTS digital_employee_workspace_files",
		"CREATE TABLE IF NOT EXISTS digital_employee_workspace_file_revisions",
		"CREATE TABLE IF NOT EXISTS digital_employee_workspace_file_syncs",
		"current_revision_id UUID",
		"storage_backend VARCHAR(50) NOT NULL",
		"object_key TEXT",
		"content_text TEXT",
		"content_hash VARCHAR(64) NOT NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_files_active_path",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_file_revisions_number",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_file_syncs_target",
		"COMMENT ON TABLE digital_employee_workspace_files IS '数字员工工作目录受控文件身份表'",
		"COMMENT ON TABLE digital_employee_workspace_file_revisions IS '数字员工工作目录受控文件内容版本表'",
		"COMMENT ON TABLE digital_employee_workspace_file_syncs IS '数字员工工作目录文件同步状态投影表'",
	}
	for _, expected := range required {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %q", expected)
		}
	}

	forbidden := []string{
		"CREATE TYPE",
		"provider_type_enum",
		"REFERENCES digital_employees",
		"ON DELETE CASCADE",
	}
	for _, value := range forbidden {
		if strings.Contains(sql, value) {
			t.Fatalf("migration must not contain %q", value)
		}
	}
}
```

- [ ] **Step 2: Run the migration test and confirm it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeWorkspaceFilesMigration -count=1
```

Expected: FAIL because `017_digital_employee_workspace_files.sql` does not exist yet.

- [ ] **Step 3: Create the migration**

Create `apps/control-plane/internal/storage/migrations/017_digital_employee_workspace_files.sql`:

```sql
CREATE TABLE IF NOT EXISTS digital_employee_workspace_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    path TEXT NOT NULL,
    file_role VARCHAR(50) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    sync_policy VARCHAR(50) NOT NULL DEFAULT 'auto',
    current_revision_id UUID,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_files_active_path
    ON digital_employee_workspace_files(tenant_id, digital_employee_id, path)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_files_active_entrypoint
    ON digital_employee_workspace_files(tenant_id, digital_employee_id)
    WHERE deleted_at IS NULL AND status = 'active' AND file_role = 'entrypoint';

CREATE INDEX IF NOT EXISTS idx_de_workspace_files_employee_status
    ON digital_employee_workspace_files(tenant_id, digital_employee_id, status, updated_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS digital_employee_workspace_file_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    file_id UUID NOT NULL,
    revision_number INTEGER NOT NULL,
    content_text TEXT,
    content_hash VARCHAR(64) NOT NULL,
    size_bytes INTEGER NOT NULL,
    storage_backend VARCHAR(50) NOT NULL DEFAULT 'db',
    object_key TEXT,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    change_note TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_de_workspace_file_revisions_db_content
        CHECK (
            (storage_backend = 'db' AND content_text IS NOT NULL AND object_key IS NULL)
            OR
            (storage_backend = 'object_store' AND object_key IS NOT NULL)
        ),
    CONSTRAINT chk_de_workspace_file_revisions_sha256
        CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_de_workspace_file_revisions_size
        CHECK (size_bytes >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_file_revisions_number
    ON digital_employee_workspace_file_revisions(file_id, revision_number);

CREATE INDEX IF NOT EXISTS idx_de_workspace_file_revisions_tenant_file
    ON digital_employee_workspace_file_revisions(tenant_id, file_id, revision_number DESC);

CREATE TABLE IF NOT EXISTS digital_employee_workspace_file_syncs (
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    execution_instance_id UUID NOT NULL,
    file_id UUID NOT NULL,
    revision_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL,
    synced_hash VARCHAR(64),
    error_message TEXT,
    last_command_id VARCHAR(255),
    last_synced_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_file_syncs_target
    ON digital_employee_workspace_file_syncs(tenant_id, digital_employee_id, execution_instance_id, file_id);

CREATE INDEX IF NOT EXISTS idx_de_workspace_file_syncs_status
    ON digital_employee_workspace_file_syncs(tenant_id, digital_employee_id, status, updated_at DESC);

COMMENT ON TABLE digital_employee_workspace_files IS '数字员工工作目录受控文件身份表';
COMMENT ON COLUMN digital_employee_workspace_files.id IS '受控文件主键 UUID';
COMMENT ON COLUMN digital_employee_workspace_files.tenant_id IS '文件所属租户 ID';
COMMENT ON COLUMN digital_employee_workspace_files.team_id IS '文件所属数字员工团队 ID，目录按团队归属组织';
COMMENT ON COLUMN digital_employee_workspace_files.digital_employee_id IS '文件所属数字员工 ID';
COMMENT ON COLUMN digital_employee_workspace_files.path IS '数字员工根目录下的安全相对路径';
COMMENT ON COLUMN digital_employee_workspace_files.file_role IS '文件角色，例如 entrypoint、supporting_doc、provider_config 或 generated，由应用层注册表校验';
COMMENT ON COLUMN digital_employee_workspace_files.mime_type IS '文件 MIME 类型';
COMMENT ON COLUMN digital_employee_workspace_files.sync_policy IS '同步策略，例如 auto、manual 或 disabled，由应用层注册表校验';
COMMENT ON COLUMN digital_employee_workspace_files.current_revision_id IS '当前激活的文件版本 ID';
COMMENT ON COLUMN digital_employee_workspace_files.status IS '文件状态，例如 active、archived 或 deleted';
COMMENT ON COLUMN digital_employee_workspace_files.metadata IS '文件扩展元数据 JSON';
COMMENT ON COLUMN digital_employee_workspace_files.created_by IS '创建文件的用户 ID，系统创建时可为空';
COMMENT ON COLUMN digital_employee_workspace_files.created_at IS '文件创建时间';
COMMENT ON COLUMN digital_employee_workspace_files.updated_at IS '文件更新时间';
COMMENT ON COLUMN digital_employee_workspace_files.archived_at IS '文件归档时间';
COMMENT ON COLUMN digital_employee_workspace_files.deleted_at IS '文件软删除时间';

COMMENT ON TABLE digital_employee_workspace_file_revisions IS '数字员工工作目录受控文件内容版本表';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.id IS '文件版本主键 UUID';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.tenant_id IS '文件版本所属租户 ID';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.file_id IS '所属受控文件 ID';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.revision_number IS '文件内递增版本号';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.content_text IS 'DB 存储模式下的文本正文';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.content_hash IS '文件正文 SHA-256 十六进制校验值';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.size_bytes IS '文件正文 UTF-8 字节数';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.storage_backend IS '正文存储后端，首版使用 db，预留 object_store';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.object_key IS '对象存储模式下的对象键';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.created_by IS '创建版本的用户 ID，系统创建时可为空';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.created_at IS '文件版本创建时间';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.change_note IS '版本变更说明';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.metadata IS '文件版本扩展元数据 JSON';

COMMENT ON TABLE digital_employee_workspace_file_syncs IS '数字员工工作目录文件同步状态投影表';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.tenant_id IS '同步状态所属租户 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.digital_employee_id IS '同步状态所属数字员工 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.execution_instance_id IS '同步目标执行实例 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.file_id IS '同步的受控文件 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.revision_id IS '同步目标文件版本 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.runtime_node_id IS '同步目标 Runtime 节点 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.status IS '同步状态，例如 pending、synced 或 failed';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.synced_hash IS 'Runtime 回写的已同步文件校验值';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.error_message IS '同步失败时的错误摘要';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.last_command_id IS '最后一次同步命令 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.last_synced_at IS '最后同步成功时间';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.updated_at IS '同步状态更新时间';
```

- [ ] **Step 4: Add sqlc queries**

Create `apps/control-plane/internal/storage/queries/employee_workspace_files.sql`:

```sql
-- name: CreateDigitalEmployeeWorkspaceFile :one
INSERT INTO digital_employee_workspace_files (
    tenant_id,
    team_id,
    digital_employee_id,
    path,
    file_role,
    mime_type,
    sync_policy,
    status,
    metadata,
    created_by
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('path')::text,
    sqlc.arg('file_role')::varchar,
    sqlc.arg('mime_type')::varchar,
    sqlc.arg('sync_policy')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_by')::uuid
) RETURNING *;

-- name: CreateDigitalEmployeeWorkspaceFileRevision :one
INSERT INTO digital_employee_workspace_file_revisions (
    tenant_id,
    file_id,
    revision_number,
    content_text,
    content_hash,
    size_bytes,
    storage_backend,
    object_key,
    created_by,
    change_note,
    metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('file_id')::uuid,
    sqlc.arg('revision_number')::integer,
    sqlc.narg('content_text')::text,
    sqlc.arg('content_hash')::varchar,
    sqlc.arg('size_bytes')::integer,
    sqlc.arg('storage_backend')::varchar,
    sqlc.narg('object_key')::text,
    sqlc.narg('created_by')::uuid,
    sqlc.narg('change_note')::text,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ActivateDigitalEmployeeWorkspaceFileRevision :one
UPDATE digital_employee_workspace_files
SET current_revision_id = sqlc.arg('revision_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('file_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: ListCurrentDigitalEmployeeWorkspaceFilesForSync :many
SELECT
    f.id AS file_id,
    f.tenant_id,
    f.team_id,
    f.digital_employee_id,
    f.path,
    f.file_role,
    f.mime_type,
    f.sync_policy,
    f.status,
    f.metadata AS file_metadata,
    r.id AS revision_id,
    r.revision_number,
    r.content_text,
    r.content_hash,
    r.size_bytes,
    r.storage_backend,
    r.object_key,
    r.metadata AS revision_metadata
FROM digital_employee_workspace_files f
JOIN digital_employee_workspace_file_revisions r
  ON r.id = f.current_revision_id
 AND r.tenant_id = f.tenant_id
WHERE f.tenant_id = sqlc.arg('tenant_id')::uuid
  AND f.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND f.status = 'active'
  AND f.deleted_at IS NULL
  AND f.sync_policy <> 'disabled'
ORDER BY f.path ASC;

-- name: UpsertDigitalEmployeeWorkspaceFileSync :exec
INSERT INTO digital_employee_workspace_file_syncs (
    tenant_id,
    digital_employee_id,
    execution_instance_id,
    file_id,
    revision_id,
    runtime_node_id,
    status,
    synced_hash,
    error_message,
    last_command_id,
    last_synced_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('execution_instance_id')::uuid,
    sqlc.arg('file_id')::uuid,
    sqlc.arg('revision_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('status')::varchar,
    sqlc.narg('synced_hash')::varchar,
    sqlc.narg('error_message')::text,
    sqlc.narg('last_command_id')::varchar,
    sqlc.narg('last_synced_at')::timestamptz
) ON CONFLICT (tenant_id, digital_employee_id, execution_instance_id, file_id) DO UPDATE SET
    revision_id = EXCLUDED.revision_id,
    runtime_node_id = EXCLUDED.runtime_node_id,
    status = EXCLUDED.status,
    synced_hash = EXCLUDED.synced_hash,
    error_message = EXCLUDED.error_message,
    last_command_id = EXCLUDED.last_command_id,
    last_synced_at = EXCLUDED.last_synced_at,
    updated_at = NOW();
```

- [ ] **Step 5: Generate sqlc output**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: sqlc regenerates `apps/control-plane/internal/storage/queries/employee_workspace_files.sql.go`, `models.go`, and `querier.go` without errors.

- [ ] **Step 6: Refresh Atlas migration checksum**

Run:

```bash
cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations
```

Expected: `apps/control-plane/internal/storage/migrations/atlas.sum` is updated.

- [ ] **Step 7: Run storage tests**

Run:

```bash
go test ./apps/control-plane/internal/storage -run 'TestDigitalEmployeeWorkspaceFilesMigration|TestMigrations' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/017_digital_employee_workspace_files.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/migrations_test.go \
  apps/control-plane/internal/storage/queries/employee_workspace_files.sql \
  apps/control-plane/internal/storage/queries/employee_workspace_files.sql.go \
  apps/control-plane/internal/storage/queries/models.go \
  apps/control-plane/internal/storage/queries/querier.go
git commit -m "feat: add digital employee workspace file storage"
```

---

### Task 2: Add Control Plane Workspace File Domain And Default AGENTS.md

**Files:**
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`

- [ ] **Step 1: Add failing service tests**

Add this test to `apps/control-plane/internal/employee/service_test.go` near the existing create/provisioning tests:

```go
func TestCreateDigitalEmployeeCreatesDefaultAgentsWorkspaceFile(t *testing.T) {
	svc, repo, _, req := newCreateDigitalEmployeeReadyFixture(t)
	req.Name = "上架助手"

	created, err := svc.CreateDigitalEmployee(context.Background(), req)
	if err != nil {
		t.Fatalf("create digital employee: %v", err)
	}

	if len(repo.workspaceFiles) != 1 {
		t.Fatalf("expected one workspace file, got %d", len(repo.workspaceFiles))
	}
	file := repo.workspaceFiles[0]
	if file.DigitalEmployeeID != created.ID || file.TeamID != req.TeamID {
		t.Fatalf("workspace file owner mismatch: %#v", file)
	}
	if file.Path != "AGENTS.md" || file.FileRole != "entrypoint" || file.SyncPolicy != "auto" {
		t.Fatalf("unexpected default workspace file: %#v", file)
	}

	if len(repo.workspaceFileRevisions) != 1 {
		t.Fatalf("expected one workspace file revision, got %d", len(repo.workspaceFileRevisions))
	}
	revision := repo.workspaceFileRevisions[0]
	if revision.FileID != file.ID || revision.StorageBackend != "db" {
		t.Fatalf("unexpected default revision: %#v", revision)
	}
	if !strings.Contains(revision.ContentText, "上架助手") || !strings.Contains(revision.ContentText, "Execution Contract") {
		t.Fatalf("default AGENTS.md content did not include role and contract: %q", revision.ContentText)
	}
	if revision.ContentHash != sha256Hex(revision.ContentText) {
		t.Fatalf("revision hash mismatch: %s", revision.ContentHash)
	}
}
```

Add this test to verify the provisioning payload:

```go
func TestCreateDigitalEmployeeProvisionPayloadUsesTeamEmployeeHomeAndWorkspaceFiles(t *testing.T) {
	svc, _, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)

	created, err := svc.CreateDigitalEmployee(context.Background(), req)
	if err != nil {
		t.Fatalf("create digital employee: %v", err)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one runtime command, got %d", len(dispatcher.commands))
	}

	var payload map[string]any
	if err := json.Unmarshal(dispatcher.commands[0].Payload, &payload); err != nil {
		t.Fatalf("decode runtime command payload: %v", err)
	}
	expectedHome := "/runtime/reported/agent-home/teams/" + (*req.TeamID).String() + "/employees/" + created.ID.String()
	if got := payload["agent_home_dir"]; got != expectedHome {
		t.Fatalf("expected agent_home_dir %q, got %#v", expectedHome, got)
	}

	rawFiles, ok := payload["workspace_files"].([]any)
	if !ok || len(rawFiles) != 1 {
		t.Fatalf("expected one workspace file payload, got %#v", payload["workspace_files"])
	}
	files, ok := rawFiles[0].(map[string]any)
	if !ok {
		t.Fatalf("expected workspace file object, got %#v", rawFiles[0])
	}
	if files["path"] != "AGENTS.md" || files["storage_backend"] != "db" {
		t.Fatalf("unexpected workspace file payload: %#v", files)
	}
	if _, ok := payload["skills"].([]any); !ok {
		t.Fatalf("expected skills array in payload, got %#v", payload["skills"])
	}
	if _, ok := payload["mcp_servers"].([]any); !ok {
		t.Fatalf("expected mcp_servers array in payload, got %#v", payload["mcp_servers"])
	}
}
```

- [ ] **Step 2: Run the new tests and confirm they fail**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateDigitalEmployeeCreatesDefaultAgentsWorkspaceFile|TestCreateDigitalEmployeeProvisionPayloadUsesTeamEmployeeHomeAndWorkspaceFiles' -count=1
```

Expected: FAIL because repository methods, file records, and payload fields do not exist.

- [ ] **Step 3: Add repository domain records and methods**

In `apps/control-plane/internal/employee/repository.go`, add:

```go
type CreateWorkspaceFileParams struct {
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Path              string
	FileRole          string
	MimeType          string
	SyncPolicy        string
	Status            string
	Metadata          map[string]any
	CreatedBy         *uuid.UUID
}

type CreateWorkspaceFileRevisionParams struct {
	TenantID       uuid.UUID
	FileID         uuid.UUID
	RevisionNumber int32
	ContentText    string
	ContentHash    string
	SizeBytes      int32
	StorageBackend string
	ObjectKey      *string
	CreatedBy      *uuid.UUID
	ChangeNote     *string
	Metadata       map[string]any
}

type WorkspaceFileRecord struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Path              string
	FileRole          string
	MimeType          string
	SyncPolicy        string
	CurrentRevisionID *uuid.UUID
	Status            string
	Metadata          map[string]any
	CreatedBy         *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type WorkspaceFileRevisionRecord struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	FileID         uuid.UUID
	RevisionNumber int32
	ContentText    string
	ContentHash    string
	SizeBytes      int32
	StorageBackend string
	ObjectKey      *string
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
	ChangeNote     *string
	Metadata       map[string]any
}

type WorkspaceFileForSyncRecord struct {
	FileID            uuid.UUID
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Path              string
	FileRole          string
	MimeType          string
	SyncPolicy        string
	FileMetadata      map[string]any
	RevisionID        uuid.UUID
	RevisionNumber    int32
	ContentText       string
	ContentHash       string
	SizeBytes         int32
	StorageBackend    string
	ObjectKey         *string
	RevisionMetadata  map[string]any
}
```

Extend the `Repository` interface with:

```go
CreateWorkspaceFile(ctx context.Context, params CreateWorkspaceFileParams) (WorkspaceFileRecord, error)
CreateWorkspaceFileRevision(ctx context.Context, params CreateWorkspaceFileRevisionParams) (WorkspaceFileRevisionRecord, error)
ActivateWorkspaceFileRevision(ctx context.Context, tenantID, fileID, revisionID uuid.UUID) (WorkspaceFileRecord, error)
ListWorkspaceFilesForSync(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]WorkspaceFileForSyncRecord, error)
```

In `apps/control-plane/internal/employee/service_test.go`, extend `memoryRepository` with:

```go
workspaceFiles         []WorkspaceFileRecord
workspaceFileRevisions []WorkspaceFileRevisionRecord
```

Add these methods to the memory repository:

```go
func (r *memoryRepository) CreateWorkspaceFile(_ context.Context, params CreateWorkspaceFileParams) (WorkspaceFileRecord, error) {
	now := time.Now().UTC()
	record := WorkspaceFileRecord{
		ID:                uuid.New(),
		TenantID:          params.TenantID,
		TeamID:            params.TeamID,
		DigitalEmployeeID: params.DigitalEmployeeID,
		Path:              params.Path,
		FileRole:          params.FileRole,
		MimeType:          params.MimeType,
		SyncPolicy:        params.SyncPolicy,
		Status:            params.Status,
		Metadata:          cloneMap(params.Metadata),
		CreatedBy:         validUUIDPtr(params.CreatedBy),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	r.workspaceFiles = append(r.workspaceFiles, record)
	return record, nil
}

func (r *memoryRepository) CreateWorkspaceFileRevision(_ context.Context, params CreateWorkspaceFileRevisionParams) (WorkspaceFileRevisionRecord, error) {
	record := WorkspaceFileRevisionRecord{
		ID:             uuid.New(),
		TenantID:       params.TenantID,
		FileID:         params.FileID,
		RevisionNumber: params.RevisionNumber,
		ContentText:    params.ContentText,
		ContentHash:    params.ContentHash,
		SizeBytes:      params.SizeBytes,
		StorageBackend: params.StorageBackend,
		ObjectKey:      cloneStringPtrForTest(params.ObjectKey),
		CreatedBy:      validUUIDPtr(params.CreatedBy),
		CreatedAt:      time.Now().UTC(),
		ChangeNote:     cloneStringPtrForTest(params.ChangeNote),
		Metadata:       cloneMap(params.Metadata),
	}
	r.workspaceFileRevisions = append(r.workspaceFileRevisions, record)
	return record, nil
}

func (r *memoryRepository) ActivateWorkspaceFileRevision(_ context.Context, tenantID, fileID, revisionID uuid.UUID) (WorkspaceFileRecord, error) {
	for index := range r.workspaceFiles {
		if r.workspaceFiles[index].TenantID == tenantID && r.workspaceFiles[index].ID == fileID {
			r.workspaceFiles[index].CurrentRevisionID = &revisionID
			r.workspaceFiles[index].UpdatedAt = time.Now().UTC()
			return r.workspaceFiles[index], nil
		}
	}
	return WorkspaceFileRecord{}, ErrNotFound
}

func (r *memoryRepository) ListWorkspaceFilesForSync(_ context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]WorkspaceFileForSyncRecord, error) {
	out := make([]WorkspaceFileForSyncRecord, 0)
	for _, file := range r.workspaceFiles {
		if file.TenantID != tenantID || file.DigitalEmployeeID != digitalEmployeeID || file.CurrentRevisionID == nil || file.SyncPolicy == "disabled" {
			continue
		}
		for _, revision := range r.workspaceFileRevisions {
			if revision.ID == *file.CurrentRevisionID {
				out = append(out, workspaceFileForSyncFromDefault(file, revision))
			}
		}
	}
	return out, nil
}
```

Update `memoryRepositorySnapshot`, `snapshot`, and `restore` so transaction rollback includes `workspaceFiles` and `workspaceFileRevisions`. Copy slices with `append([]WorkspaceFileRecord(nil), r.workspaceFiles...)` and `append([]WorkspaceFileRevisionRecord(nil), r.workspaceFileRevisions...)`.

- [ ] **Step 4: Implement path/hash/content helpers**

In `apps/control-plane/internal/employee/service.go`, add:

```go
func canonicalEmployeeHome(workspaceBaseDir string, teamID, digitalEmployeeID uuid.UUID) string {
	base := strings.TrimRight(strings.TrimSpace(workspaceBaseDir), "/")
	return base + "/teams/" + teamID.String() + "/employees/" + digitalEmployeeID.String()
}

func normalizeWorkspaceFilePath(path string) (string, error) {
	value := strings.TrimSpace(path)
	if value == "" || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return "", fmt.Errorf("%w: invalid workspace file path", ErrInvalidInput)
	}
	if strings.Contains(value, "\\") || strings.Contains(value, "\x00") {
		return "", fmt.Errorf("%w: invalid workspace file path", ErrInvalidInput)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("%w: invalid workspace file path", ErrInvalidInput)
		}
	}
	if value == "CLAUDE.md" || strings.HasPrefix(value, ".claude/") || strings.HasPrefix(value, ".opencode/") || strings.HasPrefix(value, ".git/") || strings.HasPrefix(value, ".superteam/") {
		return "", fmt.Errorf("%w: workspace file path is reserved", ErrInvalidInput)
	}
	return value, nil
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func buildDefaultAgentsContent(employee DigitalEmployeeRecord, configInput EmployeeConfigInput, preview *EffectiveConfigPreview) string {
	var builder strings.Builder
	builder.WriteString("You are an agent at SuperTeam.\n\n")
	builder.WriteString("# Execution Contract\n\n")
	builder.WriteString("- Work as digital employee: ")
	builder.WriteString(employee.Name)
	builder.WriteString("\n- Role: ")
	builder.WriteString(employee.Role)
	builder.WriteString("\n- Keep outputs aligned with the approved team and employee configuration.\n")
	builder.WriteString("- Ask for human approval before high-risk or ambiguous actions.\n")
	builder.WriteString("- Persist durable results through platform artifacts, evidence, or structured writeback.\n\n")
	if preview != nil {
		builder.WriteString("# Active Configuration\n\n")
		builder.WriteString("- Team config revision: ")
		builder.WriteString(preview.TeamConfigRevisionID.String())
		builder.WriteString("\n- Employee config revision: ")
		builder.WriteString(preview.EmployeeConfigRevisionID.String())
		builder.WriteString("\n")
	}
	if len(configInput.OutputContractAddendum) > 0 {
		builder.WriteString("\n# Output Contract Addendum\n\n")
		builder.WriteString("Additional output contract data is governed by the Control Plane effective configuration.\n")
	}
	return builder.String()
}
```

Add imports in `service.go`:

```go
import (
	"crypto/sha256"
	"encoding/hex"
)
```

- [ ] **Step 5: Implement workspace file creation in the create transaction**

In `apps/control-plane/internal/employee/service.go`, inside the create transaction after the execution instance is created and before the runtime command receipt is persisted, add this flow:

```go
agentsPath, err := normalizeWorkspaceFilePath("AGENTS.md")
if err != nil {
	return err
}
agentsContent := buildDefaultAgentsContent(employee, configInput, preview)
agentsHash := sha256Hex(agentsContent)
agentsFile, err := txRepo.CreateWorkspaceFile(ctx, CreateWorkspaceFileParams{
	TenantID:          employee.TenantID,
	TeamID:            preflight.TeamID,
	DigitalEmployeeID: employee.ID,
	Path:              agentsPath,
	FileRole:          "entrypoint",
	MimeType:          "text/markdown",
	SyncPolicy:        "auto",
	Status:            "active",
	Metadata:          map[string]any{"created_by": "digital_employee_create"},
	CreatedBy:         nil,
})
if err != nil {
	return fmt.Errorf("create default workspace file: %w", err)
}
agentsRevision, err := txRepo.CreateWorkspaceFileRevision(ctx, CreateWorkspaceFileRevisionParams{
	TenantID:        employee.TenantID,
	FileID:          agentsFile.ID,
	RevisionNumber: 1,
	ContentText:     agentsContent,
	ContentHash:     agentsHash,
	SizeBytes:       int32(len([]byte(agentsContent))),
	StorageBackend:  "db",
	Metadata:        map[string]any{"source": "default_agents"},
})
if err != nil {
	return fmt.Errorf("create default workspace file revision: %w", err)
}
agentsFile, err = txRepo.ActivateWorkspaceFileRevision(ctx, employee.TenantID, agentsFile.ID, agentsRevision.ID)
if err != nil {
	return fmt.Errorf("activate default workspace file revision: %w", err)
}
workspaceFiles := []WorkspaceFileForSyncRecord{workspaceFileForSyncFromDefault(agentsFile, agentsRevision)}
```

Define `workspaceFileForSyncFromDefault` in the same file:

```go
func workspaceFileForSyncFromDefault(file WorkspaceFileRecord, revision WorkspaceFileRevisionRecord) WorkspaceFileForSyncRecord {
	return WorkspaceFileForSyncRecord{
		FileID:            file.ID,
		TenantID:          file.TenantID,
		TeamID:            file.TeamID,
		DigitalEmployeeID: file.DigitalEmployeeID,
		Path:              file.Path,
		FileRole:          file.FileRole,
		MimeType:          file.MimeType,
		SyncPolicy:        file.SyncPolicy,
		FileMetadata:      cloneMap(file.Metadata),
		RevisionID:        revision.ID,
		RevisionNumber:    revision.RevisionNumber,
		ContentText:       revision.ContentText,
		ContentHash:       revision.ContentHash,
		SizeBytes:         revision.SizeBytes,
		StorageBackend:    revision.StorageBackend,
		ObjectKey:         revision.ObjectKey,
		RevisionMetadata:  cloneMap(revision.Metadata),
	}
}
```

- [ ] **Step 6: Implement PgRepository methods**

In `apps/control-plane/internal/employee/pg_repository.go`, implement the four new repository methods by calling the generated sqlc methods. Use the existing helper style:

```go
func (r *PgRepository) CreateWorkspaceFile(ctx context.Context, params CreateWorkspaceFileParams) (WorkspaceFileRecord, error) {
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return WorkspaceFileRecord{}, err
	}
	row, err := r.q.CreateDigitalEmployeeWorkspaceFile(ctx, queries.CreateDigitalEmployeeWorkspaceFileParams{
		TenantID:          params.TenantID,
		TeamID:            params.TeamID,
		DigitalEmployeeID: params.DigitalEmployeeID,
		Path:              params.Path,
		FileRole:          params.FileRole,
		MimeType:          params.MimeType,
		SyncPolicy:        params.SyncPolicy,
		Status:            params.Status,
		Metadata:          metadata,
		CreatedBy:         nullUUIDFromPtr(params.CreatedBy),
	})
	if err != nil {
		return WorkspaceFileRecord{}, err
	}
	return workspaceFileRecordFromQuery(row)
}
```

Add equivalent implementations for revision create, activation, and list-for-sync. Add converter helpers:

```go
func workspaceFileRecordFromQuery(row queries.DigitalEmployeeWorkspaceFile) (WorkspaceFileRecord, error) {
	metadata, err := mapFromJSONB(row.Metadata, "metadata")
	if err != nil {
		return WorkspaceFileRecord{}, err
	}
	return WorkspaceFileRecord{
		ID:                row.ID,
		TenantID:          row.TenantID,
		TeamID:            row.TeamID,
		DigitalEmployeeID: row.DigitalEmployeeID,
		Path:              row.Path,
		FileRole:          row.FileRole,
		MimeType:          row.MimeType,
		SyncPolicy:        row.SyncPolicy,
		CurrentRevisionID: uuidPtrFromNull(row.CurrentRevisionID),
		Status:            row.Status,
		Metadata:          metadata,
		CreatedBy:         uuidPtrFromNull(row.CreatedBy),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}, nil
}
```

Use existing nullable helpers where present; if `uuidPtrFromNull` is missing, add it once near other pg repository conversion helpers:

```go
func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := value.UUID
	return &id
}
```

- [ ] **Step 7: Run employee tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateDigitalEmployeeCreatesDefaultAgentsWorkspaceFile|TestCreateDigitalEmployeeProvisionPayloadUsesTeamEmployeeHomeAndWorkspaceFiles' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/employee/repository.go \
  apps/control-plane/internal/employee/pg_repository.go \
  apps/control-plane/internal/employee/service.go \
  apps/control-plane/internal/employee/service_test.go
git commit -m "feat: create default digital employee workspace file"
```

---

### Task 3: Update Provision And Start Session Payload Semantics

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/employee/run_service.go`
- Modify: `apps/control-plane/internal/employee/run_service_test.go`

- [ ] **Step 1: Add payload helper structs**

In `apps/control-plane/internal/employee/service.go`, add:

```go
type runtimeWorkspaceFilePayload struct {
	FileID         string         `json:"file_id"`
	RevisionID     string         `json:"revision_id"`
	Path           string         `json:"path"`
	FileRole       string         `json:"file_role"`
	MimeType       string         `json:"mime_type"`
	SyncPolicy     string         `json:"sync_policy"`
	ContentHash    string         `json:"content_hash"`
	SizeBytes      int32          `json:"size_bytes"`
	StorageBackend string         `json:"storage_backend"`
	ContentText    string         `json:"content_text,omitempty"`
	ObjectKey      string         `json:"object_key,omitempty"`
	Metadata       map[string]any `json:"metadata"`
}

type runtimeSkillPayload struct {
	SkillID     string         `json:"skill_id"`
	SkillKey    string         `json:"skill_key"`
	RevisionID  string         `json:"revision_id,omitempty"`
	Files       []map[string]any `json:"files"`
	ContentHash string         `json:"content_hash,omitempty"`
}

type runtimeMCPServerPayload struct {
	ServerID        string         `json:"server_id"`
	ServerKey       string         `json:"server_key"`
	Transport       string         `json:"transport"`
	ConfigRef       string         `json:"config_ref,omitempty"`
	PermissionScope map[string]any `json:"permission_scope"`
}
```

Add builders:

```go
func runtimeWorkspaceFilesPayload(files []WorkspaceFileForSyncRecord) []map[string]any {
	out := make([]map[string]any, 0, len(files))
	for _, file := range files {
		item := map[string]any{
			"file_id":         file.FileID.String(),
			"revision_id":     file.RevisionID.String(),
			"path":            file.Path,
			"file_role":       file.FileRole,
			"mime_type":       file.MimeType,
			"sync_policy":     file.SyncPolicy,
			"content_hash":    file.ContentHash,
			"size_bytes":      file.SizeBytes,
			"storage_backend": file.StorageBackend,
			"metadata":        cloneMap(file.RevisionMetadata),
		}
		if file.StorageBackend == "db" {
			item["content_text"] = file.ContentText
		}
		if file.ObjectKey != nil {
			item["object_key"] = *file.ObjectKey
		}
		out = append(out, item)
	}
	return out
}

func emptyRuntimeSkillsPayload() []map[string]any {
	return []map[string]any{}
}

func emptyRuntimeMCPServersPayload() []map[string]any {
	return []map[string]any{}
}
```

- [ ] **Step 2: Compute canonical employee home before upserting execution instance**

In the create flow, replace use of `preflight.AgentHomeDir` as the final instance home with:

```go
agentHomeDir := canonicalEmployeeHome(preflight.AgentHomeDir, preflight.TeamID, employee.ID)
```

Pass `agentHomeDir` into `UpsertExecutionInstanceParams.AgentHomeDir` and into `buildProvisionInstancePayload`.

- [ ] **Step 3: Update `buildProvisionInstancePayload`**

Change the function signature:

```go
func buildProvisionInstancePayload(commandID string, employee DigitalEmployeeRecord, instance DigitalEmployeeExecutionInstanceRecord, providerType string, preflight RuntimeProvisioningPreflight, req CreateDigitalEmployeeRequest, configInput EmployeeConfigInput, preview *EffectiveConfigPreview, workspaceFiles []WorkspaceFileForSyncRecord) map[string]any
```

Add these payload fields:

```go
"agent_home_dir":  instance.AgentHomeDir,
"workspace_files": runtimeWorkspaceFilesPayload(workspaceFiles),
"skills":          emptyRuntimeSkillsPayload(),
"mcp_servers":     emptyRuntimeMCPServersPayload(),
```

Keep `capability_selection` in the payload. The empty `skills` and `mcp_servers` arrays are the first-phase contract surface for team capability inheritance.

- [ ] **Step 4: Add failing start-session payload test**

In `apps/control-plane/internal/employee/run_service_test.go`, add:

```go
func TestRunServiceCreateRunDispatchesStartSessionWithEmployeeHomeAndWorkspaceFiles(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.workspaceFilesForSync = []WorkspaceFileForSyncRecord{{
		FileID:            uuid.MustParse("55555555-5555-4555-8555-555555555555"),
		TenantID:          runServiceTenantID,
		TeamID:            repo.preflight.TeamID,
		DigitalEmployeeID: runServiceEmployeeID,
		Path:              "AGENTS.md",
		FileRole:          "entrypoint",
		MimeType:          "text/markdown",
		SyncPolicy:        "auto",
		RevisionID:        uuid.MustParse("66666666-6666-4666-8666-666666666666"),
		RevisionNumber:    1,
		ContentText:       "# Execution Contract\n",
		ContentHash:       sha256Hex("# Execution Contract\n"),
		SizeBytes:         int32(len([]byte("# Execution Contract\n"))),
		StorageBackend:    "db",
	}}
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	req := validCreateRunServiceRequest()
	req.Metadata = map[string]any{
		"source":          "project_task_dispatch",
		"project_id":      "33333333-3333-4333-8333-333333333333",
		"project_task_id": "44444444-4444-4444-8444-444444444444",
	}

	run, err := service.CreateRun(context.Background(), req)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %d", len(dispatcher.commands))
	}

	payload := commandPayload(t, dispatcher.commands[0].command)
	if payload["run_id"] != run.ID.String() {
		t.Fatalf("run id mismatch in payload: %#v", payload)
	}
	if payload["agent_home_dir"] != repo.preflight.AgentHomeDir {
		t.Fatalf("expected preflight employee home %q, got %#v", repo.preflight.AgentHomeDir, payload["agent_home_dir"])
	}
	files, ok := payload["workspace_files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("expected AGENTS.md workspace file in start payload, got %#v", payload["workspace_files"])
	}
	file, ok := files[0].(map[string]any)
	if !ok || file["path"] != "AGENTS.md" {
		t.Fatalf("expected AGENTS.md workspace file payload, got %#v", files[0])
	}
	metadata := payload["metadata"].(map[string]any)
	if metadata["project_id"] != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("project metadata missing from start payload: %#v", metadata)
	}
}
```

- [ ] **Step 5: Extend run repository and payload builder**

Add `ListWorkspaceFilesForSync` to the run service repository dependency if the run service uses a separate fake/interface. In `dispatchStartSession`, load active sync files:

In `apps/control-plane/internal/employee/run_service_test.go`, extend `fakeRunServiceRepository` with:

```go
workspaceFilesForSync []WorkspaceFileForSyncRecord
```

Add:

```go
func (f *fakeRunServiceRepository) ListWorkspaceFilesForSync(_ context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]WorkspaceFileForSyncRecord, error) {
	out := make([]WorkspaceFileForSyncRecord, 0, len(f.workspaceFilesForSync))
	for _, file := range f.workspaceFilesForSync {
		if file.TenantID == tenantID && file.DigitalEmployeeID == digitalEmployeeID {
			out = append(out, file)
		}
	}
	return out, nil
}
```

```go
workspaceFiles, err := s.repository.ListWorkspaceFilesForSync(ctx, req.TenantID, req.DigitalEmployeeID)
if err != nil {
	return nil, fmt.Errorf("list workspace files for start session: %w", err)
}
payload := buildStartSessionPayload(req, objective, prompt, preflight, run, workspaceFiles)
```

Change `buildStartSessionPayload` signature to include `workspaceFiles []WorkspaceFileForSyncRecord` and add:

```go
"team_id":          preflight.TeamID.String(),
"workspace_files":  runtimeWorkspaceFilesPayload(workspaceFiles),
"skills":           emptyRuntimeSkillsPayload(),
"mcp_servers":      emptyRuntimeMCPServersPayload(),
```

- [ ] **Step 6: Run targeted Control Plane tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateDigitalEmployeeProvisionPayloadUsesTeamEmployeeHomeAndWorkspaceFiles|TestRunServiceCreateRunDispatchesStartSessionWithEmployeeHomeAndWorkspaceFiles' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/employee/service.go \
  apps/control-plane/internal/employee/service_test.go \
  apps/control-plane/internal/employee/run_service.go \
  apps/control-plane/internal/employee/run_service_test.go
git commit -m "feat: send workspace materialization in runtime commands"
```

---

### Task 4: Add Runtime Typed Payloads For Provision And Sync

**Files:**
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
- Modify: `apps/runtime-agent/src/commands/payload.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_payload_test.rs`

- [ ] **Step 1: Add failing payload tests**

Add to `apps/runtime-agent/tests/runtime_command_payload_test.rs`:

```rust
#[test]
fn parses_valid_provision_payload_with_workspace_file() {
    let command = RuntimeCommand {
        id: "cmd-provision".to_string(),
        command_type: RuntimeCommandType::ProvisionInstance,
        payload: serde_json::json!({
            "command_id": "cmd-provision",
            "tenant_id": "00000000-0000-4000-8000-000000000001",
            "team_id": "11111111-1111-4111-8111-111111111111",
            "digital_employee_id": "22222222-2222-4222-8222-222222222222",
            "execution_instance_id": "33333333-3333-4333-8333-333333333333",
            "runtime_node_id": "44444444-4444-4444-8444-444444444444",
            "provider_type": "claude-code",
            "agent_home_dir": "/tmp/workspaces/teams/11111111-1111-4111-8111-111111111111/employees/22222222-2222-4222-8222-222222222222",
            "workspace_files": [{
                "file_id": "55555555-5555-4555-8555-555555555555",
                "revision_id": "66666666-6666-4666-8666-666666666666",
                "path": "AGENTS.md",
                "file_role": "entrypoint",
                "mime_type": "text/markdown",
                "sync_policy": "auto",
                "content_hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
                "size_bytes": 0,
                "storage_backend": "db",
                "content_text": ""
            }],
            "skills": [],
            "mcp_servers": []
        }),
    };

    let parsed = RuntimeProvisionInstanceCommandPayload::from_command(&command).unwrap();
    assert_eq!(parsed.team_id, "11111111-1111-4111-8111-111111111111");
    assert_eq!(parsed.workspace_files[0].path, "AGENTS.md");
    assert!(parsed.skills.is_empty());
    assert!(parsed.mcp_servers.is_empty());
}

#[test]
fn parses_sync_workspace_files_command_type() {
    let raw = serde_json::json!({
        "id": "cmd-sync",
        "type": "sync_workspace_files",
        "payload": {}
    });
    let command: RuntimeCommand = serde_json::from_value(raw).unwrap();
    assert_eq!(command.command_type, RuntimeCommandType::SyncWorkspaceFiles);
}
```

- [ ] **Step 2: Run tests and confirm they fail**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_command_payload --test runtime_command_payload_test
```

Expected: FAIL because `SyncWorkspaceFiles` and `RuntimeProvisionInstanceCommandPayload` do not exist.

- [ ] **Step 3: Add command type**

In `apps/runtime-agent/src/controlplane/models.rs`, update the enum:

```rust
pub enum RuntimeCommandType {
    EnsureInstance,
    ProvisionInstance,
    SyncWorkspaceFiles,
    StartSession,
    ResumeSession,
    SendInput,
    StopSession,
    Unsupported(String),
}
```

Update deserialization:

```rust
"sync_workspace_files" => Self::SyncWorkspaceFiles,
```

- [ ] **Step 4: Add typed materialization payloads**

In `apps/runtime-agent/src/commands/payload.rs`, add:

```rust
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeWorkspaceFilePayload {
    pub file_id: String,
    pub revision_id: String,
    pub path: String,
    pub file_role: String,
    pub mime_type: String,
    pub sync_policy: String,
    pub content_hash: String,
    pub size_bytes: i32,
    pub storage_backend: String,
    #[serde(default)]
    pub content_text: Option<String>,
    #[serde(default)]
    pub object_key: Option<String>,
    #[serde(default = "default_metadata")]
    pub metadata: serde_json::Value,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeSkillPayload {
    pub skill_id: String,
    pub skill_key: String,
    #[serde(default)]
    pub revision_id: Option<String>,
    #[serde(default)]
    pub files: Vec<serde_json::Value>,
    #[serde(default)]
    pub content_hash: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeMCPServerPayload {
    pub server_id: String,
    pub server_key: String,
    pub transport: String,
    #[serde(default)]
    pub config_ref: Option<String>,
    #[serde(default = "default_metadata")]
    pub permission_scope: serde_json::Value,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeProvisionInstanceCommandPayload {
    pub command_id: String,
    pub tenant_id: String,
    pub team_id: String,
    pub digital_employee_id: String,
    pub execution_instance_id: String,
    pub runtime_node_id: String,
    pub provider_type: String,
    pub agent_home_dir: String,
    #[serde(default)]
    pub workspace_files: Vec<RuntimeWorkspaceFilePayload>,
    #[serde(default)]
    pub skills: Vec<RuntimeSkillPayload>,
    #[serde(default)]
    pub mcp_servers: Vec<RuntimeMCPServerPayload>,
}

impl RuntimeProvisionInstanceCommandPayload {
    pub fn from_command(command: &RuntimeCommand) -> Result<Self> {
        if !matches!(
            command.command_type,
            RuntimeCommandType::ProvisionInstance | RuntimeCommandType::SyncWorkspaceFiles
        ) {
            anyhow::bail!("runtime command type is not workspace materialization");
        }
        let payload: Self = serde_json::from_value(command.payload.clone())
            .context("invalid runtime provision command payload")?;
        payload.validate(command)?;
        Ok(payload)
    }

    fn validate(&self, command: &RuntimeCommand) -> Result<()> {
        if self.command_id != command.id {
            anyhow::bail!("command_id does not match runtime command id");
        }
        require_uuid_like("tenant_id", &self.tenant_id)?;
        require_uuid_like("team_id", &self.team_id)?;
        require_uuid_like("digital_employee_id", &self.digital_employee_id)?;
        require_uuid_like("execution_instance_id", &self.execution_instance_id)?;
        require_uuid_like("runtime_node_id", &self.runtime_node_id)?;
        if self.provider_type.trim().is_empty() {
            anyhow::bail!("provider_type is required");
        }
        if self.agent_home_dir.trim().is_empty() {
            anyhow::bail!("agent_home_dir is required");
        }
        for file in &self.workspace_files {
            require_uuid_like("file_id", &file.file_id)?;
            require_uuid_like("revision_id", &file.revision_id)?;
            if file.storage_backend == "db" && file.content_text.is_none() {
                anyhow::bail!("content_text is required for db-backed workspace file");
            }
        }
        Ok(())
    }
}
```

Update `RuntimeSessionCommandPayload` with new fields:

```rust
#[serde(default)]
pub tenant_id: Option<String>,
#[serde(default)]
pub team_id: Option<String>,
#[serde(default)]
pub runtime_node_id: Option<String>,
#[serde(default)]
pub agent_home_dir: Option<String>,
#[serde(default)]
pub workspace_files: Vec<RuntimeWorkspaceFilePayload>,
#[serde(default)]
pub skills: Vec<RuntimeSkillPayload>,
#[serde(default)]
pub mcp_servers: Vec<RuntimeMCPServerPayload>,
```

Do not add these fields to the global `REQUIRED_FIELDS` slice, because `StopSession` currently uses the same parser but does not need a Provider cwd. Add command-type-specific validation:

```rust
if !matches!(command.command_type, RuntimeCommandType::StopSession) {
    require_optional_uuid_like("tenant_id", &self.tenant_id)?;
    require_optional_uuid_like("team_id", &self.team_id)?;
    require_optional_uuid_like("runtime_node_id", &self.runtime_node_id)?;
    if self.agent_home_dir.as_deref().map(str::trim).filter(|value| !value.is_empty()).is_none() {
        anyhow::bail!("agent_home_dir is required");
    }
}
```

Add helper:

```rust
fn require_optional_uuid_like(field: &str, value: &Option<String>) -> Result<()> {
    match value.as_deref() {
        Some(value) => require_uuid_like(field, value),
        None => anyhow::bail!("{field} is required"),
    }
}
```

- [ ] **Step 5: Run payload tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_payload_test
```

Expected: PASS after updating existing start/resume/send-input test payload fixtures to include `tenant_id`, `team_id`, `runtime_node_id`, and `agent_home_dir`. Existing stop-session fixtures may include those fields but are not required to.

- [ ] **Step 6: Commit**

```bash
git add apps/runtime-agent/src/controlplane/models.rs \
  apps/runtime-agent/src/commands/payload.rs \
  apps/runtime-agent/tests/runtime_command_payload_test.rs
git commit -m "feat: type runtime workspace materialization payloads"
```

---

### Task 5: Implement Runtime Employee Home And Workspace File Materializer

**Files:**
- Modify: `apps/runtime-agent/src/instances.rs`
- Create: `apps/runtime-agent/src/workspace_files.rs`
- Modify: `apps/runtime-agent/src/lib.rs`
- Modify: `apps/runtime-agent/tests/instances_test.rs`
- Create: `apps/runtime-agent/tests/workspace_files_test.rs`

- [ ] **Step 1: Replace the failing instance test**

Update `apps/runtime-agent/tests/instances_test.rs` so `ensure_instance_creates_agent_home_directories` becomes:

```rust
#[test]
fn ensure_instance_creates_team_employee_home_without_generic_subdirs() {
    let temp = tempfile::tempdir().unwrap();
    let team_id = "11111111-1111-4111-8111-111111111111";
    let digital_employee_id = "22222222-2222-4222-8222-222222222222";

    let result = ensure_instance(EnsureInstanceRequest {
        base_dir: temp.path().to_path_buf(),
        team_id: team_id.to_string(),
        digital_employee_id: digital_employee_id.to_string(),
    })
    .unwrap();

    assert!(result.agent_home_dir.ends_with(format!("teams/{team_id}/employees/{digital_employee_id}")));
    assert!(result.agent_home_dir.is_dir());
    assert!(!result.agent_home_dir.join("state").exists());
    assert!(!result.agent_home_dir.join("sessions").exists());
    assert!(!result.agent_home_dir.join("runs").exists());
}
```

- [ ] **Step 2: Run instance test and confirm it fails**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test instances_test
```

Expected: FAIL because `EnsureInstanceRequest` still uses `execution_instance_id`.

- [ ] **Step 3: Update `instances.rs`**

Replace `EnsureInstanceRequest` and `ensure_instance` in `apps/runtime-agent/src/instances.rs` with:

```rust
#[derive(Debug, Clone)]
pub struct EnsureInstanceRequest {
    pub base_dir: PathBuf,
    pub team_id: String,
    pub digital_employee_id: String,
}

#[derive(Debug, Clone)]
pub struct EnsureInstanceResult {
    pub agent_home_dir: PathBuf,
}

pub fn ensure_instance(request: EnsureInstanceRequest) -> anyhow::Result<EnsureInstanceResult> {
    let agent_home_dir = request
        .base_dir
        .join("teams")
        .join(sanitize_segment("team_id", &request.team_id)?)
        .join("employees")
        .join(sanitize_segment("digital_employee_id", &request.digital_employee_id)?);
    std::fs::create_dir_all(&agent_home_dir)?;
    Ok(EnsureInstanceResult { agent_home_dir })
}

fn sanitize_segment(field: &str, value: &str) -> anyhow::Result<String> {
    if !is_uuid_like(value) {
        anyhow::bail!("{field} must be a UUID-like string");
    }
    Ok(value.to_string())
}
```

- [ ] **Step 4: Add workspace materializer tests**

Create `apps/runtime-agent/tests/workspace_files_test.rs`:

```rust
use superteam_runtime_agent::commands::payload::RuntimeWorkspaceFilePayload;
use superteam_runtime_agent::workspace_files::{
    ProviderHomeKind, WorkspaceMaterializationPlan, materialize_workspace, validate_workspace_path,
};

fn agents_file(content: &str) -> RuntimeWorkspaceFilePayload {
    RuntimeWorkspaceFilePayload {
        file_id: "55555555-5555-4555-8555-555555555555".to_string(),
        revision_id: "66666666-6666-4666-8666-666666666666".to_string(),
        path: "AGENTS.md".to_string(),
        file_role: "entrypoint".to_string(),
        mime_type: "text/markdown".to_string(),
        sync_policy: "auto".to_string(),
        content_hash: superteam_runtime_agent::workspace_files::sha256_hex(content.as_bytes()),
        size_bytes: content.len() as i32,
        storage_backend: "db".to_string(),
        content_text: Some(content.to_string()),
        object_key: None,
        metadata: serde_json::json!({}),
    }
}

#[test]
fn rejects_reserved_and_unsafe_workspace_paths() {
    for path in ["", "/AGENTS.md", "../AGENTS.md", "notes/../AGENTS.md", "CLAUDE.md", ".claude/settings.json", ".opencode/config.json", ".git/config", ".superteam/state.json"] {
        assert!(validate_workspace_path(path).is_err(), "path should be rejected: {path}");
    }
    assert_eq!(validate_workspace_path("docs/context.md").unwrap(), "docs/context.md");
}

#[test]
fn materialize_workspace_writes_agents_link_and_provider_dir() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("teams/team/employees/employee");
    std::fs::create_dir_all(&home).unwrap();

    let result = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::ClaudeCode,
        files: vec![agents_file("# Contract\n")],
    })
    .unwrap();

    assert_eq!(result.synced_files.len(), 1);
    assert_eq!(std::fs::read_to_string(home.join("AGENTS.md")).unwrap(), "# Contract\n");
    assert!(home.join(".claude").is_dir());
    assert!(home.join("CLAUDE.md").exists());
    assert!(!home.join("state").exists());
    assert!(!home.join("runs").exists());
}
```

- [ ] **Step 5: Add SHA-256 dependency**

In `apps/runtime-agent/Cargo.toml`, add:

```toml
sha2 = "0.10"
```

Then run:

```bash
cargo update --manifest-path apps/runtime-agent/Cargo.toml -p sha2
```

Expected: `apps/runtime-agent/Cargo.lock` records the `sha2` dependency graph.

- [ ] **Step 6: Implement `workspace_files.rs`**

Create `apps/runtime-agent/src/workspace_files.rs`:

```rust
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};

use crate::commands::payload::RuntimeWorkspaceFilePayload;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ProviderHomeKind {
    ClaudeCode,
    OpenCode,
}

#[derive(Debug, Clone)]
pub struct WorkspaceMaterializationPlan {
    pub agent_home_dir: PathBuf,
    pub provider_home: ProviderHomeKind,
    pub files: Vec<RuntimeWorkspaceFilePayload>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize)]
pub struct SyncedWorkspaceFile {
    pub file_id: String,
    pub revision_id: String,
    pub path: String,
    pub content_hash: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct WorkspaceMaterializationResult {
    pub agent_home_dir: PathBuf,
    pub synced_files: Vec<SyncedWorkspaceFile>,
}

pub fn provider_home_kind(provider_type: &str) -> Result<ProviderHomeKind> {
    match provider_type {
        "claude-code" => Ok(ProviderHomeKind::ClaudeCode),
        "opencode" => Ok(ProviderHomeKind::OpenCode),
        value => anyhow::bail!("unsupported provider_type for workspace materialization: {value}"),
    }
}

pub fn materialize_workspace(plan: WorkspaceMaterializationPlan) -> Result<WorkspaceMaterializationResult> {
    fs::create_dir_all(&plan.agent_home_dir).context("create agent home dir")?;
    ensure_provider_dir(&plan.agent_home_dir, &plan.provider_home)?;

    let mut synced_files = Vec::with_capacity(plan.files.len());
    for file in plan.files {
        if file.sync_policy == "disabled" {
            continue;
        }
        let safe_path = validate_workspace_path(&file.path)?;
        let content = match file.storage_backend.as_str() {
            "db" => file.content_text.clone().context("db-backed workspace file missing content_text")?,
            "object_store" => anyhow::bail!("object_store workspace files are not supported by this runtime build"),
            value => anyhow::bail!("unsupported workspace file storage_backend: {value}"),
        };
        let actual_hash = sha256_hex(content.as_bytes());
        if actual_hash != file.content_hash {
            anyhow::bail!("workspace file hash mismatch for {}", file.path);
        }
        atomic_write(&plan.agent_home_dir.join(&safe_path), content.as_bytes())?;
        synced_files.push(SyncedWorkspaceFile {
            file_id: file.file_id,
            revision_id: file.revision_id,
            path: safe_path,
            content_hash: actual_hash,
        });
    }

    ensure_claude_compatibility(&plan.agent_home_dir)?;
    Ok(WorkspaceMaterializationResult {
        agent_home_dir: plan.agent_home_dir,
        synced_files,
    })
}

pub fn validate_workspace_path(path: &str) -> Result<String> {
    let value = path.trim();
    if value.is_empty() || value.starts_with('/') || value.ends_with('/') || value.contains('\\') || value.contains('\0') {
        anyhow::bail!("invalid workspace file path");
    }
    if value == "CLAUDE.md"
        || value.starts_with(".claude/")
        || value.starts_with(".opencode/")
        || value.starts_with(".git/")
        || value.starts_with(".superteam/")
    {
        anyhow::bail!("workspace file path is reserved");
    }
    for segment in value.split('/') {
        if segment.is_empty() || segment == "." || segment == ".." {
            anyhow::bail!("invalid workspace file path");
        }
    }
    Ok(value.to_string())
}

pub fn sha256_hex(bytes: &[u8]) -> String {
    use sha2::{Digest, Sha256};
    let digest = Sha256::digest(bytes);
    format!("{digest:x}")
}

fn ensure_provider_dir(agent_home_dir: &Path, provider_home: &ProviderHomeKind) -> Result<()> {
    let name = match provider_home {
        ProviderHomeKind::ClaudeCode => ".claude",
        ProviderHomeKind::OpenCode => ".opencode",
    };
    fs::create_dir_all(agent_home_dir.join(name)).with_context(|| format!("create provider dir {name}"))
}

fn ensure_claude_compatibility(agent_home_dir: &Path) -> Result<()> {
    let agents = agent_home_dir.join("AGENTS.md");
    if !agents.exists() {
        return Ok(());
    }
    let claude = agent_home_dir.join("CLAUDE.md");
    if claude.exists() {
        return Ok(());
    }
    #[cfg(unix)]
    {
        std::os::unix::fs::symlink("AGENTS.md", &claude).or_else(|_| {
            fs::write(&claude, "See AGENTS.md for the active digital employee instructions.\n")
        })?;
    }
    #[cfg(not(unix))]
    {
        fs::write(&claude, "See AGENTS.md for the active digital employee instructions.\n")?;
    }
    Ok(())
}

fn atomic_write(path: &Path, bytes: &[u8]) -> Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).context("create workspace file parent")?;
    }
    let tmp = path.with_extension("tmp-superteam");
    {
        let mut file = fs::File::create(&tmp).context("create temporary workspace file")?;
        file.write_all(bytes).context("write temporary workspace file")?;
        file.sync_all().context("sync temporary workspace file")?;
    }
    fs::rename(&tmp, path).context("replace workspace file")
}
```

- [ ] **Step 7: Export the module**

In `apps/runtime-agent/src/lib.rs`, add:

```rust
pub mod workspace_files;
```

- [ ] **Step 8: Run Runtime workspace tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test instances_test
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test workspace_files_test
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/runtime-agent/src/instances.rs \
  apps/runtime-agent/src/workspace_files.rs \
  apps/runtime-agent/src/lib.rs \
  apps/runtime-agent/tests/instances_test.rs \
  apps/runtime-agent/tests/workspace_files_test.rs \
  apps/runtime-agent/Cargo.toml \
  apps/runtime-agent/Cargo.lock
git commit -m "feat: materialize digital employee workspace home"
```

---

### Task 6: Wire Runtime Provision, Sync, And Start Session To Employee Home

**Files:**
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/src/controlplane/ws.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_executor_test.rs`

- [ ] **Step 1: Add failing executor tests**

In `apps/runtime-agent/tests/runtime_command_executor_test.rs`, add:

```rust
#[tokio::test]
async fn provision_instance_materializes_team_employee_home() {
    let temp = tempfile::tempdir().unwrap();
    let mut config = RuntimeConfig::default();
    config.workspace.base_dir = temp.path().join("workspaces");
    let executor = RuntimeCommandExecutor::new(config.clone());

    let team_id = "11111111-1111-4111-8111-111111111111";
    let employee_id = "22222222-2222-4222-8222-222222222222";
    let home = config.workspace.base_dir.join("teams").join(team_id).join("employees").join(employee_id);
    let content = "# Execution Contract\n";
    let command = provision_command("cmd-provision", team_id, employee_id, home.to_str().unwrap(), content);

    executor.handle_command(command).await.expect("provision accepted");

    assert_eq!(std::fs::read_to_string(home.join("AGENTS.md")).unwrap(), content);
    assert!(home.join(".claude").is_dir());
    assert!(home.join("CLAUDE.md").exists());
    assert!(!home.join("state").exists());
}

#[tokio::test]
async fn start_session_uses_agent_home_dir_as_provider_cwd() {
    let temp = tempfile::tempdir().unwrap();
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-cwd",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"session-from-cwd-test"}'
printf '%s\n' '{"type":"result","result":"done"}'
"#,
    );
    let executor = configure_runtime(&temp, fake_claude);

    let team_id = "11111111-1111-4111-8111-111111111111";
    let employee_id = "22222222-2222-4222-8222-222222222222";
    let home = temp.path().join("workspaces").join("teams").join(team_id).join("employees").join(employee_id);
    std::fs::create_dir_all(&home).unwrap();

    let content = "# Execution Contract\n";
    let command = start_session_command_with_home("cmd-start", team_id, employee_id, home.to_str().unwrap(), content);
    let outcome = executor.handle_command(command).await.expect("start_session accepted");

    let run = executor.runs().get_run(outcome.run_id.as_deref().unwrap()).await.unwrap();
    assert_eq!(run.workspace_path, home);
}
```

Add these local helpers below `session_command_full`:

```rust
fn workspace_file(content: &str) -> serde_json::Value {
    serde_json::json!({
        "file_id": "55555555-5555-4555-8555-555555555555",
        "revision_id": "66666666-6666-4666-8666-666666666666",
        "path": "AGENTS.md",
        "file_role": "entrypoint",
        "mime_type": "text/markdown",
        "sync_policy": "auto",
        "content_hash": superteam_runtime_agent::workspace_files::sha256_hex(content.as_bytes()),
        "size_bytes": content.len() as i32,
        "storage_backend": "db",
        "content_text": content
    })
}

fn provision_command(
    command_id: &str,
    team_id: &str,
    employee_id: &str,
    agent_home_dir: &str,
    content: &str,
) -> RuntimeCommand {
    RuntimeCommand {
        id: command_id.to_string(),
        command_type: RuntimeCommandType::ProvisionInstance,
        payload: json!({
            "command_id": command_id,
            "tenant_id": "00000000-0000-4000-8000-000000000001",
            "team_id": team_id,
            "digital_employee_id": employee_id,
            "execution_instance_id": EXECUTION_INSTANCE_ID,
            "runtime_node_id": "44444444-4444-4444-8444-444444444444",
            "provider_type": "claude-code",
            "agent_home_dir": agent_home_dir,
            "workspace_files": [workspace_file(content)],
            "skills": [],
            "mcp_servers": []
        }),
    }
}

fn start_session_command_with_home(
    command_id: &str,
    team_id: &str,
    employee_id: &str,
    agent_home_dir: &str,
    content: &str,
) -> RuntimeCommand {
    RuntimeCommand {
        id: command_id.to_string(),
        command_type: RuntimeCommandType::StartSession,
        payload: json!({
            "command_id": command_id,
            "tenant_id": "00000000-0000-4000-8000-000000000001",
            "team_id": team_id,
            "digital_employee_id": employee_id,
            "execution_instance_id": EXECUTION_INSTANCE_ID,
            "runtime_node_id": "44444444-4444-4444-8444-444444444444",
            "provider_type": "claude-code",
            "agent_home_dir": agent_home_dir,
            "workspace_files": [workspace_file(content)],
            "skills": [],
            "mcp_servers": [],
            "session_policy": {
                "mode": "new",
                "provider_session_id": null,
                "recoverable": true
            },
            "prompt": "write the summary",
            "input": null,
            "context_refs": [],
            "artifact_refs": [],
            "model": null,
            "metadata": {"source": "executor-test"}
        }),
    }
}
```

- [ ] **Step 2: Run executor tests and confirm they fail**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_executor_test provision_instance_materializes_team_employee_home
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_executor_test start_session_uses_agent_home_dir_as_provider_cwd
```

Expected: FAIL because the executor still routes provision/start through `execution_instance_id` directory logic.

- [ ] **Step 3: Update executor imports**

In `apps/runtime-agent/src/commands/executor.rs`, update imports:

```rust
use crate::commands::payload::{
    RuntimeProvisionInstanceCommandPayload, RuntimeSessionCommandPayload, SessionPolicyMode,
};
use crate::workspace_files::{
    WorkspaceMaterializationPlan, materialize_workspace, provider_home_kind,
};
```

- [ ] **Step 4: Route `sync_workspace_files`**

In `handle_command`, add:

```rust
RuntimeCommandType::SyncWorkspaceFiles => self.handle_sync_workspace_files(command).await,
```

Implement:

```rust
async fn handle_sync_workspace_files(
    &self,
    command: RuntimeCommand,
) -> anyhow::Result<RuntimeCommandOutcome> {
    let payload = RuntimeProvisionInstanceCommandPayload::from_command(&command)
        .map_err(|error| self.recorded_error(&command.id, error))?;
    let provider_home = provider_home_kind(&payload.provider_type)
        .map_err(|error| self.recorded_error(&command.id, error))?;
    let result = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: PathBuf::from(&payload.agent_home_dir),
        provider_home,
        files: payload.workspace_files,
    })
    .map_err(|error| self.recorded_error(&command.id, error))?;
    if let Some(control_plane) = &self.control_plane {
        control_plane
            .complete_runtime_command(
                &command.id,
                &workspace_sync_completed_terminal(&result.agent_home_dir, result.synced_files),
            )
            .await?;
    }
    Ok(RuntimeCommandOutcome {
        command_id: command.id,
        accepted: true,
        run_id: None,
    })
}
```

- [ ] **Step 5: Update provision handler**

Replace `handle_provision_instance` parsing with `RuntimeProvisionInstanceCommandPayload::from_command`. Materialize the workspace using the payload `agent_home_dir`:

```rust
let payload = match RuntimeProvisionInstanceCommandPayload::from_command(&command) {
    Ok(payload) => payload,
    Err(error) => {
        let message = error.to_string();
        self.write_provisioning_failure(&command.id, message).await?;
        return Err(self.recorded_error(&command.id, error));
    }
};
let provider_home = provider_home_kind(&payload.provider_type)
    .map_err(|error| self.recorded_error(&command.id, error))?;
let result = match materialize_workspace(WorkspaceMaterializationPlan {
    agent_home_dir: PathBuf::from(&payload.agent_home_dir),
    provider_home,
    files: payload.workspace_files,
}) {
    Ok(result) => result,
    Err(error) => {
        let message = error.to_string();
        self.write_provisioning_failure(&command.id, message).await?;
        return Err(self.recorded_error(&command.id, error));
    }
};
```

Return `result.agent_home_dir` in `provisioning_completed_terminal`.

- [ ] **Step 6: Update start-session workspace resolution**

Replace `ensure_command_instance` with:

```rust
fn ensure_command_instance(
    &self,
    command_id: &str,
    payload: &RuntimeSessionCommandPayload,
) -> anyhow::Result<PathBuf> {
    let agent_home_dir_text = payload
        .agent_home_dir
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| self.recorded_error(command_id, anyhow::anyhow!("agent_home_dir is required")))?;
    let agent_home_dir = PathBuf::from(agent_home_dir_text);
    if !agent_home_dir.is_dir() {
        return Err(self.recorded_error(
            command_id,
            anyhow::anyhow!("agent_home_dir does not exist: {}", agent_home_dir_text),
        ));
    }
    let provider_home = provider_home_kind(&payload.provider_type)
        .map_err(|error| self.recorded_error(command_id, error))?;
    materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: agent_home_dir.clone(),
        provider_home,
        files: payload.workspace_files.clone(),
    })
    .map_err(|error| self.recorded_error(command_id, error))?;
    Ok(agent_home_dir)
}
```

- [ ] **Step 7: Add sync completion terminal**

In `executor.rs`, add:

```rust
fn workspace_sync_completed_terminal(
    agent_home_dir: &Path,
    synced_files: Vec<crate::workspace_files::SyncedWorkspaceFile>,
) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    result.insert(
        "agent_home_dir".to_string(),
        serde_json::Value::String(path_to_string(agent_home_dir)),
    );
    result.insert(
        "synced_files".to_string(),
        serde_json::to_value(synced_files)
            .unwrap_or_else(|_| serde_json::Value::Array(Vec::new())),
    );
    RuntimeCommandTerminalWriteback {
        status: "completed".to_string(),
        summary: Some("digital employee workspace files synced".to_string()),
        result: Some(result),
        diagnostic: None,
        provider_session_external_id: None,
        session_state_patch: None,
        log_ref: None,
        raw_result_ref: None,
        error_message: None,
        error_code: None,
        error_family: None,
    }
}
```

Derive `Serialize` for `SyncedWorkspaceFile` in `workspace_files.rs`.

- [ ] **Step 8: Update websocket tests**

In `apps/runtime-agent/src/controlplane/ws.rs`, update provision and start-session command JSON payloads to include:

```json
"tenant_id": "00000000-0000-4000-8000-000000000001",
"team_id": "11111111-1111-4111-8111-111111111111",
"digital_employee_id": "22222222-2222-4222-8222-222222222222",
"runtime_node_id": "44444444-4444-4444-8444-444444444444",
"agent_home_dir": "<workspace-base>/teams/11111111-1111-4111-8111-111111111111/employees/22222222-2222-4222-8222-222222222222",
"workspace_files": [],
"skills": [],
"mcp_servers": []
```

Replace assertions for `state`, `sessions`, and `runs` directories with assertions for `.claude` and absence of generic directories.

- [ ] **Step 9: Run Runtime command tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_executor_test
cargo test --manifest-path apps/runtime-agent/Cargo.toml controlplane::ws
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add apps/runtime-agent/src/commands/executor.rs \
  apps/runtime-agent/src/controlplane/ws.rs \
  apps/runtime-agent/tests/runtime_command_executor_test.rs
git commit -m "feat: run providers from digital employee home"
```

---

### Task 7: Preserve Team Skill And MCP Inheritance Contract

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/runtime-agent/src/workspace_files.rs`
- Modify: `apps/runtime-agent/tests/workspace_files_test.rs`

- [ ] **Step 1: Add Control Plane contract test**

In `apps/control-plane/internal/employee/service_test.go`, add:

```go
func TestCreateDigitalEmployeeProvisionPayloadCarriesEffectiveCapabilityArrays(t *testing.T) {
	svc, _, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	req.CapabilitySelection = map[string]any{
		"enabled_skills":      []any{"incident-diagnosis"},
		"enabled_mcp_servers": []any{"prometheus-readonly"},
	}

	_, err := svc.CreateDigitalEmployee(context.Background(), req)
	if err != nil {
		t.Fatalf("create digital employee: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(dispatcher.commands[0].Payload, &payload); err != nil {
		t.Fatalf("decode runtime command payload: %v", err)
	}
	if skills, ok := payload["skills"].([]any); !ok || len(skills) != 0 {
		t.Fatalf("first phase should carry skills array and allow it empty, got %#v", payload["skills"])
	}
	if servers, ok := payload["mcp_servers"].([]any); !ok || len(servers) != 0 {
		t.Fatalf("first phase should carry mcp_servers array and allow it empty, got %#v", payload["mcp_servers"])
	}
}
```

- [ ] **Step 2: Keep capability arrays explicit in payload builders**

Ensure both `buildProvisionInstancePayload` and `buildStartSessionPayload` always set:

```go
"skills":      emptyRuntimeSkillsPayload(),
"mcp_servers": emptyRuntimeMCPServersPayload(),
```

The service must not omit these fields when team capability data is empty.

- [ ] **Step 3: Add Runtime materialization no-op test**

In `apps/runtime-agent/tests/workspace_files_test.rs`, add:

```rust
#[test]
fn materialize_workspace_accepts_empty_skills_and_mcp_contract() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("employee");
    std::fs::create_dir_all(&home).unwrap();

    let result = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::OpenCode,
        files: vec![agents_file("# Contract\n")],
    })
    .unwrap();

    assert_eq!(result.synced_files.len(), 1);
    assert!(home.join(".opencode").is_dir());
}
```

This keeps first-phase Provider adapter behavior deterministic while leaving team skill/MCP file generation to the capability module integration.

- [ ] **Step 4: Run capability contract tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestCreateDigitalEmployeeProvisionPayloadCarriesEffectiveCapabilityArrays -count=1
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test workspace_files_test materialize_workspace_accepts_empty_skills_and_mcp_contract
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/employee/service.go \
  apps/control-plane/internal/employee/service_test.go \
  apps/runtime-agent/src/workspace_files.rs \
  apps/runtime-agent/tests/workspace_files_test.rs
git commit -m "feat: preserve team capability materialization contract"
```

---

### Task 8: Update Compatibility Tests And Run Full Verification

**Files:**
- Modify only test files already touched by earlier tasks if fixture payloads still use the old command shape.

- [ ] **Step 1: Search for stale directory assumptions**

Run:

```bash
rg -n 'agents/\\{|agents/|state|sessions|runs|agent_home_dir|execution_instance_id' apps/runtime-agent apps/control-plane/internal/employee -g '*.rs' -g '*.go'
```

Expected: remaining `execution_instance_id` usages are run/session/registry identifiers, not directory path builders. Remaining `state/sessions/runs` usages are run store or unrelated executor tests, not employee home provisioning assertions.

- [ ] **Step 2: Run Control Plane employee and storage tests**

Run:

```bash
go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/employee -count=1
```

Expected: PASS.

- [ ] **Step 3: Run Runtime tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: PASS.

- [ ] **Step 4: Run repo verification gates**

Run:

```bash
corepack pnpm verify:control-plane
corepack pnpm verify:runtime-agent
```

Expected: PASS. If `verify:contracts` fails because generated API files are already dirty from unrelated work, inspect the diff and either regenerate only the required contract outputs or report that the failure is unrelated and include the exact failing file list in the handoff.

- [ ] **Step 5: Run diff checks**

Run:

```bash
git diff --check
git status --short
```

Expected: `git diff --check` prints no whitespace errors. `git status --short` shows only files touched by this plan plus any pre-existing unrelated dirty files.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee \
  apps/control-plane/internal/storage \
  apps/runtime-agent
git commit -m "test: verify digital employee workspace materialization"
```

---

## Self-Review Checklist

- Spec section 2 core口径 maps to Tasks 2, 3, 5, and 6.
- Spec section 6 directory规范 maps to Tasks 5 and 6.
- Spec section 7 DB 设计 maps to Task 1.
- Spec section 8 Control Plane 行为 maps to Tasks 2, 3, and 7.
- Spec section 9 Runtime 命令 maps to Tasks 4, 5, and 6.
- Spec section 10 Provider Adapter 边界 maps to Tasks 5, 6, and 7.
- Spec section 11 团队技能与 MCP 继承 maps to Task 7.
- Spec section 12 项目任务与调试任务 maps to Task 3.
- Spec sections 13 and 14 path safety/sync status map to Tasks 1, 5, and 6.
- Spec section 15 兼容和迁移 is covered by creating new homes for new/provisioned employees and by keeping `agent_home_dir` as the execution instance record.
- Spec section 16 测试策略 maps to Tasks 1 through 8.
- No Console Instructions UI is included, matching the non-goal.
- No Provider private directory format is defined beyond creating `.claude` or `.opencode`.
