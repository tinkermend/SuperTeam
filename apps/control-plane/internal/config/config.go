package config

import (
	"errors"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTP        HTTPConfig        `yaml:"http"`
	Postgres    PostgresConfig    `yaml:"postgres"`
	Redis       RedisConfig       `yaml:"redis"`
	ObjectStore ObjectStoreConfig `yaml:"objectStore"`
}

type HTTPConfig struct {
	Addr string `yaml:"addr"`
}

type PostgresConfig struct {
	URL string `yaml:"url"`
}

type RedisConfig struct {
	URL string `yaml:"url"`
}

type ObjectStoreConfig struct {
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"accessKeyId"`
	SecretAccessKey string `yaml:"secretAccessKey"`
	ForcePathStyle  bool   `yaml:"forcePathStyle"`
}

func LoadFromEnv() (Config, error) {
	cfg := applyEnv(defaultConfig())
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func LoadFromFile(path string) (Config, error) {
	cfg := defaultConfig()
	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		if err := yaml.Unmarshal(body, &cfg); err != nil {
			return Config{}, err
		}
	}

	cfg = applyEnv(cfg)
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func defaultConfig() Config {
	return Config{
		HTTP: HTTPConfig{
			Addr: ":8080",
		},
		ObjectStore: ObjectStoreConfig{
			Region: "us-east-1",
		},
	}
}

func applyEnv(cfg Config) Config {
	cfg.HTTP.Addr = envOrDefault("CONTROL_PLANE_ADDR", cfg.HTTP.Addr)
	cfg.Postgres.URL = envOrDefault("DATABASE_URL", cfg.Postgres.URL)
	cfg.Redis.URL = envOrDefault("REDIS_URL", cfg.Redis.URL)
	cfg.ObjectStore.Endpoint = envOrDefault("S3_ENDPOINT", cfg.ObjectStore.Endpoint)
	cfg.ObjectStore.Region = envOrDefault("S3_REGION", cfg.ObjectStore.Region)
	cfg.ObjectStore.Bucket = envOrDefault("S3_BUCKET", cfg.ObjectStore.Bucket)
	cfg.ObjectStore.AccessKeyID = envOrDefault("S3_ACCESS_KEY_ID", cfg.ObjectStore.AccessKeyID)
	cfg.ObjectStore.SecretAccessKey = envOrDefault("S3_SECRET_ACCESS_KEY", cfg.ObjectStore.SecretAccessKey)
	if value, ok := os.LookupEnv("S3_FORCE_PATH_STYLE"); ok {
		cfg.ObjectStore.ForcePathStyle = parseBool(value)
	}
	return cfg
}

func (cfg Config) validate() error {
	if cfg.Postgres.URL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if cfg.Redis.URL == "" {
		return errors.New("REDIS_URL is required")
	}
	if cfg.ObjectStore.Bucket == "" {
		return errors.New("S3_BUCKET is required")
	}

	return nil
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
	return parseBool(value)
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}
