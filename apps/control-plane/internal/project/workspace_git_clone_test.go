package project

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var errListReceiptsBoom = errors.New("receipt list unavailable")

func TestHandleCloneCommandTerminalReadyOnFirstSuccess(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	ownerID := uuid.New()
	nodeID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, nodeID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "git-proj",
		Goal:             "clone ready",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{nodeID},
		RepoBinding: &ProjectRepoBindingInput{
			URL:           "https://example.com/acme/app.git",
			DefaultBranch: "main",
		},
	})
	require.NoError(t, err)
	require.Equal(t, WorkspaceReadyStatusPending, created.Project.WorkspaceReadyStatus)

	err = service.HandleCloneCommandTerminal(context.Background(), CloneCommandTerminal{
		TenantID:      tenantID,
		ProjectID:     created.Project.ID,
		RuntimeNodeID: nodeID,
		Success:       true,
		Payload:       map[string]any{workspaceCloneAttemptPayloadKey: "attempt-1"},
	})
	require.NoError(t, err)
	got, err := service.GetProject(context.Background(), tenantID, created.Project.ID)
	require.NoError(t, err)
	require.Equal(t, WorkspaceReadyStatusReady, got.WorkspaceReadyStatus)
	require.NotNil(t, got.PrimaryRuntimeNodeID)
	require.Equal(t, nodeID, *got.PrimaryRuntimeNodeID)
}

func TestHandleCloneCommandTerminalErrorWhenAllFailed(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	lister := &memoryWorkspaceReceiptLister{}
	service.SetProjectWorkspaceReceiptLister(lister)

	tenantID := uuid.New()
	ownerID := uuid.New()
	nodeID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, nodeID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "git-fail",
		Goal:             "clone fail",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{nodeID},
		RepoBinding: &ProjectRepoBindingInput{
			URL:           "https://example.com/acme/app.git",
			DefaultBranch: "main",
		},
	})
	require.NoError(t, err)

	attempt := "attempt-fail"
	lister.receipts = []WorkspaceCommandReceiptSummary{{
		CommandID:     "cmd-1",
		CommandType:   runtimeCommandCloneProjectRepository,
		RuntimeNodeID: nodeID,
		Status:        "failed",
		Payload:       map[string]any{workspaceCloneAttemptPayloadKey: attempt},
	}}

	err = service.HandleCloneCommandTerminal(context.Background(), CloneCommandTerminal{
		TenantID:      tenantID,
		ProjectID:     created.Project.ID,
		RuntimeNodeID: nodeID,
		Success:       false,
		ErrorMessage:  "clone rejected",
		Payload:       map[string]any{workspaceCloneAttemptPayloadKey: attempt},
	})
	require.NoError(t, err)
	got, err := service.GetProject(context.Background(), tenantID, created.Project.ID)
	require.NoError(t, err)
	require.Equal(t, WorkspaceReadyStatusError, got.WorkspaceReadyStatus)
	require.NotNil(t, got.WorkspaceReadyError)
	require.Contains(t, *got.WorkspaceReadyError, "clone rejected")
}

func TestHandleCloneCommandTerminalDoesNotDowngradeReady(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	ownerID := uuid.New()
	nodeA := uuid.New()
	nodeB := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, nodeA, nodeB)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "git-ready",
		Goal:             "keep ready",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{nodeA, nodeB},
		RepoBinding: &ProjectRepoBindingInput{
			URL:           "https://example.com/acme/app.git",
			DefaultBranch: "main",
		},
	})
	require.NoError(t, err)
	require.NoError(t, service.HandleCloneCommandTerminal(context.Background(), CloneCommandTerminal{
		TenantID: tenantID, ProjectID: created.Project.ID, RuntimeNodeID: nodeA, Success: true,
		Payload: map[string]any{workspaceCloneAttemptPayloadKey: "attempt-ok"},
	}))
	require.NoError(t, service.HandleCloneCommandTerminal(context.Background(), CloneCommandTerminal{
		TenantID: tenantID, ProjectID: created.Project.ID, RuntimeNodeID: nodeB, Success: false, ErrorMessage: "late fail",
		Payload: map[string]any{workspaceCloneAttemptPayloadKey: "attempt-ok"},
	}))
	got, err := service.GetProject(context.Background(), tenantID, created.Project.ID)
	require.NoError(t, err)
	require.Equal(t, WorkspaceReadyStatusReady, got.WorkspaceReadyStatus)
}

func TestHandleCloneCommandTerminalListReceiptsFailureDoesNotMarkError(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	service.SetProjectWorkspaceReceiptLister(&memoryWorkspaceReceiptLister{err: errListReceiptsBoom})

	tenantID := uuid.New()
	ownerID := uuid.New()
	nodeID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, nodeID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "git-list-fail",
		Goal:             "stay pending",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{nodeID},
		RepoBinding: &ProjectRepoBindingInput{
			URL:           "https://example.com/acme/app.git",
			DefaultBranch: "main",
		},
	})
	require.NoError(t, err)

	require.NoError(t, service.HandleCloneCommandTerminal(context.Background(), CloneCommandTerminal{
		TenantID:      tenantID,
		ProjectID:     created.Project.ID,
		RuntimeNodeID: nodeID,
		Success:       false,
		ErrorMessage:  "clone rejected",
		Payload:       map[string]any{workspaceCloneAttemptPayloadKey: "attempt-x"},
	}))
	got, err := service.GetProject(context.Background(), tenantID, created.Project.ID)
	require.NoError(t, err)
	require.Equal(t, WorkspaceReadyStatusPending, got.WorkspaceReadyStatus)
}

func TestHandleCloneCommandTerminalIgnoresFailureWithoutAttemptID(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	service.SetProjectWorkspaceReceiptLister(&memoryWorkspaceReceiptLister{})

	tenantID := uuid.New()
	ownerID := uuid.New()
	nodeID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, nodeID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "git-no-attempt",
		Goal:             "stay pending",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{nodeID},
		RepoBinding: &ProjectRepoBindingInput{
			URL:           "https://example.com/acme/app.git",
			DefaultBranch: "main",
		},
	})
	require.NoError(t, err)

	require.NoError(t, service.HandleCloneCommandTerminal(context.Background(), CloneCommandTerminal{
		TenantID:      tenantID,
		ProjectID:     created.Project.ID,
		RuntimeNodeID: nodeID,
		Success:       false,
		ErrorMessage:  "clone rejected",
	}))
	got, err := service.GetProject(context.Background(), tenantID, created.Project.ID)
	require.NoError(t, err)
	require.Equal(t, WorkspaceReadyStatusPending, got.WorkspaceReadyStatus)
}

func TestHandleCloneCommandTerminalIgnoresStaleSuccess(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	nodeA := uuid.New()
	nodeB := uuid.New()
	lister := &memoryWorkspaceReceiptLister{receipts: []WorkspaceCommandReceiptSummary{{
		CommandID:     "cmd-new",
		CommandType:   runtimeCommandCloneProjectRepository,
		RuntimeNodeID: nodeB,
		Status:        "pending",
		Payload:       map[string]any{workspaceCloneAttemptPayloadKey: "attempt-new"},
	}}}
	service.SetProjectWorkspaceReceiptLister(lister)

	tenantID := uuid.New()
	ownerID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, nodeA, nodeB)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "git-stale",
		Goal:             "ignore stale",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{nodeA, nodeB},
		RepoBinding: &ProjectRepoBindingInput{
			URL:           "https://example.com/acme/app.git",
			DefaultBranch: "main",
		},
	})
	require.NoError(t, err)

	require.NoError(t, service.HandleCloneCommandTerminal(context.Background(), CloneCommandTerminal{
		TenantID:      tenantID,
		ProjectID:     created.Project.ID,
		RuntimeNodeID: nodeA,
		Success:       true,
		Payload:       map[string]any{workspaceCloneAttemptPayloadKey: "attempt-old"},
	}))
	got, err := service.GetProject(context.Background(), tenantID, created.Project.ID)
	require.NoError(t, err)
	require.Equal(t, WorkspaceReadyStatusPending, got.WorkspaceReadyStatus)
}

func TestMarkProjectWorkspaceReadyManually(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	ownerID := uuid.New()
	nodeID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, nodeID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "git-manual",
		Goal:             "mark ready",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{nodeID},
		RepoBinding: &ProjectRepoBindingInput{
			URL:           "https://example.com/acme/app.git",
			DefaultBranch: "main",
		},
	})
	require.NoError(t, err)
	updated, err := service.MarkProjectWorkspaceReadyManually(context.Background(), WorkspaceManualActionRequest{
		TenantID: tenantID, ProjectID: created.Project.ID, ActorUserID: ownerID, Reason: "fixed disk",
	})
	require.NoError(t, err)
	require.Equal(t, WorkspaceReadyStatusReady, updated.WorkspaceReadyStatus)
}

type memoryWorkspaceReceiptLister struct {
	receipts []WorkspaceCommandReceiptSummary
	err      error
}

func (m *memoryWorkspaceReceiptLister) ListProjectWorkspaceReceipts(context.Context, uuid.UUID, uuid.UUID, string, int32) ([]WorkspaceCommandReceiptSummary, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append([]WorkspaceCommandReceiptSummary(nil), m.receipts...), nil
}
