package config

import "testing"

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
