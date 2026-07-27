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

const feishuOAuthStateKeyPrefix = "superteam:feishu:oauth:state:"

// RedisOAuthStateStore：OAuth state 的跨实例 one-shot 权威(SET + TTL + GETDEL)。
type RedisOAuthStateStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisOAuthStateStore(rdb *redis.Client) *RedisOAuthStateStore {
	return &RedisOAuthStateStore{rdb: rdb, ttl: oauthStateTTL}
}

type redisOAuthStatePayload struct {
	TenantID    string `json:"tenant_id"`
	UserID      string `json:"user_id"`
	AppConfigID string `json:"app_config_id"`
	ReturnTo    string `json:"return_to,omitempty"`
}

func feishuOAuthStateKey(state string) string {
	return feishuOAuthStateKeyPrefix + state
}

func (s *RedisOAuthStateStore) Put(ctx context.Context, state string, value oauthState, ttl time.Duration) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("feishu oauth state store: redis not configured")
	}
	state = strings.TrimSpace(state)
	if state == "" || value.TenantID == uuid.Nil || value.UserID == uuid.Nil || value.AppConfigID == uuid.Nil {
		return ErrInvalidInput
	}
	if ttl <= 0 {
		ttl = s.ttl
	}
	if ttl <= 0 {
		ttl = oauthStateTTL
	}
	raw, err := json.Marshal(redisOAuthStatePayload{
		TenantID:    value.TenantID.String(),
		UserID:      value.UserID.String(),
		AppConfigID: value.AppConfigID.String(),
		ReturnTo:    value.ReturnTo,
	})
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, feishuOAuthStateKey(state), raw, ttl).Err()
}

func (s *RedisOAuthStateStore) Take(ctx context.Context, state string) (oauthState, bool, error) {
	if s == nil || s.rdb == nil {
		return oauthState{}, false, fmt.Errorf("feishu oauth state store: redis not configured")
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return oauthState{}, false, nil
	}
	raw, err := s.rdb.GetDel(ctx, feishuOAuthStateKey(state)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return oauthState{}, false, nil
		}
		return oauthState{}, false, err
	}
	var payload redisOAuthStatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return oauthState{}, false, err
	}
	tenantID, err := uuid.Parse(payload.TenantID)
	if err != nil {
		return oauthState{}, false, nil
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return oauthState{}, false, nil
	}
	appConfigID, err := uuid.Parse(payload.AppConfigID)
	if err != nil {
		return oauthState{}, false, nil
	}
	return oauthState{
		TenantID:    tenantID,
		UserID:      userID,
		AppConfigID: appConfigID,
		ReturnTo:    payload.ReturnTo,
	}, true, nil
}
