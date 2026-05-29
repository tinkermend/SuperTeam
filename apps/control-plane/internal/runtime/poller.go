package runtime

import (
	"context"
	"errors"
	"sync"

	"github.com/superteam/control-plane/internal/task"
)

var (
	// ErrPollerClosed is returned when the poller is closed
	ErrPollerClosed = errors.New("poller is closed")
)

// Poller manages long polling for task assignment
type Poller struct {
	waiters map[string]chan *task.Task
	mu      sync.RWMutex
	closed  bool
}

// NewPoller creates a new Poller instance
func NewPoller() *Poller {
	return &Poller{
		waiters: make(map[string]chan *task.Task),
	}
}

// WaitForTask waits for a task to be assigned to the specified node
// Returns the task when available, or an error if the context is cancelled or the poller is closed
func (p *Poller) WaitForTask(ctx context.Context, nodeID string) (*task.Task, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPollerClosed
	}

	// Create a channel for this waiter
	ch := make(chan *task.Task, 1)
	p.waiters[nodeID] = ch
	p.mu.Unlock()

	// Clean up the waiter when done
	defer func() {
		p.mu.Lock()
		// Only delete and close if the channel still exists in the map
		// (it might have been removed by Close())
		if existingCh, ok := p.waiters[nodeID]; ok && existingCh == ch {
			delete(p.waiters, nodeID)
			close(ch)
		}
		p.mu.Unlock()
	}()

	// Wait for either a task or context cancellation
	select {
	case t, ok := <-ch:
		if !ok {
			// Channel was closed (poller closed)
			return nil, ErrPollerClosed
		}
		return t, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// NotifyTask notifies a waiting node that a task is available
// If no waiter exists for the node, the notification is dropped
func (p *Poller) NotifyTask(nodeID string, t *task.Task) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return
	}

	if ch, ok := p.waiters[nodeID]; ok {
		// Non-blocking send to avoid deadlock
		select {
		case ch <- t:
		default:
			// Channel is full or closed, drop the notification
		}
	}
}

// Close closes the poller and cancels all waiting requests
func (p *Poller) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	p.closed = true

	// Close all waiter channels
	for _, ch := range p.waiters {
		close(ch)
	}

	// Clear the waiters map
	p.waiters = make(map[string]chan *task.Task)
}

// ActiveWaiters returns the number of active waiters
func (p *Poller) ActiveWaiters() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.waiters)
}
