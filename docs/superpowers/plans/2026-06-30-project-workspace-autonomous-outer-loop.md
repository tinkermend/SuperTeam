# Project Workspace Autonomous Outer Loop Implementation Plan
> 复核状态：部分实现（Phase C1完成，但整体提纲未实现）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first working slice of project repo binding, role-derived Runtime workspace materialization, attestation-backed task results, and bounded revision iteration.

**Architecture:** Reuse the existing ProjectTask, ProjectTaskAttempt, RouteDecision, ProjectTaskResult, PreDispatchGate, ExecutionLedger, and budget-ledger surfaces. Phase 1 persists a nullable project git binding and dispatch metadata, separates Runtime agent home from provider CWD, requires runtime attestation refs for successful verification results, and routes revision-needed results back through existing retry/revision paths. Later resource pools, non-git side effects, strong attestation signing, and knowledge distillation remain explicit future work.

**Tech Stack:** Go Control Plane with PostgreSQL migrations and sqlc, Temporal workflow project coordinator, Rust Runtime Agent, provider CLI adapters for Claude Code/Codex/OpenCode, YAML OpenAPI contracts, existing root scripts via `corepack pnpm`.

---

## Scope And Ground Rules

- Source specs:
  - `docs/superpowers/specs/2026-06-29-project-code-workspace-runtime-affinity-design.md`
  - `docs/superpowers/specs/2026-06-30-autonomous-outer-loop-iteration-attestation-budget-design.md`
- This plan implements the combined Phase 1 plus the minimum budget/heartbeat hooks needed to prevent runaway execution.
- Do not implement Phase 2 "project resource pool + workflow template requires" in this plan. The nullable project repo binding is the Phase 1 source of truth.
- Do not model humans as digital employees.
- Do not store absolute runtime paths in Control Plane DB. Persist only stable identifiers and git binding fields.
- Use an isolated worktree at execution time because the current checkout may contain unrelated dirty frontend/spec work.
- Before any frontend edit, read `DESIGN.md`. This plan does not require frontend UI changes except OpenAPI/client schema fallout if generation touches them.

## Existing Surfaces To Reuse

- `apps/control-plane/internal/project/types.go`: Project, ProjectTask, ProjectTaskAttempt, ProjectTaskResult, budget, evidence, and event domain types.
- `apps/control-plane/internal/project/task_result_contract.go`: result contract validation and decision mapping.
- `apps/control-plane/internal/project/predispatch_gate.go`: dispatch gate budget/risk/runtime checks.
- `apps/control-plane/internal/workflow/projectcoordination/workflow.go`: Temporal coordinator signals and completion/failure routing.
- `apps/control-plane/internal/workflow/projectcoordination/project_store.go`: task decomposition, dispatch metadata, recovery task creation, run starter adapter boundary.
- `apps/control-plane/internal/employee/run_service.go`: Runtime `start_session` payload builder.
- `apps/runtime-agent/src/commands/payload.rs`: Runtime command payload schemas.
- `apps/runtime-agent/src/commands/executor.rs`: `ensure_command_instance`, provider request construction, writeback drain.
- `apps/runtime-agent/src/providers/{mod.rs,claude.rs,codex.rs,opencode.rs}`: provider launch command builders.
- `apps/runtime-agent/src/executor/workspace.rs`: per-run workspace helper, currently separate from command execution path.
- `apps/control-plane/internal/storage/migrations/029_execution_ledger_events.sql`: existing execution ledger migration.
- `apps/control-plane/internal/storage/migrations/034_project_task_results.sql`: existing task result and revision metadata migration.

## File Structure

- Create `apps/control-plane/internal/storage/migrations/039_project_repo_binding_and_attestation.sql`: nullable repo binding fields on `projects`, placement table, attestation table, attempt budget fields.
- Create `apps/control-plane/internal/storage/queries/project_runtime_affinity.sql`: sqlc queries for project repo binding, project placement, attestations, and attempt budget heartbeat updates.
- Modify generated `apps/control-plane/internal/storage/queries/*.go`: sqlc output.
- Modify `apps/control-plane/internal/storage/migrations/atlas.sum`: Atlas hash update.
- Modify `apps/control-plane/internal/storage/migrations_test.go`: migration shape tests.
- Modify `apps/control-plane/internal/project/types.go`: repo binding, workspace mode, attestation, budget guard, and project placement domain types.
- Modify `apps/control-plane/internal/project/repository.go`: repository methods for repo binding, placement, attestations, and attempt budget heartbeat.
- Modify `apps/control-plane/internal/project/pg_repository.go`: DB mappings and repository implementations.
- Modify `apps/control-plane/internal/project/service.go`: validation, project create/update repo fields, attestation recording, task result attestation validation, budget guard service method.
- Modify `apps/control-plane/internal/project/task_result_contract.go`: reject completed verification without runtime attestation refs when required.
- Modify `apps/control-plane/internal/project/task_result_contract_test.go`: attestation validation coverage.
- Modify `apps/control-plane/internal/project/service_test.go`: repo binding, attestation writeback, budget guard, revision result handling.
- Modify `apps/control-plane/internal/project/handler.go`: create/update/get project request/response fields and a runtime attestation writeback handler at `/runtime/project-task-attestations`.
- Modify `apps/control-plane/internal/project/handler_test.go`: HTTP mapping tests.
- Modify `contracts/control-plane/openapi.yaml`: project repo fields, attestation schemas/endpoints, budget heartbeat schema.
- Modify `apps/control-plane/internal/workflow/projectcoordination/types.go`: workspace mode and git binding metadata in `StartProjectTaskRunRequest`.
- Modify `apps/control-plane/internal/workflow/projectcoordination/task_type_defaults.go`: role/task-kind to workspace-mode derivation helper.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`: dispatch metadata includes repo binding, base ref, scope, workspace mode, upstream handoff refs, iteration keys.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow.go`: route revision-needed results into bounded iteration instead of unconditional human failure recovery.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`: dispatch metadata and revision-loop tests.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`: revision-loop and exhaustion tests.
- Modify `apps/control-plane/internal/app/app.go`: pass git/workspace metadata through the run starter adapter.
- Modify `apps/control-plane/internal/employee/run_types.go`: keep create-run request metadata generic and document that repo/workspace data travels through `Metadata`.
- Modify `apps/control-plane/internal/employee/run_service.go`: include repo/workspace metadata in `start_session` payload and idempotency fingerprint.
- Modify `apps/control-plane/internal/employee/run_service_test.go`: payload/fingerprint tests.
- Modify `apps/runtime-agent/src/commands/payload.rs`: add a typed `project_workspace()` accessor over the existing `metadata` value for `workspace_mode`, optional `project_git`, optional `attestation_policy`, and budget fields. Note: `agent_home_dir` is already a required top-level payload field (see `payload.rs`, validated in `validate`); do not re-add it.
- Modify `apps/runtime-agent/src/commands/executor.rs`: carry `agent_home_dir` separately from the provider workspace path. Actual git worktree materialization, provider skill symlinks, attestation capture, and heartbeat cancellation are Task 5+ work.
- Create `apps/runtime-agent/src/project_workspace.rs`: git repo/worktree materialization, sparse checkout, workspace-mode derivation support, cleanup policy.
- Create `apps/runtime-agent/src/attestation.rs`: command/runtime attestation structs, log hashing, artifact hashing, git evidence collection.
- Modify `apps/runtime-agent/src/controlplane/models.rs`: control-plane writeback structs for attestations and budget heartbeats.
- Modify `apps/runtime-agent/src/controlplane/client.rs`: attestation and budget heartbeat calls.
- Modify `apps/runtime-agent/src/providers/mod.rs`: `ProviderRequest` gains `agent_home_dir` and env/config support.
- Modify `apps/runtime-agent/src/providers/claude.rs`: `cwd` is worktree; `--mcp-config` points to agent home; skills symlink is already in cwd.
- Modify `apps/runtime-agent/src/providers/codex.rs`: `cwd`/`--cd` is worktree; do not default `CODEX_HOME` from `agent_home_dir`, so host-level Codex auth remains the default.
- Modify `apps/runtime-agent/src/providers/opencode.rs`: `cwd`/`--dir` is worktree; do not default `OPENCODE_CONFIG_DIR` / `OPENCODE_CONFIG` from `agent_home_dir`, so host-level OpenCode auth remains the default.
- Modify `apps/runtime-agent/src/executor/workspace.rs`: retain project task workspaces across iteration/human wait; only cleanup terminal task workspaces.
- Modify `contracts/runtime/openapi.yaml`: start-session project workspace and attestation fields if runtime contract is externally exposed.
- Modify the two source specs after code lands: append one dated "implemented by" note with the merged plan path and verification summary.

### Task 1: Database Schema For Repo Binding, Placement, Attestation, And Budget Heartbeat

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/039_project_repo_binding_and_attestation.sql`
- Create: `apps/control-plane/internal/storage/queries/project_runtime_affinity.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify generated: `apps/control-plane/internal/storage/queries/*.go`
- Modify generated: `apps/control-plane/internal/storage/migrations/atlas.sum`

- [ ] **Step 1: Write the failing migration shape test**

Add this test to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestProjectRepoBindingAndAttestationMigration(t *testing.T) {
	sql := readMigration(t, "039_project_repo_binding_and_attestation.sql")

	for _, want := range []string{
		"ALTER TABLE projects",
		"ADD COLUMN repo_url TEXT",
		"ADD COLUMN repo_default_branch VARCHAR(255)",
		"ADD COLUMN repo_git_credential_ref VARCHAR(255)",
		"ADD COLUMN repo_scope JSONB NOT NULL DEFAULT '[]'::jsonb",
		"ADD COLUMN repo_binding_status VARCHAR(32) NOT NULL DEFAULT 'unbound'",
		"CREATE TABLE project_placements",
		"CREATE TABLE project_task_attestations",
		"ALTER TABLE project_task_attempts",
		"ADD COLUMN budget_wall_clock_limit_sec INTEGER",
		"ADD COLUMN budget_last_heartbeat_at TIMESTAMPTZ",
		"ADD COLUMN budget_consumed_wall_clock_sec INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN budget_consumed_tokens INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN budget_tripped_at TIMESTAMPTZ",
		"ADD COLUMN budget_trip_reason VARCHAR(100)",
		"chk_projects_repo_binding_status",
		"chk_project_task_attestations_status",
		"uq_project_task_attestations_idempotency",
	} {
		assertMigrationContains(t, sql, want)
	}
}
```

- [ ] **Step 2: Run the migration test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestProjectRepoBindingAndAttestationMigration -count=1
```

Expected: FAIL because migration `039_project_repo_binding_and_attestation.sql` does not exist.

- [ ] **Step 3: Add the migration**

Create `apps/control-plane/internal/storage/migrations/039_project_repo_binding_and_attestation.sql`:

```sql
ALTER TABLE projects
    ADD COLUMN repo_url TEXT,
    ADD COLUMN repo_default_branch VARCHAR(255),
    ADD COLUMN repo_git_credential_ref VARCHAR(255),
    ADD COLUMN repo_scope JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN repo_binding_status VARCHAR(32) NOT NULL DEFAULT 'unbound';

ALTER TABLE projects
    ADD CONSTRAINT chk_projects_repo_binding_status
    CHECK (repo_binding_status IN ('unbound', 'bound'));

ALTER TABLE projects
    ADD CONSTRAINT chk_projects_repo_binding_consistent
    CHECK (
        (repo_binding_status = 'unbound' AND repo_url IS NULL AND repo_default_branch IS NULL)
        OR
        (repo_binding_status = 'bound' AND repo_url IS NOT NULL AND repo_default_branch IS NOT NULL)
    );

CREATE INDEX idx_projects_repo_binding_status
    ON projects(tenant_id, repo_binding_status)
    WHERE repo_binding_status = 'bound';

CREATE TABLE project_placements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    placement_status VARCHAR(32) NOT NULL,
    placement_reason VARCHAR(100),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_placements_project
        FOREIGN KEY (tenant_id, project_id)
        REFERENCES projects(tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_project_placements_status
        CHECK (placement_status IN ('active', 'released', 'lost'))
);

CREATE UNIQUE INDEX uq_project_placements_active
    ON project_placements(tenant_id, project_id)
    WHERE placement_status = 'active';

CREATE INDEX idx_project_placements_runtime_node
    ON project_placements(tenant_id, runtime_node_id, placement_status);

CREATE TABLE project_task_attestations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    attempt_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    provider_session_id VARCHAR(255),
    attestation_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    command_argv JSONB NOT NULL DEFAULT '[]'::jsonb,
    exit_code INTEGER,
    duration_ms BIGINT,
    log_ref TEXT,
    stdout_sha256 VARCHAR(64),
    stderr_sha256 VARCHAR(64),
    artifact_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    artifact_hashes JSONB NOT NULL DEFAULT '{}'::jsonb,
    git_branch VARCHAR(255),
    git_base_ref VARCHAR(255),
    git_head_sha VARCHAR(64),
    git_diff_sha256 VARCHAR(64),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_task_attestations_task
        FOREIGN KEY (tenant_id, project_id, project_task_id)
        REFERENCES project_tasks(tenant_id, project_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_project_task_attestations_attempt
        FOREIGN KEY (tenant_id, project_task_id, attempt_id)
        REFERENCES project_task_attempts(tenant_id, project_task_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_project_task_attestations_status
        CHECK (status IN ('succeeded', 'failed', 'cancelled', 'timed_out'))
);

CREATE UNIQUE INDEX uq_project_task_attestations_idempotency
    ON project_task_attestations(tenant_id, attempt_id, idempotency_key);

CREATE INDEX idx_project_task_attestations_task_created
    ON project_task_attestations(tenant_id, project_id, project_task_id, created_at DESC);

ALTER TABLE project_task_attempts
    ADD COLUMN budget_wall_clock_limit_sec INTEGER,
    ADD COLUMN budget_last_heartbeat_at TIMESTAMPTZ,
    ADD COLUMN budget_consumed_wall_clock_sec INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN budget_consumed_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN budget_tripped_at TIMESTAMPTZ,
    ADD COLUMN budget_trip_reason VARCHAR(100);

CREATE TRIGGER update_project_placements_updated_at
    BEFORE UPDATE ON project_placements
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_project_task_attestations_updated_at
    BEFORE UPDATE ON project_task_attestations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON COLUMN projects.repo_url IS '项目 Phase 1 源码仓库 URL；为空表示项目不绑定源码。';
COMMENT ON COLUMN projects.repo_default_branch IS '项目源码默认 base ref，不保存本地绝对路径。';
COMMENT ON COLUMN projects.repo_git_credential_ref IS '项目级 git 凭据引用，与员工 MCP 凭据分离。';
COMMENT ON COLUMN projects.repo_scope IS '仓库 sparse checkout scope，必须由业务侧保证包含传递依赖闭包。';
COMMENT ON TABLE project_placements IS '项目到 Runtime 节点的动态亲和放置状态，不保存本地绝对路径。';
COMMENT ON TABLE project_task_attestations IS 'Runtime 对项目任务执行的结构化证明，保存命令、退出码、日志哈希、产物哈希和 git 证据引用。';
COMMENT ON COLUMN project_task_attempts.budget_wall_clock_limit_sec IS '该尝试的墙钟预算上限，空表示未设置。';
COMMENT ON COLUMN project_task_attempts.budget_trip_reason IS '预算熔断原因，例如 wall_clock_exceeded、token_limit_exceeded。';
```

- [ ] **Step 4: Add sqlc queries**

Create `apps/control-plane/internal/storage/queries/project_runtime_affinity.sql`:

```sql
-- name: GetProjectRepoBinding :one
SELECT
    id,
    tenant_id,
    repo_url,
    repo_default_branch,
    repo_git_credential_ref,
    repo_scope,
    repo_binding_status
FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('project_id')::uuid;

-- name: UpsertProjectPlacement :one
INSERT INTO project_placements (
    tenant_id,
    project_id,
    runtime_node_id,
    placement_status,
    placement_reason
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    'active',
    sqlc.narg('placement_reason')::varchar
)
ON CONFLICT (tenant_id, project_id) WHERE placement_status = 'active'
DO UPDATE SET
    runtime_node_id = EXCLUDED.runtime_node_id,
    placement_reason = EXCLUDED.placement_reason,
    assigned_at = NOW(),
    updated_at = NOW()
RETURNING *;

-- name: GetActiveProjectPlacement :one
SELECT *
FROM project_placements
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND placement_status = 'active';

-- name: CreateProjectTaskAttestation :one
INSERT INTO project_task_attestations (
    tenant_id,
    project_id,
    project_task_id,
    attempt_id,
    runtime_node_id,
    provider_session_id,
    attestation_type,
    status,
    command_argv,
    exit_code,
    duration_ms,
    log_ref,
    stdout_sha256,
    stderr_sha256,
    artifact_refs,
    artifact_hashes,
    git_branch,
    git_base_ref,
    git_head_sha,
    git_diff_sha256,
    metadata,
    idempotency_key
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.arg('attempt_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.narg('provider_session_id')::varchar,
    sqlc.arg('attestation_type')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('command_argv')::jsonb, '[]'::jsonb),
    sqlc.narg('exit_code')::integer,
    sqlc.narg('duration_ms')::bigint,
    sqlc.narg('log_ref')::text,
    sqlc.narg('stdout_sha256')::varchar,
    sqlc.narg('stderr_sha256')::varchar,
    COALESCE(sqlc.narg('artifact_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('artifact_hashes')::jsonb, '{}'::jsonb),
    sqlc.narg('git_branch')::varchar,
    sqlc.narg('git_base_ref')::varchar,
    sqlc.narg('git_head_sha')::varchar,
    sqlc.narg('git_diff_sha256')::varchar,
    COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb),
    sqlc.arg('idempotency_key')::varchar
)
ON CONFLICT (tenant_id, attempt_id, idempotency_key) DO UPDATE SET
    idempotency_key = EXCLUDED.idempotency_key
RETURNING *;

-- name: ListProjectTaskAttestations :many
SELECT *
FROM project_task_attestations
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProjectTaskAttemptBudgetHeartbeat :one
UPDATE project_task_attempts
SET
    budget_last_heartbeat_at = NOW(),
    budget_consumed_wall_clock_sec = sqlc.arg('consumed_wall_clock_sec')::integer,
    budget_consumed_tokens = sqlc.arg('consumed_tokens')::integer,
    budget_tripped_at = CASE
        WHEN sqlc.narg('trip_reason')::varchar IS NULL THEN budget_tripped_at
        ELSE NOW()
    END,
    budget_trip_reason = COALESCE(sqlc.narg('trip_reason')::varchar, budget_trip_reason)
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('attempt_id')::uuid
RETURNING *;
```

- [ ] **Step 5: Run sqlc generation**

Run:

```bash
cd apps/control-plane && go generate ./internal/storage/queries
```

Expected: generated query files update without errors.

- [ ] **Step 6: Update Atlas checksum**

Run:

```bash
cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations
```

Expected: `internal/storage/migrations/atlas.sum` includes `039_project_repo_binding_and_attestation.sql`.

- [ ] **Step 7: Run storage verification**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestProjectRepoBindingAndAttestationMigration -count=1
go test ./apps/control-plane/internal/storage/queries -run 'ProjectRepoBinding|ProjectTaskAttestation|BudgetHeartbeat' -count=1
```

Expected: first command PASS. Second command PASS if query integration DB env is configured, or exits with the repository's documented skip behavior.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/039_project_repo_binding_and_attestation.sql apps/control-plane/internal/storage/queries/project_runtime_affinity.sql apps/control-plane/internal/storage/migrations_test.go apps/control-plane/internal/storage/queries apps/control-plane/internal/storage/migrations/atlas.sum
git commit -m "feat: add project repo attestation storage"
```

### Task 2: Control Plane Domain And API For Project Repo Binding

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Modify generated API files under `apps/control-plane/internal/api/gen/` and `apps/control-plane/gen/`

- [ ] **Step 1: Write failing service tests for repo binding validation**

Add tests to `apps/control-plane/internal/project/service_test.go`:

```go
func TestCreateProjectAcceptsNullableRepoBinding(t *testing.T) {
	repo := newMemoryRepository()
	service := NewService(repo)
	tenantID := uuid.New()
	ownerID := uuid.New()

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "Runtime affinity",
		HumanOwnerUserID: ownerID,
		RepoBinding: &ProjectRepoBindingInput{
			URL:              "https://github.com/acme/app.git",
			DefaultBranch:    "main",
			GitCredentialRef: "secret://git/acme",
			Scope:           []string{"apps/web", "packages/shared"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, ProjectRepoBindingStatusBound, created.Project.RepoBinding.Status)
	require.Equal(t, "https://github.com/acme/app.git", created.Project.RepoBinding.URL)
	require.Equal(t, []string{"apps/web", "packages/shared"}, created.Project.RepoBinding.Scope)
}

func TestCreateProjectRejectsPartialRepoBinding(t *testing.T) {
	repo := newMemoryRepository()
	service := NewService(repo)
	tenantID := uuid.New()
	ownerID := uuid.New()

	_, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "Broken repo binding",
		HumanOwnerUserID: ownerID,
		RepoBinding: &ProjectRepoBindingInput{
			URL: "https://github.com/acme/app.git",
		},
	})
	require.ErrorIs(t, err, ErrInvalidProject)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestCreateProject.*RepoBinding' -count=1
```

Expected: FAIL because repo binding domain types and request fields do not exist.

- [ ] **Step 3: Add domain types**

In `apps/control-plane/internal/project/types.go`, add:

```go
type ProjectRepoBindingStatus string

const (
	ProjectRepoBindingStatusUnbound ProjectRepoBindingStatus = "unbound"
	ProjectRepoBindingStatusBound   ProjectRepoBindingStatus = "bound"
)

type ProjectRepoBinding struct {
	Status           ProjectRepoBindingStatus
	URL              string
	DefaultBranch    string
	GitCredentialRef *string
	Scope            []string
}

type ProjectRepoBindingInput struct {
	URL              string   `json:"url,omitempty"`
	DefaultBranch    string   `json:"default_branch,omitempty"`
	GitCredentialRef string   `json:"git_credential_ref,omitempty"`
	Scope            []string `json:"scope,omitempty"`
}
```

Add `RepoBinding ProjectRepoBinding` to `Project`.

Add `RepoBinding *ProjectRepoBindingInput` to both `CreateProjectRequest` and `UpdateProjectConfigRequest`.

- [ ] **Step 4: Implement repo binding normalization**

In `apps/control-plane/internal/project/service.go`, add:

```go
func normalizeProjectRepoBinding(input *ProjectRepoBindingInput) (ProjectRepoBinding, error) {
	if input == nil {
		return ProjectRepoBinding{Status: ProjectRepoBindingStatusUnbound}, nil
	}
	url := strings.TrimSpace(input.URL)
	branch := strings.TrimSpace(input.DefaultBranch)
	credential := strings.TrimSpace(input.GitCredentialRef)
	scope := nonBlankUniqueStrings(input.Scope)
	if url == "" && branch == "" && credential == "" && len(scope) == 0 {
		return ProjectRepoBinding{Status: ProjectRepoBindingStatusUnbound}, nil
	}
	if url == "" || branch == "" {
		return ProjectRepoBinding{}, ErrInvalidProject
	}
	binding := ProjectRepoBinding{
		Status:        ProjectRepoBindingStatusBound,
		URL:           url,
		DefaultBranch: branch,
		Scope:         scope,
	}
	if credential != "" {
		binding.GitCredentialRef = &credential
	}
	return binding, nil
}

func nonBlankUniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
```

Use the helper in `CreateProject` and `UpdateProjectConfig` before calling the repository.

- [ ] **Step 5: Thread repo binding through repository requests and SQL mappings**

In `apps/control-plane/internal/project/repository.go`, add `RepoBinding ProjectRepoBinding` to `CreateProjectRequest` and `UpdateProjectConfigRequest`.

Update `apps/control-plane/internal/storage/queries/project.sql`:

```sql
-- Add to CreateProject INSERT columns:
repo_url,
repo_default_branch,
repo_git_credential_ref,
repo_scope,
repo_binding_status

-- Add to CreateProject VALUES:
sqlc.narg('repo_url')::text,
sqlc.narg('repo_default_branch')::varchar,
sqlc.narg('repo_git_credential_ref')::varchar,
COALESCE(sqlc.narg('repo_scope')::jsonb, '[]'::jsonb),
sqlc.arg('repo_binding_status')::varchar
```

Update `UpdateProjectConfig` query in the same file to set the same fields from args.

In `apps/control-plane/internal/project/pg_repository.go`, add mapping helpers:

```go
func repoBindingParams(binding ProjectRepoBinding) (repoURL *string, branch *string, credential *string, scope []byte, status string, err error) {
	status = string(binding.Status)
	if status == "" {
		status = string(ProjectRepoBindingStatusUnbound)
	}
	scope, err = jsonbArrayFromStrings(binding.Scope, "repo_scope")
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	if binding.Status == ProjectRepoBindingStatusBound {
		repoURL = stringPtr(binding.URL)
		branch = stringPtr(binding.DefaultBranch)
		credential = binding.GitCredentialRef
	}
	return repoURL, branch, credential, scope, status, nil
}

func projectRepoBindingFromRecord(repoURL, branch, credential *string, scope []byte, status string) (ProjectRepoBinding, error) {
	values, err := stringSliceFromJSON(scope)
	if err != nil {
		return ProjectRepoBinding{}, err
	}
	return ProjectRepoBinding{
		Status:           ProjectRepoBindingStatus(status),
		URL:              stringValue(repoURL),
		DefaultBranch:    stringValue(branch),
		GitCredentialRef: credential,
		Scope:            values,
	}, nil
}
```

Use those helpers in create/update params and `projectFromRecord`.

- [ ] **Step 6: Update HTTP request and response mapping**

In `apps/control-plane/internal/project/handler.go`, add to create/update request bodies:

```go
RepoBinding *ProjectRepoBindingInput `json:"repo_binding,omitempty"`
```

Pass it to service requests. Add to `projectResponse`:

```go
RepoBinding projectRepoBindingResponse `json:"repo_binding"`
```

with:

```go
type projectRepoBindingResponse struct {
	Status           string   `json:"status"`
	URL              string   `json:"url,omitempty"`
	DefaultBranch    string   `json:"default_branch,omitempty"`
	GitCredentialRef *string  `json:"git_credential_ref,omitempty"`
	Scope            []string `json:"scope,omitempty"`
}
```

- [ ] **Step 7: Update OpenAPI**

In `contracts/control-plane/openapi.yaml`, add `ProjectRepoBinding`, add `repo_binding` to create/update project request schemas and project response schema.

Run:

```bash
corepack pnpm generate:control-plane
```

Expected: generated API code updates without errors.

- [ ] **Step 8: Run focused tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'RepoBinding|CreateProject|UpdateProjectConfig' -count=1
go test ./apps/control-plane/internal/api -run ProjectRoutes -count=1
corepack pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/storage/queries contracts/control-plane/openapi.yaml apps/control-plane/internal/api/gen apps/control-plane/gen
git commit -m "feat: expose project repo binding"
```

### Task 3: Dispatch Metadata For Role-Derived Workspace Mode

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/task_type_defaults.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Modify: `apps/control-plane/internal/employee/run_service.go`
- Modify: `apps/control-plane/internal/employee/run_service_test.go`

- [ ] **Step 1: Write failing workspace-mode derivation tests**

Add to `apps/control-plane/internal/workflow/projectcoordination/task_type_defaults_test.go`:

```go
func TestWorkspaceModeForTaskKind(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want string
	}{
		{name: "feature development uses branch", kind: "feature_development", want: "branch"},
		{name: "review uses diff", kind: "code_review", want: "diff"},
		{name: "test uses detached run", kind: "test_verification", want: "detached_run"},
		{name: "analysis uses readonly", kind: "incident_triage", want: "readonly"},
		{name: "report uses none", kind: "status_report", want: "none"},
		{name: "unknown defaults to none", kind: "custom_coordination", want: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, WorkspaceModeForTaskKind(tt.kind))
		})
	}
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestWorkspaceModeForTaskKind -count=1
```

Expected: FAIL because `WorkspaceModeForTaskKind` does not exist.

- [ ] **Step 3: Implement workspace mode helper**

In `apps/control-plane/internal/workflow/projectcoordination/task_type_defaults.go`, add:

```go
const (
	WorkspaceModeNone        = "none"
	WorkspaceModeReadonly    = "readonly"
	WorkspaceModeDiff        = "diff"
	WorkspaceModeDetachedRun = "detached_run"
	WorkspaceModeBranch      = "branch"
)

var taskKindWorkspaceModes = map[string]string{
	"feature_development":    WorkspaceModeBranch,
	"feature_implementation": WorkspaceModeBranch,
	"software_development":   WorkspaceModeBranch,
	"code_implementation":    WorkspaceModeBranch,
	"bugfix":                 WorkspaceModeBranch,
	"code_review":            WorkspaceModeDiff,
	"review":                 WorkspaceModeDiff,
	"test_verification":      WorkspaceModeDetachedRun,
	"testing":                WorkspaceModeDetachedRun,
	"build_verification":     WorkspaceModeDetachedRun,
	"database_analysis":      WorkspaceModeReadonly,
	"incident_triage":        WorkspaceModeReadonly,
	"analysis":               WorkspaceModeReadonly,
	"status_report":          WorkspaceModeNone,
	"human_approval":         WorkspaceModeNone,
	"acceptance_summary":     WorkspaceModeNone,
}

func WorkspaceModeForTaskKind(kind string) string {
	normalized := normalizePlanningString(kind)
	if normalized == "" {
		return WorkspaceModeNone
	}
	if canonical := canonicalTaskKind(normalized); canonical != "" {
		normalized = canonical
	}
	if mode, ok := taskKindWorkspaceModes[normalized]; ok {
		return mode
	}
	return WorkspaceModeNone
}
```

- [ ] **Step 4: Write failing dispatch metadata test**

In `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`, add:

```go
func TestDispatchProjectTaskIncludesRepoBindingAndWorkspaceMode(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	attemptID := projectTaskDispatchAttemptID(taskID, 1)
	repo := newProjectStoreMemoryRepository(t)
	repo.projects = append(repo.projects, project.Project{
		ID: projectID, TenantID: tenantID, Name: "Code project", HumanOwnerUserID: uuid.New(),
		RepoBinding: project.ProjectRepoBinding{
			Status:        project.ProjectRepoBindingStatusBound,
			URL:           "https://github.com/acme/app.git",
			DefaultBranch: "main",
			Scope:         []string{"apps/web", "packages/shared"},
		},
	})
	repo.demands = append(repo.demands, project.ProjectDemand{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "Fix login"})
	repo.tasks = append(repo.tasks, project.ProjectTask{
		ID: taskID, TenantID: tenantID, ProjectID: projectID, DemandID: &repo.demands[0].ID,
		Title: "Implement fix", Status: project.ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID, TaskKind: stringPtr("feature_development"),
	})
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID: uuid.New(), RuntimeTaskID: uuid.New(), RuntimeNodeID: uuid.New(), NodeID: "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID: tenantID, ProjectID: projectID, TaskID: taskID, DispatchReason: project.DispatchReasonRootReady,
	})

	require.NoError(t, err)
	require.Len(t, starter.requests, 1)
	require.Equal(t, "branch", starter.requests[0].Metadata["workspace_mode"])
	require.Equal(t, "main", starter.requests[0].Metadata["base_ref"])
	require.Equal(t, attemptID.String(), starter.requests[0].Metadata["project_task_attempt_id"])
	require.Equal(t, map[string]any{
		"url":            "https://github.com/acme/app.git",
		"default_branch": "main",
		"scope":          []any{"apps/web", "packages/shared"},
	}, starter.requests[0].Metadata["project_git"])
}
```

- [ ] **Step 5: Run the dispatch test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'WorkspaceMode|DispatchProjectTaskIncludesRepoBinding' -count=1
```

Expected: workspace helper passes after Step 3; dispatch metadata test FAILS until metadata is added.

- [ ] **Step 6: Add metadata to run starter request**

In `apps/control-plane/internal/workflow/projectcoordination/types.go`, extend `StartProjectTaskRunRequest`:

```go
WorkspaceMode string
BaseRef       string
ProjectGit    map[string]any
```

In `project_store.go`, inside `DispatchProjectTask`, compute:

```go
workspaceMode := WorkspaceModeForTaskKind(stringValue(task.TaskKind))
projectGit := projectGitMetadata(projectRecord.RepoBinding)
baseRef := ""
if projectRecord.RepoBinding.Status == project.ProjectRepoBindingStatusBound {
	baseRef = projectRecord.RepoBinding.DefaultBranch
}
if projectRecord.RepoBinding.Status != project.ProjectRepoBindingStatusBound {
	workspaceMode = WorkspaceModeNone
}
runMetadata["workspace_mode"] = workspaceMode
runMetadata["base_ref"] = baseRef
if projectGit != nil {
	runMetadata["project_git"] = projectGit
}
```

Add:

```go
func projectGitMetadata(binding project.ProjectRepoBinding) map[string]any {
	if binding.Status != project.ProjectRepoBindingStatusBound {
		return nil
	}
	values := map[string]any{
		"url":            binding.URL,
		"default_branch": binding.DefaultBranch,
	}
	if binding.GitCredentialRef != nil && strings.TrimSpace(*binding.GitCredentialRef) != "" {
		values["git_credential_ref"] = strings.TrimSpace(*binding.GitCredentialRef)
	}
	scope := make([]any, 0, len(binding.Scope))
	for _, item := range binding.Scope {
		item = strings.TrimSpace(item)
		if item != "" {
			scope = append(scope, item)
		}
	}
	if len(scope) > 0 {
		values["scope"] = scope
	}
	return values
}
```

Pass `WorkspaceMode`, `BaseRef`, and `ProjectGit` into `StartProjectTaskRunRequest`.

- [ ] **Step 7: Thread metadata through app adapter and employee payload**

In `apps/control-plane/internal/app/app.go`, keep `Metadata: req.Metadata` and add nothing else unless tests require typed fields. The metadata is the stable transport boundary.

In `apps/control-plane/internal/employee/run_service.go`, assert `metadata` is included in `computeRunIdempotencyFingerprint`, `buildRunParams`, and `buildStartSessionPayload` already. Add focused test coverage that `workspace_mode`, `base_ref`, and `project_git` survive into the runtime command payload.

- [ ] **Step 8: Run focused tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'WorkspaceMode|DispatchProjectTaskIncludesRepoBinding' -count=1
go test ./apps/control-plane/internal/employee -run 'StartSessionPayload|IdempotencyFingerprint' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination apps/control-plane/internal/app/app.go apps/control-plane/internal/employee/run_service.go apps/control-plane/internal/employee/run_service_test.go
git commit -m "feat: dispatch project workspace metadata"
```

### Task 4: Runtime Payload Contract For Agent Home And Project Workspace

> **Pre-read:** `agent_home_dir` already exists on the start-session payload as a required field, and today `ensure_command_instance` returns it *as* the run `workspace_path` — i.e. the provider CWD currently equals the agent home. The new work in this task is NOT a new payload field. It is: (1) a typed `project_workspace()` accessor over `metadata`, and (2) adding `agent_home_dir` to `RunSpec`/`RunSnapshot`/`ProviderRequest` so the provider CWD (worktree, set in Task 5) can diverge from the agent home (config/skills/MCP).

**Files:**
- Modify: `apps/runtime-agent/src/commands/payload.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/src/runs.rs`
- Modify: `apps/runtime-agent/src/providers/mod.rs`
- Modify: `apps/runtime-agent/src/providers/claude.rs`
- Modify: `apps/runtime-agent/src/providers/codex.rs`
- Modify: `apps/runtime-agent/src/providers/opencode.rs`

- [ ] **Step 1: Write failing payload deserialization test**

Add to `apps/runtime-agent/src/commands/payload.rs` tests:

```rust
#[test]
fn test_project_workspace_metadata_deserializes() {
    let raw = r#"{
        "command_id":"cmd-test",
        "tenant_id":"00000000-0000-4000-8000-000000000001",
        "digital_employee_id":"35a3799b-7665-4913-9097-35ee53d30e74",
        "execution_instance_id":"8e64dd8c-d70d-417d-b8bf-fe57a61f4205",
        "runtime_node_id":"44444444-4444-4444-8444-444444444444",
        "provider_type":"codex",
        "agent_home_dir":"/tmp/workspaces/employees/35a3799b-7665-4913-9097-35ee53d30e74",
        "workspace_files":[],
        "skills":[],
        "mcp_servers":[],
        "environment":[],
        "session_policy":{"mode":"new"},
        "prompt":"hello",
        "input":"hello",
        "context_refs":[],
        "artifact_refs":[],
        "metadata":{
            "workspace_mode":"branch",
            "base_ref":"main",
            "project_id":"11111111-1111-4111-8111-111111111111",
            "project_task_id":"22222222-2222-4222-8222-222222222222",
            "project_task_attempt_id":"33333333-3333-4333-8333-333333333333",
            "project_git":{"url":"https://github.com/acme/app.git","default_branch":"main","scope":["apps/web"]}
        }
    }"#;
    let command = RuntimeCommand {
        id: "cmd-test".to_string(),
        command_type: RuntimeCommandType::StartSession,
        payload: serde_json::from_str(raw).unwrap(),
    };
    let payload = RuntimeSessionCommandPayload::from_command(&command).unwrap();
    assert_eq!(payload.project_workspace().workspace_mode.as_deref(), Some("branch"));
    assert_eq!(payload.project_workspace().base_ref.as_deref(), Some("main"));
    assert_eq!(payload.project_workspace().project_git.as_ref().unwrap().url, "https://github.com/acme/app.git");
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_workspace_metadata_deserializes
```

Expected: FAIL because `project_workspace()` does not exist.

- [ ] **Step 3: Add typed metadata helpers**

In `apps/runtime-agent/src/commands/payload.rs`, add:

```rust
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct RuntimeProjectGitPayload {
    pub url: String,
    #[serde(default)]
    pub default_branch: Option<String>,
    #[serde(default)]
    pub git_credential_ref: Option<String>,
    #[serde(default)]
    pub scope: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct RuntimeProjectWorkspacePayload {
    pub project_id: Option<String>,
    pub project_task_id: Option<String>,
    pub attempt_id: Option<String>,
    pub workspace_mode: Option<String>,
    pub base_ref: Option<String>,
    pub project_git: Option<RuntimeProjectGitPayload>,
}

impl RuntimeSessionCommandPayload {
    pub fn project_workspace(&self) -> RuntimeProjectWorkspacePayload {
        RuntimeProjectWorkspacePayload {
            project_id: metadata_string(&self.metadata, "project_id"),
            project_task_id: metadata_string(&self.metadata, "project_task_id"),
            attempt_id: metadata_string(&self.metadata, "project_task_attempt_id"),
            workspace_mode: metadata_string(&self.metadata, "workspace_mode"),
            base_ref: metadata_string(&self.metadata, "base_ref"),
            project_git: self.metadata.get("project_git").cloned().and_then(|value| serde_json::from_value(value).ok()),
        }
    }
}

fn metadata_string(metadata: &serde_json::Value, key: &str) -> Option<String> {
    metadata.get(key)
        .and_then(|value| value.as_str())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}
```

- [ ] **Step 4: Extend run/provider request structs**

In `apps/runtime-agent/src/runs.rs`, add to `RunSpec` and `RunSnapshot`:

```rust
#[serde(default, skip_serializing_if = "Option::is_none")]
pub agent_home_dir: Option<PathBuf>,
```

In `apps/runtime-agent/src/providers/mod.rs`, add to `ProviderRequest`:

```rust
#[serde(default, skip_serializing_if = "Option::is_none")]
pub agent_home_dir: Option<PathBuf>,
```

Update `provider_request` in `commands/executor.rs`:

```rust
agent_home_dir: spec.agent_home_dir.clone(),
```

- [ ] **Step 5: Make provider adapters use agent home for config and worktree for cwd**

Update provider command builders:

```rust
// claude.rs
if let Some(agent_home) = &request.agent_home_dir {
    let mcp_config = agent_home.join(".mcp.json");
    if mcp_config.exists() {
        command.arg("--mcp-config").arg(mcp_config);
        command.arg("--strict-mcp-config");
    }
}
```

```rust
// codex.rs
// Do not derive CODEX_HOME from agent_home_dir by default.
// Explicit request.environment values are still passed through by apply_environment.
```

```rust
// opencode.rs
// Do not derive OPENCODE_CONFIG_DIR / OPENCODE_CONFIG from agent_home_dir by default.
// Explicit request.environment values are still passed through by apply_environment.
command.arg("--dir").arg(&request.workspace_path);
```

Keep `command.current_dir(&request.workspace_path)` for all providers.

- [ ] **Step 6: Run Rust focused tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_workspace_metadata_deserializes
cargo test --manifest-path apps/runtime-agent/Cargo.toml providers
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/runtime-agent/src/commands/payload.rs apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/src/runs.rs apps/runtime-agent/src/providers
git commit -m "feat: separate provider workspace and agent home"
```

### Task 5: Runtime Project Workspace Materialization

**Files:**
- Create: `apps/runtime-agent/src/project_workspace.rs`
- Modify: `apps/runtime-agent/src/lib.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/src/executor/workspace.rs`

- [ ] **Step 1: Write failing Rust tests for workspace resolution**

Create `apps/runtime-agent/src/project_workspace.rs` with tests first:

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn empty_workspace_when_no_repo_binding() {
        let temp = TempDir::new().unwrap();
        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().to_path_buf(),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some("33333333-3333-4333-8333-333333333333".to_string()),
            workspace_mode: "none".to_string(),
            project_git: None,
            base_ref: None,
        };
        let resolved = resolve_project_workspace(request).unwrap();
        assert!(resolved.workspace_path.ends_with("workspaces/11111111-1111-4111-8111-111111111111/22222222-2222-4222-8222-222222222222/33333333-3333-4333-8333-333333333333"));
        assert!(resolved.workspace_path.exists());
        assert_eq!(resolved.mode, "none");
    }

    #[test]
    fn rejects_branch_workspace_without_git() {
        let temp = TempDir::new().unwrap();
        let request = ProjectWorkspaceRequest {
            base_dir: temp.path().to_path_buf(),
            project_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            project_task_id: Some("22222222-2222-4222-8222-222222222222".to_string()),
            attempt_id: Some("33333333-3333-4333-8333-333333333333".to_string()),
            workspace_mode: "branch".to_string(),
            project_git: None,
            base_ref: Some("main".to_string()),
        };
        let err = resolve_project_workspace(request).unwrap_err().to_string();
        assert!(err.contains("project_git is required"));
    }
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_workspace
```

Expected: FAIL until structs and functions are implemented.

- [ ] **Step 3: Implement workspace resolver**

In `apps/runtime-agent/src/project_workspace.rs`, implement:

```rust
use std::path::PathBuf;
use anyhow::{Context, Result};

use crate::commands::payload::RuntimeProjectGitPayload;

#[derive(Debug, Clone)]
pub struct ProjectWorkspaceRequest {
    pub base_dir: PathBuf,
    pub project_id: Option<String>,
    pub project_task_id: Option<String>,
    pub attempt_id: Option<String>,
    pub workspace_mode: String,
    pub project_git: Option<RuntimeProjectGitPayload>,
    pub base_ref: Option<String>,
}

#[derive(Debug, Clone)]
pub struct ResolvedProjectWorkspace {
    pub workspace_path: PathBuf,
    pub repo_path: Option<PathBuf>,
    pub mode: String,
    pub base_ref: Option<String>,
}

pub fn resolve_project_workspace(request: ProjectWorkspaceRequest) -> Result<ResolvedProjectWorkspace> {
    let mode = normalize_workspace_mode(&request.workspace_mode);
    let project_id = request.project_id.as_deref().unwrap_or("unscoped");
    let task_id = request.project_task_id.as_deref().unwrap_or("manual");
    let attempt_id = request.attempt_id.as_deref().unwrap_or("attempt");
    validate_segment(project_id, "project_id")?;
    validate_segment(task_id, "project_task_id")?;
    validate_segment(attempt_id, "attempt_id")?;

    if mode != "none" && request.project_git.is_none() {
        anyhow::bail!("project_git is required for workspace_mode={mode}");
    }

    let workspace_path = request
        .base_dir
        .join("workspaces")
        .join(project_id)
        .join(task_id)
        .join(attempt_id);
    std::fs::create_dir_all(&workspace_path).context("create project task workspace")?;

    let repo_path = if let Some(git) = &request.project_git {
        let repo_path = request.base_dir.join("repos").join(project_id);
        ensure_repo_placeholder(&repo_path, git)?;
        Some(repo_path)
    } else {
        None
    };

    Ok(ResolvedProjectWorkspace {
        workspace_path,
        repo_path,
        mode,
        base_ref: request.base_ref,
    })
}

fn ensure_repo_placeholder(repo_path: &PathBuf, git: &RuntimeProjectGitPayload) -> Result<()> {
    if git.url.trim().is_empty() {
        anyhow::bail!("project_git.url is required");
    }
    std::fs::create_dir_all(repo_path).context("create project repo cache")?;
    Ok(())
}

fn normalize_workspace_mode(value: &str) -> String {
    match value.trim() {
        "readonly" | "diff" | "detached_run" | "branch" => value.trim().to_string(),
        _ => "none".to_string(),
    }
}

fn validate_segment(value: &str, field: &str) -> Result<()> {
    if value.contains('/') || value.contains('\\') || value == "." || value == ".." || value.trim().is_empty() {
        anyhow::bail!("{field} is not a safe path segment");
    }
    Ok(())
}
```

This first implementation creates the stable directory layout and validates inputs. Actual git clone/worktree commands are added in the next step after tests lock the boundary.

- [ ] **Step 4: Wire resolver into command execution**

Add `pub mod project_workspace;` to `apps/runtime-agent/src/lib.rs`.

In `ensure_command_instance` in `apps/runtime-agent/src/commands/executor.rs`, return a struct instead of only `PathBuf`:

```rust
#[derive(Debug, Clone)]
struct CommandWorkspace {
    agent_home_dir: PathBuf,
    workspace_path: PathBuf,
}
```

After validating `agent_home_dir`, call:

```rust
let project_workspace = payload.project_workspace();
let resolved = crate::project_workspace::resolve_project_workspace(crate::project_workspace::ProjectWorkspaceRequest {
    base_dir: self.config.workspace.base_dir.clone(),
    project_id: project_workspace.project_id,
    project_task_id: project_workspace.project_task_id,
    attempt_id: project_workspace.attempt_id,
    workspace_mode: project_workspace.workspace_mode.unwrap_or_else(|| "none".to_string()),
    project_git: project_workspace.project_git,
    base_ref: project_workspace.base_ref,
}).map_err(|error| self.recorded_error(command_id, error))?;
```

Materialize `workspace_files` into `agent_home_dir`, not task workspace, preserving the existing employee home semantics:

```rust
materialize_workspace(WorkspaceMaterializationPlan {
    agent_home_dir: agent_home_dir.clone(),
    provider_home,
    files: payload.workspace_files.clone(),
})?;
```

Return:

```rust
Ok(CommandWorkspace { agent_home_dir, workspace_path: resolved.workspace_path })
```

Set `RunSpec.workspace_path = command_workspace.workspace_path` and `RunSpec.agent_home_dir = Some(command_workspace.agent_home_dir)`.

- [ ] **Step 5: Add real git worktree materialization behind small functions**

Extend `ensure_repo_placeholder` into:

```rust
fn ensure_repo_cache(repo_path: &PathBuf, git: &RuntimeProjectGitPayload, base_ref: Option<&str>) -> Result<()> {
    if repo_path.join(".git").exists() {
        run_git(repo_path.parent().unwrap(), &["-C", repo_path.to_str().unwrap(), "fetch", "--prune", "origin"])?;
        return Ok(());
    }
    std::fs::create_dir_all(repo_path.parent().context("repo parent")?)?;
    run_git(repo_path.parent().unwrap(), &["clone", "--no-checkout", &git.url, repo_path.to_str().unwrap()])?;
    if let Some(base_ref) = base_ref {
        run_git(repo_path, &["fetch", "origin", base_ref])?;
    }
    Ok(())
}
```

Add mode-specific worktree setup:

```rust
fn materialize_git_worktree(repo_path: &PathBuf, workspace_path: &PathBuf, mode: &str, base_ref: Option<&str>, scope: &[String]) -> Result<()> {
    let base = base_ref.unwrap_or("HEAD");
    match mode {
        "branch" => {
            let branch = format!("st/{}/{}", workspace_path.file_name().unwrap().to_string_lossy(), "work");
            run_git(repo_path, &["worktree", "add", "-B", &branch, workspace_path.to_str().unwrap(), base])?;
        }
        "detached_run" | "readonly" | "diff" => {
            run_git(repo_path, &["worktree", "add", "--detach", workspace_path.to_str().unwrap(), base])?;
        }
        _ => {}
    }
    if !scope.is_empty() {
        run_git(workspace_path, &["sparse-checkout", "init", "--cone"])?;
        let mut args = vec!["sparse-checkout", "set"];
        for item in scope {
            args.push(item);
        }
        run_git(workspace_path, &args)?;
    }
    Ok(())
}
```

Add `run_git` with `std::process::Command`, captured stderr, and no shell interpolation.

- [ ] **Step 6: Add provider skill symlinks into task workspace**

In `project_workspace.rs`, add:

```rust
pub fn link_provider_skills(agent_home_dir: &PathBuf, workspace_path: &PathBuf, provider_type: &str) -> Result<()> {
    let links: &[(&str, &str)] = match provider_type {
        "claude-code" | "claude" => &[(".claude/skills", ".claude/skills")],
        "codex" => &[(".agents/skills", ".agents/skills")],
        "opencode" => &[(".opencode/skills", ".opencode/skills")],
        _ => &[],
    };
    for (home_rel, workspace_rel) in links {
        let source = agent_home_dir.join(home_rel);
        if !source.exists() {
            continue;
        }
        let target = workspace_path.join(workspace_rel);
        if let Some(parent) = target.parent() {
            std::fs::create_dir_all(parent)?;
        }
        if target.exists() {
            continue;
        }
        #[cfg(unix)]
        std::os::unix::fs::symlink(&source, &target)?;
    }
    Ok(())
}
```

Call it after workspace resolution in `ensure_command_instance`.

- [ ] **Step 7: Run runtime tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_workspace
cargo test --manifest-path apps/runtime-agent/Cargo.toml commands::executor
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/runtime-agent/src/project_workspace.rs apps/runtime-agent/src/lib.rs apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/src/executor/workspace.rs
git commit -m "feat: materialize project task workspaces"
```

### Task 6: Runtime Attestation Capture And Writeback

**Files:**
- Create: `apps/runtime-agent/src/attestation.rs`
- Modify: `apps/runtime-agent/src/lib.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [ ] **Step 1: Write failing Rust attestation unit test**

Create `apps/runtime-agent/src/attestation.rs`:

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn hashes_log_files_for_attestation() {
        let temp = TempDir::new().unwrap();
        let stdout_path = temp.path().join("stdout.log");
        let stderr_path = temp.path().join("stderr.log");
        std::fs::write(&stdout_path, b"ok\n").unwrap();
        std::fs::write(&stderr_path, b"").unwrap();

        let record = CommandAttestation::from_logs(
            vec!["cargo".to_string(), "test".to_string()],
            Some(0),
            12,
            &stdout_path,
            &stderr_path,
        ).unwrap();

        assert_eq!(record.exit_code, Some(0));
        assert_eq!(record.status, "succeeded");
        assert_eq!(record.stdout_sha256.len(), 64);
        assert_eq!(record.stderr_sha256.len(), 64);
    }
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml hashes_log_files_for_attestation
```

Expected: FAIL until `CommandAttestation` is implemented.

- [ ] **Step 3: Implement attestation structs**

In `apps/runtime-agent/src/attestation.rs`:

```rust
use std::path::Path;
use anyhow::Result;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CommandAttestation {
    pub attestation_type: String,
    pub status: String,
    pub command_argv: Vec<String>,
    pub exit_code: Option<i32>,
    pub duration_ms: i64,
    pub stdout_sha256: String,
    pub stderr_sha256: String,
}

impl CommandAttestation {
    pub fn from_logs(argv: Vec<String>, exit_code: Option<i32>, duration_ms: i64, stdout_path: &Path, stderr_path: &Path) -> Result<Self> {
        let status = match exit_code {
            Some(0) => "succeeded",
            Some(_) => "failed",
            None => "cancelled",
        };
        Ok(Self {
            attestation_type: "provider_session".to_string(),
            status: status.to_string(),
            command_argv: argv,
            exit_code,
            duration_ms,
            stdout_sha256: sha256_file(stdout_path)?,
            stderr_sha256: sha256_file(stderr_path)?,
        })
    }
}

pub fn sha256_file(path: &Path) -> Result<String> {
    let bytes = std::fs::read(path)?;
    let mut hasher = Sha256::new();
    hasher.update(bytes);
    Ok(format!("{:x}", hasher.finalize()))
}
```

Add `sha2` to `apps/runtime-agent/Cargo.toml` if it is not already present.

- [ ] **Step 4: Add control-plane attestation writeback model**

In `apps/runtime-agent/src/controlplane/models.rs`, add:

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProjectTaskAttestationWriteback {
    pub tenant_id: String,
    pub project_id: String,
    pub project_task_id: String,
    pub attempt_id: String,
    pub runtime_node_id: String,
    pub provider_session_id: Option<String>,
    pub attestation_type: String,
    pub status: String,
    pub command_argv: Vec<String>,
    pub exit_code: Option<i32>,
    pub duration_ms: Option<i64>,
    pub log_ref: Option<String>,
    pub stdout_sha256: Option<String>,
    pub stderr_sha256: Option<String>,
    pub artifact_refs: Vec<serde_json::Value>,
    pub artifact_hashes: serde_json::Value,
    pub git_branch: Option<String>,
    pub git_base_ref: Option<String>,
    pub git_head_sha: Option<String>,
    pub git_diff_sha256: Option<String>,
    pub metadata: serde_json::Value,
    pub idempotency_key: String,
}
```

In `apps/runtime-agent/src/controlplane/client.rs`, add:

```rust
pub async fn create_project_task_attestation(&self, body: &ProjectTaskAttestationWriteback) -> anyhow::Result<()> {
    self.post_json("/runtime/project-task-attestations", body).await.map(|_| ())
}
```

Use the existing client request helper naming in the file; do not invent a second HTTP stack.

- [ ] **Step 5: Add Control Plane domain/service/handler for attestation**

In `apps/control-plane/internal/project/types.go`, add `ProjectTaskAttestation` and `CreateProjectTaskAttestationRequest` mirroring migration fields.

In `repository.go`, add:

```go
CreateProjectTaskAttestation(ctx context.Context, req CreateProjectTaskAttestationRequest) (ProjectTaskAttestation, error)
ListProjectTaskAttestations(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID, limit, offset int32) ([]ProjectTaskAttestation, error)
```

Implement in `pg_repository.go` using generated sqlc queries.

In `service.go`, add this attestation writeback method:

```go
func (s *Service) CreateProjectTaskAttestation(ctx context.Context, req CreateProjectTaskAttestationRequest) (*ProjectTaskAttestation, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.AttemptID == uuid.Nil || req.RuntimeNodeID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if strings.TrimSpace(req.AttestationType) == "" || strings.TrimSpace(req.Status) == "" || strings.TrimSpace(req.IdempotencyKey) == "" {
		return nil, ErrInvalidProjectEvidence
	}
	attestation, err := s.repository.CreateProjectTaskAttestation(ctx, req)
	if err != nil {
		return nil, err
	}
	return &attestation, nil
}
```

In `handler.go`, add a runtime-authenticated handler `CreateProjectTaskAttestation` that decodes the writeback body and calls the service. Register it near existing runtime ProjectTask attempt routes.

- [ ] **Step 6: Capture provider command attestation in executor**

In `apps/runtime-agent/src/commands/executor.rs`, when `provider.start(provider_request(&spec))` returns or errors, record a best-effort attestation using:

```rust
if let Some(project_task) = project_task.as_ref() {
    if let Some(control_plane) = &self.control_plane {
        let body = build_provider_start_attestation(project_task, &payload, &spec, "provider_start", None);
        let _ = control_plane.create_project_task_attestation(&body).await;
    }
}
```

For Phase 1, record at least provider invocation metadata and terminal status. Full stdout/stderr file hashing is acceptable as follow-up if current provider stream does not expose raw files. Do not claim command-level shell attestation until command execution hooks exist.

- [ ] **Step 7: Run tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml attestation
go test ./apps/control-plane/internal/project -run Attestation -count=1
go test ./apps/control-plane/internal/api -run Attestation -count=1
corepack pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/runtime-agent apps/control-plane/internal/project apps/control-plane/internal/api contracts/control-plane/openapi.yaml
git commit -m "feat: record runtime task attestations"
```

### Task 7: Require Attestation For Successful Verification Results

**Files:**
- Modify: `apps/control-plane/internal/project/task_result_contract.go`
- Modify: `apps/control-plane/internal/project/task_result_contract_test.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Write failing contract tests**

Add to `apps/control-plane/internal/project/task_result_contract_test.go`:

```go
func TestCompletedVerificationRequiresRuntimeAttestationRef(t *testing.T) {
	task := ProjectTask{
		ID: uuid.New(),
		ExpectedOutputs: []any{"verification"},
		HandoffContract: map[string]any{
			"requires_runtime_attestation": true,
		},
	}
	result := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "tests passed",
		Verification: []TaskResultVerification{{
			Status:  TaskResultVerificationStatusPassed,
			Type:    "unit_test",
			Summary: "go test passed",
		}},
	}

	validation := ValidateTaskResultContract(task, result)

	require.False(t, validation.Valid)
	require.Contains(t, validation.Errors, "verification_attestation_ref_required")
}

func TestCompletedVerificationAcceptsRuntimeAttestationRef(t *testing.T) {
	task := ProjectTask{
		ID: uuid.New(),
		ExpectedOutputs: []any{"verification"},
		HandoffContract: map[string]any{
			"requires_runtime_attestation": true,
		},
	}
	result := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "tests passed",
		Verification: []TaskResultVerification{{
			Status:  TaskResultVerificationStatusPassed,
			Type:    "unit_test",
			Summary: "go test passed",
			EvidenceRefs: []TaskResultRef{{
				Kind: "attestation",
				Type: "runtime_command",
				Ref:  "attestation:123",
			}},
		}},
	}

	validation := ValidateTaskResultContract(task, result)

	require.True(t, validation.Valid, "unexpected errors: %#v", validation.Errors)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project -run CompletedVerificationRequiresRuntimeAttestationRef -count=1
```

Expected: FAIL until validation is added.

- [ ] **Step 3: Add attestation validation**

In `task_result_contract.go`, update `validateTaskResultVerifications` to accept `task`:

```go
validation.Errors = append(validation.Errors, validateTaskResultVerifications(task, result.Status, result.Verification)...)
```

Implement:

```go
func validateTaskResultVerifications(task ProjectTask, status TaskResultStatus, verifications []TaskResultVerification) []string {
	var errors []string
	requiresRuntimeAttestation := boolFromTaskContract(task.HandoffContract, "requires_runtime_attestation")
	for _, verification := range verifications {
		switch verification.Status {
		case TaskResultVerificationStatusPassed, TaskResultVerificationStatusSkipped:
		case TaskResultVerificationStatusFailed:
			if status == TaskResultStatusCompleted {
				errors = append(errors, "verification_failed")
			}
		default:
			errors = append(errors, "verification_status_invalid:"+string(verification.Status))
		}
		if status == TaskResultStatusCompleted && verification.Status == TaskResultVerificationStatusPassed && requiresRuntimeAttestation && !verificationHasAttestationRef(verification) {
			errors = append(errors, "verification_attestation_ref_required")
		}
	}
	return errors
}

func verificationHasAttestationRef(verification TaskResultVerification) bool {
	for _, ref := range verification.EvidenceRefs {
		if strings.EqualFold(strings.TrimSpace(ref.Kind), "attestation") ||
			strings.EqualFold(strings.TrimSpace(ref.Type), "attestation") ||
			strings.HasPrefix(strings.TrimSpace(ref.Ref), "attestation:") {
			return usableTaskResultRef(ref)
		}
	}
	return false
}

func boolFromTaskContract(contract map[string]any, key string) bool {
	value, ok := contract[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}
```

- [ ] **Step 4: Ensure planner/dispatch sets attestation requirement for test/build/code tasks**

In `project_store.go`, before dispatching, set:

```go
if workspaceMode == WorkspaceModeBranch || workspaceMode == WorkspaceModeDetachedRun {
	handoffContract["requires_runtime_attestation"] = true
}
```

The Control Plane validator reads `requires_runtime_attestation` from `task.HandoffContract`, so attestation enforcement does not depend on dispatch metadata. But the Task 10 smoke (Step 4) asserts `handoff_contract` appears inside the runtime command receipt `metadata`. To make that hold, also thread it into `runMetadata` in `DispatchProjectTask` (same block as `workspace_mode`/`base_ref`/`project_git`):

```go
if len(handoffContract) > 0 {
	runMetadata["handoff_contract"] = handoffContract
}
```

Add a project store test that a `feature_development` task receives `requires_runtime_attestation: true` in both `task.HandoffContract` and dispatch `runMetadata["handoff_contract"]`.

- [ ] **Step 5: Run focused tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TaskResultContract|Attestation' -count=1
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'DispatchProjectTask.*Attestation|DispatchProjectTaskIncludesRepoBinding' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/task_result_contract.go apps/control-plane/internal/project/task_result_contract_test.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "feat: require attestation-backed verification"
```

### Task 8: Bounded Revision Iteration Loop

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Write failing ProjectStore revision task test**

In `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`, add:

```go
func TestApplyTaskResultRevisionCreatesBoundedRevisionTask(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	repo := newProjectStoreMemoryRepository(t)
	repo.tasks = append(repo.tasks, project.ProjectTask{
		ID: sourceTaskID, TenantID: tenantID, ProjectID: projectID,
		Title: "Implement login", Status: project.ProjectTaskStatusCompleted,
		AttemptCount: 1, MaxAttempts: int32Ptr(3),
		AssignedDigitalEmployeeID: uuidPtr(uuid.New()),
		PlannerMetadata: map[string]any{"iteration_key": "wi-login"},
		HandoffContract: map[string]any{"completion_path": "project_task_attempt_writeback"},
	})
	repo.projectTaskResults = append(repo.projectTaskResults, project.ProjectTaskResult{
		ID: resultID, TenantID: tenantID, ProjectID: projectID, ProjectTaskID: sourceTaskID,
		ResultStatus: project.TaskResultStatusRevisionNeeded,
		Decision: project.TaskResultDecisionRevisionAttempt,
		Contract: project.TaskResultContract{
			Status: project.TaskResultStatusRevisionNeeded,
			Summary: "tests failed",
			RevisionRequest: &project.TaskResultRevisionRequest{
				Reason: "login test failed",
				RequestedChanges: []string{"fix redirect"},
			},
		},
	})
	store := NewProjectStore(repo)

	created, err := store.CreateRevisionTaskForResult(context.Background(), CreateRevisionTaskForResultInput{
		TenantID: tenantID, ProjectID: projectID, SourceTaskID: sourceTaskID, ResultID: resultID,
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.TaskID)
	revision := repo.mustTask(created.TaskID)
	require.Equal(t, &sourceTaskID, revision.RevisionOfTaskID)
	require.Equal(t, "wi-login", revision.PlannerMetadata["iteration_key"])
	require.Equal(t, "login test failed", revision.InputRequirements["revision_reason"])
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestApplyTaskResultRevisionCreatesBoundedRevisionTask -count=1
```

Expected: FAIL because activity/types do not exist.

- [ ] **Step 3: Add activity types and ProjectStore method**

In `types.go`, add:

```go
type CreateRevisionTaskForResultInput struct {
	TenantID     uuid.UUID
	ProjectID    uuid.UUID
	SourceTaskID uuid.UUID
	ResultID     uuid.UUID
}

type CreateRevisionTaskForResultResult struct {
	TaskID uuid.UUID
}
```

Add to `ActivityStore` and `Activities`:

```go
CreateRevisionTaskForResult(ctx context.Context, input CreateRevisionTaskForResultInput) (CreateRevisionTaskForResultResult, error)
```

Implement in `project_store.go` by reading source task and result, checking `source.AttemptCount < *source.MaxAttempts` when max exists, creating a new ProjectTask with:

```go
RevisionOfTaskID:       &source.ID,
AssignedDigitalEmployeeID: source.AssignedDigitalEmployeeID,
TaskKind:               source.TaskKind,
RiskLevel:              source.RiskLevelValue(),
RequiresHumanApproval:  source.RequiresHumanApproval,
ExpectedOutputs:        append([]any(nil), source.ExpectedOutputs...),
InputRequirements: map[string]any{
	"revision_reason": result.Contract.RevisionRequest.Reason,
	"requested_changes": append([]string(nil), result.Contract.RevisionRequest.RequestedChanges...),
	"source_task_id": source.ID.String(),
	"source_result_id": result.ID.String(),
},
HandoffContract: cloneAnyMap(source.HandoffContract),
PlannerMetadata: revisionPlannerMetadata(source, result),
```

Status should be `planned`, so normal predispatch and dispatch gates apply.

- [ ] **Step 4: Wire workflow completion signal to revision loop**

In `workflow.go`, change `handleEmployeeTaskCompleted` to load the latest result decision for the completed task using a new activity:

```go
type InspectTaskResultDecisionInput struct { TenantID, ProjectID, ProjectTaskID uuid.UUID }
type InspectTaskResultDecisionResult struct {
	ResultID uuid.UUID
	Decision string
	Exhausted bool
}
```

If decision is `revision_attempt`, call `CreateRevisionTaskForResult`, then `dispatchProjectTasks` with `DispatchReasonRetry`.

If exhausted, request human decision with reason `iteration_exhausted`.

Keep existing downstream dispatch only for `complete_accepted`.

- [ ] **Step 5: Add no-progress guard**

In ProjectStore revision creation, calculate a failure fingerprint:

```go
fingerprint := revisionFailureFingerprint(result.Contract)
```

Store it in `PlannerMetadata["revision_failure_fingerprint"]`.

Before creating a new revision task, scan prior tasks with the same `revision_of_task_id` or same `iteration_key`; if the last two revision tasks have the same fingerprint, return a human-wait decision request instead of a task. Use `DecisionType: "project_task_iteration_exhausted"` and summary `"同一失败重复出现，需要人类判断是否继续"`.

- [ ] **Step 6: Run workflow tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'Revision|Iteration|EmployeeTaskCompleted' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "feat: add bounded project task revision loop"
```

### Task 9: Budget Heartbeat And Runtime Fuse

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`

- [ ] **Step 1: Write failing budget guard service test**

Add to `apps/control-plane/internal/project/service_test.go`:

```go
func TestProjectTaskAttemptBudgetHeartbeatTripsWallClock(t *testing.T) {
	repo := newMemoryRepository()
	service := NewService(repo)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{ID: taskID, TenantID: tenantID, ProjectID: projectID, Status: ProjectTaskStatusRunning})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID: attemptID, TenantID: tenantID, ProjectTaskID: taskID, Status: ProjectTaskAttemptStatusRunning,
		BudgetWallClockLimitSec: int32Ptr(10),
	})

	result, err := service.RecordProjectTaskAttemptBudgetHeartbeat(context.Background(), RecordProjectTaskAttemptBudgetHeartbeatRequest{
		TenantID: tenantID, ProjectID: projectID, ProjectTaskID: taskID, AttemptID: attemptID,
		ConsumedWallClockSec: 11,
	})

	require.NoError(t, err)
	require.True(t, result.Tripped)
	require.Equal(t, "wall_clock_exceeded", result.TripReason)
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestProjectTaskAttemptBudgetHeartbeatTripsWallClock -count=1
```

Expected: FAIL until budget heartbeat domain exists.

- [ ] **Step 3: Add budget heartbeat domain and service**

In `types.go`, add to `ProjectTaskAttempt`:

```go
BudgetWallClockLimitSec    *int32
BudgetLastHeartbeatAt      *time.Time
BudgetConsumedWallClockSec int32
BudgetConsumedTokens       int32
BudgetTrippedAt            *time.Time
BudgetTripReason           *string
```

Add:

```go
type RecordProjectTaskAttemptBudgetHeartbeatRequest struct {
	TenantID              uuid.UUID
	ProjectID             uuid.UUID
	ProjectTaskID         uuid.UUID
	AttemptID             uuid.UUID
	ConsumedWallClockSec  int32
	ConsumedTokens        int32
}

type ProjectTaskAttemptBudgetHeartbeatResult struct {
	Attempt    ProjectTaskAttempt
	Tripped    bool
	TripReason string
}
```

In `service.go`, implement wall-clock fuse:

```go
func (s *Service) RecordProjectTaskAttemptBudgetHeartbeat(ctx context.Context, req RecordProjectTaskAttemptBudgetHeartbeatRequest) (*ProjectTaskAttemptBudgetHeartbeatResult, error) {
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID)
	if err != nil {
		return nil, err
	}
	tripReason := ""
	if attempt.BudgetWallClockLimitSec != nil && req.ConsumedWallClockSec > *attempt.BudgetWallClockLimitSec {
		tripReason = "wall_clock_exceeded"
	}
	updated, err := s.repository.UpdateProjectTaskAttemptBudgetHeartbeat(ctx, req, tripReason)
	if err != nil {
		return nil, err
	}
	return &ProjectTaskAttemptBudgetHeartbeatResult{Attempt: updated, Tripped: tripReason != "", TripReason: tripReason}, nil
}
```

- [ ] **Step 4: Add HTTP endpoint and runtime client**

Add `POST /runtime/project-tasks/{task_id}/attempts/{attempt_id}/budget-heartbeat` to OpenAPI and handler. Response:

```json
{"tripped":true,"trip_reason":"wall_clock_exceeded"}
```

In Runtime client, add:

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProjectTaskBudgetHeartbeatWriteback {
    pub tenant_id: String,
    pub project_id: String,
    pub project_task_id: String,
    pub attempt_id: String,
    pub consumed_wall_clock_sec: i32,
    pub consumed_tokens: i32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProjectTaskBudgetHeartbeatResponse {
    pub tripped: bool,
    pub trip_reason: Option<String>,
}
```

- [ ] **Step 5: Runtime sends heartbeat and cancels on fuse**

In `commands/executor.rs`, record provider start time. Spawn a heartbeat task when project_task metadata exists:

```rust
let started = std::time::Instant::now();
let heartbeat = tokio::spawn(async move {
    loop {
        tokio::time::sleep(std::time::Duration::from_secs(15)).await;
        let elapsed = started.elapsed().as_secs() as i32;
        let response = client.record_project_task_budget_heartbeat(&body_for(elapsed)).await;
        if let Ok(response) = response {
            if response.tripped {
                let _ = handle.cancel().await;
                break;
            }
        }
    }
});
```

Cancel or let the heartbeat task exit when provider stream finishes. Do not block terminal writeback on heartbeat network errors.

**Phase 1 enforcement scope (accurate claim):** the only active enforcement in this plan is the Runtime cancelling its own in-flight provider when the heartbeat response reports `tripped`. The trip is also persisted to `project_task_attempts` (`budget_tripped_at`/`budget_trip_reason`) for audit. This plan does NOT add a PreDispatchGate check that blocks a *future* dispatch on a prior trip — that remains future work. Do not describe budget enforcement as "enforced on next predispatch" unless you also add a `predispatch_gate.go` step reading `budget_tripped_at` (out of scope here).

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run BudgetHeartbeat -count=1
go test ./apps/control-plane/internal/api -run BudgetHeartbeat -count=1
cargo test --manifest-path apps/runtime-agent/Cargo.toml budget
corepack pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/api contracts/control-plane/openapi.yaml apps/runtime-agent/src
git commit -m "feat: add task attempt budget heartbeat"
```

### Task 10: End-To-End Local Smoke

**Files:**
- Modify only if smoke finds real defects in files touched above.

- [ ] **Step 1: Run static gates**

Run:

```bash
git diff --check
corepack pnpm verify:contracts
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/employee ./apps/control-plane/internal/api
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: PASS.

- [ ] **Step 2: Start or restart local services**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart runtime-agent
scripts/dev-services.sh status
```

Expected: Control Plane and Runtime Agent are running from the current checkout.

- [ ] **Step 3: Create a smoke project with repo binding through real API**

Use the existing local auth/dev token flow for this repo. Submit a project payload containing:

```json
{
  "name": "Runtime affinity smoke",
  "goal": "Verify repo-bound project task dispatch metadata",
  "human_owner_user_id": "<current-user-id>",
  "repo_binding": {
    "url": "file:///tmp/superteam-smoke-repo",
    "default_branch": "main",
    "scope": ["."]
  }
}
```

Expected: response includes:

```json
"repo_binding":{"status":"bound","url":"file:///tmp/superteam-smoke-repo","default_branch":"main","scope":["."]}
```

- [ ] **Step 4: Submit a demand and verify dispatch metadata**

Submit a demand that creates a `feature_development` task. Inspect Control Plane DB or command receipt payload:

```bash
psql "$DATABASE_URL" -c "select payload->'metadata' from runtime_command_receipts order by created_at desc limit 1;"
```

Expected metadata contains:

```json
{
  "workspace_mode": "branch",
  "base_ref": "main",
  "project_git": {"url":"file:///tmp/superteam-smoke-repo","default_branch":"main","scope":["."]},
  "handoff_contract": {"completion_path":"project_task_attempt_writeback","requires_runtime_attestation":true}
}
```

- [ ] **Step 5: Verify Runtime uses worktree CWD and employee home config**

Inspect Runtime run snapshot/logs:

```bash
find . -path '*workspaces*' -maxdepth 6 -type d | head
find . -path '*employees*' -maxdepth 6 -type d | head
```

Expected: provider run workspace path is under `workspaces/{project_id}/{task_id}/{attempt_id}`, while skills/MCP remain under `employees/{digital_employee_id}`.

- [ ] **Step 6: Verify attestation writeback**

Query:

```bash
psql "$DATABASE_URL" -c "select attestation_type,status,git_base_ref,git_head_sha from project_task_attestations order by created_at desc limit 5;"
```

Expected: at least one row for the smoke attempt. If provider execution is unavailable, the row may be `failed`, but it must be Runtime-generated and tied to the attempt.

- [ ] **Step 7: Verify budget fuse with a low wall-clock limit**

Manually set the smoke attempt budget to one second:

```bash
psql "$DATABASE_URL" -c "update project_task_attempts set budget_wall_clock_limit_sec = 1 where id = '<attempt-id>';"
```

Let the Runtime heartbeat run. Query:

```bash
psql "$DATABASE_URL" -c "select budget_tripped_at,budget_trip_reason from project_task_attempts where id = '<attempt-id>';"
```

Expected: `budget_trip_reason = 'wall_clock_exceeded'`.

- [ ] **Step 8: Run completion gate**

Use the project skill:

```bash
sed -n '1,240p' .codex/skills/superteam-completion-check/SKILL.md
```

Follow it exactly. Do not claim real end-to-end completion unless Steps 2-7 passed against current services.

- [ ] **Step 9: Commit smoke fixes**

If smoke required code fixes:

```bash
git add <fixed-files>
git commit -m "fix: complete project workspace smoke"
```

If no fixes were required, do not create an empty commit.

## Verification Matrix

- Storage: `go test ./apps/control-plane/internal/storage -run ProjectRepoBindingAndAttestation -count=1`
- Control Plane project domain: `go test ./apps/control-plane/internal/project -run 'RepoBinding|Attestation|BudgetHeartbeat|TaskResultContract' -count=1`
- Coordinator: `go test ./apps/control-plane/internal/workflow/projectcoordination -run 'WorkspaceMode|DispatchProjectTask|Revision|Iteration' -count=1`
- Employee run payload: `go test ./apps/control-plane/internal/employee -run 'StartSessionPayload|IdempotencyFingerprint' -count=1`
- Runtime: `cargo test --manifest-path apps/runtime-agent/Cargo.toml`
- Contracts: `corepack pnpm verify:contracts`
- Full backend/runtime gate before merge: `corepack pnpm verify:control-plane && corepack pnpm verify:runtime-agent`
- Real smoke: `scripts/dev-services.sh status`, real Control Plane API, real Runtime Agent, DB readback for command receipt, worktree path, attestation, and budget fuse.

## Self-Review Notes

- Spec coverage:
  - Project repo binding: Tasks 1-3.
  - Project placement: Task 1 establishes dynamic placement table; scheduling policy beyond current Runtime assignment remains future work.
  - Agent home/CWD separation: Tasks 4-5.
  - Provider config/home injection: Task 4.
  - Role-derived workspace mode: Task 3.
  - Branch/worktree/ref/scope dispatch: Tasks 3 and 5.
  - Attestation-backed result validity: Tasks 6-7.
  - Bounded revision iteration: Task 8.
  - Budget fuse: Task 9.
  - Human approval throughput: existing PreDispatchGate/DecisionRequest queue is reused; broad policy delegation is future work.
  - Knowledge compounding: not implemented here; remains Phase 3 per outline.
- Placeholder scan: no unresolved placeholder markers or deferred code steps are required for this Phase 1 plan.
- Type consistency: `workspace_mode`, `base_ref`, `project_git`, `requires_runtime_attestation`, `ProjectRepoBinding`, `ProjectTaskAttestation`, and budget heartbeat names are used consistently across Control Plane and Runtime tasks.
