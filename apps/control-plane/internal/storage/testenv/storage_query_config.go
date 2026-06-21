package testenv

import "strings"

// StorageQueryConfig describes the explicit environment required for destructive
// storage query integration tests.
type StorageQueryConfig struct {
	DatabaseURL string
	RedisURL    string
}

// ResolveStorageQueryConfig only accepts dedicated test environment URLs.
func ResolveStorageQueryConfig(lookup func(string) string) (StorageQueryConfig, bool) {
	if lookup == nil {
		return StorageQueryConfig{}, false
	}

	databaseURL := strings.TrimSpace(lookup("TEST_DATABASE_URL"))
	redisURL := strings.TrimSpace(lookup("TEST_REDIS_URL"))
	if databaseURL == "" || redisURL == "" {
		return StorageQueryConfig{}, false
	}

	return StorageQueryConfig{
		DatabaseURL: databaseURL,
		RedisURL:    redisURL,
	}, true
}
