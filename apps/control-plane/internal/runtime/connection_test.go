package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestConnectionRegistryDispatchDeliversCommand(t *testing.T) {
	registry := NewConnectionRegistry()
	connection := registry.Register("node-1")

	command := RuntimeCommand{
		ID:      "cmd-1",
		Type:    "task.claim",
		Payload: json.RawMessage(`{"task_id":"task-1"}`),
	}
	if err := registry.Dispatch(context.Background(), "node-1", command); err != nil {
		t.Fatalf("dispatch command: %v", err)
	}

	select {
	case got := <-connection.Commands:
		if got.ID != command.ID || got.Type != command.Type || string(got.Payload) != string(command.Payload) {
			t.Fatalf("unexpected command: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for dispatched command")
	}
}

func TestConnectionRegistryDispatchMissingNodeReturnsErrRuntimeNotConnected(t *testing.T) {
	registry := NewConnectionRegistry()

	err := registry.Dispatch(context.Background(), "missing-node", RuntimeCommand{ID: "cmd-1", Type: "noop"})

	if !errors.Is(err, ErrRuntimeNotConnected) {
		t.Fatalf("expected ErrRuntimeNotConnected, got %v", err)
	}
}

func TestConnectionRegistryIsConnected(t *testing.T) {
	registry := NewConnectionRegistry()

	if registry.IsConnected("node-1") {
		t.Fatalf("expected missing node to be disconnected")
	}

	connection := registry.Register("node-1")
	if !registry.IsConnected("node-1") {
		t.Fatalf("expected registered node to be connected")
	}

	replacement := registry.Register("node-1")
	if !registry.IsConnected("node-1") {
		t.Fatalf("expected replacement connection to be connected")
	}
	if !isConnectionClosed(connection) {
		t.Fatalf("expected replaced connection to be closed")
	}

	registry.Unregister("node-1", connection.ID)
	if !registry.IsConnected("node-1") {
		t.Fatalf("expected stale unregister not to disconnect replacement")
	}

	registry.Unregister("node-1", replacement.ID)
	if registry.IsConnected("node-1") {
		t.Fatalf("expected unregistered node to be disconnected")
	}
}

func TestConnectionRegistryReplacesConnectionAndIgnoresStaleUnregister(t *testing.T) {
	registry := NewConnectionRegistry()
	oldConnection := registry.Register("node-1")
	newConnection := registry.Register("node-1")

	registry.Unregister("node-1", oldConnection.ID)

	command := RuntimeCommand{ID: "cmd-2", Type: "task.claim"}
	if err := registry.Dispatch(context.Background(), "node-1", command); err != nil {
		t.Fatalf("expected stale unregister not to remove active connection: %v", err)
	}
	select {
	case got := <-newConnection.Commands:
		if got.ID != command.ID {
			t.Fatalf("unexpected command after stale unregister: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for command on active connection")
	}
}

func TestConnectionRegistryDispatchAfterUnregisterReturnsErrRuntimeNotConnected(t *testing.T) {
	registry := NewConnectionRegistry()
	connection := registry.Register("node-1")

	registry.Unregister("node-1", connection.ID)

	err := registry.Dispatch(context.Background(), "node-1", RuntimeCommand{ID: "cmd-1", Type: "noop"})
	if !errors.Is(err, ErrRuntimeNotConnected) {
		t.Fatalf("expected ErrRuntimeNotConnected after unregister, got %v", err)
	}
}

func TestConnectionRegistryDispatchRespectsContextCancellationWhenChannelFull(t *testing.T) {
	registry := NewConnectionRegistry()
	connection := registry.Register("node-1")
	for i := 0; i < cap(connection.Commands); i++ {
		connection.Commands <- RuntimeCommand{ID: "fill", Type: "noop"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := registry.Dispatch(ctx, "node-1", RuntimeCommand{ID: "blocked", Type: "noop"})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline when command channel is full, got %v", err)
	}
}

func TestConnectionRegistryFullChannelDispatchDoesNotBlockRegisterOrUnregister(t *testing.T) {
	registry := NewConnectionRegistry()
	connection := registry.Register("node-1")
	fillCommandChannel(connection)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- registry.Dispatch(ctx, "node-1", RuntimeCommand{ID: "blocked", Type: "noop"})
	}()
	assertDispatchStillBlocked(t, errCh)

	registerDone := make(chan struct{})
	var replacement *RuntimeConnection
	go func() {
		replacement = registry.Register("node-1")
		close(registerDone)
	}()
	assertCompletesQuickly(t, registerDone, "register")

	unregisterDone := make(chan struct{})
	go func() {
		registry.Unregister("node-1", replacement.ID)
		close(unregisterDone)
	}()
	assertCompletesQuickly(t, unregisterDone, "unregister")

	cancel()
	assertDispatchReturnsError(t, errCh)

	unregisterRegistry := NewConnectionRegistry()
	unregisterConnection := unregisterRegistry.Register("node-2")
	fillCommandChannel(unregisterConnection)
	unregisterCtx, unregisterCancel := context.WithCancel(context.Background())
	defer unregisterCancel()
	unregisterErrCh := make(chan error, 1)
	go func() {
		unregisterErrCh <- unregisterRegistry.Dispatch(unregisterCtx, "node-2", RuntimeCommand{ID: "blocked", Type: "noop"})
	}()
	assertDispatchStillBlocked(t, unregisterErrCh)

	activeUnregisterDone := make(chan struct{})
	go func() {
		unregisterRegistry.Unregister("node-2", unregisterConnection.ID)
		close(activeUnregisterDone)
	}()
	assertCompletesQuickly(t, activeUnregisterDone, "active unregister")
	unregisterCancel()
	assertDispatchReturnsError(t, unregisterErrCh)
}

func isConnectionClosed(connection *RuntimeConnection) bool {
	select {
	case <-connection.Done():
		return true
	default:
		return false
	}
}

func fillCommandChannel(connection *RuntimeConnection) {
	for i := 0; i < cap(connection.Commands); i++ {
		connection.Commands <- RuntimeCommand{ID: "fill", Type: "noop"}
	}
}

func assertDispatchStillBlocked(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		t.Fatalf("expected dispatch to block on full channel, got %v", err)
	case <-time.After(20 * time.Millisecond):
	}
}

func assertCompletesQuickly(t *testing.T, done <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("%s blocked behind full command channel dispatch", operation)
	}
}

func assertDispatchReturnsError(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("expected blocked dispatch to end with an error")
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for blocked dispatch to finish")
	}
}
