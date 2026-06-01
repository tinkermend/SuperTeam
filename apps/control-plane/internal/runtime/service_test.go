package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func runtimeTestUUID(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", n))
}

// MockRepository is a mock implementation of Repository
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) CreateNode(ctx context.Context, params CreateNodeParams) (NodeRecord, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(NodeRecord), args.Error(1)
}

func (m *MockRepository) GetNode(ctx context.Context, nodeID string) (NodeRecord, error) {
	args := m.Called(ctx, nodeID)
	if args.Get(0) == nil {
		return NodeRecord{}, args.Error(1)
	}
	return args.Get(0).(NodeRecord), args.Error(1)
}

func (m *MockRepository) ListNodes(ctx context.Context, params ListNodesParams) ([]NodeRecord, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]NodeRecord), args.Error(1)
}

func (m *MockRepository) ListOnlineNodes(ctx context.Context, heartbeatThreshold pgtype.Timestamptz) ([]NodeRecord, error) {
	args := m.Called(ctx, heartbeatThreshold)
	return args.Get(0).([]NodeRecord), args.Error(1)
}

func (m *MockRepository) UpdateHeartbeat(ctx context.Context, params UpdateHeartbeatParams) (NodeRecord, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(NodeRecord), args.Error(1)
}

func (m *MockRepository) UpdateLoad(ctx context.Context, params UpdateLoadParams) (NodeRecord, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(NodeRecord), args.Error(1)
}

func (m *MockRepository) UpdateStatus(ctx context.Context, params UpdateStatusParams) (NodeRecord, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(NodeRecord), args.Error(1)
}

func (m *MockRepository) DeleteNode(ctx context.Context, nodeID string) error {
	args := m.Called(ctx, nodeID)
	return args.Error(0)
}

func TestNewService(t *testing.T) {
	t.Run("requires repository", func(t *testing.T) {
		_, err := NewService(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "repository is required")
	})

	t.Run("accepts valid repository", func(t *testing.T) {
		repo := new(MockRepository)
		service, err := NewService(repo)
		assert.NoError(t, err)
		assert.NotNil(t, service)
	})
}

func TestRegisterNode(t *testing.T) {
	ctx := context.Background()

	t.Run("creates new node", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		req := RegisterNodeRequest{
			NodeID:             "node-001",
			Name:               "Test Node",
			SupportedProviders: []string{"claude-code", "opencode"},
			MaxSlots:           4,
			Metadata:           map[string]interface{}{"region": "us-west"},
		}

		providersJSON, _ := json.Marshal(req.SupportedProviders)
		metadataJSON, _ := json.Marshal(req.Metadata)

		// Mock GetNode to return error (node doesn't exist)
		repo.On("GetNode", ctx, req.NodeID).Return(nil, errors.New("not found"))

		// Mock CreateNode
		expectedRecord := NodeRecord{
			ID:                 runtimeTestUUID(1),
			NodeID:             req.NodeID,
			Name:               req.Name,
			SupportedProviders: providersJSON,
			MaxSlots:           req.MaxSlots,
			CurrentLoad:        0,
			Status:             string(NodeStatusOnline),
			Metadata:           metadataJSON,
			LastHeartbeatAt:    timestamptzFromTime(time.Now()),
			CreatedAt:          timestamptzFromTime(time.Now()),
			UpdatedAt:          timestamptzFromTime(time.Now()),
		}
		repo.On("CreateNode", ctx, mock.AnythingOfType("CreateNodeParams")).Return(expectedRecord, nil)

		node, err := service.RegisterNode(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, node)
		assert.Equal(t, req.NodeID, node.NodeID)
		assert.Equal(t, req.Name, node.Name)
		assert.Equal(t, req.SupportedProviders, node.SupportedProviders)
		assert.Equal(t, req.MaxSlots, node.MaxSlots)
		assert.Equal(t, NodeStatusOnline, node.Status)

		repo.AssertExpectations(t)
	})

	t.Run("updates existing node", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		req := RegisterNodeRequest{
			NodeID:             "node-001",
			Name:               "Test Node",
			SupportedProviders: []string{"claude-code"},
			MaxSlots:           4,
		}

		providersJSON, _ := json.Marshal(req.SupportedProviders)
		existingRecord := NodeRecord{
			ID:                 runtimeTestUUID(1),
			NodeID:             req.NodeID,
			Name:               req.Name,
			SupportedProviders: providersJSON,
			MaxSlots:           req.MaxSlots,
			CurrentLoad:        2,
			Status:             string(NodeStatusOffline),
			Metadata:           []byte("{}"),
			LastHeartbeatAt:    timestamptzFromTime(time.Now().Add(-2 * time.Minute)),
			CreatedAt:          timestamptzFromTime(time.Now().Add(-1 * time.Hour)),
			UpdatedAt:          timestamptzFromTime(time.Now().Add(-2 * time.Minute)),
		}

		// Mock GetNode to return existing node
		repo.On("GetNode", ctx, req.NodeID).Return(existingRecord, nil)

		// Mock UpdateHeartbeat
		updatedRecord := existingRecord
		updatedRecord.LastHeartbeatAt = timestamptzFromTime(time.Now())
		repo.On("UpdateHeartbeat", ctx, mock.AnythingOfType("UpdateHeartbeatParams")).Return(updatedRecord, nil)

		// Mock UpdateStatus
		updatedRecord.Status = string(NodeStatusOnline)
		repo.On("UpdateStatus", ctx, mock.AnythingOfType("UpdateStatusParams")).Return(updatedRecord, nil)

		node, err := service.RegisterNode(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, node)
		assert.Equal(t, NodeStatusOnline, node.Status)

		repo.AssertExpectations(t)
	})

	t.Run("validates required fields", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		tests := []struct {
			name    string
			req     RegisterNodeRequest
			wantErr string
		}{
			{
				name:    "missing node_id",
				req:     RegisterNodeRequest{Name: "Test", SupportedProviders: []string{"claude-code"}, MaxSlots: 4},
				wantErr: "node_id is required",
			},
			{
				name:    "missing name",
				req:     RegisterNodeRequest{NodeID: "node-001", SupportedProviders: []string{"claude-code"}, MaxSlots: 4},
				wantErr: "name is required",
			},
			{
				name:    "missing supported_providers",
				req:     RegisterNodeRequest{NodeID: "node-001", Name: "Test", MaxSlots: 4},
				wantErr: "supported_providers is required",
			},
			{
				name:    "invalid max_slots",
				req:     RegisterNodeRequest{NodeID: "node-001", Name: "Test", SupportedProviders: []string{"claude-code"}, MaxSlots: 0},
				wantErr: "max_slots must be greater than 0",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := service.RegisterNode(ctx, tt.req)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			})
		}
	})
}

func TestUpdateHeartbeat(t *testing.T) {
	ctx := context.Background()

	t.Run("updates heartbeat and load", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		req := UpdateHeartbeatRequest{
			NodeID:      "node-001",
			CurrentLoad: 2,
		}

		providersJSON, _ := json.Marshal([]string{"claude-code"})
		record := NodeRecord{
			ID:                 runtimeTestUUID(1),
			NodeID:             req.NodeID,
			Name:               "Test Node",
			SupportedProviders: providersJSON,
			MaxSlots:           4,
			CurrentLoad:        req.CurrentLoad,
			Status:             string(NodeStatusOnline),
			Metadata:           []byte("{}"),
			LastHeartbeatAt:    timestamptzFromTime(time.Now()),
			CreatedAt:          timestamptzFromTime(time.Now()),
			UpdatedAt:          timestamptzFromTime(time.Now()),
		}

		repo.On("UpdateHeartbeat", ctx, mock.AnythingOfType("UpdateHeartbeatParams")).Return(record, nil)
		repo.On("UpdateLoad", ctx, mock.AnythingOfType("UpdateLoadParams")).Return(record, nil)

		node, err := service.UpdateHeartbeat(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, node)
		assert.Equal(t, req.NodeID, node.NodeID)
		assert.Equal(t, req.CurrentLoad, node.CurrentLoad)

		repo.AssertExpectations(t)
	})

	t.Run("validates required fields", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		_, err := service.UpdateHeartbeat(ctx, UpdateHeartbeatRequest{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "node_id is required")

		_, err = service.UpdateHeartbeat(ctx, UpdateHeartbeatRequest{NodeID: "node-001", CurrentLoad: -1})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "current_load must be non-negative")
	})
}

func TestGetNode(t *testing.T) {
	ctx := context.Background()

	t.Run("retrieves node by ID", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		nodeID := "node-001"
		providersJSON, _ := json.Marshal([]string{"claude-code"})
		record := NodeRecord{
			ID:                 runtimeTestUUID(1),
			NodeID:             nodeID,
			Name:               "Test Node",
			SupportedProviders: providersJSON,
			MaxSlots:           4,
			CurrentLoad:        2,
			Status:             string(NodeStatusOnline),
			Metadata:           []byte("{}"),
			LastHeartbeatAt:    timestamptzFromTime(time.Now()),
			CreatedAt:          timestamptzFromTime(time.Now()),
			UpdatedAt:          timestamptzFromTime(time.Now()),
		}

		repo.On("GetNode", ctx, nodeID).Return(record, nil)

		node, err := service.GetNode(ctx, nodeID)
		assert.NoError(t, err)
		assert.NotNil(t, node)
		assert.Equal(t, nodeID, node.NodeID)

		repo.AssertExpectations(t)
	})

	t.Run("validates node_id", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		_, err := service.GetNode(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "node_id is required")
	})
}

func TestListNodes(t *testing.T) {
	ctx := context.Background()

	t.Run("lists all nodes", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		providersJSON, _ := json.Marshal([]string{"claude-code"})
		records := []NodeRecord{
			{
				ID:                 runtimeTestUUID(1),
				NodeID:             "node-001",
				Name:               "Node 1",
				SupportedProviders: providersJSON,
				MaxSlots:           4,
				CurrentLoad:        2,
				Status:             string(NodeStatusOnline),
				Metadata:           []byte("{}"),
				LastHeartbeatAt:    timestamptzFromTime(time.Now()),
				CreatedAt:          timestamptzFromTime(time.Now()),
				UpdatedAt:          timestamptzFromTime(time.Now()),
			},
			{
				ID:                 runtimeTestUUID(2),
				NodeID:             "node-002",
				Name:               "Node 2",
				SupportedProviders: providersJSON,
				MaxSlots:           4,
				CurrentLoad:        1,
				Status:             string(NodeStatusOnline),
				Metadata:           []byte("{}"),
				LastHeartbeatAt:    timestamptzFromTime(time.Now()),
				CreatedAt:          timestamptzFromTime(time.Now()),
				UpdatedAt:          timestamptzFromTime(time.Now()),
			},
		}

		repo.On("ListNodes", ctx, mock.AnythingOfType("ListNodesParams")).Return(records, nil)

		nodes, err := service.ListNodes(ctx, ListNodesFilter{})
		assert.NoError(t, err)
		assert.Len(t, nodes, 2)

		repo.AssertExpectations(t)
	})

	t.Run("applies default limit", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		repo.On("ListNodes", ctx, mock.MatchedBy(func(params ListNodesParams) bool {
			return params.Limit == 50
		})).Return([]NodeRecord{}, nil)

		_, err := service.ListNodes(ctx, ListNodesFilter{})
		assert.NoError(t, err)

		repo.AssertExpectations(t)
	})

	t.Run("enforces max limit", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		repo.On("ListNodes", ctx, mock.MatchedBy(func(params ListNodesParams) bool {
			return params.Limit == 100
		})).Return([]NodeRecord{}, nil)

		_, err := service.ListNodes(ctx, ListNodesFilter{Limit: 200})
		assert.NoError(t, err)

		repo.AssertExpectations(t)
	})
}

func TestListOnlineNodes(t *testing.T) {
	ctx := context.Background()

	t.Run("lists online nodes", func(t *testing.T) {
		repo := new(MockRepository)
		service, _ := NewService(repo)

		providersJSON, _ := json.Marshal([]string{"claude-code"})
		records := []NodeRecord{
			{
				ID:                 runtimeTestUUID(1),
				NodeID:             "node-001",
				Name:               "Node 1",
				SupportedProviders: providersJSON,
				MaxSlots:           4,
				CurrentLoad:        2,
				Status:             string(NodeStatusOnline),
				Metadata:           []byte("{}"),
				LastHeartbeatAt:    timestamptzFromTime(time.Now()),
				CreatedAt:          timestamptzFromTime(time.Now()),
				UpdatedAt:          timestamptzFromTime(time.Now()),
			},
		}

		repo.On("ListOnlineNodes", ctx, mock.AnythingOfType("pgtype.Timestamptz")).Return(records, nil)

		nodes, err := service.ListOnlineNodes(ctx)
		assert.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, "node-001", nodes[0].NodeID)

		repo.AssertExpectations(t)
	})
}
