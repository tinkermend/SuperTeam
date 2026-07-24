package runtime

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSchedulerRepository implements Repository for testing.
// TryAcquireNodeSlot is mutex-protected and enforces the same capacity and
// liveness guards as the real SQL query, so concurrent tests faithfully model
// the atomic acquire semantics.
type mockSchedulerRepository struct {
	mu               sync.Mutex
	nodes            map[string]NodeRecord
	updateLoad       func(ctx context.Context, params UpdateLoadParams) (NodeRecord, error)
	listOnline       func(ctx context.Context, threshold pgtype.Timestamptz) ([]NodeRecord, error)
	tryAcquireSlot   func(ctx context.Context, nodeID string, threshold pgtype.Timestamptz) (NodeRecord, error)
	acquireAttempts  map[string]int
	acquireSuccesses map[string]int
}

func (m *mockSchedulerRepository) CreateNode(ctx context.Context, params CreateNodeParams) (NodeRecord, error) {
	return NodeRecord{}, nil
}

func (m *mockSchedulerRepository) GetNode(ctx context.Context, nodeID string) (NodeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if node, ok := m.nodes[nodeID]; ok {
		return node, nil
	}
	return NodeRecord{}, ErrNodeNotFound
}

func (m *mockSchedulerRepository) GetNodeByID(ctx context.Context, id uuid.UUID) (NodeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, node := range m.nodes {
		if node.ID == id {
			return node, nil
		}
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
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a defensive copy so callers can mutate without racing the map.
	records := make([]NodeRecord, 0, len(m.nodes))
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if node, ok := m.nodes[params.NodeID]; ok {
		node.CurrentLoad = params.CurrentLoad
		m.nodes[params.NodeID] = node
		return node, nil
	}
	return NodeRecord{}, ErrNodeNotFound
}

// TryAcquireNodeSlot mirrors the atomic SQL UPDATE: capacity guard
// (current_load < max_slots), online status, and heartbeat freshness are all
// re-checked under the lock so concurrent callers cannot overrun max_slots.
func (m *mockSchedulerRepository) TryAcquireNodeSlot(ctx context.Context, nodeID string, threshold pgtype.Timestamptz) (NodeRecord, error) {
	if m.tryAcquireSlot != nil {
		return m.tryAcquireSlot(ctx, nodeID, threshold)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.acquireAttempts != nil {
		m.acquireAttempts[nodeID]++
	}
	node, ok := m.nodes[nodeID]
	if !ok {
		return NodeRecord{}, pgx.ErrNoRows
	}
	if node.Status != string(NodeStatusOnline) {
		return NodeRecord{}, pgx.ErrNoRows
	}
	if node.LastHeartbeatAt.Valid && threshold.Valid && node.LastHeartbeatAt.Time.Before(threshold.Time) {
		return NodeRecord{}, pgx.ErrNoRows
	}
	if node.CurrentLoad >= node.MaxSlots {
		return NodeRecord{}, pgx.ErrNoRows
	}
	node.CurrentLoad++
	m.nodes[nodeID] = node
	if m.acquireSuccesses != nil {
		m.acquireSuccesses[nodeID]++
	}
	return node, nil
}

func (m *mockSchedulerRepository) UpdateStatus(ctx context.Context, params UpdateStatusParams) (NodeRecord, error) {
	return NodeRecord{}, nil
}

func (m *mockSchedulerRepository) PatchNodeMetadata(ctx context.Context, params PatchNodeMetadataParams) (NodeRecord, error) {
	return NodeRecord{}, nil
}

func (m *mockSchedulerRepository) CountOnlineNodesWithoutPlatformLimits(ctx context.Context, tenantID uuid.UUID, threshold pgtype.Timestamptz) (int64, error) {
	return 0, nil
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

// TestScheduler_SelectNode_Concurrency proves the atomic-acquire fix holds:
// with max_slots=2 and 10 concurrent SelectNode callers, exactly 2 may win a
// slot and the rest must receive ErrNoAvailableNode. Before the fix this test
// would fail because every goroutine read current_load=0 and each wrote 1,
// leaving the node over its slot limit.
func TestScheduler_SelectNode_Concurrency(t *testing.T) {
	const maxSlots = int32(2)
	const numCallers = 10

	// Single node with capacity for 2 slots, currently empty.
	repo := &mockSchedulerRepository{
		nodes: map[string]NodeRecord{
			"node-1": createTestNodeRecord("node-1", "Node 1", []string{"claude-code"}, maxSlots, 0),
		},
		acquireAttempts:  map[string]int{},
		acquireSuccesses: map[string]int{},
	}

	scheduler, err := NewScheduler(repo)
	require.NoError(t, err)

	var wg sync.WaitGroup
	start := make(chan struct{})
	type result struct {
		nodeID string
		err    error
	}
	results := make([]result, numCallers)
	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // release all goroutines at once to maximize contention
			node, err := scheduler.SelectNode(context.Background(), "claude-code")
			if err != nil {
				results[idx] = result{err: err}
				return
			}
			results[idx] = result{nodeID: node.NodeID}
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, r := range results {
		if r.err == nil {
			successes++
			assert.Equal(t, "node-1", r.nodeID)
		} else {
			assert.Equal(t, ErrNoAvailableNode, r.err)
		}
	}

	// Exactly maxSlots callers may win a slot; the rest are rejected.
	assert.Equal(t, int(maxSlots), successes, "must not exceed max_slots")
	assert.Equal(t, numCallers-int(maxSlots), numCallers-successes, "excess callers must be rejected")

	// Final persisted load must equal maxSlots, not the number of callers.
	repo.mu.Lock()
	finalLoad := repo.nodes["node-1"].CurrentLoad
	repo.mu.Unlock()
	assert.Equal(t, maxSlots, finalLoad, "persisted current_load must be capped at max_slots")

	// Sanity: the node was attempted more times than it had slots.
	repo.mu.Lock()
	attempts := repo.acquireAttempts["node-1"]
	repo.mu.Unlock()
	assert.Greater(t, attempts, int(maxSlots), "oversubscribed callers must have contended on acquire")
}

// TestScheduler_SelectNode_Concurrency_MultiNode ensures that when one node is
// saturated, concurrent callers fall through to the next candidate rather than
// overbooking the first.
func TestScheduler_SelectNode_Concurrency_MultiNode(t *testing.T) {
	// Two nodes, each with 1 slot. Five concurrent callers: 2 win, 3 rejected.
	repo := &mockSchedulerRepository{
		nodes: map[string]NodeRecord{
			"node-a": createTestNodeRecord("node-a", "Node A", []string{"claude-code"}, 1, 0),
			"node-b": createTestNodeRecord("node-b", "Node B", []string{"claude-code"}, 1, 0),
		},
		acquireAttempts:  map[string]int{},
		acquireSuccesses: map[string]int{},
	}

	scheduler, err := NewScheduler(repo)
	require.NoError(t, err)

	const numCallers = 5
	var wg sync.WaitGroup
	start := make(chan struct{})
	wins := make(chan string, numCallers)
	errs := make(chan error, numCallers)
	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			node, err := scheduler.SelectNode(context.Background(), "claude-code")
			if err != nil {
				errs <- err
				return
			}
			wins <- node.NodeID
		}()
	}
	close(start)
	wg.Wait()
	close(wins)
	close(errs)

	totalWins := 0
	used := map[string]int{}
	for nodeID := range wins {
		totalWins++
		used[nodeID]++
	}
	for nodeID := range used {
		assert.LessOrEqual(t, used[nodeID], 1, "node %s overbooked", nodeID)
	}
	assert.Equal(t, 2, totalWins, "exactly two slots available across both nodes")
	assert.Equal(t, 3, len(errs), "remaining callers rejected")
	for e := range errs {
		assert.Equal(t, ErrNoAvailableNode, e)
	}
}
