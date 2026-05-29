package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNoAvailableNode = errors.New("no available node found")
)

// Scheduler handles task scheduling to runtime nodes
type Scheduler struct {
	repository Repository
}

// NewScheduler creates a new scheduler instance
func NewScheduler(repository Repository) (*Scheduler, error) {
	if repository == nil {
		return nil, errors.New("runtime repository is required")
	}
	return &Scheduler{
		repository: repository,
	}, nil
}

// SelectNode selects the best available node for a given provider type
// It follows these rules:
// 1. Query nodes that support the provider and are online
// 2. Filter out nodes with full load (current_load >= max_slots)
// 3. Select the node with the lowest load
// 4. Update the node's current_load
func (s *Scheduler) SelectNode(ctx context.Context, providerType string) (*Node, error) {
	if providerType == "" {
		return nil, errors.New("provider_type is required")
	}

	// Get all online nodes
	threshold := time.Now().Add(-HeartbeatTimeout)
	nodes, err := s.repository.ListOnlineNodes(ctx, timestamptzFromTime(threshold))
	if err != nil {
		return nil, fmt.Errorf("failed to list online nodes: %w", err)
	}

	// Filter nodes that support the provider and have capacity
	var candidates []*Node
	for _, record := range nodes {
		node, err := s.recordToNode(record)
		if err != nil {
			continue
		}

		// Check if node supports the provider
		if !node.SupportsProvider(providerType) {
			continue
		}

		// Check if node has capacity
		if !node.HasCapacity() {
			continue
		}

		candidates = append(candidates, node)
	}

	// No available nodes
	if len(candidates) == 0 {
		return nil, ErrNoAvailableNode
	}

	// Select node with lowest load
	selectedNode := candidates[0]
	for _, node := range candidates[1:] {
		if node.CurrentLoad < selectedNode.CurrentLoad {
			selectedNode = node
		}
	}

	// Update node load
	record, err := s.repository.UpdateLoad(ctx, UpdateLoadParams{
		NodeID:      selectedNode.NodeID,
		CurrentLoad: selectedNode.CurrentLoad + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update node load: %w", err)
	}

	return s.recordToNode(record)
}

// Helper method to convert record to node
func (s *Scheduler) recordToNode(record NodeRecord) (*Node, error) {
	// Use the same conversion logic as Service
	svc := &Service{repository: s.repository}
	return svc.recordToNode(record)
}
