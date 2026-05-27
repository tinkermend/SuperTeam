package config

import (
	"os"
	"testing"
)

func TestLoadFromEnvBuildsControlPlaneConfig(t *testing.T) {
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

func TestLoadFromEnvRequiresStorageConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("S3_BUCKET", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected missing storage configuration to fail")
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
  bucket: "file-bucket"
`)
	t.Setenv("CONTROL_PLANE_ADDR", ":7070")
	t.Setenv("DATABASE_URL", "postgres://env")
	t.Setenv("REDIS_URL", "redis://env")
	t.Setenv("S3_BUCKET", "env-bucket")
	t.Setenv("S3_FORCE_PATH_STYLE", "true")

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
}

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()

	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}
