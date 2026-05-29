package auth

import "time"

type RuntimeToken struct {
	ID        int64     `db:"id"`
	NodeID    string    `db:"node_id"`
	TokenHash string    `db:"token_hash"`
	ExpiresAt time.Time `db:"expires_at"`
	CreatedAt time.Time `db:"created_at"`
}
