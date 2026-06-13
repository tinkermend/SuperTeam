package storage

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
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
	"digital_employee_execution_instances",
	"provider_sessions",
	"provider_session_events",
	"tasks",
	"task_runs",
	"runtime_leases",
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
		"CREATE TABLE runtime_leases",
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
		"ORDER BY last_activity_at DESC, created_at DESC, id DESC",
		`-- name: CountInboxItems :one
SELECT COUNT(*)::bigint FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = sqlc.arg('status')::varchar
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
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

func TestDualLayerSkillManagementMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/013_dual_layer_skill_management.sql")
	if err != nil {
		t.Fatalf("read dual layer skill management migration: %v", err)
	}
	sql := string(body)

	required := []string{
		"CREATE TABLE IF NOT EXISTS user_credentials",
		"CREATE TABLE IF NOT EXISTS team_mcp_servers",
		"CREATE TABLE IF NOT EXISTS digital_employee_mcp_bindings",
		"CREATE TABLE IF NOT EXISTS digital_employee_instruction_files",
		"encrypted_value TEXT NOT NULL",
		"credential_id UUID",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_user_credentials_owner_name_active",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_team_mcp_servers_team_name_active",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_mcp_bindings_employee_name_active",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_instruction_files_path_active",
		"COMMENT ON TABLE user_credentials IS '个人凭据池，保存用户可复用的外部能力授权令牌密文'",
		"COMMENT ON TABLE team_mcp_servers IS '团队公共 MCP 服务器配置，团队下数字员工强制继承'",
		"COMMENT ON TABLE digital_employee_mcp_bindings IS '数字员工个人 MCP 服务器配置'",
		"COMMENT ON TABLE digital_employee_instruction_files IS '数字员工个人 Instructions 文件内容'",
	}
	for _, expected := range required {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %q", expected)
		}
	}

	forbidden := []string{
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

func TestDigitalEmployeeCreationQueriesHandlePolicyReasonsAndAbortAnchoring(t *testing.T) {
	body, err := os.ReadFile("queries/employee_execution.sql")
	if err != nil {
		t.Fatalf("read employee execution queries: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"COALESCE((",
		"THEN 'runtime_node_outside_team_policy'",
		"THEN 'runtime_node_slug_outside_team_policy'",
		"sqlc.arg('execution_instance_id')::uuid = '00000000-0000-0000-0000-000000000000'::uuid AS abort_by_employee",
		"AND NOT abort_args.abort_by_employee",
		"abort_args.abort_by_employee OR EXISTS (SELECT 1 FROM aborted_instance)",
		"rcr.resource_id = ai.id",
		"AND EXISTS (SELECT 1 FROM aborted_employee)",
		"AND EXISTS (SELECT 1 FROM abort_scope WHERE matched)",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected employee execution queries to contain %q", expected)
		}
	}

	if got := strings.Count(sql, "COALESCE(("); got < 2 {
		t.Fatalf("expected provider policy predicate to be COALESCE-normalized in availability and disabled reason, got %d occurrences", got)
	}

	for _, forbidden := range []string{
		"WHEN pc.id IS NULL THEN 'provider_missing'",
		"resource_id = sqlc.arg('execution_instance_id')::uuid",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("employee execution query must not contain unreachable branch %q", forbidden)
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
		"runtime_leases",
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
