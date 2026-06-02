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
	ID     string
	NodeID string
	// Commands remains open after close; use Done for connection lifecycle.
	Commands  chan RuntimeCommand
	closed    chan struct{}
	closeOnce sync.Once
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
		closed:   make(chan struct{}),
	}

	r.mu.Lock()
	oldConnection := r.connections[nodeID]
	r.connections[nodeID] = connection
	r.mu.Unlock()

	if oldConnection != nil {
		oldConnection.close()
	}
	return connection
}

func (r *ConnectionRegistry) Unregister(nodeID, connectionID string) {
	var connectionToClose *RuntimeConnection
	r.mu.Lock()
	if connection := r.connections[nodeID]; connection != nil && connection.ID == connectionID {
		delete(r.connections, nodeID)
		connectionToClose = connection
	}
	r.mu.Unlock()
	if connectionToClose != nil {
		connectionToClose.close()
	}
}

func (r *ConnectionRegistry) Dispatch(ctx context.Context, nodeID string, command RuntimeCommand) error {
	r.mu.Lock()
	connection := r.connections[nodeID]
	r.mu.Unlock()
	if connection == nil {
		return ErrRuntimeNotConnected
	}
	return connection.send(ctx, command)
}

func (c *RuntimeConnection) Done() <-chan struct{} {
	return c.closed
}

func (c *RuntimeConnection) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
}

func (c *RuntimeConnection) send(ctx context.Context, command RuntimeCommand) error {
	select {
	case <-c.closed:
		return ErrRuntimeNotConnected
	default:
	}
	select {
	case c.Commands <- command:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return ErrRuntimeNotConnected
	}
}
