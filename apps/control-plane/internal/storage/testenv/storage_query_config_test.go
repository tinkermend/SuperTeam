package testenv

import "testing"

func TestResolveStorageQueryConfigIgnoresApplicationDatabaseURLFallback(t *testing.T) {
	env := map[string]string{
		"ALLOW_DATABASE_URL_FOR_QUERY_TESTS": "1",
		"DATABASE_URL":                       "postgres://dev.example/superteam",
		"REDIS_URL":                          "redis://dev.example/0",
	}

	_, ok := ResolveStorageQueryConfig(func(key string) string {
		return env[key]
	})

	if ok {
		t.Fatal("expected storage query tests to require explicit TEST_DATABASE_URL and TEST_REDIS_URL")
	}
}

func TestResolveStorageQueryConfigAcceptsExplicitTestURLs(t *testing.T) {
	env := map[string]string{
		"TEST_DATABASE_URL": "postgres://test.example/superteam_test",
		"TEST_REDIS_URL":    "redis://test.example/0",
	}

	cfg, ok := ResolveStorageQueryConfig(func(key string) string {
		return env[key]
	})

	if !ok {
		t.Fatal("expected explicit test URLs to enable storage query integration tests")
	}
	if cfg.DatabaseURL != env["TEST_DATABASE_URL"] {
		t.Fatalf("unexpected database URL %q", cfg.DatabaseURL)
	}
	if cfg.RedisURL != env["TEST_REDIS_URL"] {
		t.Fatalf("unexpected Redis URL %q", cfg.RedisURL)
	}
}
