package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SecurityConfig 平台安全相关配置。
type SecurityConfig struct {
	// CredentialEncryptionKey 是外部凭据(飞书 App Secret 等)的 AES-GCM 主密钥,
	// base64 编码 32 字节。环境变量 CONTROL_PLANE_CREDENTIAL_KEY 优先于此值。
	// 更换密钥会使库内已加密凭据不可解,需重新录入。
	CredentialEncryptionKey string `yaml:"credentialEncryptionKey"`
}

// Environment 决定安全默认值的松紧。dev 会为本地 Web 注入 localhost CORS 来源,
// prod 一律不注入——开发便利不该编进生产二进制。
const (
	EnvironmentDev  = "dev"
	EnvironmentProd = "prod"
)

type Config struct {
	// Environment 是 dev 或 prod;缺省 dev,由 CONTROL_PLANE_ENV 覆盖。
	Environment string            `yaml:"environment"`
	HTTP        HTTPConfig        `yaml:"http"`
	Postgres    PostgresConfig    `yaml:"postgres"`
	Redis       RedisConfig       `yaml:"redis"`
	ObjectStore ObjectStoreConfig `yaml:"objectStore"`
	Temporal    TemporalConfig    `yaml:"temporal"`
	Planner     PlannerConfig     `yaml:"planner"`
	Security    SecurityConfig    `yaml:"security"`
	Authz       AuthzConfig       `yaml:"authz"`
	Auth        AuthConfig        `yaml:"auth"`
	EmployeeEnv EmployeeEnvConfig `yaml:"employeeEnv"`
}

type HTTPConfig struct {
	Addr string `yaml:"addr"`
	// AllowedOrigins 是允许携带凭据跨源调用本服务的浏览器来源(scheme://host:port
	// 全匹配)。Console 与 Control Plane 默认不同端口即不同源,所以这是一条长期
	// 承重的安全边界,不是本地开发的脚手架。
	//
	// 注意:它与"前端有几个实例"无关——十个副本挂在同一域名后面仍然只有一个
	// 来源。需要多来源的是多环境、多租户自定义域名、预览部署子域这类场景。
	//
	// 显式配置后**不再**注入任何 localhost 默认值:dev 默认只在本项为空时兜底,
	// 否则生产二进制会永久放行 localhost,任何能诱导用户本地起服务的页面都能
	// 带着会话 cookie 打生产 API。
	AllowedOrigins []string `yaml:"allowedOrigins"`
}

// devDefaultAllowedOrigins 是 dev 环境下未显式配置时的兜底来源。端口须与
// apps/web/vite.config.ts 的 DEV_SERVER_PORT 一致;保留 localhost 是因为 Web
// 入口会把 localhost 跳到 127.0.0.1,跳转落地前的首个请求仍需放行。
func devDefaultAllowedOrigins() []string {
	return []string{"http://127.0.0.1:3100", "http://localhost:3100"}
}

// ResolvedAllowedOrigins 返回最终生效的 CORS 来源:显式配置优先,dev 环境为空
// 时回落本地默认,prod 为空时返回空(由 validate 提前拦成启动失败)。
func (cfg Config) ResolvedAllowedOrigins() []string {
	origins := make([]string, 0, len(cfg.HTTP.AllowedOrigins))
	for _, origin := range cfg.HTTP.AllowedOrigins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	if len(origins) > 0 {
		return origins
	}
	if cfg.IsDev() {
		return devDefaultAllowedOrigins()
	}
	return nil
}

func (cfg Config) IsDev() bool {
	return !strings.EqualFold(strings.TrimSpace(cfg.Environment), EnvironmentProd)
}

// PostgresConfig carries the connection string plus explicit pool sizing.
//
// The pool is sized by round-trip latency, not by CPU. pgx's default MaxConns is
// max(4, NumCPU), which is a CPU-shaped heuristic and wrong here: the Control
// Plane talks to a remote Postgres (~39ms RTT measured against the shared dev
// database), so a single connection sustains only ~1/RTT round trips per second
// and pool throughput is roughly MaxConns/RTT. Four connections cap the whole
// process at ~100 round trips per second no matter how large the machine is,
// and long-lived readers (the SSE change-probe loops) hold connections for a
// full RTT on every tick. Size these from latency and expected concurrency.
type PostgresConfig struct {
	URL string `yaml:"url"`
	// MaxConns caps concurrent connections. Keep well under the server's
	// max_connections so several Control Plane replicas can coexist.
	MaxConns int32 `yaml:"maxConns"`
	// MinConns keeps warm connections ready. Establishing one costs several
	// round trips (TCP, TLS, auth), which is why cold pools show up as first
	// request latency spikes on a remote database.
	MinConns          int32         `yaml:"minConns"`
	MaxConnLifetime   time.Duration `yaml:"maxConnLifetime"`
	MaxConnIdleTime   time.Duration `yaml:"maxConnIdleTime"`
	HealthCheckPeriod time.Duration `yaml:"healthCheckPeriod"`
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
		Environment: EnvironmentDev,
		HTTP: HTTPConfig{
			Addr: ":8080",
		},
		Postgres: PostgresConfig{
			// Sized for a remote database (~39ms RTT): 25 connections give the
			// process roughly 600 round trips per second of headroom, while
			// staying far enough under a default max_connections of 100 that
			// several replicas plus migrations and psql sessions still fit.
			MaxConns:          25,
			MinConns:          5,
			MaxConnLifetime:   30 * time.Minute,
			MaxConnIdleTime:   5 * time.Minute,
			HealthCheckPeriod: 30 * time.Second,
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
			// 默认关闭；仅当配置或 AUTH_CAPTCHA_ENABLED=true 时开启。
			CaptchaEnabled: false,
		},
		Authz: AuthzConfig{
			Engine: "db",
		},
	}
}

func applyEnv(cfg Config) Config {
	cfg.Environment = envOrDefault("CONTROL_PLANE_ENV", cfg.Environment)
	cfg.HTTP.Addr = envOrDefault("CONTROL_PLANE_ADDR", cfg.HTTP.Addr)
	if value, ok := os.LookupEnv("CONTROL_PLANE_CORS_ALLOWED_ORIGINS"); ok {
		cfg.HTTP.AllowedOrigins = splitAndTrim(value)
	}
	cfg.Postgres.URL = envOrDefault("DATABASE_URL", cfg.Postgres.URL)
	cfg.Postgres.MaxConns = envInt32OrDefault("DATABASE_MAX_CONNS", cfg.Postgres.MaxConns)
	cfg.Postgres.MinConns = envInt32OrDefault("DATABASE_MIN_CONNS", cfg.Postgres.MinConns)
	cfg.Postgres.MaxConnLifetime = envDurationOrDefault("DATABASE_MAX_CONN_LIFETIME", cfg.Postgres.MaxConnLifetime)
	cfg.Postgres.MaxConnIdleTime = envDurationOrDefault("DATABASE_MAX_CONN_IDLE_TIME", cfg.Postgres.MaxConnIdleTime)
	cfg.Postgres.HealthCheckPeriod = envDurationOrDefault("DATABASE_HEALTH_CHECK_PERIOD", cfg.Postgres.HealthCheckPeriod)
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
	switch strings.ToLower(strings.TrimSpace(cfg.Environment)) {
	case "", EnvironmentDev, EnvironmentProd:
	default:
		return errors.New("CONTROL_PLANE_ENV must be one of dev, prod")
	}
	// 生产必须显式声明允许的浏览器来源。留空时启动即失败,而不是让用户在浏览器
	// 里撞一个 CORS 报错再反推到这里。
	if !cfg.IsDev() && len(cfg.ResolvedAllowedOrigins()) == 0 {
		return errors.New("http.allowedOrigins (or CONTROL_PLANE_CORS_ALLOWED_ORIGINS) is required when CONTROL_PLANE_ENV=prod")
	}
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

func envInt32OrDefault(key string, fallback int32) int32 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return int32(parsed)
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

// splitAndTrim 按逗号切分并去空白,用于 env 里的列表型配置。
func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
