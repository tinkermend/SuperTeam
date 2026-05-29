package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNodeNotFound      = errors.New("node not found")
	ErrNodeAlreadyExists = errors.New("node already exists")
	ErrInvalidStatus     = errors.New("invalid node status")
)

const (
	// HeartbeatTimeout is the duration after which a node is considered offline
	HeartbeatTimeout = 60 * time.Second
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("runtime repository is required")
	}
	return &Service{
		repository: repository,
	}, nil
}

// RegisterNode registers a new runtime node or updates an existing one
func (s *Service) RegisterNode(ctx context.Context, req RegisterNodeRequest) (*Node, error) {
	// Validate request
	if req.NodeID == "" {
		return nil, errors.New("node_id is required")
	}
	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if req.MaxSlots <= 0 {
		return nil, errors.New("max_slots must be greater than 0")
	}
	if len(req.SupportedProviders) == 0 {
		return nil, errors.New("supported_providers is required")
	}

	// Serialize supported providers
	providersJSON, err := json.Marshal(req.SupportedProviders)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize supported_providers: %w", err)
	}

	// Serialize metadata
	var metadataJSON []byte
	if req.Metadata != nil {
		metadataJSON, err = json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize metadata: %w", err)
		}
	} else {
		metadataJSON = []byte("{}")
	}

	// Check if node already exists
	_, err = s.repository.GetNode(ctx, req.NodeID)
	if err == nil {
		// Node exists, update it
		// Update heartbeat
		_, err = s.repository.UpdateHeartbeat(ctx, UpdateHeartbeatParams{
			NodeID:          req.NodeID,
			LastHeartbeatAt: timestamptzFromTime(time.Now()),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update heartbeat: %w", err)
		}

		// Update status to online
		record, err := s.repository.UpdateStatus(ctx, UpdateStatusParams{
			NodeID: req.NodeID,
			Status: string(NodeStatusOnline),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update status: %w", err)
		}

		return s.recordToNode(record)
	}

	// Node doesn't exist, create it
	params := CreateNodeParams{
		NodeID:             req.NodeID,
		Name:               req.Name,
		SupportedProviders: providersJSON,
		MaxSlots:           req.MaxSlots,
		CurrentLoad:        0,
		Status:             string(NodeStatusOnline),
		Metadata:           metadataJSON,
		LastHeartbeatAt:    timestamptzFromTime(time.Now()),
	}

	record, err := s.repository.CreateNode(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create node: %w", err)
	}

	return s.recordToNode(record)
}

// UpdateHeartbeat updates the heartbeat and load of a node
func (s *Service) UpdateHeartbeat(ctx context.Context, req UpdateHeartbeatRequest) (*Node, error) {
	// Validate request
	if req.NodeID == "" {
		return nil, errors.New("node_id is required")
	}
	if req.CurrentLoad < 0 {
		return nil, errors.New("current_load must be non-negative")
	}

	// Update heartbeat
	record, err := s.repository.UpdateHeartbeat(ctx, UpdateHeartbeatParams{
		NodeID:          req.NodeID,
		LastHeartbeatAt: timestamptzFromTime(time.Now()),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update heartbeat: %w", err)
	}

	// Update load
	record, err = s.repository.UpdateLoad(ctx, UpdateLoadParams{
		NodeID:      req.NodeID,
		CurrentLoad: req.CurrentLoad,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update load: %w", err)
	}

	// Determine status based on heartbeat
	node, err := s.recordToNode(record)
	if err != nil {
		return nil, err
	}

	// Update status if needed
	expectedStatus := NodeStatusOnline
	if !node.IsOnline() {
		expectedStatus = NodeStatusOffline
	}

	if node.Status != expectedStatus {
		record, err = s.repository.UpdateStatus(ctx, UpdateStatusParams{
			NodeID: req.NodeID,
			Status: string(expectedStatus),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update status: %w", err)
		}
		node, err = s.recordToNode(record)
		if err != nil {
			return nil, err
		}
	}

	return node, nil
}

// GetNode retrieves a node by ID
func (s *Service) GetNode(ctx context.Context, nodeID string) (*Node, error) {
	if nodeID == "" {
		return nil, errors.New("node_id is required")
	}

	record, err := s.repository.GetNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return s.recordToNode(record)
}

// ListNodes lists all nodes with optional filters
func (s *Service) ListNodes(ctx context.Context, filter ListNodesFilter) ([]*Node, error) {
	// Set default limit if not specified
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100 // Max limit
	}

	params := ListNodesParams{
		Status: s.statusToText(filter.Status),
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}

	records, err := s.repository.ListNodes(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := make([]*Node, 0, len(records))
	for _, record := range records {
		node, err := s.recordToNode(record)
		if err != nil {
			// Skip invalid nodes
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// ListOnlineNodes lists all online nodes (heartbeat within threshold)
func (s *Service) ListOnlineNodes(ctx context.Context) ([]*Node, error) {
	threshold := time.Now().Add(-HeartbeatTimeout)
	records, err := s.repository.ListOnlineNodes(ctx, timestamptzFromTime(threshold))
	if err != nil {
		return nil, fmt.Errorf("failed to list online nodes: %w", err)
	}

	nodes := make([]*Node, 0, len(records))
	for _, record := range records {
		node, err := s.recordToNode(record)
		if err != nil {
			// Skip invalid nodes
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// Helper methods

func (s *Service) recordToNode(record NodeRecord) (*Node, error) {
	// Deserialize supported providers
	var supportedProviders []string
	if err := json.Unmarshal(record.SupportedProviders, &supportedProviders); err != nil {
		return nil, fmt.Errorf("failed to deserialize supported_providers: %w", err)
	}

	// Deserialize metadata
	var metadata map[string]interface{}
	if len(record.Metadata) > 0 {
		if err := json.Unmarshal(record.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize metadata: %w", err)
		}
	}

	return &Node{
		ID:                 record.ID,
		NodeID:             record.NodeID,
		Name:               record.Name,
		SupportedProviders: supportedProviders,
		MaxSlots:           record.MaxSlots,
		CurrentLoad:        record.CurrentLoad,
		Status:             NodeStatus(record.Status),
		Metadata:           metadata,
		LastHeartbeatAt:    timeFromTimestamptz(record.LastHeartbeatAt),
		CreatedAt:          timeFromTimestamptz(record.CreatedAt),
		UpdatedAt:          timeFromTimestamptz(record.UpdatedAt),
	}, nil
}

func (s *Service) statusToText(status *NodeStatus) pgtype.Text {
	if status == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: string(*status), Valid: true}
}
