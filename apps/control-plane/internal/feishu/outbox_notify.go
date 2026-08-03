package feishu

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxChangeChannel is the Postgres NOTIFY channel feishu_outbox triggers publish to.
// Payload is the tenant id as text; see migration 20260803160000.
const OutboxChangeChannel = "feishu_outbox_changed"

const outboxListenerRetryDelay = 3 * time.Second

// OutboxChangeNotifier fans a single Postgres LISTEN connection out to every
// waiting ListOutbox long-poll in this process.
//
// Delivery is best effort: NOTIFY is not durable, so ListOutbox must re-query
// after wait timeout rather than treat this as the only wake source.
type OutboxChangeNotifier struct {
	pool *pgxpool.Pool

	mu   sync.Mutex
	next int
	subs map[int]outboxSubscription
}

type outboxSubscription struct {
	tenantID uuid.UUID
	ch       chan struct{}
}

func NewOutboxChangeNotifier(pool *pgxpool.Pool) *OutboxChangeNotifier {
	return &OutboxChangeNotifier{pool: pool, subs: map[int]outboxSubscription{}}
}

// Wait blocks until the tenant's outbox changes, ctx is cancelled, or timeout elapses.
// timeout <= 0 returns immediately.
func (n *OutboxChangeNotifier) Wait(ctx context.Context, tenantID uuid.UUID, timeout time.Duration) {
	if n == nil || timeout <= 0 || tenantID == uuid.Nil {
		return
	}
	ch, cancel := n.subscribe(tenantID)
	defer cancel()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-ch:
	case <-timer.C:
	}
}

func (n *OutboxChangeNotifier) subscribe(tenantID uuid.UUID) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	id := n.next
	n.next++
	n.subs[id] = outboxSubscription{tenantID: tenantID, ch: ch}
	n.mu.Unlock()
	return ch, func() {
		n.mu.Lock()
		delete(n.subs, id)
		n.mu.Unlock()
	}
}

func (n *OutboxChangeNotifier) publish(tenantID uuid.UUID) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, sub := range n.subs {
		if sub.tenantID != tenantID {
			continue
		}
		select {
		case sub.ch <- struct{}{}:
		default:
		}
	}
}

// Start holds a dedicated connection on LISTEN until ctx is cancelled.
func (n *OutboxChangeNotifier) Start(ctx context.Context) {
	if n == nil || n.pool == nil {
		return
	}
	for {
		if err := n.listen(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("feishu outbox change listener dropped, retrying", "error", err)
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(outboxListenerRetryDelay):
		}
	}
}

func (n *OutboxChangeNotifier) listen(ctx context.Context) error {
	conn, err := n.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+OutboxChangeChannel); err != nil {
		return err
	}
	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		tenantID, parseErr := uuid.Parse(notification.Payload)
		if parseErr != nil {
			continue
		}
		n.publish(tenantID)
	}
}
