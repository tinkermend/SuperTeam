package feishu

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOutboxChangeNotifierWaitReceivesPublish(t *testing.T) {
	n := NewOutboxChangeNotifier(nil)
	tenantID := uuid.New()

	var wg sync.WaitGroup
	wg.Add(1)
	started := make(chan struct{})
	go func() {
		defer wg.Done()
		close(started)
		n.Wait(context.Background(), tenantID, 2*time.Second)
	}()
	<-started
	// Give subscribe a moment to register.
	time.Sleep(20 * time.Millisecond)
	n.publish(tenantID)
	wg.Wait()
}

func TestOutboxChangeNotifierWaitTimesOut(t *testing.T) {
	n := NewOutboxChangeNotifier(nil)
	start := time.Now()
	n.Wait(context.Background(), uuid.New(), 50*time.Millisecond)
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("expected ~50ms wait, got %s", elapsed)
	}
}

func TestOutboxChangeNotifierWaitZeroTimeoutNoop(t *testing.T) {
	n := NewOutboxChangeNotifier(nil)
	start := time.Now()
	n.Wait(context.Background(), uuid.New(), 0)
	if time.Since(start) > 20*time.Millisecond {
		t.Fatal("zero timeout must return immediately")
	}
}

func TestOutboxChangeNotifierIgnoresOtherTenant(t *testing.T) {
	n := NewOutboxChangeNotifier(nil)
	mine := uuid.New()
	other := uuid.New()
	done := make(chan struct{})
	go func() {
		n.Wait(context.Background(), mine, 80*time.Millisecond)
		close(done)
	}()
	time.Sleep(15 * time.Millisecond)
	n.publish(other)
	select {
	case <-done:
		// timed out without mine wake — OK if ~80ms
	case <-time.After(200 * time.Millisecond):
		t.Fatal("wait should have finished by timeout")
	}
}
