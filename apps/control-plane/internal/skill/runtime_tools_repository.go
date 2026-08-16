package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func (r *PgRepository) ListRequiredToolsForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]string, error) {
	if r == nil || r.q == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(nodeID) == "" {
		return nil, fmt.Errorf("%w: node_id is required", ErrInvalidInput)
	}
	tools, err := r.q.ListRequiredToolsForNode(ctx, queries.ListRequiredToolsForNodeParams{
		TenantID: tenantID,
		NodeID:   nodeID,
	})
	if err != nil {
		return nil, err
	}
	if tools == nil {
		return []string{}, nil
	}
	return tools, nil
}
