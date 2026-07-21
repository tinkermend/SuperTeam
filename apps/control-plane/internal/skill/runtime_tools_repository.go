package skill

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (r *PgRepository) ListRequiredToolsForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]string, error) {
	if r == nil || r.q == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	// dei retired: required tools are delivered via dispatch payload/MCP config.
	// Keep the heartbeat resolver surface; return empty without querying bindings.
	_ = ctx
	_ = tenantID
	_ = nodeID
	return []string{}, nil
}
