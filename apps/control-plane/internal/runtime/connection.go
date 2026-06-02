package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/google/uuid"
)

const runtimeCommandChannelSize = 16

var ErrRuntimeNotConnected = errors.New("runtime not connected")

type RuntimeCommand struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type RuntimeConnection struct {
	ID       string
	NodeID   string
	Commands chan RuntimeCommand
}

type ConnectionRegistry struct {
	mu          sync.Mutex
	connections map[string]*RuntimeConnection
}

func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		connections: map[string]*RuntimeConnection{},
	}
}

func (r *ConnectionRegistry) Register(nodeID string) *RuntimeConnection {
	connection := &RuntimeConnection{
		ID:       uuid.NewString(),
		NodeID:   nodeID,
		Commands: make(chan RuntimeCommand, runtimeCommandChannelSize),
	}

	r.mu.Lock()
	oldConnection := r.connections[nodeID]
	r.connections[nodeID] = connection
	if oldConnection != nil {
		close(oldConnection.Commands)
	}
	r.mu.Unlock()

	return connection
}

func (r *ConnectionRegistry) Unregister(nodeID, connectionID string) {
	r.mu.Lock()
	if connection := r.connections[nodeID]; connection != nil && connection.ID == connectionID {
		delete(r.connections, nodeID)
		close(connection.Commands)
	}
	r.mu.Unlock()
}

func (r *ConnectionRegistry) Dispatch(ctx context.Context, nodeID string, command RuntimeCommand) error {
	r.mu.Lock()
	connection := r.connections[nodeID]
	if connection == nil {
		r.mu.Unlock()
		return ErrRuntimeNotConnected
	}

	select {
	case connection.Commands <- command:
		r.mu.Unlock()
		return nil
	case <-ctx.Done():
		r.mu.Unlock()
		return ctx.Err()
	}
}
