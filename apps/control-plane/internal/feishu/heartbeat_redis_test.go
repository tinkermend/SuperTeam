package feishu

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestRedisHeartbeatStoreRoundTrip(t *testing.T) {
	rdb := testRedis(t)
	store := NewRedisHeartbeatStore(rdb)
	store.ttl = 30 * time.Second

	tenantID := uuid.New()
	ctx := context.Background()
	poll := time.Now().UTC().Add(-2 * time.Second)
	put, err := store.Put(ctx, UpsertConnectorHeartbeatInput{
		TenantID:         tenantID,
		ServiceName:      DefaultConnectorServiceName,
		Version:          "test-v",
		LastOutboxPollAt: &poll,
		Apps: []ConnectorAppStatus{{
			AppID:    "cli_test",
			ConfigID: uuid.New().String(),
			WSStatus: "connected",
		}},
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if put.Version != "test-v" || put.LastHeartbeatAt.IsZero() {
		t.Fatalf("unexpected put result: %+v", put)
	}

	got, err := store.Get(ctx, tenantID, DefaultConnectorServiceName)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Version != "test-v" || len(got.Apps) != 1 || got.Apps[0].WSStatus != "connected" {
		t.Fatalf("unexpected get: %+v", got)
	}
	if got.LastOutboxPollAt == nil {
		t.Fatal("expected last_outbox_poll_at")
	}

	_ = rdb.Del(ctx, feishuHeartbeatKey(tenantID, DefaultConnectorServiceName)).Err()
}

func TestGetChannelHealthStatuses(t *testing.T) {
	rdb := testRedis(t)
	store := NewRedisHeartbeatStore(rdb)
	svc := NewService(nil, nil)
	svc.SetHeartbeatStore(store)
	tenantID := uuid.New()
	ctx := context.Background()
	key := feishuHeartbeatKey(tenantID, DefaultConnectorServiceName)
	t.Cleanup(func() { _ = rdb.Del(context.Background(), key).Err() })

	h, err := svc.GetChannelHealth(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if h.Status != "missing" {
		t.Fatalf("want missing, got %s", h.Status)
	}

	if _, err := store.Put(ctx, UpsertConnectorHeartbeatInput{
		TenantID:    tenantID,
		ServiceName: DefaultConnectorServiceName,
		Version:     "v",
	}); err != nil {
		t.Fatal(err)
	}
	h, err = svc.GetChannelHealth(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if h.Status != "healthy" {
		t.Fatalf("want healthy, got %s age=%v", h.Status, h.AgeSeconds)
	}

	writeAge := func(age time.Duration) {
		t.Helper()
		payload := redisHeartbeatPayload{
			TenantID:        tenantID.String(),
			ServiceName:     DefaultConnectorServiceName,
			Version:         "v",
			LastHeartbeatAt: time.Now().UTC().Add(-age),
			Apps:            []ConnectorAppStatus{},
		}
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := rdb.Set(ctx, key, b, time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
	}

	writeAge(70 * time.Second)
	h, err = svc.GetChannelHealth(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if h.Status != "degraded" {
		t.Fatalf("want degraded, got %s age=%v", h.Status, h.AgeSeconds)
	}

	writeAge(2 * time.Minute)
	h, err = svc.GetChannelHealth(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if h.Status != "stale" {
		t.Fatalf("want stale, got %s", h.Status)
	}
}

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("TEST_REDIS_URL"))
	if url == "" {
		url = strings.TrimSpace(os.Getenv("REDIS_URL"))
	}
	if url == "" {
		url = "redis://:d862d604a7d5adb0d2f800e72ca68a38aa2cf8edb5b0b0fa@115.190.247.9:6379/0"
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Skipf("redis url: %v", err)
	}
	rdb := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	return rdb
}
