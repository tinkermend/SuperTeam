package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Repository defines the interface for runtime node data access
type Repository interface {
	// Node operations
	CreateNode(ctx context.Context, params CreateNodeParams) (NodeRecord, error)
	GetNode(ctx context.Context, nodeID string) (NodeRecord, error)
	ListNodes(ctx context.Context, params ListNodesParams) ([]NodeRecord, error)
	ListOnlineNodes(ctx context.Context, heartbeatThreshold pgtype.Timestamptz) ([]NodeRecord, error)
	UpdateHeartbeat(ctx context.Context, params UpdateHeartbeatParams) (NodeRecord, error)
	UpdateLoad(ctx context.Context, params UpdateLoadParams) (NodeRecord, error)
	UpdateStatus(ctx context.Context, params UpdateStatusParams) (NodeRecord, error)
	DeleteNode(ctx context.Context, nodeID string) error
}

// CreateNodeParams represents parameters for creating a node
type CreateNodeParams struct {
	NodeID             string
	Name               string
	SupportedProviders []byte
	MaxSlots           int32
	CurrentLoad        int32
	Status             string
	Metadata           []byte
	LastHeartbeatAt    pgtype.Timestamptz
}

// NodeRecord represents a node record from the database
type NodeRecord struct {
	ID                 uuid.UUID
	NodeID             string
	Name               string
	SupportedProviders []byte
	MaxSlots           int32
	CurrentLoad        int32
	Status             string
	Metadata           []byte
	LastHeartbeatAt    pgtype.Timestamptz
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

// ListNodesParams represents parameters for listing nodes
type ListNodesParams struct {
	Status pgtype.Text
	Offset int32
	Limit  int32
}

// UpdateHeartbeatParams represents parameters for updating heartbeat
type UpdateHeartbeatParams struct {
	NodeID          string
	LastHeartbeatAt pgtype.Timestamptz
}

// UpdateLoadParams represents parameters for updating load
type UpdateLoadParams struct {
	NodeID      string
	CurrentLoad int32
}

// UpdateStatusParams represents parameters for updating status
type UpdateStatusParams struct {
	NodeID string
	Status string
}
