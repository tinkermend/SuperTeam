package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/superteam/control-plane/internal/api"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

func TestHealthOnlyRouterIsExplicit(t *testing.T) {
	router := NewHealthOnlyRouter()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON health response: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
	if body["service"] != "control-plane" {
		t.Fatalf("expected service control-plane, got %q", body["service"])
	}
}

func TestRunContainerClosesPollerWhenContextIsCanceled(t *testing.T) {
	poller := runtimepkg.NewPoller()
	container := &Container{
		Poller: poller,
		Server: api.NewServer(nil, nil),
	}

	waitErr := make(chan error, 1)
	go func() {
		_, err := poller.WaitForTask(context.Background(), "node-1")
		waitErr <- err
	}()

	waitForActivePoller(t, poller)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runContainer(ctx, container, "127.0.0.1:0"); err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}

	if err := <-waitErr; !errors.Is(err, runtimepkg.ErrPollerClosed) {
		t.Fatalf("expected poller waiter to be closed, got %v", err)
	}
}

func waitForActivePoller(t *testing.T, poller *runtimepkg.Poller) {
	t.Helper()

	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		if poller.ActiveWaiters() > 0 {
			return
		}

		select {
		case <-deadline:
			t.Fatal("timed out waiting for active poller waiter")
		case <-ticker.C:
		}
	}
}
