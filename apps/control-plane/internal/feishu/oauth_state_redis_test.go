package feishu

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRedisOAuthStateStoreRoundTripAndOneShot(t *testing.T) {
	rdb := testRedis(t)
	store := NewRedisOAuthStateStore(rdb)
	store.ttl = 30 * time.Second
	ctx := context.Background()

	state := "oauth-state-" + uuid.NewString()
	key := feishuOAuthStateKey(state)
	t.Cleanup(func() { _ = rdb.Del(context.Background(), key).Err() })

	want := oauthState{
		TenantID:    uuid.New(),
		UserID:      uuid.New(),
		AppConfigID: uuid.New(),
		ReturnTo:    "/system-config",
	}
	if err := store.Put(ctx, state, want, store.ttl); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, ok, err := store.Take(ctx, state)
	if err != nil {
		t.Fatalf("take: %v", err)
	}
	if !ok {
		t.Fatal("expected take hit")
	}
	if got != want {
		t.Fatalf("take mismatch: got=%+v want=%+v", got, want)
	}

	_, ok, err = store.Take(ctx, state)
	if err != nil {
		t.Fatalf("second take: %v", err)
	}
	if ok {
		t.Fatal("expected one-shot miss on second take")
	}
}

func TestRedisOAuthStateStoreMiss(t *testing.T) {
	rdb := testRedis(t)
	store := NewRedisOAuthStateStore(rdb)
	_, ok, err := store.Take(context.Background(), "missing-"+uuid.NewString())
	if err != nil {
		t.Fatalf("take: %v", err)
	}
	if ok {
		t.Fatal("expected miss")
	}
}
