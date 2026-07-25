package inbox

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChangeChannel is the Postgres NOTIFY channel the inbox_items triggers publish
// to. Payload is the tenant id as text; see migration 20260724183529.
const ChangeChannel = "inbox_changed"

// listenerRetryDelay backs off before re-establishing a dropped LISTEN
// connection. Short enough that the gap is covered by the SSE fallback poll.
const listenerRetryDelay = 3 * time.Second

// ChangeNotifier fans a single Postgres LISTEN connection out to every open SSE
// stream in this process.
//
// It replaces per-connection polling: each SSE stream used to run its own
// PeekInboxChange every 2 seconds, so database load grew linearly with the
// number of open browser tabs and each probe held a pooled connection for a full
// round trip. One listener plus in-process fan-out makes that cost constant.
//
// Delivery is best effort by design. NOTIFY is not durable — notifications
// raised while the listener is reconnecting are simply lost — so subscribers
// must keep a low-frequency fallback poll rather than treat this as the only
// wake-up source.
type ChangeNotifier struct {
	pool *pgxpool.Pool

	mu   sync.Mutex
	next int
	subs map[int]subscription
}

type subscription struct {
	tenantID uuid.UUID
	ch       chan struct{}
}

func NewChangeNotifier(pool *pgxpool.Pool) *ChangeNotifier {
	return &ChangeNotifier{pool: pool, subs: map[int]subscription{}}
}

// Subscribe returns a channel that receives a token whenever the given tenant's
// inbox changes, plus a cancel func the caller must invoke when the stream ends.
//
// The channel has depth 1 and sends are non-blocking: a subscriber that has not
// drained its previous token already knows it is stale, so collapsing bursts
// into one wake-up loses nothing and keeps a slow SSE writer from stalling the
// listener for everyone.
func (n *ChangeNotifier) Subscribe(tenantID uuid.UUID) (<-chan struct{}, func()) {
	if n == nil {
		return nil, func() {}
	}
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	id := n.next
	n.next++
	n.subs[id] = subscription{tenantID: tenantID, ch: ch}
	n.mu.Unlock()
	return ch, func() {
		n.mu.Lock()
		delete(n.subs, id)
		n.mu.Unlock()
	}
}

func (n *ChangeNotifier) publish(tenantID uuid.UUID) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, sub := range n.subs {
		if sub.tenantID != tenantID {
			continue
		}
		select {
		case sub.ch <- struct{}{}:
		default: // a wake-up is already pending; collapsing is correct here
		}
	}
}

// Start holds a dedicated connection on LISTEN until ctx is cancelled,
// reconnecting on failure. It is safe to run with no subscribers.
func (n *ChangeNotifier) Start(ctx context.Context) {
	if n == nil || n.pool == nil {
		return
	}
	for {
		if err := n.listen(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("inbox change listener dropped, retrying", "error", err)
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(listenerRetryDelay):
		}
	}
}

func (n *ChangeNotifier) listen(ctx context.Context) error {
	// A dedicated connection, not a pooled query: LISTEN registrations belong to
	// a session, so the connection must be held for as long as we want to hear
	// notifications.
	conn, err := n.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+ChangeChannel); err != nil {
		return err
	}
	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		tenantID, parseErr := uuid.Parse(notification.Payload)
		if parseErr != nil {
			// Unknown payload shape: ignore rather than kill the listener.
			continue
		}
		n.publish(tenantID)
	}
}
