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

func TestConnectionRegistryReplacesConnectionAndIgnoresStaleUnregister(t *testing.T) {
	registry := NewConnectionRegistry()
	oldConnection := registry.Register("node-1")
	newConnection := registry.Register("node-1")

	select {
	case _, ok := <-oldConnection.Commands:
		if ok {
			t.Fatalf("expected old connection command channel to be closed")
		}
	default:
		t.Fatalf("expected replacing a connection to close the old channel")
	}

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
