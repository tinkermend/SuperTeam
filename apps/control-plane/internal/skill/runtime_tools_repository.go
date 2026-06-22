package skill

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func (r *PgRepository) ListRequiredToolsForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]string, error) {
	if r == nil || r.q == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	return r.q.ListRequiredToolsForNode(ctx, queries.ListRequiredToolsForNodeParams{
		TenantID: tenantID,
		NodeID:   nodeID,
	})
}
