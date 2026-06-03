package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestToDomainUserDefaultsMissingAvatarSeed(t *testing.T) {
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	user := toDomainUser(queries.AuthUser{
		ID:             uuid.New(),
		Username:       "legacy-admin",
		PasswordHash:   "hash",
		Status:         UserStatusActive,
		AvatarProvider: "dicebear",
		AvatarStyle:    "adventurer",
		AvatarOptions:  []byte(`{}`),
		CreatedAt:      now,
		UpdatedAt:      now,
	})

	if user.Avatar.Seed != "user:legacy-admin" {
		t.Fatalf("expected legacy user avatar seed fallback, got %#v", user.Avatar)
	}
}
