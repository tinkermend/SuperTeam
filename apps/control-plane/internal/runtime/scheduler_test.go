package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSchedulerRepository implements Repository for testing
type mockSchedulerRepository struct {
	nodes      map[string]NodeRecord
	updateLoad func(ctx context.Context, params UpdateLoadParams) (NodeRecord, error)
	listOnline func(ctx context.Context, threshold pgtype.Timestamptz) ([]NodeRecord, error)
}

func (m *mockSchedulerRepository) CreateNode(ctx context.Context, params CreateNodeParams) (NodeRecord, error) {
	return NodeRecord{}, nil
}

func (m *mockSchedulerRepository) GetNode(ctx context.Context, nodeID string) (NodeRecord, error) {
	if node, ok := m.nodes[nodeID]; ok {
		return node, nil
	}
	return NodeRecord{}, ErrNodeNotFound
}

func (m *mockSchedulerRepository) ListNodes(ctx context.Context, params ListNodesParams) ([]NodeRecord, error) {
	return nil, nil
}

func (m *mockSchedulerRepository) ListOnlineNodes(ctx context.Context, threshold pgtype.Timestamptz) ([]NodeRecord, error) {
	if m.listOnline != nil {
		return m.listOnline(ctx, threshold)
	}
	var records []NodeRecord
	for _, node := range m.nodes {
		records = append(records, node)
	}
	return records, nil
}

func (m *mockSchedulerRepository) UpdateHeartbeat(ctx context.Context, params UpdateHeartbeatParams) (NodeRecord, error) {
	return NodeRecord{}, nil
}

func (m *mockSchedulerRepository) UpdateLoad(ctx context.Context, params UpdateLoadParams) (NodeRecord, error) {
	if m.updateLoad != nil {
		return m.updateLoad(ctx, params)
	}
	if node, ok := m.nodes[params.NodeID]; ok {
		node.CurrentLoad = params.CurrentLoad
		m.nodes[params.NodeID] = node
		return node, nil
	}
	return NodeRecord{}, ErrNodeNotFound
}

func (m *mockSchedulerRepository) UpdateStatus(ctx context.Context, params UpdateStatusParams) (NodeRecord, error) {
	return NodeRecord{}, nil
}

func (m *mockSchedulerRepository) DeleteNode(ctx context.Context, nodeID string) error {
	return nil
}

func createTestNodeRecord(nodeID, name string, providers []string, maxSlots, currentLoad int32) NodeRecord {
	providersJSON, _ := json.Marshal(providers)
	metadataJSON := []byte("{}")

	return NodeRecord{
		ID:                 runtimeTestUUID(1),
		NodeID:             nodeID,
		Name:               name,
		SupportedProviders: providersJSON,
		MaxSlots:           maxSlots,
		CurrentLoad:        currentLoad,
		Status:             string(NodeStatusOnline),
		Metadata:           metadataJSON,
		LastHeartbeatAt:    timestamptzFromTime(time.Now()),
		CreatedAt:          timestamptzFromTime(time.Now()),
		UpdatedAt:          timestamptzFromTime(time.Now()),
	}
}

func TestNewScheduler(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mockSchedulerRepository{}
		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)
		assert.NotNil(t, scheduler)
	})

	t.Run("nil repository", func(t *testing.T) {
		scheduler, err := NewScheduler(nil)
		assert.Error(t, err)
		assert.Nil(t, scheduler)
		assert.Contains(t, err.Error(), "repository is required")
	})
}

func TestScheduler_SelectNode(t *testing.T) {
	ctx := context.Background()

	t.Run("success - single node", func(t *testing.T) {
		node1 := createTestNodeRecord("node-1", "Node 1", []string{"claude-code", "opencode"}, 5, 2)

		repo := &mockSchedulerRepository{
			nodes: map[string]NodeRecord{
				"node-1": node1,
			},
		}

		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)

		selected, err := scheduler.SelectNode(ctx, "claude-code")
		require.NoError(t, err)
		assert.NotNil(t, selected)
		assert.Equal(t, "node-1", selected.NodeID)
		assert.Equal(t, int32(3), selected.CurrentLoad) // Load increased by 1
	})

	t.Run("success - load balancing", func(t *testing.T) {
		node1 := createTestNodeRecord("node-1", "Node 1", []string{"claude-code"}, 5, 3)
		node2 := createTestNodeRecord("node-2", "Node 2", []string{"claude-code"}, 5, 1)
		node3 := createTestNodeRecord("node-3", "Node 3", []string{"claude-code"}, 5, 2)

		repo := &mockSchedulerRepository{
			nodes: map[string]NodeRecord{
				"node-1": node1,
				"node-2": node2,
				"node-3": node3,
			},
		}

		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)

		// Should select node-2 (lowest load)
		selected, err := scheduler.SelectNode(ctx, "claude-code")
		require.NoError(t, err)
		assert.NotNil(t, selected)
		assert.Equal(t, "node-2", selected.NodeID)
		assert.Equal(t, int32(2), selected.CurrentLoad)
	})

	t.Run("success - filter by provider", func(t *testing.T) {
		node1 := createTestNodeRecord("node-1", "Node 1", []string{"claude-code"}, 5, 1)
		node2 := createTestNodeRecord("node-2", "Node 2", []string{"opencode"}, 5, 0)
		node3 := createTestNodeRecord("node-3", "Node 3", []string{"claude-code", "opencode"}, 5, 2)

		repo := &mockSchedulerRepository{
			nodes: map[string]NodeRecord{
				"node-1": node1,
				"node-2": node2,
				"node-3": node3,
			},
		}

		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)

		// Should select node-1 (supports claude-code and has lower load than node-3)
		selected, err := scheduler.SelectNode(ctx, "claude-code")
		require.NoError(t, err)
		assert.NotNil(t, selected)
		assert.Equal(t, "node-1", selected.NodeID)
	})

	t.Run("success - filter by capacity", func(t *testing.T) {
		node1 := createTestNodeRecord("node-1", "Node 1", []string{"claude-code"}, 5, 5) // Full
		node2 := createTestNodeRecord("node-2", "Node 2", []string{"claude-code"}, 5, 2)
		node3 := createTestNodeRecord("node-3", "Node 3", []string{"claude-code"}, 3, 3) // Full

		repo := &mockSchedulerRepository{
			nodes: map[string]NodeRecord{
				"node-1": node1,
				"node-2": node2,
				"node-3": node3,
			},
		}

		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)

		// Should select node-2 (only one with capacity)
		selected, err := scheduler.SelectNode(ctx, "claude-code")
		require.NoError(t, err)
		assert.NotNil(t, selected)
		assert.Equal(t, "node-2", selected.NodeID)
		assert.Equal(t, int32(3), selected.CurrentLoad)
	})

	t.Run("error - no available nodes", func(t *testing.T) {
		node1 := createTestNodeRecord("node-1", "Node 1", []string{"claude-code"}, 5, 5) // Full
		node2 := createTestNodeRecord("node-2", "Node 2", []string{"opencode"}, 5, 2)    // Wrong provider

		repo := &mockSchedulerRepository{
			nodes: map[string]NodeRecord{
				"node-1": node1,
				"node-2": node2,
			},
		}

		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)

		selected, err := scheduler.SelectNode(ctx, "claude-code")
		assert.Error(t, err)
		assert.Nil(t, selected)
		assert.Equal(t, ErrNoAvailableNode, err)
	})

	t.Run("error - no nodes at all", func(t *testing.T) {
		repo := &mockSchedulerRepository{
			nodes: map[string]NodeRecord{},
		}

		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)

		selected, err := scheduler.SelectNode(ctx, "claude-code")
		assert.Error(t, err)
		assert.Nil(t, selected)
		assert.Equal(t, ErrNoAvailableNode, err)
	})

	t.Run("error - empty provider type", func(t *testing.T) {
		repo := &mockSchedulerRepository{
			nodes: map[string]NodeRecord{},
		}

		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)

		selected, err := scheduler.SelectNode(ctx, "")
		assert.Error(t, err)
		assert.Nil(t, selected)
		assert.Contains(t, err.Error(), "provider_type is required")
	})

	t.Run("success - multiple nodes with same load", func(t *testing.T) {
		node1 := createTestNodeRecord("node-1", "Node 1", []string{"claude-code"}, 5, 2)
		node2 := createTestNodeRecord("node-2", "Node 2", []string{"claude-code"}, 5, 2)
		node3 := createTestNodeRecord("node-3", "Node 3", []string{"claude-code"}, 5, 2)

		repo := &mockSchedulerRepository{
			nodes: map[string]NodeRecord{
				"node-1": node1,
				"node-2": node2,
				"node-3": node3,
			},
		}

		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)

		// Should select first node with lowest load (deterministic)
		selected, err := scheduler.SelectNode(ctx, "claude-code")
		require.NoError(t, err)
		assert.NotNil(t, selected)
		assert.Contains(t, []string{"node-1", "node-2", "node-3"}, selected.NodeID)
		assert.Equal(t, int32(3), selected.CurrentLoad)
	})

	t.Run("success - complex scenario", func(t *testing.T) {
		// Mix of different providers, loads, and capacities
		node1 := createTestNodeRecord("node-1", "Node 1", []string{"claude-code"}, 10, 8)
		node2 := createTestNodeRecord("node-2", "Node 2", []string{"opencode"}, 5, 2)
		node3 := createTestNodeRecord("node-3", "Node 3", []string{"claude-code", "opencode"}, 5, 1)
		node4 := createTestNodeRecord("node-4", "Node 4", []string{"claude-code"}, 3, 3) // Full
		node5 := createTestNodeRecord("node-5", "Node 5", []string{"claude-code", "codex"}, 5, 2)

		repo := &mockSchedulerRepository{
			nodes: map[string]NodeRecord{
				"node-1": node1,
				"node-2": node2,
				"node-3": node3,
				"node-4": node4,
				"node-5": node5,
			},
		}

		scheduler, err := NewScheduler(repo)
		require.NoError(t, err)

		// Should select node-3 (supports claude-code, has capacity, lowest load)
		selected, err := scheduler.SelectNode(ctx, "claude-code")
		require.NoError(t, err)
		assert.NotNil(t, selected)
		assert.Equal(t, "node-3", selected.NodeID)
		assert.Equal(t, int32(2), selected.CurrentLoad)
	})
}
