package storage

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

var bcryptHashPattern = regexp.MustCompile(`\$2[aby]\$[0-9]{2}\$[A-Za-z0-9./]{53}`)

func TestDevAdminSeedMigrationIsIdempotentAndUsesBcrypt(t *testing.T) {
	body, err := os.ReadFile("migrations/002_seed_dev_admin.sql")
	if err != nil {
		t.Fatalf("read dev admin seed migration: %v", err)
	}
	sql := string(body)

	if !strings.Contains(sql, "ON CONFLICT (username) DO NOTHING") {
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

func TestAuthSessionsHaveForwardOnlyMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/003_create_auth_sessions.sql")
	if err != nil {
		t.Fatalf("read auth sessions migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS auth_sessions",
		"user_id BIGINT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE",
		"token_hash VARCHAR(255) UNIQUE NOT NULL",
		"CREATE INDEX IF NOT EXISTS idx_auth_sessions_token_hash",
		"CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires_at",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected auth sessions migration to contain %q", expected)
		}
	}
}

func TestWebLogsHaveForwardOnlyMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/004_create_web_logs.sql")
	if err != nil {
		t.Fatalf("read web logs migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS web_login_logs",
		"event_type VARCHAR(50) NOT NULL CHECK (event_type IN ('login_succeeded', 'login_failed', 'logout_succeeded'))",
		"user_id BIGINT REFERENCES auth_users(id) ON DELETE SET NULL",
		"CREATE INDEX IF NOT EXISTS idx_web_login_logs_event_type_created",
		"CREATE TABLE IF NOT EXISTS web_operation_logs",
		"module VARCHAR(100) NOT NULL",
		"CREATE INDEX IF NOT EXISTS idx_web_operation_logs_module_action_created",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected web logs migration to contain %q", expected)
		}
	}
}

func TestAuthUserManagementCommentsMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/005_comment_auth_users_and_web_operation_logs.sql")
	if err != nil {
		t.Fatalf("read auth user comments migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"COMMENT ON TABLE auth_users IS 'Web 控制台平台用户表'",
		"COMMENT ON COLUMN auth_users.password_hash IS '用户密码哈希，禁止存储明文密码'",
		"COMMENT ON TABLE web_operation_logs IS 'Web 控制台操作日志表'",
		"COMMENT ON COLUMN web_operation_logs.action IS '操作动作'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected auth user comments migration to contain %q", expected)
		}
	}
}
