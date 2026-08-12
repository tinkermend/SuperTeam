package project

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

// recordingWorkspaceCommander is a test double for RuntimeWorkspaceCommander.
type recordingWorkspaceCommander struct {
	connected       bool
	nodes           map[uuid.UUID]struct{ tenantID uuid.UUID; nodeID string }
	dispatchedTypes []string
}

func (c *recordingWorkspaceCommander) GetNodeByID(_ context.Context, id uuid.UUID) (runtimepkg.NodeRecord, error) {
	if c.nodes != nil {
		if n, ok := c.nodes[id]; ok {
			return runtimepkg.NodeRecord{ID: id, TenantID: n.tenantID, NodeID: n.nodeID, Name: n.nodeID}, nil
		}
	}
	return runtimepkg.NodeRecord{ID: id, NodeID: id.String(), Name: id.String()}, nil
}

func (c *recordingWorkspaceCommander) IsConnected(nodeID string) bool {
	return c.connected
}

func (c *recordingWorkspaceCommander) Dispatch(_ context.Context, nodeID string, command runtimepkg.RuntimeCommand) error {
	c.dispatchedTypes = append(c.dispatchedTypes, command.Type)
	return nil
}

func (c *recordingWorkspaceCommander) CreateCommandReceipt(_ context.Context, _ WorkspaceCommandReceiptRequest) error {
	return nil
}

func (c *recordingWorkspaceCommander) WaitForCommandCompletion(_ context.Context, _ uuid.UUID, _ string, _ time.Duration) (*WorkspaceCommandReceipt, error) {
	return &WorkspaceCommandReceipt{Status: "completed"}, nil
}

func TestDeleteProjectEnqueuesWorkspaceDeleteWithoutDiskRemove(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	nodeID := uuid.New()
	workflowID := "project-coordinator:" + projectID.String()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "待删项目",
		DirectoryName:          "to-delete-dir",
		Goal:                   "清理",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: workflowID,
	}
	repo.projectRuntimeNodes = map[uuid.UUID][]ProjectRuntimeNode{
		projectID: {{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ProjectID:     projectID,
			RuntimeNodeID: nodeID,
			CreatedAt:     time.Now().UTC(),
		}},
	}

	err = service.DeleteProject(context.Background(), DeleteProjectRequest{
		TenantID: tenantID, ProjectID: projectID, ActorUserID: actorID,
	})
	require.NoError(t, err)
	require.NotNil(t, repo.projects[projectID].DeletedAt)
	require.Len(t, repo.workspaceDeleteRequests, 1)
	req := repo.workspaceDeleteRequests[0]
	require.Equal(t, projectID, req.ProjectID)
	require.Equal(t, nodeID, req.RuntimeNodeID)
	require.Equal(t, "to-delete-dir", req.DirectoryName)
	require.Equal(t, WorkspaceDeleteRequestStatusPending, req.Status)
	require.Equal(t, "project_delete", *req.Reason)
}

// Deleting a project must queue disk cleanup only for nodes that were actually
// supplied — an unprovisioned bind never had a directory, so minting an admin
// confirmation card for it is pure noise (same rule as RemoveProjectRuntimeNode).
func TestDeleteProjectEnqueuesOnlyProvisionedNodes(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	provisionedNode := uuid.New()
	boundOnlyNode := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "混合供给项目",
		DirectoryName:          "mixed-provision-dir",
		Goal:                   "清理",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.projectRuntimeNodes = map[uuid.UUID][]ProjectRuntimeNode{
		projectID: {
			{
				ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: provisionedNode,
				ProvisionStatus: ProvisionStatusProvisioned,
			},
			{
				ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: boundOnlyNode,
				ProvisionStatus: ProvisionStatusUnprovisioned,
			},
		},
	}

	err = service.DeleteProject(context.Background(), DeleteProjectRequest{
		TenantID: tenantID, ProjectID: projectID, ActorUserID: actorID,
	})
	require.NoError(t, err)
	require.Len(t, repo.workspaceDeleteRequests, 1)
	require.Equal(t, provisionedNode, repo.workspaceDeleteRequests[0].RuntimeNodeID)
}

func TestRemoveProjectRuntimeNodeEnqueuesWorkspaceDelete(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	nodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "摘节点项目",
		DirectoryName:    "detach-dir",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	repo.projectRuntimeNodes = map[uuid.UUID][]ProjectRuntimeNode{
		projectID: {{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeID,
			ProvisionStatus: ProvisionStatusProvisioned,
		}},
	}

	err = service.RemoveProjectRuntimeNode(context.Background(), ModifyProjectRuntimeNodeRequest{
		TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeID, ActorUserID: actorID, Reason: "ops",
	})
	require.NoError(t, err)
	require.Len(t, repo.workspaceDeleteRequests, 1)
	require.Equal(t, "detach-dir", repo.workspaceDeleteRequests[0].DirectoryName)
	require.Equal(t, nodeID, repo.workspaceDeleteRequests[0].RuntimeNodeID)
}

func TestRemoveUnprovisionedRuntimeNodeSkipsDeleteQueue(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	nodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "bind-only", DirectoryName: "bind-only-dir",
		Status: ProjectStatusRunning, HumanOwnerUserID: actorID,
	}
	repo.projectRuntimeNodes = map[uuid.UUID][]ProjectRuntimeNode{
		projectID: {{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeID,
			ProvisionStatus: ProvisionStatusUnprovisioned,
		}},
	}
	err = service.RemoveProjectRuntimeNode(context.Background(), ModifyProjectRuntimeNodeRequest{
		TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeID, ActorUserID: actorID,
	})
	require.NoError(t, err)
	require.Empty(t, repo.workspaceDeleteRequests)
}

func TestCreateProjectRejectsDirectoryNameInDeleteQueue(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	require.NoError(t, err)

	dirName := "occupied-dir"
	repo.workspaceDeleteRequests = []WorkspaceDeleteRequest{{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ProjectID:      uuid.New(),
		RuntimeNodeID:  uuid.New(),
		DirectoryName:  dirName,
		NodeIDSnapshot: "node-a",
		Ownership:      WorkspaceOwnershipPlatformManaged,
		Status:         WorkspaceDeleteRequestStatusPending,
		RequestedBy:    uuid.New(),
		RequestedAt:    time.Now().UTC(),
	}}

	// Use a minimal create path that hits directory validation.
	// validateRuntimeNodeIDs will fail if we pass nodes; pass empty and expect
	// either directory pending error first or runtime nodes required.
	// Directory check runs before runtime node validation.
	tenantID := uuid.New()
	actorID := uuid.New()
	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:          tenantID,
		ActorUserID:       actorID,
		Name:              "新项目",
		DirectoryName:     dirName,
		Goal:              "goal",
		HumanOwnerUserIDs: []uuid.UUID{actorID},
		Members: []ProjectMemberInput{{
			PrincipalType: PrincipalTypeHumanUser,
			PrincipalID:   actorID,
			ProjectRole:   ProjectRoleOwner,
		}},
	})
	require.ErrorIs(t, err, ErrProjectDirectoryNamePendingDelete)
}

func TestConfirmWorkspaceDeleteDispatchesRemoveAndMarksConfirmed(t *testing.T) {
	repo := newMemoryRepository()
	commander := &recordingWorkspaceCommander{connected: true}
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	require.NoError(t, err)
	service.SetRuntimeWorkspaceCommander(commander)

	tenantID := uuid.New()
	actorID := uuid.New()
	requestID := uuid.New()
	projectID := uuid.New()
	nodeUUID := uuid.New()
	repo.workspaceDeleteRequests = []WorkspaceDeleteRequest{{
		ID:             requestID,
		TenantID:       tenantID,
		ProjectID:      projectID,
		RuntimeNodeID:  nodeUUID,
		DirectoryName:  "confirm-dir",
		NodeIDSnapshot: "node-confirm",
		Ownership:      WorkspaceOwnershipPlatformManaged,
		Status:         WorkspaceDeleteRequestStatusPending,
		RequestedBy:    actorID,
		RequestedAt:    time.Now().UTC(),
	}}
	commander.nodes = map[uuid.UUID]struct {
		tenantID uuid.UUID
		nodeID   string
	}{
		nodeUUID: {tenantID: tenantID, nodeID: "node-confirm"},
	}

	got, err := service.ConfirmWorkspaceDelete(context.Background(), tenantID, requestID, actorID)
	require.NoError(t, err)
	require.Equal(t, WorkspaceDeleteRequestStatusConfirmed, got.Status)
	require.Contains(t, commander.dispatchedTypes, runtimeCommandRemoveProjectDirectory)
}

func TestRejectWorkspaceDeleteHandsOff(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	require.NoError(t, err)

	tenantID := uuid.New()
	actorID := uuid.New()
	requestID := uuid.New()
	repo.workspaceDeleteRequests = []WorkspaceDeleteRequest{{
		ID:             requestID,
		TenantID:       tenantID,
		ProjectID:      uuid.New(),
		RuntimeNodeID:  uuid.New(),
		DirectoryName:  "keep-dir",
		NodeIDSnapshot: "node-keep",
		Ownership:      WorkspaceOwnershipPlatformManaged,
		Status:         WorkspaceDeleteRequestStatusPending,
		RequestedBy:    actorID,
		RequestedAt:    time.Now().UTC(),
	}}

	got, err := service.RejectWorkspaceDelete(context.Background(), tenantID, requestID, actorID, "运维保留")
	require.NoError(t, err)
	require.Equal(t, WorkspaceDeleteRequestStatusRejected, got.Status)
	require.Equal(t, "运维保留", *got.Reason)
	require.Len(t, repo.workspaceDeleteAuditEvents, 1)
	require.Equal(t, "project.workspace_delete.rejected", repo.workspaceDeleteAuditEvents[0]["action"])
}
