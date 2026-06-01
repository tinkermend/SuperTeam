package auth

import (
	"time"

	"github.com/google/uuid"
)

type RuntimeToken struct {
	ID        uuid.UUID `db:"id"`
	NodeID    string    `db:"node_id"`
	TokenHash string    `db:"token_hash"`
	ExpiresAt time.Time `db:"expires_at"`
	CreatedAt time.Time `db:"created_at"`
}
