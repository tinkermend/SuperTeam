package config

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTP        HTTPConfig        `yaml:"http"`
	Postgres    PostgresConfig    `yaml:"postgres"`
	Redis       RedisConfig       `yaml:"redis"`
	ObjectStore ObjectStoreConfig `yaml:"objectStore"`
	Temporal    TemporalConfig    `yaml:"temporal"`
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

type TemporalConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Address   string `yaml:"address"`
	Namespace string `yaml:"namespace"`
	TaskQueue string `yaml:"taskQueue"`
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
		Temporal: TemporalConfig{
			Enabled:   false,
			Address:   "127.0.0.1:7233",
			Namespace: "default",
			TaskQueue: "superteam-project-coordination",
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
	if value, ok := os.LookupEnv("TEMPORAL_ENABLED"); ok {
		cfg.Temporal.Enabled = parseBool(value)
	}
	cfg.Temporal.Address = envOrDefault("TEMPORAL_ADDRESS", cfg.Temporal.Address)
	cfg.Temporal.Namespace = envOrDefault("TEMPORAL_NAMESPACE", cfg.Temporal.Namespace)
	cfg.Temporal.TaskQueue = envOrDefault("TEMPORAL_TASK_QUEUE", cfg.Temporal.TaskQueue)
	return cfg
}

func (cfg Config) validate() error {
	if strings.TrimSpace(cfg.Postgres.URL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	if strings.TrimSpace(cfg.Redis.URL) == "" {
		return errors.New("REDIS_URL is required")
	}
	if strings.TrimSpace(cfg.ObjectStore.Endpoint) == "" {
		return errors.New("S3_ENDPOINT is required")
	}
	if strings.TrimSpace(cfg.ObjectStore.Region) == "" {
		return errors.New("S3_REGION is required")
	}
	if strings.TrimSpace(cfg.ObjectStore.Bucket) == "" {
		return errors.New("S3_BUCKET is required")
	}
	if strings.TrimSpace(cfg.ObjectStore.AccessKeyID) == "" {
		return errors.New("S3_ACCESS_KEY_ID is required")
	}
	if strings.TrimSpace(cfg.ObjectStore.SecretAccessKey) == "" {
		return errors.New("S3_SECRET_ACCESS_KEY is required")
	}
	if cfg.Temporal.Enabled {
		if strings.TrimSpace(cfg.Temporal.Address) == "" {
			return errors.New("TEMPORAL_ADDRESS is required when Temporal is enabled")
		}
		if strings.TrimSpace(cfg.Temporal.Namespace) == "" {
			return errors.New("TEMPORAL_NAMESPACE is required when Temporal is enabled")
		}
		if strings.TrimSpace(cfg.Temporal.TaskQueue) == "" {
			return errors.New("TEMPORAL_TASK_QUEUE is required when Temporal is enabled")
		}
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
