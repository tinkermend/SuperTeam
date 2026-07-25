package retention

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// advisoryLockKey identifies the retention sweep lock. Any constant works as
// long as nothing else in the schema reuses it; it is deliberately far from the
// hashed per-project keys used by LockProjectEventSequence.
const advisoryLockKey int64 = 0x5375_7065_5254_4E00 // "SupeRTN\0"

// PgSingleton serialises the retention sweep across Control Plane processes with
// a session-scoped Postgres advisory lock.
//
// Session-scoped (pg_try_advisory_lock) rather than transaction-scoped: the
// sweep spans many short transactions, so the lock has to outlive each of them.
// That means it is pinned to one connection, which must be held for the whole
// sweep and explicitly released — hence the dedicated Acquire/Release pair
// rather than running through the pool.
//
// This is a stand-in for real leader election. When the Control Plane gains one,
// replace this implementation; the Singleton interface is the seam.
type PgSingleton struct {
	pool *pgxpool.Pool
	conn *pgxpool.Conn
}

func NewPgSingleton(pool *pgxpool.Pool) *PgSingleton {
	return &PgSingleton{pool: pool}
}

func (s *PgSingleton) TryAcquire(ctx context.Context) (bool, error) {
	if s == nil || s.pool == nil {
		return false, nil
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", advisoryLockKey).Scan(&acquired); err != nil {
		conn.Release()
		return false, err
	}
	if !acquired {
		conn.Release()
		return false, nil
	}
	s.conn = conn
	return true, nil
}

func (s *PgSingleton) Release(ctx context.Context) {
	if s == nil || s.conn == nil {
		return
	}
	// Release the advisory lock explicitly. Returning the connection to the pool
	// without unlocking would leak the lock for the connection's lifetime and
	// block every later sweep, including this process's own.
	if _, err := s.conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockKey); err != nil {
		slog.Error("failed to release retention advisory lock", "error", err)
		// Destroy the connection rather than return a still-locked one to the pool.
		s.conn.Conn().Close(ctx)
	}
	s.conn.Release()
	s.conn = nil
}
