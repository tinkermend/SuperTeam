package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	HTTP        HTTPConfig
	Postgres    PostgresConfig
	Redis       RedisConfig
	ObjectStore ObjectStoreConfig
}

type HTTPConfig struct {
	Addr string
}

type PostgresConfig struct {
	URL string
}

type RedisConfig struct {
	URL string
}

type ObjectStoreConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		HTTP: HTTPConfig{
			Addr: envOrDefault("CONTROL_PLANE_ADDR", ":8080"),
		},
		Postgres: PostgresConfig{
			URL: os.Getenv("DATABASE_URL"),
		},
		Redis: RedisConfig{
			URL: os.Getenv("REDIS_URL"),
		},
		ObjectStore: ObjectStoreConfig{
			Endpoint:        os.Getenv("S3_ENDPOINT"),
			Region:          envOrDefault("S3_REGION", "us-east-1"),
			Bucket:          os.Getenv("S3_BUCKET"),
			AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
			ForcePathStyle:  envBool("S3_FORCE_PATH_STYLE"),
		},
	}

	if cfg.Postgres.URL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.Redis.URL == "" {
		return Config{}, errors.New("REDIS_URL is required")
	}
	if cfg.ObjectStore.Bucket == "" {
		return Config{}, errors.New("S3_BUCKET is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string) bool {
	value := os.Getenv(key)
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}
