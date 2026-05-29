package runtime

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateNode(ctx context.Context, params CreateNodeParams) (NodeRecord, error) {
	node, err := r.q.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             params.NodeID,
		Name:               params.Name,
		SupportedProviders: params.SupportedProviders,
		MaxSlots:           params.MaxSlots,
		CurrentLoad:        params.CurrentLoad,
		Status:             params.Status,
		Metadata:           params.Metadata,
		LastHeartbeatAt:    params.LastHeartbeatAt,
	})
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) GetNode(ctx context.Context, nodeID string) (NodeRecord, error) {
	node, err := r.q.GetRuntimeNode(ctx, nodeID)
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) ListNodes(ctx context.Context, params ListNodesParams) ([]NodeRecord, error) {
	nodes, err := r.q.ListRuntimeNodes(ctx, queries.ListRuntimeNodesParams{
		Status: params.Status,
		Offset: params.Offset,
		Limit:  params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]NodeRecord, len(nodes))
	for i, node := range nodes {
		records[i] = NodeRecord{
			ID:                 node.ID,
			NodeID:             node.NodeID,
			Name:               node.Name,
			SupportedProviders: node.SupportedProviders,
			MaxSlots:           node.MaxSlots,
			CurrentLoad:        node.CurrentLoad,
			Status:             node.Status,
			Metadata:           node.Metadata,
			LastHeartbeatAt:    node.LastHeartbeatAt,
			CreatedAt:          node.CreatedAt,
			UpdatedAt:          node.UpdatedAt,
		}
	}
	return records, nil
}

func (r *PgRepository) ListOnlineNodes(ctx context.Context, threshold pgtype.Timestamptz) ([]NodeRecord, error) {
	nodes, err := r.q.ListOnlineRuntimeNodes(ctx, threshold)
	if err != nil {
		return nil, err
	}
	records := make([]NodeRecord, len(nodes))
	for i, node := range nodes {
		records[i] = NodeRecord{
			ID:                 node.ID,
			NodeID:             node.NodeID,
			Name:               node.Name,
			SupportedProviders: node.SupportedProviders,
			MaxSlots:           node.MaxSlots,
			CurrentLoad:        node.CurrentLoad,
			Status:             node.Status,
			Metadata:           node.Metadata,
			LastHeartbeatAt:    node.LastHeartbeatAt,
			CreatedAt:          node.CreatedAt,
			UpdatedAt:          node.UpdatedAt,
		}
	}
	return records, nil
}

func (r *PgRepository) UpdateHeartbeat(ctx context.Context, params UpdateHeartbeatParams) (NodeRecord, error) {
	node, err := r.q.UpdateRuntimeNodeHeartbeat(ctx, queries.UpdateRuntimeNodeHeartbeatParams{
		NodeID:          params.NodeID,
		LastHeartbeatAt: params.LastHeartbeatAt,
	})
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) UpdateLoad(ctx context.Context, params UpdateLoadParams) (NodeRecord, error) {
	node, err := r.q.UpdateRuntimeNodeLoad(ctx, queries.UpdateRuntimeNodeLoadParams{
		NodeID:      params.NodeID,
		CurrentLoad: params.CurrentLoad,
	})
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) UpdateStatus(ctx context.Context, params UpdateStatusParams) (NodeRecord, error) {
	node, err := r.q.UpdateRuntimeNodeStatus(ctx, queries.UpdateRuntimeNodeStatusParams{
		NodeID: params.NodeID,
		Status: params.Status,
	})
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) DeleteNode(ctx context.Context, nodeID string) error {
	return r.q.DeleteRuntimeNode(ctx, nodeID)
}
