package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ConnectorHeartbeatTimeout：Console/看门狗判定 stale 的年龄阈值。
// connector 默认 30s 上报；90s ≈ 允许漏 2 次仍算存活抖动。
const ConnectorHeartbeatTimeout = 90 * time.Second

// ConnectorHeartbeatDegradedAfter：仍有心跳但偏旧的降级提示（不写收件箱告警）。
const ConnectorHeartbeatDegradedAfter = 60 * time.Second

// ConnectorHeartbeatRedisTTL：探活 payload 在 Redis 的保留时间（权威在 last_heartbeat_at 年龄，
// TTL 仅防止永久脏键；略大于 timeout 以便 stale 时仍能读到末次快照）。
const ConnectorHeartbeatRedisTTL = 3 * time.Minute

const DefaultConnectorServiceName = "feishu-connector"

const feishuHeartbeatKeyPrefix = "superteam:feishu:connector:hb:"

type ConnectorAppStatus struct {
	AppID         string     `json:"app_id"`
	ConfigID      string     `json:"config_id,omitempty"`
	WSStatus      string     `json:"ws_status"` // connected | reconnecting | stopped | unknown
	LastWSEventAt *time.Time `json:"last_ws_event_at,omitempty"`
}

type ConnectorHeartbeat struct {
	TenantID         uuid.UUID
	ServiceName      string
	Version          string
	LastHeartbeatAt  time.Time
	LastOutboxPollAt *time.Time
	Apps             []ConnectorAppStatus
}

type UpsertConnectorHeartbeatInput struct {
	TenantID         uuid.UUID
	ServiceName      string
	Version          string
	LastOutboxPollAt *time.Time
	Apps             []ConnectorAppStatus
}

// heartbeatStore 是探活权威存储（生产 = Redis TTL key）。
type heartbeatStore interface {
	Put(ctx context.Context, input UpsertConnectorHeartbeatInput) (ConnectorHeartbeat, error)
	Get(ctx context.Context, tenantID uuid.UUID, serviceName string) (ConnectorHeartbeat, error)
}

// recipientDirectory 告警收件人（仍走 PG 成员表）。
type recipientDirectory interface {
	ListActiveTenantOwnersAndAdmins(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error)
}

type redisHeartbeatPayload struct {
	TenantID         string               `json:"tenant_id"`
	ServiceName      string               `json:"service_name"`
	Version          string               `json:"version"`
	LastHeartbeatAt  time.Time            `json:"last_heartbeat_at"`
	LastOutboxPollAt *time.Time           `json:"last_outbox_poll_at,omitempty"`
	Apps             []ConnectorAppStatus `json:"apps"`
}

// RedisHeartbeatStore：connector 探活唯一权威（SET + TTL）。
type RedisHeartbeatStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisHeartbeatStore(rdb *redis.Client) *RedisHeartbeatStore {
	return &RedisHeartbeatStore{rdb: rdb, ttl: ConnectorHeartbeatRedisTTL}
}

func feishuHeartbeatKey(tenantID uuid.UUID, serviceName string) string {
	if serviceName == "" {
		serviceName = DefaultConnectorServiceName
	}
	return feishuHeartbeatKeyPrefix + tenantID.String() + ":" + serviceName
}

func (s *RedisHeartbeatStore) Put(ctx context.Context, input UpsertConnectorHeartbeatInput) (ConnectorHeartbeat, error) {
	if s == nil || s.rdb == nil {
		return ConnectorHeartbeat{}, fmt.Errorf("feishu heartbeat store: redis not configured")
	}
	if input.TenantID == uuid.Nil {
		return ConnectorHeartbeat{}, ErrInvalidInput
	}
	serviceName := strings.TrimSpace(input.ServiceName)
	if serviceName == "" {
		serviceName = DefaultConnectorServiceName
	}
	now := time.Now().UTC()
	apps := input.Apps
	if apps == nil {
		apps = []ConnectorAppStatus{}
	}
	payload := redisHeartbeatPayload{
		TenantID:         input.TenantID.String(),
		ServiceName:      serviceName,
		Version:          input.Version,
		LastHeartbeatAt:  now,
		LastOutboxPollAt: input.LastOutboxPollAt,
		Apps:             apps,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ConnectorHeartbeat{}, err
	}
	ttl := s.ttl
	if ttl <= 0 {
		ttl = ConnectorHeartbeatRedisTTL
	}
	if err := s.rdb.Set(ctx, feishuHeartbeatKey(input.TenantID, serviceName), raw, ttl).Err(); err != nil {
		return ConnectorHeartbeat{}, err
	}
	return ConnectorHeartbeat{
		TenantID:         input.TenantID,
		ServiceName:      serviceName,
		Version:          input.Version,
		LastHeartbeatAt:  now,
		LastOutboxPollAt: input.LastOutboxPollAt,
		Apps:             apps,
	}, nil
}

func (s *RedisHeartbeatStore) Get(ctx context.Context, tenantID uuid.UUID, serviceName string) (ConnectorHeartbeat, error) {
	if s == nil || s.rdb == nil {
		return ConnectorHeartbeat{}, fmt.Errorf("feishu heartbeat store: redis not configured")
	}
	if tenantID == uuid.Nil {
		return ConnectorHeartbeat{}, ErrInvalidInput
	}
	if serviceName == "" {
		serviceName = DefaultConnectorServiceName
	}
	raw, err := s.rdb.Get(ctx, feishuHeartbeatKey(tenantID, serviceName)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ConnectorHeartbeat{}, ErrAppConfigNotFound
		}
		return ConnectorHeartbeat{}, err
	}
	var payload redisHeartbeatPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ConnectorHeartbeat{}, err
	}
	apps := payload.Apps
	if apps == nil {
		apps = []ConnectorAppStatus{}
	}
	return ConnectorHeartbeat{
		TenantID:         tenantID,
		ServiceName:      payload.ServiceName,
		Version:          payload.Version,
		LastHeartbeatAt:  payload.LastHeartbeatAt.UTC(),
		LastOutboxPollAt: payload.LastOutboxPollAt,
		Apps:             apps,
	}, nil
}

// ChannelHealth 是 Console 健康摘要。
type ChannelHealth struct {
	ServiceName      string               `json:"service_name"`
	Version          string               `json:"version,omitempty"`
	Status           string               `json:"status"` // healthy | degraded | stale | missing
	LastHeartbeatAt  *time.Time           `json:"last_heartbeat_at,omitempty"`
	LastOutboxPollAt *time.Time           `json:"last_outbox_poll_at,omitempty"`
	AgeSeconds       *int64               `json:"age_seconds,omitempty"`
	Apps             []ConnectorAppStatus `json:"apps"`
	TimeoutSeconds   int64                `json:"timeout_seconds"`
}

func (s *Service) SetHeartbeatStore(store heartbeatStore) {
	s.heartbeatStore = store
}

func (s *Service) RecordConnectorHeartbeat(ctx context.Context, input UpsertConnectorHeartbeatInput) (ConnectorHeartbeat, error) {
	if input.TenantID == uuid.Nil {
		return ConnectorHeartbeat{}, ErrInvalidInput
	}
	store := s.heartbeatStore
	if store == nil {
		return ConnectorHeartbeat{}, fmt.Errorf("feishu connector heartbeat store not configured")
	}
	return store.Put(ctx, input)
}

func (s *Service) ListChannelAlertRecipients(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if dir, ok := s.repo.(recipientDirectory); ok {
		return dir.ListActiveTenantOwnersAndAdmins(ctx, tenantID)
	}
	return nil, nil
}

func (s *Service) GetChannelHealth(ctx context.Context, tenantID uuid.UUID) (ChannelHealth, error) {
	if tenantID == uuid.Nil {
		return ChannelHealth{}, ErrInvalidInput
	}
	timeout := ConnectorHeartbeatTimeout
	store := s.heartbeatStore
	if store == nil {
		return ChannelHealth{
			ServiceName:    DefaultConnectorServiceName,
			Status:         "missing",
			Apps:           []ConnectorAppStatus{},
			TimeoutSeconds: int64(timeout.Seconds()),
		}, nil
	}
	hb, err := store.Get(ctx, tenantID, DefaultConnectorServiceName)
	if err != nil {
		if errors.Is(err, ErrAppConfigNotFound) {
			return ChannelHealth{
				ServiceName:    DefaultConnectorServiceName,
				Status:         "missing",
				Apps:           []ConnectorAppStatus{},
				TimeoutSeconds: int64(timeout.Seconds()),
			}, nil
		}
		return ChannelHealth{}, err
	}
	age := time.Since(hb.LastHeartbeatAt)
	if age < 0 {
		age = 0
	}
	status := "healthy"
	switch {
	case age > timeout:
		status = "stale"
	case age > ConnectorHeartbeatDegradedAfter:
		status = "degraded"
	}
	ageSec := int64(age.Seconds())
	return ChannelHealth{
		ServiceName:      hb.ServiceName,
		Version:          hb.Version,
		Status:           status,
		LastHeartbeatAt:  &hb.LastHeartbeatAt,
		LastOutboxPollAt: hb.LastOutboxPollAt,
		AgeSeconds:       &ageSec,
		Apps:             hb.Apps,
		TimeoutSeconds:   int64(timeout.Seconds()),
	}, nil
}

func (s *Service) ListOperationalOutbox(ctx context.Context, tenantID uuid.UUID, statuses []string, limit, offset int32) ([]OutboxItem, int64, error) {
	if tenantID == uuid.Nil {
		return nil, 0, ErrInvalidInput
	}
	if len(statuses) == 0 {
		statuses = []string{"failed", "skipped_unbound"}
	}
	repo := s.repoOutbox()
	items, err := repo.ListOutboxByStatuses(ctx, tenantID, statuses, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := repo.CountOutboxByStatuses(ctx, tenantID, statuses)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) RequeueFailedOutbox(ctx context.Context, tenantID, id uuid.UUID) (OutboxItem, error) {
	if tenantID == uuid.Nil || id == uuid.Nil {
		return OutboxItem{}, ErrInvalidInput
	}
	return s.repoOutbox().RequeueOutbox(ctx, tenantID, id)
}

type outboxOpsRepo interface {
	ListOutboxByStatuses(ctx context.Context, tenantID uuid.UUID, statuses []string, limit, offset int32) ([]OutboxItem, error)
	CountOutboxByStatuses(ctx context.Context, tenantID uuid.UUID, statuses []string) (int64, error)
	RequeueOutbox(ctx context.Context, tenantID, id uuid.UUID) (OutboxItem, error)
}

func (s *Service) repoOutbox() outboxOpsRepo {
	if r, ok := s.repo.(outboxOpsRepo); ok {
		return r
	}
	if r, ok := any(s.repo).(outboxOpsRepo); ok {
		return r
	}
	return noopOutboxRepo{}
}

type noopOutboxRepo struct{}

func (noopOutboxRepo) ListOutboxByStatuses(context.Context, uuid.UUID, []string, int32, int32) ([]OutboxItem, error) {
	return nil, ErrInvalidInput
}
func (noopOutboxRepo) CountOutboxByStatuses(context.Context, uuid.UUID, []string) (int64, error) {
	return 0, ErrInvalidInput
}
func (noopOutboxRepo) RequeueOutbox(context.Context, uuid.UUID, uuid.UUID) (OutboxItem, error) {
	return OutboxItem{}, ErrOutboxNotFound
}
