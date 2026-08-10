package storage

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

var bcryptHashPattern = regexp.MustCompile(`\$2[aby]\$[0-9]{2}\$[A-Za-z0-9./]{53}`)

var uuidFirstTables = []string{
	"tenants",
	"tenant_profiles",
	"tenant_teams",
	"tenant_team_config_revisions",
	"auth_users",
	"tenant_members",
	"runtime_nodes",
	"runtime_node_scopes",
	"auth_runtime_tokens",
	"runtime_bootstrap_keys",
	"runtime_enrollments",
	"runtime_sessions",
	"runtime_capabilities",
	"auth_sessions",
	"digital_employees",
	"digital_employee_config_revisions",
	"digital_employee_effective_configs",
	"provider_sessions",
	"provider_session_events",
	"tasks",
	"task_runs",
	"task_state_history",
	"task_events",
	"task_artifacts",
	"audit_events",
	"web_login_logs",
	"web_operation_logs",
}

func TestInitialSchemaIsUUIDFirst(t *testing.T) {
	body, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(body)

	for _, forbidden := range []string{
		"BIGSERIAL PRIMARY KEY",
		" user_id BIGINT",
		" creator_id BIGINT",
		" task_id BIGINT",
		" execution_id BIGINT",
		"id VARCHAR(255) PRIMARY KEY",
		"CREATE EXTENSION IF NOT EXISTS pgcrypto",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("initial schema must not contain %q", forbidden)
		}
	}

	for _, expected := range []string{
		"CREATE TABLE tenants",
		"CREATE TABLE tenant_teams",
		"CREATE TABLE runtime_node_scopes",
		"CREATE TABLE auth_sessions",
		"CREATE TABLE task_runs",
		"CREATE TABLE web_login_logs",
		"CREATE TABLE web_operation_logs",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"user_id UUID NOT NULL",
		"token_hash VARCHAR(255) UNIQUE NOT NULL",
		"creator_id UUID",
		"task_id UUID NOT NULL",
		"run_id UUID",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"cancelled_at TIMESTAMPTZ",
		"CREATE UNIQUE INDEX uq_auth_users_active_username",
		"CREATE UNIQUE INDEX uq_auth_runtime_tokens_active_node_id",
		"CREATE UNIQUE INDEX uq_task_events_task_sequence",
		"COMMENT ON TABLE tenants IS",
		"COMMENT ON COLUMN tasks.tenant_id IS",
		"COMMENT ON COLUMN tasks.cancelled_at IS",
		"COMMENT ON TABLE web_login_logs IS",
		"COMMENT ON TABLE web_operation_logs IS",
		"COMMENT ON COLUMN web_operation_logs.action IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected UUID-first initial schema to contain %q", expected)
		}
	}

	for _, expected := range []string{
		"CREATE TABLE tenant_team_config_revisions",
		"CREATE TABLE digital_employee_config_revisions",
		"CREATE TABLE digital_employee_effective_configs",
		"human_owner_user_id UUID",
		"CREATE UNIQUE INDEX uq_digital_employee_config_revisions_active",
		"COMMENT ON COLUMN tenant_teams.human_owner_user_id IS '团队负责人用户ID，第一版用于团队级审批、升级和跨团队交接决策';",
		"COMMENT ON TABLE tenant_team_config_revisions IS '团队治理配置版本表';",
		"COMMENT ON COLUMN tenant_team_config_revisions.internal_collaboration_policy IS '团队内部协作策略，定义同团队数字员工自动问询的边界';",
		"COMMENT ON TABLE digital_employee_config_revisions IS '数字员工个人治理配置版本表';",
		"COMMENT ON TABLE digital_employee_effective_configs IS '数字员工生效治理配置快照表';",
		"COMMENT ON COLUMN digital_employee_effective_configs.validation_result IS '生效配置校验结果，包含阻断错误和警告';",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected team governance schema to contain %q", expected)
		}
	}
}

func TestForwardOnlyAuthAndWebLogMigrationsWereMergedIntoInitialSchema(t *testing.T) {
	body, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE auth_sessions",
		"token_hash VARCHAR(255) UNIQUE NOT NULL",
		"CREATE TABLE web_login_logs",
		"CREATE TABLE web_operation_logs",
		"event_type VARCHAR(50) NOT NULL CHECK (event_type IN ('login_succeeded', 'login_failed', 'logout_succeeded'))",
		"session_id UUID",
		"request_id VARCHAR(255)",
		"CREATE UNIQUE INDEX uq_auth_users_active_username",
		"COMMENT ON TABLE auth_users IS 'Web 控制台平台用户表'",
		"COMMENT ON COLUMN auth_users.password_hash IS '用户密码哈希，禁止存储明文密码'",
		"COMMENT ON TABLE web_operation_logs IS 'Web 控制台操作日志表'",
		"COMMENT ON COLUMN web_operation_logs.action IS '操作动作'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected merged auth/web log schema to contain %q", expected)
		}
	}

	for _, path := range []string{
		"migrations/003_create_auth_sessions.sql",
		"migrations/004_create_web_logs.sql",
		"migrations/005_comment_auth_users_and_web_operation_logs.sql",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("%s should be merged into 001_initial.sql for rebuild-only schema", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}

func TestInboxItemsMigrationAddsActionableQueueReadModel(t *testing.T) {
	body, err := os.ReadFile("migrations/016_inbox_items.sql")
	if err != nil {
		t.Fatalf("read inbox migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE inbox_items",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"target_user_id UUID NOT NULL",
		"source_type VARCHAR(100) NOT NULL",
		"source_id UUID NOT NULL",
		"source_approval_request_id UUID",
		"action_schema JSONB NOT NULL DEFAULT '[]'::jsonb",
		"context_payload JSONB NOT NULL DEFAULT '{}'::jsonb",
		"deep_link JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE UNIQUE INDEX uq_inbox_items_tenant_source",
		"CREATE UNIQUE INDEX uq_inbox_items_tenant_approval_source",
		"CREATE INDEX idx_inbox_items_tenant_target_status_activity",
		"ON inbox_items(tenant_id, source_approval_request_id)",
		"WHERE source_approval_request_id IS NOT NULL",
		"ON inbox_items(tenant_id, source_project_id, status, last_activity_at DESC)",
		"COMMENT ON TABLE inbox_items IS",
		"COMMENT ON COLUMN inbox_items.source_approval_request_id IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected inbox migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"CREATE TYPE inbox",
		"CREATE TYPE item_type",
		"FOREIGN KEY",
		"REFERENCES",
		"ON DELETE CASCADE",
		"BIGSERIAL",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("inbox read model must avoid %q", forbidden)
		}
	}
}

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
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_files_active_path",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_file_revisions_number",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_file_syncs_target",
		"COMMENT ON TABLE digital_employee_workspace_files IS '数字员工工作目录受控文件身份表'",
		"COMMENT ON TABLE digital_employee_workspace_file_revisions IS '数字员工工作目录受控文件内容版本表'",
		"COMMENT ON TABLE digital_employee_workspace_file_syncs IS '数字员工工作目录文件同步状态投影表'",
		"COMMENT ON COLUMN digital_employee_workspace_file_syncs.id IS '同步状态主键 UUID'",
		"COMMENT ON COLUMN digital_employee_workspace_file_syncs.created_at IS '同步状态创建时间'",
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

func TestDigitalEmployeeEnvironmentVariablesMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/033_digital_employee_env_and_skill_dependencies.sql")
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

func TestAuthCaptchaChallengeMigrationAddsOneTimeChallengeStorage(t *testing.T) {
	body, err := os.ReadFile("migrations/039_auth_captcha_challenges.sql")
	if err != nil {
		t.Fatalf("read auth captcha migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS auth_captcha_challenges",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid",
		"answer_hash VARCHAR(255) NOT NULL",
		"expires_at TIMESTAMPTZ NOT NULL",
		"used_at TIMESTAMPTZ",
		"client_ip VARCHAR(255)",
		"user_agent TEXT",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_expires_at",
		"CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_used_at",
		"CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_created_at",
		"pg_trigger",
		"tgname = 'update_auth_captcha_challenges_updated_at'",
		"CREATE TRIGGER update_auth_captcha_challenges_updated_at",
		"COMMENT ON TABLE auth_captcha_challenges IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected auth captcha migration to contain %q", expected)
		}
	}
}

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
	assertMigrationContains(t, sql, "CHECK (auth_strategy IN ('none', 'bearer_env', 'headers_env'))")
	assertMigrationContains(t, sql, "COMMENT ON TABLE mcp_servers IS")

	// Backfill must carry legacy team rows into the registry.
	assertMigrationContains(t, sql, "INSERT INTO mcp_servers")
	assertMigrationContains(t, sql, "team_mcp_servers")
}

func TestDigitalEmployeeProviderIdentityMigrationObjects(t *testing.T) {
	sql := readMigration(t, "045_digital_employee_provider_identity.sql")
	require.Contains(t, sql, "ALTER TABLE digital_employees")
	require.Contains(t, sql, "ADD COLUMN provider_type VARCHAR(100)")
	require.Contains(t, sql, "UPDATE digital_employees de")
	require.Contains(t, sql, "project_task_attempts")
	require.Contains(t, sql, "digital_employee_id UUID")
	require.Contains(t, sql, "provider_type VARCHAR(100)")
}

func TestMigration046TeamConstitutionAndSkillBindingRename(t *testing.T) {
	sql := readMigration(t, "046_team_constitution_and_skill_binding_rename.sql")

	for _, expected := range []string{
		"ALTER TABLE tenant_teams",
		"ADD COLUMN IF NOT EXISTS constitution JSONB NOT NULL DEFAULT '{}'::jsonb",
		"UPDATE tenant_teams t",
		"SET constitution = r.constitution",
		"FROM tenant_team_config_revisions r",
		"r.tenant_id = t.tenant_id",
		"r.team_id = t.id",
		"r.status = 'active'",
		"ALTER TABLE skill_team_bindings RENAME TO team_skill_bindings",
	} {
		assertMigrationContains(t, sql, expected)
	}
}

func TestMigration046TeamConstitutionAndSkillBindingRenameAppliedSchema(t *testing.T) {
	pool, schemaName := applyAllMigrations(t)

	assertTableExists(t, pool, schemaName, "team_skill_bindings")
	assertTableAbsent(t, pool, schemaName, "skill_team_bindings")
	assertColumnExists(t, pool, schemaName, "tenant_teams", "constitution")
}

func TestMigration047DropsGovernanceTables(t *testing.T) {
	pool, schemaName := applyAllMigrations(t)

	assertTableAbsent(t, pool, schemaName, "tenant_team_config_revisions")
	assertTableAbsent(t, pool, schemaName, "digital_employee_effective_configs")
	assertTableExists(t, pool, schemaName, "digital_employee_config_revisions")
}

func TestInboxQueriesUseFilteredCountsApprovalSourceAndStableOrdering(t *testing.T) {
	body, err := os.ReadFile("queries/inbox.sql")
	if err != nil {
		t.Fatalf("read inbox queries: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		`-- name: UpsertInboxItem :one
INSERT INTO inbox_items (
    tenant_id,
    team_id,
    target_user_id,
    scope,
    item_type,
    source_type,
    source_id,
    source_project_id,
    source_task_id,
    source_approval_request_id,
    title,
    summary,
    risk_level,
    priority,
    status,
    action_schema,
    context_payload,
    deep_link,
    resolved_at,
    last_activity_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('scope')::varchar,
    sqlc.arg('item_type')::varchar,
    sqlc.arg('source_type')::varchar,
    sqlc.arg('source_id')::uuid,
    sqlc.narg('source_project_id')::uuid,
    sqlc.narg('source_task_id')::uuid,
    sqlc.narg('source_approval_request_id')::uuid,`,
		`-- name: UpsertInboxItemByApprovalSource :one
INSERT INTO inbox_items (
    tenant_id,
    team_id,
    target_user_id,
    scope,
    item_type,
    source_type,
    source_id,
    source_project_id,
    source_task_id,
    source_approval_request_id,
    title,
    summary,
    risk_level,
    priority,
    status,
    action_schema,
    context_payload,
    deep_link,
    resolved_at,
    last_activity_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('scope')::varchar,
    sqlc.arg('item_type')::varchar,
    sqlc.arg('source_type')::varchar,
    sqlc.arg('source_id')::uuid,
    sqlc.narg('source_project_id')::uuid,
    sqlc.narg('source_task_id')::uuid,
    sqlc.arg('source_approval_request_id')::uuid,`,
		`CASE risk_level
    WHEN 'blocked' THEN 0
    WHEN 'high'    THEN 1
    WHEN 'medium'  THEN 2
    WHEN 'low'     THEN 3
    ELSE 4
  END,
  last_activity_at DESC, created_at DESC, id DESC`,
		"ORDER BY created_at ASC, id ASC",
		`-- name: CountInboxItems :one
SELECT COUNT(*)::bigint FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (
    sqlc.narg('status')::varchar IS NULL
    OR status = sqlc.narg('status')::varchar
  )
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
    OR (
      -- any-of-N: 项目决策类事项对该项目全部 active 人类成员可见(成员同等身份)
      item_type = 'project_decision'
      AND source_project_id IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM project_members pm
        WHERE pm.tenant_id = inbox_items.tenant_id
          AND pm.project_id = inbox_items.source_project_id
          AND pm.principal_type = 'human_user'
          AND pm.status = 'active'
          AND pm.principal_id = sqlc.narg('target_user_id')::uuid
      )
    )
  )
  AND (
    sqlc.narg('item_type')::varchar IS NULL
    OR item_type = sqlc.narg('item_type')::varchar
  )
  AND (
    sqlc.narg('risk_level')::varchar IS NULL
    OR risk_level = sqlc.narg('risk_level')::varchar
  )
  AND (
    sqlc.narg('source_project_id')::uuid IS NULL
    OR source_project_id = sqlc.narg('source_project_id')::uuid
  );`,
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected inbox queries to contain %q", expected)
		}
	}
}

// TestInboxListQueriesShareWhereClause pins ListInboxItems / ListInboxItemsOldest
// visibility filters to the same WHERE body so sort-mode forks cannot drift.
func TestInboxListQueriesShareWhereClause(t *testing.T) {
	body, err := os.ReadFile("queries/inbox.sql")
	if err != nil {
		t.Fatalf("read inbox queries: %v", err)
	}
	sql := string(body)

	extractWhere := func(name string) string {
		marker := "-- name: " + name + " :many"
		start := strings.Index(sql, marker)
		if start < 0 {
			t.Fatalf("missing query %s", name)
		}
		rest := sql[start:]
		// Anchor on the SELECT body, not prose comments that contain "WHERE".
		selectIdx := strings.Index(rest, "SELECT * FROM inbox_items\nWHERE ")
		if selectIdx < 0 {
			t.Fatalf("%s: missing SELECT * FROM inbox_items WHERE", name)
		}
		whereStart := selectIdx + len("SELECT * FROM inbox_items\n")
		orderIdx := strings.Index(rest[whereStart:], "\nORDER BY")
		if orderIdx < 0 {
			t.Fatalf("%s: no ORDER BY after WHERE", name)
		}
		return rest[whereStart : whereStart+orderIdx]
	}

	riskWhere := extractWhere("ListInboxItems")
	oldestWhere := extractWhere("ListInboxItemsOldest")
	if riskWhere != oldestWhere {
		t.Fatalf("ListInboxItems and ListInboxItemsOldest WHERE clauses differ:\n--- risk ---\n%s\n--- oldest ---\n%s", riskWhere, oldestWhere)
	}
	if !strings.Contains(sql, "ORDER BY created_at ASC, id ASC") {
		t.Fatalf("expected oldest ORDER BY created_at ASC, id ASC")
	}
}

func TestAuthUserAvatarMigrationAddsGeneratedAvatarConfig(t *testing.T) {
	body, err := os.ReadFile("migrations/005_add_auth_user_avatar.sql")
	if err != nil {
		t.Fatalf("read auth avatar migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"ADD COLUMN IF NOT EXISTS avatar_provider VARCHAR(50) NOT NULL DEFAULT 'dicebear'",
		"ADD COLUMN IF NOT EXISTS avatar_style VARCHAR(100) NOT NULL DEFAULT 'adventurer'",
		"ADD COLUMN IF NOT EXISTS avatar_seed VARCHAR(255)",
		"ADD COLUMN IF NOT EXISTS avatar_options JSONB NOT NULL DEFAULT '{}'::jsonb",
		"ADD CONSTRAINT chk_auth_users_avatar_provider CHECK (avatar_provider IN ('dicebear'))",
		"ADD CONSTRAINT chk_auth_users_avatar_style CHECK (avatar_style <> '')",
		"ADD CONSTRAINT chk_auth_users_avatar_options_object CHECK (jsonb_typeof(avatar_options) = 'object')",
		"COMMENT ON COLUMN auth_users.avatar_provider IS '用户头像来源，MVP 使用 DiceBear 生成稳定卡通头像'",
		"COMMENT ON COLUMN auth_users.avatar_style IS '用户头像样式标识，MVP 默认为 DiceBear adventurer'",
		"COMMENT ON COLUMN auth_users.avatar_seed IS '用户头像生成种子；为空时由服务端使用 username 生成稳定种子'",
		"COMMENT ON COLUMN auth_users.avatar_options IS '用户头像生成选项 JSON，保留颜色、配件等后续扩展配置'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected auth avatar migration to contain %q", expected)
		}
	}
}

func TestDigitalEmployeeCreationMigrationAddsOwnerAndType(t *testing.T) {
	body, err := os.ReadFile("migrations/008_digital_employee_creation_ready.sql")
	if err != nil {
		t.Fatalf("read digital employee creation migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"ADD COLUMN IF NOT EXISTS owner_user_id UUID",
		"ADD COLUMN IF NOT EXISTS employee_type VARCHAR(100)",
		"RAISE EXCEPTION 'digital_employees.owner_user_id unresolved before NOT NULL migration'",
		"ELSE 2",
		"ALTER COLUMN owner_user_id SET NOT NULL",
		"ALTER COLUMN employee_type SET NOT NULL",
		"CREATE INDEX IF NOT EXISTS idx_digital_employees_owner_status",
		"ON digital_employees(tenant_id, owner_user_id, status, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_digital_employees_type_status",
		"ON digital_employees(tenant_id, employee_type, status, created_at DESC)",
		"COMMENT ON COLUMN digital_employees.owner_user_id IS '数字员工归属人类用户ID，由控制平面从登录上下文写入'",
		"COMMENT ON COLUMN digital_employees.employee_type IS '数字员工专业类型，由服务端注册表校验，不使用数据库枚举'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected digital employee creation migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"CREATE TYPE employee_type",
		"CHECK (employee_type IN",
		"FROM auth_users au",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("employee_type must stay registry-backed, found %q", forbidden)
		}
	}

	if got := strings.Count(sql, "FROM tenant_members tm"); got < 2 {
		t.Fatalf("expected owner backfill to use tenant_members for privileged and general fallback, got %d tenant_members lookups", got)
	}
}

func TestDigitalEmployeeBudgetPolicyMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/012_digital_employee_budget_policy.sql")
	if err != nil {
		t.Fatalf("read budget policy migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"ALTER TABLE digital_employee_config_revisions",
		"ADD COLUMN IF NOT EXISTS budget_policy JSONB NOT NULL DEFAULT '{}'::jsonb",
		"COMMENT ON COLUMN digital_employee_config_revisions.budget_policy IS '数字员工预算策略，包含每日 token 上限；空对象表示无预算上限'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"ALTER TABLE digital_employees",
		"metadata",
		"approval_policy_override",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("budget policy migration must not use %q", forbidden)
		}
	}
}

func TestDigitalEmployeeConfigFinalModelMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/051_digital_employee_config_final_model.sql")
	require.NoError(t, err)
	sql := string(body)

	for _, expected := range []string{
		"DELETE FROM digital_employee_environment_variables",
		"DELETE FROM skill_installations",
		"DELETE FROM digital_employee_mcp_bindings_v2",
		"DELETE FROM digital_employee_mcp_bindings",
		"DELETE FROM skill_agent_bindings",
		"DELETE FROM project_employee_node_affinity",
		"UPDATE project_task_attempts",
		"SET digital_employee_id = NULL",
		"UPDATE project_tasks",
		"SET assigned_digital_employee_id = NULL",
		"digital_employee_run_id = NULL",
		"DELETE FROM task_runs",
		"DELETE FROM digital_employee_workspace_file_syncs",
		"DELETE FROM digital_employee_workspace_file_revisions",
		"DELETE FROM digital_employee_workspace_files",
		"DELETE FROM runtime_command_receipts",
		"DELETE FROM provider_session_events",
		"DELETE FROM provider_sessions",
		"DELETE FROM digital_employee_execution_instances",
		"DELETE FROM digital_employee_config_revisions",
		"DELETE FROM digital_employees",
		"ADD COLUMN IF NOT EXISTS persona_memory_markdown TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS capability_bindings JSONB NOT NULL DEFAULT '{}'::jsonb",
		"DROP COLUMN IF EXISTS role_profile",
		"DROP COLUMN IF EXISTS constitution_addendum",
		"DROP COLUMN IF EXISTS capability_selection",
		"DROP COLUMN IF EXISTS context_policy_override",
		"DROP COLUMN IF EXISTS approval_policy_override",
		"DROP COLUMN IF EXISTS output_contract_addendum",
		"COMMENT ON COLUMN digital_employee_config_revisions.persona_memory_markdown IS '数字员工人格记忆 Markdown，描述人格画像、专业边界、工作方式和表达偏好'",
		"COMMENT ON COLUMN digital_employee_config_revisions.capability_bindings IS '数字员工能力绑定，保存 Skill、MCP、外部能力和环境变量引用'",
	} {
		require.Contains(t, sql, expected)
	}

	for _, forbidden := range []string{
		"INSERT INTO digital_employee_config_revisions",
		"role_profile JSONB",
		"constitution_addendum JSONB",
		"capability_selection JSONB",
		"context_policy_override JSONB",
		"approval_policy_override JSONB",
		"output_contract_addendum JSONB",
		"DELETE FROM digital_employee_effective_configs",
	} {
		require.NotContains(t, sql, forbidden)
	}
}

func TestDualLayerCapabilityManagementMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/018_dual_layer_capability_management.sql")
	if err != nil {
		t.Fatalf("read dual layer capability management migration: %v", err)
	}
	sql := string(body)

	required := []string{
		"CREATE TABLE IF NOT EXISTS user_credentials",
		"CREATE TABLE IF NOT EXISTS team_mcp_servers",
		"CREATE TABLE IF NOT EXISTS digital_employee_mcp_bindings",
		"encrypted_value TEXT NOT NULL",
		"credential_id UUID",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_user_credentials_owner_name_active",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_team_mcp_servers_team_name_active",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_mcp_bindings_employee_name_active",
		"COMMENT ON TABLE user_credentials IS '个人凭据池，保存用户可复用的外部能力授权令牌密文'",
		"COMMENT ON TABLE team_mcp_servers IS '团队公共 MCP 服务器配置，团队下数字员工强制继承'",
		"COMMENT ON TABLE digital_employee_mcp_bindings IS '数字员工个人 MCP 服务器配置'",
	}
	for _, expected := range required {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %q", expected)
		}
	}

	forbidden := []string{
		"digital_employee_instruction_files",
		"credential_value",
		"REFERENCES user_credentials",
		"ON DELETE CASCADE",
	}
	for _, value := range forbidden {
		if strings.Contains(sql, value) {
			t.Fatalf("migration must not contain %q", value)
		}
	}
}

func TestProjectTaskDependenciesMigrationHasTenantFirstIndexes(t *testing.T) {
	sql := migrationsSQL(t)
	block := createTableBlock(t, sql, "project_task_dependencies")
	for _, fragment := range []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"project_id UUID NOT NULL",
		"dependent_task_id UUID NOT NULL",
		"blocker_task_id UUID NOT NULL",
	} {
		if !strings.Contains(block, fragment) {
			t.Fatalf("expected project_task_dependencies block to include %q, got:\n%s", fragment, block)
		}
	}
	for _, fragment := range []string{
		"uq_ptd_edge",
		"idx_ptd_tenant_project_dependent",
		"idx_ptd_blocker",
		"idx_ptd_coordination_job",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to include %q", fragment)
		}
	}
}

func TestEmployeeOperationalStatusIndexesMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/023_employee_operational_status_indexes.sql")
	if err != nil {
		t.Fatalf("read employee operational status indexes migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE INDEX idx_project_tasks_tenant_assignee_status",
		"ON project_tasks(tenant_id, assigned_digital_employee_id, status)",
		"WHERE assigned_digital_employee_id IS NOT NULL",
		"CREATE INDEX idx_project_decision_requests_tenant_task_status_type",
		"ON project_decision_requests(tenant_id, project_task_id, status_snapshot, decision_type)",
		"WHERE project_task_id IS NOT NULL",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected employee operational status indexes migration to contain %q", expected)
		}
	}
}

func TestProjectDecisionRequestsPlanRevisionResumeMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/043_project_decision_plan_revision_resume.sql")
	require.NoError(t, err)
	sql := string(body)

	for _, expected := range []string{
		"ALTER TABLE project_decision_requests",
		"ADD COLUMN plan_revision_id UUID",
		"idx_project_decision_requests_plan_revision",
		"ON project_decision_requests(tenant_id, project_id, plan_revision_id)",
		"WHERE plan_revision_id IS NOT NULL",
		"COMMENT ON COLUMN project_decision_requests.plan_revision_id IS",
	} {
		require.Contains(t, sql, expected)
	}
}

func TestProjectPlanRevisionCreatedEventMigration(t *testing.T) {
	legacy := readMigration(t, "031_project_plan_revisions.sql")
	require.NotContains(t, legacy, "created_event_id UUID")

	sql := readMigration(t, "044_project_plan_revision_created_event.sql")
	for _, expected := range []string{
		"ALTER TABLE project_plan_revisions",
		"ADD COLUMN created_event_id UUID",
		"COMMENT ON COLUMN project_plan_revisions.created_event_id IS",
	} {
		require.Contains(t, sql, expected)
	}
}

func TestProjectTasksMigrationAddsGraphContractColumns(t *testing.T) {
	sql := migrationsSQL(t)
	for _, fragment := range []string{
		"ADD COLUMN coordination_job_id UUID",
		"ADD COLUMN route_decision_id UUID",
		"ADD COLUMN planned_task_key VARCHAR(100)",
		"ADD COLUMN task_kind VARCHAR(100)",
		"ADD COLUMN stage_index INTEGER",
		"ADD COLUMN expected_outputs JSONB NOT NULL DEFAULT '[]'::jsonb",
		"ADD COLUMN input_requirements JSONB NOT NULL DEFAULT '{}'::jsonb",
		"ADD COLUMN handoff_contract JSONB NOT NULL DEFAULT '{}'::jsonb",
		"ADD COLUMN planner_metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"uq_project_tasks_coordination_planned_key",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected project task graph migration fragment %q", fragment)
		}
	}
}

func TestProjectTaskAttemptsMigration(t *testing.T) {
	sql := migrationsSQL(t)
	block := createTableBlock(t, sql, "project_task_attempts")
	for _, fragment := range []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"project_task_id UUID NOT NULL",
		"attempt_no INTEGER NOT NULL",
		"status VARCHAR(50) NOT NULL",
		"digital_employee_run_id UUID",
		"runtime_task_id UUID",
		"runtime_node_id UUID",
		"provider_session_id VARCHAR(255)",
		"execution_context_packet JSONB NOT NULL DEFAULT '{}'::jsonb",
		"execution_context_packet_version VARCHAR(50) NOT NULL DEFAULT 'v1'",
		"lease_token VARCHAR(255) NOT NULL",
		"lease_expires_at TIMESTAMPTZ",
		"renewed_at TIMESTAMPTZ",
		"lost_at TIMESTAMPTZ",
		"started_at TIMESTAMPTZ",
		"finished_at TIMESTAMPTZ",
		"timeout_at TIMESTAMPTZ",
		"retryable BOOLEAN",
		"failure_family VARCHAR(100)",
		"failure_message TEXT",
		"idempotency_key VARCHAR(255) NOT NULL",
		"created_event_id UUID",
		"terminal_event_id UUID",
		"CONSTRAINT fk_project_task_attempts_project_task",
		"FOREIGN KEY (tenant_id, project_task_id) REFERENCES project_tasks(tenant_id, id) ON DELETE CASCADE",
	} {
		if !strings.Contains(block, fragment) {
			t.Fatalf("project_task_attempts block missing %q:\n%s", fragment, block)
		}
	}
	// Phase 4: error_code column (nullable, historical rows not backfilled).
	if !strings.Contains(sql, "ADD COLUMN IF NOT EXISTS error_code VARCHAR(100)") &&
		!strings.Contains(sql, "error_code VARCHAR(100)") {
		// Column may appear only in ALTER migration after initial create block.
		if !strings.Contains(sql, "error_code") {
			t.Fatalf("expected project_task_attempts.error_code migration fragment")
		}
	}
	if !strings.Contains(sql, "idx_project_task_attempts_error_code") {
		t.Fatalf("expected idx_project_task_attempts_error_code")
	}
}

func TestProjectTaskAttemptContextUpdatesMigration(t *testing.T) {
	sql := migrationsSQL(t)
	block := createTableBlock(t, sql, "project_task_attempt_context_updates")
	for _, fragment := range []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"project_task_id UUID NOT NULL",
		"attempt_id UUID",
		"update_kind VARCHAR(100) NOT NULL",
		"payload JSONB NOT NULL",
		"delivery_mode VARCHAR(50) NOT NULL",
		"created_event_id UUID",
	} {
		if !strings.Contains(block, fragment) {
			t.Fatalf("project_task_attempt_context_updates block missing %q:\n%s", fragment, block)
		}
	}
}

func TestProjectRepoBindingAndAttestationMigration(t *testing.T) {
	sql := readMigration(t, "040_project_repo_binding_and_attestation.sql")

	for _, want := range []string{
		"ALTER TABLE projects",
		"ADD COLUMN repo_url TEXT",
		"ADD COLUMN repo_default_branch VARCHAR(255)",
		"ADD COLUMN repo_git_credential_ref VARCHAR(255)",
		"ADD COLUMN repo_scope JSONB NOT NULL DEFAULT '[]'::jsonb",
		"ADD COLUMN repo_binding_status VARCHAR(32) NOT NULL DEFAULT 'unbound'",
		"CREATE UNIQUE INDEX uq_projects_tenant_id",
		"ON projects(tenant_id, id)",
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

func TestProjectTaskAttestationRuntimeContextMigration(t *testing.T) {
	sql := readMigration(t, "041_project_task_attestation_runtime_context.sql")

	for _, want := range []string{
		"ALTER TABLE project_task_attestations",
		"ADD COLUMN digital_employee_id UUID",
		"ADD COLUMN capability_manifest_version VARCHAR(255)",
		"ADD COLUMN provider_auth_mode VARCHAR(40) NOT NULL DEFAULT 'host'",
		"UPDATE project_task_attestations",
		"project_tasks.assigned_digital_employee_id",
		"ALTER COLUMN digital_employee_id SET NOT NULL",
		"chk_project_task_attestations_provider_auth_mode",
		"CHECK (provider_auth_mode IN ('host', 'employee', 'explicit_credential'))",
		"COMMENT ON COLUMN project_task_attestations.digital_employee_id IS",
	} {
		assertMigrationContains(t, sql, want)
	}
}

func TestProjectTasksDurableClosureColumns(t *testing.T) {
	sql := migrationsSQL(t)
	for _, fragment := range []string{
		"ALTER TABLE project_tasks",
		"ADD COLUMN current_attempt_id UUID",
		"ADD COLUMN accepted_plan_revision_id UUID",
		"ADD COLUMN decomposition_claim_key VARCHAR(255)",
		"ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN max_attempts INTEGER",
		"ADD COLUMN retry_not_before TIMESTAMPTZ",
		"ADD COLUMN waiting_reason VARCHAR(100)",
		"ADD COLUMN waiting_request_id UUID",
		"ADD COLUMN terminal_reason VARCHAR(100)",
		"ADD COLUMN terminal_event_id UUID",
		"ADD COLUMN cancelled_by VARCHAR(100)",
		"ADD COLUMN failed_by VARCHAR(100)",
		"ADD COLUMN status_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"idx_project_tasks_current_attempt",
		"uq_project_tasks_tenant_id",
		"uq_project_tasks_accepted_plan_decomposition",
		"uq_project_task_attempts_tenant_task_id",
		"uq_project_task_attempts_active",
		"uq_project_task_attempts_idempotency_key",
		"FOREIGN KEY (tenant_id, id, current_attempt_id)",
		"REFERENCES project_task_attempts(tenant_id, project_task_id, id)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migrations missing durable-closure fragment %q", fragment)
		}
	}
}

func TestProjectTaskDurableClosureMigrationComments(t *testing.T) {
	sql := migrationsSQL(t)
	for _, fragment := range []string{
		"任务状态：pending, planned, queued, assigned, blocked, running, waiting_human, completed, failed, cancelled；queued 表示已分派并等待 Runtime 真正启动。",
		"当前项目任务执行尝试ID",
		"生成该任务的已接受计划版本ID",
		"计划分解幂等声明键",
		"任务执行尝试次数",
		"项目任务执行尝试表",
		"执行尝试所属项目任务ID",
		"尝试序号，从 1 开始递增",
		"尝试状态：queued, running, succeeded, failed, cancelled, lost, timed_out, waiting_human",
		"Runtime 租约令牌",
		"执行上下文包 JSON",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migrations missing durable-closure comment %q", fragment)
		}
	}
}

func TestProjectTaskResultsMigrationIsAppendOnlyAndTenantScoped(t *testing.T) {
	body := readMigration(t, "034_project_task_results.sql")

	require.Contains(t, body, "CREATE TABLE project_task_results")
	require.Contains(t, body, "tenant_id UUID NOT NULL")
	require.Contains(t, body, "project_task_id UUID NOT NULL")
	require.Contains(t, body, "attempt_id UUID")
	require.Contains(t, body, "contract_payload JSONB NOT NULL")
	require.Contains(t, body, "validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb")
	require.Contains(t, body, "CREATE UNIQUE INDEX uq_project_task_results_idempotency")
	require.Contains(t, body, "ON project_task_results(tenant_id, project_task_id, attempt_id, idempotency_key)")
	require.Contains(t, body, "CREATE UNIQUE INDEX uq_project_execution_summaries_tenant_project_task_id")
	require.Contains(t, body, "ON project_execution_summaries(tenant_id, project_id, project_task_id, id)")
	require.Contains(t, body, "FOREIGN KEY (tenant_id, project_id, project_task_id, execution_summary_id)")
	require.Contains(t, body, "REFERENCES project_execution_summaries(tenant_id, project_id, project_task_id, id)")
	require.Contains(t, body, "CREATE INDEX idx_project_task_results_tenant_task_created")
	require.Contains(t, body, "COMMENT ON COLUMN project_task_results.contract_payload")
	require.NotContains(t, strings.ToLower(body), "raw_log")
	require.NotContains(t, strings.ToLower(body), "secret")
}

func TestProjectDemandSummariesMigrationIsDemandScoped(t *testing.T) {
	body := readMigration(t, "034_project_task_results.sql")

	require.Contains(t, body, "CREATE TABLE project_demand_summaries")
	require.Contains(t, body, "demand_id UUID NOT NULL")
	require.Contains(t, body, "summary_payload JSONB NOT NULL")
	require.Contains(t, body, "CREATE UNIQUE INDEX uq_project_demand_summaries_idempotency")
	require.Contains(t, body, "CREATE UNIQUE INDEX uq_project_demands_tenant_project_id")
	require.Contains(t, body, "ON project_demands(tenant_id, project_id, id)")
	require.Contains(t, body, "FOREIGN KEY (tenant_id, project_id, demand_id)")
	require.Contains(t, body, "REFERENCES project_demands(tenant_id, project_id, id)")
	require.Contains(t, body, "CREATE UNIQUE INDEX uq_project_report_refs_tenant_project_id")
	require.Contains(t, body, "ON project_report_refs(tenant_id, project_id, id)")
	require.Contains(t, body, "FOREIGN KEY (tenant_id, project_id, report_ref_id)")
	require.Contains(t, body, "REFERENCES project_report_refs(tenant_id, project_id, id)")
	require.Contains(t, body, "CREATE INDEX idx_project_demand_summaries_tenant_demand_created")
	require.Contains(t, body, "COMMENT ON COLUMN project_demand_summaries.summary_payload")
}

func TestProjectPlanRevisionsMigrationHasTenantFirstIndexes(t *testing.T) {
	migration := readMigration(t, "031_project_plan_revisions.sql")

	for _, fragment := range []string{
		"CREATE TABLE project_plan_revisions",
		"CREATE TABLE project_plan_decomposition_claims",
		"tenant_id UUID NOT NULL",
		"CREATE UNIQUE INDEX uq_project_plan_revisions_revision_number",
		"ON project_plan_revisions(tenant_id, project_id, demand_id, revision_number)",
		"CREATE UNIQUE INDEX uq_project_plan_revisions_fingerprint",
		"ON project_plan_revisions(tenant_id, project_id, demand_id, plan_fingerprint)",
		"CREATE UNIQUE INDEX uq_project_plan_revisions_current_accepted",
		"CREATE UNIQUE INDEX uq_project_plan_decomposition_claims_revision",
		"ON project_plan_decomposition_claims(tenant_id, project_id, demand_id, accepted_plan_revision_id)",
		"COMMENT ON TABLE project_plan_revisions",
		"COMMENT ON COLUMN project_plan_revisions.payload",
	} {
		if !strings.Contains(migration, fragment) {
			t.Fatalf("project plan revisions migration missing %q", fragment)
		}
	}
}

func TestProjectTaskDispatchGateMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/032_project_task_dispatch_gates.sql")
	if err != nil {
		t.Fatalf("read dispatch gate migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE project_task_dispatch_gate_results",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"project_id UUID NOT NULL",
		"project_task_id UUID NOT NULL",
		"accepted_plan_revision_id UUID",
		"selected_employee_id UUID NOT NULL",
		"attempt_no INTEGER NOT NULL",
		"idempotency_key VARCHAR(255) NOT NULL",
		"dispatch_token VARCHAR(128) NOT NULL",
		"status VARCHAR(32) NOT NULL",
		"checks JSONB NOT NULL DEFAULT '[]'::jsonb",
		"blockers JSONB NOT NULL DEFAULT '[]'::jsonb",
		"human_action_request JSONB NOT NULL DEFAULT '{}'::jsonb",
		"retry_after TIMESTAMPTZ",
		"attempt_id UUID",
		"decision_request_id UUID",
		"created_event_id UUID",
		"CREATE UNIQUE INDEX uq_project_tasks_tenant_project_task_id",
		"ON project_tasks(tenant_id, project_id, id)",
		"FOREIGN KEY (tenant_id, project_id, project_task_id)",
		"REFERENCES project_tasks(tenant_id, project_id, id) ON DELETE CASCADE",
		"CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_key",
		"ON project_task_dispatch_gate_results(tenant_id, project_task_id, idempotency_key)",
		"CREATE UNIQUE INDEX uq_approval_requests_dispatch_gate_pending",
		"ON approval_requests(tenant_id, resource_id)",
		"WHERE resource_type = 'project_task_dispatch_gate' AND status = 'pending'",
		"CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_tenant_task_id",
		"ON project_task_dispatch_gate_results(tenant_id, project_task_id, id)",
		"CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_tenant_project_id",
		"ON project_task_dispatch_gate_results(tenant_id, project_id, id)",
		"CREATE INDEX idx_project_task_dispatch_gate_results_task_created",
		"ALTER TABLE project_task_attempts",
		"ADD COLUMN dispatch_gate_result_id UUID",
		"ALTER TABLE project_tasks",
		"ADD COLUMN latest_dispatch_gate_result_id UUID",
		"ALTER TABLE project_decision_requests",
		"ADD COLUMN dispatch_gate_result_id UUID",
		"ADD CONSTRAINT fk_project_decision_requests_dispatch_gate",
		"FOREIGN KEY (tenant_id, project_id, dispatch_gate_result_id)",
		"REFERENCES project_task_dispatch_gate_results(tenant_id, project_id, id)",
		"COMMENT ON TABLE project_task_dispatch_gate_results IS",
		"COMMENT ON COLUMN project_task_dispatch_gate_results.blockers IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected dispatch gate migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"BIGSERIAL",
		"CREATE TYPE",
		"connection_string",
		"secret_value",
		"raw_log",
		"provider_stdout",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("dispatch gate migration must avoid %q", forbidden)
		}
	}

	queryBody, err := os.ReadFile("queries/project.sql")
	if err != nil {
		t.Fatalf("read project queries: %v", err)
	}
	projectSQL := string(queryBody)
	attemptStart := strings.Index(projectSQL, "-- name: CreateProjectTaskAttempt :one")
	if attemptStart < 0 {
		t.Fatalf("project queries must contain CreateProjectTaskAttempt")
	}
	attemptRest := projectSQL[attemptStart:]
	attemptEnd := strings.Index(attemptRest, "-- name: GetProjectTaskAttempt :one")
	if attemptEnd < 0 {
		t.Fatalf("CreateProjectTaskAttempt query must end before GetProjectTaskAttempt")
	}
	attemptSQL := attemptRest[:attemptEnd]

	for _, expected := range []string{
		"    dispatch_gate_result_id,\n    created_event_id",
		"    sqlc.narg('dispatch_gate_result_id')::uuid,\n    sqlc.narg('created_event_id')::uuid",
	} {
		if !strings.Contains(attemptSQL, expected) {
			t.Fatalf("expected CreateProjectTaskAttempt query to contain %q", expected)
		}
	}
}

func TestExecutionLedgerEventsMigration(t *testing.T) {
	sql := readMigration(t, "029_execution_ledger_events.sql")
	block := createTableBlock(t, sql, "execution_ledger_events")

	expected := []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"team_id UUID",
		"project_id UUID NOT NULL",
		"project_task_id UUID",
		"project_task_attempt_id UUID",
		"event_type VARCHAR(100) NOT NULL",
		"source_type VARCHAR(100) NOT NULL",
		"source_id VARCHAR(255) NOT NULL",
		"actor_type VARCHAR(80) NOT NULL",
		"actor_id VARCHAR(255)",
		"runtime_node_id UUID",
		"provider_type VARCHAR(100)",
		"provider_session_id VARCHAR(255)",
		"input_summary TEXT",
		"output_summary TEXT",
		"error_family VARCHAR(100)",
		"error_code VARCHAR(100)",
		"error_message TEXT",
		"retryable BOOLEAN",
		"artifact_refs JSONB NOT NULL DEFAULT '[]'::jsonb",
		"evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb",
		"metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"idempotency_key VARCHAR(255) NOT NULL",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
	}
	for _, want := range expected {
		if !strings.Contains(block, want) {
			t.Fatalf("expected execution_ledger_events schema to contain %q, got block:\n%s", want, block)
		}
	}

	assertMigrationContains(t, sql, "CREATE UNIQUE INDEX uq_execution_ledger_events_idempotency")
	assertMigrationContains(t, sql, "CREATE INDEX idx_execution_ledger_events_project_time")
	assertMigrationContains(t, sql, "CREATE INDEX idx_execution_ledger_events_attempt_time")
	assertMigrationContains(t, sql, "CREATE INDEX idx_execution_ledger_events_project_type")
	assertMigrationContains(t, sql, "CREATE INDEX idx_execution_ledger_events_project_error")
	assertMigrationContains(t, sql, "CREATE TRIGGER update_execution_ledger_events_updated_at")
	assertMigrationContains(t, sql, "COMMENT ON TABLE execution_ledger_events IS '执行账本事件表，记录项目任务执行、Provider、工具、MCP、外部能力和证据链的统一审计索引。'")
	assertMigrationContains(t, sql, "COMMENT ON COLUMN execution_ledger_events.input_summary IS '输入摘要，不保存完整 prompt、secret 或大 payload。'")
	assertMigrationContains(t, sql, "COMMENT ON COLUMN execution_ledger_events.provider_session_id IS 'Provider 外部会话 ID。'")
	assertMigrationContains(t, sql, "COMMENT ON COLUMN execution_ledger_events.error_family IS '错误分类，例如 provider_error、runtime_error、missing_context、capability_denied。'")
	assertMigrationContains(t, sql, "COMMENT ON COLUMN execution_ledger_events.artifact_refs IS '工件引用数组。'")
	assertMigrationContains(t, sql, "COMMENT ON COLUMN execution_ledger_events.evidence_refs IS '证据引用数组。'")
	assertMigrationContains(t, sql, "COMMENT ON COLUMN execution_ledger_events.metadata IS '结构化扩展数据，禁止写入 secret 和完整 raw payload。'")
	assertMigrationContains(t, sql, "COMMENT ON COLUMN execution_ledger_events.idempotency_key IS '执行账本事件幂等键。'")
}

func TestExecutionLedgerAppendOnlyHardeningMigration(t *testing.T) {
	sql := readMigration(t, "030_execution_ledger_append_only_hardening.sql")

	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS update_execution_ledger_events_updated_at ON execution_ledger_events",
		"CREATE OR REPLACE FUNCTION create_execution_ledger_event",
		"RETURNS execution_ledger_events",
		"ON CONFLICT (tenant_id, idempotency_key) DO NOTHING",
		"RAISE EXCEPTION 'create_execution_ledger_event could not insert or find idempotency key % for tenant %'",
		"CREATE OR REPLACE FUNCTION reject_execution_ledger_events_mutation()",
		"RAISE EXCEPTION 'execution_ledger_events is append-only and does not allow %', TG_OP",
		"CREATE TRIGGER prevent_execution_ledger_events_update",
		"BEFORE UPDATE ON execution_ledger_events",
		"CREATE TRIGGER prevent_execution_ledger_events_delete",
		"BEFORE DELETE ON execution_ledger_events",
		"CREATE INDEX IF NOT EXISTS idx_execution_ledger_events_task_time",
		"ON execution_ledger_events(tenant_id, project_task_id, occurred_at ASC, created_at ASC)",
		"WHERE project_task_id IS NOT NULL",
	} {
		assertMigrationContains(t, sql, fragment)
	}

	querySQL, err := os.ReadFile("queries/execution_ledger.sql")
	if err != nil {
		t.Fatalf("read execution ledger queries: %v", err)
	}
	query := string(querySQL)
	for _, fragment := range []string{
		"WITH candidate_attempts AS",
		"eligible_attempts AS",
		"source_event_matches AS",
		"exact_match_count > 0",
		"AND exact_provider_session_match",
		"exact_match_count = 0",
		"AND NULLIF(attempt_provider_session_id, '') IS NULL",
		"source_event AS (",
		"FROM source_event_matches",
		"WHERE match_count = 1",
		"FROM source_event source\nCROSS JOIN LATERAL create_execution_ledger_event",
	} {
		assertMigrationContains(t, query, fragment)
	}
	if strings.Contains(query, "WHERE source.match_count = 1") {
		t.Fatal("provider ledger query must filter source_event before invoking create_execution_ledger_event")
	}
}

func TestSkillManagementMigrationAddsSkillPackageTables(t *testing.T) {
	body, err := os.ReadFile("migrations/009_skill_management.sql")
	if err != nil {
		t.Fatalf("read skill management migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS skills",
		"CREATE TABLE IF NOT EXISTS skill_files",
		"CREATE TABLE IF NOT EXISTS skill_team_bindings",
		"CREATE TABLE IF NOT EXISTS skill_agent_bindings",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"tags TEXT[] NOT NULL DEFAULT '{}'::text[]",
		"content TEXT NOT NULL DEFAULT ''",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_skills_tenant_slug_active",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_skill_files_tenant_skill_path",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_skill_team_bindings_tenant_skill_team",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_skill_agent_bindings_tenant_skill_employee",
		"COMMENT ON TABLE skills IS '技能包主表，记录可上传、安装和绑定到数字员工的技能定义'",
		"COMMENT ON COLUMN skills.tags IS '上传定义的技能标签数组，技能市场展示只使用此字段'",
		"COMMENT ON TABLE skill_files IS '技能包文件表，保存 SKILL.md、脚本和附加资源的可编辑文本内容'",
		"COMMENT ON TABLE skill_agent_bindings IS '技能安装到数字员工的绑定表'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected skill management migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"rating",
		"stars",
		"BIGSERIAL",
		"CREATE TYPE skill",
	} {
		if strings.Contains(strings.ToLower(sql), strings.ToLower(forbidden)) {
			t.Fatalf("skill migration must not contain %q", forbidden)
		}
	}
}

func TestSkillArchiveStorageMigrationRefactorsStorageModel(t *testing.T) {
	body, err := os.ReadFile("migrations/025_skill_archive_storage.sql")
	if err != nil {
		t.Fatalf("read skill archive storage migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"ADD COLUMN archive_object_ref TEXT",
		"ADD COLUMN archive_filename VARCHAR(255)",
		"ADD COLUMN archive_size_bytes BIGINT NOT NULL DEFAULT 0",
		"ADD COLUMN archive_checksum_sha256 VARCHAR(64) NOT NULL DEFAULT ''",
		"ADD COLUMN archive_file_count INTEGER NOT NULL DEFAULT 0",
		"COMMENT ON COLUMN skills.archive_object_ref IS '技能 zip 包在对象存储的 URI，例如 s3://bucket/skills/.../xxx.zip'",
		"DROP INDEX IF EXISTS idx_skills_tenant_status_updated",
		"ALTER TABLE skills DROP COLUMN status",
		"DROP TABLE IF EXISTS skill_files",
		"CREATE INDEX IF NOT EXISTS idx_skills_tenant_updated",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected skill archive storage migration to contain %q", expected)
		}
	}
}

func TestSkillInstallationsMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/035_skill_installations.sql")
	if err != nil {
		t.Fatalf("read skill installations migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE skill_installations",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"skill_id UUID NOT NULL",
		"target_scope VARCHAR(40) NOT NULL",
		"team_id UUID",
		"digital_employee_id UUID NOT NULL",
		"runtime_node_id UUID NOT NULL",
		"provider_type VARCHAR(80) NOT NULL",
		"installed_path TEXT NOT NULL",
		"archive_checksum_sha256 VARCHAR(64) NOT NULL",
		"status VARCHAR(40) NOT NULL DEFAULT 'installed'",
		"installed_by UUID",
		"installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"deleted_at TIMESTAMPTZ",
		"CREATE UNIQUE INDEX uq_skill_installations_active_employee",
		"CREATE INDEX idx_skill_installations_skill",
		"CREATE INDEX idx_skill_installations_employee",
		"CREATE INDEX idx_skill_installations_team",
		"COMMENT ON TABLE skill_installations IS",
		"COMMENT ON COLUMN skill_installations.id IS",
		"COMMENT ON COLUMN skill_installations.tenant_id IS",
		"COMMENT ON COLUMN skill_installations.skill_id IS",
		"COMMENT ON COLUMN skill_installations.target_scope IS",
		"COMMENT ON COLUMN skill_installations.team_id IS",
		"COMMENT ON COLUMN skill_installations.digital_employee_id IS",
		"COMMENT ON COLUMN skill_installations.runtime_node_id IS",
		"COMMENT ON COLUMN skill_installations.provider_type IS",
		"COMMENT ON COLUMN skill_installations.installed_path IS",
		"COMMENT ON COLUMN skill_installations.archive_checksum_sha256 IS",
		"COMMENT ON COLUMN skill_installations.status IS",
		"COMMENT ON COLUMN skill_installations.installed_by IS",
		"COMMENT ON COLUMN skill_installations.installed_at IS",
		"COMMENT ON COLUMN skill_installations.metadata IS",
		"COMMENT ON COLUMN skill_installations.created_at IS",
		"COMMENT ON COLUMN skill_installations.updated_at IS",
		"COMMENT ON COLUMN skill_installations.deleted_at IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected skill installations migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"CREATE TYPE skill_install",
		"ON DELETE CASCADE",
		"BIGSERIAL",
		"status VARCHAR(40) NOT NULL DEFAULT 'failed'",
		"skill_installations_provider_supported",
		"provider_type IN ('opencode', 'codex', 'claude-code')",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("skill installations migration must not contain %q", forbidden)
		}
	}
}

func TestDigitalEmployeeCreationQueriesHandlePolicyReasonsAndAbortAnchoring(t *testing.T) {
	body, err := os.ReadFile("queries/employee_execution.sql")
	if err != nil {
		t.Fatalf("read employee execution queries: %v", err)
	}
	sql := string(body)

	// AbortProvisionedDigitalEmployee / GetRuntimeProvisioningPreflight retired with
	// physical dei drop; only the create-options provider policy reasons remain.
	for _, expected := range []string{
		"THEN 'runtime_node_outside_team_policy'",
		"THEN 'runtime_node_slug_outside_team_policy'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected employee execution queries to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"AbortProvisionedDigitalEmployee",
		"GetRuntimeProvisioningPreflight",
		"digital_employee_execution_instances",
		"WHEN pc.id IS NULL THEN 'provider_missing'",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("employee execution query must not contain retired/unreachable fragment %q", forbidden)
		}
	}
}

func TestDigitalEmployeeRunLoopMigrationAddsPersistenceSchema(t *testing.T) {
	body, err := os.ReadFile("migrations/006_digital_employee_run_loop.sql")
	if err != nil {
		t.Fatalf("read digital employee run loop migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE runtime_command_receipts",
		"ALTER TABLE task_runs",
		"ADD COLUMN command_id VARCHAR(255)",
		"ADD COLUMN digital_employee_id UUID",
		"ADD COLUMN idempotency_fingerprint VARCHAR(255)",
		"ADD COLUMN diagnostic JSONB NOT NULL DEFAULT '{}'::jsonb",
		"ADD COLUMN work_products JSONB NOT NULL DEFAULT '[]'::jsonb",
		"ADD COLUMN session_state JSONB NOT NULL DEFAULT '{}'::jsonb",
		"ADD COLUMN provider_type VARCHAR(100)",
		"ALTER TABLE task_events",
		"ADD COLUMN raw_event_ref TEXT",
		"ALTER TABLE provider_sessions",
		"ADD COLUMN session_display_id VARCHAR(255)",
		"ADD COLUMN last_sequence_number INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE provider_session_events",
		"ADD COLUMN session_state_patch JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE UNIQUE INDEX uq_task_runs_command_id",
		"CREATE UNIQUE INDEX uq_task_runs_employee_idempotency",
		"DROP INDEX IF EXISTS uq_task_events_task_sequence",
		"DROP INDEX IF EXISTS uq_task_events_run_sequence",
		"CREATE UNIQUE INDEX uq_task_events_run_sequence",
		"ON task_events(tenant_id, run_id, sequence_number)",
		"CREATE UNIQUE INDEX uq_provider_session_events_command_sequence",
		"CREATE TRIGGER update_runtime_command_receipts_updated_at",
		"COMMENT ON TABLE runtime_command_receipts IS 'Runtime 命令回执表，记录下发、回写和终态结果'",
		"COMMENT ON COLUMN runtime_command_receipts.command_id IS '控制平面生成的命令ID'",
		"COMMENT ON COLUMN task_runs.command_id IS '运行关联的Runtime命令ID'",
		"COMMENT ON COLUMN task_runs.idempotency_fingerprint IS '运行创建幂等指纹'",
		"COMMENT ON COLUMN task_runs.provider_type IS '运行使用的Provider类型'",
		"COMMENT ON COLUMN task_events.metadata IS '任务事件扩展元数据'",
		"COMMENT ON COLUMN provider_sessions.session_state IS 'Provider适配器可恢复的会话状态'",
		"COMMENT ON COLUMN provider_session_events.session_state_patch IS '事件携带的会话状态增量'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected digital employee run loop migration to contain %q", expected)
		}
	}
}

func TestInitialSchemaPreservesHistoryWithoutHeavyForeignKeys(t *testing.T) {
	body, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(body)

	for _, forbidden := range []string{
		"task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE",
		"run_id UUID REFERENCES task_runs(id) ON DELETE CASCADE",
		"user_id UUID REFERENCES auth_users(id) ON DELETE SET NULL",
		"session_id UUID REFERENCES auth_sessions(id) ON DELETE SET NULL",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("history-preserving schema must not contain heavy FK dependency %q", forbidden)
		}
	}

	for _, table := range []string{
		"tasks",
		"task_runs",
		"task_state_history",
		"task_events",
		"task_artifacts",
		"audit_events",
		"web_login_logs",
		"web_operation_logs",
	} {
		block := createTableBlock(t, sql, table)
		if strings.Contains(block, " REFERENCES ") {
			t.Fatalf("%s must keep history with application-validated UUID references, got FK in block:\n%s", table, block)
		}
	}
}

func TestInitialSchemaUsesLifecycleTimestampsForCoreEntities(t *testing.T) {
	body, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"disabled_at TIMESTAMPTZ",
		"archived_at TIMESTAMPTZ",
		"revoked_at TIMESTAMPTZ",
		"deleted_at TIMESTAMPTZ",
		"finished_at TIMESTAMPTZ",
		"COMMENT ON COLUMN auth_users.disabled_at IS",
		"COMMENT ON COLUMN runtime_nodes.disabled_at IS",
		"COMMENT ON COLUMN tenant_teams.archived_at IS",
		"COMMENT ON COLUMN task_artifacts.deleted_at IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected lifecycle-aware initial schema to contain %q", expected)
		}
	}
}

func TestProviderSessionEventsRequireCorrelationID(t *testing.T) {
	body, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(body)
	block := createTableBlock(t, sql, "provider_session_events")

	for _, expected := range []string{
		"CONSTRAINT chk_provider_session_events_correlation_id",
		"CHECK (NULLIF(request_id, '') IS NOT NULL OR NULLIF(command_id, '') IS NOT NULL)",
	} {
		if !strings.Contains(block, expected) {
			t.Fatalf("expected provider_session_events schema to contain %q, got block:\n%s", expected, block)
		}
	}

	for _, expected := range []string{
		"COMMENT ON COLUMN provider_session_events.request_id IS '触发该事件的平台请求 ID，request_id 或 command_id 至少填写一个'",
		"COMMENT ON COLUMN provider_session_events.command_id IS '触发该事件的平台命令 ID，request_id 或 command_id 至少填写一个'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected provider session event comment to contain %q", expected)
		}
	}
}

func TestRuntimeEnrollmentRequiresBootstrapKey(t *testing.T) {
	body, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(body)
	block := createTableBlock(t, sql, "runtime_enrollments")

	if !strings.Contains(block, "bootstrap_key_id UUID NOT NULL") {
		t.Fatalf("expected runtime_enrollments.bootstrap_key_id to be NOT NULL, got block:\n%s", block)
	}
}

func TestProjectManagementV2GovernanceArchiveMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/015_project_management_v2_governance_archive.sql")
	if err != nil {
		t.Fatalf("read v2 migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE project_evidence_refs",
		"CREATE TABLE project_artifact_refs",
		"CREATE TABLE project_report_refs",
		"CREATE TABLE project_budget_ledger",
		"CREATE TABLE project_acceptance_records",
		"CREATE TABLE project_archive_snapshots",
		"CREATE TABLE artifact_retention_holds",
		"ALTER TABLE project_config_revisions",
		"tenant_id UUID NOT NULL",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"CREATE INDEX idx_project_evidence_refs_tenant_project_created",
		"CREATE INDEX idx_project_budget_ledger_tenant_project_created",
		"CREATE INDEX idx_project_archive_snapshots_tenant_project_created",
		"CREATE INDEX idx_artifact_retention_holds_tenant_artifact_active",
		"COMMENT ON TABLE project_evidence_refs IS",
		"COMMENT ON COLUMN project_archive_snapshots.retained_artifact_ids IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected V2 migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"CREATE TYPE project_evidence_status",
		"CREATE TYPE project_acceptance_status",
		"BIGSERIAL PRIMARY KEY",
		"ON DELETE CASCADE",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("V2 migration must not contain %q", forbidden)
		}
	}
}

func TestUserProjectTeamScopesMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/021_user_project_team_scopes.sql")
	if err != nil {
		t.Fatalf("read user project team scopes migration: %v", err)
	}
	sql := string(body)
	block := createTableBlock(t, sql, "user_project_team_scopes")

	for _, expected := range []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"user_id UUID NOT NULL",
		"team_id UUID NOT NULL",
		"status VARCHAR(50) NOT NULL DEFAULT 'active'",
		"granted_by_user_id UUID",
		"revoked_at TIMESTAMPTZ",
		"CREATE UNIQUE INDEX uq_user_project_team_scopes_active",
		"CREATE INDEX idx_user_project_team_scopes_tenant_user_status",
		"CREATE INDEX idx_user_project_team_scopes_tenant_team_status",
		"COMMENT ON TABLE user_project_team_scopes IS",
		"COMMENT ON COLUMN user_project_team_scopes.team_id IS",
	} {
		if !strings.Contains(sql, expected) && !strings.Contains(block, expected) {
			t.Fatalf("expected user project team scope migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"BIGSERIAL",
		"tenant_team_members",
		"tenant_members",
	} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("user_project_team_scopes must not contain %q", forbidden)
		}
	}
}

func TestUserProjectTeamScopesQueriesAreNilSafeAndTenantScoped(t *testing.T) {
	body, err := os.ReadFile("queries/user_project_team_scopes.sql")
	if err != nil {
		t.Fatalf("read user project team scopes queries: %v", err)
	}
	sql := string(body)

	if !strings.Contains(sql, "ANY(COALESCE(sqlc.arg('team_ids')::uuid[], ARRAY[]::uuid[]))") {
		t.Fatal("RevokeUserProjectTeamScopes must treat nil team_ids as an empty UUID array")
	}

	if count := strings.Count(sql, "tenant_id = sqlc.arg('tenant_id')::uuid"); count < 4 {
		t.Fatalf("expected tenant-scoped aggregate CTEs plus final filters, got %d tenant filters", count)
	}
}

func TestRuntimeLeasesCleanupMigrationDropsLegacyTable(t *testing.T) {
	sql := migrationsSQL(t)
	if !strings.Contains(sql, "DROP TABLE IF EXISTS runtime_leases") {
		t.Fatal("expected a forward migration to drop legacy runtime_leases table")
	}
}

func createTableBlock(t *testing.T, sql string, table string) string {
	t.Helper()
	startMarker := "CREATE TABLE " + table + " ("
	start := strings.Index(sql, startMarker)
	if start == -1 {
		t.Fatalf("missing %s", startMarker)
	}
	rest := sql[start:]
	end := strings.Index(rest, "\n);")
	if end == -1 {
		t.Fatalf("missing end of %s create table block", table)
	}
	return rest[:end]
}

func readMigration(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile("migrations/" + name)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return string(body)
}

func assertMigrationContains(t *testing.T, sql string, fragment string) {
	t.Helper()
	if !strings.Contains(sql, fragment) {
		t.Fatalf("expected migration to contain %q", fragment)
	}
}

func migrationsSQL(t *testing.T) string {
	t.Helper()

	entries, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	var builder strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		body, err := os.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		builder.Write(body)
		builder.WriteString("\n")
	}

	return builder.String()
}

func TestInitialSchemaMetadataAndComments(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run executable UUID-first schema metadata/comment contract test")
	}

	body, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	schemaName := "uuid_contract_" + strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_"), "-", "_")
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire test database connection: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`); err != nil {
		t.Fatalf("drop test schema: %v", err)
	}
	if _, err := conn.Exec(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)

	if _, err := conn.Exec(ctx, `SET search_path TO `+schemaName); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := conn.Exec(ctx, string(body)); err != nil {
		t.Fatalf("apply initial migration: %v", err)
	}

	for _, table := range uuidFirstTables {
		var exists bool
		if err := pool.QueryRow(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.tables
	WHERE table_schema = $1 AND table_name = $2
)`, schemaName, table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected table %s to exist", table)
		}

		var dataType, columnDefault string
		if err := pool.QueryRow(ctx, `
SELECT data_type, COALESCE(column_default, '')
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2 AND column_name = 'id'`,
			schemaName, table,
		).Scan(&dataType, &columnDefault); err != nil {
			t.Fatalf("read %s.id metadata: %v", table, err)
		}
		if dataType != "uuid" {
			t.Fatalf("%s.id data type = %q, want uuid", table, dataType)
		}
		if !strings.Contains(columnDefault, "gen_random_uuid()") {
			t.Fatalf("%s.id default = %q, want gen_random_uuid()", table, columnDefault)
		}
	}

	expectedUUIDColumns := map[string]string{
		"auth_sessions":      "id",
		"auth_sessions_user": "user_id",
		"web_login_logs":     "session_id",
		"web_operation_logs": "user_id",
		"tasks":              "creator_id",
		"task_events":        "run_id",
	}
	for table, column := range expectedUUIDColumns {
		actualTable := table
		if table == "auth_sessions_user" {
			actualTable = "auth_sessions"
		}
		assertColumnType(t, pool, schemaName, actualTable, column, "uuid")
	}
	assertColumnType(t, pool, schemaName, "task_runs", "updated_at", "timestamp with time zone")

	for _, table := range uuidFirstTables {
		var comment string
		if err := pool.QueryRow(ctx, `
SELECT COALESCE(obj_description(format('%I.%I', $1::text, $2::text)::regclass, 'pg_class'), '')`,
			schemaName, table,
		).Scan(&comment); err != nil {
			t.Fatalf("read table comment for %s: %v", table, err)
		}
		if strings.TrimSpace(comment) == "" {
			t.Fatalf("expected table %s to have a non-empty comment", table)
		}
	}

	rows, err := pool.Query(ctx, `
SELECT c.table_name, c.column_name
FROM information_schema.columns c
JOIN information_schema.tables t
	ON t.table_schema = c.table_schema
	AND t.table_name = c.table_name
WHERE c.table_schema = $1
	AND t.table_type = 'BASE TABLE'
	AND c.table_name = ANY($2)
	AND col_description(format('%I.%I', c.table_schema, c.table_name)::regclass, c.ordinal_position) IS NULL
ORDER BY c.table_name, c.ordinal_position`, schemaName, uuidFirstTables)
	if err != nil {
		t.Fatalf("query uncommented columns: %v", err)
	}
	defer rows.Close()

	var uncommented []string
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatalf("scan uncommented column: %v", err)
		}
		uncommented = append(uncommented, table+"."+column)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate uncommented columns: %v", err)
	}
	if len(uncommented) > 0 {
		t.Fatalf("expected all SuperTeam-owned columns to have comments, missing: %s", strings.Join(uncommented, ", "))
	}
}

func applyAllMigrations(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run executable migration schema tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

	schemaName := "migration_contract_" + strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_"), "-", "_")
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire test database connection: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`); err != nil {
		t.Fatalf("drop test schema: %v", err)
	}
	if _, err := conn.Exec(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	})

	if _, err := conn.Exec(ctx, `SET search_path TO `+schemaName); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := conn.Exec(ctx, migrationsSQL(t)); err != nil {
		t.Fatalf("apply all migrations: %v", err)
	}

	return pool, schemaName
}

func assertTableExists(t *testing.T, pool *pgxpool.Pool, schemaName, table string) {
	t.Helper()

	var exists bool
	if err := pool.QueryRow(context.Background(), `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.tables
	WHERE table_schema = $1 AND table_name = $2
)`, schemaName, table).Scan(&exists); err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	require.Truef(t, exists, "expected table %s to exist", table)
}

func assertTableAbsent(t *testing.T, pool *pgxpool.Pool, schemaName, table string) {
	t.Helper()

	var exists bool
	if err := pool.QueryRow(context.Background(), `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.tables
	WHERE table_schema = $1 AND table_name = $2
)`, schemaName, table).Scan(&exists); err != nil {
		t.Fatalf("check table %s absence: %v", table, err)
	}
	require.Falsef(t, exists, "expected table %s to be absent", table)
}

func assertColumnExists(t *testing.T, pool *pgxpool.Pool, schemaName, table, column string) {
	t.Helper()

	var exists bool
	if err := pool.QueryRow(context.Background(), `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.columns
	WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
)`, schemaName, table, column).Scan(&exists); err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	require.Truef(t, exists, "expected column %s.%s to exist", table, column)
}

func assertColumnType(t *testing.T, pool *pgxpool.Pool, schemaName, table, column, want string) {
	t.Helper()

	var dataType string
	if err := pool.QueryRow(context.Background(), `
SELECT data_type
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2 AND column_name = $3`,
		schemaName, table, column,
	).Scan(&dataType); err != nil {
		t.Fatalf("read %s.%s metadata: %v", table, column, err)
	}
	if dataType != want {
		t.Fatalf("%s.%s data type = %q, want %q", table, column, dataType, want)
	}
}

func TestDevAdminSeedMigrationIsIdempotentAndUsesBcrypt(t *testing.T) {
	body, err := os.ReadFile("migrations/002_seed_dev_admin.sql")
	if err != nil {
		t.Fatalf("read dev admin seed migration: %v", err)
	}
	sql := string(body)

	if !strings.Contains(sql, "ON CONFLICT (username) WHERE deleted_at IS NULL DO NOTHING") {
		t.Fatal("expected dev admin seed migration to be idempotent")
	}
	if strings.Contains(sql, "password_hash, status) VALUES ('admin', 'admin'") {
		t.Fatal("expected default admin password to be stored as a bcrypt hash, not plain text")
	}

	hash := bcryptHashPattern.FindString(sql)
	if hash == "" {
		t.Fatal("expected default admin bcrypt hash in migration")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("admin")); err != nil {
		t.Fatalf("expected default admin bcrypt hash to match admin password: %v", err)
	}
}

func TestProjectManagementV0MigrationDefinesProjectFactsAndEvents(t *testing.T) {
	body, err := os.ReadFile("migrations/013_project_management_v0.sql")
	if err != nil {
		t.Fatalf("read project management migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE projects",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"team_id UUID",
		"human_owner_user_id UUID NOT NULL",
		"coordination_workflow_id VARCHAR(255)",
		"CREATE TABLE project_members",
		"principal_type VARCHAR(50) NOT NULL",
		"project_role VARCHAR(50) NOT NULL",
		"CREATE TABLE project_tasks",
		"runtime_task_id UUID",
		"digital_employee_run_id UUID",
		"CREATE TABLE project_events",
		"sequence_number BIGINT NOT NULL",
		"payload JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE UNIQUE INDEX uq_project_events_project_sequence",
		"ON project_events(project_id, sequence_number)",
		"CREATE INDEX idx_project_events_tenant_project_sequence",
		"CREATE TABLE project_demands",
		"submitted_by_user_id UUID NOT NULL",
		"source_refs JSONB NOT NULL DEFAULT '{}'::jsonb",
		"attachments JSONB NOT NULL DEFAULT '[]'::jsonb",
		"CREATE TABLE project_config_revisions",
		"revision_number INTEGER NOT NULL",
		"config_snapshot JSONB NOT NULL",
		"COMMENT ON TABLE projects IS",
		"COMMENT ON COLUMN project_events.sequence_number IS",
		"COMMENT ON COLUMN projects.coordination_policy IS",
		"COMMENT ON COLUMN project_members.settings IS",
		"COMMENT ON COLUMN project_tasks.assigned_digital_employee_id IS",
		"COMMENT ON COLUMN project_events.payload IS",
		"COMMENT ON COLUMN project_demands.attachments IS",
		"COMMENT ON COLUMN project_config_revisions.config_snapshot IS",
		"coordination_policy JSONB NOT NULL DEFAULT '{}'::jsonb",
		"approval_policy JSONB NOT NULL DEFAULT '{}'::jsonb",
		"evidence_policy JSONB NOT NULL DEFAULT '{}'::jsonb",
		"settings JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE TRIGGER update_projects_updated_at BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();",
		"CREATE TRIGGER update_project_members_updated_at BEFORE UPDATE ON project_members FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();",
		"CREATE TRIGGER update_project_tasks_updated_at BEFORE UPDATE ON project_tasks FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();",
		"CREATE TRIGGER update_project_demands_updated_at BEFORE UPDATE ON project_demands FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected project management migration to contain %q", expected)
		}
	}

	projectColumns := map[string][]string{
		"projects": {
			"id",
			"tenant_id",
			"team_id",
			"name",
			"description",
			"goal",
			"status",
			"human_owner_user_id",
			"leader_user_id",
			"acceptance_user_id",
			"coordination_workflow_id",
			"coordination_status",
			"coordination_policy",
			"approval_policy",
			"evidence_policy",
			"archived_at",
			"created_at",
			"updated_at",
		},
		"project_members": {
			"id",
			"tenant_id",
			"project_id",
			"principal_type",
			"principal_id",
			"project_role",
			"display_name_snapshot",
			"status",
			"settings",
			"created_at",
			"updated_at",
		},
		"project_tasks": {
			"id",
			"tenant_id",
			"project_id",
			"demand_id",
			"title",
			"summary",
			"status",
			"assigned_digital_employee_id",
			"runtime_task_id",
			"digital_employee_run_id",
			"risk_level",
			"requires_human_approval",
			"latest_event_id",
			"created_at",
			"updated_at",
		},
		"project_events": {
			"id",
			"tenant_id",
			"project_id",
			"sequence_number",
			"event_type",
			"actor_type",
			"actor_id",
			"resource_type",
			"resource_id",
			"summary",
			"payload",
			"created_at",
		},
		"project_demands": {
			"id",
			"tenant_id",
			"project_id",
			"submitted_by_user_id",
			"title",
			"content",
			"source_type",
			"source_refs",
			"attachments",
			"priority",
			"risk_level",
			"status",
			"created_event_id",
			"created_at",
			"updated_at",
		},
		"project_config_revisions": {
			"id",
			"tenant_id",
			"project_id",
			"revision_number",
			"config_snapshot",
			"change_summary",
			"created_by_user_id",
			"created_event_id",
			"created_at",
		},
	}
	for table, columns := range projectColumns {
		for _, column := range columns {
			expected := "COMMENT ON COLUMN " + table + "." + column + " IS"
			if !strings.Contains(sql, expected) {
				t.Fatalf("expected project management migration to contain %q", expected)
			}
		}
	}

	for _, forbidden := range []string{
		"coordinator_employee_id",
		"project_role VARCHAR(50) NOT NULL CHECK",
		"CREATE TYPE project_status",
		"BIGSERIAL PRIMARY KEY",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("project management migration must not contain %q", forbidden)
		}
	}
}

func TestProjectManagementV1TemporalCoordinationMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/014_project_management_v1_temporal_coordination.sql")
	if err != nil {
		t.Fatalf("read project management v1 migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE approval_requests",
		"CREATE TABLE approval_decisions",
		"CREATE TABLE project_coordination_jobs",
		"CREATE TABLE project_route_decisions",
		"candidate_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb",
		"selected_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb",
		"CREATE TABLE project_execution_summaries",
		"CREATE TABLE project_transfer_requests",
		"CREATE TABLE project_decision_requests",
		"approval_request_id UUID NOT NULL",
		"CREATE INDEX idx_project_route_decisions_tenant_project_created",
		"CREATE INDEX idx_project_decision_requests_tenant_project_status",
		"COMMENT ON TABLE approval_requests IS",
		"COMMENT ON COLUMN project_decision_requests.approval_request_id IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected v1 migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"CREATE TYPE approval_status",
		"CREATE TYPE project_coordination_job_status",
		"BIGSERIAL PRIMARY KEY",
		"REFERENCES digital_employees",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("v1 migration must not contain %q", forbidden)
		}
	}
}

func TestMigrationProjectRuntimeNodes(t *testing.T) {
	pool, schemaName := applyAllMigrations(t)
	assertTableExists(t, pool, schemaName, "project_runtime_nodes")
	assertTableExists(t, pool, schemaName, "project_employee_node_affinity")
}

func TestMigration049BackfillProjectRuntimeNodesFromPlacements(t *testing.T) {
	sql := readMigration(t, "049_backfill_project_runtime_nodes_from_placements.sql")

	for _, expected := range []string{
		"INSERT INTO project_runtime_nodes (tenant_id, project_id, runtime_node_id)",
		"SELECT pp.tenant_id, pp.project_id, pp.runtime_node_id",
		"FROM project_placements pp",
		"WHERE pp.placement_status = 'active'",
		"AND pp.released_at IS NULL",
		"ON CONFLICT (project_id, runtime_node_id) DO NOTHING",
	} {
		assertMigrationContains(t, sql, expected)
	}
}

func TestProjectSoftDeleteMigration(t *testing.T) {
	sql := readMigration(t, "057_project_soft_delete.sql")

	for _, expected := range []string{
		"ALTER TABLE projects",
		"ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ",
		"COMMENT ON COLUMN projects.deleted_at IS '软删除时间；非空表示项目已从当前管理面移除'",
		"CREATE INDEX IF NOT EXISTS idx_projects_tenant_deleted_created",
		"ON projects (tenant_id, created_at DESC)",
		"WHERE deleted_at IS NULL",
	} {
		assertMigrationContains(t, sql, expected)
	}
}

func TestProjectSoftDeleteQueriesFilterCurrentSurface(t *testing.T) {
	projectSQL, err := os.ReadFile("queries/project.sql")
	require.NoError(t, err)
	authzSQL, err := os.ReadFile("queries/authz.sql")
	require.NoError(t, err)
	affinitySQL, err := os.ReadFile("queries/project_runtime_affinity.sql")
	require.NoError(t, err)

	for _, expected := range []string{
		`-- name: GetProject :one
SELECT * FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;`,
		`WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)`,
		"JOIN projects p ON p.tenant_id = d.tenant_id AND p.id = d.project_id",
		"AND p.deleted_at IS NULL",
	} {
		require.Contains(t, string(projectSQL), expected)
	}
	require.Contains(t, string(authzSQL), "AND p.deleted_at IS NULL")
	require.Contains(t, string(affinitySQL), "AND deleted_at IS NULL")
}

func TestMigration079CapabilityBindingUnification(t *testing.T) {
	sql := readMigration(t, "079_capability_binding_unification.sql")

	for _, expected := range []string{
		"INSERT INTO skill_agent_bindings (tenant_id, skill_id, digital_employee_id, status)",
		"INSERT INTO digital_employee_mcp_bindings_v2 (tenant_id, digital_employee_id, mcp_server_id)",
		"jsonb_typeof(r.capability_bindings->'skills') = 'array'",
		"jsonb_typeof(r.capability_bindings->'mcp_servers') = 'array'",
		"s.deleted_at IS NULL",
		"de.deleted_at IS NULL",
		"m.status = 'active'",
		"ON CONFLICT (tenant_id, skill_id, digital_employee_id)",
		"ON CONFLICT (tenant_id, digital_employee_id, mcp_server_id) WHERE deleted_at IS NULL",
		"SET capability_bindings = capability_bindings - 'skills' - 'mcp_servers'",
	} {
		assertMigrationContains(t, sql, expected)
	}
}

func TestMigration086DropsSkillInstallations(t *testing.T) {
	pool, schemaName := applyAllMigrations(t)

	assertTableAbsent(t, pool, schemaName, "skill_installations")
	assertTableExists(t, pool, schemaName, "skill_agent_bindings")
}
