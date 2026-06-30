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
	Planner     PlannerConfig     `yaml:"planner"`
	Authz       AuthzConfig       `yaml:"authz"`
	Auth        AuthConfig        `yaml:"auth"`
	EmployeeEnv EmployeeEnvConfig `yaml:"employeeEnv"`
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

type EmployeeEnvConfig struct {
	Keys        string `yaml:"keys"`
	ActiveKeyID string `yaml:"activeKeyId"`
}

type TemporalConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Address   string `yaml:"address"`
	Namespace string `yaml:"namespace"`
	TaskQueue string `yaml:"taskQueue"`
}

type PlannerConfig struct {
	Provider    string  `yaml:"provider"`
	APIKey      string  `yaml:"apiKey"`
	BaseURL     string  `yaml:"baseURL"`
	Model       string  `yaml:"model"`
	MaxTokens   int     `yaml:"maxTokens"`
	Temperature float64 `yaml:"temperature"`
	MaxAttempts int     `yaml:"maxAttempts"`
}

type AuthConfig struct {
	CaptchaEnabled bool `yaml:"captchaEnabled"`
}

type AuthzConfig struct {
	Engine  string        `yaml:"engine"`
	OpenFGA OpenFGAConfig `yaml:"openfga"`
}

type OpenFGAConfig struct {
	APIURL   string `yaml:"apiUrl"`
	StoreID  string `yaml:"storeId"`
	ModelID  string `yaml:"modelId"`
	APIToken string `yaml:"apiToken"`
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
		Planner: PlannerConfig{
			Provider:    "openai-compatible",
			MaxTokens:   8192,
			Temperature: 0,
			MaxAttempts: 2,
		},
		Auth: AuthConfig{
			CaptchaEnabled: true,
		},
		Authz: AuthzConfig{
			Engine: "db",
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
	cfg.Planner.Provider = envOrDefault("PLANNER_PROVIDER", cfg.Planner.Provider)
	cfg.Planner.APIKey = envOrDefault("PLANNER_API_KEY", cfg.Planner.APIKey)
	cfg.Planner.BaseURL = envOrDefault("PLANNER_BASE_URL", cfg.Planner.BaseURL)
	cfg.Planner.Model = envOrDefault("PLANNER_MODEL", cfg.Planner.Model)
	if value := os.Getenv("PLANNER_MAX_TOKENS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.Planner.MaxTokens = parsed
		}
	}
	if value := os.Getenv("PLANNER_TEMPERATURE"); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Planner.Temperature = parsed
		}
	}
	if value := os.Getenv("PLANNER_MAX_ATTEMPTS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.Planner.MaxAttempts = parsed
		}
	}
	cfg.Authz.Engine = envOrDefault("AUTHZ_ENGINE", cfg.Authz.Engine)
	cfg.Authz.OpenFGA.APIURL = envOrDefault("OPENFGA_API_URL", cfg.Authz.OpenFGA.APIURL)
	cfg.Authz.OpenFGA.StoreID = envOrDefault("OPENFGA_STORE_ID", cfg.Authz.OpenFGA.StoreID)
	cfg.Authz.OpenFGA.ModelID = envOrDefault("OPENFGA_MODEL_ID", cfg.Authz.OpenFGA.ModelID)
	cfg.Authz.OpenFGA.APIToken = envOrDefault("OPENFGA_API_TOKEN", cfg.Authz.OpenFGA.APIToken)
	if value, ok := os.LookupEnv("AUTH_CAPTCHA_ENABLED"); ok {
		cfg.Auth.CaptchaEnabled = parseBool(value)
	}
	cfg.EmployeeEnv.Keys = envOrDefault("SUPERTEAM_ENV_ENCRYPTION_KEYS", cfg.EmployeeEnv.Keys)
	cfg.EmployeeEnv.ActiveKeyID = envOrDefault("SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID", cfg.EmployeeEnv.ActiveKeyID)
	return cfg
}

func (cfg Config) validate() error {
	if cfg.Authz.Engine == "" {
		cfg.Authz.Engine = "db"
	}
	switch cfg.Authz.Engine {
	case "db", "openfga_shadow", "openfga":
	default:
		return errors.New("AUTHZ_ENGINE must be one of db, openfga_shadow, openfga")
	}
	if cfg.Authz.Engine != "db" {
		if strings.TrimSpace(cfg.Authz.OpenFGA.APIURL) == "" {
			return errors.New("OPENFGA_API_URL is required when AUTHZ_ENGINE uses OpenFGA")
		}
		if strings.TrimSpace(cfg.Authz.OpenFGA.StoreID) == "" {
			return errors.New("OPENFGA_STORE_ID is required when AUTHZ_ENGINE uses OpenFGA")
		}
		if strings.TrimSpace(cfg.Authz.OpenFGA.ModelID) == "" {
			return errors.New("OPENFGA_MODEL_ID is required when AUTHZ_ENGINE uses OpenFGA")
		}
	}
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
	if strings.TrimSpace(cfg.EmployeeEnv.Keys) == "" {
		return errors.New("employeeEnv.keys is required (set employeeEnv in config.yaml or SUPERTEAM_ENV_ENCRYPTION_KEYS)")
	}
	if strings.TrimSpace(cfg.EmployeeEnv.ActiveKeyID) == "" {
		return errors.New("employeeEnv.activeKeyId is required (set employeeEnv in config.yaml or SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID)")
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
