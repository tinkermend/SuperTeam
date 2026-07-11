package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadFromEnvBuildsControlPlaneConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CONTROL_PLANE_ADDR", ":9090")
	t.Setenv("DATABASE_URL", "postgres://superteam:secret@127.0.0.1:5432/superteam?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://:secret@127.0.0.1:6379/0")
	t.Setenv("S3_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("S3_BUCKET", "superteam-artifacts")
	t.Setenv("S3_ACCESS_KEY_ID", "minio")
	t.Setenv("S3_SECRET_ACCESS_KEY", "minio-secret")
	t.Setenv("S3_FORCE_PATH_STYLE", "true")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}

	if cfg.HTTP.Addr != ":9090" {
		t.Fatalf("expected addr :9090, got %q", cfg.HTTP.Addr)
	}
	if cfg.Postgres.URL == "" {
		t.Fatal("expected postgres URL")
	}
	if cfg.Redis.URL == "" {
		t.Fatal("expected redis URL")
	}
	if cfg.ObjectStore.Endpoint != "http://127.0.0.1:9000" {
		t.Fatalf("expected S3 endpoint, got %q", cfg.ObjectStore.Endpoint)
	}
	if !cfg.ObjectStore.ForcePathStyle {
		t.Fatal("expected S3 path-style addressing")
	}
}

func TestLoadFromEnvPlannerDefaultsAreProviderNeutral(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadFromEnv()

	require.NoError(t, err)
	require.Equal(t, "openai-compatible", cfg.Planner.Provider)
	require.Empty(t, cfg.Planner.BaseURL)
	require.Empty(t, cfg.Planner.Model)
	require.Equal(t, 8192, cfg.Planner.MaxTokens)
	require.Equal(t, 2, cfg.Planner.MaxAttempts)
}

func TestLoadFromEnvAuthzDefaultsToDBEngine(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadFromEnv()

	require.NoError(t, err)
	require.Equal(t, "db", cfg.Authz.Engine)
	require.Empty(t, cfg.Authz.OpenFGA.APIURL)
	require.Empty(t, cfg.Authz.OpenFGA.StoreID)
	require.Empty(t, cfg.Authz.OpenFGA.ModelID)
}

func TestLoadFromEnvAuthCaptchaDefaultsToDisabled(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadFromEnv()

	require.NoError(t, err)
	require.False(t, cfg.Auth.CaptchaEnabled)
}

func TestLoadFromEnvAuthCaptchaEnabledOverride(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AUTH_CAPTCHA_ENABLED", "true")

	cfg, err := LoadFromEnv()

	require.NoError(t, err)
	require.True(t, cfg.Auth.CaptchaEnabled)
}

func TestLoadFromEnvAuthCaptchaDisabledOverride(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AUTH_CAPTCHA_ENABLED", "false")

	cfg, err := LoadFromEnv()

	require.NoError(t, err)
	require.False(t, cfg.Auth.CaptchaEnabled)
}

func TestLoadFromEnvAuthzOpenFGAShadowConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("AUTHZ_ENGINE", "openfga_shadow")
	t.Setenv("OPENFGA_API_URL", "http://127.0.0.1:8088")
	t.Setenv("OPENFGA_STORE_ID", "store-1")
	t.Setenv("OPENFGA_MODEL_ID", "model-1")
	t.Setenv("OPENFGA_API_TOKEN", "token-1")

	cfg, err := LoadFromEnv()

	require.NoError(t, err)
	require.Equal(t, "openfga_shadow", cfg.Authz.Engine)
	require.Equal(t, "http://127.0.0.1:8088", cfg.Authz.OpenFGA.APIURL)
	require.Equal(t, "store-1", cfg.Authz.OpenFGA.StoreID)
	require.Equal(t, "model-1", cfg.Authz.OpenFGA.ModelID)
	require.Equal(t, "token-1", cfg.Authz.OpenFGA.APIToken)
}

func TestLoadFromEnvRequiresStorageConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("S3_BUCKET", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected missing storage configuration to fail")
	}
}

func TestLoadFromEnvRequiresObjectStoreConnectionConfiguration(t *testing.T) {
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_KEYS", "v1:test-key")
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID", "v1")
	t.Setenv("DATABASE_URL", "postgres://superteam:secret@127.0.0.1:5432/superteam?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://:secret@127.0.0.1:6379/0")
	t.Setenv("S3_BUCKET", "superteam-artifacts")
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_REGION", "")
	t.Setenv("S3_ACCESS_KEY_ID", "")
	t.Setenv("S3_SECRET_ACCESS_KEY", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected missing object store connection configuration to fail")
	}
}

func TestLoadFromFileReadsControlPlaneYAML(t *testing.T) {
	path := writeConfigFile(t, `
http:
  addr: ":9090"
postgres:
  url: "postgres://superteam:secret@127.0.0.1:5432/superteam?sslmode=disable"
redis:
  url: "redis://:secret@127.0.0.1:6379/0"
objectStore:
  endpoint: "http://127.0.0.1:9000"
  region: "us-east-1"
  bucket: "superteam-artifacts"
  accessKeyId: "minio"
  secretAccessKey: "minio-secret"
  forcePathStyle: true
employeeEnv:
  keys: "v1:file-key"
  activeKeyId: "v1"
auth:
  captchaEnabled: false
`)

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("expected config file to load: %v", err)
	}

	if cfg.HTTP.Addr != ":9090" {
		t.Fatalf("expected addr from file, got %q", cfg.HTTP.Addr)
	}
	if cfg.Postgres.URL == "" {
		t.Fatal("expected postgres URL from file")
	}
	if cfg.Redis.URL == "" {
		t.Fatal("expected redis URL from file")
	}
	if cfg.ObjectStore.Bucket != "superteam-artifacts" {
		t.Fatalf("expected object store bucket, got %q", cfg.ObjectStore.Bucket)
	}
	if !cfg.ObjectStore.ForcePathStyle {
		t.Fatal("expected path-style object store config")
	}
	require.Equal(t, "v1:file-key", cfg.EmployeeEnv.Keys)
	require.Equal(t, "v1", cfg.EmployeeEnv.ActiveKeyID)
	require.False(t, cfg.Auth.CaptchaEnabled)
}

func TestLoadFromFileCaptchaEnabledTrue(t *testing.T) {
	path := writeConfigFile(t, `
http:
  addr: ":9090"
postgres:
  url: "postgres://superteam:secret@127.0.0.1:5432/superteam?sslmode=disable"
redis:
  url: "redis://:secret@127.0.0.1:6379/0"
objectStore:
  endpoint: "http://127.0.0.1:9000"
  region: "us-east-1"
  bucket: "superteam-artifacts"
  accessKeyId: "minio"
  secretAccessKey: "minio-secret"
  forcePathStyle: true
employeeEnv:
  keys: "v1:file-key"
  activeKeyId: "v1"
auth:
  captchaEnabled: true
`)

	cfg, err := LoadFromFile(path)
	require.NoError(t, err)
	require.True(t, cfg.Auth.CaptchaEnabled)
}

func TestLoadFromFileCaptchaDefaultsDisabledWhenOmitted(t *testing.T) {
	path := writeConfigFile(t, `
http:
  addr: ":9090"
postgres:
  url: "postgres://superteam:secret@127.0.0.1:5432/superteam?sslmode=disable"
redis:
  url: "redis://:secret@127.0.0.1:6379/0"
objectStore:
  endpoint: "http://127.0.0.1:9000"
  region: "us-east-1"
  bucket: "superteam-artifacts"
  accessKeyId: "minio"
  secretAccessKey: "minio-secret"
  forcePathStyle: true
employeeEnv:
  keys: "v1:file-key"
  activeKeyId: "v1"
`)

	cfg, err := LoadFromFile(path)
	require.NoError(t, err)
	require.False(t, cfg.Auth.CaptchaEnabled)
}

func TestLoadFromFileAllowsEnvOverrides(t *testing.T) {
	path := writeConfigFile(t, `
http:
  addr: ":8080"
postgres:
  url: "postgres://file"
redis:
  url: "redis://file"
objectStore:
  endpoint: "http://127.0.0.1:9000"
  region: "us-east-1"
  bucket: "file-bucket"
  accessKeyId: "file-ak"
  secretAccessKey: "file-sk"
employeeEnv:
  keys: "v1:file-key"
  activeKeyId: "v1"
`)
	t.Setenv("CONTROL_PLANE_ADDR", ":7070")
	t.Setenv("DATABASE_URL", "postgres://env")
	t.Setenv("REDIS_URL", "redis://env")
	t.Setenv("S3_BUCKET", "env-bucket")
	t.Setenv("S3_FORCE_PATH_STYLE", "true")
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_KEYS", "v2:env-key")
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID", "v2")

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("expected config file to load: %v", err)
	}

	if cfg.HTTP.Addr != ":7070" {
		t.Fatalf("expected env addr override, got %q", cfg.HTTP.Addr)
	}
	if cfg.Postgres.URL != "postgres://env" {
		t.Fatalf("expected env postgres override, got %q", cfg.Postgres.URL)
	}
	if cfg.Redis.URL != "redis://env" {
		t.Fatalf("expected env redis override, got %q", cfg.Redis.URL)
	}
	if cfg.ObjectStore.Bucket != "env-bucket" {
		t.Fatalf("expected env bucket override, got %q", cfg.ObjectStore.Bucket)
	}
	if !cfg.ObjectStore.ForcePathStyle {
		t.Fatal("expected env bool override")
	}
	require.Equal(t, "v2:env-key", cfg.EmployeeEnv.Keys)
	require.Equal(t, "v2", cfg.EmployeeEnv.ActiveKeyID)
}

func TestLoadFromFilePlannerConfig(t *testing.T) {
	setRequiredEnv(t)

	path := writeTempConfig(t, `
planner:
  provider: qwen
  apiKey: file-key
  baseURL: https://dashscope.aliyuncs.com/compatible-mode/v1
  model: qwen-plus
  maxTokens: 8192
  temperature: 0.1
  maxAttempts: 2
`)

	cfg, err := LoadFromFile(path)
	require.NoError(t, err)
	require.Equal(t, "qwen", cfg.Planner.Provider)
	require.Equal(t, "file-key", cfg.Planner.APIKey)
	require.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1", cfg.Planner.BaseURL)
	require.Equal(t, "qwen-plus", cfg.Planner.Model)
	require.Equal(t, 8192, cfg.Planner.MaxTokens)
	require.InDelta(t, 0.1, cfg.Planner.Temperature, 0.0001)
	require.Equal(t, 2, cfg.Planner.MaxAttempts)
}

func TestPlannerEnvOverridesFileConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PLANNER_PROVIDER", "qwen")
	t.Setenv("PLANNER_API_KEY", "env-key")
	t.Setenv("PLANNER_BASE_URL", "https://gateway.local")
	t.Setenv("PLANNER_MODEL", "qwen-plus")
	t.Setenv("PLANNER_MAX_TOKENS", "4096")
	t.Setenv("PLANNER_TEMPERATURE", "0")
	t.Setenv("PLANNER_MAX_ATTEMPTS", "3")

	path := writeTempConfig(t, `
planner:
  provider: openai-compatible
  apiKey: file-key
  baseURL: https://planner.example
  model: planner-model
  maxTokens: 8192
  temperature: 0.3
  maxAttempts: 1
`)

	cfg, err := LoadFromFile(path)
	require.NoError(t, err)
	require.Equal(t, "qwen", cfg.Planner.Provider)
	require.Equal(t, "env-key", cfg.Planner.APIKey)
	require.Equal(t, "https://gateway.local", cfg.Planner.BaseURL)
	require.Equal(t, "qwen-plus", cfg.Planner.Model)
	require.Equal(t, 4096, cfg.Planner.MaxTokens)
	require.Equal(t, 0.0, cfg.Planner.Temperature)
	require.Equal(t, 3, cfg.Planner.MaxAttempts)
}

func TestLoadFromEnvRequiresEmployeeEnvEncryptionConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://superteam:secret@127.0.0.1:5432/superteam?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://:secret@127.0.0.1:6379/0")
	t.Setenv("S3_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("S3_BUCKET", "superteam-artifacts")
	t.Setenv("S3_ACCESS_KEY_ID", "minio")
	t.Setenv("S3_SECRET_ACCESS_KEY", "minio-secret")
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_KEYS", "")
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID", "")

	_, err := LoadFromEnv()
	require.ErrorContains(t, err, "employeeEnv.keys is required")
}

func TestLoadFromEnvRequiresEmployeeEnvActiveKeyID(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://superteam:secret@127.0.0.1:5432/superteam?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://:secret@127.0.0.1:6379/0")
	t.Setenv("S3_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("S3_BUCKET", "superteam-artifacts")
	t.Setenv("S3_ACCESS_KEY_ID", "minio")
	t.Setenv("S3_SECRET_ACCESS_KEY", "minio-secret")
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_KEYS", "v1:test-key")
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID", "")

	_, err := LoadFromEnv()
	require.ErrorContains(t, err, "employeeEnv.activeKeyId is required")
}

func setRequiredEnv(t *testing.T) {
	t.Helper()

	t.Setenv("DATABASE_URL", "postgres://superteam:secret@127.0.0.1:5432/superteam?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://:secret@127.0.0.1:6379/0")
	t.Setenv("S3_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("S3_BUCKET", "superteam-artifacts")
	t.Setenv("S3_ACCESS_KEY_ID", "minio")
	t.Setenv("S3_SECRET_ACCESS_KEY", "minio-secret")
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_KEYS", "v1:test-key")
	t.Setenv("SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID", "v1")
}

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()

	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()

	return writeConfigFile(t, body)
}
