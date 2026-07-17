package session

import (
	"testing"
	"time"
)

func TestStorePutGetClear(t *testing.T) {
	store := NewStore(time.Minute)
	if _, ok := store.Get("ou_x"); ok {
		t.Fatalf("empty store must miss")
	}
	store.Put("ou_x", FormState{Stage: StagePickProject, UserID: "u1"})
	state, ok := store.Get("ou_x")
	if !ok || state.Stage != StagePickProject || state.UserID != "u1" {
		t.Fatalf("unexpected state %#v ok=%v", state, ok)
	}
	store.Clear("ou_x")
	if _, ok := store.Get("ou_x"); ok {
		t.Fatalf("cleared state must miss")
	}
}

func TestStoreTTLExpiry(t *testing.T) {
	store := NewStore(10 * time.Minute)
	current := time.Now()
	store.SetClock(func() time.Time { return current })
	store.Put("ou_x", FormState{Stage: StageAwaitContent})
	current = current.Add(11 * time.Minute)
	if _, ok := store.Get("ou_x"); ok {
		t.Fatalf("expired state must miss")
	}
}
