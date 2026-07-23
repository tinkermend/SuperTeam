package systemconfig

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/audit"
)

// Repository 是覆盖值存储的最小依赖面。GetOverride 无覆盖时返回 (nil, nil)。
// UpsertOverride 的 value 应为 int64(数值型)或 string(string 型),以 JSON 写入 JSONB。
type Repository interface {
	ListOverrides(ctx context.Context, tenantID uuid.UUID) ([]Override, error)
	GetOverride(ctx context.Context, tenantID uuid.UUID, key string) (*Override, error)
	UpsertOverride(ctx context.Context, tenantID uuid.UUID, key string, value any, updatedBy uuid.UUID) error
	DeleteOverride(ctx context.Context, tenantID uuid.UUID, key string) (bool, error)
}

// AuditRecorder 是审计的窄依赖面,沿用各业务模块惯例。
type AuditRecorder interface {
	RecordEvent(ctx context.Context, event *audit.Event) error
}

// Reader 是业务模块读取配置的窄接口。实现带进程内缓存并在存储故障时
// 回退注册表默认值——读路径永不因配置中心故障而失败。
type Reader interface {
	Int64(ctx context.Context, tenantID uuid.UUID, key string) int64
	Duration(ctx context.Context, tenantID uuid.UUID, key string) time.Duration
	String(ctx context.Context, tenantID uuid.UUID, key string) string
}

const readCacheTTL = 15 * time.Second

type cachedValue struct {
	numeric   int64
	str       string
	isString  bool
	expiresAt time.Time
}

type Service struct {
	repo  Repository
	audit AuditRecorder

	mu    sync.RWMutex
	cache map[string]cachedValue
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:  repo,
		cache: make(map[string]cachedValue),
	}
}

func (s *Service) SetAuditRecorder(recorder AuditRecorder) {
	s.audit = recorder
}

// List 返回注册表全量投影:定义 + 生效值 + 覆盖态。
func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]EffectiveConfig, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidValue)
	}
	overrides, err := s.repo.ListOverrides(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list system config overrides: %w", err)
	}
	byKey := make(map[string]Override, len(overrides))
	for _, o := range overrides {
		byKey[o.ConfigKey] = o
	}
	items := make([]EffectiveConfig, 0, len(registry))
	for _, def := range Definitions() {
		item := EffectiveConfig{Definition: def}
		if def.IsStringType() {
			item.EffectiveStringValue = def.DefaultStringValue
		} else {
			item.EffectiveValue = def.DefaultValue
		}
		if o, ok := byKey[def.Key]; ok {
			if def.IsStringType() {
				item.EffectiveStringValue = sanitizeStringOverride(def, o.StringValue)
			} else {
				item.EffectiveValue = clampToBounds(def, o.Value)
			}
			item.IsOverridden = true
			updatedAt := o.UpdatedAt
			item.UpdatedAt = &updatedAt
			item.UpdatedByName = o.UpdatedByName
		}
		items = append(items, item)
	}
	return items, nil
}

// Set 校验并写入数值型覆盖值。未知 key 返回 ErrUnknownKey,越界或类型不符返回 ErrInvalidValue。
func (s *Service) Set(ctx context.Context, tenantID uuid.UUID, key string, value int64, actor uuid.UUID) (EffectiveConfig, error) {
	def, ok := LookupDefinition(key)
	if !ok {
		return EffectiveConfig{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if def.IsStringType() {
		return EffectiveConfig{}, fmt.Errorf("%w: %s is a string config; use string_value", ErrInvalidValue, key)
	}
	if tenantID == uuid.Nil || actor == uuid.Nil {
		return EffectiveConfig{}, fmt.Errorf("%w: tenant and actor are required", ErrInvalidValue)
	}
	if value < def.MinValue || value > def.MaxValue {
		return EffectiveConfig{}, fmt.Errorf("%w: %s must be within [%d, %d]", ErrInvalidValue, key, def.MinValue, def.MaxValue)
	}
	previous := s.effectiveValue(ctx, tenantID, def)
	if err := s.repo.UpsertOverride(ctx, tenantID, key, value, actor); err != nil {
		return EffectiveConfig{}, fmt.Errorf("upsert system config override: %w", err)
	}
	s.invalidate(tenantID, key)
	s.recordAudit(ctx, tenantID, actor, key, "update", previous, value, def.DefaultValue)
	now := time.Now().UTC()
	return EffectiveConfig{Definition: def, EffectiveValue: value, IsOverridden: true, UpdatedAt: &now}, nil
}

// SetString 校验并写入 string 型覆盖值。
func (s *Service) SetString(ctx context.Context, tenantID uuid.UUID, key string, value string, actor uuid.UUID) (EffectiveConfig, error) {
	def, ok := LookupDefinition(key)
	if !ok {
		return EffectiveConfig{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if !def.IsStringType() {
		return EffectiveConfig{}, fmt.Errorf("%w: %s is a numeric config; use value", ErrInvalidValue, key)
	}
	if tenantID == uuid.Nil || actor == uuid.Nil {
		return EffectiveConfig{}, fmt.Errorf("%w: tenant and actor are required", ErrInvalidValue)
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return EffectiveConfig{}, fmt.Errorf("%w: %s must be a non-empty string", ErrInvalidValue, key)
	}
	if !utf8.ValidString(trimmed) {
		return EffectiveConfig{}, fmt.Errorf("%w: %s must be valid UTF-8", ErrInvalidValue, key)
	}
	maxLen := def.EffectiveMaxStringLength()
	if utf8.RuneCountInString(trimmed) > maxLen {
		return EffectiveConfig{}, fmt.Errorf("%w: %s must be at most %d characters", ErrInvalidValue, key, maxLen)
	}
	previous := s.effectiveStringValue(ctx, tenantID, def)
	if err := s.repo.UpsertOverride(ctx, tenantID, key, trimmed, actor); err != nil {
		return EffectiveConfig{}, fmt.Errorf("upsert system config override: %w", err)
	}
	s.invalidate(tenantID, key)
	s.recordStringAudit(ctx, tenantID, actor, key, "update", previous, trimmed, def.DefaultStringValue)
	now := time.Now().UTC()
	return EffectiveConfig{
		Definition:           def,
		EffectiveStringValue: trimmed,
		IsOverridden:         true,
		UpdatedAt:            &now,
	}, nil
}

// Reset 删除覆盖行恢复默认。幂等:本就无覆盖时成功返回且不写审计。
func (s *Service) Reset(ctx context.Context, tenantID uuid.UUID, key string, actor uuid.UUID) (EffectiveConfig, error) {
	def, ok := LookupDefinition(key)
	if !ok {
		return EffectiveConfig{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if tenantID == uuid.Nil || actor == uuid.Nil {
		return EffectiveConfig{}, fmt.Errorf("%w: tenant and actor are required", ErrInvalidValue)
	}
	var previousNumeric int64
	var previousString string
	if def.IsStringType() {
		previousString = s.effectiveStringValue(ctx, tenantID, def)
	} else {
		previousNumeric = s.effectiveValue(ctx, tenantID, def)
	}
	deleted, err := s.repo.DeleteOverride(ctx, tenantID, key)
	if err != nil {
		return EffectiveConfig{}, fmt.Errorf("delete system config override: %w", err)
	}
	if deleted {
		s.invalidate(tenantID, key)
		if def.IsStringType() {
			s.recordStringAudit(ctx, tenantID, actor, key, "reset", previousString, def.DefaultStringValue, def.DefaultStringValue)
		} else {
			s.recordAudit(ctx, tenantID, actor, key, "reset", previousNumeric, def.DefaultValue, def.DefaultValue)
		}
	}
	item := EffectiveConfig{Definition: def}
	if def.IsStringType() {
		item.EffectiveStringValue = def.DefaultStringValue
	} else {
		item.EffectiveValue = def.DefaultValue
	}
	return item, nil
}

// Int64 读取生效值:缓存(15s) → 覆盖行 → 注册表默认值。存储故障回退默认值。
func (s *Service) Int64(ctx context.Context, tenantID uuid.UUID, key string) int64 {
	def, ok := LookupDefinition(key)
	if !ok {
		// 使用点只应引用注册表 key 常量;此分支属编程错误,记日志并返回 0。
		slog.Default().Error("systemconfig: read of unregistered key", "key", key)
		return 0
	}
	if def.IsStringType() {
		slog.Default().Error("systemconfig: Int64 called on string key", "key", key)
		return 0
	}
	if tenantID == uuid.Nil {
		return def.DefaultValue
	}
	cacheKey := tenantID.String() + "/" + key
	s.mu.RLock()
	cached, hit := s.cache[cacheKey]
	s.mu.RUnlock()
	if hit && time.Now().Before(cached.expiresAt) && !cached.isString {
		return cached.numeric
	}
	value := s.effectiveValue(ctx, tenantID, def)
	s.mu.Lock()
	s.cache[cacheKey] = cachedValue{numeric: value, expiresAt: time.Now().Add(readCacheTTL)}
	s.mu.Unlock()
	return value
}

// Duration 以秒为单位读取 duration_seconds 型配置。
func (s *Service) Duration(ctx context.Context, tenantID uuid.UUID, key string) time.Duration {
	return time.Duration(s.Int64(ctx, tenantID, key)) * time.Second
}

// String 读取 string 型生效值:缓存 → 覆盖行 → 注册表默认值。故障回退默认。
func (s *Service) String(ctx context.Context, tenantID uuid.UUID, key string) string {
	def, ok := LookupDefinition(key)
	if !ok {
		slog.Default().Error("systemconfig: read of unregistered key", "key", key)
		return ""
	}
	if !def.IsStringType() {
		slog.Default().Error("systemconfig: String called on non-string key", "key", key)
		return ""
	}
	if tenantID == uuid.Nil {
		return def.DefaultStringValue
	}
	cacheKey := tenantID.String() + "/" + key
	s.mu.RLock()
	cached, hit := s.cache[cacheKey]
	s.mu.RUnlock()
	if hit && time.Now().Before(cached.expiresAt) && cached.isString {
		return cached.str
	}
	value := s.effectiveStringValue(ctx, tenantID, def)
	s.mu.Lock()
	s.cache[cacheKey] = cachedValue{str: value, isString: true, expiresAt: time.Now().Add(readCacheTTL)}
	s.mu.Unlock()
	return value
}

func (s *Service) effectiveValue(ctx context.Context, tenantID uuid.UUID, def Definition) int64 {
	override, err := s.repo.GetOverride(ctx, tenantID, def.Key)
	if err != nil {
		slog.Default().Warn("systemconfig: override read failed, falling back to default",
			"key", def.Key, "error", err)
		return def.DefaultValue
	}
	if override == nil {
		return def.DefaultValue
	}
	return clampToBounds(def, override.Value)
}

func (s *Service) effectiveStringValue(ctx context.Context, tenantID uuid.UUID, def Definition) string {
	override, err := s.repo.GetOverride(ctx, tenantID, def.Key)
	if err != nil {
		slog.Default().Warn("systemconfig: override read failed, falling back to default",
			"key", def.Key, "error", err)
		return def.DefaultStringValue
	}
	if override == nil {
		return def.DefaultStringValue
	}
	return sanitizeStringOverride(def, override.StringValue)
}

// clampToBounds 兜底历史覆盖值越出(注册表边界收紧后)的情形,生效值始终在界内。
func clampToBounds(def Definition, value int64) int64 {
	if value < def.MinValue {
		return def.MinValue
	}
	if value > def.MaxValue {
		return def.MaxValue
	}
	return value
}

// sanitizeStringOverride 兜底历史覆盖非法(空/超长)的情形,回退默认值。
func sanitizeStringOverride(def Definition, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return def.DefaultStringValue
	}
	if utf8.RuneCountInString(trimmed) > def.EffectiveMaxStringLength() {
		return def.DefaultStringValue
	}
	return trimmed
}

func (s *Service) invalidate(tenantID uuid.UUID, key string) {
	s.mu.Lock()
	delete(s.cache, tenantID.String()+"/"+key)
	s.mu.Unlock()
}

func (s *Service) recordAudit(ctx context.Context, tenantID, actor uuid.UUID, key, action string, oldValue, newValue, defaultValue int64) {
	if s.audit == nil {
		return
	}
	event := &audit.Event{
		TenantID:     tenantID,
		EventType:    "system_config",
		ActorType:    "user",
		ActorID:      actor.String(),
		ResourceType: "system_config",
		ResourceID:   key,
		Action:       action,
		Details: map[string]any{
			"old_value":     oldValue,
			"new_value":     newValue,
			"default_value": defaultValue,
		},
		CreatedAt: time.Now().UTC(),
	}
	// 审计失败不阻断主流程(仓库惯例)。
	_ = s.audit.RecordEvent(ctx, event)
}

func (s *Service) recordStringAudit(ctx context.Context, tenantID, actor uuid.UUID, key, action, oldValue, newValue, defaultValue string) {
	if s.audit == nil {
		return
	}
	event := &audit.Event{
		TenantID:     tenantID,
		EventType:    "system_config",
		ActorType:    "user",
		ActorID:      actor.String(),
		ResourceType: "system_config",
		ResourceID:   key,
		Action:       action,
		Details: map[string]any{
			"old_value":     oldValue,
			"new_value":     newValue,
			"default_value": defaultValue,
		},
		CreatedAt: time.Now().UTC(),
	}
	_ = s.audit.RecordEvent(ctx, event)
}
