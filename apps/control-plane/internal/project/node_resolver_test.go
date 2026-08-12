package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

// pinnedAttemptRepository overrides GetCurrentProjectTaskAttempt so pinned-node
// scenarios can be exercised without building a full task+attempt graph.
type pinnedAttemptRepository struct {
	*memoryRepository
	attempt    ProjectTaskAttempt
	attemptErr error
}

func (r *pinnedAttemptRepository) GetCurrentProjectTaskAttempt(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTaskAttempt, error) {
	if r.attemptErr != nil {
		return ProjectTaskAttempt{}, r.attemptErr
	}
	return r.attempt, nil
}

func onlineNode(id, tenantID uuid.UUID, nodeID string, currentLoad, maxSlots int32) runtimepkg.NodeRecord {
	return runtimepkg.NodeRecord{
		ID:          id,
		TenantID:    tenantID,
		NodeID:      nodeID,
		Status:      string(runtimepkg.NodeStatusOnline),
		CurrentLoad: currentLoad,
		MaxSlots:    maxSlots,
	}
}

func offlineNode(id, tenantID uuid.UUID, nodeID string) runtimepkg.NodeRecord {
	node := onlineNode(id, tenantID, nodeID, 0, 4)
	node.Status = "offline"
	return node
}

func newResolverService(t *testing.T, repo Repository, nodes ...runtimepkg.NodeRecord) *Service {
	t.Helper()
	service, err := NewService(repo)
	require.NoError(t, err)
	service.SetProjectRuntimeNodeReader(&fakeProjectRuntimeNodeReader{nodes: nodes})
	return service
}

func seedEligibility(t *testing.T, repo *memoryRepository, tenantID, projectID uuid.UUID, nodeIDs ...uuid.UUID) {
	t.Helper()
	repo.projects[projectID] = Project{
		ID:                   projectID,
		TenantID:             tenantID,
		Name:                 "resolver-test-project",
		Status:               ProjectStatusRunning,
		WorkspaceReadyStatus: WorkspaceReadyStatusReady,
	}
	for _, id := range nodeIDs {
		_, err := repo.InsertProjectRuntimeNode(context.Background(), tenantID, projectID, id, true, "create")
		require.NoError(t, err)
	}
}

func TestResolveNode_NewTaskPrefersAffinity(t *testing.T) {
	ctx := context.Background()
	tenantID, projectID, employeeID := uuid.New(), uuid.New(), uuid.New()
	nodeA, nodeB := uuid.New(), uuid.New()

	repo := newMemoryRepository()
	seedEligibility(t, repo, tenantID, projectID, nodeA, nodeB)
	_, err := repo.UpsertProjectEmployeeNodeAffinity(ctx, tenantID, projectID, employeeID, nodeB)
	require.NoError(t, err)

	service := newResolverService(t, repo,
		onlineNode(nodeA, tenantID, "node-a", 0, 4),
		onlineNode(nodeB, tenantID, "node-b", 2, 4),
	)

	resolution, err := service.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
	})
	require.NoError(t, err)
	require.Equal(t, nodeB, resolution.NodeID)
	require.False(t, resolution.Pinned)
	require.False(t, resolution.Paused)
	require.Empty(t, resolution.Reason)
}

func TestResolveNode_NewTaskAffinityOfflineFallsBack(t *testing.T) {
	ctx := context.Background()
	tenantID, projectID, employeeID := uuid.New(), uuid.New(), uuid.New()
	nodeA, nodeB := uuid.New(), uuid.New()

	repo := newMemoryRepository()
	seedEligibility(t, repo, tenantID, projectID, nodeA, nodeB)
	_, err := repo.UpsertProjectEmployeeNodeAffinity(ctx, tenantID, projectID, employeeID, nodeB)
	require.NoError(t, err)

	service := newResolverService(t, repo,
		onlineNode(nodeA, tenantID, "node-a", 0, 4),
		offlineNode(nodeB, tenantID, "node-b"),
	)

	resolution, err := service.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
	})
	require.NoError(t, err)
	require.Equal(t, nodeA, resolution.NodeID)
	require.False(t, resolution.Pinned)

	// Affinity is rewritten to the new node (cross-task switch is allowed).
	affinity, err := repo.GetProjectEmployeeNodeAffinity(ctx, tenantID, projectID, employeeID)
	require.NoError(t, err)
	require.Equal(t, nodeA, affinity.RuntimeNodeID)
}

func TestResolveNode_PinnedTaskReusesNode(t *testing.T) {
	ctx := context.Background()
	tenantID, projectID, employeeID, taskID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	nodeA, nodeB := uuid.New(), uuid.New()

	mem := newMemoryRepository()
	seedEligibility(t, mem, tenantID, projectID, nodeA, nodeB)
	repo := &pinnedAttemptRepository{memoryRepository: mem, attempt: ProjectTaskAttempt{RuntimeNodeID: &nodeA}}

	service := newResolverService(t, repo,
		onlineNode(nodeA, tenantID, "node-a", 3, 4),
		onlineNode(nodeB, tenantID, "node-b", 0, 4),
	)

	resolution, err := service.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
		ProjectTaskID:     taskID,
	})
	require.NoError(t, err)
	require.Equal(t, nodeA, resolution.NodeID)
	require.True(t, resolution.Pinned)

	// Pinned reuse never reads or writes affinity.
	_, err = mem.GetProjectEmployeeNodeAffinity(ctx, tenantID, projectID, employeeID)
	require.ErrorIs(t, err, ErrProjectNotFound)
}

func TestResolveNode_PinnedNodeOfflinePauses(t *testing.T) {
	ctx := context.Background()
	tenantID, projectID, employeeID, taskID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	nodeA, nodeB := uuid.New(), uuid.New()

	mem := newMemoryRepository()
	seedEligibility(t, mem, tenantID, projectID, nodeA, nodeB)
	repo := &pinnedAttemptRepository{memoryRepository: mem, attempt: ProjectTaskAttempt{RuntimeNodeID: &nodeA}}

	service := newResolverService(t, repo,
		offlineNode(nodeA, tenantID, "node-a"),
		onlineNode(nodeB, tenantID, "node-b", 0, 4),
	)

	resolution, err := service.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
		ProjectTaskID:     taskID,
	})
	require.NoError(t, err)
	require.True(t, resolution.Paused)
	require.Equal(t, NodeResolutionReasonPinnedNodeOffline, resolution.Reason)
	require.False(t, resolution.Pinned)
	// Critically: it must NOT switch to the other online eligible node.
	require.Equal(t, uuid.Nil, resolution.NodeID)
	require.NotEqual(t, nodeB, resolution.NodeID)
}

func TestResolveNode_NoEligibleOnlineBlocks(t *testing.T) {
	ctx := context.Background()
	tenantID, projectID, employeeID := uuid.New(), uuid.New(), uuid.New()
	nodeA, nodeB := uuid.New(), uuid.New()

	repo := newMemoryRepository()
	seedEligibility(t, repo, tenantID, projectID, nodeA, nodeB)

	service := newResolverService(t, repo,
		offlineNode(nodeA, tenantID, "node-a"),
		offlineNode(nodeB, tenantID, "node-b"),
	)

	resolution, err := service.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
	})
	require.NoError(t, err)
	require.Equal(t, NodeResolutionReasonNoEligibleOnlineNode, resolution.Reason)
	require.Equal(t, uuid.Nil, resolution.NodeID)
	require.False(t, resolution.Paused)
}

// 绑定 ≠ 供给：未供给的节点只是候选资格，不能被派发选中(spec 2026-08-12 §5.2)。
// 这条不成立时，漂移会静默落到一个没有工作区的节点上。
func TestResolveNode_SkipsUnprovisionedBindings(t *testing.T) {
	ctx := context.Background()
	tenantID, projectID, employeeID := uuid.New(), uuid.New(), uuid.New()
	supplied, boundOnly := uuid.New(), uuid.New()

	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:                   projectID,
		TenantID:             tenantID,
		Name:                 "resolver-provision-project",
		Status:               ProjectStatusRunning,
		WorkspaceReadyStatus: WorkspaceReadyStatusReady,
	}
	_, err := repo.InsertProjectRuntimeNode(ctx, tenantID, projectID, supplied, true, "create")
	require.NoError(t, err)
	_, err = repo.InsertProjectRuntimeNode(ctx, tenantID, projectID, boundOnly, false, "")
	require.NoError(t, err)

	// 未供给节点负载更低：只有供给状态能解释「为什么没选它」。
	service := newResolverService(t, repo,
		onlineNode(supplied, tenantID, "node-supplied", 3, 4),
		onlineNode(boundOnly, tenantID, "node-bound-only", 0, 4),
	)

	resolution, err := service.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
	})
	require.NoError(t, err)
	require.Equal(t, supplied, resolution.NodeID)
}

// 全部节点未供给 → 明确的「工作区不可用」，而不是静默挑一个。
func TestResolveNode_AllUnprovisionedReportsWorkspaceUnavailable(t *testing.T) {
	ctx := context.Background()
	tenantID, projectID, employeeID := uuid.New(), uuid.New(), uuid.New()
	boundOnly := uuid.New()

	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:                   projectID,
		TenantID:             tenantID,
		Name:                 "resolver-unprovisioned-project",
		Status:               ProjectStatusRunning,
		WorkspaceReadyStatus: WorkspaceReadyStatusReady,
	}
	_, err := repo.InsertProjectRuntimeNode(ctx, tenantID, projectID, boundOnly, false, "")
	require.NoError(t, err)

	service := newResolverService(t, repo, onlineNode(boundOnly, tenantID, "node-bound-only", 0, 4))

	resolution, err := service.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
	})
	require.NoError(t, err)
	require.Equal(t, uuid.Nil, resolution.NodeID)
	require.Equal(t, NodeResolutionReasonWorkspaceUnavailable, resolution.Reason)
}
