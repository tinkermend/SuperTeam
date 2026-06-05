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
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/storage/queries"
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

func TestRuntimeServiceCreatesEventBestEffortPayload(t *testing.T) {
	repo := newRuntimeOverviewFakeRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	err = service.CreateRuntimeEvent(context.Background(), CreateRuntimeEventRequest{
		TenantID:      DefaultTenantID,
		NodeID:        "node-1",
		EventType:     RuntimeEventEnrollmentApproved,
		Severity:      RuntimeEventSeveritySuccess,
		Source:        RuntimeEventSourceEnrollment,
		Title:         "Runtime 节点接入通过",
		Description:   "node-1 已批准接入",
		CorrelationID: "enrollment-1",
		Payload: map[string]any{
			"bootstrap_key": "secret",
			"safe":          "visible",
		},
	})
	require.NoError(t, err)
	require.Len(t, repo.events, 1)
	assert.Equal(t, RuntimeEventEnrollmentApproved, repo.events[0].EventType)
	assert.Equal(t, "[redacted]", repo.events[0].Payload["bootstrap_key"])
	assert.Equal(t, "visible", repo.events[0].Payload["safe"])
}

func TestRuntimeEnrollmentDecisionsCreateEvents(t *testing.T) {
	repo := newRuntimeOverviewFakeRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	approvedEnrollment := repo.seedRuntimeOverviewPendingEnrollment(t, "runtime-approve")
	rejectedEnrollment := repo.seedRuntimeOverviewPendingEnrollment(t, "runtime-reject")

	_, err = service.ApproveEnrollment(context.Background(), ApproveEnrollmentRequest{
		TenantID:     DefaultTenantID,
		EnrollmentID: approvedEnrollment.ID,
		ApprovedBy:   runtimeTestUUID(901),
	})
	require.NoError(t, err)

	_, err = service.RejectEnrollment(context.Background(), RejectEnrollmentRequest{
		TenantID:     DefaultTenantID,
		EnrollmentID: rejectedEnrollment.ID,
		RejectedBy:   runtimeTestUUID(902),
		Reason:       "missing runtime owner",
	})
	require.NoError(t, err)

	require.Len(t, repo.events, 2)
	assert.Equal(t, RuntimeEventEnrollmentApproved, repo.events[0].EventType)
	assert.Equal(t, RuntimeEventSourceEnrollment, repo.events[0].Source)
	assert.Equal(t, "Runtime 节点接入通过", repo.events[0].Title)
	assert.Equal(t, "runtime_enrollment", repo.events[0].CorrelationType)
	assert.Equal(t, approvedEnrollment.ID.String(), repo.events[0].CorrelationID)
	assert.Equal(t, "approved", repo.events[0].Payload["status"])

	assert.Equal(t, RuntimeEventEnrollmentRejected, repo.events[1].EventType)
	assert.Equal(t, RuntimeEventSourceEnrollment, repo.events[1].Source)
	assert.Equal(t, "Runtime 节点接入被拒绝", repo.events[1].Title)
	assert.Equal(t, "runtime_enrollment", repo.events[1].CorrelationType)
	assert.Equal(t, rejectedEnrollment.ID.String(), repo.events[1].CorrelationID)
	assert.Equal(t, "rejected", repo.events[1].Payload["status"])
	assert.Equal(t, "missing runtime owner", repo.events[1].Payload["reason"])
}

func TestRuntimeServiceBuildsOverview(t *testing.T) {
	repo := newRuntimeOverviewFakeRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	otherTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000099")
	repo.totalNodes = 8
	repo.onlineNodes = 6
	repo.activeProviderSessions = 14
	repo.blockedEvents = 1
	repo.pendingEnrollmentCount = 7
	repo.nodes = []NodeRecord{
		{
			ID:                 uuid.MustParse("00000000-0000-0000-0000-000000000701"),
			TenantID:           DefaultTenantID,
			NodeID:             "prod-runtime-shanghai-01",
			Name:               "prod-runtime-shanghai-01",
			SupportedProviders: []byte(`["claude-code"]`),
			MaxSlots:           10,
			CurrentLoad:        6,
			Status:             "online",
			LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		{
			ID:                 uuid.MustParse("00000000-0000-0000-0000-000000000704"),
			TenantID:           otherTenantID,
			NodeID:             "other-tenant-runtime",
			Name:               "other-tenant-runtime",
			SupportedProviders: []byte(`["codex"]`),
			MaxSlots:           2,
			CurrentLoad:        0,
			Status:             "online",
			LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
	}
	repo.enrollments = make([]RuntimeEnrollmentRecord, 0, 7)
	for i := 0; i < 7; i++ {
		repo.enrollments = append(repo.enrollments, RuntimeEnrollmentRecord{
			ID:        runtimeTestUUID(702 + i),
			TenantID:  DefaultTenantID,
			NodeID:    fmt.Sprintf("customer-vm-east-%02d", i+1),
			Status:    RuntimeEnrollmentStatusPending,
			CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		})
	}
	repo.capabilitySummaries = []RuntimeProviderCapabilitySummary{{
		ProviderType:   "claude-code",
		NodeCount:      1,
		AvailableCount: 1,
		HealthyCount:   1,
	}}
	repo.runtimeEvents = []RuntimeEvent{{
		ID:        uuid.MustParse("00000000-0000-0000-0000-000000000703"),
		TenantID:  DefaultTenantID,
		NodeID:    "prod-runtime-shanghai-01",
		EventType: RuntimeEventCommandCompleted,
		Severity:  RuntimeEventSeveritySuccess,
		Source:    RuntimeEventSourceRuntimeCommand,
		Title:     "Runtime command completed",
		CreatedAt: time.Now(),
	}}

	overview, err := service.GetOverview(context.Background(), RuntimeOverviewFilter{TenantID: DefaultTenantID})
	require.NoError(t, err)
	assert.Equal(t, int64(6), overview.Summary.OnlineNodes)
	assert.Equal(t, int64(8), overview.Summary.TotalNodes)
	assert.Equal(t, int64(14), overview.Summary.ActiveProviderSessions)
	assert.Equal(t, int64(1), overview.Summary.BlockedEvents)
	assert.Equal(t, int64(7), overview.Summary.PendingEnrollments)
	require.Len(t, overview.PendingEnrollments, 5)
	require.Len(t, overview.Nodes, 1)
	assert.Equal(t, DefaultTenantID, repo.lastListNodesForTenantParams.TenantID)
	assert.Equal(t, int32(50), repo.lastListNodesForTenantParams.Limit)
	assert.Equal(t, "prod-runtime-shanghai-01", overview.Nodes[0].NodeID)
	require.Len(t, overview.ProviderCapabilities, 1)
	require.Len(t, overview.RecentEvents, 1)
}

func TestRuntimeServiceRedactsSensitiveEventPayloadWithoutRedactingTokenMetrics(t *testing.T) {
	repo := newRuntimeOverviewFakeRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	payload := map[string]any{
		"input_tokens":  123,
		"output_tokens": 456,
		"total_tokens":  579,
		"token":         "secret-token",
		"access_token":  "secret-access",
		"refresh_token": "secret-refresh",
		"bootstrap_key": "secret-bootstrap",
		"nested": map[string]any{
			"api_key": "secret-api-key",
			"safe":    "visible",
		},
		"items": []any{
			map[string]any{"credentials": "secret-credentials"},
			map[string]any{"total_tokens": 99},
		},
	}

	err = service.CreateRuntimeEvent(context.Background(), CreateRuntimeEventRequest{
		TenantID:  DefaultTenantID,
		EventType: RuntimeEventCommandCompleted,
		Severity:  RuntimeEventSeveritySuccess,
		Source:    RuntimeEventSourceRuntimeCommand,
		Title:     "Runtime command completed",
		Payload:   payload,
	})
	require.NoError(t, err)
	require.Len(t, repo.events, 1)

	redacted := repo.events[0].Payload
	assert.Equal(t, 123, redacted["input_tokens"])
	assert.Equal(t, 456, redacted["output_tokens"])
	assert.Equal(t, 579, redacted["total_tokens"])
	assert.Equal(t, "[redacted]", redacted["token"])
	assert.Equal(t, "[redacted]", redacted["access_token"])
	assert.Equal(t, "[redacted]", redacted["refresh_token"])
	assert.Equal(t, "[redacted]", redacted["bootstrap_key"])
	nested := redacted["nested"].(map[string]any)
	assert.Equal(t, "[redacted]", nested["api_key"])
	assert.Equal(t, "visible", nested["safe"])
	items := redacted["items"].([]any)
	assert.Equal(t, "[redacted]", items[0].(map[string]any)["credentials"])
	assert.Equal(t, 99, items[1].(map[string]any)["total_tokens"])

	assert.Equal(t, "secret-token", payload["token"])
	assert.Equal(t, "secret-api-key", payload["nested"].(map[string]any)["api_key"])
	assert.Equal(t, "secret-credentials", payload["items"].([]any)[0].(map[string]any)["credentials"])
}

func TestRuntimeCapabilityFromQueryPreservesPayloadFields(t *testing.T) {
	providerVersion := pgtype.Text{String: "1.2.3", Valid: true}
	binaryPath := pgtype.Text{String: "/usr/local/bin/claude", Valid: true}
	workspaceBaseDir := pgtype.Text{String: "/workspaces", Valid: true}

	capability := runtimeCapabilityFromQuery(queries.RuntimeCapability{
		ID:               uuid.MustParse("00000000-0000-0000-0000-000000000801"),
		TenantID:         DefaultTenantID,
		RuntimeNodeID:    uuid.MustParse("00000000-0000-0000-0000-000000000802"),
		CapabilityType:   "provider",
		CapabilityKey:    "claude-code:default",
		ProviderType:     "claude-code",
		ProviderVersion:  providerVersion,
		BinaryPath:       binaryPath,
		Available:        true,
		WorkspaceBaseDir: workspaceBaseDir,
		Capacity:         []byte(`{"max_slots":4}`),
		Labels:           []byte(`{"region":"shanghai"}`),
		Status:           "active",
		Details:          []byte(`{"reason":"ok"}`),
		HealthStatus:     "healthy",
		Metadata:         []byte(`{"source":"runtime-agent"}`),
	})

	require.NotNil(t, capability.ProviderVersion)
	assert.Equal(t, "1.2.3", *capability.ProviderVersion)
	require.NotNil(t, capability.BinaryPath)
	assert.Equal(t, "/usr/local/bin/claude", *capability.BinaryPath)
	require.NotNil(t, capability.WorkspaceBaseDir)
	assert.Equal(t, "/workspaces", *capability.WorkspaceBaseDir)
	assert.Equal(t, float64(4), capability.Capacity["max_slots"])
	assert.Equal(t, "shanghai", capability.Labels["region"])
	assert.Equal(t, "ok", capability.Details["reason"])
	assert.Equal(t, "runtime-agent", capability.Metadata["source"])
}

type runtimeOverviewFakeRepository struct {
	MockRepository
	events                       []CreateRuntimeEventParams
	runtimeEvents                []RuntimeEvent
	enrollments                  []RuntimeEnrollmentRecord
	enrollmentsByID              map[uuid.UUID]RuntimeEnrollmentRecord
	nodes                        []NodeRecord
	capabilitySummaries          []RuntimeProviderCapabilitySummary
	lastListNodesForTenantParams ListRuntimeNodesForTenantParams
	totalNodes                   int64
	onlineNodes                  int64
	activeProviderSessions       int64
	blockedEvents                int64
	pendingEnrollmentCount       int64
}

func newRuntimeOverviewFakeRepository() *runtimeOverviewFakeRepository {
	return &runtimeOverviewFakeRepository{
		enrollmentsByID: map[uuid.UUID]RuntimeEnrollmentRecord{},
	}
}

func (r *runtimeOverviewFakeRepository) CreateRuntimeEvent(ctx context.Context, params CreateRuntimeEventParams) (RuntimeEvent, error) {
	r.events = append(r.events, params)
	return RuntimeEvent{
		ID:          uuid.New(),
		TenantID:    params.TenantID,
		NodeID:      params.NodeID,
		EventType:   params.EventType,
		Severity:    params.Severity,
		Source:      params.Source,
		Title:       params.Title,
		Description: params.Description,
		Payload:     params.Payload,
		CreatedAt:   time.Now(),
	}, nil
}

func (r *runtimeOverviewFakeRepository) ListRuntimeEvents(ctx context.Context, params ListRuntimeEventsParams) ([]RuntimeEvent, error) {
	return r.runtimeEvents, nil
}

func (r *runtimeOverviewFakeRepository) CountRuntimeNodesForTenant(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	return r.totalNodes, nil
}

func (r *runtimeOverviewFakeRepository) CountOnlineRuntimeNodesForTenant(ctx context.Context, tenantID uuid.UUID, threshold time.Time) (int64, error) {
	return r.onlineNodes, nil
}

func (r *runtimeOverviewFakeRepository) CountActiveProviderSessionsForTenant(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	return r.activeProviderSessions, nil
}

func (r *runtimeOverviewFakeRepository) CountBlockedRuntimeEventsSince(ctx context.Context, tenantID uuid.UUID, since time.Time) (int64, error) {
	return r.blockedEvents, nil
}

func (r *runtimeOverviewFakeRepository) ListRuntimeProviderCapabilitiesForTenant(ctx context.Context, tenantID uuid.UUID) ([]RuntimeProviderCapabilitySummary, error) {
	return r.capabilitySummaries, nil
}

func (r *runtimeOverviewFakeRepository) ListRuntimeEnrollments(ctx context.Context, params ListRuntimeEnrollmentsParams) ([]RuntimeEnrollmentRecord, error) {
	limit := int(params.Limit)
	if limit <= 0 || limit > len(r.enrollments) {
		limit = len(r.enrollments)
	}
	return r.enrollments[:limit], nil
}

func (r *runtimeOverviewFakeRepository) ListNodes(ctx context.Context, params ListNodesParams) ([]NodeRecord, error) {
	return r.nodes, nil
}

func (r *runtimeOverviewFakeRepository) ListRuntimeNodesForTenant(ctx context.Context, params ListRuntimeNodesForTenantParams) ([]NodeRecord, error) {
	r.lastListNodesForTenantParams = params
	nodes := make([]NodeRecord, 0, len(r.nodes))
	for _, node := range r.nodes {
		if node.TenantID == params.TenantID {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (r *runtimeOverviewFakeRepository) CountRuntimeEnrollmentsForTenant(ctx context.Context, tenantID uuid.UUID, status *RuntimeEnrollmentStatus) (int64, error) {
	return r.pendingEnrollmentCount, nil
}

func (r *runtimeOverviewFakeRepository) seedRuntimeOverviewPendingEnrollment(t *testing.T, nodeID string) RuntimeEnrollmentRecord {
	t.Helper()
	payload, err := json.Marshal(map[string]interface{}{
		"node_id":             nodeID,
		"name":                nodeID,
		"supported_providers": []string{"codex"},
		"max_slots":           int32(2),
		"metadata":            map[string]interface{}{"source": "test"},
	})
	require.NoError(t, err)
	record := RuntimeEnrollmentRecord{
		ID:             runtimeTestUUID(len(r.enrollmentsByID) + 950),
		TenantID:       DefaultTenantID,
		RuntimeNodeID:  uuid.Nil,
		NodeID:         nodeID,
		BootstrapKeyID: runtimeTestUUID(len(r.enrollmentsByID) + 970),
		Status:         RuntimeEnrollmentStatusPending,
		RequestPayload: payload,
		LastHelloAt:    timestamptzFromTime(time.Now()),
		CreatedAt:      timestamptzFromTime(time.Now()),
		UpdatedAt:      timestamptzFromTime(time.Now()),
	}
	r.enrollmentsByID[record.ID] = record
	return record
}

func (r *runtimeOverviewFakeRepository) GetRuntimeEnrollment(ctx context.Context, tenantID, enrollmentID uuid.UUID) (RuntimeEnrollmentRecord, error) {
	record, ok := r.enrollmentsByID[enrollmentID]
	if !ok || record.TenantID != tenantID {
		return RuntimeEnrollmentRecord{}, errors.New("not found")
	}
	return record, nil
}

func (r *runtimeOverviewFakeRepository) ListActiveRuntimeBootstrapKeys(ctx context.Context, tenantID uuid.UUID) ([]RuntimeBootstrapKeyRecord, error) {
	return nil, errors.New("not implemented")
}

func (r *runtimeOverviewFakeRepository) UpsertRuntimeEnrollmentFromHello(ctx context.Context, params UpsertRuntimeEnrollmentFromHelloParams) (RuntimeEnrollmentRecord, error) {
	return RuntimeEnrollmentRecord{}, errors.New("not implemented")
}

func (r *runtimeOverviewFakeRepository) UpsertRuntimeNodeForTenant(ctx context.Context, params UpsertRuntimeNodeForTenantParams) (NodeRecord, error) {
	return NodeRecord{}, errors.New("not implemented")
}

func (r *runtimeOverviewFakeRepository) ApproveRuntimeEnrollment(ctx context.Context, params ApproveRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	return RuntimeEnrollmentRecord{}, errors.New("not implemented")
}

func (r *runtimeOverviewFakeRepository) ApproveRuntimeEnrollmentWithNode(ctx context.Context, params ApproveRuntimeEnrollmentWithNodeParams) (RuntimeEnrollmentRecord, error) {
	record, ok := r.enrollmentsByID[params.EnrollmentID]
	if !ok || record.TenantID != params.TenantID || record.Status != RuntimeEnrollmentStatusPending {
		return RuntimeEnrollmentRecord{}, errors.New("not found")
	}
	record.Status = RuntimeEnrollmentStatusApproved
	record.RuntimeNodeID = runtimeTestUUID(len(r.nodes) + 980)
	record.ApprovedBy = uuid.NullUUID{UUID: params.ApprovedBy, Valid: params.ApprovedBy != uuid.Nil}
	record.ApprovedAt = timestamptzFromTime(time.Now())
	record.UpdatedAt = timestamptzFromTime(time.Now())
	r.enrollmentsByID[record.ID] = record
	return record, nil
}

func (r *runtimeOverviewFakeRepository) RevokeRuntimeEnrollment(ctx context.Context, params RevokeRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	return RuntimeEnrollmentRecord{}, errors.New("not implemented")
}

func (r *runtimeOverviewFakeRepository) CreateRuntimeSession(ctx context.Context, params CreateRuntimeSessionParams) (RuntimeSessionRecord, error) {
	return RuntimeSessionRecord{}, errors.New("not implemented")
}

func (r *runtimeOverviewFakeRepository) GetActiveRuntimeSessionByLookupHash(ctx context.Context, params GetActiveRuntimeSessionByLookupHashParams) (RuntimeSessionRecord, error) {
	return RuntimeSessionRecord{}, errors.New("not implemented")
}

func (r *runtimeOverviewFakeRepository) RenewRuntimeSession(ctx context.Context, params RenewRuntimeSessionParams) (RuntimeSessionRecord, error) {
	return RuntimeSessionRecord{}, errors.New("not implemented")
}

func (r *runtimeOverviewFakeRepository) TouchRuntimeSession(ctx context.Context, params TouchRuntimeSessionParams) (RuntimeSessionRecord, error) {
	return RuntimeSessionRecord{}, errors.New("not implemented")
}

func (r *runtimeOverviewFakeRepository) RejectRuntimeEnrollment(ctx context.Context, params RejectRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	record, ok := r.enrollmentsByID[params.EnrollmentID]
	if !ok || record.TenantID != params.TenantID || record.Status != RuntimeEnrollmentStatusPending {
		return RuntimeEnrollmentRecord{}, errors.New("not found")
	}
	record.Status = RuntimeEnrollmentStatusRejected
	record.RejectedBy = uuid.NullUUID{UUID: params.RejectedBy, Valid: params.RejectedBy != uuid.Nil}
	record.RejectedAt = timestamptzFromTime(time.Now())
	record.RejectReason = textFromString(&params.Reason)
	record.UpdatedAt = timestamptzFromTime(time.Now())
	r.enrollmentsByID[record.ID] = record
	return record, nil
}
