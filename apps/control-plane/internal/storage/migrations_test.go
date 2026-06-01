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
	"auth_users",
	"tenant_members",
	"runtime_nodes",
	"runtime_node_scopes",
	"auth_runtime_tokens",
	"auth_sessions",
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
