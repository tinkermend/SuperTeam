package feishu

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/storage/queries"
)

// AuthUserDisplayNamer resolves console user labels for operational outbox rows.
type AuthUserDisplayNamer struct {
	Q interface {
		GetUserByID(ctx context.Context, id uuid.UUID) (queries.AuthUser, error)
	}
}

func (n AuthUserDisplayNamer) LookupDisplayNames(ctx context.Context, ids []uuid.UUID) map[uuid.UUID]string {
	out := make(map[uuid.UUID]string, len(ids))
	if n.Q == nil {
		return out
	}
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		user, err := n.Q.GetUserByID(ctx, id)
		if err != nil {
			continue
		}
		if label := authUserLabel(user); label != "" {
			out[id] = label
		}
	}
	return out
}

func authUserLabel(user queries.AuthUser) string {
	if user.DisplayName.Valid {
		if name := strings.TrimSpace(user.DisplayName.String); name != "" {
			return name
		}
	}
	return strings.TrimSpace(user.Username)
}
